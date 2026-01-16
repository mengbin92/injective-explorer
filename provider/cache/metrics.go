// Package cache provides Redis cache client initialization and management.
package cache

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// CacheMetrics holds metrics for cache operations.
type CacheMetrics struct {
	mu sync.RWMutex

	// Operation counters
	TotalGets     int64
	TotalSets     int64
	TotalDeletes  int64
	Hits          int64
	Misses        int64
	TotalErrors   int64

	// Latency tracking (in nanoseconds)
	TotalGetLatency time.Duration
	TotalSetLatency time.Duration
	TotalLatency    time.Duration

	// Pool statistics
	PoolStats *redis.PoolStats

	// Time window for metrics collection
	LastReset time.Time
}

var (
	// metrics is the global cache metrics instance
	metrics *CacheMetrics
	// metricsOnce ensures metrics is initialized only once
	metricsOnce sync.Once
)

// initMetrics initializes the cache metrics.
func initMetrics() {
	metricsOnce.Do(func() {
		metrics = &CacheMetrics{
			LastReset: time.Now(),
		}
	})
}

// RecordGet records a cache get operation.
func RecordGet(hit bool, latency time.Duration, err error) {
	initMetrics()
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.TotalGets++
	metrics.TotalGetLatency += latency
	metrics.TotalLatency += latency

	if err != nil {
		metrics.TotalErrors++
	} else if hit {
		metrics.Hits++
	} else {
		metrics.Misses++
	}
}

// RecordSet records a cache set operation.
func RecordSet(latency time.Duration, err error) {
	initMetrics()
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.TotalSets++
	metrics.TotalSetLatency += latency
	metrics.TotalLatency += latency

	if err != nil {
		metrics.TotalErrors++
	}
}

// RecordDelete records a cache delete operation.
func RecordDelete(err error) {
	initMetrics()
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.TotalDeletes++
	if err != nil {
		metrics.TotalErrors++
	}
}

// GetMetrics returns the current cache metrics.
func GetMetrics() CacheMetrics {
	initMetrics()
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()

	// Create a copy to avoid data races
	if redisc != nil {
		metrics.PoolStats = redisc.PoolStats()
	}

	return CacheMetrics{
		TotalGets:       metrics.TotalGets,
		TotalSets:       metrics.TotalSets,
		TotalDeletes:    metrics.TotalDeletes,
		Hits:            metrics.Hits,
		Misses:          metrics.Misses,
		TotalErrors:     metrics.TotalErrors,
		TotalGetLatency: metrics.TotalGetLatency,
		TotalSetLatency: metrics.TotalSetLatency,
		TotalLatency:    metrics.TotalLatency,
		PoolStats:       metrics.PoolStats,
		LastReset:       metrics.LastReset,
	}
}

// GetHitRate returns the cache hit rate as a percentage.
func GetHitRate() float64 {
	initMetrics()
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()

	totalGets := metrics.Hits + metrics.Misses
	if totalGets == 0 {
		return 0.0
	}

	return float64(metrics.Hits) / float64(totalGets) * 100
}

// GetAverageLatency returns the average operation latency.
func GetAverageLatency() time.Duration {
	initMetrics()
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()

	totalOps := metrics.TotalGets + metrics.TotalSets + metrics.TotalDeletes
	if totalOps == 0 {
		return 0
	}

	return metrics.TotalLatency / time.Duration(totalOps)
}

// ResetMetrics resets all metrics to zero.
func ResetMetrics() {
	initMetrics()
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.TotalGets = 0
	metrics.TotalSets = 0
	metrics.TotalDeletes = 0
	metrics.Hits = 0
	metrics.Misses = 0
	metrics.TotalErrors = 0
	metrics.TotalGetLatency = 0
	metrics.TotalSetLatency = 0
	metrics.TotalLatency = 0
	metrics.LastReset = time.Now()
}

// MetricsCollectionInterval defines how often to collect and report metrics.
const MetricsCollectionInterval = 60 * time.Second

// StartMetricsCollector starts a background goroutine to periodically
// collect and log cache metrics.
func StartMetricsCollector(ctx context.Context, logger log.Logger) {
	initMetrics()

	go func() {
		ticker := time.NewTicker(MetricsCollectionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logMetrics(logger)
			}
		}
	}()
}

