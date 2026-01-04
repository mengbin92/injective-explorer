// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
)

// GetPlan gets a plan by ID
func (s *AdminService) GetPlan(ctx context.Context, req *pb.GetPlanRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	var plan models.APIPlan
	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "plan not found")
		}
		global.Logger.Errorf("failed to get plan: %v", err)
		return nil, status.Error(codes.Internal, "failed to query plan")
	}

	// Get plan statistics
	tokenCount, userCount, err := s.getPlanStats(queryCtx, plan.ID)
	if err != nil {
		global.Logger.Warnf("failed to get plan stats: %v", err)
	}

	planInfo := &pb.PlanInfo{
		Id:                 uint64(plan.ID),
		Name:               plan.Name,
		PlanType:           plan.PlanType,
		MonthlyLimit:       plan.MonthlyLimit,
		RateLimitPerSecond: int32(plan.RateLimitPerSecond),
		MaxTokens:          int32(plan.MaxTokens),
		Price:              plan.Price,
		Status:             plan.Status,
		TokenCount:         tokenCount,
		UserCount:          userCount,
		CreatedAt:          plan.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          plan.UpdatedAt.Format(time.RFC3339),
	}

	planMap := make(map[string]interface{})
	planBytes, _ := sonic.Marshal(planInfo)
	sonic.Unmarshal(planBytes, &planMap)

	data, err := structpb.NewStruct(planMap)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: "success",
		Data:    data,
	}, nil
}

// ListPlans lists all plans
func (s *AdminService) ListPlans(ctx context.Context, req *pb.ListPlansRequest) (*pb.Reply, error) {
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

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).Model(&models.APIPlan{})

	if req.PlanType != "" {
		query = query.Where("plan_type = ?", req.PlanType)
	}
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	var total int64
	err := query.Count(&total).Error
	if err != nil {
		global.Logger.Errorf("failed to count plans: %v", err)
		return nil, status.Error(codes.Internal, "failed to query plans")
	}

	var plans []models.APIPlan
	offset := int((pagination.Page - 1) * pagination.PageSize)
	err = query.Offset(offset).Limit(int(pagination.PageSize)).Order("created_at DESC").Find(&plans).Error
	if err != nil {
		global.Logger.Errorf("failed to list plans: %v", err)
		return nil, status.Error(codes.Internal, "failed to query plans")
	}

	planInfos := make([]*pb.PlanInfo, 0, len(plans))
	for _, plan := range plans {
		tokenCount, userCount, _ := s.getPlanStats(queryCtx, plan.ID)
		planInfo := &pb.PlanInfo{
			Id:                 uint64(plan.ID),
			Name:               plan.Name,
			PlanType:           plan.PlanType,
			MonthlyLimit:       plan.MonthlyLimit,
			RateLimitPerSecond: int32(plan.RateLimitPerSecond),
			MaxTokens:          int32(plan.MaxTokens),
			Price:              plan.Price,
			Status:             plan.Status,
			TokenCount:         tokenCount,
			UserCount:          userCount,
			CreatedAt:          plan.CreatedAt.Format(time.RFC3339),
			UpdatedAt:          plan.UpdatedAt.Format(time.RFC3339),
		}
		planInfos = append(planInfos, planInfo)
	}

	totalPages := int32((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
	response := map[string]interface{}{
		"plans": planInfos,
		"pagination": &pb.PaginationInfo{
			Page:      pagination.Page,
			PageSize:  pagination.PageSize,
			Total:     total,
			TotalPages: totalPages,
			HasPrev:   pagination.Page > 1,
			HasNext:   pagination.Page < totalPages,
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

// UpdatePlan updates a plan configuration
func (s *AdminService) UpdatePlan(ctx context.Context, req *pb.UpdatePlanRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	var plan models.APIPlan
	err := db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "plan not found")
		}
		global.Logger.Errorf("failed to get plan: %v", err)
		return nil, status.Error(codes.Internal, "failed to query plan")
	}

	// Update fields
	updates := make(map[string]interface{})
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.MonthlyLimit > 0 {
		updates["monthly_limit"] = req.MonthlyLimit
	}
	if req.RateLimitPerSecond > 0 {
		updates["rate_limit_per_second"] = req.RateLimitPerSecond
	}
	if req.MaxTokens > 0 {
		updates["max_tokens"] = req.MaxTokens
	}

	if len(updates) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no fields to update")
	}

	err = db.Get().WithContext(queryCtx).Model(&plan).Updates(updates).Error
	if err != nil {
		global.Logger.Errorf("failed to update plan: %v", err)
		return nil, status.Error(codes.Internal, "failed to update plan")
	}

	// Reload plan
	err = db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&plan).Error
	if err != nil {
		global.Logger.Errorf("failed to reload plan: %v", err)
		return nil, status.Error(codes.Internal, "failed to reload plan")
	}

	planInfo := &pb.PlanInfo{
		Id:                 uint64(plan.ID),
		Name:               plan.Name,
		PlanType:           plan.PlanType,
		MonthlyLimit:       plan.MonthlyLimit,
		RateLimitPerSecond: int32(plan.RateLimitPerSecond),
		MaxTokens:          int32(plan.MaxTokens),
		Price:              plan.Price,
		Status:             plan.Status,
		CreatedAt:          plan.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          plan.UpdatedAt.Format(time.RFC3339),
	}

	planMap := make(map[string]interface{})
	planBytes, _ := sonic.Marshal(planInfo)
	sonic.Unmarshal(planBytes, &planMap)

	data, err := structpb.NewStruct(planMap)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: "plan updated successfully",
		Data:    data,
	}, nil
}

// GetPlanStats gets plan usage statistics
func (s *AdminService) GetPlanStats(ctx context.Context, req *pb.GetPlanStatsRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	tokenCount, userCount, err := s.getPlanStats(queryCtx, uint(req.Id))
	if err != nil {
		global.Logger.Errorf("failed to get plan stats: %v", err)
		return nil, status.Error(codes.Internal, "failed to get plan stats")
	}

	response := map[string]interface{}{
		"token_count": tokenCount,
		"user_count":  userCount,
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

// getPlanStats gets plan statistics (token count and user count)
func (s *AdminService) getPlanStats(ctx context.Context, planID uint) (int64, int64, error) {
	planName := ""
	var plan models.APIPlan
	err := db.Get().WithContext(ctx).Where("id = ?", planID).First(&plan).Error
	if err != nil {
		return 0, 0, err
	}
	planName = plan.Name

	// Count tokens using this plan
	var tokenCount int64
	err = db.Get().WithContext(ctx).Model(&models.APIToken{}).
		Where("plan_type = ? AND deleted_at IS NULL", planName).
		Count(&tokenCount).Error
	if err != nil {
		return 0, 0, err
	}

	// Count unique users using this plan
	var userCount int64
	err = db.Get().WithContext(ctx).Model(&models.APIToken{}).
		Where("plan_type = ? AND deleted_at IS NULL", planName).
		Distinct("user_id").
		Count(&userCount).Error
	if err != nil {
		return 0, 0, err
	}

	return tokenCount, userCount, nil
}

