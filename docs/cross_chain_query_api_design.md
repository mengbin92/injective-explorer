# 跨链交易查询接口设计方案

## 1. 概述

本文档设计实现 Injective Explorer 中的三个跨链交易查询接口，与项目现有接口风格保持一致。

### 1.1 目标接口

1. **GetIBCTransferTxs** - 查询 IBC 转账交易记录
2. **GetPeggyDepositTxs** - 查询 Peggy 存款交易记录（以太坊→Injective）
3. **GetPeggyWithdrawalTxs** - 查询 Peggy 取款交易记录（Injective→以太坊）

### 1.2 设计原则

- 遵循项目现有的 proto 定义和 HTTP 路由规范
- 使用统一的 `Reply` 返回类型
- 复用现有的 `PaginationRequest` 分页模式
- 保持与现有 `ExplorerService` 的实现风格一致
- 复用现有的数据库模型和查询逻辑

## 2. Proto 接口定义

### 2.1 文件位置

在 `api/explorer/v1/explorer.proto` 中添加新的 RPC 方法定义。

### 2.2 接口定义

```protobuf
// Cross-chain transaction query interfaces
rpc GetIBCTransferTxs(GetIBCTransferTxsRequest) returns (Reply) {
  option (google.api.http) = {
    get: "/api/v1/ibc/transfers"
  };
}

rpc GetPeggyDepositTxs(GetPeggyDepositTxsRequest) returns (Reply) {
  option (google.api.http) = {
    get: "/api/v1/peggy/deposits"
  };
}

rpc GetPeggyWithdrawalTxs(GetPeggyWithdrawalTxsRequest) returns (Reply) {
  option (google.api.http) = {
    get: "/api/v1/peggy/withdrawals"
  };
}
```

### 2.3 请求消息定义

**统一使用现有的 PaginationRequest 模式：**

```protobuf
// GetIBCTransferTxsRequest requests IBC transfer transactions
message GetIBCTransferTxsRequest {
  // Sender address (optional filter)
  string sender = 1;
  // Receiver address (optional filter)
  string receiver = 2;
  // Source channel (optional filter)
  string src_channel = 3 [json_name = "src_channel"];
  // Source port (optional filter)
  string src_port = 4 [json_name = "src_port"];
  // Destination channel (optional filter)
  string dest_channel = 5 [json_name = "dest_channel"];
  // Destination port (optional filter)
  string dest_port = 6 [json_name = "dest_port"];
  // Pagination parameters
  PaginationRequest pagination = 7;
}

// GetPeggyDepositTxsRequest requests Peggy deposit transactions
message GetPeggyDepositTxsRequest {
  // Sender address (Ethereum address, optional filter)
  string sender = 1;
  // Receiver address (Injective address, optional filter)
  string receiver = 2;
  // Pagination parameters
  PaginationRequest pagination = 3;
}

// GetPeggyWithdrawalTxsRequest requests Peggy withdrawal transactions
message GetPeggyWithdrawalTxsRequest {
  // Sender address (Injective address, optional filter)
  string sender = 1;
  // Receiver address (Ethereum address, optional filter)
  string receiver = 2;
  // Pagination parameters
  PaginationRequest pagination = 3;
}
```

### 2.4 响应数据结构

响应使用 `Reply` 类型，`data` 字段包含以下结构：

```json
{
  "data": {
    "total": 100,
    "transactions": [
      {
        // IBCTransferTx / PeggyDepositTx / PeggyWithdrawalTx 字段
      }
    ],
    "pagination": {
      "next_cursor": "...",
      "has_more": true,
      "total": 100,
      "count": 20
    }
  }
}
```

## 3. 数据模型

### 3.1 数据库表

项目已存在以下表（参考 `docs/database_schema.sql`）：

- `ibc_transfer_txs` - IBC 转账交易表
- `peggy_deposit_txs` - Peggy 存款交易表
- `peggy_withdrawal_txs` - Peggy 取款交易表

### 3.2 GORM 模型

需要在 `internal/models/` 目录下创建或确认以下模型：

- `IBCTransferTx` - IBC 转账交易模型
- `PeggyDepositTx` - Peggy 存款交易模型
- `PeggyWithdrawalTx` - Peggy 取款交易模型

