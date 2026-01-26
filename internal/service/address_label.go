// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// AddressLabelService implements the address label management service.
// It provides methods to manage address labels including CRUD operations
// and query by address or label type.
type AddressLabelService struct {
	pb.UnimplementedAddressLabelServiceServer
}

// NewAddressLabelService creates a new instance of AddressLabelService.
//
// Returns:
//   - *AddressLabelService: A new service instance
func NewAddressLabelService() *AddressLabelService {
	return &AddressLabelService{}
}

// Cache key constants for address label service
const (
	CacheKeyAddressLabels = "address_label:labels:%s:%s" // chain_id:address
)

// Cache expiration constants
const (
	CacheExpireAddressLabels = 1 * time.Hour
)

// CreateAddressLabel creates a new address label.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing address label information
//
// Returns:
//   - *pb.Reply: Response containing the created label
//   - error: Error if the operation fails
func (s *AddressLabelService) CreateAddressLabel(ctx context.Context, req *pb.CreateAddressLabelRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return ErrorReply(http.StatusBadRequest, "label_type is required", errors.New("label_type is required")), errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Create label
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	label := &models.AddressLabel{
		Address:     address,
		ChainID:     req.ChainId,
		LabelType:   req.LabelType,
		LabelValue:  req.LabelValue,
		LabelColor:  req.LabelColor,
		Description: req.Description,
		Visible:     req.Visible, // New field: visibility control
		CreatedBy:   req.CreatedBy,
	}

	// Use FirstOrCreate to avoid duplicates
	// Note: Updated unique constraint to use (chain_id, address, label_value)
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_value = ?", req.ChainId, address, req.LabelValue).
		FirstOrCreate(label).Error

	if err != nil {
		global.Logger.Errorf("failed to create address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to create address label", err), err
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	// Convert to response
	labelDTO := models.ToAddressLabelDTO(label)
	return MarshalAndReply(labelDTO, "marshal label error")
}

// UpdateAddressLabel updates an existing address label.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing address label update information
//
// Returns:
//   - *pb.Reply: Response containing the updated label
//   - error: Error if the operation fails
func (s *AddressLabelService) UpdateAddressLabel(ctx context.Context, req *pb.UpdateAddressLabelRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return ErrorReply(http.StatusBadRequest, "label_type is required", errors.New("label_type is required")), errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Update label
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	updates := make(map[string]interface{})
	if req.LabelValue != "" {
		updates["label_value"] = req.LabelValue
	}
	if req.LabelColor != "" {
		updates["label_color"] = req.LabelColor
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	// New: support updating visible field
	updates["visible"] = req.Visible

	if len(updates) == 0 {
		return ErrorReply(http.StatusBadRequest, "at least one field must be provided for update", errors.New("no fields to update")), errors.New("no fields to update")
	}

	var label models.AddressLabel
	// Updated query: use (chain_id, address, label_value) instead of (chain_id, address, label_type)
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_value = ?", req.ChainId, address, req.LabelValue).
		First(&label).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "address label not found", err), err
		}
		global.Logger.Errorf("failed to find address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to find address label", err), err
	}

	err = db.Get().WithContext(queryCtx).
		Model(&models.AddressLabel{}).
		Where("chain_id = ? AND address = ? AND label_value = ?", req.ChainId, address, req.LabelValue).
		Updates(updates).Error

	if err != nil {
		global.Logger.Errorf("failed to update address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to update address label", err), err
	}

	// Reload to get updated data
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_value = ?", req.ChainId, address, req.LabelValue).
		First(&label).Error

	if err != nil {
		global.Logger.Errorf("failed to reload address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to reload address label", err), err
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	// Convert to response
	labelDTO := models.ToAddressLabelDTO(&label)
	return MarshalAndReply(labelDTO, "marshal label error")
}

