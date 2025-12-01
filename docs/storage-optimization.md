# 区块链浏览器数据存储优化方案

## 问题分析

根据实际测试，同步主网数据每天需要 **70-100GB** 的硬盘存储。这是一个严重的存储成本问题。

### 当前存储结构分析

从代码分析，主要存储的数据包括：

1. **Block 表**：区块基本信息（相对较小）
2. **Transaction 表**：交易数据（**主要存储占用**）
   - `Data` (string): 交易原始数据，可能包含大量二进制数据（base64编码）
   - `Messages` (string): 交易消息，可能很大
   - `Logs` (string): 执行日志，可能很大
   - `Events` (JSON): 事件数组，可能包含大量事件
   - `Info`, `ErrorLog` 等字段
3. **Account 表**：账户信息
4. **Validator 表**：验证者信息
5. **其他相关表**：Staking、DelegationReward 等

## 主流区块链浏览器的解决方案

### 1. Etherscan 的优化策略

#### 1.1 数据分层存储
- **热数据**（最近30-90天）：存储在高速SSD，完整数据
- **温数据**（3-12个月）：压缩存储，部分字段归档
- **冷数据**（1年以上）：高度压缩或归档到对象存储（S3/OSS）

#### 1.2 数据压缩
- 使用 **gzip/lz4** 压缩大字段（Messages, Logs, Data）
- JSON字段使用更紧凑的格式
- 二进制数据使用更高效的编码方式

#### 1.3 数据归档策略
- **定期归档**：将旧数据（如6个月以上）的大字段分离到归档表
- **只保留关键字段**：归档后只保留 hash, block_number, timestamp 等查询必需字段
- **按需加载**：查询时按需从归档存储加载完整数据

#### 1.4 数据库优化
- **分区表**：按时间或区块高度分区
- **索引优化**：只对查询频繁的字段建立索引
- **列式存储**：对分析型查询使用列式存储（如 ClickHouse）

#### 1.5 存储分离
- **关系型数据库**：只存储结构化数据和索引
- **对象存储**：大字段（Messages, Logs）存储在对象存储，数据库只存引用
- **CDN/缓存**：热点数据缓存到 Redis/CDN

## 针对当前项目的优化方案

### 方案一：数据压缩（快速实施，效果明显）

#### 1.1 字段级压缩
对大字段进行压缩存储：

```go
// 在 models/transaction.go 中添加压缩功能
import (
    "compress/gzip"
    "bytes"
    "encoding/base64"
)

// CompressData 压缩数据
func CompressData(data []byte) ([]byte, error) {
    if len(data) == 0 {
        return nil, nil
    }
    
    var buf bytes.Buffer
    writer := gzip.NewWriter(&buf)
    if _, err := writer.Write(data); err != nil {
        return nil, err
    }
    if err := writer.Close(); err != nil {
        return nil, err
    }
    
    return buf.Bytes(), nil
}

// DecompressData 解压数据
func DecompressData(compressed []byte) ([]byte, error) {
    if len(compressed) == 0 {
        return nil, nil
    }
    
    reader, err := gzip.NewReader(bytes.NewReader(compressed))
    if err != nil {
        return nil, err
    }
    defer reader.Close()
    
    return io.ReadAll(reader)
}
```

#### 1.2 修改存储逻辑
在 `TransactionFromProto` 中压缩大字段：

```go
// 压缩 Messages, Logs, Data
if len(detail.Messages) > 1024 { // 只压缩大于1KB的数据
    compressed, err := CompressData(detail.Messages)
    if err == nil {
        messagesStr = base64.StdEncoding.EncodeToString(compressed)
        // 添加压缩标记
        messagesStr = "gzip:" + messagesStr
    }
}
```

**预期效果**：可减少 60-80% 的存储空间

### 方案二：数据归档（中期方案）

#### 2.1 创建归档表结构

