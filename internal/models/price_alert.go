// Package models provides data models for price alerts.
package models

import (
	"time"

	"gorm.io/gorm"
)

// PriceAlertType represents the type of price alert.
type PriceAlertType string

const (
	// PriceAlertTypeAbove indicates price rising above threshold.
	PriceAlertTypeAbove PriceAlertType = "above"
	// PriceAlertTypeBelow indicates price falling below threshold.
	PriceAlertTypeBelow PriceAlertType = "below"
	// PriceAlertTypeChangeUp indicates price increase exceeding percentage threshold.
	PriceAlertTypeChangeUp PriceAlertType = "change_up"
	// PriceAlertTypeChangeDown indicates price decrease exceeding percentage threshold.
	PriceAlertTypeChangeDown PriceAlertType = "change_down"
)

// PriceAlertStatus represents the status of a price alert.
type PriceAlertStatus string

const (
	// PriceAlertStatusActive indicates the alert is active.
	PriceAlertStatusActive PriceAlertStatus = "active"
	// PriceAlertStatusTriggered indicates the alert has been triggered.
	PriceAlertStatusTriggered PriceAlertStatus = "triggered"
	// PriceAlertStatusDisabled indicates the alert is disabled.
	PriceAlertStatusDisabled PriceAlertStatus = "disabled"
)

// PriceAlert represents a price alert configuration.
type PriceAlert struct {
	gorm.Model

	// Basic information
	Symbol   string           `gorm:"index:idx_symbol_status" json:"symbol"`
	Denom    string           `gorm:"index" json:"denom"`
	AlertType PriceAlertType  `gorm:"index" json:"alert_type"`
	Status   PriceAlertStatus `gorm:"index:idx_symbol_status" json:"status"`

	// Threshold configuration
	ThresholdPrice   float64 `gorm:"type:decimal(30,18)" json:"threshold_price"`   // Price threshold
	ThresholdPercent float64 `gorm:"type:decimal(10,4)" json:"threshold_percent"`   // Percentage threshold
	ReferencePrice   float64 `gorm:"type:decimal(30,18)" json:"reference_price"`   // Reference price for calculating change

	// Notification configuration
	NotificationChannels []string `gorm:"serializer:json" json:"notification_channels"` // webhook, email, slack
	WebhookURL          string   `gorm:"size:512" json:"webhook_url"`
	Email              string   `gorm:"size:256" json:"email"`

	// Trigger limits
	CooldownMinutes int `json:"cooldown_minutes"` // Cooldown period in minutes
	MaxTriggers     int `json:"max_triggers"`     // Maximum number of triggers

	// Trigger statistics
	TriggerCount  int       `json:"trigger_count"`
	LastTriggered time.Time `json:"last_triggered"`
	LastPrice     float64   `gorm:"type:decimal(30,18)" json:"last_price"`

	// Metadata
	Description string `gorm:"size:500" json:"description"`
	CreatedBy   string `gorm:"size:128" json:"created_by"`
}

// TableName returns the table name.
func (PriceAlert) TableName() string {
	return "price_alerts"
}

// IsCooldownActive checks if the alert is in cooldown period.
func (a *PriceAlert) IsCooldownActive() bool {
	if a.LastTriggered.IsZero() || a.CooldownMinutes <= 0 {
		return false
	}
	return time.Since(a.LastTriggered) < time.Duration(a.CooldownMinutes)*time.Minute
}

// CanTrigger checks if the alert can be triggered.
func (a *PriceAlert) CanTrigger() bool {
	if a.Status != PriceAlertStatusActive {
		return false
	}
	if a.IsCooldownActive() {
		return false
	}
	if a.MaxTriggers > 0 && a.TriggerCount >= a.MaxTriggers {
		return false
	}
	return true
}

