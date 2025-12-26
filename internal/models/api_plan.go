// Package models provides data models for blockchain entities.
package models

import (
	"gorm.io/gorm"
)

// APIPlan represents an API plan configuration in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores API plan configurations including limits, rate limits,
// max tokens, webhook support, and pricing.
type APIPlan struct {
	gorm.Model

	// Name is the plan name (unique index)
	Name string `gorm:"column:name;type:varchar(50);not null;uniqueIndex" json:"name"`
	// PlanType is the plan type: free or paid (indexed for faster lookups)
	PlanType string `gorm:"column:plan_type;type:varchar(20);not null;index" json:"plan_type"`
	// MonthlyLimit is the monthly API call limit
	MonthlyLimit uint64 `gorm:"column:monthly_limit;type:bigint unsigned;not null" json:"monthly_limit"`
	// RateLimitPerSecond is the rate limit per second
	RateLimitPerSecond int `gorm:"column:rate_limit_per_second;type:int;not null" json:"rate_limit_per_second"`
	// MaxTokens is the maximum number of tokens allowed for this plan
	MaxTokens int `gorm:"column:max_tokens;type:int;not null" json:"max_tokens"`
	// EnableWebhook indicates whether webhook is enabled for this plan
	EnableWebhook bool `gorm:"column:enable_webhook;type:tinyint(1);not null;default:0" json:"enable_webhook"`
	// Price is the plan price
	Price float64 `gorm:"column:price;type:decimal(10,2);not null;default:0.00" json:"price"`
	// Status is the plan status: active or inactive (indexed for faster lookups)
	Status string `gorm:"column:status;type:varchar(20);not null;default:'active';index" json:"status"`
}

// TableName specifies the table name for APIPlan.
func (APIPlan) TableName() string {
	return "api_plans"
}
