// Package cache provides tiered cache implementation (local memory + Redis).
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"
)

// TieredCache implements a two-tier cache system:
// 1. Local memory cache (ristretto) for ultra-fast access
// 2. Redis cache for distributed caching
//
// This reduces Redis network latency from 0.5-1ms to 0.01ms for cache hits.
type TieredCache struct {
	local *ristretto.Cache[string, []byte]
	redis *redis.Client
}

// NewTieredCache creates a new tiered cache instance.
//
// Parameters:
//   - redisClient: Redis client instance (can be nil for local-only cache)
//
// Returns:
//   - *TieredCache: A new tiered cache instance
//   - error: Error if local cache initialization fails
func NewTieredCache(redisClient *redis.Client) (*TieredCache, error) {
	local, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: 1e7,     // Number of counters (10 million)
		MaxCost:     1 << 30, // 1GB max cost
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create local cache: %w", err)
	}

	return &TieredCache{
		local: local,
		redis: redisClient,
	}, nil
}

// Get retrieves a value from cache.
// First checks local cache, then Redis if local cache misses.
// If found in Redis, also stores it in local cache for future access.
//
// Parameters:
//   - ctx: Context for the operation
//   - key: Cache key
//   - dest: Pointer to destination struct to unmarshal into
//
// Returns:
//   - bool: true if cache hit, false if cache miss
//   - error: Error if cache lookup failed (not including cache miss)
func (tc *TieredCache) Get(ctx context.Context, key string, dest interface{}) (bool, error) {
	// 1. Check local cache
	val, found := tc.local.Get(key)
	if found {
		// Local cache hit, return directly
		if err := json.Unmarshal(val, dest); err != nil {
			return false, fmt.Errorf("failed to unmarshal cached data: %w", err)
		}
		return true, nil
	}

	// 2. Check Redis (if available)
	if tc.redis == nil {
		return false, nil // Redis unavailable, return cache miss
	}

	data, err := tc.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil // Cache miss
	}
	if err != nil {
		return false, fmt.Errorf("redis get error: %w", err)
	}

	// 3. Store in local cache
	tc.local.Set(key, data, 1) // cost=1 for all items
	tc.local.Wait()            // Wait for cache to be set

	// 4. Unmarshal and return
	if err := json.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("failed to unmarshal cached data: %w", err)
	}

	return true, nil
}

// Set stores a value in both local and Redis cache.
//
// Parameters:
//   - ctx: Context for the operation
//   - key: Cache key
//   - value: Value to cache (will be marshaled to JSON)
//   - cost: Cost for local cache (used for eviction policy)
//   - ttl: Cache TTL
//
// Returns:
//   - error: Error if caching failed
func (tc *TieredCache) Set(ctx context.Context, key string, value interface{}, cost int64, ttl time.Duration) error {
	// Marshal value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal data for caching: %w", err)
	}

	// 1. Store in local cache
	tc.local.Set(key, data, cost)
	tc.local.Wait() // Wait for cache to be set

	// 2. Store in Redis (if available)
	if tc.redis != nil {
		if err := tc.redis.Set(ctx, key, data, ttl).Err(); err != nil {
			// Log error but don't fail - local cache is still set
			return fmt.Errorf("redis set error: %w", err)
		}
	}

	return nil
}

// Delete removes a key from both local and Redis cache.
//
// Parameters:
//   - ctx: Context for the operation
//   - key: Cache key to delete
//
// Returns:
//   - error: Error if deletion failed
func (tc *TieredCache) Delete(ctx context.Context, key string) error {
	// 1. Delete from local cache
	tc.local.Del(key)

	// 2. Delete from Redis cache (if available)
	if tc.redis != nil {
		if err := tc.redis.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("redis delete error: %w", err)
		}
	}

	return nil
}

// Close closes the tiered cache and releases resources.
func (tc *TieredCache) Close() {
	if tc.local != nil {
		tc.local.Close()
	}
}
