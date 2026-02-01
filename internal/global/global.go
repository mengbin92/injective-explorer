// Package global provides global variables and initialization functions for the application.
// It manages the logger, chain configuration, and archive configuration that are used throughout the application.
package global

import (
	"injective-explorer/internal/conf"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"
	"context"

	"github.com/InjectiveLabs/sdk-go/client/common"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// Logger is the global logger instance used throughout the application.
	Logger *log.Helper
	// ChainConfig is the global chain configuration for the Injective blockchain network.
	ChainConfig common.Network
	// NetworkName is the network name ("mainnet" or "testnet").
	NetworkName string
	// ArchiveConfig is the global archive configuration.
	// It is set during initialization and can be used to access archive configuration values.
	ArchiveConfig *conf.Archive
	// TokenPriceConfig is the global token price service configuration.
	// It is set during initialization and can be used to access token price configuration.
	TokenPriceConfig *conf.TokenPrice
	// EVMConfig is the global EVM RPC configuration.
	// It is set during initialization and can be used to access EVM configuration values.
	EVMConfig *conf.Chain_EVM
	// APIManagementConfig is the global API management configuration.
	// It is set during initialization and can be used to access API management settings.
	APIManagementConfig *conf.APIManagement
)

// QuoteTokenConfig holds the configuration for quote tokens used in on-chain price fetching.
type QuoteTokenConfig struct {
	USDTAddress   string
	USDCAddress   string
	QuotePriority []string
}

// DefaultQuoteTokens provides default quote token configuration.
var DefaultQuoteTokens = QuoteTokenConfig{
	USDTAddress: "peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5",
	USDCAddress: "peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
	QuotePriority: []string{
		"peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5",
		"peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
	},
}

// Init initializes global variables including the logger, chain configuration, and archive configuration.
// It loads the chain configuration based on the network type specified in the bootstrap configuration.
// It also saves the archive configuration globally for use by archive services.
//
// Parameters:
//   - bc: The bootstrap configuration containing chain, log, data, and archive settings
//   - logger: The logger instance to use for application logging
//
// The function supports three network types:
//   - "mainnet": Uses predefined mainnet configuration
//   - "testnet": Uses predefined testnet configuration
//   - Other: Uses custom configuration from the bootstrap config
func Init(bc *conf.Bootstrap, logger log.Logger) {
	if bc == nil {
		panic("bootstrap config cannot be nil")
	}

	// Save archive config globally for later use
	ArchiveConfig = bc.GetArchive()

	// Save token price config globally for later use
	TokenPriceConfig = bc.GetTokenPrice()

	// Save EVM config globally for later use
	EVMConfig = bc.Chain.Evm

	// Save API management config globally for later use
	APIManagementConfig = bc.GetApiManagement()

	Logger = log.NewHelper(logger)
	Logger.Infof("logger initialized: %v", bc.Log)

	// Initialize chain configuration based on network type
	switch bc.Chain.Network {
	case "mainnet":
		ChainConfig = common.LoadNetwork("mainnet", "lb")
		NetworkName = "mainnet"
	case "testnet":
		ChainConfig = common.LoadNetwork("testnet", "lb")
		NetworkName = "testnet"
	default:
		// Custom network configuration
		ChainConfig = common.Network{
			LcdEndpoint:             bc.Chain.LcdEndpoint,
			TmEndpoint:              bc.Chain.TmEndpoint,
			ChainGrpcEndpoint:       bc.Chain.ChainGrpcEndpoint,
			ChainStreamGrpcEndpoint: bc.Chain.ChainStreamGrpcEndpoint,
			ExchangeGrpcEndpoint:    bc.Chain.ExchangeGrpcEndpoint,
			ExplorerGrpcEndpoint:    bc.Chain.ExplorerGrpcEndpoint,
			ChainId:                 bc.Chain.ChainId,
			FeeDenom:                bc.Chain.FeeDenom,
			Name:                    bc.Chain.Name,
			OfficialTokensListURL:   bc.Chain.OfficialTokensListUrl,
			ChainTLSCert:            insecure.NewCredentials(),
			ExchangeTLSCert:         insecure.NewCredentials(),
			ExplorerTLSCert:         insecure.NewCredentials(),
			ChainCookieAssistant:    &common.BareMetalLoadBalancedCookieAssistant{},
			ExchangeCookieAssistant: &common.BareMetalLoadBalancedCookieAssistant{},
			ExplorerCookieAssistant: &common.BareMetalLoadBalancedCookieAssistant{},
		}
		// Infer network name from chain ID for custom networks
		switch bc.Chain.ChainId {
		case "injective-1":
			NetworkName = "mainnet"
		case "injective-888":
			NetworkName = "testnet"
		default:
			NetworkName = bc.Chain.Network // Use configured network name
		}
	}

	Logger.Infof("database initialized")
	err := db.Init(context.Background(), bc.Data.Database, logger)
	if err != nil {
		panic(err)
	}

	err = cache.InitRedis(context.Background(), bc.Data.Redis, logger)
	if err != nil {
		panic(err)
	}

	Logger.Infof("object storage initialized")
	if bc.Data != nil {
		err = storage.Init(context.Background(), bc.Data.GetObjectStorage(), logger)
		if err != nil {
			panic(err)
		}
	}
}

// GetQuoteTokenConfig returns the quote token configuration for on-chain price fetching.
// It retrieves the configuration from TokenPriceConfig and falls back to default values
// if not configured.
//
// Returns:
//   - QuoteTokenConfig: The configuration containing USDT/USDC contract addresses and
//     quote token priority list. If TokenPriceConfig is nil or OnChain config is not set,
//     returns DefaultQuoteTokens.
//
// The function ensures that empty values are replaced with defaults:
//   - USDTAddress defaults to "peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5"
//   - USDCAddress defaults to "peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
//   - QuotePriority defaults to [USDTAddress, USDCAddress]
//
// Example usage:
//
//	config := GetQuoteTokenConfig()
//	usdtAddr := config.USDTAddress
//	priority := config.QuotePriority
func GetQuoteTokenConfig() QuoteTokenConfig {
	if TokenPriceConfig == nil || TokenPriceConfig.OnChain == nil {
		return DefaultQuoteTokens
	}

	onChain := TokenPriceConfig.OnChain
	config := QuoteTokenConfig{
		USDTAddress:   onChain.UsdtAddress,
		USDCAddress:   onChain.UsdcAddress,
		QuotePriority: onChain.QuotePriority,
	}

	// Use defaults if not configured
	if config.USDTAddress == "" {
		config.USDTAddress = DefaultQuoteTokens.USDTAddress
	}
	if config.USDCAddress == "" {
		config.USDCAddress = DefaultQuoteTokens.USDCAddress
	}
	if len(config.QuotePriority) == 0 {
		config.QuotePriority = DefaultQuoteTokens.QuotePriority
	}

	return config
}
