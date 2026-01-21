-- ============================================
-- 创建合约源文件表
-- 用于存储合约的源代码文件(.sol)和ABI文件(.json)
-- ============================================

-- 创建contract_source_files表
CREATE TABLE IF NOT EXISTS `contract_source_files` (
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
  KEY `idx_contract_address` (`contract_address`),
  KEY `idx_chain_id` (`chain_id`),
  KEY `idx_file_type` (`file_type`),
  KEY `idx_file_hash` (`file_hash`),
  KEY `idx_contract_address_chain_id` (`contract_address`, `chain_id`),
  KEY `idx_contract_file_type` (`contract_address`, `chain_id`, `file_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='合约源文件表';