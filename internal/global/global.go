// Package global provides global variables and initialization functions for the application.
// It manages the logger, chain configuration, and archive configuration that are used throughout the application.
package global

import (
	"injective-explorer/internal/conf"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"
	"context"
	"crypto/tls"

	"github.com/InjectiveLabs/sdk-go/client/common"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/grpc/credentials"
)

var (
	// Logger is the global logger instance used throughout the application.
	Logger *log.Helper
	// ChainConfig is the global chain configuration for the Injective blockchain network.
	ChainConfig common.Network
	// ArchiveConfig is the global archive configuration.
	// It is set during initialization and can be used to access archive configuration values.
	ArchiveConfig *conf.Archive
)

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

	Logger = log.NewHelper(logger)
	Logger.Infof("logger initialized: %v", bc.Log)

	// Initialize chain configuration based on network type
	switch bc.Chain.Network {
	case "mainnet":
		ChainConfig = common.LoadNetwork("mainnet", "lb")
	case "testnet":
		ChainConfig = common.LoadNetwork("testnet", "lb")
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
			ChainTLSCert:            credentials.NewServerTLSFromCert(&tls.Certificate{}),
			ExchangeTLSCert:         credentials.NewServerTLSFromCert(&tls.Certificate{}),
			ExplorerTLSCert:         credentials.NewServerTLSFromCert(&tls.Certificate{}),
			ChainCookieAssistant:    &common.BareMetalLoadBalancedCookieAssistant{},
			ExchangeCookieAssistant: &common.BareMetalLoadBalancedCookieAssistant{},
			ExplorerCookieAssistant: &common.BareMetalLoadBalancedCookieAssistant{},
		}
	}

	Logger.Infof("chain configuration initialized for network: %s", bc.Chain.Network)

	Logger.Infof("database initialized")
	err := db.Init(context.Background(), bc.Data.Database, logger)
	if err != nil {
		panic(err)
	}

	Logger.Infof("database migrated")
	err = db.Get().AutoMigrate(
		&models.Block{},
		&models.Transaction{},
		&models.Account{},
		&models.AccountBalance{},
		&models.TransactionDataArchive{},
	)
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
