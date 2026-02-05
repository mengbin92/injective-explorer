// Package utils provides cache utility functions used across the application.
package utils

import (
	"context"
	"fmt"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/provider/cache"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

// CacheManager provides a unified interface for Redis cache operations.
type CacheManager struct {
	prefix string
	defaultTTL time.Duration
}

// NewCacheManager creates a new CacheManager instance.
func NewCacheManager(prefix string, defaultTTL time.Duration) *CacheManager {
	return &CacheManager{
		prefix:     prefix,
		defaultTTL: defaultTTL,
	}
}

// Get retrieves data from cache.
func (c *CacheManager) Get(ctx context.Context, key string, dest interface{}) error {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return errors.New("redis client is nil")
	}

	fullKey := c.buildKey(key)
	data, err := redisClient.Get(ctx, fullKey).Bytes()
	if err != nil {
		return err
	}

	return sonic.Unmarshal(data, dest)
}

// Set stores data in cache.
func (c *CacheManager) Set(ctx context.Context, key string, value interface{}, ttl ...time.Duration) error {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil // Graceful degradation
	}

	data, err := sonic.Marshal(value)
	if err != nil {
		return errors.Wrap(err, "failed to marshal cache data")
	}

	duration := c.defaultTTL
	if len(ttl) > 0 {
		duration = ttl[0]
	}

	fullKey := c.buildKey(key)
	return redisClient.Set(ctx, fullKey, data, duration).Err()
}

// Delete removes data from cache.
func (c *CacheManager) Delete(ctx context.Context, key string) error {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil
	}

	fullKey := c.buildKey(key)
	err := redisClient.Del(ctx, fullKey).Err()
	if err != nil && err != redis.Nil {
		return err
	}
	return nil
}

// DeletePattern removes all keys matching a pattern.
func (c *CacheManager) DeletePattern(ctx context.Context, pattern string) error {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil
	}

	fullPattern := c.buildKey(pattern)
	keys, err := redisClient.Keys(ctx, fullPattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return redisClient.Del(ctx, keys...).Err()
	}
	return nil
}

// Exists checks if a key exists in cache.
func (c *CacheManager) Exists(ctx context.Context, key string) (bool, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return false, errors.New("redis client is nil")
	}

	fullKey := c.buildKey(key)
	n, err := redisClient.Exists(ctx, fullKey).Result()
	return n > 0, err
}

// GetOrSet implements cache-aside pattern: try to get from cache, if miss, compute and set.
func GetOrSet[T any](
	ctx context.Context,
	key string,
	dest *T,
	ttl time.Duration,
	compute func() (T, error),
) (bool, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		// Redis unavailable, compute directly
		value, err := compute()
		if err != nil {
			return false, err
		}
		*dest = value
		return false, nil
	}

	// Try to get from cache
	data, err := redisClient.Get(ctx, key).Bytes()
	if err == nil {
		// Cache hit
		if err := sonic.Unmarshal(data, dest); err != nil {
			return false, errors.Wrap(err, "failed to unmarshal cached data")
		}
		return true, nil
	}

	if err != redis.Nil {
		// Redis error, compute directly
		global.Logger.Warnf("redis get error for key %s: %v, computing value", key, err)
	}

	// Cache miss or error, compute value
	value, err := compute()
	if err != nil {
		return false, err
	}

	*dest = value

	// Store in cache asynchronously
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		data, err := sonic.Marshal(value)
		if err != nil {
			global.Logger.Warnf("failed to marshal value for cache key %s: %v", key, err)
			return
		}

		if err := redisClient.Set(cacheCtx, key, data, ttl).Err(); err != nil {
			global.Logger.Warnf("failed to set cache for key %s: %v", key, err)
		}
	}()

	return false, nil
}

// buildKey builds the full cache key with prefix.
func (c *CacheManager) buildKey(key string) string {
	if c.prefix == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", c.prefix, key)
}

// CacheKeyBuilder helps build cache keys with consistent format.
type CacheKeyBuilder struct {
	prefix string
}

// NewCacheKeyBuilder creates a new CacheKeyBuilder.
func NewCacheKeyBuilder(prefix string) *CacheKeyBuilder {
	return &CacheKeyBuilder{prefix: prefix}
}

// Build builds a cache key from parts.
func (b *CacheKeyBuilder) Build(parts ...string) string {
	key := b.prefix
	for _, part := range parts {
		key = fmt.Sprintf("%s:%s", key, part)
	}
	return key
}

// BuildWithID builds a cache key with an ID.
func (b *CacheKeyBuilder) BuildWithID(id interface{}) string {
	return fmt.Sprintf("%s:%v", b.prefix, id)
}

// BuildWithTwoIDs builds a cache key with two IDs.
func (b *CacheKeyBuilder) BuildWithTwoIDs(id1, id2 interface{}) string {
	return fmt.Sprintf("%s:%v:%v", b.prefix, id1, id2)
}
