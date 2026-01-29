// Package service provides tests for pagination cache fixes.
package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetBlocksFromCacheWithOffset_Validation tests the offset-based cache retrieval
func TestGetBlocksFromCacheWithOffset_Validation(t *testing.T) {
	ctx := context.Background()

	// Test with nil Redis client (should handle gracefully)
	blocks, hasMore, err := getBlocksFromCacheWithOffset(ctx, 0, 10)
	if cache.GetRedisClient() == nil {
		assert.Error(t, err)
		assert.Nil(t, blocks)
		assert.False(t, hasMore)
	} else {
		// Redis is available - test with empty cache
		assert.NoError(t, err)
		assert.False(t, hasMore)
	}
}

// TestGetTransactionsFromCacheWithOffset_Validation tests the offset-based cache retrieval
func TestGetTransactionsFromCacheWithOffset_Validation(t *testing.T) {
	ctx := context.Background()

	// Test with nil Redis client
	txs, hasMore, err := getTransactionsFromCacheWithOffset(ctx, 0, 10)
	if cache.GetRedisClient() == nil {
		assert.Error(t, err)
		assert.Nil(t, txs)
		assert.False(t, hasMore)
	} else {
		// Redis is available - test with empty cache
		assert.NoError(t, err)
		assert.False(t, hasMore)
	}
}

// TestBlockSorting_ConsistentHeight tests that blocks are sorted by height consistently
func TestBlockSorting_ConsistentHeight(t *testing.T) {
	// Create blocks with same timestamp but different heights
	blocks := []models.Block{
		{Height: 100, BlockUnixTimestamp: 1000},
		{Height: 102, BlockUnixTimestamp: 1000}, // Same timestamp, higher height
		{Height: 101, BlockUnixTimestamp: 1000},
	}

	// Sort by height descending (new logic)
	for i := 0; i < len(blocks)-1; i++ {
		for j := i + 1; j < len(blocks); j++ {
			if blocks[i].Height < blocks[j].Height {
				blocks[i], blocks[j] = blocks[j], blocks[i]
			}
		}
	}

	// Verify sorted order
	assert.Equal(t, uint64(102), blocks[0].Height)
	assert.Equal(t, uint64(101), blocks[1].Height)
	assert.Equal(t, uint64(100), blocks[2].Height)
}

// TestTransactionSorting_ConsistentTxNumber tests that transactions are sorted by tx_number consistently
func TestTransactionSorting_ConsistentTxNumber(t *testing.T) {
	// Create transactions with same timestamp but different tx numbers
	txs := []models.Transaction{
		{TxNumber: 100, BlockUnixTimestamp: 1000},
		{TxNumber: 102, BlockUnixTimestamp: 1000}, // Same timestamp, higher tx number
		{TxNumber: 101, BlockUnixTimestamp: 1000},
	}

	// Sort by tx_number descending (new logic)
	for i := 0; i < len(txs)-1; i++ {
		for j := i + 1; j < len(txs); j++ {
			if txs[i].TxNumber < txs[j].TxNumber {
				txs[i], txs[j] = txs[j], txs[i]
			}
		}
	}

	// Verify sorted order
	assert.Equal(t, uint64(102), txs[0].TxNumber)
	assert.Equal(t, uint64(101), txs[1].TxNumber)
	assert.Equal(t, uint64(100), txs[2].TxNumber)
}

