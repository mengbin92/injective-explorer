# 多代币价格配置指南

## 概述

价格服务现在支持配置多个代币，包括基于 Injective 开发的自定义链上的代币。你可以通过配置文件轻松添加和管理多个代币的价格查询。

## 配置方式

### 1. 配置文件示例

在 `configs/config.yaml` 中添加 `token_price` 配置：

```yaml
token_price:
  # 默认代币（当不指定 token_symbol 时使用）
  default_token:
    symbol: "inj"
    denom: "inj"
    coingecko_id: "injective-protocol"
    decimals: 18
    price_source: "coingecko"
    display_name: "Injective"
  
  # 多个代币配置
  tokens:
    # Injective 主网代币
    inj:
      symbol: "inj"
      denom: "inj"
      coingecko_id: "injective-protocol"
      decimals: 18
      price_source: "coingecko"
      display_name: "Injective"
    
    # 自定义链上的代币示例 1：使用 CoinGecko
    mycoin:
      symbol: "mycoin"
      denom: "mycoin"
      coingecko_id: "my-token-id"  # CoinGecko 上的代币 ID
      decimals: 18
      price_source: "coingecko"
      display_name: "My Token"
    
    # 自定义链上的代币示例 2：使用自定义价格 API
    customtoken:
      symbol: "customtoken"
      denom: "customtoken"
      decimals: 18
      price_source: "custom"
      custom_price_url: "https://api.example.com/price/customtoken"
      display_name: "Custom Token"
  
  # 缓存配置（可选）
  # 注意：Duration 格式必须使用秒数（如 "300s" 表示 5 分钟，"3600s" 表示 1 小时）
  price_cache_ttl: "300s"    # 价格缓存时间，默认 5 分钟（300秒）
  supply_cache_ttl: "3600s" # 供应量缓存时间，默认 1 小时（3600秒）
```

### 2. 配置字段说明

#### TokenConfig 字段

- `symbol` (必需): 代币符号，用于 API 请求中指定代币
- `denom` (必需): 链上的代币 denom，用于查询总供应量
- `coingecko_id` (可选): CoinGecko API 中的代币 ID（当 `price_source` 为 "coingecko" 时必需）
- `decimals` (可选): 代币精度，默认 18（与 Ethereum/Injective 相同）
- `price_source` (可选): 价格数据源，支持 "coingecko" 或 "custom"，默认 "coingecko"
- `custom_price_url` (可选): 自定义价格 API 的 URL（当 `price_source` 为 "custom" 时必需）
- `display_name` (可选): 代币显示名称

#### TokenPrice 字段

- `default_token` (可选): 默认代币配置（当不指定 token_symbol 时使用）
- `tokens` (可选): 代币配置映射表，key 为代币符号，value 为代币配置
- `price_cache_ttl` (可选): 价格缓存时间，格式为秒数（如 "300s" 表示 5 分钟），默认 300 秒
- `supply_cache_ttl` (可选): 供应量缓存时间，格式为秒数（如 "3600s" 表示 1 小时），默认 3600 秒

**注意**: `price_cache_ttl` 和 `supply_cache_ttl` 使用 `google.protobuf.Duration` 类型，必须使用秒数格式（如 "300s"、"3600s"），不支持 "5m"、"1h" 等格式。

#### 价格数据源

**CoinGecko** (`price_source: "coingecko"`):
- 需要配置 `coingecko_id`
- 自动获取价格和 24 小时变化百分比
- 免费使用，有速率限制

**自定义 API** (`price_source: "custom"`):
- 需要配置 `custom_price_url`
- API 应返回 JSON 格式：
  ```json
  {
    "price": 25.50,
    "change_24h": 2.5,
    "last_updated": 1704067200
  }
  ```

## API 使用

### 1. 查询指定代币的价格和市值

```bash
GET /demo/token/price-marketcap?token_symbol=mycoin
```

### 2. 查询默认代币（不指定 token_symbol）

