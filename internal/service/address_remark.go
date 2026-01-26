// Package service provides business logic implementations for blockchain explorer.
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
	"gorm.io/gorm"
)

// AddressRemarkService manages address remarks.
type AddressRemarkService struct {
	pb.UnimplementedAddressRemarkServiceServer
}

// NewAddressRemarkService creates a new instance of AddressRemarkService.
func NewAddressRemarkService() *AddressRemarkService {
	return &AddressRemarkService{}
}

// Cache key constants
const (
	CacheKeyAddressRemark = "address_remark:%s:%s" // chain_id:address
)

// Cache expiration constants
const (
	CacheExpireAddressRemark = 1 * time.Hour
)

// SetAddressRemark sets or updates an address remark.
func (s *AddressRemarkService) SetAddressRemark(ctx context.Context, req *pb.SetAddressRemarkRequest) (*pb.Reply, error) {
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

	if req.Remark == "" {
		return ErrorReply(http.StatusBadRequest, "remark is required", errors.New("remark is required")), errors.New("remark is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Use FirstOrCreate to avoid duplicates
	remark := &models.AddressRemark{
		Address:    address,
		ChainID:    req.ChainId,
		Remark:     req.Remark,
		CreatedBy:  req.CreatedBy,
	}

	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ?", req.ChainId, address).
		Assign(remark).
		FirstOrCreate(remark).Error

	if err != nil {
		global.Logger.Errorf("failed to set address remark: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to set address remark", err), err
	}

	// Clear cache
	s.clearAddressRemarkCache(ctx, req.ChainId, address)

	// Convert to response
	remarkDTO := remark.ToAddressRemarkDTO()
	return MarshalAndReply(remarkDTO, "marshal remark error")
}

// GetAddressRemark gets an address remark.
func (s *AddressRemarkService) GetAddressRemark(ctx context.Context, req *pb.GetAddressRemarkRequest) (*pb.Reply, error) {
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

	cachedRemark, err := s.getRemarkFromCache(ctx, req.ChainId, address)
	if err == nil && cachedRemark != nil {
		global.Logger.Debugf("address remark found in cache: address=%s", address)
		remarkDTO := cachedRemark.ToAddressRemarkDTO()
		return MarshalAndReply(remarkDTO, "marshal remark error")
	}

	// Query from database
	var remark models.AddressRemark
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ?", req.ChainId, address).
		First(&remark).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Return empty response instead of 404
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: "no remark found for this address",
				Data:    nil,
			}, nil
		}
		global.Logger.Errorf("failed to get address remark: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get address remark", err), err
	}

	// Cache the remark
	go s.cacheRemark(context.Background(), &remark)

	// Convert to response
	remarkDTO := remark.ToAddressRemarkDTO()
	return MarshalAndReply(remarkDTO, "marshal remark error")
}

// DeleteAddressRemark deletes an address remark.
func (s *AddressRemarkService) DeleteAddressRemark(ctx context.Context, req *pb.DeleteAddressRemarkRequest) (*pb.Reply, error) {
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

	// Delete remark (soft delete)
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ?", req.ChainId, address).
		Delete(&models.AddressRemark{}).Error

	if err != nil {
		global.Logger.Errorf("failed to delete address remark: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to delete address remark", err), err
	}

	// Clear cache
	s.clearAddressRemarkCache(ctx, req.ChainId, address)

	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "address remark deleted successfully",
		Data:    nil,
	}, nil
}

// Cache helper methods

// getRemarkFromCache retrieves address remark from Redis cache.
func (s *AddressRemarkService) getRemarkFromCache(ctx context.Context, chainID, address string) (*models.AddressRemark, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	key := fmt.Sprintf(CacheKeyAddressRemark, chainID, address)
	data, err := redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var remark models.AddressRemark
	if err := sonic.Unmarshal([]byte(data), &remark); err != nil {
		return nil, err
	}

	return &remark, nil
}

// cacheRemark caches address remark to Redis.
func (s *AddressRemarkService) cacheRemark(ctx context.Context, remark *models.AddressRemark) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyAddressRemark, remark.ChainID, remark.Address)
	data, err := sonic.Marshal(remark)
	if err != nil {
		global.Logger.Warnf("failed to marshal address remark for cache: %v", err)
		return
	}

	if err := redisClient.Set(ctx, key, data, CacheExpireAddressRemark).Err(); err != nil {
		global.Logger.Warnf("failed to cache address remark: %v", err)
	}
}

// clearAddressRemarkCache clears the cache for address remark.
func (s *AddressRemarkService) clearAddressRemarkCache(ctx context.Context, chainID, address string) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyAddressRemark, chainID, address)
	if err := redisClient.Del(ctx, key).Err(); err != nil {
		global.Logger.Warnf("failed to clear address remark cache: %v", err)
	}
}
