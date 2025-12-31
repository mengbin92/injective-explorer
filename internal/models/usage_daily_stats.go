// Package models provides data models for blockchain entities.
package models

import (
	"time"

	"gorm.io/gorm"
)

// UsageDailyStats represents daily usage statistics in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores daily aggregated usage statistics for dashboard display,
// avoiding real-time aggregation queries.
type UsageDailyStats struct {
	gorm.Model

	// UserID is the user ID (part of unique index with StatDate)
	UserID string `gorm:"column:user_id;type:varchar(64);not null;uniqueIndex:uk_user_date" json:"user_id"`
	// StatDate is the statistics date (part of unique index with UserID)
	StatDate time.Time `gorm:"column:stat_date;type:date;not null;uniqueIndex:uk_user_date" json:"stat_date"`
	// TotalCalls is the total number of API calls on this date
	TotalCalls uint64 `gorm:"column:total_calls;type:bigint unsigned;not null;default:0" json:"total_calls"`
	// SuccessCalls is the number of successful API calls on this date
	SuccessCalls uint64 `gorm:"column:success_calls;type:bigint unsigned;not null;default:0" json:"success_calls"`
	// FailedCalls is the number of failed API calls on this date
	FailedCalls uint64 `gorm:"column:failed_calls;type:bigint unsigned;not null;default:0" json:"failed_calls"`
}

// TableName specifies the table name for UsageDailyStats.
func (UsageDailyStats) TableName() string {
	return "usage_daily_stats"
}

