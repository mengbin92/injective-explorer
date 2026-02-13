// Package utils provides shared utility functions used across the application.
// It includes helpers for string manipulation, validation, pagination, caching,
// database operations, and context management.
package utils

import (
	"errors"
	"strings"
)

// Common error types for classification
var (
	ErrDuplicateContract     = errors.New("duplicate contract")
	ErrInvalidAddress       = errors.New("invalid address")
	ErrMissingRequiredField = errors.New("missing required field")
)

// IsRetryableError checks if an error is retryable.
// Retryable errors include:
// - 5xx HTTP status codes (500, 502, 503, 504)
// - gRPC Unavailable errors
// - Network timeout errors (but not context cancellation)
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for gRPC status codes
	if strings.Contains(errStr, "code = Unavailable") {
		return true
	}

	// Check for HTTP 5xx status codes
	if strings.Contains(errStr, "503") || strings.Contains(errStr, "Service Unavailable") {
		return true
	}
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "Internal Server Error") {
		return true
	}
	if strings.Contains(errStr, "502") || strings.Contains(errStr, "Bad Gateway") {
		return true
	}
	if strings.Contains(errStr, "504") || strings.Contains(errStr, "Gateway Timeout") {
		return true
	}

	// Check for network-related errors (but not context cancellation)
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "no such host") {
		return true
	}

	return false
}

// SanitizeError removes sensitive information from error messages.
// This prevents leaking internal details like file paths, connection strings, etc.
//
// Parameters:
//   - err: The original error that may contain sensitive information
//   - maxLength: Maximum length for the error message (0 means no limit)
//
// Returns:
//   - string: A sanitized error message safe to return to clients
func SanitizeError(err error, maxLength int) string {
	if err == nil {
		return "unknown error"
	}

	errMsg := err.Error()

	// Remove common sensitive patterns
	sensitivePatterns := []string{
		// File paths
		"/Users/",
		"/home/",
		"C:\\",
		// Connection strings
		"@tcp(",
		"postgres://",
		"redis://",
		// API keys and tokens
		"Bearer ",
		"Authorization:",
		// Stack traces (keep only first line)
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(errMsg, pattern) {
			// Return a generic error message if sensitive patterns are detected
			return "internal server error"
		}
	}

	// If error contains stack trace, only return the first line
	if idx := strings.Index(errMsg, "\n"); idx > 0 {
		errMsg = errMsg[:idx]
	}

	// Limit error message length to prevent information leakage
	if maxLength > 0 && len(errMsg) > maxLength {
		errMsg = errMsg[:maxLength] + "..."
	}

	return errMsg
}

// IsDuplicateKeyError checks if an error is a duplicate key/constraint violation.
// This is useful for distinguishing between errors that can be retried vs. those that should fail fast.
func IsDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	patterns := []string{"duplicate entry", "unique constraint", "primary key", "conflict"}
	for _, pattern := range patterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}
