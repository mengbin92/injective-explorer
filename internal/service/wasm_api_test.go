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

// setupWasmPhase3TestData creates test data for Wasm Phase 3 API tests
func setupWasmPhase3TestData(t *testing.T) func() {
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
			&models.WasmCode{},
			&models.ContractExecution{},
		)
		require.NoError(t, err, "failed to migrate test database")

		db.SetForTesting(testDB)
	}

	// Create test wasm codes
	txHash1 := "abc123"
	txHash2 := "def456"
	version1 := "1.0.0"
	sourceURL1 := "https://github.com/example/cw20"

	codes := []models.WasmCode{
		{
			Model:                 gorm.Model{ID: 1},
			CodeID:                100,
			Creator:               "inj1creator1",
			DataHash:              "hash100",
			InstantiatePermission: `{"permission":"everybody"}`,
			TxHash:                &txHash1,
			BlockHeight:           1000,
			ContractType:          "cw20",
			Version:               &version1,
			SourceURL:             &sourceURL1,
			Verified:              true,
			InstantiateCount:      10,
		},
		{
			Model:                 gorm.Model{ID: 2},
			CodeID:                101,
			Creator:               "inj1creator1",
			DataHash:              "hash101",
			InstantiatePermission: `{"permission":"nobody"}`,
			TxHash:                &txHash2,
			BlockHeight:           1100,
			ContractType:          "cw721",
			Verified:              false,
			InstantiateCount:      5,
		},
		{
			Model:                 gorm.Model{ID: 3},
			CodeID:                102,
			Creator:               "inj1creator2",
			DataHash:              "hash102",
			InstantiatePermission: "",
			BlockHeight:           1200,
			ContractType:          "cw20",
			Verified:              true,
			InstantiateCount:      20,
		},
	}

	for _, code := range codes {
		err := db.Get().Create(&code).Error
		require.NoError(t, err, "failed to create test wasm code")
	}

	// Create test contract executions
	executions := []models.ContractExecution{
		{
			ID:              1,
			ContractAddress: "inj1contract1",
			TxHash:          "tx100",
			Sender:          "inj1sender1",
			Msg:             `{"transfer":{"recipient":"inj1receiver","amount":"1000"}}`,
			Funds:           `[{"denom":"uinj","amount":"100"}]`,
			BlockHeight:     2000,
			UnixTimestamp:   1704067200000, // 2024-01-01 00:00:00
			Success:         true,
		},
		{
			ID:              2,
			ContractAddress: "inj1contract1",
			TxHash:          "tx101",
			Sender:          "inj1sender2",
			Msg:             `{"mint":{"amount":"5000"}}`,
			Funds:           "",
			BlockHeight:     2100,
			UnixTimestamp:   1704153600000, // 2024-01-02 00:00:00
			Success:         true,
		},
		{
			ID:              3,
			ContractAddress: "inj1contract1",
			TxHash:          "tx102",
			Sender:          "inj1sender1",
			Msg:             `{"burn":{"amount":"200"}}`,
			Funds:           "",
			BlockHeight:     2200,
			UnixTimestamp:   1704240000000, // 2024-01-03 00:00:00
			Success:         false,
		},
		{
			ID:              4,
			ContractAddress: "inj1contract2",
			TxHash:          "tx103",
			Sender:          "inj1sender3",
			Msg:             `{"execute":{}}`,
			Funds:           "",
			BlockHeight:     2300,
			UnixTimestamp:   1704326400000, // 2024-01-04 00:00:00
			Success:         true,
		},
	}

	for _, exec := range executions {
		err := db.Get().Create(&exec).Error
		require.NoError(t, err, "failed to create test contract execution")
	}

	// Return cleanup function
	return func() {
		db.Get().Unscoped().Where("1=1").Delete(&models.WasmCode{})
		db.Get().Unscoped().Where("1=1").Delete(&models.ContractExecution{})
	}
}

func TestGetWasmCodes(t *testing.T) {
	cleanup := setupWasmPhase3TestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get all codes with pagination", func(t *testing.T) {
		req := &pb.GetWasmCodesRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		resp, err := service.GetWasmCodes(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Data)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		require.Contains(t, data, "codes")
		require.Contains(t, data, "pagination")

		codes := data["codes"].([]interface{})
		assert.Equal(t, 3, len(codes))

		// Verify ordering (newest first, code_id DESC)
		firstCode := codes[0].(map[string]interface{})
		assert.Equal(t, float64(102), firstCode["code_id"])
	})

	t.Run("filter by creator", func(t *testing.T) {
		req := &pb.GetWasmCodesRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Creator: "inj1creator1",
		}

		resp, err := service.GetWasmCodes(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		codes := data["codes"].([]interface{})
		assert.Equal(t, 2, len(codes))

		for _, c := range codes {
			code := c.(map[string]interface{})
			assert.Equal(t, "inj1creator1", code["creator"])
		}
	})

	t.Run("filter by contract_type", func(t *testing.T) {
		req := &pb.GetWasmCodesRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			ContractType: "cw20",
		}

		resp, err := service.GetWasmCodes(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		codes := data["codes"].([]interface{})
		assert.Equal(t, 2, len(codes))

		for _, c := range codes {
			code := c.(map[string]interface{})
			assert.Equal(t, "cw20", code["contract_type"])
		}
	})

	t.Run("page_size > 100 caps at 100", func(t *testing.T) {
		req := &pb.GetWasmCodesRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 200,
			},
		}

		resp, err := service.GetWasmCodes(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(100), pagination["page_size"])
	})

	t.Run("nil pagination uses defaults", func(t *testing.T) {
		req := &pb.GetWasmCodesRequest{
			Pagination: &pb.PaginationRequest{},
		}

		resp, err := service.GetWasmCodes(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(20), pagination["page_size"])
	})

	t.Run("empty result for non-existent creator", func(t *testing.T) {
		req := &pb.GetWasmCodesRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Creator: "inj1nonexistent",
		}

		resp, err := service.GetWasmCodes(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		codes := data["codes"].([]interface{})
		assert.Equal(t, 0, len(codes))
	})

	t.Run("multiple filters combined", func(t *testing.T) {
		req := &pb.GetWasmCodesRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Creator:      "inj1creator1",
			ContractType: "cw20",
		}

		resp, err := service.GetWasmCodes(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		codes := data["codes"].([]interface{})
		assert.Equal(t, 1, len(codes))

		code := codes[0].(map[string]interface{})
		assert.Equal(t, "inj1creator1", code["creator"])
		assert.Equal(t, "cw20", code["contract_type"])
	})
}

