// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

// ListAccounts lists all API accounts with search, filter, and pagination support
func (s *AdminService) ListAccounts(ctx context.Context, req *pb.ListAccountsRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	// Initialize pagination parameters with defaults
	pagination := req.Pagination
	if pagination == nil {
		pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: 20,
		}
	}

	page, pageSize, offset, _, _ := normalizePagination(
		pagination.Page,
		pagination.PageSize,
		"",
		MaxPageSize,
	)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).
		Table("api_tokens").
		Select("DISTINCT api_tokens.user_id").
		Where("api_tokens.deleted_at IS NULL")

	if req.Search != "" {
		query = query.Where("api_tokens.user_id LIKE ?", "%"+req.Search+"%")
	}

	var userIDs []string
	err := query.Pluck("user_id", &userIDs).Error
	if err != nil {
		global.Logger.Errorf("failed to query user IDs: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query accounts", err), err
	}

	if req.Status != "" {
		filteredUserIDs := make([]string, 0)
		for _, userID := range userIDs {
			var activeCount, disabledCount, totalCount int64
			db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
				Where("user_id = ? AND deleted_at IS NULL", userID).
				Count(&totalCount)
			db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
				Where("user_id = ? AND status = 'active' AND deleted_at IS NULL", userID).
				Count(&activeCount)
			db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
				Where("user_id = ? AND status = 'disabled' AND deleted_at IS NULL", userID).
				Count(&disabledCount)

			var accountStatus string
			if totalCount == 0 {
				accountStatus = "pending"
			} else if activeCount > 0 {
				accountStatus = "active"
			} else {
				accountStatus = "disabled"
			}

			if accountStatus == req.Status {
				filteredUserIDs = append(filteredUserIDs, userID)
			}
		}
		userIDs = filteredUserIDs
	}

	total := int64(len(userIDs))

	start := offset
	end := offset + int(pageSize)
	if start > len(userIDs) {
		start = len(userIDs)
	}
	if end > len(userIDs) {
		end = len(userIDs)
	}
	paginatedUserIDs := userIDs[start:end]

	accounts := make([]*pb.APIAccountDTO, 0, len(paginatedUserIDs))
	for _, userID := range paginatedUserIDs {
		account, err := s.buildAccountDTO(queryCtx, userID, req.UsageFilter)
		if err != nil {
			global.Logger.Warnf("failed to build account DTO for user %s: %v", userID, err)
			continue
		}
		accounts = append(accounts, account)
	}

	response := buildPaginatedResponse(accounts, page, pageSize, total, true, false, 0, "", len(accounts))

	return MarshalAndReply(response, "marshal list accounts response error")
}

