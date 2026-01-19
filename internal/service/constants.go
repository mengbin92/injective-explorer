// Package service provides constants used across service packages.
package service

import (
	"time"

	"injective-explorer/internal/conf"
)

const (
	// Pagination constants
	DefaultPageSize = 20
	MaxPageSize     = 100
	MinPage         = 1
	MinPageSize     = 1

	// Timeout constants
	GRPCTimeout              = 60 * time.Second
	CountQueryTimeout        = 2 * time.Second
	BlockQueryTimeout        = 30 * time.Second
	DBQueryTimeout           = 5 * time.Second // Database query timeout (reduced from 10s to 5s for faster failure)
	AsyncEnhancementTimeout  = 10 * time.Second // Timeout for asynchronous block/transaction enhancement operations
	ValidatorUpdateInterval  = 10 * time.Minute // Interval for periodic validator updates

	// Approximate count constants
	ApproximateCountThreshold  = 10000
	ApproximateCountFallback   = 1000000
	ApproximateCountForAddress = 10000

	// Error message constants
	MaxErrorLength    = 200 // Maximum length for general error messages
	MaxErrorLogLength = 500 // Maximum length for error_log field in failed transactions

	// Graceful shutdown timeout
	GracefulShutdownTimeout = 30 * time.Second

	// Redis cache constants
	CacheMaxSize = 1000 // Cache the most recent 1000 records (increased from 100 for better cache hit rate)

	// Stream and cache query constants
	StreamInitialBlocksCount       = 10               // Number of initial blocks to send from cache in stream
	StreamInitialTransactionsCount = 20               // Number of initial transactions to send from cache in stream
	StreamFirstEventTimeout        = 10 * time.Second // Timeout for waiting first event in stream
	StreamHeartbeatInterval        = 30 * time.Second // Heartbeat ticker interval for stream

	// Account query constants
	AccountTransactionLimit = 50 // Maximum number of transactions to return for account queries

	// Transaction statistics constants
	TPSBlockCount            = 100         // Number of latest blocks to use for TPS calculation
	AvgBlockTimeSeconds      = 2.0         // Average block time in seconds (for Cosmos chains)
	MillisecondsPerSecond    = 1000.0      // Conversion factor from milliseconds to seconds
	StatsDays30              = 30          // Number of days for 30-day statistics
	StatsHours24             = 24          // Number of hours for 24-hour statistics
	GasPriceBlockCount       = 10          // Number of latest blocks to use for gas price calculation
	BlockTimeBlockCount      = 100         // Number of latest blocks to use for average block time calculation
	GasUtilizationBlockCount = 10          // Number of latest blocks to use for gas utilization calculation
	BlockGasLimit            = 150_000_000 // Default block gas limit for Injective (fallback value if consensus params query fails)

	// Chain query constants
	RecentBlockThreshold = 1000 // Block height threshold for querying from chain (within last N blocks)
	OldBlockThreshold     = 10000 // Block height threshold for considering a block as "old" (skip enhancement for very old blocks)

	// Transaction enhancement constants
	MaxTxEnhancementCount = 100 // Maximum number of transactions to enhance synchronously (blocks with more txs are enhanced asynchronously)

	// Batch size constants
	BlockBatchSizeLimit = 100 // Maximum batch size for block queries
	QueryBatchLimit      = 100 // Maximum batch size for generic queries

	// API token constants
	DefaultAPITokenMonthlyLimit = 100000 // Default monthly limit for API tokens

	// Time period constants (in days)
	PeriodLast7Days  = 7  // Last 7 days
	PeriodLast30Days = 30 // Last 30 days
	PeriodLast1Day   = 1  // Last 1 day

	// Health check constants
	HealthCheckTimeout = 2 * time.Second // Timeout for health check operations

	// Archive constants
	DefaultStorageThreshold = 3 * 1024 // Default storage threshold: 3KB (3072 bytes)
	// Note: This threshold is based on compressed data size, not original data size.
	// Data is compressed first, then if compressed size exceeds this threshold, original data is uploaded to object storage.
	// 3KB is chosen to balance database performance (large TEXT fields >3KB can impact query performance)
	// and storage costs (object storage is typically cheaper than database storage for large fields).
	DefaultArchiveAgeMonths = 6    // Default archive age: 6 months
	DefaultArchiveBatchSize = 1000 // Default archive batch size: 1000 transactions
)

