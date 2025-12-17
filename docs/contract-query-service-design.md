# 智能合约查询接口系统设计方案

## 1. 概述

本文档描述了智能合约查询接口系统的设计方案，支持合约信息的增删改查、通过代币符号查询合约地址等功能，并为后续的合约验证服务预留扩展空间。

## 2. 功能需求

### 2.1 核心功能
1. **合约信息上传**：支持上传智能合约相关信息
   - 代币合约名称（Token Name）
   - 代币符号（Token Symbol）
   - 合约类型（Contract Type）
   - 合约地址（Contract Address）
   - 合约标签（Contract Tags）
   - 验证状态（Verification Status）
   - 创建时间（Created At）

2. **合约信息管理**：支持完整的 CRUD 操作
   - 创建合约信息
   - 查询合约信息（单个/列表）
   - 更新合约信息
   - 删除合约信息

3. **代币符号查询**：支持通过代币符号查询合约地址
   - 一个代币符号可以对应多个合约地址（一对多关系）
   - 支持分页查询
   - 支持按链ID、合约类型等条件过滤

4. **扩展性设计**：为后续功能预留空间
   - 合约验证服务集成
   - 合约源代码管理
   - 合约ABI/Schema管理
   - 合约事件查询
   - 合约调用历史

## 3. 数据库设计

### 3.1 合约信息表 (token_contracts)

```sql
CREATE TABLE token_contracts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    -- 基本信息
    contract_address VARCHAR(128) NOT NULL COMMENT '合约地址',
    token_name VARCHAR(255) NOT NULL COMMENT '代币合约名称',
    token_symbol VARCHAR(50) NOT NULL COMMENT '代币符号',
    contract_type VARCHAR(50) DEFAULT 'cosmwasm' COMMENT '合约类型: cosmwasm, evm, etc.',
    chain_id VARCHAR(50) DEFAULT 'injective-1' COMMENT '链ID',
    
    -- 验证相关（预留扩展）
    verification_status VARCHAR(50) DEFAULT 'unverified' COMMENT '验证状态: unverified, pending, verified, failed',
    is_verified BOOLEAN DEFAULT FALSE COMMENT '是否已验证',
    verified_at TIMESTAMP NULL COMMENT '验证时间',
    verified_by VARCHAR(128) COMMENT '验证者地址',
    
    -- 扩展字段（JSON格式，用于存储额外信息）
    metadata JSON COMMENT '扩展元数据（JSON格式）',
    
    -- 时间戳
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    deleted_at TIMESTAMP NULL COMMENT '软删除时间',
    
    -- 索引
    UNIQUE KEY uk_chain_address (chain_id, contract_address),
    INDEX idx_token_symbol (token_symbol),
    INDEX idx_contract_type (contract_type),
    INDEX idx_chain_id (chain_id),
    INDEX idx_verification_status (verification_status),
    INDEX idx_created_at (created_at),
    INDEX idx_token_symbol_chain (token_symbol, chain_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='代币合约信息表';
```

**设计说明：**
- `contract_address` + `chain_id` 唯一约束，支持多链
- `token_symbol` 建立索引，支持快速查询
- `token_symbol` + `chain_id` 联合索引，优化多链查询
- 使用软删除（`deleted_at`），保留历史数据
- `metadata` JSON字段用于存储扩展信息，便于后续功能扩展

### 3.2 合约标签表 (contract_tags)

```sql
CREATE TABLE contract_tags (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    contract_address VARCHAR(128) NOT NULL COMMENT '合约地址',
    chain_id VARCHAR(50) NOT NULL COMMENT '链ID',
    tag_type VARCHAR(50) NOT NULL COMMENT '标签类型: official, popular, scam, verified, etc.',
    tag_value VARCHAR(255) COMMENT '标签值',
    tag_color VARCHAR(20) COMMENT '标签颜色(用于前端显示)',
    description TEXT COMMENT '标签描述',
    created_by VARCHAR(128) COMMENT '创建者地址',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    
    -- 索引
    UNIQUE KEY uk_contract_tag (chain_id, contract_address, tag_type),
    INDEX idx_contract_address (chain_id, contract_address),
    INDEX idx_tag_type (tag_type),
    FOREIGN KEY (chain_id, contract_address) REFERENCES token_contracts(chain_id, contract_address) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合约标签表';
```

**设计说明：**
- 支持一个合约多个标签
- 标签类型可扩展（official, popular, scam, verified等）
- 外键约束确保数据一致性

### 3.3 合约关联表（可选，用于关联现有合约表）

