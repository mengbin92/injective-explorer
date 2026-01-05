// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"sync"
	"time"

	"injective-explorer/internal/global"
)

// inflightRequest represents a request that is currently being processed.
// Multiple concurrent requests with the same key will wait for this request to complete.
type inflightRequest struct {
	// waiters is a channel that will be closed when the request completes.
	waiters chan struct{}
	// result stores the result of the request.
	result interface{}
	// err stores any error that occurred during the request.
	err error
	// createdAt is the time when this request was created.
	createdAt time.Time
}

// RequestCoalescer implements request coalescing to prevent duplicate processing
// of identical requests. When multiple requests arrive for the same data simultaneously,
// only one request is executed and others wait for its result.
//
// This is particularly useful for:
// - Preventing cache stampede (thundering herd problem)
// - Reducing duplicate database queries
// - Improving performance under high concurrency
type RequestCoalescer struct {
	mu        sync.RWMutex
	inflight  map[string]*inflightRequest
	cleanupMu sync.Mutex
	// maxInflightAge is the maximum age of an inflight request before cleanup.
	maxInflightAge time.Duration
	// cleanupInterval is how often to run cleanup.
	cleanupInterval time.Duration
	// maxInflightSize is the maximum number of concurrent inflight requests.
	// When exceeded, new requests will execute directly without coalescing (graceful degradation).
	maxInflightSize int
	// stopCleanup is used to stop the cleanup goroutine.
	stopCleanup chan struct{}
	// totalRequests is the total number of requests processed.
	totalRequests int64
	// coalescedRequests is the number of requests that were coalesced (waited for existing request).
	coalescedRequests int64
	// directRequests is the number of requests executed directly (not coalesced).
	directRequests int64
	// cleanupCount is the number of cleanup operations performed.
	cleanupCount int64
	// maxMemoryUsage is the maximum estimated memory usage in bytes.
	maxMemoryUsage int64
}

// NewRequestCoalescer creates a new request coalescer instance.
//
// Parameters:
//   - maxInflightAge: Maximum age of an inflight request before cleanup (default: 30s).
//   - cleanupInterval: How often to run cleanup (default: 30s).
//   - maxInflightSize: Maximum number of concurrent inflight requests (default: 1000).
//     When exceeded, new requests will execute directly without coalescing (graceful degradation).
//
// Returns:
//   - *RequestCoalescer: A new request coalescer instance.
func NewRequestCoalescer(maxInflightAge, cleanupInterval time.Duration, maxInflightSize int) *RequestCoalescer {
	if maxInflightAge <= 0 {
		maxInflightAge = 30 * time.Second
	}
	if cleanupInterval <= 0 {
		cleanupInterval = 30 * time.Second
	}
	if maxInflightSize <= 0 {
		maxInflightSize = 1000
	}

	rc := &RequestCoalescer{
		inflight:        make(map[string]*inflightRequest),
		maxInflightAge:  maxInflightAge,
		cleanupInterval: cleanupInterval,
		maxInflightSize: maxInflightSize,
		stopCleanup:     make(chan struct{}),
	}

	// Start cleanup goroutine.
	go rc.cleanupLoop()

	return rc
}

