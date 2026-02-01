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
	"sync"
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

// commonCoinGeckoIDMap maps common token symbols to their CoinGecko IDs.
// This allows tokens not in config to still fetch market data using their symbol.
var commonCoinGeckoIDMap = map[string]string{
	"inj":    "injective-protocol",
	"usdt":   "tether",
	"usdc":   "usd-coin",
	"weth":   "weth",
	"eth":    "ethereum",
	"btc":    "bitcoin",
	"wbbtc":  "wrapped-bitcoin",
	"dai":    "dai",
	"link":   "chainlink",
	"uni":    "uniswap",
	"aave":   "aave",
	"comp":   "compound-governance-token",
	"mkr":    "maker",
	"snx":    "havven",
	"yfi":    "yearn-finance",
	"crv":    "curve-dao-token",
	"1inch":  "1inch",
	"grt":    "the-graph",
	"sushi":  "sushi",
	"band":   "band-protocol",
	"atom":   "cosmos",
	"osmo":   "osmosis",
	"kava":   "kava",
	"scrt":   "secret",
	"xprt":   "persistence",
	"akt":    "akash-network",
	"dvpn":   "sentinel",
	"ion":    "ion",
	"rowan":  "sifchain",
	"ctk":    "certik",
	"btsg":   "bitsong",
	"xki":    "ki-network",
	"umee":   "umee",
}

// GetCoinGeckoIDFromSymbol attempts to get CoinGecko ID from token symbol.
// It first checks the common mapping, then tries to use lowercase symbol.
func GetCoinGeckoIDFromSymbol(symbol string) string {
	if symbol == "" {
		return ""
	}
	lowerSymbol := strings.ToLower(symbol)

	// Check common mapping first
	if id, ok := commonCoinGeckoIDMap[lowerSymbol]; ok {
		return id
	}

	// For some tokens, the CoinGecko ID is the same as the lowercase symbol
	// This works for many tokens like "bitcoin", "ethereum", etc.
	return lowerSymbol
}

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

// TokenMarketData contains market data for a token from CoinGecko.
type TokenMarketData struct {
	PriceUSD              float64
	PriceChange24h        float64
	MarketCapUSD          float64
	TotalVolume24h        float64 // 24h trading volume
	HoldersCount          int64   // Number of holders (optional, not available for all tokens)
	MaxSupply             *big.Int // Max supply (nil if unlimited)
	CirculatingSupply     *big.Int // Circulating supply
	TotalSupply           *big.Int // Total supply
	FullyDilutedMarketCap float64  // Fully diluted market cap (price * max_supply)
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

// GetTokenConfigOrCreate attempts to get token config from global config.
// If not found in config, it creates a default config using the symbol mapping.
func GetTokenConfigOrCreate(tokenSymbol string) *TokenConfig {
	// Try to load from config first
	config, err := LoadTokenConfigFromConf(nil, tokenSymbol)
	if err == nil {
		return config
	}

	// Create a default config using symbol mapping
	coinGeckoID := GetCoinGeckoIDFromSymbol(tokenSymbol)
	if coinGeckoID == "" {
		return nil
	}

	return &TokenConfig{
		Symbol:      strings.ToLower(tokenSymbol),
		Denom:       strings.ToLower(tokenSymbol),
		CoinGeckoID: coinGeckoID,
		Decimals:    defaultDecimals,
		PriceSource: defaultPriceSource,
		DisplayName: strings.ToUpper(tokenSymbol),
	}
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
			TotalVolume       struct {
				USD float64 `json:"usd"`
			} `json:"total_volume"`
			MarketCap struct {
				USD float64 `json:"usd"`
			} `json:"market_cap"`
			Holders *int64 `json:"holders"` // Optional, may not be available for all tokens
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

// GetTokenMarketDataFromCoinGecko fetches comprehensive market data from CoinGecko API.
// This includes price, market cap, 24h volume, and holder count (if available).
func GetTokenMarketDataFromCoinGecko(ctx context.Context, tokenConfig *TokenConfig) (*TokenMarketData, error) {
	if tokenConfig.CoinGeckoID == "" {
		return nil, errors.New("CoinGecko ID not configured for this token")
	}

	cacheKey := getCacheKey("market_data", tokenConfig.Symbol)

	// Check cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cachedData, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil && cachedData != "" {
			var cached TokenMarketData
			if err := sonic.Unmarshal([]byte(cachedData), &cached); err == nil {
				global.Logger.Debugf("returning cached market data for %s", tokenConfig.Symbol)
				return &cached, nil
			}
		}
	}

	// Use CoinGecko coins/{id} endpoint to get detailed market information
	apiURL := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s?localization=false&tickers=false&market_data=true&community_data=false&developer_data=false&sparkline=false",
		tokenConfig.CoinGeckoID)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch market data from CoinGecko")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("CoinGecko API returned status %d: %s", resp.StatusCode, string(body))
	}

	var coinDetailResp struct {
		MarketData struct {
			CurrentPrice struct {
				USD float64 `json:"usd"`
			} `json:"current_price"`
			PriceChange24h float64 `json:"price_change_percentage_24h"`
			MarketCap struct {
				USD float64 `json:"usd"`
			} `json:"market_cap"`
			TotalVolume struct {
				USD float64 `json:"usd"`
			} `json:"total_volume"`
			Holders *int64 `json:"holders"` // Optional, may not be available for all tokens
		} `json:"market_data"`
	}

	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&coinDetailResp); err != nil {
		return nil, errors.Wrap(err, "failed to decode CoinGecko market data response")
	}

	marketData := &TokenMarketData{
		PriceUSD:       coinDetailResp.MarketData.CurrentPrice.USD,
		PriceChange24h: coinDetailResp.MarketData.PriceChange24h,
		MarketCapUSD:   coinDetailResp.MarketData.MarketCap.USD,
		TotalVolume24h: coinDetailResp.MarketData.TotalVolume.USD,
		HoldersCount:   0,
	}

	// Holders count is optional and may not be available for all tokens
	if coinDetailResp.MarketData.Holders != nil {
		marketData.HoldersCount = *coinDetailResp.MarketData.Holders
	}

	// Cache the result
	if redisClient != nil {
		cacheJSON, _ := sonic.Marshal(marketData)
		ttl := 5 * time.Minute
		if err := redisClient.Set(ctx, cacheKey, string(cacheJSON), ttl).Err(); err != nil {
			global.Logger.Warnf("failed to cache market data for %s: %v", tokenConfig.Symbol, err)
		}
	}

	global.Logger.Debugf("fetched market data from CoinGecko for %s: price=$%.2f, volume=$%.2f, holders=%d",
		tokenConfig.Symbol, marketData.PriceUSD, marketData.TotalVolume24h, marketData.HoldersCount)
	return marketData, nil
}