// Redis cache key constants
const (
	CacheKeyBlockList             = "cache:block:list"              // Block list cache key
	CacheKeyTransactionList       = "cache:transaction:list"        // Transaction list cache key
	CacheKeyBlockPrefix           = "cache:block:"                  // Block cache key prefix
	CacheKeyTransactionPrefix     = "cache:transaction:"            // Transaction cache key prefix
	CacheKeyTotalBlocks           = "cache:total:blocks"            // Total blocks cache key
	CacheKeyTotalTransactions     = "cache:total:transactions"      // Total transactions cache key
	CacheKeyTransactionStats      = "cache:transaction:stats"       // Transaction statistics cache key
	CacheKeyFailedTransactions24H = "cache:failed:transactions:24h" // Failed transactions 24h count cache key
	CacheKeyBlockMaxGas           = "cache:block:max:gas"           // Block max gas cache key
	CacheKeyBlockGasUtilization   = "cache:block:gas:utilization"   // Block gas utilization cache key
	CacheKeyEVMDetailsPrefix      = "evm:details:"                  // Cache key prefix for EVM transaction details
	CacheKeyLatestBlockHeight     = "cache:latest:block:height"     // Latest block height cache key
	CacheKeyGasPrice              = "cache:gas:price"               // Gas price cache key
	CacheTotalTTL                 = 5 * time.Minute                 // Total count cache expiration time (5 minutes)
	CacheBlockTTL                 = 1 * time.Hour                   // Single block cache expiration time (1 hour - increased from 5 minutes)
	CacheTransactionTTL           = 5 * time.Minute                 // Single transaction cache expiration time (5 minutes)
	CacheTransactionStatsTTL      = 10 * time.Second                // Transaction statistics cache expiration time (10 seconds - optimized for real-time requirement)
	CacheFailedTransactions24HTTL = 15 * time.Minute                // Failed transactions 24h count cache expiration time (15 minutes - increased from 2 minutes)
	CacheBlockMaxGasTTL           = 10 * time.Minute                // Block max gas cache expiration time (10 minutes)
	CacheBlockGasUtilizationTTL   = 10 * time.Second                // Block gas utilization cache expiration time (10 seconds - optimized for real-time requirement)
	CacheGasPriceTTL              = 10 * time.Second                // Gas price cache expiration time (10 seconds - optimized for real-time requirement)
	CacheEVMDetailsTTL            = 24 * time.Hour                  // Cache TTL: 24 hours (EVM details are relatively static)
)

// Default sync configuration values (used when config is not provided)
const (
	defaultTxHashQueueKey             = "tx:hash:queue"        // Redis key name for transaction hash queue
	defaultTxHashProcessingQueueKey   = "tx:hash:processing"   // Redis key name for transaction hash processing queue
	defaultBlockPollInterval          = 500 * time.Millisecond // Block polling interval (0.5s)
	defaultTxProcessInterval          = 500 * time.Millisecond // Transaction processing interval
	defaultHistoricalSyncInterval     = 10 * time.Second       // Historical block sync interval
	defaultHistoricalBatchDelay       = 1 * time.Second        // Delay between historical block sync batches
	defaultBlockQueryTimeout          = 30 * time.Second       // Block query timeout
	defaultBackwardSyncLogInterval    = 10 * time.Second       // Backward sync progress log interval
	defaultRpcStatusRetryDelay        = 5 * time.Second        // Retry delay after RPC node status fetch failure
	defaultConcurrentTxProcessors     = 20                     // Number of concurrent transaction processors (increased from 5 to 20 for better parallelism)
	defaultBackwardSyncWorkers        = 10                     // Number of backward sync worker threads (increased from 5 to 10)
	defaultHistoricalSyncWorkers      = 5                      // Number of historical sync worker threads (increased from 2 to 5)
	defaultBackwardSyncBatchSize      = 10                     // Backward sync batch size
	defaultHistoricalSyncBatchSize    = 5                      // Historical sync batch size
	defaultMissingBlockCheckBatchSize = 100                    // Batch size for finding missing blocks
	defaultBackwardSyncLogBatchSize   = 100                    // Backward sync progress log interval (every N blocks)
	defaultTxBatchSize                = 50                     // Transaction batch size for batch processing
	defaultMaxRetryAttempts           = 3                      // Maximum retry attempts
	defaultRetryDelay                 = 2 * time.Second        // Initial retry delay
	defaultEarliestBlockHeight        = uint64(1)              // Earliest block height
	defaultEVMDetailsQueueKey         = "evm:details:queue"    // Redis key name for EVM details queue
	defaultEVMDetailsWorkerCount      = 10                     // Number of EVM details sync workers (increased from 3 to 10 for better parallelism)
	defaultEVMDetailsBatchSize        = 10                     // EVM details batch size
	defaultEVMDetailsRetryDelay       = 5 * time.Second        // EVM details retry delay
	defaultEVMDetailsMaxRetryAttempts = 3                      // Maximum retry attempts for EVM details
	defaultEVMDetailsProcessInterval  = 2 * time.Second        // EVM details process interval
	defaultEVMDetailsTimeout          = 90 * time.Second       // EVM details timeout
)

