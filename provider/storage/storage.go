// Package storage provides object storage abstraction for various providers.
package storage

import (
	"context"
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
