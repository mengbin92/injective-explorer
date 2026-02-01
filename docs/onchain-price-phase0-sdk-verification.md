# Phase 0: SDK API 验证方案

## 1. 验证目标

验证 Injective SDK 中以下 API 的可用性和返回格式：
1. **Oracle 模块** - 预言机价格查询
2. **Spot Market 模块** - 现货市场信息查询
3. **Explorer API** - 市场摘要数据

## 2. 验证步骤

### Step 1: Oracle 模块 API 验证

```go
// test/oracle_api_test.go
package test

import (
    "context"
    "testing"
    "time"

    chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
    "github.com/InjectiveLabs/sdk-go/client/common"
    oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestOracleAPIVerification(t *testing.T) {
    // 使用测试网进行验证
    network := common.LoadNetwork("testnet", "lb")

    clientCtx, err := chainclient.NewClientContext(network.ChainId, "", nil)
    require.NoError(t, err)

    chainClient, err := chainclient.NewChainClientV2(clientCtx, network)
    require.NoError(t, err)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // 测试 1: 查询支持的 Oracle 类型
    t.Run("QueryOracleTypes", func(t *testing.T) {
        // 尝试不同的查询方式
        // 方式 1: QueryParams
        paramsResp, err := chainClient.QueryOracleParams(ctx)
        if err != nil {
            t.Logf("QueryOracleParams error: %v", err)
        } else {
            t.Logf("Oracle Params: %+v", paramsResp)
        }

        // 方式 2: 直接查询价格（如果 API 存在）
        // 注意：需要确认实际的 API 名称
    })

    // 测试 2: 查询具体价格对
    t.Run("QueryPrice", func(t *testing.T) {
        // 尝试查询 INJ/USDC 价格
        // 需要确认实际的 API 签名
        testCases := []struct {
            base  string
            quote string
        }{
            {"INJ", "USDC"},
            {"INJ", "USDC"},
            {"ATOM", "USDC"},
        }

        for _, tc := range testCases {
            t.Run(tc.base+"_"+tc.quote, func(t *testing.T) {
                // 尝试查询
                price, err := queryOraclePrice(ctx, chainClient, tc.base, tc.quote)
                if err != nil {
                    t.Logf("Query failed for %s/%s: %v", tc.base, tc.quote, err)
                } else {
                    t.Logf("Price for %s/%s: %+v", tc.base, tc.quote, price)
                }
            })
        }
    })
}

// queryOraclePrice 尝试查询预言机价格
func queryOraclePrice(ctx context.Context, client chainclient.ChainClientV2, base, quote string) (interface{}, error) {
    // 尝试 1: 使用 QueryPrice
    // req := &oracletypes.QueryPriceRequest{
    //     BaseDenom:  base,
    //     QuoteDenom: quote,
    // }
    // return client.QueryPrice(ctx, req)

    // 尝试 2: 使用 QueryOraclePrice
    // req := &oracletypes.QueryOraclePriceRequest{...}
    // return client.QueryOraclePrice(ctx, req)

    // 尝试 3: 其他可能的 API

    return nil, nil
}
```

### Step 2: Spot Market API 验证

```go
// test/spot_market_api_test.go
package test

import (
    "context"
    "testing"
    "time"

    chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
    "github.com/InjectiveLabs/sdk-go/client/common"
    "github.com/stretchr/testify/require"
)

func TestSpotMarketAPIVerification(t *testing.T) {
    network := common.LoadNetwork("testnet", "lb")

    clientCtx, err := chainclient.NewClientContext(network.ChainId, "", nil)
    require.NoError(t, err)

    chainClient, err := chainclient.NewChainClientV2(clientCtx, network)
    require.NoError(t, err)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // 测试 1: 查询所有现货市场
    t.Run("QuerySpotMarkets", func(t *testing.T) {
        markets, err := chainClient.QuerySpotMarkets(ctx)
        if err != nil {
            t.Logf("QuerySpotMarkets error: %v", err)
            return
        }

        t.Logf("Found %d spot markets", len(markets))

        // 打印前 5 个市场
        for i, m := range markets {
            if i >= 5 {
                break
            }
            t.Logf("Market %d: %+v", i, m)
        }
    })

    // 测试 2: 查询特定市场
    t.Run("QuerySpotMarket", func(t *testing.T) {
        // 需要知道一个有效的市场 ID
        // 可以从上一步获取
        marketID := "0x..." // 待填充

        market, err := chainClient.QuerySpotMarket(ctx, marketID)
        if err != nil {
            t.Logf("QuerySpotMarket error: %v", err)
            return
        }

        t.Logf("Market details: %+v", market)
    })

    // 测试 3: 查询市场订单簿
    t.Run("QueryOrderbook", func(t *testing.T) {
        marketID := "0x..." // 待填充

        orderbook, err := chainClient.QuerySpotOrderbook(ctx, marketID)
        if err != nil {
            t.Logf("QueryOrderbook error: %v", err)
            return
        }

        t.Logf("Orderbook: %+v", orderbook)
    })
}
```