// Do executes a function with request coalescing. If multiple requests with the same key
// arrive simultaneously, only one will execute the function and others will wait for its result.
//
// Parameters:
//   - ctx: Context for cancellation and timeout.
//   - key: Unique key identifying the request (e.g., "tx:hash:0x123...").
//   - fn: Function to execute if this is the first request with this key.
//
// Returns:
//   - interface{}: Result from the function (same for all requests with the same key).
//   - error: Error from the function (same for all requests with the same key).
//
// Example:
//
//	coalescer := NewRequestCoalescer(30*time.Second, 30*time.Second, 1000)
//	result, err := coalescer.Do(ctx, "tx:hash:0x123", func() (interface{}, error) {
//	    return db.GetTransaction("0x123")
//	})
func (rc *RequestCoalescer) Do(ctx context.Context, key string, fn func() (interface{}, error)) (interface{}, error) {
	// Check if there's already an inflight request for this key.
	rc.mu.RLock()
	req, exists := rc.inflight[key]
	rc.mu.RUnlock()

	if exists {
		// Increment coalesced requests counter.
		rc.mu.Lock()
		rc.coalescedRequests++
		rc.totalRequests++
		rc.mu.Unlock()

		// Wait for the existing request to complete.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-req.waiters:
			return req.result, req.err
		}
	}

	// No existing request, check if we can add a new one.
	rc.mu.Lock()
	if len(rc.inflight) >= rc.maxInflightSize {
		// Increment direct requests counter (executed without coalescing).
		rc.directRequests++
		rc.totalRequests++
		rc.mu.Unlock()
		// Map is full, execute directly without coalescing (graceful degradation).
		if global.Logger != nil {
			global.Logger.Warnf("RequestCoalescer: maxInflightSize (%d) reached, executing request directly without coalescing for key: %s", rc.maxInflightSize, key)
		}
		return fn()
	}

	// Double-check after acquiring write lock.
	req, exists = rc.inflight[key]
	if exists {
		// Increment coalesced requests counter.
		rc.coalescedRequests++
		rc.totalRequests++
		rc.mu.Unlock()
		// Another goroutine created it, wait for it.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-req.waiters:
			return req.result, req.err
		}
	}

	// Create new inflight request.
	rc.totalRequests++
	req = &inflightRequest{
		waiters:   make(chan struct{}),
		createdAt: time.Now(),
	}
	rc.inflight[key] = req
	rc.mu.Unlock()

	// Execute the function.
	result, err := fn()

	// Store result and error.
	req.result = result
	req.err = err

	// Notify all waiters.
	close(req.waiters)

	// Remove from inflight map after a short delay to allow all waiters to read the result.
	// This prevents race conditions where a waiter might read before the result is set.
	go func() {
		time.Sleep(50 * time.Millisecond)
		rc.mu.Lock()
		delete(rc.inflight, key)
		rc.mu.Unlock()
	}()

	return result, err
}

// cleanupLoop periodically removes stale inflight requests that have exceeded maxInflightAge.
// This prevents memory leaks in case a request hangs or takes too long.
func (rc *RequestCoalescer) cleanupLoop() {
	ticker := time.NewTicker(rc.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rc.stopCleanup:
			return
		case <-ticker.C:
			rc.cleanup()
		}
	}
}

// cleanup removes stale inflight requests that have exceeded maxInflightAge.
func (rc *RequestCoalescer) cleanup() {
	rc.cleanupMu.Lock()
	defer rc.cleanupMu.Unlock()

	now := time.Now()
	rc.mu.Lock()
	defer rc.mu.Unlock()

	var staleKeys []string
	for key, req := range rc.inflight {
		if now.Sub(req.createdAt) > rc.maxInflightAge {
			staleKeys = append(staleKeys, key)
			// Close the waiters channel if not already closed.
			select {
			case <-req.waiters:
				// Already closed, do nothing.
			default:
				close(req.waiters)
			}
		}
	}

	// Remove stale requests.
	for _, key := range staleKeys {
		delete(rc.inflight, key)
	}

	// Update metrics.
	if len(staleKeys) > 0 {
		rc.cleanupCount++
		if global.Logger != nil {
			global.Logger.Warnf("RequestCoalescer: cleaned up %d stale inflight requests", len(staleKeys))
		}
	}

	// Update max memory usage estimate.
	estimatedMemory := rc.estimateMemoryUsage()
	if estimatedMemory > rc.maxMemoryUsage {
		rc.maxMemoryUsage = estimatedMemory
	}
}

// Stop stops the cleanup goroutine. This should be called during graceful shutdown.
func (rc *RequestCoalescer) Stop() {
	close(rc.stopCleanup)
}

// Stats returns statistics about the coalescer.
//
// Returns:
//   - int: Number of currently inflight requests.
func (rc *RequestCoalescer) Stats() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.inflight)
}

// GetMaxInflightSize returns the maximum number of concurrent inflight requests.
func (rc *RequestCoalescer) GetMaxInflightSize() int {
	return rc.maxInflightSize
}

