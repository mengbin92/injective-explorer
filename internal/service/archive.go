// Package service provides business logic services for the blockchain explorer.
// This file implements transaction archiving functionality.
package service

import (
	"context"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// ArchiveService handles transaction archiving operations.
// It moves old transactions to separate archive tables to keep the main transactions table lean and fast.
// Archived transactions can still be retrieved via GetArchivedTransaction.
type ArchiveService struct {
	db *gorm.DB
}

// NewArchiveService creates a new ArchiveService instance.
func NewArchiveService() *ArchiveService {
	return &ArchiveService{
		db: db.Get(),
	}
}

// ArchiveConfig contains configuration for archiving operations.
type ArchiveConfig struct {
	// ArchiveAgeMonths is the age in months before transactions are archived (default: 6)
	ArchiveAgeMonths int
	// BatchSize is the number of transactions to archive per batch (default: 1000)
	// Can be adjusted dynamically during operation
	BatchSize int
	// DryRun if true, only simulates archiving without actually moving data
	DryRun bool
	// ProgressCallback is an optional callback function called after each batch
	// Parameters: processed, total, current batch size
	// This allows external monitoring and dynamic batch size adjustment
	ProgressCallback func(processed, total int64, batchSize int)
}

// DefaultArchiveConfig returns default archive configuration.
func DefaultArchiveConfig() *ArchiveConfig {
	return &ArchiveConfig{
		ArchiveAgeMonths: 6,
		BatchSize:        1000,
		DryRun:           false,
	}
}

// ArchiveOldTransactions archives transactions older than the specified age.
// It moves large fields (Data, Messages, Logs) to archive tables and keeps
// only essential fields in the main transactions table.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - config: Archive configuration (uses DefaultArchiveConfig if nil)
//
// Returns:
//   - error: Error if archiving fails
func (s *ArchiveService) ArchiveOldTransactions(ctx context.Context, config *ArchiveConfig) error {
	if config == nil {
		config = DefaultArchiveConfig()
	}

	// Validate configuration
	if config.ArchiveAgeMonths <= 0 {
		return errors.New("archive age months must be greater than 0")
	}
	if config.BatchSize <= 0 {
		return errors.New("batch size must be greater than 0")
	}
	if config.BatchSize > 10000 {
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
		batchArchived, err := s.archiveBatch(ctx, transactions, cutoffTimestamp)
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
//
// Returns:
//   - int64: Number of successfully archived transactions
//   - error: Error if archiving fails
func (s *ArchiveService) archiveBatch(ctx context.Context, transactions []models.Transaction, archivedAt int64) (int64, error) {
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

	defer func() {
		if hasError {
			if rbErr := tx.Rollback().Error; rbErr != nil {
				global.Logger.Errorf("failed to rollback transaction: %v", rbErr)
			}
		}
	}()

	for _, txn := range transactions {
		// Create archive record
		archive := &models.TransactionArchive{
			Id:                 txn.Id,
			BlockNumber:        txn.BlockNumber,
			BlockTimestamp:     txn.BlockTimestamp,
			Hash:               txn.Hash,
			Code:               txn.Code,
			GasWanted:          txn.GasWanted,
			GasUsed:            txn.GasUsed,
			GasFee:             txn.GasFee,
			Codespace:          txn.Codespace,
			Events:             txn.Events,
			TxType:             txn.TxType,
			Signatures:         txn.Signatures,
			Memo:               txn.Memo,
			TxNumber:           txn.TxNumber,
			BlockUnixTimestamp: txn.BlockUnixTimestamp,
			ClaimIds:           txn.ClaimIds,
			Signer:             txn.Signer,
			ArchivedAt:         archivedAt,
		}

		// Insert archive record
		if err := tx.Create(archive).Error; err != nil {
			hasError = true
			return 0, errors.Wrapf(err, "create archive record for transaction %s", txn.Hash)
		}

		// Create archive data record
		archiveData := &models.TransactionDataArchive{
			TransactionID: archive.ID,
			Data:          txn.Data,
			Info:          txn.Info,
			Messages:      txn.Messages,
			Logs:          txn.Logs,
			ErrorLog:      txn.ErrorLog,
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

	return archivedCount, nil
}

// GetArchivedTransaction retrieves a transaction from archive tables.
// It combines data from TransactionArchive and TransactionDataArchive.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - hash: Transaction hash to retrieve
//
// Returns:
//   - *models.Transaction: Archived transaction, or nil if not found
//   - error: Error if retrieval fails
func (s *ArchiveService) GetArchivedTransaction(ctx context.Context, hash string) (*models.Transaction, error) {
	var archive models.TransactionArchive
	err := s.db.WithContext(ctx).Where("hash = ?", hash).First(&archive).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "get archived transaction %s", hash)
	}

	var archiveData models.TransactionDataArchive
	err = s.db.WithContext(ctx).Where("transaction_id = ?", archive.ID).First(&archiveData).Error
	if err != nil {
		return nil, errors.Wrapf(err, "get archived transaction data %s", hash)
	}

	// Reconstruct Transaction from archive
	txn := &models.Transaction{
		Model:              archive.Model,
		Id:                 archive.Id,
		BlockNumber:        archive.BlockNumber,
		BlockTimestamp:     archive.BlockTimestamp,
		Hash:               archive.Hash,
		Code:               archive.Code,
		Data:               archiveData.Data,
		Info:               archiveData.Info,
		GasWanted:          archive.GasWanted,
		GasUsed:            archive.GasUsed,
		GasFee:             archive.GasFee,
		Codespace:          archive.Codespace,
		Events:             archive.Events,
		TxType:             archive.TxType,
		Messages:           archiveData.Messages,
		Signatures:         archive.Signatures,
		Memo:               archive.Memo,
		TxNumber:           archive.TxNumber,
		BlockUnixTimestamp: archive.BlockUnixTimestamp,
		ErrorLog:           archiveData.ErrorLog,
		Logs:               archiveData.Logs,
		ClaimIds:           archive.ClaimIds,
		Signer:             archive.Signer,
		Archived:           true,
	}

	return txn, nil
}

// GetArchivedTransactions retrieves archived transactions with pagination.
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
	var archives []models.TransactionArchive
	var total int64

	// Count total
	err := s.db.WithContext(ctx).Model(&models.TransactionArchive{}).Count(&total).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "count archived transactions")
	}

	// Fetch archives
	err = s.db.WithContext(ctx).
		Order("block_unix_timestamp DESC, tx_number DESC").
		Limit(limit).
		Offset(offset).
		Find(&archives).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "fetch archived transactions")
	}

	// Fetch archive data for all transactions in one query to avoid N+1 problem
	if len(archives) == 0 {
		return []models.Transaction{}, total, nil
	}

	// Collect all transaction IDs
	archiveIDs := make([]uint, 0, len(archives))
	archiveMap := make(map[uint]*models.TransactionArchive, len(archives))
	for i := range archives {
		archiveIDs = append(archiveIDs, archives[i].ID)
		archiveMap[archives[i].ID] = &archives[i]
	}

	// Fetch all archive data in one query
	var archiveDataList []models.TransactionDataArchive
	err = s.db.WithContext(ctx).Where("transaction_id IN ?", archiveIDs).Find(&archiveDataList).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "fetch archive data")
	}

	// Build map of transaction ID to archive data
	dataMap := make(map[uint]*models.TransactionDataArchive, len(archiveDataList))
	for i := range archiveDataList {
		dataMap[archiveDataList[i].TransactionID] = &archiveDataList[i]
	}

	// Reconstruct transactions
	transactions := make([]models.Transaction, 0, len(archives))
	for _, archive := range archives {
		archiveData, exists := dataMap[archive.ID]
		if !exists {
			global.Logger.Warnf("archive data not found for transaction %s (id=%d)", archive.Hash, archive.ID)
			continue
		}

		txn := models.Transaction{
			Model:              archive.Model,
			Id:                 archive.Id,
			BlockNumber:        archive.BlockNumber,
			BlockTimestamp:     archive.BlockTimestamp,
			Hash:               archive.Hash,
			Code:               archive.Code,
			Data:               archiveData.Data,
			Info:               archiveData.Info,
			GasWanted:          archive.GasWanted,
			GasUsed:            archive.GasUsed,
			GasFee:             archive.GasFee,
			Codespace:          archive.Codespace,
			Events:             archive.Events,
			TxType:             archive.TxType,
			Messages:           archiveData.Messages,
			Signatures:         archive.Signatures,
			Memo:               archive.Memo,
			TxNumber:           archive.TxNumber,
			BlockUnixTimestamp: archive.BlockUnixTimestamp,
			ErrorLog:           archiveData.ErrorLog,
			Logs:               archiveData.Logs,
			ClaimIds:           archive.ClaimIds,
			Signer:             archive.Signer,
			Archived:           true,
		}
		transactions = append(transactions, txn)
	}

	return transactions, total, nil
}