// TokenMarketDataFetcher manages fetching and caching token market data
type TokenMarketDataFetcher struct {
	client      *http.Client
	rateLimiter chan struct{}
}

// NewTokenMarketDataFetcher creates a new fetcher with rate limiting
func NewTokenMarketDataFetcher() *TokenMarketDataFetcher {
	return &TokenMarketDataFetcher{
		client:      &http.Client{Timeout: 30 * time.Second},
		rateLimiter: make(chan struct{}, 5), // Max 5 concurrent requests
	}
}

// BatchGetTokenMarketDataFromCoinGeckoMarkets uses CoinGecko /coins/markets API
// to fetch market data for multiple tokens in a single request (up to 100 tokens per request).
// This is more efficient than individual API calls.
func BatchGetTokenMarketDataFromCoinGeckoMarkets(ctx context.Context, tokenConfigs map[string]*TokenConfig) map[string]*TokenMarketData {
	result := make(map[string]*TokenMarketData)

	// Build CoinGecko ID list and symbol mapping
	coinGeckoIDs := make([]string, 0, len(tokenConfigs))
	symbolToCoinGeckoID := make(map[string]string) // symbol -> coinGeckoID
	coinGeckoIDToSymbol := make(map[string]string) // coinGeckoID -> symbol

	for symbol, config := range tokenConfigs {
		if config != nil && config.CoinGeckoID != "" {
			coinGeckoIDs = append(coinGeckoIDs, config.CoinGeckoID)
			symbolToCoinGeckoID[symbol] = config.CoinGeckoID
			coinGeckoIDToSymbol[config.CoinGeckoID] = symbol
		}
	}

	if len(coinGeckoIDs) == 0 {
		return result
	}

	// Batch process (max 100 IDs per request)
	batchSize := 100
	for i := 0; i < len(coinGeckoIDs); i += batchSize {
		end := i + batchSize
		if end > len(coinGeckoIDs) {
			end = len(coinGeckoIDs)
		}

		batchIDs := coinGeckoIDs[i:end]
		ids := strings.Join(batchIDs, ",")

		// Call batch API
		apiURL := fmt.Sprintf(
			"https://api.coingecko.com/api/v3/coins/markets?ids=%s&vs_currency=usd&order=market_cap_desc&per_page=%d&page=1&sparkline=false&price_change_percentage=24h",
			ids, len(batchIDs),
		)

		client := &http.Client{Timeout: 15 * time.Second}
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			global.Logger.Warnf("failed to create batch market data request: %v", err)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			global.Logger.Warnf("failed to fetch batch market data: %v", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			global.Logger.Warnf("CoinGecko batch API returned status %d: %s", resp.StatusCode, string(body))
			continue
		}

		var markets []struct {
			ID             string  `json:"id"`
			Symbol         string  `json:"symbol"`
			CurrentPrice   float64 `json:"current_price"`
			PriceChange24h float64 `json:"price_change_percentage_24h"`
			MarketCap      float64 `json:"market_cap"`
			TotalVolume    float64 `json:"total_volume"`
			// Note: Holders field is not available in markets API, need separate call
		}

		if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&markets); err != nil {
			global.Logger.Warnf("failed to decode batch market data: %v", err)
			continue
		}

		// Build result map
		for _, market := range markets {
			symbol, ok := coinGeckoIDToSymbol[market.ID]
			if !ok {
				continue
			}

			result[symbol] = &TokenMarketData{
				PriceUSD:       market.CurrentPrice,
				PriceChange24h: market.PriceChange24h,
				MarketCapUSD:   market.MarketCap,
				TotalVolume24h: market.TotalVolume,
				HoldersCount:   0, // Markets API doesn't provide holders data
			}
		}
	}

	return result
}

