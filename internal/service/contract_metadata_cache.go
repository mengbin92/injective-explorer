// Package service provides Redis cache for ERC20 contract metadata.
// Key format: contract:metadata:{chain_id}:{contract_address} (lowercase)
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/provider/cache"
)

const (
	// contractMetadataCacheKeyPrefix is the Redis key prefix for ERC20 metadata.
	contractMetadataCacheKeyPrefix = "contract:metadata:"
	// defaultMetadataCacheTTL is the default TTL when not configured.
	defaultMetadataCacheTTL = 1 * time.Hour
)

// getMetadataCacheTTL returns configured cache TTL from sync config.
func getMetadataCacheTTL(bc *conf.Bootstrap) time.Duration {
	if bc == nil || bc.Sync == nil || bc.Sync.MetadataCacheTtl == nil {
		return defaultMetadataCacheTTL
	}
	if d := bc.Sync.MetadataCacheTtl.AsDuration(); d > 0 {
		return d
	}
	return defaultMetadataCacheTTL
}

// ERC20MetadataCached represents cached ERC20 metadata structure.
type ERC20MetadataCached struct {
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	Source   string `json:"source"`
}

// buildMetadataCacheKey builds Redis key for contract metadata.
func buildMetadataCacheKey(chainID, contractAddress string) string {
	addr := strings.ToLower(strings.TrimPrefix(contractAddress, "0x"))
	if addr != "" {
		addr = "0x" + addr
	}
	return fmt.Sprintf("%s%s:%s", contractMetadataCacheKeyPrefix, chainID, addr)
}

// getContractMetadataFromCache retrieves cached ERC20 metadata.
// Returns (metadata, true) if found, (nil, false) otherwise.
func getContractMetadataFromCache(ctx context.Context, chainID, contractAddress string) (*ERC20MetadataCached, bool) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, false
	}
	key := buildMetadataCacheKey(chainID, contractAddress)
	data, err := redisClient.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var m ERC20MetadataCached
	if err := sonic.Unmarshal(data, &m); err != nil {
		global.Logger.Debugf("invalid cached metadata for contract_address=%s: %v", strings.ToLower(contractAddress), err)
		return nil, false
	}
	return &m, true
}

// setContractMetadataToCache stores ERC20 metadata in Redis with configurable TTL.
func setContractMetadataToCache(ctx context.Context, chainID, contractAddress string, m *ERC20MetadataCached) {
	setContractMetadataToCacheWithTTL(ctx, chainID, contractAddress, m, getMetadataCacheTTL(global.BootstrapConfig))
}

// setContractMetadataToCacheWithTTL stores ERC20 metadata in Redis with explicit TTL.
func setContractMetadataToCacheWithTTL(ctx context.Context, chainID, contractAddress string, m *ERC20MetadataCached, ttl time.Duration) {
	if m == nil {
		return
	}
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}
	if ttl <= 0 {
		ttl = defaultMetadataCacheTTL
	}
	key := buildMetadataCacheKey(chainID, contractAddress)
	data, err := sonic.Marshal(m)
	if err != nil {
		global.Logger.Debugf("failed to marshal metadata for cache: %v", err)
		return
	}
	if err := redisClient.Set(ctx, key, data, ttl).Err(); err != nil {
		global.Logger.Warnf("failed to cache metadata for contract_address=%s: %v", strings.ToLower(contractAddress), err)
	}
}

// deleteContractMetadataCache deletes cached metadata (for force refresh).
func deleteContractMetadataCache(ctx context.Context, chainID, contractAddress string) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}
	key := buildMetadataCacheKey(chainID, contractAddress)
	if err := redisClient.Del(ctx, key).Err(); err != nil {
		global.Logger.Warnf("failed to delete metadata cache for contract_address=%s: %v", strings.ToLower(contractAddress), err)
	}
}

const (
	// contractMetadataLockKeyPrefix is the Redis key prefix for metadata update distributed lock.
	contractMetadataLockKeyPrefix = "contract:metadata:update:"
	metadataLockTTL              = 30 * time.Second
)

// acquireContractMetadataUpdateLock acquires a distributed lock for metadata update.
// Returns true if acquired, false otherwise. Caller must call releaseContractMetadataUpdateLock on success.
func acquireContractMetadataUpdateLock(ctx context.Context, chainID, contractAddress string) (bool, string) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return true, "" // no Redis, allow update (graceful degradation)
	}
	addr := strings.ToLower(strings.TrimPrefix(contractAddress, "0x"))
	if addr != "" {
		addr = "0x" + addr
	}
	lockKey := fmt.Sprintf("%s%s:%s", contractMetadataLockKeyPrefix, chainID, addr)
	lockValue := fmt.Sprintf("%d", time.Now().UnixNano())
	ok, err := redisClient.SetNX(ctx, lockKey, lockValue, metadataLockTTL).Result()
	if err != nil || !ok {
		return false, ""
	}
	return true, lockKey
}

// releaseContractMetadataUpdateLock releases the distributed lock.
func releaseContractMetadataUpdateLock(ctx context.Context, lockKey string) {
	if lockKey == "" {
		return
	}
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		_ = redisClient.Del(ctx, lockKey).Err()
	}
}

// DeleteContractMetadataCacheByPrefix deletes all contract metadata cache keys.
// Used for periodic full refresh. Key pattern: contract:metadata:*
func DeleteContractMetadataCacheByPrefix(ctx context.Context) (int64, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return 0, nil
	}
	iter := redisClient.Scan(ctx, 0, contractMetadataCacheKeyPrefix+"*", 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, nil
	}
	return redisClient.Del(ctx, keys...).Result()
}
