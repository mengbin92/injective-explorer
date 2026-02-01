// Package service provides on-chain price synchronization and aggregation functionality.
package service

import (
	"context"
	"sync"
	"time"

	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// defaultNativeInterval is the sync interval for native tokens.
	defaultNativeInterval = 30 * time.Second
	// defaultAllMarketsInterval is the sync interval for all markets.
	defaultAllMarketsInterval = 60 * time.Second
	// defaultBatchSize is the batch processing size.
	defaultBatchSize = 50
	// priceSyncLockKey is the distributed lock key.
	priceSyncLockKey = "onchain_price_sync:lock"
)

// OnChainPriceSyncer syncs token prices from on-chain markets to the database.
type OnChainPriceSyncer struct {
	provider *OnChainPriceProvider
	db       *gorm.DB
	logger   *log.Helper

	// Configuration
	nativeInterval     time.Duration
	allMarketsInterval time.Duration
	batchSize          int
	enabled            bool

	// Native tokens (priority sync)
	nativeTokens []NativeTokenConfig

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Distributed lock
	syncLockValue string
}

// NativeTokenConfig represents a native token configuration.
type NativeTokenConfig struct {
	Symbol   string
	Denom    string
	Priority int
}

// NewOnChainPriceSyncer creates a new price syncer.
func NewOnChainPriceSyncer(
	provider *OnChainPriceProvider,
	logger *log.Helper,
) *OnChainPriceSyncer {
	return NewOnChainPriceSyncerWithDB(provider, nil, logger)
}

// NewOnChainPriceSyncerWithDB creates a price syncer with the specified database.
func NewOnChainPriceSyncerWithDB(
	provider *OnChainPriceProvider,
	dbInstance *gorm.DB,
	logger *log.Helper,
) *OnChainPriceSyncer {
	if logger == nil {
		logger = log.NewHelper(log.NewStdLogger(nil))
	}

	// If no database instance is provided, try to get the global instance
	if dbInstance == nil {
		// Lazily get database connection to avoid panic during initialization
		// Ensure db.Init() is called before actual use
		dbInstance = getDBSafely()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &OnChainPriceSyncer{
		provider:           provider,
		db:                 dbInstance,
		logger:             logger,
		nativeInterval:     defaultNativeInterval,
		allMarketsInterval: defaultAllMarketsInterval,
		batchSize:          defaultBatchSize,
		enabled:            true,
		nativeTokens: []NativeTokenConfig{
			{Symbol: "INJ", Denom: "inj", Priority: 1},
		},
		ctx:           ctx,
		cancel:        cancel,
		syncLockValue: generateLockValue(),
	}
}

// getDBSafely safely retrieves the database instance.
func getDBSafely() *gorm.DB {
	defer func() {
		if r := recover(); r != nil {
			// Database not initialized, return nil
		}
	}()
	return db.Get()
}

// NewOnChainPriceSyncerWithSharedClient creates a syncer using the shared chain client.
func NewOnChainPriceSyncerWithSharedClient(logger *log.Helper) (*OnChainPriceSyncer, error) {
	provider, err := NewOnChainPriceProviderWithSharedClient(logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create price provider")
	}

	return NewOnChainPriceSyncer(provider, logger), nil
}

// SetNativeTokens sets the native token list.
func (s *OnChainPriceSyncer) SetNativeTokens(tokens []NativeTokenConfig) {
	s.nativeTokens = tokens
}

// SetIntervals sets the sync intervals.
func (s *OnChainPriceSyncer) SetIntervals(nativeInterval, allMarketsInterval time.Duration) {
	s.nativeInterval = nativeInterval
	s.allMarketsInterval = allMarketsInterval
}

// Start starts the sync service.
func (s *OnChainPriceSyncer) Start(ctx context.Context) error {
	if !s.enabled {
		s.logger.Info("on-chain price sync is disabled")
		return nil
	}

	// Try to acquire distributed lock
	if !s.acquireLock(ctx) {
		s.logger.Info("another instance is running price sync, skipping")
		return nil
	}

	s.logger.Infow("starting on-chain price sync service",
		"native_interval", s.nativeInterval,
		"all_markets_interval", s.allMarketsInterval,
	)

	// 1. High-frequency sync (native tokens)
	s.wg.Add(1)
	go s.startNativeTokenSync(s.ctx, s.nativeInterval)

	// 2. Medium-frequency sync (all markets)
	s.wg.Add(1)
	go s.startAllMarketsSync(s.ctx, s.allMarketsInterval)

	return nil
}

// Stop stops the sync service.
func (s *OnChainPriceSyncer) Stop() error {
	s.logger.Info("stopping on-chain price sync service")
	s.cancel()

	// Release distributed lock
	s.releaseLock()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("on-chain price sync service stopped gracefully")
	case <-time.After(10 * time.Second):
		s.logger.Warn("on-chain price sync service stop timeout")
	}

	return nil
}

