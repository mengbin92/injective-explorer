# EVM 交易转账显示解决方案

## 问题描述

通过小狐狸（MetaMask）发送到 injective 的交易都是 EVM 交易，在链上的 message type 显示为 `MsgEthereumTx`。用户希望通过 `/api/v1/account/transactions` 接口获取地址的转账交易，需要正确显示：

1. **主币转账**（native coin transfer）- 直接发送主币（如 INJ）
2. **ERC20 代币转账** - 调用 ERC20 合约的 transfer 方法

## 当前实现状态

### 1. 主币转账处理

系统已经通过 `evm_details_sync.go` 服务从 EVM RPC 获取交易详情，并更新 `transactions` 表的以下字段：

- `signer`: 交易发起者地址（从 EVM 交易的 `from` 字段提取）
- `to`: 接收者地址（从 EVM 交易的 `to` 字段提取）
- `values`: 转账金额（从 EVM 交易的 `value` 字段提取，格式为 `["1000000000000000000"]`）

**状态**: ✅ 主币转账应该已经在 `/api/v1/account/transactions` 接口中正确显示了

### 2. ERC20 代币转账处理

系统有 `token_transfers` 表存储 ERC20 转账信息，通过 `token_transfer_sync.go` 服务从 EVM logs 中解析 Transfer 事件并存储。

**问题**: `TransactionDTO` 中没有包含 ERC20 代币转账信息，所以接口返回的交易列表无法显示 ERC20 转账。

## 解决方案

### 方案一：在 TransactionDTO 中添加 token_transfers 字段（推荐）

**优点**：
- 清晰分离主币转账和代币转账
- 可以显示多个代币转账（一个交易可能包含多个 ERC20 转账）
- 保持向后兼容

**实现步骤**：

1. **修改 proto 定义** (`api/explorer/v1/explorer.proto`)：

```protobuf
message TransactionDTO {
  // ... 现有字段 ...
  string value = 24; // Transaction value (主币转账)
  EVMTransactionDTO evm_transaction = 25 [json_name = "evm_transaction"];
  
  // 新增：ERC20 代币转账列表
  repeated TokenTransferDTO token_transfers = 26 [json_name = "token_transfers"];
}

// 新增：代币转账 DTO
message TokenTransferDTO {
  string contract_address = 1 [json_name = "contract_address"]; // 代币合约地址
  string from_address = 2 [json_name = "from_address"];           // 发送者地址
  string to_address = 3 [json_name = "to_address"];               // 接收者地址
  string amount = 4;                                              // 转账金额（原始单位）
  string token_symbol = 5 [json_name = "token_symbol"];          // 代币符号（可选，从 contract_info 获取）
  string token_name = 6 [json_name = "token_name"];              // 代币名称（可选）
  int32 decimals = 7;                                            // 代币精度（可选）
  string method = 8;                                              // 转账类型：Transfer, Mint, Burn
}
```

2. **修改 ToTransactionDTO 函数** (`internal/models/dto.go`)：

```go
func ToTransactionDTO(tx *Transaction, isDetail bool) *pb.TransactionDTO {
    // ... 现有代码 ...
    
    dto := &pb.TransactionDTO{
        // ... 现有字段 ...
        Value: getFirstValue(tx.Values),
    }
    
    // 如果是 EVM 交易，查询关联的代币转账
    if tx.TxType == "MsgEthereumTx" || containsMsgEthereumTx(tx.MessageTypes) {
        tokenTransfers, err := getTokenTransfersForTransaction(tx.Hash)
        if err == nil && len(tokenTransfers) > 0 {
            dto.TokenTransfers = ToTokenTransferDTOs(tokenTransfers)
        }
    }
    
    return dto
}

// 查询交易关联的代币转账
func getTokenTransfersForTransaction(txHash string) ([]TokenTransfer, error) {
    var transfers []TokenTransfer
    err := db.Get().WithContext(context.Background()).
        Where("transaction_hash = ?", txHash).
        Order("log_index ASC").
        Find(&transfers).Error
    return transfers, err
}

// 转换为 TokenTransferDTO
func ToTokenTransferDTO(transfer *TokenTransfer) *pb.TokenTransferDTO {
    if transfer == nil {
        return nil
    }
    
    // 可选：从 contract_info 表获取代币信息
    var tokenSymbol, tokenName string
    var decimals int32
    var contractInfo ContractInfo
    if err := db.Get().WithContext(context.Background()).
        Where("contract_address = ?", transfer.ContractAddress).
        First(&contractInfo).Error; err == nil {
        tokenSymbol = contractInfo.TokenSymbol
        tokenName = contractInfo.TokenName
        decimals = int32(contractInfo.Decimals)
    }
    
    return &pb.TokenTransferDTO{
        ContractAddress: transfer.ContractAddress,
        FromAddress:     transfer.FromAddress,
        ToAddress:       transfer.ToAddress,
        Amount:          transfer.Amount,
        TokenSymbol:     tokenSymbol,
        TokenName:       tokenName,
        Decimals:        decimals,
        Method:          transfer.GetMethod(),
    }
}

func ToTokenTransferDTOs(transfers []TokenTransfer) []*pb.TokenTransferDTO {
    if len(transfers) == 0 {
        return nil
    }
    dtos := make([]*pb.TokenTransferDTO, 0, len(transfers))
    for i := range transfers {
        dtos = append(dtos, ToTokenTransferDTO(&transfers[i]))
    }
    return dtos
}
```

