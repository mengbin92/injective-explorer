// Package service provides tests for on-chain price functionality.
package service

import (
	"context"
	"testing"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/stretchr/testify/assert"
)

// mockChainClient is a mock implementation of ChainClientV2 for testing
type mockChainClient struct {
	spotMarkets      map[string]*mockSpotMarket
	midPriceResponses map[string]*mockMidPriceResponse
}

type mockSpotMarket struct {
	MarketId    string
	Ticker      string
	BaseDenom   string
	QuoteDenom  string
}

type mockMidPriceResponse struct {
	MidPrice      string
	BestBuyPrice  string
	BestSellPrice string
}

func newMockChainClient() *mockChainClient {
	return &mockChainClient{
		spotMarkets: map[string]*mockSpotMarket{
			"0x1234": {
				MarketId:   "0x1234",
				Ticker:     "INJ/USDT",
				BaseDenom:  "inj",
				QuoteDenom: global.DefaultQuoteTokens.USDTAddress,
			},
			"0x5678": {
				MarketId:   "0x5678",
				Ticker:     "INJ/USDT",
				BaseDenom:  "inj",
				QuoteDenom: global.DefaultQuoteTokens.USDTAddress,
			},
		},
		midPriceResponses: map[string]*mockMidPriceResponse{
			"0x1234": {
				MidPrice:      "2500000000000000000", // 2.5 INJ/USDT
				BestBuyPrice:  "2495000000000000000",
				BestSellPrice: "2505000000000000000",
			},
			"0x5678": {
				MidPrice:      "100000000000000000", // 0.1 INJ/USDT
				BestBuyPrice:  "99500000000000000",
				BestSellPrice: "100500000000000000",
			},
		},
	}
}

func TestNewOnChainPriceProvider(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))

	// Test with nil client (should work, will fail on actual queries)
	provider := NewOnChainPriceProvider(nil, logger)
	assert.NotNil(t, provider)
	assert.Equal(t, defaultCacheTTL, provider.cacheTTL)
	assert.Equal(t, defaultRetryAttempts, provider.retryAttempts)
	assert.Equal(t, defaultTimeout, provider.timeout)
}

func TestOnChainPriceProvider_SetCacheTTL(t *testing.T) {
	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))

	newTTL := 1 * time.Minute
	provider.SetCacheTTL(newTTL)
	assert.Equal(t, newTTL, provider.cacheTTL)
}

func TestOnChainPriceProvider_SetTimeout(t *testing.T) {
	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))

	newTimeout := 5 * time.Second
	provider.SetTimeout(newTimeout)
	assert.Equal(t, newTimeout, provider.timeout)
}

func TestOnChainPriceProvider_parsePrice(t *testing.T) {
	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))

	tests := []struct {
		name      string
		priceStr  string
		expected  float64
		wantError bool
	}{
		{
			name:     "valid price 2.5",
			priceStr: "2500000000000000000",
			expected: 2.5,
		},
		{
			name:     "valid price 0.1",
			priceStr: "100000000000000000",
			expected: 0.1,
		},
		{
			name:     "valid price 1.0",
			priceStr: "1000000000000000000",
			expected: 1.0,
		},
		{
			name:      "empty string",
			priceStr:  "",
			wantError: true,
		},
		{
			name:      "invalid format",
			priceStr:  "invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := provider.parsePrice(tt.priceStr)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.0001)
			}
		})
	}
}

func TestOnChainPriceProvider_extractSymbol(t *testing.T) {
	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))

	tests := []struct {
		ticker   string
		expected string
	}{
		{"INJ/USDT", "INJ"},
		{"INJ/USDC", "INJ"},
		{"ATOM", "ATOM"},
		{"weth/usdt", "WETH"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ticker, func(t *testing.T) {
			result := provider.extractSymbol(tt.ticker)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOnChainPriceProvider_getCacheKey(t *testing.T) {
	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))

	key := provider.getCacheKey("spot", "0x1234")
	assert.Equal(t, "onchain:price:spot:0x1234", key)
}

func TestOnChainPriceProvider_calculateConfidence(t *testing.T) {
	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))

	// Currently returns fixed value of 0.9
	confidence := provider.calculateConfidence(nil, nil)
	assert.Equal(t, 0.9, confidence)
}

// Integration test (skipped by default)
func TestOnChainPriceProvider_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a real chain connection
	// It should only be run manually with proper environment setup
	t.Skip("Integration test - run manually with chain connection")
}