// GetInflightCount returns the current number of inflight requests.
func (rc *RequestCoalescer) GetInflightCount() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.inflight)
}

// RequestCoalescerMetrics contains performance metrics for request coalescing.
type RequestCoalescerMetrics struct {
	// InflightCount is the current number of inflight requests.
	InflightCount int64
	// TotalRequests is the total number of requests processed.
	TotalRequests int64
	// CoalescedRequests is the number of requests that were coalesced (waited for existing request).
	CoalescedRequests int64
	// DirectRequests is the number of requests executed directly (not coalesced, including when maxInflightSize reached).
	DirectRequests int64
	// CleanupCount is the number of cleanup operations performed.
	CleanupCount int64
	// MaxMemoryUsage is the maximum estimated memory usage in bytes.
	MaxMemoryUsage int64
	// CurrentMemoryUsage is the current estimated memory usage in bytes.
	CurrentMemoryUsage int64
	// CoalescedRatio is the ratio of coalesced requests to total requests (0.0 to 1.0).
	CoalescedRatio float64
	// MaxInflightSize is the maximum number of concurrent inflight requests.
	MaxInflightSize int
}

// GetMetrics returns comprehensive metrics about the coalescer.
//
// Returns:
//   - RequestCoalescerMetrics: Metrics containing all statistics.
func (rc *RequestCoalescer) GetMetrics() RequestCoalescerMetrics {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	currentMemory := rc.estimateMemoryUsage()

	var coalescedRatio float64
	if rc.totalRequests > 0 {
		coalescedRatio = float64(rc.coalescedRequests) / float64(rc.totalRequests)
	}

	return RequestCoalescerMetrics{
		InflightCount:      int64(len(rc.inflight)),
		TotalRequests:      rc.totalRequests,
		CoalescedRequests:  rc.coalescedRequests,
		DirectRequests:     rc.directRequests,
		CleanupCount:       rc.cleanupCount,
		MaxMemoryUsage:     rc.maxMemoryUsage,
		CurrentMemoryUsage: currentMemory,
		CoalescedRatio:     coalescedRatio,
		MaxInflightSize:    rc.maxInflightSize,
	}
}

// estimateMemoryUsage estimates the current memory usage of inflight requests.
// This is a rough estimate based on the number of requests and average size.
//
// Returns:
//   - int64: Estimated memory usage in bytes.
func (rc *RequestCoalescer) estimateMemoryUsage() int64 {
	if len(rc.inflight) == 0 {
		return 0
	}

	// Rough estimation:
	// - Each inflightRequest struct: ~100 bytes (channel, pointers, time).
	// - Each key (string): ~50-100 bytes average.
	// - Each result: estimated 50KB average (Transaction ~50KB, Block ~200KB, average ~50KB).
	// - Total per entry: ~50KB + 150 bytes ≈ 50KB.

	const avgEntrySize = 50 * 1024 // 50KB average per entry.
	return int64(len(rc.inflight)) * avgEntrySize
}

// LogMetrics logs the metrics using the global logger.
func (rc *RequestCoalescer) LogMetrics() {
	metrics := rc.GetMetrics()
	if global.Logger != nil {
		global.Logger.Infof("RequestCoalescer metrics: inflight=%d, total=%d, coalesced=%d (%.2f%%), direct=%d, cleanup=%d, max_memory=%d KB, current_memory=%d KB, max_size=%d",
			metrics.InflightCount,
			metrics.TotalRequests,
			metrics.CoalescedRequests,
			metrics.CoalescedRatio*100,
			metrics.DirectRequests,
			metrics.CleanupCount,
			metrics.MaxMemoryUsage/1024,
			metrics.CurrentMemoryUsage/1024,
			metrics.MaxInflightSize)
	}
}

// ResetMetrics resets all metrics counters (useful for testing or periodic reset).
// Note: This does not reset inflight requests or maxMemoryUsage.
func (rc *RequestCoalescer) ResetMetrics() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.totalRequests = 0
	rc.coalescedRequests = 0
	rc.directRequests = 0
	rc.cleanupCount = 0
	// Note: We don't reset maxMemoryUsage as it represents historical peak.
}