模型定义应遵循项目现有风格（参考 `internal/models/block.go` 和 `internal/models/transaction.go`）。

**模型文件位置：**
- `internal/models/ibc_transfer_tx.go`
- `internal/models/peggy_deposit_tx.go`
- `internal/models/peggy_withdrawal_tx.go`

## 4. 服务层实现

### 4.1 实现位置

在 `internal/service/explorer.go` 的 `ExplorerService` 中直接添加三个方法，保持与现有接口实现风格一致：

- `GetIBCTransferTxs`
- `GetPeggyDepositTxs`
- `GetPeggyWithdrawalTxs`

### 4.2 实现模式

参考 `GetLatestBlocks` 和 `GetAccountTransactions` 的实现模式：

1. **参数验证**：
   - 检查请求是否为 nil
   - 验证地址格式（如果提供）
   - 使用 `normalizePagination` 规范化分页参数

2. **数据库查询**：
   - 构建查询条件（WHERE 子句）
   - 应用过滤条件（sender, receiver, channel, port 等）
   - 按 `block_timestamp DESC` 排序（最新优先）
   - 应用分页限制

3. **响应构建**：
   - 查询总数（用于分页信息）
   - 使用模型方法转换为 DTO
   - 使用 `SuccessReplyWithPagination` 返回成功响应

### 4.3 示例实现

```go
func (s *ExplorerService) GetIBCTransferTxs(ctx context.Context, req *pb.GetIBCTransferTxsRequest) (*pb.Reply, error) {
    // 参数验证
    if req == nil {
        return ErrorReply(http.StatusBadRequest, "request is nil", nil), nil
    }

    // 分页标准化（复用现有逻辑）
    limit, offset := normalizePagination(req.Pagination, 20, 100)

    // 构建查询
    query := db.Get().Model(&models.IBCTransferTx{})

    // 动态添加过滤条件
    if req.Sender != "" {
        if err := ValidateAddress(req.Sender); err != nil {
            return ErrorReply(http.StatusBadRequest, "invalid sender address", err), nil
        }
        query = query.Where("sender = ?", req.Sender)
    }
    if req.Receiver != "" {
        query = query.Where("receiver = ?", req.Receiver)
    }
    if req.SrcChannel != "" {
        query = query.Where("source_channel = ?", req.SrcChannel)
    }
    if req.SrcPort != "" {
        query = query.Where("source_port = ?", req.SrcPort)
    }
    if req.DestChannel != "" {
        query = query.Where("destination_channel = ?", req.DestChannel)
    }
    if req.DestPort != "" {
        query = query.Where("destination_port = ?", req.DestPort)
    }

    // 查询总数
    var total int64
    if err := query.Count(&total).Error; err != nil {
        global.Logger.Errorf("count ibc transfers failed: %v", err)
        return ErrorReply(http.StatusInternalServerError, "query failed", err), nil
    }

    // 查询数据
    var txs []*models.IBCTransferTx
    if err := query.Order("block_timestamp DESC").Limit(limit).Offset(offset).Find(&txs).Error; err != nil {
        global.Logger.Errorf("query ibc transfers failed: %v", err)
        return ErrorReply(http.StatusInternalServerError, "query failed", err), nil
    }

    // 转换为 DTO
    data := make([]*pb.IBCTransferTxDTO, len(txs))
    for i, tx := range txs {
        data[i] = tx.ToProto()
    }

    return SuccessReplyWithPagination(total, data, req.Pagination), nil
}
```

## 5. DTO 转换

### 5.1 DTO 结构（在 proto 中定义）

在 `api/explorer/v1/explorer.proto` 中添加 DTO 消息定义：