func (s *AdminService) buildAccountDTO(ctx context.Context, userID string, usageFilter string) (*pb.APIAccountDTO, error) {
	var tokenCount int64
	err := db.Get().WithContext(ctx).Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Count(&tokenCount).Error
	if err != nil {
		return nil, errors.Wrap(err, "failed to count tokens")
	}

	var userStats models.UserUsageStats
	err = db.Get().WithContext(ctx).
		Where("user_id = ?", userID).
		First(&userStats).Error

	monthlyUsed := uint64(0)
	monthlyLimit := uint64(100000)
	if err == nil {
		monthlyUsed = userStats.MonthlyUsed
		monthlyLimit = userStats.MonthlyLimit
	} else if err != gorm.ErrRecordNotFound {
		return nil, errors.Wrap(err, "failed to query user stats")
	}

	usagePercentage := 0.0
	if monthlyLimit > 0 {
		usagePercentage = float64(monthlyUsed) / float64(monthlyLimit) * 100
	}

	if usageFilter != "" {
		var matches bool
		switch usageFilter {
		case "high":
			matches = usagePercentage > 80
		case "medium":
			matches = usagePercentage >= 50 && usagePercentage <= 80
		case "low":
			matches = usagePercentage < 50
		}
		if !matches {
			return nil, errors.New("usage filter does not match")
		}
	}

	var activeCount, disabledCount int64
	db.Get().WithContext(ctx).Model(&models.APIToken{}).
		Where("user_id = ? AND status = 'active' AND deleted_at IS NULL", userID).
		Count(&activeCount)
	db.Get().WithContext(ctx).Model(&models.APIToken{}).
		Where("user_id = ? AND status = 'disabled' AND deleted_at IS NULL", userID).
		Count(&disabledCount)

	status := "pending"
	if tokenCount > 0 {
		if activeCount > 0 {
			status = "active"
		} else {
			status = "disabled"
		}
	}

	var planType string
	var latestToken models.APIToken
	err = db.Get().WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		First(&latestToken).Error
	if err == nil {
		planType = latestToken.PlanType
	} else {
		planType = "free"
	}

	var createdAt time.Time
	var firstToken models.APIToken
	err = db.Get().WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at ASC").
		First(&firstToken).Error
	if err == nil {
		createdAt = firstToken.CreatedAt
	} else {
		createdAt = time.Now()
	}

	var lastActiveAt string
	var mostRecentToken models.APIToken
	err = db.Get().WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL AND last_used_at IS NOT NULL", userID).
		Order("last_used_at DESC").
		First(&mostRecentToken).Error
	if err == nil && mostRecentToken.LastUsedAt != nil {
		lastActiveAt = mostRecentToken.LastUsedAt.Format(time.RFC3339)
	}

	account := &pb.APIAccountDTO{
		UserId:          userID,
		Username:        "",
		Email:           "",
		TokenCount:      int32(tokenCount),
		MonthlyUsed:     monthlyUsed,
		MonthlyLimit:    monthlyLimit,
		UsagePercentage: usagePercentage,
		PlanType:        planType,
		Status:          status,
		CreatedAt:       createdAt.Format(time.RFC3339),
		LastActiveAt:    lastActiveAt,
	}

	return account, nil
}

