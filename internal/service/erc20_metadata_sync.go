// Package service provides ERC20 metadata sync logic including Peggy tokens support.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/internal/service/metrics"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
)

const (
	defaultEthereumRPC   = "https://eth.llamarpc.com"
	defaultCoinGeckoURL  = "https://api.coingecko.com/api/v3"
	coinGeckoRequestPath = "/coins/ethereum/contract/%s"
	peggyPrefix          = "peggy"
)

// MetadataFetchError represents a classified metadata fetch error.
type MetadataFetchError struct {
	Type        string
	Err         error
	ShouldRetry bool
}

// validateERC20Metadata validates metadata and returns normalized values.
func validateERC20Metadata(name, symbol string, decimals int) (string, string, error) {
	if name == "" || symbol == "" {
		return "", "", errors.New("empty name or symbol")
	}
	if strings.Contains(name, "\x00") || strings.Contains(symbol, "\x00") {
		return "", "", errors.New("invalid null character in metadata")
	}
	if len(name) > 255 {
		name = name[:255]
	}
	if len(symbol) > 50 {
		symbol = symbol[:50]
	}
	if strings.EqualFold(name, symbol) {
		global.Logger.Warnf("suspicious metadata: name and symbol are identical: %s", name)
	}
	symbol = strings.ToUpper(symbol)
	if decimals < 0 || decimals > 18 {
		global.Logger.Warnf("unusual decimals: %d (expected 0-18)", decimals)
	}
	return name, symbol, nil
}

// classifyError classifies fetch errors for retry/mark decisions.
func classifyError(err error) *MetadataFetchError {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "context deadline exceeded") || strings.Contains(errStr, "DeadlineExceeded"):
		return &MetadataFetchError{Type: "rpc_timeout", Err: err, ShouldRetry: true}
	case strings.Contains(errStr, "execution reverted") || strings.Contains(errStr, "no data"):
		return &MetadataFetchError{Type: "contract_not_erc20", Err: err, ShouldRetry: false}
	case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "dial tcp"):
		return &MetadataFetchError{Type: "rpc_connection_failed", Err: err, ShouldRetry: true}
	case strings.Contains(errStr, "ABI") || strings.Contains(errStr, "decode"):
		return &MetadataFetchError{Type: "invalid_response", Err: err, ShouldRetry: false}
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests") || strings.Contains(errStr, "429"):
		return &MetadataFetchError{Type: "rate_limit", Err: err, ShouldRetry: true}
	default:
		return &MetadataFetchError{Type: "unknown", Err: err, ShouldRetry: false}
	}
}

// markContractNotERC20 marks a contract as not supporting ERC20 metadata.
func markContractNotERC20(contractAddress, chainID string) error {
	result := db.Get().Model(&models.ContractInfo{}).
		Where("contract_address = ? AND chain_id = ?", contractAddress, chainID).
		Update("metadata_not_applicable", true)
	if result.Error != nil {
		return errors.Wrap(result.Error, "failed to mark contract as not ERC20")
	}
	global.Logger.Infof("marked contract contract_address=%s (chain=%s) as not ERC20", strings.ToLower(contractAddress), chainID)
	return nil
}

// getEthereumRpcEndpoints returns the list of RPC endpoints (multi-active or single).
func getEthereumRpcEndpoints(bc *conf.Bootstrap) []string {
	cfg := getEthereumConfig(bc)
	if cfg == nil {
		return nil
	}
	if len(cfg.RpcEndpoints) > 0 {
		return cfg.RpcEndpoints
	}
	if cfg.RpcEndpoint != "" {
		return []string{cfg.RpcEndpoint}
	}
	return nil
}

// createEthereumEVMClient creates an EVM client for Ethereum mainnet (for Peggy metadata).
// Uses first endpoint from rpc_endpoints or rpc_endpoint.
func createEthereumEVMClient(bc *conf.Bootstrap) (*chain.EVMClient, error) {
	endpoints := getEthereumRpcEndpoints(bc)
	if len(endpoints) == 0 {
		return nil, errors.New("Ethereum RPC not configured or disabled")
	}
	cfg := getEthereumConfig(bc)
	chainID := big.NewInt(cfg.ChainId)
	if chainID.Int64() == 0 {
		chainID = big.NewInt(1)
	}
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = cfg.Timeout.AsDuration()
	}
	return chain.NewEVMClientWithConfig(&chain.EVMConfig{
		RPCURL:  endpoints[0],
		ChainID: chainID,
		Network: "ethereum",
		Timeout: timeout,
	})
}

func getEthereumConfig(bc *conf.Bootstrap) *conf.Chain_Ethereum {
	if bc == nil || bc.Chain == nil {
		return nil
	}
	return bc.Chain.Ethereum
}

// fetchMetadataFromEthereum fetches ERC20 metadata from Ethereum RPC.
func (s *SyncService) fetchMetadataFromEthereum(ctx context.Context, erc20Address string) (name, symbol string, decimals int, err error) {
	return fetchMetadataFromEthereumWithConfig(ctx, erc20Address, s.bc)
}

