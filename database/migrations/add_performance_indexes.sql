-- Performance optimization indexes for Injective Explorer
-- These indexes are designed to improve query performance for common access patterns

-- 1. 交易表索引优化
-- 查询: SELECT * FROM transactions WHERE signer = ? AND archived = 0 ORDER BY block_unix_timestamp DESC LIMIT 20
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_signer_timestamp
ON transactions (signer, block_unix_timestamp DESC)
WHERE archived = 0;

-- 查询: SELECT * FROM transactions WHERE evm_hash = ?
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_evm_hash
ON transactions (evm_hash)
WHERE evm_hash != '' AND archived = 0;

-- 查询: SELECT * FROM transactions WHERE block_number = ? AND code = ?
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_block_code
ON transactions (block_number, code)
WHERE archived = 0;

-- 2. 区块表索引优化
-- 查询: SELECT * FROM blocks ORDER BY height DESC LIMIT 20
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_blocks_height_desc
ON blocks (height DESC);

-- 查询: SELECT * FROM blocks WHERE height >= ? AND height <= ?
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_blocks_height_range
ON blocks (height);

-- 3. 账户余额索引优化
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_balances_address
ON account_balances (account_address);

-- 4. 统计查询优化
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_timestamp_stats
ON transactions (block_unix_timestamp DESC)
WHERE archived = 0;

-- 5. 账户交易查询优化
-- 查询: SELECT * FROM transactions WHERE (signer = ? OR to = ?) AND archived = 0 ORDER BY block_unix_timestamp DESC
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_signer_to_timestamp
ON transactions (signer, block_unix_timestamp DESC)
WHERE archived = 0;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_to_timestamp
ON transactions (to, block_unix_timestamp DESC)
WHERE archived = 0;

-- 6. 失败交易查询优化
-- 查询: SELECT * FROM transactions WHERE code != 0 AND archived = 0 ORDER BY block_unix_timestamp DESC
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_code_timestamp
ON transactions (code, block_unix_timestamp DESC)
WHERE archived = 0 AND code != 0;
