// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"encoding/json"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// UsageStatsService handles usage statistics for API requests.
type UsageStatsService struct {
	redis *redis.Client
	db    *gorm.DB
}

// NewUsageStatsService creates a new usage stats service instance.
//
// Returns:
//   - *UsageStatsService: A new service instance
func NewUsageStatsService() *UsageStatsService {
	return &UsageStatsService{
		redis: cache.GetRedisClient(),
		db:    db.Get(),
	}
}

// RecordRequest records an API request asynchronously by pushing to Redis queue.
// This is non-blocking and allows the API request to complete quickly.
//
// Parameters:
//   - ctx: Context for the operation
//   - log: The API request log to record
//
// Returns:
//   - error: Error if recording fails
func (s *UsageStatsService) RecordRequest(ctx context.Context, log *models.APIRequestLog) error {
	if s.redis == nil {
		// If Redis is not available, write directly to database (synchronous fallback)
		return s.db.WithContext(ctx).Create(log).Error
	}

	// Serialize log to JSON
	data, err := json.Marshal(log)
	if err != nil {
		global.Logger.Errorf("failed to marshal request log: %v", err)
		return err
	}

	// Push to Redis queue (non-blocking)
	queueKey := "queue:api_logs"
	return s.redis.LPush(ctx, queueKey, data).Err()
}

// ProcessLogQueue processes logs from Redis queue and writes them to database in batches.
// This should be run as a background goroutine.
//
// Parameters:
//   - ctx: Context for the operation (should be cancellable)
//
// Returns:
//   - error: Error if processing fails
func (s *UsageStatsService) ProcessLogQueue(ctx context.Context) error {
	if s.redis == nil {
		// If Redis is not available, nothing to process
		return nil
	}

	queueKey := "queue:api_logs"
	batchSize := 100
	batch := make([]*models.APIRequestLog, 0, batchSize)

	for {
		select {
		case <-ctx.Done():
			// Flush remaining batch before exit
			if len(batch) > 0 {
				if err := s.flushBatch(ctx, batch); err != nil {
					global.Logger.Errorf("failed to flush final batch: %v", err)
				}
			}
			return ctx.Err()
		default:
			// Blocking pop from queue (timeout: 5 seconds)
			result, err := s.redis.BRPop(ctx, 5*time.Second, queueKey).Result()
			if err == redis.Nil {
				// Timeout, flush batch if any
				if len(batch) > 0 {
					if err := s.flushBatch(ctx, batch); err != nil {
						global.Logger.Errorf("failed to flush batch: %v", err)
					}
					batch = batch[:0]
				}
				continue
			}
			if err != nil {
				global.Logger.Errorf("failed to pop from queue: %v", err)
				time.Sleep(1 * time.Second) // Retry after delay
				continue
			}

			// Deserialize log
			var log models.APIRequestLog
			if err := json.Unmarshal([]byte(result[1]), &log); err != nil {
				global.Logger.Errorf("failed to unmarshal log: %v", err)
				continue // Skip invalid log
			}

			batch = append(batch, &log)

			// Flush when batch is full
			if len(batch) >= batchSize {
				if err := s.flushBatch(ctx, batch); err != nil {
					global.Logger.Errorf("failed to flush batch: %v", err)
				}
				batch = batch[:0]
			}
		}
	}
}

// flushBatch flushes a batch of logs to database.
//
// Parameters:
//   - ctx: Context for the operation
//   - logs: The batch of logs to flush
//
// Returns:
//   - error: Error if flush fails
func (s *UsageStatsService) flushBatch(ctx context.Context, logs []*models.APIRequestLog) error {
	if len(logs) == 0 {
		return nil
	}

	// Batch insert
	return s.db.WithContext(ctx).CreateInBatches(logs, 100).Error
}

// StartLogProcessor starts the log queue processor in a background goroutine.
// This should be called during application startup.
//
// Parameters:
//   - ctx: Context for the operation (should be cancellable)
func (s *UsageStatsService) StartLogProcessor(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := s.ProcessLogQueue(ctx); err != nil {
					global.Logger.Errorf("log queue processing error: %v", err)
					time.Sleep(5 * time.Second) // Retry after delay
				}
			}
		}
	}()
}

