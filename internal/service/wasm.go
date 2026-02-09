// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"errors"
	"net/http"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// GetWasmContracts retrieves a list of Wasm contracts with filtering and pagination.
func (s *ExplorerService) GetWasmContracts(ctx context.Context, req *pb.GetWasmContractsRequest) (*pb.Reply, error) {
	var contracts []models.ContractInfo

	query := db.Get().Model(&models.ContractInfo{})

	// Filter by code ID
	if req.CodeId > 0 {
		query = query.Where("code_id = ?", req.CodeId)
	}

	// Filter by creator
	if req.Creator != "" {
		query = query.Where("creator = ?", req.Creator)
	}

	// Filter by label (partial match)
	if req.Label != "" {
		query = query.Where("label LIKE ?", "%"+req.Label+"%")
	}

	// Filter CW20 only
	if req.Cw20Only {
		query = query.Where("contract_type = ?", "cw20")
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count contracts")
	}

	// Apply pagination
	page := req.Pagination.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.Pagination.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	query = query.Offset(int(offset)).Limit(int(pageSize))

	// Order by block height descending (newest first)
	query = query.Order("block_height DESC")

	// Execute query
	if err := query.Find(&contracts).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch contracts")
	}

	// Calculate pagination info
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

	// Convert to response format
	contractDTOs := make([]map[string]interface{}, len(contracts))
	for i, contract := range contracts {
		contractDTOs[i] = convertContractToDTO(&contract)
	}

	// Create response data map
	responseData := map[string]interface{}{
		"contracts": contractDTOs,
		"pagination": map[string]interface{}{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
			"has_prev":    page > 1,
			"has_next":    page < totalPages,
		},
	}

	// Convert to structpb.Struct
	structData, err := ToStruct(responseData)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to convert response to struct")
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    structData,
	}, nil
}

// convertContractToDTO converts a ContractInfo model to a DTO map.
func convertContractToDTO(contract *models.ContractInfo) map[string]interface{} {
	dto := map[string]interface{}{
		"contract_address": contract.ContractAddress,
		"name":             contract.Name,
		"contract_type":    contract.ContractType,
		"chain_id":         contract.ChainID,
		"creator":          contract.Creator,
		"decimals":         contract.Decimals,
	}

	// Add optional fields if present
	if contract.CodeID != nil {
		dto["code_id"] = *contract.CodeID
	}
	if contract.Label != "" {
		dto["label"] = contract.Label
	}
	if contract.BlockHeight != nil {
		dto["block_height"] = *contract.BlockHeight
	}
	if contract.TxHash != "" {
		dto["tx_hash"] = contract.TxHash
	}
	if contract.ExecuteCount > 0 {
		dto["execute_count"] = contract.ExecuteCount
	}
	if contract.LastExecutedAt != nil {
		dto["last_executed_at"] = contract.LastExecutedAt.Unix()
	}
	if contract.InstantiatedAt != nil {
		dto["instantiated_at"] = contract.InstantiatedAt.Unix()
	}

	// Add CW20-specific fields
	if contract.ContractType == "cw20" {
		dto["token_name"] = contract.TokenName
		dto["token_symbol"] = contract.TokenSymbol
		dto["total_supply"] = contract.TotalSupply
	}

	return dto
}

// GetWasmContractByAddress retrieves detailed information for a specific contract.
func (s *ExplorerService) GetWasmContractByAddress(ctx context.Context, req *pb.GetWasmContractByAddressRequest) (*pb.Reply, error) {
	var contract models.ContractInfo

	// Query contract by address
	if err := db.Get().Where("contract_address = ?", req.ContractAddress).First(&contract).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "contract not found")
		}
		return nil, status.Error(codes.Internal, "failed to fetch contract")
	}

	// Convert to DTO
	contractDTO := convertContractToDTO(&contract)

	// Fetch code information if available
	if contract.CodeID != nil && *contract.CodeID > 0 {
		var code models.WasmCode
		if err := db.Get().Where("code_id = ?", *contract.CodeID).First(&code).Error; err == nil {
			contractDTO["code_info"] = map[string]interface{}{
				"code_id":       code.CodeID,
				"creator":       code.Creator,
				"data_hash":     code.DataHash,
				"contract_type": code.ContractType,
				"version":       code.Version,
				"verified":      code.Verified,
			}
		}
	}

	// Fetch recent execution records (last 10)
	var recentExecutions []models.ContractExecution
	db.Get().Where("contract_address = ?", req.ContractAddress).
		Order("unix_timestamp DESC").
		Limit(10).
		Find(&recentExecutions)

	if len(recentExecutions) > 0 {
		execDTOs := make([]map[string]interface{}, len(recentExecutions))
		for i, exec := range recentExecutions {
			execDTOs[i] = map[string]interface{}{
				"tx_hash":        exec.TxHash,
				"sender":         exec.Sender,
				"block_height":   exec.BlockHeight,
				"unix_timestamp": exec.UnixTimestamp,
				"success":        exec.Success,
			}
		}
		contractDTO["recent_executions"] = execDTOs
	}

	// Convert to structpb.Struct
	structData, err := ToStruct(contractDTO)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to convert response to struct")
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    structData,
	}, nil
}

// GetCW20Balance retrieves CW20 token balances for an address.
func (s *ExplorerService) GetCW20Balance(ctx context.Context, req *pb.GetCW20BalanceRequest) (*pb.Reply, error) {
	var balances []models.CW20Balance

	query := db.Get().Model(&models.CW20Balance{}).
		Where("owner_address = ?", req.Address)

	// Filter by specific contract address
	if req.ContractAddress != "" {
		query = query.Where("contract_address = ?", req.ContractAddress)
	}

	// Filter non-zero balances only
	if req.NonZeroOnly {
		query = query.Where("CAST(balance AS DECIMAL) > 0")
	}

	// Execute query
	if err := query.Find(&balances).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch balances")
	}

	// Convert to DTOs with token metadata
	balanceDTOs := make([]map[string]interface{}, 0, len(balances))
	for _, balance := range balances {
		balanceDTO := map[string]interface{}{
			"contract_address": balance.ContractAddress,
			"owner_address":    balance.OwnerAddress,
			"balance":          balance.Balance,
			"updated_at":       balance.UpdatedAt.Unix(),
		}

		// Fetch token information from contract_infos
		var contract models.ContractInfo
		if err := db.Get().Where("contract_address = ? AND contract_type = ?",
			balance.ContractAddress, "cw20").First(&contract).Error; err == nil {
			balanceDTO["token_name"] = contract.TokenName
			balanceDTO["token_symbol"] = contract.TokenSymbol
			balanceDTO["decimals"] = contract.Decimals

			// Include contract metadata if available
			if contract.Metadata != nil {
				balanceDTO["metadata"] = contract.Metadata
			}
		}

		balanceDTOs = append(balanceDTOs, balanceDTO)
	}

	// Convert to structpb.Struct
	responseData := map[string]interface{}{
		"balances": balanceDTOs,
		"total":    len(balanceDTOs),
	}

	structData, err := ToStruct(responseData)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to convert response to struct")
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    structData,
	}, nil
}
