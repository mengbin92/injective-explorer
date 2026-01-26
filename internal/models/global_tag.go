// Package models provides data models for blockchain entities.
package models

import (
	"gorm.io/gorm"
)

// GlobalTag represents a global tag in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores global tag information that can be used across multiple contracts.
// Unlike ContractTag, GlobalTag is not bound to a specific contract address.
type GlobalTag struct {
	gorm.Model

	ChainID     string `json:"chain_id" gorm:"column:chain_id;size:50;not null;uniqueIndex:uk_global_tag"`
	TagType     string `json:"tag_type" gorm:"column:tag_type;size:50;not null;uniqueIndex:uk_global_tag"`
	TagValue    string `json:"tag_value" gorm:"column:tag_value;size:255"`
	TagColor    string `json:"tag_color" gorm:"column:tag_color;size:20"`
	Description string `json:"description" gorm:"column:description;type:text"`
	CreatedBy   string `json:"created_by" gorm:"column:created_by;size:128"`
}

// TableName specifies the table name for GlobalTag.
func (GlobalTag) TableName() string {
	return "global_tags"
}
