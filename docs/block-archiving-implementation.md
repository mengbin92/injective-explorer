# 区块归档功能实现说明

## 概述

区块归档功能允许将旧区块的 `txs` 字段（交易列表）从主表移至归档表和对象存储，从而减少数据库大小并提升查询性能。

该功能与现有的交易归档功能类似，采用统一的归档策略。

---

## 数据库变更

### 1. 修改 `blocks` 表

在 `database/init_database.sql` 中添加了以下字段：

```sql
ALTER TABLE `blocks`
ADD COLUMN `archived` TINYINT(1) DEFAULT '0' COMMENT '是否已归档（0=未归档，1=已归档）',
ADD COLUMN `txs_storage_key` VARCHAR(255) DEFAULT NULL COMMENT 'txs字段的对象存储key，格式: blocks/archive/txs/{height}',
ADD INDEX `idx_blocks_archived_timestamp` (`archived`, `block_unix_timestamp`);
```

### 2. 创建 `block_data_archives` 表

```sql
CREATE TABLE `block_data_archives` (
  `block_id` BIGINT UNSIGNED NOT NULL COMMENT '关联blocks.id',
  `txs` LONGTEXT COMMENT '区块交易列表（压缩后的JSON）',
  `txs_storage_key` VARCHAR(255) DEFAULT NULL COMMENT 'txs字段的对象存储key（如果已上传到对象存储）',
  `archived_at` DATETIME(3) DEFAULT NULL COMMENT '归档时间',
  PRIMARY KEY (`block_id`),
  KEY `idx_block_data_archives_txs_storage_key` (`txs_storage_key`),
  KEY `idx_block_data_archives_archived_at` (`archived_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='区块数据归档表';
```

---

## 代码实现

### 1. 模型文件

#### `internal/models/block_archive.go` (新增)
```go
type BlockDataArchive struct {
    BlockID         uint64    `json:"block_id" gorm:"primaryKey;column:block_id"`
    Txs             string    `json:"txs" gorm:"column:txs"`
    TxsStorageKey   string    `json:"txs_storage_key" gorm:"column:txs_storage_key"`
    ArchivedAt      time.Time `json:"archived_at" gorm:"column:archived_at"`
}
```

#### `internal/models/block.go` (更新)
添加了归档相关字段：
```go
type Block struct {
    // ... 现有字段
    Archived      bool   `json:"archived" gorm:"column:archived;default:0"`
    TxsStorageKey string `json:"txs_storage_key" gorm:"column:txs_storage_key"`
}
```

### 2. 归档服务

#### `internal/service/archive.go` (新增方法)

**核心方法：**

1. **`ArchiveOldBlocks(ctx, config)`**
   - 归档旧区块数据
   - 参数：
     - `age_months`: 归档几个月前的区块（默认：6个月）
     - `batch_size`: 每批处理的区块数（默认：1000）
     - `use_object_storage`: 是否使用对象存储（默认：自动检测）
     - `storage_threshold`: 对象存储阈值（默认：3KB）
   - 返回：`BlockArchiveMetrics` 包含归档统计信息

2. **`LoadArchivedBlockTxs(ctx, blockID)`**
   - 加载归档区块的交易列表
   - 自动从对象存储或归档表加载数据
   - 自动解压缩数据

3. **`GetArchiveProgress(ctx)`**
   - 获取归档进度
   - 返回已归档区块数 / 总区块数

### 3. 查询逻辑

#### `internal/service/explorer.go` (更新)

添加了辅助函数 `loadArchivedBlockTxs`：
- 自动检测区块是否已归档
- 如果已归档且 `txs` 为空，自动从归档存储加载
- 在 `GetBlockByHeight` 中自动调用

---

## 使用方法

### 1. 数据库初始化

```bash
# 重新执行数据库初始化脚本
mysql -u username -p database_name < database/init_database.sql
```

### 2. 执行归档

#### 方法1：通过代码调用
```go
import "injective-explorer/internal/service"

// 创建归档服务
archiveService := service.NewArchiveService()

