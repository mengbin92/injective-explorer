// Package service provides Wasm API implementations.
package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/bytedance/sonic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
)

// convertWasmCodeToDTO converts WasmCode model to DTO
func convertWasmCodeToDTO(code *models.WasmCode) map[string]interface{} {
	dto := map[string]interface{}{
		"code_id":           float64(code.CodeID),
		"creator":           code.Creator,
		"data_hash":         code.DataHash,
		"contract_type":     code.ContractType,
		"verified":          code.Verified,
		"instantiate_count": float64(code.InstantiateCount),
		"block_height":      float64(code.BlockHeight),
	}

	// Optional fields
	if code.TxHash != nil {
		dto["tx_hash"] = *code.TxHash
	}
	if code.Version != nil {
		dto["version"] = *code.Version
	}
	if code.SourceURL != nil {
		dto["source_url"] = *code.SourceURL
	}

	// Parse instantiate_permission if not empty
	if code.InstantiatePermission != "" {
		var perm map[string]interface{}
		if err := sonic.UnmarshalString(code.InstantiatePermission, &perm); err == nil {
			dto["instantiate_permission"] = perm
		} else {
			dto["instantiate_permission"] = code.InstantiatePermission
		}
	}

	dto["created_at"] = code.CreatedAt.Format(time.RFC3339)

	return dto
}

// convertContractExecutionToDTO converts ContractExecution to DTO
func convertContractExecutionToDTO(exec *models.ContractExecution) map[string]interface{} {
	dto := map[string]interface{}{
		"tx_hash":          exec.TxHash,
		"sender":           exec.Sender,
		"contract_address": exec.ContractAddress,
		"block_height":     float64(exec.BlockHeight),
		"unix_timestamp":   float64(exec.UnixTimestamp),
		"success":          exec.Success,
	}

	// Parse msg from JSON string
	var msg map[string]interface{}
	if err := sonic.UnmarshalString(exec.Msg, &msg); err == nil {
		dto["msg"] = msg
	} else {
		dto["msg"] = exec.Msg
	}

	// Parse funds if not empty
	if exec.Funds != "" {
		var funds interface{}
		if err := sonic.UnmarshalString(exec.Funds, &funds); err == nil {
			dto["funds"] = funds
		} else {
			dto["funds"] = exec.Funds
		}
	}

	if exec.CreatedAt != nil {
		dto["created_at"] = exec.CreatedAt.Format(time.RFC3339)
	}

	return dto
}

// GetWasmCodes retrieves a list of Wasm codes with filtering and pagination.
func (s *ExplorerService) GetWasmCodes(ctx context.Context, req *pb.GetWasmCodesRequest) (*pb.Reply, error) {
	var wasmCodes []models.WasmCode

	query := db.Get().Model(&models.WasmCode{})

	// Apply filters
	if req.Creator != "" {
		query = query.Where("creator = ?", req.Creator)
	}
	if req.ContractType != "" {
		query = query.Where("contract_type = ?", req.ContractType)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count wasm codes")
	}

	// Pagination
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	query = query.Offset(int(offset)).Limit(int(pageSize))

	// Order by code_id DESC (newest first)
	query = query.Order("code_id DESC")

	// Execute query
	if err := query.Find(&wasmCodes).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch wasm codes")
	}

	// Convert to DTOs
	codeDTOs := make([]interface{}, len(wasmCodes))
	for i, code := range wasmCodes {
		codeDTOs[i] = convertWasmCodeToDTO(&code)
	}

	// Build response
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

	dataStruct, err := structpb.NewStruct(map[string]interface{}{
		"codes": codeDTOs,
		"pagination": map[string]interface{}{
			"page":        float64(page),
			"page_size":   float64(pageSize),
			"total":       float64(total),
			"total_pages": float64(totalPages),
			"has_prev":    page > 1,
			"has_next":    page < totalPages,
		},
	})
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create response struct: %v", err))
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    dataStruct,
	}, nil
}

