// Package service provides optimized on-chain price fetching with tiered caching.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/internal/service/metrics"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
)

const (
	// cacheKeyPrefixV2 缓存键前缀
	cacheKeyPrefixV2 = "onchain:v2:price"
	// localCacheTTL 本地缓存TTL
	localCacheTTL = 5 * time.Second
	// redisCacheTTL Redis缓存TTL
	redisCacheTTL = 30 * time.Second
	// batchSizeV2 批处理大小
	batchSizeV2 = 100
	// maxConcurrencyV2 最大并发数
	maxConcurrencyV2 = 10
)

// OnChainPriceProviderV2 优化版链上价格提供者
type OnChainPriceProviderV2 struct {
	*OnChainPriceProvider
	tieredCache *cache.TieredCache
	logger      *log.Helper

	// 本地内存缓存热门币种
	hotPrices   map[string]*models.PriceInfo
	hotMutex    sync.RWMutex
	hotSymbols  []string // 热门币种列表

	// 批量请求缓冲
	batchBuffer chan *priceRequest
	batchWorker sync.Once
}

// priceRequest 价格请求
type priceRequest struct {
	denom    string
	result   chan *priceResult
	ctx      context.Context
}

// priceResult 价格结果
type priceResult struct {
	price *models.PriceInfo
	err   error
}

// NewOnChainPriceProviderV2 创建优化版价格提供者
func NewOnChainPriceProviderV2(
	baseProvider *OnChainPriceProvider,
	tieredCache *cache.TieredCache,
	logger *log.Helper,
) (*OnChainPriceProviderV2, error) {
	if tieredCache == nil {
		return nil, errors.New("tiered cache is required")
	}

	provider := &OnChainPriceProviderV2{
		OnChainPriceProvider: baseProvider,
		tieredCache:          tieredCache,
		logger:               logger,
		hotPrices:            make(map[string]*models.PriceInfo),
		hotSymbols:           []string{"inj"}, // 默认热门币种
		batchBuffer:          make(chan *priceRequest, 100),
	}

	// 启动批量处理worker
	provider.batchWorker.Do(func() {
		go provider.batchProcessor()
	})

	return provider, nil
}

// GetPriceByDenomV2 优化版获取价格
func (p *OnChainPriceProviderV2) GetPriceByDenomV2(ctx context.Context, denom string) (*models.PriceInfo, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RecordOnChainPriceSyncDuration("get_price", "success", duration)
	}()

	cacheKey := fmt.Sprintf("%s:%s", cacheKeyPrefixV2, denom)

	// 1. 尝试多级缓存
	var cached models.PriceCacheData
	if hit, err := p.tieredCache.Get(ctx, cacheKey, &cached); err == nil && hit {
		metrics.RecordCacheHit("onchain_price")
		return p.convertCacheToPriceInfo(&cached, denom), nil
	}
	metrics.RecordCacheMiss("onchain_price")

	// 2. 检查热门币种本地缓存
	if price := p.getHotPrice(denom); price != nil {
		metrics.RecordCacheHit("onchain_price_hot")
		return price, nil
	}

	// 3. 调用基础提供者获取价格
	price, err := p.OnChainPriceProvider.GetPriceByDenom(ctx, denom)
	if err != nil {
		metrics.RecordOnChainPriceSyncError("get_price", err.Error())
		return nil, err
	}

	// 4. 更新缓存
	p.updateCaches(ctx, cacheKey, price)

	// 5. 如果是热门币种，更新本地缓存
	p.updateHotPrice(denom, price)

	return price, nil
}

// BatchGetPricesV2 优化版批量获取价格
func (p *OnChainPriceProviderV2) BatchGetPricesV2(ctx context.Context, denoms []string) map[string]*models.PriceInfo {
	result := make(map[string]*models.PriceInfo)
	var resultMutex sync.Mutex

	// 使用信号量限制并发
	sem := make(chan struct{}, maxConcurrencyV2)
	var wg sync.WaitGroup

	for _, denom := range denoms {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			price, err := p.GetPriceByDenomV2(ctx, d)
			if err != nil {
				p.logger.Warnw("failed to get price in batch", "denom", d, "error", err)
				return
			}

			resultMutex.Lock()
			result[d] = price
			resultMutex.Unlock()
		}(denom)
	}

	wg.Wait()
	return result
}

// GetHotPrices 获取热门币种价格（极高速）
func (p *OnChainPriceProviderV2) GetHotPrices() map[string]*models.PriceInfo {
	p.hotMutex.RLock()
	defer p.hotMutex.RUnlock()

	result := make(map[string]*models.PriceInfo)
	for denom, price := range p.hotPrices {
		// 检查缓存是否过期（10秒）
		if time.Now().Unix()-price.Timestamp < 10 {
			result[denom] = price
		}
	}
	return result
}

// RefreshHotPrices 刷新热门币种价格
func (p *OnChainPriceProviderV2) RefreshHotPrices(ctx context.Context) error {
	p.hotMutex.RLock()
	denoms := make([]string, 0, len(p.hotSymbols))
	for _, symbol := range p.hotSymbols {
		denoms = append(denoms, symbolToDenom(symbol))
	}
	p.hotMutex.RUnlock()

	prices := p.BatchGetPricesV2(ctx, denoms)

	p.hotMutex.Lock()
	for denom, price := range prices {
		p.hotPrices[denom] = price
	}
	p.hotMutex.Unlock()

	return nil
}

