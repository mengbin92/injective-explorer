// Package models provides data models for on-chain price data.
package models

import (
	"time"
)

// OnChainPrice represents an on-chain price record.
type OnChainPrice struct {
	ID          uint64    `gorm:"primaryKey"`
	Symbol      string    `gorm:"index:idx_symbol_time"`
	Denom       string    `gorm:"index"`
	MarketID    string    `gorm:"index"`
	PriceUSD    float64   `gorm:"type:decimal(32,18)"`
	PriceRaw    string    `gorm:"type:varchar(100)"` // Raw price (18 decimals)
	BestBuy     string    `gorm:"type:varchar(100)"`
	BestSell    string    `gorm:"type:varchar(100)"`
	Source      string    // "spot_market" / "derivative_market"
	Timestamp   int64     `gorm:"index:idx_symbol_time"`
	BlockHeight int64
	Confidence  float64   `gorm:"type:decimal(5,4)"`
	CreatedAt   time.Time
}

// TableName returns the table name.
func (OnChainPrice) TableName() string {
	return "onchain_prices"
}

// TokenMarketMapping represents token to market mapping.
type TokenMarketMapping struct {
	ID           uint64 `gorm:"primaryKey"`
	Symbol       string `gorm:"index"`
	Denom        string `gorm:"uniqueIndex"`
	SpotMarketID string
	IsActive     bool
	Priority     int
	LastUpdated  time.Time
}

// TableName returns the table name.
func (TokenMarketMapping) TableName() string {
	return "token_market_mappings"
}

// PriceInfo represents price information (used in memory).
type PriceInfo struct {
	Symbol      string
	Denom       string
	PriceUSD    float64
	PriceRaw    string  // Raw price string (18 decimals)
	BestBuy     string  // Best buy price
	BestSell    string  // Best sell price
	Source      string  // "spot_market" / "derivative_market"
	MarketID    string
	Timestamp   int64
	BlockHeight int64
	Confidence  float64 // Confidence level 0-1
}

// PriceCacheData represents cache data structure.
type PriceCacheData struct {
	Price       float64 `json:"price"`
	PriceRaw    string  `json:"price_raw"`
	BestBuy     string  `json:"best_buy"`
	BestSell    string  `json:"best_sell"`
	Source      string  `json:"source"`
	MarketID    string  `json:"market_id"`
	Timestamp   int64   `json:"timestamp"`
	BlockHeight int64   `json:"block_height"`
	Confidence  float64 `json:"confidence"`
}

// QuoteTokenConfig represents quote token configuration.
type QuoteTokenConfig struct {
	USDTAddress    string
	USDCAddress    string
	QuotePriority []string
}

// DefaultQuoteTokens is the default quote token configuration.
var DefaultQuoteTokens = QuoteTokenConfig{
	USDTAddress: "peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5",
	USDCAddress: "peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
	QuotePriority: []string{
		"peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5",
		"peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
	},
}
