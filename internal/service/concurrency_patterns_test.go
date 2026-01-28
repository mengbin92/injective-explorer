package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestConcurrencySemaphorePattern demonstrates and tests the use of buffered channels
// as semaphores for limiting concurrent operations, a key Go pattern used throughout
// the codebase for controlling API call concurrency.
func TestConcurrencySemaphorePattern(t *testing.T) {
	t.Run("Basic semaphore limits concurrency", func(t *testing.T) {
		maxConcurrency := 3
		semaphore := make(chan struct{}, maxConcurrency)

		var concurrentCount int32
		var maxConcurrent int32
		var wg sync.WaitGroup

		// Launch more goroutines than the limit
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				// Acquire semaphore
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				// Track concurrent execution
				current := atomic.AddInt32(&concurrentCount, 1)
				for {
					max := atomic.LoadInt32(&maxConcurrent)
					if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
						break
					}
				}

				// Simulate work
				time.Sleep(10 * time.Millisecond)

				atomic.AddInt32(&concurrentCount, -1)
			}()
		}

		wg.Wait()

		// Verify concurrency was limited
		assert.LessOrEqual(t, maxConcurrent, int32(maxConcurrency),
			"Concurrent operations should not exceed semaphore capacity")
	})

	t.Run("Semaphore prevents goroutine leaks", func(t *testing.T) {
		maxConcurrency := 2
		semaphore := make(chan struct{}, maxConcurrency)

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup

		// Launch workers that respect context cancellation
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				select {
				case semaphore <- struct{}{}:
					// Acquired semaphore, do work
					select {
					case <-ctx.Done():
						// Context cancelled, release semaphore and exit
						<-semaphore
						return
					case <-time.After(5 * time.Millisecond):
						// Work completed
						<-semaphore
					}
				case <-ctx.Done():
					// Context cancelled before acquiring semaphore
					return
				}
			}(i)
		}

		// Cancel context to trigger graceful shutdown
		time.Sleep(20 * time.Millisecond)
		cancel()

		// Wait with timeout to detect goroutine leaks
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// All goroutines exited cleanly
		case <-time.After(2 * time.Second):
			t.Fatal("Goroutines did not exit within timeout - potential goroutine leak")
		}
	})

	t.Run("Semaphore with timeout pattern", func(t *testing.T) {
		maxConcurrency := 1
		semaphore := make(chan struct{}, maxConcurrency)

		// Fill the semaphore
		semaphore <- struct{}{}

		// Try to acquire with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		select {
		case semaphore <- struct{}{}:
			t.Fatal("Should not be able to acquire full semaphore")
		case <-ctx.Done():
			// Expected - timeout occurred
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())
		}

		// Release and verify we can now acquire
		<-semaphore

		select {
		case semaphore <- struct{}{}:
			// Success
			<-semaphore
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Should be able to acquire after release")
		}
	})
}

// TestWorkerPoolPattern tests the worker pool pattern used for concurrent processing
func TestWorkerPoolPattern(t *testing.T) {
	t.Run("Worker pool processes all tasks", func(t *testing.T) {
		numWorkers := 3
		numTasks := 100

		taskChan := make(chan int, numTasks)
		resultChan := make(chan int, numTasks)

		var wg sync.WaitGroup

		// Start workers
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for task := range taskChan {
					// Process task
					resultChan <- task * 2
				}
			}(i)
		}

		// Send tasks
		go func() {
			for i := 0; i < numTasks; i++ {
				taskChan <- i
			}
			close(taskChan)
		}()

		// Wait for completion
		wg.Wait()
		close(resultChan)

		// Verify all tasks processed
		results := make([]int, 0, numTasks)
		for r := range resultChan {
			results = append(results, r)
		}

		assert.Len(t, results, numTasks)

		// Verify results are correct (each task multiplied by 2)
		resultMap := make(map[int]bool)
		for _, r := range results {
			resultMap[r] = true
		}
		for i := 0; i < numTasks; i++ {
			assert.True(t, resultMap[i*2], "Missing result for task %d", i)
		}
	})

	t.Run("Worker pool with context cancellation", func(t *testing.T) {
		numWorkers := 3
		ctx, cancel := context.WithCancel(context.Background())

		taskChan := make(chan int)
		var processedCount int32
		var wg sync.WaitGroup

		// Start workers
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					case task, ok := <-taskChan:
						if !ok {
							return
						}
						atomic.AddInt32(&processedCount, 1)
						_ = task
					}
				}
			}()
		}

		// Send some tasks
		for i := 0; i < 10; i++ {
			taskChan <- i
		}

		// Cancel context
		time.Sleep(10 * time.Millisecond)
		cancel()
		close(taskChan)

		// Wait for workers to exit
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Workers exited cleanly
			assert.Greater(t, atomic.LoadInt32(&processedCount), int32(0),
				"Some tasks should have been processed")
		case <-time.After(2 * time.Second):
			t.Fatal("Workers did not exit within timeout - potential goroutine leak")
		}
	})
}