// fetchMetadataFromEthereumWithConfig fetches ERC20 metadata from Ethereum RPC (standalone).
// When rpc_endpoints is configured, tries each endpoint in order (failover).
func fetchMetadataFromEthereumWithConfig(ctx context.Context, erc20Address string, bc *conf.Bootstrap) (name, symbol string, decimals int, err error) {
	endpoints := getEthereumEndpointsOrderedByHealth(bc)
	if len(endpoints) == 0 {
		return "", "", 0, errors.New("Ethereum RPC not configured or disabled")
	}
	addr := strings.TrimPrefix(erc20Address, "0x")
	if !strings.HasPrefix(addr, "0x") {
		addr = "0x" + addr
	} else {
		addr = erc20Address
	}
	cfg := getEthereumConfig(bc)
	chainID := big.NewInt(cfg.ChainId)
	if chainID.Int64() == 0 {
		chainID = big.NewInt(1)
	}
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = cfg.Timeout.AsDuration()
	}
	var lastErr error
	for i, rpcURL := range endpoints {
		client, dialErr := chain.NewEVMClientWithConfig(&chain.EVMConfig{
			RPCURL:  rpcURL,
			ChainID: chainID,
			Network: "ethereum",
			Timeout: timeout,
		})
		if dialErr != nil {
			global.Logger.Debugf("Ethereum RPC endpoint %d (%s) dial failed: %v", i+1, rpcURL, dialErr)
			lastErr = dialErr
			continue
		}
		name, symbol, decimals, fetchErr := client.GetERC20Metadata(ctx, addr)
		client.Close()
		if fetchErr == nil {
			return name, symbol, decimals, nil
		}
		global.Logger.Debugf("Ethereum RPC endpoint %d (%s) fetch failed: %v", i+1, rpcURL, fetchErr)
		lastErr = fetchErr
	}
	return "", "", 0, errors.Wrapf(lastErr, "all %d Ethereum RPC endpoints failed", len(endpoints))
}

// fetchMetadataFromCoinGecko fetches token metadata from CoinGecko API.
func (s *SyncService) fetchMetadataFromCoinGecko(ctx context.Context, contractAddress string) (name, symbol string, decimals int, err error) {
	return fetchMetadataFromCoinGeckoWithConfig(ctx, contractAddress, s.getCoinGeckoConfig())
}

