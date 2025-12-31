// Package models provides data models for blockchain entities.
package models

import (
	"time"

	"gorm.io/gorm"
)

// APIToken represents an API token in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores API token information including the token hash (for validation),
// token preview (for display), plan type, and status.
type APIToken struct {
	gorm.Model

	// UserID is the user ID from external system
	UserID string `gorm:"column:user_id;type:varchar(64);not null" json:"user_id"`
	// Name is the token name
	Name string `gorm:"column:name;type:varchar(50);not null" json:"name"`
	// TokenHash is the SHA256 hash of the token (unique index for validation)
	TokenHash string `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex" json:"token_hash"`
	// TokenPreview is the token preview format (inj_xxx...xxx) for display
	TokenPreview string `gorm:"column:token_preview;type:varchar(20);not null" json:"token_preview"`
	// PlanType is the plan type: free or paid
	PlanType string `gorm:"column:plan_type;type:varchar(20);not null;default:'free'" json:"plan_type"`
	// Status is the token status: active or disabled
	Status string `gorm:"column:status;type:varchar(20);not null;default:'active'" json:"status"`
	// LastUsedAt is the last time the token was used (nullable)
	LastUsedAt *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
}

// TableName specifies the table name for APIToken.
func (APIToken) TableName() string {
	return "api_tokens"
}
