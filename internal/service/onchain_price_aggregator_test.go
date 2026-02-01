// Package service provides tests for price aggregator functionality.
package service

import (
	"math"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/stretchr/testify/assert"
)

func TestNewPriceAggregator(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)

	aggregator := NewPriceAggregator(provider, logger)

	assert.NotNil(t, aggregator)
	assert.Equal(t, provider, aggregator.onchainProvider)
	assert.Equal(t, defaultAggregatorCacheTTL, aggregator.cacheTTL)
	assert.Equal(t, defaultMaxPriceDeviation, aggregator.maxPriceDeviation)
	assert.NotNil(t, aggregator.sources)
	assert.Empty(t, aggregator.sources)
}

func TestPriceAggregator_SetMaxPriceDeviation(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	aggregator := NewPriceAggregator(provider, logger)

	newDeviation := 0.1
	aggregator.SetMaxPriceDeviation(newDeviation)
	assert.Equal(t, newDeviation, aggregator.maxPriceDeviation)
}

func TestPriceAggregator_AddSource(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	aggregator := NewPriceAggregator(provider, logger)

	source := NewCoinGeckoPriceSource()
	aggregator.AddSource(source)

	assert.Len(t, aggregator.sources, 1)
	assert.Equal(t, source, aggregator.sources[0])
}

func TestCalculateMedian(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "odd number of values",
			values:   []float64{1.0, 2.0, 3.0},
			expected: 2.0,
		},
		{
			name:     "even number of values",
			values:   []float64{1.0, 2.0, 3.0, 4.0},
			expected: 2.5,
		},
		{
			name:     "single value",
			values:   []float64{5.0},
			expected: 5.0,
		},
		{
			name:     "empty slice",
			values:   []float64{},
			expected: 0,
		},
		{
			name:     "unsorted values",
			values:   []float64{5.0, 1.0, 3.0, 2.0, 4.0},
			expected: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateMedian(tt.values)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestSymbolToDenom(t *testing.T) {
	tests := []struct {
		symbol   string
		expected string
	}{
		{"INJ", "inj"},
		{"INJ", "inj"},
		{"ATOM", "ATOM"},
		{"", ""},
		{"eth", "eth"},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			result := symbolToDenom(tt.symbol)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewCoinGeckoPriceSource(t *testing.T) {
	source := NewCoinGeckoPriceSource()

	assert.NotNil(t, source)
	assert.Equal(t, "coingecko", source.name)
	assert.Equal(t, 1, source.priority)
	assert.True(t, source.healthy)
}

func TestCoinGeckoPriceSource_Name(t *testing.T) {
	source := NewCoinGeckoPriceSource()
	assert.Equal(t, "coingecko", source.Name())
}

func TestCoinGeckoPriceSource_GetPriority(t *testing.T) {
	source := NewCoinGeckoPriceSource()
	assert.Equal(t, 1, source.GetPriority())
}

func TestCoinGeckoPriceSource_IsHealthy(t *testing.T) {
	source := NewCoinGeckoPriceSource()
	assert.True(t, source.IsHealthy())

	source.SetHealthy(false)
	assert.False(t, source.IsHealthy())

	source.SetHealthy(true)
	assert.True(t, source.IsHealthy())
}

func TestNewFallbackPriceProvider(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)

	fallback := NewFallbackPriceProvider(provider, logger)

	assert.NotNil(t, fallback)
	assert.Equal(t, provider, fallback.onchainProvider)
	assert.NotNil(t, fallback.coingeckoSource)
	assert.NotNil(t, fallback.logger)
}

func TestPriceAggregator_calculateConfidence(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	aggregator := NewPriceAggregator(provider, logger)

	tests := []struct {
		name        string
		sourceCount int
		isOutlier   bool
		minExpected float64
		maxExpected float64
	}{
		{
			name:        "single source no outlier",
			sourceCount: 1,
			isOutlier:   false,
			minExpected: 0.8,
			maxExpected: 1.0,
		},
		{
			name:        "two sources no outlier",
			sourceCount: 2,
			isOutlier:   false,
			minExpected: 0.9,
			maxExpected: 1.0,
		},
		{
			name:        "three sources no outlier",
			sourceCount: 3,
			isOutlier:   false,
			minExpected: 0.95,
			maxExpected: 1.0,
		},
		{
			name:        "single source with outlier",
			sourceCount: 1,
			isOutlier:   true,
			minExpected: 0.6,
			maxExpected: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := aggregator.calculateConfidence(tt.sourceCount, tt.isOutlier)
			assert.True(t, confidence >= tt.minExpected && confidence <= tt.maxExpected,
				"confidence %f should be between %f and %f", confidence, tt.minExpected, tt.maxExpected)
		})
	}
}

func TestPriceAggregator_selectBestSource(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	aggregator := NewPriceAggregator(provider, logger)

	tests := []struct {
		name          string
		sourcePrices  map[string]float64
		validSources  []string
		expectedBest  string
	}{
		{
			name:         "prefer onchain",
			sourcePrices: map[string]float64{"onchain": 2.5, "coingecko": 2.51},
			validSources: []string{"onchain", "coingecko"},
			expectedBest: "onchain",
		},
		{
			name:         "only coingecko",
			sourcePrices: map[string]float64{"coingecko": 2.5},
			validSources: []string{"coingecko"},
			expectedBest: "coingecko",
		},
		{
			name:         "empty sources",
			sourcePrices: map[string]float64{},
			validSources: []string{},
			expectedBest: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			best := aggregator.selectBestSource(tt.sourcePrices, tt.validSources)
			assert.Equal(t, tt.expectedBest, best)
		})
	}
}

