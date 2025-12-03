-- ============================================
-- 交易归档表结构
-- 支持 MySQL 和 PostgreSQL
-- 
-- 归档策略：
-- - 将超过6个月的交易数据归档
-- - 主表只保留关键字段（用于查询）
-- - 详细数据（Messages, Logs, Data）存储在归档表
-- ============================================

-- ============================================
-- MySQL 版本
-- ============================================

-- 创建交易归档表（只保留关键字段）
CREATE TABLE IF NOT EXISTS transaction_archives (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    
    tx_data_id VARCHAR(255) NOT NULL COMMENT '交易数据ID',
    block_number BIGINT UNSIGNED NOT NULL COMMENT '区块号',
    block_timestamp VARCHAR(255) COMMENT '区块时间戳',
    hash VARCHAR(255) NOT NULL COMMENT '交易哈希',
    code INT UNSIGNED COMMENT '交易执行代码',
    gas_wanted BIGINT COMMENT '请求的Gas',
    gas_used BIGINT COMMENT '使用的Gas',
    gas_fee TEXT COMMENT 'Gas费用（JSON格式）',
    codespace VARCHAR(255) COMMENT '错误代码空间',
    events JSON COMMENT '事件列表',
    tx_type VARCHAR(100) COMMENT '交易类型',
    signatures JSON COMMENT '签名列表',
    memo TEXT COMMENT '备注',
    tx_number BIGINT UNSIGNED NOT NULL COMMENT '交易编号',
    block_unix_timestamp BIGINT UNSIGNED COMMENT '区块Unix时间戳',
    claim_ids JSON COMMENT 'Claim ID列表',
    signer VARCHAR(128) COMMENT '交易发起者地址（第一个签名者）',
    archived_at BIGINT COMMENT '归档时间戳',
    
    INDEX idx_tx_data_id (tx_data_id),
    INDEX idx_block_number (block_number),
    INDEX idx_hash (hash),
    INDEX idx_tx_number (tx_number),
    INDEX idx_block_tx (block_number, tx_number),
    INDEX idx_timestamp_tx (block_unix_timestamp, tx_number),
    INDEX idx_archived_at (archived_at),
    INDEX idx_signer (signer)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='交易归档表';

-- 创建交易详细数据归档表（存储大字段）
CREATE TABLE IF NOT EXISTS transaction_data_archives (
    transaction_id BIGINT PRIMARY KEY COMMENT '关联transaction_archives.id',
    data LONGTEXT COMMENT '交易数据（压缩后）',
    info TEXT COMMENT '附加信息',
    messages LONGTEXT COMMENT '交易消息（压缩后）',
    logs LONGTEXT COMMENT '执行日志（压缩后）',
    error_log TEXT COMMENT '错误日志',
    
    FOREIGN KEY (transaction_id) REFERENCES transaction_archives(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='交易详细数据归档表';

-- ============================================
-- PostgreSQL 版本
-- ============================================

-- PostgreSQL: 创建交易归档表
-- CREATE TABLE IF NOT EXISTS transaction_archives (
--     id BIGSERIAL PRIMARY KEY,
--     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
--     updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
--     deleted_at TIMESTAMP NULL,
--     
--     tx_data_id VARCHAR(255) NOT NULL,
--     block_number BIGINT NOT NULL,
--     block_timestamp VARCHAR(255),
--     hash VARCHAR(255) NOT NULL,
--     code INTEGER,
--     gas_wanted BIGINT,
--     gas_used BIGINT,
--     gas_fee TEXT,
--     codespace VARCHAR(255),
--     events JSONB,
--     tx_type VARCHAR(100),
--     signatures JSONB,
--     memo TEXT,
--     tx_number BIGINT NOT NULL,
--     block_unix_timestamp BIGINT,
--     claim_ids JSONB,
--     signer VARCHAR(128),
--     archived_at BIGINT
-- );
-- 
-- CREATE INDEX idx_tx_data_id ON transaction_archives(tx_data_id);
-- CREATE INDEX idx_block_number ON transaction_archives(block_number);
-- CREATE INDEX idx_hash ON transaction_archives(hash);
-- CREATE INDEX idx_tx_number ON transaction_archives(tx_number);
-- CREATE INDEX idx_block_tx ON transaction_archives(block_number, tx_number);
-- CREATE INDEX idx_timestamp_tx ON transaction_archives(block_unix_timestamp, tx_number);
-- CREATE INDEX idx_archived_at ON transaction_archives(archived_at);
-- CREATE INDEX idx_signer ON transaction_archives(signer);
-- 
-- -- PostgreSQL: 创建交易详细数据归档表
-- CREATE TABLE IF NOT EXISTS transaction_data_archives (
--     transaction_id BIGINT PRIMARY KEY REFERENCES transaction_archives(id) ON DELETE CASCADE,
--     data TEXT,
--     info TEXT,
--     messages TEXT,
--     logs TEXT,
--     error_log TEXT
-- );

-- ============================================
-- 归档操作说明
-- ============================================

-- 1. 归档超过6个月的交易数据
-- INSERT INTO transaction_archives (
--     tx_data_id, block_number, block_timestamp, hash, code,
--     gas_wanted, gas_used, gas_fee, codespace, events, tx_type,
--     signatures, memo, tx_number, block_unix_timestamp, claim_ids, signer, archived_at
-- )
-- SELECT 
--     tx_data_id, block_number, block_timestamp, hash, code,
--     gas_wanted, gas_used, gas_fee, codespace, events, tx_type,
--     signatures, memo, tx_number, block_unix_timestamp, claim_ids, signer,
--     UNIX_TIMESTAMP(NOW()) as archived_at
-- FROM transactions
-- WHERE created_at < DATE_SUB(NOW(), INTERVAL 6 MONTH)
-- AND (archived IS NULL OR archived = 0);

-- 2. 将详细数据移动到归档表
-- INSERT INTO transaction_data_archives (transaction_id, data, info, messages, logs, error_log)
-- SELECT 
--     ta.id, t.data, t.info, t.messages, t.logs, t.error_log
-- FROM transactions t
-- INNER JOIN transaction_archives ta ON t.hash = ta.hash
-- WHERE t.created_at < DATE_SUB(NOW(), INTERVAL 6 MONTH)
-- AND (t.archived IS NULL OR t.archived = 0);

-- 3. 更新主表，清空大字段并标记为已归档
-- UPDATE transactions t
-- INNER JOIN transaction_archives ta ON t.hash = ta.hash
-- SET 
--     t.data = NULL,
--     t.messages = NULL,
--     t.logs = NULL,
--     t.info = NULL,
--     t.error_log = NULL,
--     t.archived = 1
-- WHERE t.created_at < DATE_SUB(NOW(), INTERVAL 6 MONTH)
-- AND (t.archived IS NULL OR t.archived = 0);