// startNativeTokenSync starts the native token price sync loop.
func (s *OnChainPriceSyncer) startNativeTokenSync(
	ctx context.Context,
	interval time.Duration,
) {
	defer s.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Execute immediately on start
	s.syncNativeTokens(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("native token sync stopped")
			return
		case <-ticker.C:
			if !s.renewLock() {
				s.logger.Warn("lost distributed lock, stopping native token sync")
				return
			}
			s.syncNativeTokens(ctx)
		}
	}
}

// syncNativeTokens syncs native token prices.
func (s *OnChainPriceSyncer) syncNativeTokens(ctx context.Context) {
	s.logger.Debug("syncing native token prices")

	for _, token := range s.nativeTokens {
		price, err := s.provider.GetNativeTokenPrice(ctx, token.Denom)
		if err != nil {
			s.logger.Warnw("failed to sync native token price",
				"symbol", token.Symbol,
				"denom", token.Denom,
				"error", err,
			)
			continue
		}

		// Save to database
		if err := s.savePrice(ctx, price); err != nil {
			s.logger.Warnw("failed to save price",
				"symbol", token.Symbol,
				"error", err,
			)
			continue
		}

		s.logger.Debugw("synced native token price",
			"symbol", token.Symbol,
			"price", price.PriceUSD,
			"source", price.Source,
		)
	}
}

// startAllMarketsSync starts the all markets sync loop.
func (s *OnChainPriceSyncer) startAllMarketsSync(
	ctx context.Context,
	interval time.Duration,
) {
	defer s.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("all markets sync stopped")
			return
		case <-ticker.C:
			if !s.renewLock() {
				s.logger.Warn("lost distributed lock, stopping all markets sync")
				return
			}
			s.syncAllMarkets(ctx)
		}
	}
}

// syncAllMarkets syncs prices for all markets.
func (s *OnChainPriceSyncer) syncAllMarkets(ctx context.Context) {
	s.logger.Debug("syncing all market prices")

	// Fetch all markets list
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	chainClient, err := GetSharedChainClient()
	if err != nil {
		s.logger.Warnw("failed to get chain client", "error", err)
		return
	}

	marketsResp, err := chainClient.FetchChainSpotMarkets(queryCtx, "", nil)
	if err != nil {
		s.logger.Warnw("failed to fetch markets", "error", err)
		return
	}

	s.logger.Debugw("fetched markets", "count", len(marketsResp.Markets))

	// Process in batches
	for i := 0; i < len(marketsResp.Markets); i += s.batchSize {
		end := i + s.batchSize
		if end > len(marketsResp.Markets) {
			end = len(marketsResp.Markets)
		}

		batch := marketsResp.Markets[i:end]
		s.syncMarketBatch(ctx, batch)
	}
}

// syncMarketBatch syncs a batch of markets.
func (s *OnChainPriceSyncer) syncMarketBatch(
	ctx context.Context,
	markets interface{},
) {
	// Use reflection or type assertion to handle market list
	// Simplified here, actual implementation depends on SDK return type
}

