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

// denomToSymbolMap maps common denoms to their token symbols for CoinGecko lookup.
// This is used when falling back to CoinGecko price API.
var denomToSymbolMap = map[string]string{
	// Native tokens
	"inj": "inj",

	// Stablecoins
	"peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5": "usdt",
	"peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48": "usdc",
	"peggy0xdAC17F958D2ee523a2206206994597C13D831ec7": "usdt",

	// Major tokens
	"peggy0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599": "wbtc",
	"peggy0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2": "weth",
	"peggy0x6B3595068778DD592e39A122f4f5a5cF09C90fE2": "sushi",
	"peggy0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984": "uni",
	"peggy0x514910771AF9Ca656af840dff83E8264EcF986CA": "link",
	"peggy0x7Fc66500c84A76Ad7e9c93437bFc5Ac33E2DDaE9": "aave",
	"peggy0xc00e94Cb662C3520282E6f5717214004A7f26888": "comp",
	"peggy0x9f8F72aA9304c8B593d555F12eF6589cC3A579A2": "mkr",
	"peggy0xc011a73ee8576Fb46F5E1c5751cA3B9Fe0af2a6F": "snx",
	"peggy0x0bc529c00C6401aEF6D220BE8C6Ea1667F6Ad93e": "yfi",
	"peggy0xD533a949740bb3306d119CC777fa900bA034cd52": "crv",
	"peggy0x111111111117dC0aa78b770fA6A738034120C302": "1inch",
	"peggy0xc944E90C64B2c07662A292be6244BDf05Cda44a7": "grt",
}

// extractSymbolFromDenom extracts token symbol from denom for CoinGecko lookup.
// It first checks the mapping, then falls back to using the denom directly.
func extractSymbolFromDenom(denom string) string {
	if symbol, ok := denomToSymbolMap[strings.ToLower(denom)]; ok {
		return symbol
	}
	// For unknown denoms, try to extract the last part after "peggy0x"
	denom = strings.ToLower(denom)
	if strings.HasPrefix(denom, "peggy0x") {
		// Return the full peggy address as-is for lookup
		return denom
	}
	return denom
}

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
) (priceInfo *models.PriceInfo, err error) {
	// Recover from any panic to prevent service crash
	defer func() {
		if r := recover(); r != nil {
			p.logger.Errorw("panic in GetPriceFromSpotMarket",
				"market_id", marketID,
				"recover", r,
			)
			priceInfo = nil
			err = errors.Errorf("panic in price fetch: %v", r)
		}
	}()

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

	// Check if midPriceResp is nil or has nil price fields
	if midPriceResp == nil || midPriceResp.MidPrice == nil {
		return nil, errors.Errorf("no mid price available for market: %s", marketID)
	}

	// Parse price from raw value
	price, err := p.parsePrice(midPriceResp.MidPrice.String())
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse price")
	}

	// Safely get optional price fields
	bestBuy := ""
	if midPriceResp.BestBuyPrice != nil {
		bestBuy = midPriceResp.BestBuyPrice.String()
	}
	bestSell := ""
	if midPriceResp.BestSellPrice != nil {
		bestSell = midPriceResp.BestSellPrice.String()
	}

	priceInfo = &models.PriceInfo{
		Symbol:     p.extractSymbol(market.Ticker),
		Denom:      market.BaseDenom,
		PriceUSD:   price,
		PriceRaw:   midPriceResp.MidPrice.String(),
		BestBuy:    bestBuy,
		BestSell:   bestSell,
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

	// Debug: log first few markets to see what's available
	if len(marketsResp.Markets) > 0 {
		p.logger.Debugw("sample markets from chain",
			"total_markets", len(marketsResp.Markets),
			"market_0_base", marketsResp.Markets[0].BaseDenom,
			"market_0_quote", marketsResp.Markets[0].QuoteDenom,
			"market_0_ticker", marketsResp.Markets[0].Ticker,
		)
	}

	// Find the best matching market
	for _, quote := range preferredQuotes {
		for _, market := range marketsResp.Markets {
			if strings.EqualFold(market.BaseDenom, baseDenom) &&
				strings.EqualFold(market.QuoteDenom, quote) {
				p.logger.Infow("found market for denom",
					"denom", baseDenom,
					"market_id", market.MarketId,
					"ticker", market.Ticker,
				)
				return market.MarketId, nil
			}
		}
	}

	// Debug: log available markets for this denom to help troubleshooting
	var availableMarkets []string
	for _, market := range marketsResp.Markets {
		if strings.EqualFold(market.BaseDenom, baseDenom) {
			availableMarkets = append(availableMarkets,
				fmt.Sprintf("%s/%s(id=%s)", market.BaseDenom, market.QuoteDenom, market.MarketId[:20]))
		}
	}
	if len(availableMarkets) > 0 {
		p.logger.Infow("markets found for denom but no preferred quote match",
			"denom", baseDenom,
			"preferred_quotes", preferredQuotes,
			"available_markets", availableMarkets,
		)
	} else {
		p.logger.Infow("no markets found for denom at all",
			"denom", baseDenom,
			"total_markets_checked", len(marketsResp.Markets),
		)
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
			// Recover from any panic in goroutine to prevent service crash
			defer func() {
				if r := recover(); r != nil {
					p.logger.Errorw("panic in BatchGetPrices goroutine",
						"denom", d,
						"recover", r,
					)
					resultChan <- resultItem{
						denom: d,
						price: nil,
						err:   errors.Errorf("panic: %v", r),
					}
					<-semaphore
				}
			}()

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
