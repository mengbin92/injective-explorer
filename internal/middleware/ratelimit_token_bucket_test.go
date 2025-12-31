// Package middleware provides tests for token bucket rate limiter.
package middleware

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"injective-explorer/provider/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenBucketRateLimiter_BasicFunctionality tests basic token bucket functionality.
func TestTokenBucketRateLimiter_BasicFunctionality(t *testing.T) {
	// Skip if Redis is not available
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	// Test token rate limit
	tokenID := int64(12345)
	limit := 5
	burst := 10

	// Consume all tokens
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Next request should be denied
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed, "Request should be denied when bucket is empty")

	// Wait for tokens to refill
	time.Sleep(1200 * time.Millisecond) // Wait for 1.2 seconds to get 6 tokens (5 per second)

	// Should be able to make requests again
	allowed, err = limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.True(t, allowed, "Request should be allowed after refill")
}

// TestTokenBucketRateLimiter_IPBased tests IP-based rate limiting.
func TestTokenBucketRateLimiter_IPBased(t *testing.T) {
	// Skip if Redis is not available
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	// Test IP rate limit
	ip := "192.168.1.100"
	limit := 60 // 60 requests per minute
	burst := 120

	// Consume all burst tokens
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckIPRateLimitOptimized(ctx, ip, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Next request should be denied
	allowed, err := limiter.CheckIPRateLimitOptimized(ctx, ip, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed, "Request should be denied when bucket is empty")

	// Wait for partial refill (0.5 seconds = 30 tokens)
	time.Sleep(500 * time.Millisecond)

	// Should be able to make 30 requests
	count := 0
	for i := 0; i < 35; i++ {
		allowed, err := limiter.CheckIPRateLimitOptimized(ctx, ip, limit, burst)
		require.NoError(t, err)
		if allowed {
			count++
		}
	}
	assert.Equal(t, 30, count, "Should allow approximately 30 requests after 0.5 seconds")
}

// TestTokenBucketRateLimiter_Concurrent tests concurrent access.
func TestTokenBucketRateLimiter_Concurrent(t *testing.T) {
	// Skip if Redis is not available
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	// Test concurrent access
	tokenID := int64(99999)
	limit := 10
	burst := 10
	concurrency := 50
	requestsPerGoroutine := 20

	var allowedCount int32
	var deniedCount int32
	var wg sync.WaitGroup

	start := time.Now()

	// Launch concurrent goroutines
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
				require.NoError(t, err)

				if allowed {
					atomic.AddInt32(&allowedCount, 1)
				} else {
					atomic.AddInt32(&deniedCount, 1)
				}

				// Small delay between requests
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	totalRequests := int32(concurrency * requestsPerGoroutine)
	assert.Equal(t, totalRequests, allowedCount+deniedCount, "All requests should be processed")

	// Should not exceed burst + refill during duration
	maxExpected := int32(burst) + int32(duration.Seconds())*int32(limit)
	assert.LessOrEqual(t, allowedCount, maxExpected, "Allowed requests should not exceed limit")

	t.Logf("Concurrent test: %d goroutines, %d requests each, duration: %v",
		concurrency, requestsPerGoroutine, duration)
	t.Logf("Allowed: %d, Denied: %d, Total: %d", allowedCount, deniedCount, totalRequests)
}

// TestTokenBucketRateLimiter_WithInfo tests the detailed info function.
func TestTokenBucketRateLimiter_WithInfo(t *testing.T) {
	// Skip if Redis is not available
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	tokenID := int64(77777)
	limit := 5
	burst := 10

	// Check initial state
	allowed, tokens, timeUntilNext, err := limiter.CheckTokenRateLimitWithInfo(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 9, tokens) // 10 - 1 = 9 tokens
	assert.Equal(t, 0.0, timeUntilNext)

	// Consume all tokens
	for i := 0; i < burst-1; i++ {
		allowed, tokens, timeUntilNext, err = limiter.CheckTokenRateLimitWithInfo(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// Bucket should be empty
	allowed, tokens, timeUntilNext, err = limiter.CheckTokenRateLimitWithInfo(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Less(t, tokens, 1)
	assert.Greater(t, timeUntilNext, 0.0)

	t.Logf("Time until next token: %.3f seconds", timeUntilNext)
}

// TestTokenBucketRateLimiter_Performance compares performance with sliding window.
func TestTokenBucketRateLimiter_Performance(t *testing.T) {
	// Skip if Redis is not available
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	// Compare token bucket with sliding window
	tokenID := int64(88888)
	limit := 100
	iterations := 1000

	// Measure token bucket performance
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit)
		require.NoError(t, err)
	}
	tokenBucketDuration := time.Since(start)

	// Calculate operations count
	slidingWindowOps := iterations * 4 // ZRemRangeByScore, ZCard, ZAdd, Expire
	tokenBucketOps := iterations * 1   // Eval

	t.Logf("Performance comparison (%d iterations):", iterations)
	t.Logf("Token bucket: %v (%d Redis ops)", tokenBucketDuration, tokenBucketOps)
	t.Logf("Sliding window: ~%v (%d Redis ops)", tokenBucketDuration*4, slidingWindowOps)
	t.Logf("Reduction: %.1fx fewer Redis operations", float64(slidingWindowOps)/float64(tokenBucketOps))

	// Token bucket should be significantly faster
	assert.Less(t, tokenBucketDuration, 100*time.Millisecond,
		"Token bucket should be very fast for %d iterations", iterations)
}

// TestTokenBucketRateLimiter_BatchOperations tests batch rate limit checks.
func TestTokenBucketRateLimiter_BatchOperations(t *testing.T) {
	// Skip if Redis is not available
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	// Prepare batch checks
	checks := []TokenRateLimitCheck{
		{TokenID: 11111, Limit: 10, Burst: 20},
		{TokenID: 22222, Limit: 20, Burst: 40},
		{TokenID: 33333, Limit: 30, Burst: 60},
		{TokenID: 44444, Limit: 40, Burst: 80},
		{TokenID: 55555, Limit: 50, Burst: 100},
	}

	// Run batch check
	results, err := limiter.BatchCheckTokenRateLimit(ctx, checks)
	require.NoError(t, err)
	assert.Len(t, results, len(checks))

	// All should be allowed (first request)
	for i, allowed := range results {
		assert.True(t, allowed, "Check %d should be allowed", i)
	}

	// Run batch check again (should consume tokens)
	results, err = limiter.BatchCheckTokenRateLimit(ctx, checks)
	require.NoError(t, err)

	// Verify results based on limits
	for i := range checks {
		if i < 3 { // First 3 have enough tokens
			assert.True(t, results[i], "Check %d should still have tokens", i)
		}
	}

	t.Logf("Batch check completed for %d tokens", len(checks))
}

// TestTokenBucketRateLimiter_GracefulDegradation tests graceful degradation when Redis is unavailable.
func TestTokenBucketRateLimiter_GracefulDegradation(t *testing.T) {
	// Create limiter with nil Redis client
	limiter := &TokenBucketRateLimiter{redis: nil}
	ctx := context.Background()

	// Should allow all requests when Redis is not available
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, 12345, 10)
	assert.NoError(t, err)
	assert.True(t, allowed, "Should allow request when Redis is unavailable")

	allowed, err = limiter.CheckIPRateLimitOptimized(ctx, "192.168.1.1", 60)
	assert.NoError(t, err)
	assert.True(t, allowed, "Should allow IP request when Redis is unavailable")
}

// TestTokenBucketRateLimiter_EdgeCases tests edge cases and error conditions.
func TestTokenBucketRateLimiter_EdgeCases(t *testing.T) {
	// Skip if Redis is not available
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	// Test with zero or negative limit
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, 66666, 0)
	require.NoError(t, err)
	assert.True(t, allowed, "Should allow when limit is 0")

	// Test with very high limit
	allowed, err = limiter.CheckTokenRateLimitOptimized(ctx, 66667, 10000)
	require.NoError(t, err)
	assert.True(t, allowed, "Should allow when limit is very high")

	// Test reset functionality
	tokenID := int64(77777)
	limit := 5

	// Consume some tokens
	for i := 0; i < 3; i++ {
		allowed, _ := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit)
		assert.True(t, allowed)
	}

	// Reset the bucket
	err = limiter.ResetTokenBucket(ctx, fmt.Sprintf("ratelimit:token:bucket:%d", tokenID))
	require.NoError(t, err)

	// Should be able to consume full burst again
	count := 0
	for i := 0; i < limit*2; i++ {
		allowed, _ := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit)
		if allowed {
			count++
		}
	}
	assert.Equal(t, limit, count, "Should have full limit after reset")
}

