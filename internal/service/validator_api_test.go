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

// setupValidatorTestData creates test data for validator tests
func setupValidatorTestData(t *testing.T) func() {
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
			&models.Validator{},
			&models.ValidatorBlockStats{},
		)
		require.NoError(t, err, "failed to migrate test database")

		db.SetForTesting(testDB)
	}

	// Create test validators
	validators := []models.Validator{
		{
			Model:               gorm.Model{ID: 1},
			Id:                  "val1",
			Moniker:             "Test Validator 1",
			OperatorAddress:     "injvaloper1test1",
			ConsensusAddress:    "injvalcons1test1",
			Status:              3, // BONDED
			Jailed:              false,
			Tokens:              "1000000000000",
			DelegatorShares:     "1000000000000",
			CommissionRate:      "0.10",
			CommissionMaxRate:   "0.20",
			CommissionMaxChangeRate: "0.01",
			UptimePercentage:    98.5,
			Description:         `{"moniker":"Test Validator 1","website":"https://test1.com"}`,
			SlashingEvents:      []string{},
		},
		{
			Model:               gorm.Model{ID: 2},
			Id:                  "val2",
			Moniker:             "Test Validator 2",
			OperatorAddress:     "injvaloper1test2",
			ConsensusAddress:    "injvalcons1test2",
			Status:              3, // BONDED
			Jailed:              false,
			Tokens:              "500000000000",
			DelegatorShares:     "500000000000",
			CommissionRate:      "0.05",
			CommissionMaxRate:   "0.15",
			CommissionMaxChangeRate: "0.01",
			UptimePercentage:    99.2,
		},
		{
			Model:               gorm.Model{ID: 3},
			Id:                  "val3",
			Moniker:             "Jailed Validator",
			OperatorAddress:     "injvaloper1test3",
			ConsensusAddress:    "injvalcons1test3",
			Status:              2, // UNBONDING
			Jailed:              true,
			Tokens:              "200000000000",
			DelegatorShares:     "200000000000",
			CommissionRate:      "0.15",
			CommissionMaxRate:   "0.25",
			CommissionMaxChangeRate: "0.02",
			UptimePercentage:    85.0,
		},
	}

	for _, val := range validators {
		err := db.Get().Create(&val).Error
		require.NoError(t, err, "failed to create test validator")
	}

	// Create test validator_block_stats
	stats := []models.ValidatorBlockStats{
		{
			ValidatorOperatorAddress: "injvaloper1test1",
			PeriodStartHeight:        1000,
			PeriodEndHeight:          2420,
			ProposedCount:            10,
			SignedCount:              1400,
			MissedCount:              20,
			ShouldSignCount:          1420,
		},
		{
			ValidatorOperatorAddress: "injvaloper1test1",
			PeriodStartHeight:        500,
			PeriodEndHeight:          1920,
			ProposedCount:            12,
			SignedCount:              1390,
			MissedCount:              30,
			ShouldSignCount:          1420,
		},
	}

	for _, stat := range stats {
		err := db.Get().Create(&stat).Error
		require.NoError(t, err, "failed to create test stats")
	}

	// Return cleanup function
	return func() {
		db.Get().Unscoped().Where("1=1").Delete(&models.Validator{})
		db.Get().Unscoped().Where("1=1").Delete(&models.ValidatorBlockStats{})
	}
}

