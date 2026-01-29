// Package service provides token market cache synchronization service.
package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TokenMarketCacheSyncService handles synchronization of token market data to cache table.
type TokenMarketCacheSyncService struct {
	ctx           context.Context
	cancel        context.CancelFunc
	config        *TokenMarketCacheSyncConfig
	wg            sync.WaitGroup
	syncMutex     sync.Mutex
	isRunning     bool
	refreshTicker *time.Ticker
}

// TokenMarketCacheSyncConfig holds configuration for the sync service.
type TokenMarketCacheSyncConfig struct {
	RefreshInterval    time.Duration // How often to refresh market data
	BatchSize          int           // Number of tokens to process in one batch
	MaxConcurrency     int           // Maximum concurrent API calls
	CacheTTL           time.Duration // How long cached data is considered valid
	EnabledTokens      []string      // List of token symbols to sync (empty = all)
}

// DefaultTokenMarketCacheSyncConfig returns default configuration.
func DefaultTokenMarketCacheSyncConfig() *TokenMarketCacheSyncConfig {
	return &TokenMarketCacheSyncConfig{
		RefreshInterval: 5 * time.Minute, // Refresh every 5 minutes
		BatchSize:       100,
		MaxConcurrency:  5,
		CacheTTL:        10 * time.Minute,
		EnabledTokens:   []string{}, // Empty means all tokens
	}
}

// LoadTokenMarketCacheSyncConfig loads configuration from bootstrap config.
func LoadTokenMarketCacheSyncConfig(bc *conf.Bootstrap) *TokenMarketCacheSyncConfig {
	config := DefaultTokenMarketCacheSyncConfig()

	// Load from environment variables
	if interval := os.Getenv("TOKEN_MARKET_CACHE_REFRESH_INTERVAL"); interval != "" {
		if d, err := time.ParseDuration(interval); err == nil {
			config.RefreshInterval = d
		}
	}

	if batchSize := os.Getenv("TOKEN_MARKET_CACHE_BATCH_SIZE"); batchSize != "" {
		if n, err := strconv.Atoi(batchSize); err == nil && n > 0 {
			config.BatchSize = n
		}
	}

	if ttl := os.Getenv("TOKEN_MARKET_CACHE_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil {
			config.CacheTTL = d
		}
	}

	// Load enabled tokens from environment
	if tokens := os.Getenv("TOKEN_MARKET_CACHE_TOKENS"); tokens != "" {
		config.EnabledTokens = strings.Split(tokens, ",")
	}

	return config
}

// NewTokenMarketCacheSyncService creates a new sync service instance.
func NewTokenMarketCacheSyncService(ctx context.Context, config *TokenMarketCacheSyncConfig) *TokenMarketCacheSyncService {
	serviceCtx, cancel := context.WithCancel(ctx)

	return &TokenMarketCacheSyncService{
		ctx:           serviceCtx,
		cancel:        cancel,
		config:        config,
		isRunning:     false,
		refreshTicker: time.NewTicker(config.RefreshInterval),
	}
}

// Start starts the market cache sync service.
func (s *TokenMarketCacheSyncService) Start() {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	if s.isRunning {
		global.Logger.Warn("Token market cache sync service is already running")
		return
	}

	s.isRunning = true

	// Do initial sync
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.syncAllTokens(); err != nil {
			global.Logger.Errorf("Initial token market cache sync failed: %v", err)
		}
	}()

	// Start periodic refresh worker
	s.wg.Add(1)
	go s.refreshWorker()

	global.Logger.Infof("Token market cache sync service started (refresh interval: %v)", s.config.RefreshInterval)
}

// Stop stops the sync service.
func (s *TokenMarketCacheSyncService) Stop() error {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	if !s.isRunning {
		return nil
	}

	s.cancel()

	if s.refreshTicker != nil {
		s.refreshTicker.Stop()
	}

	s.wg.Wait()
	s.isRunning = false

	global.Logger.Info("Token market cache sync service stopped")
	return nil
}

// refreshWorker periodically refreshes market data.
func (s *TokenMarketCacheSyncService) refreshWorker() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.refreshTicker.C:
			if err := s.syncAllTokens(); err != nil {
				global.Logger.Errorf("Token market cache sync failed: %v", err)
			}
		}
	}
}

// syncAllTokens synchronizes market data for all configured tokens.
func (s *TokenMarketCacheSyncService) syncAllTokens() error {
	startTime := time.Now()

	// Get list of tokens to sync
	tokens := s.getTokensToSync()
	if len(tokens) == 0 {
		global.Logger.Debug("No tokens configured for market cache sync")
		return nil
	}

	global.Logger.Infof("Starting token market cache sync for %d tokens", len(tokens))

	// Build token configs
	tokenConfigs := make(map[string]*TokenConfig)
	for _, symbol := range tokens {
		config := GetTokenConfigOrCreate(symbol)
		if config == nil {
			global.Logger.Warnf("Failed to create token config for %s: no CoinGecko ID mapping", symbol)
			continue
		}
		tokenConfigs[symbol] = config
	}

	if len(tokenConfigs) == 0 {
		return errors.New("no valid token configs found")
	}

	// Fetch market data using batch API
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	marketDataMap := BatchGetTokenMarketData(ctx, tokenConfigs)

	// Update cache for each token
	updatedCount := 0
	for symbol, data := range marketDataMap {
		if data == nil {
			continue
		}

		config := tokenConfigs[symbol]
		if config == nil {
			continue
		}

		if err := s.updateTokenCache(symbol, config, data); err != nil {
			global.Logger.Warnf("Failed to update cache for %s: %v", symbol, err)
			continue
		}

		updatedCount++
	}

	elapsed := time.Since(startTime)
	global.Logger.Infof("Token market cache sync completed: %d/%d tokens updated in %v",
		updatedCount, len(tokenConfigs), elapsed)

	return nil
}

