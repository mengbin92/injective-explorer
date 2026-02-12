# 交易缓存问题分析与修复方案

## 问题描述

调用 `api/v1/transaction/latest?page=1&page_size=10` 返回 10 条数据后，再调用 `/api/v1/transaction/by-hash` 查询特定交易详情，然后再次调用 `api/v1/transaction/latest?page=1&page_size=10` 时，**只返回 1 条数据**（即刚才查询的那条交易）。

明明数据保存在 Redis 不同的 key 中，为什么会出现这种问题？

---

## 根因分析

### 1. Hash Key 前缀处理不一致（导致缓存无法命中）

**位置**: `internal/service/explorer.go:1137-1149`

```go
// SanitizeHash 会移除 "0x" 前缀
hash := SanitizeHash(req.GetHash())
// ...
// 但这里又加回 "0x"，导致双前缀
cachedTx, err := getTransactionFromCache(queryCtx, "0x"+hash)
```

**SanitizeHash 实现** (`validator.go:203-209`):
```go
func SanitizeHash(hash string) string {
    hash = strings.TrimSpace(hash)
    hash = strings.ToLower(hash)
    hash = strings.TrimPrefix(hash, "0x")  // 移除 0x 前缀
    return hash  // 返回无 0x 前缀的 hash
}
```

**结果**: 缓存 key 变成 `cache:transaction:0x0x<hash>`，导致单个交易缓存永远无法命中。

---

### 2. TxNumber 计算异常（导致 ZSet Score 错误）

**位置**: `internal/models/transaction.go:1188-1244`

当从链上获取交易时（`by-hash` 流程），`ConvertGetTxResponseToTxDetailData` 函数计算 TxNumber：

```go
// 如果 blockResp 为 nil 或找不到交易，使用 legacy 计算
if txNumber == 0 {
    var hashOffset uint64
    hashHex := txHash[2:]  // 移除 "0x" 前缀
    for _, char := range hashHex {
        hashOffset = (hashOffset*31 + uint64(char)) % 1000000
    }
    txNumber = blockHeight*1000000 + hashOffset
}
```

**问题**: `TxNumber` 已经是 `blockHeight*1000000 + hashOffset` 的形式。

然后 `cacheTransaction` (`sync.go:3362`) 计算 score：
```go
compositeScore := float64(tx.BlockNumber)*1000000 + float64(tx.TxNumber)
                = blockNumber*1000000 + (blockNumber*1000000 + hashOffset)
                = 2*blockNumber*1000000 + hashOffset
```

**结果**: Score 几乎是正常值的两倍，导致 ZSet 排序混乱！

---

### 3. ZSet Member 使用完整 JSON（导致重复数据）

**位置**: `internal/service/sync.go:3364-3366`

```go
pipe.ZAdd(ctx, CacheKeyTransactionZSet, redis.Z{
    Score:  compositeScore,
    Member: string(txJSON),  // 使用完整 JSON 作为 member
})
```

**问题**: 从链上获取的交易 JSON 表示与同步进程缓存的可能不同（字段缺失或顺序不同），导致同一交易在 ZSet 中出现多次。

---

### 4. ZRemRangeByRank 清理机制

**位置**: `internal/service/sync.go:3370`

```go
pipe.ZRemRangeByRank(ctx, CacheKeyTransactionZSet, 0, -int64(CacheMaxSize)-1)
```

**CacheMaxSize = 1000**，只保留最新的 1000 条。

当 `by-hash` 调用 `cacheTransaction` 时：
1. 如果 score 异常高，新交易会被排在最前面
2. 如果 score 异常低，新交易会被排在最后面
3. 无论哪种情况，都可能导致原有数据被错误地移除

---

## 最可能导致问题的场景

### 场景 A: ZSet 过期后重建

1. 初始 `latest` 查询时，ZSet 已过期（TTL 5 分钟）
2. `by-hash` 从链上获取交易，TxNumber 计算异常
3. `cacheTransaction` 执行，ZSet 被重新创建，只包含这 1 条交易
4. 再次调用 `latest` 时，只返回这 1 条

### 场景 B: Score 异常导致排序混乱

1. 正常同步进程缓存的交易 score 为 `blockNumber*1000000 + txNumber`
2. `by-hash` 缓存的交易 score 为 `2*blockNumber*1000000 + hashOffset`
3. 后者 score 异常高，被排在最前面
4. ZRemRangeByRank 可能错误地移除了正常数据

---

## 修复方案

### 方案 1: 修复 Hash Key 前缀问题

**文件**: `internal/service/explorer.go:1149`

```go
// 修改前
cachedTx, err := getTransactionFromCache(queryCtx, "0x"+hash)

// 修改后
cachedTx, err := getTransactionFromCache(queryCtx, hash)  // hash 已包含 0x 前缀
```

**注意**: 需要检查 `cacheTransaction` 中保存的 hash 格式是否一致。

---

### 方案 2: 修复 TxNumber 导致的 Score 计算问题

**文件**: `internal/service/sync.go:3362`

当前代码：
```go
compositeScore := float64(tx.BlockNumber)*1000000 + float64(tx.TxNumber)
```

问题：如果 TxNumber 本身已经包含了 BlockNumber 信息（从链上获取时），会导致重复计算。

**修复方案 A**: 直接使用 TxNumber 作为 score（如果 TxNumber 全局唯一）
```go
compositeScore := float64(tx.TxNumber)
```

