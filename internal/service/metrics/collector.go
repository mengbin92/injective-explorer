// Package metrics provides background metrics collection services.
package metrics

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"injective-explorer/internal/global"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

// Collector handles background metrics collection.
type Collector struct {
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	db         *sql.DB
	redis      *redis.Client
	interval   time.Duration
	logger     *log.Helper
	once       sync.Once
}

// NewCollector creates a new metrics collector.
func NewCollector(db *sql.DB, redisClient *redis.Client, interval time.Duration) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	return &Collector{
		ctx:      ctx,
		cancel:   cancel,
		db:       db,
		redis:    redisClient,
		interval: interval,
		logger:   global.Logger,
	}
}

// Start begins the metrics collection goroutine.
func (c *Collector) Start() {
	c.once.Do(func() {
		c.wg.Add(1)
		go c.collectLoop()
		c.logger.Infof("metrics collector started (interval: %v)", c.interval)
	})
}

// Stop gracefully stops the metrics collector.
func (c *Collector) Stop() {
	c.cancel()
	c.wg.Wait()
	c.logger.Info("metrics collector stopped")
}

// collectLoop runs the metrics collection loop.
func (c *Collector) collectLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Collect immediately on start
	c.collectMetrics()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.collectMetrics()
		}
	}
}

// collectMetrics collects all system metrics.
func (c *Collector) collectMetrics() {
	// Collect runtime metrics
	RecordRuntimeMetrics()

	// Collect database connection pool metrics
	if c.db != nil {
		c.collectDatabaseMetrics()
	}

	// Collect Redis connection pool metrics
	if c.redis != nil {
		c.collectRedisMetrics()
	}
}

// collectDatabaseMetrics collects database connection pool statistics.
func (c *Collector) collectDatabaseMetrics() {
	stats := c.db.Stats()

	// Update Prometheus metrics
	SetDatabaseConnections("idle", float64(stats.Idle))
	SetDatabaseConnections("in_use", float64(stats.InUse))
	SetDatabaseConnections("open", float64(stats.OpenConnections))

	// Calculate wait statistics
	waitCount := float64(stats.WaitCount)
	waitDuration := float64(stats.WaitDuration.Nanoseconds())

	SetDatabaseConnectionStats("wait_count", waitCount)
	SetDatabaseConnectionStats("wait_duration_ns", waitDuration)
	SetDatabaseConnectionStats("max_idle_closed", float64(stats.MaxIdleClosed))
	SetDatabaseConnectionStats("max_lifetime_closed", float64(stats.MaxLifetimeClosed))

	c.logger.Debugf("database pool metrics: in_use=%d, idle=%d, open=%d, wait_count=%d",
		stats.InUse, stats.Idle, stats.OpenConnections, stats.WaitCount)
}

// collectRedisMetrics collects Redis connection pool statistics.
func (c *Collector) collectRedisMetrics() {
	poolStats := c.redis.PoolStats()

	SetRedisConnections("hits", float64(poolStats.Hits))
	SetRedisConnections("misses", float64(poolStats.Misses))
	SetRedisConnections("timeouts", float64(poolStats.Timeouts))
	SetRedisConnections("total_conns", float64(poolStats.TotalConns))
	SetRedisConnections("idle_conns", float64(poolStats.IdleConns))
	SetRedisConnections("stale_conns", float64(poolStats.StaleConns))

	c.logger.Debugf("redis pool stats: total=%d, idle=%d, hits=%d, misses=%d, stale=%d",
		poolStats.TotalConns, poolStats.IdleConns, poolStats.Hits, poolStats.Misses, poolStats.StaleConns)
}

// globalCollector holds the singleton collector instance.
var (
	globalCollector *Collector
	collectorMutex sync.Mutex
)

// InitGlobalCollector initializes the global metrics collector.
func InitGlobalCollector(db *sql.DB, redisClient *redis.Client, interval time.Duration) {
	collectorMutex.Lock()
	defer collectorMutex.Unlock()

	if globalCollector != nil {
		global.Logger.Warn("metrics collector already initialized, skipping")
		return
	}

	globalCollector = NewCollector(db, redisClient, interval)
	globalCollector.Start()
}

// StopGlobalCollector stops the global metrics collector.
func StopGlobalCollector() {
	collectorMutex.Lock()
	defer collectorMutex.Unlock()

	if globalCollector != nil {
		globalCollector.Stop()
		globalCollector = nil
	}
}
