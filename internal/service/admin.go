// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"
	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"
	"gorm.io/gorm"
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
		return ErrorReply(http.StatusInternalServerError, "failed to query error requests", err), err
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
		return ErrorReply(http.StatusInternalServerError, "failed to query error requests", err), err
	}

	// Convert to DTOs
	errors := models.ToUsageErrorDTOs(logs)

	// Build paginated response using buildPaginatedResponse
	response := buildPaginatedResponse(errors, pagination.Page, pagination.PageSize, total, true, false, 0, "", len(logs))

	return MarshalAndReply(response, "marshal response error")
}

// ListUserTokens lists all tokens for a specific user
func (s *AdminService) ListUserTokens(ctx context.Context, req *pb.ListUserTokensRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	// Validate user_id
	if req.UserId == "" {
		err := errors.New("user_id is required")
		return ErrorReply(http.StatusBadRequest, "user_id is required", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Query all tokens for the user
	var tokens []models.APIToken
	err := db.Get().WithContext(queryCtx).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Order("created_at DESC").
		Find(&tokens).Error

	if err != nil {
		global.Logger.Errorf("failed to query user tokens for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query user tokens", err), err
	}

	// Convert to DTOs using the ToDTOs method
	tokenDTOs := models.ToTokenDTOs(tokens)

	response := buildPaginatedResponse(tokenDTOs, 1, 100, int64(len(tokenDTOs)), true, false, 0, "", len(tokenDTOs))
	return MarshalAndReply(response, "marshal list user tokens response error")
}

// GetUserTokenStats gets token statistics for a specific user
func (s *AdminService) GetUserTokenStats(ctx context.Context, req *pb.GetUserTokenStatsRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	// Validate user_id
	if req.UserId == "" {
		err := errors.New("user_id is required")
		return ErrorReply(http.StatusBadRequest, "user_id is required", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Get token counts by status
	var tokenStats struct {
		Total    int64
		Active   int64
		Disabled int64
	}

	err := db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Count(&tokenStats.Total).Error
	if err != nil {
		global.Logger.Errorf("failed to count user tokens for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query token statistics", err), err
	}

	err = db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND status = ? AND deleted_at IS NULL", req.UserId, "active").
		Count(&tokenStats.Active).Error
	if err != nil {
		global.Logger.Errorf("failed to count active tokens for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query token statistics", err), err
	}

	err = db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND status = ? AND deleted_at IS NULL", req.UserId, "disabled").
		Count(&tokenStats.Disabled).Error
	if err != nil {
		global.Logger.Errorf("failed to count disabled tokens for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query token statistics", err), err
	}

	// Get usage statistics from user_usage_stats
	var userStats models.UserUsageStats
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ?", req.UserId).
		First(&userStats).Error

	monthlyUsed := uint64(0)
	monthlyLimit := uint64(100000)
	totalUsage := uint64(0)
	planType := "free"

	if err == nil {
		monthlyUsed = userStats.MonthlyUsed
		monthlyLimit = userStats.MonthlyLimit
		totalUsage = userStats.TotalUsage
	} else if err != gorm.ErrRecordNotFound {
		global.Logger.Errorf("failed to query user stats for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query user usage statistics", err), err
	}

	// Get user's plan type from one of their tokens
	var token models.APIToken
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Order("created_at DESC").
		First(&token).Error

	if err == nil {
		planType = token.PlanType
	}

	stats := &pb.UserTokenStatsDTO{
		TokenCount:         int32(tokenStats.Total),
		ActiveTokenCount:   int32(tokenStats.Active),
		DisabledTokenCount: int32(tokenStats.Disabled),
		MonthlyUsed:        monthlyUsed,
		MonthlyLimit:       monthlyLimit,
		TotalUsage:         totalUsage,
		PlanType:           planType,
	}

	return MarshalAndReply(stats, "marshal user token stats response error")
}