// GetAccountDetail gets detailed information for a specific account
func (s *AdminService) GetAccountDetail(ctx context.Context, req *pb.GetAccountDetailRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	if req.UserId == "" {
		err := errors.New("user_id is required")
		return ErrorReply(http.StatusBadRequest, "user_id is required", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var userStats models.UserUsageStats
	err := db.Get().WithContext(queryCtx).
		Where("user_id = ?", req.UserId).
		First(&userStats).Error

	monthlyUsed := uint64(0)
	monthlyLimit := uint64(100000)
	totalUsage := uint64(0)
	if err == nil {
		monthlyUsed = userStats.MonthlyUsed
		monthlyLimit = userStats.MonthlyLimit
		totalUsage = userStats.TotalUsage
	} else if err != gorm.ErrRecordNotFound {
		global.Logger.Errorf("failed to query user stats for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query user statistics", err), err
	}

	usagePercentage := 0.0
	monthlyRemaining := uint64(0)
	if monthlyLimit > 0 {
		usagePercentage = float64(monthlyUsed) / float64(monthlyLimit) * 100
		if monthlyUsed < monthlyLimit {
			monthlyRemaining = monthlyLimit - monthlyUsed
		}
	}

	var activeCount, disabledCount, totalCount int64
	db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Count(&totalCount)
	db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND status = 'active' AND deleted_at IS NULL", req.UserId).
		Count(&activeCount)
	db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("user_id = ? AND status = 'disabled' AND deleted_at IS NULL", req.UserId).
		Count(&disabledCount)

	status := "pending"
	if totalCount > 0 {
		if activeCount > 0 {
			status = "active"
		} else {
			status = "disabled"
		}
	}

	var planType string
	var latestToken models.APIToken
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Order("created_at DESC").
		First(&latestToken).Error
	if err == nil {
		planType = latestToken.PlanType
	} else {
		planType = "free"
	}

	var createdAt time.Time
	var firstToken models.APIToken
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Order("created_at ASC").
		First(&firstToken).Error
	if err == nil {
		createdAt = firstToken.CreatedAt
	} else {
		createdAt = time.Now()
	}

	var lastActiveAt string
	var mostRecentToken models.APIToken
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ? AND deleted_at IS NULL AND last_used_at IS NOT NULL", req.UserId).
		Order("last_used_at DESC").
		First(&mostRecentToken).Error
	if err == nil && mostRecentToken.LastUsedAt != nil {
		lastActiveAt = mostRecentToken.LastUsedAt.Format(time.RFC3339)
	}

	var tokens []models.APIToken
	err = db.Get().WithContext(queryCtx).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Order("created_at DESC").
		Find(&tokens).Error
	if err != nil {
		global.Logger.Errorf("failed to query tokens for user %s: %v", req.UserId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query tokens", err), err
	}

	tokenDTOs := make([]*pb.APIAccountTokenDTO, 0, len(tokens))
	for _, token := range tokens {
		lastUsedAt := ""
		if token.LastUsedAt != nil {
			lastUsedAt = token.LastUsedAt.Format(time.RFC3339)
		}

		tokenDTO := &pb.APIAccountTokenDTO{
			Id:           int64(token.ID),
			Name:         token.Name,
			TokenPreview: token.TokenPreview,
			Status:       token.Status,
			CreatedAt:    token.CreatedAt.Format(time.RFC3339),
			LastUsedAt:   lastUsedAt,
		}
		tokenDTOs = append(tokenDTOs, tokenDTO)
	}

	detail := &pb.APIAccountDetailDTO{
		UserId:           req.UserId,
		Username:         "",
		Email:            "",
		Status:           status,
		PlanType:         planType,
		CreatedAt:        createdAt.Format(time.RFC3339),
		LastActiveAt:     lastActiveAt,
		MonthlyUsed:      monthlyUsed,
		MonthlyLimit:     monthlyLimit,
		MonthlyRemaining: monthlyRemaining,
		UsagePercentage:  usagePercentage,
		TotalUsage:       totalUsage,
		Tokens:           tokenDTOs,
	}

	return MarshalAndReply(detail, "marshal account detail response error")
}

// GetAccountStats gets account statistics
func (s *AdminService) GetAccountStats(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var totalAccounts int64
	err := db.Get().WithContext(queryCtx).
		Table("api_tokens").
		Select("COUNT(DISTINCT user_id)").
		Where("deleted_at IS NULL").
		Count(&totalAccounts).Error
	if err != nil {
		global.Logger.Errorf("failed to count total accounts: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query account statistics", err), err
	}

	var activeAccounts int64
	err = db.Get().WithContext(queryCtx).
		Table("api_tokens").
		Select("COUNT(DISTINCT user_id)").
		Where("status = 'active' AND deleted_at IS NULL").
		Count(&activeAccounts).Error
	if err != nil {
		global.Logger.Errorf("failed to count active accounts: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query account statistics", err), err
	}

	var totalTokens int64
	err = db.Get().WithContext(queryCtx).Model(&models.APIToken{}).
		Where("deleted_at IS NULL").
		Count(&totalTokens).Error
	if err != nil {
		global.Logger.Errorf("failed to count total tokens: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query account statistics", err), err
	}

	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	var monthlyActiveAccounts int64
	err = db.Get().WithContext(queryCtx).
		Table("api_tokens").
		Select("COUNT(DISTINCT user_id)").
		Where("last_used_at >= ? AND deleted_at IS NULL", thirtyDaysAgo).
		Count(&monthlyActiveAccounts).Error
	if err != nil {
		global.Logger.Errorf("failed to count monthly active accounts: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query account statistics", err), err
	}

	stats := &pb.APIAccountStatsDTO{
		TotalAccounts:         totalAccounts,
		ActiveAccounts:        activeAccounts,
		TotalTokens:           totalTokens,
		MonthlyActiveAccounts: monthlyActiveAccounts,
	}

	return MarshalAndReply(stats, "marshal account stats response error")
}

// ToggleAccountStatus enables or disables an account
func (s *AdminService) ToggleAccountStatus(ctx context.Context, req *pb.ToggleAccountStatusRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	if req.UserId == "" {
		err := errors.New("user_id is required")
		return ErrorReply(http.StatusBadRequest, "user_id is required", err), err
	}

	if req.Action != "enable" && req.Action != "disable" {
		err := errors.New("action must be 'enable' or 'disable'")
		return ErrorReply(http.StatusBadRequest, "action must be 'enable' or 'disable'", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	newStatus := "active"
	if req.Action == "disable" {
		newStatus = "disabled"
	}

	result := db.Get().WithContext(queryCtx).
		Model(&models.APIToken{}).
		Where("user_id = ? AND deleted_at IS NULL", req.UserId).
		Update("status", newStatus)

	if result.Error != nil {
		global.Logger.Errorf("failed to update account status for user %s: %v", req.UserId, result.Error)
		return ErrorReply(http.StatusInternalServerError, "failed to update account status", result.Error), result.Error
	}

	response := map[string]interface{}{
		"message": fmt.Sprintf("Account %sd successfully", req.Action),
		"user_id": req.UserId,
		"status":  newStatus,
		"updated": result.RowsAffected,
	}

	return MarshalAndReply(response, "marshal toggle account status response error")
}

// ToggleTokenStatus enables or disables a token
func (s *AdminService) ToggleTokenStatus(ctx context.Context, req *pb.ToggleTokenStatusRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	if req.TokenId == 0 {
		err := errors.New("token_id is required")
		return ErrorReply(http.StatusBadRequest, "token_id is required", err), err
	}

	if req.Action != "enable" && req.Action != "disable" {
		err := errors.New("action must be 'enable' or 'disable'")
		return ErrorReply(http.StatusBadRequest, "action must be 'enable' or 'disable'", err), err
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var token models.APIToken
	err := db.Get().WithContext(queryCtx).
		Where("id = ? AND deleted_at IS NULL", req.TokenId).
		First(&token).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrorReply(http.StatusNotFound, "token not found", err), err
		}
		global.Logger.Errorf("failed to query token %d: %v", req.TokenId, err)
		return ErrorReply(http.StatusInternalServerError, "failed to query token", err), err
	}

	newStatus := "active"
	if req.Action == "disable" {
		newStatus = "disabled"
	}

	result := db.Get().WithContext(queryCtx).
		Model(&models.APIToken{}).
		Where("id = ?", req.TokenId).
		Update("status", newStatus)

	if result.Error != nil {
		global.Logger.Errorf("failed to update token status for token %d: %v", req.TokenId, result.Error)
		return ErrorReply(http.StatusInternalServerError, "failed to update token status", result.Error), result.Error
	}

	response := map[string]interface{}{
		"message":  fmt.Sprintf("Token %sd successfully", req.Action),
		"token_id": req.TokenId,
		"status":   newStatus,
	}

	return MarshalAndReply(response, "marshal toggle token status response error")
}

// ExportAccounts exports account list
func (s *AdminService) ExportAccounts(ctx context.Context, req *pb.ExportAccountsRequest) (*pb.Reply, error) {
	if req == nil {
		err := errors.New("nil request")
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", err), err
	}

	listReq := &pb.ListAccountsRequest{
		Pagination: &pb.PaginationRequest{
			Page:     1,
			PageSize: 10000,
		},
		Search:      req.Search,
		Status:      req.Status,
		UsageFilter: req.UsageFilter,
	}

	reply, err := s.ListAccounts(ctx, listReq)
	if err != nil {
		return reply, err
	}

	response := map[string]interface{}{
		"message":      "Export completed",
		"format":       req.Format,
		"total_count":  0,
		"download_url": "",
	}

	return MarshalAndReply(response, "marshal export accounts response error")
}
