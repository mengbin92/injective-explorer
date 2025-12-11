# 归档到对象存储的本地测试指南

## 概述

本指南说明如何在本地测试环境中验证历史数据是否成功归档到对象存储（MinIO）。

## 前置条件

1. **MinIO 服务运行中**
   ```bash
   # 检查 MinIO 是否运行
   docker ps | grep minio
   
   # 如果没有运行，启动 MinIO
   docker-compose up -d minio
   ```

2. **对象存储配置已启用**
   - 检查 `configs/config.yaml` 中 `object_storage.enabled: true`
   - 检查 `archive.use_object_storage` 配置（如果设置了）

3. **数据库中有历史数据**
   - 确保 `transactions` 表中有足够的历史数据
   - 数据需要满足归档条件（超过配置的 `age_months`）

## 测试步骤

### 步骤 1：准备测试数据

#### 方法一：修改现有数据的创建时间（推荐用于测试）

```sql
-- 连接到数据库
mysql -u root -p explorer

-- 查看当前有多少交易数据
SELECT COUNT(*) FROM transactions WHERE archived = 0 OR archived IS NULL;

-- 将一些交易的时间修改为6个月前（假设归档周期是6个月）
UPDATE transactions 
SET created_at = DATE_SUB(NOW(), INTERVAL 7 MONTH)
WHERE id IN (
    SELECT id FROM (
        SELECT id FROM transactions 
        WHERE (archived = 0 OR archived IS NULL)
        LIMIT 100
    ) AS t
);

-- 确保这些交易有大字段数据（超过存储阈值，默认10KB）
-- 检查数据大小
SELECT 
    id, 
    hash,
    LENGTH(data) as data_size,
    LENGTH(messages) as messages_size,
    LENGTH(logs) as logs_size
FROM transactions 
WHERE created_at < DATE_SUB(NOW(), INTERVAL 6 MONTH)
AND (archived = 0 OR archived IS NULL)
LIMIT 10;
```

#### 方法二：使用测试脚本创建测试数据

创建一个测试脚本 `scripts/create_test_archive_data.go`：

```go
package main

import (
    "context"
    "time"
    "injective-explorer/internal/global"
    "injective-explorer/internal/models"
    "injective-explorer/provider/db"
    // ... 其他导入
)

func main() {
    // 初始化数据库连接
    // 创建一些测试交易数据，设置 created_at 为7个月前
    // 确保 data/messages/logs 字段超过 10KB
}
```

### 步骤 2：配置归档参数

在 `configs/config.yaml` 中配置：

```yaml
archive:
  age_months: 6  # 归档6个月前的数据（测试时可以设置更小，如1）
  batch_size: 10  # 测试时使用小批次，便于观察
  storage_threshold: 10240  # 10KB，确保大字段会上传到对象存储
  use_object_storage: true  # 确保启用对象存储
```

### 步骤 3：执行归档

#### 方法一：通过代码调用（推荐）

创建一个测试脚本 `scripts/test_archive.go`：

```go
package main

import (
    "context"
    "fmt"
    "log"
    "injective-explorer/internal/service"
    "injective-explorer/internal/global"
    "injective-explorer/internal/conf"
    "injective-explorer/provider/logger"
    // ... 其他导入
)

func main() {
    // 1. 加载配置
    // 2. 初始化 global（包括数据库和存储）
    // 3. 创建归档服务
    archiveService := service.NewArchiveService()
    
    // 4. 使用配置加载归档配置
    config := service.DefaultArchiveConfig()
    config.DryRun = false  // 实际执行，不是模拟
    
    // 5. 执行归档
    ctx := context.Background()
    err := archiveService.ArchiveOldTransactions(ctx, config)
    if err != nil {
        log.Fatalf("归档失败: %v", err)
    }
    
    fmt.Println("归档完成！")
}
```

#### 方法二：在应用代码中调用

在应用启动时或通过 API 调用归档服务。

### 步骤 4：验证数据库中的归档记录

