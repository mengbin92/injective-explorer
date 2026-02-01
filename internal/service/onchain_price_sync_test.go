// Package service provides tests for on-chain price sync functionality.
package service

import (
	"context"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/stretchr/testify/assert"
)

func TestNewOnChainPriceSyncer(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)

	syncer := NewOnChainPriceSyncer(provider, logger)

	assert.NotNil(t, syncer)
	assert.NotNil(t, syncer.provider)
	assert.NotNil(t, syncer.logger)
	assert.Equal(t, defaultNativeInterval, syncer.nativeInterval)
	assert.Equal(t, defaultAllMarketsInterval, syncer.allMarketsInterval)
	assert.Equal(t, defaultBatchSize, syncer.batchSize)
	assert.True(t, syncer.enabled)
	assert.Len(t, syncer.nativeTokens, 2) // 默认 INJ 和 INJ
}

func TestOnChainPriceSyncer_SetNativeTokens(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	syncer := NewOnChainPriceSyncer(provider, logger)

	newTokens := []NativeTokenConfig{
		{Symbol: "TEST", Denom: "test", Priority: 1},
	}

	syncer.SetNativeTokens(newTokens)
	assert.Equal(t, newTokens, syncer.nativeTokens)
}

func TestOnChainPriceSyncer_SetIntervals(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	syncer := NewOnChainPriceSyncer(provider, logger)

	newNativeInterval := 10 * time.Second
	newAllMarketsInterval := 30 * time.Second

	syncer.SetIntervals(newNativeInterval, newAllMarketsInterval)
	assert.Equal(t, newNativeInterval, syncer.nativeInterval)
	assert.Equal(t, newAllMarketsInterval, syncer.allMarketsInterval)
}

func TestOnChainPriceSyncer_EnableDisable(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	syncer := NewOnChainPriceSyncer(provider, logger)

	assert.True(t, syncer.enabled)

	syncer.Disable()
	assert.False(t, syncer.enabled)

	syncer.Enable()
	assert.True(t, syncer.enabled)
}

func TestOnChainPriceSyncer_GetSyncStatus(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	syncer := NewOnChainPriceSyncer(provider, logger)

	status := syncer.GetSyncStatus()
	assert.NotNil(t, status)
	assert.True(t, status["enabled"].(bool))
	assert.Equal(t, defaultNativeInterval.String(), status["native_interval"])
	assert.Equal(t, defaultAllMarketsInterval.String(), status["all_markets_interval"])
	assert.Equal(t, defaultBatchSize, status["batch_size"])
}

func TestOnChainPriceSyncer_generateLockValue(t *testing.T) {
	value1 := generateLockValue()
	value2 := generateLockValue()

	assert.NotEmpty(t, value1)
	assert.NotEmpty(t, value2)
	assert.NotEqual(t, value1, value2) // 应该生成不同的值
}

func TestOnChainPriceSyncer_generateRandomString(t *testing.T) {
	tests := []struct {
		length int
	}{
		{8},
		{16},
		{32},
	}

	for _, tt := range tests {
		result := generateRandomString(tt.length)
		assert.Len(t, result, tt.length)
	}
}

func TestNativeTokenConfig(t *testing.T) {
	config := NativeTokenConfig{
		Symbol:   "TEST",
		Denom:    "test",
		Priority: 1,
	}

	assert.Equal(t, "TEST", config.Symbol)
	assert.Equal(t, "test", config.Denom)
	assert.Equal(t, 1, config.Priority)
}

func TestOnChainPriceSyncer_savePrice(t *testing.T) {
	// 这个测试需要数据库连接，这里仅测试方法存在性
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	syncer := NewOnChainPriceSyncer(provider, logger)

	// 没有数据库连接时，db 应该为 nil
	assert.Nil(t, syncer.db)
}

func TestOnChainPriceSyncer_GetLatestPrice(t *testing.T) {
	// 这个测试需要数据库连接
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	syncer := NewOnChainPriceSyncer(provider, logger)

	// 测试没有数据库的情况
	ctx := context.Background()
	_, err := syncer.GetLatestPrice(ctx, "NONEXISTENT")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

func TestOnChainPriceSyncer_GetPriceHistory(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	syncer := NewOnChainPriceSyncer(provider, logger)

	ctx := context.Background()
	now := time.Now().Unix()
	_, err := syncer.GetPriceHistory(ctx, "TEST", now-3600, now, 100)

	// 没有数据库时应该返回错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

// Test context cancellation
func TestOnChainPriceSyncer_Context(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(nil))
	provider := NewOnChainPriceProvider(nil, logger)
	syncer := NewOnChainPriceSyncer(provider, logger)

	// 测试 context 是否可取消
	assert.NotNil(t, syncer.ctx)
	assert.NotNil(t, syncer.cancel)

	// 测试取消
	syncer.cancel()
	select {
	case <-syncer.ctx.Done():
		// 成功取消
		assert.True(t, true)
	case <-time.After(100 * time.Millisecond):
		t.Error("context should be cancelled")
	}
}

// Benchmark generateLockValue
func BenchmarkGenerateLockValue(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateLockValue()
	}
}

// Benchmark generateRandomString
func BenchmarkGenerateRandomString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateRandomString(16)
	}
}

// Integration test (skipped by default)
func TestOnChainPriceSyncer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Skip("Integration test - run manually with proper environment")
}

// Example usage
func ExampleOnChainPriceSyncer() {
	// 创建同步器
	// logger := log.NewHelper(log.NewStdLogger(nil))
	// syncer, err := NewOnChainPriceSyncerWithSharedClient(logger)
	// if err != nil {
	//     log.Fatal(err)
	// }

	// 启动同步
	// ctx := context.Background()
	// if err := syncer.Start(ctx); err != nil {
	//     log.Fatal(err)
	// }

	// 停止同步
	// defer syncer.Stop()
}

// Test manual sync
func TestOnChainPriceSyncer_ManualSync(t *testing.T) {
	// 这个测试需要链客户端连接，跳过
	t.Skip("ManualSync test requires chain client connection")
}

// Test default constants
func TestSyncConstants(t *testing.T) {
	assert.Equal(t, 30*time.Second, defaultNativeInterval)
	assert.Equal(t, 60*time.Second, defaultAllMarketsInterval)
	assert.Equal(t, 50, defaultBatchSize)
}
