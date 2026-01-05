/*
 * Query Optimization Suggestions Based on Codebase Analysis
 * 
 * This script creates additional indexes based on common query patterns found in the codebase.
 * Run this AFTER analyzing slow queries with analyze_slow_queries.sql
 * 
 * Usage:
 *   mysql -u username -p database_name < scripts/query_optimization_suggestions.sql
 * 
 * WARNING: 
 *   - Review each index before creating. Some may already exist.
 *   - If an index already exists, MySQL will return an error which can be safely ignored.
 *   - For MySQL < 5.7.4, IF NOT EXISTS is not supported for CREATE INDEX.
 *   - Run this script during low-traffic periods as index creation can lock tables.
 * 
 * Date: 2025-01-XX
 */

SET @database_name = DATABASE();

-- ============================================================================
-- CHECK EXISTING INDEXES FIRST
-- ============================================================================

SELECT 
    '=== CHECKING EXISTING INDEXES ===' AS status;

SELECT
    TABLE_NAME,
    INDEX_NAME,
    GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', ') AS columns,
    CARDINALITY
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = @database_name
    AND TABLE_NAME IN ('transactions', 'blocks', 'account_balances', 'contract_infos', 'accounts', 'api_request_logs')
    AND INDEX_NAME LIKE 'idx_%'
GROUP BY TABLE_NAME, INDEX_NAME, CARDINALITY
ORDER BY TABLE_NAME, INDEX_NAME;

-- ============================================================================
-- OPTIMIZATION 1: Transactions Table - Composite Indexes for Common Query Patterns
-- ============================================================================

-- Index for: WHERE block_unix_timestamp >= ? AND signer != '' (getActiveAddresses24h)
-- This query uses COUNT(DISTINCT signer) which can benefit from a composite index
-- Note: idx_signer_timestamp already exists, but timestamp-first helps range queries
SELECT '=== Checking index for active addresses query ===' AS status;
-- Check manually if idx_timestamp_signer_distinct exists before running:
-- SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'transactions' AND INDEX_NAME = 'idx_timestamp_signer_distinct';
-- CREATE INDEX idx_timestamp_signer_distinct ON transactions(block_unix_timestamp, signer);

-- Index for: WHERE block_unix_timestamp >= ? AND code != 0 (getFailedTxCount24h)
-- Note: idx_timestamp_code already exists and should cover this query pattern
-- Verify it's being used with EXPLAIN

-- ============================================================================
-- OPTIMIZATION 2: Transactions Table - Pagination Optimization
-- ============================================================================

-- Many queries use: ORDER BY block_unix_timestamp DESC, tx_number DESC LIMIT ? OFFSET ?
-- Ensure we have a covering index for these queries
-- Note: idx_archived_timestamp_tx already exists for archived transactions

-- Index for general transaction pagination (non-archived)
SELECT '=== Checking index for transaction pagination ===' AS status;
-- CREATE INDEX idx_timestamp_tx_pagination ON transactions(block_unix_timestamp DESC, tx_number DESC, id);

-- ============================================================================
-- OPTIMIZATION 3: Transactions Table - Signer Queries with Pagination
-- ============================================================================

-- Query pattern: WHERE signer = ? ORDER BY block_unix_timestamp DESC, tx_number DESC
-- The idx_signer_timestamp exists but may not include tx_number for optimal sorting
SELECT '=== Checking index for signer queries with pagination ===' AS status;
-- CREATE INDEX idx_signer_timestamp_tx ON transactions(signer, block_unix_timestamp DESC, tx_number DESC);

-- ============================================================================
-- OPTIMIZATION 4: Transactions Table - Block Number Queries (Covering Index)
-- ============================================================================

-- Query pattern: SELECT hash, signer, to, value FROM transactions WHERE block_number = ?
-- This is used in enhanceBlockDTOWithTransactions
-- Note: idx_block_tx exists, but covering index avoids table lookups
-- WARNING: Covering indexes with many columns can be large. Monitor index size.
SELECT '=== Checking covering index for block transactions ===' AS status;
-- CREATE INDEX idx_block_tx_covering ON transactions(block_number, tx_number, hash, signer, `to`, value);

-- ============================================================================
-- OPTIMIZATION 5: Blocks Table - Height Queries
-- ============================================================================

-- Query pattern: WHERE height = ? (already has idx_height)
-- Query pattern: WHERE height < ? ORDER BY height DESC (already has idx_height_desc)
-- Query pattern: WHERE height IN ? (can benefit from idx_height)
-- These should already be optimized, verify with EXPLAIN

-- ============================================================================
-- OPTIMIZATION 6: Account Balances - Address + Denom Queries
-- ============================================================================

