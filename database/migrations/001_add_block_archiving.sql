-- ============================================================================
-- Block Archiving Migration
-- ============================================================================
-- Purpose: Add archiving support for blocks table
--
-- This migration:
-- 1. Adds 'archived' and 'txs_storage_key' columns to blocks table
-- 2. Creates block_data_archives table
-- 3. Adds indexes for archiving queries
--
-- Author: System
-- Date: 2025-01-25
-- Version: 001
-- ============================================================================

-- Set character set
SET NAMES utf8mb4;

-- Disable foreign key checks during migration
SET FOREIGN_KEY_CHECKS = 0;

-- ============================================================================
-- PART 1: Add columns to blocks table
-- ============================================================================

-- Add 'archived' column if not exists
SET @dbname = DATABASE();
SET @tablename = 'blocks';
SET @columnname = 'archived';
SET @preparedStatement = (SELECT IF(
  (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = @dbname
    AND TABLE_NAME = @tablename
    AND COLUMN_NAME = @columnname
  ) > 0,
  'SELECT 1',
  CONCAT('ALTER TABLE ', @tablename, ' ADD COLUMN `', @columnname, '` TINYINT(1) DEFAULT 0 COMMENT ''是否已归档（0=未归档，1=已归档）''')
));
PREPARE alterIfNotExists FROM @preparedStatement;
EXECUTE alterIfNotExists;
DEALLOCATE PREPARE alterIfNotExists;

-- Add 'txs_storage_key' column if not exists
SET @columnname = 'txs_storage_key';
SET @preparedStatement = (SELECT IF(
  (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = @dbname
    AND TABLE_NAME = @tablename
    AND COLUMN_NAME = @columnname
  ) > 0,
  'SELECT 1',
  CONCAT('ALTER TABLE ', @tablename, ' ADD COLUMN `', @columnname, '` VARCHAR(255) DEFAULT NULL COMMENT ''txs字段的对象存储key，格式: blocks/archive/txs/{height}''')
));
PREPARE alterIfNotExists FROM @preparedStatement;
EXECUTE alterIfNotExists;
DEALLOCATE PREPARE alterIfNotExists;

-- ============================================================================
-- PART 2: Create block_data_archives table
-- ============================================================================

-- Create table if not exists
CREATE TABLE IF NOT EXISTS `block_data_archives` (
  `block_id` bigint unsigned NOT NULL COMMENT '关联blocks.id',
  `txs` longtext COMMENT '区块交易列表（压缩后的JSON）',
  `txs_storage_key` varchar(255) DEFAULT NULL COMMENT 'txs字段的对象存储key（如果已上传到对象存储）',
  `archived_at` datetime(3) DEFAULT NULL COMMENT '归档时间',
  PRIMARY KEY (`block_id`),
  KEY `idx_block_data_archives_txs_storage_key` (`txs_storage_key`),
  KEY `idx_block_data_archives_archived_at` (`archived_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='区块数据归档表';

-- ============================================================================
-- PART 3: Add indexes to blocks table
-- ============================================================================

-- Add composite index for archiving queries if not exists
SET @indexname = 'idx_blocks_archived_timestamp';
SET @preparedStatement = (SELECT IF(
  (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
    WHERE TABLE_SCHEMA = @dbname
    AND TABLE_NAME = @tablename
    AND INDEX_NAME = @indexname
  ) > 0,
  'SELECT 1',
  CONCAT('CREATE INDEX ', @indexname, ' ON ', @tablename, ' (`archived`, `block_unix_timestamp`)')
));
PREPARE createIndexIfNotExists FROM @preparedStatement;
EXECUTE createIndexIfNotExists;
DEALLOCATE PREPARE createIndexIfNotExists;

-- Re-enable foreign key checks
SET FOREIGN_KEY_CHECKS = 1;

-- ============================================================================
-- VERIFICATION
-- ============================================================================

-- Display migration results
SELECT 'Block archiving migration completed successfully!' AS status;

-- Show blocks table structure
SELECT
    COLUMN_NAME as 'Column',
    DATA_TYPE as 'Type',
    IS_NULLABLE as 'Nullable',
    COLUMN_DEFAULT as 'Default',
    COLUMN_COMMENT as 'Comment'
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = @dbname
AND TABLE_NAME = 'blocks'
AND COLUMN_NAME IN ('archived', 'txs_storage_key')
ORDER BY ORDINAL_POSITION;

-- Show block_data_archives table info
SELECT
    TABLE_NAME as 'Table',
    TABLE_ROWS as 'Rows',
    CREATE_TIME as 'Created',
    TABLE_COMMENT as 'Comment'
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = @dbname
AND TABLE_NAME = 'block_data_archives';

-- Show indexes
SELECT
    INDEX_NAME as 'Index',
    COLUMN_NAME as 'Column',
    SEQ_IN_INDEX as 'Seq',
    INDEX_TYPE as 'Type'
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = @dbname
AND TABLE_NAME = 'blocks'
AND INDEX_NAME = 'idx_blocks_archived_timestamp'
ORDER BY INDEX_NAME, SEQ_IN_INDEX;
