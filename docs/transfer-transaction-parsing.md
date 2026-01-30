# 转账交易解析与代币精度支持

## 功能概述

本次更新包含以下核心功能：

1. **完整转账交易类型支持** - 自动解析所有转账类交易的 `to` 和 `value` 字段
2. **ERC20 代币精度支持** - 从链上自动获取代币精度并缓存到数据库
3. **前端精度查询** - 通过合约详情 API 获取代币精度

---

## 一、支持的转账交易类型

以下交易类型现在都能正确解析接收方地址 (`to`) 和金额 (`value`)：

### 基础转账
- `/cosmos.bank.v1beta1.MsgSend` - 原生币转账
- `/ibc.applications.transfer.v1.MsgTransfer` - IBC 跨链转账

### Injective 交易所
- `/injective.exchange.v1beta1.MsgDeposit` - 存入交易所
- `/injective.exchange.v1beta1.MsgWithdraw` - 从交易所取出
- `/injective.exchange.v1beta1.MsgSubaccountTransfer` - 子账户间转账
- `/injective.exchange.v1beta1.MsgExternalTransfer` - 外部转账

### 跨链桥与保险
- `/injective.peggy.v1.MsgSendToEth` - Peggy 跨链桥转账到以太坊
- `/injective.insurance.v1beta1.MsgUnderwrite` - 保险承保
- `/injective.insurance.v1beta1.MsgRequestRedemption` - 保险赎回

### TokenFactory
- `/injective.tokenfactory.v1beta1.MsgMint` - 代币铸造
- `/injective.tokenfactory.v1beta1.MsgBurn` - 代币销毁

### 合约执行
- `/cosmwasm.wasm.v1.MsgExecuteContract` - CosmWasm 合约执行（带资金）
- `/injective.wasmx.v1.MsgExecuteContractCompat` - WasmX 合约执行（带资金）

---

## 二、数据库更新

### 自动迁移

首次启动服务时，会自动处理代币精度：

1. **新合约** - 从链上查询 `decimals()` 函数获取精度
2. **已有合约** - 如果 `decimals` 为默认值 18，自动从链上更新

### 手动数据库迁移

如果是现有数据库，执行：

```sql
-- 执行迁移脚本
SOURCE database/migrations/add_contract_decimals.sql

-- 更新已有代币精度
UPDATE contract_infos SET decimals = 6 WHERE token_symbol IN ('USDT', 'USDC');
UPDATE contract_infos SET decimals = 8 WHERE token_symbol = 'WBTC';
```

---

## 三、API 使用指南

### 1. 获取交易详情

```http
GET /api/v1/transaction/by-hash?hash=0x...
```

**响应示例：**
```json
{
  "hash": "0xabc123...",
  "signer": "0x111111...",
  "to": "0x222222...",
  "value": "1.5 USDC",
  "tx_type": "/injective.peggy.v1.MsgSendToEth",
  ...
}
```

### 2. 获取代币精度（前端使用）

从交易详情中获取合约地址后，查询合约详情：

```http
GET /api/v1/contract/info/by-address?contract_address=0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48
```

**响应示例：**
```json
{
  "contract_address": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
  "token_symbol": "USDC",
  "token_name": "USD Coin",
  "decimals": 6,
  ...
}
```

### 3. 前端金额格式化示例

```javascript
// 1. 获取交易详情
const tx = await fetch('/api/v1/transaction/by-hash?hash=0x...').then(r => r.json());

// 2. 解析 value (例如 "1500000 uusdc")
const [amount, denom] = tx.value.split(' ');

// 3. 查询合约获取精度
const contract = await fetch(`/api/v1/contract/info/by-address?contract_address=${tx.to}`).then(r => r.json());
const decimals = contract.decimals || 18;

// 4. 格式化显示金额
const displayAmount = (BigInt(amount) / BigInt(10 ** decimals)).toString();
console.log(`${displayAmount} ${contract.token_symbol}`); // "1.5 USDC"
```

---

## 四、常见代币精度参考

| 代币 | 精度 | 说明 |
|------|------|------|
| INJ (原生) | 18 | Injective 原生代币 |
| USDT | 6 | Tether 稳定币 |
| USDC | 6 | USD Coin 稳定币 |
| WETH | 18 | 包装以太币 |
| WBTC | 8 | 包装比特币 |
| LINK/UNI/AAVE/COMP/MKR | 18 | 标准 ERC20 代币 |

---

## 五、测试数据

使用合并后的测试数据脚本初始化：

```bash
# 进入数据库
mysql -u root -p injective_explorer

# 执行测试数据（包含代币数据和IP管理数据）
SOURCE database/test_data.sql
```

---

## 六、注意事项

1. **精度缓存** - 合约精度首次从链上获取后会缓存到数据库，不会每次都查询
2. **默认值** - 如果链上查询失败，使用默认精度 18
3. **纯代币交易** - 对于没有原生币转账的纯代币交易，`to` 和 `value` 会自动从 `token_transfers` 表同步

---

## 七、故障排查

### 交易 to/value 为空

检查日志：
```
# 查看 token_transfer_sync 日志
grep "TokenTransferSyncService" logs/explorer.log

# 查看 decimals 更新日志
grep "decimals" logs/explorer.log
```

### 合约精度不正确

手动更新：
```sql
UPDATE contract_infos SET decimals = 6 WHERE contract_address = '0x...';
```

或重启服务，会自动重新从链上获取。
