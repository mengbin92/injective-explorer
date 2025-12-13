# 合约同步功能实现说明

## 概述

已在现有的同步服务中增加了合约同步功能，能够自动识别和同步 Injective 链上的 CosmWasm 智能合约。

## 实现位置

文件：`internal/service/sync.go`

## 新增功能

### 1. 合约识别和同步

在账户同步过程中，自动检测合约账户并同步合约信息：

- **触发时机**：当更新账户信息时，如果账户的 `CodeHash` 不为空，则识别为合约账户
- **同步内容**：
  - 合约地址
  - 代码哈希（CodeHash）
  - 合约类型（默认 cosmwasm）
  - 链ID（默认 injective-1）
  - 验证状态（默认 unverified）

### 2. 从交易中提取合约地址

从交易事件中提取合约地址，特别是：
- 合约实例化事件（`wasm.instantiate`）
- 合约执行事件（`wasm.execute`）
- 合约相关的消息类型

### 3. 新增方法

#### `syncContractFromAccount`
- **功能**：从账户信息同步合约到数据库
- **参数**：
  - `ctx`: 上下文
  - `address`: 合约地址
  - `codeHash`: 代码哈希
- **逻辑**：
  1. 检查合约是否已存在
  2. 如果存在且 CodeHash 有变化，更新合约信息
  3. 如果不存在，创建新的合约记录

#### `extractContractAddressesFromTransaction`
- **功能**：从交易中提取合约地址
- **参数**：`txData`: 交易数据
- **返回**：合约地址列表
- **逻辑**：
  1. 从事件中提取（查找 wasm、instantiate、execute 相关事件）
  2. 从事件属性中提取（查找 contract、code_id 相关属性）
  3. 从消息中提取（查找合约相关消息类型）

#### `syncContractsFromTransaction`
- **功能**：从交易中同步合约信息
- **参数**：
  - `ctx`: 上下文
  - `txData`: 交易数据
- **逻辑**：
  1. 提取交易中的合约地址
  2. 对每个地址，获取账户信息
  3. 检查是否为合约（CodeHash 不为空）
  4. 如果是合约，调用 `syncContractFromAccount` 同步

## 集成点

### 1. 账户同步时触发

在 `updateAccountsFromTransactionWithTx` 方法中：
```go
// Check if this is a contract account and sync contract information
if accountModel.CodeHash != "" {
    if err := s.syncContractFromAccount(ctx, address, accountModel.CodeHash); err != nil {
        // Log error but continue
    }
}
```

在 `updateAccountsFromTransaction` 方法中：
```go
// Check if this is a contract account and sync contract information
if accountModel.CodeHash != "" {
    if err := s.syncContractFromAccount(ctx, address, accountModel.CodeHash); err != nil {
        // Log error but continue
    }
}

// Also extract and sync contracts from transaction events
if err := s.syncContractsFromTransaction(ctx, txData); err != nil {
    // Log error but continue
}
```

## 数据流程

1. **交易处理** → 提取地址 → 更新账户
2. **账户更新** → 检查 CodeHash → 如果是合约 → 同步合约信息
3. **交易事件** → 提取合约地址 → 验证是否为合约 → 同步合约信息

## 数据库操作

- **创建合约**：当发现新合约时，在 `contracts` 表中创建记录
- **更新合约**：当合约的 CodeHash 发生变化时，更新合约记录
- **幂等性**：通过地址唯一索引确保不会重复创建

## 错误处理

- 所有合约同步操作都使用 `Warnf` 记录错误，不会中断主流程
- 即使合约同步失败，账户同步和交易处理仍会继续

## 性能考虑

1. **独立上下文**：合约同步使用独立的 context 和超时控制
2. **异步处理**：合约同步在账户更新后异步进行，不阻塞主流程
3. **批量处理**：可以批量处理多个合约地址（未来优化）

## 后续优化建议

1. **批量查询**：批量查询合约信息，减少数据库和链上查询
2. **缓存机制**：缓存已同步的合约信息，避免重复查询
3. **CodeID 获取**：实现从链上获取 CodeID 的逻辑
4. **WASM 代码获取**：实现从链上获取 WASM 字节码的逻辑
5. **合约事件监听**：专门监听合约部署事件，提高同步效率

## 测试建议

1. **单元测试**：测试合约识别逻辑
2. **集成测试**：测试从交易中提取合约地址
3. **端到端测试**：测试完整的合约同步流程
4. **性能测试**：测试大量合约同步的性能

## 注意事项

1. **CodeHash 格式**：CodeHash 使用 hex 编码的字符串格式存储
2. **地址格式**：只处理 `inj1` 格式的地址
3. **合约类型**：目前默认设置为 `cosmwasm`
4. **验证状态**：新合约默认状态为 `unverified`