```sql
-- 交易归档表（只保留关键字段）
CREATE TABLE transactions_archive (
    id BIGINT PRIMARY KEY,
    hash VARCHAR(255) UNIQUE NOT NULL,
    block_number BIGINT NOT NULL,
    block_timestamp VARCHAR(255),
    tx_type VARCHAR(100),
    gas_used BIGINT,
    gas_wanted BIGINT,
    code INT,
    created_at TIMESTAMP,
    INDEX idx_block_number (block_number),
    INDEX idx_hash (hash),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 交易详细数据归档表（存储大字段）
CREATE TABLE transaction_data_archive (
    transaction_id BIGINT PRIMARY KEY,
    data LONGBLOB,  -- 压缩后的数据
    messages LONGBLOB,
    logs LONGBLOB,
    events LONGBLOB,
    info TEXT,
    error_log TEXT,
    FOREIGN KEY (transaction_id) REFERENCES transactions_archive(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

#### 2.2 归档策略
- **归档时间点**：数据超过6个月自动归档
- **归档流程**：
  1. 将大字段移动到 `transaction_data_archive`
  2. 在 `transactions` 表中只保留关键字段
  3. 添加 `archived` 标记字段

**预期效果**：主表存储减少 70-90%，查询性能提升

### 方案三：分区表（长期方案）

#### 3.1 MySQL 分区表

```sql
-- 按区块高度分区（每100万区块一个分区）
ALTER TABLE transactions 
PARTITION BY RANGE (block_number) (
    PARTITION p0 VALUES LESS THAN (1000000),
    PARTITION p1 VALUES LESS THAN (2000000),
    PARTITION p2 VALUES LESS THAN (3000000),
    -- ... 更多分区
    PARTITION p_max VALUES LESS THAN MAXVALUE
);

-- 按时间分区（每月一个分区）
ALTER TABLE transactions 
PARTITION BY RANGE (UNIX_TIMESTAMP(created_at)) (
    PARTITION p202401 VALUES LESS THAN (UNIX_TIMESTAMP('2024-02-01')),
    PARTITION p202402 VALUES LESS THAN (UNIX_TIMESTAMP('2024-03-01')),
    -- ... 更多分区
    PARTITION p_future VALUES LESS THAN MAXVALUE
);
```

**优势**：
- 查询时只扫描相关分区
- 可以快速删除旧分区
- 维护更方便

### 方案四：存储分离（高级方案）

#### 4.1 对象存储方案
- **大字段存储**：将 Messages, Logs, Data 存储到对象存储（MinIO/S3/OSS）
- **数据库只存引用**：在数据库中只存储对象存储的 key/URL

```go
type Transaction struct {
    // ... 其他字段
    MessagesStorageKey string `gorm:"column:messages_storage_key"` // 对象存储key
    LogsStorageKey     string `gorm:"column:logs_storage_key"`
    DataStorageKey     string `gorm:"column:data_storage_key"`
}
```

**优势**：
- 数据库体积大幅减少
- 对象存储成本更低
- 可以设置生命周期策略自动归档

### 方案五：数据清理策略

#### 5.1 选择性存储
- **不存储失败交易的详细日志**：只存储错误码和简要错误信息
- **不存储空字段**：NULL 不占用空间
- **事件去重**：相同类型的事件只存储一次

#### 5.2 定期清理
```sql
-- 删除超过1年的失败交易详细日志
UPDATE transactions 
SET logs = NULL, error_log = SUBSTRING(error_log, 1, 500)
WHERE code != 0 
AND created_at < DATE_SUB(NOW(), INTERVAL 1 YEAR);

