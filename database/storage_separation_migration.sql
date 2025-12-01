-- ============================================
-- 存储分离迁移脚本
-- 添加对象存储键字段到transactions表
-- ============================================

-- MySQL 版本
ALTER TABLE transactions 
ADD COLUMN data_storage_key VARCHAR(255) NULL AFTER data,
ADD COLUMN messages_storage_key VARCHAR(255) NULL AFTER messages,
ADD COLUMN logs_storage_key VARCHAR(255) NULL AFTER logs;

-- 创建索引
CREATE INDEX idx_data_storage_key ON transactions(data_storage_key);
CREATE INDEX idx_messages_storage_key ON transactions(messages_storage_key);
CREATE INDEX idx_logs_storage_key ON transactions(logs_storage_key);

-- ============================================
-- PostgreSQL 版本
-- ============================================

-- ALTER TABLE transactions 
-- ADD COLUMN data_storage_key VARCHAR(255) NULL,
-- ADD COLUMN messages_storage_key VARCHAR(255) NULL,
-- ADD COLUMN logs_storage_key VARCHAR(255) NULL;
-- 
-- CREATE INDEX idx_data_storage_key ON transactions(data_storage_key);
-- CREATE INDEX idx_messages_storage_key ON transactions(messages_storage_key);
-- CREATE INDEX idx_logs_storage_key ON transactions(logs_storage_key);

-- ============================================
-- 说明
-- ============================================
-- 1. 这些字段用于存储对象存储中的key，而不是实际数据
-- 2. 字段可以为NULL，以支持向后兼容（旧数据仍在数据库中）
-- 3. 创建索引以支持快速查询
-- 4. 迁移后，新数据会使用对象存储，旧数据仍可从数据库读取

