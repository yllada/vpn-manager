// Package app provides safe goroutine execution with panic recovery.
// This prevents the application from crashing silently when a goroutine panics.
package app

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// SafeGo launches a goroutine with panic recovery.
// If the goroutine panics, the panic is logged with a full stack trace
// and the application continues running.
//
// Usage:
//
//	app.SafeGo(func() {
//	    // your code here
//	})
func SafeGo(fn func()) {
	go func() {
		defer RecoverPanic("goroutine")
		fn()
	}()
}

// SafeGoWithName launches a goroutine with panic recovery and a descriptive name.
// The name is included in the log message if a panic occurs.
//
// Usage:
//
//	app.SafeGoWithName("uptimeCounter", func() {
//	    // your code here
//	})
func SafeGoWithName(name string, fn func()) {
	go func() {
		defer RecoverPanic(name)
		fn()
	}()
}

// RecoverPanic recovers from a panic and logs it.
// This should be called with defer at the start of a goroutine.
//
// Usage:
//
//	go func() {
//	    defer app.RecoverPanic("myFunction")
//	    // your code here
//	}()
func RecoverPanic(context string) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		LogError("PANIC RECOVERED [%s]: %v\nStack trace:\n%s", context, r, string(stack))
	}
}

// RecoverPanicWithCallback recovers from a panic, logs it, and calls a callback.
// Useful when you need to perform cleanup or notify another component.
//
// Usage:
//
//	go func() {
//	    defer app.RecoverPanicWithCallback("myFunction", func(err interface{}) {
//	        // handle the panic
//	    })
//	    // your code here
//	}()
func RecoverPanicWithCallback(context string, callback func(interface{})) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		LogError("PANIC RECOVERED [%s]: %v\nStack trace:\n%s", context, r, string(stack))
		if callback != nil {
			callback(r)
		}
	}
}

// SafeChannel provides a thread-safe channel wrapper that prevents
// panics from writing to a closed channel.
type SafeChannel[T any] struct {
	ch     chan T
	closed bool
	mu     sync.RWMutex
	once   sync.Once
}

// NewSafeChannel creates a new SafeChannel with the given buffer size.
func NewSafeChannel[T any](size int) *SafeChannel[T] {
	return &SafeChannel[T]{
		ch: make(chan T, size),
	}
}

// Send sends a value to the channel if it's not closed.
// Returns false if the channel is closed.
func (sc *SafeChannel[T]) Send(value T) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.closed {
		return false
	}

	select {
	case sc.ch <- value:
		return true
	default:
		// Channel buffer is full
		return false
	}
}

// SendBlocking sends a value to the channel, blocking if necessary.
// Returns false if the channel is closed.
func (sc *SafeChannel[T]) SendBlocking(value T) bool {
	sc.mu.RLock()
	if sc.closed {
		sc.mu.RUnlock()
		return false
	}
	sc.mu.RUnlock()

	// Note: There's a small race window here, but it's acceptable
	// because the defer recover will catch any panic
	defer func() {
		if r := recover(); r != nil {
			LogWarn("SafeChannel: attempted to send on closed channel")
		}
	}()

	sc.ch <- value
	return true
}

// Receive returns the underlying channel for receiving values.
func (sc *SafeChannel[T]) Receive() <-chan T {
	return sc.ch
}

// Close closes the channel safely (only once).
func (sc *SafeChannel[T]) Close() {
	sc.once.Do(func() {
		sc.mu.Lock()
		sc.closed = true
		sc.mu.Unlock()
		close(sc.ch)
	})
}

// IsClosed returns whether the channel has been closed.
func (sc *SafeChannel[T]) IsClosed() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.closed
}

// SafeClose safely closes a channel, preventing double-close panics.
// Returns true if the channel was closed, false if it was already closed.
func SafeClose[T any](ch chan T, closeOnce *sync.Once) {
	if closeOnce == nil {
		// Without sync.Once, we use recover
		defer func() {
			if r := recover(); r != nil {
				LogDebug("SafeClose: channel was already closed")
			}
		}()
		close(ch)
		return
	}

	closeOnce.Do(func() {
		close(ch)
	})
}

// WrapGoroutine is a helper that wraps a function for safe execution.
// Returns a function that can be passed to go keyword.
//
// Usage:
//
//	go app.WrapGoroutine("myTask", func() {
//	    // your code
//	})()
func WrapGoroutine(name string, fn func()) func() {
	return func() {
		defer RecoverPanic(name)
		fn()
	}
}

// GoroutineGroup manages a group of goroutines with panic recovery.
type GoroutineGroup struct {
	wg      sync.WaitGroup
	errChan chan error
	errOnce sync.Once
	name    string
}

// NewGoroutineGroup creates a new goroutine group.
func NewGoroutineGroup(name string) *GoroutineGroup {
	return &GoroutineGroup{
		name:    name,
		errChan: make(chan error, 1),
	}
}

// Go launches a goroutine in the group with panic recovery.
func (g *GoroutineGroup) Go(fn func() error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				LogError("PANIC in GoroutineGroup [%s]: %v\nStack trace:\n%s", g.name, r, string(stack))
				g.errOnce.Do(func() {
					g.errChan <- fmt.Errorf("panic in %s: %v", g.name, r)
				})
			}
		}()

		if err := fn(); err != nil {
			g.errOnce.Do(func() {
				g.errChan <- err
			})
		}
	}()
}

// Wait waits for all goroutines to complete and returns the first error (if any).
func (g *GoroutineGroup) Wait() error {
	g.wg.Wait()
	close(g.errChan)
	return <-g.errChan
}