// DeleteAddressLabel deletes an address label.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing address, chain ID, and label type
//
// Returns:
//   - *pb.Reply: Response indicating success
//   - error: Error if the operation fails
func (s *AddressLabelService) DeleteAddressLabel(ctx context.Context, req *pb.DeleteAddressLabelRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.LabelType == "" {
		return ErrorReply(http.StatusBadRequest, "label_type is required", errors.New("label_type is required")), errors.New("label_type is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Delete label (soft delete)
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ? AND label_type = ?", req.ChainId, address, req.LabelType).
		Delete(&models.AddressLabel{}).Error

	if err != nil {
		global.Logger.Errorf("failed to delete address label: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to delete address label", err), err
	}

	// Clear cache
	s.clearAddressLabelCache(ctx, req.ChainId, address)

	// Return success response
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
	}, nil
}

// GetAddressLabels retrieves all labels for an address.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing address and chain ID
//
// Returns:
//   - *pb.Reply: Response containing list of labels
//   - error: Error if the operation fails
func (s *AddressLabelService) GetAddressLabels(ctx context.Context, req *pb.GetAddressLabelsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Address == "" {
		return ErrorReply(http.StatusBadRequest, "address is required", errors.New("address is required")), errors.New("address is required")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	// Convert address to Ethereum format for storage
	address := models.ConvertAddressToEthForStorage(req.Address)

	// Try to get from cache
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	cachedLabels, err := s.getLabelsFromCache(queryCtx, req.ChainId, address)
	if err == nil && cachedLabels != nil {
		global.Logger.Debugf("address labels found in cache: address=%s", address)
		labelDTOs := models.ToAddressLabelDTOs(cachedLabels)
		result := map[string]interface{}{"labels": labelDTOs}
		return MarshalAndReply(result, "marshal labels error")
	}

	// Query from database
	var labels []models.AddressLabel
	err = db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND address = ?", req.ChainId, address).
		Find(&labels).Error

	if err != nil {
		global.Logger.Errorf("failed to get address labels: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get address labels", err), err
	}

	// Cache the labels
	go s.cacheLabels(context.Background(), req.ChainId, address, labels)

	// Convert to response
	labelDTOs := models.ToAddressLabelDTOs(labels)
	result := map[string]interface{}{"labels": labelDTOs}
	return MarshalAndReply(result, "marshal labels error")
}

// ListAddressLabels lists address labels with filter conditions.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing filter conditions and pagination parameters
//
// Returns:
//   - *pb.Reply: Response containing paginated list of labels
//   - error: Error if the operation fails
func (s *AddressLabelService) ListAddressLabels(ctx context.Context, req *pb.ListAddressLabelsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	var paginationReq *pb.PaginationRequest
	if req.Pagination != nil {
		paginationReq = req.Pagination
	} else {
		paginationReq = &pb.PaginationRequest{
			Page:     MinPage,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, useCursor, cursorValue := normalizePagination(paginationReq.Page, paginationReq.PageSize, paginationReq.Cursor, MaxPageSize)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.AddressLabel{})

	// Apply filters
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	if req.LabelType != "" {
		query = query.Where("label_type = ?", req.LabelType)
	}

	if req.Address != "" {
		address := models.ConvertAddressToEthForStorage(req.Address)
		query = query.Where("address LIKE ?", "%"+address+"%")
	}

	if req.LabelValue != "" {
		query = query.Where("label_value LIKE ?", "%"+req.LabelValue+"%")
	}

	// Get total count
	var total int64
	var isExactCount bool
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(db.Get(), &models.AddressLabel{}, ctx)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for address labels: %v, using fallback value", countErr)
		total = ApproximateCountFallback
		isExactCount = false
	}

	// Apply pagination
	var labels []models.AddressLabel
	if useCursor {
		query = query.Where("id < ?", cursorValue).
			Order("id DESC").
			Limit(int(pageSize))
	} else {
		query = query.Order("id DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	err := query.Find(&labels).Error
	if err != nil {
		global.Logger.Errorf("failed to list address labels: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to list address labels", err), err
	}

	// Build response
	nextCursor := calculateNextCursor(useCursor, cursorValue, labels, func(l models.AddressLabel) uint64 { return uint64(l.ID) })
	labelDTOs := models.ToAddressLabelDTOs(labels)
	labelsResp := buildPaginatedResponse(labelDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(labels))

	return MarshalAndReply(labelsResp, "marshal labels error")
}

// Cache helper methods

// getLabelsFromCache retrieves address labels from Redis cache.
func (s *AddressLabelService) getLabelsFromCache(ctx context.Context, chainID, address string) ([]models.AddressLabel, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	data, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found in cache
		}
		return nil, err
	}

	var labels []models.AddressLabel
	err = sonic.Unmarshal([]byte(data), &labels)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal labels from cache")
	}

	return labels, nil
}

// cacheLabels caches address labels to Redis.
func (s *AddressLabelService) cacheLabels(ctx context.Context, chainID, address string, labels []models.AddressLabel) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	labelsJSON, err := sonic.Marshal(labels)
	if err != nil {
		global.Logger.Warnf("failed to marshal labels to JSON: %v", err)
		return
	}

	err = redisClient.Set(ctx, key, labelsJSON, CacheExpireAddressLabels).Err()
	if err != nil {
		global.Logger.Warnf("failed to cache labels: %v", err)
	}
}

