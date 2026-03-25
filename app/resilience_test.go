package app

import (
	"context"
	"errors"
	"sync/atomic"
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

func TestRetry_SuccessFirstTry(t *testing.T) {
	config := DefaultRetryConfig()
	retry := NewRetry(config)
	ctx := context.Background()

	attempts := 0
	err := retry.Do(ctx, func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got: %d", attempts)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialDelay = 1 * time.Millisecond
	retry := NewRetry(config)
	ctx := context.Background()

	attempts := 0
	err := retry.Do(ctx, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}
}

func TestRetry_MaxAttempts(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.InitialDelay = 1 * time.Millisecond
	retry := NewRetry(config)
	ctx := context.Background()

	attempts := 0
	err := retry.Do(ctx, func() error {
		attempts++
		return errors.New("always fails")
	})

	if err == nil {
		t.Error("Expected error after max attempts")
	}
	if attempts != 3 {
		t.Errorf("Expected %d attempts, got: %d", config.MaxAttempts, attempts)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialDelay = 100 * time.Millisecond
	retry := NewRetry(config)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := retry.Do(ctx, func() error {
		return errors.New("always fails")
	})

	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Error("Should have cancelled quickly")
	}

	if err == nil {
		t.Error("Expected error after cancellation")
	}
}

func TestRetry_OnRetryCallback(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.InitialDelay = 1 * time.Millisecond

	var retryCount int32
	config.OnRetry = func(attempt int, err error, delay time.Duration) {
		atomic.AddInt32(&retryCount, 1)
	}

	retry := NewRetry(config)
	ctx := context.Background()

	_ = retry.Do(ctx, func() error {
		return errors.New("always fails")
	})

	if atomic.LoadInt32(&retryCount) != 2 { // 3 attempts = 2 retries
		t.Errorf("Expected 2 retry callbacks, got: %d", retryCount)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(5, 1.0) // 5 tokens, 1 per second refill

	// Should allow 5 requests immediately
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th should be denied
	if rl.Allow() {
		t.Error("Request 6 should be denied")
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	rl := NewRateLimiter(2, 100.0) // 2 tokens, 100 per second

	// Use all tokens
	rl.Allow()
	rl.Allow()

	if rl.Allow() {
		t.Error("Should be denied after using all tokens")
	}

	// Wait for refill
	time.Sleep(30 * time.Millisecond)

	// Should have tokens again
	if !rl.Allow() {
		t.Error("Should be allowed after refill")
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(1, 10.0) // 1 token, 10 per second

	rl.Allow() // Use the token

	ctx := context.Background()
	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Wait should succeed, got: %v", err)
	}

	if elapsed < 50*time.Millisecond {
		t.Error("Wait should have waited for refill")
	}
}

func TestRateLimiter_WaitWithCancel(t *testing.T) {
	rl := NewRateLimiter(1, 0.1) // 1 token, very slow refill

	rl.Allow() // Use the token

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	if err == nil {
		t.Error("Wait should return error on timeout")
	}
}
