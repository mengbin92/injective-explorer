# 链上币价获取技术方案（修订版）

## 1. 概述

### 1.1 背景

Injective Explorer 目前通过 CoinGecko API 获取代币价格，存在以下限制：
- 依赖第三方服务，存在延迟和可用性问题
- 无法覆盖 Injective 链上所有交易对
- 缺乏链上实时价格数据
- 无法获取历史价格数据用于分析

### 1.2 目标

建立多层次的链上价格获取体系：
1. **主币价格**：通过现货/永续市场获取主币价格
2. **代币价格**：通过 DEX 现货市场获取所有上市代币价格
3. **价格聚合**：多源价格聚合，确保准确性和可用性
4. **历史数据**：存储历史价格数据，支持价格分析和图表展示
5. **质量保证**：价格验证、异常检测、自动降级

### 1.3 架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                   链上价格获取服务 (OnChainPriceService)                    │
├─────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐  │
│  │  主币价格获取    │  │  代币价格获取    │  │      价格聚合层          │  │
│  │  (Native Token) │  │  (Token Price)  │  │   (Price Aggregation)   │  │
│  └────────┬────────┘  └────────┬────────┘  └───────────┬─────────────┘  │
│           │                    │                         │                │
│  ┌────────▼────────┐  ┌────────▼────────┐  ┌───────────▼─────────────┐  │
│  │  Spot Markets   │  │  Spot Markets   │  │   价格验证与质量控制      │  │
│  │  (现货市场 - 主币)│  │  (现货市场 - 代币)│  │  (Validation & Quality) │  │
│  └────────┬────────┘  └────────┬────────┘  └───────────┬─────────────┘  │
│           │                    │                         │                │
│  ┌────────▼────────────────────▼─────────────────────────▼─────────────┐  │
│  │              Injective Chain gRPC API (ChainClientV2)               │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│           │                    │                         │                │
│  ┌────────▼────────┐  ┌────────▼────────┐  ┌───────────▼─────────────┐  │
│  │   价格缓存层     │  │   历史数据存储   │  │   监控与告警            │  │
│  │  (Redis Cache)  │  │  (DB Storage)   │  │  (Monitoring & Alert)  │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1.4 数据源说明

> **重要**: Phase 0 验证确认 SDK 没有直接暴露 Oracle 模块查询接口，Explorer API 也不可用(503)。因此采用以下替代方案：

| 数据源 | 可用性 | 获取方式 | 适用场景 |
|--------|--------|----------|----------|
| **Spot Market** | ✅ 可用 | `FetchChainSpotMarkets` + `FetchSpotMidPriceAndTOB` | 主要价格源 |
| **Derivative Market** | ✅ 可用 | `FetchChainDerivativeMarkets` | 含预言机参考价 |
| **CoinGecko** | ✅ 备用 | HTTP API | 降级方案 |

---

## 2. 技术方案

### 2.1 价格获取核心实现

