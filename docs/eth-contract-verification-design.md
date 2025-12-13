# 智能合约查看功能技术方案

## 1. 概述

本文档描述了智能合约查看功能的后端技术方案，包括合约验证状态管理、合约标签管理、合约源代码和ABI存储等功能。

参考实现：
- [Etherscan Contract View](https://etherscan.io/address/0x000000000022d473030f116ddee9f6b43ac78ba3#code)
- [Blockscout Contract View](https://eth.blockscout.com/address/0x000000000022D473030F116dDEE9F6B43aC78BA3?tab=index)

## 2. 数据库设计

### 2.1 合约信息表 (contracts)

存储合约的基本信息和验证状态。

```sql
CREATE TABLE contracts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    address VARCHAR(128) NOT NULL UNIQUE COMMENT '合约地址',
    name VARCHAR(255) COMMENT '合约名称',
    is_verified BOOLEAN DEFAULT FALSE COMMENT '是否已验证',
    verification_status VARCHAR(50) DEFAULT 'unverified' COMMENT '验证状态: unverified, pending, verified, failed',
    compiler_version VARCHAR(100) COMMENT '编译器版本',
    optimization_enabled BOOLEAN DEFAULT FALSE COMMENT '是否启用优化',
    optimization_runs INT DEFAULT 0 COMMENT '优化运行次数',
    evm_version VARCHAR(50) COMMENT 'EVM版本',
    license VARCHAR(100) COMMENT '许可证类型',
    creation_code TEXT COMMENT '创建代码(bytecode)',
    runtime_code TEXT COMMENT '运行时代码(bytecode)',
    code_hash VARCHAR(128) COMMENT '代码哈希',
    source_code_storage_key VARCHAR(512) COMMENT '源代码在对象存储中的key',
    abi_storage_key VARCHAR(512) COMMENT 'ABI在对象存储中的key',
    constructor_args TEXT COMMENT '构造函数参数',
    verified_at TIMESTAMP NULL COMMENT '验证时间',
    verified_by VARCHAR(128) COMMENT '验证者地址',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    INDEX idx_address (address),
    INDEX idx_verification_status (verification_status),
    INDEX idx_is_verified (is_verified)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='智能合约信息表';
```

### 2.2 合约标签表 (contract_tags)

存储合约的标签信息，支持多种标签类型。

```sql
CREATE TABLE contract_tags (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    contract_address VARCHAR(128) NOT NULL COMMENT '合约地址',
    tag_type VARCHAR(50) NOT NULL COMMENT '标签类型: verified, official, popular, scam, etc.',
    tag_value VARCHAR(255) COMMENT '标签值',
    tag_color VARCHAR(20) COMMENT '标签颜色(用于前端显示)',
    description TEXT COMMENT '标签描述',
    created_by VARCHAR(128) COMMENT '创建者地址',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    UNIQUE KEY uk_contract_tag (contract_address, tag_type),
    INDEX idx_contract_address (contract_address),
    INDEX idx_tag_type (tag_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合约标签表';
```

### 2.3 合约验证记录表 (contract_verifications)

存储合约验证的详细记录和历史。

```sql
CREATE TABLE contract_verifications (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    contract_address VARCHAR(128) NOT NULL COMMENT '合约地址',
    verification_type VARCHAR(50) NOT NULL COMMENT '验证类型: full_match, partial_match, etc.',
    compiler_version VARCHAR(100) NOT NULL COMMENT '编译器版本',
    optimization_enabled BOOLEAN DEFAULT FALSE COMMENT '是否启用优化',
    optimization_runs INT DEFAULT 0 COMMENT '优化运行次数',
    evm_version VARCHAR(50) COMMENT 'EVM版本',
    license VARCHAR(100) COMMENT '许可证类型',
    source_code TEXT COMMENT '源代码(JSON格式)',
    source_code_storage_key VARCHAR(512) COMMENT '源代码在对象存储中的key',
    abi JSON COMMENT 'ABI(JSON格式)',
    abi_storage_key VARCHAR(512) COMMENT 'ABI在对象存储中的key',
    constructor_args TEXT COMMENT '构造函数参数',
    verification_status VARCHAR(50) DEFAULT 'pending' COMMENT '验证状态: pending, verified, failed',
    error_message TEXT COMMENT '错误信息(如果验证失败)',
    verified_at TIMESTAMP NULL COMMENT '验证时间',
    verified_by VARCHAR(128) COMMENT '验证者地址',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_contract_address (contract_address),
    INDEX idx_verification_status (verification_status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合约验证记录表';
```

### 2.4 合约源代码文件表 (contract_source_files)

存储合约的源代码文件信息（支持多文件合约）。

```sql
CREATE TABLE contract_source_files (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    contract_address VARCHAR(128) NOT NULL COMMENT '合约地址',
    verification_id BIGINT COMMENT '关联的验证记录ID',
    file_path VARCHAR(512) NOT NULL COMMENT '文件路径',
    file_name VARCHAR(255) NOT NULL COMMENT '文件名',
    file_content TEXT COMMENT '文件内容',
    file_content_storage_key VARCHAR(512) COMMENT '文件内容在对象存储中的key',
    file_hash VARCHAR(128) COMMENT '文件哈希',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_contract_address (contract_address),
    INDEX idx_verification_id (verification_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合约源代码文件表';
```

## 3. 数据模型设计

### 3.1 Contract 模型

```go
// internal/models/contract.go
package models

import (
    "gorm.io/gorm"
    "time"
)

// Contract 表示智能合约信息
type Contract struct {
    gorm.Model
    
    Address              string    `json:"address" gorm:"column:address;uniqueIndex;size:128"`
    Name                 string    `json:"name" gorm:"column:name;size:255"`
    IsVerified           bool      `json:"is_verified" gorm:"column:is_verified;default:false"`
    VerificationStatus   string    `json:"verification_status" gorm:"column:verification_status;default:unverified;size:50;index"`
    CompilerVersion      string    `json:"compiler_version" gorm:"column:compiler_version;size:100"`
    OptimizationEnabled  bool      `json:"optimization_enabled" gorm:"column:optimization_enabled;default:false"`
    OptimizationRuns     int       `json:"optimization_runs" gorm:"column:optimization_runs;default:0"`
    EVMVersion           string    `json:"evm_version" gorm:"column:evm_version;size:50"`
    License              string    `json:"license" gorm:"column:license;size:100"`
    CreationCode         string    `json:"creation_code" gorm:"column:creation_code;type:text"`
    RuntimeCode          string    `json:"runtime_code" gorm:"column:runtime_code;type:text"`
    CodeHash             string    `json:"code_hash" gorm:"column:code_hash;size:128"`
    SourceCodeStorageKey string    `json:"source_code_storage_key" gorm:"column:source_code_storage_key;size:512"`
    ABIStorageKey        string    `json:"abi_storage_key" gorm:"column:abi_storage_key;size:512"`
    ConstructorArgs      string    `json:"constructor_args" gorm:"column:constructor_args;type:text"`
    VerifiedAt           *time.Time `json:"verified_at" gorm:"column:verified_at"`
    VerifiedBy           string    `json:"verified_by" gorm:"column:verified_by;size:128"`
    
    // 关联关系
    Tags                 []ContractTag `json:"tags" gorm:"foreignKey:ContractAddress;references:Address"`
    Verifications        []ContractVerification `json:"verifications" gorm:"foreignKey:ContractAddress;references:Address"`
}

// ContractTag 表示合约标签
type ContractTag struct {
    gorm.Model
    
    ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;uniqueIndex:uk_contract_tag"`
    TagType         string `json:"tag_type" gorm:"column:tag_type;size:50;uniqueIndex:uk_contract_tag"`
    TagValue        string `json:"tag_value" gorm:"column:tag_value;size:255"`
    TagColor        string `json:"tag_color" gorm:"column:tag_color;size:20"`
    Description     string `json:"description" gorm:"column:description;type:text"`
    CreatedBy       string `json:"created_by" gorm:"column:created_by;size:128"`
}

// ContractVerification 表示合约验证记录
type ContractVerification struct {
    gorm.Model
    
    ContractAddress      string    `json:"contract_address" gorm:"column:contract_address;size:128;index"`
    VerificationType     string    `json:"verification_type" gorm:"column:verification_type;size:50"`
    CompilerVersion      string    `json:"compiler_version" gorm:"column:compiler_version;size:100"`
    OptimizationEnabled  bool      `json:"optimization_enabled" gorm:"column:optimization_enabled;default:false"`
    OptimizationRuns     int       `json:"optimization_runs" gorm:"column:optimization_runs;default:0"`
    EVMVersion           string    `json:"evm_version" gorm:"column:evm_version;size:50"`
    License              string    `json:"license" gorm:"column:license;size:100"`
    SourceCode           string    `json:"source_code" gorm:"column:source_code;type:text"`
    SourceCodeStorageKey string    `json:"source_code_storage_key" gorm:"column:source_code_storage_key;size:512"`
    ABI                  string    `json:"abi" gorm:"column:abi;type:json"`
    ABIStorageKey        string    `json:"abi_storage_key" gorm:"column:abi_storage_key;size:512"`
    ConstructorArgs      string    `json:"constructor_args" gorm:"column:constructor_args;type:text"`
    VerificationStatus   string    `json:"verification_status" gorm:"column:verification_status;default:pending;size:50;index"`
    ErrorMessage         string    `json:"error_message" gorm:"column:error_message;type:text"`
    VerifiedAt           *time.Time `json:"verified_at" gorm:"column:verified_at"`
    VerifiedBy           string    `json:"verified_by" gorm:"column:verified_by;size:128"`
}

// ContractSourceFile 表示合约源代码文件
type ContractSourceFile struct {
    gorm.Model
    
    ContractAddress      string `json:"contract_address" gorm:"column:contract_address;size:128;index"`
    VerificationID       uint   `json:"verification_id" gorm:"column:verification_id;index"`
    FilePath             string `json:"file_path" gorm:"column:file_path;size:512"`
    FileName             string `json:"file_name" gorm:"column:file_name;size:255"`
    FileContent          string `json:"file_content" gorm:"column:file_content;type:text"`
    FileContentStorageKey string `json:"file_content_storage_key" gorm:"column:file_content_storage_key;size:512"`
    FileHash             string `json:"file_hash" gorm:"column:file_hash;size:128"`
}
```

## 4. API 接口设计

### 4.1 Protobuf 定义

```protobuf
// api/explorer/v1/contract.proto
syntax = "proto3";

package api.explorer.v1;

import "google/api/annotations.proto";
import "explorer/v1/common.proto";

service Contract {
  // 获取合约信息
  rpc GetContractInfo(GetContractInfoRequest) returns (ContractInfoResponse) {
    option (google.api.http) = {
      get: "/api/v1/contract/info"
    };
  }

  // 获取合约源代码
  rpc GetContractSourceCode(GetContractSourceCodeRequest) returns (ContractSourceCodeResponse) {
    option (google.api.http) = {
      get: "/api/v1/contract/source-code"
    };
  }

  // 获取合约ABI
  rpc GetContractABI(GetContractABIRequest) returns (ContractABIResponse) {
    option (google.api.http) = {
      get: "/api/v1/contract/abi"
    };
  }

  // 提交合约验证
  rpc SubmitContractVerification(SubmitContractVerificationRequest) returns (SubmitContractVerificationResponse) {
    option (google.api.http) = {
      post: "/api/v1/contract/verify"
      body: "*"
    };
  }

  // 获取合约验证状态
  rpc GetVerificationStatus(GetVerificationStatusRequest) returns (VerificationStatusResponse) {
    option (google.api.http) = {
      get: "/api/v1/contract/verification-status"
    };
  }

  // 获取合约标签
  rpc GetContractTags(GetContractTagsRequest) returns (ContractTagsResponse) {
    option (google.api.http) = {
      get: "/api/v1/contract/tags"
    };
  }

  // 添加合约标签
  rpc AddContractTag(AddContractTagRequest) returns (AddContractTagResponse) {
    option (google.api.http) = {
      post: "/api/v1/contract/tag"
      body: "*"
    };
  }

  // 删除合约标签
  rpc RemoveContractTag(RemoveContractTagRequest) returns (RemoveContractTagResponse) {
    option (google.api.http) = {
      delete: "/api/v1/contract/tag"
    };
  }
}

// 请求消息
message GetContractInfoRequest {
  string address = 1;
}

message GetContractSourceCodeRequest {
  string address = 1;
}

message GetContractABIRequest {
  string address = 1;
}

message SubmitContractVerificationRequest {
  string address = 1;
  string compiler_version = 2;
  bool optimization_enabled = 3;
  int32 optimization_runs = 4;
  string evm_version = 5;
  string license = 6;
  string source_code = 7; // JSON格式的源代码
  string constructor_args = 8;
  string verification_type = 9; // full_match, partial_match
}

message GetVerificationStatusRequest {
  string address = 1;
}

message GetContractTagsRequest {
  string address = 1;
}

message AddContractTagRequest {
  string address = 1;
  string tag_type = 2; // verified, official, popular, scam, etc.
  string tag_value = 3;
  string tag_color = 4;
  string description = 5;
  string created_by = 6;
}

message RemoveContractTagRequest {
  string address = 1;
  string tag_type = 2;
}

// 响应消息
message ContractInfoResponse {
  ContractInfo contract = 1;
}

message ContractInfo {
  string address = 1;
  string name = 2;
  bool is_verified = 3;
  string verification_status = 4;
  string compiler_version = 5;
  bool optimization_enabled = 6;
  int32 optimization_runs = 7;
  string evm_version = 8;
  string license = 9;
  string code_hash = 10;
  int64 verified_at = 11;
  string verified_by = 12;
  repeated ContractTag tags = 13;
}

message ContractSourceCodeResponse {
  string address = 1;
  repeated SourceFile source_files = 2;
  string compiler_version = 3;
  bool optimization_enabled = 4;
  int32 optimization_runs = 5;
  string evm_version = 6;
  string license = 7;
}

message SourceFile {
  string file_path = 1;
  string file_name = 2;
  string file_content = 3;
}

message ContractABIResponse {
  string address = 1;
  string abi = 2; // JSON格式的ABI
}

message SubmitContractVerificationResponse {
  bool success = 1;
  string message = 2;
  string verification_id = 3;
}

message VerificationStatusResponse {
  string address = 1;
  string verification_status = 2;
  string error_message = 3;
  int64 verified_at = 4;
}

message ContractTagsResponse {
  string address = 1;
  repeated ContractTag tags = 2;
}

message ContractTag {
  string tag_type = 1;
  string tag_value = 2;
  string tag_color = 3;
  string description = 4;
  int64 created_at = 5;
}
```

## 5. 服务层实现

### 5.1 合约服务接口

```go
// internal/service/contract.go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    pb "injective-explorer/api/explorer/v1"
    "injective-explorer/internal/models"
    "injective-explorer/provider/db"
    "injective-explorer/provider/storage"

    "github.com/pkg/errors"
    "gorm.io/gorm"
)

