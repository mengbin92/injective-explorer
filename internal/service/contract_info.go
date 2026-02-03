// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ContractInfoService implements the contract information query and management service.
// It provides methods to manage contract information including CRUD operations
// and query by symbol. This service supports various contract types (token contracts,
// NFT contracts, etc.), with token contracts being the primary use case currently.
type ContractInfoService struct {
	pb.UnimplementedContractInfoServiceServer
}

// NewContractInfoService creates a new instance of ContractInfoService.
//
// Returns:
//   - *ContractInfoService: A new service instance
func NewContractInfoService() *ContractInfoService {
	return &ContractInfoService{}
}

// Cache key constants for contract info service
const (
	CacheKeyContractInfoByAddress = "contract_info:address:%s:%s" // chain_id:address
	CacheKeyContractsBySymbol     = "contract_info:symbol:%s:%s"  // chain_id:symbol
	CacheKeyContractTags          = "contract_info:tags:%s:%s"    // chain_id:address
)

// Cache expiration constants
const (
	CacheExpireContractInfo = 1 * time.Hour
	CacheExpireContractTags = 1 * time.Hour
)

// Default values
const (
	DefaultContractType       = "solidity"
	DefaultChainID            = "injective-1"
	DefaultVerificationStatus = "unverified"
)

// CreateContractInfo creates a new contract information.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing contract information
//
// Returns:
//   - *pb.Reply: Response containing the created contract information
//   - error: Error if the operation fails
func (s *ContractInfoService) CreateContractInfo(ctx context.Context, req *pb.CreateContractInfoRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ContractAddress == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address is required", errors.New("contract_address is required")), errors.New("contract_address is required")
	}

	// At least one of name, token_name, or token_symbol should be provided
	if req.Name == "" && req.TokenName == "" && req.TokenSymbol == "" {
		return ErrorReply(http.StatusBadRequest, "at least one of name, token_name, or token_symbol is required", errors.New("name, token_name, or token_symbol is required")), errors.New("name, token_name, or token_symbol is required")
	}

	// Set default values
	contractType := req.ContractType
	if contractType == "" {
		contractType = DefaultContractType
	}

	chainID := req.ChainId
	if chainID == "" {
		chainID = DefaultChainID
	}

	verificationStatus := req.VerificationStatus
	if verificationStatus == "" {
		verificationStatus = DefaultVerificationStatus
	}

	// Convert address to Ethereum format for storage (consistent with existing code)
	contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)

	// Check if contract already exists
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var existingContract models.ContractInfo
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ?", chainID, contractAddress).
		First(&existingContract).Error

	if err == nil {
		// Contract already exists
		return ErrorReply(http.StatusConflict, "contract already exists", errors.New("contract already exists")), errors.New("contract already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error
		global.Logger.Errorf("failed to check existing contract: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to check existing contract", err), err
	}

	// Create contract info
	contractInfo := &models.ContractInfo{
		ContractAddress:    contractAddress,
		Name:               req.Name,
		TokenName:          req.TokenName,
		TokenSymbol:        req.TokenSymbol,
		ContractType:       contractType,
		ChainID:            chainID,
		VerificationStatus: verificationStatus,
		IsVerified:         false,
	}

	// Convert metadata from map[string]string to map[string]interface{}
	if len(req.Metadata) > 0 {
		metadata := make(map[string]interface{})
		for k, v := range req.Metadata {
			metadata[k] = v
		}
		contractInfo.Metadata = metadata
	}

	// Create contract in database
	err = db.Get().WithContext(queryCtx).Create(contractInfo).Error
	if err != nil {
		global.Logger.Errorf("failed to create contract info: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to create contract info", err), err
	}

	// Create tags if provided
	if len(req.Tags) > 0 {
		for _, tagType := range req.Tags {
			tag := &models.ContractTag{
				ContractAddress: contractAddress,
				ChainID:         chainID,
				TagType:         tagType,
			}
			// Use FirstOrCreate to avoid duplicates
			err = db.Get().WithContext(queryCtx).
				Where("chain_id = ? AND contract_address = ? AND tag_type = ?", chainID, contractAddress, tagType).
				FirstOrCreate(tag).Error
			if err != nil {
				global.Logger.Warnf("failed to create tag %s for contract %s: %v", tagType, contractAddress, err)
				// Continue with other tags even if one fails
			}
		}
	}

	// Load tags for response
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ?", chainID, contractAddress).
		Find(&contractInfo.Tags).Error
	if err != nil {
		global.Logger.Warnf("failed to load tags: %v", err)
	}

	// Clear cache
	s.clearContractCache(ctx, chainID, contractAddress)

	// Convert to response
	info := models.ToContractInfoDTO(contractInfo)
	return MarshalAndReply(info, "marshal contract info error")
}