如果需要与现有的 `contracts` 表（见 `injective-contract-verification-design.md`）关联，可以建立关联表：

```sql
CREATE TABLE contract_relations (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    token_contract_id BIGINT NOT NULL COMMENT '代币合约ID（token_contracts表）',
    contract_id BIGINT COMMENT '合约ID（contracts表，可选）',
    relation_type VARCHAR(50) DEFAULT 'same' COMMENT '关联类型: same, proxy, upgrade, etc.',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_token_contract (token_contract_id),
    INDEX idx_contract (contract_id),
    FOREIGN KEY (token_contract_id) REFERENCES token_contracts(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合约关联表';
```

## 4. 数据模型设计

### 4.1 TokenContract 模型

```go
// internal/models/token_contract.go (模型文件保持原名，因为表名是 token_contracts)
package models

import (
    "time"
    "gorm.io/gorm"
)

// TokenContract 代币合约信息模型
type TokenContract struct {
    gorm.Model
    
    // 基本信息
    ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null"`
    TokenName       string `json:"token_name" gorm:"column:token_name;size:255;not null"`
    TokenSymbol     string `json:"token_symbol" gorm:"column:token_symbol;size:50;not null;index:idx_token_symbol"`
    ContractType    string `json:"contract_type" gorm:"column:contract_type;size:50;default:cosmwasm;index"`
    ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50;default:injective-1;index"`
    
    // 验证相关（预留扩展）
    VerificationStatus string     `json:"verification_status" gorm:"column:verification_status;size:50;default:unverified;index"`
    IsVerified         bool       `json:"is_verified" gorm:"column:is_verified;default:false"`
    VerifiedAt         *time.Time `json:"verified_at" gorm:"column:verified_at"`
    VerifiedBy         string     `json:"verified_by" gorm:"column:verified_by;size:128"`
    
    // 扩展字段
    Metadata map[string]interface{} `json:"metadata" gorm:"column:metadata;type:json"`
    
    // 关联关系
    Tags []ContractTag `json:"tags" gorm:"foreignKey:ContractAddress,ChainID;references:ContractAddress,ChainID"`
    
    // 唯一约束
    // UNIQUE(chain_id, contract_address) - 通过数据库约束实现
}

// TableName 指定表名
func (TokenContract) TableName() string {
    return "token_contracts"
}
```

### 4.2 ContractTag 模型（复用或扩展现有模型）

如果已有 `ContractTag` 模型，可以扩展；否则创建新模型：

```go
// internal/models/contract_tag.go
package models

import (
    "gorm.io/gorm"
)

// ContractTag 合约标签模型
type ContractTag struct {
    gorm.Model
    
    ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null"`
    ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50;not null"`
    TagType         string `json:"tag_type" gorm:"column:tag_type;size:50;not null;index"`
    TagValue        string `json:"tag_value" gorm:"column:tag_value;size:255"`
    TagColor        string `json:"tag_color" gorm:"column:tag_color;size:20"`
    Description     string `json:"description" gorm:"column:description;type:text"`
    CreatedBy       string `json:"created_by" gorm:"column:created_by;size:128"`
    
    // 唯一约束
    // UNIQUE(chain_id, contract_address, tag_type) - 通过数据库约束实现
}

// TableName 指定表名
func (ContractTag) TableName() string {
    return "contract_tags"
}
```

## 5. Protobuf API 设计

### 5.1 创建新的合约服务 Proto 文件

```protobuf
// api/explorer/v1/contract_info.proto
syntax = "proto3";

package api.explorer.v1;

import "explorer/v1/common.proto";
import "google/api/annotations.proto";
import "google/protobuf/empty.proto";

option go_package = "injective-explorer/api/explorer/v1;v1";
option java_multiple_files = true;
option java_package = "api.explorer.v1";

