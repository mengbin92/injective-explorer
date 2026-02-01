// Package service provides on-chain price aggregation and fallback functionality.
package service

import (
	"context"
	"math"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/provider/cache"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
)

const (
	// priceAggregatorCacheKey is the cache key for aggregated prices.
	priceAggregatorCacheKey = "onchain:price:aggregated"
	// defaultAggregatorCacheTTL is the default cache TTL for aggregated prices.
	defaultAggregatorCacheTTL = 60 * time.Second
	// defaultMaxPriceDeviation is the max price deviation for outlier detection (5%).
	defaultMaxPriceDeviation = 0.05
)

// PriceAggregator aggregates prices from multiple sources.
type PriceAggregator struct {
	onchainProvider *OnChainPriceProvider
	logger          *log.Helper

	cacheTTL          time.Duration
	maxPriceDeviation float64

	// Price source configurations
	sources []PriceSource
}

// PriceSource is the interface for price sources.
type PriceSource interface {
	Name() string
	GetPrice(ctx context.Context, symbol string) (float64, error)
	GetPriority() int
	IsHealthy() bool
}

// AggregatedPrice represents an aggregated price from multiple sources.
type AggregatedPrice struct {
	Symbol        string
	PriceUSD      float64
	Sources       map[string]float64
	BestSource    string
	Confidence    float64
	Timestamp     int64
	IsOutlier     bool
	OutlierReason string
}

// NewPriceAggregator creates a new price aggregator.
func NewPriceAggregator(
	onchainProvider *OnChainPriceProvider,
	logger *log.Helper,
) *PriceAggregator {
	if logger == nil {
		logger = global.Logger
	}

	return &PriceAggregator{
		onchainProvider:   onchainProvider,
		logger:            logger,
		cacheTTL:          defaultAggregatorCacheTTL,
		maxPriceDeviation: defaultMaxPriceDeviation,
		sources:           make([]PriceSource, 0),
	}
}

// AddSource adds a price source to the aggregator.
func (a *PriceAggregator) AddSource(source PriceSource) {
	a.sources = append(a.sources, source)
}

// SetMaxPriceDeviation sets the maximum allowed price deviation.
func (a *PriceAggregator) SetMaxPriceDeviation(deviation float64) {
	a.maxPriceDeviation = deviation
}

// GetAggregatedPrice gets the aggregated price from multiple sources.
func (a *PriceAggregator) GetAggregatedPrice(
	ctx context.Context,
	symbol string,
) (*AggregatedPrice, error) {
	// Check cache first
	if cached := a.getFromCache(ctx, symbol); cached != nil {
		return cached, nil
	}

	// Collect prices from all sources
	sourcePrices := make(map[string]float64)
	var validPrices []float64
	var validSources []string

	// 1. Try on-chain price first
	if a.onchainProvider != nil {
		if price, err := a.getOnChainPrice(ctx, symbol); err == nil {
			sourcePrices["onchain"] = price
			validPrices = append(validPrices, price)
			validSources = append(validSources, "onchain")
		} else {
			a.logger.Debugw("failed to get on-chain price",
				"symbol", symbol,
				"error", err,
			)
		}
	}

	// 2. Try other price sources
	for _, source := range a.sources {
		if !source.IsHealthy() {
			continue
		}

		price, err := source.GetPrice(ctx, symbol)
		if err != nil {
			a.logger.Debugw("failed to get price from source",
				"symbol", symbol,
				"source", source.Name(),
				"error", err,
			)
			continue
		}

		sourcePrices[source.Name()] = price
		validPrices = append(validPrices, price)
		validSources = append(validSources, source.Name())
	}

	if len(validPrices) == 0 {
		return nil, errors.Errorf("no valid price source for symbol: %s", symbol)
	}

	// Calculate aggregated price
	aggregated := a.calculateAggregatedPrice(symbol, sourcePrices, validPrices, validSources)

	// Save to cache
	a.saveToCache(ctx, symbol, aggregated)

	return aggregated, nil
}

