# 需要实现的接口完整方案

## 一、当前实现状态总结

### ✅ 已完全实现的核心接口 (18个)

1. **区块查询**
   - GetLatestBlocks (对应SDK GetBlocks)
   - GetBlockByHeight (对应SDK GetBlock)
   - GetLatestBlockHeight
   - GetBlockTransactions
   - GetBlocksBatch
   - GetBlockGasUtilization

2. **交易查询**
   - GetLatestTransactions (对应SDK GetTxs)
   - GetTransactionByHash (对应SDK GetTxByTxHash)
   - GetAccountTransactions (对应SDK GetAccountTxs)
   - GetTransactionsBatch
   - GetTransactionStats
   - GetFailedTransactions24H
   - GetFailedTransactions1000

3. **验证者查询** (Phase 2)
   - GetValidators (对应SDK FetchValidators)
   - GetValidator (对应SDK FetchValidator)
   - GetValidatorUptime (对应SDK FetchValidatorUptime)
   - GetValidatorBlockStats

4. **Wasm合约查询** (Phase 1 + 3)
   - GetWasmContracts (对应SDK GetWasmContracts)
   - GetWasmContractByAddress (对应SDK GetWasmContractByAddress)
   - GetCW20Balance (对应SDK GetCW20Balance)
   - GetWasmCodes (对应SDK GetWasmCodes) - Phase 3
   - GetWasmCodeByID (对应SDK GetWasmCodeByID) - Phase 3
   - GetContractTxs (对应SDK FetchContractTxs) - Phase 3

5. **跨链交易**
   - GetIBCTransferTxs (对应SDK GetIBCTransfers)
   - GetPeggyDepositTxs (对应SDK GetPeggyDeposits)
   - GetPeggyWithdrawalTxs (对应SDK GetPeggyWithdrawals)

6. **账户查询**
   - GetAccountInfo
   - GetAccountBalances

7. **EVM查询**
   - GetEVMTransaction
   - GetEVMTransactions
   - GetEVMAccountTransactions

8. **其他功能**
   - GetTokenPriceAndMarketCap
   - GetAllContractTags
   - AddGlobalTag
   - RemoveGlobalTag
   - CheckHealthy

9. **流式订阅** (已实现)
   - StreamLatestBlocks (对应SDK StreamBlocks)
   - StreamLatestTransactions (对应SDK StreamTxs)
   - StreamAccountUpdates

### ❌ SDK文档中提到但项目未实现的接口 (2个)

1. **FetchRelayers** - 获取中继者市场信息
2. **FetchBankTransfers** - 获取银行转账记录

---

## 二、需要实现的接口详细方案

### 接口1: GetBankTransfers - 获取银行转账记录

**优先级**: 🔴 **高** (基础功能，用于查询原生代币转账)

#### 1.1 接口设计

**Endpoint**: `GET /api/v1/bank/transfers`

**功能**: 查询链上原生代币(如UINJ)的转账记录，支持发送者/接收者过滤和时间范围查询。

**请求参数**:
```protobuf
message GetBankTransfersRequest {
  // Pagination
  PaginationRequest pagination = 1;

  // Filter by sender addresses (optional, multiple)
  repeated string senders = 2;

  // Filter by recipient addresses (optional, multiple)
  repeated string recipients = 3;

  // Filter by address (sender or recipient, optional, multiple)
  repeated string addresses = 4;

  // Time range filter
  int64 start_time = 5 [json_name = "start_time"]; // Unix milliseconds
  int64 end_time = 6 [json_name = "end_time"];     // Unix milliseconds

  // Filter by denom (optional)
  string denom = 7;

  // Only show community pool related transfers
  bool is_community_pool_related = 8 [json_name = "is_community_pool_related"];
}
```

**响应结构**:
```json
{
  "transfers": [
    {
      "tx_hash": "abc123...",
      "sender": "inj1sender...",
      "recipient": "inj1recipient...",
      "amounts": [
        {
          "denom": "uinj",
          "amount": "1000000"
        }
      ],
      "block_height": 1000000,
      "unix_timestamp": 1704067200000,
      "success": true,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 150,
    "total_pages": 8
  }
}
```

#### 1.2 数据库设计

**表名**: `bank_transfers`

