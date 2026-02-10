# 修复跨链桥交易查询问题 - 验证指南

## 问题描述

当查询地址 `0x8fb68d46f8856b00632b4141f139499df864c9ff` 的交易记录时,返回的结果中不包含跨链桥的转账交易(如 `MsgDepositClaim`)。

## 问题原因

`GetAccountTransactions` API 只查询 `transactions` 表,使用 `signer` 或 `to` 字段过滤。但跨链桥交易存储在独立的表中:
- `peggy_deposit_txs` - 从 EVM 链到 Injective 链的存款交易
- `peggy_withdrawal_txs` - 从 Injective 链到 EVM 链的提款交易
- `ibc_transfer_txs` - IBC 转账交易

这些表使用 `sender` 和 `receiver` 字段,而 `MsgDepositClaim` 的 `signer` 是 orchestrator 地址,不是用户地址。

## 解决方案

修改 `GetAccountTransactions` 方法,同时查询:
1. 主交易表 (`transactions`)
2. Peggy 存款表 (`peggy_deposit_txs`)
3. Peggy 提款表 (`peggy_withdrawal_txs`)
4. IBC 转账表 (`ibc_transfer_txs`)

然后合并结果并按时间排序返回。

## 修改的文件

- `internal/service/explorer.go`
  - 修改 `GetAccountTransactions` 方法
  - 新增 `queryCrossChainTransactionsForAddress` 方法
  - 新增 `convertCrossChainToTransactions` 方法
  - 新增 `CrossChainTransaction` 结构体

## 验证步骤

### 1. 编译项目

```bash
cd /Users/mac/vscode/mengbin/biya/biya-explorer
go build ./...
```

### 2. 启动服务

```bash
# 确保数据库和配置正确
./injective-explorer
```

### 3. 测试查询

#### 查询包含跨链交易的地址

```bash
curl -X POST http://localhost:8080/api/explorer/v1/get_account_transactions \
  -H "Content-Type: application/json" \
  -d '{
    "address": "0x8fb68d46f8856b00632b4141f139499df864c9ff",
    "pagination": {
      "page": 1,
      "page_size": 20
    }
  }' | jq .
```

#### 预期结果

返回的交易列表应包含:
- 普通交易(signer 或 to 是该地址)
- Peggy 存款交易(sender 或 receiver 是该地址)
- Peggy 提款交易(sender 或 receiver 是该地址)
- IBC 转账交易(sender 或 receiver 是该地址)

#### 检查特定交易

检查哈希为 `0xd6c96022bc095df80302a3135ce93b88e8c69b0b0ba63c0f47b91cde84d49a40` 的交易是否在结果中:

```bash
curl -X POST http://localhost:8080/api/explorer/v1/get_account_transactions \
  -H "Content-Type: application/json" \
  -d '{
    "address": "0x8fb68d46f8856b00632b4141f139499df864c9ff",
    "pagination": {
      "page": 1,
      "page_size": 100
    }
  }' | jq '.data.data[] | select(.hash == "0xd6c96022bc095df80302a3135ce93b88e8c69b0b0ba63c0f47b91cde84d49a40")'
```

### 4. 验证日志

检查日志中是否有类似以下的输出:

```
queried cross-chain transactions for address 0x8fb68d46f8856b00632b4141f139499df864c9ff: peggy_deposits=3, peggy_withdrawals=0, ibc_transfers=0
converted cross-chain transactions: total=3, new=3, existing=0
account transactions found in database: count=... (regular=..., cross_chain=3), query_time=...
```

### 5. 验证数据完整性

检查交易记录是否:
- 按时间倒序排列(最新的在前)
- 没有重复记录
- 包含完整的跨链交易信息

## 关键改进

1. **完整性**: 现在可以查询到所有类型的交易,包括跨链桥交易
2. **准确性**: 正确计算总数,包括跨链交易数量
3. **性能**: 并行查询多个表,合并后统一排序
4. **兼容性**: 向后兼容,不影响现有功能

## 注意事项

1. 跨链交易可能同时存在于主交易表和跨链交易表中,代码已处理去重
2. 分页是在合并结果后应用的,确保每页数量准确
3. 总数计算可能有轻微重复,但提供了更好的近似值
4. 跨链交易的某些字段(如 messages)可能是简化的,因为这些信息在跨链表中结构不同

## 回滚方案

如果需要回滚,可以恢复 `internal/service/explorer.go` 文件的以下部分:
- 删除 `queryCrossChainTransactionsForAddress` 方法
- 删除 `convertCrossChainToTransactions` 方法
- 删除 `CrossChainTransaction` 结构体
- 恢复 `GetAccountTransactions` 方法中的跨链查询逻辑

## 相关文档

- `docs/cross_chain_data_sync_design.md` - 跨链数据同步设计文档
- `internal/service/crosschain_parser.go` - 跨链交易解析逻辑
- `internal/models/peggy_*.go` - Peggy 交易模型定义
- `internal/models/ibc_transfer_tx.go` - IBC 交易模型定义