// ContractService 处理合约相关的业务逻辑
type ContractService struct {
    pb.UnimplementedContractServer
    storage storage.Storage
}

// NewContractService 创建合约服务实例
func NewContractService(storage storage.Storage) *ContractService {
    return &ContractService{
        storage: storage,
    }
}

// GetContractInfo 获取合约信息
func (s *ContractService) GetContractInfo(ctx context.Context, req *pb.GetContractInfoRequest) (*pb.ContractInfoResponse, error) {
    if req == nil || req.Address == "" {
        return nil, errors.New("address is required")
    }

    var contract models.Contract
    err := db.Get().WithContext(ctx).
        Preload("Tags").
        Where("address = ?", req.Address).
        First(&contract).Error
    
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            // 如果合约不存在，尝试从链上获取基本信息
            return s.getContractInfoFromChain(ctx, req.Address)
        }
        return nil, errors.Wrap(err, "failed to get contract info")
    }

    return &pb.ContractInfoResponse{
        Contract: s.contractToProto(&contract),
    }, nil
}

// SubmitContractVerification 提交合约验证
func (s *ContractService) SubmitContractVerification(ctx context.Context, req *pb.SubmitContractVerificationRequest) (*pb.SubmitContractVerificationResponse, error) {
    if req == nil || req.Address == "" {
        return nil, errors.New("address is required")
    }

    // 1. 创建验证记录
    verification := &models.ContractVerification{
        ContractAddress:     req.Address,
        VerificationType:    req.VerificationType,
        CompilerVersion:     req.CompilerVersion,
        OptimizationEnabled: req.OptimizationEnabled,
        OptimizationRuns:    int(req.OptimizationRuns),
        EVMVersion:          req.EvmVersion,
        License:             req.License,
        SourceCode:          req.SourceCode,
        ConstructorArgs:     req.ConstructorArgs,
        VerificationStatus:  "pending",
    }

    // 2. 存储源代码到对象存储
    sourceCodeKey := fmt.Sprintf("contracts/%s/source-code/%d.json", req.Address, time.Now().Unix())
    if err := s.storage.Put(ctx, sourceCodeKey, []byte(req.SourceCode)); err != nil {
        return nil, errors.Wrap(err, "failed to store source code")
    }
    verification.SourceCodeStorageKey = sourceCodeKey

    // 3. 保存验证记录
    if err := db.Get().WithContext(ctx).Create(verification).Error; err != nil {
        return nil, errors.Wrap(err, "failed to create verification record")
    }

    // 4. 异步执行验证
    go s.verifyContract(context.Background(), verification)

    return &pb.SubmitContractVerificationResponse{
        Success:        true,
        Message:        "Verification submitted successfully",
        VerificationId: fmt.Sprintf("%d", verification.ID),
    }, nil
}

