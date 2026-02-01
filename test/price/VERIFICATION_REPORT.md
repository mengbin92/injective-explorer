# Phase 0 SDK API 验证报告

## 验证日期
2026-02-01

## 验证环境
- 网络: Testnet (load balanced)
- SDK 版本: v1.58.3 / v1.59.0
- Go 版本: 1.25.3

---

## 1. API 可用性汇总

| 模块 | API 方法 | 可用性 | 备注 |
|-----|---------|--------|------|
| **Chain** | FetchChainSpotMarkets | ✅ 可用 | 返回 209 个市场 |
| **Chain** | FetchChainSpotMarket | ✅ 可用 | 单市场详情查询 |
| **Chain** | FetchChainFullSpotMarkets | ✅ 可用 | 包含中间价信息 |
| **Chain** | FetchSpotMidPriceAndTOB | ✅ 可用 | 最优买卖价 + 中间价 |
| **Chain** | FetchChainDerivativeMarkets | ✅ 可用 | 返回 21 个永续合约市场 |
| **Explorer** | GetBlocks | ❌ 不可用 | 503 Service Unavailable |
| **Explorer** | GetBlock | ❓ 未测试 | 预计同样问题 |

---

## 2. 价格获取方式确认

### 2.1 Spot Market 价格获取（推荐）

**API 调用链:**
```go
// 步骤 1: 获取市场列表
markets, err := chainClient.FetchChainSpotMarkets(ctx, "", nil)

// 步骤 2: 获取特定市场的中间价
midPrice, err := chainClient.FetchSpotMidPriceAndTOB(ctx, marketID)
```

**返回数据结构:**
```json
{
  "mid_price": "1.103312364400000000",
  "best_buy_price": "1.092279240700000000",
  "best_sell_price": "1.114345488100000000"
}
```

**精度:** 18 位小数 (sdk.Dec 格式)

### 2.2 Derivative Market 价格获取（备用）

**API 调用:**
```go
markets, err := chainClient.FetchChainDerivativeMarkets(ctx, "", nil, true)
```

**包含字段:**
- `mark_price`: 标记价格
- `oracle_base`/`oracle_quote`: 预言机喂价标识
- `mid_price_and_tob`: 中间价和最优买卖价

---

## 3. 关键数据结构

### 3.1 SpotMarket (v2)

```go
type SpotMarket struct {
    Ticker                string  // "NLT2/INJ"
    BaseDenom             string  // "factory/inj17gkuet8f6pssxd8nycm3qr9d9y699rupv6397z/NLT2"
    QuoteDenom            string  // "inj"
    MarketId              string  // "0x0033118e7cf665369d4208ea03c88549f151c1303f43442413679c8c407cc0d3"
    Status                int32   // 1 (Active)
    BaseDecimals          int32   // 6
    QuoteDecimals         int32   // 18
    MinPriceTickSize      string  // "0.000000000100000000"
    MinQuantityTickSize   string  // "0.000100000000000000"
    MakerFeeRate          string  // "-0.000100000000000000"
    TakerFeeRate          string  // "0.000500000000000000"
}
```

### 3.2 MidPriceAndTOB

```go
type QuerySpotMidPriceAndTOBResponse struct {
    MidPrice       string  // "1.103312364400000000"
    BestBuyPrice   string  // "1.092279240700000000"
    BestSellPrice  string  // "1.114345488100000000"
}
```

---

## 4. 重要发现

### 4.1 Oracle 模块状态
- ❌ **SDK 没有直接暴露 Oracle 查询接口**
- ✅ 永续合约市场包含 `oracle_base`/`oracle_quote` 字段
- ✅ 可以通过 `mark_price` 获取预言机相关价格

### 4.2 推荐价格获取策略

| 场景 | 推荐 API | 原因 |
|-----|---------|------|
| 获取代币 USD 价格 | `FetchChainFullSpotMarkets` + `FetchSpotMidPriceAndTOB` | 流动性好，价格实时 |
| 获取 INJ 价格 | 查找 INJ/USDT 或 INJ/USDC 市场 | 主流交易对 |
| 获取主币价格 | `FetchChainDerivativeMarkets` | 包含预言机价格参考 |

### 4.3 主要交易对

从 Testnet 获取的市场中，主流交易对包括:
- WAVAX/USDT
- SKK/USDT
- (需要通过 quote_denom 识别 USDT/USDC 交易对)

---

## 5. 技术方案更新建议

### 5.1 实现调整

原方案中的 Oracle 直接查询需要改为:
```go
// ❌ 原方案（不可用）
// chainClient.QueryOraclePrice(ctx, req)

// ✅ 新方案
chainClient.FetchChainSpotMarkets(ctx, "", nil)
chainClient.FetchSpotMidPriceAndTOB(ctx, marketID)
```

### 5.2 价格精度处理

```go
import sdkmath "cosmossdk.io/math"

// 价格解析
priceDec, err := sdkmath.LegacyNewDecFromStr(midPriceResp.MidPrice)
if err != nil {
    return err
}

// 转换为 float64 (保留 18 位精度)
priceFloat, _ := priceDec.Float64()
```

### 5.3 市场映射

通过 `base_denom` 和 `quote_denom` 识别交易对:
```go
// USDT denom 示例
usdtDenom := "peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5"

// INJ denom 示例
injDenom := "inj"
```

---

## 6. 风险与注意事项

1. **Explorer API 不稳定**: 503 错误，不建议依赖
2. **Testnet 数据有限**: Mainnet 会有更多流动性和交易对
3. **价格精度**: 所有价格都是 18 位小数的字符串，需要正确解析
4. **市场状态**: 需要检查 `status` 字段确保市场活跃

---

## 7. 下一步行动

1. ✅ **验证完成** - SDK API 可用性已确认
2. 🔄 **更新技术方案** - 基于实际 API 调整设计
3. 🔄 **实现价格服务** - 使用 `FetchChainSpotMarkets` + `FetchSpotMidPriceAndTOB`
4. ⏳ **Mainnet 验证** - 在生产环境测试更多交易对

---

**验证状态**: ✅ 通过
**建议实施**: 可以继续 Phase 1 开发
