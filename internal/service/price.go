// Package service provides price and market cap services for tokens.
// This file contains functions for fetching token prices from various sources
// and calculating market cap based on total supply.
package service

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/provider/cache"

	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
)

const (
	// Default decimals for tokens (like Ethereum/Injective)
	defaultDecimals = 18
	// Default price source
	defaultPriceSource = "coingecko"
)

// TokenConfig represents configuration for a token.
type TokenConfig struct {
	Symbol         string
	Denom          string
	CoinGeckoID    string
	Decimals       int32
	PriceSource    string
	CustomPriceURL string
	DisplayName    string
}

// CoinGeckoResponse represents the response from CoinGecko API.
type CoinGeckoResponse map[string]struct {
	USD          float64 `json:"usd"`
	USD24hChange float64 `json:"usd_24h_change"`
}

// CoinGeckoDetailResponse represents the detailed response from CoinGecko API with supply data.
type CoinGeckoDetailResponse map[string]struct {
	USD               float64  `json:"usd"`
	USD24hChange      float64  `json:"usd_24h_change"`
	TotalSupply       *float64 `json:"total_supply,omitempty"`
	CirculatingSupply *float64 `json:"circulating_supply,omitempty"`
	MaxSupply         *float64 `json:"max_supply,omitempty"`
}

// CustomPriceResponse represents the response from custom price API.
type CustomPriceResponse struct {
	Price       float64 `json:"price"`
	Change24h   float64 `json:"change_24h"`
	LastUpdated int64   `json:"last_updated,omitempty"`
}

// PriceAndSupplyData contains price and supply information.
type PriceAndSupplyData struct {
	TokenSymbol          string
	TokenName            string
	PriceUSD             float64
	PriceChange24h       float64
	TotalSupply          *big.Int // Total supply (after burn, from CoinGecko if available)
	TotalSupplyString    string
	TotalSupplyFromChain *big.Int // Chain supply (before burn, for reference)
	LastUpdated          int64
	PriceSource          string
	SupplySource         string // "coingecko" or "chain"
}

// getTokenConfig retrieves token configuration from global config.
func getTokenConfig(tokenSymbol string) (*TokenConfig, error) {
	if global.TokenPriceConfig != nil {
		return LoadTokenConfigFromConf(nil, tokenSymbol)
	}

	// Fallback to default INJ config
	if tokenSymbol == "" || tokenSymbol == "inj" {
		return &TokenConfig{
			Symbol:      "inj",
			Denom:       "inj",
			CoinGeckoID: "injective-protocol",
			Decimals:    18,
			PriceSource: "coingecko",
			DisplayName: "Injective",
		}, nil
	}

	return nil, errors.Errorf("token configuration not found for symbol: %s", tokenSymbol)
}

// getCacheKey generates cache key for a token.
func getCacheKey(prefix, tokenSymbol string) string {
	return fmt.Sprintf("%s:%s", prefix, tokenSymbol)
}

