// Package daemon provides the handler registry and context for RPC handlers.
package daemon

import (
	"context"
	"log"
	"sync"

	"github.com/yllada/vpn-manager/protocol"
)

// HandlerFunc is the signature for RPC method handlers.
// It receives a context with request info and state, and returns a result or error.
type HandlerFunc func(ctx *HandlerContext) (any, error)

// HandlerContext provides context information to handlers.
type HandlerContext struct {
	// Context is the request context (for cancellation, deadlines).
	Context context.Context

	// Request is the incoming RPC request.
	Request *protocol.Request

	// UID is the Unix user ID of the client.
	UID uint32

	// GID is the Unix group ID of the client.
	GID uint32

	// PID is the process ID of the client.
	PID int32

	// State provides access to daemon state.
	State *State

	// Logger for handler logging.
	Logger *log.Logger
}

// UnmarshalParams extracts request parameters into the target struct.
func (ctx *HandlerContext) UnmarshalParams(target any) error {
	return ctx.Request.UnmarshalParams(target)
}

// HandlerRegistry manages registered RPC method handlers.
// It is safe for concurrent use.
type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]HandlerFunc
}

// NewHandlerRegistry creates a new handler registry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]HandlerFunc),
	}
}

// Register adds a handler for the given method.
// Panics if a handler is already registered for the method.
func (r *HandlerRegistry) Register(method string, handler HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[method]; exists {
		panic("handler already registered for method: " + method)
	}

	r.handlers[method] = handler
}

// Get returns the handler for the given method.
// Returns false if no handler is registered.
func (r *HandlerRegistry) Get(method string) (HandlerFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, ok := r.handlers[method]
	return handler, ok
}

// Methods returns a list of all registered method names.
func (r *HandlerRegistry) Methods() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	methods := make([]string, 0, len(r.handlers))
	for method := range r.handlers {
		methods = append(methods, method)
	}
	return methods
}
