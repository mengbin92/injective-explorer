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

// setupRelayerTestData creates test data for Relayer API tests
func setupRelayerTestData(t *testing.T) func() {
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
			&models.Relayer{},
			&models.BankTransfer{},
		)
		require.NoError(t, err, "failed to migrate test database")

		db.SetForTesting(testDB)
	} else {
		// Database already exists, just ensure Relayer table is migrated
		err := db.Get().AutoMigrate(&models.Relayer{})
		require.NoError(t, err, "failed to migrate relayer table")
	}

	// Create test relayers
	relayers := []models.Relayer{
		{
			ID:                  1,
			MarketID:            "0xmarket1",
			RelayerAddress:      "inj1relayer1",
			TotalPacketsRelayed: 1000,
			TotalPacketsFailed:  10,
			SuccessRate:         99.0,
			AvgRelayTimeMs:      2500,
			TotalFeesEarned:     `[{"denom":"uinj","amount":"500000"}]`,
			ActiveChannels:      `["channel-0","channel-1"]`,
			LastActiveAt:        1704412800000, // 2024-01-05 00:00:00
		},
		{
			ID:                  2,
			MarketID:            "0xmarket1",
			RelayerAddress:      "inj1relayer2",
			TotalPacketsRelayed: 2000,
			TotalPacketsFailed:  5,
			SuccessRate:         99.75,
			AvgRelayTimeMs:      2000,
			TotalFeesEarned:     `[{"denom":"uinj","amount":"800000"}]`,
			ActiveChannels:      `["channel-0","channel-2"]`,
			LastActiveAt:        1704326400000, // 2024-01-04 00:00:00
		},
		{
			ID:                  3,
			MarketID:            "0xmarket2",
			RelayerAddress:      "inj1relayer3",
			TotalPacketsRelayed: 500,
			TotalPacketsFailed:  50,
			SuccessRate:         90.91,
			AvgRelayTimeMs:      3000,
			TotalFeesEarned:     `[{"denom":"uinj","amount":"200000"}]`,
			ActiveChannels:      `["channel-3"]`,
			LastActiveAt:        1704240000000, // 2024-01-03 00:00:00
		},
		{
			ID:                  4,
			MarketID:            "0xmarket2",
			RelayerAddress:      "inj1relayer4",
			TotalPacketsRelayed: 1500,
			TotalPacketsFailed:  15,
			SuccessRate:         99.01,
			AvgRelayTimeMs:      2200,
			TotalFeesEarned:     `[{"denom":"uinj","amount":"600000"},{"denom":"usdt","amount":"100"}]`,
			ActiveChannels:      `["channel-3","channel-4"]`,
			LastActiveAt:        1704153600000, // 2024-01-02 00:00:00
		},
	}

	for _, relayer := range relayers {
		err := db.Get().Create(&relayer).Error
		require.NoError(t, err, "failed to create test relayer")
	}

	// Return cleanup function
	return func() {
		db.Get().Unscoped().Where("1=1").Delete(&models.Relayer{})
	}
}

func TestGetRelayers(t *testing.T) {
	cleanup := setupRelayerTestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get all relayers with pagination", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Data)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		require.Contains(t, data, "relayers")
		require.Contains(t, data, "pagination")

		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 4, len(relayers))

		// Verify ordering (most recent activity first, last_active_at DESC)
		firstRelayer := relayers[0].(map[string]interface{})
		assert.Equal(t, "inj1relayer1", firstRelayer["relayer_address"])
	})

	t.Run("filter by market_id", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			MarketIds: []string{"0xmarket1"},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 2, len(relayers))

		for _, rl := range relayers {
			relayer := rl.(map[string]interface{})
			assert.Equal(t, "0xmarket1", relayer["market_id"])
		}
	})

	t.Run("filter by multiple market_ids", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			MarketIds: []string{"0xmarket1", "0xmarket2"},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 4, len(relayers))
	})

	t.Run("filter by relayer_address", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			RelayerAddresses: []string{"inj1relayer1"},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 1, len(relayers))

		relayer := relayers[0].(map[string]interface{})
		assert.Equal(t, "inj1relayer1", relayer["relayer_address"])
		assert.Equal(t, "0xmarket1", relayer["market_id"])
		assert.Equal(t, float64(1000), relayer["total_packets_relayed"])
		assert.Equal(t, 99.0, relayer["success_rate"])
	})

	t.Run("filter by multiple relayer_addresses", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			RelayerAddresses: []string{"inj1relayer1", "inj1relayer2"},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 2, len(relayers))
	})

	t.Run("combined filters: market_id and relayer_address", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			MarketIds:        []string{"0xmarket2"},
			RelayerAddresses: []string{"inj1relayer3"},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 1, len(relayers))

		relayer := relayers[0].(map[string]interface{})
		assert.Equal(t, "0xmarket2", relayer["market_id"])
		assert.Equal(t, "inj1relayer3", relayer["relayer_address"])
	})

	t.Run("verify JSON parsing for fees and channels", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			RelayerAddresses: []string{"inj1relayer4"},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 1, len(relayers))

		relayer := relayers[0].(map[string]interface{})

		// Check total_fees_earned is parsed as array
		fees := relayer["total_fees_earned"].([]interface{})
		assert.Equal(t, 2, len(fees))

		// Check active_channels is parsed as array
		channels := relayer["active_channels"].([]interface{})
		assert.Equal(t, 2, len(channels))
	})

	t.Run("page_size > 100 caps at 100", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 200,
			},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(100), pagination["page_size"])
	})

	t.Run("nil pagination uses defaults", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(20), pagination["page_size"])
	})

	t.Run("empty result for non-existent market_id", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			MarketIds: []string{"0xnonexistent"},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 0, len(relayers))
	})

	t.Run("verify pagination metadata", func(t *testing.T) {
		req := &pb.GetRelayersRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 2,
			},
		}

		resp, err := service.GetRelayers(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		relayers := data["relayers"].([]interface{})
		assert.Equal(t, 2, len(relayers))

		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(2), pagination["page_size"])
		assert.Equal(t, float64(4), pagination["total"])
		assert.Equal(t, float64(2), pagination["total_pages"])
		assert.Equal(t, false, pagination["has_prev"])
		assert.Equal(t, true, pagination["has_next"])
	})
}