// getTokensToSync returns the list of tokens to synchronize.
func (s *TokenMarketCacheSyncService) getTokensToSync() []string {
	// If specific tokens are configured, use those
	if len(s.config.EnabledTokens) > 0 {
		return s.config.EnabledTokens
	}

	// Otherwise, get all tokens from contract_infos table
	var tokens []string
	db.Get().Model(&models.ContractInfo{}).
		Where("token_symbol IS NOT NULL AND token_symbol != ''").
		Distinct("token_symbol").
		Pluck("token_symbol", &tokens)

	return tokens
}

// updateTokenCache updates the cache record for a single token.
func (s *TokenMarketCacheSyncService) updateTokenCache(symbol string, config *TokenConfig, data *TokenMarketData) error {
	// Get contract address from config or database
	contractAddress := ""
	if config.Denom != "" && len(config.Denom) > 3 {
		// Try to find contract address from database
		var contract models.ContractInfo
		err := db.Get().Model(&models.ContractInfo{}).
			Where("token_symbol = ?", symbol).
			First(&contract).Error
		if err == nil {
			contractAddress = contract.ContractAddress
		}
	}

	// Prepare cache record
	cache := &models.TokenMarketCache{
		TokenSymbol:           symbol,
		ContractAddress:       contractAddress,
		ChainID:               DefaultChainID,
		PriceUSD:              data.PriceUSD,
		PriceChange24h:        data.PriceChange24h,
		MarketCapUSD:          data.MarketCapUSD,
		FullyDilutedMarketCap: data.FullyDilutedMarketCap,
		TotalVolume24h:        data.TotalVolume24h,
		HoldersCount:          data.HoldersCount,
		LastUpdated:           time.Now().Unix(),
		DataSource:            "coingecko",
	}

	// Convert supply values
	if data.TotalSupply != nil {
		cache.TotalSupply = data.TotalSupply
	}
	if data.CirculatingSupply != nil {
		cache.CirculatingSupply = data.CirculatingSupply
	}
	if data.MaxSupply != nil {
		cache.MaxSupply = data.MaxSupply
	}

	// Upsert to database
	return db.Get().Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "token_symbol"}, {Name: "chain_id"}},
			UpdateAll: true,
		}).Create(cache).Error
	})
}

// GetTokenMarketFromCache retrieves token market data from cache.
func GetTokenMarketFromCache(ctx context.Context, tokenSymbol string) (*models.TokenMarketCache, error) {
	var cache models.TokenMarketCache
	err := db.Get().WithContext(ctx).
		Where("token_symbol = ? AND chain_id = ?", tokenSymbol, DefaultChainID).
		First(&cache).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Not found, not an error
		}
		return nil, err
	}

	return &cache, nil
}

// ListTokenMarketsFromCache retrieves a list of token market data from cache with filtering and sorting.
func ListTokenMarketsFromCache(ctx context.Context, query *models.TokenMarketCacheQuery) ([]*models.TokenMarketCache, int64, error) {
	dbQuery := db.Get().WithContext(ctx).Model(&models.TokenMarketCache{}).
		Where("deleted_at IS NULL")

	if query.ChainID != "" {
		dbQuery = dbQuery.Where("chain_id = ?", query.ChainID)
	}

	if query.MinMarketCap > 0 {
		dbQuery = dbQuery.Where("market_cap_usd >= ?", query.MinMarketCap)
	}

	if query.MaxMarketCap > 0 {
		dbQuery = dbQuery.Where("market_cap_usd <= ?", query.MaxMarketCap)
	}

	// Get total count
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply sorting
	sortField := "market_cap_usd"
	switch query.SortBy {
	case "volume_24h":
		sortField = "total_volume_24h"
	case "price":
		sortField = "price_usd"
	case "holders":
		sortField = "holders_count"
	case "price_change":
		sortField = "price_change_24h"
	}

	sortOrder := "DESC"
	if strings.ToLower(query.SortOrder) == "asc" {
		sortOrder = "ASC"
	}

	dbQuery = dbQuery.Order(sortField + " " + sortOrder)

	// Apply pagination
	if query.Page > 0 && query.PageSize > 0 {
		offset := (query.Page - 1) * query.PageSize
		dbQuery = dbQuery.Offset(offset).Limit(query.PageSize)
	}

	var results []*models.TokenMarketCache
	if err := dbQuery.Find(&results).Error; err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// InvalidateTokenMarketCache marks cache entries as expired (soft delete).
func InvalidateTokenMarketCache(ctx context.Context, tokenSymbol string) error {
	return db.Get().WithContext(ctx).
		Where("token_symbol = ?", tokenSymbol).
		Delete(&models.TokenMarketCache{}).Error
}

// TokenMarketCacheStats represents statistics about the cache.
type TokenMarketCacheStats struct {
	TotalTokens     int64     `json:"total_tokens"`
	LastUpdated     time.Time `json:"last_updated"`
	ExpiredEntries  int64     `json:"expired_entries"`
}

// GetTokenMarketCacheStats returns statistics about the cache.
func GetTokenMarketCacheStats(ctx context.Context) (*TokenMarketCacheStats, error) {
	var stats TokenMarketCacheStats

	// Get total count
	db.Get().WithContext(ctx).Model(&models.TokenMarketCache{}).
		Where("deleted_at IS NULL").
		Count(&stats.TotalTokens)

	// Get last updated
	var lastUpdated int64
	db.Get().WithContext(ctx).Model(&models.TokenMarketCache{}).
		Select("MAX(last_updated)").
		Scan(&lastUpdated)
	stats.LastUpdated = time.Unix(lastUpdated, 0)

	return &stats, nil
}
