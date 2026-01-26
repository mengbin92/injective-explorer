// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
)

const (
	// API token configuration constants
	apiTokenQueryTimeout      = 5 * time.Second  // Default timeout for database queries
	apiTokenQueryLongTimeout  = 10 * time.Second // Longer timeout for complex queries
	apiTokenQueryExportTimeout = 30 * time.Second // Timeout for export operations

	// Token generation constants
	apiTokenRandomBytesSize = 32 // Size of random bytes for token generation (256 bits)
	apiTokenNameMinLength   = 1  // Minimum token name length
	apiTokenNameMaxLength   = 50 // Maximum token name length
	apiTokenDefaultMaxCount = 3  // Default maximum number of tokens per user (Free plan)
)

// APITokenService implements the API token management service.
// It provides methods to create, list, delete tokens and query token usage statistics.
type APITokenService struct {
	pb.UnimplementedAPITokenServiceServer
}

// NewAPITokenService creates a new instance of APITokenService.
//
// Returns:
//   - *APITokenService: A new service instance
func NewAPITokenService() *APITokenService {
	return &APITokenService{}
}

// CreateToken creates a new API token for the authenticated user.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing token name
//
// Returns:
//   - *pb.Reply: Response containing the created token (shown only once)
//   - error: Error if the operation fails
func (s *APITokenService) CreateToken(ctx context.Context, req *pb.CreateTokenRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Validate name
	if len(req.Name) < apiTokenNameMinLength || len(req.Name) > apiTokenNameMaxLength {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Token name must be %d-%d characters", apiTokenNameMinLength, apiTokenNameMaxLength))
	}

	// Get user ID from HTTP header (set by external auth system)
	// In production, this should validate JWT or session token
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	// Check user token count
	var count int64
	queryCtx, queryCancel := context.WithTimeout(context.Background(), apiTokenQueryTimeout)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Count(&count).Error
	if err != nil {
		global.Logger.Errorf("failed to count user tokens: %v", err)
		return nil, status.Error(codes.Internal, "failed to query tokens")
	}

	// Get user's plan to check max tokens
	// For now, default to apiTokenDefaultMaxCount tokens (Free plan)
	maxTokens := apiTokenDefaultMaxCount
	if count >= int64(maxTokens) {
		return nil, status.Error(codes.ResourceExhausted,
			fmt.Sprintf("Maximum token limit reached (%d tokens)", maxTokens))
	}

	// Generate token
	token, tokenHash, tokenPreview, err := generateAPIToken()
	if err != nil {
		global.Logger.Errorf("failed to generate token: %v", err)
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	// Create token record
	apiToken := &models.APIToken{
		UserID:       userID,
		Name:         req.Name,
		TokenHash:    tokenHash,
		TokenPreview: tokenPreview,
		PlanType:     "free", // Default to free plan
		Status:       "active",
	}

	err = db.Get().WithContext(queryCtx).Create(apiToken).Error
	if err != nil {
		global.Logger.Errorf("failed to create token: %v", err)
		return nil, status.Error(codes.Internal, "failed to create token")
	}

	response := &pb.CreateTokenResponse{
		Token:        token,
		TokenPreview: tokenPreview,
		TokenId:      int64(apiToken.ID),
		Message:      "Token created successfully",
	}

	return MarshalAndReply(response, "marshal create token response error")
}

// ListTokens lists all tokens for the authenticated user.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Empty request (user ID is extracted from context)
//
// Returns:
//   - *pb.Reply: Response containing list of tokens
//   - error: Error if the operation fails
func (s *APITokenService) ListTokens(ctx context.Context, req *pb.ListTokensRequest) (*pb.Reply, error) {
	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), apiTokenQueryTimeout)
	defer queryCancel()

	var tokens []models.APIToken
	err := db.Get().WithContext(queryCtx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Find(&tokens).Error
	if err != nil {
		global.Logger.Errorf("failed to query tokens: %v", err)
		return nil, status.Error(codes.Internal, "failed to query tokens")
	}

	// Convert to protobuf response
	tokenInfos := make([]*pb.TokenInfo, 0, len(tokens))
	for _, token := range tokens {
		lastUsedAt := ""
		if token.LastUsedAt != nil {
			lastUsedAt = token.LastUsedAt.Format(time.RFC3339)
		}

		tokenInfos = append(tokenInfos, &pb.TokenInfo{
			Id:           int64(token.ID),
			Name:         token.Name,
			TokenPreview: token.TokenPreview,
			PlanType:     token.PlanType,
			Status:       token.Status,
			CreatedAt:    token.CreatedAt.Format(time.RFC3339),
			LastUsedAt:   lastUsedAt,
		})
	}

	response := &pb.ListTokensResponse{
		Tokens: tokenInfos,
	}

	return MarshalAndReply(response, "marshal list tokens response error")
}

