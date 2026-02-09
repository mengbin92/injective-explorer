// Package service provides Relayer API implementations.
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

// convertRelayerToDTO converts Relayer model to DTO
func convertRelayerToDTO(relayer *models.Relayer) map[string]interface{} {
	dto := map[string]interface{}{
		"market_id":             relayer.MarketID,
		"relayer_address":       relayer.RelayerAddress,
		"total_packets_relayed": float64(relayer.TotalPacketsRelayed),
		"total_packets_failed":  float64(relayer.TotalPacketsFailed),
		"success_rate":          relayer.SuccessRate,
		"avg_relay_time_ms":     float64(relayer.AvgRelayTimeMs),
		"last_active_at":        float64(relayer.LastActiveAt),
	}

	// Parse total_fees_earned from JSON string
	var fees []interface{}
	if relayer.TotalFeesEarned != "" {
		if err := sonic.UnmarshalString(relayer.TotalFeesEarned, &fees); err == nil {
			dto["total_fees_earned"] = fees
		} else {
			dto["total_fees_earned"] = relayer.TotalFeesEarned
		}
	} else {
		dto["total_fees_earned"] = []interface{}{}
	}

	// Parse active_channels from JSON string
	var channels []interface{}
	if relayer.ActiveChannels != "" {
		if err := sonic.UnmarshalString(relayer.ActiveChannels, &channels); err == nil {
			dto["active_channels"] = channels
		} else {
			dto["active_channels"] = relayer.ActiveChannels
		}
	} else {
		dto["active_channels"] = []interface{}{}
	}

	if relayer.CreatedAt != nil {
		dto["created_at"] = relayer.CreatedAt.Format(time.RFC3339)
	}

	if relayer.UpdatedAt != nil {
		dto["updated_at"] = relayer.UpdatedAt.Format(time.RFC3339)
	}

	return dto
}

// GetRelayers retrieves relayer records with filtering.
func (s *ExplorerService) GetRelayers(ctx context.Context, req *pb.GetRelayersRequest) (*pb.Reply, error) {
	var relayers []models.Relayer

	query := db.Get().Model(&models.Relayer{})

	// Apply filters
	if len(req.MarketIds) > 0 {
		query = query.Where("market_id IN ?", req.MarketIds)
	}
	if len(req.RelayerAddresses) > 0 {
		query = query.Where("relayer_address IN ?", req.RelayerAddresses)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count relayers")
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

	// Order by last_active_at DESC (most recent activity first)
	query = query.Order("last_active_at DESC")

	// Execute query
	if err := query.Find(&relayers).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch relayers")
	}

	// Convert to DTOs
	relayerDTOs := make([]interface{}, len(relayers))
	for i, relayer := range relayers {
		relayerDTOs[i] = convertRelayerToDTO(&relayer)
	}

	// Build response
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

	dataStruct, err := structpb.NewStruct(map[string]interface{}{
		"relayers": relayerDTOs,
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
