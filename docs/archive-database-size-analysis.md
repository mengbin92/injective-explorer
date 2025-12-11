# 归档对数据库体积的影响分析

## 归档流程

归档操作会执行以下步骤：

1. **创建归档记录**：在 `transaction_data_archives` 表中创建记录
2. **处理大字段**：
   - 如果启用对象存储且数据超过阈值（默认10KB），上传到对象存储
   - 如果未启用对象存储或数据小于阈值，存储在 `transaction_data_archives` 表中
3. **更新原记录**：将 `transactions` 表中的大字段设置为 NULL

## 数据库体积变化分析

### 情况1：启用对象存储且数据成功上传 ✅ **数据库体积会减小**

**transactions 表变化**：
- `data` 字段：从大文本 → NULL（体积减小）
- `messages` 字段：从大文本 → NULL（体积减小）
- `logs` 字段：从大文本 → NULL（体积减小）
- `info` 字段：从文本 → NULL（体积减小）
- `error_log` 字段：从文本 → NULL（体积减小）

**transaction_data_archives 表变化**：
- 只存储存储键（很小的字符串，通常 < 200 字节）
- `data_storage_key`、`messages_storage_key`、`logs_storage_key` 字段
- 大字段为空（`data`、`messages`、`logs` 为空字符串）

**结果**：
- ✅ `transactions` 表体积显著减小（大字段被清空）
- ✅ `transaction_data_archives` 表体积很小（只存储存储键）
- ✅ **数据库总体积减小**

### 情况2：未启用对象存储或数据小于阈值 ⚠️ **数据库体积可能不会减小**

**transactions 表变化**：
- 大字段被设置为 NULL（体积减小）

**transaction_data_archives 表变化**：
- 存储完整的大字段数据（`data`、`messages`、`logs` 等）
- 数据从 `transactions` 表移动到 `transaction_data_archives` 表

**结果**：
- ✅ `transactions` 表体积减小
- ❌ `transaction_data_archives` 表体积增加（存储了完整数据）
- ⚠️ **数据库总体积可能不会减小**（数据只是移动了位置）

### 情况3：部分数据上传到对象存储 📊 **数据库体积部分减小**

如果某些字段超过阈值（上传到对象存储），某些字段未超过阈值（存储在数据库）：

**结果**：
- ✅ 上传到对象存储的字段：数据库体积减小
- ⚠️ 未上传的字段：数据移动到 `transaction_data_archives` 表，总体积可能不变

## 实际效果

### 启用对象存储的情况（推荐）

假设一条交易记录：
- `data` 字段：50KB
- `messages` 字段：30KB
- `logs` 字段：20KB
- `info` 字段：1KB
- `error_log` 字段：0.5KB

**归档前（transactions 表）**：
- 总大小：~101.5KB

**归档后（启用对象存储）**：
- `transactions` 表：大字段为 NULL，只保留元数据（~2KB）
- `transaction_data_archives` 表：只存储存储键（~200 字节）
- **节省空间**：~99.3KB（约 98%）

### 未启用对象存储的情况

**归档前（transactions 表）**：
- 总大小：~101.5KB

**归档后（未启用对象存储）**：
- `transactions` 表：大字段为 NULL（~2KB）
- `transaction_data_archives` 表：存储完整数据（~101.5KB）
- **节省空间**：0KB（数据只是移动了位置）

## 建议

### 1. 启用对象存储（推荐）

```yaml
archive:
  use_object_storage: true
  storage_threshold: 10240  # 10KB
```

这样可以：
- ✅ 显著减小数据库体积
- ✅ 提高数据库查询性能
- ✅ 降低数据库存储成本

### 2. 监控数据库体积变化

归档后可以执行以下 SQL 检查体积变化：

```sql
-- 检查 transactions 表大小
SELECT 
    table_name AS '表名',
    ROUND(((data_length + index_length) / 1024 / 1024), 2) AS '大小(MB)'
FROM information_schema.TABLES
WHERE table_schema = 'explorer'
  AND table_name = 'transactions';

-- 检查 transaction_data_archives 表大小
SELECT 
    table_name AS '表名',
    ROUND(((data_length + index_length) / 1024 / 1024), 2) AS '大小(MB)'
FROM information_schema.TABLES
WHERE table_schema = 'explorer'
  AND table_name = 'transaction_data_archives';

-- 检查归档数据中存储键的使用情况
SELECT 
    COUNT(*) as total_records,
    SUM(CASE WHEN data_storage_key IS NOT NULL AND data_storage_key != '' THEN 1 ELSE 0 END) as data_in_storage,
    SUM(CASE WHEN messages_storage_key IS NOT NULL AND messages_storage_key != '' THEN 1 ELSE 0 END) as messages_in_storage,
    SUM(CASE WHEN logs_storage_key IS NOT NULL AND logs_storage_key != '' THEN 1 ELSE 0 END) as logs_in_storage,
    SUM(CASE WHEN data_storage_key IS NULL OR data_storage_key = '' THEN LENGTH(data) ELSE 0 END) as data_in_db_size,
    SUM(CASE WHEN messages_storage_key IS NULL OR messages_storage_key = '' THEN LENGTH(messages) ELSE 0 END) as messages_in_db_size,
    SUM(CASE WHEN logs_storage_key IS NULL OR logs_storage_key = '' THEN LENGTH(logs) ELSE 0 END) as logs_in_db_size
FROM transaction_data_archives;
```

### 3. 优化存储阈值

根据实际情况调整 `storage_threshold`：

- **较小的阈值**（如 5KB）：更多数据上传到对象存储，数据库体积减小更多
- **较大的阈值**（如 50KB）：只有非常大的数据才上传，数据库体积减小较少

## 总结

**回答：归档到对象存储后，数据库体积会减小吗？**

- ✅ **启用对象存储**：**会显著减小**（通常可减少 80-98% 的大字段数据）
- ⚠️ **未启用对象存储**：**不会减小**（数据只是从 `transactions` 表移动到 `transaction_data_archives` 表）

**建议**：始终启用对象存储以获得最佳的数据库体积优化效果。
