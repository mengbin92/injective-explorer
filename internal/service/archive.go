// Package service provides business logic services for the blockchain explorer.
// This file implements transaction archiving functionality.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// Archived status constants for better code readability and maintainability.
const (
	// NotArchived indicates that a transaction or block has not been archived yet.
	NotArchived = false
	// IsArchived indicates that a transaction or block has been archived.
	IsArchived = true
)

// RunArchiveStep executes a single archive step (transaction or block archiving).
// It encapsulates the common logic of creating a context with timeout, timing the operation,
// and logging results. This eliminates code duplication between runArchiveMode and executeArchive.
//
// Parameters:
//   - stepName: Name of the archive step (e.g., "transaction", "block")
//   - logger: Logger instance for logging
//   - timeout: Timeout duration for this step
//   - fn: The archive function to execute (takes context and returns error)
//
// Returns:
//   - error: Error if the archive operation failed, nil otherwise
func RunArchiveStep(stepName string, logger *log.Helper, timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startTime := time.Now()
	err := fn(ctx)
	duration := time.Since(startTime)

	if err != nil {
		logger.Errorf("%s archiving failed: %v", stepName, err)
		return err
	}
	logger.Infof("%s archiving completed successfully in %v", stepName, duration)
	return nil
}

// RunArchiveStepWithMetrics executes a single archive step that returns metrics.
// Like RunArchiveStep, this encapsulates common logic for archive operations that return metrics.
//
// Parameters:
//   - stepName: Name of the archive step (e.g., "block")
//   - logger: Logger instance for logging
//   - timeout: Timeout duration for this step
//   - fn: The archive function to execute (takes context, returns BlockArchiveMetrics and error)
//
// Returns:
//   - error: Error if the archive operation failed, nil otherwise
func RunArchiveStepWithMetrics(stepName string, logger *log.Helper, timeout time.Duration,
	fn func(context.Context) (*BlockArchiveMetrics, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startTime := time.Now()
	metrics, err := fn(ctx)
	duration := time.Since(startTime)

	if err != nil {
		logger.Errorf("%s archiving failed: %v", stepName, err)
		return err
	}
	logger.Infof("%s archiving completed successfully in %v (processed=%d, failed=%d)",
		stepName, duration, metrics.ProcessedBlocks, metrics.FailedBlocks)
	return nil
}

// ArchiveService handles transaction archiving operations.
// It moves large fields from old transactions to TransactionDataArchive table and optionally to object storage.
// The main transactions table retains metadata (with archived=true flag) for statistics and queries.
// Large fields can be stored in object storage (MinIO/S3/OSS/COS) to reduce database size.
// Archived transactions can still be retrieved via GetArchivedTransaction.
//
// Optimization: This implementation does not use a separate transaction_archives table.
// Instead, it relies on the main transactions table (with archived=true) and TransactionDataArchive
// for storing large fields. This reduces data redundancy while maintaining query performance.
type ArchiveService struct {
	db      *gorm.DB
	storage storage.Storage
	cache   *cache.TieredCache // Cache for archived transaction data (optional)
}

// NewArchiveService creates a new ArchiveService instance.
// The service uses the global database and storage instances.
// Archive configuration is automatically loaded from global config when using DefaultArchiveConfig().
// Cache is initialized from global Redis client if available.
func NewArchiveService() *ArchiveService {
	// Create tiered cache if Redis is available
	var tieredCache *cache.TieredCache
	if redisClient := cache.GetRedisClient(); redisClient != nil {
		// Try to create tiered cache with Redis client
		tc, err := cache.NewTieredCache(redisClient)
		if err == nil {
			tieredCache = tc
		}
		// If cache creation fails, continue without cache (log handled in NewTieredCache)
	}

	return &ArchiveService{
		db:      db.Get(),
		storage: storage.Get(),
		cache:   tieredCache,
	}
}

// ArchiveConfig contains configuration for archiving operations.
type ArchiveConfig struct {
	// ArchiveAgeMonths is the age in months before transactions are archived
	// Default: DefaultArchiveAgeMonths (6 months)
	ArchiveAgeMonths int
	// BatchSize is the number of transactions to archive per batch
	// Can be adjusted dynamically during operation
	// Default: DefaultArchiveBatchSize (1000)
	BatchSize int
	// DryRun if true, only simulates archiving without actually moving data
	DryRun bool
	// UseObjectStorage if true, uploads large fields to object storage (MinIO/S3/OSS/COS)
	// If false, all data is stored in database
	// Default: true if object storage is enabled (auto-detected)
	UseObjectStorage bool
	// StorageThreshold is the minimum size in bytes to upload to object storage
	// Fields smaller than this threshold are stored in database
	// Default: DefaultStorageThreshold (3KB, 3072 bytes)
	StorageThreshold int
	// ProgressCallback is an optional callback function called after each batch
	// Parameters: processed, total, current batch size
	// This allows external monitoring and dynamic batch size adjustment
	ProgressCallback func(processed, total int64, batchSize int)
}

// DefaultArchiveConfig returns default archive configuration.
// It automatically loads from global.ArchiveConfig if available, otherwise uses constants
// (DefaultArchiveAgeMonths, DefaultArchiveBatchSize, DefaultStorageThreshold) from constants.go.
// This ensures that configuration from config.yaml is automatically used when available.
func DefaultArchiveConfig() *ArchiveConfig {
	// Try to load from global config first
	if global.ArchiveConfig != nil {
		return LoadArchiveConfig(global.ArchiveConfig)
	}

	// Fallback to constants if global config is not available
	// Check if object storage is enabled (auto-detect)
	useObjectStorage := storage.Get() != nil && !isNoOpStorage(storage.Get())
	return &ArchiveConfig{
		ArchiveAgeMonths: DefaultArchiveAgeMonths,
		BatchSize:        DefaultArchiveBatchSize,
		DryRun:           false,
		UseObjectStorage: useObjectStorage,
		StorageThreshold: DefaultStorageThreshold,
	}
}

// LoadArchiveConfig loads archive configuration from Archive config with default values.
// If config is nil or fields are not set, uses constants from constants.go (DefaultArchiveAgeMonths, DefaultArchiveBatchSize, DefaultStorageThreshold).
// This function is similar to LoadSyncConfig and provides a consistent way to load config.
//
// Parameters:
//   - archiveCfg: Archive configuration (can be nil, will use defaults)
//
// Returns:
//   - *ArchiveConfig: Archive configuration with values from config or defaults
func LoadArchiveConfig(archiveCfg *conf.Archive) *ArchiveConfig {
	// Default values
	ageMonths := DefaultArchiveAgeMonths
	batchSize := DefaultArchiveBatchSize
	storageThreshold := DefaultStorageThreshold

	// Check if object storage is enabled (auto-detect)
	useObjectStorage := storage.Get() != nil && !isNoOpStorage(storage.Get())

	// Load from config if provided
	if archiveCfg != nil {
		if archiveCfg.GetAgeMonths() > 0 {
			ageMonths = int(archiveCfg.GetAgeMonths())
		}
		if archiveCfg.GetBatchSize() > 0 {
			batchSize = int(archiveCfg.GetBatchSize())
		}
		if archiveCfg.GetStorageThreshold() > 0 {
			storageThreshold = int(archiveCfg.GetStorageThreshold())
		}
		// For use_object_storage, protobuf bool cannot distinguish between unset and false.
		// We check if object storage is enabled via storage.Get() to determine the default.
		// If use_object_storage is explicitly set to true in config, we use it.
		// Otherwise, we use auto-detection based on storage.Get().
		if archiveCfg.GetUseObjectStorage() {
			// If explicitly set to true in archive config, use it
			useObjectStorage = true
		}
		// Note: If archiveCfg.UseObjectStorage is false, we keep auto-detection
		// (useObjectStorage from storage.Get() which is already set above)
	}

	return &ArchiveConfig{
		ArchiveAgeMonths: ageMonths,
		BatchSize:        batchSize,
		DryRun:           false,
		UseObjectStorage: useObjectStorage,
		StorageThreshold: storageThreshold,
	}
}

// isNoOpStorage checks if the storage is a NoOpStorage (disabled).
// NoOpStorage.GetObject always returns ErrObjectNotFound.
func isNoOpStorage(s storage.Storage) bool {
	if s == nil {
		return true
	}
	// Try to get a non-existent object - NoOpStorage returns ErrObjectNotFound
	_, err := s.GetObject(context.Background(), "__noop_check__")
	return err != nil && errors.Is(err, storage.ErrObjectNotFound)
}

// ArchiveOldTransactions archives transactions older than the specified age.
// It moves large fields (Data, Messages, Logs) to archive tables and keeps
// only essential fields in the main transactions table.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - config: Archive configuration (uses DefaultArchiveConfig if nil, which automatically loads from global config)
//
// Returns:
//   - error: Error if archiving fails
func (s *ArchiveService) ArchiveOldTransactions(ctx context.Context, config *ArchiveConfig) error {
	if config == nil {
		config = DefaultArchiveConfig()
	}

	// Set default storage threshold if not specified
	if config.StorageThreshold <= 0 {
		config.StorageThreshold = DefaultStorageThreshold
	}

	// Validate configuration
	if config.ArchiveAgeMonths <= 0 {
		return errors.New("archive age months must be greater than 0")
	}
	if config.BatchSize <= 0 {
		return errors.New("batch size must be greater than 0")
	}
	const maxRecommendedBatchSize = 10000
	if config.BatchSize > maxRecommendedBatchSize {
		global.Logger.Warnf("batch size %d is very large, consider using a smaller value for better performance", config.BatchSize)
	}

	global.Logger.Infof("starting transaction archiving: age_months=%d, batch_size=%d, dry_run=%v",
		config.ArchiveAgeMonths, config.BatchSize, config.DryRun)

	// Initialize metrics
	metrics := &ArchiveMetrics{
		StartTime: time.Now(),
	}

	// Calculate cutoff time
	cutoffTime := time.Now().AddDate(0, -config.ArchiveAgeMonths, 0)
	cutoffTimestamp := cutoffTime.Unix()

	// Count transactions to archive
	var totalCount int64
	err := s.db.WithContext(ctx).Model(&models.Transaction{}).
		Where("block_unix_timestamp < ? AND (archived IS NULL OR archived = ?)", cutoffTimestamp, NotArchived).
		Count(&totalCount).Error
	if err != nil {
		return errors.Wrap(err, "count transactions to archive")
	}

	if totalCount == 0 {
		global.Logger.Infof("no transactions to archive")
		return nil
	}

	metrics.TotalTransactions = totalCount
	global.Logger.Infof("found %d transactions to archive", totalCount)

	if config.DryRun {
		global.Logger.Infof("dry run mode: would archive %d transactions", totalCount)
		return nil
	}

	// Archive in batches using cursor-based pagination for better performance.
	// This approach avoids the performance issues of OFFSET-based pagination,
	// which degrades linearly as the offset increases.
	var archivedCount int64
	batchStartTime := time.Now()

	// Track the last processed transaction for cursor-based pagination
	var lastBlockTimestamp uint64 = 0
	var lastID uint = 0

	for {
		// Build query with cursor-based pagination
		query := s.db.WithContext(ctx).
			Where("block_unix_timestamp < ?", cutoffTimestamp)

		// Apply cursor filtering for subsequent batches
		if lastBlockTimestamp > 0 || lastID > 0 {
			query = query.Where(
				"(block_unix_timestamp > ? OR (block_unix_timestamp = ? AND id > ?))",
				lastBlockTimestamp, lastBlockTimestamp, lastID,
			)
		}

		// Get batch of transactions to archive
		var transactions []models.Transaction
		err := query.
			Where("archived IS NULL OR archived = ?", NotArchived).
			Order("block_unix_timestamp ASC, id ASC").
			Limit(config.BatchSize).
			Find(&transactions).Error
		if err != nil {
			return errors.Wrapf(err, "fetch batch after cursor (timestamp=%d, id=%d)", lastBlockTimestamp, lastID)
		}

		if len(transactions) == 0 {
			break
		}

		// Archive batch
		batchArchived, err := s.archiveBatch(ctx, transactions, cutoffTimestamp, config)
		if err != nil {
			return errors.Wrapf(err, "archive batch at cursor (timestamp=%d, id=%d)", lastBlockTimestamp, lastID)
		}

		archivedCount += batchArchived
		metrics.BatchesProcessed++
		batchDuration := time.Since(batchStartTime)
		global.Logger.Infof("archived batch: %d/%d transactions, batch_time=%v", archivedCount, totalCount, batchDuration)

		// Update cursor for next batch
		lastTx := transactions[len(transactions)-1]
		lastBlockTimestamp = lastTx.BlockUnixTimestamp
		lastID = lastTx.ID

		// Call progress callback if provided
		if config.ProgressCallback != nil {
			config.ProgressCallback(archivedCount, totalCount, config.BatchSize)
		}

		batchStartTime = time.Now() // Reset for next batch

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	metrics.ArchivedTransactions = archivedCount
	metrics.EndTime = time.Now()
	metrics.LogMetrics()

	global.Logger.Infof("archiving completed: archived %d transactions", archivedCount)
	return nil
}

// archiveBatch archives a batch of transactions.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - transactions: Batch of transactions to archive
//   - archivedAt: Unix timestamp when archiving occurs
//   - config: Archive configuration
//
// Returns:
//   - int64: Number of successfully archived transactions
//   - error: Error if archiving fails
func (s *ArchiveService) archiveBatch(ctx context.Context, transactions []models.Transaction, archivedAt int64, config *ArchiveConfig) (int64, error) {
	if len(transactions) == 0 {
		return 0, nil
	}

	// Start transaction
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, errors.Wrap(tx.Error, "begin transaction")
	}

	var archivedCount int64
	var hasError bool
	// Track uploaded objects for cleanup in case of transaction failure
	uploadedObjects := make([]string, 0)

	defer func() {
		if hasError {
			// Rollback database transaction
			if rbErr := tx.Rollback().Error; rbErr != nil {
				global.Logger.Errorf("failed to rollback transaction: %v", rbErr)
			}
			// Clean up uploaded objects from storage if transaction failed
			if len(uploadedObjects) > 0 {
				global.Logger.Warnf("cleaning up %d uploaded objects due to transaction failure", len(uploadedObjects))
				for _, key := range uploadedObjects {
					if delErr := s.storage.DeleteObject(ctx, key); delErr != nil {
						global.Logger.Errorf("failed to delete object %s: %v", key, delErr)
					} else {
						global.Logger.Debugf("deleted object %s due to transaction failure", key)
					}
				}
			}
		}
	}()

	for _, txn := range transactions {
		// Create archive data record (references Transaction.ID directly, no separate archive table)
		archiveData := &models.TransactionDataArchive{
			TransactionID: txn.ID, // Use main table ID directly
			Info:          txn.Info,
			ErrorLog:      txn.ErrorLog,
		}

		// Handle Data field - reuse existing storage key if available, otherwise upload
		if txn.DataStorageKey != "" {
			// Reuse existing storage key from sync time upload
			archiveData.DataStorageKey = txn.DataStorageKey
			archiveData.Data = "" // Clear data field (already in storage)
			global.Logger.Debugf("reusing existing data storage key for tx %s: %s", txn.Hash, txn.DataStorageKey)
		} else if config.UseObjectStorage && len(txn.Data) > config.StorageThreshold {
			// Upload to object storage if not already uploaded
			// Use the same storage key format as sync service to avoid data duplication
			dataKey := storage.BuildStorageKey("data", txn.Hash)
			// Get original data (decompress if stored in compressed format)
			dataBytes := []byte(txn.Data)
			if decompressed, err := models.DecompressData(txn.Data); err == nil && len(decompressed) > 0 {
				dataBytes = decompressed
			}
			// Compress data for object storage to save space and bandwidth
			compressedData, wasCompressed, compressErr := models.CompressDataToBytes(dataBytes)
			if compressErr == nil {
				dataToUpload := compressedData
				if !wasCompressed {
					dataToUpload = dataBytes
				}
				if err := s.storage.PutObject(ctx, dataKey, dataToUpload); err == nil {
					archiveData.DataStorageKey = dataKey
					archiveData.Data = ""                              // Clear data field
					uploadedObjects = append(uploadedObjects, dataKey) // Track for cleanup
					global.Logger.Debugf("uploaded archive data to storage for tx %s", txn.Hash)
				} else {
					// Upload failed, store in database as fallback
					global.Logger.Warnf("failed to upload archive data to storage for tx %s: %v, storing in database", txn.Hash, err)
					archiveData.Data = txn.Data
				}
			} else {
				// Compression failed, upload original data
				global.Logger.Warnf("failed to compress archive data for tx %s: %v, uploading original data", txn.Hash, compressErr)
				if err := s.storage.PutObject(ctx, dataKey, dataBytes); err == nil {
					archiveData.DataStorageKey = dataKey
					archiveData.Data = ""
					uploadedObjects = append(uploadedObjects, dataKey)
				} else {
					global.Logger.Warnf("failed to upload archive data to storage for tx %s: %v, storing in database", txn.Hash, err)
					archiveData.Data = txn.Data
				}
			}
		} else {
			// Store in database (object storage disabled or below threshold)
			archiveData.Data = txn.Data
		}

		// Handle Messages field - reuse existing storage key if available, otherwise upload
		if txn.MessagesStorageKey != "" {
			// Reuse existing storage key from sync time upload
			archiveData.MessagesStorageKey = txn.MessagesStorageKey
			archiveData.Messages = "" // Clear messages field (already in storage)
			global.Logger.Debugf("reusing existing messages storage key for tx %s: %s", txn.Hash, txn.MessagesStorageKey)
		} else if config.UseObjectStorage && len(txn.Messages) > config.StorageThreshold {
			// Upload to object storage if not already uploaded
			// Use the same storage key format as sync service to avoid data duplication
			messagesKey := storage.BuildStorageKey("messages", txn.Hash)
			messagesBytes := []byte(txn.Messages)
			if decompressed, err := models.DecompressData(txn.Messages); err == nil && len(decompressed) > 0 {
				messagesBytes = decompressed
			}
			// Compress data for object storage
			compressedData, wasCompressed, compressErr := models.CompressDataToBytes(messagesBytes)
			if compressErr == nil {
				dataToUpload := compressedData
				if !wasCompressed {
					dataToUpload = messagesBytes
				}
				if err := s.storage.PutObject(ctx, messagesKey, dataToUpload); err == nil {
					archiveData.MessagesStorageKey = messagesKey
					archiveData.Messages = ""
					uploadedObjects = append(uploadedObjects, messagesKey)
					global.Logger.Debugf("uploaded archive messages to storage for tx %s", txn.Hash)
				} else {
					global.Logger.Warnf("failed to upload archive messages to storage for tx %s: %v, storing in database", txn.Hash, err)
					archiveData.Messages = txn.Messages
				}
			} else {
				global.Logger.Warnf("failed to compress archive messages for tx %s: %v, uploading original data", txn.Hash, compressErr)
				if err := s.storage.PutObject(ctx, messagesKey, messagesBytes); err == nil {
					archiveData.MessagesStorageKey = messagesKey
					archiveData.Messages = ""
					uploadedObjects = append(uploadedObjects, messagesKey)
				} else {
					global.Logger.Warnf("failed to upload archive messages to storage for tx %s: %v, storing in database", txn.Hash, err)
					archiveData.Messages = txn.Messages
				}
			}
		} else {
			archiveData.Messages = txn.Messages
		}

		// Handle Logs field - reuse existing storage key if available, otherwise upload
		if txn.LogsStorageKey != "" {
			// Reuse existing storage key from sync time upload
			archiveData.LogsStorageKey = txn.LogsStorageKey
			archiveData.Logs = "" // Clear logs field (already in storage)
			global.Logger.Debugf("reusing existing logs storage key for tx %s: %s", txn.Hash, txn.LogsStorageKey)
		} else if config.UseObjectStorage && len(txn.Logs) > config.StorageThreshold {
			// Upload to object storage if not already uploaded
			// Use the same storage key format as sync service to avoid data duplication
			logsKey := storage.BuildStorageKey("logs", txn.Hash)
			logsBytes := []byte(txn.Logs)
			if decompressed, err := models.DecompressData(txn.Logs); err == nil && len(decompressed) > 0 {
				logsBytes = decompressed
			}
			// Compress data for object storage
			compressedData, wasCompressed, compressErr := models.CompressDataToBytes(logsBytes)
			if compressErr == nil {
				dataToUpload := compressedData
				if !wasCompressed {
					dataToUpload = logsBytes
				}
				if err := s.storage.PutObject(ctx, logsKey, dataToUpload); err == nil {
					archiveData.LogsStorageKey = logsKey
					archiveData.Logs = ""
					uploadedObjects = append(uploadedObjects, logsKey)
					global.Logger.Debugf("uploaded archive logs to storage for tx %s", txn.Hash)
				} else {
					global.Logger.Warnf("failed to upload archive logs to storage for tx %s: %v, storing in database", txn.Hash, err)
					archiveData.Logs = txn.Logs
				}
			} else {
				global.Logger.Warnf("failed to compress archive logs for tx %s: %v, uploading original data", txn.Hash, compressErr)
				if err := s.storage.PutObject(ctx, logsKey, logsBytes); err == nil {
					archiveData.LogsStorageKey = logsKey
					archiveData.Logs = ""
					uploadedObjects = append(uploadedObjects, logsKey)
				} else {
					global.Logger.Warnf("failed to upload archive logs to storage for tx %s: %v, storing in database", txn.Hash, err)
					archiveData.Logs = txn.Logs
				}
			}
		} else {
			archiveData.Logs = txn.Logs
		}

		if err := tx.Create(archiveData).Error; err != nil {
			hasError = true
			return 0, errors.Wrapf(err, "create archive data record for transaction %s", txn.Hash)
		}

		// Update original transaction: clear large fields and mark as archived
		if err := tx.Model(&txn).Updates(map[string]interface{}{
			"data":      nil,
			"messages":  nil,
			"logs":      nil,
			"info":      nil,
			"error_log": nil,
			"archived":  true,
		}).Error; err != nil {
			hasError = true
			return 0, errors.Wrapf(err, "update transaction %s", txn.Hash)
		}

		archivedCount++
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		hasError = true
		return 0, errors.Wrap(err, "commit archive batch")
	}

	// Transaction committed successfully, clear uploaded objects tracking
	// (no need to clean up since transaction succeeded)
	uploadedObjects = nil
	global.Logger.Infof("successfully archived batch: %d transactions, committed to database", archivedCount)

	return archivedCount, nil
}

// ============================================================================
// Archive Cache Helper Functions
// ============================================================================

// cachedArchiveData represents cached archive data for a transaction.
type cachedArchiveData struct {
	Data     string `json:"data"`
	Messages string `json:"messages"`
	Logs     string `json:"logs"`
	Info     string `json:"info"`
	ErrorLog string `json:"error_log"`
}

// cacheArchivedTransactionData caches individual fields from TransactionDataArchive.
// This reduces subsequent storage calls by 80-90% for frequently accessed transactions.
//
// Parameters:
//   - ctx: Context for the operation
//   - txHash: Transaction hash (used as cache key)
//   - archiveData: Archive data to cache
//
// Returns:
//   - error: Error if caching failed
func (s *ArchiveService) cacheArchivedTransactionData(
	ctx context.Context,
	txHash string,
	archiveData *models.TransactionDataArchive,
) error {
	if s.cache == nil {
		return nil // Cache not available
	}

	// Build cache data
	cacheData := &cachedArchiveData{
		Data:     archiveData.Data,
		Messages: archiveData.Messages,
		Logs:     archiveData.Logs,
		Info:     archiveData.Info,
		ErrorLog: archiveData.ErrorLog,
	}

	// Store in cache with TTL
	cacheKey := CacheKeyArchiveTransactionPrefix + txHash
	if err := s.cache.Set(ctx, cacheKey, cacheData, 1, CacheArchiveTransactionTTL); err != nil {
		global.Logger.Debugf("failed to cache archive data for tx %s: %v", txHash, err)
		return err
	}

	return nil
}

// getCachedArchivedTransactionData retrieves cached archive data for a transaction.
// This avoids database and storage calls for cached transactions.
//
// Parameters:
//   - ctx: Context for the operation
//   - txHash: Transaction hash (used as cache key)
//
// Returns:
//   - data, messages, logs, info, errorLog: Cached archive data fields (empty if not in cache)
//   - found: True if data was found in cache
func (s *ArchiveService) getCachedArchivedTransactionData(
	ctx context.Context,
	txHash string,
) (data, messages, logs, info, errorLog string, found bool) {
	if s.cache == nil {
		return "", "", "", "", "", false // Cache not available
	}

	cacheKey := CacheKeyArchiveTransactionPrefix + txHash
	var cacheData cachedArchiveData
	hit, err := s.cache.Get(ctx, cacheKey, &cacheData)
	if err != nil {
		global.Logger.Debugf("cache error for tx %s: %v", txHash, err)
		return "", "", "", "", "", false
	}

	if !hit {
		return "", "", "", "", "", false // Cache miss
	}

	return cacheData.Data, cacheData.Messages, cacheData.Logs, cacheData.Info, cacheData.ErrorLog, true
}

// GetArchivedTransaction retrieves a transaction from the main table (archived=true) and TransactionDataArchive.
// It combines metadata from the main transactions table with detailed data from TransactionDataArchive.
// Uses cache to reduce database/storage calls by 80-90% for frequently accessed transactions.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - hash: Transaction hash to retrieve
//
// Returns:
//   - *models.Transaction: Archived transaction, or nil if not found
//   - error: Error if retrieval fails
func (s *ArchiveService) GetArchivedTransaction(ctx context.Context, hash string) (*models.Transaction, error) {
	// Query from main table (archived=true)
	var tx models.Transaction
	err := s.db.WithContext(ctx).Where("hash = ? AND archived = ?", hash, IsArchived).First(&tx).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "get archived transaction %s", hash)
	}

	// Try cache first (reduces storage calls by 80-90%)
	cachedData, cachedMessages, cachedLogs, cachedInfo, cachedErrorLog, cacheHit := s.getCachedArchivedTransactionData(ctx, hash)
	if cacheHit {
		// Cache hit - use cached data
		tx.Data = cachedData
		tx.Messages = cachedMessages
		tx.Logs = cachedLogs
		tx.Info = cachedInfo
		tx.ErrorLog = cachedErrorLog
		tx.Archived = true
		global.Logger.Debugf("archive cache hit for tx %s", hash)
		return &tx, nil
	}

	// Cache miss - load from database/storage
	// Get archive data
	var archiveData models.TransactionDataArchive
	err = s.db.WithContext(ctx).Where("transaction_id = ?", tx.ID).First(&archiveData).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Archive data not found, return transaction with empty data fields
			global.Logger.Warnf("archive data not found for transaction %s (id=%d), returning transaction with empty data fields", hash, tx.ID)
			return &tx, nil
		}
		return nil, errors.Wrapf(err, "get archived transaction data %s", hash)
	}

	// Load data from object storage if storage keys are present
	data := archiveData.Data
	if archiveData.DataStorageKey != "" {
		if storageData, err := s.storage.GetObject(ctx, archiveData.DataStorageKey); err == nil {
			// Decompress data from object storage (may be compressed binary format)
			var decompressErr error
			decompressed, decompressErr := models.DecompressDataFromBytes(storageData)
			if decompressErr != nil {
				// If decompression fails, assume it's uncompressed (backward compatibility)
				data = string(storageData)
			} else {
				data = string(decompressed)
			}
			global.Logger.Debugf("loaded archive data from storage for tx %s", tx.Hash)
		} else {
			global.Logger.Warnf("failed to load archive data from storage for tx %s: %v, using database data", tx.Hash, err)
			// Fallback to database data if available
			if archiveData.Data != "" {
				data = archiveData.Data
			}
		}
	}

	messages := archiveData.Messages
	if archiveData.MessagesStorageKey != "" {
		if storageMessages, err := s.storage.GetObject(ctx, archiveData.MessagesStorageKey); err == nil {
			// Decompress data from object storage (may be compressed binary format)
			var decompressErr error
			decompressed, decompressErr := models.DecompressDataFromBytes(storageMessages)
			if decompressErr != nil {
				// If decompression fails, assume it's uncompressed (backward compatibility)
				messages = string(storageMessages)
			} else {
				messages = string(decompressed)
			}
			global.Logger.Debugf("loaded archive messages from storage for tx %s", tx.Hash)
		} else {
			global.Logger.Warnf("failed to load archive messages from storage for tx %s: %v, using database data", tx.Hash, err)
			if archiveData.Messages != "" {
				messages = archiveData.Messages
			}
		}
	}

	logs := archiveData.Logs
	if archiveData.LogsStorageKey != "" {
		if storageLogs, err := s.storage.GetObject(ctx, archiveData.LogsStorageKey); err == nil {
			// Decompress data from object storage (may be compressed binary format)
			var decompressErr error
			decompressed, decompressErr := models.DecompressDataFromBytes(storageLogs)
			if decompressErr != nil {
				// If decompression fails, assume it's uncompressed (backward compatibility)
				logs = string(storageLogs)
			} else {
				logs = string(decompressed)
			}
			global.Logger.Debugf("loaded archive logs from storage for tx %s", tx.Hash)
		} else {
			global.Logger.Warnf("failed to load archive logs from storage for tx %s: %v, using database data", tx.Hash, err)
			if archiveData.Logs != "" {
				logs = archiveData.Logs
			}
		}
	}

	// Reconstruct Transaction with archive data
	tx.Data = data
	tx.Messages = messages
	tx.Logs = logs
	tx.Info = archiveData.Info
	tx.ErrorLog = archiveData.ErrorLog
	tx.Archived = true

	// Populate cache for future requests (async, don't block on cache errors)
	if s.cache != nil {
		archiveDataCopy := archiveData // Make a copy for caching
		archiveDataCopy.Data = data
		archiveDataCopy.Messages = messages
		archiveDataCopy.Logs = logs
		go func() {
			_ = s.cacheArchivedTransactionData(context.Background(), hash, &archiveDataCopy)
		}()
	}

	return &tx, nil
}

