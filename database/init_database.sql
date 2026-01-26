/*
 Database Initialization Script for Injective Explorer
 This script creates all tables and indexes in one go.
 Can be executed at MySQL startup or manually.

 Usage:
   mysql -u username -p database_name < scripts/init_database.sql

 Date: 2025-01-XX
 MySQL Version: 5.6+ (8.0+ recommended)
*/

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ============================================================================
-- TABLE CREATION
-- ============================================================================

-- ----------------------------
-- Table structure for account_balances
-- ----------------------------
DROP TABLE IF EXISTS `account_balances`;
CREATE TABLE `account_balances` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `address` varchar(191) DEFAULT NULL,
  `denom` varchar(191) DEFAULT NULL,
  `amount` longtext,
  PRIMARY KEY (`id`),
  KEY `idx_account_balances_deleted_at` (`deleted_at`),
  KEY `idx_account_balances_address` (`address`),
  KEY `idx_account_balances_denom` (`denom`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for address_labels
-- ----------------------------
DROP TABLE IF EXISTS `address_labels`;
CREATE TABLE `address_labels` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `address` varchar(128) NOT NULL COMMENT '地址（统一存储为以太坊格式0x...）',
  `chain_id` varchar(50) NOT NULL COMMENT '链ID',
  `label_type` varchar(50) NOT NULL COMMENT '标签类型: exchange, project, personal, scam, verified, custom等',
  `label_value` varchar(255) DEFAULT NULL COMMENT '标签值（标签的具体内容）',
  `label_color` varchar(20) DEFAULT NULL COMMENT '标签颜色（用于前端显示，如#FF5733）',
  `description` text COMMENT '标签描述',
  `visible` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否在前端显示（1=显示，0=隐藏）',
  `created_by` varchar(128) DEFAULT NULL COMMENT '创建者地址或用户名',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_address_label_value` (`chain_id`,`address`,`label_value`,`deleted_at`),
  KEY `idx_address_labels_deleted_at` (`deleted_at`),
  KEY `idx_address_labels_address` (`chain_id`,`address`),
  KEY `idx_address_labels_label_type` (`label_type`),
  KEY `idx_address_labels_label_value` (`label_value`),
  KEY `idx_address_labels_visible` (`visible`),
  KEY `idx_address_labels_created_by` (`created_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for accounts
-- ----------------------------
DROP TABLE IF EXISTS `accounts`;
CREATE TABLE `accounts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `address` varchar(191) DEFAULT NULL,
  `base_account` longtext,
  `code_hash` longtext,
  PRIMARY KEY (`id`),
  KEY `idx_accounts_deleted_at` (`deleted_at`),
  KEY `idx_accounts_address` (`address`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for api_plans
-- ----------------------------
DROP TABLE IF EXISTS `api_plans`;
CREATE TABLE `api_plans` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `name` varchar(50) NOT NULL,
  `plan_type` varchar(20) NOT NULL,
  `description` text DEFAULT NULL,
  `monthly_limit` bigint unsigned NOT NULL,
  `rate_limit_per_second` bigint NOT NULL,
  `max_tokens` bigint NOT NULL,
  `enable_webhook` tinyint(1) NOT NULL DEFAULT '0',
  `price` decimal(10,2) NOT NULL DEFAULT '0.00',
  `status` varchar(20) NOT NULL DEFAULT 'active',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_api_plans_name` (`name`),
  KEY `idx_api_plans_deleted_at` (`deleted_at`),
  KEY `idx_api_plans_plan_type` (`plan_type`),
  KEY `idx_api_plans_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Initialize Free plan data
INSERT INTO `api_plans` (`name`, `plan_type`, `monthly_limit`, `rate_limit_per_second`, `max_tokens`, `enable_webhook`, `price`, `status`) 
VALUES ('Free', 'free', 100000, 5, 3, 0, 0.00, 'active')
ON DUPLICATE KEY UPDATE `name`=`name`;

-- ----------------------------
-- Table structure for api_request_logs
-- ----------------------------
DROP TABLE IF EXISTS `api_request_logs`;
CREATE TABLE `api_request_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `token_id` bigint unsigned DEFAULT NULL,
  `user_id` varchar(64) DEFAULT NULL,
  `ip_address` varchar(45) NOT NULL,
  `ip_location` varchar(100) DEFAULT NULL,
  `endpoint` varchar(255) NOT NULL,
  `method` varchar(10) NOT NULL,
  `status_code` bigint NOT NULL,
  `response_time` bigint NOT NULL,
  `error_message` text,
  PRIMARY KEY (`id`),
  KEY `idx_api_request_logs_deleted_at` (`deleted_at`),
  KEY `idx_api_request_logs_token_id` (`token_id`),
  KEY `idx_api_request_logs_user_id` (`user_id`),
  KEY `idx_api_request_logs_ip_address` (`ip_address`),
  KEY `idx_api_request_logs_endpoint` (`endpoint`),
  KEY `idx_api_request_logs_status_code` (`status_code`),
  -- Performance indexes for time-based queries (added after optimization analysis)
  KEY `idx_api_logs_created_at` (`created_at`),
  KEY `idx_api_logs_token_time` (`token_id`,`created_at`),
  KEY `idx_api_logs_ip_time` (`ip_address`,`created_at`),
  KEY `idx_api_logs_user_time` (`user_id`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for api_tokens
-- ----------------------------
DROP TABLE IF EXISTS `api_tokens`;
CREATE TABLE `api_tokens` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_id` varchar(64) NOT NULL,
  `name` varchar(50) NOT NULL,
  `token_hash` varchar(64) NOT NULL,
  `token_preview` varchar(20) NOT NULL,
  `plan_type` varchar(20) NOT NULL DEFAULT 'free',
  `status` varchar(20) NOT NULL DEFAULT 'active',
  `last_used_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_api_tokens_token_hash` (`token_hash`),
  KEY `idx_api_tokens_deleted_at` (`deleted_at`),
  KEY `idx_api_tokens_user_id` (`user_id`),
  KEY `idx_api_tokens_plan_type` (`plan_type`),
  KEY `idx_api_tokens_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for blocks
-- ----------------------------
DROP TABLE IF EXISTS `blocks`;
CREATE TABLE `blocks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `height` bigint unsigned DEFAULT NULL,
  `proposer` longtext,
  `moniker` longtext,
  `block_hash` varchar(191) DEFAULT NULL,
  `parent_hash` longtext,
  `num_precommits` bigint DEFAULT NULL,
  `num_txs` bigint DEFAULT NULL,
  `total_txs` bigint DEFAULT NULL,
  `txs` longtext,
  `timestamp` longtext,
  `block_unix_timestamp` bigint unsigned DEFAULT NULL,
  `archived` tinyint(1) DEFAULT '0' COMMENT '是否已归档（0=未归档，1=已归档）',
  `txs_storage_key` varchar(255) DEFAULT NULL COMMENT 'txs字段的对象存储key，格式: blocks/archive/txs/{height}',
  PRIMARY KEY (`id`),
  KEY `idx_blocks_deleted_at` (`deleted_at`),
  KEY `idx_height` (`height`),
  KEY `idx_height_desc` (`height`),
  KEY `idx_blocks_block_hash` (`block_hash`),
  KEY `idx_timestamp_desc` (`block_unix_timestamp`),
  KEY `idx_blocks_archived_timestamp` (`archived`, `block_unix_timestamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for contract_infos
-- ----------------------------
DROP TABLE IF EXISTS `contract_infos`;
CREATE TABLE `contract_infos` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `contract_address` varchar(128) NOT NULL,
  `name` varchar(255) DEFAULT NULL,
  `token_name` varchar(255) DEFAULT NULL,
  `token_symbol` varchar(50) DEFAULT NULL,
  `contract_type` varchar(50) DEFAULT 'solidity',
  `chain_id` varchar(50) DEFAULT 'injective-1',
  `verification_status` varchar(50) DEFAULT 'unverified',
  `is_verified` tinyint(1) DEFAULT '0',
  `verified_at` datetime(3) DEFAULT NULL,
  `verified_by` varchar(128) DEFAULT NULL,
  `metadata` json DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_contract_infos_deleted_at` (`deleted_at`),
  KEY `idx_token_symbol` (`token_symbol`),
  KEY `idx_contract_infos_contract_type` (`contract_type`),
  KEY `idx_contract_infos_chain_id` (`chain_id`),
  KEY `idx_contract_infos_verification_status` (`verification_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for contract_tags
-- ----------------------------
DROP TABLE IF EXISTS `contract_tags`;
CREATE TABLE `contract_tags` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `contract_address` varchar(128) NOT NULL,
  `chain_id` varchar(50) NOT NULL,
  `tag_type` varchar(50) NOT NULL,
  `tag_value` varchar(255) DEFAULT NULL,
  `tag_color` varchar(20) DEFAULT NULL,
  `description` text,
  `visible` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否在前端显示（1=显示，0=隐藏）',
  `tag_definition_id` bigint unsigned DEFAULT NULL COMMENT '关联的预置标签定义ID（可选，如果从预设标签选择则填写）',
  `is_preset` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否为系统预置标签（1=是，0=否）',
  `created_by` varchar(128) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_contract_tags_deleted_at` (`deleted_at`),
  KEY `idx_contract_tags_tag_type` (`tag_type`),
  KEY `idx_contract_tags_visible` (`visible`),
  KEY `idx_contract_tags_is_preset` (`is_preset`),
  KEY `idx_contract_tags_tag_definition_id` (`tag_definition_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for tag_definitions
-- ----------------------------
DROP TABLE IF EXISTS `tag_definitions`;
CREATE TABLE `tag_definitions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `tag_type` varchar(20) NOT NULL COMMENT 'Tag type: contract, address',
  `tag_name` varchar(50) NOT NULL COMMENT 'Tag name',
  `tag_category` varchar(20) NOT NULL COMMENT 'Tag category: normal, abnormal',
  `tag_color` varchar(20) DEFAULT NULL COMMENT 'Tag color (hex code)',
  `description` text COMMENT 'Tag description',
  `is_system` tinyint(1) NOT NULL DEFAULT '1' COMMENT 'Is system preset tag (1: yes, 0: no)',
  `usage_count` int NOT NULL DEFAULT '0' COMMENT 'Usage count (how many times this tag is used)',
  `sort_order` int NOT NULL DEFAULT '0' COMMENT 'Sort order (for display ordering)',
  `created_by` varchar(128) DEFAULT NULL COMMENT 'Creator',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_tag_type_name` (`tag_type`,`tag_name`,`deleted_at`),
  KEY `idx_tag_definitions_deleted_at` (`deleted_at`),
  KEY `idx_tag_type` (`tag_type`),
  KEY `idx_tag_category` (`tag_category`),
  KEY `idx_is_system` (`is_system`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for address_remarks
-- ----------------------------
DROP TABLE IF EXISTS `address_remarks`;
CREATE TABLE `address_remarks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `address` varchar(128) NOT NULL COMMENT 'Address',
  `chain_id` varchar(50) NOT NULL COMMENT 'Chain ID',
  `remark` text NOT NULL COMMENT 'Remark content',
  `created_by` varchar(128) DEFAULT NULL COMMENT 'Creator',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_address_remark` (`chain_id`,`address`,`deleted_at`),
  KEY `idx_address_remarks_deleted_at` (`deleted_at`),
  KEY `idx_address` (`chain_id`,`address`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for global_tags
-- ----------------------------
DROP TABLE IF EXISTS `global_tags`;
CREATE TABLE `global_tags` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `chain_id` varchar(50) NOT NULL,
  `tag_type` varchar(50) NOT NULL,
  `tag_value` varchar(255) DEFAULT NULL,
  `tag_color` varchar(20) DEFAULT NULL,
  `description` text,
  `created_by` varchar(128) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_global_tag` (`chain_id`,`tag_type`),
  KEY `idx_global_tags_deleted_at` (`deleted_at`),
  KEY `idx_global_tags_tag_type` (`tag_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for contract_source_files
-- ----------------------------
DROP TABLE IF EXISTS `contract_source_files`;
CREATE TABLE `contract_source_files` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `contract_address` varchar(128) NOT NULL COMMENT '合约地址',
  `chain_id` varchar(50) NOT NULL DEFAULT 'injective-1' COMMENT '链ID',
  `file_name` varchar(255) NOT NULL COMMENT '文件名',
  `file_type` varchar(50) NOT NULL COMMENT '文件类型: source 或 abi',
  `file_path` varchar(512) DEFAULT NULL COMMENT '文件路径(可选,用于多文件合约)',
  `file_content` mediumtext COMMENT '文件内容(小文件直接存储)',
  `file_content_storage_key` varchar(512) DEFAULT NULL COMMENT '文件内容在对象存储中的key(大文件)',
  `file_size` bigint DEFAULT '0' COMMENT '文件大小(字节)',
  `file_hash` varchar(128) DEFAULT NULL COMMENT '文件内容的SHA256哈希',
  `mime_type` varchar(100) DEFAULT NULL COMMENT 'MIME类型',
  `description` text COMMENT '描述信息',
  `uploaded_by` varchar(128) DEFAULT NULL COMMENT '上传者地址',
  PRIMARY KEY (`id`),
  KEY `idx_contract_source_files_deleted_at` (`deleted_at`),
  KEY `idx_contract_source_files_contract_address` (`contract_address`),
  KEY `idx_contract_source_files_chain_id` (`chain_id`),
  KEY `idx_contract_source_files_file_type` (`file_type`),
  KEY `idx_contract_source_files_file_hash` (`file_hash`),
  KEY `idx_contract_source_files_contract_chain` (`contract_address`, `chain_id`),
  KEY `idx_contract_source_files_contract_chain_type` (`contract_address`, `chain_id`, `file_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='合约源文件表';

-- ----------------------------
-- Table structure for transaction_data_archives
-- ----------------------------
DROP TABLE IF EXISTS `transaction_data_archives`;
CREATE TABLE `transaction_data_archives` (
  `transaction_id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `data` longtext,
  `data_storage_key` varchar(191) DEFAULT NULL,
  `info` longtext,
  `messages` longtext,
  `messages_storage_key` varchar(191) DEFAULT NULL,
  `logs` longtext,
  `logs_storage_key` varchar(191) DEFAULT NULL,
  `error_log` longtext,
  PRIMARY KEY (`transaction_id`),
  KEY `idx_transaction_data_archives_data_storage_key` (`data_storage_key`),
  KEY `idx_transaction_data_archives_messages_storage_key` (`messages_storage_key`),
  KEY `idx_transaction_data_archives_logs_storage_key` (`logs_storage_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for block_data_archives
-- ----------------------------
DROP TABLE IF EXISTS `block_data_archives`;
CREATE TABLE `block_data_archives` (
  `block_id` bigint unsigned NOT NULL COMMENT '关联blocks.id',
  `txs` longtext COMMENT '区块交易列表（压缩后的JSON）',
  `txs_storage_key` varchar(255) DEFAULT NULL COMMENT 'txs字段的对象存储key（如果已上传到对象存储）',
  `archived_at` datetime(3) DEFAULT NULL COMMENT '归档时间',
  PRIMARY KEY (`block_id`),
  KEY `idx_block_data_archives_txs_storage_key` (`txs_storage_key`),
  KEY `idx_block_data_archives_archived_at` (`archived_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='区块数据归档表';

-- ----------------------------
-- Table structure for transactions
-- ----------------------------
DROP TABLE IF EXISTS `transactions`;
CREATE TABLE `transactions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `tx_data_id` longtext,
  `block_number` bigint unsigned DEFAULT NULL,
  `block_timestamp` longtext,
  `hash` varchar(191) DEFAULT NULL,
  `code` int unsigned DEFAULT NULL,
  `data` longtext,
  `info` longtext,
  `gas_wanted` bigint DEFAULT NULL,
  `gas_used` bigint DEFAULT NULL,
  `gas_fee` longtext,
  `codespace` longtext,
  `events` longtext,
  `tx_type` longtext,
  `message_types` longtext,
  `messages` longtext,
  `signatures` longtext,
  `memo` longtext,
  `tx_number` bigint unsigned DEFAULT NULL,
  `block_unix_timestamp` bigint unsigned DEFAULT NULL,
  `error_log` longtext,
  `logs` longtext,
  `claim_ids` longtext,
  `signer` varchar(128) DEFAULT NULL,
  `to` varchar(128) DEFAULT NULL,
  `value` varchar(128) DEFAULT NULL,
  `archived` tinyint(1) DEFAULT '0',
  `data_storage_key` varchar(255) DEFAULT NULL COMMENT '对象存储key，格式: {type}/{hash[:8]}/{hash}',
  `messages_storage_key` varchar(255) DEFAULT NULL COMMENT '对象存储key，格式: {type}/{hash[:8]}/{hash}',
  `logs_storage_key` varchar(255) DEFAULT NULL COMMENT '对象存储key，格式: {type}/{hash[:8]}/{hash}',
  PRIMARY KEY (`id`),
  KEY `idx_transactions_deleted_at` (`deleted_at`),
  KEY `idx_block_tx` (`block_number`,`tx_number`),
  KEY `idx_transactions_hash` (`hash`),
  KEY `idx_timestamp_code` (`code`,`block_unix_timestamp`),
  KEY `idx_tx_number` (`tx_number`),
  KEY `idx_timestamp_tx` (`block_unix_timestamp`),
  KEY `idx_signer` (`signer`),
  KEY `idx_signer_timestamp` (`signer`),
  KEY `idx_transactions_archived` (`archived`),
  KEY `idx_timestamp_code_deleted` (`block_unix_timestamp`,`code`,`deleted_at`),
  -- Performance indexes for transaction pagination and query patterns (added after optimization analysis)
  KEY `idx_timestamp_tx_pagination` (`block_unix_timestamp` DESC,`tx_number` DESC,`id`),
  KEY `idx_signer_timestamp_tx` (`signer`,`block_unix_timestamp` DESC,`tx_number` DESC),
  -- Covering index for block transaction queries
  KEY `idx_block_tx_covering` (`block_number`,`tx_number`,`hash`,`signer`,`to`,`value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for evm_transactions
-- ----------------------------
DROP TABLE IF EXISTS `evm_transactions`;
CREATE TABLE `evm_transactions` (
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
  `raw_details_json` longtext COMMENT '原始EVM RPC响应JSON，用于解析失败时的降级存储',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_transaction_hash` (`transaction_hash`),
  KEY `idx_evm_transactions_deleted_at` (`deleted_at`),
  KEY `idx_evm_hash` (`evm_hash`),
  KEY `idx_evm_from_address` (`evm_from_address`),
  KEY `idx_evm_to_address` (`evm_to_address`),
  KEY `idx_evm_details_synced` (`evm_details_synced`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='EVM交易表';

-- ----------------------------
-- Table structure for usage_daily_stats
-- ----------------------------
DROP TABLE IF EXISTS `usage_daily_stats`;
CREATE TABLE `usage_daily_stats` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_id` varchar(64) NOT NULL,
  `stat_date` date NOT NULL,
  `total_calls` bigint unsigned NOT NULL DEFAULT '0',
  `success_calls` bigint unsigned NOT NULL DEFAULT '0',
  `failed_calls` bigint unsigned NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_date` (`user_id`,`stat_date`),
  KEY `idx_usage_daily_stats_deleted_at` (`deleted_at`),
  KEY `idx_usage_daily_stats_stat_date` (`stat_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for user_usage_stats
-- ----------------------------
DROP TABLE IF EXISTS `user_usage_stats`;
CREATE TABLE `user_usage_stats` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `user_id` varchar(64) NOT NULL,
  `monthly_used` bigint unsigned NOT NULL DEFAULT '0',
  `monthly_limit` bigint unsigned NOT NULL DEFAULT '100000',
  `total_usage` bigint unsigned NOT NULL DEFAULT '0',
  `success_count` bigint unsigned NOT NULL DEFAULT '0',
  `failed_count` bigint unsigned NOT NULL DEFAULT '0',
  `last_reset_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_user_usage_stats_user_id` (`user_id`),
  KEY `idx_user_usage_stats_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for webhooks
-- ----------------------------
DROP TABLE IF EXISTS `webhooks`;
CREATE TABLE `webhooks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `token_id` bigint unsigned NOT NULL,
  `webhook_url` varchar(512) NOT NULL,
  `secret` varchar(64) NOT NULL,
  `events` longtext,
  `status` varchar(20) NOT NULL DEFAULT 'active',
  `last_triggered_at` datetime(3) DEFAULT NULL,
  `failure_count` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_webhooks_deleted_at` (`deleted_at`),
  KEY `idx_webhooks_token_id` (`token_id`),
  KEY `idx_webhooks_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for ip_blacklist
-- ----------------------------
DROP TABLE IF EXISTS `ip_blacklist`;
CREATE TABLE `ip_blacklist` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `ip_address` varchar(45) NOT NULL COMMENT 'IP地址（支持IPv4和IPv6）',
  `ip_location` varchar(100) DEFAULT NULL COMMENT 'IP归属地',
  `reason` varchar(50) NOT NULL COMMENT '加入原因：batch_register/api_abuse/proxy/spam/other',
  `reason_text` varchar(255) DEFAULT NULL COMMENT '原因描述',
  `block_count` bigint unsigned NOT NULL DEFAULT '0' COMMENT '拦截次数',
  `operator_id` varchar(64) DEFAULT NULL COMMENT '操作人ID',
  `operator_name` varchar(100) DEFAULT NULL COMMENT '操作人名称',
  `block_type` varchar(20) NOT NULL DEFAULT 'all' COMMENT '限制类型：all/register/api',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_ip_blacklist_ip_address` (`ip_address`),
  KEY `idx_ip_blacklist_deleted_at` (`deleted_at`),
  KEY `idx_ip_blacklist_reason` (`reason`),
  KEY `idx_ip_blacklist_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for ip_registration_records
-- ----------------------------
DROP TABLE IF EXISTS `ip_registration_records`;
CREATE TABLE `ip_registration_records` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `ip_address` varchar(45) NOT NULL COMMENT 'IP地址',
  `ip_location` varchar(100) DEFAULT NULL COMMENT 'IP归属地',
  `user_id` varchar(64) NOT NULL COMMENT '注册的用户ID',
  `email` varchar(255) DEFAULT NULL COMMENT '注册邮箱',
  PRIMARY KEY (`id`),
  KEY `idx_ip_registration_records_deleted_at` (`deleted_at`),
  KEY `idx_ip_registration_records_ip_address` (`ip_address`),
  KEY `idx_ip_registration_records_user_id` (`user_id`),
  KEY `idx_ip_registration_records_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- ----------------------------
-- Table structure for email_blacklist
-- ----------------------------
DROP TABLE IF EXISTS `email_blacklist`;
CREATE TABLE `email_blacklist` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `domain` varchar(255) NOT NULL COMMENT '邮箱域名（小写）',
  `status` varchar(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/disabled',
  `block_count` bigint unsigned NOT NULL DEFAULT '0' COMMENT '拦截次数',
  `note` text COMMENT '备注',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_email_blacklist_domain` (`domain`),
  KEY `idx_email_blacklist_deleted_at` (`deleted_at`),
  KEY `idx_email_blacklist_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Initialize some common disposable email domains
INSERT INTO `email_blacklist` (`domain`, `status`, `note`) VALUES
('tempmail.com', 'active', '知名临时邮箱服务'),
('guerrillamail.com', 'active', '游击邮件服务'),
('10minutemail.com', 'active', '10分钟临时邮箱'),
('mailinator.com', 'active', '临时邮箱服务'),
('throwaway.email', 'active', '一次性邮箱')
ON DUPLICATE KEY UPDATE `domain`=`domain`;

SET FOREIGN_KEY_CHECKS = 1;

-- ----------------------------
-- Table structure for validators
-- ----------------------------
DROP TABLE IF EXISTS `validators`;
CREATE TABLE `validators` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `validator_id` varchar(128) DEFAULT NULL COMMENT 'Validator unique identifier',
  `moniker` varchar(256) DEFAULT NULL COMMENT 'Validator display name',
  `operator_address` varchar(128) DEFAULT NULL COMMENT 'Validator operator address',
  `consensus_address` varchar(128) DEFAULT NULL COMMENT 'Validator consensus address',
  `jailed` tinyint(1) DEFAULT '0' COMMENT 'Whether the validator is jailed',
  `status` int DEFAULT NULL COMMENT 'Validator status code',
  `tokens` varchar(128) DEFAULT NULL COMMENT 'Amount of tokens delegated',
  `delegator_shares` varchar(128) DEFAULT NULL COMMENT 'Total delegator shares',
  `description` text COMMENT 'Validator description (JSON format)',
  `unbonding_height` bigint DEFAULT NULL COMMENT 'Height at which unbonding started',
  `unbonding_time` varchar(128) DEFAULT NULL COMMENT 'Time when unbonding completes',
  `commission_rate` varchar(128) DEFAULT NULL COMMENT 'Current commission rate',
  `commission_max_rate` varchar(128) DEFAULT NULL COMMENT 'Maximum commission rate',
  `commission_max_change_rate` varchar(128) DEFAULT NULL COMMENT 'Maximum change rate per period',
  `commission_update_time` varchar(128) DEFAULT NULL COMMENT 'Last commission update time',
  `proposed` bigint unsigned DEFAULT NULL COMMENT 'Number of blocks proposed',
  `signed` bigint unsigned DEFAULT NULL COMMENT 'Number of blocks signed',
  `missed` bigint unsigned DEFAULT NULL COMMENT 'Number of blocks missed',
  `timestamp` varchar(128) DEFAULT NULL COMMENT 'Last update timestamp',
  `uptimes` json DEFAULT NULL COMMENT 'Validator uptime information (JSON array)',
  `slashing_events` json DEFAULT NULL COMMENT 'Slashing event information (JSON array)',
  `uptime_percentage` double DEFAULT NULL COMMENT 'Validator uptime percentage',
  `image_url` varchar(512) DEFAULT NULL COMMENT 'Validator image URL',
  `registration_time` bigint DEFAULT NULL COMMENT 'Registration timestamp (first block time)',
  `wallet_address` varchar(128) DEFAULT NULL COMMENT 'Validator wallet address',
  PRIMARY KEY (`id`),
  KEY `idx_validators_deleted_at` (`deleted_at`),
  KEY `idx_validators_operator_address` (`operator_address`),
  KEY `idx_validators_status` (`status`),
  KEY `idx_validators_jailed` (`jailed`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Validator information table';

-- Note: block_signers table has been removed in favor of validator_block_stats (aggregated statistics table)
-- This reduces data volume by 99.8% (100 records per period vs 50,000 records per 1000 blocks)
-- If detailed block-by-block query is needed in the future, this table can be re-added

-- ----------------------------
-- Table structure for validator_block_stats
-- ----------------------------
-- This table stores aggregated validator block statistics by period.
-- This is the most optimized approach: only stores aggregated counts, not individual block records.
-- Each period (e.g., 1000 blocks) aggregates statistics for all validators.
-- Data volume: For 100 validators, 1000 blocks per period = 100 records per period (vs 50,000 in block_signers)
DROP TABLE IF EXISTS `validator_block_stats`;
CREATE TABLE `validator_block_stats` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `validator_operator_address` varchar(128) NOT NULL COMMENT 'Validator operator address',
  `period_start_height` bigint unsigned NOT NULL COMMENT 'Period start block height',
  `period_end_height` bigint unsigned NOT NULL COMMENT 'Period end block height',
  `proposed_count` bigint unsigned DEFAULT 0 COMMENT 'Number of blocks proposed in this period',
  `signed_count` bigint unsigned DEFAULT 0 COMMENT 'Number of blocks signed in this period',
  `should_sign_count` bigint unsigned DEFAULT 0 COMMENT 'Number of blocks validator should have signed (based on validator set)',
  `missed_count` bigint unsigned DEFAULT 0 COMMENT 'Number of blocks missed in this period',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_validator_period` (`validator_operator_address`, `period_start_height`),
  KEY `idx_validator_stats_validator` (`validator_operator_address`),
  KEY `idx_validator_stats_period` (`period_start_height`, `period_end_height`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Validator block statistics by period (aggregated)';


-- ============================================================================
-- PRESET TAG DEFINITIONS DATA
-- ============================================================================

-- Insert preset contract tags - normal (green) category
INSERT INTO `tag_definitions` (`tag_type`, `tag_name`, `tag_category`, `tag_color`, `description`, `is_system`, `sort_order`, `created_by`) VALUES
('contract', '官方', 'normal', '#52c41a', '官方项目标签', 1, 1, 'system'),
('contract', '稳定币', 'normal', '#52c41a', '稳定币标签', 1, 2, 'system'),
('contract', 'DeFi', 'normal', '#52c41a', 'DeFi协议标签', 1, 3, 'system'),
('contract', 'CEX', 'normal', '#52c41a', '中心化交易所标签', 1, 4, 'system'),
('contract', '跨链桥', 'normal', '#52c41a', '跨链桥标签', 1, 5, 'system'),
('contract', 'DEX', 'normal', '#52c41a', '去中心化交易所标签', 1, 6, 'system'),
('contract', 'NFT', 'normal', '#52c41a', 'NFT相关标签', 1, 7, 'system');

-- Insert preset contract tags - abnormal (red) category
INSERT INTO `tag_definitions` (`tag_type`, `tag_name`, `tag_category`, `tag_color`, `description`, `is_system`, `sort_order`, `created_by`) VALUES
('contract', '诈骗', 'abnormal', '#ff4d4f', '诈骗项目标签', 1, 100, 'system'),
('contract', '洗钱', 'abnormal', '#ff4d4f', '洗钱相关标签', 1, 101, 'system'),
('contract', '高风险', 'abnormal', '#ff4d4f', '高风险项目标签', 1, 102, 'system'),
('contract', '已废弃', 'abnormal', '#ff4d4f', '已废弃项目标签', 1, 103, 'system');

-- Insert preset address tags
INSERT INTO `tag_definitions` (`tag_type`, `tag_name`, `tag_category`, `tag_color`, `description`, `is_system`, `sort_order`, `created_by`) VALUES
('address', '交易所', 'normal', '#1890ff', '交易所地址', 1, 1, 'system'),
('address', '做市商', 'normal', '#1890ff', '做市商地址', 1, 2, 'system'),
('address', '巨鲸', 'normal', '#1890ff', '巨鲸地址', 1, 3, 'system'),
('address', '内部账户', 'normal', '#1890ff', '内部账户', 1, 4, 'system');


-- ============================================================================
-- ADDITIONAL INDEXES FOR QUERY OPTIMIZATION
-- ============================================================================

-- Note: Some indexes may already exist from table creation above.
-- For MySQL 5.6 compatibility, we create indexes directly.
-- If an index already exists, MySQL will return an error which can be safely ignored.
-- For MySQL 5.7.4+, IF NOT EXISTS syntax is supported, but we use direct CREATE for compatibility.

-- ============================================================================
-- TRANSACTION TABLE ADDITIONAL INDEXES
-- ============================================================================

-- Index for 'to' field queries (used in enhanceBlockDTOWithTransactions)
-- Query: SELECT hash, signer, to, value FROM transactions WHERE block_number = ?
-- Note: If index already exists, MySQL will return error which can be safely ignored
CREATE INDEX idx_transaction_to ON transactions(`to`);

-- Index for active addresses query (getActiveAddresses24h)
-- Query: WHERE block_unix_timestamp >= ? AND signer != ''
CREATE INDEX idx_timestamp_signer_distinct ON transactions(block_unix_timestamp, signer);

-- Index for storage migration queries (IS NULL checks on storage keys)
-- Query: WHERE (data_storage_key IS NULL OR data_storage_key = '') AND ...
-- Note: VARCHAR columns can be indexed directly without prefix length
CREATE INDEX idx_data_storage_key ON transactions(data_storage_key);
CREATE INDEX idx_messages_storage_key ON transactions(messages_storage_key);
CREATE INDEX idx_logs_storage_key ON transactions(logs_storage_key);

-- Composite index for archived transactions queries
-- Query: WHERE archived = ? ORDER BY block_unix_timestamp DESC, tx_number DESC
CREATE INDEX idx_archived_timestamp_tx ON transactions(archived, block_unix_timestamp DESC, tx_number DESC);

-- Index for message_types JSON queries (MySQL 5.7+)
-- Query: WHERE message_types LIKE ? OR JSON_CONTAINS(message_types, ?)
-- Use prefix index directly on JSON column (simpler, works for LIKE queries)
CREATE INDEX idx_message_types_prefix ON transactions(message_types(255));

-- Performance optimization indexes (added for performance requirements)
-- Query: SELECT * FROM transactions WHERE signer = ? AND archived = 0 ORDER BY block_unix_timestamp DESC LIMIT 20
-- Note: MySQL doesn't support partial indexes (WHERE clause), so we create full index
-- The application should filter archived = 0 in WHERE clause
CREATE INDEX idx_transactions_signer_timestamp ON transactions(signer, block_unix_timestamp DESC);

-- Query: SELECT * FROM transactions WHERE block_number = ? AND code = ?
-- Note: MySQL doesn't support partial indexes, so we create full index
-- The application should filter archived = 0 in WHERE clause
CREATE INDEX idx_transactions_block_code ON transactions(block_number, code);

-- Query: SELECT * FROM transactions WHERE (signer = ? OR to = ?) AND archived = 0 ORDER BY block_unix_timestamp DESC
-- Note: idx_transactions_signer_timestamp already covers signer queries
-- This index covers 'to' field queries
CREATE INDEX idx_transactions_to_timestamp ON transactions(`to`, block_unix_timestamp DESC);

-- Query: SELECT * FROM transactions WHERE code != 0 AND archived = 0 ORDER BY block_unix_timestamp DESC
-- Note: MySQL doesn't support partial indexes, so we create full index
-- The application should filter code != 0 AND archived = 0 in WHERE clause
CREATE INDEX idx_transactions_code_timestamp ON transactions(code, block_unix_timestamp DESC);

-- Query: Statistics queries - SELECT * FROM transactions WHERE archived = 0 ORDER BY block_unix_timestamp DESC
-- Note: idx_timestamp_tx already exists in table creation (line 313) which covers this query pattern

-- ============================================================================
-- EVM TRANSACTIONS TABLE ADDITIONAL INDEXES
-- ============================================================================

-- Composite index for EVM account transactions query
-- Query: WHERE (evm_from_address = ? OR evm_to_address = ?) ORDER BY evm_details_sync_at DESC
CREATE INDEX idx_evm_from_sync_at ON evm_transactions(evm_from_address, evm_details_sync_at DESC);
CREATE INDEX idx_evm_to_sync_at ON evm_transactions(evm_to_address, evm_details_sync_at DESC);

-- Composite index for EVM transactions sync status queries
-- Query: WHERE evm_details_synced = ? AND evm_hash != ''
CREATE INDEX idx_evm_synced_hash ON evm_transactions(evm_details_synced, evm_hash);

-- Index for storage key queries
-- Query: WHERE evm_details_storage_key IS NOT NULL AND evm_details_storage_key != ''
CREATE INDEX idx_evm_details_storage_key ON evm_transactions(evm_details_storage_key);

-- ============================================================================
-- BLOCKS TABLE ADDITIONAL INDEXES
-- ============================================================================

-- Performance optimization indexes (added for performance requirements)
-- Query: SELECT * FROM blocks ORDER BY height DESC LIMIT 20
-- Note: idx_height_desc already exists in table creation (line 188)
-- Query: SELECT * FROM blocks WHERE height >= ? AND height <= ?
-- Note: idx_height already exists in table creation (line 187)

-- ============================================================================
-- ACCOUNT BALANCE TABLE ADDITIONAL INDEXES
-- ============================================================================

-- Composite index for address + denom queries
-- Query: WHERE address = ? AND denom = ?
CREATE INDEX idx_account_balance_address_denom ON account_balances(address, denom);

-- Note: idx_account_balances_address already exists in table creation (line 34)

-- ============================================================================
-- CONTRACT INFO TABLE ADDITIONAL INDEXES
-- ============================================================================

-- Index for name and token_name LIKE queries
-- Query: WHERE name LIKE ? OR token_name LIKE ?
CREATE INDEX idx_contract_name_prefix ON contract_infos(name(50));
CREATE INDEX idx_contract_token_name_prefix ON contract_infos(token_name(50));

-- Full-text index for name and token_name (MySQL 5.6+)
-- This enables MATCH() AGAINST() queries for better full-text search
-- Note: FULLTEXT INDEX doesn't support IF NOT EXISTS, so we check first or ignore error
ALTER TABLE contract_infos ADD FULLTEXT INDEX idx_contract_name_fulltext (name, token_name);

-- Composite index for common filter combinations
-- Query: WHERE chain_id = ? AND contract_type = ? AND verification_status = ?
CREATE INDEX idx_contract_filters ON contract_infos(chain_id, contract_type, verification_status);

-- ============================================================================
-- ADDRESS LABEL TABLE ADDITIONAL INDEXES
-- ============================================================================

-- Index for label_value LIKE queries
-- Query: WHERE label_value LIKE ?
CREATE INDEX idx_address_label_value_prefix ON address_labels(label_value(50));

-- Composite index for list queries with filters
-- Query: WHERE chain_id = ? AND label_type = ? ORDER BY id DESC
CREATE INDEX idx_address_label_list ON address_labels(chain_id, label_type, id DESC);

-- ============================================================================
-- VERIFICATION AND COMPLETION
-- ============================================================================

-- Verify all indexes were created successfully
-- Note: This will show all indexes including duplicates, which is expected
SELECT 'Index creation completed. Verifying indexes...' AS status;

-- Show index summary
SELECT
    TABLE_NAME,
    INDEX_NAME,
    GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', ') AS COLUMNS,
    CARDINALITY,
    INDEX_TYPE
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
AND TABLE_NAME IN ('transactions', 'evm_transactions', 'blocks', 'account_balances', 'transaction_data_archives', 'contract_infos', 'address_labels')
AND INDEX_NAME LIKE 'idx_%'
GROUP BY TABLE_NAME, INDEX_NAME, CARDINALITY, INDEX_TYPE
ORDER BY TABLE_NAME, INDEX_NAME;

SELECT 'Database initialization completed successfully!' AS status;

