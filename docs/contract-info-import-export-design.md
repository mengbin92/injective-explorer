# 合约信息导入导出功能技术方案

## 一、需求分析

### 1.1 功能概述
为合约信息管理服务新增导入导出功能，支持CSV和JSON两种格式，方便用户批量管理和迁移合约数据。

### 1.2 业务场景
- **导出场景**：用户需要批量导出合约信息用于备份、分析或迁移
- **导入场景**：用户需要批量导入合约信息，支持从其他系统迁移数据或批量创建合约

### 1.3 功能要求
- 支持CSV和JSON两种格式
- 导出支持过滤条件（链ID、合约类型、验证状态等）
- 导入支持批量创建和更新
- 导入时支持数据校验和错误报告
- 支持标签信息的导入导出

## 二、接口设计

### 2.1 Proto定义

在 `api/explorer/v1/contract_info.proto` 中新增以下接口：

```protobuf
service ContractInfoService {
  // ... 现有接口 ...

  // ExportContractInfos exports contract information in CSV or JSON format
  // Returns Reply with data containing ExportContractInfosResponseData
  rpc ExportContractInfos(ExportContractInfosRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/contract/info/export"
    };
  }

  // ImportContractInfos imports contract information from CSV or JSON file
  // Returns Reply with data containing ImportContractInfosResponseData
  rpc ImportContractInfos(ImportContractInfosRequest) returns (Reply) {
    option (google.api.http) = {
      post: "/api/v1/contract/info/import"
      body: "*"
    };
  }
}

// 导出请求
message ExportContractInfosRequest {
  // Export format: "csv" or "json" (required)
  string format = 1;
  
  // Filter conditions (optional)
  string chain_id = 2;              // Filter by chain ID
  string contract_type = 3;         // Filter by contract type
  string verification_status = 4;   // Filter by verification status
  string name = 5;                  // Fuzzy match on name or token_name
  string symbol = 6;                // Fuzzy match on token_symbol
  
  // Specific contract addresses to export (optional, if empty exports all matching filters)
  repeated string contract_addresses = 7;
  
  // Include tags in export (default: true)
  bool include_tags = 8;
  
  // Include metadata in export (default: true)
  bool include_metadata = 9;
}

// 导入请求
message ImportContractInfosRequest {
  // Import format: "csv" or "json" (required)
  string format = 1;
  
  // File content as base64 encoded string
  string file_content_base64 = 2;
  
  // Import mode: "create" (only create new), "update" (only update existing), "upsert" (create or update)
  // Default: "upsert"
  string import_mode = 3;
  
  // Skip validation errors and continue importing (default: false)
  bool skip_errors = 4;
  
  // Default chain_id for contracts without chain_id specified (optional)
  string default_chain_id = 5;
  
  // Default contract_type for contracts without contract_type specified (optional)
  string default_contract_type = 6;
}

// 导出响应数据（放在Reply.data中）
// 注意：这些message仅用于文档说明，实际响应通过Reply.data返回
message ExportContractInfosResponseData {
  // File content as base64 encoded string (for small files)
  // Or download URL for large files
  oneof result {
    string file_content_base64 = 1;
    string download_url = 2;
  }
  
  // File format: "csv" or "json"
  string format = 3;
  
  // Total number of contracts exported
  int32 total_count = 4 [ json_name = "total_count" ];
  
  // File size in bytes
  int64 file_size = 5 [ json_name = "file_size" ];
  
  // Export timestamp
  string exported_at = 6 [ json_name = "exported_at" ];
}

// 导入响应数据（放在Reply.data中）
// 注意：这些message仅用于文档说明，实际响应通过Reply.data返回
message ImportContractInfosResponseData {
  // Total number of records processed
  int32 total_count = 1 [ json_name = "total_count" ];
  
  // Number of successfully imported contracts
  int32 success_count = 2 [ json_name = "success_count" ];
  
  // Number of failed imports
  int32 failed_count = 3 [ json_name = "failed_count" ];
  
  // Number of contracts created
  int32 created_count = 4 [ json_name = "created_count" ];
  
  // Number of contracts updated
  int32 updated_count = 5 [ json_name = "updated_count" ];
  
  // Import errors (if any)
  repeated ImportError errors = 6;
  
  // Import timestamp
  string imported_at = 7 [ json_name = "imported_at" ];
}

// 导入错误信息
message ImportError {
  // Row number (for CSV) or index (for JSON array)
  int32 row_number = 1 [ json_name = "row_number" ];
  
  // Contract address (if available)
  string contract_address = 2 [ json_name = "contract_address" ];
  
  // Error message
  string error_message = 3 [ json_name = "error_message" ];
  
  // Error code
  string error_code = 4 [ json_name = "error_code" ];
}
```