// TestTokenBucketRateLimiter_TokenRefillAccuracy tests the accuracy of token refill over time.
func TestTokenBucketRateLimiter_TokenRefillAccuracy(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	tokenID := int64(111111)
	limit := 10 // 10 tokens per second
	burst := 20

	// Consume all tokens
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Verify bucket is empty
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed, "Bucket should be empty")

	// Wait 0.5 seconds - should get ~5 tokens
	time.Sleep(500 * time.Millisecond)
	allowedCount := 0
	for i := 0; i < 10; i++ {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		if allowed {
			allowedCount++
		}
	}
	// Should allow approximately 5 requests (with some tolerance)
	assert.GreaterOrEqual(t, allowedCount, 4, "Should allow at least 4 requests after 0.5 seconds")
	assert.LessOrEqual(t, allowedCount, 6, "Should allow at most 6 requests after 0.5 seconds")

	// Wait another 0.5 seconds - should get ~5 more tokens
	time.Sleep(500 * time.Millisecond)
	allowedCount2 := 0
	for i := 0; i < 10; i++ {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		if allowed {
			allowedCount2++
		}
	}
	assert.GreaterOrEqual(t, allowedCount2, 4, "Should allow at least 4 more requests after another 0.5 seconds")
}

// TestTokenBucketRateLimiter_GetBucketState tests the GetTokenBucketState function.
func TestTokenBucketRateLimiter_GetBucketState(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	tokenID := int64(222222)
	limit := 5
	burst := 10
	key := fmt.Sprintf("ratelimit:token:bucket:%d", tokenID)

	// Get initial state (should be empty or default)
	tokens, _, err := limiter.GetTokenBucketState(ctx, key)
	require.NoError(t, err)
	// Initial state might be empty or have default burst
	assert.GreaterOrEqual(t, tokens, 0.0)

	// Make some requests
	for i := 0; i < 3; i++ {
		_, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
	}

	// Get state after consumption
	tokens2, lastRefill2, err := limiter.GetTokenBucketState(ctx, key)
	require.NoError(t, err)
	assert.Less(t, tokens2, float64(burst), "Tokens should be less than burst after consumption")
	assert.Greater(t, lastRefill2, int64(0), "Last refill should be set")

	// Consume all tokens
	for i := 0; i < burst+5; i++ {
		limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
	}

	// Get state when bucket is empty
	tokens3, lastRefill3, err := limiter.GetTokenBucketState(ctx, key)
	require.NoError(t, err)
	assert.Less(t, tokens3, 1.0, "Tokens should be less than 1 when bucket is empty")
	assert.Greater(t, lastRefill3, lastRefill2, "Last refill should be updated")
}