// DeleteToken deletes a token owned by the authenticated user.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing token ID to delete
//
// Returns:
//   - *pb.Reply: Response confirming deletion
//   - error: Error if the operation fails (token not found, not owned by user, etc.)
func (s *APITokenService) DeleteToken(ctx context.Context, req *pb.DeleteTokenRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), apiTokenQueryTimeout)
	defer queryCancel()

	// Verify ownership
	var token models.APIToken
	err := db.Get().WithContext(queryCtx).
		Where("id = ? AND user_id = ?", req.TokenId, userID).
		First(&token).Error
	if err == gorm.ErrRecordNotFound {
		return nil, status.Error(codes.NotFound, "Token not found")
	}
	if err != nil {
		global.Logger.Errorf("failed to query token: %v", err)
		return nil, status.Error(codes.Internal, "failed to query token")
	}

	// Soft delete
	err = db.Get().WithContext(queryCtx).Delete(&token).Error
	if err != nil {
		global.Logger.Errorf("failed to delete token: %v", err)
		return nil, status.Error(codes.Internal, "failed to delete token")
	}

	response := &pb.DeleteTokenResponse{
		Message: "Token deleted successfully",
	}

	return MarshalAndReply(response, "marshal delete token response error")
}

// GetTokenUsage gets token usage statistics for a token owned by the authenticated user.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing token ID
//
// Returns:
//   - *pb.Reply: Response containing usage statistics
//   - error: Error if the operation fails (token not found, not owned by user, etc.)
func (s *APITokenService) GetTokenUsage(ctx context.Context, req *pb.GetTokenUsageRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), apiTokenQueryTimeout)
	defer queryCancel()

	// Verify token ownership
	var token models.APIToken
	err := db.Get().WithContext(queryCtx).
		Where("id = ? AND user_id = ?", req.TokenId, userID).
		First(&token).Error
	if err == gorm.ErrRecordNotFound {
		return nil, status.Error(codes.NotFound, "Token not found")
	}
	if err != nil {
		global.Logger.Errorf("failed to query token: %v", err)
		return nil, status.Error(codes.Internal, "failed to query token")
	}

	// Query usage statistics
	var totalCalls, successCalls, failedCalls int64
	err = db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("token_id = ?", req.TokenId).
		Count(&totalCalls).Error
	if err != nil {
		global.Logger.Errorf("failed to count total calls: %v", err)
		return nil, status.Error(codes.Internal, "failed to query usage")
	}

	err = db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("token_id = ? AND status_code >= 200 AND status_code < 300", req.TokenId).
		Count(&successCalls).Error
	if err != nil {
		global.Logger.Errorf("failed to count success calls: %v", err)
		return nil, status.Error(codes.Internal, "failed to query usage")
	}

	err = db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("token_id = ? AND status_code >= 400", req.TokenId).
		Count(&failedCalls).Error
	if err != nil {
		global.Logger.Errorf("failed to count failed calls: %v", err)
		return nil, status.Error(codes.Internal, "failed to query usage")
	}

	lastUsedAt := ""
	if token.LastUsedAt != nil {
		lastUsedAt = token.LastUsedAt.Format(time.RFC3339)
	}

	response := &pb.GetTokenUsageResponse{
		TotalCalls:   totalCalls,
		SuccessCalls: successCalls,
		FailedCalls:  failedCalls,
		LastUsedAt:   lastUsedAt,
	}

	return MarshalAndReply(response, "marshal token usage response error")
}