```go
// internal/service/onchain_price.go
package service

import (
    "context"
    "fmt"
    "strings"
    "time"

    sdkmath "cosmossdk.io/math"
    chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
    "github.com/InjectiveLabs/sdk-go/client/common"
    "github.com/pkg/errors"
    "go.uber.org/zap"

    "injective-explorer/internal/cache"
)

// OnChainPriceProvider 链上价格提供者
type OnChainPriceProvider struct {
    chainClient chainclient.ChainClientV2
    cache       *cache.RedisClient
    logger      *zap.Logger

    cacheTTL      time.Duration
    retryAttempts int
    timeout       time.Duration
}

// PriceInfo 价格信息
type PriceInfo struct {
    Symbol      string
    Denom       string
    PriceUSD    float64
    PriceRaw    string        // 原始价格字符串（18位小数）
    BestBuy     string        // 最优买价
    BestSell    string        // 最优卖价
    Source      string        // "spot_market" / "derivative_market"
    MarketID    string
    Timestamp   int64
    BlockHeight int64
    Confidence  float64       // 置信度 0-1
}

// NewOnChainPriceProvider 创建价格提供者
func NewOnChainPriceProvider(
    chainClient chainclient.ChainClientV2,
    cache *cache.RedisClient,
    logger *zap.Logger,
) *OnChainPriceProvider {
    return &OnChainPriceProvider{
        chainClient:   chainClient,
        cache:         cache,
        logger:        logger,
        cacheTTL:      30 * time.Second,
        retryAttempts: 3,
        timeout:       10 * time.Second,
    }
}

// GetPriceFromSpotMarket 从现货市场获取价格
func (p *OnChainPriceProvider) GetPriceFromSpotMarket(
    ctx context.Context,
    marketID string,
) (*PriceInfo, error) {
    cacheKey := fmt.Sprintf("spot_price:%s", marketID)

    // 检查缓存
    if cached := p.getFromCache(ctx, cacheKey); cached != nil {
        return cached, nil
    }

    queryCtx, cancel := context.WithTimeout(ctx, p.timeout)
    defer cancel()

    // 获取市场信息
    marketsResp, err := p.chainClient.FetchChainSpotMarkets(queryCtx, "", []string{marketID})
    if err != nil {
        return nil, errors.Wrap(err, "failed to fetch spot market")
    }

    if len(marketsResp.Markets) == 0 {
        return nil, errors.New("market not found")
    }

    market := marketsResp.Markets[0]

    // 获取中间价
    midPriceResp, err := p.chainClient.FetchSpotMidPriceAndTOB(queryCtx, marketID)
    if err != nil {
        return nil, errors.Wrap(err, "failed to fetch mid price")
    }

    // 解析价格
    price, err := p.parsePrice(midPriceResp.MidPrice)
    if err != nil {
        return nil, errors.Wrap(err, "failed to parse price")
    }

    priceInfo := &PriceInfo{
        Symbol:     p.extractSymbol(market.Ticker),
        Denom:      market.BaseDenom,
        PriceUSD:   price,
        PriceRaw:   midPriceResp.MidPrice,
        BestBuy:    midPriceResp.BestBuyPrice,
        BestSell:   midPriceResp.BestSellPrice,
        Source:     "spot_market",
        MarketID:   marketID,
        Timestamp:  time.Now().Unix(),
        Confidence: p.calculateConfidence(market, midPriceResp),
    }

    // 缓存结果
    p.saveToCache(ctx, cacheKey, priceInfo)

    return priceInfo, nil
}

// FindMarketByDenom 通过 denom 查找市场
func (p *OnChainPriceProvider) FindMarketByDenom(
    ctx context.Context,
    baseDenom string,
    preferredQuotes []string,
) (string, error) {
    if len(preferredQuotes) == 0 {
        preferredQuotes = []string{"peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5"} // USDT
    }

    queryCtx, cancel := context.WithTimeout(ctx, p.timeout)
    defer cancel()

    // 获取所有市场
    marketsResp, err := p.chainClient.FetchChainSpotMarkets(queryCtx, "", nil)
    if err != nil {
        return "", errors.Wrap(err, "failed to fetch markets")
    }

    // 查找最佳市场
    for _, quote := range preferredQuotes {
        for _, market := range marketsResp.Markets {
            if strings.EqualFold(market.BaseDenom, baseDenom) &&
               strings.EqualFold(market.QuoteDenom, quote) {
                return market.MarketId, nil
            }
        }
    }

    return "", errors.Errorf("no market found for denom: %s", baseDenom)
}

// parsePrice 解析价格（18位小数）
func (p *OnChainPriceProvider) parsePrice(priceStr string) (float64, error) {
    priceDec, err := sdkmath.LegacyNewDecFromStr(priceStr)
    if err != nil {
        return 0, err
    }
    priceFloat, _ := priceDec.Float64()
    return priceFloat, nil
}

// extractSymbol 从 ticker 提取 symbol
func (p *OnChainPriceProvider) extractSymbol(ticker string) string {
    parts := strings.Split(ticker, "/")
    if len(parts) > 0 {
        return parts[0]
    }
    return ticker
}

// calculateConfidence 计算价格置信度
func (p *OnChainPriceProvider) calculateConfidence(
    market interface{},
    midPriceResp interface{},
) float64 {
    // 基础置信度
    confidence := 0.9

    // 可根据市场流动性、价差等因素调整
    // TODO: 实现更复杂的置信度计算

    return confidence
}

// cache 相关方法
func (p *OnChainPriceProvider) getFromCache(ctx context.Context, key string) *PriceInfo {
    // 实现缓存读取
    return nil
}

func (p *OnChainPriceProvider) saveToCache(ctx context.Context, key string, price *PriceInfo) {
    // 实现缓存写入
}
```