```sql
CREATE TABLE bank_transfers (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,

  -- Transfer details
  tx_hash VARCHAR(128) NOT NULL COMMENT '交易哈希',
  sender VARCHAR(128) NOT NULL COMMENT '发送者地址',
  recipient VARCHAR(128) NOT NULL COMMENT '接收者地址',
  amounts TEXT NOT NULL COMMENT 'JSON数组: [{denom, amount}]',

  -- Block info
  block_height BIGINT NOT NULL COMMENT '区块高度',
  unix_timestamp BIGINT NOT NULL COMMENT '时间戳(毫秒)',

  -- Status
  success BOOLEAN NOT NULL DEFAULT TRUE COMMENT '是否成功',
  is_community_pool_related BOOLEAN DEFAULT FALSE COMMENT '是否与社区池相关',

  -- Metadata
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

  -- Indexes
  INDEX idx_bank_transfers_sender (sender),
  INDEX idx_bank_transfers_recipient (recipient),
  INDEX idx_bank_transfers_tx_hash (tx_hash),
  INDEX idx_bank_transfers_timestamp (unix_timestamp),
  INDEX idx_bank_transfers_block_height (block_height),
  INDEX idx_bank_transfers_sender_time (sender, unix_timestamp DESC),
  INDEX idx_bank_transfers_recipient_time (recipient, unix_timestamp DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='银行转账记录表';
```

#### 1.3 Model定义

**文件**: `internal/models/bank_transfer.go`

```go
package models

import "time"

// BankTransfer represents a bank transfer record in the database.
type BankTransfer struct {
	ID uint64 `json:"id" gorm:"column:id;primaryKey"`

	// Transfer details
	TxHash    string `json:"tx_hash" gorm:"column:tx_hash;size:128;not null;index:idx_bank_transfers_tx_hash"`
	Sender    string `json:"sender" gorm:"column:sender;size:128;not null;index:idx_bank_transfers_sender;index:idx_bank_transfers_sender_time,priority:1"`
	Recipient string `json:"recipient" gorm:"column:recipient;size:128;not null;index:idx_bank_transfers_recipient;index:idx_bank_transfers_recipient_time,priority:1"`
	Amounts   string `json:"amounts" gorm:"column:amounts;type:text;not null"` // JSON: [{"denom":"uinj","amount":"1000"}]

	// Block info
	BlockHeight   int64 `json:"block_height" gorm:"column:block_height;not null;index:idx_bank_transfers_block_height"`
	UnixTimestamp int64 `json:"unix_timestamp" gorm:"column:unix_timestamp;not null;index:idx_bank_transfers_timestamp;index:idx_bank_transfers_sender_time,priority:2,sort:desc;index:idx_bank_transfers_recipient_time,priority:2,sort:desc"`

	// Status
	Success                  bool       `json:"success" gorm:"column:success;not null;default:true"`
	IsCommunityPoolRelated   bool       `json:"is_community_pool_related" gorm:"column:is_community_pool_related;default:false"`
	CreatedAt                *time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName returns the table name for BankTransfer model.
func (BankTransfer) TableName() string {
	return "bank_transfers"
}
```

#### 1.4 Service实现

**文件**: `internal/service/bank_api.go`