// generateAPIToken generates a new API token.
// Format: byh_ + 64 hex characters (hex encoded).
// The token is hashed using SHA256 for storage, and a preview is generated for display.
//
// Returns:
//   - token: The full token string (byh_xxx...)
//   - tokenHash: The SHA256 hash of the token (for storage)
//   - tokenPreview: The token preview (byh_xxx...xxx)
//   - err: Error if generation fails
func generateAPIToken() (token, tokenHash, tokenPreview string, err error) {
	// Generate random bytes for token
	randomBytes := make([]byte, apiTokenRandomBytesSize)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", "", err
	}

	// Hex encoding (64 characters for 32 bytes)
	encoded := hex.EncodeToString(randomBytes)
	token = "byh_" + encoded

	// Calculate SHA256 hash
	hash := sha256.Sum256([]byte(token))
	tokenHash = hex.EncodeToString(hash[:])

	// Generate preview (first 9 chars + ... + last 6 chars)
	const tokenPreviewPrefixLen = 9
	const tokenPreviewSuffixLen = 6
	const tokenMinPreviewLen = 15

	if len(token) > tokenMinPreviewLen {
		tokenPreview = token[:tokenPreviewPrefixLen] + "..." + token[len(token)-tokenPreviewSuffixLen:]
	} else {
		tokenPreview = token
	}

	return token, tokenHash, tokenPreview, nil
}

// getUserIDFromContext extracts user_id from context or HTTP header.
// Priority: context value (set by middleware) > HTTP header X-User-ID.
//
// Parameters:
//   - ctx: Context that may contain user_id or HTTP request
//
// Returns:
//   - string: The user ID, or empty string if not found
func (s *APITokenService) getUserIDFromContext(ctx context.Context) string {
	// First try to get from context (set by middleware)
	if userID, ok := ctx.Value("user_id").(string); ok && userID != "" {
		return userID
	}

	// Fallback to HTTP header (for token management endpoints)
	if request, ok := khttp.RequestFromServerContext(ctx); ok {
		return request.Header.Get("X-User-ID")
	}

	return ""
}

// GetDashboardStats gets dashboard statistics for the authenticated user.
// Returns aggregated usage statistics including daily stats and monthly quota information.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing number of days to query (default: 30)
//
// Returns:
//   - *pb.Reply: Response containing dashboard statistics
//   - error: Error if the operation fails
func (s *APITokenService) GetDashboardStats(ctx context.Context, req *pb.GetDashboardStatsRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	// Default to 30 days if not specified
	days := int(req.Days)
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365 // Cap at 1 year
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), apiTokenQueryLongTimeout)
	defer queryCancel()

	// Calculate date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	// Query total statistics for the period
	var totalCalls, successCalls, failedCalls int64
	err := db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startDate, endDate).
		Count(&totalCalls).Error
	if err != nil {
		global.Logger.Errorf("failed to count total calls: %v", err)
		return nil, status.Error(codes.Internal, "failed to query statistics")
	}

	err = db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ? AND status_code >= 200 AND status_code < 300", userID, startDate, endDate).
		Count(&successCalls).Error
	if err != nil {
		global.Logger.Errorf("failed to count success calls: %v", err)
		return nil, status.Error(codes.Internal, "failed to query statistics")
	}

	err = db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ? AND status_code >= 400", userID, startDate, endDate).
		Count(&failedCalls).Error
	if err != nil {
		global.Logger.Errorf("failed to count failed calls: %v", err)
		return nil, status.Error(codes.Internal, "failed to query statistics")
	}

	// Query monthly quota information from user_usage_stats
	var userStats models.UserUsageStats
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ?", userID).
		First(&userStats).Error
	monthlyLimit := uint64(DefaultAPITokenMonthlyLimit) // Default limit
	monthlyUsed := uint64(0)
	if err == nil {
		monthlyLimit = userStats.MonthlyLimit
		monthlyUsed = userStats.MonthlyUsed
	} else if err != gorm.ErrRecordNotFound {
		global.Logger.Errorf("failed to query user stats: %v", err)
		// Continue with defaults
	}

	// Query daily statistics from usage_daily_stats table
	var dailyStatsList []models.UsageDailyStats
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ? AND stat_date >= ? AND stat_date < ?", userID, startDate, endDate).
		Order("stat_date ASC").
		Find(&dailyStatsList).Error
	if err != nil {
		global.Logger.Errorf("failed to query daily stats: %v", err)
		return nil, status.Error(codes.Internal, "failed to query daily statistics")
	}

	// Convert to protobuf format
	dailyStats := make([]*pb.DailyStats, 0, len(dailyStatsList))
	for _, stat := range dailyStatsList {
		dailyStats = append(dailyStats, &pb.DailyStats{
			Date:         stat.StatDate.Format("2006-01-02"),
			TotalCalls:   int64(stat.TotalCalls),
			SuccessCalls: int64(stat.SuccessCalls),
			FailedCalls:  int64(stat.FailedCalls),
		})
	}

	response := &pb.GetDashboardStatsResponse{
		TotalCalls:   totalCalls,
		SuccessCalls: successCalls,
		FailedCalls:  failedCalls,
		MonthlyLimit: int64(monthlyLimit),
		MonthlyUsed:  int64(monthlyUsed),
		DailyStats:   dailyStats,
	}

	return MarshalAndReply(response, "marshal dashboard stats response error")
}

