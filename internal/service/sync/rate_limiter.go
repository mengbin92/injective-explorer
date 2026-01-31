// Package sync provides synchronization utilities for blockchain data.
package sync

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting functionality for external API calls.
// It wraps golang.org/x/time/rate.Limiter with additional convenience methods.
type RateLimiter struct {
	limiter *rate.Limiter
	mu      sync.RWMutex
}

// NewRateLimiter creates a new RateLimiter with the specified rate and burst.
//
// Parameters:
//   - r: The rate limit (requests per second)
//   - b: The burst size (maximum number of requests that can be made at once)
//
// Returns:
//   - *RateLimiter: A new rate limiter instance
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(r, b),
	}
}

// Wait blocks until the rate limiter allows the next request.
// It respects the context cancellation.
//
// Parameters:
//   - ctx: The context for cancellation
//
// Returns:
//   - error: Error if context is cancelled
func (rl *RateLimiter) Wait(ctx context.Context) error {
	return rl.limiter.Wait(ctx)
}

// Allow checks if a request is allowed without blocking.
//
// Returns:
//   - bool: True if the request is allowed, false otherwise
func (rl *RateLimiter) Allow() bool {
	return rl.limiter.Allow()
}

// Reserve reserves a token for a future request.
//
// Returns:
//   - *rate.Reservation: The reservation which can be used to delay
func (rl *RateLimiter) Reserve() *rate.Reservation {
	return rl.limiter.Reserve()
}

// Tokens returns the current number of available tokens.
//
// Returns:
//   - float64: The number of available tokens
func (rl *RateLimiter) Tokens() float64 {
	return rl.limiter.Tokens()
}

// CircuitBreakerState represents the state of a circuit breaker.
type CircuitBreakerState int

const (
	// StateClosed means the circuit is closed and requests are allowed.
	StateClosed CircuitBreakerState = iota
	// StateOpen means the circuit is open and requests are blocked.
	StateOpen
	// StateHalfOpen means the circuit is half-open and a test request is allowed.
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern for fault tolerance.
// It prevents cascading failures by blocking requests after a threshold of failures.
type CircuitBreaker struct {
	mu                sync.RWMutex
	state             CircuitBreakerState
	failureCount      int
	failureThreshold  int
	successThreshold  int
	timeout           time.Duration
	lastFailureTime   time.Time
	consecutiveSuccess int
}

// NewCircuitBreaker creates a new CircuitBreaker.
//
// Parameters:
//   - failureThreshold: Number of failures before opening the circuit
//   - successThreshold: Number of consecutive successes needed in half-open state to close
//   - timeout: Duration to wait before transitioning from open to half-open
//
// Returns:
//   - *CircuitBreaker: A new circuit breaker instance
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
	}
}

// Allow checks if a request should be allowed through the circuit breaker.
//
// Returns:
//   - bool: True if the request is allowed, false if it should be blocked
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = StateHalfOpen
			cb.consecutiveSuccess = 0
			return true
		}
		return false
	case StateHalfOpen:
		// In half-open state, allow one test request
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful request and updates the circuit state.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failureCount = 0
	case StateHalfOpen:
		cb.consecutiveSuccess++
		if cb.consecutiveSuccess >= cb.successThreshold {
			cb.state = StateClosed
			cb.failureCount = 0
			cb.consecutiveSuccess = 0
		}
	}
}

// RecordFailure records a failed request and updates the circuit state.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		cb.state = StateOpen
		cb.consecutiveSuccess = 0
	}
}

// State returns the current state of the circuit breaker.
//
// Returns:
//   - CircuitBreakerState: The current state
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// IsOpen checks if the circuit is open.
//
// Returns:
//   - bool: True if the circuit is open
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.State() == StateOpen
}

// IsClosed checks if the circuit is closed.
//
// Returns:
//   - bool: True if the circuit is closed
func (cb *CircuitBreaker) IsClosed() bool {
	return cb.State() == StateClosed
}

// Metrics returns the current metrics of the circuit breaker.
//
// Returns:
//   - state: Current state
//   - failureCount: Current failure count
//   - consecutiveSuccess: Current consecutive success count
func (cb *CircuitBreaker) Metrics() (state CircuitBreakerState, failureCount, consecutiveSuccess int) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state, cb.failureCount, cb.consecutiveSuccess
}
