# 归档服务使用指南

## 概述

归档服务现在支持**自动调度模式**（类似同步服务），可以在应用启动时自动启动，定期执行归档任务。

## 问题说明

如果发现MinIO中有数据，但`SELECT COUNT(*) FROM transactions WHERE archived = 1;`返回0，说明**归档服务从未被触发执行**。

MinIO中的数据可能来自：
- 数据同步时的自动对象存储上传（`TransactionFromProto`方法）
- 这些数据存储在`transactions/data/...`路径下
- 这些数据**不是归档数据**，所以`archived = 0`是正常的

归档数据应该存储在`transactions/archive/data/...`路径下，并且`transactions`表的`archived`字段应该为`true`。

## 解决方案

现在提供了四种方式来触发归档服务：

### 方案零：自动调度模式（推荐，类似同步服务）⭐

这是最简单的方式，类似同步服务，在应用启动时自动启动归档调度服务，定期执行归档任务。

#### 1. 启用自动归档

在`configs/config.yaml`中设置：

```yaml
archive:
  # 启用自动归档（默认: false）
  enabled: true
  # 归档执行间隔（默认: 24h），格式: "24h", "12h", "1h" 等
  interval: 24h
  # 归档周期（月），归档此时间之前的交易数据（默认: 6）
  age_months: 6
  # 批次大小，每批处理的交易数量（默认: 1000）
  batch_size: 1000
  # 存储阈值（字节），超过此大小的字段会上传到对象存储（默认: 10KB）
  storage_threshold: 10240
  # 是否使用对象存储
  use_object_storage: true
```

#### 2. 启动应用

```bash
# 正常启动应用，归档服务会自动启动
./bin/injective-explorer -conf configs
```

归档服务会在后台定期执行归档任务，就像同步服务一样。

#### 3. 查看日志

归档服务的执行日志会输出到应用日志中：

```
INFO archive scheduler service enabled and started
INFO starting scheduled archive operation
INFO scheduled archive operation completed successfully in 5m30s
```

### 方案一：使用独立的归档命令工具

### 方案一：使用独立的归档命令工具（推荐）

这是最灵活的方式，适合手动执行或通过Cron定时任务执行。

#### 1. 直接运行（开发环境）

```bash
# 使用默认配置（从config.yaml加载）
go run cmd/archive/main.go -conf configs

# 指定归档年龄（覆盖配置文件）
go run cmd/archive/main.go -conf configs -age-months 6

# 指定批次大小
go run cmd/archive/main.go -conf configs -batch-size 2000

# 干运行模式（仅模拟，不实际归档）
go run cmd/archive/main.go -conf configs -dry-run

# 组合使用
go run cmd/archive/main.go -conf configs -age-months 6 -batch-size 2000
```

#### 2. 编译后运行

```bash
# 编译归档工具
go build -o bin/archive cmd/archive/main.go

# 运行
./bin/archive -conf configs

# 或使用完整路径
./bin/archive -conf /path/to/configs
```

#### 3. 通过Cron定时执行（生产环境推荐）

```bash
# 创建归档脚本
cat > /path/to/archive.sh << 'EOF'
#!/bin/bash
cd /path/to/injective-explorer
./bin/archive -conf configs >> /var/log/archive.log 2>&1
EOF

chmod +x /path/to/archive.sh

# 添加到crontab（每天凌晨2点执行）
crontab -e
# 添加以下行：
0 2 * * * /path/to/archive.sh
```

### 方案二：使用独立的归档命令工具（手动执行）

适用于需要手动触发归档的场景。

主程序现在支持`-archive`参数，可以直接执行归档后退出。

```bash
# 使用默认配置归档
./bin/injective-explorer -conf configs -archive

# 指定归档年龄
./bin/injective-explorer -conf configs -archive -archive-age-months 6

# 指定批次大小
./bin/injective-explorer -conf configs -archive -archive-batch-size 2000

# 干运行模式
./bin/injective-explorer -conf configs -archive -archive-dry-run

# 组合使用
./bin/injective-explorer -conf configs -archive -archive-age-months 6 -archive-batch-size 2000
```

### 方案三：使用主程序的归档模式（一次性执行）

适用于需要立即执行一次归档的场景。

### 方案四：在代码中直接调用（开发/测试）

如果需要集成到其他代码中，可以直接调用归档服务：

```go
import (
    "context"
    "injective-explorer/internal/service"
)

// 创建归档服务
archiveService := service.NewArchiveService()

// 使用默认配置（自动从config.yaml加载）
config := service.DefaultArchiveConfig()
ctx := context.Background()

// 执行归档
err := archiveService.ArchiveOldTransactions(ctx, config)
if err != nil {
    log.Printf("归档失败: %v", err)
} else {
    log.Printf("归档完成")
}

// 或使用自定义配置
customConfig := &service.ArchiveConfig{
    ArchiveAgeMonths: 6,        // 归档6个月前的数据
    BatchSize:        1000,     // 每批处理1000条
    DryRun:           false,    // false=实际执行，true=仅模拟
    UseObjectStorage: true,     // 是否使用对象存储
    StorageThreshold: 10 * 1024, // 10KB，超过此大小的字段会上传到对象存储
}
err := archiveService.ArchiveOldTransactions(ctx, customConfig)
```

## 配置说明

归档配置可以在`configs/config.yaml`中设置：