// fetchMetadataFromCoinGeckoWithConfig fetches token metadata from CoinGecko (standalone).
func fetchMetadataFromCoinGeckoWithConfig(ctx context.Context, contractAddress string, cfg *conf.Chain_CoinGecko) (name, symbol string, decimals int, err error) {
	if cfg == nil || !cfg.Enabled {
		return "", "", 0, errors.New("CoinGecko not enabled")
	}
	addr := strings.TrimPrefix(strings.ToLower(contractAddress), "0x")
	baseURL := cfg.BaseUrl
	if baseURL == "" {
		baseURL = defaultCoinGeckoURL
	}
	url := fmt.Sprintf("%s%s", strings.TrimSuffix(baseURL, "/"), fmt.Sprintf(coinGeckoRequestPath, addr))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", 0, errors.Wrap(err, "failed to create request")
	}
	if cfg.ApiKey != "" {
		req.Header.Set("x-cg-demo-api-key", cfg.ApiKey)
	}
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = cfg.Timeout.AsDuration()
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, errors.Wrap(err, "failed to fetch from CoinGecko")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", 0, errors.Errorf("CoinGecko returned status %d", resp.StatusCode)
	}
	var data struct {
		Name              string `json:"name"`
		Symbol            string `json:"symbol"`
		DetailPlatforms   struct {
			Ethereum struct {
				DecimalPlace int `json:"decimal_place"`
			} `json:"ethereum"`
		} `json:"detail_platforms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", 0, errors.Wrap(err, "failed to decode CoinGecko response")
	}
	symbol = strings.ToUpper(data.Symbol)
	decimals = data.DetailPlatforms.Ethereum.DecimalPlace
	if decimals == 0 {
		decimals = 18
	}
	return data.Name, symbol, decimals, nil
}

func (s *SyncService) getCoinGeckoConfig() *conf.Chain_CoinGecko {
	return getCoinGeckoConfigFromBootstrap(s.bc)
}

func getCoinGeckoConfigFromBootstrap(bc *conf.Bootstrap) *conf.Chain_CoinGecko {
	if bc == nil || bc.Chain == nil {
		return nil
	}
	return bc.Chain.Coingecko
}

// extractSymbolFromDenom extracts a symbol from bank_denom for Peggy tokens.
func extractSymbolFromDenom(bankDenom string) string {
	denom := strings.TrimPrefix(bankDenom, peggyPrefix)
	if strings.HasPrefix(denom, "0x") {
		return "PEGGY"
	}
	if len(denom) > 0 && !strings.HasPrefix(denom, "0x") {
		return strings.ToUpper(denom)
	}
	return "PEGGY"
}

// getPeggyTokenDefaults returns default metadata for Peggy tokens when all sources fail.
func getPeggyTokenDefaults(bankDenom string) (name, symbol string, decimals int) {
	denom := strings.TrimPrefix(bankDenom, peggyPrefix)
	if strings.Contains(strings.ToLower(denom), "usdt") {
		return "Tether USD", "USDT", 6
	}
	if strings.Contains(strings.ToLower(denom), "usdc") {
		return "USD Coin", "USDC", 6
	}
	return "Bridge Token", extractSymbolFromDenom(bankDenom), 18
}

// isERC20Contract performs a lightweight symbol() check to detect ERC20 support.
// When true, caller can proceed with GetERC20Metadata. When false, saves 2 RPC (name, decimals) for non-ERC20 contracts.
// Design doc 4.6.2: optional pre-check; enable via sync.erc20_pre_check when non-ERC20 contracts are common.
func isERC20Contract(ctx context.Context, evmClient *chain.EVMClient, contractAddress string) bool {
	symbol, err := evmClient.GetERC20Symbol(ctx, contractAddress)
	return err == nil && symbol != "" && symbol != "0x"
}

// fetchPeggyTokenMetadata fetches metadata for Peggy tokens with fallback chain.
func (s *SyncService) fetchPeggyTokenMetadata(ctx context.Context, bankDenom, _ string) (name, symbol string, decimals int, metadataSource string, err error) {
	return fetchPeggyTokenMetadataWithConfig(ctx, bankDenom, s.bc)
}

// fetchPeggyTokenMetadataWithConfig fetches metadata for Peggy tokens (standalone).
func fetchPeggyTokenMetadataWithConfig(ctx context.Context, bankDenom string, bc *conf.Bootstrap) (name, symbol string, decimals int, metadataSource string, err error) {
	metrics.RecordPeggyTokensDetected()

	originalERC20Address := strings.TrimPrefix(bankDenom, peggyPrefix)
	if !strings.HasPrefix(originalERC20Address, "0x") {
		originalERC20Address = "0x" + originalERC20Address
	}

	// 1. Try Ethereum RPC (P0)
	if bc != nil && bc.Chain != nil && bc.Chain.Ethereum != nil && bc.Chain.Ethereum.Enabled && bc.Chain.Ethereum.RpcEndpoint != "" {
		start := time.Now()
		name, symbol, decimals, err = fetchMetadataFromEthereumWithConfig(ctx, originalERC20Address, bc)
		metrics.RecordPeggyMetadataFetchDuration("ethereum", time.Since(start).Seconds())
		if err == nil && name != "" && symbol != "" {
			metrics.RecordPeggyMetadataFetch("ethereum", "success")
			global.Logger.Infof("fetched Peggy token metadata from Ethereum: %s (%s)", name, symbol)
			return name, symbol, decimals, "ethereum", nil
		}
		metrics.RecordPeggyMetadataFetch("ethereum", "failure")
		global.Logger.Debugf("failed to fetch from Ethereum: %v", err)
	}

	// 2. Try CoinGecko (P1)
	cfg := getCoinGeckoConfigFromBootstrap(bc)
	if cfg != nil && cfg.Enabled {
		start := time.Now()
		name, symbol, decimals, err = fetchMetadataFromCoinGeckoWithConfig(ctx, originalERC20Address, cfg)
		metrics.RecordPeggyMetadataFetchDuration("coingecko", time.Since(start).Seconds())
		if err == nil && name != "" && symbol != "" {
			metrics.RecordPeggyMetadataFetch("coingecko", "success")
			global.Logger.Infof("fetched Peggy token metadata from CoinGecko: %s (%s)", name, symbol)
			return name, symbol, decimals, "coingecko", nil
		}
		metrics.RecordPeggyMetadataFetch("coingecko", "failure")
		global.Logger.Debugf("failed to fetch from CoinGecko: %v", err)
	}

	// 3. Default values (P2)
	metrics.RecordPeggyMetadataFetch("manual", "success")
	metrics.RecordPeggyMetadataFetchDuration("manual", 0)
	name, symbol, decimals = getPeggyTokenDefaults(bankDenom)
	global.Logger.Infof("using default Peggy token metadata for %s: %s (%s)", bankDenom, name, symbol)
	return name, symbol, decimals, "manual", nil
}

// RefreshContractMetadataStandalone refreshes ERC20 metadata for a contract (for use by Admin API).
// Uses global.BootstrapConfig if bc is nil.
// forceRefresh: when true, bypasses cache and deletes existing cache (Admin API refresh).
func RefreshContractMetadataStandalone(ctx context.Context, contractAddress, chainID string, bc *conf.Bootstrap, forceRefresh bool) error {
	if bc == nil {
		bc = global.BootstrapConfig
	}
	if bc == nil {
		return errors.New("bootstrap config required for metadata refresh")
	}
	syncSvc := &SyncService{bc: bc}
	return syncSvc.fetchAndUpdateERC20Metadata(ctx, contractAddress, chainID, "", forceRefresh)
}
