// Package main provides a command-line tool for archiving old transactions and blocks.
// Usage: go run cmd/archive/main.go [flags]
//
//	-conf: Configuration file path (default: ../../configs)
//	-type: Type to archive: "transactions", "blocks", or "all" (default: "transactions")
//	-age-months: Archive transactions older than N months (default: 6, uses config if available)
//	-batch-size: Number of transactions to process per batch (default: 1000, uses config if available)
//	-dry-run: Simulate archiving without actually moving data (default: false)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
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
	archiveType string
	ageMonths   int
	batchSize   int
	dryRun      bool
	showVersion bool
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.StringVar(&archiveType, "type", "transactions", "archive type: transactions, blocks, or all (default: transactions)")
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

	// Validate archive type
	if archiveType != "transactions" && archiveType != "blocks" && archiveType != "all" {
		logHelper.Errorf("Invalid archive type: %s (must be 'transactions', 'blocks', or 'all')", archiveType)
		os.Exit(1)
	}

	// Display configuration summary
	logHelper.Infof("Archive configuration:")
	logHelper.Infof("  - Archive type: %s", archiveType)
	logHelper.Infof("  - Age months: %d", config.ArchiveAgeMonths)
	logHelper.Infof("  - Batch size: %d", config.BatchSize)
	logHelper.Infof("  - Dry run: %v", config.DryRun)
	logHelper.Infof("  - Use object storage: %v", config.UseObjectStorage)
	logHelper.Infof("  - Storage threshold: %d bytes", config.StorageThreshold)
	logHelper.Info("===========================================")

	var totalErrors int
	var blockMetrics *service.BlockArchiveMetrics

	// When archiving both, run in parallel for better performance
	if archiveType == "all" {
		logHelper.Info("🚀 Running both transaction and block archiving in parallel...")
		logHelper.Info("")

		var wg sync.WaitGroup
		var txErr, blockErr error

		// Run transaction archiving in parallel
		wg.Add(1)
		go func() {
			defer wg.Done()
			txCtx, txCancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer txCancel()
			startTime := time.Now()
			err := archiveService.ArchiveOldTransactions(txCtx, config)
			if err != nil {
				logHelper.Errorf("❌ Transaction archiving failed: %v", err)
				txErr = err
			} else {
				duration := time.Since(startTime)
				logHelper.Info("✅ Transaction Archiving Completed!")
				logHelper.Infof("  Duration:            %v", duration)
				logHelper.Info("")
			}
		}()

		// Run block archiving in parallel
		wg.Add(1)
		go func() {
			defer wg.Done()
			blockCtx, blockCancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer blockCancel()
			startTime := time.Now()
			metrics, err := archiveService.ArchiveOldBlocks(blockCtx, config)
			if err != nil {
				logHelper.Errorf("❌ Block archiving failed: %v", err)
				blockErr = err
			} else {
				duration := time.Since(startTime)
				logHelper.Info("✅ Block Archiving Completed!")
				logHelper.Infof("  Total Blocks:        %d", metrics.TotalBlocks)
				logHelper.Infof("  Processed:           %d", metrics.ProcessedBlocks)
				logHelper.Infof("  Failed:              %d", metrics.FailedBlocks)
				logHelper.Infof("  Compressed:          %d", metrics.CompressedBlocks)
				logHelper.Infof("  Uploaded to Storage: %d", metrics.UploadedToStorageBlocks)
				logHelper.Infof("  Duration:            %v", duration)
				blockMetrics = metrics
			}
		}()

		// Wait for both operations to complete
		wg.Wait()

		// Count errors
		if txErr != nil {
			totalErrors++
		}
		if blockErr != nil {
			totalErrors++
		}

		// Show archive progress after both complete
		if blockMetrics != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			archived, total, _ := archiveService.GetArchiveProgress(ctx)
			logHelper.Infof("  Archive Progress:    %d / %d (%.2f%%)",
				archived, total, float64(archived)*100/float64(total))
			logHelper.Info("")
		}
	} else {
		// Archive only blocks if requested (sequential execution)
		if archiveType == "blocks" {
			logHelper.Info("📦 Starting Block Archiving...")
			logHelper.Info("")

			blockCtx, blockCancel := context.WithTimeout(context.Background(), 30*time.Minute)
			startTime := time.Now()
			metrics, err := archiveService.ArchiveOldBlocks(blockCtx, config)
			blockCancel()
			duration := time.Since(startTime)

			if err != nil {
				logHelper.Errorf("❌ Block archiving failed: %v", err)
				totalErrors++
			} else {
				logHelper.Info("✅ Block Archiving Completed!")
				logHelper.Infof("  Total Blocks:        %d", metrics.TotalBlocks)
				logHelper.Infof("  Processed:           %d", metrics.ProcessedBlocks)
				logHelper.Infof("  Failed:              %d", metrics.FailedBlocks)
				logHelper.Infof("  Compressed:          %d", metrics.CompressedBlocks)
				logHelper.Infof("  Uploaded to Storage: %d", metrics.UploadedToStorageBlocks)
				logHelper.Infof("  Duration:            %v", duration)

				// Show archive progress
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				archived, total, _ := archiveService.GetArchiveProgress(ctx)
				logHelper.Infof("  Archive Progress:    %d / %d (%.2f%%)",
					archived, total, float64(archived)*100/float64(total))
				logHelper.Info("")
			}
		}

		// Archive only transactions if requested (sequential execution)
		if archiveType == "transactions" {
			logHelper.Info("💰 Starting Transaction Archiving...")
			logHelper.Info("")

			txCtx, txCancel := context.WithTimeout(context.Background(), 30*time.Minute)
			startTime := time.Now()
			err := archiveService.ArchiveOldTransactions(txCtx, config)
			txCancel()
			duration := time.Since(startTime)

			if err != nil {
				logHelper.Errorf("❌ Transaction archiving failed: %v", err)
				totalErrors++
			} else {
				logHelper.Info("✅ Transaction Archiving Completed!")
				logHelper.Infof("  Duration:            %v", duration)
				logHelper.Info("")
			}
		}
	}

	// Final summary
	logHelper.Info("===========================================")
	if totalErrors > 0 {
		logHelper.Warnf("⚠️  Completed with %d error(s)", totalErrors)
		os.Exit(1)
	} else {
		logHelper.Info("✅ All operations completed successfully!")
		logHelper.Infof("Finished at %s", time.Now().Format(time.RFC3339))
	}
}
