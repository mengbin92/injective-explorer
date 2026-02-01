// Package service provides on-chain price fetching and aggregation functionality.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"

	"cosmossdk.io/math"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/bytedance/sonic"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

const (
	// defaultCacheTTL is the default cache time-to-live.
	defaultCacheTTL = 30 * time.Second
	// defaultRetryAttempts is the default number of retry attempts.
	defaultRetryAttempts = 3
	// defaultTimeout is the default query timeout.
	defaultTimeout = 10 * time.Second
	// priceCacheKeyPrefix is the cache key prefix for price data.
	priceCacheKeyPrefix = "onchain:price"
)

// OnChainPriceProvider fetches token prices from on-chain spot markets.
type OnChainPriceProvider struct {
	chainClient chainclient.ChainClientV2
	logger      *log.Helper

	cacheTTL      time.Duration
	retryAttempts int
	timeout       time.Duration
}

// NewOnChainPriceProvider creates a new price provider with the given chain client and logger.
func NewOnChainPriceProvider(
	chainClient chainclient.ChainClientV2,
	logger *log.Helper,
) *OnChainPriceProvider {
	if logger == nil {
		logger = global.Logger
	}

	return &OnChainPriceProvider{
		chainClient:   chainClient,
		logger:        logger,
		cacheTTL:      defaultCacheTTL,
		retryAttempts: defaultRetryAttempts,
		timeout:       defaultTimeout,
	}
}

// NewOnChainPriceProviderWithSharedClient creates a price provider using the shared chain client.
func NewOnChainPriceProviderWithSharedClient(logger *log.Helper) (*OnChainPriceProvider, error) {
	chainClient, err := GetSharedChainClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get shared chain client")
	}

	return NewOnChainPriceProvider(chainClient, logger), nil
}

// GetPriceFromSpotMarket fetches the price from a spot market by market ID.
func (p *OnChainPriceProvider) GetPriceFromSpotMarket(
	ctx context.Context,
	marketID string,
) (*models.PriceInfo, error) {
	cacheKey := p.getCacheKey("spot", marketID)

	// Check cache first
	if cached := p.getFromCache(ctx, cacheKey); cached != nil {
		p.logger.Debugw("returning cached spot price",
			"market_id", marketID,
			"price", cached.PriceUSD,
		)
		return cached, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Fetch market info
	marketsResp, err := p.chainClient.FetchChainSpotMarkets(queryCtx, "", []string{marketID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch spot market")
	}

	if len(marketsResp.Markets) == 0 {
		return nil, errors.Errorf("market not found: %s", marketID)
	}

	market := marketsResp.Markets[0]

	// Fetch mid price and top-of-book
	midPriceResp, err := p.chainClient.FetchSpotMidPriceAndTOB(queryCtx, marketID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch mid price")
	}

	// Parse price from raw value
	price, err := p.parsePrice(midPriceResp.MidPrice.String())
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse price")
	}

	priceInfo := &models.PriceInfo{
		Symbol:     p.extractSymbol(market.Ticker),
		Denom:      market.BaseDenom,
		PriceUSD:   price,
		PriceRaw:   midPriceResp.MidPrice.String(),
		BestBuy:    midPriceResp.BestBuyPrice.String(),
		BestSell:   midPriceResp.BestSellPrice.String(),
		Source:     "spot_market",
		MarketID:   marketID,
		Timestamp:  time.Now().Unix(),
		Confidence: p.calculateConfidence(market, midPriceResp),
	}

	// Cache the result
	p.saveToCache(ctx, cacheKey, priceInfo)

	return priceInfo, nil
}

// FindMarketByDenom finds a market by base denom and preferred quote denoms.
func (p *OnChainPriceProvider) FindMarketByDenom(
	ctx context.Context,
	baseDenom string,
	preferredQuotes []string,
) (string, error) {
	if len(preferredQuotes) == 0 {
		quoteConfig := global.GetQuoteTokenConfig()
		preferredQuotes = quoteConfig.QuotePriority
	}

	queryCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Fetch all markets
	marketsResp, err := p.chainClient.FetchChainSpotMarkets(queryCtx, "", nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch markets")
	}

	// Find the best matching market
	for _, quote := range preferredQuotes {
		for _, market := range marketsResp.Markets {
			if strings.EqualFold(market.BaseDenom, baseDenom) &&
				strings.EqualFold(market.QuoteDenom, quote) {
				p.logger.Debugw("found market for denom",
					"denom", baseDenom,
					"market_id", market.MarketId,
					"ticker", market.Ticker,
				)
				return market.MarketId, nil
			}
		}
	}

	return "", errors.Errorf("no market found for denom: %s", baseDenom)
}

// GetPriceByDenom gets the token price by its denom.
func (p *OnChainPriceProvider) GetPriceByDenom(
	ctx context.Context,
	denom string,
) (*models.PriceInfo, error) {
	// 1. Find the corresponding market
	marketID, err := p.FindMarketByDenom(ctx, denom, nil)
	if err != nil {
		return nil, err
	}

	// 2. Fetch the price from the market
	return p.GetPriceFromSpotMarket(ctx, marketID)
}

// GetNativeTokenPrice gets the native token price (e.g., INJ, INJ).
func (p *OnChainPriceProvider) GetNativeTokenPrice(
	ctx context.Context,
	nativeDenom string,
) (*models.PriceInfo, error) {
	// Prioritize USDT/USDC trading pairs
	quoteConfig := global.GetQuoteTokenConfig()
	preferredQuotes := quoteConfig.QuotePriority

	marketID, err := p.FindMarketByDenom(ctx, nativeDenom, preferredQuotes)
	if err != nil {
		return nil, err
	}

	return p.GetPriceFromSpotMarket(ctx, marketID)
}

// BatchGetPrices gets prices for multiple denoms concurrently.
func (p *OnChainPriceProvider) BatchGetPrices(
	ctx context.Context,
	denoms []string,
) map[string]*models.PriceInfo {
	result := make(map[string]*models.PriceInfo)

	// Concurrent fetching with limited concurrency
	const maxConcurrency = 5
	semaphore := make(chan struct{}, maxConcurrency)

	type resultItem struct {
		denom string
		price *models.PriceInfo
		err   error
	}

	resultChan := make(chan resultItem, len(denoms))

	for _, denom := range denoms {
		go func(d string) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			price, err := p.GetPriceByDenom(ctx, d)
			resultChan <- resultItem{
				denom: d,
				price: price,
				err:   err,
			}
		}(denom)
	}

	// Collect results
	for i := 0; i < len(denoms); i++ {
		item := <-resultChan
		if item.err != nil {
			p.logger.Warnw("failed to get price",
				"denom", item.denom,
				"error", item.err,
			)
			continue
		}
		result[item.denom] = item.price
	}

	return result
}

