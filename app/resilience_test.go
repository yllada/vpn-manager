package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	// Should allow requests when closed
	if err := cb.Allow(); err != nil {
		t.Errorf("Expected Allow() in closed state, got: %v", err)
	}

	// State should be closed
	if cb.State() != CircuitClosed {
		t.Errorf("Expected CircuitClosed, got: %v", cb.State())
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 3
	cb := NewCircuitBreaker(config)

	// Record failures
	for i := 0; i < 3; i++ {
		_ = cb.Allow()
		cb.RecordFailure()
	}

	// Should be open now
	if cb.State() != CircuitOpen {
		t.Errorf("Expected CircuitOpen after failures, got: %v", cb.State())
	}

	// Should not allow new requests
	if err := cb.Allow(); err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got: %v", err)
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.Timeout = 50 * time.Millisecond
	cb := NewCircuitBreaker(config)

	// Open the circuit
	_ = cb.Allow()
	cb.RecordFailure()
	_ = cb.Allow()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("Expected CircuitOpen, got: %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow one request
	if err := cb.Allow(); err != nil {
		t.Errorf("Expected Allow() after timeout, got: %v", err)
	}

	if cb.State() != CircuitHalfOpen {
		t.Errorf("Expected CircuitHalfOpen, got: %v", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterSuccesses(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.SuccessThreshold = 2
	config.Timeout = 10 * time.Millisecond
	cb := NewCircuitBreaker(config)

	// Open the circuit
	_ = cb.Allow()
	cb.RecordFailure()
	_ = cb.Allow()
	cb.RecordFailure()

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Transition to half-open
	_ = cb.Allow()
	cb.RecordSuccess()

	// Need another success to close
	_ = cb.Allow()
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Errorf("Expected CircuitClosed after successes, got: %v", cb.State())
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	ctx := context.Background()

	// Successful execution
	called := false
	err := cb.Execute(ctx, func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !called {
		t.Error("Function was not called")
	}
}

func TestCircuitBreaker_ExecuteWithError(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	ctx := context.Background()

	testErr := errors.New("test error")
	err := cb.Execute(ctx, func() error {
		return testErr
	})

	if !errors.Is(err, testErr) {
		t.Errorf("Expected test error, got: %v", err)
	}
}

// NOTE: Tests for Retry and RateLimiter removed - these structs were part of
// dead code that was never implemented in resilience.go. The source only has
// CircuitBreaker which is tested above.