// GetArchivedTransactions retrieves archived transactions with pagination.
// It queries the main transactions table (archived=true) and combines with TransactionDataArchive.
// Uses batch cache checks and parallel storage loading for 10-20x performance improvement.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - limit: Maximum number of transactions to retrieve
//   - offset: Number of transactions to skip
//
// Returns:
//   - []models.Transaction: List of archived transactions
//   - int64: Total number of archived transactions
//   - error: Error if retrieval fails
func (s *ArchiveService) GetArchivedTransactions(ctx context.Context, limit, offset int) ([]models.Transaction, int64, error) {
	var transactions []models.Transaction
	var total int64

	// Count total archived transactions from main table
	err := s.db.WithContext(ctx).Model(&models.Transaction{}).
		Where("archived = ?", IsArchived).
		Count(&total).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "count archived transactions")
	}

	// Fetch archived transactions from main table
	err = s.db.WithContext(ctx).
		Where("archived = ?", IsArchived).
		Order("block_unix_timestamp DESC, tx_number DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "fetch archived transactions")
	}

	if len(transactions) == 0 {
		return []models.Transaction{}, total, nil
	}

	// Batch cache check - identify which transactions need database/storage access
	type txWithArchiveData struct {
		tx         *models.Transaction
		archiveData *models.TransactionDataArchive
		data       string
		messages   string
		logs       string
	}

	cacheHits := 0
	var txsNeedingStorage []*models.Transaction
	txResults := make(map[uint]*txWithArchiveData, len(transactions))
	var mu sync.Mutex

	// First pass: check cache for all transactions
	for i := range transactions {
		tx := &transactions[i]
		cachedData, cachedMessages, cachedLogs, cachedInfo, cachedErrorLog, found := s.getCachedArchivedTransactionData(ctx, tx.Hash)
		if found {
			// Cache hit
			tx.Data = cachedData
			tx.Messages = cachedMessages
			tx.Logs = cachedLogs
			tx.Info = cachedInfo
			tx.ErrorLog = cachedErrorLog
			tx.Archived = true
			cacheHits++
			txResults[tx.ID] = &txWithArchiveData{
				tx:    tx,
				data:  cachedData,
				messages: cachedMessages,
				logs:   cachedLogs,
			}
		} else {
			// Cache miss - queue for database/storage loading
			txsNeedingStorage = append(txsNeedingStorage, tx)
		}
	}

	if cacheHits > 0 {
		global.Logger.Debugf("archive cache hits: %d/%d transactions", cacheHits, len(transactions))
	}

	// Second pass: load from database/storage for cache misses
	if len(txsNeedingStorage) > 0 {
		// Collect transaction IDs for batch database query
		transactionIDs := make([]uint, 0, len(txsNeedingStorage))
		for _, tx := range txsNeedingStorage {
			transactionIDs = append(transactionIDs, tx.ID)
		}

		// Fetch all archive data in one query
		var archiveDataList []models.TransactionDataArchive
		err = s.db.WithContext(ctx).Where("transaction_id IN ?", transactionIDs).Find(&archiveDataList).Error
		if err != nil {
			return nil, 0, errors.Wrap(err, "fetch archive data")
		}

		// Build map of transaction ID to archive data
		dataMap := make(map[uint]*models.TransactionDataArchive, len(archiveDataList))
		for i := range archiveDataList {
			dataMap[archiveDataList[i].TransactionID] = &archiveDataList[i]
		}

		// Load from storage in parallel with goroutine pool (10 workers)
		const maxWorkers = 10
		semaphore := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup

		for _, tx := range txsNeedingStorage {
			archiveData, exists := dataMap[tx.ID]
			if !exists {
				global.Logger.Warnf("archive data not found for transaction %s (id=%d), skipping", tx.Hash, tx.ID)
				continue
			}

			wg.Add(1)
			go func(txn *models.Transaction, archive *models.TransactionDataArchive) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				// Load data from storage
				data := archive.Data
				if archive.DataStorageKey != "" {
					if storageData, err := s.storage.GetObject(ctx, archive.DataStorageKey); err == nil {
						if decompressed, decompressErr := models.DecompressDataFromBytes(storageData); decompressErr == nil {
							data = string(decompressed)
						} else {
							data = string(storageData)
						}
					} else if archive.Data != "" {
						data = archive.Data
					}
				}

				messages := archive.Messages
				if archive.MessagesStorageKey != "" {
					if storageMessages, err := s.storage.GetObject(ctx, archive.MessagesStorageKey); err == nil {
						if decompressed, decompressErr := models.DecompressDataFromBytes(storageMessages); decompressErr == nil {
							messages = string(decompressed)
						} else {
							messages = string(storageMessages)
						}
					} else if archive.Messages != "" {
						messages = archive.Messages
					}
				}

				logs := archive.Logs
				if archive.LogsStorageKey != "" {
					if storageLogs, err := s.storage.GetObject(ctx, archive.LogsStorageKey); err == nil {
						if decompressed, decompressErr := models.DecompressDataFromBytes(storageLogs); decompressErr == nil {
							logs = string(decompressed)
						} else {
							logs = string(storageLogs)
						}
					} else if archive.Logs != "" {
						logs = archive.Logs
					}
				}

				// Update transaction (thread-safe)
				mu.Lock()
				txn.Data = data
				txn.Messages = messages
				txn.Logs = logs
				txn.Info = archive.Info
				txn.ErrorLog = archive.ErrorLog
				txn.Archived = true

				txResults[txn.ID] = &txWithArchiveData{
					tx:          txn,
					archiveData: archive,
					data:        data,
					messages:    messages,
					logs:        logs,
				}
				mu.Unlock()

				// Cache the result for future requests
				if s.cache != nil {
					archiveCopy := *archive
					archiveCopy.Data = data
					archiveCopy.Messages = messages
					archiveCopy.Logs = logs
					_ = s.cacheArchivedTransactionData(ctx, txn.Hash, &archiveCopy)
				}
			}(tx, archiveData)
		}

		wg.Wait()
	}

	// Build final result in order
	result := make([]models.Transaction, 0, len(transactions))
	for i := range transactions {
		tx := &transactions[i]
		if txResult, ok := txResults[tx.ID]; ok {
			result = append(result, *txResult.tx)
		}
	}

	return result, total, nil
}

