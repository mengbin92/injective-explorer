// Package service provides business logic services for the blockchain explorer.
// This file implements transaction archiving functionality.
package service

import (
	"context"
	"sync"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

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
}

// NewArchiveService creates a new ArchiveService instance.
// The service uses the global database and storage instances.
// Archive configuration is automatically loaded from global config when using DefaultArchiveConfig().
func NewArchiveService() *ArchiveService {
	return &ArchiveService{
		db:      db.Get(),
		storage: storage.Get(),
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
		Where("created_at < ? AND (archived IS NULL OR archived = ?)", cutoffTime, false).
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

	// Archive in batches
	var archivedCount int64
	var offset int64
	batchStartTime := time.Now()

	for offset < totalCount {
		// Get batch of transactions to archive
		var transactions []models.Transaction
		err := s.db.WithContext(ctx).
			Where("created_at < ? AND (archived IS NULL OR archived = ?)", cutoffTime, false).
			Order("created_at ASC").
			Limit(config.BatchSize).
			Offset(int(offset)).
			Find(&transactions).Error
		if err != nil {
			return errors.Wrapf(err, "fetch batch at offset %d", offset)
		}

		if len(transactions) == 0 {
			break
		}

		// Archive batch
		batchArchived, err := s.archiveBatch(ctx, transactions, cutoffTimestamp, config)
		if err != nil {
			return errors.Wrapf(err, "archive batch at offset %d", offset)
		}

		archivedCount += batchArchived
		offset += int64(len(transactions))
		metrics.BatchesProcessed++
		batchDuration := time.Since(batchStartTime)
		global.Logger.Infof("archived batch: %d/%d transactions, batch_time=%v", archivedCount, totalCount, batchDuration)

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

// GetArchivedTransaction retrieves a transaction from the main table (archived=true) and TransactionDataArchive.
// It combines metadata from the main transactions table with detailed data from TransactionDataArchive.
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
	err := s.db.WithContext(ctx).Where("hash = ? AND archived = ?", hash, true).First(&tx).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "get archived transaction %s", hash)
	}

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

	return &tx, nil
}

// GetArchivedTransactions retrieves archived transactions with pagination.
// It queries the main transactions table (archived=true) and combines with TransactionDataArchive.
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
		Where("archived = ?", true).
		Count(&total).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "count archived transactions")
	}

	// Fetch archived transactions from main table
	err = s.db.WithContext(ctx).
		Where("archived = ?", true).
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

	// Collect all transaction IDs
	transactionIDs := make([]uint, 0, len(transactions))
	transactionMap := make(map[uint]*models.Transaction, len(transactions))
	for i := range transactions {
		transactionIDs = append(transactionIDs, transactions[i].ID)
		transactionMap[transactions[i].ID] = &transactions[i]
	}

	// Fetch all archive data in one query to avoid N+1 problem
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

	// Reconstruct transactions with archive data
	result := make([]models.Transaction, 0, len(transactions))
	for i := range transactions {
		tx := &transactions[i]
		archiveData, exists := dataMap[tx.ID]
		if !exists {
			global.Logger.Warnf("archive data not found for transaction %s (id=%d), skipping", tx.Hash, tx.ID)
			continue
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
			} else {
				global.Logger.Warnf("failed to load archive data from storage for tx %s: %v, using database data", tx.Hash, err)
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
			} else {
				global.Logger.Warnf("failed to load archive logs from storage for tx %s: %v, using database data", tx.Hash, err)
				if archiveData.Logs != "" {
					logs = archiveData.Logs
				}
			}
		}

		// Update transaction with archive data
		tx.Data = data
		tx.Messages = messages
		tx.Logs = logs
		tx.Info = archiveData.Info
		tx.ErrorLog = archiveData.ErrorLog
		tx.Archived = true

		result = append(result, *tx)
	}

	return result, total, nil
}

// ArchiveSchedulerService handles periodic archiving of old transactions.
// It runs in the background and periodically executes archiving operations.
// Similar to SyncService, it provides Start() and Stop() methods for lifecycle management.
type ArchiveSchedulerService struct {
	// Configuration
	config *ArchiveConfig

	// Archive execution interval
	interval time.Duration

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
//
// Returns:
//   - *ArchiveSchedulerService: A new archive scheduler service instance
func NewArchiveSchedulerService(config *ArchiveConfig, interval time.Duration) *ArchiveSchedulerService {
	if config == nil {
		config = DefaultArchiveConfig()
	}
	if interval <= 0 {
		interval = 24 * time.Hour // Default: run once per day
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ArchiveSchedulerService{
		config:   config,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the archive scheduler service.
// It runs archiving operations periodically in the background.
func (s *ArchiveSchedulerService) Start() error {
	global.Logger.Infof("starting archive scheduler service (interval: %v)", s.interval)

	// Start archive scheduler goroutine
	s.wg.Add(1)
	go s.archiveScheduler()

	global.Logger.Infof("archive scheduler service started")
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

	global.Logger.Infof("archive scheduler running, next execution in %v", s.interval)

	for {
		select {
		case <-s.ctx.Done():
			global.Logger.Infof("archive scheduler stopped")
			return
		case <-ticker.C:
			s.executeArchive(archiveService)
		}
	}
}

// executeArchive executes a single archiving operation.
func (s *ArchiveSchedulerService) executeArchive(archiveService *ArchiveService) {
	global.Logger.Infof("starting scheduled archive operation")

	// Create context with timeout (1 hour should be enough for most cases)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	startTime := time.Now()
	err := archiveService.ArchiveOldTransactions(ctx, s.config)
	if err != nil {
		global.Logger.Errorf("scheduled archive operation failed: %v", err)
		return
	}

	duration := time.Since(startTime)
	global.Logger.Infof("scheduled archive operation completed successfully in %v", duration)
}

// Stop stops the archive scheduler service.
// It cancels the context and waits for the scheduler goroutine to finish.
func (s *ArchiveSchedulerService) Stop() error {
	global.Logger.Infof("stopping archive scheduler service")
	s.cancel()

	// Wait for goroutine to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		global.Logger.Infof("archive scheduler service stopped gracefully")
		return nil
	case <-time.After(GracefulShutdownTimeout):
		global.Logger.Warnf("archive scheduler service stop timeout after %v", GracefulShutdownTimeout)
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

	return NewArchiveSchedulerService(archiveConfig, interval)
}