// GetContractInfoByAddress retrieves contract information by address.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing contract address
//
// Returns:
//   - *pb.Reply: Response containing the contract information
//   - error: Error if the operation fails
func (s *ContractInfoService) GetContractInfoByAddress(ctx context.Context, req *pb.GetContractInfoByAddressRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ContractAddress == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address is required", errors.New("contract_address is required")), errors.New("contract_address is required")
	}

	// Set default chain ID
	chainID := req.ChainId
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Convert address to Ethereum format for storage
	contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)

	// Try to get from cache
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	cachedContract, err := s.getContractFromCache(queryCtx, chainID, contractAddress)
	if err == nil && cachedContract != nil {
		global.Logger.Debugf("contract info found in cache: address=%s", contractAddress)
		info := models.ToContractInfoDTO(cachedContract)
		return MarshalAndReply(info, "marshal contract info error")
	}

	// Query from database
	var contractInfo models.ContractInfo
	err = db.Get().WithContext(queryCtx).
		Preload("Tags").
		Where("chain_id = ? AND contract_address = ?", chainID, contractAddress).
		First(&contractInfo).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: "contract info not found",
				Data:    nil,
			}, nil
		}
		global.Logger.Errorf("failed to get contract info: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get contract info", err), err
	}

	// Cache the contract
	go s.cacheContract(context.Background(), &contractInfo)

	// Convert to response
	info := models.ToContractInfoDTO(&contractInfo)
	return MarshalAndReply(info, "marshal contract info error")
}

// GetContractsBySymbol retrieves contracts by symbol (supports one-to-many relationship).
// This is primarily used for token contracts, but can be extended for other contract types.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing symbol and optional filters
//
// Returns:
//   - *pb.Reply: Response containing list of contract information
//   - error: Error if the operation fails
func (s *ContractInfoService) GetContractsBySymbol(ctx context.Context, req *pb.GetContractsBySymbolRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Symbol == "" {
		return ErrorReply(http.StatusBadRequest, "symbol is required", errors.New("symbol is required")), errors.New("symbol is required")
	}

	// Normalize pagination parameters
	var pagination *pb.PaginationRequest
	if req.Pagination != nil {
		pagination = req.Pagination
	} else {
		pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, useCursor, _ := normalizePagination(
		pagination.Page,
		pagination.PageSize,
		pagination.Cursor,
		MaxPageSize,
	)

	// Build query
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).
		Model(&models.ContractInfo{}).
		Where("token_symbol = ?", req.Symbol)

	// Apply filters
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	if req.ContractType != "" {
		query = query.Where("contract_type = ?", req.ContractType)
	}

	// Get total count
	var total int64
	var isExactCount bool
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(query, &models.ContractInfo{}, queryCtx)
	if countErr != nil {
		global.Logger.Warnf("failed to get count for token contracts by symbol: %v", countErr)
		total = ApproximateCountFallback
		isExactCount = false
	}

	// Apply pagination
	if useCursor {
		// Cursor-based pagination (not implemented for this query, fallback to page-based)
		query = query.Order("created_at DESC").Limit(int(pageSize))
	} else {
		query = query.Order("created_at DESC").Offset(offset).Limit(int(pageSize))
	}

	// Execute query
	var contracts []models.ContractInfo
	err := query.Preload("Tags").Find(&contracts).Error
	if err != nil {
		global.Logger.Errorf("failed to query contracts by symbol: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query contracts", err), err
	}

	// Convert to DTO
	contractInfos := models.ToContractInfoDTOs(contracts)

	// Build paginated response
	response := buildPaginatedResponse(contractInfos, page, pageSize, total, isExactCount, useCursor, 0, "", len(contracts))
	return MarshalAndReply(response, "marshal contracts error")
}

