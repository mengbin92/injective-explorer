// Package sync provides worker pool utilities for concurrent processing.
package sync

import (
	"context"
	"fmt"
	"sync"
)

// Task represents a unit of work that can be processed by the worker pool.
type Task[T any] struct {
	Item T
	Err  error
}

// TaskProcessor is a function that processes a single item.
type TaskProcessor[T any] func(ctx context.Context, item T) error

// WorkerPool provides concurrent processing capabilities with controlled parallelism.
// It uses a semaphore pattern to limit the number of concurrent goroutines.
type WorkerPool[T any] struct {
	name        string
	concurrency int
}

// NewWorkerPool creates a new WorkerPool with the specified name and concurrency level.
//
// Parameters:
//   - name: A descriptive name for the worker pool (used for logging)
//   - concurrency: Maximum number of concurrent workers
//
// Returns:
//   - *WorkerPool[T]: A new worker pool instance
func NewWorkerPool[T any](name string, concurrency int) *WorkerPool[T] {
	if concurrency <= 0 {
		concurrency = 10 // default concurrency
	}
	return &WorkerPool[T]{
		name:        name,
		concurrency: concurrency,
	}
}

// ProcessConcurrently processes a slice of items concurrently using the worker pool.
// It maintains the order of results and handles errors gracefully.
//
// Parameters:
//   - ctx: Context for cancellation
//   - items: Slice of items to process
//   - processor: Function to process each item
//
// Returns:
//   - []error: Slice of errors (one per item, nil if successful)
//   - error: Error if the processing was cancelled or failed
func (wp *WorkerPool[T]) ProcessConcurrently(
	ctx context.Context,
	items []T,
	processor TaskProcessor[T],
) ([]error, error) {
	if len(items) == 0 {
		return []error{}, nil
	}

	numItems := len(items)
	errors := make([]error, numItems)

	// Create a buffered channel to act as semaphore
	semaphore := make(chan struct{}, wp.concurrency)

	var wg sync.WaitGroup
	errChan := make(chan struct {
		index int
		err   error
	}, numItems)

	// Start a goroutine to collect errors
	done := make(chan struct{})
	go func() {
		for e := range errChan {
			errors[e.index] = e.err
		}
		close(done)
	}()

	// Process items concurrently
	for i, item := range items {
		wg.Add(1)
		go func(index int, item T) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
				// Acquired
			case <-ctx.Done():
				errChan <- struct {
					index int
					err   error
				}{index: index, err: ctx.Err()}
				return
			}
			defer func() { <-semaphore }() // Release semaphore

			// Process the item
			if err := processor(ctx, item); err != nil {
				errChan <- struct {
					index int
					err   error
				}{index: index, err: err}
			}
		}(i, item)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(errChan)
	<-done

	// Check if context was cancelled
	if ctx.Err() != nil {
		return errors, ctx.Err()
	}

	return errors, nil
}

// ProcessConcurrentlyWithResults processes items concurrently and returns results.
// Similar to ProcessConcurrently but also returns processed results.
//
// Parameters:
//   - ctx: Context for cancellation
//   - items: Slice of items to process
//   - processor: Function to process each item and return a result
//
// Returns:
//   - []R: Slice of results (one per item)
//   - []error: Slice of errors (one per item, nil if successful)
//   - error: Error if the processing was cancelled
func ProcessConcurrentlyWithResults[T any, R any](
	ctx context.Context,
	items []T,
	concurrency int,
	processor func(ctx context.Context, item T) (R, error),
) ([]R, []error, error) {
	if len(items) == 0 {
		return []R{}, []error{}, nil
	}

	if concurrency <= 0 {
		concurrency = 10
	}

	numItems := len(items)
	results := make([]R, numItems)
	errors := make([]error, numItems)

	semaphore := make(chan struct{}, concurrency)
	type result struct {
		index  int
		result R
		err    error
	}
	resultChan := make(chan result, numItems)

	var wg sync.WaitGroup

	// Start a goroutine to collect results
	done := make(chan struct{})
	go func() {
		for r := range resultChan {
			results[r.index] = r.result
			errors[r.index] = r.err
		}
		close(done)
	}()

	// Process items concurrently
	for i, item := range items {
		wg.Add(1)
		go func(index int, item T) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
				// Acquired
			case <-ctx.Done():
				resultChan <- result{index: index, err: ctx.Err()}
				return
			}
			defer func() { <-semaphore }()

			res, err := processor(ctx, item)
			resultChan <- result{index: index, result: res, err: err}
		}(i, item)
	}

	wg.Wait()
	close(resultChan)
	<-done

	if ctx.Err() != nil {
		return results, errors, ctx.Err()
	}

	return results, errors, nil
}

// BatchProcessor provides batch processing capabilities for large datasets.
type BatchProcessor[T any] struct {
	batchSize   int
	concurrency int
}

// NewBatchProcessor creates a new BatchProcessor.
//
// Parameters:
//   - batchSize: Number of items to process in each batch
//   - concurrency: Maximum number of concurrent batches
//
// Returns:
//   - *BatchProcessor[T]: A new batch processor instance
func NewBatchProcessor[T any](batchSize, concurrency int) *BatchProcessor[T] {
	if batchSize <= 0 {
		batchSize = 100
	}
	if concurrency <= 0 {
		concurrency = 5
	}
	return &BatchProcessor[T]{
		batchSize:   batchSize,
		concurrency: concurrency,
	}
}

// ProcessInBatches processes items in batches with controlled concurrency.
//
// Parameters:
//   - ctx: Context for cancellation
//   - items: Slice of items to process
//   - processor: Function to process a batch of items
//
// Returns:
//   - error: Error if processing failed
func (bp *BatchProcessor[T]) ProcessInBatches(
	ctx context.Context,
	items []T,
	processor func(ctx context.Context, batch []T) error,
) error {
	if len(items) == 0 {
		return nil
	}

	numItems := len(items)
	numBatches := (numItems + bp.batchSize - 1) / bp.batchSize

	semaphore := make(chan struct{}, bp.concurrency)
	var wg sync.WaitGroup
	errChan := make(chan error, numBatches)

	for i := 0; i < numItems; i += bp.batchSize {
		end := i + bp.batchSize
		if end > numItems {
			end = numItems
		}
		batch := items[i:end]

		wg.Add(1)
		go func(batchIndex int, batch []T) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
				// Acquired
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
			defer func() { <-semaphore }()

			if err := processor(ctx, batch); err != nil {
				errChan <- fmt.Errorf("batch %d failed: %w", batchIndex, err)
			}
		}(i/bp.batchSize, batch)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("batch processing failed with %d errors: %v", len(errs), errs[0])
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}
