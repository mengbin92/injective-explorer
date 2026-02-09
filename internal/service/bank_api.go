// Package service provides Bank API implementations.
package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// convertBankTransferToDTO converts BankTransfer model to DTO.
func convertBankTransferToDTO(transfer *models.BankTransfer) map[string]interface{} {
	dto := map[string]interface{}{
		"tx_hash":        transfer.TxHash,
		"sender":         transfer.Sender,
		"recipient":      transfer.Recipient,
		"block_height":   float64(transfer.BlockHeight),
		"unix_timestamp": float64(transfer.UnixTimestamp),
		"success":        transfer.Success,
	}

	// Parse amounts from JSON string
	var amounts []interface{}
	if err := sonic.UnmarshalString(transfer.Amounts, &amounts); err == nil {
		dto["amounts"] = amounts
	} else {
		dto["amounts"] = transfer.Amounts
	}

	if transfer.IsCommunityPoolRelated {
		dto["is_community_pool_related"] = true
	}

	if transfer.CreatedAt != nil {
		dto["created_at"] = transfer.CreatedAt.Format(time.RFC3339)
	}

	return dto
}

// GetBankTransfers retrieves bank transfer records with filtering and pagination.
func (s *ExplorerService) GetBankTransfers(ctx context.Context, req *pb.GetBankTransfersRequest) (*pb.Reply, error) {
	var transfers []models.BankTransfer

	query := db.Get().Model(&models.BankTransfer{})

	// Apply filters
	if len(req.Senders) > 0 {
		query = query.Where("sender IN ?", req.Senders)
	}
	if len(req.Recipients) > 0 {
		query = query.Where("recipient IN ?", req.Recipients)
	}
	if len(req.Addresses) > 0 {
		query = query.Where("sender IN ? OR recipient IN ?", req.Addresses, req.Addresses)
	}
	if req.StartTime > 0 {
		query = query.Where("unix_timestamp >= ?", req.StartTime)
	}
	if req.EndTime > 0 {
		query = query.Where("unix_timestamp <= ?", req.EndTime)
	}
	if req.Denom != "" {
		// Search in JSON field using LIKE
		query = query.Where("amounts LIKE ?", "%\"denom\":\""+req.Denom+"\"%")
	}
	if req.IsCommunityPoolRelated {
		query = query.Where("is_community_pool_related = ?", true)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count bank transfers")
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
	if err := query.Find(&transfers).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch bank transfers")
	}

	// Convert to DTOs
	transferDTOs := make([]interface{}, len(transfers))
	for i, transfer := range transfers {
		transferDTOs[i] = convertBankTransferToDTO(&transfer)
	}

	// Build response
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

	dataStruct, err := structpb.NewStruct(map[string]interface{}{
		"transfers": transferDTOs,
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