// ContractInfoService 合约信息查询服务（支持多种合约类型）
service ContractInfoService {
  // 创建合约信息
  rpc CreateTokenContract(CreateTokenContractRequest) returns (TokenContractResponse) {
    option (google.api.http) = {
      post: "/api/v1/token-contract"
      body: "*"
    };
  }
  
  // 获取合约信息（通过地址）
  rpc GetTokenContractByAddress(GetTokenContractByAddressRequest) returns (TokenContractResponse) {
    option (google.api.http) = {
      get: "/api/v1/token-contract/by-address"
    };
  }
  
  // 通过代币符号查询合约地址（支持一对多）
  rpc GetTokenContractsBySymbol(GetTokenContractsBySymbolRequest) returns (TokenContractListResponse) {
    option (google.api.http) = {
      get: "/api/v1/token-contract/by-symbol"
    };
  }
  
  // 查询合约列表（支持多种过滤条件）
  rpc ListTokenContracts(ListTokenContractsRequest) returns (TokenContractListResponse) {
    option (google.api.http) = {
      get: "/api/v1/token-contract/list"
    };
  }
  
  // 更新合约信息
  rpc UpdateTokenContract(UpdateTokenContractRequest) returns (TokenContractResponse) {
    option (google.api.http) = {
      put: "/api/v1/token-contract"
      body: "*"
    };
  }
  
  // 删除合约信息（软删除）
  rpc DeleteTokenContract(DeleteTokenContractRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      delete: "/api/v1/token-contract"
    };
  }
  
  // 添加合约标签
  rpc AddContractTag(AddContractTagRequest) returns (ContractTagResponse) {
    option (google.api.http) = {
      post: "/api/v1/token-contract/tag"
      body: "*"
    };
  }
  
  // 删除合约标签
  rpc RemoveContractTag(RemoveContractTagRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      delete: "/api/v1/token-contract/tag"
    };
  }
  
  // 获取合约标签列表
  rpc GetContractTags(GetContractTagsRequest) returns (ContractTagListResponse) {
    option (google.api.http) = {
      get: "/api/v1/token-contract/tags"
    };
  }
}

// 请求消息
message CreateTokenContractRequest {
  string contract_address = 1;  // 合约地址
  string token_name = 2;         // 代币合约名称
  string token_symbol = 3;       // 代币符号
  string contract_type = 4;      // 合约类型（可选，默认cosmwasm）
  string chain_id = 5;            // 链ID（可选，默认injective-1）
  string verification_status = 6; // 验证状态（可选，默认unverified）
  map<string, string> metadata = 7; // 扩展元数据（可选）
  repeated string tags = 8;      // 标签列表（可选）
}

message GetTokenContractByAddressRequest {
  string contract_address = 1;    // 合约地址
  string chain_id = 2;            // 链ID（可选）
}

message GetTokenContractsBySymbolRequest {
  string token_symbol = 1;        // 代币符号
  string chain_id = 2;            // 链ID（可选，用于过滤）
  string contract_type = 3;       // 合约类型（可选，用于过滤）
  PaginationRequest pagination = 4; // 分页参数
}

message ListTokenContractsRequest {
  string chain_id = 1;            // 链ID（可选）
  string contract_type = 2;       // 合约类型（可选）
  string verification_status = 3; // 验证状态（可选）
  string token_symbol = 4;        // 代币符号（可选，模糊匹配）
  PaginationRequest pagination = 5; // 分页参数
}

message UpdateTokenContractRequest {
  string contract_address = 1;    // 合约地址（必填）
  string chain_id = 2;            // 链ID（必填）
  string token_name = 3;          // 代币合约名称（可选）
  string token_symbol = 4;        // 代币符号（可选）
  string contract_type = 5;       // 合约类型（可选）
  string verification_status = 6; // 验证状态（可选）
  map<string, string> metadata = 7; // 扩展元数据（可选）
}

message DeleteTokenContractRequest {
  string contract_address = 1;    // 合约地址
  string chain_id = 2;            // 链ID
}

message AddContractTagRequest {
  string contract_address = 1;    // 合约地址
  string chain_id = 2;            // 链ID
  string tag_type = 3;            // 标签类型
  string tag_value = 4;           // 标签值（可选）
  string tag_color = 5;           // 标签颜色（可选）
  string description = 6;         // 标签描述（可选）
  string created_by = 7;          // 创建者（可选）
}

message RemoveContractTagRequest {
  string contract_address = 1;    // 合约地址
  string chain_id = 2;           // 链ID
  string tag_type = 3;           // 标签类型
}

message GetContractTagsRequest {
  string contract_address = 1;    // 合约地址
  string chain_id = 2;           // 链ID
}

// 响应消息
message TokenContractInfo {
  uint64 id = 1;
  string contract_address = 2;
  string token_name = 3;
  string token_symbol = 4;
  string contract_type = 5;
  string chain_id = 6;
  string verification_status = 7;
  bool is_verified = 8;
  string verified_at = 9;          // ISO 8601格式时间戳
  string verified_by = 10;
  map<string, string> metadata = 11;
  repeated ContractTagInfo tags = 12;
  string created_at = 13;         // ISO 8601格式时间戳
  string updated_at = 14;         // ISO 8601格式时间戳
}

message ContractTagInfo {
  uint64 id = 1;
  string contract_address = 2;
  string chain_id = 3;
  string tag_type = 4;
  string tag_value = 5;
  string tag_color = 6;
  string description = 7;
  string created_by = 8;
  string created_at = 9;
}

