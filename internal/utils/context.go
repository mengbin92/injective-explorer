// Package utils provides shared utility functions used across the application.
// It includes helpers for string manipulation, validation, pagination, caching,
// database operations, and context management.
package utils

import (
	"context"
	"time"
)

// DBQueryTimeout is the default timeout for database queries.
// This is used by WithDBTimeout to create contexts with appropriate timeout.
const DBQueryTimeout = 5 * time.Second

// CountQueryTimeout is the timeout for count queries.
const CountQueryTimeout = 2 * time.Second

// BlockQueryTimeout is the timeout for block-related queries.
const BlockQueryTimeout = 30 * time.Second

// AccountQueryTimeout is the timeout for account-related queries.
const AccountQueryTimeout = 30 * time.Second

// AsyncEnhancementTimeout is the timeout for asynchronous enhancement operations.
const AsyncEnhancementTimeout = 10 * time.Second

// WithDBTimeout creates a new context with the default database query timeout.
// This is a convenience wrapper around context.WithTimeout that uses the standard
// DBQueryTimeout duration.
//
// The caller must call the returned cancel function to release resources.
//
// Example usage:
//
//	ctx, cancel := utils.WithDBTimeout()
//	defer cancel()
//	result := db.WithContext(ctx).Model(&Model{}).Find(&results)
//
// Returns:
//   - context.Context: A context with timeout
//   - context.CancelFunc: The cancel function to release resources
func WithDBTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DBQueryTimeout)
}

// WithDBTimeoutContext creates a new context with timeout based on a parent context.
// This is useful when you need to derive a timeout context from an existing context
// (e.g., a request context) rather than from context.Background().
//
// The caller must call the returned cancel function to release resources.
//
// Example usage:
//
//	ctx, cancel := utils.WithDBTimeoutContext(reqCtx)
//	defer cancel()
//	result := db.WithContext(ctx).Model(&Model{}).Find(&results)
//
// Parameters:
//   - parent: The parent context to derive from
//
// Returns:
//   - context.Context: A context with timeout
//   - context.CancelFunc: The cancel function to release resources
func WithDBTimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, DBQueryTimeout)
}

// WithCustomTimeout creates a new context with a custom timeout duration.
// This allows using different timeouts for specific operations while still
// providing a convenient wrapper.
//
// The caller must call the returned cancel function to release resources.
//
// Example usage:
//
//	ctx, cancel := utils.WithCustomTimeout(10 * time.Second)
//	defer cancel()
//	result := db.WithContext(ctx).Model(&Model{}).Find(&results)
//
// Parameters:
//   - timeout: The timeout duration
//
// Returns:
//   - context.Context: A context with timeout
//   - context.CancelFunc: The cancel function to release resources
func WithCustomTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// WithCustomTimeoutContext creates a new context with a custom timeout based on a parent context.
//
// The caller must call the returned cancel function to release resources.
//
// Example usage:
//
//	ctx, cancel := utils.WithCustomTimeoutContext(reqCtx, 10 * time.Second)
//	defer cancel()
//	result := db.WithContext(ctx).Model(&Model{}).Find(&results)
//
// Parameters:
//   - parent: The parent context to derive from
//   - timeout: The timeout duration
//
// Returns:
//   - context.Context: A context with timeout
//   - context.CancelFunc: The cancel function to release resources
func WithCustomTimeoutContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