```sql
-- 1. 检查有多少交易被归档
SELECT COUNT(*) FROM transactions WHERE archived = 1;

-- 2. 检查归档数据表
SELECT COUNT(*) FROM transaction_data_archives;

-- 3. 检查哪些数据有存储键（说明已上传到对象存储）
SELECT 
    transaction_id,
    data_storage_key,
    messages_storage_key,
    logs_storage_key,
    CASE 
        WHEN data_storage_key IS NOT NULL THEN 'Data in storage'
        ELSE 'Data in DB'
    END as data_location,
    CASE 
        WHEN messages_storage_key IS NOT NULL THEN 'Messages in storage'
        ELSE 'Messages in DB'
    END as messages_location,
    CASE 
        WHEN logs_storage_key IS NOT NULL THEN 'Logs in storage'
        ELSE 'Logs in DB'
    END as logs_location
FROM transaction_data_archives
LIMIT 20;

-- 4. 检查数据字段是否已清空（如果已上传到对象存储）
SELECT 
    transaction_id,
    data_storage_key,
    LENGTH(data) as data_length,
    messages_storage_key,
    LENGTH(messages) as messages_length,
    logs_storage_key,
    LENGTH(logs) as logs_length
FROM transaction_data_archives
WHERE data_storage_key IS NOT NULL 
   OR messages_storage_key IS NOT NULL 
   OR logs_storage_key IS NOT NULL
LIMIT 10;
```

### 步骤 5：验证 MinIO 中的对象

#### 方法一：使用 MinIO 控制台（Web UI）

1. 打开 MinIO 控制台：http://localhost:19001
2. 登录（默认用户名/密码：minioadmin/minioadmin）
3. 选择 bucket：`injective-explorer`
4. 浏览路径：`transactions/archive/`
5. 应该能看到类似以下路径的对象：
   - `transactions/archive/data/0x1234567/0x1234567890abcdef...`
   - `transactions/archive/messages/0x1234567/0x1234567890abcdef...`
   - `transactions/archive/logs/0x1234567/0x1234567890abcdef...`

#### 方法二：使用 MinIO 客户端（mc）

```bash
# 安装 MinIO 客户端（如果未安装）
# macOS
brew install minio/stable/mc

# 配置 MinIO 客户端
mc alias set local http://localhost:19000 minioadmin minioadmin

# 列出 bucket
mc ls local/

# 列出归档对象
mc ls local/injective-explorer/transactions/archive/

# 递归列出所有归档对象
mc ls -r local/injective-explorer/transactions/archive/

# 统计归档对象数量
mc ls -r local/injective-explorer/transactions/archive/ | wc -l

# 查看特定对象
mc cat local/injective-explorer/transactions/archive/data/0x1234567/0x1234567890abcdef...
```

#### 方法三：使用 SQL 查询存储键，然后验证

```sql
-- 获取一个存储键示例
SELECT data_storage_key 
FROM transaction_data_archives 
WHERE data_storage_key IS NOT NULL 
LIMIT 1;
```

然后使用代码验证：

```go
import (
    "context"
    "injective-explorer/provider/storage"
)

func verifyObjectInStorage(storageKey string) error {
    ctx := context.Background()
    storage := storage.Get()
    
    // 检查对象是否存在
    exists, err := storage.Exists(ctx, storageKey)
    if err != nil {
        return err
    }
    if !exists {
        return fmt.Errorf("对象不存在: %s", storageKey)
    }
    
    // 读取对象内容
    data, err := storage.GetObject(ctx, storageKey)
    if err != nil {
        return err
    }
    
    fmt.Printf("对象大小: %d 字节\n", len(data))
    return nil
}
```

### 步骤 6：验证数据完整性

#### 验证归档数据可以正确读取

```go
// 使用归档服务查询已归档的交易
archiveService := service.NewArchiveService()
ctx := context.Background()

// 查询一个已归档的交易
tx, err := archiveService.GetArchivedTransaction(ctx, "0x...交易hash...")
if err != nil {
    log.Fatalf("查询归档交易失败: %v", err)
}

// 验证数据是否完整
if len(tx.Data) == 0 && tx.DataStorageKey == "" {
    log.Println("警告: Data 字段为空且没有存储键")
} else if tx.DataStorageKey != "" {
    log.Printf("Data 已存储在对象存储: %s", tx.DataStorageKey)
    // 可以从对象存储读取验证
}
```

#### 使用 SQL 验证

```sql
-- 对比主表和归档表的数据
SELECT 
    t.id,
    t.hash,
    t.archived,
    tda.transaction_id,
    tda.data_storage_key,
    tda.messages_storage_key,
    tda.logs_storage_key
FROM transactions t
LEFT JOIN transaction_data_archives tda ON t.id = tda.transaction_id
WHERE t.archived = 1
LIMIT 10;
```

## 验证清单

