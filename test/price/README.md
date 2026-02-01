# Phase 0: SDK API 验证

## 快速开始

### 1. 运行所有验证测试

```bash
# 在项目根目录执行
./test/price/run_verification.sh
```

### 2. 运行单个测试

```bash
# Oracle API 测试
go test -v ./test/price/... -run TestOraclePriceBySymbolAPI

# Spot Market API 测试
go test -v ./test/price/... -run TestSpotMarketsListAPI

# Explorer API 测试
go test -v ./test/price/... -run TestExplorerMarketSummaryAPI
```

### 3. 运行特定分组测试

```bash
# 所有 Oracle 测试
go test -v ./test/price/... -run "TestOracle"

# 所有 Spot Market 测试
go test -v ./test/price/... -run "TestSpot"

# 所有 Explorer 测试
go test -v ./test/price/... -run "TestExplorer"
```

## 测试文件说明

| 文件 | 内容 | 目的 |
|-----|------|------|
| `oracle_api_test.go` | Oracle 模块 API 测试 | 验证预言机价格查询 |
| `spot_market_api_test.go` | Spot Market 模块测试 | 验证现货市场查询 |
| `explorer_api_test.go` | Explorer API 测试 | 验证 Explorer 接口 |
| `run_verification.sh` | 自动化验证脚本 | 批量运行所有测试 |
| `verification_report_template.md` | 报告模板 | 记录验证结果 |

## 验证检查清单

### Oracle 模块

- [ ] QueryOracleParams 可用
- [ ] QueryOraclePriceFeeds 可用
- [ ] QueryOraclePrice 可用（至少一个交易对）
- [ ] 价格精度确认（18位小数）
- [ ] 时间戳格式确认

### Spot Market 模块

- [ ] QuerySpotMarkets 可用
- [ ] QuerySpotMarket 可用
- [ ] QuerySpotOrderbook 可用
- [ ] 能从订单簿计算中间价
- [ ] 市场精度信息可用

### Explorer API

- [ ] GetSpotMarkets 可用
- [ ] GetSpotMarketSummary 可用
- [ ] 包含价格和成交量数据
- [ ] 数据更新频率可接受

## 常见问题

### 测试超时

如果测试超时，检查：
1. 网络连接是否正常
2. Testnet/Mainnet 是否可用
3. 本地防火墙设置

```bash
# 增加超时时间
go test -v ./test/price/... -run TestOraclePriceBySymbolAPI -timeout 30s
```

### API 不可用

某些 API 可能在特定网络不可用：
- Testnet 数据可能不完整
- Mainnet 需要稳定的连接

### SDK 版本问题

如果编译失败，检查 SDK 版本：

```bash
cat go.mod | grep injective
```

## 验证完成后

1. **填写验证报告**
   - 复制 `verification_report_template.md`
   - 填写实际测试结果

2. **更新技术方案**
   - 修改 `docs/onchain-price-technical-design.md`
   - 替换所有 TODO 为实际 API 调用

3. **调整实现计划**
   - 根据 API 可用性调整优先级
   - 标记不可用的功能

## 输出示例

```
==========================================
Phase 0: SDK API Verification
==========================================

----------------------------------------
运行: Oracle Price By Symbol
----------------------------------------
=== RUN   TestOraclePriceBySymbolAPI
=== RUN   TestOraclePriceBySymbolAPI/INJ_USDC
    oracle_api_test.go:XX: Oracle Price for INJ/USDC:
    oracle_api_test.go:XX:   Price: 12.345678901234567890
    oracle_api_test.go:XX:   Timestamp: 1677654321
=== RUN   TestOraclePriceBySymbolAPI/ATOM_USDC
...
PASS
✓ Oracle Price By Symbol 通过
```
