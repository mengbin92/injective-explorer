// Package utils provides validation utilities used across the application.
package utils

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

// ValidationError represents a validation error with HTTP status code.
type ValidationError struct {
	Code    int
	Message string
	Err     error
}

// Error implements the error interface.
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

// NewValidationError creates a new validation error.
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
// Usage: ValidateRequiredFields("field1", req.Field1, "field2", req.Field2)
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

// ValidateRange validates that a value is within a range.
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

// ValidatePositive validates that a number is positive.
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
type ValidationResult struct {
	Errors []*ValidationError
}

// HasErrors returns true if there are validation errors.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// FirstError returns the first validation error or nil.
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

// ValidateAll runs multiple validations and collects all errors.
func ValidateAll(validations ...*ValidationError) *ValidationResult {
	result := &ValidationResult{}
	for _, v := range validations {
		result.Add(v)
	}
	return result
}