### 2.2 响应格式说明

接口返回标准的 `Reply` 类型，响应数据放在 `Reply.data` 字段中：

- **成功响应**：
  ```json
  {
    "code": 200,
    "message": "success",
    "data": {
      // ExportContractInfosResponseData 或 ImportContractInfosResponseData 的内容
    }
  }
  ```

- **错误响应**：
  ```json
  {
    "code": 400,
    "message": "error message",
    "data": null
  }
  ```

**重要说明**：
- `ExportContractInfosResponseData` 和 `ImportContractInfosResponseData` 这些 message 定义仅用于文档说明响应数据结构
- 在实际 proto 文件中，**不需要定义这些 ResponseData message**，因为接口直接返回 `Reply` 类型
- 响应数据通过 Go 结构体定义，使用 `MarshalAndReply` 函数自动转换为 `google.protobuf.Struct` 并放入 `Reply.data` 字段
- `ImportError` message 可以在 proto 中定义（如果需要在多个地方复用），也可以直接作为 Go 结构体使用

## 三、数据结构设计

### 3.1 CSV格式设计

#### 3.1.1 CSV列定义

| 列名                | 说明                 | 必填 | 示例                                         |
| ------------------- | -------------------- | ---- | -------------------------------------------- |
| contract_address    | 合约地址             | 是   | 0x1234...abcd                                |
| chain_id            | 链ID                 | 是   | injective-1                                  |
| name                | 合约名称             | 否   | My Contract                                  |
| token_name          | Token名称            | 否   | My Token                                     |
| token_symbol        | Token符号            | 否   | MTK                                          |
| contract_type       | 合约类型             | 否   | cosmwasm                                     |
| verification_status | 验证状态             | 否   | verified                                     |
| is_verified         | 是否已验证           | 否   | true                                         |
| verified_at         | 验证时间             | 否   | 2024-01-01T00:00:00Z                         |
| verified_by         | 验证者               | 否   | admin                                        |
| metadata            | 元数据(JSON字符串)   | 否   | {"key":"value"}                              |
| tags                | 标签(JSON数组字符串) | 否   | [{"tag_type":"category","tag_value":"defi"}] |

#### 3.1.2 CSV示例

```csv
contract_address,chain_id,name,token_name,token_symbol,contract_type,verification_status,is_verified,metadata,tags
0x1234...abcd,injective-1,My Contract,My Token,MTK,cosmwasm,verified,true,"{""key"":""value""}","[{""tag_type"":""category"",""tag_value"":""defi""}]"
```

### 3.2 JSON格式设计

#### 3.2.1 JSON结构

```json
{
  "version": "1.0",
  "exported_at": "2024-01-01T00:00:00Z",
  "total_count": 2,
  "contracts": [
    {
      "contract_address": "0x1234...abcd",
      "chain_id": "injective-1",
      "name": "My Contract",
      "token_name": "My Token",
      "token_symbol": "MTK",
      "contract_type": "cosmwasm",
      "verification_status": "verified",
      "is_verified": true,
      "verified_at": "2024-01-01T00:00:00Z",
      "verified_by": "admin",
      "metadata": {
        "key": "value"
      },
      "tags": [
        {
          "tag_type": "category",
          "tag_value": "defi",
          "tag_color": "#FF0000",
          "description": "DeFi category",
          "created_by": "admin"
        }
      ],
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

## 四、实现方案

### 4.1 导出功能实现

#### 4.1.1 导出流程

```
1. 接收导出请求，解析过滤条件
2. 构建数据库查询（复用ListContractInfos的查询逻辑）
3. 查询合约信息（包含Tags关联）
4. 根据format参数选择CSV或JSON格式转换
5. 生成文件内容
6. 如果文件较小（<10MB），直接返回base64编码
7. 如果文件较大，上传到对象存储，返回下载URL
8. 返回响应
```

#### 4.1.2 CSV导出实现要点

- 使用标准库 `encoding/csv` 进行CSV编码
- 处理特殊字符转义（引号、换行符等）
- Metadata和Tags字段序列化为JSON字符串
- 使用UTF-8 BOM确保Excel正确识别中文

#### 4.1.3 JSON导出实现要点

- 使用 `encoding/json` 或 `github.com/bytedance/sonic` 进行JSON编码
- 格式化输出，便于阅读
- 包含版本信息和导出时间戳

### 4.2 导入功能实现

#### 4.2.1 导入流程

```
1. 接收导入请求，解析文件格式和内容
2. Base64解码文件内容
3. 根据format参数选择CSV或JSON解析器
4. 解析文件，转换为ContractInfo结构
5. 数据校验：
   - 必填字段检查
   - 地址格式验证
   - 数据类型验证