// BatchGetTokenMarketData fetches market data for multiple tokens.
// It first tries to use the batch /coins/markets API for efficiency,
// then falls back to individual calls for missing tokens or holders data.
func BatchGetTokenMarketData(ctx context.Context, tokenConfigs map[string]*TokenConfig) map[string]*TokenMarketData {
	// Try batch API first
	result := BatchGetTokenMarketDataFromCoinGeckoMarkets(ctx, tokenConfigs)

	// Find missing tokens (not in batch result)
	missingSymbols := make([]string, 0)
	for symbol, config := range tokenConfigs {
		if config == nil || config.CoinGeckoID == "" {
			continue
		}
		if _, ok := result[symbol]; !ok {
			missingSymbols = append(missingSymbols, symbol)
		}
	}

	// For missing tokens, use individual API calls (fallback)
	// Also fetch holders data for tokens that need it
	if len(missingSymbols) > 0 {
		var mu sync.Mutex
		var wg sync.WaitGroup
		maxConcurrency := 5 // Lower concurrency for fallback
		semaphore := make(chan struct{}, maxConcurrency)

		for _, symbol := range missingSymbols {
			config := tokenConfigs[symbol]
			if config == nil {
				continue
			}

			wg.Add(1)
			go func(sym string, cfg *TokenConfig) {
				defer wg.Done()

				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				data, err := GetTokenMarketDataFromCoinGecko(ctx, cfg)
				if err != nil {
					global.Logger.Warnf("failed to get market data for %s (fallback): %v", sym, err)
					return
				}

				mu.Lock()
				result[sym] = data
				mu.Unlock()
			}(symbol, config)
		}

		wg.Wait()
	}

	// For tokens that got data from batch API but need holders count,
	// fetch holders data separately
	fetchAndMergeHoldersData(ctx, result, tokenConfigs)

	return result
}

// fetchAndMergeHoldersData fetches holders count for tokens that need it
// and merges into the existing result map
func fetchAndMergeHoldersData(ctx context.Context, result map[string]*TokenMarketData, tokenConfigs map[string]*TokenConfig) {
	tokensNeedingHolders := make([]string, 0)
	for symbol, data := range result {
		if data != nil && data.HoldersCount == 0 {
			tokensNeedingHolders = append(tokensNeedingHolders, symbol)
		}
	}

	if len(tokensNeedingHolders) == 0 {
		return
	}

	// Try to get from cache first
	for _, symbol := range tokensNeedingHolders {
		config := tokenConfigs[symbol]
		if config == nil {
			continue
		}

		cacheKey := getCacheKey("holders", symbol)
		redisClient := cache.GetRedisClient()
		if redisClient != nil {
			cached, err := redisClient.Get(ctx, cacheKey).Result()
			if err == nil && cached != "" {
				var holdersData struct {
					HoldersCount int64 `json:"holders_count"`
					Timestamp    int64 `json:"timestamp"`
				}
				if err := sonic.Unmarshal([]byte(cached), &holdersData); err == nil {
					if time.Now().Unix()-holdersData.Timestamp < 3600 { // 1 hour cache
						result[symbol].HoldersCount = holdersData.HoldersCount
						continue
					}
				}
			}
		}
	}

	// Fetch holders for remaining tokens in parallel with limited concurrency
	var mu sync.Mutex
	var wg sync.WaitGroup
	maxConcurrency := 3
	semaphore := make(chan struct{}, maxConcurrency)

	for _, symbol := range tokensNeedingHolders {
		config := tokenConfigs[symbol]
		if config == nil {
			continue
		}

		wg.Add(1)
		go func(sym string, cfg *TokenConfig) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			holdersData, err := fetchHoldersData(ctx, cfg)
			if err != nil {
				global.Logger.Debugf("failed to fetch holders for %s: %v", sym, err)
				return
			}

			mu.Lock()
			if result[sym] != nil {
				result[sym].HoldersCount = holdersData.HoldersCount
			}
			mu.Unlock()
		}(symbol, config)
	}

	wg.Wait()
}

