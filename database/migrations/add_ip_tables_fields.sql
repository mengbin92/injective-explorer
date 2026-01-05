-- Migration: Add missing fields to IP tables
-- Created: 2025-01-05
-- Description: Add deleted_at and updated_at fields to IP-related tables to match GORM model
-- Note: Run this migration only if the tables already exist without these fields

-- Add deleted_at field to ip_blacklist table
-- If the column already exists, this will fail - that's OK, just skip it
ALTER TABLE `ip_blacklist` 
ADD COLUMN `deleted_at` TIMESTAMP NULL COMMENT '软删除时间' AFTER `updated_at`;

-- Add index for deleted_at (will fail if index exists, that's OK)
ALTER TABLE `ip_blacklist` 
ADD INDEX `idx_deleted_at` (`deleted_at`);

-- Add updated_at field to ip_registration_records table
-- If the column already exists, this will fail - that's OK, just skip it
ALTER TABLE `ip_registration_records` 
ADD COLUMN `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP AFTER `created_at`;

-- Add deleted_at field to ip_registration_records table
-- If the column already exists, this will fail - that's OK, just skip it
ALTER TABLE `ip_registration_records` 
ADD COLUMN `deleted_at` TIMESTAMP NULL COMMENT '软删除时间' AFTER `updated_at`;

-- Add index for deleted_at (will fail if index exists, that's OK)
ALTER TABLE `ip_registration_records` 
ADD INDEX `idx_deleted_at` (`deleted_at`);