func TestAggregatedPrice_Struct(t *testing.T) {
	price := &AggregatedPrice{
		Symbol:        "INJ",
		PriceUSD:      2.5,
		Sources:       map[string]float64{"onchain": 2.5, "coingecko": 2.51},
		BestSource:    "onchain",
		Confidence:    0.95,
		Timestamp:     time.Now().Unix(),
		IsOutlier:     true,
		OutlierReason: "price deviation detected",
	}

	assert.Equal(t, "INJ", price.Symbol)
	assert.Equal(t, 2.5, price.PriceUSD)
	assert.Equal(t, "onchain", price.BestSource)
	assert.True(t, price.IsOutlier)
	assert.Equal(t, "price deviation detected", price.OutlierReason)
}

// Benchmark calculateMedian
func BenchmarkCalculateMedian(b *testing.B) {
	values := []float64{5.0, 1.0, 3.0, 2.0, 4.0, 6.0, 7.0, 8.0, 9.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calculateMedian(values)
	}
}

// Benchmark symbolToDenom
func BenchmarkSymbolToDenom(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = symbolToDenom("INJ")
	}
}

// Test price deviation calculation
func TestPriceDeviation(t *testing.T) {
	median := 2.5
	price1 := 2.5
	price2 := 2.6
	price3 := 3.0

	deviation1 := math.Abs(price1-median) / median
	deviation2 := math.Abs(price2-median) / median
	deviation3 := math.Abs(price3-median) / median

	assert.InDelta(t, 0.0, deviation1, 0.0001)
	assert.InDelta(t, 0.04, deviation2, 0.0001) // 4%
	assert.InDelta(t, 0.2, deviation3, 0.0001)  // 20%
}

// Integration test for GetAggregatedPrice
func TestPriceAggregator_GetAggregatedPrice_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This would require a real chain connection
	t.Skip("Integration test - requires chain connection")
}

// Test aggregator constants
func TestAggregatorConstants(t *testing.T) {
	assert.Equal(t, 60*time.Second, defaultAggregatorCacheTTL)
	assert.Equal(t, 0.05, defaultMaxPriceDeviation)
	assert.Equal(t, "onchain:price:aggregated", priceAggregatorCacheKey)
}