// 使用默认配置（6个月前的数据）
config := service.DefaultArchiveConfig()
metrics, err := archiveService.ArchiveOldBlocks(context.Background(), config)
if err != nil {
    log.Fatal(err)
}

// 打印统计信息
log.Printf("归档完成: 处理=%d, 失败=%d, 耗时=%v",
    metrics.ProcessedBlocks, metrics.FailedBlocks, metrics.Duration)
```

#### 方法2：自定义配置
```go
config := &service.ArchiveConfig{
    ArchiveAgeMonths: 12,       // 归档12个月前的数据
    BatchSize:        500,       // 每批处理500个区块
    DryRun:           false,     // 实际执行（true=仅模拟）
    UseObjectStorage: true,      // 使用对象存储
    StorageThreshold: 10 * 1024, // 10KB阈值
}

metrics, err := archiveService.ArchiveOldBlocks(context.Background(), config)
```

### 3. 定时归档（通过调度器）

区块归档与交易归档使用相同的调度器配置。在 `configs/config.yaml` 中配置：

```yaml
archive:
  enabled: true
  age_months: 6
  batch_size: 1000
  storage_threshold: 10240  # 10KB
  use_object_storage: true
  interval: 24h  # 每24小时执行一次
```

调度器会自动执行区块归档和交易归档。

### 4. 查询归档数据

查询逻辑已自动集成，无需修改代码：

```go
// 查询区块（自动加载归档数据）
block, err := getBlockByHeight(12345)
if block.Archived {
    // txs 数据已自动加载
    fmt.Printf("区块 %d 包含 %d 笔交易\n", block.Height, len(block.Txs))
}
```

---

## 归档策略

### 存储策略

| 数据大小 | 存储位置 | 说明 |
|---------|---------|------|
| ≤ 3KB | `block_data_archives.txs` | 存储在数据库（压缩后）|
| > 3KB | 对象存储 + `txs_storage_key` | 存储在对象存储（MinIO/S3/OSS/COS）|

### 数据流程

```
原始区块 (blocks 表)
├── txs: ["0xabc...", "0xdef...", ...]  ← JSON 数组
│
↓ 归档操作
│
├── archived: true
├── txs: NULL                            ← 主表清空
├── txs_storage_key: "blocks/archive/txs/12345"
│
↓
├── block_data_archives 表
│   ├── block_id: 12345
│   ├── txs: "[压缩后的JSON]" 或 NULL
│   └── txs_storage_key: "blocks/archive/txs/12345"
│
└── 对象存储 (如果启用)
    └── blocks/archive/txs/12345  ← 压缩后的数据
