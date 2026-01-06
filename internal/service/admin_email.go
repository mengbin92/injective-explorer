// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

var (
	domainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$`)
)

// ListEmailBlacklist lists email blacklist domains
func (s *AdminService) ListEmailBlacklist(ctx context.Context, req *pb.ListEmailBlacklistRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
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

	query := db.Get().WithContext(queryCtx).Model(&models.EmailBlacklist{})

	if req.Domain != "" {
		query = query.Where("domain LIKE ?", "%"+req.Domain+"%")
	}

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	var total int64
	err := query.Count(&total).Error
	if err != nil {
		global.Logger.Errorf("failed to count email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query email blacklist", err), err
	}

	var blacklist []models.EmailBlacklist
	offset := (pagination.Page - 1) * pagination.PageSize
	err = query.Offset(int(offset)).Limit(int(pagination.PageSize)).Order("created_at DESC").Find(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to list email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query email blacklist", err), err
	}

	// Convert to DTOs
	infos := models.ToEmailBlacklistDTOs(blacklist)

	// Build paginated response using buildPaginatedResponse
	response := buildPaginatedResponse(infos, pagination.Page, pagination.PageSize, total, true, false, 0, "", len(blacklist))

	return MarshalAndReply(response, "failed to marshal response")
}

// AddEmailDomain adds an email domain to blacklist
func (s *AdminService) AddEmailDomain(ctx context.Context, req *pb.AddEmailDomainRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if req.Domain == "" {
		return ErrorReply(http.StatusBadRequest, "domain is required", nil), nil
	}

	// Normalize domain (lowercase, trim)
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	domain = strings.TrimPrefix(domain, "@")

	// Validate domain format
	if !domainRegex.MatchString(domain) {
		return ErrorReply(http.StatusBadRequest, "invalid domain format", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Check if domain already exists
	var existing models.EmailBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("domain = ?", domain).
		First(&existing).Error
	if err == nil {
		return ErrorReply(http.StatusConflict, "domain already exists in blacklist", nil), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Errorf("failed to check email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to check email blacklist", err), err
	}

	// Set default status
	domainStatus := req.Status
	if domainStatus == "" {
		domainStatus = "active"
	}
	if domainStatus != "active" && domainStatus != "disabled" {
		domainStatus = "active"
	}

	// Create blacklist entry
	blacklist := &models.EmailBlacklist{
		Domain:     domain,
		Status:     domainStatus,
		BlockCount: 0,
		Note:       req.Note,
	}

	err = db.Get().WithContext(queryCtx).Create(blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to add email domain to blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to add email domain to blacklist", err), err
	}

	// Convert to DTO
	info := models.ToEmailBlacklistDTO(blacklist)

	return MarshalAndReply(info, "failed to marshal response")
}

// UpdateEmailDomain updates an email domain
func (s *AdminService) UpdateEmailDomain(ctx context.Context, req *pb.UpdateEmailDomainRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if req.Id == 0 {
		return ErrorReply(http.StatusBadRequest, "id is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var blacklist models.EmailBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("id = ?", req.Id).
		First(&blacklist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "email domain not found", nil), nil
		}
		global.Logger.Errorf("failed to get email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query email blacklist", err), err
	}

	// Update fields
	updates := make(map[string]interface{})
	if req.Status != "" {
		if req.Status != "active" && req.Status != "disabled" {
			return ErrorReply(http.StatusBadRequest, "status must be 'active' or 'disabled'", nil), nil
		}
		updates["status"] = req.Status
	}
	if req.Note != "" {
		updates["note"] = req.Note
	}

	if len(updates) == 0 {
		return ErrorReply(http.StatusBadRequest, "no fields to update", nil), nil
	}

	err = db.Get().WithContext(queryCtx).Model(&blacklist).Updates(updates).Error
	if err != nil {
		global.Logger.Errorf("failed to update email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to update email blacklist", err), err
	}

	// Reload
	err = db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to reload email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to reload email blacklist", err), err
	}

	// Convert to DTO
	info := models.ToEmailBlacklistDTO(&blacklist)

	return MarshalAndReply(info, "failed to marshal response")
}

// DeleteEmailDomain deletes an email domain from blacklist
func (s *AdminService) DeleteEmailDomain(ctx context.Context, req *pb.DeleteEmailDomainRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if req.Id == 0 {
		return ErrorReply(http.StatusBadRequest, "id is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var blacklist models.EmailBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("id = ?", req.Id).
		First(&blacklist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "email domain not found", nil), nil
		}
		global.Logger.Errorf("failed to get email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query email blacklist", err), err
	}

	err = db.Get().WithContext(queryCtx).Delete(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to delete email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to delete email blacklist", err), err
	}

	// Return simple success response
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "Email domain deleted successfully",
		Data:    nil,
	}, nil
}

// BatchImportEmailDomains batch imports email domains
func (s *AdminService) BatchImportEmailDomains(ctx context.Context, req *pb.BatchImportEmailDomainsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if len(req.Domains) == 0 {
		return ErrorReply(http.StatusBadRequest, "domains is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Normalize and validate domains
	validDomains := make([]string, 0)
	invalidDomains := make([]string, 0)
	domainSet := make(map[string]bool)

	for _, domain := range req.Domains {
		// Normalize domain
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			continue
		}
		domain = strings.TrimPrefix(domain, "@")

		// Check for duplicates in this batch
		if domainSet[domain] {
			continue
		}
		domainSet[domain] = true

		// Validate format
		if !domainRegex.MatchString(domain) {
			invalidDomains = append(invalidDomains, domain)
			continue
		}

		validDomains = append(validDomains, domain)
	}

	if len(validDomains) == 0 {
		return ErrorReply(http.StatusBadRequest, "no valid domains to import", nil), nil
	}

	// Check which domains already exist in blacklist
	var existing []models.EmailBlacklist
	db.Get().WithContext(queryCtx).
		Where("domain IN ?", validDomains).
		Find(&existing)

	existingMap := make(map[string]bool)
	for _, e := range existing {
		existingMap[e.Domain] = true
	}

	// Import new domains
	duplicate := 0
	newDomains := make([]models.EmailBlacklist, 0)

	for _, domain := range validDomains {
		if existingMap[domain] {
			duplicate++
			continue
		}

		newDomains = append(newDomains, models.EmailBlacklist{
			Domain:     domain,
			Status:     "active",
			BlockCount: 0,
		})
	}

	if len(newDomains) > 0 {
		// Batch insert
		err := db.Get().WithContext(queryCtx).CreateInBatches(newDomains, 100).Error
		if err != nil {
			global.Logger.Errorf("failed to batch import email domains: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to import email domains", err), err
		}
	}

	result := &pb.BatchImportEmailDomainsDTO{
		Total:          int32(len(req.Domains)),
		Valid:          int32(len(validDomains)),
		Invalid:        int32(len(invalidDomains)),
		Duplicate:      int32(duplicate),
		InvalidDomains: invalidDomains,
	}

	return MarshalAndReply(result, "failed to marshal response")
}

// ToggleEmailDomainStatus toggles email domain status (active/disabled)
func (s *AdminService) ToggleEmailDomainStatus(ctx context.Context, req *pb.ToggleEmailDomainStatusRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if req.Id == 0 {
		return ErrorReply(http.StatusBadRequest, "id is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var blacklist models.EmailBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("id = ?", req.Id).
		First(&blacklist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "email domain not found", nil), nil
		}
		global.Logger.Errorf("failed to get email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query email blacklist", err), err
	}

	// Toggle status
	newStatus := "active"
	if blacklist.Status == "active" {
		newStatus = "disabled"
	}

	err = db.Get().WithContext(queryCtx).Model(&blacklist).Update("status", newStatus).Error
	if err != nil {
		global.Logger.Errorf("failed to toggle email blacklist status: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to toggle email blacklist status", err), err
	}

	// Reload
	err = db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to reload email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to reload email blacklist", err), err
	}

	// Convert to DTO
	info := models.ToEmailBlacklistDTO(&blacklist)

	return MarshalAndReply(info, "failed to marshal response")
}

// GetEmailBlacklistStats gets email blacklist statistics
func (s *AdminService) GetEmailBlacklistStats(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Get total count
	var totalCount int64
	err := db.Get().WithContext(queryCtx).Model(&models.EmailBlacklist{}).Count(&totalCount).Error
	if err != nil {
		global.Logger.Errorf("failed to count email blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query email blacklist stats", err), err
	}

	// Get active count
	var activeCount int64
	db.Get().WithContext(queryCtx).
		Model(&models.EmailBlacklist{}).
		Where("status = ?", "active").
		Count(&activeCount)

	// Get disabled count
	var disabledCount int64
	db.Get().WithContext(queryCtx).
		Model(&models.EmailBlacklist{}).
		Where("status = ?", "disabled").
		Count(&disabledCount)

	stats := &pb.EmailBlacklistStatsDTO{
		TotalCount:    totalCount,
		ActiveCount:   activeCount,
		DisabledCount: disabledCount,
	}

	return MarshalAndReply(stats, "failed to marshal response")
}

// PreviewImportEmailDomains previews batch import email domains
func (s *AdminService) PreviewImportEmailDomains(ctx context.Context, req *pb.PreviewImportEmailDomainsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if len(req.Domains) == 0 {
		return ErrorReply(http.StatusBadRequest, "domains is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Normalize and validate domains
	validDomains := make([]string, 0)
	invalidDomains := make([]string, 0)
	domainSet := make(map[string]bool)

	for _, domain := range req.Domains {
		// Normalize domain
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			continue
		}
		domain = strings.TrimPrefix(domain, "@")

		// Check for duplicates in this batch
		if domainSet[domain] {
			continue
		}
		domainSet[domain] = true

		// Validate format
		if !domainRegex.MatchString(domain) {
			invalidDomains = append(invalidDomains, domain)
			continue
		}

		validDomains = append(validDomains, domain)
	}

	// Check which domains already exist in blacklist
	var existing []models.EmailBlacklist
	if len(validDomains) > 0 {
		err := db.Get().WithContext(queryCtx).
			Where("domain IN ?", validDomains).
			Find(&existing).Error
		if err != nil {
			global.Logger.Errorf("failed to check existing domains: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to check existing domains", err), err
		}
	}

	existingMap := make(map[string]bool)
	for _, e := range existing {
		existingMap[e.Domain] = true
	}

	duplicateDomains := make([]string, 0)
	newDomains := make([]string, 0)
	for _, domain := range validDomains {
		if existingMap[domain] {
			duplicateDomains = append(duplicateDomains, domain)
		} else {
			newDomains = append(newDomains, domain)
		}
	}

	result := &pb.BatchImportEmailDomainsDTO{
		Total:            int32(len(req.Domains)),
		Valid:            int32(len(validDomains)),
		Invalid:          int32(len(invalidDomains)),
		Duplicate:        int32(len(duplicateDomains)),
		InvalidDomains:   invalidDomains,
		DuplicateDomains: duplicateDomains,
	}

	return MarshalAndReply(result, "failed to marshal response")
}

// BatchDeleteEmailDomains batch deletes email domains
func (s *AdminService) BatchDeleteEmailDomains(ctx context.Context, req *pb.BatchDeleteEmailDomainsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if len(req.Ids) == 0 {
		return ErrorReply(http.StatusBadRequest, "ids is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	result := db.Get().WithContext(queryCtx).
		Where("id IN ?", req.Ids).
		Delete(&models.EmailBlacklist{})

	if result.Error != nil {
		global.Logger.Errorf("failed to batch delete email domains: %v", result.Error)
		return ErrorReply(http.StatusInternalServerError, "failed to delete email domains", result.Error), result.Error
	}

	response := &pb.BatchDeleteEmailDomainsDTO{
		Deleted: result.RowsAffected,
	}

	return MarshalAndReply(response, "failed to marshal response")
}
