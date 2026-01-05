# Query Performance Optimization Guide

This guide explains how to analyze and optimize slow database queries in the Injective Explorer application.

## Overview

The optimization process consists of two main steps:
1. **Analysis**: Identify slow queries using `analyze_slow_queries.sql`
2. **Optimization**: Apply indexes and optimizations using `query_optimization_suggestions.sql`

## Prerequisites

- MySQL 5.6+ (MySQL 8.0+ recommended)
- Performance Schema enabled: `SHOW VARIABLES LIKE 'performance_schema';`
- Database access with appropriate permissions
- Run during low-traffic periods (index creation can lock tables)

## Step 1: Analyze Slow Queries

### Run the Analysis Script

```bash
mysql -u username -p database_name < scripts/analyze_slow_queries.sql > slow_queries_analysis.txt
```

### Understanding the Results

The script outputs 10 analysis sections:

#### 1. Top 10 Slowest Queries by Average Time
- **What to look for**: Queries with `avg_time_sec > 1.0` (1 second or more)
- **Action**: These are your highest priority optimization targets

#### 2. Queries with High Row Examination
- **What to look for**: `examination_ratio > 10` (examining 10x more rows than returned)
- **Action**: Likely missing indexes on WHERE clause columns

#### 3. Queries with Full Table Scans
- **What to look for**: `total_no_index_used > 0` or `total_no_good_index_used > 0`
- **Action**: Create indexes on WHERE clause columns

#### 4. Queries Creating Temporary Tables
- **What to look for**: `total_temp_tables > 0`, especially `total_disk_temp_tables > 0`
- **Action**: May need indexes on ORDER BY or GROUP BY columns, or query rewrite

#### 5. Queries with Expensive Sort Operations
- **What to look for**: `avg_sort_rows > 1000` or `total_sort_merge_passes > 0`
- **Action**: Create indexes on ORDER BY columns

#### 6. Most Frequently Executed Queries
- **What to look for**: High `execution_count` with moderate `avg_time_sec`
- **Action**: Small improvements here have big impact due to frequency

#### 7-8. Table-Specific Analysis
- **What to look for**: Patterns specific to `transactions` or `blocks` tables
- **Action**: Focus optimization on these high-traffic tables

#### 9. Index Usage Statistics
- **What to look for**: 
  - Low `CARDINALITY` (< 100) - index may not be selective enough
  - Missing indexes for common query patterns
- **Action**: Review and potentially drop low-cardinality indexes, add missing ones

#### 10. Table Size Analysis
- **What to look for**: 
  - Large `index_size_mb` relative to `data_size_mb` (> 50%)
  - Very large tables (> 10GB)
- **Action**: Consider partitioning or archiving old data

## Step 2: Apply Optimizations

### Review Before Running

**IMPORTANT**: Review the optimization script before running:

```bash
cat scripts/query_optimization_suggestions.sql
```

Check which indexes will be created and verify they don't already exist.

### Run the Optimization Script

```bash
mysql -u username -p database_name < scripts/query_optimization_suggestions.sql > optimization_results.txt
```

### What the Script Does

1. **Checks existing indexes** - Shows current index structure
2. **Creates new indexes** for common query patterns:
   - `idx_timestamp_signer_distinct`: For COUNT(DISTINCT signer) queries
   - `idx_timestamp_tx_pagination`: For transaction pagination
   - `idx_signer_timestamp_tx`: For signer queries with pagination
   - `idx_block_tx_covering`: Covering index for block transaction queries
   - `idx_api_logs_*`: Various indexes for API request logs
3. **Updates table statistics** - Runs ANALYZE TABLE to update optimizer statistics
4. **Shows index size analysis** - Helps identify large indexes

### Handling Errors

If you see errors like:
```
ERROR 1061 (42000): Duplicate key name 'idx_xxx'
```

This means the index already exists. You can safely ignore these errors, or modify the script to skip existing indexes.

## Step 3: Verify Improvements

### Check Query Performance

After creating indexes, verify they're being used:

```sql
-- Check if a specific query uses the new index
EXPLAIN SELECT * FROM transactions 
WHERE block_unix_timestamp >= ? AND signer != '' 
ORDER BY block_unix_timestamp DESC 
LIMIT 100;
```

Look for `key` column showing your new index name.

### Monitor Performance

1. **Re-run the analysis script** after a few hours/days:
   ```bash
   mysql -u username -p database_name < scripts/analyze_slow_queries.sql > slow_queries_after.txt
   ```

2. **Compare results**:
   ```bash
   diff slow_queries_analysis.txt slow_queries_after.txt
   ```

