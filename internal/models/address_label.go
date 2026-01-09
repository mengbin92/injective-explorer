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
type AddressLabel struct {
	gorm.Model

	Address     string `json:"address" gorm:"column:address;size:128;not null"`
	ChainID     string `json:"chain_id" gorm:"column:chain_id;size:50;not null"`
	LabelType   string `json:"label_type" gorm:"column:label_type;size:50;not null"`
	LabelValue  string `json:"label_value" gorm:"column:label_value;size:255"`
	LabelColor  string `json:"label_color" gorm:"column:label_color;size:20"`
	Description string `json:"description" gorm:"column:description;type:text"`
	CreatedBy   string `json:"created_by" gorm:"column:created_by;size:128"`
}

// TableName specifies the table name for AddressLabel.
func (AddressLabel) TableName() string {
	return "address_labels"
}
