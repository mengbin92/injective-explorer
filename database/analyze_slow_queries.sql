/*
 * Performance Analysis and Query Optimization Script
 * 
 * This script analyzes slow queries from MySQL performance_schema and suggests optimizations.
 * 
 * Usage:
 *   mysql -u username -p database_name < scripts/analyze_slow_queries.sql
 * 
 * Prerequisites:
 *   - MySQL 5.6+ with performance_schema enabled
 *   - Performance schema must be enabled: SHOW VARIABLES LIKE 'performance_schema';
 * 
 * Date: 2025-01-XX
 */

SET @database_name = DATABASE();

-- ============================================================================
-- 1. FIND SLOWEST QUERIES BY AVERAGE EXECUTION TIME
-- ============================================================================

SELECT 
    '=== TOP 10 SLOWEST QUERIES BY AVERAGE TIME ===' AS analysis_section;

SELECT
    DIGEST_TEXT AS query_pattern,
    COUNT_STAR AS execution_count,
    ROUND(AVG_TIMER_WAIT/1000000000000, 4) AS avg_time_sec,
    ROUND(SUM_TIMER_WAIT/1000000000000, 4) AS total_time_sec,
    ROUND(MAX_TIMER_WAIT/1000000000000, 4) AS max_time_sec,
    ROUND(SUM_ROWS_EXAMINED/COUNT_STAR, 0) AS avg_rows_examined,
    ROUND(SUM_ROWS_SENT/COUNT_STAR, 0) AS avg_rows_sent,
    ROUND(SUM_SORT_ROWS/COUNT_STAR, 0) AS avg_sort_rows,
    SUM_SORT_MERGE_PASSES AS total_sort_merge_passes,
    SUM_CREATED_TMP_TABLES AS total_temp_tables,
    SUM_SELECT_FULL_JOIN AS total_full_joins,
    SUM_SELECT_FULL_RANGE_JOIN AS total_full_range_joins,
    SUM_SELECT_RANGE AS total_range_scans,
    SUM_SELECT_RANGE_CHECK AS total_range_checks,
    SUM_NO_INDEX_USED AS total_no_index_used,
    SUM_NO_GOOD_INDEX_USED AS total_no_good_index_used
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME = @database_name
    AND DIGEST_TEXT IS NOT NULL
    AND DIGEST_TEXT != ''
    AND COUNT_STAR > 0
ORDER BY AVG_TIMER_WAIT DESC
LIMIT 10;

-- ============================================================================
-- 2. FIND QUERIES WITH HIGH ROW EXAMINATION (Potential Missing Indexes)
-- ============================================================================

SELECT 
    '=== TOP 10 QUERIES WITH HIGHEST ROW EXAMINATION ===' AS analysis_section;

SELECT
    DIGEST_TEXT AS query_pattern,
    COUNT_STAR AS execution_count,
    ROUND(AVG_TIMER_WAIT/1000000000000, 4) AS avg_time_sec,
    ROUND(SUM_ROWS_EXAMINED/COUNT_STAR, 0) AS avg_rows_examined,
    ROUND(SUM_ROWS_SENT/COUNT_STAR, 0) AS avg_rows_sent,
    ROUND((SUM_ROWS_EXAMINED/COUNT_STAR) / NULLIF(SUM_ROWS_SENT/COUNT_STAR, 0), 2) AS examination_ratio,
    SUM_NO_INDEX_USED AS total_no_index_used,
    SUM_NO_GOOD_INDEX_USED AS total_no_good_index_used
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME = @database_name
    AND DIGEST_TEXT IS NOT NULL
    AND DIGEST_TEXT != ''
    AND COUNT_STAR > 0
    AND SUM_ROWS_EXAMINED > SUM_ROWS_SENT * 10  -- At least 10x more rows examined than sent
ORDER BY (SUM_ROWS_EXAMINED/COUNT_STAR) DESC
LIMIT 10;

-- ============================================================================
-- 3. FIND QUERIES WITH FULL TABLE SCANS (No Index Used)
-- ============================================================================

SELECT 
    '=== QUERIES WITH FULL TABLE SCANS (NO INDEX USED) ===' AS analysis_section;

