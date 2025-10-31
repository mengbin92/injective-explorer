// Package cache provides Redis cache client initialization and management.
package cache

import (
	"injective-explorer/internal/conf"
	"context"
	"sync"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
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
		redisc = redis.NewClient(&redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			DB:           int(cfg.Db),
			PoolSize:     int(cfg.PoolSize),
			ReadTimeout:  cfg.ReadTimeout.AsDuration(),
			WriteTimeout: cfg.WriteTimeout.AsDuration(),
		})

		// Test connection
		_, err := redisc.Ping(ctx).Result()
		if err != nil {
			initErr = errors.Wrap(err, "redis ping error")
			redisc = nil // Reset client on error
			return
		}
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
