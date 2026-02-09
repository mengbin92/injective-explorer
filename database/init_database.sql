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
  `creator` varchar(128) DEFAULT NULL COMMENT '合约创建者/部署者地址',
  `decimals` int DEFAULT 18 COMMENT 'ERC20代币精度（默认18位，USDC/USDT为6位）',
  `metadata` json DEFAULT NULL,
  `code_id` bigint DEFAULT NULL COMMENT 'Wasm code ID',
  `label` varchar(255) DEFAULT NULL COMMENT 'Contract label',
  `init_msg` text DEFAULT NULL COMMENT 'Initialization message (JSON)',
  `admin` varchar(128) DEFAULT NULL COMMENT 'Admin address',
  `execute_count` bigint DEFAULT 0 COMMENT 'Total execution count',
  `last_executed_at` datetime(3) DEFAULT NULL COMMENT 'Last execution timestamp',
  `instantiated_at` datetime(3) DEFAULT NULL COMMENT 'Instantiation timestamp',
  `block_height` bigint DEFAULT NULL COMMENT 'Creation block height',
  `tx_hash` varchar(128) DEFAULT NULL COMMENT 'Creation transaction hash',
  `total_supply` varchar(255) DEFAULT NULL COMMENT 'CW20 total supply',
  PRIMARY KEY (`id`),
  KEY `idx_contract_infos_deleted_at` (`deleted_at`),
  KEY `idx_token_symbol` (`token_symbol`),
  KEY `idx_contract_infos_contract_type` (`contract_type`),
  KEY `idx_contract_infos_chain_id` (`chain_id`),
  KEY `idx_contract_infos_verification_status` (`verification_status`),
  KEY `idx_contract_infos_creator` (`creator`),
  KEY `idx_contract_infos_decimals` (`decimals`),
  KEY `idx_contract_infos_code_id` (`code_id`),
  KEY `idx_contract_infos_block_height` (`block_height`),
  KEY `idx_contract_infos_label` (`label`),
  KEY `idx_contract_infos_admin` (`admin`)
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
  `messages` longtext COMMENT 'Transaction messages in formatted JSON structure. Messages are parsed and formatted at WRITE time (when saving to database) from protobuf to human-readable JSON according to the PDF specification. Format: {\"messages\":[{\"@type\":\"...\",\"field\":\"value\",...}],\"memo\":\"...\"} All Cosmos SDK and Injective module types are supported. Addresses are automatically converted to EVM format (0x...). For large transactions, content may be stored in object storage with key in messages_storage_key.',
  `signatures` longtext,
  `memo` longtext,
  `tx_number` bigint unsigned DEFAULT NULL,
  `block_unix_timestamp` bigint unsigned DEFAULT NULL,
  `error_log` longtext,
  `logs` longtext,
  `claim_ids` longtext,
  `signer` varchar(128) DEFAULT NULL,
  `to` varchar(128) DEFAULT NULL,
  `values` longtext DEFAULT NULL COMMENT 'transaction values as JSON array, e.g., ["1000000inj", "500atom"]',
  `archived` tinyint(1) DEFAULT '0',
  `data_storage_key` varchar(255) DEFAULT NULL COMMENT '对象存储key，格式: {type}/{hash[:8]}/{hash}',
  `messages_storage_key` varchar(255) DEFAULT NULL COMMENT '对象存储key，格式: {type}/{hash[:8]}/{hash}',
  `logs_storage_key` varchar(255) DEFAULT NULL COMMENT '对象存储key，格式: {type}/{hash[:8]}/{hash}',
  `has_token_transfer` tinyint(1) DEFAULT '0' NOT NULL COMMENT 'Indicates if this transaction has ERC20/ERC721 token transfers (for efficient filtering)',
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
  KEY `idx_transactions_has_token_transfer` (`has_token_transfer`)
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
-- Table structure for token_market_cache
-- ----------------------------
DROP TABLE IF EXISTS `token_market_cache`;
CREATE TABLE `token_market_cache` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `token_symbol` varchar(50) NOT NULL COMMENT '代币符号',
  `contract_address` varchar(128) DEFAULT NULL COMMENT '合约地址',
  `chain_id` varchar(50) DEFAULT 'injective-1' COMMENT '链ID',
  `price_usd` decimal(30,18) DEFAULT NULL COMMENT 'USD价格',
  `price_change_24h` decimal(10,4) DEFAULT NULL COMMENT '24小时价格变化百分比',
  `high_24h` decimal(30,18) DEFAULT 0 COMMENT '24小时最高价(USD)',
  `low_24h` decimal(30,18) DEFAULT 0 COMMENT '24小时最低价(USD)',
  `market_cap_usd` decimal(40,2) DEFAULT NULL COMMENT '市值（USD）',
  `fully_diluted_market_cap` decimal(40,2) DEFAULT NULL COMMENT '完全稀释市值（USD）',
  `total_volume_24h` decimal(40,2) DEFAULT NULL COMMENT '24小时交易量（USD）',
  `holders_count` bigint DEFAULT NULL COMMENT '持有者数量',
  `total_supply` decimal(65,0) DEFAULT NULL COMMENT '总供应量（原始单位）',
  `circulating_supply` decimal(65,0) DEFAULT NULL COMMENT '流通供应量（原始单位）',
  `max_supply` decimal(65,0) DEFAULT NULL COMMENT '最大供应量（原始单位，NULL表示无上限）',
  `last_updated` bigint DEFAULT NULL COMMENT '数据最后更新时间戳',
  `data_source` varchar(50) DEFAULT 'coingecko' COMMENT '数据来源',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_token_symbol_chain` (`token_symbol`,`chain_id`),
  KEY `idx_token_market_cache_deleted_at` (`deleted_at`),
  KEY `idx_contract_address` (`contract_address`),
  KEY `idx_price_usd` (`price_usd`),
  KEY `idx_market_cap_usd` (`market_cap_usd`),
  KEY `idx_token_market_cache_high_24h` (`high_24h`),
  KEY `idx_token_market_cache_low_24h` (`low_24h`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='代币市场数据预计算表';

-- ----------------------------
-- Table structure for token_transfers
-- ----------------------------
DROP TABLE IF EXISTS `token_transfers`;
CREATE TABLE `token_transfers` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `transaction_hash` varchar(128) NOT NULL COMMENT 'Cosmos交易hash，关联transactions.hash',
  `evm_hash` varchar(128) DEFAULT NULL COMMENT 'EVM交易hash',
  `contract_address` varchar(128) NOT NULL COMMENT 'Token合约地址',
  `from_address` varchar(128) NOT NULL COMMENT '发送地址',
  `to_address` varchar(128) NOT NULL COMMENT '接收地址',
  `amount` varchar(128) NOT NULL COMMENT '转账金额（原始单位）',
  `method` varchar(50) DEFAULT 'Transfer' COMMENT '转账类型：Transfer, Mint, Burn',
  `block_number` bigint unsigned DEFAULT NULL COMMENT '区块高度',
  `block_timestamp` bigint unsigned DEFAULT NULL COMMENT '区块Unix时间戳',
  `log_index` int unsigned DEFAULT NULL COMMENT '交易日志索引',
  `chain_id` varchar(50) DEFAULT 'injective-1' COMMENT '链ID',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_tx_log_index` (`transaction_hash`,`log_index`),
  KEY `idx_token_transfers_deleted_at` (`deleted_at`),
  KEY `idx_contract_address_timestamp` (`contract_address`,`block_timestamp`),
  KEY `idx_from_address_timestamp` (`from_address`,`block_timestamp`),
  KEY `idx_to_address_timestamp` (`to_address`,`block_timestamp`),
  KEY `idx_block_number` (`block_number`),
  KEY `idx_method` (`method`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Token转账记录表';

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
  `domain` varchar(255) NOT NULL COMMENT '邮箱域名（小写）',
  `status` varchar(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/disabled',
  `block_count` bigint unsigned NOT NULL DEFAULT '0' COMMENT '拦截次数',
  `note` text COMMENT '备注',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_email_blacklist_domain` (`domain`),
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

SET FOREIGN_KEY_CHECKS = 0;

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

-- ----------------------------
-- Table structure for onchain_prices
-- ----------------------------
DROP TABLE IF EXISTS `onchain_prices`;
CREATE TABLE `onchain_prices` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `symbol` varchar(50) NOT NULL COMMENT '代币符号',
  `denom` varchar(100) DEFAULT NULL COMMENT '代币denom',
  `market_id` varchar(100) DEFAULT NULL COMMENT '市场ID',
  `price_usd` decimal(32,18) DEFAULT NULL COMMENT 'USD价格',
  `price_raw` varchar(100) DEFAULT NULL COMMENT '原始价格（18位小数）',
  `best_buy` varchar(100) DEFAULT NULL COMMENT '最优买价',
  `best_sell` varchar(100) DEFAULT NULL COMMENT '最优卖价',
  `source` varchar(50) DEFAULT NULL COMMENT '价格来源：spot_market/derivative_market',
  `timestamp` bigint DEFAULT NULL COMMENT '价格时间戳',
  `block_height` bigint DEFAULT NULL COMMENT '区块高度',
  `confidence` decimal(5,4) DEFAULT NULL COMMENT '置信度 0-1',
  PRIMARY KEY (`id`),
  KEY `idx_onchain_prices_symbol` (`symbol`),
  KEY `idx_onchain_prices_denom` (`denom`),
  KEY `idx_onchain_prices_market_id` (`market_id`),
  KEY `idx_onchain_prices_timestamp` (`timestamp`),
  KEY `idx_symbol_time` (`symbol`,`timestamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='链上价格记录表';

-- ----------------------------
-- Table structure for token_market_mappings
-- ----------------------------
DROP TABLE IF EXISTS `token_market_mappings`;
CREATE TABLE `token_market_mappings` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `symbol` varchar(50) DEFAULT NULL COMMENT '代币符号',
  `denom` varchar(100) DEFAULT NULL COMMENT '代币denom',
  `spot_market_id` varchar(100) DEFAULT NULL COMMENT '现货市场ID',
  `is_active` tinyint(1) DEFAULT '1' COMMENT '是否激活',
  `priority` int DEFAULT '0' COMMENT '优先级',
  `last_updated` datetime(3) DEFAULT NULL COMMENT '最后更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_token_market_mappings_denom` (`denom`),
  KEY `idx_token_market_mappings_symbol` (`symbol`),
  KEY `idx_token_market_mappings_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='代币与市场映射表';


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

-- ----------------------------
-- Table structure for ibc_transfer_txs
-- ----------------------------
DROP TABLE IF EXISTS `ibc_transfer_txs`;
CREATE TABLE `ibc_transfer_txs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `sender` varchar(128) NOT NULL COMMENT '发送地址',
  `receiver` varchar(128) NOT NULL COMMENT '接收地址',
  `source_port` varchar(100) DEFAULT NULL COMMENT '源端口',
  `source_channel` varchar(100) DEFAULT NULL COMMENT '源通道',
  `destination_port` varchar(100) DEFAULT NULL COMMENT '目标端口',
  `destination_channel` varchar(100) DEFAULT NULL COMMENT '目标通道',
  `amount` varchar(100) DEFAULT NULL COMMENT '转账金额',
  `denom` varchar(100) DEFAULT NULL COMMENT '代币denom',
  `timeout_height` varchar(100) DEFAULT NULL COMMENT '超时高度',
  `timeout_timestamp` bigint unsigned DEFAULT NULL COMMENT '超时时间戳',
  `packet_sequence` bigint unsigned DEFAULT NULL COMMENT '包序列号',
  `data_hex` blob COMMENT '原始数据(hex)',
  `state` varchar(50) DEFAULT 'completed' COMMENT '状态: completed/pending/failed',
  `tx_hash` varchar(128) NOT NULL COMMENT '交易hash',
  `block_height` bigint unsigned DEFAULT NULL COMMENT '区块高度',
  `block_timestamp` bigint unsigned DEFAULT NULL COMMENT '区块时间戳',
  PRIMARY KEY (`id`),
  KEY `idx_ibc_transfer_txs_deleted_at` (`deleted_at`),
  KEY `idx_ibc_transfer_txs_sender` (`sender`),
  KEY `idx_ibc_transfer_txs_receiver` (`receiver`),
  KEY `idx_ibc_transfer_txs_source_channel` (`source_channel`),
  KEY `idx_ibc_transfer_txs_destination_channel` (`destination_channel`),
  KEY `idx_ibc_transfer_txs_tx_hash` (`tx_hash`),
  KEY `idx_ibc_transfer_txs_block_height` (`block_height`),
  KEY `idx_ibc_transfer_txs_block_timestamp` (`block_timestamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='IBC跨链转账交易表';

-- ----------------------------
-- Table structure for peggy_deposit_txs
-- ----------------------------
DROP TABLE IF EXISTS `peggy_deposit_txs`;
CREATE TABLE `peggy_deposit_txs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `sender` varchar(128) NOT NULL COMMENT '发送地址(以太坊地址)',
  `receiver` varchar(128) NOT NULL COMMENT '接收地址(injective地址)',
  `amount` varchar(100) DEFAULT NULL COMMENT '存款金额',
  `denom` varchar(100) DEFAULT NULL COMMENT '代币denom',
  `event_nonce` bigint unsigned DEFAULT NULL COMMENT '事件nonce',
  `event_height` bigint unsigned DEFAULT NULL COMMENT '事件高度',
  `orchestrator_address` varchar(128) DEFAULT NULL COMMENT '协调者地址',
  `state` varchar(50) DEFAULT 'completed' COMMENT '状态: completed/pending/failed',
  `claim_type` int DEFAULT 1 COMMENT '声明类型: 1=deposit',
  `tx_hash` varchar(128) NOT NULL COMMENT '交易hash',
  `eth_tx_hash` varchar(128) DEFAULT NULL COMMENT '以太坊链交易hash',
  `block_height` bigint unsigned DEFAULT NULL COMMENT '区块高度',
  `block_timestamp` bigint unsigned DEFAULT NULL COMMENT '区块时间戳',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_peggy_deposit_event_nonce` (`event_nonce`),
  KEY `idx_peggy_deposit_txs_deleted_at` (`deleted_at`),
  KEY `idx_peggy_deposit_txs_sender` (`sender`),
  KEY `idx_peggy_deposit_txs_receiver` (`receiver`),
  KEY `idx_peggy_deposit_txs_tx_hash` (`tx_hash`),
  KEY `idx_peggy_deposit_txs_eth_tx_hash` (`eth_tx_hash`),
  KEY `idx_peggy_deposit_txs_block_height` (`block_height`),
  KEY `idx_peggy_deposit_txs_block_timestamp` (`block_timestamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Peggy跨链存款交易表';

-- ----------------------------
-- Table structure for peggy_withdrawal_txs
-- ----------------------------
DROP TABLE IF EXISTS `peggy_withdrawal_txs`;
CREATE TABLE `peggy_withdrawal_txs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `sender` varchar(128) NOT NULL COMMENT '发送地址(injective地址)',
  `receiver` varchar(128) NOT NULL COMMENT '接收地址(以太坊地址)',
  `amount` varchar(100) DEFAULT NULL COMMENT '提款金额',
  `denom` varchar(100) DEFAULT NULL COMMENT '代币denom',
  `bridge_fee` varchar(100) DEFAULT NULL COMMENT '跨链桥费用',
  `outgoing_tx_id` bigint unsigned DEFAULT NULL COMMENT '出账交易ID',
  `batch_timeout` bigint unsigned DEFAULT NULL COMMENT '批次超时时间',
  `batch_nonce` bigint unsigned DEFAULT NULL COMMENT '批次nonce',
  `orchestrator_address` varchar(128) DEFAULT NULL COMMENT '协调者地址',
  `event_nonce` bigint unsigned DEFAULT NULL COMMENT '事件nonce',
  `event_height` bigint unsigned DEFAULT NULL COMMENT '事件高度',
  `state` varchar(50) DEFAULT 'pending' COMMENT '状态: pending/completed/failed',
  `claim_type` int DEFAULT 2 COMMENT '声明类型: 2=withdrawal',
  `tx_hash` varchar(128) NOT NULL COMMENT '交易hash',
  `eth_tx_hash` varchar(128) DEFAULT NULL COMMENT '以太坊链交易hash',
  `block_height` bigint unsigned DEFAULT NULL COMMENT '区块高度',
  `block_timestamp` bigint unsigned DEFAULT NULL COMMENT '区块时间戳',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_peggy_withdrawal_outgoing_tx_id` (`outgoing_tx_id`),
  KEY `idx_peggy_withdrawal_txs_deleted_at` (`deleted_at`),
  KEY `idx_peggy_withdrawal_txs_sender` (`sender`),
  KEY `idx_peggy_withdrawal_txs_receiver` (`receiver`),
  KEY `idx_peggy_withdrawal_txs_tx_hash` (`tx_hash`),
  KEY `idx_peggy_withdrawal_txs_eth_tx_hash` (`eth_tx_hash`),
  KEY `idx_peggy_withdrawal_txs_block_height` (`block_height`),
  KEY `idx_peggy_withdrawal_txs_block_timestamp` (`block_timestamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Peggy跨链提款交易表';

-- ----------------------------
-- Table structure for wasm_codes
-- ----------------------------
DROP TABLE IF EXISTS `wasm_codes`;
CREATE TABLE `wasm_codes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `code_id` bigint NOT NULL COMMENT 'Wasm code ID',
  `creator` varchar(128) NOT NULL COMMENT 'Creator address',
  `data_hash` varchar(128) NOT NULL COMMENT 'Code hash (SHA256)',
  `instantiate_permission` text COMMENT 'Instantiation permission (JSON)',
  `tx_hash` varchar(128) DEFAULT NULL COMMENT 'Upload transaction hash',
  `block_height` bigint NOT NULL COMMENT 'Upload block height',
  `contract_type` varchar(100) DEFAULT NULL COMMENT 'Contract type (cw20, cw721, etc)',
  `version` varchar(50) DEFAULT NULL COMMENT 'Contract version',
  `source_url` text COMMENT 'Source code URL',
  `verified` tinyint(1) DEFAULT '0' COMMENT 'Whether code is verified',
  `instantiate_count` bigint unsigned DEFAULT 0 COMMENT 'Number of instantiations',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_wasm_codes_code_id` (`code_id`),
  KEY `idx_wasm_codes_deleted_at` (`deleted_at`),
  KEY `idx_wasm_codes_creator` (`creator`),
  KEY `idx_wasm_codes_block_height` (`block_height`),
  KEY `idx_wasm_codes_contract_type` (`contract_type`),
  KEY `idx_wasm_codes_verified` (`verified`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Wasm code registry';

-- ----------------------------
-- Table structure for contract_executions
-- ----------------------------
DROP TABLE IF EXISTS `contract_executions`;
CREATE TABLE `contract_executions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `contract_address` varchar(128) NOT NULL COMMENT 'Contract address',
  `tx_hash` varchar(128) NOT NULL COMMENT 'Transaction hash',
  `sender` varchar(128) NOT NULL COMMENT 'Sender address',
  `msg` text NOT NULL COMMENT 'Execution message (JSON)',
  `funds` text COMMENT 'Attached funds (JSON)',
  `block_height` bigint NOT NULL COMMENT 'Block height',
  `unix_timestamp` bigint NOT NULL COMMENT 'Unix timestamp in milliseconds',
  `success` tinyint(1) NOT NULL COMMENT 'Execution success status',
  PRIMARY KEY (`id`),
  KEY `idx_contract_executions_contract_address` (`contract_address`),
  KEY `idx_contract_executions_tx_hash` (`tx_hash`),
  KEY `idx_contract_executions_sender` (`sender`),
  KEY `idx_contract_executions_block_height` (`block_height`),
  KEY `idx_contract_executions_timestamp` (`unix_timestamp`),
  KEY `idx_contract_executions_success` (`success`),
  KEY `idx_contract_executions_address_time` (`contract_address`,`unix_timestamp` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Contract execution records';

-- ----------------------------
-- Table structure for cw20_balances
-- ----------------------------
DROP TABLE IF EXISTS `cw20_balances`;
CREATE TABLE `cw20_balances` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `contract_address` varchar(128) NOT NULL COMMENT 'CW20 contract address',
  `owner_address` varchar(128) NOT NULL COMMENT 'Token holder address',
  `balance` varchar(255) NOT NULL COMMENT 'Token balance (string for large numbers)',
  `updated_at` datetime(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'Last balance update timestamp',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_cw20_contract_owner` (`contract_address`,`owner_address`),
  KEY `idx_cw20_balances_contract_address` (`contract_address`),
  KEY `idx_cw20_balances_owner_address` (`owner_address`),
  KEY `idx_cw20_balances_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='CW20 token balances';

-- ----------------------------
-- Table structure for cw20_transfers
-- ----------------------------
DROP TABLE IF EXISTS `cw20_transfers`;
CREATE TABLE `cw20_transfers` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `contract_address` varchar(128) NOT NULL COMMENT 'CW20 contract address',
  `from_address` varchar(128) NOT NULL COMMENT 'Sender address',
  `to_address` varchar(128) NOT NULL COMMENT 'Receiver address',
  `amount` varchar(255) NOT NULL COMMENT 'Transfer amount',
  `tx_hash` varchar(128) NOT NULL COMMENT 'Transaction hash',
  `block_height` bigint NOT NULL COMMENT 'Block height',
  `unix_timestamp` bigint NOT NULL COMMENT 'Unix timestamp in milliseconds',
  PRIMARY KEY (`id`),
  KEY `idx_cw20_transfers_contract_address` (`contract_address`),
  KEY `idx_cw20_transfers_from_address` (`from_address`),
  KEY `idx_cw20_transfers_to_address` (`to_address`),
  KEY `idx_cw20_transfers_tx_hash` (`tx_hash`),
  KEY `idx_cw20_transfers_block_height` (`block_height`),
  KEY `idx_cw20_transfers_timestamp` (`unix_timestamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='CW20 transfer history';

SET FOREIGN_KEY_CHECKS = 1;