// ListContractInfos lists contract information with various filter conditions.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing filter conditions and pagination
//
// Returns:
//   - *pb.Reply: Response containing list of contract information
//   - error: Error if the operation fails
func (s *ContractInfoService) ListContractInfos(ctx context.Context, req *pb.ListContractInfosRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	var pagination *pb.PaginationRequest
	if req.Pagination != nil {
		pagination = req.Pagination
	} else {
		pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, useCursor, _ := normalizePagination(
		pagination.Page,
		pagination.PageSize,
		pagination.Cursor,
		MaxPageSize,
	)

	// Build query
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).Model(&models.ContractInfo{})

	// Apply filters
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	if req.ContractType != "" {
		query = query.Where("contract_type = ?", req.ContractType)
	}

	if req.VerificationStatus != "" {
		query = query.Where("verification_status = ?", req.VerificationStatus)
	}

	if req.Name != "" {
		// Fuzzy match on name or token_name
		query = query.Where("name LIKE ? OR token_name LIKE ?", "%"+req.Name+"%", "%"+req.Name+"%")
	}

	if req.Symbol != "" {
		// Fuzzy match on token symbol
		query = query.Where("token_symbol LIKE ?", "%"+req.Symbol+"%")
	}

	// Get total count
	var total int64
	var isExactCount bool
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(query, &models.ContractInfo{}, queryCtx)
	if countErr != nil {
		global.Logger.Warnf("failed to get count for token contracts: %v", countErr)
		total = ApproximateCountFallback
		isExactCount = false
	}

	// Apply pagination
	if useCursor {
		// Cursor-based pagination (not implemented for this query, fallback to page-based)
		query = query.Order("created_at DESC").Limit(int(pageSize))
	} else {
		query = query.Order("created_at DESC").Offset(offset).Limit(int(pageSize))
	}

	// Execute query
	var contracts []models.ContractInfo
	err := query.Preload("Tags").Find(&contracts).Error
	if err != nil {
		global.Logger.Errorf("failed to query contract infos: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query contract infos", err), err
	}

	// Convert to DTO
	contractInfos := models.ToContractInfoDTOs(contracts)

	// Build paginated response
	response := buildPaginatedResponse(contractInfos, page, pageSize, total, isExactCount, useCursor, 0, "", len(contracts))
	return MarshalAndReply(response, "marshal contracts error")
}

