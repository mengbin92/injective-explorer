// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"net/http"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// GetPlan gets a plan by ID
func (s *AdminService) GetPlan(ctx context.Context, req *pb.GetPlanRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	var plan models.APIPlan
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "plan not found", err), err
		}
		global.Logger.Errorf("failed to get plan: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query plan", err), err
	}

	// Get plan statistics
	tokenCount, userCount, err := s.getPlanStats(queryCtx, plan.ID)
	if err != nil {
		global.Logger.Warnf("failed to get plan stats: %v", err)
		// Use fallback values
		tokenCount = 0
		userCount = 0
	}

	// Convert to DTO
	planInfo := models.ToPlanDTO(&plan, tokenCount, userCount)

	return MarshalAndReply(planInfo, "failed to marshal response")
}

// ListPlans lists all plans
func (s *AdminService) ListPlans(ctx context.Context, req *pb.ListPlansRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
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

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
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
		return ErrorReply(http.StatusInternalServerError, "failed to query plans", err), err
	}

	var plans []models.APIPlan
	offset := int((pagination.Page - 1) * pagination.PageSize)
	err = query.Offset(offset).Limit(int(pagination.PageSize)).Order("created_at DESC").Find(&plans).Error
	if err != nil {
		global.Logger.Errorf("failed to list plans: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query plans", err), err
	}

	// Convert plans to DTOs
	getStatsFunc := func(planID uint) (int64, int64, error) {
		return s.getPlanStats(queryCtx, planID)
	}
	planInfos := models.ToPlanDTOs(plans, getStatsFunc)

	// Build paginated response using buildPaginatedResponse
	response := buildPaginatedResponse(planInfos, pagination.Page, pagination.PageSize, total, true, false, 0, "", len(plans))

	return MarshalAndReply(response, "failed to marshal response")
}

// CreatePlan creates a new plan
func (s *AdminService) CreatePlan(ctx context.Context, req *pb.CreatePlanRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	// Validate required fields
	if req.Name == "" {
		err := errors.New("missing name")
		return ErrorReply(http.StatusBadRequest, "name is required", err), err
	}
	if req.PlanType == "" {
		err := errors.New("missing plan_type")
		return ErrorReply(http.StatusBadRequest, "plan_type is required", err), err
	}
	if req.PlanType != "free" && req.PlanType != "paid" {
		err := errors.New("invalid plan_type")
		return ErrorReply(http.StatusBadRequest, "plan_type must be 'free' or 'paid'", err), err
	}
	if req.MonthlyLimit == 0 {
		err := errors.New("invalid monthly_limit")
		return ErrorReply(http.StatusBadRequest, "monthly_limit must be greater than 0", err), err
	}
	if req.RateLimitPerSecond <= 0 {
		err := errors.New("invalid rate_limit_per_second")
		return ErrorReply(http.StatusBadRequest, "rate_limit_per_second must be greater than 0", err), err
	}
	if req.MaxTokens <= 0 {
		err := errors.New("invalid max_tokens")
		return ErrorReply(http.StatusBadRequest, "max_tokens must be greater than 0", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Check if plan name already exists
	var existing models.APIPlan
	err := db.Get().WithContext(queryCtx).
		Where("name = ?", req.Name).
		First(&existing).Error
	if err == nil {
		err := errors.New("plan name already exists")
		return ErrorReply(http.StatusConflict, "plan name already exists", err), err
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Errorf("failed to check plan name: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to check plan name", err), err
	}

	// Set default values
	planStatus := req.Status
	if planStatus == "" {
		planStatus = "active"
	}
	if planStatus != "active" && planStatus != "inactive" {
		planStatus = "active"
	}

	price := req.Price
	if price < 0 {
		price = 0.00
	}

	// Create plan
	plan := &models.APIPlan{
		Name:               req.Name,
		PlanType:           req.PlanType,
		Description:        req.Description,
		MonthlyLimit:       req.MonthlyLimit,
		RateLimitPerSecond: int(req.RateLimitPerSecond),
		MaxTokens:          int(req.MaxTokens),
		EnableWebhook:      req.EnableWebhook,
		Price:              price,
		Status:             planStatus,
	}

	err = db.Get().WithContext(queryCtx).Create(plan).Error
	if err != nil {
		global.Logger.Errorf("failed to create plan: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to create plan", err), err
	}

	// Get plan statistics (will be 0 for new plan)
	tokenCount, userCount, _ := s.getPlanStats(queryCtx, plan.ID)

	// Convert to DTO
	planInfo := models.ToPlanDTO(plan, tokenCount, userCount)

	return MarshalAndReply(planInfo, "failed to marshal response")
}

// UpdatePlan updates a plan configuration
func (s *AdminService) UpdatePlan(ctx context.Context, req *pb.UpdatePlanRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var plan models.APIPlan
	err := db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "plan not found", err), err
		}
		global.Logger.Errorf("failed to get plan: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query plan", err), err
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
		err := errors.New("no fields to update")
		return ErrorReply(http.StatusBadRequest, "no fields to update", err), err
	}

	err = db.Get().WithContext(queryCtx).Model(&plan).Updates(updates).Error
	if err != nil {
		global.Logger.Errorf("failed to update plan: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to update plan", err), err
	}

	// Reload plan
	err = db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&plan).Error
	if err != nil {
		global.Logger.Errorf("failed to reload plan: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to reload plan", err), err
	}

	// Get plan statistics
	tokenCount, userCount, _ := s.getPlanStats(queryCtx, plan.ID)

	// Convert to DTO
	planInfo := models.ToPlanDTO(&plan, tokenCount, userCount)

	return MarshalAndReply(planInfo, "failed to marshal response")
}

// GetPlanStats gets plan usage statistics
func (s *AdminService) GetPlanStats(ctx context.Context, req *pb.GetPlanStatsRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	tokenCount, userCount, err := s.getPlanStats(queryCtx, uint(req.Id))
	if err != nil {
		global.Logger.Errorf("failed to get plan stats: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get plan stats", err), err
	}

	response := &pb.PlanStatsDTO{
		TokenCount: tokenCount,
		UserCount:  userCount,
	}

	return MarshalAndReply(response, "failed to marshal response")
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
