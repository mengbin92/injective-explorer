-- API Management Tables
-- Created: 2025-12-24
-- Description: Core tables for API token management, rate limiting, and usage statistics

-- API Token表
CREATE TABLE IF NOT EXISTS `api_tokens` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID（来自外部系统）',
  `name` VARCHAR(50) NOT NULL COMMENT 'Token名称',
  `token_hash` VARCHAR(64) NOT NULL UNIQUE COMMENT 'Token哈希值（SHA256）',
  `token_preview` VARCHAR(20) NOT NULL COMMENT 'Token预览（inj_xxx...xxx）',
  `plan_type` VARCHAR(20) NOT NULL DEFAULT 'free' COMMENT '套餐类型：free/paid',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/disabled',
  `last_used_at` TIMESTAMP NULL COMMENT '最后使用时间',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_token_hash` (`token_hash`),
  INDEX `idx_status` (`status`),
  INDEX `idx_plan_type` (`plan_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='API Token表';

-- 用户使用量统计表
CREATE TABLE IF NOT EXISTS `user_usage_stats` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID',
  `monthly_used` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本月已使用次数',
  `monthly_limit` BIGINT UNSIGNED NOT NULL DEFAULT 100000 COMMENT '月度限额',
  `total_usage` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '历史总使用量',
  `success_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '成功请求数',
  `failed_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '失败请求数',
  `last_reset_at` TIMESTAMP NOT NULL COMMENT '上次重置时间',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  UNIQUE KEY `uk_user_id` (`user_id`),
  INDEX `idx_last_reset_at` (`last_reset_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='用户使用量统计表';

-- API请求日志表
CREATE TABLE IF NOT EXISTS `api_request_logs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `token_id` BIGINT UNSIGNED NULL COMMENT 'Token ID（未认证请求为NULL）',
  `user_id` VARCHAR(64) NULL COMMENT '用户ID（未认证请求为NULL）',
  `ip_address` VARCHAR(45) NOT NULL COMMENT 'IP地址（支持IPv6）',
  `ip_location` VARCHAR(100) NULL COMMENT 'IP归属地',
  `endpoint` VARCHAR(255) NOT NULL COMMENT '请求接口路径',
  `method` VARCHAR(10) NOT NULL COMMENT 'HTTP方法',
  `status_code` INT NOT NULL COMMENT 'HTTP状态码',
  `response_time` INT NOT NULL COMMENT '响应时间（毫秒）',
  `error_message` TEXT NULL COMMENT '错误信息',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  
  INDEX `idx_token_id` (`token_id`),
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_ip_address` (`ip_address`),
  INDEX `idx_endpoint` (`endpoint`),
  INDEX `idx_status_code` (`status_code`),
  INDEX `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='API请求日志表';

-- 使用量日统计表
CREATE TABLE IF NOT EXISTS `usage_daily_stats` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID',
  `stat_date` DATE NOT NULL COMMENT '统计日期',
  `total_calls` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '总请求数',
  `success_calls` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '成功请求数',
  `failed_calls` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '失败请求数',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  UNIQUE KEY `uk_user_date` (`user_id`, `stat_date`),
  INDEX `idx_stat_date` (`stat_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='使用量日统计表';

-- 套餐配置表
CREATE TABLE IF NOT EXISTS `api_plans` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(50) NOT NULL UNIQUE COMMENT '套餐名称',
  `plan_type` VARCHAR(20) NOT NULL COMMENT '套餐类型：free/paid',
  `monthly_limit` BIGINT UNSIGNED NOT NULL COMMENT '月度调用限额',
  `rate_limit_per_second` INT NOT NULL COMMENT '每秒请求限制',
  `max_tokens` INT NOT NULL COMMENT '最大Token数',
  `enable_webhook` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否支持Webhook',
  `price` DECIMAL(10,2) NOT NULL DEFAULT 0.00 COMMENT '价格',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/inactive',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  INDEX `idx_plan_type` (`plan_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='套餐配置表';

-- 初始化Free套餐数据
INSERT INTO `api_plans` (`name`, `plan_type`, `monthly_limit`, `rate_limit_per_second`, `max_tokens`, `enable_webhook`, `price`, `status`) 
VALUES ('Free', 'free', 100000, 5, 3, 0, 0.00, 'active')
ON DUPLICATE KEY UPDATE `name`=`name`;

-- Webhook配置表（预留，暂不实现功能）
CREATE TABLE IF NOT EXISTS `webhooks` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `token_id` BIGINT UNSIGNED NOT NULL COMMENT 'Token ID（必须是付费套餐）',
  `webhook_url` VARCHAR(512) NOT NULL COMMENT 'Webhook URL',
  `secret` VARCHAR(64) NOT NULL COMMENT 'Webhook签名密钥',
  `events` JSON NOT NULL COMMENT '订阅的事件类型列表',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/inactive',
  `last_triggered_at` TIMESTAMP NULL COMMENT '最后触发时间',
  `failure_count` INT NOT NULL DEFAULT 0 COMMENT '连续失败次数',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  
  INDEX `idx_token_id` (`token_id`),
  INDEX `idx_status` (`status`),
  INDEX `idx_last_triggered_at` (`last_triggered_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='Webhook配置表（预留）';

-- IP黑名单表
CREATE TABLE IF NOT EXISTS `ip_blacklist` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `ip_address` VARCHAR(45) NOT NULL UNIQUE COMMENT 'IP地址（支持IPv4和IPv6）',
  `ip_location` VARCHAR(100) NULL COMMENT 'IP归属地',
  `reason` VARCHAR(50) NOT NULL COMMENT '加入原因：batch_register/api_abuse/proxy/spam/other',
  `reason_text` VARCHAR(255) NULL COMMENT '原因描述',
  `block_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '拦截次数',
  `operator_id` VARCHAR(64) NULL COMMENT '操作人ID',
  `operator_name` VARCHAR(100) NULL COMMENT '操作人名称',
  `block_type` VARCHAR(20) NOT NULL DEFAULT 'all' COMMENT '限制类型：all/register/api',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL COMMENT '软删除时间',
  
  INDEX `idx_ip_address` (`ip_address`),
  INDEX `idx_reason` (`reason`),
  INDEX `idx_created_at` (`created_at`),
  INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='IP黑名单表';

-- IP注册记录表（用于统计和管理）
CREATE TABLE IF NOT EXISTS `ip_registration_records` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `ip_address` VARCHAR(45) NOT NULL COMMENT 'IP地址',
  `ip_location` VARCHAR(100) NULL COMMENT 'IP归属地',
  `user_id` VARCHAR(64) NOT NULL COMMENT '注册的用户ID',
  `email` VARCHAR(255) NULL COMMENT '注册邮箱',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL COMMENT '软删除时间',
  
  INDEX `idx_ip_address` (`ip_address`),
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_created_at` (`created_at`),
  INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='IP注册记录表';

-- 邮箱黑名单表
CREATE TABLE IF NOT EXISTS `email_blacklist` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `domain` VARCHAR(255) NOT NULL UNIQUE COMMENT '邮箱域名（小写）',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/disabled',
  `block_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '拦截次数',
  `note` TEXT NULL COMMENT '备注',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  INDEX `idx_domain` (`domain`),
  INDEX `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='邮箱黑名单表';

-- 初始化一些常见的一次性邮箱域名
INSERT INTO `email_blacklist` (`domain`, `status`, `note`) VALUES
('tempmail.com', 'active', '知名临时邮箱服务'),
('guerrillamail.com', 'active', '游击邮件服务'),
('10minutemail.com', 'active', '10分钟临时邮箱'),
('mailinator.com', 'active', '临时邮箱服务'),
('throwaway.email', 'active', '一次性邮箱')
ON DUPLICATE KEY UPDATE `domain`=`domain`;