// UpdateContractInfo updates contract information.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing update information
//
// Returns:
//   - *pb.Reply: Response containing the updated contract information
//   - error: Error if the operation fails
func (s *ContractInfoService) UpdateContractInfo(ctx context.Context, req *pb.UpdateContractInfoRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ContractAddress == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address is required", errors.New("contract_address is required")), errors.New("contract_address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	// Convert address to Ethereum format for storage
	contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)

	// Query existing contract
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var contractInfo models.ContractInfo
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ?", req.ChainId, contractAddress).
		First(&contractInfo).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: "contract info not found",
				Data:    nil,
			}, nil
		}
		global.Logger.Errorf("failed to get contract info: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get contract info", err), err
	}

	// Update fields (only update provided fields)
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.TokenName != "" {
		updates["token_name"] = req.TokenName
	}
	if req.TokenSymbol != "" {
		updates["token_symbol"] = req.TokenSymbol
	}
	if req.ContractType != "" {
		updates["contract_type"] = req.ContractType
	}
	if req.VerificationStatus != "" {
		updates["verification_status"] = req.VerificationStatus
		// Update is_verified based on verification_status
		updates["is_verified"] = (req.VerificationStatus == "verified")
	}
	if len(req.Metadata) > 0 {
		metadata := make(map[string]interface{})
		for k, v := range req.Metadata {
			metadata[k] = v
		}
		updates["metadata"] = metadata
	}

	// Update in database
	if len(updates) > 0 {
		err = db.Get().WithContext(queryCtx).
			Model(&contractInfo).
			Updates(updates).Error
		if err != nil {
			global.Logger.Errorf("failed to update contract info: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to update contract info", err), err
		}
	}

	// Reload contract with tags
	err = db.Get().WithContext(queryCtx).
		Preload("Tags").
		Where("chain_id = ? AND contract_address = ?", req.ChainId, contractAddress).
		First(&contractInfo).Error
	if err != nil {
		global.Logger.Warnf("failed to reload contract info: %v", err)
	}

	// Clear cache
	s.clearContractCache(ctx, req.ChainId, contractAddress)

	// Convert to response
	info := models.ToContractInfoDTO(&contractInfo)
	return MarshalAndReply(info, "marshal contract info error")
}

// DeleteContractInfo deletes contract information (soft delete).
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing contract address and chain ID
//
// Returns:
//   - *pb.Reply: Response indicating success
//   - error: Error if the operation fails
func (s *ContractInfoService) DeleteContractInfo(ctx context.Context, req *pb.DeleteContractInfoRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ContractAddress == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address is required", errors.New("contract_address is required")), errors.New("contract_address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	// Convert address to Ethereum format for storage
	contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)

	// Soft delete
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ?", req.ChainId, contractAddress).
		Delete(&models.ContractInfo{}).Error

	if err != nil {
		global.Logger.Errorf("failed to delete contract info: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to delete contract info", err), err
	}

	// Clear cache
	s.clearContractCache(ctx, req.ChainId, contractAddress)

	// Return success response
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
	}, nil
}

// AddContractTag adds a tag to a contract.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing tag information
//
// Returns:
//   - *pb.Reply: Response containing the created tag
//   - error: Error if the operation fails
func (s *ContractInfoService) AddContractTag(ctx context.Context, req *pb.AddContractTagRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ContractAddress == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address is required", errors.New("contract_address is required")), errors.New("contract_address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	// Validation: tag_type is optional if tag_definition_id is provided
	if req.TagType == "" && req.TagDefinitionId == 0 {
		return ErrorReply(http.StatusBadRequest, "either tag_type or tag_definition_id is required",
			errors.New("either tag_type or tag_definition_id is required")), errors.New("either tag_type or tag_definition_id is required")
	}

	// Convert address to Ethereum format for storage
	contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)

	// Create or update tag
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var tag *models.ContractTag

	// If tag_definition_id is provided, use preset tag definition
	if req.TagDefinitionId > 0 {
		// Query tag definition
		var tagDef models.TagDefinition
		err := db.Get().WithContext(queryCtx).
			Where("id = ? AND tag_type = ?", req.TagDefinitionId, "contract").
			First(&tagDef).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &pb.Reply{
					Code:    int32(http.StatusOK),
					Message: "tag definition not found",
					Data:    nil,
				}, nil
			}
			global.Logger.Errorf("failed to query tag definition: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to query tag definition", err), err
		}

		// Create tag from preset definition
		tag = &models.ContractTag{
			ContractAddress:  contractAddress,
			ChainID:          req.ChainId,
			TagType:          tagDef.TagCategory, // Use tag_category from definition
			TagValue:         tagDef.TagName,     // Use tag_name from definition
			TagColor:         tagDef.TagColor,    // Use tag_color from definition
			Description:      tagDef.Description, // Use description from definition
			TagDefinitionID:  &req.TagDefinitionId,
			IsPreset:         true,               // Mark as preset
			Visible:          req.Visible,        // Use visible from request
			CreatedBy:        req.CreatedBy,
		}
	} else {
		// Custom tag
		tag = &models.ContractTag{
			ContractAddress: contractAddress,
			ChainID:         req.ChainId,
			TagType:         req.TagType,
			TagValue:        req.TagValue,
			TagColor:        req.TagColor,
			Description:     req.Description,
			TagDefinitionID:  nil,
			IsPreset:        false,
			Visible:         req.Visible,
			CreatedBy:       req.CreatedBy,
		}
	}

	// Use FirstOrCreate to avoid duplicates
	// Note: Unique constraint is (chain_id, contract_address, tag_type, tag_value)
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ? AND tag_type = ? AND tag_value = ?",
			req.ChainId, contractAddress, tag.TagType, tag.TagValue).
		FirstOrCreate(tag).Error

	if err != nil {
		global.Logger.Errorf("failed to add contract tag: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to add contract tag", err), err
	}

	// If this is a newly created preset tag, update usage_count
	if req.TagDefinitionId > 0 {
		// Check if this is a newly created record by comparing CreatedAt and UpdatedAt
		// If they are equal or very close (within 1 second), it's a new record
		if tag.CreatedAt.Equal(tag.UpdatedAt) || tag.UpdatedAt.Sub(tag.CreatedAt) < time.Second {
			// This is a newly created tag, update usage_count
			err = db.Get().WithContext(queryCtx).
				Model(&models.TagDefinition{}).
				Where("id = ?", req.TagDefinitionId).
				Update("usage_count", gorm.Expr("usage_count + ?", 1)).Error
			if err != nil {
				global.Logger.Warnf("failed to update usage count: %v", err)
			}
		}
	}

	// Clear cache
	s.clearContractCache(ctx, req.ChainId, contractAddress)

	// Convert to response
	tagInfo := models.ToContractTagInfoDTO(tag)
	return MarshalAndReply(tagInfo, "marshal tag error")
}