// SyncConfig holds the sync service configuration with default values.
type SyncConfig struct {
	TxHashQueueKey             string
	TxHashProcessingQueueKey   string
	EVMDetailsQueueKey         string
	BlockPollInterval          time.Duration
	TxProcessInterval          time.Duration
	HistoricalSyncInterval     time.Duration
	HistoricalBatchDelay       time.Duration
	BlockQueryTimeout          time.Duration
	BackwardSyncLogInterval    time.Duration
	RpcStatusRetryDelay        time.Duration
	ConcurrentTxProcessors     int
	BackwardSyncWorkers        int
	HistoricalSyncWorkers      int
	BackwardSyncBatchSize      int
	HistoricalSyncBatchSize    int
	MissingBlockCheckBatchSize int
	BackwardSyncLogBatchSize   int
	TxBatchSize                int
	MaxRetryAttempts           int
	RetryDelay                 time.Duration
	EarliestBlockHeight        uint64
}

// LoadSyncConfig loads sync configuration from Bootstrap config with default values.
// If config is nil or fields are not set, default values are used.
func LoadSyncConfig(bc *conf.Bootstrap) *SyncConfig {
	cfg := &SyncConfig{
		TxHashQueueKey:             defaultTxHashQueueKey,
		TxHashProcessingQueueKey:   defaultTxHashProcessingQueueKey,
		EVMDetailsQueueKey:         defaultEVMDetailsQueueKey,
		BlockPollInterval:          defaultBlockPollInterval,
		TxProcessInterval:          defaultTxProcessInterval,
		HistoricalSyncInterval:     defaultHistoricalSyncInterval,
		HistoricalBatchDelay:       defaultHistoricalBatchDelay,
		BlockQueryTimeout:          defaultBlockQueryTimeout,
		BackwardSyncLogInterval:    defaultBackwardSyncLogInterval,
		RpcStatusRetryDelay:        defaultRpcStatusRetryDelay,
		ConcurrentTxProcessors:     defaultConcurrentTxProcessors,
		BackwardSyncWorkers:        defaultBackwardSyncWorkers,
		HistoricalSyncWorkers:      defaultHistoricalSyncWorkers,
		BackwardSyncBatchSize:      defaultBackwardSyncBatchSize,
		HistoricalSyncBatchSize:    defaultHistoricalSyncBatchSize,
		MissingBlockCheckBatchSize: defaultMissingBlockCheckBatchSize,
		BackwardSyncLogBatchSize:   defaultBackwardSyncLogBatchSize,
		TxBatchSize:                defaultTxBatchSize,
		MaxRetryAttempts:           defaultMaxRetryAttempts,
		RetryDelay:                 defaultRetryDelay,
		EarliestBlockHeight:        defaultEarliestBlockHeight,
	}

	if bc == nil || bc.Sync == nil {
		return cfg
	}

	sync := bc.Sync

	// Redis config
	if sync.Redis != nil && sync.Redis.QueueKey != "" {
		cfg.TxHashQueueKey = sync.Redis.QueueKey
	}

	// Intervals
	if sync.Intervals != nil {
		if sync.Intervals.BlockPoll != nil {
			cfg.BlockPollInterval = sync.Intervals.BlockPoll.AsDuration()
		}
		if sync.Intervals.TxProcess != nil {
			cfg.TxProcessInterval = sync.Intervals.TxProcess.AsDuration()
		}
		if sync.Intervals.HistoricalSync != nil {
			cfg.HistoricalSyncInterval = sync.Intervals.HistoricalSync.AsDuration()
		}
		if sync.Intervals.HistoricalBatchDelay != nil {
			cfg.HistoricalBatchDelay = sync.Intervals.HistoricalBatchDelay.AsDuration()
		}
		if sync.Intervals.BackwardSyncLog != nil {
			cfg.BackwardSyncLogInterval = sync.Intervals.BackwardSyncLog.AsDuration()
		}
		if sync.Intervals.RpcStatusRetry != nil {
			cfg.RpcStatusRetryDelay = sync.Intervals.RpcStatusRetry.AsDuration()
		}
	}

	// Timeouts
	if sync.Timeouts != nil && sync.Timeouts.BlockQuery != nil {
		cfg.BlockQueryTimeout = sync.Timeouts.BlockQuery.AsDuration()
	}

	// Concurrency
	if sync.Concurrency != nil {
		if sync.Concurrency.TxProcessors > 0 {
			cfg.ConcurrentTxProcessors = int(sync.Concurrency.TxProcessors)
		}
		if sync.Concurrency.BackwardSyncWorkers > 0 {
			cfg.BackwardSyncWorkers = int(sync.Concurrency.BackwardSyncWorkers)
		}
		if sync.Concurrency.HistoricalSyncWorkers > 0 {
			cfg.HistoricalSyncWorkers = int(sync.Concurrency.HistoricalSyncWorkers)
		}
	}

	// Batch sizes
	if sync.BatchSize != nil {
		if sync.BatchSize.BackwardSync > 0 {
			cfg.BackwardSyncBatchSize = int(sync.BatchSize.BackwardSync)
		}
		if sync.BatchSize.HistoricalSync > 0 {
			cfg.HistoricalSyncBatchSize = int(sync.BatchSize.HistoricalSync)
		}
		if sync.BatchSize.MissingBlockCheck > 0 {
			cfg.MissingBlockCheckBatchSize = int(sync.BatchSize.MissingBlockCheck)
		}
		if sync.BatchSize.BackwardSyncLog > 0 {
			cfg.BackwardSyncLogBatchSize = int(sync.BatchSize.BackwardSyncLog)
		}
		if sync.BatchSize.TxBatch > 0 {
			cfg.TxBatchSize = int(sync.BatchSize.TxBatch)
		}
	}

	// Retry
	if sync.Retry != nil {
		if sync.Retry.MaxAttempts > 0 {
			cfg.MaxRetryAttempts = int(sync.Retry.MaxAttempts)
		}
		if sync.Retry.InitialDelay != nil {
			cfg.RetryDelay = sync.Retry.InitialDelay.AsDuration()
		}
	}

	// Blocks
	if sync.Blocks != nil && sync.Blocks.EarliestHeight > 0 {
		cfg.EarliestBlockHeight = sync.Blocks.EarliestHeight
	}

	return cfg
}

// LoadEVMDetailsSyncConfig loads EVM details sync configuration with default values.
//
// Parameters:
//   - bc: Bootstrap config (can be nil to use defaults)
//
// Returns:
//   - *EVMDetailsSyncConfig: EVM details sync configuration
func LoadEVMDetailsSyncConfig(bc *conf.Bootstrap) *EVMDetailsSyncConfig {
	return &EVMDetailsSyncConfig{
		QueueKey:         defaultEVMDetailsQueueKey,
		WorkerCount:      defaultEVMDetailsWorkerCount,
		BatchSize:        defaultEVMDetailsBatchSize,
		RetryDelay:       defaultEVMDetailsRetryDelay,
		MaxRetryAttempts: defaultEVMDetailsMaxRetryAttempts,
		ProcessInterval:  defaultEVMDetailsProcessInterval,
		Timeout:          defaultEVMDetailsTimeout,
	}
}