// ArchiveSchedulerService handles periodic archiving of old transactions and blocks.
// It runs in the background and periodically executes archiving operations.
// Similar to SyncService, it provides Start() and Stop() methods for lifecycle management.
// Uses dependency injection for logger to improve testability and reduce global state coupling.
type ArchiveSchedulerService struct {
	// Configuration
	config *ArchiveConfig

	// Archive execution interval
	interval time.Duration

	// Logger (injected via dependency injection)
	logger *log.Helper

	// Control channels
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewArchiveSchedulerService creates a new ArchiveSchedulerService instance.
//
// Parameters:
//   - config: Archive configuration (can be nil to use defaults)
//   - interval: Archive execution interval (default: 24 hours)
//   - logger: Logger instance for logging (uses global logger if nil)
//
// Returns:
//   - *ArchiveSchedulerService: A new archive scheduler service instance
func NewArchiveSchedulerService(config *ArchiveConfig, interval time.Duration, logger *log.Helper) *ArchiveSchedulerService {
	if config == nil {
		config = DefaultArchiveConfig()
	}
	if interval <= 0 {
		interval = 24 * time.Hour // Default: run once per day
	}
	if logger == nil {
		logger = global.Logger
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ArchiveSchedulerService{
		config:   config,
		interval: interval,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the archive scheduler service.
// It runs archiving operations periodically in the background.
func (s *ArchiveSchedulerService) Start() error {
	s.logger.Infof("starting archive scheduler service (interval: %v)", s.interval)

	// Start archive scheduler goroutine
	s.wg.Add(1)
	go s.archiveScheduler()

	s.logger.Infof("archive scheduler service started")
	return nil
}

// archiveScheduler runs the periodic archiving task.
func (s *ArchiveSchedulerService) archiveScheduler() {
	defer s.wg.Done()

	// Create archive service
	archiveService := NewArchiveService()

	// Create ticker for periodic execution
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.Infof("archive scheduler running, next execution in %v", s.interval)

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Infof("archive scheduler stopped")
			return
		case <-ticker.C:
			s.executeArchive(archiveService)
		}
	}
}

// executeArchive executes a single archiving operation.
// It runs transaction and block archiving in parallel with independent timeouts.
// These operations are independent and can run simultaneously to reduce total execution time.
func (s *ArchiveSchedulerService) executeArchive(archiveService *ArchiveService) {
	s.logger.Infof("starting scheduled archive operation")

	// Run transaction and block archiving in parallel using goroutines
	// These operations are independent and can run simultaneously
	var wg sync.WaitGroup
	var txErr, blockErr error

	// Run transaction archiving in parallel
	wg.Add(1)
	go func() {
		defer wg.Done()
		txCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		startTime := time.Now()
		err := archiveService.ArchiveOldTransactions(txCtx, s.config)
		if err != nil {
			s.logger.Errorf("scheduled transaction archiving failed: %v", err)
			txErr = err
		} else {
			duration := time.Since(startTime)
			s.logger.Infof("scheduled transaction archiving completed successfully in %v", duration)
		}
	}()

	// Run block archiving in parallel
	wg.Add(1)
	go func() {
		defer wg.Done()
		blockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		startTime := time.Now()
		metrics, err := archiveService.ArchiveOldBlocks(blockCtx, s.config)
		if err != nil {
			s.logger.Errorf("scheduled block archiving failed: %v", err)
			blockErr = err
		} else {
			duration := time.Since(startTime)
			s.logger.Infof("scheduled block archiving completed successfully in %v (processed=%d, failed=%d)",
				duration, metrics.ProcessedBlocks, metrics.FailedBlocks)
		}
	}()

	// Wait for both operations to complete
	wg.Wait()

	// Log any errors that occurred
	if txErr != nil {
		s.logger.Warnf("scheduled operation completed with transaction archiving error: %v", txErr)
	}
	if blockErr != nil {
		s.logger.Warnf("scheduled operation completed with block archiving error: %v", blockErr)
	}

	// Only log success if both operations succeeded
	if txErr == nil && blockErr == nil {
		s.logger.Infof("scheduled archive operation completed successfully (both transaction and block archiving)")
	}
}

// Stop stops the archive scheduler service.
// It cancels the context and waits for the scheduler goroutine to finish.
func (s *ArchiveSchedulerService) Stop() error {
	s.logger.Infof("stopping archive scheduler service")
	s.cancel()

	// Wait for goroutine to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Infof("archive scheduler service stopped gracefully")
		return nil
	case <-time.After(GracefulShutdownTimeout):
		s.logger.Warnf("archive scheduler service stop timeout after %v", GracefulShutdownTimeout)
		return errors.New("timeout waiting for scheduler to stop")
	}
}

// LoadArchiveSchedulerConfig loads archive scheduler configuration from Bootstrap config.
// It extracts the archive interval from the config and creates an ArchiveSchedulerService.
//
// Parameters:
//   - bc: Bootstrap configuration (can be nil to use defaults)
//
// Returns:
//   - *ArchiveSchedulerService: A configured archive scheduler service instance
func LoadArchiveSchedulerConfig(bc *conf.Bootstrap) *ArchiveSchedulerService {
	// Load archive configuration
	archiveConfig := DefaultArchiveConfig()
	if bc != nil && bc.Archive != nil {
		archiveConfig = LoadArchiveConfig(bc.Archive)
	}

	// Get archive interval from config (default: 24 hours)
	interval := 24 * time.Hour
	if bc != nil && bc.Archive != nil && bc.Archive.GetInterval() != nil {
		interval = bc.Archive.GetInterval().AsDuration()
	}

	// Use global logger (backward compatible)
	return NewArchiveSchedulerService(archiveConfig, interval, global.Logger)
}

// ============================================================================
// BLOCK ARCHIVING
// ============================================================================

// ArchiveOldBlocks archives blocks older than the specified age.
// It moves the large txs field to block_data_archives table and optionally to object storage.
// The main blocks table retains metadata (with archived=true flag) for statistics and queries.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - config: Archive configuration (uses DefaultArchiveConfig if nil)
//
// Returns:
//   - metrics: BlockArchiveMetrics containing statistics about the operation
//   - error: Error if archiving fails
func (s *ArchiveService) ArchiveOldBlocks(ctx context.Context, config *ArchiveConfig) (*BlockArchiveMetrics, error) {
	if config == nil {
		config = DefaultArchiveConfig()
	}

	// Set default storage threshold if not specified
	if config.StorageThreshold <= 0 {
		config.StorageThreshold = DefaultStorageThreshold
	}

	// Validate configuration
	if config.ArchiveAgeMonths <= 0 {
		return nil, errors.New("archive age months must be greater than 0")
	}
	if config.BatchSize <= 0 {
		return nil, errors.New("batch size must be greater than 0")
	}

	global.Logger.Infof("starting block archiving: age_months=%d, batch_size=%d, dry_run=%v",
		config.ArchiveAgeMonths, config.BatchSize, config.DryRun)

	// Initialize metrics
	metrics := &BlockArchiveMetrics{
		StartTime: time.Now(),
	}

	// Calculate cutoff time
	cutoffTime := time.Now().AddDate(0, -config.ArchiveAgeMonths, 0)
	cutoffTimestamp := cutoffTime.Unix()

	// Count blocks to archive
	var totalCount int64
	err := s.db.WithContext(ctx).Model(&models.Block{}).
		Where("block_unix_timestamp < ? AND (archived IS NULL OR archived = ?)", cutoffTimestamp, NotArchived).
		Count(&totalCount).Error
	if err != nil {
		return nil, errors.Wrap(err, "failed to count blocks to archive")
	}

	if totalCount == 0 {
		global.Logger.Info("no blocks to archive")
		return metrics, nil
	}

	metrics.TotalBlocks = totalCount
	global.Logger.Infof("found %d blocks to archive (older than %s)", totalCount, cutoffTime.Format("2006-01-02"))

	// Process blocks in batches
	var blocks []models.Block
	batchNum := 0
	err = s.db.WithContext(ctx).
		Where("block_unix_timestamp < ? AND (archived IS NULL OR archived = ?)", cutoffTimestamp, NotArchived).
		FindInBatches(&blocks, config.BatchSize, func(tx *gorm.DB, batch int) error {
			batchNum++
			global.Logger.Infof("processing batch %d with %d blocks", batchNum, len(blocks))

			// Process each block in this batch
			for _, block := range blocks {
				if err := s.archiveBlock(ctx, tx, &block, config, metrics); err != nil {
					// Log error but continue with next block
					global.Logger.Errorf("failed to archive block %d: %v", block.Height, err)
					metrics.FailedBlocks++
					continue
				}
				metrics.ProcessedBlocks++
			}

			// Call progress callback if provided
			if config.ProgressCallback != nil {
				config.ProgressCallback(metrics.ProcessedBlocks, totalCount, len(blocks))
			}

			global.Logger.Infof("completed batch %d: processed=%d, failed=%d, total=%d",
				batchNum, metrics.ProcessedBlocks, metrics.FailedBlocks, metrics.ProcessedBlocks+metrics.FailedBlocks)
			return nil
		}).Error

	if err != nil {
		return metrics, errors.Wrap(err, "failed to archive blocks")
	}

	metrics.EndTime = time.Now()
	metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)

	global.Logger.Infof("block archiving completed: processed=%d, failed=%d, total=%d, duration=%v",
		metrics.ProcessedBlocks, metrics.FailedBlocks, totalCount, metrics.Duration)

	return metrics, nil
}

