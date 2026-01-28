// Package models provides data models for token market cache.
package models

import (
	"math/big"
	"time"

	"gorm.io/gorm"
)

// TokenMarketCache represents pre-computed token market data for fast queries.
// This table stores aggregated market data from CoinGecko and other sources
// to enable efficient sorting and filtering at the database level.
type TokenMarketCache struct {
	gorm.Model

	// Token identification
	TokenSymbol     string `json:"token_symbol" gorm:"column:token_symbol;size:50;not null;uniqueIndex:uk_token_symbol_chain"` // Token symbol (e.g., "INJ", "USDT")
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;index"`                           // Contract address for the token
	ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50;default:'injective-1';uniqueIndex:uk_token_symbol_chain"` // Chain ID

	// Market data (from CoinGecko)
	PriceUSD             float64 `json:"price_usd" gorm:"column:price_usd;type:decimal(30,18)"`   // Current price in USD
	PriceChange24h       float64 `json:"price_change_24h" gorm:"column:price_change_24h;type:decimal(10,4)"` // 24h price change percentage
	MarketCapUSD         float64 `json:"market_cap_usd" gorm:"column:market_cap_usd;type:decimal(40,2);index"` // Market cap in USD
	FullyDilutedMarketCap float64 `json:"fully_diluted_market_cap" gorm:"column:fully_diluted_market_cap;type:decimal(40,2)"` // Fully diluted market cap
	TotalVolume24h       float64 `json:"total_volume_24h" gorm:"column:total_volume_24h;type:decimal(40,2)"` // 24h trading volume in USD

	// Supply data
	HoldersCount       int64          `json:"holders_count" gorm:"column:holders_count"`                                // Number of holders
	TotalSupply        *big.Int        `json:"total_supply" gorm:"column:total_supply;type:decimal(78,0)"`                // Total supply (raw units)
	CirculatingSupply  *big.Int        `json:"circulating_supply" gorm:"column:circulating_supply;type:decimal(78,0)"`    // Circulating supply (raw units)
	MaxSupply          *big.Int        `json:"max_supply" gorm:"column:max_supply;type:decimal(78,0)"`                    // Max supply (raw units, nil if unlimited)

	// Metadata
	LastUpdated int64  `json:"last_updated" gorm:"column:last_updated"` // Unix timestamp of last data update
	DataSource  string `json:"data_source" gorm:"column:data_source;size:50;default:'coingecko'"` // Source of the data
}

// TableName returns the table name for TokenMarketCache model.
func (TokenMarketCache) TableName() string {
	return "token_market_cache"
}

// IsExpired checks if the cached data is expired (older than given duration).
func (t *TokenMarketCache) IsExpired(duration time.Duration) bool {
	return time.Now().Unix()-t.LastUpdated > int64(duration.Seconds())
}

// GetMarketCap returns the market cap for sorting (uses circulating market cap).
func (t *TokenMarketCache) GetMarketCap() float64 {
	if t.MarketCapUSD > 0 {
		return t.MarketCapUSD
	}
	return t.FullyDilutedMarketCap
}

// TokenMarketCacheQuery represents query parameters for token market cache.
type TokenMarketCacheQuery struct {
	ChainID      string
	MinMarketCap float64
	MaxMarketCap float64
	SortBy       string // "market_cap", "volume_24h", "price", "holders", "price_change"
	SortOrder    string // "asc", "desc"
	Page         int
	PageSize     int
}

// TokenMarketCacheUpdate represents fields that can be updated.
type TokenMarketCacheUpdate struct {
	PriceUSD              float64
	PriceChange24h        float64
	MarketCapUSD          float64
	FullyDilutedMarketCap float64
	TotalVolume24h        float64
	HoldersCount          int64
	TotalSupply           *big.Int
	CirculatingSupply     *big.Int
	MaxSupply             *big.Int
	LastUpdated           int64
	DataSource            string
}
