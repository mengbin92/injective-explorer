package service

import (
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/db"
)

// NewPeggyBridgeEVMClientFromConfig creates a Peggy bridge EVM client from bootstrap config.
func NewPeggyBridgeEVMClientFromConfig(bc *conf.Bootstrap) (PeggyEvmHashClient, error) {
	if bc == nil || bc.Chain == nil || bc.Chain.PeggyBridgeEvm == nil {
		return nil, nil
	}

	peggyEVM := bc.Chain.PeggyBridgeEvm
	if !peggyEVM.Enabled {
		return nil, nil
	}

	if peggyEVM.RpcEndpoint == "" {
		global.Logger.Warn("Peggy bridge EVM enabled but RPC endpoint not configured")
		return nil, nil
	}

	// Set defaults
	timeout := 10 * time.Second
	if peggyEVM.Timeout != nil {
		timeout = peggyEVM.Timeout.AsDuration()
	}

	rateLimit := int(peggyEVM.RateLimit)
	if rateLimit <= 0 {
		rateLimit = 10 // Default: 10 requests per second
	}

	cfg := chain.Config{
		RPCEndpoint: peggyEVM.RpcEndpoint,
		ChainID:     peggyEVM.ChainId,
		Timeout:     timeout,
		RateLimit:   rateLimit,
	}

	client, err := chain.NewPeggyBridgeEVMClient(cfg)
	if err != nil {
		return nil, err
	}

	global.Logger.Infof("Peggy bridge EVM client initialized: endpoint=%s, chain_id=%d",
		peggyEVM.RpcEndpoint, peggyEVM.ChainId)

	return client, nil
}

// NewPeggyEvmHashSyncServiceFromConfig creates a Peggy EVM hash sync service from bootstrap config.
func NewPeggyEvmHashSyncServiceFromConfig(bc *conf.Bootstrap) (*PeggyEvmHashSyncService, error) {
	if bc == nil {
		return nil, nil
	}

	// Create EVM client
	client, err := NewPeggyBridgeEVMClientFromConfig(bc)
	if err != nil {
		return nil, err
	}
	if client == nil {
		global.Logger.Info("Peggy bridge EVM client not configured, skipping EVM hash sync service")
		return nil, nil
	}

	// Load config
	cfg := LoadPeggyEvmHashSyncConfig(bc)

	// Get Redis client
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		global.Logger.Warn("Redis client not available for Peggy EVM hash sync service")
		return nil, nil
	}

	// Get DB
	db := db.Get()
	if db == nil {
		global.Logger.Warn("Database not available for Peggy EVM hash sync service")
		return nil, nil
	}

	svc := NewPeggyEvmHashSyncService(cfg, client, redisClient, db)

	global.Logger.Infof("Peggy EVM hash sync service created: workers=%d, queue=%s",
		cfg.WorkerCount, cfg.QueueKey)

	return svc, nil
}
