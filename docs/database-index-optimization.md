# 数据库索引优化文档

## 概述

本文档记录了为提升数据库查询性能而创建的索引。这些索引基于对项目中所有数据库查询操作的全面分析。

## 迁移文件

- **迁移文件**: `scripts/migrations/004_add_comprehensive_query_indexes.sql`
- **创建日期**: 2025-01-XX
- **数据库**: MySQL 5.6+ (兼容 InnoDB)

## 索引列表

### 1. Transaction 表索引

#### 1.1 `idx_transaction_to`
- **字段**: `to`
- **用途**: 优化 `enhanceBlockDTOWithTransactions` 函数中的查询
- **查询模式**: `SELECT hash, signer, to, value FROM transactions WHERE block_number = ?`
- **性能影响**: 提升按区块号查询交易时的性能

#### 1.2 `idx_data_storage_key`, `idx_messages_storage_key`, `idx_logs_storage_key`, `idx_evm_details_storage_key`
- **字段**: `data_storage_key`, `messages_storage_key`, `logs_storage_key`, `evm_details_storage_key`
- **用途**: 优化存储迁移查询
- **查询模式**: `WHERE (data_storage_key IS NULL OR data_storage_key = '') AND ...`
- **性能影响**: 加速存储迁移过程中的批量查询

#### 1.3 `idx_archived_timestamp_tx`
- **字段**: `archived`, `block_unix_timestamp DESC`, `tx_number DESC`
- **用途**: 优化归档交易查询
- **查询模式**: `WHERE archived = ? ORDER BY block_unix_timestamp DESC, tx_number DESC`
- **性能影响**: 显著提升归档交易列表查询性能

#### 1.4 `idx_evm_hash_synced`
- **字段**: `evm_hash`, `evm_details_synced`
- **用途**: 优化 EVM 交易同步状态查询
- **查询模式**: `WHERE evm_hash != '' AND evm_hash IS NOT NULL AND evm_details_synced = ?`
- **性能影响**: 提升 EVM 交易过滤和同步状态检查的性能

#### 1.5 `idx_message_types_prefix`
- **字段**: `message_types(255)` (前缀索引)
- **用途**: 优化 JSON 字段的 LIKE 查询
- **查询模式**: `WHERE message_types LIKE ? OR JSON_CONTAINS(message_types, ?)`
- **性能影响**: 提升消息类型过滤查询的性能

#### 1.6 `idx_evm_from_timestamp`, `idx_evm_to_timestamp`
- **字段**: `evm_from_address`, `block_unix_timestamp DESC`, `tx_number DESC` 和 `evm_to_address`, `block_unix_timestamp DESC`, `tx_number DESC`
- **用途**: 优化 EVM 账户交易查询
- **查询模式**: `WHERE (evm_from_address = ? OR evm_to_address = ?) ORDER BY block_unix_timestamp DESC`
- **性能影响**: 提升 EVM 账户交易列表查询和排序性能

### 2. AccountBalance 表索引

#### 2.1 `idx_account_balance_address_denom`
- **字段**: `address`, `denom`
- **用途**: 优化账户余额查询
- **查询模式**: `WHERE address = ? AND denom = ?`
- **性能影响**: 显著提升账户特定代币余额查询性能

### 3. ContractInfo 表索引

#### 3.1 `idx_contract_name_prefix`, `idx_contract_token_name_prefix`
- **字段**: `name(50)`, `token_name(50)` (前缀索引)
- **用途**: 优化合约名称模糊查询
- **查询模式**: `WHERE name LIKE ? OR token_name LIKE ?`
- **性能影响**: 提升合约搜索功能性能

#### 3.2 `idx_contract_name_fulltext`
- **字段**: `name`, `token_name` (全文索引)
- **用途**: 支持全文搜索
- **查询模式**: `MATCH(name, token_name) AGAINST(? IN NATURAL LANGUAGE MODE)`
- **性能影响**: 提供更高效的全文搜索能力

#### 3.3 `idx_contract_filters`
- **字段**: `chain_id`, `contract_type`, `verification_status`
- **用途**: 优化多条件过滤查询
- **查询模式**: `WHERE chain_id = ? AND contract_type = ? AND verification_status = ?`
- **性能影响**: 提升合约列表多条件过滤性能

## 查询分析来源

以下查询操作被分析并优化：

### Transaction 表查询
1. `WHERE hash = ?` - 已有索引 `hash`
2. `WHERE block_number = ?` - 已有索引 `idx_block_tx`
3. `WHERE block_unix_timestamp >= ?` - 已有索引 `idx_timestamp_tx`, `idx_timestamp_code`
4. `WHERE signer = ?` - 已有索引 `idx_signer`, `idx_signer_timestamp`
5. `WHERE evm_hash != '' AND evm_hash IS NOT NULL` - 已有索引 `idx_evm_hash`
6. `WHERE evm_from_address = ? OR evm_to_address = ?` - 已有索引 `idx_evm_from_address`, `idx_evm_to_address`
7. `WHERE archived = ?` - 已有索引 `archived`
8. `WHERE code > 0` - 已有索引 `idx_timestamp_code`
9. `WHERE message_types LIKE ?` - **新增索引** `idx_message_types_prefix`
10. `WHERE (data_storage_key IS NULL OR data_storage_key = '')` - **新增索引** `idx_data_storage_key`
11. `ORDER BY block_unix_timestamp DESC, tx_number DESC` - 已有索引，**新增复合索引**优化排序

