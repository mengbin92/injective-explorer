// Package cache provides Redis cache client initialization and management.
package cache

import (
	"injective-explorer/internal/conf"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

const (
	// Redis connection pool defaults
	redisDefaultPoolSize = 10 // Default connection pool size

	// Redis timeout defaults
	redisDefaultDialTimeout  = 5 * time.Second // Default dial timeout
	redisDefaultPoolTimeout  = 5 * time.Second // Default pool timeout
	redisDefaultHealthCheckInterval = 30 * time.Second // Default health check interval

	// Redis retry defaults
	redisDefaultMaxRetries = 3 // Default maximum retry attempts

	// Cache warming defaults
	redisDefaultWarmTTL = 1 * time.Hour // Default TTL for warmed cache keys
)

var (
	// redisc is the global Redis client instance
	redisc *redis.Client
	// initRedisOnce ensures Redis is initialized only once (thread-safe)
	initRedisOnce sync.Once
)

// InitRedis initializes the Redis client connection.
// It uses sync.Once to ensure the client is initialized only once, even if called multiple times.
//
// Parameters:
//   - ctx: Context for the initialization operation (used for ping)
//   - cfg: Redis configuration containing address, password, database, and pool settings
//   - logger: Logger instance for logging initialization messages
//
// Returns:
//   - error: Error if initialization or connection test fails
func InitRedis(ctx context.Context, cfg *conf.Data_Redis, logger log.Logger) error {
	if cfg == nil {
		return errors.New("redis config cannot be nil")
	}

	var initErr error
	initRedisOnce.Do(func() {
		// Build Redis options with enhanced configuration
		options := &redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			DB:           int(cfg.Db),
			ReadTimeout:  cfg.ReadTimeout.AsDuration(),
			WriteTimeout: cfg.WriteTimeout.AsDuration(),
			// Disable maint_notifications to avoid errors on open-source Redis
			MaintNotificationsConfig: &maintnotifications.Config{
				Mode: maintnotifications.ModeDisabled,
			},
		}

		// Set pool size with default value if not configured
		if cfg.PoolSize > 0 {
			options.PoolSize = int(cfg.PoolSize)
		} else {
			options.PoolSize = redisDefaultPoolSize
		}

		// Set advanced connection pool settings
		if cfg.MinIdleConns > 0 {
			options.MinIdleConns = int(cfg.MinIdleConns)
		}
		if cfg.ConnMaxLifetime != nil {
			options.ConnMaxLifetime = cfg.ConnMaxLifetime.AsDuration()
		}
		if cfg.ConnMaxIdleTime != nil {
			options.ConnMaxIdleTime = cfg.ConnMaxIdleTime.AsDuration()
		}
		if cfg.DialTimeout != nil {
			options.DialTimeout = cfg.DialTimeout.AsDuration()
		} else {
			options.DialTimeout = redisDefaultDialTimeout
		}
		if cfg.PoolTimeout != nil {
			options.PoolTimeout = cfg.PoolTimeout.AsDuration()
		} else {
			options.PoolTimeout = redisDefaultPoolTimeout
		}

		// Set retry policy
		if cfg.MaxRetries > 0 {
			options.MaxRetries = int(cfg.MaxRetries)
		} else {
			options.MaxRetries = redisDefaultMaxRetries
		}
		if cfg.MinRetryBackoff != nil {
			options.MinRetryBackoff = cfg.MinRetryBackoff.AsDuration()
		}
		if cfg.MaxRetryBackoff != nil {
			options.MaxRetryBackoff = cfg.MaxRetryBackoff.AsDuration()
		}

		redisc = redis.NewClient(options)

		// Test connection
		_, err := redisc.Ping(ctx).Result()
		if err != nil {
			initErr = errors.Wrap(err, "redis ping error")
			redisc = nil // Reset client on error
			return
		}

		// Setup health check if enabled
		if cfg.EnableMetrics {
			setupHealthCheck(ctx, cfg, logger)
		}

		// Perform cache warming if enabled
		if cfg.EnableWarming && len(cfg.WarmKeys) > 0 {
			if err := warmCache(ctx, cfg, logger); err != nil {
				logger.Log(log.LevelWarn, "msg", "cache warming failed (non-critical)", "error", err)
			}
		}

		// Add metrics hook if enabled
		if cfg.EnableMetrics {
			AddMetricsHook(redisc)
		}

		logger.Log(log.LevelInfo, "msg", "Redis client initialized", "pool_size", options.PoolSize)
	})

	if initErr != nil {
		return initErr
	}

	if redisc == nil {
		return errors.New("redis client is nil after initialization")
	}

	return nil
}

