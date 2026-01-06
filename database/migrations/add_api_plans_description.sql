-- Add description field to api_plans table
-- Created: 2025-01-XX
-- Description: Add description field to store plan description

ALTER TABLE `api_plans` 
ADD COLUMN `description` TEXT NULL COMMENT 'Plan description' AFTER `plan_type`;