message TokenContractResponse {
  int32 code = 1;
  string message = 2;
  TokenContractInfo data = 3;
}

message TokenContractListResponse {
  int32 code = 1;
  string message = 2;
  repeated TokenContractInfo data = 3;
  PaginationInfo pagination = 4;
}

message ContractTagResponse {
  int32 code = 1;
  string message = 2;
  ContractTagInfo data = 3;
}

message ContractTagListResponse {
  int32 code = 1;
  string message = 2;
  repeated ContractTagInfo data = 3;
}
```

## 6. 服务层实现设计

### 6.1 TokenContractService 服务结构

```go
// internal/service/contract_info.go
package service

import (
    "context"
    "time"
    
    pb "injective-explorer/api/explorer/v1"
    "injective-explorer/internal/models"
    "injective-explorer/provider/cache"
    "injective-explorer/provider/db"
    
    "github.com/pkg/errors"
    "gorm.io/gorm"
)

type TokenContractService struct {
    pb.UnimplementedTokenContractServiceServer
}

func NewTokenContractService() *TokenContractService {
    return &TokenContractService{}
}

// CreateTokenContract 创建合约信息
func (s *TokenContractService) CreateTokenContract(ctx context.Context, req *pb.CreateTokenContractRequest) (*pb.TokenContractResponse, error) {
    // 1. 参数验证
    // 2. 检查合约是否已存在
    // 3. 创建合约记录
    // 4. 创建标签（如果有）
    // 5. 清除缓存
    // 6. 返回结果
}

// GetContractInfoByAddress 通过地址获取合约信息
func (s *ContractInfoService) GetContractInfoByAddress(ctx context.Context, req *pb.GetContractInfoByAddressRequest) (*pb.ContractInfoResponse, error) {
    // 1. 参数验证
    // 2. 尝试从缓存获取
    // 3. 从数据库查询
    // 4. 加载关联标签
    // 5. 写入缓存
    // 6. 返回结果
}

// GetContractsBySymbol 通过符号查询合约地址（一对多）
func (s *ContractInfoService) GetContractsBySymbol(ctx context.Context, req *pb.GetContractsBySymbolRequest) (*pb.ContractInfoListResponse, error) {
    // 1. 参数验证
    // 2. 构建查询条件（token_symbol, chain_id, contract_type）
    // 3. 分页查询
    // 4. 加载关联标签
    // 5. 返回结果列表
}

// ListContractInfos 查询合约列表
func (s *ContractInfoService) ListContractInfos(ctx context.Context, req *pb.ListContractInfosRequest) (*pb.ContractInfoListResponse, error) {
    // 1. 参数验证
    // 2. 构建查询条件（支持多条件过滤）
    // 3. 分页查询
    // 4. 返回结果列表
}

// UpdateContractInfo 更新合约信息
func (s *ContractInfoService) UpdateContractInfo(ctx context.Context, req *pb.UpdateContractInfoRequest) (*pb.ContractInfoResponse, error) {
    // 1. 参数验证
    // 2. 查询合约是否存在
    // 3. 更新字段（只更新提供的字段）
    // 4. 清除缓存
    // 5. 返回更新后的结果
}

// DeleteContractInfo 删除合约信息（软删除）
func (s *ContractInfoService) DeleteContractInfo(ctx context.Context, req *pb.DeleteContractInfoRequest) (*emptypb.Empty, error) {
    // 1. 参数验证
    // 2. 软删除（设置deleted_at）
    // 3. 清除缓存
    // 4. 返回成功
}
```

### 6.2 缓存策略

```go
const (
    CacheKeyTokenContractByAddress = "token_contract:address:%s:%s"  // chain_id:address
    CacheKeyTokenContractsBySymbol = "token_contract:symbol:%s:%s" // chain_id:symbol
    CacheKeyContractTags = "token_contract:tags:%s:%s"              // chain_id:address
)

const (
    CacheExpireTokenContract = 1 * time.Hour
    CacheExpireContractTags = 1 * time.Hour
)
```

## 7. 数据库迁移

### 7.1 创建迁移文件

在 `database/` 目录下创建 SQL 迁移文件：

```sql
-- database/token_contracts.sql
-- 创建代币合约信息表
CREATE TABLE IF NOT EXISTS token_contracts (
    -- ... (见3.1节)
);

