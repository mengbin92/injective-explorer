// Package main provides a command-line tool for archiving old transactions.
// Usage: go run cmd/archive/main.go [flags]
//
//	-conf: Configuration file path (default: ../../configs)
//	-age-months: Archive transactions older than N months (default: 6, uses config if available)
//	-batch-size: Number of transactions to process per batch (default: 1000, uses config if available)
//	-dry-run: Simulate archiving without actually moving data (default: false)
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

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"go.uber.org/zap"
)

var (
	flagconf    string
	ageMonths   int
	batchSize   int
	dryRun      bool
	showVersion bool
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.IntVar(&ageMonths, "age-months", 0, "archive transactions older than N months (0 = use config default)")
	flag.IntVar(&batchSize, "batch-size", 0, "number of transactions per batch (0 = use config default)")
	flag.BoolVar(&dryRun, "dry-run", false, "simulate archiving without actually moving data")
	flag.BoolVar(&showVersion, "version", false, "show version and exit")
}

func main() {
	flag.Parse()

	if showVersion {
		fmt.Println("injective-explorer archive tool")
		fmt.Println("Version: (built from source)")
		os.Exit(0)
	}

	// Load configuration
	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
			env.NewSource(""),
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

	// Initialize logger
	logger := logger.NewZapLogger(bc.Log)
	logger = logger.With(
		zap.String("tool", "archive"),
	)

	// Initialize global variables (database, storage, etc.)
	global.Init(&bc, logger)
	logHelper := log.NewHelper(logger)

	logHelper.Infof("Starting archive tool...")
	logHelper.Infof("Configuration loaded from: %s", flagconf)

	// Create archive service
	archiveService := service.NewArchiveService()

	// Load archive configuration
	config := service.DefaultArchiveConfig()

	// Override with command-line flags if provided
	if ageMonths > 0 {
		config.ArchiveAgeMonths = ageMonths
		logHelper.Infof("Using command-line age-months: %d", ageMonths)
	} else {
		logHelper.Infof("Using configured age-months: %d", config.ArchiveAgeMonths)
	}

	if batchSize > 0 {
		config.BatchSize = batchSize
		logHelper.Infof("Using command-line batch-size: %d", batchSize)
	} else {
		logHelper.Infof("Using configured batch-size: %d", config.BatchSize)
	}

	config.DryRun = dryRun
	if dryRun {
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
