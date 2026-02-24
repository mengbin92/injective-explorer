// Package service provides Ethereum RPC endpoint health checking for Peggy metadata.
package service

import (
	"context"
	"math/big"
	"sync"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/service/metrics"
	"injective-explorer/provider/chain"
)

// ethereumHealthState holds per-endpoint health and failure count.
type ethereumHealthState struct {
	endpoints       []string
	failureCount    []int
	healthyOrder    []int // indices of endpoints sorted by health (healthy first)
	mu              sync.RWMutex
	interval        time.Duration
	timeout         time.Duration
	failureThreshold int
	chainID         *big.Int
}

var (
	ethereumHealth   *ethereumHealthState
	ethereumHealthMu sync.Mutex
)

// StartEthereumHealthChecker starts background health checks for Ethereum RPC endpoints.
// Call once at startup when Ethereum has multiple endpoints and health_check is enabled.
func StartEthereumHealthChecker(ctx context.Context, bc *conf.Bootstrap) {
	cfg := getEthereumConfig(bc)
	if cfg == nil || cfg.HealthCheck == nil || !cfg.HealthCheck.Enabled {
		return
	}
	endpoints := getEthereumRpcEndpoints(bc)
	if len(endpoints) < 2 {
		return
	}
	interval := 30 * time.Second
	if cfg.HealthCheck.Interval != nil {
		interval = cfg.HealthCheck.Interval.AsDuration()
	}
	timeout := 5 * time.Second
	if cfg.HealthCheck.Timeout != nil {
		timeout = cfg.HealthCheck.Timeout.AsDuration()
	}
	threshold := int32(3)
	if cfg.HealthCheck.FailureThreshold > 0 {
		threshold = cfg.HealthCheck.FailureThreshold
	}
	chainID := big.NewInt(cfg.ChainId)
	if chainID.Int64() == 0 {
		chainID = big.NewInt(1)
	}
	ethereumHealthMu.Lock()
	if ethereumHealth != nil {
		ethereumHealthMu.Unlock()
		return
	}
	ethereumHealth = &ethereumHealthState{
		endpoints:        endpoints,
		failureCount:     make([]int, len(endpoints)),
		healthyOrder:     make([]int, len(endpoints)),
		interval:         interval,
		timeout:          timeout,
		failureThreshold: int(threshold),
		chainID:          chainID,
	}
	for i := range endpoints {
		ethereumHealth.healthyOrder[i] = i
	}
	ethereumHealthMu.Unlock()
	go runEthereumHealthChecker(ctx)
	global.Logger.Infof("Ethereum RPC health checker started: %d endpoints, interval=%v", len(endpoints), interval)
}

func runEthereumHealthChecker(ctx context.Context) {
	ticker := time.NewTicker(ethereumHealth.interval)
	defer ticker.Stop()
	// Run first check immediately
	doHealthCheck()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			doHealthCheck()
		}
	}
}

func doHealthCheck() {
	ethereumHealth.mu.Lock()
	defer ethereumHealth.mu.Unlock()
	timeout := ethereumHealth.timeout
	if timeout < time.Second {
		timeout = 5 * time.Second
	}
	for i, ep := range ethereumHealth.endpoints {
		client, err := chain.NewEVMClientWithConfig(&chain.EVMConfig{
			RPCURL:  ep,
			ChainID: ethereumHealth.chainID,
			Network: "ethereum",
			Timeout: timeout,
		})
		if err != nil {
			ethereumHealth.failureCount[i]++
			metrics.SetEthereumEndpointHealth(ep, false)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		_, err = client.GetBlockNumber(ctx)
		cancel()
		client.Close()
		if err != nil {
			ethereumHealth.failureCount[i]++
			metrics.SetEthereumEndpointHealth(ep, false)
		} else {
			ethereumHealth.failureCount[i] = 0
			metrics.SetEthereumEndpointHealth(ep, true)
		}
	}
	// Reorder: healthy endpoints first (failureCount == 0), then by failureCount ascending
	// Simple bubble sort by health
	for i := 0; i < len(ethereumHealth.healthyOrder)-1; i++ {
		for j := 0; j < len(ethereumHealth.healthyOrder)-1-i; j++ {
			a, b := ethereumHealth.healthyOrder[j], ethereumHealth.healthyOrder[j+1]
			if ethereumHealth.failureCount[a] > ethereumHealth.failureCount[b] {
				ethereumHealth.healthyOrder[j], ethereumHealth.healthyOrder[j+1] = b, a
			}
		}
	}
}

// getEthereumEndpointsOrderedByHealth returns endpoints with healthy ones first (when health checker is running).
func getEthereumEndpointsOrderedByHealth(bc *conf.Bootstrap) []string {
	base := getEthereumRpcEndpoints(bc)
	if len(base) == 0 {
		return nil
	}
	ethereumHealthMu.Lock()
	defer ethereumHealthMu.Unlock()
	if ethereumHealth == nil || len(ethereumHealth.endpoints) != len(base) {
		return base
	}
	ordered := make([]string, len(ethereumHealth.endpoints))
	for i, idx := range ethereumHealth.healthyOrder {
		if idx < len(ethereumHealth.endpoints) {
			ordered[i] = ethereumHealth.endpoints[idx]
		}
	}
	return ordered
}