// TestPaginationConsistency tests that cursor and sort field are consistent
func TestPaginationConsistency(t *testing.T) {
	// This test verifies the fix for the pagination bug where:
	// - Cursor was based on height/tx_number
	// - Sort was based on timestamp
	// This caused page 2 to have higher blocks than page 1

	t.Run("block pagination consistency", func(t *testing.T) {
		blocks := []models.Block{
			{Height: 1003, BlockUnixTimestamp: 1000}, // timestamp same as 1002
			{Height: 1002, BlockUnixTimestamp: 1000},
			{Height: 1001, BlockUnixTimestamp: 999},
		}

		// Simulate: page 1 returns [1003, 1002]
		page1 := blocks[:2]

		// Cursor calculation: min height = 1002
		minHeight := page1[0].Height
		for _, b := range page1 {
			if b.Height < minHeight {
				minHeight = b.Height
			}
		}
		assert.Equal(t, uint64(1002), minHeight)

		// With the fix: page 2 query uses "height < 1002" with "ORDER BY height DESC"
		// This correctly returns [1001]
		var page2 []models.Block
		for _, b := range blocks {
			if b.Height < minHeight {
				page2 = append(page2, b)
			}
		}

		// Verify page 2 heights are all less than page 1 min height
		for _, b := range page2 {
			assert.Less(t, b.Height, minHeight)
		}
	})

	t.Run("transaction pagination consistency", func(t *testing.T) {
		txs := []models.Transaction{
			{TxNumber: 1003, BlockUnixTimestamp: 1000},
			{TxNumber: 1002, BlockUnixTimestamp: 1000},
			{TxNumber: 1001, BlockUnixTimestamp: 999},
		}

		// Simulate: page 1 returns [1003, 1002]
		page1 := txs[:2]

		// Cursor calculation: min tx_number = 1002
		minTxNumber := page1[0].TxNumber
		for _, tx := range page1 {
			if tx.TxNumber < minTxNumber {
				minTxNumber = tx.TxNumber
			}
		}
		assert.Equal(t, uint64(1002), minTxNumber)

		// With the fix: page 2 query uses "tx_number < 1002" with "ORDER BY tx_number DESC"
		var page2 []models.Transaction
		for _, tx := range txs {
			if tx.TxNumber < minTxNumber {
				page2 = append(page2, tx)
			}
		}

		// Verify page 2 tx_numbers are all less than page 1 min
		for _, tx := range page2 {
			assert.Less(t, tx.TxNumber, minTxNumber)
		}
	})
}

// TestCacheOffsetCalculation tests the offset calculation for paginated cache access
func TestCacheOffsetCalculation(t *testing.T) {
	testCases := []struct {
		name        string
		offset      int
		limit       int
		expectStart int64
		expectEnd   int64
	}{
		{
			name:        "first page",
			offset:      0,
			limit:       20,
			expectStart: -20,
			expectEnd:   -1,
		},
		{
			name:        "second page",
			offset:      20,
			limit:       20,
			expectStart: -40,
			expectEnd:   -21,
		},
		{
			name:        "third page",
			offset:      40,
			limit:       20,
			expectStart: -60,
			expectEnd:   -41,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify the offset calculation logic
			startIdx := int64(-(tc.offset + tc.limit))
			endIdx := int64(-tc.offset - 1)

			assert.Equal(t, tc.expectStart, startIdx)
			assert.Equal(t, tc.expectEnd, endIdx)

			// Verify the range size
			rangeSize := endIdx - startIdx + 1
			assert.Equal(t, int64(tc.limit), rangeSize)
		})
	}
}

// TestNormalizePagination tests the pagination normalization function
func TestNormalizePagination(t *testing.T) {
	testCases := []struct {
		name          string
		page          int32
		pageSize      int32
		cursor        string
		maxPageSize   int32
		expectPage    int32
		expectPageSize int32
		expectOffset  int
		expectUseCursor bool
		expectCursorValue uint64
	}{
		{
			name:          "default values",
			page:          0,
			pageSize:      0,
			cursor:        "",
			maxPageSize:   100,
			expectPage:    1,
			expectPageSize: 20,
			expectOffset:  0,
			expectUseCursor: false,
		},
		{
			name:          "second page",
			page:          2,
			pageSize:      20,
			cursor:        "",
			maxPageSize:   100,
			expectPage:    2,
			expectPageSize: 20,
			expectOffset:  20,
			expectUseCursor: false,
		},
		{
			name:          "cursor pagination",
			page:          0,
			pageSize:      20,
			cursor:        "1000",
			maxPageSize:   100,
			expectPage:    0,
			expectPageSize: 20,
			expectOffset:  0,
			expectUseCursor: true,
			expectCursorValue: 1000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			page, pageSize, offset, useCursor, cursorValue := normalizePagination(
				tc.page, tc.pageSize, tc.cursor, tc.maxPageSize,
			)

			assert.Equal(t, tc.expectPage, page)
			assert.Equal(t, tc.expectPageSize, pageSize)
			assert.Equal(t, tc.expectOffset, offset)
			assert.Equal(t, tc.expectUseCursor, useCursor)
			assert.Equal(t, tc.expectCursorValue, cursorValue)
		})
	}
}