// GetTokenPriceFromCoinGecko fetches token price from CoinGecko API.
func GetTokenPriceFromCoinGecko(ctx context.Context, tokenConfig *TokenConfig) (float64, float64, error) {
	if tokenConfig.CoinGeckoID == "" {
		return 0, 0, errors.New("CoinGecko ID not configured for this token")
	}

	cacheKey := getCacheKey("price", tokenConfig.Symbol)

	// Check cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cachedPrice, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			var cachedData struct {
				Price       float64 `json:"price"`
				Change24h   float64 `json:"change_24h"`
				LastUpdated int64   `json:"last_updated"`
			}
			if err := sonic.Unmarshal([]byte(cachedPrice), &cachedData); err == nil {
				// Check if cache is still valid (within TTL, default 5 minutes)
				ttl := 5 * time.Minute
				if time.Now().Unix()-cachedData.LastUpdated < int64(ttl.Seconds()) {
					global.Logger.Debugf("returning cached price for %s: $%.2f",
						tokenConfig.Symbol, cachedData.Price)
					return cachedData.Price, cachedData.Change24h, nil
				}
			}
		}
	}

	// Build CoinGecko API URL with supply data
	// Include total_supply and circulating_supply for accurate market cap calculation
	apiURL := fmt.Sprintf(
		"https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_24hr_change=true&include_market_cap=false",
		tokenConfig.CoinGeckoID)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to create request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to fetch price from CoinGecko")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, 0, errors.Errorf("CoinGecko API returned status %d: %s", resp.StatusCode, string(body))
	}

	var coingeckoResp CoinGeckoResponse
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&coingeckoResp); err != nil {
		return 0, 0, errors.Wrap(err, "failed to decode CoinGecko response")
	}

	// Extract price data (CoinGecko uses the coin ID as the key)
	priceData, ok := coingeckoResp[tokenConfig.CoinGeckoID]
	if !ok {
		return 0, 0, errors.Errorf("price data not found for CoinGecko ID: %s", tokenConfig.CoinGeckoID)
	}

	price := priceData.USD
	change24h := priceData.USD24hChange

	// Cache the result
	if redisClient != nil {
		cacheData := map[string]interface{}{
			"price":        price,
			"change_24h":   change24h,
			"last_updated": time.Now().Unix(),
		}
		cacheJSON, _ := sonic.Marshal(cacheData)
		ttl := 5 * time.Minute
		if err := redisClient.Set(ctx, cacheKey, string(cacheJSON), ttl).Err(); err != nil {
			global.Logger.Warnf("failed to cache price for %s: %v", tokenConfig.Symbol, err)
		}
	}

	global.Logger.Debugf("fetched price from CoinGecko for %s: $%.2f (24h change: %.2f%%)",
		tokenConfig.Symbol, price, change24h)
	return price, change24h, nil
}

// GetTokenPriceFromCustom fetches token price from custom API.
func GetTokenPriceFromCustom(ctx context.Context, tokenConfig *TokenConfig) (float64, float64, error) {
	if tokenConfig.CustomPriceURL == "" {
		return 0, 0, errors.New("custom price URL not configured")
	}

	cacheKey := getCacheKey("price", tokenConfig.Symbol)

	// Check cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cachedPrice, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			var cachedData struct {
				Price       float64 `json:"price"`
				Change24h   float64 `json:"change_24h"`
				LastUpdated int64   `json:"last_updated"`
			}
			if err := sonic.Unmarshal([]byte(cachedPrice), &cachedData); err == nil {
				ttl := 5 * time.Minute
				if time.Now().Unix()-cachedData.LastUpdated < int64(ttl.Seconds()) {
					global.Logger.Debugf("returning cached price for %s: $%.2f",
						tokenConfig.Symbol, cachedData.Price)
					return cachedData.Price, cachedData.Change24h, nil
				}
			}
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", tokenConfig.CustomPriceURL, nil)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to create request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to fetch price from custom API")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, 0, errors.Errorf("custom price API returned status %d: %s", resp.StatusCode, string(body))
	}

	var customResp CustomPriceResponse
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&customResp); err != nil {
		return 0, 0, errors.Wrap(err, "failed to decode custom price response")
	}

	price := customResp.Price
	change24h := customResp.Change24h

	// Cache the result
	if redisClient != nil {
		cacheData := map[string]interface{}{
			"price":        price,
			"change_24h":   change24h,
			"last_updated": time.Now().Unix(),
		}
		cacheJSON, _ := sonic.Marshal(cacheData)
		ttl := 5 * time.Minute
		if err := redisClient.Set(ctx, cacheKey, string(cacheJSON), ttl).Err(); err != nil {
			global.Logger.Warnf("failed to cache price for %s: %v", tokenConfig.Symbol, err)
		}
	}

	global.Logger.Debugf("fetched price from custom API for %s: $%.2f (24h change: %.2f%%)",
		tokenConfig.Symbol, price, change24h)
	return price, change24h, nil
}

// GetTokenPrice fetches token price based on configured price source.
func GetTokenPrice(ctx context.Context, tokenConfig *TokenConfig) (float64, float64, error) {
	priceSource := tokenConfig.PriceSource
	if priceSource == "" {
		priceSource = defaultPriceSource
	}

	switch priceSource {
	case "coingecko":
		return GetTokenPriceFromCoinGecko(ctx, tokenConfig)
	case "custom":
		return GetTokenPriceFromCustom(ctx, tokenConfig)
	default:
		return 0, 0, errors.Errorf("unsupported price source: %s", priceSource)
	}
}

