// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
)

var (
	domainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$`)
)

// ListEmailBlacklist lists email blacklist domains
func (s *AdminService) ListEmailBlacklist(ctx context.Context, req *pb.ListEmailBlacklistRequest) (*pb.Reply, error) {
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
		return nil, status.Error(codes.Internal, "failed to query email blacklist")
	}

	var blacklist []models.EmailBlacklist
	offset := (pagination.Page - 1) * pagination.PageSize
	err = query.Offset(int(offset)).Limit(int(pagination.PageSize)).Order("created_at DESC").Find(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to list email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to query email blacklist")
	}

	infos := make([]*pb.EmailBlacklistInfo, 0, len(blacklist))
	for _, b := range blacklist {
		infos = append(infos, &pb.EmailBlacklistInfo{
			Id:         uint64(b.ID),
			Domain:     b.Domain,
			Status:     b.Status,
			BlockCount: b.BlockCount,
			Note:       b.Note,
			CreatedAt:  b.CreatedAt.Format(time.RFC3339),
			UpdatedAt:  b.UpdatedAt.Format(time.RFC3339),
		})
	}

	totalPages := int32((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
	response := map[string]interface{}{
		"domains": infos,
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

// AddEmailDomain adds an email domain to blacklist
func (s *AdminService) AddEmailDomain(ctx context.Context, req *pb.AddEmailDomainRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if req.Domain == "" {
		return nil, status.Error(codes.InvalidArgument, "domain is required")
	}

	// Normalize domain (lowercase, trim)
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if strings.HasPrefix(domain, "@") {
		domain = domain[1:]
	}

	// Validate domain format
	if !domainRegex.MatchString(domain) {
		return nil, status.Error(codes.InvalidArgument, "invalid domain format")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	// Check if domain already exists
	var existing models.EmailBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("domain = ?", domain).
		First(&existing).Error
	if err == nil {
		return nil, status.Error(codes.AlreadyExists, "domain already exists in blacklist")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Errorf("failed to check email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to check email blacklist")
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
		return nil, status.Error(codes.Internal, "failed to add email domain to blacklist")
	}

	info := &pb.EmailBlacklistInfo{
		Id:         uint64(blacklist.ID),
		Domain:     blacklist.Domain,
		Status:     blacklist.Status,
		BlockCount: blacklist.BlockCount,
		Note:       blacklist.Note,
		CreatedAt:  blacklist.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  blacklist.UpdatedAt.Format(time.RFC3339),
	}

	infoMap := make(map[string]interface{})
	infoBytes, _ := sonic.Marshal(info)
	sonic.Unmarshal(infoBytes, &infoMap)

	data, err := structpb.NewStruct(infoMap)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: "Email domain added to blacklist successfully",
		Data:    data,
	}, nil
}

// UpdateEmailDomain updates an email domain
func (s *AdminService) UpdateEmailDomain(ctx context.Context, req *pb.UpdateEmailDomainRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	var blacklist models.EmailBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("id = ?", req.Id).
		First(&blacklist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "email domain not found")
		}
		global.Logger.Errorf("failed to get email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to query email blacklist")
	}

	// Update fields
	updates := make(map[string]interface{})
	if req.Status != "" {
		if req.Status != "active" && req.Status != "disabled" {
			return nil, status.Error(codes.InvalidArgument, "status must be 'active' or 'disabled'")
		}
		updates["status"] = req.Status
	}
	if req.Note != "" {
		updates["note"] = req.Note
	}

	if len(updates) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no fields to update")
	}

	err = db.Get().WithContext(queryCtx).Model(&blacklist).Updates(updates).Error
	if err != nil {
		global.Logger.Errorf("failed to update email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to update email blacklist")
	}

	// Reload
	err = db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to reload email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to reload email blacklist")
	}

	info := &pb.EmailBlacklistInfo{
		Id:         uint64(blacklist.ID),
		Domain:     blacklist.Domain,
		Status:     blacklist.Status,
		BlockCount: blacklist.BlockCount,
		Note:       blacklist.Note,
		CreatedAt:  blacklist.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  blacklist.UpdatedAt.Format(time.RFC3339),
	}

	infoMap := make(map[string]interface{})
	infoBytes, _ := sonic.Marshal(info)
	sonic.Unmarshal(infoBytes, &infoMap)

	data, err := structpb.NewStruct(infoMap)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: "Email domain updated successfully",
		Data:    data,
	}, nil
}

// DeleteEmailDomain deletes an email domain from blacklist
func (s *AdminService) DeleteEmailDomain(ctx context.Context, req *pb.DeleteEmailDomainRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	var blacklist models.EmailBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("id = ?", req.Id).
		First(&blacklist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "email domain not found")
		}
		global.Logger.Errorf("failed to get email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to query email blacklist")
	}

	err = db.Get().WithContext(queryCtx).Delete(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to delete email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to delete email blacklist")
	}

	return &pb.Reply{
		Code:    0,
		Message: "Email domain deleted successfully",
	}, nil
}

// BatchImportEmailDomains batch imports email domains
func (s *AdminService) BatchImportEmailDomains(ctx context.Context, req *pb.BatchImportEmailDomainsRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if len(req.Domains) == 0 {
		return nil, status.Error(codes.InvalidArgument, "domains is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		if strings.HasPrefix(domain, "@") {
			domain = domain[1:]
		}

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
		return nil, status.Error(codes.InvalidArgument, "no valid domains to import")
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
	added := 0
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
			return nil, status.Error(codes.Internal, "failed to import email domains")
		}
		added = len(newDomains)
	}

	result := &pb.BatchImportResult{
		Total:          int32(len(req.Domains)),
		Valid:          int32(len(validDomains)),
		Invalid:        int32(len(invalidDomains)),
		Duplicate:      int32(duplicate),
		InvalidDomains: invalidDomains,
	}

	resultMap := make(map[string]interface{})
	resultBytes, _ := sonic.Marshal(result)
	sonic.Unmarshal(resultBytes, &resultMap)

	data, err := structpb.NewStruct(resultMap)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: fmt.Sprintf("Batch import completed: %d added, %d duplicate, %d invalid", added, duplicate, len(invalidDomains)),
		Data:    data,
	}, nil
}

// ToggleEmailDomainStatus toggles email domain status (active/disabled)
func (s *AdminService) ToggleEmailDomainStatus(ctx context.Context, req *pb.ToggleEmailDomainStatusRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	var blacklist models.EmailBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("id = ?", req.Id).
		First(&blacklist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "email domain not found")
		}
		global.Logger.Errorf("failed to get email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to query email blacklist")
	}

	// Toggle status
	newStatus := "active"
	if blacklist.Status == "active" {
		newStatus = "disabled"
	}

	err = db.Get().WithContext(queryCtx).Model(&blacklist).Update("status", newStatus).Error
	if err != nil {
		global.Logger.Errorf("failed to toggle email blacklist status: %v", err)
		return nil, status.Error(codes.Internal, "failed to toggle email blacklist status")
	}

	// Reload
	err = db.Get().WithContext(queryCtx).Where("id = ?", req.Id).First(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to reload email blacklist: %v", err)
		return nil, status.Error(codes.Internal, "failed to reload email blacklist")
	}

	info := &pb.EmailBlacklistInfo{
		Id:         uint64(blacklist.ID),
		Domain:     blacklist.Domain,
		Status:     blacklist.Status,
		BlockCount: blacklist.BlockCount,
		Note:       blacklist.Note,
		CreatedAt:  blacklist.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  blacklist.UpdatedAt.Format(time.RFC3339),
	}

	infoMap := make(map[string]interface{})
	infoBytes, _ := sonic.Marshal(info)
	sonic.Unmarshal(infoBytes, &infoMap)

	data, err := structpb.NewStruct(infoMap)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: fmt.Sprintf("Email domain status toggled to %s", newStatus),
		Data:    data,
	}, nil
}

// GetEmailBlacklistStats gets email blacklist statistics
func (s *AdminService) GetEmailBlacklistStats(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	// Get total count
	var totalCount int64
	db.Get().WithContext(queryCtx).Model(&models.EmailBlacklist{}).Count(&totalCount)

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

	stats := &pb.EmailBlacklistStatsInfo{
		TotalCount:    totalCount,
		ActiveCount:   activeCount,
		DisabledCount: disabledCount,
	}

	statsMap := make(map[string]interface{})
	statsBytes, _ := sonic.Marshal(stats)
	sonic.Unmarshal(statsBytes, &statsMap)

	data, err := structpb.NewStruct(statsMap)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: "success",
		Data:    data,
	}, nil
}

// PreviewImportEmailDomains previews batch import email domains
func (s *AdminService) PreviewImportEmailDomains(ctx context.Context, req *pb.PreviewImportEmailDomainsRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if len(req.Domains) == 0 {
		return nil, status.Error(codes.InvalidArgument, "domains is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		if strings.HasPrefix(domain, "@") {
			domain = domain[1:]
		}

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
		db.Get().WithContext(queryCtx).
			Where("domain IN ?", validDomains).
			Find(&existing)
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

	result := &pb.BatchImportResult{
		Total:            int32(len(req.Domains)),
		Valid:            int32(len(validDomains)),
		Invalid:          int32(len(invalidDomains)),
		Duplicate:        int32(len(duplicateDomains)),
		InvalidDomains:   invalidDomains,
		DuplicateDomains: duplicateDomains,
	}

	resultMap := make(map[string]interface{})
	resultBytes, _ := sonic.Marshal(result)
	sonic.Unmarshal(resultBytes, &resultMap)

	data, err := structpb.NewStruct(resultMap)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: "Preview completed",
		Data:    data,
	}, nil
}

// BatchDeleteEmailDomains batch deletes email domains
func (s *AdminService) BatchDeleteEmailDomains(ctx context.Context, req *pb.BatchDeleteEmailDomainsRequest) (*pb.Reply, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if len(req.Ids) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ids is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer queryCancel()

	result := db.Get().WithContext(queryCtx).
		Where("id IN ?", req.Ids).
		Delete(&models.EmailBlacklist{})

	if result.Error != nil {
		global.Logger.Errorf("failed to batch delete email domains: %v", result.Error)
		return nil, status.Error(codes.Internal, "failed to delete email domains")
	}

	response := map[string]interface{}{
		"deleted": int64(result.RowsAffected),
	}

	data, err := structpb.NewStruct(response)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal response")
	}

	return &pb.Reply{
		Code:    0,
		Message: fmt.Sprintf("Deleted %d email domains", result.RowsAffected),
		Data:    data,
	}, nil
}

