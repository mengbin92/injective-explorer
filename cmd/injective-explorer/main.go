// Package main is the entry point of the injective-explorer application.
// It initializes configuration, logging, and starts the gRPC and HTTP servers.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/service"
	"injective-explorer/internal/service/metrics"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"
	"injective-explorer/provider/logger"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	sdk "github.com/cosmos/cosmos-sdk/types"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"
)

// Build-time variables set via ldflags.
// Example: go build -ldflags "-X main.Version=x.y.z -X main.Name=injective-explorer"
var (
	// Name is the name of the compiled software.
	Name string
	// Version is the version of the compiled software.
	Version string
	// flagconf is the config flag for specifying the configuration file path.
	flagconf string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")

	// Configure Cosmos SDK Bech32 prefixes for INJ chain
	initSDKConfig()
}

// initSDKConfig initializes the Cosmos SDK configuration with INJ chain prefixes
func initSDKConfig() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("inj", "injpub")
	config.SetBech32PrefixForValidator("injvaloper", "injvaloperpub")
	config.SetBech32PrefixForConsensusNode("injvalcons", "injvalconspub")
	config.Seal()
}

// newApp creates a new kratos application with the given logger and servers.
//
// Parameters:
//   - logger: The logger instance for application logging
//   - gs: The gRPC server instance
//   - hs: The HTTP server instance
//
// Returns:
//   - *kratos.App: A configured kratos application ready to run
func newApp(logger log.Logger, gs *grpc.Server, hs *khttp.Server) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			gs,
			hs,
		),
	)
}

