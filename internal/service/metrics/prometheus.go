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

	// DatabaseConnectionStats tracks database connection pool statistics.
	DatabaseConnectionStats = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "database_connection_stats",
			Help: "Database connection pool statistics",
		},
		[]string{"stat_type"},
	)

	// RedisConnections tracks Redis connection pool metrics.
	RedisConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_connections",
			Help: "Redis connection pool metrics",
		},
		[]string{"state"},
	)

	// OnChainPriceUSD tracks the current on-chain price in USD.
	OnChainPriceUSD = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "onchain_price_usd",
			Help: "Current on-chain price in USD",
		},
		[]string{"symbol", "denom"},
	)

	// OnChainPriceConfidence tracks the confidence level of on-chain price.
	OnChainPriceConfidence = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "onchain_price_confidence",
			Help: "Confidence level of on-chain price (0-1)",
		},
		[]string{"symbol"},
	)

	// OnChainPriceSyncDuration tracks the duration of price sync operations.
	OnChainPriceSyncDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "onchain_price_sync_duration_seconds",
			Help:    "Duration of on-chain price sync operations",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"operation", "status"},
	)

	// OnChainPriceSyncErrors tracks the number of price sync errors.
	OnChainPriceSyncErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "onchain_price_sync_errors_total",
			Help: "Total number of on-chain price sync errors",
		},
		[]string{"operation", "error_type"},
	)

	// OnChainPriceAPICalls tracks the number of price API calls.
	OnChainPriceAPICalls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "onchain_price_api_calls_total",
			Help: "Total number of on-chain price API calls",
		},
		[]string{"endpoint", "status"},
	)

	// ContractMetadataFetchTotal tracks ERC20 metadata fetch attempts.
	ContractMetadataFetchTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "contract_metadata_fetch_total",
			Help: "Total number of ERC20 metadata fetch attempts",
		},
		[]string{"status", "error_type"},
	)

	// ContractMetadataFetchDuration tracks metadata fetch duration.
	ContractMetadataFetchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "contract_metadata_fetch_duration_seconds",
			Help:    "Duration of ERC20 metadata fetch in seconds",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
		},
		[]string{"source"},
	)

	// ContractsMissingMetadata tracks contracts with incomplete metadata.
	ContractsMissingMetadata = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "contracts_missing_metadata_total",
			Help: "Number of contracts with incomplete metadata",
		},
	)

	// ContractsNotERC20 tracks contracts marked as not ERC20.
	ContractsNotERC20 = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "contracts_not_erc20_total",
			Help: "Number of contracts marked as not ERC20",
		},
	)

	// PeggyMetadataFetchTotal tracks Peggy token metadata fetch by source.
	PeggyMetadataFetchTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "peggy_metadata_fetch_total",
			Help: "Total number of Peggy token metadata fetch attempts",
		},
		[]string{"source", "status"},
	)

	// PeggyMetadataFetchDuration tracks duration of Peggy token metadata fetch by source.
	PeggyMetadataFetchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "peggy_metadata_fetch_duration_seconds",
			Help:    "Duration of Peggy token metadata fetch by source",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
		},
		[]string{"source"},
	)

	// PeggyTokensDetected counts total Peggy tokens detected/processed.
	PeggyTokensDetected = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "peggy_tokens_detected_total",
			Help: "Total number of Peggy tokens detected",
		},
	)

	// EthereumRPCEndpointHealth tracks health of Ethereum RPC endpoints (1=healthy, 0=unhealthy).
	EthereumRPCEndpointHealth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ethereum_rpc_endpoint_health",
			Help: "Health status of Ethereum RPC endpoints for Peggy metadata (1=healthy, 0=unhealthy)",
		},
		[]string{"endpoint"},
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

// SetDatabaseConnectionStats sets database connection pool statistics.
func SetDatabaseConnectionStats(statType string, value float64) {
	DatabaseConnectionStats.WithLabelValues(statType).Set(value)
}

// SetRedisConnections sets Redis connection pool metrics.
func SetRedisConnections(state string, count float64) {
	RedisConnections.WithLabelValues(state).Set(count)
}

// SetOnChainPrice sets the current on-chain price metric.
func SetOnChainPrice(symbol, denom string, price float64) {
	OnChainPriceUSD.WithLabelValues(symbol, denom).Set(price)
}

// SetOnChainPriceConfidence sets the on-chain price confidence metric.
func SetOnChainPriceConfidence(symbol string, confidence float64) {
	OnChainPriceConfidence.WithLabelValues(symbol).Set(confidence)
}

// RecordOnChainPriceSyncDuration records the duration of a price sync operation.
func RecordOnChainPriceSyncDuration(operation string, status string, duration float64) {
	OnChainPriceSyncDuration.WithLabelValues(operation, status).Observe(duration)
}

// RecordOnChainPriceSyncError records a price sync error.
func RecordOnChainPriceSyncError(operation string, errorType string) {
	OnChainPriceSyncErrors.WithLabelValues(operation, errorType).Inc()
}

// RecordOnChainPriceAPICall records an on-chain price API call.
func RecordOnChainPriceAPICall(endpoint string, status string) {
	OnChainPriceAPICalls.WithLabelValues(endpoint, status).Inc()
}

// RecordContractMetadataFetch records an ERC20 metadata fetch attempt.
func RecordContractMetadataFetch(status, errorType string) {
	ContractMetadataFetchTotal.WithLabelValues(status, errorType).Inc()
}

// RecordContractMetadataFetchDuration records metadata fetch duration.
func RecordContractMetadataFetchDuration(source string, duration float64) {
	ContractMetadataFetchDuration.WithLabelValues(source).Observe(duration)
}

// SetContractsMissingMetadata sets the count of contracts missing metadata.
func SetContractsMissingMetadata(count float64) {
	ContractsMissingMetadata.Set(count)
}

// SetContractsNotERC20 sets the count of contracts marked as not ERC20.
func SetContractsNotERC20(count float64) {
	ContractsNotERC20.Set(count)
}

// RecordPeggyMetadataFetch records a Peggy token metadata fetch.
func RecordPeggyMetadataFetch(source, status string) {
	PeggyMetadataFetchTotal.WithLabelValues(source, status).Inc()
}

// RecordPeggyMetadataFetchDuration records duration of Peggy token metadata fetch by source.
func RecordPeggyMetadataFetchDuration(source string, duration float64) {
	PeggyMetadataFetchDuration.WithLabelValues(source).Observe(duration)
}

// RecordPeggyTokensDetected increments the count of Peggy tokens detected.
func RecordPeggyTokensDetected() {
	PeggyTokensDetected.Inc()
}

// SetEthereumEndpointHealth sets health status for an Ethereum RPC endpoint (1=healthy, 0=unhealthy).
func SetEthereumEndpointHealth(endpoint string, healthy bool) {
	v := 0.0
	if healthy {
		v = 1.0
	}
	EthereumRPCEndpointHealth.WithLabelValues(endpoint).Set(v)
}