```protobuf
// IBCTransferTxDTO represents IBC transfer transaction in API response
message IBCTransferTxDTO {
  string sender = 1;
  string receiver = 2;
  string source_port = 2 [json_name = "source_port"];
  string source_channel = 3 [json_name = "source_channel"];
  string destination_port = 4 [json_name = "destination_port"];
  string destination_channel = 5 [json_name = "destination_channel"];
  string amount = 6;
  string denom = 7;
  string timeout_height = 8 [json_name = "timeout_height"];
  uint64 timeout_timestamp = 9 [json_name = "timeout_timestamp"];
  uint64 packet_sequence = 10 [json_name = "packet_sequence"];
  bytes data_hex = 11 [json_name = "data_hex"];
  string state = 12;
  repeated string tx_hashes = 13 [json_name = "tx_hashes"];
  string created_at = 14 [json_name = "created_at"];
  string updated_at = 15 [json_name = "updated_at"];
}

// PeggyDepositTxDTO represents Peggy deposit transaction in API response
message PeggyDepositTxDTO {
  string sender = 1;
  string receiver = 2;
  uint64 event_nonce = 3 [json_name = "event_nonce"];
  uint64 event_height = 4 [json_name = "event_height"];
  string amount = 5;
  string denom = 6;
  string orchestrator_address = 7 [json_name = "orchestrator_address"];
  string state = 8;
  int32 claim_type = 9 [json_name = "claim_type"];
  repeated string tx_hashes = 10 [json_name = "tx_hashes"];
  string created_at = 11 [json_name = "created_at"];
  string updated_at = 12 [json_name = "updated_at"];
}

// PeggyWithdrawalTxDTO represents Peggy withdrawal transaction in API response
message PeggyWithdrawalTxDTO {
  string sender = 1;
  string receiver = 2;
  string amount = 3;
  string denom = 4;
  string bridge_fee = 5 [json_name = "bridge_fee"];
  uint64 outgoing_tx_id = 6 [json_name = "outgoing_tx_id"];
  uint64 batch_timeout = 7 [json_name = "batch_timeout"];
  uint64 batch_nonce = 8 [json_name = "batch_nonce"];
  string orchestrator_address = 9 [json_name = "orchestrator_address"];
  uint64 event_nonce = 10 [json_name = "event_nonce"];
  uint64 event_height = 11 [json_name = "event_height"];
  string state = 12;
  int32 claim_type = 13 [json_name = "claim_type"];
  repeated string tx_hashes = 14 [json_name = "tx_hashes"];
  string created_at = 15 [json_name = "created_at"];
  string updated_at = 16 [json_name = "updated_at"];
}
```

### 5.2 模型转换方法

在模型文件中添加 `ToProto()` 方法，保持与现有 `ToBlockDTO`, `ToTransactionDTO` 一致的风格：

```go
// internal/models/ibc_transfer_tx.go

func (t *IBCTransferTx) ToProto() *pb.IBCTransferTxDTO {
    return &pb.IBCTransferTxDTO{
        Sender:             t.Sender,
        Receiver:           t.Receiver,
        SourcePort:         t.SourcePort,
        SourceChannel:      t.SourceChannel,
        DestinationPort:    t.DestinationPort,
        DestinationChannel: t.DestinationChannel,
        Amount:             t.Amount,
        Denom:              t.Denom,
        TimeoutHeight:      t.TimeoutHeight,
        TimeoutTimestamp:   t.TimeoutTimestamp,
        PacketSequence:     t.PacketSequence,
        DataHex:            t.DataHex,
        State:              t.State,
        TxHashes:           []string{t.TxHash}, // 单条记录包装为数组
        CreatedAt:          t.CreatedAt.Format(time.RFC3339),
        UpdatedAt:          t.UpdatedAt.Format(time.RFC3339),
    }
}

// ToIBCTransferTxProtos converts slice of IBCTransferTx to proto DTOs
func ToIBCTransferTxProtos(txs []*IBCTransferTx) []*pb.IBCTransferTxDTO {
    result := make([]*pb.IBCTransferTxDTO, len(txs))
    for i, tx := range txs {
        result[i] = tx.ToProto()
    }
    return result
}
```

## 6. 分页处理

### 6.1 分页策略

统一复用现有的 `PaginationRequest` 和 `normalizePagination` 函数：

```go
// normalizePagination 复用现有的函数
func normalizePagination(req *pb.PaginationRequest, defaultLimit, maxLimit int32) (limit int, offset int) {
    if req == nil {
        return int(defaultLimit), 0
    }

    limit = int(req.Limit)
    if limit <= 0 {
        limit = int(defaultLimit)
    }
    if limit > int(maxLimit) {
        limit = int(maxLimit)
    }

    offset = int(req.Offset)
    if offset < 0 {
        offset = 0
    }

    return limit, offset
}
```