3. **Check slow query log** (if enabled):
   ```sql
   SHOW VARIABLES LIKE 'slow_query_log%';
   ```

## Common Query Patterns and Optimizations

### Pattern 1: COUNT(DISTINCT column) with WHERE

**Query**:
```sql
SELECT COUNT(DISTINCT signer) FROM transactions 
WHERE block_unix_timestamp >= ? AND signer != '';
```

**Optimization**: Create composite index on (timestamp, signer)
```sql
CREATE INDEX idx_timestamp_signer_distinct 
ON transactions(block_unix_timestamp, signer);
```

### Pattern 2: Pagination with ORDER BY

**Query**:
```sql
SELECT * FROM transactions 
ORDER BY block_unix_timestamp DESC, tx_number DESC 
LIMIT 100 OFFSET 1000;
```

**Optimization**: Create index on ORDER BY columns
```sql
CREATE INDEX idx_timestamp_tx_pagination 
ON transactions(block_unix_timestamp DESC, tx_number DESC, id);
```

### Pattern 3: WHERE + ORDER BY

**Query**:
```sql
SELECT * FROM transactions 
WHERE signer = ? 
ORDER BY block_unix_timestamp DESC, tx_number DESC;
```

**Optimization**: Create composite index covering WHERE and ORDER BY
```sql
CREATE INDEX idx_signer_timestamp_tx 
ON transactions(signer, block_unix_timestamp DESC, tx_number DESC);
```

### Pattern 4: Covering Index (SELECT specific columns)

**Query**:
```sql
SELECT hash, signer, to, value FROM transactions 
WHERE block_number = ?;
```

**Optimization**: Create covering index with all selected columns
```sql
CREATE INDEX idx_block_tx_covering 
ON transactions(block_number, tx_number, hash, signer, `to`, value);
```

## Index Maintenance

### When to Rebuild Indexes

Indexes can become fragmented over time. Rebuild them if:
- Table has had many DELETE operations
- Index cardinality seems incorrect
- Query performance degrades over time

```sql
-- Rebuild index (MySQL 8.0+)
ALTER TABLE transactions DROP INDEX idx_xxx, ADD INDEX idx_xxx (...);

-- Or optimize table (rebuilds all indexes)
OPTIMIZE TABLE transactions;
```

### Monitoring Index Usage

Check which indexes are actually being used:

```sql
-- MySQL 8.0+
SELECT 
    OBJECT_SCHEMA,
    OBJECT_NAME,
    INDEX_NAME,
    COUNT_FETCH,
    COUNT_INSERT,
    COUNT_UPDATE,
    COUNT_DELETE
FROM performance_schema.table_io_waits_summary_by_index_usage
WHERE OBJECT_SCHEMA = DATABASE()
ORDER BY COUNT_FETCH DESC;
```

Unused indexes can be dropped to save space and improve write performance.

## Best Practices

1. **Create indexes incrementally** - Don't create all indexes at once. Create them one at a time and measure impact.

2. **Monitor index size** - Large indexes consume disk space and slow down INSERT/UPDATE operations.

3. **Use EXPLAIN** - Always use EXPLAIN to verify indexes are being used before and after optimization.

4. **Test in staging first** - Apply optimizations to staging environment before production.

5. **Document changes** - Keep track of which indexes were created and why.

6. **Regular maintenance** - Run analysis script monthly to catch new slow queries.

## Troubleshooting

### Index Not Being Used

1. Check if statistics are up to date: `ANALYZE TABLE table_name;`
2. Verify index exists: `SHOW INDEX FROM table_name;`
3. Check query with EXPLAIN
4. Consider index hints if needed: `USE INDEX (idx_name)`

### Index Creation Taking Too Long

- Large tables (> 10M rows) can take hours to index
- Use `pt-online-schema-change` for zero-downtime index creation
- Consider creating indexes during maintenance windows

### High Index Overhead

- If index size > 50% of table size, consider:
  - Dropping unused indexes
  - Using prefix indexes for long VARCHAR columns
  - Partitioning large tables

## Additional Resources

- [MySQL Index Optimization](https://dev.mysql.com/doc/refman/8.0/en/optimization-indexes.html)
- [EXPLAIN Output Format](https://dev.mysql.com/doc/refman/8.0/en/explain-output.html)
- [Performance Schema Documentation](https://dev.mysql.com/doc/refman/8.0/en/performance-schema.html)

## Support

For questions or issues with query optimization:
1. Check the analysis output for specific query patterns
2. Review the codebase for query usage (`internal/service/explorer.go`, etc.)
3. Consult the database index optimization documentation (`docs/database-index-optimization.md`)

