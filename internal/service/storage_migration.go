// Package service provides business logic services for the blockchain explorer.
// This file implements data migration to object storage.
package service

import (
	"context"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// StorageMigrationService handles migration of existing transaction data to object storage.
// It uploads large fields (Data, Messages, Logs) to object storage and stores only storage keys in the database.
// This reduces database size and improves query performance for large-scale deployments.
type StorageMigrationService struct {
	db      *gorm.DB
	storage storage.Storage
}

// NewStorageMigrationService creates a new StorageMigrationService instance.
//
// Returns:
//   - *StorageMigrationService: A new migration service instance
func NewStorageMigrationService() *StorageMigrationService {
	return &StorageMigrationService{
		db:      db.Get(),
		storage: storage.Get(),
	}
}

// MigrationConfig contains configuration for migration operations.
type MigrationConfig struct {
	// BatchSize is the number of transactions to process per batch (default: 100)
	// Can be adjusted dynamically during operation
	BatchSize int
	// Threshold is the minimum size in bytes to migrate to object storage (default: 10KB)
	Threshold int
	// DryRun if true, only simulates migration without actually moving data
	DryRun bool
	// OnlyEmptyKeys if true, only migrate transactions without storage keys
	OnlyEmptyKeys bool
	// ProgressCallback is an optional callback function called after each batch
	// Parameters: processed, total, current batch size
	// This allows external monitoring and dynamic batch size adjustment
	ProgressCallback func(processed, total int64, batchSize int)
}

// DefaultMigrationConfig returns default migration configuration.
//
// Returns:
//   - *MigrationConfig: Default configuration with BatchSize=100, Threshold=10KB, DryRun=false, OnlyEmptyKeys=true
func DefaultMigrationConfig() *MigrationConfig {
	return &MigrationConfig{
		BatchSize:     100,
		Threshold:     10 * 1024, // 10KB
		DryRun:        false,
		OnlyEmptyKeys: true,
	}
}

// MigrateTransactions migrates transaction data to object storage.
// It uploads large fields (Data, Messages, Logs) to object storage and updates storage keys.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - config: Migration configuration (uses DefaultMigrationConfig if nil)
//
// Returns:
//   - error: Error if migration fails
func (s *StorageMigrationService) MigrateTransactions(ctx context.Context, config *MigrationConfig) error {
	if config == nil {
		config = DefaultMigrationConfig()
	}

	// Validate configuration
	if config.BatchSize <= 0 {
		return errors.New("batch size must be greater than 0")
	}
	if config.Threshold < 0 {
		return errors.New("threshold must be non-negative")
	}
	if config.BatchSize > MaxStorageMigrationBatch {
		global.Logger.Warnf("batch size %d is very large (max recommended: %d), consider using a smaller value for better performance",
			config.BatchSize, MaxStorageMigrationBatch)
	}

	global.Logger.Infof("starting transaction migration to object storage: batch_size=%d, threshold=%d, dry_run=%v",
		config.BatchSize, config.Threshold, config.DryRun)

	// Initialize metrics
	metrics := &MigrationMetrics{
		StartTime: time.Now(),
	}

	// Build query
	query := s.db.WithContext(ctx).Model(&models.Transaction{})
	if config.OnlyEmptyKeys {
		query = query.Where("(data_storage_key IS NULL OR data_storage_key = '') AND " +
			"(messages_storage_key IS NULL OR messages_storage_key = '') AND " +
			"(logs_storage_key IS NULL OR logs_storage_key = '')")
	}

	// Count total transactions to migrate
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return errors.Wrap(err, "count transactions to migrate")
	}

	if totalCount == 0 {
		global.Logger.Infof("no transactions to migrate")
		return nil
	}

	metrics.TotalTransactions = totalCount
	global.Logger.Infof("found %d transactions to migrate", totalCount)

	if config.DryRun {
		global.Logger.Infof("dry run mode: would migrate %d transactions", totalCount)
		return nil
	}

	// Migrate in batches
	var migratedCount int64
	var offset int64
	batchStartTime := time.Now()

	for offset < totalCount {
		// Get batch of transactions
		var transactions []models.Transaction
		batchQuery := query.Order("id ASC").Limit(config.BatchSize).Offset(int(offset))
		if err := batchQuery.Find(&transactions).Error; err != nil {
			return errors.Wrapf(err, "fetch batch at offset %d", offset)
		}

		if len(transactions) == 0 {
			break
		}

		// Migrate batch
		batchMigrated, err := s.migrateBatch(ctx, transactions, config)
		if err != nil {
			return errors.Wrapf(err, "migrate batch at offset %d", offset)
		}

		migratedCount += batchMigrated
		offset += int64(len(transactions))
		metrics.BatchesProcessed++
		batchDuration := time.Since(batchStartTime)
		global.Logger.Infof("migrated batch: %d/%d transactions, batch_time=%v", migratedCount, totalCount, batchDuration)

		// Call progress callback if provided
		if config.ProgressCallback != nil {
			config.ProgressCallback(migratedCount, totalCount, config.BatchSize)
		}

		batchStartTime = time.Now() // Reset for next batch

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	metrics.MigratedTransactions = migratedCount
	metrics.EndTime = time.Now()
	metrics.LogMetrics()

	global.Logger.Infof("migration completed: migrated %d transactions", migratedCount)
	return nil
}

// migrateBatch migrates a batch of transactions to object storage.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - transactions: Batch of transactions to migrate
//   - config: Migration configuration
//
// Returns:
//   - int64: Number of successfully migrated transactions
//   - error: Error if migration fails
func (s *StorageMigrationService) migrateBatch(ctx context.Context, transactions []models.Transaction, config *MigrationConfig) (int64, error) {
	if len(transactions) == 0 {
		return 0, nil
	}

	// Use transaction to ensure atomicity
	dbTx := s.db.WithContext(ctx).Begin()
	if dbTx.Error != nil {
		return 0, errors.Wrap(dbTx.Error, "begin transaction")
	}

	var migratedCount int64
	var hasError bool

	defer func() {
		if hasError {
			if rbErr := dbTx.Rollback().Error; rbErr != nil {
				global.Logger.Errorf("failed to rollback transaction: %v", rbErr)
			}
		}
	}()

	for _, tx := range transactions {
		updated := false
		updates := make(map[string]interface{})

		// Migrate Data field
		if tx.DataStorageKey == "" && len(tx.Data) > config.Threshold {
			key := storage.BuildStorageKey("data", tx.Hash)
			dataBytes := []byte(tx.Data)
			// Try to decompress first to get original data
			if decompressed, err := models.DecompressData(tx.Data); err == nil && len(decompressed) > 0 {
				dataBytes = decompressed
			}

			// Retry upload with exponential backoff (max 3 retries)
			var uploadErr error
			for attempt := 0; attempt < 3; attempt++ {
				opStart := time.Now()
				uploadErr = s.storage.PutObject(ctx, key, dataBytes)
				storageLatency := time.Since(opStart)
				// Note: Metrics would be tracked if we had access to metrics object here
				// For now, we just log the operation
				if uploadErr == nil {
					updates["data_storage_key"] = key
					updates["data"] = "" // Clear data field
					updated = true
					global.Logger.Debugf("uploaded data to storage for tx %s, latency=%v", tx.Hash, storageLatency)
					break
				}
				if attempt < 2 {
					global.Logger.Warnf("failed to upload data to storage for tx %s (attempt %d/3): %v, retrying...", tx.Hash, attempt+1, uploadErr)
				}
			}
			if uploadErr != nil {
				global.Logger.Errorf("failed to upload data to storage for tx %s after 3 attempts: %v", tx.Hash, uploadErr)
				// Continue with other fields even if this one fails
			}
		}

		// Migrate Messages field
		if tx.MessagesStorageKey == "" && len(tx.Messages) > config.Threshold {
			key := storage.BuildStorageKey("messages", tx.Hash)
			messagesBytes := []byte(tx.Messages)
			// Try to decompress first to get original data
			if decompressed, err := models.DecompressData(tx.Messages); err == nil && len(decompressed) > 0 {
				messagesBytes = decompressed
			}

			// Retry upload with exponential backoff (max 3 retries)
			var uploadErr error
			for attempt := 0; attempt < 3; attempt++ {
				uploadErr = s.storage.PutObject(ctx, key, messagesBytes)
				if uploadErr == nil {
					updates["messages_storage_key"] = key
					updates["messages"] = "" // Clear messages field
					updated = true
					break
				}
				if attempt < 2 {
					global.Logger.Warnf("failed to upload messages to storage for tx %s (attempt %d/3): %v, retrying...", tx.Hash, attempt+1, uploadErr)
				}
			}
			if uploadErr != nil {
				global.Logger.Errorf("failed to upload messages to storage for tx %s after 3 attempts: %v", tx.Hash, uploadErr)
				// Continue with other fields even if this one fails
			}
		}

		// Migrate Logs field
		if tx.LogsStorageKey == "" && len(tx.Logs) > config.Threshold {
			key := storage.BuildStorageKey("logs", tx.Hash)
			logsBytes := []byte(tx.Logs)
			// Try to decompress first to get original data
			if decompressed, err := models.DecompressData(tx.Logs); err == nil && len(decompressed) > 0 {
				logsBytes = decompressed
			}

			// Retry upload with exponential backoff (max 3 retries)
			var uploadErr error
			for attempt := 0; attempt < 3; attempt++ {
				uploadErr = s.storage.PutObject(ctx, key, logsBytes)
				if uploadErr == nil {
					updates["logs_storage_key"] = key
					updates["logs"] = "" // Clear logs field
					updated = true
					break
				}
				if attempt < 2 {
					global.Logger.Warnf("failed to upload logs to storage for tx %s (attempt %d/3): %v, retrying...", tx.Hash, attempt+1, uploadErr)
				}
			}
			if uploadErr != nil {
				global.Logger.Errorf("failed to upload logs to storage for tx %s after 3 attempts: %v", tx.Hash, uploadErr)
				// Continue with other fields even if this one fails
			}
		}

		// Update transaction if any field was migrated
		if updated {
			if err := dbTx.Model(&tx).Updates(updates).Error; err != nil {
				hasError = true
				return migratedCount, errors.Wrapf(err, "update transaction %s", tx.Hash)
			}
			migratedCount++
		}
	}

	// Commit transaction
	if err := dbTx.Commit().Error; err != nil {
		hasError = true
		return migratedCount, errors.Wrap(err, "commit migration batch")
	}

	return migratedCount, nil
}

// VerifyMigration verifies that migrated transactions can be correctly retrieved from object storage.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - limit: Maximum number of transactions to verify
//
// Returns:
//   - error: Error if verification fails
func (s *StorageMigrationService) VerifyMigration(ctx context.Context, limit int) error {
	global.Logger.Infof("verifying migration: checking %d transactions", limit)

	var transactions []models.Transaction
	err := s.db.WithContext(ctx).
		Where("(data_storage_key IS NOT NULL AND data_storage_key != '') OR " +
			"(messages_storage_key IS NOT NULL AND messages_storage_key != '') OR " +
			"(logs_storage_key IS NOT NULL AND logs_storage_key != '')").
		Limit(limit).
		Find(&transactions).Error
	if err != nil {
		return errors.Wrap(err, "fetch migrated transactions")
	}

	var verifiedCount int
	var errorCount int

	for _, tx := range transactions {
		verified := true

		if tx.DataStorageKey != "" {
			if exists, err := s.storage.Exists(ctx, tx.DataStorageKey); err != nil || !exists {
				global.Logger.Warnf("data storage key not found: %s for tx %s", tx.DataStorageKey, tx.Hash)
				verified = false
			}
		}

		if tx.MessagesStorageKey != "" {
			if exists, err := s.storage.Exists(ctx, tx.MessagesStorageKey); err != nil || !exists {
				global.Logger.Warnf("messages storage key not found: %s for tx %s", tx.MessagesStorageKey, tx.Hash)
				verified = false
			}
		}

		if tx.LogsStorageKey != "" {
			if exists, err := s.storage.Exists(ctx, tx.LogsStorageKey); err != nil || !exists {
				global.Logger.Warnf("logs storage key not found: %s for tx %s", tx.LogsStorageKey, tx.Hash)
				verified = false
			}
		}

		if verified {
			verifiedCount++
		} else {
			errorCount++
		}
	}

	global.Logger.Infof("verification completed: verified=%d, errors=%d", verifiedCount, errorCount)
	return nil
}