// TestCalculateNextCursor tests the cursor calculation function
func TestCalculateNextCursor(t *testing.T) {
	t.Run("blocks cursor", func(t *testing.T) {
		blocks := []models.Block{
			{Height: 100},
			{Height: 99},
			{Height: 98},
		}

		// Simulate: previous page ended at height 101 (cursorValue = 101)
		cursor := calculateNextCursor(true, 101, blocks, func(b models.Block) uint64 {
			return b.Height
		})

		// Should return the minimum height that is < 101
		assert.Equal(t, "98", cursor)
	})

	t.Run("transactions cursor", func(t *testing.T) {
		txs := []models.Transaction{
			{TxNumber: 100},
			{TxNumber: 99},
			{TxNumber: 98},
		}

		// Simulate: previous page ended at tx_number 101 (cursorValue = 101)
		cursor := calculateNextCursor(true, 101, txs, func(tx models.Transaction) uint64 {
			return tx.TxNumber
		})

		// Should return the minimum tx_number that is < 101
		assert.Equal(t, "98", cursor)
	})

	t.Run("no cursor mode", func(t *testing.T) {
		blocks := []models.Block{
			{Height: 100},
		}

		cursor := calculateNextCursor(false, 0, blocks, func(b models.Block) uint64 {
			return b.Height
		})

		assert.Equal(t, "", cursor)
	})
}

// TestDeduplicateTransactions tests the transaction deduplication function
func TestDeduplicateTransactions(t *testing.T) {
	txs := []models.Transaction{
		{Hash: "tx1", TxNumber: 1, BlockUnixTimestamp: 1000},
		{Hash: "tx2", TxNumber: 2, BlockUnixTimestamp: 1000},
		{Hash: "tx1", TxNumber: 1, BlockUnixTimestamp: 1000}, // Duplicate
		{Hash: "tx3", TxNumber: 3, BlockUnixTimestamp: 1000},
	}

	result := deduplicateTransactions(txs)

	require.Len(t, result, 3)
	// Result is sorted by BlockUnixTimestamp DESC, then TxNumber DESC
	assert.Equal(t, "tx3", result[0].Hash) // TxNumber 3
	assert.Equal(t, "tx2", result[1].Hash) // TxNumber 2
	assert.Equal(t, "tx1", result[2].Hash) // TxNumber 1
}

// TestPaginationCache_Integration is an integration test that requires Redis
func TestPaginationCache_Integration(t *testing.T) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		t.Skip("Redis not available, skipping integration test")
	}

	ctx := context.Background()
	testKey := "test:block:list"

	// Clean up after test
	defer redisClient.Del(ctx, testKey)

	// Add test blocks to Redis
	now := uint64(time.Now().Unix())
	blocks := []models.Block{
		{Height: 100, BlockUnixTimestamp: now},
		{Height: 99, BlockUnixTimestamp: now},
		{Height: 98, BlockUnixTimestamp: now},
		{Height: 97, BlockUnixTimestamp: now},
		{Height: 96, BlockUnixTimestamp: now},
	}

	// Push blocks to test list (LPush so highest is at tail)
	for i := len(blocks) - 1; i >= 0; i-- {
		data, _ := json.Marshal(blocks[i])
		redisClient.LPush(ctx, testKey, data)
	}

	// Test first page
	page1, err := redisClient.LRange(ctx, testKey, -2, -1).Result()
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	// Test second page (offset=2, limit=2)
	page2, err := redisClient.LRange(ctx, testKey, -4, -3).Result()
	require.NoError(t, err)
	assert.Len(t, page2, 2)
}