### Step 3: Explorer API 验证

```go
// test/explorer_api_test.go
package test

import (
    "context"
    "testing"
    "time"

    "github.com/InjectiveLabs/sdk-go/client/common"
    explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
    "github.com/stretchr/testify/require"
)

func TestExplorerAPIVerification(t *testing.T) {
    network := common.LoadNetwork("testnet", "lb")

    explorerClient, err := explorerclient.NewExplorerClient(network)
    require.NoError(t, err)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // 测试 1: 获取市场信息
    t.Run("GetMarkets", func(t *testing.T) {
        markets, err := explorerClient.GetSpotMarkets(ctx)
        if err != nil {
            t.Logf("GetSpotMarkets error: %v", err)
            return
        }

        t.Logf("Found markets via Explorer: %d", len(markets))
    })

    // 测试 2: 获取市场最近交易
    t.Run("GetMarketTrades", func(t *testing.T) {
        marketID := "0x..." // 待填充

        trades, err := explorerClient.GetSpotMarketTrades(ctx, marketID)
        if err != nil {
            t.Logf("GetMarketTrades error: %v", err)
            return
        }

        t.Logf("Recent trades: %+v", trades)
    })
}
```

## 3. 验证检查清单

### Oracle 模块

- [ ] `QueryOracleParams` - 查询预言机参数
- [ ] `QueryPrice` 或 `QueryOraclePrice` - 查询价格
- [ ] `QueryPriceFeed` - 查询价格源
- [ ] 确认价格精度（小数位数）
- [ ] 确认返回字段（timestamp, block_height 等）

### Spot Market 模块

- [ ] `QuerySpotMarkets` - 查询所有市场
- [ ] `QuerySpotMarket` - 查询特定市场
- [ ] `QuerySpotOrderbook` - 查询订单簿
- [ ] `QuerySpotOrders` - 查询订单
- [ ] 确认市场状态字段
- [ ] 确认价格计算方式

### Explorer API

- [ ] `GetSpotMarkets` - 获取市场列表
- [ ] `GetSpotMarket` - 获取市场详情
- [ ] `GetSpotMarketTrades` - 获取市场交易
- [ ] 确认是否有市场统计接口（成交量、价格变化等）

## 4. 验证输出

验证完成后，需要输出以下文档：

### 4.1 API 可用性报告

```markdown
## API 可用性报告

### Oracle 模块
| API 名称 | 可用性 | 返回格式 | 备注 |
|---------|--------|---------|------|
| QueryOracleParams | ✅/❌ | ... | ... |
| QueryPrice | ✅/❌ | ... | ... |

### Spot Market 模块
| API 名称 | 可用性 | 返回格式 | 备注 |
|---------|--------|---------|------|
| QuerySpotMarkets | ✅/❌ | ... | ... |
| ... | ... | ... | ... |
```

### 4.2 价格精度确认

```markdown
## 价格精度确认

### Oracle 价格
- 精度: 18 位小数
- 示例: 1000000000000000000 = 1.0 USD

### Spot Market 价格
- 精度: ...
- 示例: ...
```

### 4.3 更新技术方案

根据验证结果，更新 `onchain-price-technical-design.md` 中的 TODO 部分。

## 5. 运行验证

```bash
# 进入项目目录
cd /Users/mac/vscode/mengbin/biya/biya-explorer

# 运行 Oracle API 测试
go test -v ./test/... -run TestOracleAPIVerification

# 运行 Spot Market API 测试
go test -v ./test/... -run TestSpotMarketAPIVerification

# 运行 Explorer API 测试
go test -v ./test/... -run TestExplorerAPIVerification

# 运行所有验证
go test -v ./test/... -run "APIVerification"
```

## 6. 时间估算

| 步骤 | 预计时间 | 说明 |
|------|---------|------|
| 环境准备 | 30 分钟 | 创建测试文件、配置网络 |
| Oracle API 验证 | 1-2 小时 | 尝试不同 API，确认可用性 |
| Spot Market API 验证 | 1-2 小时 | 验证市场查询接口 |
| Explorer API 验证 | 1 小时 | 验证 Explorer 接口 |
| 文档整理 | 30 分钟 | 整理验证结果 |

**总计: 1 天**
