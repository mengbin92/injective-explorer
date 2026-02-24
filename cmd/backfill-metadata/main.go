// Package main provides a CLI tool for ERC20 metadata backfill.
// Usage: go run cmd/backfill-metadata/main.go [flags]
//
//	-conf: Configuration file path (default: ../../configs)
//	-limit: Max contracts to process per run (default: 50)
//	-chain-id: Chain ID to backfill (empty = use config default)
//	-delay: Delay between each contract in ms (RPC rate limiting, default: 0)
//	-dry-run: Simulate without actually refreshing metadata (default: false)
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
	limit       int
	chainID     string
	delayMs     int
	dryRun      bool
	showVersion bool
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.IntVar(&limit, "limit", 50, "max contracts to process per run")
	flag.StringVar(&chainID, "chain-id", "", "chain ID to backfill (empty = use config default)")
	flag.IntVar(&delayMs, "delay", 0, "delay between each contract in ms (for RPC rate limiting)")
	flag.BoolVar(&dryRun, "dry-run", false, "simulate without actually refreshing metadata")
	flag.BoolVar(&showVersion, "version", false, "show version and exit")
}

func main() {
	flag.Parse()

	if showVersion {
		fmt.Println("injective-explorer backfill-metadata tool")
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
		zap.String("tool", "backfill-metadata"),
	)

	// Initialize global (database, redis, etc.)
	global.Init(&bc, logger)
	logHelper := log.NewHelper(logger)

	logHelper.Infof("Starting metadata backfill tool...")
	logHelper.Infof("Configuration loaded from: %s", flagconf)
	logHelper.Infof("Options: limit=%d, chain-id=%q, delay=%dms, dry-run=%v", limit, chainID, delayMs, dryRun)

	if dryRun {
		logHelper.Warn("DRY RUN MODE: No metadata will be actually refreshed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	opt := service.BackfillOptions{
		Limit:   limit,
		ChainID: chainID,
		DryRun:  dryRun,
	}
	if delayMs > 0 {
		opt.Delay = time.Duration(delayMs) * time.Millisecond
	}

	startTime := time.Now()
	processed, succeeded, failed := service.RunBackfillMetadata(ctx, opt)
	duration := time.Since(startTime)

	logHelper.Infof("Backfill completed: processed=%d, succeeded=%d, failed=%d, duration=%v",
		processed, succeeded, failed, duration)

	// Update Prometheus metrics (when not dry-run)
	if !dryRun && processed > 0 {
		service.UpdateContractMetadataMetrics(ctx, chainID)
	}

	if failed > 0 && !dryRun {
		logHelper.Warnf("Completed with %d failure(s)", failed)
		os.Exit(1)
	}

	logHelper.Info("Done.")
}