// SetHotSymbols 设置热门币种
func (p *OnChainPriceProviderV2) SetHotSymbols(symbols []string) {
	p.hotMutex.Lock()
	defer p.hotMutex.Unlock()
	p.hotSymbols = symbols
}

// updateCaches 更新多级缓存
func (p *OnChainPriceProviderV2) updateCaches(ctx context.Context, key string, price *models.PriceInfo) {
	cacheData := models.PriceCacheData{
		Price:       price.PriceUSD,
		PriceRaw:    price.PriceRaw,
		BestBuy:     price.BestBuy,
		BestSell:    price.BestSell,
		Source:      price.Source,
		MarketID:    price.MarketID,
		Timestamp:   price.Timestamp,
		BlockHeight: price.BlockHeight,
		Confidence:  price.Confidence,
	}

	// 设置多级缓存（本地5秒，Redis 30秒）
	if err := p.tieredCache.Set(ctx, key, cacheData, 1, redisCacheTTL); err != nil {
		p.logger.Warnw("failed to update tiered cache", "error", err)
	}
}

// updateHotPrice 更新热门币种缓存
func (p *OnChainPriceProviderV2) updateHotPrice(denom string, price *models.PriceInfo) {
	p.hotMutex.Lock()
	defer p.hotMutex.Unlock()

	for _, symbol := range p.hotSymbols {
		if symbolToDenom(symbol) == denom {
			p.hotPrices[denom] = price
			return
		}
	}
}

// getHotPrice 获取热门币种缓存
func (p *OnChainPriceProviderV2) getHotPrice(denom string) *models.PriceInfo {
	p.hotMutex.RLock()
	defer p.hotMutex.RUnlock()

	if price, ok := p.hotPrices[denom]; ok {
		// 检查是否过期（5秒）
		if time.Now().Unix()-price.Timestamp < 5 {
			return price
		}
	}
	return nil
}

// convertCacheToPriceInfo 转换缓存数据到PriceInfo
func (p *OnChainPriceProviderV2) convertCacheToPriceInfo(cache *models.PriceCacheData, denom string) *models.PriceInfo {
	symbol := denomToSymbol(denom)
	return &models.PriceInfo{
		Symbol:      symbol,
		Denom:       denom,
		PriceUSD:    cache.Price,
		PriceRaw:    cache.PriceRaw,
		BestBuy:     cache.BestBuy,
		BestSell:    cache.BestSell,
		Source:      cache.Source,
		MarketID:    cache.MarketID,
		Timestamp:   cache.Timestamp,
		BlockHeight: cache.BlockHeight,
		Confidence:  cache.Confidence,
	}
}

// batchProcessor 批量处理器
func (p *OnChainPriceProviderV2) batchProcessor() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var batch []*priceRequest

	for {
		select {
		case req := <-p.batchBuffer:
			batch = append(batch, req)
			if len(batch) >= batchSizeV2 {
				p.processBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				p.processBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

// processBatch 处理批量请求
func (p *OnChainPriceProviderV2) processBatch(batch []*priceRequest) {
	if len(batch) == 0 {
		return
	}

	// 收集所有denom
	denoms := make([]string, len(batch))
	for i, req := range batch {
		denoms[i] = req.denom
	}

	// 批量获取价格
	prices := p.BatchGetPricesV2(context.Background(), denoms)

	// 分发结果
	for i, req := range batch {
		price := prices[denoms[i]]
		var err error
		if price == nil {
			err = errors.Errorf("price not found for %s", denoms[i])
		}

		select {
		case req.result <- &priceResult{price: price, err: err}:
		case <-time.After(100 * time.Millisecond):
			// 超时处理
		}
	}
}

// AsyncGetPrice 异步获取价格
func (p *OnChainPriceProviderV2) AsyncGetPrice(ctx context.Context, denom string) (<-chan *priceResult, error) {
	req := &priceRequest{
		denom:  denom,
		result: make(chan *priceResult, 1),
		ctx:    ctx,
	}

	select {
	case p.batchBuffer <- req:
		return req.result, nil
	case <-time.After(50 * time.Millisecond):
		// 缓冲满，直接同步获取
		price, err := p.GetPriceByDenomV2(ctx, denom)
		req.result <- &priceResult{price: price, err: err}
		close(req.result)
		return req.result, nil
	}
}

// GetCacheStats 获取缓存统计
func (p *OnChainPriceProviderV2) GetCacheStats() map[string]interface{} {
	p.hotMutex.RLock()
	defer p.hotMutex.RUnlock()

	return map[string]interface{}{
		"hot_symbols_count": len(p.hotSymbols),
		"hot_prices_cached": len(p.hotPrices),
		"hot_symbols":       p.hotSymbols,
	}
}

// denomToSymbol 转换denom到symbol
func denomToSymbol(denom string) string {
	switch denom {
	case "inj":
		return "INJ"
	default:
		return denom
	}
}