// GetWasmCodeByID retrieves detailed information for a specific Wasm code.
// Falls back to chain query if not found in database (when chain client is available).
func (s *ExplorerService) GetWasmCodeByID(ctx context.Context, req *pb.GetWasmCodeByIDRequest) (*pb.Reply, error) {
	var code models.WasmCode

	err := db.Get().Where("code_id = ?", req.CodeId).First(&code).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Try chain fallback if available
			chainClient, chainErr := GetSharedChainClient()
			if chainErr == nil {
				chainCode, queryErr := s.queryWasmCodeFromChain(ctx, chainClient, uint64(req.CodeId))
				if queryErr == nil {
					// Cache to database asynchronously
					go func() {
						_ = db.Get().Create(chainCode).Error
					}()
					code = *chainCode
					err = nil // Clear the error
				}
			}

			if err != nil {
				return nil, status.Error(codes.NotFound, "wasm code not found")
			}
		} else {
			return nil, status.Error(codes.Internal, "failed to fetch wasm code")
		}
	}

	dto := convertWasmCodeToDTO(&code)

	dataStruct, err := structpb.NewStruct(dto)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create response struct")
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    dataStruct,
	}, nil
}

// queryWasmCodeFromChain queries wasm code from chain
func (s *ExplorerService) queryWasmCodeFromChain(ctx context.Context, chainClient chainclient.ChainClientV2, codeID uint64) (*models.WasmCode, error) {
	resp, err := chainClient.FetchCode(ctx, codeID)
	if err != nil {
		return nil, err
	}

	if resp.CodeInfoResponse == nil {
		return nil, fmt.Errorf("code info response is nil")
	}

	code := &models.WasmCode{
		CodeID:      int64(codeID),
		Creator:     resp.CodeInfoResponse.Creator,
		DataHash:    hex.EncodeToString(resp.CodeInfoResponse.DataHash),
		BlockHeight: 0, // Will be filled by sync service later
	}

	// Parse InstantiatePermission
	if permJSON, err := sonic.MarshalString(resp.CodeInfoResponse.InstantiatePermission); err == nil {
		code.InstantiatePermission = permJSON
	}

	return code, nil
}

// GetContractTxs retrieves contract execution history.
func (s *ExplorerService) GetContractTxs(ctx context.Context, req *pb.GetContractTxsRequest) (*pb.Reply, error) {
	if req.ContractAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "contract_address is required")
	}

	var executions []models.ContractExecution

	query := db.Get().Model(&models.ContractExecution{}).
		Where("contract_address = ?", req.ContractAddress)

	// Apply filters
	if req.Sender != "" {
		query = query.Where("sender = ?", req.Sender)
	}
	if req.StartTime > 0 {
		query = query.Where("unix_timestamp >= ?", req.StartTime)
	}
	if req.EndTime > 0 {
		query = query.Where("unix_timestamp <= ?", req.EndTime)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count executions")
	}

	// Pagination
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	query = query.Offset(int(offset)).Limit(int(pageSize))

	// Order by timestamp DESC (newest first)
	query = query.Order("unix_timestamp DESC")

	// Execute query
	if err := query.Find(&executions).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch executions")
	}

	// Convert to DTOs
	execDTOs := make([]interface{}, len(executions))
	for i, exec := range executions {
		execDTOs[i] = convertContractExecutionToDTO(&exec)
	}

	// Build response
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

	dataStruct, err := structpb.NewStruct(map[string]interface{}{
		"executions": execDTOs,
		"pagination": map[string]interface{}{
			"page":        float64(page),
			"page_size":   float64(pageSize),
			"total":       float64(total),
			"total_pages": float64(totalPages),
			"has_prev":    page > 1,
			"has_next":    page < totalPages,
		},
	})
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create response struct: %v", err))
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    dataStruct,
	}, nil
}