// fetchHoldersData fetches holders count from CoinGecko and caches the result
func fetchHoldersData(ctx context.Context, tokenConfig *TokenConfig) (*TokenMarketData, error) {
	if tokenConfig.CoinGeckoID == "" {
		return nil, errors.New("CoinGecko ID not configured")
	}

	cacheKey := getCacheKey("holders", tokenConfig.Symbol)
	redisClient := cache.GetRedisClient()

	// Try cache first
	if redisClient != nil {
		cached, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			var data struct {
				HoldersCount int64 `json:"holders_count"`
				Timestamp    int64 `json:"timestamp"`
			}
			if err := sonic.Unmarshal([]byte(cached), &data); err == nil {
				return &TokenMarketData{HoldersCount: data.HoldersCount}, nil
			}
		}
	}

	// Fetch from CoinGecko
	apiURL := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s?localization=false&tickers=false&market_data=true&community_data=false&developer_data=false&sparkline=false",
		tokenConfig.CoinGeckoID)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("API returned status %d", resp.StatusCode)
	}

	var coinResp struct {
		MarketData struct {
			Holders *int64 `json:"holders"`
		} `json:"market_data"`
	}

	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&coinResp); err != nil {
		return nil, err
	}

	result := &TokenMarketData{
		HoldersCount: 0,
	}
	if coinResp.MarketData.Holders != nil {
		result.HoldersCount = *coinResp.MarketData.Holders
	}

	// Cache result
	if redisClient != nil {
		cacheData := map[string]interface{}{
			"holders_count": result.HoldersCount,
			"timestamp":     time.Now().Unix(),
		}
		cacheJSON, _ := sonic.Marshal(cacheData)
		redisClient.Set(ctx, cacheKey, string(cacheJSON), 30*time.Minute)
	}

	return result, nil
}

// GetTokenFullMarketData fetches complete market data including supply info
func GetTokenFullMarketData(ctx context.Context, tokenConfig *TokenConfig) (*TokenMarketData, error) {
	if tokenConfig.CoinGeckoID == "" {
		return nil, errors.New("CoinGecko ID not configured")
	}

	cacheKey := getCacheKey("full_market", tokenConfig.Symbol)
	redisClient := cache.GetRedisClient()

	// Try cache first
	if redisClient != nil {
		cached, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			var data TokenMarketData
			if err := sonic.Unmarshal([]byte(cached), &data); err == nil {
				return &data, nil
			}
		}
	}

	// Fetch from CoinGecko
	apiURL := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s?localization=false&tickers=false&market_data=true&community_data=false&developer_data=false&sparkline=false",
		tokenConfig.CoinGeckoID)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch from CoinGecko")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var coinResp struct {
		MarketData struct {
			CurrentPrice struct {
				USD float64 `json:"usd"`
			} `json:"current_price"`
			PriceChange24h float64 `json:"price_change_percentage_24h"`
			MarketCap      struct {
				USD float64 `json:"usd"`
			} `json:"market_cap"`
			TotalVolume struct {
				USD float64 `json:"usd"`
			} `json:"total_volume"`
			TotalSupply       *float64 `json:"total_supply"`
			CirculatingSupply *float64 `json:"circulating_supply"`
			MaxSupply         *float64 `json:"max_supply"`
			FullyDilutedValuation struct {
				USD float64 `json:"usd"`
			} `json:"fully_diluted_valuation"`
			Holders *int64 `json:"holders"`
		} `json:"market_data"`
	}

	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&coinResp); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	decimals := tokenConfig.Decimals
	if decimals == 0 {
		decimals = 18
	}

	result := &TokenMarketData{
		PriceUSD:              coinResp.MarketData.CurrentPrice.USD,
		PriceChange24h:        coinResp.MarketData.PriceChange24h,
		MarketCapUSD:          coinResp.MarketData.MarketCap.USD,
		TotalVolume24h:        coinResp.MarketData.TotalVolume.USD,
		FullyDilutedMarketCap: coinResp.MarketData.FullyDilutedValuation.USD,
	}

	// Convert supply values
	if coinResp.MarketData.TotalSupply != nil {
		supply := float64ToBigInt(*coinResp.MarketData.TotalSupply, decimals)
		result.TotalSupply = supply
	}

	if coinResp.MarketData.CirculatingSupply != nil {
		supply := float64ToBigInt(*coinResp.MarketData.CirculatingSupply, decimals)
		result.CirculatingSupply = supply
	}

	if coinResp.MarketData.MaxSupply != nil {
		supply := float64ToBigInt(*coinResp.MarketData.MaxSupply, decimals)
		result.MaxSupply = supply
	}

	if coinResp.MarketData.Holders != nil {
		result.HoldersCount = *coinResp.MarketData.Holders
	}

	// Cache result
	if redisClient != nil {
		cacheJSON, _ := sonic.Marshal(result)
		redisClient.Set(ctx, cacheKey, string(cacheJSON), 5*time.Minute)
	}

	return result, nil
}