```

---

## 监控和验证

### 1. 查询归档状态

```sql
-- 查看归档进度
SELECT
    COUNT(*) as total_blocks,
    SUM(CASE WHEN archived = 1 THEN 1 ELSE 0 END) as archived_blocks,
    SUM(CASE WHEN archived = 0 OR archived IS NULL THEN 1 ELSE 0 END) as active_blocks,
    ROUND(SUM(CASE WHEN archived = 1 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as archive_percent
FROM blocks;

-- 查看归档表大小
SELECT
    COUNT(*) as archived_blocks,
    SUM(CASE WHEN txs_storage_key IS NOT NULL AND txs_storage_key != '' THEN 1 ELSE 0 END) as in_object_storage,
    SUM(CASE WHEN txs IS NOT NULL AND txs != '' THEN 1 ELSE 0 END) as in_database
FROM block_data_archives;
```

### 2. 验证归档数据

```sql
-- 检查是否有遗漏（已归档但没有归档记录）
SELECT b.id, b.height, b.archived, a.block_id
FROM blocks b
LEFT JOIN block_data_archives a ON b.id = a.block_id
WHERE b.archived = 1 AND a.block_id IS NULL;

-- 检查对象存储文件（通过 txs_storage_key）
SELECT block_id, txs_storage_key
FROM block_data_archives
WHERE txs_storage_key IS NOT NULL AND txs_storage_key != ''
LIMIT 10;
```

### 3. 性能对比

```sql
-- 查询未归档区块（主表数据完整）
EXPLAIN SELECT * FROM blocks WHERE height = 12345 AND archived = 0;

-- 查询归档区块（需要加载归档数据）
EXPLAIN SELECT * FROM blocks WHERE height = 12345 AND archived = 1;
```

---

## 注意事项

1. **对象存储依赖**
   - 对象存储（MinIO/S3/OSS/COS）必须正确配置
   - 如果未启用对象存储，数据将存储在 `block_data_archives.txs` 字段

2. **查询性能**
   - 归档区块的 `txs` 数据需要额外查询，查询时间会略有增加
   - 建议为 `archived` 和 `block_unix_timestamp` 字段创建复合索引

3. **数据恢复**
   - 归档数据可以从 `block_data_archives` 表和对象存储中恢复
   - 如果误删归档记录，可以从对象存储恢复（如果有 `txs_storage_key`）

4. **并发安全**
   - 归档操作使用数据库事务，确保原子性
   - 失败时会自动回滚，不会留下不一致数据

5. **增量归档**
   - 每次归档只会处理 `archived = 0` 的区块
   - 可以多次执行归档操作，不会重复处理已归档区块

---

## 故障排除

### 1. 归档失败

**问题**: 归档过程中部分区块失败

**解决方案**:
```go
// 查看失败的区块
metrics, _ := archiveService.ArchiveOldBlocks(ctx, config)
log.Printf("失败的区块数: %d", metrics.FailedBlocks)

// 重新执行归档（只会处理未归档的区块）
metrics, _ := archiveService.ArchiveOldBlocks(ctx, config)
```

### 2. 归档数据无法加载

**问题**: 查询归档区块时 `txs` 为空

**解决方案**:
```sql
-- 检查归档记录是否存在
SELECT * FROM block_data_archives WHERE block_id = (SELECT id FROM blocks WHERE height = 12345);

-- 检查对象存储文件是否可访问
-- (需要通过对象存储管理工具或API验证)
```

### 3. 对象存储上传失败

**问题**: `txs_storage_key` 为空但 `txs` 字段也为空

**解决方案**:
```go
// 检查对象存储配置
storage := storage.Get()
if storage == nil {
    log.Fatal("对象存储未启用")
}

// 手动测试上传
err := storage.PutObject(ctx, "test", []byte("test data"))
if err != nil {
    log.Fatal("对象存储上传失败:", err)
}
```

---

## 后续优化

### 1. 并行处理
当前是顺序处理批次，可以考虑添加并行处理：
```go
// 类似 sync service 的 worker pool
workers := 5
var wg sync.WaitGroup
batchChan := make(chan []*models.Block, workers*2)

for i := 0; i < workers; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for batch := range batchChan {
            archiveBlockBatch(batch)
        }
    }()
}
```

### 2. 分区表集成
将 `blocks` 表按时间分区，删除旧分区前先归档：
```sql
-- 删除6个月前的分区
ALTER TABLE blocks DROP PARTITION p202401;
```

### 3. 自动归档触发
在数据同步时自动触发归档检查，避免手动执行。

---

## 相关文件

| 文件 | 说明 |
|------|------|
| `database/init_database.sql` | 数据库表结构定义 |
| `internal/models/block_archive.go` | 归档模型（新增）|
| `internal/models/block.go` | 区块模型（更新）|
| `internal/service/archive.go` | 归档服务（新增区块归档方法）|
| `internal/service/explorer.go` | 查询逻辑（更新自动加载）|
| `internal/conf/conf.proto` | 配置定义（共享 Archive 配置）|

---

## 总结

区块归档功能已完全集成到现有归档系统中：

- ✅ 数据库表结构已更新
- ✅ 归档逻辑已实现
- ✅ 查询逻辑已集成自动加载
- ✅ 配置与交易归档共享
- ✅ 代码编译通过

**下一步**: 执行数据库初始化脚本，然后可以通过代码或调度器开始归档区块数据。
