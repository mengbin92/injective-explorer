// Package server provides HTTP server setup including metrics endpoints.
package server

import (
	"injective-explorer/internal/service/metrics"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RegisterMetricsEndpoint registers Prometheus metrics endpoint.
//
// Parameters:
//   - srv: The HTTP server instance
//   - logger: Logger instance for logging
func RegisterMetricsEndpoint(srv *khttp.Server, logger log.Logger) {
	log.NewHelper(logger).Info("Registering Prometheus metrics endpoint at /metrics")

	r := srv.Route("/")

	// Register /metrics endpoint for Prometheus
	r.GET("/metrics", func(ctx khttp.Context) error {
		// Use promhttp.Handler() to serve Prometheus metrics
		promhttp.Handler().ServeHTTP(ctx.Response(), ctx.Request())
		return nil
	})

	log.NewHelper(logger).Info("Prometheus metrics endpoint registered successfully")
}

// init initializes metrics on package import.
func init() {
	// This ensures metrics are registered when the package is imported
	// The metrics package uses promauto which registers metrics on import
	_ = metrics.SyncDuration
	_ = metrics.SyncErrors
	_ = metrics.SyncOperations
	_ = metrics.DatabaseQueryDuration
	_ = metrics.DatabaseConnections
	_ = metrics.DatabaseConnectionStats
	_ = metrics.CacheHits
	_ = metrics.CacheMisses
	_ = metrics.CacheHitRate
	_ = metrics.CacheTotalRequests
	_ = metrics.BlockSyncHeight
	_ = metrics.TransactionSyncTotal
	_ = metrics.ArchiveOperations
	_ = metrics.ArchiveDuration
	_ = metrics.WorkerPoolQueueSize
	_ = metrics.WorkerPoolActiveWorkers
	_ = metrics.HTTPRequestDuration
	_ = metrics.HTTPRequestTotal
	_ = metrics.HTTPRequestErrors
	_ = metrics.AppGoroutines
	_ = metrics.AppMemoryBytes
	_ = metrics.AppGCStats
	_ = metrics.RedisConnections
}