// GetTokenTotalSupplyFromCoinGecko fetches token total supply from CoinGecko API.
// This returns the total supply after accounting for burned tokens.
func GetTokenTotalSupplyFromCoinGecko(ctx context.Context, tokenConfig *TokenConfig) (*big.Int, string, error) {
	if tokenConfig.CoinGeckoID == "" {
		return nil, "", errors.New("CoinGecko ID not configured for this token")
	}

	cacheKey := getCacheKey("supply_coingecko", tokenConfig.Symbol)

	// Check cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cachedSupply, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			var cachedData struct {
				Supply      string `json:"supply"`
				LastUpdated int64  `json:"last_updated"`
			}
			if err := sonic.Unmarshal([]byte(cachedSupply), &cachedData); err == nil {
				ttl := 1 * time.Hour
				if time.Now().Unix()-cachedData.LastUpdated < int64(ttl.Seconds()) {
					supply, ok := new(big.Int).SetString(cachedData.Supply, 10)
					if ok {
						global.Logger.Debugf("returning cached CoinGecko supply for %s: %s",
							tokenConfig.Symbol, cachedData.Supply)
						formatted := formatTokenAmount(supply, tokenConfig.Decimals)
						return supply, formatted, nil
					}
				}
			}
		}
	}

	// Use CoinGecko coins/{id} endpoint to get detailed information including total_supply
	apiURL := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s?localization=false&tickers=false&market_data=true&community_data=false&developer_data=false&sparkline=false",
		tokenConfig.CoinGeckoID)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to create request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to fetch supply from CoinGecko")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", errors.Errorf("CoinGecko API returned status %d: %s", resp.StatusCode, string(body))
	}

	var coinDetailResp struct {
		MarketData struct {
			TotalSupply       *float64 `json:"total_supply"`
			CirculatingSupply *float64 `json:"circulating_supply"`
		} `json:"market_data"`
	}

	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&coinDetailResp); err != nil {
		return nil, "", errors.Wrap(err, "failed to decode CoinGecko detail response")
	}

	// Prefer total_supply, fallback to circulating_supply if total_supply is not available
	var supplyFloat float64
	if coinDetailResp.MarketData.TotalSupply != nil {
		supplyFloat = *coinDetailResp.MarketData.TotalSupply
	} else if coinDetailResp.MarketData.CirculatingSupply != nil {
		supplyFloat = *coinDetailResp.MarketData.CirculatingSupply
		global.Logger.Debugf("total_supply not available for %s, using circulating_supply", tokenConfig.Symbol)
	} else {
		return nil, "", errors.New("neither total_supply nor circulating_supply available from CoinGecko")
	}

	// Convert float64 to big.Int (multiply by 10^decimals to get base units)
	decimals := tokenConfig.Decimals
	if decimals == 0 {
		decimals = defaultDecimals
	}

	// Multiply by 10^decimals to convert to base units
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	supplyBigFloat := new(big.Float).SetFloat64(supplyFloat)
	supplyBigFloat.Mul(supplyBigFloat, multiplier)

	// Convert to big.Int
	supply, _ := supplyBigFloat.Int(nil)
	supplyStr := supply.String()

	formatted := formatTokenAmount(supply, tokenConfig.Decimals)

	// Cache the result
	if redisClient != nil {
		cacheData := map[string]interface{}{
			"supply":       supplyStr,
			"last_updated": time.Now().Unix(),
		}
		cacheJSON, _ := sonic.Marshal(cacheData)
		ttl := 1 * time.Hour
		if err := redisClient.Set(ctx, cacheKey, string(cacheJSON), ttl).Err(); err != nil {
			global.Logger.Warnf("failed to cache CoinGecko supply for %s: %v", tokenConfig.Symbol, err)
		}
	}

	global.Logger.Debugf("fetched total supply from CoinGecko for %s: %s %s",
		tokenConfig.Symbol, formatted, tokenConfig.Symbol)
	return supply, formatted, nil
}