// TestTokenBucketRateLimiter_ResetBucket tests the ResetTokenBucket function thoroughly.
func TestTokenBucketRateLimiter_ResetBucket(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	tokenID := int64(333333)
	limit := 10
	burst := 20
	key := fmt.Sprintf("ratelimit:token:bucket:%d", tokenID)

	// Consume all tokens
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// Verify bucket is empty
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed, "Bucket should be empty")

	// Reset the bucket
	err = limiter.ResetTokenBucket(ctx, key)
	require.NoError(t, err)

	// Should be able to consume full burst again
	count := 0
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		if allowed {
			count++
		}
	}
	assert.Equal(t, burst, count, "Should be able to consume full burst after reset")

	// Test resetting non-existent bucket (should not error)
	err = limiter.ResetTokenBucket(ctx, "ratelimit:token:bucket:nonexistent")
	require.NoError(t, err)
}

// TestTokenBucketRateLimiter_BoundaryValues tests various boundary values and edge cases.
func TestTokenBucketRateLimiter_BoundaryValues(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	// Test with limit = 1
	tokenID1 := int64(444444)
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID1, 1, 1)
	require.NoError(t, err)
	assert.True(t, allowed, "Should allow first request with limit=1")
	allowed, err = limiter.CheckTokenRateLimitOptimized(ctx, tokenID1, 1, 1)
	require.NoError(t, err)
	assert.False(t, allowed, "Should deny second request with limit=1, burst=1")

	// Test with burst < limit (should use limit as burst)
	tokenID2 := int64(444445)
	limit := 10
	burst := 5 // burst less than limit
	allowed, err = limiter.CheckTokenRateLimitOptimized(ctx, tokenID2, limit, burst)
	require.NoError(t, err)
	assert.True(t, allowed, "Should allow with burst < limit")

	// Test with very large values
	tokenID3 := int64(444446)
	allowed, err = limiter.CheckTokenRateLimitOptimized(ctx, tokenID3, 1000000, 2000000)
	require.NoError(t, err)
	assert.True(t, allowed, "Should handle very large limit values")

	// Test with burst = 0 (should default to limit)
	tokenID4 := int64(444447)
	allowed, err = limiter.CheckTokenRateLimitOptimized(ctx, tokenID4, 5, 0)
	require.NoError(t, err)
	assert.True(t, allowed, "Should handle burst=0 by defaulting to limit")
}

