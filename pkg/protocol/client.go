// Package protocol provides the daemon client implementation.
package protocol

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultSocketPath is the default Unix socket path for the daemon.
const DefaultSocketPath = "/var/run/vpn-manager/vpn-managerd.sock"

// DefaultTimeout is the default timeout for RPC calls.
const DefaultTimeout = 30 * time.Second

// Client communicates with the VPN Manager daemon over Unix socket.
// It handles connection management, request/response matching, and reconnection.
// Client is safe for concurrent use from multiple goroutines.
type Client struct {
	socketPath string
	timeout    time.Duration

	mu        sync.Mutex
	codec     *Codec
	connected bool

	// Request ID counter (atomic for thread safety)
	nextID atomic.Int64

	// Pending requests waiting for responses
	pending   map[int]chan *Response
	pendingMu sync.Mutex

	// Done channel for shutdown
	done chan struct{}
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithSocketPath sets a custom socket path.
func WithSocketPath(path string) ClientOption {
	return func(c *Client) {
		c.socketPath = path
	}
}

// WithTimeout sets the default timeout for RPC calls.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = d
	}
}

// NewClient creates a new daemon client with the given options.
// The client does not connect until Connect() is called.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		socketPath: DefaultSocketPath,
		timeout:    DefaultTimeout,
		pending:    make(map[int]chan *Response),
		done:       make(chan struct{}),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Connect establishes a connection to the daemon.
// Returns ErrDaemonUnavailable if the socket does not exist.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// Check if socket exists
	if _, err := os.Stat(c.socketPath); os.IsNotExist(err) {
		return ErrDaemonUnavailable
	}

	// Connect with context deadline
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("dial daemon: %w", err)
	}

	c.codec = NewCodec(conn)
	c.connected = true

	// Start response reader goroutine
	go c.readResponses()

	return nil
}

// IsConnected returns true if the client is connected to the daemon.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	// Signal shutdown
	close(c.done)

	// Close codec
	if c.codec != nil {
		if err := c.codec.Close(); err != nil {
			return err
		}
	}

	c.connected = false

	// Cancel all pending requests
	c.pendingMu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

	return nil
}

// Call invokes a remote method and waits for the response.
// The params and result should be pointers to the appropriate types.
// Returns error if the call fails or the response contains an error.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	// Ensure we're connected
	if !c.IsConnected() {
		if err := c.Connect(ctx); err != nil {
			return err
		}
	}

	// Generate request ID
	id := int(c.nextID.Add(1))

	// Create request
	req, err := NewRequest(id, method, params)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Register pending response channel
	respCh := make(chan *Response, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	// Clean up on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// Send request
	c.mu.Lock()
	if c.codec == nil {
		c.mu.Unlock()
		return ErrConnectionClosed
	}
	err = c.codec.WriteRequest(req)
	c.mu.Unlock()

	if err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	// Wait for response with timeout
	timeout := c.timeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	select {
	case resp, ok := <-respCh:
		if !ok {
			return ErrConnectionClosed
		}
		return c.handleResponse(resp, result)

	case <-time.After(timeout):
		return ErrTimeout

	case <-ctx.Done():
		return ctx.Err()

	case <-c.done:
		return ErrConnectionClosed
	}
}

// handleResponse processes a response and extracts the result.
func (c *Client) handleResponse(resp *Response, result any) error {
	if resp.Error != nil {
		return resp.Error
	}

	if result != nil && resp.Result != nil {
		if err := resp.UnmarshalResult(result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	return nil
}

// readResponses continuously reads responses and dispatches to waiting callers.
func (c *Client) readResponses() {
	for {
		select {
		case <-c.done:
			return
		default:
		}

		c.mu.Lock()
		codec := c.codec
		c.mu.Unlock()

		if codec == nil {
			return
		}

		resp, err := codec.ReadResponse()
		if err != nil {
			// Connection closed or error - close client
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			return
		}

		// Dispatch to waiting caller
		c.pendingMu.Lock()
		if ch, ok := c.pending[resp.ID]; ok {
			select {
			case ch <- resp:
			default:
				// Channel full or closed, ignore
			}
		}
		c.pendingMu.Unlock()
	}
}

// Ping sends a ping request to verify the daemon is responsive.
func (c *Client) Ping(ctx context.Context) error {
	var result string
	return c.Call(ctx, "system.ping", nil, &result)
}

// IsDaemonAvailable checks if the daemon socket exists.
// This is a quick check that doesn't establish a connection.
func IsDaemonAvailable() bool {
	_, err := os.Stat(DefaultSocketPath)
	return err == nil
}

// IsDaemonAvailableAt checks if a daemon socket exists at the given path.
func IsDaemonAvailableAt(socketPath string) bool {
	_, err := os.Stat(socketPath)
	return err == nil
}
