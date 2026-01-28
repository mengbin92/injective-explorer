# 代币测试数据验证指南

本文档介绍如何在导入测试数据后验证代币列表和代币详情功能。

## 目录

1. [启动服务](#1-启动服务)
2. [获取 API Token](#2-获取-api-token)
3. [测试代币列表 API](#3-测试代币列表-api)
4. [测试代币详情 API](#4-测试代币详情-api)
5. [运行单元测试](#5-运行单元测试)
6. [负载测试](#6-负载测试)
7. [Postman 测试](#7-postman-测试)
8. [数据库验证](#8-数据库验证)

---

## 1. 启动服务

### 使用 Docker Compose（推荐）

```bash
docker-compose up -d
```

启动的服务：
- API 服务: http://localhost:8000
- MySQL: localhost:3306
- Redis: localhost:6379
- MinIO: localhost:19000
- Envoy: localhost:9001

### 本地运行

```bash
# 构建
make build

# 运行
./bin/injective-explorer
# 或
go run ./cmd/injective-explorer
```

### 验证服务状态

```bash
curl http://localhost:8000/api/v1/health
```

---

## 2. 获取 API Token

所有 API 请求都需要 Bearer Token 认证。

```bash
curl -X POST "http://localhost:8000/api/v1/tokens" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: test_user_123" \
  -d '{"name": "Test Token"}'
```

**响应示例：**

```json
{
  "token": "inj_xxxxxxxxxxxx",
  "token_preview": "inj_xxx...xxx"
}
```

保存返回的 `token` 值，用于后续请求的 `Authorization` 头。

---

## 3. 测试代币列表 API

### 3.1 基础列表查询

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&pagination.page=1&pagination.page_size=10" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- 返回 10 条代币数据
- 包含字段：token_symbol, price_usd, market_cap_usd, holders_count 等

### 3.2 按市值排序（降序）

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&sort_by=market_cap&sort_order=desc&pagination.page=1&pagination.page_size=20" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- ETH 应该排在第一位（市值 $390B）
- USDT 排在第二位（市值 $95B）
- INJ 在列表中

### 3.3 按 24h 交易量排序

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&sort_by=volume_24h&sort_order=desc" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- USDT 交易量最高（$35B）
- ETH 排在第二位（$15B）

### 3.4 按持有者数量排序

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&sort_by=holders&sort_order=desc" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- ETH 持有者最多（250,000,000）
- USDT 排在第二位（8,500,000）

### 3.5 搜索功能

```bash
# 按符号搜索
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&search=INJ" \
  -H "Authorization: Bearer YOUR_API_TOKEN"

# 按名称搜索
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&search=Bitcoin" \
  -H "Authorization: Bearer YOUR_API_TOKEN"

# 按合约地址搜索
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&search=0xdAC17F958D2ee523a2206206994597C13D831ec7" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- 搜索 "INJ" 返回 INJ 代币
- 搜索 "Bitcoin" 返回 WBTC
- 搜索合约地址返回对应代币

### 3.6 按合约类型过滤

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&contract_type=erc20" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

### 3.7 按验证状态过滤

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&verification_status=verified" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

---

## 4. 测试代币详情 API

### 4.1 获取代币详情 - INJ

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/detail?contract_address=0x0000000000000000000000000000000000000000&chain_id=injective-1" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- token_symbol: "INJ"
- price_usd: 1.25
- market_cap_usd: 1250000000.00
- holders_count: 125000

### 4.2 获取代币详情 - USDT

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/detail?contract_address=0xdAC17F958D2ee523a2206206994597C13D831ec7&chain_id=injective-1" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- token_symbol: "USDT"
- price_usd: 1.00
- decimals: 6
- contract_type: "erc20"

### 4.3 获取代币转账记录

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/transfers?contract_address=0x0000000000000000000000000000000000000000&pagination.page=1&pagination.page_size=10" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- 返回 INJ 的转账记录
- 包含字段：transaction_hash, from_address, to_address, amount, method
- method 包括：Transfer, Mint, Burn

### 4.4 获取代币持有者列表

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/holders?contract_address=0x0000000000000000000000000000000000000000&sort_by=balance&sort_order=desc&pagination.page=1" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- 返回持有 INJ 的账户列表
- 按余额降序排列
- 地址 0x3333... 应该排在前面（余额 25000 INJ）

### 4.5 获取持有者统计

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/holders-stats?contract_address=0x0000000000000000000000000000000000000000" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**验证点：**
- total_holders 与 token_market_cache 中的 holders_count 一致

### 4.6 获取合约代码

```bash
curl -X GET "http://localhost:8000/api/v1/tokens/contract-code?contract_address=0xdAC17F958D2ee523a2206206994597C13D831ec7" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

---

## 5. 运行单元测试

### 5.1 运行所有代币相关测试

```bash
go test ./internal/service/... -run Token -v
```

### 5.2 运行特定测试

```bash
# 测试缓存键构建
go test ./internal/service -run TestBuildCacheKeySuffix -v

# 测试代币排序
go test ./internal/service -run TestTokenSorting -v

# 测试地址格式化
go test ./internal/service -run TestFormatAddress -v
```

### 5.3 运行基准测试

```bash
go test ./internal/service -bench=BenchmarkBuildCacheKeySuffix -benchmem
```

### 5.4 运行所有服务测试

```bash
go test ./internal/service/... -v
```

---

## 6. 负载测试

### 6.1 构建负载测试工具

```bash
make build-loadtest
```

### 6.2 测试代币列表接口

```bash
./bin/loadtest -url http://localhost:8000 -c 50 -d 30s -endpoint /api/v1/tokens/list
```

参数说明：
- `-c 50`: 50 个并发连接
- `-d 30s`: 持续 30 秒
- `-endpoint`: 指定测试端点

### 6.3 测试代币详情接口

```bash
./bin/loadtest -url http://localhost:8000 -c 30 -d 30s -endpoint /api/v1/tokens/detail
```

### 6.4 高并发压力测试

```bash
./bin/loadtest -url http://localhost:8000 -c 100 -d 1m -skip-data-load
```

### 6.5 快速测试

```bash
./bin/loadtest -url http://localhost:8000 -c 10 -d 10s -skip-data-load
```

---

## 7. Postman 测试

### 7.1 导入集合

1. 打开 Postman
2. 点击 "Import" 按钮
3. 选择文件：`postman/Injective_Explorer_Complete_API.postman_collection.json`

### 7.2 配置环境变量

创建新环境，设置以下变量：

| 变量名 | 值 | 说明 |
|--------|-----|------|
| base_url | http://localhost:8000 | API 基础地址 |
| user_id | test_user_123 | 测试用户 ID |
| api_token | {{inj_token}} | 自动填充 |

### 7.3 测试步骤

1. **创建 API Token**
   - 选择 "API Management" → "Create Token"
   - 发送请求
   - 保存返回的 token 到环境变量

2. **测试代币列表**
   - 选择 "Token List" → "List Tokens"
   - 验证返回 10 条数据
   - 测试各种排序参数

3. **测试代币详情**
   - 选择 "Token Detail" → "Get Token Detail"
   - 设置参数：`contract_address=0x0000000000000000000000000000000000000000`
   - 验证返回 INJ 详情

4. **测试转账记录**
   - 选择 "Token Detail" → "Get Token Transfers"
   - 验证包含 Transfer/Mint/Burn 记录

5. **测试持有者列表**
   - 选择 "Token Detail" → "Get Token Holders"
   - 验证按余额排序

### 7.4 自动化测试脚本

Postman 集合包含自动化测试脚本，会自动验证：
- 响应状态码是否为 200
- 响应格式是否正确
- 必要字段是否存在
- 数据类型是否正确

---

## 8. 数据库验证

### 8.1 验证代币市场数据

```sql
-- 查看所有代币
SELECT
    token_symbol,
    price_usd,
    price_change_24h,
    market_cap_usd,
    total_volume_24h,
    holders_count,
    circulating_supply / 1e18 as circulating_supply_eth
FROM token_market_cache
WHERE deleted_at IS NULL
ORDER BY market_cap_usd DESC;
```

**预期结果：**
- 10 条代币记录
- ETH 市值最高（$390B）
- INJ 价格 $1.25，涨幅 5.32%

### 8.2 验证合约信息

```sql
-- 查看合约详情
SELECT
    contract_address,
    token_name,
    token_symbol,
    contract_type,
    verification_status,
    is_verified
FROM contract_infos
WHERE deleted_at IS NULL
ORDER BY token_symbol;
```

**预期结果：**
- 10 条合约记录
- 所有合约都是 verified 状态
- INJ 是 native 类型，其他是 erc20 类型

### 8.3 验证转账记录

```sql
-- 查看转账记录（带代币符号）
SELECT
    tt.transaction_hash,
    ci.token_symbol,
    tt.from_address,
    tt.to_address,
    tt.amount,
    tt.method,
    FROM_UNIXTIME(tt.block_timestamp) as transfer_time
FROM token_transfers tt
LEFT JOIN contract_infos ci ON tt.contract_address = ci.contract_address
WHERE tt.deleted_at IS NULL
ORDER BY tt.block_timestamp DESC
LIMIT 15;
```

**预期结果：**
- 15 条转账记录
- 包含 Transfer、Mint、Burn 三种类型
- INJ 和 USDT 转账记录最多

### 8.4 验证账户余额

```sql
-- 查看各账户余额
SELECT
    address,
    denom,
    amount / 1e18 as balance
FROM account_balances
WHERE deleted_at IS NULL
ORDER BY address, denom;
```

**预期结果：**
- 账户 0x1111... 持有 INJ、USDT、ETH
- 账户 0x8888... 持有大量 ETH 和 WBTC
- 账户 0xAAAA... 持有治理代币（AAVE、COMP、MKR）

### 8.5 统计验证

```sql
-- 各代币转账次数统计
SELECT
    ci.token_symbol,
    COUNT(*) as transfer_count,
    SUM(CASE WHEN tt.method = 'Transfer' THEN 1 ELSE 0 END) as transfers,
    SUM(CASE WHEN tt.method = 'Mint' THEN 1 ELSE 0 END) as mints,
    SUM(CASE WHEN tt.method = 'Burn' THEN 1 ELSE 0 END) as burns
FROM token_transfers tt
LEFT JOIN contract_infos ci ON tt.contract_address = ci.contract_address
WHERE tt.deleted_at IS NULL
GROUP BY ci.token_symbol;
```

```sql
-- 各代币持有者统计
SELECT
    denom,
    COUNT(*) as holder_count
FROM account_balances
WHERE deleted_at IS NULL
GROUP BY denom
ORDER BY holder_count DESC;
```

---

## 9. 常见问题排查

### 9.1 返回空数据

**检查：**
```sql
-- 确认数据存在
SELECT COUNT(*) FROM token_market_cache WHERE deleted_at IS NULL;
SELECT COUNT(*) FROM contract_infos WHERE deleted_at IS NULL;
```

**解决：**
- 重新执行测试数据 SQL 文件
- 检查 `deleted_at` 字段是否为 NULL

### 9.2 排序不正确

**检查：**
```sql
-- 确认字段有值
SELECT token_symbol, market_cap_usd, price_usd
FROM token_market_cache
WHERE market_cap_usd IS NULL OR price_usd IS NULL;
```

### 9.3 搜索无结果

**检查：**
- 搜索关键词是否包含特殊字符
- 合约地址格式是否正确（0x 开头，42 位长度）

### 9.4 API 401 错误

**解决：**
- 确认已创建 API Token
- 请求头格式：`Authorization: Bearer YOUR_TOKEN`
- Token 是否过期（检查 `api_tokens` 表的 `status` 字段）

---

## 10. 测试数据汇总

| 代币 | 符号 | 价格 | 市值 | 持有者数 | 合约地址 |
|------|------|------|------|----------|----------|
| Injective | INJ | $1.25 | $1.25B | 125,000 | 0x0000...0000 |
| Tether | USDT | $1.00 | $95B | 8,500,000 | 0xdAC1...1ec7 |
| USD Coin | USDC | $1.00 | $25B | 3,200,000 | 0xA0b8...6eB48 |
| Ethereum | ETH | $3,250 | $390B | 250,000,000 | 0xC02a...56Cc2 |
| Wrapped BTC | WBTC | $98,500 | $16B | 65,000 | 0x2260...C599 |
| ChainLink | LINK | $18.75 | $11.25B | 180,000 | 0x5149...986CA |
| Uniswap | UNI | $9.85 | $7.4B | 420,000 | 0x1f98...1F984 |
| Aave | AAVE | $165.20 | $2.45B | 125,000 | 0x7Fc6...DDaE9 |
| Compound | COMP | $58.45 | $485M | 68,000 | 0xc00e...68688 |
| Maker | MKR | $1,450 | $1.35B | 92,000 | 0x9f8F...579A2 |

---

## 11. 清理测试数据

如需清理测试数据：

```sql
-- 软删除测试数据（推荐）
UPDATE token_market_cache SET deleted_at = NOW() WHERE data_source = 'coingecko';
UPDATE contract_infos SET deleted_at = NOW() WHERE chain_id = 'injective-1';
UPDATE token_transfers SET deleted_at = NOW() WHERE chain_id = 'injective-1';
UPDATE account_balances SET deleted_at = NOW();

-- 硬删除（谨慎使用）
-- DELETE FROM token_market_cache WHERE data_source = 'coingecko';
-- DELETE FROM contract_infos WHERE chain_id = 'injective-1';
-- DELETE FROM token_transfers WHERE chain_id = 'injective-1';
-- DELETE FROM account_balances;
```

---

**文档版本：** 1.0
**最后更新：** 2025-01-28
**适用项目：** Injective Explorer