// savePrice saves price to database using UPSERT.
func (s *OnChainPriceSyncer) savePrice(
	ctx context.Context,
	price *models.PriceInfo,
) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	onChainPrice := models.OnChainPrice{
		Symbol:      price.Symbol,
		Denom:       price.Denom,
		MarketID:    price.MarketID,
		PriceUSD:    price.PriceUSD,
		PriceRaw:    price.PriceRaw,
		BestBuy:     price.BestBuy,
		BestSell:    price.BestSell,
		Source:      price.Source,
		Timestamp:   price.Timestamp,
		BlockHeight: price.BlockHeight,
		Confidence:  price.Confidence,
	}

	// Use UPSERT to avoid duplicate records
	result := s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "symbol"}, {Name: "timestamp"}},
			DoUpdates: clause.AssignmentColumns([]string{"price_usd", "price_raw", "best_buy", "best_sell", "confidence"}),
		},
	).Create(&onChainPrice)

	if result.Error != nil {
		return errors.Wrap(result.Error, "failed to save price to database")
	}

	return nil
}

// GetLatestPrice gets the latest price for a symbol.
func (s *OnChainPriceSyncer) GetLatestPrice(
	ctx context.Context,
	symbol string,
) (*models.OnChainPrice, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	var price models.OnChainPrice

	result := s.db.WithContext(ctx).
		Where("symbol = ?", symbol).
		Order("timestamp DESC").
		First(&price)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, errors.Errorf("no price found for symbol: %s", symbol)
		}
		return nil, errors.Wrap(result.Error, "failed to query price")
	}

	return &price, nil
}

// GetPriceHistory gets the price history for a symbol within a time range.
func (s *OnChainPriceSyncer) GetPriceHistory(
	ctx context.Context,
	symbol string,
	startTime, endTime int64,
	limit int,
) ([]*models.OnChainPrice, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	var prices []*models.OnChainPrice

	query := s.db.WithContext(ctx).
		Where("symbol = ?", symbol).
		Where("timestamp >= ?", startTime).
		Where("timestamp <= ?", endTime).
		Order("timestamp DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	result := query.Find(&prices)
	if result.Error != nil {
		return nil, errors.Wrap(result.Error, "failed to query price history")
	}

	return prices, nil
}

// acquireLock acquires the distributed lock.
func (s *OnChainPriceSyncer) acquireLock(ctx context.Context) bool {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		// No Redis, return true for single-instance mode
		return true
	}

	success, err := redisClient.SetNX(ctx, priceSyncLockKey, s.syncLockValue, 60*time.Second).Result()
	if err != nil {
		s.logger.Warnw("failed to acquire lock", "error", err)
		return false
	}

	return success
}

// renewLock renews the distributed lock.
func (s *OnChainPriceSyncer) renewLock() bool {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return true
	}

	// Use Lua script to ensure only lock holder can renew
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("expire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result, err := redisClient.Eval(context.Background(), script, []string{priceSyncLockKey}, s.syncLockValue, 60).Result()
	if err != nil {
		s.logger.Warnw("failed to renew lock", "error", err)
		return false
	}

	return result.(int64) == 1
}

// releaseLock releases the distributed lock.
func (s *OnChainPriceSyncer) releaseLock() {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Use Lua script to ensure only lock holder can release
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	_, err := redisClient.Eval(context.Background(), script, []string{priceSyncLockKey}, s.syncLockValue).Result()
	if err != nil {
		s.logger.Warnw("failed to release lock", "error", err)
	}
}

// generateLockValue generates a unique lock value.
func generateLockValue() string {
	return time.Now().Format(time.RFC3339Nano) + "-" + generateRandomString(8)
}

// generateRandomString generates a random string of the given length.
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}

// GetSyncStatus returns the current sync status.
func (s *OnChainPriceSyncer) GetSyncStatus() map[string]interface{} {
	return map[string]interface{}{
		"enabled":              s.enabled,
		"native_interval":      s.nativeInterval.String(),
		"all_markets_interval": s.allMarketsInterval.String(),
		"native_tokens":        s.nativeTokens,
		"batch_size":           s.batchSize,
	}
}

// Enable enables the sync service.
func (s *OnChainPriceSyncer) Enable() {
	s.enabled = true
}

// Disable disables the sync service.
func (s *OnChainPriceSyncer) Disable() {
	s.enabled = false
}

// ManualSync triggers a manual sync.
func (s *OnChainPriceSyncer) ManualSync(ctx context.Context) error {
	if s.logger != nil {
		s.logger.Info("manual sync triggered")
	}

	// Sync native tokens
	s.syncNativeTokens(ctx)

	// Sync all markets
	s.syncAllMarkets(ctx)

	return nil
}
