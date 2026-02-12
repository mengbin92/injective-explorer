# 合约同步增强技术方案

## 一、问题背景

服务已实现 EVM 交易详情同步，并具备将合约创建交易的合约地址写入 `contract_infos` 的能力。但实际测试发现：

**现象**：交易 hash `70AF765A4B927BE4EF9FC63338DAE13DF5E28B26592A2EFF4167A6BE11DF2B16` 为创建交易对操作，链上部署的合约地址为 `0x9772154BF1823353C829B43F9AD96CF9f720c768`，但 `contract_infos` 表中没有对应记录。

**根因分析**：当前实现存在以下缺口

| 缺口 | 说明 |
|------|------|
| 1. 仅支持直接合约创建 | 只识别 `receipt.ContractAddress`，Factory 模式（如 `createPair`）创建的交易对，新合约地址在 Event Logs 中，不在 receipt 里 |
| 2. ParseStatus 限制过严 | 合约保存要求 `ParseStatus == "success"`，解析失败走 Fallback 时不会保存 |
| 3. Fallback 未提取 ContractAddress | `getRawTransactionDetailsFallback` 未从 receipt 中提取 `ContractAddress`，即使有值也不会保存 |

---

## 二、设计目标

**确保所有在链上部署的合约（包括直接 CREATE 和 Factory 创建的交易对等）都能被正确识别并写入 `contract_infos` 表。**

---

## 三、整体设计思路

采用分层策略，按优先级扩展合约识别能力：

| 层级 | 数据来源 | 覆盖场景 |
|------|----------|----------|
| L1 | `receipt.ContractAddress` | 直接 CREATE 部署（`tx.To() == nil`） |
| L2 | Event Logs 解析 | Factory 创建（如 Uniswap V2 风格 `createPair`） |
| L3 | Fallback 补充 | 主流程解析失败但 receipt 可用时 |

---

## 四、详细技术方案

### 4.1 方案 A：从 Event Logs 解析 Factory 创建的合约地址

**适用场景**：通过 Factory 合约的 `createPair()` 等方法创建交易对，新 pair 地址在 `PairCreated` 等事件中。

**实现思路**：在 provider 层解析 receipt.Logs，识别已知的合约创建类事件，从中提取新部署的合约地址。

**实现要点**：

1. **支持的 event 签名**：
   - Uniswap V2 风格：`PairCreated(address,address,address,uint256)` 
     - topic0 = `keccak256("PairCreated(address,address,address,uint256)")`
   - 可根据 Injective 链实际 DEX 合约补充其他 event 签名

2. **实现位置**：
   - 新增辅助函数：`provider/chain/extract_contract_from_logs.go` 或合并到 `transaction_details.go`
   - 函数签名：`extractContractAddressFromLogs(receipt *types.Receipt) string`

3. **调用逻辑**：
   - 当 `receipt.ContractAddress` 为空时，调用 logs 解析
   - 若 logs 中解析出地址，则填充到 `TransactionDetails.ContractAddress`
   - 在 `getTransactionDetailsInternal` 和 `buildTransactionDetailsFromComponents` 两处调用

4. **可配置化（可选）**：
   - 在 `configs/config.yaml` 中增加 `evm.contract_creation_events` 配置，支持多 event 签名，便于扩展不同 DEX

**优点**：覆盖 Factory 创建交易对的常见场景  
**缺点**：需要维护已知 event 签名列表，不同 DEX 可能需额外配置

---

### 4.2 方案 B：Fallback 中补充 ContractAddress 提取

**适用场景**：主流程解析失败（如 signer 恢复失败）时走 Fallback，此时 receipt 仍可用，且可能有 `ContractAddress`。

**实现思路**：在 `getRawTransactionDetailsFallback` 中，当 `receipt != nil` 且 `receipt.ContractAddress != (common.Address{})` 时，设置 `details.ContractAddress`。

**修改位置**：`provider/chain/transaction_details.go`，在 fallback 函数内 `if receipt != nil` 分支中增加：

```go
if receipt.ContractAddress != (common.Address{}) {
    details.ContractAddress = receipt.ContractAddress.Hex()
}
```

**优点**：改动小，低风险  
**缺点**：仅覆盖直接 CREATE，不解决 Factory 场景

---

### 4.3 方案 C：放宽 ParseStatus 条件

