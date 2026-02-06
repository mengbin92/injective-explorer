// Package utils provides shared utility functions used across the application.
// It includes helpers for string manipulation, validation, pagination, caching,
// database operations, and context management.
package utils

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// DBExecutor provides a unified interface for database operations with timeout.
type DBExecutor struct {
	db *gorm.DB
}

// NewDBExecutor creates a new DBExecutor instance.
func NewDBExecutor(db *gorm.DB) *DBExecutor {
	return &DBExecutor{db: db}
}

// WithTimeout executes a database operation with a specified timeout context.
// This eliminates the repetitive pattern of creating context and deferring cancel.
//
// Type Parameters:
//   - T: The return type of the operation
//
// Parameters:
//   - db: The GORM database connection
//   - timeout: The timeout duration for the operation
//   - operation: The database operation to execute
//
// Returns:
//   - T: The result of the operation
//   - error: Any error that occurred during the operation
//
// Example:
//
//	result, err := utils.WithTimeout(db, 5*time.Second, func(ctx context.Context, db *gorm.DB) (interface{}, error) {
//	    var model MyModel
//	    err := db.WithContext(ctx).First(&model, id).Error
//	    return &model, err
//	})
func WithTimeout[T any](db *gorm.DB, timeout time.Duration, operation func(ctx context.Context, db *gorm.DB) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return operation(ctx, db)
}

// WithDefaultTimeout executes a database operation with the default DB timeout.
func WithDefaultTimeout[T any](db *gorm.DB, operation func(ctx context.Context, db *gorm.DB) (T, error)) (T, error) {
	return WithTimeout(db, DBQueryTimeout, operation)
}

// QueryOptions provides common query options.
type QueryOptions struct {
	Timeout   time.Duration
	OrderBy   string
	Limit     int
	Offset    int
	UseCursor bool
	Cursor    uint64
}

// DefaultQueryOptions returns default query options.
func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		Timeout: DBQueryTimeout,
		Limit:   DefaultPageSize,
		Offset:  0,
	}
}

// ExecuteQuery executes a database query with standardized error handling.
func ExecuteQuery[T any](
	db *gorm.DB,
	opts QueryOptions,
	buildQuery func(db *gorm.DB) *gorm.DB,
) (T, error) {
	var result T

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	query := db.WithContext(ctx)
	query = buildQuery(query)

	if opts.OrderBy != "" {
		query = query.Order(opts.OrderBy)
	}

	if opts.UseCursor {
		if opts.Cursor > 0 {
			query = query.Where("id < ?", opts.Cursor)
		}
		query = query.Order("id DESC").Limit(opts.Limit)
	} else {
		if opts.Offset > 0 {
			query = query.Offset(opts.Offset)
		}
		if opts.Limit > 0 {
			query = query.Limit(opts.Limit)
		}
	}

	err := query.Find(&result).Error
	return result, err
}

// ExecuteCount executes a count query with timeout.
func ExecuteCount(db *gorm.DB, timeout time.Duration, buildQuery func(db *gorm.DB) *gorm.DB) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var count int64
	query := db.WithContext(ctx)
	query = buildQuery(query)
	err := query.Count(&count).Error
	return count, err
}

// ExecuteFirst executes a First query with timeout and standardized not-found handling.
func ExecuteFirst[T any](
	db *gorm.DB,
	timeout time.Duration,
	buildQuery func(db *gorm.DB) *gorm.DB,
) (T, error) {
	var result T

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	query := db.WithContext(ctx)
	query = buildQuery(query)
	err := query.First(&result).Error

	return result, err
}

// Transaction executes a function within a database transaction.
// Automatically handles transaction commit and rollback based on the function's return error.
//
// Type Parameters:
//   - T: The return type of the function
//
// Parameters:
//   - db: The GORM database connection
//   - fn: The function to execute within the transaction
//
// Returns:
//   - T: The result of the function
//   - error: Any error that occurred during the transaction
//
// Example:
//
//	result, err := utils.Transaction(db, func(tx *gorm.DB) (*MyModel, error) {
//	    model := &MyModel{Name: "test"}
//	    if err := tx.Create(model).Error; err != nil {
//	        return nil, err
//	    }
//	    return model, nil
//	})
func Transaction[T any](db *gorm.DB, fn func(tx *gorm.DB) (T, error)) (T, error) {
	var result T
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = fn(tx)
		return err
	})
	return result, err
}