6. 根据import_mode执行操作：
   - create: 仅创建新记录，已存在则报错
   - update: 仅更新已存在记录，不存在则报错
   - upsert: 创建或更新
7. 批量处理（使用事务）
8. 收集错误信息
9. 返回导入结果
```

#### 4.2.2 CSV导入实现要点

- 使用 `encoding/csv` 解析CSV
- 处理CSV头部行，支持列顺序灵活
- 解析JSON字符串字段（metadata、tags）
- 行号记录用于错误定位

#### 4.2.3 JSON导入实现要点

- 解析JSON数组或对象结构
- 支持版本兼容性检查
- 索引记录用于错误定位

### 4.3 数据校验规则

#### 4.3.1 必填字段
- contract_address: 必填，不能为空
- chain_id: 必填，如果未提供则使用default_chain_id或默认值

#### 4.3.2 格式校验
- contract_address: 地址格式验证（支持0x和inj1格式）
- chain_id: 长度限制（最大50字符）
- contract_type: 枚举值验证（cosmwasm, evm等）
- verification_status: 枚举值验证（verified, unverified等）

#### 4.3.3 业务规则
- 同一chain_id + contract_address组合唯一性检查
- Metadata必须是有效的JSON对象
- Tags必须是有效的JSON数组

### 4.4 批量处理优化

#### 4.4.1 批量插入/更新
- 使用GORM的批量操作（Batch Create/Update）
- 每批处理100-500条记录，避免单次事务过大
- 使用事务确保数据一致性

#### 4.4.2 并发控制
- 导入操作使用互斥锁，避免并发导入冲突
- 导出操作可以并发，但需要控制数据库查询压力

## 五、错误处理

### 5.1 导出错误
- 查询失败：返回500错误，记录日志
- 格式不支持：返回400错误
- 文件过大：返回413错误或上传到对象存储

### 5.2 导入错误
- 文件格式错误：返回400错误，提示格式问题
- 数据校验失败：记录错误信息，根据skip_errors决定是否继续
- 数据库操作失败：回滚事务，返回错误详情

### 5.3 错误响应格式

所有错误响应都使用标准的 `Reply` 格式：

#### 5.3.1 请求错误（如参数验证失败）
```json
{
  "code": 400,
  "message": "contract_address is required",
  "data": null
}
```

#### 5.3.2 导入部分失败（部分成功部分失败）
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "total_count": 10,
    "success_count": 7,
    "failed_count": 3,
    "created_count": 5,
    "updated_count": 2,
    "errors": [
      {
        "row_number": 5,
        "contract_address": "0x1234...abcd",
        "error_message": "contract_address is required",
        "error_code": "MISSING_REQUIRED_FIELD"
      },
      {
        "row_number": 8,
        "contract_address": "0x5678...efgh",
        "error_message": "invalid chain_id",
        "error_code": "INVALID_CHAIN_ID"
      }
    ],
    "imported_at": "2024-01-01T00:00:00Z"
  }
}
```

#### 5.3.3 导入完全失败（所有记录都失败）
```json
{
  "code": 400,
  "message": "Import failed: all records failed validation",
  "data": {
    "total_count": 10,
    "success_count": 0,
    "failed_count": 10,
    "created_count": 0,
    "updated_count": 0,
    "errors": [
      // ... 所有错误信息
    ],
    "imported_at": "2024-01-01T00:00:00Z"
  }
}
```

注意：即使有部分失败，只要 `success_count > 0`，接口仍然返回 `code=200`，错误详情在 `data.errors` 中。只有当所有记录都失败时，才返回错误状态码。

## 六、性能考虑

### 6.1 导出性能
- 大数据量导出使用流式处理，避免内存溢出
- 超过10MB的文件使用对象存储，返回下载URL
- 添加导出任务队列，支持异步导出

