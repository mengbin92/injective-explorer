# 区块归档功能测试指南

## 测试准备

### 1. 确保数据库已更新

首先确认数据库表已包含新的归档字段：

```sql
-- 检查 blocks 表是否有 archived 字段
DESCRIBE blocks;

-- 应该看到以下字段:
-- archived           tinyint(1)  DEFAULT 0
-- txs_storage_key    varchar(255) DEFAULT NULL

-- 检查 block_data_archives 表是否存在
SHOW TABLES LIKE 'block_data_archives';

-- 如果不存在，执行数据库初始化脚本
-- mysql -u username -p database_name < database/init_database.sql
```

### 2. 查看当前数据状态

```sql
-- 查看区块总数和归档状态
SELECT
    COUNT(*) as total_blocks,
    SUM(CASE WHEN archived = 1 THEN 1 ELSE 0 END) as archived_blocks,
    SUM(CASE WHEN archived = 0 OR archived IS NULL THEN 1 ELSE 0 END) as active_blocks
FROM blocks;

-- 查看区块时间范围
SELECT
    MIN(FROM_UNIXTIME(block_unix_timestamp)) as earliest_block,
    MAX(FROM_UNIXTIME(block_unix_timestamp)) as latest_block,
    COUNT(*) as total_blocks
FROM blocks;

-- 查看6个月前的区块数量
SELECT
    COUNT(*) as blocks_older_than_6_months
FROM blocks
WHERE block_unix_timestamp < UNIX_TIMESTAMP(DATE_SUB(NOW(), INTERVAL 6 MONTH));
```

---

## 测试步骤

### 第1步：编译归档工具

```bash
cd /Users/mac/vscode/mengbin/biya/biya-explorer
go build -o bin/archive cmd/archive/main.go
```

### 第2步：配置环境变量（可选）

如果使用非默认数据库配置，设置环境变量：

```bash
export DB_HOST=localhost
export DB_PORT=3306
export DB_USER=root
export DB_PASSWORD=your_password
export DB_NAME=explorer
```

### 第3步：执行 Dry-Run 测试

**测试区块归档（模拟运行）:**

```bash
./bin/archive -conf configs -type blocks -age-months 6 -batch-size 100 -dry-run
```

**预期输出:**
```
level=info msg="Starting archive tool..."
level=info msg="Archive configuration:"
level=info msg="  - Archive type: blocks"
level=info msg="  - Age months: 6"
level=info msg="  - Batch size: 100"
level=info msg="  - Dry run: true"
level=warn msg="DRY RUN MODE: No data will be actually archived"
level=info msg="📦 Starting Block Archiving..."
level=info msg="found XXX blocks to archive (older than 2024-07-25)"
...
level=info msg="✅ Block Archiving Completed!"
level=info msg="  Total Blocks:        XXX"
level=info msg="  Processed:           XXX"
level=info msg="  Duration:            XXX"
```

**测试交易归档（模拟运行）:**

```bash
./bin/archive -conf configs -type transactions -age-months 6 -batch-size 100 -dry-run
```

**测试两者（模拟运行）:**

```bash
./bin/archive -conf configs -type all -age-months 6 -batch-size 100 -dry-run
```

### 第4步：验证 Dry-Run 结果

在 dry-run 后，检查数据库（应该没有任何变化）：

```sql
-- 确认没有数据被修改
SELECT COUNT(*) FROM blocks WHERE archived = 1;
-- 应该与执行前相同

SELECT COUNT(*) FROM block_data_archives;
-- 应该与执行前相同
```

### 第5步：执行实际归档

**小批量测试（推荐）:**

```bash
# 先用小批量测试（比如100个区块）
./bin/archive -conf configs -type blocks -age-months 6 -batch-size 100
```

**大规模归档:**

```bash
# 确认小批量测试成功后，执行大规模归档
./bin/archive -conf configs -type blocks -age-months 6 -batch-size 1000
```

**归档区块和交易:**

```bash
./bin/archive -conf configs -type all -age-months 6 -batch-size 1000
```

---

## 验证归档结果

### 1. 检查归档统计

```sql
-- 查看归档进度
SELECT
    COUNT(*) as total_blocks,
    SUM(CASE WHEN archived = 1 THEN 1 ELSE 0 END) as archived_blocks,
    SUM(CASE WHEN archived = 0 OR archived IS NULL THEN 1 ELSE 0 END) as active_blocks,
    ROUND(SUM(CASE WHEN archived = 1 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as archive_percent
FROM blocks;

-- 查看归档表数据
SELECT
    COUNT(*) as total_archived,
    SUM(CASE WHEN txs_storage_key IS NOT NULL AND txs_storage_key != '' THEN 1 ELSE 0 END) as in_object_storage,
    SUM(CASE WHEN txs IS NOT NULL AND txs != '' THEN 1 ELSE 0 END) as in_database
FROM block_data_archives;
```

### 2. 验证特定区块

```sql
-- 选择一个已归档的区块进行验证
SELECT b.id, b.height, b.archived, b.txs_storage_key,
       a.txs as archived_txs,
       a.txs_storage_key as archive_storage_key
FROM blocks b
LEFT JOIN block_data_archives a ON b.id = a.block_id
WHERE b.archived = 1
LIMIT 5;
```

### 3. 测试查询功能

通过 API 或代码查询一个已归档的区块，确认 txs 数据能正确加载：

```go
// 测试代码示例
archiveService := service.NewArchiveService()

// 查询已归档区块
var block models.Block
db.Get().Where("height = ?", 12345).First(&block)

if block.Archived {
    fmt.Printf("Block %d is archived\n", block.Height)

    // 加载归档数据
    txs, err := archiveService.LoadArchivedBlockTxs(ctx, uint64(block.ID))
    if err != nil {
        log.Fatal("Failed to load archived txs:", err)
    }

    fmt.Printf("Loaded %d transactions from archive\n", len(txs))
}
```

