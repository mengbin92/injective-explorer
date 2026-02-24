-- migrations/add_erc20_metadata_sync_fields.sql
-- 新增 ERC20 元数据同步相关字段
-- 适用于 MySQL 5.7+/8.0+
-- 执行顺序：先加字段，再加索引

-- 1. 新增基础字段
ALTER TABLE contract_infos ADD COLUMN metadata_not_applicable TINYINT(1) DEFAULT 0 COMMENT '是否非 ERC20 合约（避免重复尝试）' AFTER decimals;
ALTER TABLE contract_infos ADD COLUMN version INT DEFAULT 0 COMMENT '乐观锁版本' AFTER metadata_not_applicable;
ALTER TABLE contract_infos ADD COLUMN metadata_updated_at DATETIME(3) DEFAULT NULL COMMENT '元数据最后更新时间' AFTER version;

-- 2. 新增 Peggy tokens 相关字段
ALTER TABLE contract_infos ADD COLUMN is_peggy_token TINYINT(1) DEFAULT 0 COMMENT '是否为 Peggy bridge token' AFTER metadata_updated_at;
ALTER TABLE contract_infos ADD COLUMN peggy_denom VARCHAR(255) DEFAULT NULL COMMENT 'Peggy bank_denom，如 peggy0xa8c8...' AFTER is_peggy_token;
ALTER TABLE contract_infos ADD COLUMN peggy_original_address VARCHAR(42) DEFAULT NULL COMMENT 'Peggy 原始链 ERC20 地址' AFTER peggy_denom;
ALTER TABLE contract_infos ADD COLUMN metadata_source VARCHAR(50) DEFAULT NULL COMMENT '元数据来源：evm/ethereum/coingecko/manual' AFTER peggy_original_address;

-- 3. 新增索引（MySQL 5.7 需忽略重复错误）
-- CREATE INDEX IF NOT EXISTS 为 MySQL 8.0.13+ 支持，5.7 需手动检查或忽略错误
CREATE INDEX idx_contract_infos_metadata_not_applicable ON contract_infos(metadata_not_applicable);
CREATE INDEX idx_contract_infos_metadata_missing ON contract_infos(contract_type, chain_id, metadata_not_applicable);
CREATE INDEX idx_contract_infos_metadata_updated_at ON contract_infos(metadata_updated_at);
CREATE INDEX idx_contract_infos_is_peggy ON contract_infos(is_peggy_token);
CREATE INDEX idx_contract_infos_peggy_denom ON contract_infos(peggy_denom);
