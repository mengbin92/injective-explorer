// Package service provides integration tests for the Wasm contract APIs.
package service

import (
	"context"
	"testing"
	"time"

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

// setupWasmTestData creates test data for Wasm contract tests
func setupWasmTestData(t *testing.T) func() {
	// Initialize global logger if not set
	if global.Logger == nil {
		global.Logger = log.NewHelper(log.DefaultLogger)
	}

	// Check if database is initialized, if not create one
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
			&models.ContractInfo{},
			&models.CW20Balance{},
			&models.WasmCode{},
			&models.ContractExecution{},
		)
		require.NoError(t, err, "failed to migrate test database")

		// Set test database
		db.SetForTesting(testDB)
	}

	// Create test contract
	now := time.Now()
	codeID := int64(1)
	blockHeight := int64(1000)

	testContract := models.ContractInfo{
		ContractAddress: "inj1test123",
		Name:            "Test Contract",
		ContractType:    "cw20",
		ChainID:         "test-chain",
		Creator:         "inj1creator",
		TokenName:       "Test Token",
		TokenSymbol:     "TEST",
		Decimals:        18,
		CodeID:          &codeID,
		Label:           "test-label",
		BlockHeight:     &blockHeight,
		TxHash:          "test-tx-hash",
		InstantiatedAt:  &now,
		TotalSupply:     "1000000000000000000000",
	}

	err := db.Get().Create(&testContract).Error
	require.NoError(t, err, "failed to create test contract")

	// Create test CW20 balance
	testBalance := models.CW20Balance{
		ContractAddress: "inj1test123",
		OwnerAddress:    "inj1owner",
		Balance:         "100000000000000000000",
		UpdatedAt:       &now,
	}

	err = db.Get().Create(&testBalance).Error
	require.NoError(t, err, "failed to create test balance")

	// Create test Wasm code
	version := "1.0.0"
	testCode := models.WasmCode{
		CodeID:       1,
		Creator:      "inj1creator",
		DataHash:     "abc123",
		ContractType: "cw20",
		Version:      &version,
		Verified:     true,
		BlockHeight:  1000,
	}

	err = db.Get().Create(&testCode).Error
	require.NoError(t, err, "failed to create test code")

	// Return cleanup function
	return func() {
		// Clean up test data
		db.Get().Where("contract_address = ?", "inj1test123").Unscoped().Delete(&models.ContractInfo{})
		db.Get().Where("contract_address = ?", "inj1test123").Unscoped().Delete(&models.CW20Balance{})
		db.Get().Where("code_id = ?", 1).Unscoped().Delete(&models.WasmCode{})
	}
}

