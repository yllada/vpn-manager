// Package app provides resilience utilities for network operations.
// Implements Circuit Breaker pattern and exponential backoff following
// industry standards from cloud-native applications.
//
// References:
// - https://docs.microsoft.com/en-us/azure/architecture/patterns/circuit-breaker
// - https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
package app

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Circuit breaker errors
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrCircuitHalfOpen = errors.New("circuit breaker is half-open, limited requests allowed")
	ErrMaxRetries      = errors.New("maximum retry attempts exceeded")
	ErrContextCanceled = errors.New("operation canceled")
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int32

const (
	// CircuitClosed allows requests to pass through.
	CircuitClosed CircuitState = iota
	// CircuitOpen blocks all requests.
	CircuitOpen
	// CircuitHalfOpen allows limited requests to test recovery.
	CircuitHalfOpen
)

// String returns a human-readable state name.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures the circuit breaker behavior.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before opening the circuit.
	FailureThreshold int
	// SuccessThreshold is the number of successes needed to close from half-open.
	SuccessThreshold int
	// Timeout is how long the circuit stays open before transitioning to half-open.
	Timeout time.Duration
	// MaxConcurrentHalfOpen limits requests allowed in half-open state.
	MaxConcurrentHalfOpen int
	// OnStateChange is called when the circuit state changes.
	OnStateChange func(from, to CircuitState)
}

// DefaultCircuitBreakerConfig returns sensible defaults for VPN operations.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:      5,
		SuccessThreshold:      2,
		Timeout:               30 * time.Second,
		MaxConcurrentHalfOpen: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
// It prevents cascading failures by failing fast when a service is unhealthy.
type CircuitBreaker struct {
	config CircuitBreakerConfig

	mu               sync.RWMutex
	state            CircuitState
	failures         int
	successes        int
	lastFailure      time.Time
	halfOpenRequests int32
}

// NewCircuitBreaker creates a new circuit breaker with the given config.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 2
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxConcurrentHalfOpen <= 0 {
		config.MaxConcurrentHalfOpen = 1
	}

	return &CircuitBreaker{
		config: config,
		state:  CircuitClosed,
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Allow checks if a request should be allowed.
// Returns an error if the circuit is open.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return nil

	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailure) >= cb.config.Timeout {
			cb.transitionTo(CircuitHalfOpen)
			atomic.AddInt32(&cb.halfOpenRequests, 1)
			return nil
		}
		return ErrCircuitOpen

	case CircuitHalfOpen:
		// Allow limited concurrent requests
		current := atomic.LoadInt32(&cb.halfOpenRequests)
		if current >= int32(cb.config.MaxConcurrentHalfOpen) {
			return ErrCircuitHalfOpen
		}
		atomic.AddInt32(&cb.halfOpenRequests, 1)
		return nil
	}

	return nil
}

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		cb.failures = 0

	case CircuitHalfOpen:
		atomic.AddInt32(&cb.halfOpenRequests, -1)
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.transitionTo(CircuitClosed)
		}
	}
}

// RecordFailure records a failed operation.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailure = time.Now()

	switch cb.state {
	case CircuitClosed:
		cb.failures++
		if cb.failures >= cb.config.FailureThreshold {
			cb.transitionTo(CircuitOpen)
		}

	case CircuitHalfOpen:
		atomic.AddInt32(&cb.halfOpenRequests, -1)
		cb.transitionTo(CircuitOpen)
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	atomic.StoreInt32(&cb.halfOpenRequests, 0)
}

func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	cb.state = newState
	cb.failures = 0
	cb.successes = 0

	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(oldState, newState)
	}
}

