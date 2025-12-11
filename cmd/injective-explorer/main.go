// Package main is the entry point of the injective-explorer application.
// It initializes configuration, logging, and starts the gRPC and HTTP servers.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/service"
	"injective-explorer/provider/logger"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

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
	// archiveMode enables archive mode (runs archive and exits)
	archiveMode bool
	// archiveAgeMonths is the age in months for archiving (only used in archive mode)
	archiveAgeMonths int
	// archiveBatchSize is the batch size for archiving (only used in archive mode)
	archiveBatchSize int
	// archiveDryRun enables dry run mode for archiving (only used in archive mode)
	archiveDryRun bool

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.BoolVar(&archiveMode, "archive", false, "run archive mode (archive old transactions and exit)")
	flag.IntVar(&archiveAgeMonths, "archive-age-months", 0, "archive transactions older than N months (0 = use config default, only used with -archive)")
	flag.IntVar(&archiveBatchSize, "archive-batch-size", 0, "number of transactions per batch (0 = use config default, only used with -archive)")
	flag.BoolVar(&archiveDryRun, "archive-dry-run", false, "simulate archiving without actually moving data (only used with -archive)")
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
	// Note: Kratos config system supports environment variable override using dot-notation:
	//   - SERVER_HTTP_ADDR overrides server.http.addr
	//   - DATA_DATABASE_SOURCE overrides data.database.source
	//   - REDIS_ADDR overrides data.redis.addr
	//
	// The YAML placeholder syntax (${VAR:default}) in config.yaml may not be supported
	// by Kratos file source. Use environment variables with dot-notation instead.
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

	// Handle archive mode
	if archiveMode {
		runArchiveMode(&bc, logger)
		return
	}

	// Enable pprof in development/staging (only if explicitly enabled via environment variable)
	// In production, this should be disabled for security reasons
	if os.Getenv("ENABLE_PPROF") == "true" {
		if err := service.EnablePprof("localhost:6060"); err != nil {
			log.NewHelper(logger).Warnf("Failed to enable pprof: %v", err)
		} else {
			log.NewHelper(logger).Infof("pprof enabled on localhost:6060")
		}
	}

	// Start blockchain sync service
	syncConfig := service.LoadSyncConfig(&bc)
	syncService := service.NewSyncService(syncConfig)
	if err := syncService.Start(); err != nil {
		log.NewHelper(logger).Errorf("Failed to start sync service: %v", err)
		os.Exit(1)
	}
	defer func() {
		if err := syncService.Stop(); err != nil {
			log.NewHelper(logger).Errorf("Failed to stop sync service: %v", err)
		}
	}()

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

	app, cleanup, err := wireApp(bc.Server, bc.Data, logger)
	if err != nil {
		log.NewHelper(logger).Errorf("Failed to wire application: %v", err)
		os.Exit(1)
	}
	defer cleanup()

	// Start and wait for stop signal
	if err := app.Run(); err != nil {
		log.NewHelper(logger).Errorf("Application run error: %v", err)
		os.Exit(1)
	}
}

// runArchiveMode runs the archive process and exits.
// This mode is triggered by the -archive flag.
func runArchiveMode(bc *conf.Bootstrap, logger log.Logger) {
	logHelper := log.NewHelper(logger)
	logHelper.Infof("Running in archive mode...")

	// Create archive service
	archiveService := service.NewArchiveService()

	// Load archive configuration
	config := service.DefaultArchiveConfig()

	// Override with command-line flags if provided
	if archiveAgeMonths > 0 {
		config.ArchiveAgeMonths = archiveAgeMonths
		logHelper.Infof("Using command-line age-months: %d", archiveAgeMonths)
	} else {
		logHelper.Infof("Using configured age-months: %d", config.ArchiveAgeMonths)
	}

	if archiveBatchSize > 0 {
		config.BatchSize = archiveBatchSize
		logHelper.Infof("Using command-line batch-size: %d", archiveBatchSize)
	} else {
		logHelper.Infof("Using configured batch-size: %d", config.BatchSize)
	}

	config.DryRun = archiveDryRun
	if archiveDryRun {
		logHelper.Warnf("DRY RUN MODE: No data will be actually archived")
	}

	// Display configuration summary
	logHelper.Infof("Archive configuration:")
	logHelper.Infof("  - Age months: %d", config.ArchiveAgeMonths)
	logHelper.Infof("  - Batch size: %d", config.BatchSize)
	logHelper.Infof("  - Dry run: %v", config.DryRun)
	logHelper.Infof("  - Use object storage: %v", config.UseObjectStorage)
	logHelper.Infof("  - Storage threshold: %d bytes", config.StorageThreshold)

	// Create context with timeout (1 hour should be enough for most cases)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	// Execute archiving
	startTime := time.Now()
	logHelper.Infof("Starting archive process at %s", startTime.Format(time.RFC3339))

	err := archiveService.ArchiveOldTransactions(ctx, config)
	if err != nil {
		logHelper.Errorf("Archive failed: %v", err)
		os.Exit(1)
	}

	duration := time.Since(startTime)
	logHelper.Infof("Archive completed successfully in %v", duration)
	logHelper.Infof("Finished at %s", time.Now().Format(time.RFC3339))
}