// parsePrice parses price from a raw string with 18 decimal places.
func (p *OnChainPriceProvider) parsePrice(priceStr string) (float64, error) {
	if priceStr == "" {
		return 0, errors.New("empty price string")
	}

	priceDec, err := math.LegacyNewDecFromStr(priceStr)
	if err != nil {
		return 0, errors.Wrap(err, "invalid price format")
	}

	// Convert from 18 decimal places
	divisor := math.LegacyNewDecWithPrec(1, 18) // 10^-18
	actualPrice := priceDec.Mul(divisor)

	priceFloat, _ := actualPrice.Float64()
	return priceFloat, nil
}

// extractSymbol extracts the symbol from a ticker (e.g., "INJ/USDT" -> "INJ").
func (p *OnChainPriceProvider) extractSymbol(ticker string) string {
	parts := strings.Split(ticker, "/")
	if len(parts) > 0 {
		return strings.ToUpper(parts[0])
	}
	return strings.ToUpper(ticker)
}

// calculateConfidence calculates price confidence based on market data.
func (p *OnChainPriceProvider) calculateConfidence(
	market interface{},
	midPriceResp interface{},
) float64 {
	// Base confidence
	confidence := 0.9

	// TODO: Adjust confidence based on:
	// - Market liquidity (order book depth)
	// - Bid-ask spread
	// - Market status
	// - Recent trading activity

	return confidence
}

// getCacheKey generates a cache key with the given prefix and identifier.
func (p *OnChainPriceProvider) getCacheKey(prefix, identifier string) string {
	return fmt.Sprintf("%s:%s:%s", priceCacheKeyPrefix, prefix, identifier)
}

// getFromCache reads price from cache.
func (p *OnChainPriceProvider) getFromCache(ctx context.Context, key string) *models.PriceInfo {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil
	}

	cached, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			p.logger.Warnw("failed to get from cache", "error", err)
		}
		return nil
	}

	var cacheData models.PriceCacheData
	if err := sonic.Unmarshal([]byte(cached), &cacheData); err != nil {
		p.logger.Warnw("failed to unmarshal cache data", "error", err)
		return nil
	}

	// Check if cache has expired
	if time.Now().Unix()-cacheData.Timestamp > int64(p.cacheTTL.Seconds()) {
		return nil
	}

	return &models.PriceInfo{
		Symbol:      "", // Symbol not stored in cache, will be fetched separately
		PriceUSD:    cacheData.Price,
		PriceRaw:    cacheData.PriceRaw,
		BestBuy:     cacheData.BestBuy,
		BestSell:    cacheData.BestSell,
		Source:      cacheData.Source,
		MarketID:    cacheData.MarketID,
		Timestamp:   cacheData.Timestamp,
		BlockHeight: cacheData.BlockHeight,
		Confidence:  cacheData.Confidence,
	}
}

// saveToCache saves price to cache.
func (p *OnChainPriceProvider) saveToCache(ctx context.Context, key string, price *models.PriceInfo) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	cacheData := models.PriceCacheData{
		Price:       price.PriceUSD,
		PriceRaw:    price.PriceRaw,
		BestBuy:     price.BestBuy,
		BestSell:    price.BestSell,
		Source:      price.Source,
		MarketID:    price.MarketID,
		Timestamp:   price.Timestamp,
		BlockHeight: price.BlockHeight,
		Confidence:  price.Confidence,
	}

	data, err := sonic.Marshal(cacheData)
	if err != nil {
		p.logger.Warnw("failed to marshal cache data", "error", err)
		return
	}

	if err := redisClient.Set(ctx, key, string(data), p.cacheTTL).Err(); err != nil {
		p.logger.Warnw("failed to save to cache", "error", err)
	}
}

// SetCacheTTL sets the cache TTL (for testing or special scenarios).
func (p *OnChainPriceProvider) SetCacheTTL(ttl time.Duration) {
	p.cacheTTL = ttl
}

// SetTimeout sets the query timeout.
func (p *OnChainPriceProvider) SetTimeout(timeout time.Duration) {
	p.timeout = timeout
}