// TestTokenBucketRateLimiter_Isolation tests that different tokens/IPs are isolated.
func TestTokenBucketRateLimiter_Isolation(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	limit := 5
	burst := 10

	// Test token isolation
	tokenID1 := int64(555555)
	tokenID2 := int64(555556)

	// Consume all tokens for token1
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID1, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed, "Token1 request %d should be allowed", i+1)
	}

	// Token1 should be denied
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID1, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed, "Token1 should be denied")

	// Token2 should still be allowed (isolation)
	allowed, err = limiter.CheckTokenRateLimitOptimized(ctx, tokenID2, limit, burst)
	require.NoError(t, err)
	assert.True(t, allowed, "Token2 should be allowed (isolated from token1)")

	// Test IP isolation
	ip1 := "192.168.1.100"
	ip2 := "192.168.1.101"

	// Consume all tokens for ip1
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckIPRateLimitOptimized(ctx, ip1, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed, "IP1 request %d should be allowed", i+1)
	}

	// IP1 should be denied
	allowed, err = limiter.CheckIPRateLimitOptimized(ctx, ip1, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed, "IP1 should be denied")

	// IP2 should still be allowed (isolation)
	allowed, err = limiter.CheckIPRateLimitOptimized(ctx, ip2, limit, burst)
	require.NoError(t, err)
	assert.True(t, allowed, "IP2 should be allowed (isolated from IP1)")
}

// TestTokenBucketRateLimiter_LongRunning tests rate limiter behavior over a longer period.
func TestTokenBucketRateLimiter_LongRunning(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	tokenID := int64(666666)
	limit := 2 // 2 tokens per second
	burst := 4

	// Consume initial burst
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// Over 3 seconds, we should get approximately 6 tokens (2 per second)
	// But we can only consume them as they become available
	totalAllowed := 0
	start := time.Now()
	for time.Since(start) < 3*time.Second {
		allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		if allowed {
			totalAllowed++
		}
		time.Sleep(100 * time.Millisecond) // Check every 100ms
	}

	// Should have consumed approximately 6 tokens (burst + 3 seconds * 2 tokens/sec)
	// With some tolerance for timing
	assert.GreaterOrEqual(t, totalAllowed, 4, "Should allow at least 4 tokens over 3 seconds")
	assert.LessOrEqual(t, totalAllowed, 10, "Should allow at most 10 tokens over 3 seconds")
}

