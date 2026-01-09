// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

// AddressLabelService implements the address label management service.
// It provides methods to manage address labels including CRUD operations
// and query by address or label type.
type AddressLabelService struct {
	pb.UnimplementedAddressLabelServiceServer
}

// NewAddressLabelService creates a new instance of AddressLabelService.
//
// Returns:
//   - *AddressLabelService: A new service instance
func NewAddressLabelService() *AddressLabelService {
	return &AddressLabelService{}
}

// Cache key constants for address label service
const (
	CacheKeyAddressLabels = "address_label:labels:%s:%s" // chain_id:address
)

// Cache expiration constants
const (
	CacheExpireAddressLabels = 1 * time.Hour
)

// CreateAddressLabel creates a new address label.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing address label information
//
// Returns:
//   - *pb.Reply: Response containing the created label
//   - error: Error if the operation fails
func (s *AddressLabelService) CreateAddressLabel(ctx context.Context, req *pb.CreateAddressLabelRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return ErrorReply(http.StatusBadRequest, "label_type is required", errors.New("label_type is required")), errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Create label
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	label := &models.AddressLabel{
		Address:     address,
		ChainID:     req.ChainId,
		LabelType:   req.LabelType,
		LabelValue:  req.LabelValue,
		LabelColor:  req.LabelColor,
		Description: req.Description,
		CreatedBy:   req.CreatedBy,
	}

	// Use FirstOrCreate to avoid duplicates
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		FirstOrCreate(label).Error

	if err != nil {
		global.Logger.Errorf("failed to create address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to create address label", err), err
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	// Convert to response
	labelDTO := models.ToAddressLabelDTO(label)
	return MarshalAndReply(labelDTO, "marshal label error")
}

// UpdateAddressLabel updates an existing address label.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing address label update information
//
// Returns:
//   - *pb.Reply: Response containing the updated label
//   - error: Error if the operation fails
func (s *AddressLabelService) UpdateAddressLabel(ctx context.Context, req *pb.UpdateAddressLabelRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return ErrorReply(http.StatusBadRequest, "label_type is required", errors.New("label_type is required")), errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Update label
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	updates := make(map[string]interface{})
	if req.LabelValue != "" {
		updates["label_value"] = req.LabelValue
	}
	if req.LabelColor != "" {
		updates["label_color"] = req.LabelColor
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}

	if len(updates) == 0 {
		return ErrorReply(http.StatusBadRequest, "at least one field must be provided for update", errors.New("no fields to update")), errors.New("no fields to update")
	}

	var label models.AddressLabel
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		First(&label).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "address label not found", err), err
		}
		global.Logger.Errorf("failed to find address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to find address label", err), err
	}

	err = db.Get().WithContext(queryCtx).
		Model(&models.AddressLabel{}).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		Updates(updates).Error

	if err != nil {
		global.Logger.Errorf("failed to update address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to update address label", err), err
	}

	// Reload to get updated data
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		First(&label).Error

	if err != nil {
		global.Logger.Errorf("failed to reload address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to reload address label", err), err
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	// Convert to response
	labelDTO := models.ToAddressLabelDTO(&label)
	return MarshalAndReply(labelDTO, "marshal label error")
}