// logMetrics logs the current cache metrics.
func logMetrics(logger log.Logger) {
	m := GetMetrics()

	// Use structured logging with key-value pairs
	logger.Log(log.LevelInfo,
		"msg", "Cache metrics report",
		"total_gets", m.TotalGets,
		"total_sets", m.TotalSets,
		"total_deletes", m.TotalDeletes,
		"hits", m.Hits,
		"misses", m.Misses,
		"errors", m.TotalErrors,
		"hit_rate_percent", fmt.Sprintf("%.2f", GetHitRate()),
		"avg_latency", GetAverageLatency().String(),
	)

	if m.PoolStats != nil {
		logger.Log(log.LevelInfo,
			"msg", "Redis pool stats",
			"pool_hits", m.PoolStats.Hits,
			"pool_misses", m.PoolStats.Misses,
			"pool_timeouts", m.PoolStats.Timeouts,
			"total_conns", m.PoolStats.TotalConns,
			"idle_conns", m.PoolStats.IdleConns,
			"stale_conns", m.PoolStats.StaleConns,
		)
	}
}

// MetricsMiddleware provides a Redis hook for automatic metrics collection.
type MetricsMiddleware struct{}

var _ redis.Hook = (*MetricsMiddleware)(nil)

// DialHook implements redis.Hook interface for connection dialing.
func (m *MetricsMiddleware) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

// ProcessHook implements redis.Hook interface (alias for BeforeProcess/AfterProcess).
func (m *MetricsMiddleware) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		ctx, err := m.BeforeProcess(ctx, cmd)
		if err != nil {
			return err
		}
		defer m.AfterProcess(ctx, cmd)
		return next(ctx, cmd)
	}
}

// ProcessPipelineHook implements redis.Hook interface for pipeline operations.
func (m *MetricsMiddleware) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		ctx, err := m.BeforeProcessPipeline(ctx, cmds)
		if err != nil {
			return err
		}
		defer m.AfterProcessPipeline(ctx, cmds)
		return next(ctx, cmds)
	}
}

// BeforeProcess implements redis.Hook interface.
func (m *MetricsMiddleware) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	// Store start time in context
	return context.WithValue(ctx, "metrics_start_time", time.Now()), nil
}

// AfterProcess implements redis.Hook interface.
func (m *MetricsMiddleware) AfterProcess(ctx context.Context, cmd redis.Cmder) error {
	// Calculate latency
	if startTime, ok := ctx.Value("metrics_start_time").(time.Time); ok {
		latency := time.Since(startTime)

		// Record based on command type
		switch cmd.Name() {
		case "get":
			hit := cmd.Err() == nil && cmd.(*redis.StringCmd).Val() != ""
			RecordGet(hit, latency, cmd.Err())
		case "mget":
			sliceCmd := cmd.(*redis.SliceCmd)
			values := sliceCmd.Val()
			hit := cmd.Err() == nil && len(values) > 0
			if hit {
				// Check if any value is non-empty
				for _, v := range values {
					if v != nil && v != "" {
						hit = true
						break
					}
					hit = false
				}
			}
			RecordGet(hit, latency, cmd.Err())
		case "set", "mset":
			RecordSet(latency, cmd.Err())
		case "del":
			RecordDelete(cmd.Err())
		}
	}

	return nil
}

// BeforeProcessPipeline implements redis.Hook interface for pipeline operations.
func (m *MetricsMiddleware) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	return context.WithValue(ctx, "metrics_start_time", time.Now()), nil
}

// AfterProcessPipeline implements redis.Hook interface for pipeline operations.
func (m *MetricsMiddleware) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	if startTime, ok := ctx.Value("metrics_start_time").(time.Time); ok {
		latency := time.Since(startTime)

		// For pipelines, record each command type
		for _, cmd := range cmds {
			switch cmd.Name() {
			case "get", "mget":
				hit := cmd.Err() == nil
				if cmd.Name() == "get" && cmd.(*redis.StringCmd).Val() != "" {
					hit = true
				}
				RecordGet(hit, latency/time.Duration(len(cmds)), cmd.Err())
			case "set", "mset":
				RecordSet(latency/time.Duration(len(cmds)), cmd.Err())
			case "del":
				RecordDelete(cmd.Err())
			}
		}
	}

	return nil
}

// AddMetricsHook adds the metrics collection hook to the Redis client.
func AddMetricsHook(client *redis.Client) {
	if client == nil {
		return
	}
	client.AddHook(&MetricsMiddleware{})
}
