// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/provider/cache"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// Scheduler manages background scheduled tasks for API management.
// It handles daily stats updates, monthly quota resets, and log archiving.
type Scheduler struct {
	statsService    *UsageStatsService
	archiveService  *ArchiveUsageService
	redis           *redis.Client
	logger          *log.Helper
}

// NewScheduler creates a new scheduler instance.
//
// Parameters:
//   - statsService: The usage stats service for daily stats updates
//   - archiveService: The archive service for archiving old logs
//
// Returns:
//   - *Scheduler: A new scheduler instance
func NewScheduler(statsService *UsageStatsService, archiveService *ArchiveUsageService) *Scheduler {
	return &Scheduler{
		statsService:   statsService,
		archiveService: archiveService,
		redis:          cache.GetRedisClient(),
		logger:         global.Logger,
	}
}

// Start starts all background scheduled tasks.
// This should be called during application startup.
//
// Parameters:
//   - ctx: Context for the operation (should be cancellable)
func (s *Scheduler) Start(ctx context.Context) {
	// Start daily stats updater (runs at 1 AM daily)
	go s.scheduleDailyStats(ctx)

	// Start monthly quota reset (runs at the start of each month)
	go s.scheduleMonthlyQuotaReset(ctx)

	// Start archive task (runs daily at 2 AM)
	go s.scheduleArchive(ctx)
}

// scheduleDailyStats schedules daily stats update task.
// It runs at 1 AM every day to aggregate statistics from the previous day.
func (s *Scheduler) scheduleDailyStats(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour) // Check every hour
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			// Run at 1 AM
			if now.Hour() == 1 && now.Minute() < 5 {
				yesterday := now.AddDate(0, 0, -1)
				s.logger.Infof("Starting daily stats update for %s", yesterday.Format("2006-01-02"))
				if err := s.statsService.UpdateDailyStats(ctx, yesterday); err != nil {
					s.logger.Errorf("Daily stats update error: %v", err)
				} else {
					s.logger.Infof("Daily stats update completed for %s", yesterday.Format("2006-01-02"))
				}
				// Sleep for 5 minutes to avoid running multiple times
				time.Sleep(5 * time.Minute)
			}
		}
	}
}

// scheduleMonthlyQuotaReset schedules monthly quota reset task.
// It runs at the start of each month (1st day, 00:00-00:05) to reset user monthly quotas.
func (s *Scheduler) scheduleMonthlyQuotaReset(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour) // Check every hour
	defer ticker.Stop()

	var lastResetMonth time.Month = 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			// Run at the start of each month (1st day, 00:00-00:05)
			if now.Day() == 1 && now.Hour() == 0 && now.Minute() < 5 && now.Month() != lastResetMonth {
				s.logger.Infof("Starting monthly quota reset for %s", now.Format("2006-01"))
				if err := s.resetMonthlyQuota(ctx); err != nil {
					s.logger.Errorf("Monthly quota reset error: %v", err)
				} else {
					s.logger.Infof("Monthly quota reset completed for %s", now.Format("2006-01"))
					lastResetMonth = now.Month()
				}
				// Sleep for 5 minutes to avoid running multiple times
				time.Sleep(5 * time.Minute)
			}
		}
	}
}

// resetMonthlyQuota resets monthly quota for all users at the start of a new month.
// It clears Redis counters for monthly usage.
func (s *Scheduler) resetMonthlyQuota(ctx context.Context) error {
	if s.redis == nil {
		// If Redis is not available, skip reset (graceful degradation)
		return nil
	}

	// Find all monthly usage keys
	pattern := "usage:user:*:monthly"
	iter := s.redis.Scan(ctx, 0, pattern, 0).Iterator()

	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}

	// Delete all monthly usage keys (they will be recreated on next increment)
	if len(keys) > 0 {
		return s.redis.Del(ctx, keys...).Err()
	}

	return nil
}

// scheduleArchive schedules archive task.
// It runs daily at 2 AM to archive old API request logs (90+ days old).
func (s *Scheduler) scheduleArchive(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour) // Check every hour
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			// Run at 2 AM
			if now.Hour() == 2 && now.Minute() < 5 {
				s.logger.Infof("Starting API log archive task")
				config := DefaultArchiveUsageConfig()
				if err := s.archiveService.ArchiveOldLogs(ctx, config); err != nil {
					s.logger.Errorf("API log archive error: %v", err)
				} else {
					s.logger.Infof("API log archive completed")
				}
				// Sleep for 5 minutes to avoid running multiple times
				time.Sleep(5 * time.Minute)
			}
		}
	}
}