// GetRedisClient returns the global Redis client instance.
//
// Returns:
//   - *redis.Client: The Redis client instance, or nil if not initialized
//
// Note: The client should be initialized using InitRedis before calling this function.
func GetRedisClient() *redis.Client {
	return redisc
}

// setupHealthCheck sets up periodic health checking for Redis connection.
func setupHealthCheck(ctx context.Context, cfg *conf.Data_Redis, logger log.Logger) {
	interval := redisDefaultHealthCheckInterval
	if cfg.HealthCheckInterval != nil {
		interval = cfg.HealthCheckInterval.AsDuration()
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if redisc != nil {
					_, err := redisc.Ping(ctx).Result()
					if err != nil {
						logger.Log(log.LevelError, "msg", "Redis health check failed", "error", err)
						// Consider implementing alerting or automatic recovery here
					}
				}
			}
		}
	}()

	logger.Log(log.LevelInfo, "msg", "Redis health check started", "interval", interval)
}

// warmCache performs cache warming by preloading specified keys.
func warmCache(ctx context.Context, cfg *conf.Data_Redis, logger log.Logger) error {
	if redisc == nil {
		return errors.New("redis client not initialized")
	}

	logger.Log(log.LevelInfo, "msg", "Starting cache warming")
	startTime := time.Now()

	// Get default TTL for warmed keys
	ttl := redisDefaultWarmTTL
	if cfg.WarmTtl != nil {
		ttl = cfg.WarmTtl.AsDuration()
	}

	// For each key to warm, check if it exists and optionally refresh it
	// Note: Actual warming logic would depend on your application requirements
	// This is a placeholder that demonstrates the pattern
	for _, key := range cfg.WarmKeys {
		// Check if key already exists
		exists, err := redisc.Exists(ctx, key).Result()
		if err != nil {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("Failed to check key %s during warming", key), "error", err)
			continue
		}

		if exists == 0 {
			// Key doesn't exist - in a real implementation, you might fetch
			// the data from the database or API and cache it here
			logger.Log(log.LevelDebug, "msg", fmt.Sprintf("Key %s does not exist, skipped warming (no data source defined)", key))
		} else {
			// Key exists - refresh its TTL
			if err := redisc.Expire(ctx, key, ttl).Err(); err != nil {
				logger.Log(log.LevelWarn, "msg", fmt.Sprintf("Failed to refresh TTL for key %s", key), "error", err)
			} else {
				logger.Log(log.LevelDebug, "msg", fmt.Sprintf("Refreshed TTL for key %s", key))
			}
		}
	}

	elapsed := time.Since(startTime)
	logger.Log(log.LevelInfo, "msg", "Cache warming completed", "elapsed", elapsed, "keys_refreshed", len(cfg.WarmKeys))

	return nil
}

// GetPoolStats returns the connection pool statistics.
// This is useful for monitoring and debugging connection pool issues.
func GetPoolStats() *redis.PoolStats {
	if redisc == nil {
		return nil
	}
	return redisc.PoolStats()
}

// GetHealthStatus returns the current Redis health status.
func GetHealthStatus(ctx context.Context) error {
	if redisc == nil {
		return errors.New("redis client not initialized")
	}
	_, err := redisc.Ping(ctx).Result()
	return err
}