// verifyContract 验证合约（异步执行）
func (s *ContractService) verifyContract(ctx context.Context, verification *models.ContractVerification) {
    // 1. 从链上获取合约字节码
    runtimeCode, err := s.getContractBytecode(ctx, verification.ContractAddress)
    if err != nil {
        s.updateVerificationStatus(ctx, verification.ID, "failed", err.Error())
        return
    }

    // 2. 编译源代码
    compiledCode, abi, err := s.compileSourceCode(ctx, verification)
    if err != nil {
        s.updateVerificationStatus(ctx, verification.ID, "failed", err.Error())
        return
    }

    // 3. 比较字节码
    if compiledCode != runtimeCode {
        s.updateVerificationStatus(ctx, verification.ID, "failed", "Bytecode mismatch")
        return
    }

    // 4. 保存ABI
    abiKey := fmt.Sprintf("contracts/%s/abi/%d.json", verification.ContractAddress, time.Now().Unix())
    abiBytes, _ := json.Marshal(abi)
    if err := s.storage.Put(ctx, abiKey, abiBytes); err != nil {
        global.Logger.Warnf("failed to store ABI: %v", err)
    } else {
        verification.ABIStorageKey = abiKey
        verification.ABI = string(abiBytes)
    }

    // 5. 更新验证状态
    now := time.Now()
    verification.VerificationStatus = "verified"
    verification.VerifiedAt = &now
    db.Get().WithContext(ctx).Save(verification)

    // 6. 更新合约信息
    s.updateContractInfo(ctx, verification)
}

