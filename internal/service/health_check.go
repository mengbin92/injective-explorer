// Package service provides health check functionality for on-chain price services.
package service

import (
	"context"
	"sync"
	"time"

	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
)

// HealthStatus represents the health status.
type HealthStatus string

const (
	// HealthStatusHealthy indicates healthy status.
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusDegraded indicates degraded status.
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnhealthy indicates unhealthy status.
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentHealth represents the health status of a component.
type ComponentHealth struct {
	Name      string            `json:"name"`
	Status    HealthStatus      `json:"status"`
	Latency   time.Duration     `json:"latency"`
	Error     string            `json:"error,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	LastCheck time.Time         `json:"last_check"`
}

// HealthReport represents a health report.
type HealthReport struct {
	Status      HealthStatus       `json:"status"`
	Timestamp   time.Time          `json:"timestamp"`
	Version     string             `json:"version"`
	Components  []*ComponentHealth `json:"components"`
	OverallLatency time.Duration   `json:"overall_latency"`
}

// HealthChecker performs health checks.
type HealthChecker struct {
	provider   *OnChainPriceProvider
	logger     *log.Helper

	// Cache for check results.
	lastReport   *HealthReport
	reportMutex  sync.RWMutex
	cacheExpiry  time.Duration
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(provider *OnChainPriceProvider, logger *log.Helper) *HealthChecker {
	return &HealthChecker{
		provider:    provider,
		logger:      logger,
		cacheExpiry: 30 * time.Second,
	}
}

// CheckHealth performs a health check.
func (h *HealthChecker) CheckHealth(ctx context.Context) *HealthReport {
	start := time.Now()

	// Check if cache has expired.
	h.reportMutex.RLock()
	cached := h.lastReport
	h.reportMutex.RUnlock()

	if cached != nil && time.Since(cached.Timestamp) < h.cacheExpiry {
		return cached
	}

	report := &HealthReport{
		Timestamp: start,
		Version:   "1.0.0",
		Components: make([]*ComponentHealth, 0),
	}

	// 1. Check chain client.
	chainHealth := h.checkChainClient(ctx)
	report.Components = append(report.Components, chainHealth)

	// 2. Check database.
	dbHealth := h.checkDatabase(ctx)
	report.Components = append(report.Components, dbHealth)

	// 3. Check Redis.
	redisHealth := h.checkRedis(ctx)
	report.Components = append(report.Components, redisHealth)

	// 4. Check price fetching capability.
	priceHealth := h.checkPriceFetching(ctx)
	report.Components = append(report.Components, priceHealth)

	// Calculate overall status.
	report.Status = h.calculateOverallStatus(report.Components)
	report.OverallLatency = time.Since(start)

	// Cache result.
	h.reportMutex.Lock()
	h.lastReport = report
	h.reportMutex.Unlock()

	return report
}

// checkChainClient checks the chain client.
func (h *HealthChecker) checkChainClient(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:    "chain_client",
		Details: make(map[string]any),
	}

	if h.provider == nil {
		health.Status = HealthStatusUnhealthy
		health.Error = "provider not initialized"
		health.LastCheck = time.Now()
		return health
	}

	// Try to query on-chain market list.
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Check connection by querying a known market.
	marketID, err := h.provider.FindMarketByDenom(queryCtx, "inj", nil)
	health.Latency = time.Since(start)
	health.LastCheck = time.Now()

	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.Error = errors.Wrap(err, "failed to query chain").Error()
	} else {
		health.Status = HealthStatusHealthy
		health.Details["sample_market"] = marketID
	}

	return health
}

// checkDatabase checks the database.
func (h *HealthChecker) checkDatabase(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:    "database",
		Details: make(map[string]any),
	}

	dbInstance := db.Get()
	if dbInstance == nil {
		health.Status = HealthStatusUnhealthy
		health.Error = "database not initialized"
		health.LastCheck = time.Now()
		return health
	}

	// Execute simple query.
	sqlDB, err := dbInstance.DB()
	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.Error = errors.Wrap(err, "failed to get sql db").Error()
		health.LastCheck = time.Now()
		return health
	}

	health.Latency = time.Since(start)
	health.LastCheck = time.Now()

	// Check connection pool status.
	stats := sqlDB.Stats()
	health.Details["open_connections"] = stats.OpenConnections
	health.Details["in_use"] = stats.InUse
	health.Details["idle"] = stats.Idle

	// Determine health status.
	if stats.OpenConnections > int(stats.MaxOpenConnections)*90/100 {
		health.Status = HealthStatusDegraded
		health.Error = "connection pool near capacity"
	} else {
		health.Status = HealthStatusHealthy
	}

	return health
}

// checkRedis checks Redis.
func (h *HealthChecker) checkRedis(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:    "redis",
		Details: make(map[string]any),
	}

	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		health.Status = HealthStatusDegraded
		health.Error = "redis not available"
		health.LastCheck = time.Now()
		return health
	}

	// Execute PING.
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	start = time.Now()
	if err := redisClient.Ping(queryCtx).Err(); err != nil {
		health.Status = HealthStatusUnhealthy
		health.Error = errors.Wrap(err, "redis ping failed").Error()
	} else {
		health.Status = HealthStatusHealthy
	}
	health.Latency = time.Since(start)
	health.LastCheck = time.Now()

	return health
}

// checkPriceFetching checks price fetching capability.
func (h *HealthChecker) checkPriceFetching(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:    "price_fetching",
		Details: make(map[string]any),
	}

	if h.provider == nil {
		health.Status = HealthStatusUnhealthy
		health.Error = "provider not initialized"
		health.LastCheck = time.Now()
		return health
	}

	// Try to get INJ price.
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	price, err := h.provider.GetNativeTokenPrice(queryCtx, "inj")
	health.Latency = time.Since(start)
	health.LastCheck = time.Now()

	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.Error = errors.Wrap(err, "failed to fetch price").Error()
	} else {
		health.Status = HealthStatusHealthy
		health.Details["sample_price"] = price.PriceUSD
		health.Details["confidence"] = price.Confidence
	}

	return health
}

// calculateOverallStatus calculates the overall health status.
func (h *HealthChecker) calculateOverallStatus(components []*ComponentHealth) HealthStatus {
	hasUnhealthy := false
	hasDegraded := false

	for _, c := range components {
		switch c.Status {
		case HealthStatusUnhealthy:
			hasUnhealthy = true
		case HealthStatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return HealthStatusUnhealthy
	}
	if hasDegraded {
		return HealthStatusDegraded
	}
	return HealthStatusHealthy
}

// IsHealthy checks if the system is healthy.
func (h *HealthChecker) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report := h.CheckHealth(ctx)
	return report.Status == HealthStatusHealthy
}

// GetComponentHealth gets the health status of a specific component.
func (h *HealthChecker) GetComponentHealth(componentName string) *ComponentHealth {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report := h.CheckHealth(ctx)
	for _, c := range report.Components {
		if c.Name == componentName {
			return c
		}
	}
	return nil
}

// SelfCheck performs a self-check (simplified version for quick checks).
func (h *HealthChecker) SelfCheck() bool {
	if h.provider == nil {
		return false
	}

	// Simple check: verify chain client is available.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := h.provider.FindMarketByDenom(ctx, "inj", nil)
	return err == nil
}

// HealthCheckHandler is the HTTP health check handler.
func (h *HealthChecker) HealthCheckHandler(ctx context.Context) (map[string]interface{}, error) {
	report := h.CheckHealth(ctx)

	return map[string]interface{}{
		"status":     report.Status,
		"timestamp":  report.Timestamp,
		"version":    report.Version,
		"latency_ms": report.OverallLatency.Milliseconds(),
		"components": report.Components,
	}, nil
}

// ReadinessCheck performs a readiness check.
func (h *HealthChecker) ReadinessCheck(ctx context.Context) bool {
	report := h.CheckHealth(ctx)

	// Ready if chain client and price fetching are healthy.
	for _, c := range report.Components {
		if c.Name == "chain_client" || c.Name == "price_fetching" {
			if c.Status != HealthStatusHealthy {
				return false
			}
		}
	}
	return true
}

// LivenessCheck performs a liveness check.
func (h *HealthChecker) LivenessCheck() bool {
	// Simple check if the process is alive.
	return h != nil
}