// ExportUsageLogs exports API usage logs to CSV format.
// Returns CSV data in the response, similar to ExportIPList.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing optional filters
//
// Returns:
//   - *pb.Reply: Response containing CSV data in data.csv_data field
//   - error: Error if export fails
func (s *APITokenService) ExportUsageLogs(ctx context.Context, req *pb.ExportUsageLogsRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Get user ID from context
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), apiTokenQueryExportTimeout)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("user_id = ?", userID)

	// Apply time range filter if provided
	if req.TimeRange != "" {
		now := time.Now()
		switch req.TimeRange {
		case "today":
			query = query.Where("created_at >= ?", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
		case "7days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast7Days))
		case "30days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -PeriodLast30Days))
		}
	}

	// Query logs
	var logs []models.APIRequestLog
	err := query.Order("created_at DESC").Find(&logs).Error
	if err != nil {
		global.Logger.Errorf("failed to query usage logs: %v", err)
		return nil, status.Error(codes.Internal, "failed to query usage logs")
	}

	// Build CSV using strings.Builder and encoding/csv
	var builder strings.Builder
	writer := csv.NewWriter(&builder)

	// Write header
	if err := writer.Write([]string{"时间", "IP地址", "IP归属地", "接口", "方法", "状态码", "响应时间(ms)", "错误信息"}); err != nil {
		global.Logger.Errorf("failed to write CSV header: %v", err)
		return nil, status.Error(codes.Internal, "failed to generate CSV")
	}

	// Write data rows
	for _, log := range logs {
		timeStr := log.CreatedAt.Format("2006-01-02 15:04:05")
		if err := writer.Write([]string{
			timeStr,
			log.IPAddress,
			log.IPLocation,
			log.Endpoint,
			log.Method,
			fmt.Sprintf("%d", log.StatusCode),
			fmt.Sprintf("%d", log.ResponseTime),
			log.ErrorMessage, // encoding/csv will handle escaping automatically
		}); err != nil {
			global.Logger.Errorf("failed to write CSV row: %v", err)
			return nil, status.Error(codes.Internal, "failed to generate CSV")
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		global.Logger.Errorf("failed to flush CSV: %v", err)
		return nil, status.Error(codes.Internal, "failed to generate CSV")
	}

	response := map[string]interface{}{
		"csv_data": builder.String(),
	}

	data, err := structpb.NewStruct(response)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
		Data:    data,
	}, nil
}