// main is the entry point of the application.
// It loads configuration, initializes global variables, sets up logging,
// and starts the application servers.
//
// The function will panic if critical initialization steps fail:
//   - Configuration loading fails
//   - Application wiring fails
//   - Application startup fails
func main() {
	flag.Parse()

	// Configure configuration sources with priority order
	// Priority: env.NewSource (highest) > file.NewSource (lowest)
	// This means environment variables will override values from config file
	//
	// Service enable/disable flags can be configured via config.yaml (service.*) or environment variables:
	//   - SERVICE_ENABLE_PPROF: Enable pprof profiling (default: false)
	//   - SERVICE_PPROF_ADDR: pprof endpoint address (default: localhost:6060)
	//   - SERVICE_ENABLE_SYNC: Enable blockchain sync service (default: true)
	//   - SERVICE_ENABLE_TOKEN_TRANSFER_SYNC: Enable token transfer sync (default: true)
	//   - SERVICE_ENABLE_TOKEN_MARKET_CACHE_SYNC: Enable token market cache sync (default: true)
	//
	// Kratos config system also supports environment variable override using dot-notation:
	//   - SERVER_HTTP_ADDR overrides server.http.addr
	//   - DATA_DATABASE_SOURCE overrides data.database.source
	c := config.New(
		config.WithSource(
			file.NewSource(flagconf), // Load config file first (lower priority)
			env.NewSource(""),        // Environment variables override file config (higher priority)
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to scan configuration: %v\n", err)
		os.Exit(1)
	}

	logger := logger.NewZapLogger(bc.Log)

	// Add fixed fields to logger
	// Note: trace.id and span.id are not added here as they are context-dependent
	// and should be added via middleware during request processing
	logger = logger.With(
		zap.String("service.id", id),
		zap.String("service.name", Name),
		zap.String("service.version", Version),
	)

	// Initialize global variables
	global.Init(&bc, logger)

	// Enable pprof in development/staging (only if explicitly enabled via config)
	// In production, this should be disabled for security reasons
	if bc.Service != nil && bc.Service.EnablePprof {
		pprofAddr := bc.Service.PprofAddr
		if pprofAddr == "" {
			pprofAddr = "localhost:6060"
		}
		if err := service.EnablePprof(pprofAddr); err != nil {
			log.NewHelper(logger).Warnf("Failed to enable pprof: %v", err)
		} else {
			log.NewHelper(logger).Infof("pprof enabled on %s", pprofAddr)
		}
	}

	// Start blockchain sync service (only if enabled via config)
	// In horizontal scaling scenarios, only one instance should run sync service
	if bc.Service == nil || bc.Service.EnableSync {
		syncConfig := service.LoadSyncConfig(&bc)
		syncService := service.NewSyncService(syncConfig, &bc)

		// Try to acquire distributed lock for sync service
		// This ensures only one instance runs sync even if multiple instances have enable_sync=true
		if acquired, err := syncService.TryAcquireSyncLock(context.Background()); err != nil {
			log.NewHelper(logger).Warnf("Failed to check sync lock, starting sync service anyway: %v", err)
			// Continue to start sync service if lock check fails (graceful degradation)
		} else if !acquired {
			log.NewHelper(logger).Infof("Sync service lock already acquired by another instance, skipping sync service startup")
			// Don't start sync service if lock is already held
		} else {
			// Lock acquired, start sync service
			if err := syncService.Start(); err != nil {
				log.NewHelper(logger).Errorf("Failed to start sync service: %v", err)
				os.Exit(1)
			}
			defer func() {
				if err := syncService.Stop(); err != nil {
					log.NewHelper(logger).Errorf("Failed to stop sync service: %v", err)
				}
			}()
		}
	} else {
		log.NewHelper(logger).Infof("Sync service disabled via service.enable_sync=false")
	}

	// Start token transfer sync service (if enabled via config)
	if bc.Service == nil || bc.Service.EnableTokenTransferSync {
		tokenTransferSyncConfig := service.LoadTokenTransferSyncConfig(&bc)
		tokenTransferSyncService, err := service.NewTokenTransferSyncService(context.Background(), tokenTransferSyncConfig, &bc)
		if err != nil {
			log.NewHelper(logger).Warnf("Failed to create token transfer sync service: %v", err)
		} else {
			tokenTransferSyncService.Start()
			defer func() {
				if err := tokenTransferSyncService.Stop(); err != nil {
					log.NewHelper(logger).Errorf("Failed to stop token transfer sync service: %v", err)
				}
			}()
			log.NewHelper(logger).Infof("Token transfer sync service started")
		}
	} else {
		log.NewHelper(logger).Infof("Token transfer sync service disabled via service.enable_token_transfer_sync=false")
	}

	// Start token market cache sync service (if enabled via config)
	if bc.Service == nil || bc.Service.EnableTokenMarketCacheSync {
		marketCacheSyncConfig := service.LoadTokenMarketCacheSyncConfig(&bc)
		tokenMarketCacheSyncService := service.NewTokenMarketCacheSyncService(context.Background(), marketCacheSyncConfig)
		tokenMarketCacheSyncService.Start()
		defer func() {
			if err := tokenMarketCacheSyncService.Stop(); err != nil {
				log.NewHelper(logger).Errorf("Failed to stop token market cache sync service: %v", err)
			}
		}()
		log.NewHelper(logger).Infof("Token market cache sync service started")
	} else {
		log.NewHelper(logger).Infof("Token market cache sync service disabled via service.enable_token_market_cache_sync=false")
	}

	// Start archive scheduler service (if enabled)
	if bc.Archive != nil && bc.Archive.GetEnabled() {
		archiveSchedulerService := service.LoadArchiveSchedulerConfig(&bc)
		if err := archiveSchedulerService.Start(); err != nil {
			log.NewHelper(logger).Errorf("Failed to start archive scheduler service: %v", err)
			os.Exit(1)
		}
		defer func() {
			if err := archiveSchedulerService.Stop(); err != nil {
				log.NewHelper(logger).Errorf("Failed to stop archive scheduler service: %v", err)
			}
		}()
		log.NewHelper(logger).Infof("Archive scheduler service enabled and started")
	} else {
		log.NewHelper(logger).Infof("Archive scheduler service is disabled (set archive.enabled=true to enable)")
	}

	app, cleanup, err := wireApp(bc.Server, bc.Data, logger, &bc)
	if err != nil {
		log.NewHelper(logger).Errorf("Failed to wire application: %v", err)
		os.Exit(1)
	}
	defer cleanup()

	// Initialize metrics collector with proper context cancellation
	// This replaces the old metrics goroutine to prevent goroutine leaks
	var sqlDB *sql.DB
	if gdb := db.Get(); gdb != nil {
		sqlDB, _ = gdb.DB()
	}
	redisClient := cache.GetRedisClient()

	// Initialize global metrics collector with database and Redis clients
	// Default interval: 15 seconds (same as before)
	metrics.InitGlobalCollector(sqlDB, redisClient, 15*time.Second)
	defer metrics.StopGlobalCollector()

	// Start and wait for stop signal
	if err := app.Run(); err != nil {
		log.NewHelper(logger).Errorf("Application run error: %v", err)
		os.Exit(1)
	}
}