**修复方案 B**: 规范化 TxNumber 计算，确保从链上获取时 TxNumber 不包含 BlockNumber

检查 `ConvertGetTxResponseToTxDetailData` 中的 legacy 计算逻辑，考虑移除或修改：
```go
// 移除 legacy 计算，或者确保 TxNumber 不会重复包含 BlockNumber
if txNumber == 0 && blockResp != nil {
    // 只有在有 blockResp 时才计算 TxNumber
    txIndex, calculatedTxNumber, found := CalculateTxIndexFromBlock(blockResp, txHash)
    if found {
        txNumber = calculatedTxNumber
    } else {
        // 使用一个默认值，而不是 legacy 计算
        txNumber = 1  // 或从其他方式获取
    }
}
```

---

### 方案 3: 使用 Hash 作为 ZSet Member

**文件**: `internal/service/sync.go:3364-3366`

```go
// 修改前
pipe.ZAdd(ctx, CacheKeyTransactionZSet, redis.Z{
    Score:  compositeScore,
    Member: string(txJSON),
})

// 修改后
pipe.ZAdd(ctx, CacheKeyTransactionZSet, redis.Z{
    Score:  compositeScore,
    Member: tx.Hash,  // 使用 hash 作为 member，确保唯一性
})
```

**注意**: 这要求我们在其他地方存储完整的交易数据，比如在单独的 key 中：
```go
// 同时保存完整数据到单独的 key
pipe.Set(ctx, CacheKeyTransactionPrefix+tx.Hash, txJSON, CacheTransactionTTL)
```

---

### 方案 4: 优化缓存读取逻辑

**文件**: `internal/service/explorer.go:3521-3561`

当前 `getTransactionsFromCache` 从 ZSet 读取完整 JSON。如果使用 hash 作为 member，需要修改读取逻辑：

```go
func getTransactionsFromCache(ctx context.Context, limit int) ([]models.Transaction, error) {
    // 1. 从 ZSet 获取 hash 列表
    hashes, err := redisClient.ZRevRange(ctx, CacheKeyTransactionZSet, 0, int64(limit-1)).Result()
    if err != nil {
        return nil, err
    }

    // 2. 使用 pipeline 批量获取完整数据
    pipe := redisClient.Pipeline()
    cmds := make([]*redis.StringCmd, len(hashes))
    for i, hash := range hashes {
        cmds[i] = pipe.Get(ctx, CacheKeyTransactionPrefix+hash)
    }
    _, err = pipe.Exec(ctx)
    if err != nil {
        return nil, err
    }

    // 3. 解析数据
    txs := make([]models.Transaction, 0, len(hashes))
    for _, cmd := range cmds {
        data, err := cmd.Result()
        if err != nil {
            continue
        }
        var tx models.Transaction
        if err := sonic.Unmarshal([]byte(data), &tx); err != nil {
            continue
        }
        txs = append(txs, tx)
    }
    return txs, nil
}
```

---

## 推荐修复优先级

| 优先级 | 修复项 | 影响 | 复杂度 |
|--------|--------|------|--------|
| P0 | 修复 TxNumber/Score 计算问题 | 解决排序混乱的根本问题 | 中 |
| P1 | 修复 Hash Key 前缀问题 | 修复单个交易缓存无法命中 | 低 |
| P2 | 使用 Hash 作为 ZSet Member | 防止重复数据，减少内存占用 | 中 |
| P3 | 优化缓存读取逻辑 | 配合方案 3 的读取优化 | 中 |

---

## 验证方法

1. **添加日志监控**:
   - 在 `cacheTransaction` 中记录 BlockNumber, TxNumber, 计算的 Score
   - 在 `getTransactionsFromCache` 中记录返回的数据条数和 hash

2. **Redis 监控**:
   ```bash
   # 查看 ZSet 内容
   ZREVRANGE cache:transaction:zset 0 -1 WITHSCORES

   # 查看 ZSet 大小
   ZCARD cache:transaction:zset

   # 查看特定交易的缓存
   GET cache:transaction:0x<hash>
   ```

3. **单元测试**:
   - 测试从链上获取交易后的缓存行为
   - 测试 TxNumber 计算的正确性
   - 测试 ZSet 排序的正确性

---

## 相关代码文件

| 文件 | 相关功能 |
|------|----------|
| `internal/service/explorer.go:889-1024` | GetLatestTransactions 实现 |
| `internal/service/explorer.go:1130-1301` | GetTransactionByHash 实现 |
| `internal/service/explorer.go:3521-3612` | 缓存读取函数 |
| `internal/service/sync.go:3362-3379` | cacheTransaction 函数 |
| `internal/service/validator.go:203-209` | SanitizeHash 函数 |
| `internal/models/transaction.go:1057-1270` | ConvertGetTxResponseToTxDetailData |
| `internal/service/constants.go:133-157` | 缓存 Key 常量 |

---

## 附录：问题复现步骤

1. 确保 Redis 中 ZSet 有少量数据（如 10 条）
2. 调用 `api/v1/transaction/latest?page=1&page_size=10`，确认返回 10 条
3. 调用 `/api/v1/transaction/by-hash?hash=0xd4f622d56028530ce7dad3a6f59f3b99246af7cd2e7c21637d1ec23dc147d8b2`
4. 再次调用 `api/v1/transaction/latest?page=1&page_size=10`
5. 观察返回是否只有 1 条数据

**预期**: 两次 `latest` 调用应该返回相同的数据（假设数据没有变化）
**实际**: 第二次只返回 1 条数据
