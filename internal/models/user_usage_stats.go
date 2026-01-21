// Package models provides data models for blockchain entities.
package models

import (
	"time"

	"gorm.io/gorm"
)

// UserUsageStats represents user usage statistics in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores user API usage statistics including monthly usage, limits,
// and success/failure counts.
type UserUsageStats struct {
	gorm.Model

	// UserID is the user ID (unique index for faster lookups)
	UserID string `gorm:"column:user_id;type:varchar(64);not null;uniqueIndex" json:"user_id"`
	// MonthlyUsed is the number of API calls used this month
	MonthlyUsed uint64 `gorm:"column:monthly_used;type:bigint unsigned;not null" json:"monthly_used"`
	// MonthlyLimit is the monthly API call limit
	MonthlyLimit uint64 `gorm:"column:monthly_limit;type:bigint unsigned;not null" json:"monthly_limit"`
	// TotalUsage is the total number of API calls made historically
	TotalUsage uint64 `gorm:"column:total_usage;type:bigint unsigned;not null" json:"total_usage"`
	// SuccessCount is the number of successful API calls
	SuccessCount uint64 `gorm:"column:success_count;type:bigint unsigned;not null" json:"success_count"`
	// FailedCount is the number of failed API calls
	FailedCount uint64 `gorm:"column:failed_count;type:bigint unsigned;not null" json:"failed_count"`
	// LastResetAt is the last time the monthly usage was reset
	LastResetAt time.Time `gorm:"column:last_reset_at;not null" json:"last_reset_at"`
}

// TableName specifies the table name for UserUsageStats.
func (UserUsageStats) TableName() string {
	return "user_usage_stats"
}

