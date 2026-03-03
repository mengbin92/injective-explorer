-- ============================================
-- 创建合约验证记录表
-- 用于存储合约验证的详细记录和历史
-- ============================================

CREATE TABLE IF NOT EXISTS `contract_verifications` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `contract_address` varchar(128) NOT NULL COMMENT '合约地址',
  `chain_id` varchar(50) NOT NULL DEFAULT 'injective-1' COMMENT '链ID',
  `contract_type` varchar(50) NOT NULL COMMENT '合约类型: solidity/cosmwasm',
  `verification_type` varchar(50) NOT NULL COMMENT '验证类型: full_match/partial_match',
  `compiler_version` varchar(100) DEFAULT NULL COMMENT '编译器版本(solc版本或rustc版本)',
  `optimization_enabled` tinyint(1) DEFAULT '0' COMMENT '是否启用优化',
  `optimization_runs` int DEFAULT '0' COMMENT '优化运行次数',
  `evm_version` varchar(50) DEFAULT NULL COMMENT 'EVM版本(solidity)',
  `cargo_version` varchar(100) DEFAULT NULL COMMENT 'Cargo版本(cosmwasm)',
  `code_id` bigint DEFAULT NULL COMMENT '代码ID(cosmwasm)',
  `license` varchar(100) DEFAULT NULL COMMENT '许可证类型',
  `source_code` mediumtext COMMENT '源代码(JSON格式,包含所有源文件)',
  `source_code_storage_key` varchar(512) DEFAULT NULL COMMENT '源代码在对象存储中的key',
  `abi` json DEFAULT NULL COMMENT 'ABI(JSON格式)',
  `abi_storage_key` varchar(512) DEFAULT NULL COMMENT 'ABI在对象存储中的key',
  `constructor_args` text COMMENT '构造函数参数(hex)',
  `verification_status` varchar(50) NOT NULL DEFAULT 'pending' COMMENT '验证状态: pending/verifying/verified/failed',
  `error_message` text COMMENT '错误信息(验证失败时)',
  `verified_at` datetime(3) DEFAULT NULL COMMENT '验证通过时间',
  `verified_by` varchar(128) DEFAULT NULL COMMENT '验证提交者',
  PRIMARY KEY (`id`),
  KEY `idx_contract_verifications_deleted_at` (`deleted_at`),
  KEY `idx_contract_verifications_address` (`contract_address`),
  KEY `idx_contract_verifications_chain_id` (`chain_id`),
  KEY `idx_contract_verifications_status` (`verification_status`),
  KEY `idx_contract_verifications_address_chain` (`contract_address`, `chain_id`),
  KEY `idx_contract_verifications_type` (`contract_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='合约验证记录表';