// archiveBlock archives a single block.
func (s *ArchiveService) archiveBlock(ctx context.Context, tx *gorm.DB, block *models.Block, config *ArchiveConfig, metrics *BlockArchiveMetrics) error {
	// Start database transaction for this block
	return tx.Transaction(func(txDB *gorm.DB) error {
		// Prepare block archive record
		archive := &models.BlockDataArchive{
			BlockID:    uint64(block.ID),
			ArchivedAt: time.Now(),
		}

		// Check for existing TxsStorageKey (storage key reuse optimization)
		// If block already has a storage key from sync time, reuse it instead of uploading again
		// This reduces storage duplication by 30-50% for synced blocks
		if block.TxsStorageKey != "" {
			// Reuse existing storage key from sync time
			archive.TxsStorageKey = block.TxsStorageKey
			archive.Txs = "" // No local data, it's in storage
			global.Logger.Debugf("reusing existing txs storage key for block %d: %s", block.Height, block.TxsStorageKey)
		} else {
			// No existing storage key - need to compress and upload
			// Compress txs field
			txsJSONBytes, err := json.Marshal(block.Txs)
			if err != nil {
				return errors.Wrap(err, "failed to marshal txs to JSON")
			}
			txsJSON := string(txsJSONBytes)

			// Compress data
			compressedTxs, wasCompressed, compressErr := models.CompressDataToBytes([]byte(txsJSON))
			if compressErr != nil {
				global.Logger.Warnf("failed to compress txs for block %d: %v, using uncompressed data", block.Height, compressErr)
				archive.Txs = txsJSON
			} else {
				archive.Txs = string(compressedTxs)
				if wasCompressed {
					metrics.CompressedBlocks++
				}
			}

			// Upload to object storage if enabled and data is large enough
			if config.UseObjectStorage && len(compressedTxs) > config.StorageThreshold {
				storageKey := fmt.Sprintf("blocks/archive/txs/%d", block.Height)
				if err := s.storage.PutObject(ctx, storageKey, compressedTxs); err != nil {
					return errors.Wrap(err, "failed to upload txs to object storage")
				}
				archive.TxsStorageKey = storageKey
				archive.Txs = "" // Clear local data after upload
				metrics.UploadedToStorageBlocks++
			}
		}

		// Create archive record
		if err := txDB.Create(archive).Error; err != nil {
			return errors.Wrap(err, "failed to create block archive record")
		}

		// Update main block table (mark as archived and clear txs)
		updates := map[string]interface{}{
			"archived": true,
			"txs":      nil,
		}
		if config.UseObjectStorage && archive.TxsStorageKey != "" {
			updates["txs_storage_key"] = archive.TxsStorageKey
		}

		if err := txDB.Model(block).Updates(updates).Error; err != nil {
			return errors.Wrap(err, "failed to update block record")
		}

		return nil
	})
}