-- 压缩旧数据
OPTIMIZE TABLE transactions;
```

## 实施建议

### 短期（1-2周）
1. ✅ **实施数据压缩**（方案一）**已完成**
   - ✅ 创建 `compression.go` 实现压缩/解压功能
   - ✅ 修改 `TransactionFromProto` 添加压缩逻辑
   - ✅ 修改 `TransactionToProto` 添加解压逻辑
   - ✅ 添加完整的单元测试
   - ✅ 预期减少 60-80% 存储
   - 📖 详细说明见 [compression-implementation.md](./compression-implementation.md)

2. ✅ **优化字段类型**
   - 将 TEXT 字段改为合适的 VARCHAR 长度
   - 添加字段长度限制

### 中期（1-2个月）
3. ✅ **实施数据归档**（方案二）**已完成**
   - ✅ 创建归档表模型（TransactionArchive和TransactionDataArchive）
   - ✅ 创建归档表SQL迁移脚本（支持MySQL和PostgreSQL）
   - ✅ 实现归档服务（ArchiveService）
   - ✅ 修改查询逻辑支持从归档表查询数据
   - ✅ 添加archived标记字段到Transaction模型
   - 📖 详细说明见下方"数据归档实施"

4. ✅ **实施分区表**（方案三）**已完成**
   - ✅ 创建分区表SQL脚本（按区块高度和时间分区）
   - ✅ 支持MySQL和PostgreSQL
   - 📖 详细说明见下方"分区表实施"

### 长期（3-6个月）
5. ✅ **存储分离**（方案四）
   - 集成对象存储
   - 迁移大字段到对象存储

6. ✅ **数据清理策略**（方案五）
   - 实现自动清理脚本
   - 设置定期任务

## 预期效果

| 方案     | 存储减少 | 实施难度 | 推荐优先级 |
| -------- | -------- | -------- | ---------- |
| 数据压缩 | 60-80%   | 低       | ⭐⭐⭐⭐⭐      |
| 数据归档 | 70-90%   | 中       | ⭐⭐⭐⭐       |
| 分区表   | 20-30%   | 中       | ⭐⭐⭐        |
| 存储分离 | 80-95%   | 高       | ⭐⭐⭐⭐       |
| 数据清理 | 10-20%   | 低       | ⭐⭐⭐        |

## 参考资源

- [Etherscan 技术架构分析](https://etherscan.io/apis)
- [MySQL 分区表最佳实践](https://dev.mysql.com/doc/refman/8.0/en/partitioning.html)
- [数据压缩算法对比](https://github.com/facebook/zstd)
- [区块链数据存储优化案例](https://github.com/ethereum/go-ethereum)

## 数据归档实施

### 已实现功能

1. **归档表结构**
   - `transaction_archives`: 存储归档交易的关键字段
   - `transaction_data_archives`: 存储归档交易的大字段（Data, Messages, Logs）

2. **归档服务** (`internal/service/archive.go`)
   - `ArchiveOldTransactions`: 归档超过指定年龄的交易
   - `GetArchivedTransaction`: 从归档表查询单个交易
   - `GetArchivedTransactions`: 从归档表查询交易列表（支持分页）

3. **查询逻辑增强**
   - `GetTransactionByHash` 方法已支持自动从归档表查询
   - 如果主表未找到，会自动查询归档表

### 使用方法

#### 1. 创建归档表

执行SQL脚本创建归档表：
```bash
# MySQL
mysql -u user -p database < database/archive_tables.sql

# PostgreSQL
psql -U user -d database -f database/archive_tables.sql
```

或者使用GORM自动迁移（已在 `global.go` 中配置）：
```go
// 归档表会自动创建
db.Get().AutoMigrate(
    &models.TransactionArchive{},
    &models.TransactionDataArchive{},
)
```

#### 2. 执行归档

```go
import "injective-explorer/internal/service"

// 创建归档服务
archiveService := service.NewArchiveService()

// 归档超过6个月的交易（默认配置）
config := service.DefaultArchiveConfig()
err := archiveService.ArchiveOldTransactions(ctx, config)

// 自定义配置
customConfig := &service.ArchiveConfig{
    ArchiveAgeMonths: 6,  // 归档6个月前的数据
    BatchSize:        1000, // 每批处理1000条
    DryRun:           false, // false=实际执行，true=仅模拟
}
err := archiveService.ArchiveOldTransactions(ctx, customConfig)
```

#### 3. 查询归档数据

查询逻辑已自动支持归档数据，无需额外代码。如果主表未找到，会自动查询归档表。

手动查询归档数据：
```go
archiveService := service.NewArchiveService()