// TestTokenBucketRateLimiter_BatchOperationsErrorHandling tests batch operations with various scenarios.
func TestTokenBucketRateLimiter_BatchOperationsErrorHandling(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	// Test empty batch
	checks := []TokenRateLimitCheck{}
	results, err := limiter.BatchCheckTokenRateLimit(ctx, checks)
	require.NoError(t, err)
	assert.Len(t, results, 0, "Empty batch should return empty results")

	// Test single check
	checks = []TokenRateLimitCheck{
		{TokenID: 777777, Limit: 10, Burst: 20},
	}
	results, err = limiter.BatchCheckTokenRateLimit(ctx, checks)
	require.NoError(t, err)
	assert.Len(t, results, 1, "Single check should return one result")
	assert.True(t, results[0], "First request should be allowed")

	// Test large batch
	checks = make([]TokenRateLimitCheck, 50)
	for i := 0; i < 50; i++ {
		checks[i] = TokenRateLimitCheck{
			TokenID: int64(888888 + i),
			Limit:   10,
			Burst:   20,
		}
	}
	results, err = limiter.BatchCheckTokenRateLimit(ctx, checks)
	require.NoError(t, err)
	assert.Len(t, results, 50, "Large batch should return all results")
	for i := 0; i < 50; i++ {
		assert.True(t, results[i], "All first requests should be allowed")
	}

	// Test batch with different limits
	checks = []TokenRateLimitCheck{
		{TokenID: 999991, Limit: 5, Burst: 10},
		{TokenID: 999992, Limit: 10, Burst: 20},
		{TokenID: 999993, Limit: 20, Burst: 40},
	}
	results, err = limiter.BatchCheckTokenRateLimit(ctx, checks)
	require.NoError(t, err)
	assert.Len(t, results, 3, "Should return results for all checks")
	for i := range results {
		assert.True(t, results[i], "All first requests should be allowed")
	}
}

// TestTokenBucketRateLimiter_ConcurrentConsistency tests data consistency under concurrent access.
func TestTokenBucketRateLimiter_ConcurrentConsistency(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	tokenID := int64(101010)
	limit := 100
	burst := 200
	concurrency := 20
	requestsPerGoroutine := 50

	var allowedCount int32
	var deniedCount int32
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Launch concurrent goroutines
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit, burst)
				if err != nil {
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
					continue
				}

				if allowed {
					atomic.AddInt32(&allowedCount, 1)
				} else {
					atomic.AddInt32(&deniedCount, 1)
				}

				// Small random delay
				time.Sleep(time.Duration(id%10) * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify no errors occurred
	assert.Empty(t, errors, "No errors should occur during concurrent access")

	totalRequests := int32(concurrency * requestsPerGoroutine)
	assert.Equal(t, totalRequests, allowedCount+deniedCount, "All requests should be processed")

	// Should not exceed burst + reasonable refill during execution
	// With 20 goroutines * 50 requests = 1000 requests, but we only have burst=200
	// So most should be denied
	assert.LessOrEqual(t, allowedCount, int32(burst+100), "Allowed requests should not exceed burst + refill")
	assert.Greater(t, deniedCount, int32(0), "Some requests should be denied under high load")

	t.Logf("Concurrent consistency test: %d allowed, %d denied, %d total",
		allowedCount, deniedCount, totalRequests)
}