// GetTokenTotalSupply fetches token total supply from the chain.
// This returns the on-chain supply (before accounting for burned tokens).
func GetTokenTotalSupply(ctx context.Context, chainClient chainclient.ChainClientV2, tokenConfig *TokenConfig) (*big.Int, string, error) {
	cacheKey := getCacheKey("supply", tokenConfig.Symbol)

	// Check cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cachedSupply, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			var cachedData struct {
				Supply      string `json:"supply"`
				LastUpdated int64  `json:"last_updated"`
			}
			if err := sonic.Unmarshal([]byte(cachedSupply), &cachedData); err == nil {
				ttl := 1 * time.Hour
				if time.Now().Unix()-cachedData.LastUpdated < int64(ttl.Seconds()) {
					supply, ok := new(big.Int).SetString(cachedData.Supply, 10)
					if ok {
						global.Logger.Debugf("returning cached supply for %s: %s",
							tokenConfig.Symbol, cachedData.Supply)
						formatted := formatTokenAmount(supply, tokenConfig.Decimals)
						return supply, formatted, nil
					}
				}
			}
		}
	}

	// Query total supply from chain using SDK's GetBankSupplyOf method
	// This is more reliable than LCD API as it uses gRPC directly
	if chainClient == nil {
		return nil, "", errors.New("chain client is required for supply query")
	}

	supplyResp, err := chainClient.GetBankSupplyOf(ctx, tokenConfig.Denom)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to get bank supply of %s", tokenConfig.Denom)
	}

	if supplyResp == nil {
		return nil, "", errors.Errorf("empty supply response for denom: %s", tokenConfig.Denom)
	}

	// Convert amount to string, then to big.Int
	// supplyResp.Amount is a types.Coin, and Amount is a types.Int
	supplyStr := supplyResp.Amount.Amount.String()
	if supplyStr == "" || supplyStr == "0" {
		return nil, "", errors.Errorf("invalid supply amount for denom: %s", tokenConfig.Denom)
	}

	supply, ok := new(big.Int).SetString(supplyStr, 10)
	if !ok {
		return nil, "", errors.Errorf("failed to parse supply amount: %s", supplyStr)
	}

	formatted := formatTokenAmount(supply, tokenConfig.Decimals)

	// Cache the result
	if redisClient != nil {
		cacheData := map[string]interface{}{
			"supply":       supplyStr,
			"last_updated": time.Now().Unix(),
		}
		cacheJSON, _ := sonic.Marshal(cacheData)
		ttl := 1 * time.Hour
		if err := redisClient.Set(ctx, cacheKey, string(cacheJSON), ttl).Err(); err != nil {
			global.Logger.Warnf("failed to cache supply for %s: %v", tokenConfig.Symbol, err)
		}
	}

	global.Logger.Debugf("fetched total supply from chain for %s: %s %s",
		tokenConfig.Symbol, formatted, tokenConfig.Symbol)
	return supply, formatted, nil
}

