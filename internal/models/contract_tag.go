// Package models provides data models for blockchain entities.
package models

import (
	"gorm.io/gorm"
)

// ContractTag represents a contract tag in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores tag information for contracts, supporting multiple tags per contract.
type ContractTag struct {
	gorm.Model

	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null"`
	ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50;not null"`
	TagType         string `json:"tag_type" gorm:"column:tag_type;size:50;not null"`
	TagValue        string `json:"tag_value" gorm:"column:tag_value;size:255"`
	TagColor        string `json:"tag_color" gorm:"column:tag_color;size:20"`
	Description     string `json:"description" gorm:"column:description;type:text"`
	CreatedBy       string `json:"created_by" gorm:"column:created_by;size:128"`
}

// TableName specifies the table name for ContractTag.
func (ContractTag) TableName() string {
	return "contract_tags"
}
