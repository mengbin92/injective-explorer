# 方案四：存储分离 - 资源清单

## 概述

方案四（存储分离）是将交易数据的大字段（Messages, Logs, Data）存储到对象存储服务，数据库中只存储引用（storage key）。这样可以大幅减少数据库存储空间，降低存储成本。

## 所需资源清单

### 1. 对象存储服务

#### 1.1 云服务商选项

**AWS S3**
- **优势**：成熟稳定、全球可用、丰富的功能
- **成本**：按存储量、请求次数、数据传输计费
- **适用场景**：生产环境、全球部署
- **参考价格**：标准存储约 $0.023/GB/月

**阿里云 OSS**
- **优势**：国内访问速度快、价格相对便宜
- **成本**：按存储量、请求次数计费
- **适用场景**：国内部署、需要快速访问
- **参考价格**：标准存储约 ¥0.12/GB/月

**腾讯云 COS**
- **优势**：与腾讯云生态集成好
- **成本**：按存储量、请求次数计费
- **适用场景**：使用腾讯云基础设施

**MinIO（自建）**
- **优势**：开源免费、S3兼容、可自建
- **成本**：服务器成本
- **适用场景**：私有部署、开发测试、成本敏感场景
- **部署要求**：需要服务器资源

#### 1.2 存储容量估算

根据当前数据量：
- **每天数据量**：70-100GB（压缩后约 20-30GB）
- **每月数据量**：约 600-900GB（压缩后约 180-270GB）
- **每年数据量**：约 7-11TB（压缩后约 2-3TB）

**建议配置**：
- **热数据**（最近3个月）：标准存储，约 540-810GB
- **温数据**（3-12个月）：低频访问存储，约 1.8-2.7TB
- **冷数据**（1年以上）：归档存储，约 2-3TB

### 2. Go SDK/客户端库

#### 2.1 AWS S3 SDK

```go
// 需要添加的依赖
github.com/aws/aws-sdk-go-v2/service/s3 v1.50.0
github.com/aws/aws-sdk-go-v2/config v1.26.0
github.com/aws/aws-sdk-go-v2/credentials v1.16.0
```

**安装命令**：
```bash
go get github.com/aws/aws-sdk-go-v2/service/s3
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/credentials
```

#### 2.2 阿里云 OSS SDK

```go
// 需要添加的依赖
github.com/aliyun/aliyun-oss-go-sdk/oss v2.2.7+incompatible
```

**安装命令**：
```bash
go get github.com/aliyun/aliyun-oss-go-sdk/oss
```

#### 2.3 腾讯云 COS SDK

```go
// 需要添加的依赖
github.com/tencentyun/cos-go-sdk-v5 v0.7.41
```

**安装命令**：
```bash
go get github.com/tencentyun/cos-go-sdk-v5
```

#### 2.4 MinIO SDK（推荐，S3兼容）

```go
// 需要添加的依赖
github.com/minio/minio-go/v7 v7.0.66
```

**安装命令**：
```bash
go get github.com/minio/minio-go/v7/v7
```

**优势**：
- S3兼容，可以无缝切换到S3
- 支持多种存储后端
- 文档完善

### 3. 配置管理

需要在 `internal/conf/conf.proto` 中添加对象存储配置：

```protobuf
message Data {
  Database database = 1;
  Redis redis = 2;
  ObjectStorage object_storage = 3; // 新增
}

message ObjectStorage {
  string provider = 1; // "s3", "oss", "cos", "minio"
  string endpoint = 2; // 对象存储端点
  string access_key_id = 3; // 访问密钥ID
  string secret_access_key = 4; // 访问密钥
  string bucket_name = 5; // 存储桶名称
  string region = 6; // 区域（S3/OSS需要）
  bool use_ssl = 7; // 是否使用SSL
  string path_prefix = 8; // 路径前缀，如 "transactions/"
}
```

在 `configs/config.yaml` 中添加配置：

```yaml
data:
  object_storage:
    provider: minio  # s3, oss, cos, minio
    endpoint: localhost:9000
    access_key_id: minioadmin
    secret_access_key: minioadmin
    bucket_name: injective-explorer
    region: us-east-1
    use_ssl: false
    path_prefix: transactions/
```

### 4. 存储服务封装

需要创建 `provider/storage/` 目录，实现统一的存储接口：

**文件结构**：
```
provider/storage/
├── storage.go          # 统一存储接口
├── s3.go              # AWS S3 实现
├── oss.go             # 阿里云 OSS 实现
├── cos.go             # 腾讯云 COS 实现
└── minio.go           # MinIO 实现
```

**接口定义**：
```go
type Storage interface {
    // PutObject 上传对象
    PutObject(ctx context.Context, key string, data []byte) error
    
    // GetObject 获取对象
    GetObject(ctx context.Context, key string) ([]byte, error)
    
    // DeleteObject 删除对象
    DeleteObject(ctx context.Context, key string) error
    
    // Exists 检查对象是否存在
    Exists(ctx context.Context, key string) (bool, error)
    
    // GetObjectURL 获取对象访问URL（可选）
    GetObjectURL(ctx context.Context, key string) (string, error)
}
```

### 5. 数据模型修改

需要修改 `internal/models/transaction.go`：

```go
type Transaction struct {
    // ... 现有字段
    
    // 对象存储相关字段
    DataStorageKey     string `json:"data_storage_key" gorm:"column:data_storage_key;index"`
    MessagesStorageKey string `json:"messages_storage_key" gorm:"column:messages_storage_key;index"`
    LogsStorageKey     string `json:"logs_storage_key" gorm:"column:logs_storage_key;index"`
    
    // 保留字段用于向后兼容（可选）
    // Data, Messages, Logs 字段可以保留但标记为废弃
}
```