// TestBatchProcessingPattern tests the batch processing pattern used for database operations
func TestBatchProcessingPattern(t *testing.T) {
	t.Run("Process in batches", func(t *testing.T) {
		items := make([]int, 250)
		for i := range items {
			items[i] = i
		}

		batchSize := 100
		var batches [][]int

		for i := 0; i < len(items); i += batchSize {
			end := i + batchSize
			if end > len(items) {
				end = len(items)
			}
			batches = append(batches, items[i:end])
		}

		// Verify batching
		assert.Len(t, batches, 3)
		assert.Len(t, batches[0], 100)
		assert.Len(t, batches[1], 100)
		assert.Len(t, batches[2], 50)
	})

	t.Run("Batch processing with pre-allocation", func(t *testing.T) {
		input := make([]int, 1000)
		for i := range input {
			input[i] = i
		}

		// Pre-allocate result slice with known capacity
		results := make([]int, 0, len(input))

		batchSize := 100
		for i := 0; i < len(input); i += batchSize {
			end := i + batchSize
			if end > len(input) {
				end = len(input)
			}

			// Process batch
			for _, item := range input[i:end] {
				results = append(results, item*2)
			}
		}

		assert.Len(t, results, len(input))
	})
}

// TestPreAllocationPattern tests slice pre-allocation for performance
func TestPreAllocationPattern(t *testing.T) {
	t.Run("Pre-allocated vs dynamic growth", func(t *testing.T) {
		n := 10000

		// Dynamic growth (inefficient)
		var dynamic []int
		for i := 0; i < n; i++ {
			dynamic = append(dynamic, i)
		}

		// Pre-allocated (efficient)
		prealloc := make([]int, 0, n)
		for i := 0; i < n; i++ {
			prealloc = append(prealloc, i)
		}

		assert.Len(t, dynamic, n)
		assert.Len(t, prealloc, n)
		assert.Equal(t, cap(prealloc), n)
	})

	t.Run("Map pre-allocation with known size", func(t *testing.T) {
		n := 1000

		// Pre-allocate map
		m := make(map[int]bool, n)

		for i := 0; i < n; i++ {
			m[i] = true
		}

		assert.Len(t, m, n)
	})
}

// BenchmarkSemaphoreOverhead benchmarks the overhead of semaphore operations
func BenchmarkSemaphoreOverhead(b *testing.B) {
	tests := []struct {
		name    string
		workers int
		limit   int
	}{
		{"Unlimited", 100, 100},
		{"Limit-10", 100, 10},
		{"Limit-5", 100, 5},
		{"Limit-1", 100, 1},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			semaphore := make(chan struct{}, tt.limit)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				for j := 0; j < tt.workers; j++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						semaphore <- struct{}{}
						// Simulate minimal work
						<-semaphore
					}()
				}
				wg.Wait()
			}
		})
	}
}

// BenchmarkBatchProcessing benchmarks different batch sizes
func BenchmarkBatchProcessing(b *testing.B) {
	sizes := []int{10, 50, 100, 500, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("BatchSize-%d", size), func(b *testing.B) {
			items := make([]int, 10000)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				for j := 0; j < len(items); j += size {
					end := j + size
					if end > len(items) {
						end = len(items)
					}
					_ = items[j:end]
				}
			}
		})
	}
}