**适用场景**：已有 `ContractAddress` 但主解析失败导致 `ParseStatus` 为 `partial` 或 `failed`，目前不会保存。

**实现思路**：在 `evm_details_sync.go` 中，将合约保存条件从「仅 success」放宽为「有地址 + 非完全失败」。

**当前逻辑**：
```go
if details.ContractAddress != "" && details.ParseStatus == "success" {
    s.saveContractInfoIfNotExists(ctx, evmTx, details)
}
```

**修改为**：
```go
if details.ContractAddress != "" && (details.ParseStatus == "success" || details.ParseStatus == "partial") {
    s.saveContractInfoIfNotExists(ctx, evmTx, details)
}
```

是否包含 `"failed"` 需评估：failed 时 `ContractAddress` 是否可靠（如来自 raw receipt 解析）。

**优点**：与 Fallback 提取互补，扩大有效数据保存范围  
**缺点**：若 `failed` 状态下地址不可靠，可能引入脏数据，建议仅放宽到 `partial`

---

### 4.4 方案 D：Logs 通用合约创建事件解析（可选扩展）

**适用场景**：不局限于 Uniswap V2，希望支持多种 DEX 或通用合约创建事件。

**实现思路**：维护可配置的 event 签名列表，从 logs 中解析合约地址：

1. 支持通过配置定义 event 签名及参数位置（如 `PairCreated` 第 3 个 indexed 参数为 pair 地址）
2. 遍历 `receipt.Logs`，匹配 topic0，解析出 `address` 类型参数
3. 若有多个合约创建事件，可取第一个或按策略选择

**优点**：扩展性强  
**缺点**：实现与维护成本较高，需处理多种 event 格式

---

## 五、推荐实施顺序

| 阶段 | 方案 | 工作量 | 收益 |
|------|------|--------|------|
| 阶段 1 | B + C | 小 | 覆盖直接 CREATE + 解析失败兜底 |
| 阶段 2 | A | 中 | 覆盖 Factory 创建交易对 |
| 阶段 3 | D（可选） | 大 | 支持多 DEX 配置化扩展 |

---

## 六、数据流与调用关系

```
TransactionReceipt
    ├── ContractAddress (直接 CREATE) ──────────────┐
    └── Logs                                        │
            └── Event 解析 (Factory 创建) ───────────┼──→ TransactionDetails.ContractAddress
                                                    │
getRawTransactionDetailsFallback                    │
    └── receipt.ContractAddress 补充 ───────────────┘

evm_details_sync.saveAndProcessDetails
    └── 条件：ContractAddress != "" && (ParseStatus == "success" || ParseStatus == "partial")
        └── saveContractInfoIfNotExists → contract_infos 表
```

---

## 七、待确认信息

1. **Injective 链上创建交易对的具体实现**：是否使用 Uniswap V2 风格 Factory？实际 event 名称与签名是什么？
2. **是否需要支持多 DEX**：若有多个 DEX 采用不同 event，是否需要可配置列表？
3. **ParseStatus 放宽策略**：是否接受 `partial`？是否考虑 `failed`？

---

## 八、风险与 mitigations

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 重复写入 | 同一合约被多次写入 | 使用 `FirstOrCreate` 或唯一约束 `(contract_address, chain_id)` |
| 错误 event 解析 | 解析到错误地址 | 校验地址格式；可选：对已知 Factory 做白名单 |
| 性能 | logs 解析增加 CPU | 仅在 receipt.ContractAddress 为空时解析；logs 数量通常有限 |

---

## 九、涉及文件

| 文件 | 变更类型 |
|------|----------|
| `provider/chain/transaction_details.go` | 修改：Fallback 补充 ContractAddress；可选：logs 解析 |
| `provider/chain/extract_contract_from_logs.go` | 新增（方案 A/D）：Event logs 解析 |
| `internal/service/evm_details_sync.go` | 修改：放宽 ParseStatus 条件 |
| `configs/config.yaml` | 可选：event 签名配置 |

---

## 十、参考资料

- Uniswap V2 PairCreated event: `PairCreated(address indexed token0, address indexed token1, address pair, uint)`
- EVM receipt 结构：`receipt.ContractAddress` 仅在 `tx.To() == nil`（CREATE 交易）时有值
- 现有实现：`internal/service/evm_details_sync.go` 第 765-768 行