### 6.2 响应格式

使用 `SuccessReplyWithPagination` 函数构建响应，与现有接口保持一致：

```go
func SuccessReplyWithPagination(total int64, data interface{}, paginationReq *pb.PaginationRequest) *pb.Reply {
    // 构建 PaginationInfo
    pagination := &pb.PaginationInfo{
        Total: uint64(total),
        Count: uint64(len(data)),
    }

    if paginationReq != nil {
        pagination.NextCursor = paginationReq.Offset + uint64(len(data))
        pagination.HasMore = pagination.NextCursor < uint64(total)
    }

    response := map[string]interface{}{
        "total": total,
        "transactions": data,
        "pagination": pagination,
    }

    return MarshalAndReply(response)
}
```

## 7. 查询优化

### 7.1 索引利用

确保查询字段都有对应的索引（已定义在 schema 中）：

- `ibc_transfer_txs`: `sender`, `receiver`, `source_channel`, `destination_channel`, `block_timestamp`
- `peggy_deposit_txs`: `sender`, `receiver`, `block_timestamp`
- `peggy_withdrawal_txs`: `sender`, `receiver`, `block_timestamp`

**建议增加复合索引优化高频查询：**

```sql
-- 复合索引优化高频查询
ALTER TABLE `ibc_transfer_txs`
ADD INDEX `idx_ibc_sender_timestamp` (`sender`, `block_timestamp DESC`);

ALTER TABLE `peggy_deposit_txs`
ADD INDEX `idx_peggy_deposit_receiver_timestamp` (`receiver`, `block_timestamp DESC`);

ALTER TABLE `peggy_withdrawal_txs`
ADD INDEX `idx_peggy_withdrawal_sender_timestamp` (`sender`, `block_timestamp DESC`);
```

### 7.2 查询构建

使用 GORM 构建查询，利用索引：

```go
query := db.Get().Model(&models.IBCTransferTx{})

// 应用过滤条件
if req.Sender != "" {
    query = query.Where("sender = ?", req.Sender)
}
if req.Receiver != "" {
    query = query.Where("receiver = ?", req.Receiver)
}
// ... 其他过滤条件

// 排序和分页
query = query.Order("block_timestamp DESC").Limit(limit).Offset(offset)
```

### 7.3 缓存策略（可选优化）

参考 `GetLatestBlocks` 实现 Redis 缓存：

```go
const (
    cacheKeyIBCTransfersPrefix = "ibc_transfers:"
    cacheTTLCrossChain         = 30 * time.Second
)

func (s *ExplorerService) getCachedIBCTransfers(ctx context.Context, cacheKey string) ([]*pb.IBCTransferTxDTO, bool) {
    cached, err := global.Redis.Get(ctx, cacheKey).Result()
    if err != nil {
        return nil, false
    }

    var result []*pb.IBCTransferTxDTO
    if err := sonic.UnmarshalString(cached, &result); err != nil {
        return nil, false
    }
    return result, true
}
```

**缓存策略：**
- 按 sender/receiver + 分页参数生成缓存 key
- TTL 30 秒（跨链交易相对不频繁）
- 仅缓存第一页（最热数据）

## 8. 错误处理

### 8.1 参数验证错误

- 返回 HTTP 400 Bad Request
- 使用 `ErrorReply` 返回错误信息

```go
if req == nil {
    return ErrorReply(http.StatusBadRequest, "request is nil", nil), nil
}

if req.Sender != "" {
    if err := ValidateAddress(req.Sender); err != nil {
        return ErrorReply(http.StatusBadRequest, "invalid sender address", err), nil
    }
}
```

### 8.2 数据库错误

- 返回 HTTP 500 Internal Server Error
- 记录详细错误日志
- 不暴露内部错误信息给客户端

```go
if err := query.Count(&total).Error; err != nil {
    global.Logger.Errorf("count ibc transfers failed: %v", err)
    return ErrorReply(http.StatusInternalServerError, "query failed", nil), nil
}
```

