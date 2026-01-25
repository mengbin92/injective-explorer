-- ============================================================================
-- Block Archiving Migration (Simple Version)
-- ============================================================================
-- Purpose: Add archiving support for blocks table
--
-- This is a simplified version for manual execution.
-- Checks for existing columns/tables before creating them.
--
-- Usage:
--   mysql -u username -p database_name < database/migrations/001_add_block_archiving_simple.sql
--
-- Author: System
-- Date: 2025-01-25
-- Version: 001
-- ============================================================================

SET NAMES utf8mb4;

-- ============================================================================
-- PART 1: Add columns to blocks table
-- ============================================================================

-- Add 'archived' column to blocks table
-- Use ALTER TABLE ... ADD COLUMN IF NOT EXISTS (MySQL 8.0.12+)
-- For older versions, ignore the error if column already exists
ALTER TABLE `blocks`
ADD COLUMN IF NOT EXISTS `archived` TINYINT(1) DEFAULT '0' COMMENT '是否已归档（0=未归档，1=已归档）';

-- Add 'txs_storage_key' column to blocks table
ALTER TABLE `blocks`
ADD COLUMN IF NOT EXISTS `txs_storage_key` VARCHAR(255) DEFAULT NULL COMMENT 'txs字段的对象存储key，格式: blocks/archive/txs/{height}';

-- ============================================================================
-- PART 2: Create block_data_archives table
-- ============================================================================

-- Create block_data_archives table
CREATE TABLE IF NOT EXISTS `block_data_archives` (
  `block_id` BIGINT UNSIGNED NOT NULL COMMENT '关联blocks.id',
  `txs` LONGTEXT COMMENT '区块交易列表（压缩后的JSON）',
  `txs_storage_key` VARCHAR(255) DEFAULT NULL COMMENT 'txs字段的对象存储key（如果已上传到对象存储）',
  `archived_at` DATETIME(3) DEFAULT NULL COMMENT '归档时间',
  PRIMARY KEY (`block_id`),
  KEY `idx_block_data_archives_txs_storage_key` (`txs_storage_key`),
  KEY `idx_block_data_archives_archived_at` (`archived_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='区块数据归档表';

-- ============================================================================
-- PART 3: Add indexes to blocks table
-- ============================================================================

-- Add composite index for archiving queries
ALTER TABLE `blocks`
ADD INDEX IF NOT EXISTS `idx_blocks_archived_timestamp` (`archived`, `block_unix_timestamp`);

-- ============================================================================
-- VERIFICATION
-- ============================================================================

SELECT '===========================================' AS '';
SELECT 'Block Archiving Migration' AS 'Migration';
SELECT '===========================================' AS '';

SELECT '✓ Migration completed successfully!' AS 'Status';
SELECT '' AS '';

-- Show new columns in blocks table
SELECT 'New columns added to blocks table:' AS '';
SELECT
    COLUMN_NAME AS 'Column',
    DATA_TYPE AS 'Type',
    COLUMN_DEFAULT AS 'Default',
    COLUMN_COMMENT AS 'Comment'
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
AND TABLE_NAME = 'blocks'
AND COLUMN_NAME IN ('archived', 'txs_storage_key');

SELECT '' AS '';

-- Show block_data_archives table info
SELECT 'block_data_archives table created:' AS '';
SELECT
    TABLE_NAME AS 'Table',
    TABLE_COMMENT AS 'Description'
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
AND TABLE_NAME = 'block_data_archives';

SELECT '' AS '';

-- Show current archiving status
SELECT 'Current blocks archiving status:' AS '';
SELECT
    COUNT(*) AS 'Total Blocks',
    SUM(CASE WHEN archived = 1 THEN 1 ELSE 0 END) AS 'Archived',
    SUM(CASE WHEN archived = 0 OR archived IS NULL THEN 1 ELSE 0 END) AS 'Not Archived'
FROM blocks;

SELECT '===========================================' AS '';