### 6.2 导入性能
- 批量处理，减少数据库交互次数
- 使用事务批量提交，提高写入性能
- 限制单次导入最大记录数（建议10000条）

### 6.3 缓存处理
- 导入后清除相关缓存（复用clearContractCache方法）
- 导出不需要清除缓存

## 七、安全考虑

### 7.1 文件大小限制
- 导入文件大小限制：最大50MB
- 导出文件大小限制：最大100MB（超过则使用对象存储）

### 7.2 文件内容验证
- 验证文件格式（CSV/JSON）
- 防止恶意文件注入
- 限制导入记录数量

### 7.3 权限控制
- 导入导出操作需要认证
- 考虑添加角色权限控制（如：仅管理员可导入）

### 7.4 数据安全
- 导入时验证数据来源
- 敏感操作记录审计日志

## 八、实现细节

### 8.1 文件结构

```
internal/
  service/
    contract_info.go          # 现有服务文件
    contract_info_import.go   # 导入功能实现
    contract_info_export.go   # 导出功能实现
  utils/
    csv_parser.go             # CSV解析工具
    json_parser.go             # JSON解析工具
    validator.go              # 数据校验工具
```

### 8.2 核心函数设计

#### 8.2.1 导出函数
```go
// ExportContractInfos 导出合约信息
// 返回标准的 Reply 类型，响应数据放在 Reply.data 中
func (s *ContractInfoService) ExportContractInfos(
    ctx context.Context, 
    req *pb.ExportContractInfosRequest,
) (*pb.Reply, error) {
    // ... 实现逻辑 ...
    
    // 构建响应数据
    responseData := &ExportContractInfosResponseData{
        FileContentBase64: base64.StdEncoding.EncodeToString(fileContent),
        Format: req.Format,
        TotalCount: int32(len(contracts)),
        FileSize: int64(len(fileContent)),
        ExportedAt: time.Now().Format(time.RFC3339),
    }
    
    // 使用 MarshalAndReply 封装响应
    return MarshalAndReply(responseData, "marshal export response error")
}

// exportToCSV 导出为CSV格式
func exportToCSV(contracts []models.ContractInfo) ([]byte, error)

// exportToJSON 导出为JSON格式
func exportToJSON(contracts []models.ContractInfo) ([]byte, error)
```

#### 8.2.2 导入函数
```go
// ImportContractInfos 导入合约信息
// 返回标准的 Reply 类型，响应数据放在 Reply.data 中
func (s *ContractInfoService) ImportContractInfos(
    ctx context.Context, 
    req *pb.ImportContractInfosRequest,
) (*pb.Reply, error) {
    // ... 实现逻辑 ...
    
    // 构建响应数据
    responseData := &ImportContractInfosResponseData{
        TotalCount: result.TotalCount,
        SuccessCount: result.SuccessCount,
        FailedCount: result.FailedCount,
        CreatedCount: result.CreatedCount,
        UpdatedCount: result.UpdatedCount,
        Errors: result.Errors,
        ImportedAt: time.Now().Format(time.RFC3339),
    }
    
    // 使用 MarshalAndReply 封装响应
    return MarshalAndReply(responseData, "marshal import response error")
}

// parseCSVFile 解析CSV文件
func parseCSVFile(content []byte) ([]*ContractInfoImport, error)

// parseJSONFile 解析JSON文件
func parseJSONFile(content []byte) ([]*ContractInfoImport, error)

// validateContractInfo 校验合约信息
func validateContractInfo(info *ContractInfoImport) error

// batchImportContracts 批量导入合约
func batchImportContracts(
    ctx context.Context,
    contracts []*ContractInfoImport,
    mode string,
) (*ImportResult, error)
```

### 8.3 辅助结构体