// getOnChainPrice gets the on-chain price for a symbol.
func (a *PriceAggregator) getOnChainPrice(
	ctx context.Context,
	symbol string,
) (float64, error) {
	// Convert symbol to denom
	denom := symbolToDenom(symbol)

	priceInfo, err := a.onchainProvider.GetPriceByDenom(ctx, denom)
	if err != nil {
		return 0, err
	}

	return priceInfo.PriceUSD, nil
}

// calculateAggregatedPrice calculates the aggregated price from multiple sources.
func (a *PriceAggregator) calculateAggregatedPrice(
	symbol string,
	sourcePrices map[string]float64,
	validPrices []float64,
	validSources []string,
) *AggregatedPrice {
	// Use median as the base price
	medianPrice := calculateMedian(validPrices)

	// Detect outliers
	isOutlier := false
	outlierReason := ""

	if len(validPrices) >= 2 {
		for i, price := range validPrices {
			deviation := math.Abs(price-medianPrice) / medianPrice
			if deviation > a.maxPriceDeviation {
				isOutlier = true
				outlierReason = validSources[i] + " price deviates too much"
				a.logger.Warnw("price outlier detected",
					"symbol", symbol,
					"source", validSources[i],
					"price", price,
					"median", medianPrice,
					"deviation", deviation,
				)
			}
		}
	}

	// Calculate confidence score
	confidence := a.calculateConfidence(len(validPrices), isOutlier)

	// Select best price source (prefer on-chain)
	bestSource := a.selectBestSource(sourcePrices, validSources)
	bestPrice := sourcePrices[bestSource]

	return &AggregatedPrice{
		Symbol:        symbol,
		PriceUSD:      bestPrice,
		Sources:       sourcePrices,
		BestSource:    bestSource,
		Confidence:    confidence,
		Timestamp:     time.Now().Unix(),
		IsOutlier:     isOutlier,
		OutlierReason: outlierReason,
	}
}

// selectBestSource selects the best price source.
func (a *PriceAggregator) selectBestSource(
	sourcePrices map[string]float64,
	validSources []string,
) string {
	// Prefer on-chain price
	if _, ok := sourcePrices["onchain"]; ok {
		return "onchain"
	}

	// Then use highest priority source
	for _, source := range a.sources {
		if _, ok := sourcePrices[source.Name()]; ok {
			return source.Name()
		}
	}

	// Default to first valid source
	if len(validSources) > 0 {
		return validSources[0]
	}

	return "unknown"
}

// calculateConfidence calculates the confidence score based on source count and outliers.
func (a *PriceAggregator) calculateConfidence(
	sourceCount int,
	isOutlier bool,
) float64 {
	baseConfidence := 0.9

	// Adjust based on number of sources
	if sourceCount >= 2 {
		baseConfidence += 0.05
	}
	if sourceCount >= 3 {
		baseConfidence += 0.03
	}

	// Reduce confidence if outlier detected
	if isOutlier {
		baseConfidence -= 0.2
	}

	// Clamp to [0, 1] range
	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}
	if baseConfidence < 0 {
		baseConfidence = 0
	}

	return baseConfidence
}

// getFromCache gets the aggregated price from cache.
func (a *PriceAggregator) getFromCache(ctx context.Context, symbol string) *AggregatedPrice {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil
	}

	cacheKey := priceAggregatorCacheKey + ":" + symbol
	// Simplified implementation - actual implementation needs deserialization
	// Returns nil to indicate cache miss
	_ = cacheKey
	return nil
}

// saveToCache saves the aggregated price to cache.
func (a *PriceAggregator) saveToCache(ctx context.Context, symbol string, price *AggregatedPrice) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	cacheKey := priceAggregatorCacheKey + ":" + symbol
	// Simplified implementation - actual implementation needs serialization
	_ = cacheKey
	_ = price
}

// calculateMedian calculates the median of a slice of values.
func calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Copy slice to avoid modifying original data
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Bubble sort (efficient enough for small datasets)
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// symbolToDenom converts a symbol to its denom.
func symbolToDenom(symbol string) string {
	switch symbol {
	case "INJ":
		return "inj"
	default:
		return symbol
	}
}

