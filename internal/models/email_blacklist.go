// Package models provides data models for blockchain entities.
package models

import (
	"time"
)

// EmailBlacklist represents an email domain in the blacklist.
type EmailBlacklist struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`

	// Domain is the email domain (lowercase, without @)
	Domain string `gorm:"column:domain;type:varchar(255);not null;uniqueIndex" json:"domain"`
	// Status is the status: active/disabled
	Status string `gorm:"column:status;type:varchar(20);not null" json:"status"`
	// BlockCount is the number of times this domain was blocked
	BlockCount uint64 `gorm:"column:block_count;type:bigint unsigned;not null" json:"block_count"`
	// Note is the note/description
	Note string `gorm:"column:note;type:text" json:"note"`
}

// TableName specifies the table name for EmailBlacklist.
func (EmailBlacklist) TableName() string {
	return "email_blacklist"
}

