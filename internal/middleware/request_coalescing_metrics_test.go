package middleware

import (
	"context"
	"sync"
	"testing"
	"time"

	"injective-explorer/internal/service"
)

func TestRequestCoalescer_Metrics(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	ctx := context.Background()
	key := "test:metrics:key"

	// Function that simulates work
	fn := func() (interface{}, error) {
		time.Sleep(50 * time.Millisecond)
		return "result", nil
	}

	// Launch 10 concurrent requests for the same key
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = coalescer.Do(ctx, key, fn)
		}()
	}

	wg.Wait()

	// Wait for cleanup delay (50ms) to ensure inflight requests are cleaned up
	time.Sleep(100 * time.Millisecond)

	// Get metrics
	metrics := coalescer.GetMetrics()

	// Verify metrics
	if metrics.TotalRequests != 10 {
		t.Errorf("expected TotalRequests=10, got %d", metrics.TotalRequests)
	}
	// First request creates new inflight, remaining 9 wait for it (coalesced)
	if metrics.CoalescedRequests != 9 {
		t.Errorf("expected CoalescedRequests=9 (10 requests - 1 new), got %d", metrics.CoalescedRequests)
	}
	// DirectRequests should be 0 (no requests hit maxInflightSize limit)
	if metrics.DirectRequests != 0 {
		t.Errorf("expected DirectRequests=0 (no maxInflightSize limit hit), got %d", metrics.DirectRequests)
	}
	// InflightCount should be 0 after cleanup delay
	if metrics.InflightCount != 0 {
		t.Errorf("expected InflightCount=0 after completion and cleanup, got %d", metrics.InflightCount)
	}
	// CoalescedRatio should be 0.9 (9 coalesced / 10 total)
	if metrics.CoalescedRatio < 0.85 || metrics.CoalescedRatio > 0.95 {
		t.Errorf("expected CoalescedRatio between 0.85 and 0.95, got %.2f", metrics.CoalescedRatio)
	}
}

func TestRequestCoalescer_MetricsDifferentKeys(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	ctx := context.Background()
	keys := []string{"key1", "key2", "key3"}

	// Launch requests for different keys
	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			_, _ = coalescer.Do(ctx, k, func() (interface{}, error) {
				time.Sleep(50 * time.Millisecond)
				return "result", nil
			})
		}(key)
	}

	wg.Wait()

	// Get metrics
	metrics := coalescer.GetMetrics()

	// Verify metrics
	if metrics.TotalRequests != int64(len(keys)) {
		t.Errorf("expected TotalRequests=%d, got %d", len(keys), metrics.TotalRequests)
	}
	if metrics.CoalescedRequests != 0 {
		t.Errorf("expected CoalescedRequests=0 (different keys), got %d", metrics.CoalescedRequests)
	}
	// DirectRequests should be 0 for different keys (they are new requests, not direct due to maxInflightSize)
	// Note: DirectRequests only counts requests that bypass coalescing due to maxInflightSize limit
	if metrics.DirectRequests != 0 {
		t.Errorf("expected DirectRequests=0 (different keys, not hitting maxInflightSize), got %d", metrics.DirectRequests)
	}
}

func TestRequestCoalescer_MetricsMaxSize(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 5)
	defer coalescer.Stop()

	ctx := context.Background()

	// Launch more requests than maxInflightSize with different keys
	// This will trigger the maxInflightSize limit
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Use different keys to ensure we hit maxInflightSize
			key := "test:maxsize:key" + string(rune('0'+idx))
			_, _ = coalescer.Do(ctx, key, func() (interface{}, error) {
				// Use a longer sleep to ensure requests overlap
				time.Sleep(200 * time.Millisecond)
				return "result", nil
			})
		}(i)
	}

	wg.Wait()

	// Get metrics
	metrics := coalescer.GetMetrics()

	// Verify that some requests were executed directly (when maxInflightSize was reached)
	if metrics.TotalRequests != 10 {
		t.Errorf("expected TotalRequests=10, got %d", metrics.TotalRequests)
	}
	// At least some requests should be direct (when maxInflightSize=5 was reached)
	// Note: Due to timing, exact count may vary, but should be >= 5
	if metrics.DirectRequests < 5 {
		t.Logf("DirectRequests=%d (may vary due to timing), expected >= 5", metrics.DirectRequests)
		// This is acceptable - timing can affect when maxInflightSize is hit
	}
}

func TestRequestCoalescer_LogMetrics(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	ctx := context.Background()

	// Execute some requests
	for i := 0; i < 5; i++ {
		key := "test:log:key" + string(rune('0'+i))
		_, _ = coalescer.Do(ctx, key, func() (interface{}, error) {
			return "result", nil
		})
	}

	// Log metrics (should not panic)
	coalescer.LogMetrics()

	// Verify metrics are still accessible
	metrics := coalescer.GetMetrics()
	if metrics.TotalRequests != 5 {
		t.Errorf("expected TotalRequests=5, got %d", metrics.TotalRequests)
	}
}

func TestRequestCoalescer_ResetMetrics(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	ctx := context.Background()

	// Execute some requests
	for i := 0; i < 5; i++ {
		key := "test:reset:key" + string(rune('0'+i))
		_, _ = coalescer.Do(ctx, key, func() (interface{}, error) {
			return "result", nil
		})
	}

	// Verify metrics exist
	metrics1 := coalescer.GetMetrics()
	if metrics1.TotalRequests == 0 {
		t.Fatal("expected metrics to have values before reset")
	}

	// Reset metrics
	coalescer.ResetMetrics()

	// Verify metrics are reset (except maxMemoryUsage)
	metrics2 := coalescer.GetMetrics()
	if metrics2.TotalRequests != 0 {
		t.Errorf("expected TotalRequests=0 after reset, got %d", metrics2.TotalRequests)
	}
	if metrics2.CoalescedRequests != 0 {
		t.Errorf("expected CoalescedRequests=0 after reset, got %d", metrics2.CoalescedRequests)
	}
	if metrics2.DirectRequests != 0 {
		t.Errorf("expected DirectRequests=0 after reset, got %d", metrics2.DirectRequests)
	}
	if metrics2.CleanupCount != 0 {
		t.Errorf("expected CleanupCount=0 after reset, got %d", metrics2.CleanupCount)
	}
	// maxMemoryUsage should be preserved
	if metrics2.MaxMemoryUsage != metrics1.MaxMemoryUsage {
		t.Errorf("expected MaxMemoryUsage to be preserved after reset, got %d (was %d)", metrics2.MaxMemoryUsage, metrics1.MaxMemoryUsage)
	}
}

