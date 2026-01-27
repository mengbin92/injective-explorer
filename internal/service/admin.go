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

// GetUsageErrors retrieves a paginated list of API error requests (status code >= 400).
// It supports filtering by status code, time range, and pagination parameters.
// Returns a *pb.Reply containing a list of UsageErrorDTO with pagination metadata.
//
// Supported time ranges: today, 7days, 30days
// Default pagination: page=1, page_size=20, max page_size=100
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
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast7Days))
		case "30days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast30Days))
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

// ListUserTokens retrieves all tokens for a specific user with pagination support.
// It returns a paginated list of *pb.TokenDTO packed in a standard *pb.Reply.
//
// This function implements proper pagination with configurable page and page size,
// following RESTful API best practices. Default values are page=1 and page_size=20,
// with a maximum page_size of 100.
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

	// Initialize pagination parameters with defaults
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

	// Apply page size limits: minimum 1, maximum 100
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 100 {
		pageSize = 100
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Get total count for pagination metadata
	var totalCount int64
	err := db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Count(&totalCount).Error
	if err != nil {
		global.Logger.Errorf("failed to count user tokens for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query token count", err), err
	}

	// Query tokens for the user with pagination
	offset := (page - 1) * pageSize
	var tokens []models.APIToken
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&tokens).Error

	if err != nil {
		global.Logger.Errorf("failed to query user tokens for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query user tokens", err), err
	}

	// Convert to DTOs using the ToDTOs method
	tokenDTOs := models.ToTokenDTOs(tokens)

	// Calculate pagination metadata
	hasNext := (int64(offset) + int64(len(tokens))) < totalCount
	hasPrev := page > 1

	response := buildPaginatedResponse(tokenDTOs, page, pageSize, totalCount, hasNext, hasPrev, uint64(offset), "", len(tokens))
	return MarshalAndReply(response, "marshal list user tokens response error")
}

// GetUserTokenStats retrieves aggregated statistics for a user's tokens,
// including total count, status breakdown, usage data, and plan information.
// It returns a *pb.UserTokenStatsDTO packed in a standard *pb.Reply.
//
// The function optimizes database queries by using a single aggregation query
// for token statistics instead of multiple separate count queries.
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

	// Get token counts by status using a single optimized aggregate query
	// This replaces three separate Count queries with one aggregation
	var tokenStats struct {
		Total    int64
		Active   int64
		Disabled int64
	}

	err := db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Select("COUNT(id) as total, SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) as active, SUM(CASE WHEN status = 'disabled' THEN 1 ELSE 0 END) as disabled").
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Row().Scan(&tokenStats.Total, &tokenStats.Active, &tokenStats.Disabled)
	if err != nil {
		global.Logger.Errorf("failed to count user tokens for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query token statistics", err), err
	}

	// Get usage statistics from user_usage_stats
	var userStats models.UserUsageStats
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ?", req.UserId).
		First(&userStats).Error

	monthlyUsed := uint64(0)
	monthlyLimit := uint64(0)
	totalUsage := uint64(0)

	if err == nil {
		monthlyUsed = userStats.MonthlyUsed
		monthlyLimit = userStats.MonthlyLimit
		totalUsage = userStats.TotalUsage
	} else if err != gorm.ErrRecordNotFound {
		global.Logger.Errorf("failed to query user stats for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query user usage statistics", err), err
	}

	// Get user's plan type from their most recently created token
	var planType string
	err = db.Get().WithContext(queryCtx).
		Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Order("created_at DESC").
		Limit(1).
		Pluck("plan_type", &planType).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		global.Logger.Errorf("failed to query plan type for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query plan type", err), err
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

