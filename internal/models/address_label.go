// Package models provides data models for blockchain entities.
package models

import (
	"gorm.io/gorm"
)

// AddressLabel represents an address label in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores label information for addresses, supporting multiple labels per address.
// Labels can be used to categorize and identify addresses (e.g., exchange, project, personal, scam).
//
// Important: The unique constraint is based on (chain_id, address, label_value), not label_type.
// This means one address can have multiple labels with different label_value, regardless of label_type.
type AddressLabel struct {
	gorm.Model

	Address     string `json:"address" gorm:"column:address;size:128;not null;index:idx_address,priority:1"`
	ChainID     string `json:"chain_id" gorm:"column:chain_id;size:50;not null;index:idx_address,priority:2"`
	LabelType   string `json:"label_type" gorm:"column:label_type;size:50;not null"`
	LabelValue  string `json:"label_value" gorm:"column:label_value;size:255;not null;uniqueIndex:uk_address_label_value,priority:3;index:idx_label_value"`
	LabelColor  string `json:"label_color" gorm:"column:label_color;size:20"`
	Description string `json:"description" gorm:"column:description;type:text"`
	Visible     bool   `json:"visible" gorm:"column:visible;default:1;index:idx_visible"` // Whether to display in frontend
	CreatedBy   string `json:"created_by" gorm:"column:created_by;size:128"`
}

// TableName specifies the table name for AddressLabel.
func (AddressLabel) TableName() string {
	return "address_labels"
}
