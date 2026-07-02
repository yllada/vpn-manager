// Package daemon implements the privileged VPN Manager daemon.
// It listens on a Unix socket and handles requests from unprivileged clients.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/yllada/vpn-manager/pkg/protocol"
)

// DefaultHandlerTimeout is the default timeout for RPC handler execution.
const DefaultHandlerTimeout = 30 * time.Second

// Connection limits bound resource use so a buggy or hostile client cannot
// exhaust the root daemon's goroutines/file descriptors/memory.
const (
	// maxConcurrentClients caps total simultaneous connections.
	maxConcurrentClients = 64
	// maxClientsPerUID caps simultaneous connections from a single UID, so one
	// user (or one runaway process) cannot consume the whole global budget.
	maxClientsPerUID = 8
)

// DefaultSocketGroup is the system group granted access to the daemon socket.
// The socket is created root:<this group> with mode 0660, so only root and
// members of this group can talk to the daemon. Packaging creates the group and
// adds the installing user to it.
const DefaultSocketGroup = "vpn-manager"

// methodTimeouts defines custom timeouts for specific methods that need more time.
// Methods not in this map use DefaultHandlerTimeout.
var methodTimeouts = map[string]time.Duration{
	"openvpn.connect":   60 * time.Second,  // VPN connection can take time
	"wireguard.connect": 60 * time.Second,  // WireGuard setup
	"tailscale.up":      60 * time.Second,  // Tailscale connection
	"tailscale.login":   120 * time.Second, // May require browser auth
}

// getMethodTimeout returns the timeout for a given method.
// Returns the override if one exists, otherwise DefaultHandlerTimeout.
func getMethodTimeout(method string) time.Duration {
	if t, ok := methodTimeouts[method]; ok {
		return t
	}
	return DefaultHandlerTimeout
}

// Server is the daemon server that handles client connections.
type Server struct {
	socketPath  string
	socketGroup string
	listener    net.Listener

	// Handler registry
	handlers *HandlerRegistry

	// State management
	state *State

	// Client tracking
	clients   map[*clientConn]struct{}
	clientsMu sync.RWMutex

	// Shutdown coordination
	done   chan struct{}
	wg     sync.WaitGroup
	logger *log.Logger
}

// clientConn represents a connected client.
type clientConn struct {
	conn   net.Conn
	codec  *protocol.Codec
	uid    uint32
	gid    uint32
	pid    int32
	server *Server
}

// ServerOption configures the Server.
type ServerOption func(*Server)

// WithSocketPath sets a custom socket path for the server.
func WithSocketPath(path string) ServerOption {
	return func(s *Server) {
		s.socketPath = path
	}
}

// WithLogger sets a custom logger for the server.
func WithLogger(logger *log.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// WithSocketGroup sets the system group granted access to the daemon socket.
// An empty value keeps DefaultSocketGroup.
func WithSocketGroup(group string) ServerOption {
	return func(s *Server) {
		if group != "" {
			s.socketGroup = group
		}
	}
}

// NewServer creates a new daemon server with the given options.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		socketPath:  protocol.DefaultSocketPath,
		socketGroup: DefaultSocketGroup,
		handlers:    NewHandlerRegistry(),
		state:       NewState(),
		clients:     make(map[*clientConn]struct{}),
		done:        make(chan struct{}),
		logger:      log.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start begins listening for client connections.
// It creates the socket directory if needed and removes any stale socket.
func (s *Server) Start(ctx context.Context) error {
	// Ensure socket directory exists
	socketDir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}

	// Remove stale socket if exists
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	// SECURITY (C3): create the socket with restrictive permissions from birth.
	// Otherwise there is a race window between net.Listen (which creates the socket
	// using the process umask, potentially world-accessible) and the chmod/chown in
	// secureSocket, during which a hostile local process could connect. A temporary
	// umask of 0177 forces the new socket to at most 0600; secureSocket then relaxes
	// it to 0660 for the desktop group.
	oldUmask := syscall.Umask(0177)
	listener, err := net.Listen("unix", s.socketPath)
	syscall.Umask(oldUmask)
	if err != nil {
		return fmt.Errorf("listen on socket: %w", err)
	}
	s.listener = listener

	// SECURITY (C3): restrict the socket to root and the desktop group instead of
	// world-accessible 0666. This is the primary access boundary — a world-writable
	// socket lets any local process (a malicious npm/pip postinstall, a compromised
	// browser tab) drive the root daemon.
	if err := s.secureSocket(); err != nil {
		_ = s.listener.Close()
		return fmt.Errorf("secure socket: %w", err)
	}

	s.logger.Printf("Daemon listening on %s", s.socketPath)

	// Register built-in handlers
	s.registerBuiltinHandlers()

	// Start accepting connections
	s.wg.Add(1)
	go s.acceptLoop(ctx)

	return nil
}