func TestGetWasmContracts(t *testing.T) {
	cleanup := setupWasmTestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get all contracts", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		assert.GreaterOrEqual(t, len(contracts), 1)
	})

	t.Run("filter by cw20 only", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Cw20Only: true,
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		for _, c := range contracts {
			contract := c.(map[string]interface{})
			assert.Equal(t, "cw20", contract["contract_type"])
		}
	})

	t.Run("pagination works correctly", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 5,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(5), pagination["page_size"])
	})

	// New test cases for edge cases and parameter validation

	t.Run("nil pagination should use defaults", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		// Should default to page 1 and page_size 20
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(20), pagination["page_size"])
	})

	t.Run("page 0 should default to 1", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     0,
				PageSize: 10,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
	})

	t.Run("negative page should default to 1", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     -5,
				PageSize: 10,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
	})

	t.Run("page_size 0 should default to 20", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 0,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(20), pagination["page_size"])
	})

	t.Run("negative page_size should default to 20", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: -10,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(20), pagination["page_size"])
	})

	t.Run("page_size > 100 should cap at 100", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 500,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(100), pagination["page_size"])
	})

	t.Run("filter by code_id", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			CodeId: 1,
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		for _, c := range contracts {
			contract := c.(map[string]interface{})
			assert.Equal(t, float64(1), contract["code_id"])
		}
	})

	t.Run("filter by creator", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Creator: "inj1creator",
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		for _, c := range contracts {
			contract := c.(map[string]interface{})
			assert.Equal(t, "inj1creator", contract["creator"])
		}
	})

	t.Run("filter by label - exact match", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Label: "test-label",
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		assert.GreaterOrEqual(t, len(contracts), 1)
		for _, c := range contracts {
			contract := c.(map[string]interface{})
			assert.Contains(t, contract["label"], "test-label")
		}
	})

	t.Run("filter by label - partial match", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Label: "label",
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		assert.GreaterOrEqual(t, len(contracts), 1)
		for _, c := range contracts {
			contract := c.(map[string]interface{})
			assert.Contains(t, contract["label"], "label")
		}
	})

	t.Run("multiple filters combined", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			CodeId:   1,
			Creator:  "inj1creator",
			Cw20Only: true,
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		for _, c := range contracts {
			contract := c.(map[string]interface{})
			assert.Equal(t, float64(1), contract["code_id"])
			assert.Equal(t, "inj1creator", contract["creator"])
			assert.Equal(t, "cw20", contract["contract_type"])
		}
	})

	t.Run("empty results with non-existent code_id filter", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			CodeId: 9999,
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		assert.Equal(t, 0, len(contracts))
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(0), pagination["total"])
	})

	t.Run("empty results with non-existent creator filter", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
			Creator: "inj1nonexistent",
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		assert.Equal(t, 0, len(contracts))
	})

	t.Run("pagination beyond last page", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     999,
				PageSize: 10,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		contracts := data["contracts"].([]interface{})
		assert.Equal(t, 0, len(contracts))
		pagination := data["pagination"].(map[string]interface{})
		assert.False(t, pagination["has_next"].(bool))
	})

	t.Run("last page has_next should be false", func(t *testing.T) {
		// First get total count
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 1,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		totalPages := int32(pagination["total_pages"].(float64))

		// Request last page
		req.Pagination.Page = totalPages
		resp, err = service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data = resp.Data.AsMap()
		pagination = data["pagination"].(map[string]interface{})
		assert.False(t, pagination["has_next"].(bool))
	})

	t.Run("first page has_prev should be false", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		assert.False(t, pagination["has_prev"].(bool))
	})

	t.Run("second page has_prev should be true", func(t *testing.T) {
		req := &pb.GetWasmContractsRequest{
			Pagination: &pb.PaginationRequest{
				Page:     2,
				PageSize: 1,
			},
		}

		resp, err := service.GetWasmContracts(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		pagination := data["pagination"].(map[string]interface{})
		// Only has_prev if total > 1
		total := pagination["total"].(float64)
		if total > 1 {
			assert.True(t, pagination["has_prev"].(bool))
		}
	})
}