SELECT
    DIGEST_TEXT AS query_pattern,
    COUNT_STAR AS execution_count,
    ROUND(AVG_TIMER_WAIT/1000000000000, 4) AS avg_time_sec,
    SUM_NO_INDEX_USED AS total_no_index_used,
    SUM_NO_GOOD_INDEX_USED AS total_no_good_index_used,
    ROUND(SUM_ROWS_EXAMINED/COUNT_STAR, 0) AS avg_rows_examined
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME = @database_name
    AND DIGEST_TEXT IS NOT NULL
    AND DIGEST_TEXT != ''
    AND COUNT_STAR > 0
    AND (SUM_NO_INDEX_USED > 0 OR SUM_NO_GOOD_INDEX_USED > 0)
ORDER BY (SUM_NO_INDEX_USED + SUM_NO_GOOD_INDEX_USED) DESC, AVG_TIMER_WAIT DESC
LIMIT 20;

-- ============================================================================
-- 4. FIND QUERIES WITH TEMPORARY TABLES (May Need Optimization)
-- ============================================================================

SELECT 
    '=== QUERIES CREATING TEMPORARY TABLES ===' AS analysis_section;

SELECT
    DIGEST_TEXT AS query_pattern,
    COUNT_STAR AS execution_count,
    ROUND(AVG_TIMER_WAIT/1000000000000, 4) AS avg_time_sec,
    SUM_CREATED_TMP_TABLES AS total_temp_tables,
    SUM_CREATED_TMP_DISK_TABLES AS total_disk_temp_tables,
    ROUND(SUM_ROWS_EXAMINED/COUNT_STAR, 0) AS avg_rows_examined
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME = @database_name
    AND DIGEST_TEXT IS NOT NULL
    AND DIGEST_TEXT != ''
    AND COUNT_STAR > 0
    AND SUM_CREATED_TMP_TABLES > 0
ORDER BY SUM_CREATED_TMP_TABLES DESC, AVG_TIMER_WAIT DESC
LIMIT 10;

-- ============================================================================
-- 5. FIND QUERIES WITH SORT OPERATIONS (May Need Indexes for ORDER BY)
-- ============================================================================

SELECT 
    '=== QUERIES WITH EXPENSIVE SORT OPERATIONS ===' AS analysis_section;

SELECT
    DIGEST_TEXT AS query_pattern,
    COUNT_STAR AS execution_count,
    ROUND(AVG_TIMER_WAIT/1000000000000, 4) AS avg_time_sec,
    ROUND(SUM_SORT_ROWS/COUNT_STAR, 0) AS avg_sort_rows,
    SUM_SORT_MERGE_PASSES AS total_sort_merge_passes,
    SUM_SORT_SCAN AS total_sort_scans,
    SUM_SORT_RANGE AS total_sort_ranges
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME = @database_name
    AND DIGEST_TEXT IS NOT NULL
    AND DIGEST_TEXT != ''
    AND COUNT_STAR > 0
    AND SUM_SORT_ROWS > 0
ORDER BY (SUM_SORT_ROWS/COUNT_STAR) DESC, AVG_TIMER_WAIT DESC
LIMIT 10;

-- ============================================================================
-- 6. FIND MOST FREQUENTLY EXECUTED QUERIES (High Impact if Slow)
-- ============================================================================

SELECT 
    '=== MOST FREQUENTLY EXECUTED QUERIES ===' AS analysis_section;

SELECT
    DIGEST_TEXT AS query_pattern,
    COUNT_STAR AS execution_count,
    ROUND(AVG_TIMER_WAIT/1000000000000, 4) AS avg_time_sec,
    ROUND(SUM_TIMER_WAIT/1000000000000, 4) AS total_time_sec,
    ROUND((SUM_TIMER_WAIT/1000000000000) / 3600, 2) AS total_time_hours,
    ROUND(SUM_ROWS_EXAMINED/COUNT_STAR, 0) AS avg_rows_examined
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME = @database_name
    AND DIGEST_TEXT IS NOT NULL
    AND DIGEST_TEXT != ''
    AND COUNT_STAR > 100  -- At least 100 executions
ORDER BY COUNT_STAR DESC
LIMIT 10;

-- ============================================================================
-- 7. ANALYZE SPECIFIC TABLE QUERIES
-- ============================================================================

