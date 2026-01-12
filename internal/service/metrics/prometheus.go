// Package metrics provides Prometheus metrics for monitoring operations.
package metrics

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Note: promauto automatically registers metrics with the default registry
// when the package is imported. This ensures metrics are available at /metrics endpoint.

var (
	// HTTPRequestDuration tracks HTTP request duration.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10},
		},
		[]string{"method", "endpoint", "status"},
	)

	// HTTPRequestTotal tracks total HTTP requests.
	HTTPRequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	// HTTPRequestErrors tracks HTTP request errors.
	HTTPRequestErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_errors_total",
			Help: "Total number of HTTP request errors",
		},
		[]string{"method", "endpoint", "error_type"},
	)

	// SyncDuration tracks the duration of sync operations.
	SyncDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sync_duration_seconds",
			Help:    "Duration of sync operations in seconds",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
		},
		[]string{"operation", "status"},
	)

	// SyncErrors tracks the number of sync errors.
	SyncErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sync_errors_total",
			Help: "Total number of sync errors",
		},
		[]string{"operation", "error_type"},
	)

	// SyncOperations tracks the number of sync operations.
	SyncOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sync_operations_total",
			Help: "Total number of sync operations",
		},
		[]string{"operation", "status"},
	)

	// DatabaseQueryDuration tracks database query duration.
	DatabaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_query_duration_seconds",
			Help:    "Duration of database queries in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
		},
		[]string{"operation", "table"},
	)

	// DatabaseConnections tracks database connection pool metrics.
	DatabaseConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "database_connections",
			Help: "Current number of database connections",
		},
		[]string{"state"},
	)

	// CacheHits tracks cache hits.
	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	// CacheMisses tracks cache misses.
	CacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	// CacheHitRate tracks cache hit rate as percentage (0-100).
	CacheHitRate = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cache_hit_rate",
			Help: "Cache hit rate as percentage (0-100)",
		},
		[]string{"cache_type"},
	)

	// CacheTotalRequests tracks total cache requests by result type.
	CacheTotalRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_total_requests",
			Help: "Total number of cache requests by result type",
		},
		[]string{"cache_type", "result"},
	)

	// BlockSyncHeight tracks the latest synced block height.
	BlockSyncHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "block_sync_height",
			Help: "Latest synced block height",
		},
	)

	// TransactionSyncTotal tracks total synced transactions.
	TransactionSyncTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "transaction_sync_total",
			Help: "Total number of synced transactions",
		},
		[]string{"status"},
	)

	// ArchiveOperations tracks archive operations.
	ArchiveOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "archive_operations_total",
			Help: "Total number of archive operations",
		},
		[]string{"operation", "status"},
	)

	// ArchiveDuration tracks archive operation duration.
	ArchiveDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "archive_duration_seconds",
			Help:    "Duration of archive operations in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800},
		},
		[]string{"operation"},
	)

	// WorkerPoolQueueSize tracks worker pool queue size.
	WorkerPoolQueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "worker_pool_queue_size",
			Help: "Current size of worker pool queue",
		},
		[]string{"pool_name"},
	)

	// WorkerPoolActiveWorkers tracks active workers.
	WorkerPoolActiveWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "worker_pool_active_workers",
			Help: "Number of active workers in the pool",
		},
		[]string{"pool_name"},
	)

	// AppGoroutines tracks the number of goroutines.
	AppGoroutines = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "app_goroutines",
			Help: "Number of goroutines that currently exist",
		},
	)

	// AppMemoryBytes tracks memory usage by memory type.
	AppMemoryBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_memory_bytes",
			Help: "Memory usage in bytes by type",
		},
		[]string{"type"},
	)

	// AppGCStats tracks garbage collection statistics by metric type.
	AppGCStats = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_gc_stats",
			Help: "Garbage collection statistics by metric type",
		},
		[]string{"type"},
	)
)

// RecordHTTPRequestDuration records the duration of an HTTP request.
func RecordHTTPRequestDuration(method, endpoint, status string, duration float64) {
	HTTPRequestDuration.WithLabelValues(method, endpoint, status).Observe(duration)
}

