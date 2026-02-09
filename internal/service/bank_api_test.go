package service

import (
	"context"
	"testing"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupBankTestData creates test data for Bank API tests
func setupBankTestData(t *testing.T) func() {
	// Initialize global logger if not set
	if global.Logger == nil {
		global.Logger = log.NewHelper(log.DefaultLogger)
	}

	// Check if database is initialized
	var needsInit bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				needsInit = true
			}
		}()
		_ = db.Get()
	}()

	if needsInit {
		// Create test database
		testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err, "failed to connect test database")

		// Auto-migrate test tables
		err = testDB.AutoMigrate(
			&models.BankTransfer{},
		)
		require.NoError(t, err, "failed to migrate test database")

		db.SetForTesting(testDB)
	}

	// Create test bank transfers
	transfers := []models.BankTransfer{
		{
			ID:            1,
			TxHash:        "tx100",
			Sender:        "inj1sender1",
			Recipient:     "inj1recipient1",
			Amounts:       `[{"denom":"uinj","amount":"1000000"}]`,
			BlockHeight:   1000,
			UnixTimestamp: 1704067200000, // 2024-01-01 00:00:00
			Success:       true,
		},
		{
			ID:            2,
			TxHash:        "tx101",
			Sender:        "inj1sender1",
			Recipient:     "inj1recipient2",
			Amounts:       `[{"denom":"uinj","amount":"2000000"}]`,
			BlockHeight:   1100,
			UnixTimestamp: 1704153600000, // 2024-01-02 00:00:00
			Success:       true,
		},
		{
			ID:            3,
			TxHash:        "tx102",
			Sender:        "inj1sender2",
			Recipient:     "inj1recipient1",
			Amounts:       `[{"denom":"uinj","amount":"500000"}]`,
			BlockHeight:   1200,
			UnixTimestamp: 1704240000000, // 2024-01-03 00:00:00
			Success:       false,
		},
		{
			ID:            4,
			TxHash:        "tx103",
			Sender:        "inj1sender3",
			Recipient:     "inj1recipient3",
			Amounts:       `[{"denom":"usdt","amount":"10000"}]`,
			BlockHeight:   1300,
			UnixTimestamp: 1704326400000, // 2024-01-04 00:00:00
			Success:       true,
		},
		{
			ID:                     5,
			TxHash:                 "tx104",
			Sender:                 "inj1communitypool",
			Recipient:              "inj1recipient1",
			Amounts:                `[{"denom":"uinj","amount":"3000000"}]`,
			BlockHeight:            1400,
			UnixTimestamp:          1704412800000, // 2024-01-05 00:00:00
			Success:                true,
			IsCommunityPoolRelated: true,
		},
	}

	for _, transfer := range transfers {
		err := db.Get().Create(&transfer).Error
		require.NoError(t, err, "failed to create test bank transfer")
	}

	// Return cleanup function
	return func() {
		db.Get().Unscoped().Where("1=1").Delete(&models.BankTransfer{})
	}
}

func TestGetBankTransfers(t *testing.T) {
	cleanup := setupBankTestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get all transfers with pagination", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Data)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		require.Contains(t, data, "transfers")
		require.Contains(t, data, "pagination")

		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 5, len(transfers))

		// Verify ordering (newest first, unix_timestamp DESC)
		firstTransfer := transfers[0].(map[string]interface{})
		assert.Equal(t, "tx104", firstTransfer["tx_hash"])
	})

	t.Run("filter by sender", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Senders: []string{"inj1sender1"},
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 2, len(transfers))

		for _, tr := range transfers {
			transfer := tr.(map[string]interface{})
			assert.Equal(t, "inj1sender1", transfer["sender"])
		}
	})

	t.Run("filter by recipient", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Recipients: []string{"inj1recipient1"},
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 3, len(transfers))

		for _, tr := range transfers {
			transfer := tr.(map[string]interface{})
			assert.Equal(t, "inj1recipient1", transfer["recipient"])
		}
	})

	t.Run("filter by address (sender or recipient)", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Addresses: []string{"inj1sender1"},
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 2, len(transfers))
	})

	t.Run("filter by start_time", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			StartTime: 1704240000000, // 2024-01-03 00:00:00
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 3, len(transfers)) // tx102, tx103, tx104
	})

	t.Run("filter by end_time", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			EndTime: 1704240000000, // 2024-01-03 00:00:00
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 3, len(transfers)) // tx100, tx101, tx102
	})

	t.Run("filter by time range", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			StartTime: 1704153600000, // 2024-01-02 00:00:00
			EndTime:   1704326400000, // 2024-01-04 00:00:00
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 3, len(transfers)) // tx101, tx102, tx103
	})

	t.Run("filter by denom", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Denom: "usdt",
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 1, len(transfers))

		transfer := transfers[0].(map[string]interface{})
		assert.Equal(t, "tx103", transfer["tx_hash"])
	})

	t.Run("filter by community pool related", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			IsCommunityPoolRelated: true,
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 1, len(transfers))

		transfer := transfers[0].(map[string]interface{})
		assert.Equal(t, "tx104", transfer["tx_hash"])
		assert.Equal(t, true, transfer["is_community_pool_related"])
	})

	t.Run("page_size > 100 caps at 100", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 200,
			},
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(100), pagination["page_size"])
	})

	t.Run("nil pagination uses defaults", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{},
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(20), pagination["page_size"])
	})

	t.Run("empty result for non-existent sender", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Senders: []string{"inj1nonexistent"},
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 0, len(transfers))
	})

	t.Run("multiple filters combined", func(t *testing.T) {
		req := &pb.GetBankTransfersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Senders:   []string{"inj1sender1"},
			StartTime: 1704153600000, // 2024-01-02 00:00:00
		}

		resp, err := service.GetBankTransfers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		transfers := data["transfers"].([]interface{})
		assert.Equal(t, 1, len(transfers))

		transfer := transfers[0].(map[string]interface{})
		assert.Equal(t, "tx101", transfer["tx_hash"])
		assert.Equal(t, "inj1sender1", transfer["sender"])
	})
}
