-- Migration: Allow NULL values for token_id and user_id in api_request_logs
-- This allows logging of unauthenticated requests (e.g., 401 errors)
-- Date: 2024

-- Modify token_id column to allow NULL
ALTER TABLE `api_request_logs` 
  MODIFY COLUMN `token_id` BIGINT UNSIGNED NULL COMMENT 'Token ID（未认证请求为NULL）';

-- Modify user_id column to allow NULL
ALTER TABLE `api_request_logs` 
  MODIFY COLUMN `user_id` VARCHAR(64) NULL COMMENT '用户ID（未认证请求为NULL）';

