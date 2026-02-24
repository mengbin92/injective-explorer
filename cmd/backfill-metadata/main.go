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

// CLIConfig holds all command-line flags for the backfill tool.
type CLIConfig struct {
	ConfPath  string
	Limit     int
	ChainID   string
	DelayMs   int
	DryRun    bool
	ShowVersion bool
}

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.IntVar(&limit, "limit", 50, "max contracts to process per run")
	flag.StringVar(&chainID, "chain-id", "", "chain ID to backfill (empty = use config default)")
	flag.IntVar(&delayMs, "delay", 0, "delay between each contract in ms (for RPC rate limiting)")
	flag.BoolVar(&dryRun, "dry-run", false, "simulate without actually refreshing metadata")
	flag.BoolVar(&showVersion, "version", false, "show version and exit")
}

// parseFlags parses and returns CLI flags.
func parseFlags() *CLIConfig {
	return &CLIConfig{
		ConfPath:    flagconf,
		Limit:       limit,
		ChainID:     chainID,
		DelayMs:     delayMs,
		DryRun:      dryRun,
		ShowVersion: showVersion,
	}
}

// loadConfig loads the application configuration from file and environment.
func loadConfig(confPath string) (*conf.Bootstrap, config.Config, error) {
	c := config.New(
		config.WithSource(
			file.NewSource(confPath),
			env.NewSource(""),
		),
	)

	if err := c.Load(); err != nil {
		c.Close() // Close before exiting
		return nil, nil, err
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		c.Close() // Close before exiting
		return nil, nil, err
	}

	return &bc, c, nil
}

// initApp initializes the application: loads config, sets up logger and global components.
func initApp(confPath string) (*conf.Bootstrap, *log.Helper, config.Config, error) {
	bc, c, err := loadConfig(confPath)
	if err != nil {
		return nil, nil, nil, err
	}

	// Initialize logger
	logger := logger.NewZapLogger(bc.Log)
	logger = logger.With(
		zap.String("tool", "backfill-metadata"),
	)

	// Initialize global (database, redis, etc.)
	global.Init(bc, logger)

	return bc, log.NewHelper(logger), c, nil
}

func main() {
	flag.Parse()

	cliCfg := parseFlags()

	if cliCfg.ShowVersion {
		fmt.Println("injective-explorer backfill-metadata tool")
		fmt.Println("Version: (built from source)")
		os.Exit(0)
	}

	// Initialize application
	_, logHelper, cfg, err := initApp(cliCfg.ConfPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}
	defer cfg.Close() // Ensure config is closed on normal exit

	logHelper.Infof("Starting metadata backfill tool...")
	logHelper.Infof("Configuration loaded from: %s", cliCfg.ConfPath)
	logHelper.Infof("Options: limit=%d, chain-id=%q, delay=%dms, dry-run=%v",
		cliCfg.Limit, cliCfg.ChainID, cliCfg.DelayMs, cliCfg.DryRun)

	if cliCfg.DryRun {
		logHelper.Warn("DRY RUN MODE: No metadata will be actually refreshed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	opt := service.BackfillOptions{
		Limit:   cliCfg.Limit,
		ChainID: cliCfg.ChainID,
		DryRun:  cliCfg.DryRun,
	}
	if cliCfg.DelayMs > 0 {
		opt.Delay = time.Duration(cliCfg.DelayMs) * time.Millisecond
	}

	startTime := time.Now()
	processed, succeeded, failed := service.RunBackfillMetadata(ctx, opt)
	duration := time.Since(startTime)

	logHelper.Infof("Backfill completed: processed=%d, succeeded=%d, failed=%d, duration=%v",
		processed, succeeded, failed, duration)

	// Update Prometheus metrics (when not dry-run)
	if !cliCfg.DryRun && processed > 0 {
		service.UpdateContractMetadataMetrics(ctx, cliCfg.ChainID)
	}

	if failed > 0 && !cliCfg.DryRun {
		logHelper.Warnf("Completed with %d failure(s)", failed)
		os.Exit(1)
	}

	logHelper.Info("Done.")
}
