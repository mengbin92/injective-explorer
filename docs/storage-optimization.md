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
3. ✅ **实施数据归档**（方案二）
   - 创建归档表
   - 实现归档脚本
   - 修改查询逻辑支持归档数据

4. ✅ **实施分区表**（方案三）
   - 按时间或区块高度分区
   - 迁移现有数据

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

## 注意事项

1. **向后兼容**：压缩/归档后的数据需要能正确读取旧数据
2. **性能影响**：压缩/解压会增加 CPU 开销，需要测试
3. **备份策略**：归档前确保有完整备份
4. **监控告警**：设置存储空间监控，及时告警