// ShouldTrigger checks if the alert should be triggered based on current price.
func (a *PriceAlert) ShouldTrigger(currentPrice float64) bool {
	if !a.CanTrigger() {
		return false
	}

	switch a.AlertType {
	case PriceAlertTypeAbove:
		return currentPrice >= a.ThresholdPrice
	case PriceAlertTypeBelow:
		return currentPrice <= a.ThresholdPrice
	case PriceAlertTypeChangeUp:
		if a.ReferencePrice == 0 {
			return false
		}
		change := (currentPrice - a.ReferencePrice) / a.ReferencePrice * 100
		return change >= a.ThresholdPercent
	case PriceAlertTypeChangeDown:
		if a.ReferencePrice == 0 {
			return false
		}
		change := (a.ReferencePrice - currentPrice) / a.ReferencePrice * 100
		return change >= a.ThresholdPercent
	}

	return false
}

// RecordTrigger records a trigger event.
func (a *PriceAlert) RecordTrigger(price float64) {
	a.TriggerCount++
	a.LastTriggered = time.Now()
	a.LastPrice = price
}

// PriceAlertHistory represents the trigger history of price alerts.
type PriceAlertHistory struct {
	gorm.Model

	AlertID   uint    `gorm:"index" json:"alert_id"`
	Symbol    string  `gorm:"index" json:"symbol"`
	AlertType string  `json:"alert_type"`

	TriggerPrice     float64 `gorm:"type:decimal(30,18)" json:"trigger_price"`
	ReferencePrice   float64 `gorm:"type:decimal(30,18)" json:"reference_price"`
	ChangePercent    float64 `gorm:"type:decimal(10,4)" json:"change_percent"`

	NotificationSent bool   `json:"notification_sent"`
	NotificationError string `gorm:"size:500" json:"notification_error"`
}

// TableName returns the table name.
func (PriceAlertHistory) TableName() string {
	return "price_alert_history"
}

// AggregatedPriceData represents aggregated price data (candlestick/K-line).
type AggregatedPriceData struct {
	gorm.Model

	Symbol   string `gorm:"index:idx_symbol_interval_time" json:"symbol"`
	Interval string `gorm:"index:idx_symbol_interval_time" json:"interval"` // 1m, 5m, 15m, 1h, 4h, 1d

	// Timestamp aligned to interval boundary
	Timestamp int64 `gorm:"index:idx_symbol_interval_time" json:"timestamp"`

	// OHLCV data
	Open   float64 `gorm:"type:decimal(30,18)" json:"open"`
	High   float64 `gorm:"type:decimal(30,18)" json:"high"`
	Low    float64 `gorm:"type:decimal(30,18)" json:"low"`
	Close  float64 `gorm:"type:decimal(30,18)" json:"close"`
	Volume float64 `gorm:"type:decimal(40,8)" json:"volume"`

	// Additional statistics
	TradeCount int     `json:"trade_count"`
	AvgPrice   float64 `gorm:"type:decimal(30,18)" json:"avg_price"`

	// Data source
	DataSource string `json:"data_source"` // onchain, aggregated
}

// TableName returns the table name.
func (AggregatedPriceData) TableName() string {
	return "aggregated_price_data"
}

// IsComplete checks if the data for this time period is complete.
func (d *AggregatedPriceData) IsComplete() bool {
	return d.High >= d.Low && d.High > 0 && d.Low > 0
}

// GetPriceChange returns the price change percentage.
func (d *AggregatedPriceData) GetPriceChange() float64 {
	if d.Open == 0 {
		return 0
	}
	return (d.Close - d.Open) / d.Open * 100
}

// PriceAlertQuery represents query parameters for price alerts.
type PriceAlertQuery struct {
	Symbol   string
	Status   PriceAlertStatus
	AlertType PriceAlertType
	Page     int
	PageSize int
}

// AggregatedPriceQuery represents query parameters for aggregated price data.
type AggregatedPriceQuery struct {
	Symbol    string
	Interval  string
	StartTime int64
	EndTime   int64
	Limit     int
}

// Common intervals
const (
	Interval1Minute  = "1m"
	Interval5Minutes = "5m"
	Interval15Minutes = "15m"
	Interval1Hour    = "1h"
	Interval4Hours   = "4h"
	Interval1Day     = "1d"
	Interval1Week    = "1w"
)