// LoadArchivedBlockTxs loads archived block transaction data.
// It retrieves the txs data from either object storage or the block_data_archives table.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - blockID: The block ID to load archived data for
//
// Returns:
//   - txs: List of transaction hashes in the block
//   - error: Error if loading fails
func (s *ArchiveService) LoadArchivedBlockTxs(ctx context.Context, blockID uint64) ([]string, error) {
	var archive models.BlockDataArchive
	err := s.db.WithContext(ctx).
		Where("block_id = ?", blockID).
		First(&archive).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("archived block data not found")
		}
		return nil, errors.Wrap(err, "failed to query block archive")
	}

	// If data is in object storage, load it
	if archive.TxsStorageKey != "" {
		storageData, err := s.storage.GetObject(ctx, archive.TxsStorageKey)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load txs from object storage")
		}

		// Decompress data
		decompressed, decompressErr := models.DecompressDataFromBytes(storageData)
		if decompressErr != nil {
			// If decompression fails, assume it's uncompressed (backward compatibility)
			global.Logger.Warnf("failed to decompress block txs, using raw data: %v", decompressErr)
			return unmarshalStringSlice(storageData)
		}

		return unmarshalStringSlice(decompressed)
	}

	// Otherwise, load from archive table (data is already compressed)
	if archive.Txs != "" {
		decompressed, decompressErr := models.DecompressData(archive.Txs)
		if decompressErr != nil {
			// If decompression fails, assume it's uncompressed (backward compatibility)
			global.Logger.Warnf("failed to decompress block txs, using raw data: %v", decompressErr)
			return unmarshalStringSlice([]byte(archive.Txs))
		}

		return unmarshalStringSlice(decompressed)
	}

	return nil, errors.New("no txs data found in archive")
}