-- 创建合约标签表
CREATE TABLE IF NOT EXISTS contract_tags (
    -- ... (见3.2节)
);
```

### 7.2 更新 AutoMigrate

在 `internal/global/global.go` 中添加：

```go
err = db.Get().AutoMigrate(
    &models.Block{},
    &models.Transaction{},
    &models.Account{},
    &models.AccountBalance{},
    &models.TransactionDataArchive{},
    &models.TokenContract{},  // 新增
    &models.ContractTag{},    // 新增或更新
)
```

## 8. 服务注册

### 8.1 注册到 gRPC 服务器

在 `internal/server/grpc.go` 中：

```go
contractInfoService := service.NewContractInfoService()
v1.RegisterContractInfoServiceServer(srv, contractInfoService)
```

### 8.2 注册到 HTTP 服务器

在 `internal/server/http.go` 中：

```go
contractInfoService := service.NewContractInfoService()
v1.RegisterContractInfoServiceHTTPServer(srv, contractInfoService)
```

## 9. 扩展性设计

### 9.1 与现有合约验证服务集成

- 通过 `contract_address` 和 `chain_id` 关联现有的 `contracts` 表
- 共享验证状态和验证记录
- 可以创建 `contract_relations` 表建立关联关系

### 9.2 预留扩展字段

- `metadata` JSON字段：可以存储任意扩展信息
  - 代币精度（decimals）
  - 代币总量（total_supply）
  - 合约ABI
  - 合约文档链接
  - 社交媒体链接
  - 等等

### 9.3 后续功能扩展点

1. **合约验证服务集成**
   - 关联 `contracts` 表和 `contract_verifications` 表
   - 同步验证状态

2. **合约源代码管理**
   - 关联 `contract_source_files` 表
   - 支持源代码查看和下载

3. **合约ABI/Schema管理**
   - 在 `metadata` 中存储或关联独立表

4. **合约事件查询**
   - 关联交易表，查询合约相关事件

5. **合约调用历史**
   - 关联交易表，查询合约调用记录

6. **代币价格和市值**
   - 在 `metadata` 中存储或关联价格表

## 10. API 使用示例

### 10.1 创建合约信息

```bash
curl -X POST "http://localhost:8000/api/v1/token-contract" \
  -H "Content-Type: application/json" \
  -d '{
    "contract_address": "inj1...",
    "token_name": "My Token",
    "token_symbol": "MTK",
    "contract_type": "cosmwasm",
    "chain_id": "injective-1",
    "verification_status": "unverified",
    "tags": ["official", "popular"]
  }'
```

### 10.2 通过代币符号查询合约地址

```bash
curl "http://localhost:8000/api/v1/token-contract/by-symbol?token_symbol=MTK&chain_id=injective-1&page=1&page_size=20"
```

### 10.3 查询合约信息

```bash
curl "http://localhost:8000/api/v1/token-contract/by-address?contract_address=inj1...&chain_id=injective-1"
```

### 10.4 更新合约信息

```bash
curl -X PUT "http://localhost:8000/api/v1/token-contract" \
  -H "Content-Type: application/json" \
  -d '{
    "contract_address": "inj1...",
    "chain_id": "injective-1",
    "token_name": "Updated Token Name",
    "verification_status": "verified"
  }'
```

### 10.5 添加标签

```bash
curl -X POST "http://localhost:8000/api/v1/token-contract/tag" \
  -H "Content-Type: application/json" \
  -d '{
    "contract_address": "inj1...",
    "chain_id": "injective-1",
    "tag_type": "official",
    "tag_value": "Official Contract",
    "tag_color": "green"
  }'
```

## 11. 实现步骤

1. **第一阶段：数据库设计**
   - 创建数据库表结构
   - 创建数据模型
   - 更新 AutoMigrate

2. **第二阶段：Protobuf 定义**
   - 创建 `contract_info.proto`
   - 生成 Protobuf 代码

3. **第三阶段：服务层实现**
   - 实现 ContractInfoService
   - 实现缓存逻辑
   - 实现错误处理

4. **第四阶段：服务注册**
   - 注册到 gRPC 服务器
   - 注册到 HTTP 服务器

5. **第五阶段：测试**
   - 单元测试
   - 集成测试
   - API 测试

## 12. 注意事项

1. **数据一致性**：确保 `chain_id` + `contract_address` 的唯一性
2. **缓存失效**：更新或删除合约时，需要清除相关缓存
3. **软删除**：使用软删除保留历史数据，便于审计和恢复
4. **索引优化**：根据查询模式优化索引，特别是 `token_symbol` 查询
5. **分页性能**：对于大数据集，考虑使用游标分页
6. **扩展性**：使用 JSON 字段存储扩展信息，避免频繁修改表结构