// clearAddressLabelCache clears the cache for address labels.
func (s *AddressLabelService) clearAddressLabelCache(ctx context.Context, chainID, address string) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := fmt.Sprintf(CacheKeyAddressLabels, chainID, address)
	if err := redisClient.Del(ctx, key).Err(); err != nil && err != redis.Nil {
		global.Logger.Warnf("failed to clear address label cache: %v", err)
	}
}

// GetAddressLabelStats gets statistics for address labels.
func (s *AddressLabelService) GetAddressLabelStats(ctx context.Context, req *pb.GetAddressLabelStatsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.AddressLabel{})
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	// Get total labeled addresses (distinct)
	var totalLabeledAddresses int64
	query.Model(&models.AddressLabel{}).
		Select("COUNT(DISTINCT CONCAT(chain_id, ':', address))").
		Scan(&totalLabeledAddresses)

	// Get visible labels count
	var visibleLabelsCount int64
	query.Where("visible = ?", true).Count(&visibleLabelsCount)

	// Get hidden labels count
	var hiddenLabelsCount int64
	hiddenQuery := db.Get().WithContext(queryCtx).Model(&models.AddressLabel{}).
		Where("visible = ?", false)
	if req.ChainId != "" {
		hiddenQuery = hiddenQuery.Where("chain_id = ?", req.ChainId)
	}
	hiddenQuery.Count(&hiddenLabelsCount)

	// Get total labels count
	var totalLabelsCount int64
	totalQuery := db.Get().WithContext(queryCtx).Model(&models.AddressLabel{})
	if req.ChainId != "" {
		totalQuery = totalQuery.Where("chain_id = ?", req.ChainId)
	}
	totalQuery.Count(&totalLabelsCount)

	// Build response
	stats := &pb.AddressLabelStatsDTO{
		TotalLabeledAddresses: int32(totalLabeledAddresses),
		VisibleLabelsCount:    int32(visibleLabelsCount),
		HiddenLabelsCount:     int32(hiddenLabelsCount),
		TotalLabelsCount:      int32(totalLabelsCount),
	}

	return MarshalAndReply(stats, "marshal stats error")
}