### Block 表查询
1. `WHERE height = ?` - 已有索引 `idx_height`, `idx_height_desc`
2. `WHERE height < ?` - 已有索引覆盖
3. `WHERE height IN ?` - 已有索引覆盖
4. `ORDER BY height DESC` - 已有索引 `idx_height_desc`

### AccountBalance 表查询
1. `WHERE address = ?` - 已有索引 `address`
2. `WHERE address = ? AND denom = ?` - **新增复合索引** `idx_account_balance_address_denom`
3. `WHERE address IN ?` - 已有索引覆盖

### ContractInfo 表查询
1. `WHERE chain_id = ?` - 已有索引 `chain_id`
2. `WHERE contract_type = ?` - 已有索引 `contract_type`
3. `WHERE verification_status = ?` - 已有索引 `verification_status`
4. `WHERE name LIKE ? OR token_name LIKE ?` - **新增前缀索引**和全文索引
5. `WHERE token_symbol LIKE ?` - 已有索引 `idx_token_symbol`

### TransactionDataArchive 表查询
1. `WHERE transaction_id = ?` - 主键自动索引
2. `WHERE transaction_id IN ?` - 主键自动索引

## 性能预期

### 查询性能提升
- **归档交易查询**: 10-100x 提升（从全表扫描到索引扫描）
- **存储迁移查询**: 5-20x 提升
- **EVM 账户交易查询**: 5-50x 提升（取决于数据量）
- **合约搜索查询**: 3-10x 提升（全文索引）
- **账户余额查询**: 2-5x 提升

### 索引维护成本
- **存储空间**: 每个索引约占用表大小的 5-15%
- **写入性能**: INSERT/UPDATE 操作可能略微下降（5-10%），但查询性能提升显著
- **维护时间**: 索引创建时间取决于表大小，建议在低峰期执行

## 执行建议

### 1. 执行前检查
```sql
-- 检查表大小
SELECT 
    TABLE_NAME,
    ROUND(((DATA_LENGTH + INDEX_LENGTH) / 1024 / 1024), 2) AS 'Size (MB)'
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
AND TABLE_NAME IN ('transactions', 'blocks', 'account_balances', 'contract_infos');

-- 检查现有索引
SHOW INDEX FROM transactions;
SHOW INDEX FROM account_balances;
SHOW INDEX FROM contract_infos;
```

### 2. 执行迁移
```bash
# 在低峰期执行
mysql -u username -p database_name < scripts/migrations/004_add_comprehensive_query_indexes.sql
```

### 3. 验证索引
```sql
-- 验证索引创建成功
SHOW INDEX FROM transactions WHERE Key_name LIKE 'idx_%';
SHOW INDEX FROM account_balances WHERE Key_name LIKE 'idx_%';
SHOW INDEX FROM contract_infos WHERE Key_name LIKE 'idx_%';

-- 检查查询计划
EXPLAIN SELECT * FROM transactions WHERE archived = 1 ORDER BY block_unix_timestamp DESC LIMIT 100;
```

### 4. 监控性能
- 监控查询执行时间
- 检查慢查询日志
- 观察索引使用情况

## 回滚方案

如果需要回滚，执行以下 SQL：

```sql
DROP INDEX idx_transaction_to ON transactions;
DROP INDEX idx_data_storage_key ON transactions;
DROP INDEX idx_messages_storage_key ON transactions;
DROP INDEX idx_logs_storage_key ON transactions;
DROP INDEX idx_evm_details_storage_key ON transactions;
DROP INDEX idx_archived_timestamp_tx ON transactions;
DROP INDEX idx_evm_hash_synced ON transactions;
DROP INDEX idx_message_types_prefix ON transactions;
DROP INDEX idx_evm_from_timestamp ON transactions;
DROP INDEX idx_evm_to_timestamp ON transactions;
DROP INDEX idx_account_balance_address_denom ON account_balances;
DROP INDEX idx_contract_name_prefix ON contract_infos;
DROP INDEX idx_contract_token_name_prefix ON contract_infos;
DROP INDEX idx_contract_name_fulltext ON contract_infos;
DROP INDEX idx_contract_filters ON contract_infos;
```

## 注意事项

1. **MySQL 版本兼容性**:
   - MySQL 5.6+: 支持所有索引类型
   - MySQL 5.7+: 支持 JSON 索引和虚拟列
   - MySQL 8.0+: 支持函数索引和降序索引

2. **索引大小**:
   - 大表（>1000万行）的索引创建可能需要较长时间
   - 建议在低峰期执行，或使用 `pt-online-schema-change` 工具

3. **查询优化**:
   - 索引创建后，建议运行 `ANALYZE TABLE` 更新统计信息
   - 监控慢查询日志，确保索引被正确使用

4. **维护**:
   - 定期检查索引使用情况
   - 删除未使用的索引以节省空间
   - 根据查询模式调整索引策略