// GetUsageStats retrieves usage statistics overview.
func (s *AdminService) GetUsageStats(ctx context.Context, req *pb.GetUsageStatsRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build base query
	query := db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{})

	// Apply time range filter
	now := time.Now()
	if req.TimeRange != "" && req.TimeRange != "all" {
		switch req.TimeRange {
		case "today":
			query = query.Where("created_at >= ?", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
		case "7days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast7Days))
		case "30days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast30Days))
		}
	}

	// Get statistics using aggregation
	type UsageStats struct {
		TotalRequests   int64
		SuccessRequests int64
		FailedRequests  int64
		AvgResponseTime int64
		UniqueUsers     int64
		UniqueIPs       int64
	}

	var stats UsageStats
	err := query.Select(
		"COUNT(*) as total_requests",
		"SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 ELSE 0 END) as success_requests",
		"SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as failed_requests",
		"AVG(response_time) as avg_response_time",
		"COUNT(DISTINCT user_id) as unique_users",
		"COUNT(DISTINCT ip_address) as unique_ips",
	).Row().Scan(
		&stats.TotalRequests,
		&stats.SuccessRequests,
		&stats.FailedRequests,
		&stats.AvgResponseTime,
		&stats.UniqueUsers,
		&stats.UniqueIPs,
	)

	if err != nil {
		global.Logger.Errorf("failed to get usage stats: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query usage statistics", err), err
	}

	// Calculate error rate
	errorRate := 0.0
	if stats.TotalRequests > 0 {
		errorRate = float64(stats.FailedRequests) / float64(stats.TotalRequests) * 100
	}

	response := &pb.UsageStatsDTO{
		TotalRequests:   stats.TotalRequests,
		SuccessRequests: stats.SuccessRequests,
		FailedRequests:  stats.FailedRequests,
		ErrorRate:       errorRate,
		AvgResponseTime: stats.AvgResponseTime,
		UniqueUsers:     stats.UniqueUsers,
		UniqueIps:       stats.UniqueIPs,
	}

	return MarshalAndReply(response, "marshal usage stats response error")
}