// GetTokenPriceAndMarketCap fetches both price and supply, then calculates market cap.
// It prioritizes CoinGecko's total supply (after burn) over chain supply.
// If supply query fails, it still returns price information with empty supply.
func GetTokenPriceAndMarketCap(ctx context.Context, chainClient chainclient.ChainClientV2, tokenConfig *TokenConfig) (*PriceAndSupplyData, error) {
	// Fetch price and supplies in parallel
	type priceResult struct {
		price     float64
		change24h float64
		err       error
	}
	type supplyResult struct {
		supply       *big.Int
		supplyString string
		err          error
	}

	priceChan := make(chan priceResult, 1)
	coingeckoSupplyChan := make(chan supplyResult, 1)
	chainSupplyChan := make(chan supplyResult, 1)

	// Fetch price
	go func() {
		price, change24h, err := GetTokenPrice(ctx, tokenConfig)
		priceChan <- priceResult{price: price, change24h: change24h, err: err}
	}()

	// Fetch supply from CoinGecko (preferred, includes burned tokens)
	go func() {
		if tokenConfig.PriceSource == "coingecko" && tokenConfig.CoinGeckoID != "" {
			supply, supplyString, err := GetTokenTotalSupplyFromCoinGecko(ctx, tokenConfig)
			coingeckoSupplyChan <- supplyResult{supply: supply, supplyString: supplyString, err: err}
		} else {
			coingeckoSupplyChan <- supplyResult{supply: nil, supplyString: "", err: errors.New("CoinGecko not configured")}
		}
	}()

	// Fetch supply from chain (as fallback and for reference)
	go func() {
		supply, supplyString, err := GetTokenTotalSupply(ctx, chainClient, tokenConfig)
		chainSupplyChan <- supplyResult{supply: supply, supplyString: supplyString, err: err}
	}()

	// Wait for all results
	priceRes := <-priceChan
	coingeckoSupplyRes := <-coingeckoSupplyChan
	chainSupplyRes := <-chainSupplyChan

	// Price is required
	if priceRes.err != nil {
		return nil, errors.Wrap(priceRes.err, "failed to fetch price")
	}

	// Determine which supply to use
	var finalSupply *big.Int
	var finalSupplyString string
	var supplySource string
	var chainSupply *big.Int

	// Prefer CoinGecko supply (after burn) if available
	if coingeckoSupplyRes.err == nil && coingeckoSupplyRes.supply != nil {
		finalSupply = coingeckoSupplyRes.supply
		finalSupplyString = coingeckoSupplyRes.supplyString
		supplySource = "coingecko"
		global.Logger.Debugf("using CoinGecko total supply for %s: %s", tokenConfig.Symbol, finalSupplyString)
	} else if chainSupplyRes.err == nil && chainSupplyRes.supply != nil {
		// Fallback to chain supply
		finalSupply = chainSupplyRes.supply
		finalSupplyString = chainSupplyRes.supplyString
		supplySource = "chain"
		global.Logger.Debugf("using chain supply for %s: %s (CoinGecko unavailable)", tokenConfig.Symbol, finalSupplyString)
	} else {
		// Both failed, return price only
		global.Logger.Warnf("failed to fetch supply for %s (CoinGecko: %v, Chain: %v), returning price only",
			tokenConfig.Symbol, coingeckoSupplyRes.err, chainSupplyRes.err)
		return &PriceAndSupplyData{
			TokenSymbol:          tokenConfig.Symbol,
			TokenName:            tokenConfig.DisplayName,
			PriceUSD:             priceRes.price,
			PriceChange24h:       priceRes.change24h,
			TotalSupply:          nil,
			TotalSupplyString:    "",
			TotalSupplyFromChain: nil,
			LastUpdated:          time.Now().Unix(),
			PriceSource:          tokenConfig.PriceSource,
			SupplySource:         "none",
		}, nil
	}

	// Store chain supply for reference (if available)
	if chainSupplyRes.err == nil && chainSupplyRes.supply != nil {
		chainSupply = chainSupplyRes.supply
	}

	return &PriceAndSupplyData{
		TokenSymbol:          tokenConfig.Symbol,
		TokenName:            tokenConfig.DisplayName,
		PriceUSD:             priceRes.price,
		PriceChange24h:       priceRes.change24h,
		TotalSupply:          finalSupply,
		TotalSupplyString:    finalSupplyString,
		TotalSupplyFromChain: chainSupply,
		LastUpdated:          time.Now().Unix(),
		PriceSource:          tokenConfig.PriceSource,
		SupplySource:         supplySource,
	}, nil
}

// formatTokenAmount formats token amount from base units to human-readable format.
func formatTokenAmount(amount *big.Int, decimals int32) string {
	if decimals <= 0 {
		decimals = defaultDecimals
	}

	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	quotient := new(big.Int).Div(amount, divisor)
	remainder := new(big.Int).Mod(amount, divisor)

	if remainder.Sign() == 0 {
		return quotient.String()
	}

	// Format with decimals
	remainderStr := remainder.String()
	// Pad with zeros if needed
	for len(remainderStr) < int(decimals) {
		remainderStr = "0" + remainderStr
	}
	// Remove trailing zeros
	remainderStr = strings.TrimRight(remainderStr, "0")
	if remainderStr == "" {
		return quotient.String()
	}

	return fmt.Sprintf("%s.%s", quotient.String(), remainderStr)
}

// CalculateMarketCap calculates market cap from price and supply.
func CalculateMarketCap(priceUSD float64, totalSupply *big.Int, decimals int32) float64 {
	if decimals <= 0 {
		decimals = defaultDecimals
	}

	// Convert supply to float64 (with decimals)
	supplyFloat, _ := new(big.Float).SetInt(totalSupply).Float64()
	// Divide by 10^decimals to get token amount
	divisor := 1.0
	for i := int32(0); i < decimals; i++ {
		divisor *= 10
	}
	supplyTokens := supplyFloat / divisor
	// Calculate market cap
	return priceUSD * supplyTokens
}

