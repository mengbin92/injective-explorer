# Blockscout 交易详情页面实现分析

## 概述

本文档分析如何使用 `ethclient.TransactionByHash` 和 `ethclient.TransactionReceipt` 来实现类似 Blockscout 交易详情页面的展示功能。

参考页面：https://blockscout.injective.network/tx/0x5420a1d3b4b6ae089621f37d6941f5503d916bda98ad57fa0aaec9d907c4c577

## Blockscout 页面展示内容

### Details 标签页（主要信息）

1. **Transaction hash** - 交易哈希
2. **Status and method** - 状态（Success/Failed）和方法签名（如 `0xb9256d28`）
3. **Block** - 区块号（如 `147348889`）和确认数（如 `7618 Block confirmations`）
4. **Timestamp** - 时间戳（如 `Dec 26 2025 10:35:30 AM`）
5. **From** - 发送地址（如 `0xfFc239c2E8012e546D600c600C08cB8CdE3EE1EC`）
6. **Interacted with contract** - 交互的合约地址（如 `0x36825bf3Fbdf5a29E2d5148bfe7Dcf7B5639e320`）
7. **Value** - 交易金额（如 `0.000000000000000001 INJ`）
8. **Transaction fee** - 交易手续费（如 `0.00003153728 INJ`）
9. **Gas price** - Gas 价格（如 `0.16 Gwei`）
10. **Gas usage & limit** - Gas 使用量和限制（如 `197,108 / 199,431`，使用率 `98.84%`）
11. **Gas fees (Gwei)** - Gas 费用详情（如 `Base: 0.16`）

### 其他标签页

- **Token transfers** - Token 转账记录（需要解析 Logs）
- **Internal txns** - 内部交易（需要 trace 数据，这两个方法无法提供）
- **Logs** - 事件日志（可以从 Receipt 获取）
- **State** - 状态变化（需要 trace 数据）
- **Raw trace** - 原始 trace（需要 trace 数据）

## 数据来源分析

### 1. TransactionByHash 提供的数据

`types.Transaction` 结构包含以下字段：

```go
type Transaction struct {
    data txdata
    // caches
    hash atomic.Value
    size atomic.Value
    from atomic.Value
}

type txdata struct {
    AccountNonce    uint64          // nonce
    Price           *big.Int        // gas price
    GasLimit        uint64          // gas limit
    Recipient       *common.Address // 接收地址（To），nil 表示合约创建
    Amount          *big.Int        // 交易金额（Value）
    Payload         []byte          // 交易数据（Data/Input）
    V, R, S         *big.Int        // 签名信息
    Hash            *common.Hash    // 交易哈希
}
```

**可获取的信息：**
- ✅ Transaction hash: `tx.Hash()`
- ✅ To 地址: `tx.To()`（合约地址或接收地址）
- ✅ Value: `tx.Value()`
- ✅ Gas limit: `tx.Gas()`
- ✅ Gas price: `tx.GasPrice()`（Legacy 交易）或 `tx.GasTipCap()` + `tx.GasFeeCap()`（EIP-1559）
- ✅ Nonce: `tx.Nonce()`
- ✅ Data/Input: `tx.Data()`
- ✅ 方法签名: `tx.Data()` 的前 4 字节（如 `0xb9256d28`）
- ⚠️ From 地址: 需要通过签名恢复（使用 `types.Sender()`）

### 2. TransactionReceipt 提供的数据

`types.Receipt` 结构包含以下字段：

```go
type Receipt struct {
    // Consensus fields
    PostState         []byte          // 状态根（pre-Byzantium）
    Status            uint64          // 状态（0=失败，1=成功）
    CumulativeGasUsed uint64          // 累计 Gas 使用量
    Bloom             Bloom           // 日志布隆过滤器
    Logs              []*Log          // 事件日志
    
    // Implementation fields
    TxHash            common.Hash     // 交易哈希
    ContractAddress   common.Address  // 合约地址（如果是合约创建）
    GasUsed           uint64          // Gas 使用量
    BlockHash         common.Hash     // 区块哈希
    BlockNumber       *big.Int        // 区块号
    TransactionIndex  uint            // 交易在区块中的索引
}
```

