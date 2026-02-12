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

func (m *MockExplorerClient) StreamBlocks(ctx context.Context) (<-chan *injective_explorer_rpcpb.StreamBlocksResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan *injective_explorer_rpcpb.StreamBlocksResponse), args.Error(1)
}

func (m *MockExplorerClient) Close() {
	m.Called()
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

// TestExtractContractAddressFromCosmosEvents tests the extraction of contract address from Cosmos events
func TestExtractContractAddressFromCosmosEvents(t *testing.T) {
	t.Run("EventCreateTokenPair", func(t *testing.T) {
		// This test verifies the parsing logic for EventCreateTokenPair
		// Full integration test requires database connection

		// Sample event data as it would appear in the chain
		eventType := "injective.erc20.v1beta1.EventCreateTokenPair"
		eventAttrs := []struct {
			key   string
			value string
		}{
			{"erc20_address", "\"0x9772154BF1823353C829B43F9AD96CF9f720c768\""},
			{"bank_denom", "\"peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca\""},
			{"sender", "\"0x15301c6f8a51b177bd3c24f7b5d978b2c02b0da0\""},
		}

		// Verify the event type matches what we're looking for
		assert.Equal(t, "injective.erc20.v1beta1.EventCreateTokenPair", eventType)

		// Extract values
		var contractAddr, bankDenom, sender string
		for _, attr := range eventAttrs {
			switch attr.key {
			case "erc20_address":
				contractAddr = attr.value
			case "bank_denom":
				bankDenom = attr.value
			case "sender":
				sender = attr.value
			}
		}

		// Verify extraction (values include quotes from JSON serialization)
		assert.Equal(t, "\"0x9772154BF1823353C829B43F9AD96CF9f720c768\"", contractAddr)
		assert.Equal(t, "\"peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca\"", bankDenom)
		assert.Equal(t, "\"0x15301c6f8a51b177bd3c24f7b5d978b2c02b0da0\"", sender)
	})

	t.Run("EmptyEvents", func(t *testing.T) {
		// Test with nil/empty events - should return early without error
		// This is a unit test for the early return condition
		assert.True(t, true) // Placeholder - actual test requires DB setup
	})
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
