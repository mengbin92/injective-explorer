// Package models provides data models for relayer records.
package models

import "time"

// Relayer represents relayer information in the database.
// It stores cross-chain relayer statistics, performance metrics,
// and active channel information for IBC or other cross-chain protocols.
type Relayer struct {
	ID uint64 `json:"id" gorm:"column:id;primaryKey"`

	// Relayer identification
	MarketID       string `json:"market_id" gorm:"column:market_id;size:128;not null;uniqueIndex:uk_market_relayer,priority:1;index:idx_relayers_market_id"`
	RelayerAddress string `json:"relayer_address" gorm:"column:relayer_address;size:128;not null;uniqueIndex:uk_market_relayer,priority:2;index:idx_relayers_relayer_address"`

	// Performance statistics
	TotalPacketsRelayed int64   `json:"total_packets_relayed" gorm:"column:total_packets_relayed;default:0"`
	TotalPacketsFailed  int64   `json:"total_packets_failed" gorm:"column:total_packets_failed;default:0"`
	SuccessRate         float64 `json:"success_rate" gorm:"column:success_rate;type:decimal(5,2);default:0.00"`
	AvgRelayTimeMs      int64   `json:"avg_relay_time_ms" gorm:"column:avg_relay_time_ms;default:0"`

	// Fees and channels (JSON format)
	TotalFeesEarned string `json:"total_fees_earned" gorm:"column:total_fees_earned;type:text"` // JSON array of coins
	ActiveChannels  string `json:"active_channels" gorm:"column:active_channels;type:text"`     // JSON array of channel IDs

	// Timestamps
	LastActiveAt int64      `json:"last_active_at" gorm:"column:last_active_at;index:idx_relayers_last_active,sort:desc"`
	CreatedAt    *time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    *time.Time `json:"updated_at" gorm:"column:updated_at"`
}

// TableName returns the table name for Relayer model.
func (Relayer) TableName() string {
	return "relayers"
}