3. **优化查询性能**：

在 `GetAccountTransactions` 中批量查询代币转账，避免 N+1 查询：

```go
func (s *ExplorerService) GetAccountTransactions(ctx context.Context, req *pb.GetAccountTransactionsRequest) (*pb.Reply, error) {
    // ... 现有查询逻辑 ...
    
    // 获取交易列表后，批量查询代币转账
    if len(dbTxs) > 0 {
        txHashes := make([]string, 0, len(dbTxs))
        for _, tx := range dbTxs {
            // 只查询 EVM 交易
            if tx.TxType == "MsgEthereumTx" || containsMsgEthereumTx(tx.MessageTypes) {
                txHashes = append(txHashes, tx.Hash)
            }
        }
        
        // 批量查询代币转账
        tokenTransfersMap := s.batchGetTokenTransfers(ctx, txHashes)
        
        // 转换为 DTO 时关联代币转账
        txDTOs := make([]*pb.TransactionDTO, 0, len(dbTxs))
        for i := range dbTxs {
            dto := models.ToTransactionDTO(&dbTxs[i], false)
            if transfers, ok := tokenTransfersMap[dbTxs[i].Hash]; ok {
                dto.TokenTransfers = models.ToTokenTransferDTOs(transfers)
            }
            txDTOs = append(txDTOs, dto)
        }
        
        // ... 返回结果 ...
    }
}

// 批量查询代币转账
func (s *ExplorerService) batchGetTokenTransfers(ctx context.Context, txHashes []string) map[string][]models.TokenTransfer {
    if len(txHashes) == 0 {
        return make(map[string][]models.TokenTransfer)
    }
    
    var transfers []models.TokenTransfer
    err := db.Get().WithContext(ctx).
        Where("transaction_hash IN ?", txHashes).
        Order("transaction_hash, log_index ASC").
        Find(&transfers).Error
    
    if err != nil {
        global.Logger.Warnf("failed to batch query token transfers: %v", err)
        return make(map[string][]models.TokenTransfer)
    }
    
    // 按交易 hash 分组
    result := make(map[string][]models.TokenTransfer)
    for i := range transfers {
        hash := transfers[i].TransactionHash
        result[hash] = append(result[hash], transfers[i])
    }
    
    return result
}
```

### 方案二：将 ERC20 转账合并到 values 字段

**优点**：
- 不需要修改 proto 定义
- 实现简单

**缺点**：
- 无法区分主币转账和代币转账
- 无法显示代币符号、名称等信息

**实现**：

在 `ToTransactionDTO` 中，将 ERC20 转账金额合并到 `values` 字段：

```go
func ToTransactionDTO(tx *Transaction, isDetail bool) *pb.TransactionDTO {
    // ... 现有代码 ...
    
    values := tx.Values
    
    // 如果是 EVM 交易，查询并合并代币转账
    if tx.TxType == "MsgEthereumTx" || containsMsgEthereumTx(tx.MessageTypes) {
        tokenTransfers, err := getTokenTransfersForTransaction(tx.Hash)
        if err == nil {
            for _, transfer := range tokenTransfers {
                // 格式：amount + contractAddress (例如: "1000000peggy0x1234...")
                valueStr := transfer.Amount + transfer.ContractAddress
                values = append(values, valueStr)
            }
        }
    }
    
    dto := &pb.TransactionDTO{
        // ... 其他字段 ...
        Value: getFirstValue(values), // 只返回第一个值（主币）
        // 注意：values 字段在 TransactionDTO 中不存在，需要添加到 proto
    }
    
    return dto
}
```