-- Query pattern: WHERE address = ? AND denom = ?
-- Note: idx_account_balance_address_denom should already exist from init_database.sql
-- Verify it exists, if not create it
SELECT '=== Verifying account balance index ===' AS status;
-- This index should already exist, but create if missing:
-- CREATE INDEX idx_account_balance_address_denom ON account_balances(address, denom);

-- ============================================================================
-- OPTIMIZATION 7: API Request Logs - Time-based Queries
-- ============================================================================

-- Query pattern: WHERE created_at >= ? (for daily/weekly/monthly stats)
SELECT '=== Creating index for API request logs time queries ===' AS status;
CREATE INDEX idx_api_logs_created_at ON api_request_logs(created_at);

-- Query pattern: WHERE token_id = ? AND created_at >= ?
SELECT '=== Creating composite index for API logs token + time ===' AS status;
CREATE INDEX idx_api_logs_token_time ON api_request_logs(token_id, created_at);

-- Query pattern: WHERE ip_address = ? AND created_at >= ?
SELECT '=== Creating composite index for API logs IP + time ===' AS status;
CREATE INDEX idx_api_logs_ip_time ON api_request_logs(ip_address, created_at);

-- Query pattern: WHERE user_id = ? AND created_at >= ?
SELECT '=== Creating composite index for API logs user + time ===' AS status;
CREATE INDEX idx_api_logs_user_time ON api_request_logs(user_id, created_at);

-- ============================================================================
-- OPTIMIZATION 8: Accounts Table - Address Lookups
-- ============================================================================

-- Query pattern: WHERE address = ? (already has idx_accounts_address)
-- Query pattern: WHERE address IN ? (can use idx_accounts_address)
-- These should already be optimized

-- ============================================================================
-- OPTIMIZATION 9: Contract Info - Search Queries
-- ============================================================================

-- Query pattern: WHERE name LIKE ? OR token_name LIKE ?
-- Note: Prefix indexes and fulltext index should already exist from init_database.sql
-- Verify they exist and are being used

-- ============================================================================
-- OPTIMIZATION 10: Transaction Archive - Lookup Queries
-- ============================================================================

-- Query pattern: WHERE transaction_id = ? (primary key, already indexed)
-- Query pattern: WHERE transaction_id IN ? (primary key, already indexed)
-- These should already be optimized

-- ============================================================================
-- VERIFY NEW INDEXES
-- ============================================================================

SELECT 
    '=== VERIFYING ALL INDEXES ===' AS status;

SELECT
    TABLE_NAME,
    INDEX_NAME,
    GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', ') AS columns,
    CARDINALITY,
    INDEX_TYPE,
    CASE 
        WHEN CARDINALITY = 0 THEN 'LOW'
        WHEN CARDINALITY < 100 THEN 'VERY_LOW'
        WHEN CARDINALITY < 1000 THEN 'LOW'
        WHEN CARDINALITY < 10000 THEN 'MEDIUM'
        ELSE 'HIGH'
    END AS cardinality_level
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = @database_name
    AND TABLE_NAME IN ('transactions', 'blocks', 'account_balances', 'contract_infos', 'accounts', 'api_request_logs')
    AND INDEX_NAME LIKE 'idx_%'
GROUP BY TABLE_NAME, INDEX_NAME, CARDINALITY, INDEX_TYPE
ORDER BY TABLE_NAME, INDEX_NAME;

-- ============================================================================
-- ANALYZE TABLES TO UPDATE STATISTICS
-- ============================================================================

SELECT 
    '=== UPDATING TABLE STATISTICS ===' AS status;

ANALYZE TABLE transactions;
ANALYZE TABLE blocks;
ANALYZE TABLE account_balances;
ANALYZE TABLE contract_infos;
ANALYZE TABLE accounts;
ANALYZE TABLE api_request_logs;

-- ============================================================================
-- INDEX SIZE ANALYSIS
-- ============================================================================

SELECT 
    '=== INDEX SIZE ANALYSIS ===' AS status;

SELECT
    TABLE_NAME,
    INDEX_NAME,
    ROUND(((SUM(CARDINALITY) * AVG(SUB_PART_LEN + 4)) / 1024 / 1024), 2) AS estimated_size_mb,
    COUNT(*) AS columns_in_index
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = @database_name
    AND TABLE_NAME IN ('transactions', 'blocks', 'account_balances', 'contract_infos', 'accounts', 'api_request_logs')
    AND INDEX_NAME LIKE 'idx_%'
GROUP BY TABLE_NAME, INDEX_NAME
ORDER BY estimated_size_mb DESC;

SELECT 
    '=== OPTIMIZATION SCRIPT COMPLETE ===' AS status,
    NOW() AS completion_time,
    'Review the index analysis above and monitor query performance' AS next_steps;
