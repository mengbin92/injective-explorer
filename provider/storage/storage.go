// Package storage provides object storage abstraction for various providers.
package storage

import (
	"context"
	"fmt"
	"io"
)

// Storage defines the interface for object storage operations.
// It abstracts different storage providers (S3, OSS, COS, MinIO) behind a common interface.
type Storage interface {
	// PutObject uploads an object to storage.
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: Object key (path) in storage
	//   - data: Object data to upload
	// Returns:
	//   - error: Error if upload fails
	PutObject(ctx context.Context, key string, data []byte) error

	// PutObjectFromReader uploads an object from a reader.
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: Object key (path) in storage
	//   - reader: Reader to read data from
	//   - size: Size of the data (use -1 if unknown)
	// Returns:
	//   - error: Error if upload fails
	PutObjectFromReader(ctx context.Context, key string, reader io.Reader, size int64) error

	// GetObject retrieves an object from storage.
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: Object key (path) in storage
	// Returns:
	//   - []byte: Object data
	//   - error: Error if retrieval fails
	GetObject(ctx context.Context, key string) ([]byte, error)

	// GetObjectReader returns a reader for the object.
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: Object key (path) in storage
	// Returns:
	//   - io.ReadCloser: Reader for the object data
	//   - error: Error if retrieval fails
	GetObjectReader(ctx context.Context, key string) (io.ReadCloser, error)

	// DeleteObject deletes an object from storage.
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: Object key (path) in storage
	// Returns:
	//   - error: Error if deletion fails
	DeleteObject(ctx context.Context, key string) error

	// Exists checks if an object exists in storage.
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: Object key (path) in storage
	// Returns:
	//   - bool: True if object exists
	//   - error: Error if check fails
	Exists(ctx context.Context, key string) (bool, error)

	// GetObjectURL returns a URL for accessing the object (optional, may return empty string).
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: Object key (path) in storage
	//   - expiresIn: URL expiration time in seconds (0 for no expiration)
	// Returns:
	//   - string: URL for accessing the object
	//   - error: Error if URL generation fails
	GetObjectURL(ctx context.Context, key string, expiresIn int64) (string, error)
}

// NoOpStorage is a no-op implementation of Storage interface.
// It can be used when object storage is disabled.
// All operations return success or ErrObjectNotFound for read operations.
type NoOpStorage struct{}

// PutObject is a no-op that always succeeds.
func (n *NoOpStorage) PutObject(ctx context.Context, key string, data []byte) error {
	return nil
}

// PutObjectFromReader is a no-op that always succeeds.
func (n *NoOpStorage) PutObjectFromReader(ctx context.Context, key string, reader io.Reader, size int64) error {
	return nil
}

// GetObject always returns ErrObjectNotFound.
func (n *NoOpStorage) GetObject(ctx context.Context, key string) ([]byte, error) {
	return nil, ErrObjectNotFound
}

// GetObjectReader always returns ErrObjectNotFound.
func (n *NoOpStorage) GetObjectReader(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, ErrObjectNotFound
}

// DeleteObject is a no-op that always succeeds.
func (n *NoOpStorage) DeleteObject(ctx context.Context, key string) error {
	return nil
}

// Exists always returns false.
func (n *NoOpStorage) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}

// GetObjectURL always returns an empty string.
func (n *NoOpStorage) GetObjectURL(ctx context.Context, key string, expiresIn int64) (string, error) {
	return "", nil
}

// Storage operation errors.
var (
	// ErrObjectNotFound is returned when an object does not exist in storage.
	ErrObjectNotFound = &StorageError{Message: "object not found"}
	// ErrInvalidConfig is returned when storage configuration is invalid.
	ErrInvalidConfig = &StorageError{Message: "invalid storage configuration"}
)

// StorageError represents a storage operation error.
type StorageError struct {
	Message string
	Err     error
}

// Error returns the error message, including wrapped error if present.
func (e *StorageError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap returns the wrapped error, if any.
func (e *StorageError) Unwrap() error {
	return e.Err
}

// BuildStorageKey generates a storage key for object storage.
// Format: {type}/{hash[:8]}/{hash} or {type}/{hash} if hash is too short.
// Removes "0x" prefix from hash if present.
//
// This function is used by sync, archive, and migration services to ensure consistent storage paths
// and avoid data duplication.
//
// Parameters:
//   - dataType: Type of data (e.g., "data", "messages", "logs")
//   - hash: Transaction hash (with or without "0x" prefix)
//
// Returns:
//   - string: Storage key for the object
//
// Example:
//   - BuildStorageKey("data", "0xabcdef1234567890") -> "data/abcdef12/abcdef1234567890"
//   - BuildStorageKey("messages", "abc123") -> "messages/abc123"
func BuildStorageKey(dataType, hash string) string {
	// Remove "0x" prefix if present
	if len(hash) > 2 && hash[:2] == "0x" {
		hash = hash[2:]
	}
	// Use first 8 characters for directory structure
	if len(hash) > 8 {
		return fmt.Sprintf("%s/%s/%s", dataType, hash[:8], hash)
	}
	return fmt.Sprintf("%s/%s", dataType, hash)
}
