package models

import (
	"injective-explorer/provider/db"
	"context"
	"time"
)

// CreateOptimizedIndexes creates performance optimization indexes
// This function should be called after AutoMigrate to add composite indexes
func CreateOptimizedIndexes() error {
	database := db.Get()
	if database == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create composite index for enhanceBlockDTOWithTransactions optimization
	// This index covers all fields used in the query: SELECT hash, signer, to, value WHERE block_number = ?
	err := database.WithContext(ctx).Exec(`
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_block_enhance
		ON transactions(block_number, hash, signer, "to", value)
	`).Error

	if err != nil {
		// Check if the index already exists
		var count int64
		database.Raw(`
			SELECT COUNT(*)
			FROM pg_indexes
			WHERE indexname = 'idx_block_enhance'
		`).Scan(&count)

		if count == 0 {
			return err // Only return error if index creation failed and index doesn't exist
		}
	}

	// Additional indexes for block queries
	database.WithContext(ctx).Exec(`
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_height_desc
		ON blocks(height DESC)
	`)

	database.WithContext(ctx).Exec(`
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_timestamp_desc
		ON blocks(block_unix_timestamp DESC)
	`)

	return nil
}

// DropOptimizedIndexes drops the optimization indexes (for rollback)
func DropOptimizedIndexes() error {
	database := db.Get()
	if database == nil {
		return nil
	}

	// Drop indexes if they exist
	database.Exec(`DROP INDEX IF EXISTS idx_block_enhance`)
	database.Exec(`DROP INDEX IF EXISTS idx_height_desc`)
	database.Exec(`DROP INDEX IF EXISTS idx_timestamp_desc`)

	return nil
}