// AddContractTag 添加合约标签
func (s *ContractService) AddContractTag(ctx context.Context, req *pb.AddContractTagRequest) (*pb.AddContractTagResponse, error) {
    if req == nil || req.Address == "" || req.TagType == "" {
        return nil, errors.New("address and tag_type are required")
    }

    tag := &models.ContractTag{
        ContractAddress: req.Address,
        TagType:         req.TagType,
        TagValue:        req.TagValue,
        TagColor:        req.TagColor,
        Description:     req.Description,
        CreatedBy:       req.CreatedBy,
    }

    // 使用 FirstOrCreate 避免重复
    err := db.Get().WithContext(ctx).
        Where("contract_address = ? AND tag_type = ?", req.Address, req.TagType).
        FirstOrCreate(tag).Error

    if err != nil {
        return nil, errors.Wrap(err, "failed to add contract tag")
    }

    return &pb.AddContractTagResponse{
        Success: true,
        Message: "Tag added successfully",
    }, nil
}

// RemoveContractTag 删除合约标签
func (s *ContractService) RemoveContractTag(ctx context.Context, req *pb.RemoveContractTagRequest) (*pb.RemoveContractTagResponse, error) {
    if req == nil || req.Address == "" || req.TagType == "" {
        return nil, errors.New("address and tag_type are required")
    }

    err := db.Get().WithContext(ctx).
        Where("contract_address = ? AND tag_type = ?", req.Address, req.TagType).
        Delete(&models.ContractTag{}).Error

    if err != nil {
        return nil, errors.Wrap(err, "failed to remove contract tag")
    }

    return &pb.RemoveContractTagResponse{
        Success: true,
        Message: "Tag removed successfully",
    }, nil
}

