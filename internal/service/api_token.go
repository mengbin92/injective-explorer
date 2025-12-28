// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
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
//   - *pb.CreateTokenResponse: Response containing the created token (shown only once)
//   - error: Error if the operation fails
func (s *APITokenService) CreateToken(ctx context.Context, req *pb.CreateTokenRequest) (*pb.CreateTokenResponse, error) {
	// Input validation
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Validate name
	if len(req.Name) == 0 || len(req.Name) > 50 {
		return nil, status.Error(codes.InvalidArgument, "Token name must be 1-50 characters")
	}

	// Get user ID from HTTP header (set by external auth system)
	// In production, this should validate JWT or session token
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	// Check user token count
	var count int64
	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Count(&count).Error
	if err != nil {
		global.Logger.Errorf("failed to count user tokens: %v", err)
		return nil, status.Error(codes.Internal, "failed to query tokens")
	}

	// Get user's plan to check max tokens
	// For now, default to 3 tokens (Free plan)
	maxTokens := 3
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

	return &pb.CreateTokenResponse{
		Token:        token,
		TokenPreview: tokenPreview,
		TokenId:      int64(apiToken.ID),
		Message:      "Token created successfully",
	}, nil
}

// ListTokens lists all tokens for the authenticated user.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Empty request (user ID is extracted from context)
//
// Returns:
//   - *pb.ListTokensResponse: Response containing list of tokens
//   - error: Error if the operation fails
func (s *APITokenService) ListTokens(ctx context.Context, req *pb.ListTokensRequest) (*pb.ListTokensResponse, error) {
	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	return &pb.ListTokensResponse{
		Tokens: tokenInfos,
	}, nil
}

// DeleteToken deletes a token owned by the authenticated user.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing token ID to delete
//
// Returns:
//   - *pb.DeleteTokenResponse: Response confirming deletion
//   - error: Error if the operation fails (token not found, not owned by user, etc.)
func (s *APITokenService) DeleteToken(ctx context.Context, req *pb.DeleteTokenRequest) (*pb.DeleteTokenResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	return &pb.DeleteTokenResponse{
		Message: "Token deleted successfully",
	}, nil
}

// GetTokenUsage gets token usage statistics for a token owned by the authenticated user.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing token ID
//
// Returns:
//   - *pb.GetTokenUsageResponse: Response containing usage statistics
//   - error: Error if the operation fails (token not found, not owned by user, etc.)
func (s *APITokenService) GetTokenUsage(ctx context.Context, req *pb.GetTokenUsageRequest) (*pb.GetTokenUsageResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	// Get user ID from HTTP header
	userID := s.getUserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user_id not found. Please provide X-User-ID header")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	return &pb.GetTokenUsageResponse{
		TotalCalls:   totalCalls,
		SuccessCalls: successCalls,
		FailedCalls:  failedCalls,
		LastUsedAt:   lastUsedAt,
	}, nil
}

// generateAPIToken generates a new API token.
// Format: inj_ + 32 random characters (Base64 URL encoded).
// The token is hashed using SHA256 for storage, and a preview is generated for display.
//
// Returns:
//   - token: The full token string (inj_xxx...)
//   - tokenHash: The SHA256 hash of the token (for storage)
//   - tokenPreview: The token preview (inj_xxx...xxx)
//   - err: Error if generation fails
func generateAPIToken() (token, tokenHash, tokenPreview string, err error) {
	// Generate 32 random bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", "", err
	}

	// Base64 URL encoding
	encoded := base64.URLEncoding.EncodeToString(randomBytes)
	token = "inj_" + encoded

	// Calculate SHA256 hash
	hash := sha256.Sum256([]byte(token))
	tokenHash = hex.EncodeToString(hash[:])

	// Generate preview (first 9 chars + ... + last 6 chars)
	if len(token) > 15 {
		tokenPreview = token[:9] + "..." + token[len(token)-6:]
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
//   - *pb.GetDashboardStatsResponse: Response containing dashboard statistics
//   - error: Error if the operation fails
func (s *APITokenService) GetDashboardStats(ctx context.Context, req *pb.GetDashboardStatsRequest) (*pb.GetDashboardStatsResponse, error) {
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

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	monthlyLimit := uint64(100000) // Default limit
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
			Date:        stat.StatDate.Format("2006-01-02"),
			TotalCalls:  int64(stat.TotalCalls),
			SuccessCalls: int64(stat.SuccessCalls),
			FailedCalls:  int64(stat.FailedCalls),
		})
	}

	return &pb.GetDashboardStatsResponse{
		TotalCalls:   totalCalls,
		SuccessCalls: successCalls,
		FailedCalls:  failedCalls,
		MonthlyLimit: int64(monthlyLimit),
		MonthlyUsed:  int64(monthlyUsed),
		DailyStats:   dailyStats,
	}, nil
}

// ExportUsageLogs exports API usage logs to Excel format.
// Note: This method is reserved for future implementation.
// File downloads are handled by custom HTTP route handler in server/http.go.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request (empty, uses authenticated user from context)
//
// Returns:
//   - *pb.ExportUsageLogsResponse: Response with success message
//   - error: Error if export fails
func (s *APITokenService) ExportUsageLogs(ctx context.Context, req *pb.ExportUsageLogsRequest) (*pb.ExportUsageLogsResponse, error) {
	// This endpoint should be handled by a custom HTTP route handler for file download
	// Protobuf services are better suited for JSON responses
	return nil, status.Error(codes.Unimplemented, "Export functionality is handled via custom HTTP route at /api/v1/export/logs")
}