```go
package service

import (
	"context"
	"fmt"
	"net/http"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// convertBankTransferToDTO converts BankTransfer model to DTO
func convertBankTransferToDTO(transfer *models.BankTransfer) map[string]interface{} {
	dto := map[string]interface{}{
		"tx_hash":        transfer.TxHash,
		"sender":         transfer.Sender,
		"recipient":      transfer.Recipient,
		"block_height":   float64(transfer.BlockHeight),
		"unix_timestamp": float64(transfer.UnixTimestamp),
		"success":        transfer.Success,
	}

	// Parse amounts from JSON string
	var amounts []interface{}
	if err := sonic.UnmarshalString(transfer.Amounts, &amounts); err == nil {
		dto["amounts"] = amounts
	} else {
		dto["amounts"] = transfer.Amounts
	}

	if transfer.IsCommunityPoolRelated {
		dto["is_community_pool_related"] = true
	}

	if transfer.CreatedAt != nil {
		dto["created_at"] = transfer.CreatedAt.Format(time.RFC3339)
	}

	return dto
}

// GetBankTransfers retrieves bank transfer records with filtering.
func (s *ExplorerService) GetBankTransfers(ctx context.Context, req *pb.GetBankTransfersRequest) (*pb.Reply, error) {
	var transfers []models.BankTransfer

	query := db.Get().Model(&models.BankTransfer{})

	// Apply filters
	if len(req.Senders) > 0 {
		query = query.Where("sender IN ?", req.Senders)
	}
	if len(req.Recipients) > 0 {
		query = query.Where("recipient IN ?", req.Recipients)
	}
	if len(req.Addresses) > 0 {
		query = query.Where("sender IN ? OR recipient IN ?", req.Addresses, req.Addresses)
	}
	if req.StartTime > 0 {
		query = query.Where("unix_timestamp >= ?", req.StartTime)
	}
	if req.EndTime > 0 {
		query = query.Where("unix_timestamp <= ?", req.EndTime)
	}
	if req.Denom != "" {
		// Search in JSON field using LIKE (MySQL JSON functions would be better)
		query = query.Where("amounts LIKE ?", "%\"denom\":\""+req.Denom+"\"%")
	}
	if req.IsCommunityPoolRelated {
		query = query.Where("is_community_pool_related = ?", true)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count bank transfers")
	}

	// Pagination
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	query = query.Offset(int(offset)).Limit(int(pageSize))

	// Order by timestamp DESC (newest first)
	query = query.Order("unix_timestamp DESC")

	// Execute query
	if err := query.Find(&transfers).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch bank transfers")
	}

	// Convert to DTOs
	transferDTOs := make([]interface{}, len(transfers))
	for i, transfer := range transfers {
		transferDTOs[i] = convertBankTransferToDTO(&transfer)
	}

	// Build response
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

	dataStruct, err := structpb.NewStruct(map[string]interface{}{
		"transfers": transferDTOs,
		"pagination": map[string]interface{}{
			"page":        float64(page),
			"page_size":   float64(pageSize),
			"total":       float64(total),
			"total_pages": float64(totalPages),
			"has_prev":    page > 1,
			"has_next":    page < totalPages,
		},
	})
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create response struct: %v", err))
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    dataStruct,
	}, nil
}
```

#### 1.5 单元测试

**文件**: `internal/service/bank_api_test.go`

测试用例:
- 获取所有转账记录 (带分页)
- 按发送者过滤
- 按接收者过滤
- 按地址过滤 (发送者或接收者)
- 按时间范围过滤
- 按denom过滤
- 社区池相关转账过滤
- 空结果集
- 分页限制

目标: 9个测试用例，覆盖率>90%

---

### 接口2: GetRelayers - 获取中继者信息

**优先级**: 🟡 **中** (特定场景功能，如果项目使用中继者则需要)

#### 2.1 接口设计

**Endpoint**: `GET /api/v1/relayers`

**功能**: 获取跨链中继者的市场信息和统计数据。

**请求参数**:
```protobuf
message GetRelayersRequest {
  // Filter by market IDs (optional)
  repeated string market_ids = 1 [json_name = "market_ids"];

  // Pagination
  PaginationRequest pagination = 2;
}
```

**响应结构**:
```json
{
  "relayers": [
    {
      "market_id": "0x...",
      "relayer_address": "inj1relayer...",
      "total_packets_relayed": 1000,
      "success_rate": 99.5,
      "avg_relay_time_ms": 2500,
      "total_fees_earned": "500000uinj",
      "active_channels": ["channel-0", "channel-1"],
      "last_active_at": 1704067200000,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 50,
    "total_pages": 3
  }
}
```

#### 2.2 数据库设计

**表名**: `relayers`

