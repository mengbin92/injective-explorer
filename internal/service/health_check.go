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

// HealthStatus 健康状态
type HealthStatus string

const (
	// HealthStatusHealthy 健康
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusDegraded 降级
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnhealthy 不健康
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentHealth 组件健康状态
type ComponentHealth struct {
	Name      string            `json:"name"`
	Status    HealthStatus      `json:"status"`
	Latency   time.Duration     `json:"latency"`
	Error     string            `json:"error,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	LastCheck time.Time         `json:"last_check"`
}

// HealthReport 健康报告
type HealthReport struct {
	Status      HealthStatus       `json:"status"`
	Timestamp   time.Time          `json:"timestamp"`
	Version     string             `json:"version"`
	Components  []*ComponentHealth `json:"components"`
	OverallLatency time.Duration   `json:"overall_latency"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	provider   *OnChainPriceProvider
	logger     *log.Helper

	// 检查结果缓存
	lastReport   *HealthReport
	reportMutex  sync.RWMutex
	cacheExpiry  time.Duration
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(provider *OnChainPriceProvider, logger *log.Helper) *HealthChecker {
	return &HealthChecker{
		provider:    provider,
		logger:      logger,
		cacheExpiry: 30 * time.Second,
	}
}

// CheckHealth 执行健康检查
func (h *HealthChecker) CheckHealth(ctx context.Context) *HealthReport {
	start := time.Now()

	// 检查缓存是否过期
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

	// 1. 检查链客户端
	chainHealth := h.checkChainClient(ctx)
	report.Components = append(report.Components, chainHealth)

	// 2. 检查数据库
	dbHealth := h.checkDatabase(ctx)
	report.Components = append(report.Components, dbHealth)

	// 3. 检查Redis
	redisHealth := h.checkRedis(ctx)
	report.Components = append(report.Components, redisHealth)

	// 4. 检查价格获取能力
	priceHealth := h.checkPriceFetching(ctx)
	report.Components = append(report.Components, priceHealth)

	// 计算整体状态
	report.Status = h.calculateOverallStatus(report.Components)
	report.OverallLatency = time.Since(start)

	// 缓存结果
	h.reportMutex.Lock()
	h.lastReport = report
	h.reportMutex.Unlock()

	return report
}

// checkChainClient 检查链客户端
func (h *HealthChecker) checkChainClient(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:    "chain_client",
		Details: make(map[string]interface{}),
	}

	if h.provider == nil {
		health.Status = HealthStatusUnhealthy
		health.Error = "provider not initialized"
		health.LastCheck = time.Now()
		return health
	}

	// 尝试获取链上市场列表
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 这里我们通过尝试获取一个已知市场来检查连接
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

// checkDatabase 检查数据库
func (h *HealthChecker) checkDatabase(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:    "database",
		Details: make(map[string]interface{}),
	}

	dbInstance := db.Get()
	if dbInstance == nil {
		health.Status = HealthStatusUnhealthy
		health.Error = "database not initialized"
		health.LastCheck = time.Now()
		return health
	}

	// 执行简单查询
	sqlDB, err := dbInstance.DB()
	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.Error = errors.Wrap(err, "failed to get sql db").Error()
		health.LastCheck = time.Now()
		return health
	}

	health.Latency = time.Since(start)
	health.LastCheck = time.Now()

	// 检查连接池状态
	stats := sqlDB.Stats()
	health.Details["open_connections"] = stats.OpenConnections
	health.Details["in_use"] = stats.InUse
	health.Details["idle"] = stats.Idle

	// 判断健康状态
	if stats.OpenConnections > int(stats.MaxOpenConnections)*90/100 {
		health.Status = HealthStatusDegraded
		health.Error = "connection pool near capacity"
	} else {
		health.Status = HealthStatusHealthy
	}

	return health
}

// checkRedis 检查Redis
func (h *HealthChecker) checkRedis(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:    "redis",
		Details: make(map[string]interface{}),
	}

	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		health.Status = HealthStatusDegraded
		health.Error = "redis not available"
		health.LastCheck = time.Now()
		return health
	}

	// 执行PING
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

// checkPriceFetching 检查价格获取能力
func (h *HealthChecker) checkPriceFetching(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:    "price_fetching",
		Details: make(map[string]interface{}),
	}

	if h.provider == nil {
		health.Status = HealthStatusUnhealthy
		health.Error = "provider not initialized"
		health.LastCheck = time.Now()
		return health
	}

	// 尝试获取INJ价格
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

// calculateOverallStatus 计算整体健康状态
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

// IsHealthy 是否健康
func (h *HealthChecker) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report := h.CheckHealth(ctx)
	return report.Status == HealthStatusHealthy
}

// GetComponentHealth 获取特定组件健康状态
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

// SelfCheck 自检（简化版，用于快速检查）
func (h *HealthChecker) SelfCheck() bool {
	if h.provider == nil {
		return false
	}

	// 简单检查：链客户端是否可用
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := h.provider.FindMarketByDenom(ctx, "inj", nil)
	return err == nil
}

// HealthCheckHandler HTTP健康检查处理器
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

// ReadinessCheck 就绪检查
func (h *HealthChecker) ReadinessCheck(ctx context.Context) bool {
	report := h.CheckHealth(ctx)

	// 只要链客户端和价格获取健康，就认为已就绪
	for _, c := range report.Components {
		if c.Name == "chain_client" || c.Name == "price_fetching" {
			if c.Status != HealthStatusHealthy {
				return false
			}
		}
	}
	return true
}

// LivenessCheck 存活检查
func (h *HealthChecker) LivenessCheck() bool {
	// 简单检查进程是否存活
	return h != nil
}