// GetActiveTokenCount gets the count of active tokens for the authenticated user.
// A token is considered active if it has been used within the specified number of days.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing days threshold (default: 30)
//
// Returns:
//   - *pb.Reply: Response containing total and active token counts
//   - error: Error if the operation fails
func (s *APITokenService) GetActiveTokenCount(ctx context.Context, req *pb.GetActiveTokenCountRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	// Default to 30 days if not specified
	days := int(req.Days)
	if days <= 0 {
		days = 30
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), apiTokenQueryTimeout)
	defer queryCancel()

	// Calculate the threshold date
	thresholdDate := time.Now().AddDate(0, 0, -days)

	// Count total tokens (not deleted)
	var totalTokens int64
	err := db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Count(&totalTokens).Error
	if err != nil {
		global.Logger.Errorf("failed to count total tokens: %v", err)
		return nil, status.Error(codes.Internal, "failed to query tokens")
	}

	// Count active tokens (used within N days)
	var activeTokens int64
	err = db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL AND status = ? AND last_used_at IS NOT NULL AND last_used_at >= ?",
			userID, "active", thresholdDate).
		Count(&activeTokens).Error
	if err != nil {
		global.Logger.Errorf("failed to count active tokens: %v", err)
		return nil, status.Error(codes.Internal, "failed to query active tokens")
	}

	response := &pb.GetActiveTokenCountResponse{
		TotalTokens:  int32(totalTokens),
		ActiveTokens: int32(activeTokens),
	}

	return MarshalAndReply(response, "marshal active token count response error")
}

// GetErrorCodeAnalysis gets error code analysis for the authenticated user.
// Returns statistics grouped by HTTP status code, including count, percentage,
// and most common error message for each error code.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing optional token_id filter and days threshold
//
// Returns:
//   - *pb.Reply: Response containing error code statistics
//   - error: Error if the operation fails
func (s *APITokenService) GetErrorCodeAnalysis(ctx context.Context, req *pb.GetErrorCodeAnalysisRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	// Default to 30 days if not specified
	days := int(req.Days)
	if days <= 0 {
		days = 30
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), apiTokenQueryLongTimeout)
	defer queryCancel()

	// Calculate the date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.APIRequestLog{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ? AND status_code >= 400",
			userID, startDate, endDate)

	// Filter by token if specified
	if req.TokenId > 0 {
		// Verify token ownership
		var token models.APIToken
		err := db.Get().WithContext(queryCtx).
			Where("id = ? AND user_id = ?", req.TokenId, userID).
			First(&token).Error
		if err == gorm.ErrRecordNotFound {
			return nil, status.Error(codes.NotFound, "Token not found")
		}
		if err != nil {
			global.Logger.Errorf("failed to verify token ownership: %v", err)
			return nil, status.Error(codes.Internal, "failed to verify token")
		}
		query = query.Where("token_id = ?", req.TokenId)
	}

	// Query error code statistics with most common message
	type ErrorCodeResult struct {
		StatusCode        int
		Count             int64
		MostCommonMessage string
	}

	var results []ErrorCodeResult
	err := query.Select("status_code, COUNT(*) as count, "+
		"SUBSTRING_INDEX(GROUP_CONCAT(error_message ORDER BY created_at DESC SEPARATOR '||'), '||', 1) as most_common_message").
		Group("status_code").
		Order("count DESC").
		Find(&results).Error
	if err != nil {
		global.Logger.Errorf("failed to query error code analysis: %v", err)
		return nil, status.Error(codes.Internal, "failed to query error statistics")
	}

	// Calculate total errors for percentage calculation
	var totalErrors int64
	for _, result := range results {
		totalErrors += result.Count
	}

	// Build response
	errorStats := make([]*pb.ErrorCodeStats, 0, len(results))
	for _, result := range results {
		percentage := 0.0
		if totalErrors > 0 {
			percentage = float64(result.Count) / float64(totalErrors) * 100
		}

		errorStats = append(errorStats, &pb.ErrorCodeStats{
			StatusCode:         int32(result.StatusCode),
			Count:              result.Count,
			Percentage:         percentage,
			MostCommonMessage:  result.MostCommonMessage,
		})
	}

	response := &pb.GetErrorCodeAnalysisResponse{
		ErrorStats:  errorStats,
		TotalErrors: totalErrors,
	}

	return MarshalAndReply(response, "marshal error code analysis response error")
}