## 推荐方案

**推荐使用方案一**，原因：
1. 清晰分离主币转账和代币转账
2. 可以显示完整的代币信息（符号、名称、精度）
3. 支持一个交易包含多个代币转账的场景
4. 更符合 API 设计最佳实践

## 测试建议

1. **测试主币转账**：
   - 通过小狐狸发送主币转账
   - 验证 `/api/v1/account/transactions` 返回的 `value` 字段正确

2. **测试 ERC20 转账**：
   - 通过小狐狸发送 ERC20 代币转账
   - 验证返回的 `token_transfers` 数组包含正确的转账信息

3. **测试混合场景**：
   - 一个交易同时包含主币转账和 ERC20 转账
   - 验证两者都正确显示

4. **测试性能**：
   - 批量查询大量交易时，验证性能不受影响
   - 确保使用了批量查询优化

## 重要问题：交易类型过滤

### 问题描述

当前的 `/api/v1/account/transactions` 接口支持通过 `tx_types` 参数过滤交易类型，例如：
- `tx_types=MsgSend` - 过滤 Cosmos 标准转账 (`/cosmos.bank.v1beta1.MsgSend`)
- `tx_types=MsgTransfer` - 过滤 IBC 转账 (`/ibc.applications.transfer.v1.MsgTransfer`)

**但是**，通过小狐狸发送的 EVM 交易的 message type 都是 `MsgEthereumTx`，无法通过这种方式过滤出转账交易。如果用户想获取"所有转账交易"，使用 `tx_types=MsgSend` 这样的参数，就无法获取到 EVM 的主币转账和 ERC20 代币转账。

### 解决方案：扩展过滤逻辑

需要修改 `GetAccountTransactions` 函数，当用户请求转账类型的交易时，也包含 EVM 转账交易。

**实现步骤**：

1. **识别转账类型的消息类型**：

```go
// 转账类型的消息类型列表
var transferMessageTypes = map[string]bool{
    "/cosmos.bank.v1beta1.MsgSend":                    true,
    "/ibc.applications.transfer.v1.MsgTransfer":       true,
    "/injective.peggy.v1.MsgSendToEth":               true,
    "/injective.exchange.v1beta1.MsgDeposit":         true,
    "/injective.exchange.v1beta1.MsgWithdraw":        true,
    "/injective.exchange.v1beta1.MsgSubaccountTransfer": true,
    "/injective.exchange.v1beta1.MsgExternalTransfer": true,
}

// 检查是否是转账类型的消息
func isTransferMessageType(msgType string) bool {
    // 检查完整类型 URL
    if transferMessageTypes[msgType] {
        return true
    }
    // 检查短名称
    normalized := normalizeMessageType(msgType)
    return transferMessageTypes[normalized]
}
```

2. **修改过滤逻辑** (`internal/service/explorer.go`)：

