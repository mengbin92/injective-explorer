// Package models provides data models for blockchain entities.
package models

import (
	"time"

	"gorm.io/gorm"
)

// Webhook represents a webhook configuration in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores webhook configuration including URL, events, and status.
// This is reserved for future implementation.
type Webhook struct {
	gorm.Model

	// TokenID is the API token ID (must be paid plan, indexed for faster lookups)
	TokenID uint `gorm:"column:token_id;type:bigint unsigned;not null;index" json:"token_id"`
	// WebhookURL is the webhook callback URL
	WebhookURL string `gorm:"column:webhook_url;type:varchar(512);not null" json:"webhook_url"`
	// Secret is the webhook signing secret for payload verification
	Secret string `gorm:"column:secret;type:varchar(64);not null" json:"secret"`
	// Events is the list of subscribed event types (stored as JSON)
	Events []string `gorm:"column:events;serializer:json" json:"events"`
	// Status is the webhook status (active/inactive, indexed for faster lookups)
	Status string `gorm:"column:status;type:varchar(20);not null;default:'active';index" json:"status"`
	// LastTriggeredAt is the last time the webhook was triggered
	LastTriggeredAt *time.Time `gorm:"column:last_triggered_at" json:"last_triggered_at"`
	// FailureCount is the number of consecutive failures
	FailureCount int `gorm:"column:failure_count;type:int;not null;default:0" json:"failure_count"`
}

// TableName specifies the table name for Webhook.
func (Webhook) TableName() string {
	return "webhooks"
}
