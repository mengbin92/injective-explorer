# Injective Explorer / Indexer 中 tx_number 与 Action 顺序技术方案

## 1. 背景与目标

在 Injective（Cosmos SDK + EVM 混合执行）链上，**交易在区块内的执行顺序**对于 Explorer 展示、Indexer 数据一致性、EVM/Cosmos 行为对齐至关重要。

Injective 链上**不存在原生的 tx_number 字段**，Explorer（如 injscan）中的 `tx_number` 属于 **Indexer / UI 层派生字段**。

本方案目标：

* 定义 **tx_number 的权威计算方式**
* 基于 **ChainClientV2** 实现稳定获取 `tx_index`、`msg_index`
* 构建 **Block → Tx → Msg → Action** 的全序执行模型
* 支持 Cosmos Tx 与 EVM Tx 的统一排序

---

## 2. 核心概念定义

### 2.1 tx_index（链上真实顺序）

* 含义：交易在 **区块内的执行顺序**
* 范围：0-based（从 0 开始）
* 权威来源：CometBFT Block 数据中的 `block.data.txs` 顺序

> tx_index 是整个执行顺序体系的**唯一权威来源**

---

### 2.2 tx_number（Explorer / Indexer 层）

* 含义：交易在区块内的 **人类可读序号**
* 计算方式：

```text
tx_number = tx_index + 1
```

* 特点：

  * 仅用于展示 / 排序
  * 不参与链上语义

---

### 2.3 msg_index

* 含义：交易内部 Msg 的执行顺序
* 范围：0-based
* 计算方式：

```text
tx.body.messages[i] → msg_index = i
```

Cosmos SDK 保证 Msg 按数组顺序严格执行。

---

### 2.4 Action 顺序模型

在一个区块内，Injective 的执行顺序遵循：

```text
for tx in block.txs (tx_index asc):
    for msg in tx.body.messages (msg_index asc):
        execute(msg)
```

对于 `MsgEthereumTx`：

```text
execute EVM
  → receipts
  → logs (logIndex asc)
```

---

## 3. ChainClientV2 能力分析

### 3.1 ChainClientV2 的定位

ChainClientV2 更偏向 **Indexer / 业务层抽象**，而非 CometBFT RPC 的完整封装：

* ✅ 支持 Tx 查询、Indexer 数据
* ❌ **不保证返回 tx_index**

---

### 3.2 关键结论

> **tx_index 无法仅通过 `GetTx(hash)` 获得**

要获取真实执行顺序，必须：

* 以 **Block 为驱动**
* 从 Block 的 tx 数组顺序中计算 tx_index

---

## 4. tx_index 的权威获取方案

### 4.1 推荐方案：Block 驱动（强烈推荐）

#### Step 1：通过 ChainClientV2 获取区块

```go
block, err := chainClient.ChainClientV2.GetBlock(ctx, height)
```

最终可获得：

```go
block.Block.Data.Txs [][]byte
```

---

#### Step 2：遍历区块交易，计算 tx_index

```go
for txIndex, txBytes := range block.Block.Data.Txs {
    // txIndex 即为 0-based tx_index
}
```

注意事项：

* 遍历顺序即执行顺序
* 不允许排序 / 重排
* 不依赖 tx hash

---

#### Step 3：解析交易内容

```go
tx, err := txConfig.TxDecoder()(txBytes)
```

同时可计算：

```go
txHash := hex.EncodeToString(tmhash.Sum(txBytes))
```

---

## 5. tx_number 与 msg_index 的计算

### 5.1 tx_number

```go
txNumber := txIndex + 1
```

该值与 injscan 等 Explorer 行为一致。

---

### 5.2 msg_index

```go
for msgIndex, msg := range tx.Body.Messages {
    // msgIndex 即为 0-based msg_index
}
```

---

### 5.3 MsgExec / Batch 消息处理策略

结构示例：

```text
Tx
 └─ MsgExec
     ├─ inner Msg[0]
     ├─ inner Msg[1]
```

推荐建模方式：

```text
tx_index = X
msg_index = outer_msg_index
inner_index = i
```

而不是将 inner Msg 当作新的 Tx。

---

## 6. Action 全序执行模型

### 6.1 推荐的 Action 排序 Key（逻辑全序）

```text
(block_height, tx_index, msg_index, inner_index)
```

排序规则：

```sql
ORDER BY
  block_height,
  tx_index,
  msg_index,
  inner_index
```

优势：

* 100% 可复现
* Cosmos / EVM 统一
* 不受 tx/msg 数量变化影响

---

### 6.2 数字型 action_number（可选）

```text
action_number =
  tx_index   * TX_MSG_STRIDE
+ msg_index  * MSG_ACTION_STRIDE
+ inner_index
```

示例配置：

```text
TX_MSG_STRIDE     = 1000
MSG_ACTION_STRIDE = 100
```

⚠️ 需要确保 stride 大于最大可能值。

---

## 7. 数据结构设计建议

```go
type IndexedTx struct {
    BlockHeight int64
    TxHash      string
    TxIndex     int // 0-based
    TxNumber    int // TxIndex + 1
}

type IndexedMsg struct {
    TxHash   string
    TxIndex  int
    MsgIndex int
    MsgType  string
}

type IndexedAction struct {
    BlockHeight int64
    TxIndex     int
    MsgIndex    int
    InnerIndex  int
}
```

---

## 8. 常见错误与规避

* ❌ 使用 tx hash 排序（与执行顺序无关）
* ❌ 使用 Indexer 返回的 tx 列表顺序
* ❌ 通过 GetTx(hash) 推断 tx_index

> **Block → tx_index 是唯一可信执行顺序来源**

---

## 9. 方案收益

通过本方案可以确保：

* tx_number 与 injscan 行为完全一致
* Cosmos / EVM 交易统一排序
* Block → Tx → Msg → Action 时间线可重放
* 为 Balance Change、Validator 行为、EVM Trace 提供可靠基础

---

## 10. 后续可扩展方向

* Action → Balance Change 映射
* EVM logIndex 与 Cosmos Event 对齐
* Block 内全局 Execution Timeline 构建
* injscan 级 Action Explorer 实现

---

**本方案适用于 Injective Indexer / Explorer / 数据分析系统的生产级实现。**
