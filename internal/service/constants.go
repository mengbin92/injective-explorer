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
	GRPCTimeout       = 60 * time.Second
	CountQueryTimeout = 2 * time.Second
	BlockQueryTimeout = 30 * time.Second
	DBQueryTimeout    = 10 * time.Second // Database query timeout

	// Approximate count constants
	ApproximateCountThreshold  = 10000
	ApproximateCountFallback   = 1000000
	ApproximateCountForAddress = 10000

	// Error message constants
	MaxErrorLength = 200

	// Graceful shutdown timeout
	GracefulShutdownTimeout = 30 * time.Second
)

// Default sync configuration values (used when config is not provided)
const (
	defaultTxHashQueueKey             = "tx:hash:queue"        // 交易哈希队列的 Redis 键名
	defaultTxHashProcessingQueueKey   = "tx:hash:processing"   // 交易哈希处理队列的 Redis 键名
	defaultBlockPollInterval          = 5 * time.Second        // 区块轮询间隔
	defaultTxProcessInterval          = 500 * time.Millisecond // 交易处理间隔
	defaultHistoricalSyncInterval     = 10 * time.Second       // 历史区块同步间隔
	defaultHistoricalBatchDelay       = 1 * time.Second        // 历史区块同步批次之间的延迟
	defaultBlockQueryTimeout          = 30 * time.Second       // 区块查询超时
	defaultBackwardSyncLogInterval    = 10 * time.Second       // 向后同步进度日志时间间隔
	defaultRpcStatusRetryDelay        = 5 * time.Second        // RPC 节点状态获取失败后的重试延迟
	defaultValidatorSyncInterval      = 10 * time.Minute       // Validator 同步间隔（默认10分钟）
	defaultConcurrentTxProcessors     = 5                      // 并发交易处理器数量
	defaultBackwardSyncWorkers        = 5                      // 向后同步工作线程数
	defaultHistoricalSyncWorkers      = 2                      // 历史同步工作线程数
	defaultBackwardSyncBatchSize      = 10                     // 向后同步批次大小
	defaultHistoricalSyncBatchSize    = 5                      // 历史同步批次大小
	defaultMissingBlockCheckBatchSize = 100                    // 查找缺失区块的批次大小
	defaultBackwardSyncLogBatchSize   = 100                    // 向后同步进度日志间隔（每 N 个区块）
	defaultMaxRetryAttempts           = 3                      // 最大重试次数
	defaultRetryDelay                 = 2 * time.Second        // 初始重试延迟
	defaultEarliestBlockHeight        = uint64(1)              // 最早区块高度
)

// SyncConfig holds the sync service configuration with default values.
type SyncConfig struct {
	TxHashQueueKey             string
	TxHashProcessingQueueKey   string
	BlockPollInterval          time.Duration
	TxProcessInterval          time.Duration
	HistoricalSyncInterval     time.Duration
	HistoricalBatchDelay       time.Duration
	BlockQueryTimeout          time.Duration
	BackwardSyncLogInterval    time.Duration
	RpcStatusRetryDelay        time.Duration
	ValidatorSyncInterval      time.Duration
	ConcurrentTxProcessors     int
	BackwardSyncWorkers        int
	HistoricalSyncWorkers      int
	BackwardSyncBatchSize      int
	HistoricalSyncBatchSize    int
	MissingBlockCheckBatchSize int
	BackwardSyncLogBatchSize   int
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
		BlockPollInterval:          defaultBlockPollInterval,
		TxProcessInterval:          defaultTxProcessInterval,
		HistoricalSyncInterval:     defaultHistoricalSyncInterval,
		HistoricalBatchDelay:       defaultHistoricalBatchDelay,
		BlockQueryTimeout:          defaultBlockQueryTimeout,
		BackwardSyncLogInterval:    defaultBackwardSyncLogInterval,
		RpcStatusRetryDelay:        defaultRpcStatusRetryDelay,
		ValidatorSyncInterval:      defaultValidatorSyncInterval,
		ConcurrentTxProcessors:     defaultConcurrentTxProcessors,
		BackwardSyncWorkers:        defaultBackwardSyncWorkers,
		HistoricalSyncWorkers:      defaultHistoricalSyncWorkers,
		BackwardSyncBatchSize:      defaultBackwardSyncBatchSize,
		HistoricalSyncBatchSize:    defaultHistoricalSyncBatchSize,
		MissingBlockCheckBatchSize: defaultMissingBlockCheckBatchSize,
		BackwardSyncLogBatchSize:   defaultBackwardSyncLogBatchSize,
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
		if sync.Intervals.ValidatorSync != nil {
			cfg.ValidatorSyncInterval = sync.Intervals.ValidatorSync.AsDuration()
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