// RemoveContractTag removes a tag from a contract.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing contract address, chain ID, and tag type
//
// Returns:
//   - *pb.Reply: Response indicating success
//   - error: Error if the operation fails
func (s *ContractInfoService) RemoveContractTag(ctx context.Context, req *pb.RemoveContractTagRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ContractAddress == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address is required", errors.New("contract_address is required")), errors.New("contract_address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.TagType == "" {
		return ErrorReply(http.StatusBadRequest, "tag_type is required", errors.New("tag_type is required")), errors.New("tag_type is required")
	}

	// Convert address to Ethereum format for storage
	contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Before deletion, query the tag to check if it's a preset tag
	var tag models.ContractTag
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ? AND tag_type = ?", req.ChainId, contractAddress, req.TagType).
		First(&tag).Error

	if err == nil && tag.TagDefinitionID != nil {
		// If this is a preset tag, decrement usage_count in tag_definitions table
		err = db.Get().WithContext(queryCtx).
			Model(&models.TagDefinition{}).
			Where("id = ?", *tag.TagDefinitionID).
			Update("usage_count", gorm.Expr("GREATEST(usage_count - 1, 0)")).Error
		if err != nil {
			global.Logger.Warnf("failed to decrease usage count: %v", err)
			// Continue with deletion even if usage_count update fails
		}
	}

	// Delete tag
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ? AND tag_type = ?", req.ChainId, contractAddress, req.TagType).
		Delete(&models.ContractTag{}).Error

	if err != nil {
		global.Logger.Errorf("failed to remove contract tag: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to remove contract tag", err), err
	}

	// Clear cache
	s.clearContractCache(ctx, req.ChainId, contractAddress)

	// Return success response
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
	}, nil
}

