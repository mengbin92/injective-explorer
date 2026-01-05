// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// AdminService implements the admin management service.
// It provides methods to manage API plans, IP blacklist, and email blacklist.
type AdminService struct {
	pb.UnimplementedAdminServiceServer
}

// NewAdminService creates a new instance of AdminService.
//
// Returns:
//   - *AdminService: A new service instance
func NewAdminService() *AdminService {
	return &AdminService{}
}

// GetUsageErrors gets error request list
func (s *AdminService) GetUsageErrors(ctx context.Context, req *pb.GetUsageErrorsRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	pagination := req.Pagination
	if pagination == nil {
		pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: 20,
		}
	}

	if pagination.Page < 1 {
		pagination.Page = 1
	}
	if pagination.PageSize < 1 || pagination.PageSize > 100 {
		pagination.PageSize = 20
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer queryCancel()

	// Build query for error requests (status code >= 400)
	query := db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("status_code >= ?", 400)

	// Filter by status code if provided
	if req.StatusCode > 0 {
		query = query.Where("status_code = ?", req.StatusCode)
	}

	// Apply time range filter
	now := time.Now()
	if req.TimeRange != "" {
		switch req.TimeRange {
		case "today":
			query = query.Where("created_at >= ?", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
		case "7days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -7))
		case "30days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -30))
		}
	}

	// Get total count
	var total int64
	err := query.Count(&total).Error
	if err != nil {
		global.Logger.Errorf("failed to count error requests: %v", err)
		return nil, status.Error(codes.Internal, "failed to query error requests")
	}

	// Get error logs with pagination
	var logs []models.APIRequestLog
	offset := (pagination.Page - 1) * pagination.PageSize
	err = query.Order("created_at DESC").
		Offset(int(offset)).
		Limit(int(pagination.PageSize)).
		Find(&logs).Error

	if err != nil {
		global.Logger.Errorf("failed to list error requests: %v", err)
		return nil, status.Error(codes.Internal, "failed to query error requests")
	}

	// Convert to response format
	errors := make([]*pb.UsageErrorInfo, 0, len(logs))
	for _, log := range logs {
		errors = append(errors, &pb.UsageErrorInfo{
			Time:         log.CreatedAt.Format(time.RFC3339),
			IpAddress:    log.IPAddress,
			Endpoint:     log.Endpoint,
			Method:       log.Method,
			StatusCode:   int32(log.StatusCode),
			ErrorMessage: log.ErrorMessage,
			ResponseTime: int32(log.ResponseTime),
		})
	}

	totalPages := int32((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
	response := map[string]interface{}{
		"errors": errors,
		"pagination": &pb.PaginationInfo{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			Total:      total,
			TotalPages: totalPages,
			HasPrev:    pagination.Page > 1,
			HasNext:    pagination.Page < totalPages,
		},
	}

	data, err := structpb.NewStruct(response)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: "success",
		Data:    data,
	}, nil
}