// 辅助方法
func (s *ContractService) contractToProto(contract *models.Contract) *pb.ContractInfo {
    tags := make([]*pb.ContractTag, 0, len(contract.Tags))
    for _, tag := range contract.Tags {
        tags = append(tags, &pb.ContractTag{
            TagType:     tag.TagType,
            TagValue:    tag.TagValue,
            TagColor:    tag.TagColor,
            Description: tag.Description,
            CreatedAt:   tag.CreatedAt.Unix(),
        })
    }

    var verifiedAt int64
    if contract.VerifiedAt != nil {
        verifiedAt = contract.VerifiedAt.Unix()
    }

    return &pb.ContractInfo{
        Address:            contract.Address,
        Name:               contract.Name,
        IsVerified:         contract.IsVerified,
        VerificationStatus: contract.VerificationStatus,
        CompilerVersion:    contract.CompilerVersion,
        OptimizationEnabled: contract.OptimizationEnabled,
        OptimizationRuns:   int32(contract.OptimizationRuns),
        EvmVersion:         contract.EVMVersion,
        License:            contract.License,
        CodeHash:           contract.CodeHash,
        VerifiedAt:         verifiedAt,
        VerifiedBy:         contract.VerifiedBy,
        Tags:               tags,
    }
}
```

## 6. 合约验证流程

### 6.1 验证步骤

1. **接收验证请求**
   - 验证输入参数
   - 创建验证记录，状态为 `pending`

2. **存储源代码**
   - 将源代码存储到对象存储（MinIO/S3）
   - 记录存储key

3. **编译源代码**
   - 调用Solidity编译器（或使用Docker容器）
   - 生成字节码和ABI

4. **获取链上字节码**
   - 从区块链节点获取合约的运行时字节码

5. **比较字节码**
   - 比较编译后的字节码与链上字节码
   - 支持完全匹配和部分匹配

6. **保存验证结果**
   - 如果匹配成功，更新合约信息
   - 保存ABI到对象存储
   - 更新验证状态为 `verified`

### 6.2 编译器集成

可以使用以下方式集成Solidity编译器：

1. **Docker容器方式**（推荐）
   ```go
   // 使用Docker运行solc编译器
   docker run --rm -v $(pwd):/sources ethereum/solc:0.8.17 --abi --bin source.sol
   ```

2. **HTTP API方式**
   - 使用第三方编译服务（如Sourcify API）
   - 或自建编译服务

3. **本地二进制方式**
   - 直接在服务器上安装solc
   - 通过命令行调用

## 7. 标签系统设计

### 7.1 预定义标签类型

- `verified`: 已验证合约
- `official`: 官方合约
- `popular`: 热门合约
- `scam`: 诈骗合约
- `verified_audit`: 已审计合约
- `proxy`: 代理合约
- `multisig`: 多签合约

### 7.2 标签管理权限

- 系统标签：由管理员设置（如 `verified`, `official`）
- 用户标签：由用户设置（需要权限验证）

## 8. 缓存策略

### 8.1 Redis缓存

```go
// 缓存键设计
const (
    CacheKeyContractInfo = "contract:info:%s"           // 合约信息
    CacheKeyContractABI = "contract:abi:%s"             // 合约ABI
    CacheKeyContractTags = "contract:tags:%s"           // 合约标签
    CacheKeyVerificationStatus = "contract:verify:%s"   // 验证状态
)