## 9. 实现步骤

### 9.1 第一阶段：Proto 定义

1. 在 `api/explorer/v1/explorer.proto` 中添加三个 RPC 方法定义
2. 添加请求消息定义（使用 PaginationRequest）
3. 添加 DTO 消息定义
4. 生成 proto 代码：`make proto`

### 9.2 第二阶段：数据模型

1. 创建 GORM 模型文件
   - `internal/models/ibc_transfer_tx.go`
   - `internal/models/peggy_deposit_tx.go`
   - `internal/models/peggy_withdrawal_tx.go`
2. 在模型中实现 `ToProto()` 转换方法
3. 确保数据库表结构正确（参考 `docs/database_schema.sql`）
4. 执行数据库迁移

### 9.3 第三阶段：查询服务实现

1. 在 `internal/service/explorer.go` 中实现三个查询方法
2. 复用现有的 `normalizePagination` 函数
3. 实现动态查询条件构建
4. 集成缓存（可选）

### 9.4 第四阶段：测试

1. 单元测试：测试查询逻辑和 DTO 转换
2. 集成测试：测试完整的 API 流程
3. 性能测试：测试大数据集下的查询性能
4. 验证索引效果

## 10. API 使用示例

### 10.1 GetIBCTransferTxs

**基本查询**：
```http
GET /api/v1/ibc/transfers?sender=inj1xxx&pagination.limit=20
```

**带多个过滤条件**：
```http
GET /api/v1/ibc/transfers?sender=inj1xxx&receiver=cosmos1xxx&src_channel=channel-0&dest_channel=channel-1
```

**Response**：
```json
{
  "data": {
    "total": 100,
    "transactions": [
      {
        "sender": "inj1...",
        "receiver": "cosmos1...",
        "source_port": "transfer",
        "source_channel": "channel-0",
        "destination_port": "transfer",
        "destination_channel": "channel-1",
        "amount": "1000000",
        "denom": "inj",
        "timeout_height": "1-1234567",
        "timeout_timestamp": 1699999999000000000,
        "packet_sequence": 42,
        "state": "completed",
        "tx_hashes": ["0xabc..."],
        "created_at": "2024-01-30T10:00:00Z",
        "updated_at": "2024-01-30T10:00:00Z"
      }
    ],
    "pagination": {
      "next_cursor": 20,
      "has_more": true,
      "total": 100,
      "count": 20
    }
  }
}
```

### 10.2 GetPeggyDepositTxs

```http
GET /api/v1/peggy/deposits?sender=0xabc...&receiver=inj1xxx&pagination.limit=20
```

### 10.3 GetPeggyWithdrawalTxs

```http
GET /api/v1/peggy/withdrawals?sender=inj1xxx&receiver=0xabc...&pagination.limit=20
```

## 11. 注意事项

### 11.1 字段命名

- Proto 字段使用 `snake_case` 并添加 `json_name` 标签
- 与 Injective Explorer 接口保持一致

### 11.2 TxHashes 字段

- 数据库表中只有单个 `tx_hash` 字段
- DTO 中转换为 `tx_hashes` 数组以兼容 Injective Explorer 接口
- 使用单元素数组包装

### 11.3 DataHex 字段

- IBCTransferTx 中的 `DataHex` 字段在数据库中以 `[]byte` 存储
- 在 proto 中保持 `bytes` 类型，客户端可自行解码

### 11.4 时间戳格式

- `CreatedAt` 和 `UpdatedAt` 使用 ISO 8601 格式字符串（RFC3339）
- `TimeoutTimestamp` 使用纳秒级 uint64

## 12. 后续优化

### 12.1 缓存策略

- 对于热门查询（如最新交易），添加 Redis 缓存
- 参考 `GetLatestBlocks` 的缓存实现

### 12.2 批量查询

- 如需支持批量查询多个地址的交易，可添加批量接口
- 参考 `GetTransactionsBatch` 的实现

### 12.3 实时更新

- 如需实时推送跨链交易更新，可添加 WebSocket 流式接口
- 参考 `StreamLatestTransactions` 的实现

---

**文档版本**: 2.0
**更新日期**: 2026-01-31
**作者**: Injective Explorer Team