// secureSocket sets ownership and permissions on the listening socket so that
// only root and members of the configured desktop group may connect.
//
// It looks up the group by name; when the group exists the socket becomes
// root:<group> mode 0660. When the group is missing (e.g. packaging did not
// create it), it FAILS CLOSED to 0660 root:root and logs a prominent warning:
// the daemon stays secure, but the GUI cannot connect until the group is created
// and the user is added to it. It never falls back to a world-accessible mode.
func (s *Server) secureSocket() error {
	if err := os.Chmod(s.socketPath, 0660); err != nil {
		return fmt.Errorf("chmod 0660: %w", err)
	}

	grp, err := user.LookupGroup(s.socketGroup)
	if err != nil {
		s.logger.Printf("WARN: socket group %q not found (%v). Socket locked to root only (0660 root:root); "+
			"the GUI will not be able to connect. Create the group and add your user: "+
			"`sudo groupadd -f %s && sudo usermod -aG %s $USER`, then re-login and restart the daemon.",
			s.socketGroup, err, s.socketGroup, s.socketGroup)
		return nil
	}

	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		return fmt.Errorf("parse gid for group %q: %w", s.socketGroup, err)
	}

	// Owner root (uid 0, the daemon's own uid), group = desktop group.
	if err := os.Chown(s.socketPath, 0, gid); err != nil {
		return fmt.Errorf("chown socket to group %q: %w", s.socketGroup, err)
	}

	s.logger.Printf("Socket secured: %s (root:%s, mode 0660)", s.socketPath, s.socketGroup)
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	s.logger.Println("Daemon shutting down...")

	// Signal shutdown
	close(s.done)

	// Close listener to stop accepting new connections
	if s.listener != nil {
		_ = s.listener.Close()
	}

	// Close all client connections
	s.clientsMu.Lock()
	for client := range s.clients {
		_ = client.conn.Close()
	}
	s.clientsMu.Unlock()

	// Wait for goroutines to finish
	s.wg.Wait()

	// Remove socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		s.logger.Printf("Warning: failed to remove socket: %v", err)
	}

	s.logger.Println("Daemon stopped")
	return nil
}

// acceptLoop accepts incoming connections.
func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			// Check if we're shutting down
			select {
			case <-s.done:
				return
			default:
				s.logger.Printf("Accept error: %v", err)
				continue
			}
		}

		// Get peer credentials for authentication
		unixConn, ok := conn.(*net.UnixConn)
		if !ok {
			s.logger.Printf("Connection is not a Unix socket")
			_ = conn.Close()
			continue
		}

		creds, err := getPeerCredentials(unixConn)
		if err != nil {
			s.logger.Printf("Failed to get peer credentials: %v", err)
			_ = conn.Close()
			continue
		}

		// Create client connection
		client := &clientConn{
			conn:   conn,
			codec:  protocol.NewCodec(conn),
			uid:    creds.Uid,
			gid:    creds.Gid,
			pid:    creds.Pid,
			server: s,
		}

		// Enforce connection limits atomically before registering. Rejecting here
		// (rather than after spawning a goroutine) keeps a flood from exhausting
		// the daemon's resources.
		if !s.registerClient(client) {
			s.logger.Printf("Rejecting connection from uid=%d pid=%d: connection limit reached", creds.Uid, creds.Pid)
			_ = conn.Close()
			continue
		}

		s.logger.Printf("Client connected: uid=%d pid=%d", creds.Uid, creds.Pid)

		// Handle client in goroutine
		s.wg.Add(1)
		go s.handleClient(client)
	}
}

// registerClient enforces the connection caps and, if within limits, registers
// the client. It returns false (without registering) when the total or per-UID
// limit would be exceeded. Root (uid 0) is exempt from the per-UID cap so system
// callers are never starved by a misbehaving user process.
func (s *Server) registerClient(client *clientConn) bool {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	if len(s.clients) >= maxConcurrentClients {
		return false
	}
	if client.uid != 0 {
		perUID := 0
		for c := range s.clients {
			if c.uid == client.uid {
				perUID++
			}
		}
		if perUID >= maxClientsPerUID {
			return false
		}
	}

	s.clients[client] = struct{}{}
	return true
}

// handleClient processes requests from a single client.
func (s *Server) handleClient(client *clientConn) {
	defer s.wg.Done()
	defer func() {
		// Unregister client
		s.clientsMu.Lock()
		delete(s.clients, client)
		s.clientsMu.Unlock()

		_ = client.conn.Close()
		s.logger.Printf("Client disconnected: uid=%d pid=%d", client.uid, client.pid)
	}()

	for {
		select {
		case <-s.done:
			return
		default:
		}

		// Read request
		req, err := client.codec.ReadRequest()
		if err != nil {
			if errors.Is(err, protocol.ErrConnectionClosed) {
				return
			}
			s.logger.Printf("Read error from uid=%d: %v", client.uid, err)
			return
		}

		// Process request
		resp := s.processRequest(client, req)

		// Send response
		if err := client.codec.WriteResponse(resp); err != nil {
			s.logger.Printf("Write error to uid=%d: %v", client.uid, err)
			return
		}
	}
}