### 2.2 代币价格获取（通过 denom）

```go
// GetPriceByDenom 通过 denom 获取代币价格
func (p *OnChainPriceProvider) GetPriceByDenom(
    ctx context.Context,
    denom string,
) (*PriceInfo, error) {
    // 1. 查找对应的市场
    marketID, err := p.FindMarketByDenom(ctx, denom, nil)
    if err != nil {
        return nil, err
    }

    // 2. 获取价格
    return p.GetPriceFromSpotMarket(ctx, marketID)
}
```

### 2.3 主币价格获取

```go
// GetNativeTokenPrice 获取主币价格（INJ/INJ 等）
func (p *OnChainPriceProvider) GetNativeTokenPrice(
    ctx context.Context,
    nativeDenom string,
) (*PriceInfo, error) {
    // 优先查找与 USDT/USDC 的交易对
    preferredQuotes := []string{
        "peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5", // USDT
        "peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", // USDC
    }

    marketID, err := p.FindMarketByDenom(ctx, nativeDenom, preferredQuotes)
    if err != nil {
        return nil, err
    }

    return p.GetPriceFromSpotMarket(ctx, marketID)
}
```

### 2.4 数据模型设计

```go
// internal/models/onchain_price.go

// OnChainPrice 链上价格记录
type OnChainPrice struct {
    ID          uint64    `gorm:"primaryKey"`
    Symbol      string    `gorm:"index:idx_symbol_time"`
    Denom       string    `gorm:"index"`
    MarketID    string    `gorm:"index"`
    PriceUSD    float64   `gorm:"type:decimal(32,18)"`
    PriceRaw    string    `gorm:"type:varchar(100)"` // 原始价格（18位小数）
    BestBuy     string    `gorm:"type:varchar(100)"`
    BestSell    string    `gorm:"type:varchar(100)"`
    Source      string    // "spot_market" / "derivative_market"
    Timestamp   int64     `gorm:"index:idx_symbol_time"`
    BlockHeight int64
    Confidence  float64   `gorm:"type:decimal(5,4)"`
    CreatedAt   time.Time
}

func (OnChainPrice) TableName() string {
    return "onchain_prices"
}

// TokenMarketMapping 代币与市场映射
type TokenMarketMapping struct {
    ID           uint64 `gorm:"primaryKey"`
    Symbol       string `gorm:"index"`
    Denom        string `gorm:"uniqueIndex"`
    SpotMarketID string
    IsActive     bool
    Priority     int
    LastUpdated  time.Time
}
```

### 2.5 同步服务

```go
// internal/service/onchain_price_sync.go

// OnChainPriceSyncer 链上价格同步器
type OnChainPriceSyncer struct {
    provider *OnChainPriceProvider
    db       *gorm.DB
    cache    *cache.RedisClient
    logger   *zap.Logger
}

// Start 启动同步
func (s *OnChainPriceSyncer) Start(ctx context.Context) error {
    // 1. 高频同步（主要代币）- 每 30 秒
    go s.startPriceSync(ctx, 30*time.Second, []string{"inj"})

    // 2. 中频同步（其他代币）- 每 60 秒
    go s.startAllMarketsSync(ctx, 60*time.Second)

    return nil
}

func (s *OnChainPriceSyncer) startPriceSync(
    ctx context.Context,
    interval time.Duration,
    denoms []string,
) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for _, denom := range denoms {
                price, err := s.provider.GetPriceByDenom(ctx, denom)
                if err != nil {
                    s.logger.Warn("failed to sync price",
                        zap.String("denom", denom),
                        zap.Error(err),
                    )
                    continue
                }
                s.savePrice(ctx, price)
            }
        }
    }
}
```

---

## 3. 配置设计

