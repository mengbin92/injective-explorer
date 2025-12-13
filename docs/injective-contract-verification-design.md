# Injective 智能合约查看功能技术方案

## 1. 概述

本文档描述了针对 Injective 链的智能合约查看功能后端技术方案。Injective 链主要支持 CosmWasm 智能合约（WebAssembly），需要实现类似 Etherscan 的合约查看、验证和标签管理功能。

参考实现：
- [Etherscan Contract View](https://etherscan.io/address/0x000000000022d473030f116ddee9f6b43ac78ba3#code)
- [Blockscout Contract View](https://eth.blockscout.com/address/0x000000000022D473030F116dDEE9F6B43aC78BA3?tab=index)

## 2. Injective 链合约特性

### 2.1 合约类型
- **CosmWasm 合约**: 主要合约类型，使用 WebAssembly (WASM) 字节码
- **合约地址**: Cosmos 格式 `inj1...` (42字符，Bech32编码)
- **源代码语言**: 主要是 Rust
- **编译输出**: WebAssembly (WASM) 字节码

### 2.2 合约识别
- 通过 `Account.CodeHash` 字段判断是否为合约账户
- `CodeHash` 为空表示普通账户
- `CodeHash` 有值表示合约账户（CosmWasm 合约）

### 2.3 合约信息获取
- 通过 Injective SDK 查询合约信息
- 获取合约代码（WASM 字节码）
- 获取合约状态和存储
- 查询合约相关交易

## 3. 数据库设计

### 3.1 合约信息表 (contracts)

```sql
CREATE TABLE contracts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    address VARCHAR(128) NOT NULL UNIQUE COMMENT '合约地址(inj1格式)',
    name VARCHAR(255) COMMENT '合约名称',
    contract_type VARCHAR(50) DEFAULT 'cosmwasm' COMMENT '合约类型: cosmwasm',
    chain_id VARCHAR(50) DEFAULT 'injective-1' COMMENT '链ID',
    is_verified BOOLEAN DEFAULT FALSE COMMENT '是否已验证',
    verification_status VARCHAR(50) DEFAULT 'unverified' COMMENT '验证状态: unverified, pending, verified, failed',
    code_id BIGINT COMMENT '代码ID(用于CosmWasm)',
    code_hash VARCHAR(128) COMMENT '代码哈希',
    wasm_code TEXT COMMENT 'WASM字节码(hex格式)',
    wasm_code_storage_key VARCHAR(512) COMMENT 'WASM代码在对象存储中的key',
    source_code_storage_key VARCHAR(512) COMMENT '源代码在对象存储中的key',
    schema_storage_key VARCHAR(512) COMMENT 'Schema在对象存储中的key',
    compiler_version VARCHAR(100) COMMENT '编译器版本(如rustc版本)',
    cargo_version VARCHAR(100) COMMENT 'Cargo版本',
    optimization_enabled BOOLEAN DEFAULT FALSE COMMENT '是否启用优化',
    license VARCHAR(100) COMMENT '许可证类型',
    verified_at TIMESTAMP NULL COMMENT '验证时间',
    verified_by VARCHAR(128) COMMENT '验证者地址',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    INDEX idx_address (address),
    INDEX idx_code_id (code_id),
    INDEX idx_verification_status (verification_status),
    INDEX idx_is_verified (is_verified),
    INDEX idx_chain_id (chain_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='智能合约信息表';
```

### 3.2 合约标签表 (contract_tags)

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

### 3.3 合约验证记录表 (contract_verifications)

```sql
CREATE TABLE contract_verifications (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    contract_address VARCHAR(128) NOT NULL COMMENT '合约地址',
    verification_type VARCHAR(50) NOT NULL COMMENT '验证类型: full_match',
    code_id BIGINT COMMENT '代码ID',
    compiler_version VARCHAR(100) NOT NULL COMMENT '编译器版本',
    cargo_version VARCHAR(100) COMMENT 'Cargo版本',
    optimization_enabled BOOLEAN DEFAULT FALSE COMMENT '是否启用优化',
    license VARCHAR(100) COMMENT '许可证类型',
    source_code TEXT COMMENT '源代码(JSON格式，包含所有源文件)',
    source_code_storage_key VARCHAR(512) COMMENT '源代码在对象存储中的key',
    schema JSON COMMENT 'Schema(JSON格式)',
    schema_storage_key VARCHAR(512) COMMENT 'Schema在对象存储中的key',
    wasm_code TEXT COMMENT '编译后的WASM代码(hex格式)',
    wasm_code_storage_key VARCHAR(512) COMMENT 'WASM代码在对象存储中的key',
    verification_status VARCHAR(50) DEFAULT 'pending' COMMENT '验证状态: pending, verified, failed',
    error_message TEXT COMMENT '错误信息(如果验证失败)',
    verified_at TIMESTAMP NULL COMMENT '验证时间',
    verified_by VARCHAR(128) COMMENT '验证者地址',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_contract_address (contract_address),
    INDEX idx_code_id (code_id),
    INDEX idx_verification_status (verification_status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合约验证记录表';
```

### 3.4 合约源代码文件表 (contract_source_files)

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

## 4. 数据模型设计

### 4.1 Contract 模型更新

```go
// internal/models/contract.go
type Contract struct {
    gorm.Model
    
    Address              string     `json:"address" gorm:"column:address;uniqueIndex;size:128"`
    Name                 string     `json:"name" gorm:"column:name;size:255"`
    ContractType         string     `json:"contract_type" gorm:"column:contract_type;size:50;default:cosmwasm"`
    ChainID              string     `json:"chain_id" gorm:"column:chain_id;size:50;default:injective-1;index"`
    IsVerified           bool       `json:"is_verified" gorm:"column:is_verified;default:false"`
    VerificationStatus   string     `json:"verification_status" gorm:"column:verification_status;default:unverified;size:50;index"`
    CodeID               uint64     `json:"code_id" gorm:"column:code_id;index"`
    CodeHash             string     `json:"code_hash" gorm:"column:code_hash;size:128"`
    WASMCode             string     `json:"wasm_code" gorm:"column:wasm_code;type:text"`
    WASMCodeStorageKey   string     `json:"wasm_code_storage_key" gorm:"column:wasm_code_storage_key;size:512"`
    SourceCodeStorageKey string     `json:"source_code_storage_key" gorm:"column:source_code_storage_key;size:512"`
    SchemaStorageKey     string     `json:"schema_storage_key" gorm:"column:schema_storage_key;size:512"`
    CompilerVersion      string     `json:"compiler_version" gorm:"column:compiler_version;size:100"`
    CargoVersion         string     `json:"cargo_version" gorm:"column:cargo_version;size:100"`
    OptimizationEnabled  bool       `json:"optimization_enabled" gorm:"column:optimization_enabled;default:false"`
    License              string     `json:"license" gorm:"column:license;size:100"`
    VerifiedAt           *time.Time `json:"verified_at" gorm:"column:verified_at"`
    VerifiedBy           string     `json:"verified_by" gorm:"column:verified_by;size:128"`
    
    // 关联关系
    Tags          []ContractTag          `json:"tags" gorm:"foreignKey:ContractAddress;references:Address"`
    Verifications []ContractVerification `json:"verifications" gorm:"foreignKey:ContractAddress;references:Address"`
    SourceFiles   []ContractSourceFile   `json:"source_files" gorm:"foreignKey:ContractAddress;references:Address"`
}
```

## 5. API 接口设计

### 5.1 Protobuf 定义（已存在，需要调整）

主要接口：
- `GetContractInfo`: 获取合约基本信息
- `GetContractCode`: 获取合约 WASM 代码
- `GetContractSourceCode`: 获取合约源代码（如果已验证）
- `GetContractSchema`: 获取合约 Schema（消息定义）
- `SubmitContractVerification`: 提交合约验证
- `GetVerificationStatus`: 获取验证状态
- `GetContractTags`: 获取合约标签
- `AddContractTag`: 添加标签
- `RemoveContractTag`: 删除标签

## 6. 服务层实现要点

### 6.1 合约识别

```go
// isContract 检查地址是否为合约
func (s *ContractService) isContract(ctx context.Context, address string) (bool, error) {
    account, err := s.chainClient.GetAccount(ctx, address)
    if err != nil {
        return false, errors.Wrap(err, "failed to get account")
    }
    
    // 解析账户信息
    ethAccount := &types.EthAccount{}
    err = s.chainClient.ClientContext().Codec.Unmarshal(account.Account.Value, ethAccount)
    if err != nil {
        return false, errors.Wrap(err, "failed to unmarshal account")
    }
    
    // CodeHash 不为空表示是合约
    return len(ethAccount.CodeHash) > 0, nil
}
```

### 6.2 获取合约代码

```go
// getContractCode 从链上获取合约 WASM 代码
func (s *ContractService) getContractCode(ctx context.Context, address string) ([]byte, error) {
    // 方式1: 通过 CosmWasm 查询接口获取
    // 需要查看 Injective SDK 是否提供直接获取合约代码的方法
    
    // 方式2: 通过账户信息获取 CodeHash，然后查询代码
    account, err := s.chainClient.GetAccount(ctx, address)
    if err != nil {
        return nil, errors.Wrap(err, "failed to get account")
    }
    
    ethAccount := &types.EthAccount{}
    err = s.chainClient.ClientContext().Codec.Unmarshal(account.Account.Value, ethAccount)
    if err != nil {
        return nil, errors.Wrap(err, "failed to unmarshal account")
    }
    
    // 通过 CodeHash 或 CodeID 查询 WASM 代码
    // 需要查看 Injective SDK 的 CosmWasm 查询方法
    // 可能需要使用 wasm 模块的查询接口
    
    return nil, errors.New("not implemented: need to check SDK for WASM code query")
}
```

### 6.3 合约验证流程

1. **接收验证请求**
   - 验证地址格式（必须是 inj1 格式）
   - 验证源代码格式（Rust 项目结构）

2. **检查合约是否存在**
   - 通过 `GetAccount` 获取账户信息
   - 检查 `CodeHash` 判断是否为合约

3. **获取链上 WASM 代码**
   - 从链上获取合约的 WASM 字节码
   - 计算 WASM 代码哈希

4. **编译源代码**
   - 使用 Rust 编译器和 CosmWasm 工具链
   - 生成 WASM 字节码
   - 生成 Schema（消息定义）

5. **比较 WASM 代码**
   - 比较编译后的 WASM 与链上 WASM
   - 支持完全匹配验证

6. **保存验证结果**
   - 更新合约信息
   - 保存源代码和 Schema 到对象存储

### 6.4 Rust 编译集成

```go
// compileCosmWasmContract 编译 CosmWasm 合约
func (s *ContractService) compileCosmWasmContract(ctx context.Context, sourceCode string) (wasmCode []byte, schema map[string]interface{}, err error) {
    // 方式1: 使用 Docker 容器编译
    // docker run --rm -v $(pwd):/code cosmwasm/workspace-optimizer:0.14.0
    
    // 方式2: 使用本地 Rust 工具链
    // cargo wasm
    // cosmwasm-opt
    
    // 方式3: 使用第三方编译服务 API
    
    return nil, nil, errors.New("not implemented: need Rust compilation setup")
}
```

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

- 系统标签：由管理员设置
- 用户标签：由用户设置（需要权限验证）

## 8. 缓存策略

```go
const (
    CacheKeyContractInfo = "contract:info:%s"
    CacheKeyContractCode = "contract:code:%s"
    CacheKeyContractTags = "contract:tags:%s"
    CacheKeyVerificationStatus = "contract:verify:%s"
)

const (
    CacheExpireContractInfo = 1 * time.Hour
    CacheExpireContractCode = 24 * time.Hour
    CacheExpireContractTags = 1 * time.Hour
)
```

## 9. 对象存储设计

### 9.1 存储路径结构

```
contracts/
  {address}/
    wasm-code/
      {timestamp}.wasm          # WASM 字节码
    source-code/
      {timestamp}.tar.gz        # 源代码压缩包
    schema/
      {timestamp}.json          # Schema 定义
    source-files/
      {file_path}               # 源代码文件
```

## 10. 实现注意事项

1. **CosmWasm 查询接口**: 需要查看 Injective SDK 是否提供 CosmWasm 合约查询方法
2. **WASM 代码获取**: 可能需要通过链上查询或 RPC 调用获取
3. **Rust 编译环境**: 需要设置 Rust 编译环境或使用 Docker 容器
4. **Schema 生成**: CosmWasm 合约需要生成 Schema 文件用于消息定义
5. **代码优化**: CosmWasm 合约通常需要优化以减少大小

## 11. 后续扩展

1. **合约交互**: 支持查询合约状态和执行合约方法
2. **合约事件**: 查询合约相关事件和日志
3. **合约分析**: 合约代码分析和安全审计
4. **多版本支持**: 支持合约升级和版本管理
