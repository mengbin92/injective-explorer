package chain

import (
	"fmt"

	"injective-explorer/internal/global"

	"github.com/ethereum/go-ethereum/common"
)

// RecoveryContext provides context for recovery operations, enabling detailed logging.
type RecoveryContext struct {
	// Operation describes what operation was being performed (e.g., "accessing block.BaseFee()")
	Operation string
	// Identifier provides a unique identifier for the affected entity (e.g., transaction hash)
	Identifier string
	// FallbackBehavior describes what happens when recovery occurs (e.g., "using zero value", "using tx.GasPrice()")
	FallbackBehavior string
}

// SafeExecute executes a function with panic recovery and consistent logging.
//
// This helper function centralizes panic recovery logic to avoid code duplication (DRY principle)
// and ensures consistent error handling across the codebase.
//
// IMPORTANT: This function recovers from panics and continues execution with fallback values.
// This is acceptable for:
//   - Handling corrupted blockchain data from external sources (RPC nodes)
//   - Defensive programming against nil pointer access in optional fields
//   - Graceful degradation when non-critical data is unavailable
//
// This is NOT appropriate for:
//   - Expected errors that should be returned to the caller
//   - Critical failures where continuing would produce incorrect data
//   - Panics that indicate bugs in our own code
//
// Parameters:
//   - ctx: Recovery context for logging (operation, identifier, fallback behavior)
//   - fn: Function to execute safely
//
// Returns:
//   - bool: true if a panic was recovered, false otherwise
//
// Usage Example:
//
//	baseFee := safeExecute(&RecoveryContext{
//	    Operation:        "accessing block.BaseFee()",
//	    Identifier:       txHash.Hex(),
//	    FallbackBehavior: "using tx.GasPrice()",
//	}, func() {
//	    if block != nil && block.BaseFee() != nil {
//	        baseFee = block.BaseFee()
//	        gasPrice = new(big.Int).Add(baseFee, maxPriorityFee)
//	    }
//	})
func SafeExecute(ctx *RecoveryContext, fn func()) (recovered bool) {
	if ctx == nil {
		ctx = &RecoveryContext{
			Operation:        "unknown operation",
			Identifier:       "unknown",
			FallbackBehavior: "continuing with default/fallback value",
		}
	}

	defer func() {
		if r := recover(); r != nil {
			recovered = true
			global.Logger.Warnf(
				"Recovered from panic during %s for %s: %v. Fallback: %s",
				ctx.Operation,
				ctx.Identifier,
				r,
				ctx.FallbackBehavior,
			)
		}
	}()

	fn()
	return false
}

// SafeExecuteWithTxHash is a convenience wrapper for SafeExecute with transaction hash context.
//
// This is the most common use case for SafeExecute in transaction processing.
//
// Parameters:
//   - txHash: Transaction hash for logging
//   - operation: Description of the operation being performed
//   - fallback: Description of fallback behavior
//   - fn: Function to execute safely
//
// Returns:
//   - bool: true if a panic was recovered, false otherwise
func SafeExecuteWithTxHash(txHash common.Hash, operation, fallback string, fn func()) bool {
	return SafeExecute(&RecoveryContext{
		Operation:        operation,
		Identifier:       txHash.Hex(),
		FallbackBehavior: fallback,
	}, fn)
}

// SafeExecuteWithIdentifier is a convenience wrapper for SafeExecute with a custom identifier.
//
// Parameters:
//   - identifier: Unique identifier for the affected entity (e.g., address, block number)
//   - operation: Description of the operation being performed
//   - fallback: Description of fallback behavior
//   - fn: Function to execute safely
//
// Returns:
//   - bool: true if a panic was recovered, false otherwise
func SafeExecuteWithIdentifier(identifier, operation, fallback string, fn func()) bool {
	return SafeExecute(&RecoveryContext{
		Operation:        operation,
		Identifier:       identifier,
		FallbackBehavior: fallback,
	}, fn)
}

// MustRecover is a stricter variant that logs errors instead of warnings.
//
// Use this when a panic indicates a data corruption issue that should be investigated.
//
// Parameters:
//   - ctx: Recovery context for logging
//   - fn: Function to execute safely
//
// Returns:
//   - bool: true if a panic was recovered, false otherwise
func MustRecover(ctx *RecoveryContext, fn func()) (recovered bool) {
	if ctx == nil {
		ctx = &RecoveryContext{
			Operation:        "unknown operation",
			Identifier:       "unknown",
			FallbackBehavior: "continuing with default/fallback value",
		}
	}

	defer func() {
		if r := recover(); r != nil {
			recovered = true
			global.Logger.Errorf(
				"Data corruption detected: panic during %s for %s: %v. Fallback: %s. "+
					"This should be investigated - corrupted blockchain data may indicate upstream issues.",
				ctx.Operation,
				ctx.Identifier,
				r,
				ctx.FallbackBehavior,
			)
		}
	}()

	fn()
	return false
}

// LogPanic logs a panic without recovery (for use in existing defer functions).
//
// This is useful for database transaction rollback scenarios where you need
// to perform cleanup before re-panicking.
//
// Parameters:
//   - operation: Description of the operation that panicked
//   - identifier: Unique identifier for context
//   - r: The recovered panic value
func LogPanic(operation, identifier string, r interface{}) {
	global.Logger.Errorf(
		"Panic during %s for %s: %v. Performing cleanup and re-panicking.",
		operation,
		identifier,
		r,
	)
}

// CreateRecoveryContext creates a RecoveryContext with sensible defaults.
//
// Parameters:
//   - operation: Description of the operation being performed
//   - identifier: Unique identifier for the affected entity
//   - fallback: Description of fallback behavior
//
// Returns:
//   - *RecoveryContext: Configured recovery context
func CreateRecoveryContext(operation, identifier, fallback string) *RecoveryContext {
	return &RecoveryContext{
		Operation:        operation,
		Identifier:       identifier,
		FallbackBehavior: fallback,
	}
}

// FormatRecoveryMessage formats a recovery message for custom logging scenarios.
//
// This can be used when you need to include recovery information in
// application-level logs or metrics.
func FormatRecoveryMessage(ctx *RecoveryContext, r interface{}) string {
	return fmt.Sprintf(
		"Recovered from panic during %s for %s: %v. Fallback: %s",
		ctx.Operation,
		ctx.Identifier,
		r,
		ctx.FallbackBehavior,
	)
}