```go
// ExportContractInfosResponseData 导出响应数据结构
// 注意：此结构体不在proto中定义，仅作为Go结构体用于响应数据封装
type ExportContractInfosResponseData struct {
    FileContentBase64 string `json:"file_content_base64,omitempty"` // 小文件直接返回base64
    DownloadURL       string `json:"download_url,omitempty"`         // 大文件返回下载URL
    Format            string `json:"format"`                          // 文件格式: csv/json
    TotalCount        int32  `json:"total_count"`                    // 导出记录数
    FileSize          int64  `json:"file_size"`                      // 文件大小（字节）
    ExportedAt        string `json:"exported_at"`                   // 导出时间
}

// ImportContractInfosResponseData 导入响应数据结构
// 注意：此结构体不在proto中定义，仅作为Go结构体用于响应数据封装
type ImportContractInfosResponseData struct {
    TotalCount   int32              `json:"total_count"`   // 总记录数
    SuccessCount int32              `json:"success_count"` // 成功数
    FailedCount  int32              `json:"failed_count"`  // 失败数
    CreatedCount int32              `json:"created_count"` // 创建数
    UpdatedCount int32              `json:"updated_count"` // 更新数
    Errors       []*ImportErrorInfo `json:"errors"`        // 错误列表
    ImportedAt   string             `json:"imported_at"`  // 导入时间
}

// ImportErrorInfo 导入错误信息
type ImportErrorInfo struct {
    RowNumber       int32  `json:"row_number"`        // 行号
    ContractAddress string `json:"contract_address"`   // 合约地址
    ErrorMessage    string `json:"error_message"`      // 错误消息
    ErrorCode       string `json:"error_code"`        // 错误代码
}

// ContractInfoImport 导入用的合约信息结构
type ContractInfoImport struct {
    ContractAddress    string
    ChainID            string
    Name               string
    TokenName          string
    TokenSymbol        string
    ContractType       string
    VerificationStatus string
    IsVerified         bool
    VerifiedAt         *time.Time
    VerifiedBy         string
    Metadata           map[string]interface{}
    Tags               []*TagImport
}

// TagImport 导入用的标签结构
type TagImport struct {
    TagType     string
    TagValue    string
    TagColor    string
    Description string
    CreatedBy   string
}

// ImportResult 导入结果
type ImportResult struct {
    TotalCount  int32
    SuccessCount int32
    FailedCount int32
    CreatedCount int32
    UpdatedCount int32
    Errors      []*ImportErrorInfo
}
```

### 8.4 响应封装说明

所有接口都使用标准的 `Reply` 类型返回，通过 `MarshalAndReply` 函数将响应数据封装到 `Reply.data` 字段中：

1. **成功响应**：`code=200`, `message="success"`, `data` 包含响应数据
2. **错误响应**：`code` 为HTTP状态码, `message` 包含错误信息, `data` 为 `null`

响应数据通过 `MarshalAndReply` 自动转换为 `google.protobuf.Struct` 类型，支持任意JSON结构。

## 九、测试方案

### 9.1 单元测试
- CSV解析/生成测试
- JSON解析/生成测试
- 数据校验测试
- 批量导入测试

### 9.2 集成测试
- 完整导出流程测试
- 完整导入流程测试
- 错误处理测试
- 性能测试（大数据量）

### 9.3 测试用例

#### 9.3.1 导出测试
- 导出所有合约（CSV/JSON）
- 带过滤条件导出
- 空结果导出
- 大数据量导出

#### 9.3.2 导入测试
- 正常导入（create/update/upsert）
- 数据校验失败
- 部分成功部分失败
- 大数据量导入
- 格式错误处理

## 十、部署和监控

### 10.1 配置项
- 最大导入文件大小
- 最大导入记录数
- 批量处理大小
- 导出文件存储路径/对象存储配置

### 10.2 监控指标
- 导出请求数、成功率、平均耗时
- 导入请求数、成功率、平均耗时
- 导入/导出记录数统计
- 错误率统计

### 10.3 日志记录
- 导出操作日志（用户、时间、记录数）
- 导入操作日志（用户、时间、成功/失败数）
- 错误详情日志

## 十一、后续优化

### 11.1 功能增强
- 支持Excel格式导入导出
- 支持增量导出（仅导出变更数据）
- 支持导入预览（先验证，再确认导入）
- 支持导入模板下载

### 11.2 性能优化
- 异步导出（大文件）
- 导入进度查询接口
- 导入任务队列

### 11.3 用户体验
- 提供导入模板文件
- 详细的错误提示和修复建议
- 导入结果统计报告

## 十二、实施计划

### 12.1 第一阶段：基础功能
1. Proto定义和代码生成
2. CSV导出功能
3. JSON导出功能
4. CSV导入功能（基础）
5. JSON导入功能（基础）

### 12.2 第二阶段：完善功能
1. 数据校验增强
2. 错误处理完善
3. 批量处理优化
4. 单元测试和集成测试

### 12.3 第三阶段：优化和上线
1. 性能优化
2. 监控和日志完善
3. 文档编写
4. 上线部署