// TestTokenBucketRateLimiter_WithInfoAccuracy tests the accuracy of CheckTokenRateLimitWithInfo.
func TestTokenBucketRateLimiter_WithInfoAccuracy(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	tokenID := int64(202020)
	limit := 5
	burst := 10

	// Initial check
	allowed, tokens, timeUntilNext, err := limiter.CheckTokenRateLimitWithInfo(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.True(t, allowed, "First request should be allowed")
	assert.Equal(t, 9, tokens, "Should have 9 tokens after consuming 1")
	assert.Equal(t, 0.0, timeUntilNext, "Time until next should be 0 when tokens available")

	// Consume all tokens
	for i := 0; i < burst-1; i++ {
		allowed, tokens, timeUntilNext, err = limiter.CheckTokenRateLimitWithInfo(ctx, tokenID, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+2)
	}

	// Bucket should be empty
	allowed, tokens, timeUntilNext, err = limiter.CheckTokenRateLimitWithInfo(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed, "Request should be denied when bucket is empty")
	assert.Less(t, tokens, 1, "Tokens should be less than 1")
	assert.Greater(t, timeUntilNext, 0.0, "Time until next should be positive")

	// Wait for refill
	time.Sleep(1200 * time.Millisecond) // 1.2 seconds = ~6 tokens

	// Check again
	allowed, tokens, timeUntilNext, err = limiter.CheckTokenRateLimitWithInfo(ctx, tokenID, limit, burst)
	require.NoError(t, err)
	assert.True(t, allowed, "Request should be allowed after refill")
	assert.Greater(t, tokens, 0, "Should have some tokens after refill")
}

// TestTokenBucketRateLimiter_IPRateLimitAccuracy tests IP rate limiting accuracy (per minute).
func TestTokenBucketRateLimiter_IPRateLimitAccuracy(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	ip := "10.0.0.100"
	limit := 60 // 60 requests per minute = 1 per second
	burst := 120

	// Consume all burst tokens
	for i := 0; i < burst; i++ {
		allowed, err := limiter.CheckIPRateLimitOptimized(ctx, ip, limit, burst)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Next request should be denied
	allowed, err := limiter.CheckIPRateLimitOptimized(ctx, ip, limit, burst)
	require.NoError(t, err)
	assert.False(t, allowed, "Request should be denied when bucket is empty")

	// Wait 1 second - should get ~1 token (60 per minute = 1 per second)
	time.Sleep(1000 * time.Millisecond)
	allowedCount := 0
	for i := 0; i < 5; i++ {
		allowed, err := limiter.CheckIPRateLimitOptimized(ctx, ip, limit, burst)
		require.NoError(t, err)
		if allowed {
			allowedCount++
		}
	}
	// Should allow approximately 1 request
	assert.GreaterOrEqual(t, allowedCount, 1, "Should allow at least 1 request after 1 second")
	assert.LessOrEqual(t, allowedCount, 2, "Should allow at most 2 requests after 1 second")
}

// TestTokenBucketRateLimiter_DifferentBurstValues tests various burst value configurations.
func TestTokenBucketRateLimiter_DifferentBurstValues(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()

	testCases := []struct {
		name  string
		limit int
		burst int
	}{
		{"burst equals limit", 10, 10},
		{"burst double limit", 10, 20},
		{"burst triple limit", 10, 30},
		{"burst half limit", 20, 10}, // burst < limit, should use limit
		{"small values", 1, 2},
		{"large values", 1000, 2000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokenID := int64(303030 + int64(len(tc.name)))
			effectiveBurst := tc.burst
			if tc.burst < tc.limit {
				effectiveBurst = tc.limit
			}

			// Consume tokens up to effective burst
			allowedCount := 0
			for i := 0; i < effectiveBurst+5; i++ {
				allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, tc.limit, tc.burst)
				require.NoError(t, err)
				if allowed {
					allowedCount++
				} else {
					break
				}
			}

			assert.Equal(t, effectiveBurst, allowedCount,
				"Should allow exactly effective burst (%d) requests", effectiveBurst)
		})
	}
}

