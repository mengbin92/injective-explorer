# 合约服务实现总结

## 已完成的工作

### 1. Protobuf 代码生成 ✅
- 已运行 `make api` 生成 Protobuf 代码
- 生成的文件：
  - `api/explorer/v1/contract.pb.go`
  - `api/explorer/v1/contract_grpc.pb.go`
  - `api/explorer/v1/contract_http.pb.go`

### 2. 合约服务实现 ✅
- 文件：`internal/service/contract.go`
- 实现的方法：
  - `GetContractInfo`: 获取合约基本信息
  - `GetContractSourceCode`: 获取合约源代码（已验证）
  - `GetContractSchema`: 获取合约 Schema
  - `GetContractCode`: 获取合约 WASM 代码
  - `SubmitContractVerification`: 提交合约验证
  - `GetVerificationStatus`: 获取验证状态
  - `GetContractTags`: 获取合约标签
  - `AddContractTag`: 添加合约标签
  - `RemoveContractTag`: 删除合约标签

### 3. 服务注册 ✅
- gRPC 服务器：`internal/server/grpc.go`
- HTTP 服务器：`internal/server/http.go`
- 已注册 Contract 服务到 gRPC 和 HTTP 服务器

### 4. 数据库迁移 ✅
- 文件：`internal/global/global.go`
- 已添加合约相关模型的 AutoMigrate：
  - `models.Contract`
  - `models.ContractTag`
  - `models.ContractVerification`
  - `models.ContractSourceFile`

### 5. 合约同步功能 ✅
- 文件：`internal/service/sync.go`
- 已实现：
  - `syncContractFromAccount`: 从账户同步合约
  - `extractContractAddressesFromTransaction`: 从交易提取合约地址
  - `syncContractsFromTransaction`: 从交易同步合约
- 已集成到账户同步流程

## API 端点

### HTTP 端点
- `GET /api/v1/contract/info?address={address}` - 获取合约信息
- `GET /api/v1/contract/source-code?address={address}` - 获取源代码
- `GET /api/v1/contract/schema?address={address}` - 获取 Schema
- `GET /api/v1/contract/code?address={address}` - 获取 WASM 代码
- `POST /api/v1/contract/verify` - 提交验证
- `GET /api/v1/contract/verification-status?address={address}` - 获取验证状态
- `GET /api/v1/contract/tags?address={address}` - 获取标签
- `POST /api/v1/contract/tag` - 添加标签
- `DELETE /api/v1/contract/tag?address={address}&tag_type={type}` - 删除标签

### gRPC 端点
- `Contract.GetContractInfo`
- `Contract.GetContractSourceCode`
- `Contract.GetContractSchema`
- `Contract.GetContractCode`
- `Contract.SubmitContractVerification`
- `Contract.GetVerificationStatus`
- `Contract.GetContractTags`
- `Contract.AddContractTag`
- `Contract.RemoveContractTag`

## 功能特性

### 1. 合约信息查询
- 从数据库查询合约信息
- 如果数据库中没有，从链上获取
- 支持缓存（Redis）

### 2. 合约验证
- 提交源代码进行验证
- 异步验证流程
- 保存验证记录
- 存储源代码到对象存储

### 3. 标签管理
- 支持多种标签类型
- 添加/删除标签
- 标签缓存

### 4. 自动同步
- 在账户同步时自动识别合约
- 从交易事件中提取合约地址
- 自动创建合约记录

## 缓存策略

- **合约信息**: 缓存 1 小时
- **合约标签**: 缓存 1 小时
- **合约 Schema**: 缓存 24 小时

## 待实现功能

### 1. 合约验证逻辑
- `verifyContract` 方法目前是占位实现
- 需要实现：
  - 从链上获取 WASM 代码
  - Rust 源代码编译
  - WASM 代码比较
  - Schema 生成

### 2. CodeID 获取
- 目前 CodeID 默认为 0
- 需要实现从链上获取 CodeID 的逻辑

### 3. WASM 代码获取
- 需要实现从链上获取合约 WASM 代码的方法
- 可能需要使用 CosmWasm 查询接口

## 测试建议

1. **单元测试**
   - 测试合约信息查询
   - 测试标签管理
   - 测试缓存逻辑

2. **集成测试**
   - 测试从链上获取合约信息
   - 测试合约同步流程
   - 测试验证提交流程

3. **端到端测试**
   - 测试完整的合约查看流程
   - 测试标签添加和删除
   - 测试验证状态查询

## 使用示例

### 获取合约信息
```bash
curl "http://localhost:8000/api/v1/contract/info?address=inj1..."
```

### 添加标签
```bash
curl -X POST "http://localhost:8000/api/v1/contract/tag" \
  -H "Content-Type: application/json" \
  -d '{
    "address": "inj1...",
    "tag_type": "official",
    "tag_value": "Official Contract",
    "tag_color": "green",
    "description": "This is an official contract"
  }'
```

### 提交验证
```bash
curl -X POST "http://localhost:8000/api/v1/contract/verify" \
  -H "Content-Type: application/json" \
  -d '{
    "address": "inj1...",
    "code_id": 123,
    "compiler_version": "1.70.0",
    "cargo_version": "1.70.0",
    "optimization_enabled": true,
    "license": "MIT",
    "source_code": "{...}",
    "verification_type": "full_match"
  }'
```

## 注意事项

1. **验证功能**: 合约验证逻辑需要后续实现，目前只是占位
2. **WASM 代码**: 需要实现从链上获取 WASM 代码的方法
3. **CodeID**: 需要实现从链上获取 CodeID 的逻辑
4. **错误处理**: 所有操作都有适当的错误处理和日志记录
5. **性能**: 使用了缓存和异步处理来优化性能