```bash
GET /demo/token/price-marketcap
```

### 3. 查询默认代币（不指定 token_symbol）

```bash
GET /demo/token/price-marketcap
```

这将使用配置中的 `default_token`。

### 响应示例

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "token_symbol": "mycoin",
    "token_name": "My Token",
    "price_usd": 1.25,
    "total_supply": "1000000000000000000000000",
    "total_supply_formatted": "1000000",
    "market_cap_usd": 1250000.0,
    "price_change_24h": 5.2,
    "last_updated": 1704067200,
    "price_source": "coingecko"
  }
}
```

## 部署步骤

### 1. 生成配置代码

首先需要生成 proto 配置代码：

```bash
make config
```

这将生成 `internal/conf/conf.pb.go`，包含新的 `TokenPrice` 配置类型。

### 2. 生成 API 代码

```bash
make api
```

这将生成新的 API 消息类型。

### 3. 更新配置文件

在 `configs/config.yaml` 中添加 `token_price` 配置（见上面的示例）。

### 4. 重启服务

重启服务以加载新配置。

## 自定义链配置示例

假设你有一个基于 Injective 开发的自定义链，代币符号为 `mychain`：

```yaml
chain:
  network: "custom"
  lcd_endpoint: "https://lcd.mychain.com"
  tm_endpoint: "https://tm.mychain.com:443"
  chain_id: "mychain-1"
  fee_denom: "mychain"
  name: "My Chain"

token_price:
  default_token:
    symbol: "mychain"
    denom: "mychain"
    coingecko_id: ""  # 如果不在 CoinGecko 上，留空
    decimals: 18
    price_source: "custom"
    custom_price_url: "https://api.mychain.com/price"
    display_name: "My Chain Token"
  
  tokens:
    mychain:
      symbol: "mychain"
      denom: "mychain"
      decimals: 18
      price_source: "custom"
      custom_price_url: "https://api.mychain.com/price"
      display_name: "My Chain Token"
```

## 注意事项

1. **CoinGecko ID**: 如果代币不在 CoinGecko 上，可以：
   - 使用 `price_source: "custom"` 并提供自定义价格 API
   - 或者留空 `coingecko_id`，但必须使用自定义价格源

2. **自定义价格 API**: 
   - 必须返回 JSON 格式
   - 必须包含 `price` 字段（USD 价格）
   - 建议包含 `change_24h` 字段（24 小时变化百分比）

3. **Denom 匹配**: 确保配置中的 `denom` 与链上实际的代币 denom 完全匹配（大小写敏感）

4. **缓存**: 价格和供应量数据会被缓存，减少外部 API 调用。缓存时间可以通过配置调整。

## 故障排查

### 问题：代币配置未找到

**错误**: `token configuration not found for symbol: xxx`

**解决**: 
- 检查配置文件中是否正确添加了代币配置
- 确保 `symbol` 与请求中的 `token_symbol` 参数匹配
- 检查配置文件格式是否正确（YAML 缩进）

### 问题：价格获取失败

**错误**: `failed to fetch price from CoinGecko`

**解决**:
- 检查 `coingecko_id` 是否正确
- 检查网络连接
- 如果是自定义价格源，检查 `custom_price_url` 是否可访问

### 问题：供应量查询失败

**错误**: `failed to fetch supply from LCD`

**解决**:
- 检查 `lcd_endpoint` 配置是否正确
- 检查 `denom` 是否与链上实际 denom 匹配
- 检查 LCD 端点是否可访问

## 扩展性

该方案支持：
- ✅ 多个代币配置
- ✅ 多种价格数据源（CoinGecko、自定义 API）
- ✅ 自定义链上的代币
- ✅ 可配置的缓存时间
- ✅ 向后兼容的 API

未来可以轻松扩展支持：
- 更多价格数据源（CoinMarketCap、Binance 等）
- 历史价格查询
- 多币种价格（EUR、CNY 等）
- WebSocket 实时价格推送

