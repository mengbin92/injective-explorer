// Package constants provides centralized constant definitions for the entire application.
// This package contains all configurable limits, thresholds, and quotas used across
// different modules to ensure consistency and ease of maintenance.
package constants

import "time"

// ============== Validator Limits ==============
// Constants limiting the size of validator data structures to prevent
// unbounded memory growth.
const (
	// MaxUptimesEntries limits the number of uptime records stored per validator.
	// Only the most recent N uptime entries are kept to prevent memory bloat.
	// With ~2 hour block times and 1000 entries, this covers ~83 days of history.
	MaxUptimesEntries = 1000

	// MaxSlashingEventsEntries limits the number of slashing events stored per validator.
	// Slashing events are rare, so 100 entries is more than sufficient.
	MaxSlashingEventsEntries = 100
)

// ============== Cache Limits ==============
// Constants for cache operation limits and concurrency control.
const (
	// MaxConcurrentCacheWrites is the maximum number of concurrent cache write operations.
	// Reduced from 100 to 50 to minimize memory usage during high load.
	MaxConcurrentCacheWrites = 50

	// MaxCacheItemSize is the maximum size in bytes for a single cached item.
	// Items larger than this will not be cached to prevent memory bloat.
	MaxCacheItemSize = 1 * 1024 * 1024 // 1 MB

	// CacheWorkerPoolSize is the size of the worker pool for async cache operations.
	CacheWorkerPoolSize = MaxConcurrentCacheWrites
)

// ============== Memory Management ==============
// Constants for memory management and GC triggering thresholds.
const (
	// MemoryGCThreshold is the heap allocation threshold in bytes that triggers
	// automatic garbage collection to proactively manage memory and prevent bloat.
	MemoryGCThreshold = 500 * 1024 * 1024 // 500 MB

	// MemoryWarningThreshold is the heap allocation threshold in bytes that
	// triggers a warning log entry for monitoring purposes.
	MemoryWarningThreshold = 1024 * 1024 * 1024 // 1 GB

	// GCCooldownPeriod is the minimum time between forced GC operations to prevent
	// excessive GC cycles that could impact performance.
	GCCooldownPeriod = 30 * time.Second
)

// ============== Database Connection Pool ==============
// Constants for database connection pool configuration.
const (
	// MaxIdleConns is the maximum number of idle database connections.
	// Reduced from 10 to 5 to minimize memory usage.
	MaxIdleConns = 5

	// MaxOpenConns is the maximum number of open database connections.
	// Reduced from 100 to 20 to minimize memory usage.
	MaxOpenConns = 20

	// ConnMaxLifetime is the maximum duration a database connection can be kept open.
	ConnMaxLifetime = 30 * time.Minute

	// ConnMaxIdleTime is the maximum duration a connection can remain idle before being closed.
	ConnMaxIdleTime = 10 * time.Minute
)

// ============== Batch Processing ==============
// Constants for batch processing to limit memory usage.
const (
	// StakingInfoBatchSize is the number of addresses to process per batch
	// in the staking info sync to avoid loading all addresses at once.
	StakingInfoBatchSize = 1000

	// DelegationSaveBatchSize is the number of delegations to accumulate
	// before saving to storage to prevent excessive memory accumulation.
	DelegationSaveBatchSize = 100
)

// ============== Timeout Constants ==============
// Constants for various operation timeouts.
const (
	// DatabaseOperationTimeout is the timeout for database operations.
	DatabaseOperationTimeout = 2 * time.Minute

	// CacheOperationTimeout is the timeout for cache write operations.
	CacheOperationTimeout = 10 * time.Second

	// GCStopTimeout is the maximum time to wait for graceful shutdown.
	GCStopTimeout = 30 * time.Second
)

// ============== Sync Intervals ==============
// Constants for synchronization intervals.
const (
	// PersistenceInterval is the interval between Redis→Database persistence cycles.
	PersistenceInterval = 5 * time.Minute

	// ValidatorSyncDelay is the initial delay before starting validator sync.
	ValidatorSyncDelay = 30 * time.Second

	// StakingInfoSyncDelay is the initial delay before starting staking info sync.
	StakingInfoSyncDelay = 60 * time.Second
)

// ============== Redis TTL ==============
// Constants for Redis key time-to-live values.
const (
	// NoTTL indicates a key should not expire (used for primary data source).
	NoTTL time.Duration = 0

	// CacheTTL is the default TTL for cached/derived data.
	CacheTTL = 5 * time.Minute
)