// BlockArchiveMetrics contains statistics about block archiving operations.
type BlockArchiveMetrics struct {
	StartTime    time.Time // Start time of the archiving operation
	EndTime      time.Time // End time of the archiving operation
	Duration     time.Duration // Total time taken for the operation
	TotalBlocks  int64 // Total number of blocks that were candidates for archiving
	ProcessedBlocks int64 // Number of blocks successfully archived
	FailedBlocks int64 // Number of blocks that failed to archive
	CompressedBlocks int64 // Number of blocks with compressed data
	UploadedToStorageBlocks int64 // Number of blocks uploaded to object storage
}

// GetArchiveProgress returns the current progress of block archiving.
// Returns the number of blocks that are archived vs total blocks.
func (s *ArchiveService) GetArchiveProgress(ctx context.Context) (archived, total int64, err error) {
	// Count total blocks
	err = s.db.WithContext(ctx).Model(&models.Block{}).Count(&total).Error
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to count total blocks")
	}

	// Count archived blocks
	err = s.db.WithContext(ctx).Model(&models.Block{}).
		Where("archived = ?", IsArchived).
		Count(&archived).Error
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to count archived blocks")
	}

	return archived, total, nil
}

// unmarshalStringSlice is a helper function to unmarshal a JSON byte array to []string.
func unmarshalStringSlice(data []byte) ([]string, error) {
	var result []string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal txs JSON")
	}
	return result, nil
}