// 查询单个归档交易
tx, err := archiveService.GetArchivedTransaction(ctx, "0x...")

// 查询归档交易列表（分页）
transactions, total, err := archiveService.GetArchivedTransactions(ctx, 100, 0)
```

#### 4. 定期归档（建议使用cron任务）

```bash
# 每天凌晨2点执行归档
0 2 * * * /path/to/injective-explorer archive --age-months=6 --batch-size=1000
```

### 归档策略建议

- **归档时间点**：建议归档6个月前的数据
- **归档频率**：建议每周或每月执行一次
- **批量大小**：根据数据库性能调整，默认1000条/批
- **监控**：归档前后监控数据库存储空间变化

## 分区表实施

### 已实现功能

1. **分区表SQL脚本** (`database/partition_tables.sql`)
   - 支持按区块高度分区（每100万区块一个分区）
   - 支持按时间分区（每月一个分区）
   - 支持MySQL和PostgreSQL

### 使用方法

#### 1. 创建分区表

**注意**：分区表需要在创建表时定义，不能对现有表直接添加分区。建议先创建新分区表，然后迁移数据。

**MySQL - 按区块高度分区**：
```sql
-- 参考 database/partition_tables.sql 中的MySQL版本
-- 创建分区表
CREATE TABLE transactions_partitioned (
    -- 表结构...
) PARTITION BY RANGE (block_number) (
    PARTITION p0 VALUES LESS THAN (1000000),
    PARTITION p1 VALUES LESS THAN (2000000),
    -- ...
    PARTITION p_max VALUES LESS THAN MAXVALUE
);
```

**PostgreSQL - 按时间分区**：
```sql
-- 参考 database/partition_tables.sql 中的PostgreSQL版本
CREATE TABLE transactions_partitioned_by_time (
    -- 表结构...
) PARTITION BY RANGE (created_at);

-- 创建分区
CREATE TABLE transactions_partitioned_by_time_202401 PARTITION OF transactions_partitioned_by_time
    FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');
```

#### 2. 迁移数据

```sql
-- 1. 迁移现有数据到分区表
INSERT INTO transactions_partitioned 
SELECT * FROM transactions;

-- 2. 验证数据完整性
SELECT COUNT(*) FROM transactions;
SELECT COUNT(*) FROM transactions_partitioned;

-- 3. 重命名表
RENAME TABLE transactions TO transactions_old;
RENAME TABLE transactions_partitioned TO transactions;

-- 4. 删除旧表（确认无误后）
DROP TABLE transactions_old;
```

#### 3. 管理分区

**添加新分区**：
```sql
-- MySQL
ALTER TABLE transactions_partitioned 
ADD PARTITION (PARTITION p10 VALUES LESS THAN (11000000));

-- PostgreSQL
CREATE TABLE transactions_partitioned_p10 PARTITION OF transactions_partitioned
    FOR VALUES FROM (10000000) TO (11000000);
```

**删除旧分区**（会删除分区中的所有数据）：
```sql
-- MySQL
ALTER TABLE transactions_partitioned DROP PARTITION p0;

-- PostgreSQL
DROP TABLE transactions_partitioned_p0;
```

### 分区策略建议

- **按区块高度分区**：适合区块高度增长稳定的场景
- **按时间分区**：适合需要定期清理旧数据的场景
- **分区大小**：建议每个分区不超过1000万条记录
- **分区数量**：建议不超过100个分区

## 注意事项

1. **向后兼容**：压缩/归档后的数据需要能正确读取旧数据
2. **性能影响**：压缩/解压会增加 CPU 开销，需要测试
3. **备份策略**：归档前确保有完整备份
4. **监控告警**：设置存储空间监控，及时告警
5. **归档时机**：建议在业务低峰期执行归档操作
6. **分区表限制**：分区表不支持外键约束，需要特别注意
7. **数据迁移**：分区表迁移需要停机或使用双写策略