// GetContractTags retrieves all tags for a contract.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing contract address and chain ID
//
// Returns:
//   - *pb.Reply: Response containing list of tags
//   - error: Error if the operation fails
func (s *ContractInfoService) GetContractTags(ctx context.Context, req *pb.GetContractTagsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ContractAddress == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address is required", errors.New("contract_address is required")), errors.New("contract_address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	// Convert address to Ethereum format for storage
	contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)

	// Try to get from cache
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	cachedTags, err := s.getTagsFromCache(queryCtx, req.ChainId, contractAddress)
	if err == nil && cachedTags != nil {
		global.Logger.Debugf("contract tags found in cache: address=%s", contractAddress)
		tagInfos := models.ToContractTagInfoDTOs(cachedTags)
		return MarshalAndReply(tagInfos, "marshal tags error")
	}

	// Query from database
	var tags []models.ContractTag
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ?", req.ChainId, contractAddress).
		Find(&tags).Error

	if err != nil {
		global.Logger.Errorf("failed to get contract tags: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get contract tags", err), err
	}

	// Cache the tags
	go s.cacheTags(context.Background(), req.ChainId, contractAddress, tags)

	// Convert to response
	tagInfos := models.ToContractTagInfoDTOs(tags)

	return MarshalAndReply(tagInfos, "marshal tags error")
}

// Cache helper methods

// getContractFromCache retrieves contract info from Redis cache.
func (s *ContractInfoService) getContractFromCache(ctx context.Context, chainID, contractAddress string) (*models.ContractInfo, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	key := fmt.Sprintf(CacheKeyContractInfoByAddress, chainID, contractAddress)
	data, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found in cache
		}
		return nil, err
	}

	var contract models.ContractInfo
	err = sonic.Unmarshal([]byte(data), &contract)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal contract from cache")
	}

	return &contract, nil
}

// cacheContract caches contract info to Redis.
func (s *ContractInfoService) cacheContract(ctx context.Context, contract *models.ContractInfo) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Load tags before caching
	if len(contract.Tags) == 0 {
		queryCtx, cancel := context.WithTimeout(context.Background(), DBQueryTimeout)
		defer cancel()
		db.Get().WithContext(queryCtx).
			Where("chain_id = ? AND contract_address = ?", contract.ChainID, contract.ContractAddress).
			Find(&contract.Tags)
	}

	key := fmt.Sprintf(CacheKeyContractInfoByAddress, contract.ChainID, contract.ContractAddress)
	contractJSON, err := sonic.Marshal(contract)
	if err != nil {
		global.Logger.Warnf("failed to marshal contract to JSON: %v", err)
		return
	}

	err = redisClient.Set(ctx, key, contractJSON, CacheExpireContractInfo).Err()
	if err != nil {
		global.Logger.Warnf("failed to cache contract: %v", err)
	}
}

// getTagsFromCache retrieves contract tags from Redis cache.
func (s *ContractInfoService) getTagsFromCache(ctx context.Context, chainID, contractAddress string) ([]models.ContractTag, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	key := fmt.Sprintf(CacheKeyContractTags, chainID, contractAddress)
	data, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found in cache
		}
		return nil, err
	}

	var tags []models.ContractTag
	err = sonic.Unmarshal([]byte(data), &tags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal tags from cache")
	}

	return tags, nil
}

// cacheTags caches contract tags to Redis.
func (s *ContractInfoService) cacheTags(ctx context.Context, chainID, contractAddress string, tags []models.ContractTag) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyContractTags, chainID, contractAddress)
	tagsJSON, err := sonic.Marshal(tags)
	if err != nil {
		global.Logger.Warnf("failed to marshal tags to JSON: %v", err)
		return
	}

	err = redisClient.Set(ctx, key, tagsJSON, CacheExpireContractTags).Err()
	if err != nil {
		global.Logger.Warnf("failed to cache tags: %v", err)
	}
}

// clearContractCache clears all cache entries related to a contract.
func (s *ContractInfoService) clearContractCache(ctx context.Context, chainID, contractAddress string) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	keys := []string{
		fmt.Sprintf(CacheKeyContractInfoByAddress, chainID, contractAddress),
		fmt.Sprintf(CacheKeyContractTags, chainID, contractAddress),
	}

	for _, key := range keys {
		err := redisClient.Del(ctx, key).Err()
		if err != nil && err != redis.Nil {
			global.Logger.Warnf("failed to clear cache key %s: %v", key, err)
		}
	}
}

