// Package models provides data models for blockchain entities.
package models

import (
	"time"

	pb "injective-explorer/api/explorer/v1"
	"gorm.io/gorm"
)

// TagDefinition represents a predefined tag definition in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// This table stores system-preset tags that can be used for addresses and contracts.
// It provides standardized tag definitions with colors, categories, and usage statistics.
type TagDefinition struct {
	gorm.Model

	// TagType specifies the type of entity this tag applies to: "address" or "contract"
	TagType string `json:"tag_type" gorm:"column:tag_type;size:20;not null;index:idx_tag_type;uniqueIndex:uk_tag_type_name,priority:1"`

	// TagName is the display name of the tag (e.g., "官方", "稳定币", "诈骗")
	TagName string `json:"tag_name" gorm:"column:tag_name;size:50;not null;uniqueIndex:uk_tag_type_name,priority:2"`

	// TagCategory classifies the tag: "normal" (green) or "abnormal" (red)
	TagCategory string `json:"tag_category" gorm:"column:tag_category;size:20;not null;index:idx_tag_category"`

	// TagColor is the hex color code for displaying the tag (e.g., "#28a745", "#dc3545")
	TagColor string `json:"tag_color" gorm:"column:tag_color;size:20"`

	// Description provides additional information about the tag
	Description string `json:"description" gorm:"column:description;type:text"`

	// IsSystem indicates whether this is a system-preset tag (true) or user-defined (false)
	IsSystem bool `json:"is_system" gorm:"column:is_system;default:1;index:idx_is_system"`

	// UsageCount tracks how many times this tag has been used
	UsageCount int `json:"usage_count" gorm:"column:usage_count;default:0"`

	// SortOrder controls the display order in frontend selectors
	SortOrder int `json:"sort_order" gorm:"column:sort_order;default:0"`

	// CreatedBy is the user who created this tag definition
	CreatedBy string `json:"created_by" gorm:"column:created_by;size:128"`
}

// TableName specifies the table name for TagDefinition.
func (TagDefinition) TableName() string {
	return "tag_definitions"
}

// ToTagDefinitionDTO converts TagDefinition to proto TagDefinitionDTO.
func (t *TagDefinition) ToTagDefinitionDTO() *pb.TagDefinitionDTO {
	if t == nil {
		return nil
	}

	var createdAt string
	if !t.CreatedAt.IsZero() {
		createdAt = t.CreatedAt.Format(time.RFC3339)
	}

	var updatedAt string
	if !t.UpdatedAt.IsZero() {
		updatedAt = t.UpdatedAt.Format(time.RFC3339)
	}

	return &pb.TagDefinitionDTO{
		Id:          uint64(t.ID),
		TagType:     t.TagType,
		TagName:     t.TagName,
		TagCategory: t.TagCategory,
		TagColor:    t.TagColor,
		Description: t.Description,
		IsSystem:    t.IsSystem,
		UsageCount:  int32(t.UsageCount),
		SortOrder:   int32(t.SortOrder),
		CreatedBy:   t.CreatedBy,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}