func TestGetWasmContractByAddress(t *testing.T) {
	cleanup := setupWasmTestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get existing contract", func(t *testing.T) {
		req := &pb.GetWasmContractByAddressRequest{
			ContractAddress: "inj1test123",
		}

		resp, err := service.GetWasmContractByAddress(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		assert.Equal(t, "inj1test123", data["contract_address"])
		assert.Equal(t, "Test Token", data["token_name"])

		// Should include code info
		_, hasCodeInfo := data["code_info"]
		assert.True(t, hasCodeInfo)
	})

	t.Run("contract not found", func(t *testing.T) {
		req := &pb.GetWasmContractByAddressRequest{
			ContractAddress: "inj1nonexistent",
		}

		_, err := service.GetWasmContractByAddress(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	// New test cases for edge cases

	t.Run("empty contract_address", func(t *testing.T) {
		req := &pb.GetWasmContractByAddressRequest{
			ContractAddress: "",
		}

		_, err := service.GetWasmContractByAddress(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("code_info exists when code exists", func(t *testing.T) {
		req := &pb.GetWasmContractByAddressRequest{
			ContractAddress: "inj1test123",
		}

		resp, err := service.GetWasmContractByAddress(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		codeInfo, hasCodeInfo := data["code_info"]
		assert.True(t, hasCodeInfo)

		codeInfoMap := codeInfo.(map[string]interface{})
		assert.Equal(t, float64(1), codeInfoMap["code_id"])
		assert.Equal(t, "inj1creator", codeInfoMap["creator"])
		assert.Equal(t, "abc123", codeInfoMap["data_hash"])
		assert.Equal(t, "cw20", codeInfoMap["contract_type"])
		assert.True(t, codeInfoMap["verified"].(bool))
	})

	t.Run("code_info when code doesn't exist", func(t *testing.T) {
		// Create a contract without a valid code_id
		now := time.Now()
		codeID := int64(999)
		testContract := models.ContractInfo{
			ContractAddress: "inj1nocode",
			Name:            "No Code Contract",
			ContractType:    "cw20",
			ChainID:         "test-chain",
			Creator:         "inj1creator",
			CodeID:          &codeID,
			InstantiatedAt:  &now,
		}

		err := db.Get().Create(&testContract).Error
		require.NoError(t, err)
		defer db.Get().Where("contract_address = ?", "inj1nocode").Unscoped().Delete(&models.ContractInfo{})

		req := &pb.GetWasmContractByAddressRequest{
			ContractAddress: "inj1nocode",
		}

		resp, err := service.GetWasmContractByAddress(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		_, hasCodeInfo := data["code_info"]
		// code_info should not exist when code record not found
		assert.False(t, hasCodeInfo)
	})

	t.Run("recent_executions when executions exist", func(t *testing.T) {
		// Create test executions
		now := time.Now()
		executions := []models.ContractExecution{
			{
				ContractAddress: "inj1test123",
				TxHash:          "tx1",
				Sender:          "inj1sender1",
				BlockHeight:     1001,
				UnixTimestamp:   now.Unix(),
				Success:         true,
			},
			{
				ContractAddress: "inj1test123",
				TxHash:          "tx2",
				Sender:          "inj1sender2",
				BlockHeight:     1002,
				UnixTimestamp:   now.Unix() + 10,
				Success:         true,
			},
		}

		for _, exec := range executions {
			err := db.Get().Create(&exec).Error
			require.NoError(t, err)
		}
		defer db.Get().Where("contract_address = ?", "inj1test123").Unscoped().Delete(&models.ContractExecution{})

		req := &pb.GetWasmContractByAddressRequest{
			ContractAddress: "inj1test123",
		}

		resp, err := service.GetWasmContractByAddress(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		recentExecs, hasExecs := data["recent_executions"]
		assert.True(t, hasExecs)

		execsList := recentExecs.([]interface{})
		assert.Equal(t, 2, len(execsList))

		// Check first execution (should be most recent)
		exec := execsList[0].(map[string]interface{})
		assert.Equal(t, "tx2", exec["tx_hash"])
		assert.Equal(t, "inj1sender2", exec["sender"])
		assert.True(t, exec["success"].(bool))
	})

	t.Run("no recent_executions when none exist", func(t *testing.T) {
		// Create a contract without executions
		now := time.Now()
		codeID := int64(1)
		testContract := models.ContractInfo{
			ContractAddress: "inj1noexec",
			Name:            "No Exec Contract",
			ContractType:    "cw20",
			ChainID:         "test-chain",
			Creator:         "inj1creator",
			CodeID:          &codeID,
			InstantiatedAt:  &now,
		}

		err := db.Get().Create(&testContract).Error
		require.NoError(t, err)
		defer db.Get().Where("contract_address = ?", "inj1noexec").Unscoped().Delete(&models.ContractInfo{})

		req := &pb.GetWasmContractByAddressRequest{
			ContractAddress: "inj1noexec",
		}

		resp, err := service.GetWasmContractByAddress(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		_, hasExecs := data["recent_executions"]
		// recent_executions should not be present when there are none
		assert.False(t, hasExecs)
	})

	t.Run("contract with nil code_id", func(t *testing.T) {
		// Create a contract without code_id
		now := time.Now()
		testContract := models.ContractInfo{
			ContractAddress: "inj1nilcode",
			Name:            "Nil Code Contract",
			ContractType:    "other",
			ChainID:         "test-chain",
			Creator:         "inj1creator",
			CodeID:          nil,
			InstantiatedAt:  &now,
		}

		err := db.Get().Create(&testContract).Error
		require.NoError(t, err)
		defer db.Get().Where("contract_address = ?", "inj1nilcode").Unscoped().Delete(&models.ContractInfo{})

		req := &pb.GetWasmContractByAddressRequest{
			ContractAddress: "inj1nilcode",
		}

		resp, err := service.GetWasmContractByAddress(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		_, hasCodeInfo := data["code_info"]
		// code_info should not exist when code_id is nil
		assert.False(t, hasCodeInfo)
	})
}

func TestGetCW20Balance(t *testing.T) {
	cleanup := setupWasmTestData(t)
	defer cleanup()

	service := &ExplorerService{}
	ctx := context.Background()

	t.Run("get balances for address", func(t *testing.T) {
		req := &pb.GetCW20BalanceRequest{
			Address: "inj1owner",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(200), resp.Code)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		assert.GreaterOrEqual(t, len(balances), 1)

		balance := balances[0].(map[string]interface{})
		assert.Equal(t, "inj1test123", balance["contract_address"])
		assert.Equal(t, "Test Token", balance["token_name"])
		assert.Equal(t, "TEST", balance["token_symbol"])
	})

	t.Run("filter by contract address", func(t *testing.T) {
		req := &pb.GetCW20BalanceRequest{
			Address:         "inj1owner",
			ContractAddress: "inj1test123",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		for _, b := range balances {
			balance := b.(map[string]interface{})
			assert.Equal(t, "inj1test123", balance["contract_address"])
		}
	})

	t.Run("no balances found", func(t *testing.T) {
		req := &pb.GetCW20BalanceRequest{
			Address: "inj1nobalance",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		assert.Equal(t, 0, len(balances))
	})

	// New test cases for edge cases

	t.Run("empty address", func(t *testing.T) {
		req := &pb.GetCW20BalanceRequest{
			Address: "",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		// Should return empty results for empty address
		assert.Equal(t, 0, len(balances))
	})

	t.Run("non_zero_only filter with zero balances", func(t *testing.T) {
		// Create a zero balance
		now := time.Now()
		zeroBalance := models.CW20Balance{
			ContractAddress: "inj1test123",
			OwnerAddress:    "inj1zeroowner",
			Balance:         "0",
			UpdatedAt:       &now,
		}

		err := db.Get().Create(&zeroBalance).Error
		require.NoError(t, err)
		defer db.Get().Where("owner_address = ?", "inj1zeroowner").Unscoped().Delete(&models.CW20Balance{})

		// Request with non_zero_only = true
		req := &pb.GetCW20BalanceRequest{
			Address:     "inj1zeroowner",
			NonZeroOnly: true,
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		// Should filter out zero balance
		assert.Equal(t, 0, len(balances))
	})

	t.Run("non_zero_only filter with non-zero balances", func(t *testing.T) {
		req := &pb.GetCW20BalanceRequest{
			Address:     "inj1owner",
			NonZeroOnly: true,
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		// Should include non-zero balance
		assert.GreaterOrEqual(t, len(balances), 1)

		for _, b := range balances {
			balance := b.(map[string]interface{})
			assert.NotEqual(t, "0", balance["balance"])
		}
	})

	t.Run("when contract metadata doesn't exist", func(t *testing.T) {
		// Create a balance for a non-existent contract
		now := time.Now()
		orphanBalance := models.CW20Balance{
			ContractAddress: "inj1orphan",
			OwnerAddress:    "inj1orphanowner",
			Balance:         "1000",
			UpdatedAt:       &now,
		}

		err := db.Get().Create(&orphanBalance).Error
		require.NoError(t, err)
		defer db.Get().Where("owner_address = ?", "inj1orphanowner").Unscoped().Delete(&models.CW20Balance{})

		req := &pb.GetCW20BalanceRequest{
			Address: "inj1orphanowner",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		assert.Equal(t, 1, len(balances))

		balance := balances[0].(map[string]interface{})
		assert.Equal(t, "inj1orphan", balance["contract_address"])
		// Token info should not be present
		_, hasTokenName := balance["token_name"]
		assert.False(t, hasTokenName)
		_, hasTokenSymbol := balance["token_symbol"]
		assert.False(t, hasTokenSymbol)
		_, hasDecimals := balance["decimals"]
		assert.False(t, hasDecimals)
	})

	t.Run("multiple balances for same owner", func(t *testing.T) {
		// Create additional test data
		now := time.Now()
		codeID := int64(2)
		blockHeight := int64(2000)

		// Create second contract
		contract2 := models.ContractInfo{
			ContractAddress: "inj1test456",
			Name:            "Test Contract 2",
			ContractType:    "cw20",
			ChainID:         "test-chain",
			Creator:         "inj1creator",
			TokenName:       "Test Token 2",
			TokenSymbol:     "TEST2",
			Decimals:        6,
			CodeID:          &codeID,
			BlockHeight:     &blockHeight,
			InstantiatedAt:  &now,
		}

		err := db.Get().Create(&contract2).Error
		require.NoError(t, err)
		defer db.Get().Where("contract_address = ?", "inj1test456").Unscoped().Delete(&models.ContractInfo{})

		// Create second balance
		balance2 := models.CW20Balance{
			ContractAddress: "inj1test456",
			OwnerAddress:    "inj1owner",
			Balance:         "200000000",
			UpdatedAt:       &now,
		}

		err = db.Get().Create(&balance2).Error
		require.NoError(t, err)
		defer db.Get().Where("contract_address = ?", "inj1test456").Unscoped().Delete(&models.CW20Balance{})

		req := &pb.GetCW20BalanceRequest{
			Address: "inj1owner",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		assert.GreaterOrEqual(t, len(balances), 2)

		// Check total count
		total := data["total"].(float64)
		assert.Equal(t, float64(len(balances)), total)
	})

	t.Run("filter by specific contract with multiple balances", func(t *testing.T) {
		req := &pb.GetCW20BalanceRequest{
			Address:         "inj1owner",
			ContractAddress: "inj1test123",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})

		for _, b := range balances {
			balance := b.(map[string]interface{})
			assert.Equal(t, "inj1test123", balance["contract_address"])
		}
	})

	t.Run("balance includes updated_at timestamp", func(t *testing.T) {
		req := &pb.GetCW20BalanceRequest{
			Address: "inj1owner",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		assert.GreaterOrEqual(t, len(balances), 1)

		balance := balances[0].(map[string]interface{})
		_, hasUpdatedAt := balance["updated_at"]
		assert.True(t, hasUpdatedAt)
		// Verify it's a valid Unix timestamp
		updatedAt := balance["updated_at"].(float64)
		assert.Greater(t, updatedAt, float64(0))
	})

	t.Run("balance includes all required fields", func(t *testing.T) {
		req := &pb.GetCW20BalanceRequest{
			Address: "inj1owner",
		}

		resp, err := service.GetCW20Balance(ctx, req)
		require.NoError(t, err)

		data := resp.Data.AsMap()
		balances := data["balances"].([]interface{})
		assert.GreaterOrEqual(t, len(balances), 1)

		balance := balances[0].(map[string]interface{})
		// Verify all required fields exist
		assert.Contains(t, balance, "contract_address")
		assert.Contains(t, balance, "owner_address")
		assert.Contains(t, balance, "balance")
		assert.Contains(t, balance, "updated_at")
		assert.Equal(t, "inj1owner", balance["owner_address"])
	})
}
