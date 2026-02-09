# Injective Explorer API 实现状态与技术方案

本文档分析当前 Injective Explorer 项目的接口实现状态，与 SDK-Go Explorer 接口进行对比，并为缺失的接口提供完整的技术实现方案。

**文档版本**: 1.1 (已根据项目现有结构调整)
**最后更新**: 2026-02-08
**项目路径**: `/Users/mac/vscode/mengbin/biya/biya-explorer`

---

## 目录

- [实现状态总览](#实现状态总览)
- [已实现接口详情](#已实现接口详情)
- [缺失接口分析](#缺失接口分析)
- [技术实现方案](#技术实现方案)
  - [1. 验证者相关接口](#1-验证者相关接口)
  - [2. Wasm合约相关接口](#2-wasm合约相关接口)
  - [3. 中继者接口](#3-中继者接口)
  - [4. 银行转账接口](#4-银行转账接口)
- [数据库设计方案](#数据库设计方案)
- [Proto定义方案](#proto定义方案)
- [实施优先级建议](#实施优先级建议)
- [技术风险与挑战](#技术风险与挑战)

---

## 实现状态总览

### 对比 SDK-Go Explorer 的 22 个核心接口

| 类别 | SDK-Go 接口 | 当前项目接口 | 状态 | 优先级 |
|------|------------|------------|------|--------|
| **区块查询** | GetBlocks | GetLatestBlocks | ✅ 已实现 | - |
| | GetBlock | GetBlockByHeight | ✅ 已实现 | - |
| | StreamBlocks | StreamLatestBlocks | ✅ 已实现 | - |
| **交易查询** | GetTxs | GetLatestTransactions | ✅ 已实现 | - |
| | GetTxByTxHash | GetTransactionByHash | ✅ 已实现 | - |
| | StreamTxs | StreamLatestTransactions | ✅ 已实现 | - |
| | GetAccountTxs | GetAccountTransactions | ✅ 已实现 | - |
| **账户查询** | - | GetAccountInfo | ✅ 已实现(扩展) | - |
| | - | GetAccountBalances | ✅ 已实现(扩展) | - |
| **跨链交易** | GetIBCTransfers | GetIBCTransferTxs | ✅ 已实现 | - |
| | GetPeggyDeposits | GetPeggyDepositTxs | ✅ 已实现 | - |
| | GetPeggyWithdrawals | GetPeggyWithdrawalTxs | ✅ 已实现 | - |
| **验证者** | FetchValidators | - | ❌ 缺失 | P1 |
| | FetchValidator | - | ❌ 缺失 | P1 |
| | FetchValidatorUptime | - | ❌ 缺失 | P2 |
| **Wasm合约** | GetWasmCodes | - | ❌ 缺失 | P1 |
| | GetWasmCodeByID | - | ❌ 缺失 | P1 |
| | GetWasmContracts | - | ❌ 缺失 | P0 |
| | GetWasmContractByAddress | - | ❌ 缺失 | P0 |
| | GetCW20Balance | - | ❌ 缺失 | P0 |
| | FetchContractTxs | - | ❌ 缺失 | P1 |
| | FetchContractTxsV2 | - | ❌ 缺失 | P1 |
| **其他** | FetchRelayers | - | ❌ 缺失 | P3 |
| | FetchBankTransfers | - | ❌ 缺失 | P2 |

### 统计
- **已实现**: 12/22 (54.5%)
- **缺失**: 10/22 (45.5%)
- **P0 (关键)**: 3个
- **P1 (重要)**: 5个
- **P2 (一般)**: 2个
- **P3 (可选)**: 1个

---

## 已实现接口详情

### 核心功能

#### 1. 区块查询 (3/3 完成)
- ✅ `GetLatestBlocks` - 获取最新区块列表，支持分页
- ✅ `GetBlockByHeight` - 根据高度获取单个区块详情
- ✅ `StreamLatestBlocks` - 实时区块流订阅

#### 2. 交易查询 (4/4 完成)
- ✅ `GetLatestTransactions` - 获取最新交易列表
- ✅ `GetTransactionByHash` - 根据哈希获取交易详情
- ✅ `GetBlockTransactions` - 获取指定区块的所有交易
- ✅ `StreamLatestTransactions` - 实时交易流订阅

#### 3. 账户查询 (3/3 完成)
- ✅ `GetAccountInfo` - 获取账户基本信息
- ✅ `GetAccountTransactions` - 获取账户交易记录
- ✅ `GetAccountBalances` - 获取账户余额(包括原生代币和ERC20)

#### 4. 跨链交易 (3/3 完成)
- ✅ `GetIBCTransferTxs` - 获取IBC跨链转账记录
- ✅ `GetPeggyDepositTxs` - 获取Peggy充值记录
- ✅ `GetPeggyWithdrawalTxs` - 获取Peggy提现记录

### 扩展功能(超出SDK-Go范围)

#### 1. EVM支持
- ✅ `GetEVMTransaction` - 获取EVM交易详情
- ✅ `GetEVMTransactions` - 获取EVM交易列表
- ✅ `GetEVMAccountTransactions` - 获取账户EVM交易

#### 2. 批量查询
- ✅ `GetTransactionsBatch` - 批量获取交易
- ✅ `GetBlocksBatch` - 批量获取区块

#### 3. 统计分析
- ✅ `GetTransactionStats` - 获取交易统计数据
- ✅ `GetBlockGasUtilization` - 获取区块Gas使用率
- ✅ `GetFailedTransactions24h` - 获取24小时失败交易
- ✅ `GetFailedTransactions1000` - 获取最近1000条失败交易
- ✅ `GetValidatorBlockStats` - 获取验证者出块统计

#### 4. 标签和元数据
- ✅ `GetAllContractTags` - 获取所有合约标签
- ✅ `AddGlobalTag` - 添加全局标签
- ✅ `RemoveGlobalTag` - 移除全局标签

#### 5. 其他
- ✅ `GetTokenPriceAndMarketCap` - 获取代币价格和市值
- ✅ `StreamAccountUpdates` - 账户更新流订阅
- ✅ `CheckHealthy` - 健康检查

---

## 缺失接口分析

### P0 - 关键优先级(必须实现)

#### 1. GetWasmContracts
**功能**: 获取已部署的Wasm智能合约列表
**重要性**: 智能合约是生态的核心，用户需要浏览和发现合约
**使用场景**: 合约列表页、合约搜索、合约分析

#### 2. GetWasmContractByAddress
**功能**: 根据地址获取合约详情
**重要性**: 查看合约详细信息，包括代码、执行记录、资金等
**使用场景**: 合约详情页、合约验证、审计

#### 3. GetCW20Balance
**功能**: 获取CW20代币余额
**重要性**: CW20是CosmWasm的标准代币协议，类似ERC20
**使用场景**: 钱包余额显示、代币持有者查询

### P1 - 重要优先级(建议实现)

#### 4. FetchValidators
**功能**: 获取所有验证者信息
**重要性**: PoS链的核心组件，用户需要了解验证者状态
**使用场景**: 验证者列表页、质押决策

#### 5. FetchValidator
**功能**: 获取单个验证者详情
**重要性**: 查看验证者详细信息、委托记录等
**使用场景**: 验证者详情页、质押管理

#### 6. GetWasmCodes
**功能**: 获取已上传的Wasm代码列表
**重要性**: 了解链上可用的合约代码模板
**使用场景**: 代码库浏览、合约部署

#### 7. GetWasmCodeByID
**功能**: 根据ID获取Wasm代码详情
**重要性**: 查看代码详情、验证合约代码
**使用场景**: 代码验证、安全审计

#### 8. FetchContractTxs / FetchContractTxsV2
**功能**: 获取合约交易记录
**重要性**: 追踪合约活动、分析合约使用情况
**使用场景**: 合约活动监控、交易历史

### P2 - 一般优先级(可以实现)

#### 9. FetchValidatorUptime
**功能**: 获取验证者在线率统计
**重要性**: 评估验证者可靠性
**使用场景**: 质押决策、验证者评级

#### 10. FetchBankTransfers
**功能**: 获取银行转账记录
**重要性**: 追踪原生代币转账
**使用场景**: 转账历史、资金流向分析

### P3 - 可选优先级(低优先级)

#### 11. FetchRelayers
**功能**: 获取中继者信息
**重要性**: 主要用于DEX等特定场景
**使用场景**: 交易中继、市场数据

---

## 技术实现方案

### 1. 验证者相关接口

#### 1.1 复用现有数据库表

项目中已经存在完整的验证者相关表结构：

**已有表结构：**
- `validators` - 验证者基本信息 (models/validators.go)
- `validator_block_stats` - 验证者出块统计 (models/validator_block_stats.go)
- `validator_set_snapshots` - 验证者集合快照 (models/validator_set_snapshot.go)
- `block_signers` - 区块签名记录 (models/block_signer.go)

**需要补充的字段：**

```sql
-- validators表已经包含所需字段，无需修改
-- 现有字段完全满足需求：
-- - operator_address, consensus_address
-- - moniker, description (JSON格式)
-- - tokens, delegator_shares
-- - jailed, status
-- - commission相关字段
-- - proposed, signed, missed
-- - uptime_percentage

-- 可选：添加索引优化查询性能
CREATE INDEX IF NOT EXISTS idx_validators_status_tokens
ON validators(status, CAST(tokens AS DECIMAL));

CREATE INDEX IF NOT EXISTS idx_validators_uptime_percentage
ON validators(uptime_percentage DESC);
```

#### 1.2 服务实现

**文件**: `internal/service/validator_service.go` (新建)

```go
package service

import (
    "context"
    "net/http"

    pb "injective-explorer/api/explorer/v1"
    "injective-explorer/internal/models"
    "injective-explorer/provider/db"

    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    "gorm.io/gorm"
)

// GetValidators 获取所有验证者列表
func (s *ExplorerService) GetValidators(ctx context.Context, req *pb.GetValidatorsRequest) (*pb.Reply, error) {
    var validators []models.Validator

    query := db.Get().Model(&models.Validator{})

    // 按状态过滤
    if req.Status != "" {
        // Status: 1=UNBONDED, 2=UNBONDING, 3=BONDED
        var statusCode int32
        switch req.Status {
        case "bonded":
            statusCode = 3
        case "unbonding":
            statusCode = 2
        case "unbonded":
            statusCode = 1
        }
        if statusCode > 0 {
            query = query.Where("status = ?", statusCode)
        }
    }

    // 按是否监禁过滤
    if req.Jailed != nil {
        query = query.Where("jailed = ?", *req.Jailed)
    }

    // 获取总数
    var total int64
    if err := query.Count(&total).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to count validators")
    }

    // 分页参数
    page := req.Pagination.Page
    if page < 1 {
        page = 1
    }
    pageSize := req.Pagination.PageSize
    if pageSize <= 0 {
        pageSize = 20
    }
    if pageSize > 100 {
        pageSize = 100
    }

    offset := (page - 1) * pageSize
    query = query.Offset(int(offset)).Limit(int(pageSize))

    // 排序：默认按投票权(tokens)降序
    if req.OrderBy != "" {
        query = query.Order(req.OrderBy)
    } else {
        query = query.Order("CAST(tokens AS DECIMAL) DESC")
    }

    if err := query.Find(&validators).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to fetch validators")
    }

    // 构建响应
    totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data: map[string]interface{}{
            "validators": convertValidatorsToDTO(validators),
            "pagination": &pb.PaginationInfo{
                Page:       page,
                PageSize:   pageSize,
                Total:      total,
                TotalPages: totalPages,
                HasPrev:    page > 1,
                HasNext:    page < totalPages,
            },
        },
    }, nil
}

// GetValidator 获取单个验证者详情
func (s *ExplorerService) GetValidator(ctx context.Context, req *pb.GetValidatorRequest) (*pb.Reply, error) {
    var validator models.Validator

    if err := db.Get().Where("operator_address = ?", req.Address).First(&validator).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, status.Error(codes.NotFound, "validator not found")
        }
        return nil, status.Error(codes.Internal, "failed to fetch validator")
    }

    // 获取聚合统计
    proposed, signed, missed, err := models.GetValidatorStatsAggregated(req.Address)
    if err != nil {
        s.logger.Warnf("Failed to get validator stats: %v", err)
    }

    validatorDTO := convertValidatorToDTO(&validator)
    validatorDTO.ProposedBlocks = proposed
    validatorDTO.SignedBlocks = signed
    validatorDTO.MissedBlocks = missed

    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data:    validatorDTO,
    }, nil
}

// GetValidatorUptime 获取验证者在线率
func (s *ExplorerService) GetValidatorUptime(ctx context.Context, req *pb.GetValidatorUptimeRequest) (*pb.Reply, error) {
    // 默认查询最近10000个区块
    blocksToCheck := int64(10000)
    if req.Blocks > 0 {
        blocksToCheck = req.Blocks
    }

    // 获取当前高度
    var latestBlock models.Block
    if err := db.Get().Order("height DESC").First(&latestBlock).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to get latest block")
    }

    currentHeight := latestBlock.Height
    startHeight := currentHeight - blocksToCheck
    if startHeight < 1 {
        startHeight = 1
    }

    // 从block_signers表查询签名记录
    var signedCount int64
    err := db.Get().Model(&models.BlockSigner{}).
        Where("validator_address = ? AND block_height >= ? AND block_height <= ?",
            req.Address, startHeight, currentHeight).
        Where("signed = ?", true).
        Count(&signedCount).Error

    if err != nil {
        return nil, status.Error(codes.Internal, "failed to count signed blocks")
    }

    totalBlocks := currentHeight - startHeight + 1
    uptime := float64(signedCount) / float64(totalBlocks) * 100

    // 按天聚合
    type DailyUptime struct {
        Date         string  `json:"date"`
        SignedBlocks int64   `json:"signed_blocks"`
        TotalBlocks  int64   `json:"total_blocks"`
        Uptime       float64 `json:"uptime"`
    }

    var dailyUptimes []DailyUptime
    err = db.Get().Raw(`
        SELECT
            DATE(FROM_UNIXTIME(b.unix_timestamp/1000)) as date,
            SUM(CASE WHEN bs.signed THEN 1 ELSE 0 END) as signed_blocks,
            COUNT(*) as total_blocks,
            (SUM(CASE WHEN bs.signed THEN 1 ELSE 0 END) * 100.0 / COUNT(*)) as uptime
        FROM blocks b
        LEFT JOIN block_signers bs ON b.height = bs.block_height AND bs.validator_address = ?
        WHERE b.height >= ? AND b.height <= ?
        GROUP BY DATE(FROM_UNIXTIME(b.unix_timestamp/1000))
        ORDER BY date DESC
        LIMIT 30
    `, req.Address, startHeight, currentHeight).Scan(&dailyUptimes).Error

    if err != nil {
        s.logger.Warnf("Failed to get daily uptime stats: %v", err)
    }

    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data: map[string]interface{}{
            "address":       req.Address,
            "uptime":        uptime,
            "signed_blocks": signedCount,
            "total_blocks":  totalBlocks,
            "start_height":  startHeight,
            "end_height":    currentHeight,
            "daily_stats":   dailyUptimes,
        },
    }, nil
}

// 辅助方法：转换验证者为DTO
func convertValidatorToDTO(v *models.Validator) map[string]interface{} {
    return map[string]interface{}{
        "id":                        v.Id,
        "moniker":                   v.Moniker,
        "operator_address":          v.OperatorAddress,
        "consensus_address":         v.ConsensusAddress,
        "jailed":                    v.Jailed,
        "status":                    v.Status,
        "tokens":                    v.Tokens,
        "delegator_shares":          v.DelegatorShares,
        "description":               v.Description,
        "commission_rate":           v.CommissionRate,
        "commission_max_rate":       v.CommissionMaxRate,
        "commission_max_change_rate": v.CommissionMaxChangeRate,
        "uptime_percentage":         v.UptimePercentage,
        "image_url":                 v.ImageUrl,
        "registration_time":         v.RegistrationTime,
    }
}
```

---

### 2. Wasm合约相关接口

#### 2.1 扩展现有数据库表

项目中已有 `contract_infos` 表，我们在此基础上扩展：

```sql
-- 扩展contract_infos表，添加Wasm相关字段
ALTER TABLE contract_infos
ADD COLUMN IF NOT EXISTS code_id BIGINT COMMENT 'Wasm代码ID',
ADD COLUMN IF NOT EXISTS label VARCHAR(255) COMMENT '合约标签',
ADD COLUMN IF NOT EXISTS init_msg TEXT COMMENT '初始化消息(JSON)',
ADD COLUMN IF NOT EXISTS admin VARCHAR(128) COMMENT '管理员地址',
ADD COLUMN IF NOT EXISTS execute_count BIGINT DEFAULT 0 COMMENT '执行次数',
ADD COLUMN IF NOT EXISTS last_executed_at TIMESTAMP NULL COMMENT '最后执行时间',
ADD COLUMN IF NOT EXISTS instantiated_at TIMESTAMP NULL COMMENT '实例化时间',
ADD COLUMN IF NOT EXISTS block_height BIGINT COMMENT '创建区块高度',
ADD COLUMN IF NOT EXISTS tx_hash VARCHAR(128) COMMENT '创建交易哈希',
ADD COLUMN IF NOT EXISTS total_supply VARCHAR(255) COMMENT 'CW20总供应量';

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_contract_infos_code_id ON contract_infos(code_id);
CREATE INDEX IF NOT EXISTS idx_contract_infos_block_height ON contract_infos(block_height);
CREATE INDEX IF NOT EXISTS idx_contract_infos_label ON contract_infos(label);

-- 新建wasm_codes表
CREATE TABLE IF NOT EXISTS wasm_codes (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    code_id BIGINT UNIQUE NOT NULL COMMENT '代码ID',
    creator VARCHAR(128) NOT NULL COMMENT '创建者地址',
    data_hash VARCHAR(128) NOT NULL COMMENT '代码哈希',
    instantiate_permission TEXT COMMENT '实例化权限(JSON)',
    tx_hash VARCHAR(128) COMMENT '上传交易哈希',
    block_height BIGINT NOT NULL COMMENT '上传区块高度',

    -- 扩展字段
    contract_type VARCHAR(100) COMMENT '合约类型',
    version VARCHAR(50) COMMENT '版本',
    source_url TEXT COMMENT '源代码URL',
    verified BOOLEAN DEFAULT FALSE COMMENT '是否验证',
    instantiate_count INT DEFAULT 0 COMMENT '实例化次数',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,

    INDEX idx_code_id (code_id),
    INDEX idx_creator (creator),
    INDEX idx_block_height (block_height),
    INDEX idx_contract_type (contract_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Wasm代码表';

-- 新建contract_executions表
CREATE TABLE IF NOT EXISTS contract_executions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    contract_address VARCHAR(128) NOT NULL COMMENT '合约地址',
    tx_hash VARCHAR(128) NOT NULL COMMENT '交易哈希',
    sender VARCHAR(128) NOT NULL COMMENT '发送者',
    msg TEXT NOT NULL COMMENT '执行消息(JSON)',
    funds TEXT COMMENT '附带资金(JSON)',
    block_height BIGINT NOT NULL COMMENT '区块高度',
    unix_timestamp BIGINT NOT NULL COMMENT '时间戳',
    success BOOLEAN NOT NULL COMMENT '是否成功',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_contract_address (contract_address),
    INDEX idx_tx_hash (tx_hash),
    INDEX idx_sender (sender),
    INDEX idx_block_height (block_height),
    INDEX idx_timestamp (unix_timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合约执行记录表';

-- 新建cw20_balances表
CREATE TABLE IF NOT EXISTS cw20_balances (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    contract_address VARCHAR(128) NOT NULL COMMENT 'CW20合约地址',
    owner_address VARCHAR(128) NOT NULL COMMENT '持有者地址',
    balance VARCHAR(255) NOT NULL COMMENT '余额',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uk_contract_owner (contract_address, owner_address),
    INDEX idx_contract_address (contract_address),
    INDEX idx_owner_address (owner_address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='CW20余额表';

-- 新建cw20_transfers表
CREATE TABLE IF NOT EXISTS cw20_transfers (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    contract_address VARCHAR(128) NOT NULL COMMENT 'CW20合约地址',
    from_address VARCHAR(128) NOT NULL COMMENT '发送者',
    to_address VARCHAR(128) NOT NULL COMMENT '接收者',
    amount VARCHAR(255) NOT NULL COMMENT '转账金额',
    tx_hash VARCHAR(128) NOT NULL COMMENT '交易哈希',
    block_height BIGINT NOT NULL COMMENT '区块高度',
    unix_timestamp BIGINT NOT NULL COMMENT '时间戳',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_contract_address (contract_address),
    INDEX idx_from_address (from_address),
    INDEX idx_to_address (to_address),
    INDEX idx_tx_hash (tx_hash),
    INDEX idx_block_height (block_height)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='CW20转账记录表';
```

#### 2.2 扩展Models

**文件**: `internal/models/wasm_code.go` (新建)

```go
package models

import (
    "gorm.io/gorm"
)

// WasmCode represents a stored Wasm code on the blockchain
type WasmCode struct {
    gorm.Model

    CodeID                int64  `json:"code_id" gorm:"column:code_id;uniqueIndex;not null"`
    Creator               string `json:"creator" gorm:"column:creator;size:128;index"`
    DataHash              string `json:"data_hash" gorm:"column:data_hash;size:128"`
    InstantiatePermission string `json:"instantiate_permission" gorm:"column:instantiate_permission;type:text"`
    TxHash                string `json:"tx_hash" gorm:"column:tx_hash;size:128"`
    BlockHeight           int64  `json:"block_height" gorm:"column:block_height;index"`

    // 扩展字段
    ContractType      string `json:"contract_type" gorm:"column:contract_type;size:100;index"`
    Version           string `json:"version" gorm:"column:version;size:50"`
    SourceURL         string `json:"source_url" gorm:"column:source_url;type:text"`
    Verified          bool   `json:"verified" gorm:"column:verified;default:false"`
    InstantiateCount  int    `json:"instantiate_count" gorm:"column:instantiate_count;default:0"`
}

func (WasmCode) TableName() string {
    return "wasm_codes"
}
```

**文件**: `internal/models/contract_execution.go` (新建)

```go
package models

import "time"

// ContractExecution represents a contract execution transaction
type ContractExecution struct {
    ID              uint64    `json:"id" gorm:"primaryKey"`
    ContractAddress string    `json:"contract_address" gorm:"column:contract_address;size:128;index;not null"`
    TxHash          string    `json:"tx_hash" gorm:"column:tx_hash;size:128;index;not null"`
    Sender          string    `json:"sender" gorm:"column:sender;size:128;index;not null"`
    Msg             string    `json:"msg" gorm:"column:msg;type:text;not null"`
    Funds           string    `json:"funds" gorm:"column:funds;type:text"`
    BlockHeight     int64     `json:"block_height" gorm:"column:block_height;index;not null"`
    UnixTimestamp   int64     `json:"unix_timestamp" gorm:"column:unix_timestamp;index;not null"`
    Success         bool      `json:"success" gorm:"column:success;not null"`
    CreatedAt       time.Time `json:"created_at" gorm:"column:created_at"`
}

func (ContractExecution) TableName() string {
    return "contract_executions"
}
```

**文件**: `internal/models/cw20_balance.go` (新建)

```go
package models

import "time"

// CW20Balance represents a CW20 token balance
type CW20Balance struct {
    ID              uint64    `json:"id" gorm:"primaryKey"`
    ContractAddress string    `json:"contract_address" gorm:"column:contract_address;size:128;uniqueIndex:uk_contract_owner;not null"`
    OwnerAddress    string    `json:"owner_address" gorm:"column:owner_address;size:128;uniqueIndex:uk_contract_owner;index;not null"`
    Balance         string    `json:"balance" gorm:"column:balance;size:255;not null"`
    UpdatedAt       time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (CW20Balance) TableName() string {
    return "cw20_balances"
}
```

#### 2.3 服务实现

**文件**: `internal/service/wasm_service.go` (新建)

```go
package service

import (
    "context"
    "encoding/json"
    "net/http"

    pb "injective-explorer/api/explorer/v1"
    "injective-explorer/internal/models"
    "injective-explorer/provider/db"

    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    "gorm.io/gorm"
)

// GetWasmContracts 获取Wasm合约列表
func (s *ExplorerService) GetWasmContracts(ctx context.Context, req *pb.GetWasmContractsRequest) (*pb.Reply, error) {
    var contracts []models.ContractInfo

    query := db.Get().Model(&models.ContractInfo{})

    // 按代码ID过滤
    if req.CodeId > 0 {
        query = query.Where("code_id = ?", req.CodeId)
    }

    // 按创建者过滤
    if req.Creator != "" {
        query = query.Where("creator = ?", req.Creator)
    }

    // 按标签过滤
    if req.Label != "" {
        query = query.Where("label LIKE ?", "%"+req.Label+"%")
    }

    // 仅CW20
    if req.Cw20Only {
        query = query.Where("contract_type = ?", "cw20")
    }

    // 获取总数
    var total int64
    if err := query.Count(&total).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to count contracts")
    }

    // 分页
    page := req.Pagination.Page
    if page < 1 {
        page = 1
    }
    pageSize := req.Pagination.PageSize
    if pageSize <= 0 {
        pageSize = 20
    }
    if pageSize > 100 {
        pageSize = 100
    }

    offset := (page - 1) * pageSize
    query = query.Offset(int(offset)).Limit(int(pageSize))
    query = query.Order("block_height DESC")

    if err := query.Find(&contracts).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to fetch contracts")
    }

    totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data: map[string]interface{}{
            "contracts": convertContractsToDTO(contracts),
            "pagination": &pb.PaginationInfo{
                Page:       page,
                PageSize:   pageSize,
                Total:      total,
                TotalPages: totalPages,
                HasPrev:    page > 1,
                HasNext:    page < totalPages,
            },
        },
    }, nil
}

// GetWasmContractByAddress 根据地址获取合约详情
func (s *ExplorerService) GetWasmContractByAddress(ctx context.Context, req *pb.GetWasmContractByAddressRequest) (*pb.Reply, error) {
    var contract models.ContractInfo

    if err := db.Get().Where("contract_address = ?", req.ContractAddress).First(&contract).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, status.Error(codes.NotFound, "contract not found")
        }
        return nil, status.Error(codes.Internal, "failed to fetch contract")
    }

    contractDTO := convertContractToDTO(&contract)

    // 获取代码信息
    if contract.CodeID != nil && *contract.CodeID > 0 {
        var code models.WasmCode
        if err := db.Get().Where("code_id = ?", *contract.CodeID).First(&code).Error; err == nil {
            contractDTO["code_info"] = convertWasmCodeToDTO(&code)
        }
    }

    // 获取最近执行记录
    var recentExecutions []models.ContractExecution
    db.Get().Where("contract_address = ?", req.ContractAddress).
        Order("unix_timestamp DESC").
        Limit(10).
        Find(&recentExecutions)

    contractDTO["recent_executions"] = convertExecutionsToDTO(recentExecutions)

    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data:    contractDTO,
    }, nil
}

// GetCW20Balance 获取CW20余额
func (s *ExplorerService) GetCW20Balance(ctx context.Context, req *pb.GetCW20BalanceRequest) (*pb.Reply, error) {
    var balances []models.CW20Balance

    query := db.Get().Model(&models.CW20Balance{}).
        Where("owner_address = ?", req.Address)

    // 按合约地址过滤
    if req.ContractAddress != "" {
        query = query.Where("contract_address = ?", req.ContractAddress)
    }

    if err := query.Find(&balances).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to fetch balances")
    }

    // 附加代币信息
    balanceDTOs := make([]map[string]interface{}, 0, len(balances))
    for _, balance := range balances {
        balanceDTO := map[string]interface{}{
            "contract_address": balance.ContractAddress,
            "owner_address":    balance.OwnerAddress,
            "balance":          balance.Balance,
            "updated_at":       balance.UpdatedAt.Unix(),
        }

        // 获取代币信息
        var contract models.ContractInfo
        if err := db.Get().Where("contract_address = ? AND contract_type = ?",
            balance.ContractAddress, "cw20").First(&contract).Error; err == nil {
            balanceDTO["token_name"] = contract.TokenName
            balanceDTO["token_symbol"] = contract.TokenSymbol
            balanceDTO["decimals"] = contract.Decimals
        }

        balanceDTOs = append(balanceDTOs, balanceDTO)
    }

    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data: map[string]interface{}{
            "balances": balanceDTOs,
            "total":    len(balanceDTOs),
        },
    }, nil
}

// GetContractTxs 获取合约交易记录
func (s *ExplorerService) GetContractTxs(ctx context.Context, req *pb.GetContractTxsRequest) (*pb.Reply, error) {
    var executions []models.ContractExecution

    query := db.Get().Model(&models.ContractExecution{}).
        Where("contract_address = ?", req.ContractAddress)

    // 按发送者过滤
    if req.Sender != "" {
        query = query.Where("sender = ?", req.Sender)
    }

    // 时间范围
    if req.StartTime > 0 {
        query = query.Where("unix_timestamp >= ?", req.StartTime)
    }
    if req.EndTime > 0 {
        query = query.Where("unix_timestamp <= ?", req.EndTime)
    }

    // 获取总数
    var total int64
    if err := query.Count(&total).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to count executions")
    }

    // 分页
    page := req.Pagination.Page
    if page < 1 {
        page = 1
    }
    pageSize := req.Pagination.PageSize
    if pageSize <= 0 {
        pageSize = 20
    }
    if pageSize > 100 {
        pageSize = 100
    }

    offset := (page - 1) * pageSize
    query = query.Offset(int(offset)).Limit(int(pageSize))
    query = query.Order("unix_timestamp DESC")

    if err := query.Find(&executions).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to fetch executions")
    }

    totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data: map[string]interface{}{
            "transactions": convertExecutionsToDTO(executions),
            "pagination": &pb.PaginationInfo{
                Page:       page,
                PageSize:   pageSize,
                Total:      total,
                TotalPages: totalPages,
                HasPrev:    page > 1,
                HasNext:    page < totalPages,
            },
        },
    }, nil
}
```

---

### 3. 中继者接口

#### 3.1 数据库设计

```sql
CREATE TABLE IF NOT EXISTS relayers (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    address VARCHAR(128) UNIQUE NOT NULL COMMENT '中继者地址',
    moniker VARCHAR(255) COMMENT '名称',
    website VARCHAR(255) COMMENT '网站',
    active BOOLEAN DEFAULT TRUE COMMENT '是否活跃',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_address (address),
    INDEX idx_active (active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='中继者表';

CREATE TABLE IF NOT EXISTS relayer_markets (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    relayer_address VARCHAR(128) NOT NULL COMMENT '中继者地址',
    market_id VARCHAR(255) NOT NULL COMMENT '市场ID',
    fee_rate VARCHAR(50) COMMENT '费率',
    active BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uk_relayer_market (relayer_address, market_id),
    INDEX idx_relayer (relayer_address),
    INDEX idx_market (market_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='中继者市场关联表';
```

#### 3.2 服务实现(简化)

由于中继者功能优先级较低(P3)，暂时提供基本框架：

```go
// GetRelayers 获取中继者信息
func (s *ExplorerService) GetRelayers(ctx context.Context, req *pb.GetRelayersRequest) (*pb.Reply, error) {
    // 实现逻辑...
    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data:    map[string]interface{}{},
    }, nil
}
```

---

### 4. 银行转账接口

#### 4.1 实现方案

银行转账数据已存在于transactions表的messages字段中，可以直接查询：

```go
// GetBankTransfers 获取银行转账记录
func (s *ExplorerService) GetBankTransfers(ctx context.Context, req *pb.GetBankTransfersRequest) (*pb.Reply, error) {
    var txs []models.Transaction

    query := db.Get().Model(&models.Transaction{})

    // 查找包含MsgSend的交易
    query = query.Where("messages LIKE ?", "%/cosmos.bank.v1beta1.MsgSend%")

    // 按发送者过滤 (需要JSON查询)
    if len(req.Senders) > 0 {
        // 构建JSON查询条件
        for _, sender := range req.Senders {
            query = query.Where("JSON_SEARCH(messages, 'one', ?, NULL, '$[*].from_address') IS NOT NULL", sender)
        }
    }

    // 时间范围
    if req.StartTime > 0 {
        query = query.Where("unix_timestamp >= ?", req.StartTime)
    }
    if req.EndTime > 0 {
        query = query.Where("unix_timestamp <= ?", req.EndTime)
    }

    // 分页和查询...
    // 提取转账信息并返回

    return &pb.Reply{
        Code:    http.StatusOK,
        Message: "Success",
        Data:    map[string]interface{}{},
    }, nil
}
```

---

## Proto定义方案

遵循项目现有风格，复用 `PaginationRequest` 和 `PaginationInfo`：

**文件**: `api/explorer/v1/validator.proto` (新建)

```protobuf
syntax = "proto3";

package api.explorer.v1;

import "api/explorer/v1/common.proto";

option go_package = "injective-explorer/api/explorer/v1;v1";

// 验证者相关请求和响应

message GetValidatorsRequest {
  PaginationRequest pagination = 1;

  // 按状态过滤: bonded, unbonding, unbonded
  string status = 2;

  // 按是否监禁过滤
  optional bool jailed = 3;

  // 排序字段，如: "tokens DESC", "uptime_percentage DESC"
  string order_by = 4 [json_name = "order_by"];
}

message GetValidatorRequest {
  // 验证者操作者地址 (injvaloper...)
  string address = 1;
}

message GetValidatorUptimeRequest {
  // 验证者操作者地址
  string address = 1;

  // 查询最近N个区块，默认10000
  int64 blocks = 2;
}
```

**文件**: `api/explorer/v1/wasm.proto` (新建)

```protobuf
syntax = "proto3";

package api.explorer.v1;

import "api/explorer/v1/common.proto";

option go_package = "injective-explorer/api/explorer/v1;v1";

// Wasm合约相关请求和响应

message GetWasmContractsRequest {
  PaginationRequest pagination = 1;

  // 按代码ID过滤
  int64 code_id = 2 [json_name = "code_id"];

  // 按创建者地址过滤
  string creator = 3;

  // 按标签过滤
  string label = 4;

  // 仅返回CW20代币合约
  bool cw20_only = 5 [json_name = "cw20_only"];
}

message GetWasmContractByAddressRequest {
  // 合约地址
  string contract_address = 1 [json_name = "contract_address"];
}

message GetCW20BalanceRequest {
  // 账户地址
  string address = 1;

  // 可选：指定合约地址
  string contract_address = 2 [json_name = "contract_address"];
}

message GetContractTxsRequest {
  // 合约地址
  string contract_address = 1 [json_name = "contract_address"];

  PaginationRequest pagination = 2;

  // 按发送者过滤
  string sender = 3;

  // 时间范围 (UNIX毫秒)
  int64 start_time = 4 [json_name = "start_time"];
  int64 end_time = 5 [json_name = "end_time"];
}

message GetWasmCodesRequest {
  PaginationRequest pagination = 1;

  // 按创建者过滤
  string creator = 2;

  // 按合约类型过滤
  string contract_type = 3 [json_name = "contract_type"];
}

message GetWasmCodeByIDRequest {
  // 代码ID
  int64 code_id = 1 [json_name = "code_id"];
}
```

**在 `explorer.proto` 中添加服务定义：**

```protobuf
service Explorer {
  // ... 现有接口 ...

  // 验证者接口
  rpc GetValidators(GetValidatorsRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/validator/list"
    };
  }

  rpc GetValidator(GetValidatorRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/validator/info"
    };
  }

  rpc GetValidatorUptime(GetValidatorUptimeRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/validator/uptime"
    };
  }

  // Wasm合约接口
  rpc GetWasmContracts(GetWasmContractsRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/wasm/contracts"
    };
  }

  rpc GetWasmContractByAddress(GetWasmContractByAddressRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/wasm/contract/info"
    };
  }

  rpc GetCW20Balance(GetCW20BalanceRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/wasm/cw20/balance"
    };
  }

  rpc GetContractTxs(GetContractTxsRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/wasm/contract/transactions"
    };
  }

  rpc GetWasmCodes(GetWasmCodesRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/wasm/codes"
    };
  }

  rpc GetWasmCodeByID(GetWasmCodeByIDRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/wasm/code/info"
    };
  }

  // 银行转账接口
  rpc GetBankTransfers(GetBankTransfersRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/bank/transfers"
    };
  }

  // 中继者接口
  rpc GetRelayers(GetRelayersRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/relayer/list"
    };
  }
}

// 示例调用方式：
// GET /api/v1/validator/list?page=1&page_size=20&status=bonded
// GET /api/v1/validator/info?address=injvaloper1...
// GET /api/v1/validator/uptime?address=injvaloper1...&blocks=10000
// GET /api/v1/wasm/contracts?page=1&page_size=20&code_id=1
// GET /api/v1/wasm/contract/info?contract_address=inj1...
// GET /api/v1/wasm/cw20/balance?address=inj1...&contract_address=inj1...
// GET /api/v1/wasm/contract/transactions?contract_address=inj1...&page=1&page_size=20
// GET /api/v1/wasm/codes?page=1&page_size=20&creator=inj1...
// GET /api/v1/wasm/code/info?code_id=1
// GET /api/v1/bank/transfers?page=1&page_size=20&senders=inj1...
// GET /api/v1/relayer/list?market_ids=0x...
```

---

## 数据库设计方案

### 关键点

1. **复用现有表结构**: validators, validator_block_stats, validator_set_snapshots, block_signers
2. **扩展contract_infos表**: 添加Wasm相关字段，保持向后兼容
3. **新增补充表**: wasm_codes, contract_executions, cw20_balances, cw20_transfers
4. **索引优化**: 基于查询模式添加合适的索引

### 数据同步策略

#### 1. 实时同步 (Block Processor)
在现有的区块处理器中添加Wasm事件处理：

```go
// internal/processor/block_processor.go (修改)
func (p *BlockProcessor) ProcessBlock(block *models.Block) error {
    // ... 现有逻辑 ...

    // 处理Wasm事件
    if err := p.processWasmEvents(block); err != nil {
        p.logger.Errorf("Failed to process wasm events: %v", err)
    }

    return nil
}

func (p *BlockProcessor) processWasmEvents(block *models.Block) error {
    // 从交易中提取Wasm消息
    // 更新合约信息、执行记录、CW20余额等
    return nil
}
```

#### 2. 定期同步 (Cron)
每小时同步验证者信息：

```go
// internal/cron/sync_validators.go (新建)
func SyncValidators() {
    // 从链上同步最新的验证者状态
    // 更新validators表
}
```

---

## 实施优先级建议

### Phase 1 - 关键功能 (2周)
**目标**: 支持基本的Wasm合约浏览

1. **数据库迁移** (2天)
   - 创建新表和扩展字段
   - 数据迁移脚本

2. **Wasm合约核心接口** (1周)
   - GetWasmContracts
   - GetWasmContractByAddress
   - GetCW20Balance

3. **Proto定义和代码生成** (1天)
   - 定义proto文件
   - 生成Go代码

4. **集成测试** (2天)

### Phase 2 - 验证者功能 (1周)
**目标**: 提供验证者查询能力

1. **验证者接口** (4天)
   - GetValidators
   - GetValidator
   - GetValidatorUptime (利用现有表)

2. **测试和优化** (1天)

### Phase 3 - 补充功能 (1周)
**目标**: 完善剩余接口

1. **Wasm代码接口** (2天)
   - GetWasmCodes
   - GetWasmCodeByID

2. **合约交易接口** (2天)
   - GetContractTxs

3. **其他接口** (1天)
   - GetBankTransfers

---

## 技术风险与挑战

### 1. 数据量问题
**风险**: 合约执行记录可能非常大
**方案**:
- 使用分区表按月分区
- 历史数据归档到对象存储
- 实施数据保留策略

### 2. 查询性能
**风险**: 复杂的JSON查询可能影响性能
**方案**:
- 使用Redis缓存常用查询
- 添加生成列和索引(MySQL 5.7+)
- 考虑使用专门的搜索引擎(Elasticsearch)

### 3. 数据一致性
**风险**: CW20余额可能与链上不一致
**方案**:
- 定期全量对账
- 提供"刷新"功能查询链上最新数据
- 显示数据更新时间

### 4. 向后兼容
**风险**: 扩展contract_infos表可能影响现有功能
**方案**:
- 新字段使用DEFAULT或NULL
- 保持现有API不变
- 添加版本标识

---

## 附录

### 代码生成命令

```bash
# 生成proto代码
make api

# 数据库迁移
make migrate

# 运行测试
make test
```

### 监控指标

建议添加以下监控：
- Wasm事件处理延迟
- CW20余额更新延迟
- 验证者数据同步状态
- 接口响应时间
- 缓存命中率

---

**文档结束**

本方案已根据项目现有结构调整，复用了现有的validators相关表，遵循了项目的Proto定义风格，并保持了代码一致性。
