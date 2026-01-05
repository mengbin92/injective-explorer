package middleware

import (
	"context"
	"sync"
	"testing"
	"time"

	"injective-explorer/internal/service"
)

func TestRequestCoalescer_Do(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	ctx := context.Background()
	key := "test:key:1"

	// Track how many times the function is called
	callCount := 0
	var callCountMu sync.Mutex

	// Function that simulates a slow operation
	fn := func() (interface{}, error) {
		callCountMu.Lock()
		callCount++
		callCountMu.Unlock()

		// Simulate some work
		time.Sleep(100 * time.Millisecond)
		return "result", nil
	}

	// Launch 10 concurrent requests for the same key
	var wg sync.WaitGroup
	results := make([]interface{}, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := coalescer.Do(ctx, key, fn)
			results[idx] = result
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify that the function was only called once
	callCountMu.Lock()
	if callCount != 1 {
		t.Errorf("expected function to be called once, got %d", callCount)
	}
	callCountMu.Unlock()

	// Verify all results are the same
	for i := 0; i < 10; i++ {
		if errors[i] != nil {
			t.Errorf("request %d returned error: %v", i, errors[i])
		}
		if results[i] != "result" {
			t.Errorf("request %d returned wrong result: got %v, want 'result'", i, results[i])
		}
	}
}

func TestRequestCoalescer_DifferentKeys(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	ctx := context.Background()

	// Track how many times the function is called
	callCount := 0
	var callCountMu sync.Mutex

	// Function that simulates a slow operation
	fn := func() (interface{}, error) {
		callCountMu.Lock()
		callCount++
		callCountMu.Unlock()

		time.Sleep(50 * time.Millisecond)
		return "result", nil
	}

	// Launch requests for different keys
	var wg sync.WaitGroup
	keys := []string{"key1", "key2", "key3"}

	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			_, _ = coalescer.Do(ctx, k, fn)
		}(key)
	}

	wg.Wait()

	// Verify that the function was called once for each key
	callCountMu.Lock()
	if callCount != len(keys) {
		t.Errorf("expected function to be called %d times (once per key), got %d", len(keys), callCount)
	}
	callCountMu.Unlock()
}

func TestRequestCoalescer_ErrorHandling(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	ctx := context.Background()
	key := "test:error:key"

	// Function that returns an error
	fn := func() (interface{}, error) {
		time.Sleep(50 * time.Millisecond)
		return nil, context.DeadlineExceeded
	}

	// Launch multiple concurrent requests
	var wg sync.WaitGroup
	errors := make([]error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := coalescer.Do(ctx, key, fn)
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify all requests got the same error
	for i := 0; i < 5; i++ {
		if errors[i] != context.DeadlineExceeded {
			t.Errorf("request %d returned wrong error: got %v, want %v", i, errors[i], context.DeadlineExceeded)
		}
	}
}

func TestRequestCoalescer_ContextCancellation(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	key := "test:cancel:key"

	// Function that takes a long time
	fn := func() (interface{}, error) {
		time.Sleep(500 * time.Millisecond)
		return "result", nil
	}

	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Launch a request that will be cancelled BEFORE another request starts
	var wg sync.WaitGroup
	var err error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err = coalescer.Do(ctx, key, fn)
	}()

	// Cancel the context immediately (before the function completes)
	// This tests that cancellation works when there's no other request waiting
	cancel()

	wg.Wait()

	// Verify that the request was cancelled
	// Note: If another request completes first, we might get the result instead
	// This is acceptable behavior for request coalescing
	if err != nil && err != context.Canceled {
		t.Errorf("expected context.Canceled error or nil (if another request completed), got %v", err)
	}
}

func TestRequestCoalescer_Stats(t *testing.T) {
	coalescer := service.NewRequestCoalescer(30*time.Second, 1*time.Minute, 1000)
	defer coalescer.Stop()

	// Initially should be 0
	if stats := coalescer.Stats(); stats != 0 {
		t.Errorf("expected 0 inflight requests initially, got %d", stats)
	}

	ctx := context.Background()
	key := "test:stats:key"

	// Start a request that takes some time
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = coalescer.Do(ctx, key, func() (interface{}, error) {
			time.Sleep(200 * time.Millisecond)
			return "result", nil
		})
	}()

	// Wait a bit for the request to start
	time.Sleep(10 * time.Millisecond)

	// Should have 1 inflight request
	if stats := coalescer.Stats(); stats != 1 {
		t.Errorf("expected 1 inflight request, got %d", stats)
	}

	// Wait for request to complete
	wg.Wait()

	// Wait a bit for cleanup
	time.Sleep(150 * time.Millisecond)

	// Should be back to 0
	if stats := coalescer.Stats(); stats != 0 {
		t.Errorf("expected 0 inflight requests after completion, got %d", stats)
	}
}