// processRequest routes a request to the appropriate handler.
func (s *Server) processRequest(client *clientConn, req *protocol.Request) *protocol.Response {
	// Find handler
	handler, ok := s.handlers.Get(req.Method)
	if !ok {
		return protocol.MethodNotFoundError(req.ID, req.Method)
	}

	// Check authorization
	if !s.isAuthorized(client, req.Method) {
		s.logger.Printf("Unauthorized request from uid=%d: %s", client.uid, req.Method)
		return protocol.UnauthorizedError(req.ID)
	}

	// Audit trail: record privileged (state-mutating) invocations with caller
	// identity. This does not gate access — it provides forensics for the residual
	// "same-user process" risk that the socket-group model cannot eliminate.
	if isPrivilegedMethod(req.Method) {
		s.logger.Printf("AUDIT: privileged call %s by uid=%d pid=%d", req.Method, client.uid, client.pid)
	}

	// Create context with timeout for this request
	timeout := getMethodTimeout(req.Method)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create handler context
	handlerCtx := &HandlerContext{
		Context: ctx,
		Request: req,
		UID:     client.uid,
		GID:     client.gid,
		PID:     client.pid,
		State:   s.state,
		Logger:  s.logger,
	}

	// Execute handler
	result, err := handler(handlerCtx)
	if err != nil {
		// Check if it was a timeout
		if errors.Is(err, context.DeadlineExceeded) {
			s.logger.Printf("WARN: Handler timeout for %s after %v (uid=%d)", req.Method, timeout, client.uid)
			return protocol.NewErrorResponse(req.ID, protocol.ErrCodeTimeout, "Operation timed out", nil)
		}
		s.logger.Printf("Handler error for %s: %v", req.Method, err)
		return protocol.OperationFailedError(req.ID, err)
	}

	// Create success response
	resp, err := protocol.NewResponse(req.ID, result)
	if err != nil {
		return protocol.InternalError(req.ID, err)
	}

	return resp
}

// isAuthorized applies a per-request UID floor. It is a secondary sanity check;
// the real, kernel-enforced authorization boundary is the socket's group ownership
// (root:<group>, mode 0660 — see secureSocket and authz.go), which already limits
// connections to root and desktop-group members.
//
//   - UID 0 (root): allowed — system scripts and daemon self-calls.
//   - UID 1–999 (system service accounts): denied — no legitimate reason to drive VPNs.
//   - UID 65534 (nobody) / 65535 (overflow sentinel): denied.
//   - UID ≥ 1000: allowed. A connected regular user is already a socket-group
//     member; the daemon does not withhold any method from such a user (the GUI
//     legitimately drives all operations). See authz.go for the residual-risk note.
//
// The method argument is intentionally not used to grant/deny here; per-method
// classification is used only for audit logging in processRequest.
func (s *Server) isAuthorized(client *clientConn, _ string) bool {
	if client.uid == 0 {
		return true
	}
	if client.uid == 65534 || client.uid == 65535 {
		return false
	}
	return client.uid >= 1000
}

// registerBuiltinHandlers registers the built-in system handlers.
func (s *Server) registerBuiltinHandlers() {
	// System handlers
	s.handlers.Register("system.ping", handlePing)
	s.handlers.Register("system.version", handleVersion)
	s.handlers.Register("state.get", handleGetState)
}

// Handlers returns the handler registry for registering custom handlers.
func (s *Server) Handlers() *HandlerRegistry {
	return s.handlers
}

// State returns the server state for reading/modifying daemon state.
func (s *Server) State() *State {
	return s.state
}

// BroadcastEvent sends an event to all connected clients.
// This is used to notify clients of state changes.
func (s *Server) BroadcastEvent(eventType string, data any) {
	notification, err := protocol.NewRequest(0, "event."+eventType, data)
	if err != nil {
		s.logger.Printf("Failed to create notification: %v", err)
		return
	}

	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for client := range s.clients {
		if err := client.codec.WriteRequest(notification); err != nil {
			s.logger.Printf("Failed to send notification to client: %v", err)
		}
	}
}

// getPeerCredentials retrieves the credentials of the connected peer.
func getPeerCredentials(conn *net.UnixConn) (*syscall.Ucred, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("get syscall conn: %w", err)
	}

	var creds *syscall.Ucred
	var credErr error

	err = raw.Control(func(fd uintptr) {
		creds, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})

	if err != nil {
		return nil, fmt.Errorf("control: %w", err)
	}
	if credErr != nil {
		return nil, fmt.Errorf("getsockopt: %w", credErr)
	}

	return creds, nil
}

// Built-in handlers

func handlePing(ctx *HandlerContext) (any, error) {
	return "pong", nil
}

func handleVersion(ctx *HandlerContext) (any, error) {
	return map[string]string{
		"version": "1.0.0",
		"name":    "vpn-managerd",
	}, nil
}

func handleGetState(ctx *HandlerContext) (any, error) {
	return ctx.State.Snapshot(), nil
}