// Execute runs the given function with circuit breaker protection.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	// Check context before execution
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// RetryConfig configures retry behavior with exponential backoff.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts (including initial).
	MaxAttempts int
	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration
	// MaxDelay caps the maximum delay between retries.
	MaxDelay time.Duration
	// Multiplier increases delay after each attempt.
	Multiplier float64
	// JitterFactor adds randomness to prevent thundering herd (0.0-1.0).
	JitterFactor float64
	// RetryableErrors defines which errors should trigger a retry.
	// If nil, all errors are retryable.
	RetryableErrors []error
	// OnRetry is called before each retry attempt.
	OnRetry func(attempt int, err error, delay time.Duration)
}

// DefaultRetryConfig returns sensible defaults for VPN operations.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.3,
	}
}

// Retry executes a function with exponential backoff retries.
type Retry struct {
	config RetryConfig
}

// NewRetry creates a new Retry with the given config.
func NewRetry(config RetryConfig) *Retry {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 1 * time.Second
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 60 * time.Second
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}
	if config.JitterFactor < 0 || config.JitterFactor > 1 {
		config.JitterFactor = 0.3
	}

	return &Retry{config: config}
}

// Do executes the function with retries on failure.
func (r *Retry) Do(ctx context.Context, fn func() error) error {
	var lastErr error
	delay := r.config.InitialDelay

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		// Check context
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !r.isRetryable(err) {
			return err
		}

		// Check if we have more attempts
		if attempt >= r.config.MaxAttempts {
			break
		}

		// Calculate delay with jitter
		jitteredDelay := r.addJitter(delay)

		// Call retry callback
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt, err, jitteredDelay)
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(jitteredDelay):
		}

		// Increase delay for next iteration
		delay = time.Duration(float64(delay) * r.config.Multiplier)
		if delay > r.config.MaxDelay {
			delay = r.config.MaxDelay
		}
	}

	return WrapError(lastErr, "max retries exceeded")
}

func (r *Retry) isRetryable(err error) bool {
	if r.config.RetryableErrors == nil {
		// All errors are retryable by default
		return true
	}

	for _, retryable := range r.config.RetryableErrors {
		if errors.Is(err, retryable) {
			return true
		}
	}

	return false
}

func (r *Retry) addJitter(d time.Duration) time.Duration {
	if r.config.JitterFactor == 0 {
		return d
	}

	jitter := float64(d) * r.config.JitterFactor
	return d + time.Duration(rand.Float64()*jitter)
}

// RetryWithBackoff is a convenience function for simple retry operations.
func RetryWithBackoff(ctx context.Context, maxAttempts int, fn func() error) error {
	config := DefaultRetryConfig()
	config.MaxAttempts = maxAttempts
	return NewRetry(config).Do(ctx, fn)
}

// ExecuteWithCircuitBreaker combines circuit breaker with retry logic.
func ExecuteWithCircuitBreaker(
	ctx context.Context,
	cb *CircuitBreaker,
	retry *Retry,
	fn func() error,
) error {
	return retry.Do(ctx, func() error {
		return cb.Execute(ctx, fn)
	})
}

// RateLimiter implements a simple token bucket rate limiter.
type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter creates a rate limiter.
// maxTokens is the bucket size, refillRate is tokens added per second.
func NewRateLimiter(maxTokens int, refillRate float64) *RateLimiter {
	return &RateLimiter{
		tokens:     float64(maxTokens),
		maxTokens:  float64(maxTokens),
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if the operation should be allowed.
func (rl *RateLimiter) Allow() bool {
	return rl.AllowN(1)
}

// AllowN checks if n tokens are available.
func (rl *RateLimiter) AllowN(n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		return true
	}

	return false
}

// Wait blocks until a token is available or context is canceled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Check again
		}
	}
}

func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.lastRefill = now

	rl.tokens = math.Min(rl.maxTokens, rl.tokens+elapsed*rl.refillRate)
}

// Throttle wraps a function with rate limiting.
func Throttle(ctx context.Context, rl *RateLimiter, fn func() error) error {
	if err := rl.Wait(ctx); err != nil {
		return err
	}
	return fn()
}