```yaml
# configs/config.yaml
onchain_price:
  enabled: true

  # 价格源配置
  spot_markets:
    enabled: true
    cache_ttl: 30s
    retry_attempts: 3
    timeout: 10s

    # 主要代币配置
    native_tokens:
      - denom: "inj"
        symbol: "INJ"
        priority: 1
      - denom: "inj"
        symbol: "INJ"
        priority: 2

    # 优先报价币种（按优先级排序）
    preferred_quotes:
      - "peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5"  # USDT
      - "peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"  # USDC

  # 降级配置
  fallback:
    enabled: true
    coingecko:
      enabled: true
      cache_ttl: 300s

  # 同步配置
  sync:
    native_interval: 30s
    all_markets_interval: 60s
    batch_size: 50

  # 监控配置
  monitoring:
    enabled: true
    alert_on_error_rate: 0.1
```

---

## 4. API 接口设计

```protobuf
// api/explorer/v1/onchain_price.proto
syntax = "proto3";
package explorer.v1;

service OnChainPriceService {
  // 获取代币价格（链上）
  rpc GetOnChainPrice(GetOnChainPriceRequest) returns (GetOnChainPriceResponse);

  // 批量获取价格
  rpc GetBatchOnChainPrices(GetBatchOnChainPricesRequest) returns (GetBatchOnChainPricesResponse);
}

message GetOnChainPriceRequest {
  string symbol = 1;  // 代币符号，如 "INJ"
  string denom = 2;   // 链上 denom，优先使用
}

message OnChainPrice {
  string symbol = 1;
  string denom = 2;
  double price_usd = 3;
  string price_raw = 4;     // 原始价格（18位小数）
  string best_buy = 5;
  string best_sell = 6;
  string source = 7;        // "spot_market"
  string market_id = 8;
  int64 timestamp = 9;
  double confidence = 10;
}

message GetOnChainPriceResponse {
  OnChainPrice price = 1;
}

message GetBatchOnChainPricesRequest {
  repeated string denoms = 1;
}

message GetBatchOnChainPricesResponse {
  repeated OnChainPrice prices = 1;
}
```

---

## 5. 实现计划

| 阶段 | 功能 | 优先级 | 工作量 | 依赖 |
|------|------|--------|--------|------|
| Phase 1 | OnChainPriceProvider 基础实现 | P0 | 2 天 | SDK 验证完成 ✅ |
| Phase 2 | 主要代币价格同步 | P0 | 2 天 | Phase 1 |
| Phase 3 | 价格聚合与 CoinGecko 降级 | P1 | 2 天 | Phase 2 |
| Phase 4 | 历史数据存储与查询 | P2 | 3 天 | Phase 3 |
| Phase 5 | API 接口与监控 | P2 | 2 天 | Phase 4 |

---

## 6. 关键设计决策

### 6.1 为什么不用 Explorer API?
- Phase 0 验证显示 Explorer API 返回 503 错误，服务不稳定
- Chain Client 直接访问 gRPC 更可靠

### 6.2 为什么不用 Oracle 模块?
- SDK 没有直接暴露 Oracle 查询接口
- 永续合约市场包含预言机参考价可作为备选

### 6.3 价格精度处理
- 所有价格都是 18 位小数的字符串
- 使用 `sdkmath.LegacyNewDecFromStr` 解析
- 转换为 float64 时可能损失精度，保留原始字符串用于精确计算

### 6.4 USDT/USDC Denom 识别
```go
// 主要稳定币 denom 常量
const (
    USDTDenom = "peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5"
    USDCDenom = "peggy0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
)
```

---

## 7. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|----------|
| Chain API 限流 | 高 | 缓存 + 限流控制 |
| 市场流动性不足 | 中 | 多市场选择 + 置信度计算 |
| 价格精度损失 | 低 | 保留原始字符串 |
| 新市场未映射 | 低 | 定期同步市场列表 |

---

## 附录: 验证结果

详见: `test/price/VERIFICATION_REPORT.md`

---

**文档版本**: v1.1 (修订版)
**更新日期**: 2026-02-01
**更新说明**: 基于 Phase 0 SDK 验证结果，移除 Explorer 依赖，调整 Oracle 模块方案