// BatchImportAddressLabels imports address labels from CSV/JSON file.
func (s *AddressLabelService) BatchImportAddressLabels(ctx context.Context, req *pb.BatchImportAddressLabelsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.Format == "" {
		return ErrorReply(http.StatusBadRequest, "format is required (csv or json)", errors.New("format is required")), errors.New("format is required")
	}

	if req.FileContentBase64 == "" {
		return ErrorReply(http.StatusBadRequest, "file_content_base64 is required", errors.New("file_content_base64 is required")), errors.New("file_content_base64 is required")
	}

	var labels []AddressLabelImport
	var err error

	// Decode base64 content
	fileContent, err := base64.StdEncoding.DecodeString(req.FileContentBase64)
	if err != nil {
		return ErrorReply(http.StatusBadRequest, "failed to decode base64 content", err), err
	}

	// Parse based on format
	if strings.ToLower(req.Format) == "csv" {
		labels, err = s.parseCSVLabels(fileContent, req.CsvFormat)
		if err != nil {
			return ErrorReply(http.StatusBadRequest, "failed to parse CSV", err), err
		}
	} else if strings.ToLower(req.Format) == "json" {
		labels, err = s.parseJSONLabels(fileContent)
		if err != nil {
			return ErrorReply(http.StatusBadRequest, "failed to parse JSON", err), err
		}
	} else {
		return ErrorReply(http.StatusBadRequest, "unsupported format: "+req.Format,
			errors.New("unsupported format")), errors.New("unsupported format")
	}

	// Import labels
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	successCount := 0
	skippedCount := 0
	failedCount := 0
	var errorsList []string

	for _, label := range labels {
		// Convert address to Ethereum format
		address := models.ConvertAddressToEthForStorage(label.Address)

		// Check if label already exists
		var existingLabel models.AddressLabel
		err := db.Get().WithContext(queryCtx).
			Where("chain_id = ? AND address = ? AND label_value = ?", req.ChainId, address, label.LabelValue).
			First(&existingLabel).Error

		if err == nil {
			// Label already exists
			if req.ImportMode == "create" {
				skippedCount++
				continue
			} else if req.ImportMode == "update" || req.ImportMode == "upsert" {
				// Update existing label
				updates := map[string]interface{}{
					"label_type":  label.LabelType,
					"label_color": label.LabelColor,
					"description": label.Description,
					"visible":     label.Visible,
				}
				err = db.Get().WithContext(queryCtx).Model(&existingLabel).Updates(updates).Error
				if err != nil && !req.SkipErrors {
					failedCount++
					errorsList = append(errorsList, fmt.Sprintf("Failed to update label for %s: %v", label.Address, err))
					continue
				}
				successCount++
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new label
			newLabel := &models.AddressLabel{
				Address:     address,
				ChainID:     req.ChainId,
				LabelType:   label.LabelType,
				LabelValue:  label.LabelValue,
				LabelColor:  label.LabelColor,
				Description: label.Description,
				Visible:     label.Visible,
				CreatedBy:   label.CreatedBy,
			}
			err = db.Get().WithContext(queryCtx).Create(newLabel).Error
			if err != nil && !req.SkipErrors {
				failedCount++
				errorsList = append(errorsList, fmt.Sprintf("Failed to create label for %s: %v", label.Address, err))
				continue
			}
			successCount++
		} else {
			failedCount++
			errorsList = append(errorsList, fmt.Sprintf("Database error for %s: %v", label.Address, err))
			if !req.SkipErrors {
				break
			}
		}

		// Handle remark if present
		if label.Remark != "" {
			var remark models.AddressRemark
			err := db.Get().WithContext(queryCtx).
				Where("chain_id = ? AND address = ?", req.ChainId, address).
				First(&remark).Error

			if err == nil {
				// Update existing remark
				db.Get().WithContext(queryCtx).Model(&remark).Update("remark", label.Remark)
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				// Create new remark
				newRemark := &models.AddressRemark{
					Address:   address,
					ChainID:   req.ChainId,
					Remark:    label.Remark,
					CreatedBy: label.CreatedBy,
				}
				db.Get().WithContext(queryCtx).Create(newRemark)
			}
		}
	}

	// Build response
	response := &pb.BatchImportAddressLabelsResponse{
		TotalCount:   int32(len(labels)),
		SuccessCount: int32(successCount),
		SkippedCount: int32(skippedCount),
		FailedCount:  int32(failedCount),
		Errors:       errorsList,
	}

	return MarshalAndReply(response, "marshal response error")
}

// BatchExportAddressLabels exports address labels to CSV/JSON file.
func (s *AddressLabelService) BatchExportAddressLabels(ctx context.Context, req *pb.BatchExportAddressLabelsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Format == "" {
		return ErrorReply(http.StatusBadRequest, "format is required (csv or json)", errors.New("format is required")), errors.New("format is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.AddressLabel{})
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	if len(req.Addresses) > 0 {
		// Filter by specific addresses
		addresses := make([]string, len(req.Addresses))
		for i, addr := range req.Addresses {
			addresses[i] = models.ConvertAddressToEthForStorage(addr)
		}
		query = query.Where("address IN ?", addresses)
	}

	if !req.IncludeHidden {
		query = query.Where("visible = ?", true)
	}

	// Query labels
	var labels []models.AddressLabel
	err := query.Order("id DESC").Find(&labels).Error
	if err != nil {
		global.Logger.Errorf("failed to query labels for export: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query labels", err), err
	}

	// Generate file content
	var fileContent string
	var fileName string

	if strings.ToLower(req.Format) == "csv" {
		fileContent, fileName = s.exportLabelsToCSV(labels)
	} else if strings.ToLower(req.Format) == "json" {
		fileContent, fileName = s.exportLabelsToJSON(labels)
	} else {
		return ErrorReply(http.StatusBadRequest, "unsupported format: "+req.Format,
			errors.New("unsupported format")), errors.New("unsupported format")
	}

	// Encode to base64
	fileContentBase64 := base64.StdEncoding.EncodeToString([]byte(fileContent))

	// Build response
	response := &pb.BatchExportAddressLabelsResponse{
		FileContentBase64: fileContentBase64,
		FileName:          fileName,
		RecordCount:       int32(len(labels)),
	}

	return MarshalAndReply(response, "marshal response error")
}

// Helper types and functions

// AddressLabelImport represents an address label from import file.
type AddressLabelImport struct {
	Address     string
	LabelType   string
	LabelValue  string
	LabelColor  string
	Description string
	Visible     bool
	CreatedBy   string
	Remark      string
}

// parseCSVLabels parses CSV content into address labels.
func (s *AddressLabelService) parseCSVLabels(content []byte, csvFormat string) ([]AddressLabelImport, error) {
	reader := csv.NewReader(strings.NewReader(string(content)))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read CSV")
	}

	var labels []AddressLabelImport

	// Detect CSV format
	if csvFormat == "" {
		// Auto-detect format from first row
		if len(records) > 0 && len(records[0]) >= 3 {
			firstRow := records[0]
			if strings.Contains(strings.ToLower(firstRow[0]), "address") ||
			   strings.Contains(strings.ToLower(firstRow[1]), "label") {
				csvFormat = "full"
			} else {
				csvFormat = "simple"
			}
		} else {
			csvFormat = "simple"
		}
	}

	for i, record := range records {
		// Skip header row if present
		if i == 0 && (strings.Contains(strings.ToLower(record[0]), "address") ||
		   strings.Contains(strings.ToLower(record[0]), "地址")) {
			continue
		}

		var label AddressLabelImport

		if csvFormat == "simple" {
			// Simple format: address,label_name,remark
			if len(record) < 3 {
				continue
			}
			label = AddressLabelImport{
				Address:    record[0],
				LabelValue: record[1],
				Remark:     record[2],
				LabelType:  "custom", // Default type
				Visible:    true,    // Default visible
			}
		} else {
			// Full format: address,label_name,label_type,label_color,description,visible,remark
			if len(record) < 7 {
				continue
			}
			label = AddressLabelImport{
				Address:     record[0],
				LabelValue:  record[1],
				LabelType:   record[2],
				LabelColor:  record[3],
				Description: record[4],
				Visible:     strings.ToLower(record[5]) == "true",
				Remark:      record[6],
			}
		}

		labels = append(labels, label)
	}

	return labels, nil
}

// parseJSONLabels parses JSON content into address labels.
func (s *AddressLabelService) parseJSONLabels(content []byte) ([]AddressLabelImport, error) {
	var labels []AddressLabelImport
	err := sonic.Unmarshal(content, &labels)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON")
	}
	return labels, nil
}

// exportLabelsToCSV exports labels to CSV format.
func (s *AddressLabelService) exportLabelsToCSV(labels []models.AddressLabel) (string, string) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"Address", "LabelValue", "LabelType", "LabelColor", "Description", "Visible", "CreatedAt", "UpdatedAt"})

	// Write data
	for _, label := range labels {
		visible := "false"
		if label.Visible {
			visible = "true"
		}
		writer.Write([]string{
			label.Address,
			label.LabelValue,
			label.LabelType,
			label.LabelColor,
			label.Description,
			visible,
			label.CreatedAt.Format("2006-01-02 15:04:05"),
			label.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	fileName := fmt.Sprintf("address_labels_%s.csv", time.Now().Format("20060102_150405"))
	return buf.String(), fileName
}

// exportLabelsToJSON exports labels to JSON format.
func (s *AddressLabelService) exportLabelsToJSON(labels []models.AddressLabel) (string, string) {
	data := make([]map[string]interface{}, 0, len(labels))

	for _, label := range labels {
		data = append(data, map[string]interface{}{
			"address":     label.Address,
			"chain_id":    label.ChainID,
			"label_type":  label.LabelType,
			"label_value": label.LabelValue,
			"label_color": label.LabelColor,
			"description": label.Description,
			"visible":     label.Visible,
			"created_by":  label.CreatedBy,
			"created_at":  label.CreatedAt.Format(time.RFC3339),
			"updated_at":  label.UpdatedAt.Format(time.RFC3339),
		})
	}

	jsonBytes, err := sonic.Marshal(data)
	if err != nil {
		global.Logger.Errorf("failed to marshal JSON: %v", err)
		return "", ""
	}

	fileName := fmt.Sprintf("address_labels_%s.json", time.Now().Format("20060102_150405"))
	return string(jsonBytes), fileName
}
