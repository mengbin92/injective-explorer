// Package models provides data models for price alerts.
package models

import (
	"time"

	"gorm.io/gorm"
)

// PriceAlertType 价格预警类型
type PriceAlertType string

const (
	// PriceAlertTypeAbove 价格上涨超过阈值
	PriceAlertTypeAbove PriceAlertType = "above"
	// PriceAlertTypeBelow 价格下跌低于阈值
	PriceAlertTypeBelow PriceAlertType = "below"
	// PriceAlertTypeChangeUp 价格涨幅超过百分比
	PriceAlertTypeChangeUp PriceAlertType = "change_up"
	// PriceAlertTypeChangeDown 价格跌幅超过百分比
	PriceAlertTypeChangeDown PriceAlertType = "change_down"
)

// PriceAlertStatus 预警状态
type PriceAlertStatus string

const (
	// PriceAlertStatusActive 活跃状态
	PriceAlertStatusActive PriceAlertStatus = "active"
	// PriceAlertStatusTriggered 已触发
	PriceAlertStatusTriggered PriceAlertStatus = "triggered"
	// PriceAlertStatusDisabled 已禁用
	PriceAlertStatusDisabled PriceAlertStatus = "disabled"
)

// PriceAlert 价格预警配置
type PriceAlert struct {
	gorm.Model

	// 基本信息
	Symbol   string           `gorm:"index:idx_symbol_status" json:"symbol"`
	Denom    string           `gorm:"index" json:"denom"`
	AlertType PriceAlertType  `gorm:"index" json:"alert_type"`
	Status   PriceAlertStatus `gorm:"index:idx_symbol_status" json:"status"`

	// 阈值配置
	ThresholdPrice   float64 `gorm:"type:decimal(30,18)" json:"threshold_price"`   // 价格阈值
	ThresholdPercent float64 `gorm:"type:decimal(10,4)" json:"threshold_percent"`   // 百分比阈值
	ReferencePrice   float64 `gorm:"type:decimal(30,18)" json:"reference_price"`   // 参考价格（用于计算涨跌幅）

	// 通知配置
	NotificationChannels []string `gorm:"serializer:json" json:"notification_channels"` // webhook, email, slack
	WebhookURL          string   `gorm:"size:512" json:"webhook_url"`
	Email              string   `gorm:"size:256" json:"email"`

	// 触发限制
	CooldownMinutes int `json:"cooldown_minutes"` // 冷却时间（分钟）
	MaxTriggers     int `json:"max_triggers"`     // 最大触发次数

	// 触发统计
	TriggerCount  int       `json:"trigger_count"`
	LastTriggered time.Time `json:"last_triggered"`
	LastPrice     float64   `gorm:"type:decimal(30,18)" json:"last_price"`

	// 元数据
	Description string `gorm:"size:500" json:"description"`
	CreatedBy   string `gorm:"size:128" json:"created_by"`
}

// TableName 返回表名
func (PriceAlert) TableName() string {
	return "price_alerts"
}

// IsCooldownActive 检查是否处于冷却期
func (a *PriceAlert) IsCooldownActive() bool {
	if a.LastTriggered.IsZero() || a.CooldownMinutes <= 0 {
		return false
	}
	return time.Since(a.LastTriggered) < time.Duration(a.CooldownMinutes)*time.Minute
}

// CanTrigger 检查是否可以触发
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

// ShouldTrigger 检查是否应该触发预警
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

// RecordTrigger 记录触发
func (a *PriceAlert) RecordTrigger(price float64) {
	a.TriggerCount++
	a.LastTriggered = time.Now()
	a.LastPrice = price
}

// PriceAlertHistory 预警触发历史
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

// TableName 返回表名
func (PriceAlertHistory) TableName() string {
	return "price_alert_history"
}

// AggregatedPriceData 聚合价格数据（K线）
type AggregatedPriceData struct {
	gorm.Model

	Symbol   string `gorm:"index:idx_symbol_interval_time" json:"symbol"`
	Interval string `gorm:"index:idx_symbol_interval_time" json:"interval"` // 1m, 5m, 15m, 1h, 4h, 1d

	// 时间戳（对齐到区间边界）
	Timestamp int64 `gorm:"index:idx_symbol_interval_time" json:"timestamp"`

	// OHLCV 数据
	Open   float64 `gorm:"type:decimal(30,18)" json:"open"`
	High   float64 `gorm:"type:decimal(30,18)" json:"high"`
	Low    float64 `gorm:"type:decimal(30,18)" json:"low"`
	Close  float64 `gorm:"type:decimal(30,18)" json:"close"`
	Volume float64 `gorm:"type:decimal(40,8)" json:"volume"`

	// 额外统计
	TradeCount int     `json:"trade_count"`
	AvgPrice   float64 `gorm:"type:decimal(30,18)" json:"avg_price"`

	// 数据源
	DataSource string `json:"data_source"` // onchain, aggregated
}

// TableName 返回表名
func (AggregatedPriceData) TableName() string {
	return "aggregated_price_data"
}

// IsComplete 检查该时间段数据是否完整
func (d *AggregatedPriceData) IsComplete() bool {
	return d.High >= d.Low && d.High > 0 && d.Low > 0
}

// GetPriceChange 获取价格变化
func (d *AggregatedPriceData) GetPriceChange() float64 {
	if d.Open == 0 {
		return 0
	}
	return (d.Close - d.Open) / d.Open * 100
}

// PriceAlertQuery 预警查询参数
type PriceAlertQuery struct {
	Symbol   string
	Status   PriceAlertStatus
	AlertType PriceAlertType
	Page     int
	PageSize int
}

// AggregatedPriceQuery 聚合价格查询参数
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
