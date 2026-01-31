-- Injective Explorer 跨链交易数据库表结构
-- 版本: 1.0
-- 日期: 2024-01-30

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ============================================================================
-- IBC 转账交易表
-- ============================================================================
DROP TABLE IF EXISTS `ibc_transfer_txs`;
CREATE TABLE `ibc_transfer_txs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,

  -- 核心字段
  `sender` varchar(128) NOT NULL COMMENT '发送方地址',
  `receiver` varchar(128) NOT NULL COMMENT '接收方地址',
  `source_port` varchar(100) DEFAULT NULL COMMENT '源端口',
  `source_channel` varchar(100) DEFAULT NULL COMMENT '源通道',
  `destination_port` varchar(100) DEFAULT NULL COMMENT '目标端口',
  `destination_channel` varchar(100) DEFAULT NULL COMMENT '目标通道',
  `amount` varchar(100) DEFAULT NULL COMMENT '转账金额',
  `denom` varchar(100) DEFAULT NULL COMMENT '代币种类',
  `timeout_height` varchar(100) DEFAULT NULL COMMENT '超时高度',
  `timeout_timestamp` bigint unsigned DEFAULT NULL COMMENT '超时时间戳（纳秒）',
  `packet_sequence` bigint unsigned DEFAULT NULL COMMENT '数据包序列号',
  `state` varchar(50) DEFAULT 'completed' COMMENT '交易状态：pending, completed, failed, timeout',

  -- 关联字段
  `tx_hash` varchar(128) NOT NULL COMMENT '交易哈希',
  `block_height` bigint unsigned NOT NULL COMMENT '区块高度',
  `block_timestamp` bigint unsigned NOT NULL COMMENT '区块时间戳（秒）',

  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ibc_tx_hash_seq` (`tx_hash`, `packet_sequence`),
  KEY `idx_ibc_sender` (`sender`),
  KEY `idx_ibc_receiver` (`receiver`),
  KEY `idx_ibc_src_channel` (`source_channel`),
  KEY `idx_ibc_dest_channel` (`destination_channel`),
  KEY `idx_ibc_block_height` (`block_height`),
  KEY `idx_ibc_block_timestamp` (`block_timestamp`),
  KEY `idx_ibc_state` (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='IBC 转账交易表';

-- ============================================================================
-- Peggy 存款交易表（以太坊→Injective）
-- ============================================================================
DROP TABLE IF EXISTS `peggy_deposit_txs`;
CREATE TABLE `peggy_deposit_txs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,

  -- 核心字段
  `sender` varchar(128) NOT NULL COMMENT '以太坊发送地址（0x...）',
  `receiver` varchar(128) NOT NULL COMMENT 'Injective 接收地址（inj1...）',
  `amount` varchar(100) DEFAULT NULL COMMENT '存款金额',
  `denom` varchar(100) DEFAULT NULL COMMENT '代币种类（peggy0x...）',
  `event_nonce` bigint unsigned DEFAULT NULL COMMENT 'WithdrawalClaim 事件的 nonce',
  `event_height` bigint unsigned DEFAULT NULL COMMENT 'WithdrawalClaim 事件的区块高度',
  `orchestrator_address` varchar(128) DEFAULT NULL COMMENT '创建批次请求的协调者地址',
  `state` varchar(50) DEFAULT 'completed' COMMENT '交易状态：pending, completed, failed',
  `claim_type` int DEFAULT 1 COMMENT '声明类型（存款为 DepositClaim=1）',

  -- 关联字段
  `tx_hash` varchar(128) NOT NULL COMMENT 'Injective 链交易哈希',
  `block_height` bigint unsigned NOT NULL COMMENT '区块高度',
  `block_timestamp` bigint unsigned NOT NULL COMMENT '区块时间戳（秒）',

  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_peggy_deposit_event_nonce` (`event_nonce`),
  KEY `idx_peggy_deposit_sender` (`sender`),
  KEY `idx_peggy_deposit_receiver` (`receiver`),
  KEY `idx_peggy_deposit_tx_hash` (`tx_hash`),
  KEY `idx_peggy_deposit_block_height` (`block_height`),
  KEY `idx_peggy_deposit_block_timestamp` (`block_timestamp`),
  KEY `idx_peggy_deposit_state` (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Peggy 存款交易表（以太坊→Injective）';

-- ============================================================================
-- Peggy 取款交易表（Injective→以太坊）
-- ============================================================================
DROP TABLE IF EXISTS `peggy_withdrawal_txs`;
CREATE TABLE `peggy_withdrawal_txs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,

  -- 核心字段
  `sender` varchar(128) NOT NULL COMMENT 'Injective 发送地址（inj1...）',
  `receiver` varchar(128) NOT NULL COMMENT '以太坊接收地址（0x...）',
  `amount` varchar(100) DEFAULT NULL COMMENT '取款金额',
  `denom` varchar(100) DEFAULT NULL COMMENT '代币种类（peggy0x...）',
  `bridge_fee` varchar(100) DEFAULT NULL COMMENT '发送方支付的跨链桥费用',
  `outgoing_tx_id` bigint unsigned DEFAULT NULL COMMENT '取款请求的自增唯一 ID',
  `batch_timeout` bigint unsigned DEFAULT NULL COMMENT '批次请求被丢弃的时间戳',
  `batch_nonce` bigint unsigned DEFAULT NULL COMMENT '取款批次的自增唯一 ID',
  `orchestrator_address` varchar(128) DEFAULT NULL COMMENT '创建批次请求的协调者地址',
  `event_nonce` bigint unsigned DEFAULT NULL COMMENT 'WithdrawalClaim 事件的 nonce',
  `event_height` bigint unsigned DEFAULT NULL COMMENT 'WithdrawalClaim 事件的区块高度',
  `state` varchar(50) DEFAULT 'pending' COMMENT '交易状态：pending, batched, completed, failed',
  `claim_type` int DEFAULT 2 COMMENT '声明类型（批次取款为 WithdrawalClaim=2）',

  -- 关联字段
  `tx_hash` varchar(128) NOT NULL COMMENT 'Injective 链交易哈希',
  `block_height` bigint unsigned NOT NULL COMMENT '区块高度',
  `block_timestamp` bigint unsigned NOT NULL COMMENT '区块时间戳（秒）',

  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_peggy_withdrawal_outgoing_tx_id` (`outgoing_tx_id`),
  KEY `idx_peggy_withdrawal_sender` (`sender`),
  KEY `idx_peggy_withdrawal_receiver` (`receiver`),
  KEY `idx_peggy_withdrawal_tx_hash` (`tx_hash`),
  KEY `idx_peggy_withdrawal_block_height` (`block_height`),
  KEY `idx_peggy_withdrawal_block_timestamp` (`block_timestamp`),
  KEY `idx_peggy_withdrawal_state` (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Peggy 取款交易表（Injective→以太坊）';

-- ============================================================================
-- 跨链交易同步状态表（用于记录同步进度）
-- ============================================================================
DROP TABLE IF EXISTS `cross_chain_sync_state`;
CREATE TABLE `cross_chain_sync_state` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,

  `sync_type` varchar(50) NOT NULL COMMENT '同步类型：ibc_transfer, peggy_deposit, peggy_withdrawal',
  `last_processed_height` bigint unsigned NOT NULL DEFAULT '0' COMMENT '最后处理的区块高度',
  `last_processed_time` bigint unsigned DEFAULT NULL COMMENT '最后处理时间戳',
  `sync_status` varchar(20) DEFAULT 'syncing' COMMENT '同步状态：syncing, paused, error',
  `error_message` text COMMENT '错误信息',

  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sync_type` (`sync_type`),
  KEY `idx_sync_status` (`sync_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='跨链交易同步状态表';

-- 初始化同步状态
INSERT INTO `cross_chain_sync_state` (`sync_type`, `last_processed_height`, `sync_status`) VALUES
('ibc_transfer', 0, 'syncing'),
('peggy_deposit', 0, 'syncing'),
('peggy_withdrawal', 0, 'syncing')
ON DUPLICATE KEY UPDATE `sync_type` = `sync_type`;

SET FOREIGN_KEY_CHECKS = 1;

-- ============================================================================
-- 常用查询示例
-- ============================================================================

-- 1. 查询某个地址的所有 IBC 发送交易
-- SELECT * FROM ibc_transfer_txs
-- WHERE sender = 'inj1...' AND deleted_at IS NULL
-- ORDER BY block_timestamp DESC LIMIT 20;

-- 2. 查询某个地址的所有 Peggy 存款
-- SELECT * FROM peggy_deposit_txs
-- WHERE receiver = 'inj1...' AND deleted_at IS NULL
-- ORDER BY block_timestamp DESC LIMIT 20;

-- 3. 查询某个地址的所有 Peggy 取款
-- SELECT * FROM peggy_withdrawal_txs
-- WHERE sender = 'inj1...' AND deleted_at IS NULL
-- ORDER BY block_timestamp DESC LIMIT 20;

-- 4. 查询特定通道的所有 IBC 转账
-- SELECT * FROM ibc_transfer_txs
-- WHERE source_channel = 'channel-0' AND deleted_at IS NULL
-- ORDER BY block_timestamp DESC LIMIT 100;

-- 5. 统计每日 Peggy 存款总额
-- SELECT DATE(FROM_UNIXTIME(block_timestamp)) as date, COUNT(*) as count, SUM(CAST(amount AS DECIMAL(65,0))) as total
-- FROM peggy_deposit_txs
-- WHERE deleted_at IS NULL
-- GROUP BY DATE(FROM_UNIXTIME(block_timestamp))
-- ORDER BY date DESC;