- [ ] MinIO 服务运行正常
- [ ] 对象存储配置已启用（`object_storage.enabled: true`）
- [ ] 归档配置正确（`archive.use_object_storage: true`，`storage_threshold` 设置合理）
- [ ] 数据库中有满足归档条件的数据（时间 + 大小）
- [ ] 归档执行成功（无错误日志）
- [ ] 数据库中的 `transactions.archived = 1` 记录增加
- [ ] `transaction_data_archives` 表中有新记录
- [ ] `transaction_data_archives` 表中的 `*_storage_key` 字段有值
- [ ] MinIO 中存在对应的对象文件
- [ ] 可以通过存储键从对象存储读取数据
- [ ] 归档后的交易可以通过 `GetArchivedTransaction` 正确查询

## 常见问题排查

### 问题 1：归档执行了但没有上传到对象存储

**检查项：**
1. 对象存储是否启用：`object_storage.enabled: true`
2. 归档配置中 `use_object_storage` 是否为 `true`
3. 数据大小是否超过 `storage_threshold`（默认10KB）
4. 查看应用日志，是否有上传失败的错误

**解决方案：**
```sql
-- 检查数据大小
SELECT 
    id,
    hash,
    LENGTH(data) as data_size,
    LENGTH(messages) as messages_size,
    LENGTH(logs) as logs_size
FROM transactions 
WHERE archived = 1
LIMIT 10;
```

### 问题 2：MinIO 连接失败

**检查项：**
1. MinIO 服务是否运行：`docker ps | grep minio`
2. 端口是否正确：`localhost:19000`（API），`localhost:19001`（Console）
3. 配置中的 endpoint 是否正确
4. 网络连接是否正常

**解决方案：**
```bash
# 检查 MinIO 日志
docker logs injective-explorer-minio

# 测试连接
curl http://localhost:19000/minio/health/live
```

### 问题 3：存储键存在但对象不存在

**可能原因：**
- 对象上传后又被删除
- 存储键格式错误
- bucket 名称不匹配

**解决方案：**
```sql
-- 检查存储键格式
SELECT data_storage_key 
FROM transaction_data_archives 
WHERE data_storage_key IS NOT NULL 
LIMIT 5;

-- 应该类似：transactions/archive/data/0x1234567/0x1234567890abcdef...
```

## 测试脚本示例

创建一个完整的测试脚本 `scripts/verify_archive_storage.sh`：

```bash
#!/bin/bash

echo "=== 归档到对象存储验证脚本 ==="

# 1. 检查 MinIO 服务
echo "1. 检查 MinIO 服务..."
docker ps | grep minio || { echo "错误: MinIO 服务未运行"; exit 1; }

# 2. 检查数据库中的归档记录
echo "2. 检查数据库归档记录..."
mysql -u root -p explorer -e "
SELECT 
    COUNT(*) as archived_count,
    SUM(CASE WHEN data_storage_key IS NOT NULL THEN 1 ELSE 0 END) as data_in_storage,
    SUM(CASE WHEN messages_storage_key IS NOT NULL THEN 1 ELSE 0 END) as messages_in_storage,
    SUM(CASE WHEN logs_storage_key IS NOT NULL THEN 1 ELSE 0 END) as logs_in_storage
FROM transaction_data_archives;
"

# 3. 检查 MinIO 中的对象
echo "3. 检查 MinIO 中的对象..."
mc ls -r local/injective-explorer/transactions/archive/ | wc -l

# 4. 验证存储键和对象的对应关系
echo "4. 验证存储键..."
# 这里可以添加更详细的验证逻辑

echo "=== 验证完成 ==="
```

## 日志监控

在执行归档时，关注以下日志：

```
starting transaction archiving: age_months=6, batch_size=1000, dry_run=false
found X transactions to archive
uploaded archive data to storage for tx 0x...
uploaded archive messages to storage for tx 0x...
uploaded archive logs to storage for tx 0x...
archiving completed: archived X transactions
```

如果看到 `failed to upload archive data to storage`，说明上传失败，会回退到数据库存储。

## 性能测试

测试归档性能：

```go
config := service.DefaultArchiveConfig()
config.ProgressCallback = func(processed, total int64, batchSize int) {
    fmt.Printf("进度: %d/%d (%.2f%%), 批次大小: %d\n", 
        processed, total, float64(processed)/float64(total)*100, batchSize)
}
err := archiveService.ArchiveOldTransactions(ctx, config)
```

## 总结

通过以上步骤，可以全面验证归档功能是否正常工作，以及数据是否成功上传到对象存储。关键验证点：

1. **数据库验证**：检查 `archived` 标志和 `storage_key` 字段
2. **对象存储验证**：检查 MinIO 中是否存在对应的对象文件
3. **数据完整性验证**：确保可以通过存储键正确读取数据
