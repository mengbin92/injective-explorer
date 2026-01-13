// Package service provides cache helper functions for API response caching.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"injective-explorer/provider/cache"

	"github.com/redis/go-redis/v9"
)

// GetCached retrieves cached data from Redis.
// Returns (true, nil) if cache hit, (false, nil) if cache miss, (false, error) on error.
//
// Parameters:
//   - ctx: Context for the operation
//   - key: Cache key
//   - dest: Pointer to destination struct to unmarshal into
//   - ttl: Cache TTL (used for logging, actual TTL is set when caching)
//
// Returns:
//   - bool: true if cache hit, false if cache miss
//   - error: Error if cache lookup failed (not including cache miss)
func GetCached(ctx context.Context, key string, dest interface{}, ttl time.Duration) (bool, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return false, nil // Redis unavailable, skip cache
	}

	data, err := redisClient.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil // Cache miss
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("failed to unmarshal cached data: %w", err)
	}

	return true, nil // Cache hit
}

// SetCached sets cached data in Redis.
// If Redis is unavailable, it silently skips caching (graceful degradation).
//
// Parameters:
//   - ctx: Context for the operation
//   - key: Cache key
//   - value: Value to cache (will be marshaled to JSON)
//   - ttl: Cache TTL
//
// Returns:
//   - error: Error if caching failed (nil if Redis unavailable - graceful degradation)
func SetCached(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil // Redis unavailable, skip cache
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal data for caching: %w", err)
	}

	return redisClient.Set(ctx, key, data, ttl).Err()
}

// GetCachedAPIResponse retrieves cached API response from Redis.
// This is a convenience wrapper for GetCached with API-specific key prefix.
//
// Parameters:
//   - ctx: Context for the operation
//   - endpoint: API endpoint name (e.g., "GetBlock", "GetTransaction")
//   - params: Parameters used to build cache key (e.g., block height, transaction hash)
//   - dest: Pointer to destination struct to unmarshal into
//   - ttl: Cache TTL
//
// Returns:
//   - bool: true if cache hit, false if cache miss
//   - error: Error if cache lookup failed
func GetCachedAPIResponse(ctx context.Context, endpoint string, params string, dest interface{}, ttl time.Duration) (bool, error) {
	cacheKey := fmt.Sprintf("api:response:%s:%s", endpoint, params)
	return GetCached(ctx, cacheKey, dest, ttl)
}

// SetCachedAPIResponse sets cached API response in Redis.
// This is a convenience wrapper for SetCached with API-specific key prefix.
//
// Parameters:
//   - ctx: Context for the operation
//   - endpoint: API endpoint name (e.g., "GetBlock", "GetTransaction")
//   - params: Parameters used to build cache key (e.g., block height, transaction hash)
//   - value: Value to cache
//   - ttl: Cache TTL
//
// Returns:
//   - error: Error if caching failed
func SetCachedAPIResponse(ctx context.Context, endpoint string, params string, value interface{}, ttl time.Duration) error {
	cacheKey := fmt.Sprintf("api:response:%s:%s", endpoint, params)
	return SetCached(ctx, cacheKey, value, ttl)
}