---

## 性能监控

### 1. 数据库空间对比

```sql
-- 查看表大小（归档前后对比）
SELECT
    table_name AS 'Table',
    ROUND(((data_length + index_length) / 1024 / 1024), 2) AS 'Size (MB)'
FROM information_schema.TABLES
WHERE table_schema = DATABASE()
    AND table_name IN ('blocks', 'block_data_archives')
ORDER BY (data_length + index_length) DESC;
```

### 2. 查询性能测试

```sql
-- 测试查询未归档区块的性能
EXPLAIN SELECT * FROM blocks WHERE height = 12345 AND archived = 0;

-- 测试查询归档区块的性能（需要加载归档数据）
EXPLAIN SELECT * FROM blocks WHERE height = 67890 AND archived = 1;
```

---

## 故障排除

### 问题1: 归档工具无法连接数据库

**症状:**
```
level=warn msg="database connection attempt 1/10 failed: dial tcp [::1]:3306: connect: connection refused"
```

**解决方案:**
1. 确认 MySQL 服务正在运行
2. 检查 `configs/config.yaml` 中的数据库配置
3. 设置正确的环境变量（`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`）

### 问题2: block_data_archives 表不存在

**症状:**
```
Error 1146: Table 'explorer.block_data_archives' doesn't exist
```

**解决方案:**
```sql
-- 执行数据库初始化脚本
SOURCE database/init_database.sql;

-- 或手动创建表
CREATE TABLE `block_data_archives` (
  `block_id` bigint unsigned NOT NULL COMMENT '关联blocks.id',
  `txs` longtext COMMENT '区块交易列表（压缩后的JSON）',
  `txs_storage_key` varchar(255) DEFAULT NULL COMMENT 'txs字段的对象存储key',
  `archived_at` datetime(3) DEFAULT NULL COMMENT '归档时间',
  PRIMARY KEY (`block_id`),
  KEY `idx_block_data_archives_txs_storage_key` (`txs_storage_key`),
  KEY `idx_block_data_archives_archived_at` (`archived_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 问题3: blocks 表缺少 archived 字段

**解决方案:**
```sql
-- 添加归档相关字段
ALTER TABLE `blocks`
ADD COLUMN `archived` tinyint(1) DEFAULT '0' COMMENT '是否已归档',
ADD COLUMN `txs_storage_key` varchar(255) DEFAULT NULL COMMENT 'txs字段的对象存储key',
ADD INDEX `idx_blocks_archived_timestamp` (`archived`, `block_unix_timestamp`);
```

### 问题4: 归档数据无法加载

**症状:** 查询已归档区块时 txs 为空

**诊断:**
```sql
-- 检查归档记录
SELECT * FROM block_data_archives WHERE block_id = (
    SELECT id FROM blocks WHERE height = 12345
);

-- 检查对象存储配置
-- (通过应用日志或管理界面验证)
```

**解决方案:**
- 确认 `block_data_archives` 表中有对应记录
- 检查对象存储配置是否正确
- 验证 `txs_storage_key` 指向的文件是否存在

---

## 成功标准

归档测试成功的标准：

✅ **数据完整性**
- 所有归档区块的 `archived` 字段为 `1`
- `block_data_archives` 表中每个归档区块都有对应记录
- 可以通过 `LoadArchivedBlockTxs` 正确加载归档数据

✅ **空间优化**
- `blocks` 表大小明显减少（txs 字段已清空）
- 如果启用了对象存储，大部分数据在对象存储中

✅ **查询功能**
- 查询归档区块时能正确加载 txs 数据
- API 响应正常，数据完整

✅ **无数据丢失**
- 归档前后的区块总数一致
- 每个区块的交易数量正确

---

## 下一步

测试通过后，可以：

1. **设置定时归档**: 在 `configs/config.yaml` 中配置调度器
2. **监控归档进度**: 定期检查归档百分比
3. **优化归档策略**: 根据实际情况调整 `age_months` 和 `batch_size`
4. **数据生命周期管理**: 考虑分区表和对象存储生命周期策略

---

## 示例输出

**成功的归档输出示例:**

```
level=info msg="Starting archive tool..."
level=info msg="Configuration loaded from: configs"
level=info msg="Archive configuration:"
level=info msg="  - Archive type: blocks"
level=info msg="  - Age months: 6"
level=info msg="  - Batch size: 1000"
level=info msg="  - Dry run: false"
level=info msg="  - Use object storage: true"
level=info msg="  - Storage threshold: 3072 bytes"
level=info msg="==========================================="
level=info msg="📦 Starting Block Archiving..."
level=info msg="found 50000 blocks to archive (older than 2024-07-25)"
level=info msg="processing batch 1 with 1000 blocks"
level=info msg="completed batch 1: processed=1000, failed=0, total=1000"
level=info msg="processing batch 2 with 1000 blocks"
level=info msg="completed batch 2: processed=2000, failed=0, total=2000"
...
level=info msg="✅ Block Archiving Completed!"
level=info msg="  Total Blocks:        50000"
level=info msg="  Processed:           50000"
level=info msg="  Failed:              0"
level=info msg="  Compressed:          50000"
level=info msg="  Uploaded to Storage: 48500"
level=info msg="  Duration:            15m32s"
level=info msg="  Archive Progress:    50000 / 100000 (50.00%)"
level=info msg="==========================================="
level=info msg="✅ All operations completed successfully!"
```