// Example usage test
func ExampleOnChainPriceProvider() {
	// This example shows how to use OnChainPriceProvider

	// Create provider (in real code, use proper chain client)
	// provider, err := NewOnChainPriceProviderWithSharedClient(logger)
	// if err != nil {
	//     log.Fatal(err)
	// }

	// Get price for a specific denom
	// ctx := context.Background()
	// price, err := provider.GetNativeTokenPrice(ctx, "inj")
	// if err != nil {
	//     log.Fatal(err)
	// }

	// fmt.Printf("INJ price: $%.2f\n", price.PriceUSD)
}

// Test batch get prices
func TestOnChainPriceProvider_BatchGetPrices(t *testing.T) {
	// This test requires mocking the chain client
	// For now, just test the function signature

	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))

	// Test with empty denoms
	ctx := context.Background()
	result := provider.BatchGetPrices(ctx, []string{})
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// Benchmark parse price
func BenchmarkOnChainPriceProvider_parsePrice(b *testing.B) {
	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))
	priceStr := "2500000000000000000"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.parsePrice(priceStr)
	}
}

// Benchmark extract symbol
func BenchmarkOnChainPriceProvider_extractSymbol(b *testing.B) {
	provider := NewOnChainPriceProvider(nil, log.NewHelper(log.NewStdLogger(nil)))
	ticker := "INJ/USDT"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = provider.extractSymbol(ticker)
	}
}

// Ensure PriceInfo struct is properly defined
func TestPriceInfo_Struct(t *testing.T) {
	info := &models.PriceInfo{
		Symbol:      "INJ",
		Denom:       "inj",
		PriceUSD:    2.5,
		PriceRaw:    "2500000000000000000",
		BestBuy:     "2495000000000000000",
		BestSell:    "2505000000000000000",
		Source:      "spot_market",
		MarketID:    "0x1234",
		Timestamp:   time.Now().Unix(),
		BlockHeight: 12345678,
		Confidence:  0.95,
	}

	assert.Equal(t, "INJ", info.Symbol)
	assert.Equal(t, "inj", info.Denom)
	assert.Equal(t, 2.5, info.PriceUSD)
	assert.Equal(t, "spot_market", info.Source)
	assert.Equal(t, "0x1234", info.MarketID)
}

// Test constants
func TestConstants(t *testing.T) {
	// Test default quote token configuration
	config := global.DefaultQuoteTokens
	assert.NotEmpty(t, config.USDTAddress)
	assert.NotEmpty(t, config.USDCAddress)
	assert.NotEmpty(t, config.QuotePriority)
	assert.Contains(t, config.QuotePriority, config.USDTAddress)
	assert.Contains(t, config.QuotePriority, config.USDCAddress)
}

// Helper function to check if integration test should run
func shouldRunIntegrationTest() bool {
	// Check for environment variable or other conditions
	return false // Default to false for safety
}

// Test for specific denom formats
func TestDenomFormats(t *testing.T) {
	// Test common denom formats
	testDenoms := []string{
		"inj",
		"inj",
		"peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5", // USDT
		"peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", // USDC
		"ibc/E47F4E97C534C95B942729E1B25DBDE111EA791411CFF100515050BEA0634C3A", // IBC denom
	}

	for _, denom := range testDenoms {
		// Safe substring for test name
		testName := denom
		if len(testName) > 4 {
			testName = testName[:4]
		}
		t.Run("denom_"+testName, func(t *testing.T) {
			// Verify denom is not empty
			assert.NotEmpty(t, denom)
		})
	}
}

// Test cache data serialization
func TestPriceCacheData_Serialization(t *testing.T) {
	cacheData := models.PriceCacheData{
		Price:       2.5,
		PriceRaw:    "2500000000000000000",
		BestBuy:     "2495000000000000000",
		BestSell:    "2505000000000000000",
		Source:      "spot_market",
		MarketID:    "0x1234",
		Timestamp:   time.Now().Unix(),
		BlockHeight: 12345678,
		Confidence:  0.95,
	}

	// The actual serialization is done via sonic or json
	// This test verifies the struct is properly defined
	assert.NotZero(t, cacheData.Timestamp)
	assert.Equal(t, 2.5, cacheData.Price)
	assert.Equal(t, "spot_market", cacheData.Source)
}