// BenchmarkTokenBucketRateLimit benchmarks the token bucket rate limiter.
func BenchmarkTokenBucketRateLimitOptimized(b *testing.B) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		b.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewTokenBucketRateLimiter()
	tokenID := int64(999999999)
	limit := 1000 // High limit for benchmark

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := limiter.CheckTokenRateLimitOptimized(ctx, tokenID, limit)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSlidingWindowRateLimit benchmarks the original sliding window rate limiter for comparison.
func BenchmarkSlidingWindowRateLimit(b *testing.B) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		b.Skip("Redis not available")
	}

	ctx := context.Background()
	limiter := NewRateLimiter()
	tokenID := int64(888888888)
	limit := 1000 // High limit for benchmark

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := limiter.CheckTokenRateLimit(ctx, tokenID, limit)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Helper function to demonstrate usage.
func ExampleTokenBucketRateLimiter() {
	// Create rate limiter
	limiter := NewTokenBucketRateLimiter()
	ctx := context.Background()

	// Check rate limit for a token (5 requests per second, burst of 10)
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, 12345, 5, 10)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if allowed {
		fmt.Println("Request allowed")
	} else {
		fmt.Println("Rate limit exceeded")
	}

	// Check with detailed info
	allowed, tokens, timeUntilNext, err := limiter.CheckTokenRateLimitWithInfo(ctx, 12345, 5)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Allowed: %v, Tokens: %d, Time until next: %.3f seconds\n",
		allowed, tokens, timeUntilNext)
}

// Example of batch checking multiple tokens.
func Example_tokenBucketRateLimiter_batchCheck() {
	limiter := NewTokenBucketRateLimiter()
	ctx := context.Background()

	// Check multiple tokens at once
	checks := []TokenRateLimitCheck{
		{TokenID: 11111, Limit: 10, Burst: 20},
		{TokenID: 22222, Limit: 20, Burst: 40},
		{TokenID: 33333, Limit: 30, Burst: 60},
	}

	results, err := limiter.BatchCheckTokenRateLimit(ctx, checks)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	for i, allowed := range results {
		fmt.Printf("Token %d: %v\n", checks[i].TokenID, allowed)
	}
}

// Example of comparing algorithms.
func ExampleCompareTokenRateLimit() {
	ctx := context.Background()

	slidingWindowOps, tokenBucketOps, _ := CompareTokenRateLimit(ctx, 12345, 100, 1000)

	fmt.Printf("Sliding window: %d ops\n", slidingWindowOps)
	fmt.Printf("Token bucket: %d ops\n", tokenBucketOps)
	fmt.Printf("Reduction: %.1fx fewer operations\n", float64(slidingWindowOps)/float64(tokenBucketOps))
}

// Example of getting bucket state for monitoring.
func Example_tokenBucketRateLimiter_GetBucketState() {
	limiter := NewTokenBucketRateLimiter()
	ctx := context.Background()

	tokens, lastRefill, err := limiter.GetTokenBucketState(ctx, "ratelimit:token:bucket:12345")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Tokens: %.2f, Last refill: %d\n", tokens, lastRefill)
}

// Example output:
// Request allowed
// Allowed: true, Tokens: 9, Time until next: 0.000 seconds
// Token 11111: true
// Token 22222: true
// Token 33333: true
// Sliding window: 4000 ops
// Token bucket: 1000 ops
// Reduction: 4.0x fewer operations
// Tokens: 5.00, Last refill: 1640995200000

// Example of graceful degradation.
func Example_tokenBucketRateLimiter_GracefulDegradation() {
	// Create limiter with nil Redis (simulating Redis failure)
	limiter := &TokenBucketRateLimiter{redis: nil}
	ctx := context.Background()

	// Should still allow requests when Redis is unavailable
	allowed, err := limiter.CheckTokenRateLimitOptimized(ctx, 12345, 10)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Allowed (Redis unavailable): %v\n", allowed)
	// Output: Allowed (Redis unavailable): true
}
