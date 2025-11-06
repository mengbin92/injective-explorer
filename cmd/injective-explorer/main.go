// Package main is the entry point of the injective-explorer application.
// It initializes configuration, logging, and starts the gRPC and HTTP servers.
package main

import (
	"flag"
	"fmt"
	"os"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/service"
	"injective-explorer/provider/logger"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"

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
func newApp(logger log.Logger, gs *grpc.Server, hs *http.Server) *kratos.App {
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

	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
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
