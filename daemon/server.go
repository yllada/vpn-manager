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
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/yllada/vpn-manager/pkg/protocol"
)

// DefaultHandlerTimeout is the default timeout for RPC handler execution.
const DefaultHandlerTimeout = 30 * time.Second

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
	socketPath string
	listener   net.Listener

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

// NewServer creates a new daemon server with the given options.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		socketPath: protocol.DefaultSocketPath,
		handlers:   NewHandlerRegistry(),
		state:      NewState(),
		clients:    make(map[*clientConn]struct{}),
		done:       make(chan struct{}),
		logger:     log.Default(),
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

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on socket: %w", err)
	}
	s.listener = listener

	// Set socket permissions (world read/write so unprivileged clients can connect)
	// Security note: The daemon validates operations via SO_PEERCRED (caller UID/GID)
	if err := os.Chmod(s.socketPath, 0666); err != nil {
		_ = s.listener.Close()
		return fmt.Errorf("set socket permissions: %w", err)
	}

	s.logger.Printf("Daemon listening on %s", s.socketPath)

	// Register built-in handlers
	s.registerBuiltinHandlers()

	// Start accepting connections
	s.wg.Add(1)
	go s.acceptLoop(ctx)

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

		// Register client
		s.clientsMu.Lock()
		s.clients[client] = struct{}{}
		s.clientsMu.Unlock()

		s.logger.Printf("Client connected: uid=%d pid=%d", creds.Uid, creds.Pid)

		// Handle client in goroutine
		s.wg.Add(1)
		go s.handleClient(client)
	}
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

// isAuthorized checks if the client is authorized for the given method.
// Uses SO_PEERCRED UID already captured at connection time.
//
// Policy:
//   - UID 0 (root): always authorized — system scripts and daemon self-calls
//   - UID ≥ 1000 (regular users): authorized — the GUI app runs as the logged-in user
//   - UID 1–999 (system service accounts): denied — no legitimate reason to control VPNs
//   - UID 65534 (nobody) and 65535: explicitly denied — overflow/sentinel UIDs
func (s *Server) isAuthorized(client *clientConn, method string) bool {
	if client.uid == 0 {
		return true
	}
	// Deny overflow/sentinel UIDs: nobody (65534) and the unsigned overflow sentinel (65535).
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
