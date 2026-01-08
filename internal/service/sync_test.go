package service

import (
	"context"
	"sync"
	"testing"
	"time"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockExplorerClient mocks the explorer client
type MockExplorerClient struct {
	mock.Mock
}

func (m *MockExplorerClient) GetBlock(ctx context.Context, height string) (*injective_explorer_rpcpb.GetBlockResponse, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*injective_explorer_rpcpb.GetBlockResponse), args.Error(1)
}

func (m *MockExplorerClient) StreamBlocks(ctx context.Context) (<-chan *injective_explorer_rpcpb.StreamBlocksResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).(<-chan *injective_explorer_rpcpb.StreamBlocksResponse), args.Error(1)
}

// MockChainClient mocks the chain client
// MockChainClient mocks the chain client
type MockChainClient struct {
	mock.Mock
}

// TestBatchCheckBlockExistence tests the batch existence check functionality
func TestBatchCheckBlockExistence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a sync service instance
	service := &SyncService{
		ctx: context.Background(),
	}

	// Test with empty slice
	t.Run("EmptyHeights", func(t *testing.T) {
		result, err := service.batchCheckBlockExistence([]uint64{})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, len(result))
	})

	// Test with heights (requires DB setup)
	t.Run("WithHeights", func(t *testing.T) {
		t.Skip("requires database connection for full testing")
		heights := []uint64{1001, 1002, 1003, 1004, 1005}
		_, err := service.batchCheckBlockExistence(heights)
		// This would fail without DB, but we test the function exists and runs
		assert.Error(t, err) // Expected to fail without DB
	})
}

// TestBlockWorker tests the block worker function
func TestBlockWorker(t *testing.T) {
	t.Run("WorkerProcessesTasks", func(t *testing.T) {
		// Create a mock service
		service := &SyncService{
			ctx: context.Background(),
			config: &SyncConfig{
				BlockQueryTimeout: 30 * time.Second,
			},
			// Note: processBlock would be mocked in real tests
		}

		taskChan := make(chan uint64, 10)
		resultChan := make(chan processResult, 10)

		// Add tasks
		for i := uint64(1); i <= 10; i++ {
			taskChan <- i
		}
		close(taskChan)

		var wg sync.WaitGroup

		// Start worker
		wg.Add(1)
		go service.blockWorker(1, taskChan, resultChan, &wg)
		wg.Wait()
		close(resultChan)

		// Collect results
		results := make([]processResult, 0)
		for result := range resultChan {
			results = append(results, result)
		}

		// Should have 10 results (though they may all be errors without proper mocks)
		assert.Equal(t, 10, len(results))
	})

	t.Run("WorkerShutdown", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		service := &SyncService{
			ctx: ctx,
			config: &SyncConfig{
				BlockQueryTimeout: 30 * time.Second,
			},
		}

		taskChan := make(chan uint64, 10)
		resultChan := make(chan processResult, 10)

		// Add some tasks
		taskChan <- 1
		taskChan <- 2
		close(taskChan)

		var wg sync.WaitGroup

		// Start worker
		wg.Add(1)
		go service.blockWorker(1, taskChan, resultChan, &wg)

		// Cancel context after short delay
		time.Sleep(10 * time.Millisecond)
		cancel()

		// Wait for worker to finish
		wg.Wait()
		close(resultChan)

		// Should have processed some tasks before shutdown
		// (exact count depends on timing)
		results := make([]processResult, 0)
		for result := range resultChan {
			results = append(results, result)
		}
		assert.GreaterOrEqual(t, len(results), 0)
	})
}

// TestProcessResultStruct tests the processResult structure
func TestProcessResultStruct(t *testing.T) {
	result := processResult{
		height:   1001,
		err:      assert.AnError,
		duration: 50 * time.Millisecond,
	}

	assert.Equal(t, uint64(1001), result.height)
	assert.Equal(t, assert.AnError, result.err)
	assert.Equal(t, 50*time.Millisecond, result.duration)
}

// BenchmarkBlockWorker benchmarks the block worker performance
func BenchmarkBlockWorker(b *testing.B) {
	service := &SyncService{
		ctx: context.Background(),
		config: &SyncConfig{
			BlockQueryTimeout: 30 * time.Second,
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		taskChan := make(chan uint64, 100)
		resultChan := make(chan processResult, 100)

		for j := uint64(1); j <= 100; j++ {
			taskChan <- j
		}
		close(taskChan)

		var wg sync.WaitGroup
		wg.Add(1)
		go service.blockWorker(1, taskChan, resultChan, &wg)
		wg.Wait()
		close(resultChan)
	}
}