**可获取的信息：**
- ✅ Status: `receipt.Status`（0=失败，1=成功）
- ✅ Gas used: `receipt.GasUsed`
- ✅ Block number: `receipt.BlockNumber`
- ✅ Block hash: `receipt.BlockHash`
- ✅ Transaction index: `receipt.TransactionIndex`
- ✅ Contract address: `receipt.ContractAddress`（如果是合约创建）
- ✅ Logs: `receipt.Logs`（事件日志，可用于 Token transfers）

### 3. 需要额外获取的数据

以下信息需要额外的 RPC 调用：

1. **Block 信息**（用于获取时间戳和确认数）：
   - `BlockByNumber` 或 `BlockByHash` 获取区块头
   - `Header.Time` 提供时间戳
   - 当前区块号 - 交易区块号 = 确认数

2. **From 地址**：
   - 使用 `types.Sender(signer, tx)` 从签名恢复
   - 需要 `chainID` 创建正确的 signer

3. **Transaction fee**：
   - `GasUsed * GasPrice`（Legacy）
   - `GasUsed * (BaseFee + PriorityFee)`（EIP-1559）

4. **Token transfers**：
   - 解析 `receipt.Logs` 中的 Transfer 事件
   - 需要合约 ABI 或标准 ERC20/ERC721 事件签名

5. **Internal transactions**：
   - ❌ 无法通过这两个方法获取
   - 需要 `debug_traceTransaction` 或 `trace_transaction`

6. **State changes**：
   - ❌ 无法通过这两个方法获取
   - 需要 trace 数据

## 实现方案

### 核心数据结构

```go
type TransactionDetails struct {
    // 基本信息
    Hash            string    `json:"hash"`
    Status          string    `json:"status"`          // "Success" or "Failed"
    Method          string    `json:"method"`          // 方法签名（如 "0xb9256d28"）
    
    // 区块信息
    BlockNumber     uint64    `json:"block_number"`
    BlockHash       string    `json:"block_hash"`
    Confirmations   uint64    `json:"confirmations"`
    Timestamp       time.Time `json:"timestamp"`
    
    // 地址信息
    From            string    `json:"from"`
    To              string    `json:"to"`              // 合约地址或接收地址
    ContractAddress string    `json:"contract_address"` // 如果是合约创建
    
    // 金额和费用
    Value           string    `json:"value"`           // 交易金额（wei）
    TransactionFee  string    `json:"transaction_fee"` // 交易手续费
    GasPrice        string    `json:"gas_price"`       // Gas 价格（wei）
    GasLimit        uint64    `json:"gas_limit"`
    GasUsed         uint64    `json:"gas_used"`
    GasUsedPercent  float64   `json:"gas_used_percent"`
    
    // Gas 费用详情（EIP-1559）
    BaseFee         string    `json:"base_fee,omitempty"`
    MaxFeePerGas    string    `json:"max_fee_per_gas,omitempty"`
    MaxPriorityFee  string    `json:"max_priority_fee,omitempty"`
    
    // 其他
    Nonce           uint64    `json:"nonce"`
    Input           string    `json:"input"`           // 交易数据
    TransactionIndex uint    `json:"transaction_index"`
    
    // 日志
    Logs            []LogInfo  `json:"logs"`
}

type LogInfo struct {
    Address     string   `json:"address"`
    Topics      []string `json:"topics"`
    Data        string   `json:"data"`
    Index       uint     `json:"index"`
    Removed     bool     `json:"removed"`
}
```

### 实现函数