// UploadContractFiles uploads contract source code files (.sol) and ABI files (.json).
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing file contents and metadata
//
// Returns:
//   - *pb.Reply: Response containing upload result
//   - error: Error if the operation fails
func (s *ContractInfoService) UploadContractFiles(ctx context.Context, req *pb.UploadContractFilesRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ContractAddress == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address is required", errors.New("contract_address is required")), errors.New("contract_address is required")
	}

	// At least one file must be provided
	if len(req.SourceCode) == 0 && len(req.Abi) == 0 {
		return ErrorReply(http.StatusBadRequest, "at least one file (source_code or abi) is required", errors.New("no file provided")), errors.New("no file provided")
	}

	// Set default chain ID
	chainID := req.ChainId
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Convert address to Ethereum format for storage
	contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)

	queryCtx, queryCancel := context.WithTimeout(ctx, DBQueryTimeout)
	defer queryCancel()

	// Check if contract exists
	var contractInfo models.ContractInfo
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND contract_address = ?", chainID, contractAddress).
		First(&contractInfo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: "contract not found",
				Data:    nil,
			}, nil
		}
		global.Logger.Errorf("failed to check contract existence: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to check contract existence", err), err
	}

	uploadedFiles := make([]map[string]interface{}, 0)
	const maxFileSize = 10 * 1024 * 1024 // 10MB max file size
	const storageThreshold = 100 * 1024  // 100KB threshold for using object storage

	// Concurrent file upload using WaitGroup
	var wg sync.WaitGroup
	var mu sync.Mutex
	var uploadErr error

	// Upload source code file if provided
	if len(req.SourceCode) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fileData, err := validateFile(req.SourceCode, "source", ".sol", req.SourceCodeFileName, maxFileSize)
			if err != nil {
				uploadErr = err
				return
			}

			sourceFile, err := s.saveContractFile(queryCtx, contractAddress, chainID, "source", fileData, req.SourceCodeFileName, req.Description, req.UploadedBy, storageThreshold)
			if err != nil {
				global.Logger.Errorf("failed to save source code file: %v", err)
				uploadErr = err
				return
			}

			mu.Lock()
			uploadedFiles = append(uploadedFiles, map[string]interface{}{
				"file_type":    "source",
				"file_name":    sourceFile.FileName,
				"file_id":      sourceFile.ID,
				"file_size":    sourceFile.FileSize,
				"storage_type": s.getStorageType(sourceFile),
			})
			mu.Unlock()
		}()
	}

	// Upload ABI file if provided
	if len(req.Abi) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fileData, err := validateFile(req.Abi, "abi", ".json", req.AbiFileName, maxFileSize)
			if err != nil {
				uploadErr = err
				return
			}

			abiFile, err := s.saveContractFile(queryCtx, contractAddress, chainID, "abi", fileData, req.AbiFileName, req.Description, req.UploadedBy, storageThreshold)
			if err != nil {
				global.Logger.Errorf("failed to save ABI file: %v", err)
				uploadErr = err
				return
			}

			mu.Lock()
			uploadedFiles = append(uploadedFiles, map[string]interface{}{
				"file_type":    "abi",
				"file_name":    abiFile.FileName,
				"file_id":      abiFile.ID,
				"file_size":    abiFile.FileSize,
				"storage_type": s.getStorageType(abiFile),
			})
			mu.Unlock()
		}()
	}

	// Wait for all uploads to complete
	wg.Wait()

	// Check for upload errors
	if uploadErr != nil {
		// Check if it's a validation error by inspecting the error message
		errMsg := uploadErr.Error()
		if len(errMsg) > 0 && (errMsg == "invalid base64 encoding" || errMsg == "file size exceeds maximum" || errMsg == "invalid JSON format for ABI file") {
			return ErrorReply(http.StatusBadRequest, fmt.Sprintf("invalid file: %v", uploadErr), uploadErr), uploadErr
		}
		return ErrorReply(http.StatusInternalServerError, "failed to save file", uploadErr), uploadErr
	}

	// Build response
	responseData := map[string]interface{}{
		"contract_address": contractAddress,
		"chain_id":         chainID,
		"uploaded_files":   uploadedFiles,
		"uploaded_at":      time.Now().Format(time.RFC3339),
	}

	return MarshalAndReply(responseData, "marshal upload response error")
}