// GetUsageTrends retrieves usage trends over time.
func (s *AdminService) GetUsageTrends(ctx context.Context, req *pb.GetUsageTrendsRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	// Set defaults
	timeRange := req.TimeRange
	if timeRange == "" {
		timeRange = "7days"
	}
	interval := req.Interval
	if interval == "" {
		interval = "day"
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Determine date range and grouping
	now := time.Now()
	var startDate time.Time
	var days int

	switch timeRange {
	case "7days":
		days = PeriodLast7Days
	case "30days":
		days = PeriodLast30Days
	case "90days":
		days = 90
	default:
		days = PeriodLast7Days
	}

	startDate = now.AddDate(0, 0, -days)

	// Build query with date grouping
	groupBy := "DATE(created_at)"
	if interval == "week" {
		groupBy = "DATE_FORMAT(created_at, '%Y-%u')"
	} else if interval == "month" {
		groupBy = "DATE_FORMAT(created_at, '%Y-%m')"
	}

	type TrendResult struct {
		Date            string
		TotalRequests   int64
		SuccessRequests int64
		FailedRequests  int64
		AvgResponseTime int64
	}

	var results []TrendResult
	err := db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Select(
			groupBy+" as date",
			"COUNT(*) as total_requests",
			"SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 ELSE 0 END) as success_requests",
			"SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as failed_requests",
			"AVG(response_time) as avg_response_time",
		).
		Where("created_at >= ?", startDate).
		Group(groupBy).
		Order("date ASC").
		Scan(&results).Error

	if err != nil {
		global.Logger.Errorf("failed to get usage trends: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query usage trends", err), err
	}

	// Convert to DTOs
	trends := make([]*pb.UsageTrendDTO, 0, len(results))
	for _, r := range results {
		trends = append(trends, &pb.UsageTrendDTO{
			Date:            r.Date,
			TotalRequests:   r.TotalRequests,
			SuccessRequests: r.SuccessRequests,
			FailedRequests:  r.FailedRequests,
			AvgResponseTime: r.AvgResponseTime,
		})
	}

	return MarshalAndReply(trends, "marshal usage trends response error")
}

// GetUsageByEndpoint retrieves usage statistics grouped by endpoint.
func (s *AdminService) GetUsageByEndpoint(ctx context.Context, req *pb.GetUsageByEndpointRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	// Set default top_n
	topN := req.TopN
	if topN == 0 || topN < 1 {
		topN = 10
	}
	if topN > 100 {
		topN = 100
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build base query
	query := db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{})

	// Apply time range filter
	now := time.Now()
	if req.TimeRange != "" && req.TimeRange != "all" {
		switch req.TimeRange {
		case "today":
			query = query.Where("created_at >= ?", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
		case "7days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast7Days))
		case "30days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast30Days))
		}
	}

	// Get total requests for error rate calculation
	var totalRequests int64
	err := query.Count(&totalRequests).Error
	if err != nil {
		global.Logger.Errorf("failed to count total requests: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query endpoint usage", err), err
	}

	type EndpointResult struct {
		Endpoint        string
		Method          string
		RequestCount    int64
		SuccessCount    int64
		FailedCount     int64
		AvgResponseTime int64
	}

	var results []EndpointResult
	err = query.Select(
		"endpoint",
		"method",
		"COUNT(*) as request_count",
		"SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 ELSE 0 END) as success_count",
		"SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as failed_count",
		"AVG(response_time) as avg_response_time",
	).
		Group("endpoint, method").
		Order("request_count DESC").
		Limit(int(topN)).
		Scan(&results).Error

	if err != nil {
		global.Logger.Errorf("failed to get usage by endpoint: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query endpoint usage", err), err
	}

	// Convert to DTOs
	endpoints := make([]*pb.EndpointUsageDTO, 0, len(results))
	for _, r := range results {
		errorRate := 0.0
		if r.RequestCount > 0 {
			errorRate = float64(r.FailedCount) / float64(r.RequestCount) * 100
		}

		endpoints = append(endpoints, &pb.EndpointUsageDTO{
			Endpoint:        r.Endpoint,
			Method:          r.Method,
			RequestCount:    r.RequestCount,
			SuccessCount:    r.SuccessCount,
			FailedCount:     r.FailedCount,
			ErrorRate:       errorRate,
			AvgResponseTime: r.AvgResponseTime,
		})
	}

	return MarshalAndReply(endpoints, "marshal endpoint usage response error")
}

// GetUsageByErrorCode retrieves usage statistics grouped by HTTP status code.
func (s *AdminService) GetUsageByErrorCode(ctx context.Context, req *pb.GetUsageByErrorCodeRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build base query - only error requests (status code >= 400)
	query := db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("status_code >= ?", 400)

	// Apply time range filter
	now := time.Now()
	if req.TimeRange != "" && req.TimeRange != "all" {
		switch req.TimeRange {
		case "today":
			query = query.Where("created_at >= ?", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
		case "7days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast7Days))
		case "30days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast30Days))
		}
	}

	// Get total error count
	var totalErrors int64
	err := query.Count(&totalErrors).Error
	if err != nil {
		global.Logger.Errorf("failed to count total errors: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query error code usage", err), err
	}

	if totalErrors == 0 {
		return MarshalAndReply([]*pb.ErrorCodeUsageDTO{}, "marshal error code usage response error")
	}

	// Get error counts by status code
	type ErrorCodeResult struct {
		StatusCode int32
		Count      int64
	}

	var errorResults []ErrorCodeResult
	err = query.Select("status_code, COUNT(*) as count").
		Group("status_code").
		Order("count DESC").
		Scan(&errorResults).Error

	if err != nil {
		global.Logger.Errorf("failed to get usage by error code: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query error code usage", err), err
	}

	// Convert to DTOs with top error messages
	errorCodes := make([]*pb.ErrorCodeUsageDTO, 0, len(errorResults))
	for _, er := range errorResults {
		percentage := 0.0
		if totalErrors > 0 {
			percentage = float64(er.Count) / float64(totalErrors) * 100
		}

		// Get top 5 error messages for this status code
		var topMessages []string
		err = db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
			Where("status_code = ?", er.StatusCode).
			Where("created_at >= ?", query).
			Select("error_message").
			Group("error_message").
			Order("COUNT(*) DESC").
			Limit(5).
			Pluck("error_message", &topMessages).Error

		if err != nil {
			global.Logger.Warnf("failed to get top error messages for status code %d: %v", er.StatusCode, err)
			topMessages = []string{}
		}

		errorCodes = append(errorCodes, &pb.ErrorCodeUsageDTO{
			StatusCode:        er.StatusCode,
			Count:             er.Count,
			Percentage:        percentage,
			TopErrorMessages:  topMessages,
		})
	}

	return MarshalAndReply(errorCodes, "marshal error code usage response error")
}