// LoadTokenConfigFromConf loads token configuration from conf.Bootstrap or global config.
// If bc is nil, it will try to use global.TokenPriceConfig.
func LoadTokenConfigFromConf(bc *conf.Bootstrap, tokenSymbol string) (*TokenConfig, error) {
	var tokenPrice *conf.TokenPrice

	// Try to get config from parameter first, then from global
	if bc != nil && bc.TokenPrice != nil {
		tokenPrice = bc.TokenPrice
	} else if global.TokenPriceConfig != nil {
		tokenPrice = global.TokenPriceConfig
	}

	if tokenPrice == nil {
		// Return default INJ config if no config available
		if tokenSymbol == "" || tokenSymbol == "inj" {
			return &TokenConfig{
				Symbol:      "inj",
				Denom:       "inj",
				CoinGeckoID: "injective-protocol",
				Decimals:    18,
				PriceSource: "coingecko",
				DisplayName: "Injective",
			}, nil
		}
		return nil, errors.Errorf("token configuration not available and no default for: %s", tokenSymbol)
	}

	// If token symbol is empty, use default token
	if tokenSymbol == "" {
		if tokenPrice.DefaultToken != nil {
			return convertTokenConfig(tokenPrice.DefaultToken), nil
		}
		// Fallback to INJ if no default configured
		tokenSymbol = "inj"
	}

	// Check if token is in the tokens map
	if tokenPrice.Tokens != nil {
		if tokenCfg, ok := tokenPrice.Tokens[tokenSymbol]; ok {
			cfg := convertTokenConfig(tokenCfg)
			// Ensure symbol is set
			if cfg.Symbol == "" {
				cfg.Symbol = tokenSymbol
			}
			return cfg, nil
		}
	}

	// If not found and requested token is INJ, return default INJ config
	if tokenSymbol == "inj" {
		return &TokenConfig{
			Symbol:      "inj",
			Denom:       "inj",
			CoinGeckoID: "injective-protocol",
			Decimals:    18,
			PriceSource: "coingecko",
			DisplayName: "Injective",
		}, nil
	}

	return nil, errors.Errorf("token configuration not found for symbol: %s", tokenSymbol)
}

// convertTokenConfig converts conf.TokenConfig to service.TokenConfig.
func convertTokenConfig(cfg *conf.TokenConfig) *TokenConfig {
	if cfg == nil {
		return nil
	}

	decimals := cfg.Decimals
	if decimals == 0 {
		decimals = defaultDecimals
	}

	priceSource := cfg.PriceSource
	if priceSource == "" {
		priceSource = defaultPriceSource
	}

	return &TokenConfig{
		Symbol:         cfg.Symbol,
		Denom:          cfg.Denom,
		CoinGeckoID:    cfg.CoingeckoId,
		Decimals:       decimals,
		PriceSource:    priceSource,
		CustomPriceURL: cfg.CustomPriceUrl,
		DisplayName:    cfg.DisplayName,
	}
}

// GetInjectivePriceFromCoinGecko is a convenience function for backward compatibility.
func GetInjectivePriceFromCoinGecko(ctx context.Context) (float64, float64, error) {
	tokenConfig, err := getTokenConfig("inj")
	if err != nil {
		return 0, 0, err
	}
	return GetTokenPriceFromCoinGecko(ctx, tokenConfig)
}

// GetInjectiveTotalSupply is a convenience function for backward compatibility.
func GetInjectiveTotalSupply(ctx context.Context, chainClient chainclient.ChainClientV2) (*big.Int, string, error) {
	tokenConfig, err := getTokenConfig("inj")
	if err != nil {
		return nil, "", err
	}
	return GetTokenTotalSupply(ctx, chainClient, tokenConfig)
}

// GetInjectivePriceAndMarketCap is a convenience function for backward compatibility.
func GetInjectivePriceAndMarketCap(ctx context.Context, chainClient chainclient.ChainClientV2) (*PriceAndSupplyData, error) {
	tokenConfig, err := getTokenConfig("inj")
	if err != nil {
		return nil, err
	}
	return GetTokenPriceAndMarketCap(ctx, chainClient, tokenConfig)
}