// FileData represents file data
type FileData struct {
	Content  []byte
	Name     string
	MIMEType string
}

// validateFile validates file content and metadata.
// This function operates directly on bytes, avoiding unnecessary base64 decoding.
func validateFile(fileContent []byte, fileType, expectedExt, fileName string, maxSize int) (*FileData, error) {
	// Check file size
	if len(fileContent) > maxSize {
		return nil, errors.Errorf("file size exceeds maximum %d bytes", maxSize)
	}

	// Validate file extension
	if fileName == "" {
		fileName = fmt.Sprintf("file%s", expectedExt)
	}

	// Determine MIME type
	mimeType := "text/plain"
	if fileType == "abi" || expectedExt == ".json" {
		// Validate JSON format for ABI files
		var jsonData interface{}
		if err := sonic.Unmarshal(fileContent, &jsonData); err != nil {
			return nil, errors.Wrap(err, "invalid JSON format for ABI file")
		}
		mimeType = "application/json"
	}

	return &FileData{
		Content:  fileContent,
		Name:     fileName,
		MIMEType: mimeType,
	}, nil
}

// saveContractFile saves a contract file to database or object storage
func (s *ContractInfoService) saveContractFile(ctx context.Context, contractAddress, chainID, fileType string, fileData *FileData, _, description, uploadedBy string, storageThreshold int) (*models.ContractSourceFile, error) {
	// Calculate file hash
	hash := calculateFileHash(fileData.Content)

	// Check if file already exists (by hash)
	var existingFile models.ContractSourceFile
	err := db.Get().WithContext(ctx).
		Where("chain_id = ? AND contract_address = ? AND file_type = ? AND file_hash = ?", chainID, contractAddress, fileType, hash).
		First(&existingFile).Error

	if err == nil {
		// File already exists, return existing file
		return &existingFile, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrap(err, "failed to check existing file")
	}

	// Create new file record
	sourceFile := &models.ContractSourceFile{
		ContractAddress: contractAddress,
		ChainID:         chainID,
		FileName:        fileData.Name,
		FileType:        fileType,
		FileSize:        int64(len(fileData.Content)),
		FileHash:        hash,
		MIMEType:        fileData.MIMEType,
		Description:     description,
		UploadedBy:      uploadedBy,
	}

	// Decide storage location based on file size
	if len(fileData.Content) > storageThreshold {
		// Store in object storage for large files
		storageClient := storage.Get()
		storageKey := fmt.Sprintf("contract-files/%s/%s/%s/%s", chainID, contractAddress, fileType, hash)

		if err := storageClient.PutObject(ctx, storageKey, fileData.Content); err != nil {
			global.Logger.Warnf("failed to upload file to object storage, falling back to database: %v", err)
			// Fallback to database storage
			sourceFile.FileContent = string(fileData.Content)
		} else {
			sourceFile.FileContentStorageKey = storageKey
		}
	} else {
		// Store in database for small files
		sourceFile.FileContent = string(fileData.Content)
	}

	// Save to database
	err = db.Get().WithContext(ctx).Create(sourceFile).Error
	if err != nil {
		return nil, errors.Wrap(err, "failed to create contract source file")
	}

	return sourceFile, nil
}

// getStorageType returns the storage type used for a file
func (s *ContractInfoService) getStorageType(file *models.ContractSourceFile) string {
	if file.FileContentStorageKey != "" {
		return "object_storage"
	}
	return "database"
}

// calculateFileHash calculates SHA256 hash of file content
func calculateFileHash(content []byte) string {
	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash)
}