```go
func (s *ExplorerService) GetAccountTransactions(ctx context.Context, req *pb.GetAccountTransactionsRequest) (*pb.Reply, error) {
    // ... 现有代码 ...
    
    if hasTxTypeFilter {
        dbType := db.Get().Dialector.Name()
        conditions := make([]string, 0, len(req.TxTypes))
        args := make([]interface{}, 0, len(req.TxTypes)*2)
        
        // 检查是否请求了转账类型的交易
        hasTransferFilter := false
        for _, msgType := range req.TxTypes {
            normalizedType := normalizeMessageType(msgType)
            if isTransferMessageType(normalizedType) {
                hasTransferFilter = true
                break
            }
        }
        
        for _, msgType := range req.TxTypes {
            normalizedType := normalizeMessageType(msgType)
            
            // 标准消息类型过滤
            if dbType == "mysql" {
                conditions = append(conditions, "(JSON_CONTAINS(message_types, ?) OR JSON_SEARCH(message_types, 'one', ?) IS NOT NULL)")
                args = append(args, fmt.Sprintf(`"%s"`, normalizedType), fmt.Sprintf(`%%"%s"%%`, normalizedType))
            } else if dbType == "postgres" {
                conditions = append(conditions, "(message_types @> ?::jsonb OR message_types::text LIKE ?)")
                args = append(args, fmt.Sprintf(`["%s"]`, normalizedType), fmt.Sprintf(`%%"%s"%%`, normalizedType))
            } else {
                conditions = append(conditions, "message_types LIKE ?")
                args = append(args, fmt.Sprintf(`%%"%s"%%`, normalizedType))
            }
        }
        
        // 如果请求了转账类型的交易，也需要包含 EVM 转账交易
        if hasTransferFilter {
            // EVM 主币转账：to 字段不为空，values 字段不为空
            evmNativeTransferCondition := "((tx_type = 'MsgEthereumTx' OR JSON_SEARCH(message_types, 'one', '%MsgEthereumTx%') IS NOT NULL) AND `to` != '' AND `to` IS NOT NULL AND JSON_LENGTH(values) > 0)"
            
            // EVM ERC20 转账：在 token_transfers 表中有记录
            // 需要通过 JOIN 或子查询来检查
            evmTokenTransferCondition := `EXISTS (
                SELECT 1 FROM token_transfers tt 
                WHERE tt.transaction_hash = transactions.hash 
                AND (tt.from_address = ? OR tt.to_address = ?)
            )`
            
            if dbType == "mysql" {
                conditions = append(conditions, evmNativeTransferCondition)
                conditions = append(conditions, evmTokenTransferCondition)
                args = append(args, dbQueryAddress, dbQueryAddress)
            } else if dbType == "postgres" {
                // PostgreSQL 版本
                evmNativeTransferConditionPG := "((tx_type = 'MsgEthereumTx' OR message_types::text LIKE '%MsgEthereumTx%') AND \"to\" != '' AND \"to\" IS NOT NULL AND jsonb_array_length(values::jsonb) > 0)"
                conditions = append(conditions, evmNativeTransferConditionPG)
                conditions = append(conditions, evmTokenTransferCondition)
                args = append(args, dbQueryAddress, dbQueryAddress)
            }
        }
        
        if len(conditions) > 0 {
            baseQuery = baseQuery.Where("("+strings.Join(conditions, " OR ")+")", args...)
        }
    }
    
    // ... 后续代码 ...
}
```

3. **优化性能**：

为了避免 JOIN 查询影响性能，可以考虑：
- 在 `transactions` 表中添加一个 `has_token_transfer` 布尔字段，在同步时更新
- 或者使用 LEFT JOIN 但限制在特定条件下才执行

**更优的实现**（推荐）：

```go
// 在 transactions 表中添加字段（需要数据库迁移）
// has_token_transfer BOOLEAN DEFAULT FALSE
// 在 token_transfer_sync.go 中同步时更新这个字段

// 然后在过滤时直接使用这个字段
if hasTransferFilter {
    evmNativeTransferCondition := "((tx_type = 'MsgEthereumTx' OR message_types LIKE '%MsgEthereumTx%') AND `to` != '' AND `to` IS NOT NULL AND JSON_LENGTH(values) > 0)"
    evmTokenTransferCondition := "(tx_type = 'MsgEthereumTx' AND has_token_transfer = TRUE)"
    
    conditions = append(conditions, evmNativeTransferCondition)
    conditions = append(conditions, evmTokenTransferCondition)
}
```

## 注意事项

1. **数据同步**：确保 `token_transfer_sync.go` 服务正常运行，代币转账数据已同步到 `token_transfers` 表

2. **地址格式**：确保代币转账中的地址格式与 `transactions` 表中的地址格式一致（都是 EVM 格式 0x...）

3. **空值处理**：如果交易没有代币转账，`token_transfers` 字段应该返回空数组 `[]`，而不是 `null`

4. **向后兼容**：新增字段应该是可选的，不影响现有客户端

5. **性能优化**：EVM 转账过滤可能涉及 JOIN 查询，需要考虑性能影响。建议添加 `has_token_transfer` 字段来优化查询性能

6. **过滤逻辑**：当用户请求转账类型的交易时（如 `tx_types=MsgSend`），需要同时包含：
   - Cosmos 标准转账 (`MsgSend`)
   - IBC 转账 (`MsgTransfer`)
   - EVM 主币转账（`MsgEthereumTx` + `to` 不为空 + `values` 不为空）
   - EVM ERC20 转账（`MsgEthereumTx` + `token_transfers` 表中有记录）
