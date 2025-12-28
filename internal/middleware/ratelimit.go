// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"
	"fmt"
	"time"

	"injective-explorer/provider/cache"

	"github.com/redis/go-redis/v9"
)

// RateLimiter implements rate limiting using Redis sorted sets for sliding window.
type RateLimiter struct {
	redis *redis.Client
}

// NewRateLimiter creates a new rate limiter instance.
//
// Returns:
//   - *RateLimiter: A new rate limiter instance
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		redis: cache.GetRedisClient(),
	}
}

// CheckIPRateLimit checks IP rate limit using a sliding window approach.
// If Redis is not available, it allows the request (graceful degradation).
//
// Parameters:
//   - ctx: Context for the operation
//   - ip: The client IP address
//   - limit: Maximum requests per minute
//
// Returns:
//   - bool: true if request is allowed, false if rate limit exceeded
//   - error: Error if the check fails
func (r *RateLimiter) CheckIPRateLimit(ctx context.Context, ip string, limit int) (bool, error) {
	if r.redis == nil {
		// If Redis is not available, allow the request (graceful degradation)
		return true, nil
	}

	key := fmt.Sprintf("ratelimit:ip:%s", ip)
	now := time.Now()
	windowStart := now.Add(-1 * time.Minute)

	// Remove expired entries
	r.redis.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.Unix()))

	// Count current requests
	count, err := r.redis.ZCard(ctx, key).Result()
	if err != nil {
		return false, err
	}

	if count >= int64(limit) {
		return false, nil // Rate limit exceeded
	}

	// Add current request
	r.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.Unix()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})
	r.redis.Expire(ctx, key, 2*time.Minute) // Auto cleanup

	return true, nil
}

// CheckTokenRateLimit checks token rate limit using a sliding window approach.
// If Redis is not available, it allows the request (graceful degradation).
//
// Parameters:
//   - ctx: Context for the operation
//   - tokenID: The token ID
//   - limit: Maximum requests per second
//
// Returns:
//   - bool: true if request is allowed, false if rate limit exceeded
//   - error: Error if the check fails
func (r *RateLimiter) CheckTokenRateLimit(ctx context.Context, tokenID int64, limit int) (bool, error) {
	if r.redis == nil {
		// If Redis is not available, allow the request (graceful degradation)
		return true, nil
	}

	key := fmt.Sprintf("ratelimit:token:%d", tokenID)
	now := time.Now()
	windowStart := now.Add(-1 * time.Second)

	// Remove expired entries
	r.redis.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.Unix()))

	// Count current requests
	count, err := r.redis.ZCard(ctx, key).Result()
	if err != nil {
		return false, err
	}

	if count >= int64(limit) {
		return false, nil // Rate limit exceeded
	}

	// Add current request
	r.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.Unix()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})
	r.redis.Expire(ctx, key, 2*time.Second) // Auto cleanup

	return true, nil
}

// CheckUserMonthlyLimit checks user monthly usage limit.
// If Redis is not available, it allows the request (graceful degradation).
//
// Parameters:
//   - ctx: Context for the operation
//   - userID: The user ID
//   - limit: Maximum requests per month
//
// Returns:
//   - bool: true if request is allowed, false if limit exceeded
//   - uint64: Current usage count
//   - error: Error if the check fails
func (r *RateLimiter) CheckUserMonthlyLimit(ctx context.Context, userID string, limit uint64) (bool, uint64, error) {
	if r.redis == nil {
		// If Redis is not available, allow the request (graceful degradation)
		return true, 0, nil
	}

	key := fmt.Sprintf("usage:user:%s:monthly", userID)

	// Get current usage from Redis
	current, err := r.redis.Get(ctx, key).Uint64()
	if err == redis.Nil {
		current = 0
	} else if err != nil {
		return false, 0, err
	}

	if current >= limit {
		return false, current, nil // Limit exceeded
	}

	return true, current, nil
}

// IncrementUserUsage increments the user usage counter and sets expiration to end of month.
// If Redis is not available, it skips increment (graceful degradation).
//
// Parameters:
//   - ctx: Context for the operation
//   - userID: The user ID
//
// Returns:
//   - error: Error if increment fails
func (r *RateLimiter) IncrementUserUsage(ctx context.Context, userID string) error {
	if r.redis == nil {
		// If Redis is not available, skip increment (graceful degradation)
		return nil
	}

	key := fmt.Sprintf("usage:user:%s:monthly", userID)

	// Increment counter
	_, err := r.redis.Incr(ctx, key).Result()
	if err != nil {
		return err
	}

	// Set expiration to end of month
	now := time.Now()
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	ttl := nextMonth.Sub(now)
	r.redis.Expire(ctx, key, ttl)

	return nil
}

// ResetMonthlyQuota resets monthly quota for all users at the start of a new month.
// This should be called as a scheduled task (e.g., at the start of each month).
// It clears Redis counters and updates database records.
//
// Parameters:
//   - ctx: Context for the operation
//
// Returns:
//   - error: Error if reset fails
func (r *RateLimiter) ResetMonthlyQuota(ctx context.Context) error {
	if r.redis == nil {
		// If Redis is not available, skip reset (graceful degradation)
		return nil
	}

	// Find all monthly usage keys
	pattern := "usage:user:*:monthly"
	iter := r.redis.Scan(ctx, 0, pattern, 0).Iterator()

	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}

	// Delete all monthly usage keys (they will be recreated on next increment)
	if len(keys) > 0 {
		return r.redis.Del(ctx, keys...).Err()
	}

	return nil
}