```go
// GetTransactionDetails 获取完整的交易详情
func (c *EVMClient) GetTransactionDetails(ctx context.Context, txHash common.Hash) (*TransactionDetails, error) {
    // 1. 获取交易
    tx, isPending, err := c.client.TransactionByHash(ctx, txHash)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get transaction")
    }
    
    if isPending {
        return nil, fmt.Errorf("transaction is still pending")
    }
    
    // 2. 获取交易回执
    receipt, err := c.client.TransactionReceipt(ctx, txHash)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get transaction receipt")
    }
    
    // 3. 获取区块信息（用于时间戳和确认数）
    block, err := c.client.BlockByNumber(ctx, receipt.BlockNumber)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get block")
    }
    
    // 4. 获取当前区块号（用于计算确认数）
    currentBlockNumber, err := c.client.BlockNumber(ctx)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get current block number")
    }
    
    // 5. 恢复 From 地址
    signer := types.NewEIP155Signer(c.chainID)
    from, err := types.Sender(signer, tx)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to recover sender")
    }
    
    // 6. 提取方法签名
    method := ""
    if len(tx.Data()) >= 4 {
        method = "0x" + hex.EncodeToString(tx.Data()[:4])
    }
    
    // 7. 计算 Gas 价格和手续费
    gasPrice := tx.GasPrice()
    var baseFee, maxFeePerGas, maxPriorityFee *big.Int
    
    // 检查是否是 EIP-1559 交易
    if tx.Type() == types.DynamicFeeTxType {
        maxFeePerGas = tx.GasFeeCap()
        maxPriorityFee = tx.GasTipCap()
        // Base fee 需要从区块头获取
        if block.BaseFee() != nil {
            baseFee = block.BaseFee()
            // 实际支付的 gas price = baseFee + priorityFee
            gasPrice = new(big.Int).Add(baseFee, maxPriorityFee)
        }
    }
    
    // 计算交易手续费
    transactionFee := new(big.Int).Mul(
        big.NewInt(int64(receipt.GasUsed)),
        gasPrice,
    )
    
    // 8. 计算 Gas 使用率
    gasUsedPercent := float64(receipt.GasUsed) / float64(tx.Gas()) * 100
    
    // 9. 构建日志信息
    logs := make([]LogInfo, len(receipt.Logs))
    for i, log := range receipt.Logs {
        topics := make([]string, len(log.Topics))
        for j, topic := range log.Topics {
            topics[j] = topic.Hex()
        }
        logs[i] = LogInfo{
            Address: log.Address.Hex(),
            Topics:  topics,
            Data:    hex.EncodeToString(log.Data),
            Index:   log.Index,
            Removed: log.Removed,
        }
    }
    
    // 10. 构建结果
    details := &TransactionDetails{
        Hash:            txHash.Hex(),
        Status:          getStatusString(receipt.Status),
        Method:          method,
        BlockNumber:     receipt.BlockNumber.Uint64(),
        BlockHash:       receipt.BlockHash.Hex(),
        Confirmations:   currentBlockNumber - receipt.BlockNumber.Uint64(),
        Timestamp:       time.Unix(int64(block.Time()), 0),
        From:            from.Hex(),
        To:              getToAddress(tx, receipt),
        ContractAddress: receipt.ContractAddress.Hex(),
        Value:           tx.Value().String(),
        TransactionFee:  transactionFee.String(),
        GasPrice:        gasPrice.String(),
        GasLimit:        tx.Gas(),
        GasUsed:         receipt.GasUsed,
        GasUsedPercent:  gasUsedPercent,
        BaseFee:         baseFee.String(),
        MaxFeePerGas:    maxFeePerGas.String(),
        MaxPriorityFee:  maxPriorityFee.String(),
        Nonce:           tx.Nonce(),
        Input:           hex.EncodeToString(tx.Data()),
        TransactionIndex: receipt.TransactionIndex,
        Logs:            logs,
    }
    
    return details, nil
}

// 辅助函数
func getStatusString(status uint64) string {
    if status == 1 {
        return "Success"
    }
    return "Failed"
}

func getToAddress(tx *types.Transaction, receipt *types.Receipt) string {
    // 如果是合约创建，返回合约地址
    if receipt.ContractAddress != (common.Address{}) {
        return receipt.ContractAddress.Hex()
    }
    // 否则返回 To 地址
    if tx.To() != nil {
        return tx.To().Hex()
    }
    return ""
}
```

## 数据映射表

| Blockscout 显示项 | 数据来源 | 字段/方法 |
|------------------|---------|----------|
| Transaction hash | Transaction | `tx.Hash().Hex()` |
| Status | Receipt | `receipt.Status` (1=Success, 0=Failed) |
| Method | Transaction | `tx.Data()[:4]` (前4字节) |
| Block number | Receipt | `receipt.BlockNumber` |
| Confirmations | 计算 | `currentBlockNumber - receipt.BlockNumber` |
| Timestamp | Block | `block.Time()` |
| From | Transaction | `types.Sender(signer, tx)` |
| To/Contract | Transaction/Receipt | `tx.To()` 或 `receipt.ContractAddress` |
| Value | Transaction | `tx.Value()` |
| Transaction fee | 计算 | `receipt.GasUsed * gasPrice` |
| Gas price | Transaction | `tx.GasPrice()` 或 `baseFee + priorityFee` |
| Gas limit | Transaction | `tx.Gas()` |
| Gas used | Receipt | `receipt.GasUsed` |
| Gas used % | 计算 | `gasUsed / gasLimit * 100` |
| Logs | Receipt | `receipt.Logs` |

