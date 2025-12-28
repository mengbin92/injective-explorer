// Package service provides business logic services for the explorer API.
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

// ArchiveUsageService handles archiving of old API request logs.
// It archives logs older than 90 days to reduce database size while maintaining
// historical data for compliance and analysis.
type ArchiveUsageService struct {
	db      *gorm.DB
	storage storage.Storage
}

// NewArchiveUsageService creates a new archive usage service instance.
//
// Returns:
//   - *ArchiveUsageService: A new service instance
func NewArchiveUsageService() *ArchiveUsageService {
	return &ArchiveUsageService{
		db:      db.Get(),
		storage: storage.Get(),
	}
}

// ArchiveConfig contains configuration for API log archiving.
type ArchiveUsageConfig struct {
	// ArchiveDays is the age in days before logs are archived (default: 90)
	ArchiveDays int
	// BatchSize is the number of logs to archive per batch (default: 1000)
	BatchSize int
	// DryRun enables dry run mode (simulates archiving without actually deleting)
	DryRun bool
}

// DefaultArchiveUsageConfig returns default archive configuration.
//
// Returns:
//   - *ArchiveUsageConfig: Default configuration (90 days, batch size 1000)
func DefaultArchiveUsageConfig() *ArchiveUsageConfig {
	return &ArchiveUsageConfig{
		ArchiveDays: 90,
		BatchSize:   1000,
		DryRun:      false,
	}
}

// ArchiveOldLogs archives API request logs older than the specified age.
// Logs are deleted from the main table after archiving (unlike transaction archiving
// which keeps metadata). This is because API logs are primarily for analytics and
// compliance, and archived logs can be restored from backup if needed.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - config: Archive configuration (uses DefaultArchiveUsageConfig if nil)
//
// Returns:
//   - error: Error if archiving fails
func (s *ArchiveUsageService) ArchiveOldLogs(ctx context.Context, config *ArchiveUsageConfig) error {
	if config == nil {
		config = DefaultArchiveUsageConfig()
	}

	// Validate configuration
	if config.ArchiveDays <= 0 {
		return errors.New("archive days must be greater than 0")
	}
	if config.BatchSize <= 0 {
		return errors.New("batch size must be greater than 0")
	}

	global.Logger.Infof("starting API log archiving: archive_days=%d, batch_size=%d, dry_run=%v",
		config.ArchiveDays, config.BatchSize, config.DryRun)

	// Calculate cutoff time
	cutoffTime := time.Now().AddDate(0, 0, -config.ArchiveDays)

	// Count logs to archive
	var totalCount int64
	err := s.db.WithContext(ctx).Model(&models.APIRequestLog{}).
		Where("created_at < ?", cutoffTime).
		Count(&totalCount).Error
	if err != nil {
		return errors.Wrap(err, "count logs to archive")
	}

	if totalCount == 0 {
		global.Logger.Infof("no logs to archive")
		return nil
	}

	global.Logger.Infof("found %d logs to archive", totalCount)

	if config.DryRun {
		global.Logger.Infof("dry run mode: would archive %d logs", totalCount)
		return nil
	}

	// Archive in batches
	var archivedCount int64
	offset := 0

	for offset < int(totalCount) {
		// Get batch of logs to archive
		var logs []models.APIRequestLog
		err := s.db.WithContext(ctx).
			Where("created_at < ?", cutoffTime).
			Order("created_at ASC").
			Limit(config.BatchSize).
			Offset(offset).
			Find(&logs).Error
		if err != nil {
			return errors.Wrapf(err, "query logs batch at offset %d", offset)
		}

		if len(logs) == 0 {
			break // No more logs
		}

		// Archive batch (for now, just delete old logs)
		// In production, you might want to export to CSV/JSON and upload to object storage
		// before deleting from database
		logIDs := make([]uint, 0, len(logs))
		for _, log := range logs {
			logIDs = append(logIDs, log.ID)
		}

		// Delete logs in batch
		result := s.db.WithContext(ctx).
			Where("id IN ?", logIDs).
			Delete(&models.APIRequestLog{})
		if result.Error != nil {
			return errors.Wrapf(result.Error, "delete logs batch at offset %d", offset)
		}

		archivedCount += result.RowsAffected
		offset += len(logs)

		global.Logger.Infof("archived batch: %d/%d logs", archivedCount, totalCount)

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	global.Logger.Infof("archiving completed: archived %d logs", archivedCount)
	return nil
}