// float64ToBigInt converts a float64 value to big.Int with given decimals
func float64ToBigInt(value float64, decimals int32) *big.Int {
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	valueBigFloat := new(big.Float).SetFloat64(value)
	valueBigFloat.Mul(valueBigFloat, multiplier)
	result, _ := valueBigFloat.Int(nil)
	return result
}

// GetNativeTokenPriceFromChain fetches native token price from chain via Exchange gRPC.
// This function queries spot markets to find trading pairs with stablecoins (USDT/USDC)
// and returns the native token price in USD.
//
// Parameters:
//   - ctx: Context for the operation
//   - chainClient: Chain client for querying exchange data
//   - nativeTokenDenom: The denom of the native token (e.g., "inj")
//   - quoteDenoms: List of quote token denoms to search for (e.g., ["usdt", "usdc"])
//
// Returns:
//   - priceUSD: Price in USD (0 if not found)
//   - error: Error if query fails
func GetNativeTokenPriceFromChain(ctx context.Context, chainClient chainclient.ChainClientV2, nativeTokenDenom string, quoteDenoms []string) (float64, error) {
	if chainClient == nil {
		return 0, errors.New("chain client is required")
	}

	if nativeTokenDenom == "" {
		return 0, errors.New("native token denom is required")
	}

	// Default quote denoms if not provided
	if len(quoteDenoms) == 0 {
		quoteDenoms = []string{"usdt", "usdc", "usd"}
	}

	cacheKey := fmt.Sprintf("price:chain:%s", nativeTokenDenom)

	// Check cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cachedPrice, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			var cachedData struct {
				Price       float64 `json:"price"`
				LastUpdated int64   `json:"last_updated"`
			}
			if err := sonic.Unmarshal([]byte(cachedPrice), &cachedData); err == nil {
				ttl := 1 * time.Minute // Cache for 1 minute
				if time.Now().Unix()-cachedData.LastUpdated < int64(ttl.Seconds()) {
					global.Logger.Debugf("returning cached chain price for %s: $%.2f",
						nativeTokenDenom, cachedData.Price)
					return cachedData.Price, nil
				}
			}
		}
	}

	// Try to query spot markets from chain
	// Note: This requires the chain to have Exchange module and spot markets configured
	// For now, we'll return an error indicating this feature needs Exchange gRPC implementation
	// The actual implementation would use chainClient to query spot markets like:
	// markets, err := chainClient.GetSpotMarkets(ctx, ...)
	// Then find markets where base_denom == nativeTokenDenom and quote_denom in quoteDenoms
	// Finally, get the latest price from the market

	global.Logger.Warnf("GetNativeTokenPriceFromChain: Exchange gRPC query not yet implemented. "+
		"Native token: %s, Quote denoms: %v", nativeTokenDenom, quoteDenoms)

	// TODO: Implement Exchange gRPC query for spot market prices
	// This would involve:
	// 1. Query spot markets: chainClient.GetSpotMarkets(ctx)
	// 2. Filter markets by base_denom (nativeTokenDenom) and quote_denom (in quoteDenoms)
	// 3. Get market summary or orderbook to extract latest price
	// 4. Convert price to USD (if quote is stablecoin, use 1:1; otherwise need conversion)

	return 0, errors.New("chain price query not yet implemented - requires Exchange gRPC integration")
}