### 6. 数据库迁移脚本

需要创建 `database/storage_separation_migration.sql`：

```sql
-- 添加存储键字段
ALTER TABLE transactions 
ADD COLUMN data_storage_key VARCHAR(255) NULL AFTER data,
ADD COLUMN messages_storage_key VARCHAR(255) NULL AFTER messages,
ADD COLUMN logs_storage_key VARCHAR(255) NULL AFTER logs;

-- 创建索引
CREATE INDEX idx_data_storage_key ON transactions(data_storage_key);
CREATE INDEX idx_messages_storage_key ON transactions(messages_storage_key);
CREATE INDEX idx_logs_storage_key ON transactions(logs_storage_key);
```

### 7. 数据迁移工具

需要创建迁移服务 `internal/service/migration.go`：

**功能**：
- 将现有数据库中的大字段迁移到对象存储
- 支持批量迁移
- 支持断点续传
- 迁移后验证数据完整性

### 8. 查询逻辑修改

需要修改以下文件：
- `internal/models/transaction.go` - `TransactionFromProto` 和 `TransactionToProto`
- `internal/service/demo.go` - 查询逻辑需要从对象存储加载数据

### 9. 缓存策略

可以结合Redis缓存：
- 热点数据缓存到Redis
- 减少对象存储访问次数
- 降低延迟

### 10. 监控和告警

需要监控：
- 对象存储使用量
- 对象存储请求次数
- 对象存储访问延迟
- 存储成本

## 实施步骤

### 阶段一：基础设施准备（1-2周）

1. **选择对象存储服务**
   - 评估各选项的成本、性能、可用性
   - 创建存储桶（Bucket）
   - 配置访问权限和生命周期策略

2. **安装SDK依赖**
   ```bash
   go get github.com/minio/minio-go/v7/v7
   # 或其他选择的SDK
   ```

3. **创建存储服务封装**
   - 实现统一存储接口
   - 支持多种存储后端
   - 添加错误处理和重试逻辑

### 阶段二：代码实现（2-3周）

1. **配置管理**
   - 添加对象存储配置到 `conf.proto`
   - 更新配置文件

2. **数据模型修改**
   - 添加存储键字段
   - 创建数据库迁移脚本

3. **存储逻辑实现**
   - 修改 `TransactionFromProto` 上传数据到对象存储
   - 修改 `TransactionToProto` 从对象存储加载数据
   - 添加缓存层

4. **查询逻辑修改**
   - 修改查询方法支持从对象存储加载数据
   - 添加错误处理和降级策略

### 阶段三：数据迁移（1-2周）

1. **迁移工具开发**
   - 实现批量迁移逻辑
   - 支持断点续传
   - 数据验证

2. **执行迁移**
   - 在测试环境验证
   - 生产环境分批迁移
   - 监控迁移进度和错误

### 阶段四：测试和优化（1-2周）

1. **功能测试**
   - 数据读写测试
   - 错误场景测试
   - 性能测试

2. **优化**
   - 缓存策略优化
   - 批量操作优化
   - 错误重试策略优化

## 成本估算

### 对象存储成本（以阿里云OSS为例）

**存储成本**：
- 标准存储：¥0.12/GB/月
- 低频访问：¥0.08/GB/月
- 归档存储：¥0.033/GB/月

**请求成本**：
- PUT请求：¥0.01/万次
- GET请求：¥0.01/万次

**数据传输成本**：
- 内网传输：免费
- 外网传输：¥0.50/GB

**示例计算**（假设每月新增600GB）：
- 标准存储（3个月）：600GB × 3 × ¥0.12 = ¥216/月
- 低频存储（9个月）：600GB × 9 × ¥0.08 = ¥432/月
- 归档存储（1年+）：600GB × 12 × ¥0.033 = ¥238/月
- **总计**：约 ¥886/月（约 $125/月）

相比数据库存储（假设每GB成本¥5）：
- 数据库存储：600GB × ¥5 = ¥3000/月
- **节省**：约 70% 成本

## 技术选型建议

### 开发/测试环境
- **推荐**：MinIO（自建）
- **原因**：免费、S3兼容、易于测试

### 生产环境（国内）
- **推荐**：阿里云 OSS
- **原因**：访问速度快、价格合理、服务稳定

### 生产环境（海外）
- **推荐**：AWS S3
- **原因**：全球可用、功能丰富、生态完善

### 混合方案
- **热数据**：数据库（最近1个月）
- **温数据**：对象存储标准存储（1-6个月）
- **冷数据**：对象存储归档存储（6个月以上）

## 注意事项

1. **向后兼容**：需要支持读取旧数据（数据库中的大字段）
2. **数据一致性**：确保数据库和对象存储数据一致
3. **错误处理**：对象存储不可用时需要有降级策略
4. **安全性**：访问密钥需要妥善保管，建议使用IAM角色
5. **性能**：对象存储访问有延迟，需要合理使用缓存
6. **成本控制**：设置存储生命周期策略，自动归档旧数据
7. **监控告警**：监控存储使用量和成本，设置告警阈值

## 参考资源

- [AWS S3 Go SDK](https://docs.aws.amazon.com/sdk-for-go/v2/service/s3/)
- [阿里云 OSS Go SDK](https://help.aliyun.com/document_detail/32100.html)
- [MinIO Go SDK](https://docs.min.io/docs/golang-client-quickstart-guide.html)
- [对象存储最佳实践](https://docs.aws.amazon.com/AmazonS3/latest/userguide/best-practices.html)

