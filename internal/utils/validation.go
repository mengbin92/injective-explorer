// Package utils provides shared utility functions used across the application.
// It includes helpers for string manipulation, validation, pagination, caching,
// database operations, and context management.
package utils

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

// ValidationError represents a validation error with HTTP status code.
// It implements the error interface and supports error unwrapping.
type ValidationError struct {
	Code    int
	Message string
	Err     error
}

// Error implements the error interface.
// Returns the formatted error message including the wrapped error if present.
func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the wrapped error.
func (e *ValidationError) Unwrap() error {
	return e.Err
}

// NewValidationError creates a new validation error with the specified code, message, and wrapped error.
//
// Parameters:
//   - code: HTTP status code (e.g., http.StatusBadRequest)
//   - message: Human-readable error message
//   - err: The underlying error to wrap (can be nil)
//
// Returns:
//   - *ValidationError: A new validation error instance
func NewValidationError(code int, message string, err error) *ValidationError {
	return &ValidationError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Validator is a generic validator interface.
type Validator[T any] interface {
	Validate(req T) *ValidationError
}

// ValidateNil checks if a request is nil.
// Returns a ValidationError with http.StatusBadRequest if req is nil.
//
// Parameters:
//   - req: The request object to check
//
// Returns:
//   - *ValidationError: Validation error if req is nil, nil otherwise
func ValidateNil(req interface{}) *ValidationError {
	if req == nil {
		return NewValidationError(
			http.StatusBadRequest,
			"request cannot be nil",
			errors.New("nil request"),
		)
	}
	return nil
}

// ValidateRequiredString checks if a string field is not empty.
// Returns a ValidationError with http.StatusBadRequest if value is empty.
//
// Parameters:
//   - value: The string value to check
//   - fieldName: The name of the field for error messages
//
// Returns:
//   - *ValidationError: Validation error if value is empty, nil otherwise
func ValidateRequiredString(value, fieldName string) *ValidationError {
	if value == "" {
		return NewValidationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s is required", fieldName),
			fmt.Errorf("%s is required", fieldName),
		)
	}
	return nil
}

// ValidateRequiredFields validates multiple required string fields.
// Accepts pairs of field names and values (fieldName, value, fieldName2, value2, ...).
// Returns the first validation error encountered, or nil if all fields are valid.
//
// Parameters:
//   - fields: Alternating field names (string) and values (string) to validate
//
// Returns:
//   - *ValidationError: First validation error found, or nil if all fields are valid
//
// Example:
//
//	if err := ValidateRequiredFields(
//	    "address", req.GetAddress(),
//	    "chain_id", req.GetChainId(),
//	); err != nil {
//	    return err
//	}
func ValidateRequiredFields(fields ...interface{}) *ValidationError {
	if len(fields)%2 != 0 {
		return NewValidationError(
			http.StatusInternalServerError,
			"invalid ValidateRequiredFields usage: must provide pairs of field names and values",
			errors.New("invalid validation usage"),
		)
	}

	for i := 0; i < len(fields); i += 2 {
		fieldName, ok := fields[i].(string)
		if !ok {
			continue
		}
		value, ok := fields[i+1].(string)
		if !ok {
			continue
		}
		if err := ValidateRequiredString(value, fieldName); err != nil {
			return err
		}
	}
	return nil
}

// ValidateMinLength checks if a string has minimum length.
// Returns a ValidationError if the string length is less than minLength.
//
// Parameters:
//   - value: The string to check
//   - fieldName: The name of the field for error messages
//   - minLength: The minimum required length
//
// Returns:
//   - *ValidationError: Validation error if string is too short, nil otherwise
func ValidateMinLength(value, fieldName string, minLength int) *ValidationError {
	if len(value) < minLength {
		return NewValidationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s must be at least %d characters", fieldName, minLength),
			fmt.Errorf("%s too short", fieldName),
		)
	}
	return nil
}

// ValidateMaxLength checks if a string doesn't exceed maximum length.
// Returns a ValidationError if the string length exceeds maxLength.
//
// Parameters:
//   - value: The string to check
//   - fieldName: The name of the field for error messages
//   - maxLength: The maximum allowed length
//
// Returns:
//   - *ValidationError: Validation error if string is too long, nil otherwise
func ValidateMaxLength(value, fieldName string, maxLength int) *ValidationError {
	if len(value) > maxLength {
		return NewValidationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s must not exceed %d characters", fieldName, maxLength),
			fmt.Errorf("%s too long", fieldName),
		)
	}
	return nil
}

// ValidateRange validates that a value is within a range [min, max] (inclusive).
// Uses the provided comparison function for ordering.
//
// Type Parameters:
//   - T: The comparable type of the values
//
// Parameters:
//   - value: The value to validate
//   - min: The minimum allowed value
//   - max: The maximum allowed value
//   - fieldName: The name of the field for error messages
//   - compare: Comparison function that returns negative if a < b, zero if a == b, positive if a > b
//
// Returns:
//   - *ValidationError: Validation error if value is out of range, nil otherwise
func ValidateRange[T comparable](value, min, max T, fieldName string, compare func(a, b T) int) *ValidationError {
	if compare(value, min) < 0 {
		return NewValidationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s must be at least %v", fieldName, min),
			fmt.Errorf("%s below minimum", fieldName),
		)
	}
	if compare(value, max) > 0 {
		return NewValidationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s must not exceed %v", fieldName, max),
			fmt.Errorf("%s above maximum", fieldName),
		)
	}
	return nil
}

// ValidatePositive validates that a number is positive (greater than zero).
// Supports all numeric types (signed and unsigned integers, floats).
//
// Type Parameters:
//   - T: A numeric type constraint
//
// Parameters:
//   - value: The numeric value to validate
//   - fieldName: The name of the field for error messages
//
// Returns:
//   - *ValidationError: Validation error if value is not positive, nil otherwise
func ValidatePositive[T ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64](value T, fieldName string) *ValidationError {
	var zero T
	if value <= zero {
		return NewValidationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s must be positive", fieldName),
			fmt.Errorf("%s is not positive", fieldName),
		)
	}
	return nil
}

// ValidationResult holds the result of multiple validations.
// Use this to collect all validation errors before returning.
type ValidationResult struct {
	// Errors is the list of validation errors collected
	Errors []*ValidationError
}

// HasErrors returns true if there are any validation errors.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// FirstError returns the first validation error or nil if there are no errors.
func (r *ValidationResult) FirstError() *ValidationError {
	if len(r.Errors) > 0 {
		return r.Errors[0]
	}
	return nil
}

// Add adds a validation error if err is not nil.
func (r *ValidationResult) Add(err *ValidationError) *ValidationResult {
	if err != nil {
		r.Errors = append(r.Errors, err)
	}
	return r
}

// ValidateAll runs multiple validations and collects all errors into a single result.
// This allows validating all fields at once and returning all errors together.
//
// Parameters:
//   - validations: Variable number of validation errors to collect
//
// Returns:
//   - *ValidationResult: A result containing all validation errors
//
// Example:
//
//	result := ValidateAll(
//	    ValidateRequiredString(req.GetName(), "name"),
//	    ValidateRequiredString(req.GetEmail(), "email"),
//	    ValidateMinLength(req.GetPassword(), "password", 8),
//	)
//	if result.HasErrors() {
//	    return result.FirstError()
//	}
func ValidateAll(validations ...*ValidationError) *ValidationResult {
	result := &ValidationResult{}
	for _, v := range validations {
		result.Add(v)
	}
	return result
}
