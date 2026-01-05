// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"encoding/csv"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

// ListIPRegistrationRecords lists IP registration records
func (s *AdminService) ListIPRegistrationRecords(ctx context.Context, req *pb.ListIPRegistrationRecordsRequest) (*pb.Reply, error) {
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

	// Build query for IP registration records
	// Group by IP address and aggregate registration data
	query := db.Get().WithContext(queryCtx).
		Table("ip_registration_records").
		Select("ip_address, ip_location, COUNT(*) as register_count, MAX(created_at) as last_register_time, MIN(created_at) as first_register_time").
		Group("ip_address, ip_location")

	// Apply filters
	if req.IpAddress != "" {
		query = query.Where("ip_address LIKE ?", "%"+req.IpAddress+"%")
	}

	// Time range filter
	if req.TimeRange != "" {
		now := time.Now()
		switch req.TimeRange {
		case "today":
			query = query.Where("created_at >= ?", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
		case "7days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -7))
		case "30days":
			query = query.Where("created_at >= ?", now.AddDate(0, 0, -30))
		}
	}

	// Min register count filter
	if req.MinRegisterCount > 0 {
		query = query.Having("COUNT(*) >= ?", req.MinRegisterCount)
	}

	// Get total count
	var total int64
	countQuery := db.Get().WithContext(queryCtx).
		Table("(?) as t", query).
		Count(&total)

	if err := countQuery.Error; err != nil {
		global.Logger.Errorf("failed to count IP records: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query IP records", err), err
	}

	// Apply pagination and ordering
	offset := (pagination.Page - 1) * pagination.PageSize
	var results []struct {
		IPAddress         string    `gorm:"column:ip_address"`
		IPLocation        string    `gorm:"column:ip_location"`
		RegisterCount     int64     `gorm:"column:register_count"`
		LastRegisterTime  time.Time `gorm:"column:last_register_time"`
		FirstRegisterTime time.Time `gorm:"column:first_register_time"`
	}

	err := query.Order("register_count DESC").Offset(int(offset)).Limit(int(pagination.PageSize)).Scan(&results).Error
	if err != nil {
		global.Logger.Errorf("failed to list IP records: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query IP records", err), err
	}

	// Check IP status (in blacklist or not)
	ipAddresses := make([]string, 0, len(results))
	for _, r := range results {
		ipAddresses = append(ipAddresses, r.IPAddress)
	}

	var blacklistedIPs []models.IPBlacklist
	if len(ipAddresses) > 0 {
		err := db.Get().WithContext(queryCtx).
			Where("ip_address IN ?", ipAddresses).
			Find(&blacklistedIPs).Error
		if err != nil {
			global.Logger.Warnf("failed to check blacklisted IPs: %v", err)
			// Continue with empty blacklist map if query fails
		}
	}

	blacklistMap := make(map[string]bool)
	for _, b := range blacklistedIPs {
		blacklistMap[b.IPAddress] = true
	}

	// Convert to response format
	records := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		status := "normal"
		if blacklistMap[r.IPAddress] {
			status = "blocked"
		}

		records = append(records, map[string]interface{}{
			"ip_address":         r.IPAddress,
			"ip_location":        r.IPLocation,
			"register_count":     r.RegisterCount,
			"last_register_time": r.LastRegisterTime.Format(time.RFC3339),
			"status":             status,
		})
	}

	totalPages := int32((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
	response := map[string]interface{}{
		"records": records,
		"pagination": map[string]interface{}{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": totalPages,
			"has_prev":    pagination.Page > 1,
			"has_next":    pagination.Page < totalPages,
		},
	}

	return MarshalAndReply(response, "failed to marshal response")
}

// GetIPDetail gets IP detail information
func (s *AdminService) GetIPDetail(ctx context.Context, req *pb.GetIPDetailRequest) (*pb.Reply, error) {
	if req == nil || req.IpAddress == "" {
		return ErrorReply(http.StatusBadRequest, "ip_address is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Get IP registration records
	var records []models.IPRegistrationRecord
	err := db.Get().WithContext(queryCtx).
		Where("ip_address = ?", req.IpAddress).
		Order("created_at DESC").
		Find(&records).Error
	if err != nil {
		global.Logger.Errorf("failed to get IP records: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query IP records", err), err
	}

	if len(records) == 0 {
		return ErrorReply(http.StatusNotFound, "IP address not found", nil), nil
	}

	// Get IP location (from first record)
	ipLocation := records[0].IPLocation
	if ipLocation == "" {
		ipLocation = "未知地区"
	}

	// Check if IP is in blacklist
	var blacklist models.IPBlacklist
	isBlacklisted := false
	err = db.Get().WithContext(queryCtx).
		Where("ip_address = ?", req.IpAddress).
		First(&blacklist).Error
	if err == nil {
		isBlacklisted = true
	}

	ipStatus := "normal"
	if isBlacklisted {
		ipStatus = "blocked"
	}

	// Get first and last registration time
	firstTime := records[len(records)-1].CreatedAt

	// Convert accounts to []map[string]interface{}
	accountsList := make([]map[string]interface{}, 0, len(records))
	for _, r := range records {
		accountsList = append(accountsList, map[string]interface{}{
			"user_id":       r.UserID,
			"email":         r.Email,
			"username":      "", // Username not available in Account model
			"register_time": r.CreatedAt.Format(time.RFC3339),
		})
	}

	detailMap := map[string]interface{}{
		"ip_address":          req.IpAddress,
		"ip_location":         ipLocation,
		"status":              ipStatus,
		"first_register_time": firstTime.Format(time.RFC3339),
		"total_registers":     int64(len(records)),
		"accounts":            accountsList,
	}

	return MarshalAndReply(detailMap, "failed to marshal response")
}

// ListIPBlacklist lists IP blacklist
func (s *AdminService) ListIPBlacklist(ctx context.Context, req *pb.ListIPBlacklistRequest) (*pb.Reply, error) {
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

	query := db.Get().WithContext(queryCtx).Model(&models.IPBlacklist{})

	if req.IpAddress != "" {
		query = query.Where("ip_address LIKE ?", "%"+req.IpAddress+"%")
	}

	var total int64
	err := query.Count(&total).Error
	if err != nil {
		global.Logger.Errorf("failed to count IP blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query IP blacklist", err), err
	}

	var blacklist []models.IPBlacklist
	offset := (pagination.Page - 1) * pagination.PageSize
	err = query.Offset(int(offset)).Limit(int(pagination.PageSize)).Order("created_at DESC").Find(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to list IP blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query IP blacklist", err), err
	}

	// Convert blacklist to []map[string]interface{}
	infos := make([]map[string]interface{}, 0, len(blacklist))
	for _, b := range blacklist {
		infos = append(infos, map[string]interface{}{
			"id":            uint64(b.ID),
			"ip_address":    b.IPAddress,
			"ip_location":   b.IPLocation,
			"reason":        b.Reason,
			"reason_text":   b.ReasonText,
			"block_count":   b.BlockCount,
			"operator_name": b.OperatorName,
			"block_type":    b.BlockType,
			"created_at":    b.CreatedAt.Format(time.RFC3339),
		})
	}

	totalPages := int32((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
	response := map[string]interface{}{
		"blacklist": infos,
		"pagination": map[string]interface{}{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": totalPages,
			"has_prev":    pagination.Page > 1,
			"has_next":    pagination.Page < totalPages,
		},
	}

	return MarshalAndReply(response, "failed to marshal response")
}

// AddIPToBlacklist adds an IP to blacklist
func (s *AdminService) AddIPToBlacklist(ctx context.Context, req *pb.AddIPToBlacklistRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if req.IpAddress == "" {
		return ErrorReply(http.StatusBadRequest, "ip_address is required", nil), nil
	}

	// Validate IP address format
	ip := net.ParseIP(req.IpAddress)
	if ip == nil {
		return ErrorReply(http.StatusBadRequest, "invalid IP address format", nil), nil
	}

	if req.Reason == "" {
		return ErrorReply(http.StatusBadRequest, "reason is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	// Check if IP already in blacklist
	var existing models.IPBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("ip_address = ?", req.IpAddress).
		First(&existing).Error
	if err == nil {
		return ErrorReply(http.StatusConflict, "IP already in blacklist", nil), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Errorf("failed to check IP blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to check IP blacklist", err), err
	}

	// Get IP location (try to get from registration records)
	ipLocation := ""
	var record models.IPRegistrationRecord
	if err := db.Get().WithContext(queryCtx).
		Where("ip_address = ?", req.IpAddress).
		First(&record).Error; err == nil {
		ipLocation = record.IPLocation
	}

	// Create blacklist entry
	blacklist := &models.IPBlacklist{
		IPAddress:    req.IpAddress,
		IPLocation:   ipLocation,
		Reason:       req.Reason,
		ReasonText:   req.ReasonText,
		BlockCount:   0,
		OperatorID:   req.OperatorId,
		OperatorName: req.OperatorName,
		BlockType:    req.BlockType,
	}

	if blacklist.BlockType == "" {
		blacklist.BlockType = "all"
	}

	err = db.Get().WithContext(queryCtx).Create(blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to add IP to blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to add IP to blacklist", err), err
	}

	infoMap := map[string]interface{}{
		"id":            uint64(blacklist.ID),
		"ip_address":    blacklist.IPAddress,
		"ip_location":   blacklist.IPLocation,
		"reason":        blacklist.Reason,
		"reason_text":   blacklist.ReasonText,
		"block_count":   blacklist.BlockCount,
		"operator_name": blacklist.OperatorName,
		"block_type":    blacklist.BlockType,
		"created_at":    blacklist.CreatedAt.Format(time.RFC3339),
	}

	return MarshalAndReply(infoMap, "failed to marshal response")
}

// RemoveIPFromBlacklist removes an IP from blacklist
func (s *AdminService) RemoveIPFromBlacklist(ctx context.Context, req *pb.RemoveIPFromBlacklistRequest) (*pb.Reply, error) {
	if req == nil || req.IpAddress == "" {
		return ErrorReply(http.StatusBadRequest, "ip_address is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var blacklist models.IPBlacklist
	err := db.Get().WithContext(queryCtx).
		Where("ip_address = ?", req.IpAddress).
		First(&blacklist).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "IP not found in blacklist", nil), nil
		}
		global.Logger.Errorf("failed to get IP blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query IP blacklist", err), err
	}

	err = db.Get().WithContext(queryCtx).Delete(&blacklist).Error
	if err != nil {
		global.Logger.Errorf("failed to remove IP from blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to remove IP from blacklist", err), err
	}

	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "IP removed from blacklist successfully",
		Data:    nil,
	}, nil
}

// BatchAddIPToBlacklist batch adds IPs to blacklist
func (s *AdminService) BatchAddIPToBlacklist(ctx context.Context, req *pb.BatchAddIPToBlacklistRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if len(req.IpAddresses) == 0 {
		return ErrorReply(http.StatusBadRequest, "ip_addresses is required", nil), nil
	}

	if req.Reason == "" {
		return ErrorReply(http.StatusBadRequest, "reason is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Validate all IPs
	validIPs := make([]string, 0)
	invalidIPs := make([]string, 0)
	for _, ipStr := range req.IpAddresses {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			invalidIPs = append(invalidIPs, ipStr)
		} else {
			validIPs = append(validIPs, ipStr)
		}
	}

	if len(validIPs) == 0 {
		return ErrorReply(http.StatusBadRequest, "no valid IP addresses", nil), nil
	}

	// Check which IPs are already in blacklist
	var existing []models.IPBlacklist
	db.Get().WithContext(queryCtx).
		Where("ip_address IN ?", validIPs).
		Find(&existing)

	existingMap := make(map[string]bool)
	for _, e := range existing {
		existingMap[e.IPAddress] = true
	}

	// Get IP locations
	ipLocationMap := make(map[string]string)
	var records []models.IPRegistrationRecord
	db.Get().WithContext(queryCtx).
		Where("ip_address IN ?", validIPs).
		Find(&records)
	for _, r := range records {
		if _, ok := ipLocationMap[r.IPAddress]; !ok {
			ipLocationMap[r.IPAddress] = r.IPLocation
		}
	}

	// Add new IPs to blacklist
	blockType := req.BlockType
	if blockType == "" {
		blockType = "all"
	}

	added := 0
	skipped := 0
	for _, ipStr := range validIPs {
		if existingMap[ipStr] {
			skipped++
			continue
		}

		blacklist := &models.IPBlacklist{
			IPAddress:    ipStr,
			IPLocation:   ipLocationMap[ipStr],
			Reason:       req.Reason,
			ReasonText:   req.ReasonText,
			BlockCount:   0,
			OperatorID:   req.OperatorId,
			OperatorName: req.OperatorName,
			BlockType:    blockType,
		}

		if err := db.Get().WithContext(queryCtx).Create(blacklist).Error; err != nil {
			global.Logger.Errorf("failed to add IP %s to blacklist: %v", ipStr, err)
			continue
		}
		added++
	}

	response := map[string]interface{}{
		"added":       added,
		"skipped":     skipped,
		"invalid":     len(invalidIPs),
		"invalid_ips": invalidIPs,
	}

	return MarshalAndReply(response, "failed to marshal response")
}

// BatchRemoveIPFromBlacklist batch removes IPs from blacklist
func (s *AdminService) BatchRemoveIPFromBlacklist(ctx context.Context, req *pb.BatchRemoveIPFromBlacklistRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if len(req.IpAddresses) == 0 {
		return ErrorReply(http.StatusBadRequest, "ip_addresses is required", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	result := db.Get().WithContext(queryCtx).
		Where("ip_address IN ?", req.IpAddresses).
		Delete(&models.IPBlacklist{})

	if result.Error != nil {
		global.Logger.Errorf("failed to batch remove IPs from blacklist: %v", result.Error)
		return ErrorReply(http.StatusInternalServerError, "failed to remove IPs from blacklist", result.Error), result.Error
	}

	response := map[string]interface{}{
		"removed": int64(result.RowsAffected),
	}

	return MarshalAndReply(response, "failed to marshal response")
}

// GetIPRequestMonitor gets IP request monitoring data
func (s *AdminService) GetIPRequestMonitor(ctx context.Context, req *pb.GetIPRequestMonitorRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Determine time range
	now := time.Now()
	var startTime time.Time
	switch req.TimeRange {
	case "1h":
		startTime = now.Add(-1 * time.Hour)
	case "6h":
		startTime = now.Add(-6 * time.Hour)
	case "24h", "":
		startTime = now.Add(-24 * time.Hour)
	case "7d":
		startTime = now.AddDate(0, 0, -7)
	default:
		startTime = now.Add(-24 * time.Hour)
	}

	// Build query
	query := db.Get().WithContext(queryCtx).
		Model(&models.APIRequestLog{}).
		Where("created_at >= ?", startTime)

	if req.IpAddress != "" {
		query = query.Where("ip_address = ?", req.IpAddress)
	}

	// Get total requests and unique IPs
	var totalRequests int64
	var uniqueIPs int64
	query.Count(&totalRequests)
	query.Select("COUNT(DISTINCT ip_address)").Scan(&uniqueIPs)

	// Get rate limited count
	var rateLimitedCount int64
	db.Get().WithContext(queryCtx).
		Model(&models.APIRequestLog{}).
		Where("created_at >= ? AND status_code = 429", startTime).
		Count(&rateLimitedCount)

	// Get IP request top 10
	var topIPs []struct {
		IPAddress    string `gorm:"column:ip_address"`
		RequestCount int64  `gorm:"column:request_count"`
	}
	db.Get().WithContext(queryCtx).
		Model(&models.APIRequestLog{}).
		Select("ip_address, COUNT(*) as request_count").
		Where("created_at >= ?", startTime).
		Group("ip_address").
		Order("request_count DESC").
		Limit(10).
		Scan(&topIPs)

	// Get IP rate limit config for comparison (default values)
	ipPerHour := int32(5000)
	// TODO: Get from config when available
	// config := global.GetConfig()
	// if config.APIManagement != nil && config.APIManagement.RateLimit != nil {
	// 	ipPerHour = config.APIManagement.RateLimit.IpPerHour
	// }

	// Build top IPs list with status
	topIPInfos := make([]map[string]interface{}, 0, len(topIPs))
	abnormalIPs := make([]map[string]interface{}, 0)
	for i, topIP := range topIPs {
		ipStatus := "normal"
		abnormalType := ""

		// Check if abnormal (exceeds rate limit)
		requestsPerHour := float64(topIP.RequestCount) / (float64(time.Since(startTime)) / float64(time.Hour))
		if requestsPerHour > float64(ipPerHour)*2 {
			ipStatus = "abnormal"
			abnormalType = "请求量异常"
			abnormalIPs = append(abnormalIPs, map[string]interface{}{
				"ip_address":    topIP.IPAddress,
				"ip_location":   "", // Could be fetched from registration records
				"abnormal_type": abnormalType,
				"detected_time": now.Format(time.RFC3339),
			})
		} else if requestsPerHour > float64(ipPerHour) {
			ipStatus = "high_frequency"
		}

		topIPInfos = append(topIPInfos, map[string]interface{}{
			"rank":          int32(i + 1),
			"ip_address":    topIP.IPAddress,
			"request_count": topIP.RequestCount,
			"status":        ipStatus,
		})
	}

	// Get rate limit logs (recent 429 errors)
	var rateLimitLogs []models.APIRequestLog
	err := db.Get().WithContext(queryCtx).
		Model(&models.APIRequestLog{}).
		Where("created_at >= ? AND status_code = 429", startTime).
		Order("created_at DESC").
		Limit(100).
		Find(&rateLimitLogs).Error
	if err != nil {
		global.Logger.Warnf("failed to get rate limit logs: %v", err)
		// Continue with empty logs if query fails
	}

	// Convert rate limit logs to []map[string]interface{}
	logInfosList := make([]map[string]interface{}, 0, len(rateLimitLogs))
	for _, log := range rateLimitLogs {
		logInfosList = append(logInfosList, map[string]interface{}{
			"time":         log.CreatedAt.Format(time.RFC3339),
			"ip_address":   log.IPAddress,
			"endpoint":     log.Endpoint,
			"limit_reason": "IP 频率超限",
			"current_rate": 0, // Could be calculated from logs
		})
	}

	monitorMap := map[string]interface{}{
		"total_requests":     totalRequests,
		"unique_ips":         uniqueIPs,
		"rate_limited_count": rateLimitedCount,
		"abnormal_ips":       int64(len(abnormalIPs)),
		"top_ips":            topIPInfos,
		"abnormal_ip_list":   abnormalIPs,
		"rate_limit_logs":    logInfosList,
	}

	return MarshalAndReply(monitorMap, "failed to marshal response")
}

// GetIPRateLimitConfig gets IP rate limit configuration
func (s *AdminService) GetIPRateLimitConfig(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	// Default values
	ipPerMinute := int32(100)
	ipPerHour := int32(5000)
	blockType := "all"

	// TODO: Get from config when available
	// config := global.GetConfig()
	// if config.APIManagement != nil && config.APIManagement.RateLimit != nil {
	// 	ipPerMinute = config.APIManagement.RateLimit.IpPerMinute
	// 	ipPerHour = config.APIManagement.RateLimit.IpPerHour
	// }

	response := map[string]interface{}{
		"ip_per_minute": ipPerMinute,
		"ip_per_hour":   ipPerHour,
		"block_type":    blockType,
	}

	return MarshalAndReply(response, "failed to marshal response")
}

// UpdateIPRateLimitConfig updates IP rate limit configuration
func (s *AdminService) UpdateIPRateLimitConfig(ctx context.Context, req *pb.UpdateIPRateLimitConfigRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	// Validate values
	if req.IpPerMinute < 10 || req.IpPerMinute > 1000 {
		return ErrorReply(http.StatusBadRequest, "ip_per_minute must be between 10 and 1000", nil), nil
	}
	if req.IpPerHour < 100 || req.IpPerHour > 50000 {
		return ErrorReply(http.StatusBadRequest, "ip_per_hour must be between 100 and 50000", nil), nil
	}

	// Update config (in production, this should be persisted to database or config file)
	// For now, we'll just return success
	// TODO: Implement config persistence

	response := map[string]interface{}{
		"ip_per_minute": req.IpPerMinute,
		"ip_per_hour":   req.IpPerHour,
		"block_type":    req.BlockType,
	}

	return MarshalAndReply(response, "failed to marshal response")
}

// GetIPRecordsStats gets IP registration records statistics
func (s *AdminService) GetIPRecordsStats(ctx context.Context, req *pb.GetIPRecordsStatsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build base query
	query := db.Get().WithContext(queryCtx).Table("ip_registration_records")

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

	// Get total records
	var totalRecords int64
	err := query.Count(&totalRecords).Error
	if err != nil {
		global.Logger.Errorf("failed to count IP records: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query IP records stats", err), err
	}

	// Get today's new records
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var todayNew int64
	db.Get().WithContext(queryCtx).
		Table("ip_registration_records").
		Where("created_at >= ?", todayStart).
		Count(&todayNew)

	// Get blocked IPs count (IPs in blacklist)
	var blockedCount int64
	db.Get().WithContext(queryCtx).
		Table("ip_registration_records").
		Select("COUNT(DISTINCT ip_registration_records.ip_address)").
		Joins("INNER JOIN ip_blacklist ON ip_registration_records.ip_address = ip_blacklist.ip_address").
		Count(&blockedCount)

	// Get normal IPs count (total unique IPs - blocked IPs)
	var totalUniqueIPs int64
	db.Get().WithContext(queryCtx).
		Table("ip_registration_records").
		Select("COUNT(DISTINCT ip_address)").
		Count(&totalUniqueIPs)
	normalCount := totalUniqueIPs - blockedCount
	if normalCount < 0 {
		normalCount = 0
	}

	statsMap := map[string]interface{}{
		"total_records": totalRecords,
		"today_new":     todayNew,
		"blocked_count": blockedCount,
		"normal_count":  normalCount,
	}

	return MarshalAndReply(statsMap, "failed to marshal response")
}

// GetIPBlacklistStats gets IP blacklist statistics
func (s *AdminService) GetIPBlacklistStats(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Get total count
	var totalCount int64
	err := db.Get().WithContext(queryCtx).Model(&models.IPBlacklist{}).Count(&totalCount).Error
	if err != nil {
		global.Logger.Errorf("failed to count IP blacklist: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query IP blacklist stats", err), err
	}

	// Get today's added count
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var todayAdded int64
	db.Get().WithContext(queryCtx).
		Model(&models.IPBlacklist{}).
		Where("created_at >= ?", todayStart).
		Count(&todayAdded)

	// Get total blocked count (sum of block_count)
	var totalBlocked int64
	db.Get().WithContext(queryCtx).
		Model(&models.IPBlacklist{}).
		Select("COALESCE(SUM(block_count), 0)").
		Scan(&totalBlocked)

	statsMap := map[string]interface{}{
		"total_count":   totalCount,
		"today_added":   todayAdded,
		"total_blocked": totalBlocked,
	}

	return MarshalAndReply(statsMap, "failed to marshal response")
}

// ExportIPList exports IP list (records/blacklist/monitor)
func (s *AdminService) ExportIPList(ctx context.Context, req *pb.ExportIPListRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", nil), nil
	}

	if req.Type == "" {
		return ErrorReply(http.StatusBadRequest, "type is required (records/blacklist/monitor)", nil), nil
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	var csvData string
	var err error

	switch req.Type {
	case "records":
		csvData, err = s.exportIPRecords(queryCtx)
	case "blacklist":
		csvData, err = s.exportIPBlacklist(queryCtx)
	case "monitor":
		csvData, err = s.exportIPMonitor(queryCtx, req.TimeRange)
	default:
		return ErrorReply(http.StatusBadRequest, "invalid type, must be records/blacklist/monitor", nil), nil
	}

	if err != nil {
		global.Logger.Errorf("failed to export IP list: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to export IP list", err), err
	}

	response := map[string]interface{}{
		"csv_data": csvData,
		"type":     req.Type,
	}

	return MarshalAndReply(response, "failed to marshal response")
}

// exportIPRecords exports IP registration records to CSV
func (s *AdminService) exportIPRecords(ctx context.Context) (string, error) {
	var results []struct {
		IPAddress        string    `gorm:"column:ip_address"`
		IPLocation       string    `gorm:"column:ip_location"`
		RegisterCount    int64     `gorm:"column:register_count"`
		LastRegisterTime time.Time `gorm:"column:last_register_time"`
	}

	err := db.Get().WithContext(ctx).
		Table("ip_registration_records").
		Select("ip_address, ip_location, COUNT(*) as register_count, MAX(created_at) as last_register_time").
		Group("ip_address, ip_location").
		Order("register_count DESC").
		Scan(&results).Error

	if err != nil {
		return "", err
	}

	// Build CSV using strings.Builder and encoding/csv
	var builder strings.Builder
	writer := csv.NewWriter(&builder)

	// Write header
	if err := writer.Write([]string{"IP地址", "IP归属地", "注册次数", "最后注册时间"}); err != nil {
		return "", err
	}

	// Write data rows
	for _, r := range results {
		if err := writer.Write([]string{
			r.IPAddress,
			r.IPLocation,
			fmt.Sprintf("%d", r.RegisterCount),
			r.LastRegisterTime.Format("2006-01-02 15:04:05"),
		}); err != nil {
			return "", err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return builder.String(), nil
}

// exportIPBlacklist exports IP blacklist to CSV
func (s *AdminService) exportIPBlacklist(ctx context.Context) (string, error) {
	var blacklist []models.IPBlacklist
	err := db.Get().WithContext(ctx).
		Model(&models.IPBlacklist{}).
		Order("created_at DESC").
		Find(&blacklist).Error

	if err != nil {
		return "", err
	}

	// Build CSV using strings.Builder and encoding/csv
	var builder strings.Builder
	writer := csv.NewWriter(&builder)

	// Write header
	if err := writer.Write([]string{"IP地址", "IP归属地", "原因", "原因描述", "拦截次数", "操作人", "限制类型", "创建时间"}); err != nil {
		return "", err
	}

	// Write data rows
	for _, b := range blacklist {
		if err := writer.Write([]string{
			b.IPAddress,
			b.IPLocation,
			b.Reason,
			b.ReasonText,
			fmt.Sprintf("%d", b.BlockCount),
			b.OperatorName,
			b.BlockType,
			b.CreatedAt.Format("2006-01-02 15:04:05"),
		}); err != nil {
			return "", err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return builder.String(), nil
}

// exportIPMonitor exports IP monitor data to CSV
func (s *AdminService) exportIPMonitor(ctx context.Context, timeRange string) (string, error) {
	// Determine time range
	now := time.Now()
	var startTime time.Time
	switch timeRange {
	case "1h":
		startTime = now.Add(-1 * time.Hour)
	case "6h":
		startTime = now.Add(-6 * time.Hour)
	case "24h", "":
		startTime = now.Add(-24 * time.Hour)
	case "7d":
		startTime = now.AddDate(0, 0, -7)
	default:
		startTime = now.Add(-24 * time.Hour)
	}

	var topIPs []struct {
		IPAddress    string `gorm:"column:ip_address"`
		RequestCount int64  `gorm:"column:request_count"`
	}

	err := db.Get().WithContext(ctx).
		Model(&models.APIRequestLog{}).
		Select("ip_address, COUNT(*) as request_count").
		Where("created_at >= ?", startTime).
		Group("ip_address").
		Order("request_count DESC").
		Limit(1000).
		Scan(&topIPs).Error

	if err != nil {
		return "", err
	}

	// Build CSV using strings.Builder and encoding/csv
	var builder strings.Builder
	writer := csv.NewWriter(&builder)

	// Write header
	if err := writer.Write([]string{"IP地址", "请求次数"}); err != nil {
		return "", err
	}

	// Write data rows
	for _, ip := range topIPs {
		if err := writer.Write([]string{
			ip.IPAddress,
			fmt.Sprintf("%d", ip.RequestCount),
		}); err != nil {
			return "", err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return builder.String(), nil
}