func TestGetValidators(t *testing.T) {
	cleanup := setupValidatorTestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get all validators with pagination", func(t *testing.T) {
		req := &pb.GetValidatorsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		resp, err := service.GetValidators(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Data, "resp.Data should not be nil")
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		t.Logf("Data map: %+v", data)
		require.Contains(t, data, "validators")
		require.Contains(t, data, "pagination")

		validators := data["validators"].([]interface{})
		assert.GreaterOrEqual(t, len(validators), 1)
	})

	t.Run("filter by status bonded", func(t *testing.T) {
		req := &pb.GetValidatorsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Status: "bonded",
		}

		resp, err := service.GetValidators(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		validators := data["validators"].([]interface{})
		for _, v := range validators {
			val := v.(map[string]interface{})
			assert.Equal(t, "bonded", val["status"])
		}
	})

	t.Run("filter by jailed true", func(t *testing.T) {
		jailed := true
		req := &pb.GetValidatorsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Jailed: &jailed,
		}

		resp, err := service.GetValidators(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		validators := data["validators"].([]interface{})
		for _, v := range validators {
			val := v.(map[string]interface{})
			assert.True(t, val["jailed"].(bool))
		}
	})

	t.Run("search by moniker", func(t *testing.T) {
		req := &pb.GetValidatorsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Search: "Test Validator 1",
		}

		resp, err := service.GetValidators(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		validators := data["validators"].([]interface{})
		assert.GreaterOrEqual(t, len(validators), 1)
	})

	t.Run("filter by commission range", func(t *testing.T) {
		req := &pb.GetValidatorsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			MinCommission: "0.05",
			MaxCommission: "0.10",
		}

		resp, err := service.GetValidators(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)
	})

	t.Run("sort by tokens DESC", func(t *testing.T) {
		req := &pb.GetValidatorsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			OrderBy: "tokens DESC",
		}

		resp, err := service.GetValidators(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)
	})

	t.Run("nil pagination uses defaults", func(t *testing.T) {
		req := &pb.GetValidatorsRequest{
			Pagination: &pb.PaginationRequest{},
		}

		resp, err := service.GetValidators(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(20), pagination["page_size"])
	})

	t.Run("page_size > 100 caps at 100", func(t *testing.T) {
		req := &pb.GetValidatorsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 200,
			},
		}

		resp, err := service.GetValidators(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(100), pagination["page_size"])
	})
}

func TestGetValidator(t *testing.T) {
	cleanup := setupValidatorTestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get existing validator by operator_address", func(t *testing.T) {
		req := &pb.GetValidatorRequest{
			Address: "injvaloper1test1",
		}

		resp, err := service.GetValidator(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		assert.Equal(t, "injvaloper1test1", data["operator_address"])
		assert.Equal(t, "Test Validator 1", data["moniker"])
	})

	t.Run("get existing validator by consensus_address", func(t *testing.T) {
		req := &pb.GetValidatorRequest{
			Address: "injvalcons1test1",
		}

		resp, err := service.GetValidator(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		assert.Equal(t, "injvalcons1test1", data["consensus_address"])
	})

	t.Run("validator not found", func(t *testing.T) {
		req := &pb.GetValidatorRequest{
			Address: "injvaloper1nonexistent",
		}

		_, err := service.GetValidator(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("description decoded", func(t *testing.T) {
		req := &pb.GetValidatorRequest{
			Address: "injvaloper1test1",
		}

		resp, err := service.GetValidator(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		require.Contains(t, data, "description")
		desc := data["description"].(map[string]interface{})
		assert.Equal(t, "Test Validator 1", desc["moniker"])
	})

	t.Run("uptime trend included", func(t *testing.T) {
		req := &pb.GetValidatorRequest{
			Address: "injvaloper1test1",
		}

		resp, err := service.GetValidator(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		require.Contains(t, data, "uptime_trend")
	})
}

func TestGetValidatorUptime(t *testing.T) {
	cleanup := setupValidatorTestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("time-based query 7d", func(t *testing.T) {
		req := &pb.GetValidatorUptimeRequest{
			Address: "injvaloper1test1",
			Period:  "7d",
		}

		resp, err := service.GetValidatorUptime(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		assert.Equal(t, "7d", data["period"])
		require.Contains(t, data, "overall")
		require.Contains(t, data, "time_series")
	})

	t.Run("block-based query", func(t *testing.T) {
		req := &pb.GetValidatorUptimeRequest{
			Address: "injvaloper1test1",
			Blocks:  10000,
		}

		resp, err := service.GetValidatorUptime(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		assert.Contains(t, data["period"].(string), "blocks")
	})

	t.Run("default blocks when no params", func(t *testing.T) {
		req := &pb.GetValidatorUptimeRequest{
			Address: "injvaloper1test1",
		}

		resp, err := service.GetValidatorUptime(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)
	})

	t.Run("overall metrics calculated correctly", func(t *testing.T) {
		req := &pb.GetValidatorUptimeRequest{
			Address: "injvaloper1test1",
			Period:  "7d",
		}

		resp, err := service.GetValidatorUptime(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		overall := data["overall"].(map[string]interface{})
		assert.Contains(t, overall, "uptime_percentage")
		assert.Contains(t, overall, "signed_blocks")
		assert.Contains(t, overall, "missed_blocks")
	})
}