## 限制和不足

### 无法通过这两个方法获取的信息

1. **Internal transactions（内部交易）**
   - 需要 `debug_traceTransaction` 或 `trace_transaction`
   - 这些是高级 RPC 方法，不是所有节点都支持

2. **State changes（状态变化）**
   - 需要 trace 数据
   - 需要 `debug_traceTransaction` 或 `trace_transaction`

3. **Raw trace（原始 trace）**
   - 需要 trace 数据
   - 需要 `debug_traceTransaction` 或 `trace_transaction`

### 需要额外处理的信息

1. **Token transfers（Token 转账）**
   - 可以从 `receipt.Logs` 解析
   - 需要识别标准的 Transfer 事件（ERC20/ERC721）
   - 事件签名：`Transfer(address indexed from, address indexed to, uint256 value)`
   - Topic[0]: `keccak256("Transfer(address,address,uint256)")`

2. **方法名称解析**
   - 方法签名（如 `0xb9256d28`）需要合约 ABI 才能解析为方法名
   - 可以使用公共数据库（如 4byte.directory）或本地 ABI 数据库

3. **Gas 费用详情（EIP-1559）**
   - Base fee 需要从区块头获取
   - `block.BaseFee()` 提供基础费用

## 建议的实现步骤

1. **基础实现**
   - 使用 `TransactionByHash` 和 `TransactionReceipt` 获取基本信息
   - 实现 Details 标签页的核心功能

2. **增强功能**
   - 解析 Logs 提取 Token transfers
   - 添加方法名称解析（通过 ABI 或公共数据库）
   - 支持 EIP-1559 交易的 Gas 费用显示

3. **高级功能（需要额外 RPC）**
   - 如果节点支持，添加 `debug_traceTransaction` 获取内部交易
   - 实现 State 和 Raw trace 标签页

## 代码示例

参考 `provider/chain/evm_client.go` 中的现有实现：

- `GetTransactionByHash` - 已实现
- `GetTransactionReceiptByHash` - 已实现
- `GetTransactionSender` - 已实现（用于获取 From 地址）

可以在此基础上扩展实现 `GetTransactionDetails` 函数。

## 总结

### 可以满足的需求（使用 TransactionByHash + TransactionReceipt）

✅ **Details 标签页的核心信息：**
- Transaction hash
- Status（成功/失败）
- Method signature（方法签名）
- Block number 和 confirmations
- Timestamp
- From 地址
- To/Contract 地址
- Value（交易金额）
- Transaction fee（交易手续费）
- Gas price、Gas limit、Gas used
- Logs（事件日志）

✅ **Token transfers（通过解析 Logs）：**
- 可以从 `receipt.Logs` 中解析 ERC20/ERC721 Transfer 事件
- 需要识别标准事件签名

### 无法满足的需求（需要额外 RPC）

❌ **Internal transactions（内部交易）：**
- 需要 `debug_traceTransaction` 或 `trace_transaction`

❌ **State changes（状态变化）：**
- 需要 trace 数据

❌ **Raw trace（原始 trace）：**
- 需要 trace 数据

### 关键要点

1. **TransactionByHash** 提供交易本身的信息（From、To、Value、Gas 等）
2. **TransactionReceipt** 提供执行结果（Status、GasUsed、Logs 等）
3. **BlockByNumber** 提供时间戳和 BaseFee（EIP-1559）
4. **BlockNumber** 用于计算确认数
5. **From 地址** 需要通过签名恢复（`types.Sender`）

### 实现建议

1. 先实现基础功能，使用这两个方法获取 Details 页面的核心信息
2. 解析 Logs 实现 Token transfers 标签页
3. 如果节点支持，再添加 trace 相关功能实现 Internal txns 和 State 标签页

### 性能考虑

- `TransactionByHash` 和 `TransactionReceipt` 是标准 RPC 方法，性能较好
- `BlockByNumber` 可能需要额外调用，建议缓存区块信息
- Trace 相关方法（如 `debug_traceTransaction`）性能较差，建议异步处理或限制使用