// DeleteAddressLabel deletes an address label.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing address, chain ID, and label type
//
// Returns:
//   - *emptypb.Empty: Empty response on success
//   - error: Error if the operation fails
func (s *AddressLabelService) DeleteAddressLabel(ctx context.Context, req *pb.DeleteAddressLabelRequest) (*emptypb.Empty, error) {
	// Input validation
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	if req.Address == "" {
		return nil, errors.New("address is required")
	}

	if req.ChainId == "" {
		return nil, errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return nil, errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Delete label (soft delete)
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		Delete(&models.AddressLabel{}).Error

	if err != nil {
		global.Logger.Errorf("failed to delete address label: %v", err)
		return nil, errors.Wrap(err, "failed to delete address label")
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	return &emptypb.Empty{}, nil
}

// GetAddressLabels retrieves all labels for an address.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing address and chain ID
//
// Returns:
//   - *pb.Reply: Response containing list of labels
//   - error: Error if the operation fails
func (s *AddressLabelService) GetAddressLabels(ctx context.Context, req *pb.GetAddressLabelsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Try to get from cache
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	cachedLabels, err := s.getLabelsFromCache(queryCtx, req.ChainId, address)
	if err == nil && cachedLabels != nil {
		global.Logger.Debugf("address labels found in cache: address=%s", address)
		labelDTOs := models.ToAddressLabelDTOs(cachedLabels)
		return MarshalAndReply(labelDTOs, "marshal labels error")
	}

	// Query from database
	var labels []models.AddressLabel
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ?", req.ChainId, address).
		Find(&labels).Error

	if err != nil {
		global.Logger.Errorf("failed to get address labels: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get address labels", err), err
	}

	// Cache the labels
	go s.cacheLabels(context.Background(), req.ChainId, address, labels)

	// Convert to response
	labelDTOs := models.ToAddressLabelDTOs(labels)
	return MarshalAndReply(labelDTOs, "marshal labels error")
}

// ListAddressLabels lists address labels with filter conditions.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing filter conditions and pagination parameters
//
// Returns:
//   - *pb.Reply: Response containing paginated list of labels
//   - error: Error if the operation fails
func (s *AddressLabelService) ListAddressLabels(ctx context.Context, req *pb.ListAddressLabelsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	var paginationReq *pb.PaginationRequest
	if req.Pagination != nil {
		paginationReq = req.Pagination
	} else {
		paginationReq = &pb.PaginationRequest{
			Page:     MinPage,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, useCursor, cursorValue := normalizePagination(paginationReq.Page, paginationReq.PageSize, paginationReq.Cursor, MaxPageSize)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.AddressLabel{})

	// Apply filters
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	if req.LabelType != "" {
		query = query.Where("label_type = ?", req.LabelType)
	}

	if req.Address != "" {
		address := models.ConvertAddressToEthForStorage(req.Address)
		query = query.Where("address LIKE ?", "%"+address+"%")
	}

	if req.LabelValue != "" {
		query = query.Where("label_value LIKE ?", "%"+req.LabelValue+"%")
	}

	// Get total count
	var total int64
	var isExactCount bool
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(db.Get(), &models.AddressLabel{}, ctx)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for address labels: %v, using fallback value", countErr)
		total = ApproximateCountFallback
		isExactCount = false
	}

	// Apply pagination
	var labels []models.AddressLabel
	if useCursor {
		query = query.Where("id < ?", cursorValue).
			Order("id DESC").
			Limit(int(pageSize))
	} else {
		query = query.Order("id DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	err := query.Find(&labels).Error
	if err != nil {
		global.Logger.Errorf("failed to list address labels: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to list address labels", err), err
	}

	// Build response
	nextCursor := calculateNextCursor(useCursor, cursorValue, labels, func(l models.AddressLabel) uint64 { return uint64(l.ID) })
	labelDTOs := models.ToAddressLabelDTOs(labels)
	labelsResp := buildPaginatedResponse(labelDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(labels))

	return MarshalAndReply(labelsResp, "marshal labels error")
}

// Cache helper methods

// getLabelsFromCache retrieves address labels from Redis cache.
func (s *AddressLabelService) getLabelsFromCache(ctx context.Context, chainID, address string) ([]models.AddressLabel, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	data, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found in cache
		}
		return nil, err
	}

	var labels []models.AddressLabel
	err = sonic.Unmarshal([]byte(data), &labels)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal labels from cache")
	}

	return labels, nil
}

// cacheLabels caches address labels to Redis.
func (s *AddressLabelService) cacheLabels(ctx context.Context, chainID, address string, labels []models.AddressLabel) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	labelsJSON, err := sonic.Marshal(labels)
	if err != nil {
		global.Logger.Warnf("failed to marshal labels to JSON: %v", err)
		return
	}

	err = redisClient.Set(ctx, key, labelsJSON, CacheExpireAddressLabels).Err()
	if err != nil {
		global.Logger.Warnf("failed to cache labels: %v", err)
	}
}

// clearAddressLabelCache clears the cache for address labels.
func (s *AddressLabelService) clearAddressLabelCache(ctx context.Context, chainID, address string) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	if err := redisClient.Del(ctx, key).Err(); err != nil && err != redis.Nil {
		global.Logger.Warnf("failed to clear address label cache: %v", err)
	}
}