// CoinGeckoPriceSource is a price source that fetches from CoinGecko.
type CoinGeckoPriceSource struct {
	name     string
	priority int
	healthy  bool
}

// NewCoinGeckoPriceSource creates a new CoinGecko price source.
func NewCoinGeckoPriceSource() *CoinGeckoPriceSource {
	return &CoinGeckoPriceSource{
		name:     "coingecko",
		priority: 1,
		healthy:  true,
	}
}

// Name returns the price source name.
func (c *CoinGeckoPriceSource) Name() string {
	return c.name
}

// GetPrice fetches the price from CoinGecko.
func (c *CoinGeckoPriceSource) GetPrice(ctx context.Context, symbol string) (float64, error) {
	// Get token configuration
	tokenConfig := GetTokenConfigOrCreate(symbol)
	if tokenConfig == nil {
		return 0, errors.Errorf("token config not found for symbol: %s", symbol)
	}

	// Call existing CoinGecko interface
	price, _, err := GetTokenPriceFromCoinGecko(ctx, tokenConfig)
	if err != nil {
		c.healthy = false
		return 0, err
	}

	c.healthy = true
	return price, nil
}

// GetPriority returns the priority of this source.
func (c *CoinGeckoPriceSource) GetPriority() int {
	return c.priority
}

// IsHealthy returns whether the source is healthy.
func (c *CoinGeckoPriceSource) IsHealthy() bool {
	return c.healthy
}

// SetHealthy sets the health status of this source.
func (c *CoinGeckoPriceSource) SetHealthy(healthy bool) {
	c.healthy = healthy
}

// FallbackPriceProvider provides price with fallback to alternative sources.
type FallbackPriceProvider struct {
	onchainProvider *OnChainPriceProvider
	coingeckoSource *CoinGeckoPriceSource
	logger          *log.Helper
}

// NewFallbackPriceProvider creates a new fallback price provider.
func NewFallbackPriceProvider(
	onchainProvider *OnChainPriceProvider,
	logger *log.Helper,
) *FallbackPriceProvider {
	if logger == nil {
		logger = global.Logger
	}

	return &FallbackPriceProvider{
		onchainProvider: onchainProvider,
		coingeckoSource: NewCoinGeckoPriceSource(),
		logger:          logger,
	}
}

// GetPriceWithFallback gets the price with fallback to CoinGecko.
func (f *FallbackPriceProvider) GetPriceWithFallback(
	ctx context.Context,
	symbol string,
) (*AggregatedPrice, error) {
	// 1. Try on-chain price first
	if f.onchainProvider != nil {
		denom := symbolToDenom(symbol)
		if priceInfo, err := f.onchainProvider.GetPriceByDenom(ctx, denom); err == nil {
			return &AggregatedPrice{
				Symbol:     symbol,
				PriceUSD:   priceInfo.PriceUSD,
				Sources:    map[string]float64{"onchain": priceInfo.PriceUSD},
				BestSource: "onchain",
				Confidence: priceInfo.Confidence,
				Timestamp:  time.Now().Unix(),
			}, nil
		} else {
			f.logger.Warnw("on-chain price failed, falling back to CoinGecko",
				"symbol", symbol,
				"error", err,
			)
		}
	}

	// 2. Fallback to CoinGecko
	if f.coingeckoSource != nil {
		price, err := f.coingeckoSource.GetPrice(ctx, symbol)
		if err == nil {
			return &AggregatedPrice{
				Symbol:     symbol,
				PriceUSD:   price,
				Sources:    map[string]float64{"coingecko": price},
				BestSource: "coingecko",
				Confidence: 0.85, // Lower confidence for fallback source
				Timestamp:  time.Now().Unix(),
			}, nil
		} else {
			f.logger.Warnw("CoinGecko price also failed",
				"symbol", symbol,
				"error", err,
			)
		}
	}

	return nil, errors.Errorf("all price sources failed for symbol: %s", symbol)
}