func TestGetWasmCodeByID(t *testing.T) {
	cleanup := setupWasmPhase3TestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get existing code by code_id", func(t *testing.T) {
		req := &pb.GetWasmCodeByIDRequest{
			CodeId: 100,
		}

		resp, err := service.GetWasmCodeByID(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		assert.Equal(t, float64(100), data["code_id"])
		assert.Equal(t, "inj1creator1", data["creator"])
		assert.Equal(t, "cw20", data["contract_type"])
		assert.Equal(t, true, data["verified"])
	})

	t.Run("code not found returns 404", func(t *testing.T) {
		req := &pb.GetWasmCodeByIDRequest{
			CodeId: 999,
		}

		_, err := service.GetWasmCodeByID(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("instantiate_permission parsed correctly", func(t *testing.T) {
		req := &pb.GetWasmCodeByIDRequest{
			CodeId: 100,
		}

		resp, err := service.GetWasmCodeByID(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		require.Contains(t, data, "instantiate_permission")

		perm := data["instantiate_permission"].(map[string]interface{})
		assert.Equal(t, "everybody", perm["permission"])
	})
}

func TestGetContractTxs(t *testing.T) {
	cleanup := setupWasmPhase3TestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get all executions for contract with pagination", func(t *testing.T) {
		req := &pb.GetContractTxsRequest{
			ContractAddress: "inj1contract1",
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		resp, err := service.GetContractTxs(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		require.Contains(t, data, "executions")
		require.Contains(t, data, "pagination")

		executions := data["executions"].([]interface{})
		assert.Equal(t, 3, len(executions))

		// Verify ordering (newest first, timestamp DESC)
		firstExec := executions[0].(map[string]interface{})
		assert.Equal(t, "tx102", firstExec["tx_hash"])
	})

	t.Run("filter by sender", func(t *testing.T) {
		req := &pb.GetContractTxsRequest{
			ContractAddress: "inj1contract1",
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Sender: "inj1sender1",
		}

		resp, err := service.GetContractTxs(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		executions := data["executions"].([]interface{})
		assert.Equal(t, 2, len(executions))

		for _, e := range executions {
			exec := e.(map[string]interface{})
			assert.Equal(t, "inj1sender1", exec["sender"])
		}
	})

	t.Run("filter by start_time", func(t *testing.T) {
		req := &pb.GetContractTxsRequest{
			ContractAddress: "inj1contract1",
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			StartTime: 1704153600000, // 2024-01-02 00:00:00
		}

		resp, err := service.GetContractTxs(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		executions := data["executions"].([]interface{})
		assert.Equal(t, 2, len(executions)) // tx101 and tx102
	})

	t.Run("filter by end_time", func(t *testing.T) {
		req := &pb.GetContractTxsRequest{
			ContractAddress: "inj1contract1",
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			EndTime: 1704153600000, // 2024-01-02 00:00:00
		}

		resp, err := service.GetContractTxs(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		executions := data["executions"].([]interface{})
		assert.Equal(t, 2, len(executions)) // tx100 and tx101
	})

	t.Run("filter by time range (both start and end)", func(t *testing.T) {
		req := &pb.GetContractTxsRequest{
			ContractAddress: "inj1contract1",
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			StartTime: 1704153600000, // 2024-01-02 00:00:00
			EndTime:   1704240000000, // 2024-01-03 00:00:00
		}

		resp, err := service.GetContractTxs(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		executions := data["executions"].([]interface{})
		assert.Equal(t, 2, len(executions)) // tx101 and tx102
	})

	t.Run("missing contract_address returns error", func(t *testing.T) {
		req := &pb.GetContractTxsRequest{
			ContractAddress: "",
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		_, err := service.GetContractTxs(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("empty result for non-existent contract", func(t *testing.T) {
		req := &pb.GetContractTxsRequest{
			ContractAddress: "inj1nonexistent",
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		resp, err := service.GetContractTxs(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		executions := data["executions"].([]interface{})
		assert.Equal(t, 0, len(executions))
	})
}