// RecordHTTPRequest records an HTTP request.
func RecordHTTPRequest(method, endpoint, status string) {
	HTTPRequestTotal.WithLabelValues(method, endpoint, status).Inc()
}

// RecordHTTPError records an HTTP error.
func RecordHTTPError(method, endpoint, errorType string) {
	HTTPRequestErrors.WithLabelValues(method, endpoint, errorType).Inc()
}

// RecordSyncDuration records the duration of a sync operation.
func RecordSyncDuration(operation string, status string, duration float64) {
	SyncDuration.WithLabelValues(operation, status).Observe(duration)
}

// RecordSyncError records a sync error.
func RecordSyncError(operation string, errorType string) {
	SyncErrors.WithLabelValues(operation, errorType).Inc()
}

// RecordSyncOperation records a sync operation.
func RecordSyncOperation(operation string, status string) {
	SyncOperations.WithLabelValues(operation, status).Inc()
}

// RecordDatabaseQueryDuration records database query duration.
func RecordDatabaseQueryDuration(operation string, table string, duration float64) {
	DatabaseQueryDuration.WithLabelValues(operation, table).Observe(duration)
}

// SetDatabaseConnections sets the number of database connections.
func SetDatabaseConnections(state string, count float64) {
	DatabaseConnections.WithLabelValues(state).Set(count)
}

// RecordCacheHit records a cache hit.
func RecordCacheHit(cacheType string) {
	CacheHits.WithLabelValues(cacheType).Inc()
	CacheTotalRequests.WithLabelValues(cacheType, "hit").Inc()
}

// RecordCacheMiss records a cache miss.
func RecordCacheMiss(cacheType string) {
	CacheMisses.WithLabelValues(cacheType).Inc()
	CacheTotalRequests.WithLabelValues(cacheType, "miss").Inc()
}

// SetCacheHitRate sets the cache hit rate.
func SetCacheHitRate(cacheType string, rate float64) {
	CacheHitRate.WithLabelValues(cacheType).Set(rate)
}

// SetBlockSyncHeight sets the latest synced block height.
func SetBlockSyncHeight(height float64) {
	BlockSyncHeight.Set(height)
}

// RecordTransactionSync records a synced transaction.
func RecordTransactionSync(status string) {
	TransactionSyncTotal.WithLabelValues(status).Inc()
}

// RecordArchiveOperation records an archive operation.
func RecordArchiveOperation(operation string, status string) {
	ArchiveOperations.WithLabelValues(operation, status).Inc()
}

// RecordArchiveDuration records archive operation duration.
func RecordArchiveDuration(operation string, duration float64) {
	ArchiveDuration.WithLabelValues(operation).Observe(duration)
}

// SetWorkerPoolQueueSize sets the worker pool queue size.
func SetWorkerPoolQueueSize(poolName string, size float64) {
	WorkerPoolQueueSize.WithLabelValues(poolName).Set(size)
}

// SetWorkerPoolActiveWorkers sets the number of active workers.
func SetWorkerPoolActiveWorkers(poolName string, count float64) {
	WorkerPoolActiveWorkers.WithLabelValues(poolName).Set(count)
}

// RecordRuntimeMetrics records Go runtime metrics including goroutines and memory.
// Should be called periodically (e.g., every 15 seconds) to track resource usage.
func RecordRuntimeMetrics() {
	// Number of goroutines
	AppGoroutines.Set(float64(runtime.NumGoroutine()))

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	AppMemoryBytes.WithLabelValues("heap_alloc").Set(float64(m.HeapAlloc))
	AppMemoryBytes.WithLabelValues("heap_sys").Set(float64(m.HeapSys))
	AppMemoryBytes.WithLabelValues("stack_inuse").Set(float64(m.StackInuse))
	AppMemoryBytes.WithLabelValues("sys_total").Set(float64(m.Sys))

	// GC stats
	AppGCStats.WithLabelValues("num_gc").Set(float64(m.NumGC))
	AppGCStats.WithLabelValues("pause_total_ns").Set(float64(m.PauseTotalNs))
}