```sql
CREATE TABLE relayers (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,

  -- Relayer info
  market_id VARCHAR(128) NOT NULL COMMENT '市场ID',
  relayer_address VARCHAR(128) NOT NULL COMMENT '中继者地址',

  -- Statistics
  total_packets_relayed BIGINT DEFAULT 0 COMMENT '总中继包数',
  total_packets_failed BIGINT DEFAULT 0 COMMENT '失败包数',
  success_rate DECIMAL(5,2) DEFAULT 0.00 COMMENT '成功率(%)',
  avg_relay_time_ms BIGINT DEFAULT 0 COMMENT '平均中继时间(毫秒)',
  total_fees_earned TEXT COMMENT '总手续费收入(JSON)',

  -- Channels
  active_channels TEXT COMMENT '活跃通道列表(JSON数组)',

  -- Timestamps
  last_active_at BIGINT COMMENT '最后活跃时间(毫秒)',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  -- Indexes
  UNIQUE KEY uk_market_relayer (market_id, relayer_address),
  INDEX idx_relayers_market_id (market_id),
  INDEX idx_relayers_relayer_address (relayer_address),
  INDEX idx_relayers_last_active (last_active_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='中继者信息表';
```

#### 2.3 说明

由于中继者功能依赖于:
1. 是否启用了跨链中继功能
2. 是否有中继者数据采集机制
3. 具体的中继者协议实现

**建议**:
- 如果项目确实需要中继者功能，先确认数据来源
- 可能需要从IBC模块或自定义中继服务采集数据
- 如果不需要，可以暂缓实现

---

## 三、实现优先级建议

### Phase 4 - 银行转账接口 (推荐立即实现)

**预计时间**: 2-3天

**实现内容**:
1. ✅ 数据库表设计和创建 (0.5天)
2. ✅ Model定义 (0.5天)
3. ✅ Service实现 (1天)
   - GetBankTransfers方法
   - DTO转换函数
4. ✅ Proto定义和生成 (0.5天)
5. ✅ 单元测试 (1天)
   - 9个测试用例
   - >90%覆盖率
6. ✅ Postman集合更新 (0.5天)

**价值**:
- 🔴 高优先级基础功能
- 查询原生代币转账是常见需求
- 完善账户相关查询能力

### Phase 5 - 中继者接口 (可选)

**预计时间**: 2-3天

**前置条件**:
- 确认是否需要中继者功能
- 确认数据来源

**建议**:
- 先评估业务需求
- 如不需要可暂缓

---

## 四、Proto定义补充

**文件**: `api/explorer/v1/bank.proto` (新建)

```protobuf
syntax = "proto3";

package api.explorer.v1;

import "explorer/v1/common.proto";

option go_package = "injective-explorer/api/explorer/v1;v1";

// Bank transfer query messages

message GetBankTransfersRequest {
  // Pagination
  PaginationRequest pagination = 1;

  // Filter by sender addresses (optional, multiple)
  repeated string senders = 2;

  // Filter by recipient addresses (optional, multiple)
  repeated string recipients = 3;

  // Filter by address (sender or recipient, optional, multiple)
  repeated string addresses = 4;

  // Time range filter
  int64 start_time = 5 [json_name = "start_time"]; // Unix milliseconds
  int64 end_time = 6 [json_name = "end_time"];     // Unix milliseconds

  // Filter by denom (optional)
  string denom = 7;

  // Only show community pool related transfers
  bool is_community_pool_related = 8 [json_name = "is_community_pool_related"];
}
```

**文件**: `api/explorer/v1/explorer.proto` (修改)

```protobuf
// Add to Explorer service:

// Get bank transfer records
rpc GetBankTransfers(GetBankTransfersRequest) returns (Reply) {
  option (google.api.http) = {
    get: "/api/v1/bank/transfers"
  };
}
```

---

## 五、总结

### 当前项目完成度

- ✅ **核心功能**: 100% (区块、交易、账户查询)
- ✅ **验证者**: 100% (Phase 2完成)
- ✅ **Wasm合约**: 100% (Phase 1+3完成)
- ✅ **跨链交易**: 100% (IBC, Peggy)
- ✅ **流式订阅**: 100% (Stream APIs)
- ⚠️ **银行转账**: 0% (待实现 - 推荐Phase 4)
- ⚠️ **中继者**: 0% (待评估需求)

### 推荐实施路线

1. **立即实现** (Phase 4):
   - GetBankTransfers
   - 完善基础查询能力

2. **评估后决定**:
   - GetRelayers (取决于业务需求)

3. **最终状态**:
   - SDK文档覆盖率: 95%+ (除非确定不需要Relayers)
   - 完整的Explorer API能力

---

**文档版本**: 1.0
**创建日期**: 2026-02-09
**下一步**: Phase 4 - 银行转账接口实现
