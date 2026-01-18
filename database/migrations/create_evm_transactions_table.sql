-- ============================================
-- 创建EVM交易表
-- 将EVM相关信息从transactions表中分离出来
-- 避免更新transactions表时影响signatures和signer字段
-- ============================================

-- 创建evm_transactions表
CREATE TABLE IF NOT EXISTS `evm_transactions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `transaction_hash` varchar(128) NOT NULL COMMENT 'Cosmos交易hash，关联transactions.hash',
  `evm_hash` varchar(128) NOT NULL COMMENT 'EVM交易hash',
  `evm_from_address` varchar(128) DEFAULT NULL COMMENT 'EVM交易发送地址',
  `evm_to_address` varchar(128) DEFAULT NULL COMMENT 'EVM交易接收地址',
  `evm_details_storage_key` varchar(255) DEFAULT NULL COMMENT 'EVM交易详情的对象存储key，格式: evm_details/{hash[:8]}/{hash}',
  `evm_details_synced` tinyint(1) DEFAULT '0' COMMENT 'EVM交易详情是否已同步',
  `evm_details_sync_error` text COMMENT 'EVM交易详情同步错误信息',
  `evm_details_sync_at` datetime(3) DEFAULT NULL COMMENT 'EVM交易详情同步时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_transaction_hash` (`transaction_hash`),
  KEY `idx_evm_transactions_deleted_at` (`deleted_at`),
  KEY `idx_evm_hash` (`evm_hash`),
  KEY `idx_evm_from_address` (`evm_from_address`),
  KEY `idx_evm_to_address` (`evm_to_address`),
  KEY `idx_evm_details_synced` (`evm_details_synced`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='EVM交易表';

-- 可选：迁移现有数据（如果transactions表中已有EVM数据）
-- 注意：只在transactions表中存在EVM相关数据时执行
-- INSERT INTO evm_transactions (transaction_hash, evm_hash, evm_from_address, evm_to_address, evm_details_storage_key, evm_details_synced, evm_details_sync_error, evm_details_sync_at, created_at, updated_at)
-- SELECT 
--   hash as transaction_hash,
--   evm_hash,
--   evm_from_address,
--   evm_to_address,
--   evm_details_storage_key,
--   evm_details_synced,
--   evm_details_sync_error,
--   evm_details_sync_at,
--   created_at,
--   updated_at
-- FROM transactions
-- WHERE evm_hash IS NOT NULL AND evm_hash != ''
-- ON DUPLICATE KEY UPDATE
--   evm_hash = VALUES(evm_hash),
--   evm_from_address = VALUES(evm_from_address),
--   evm_to_address = VALUES(evm_to_address),
--   evm_details_storage_key = VALUES(evm_details_storage_key),
--   evm_details_synced = VALUES(evm_details_synced),
--   evm_details_sync_error = VALUES(evm_details_sync_error),
--   evm_details_sync_at = VALUES(evm_details_sync_at),
--   updated_at = VALUES(updated_at);