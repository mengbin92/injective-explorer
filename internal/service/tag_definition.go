// Package service provides business logic implementations for blockchain explorer.
package service

import (
	"context"
	"fmt"
	"net/http"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// TagDefinitionService manages predefined tag definitions.
type TagDefinitionService struct {
	pb.UnimplementedTagDefinitionServiceServer
}

// NewTagDefinitionService creates a new instance of TagDefinitionService.
func NewTagDefinitionService() *TagDefinitionService {
	return &TagDefinitionService{}
}

// ListTagDefinitions lists tag definitions with filters.
func (s *TagDefinitionService) ListTagDefinitions(ctx context.Context, req *pb.ListTagDefinitionsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Pagination.Page, req.Pagination.PageSize, req.Pagination.Cursor, MaxPageSize)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.TagDefinition{})

	// Apply filters
	if req.TagType != "" {
		query = query.Where("tag_type = ?", req.TagType)
	}

	if req.TagCategory != "" {
		query = query.Where("tag_category = ?", req.TagCategory)
	}

	if req.IsSystem {
		query = query.Where("is_system = ?", 1)
	}

	// Get total count
	var total int64
	var isExactCount bool
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(db.Get(), &models.TagDefinition{}, ctx)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for tag definitions: %v, using fallback value", countErr)
		total = ApproximateCountFallback
		isExactCount = false
	}

	// Apply pagination
	var definitions []models.TagDefinition
	if useCursor {
		query = query.Where("id < ?", cursorValue).
			Order("id DESC").
			Limit(int(pageSize))
	} else {
		query = query.Order("sort_order ASC, id DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	err := query.Find(&definitions).Error
	if err != nil {
		global.Logger.Errorf("failed to list tag definitions: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to list tag definitions", err), err
	}

	// Build response
	nextCursor := calculateNextCursor(useCursor, cursorValue, definitions, func(d models.TagDefinition) uint64 { return uint64(d.ID) })
	definitionDTOs := toTagDefinitionDTOs(definitions)
	defsResp := buildPaginatedResponse(definitionDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(definitions))

	return MarshalAndReply(defsResp, "marshal tag definitions error")
}

// CreateTagDefinition creates a new tag definition.
func (s *TagDefinitionService) CreateTagDefinition(ctx context.Context, req *pb.CreateTagDefinitionRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.TagType == "" {
		return ErrorReply(http.StatusBadRequest, "tag_type is required", errors.New("tag_type is required")), errors.New("tag_type is required")
	}

	if req.TagName == "" {
		return ErrorReply(http.StatusBadRequest, "tag_name is required", errors.New("tag_name is required")), errors.New("tag_name is required")
	}

	if req.TagCategory == "" {
		return ErrorReply(http.StatusBadRequest, "tag_category is required", errors.New("tag_category is required")), errors.New("tag_category is required")
	}

	// Validate tag_type
	if req.TagType != "address" && req.TagType != "contract" {
		return ErrorReply(http.StatusBadRequest, "tag_type must be 'address' or 'contract'",
			fmt.Errorf("invalid tag_type: %s", req.TagType)), fmt.Errorf("invalid tag_type: %s", req.TagType)
	}

	// Validate tag_category
	if req.TagCategory != "normal" && req.TagCategory != "abnormal" {
		return ErrorReply(http.StatusBadRequest, "tag_category must be 'normal' or 'abnormal'",
			fmt.Errorf("invalid tag_category: %s", req.TagCategory)), fmt.Errorf("invalid tag_category: %s", req.TagCategory)
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Check if tag definition already exists
	var existingDef models.TagDefinition
	err := db.Get().WithContext(queryCtx).
		Where("tag_type = ? AND tag_name = ?", req.TagType, req.TagName).
		First(&existingDef).Error

	if err == nil {
		// Tag definition already exists
		return ErrorReply(http.StatusConflict, "tag definition already exists",
			errors.New("tag definition already exists")), errors.New("tag definition already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Errorf("failed to check existing tag definition: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to check existing tag definition", err), err
	}

	// Create tag definition
	definition := &models.TagDefinition{
		TagType:     req.TagType,
		TagName:     req.TagName,
		TagCategory: req.TagCategory,
		TagColor:    req.TagColor,
		Description: req.Description,
		IsSystem:    false, // User-created tags are not system tags
		SortOrder:   int(req.SortOrder),
		CreatedBy:   req.CreatedBy,
	}

	err = db.Get().WithContext(queryCtx).Create(definition).Error
	if err != nil {
		global.Logger.Errorf("failed to create tag definition: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to create tag definition", err), err
	}

	// Convert to response
	dto := definition.ToTagDefinitionDTO()
	return MarshalAndReply(dto, "marshal tag definition error")
}

// UpdateTagDefinition updates an existing tag definition.
func (s *TagDefinitionService) UpdateTagDefinition(ctx context.Context, req *pb.UpdateTagDefinitionRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Id == 0 {
		return ErrorReply(http.StatusBadRequest, "id is required", errors.New("id is required")), errors.New("id is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Find tag definition
	var definition models.TagDefinition
	err := db.Get().WithContext(queryCtx).First(&definition, req.Id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "tag definition not found", err), err
		}
		global.Logger.Errorf("failed to find tag definition: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to find tag definition", err), err
	}

	// Build updates map
	updates := make(map[string]interface{})

	if req.TagName != "" {
		// Check if new name conflicts with existing definition
		var existingDef models.TagDefinition
		err := db.Get().WithContext(queryCtx).
			Where("tag_type = ? AND tag_name = ? AND id != ?", definition.TagType, req.TagName, req.Id).
			First(&existingDef).Error
		if err == nil {
			return ErrorReply(http.StatusConflict, "tag name already exists",
				errors.New("tag name already exists")), errors.New("tag name already exists")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusInternalServerError, "failed to check existing tag definition", err), err
		}

		updates["tag_name"] = req.TagName
	}

	if req.TagCategory != "" {
		updates["tag_category"] = req.TagCategory
	}

	if req.TagColor != "" {
		updates["tag_color"] = req.TagColor
	}

	if req.Description != "" {
		updates["description"] = req.Description
	}

	if req.SortOrder > 0 {
		updates["sort_order"] = req.SortOrder
	}

	if len(updates) == 0 {
		return ErrorReply(http.StatusBadRequest, "at least one field must be provided for update",
			errors.New("no fields to update")), errors.New("no fields to update")
	}

	// Update
	err = db.Get().WithContext(queryCtx).Model(&definition).Updates(updates).Error
	if err != nil {
		global.Logger.Errorf("failed to update tag definition: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to update tag definition", err), err
	}

	// Reload to get updated data
	err = db.Get().WithContext(queryCtx).First(&definition, req.Id).Error
	if err != nil {
		global.Logger.Errorf("failed to reload tag definition: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to reload tag definition", err), err
	}

	// Convert to response
	dto := definition.ToTagDefinitionDTO()
	return MarshalAndReply(dto, "marshal tag definition error")
}

// DeleteTagDefinition deletes a tag definition.
func (s *TagDefinitionService) DeleteTagDefinition(ctx context.Context, req *pb.DeleteTagDefinitionRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Id == 0 {
		return ErrorReply(http.StatusBadRequest, "id is required", errors.New("id is required")), errors.New("id is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Check if tag definition exists and is not a system tag
	var definition models.TagDefinition
	err := db.Get().WithContext(queryCtx).First(&definition, req.Id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "tag definition not found", err), err
		}
		global.Logger.Errorf("failed to find tag definition: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to find tag definition", err), err
	}

	// Prevent deletion of system tags
	if definition.IsSystem {
		return ErrorReply(http.StatusForbidden, "cannot delete system preset tags",
			errors.New("cannot delete system preset tags")), errors.New("cannot delete system preset tags")
	}

	// Check if tag is in use
	usageCount := 0
	if definition.TagType == "contract" {
		var count int64
		db.Get().WithContext(queryCtx).Model(&models.ContractTag{}).
			Where("tag_definition_id = ?", req.Id).
			Count(&count)
		usageCount = int(count)
	}

	if usageCount > 0 {
		return ErrorReply(http.StatusConflict,
			fmt.Sprintf("cannot delete tag definition: it is being used by %d contracts", usageCount),
			fmt.Errorf("tag definition is in use by %d contracts", usageCount)),
			fmt.Errorf("tag definition is in use")
	}

	// Soft delete
	err = db.Get().WithContext(queryCtx).Delete(&definition).Error
	if err != nil {
		global.Logger.Errorf("failed to delete tag definition: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to delete tag definition", err), err
	}

	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "tag definition deleted successfully",
		Data:    nil,
	}, nil
}

// GetTagUsageStats gets usage statistics for tags.
func (s *TagDefinitionService) GetTagUsageStats(ctx context.Context, req *pb.GetTagUsageStatsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query
	var stats []*pb.TagUsageStatsDTO

	if req.TagType == "" || req.TagType == "contract" {
		// Get contract tag usage stats from tag_definitions
		var contractStats []models.TagDefinition
		err := db.Get().WithContext(queryCtx).
			Select("tag_name, usage_count, tag_category").
			Where("tag_type = ?", "contract").
			Order("usage_count DESC").
			Find(&contractStats).Error
		if err != nil {
			global.Logger.Errorf("failed to get contract tag stats: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to get contract tag stats", err), err
		}

		for _, stat := range contractStats {
			stats = append(stats, &pb.TagUsageStatsDTO{
				TagName:     stat.TagName,
				UsageCount:  int32(stat.UsageCount),
				TagCategory: stat.TagCategory,
			})
		}
	}

	if req.TagType == "" || req.TagType == "address" {
		// Get address tag usage stats
		// Note: address_labels don't have tag_definition_id, so we count by label_value
		type AddressTagStat struct {
			LabelValue string
			Count      int64
		}

		var addressStats []AddressTagStat
		err := db.Get().WithContext(queryCtx).
			Model(&models.AddressLabel{}).
			Select("label_value as tag_name, count(*) as count").
			Group("label_value").
			Order("count DESC").
			Find(&addressStats).Error
		if err != nil {
			global.Logger.Errorf("failed to get address tag stats: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to get address tag stats", err), err
		}

		for _, stat := range addressStats {
			stats = append(stats, &pb.TagUsageStatsDTO{
				TagName:    stat.LabelValue,
				UsageCount: int32(stat.Count),
				// Address tags don't have category, set to "normal" by default
				TagCategory: "normal",
			})
		}
	}

	return MarshalAndReply(stats, "marshal tag stats error")
}

// GetTagDefinitionsForSelection gets tag definitions for frontend selection.
func (s *TagDefinitionService) GetTagDefinitionsForSelection(ctx context.Context, req *pb.GetTagDefinitionsForSelectionRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.TagType == "" {
		return ErrorReply(http.StatusBadRequest, "tag_type is required", errors.New("tag_type is required")), errors.New("tag_type is required")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).Model(&models.TagDefinition{}).
		Where("tag_type = ?", req.TagType)

	if req.TagCategory != "" {
		query = query.Where("tag_category = ?", req.TagCategory)
	}

	// Order by sort_order and name
	query = query.Order("sort_order ASC, tag_name ASC")

	var definitions []models.TagDefinition
	err := query.Find(&definitions).Error
	if err != nil {
		global.Logger.Errorf("failed to get tag definitions for selection: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get tag definitions for selection", err), err
	}

	// Convert to DTO
	dtos := toTagDefinitionDTOs(definitions)
	return MarshalAndReply(dtos, "marshal tag definitions error")
}

// Helper functions

// toTagDefinitionDTOs converts []TagDefinition to []*pb.TagDefinitionDTO.
func toTagDefinitionDTOs(definitions []models.TagDefinition) []*pb.TagDefinitionDTO {
	if definitions == nil {
		return nil
	}

	dtos := make([]*pb.TagDefinitionDTO, 0, len(definitions))
	for i := range definitions {
		if dto := definitions[i].ToTagDefinitionDTO(); dto != nil {
			dtos = append(dtos, dto)
		}
	}
	return dtos
}