```yaml
archive:
  # 归档周期（月），归档此时间之前的交易数据（默认: 6）
  age_months: ${ARCHIVE_AGE_MONTHS:6}
  # 批次大小，每批处理的交易数量（默认: 1000）
  batch_size: ${ARCHIVE_BATCH_SIZE:1000}
  # 存储阈值（字节），超过此大小的字段会上传到对象存储（默认: 10KB）
  storage_threshold: ${ARCHIVE_STORAGE_THRESHOLD:10240}
  # 是否使用对象存储，true=使用对象存储，false=存储在数据库
  use_object_storage: true
```

命令行参数会覆盖配置文件中的设置。

## 首次归档建议

1. **先使用干运行模式测试**：
   ```bash
   ./bin/archive -conf configs -dry-run
   ```
   这会显示将要归档的数据量，但不会实际执行。

2. **备份数据库**：
   ```bash
   mysqldump -u root -p explorer > backup_before_archive.sql
   ```

3. **执行归档**：
   ```bash
   ./bin/archive -conf configs
   ```

4. **验证归档结果**：
   ```sql
   -- 检查归档状态
   SELECT COUNT(*) FROM transactions WHERE archived = 1;
   
   -- 检查归档数据表
   SELECT COUNT(*) FROM transaction_data_archives;
   
   -- 检查存储键
   SELECT COUNT(*) FROM transaction_data_archives 
   WHERE data_storage_key LIKE 'archive/data/%';
   ```

## 验证归档成功

### 1. 数据库验证

```sql
-- 检查归档状态概览
SELECT 
    COUNT(*) as total_transactions,
    SUM(CASE WHEN archived = 1 THEN 1 ELSE 0 END) as archived_count,
    SUM(CASE WHEN archived = 0 OR archived IS NULL THEN 1 ELSE 0 END) as not_archived_count
FROM transactions;

-- 检查归档数据表记录数
SELECT COUNT(*) FROM transaction_data_archives;

-- 检查存储键统计
SELECT 
    COUNT(*) as total_archive_records,
    SUM(CASE WHEN data_storage_key IS NOT NULL AND data_storage_key != '' THEN 1 ELSE 0 END) as data_in_storage_count,
    SUM(CASE WHEN messages_storage_key IS NOT NULL AND messages_storage_key != '' THEN 1 ELSE 0 END) as messages_in_storage_count,
    SUM(CASE WHEN logs_storage_key IS NOT NULL AND logs_storage_key != '' THEN 1 ELSE 0 END) as logs_in_storage_count
FROM transaction_data_archives;
```

### 2. MinIO验证

```bash
# 使用MinIO客户端
mc ls -r local/injective-explorer/transactions/archive/

# 统计归档对象数量
mc ls -r local/injective-explorer/transactions/archive/data/ | wc -l
mc ls -r local/injective-explorer/transactions/archive/messages/ | wc -l
mc ls -r local/injective-explorer/transactions/archive/logs/ | wc -l
```

### 3. 使用验证脚本

```bash
# 运行SQL验证脚本
mysql -u root -p explorer < scripts/verify_archive_storage.sql

# 运行MinIO验证脚本
./scripts/verify_minio_objects.sh
```

## 注意事项

1. **归档时间点**：建议归档6个月前的数据
2. **归档频率**：建议每周或每月执行一次
3. **性能影响**：归档操作会占用数据库资源，建议在低峰期执行
4. **监控**：归档过程中监控数据库和对象存储的使用情况
5. **备份**：执行归档前建议备份数据库

## 故障排查

### 问题：归档后`archived = 1`仍然为0

**可能原因**：
1. 归档服务执行失败（检查日志）
2. 事务回滚（检查数据库错误日志）
3. 没有满足归档条件的交易（检查`created_at`时间）

**解决方法**：
```sql
-- 检查是否有满足归档条件的交易
SELECT COUNT(*) FROM transactions 
WHERE created_at < DATE_SUB(NOW(), INTERVAL 6 MONTH) 
  AND (archived IS NULL OR archived = 0);
```

### 问题：MinIO中有数据但数据库中没有归档记录

**可能原因**：
- MinIO中的数据是同步时自动上传的（`transactions/data/...`），不是归档数据
- 归档数据应该在`transactions/archive/data/...`路径下

**解决方法**：
- 确认MinIO中的路径
- 如果路径是`transactions/data/...`，这是正常的（同步时上传）
- 如果路径是`transactions/archive/data/...`，说明归档执行了，但数据库更新可能失败

## 自动化建议

### 使用Cron定时任务（推荐）

```bash
# 每天凌晨2点执行归档
0 2 * * * cd /path/to/injective-explorer && ./bin/archive -conf configs >> /var/log/archive.log 2>&1
```

### 使用systemd定时器

创建`/etc/systemd/system/archive.service`：
```ini
[Unit]
Description=Injective Explorer Archive Service
After=network.target

[Service]
Type=oneshot
User=your-user
WorkingDirectory=/path/to/injective-explorer
ExecStart=/path/to/injective-explorer/bin/archive -conf configs
StandardOutput=append:/var/log/archive.log
StandardError=append:/var/log/archive.log
```

创建`/etc/systemd/system/archive.timer`：
```ini
[Unit]
Description=Run archive service daily
Requires=archive.service

[Timer]
OnCalendar=daily
OnCalendar=02:00
Persistent=true

[Install]
WantedBy=timers.target
```

启用定时器：
```bash
sudo systemctl enable archive.timer
sudo systemctl start archive.timer
```