SELECT 
    '=== TRANSACTIONS TABLE QUERY ANALYSIS ===' AS analysis_section;

SELECT
    DIGEST_TEXT AS query_pattern,
    COUNT_STAR AS execution_count,
    ROUND(AVG_TIMER_WAIT/1000000000000, 4) AS avg_time_sec,
    ROUND(SUM_ROWS_EXAMINED/COUNT_STAR, 0) AS avg_rows_examined,
    SUM_NO_INDEX_USED AS total_no_index_used,
    SUM_NO_GOOD_INDEX_USED AS total_no_good_index_used
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME = @database_name
    AND DIGEST_TEXT LIKE '%transactions%'
    AND DIGEST_TEXT IS NOT NULL
    AND COUNT_STAR > 0
ORDER BY AVG_TIMER_WAIT DESC
LIMIT 10;

SELECT 
    '=== BLOCKS TABLE QUERY ANALYSIS ===' AS analysis_section;

SELECT
    DIGEST_TEXT AS query_pattern,
    COUNT_STAR AS execution_count,
    ROUND(AVG_TIMER_WAIT/1000000000000, 4) AS avg_time_sec,
    ROUND(SUM_ROWS_EXAMINED/COUNT_STAR, 0) AS avg_rows_examined,
    SUM_NO_INDEX_USED AS total_no_index_used
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME = @database_name
    AND DIGEST_TEXT LIKE '%blocks%'
    AND DIGEST_TEXT IS NOT NULL
    AND COUNT_STAR > 0
ORDER BY AVG_TIMER_WAIT DESC
LIMIT 10;

-- ============================================================================
-- 8. INDEX USAGE ANALYSIS
-- ============================================================================

SELECT 
    '=== INDEX USAGE STATISTICS ===' AS analysis_section;

SELECT
    TABLE_NAME,
    INDEX_NAME,
    SEQ_IN_INDEX,
    COLUMN_NAME,
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
    AND TABLE_NAME IN ('transactions', 'blocks', 'account_balances', 'contract_infos', 'accounts')
ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX;

-- ============================================================================
-- 9. TABLE SIZE AND ROW COUNT ANALYSIS
-- ============================================================================

SELECT 
    '=== TABLE SIZE AND ROW COUNT ===' AS analysis_section;

SELECT
    TABLE_NAME,
    ROUND(((DATA_LENGTH + INDEX_LENGTH) / 1024 / 1024), 2) AS size_mb,
    ROUND((DATA_LENGTH / 1024 / 1024), 2) AS data_size_mb,
    ROUND((INDEX_LENGTH / 1024 / 1024), 2) AS index_size_mb,
    TABLE_ROWS AS estimated_rows,
    ROUND((INDEX_LENGTH / NULLIF(DATA_LENGTH, 0)) * 100, 2) AS index_overhead_pct
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = @database_name
    AND TABLE_NAME IN ('transactions', 'blocks', 'account_balances', 'contract_infos', 'accounts', 'transaction_data_archives')
ORDER BY (DATA_LENGTH + INDEX_LENGTH) DESC;

-- ============================================================================
-- 10. SUMMARY AND RECOMMENDATIONS
-- ============================================================================

SELECT 
    '=== OPTIMIZATION RECOMMENDATIONS ===' AS analysis_section;

SELECT
    '1. Review queries with high avg_rows_examined/avg_rows_sent ratio - may need better indexes' AS recommendation
UNION ALL
SELECT '2. Queries with SUM_NO_INDEX_USED > 0 need indexes on WHERE clause columns'
UNION ALL
SELECT '3. Queries with high SUM_SORT_ROWS may benefit from indexes on ORDER BY columns'
UNION ALL
SELECT '4. Queries creating temporary tables may need indexes or query rewrite'
UNION ALL
SELECT '5. Most frequently executed queries should be prioritized for optimization'
UNION ALL
SELECT '6. Consider composite indexes for queries with multiple WHERE conditions'
UNION ALL
SELECT '7. Review index cardinality - very low cardinality indexes may not be useful'
UNION ALL
SELECT '8. Monitor queries with high total_time_sec - small improvements have big impact';

SELECT 
    '=== ANALYSIS COMPLETE ===' AS status,
    NOW() AS analysis_time;