// 缓存过期时间
const (
    CacheExpireContractInfo = 1 * time.Hour
    CacheExpireContractABI = 24 * time.Hour
    CacheExpireContractTags = 1 * time.Hour
)
```

## 9. 对象存储设计

### 9.1 存储路径结构

```
contracts/
  {address}/
    source-code/
      {timestamp}.json          # 源代码（JSON格式）
    abi/
      {timestamp}.json          # ABI（JSON格式）
    source-files/
      {file_path}               # 源代码文件
```

### 9.2 存储内容格式

**源代码JSON格式**（参考Etherscan）：
```json
{
  "language": "Solidity",
  "sources": {
    "Contract.sol": {
      "content": "pragma solidity ^0.8.0; ..."
    }
  },
  "settings": {
    "compilerVersion": "0.8.17",
    "optimization": {
      "enabled": true,
      "runs": 200
    },
    "evmVersion": "london"
  }
}
```

## 10. 安全考虑

1. **输入验证**
   - 验证合约地址格式
   - 限制源代码大小
   - 防止代码注入

2. **权限控制**
   - 标签管理需要权限验证
   - 验证提交需要防刷机制

3. **资源限制**
   - 限制编译时间
   - 限制并发验证数量
   - 限制存储空间

## 11. 性能优化

1. **异步验证**
   - 验证过程异步执行
   - 使用消息队列处理大量验证请求

2. **缓存策略**
   - 合约信息缓存1小时
   - ABI缓存24小时
   - 标签信息缓存1小时

3. **数据库优化**
   - 合约地址建立唯一索引
   - 验证状态建立索引
   - 使用读写分离

## 12. 部署建议

1. **数据库迁移**
   - 使用GORM AutoMigrate创建表结构
   - 或使用数据库迁移工具

2. **初始化数据**
   - 预定义标签类型
   - 设置系统配置

3. **监控告警**
   - 监控验证成功率
   - 监控存储使用量
   - 监控API响应时间

## 13. 后续扩展

1. **多链支持**
   - 支持不同链的合约验证
   - 链标识字段

2. **验证历史**
   - 保存所有验证尝试记录
   - 支持回滚到历史版本

3. **自动验证**
   - 监听新部署的合约
   - 自动尝试验证

4. **社区功能**
   - 用户评论
   - 合约评分
   - 使用统计
