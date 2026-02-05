-- Migration: Add 24h high/low price fields to token_market_cache
-- Date: 2026-02-05
-- Purpose: Support displaying 24h high/low prices in token market overview

-- Add high_24h and low_24h columns
ALTER TABLE token_market_cache
ADD COLUMN IF NOT EXISTS high_24h DECIMAL(30,18) DEFAULT 0 COMMENT '24小时最高价(USD)',
ADD COLUMN IF NOT EXISTS low_24h DECIMAL(30,18) DEFAULT 0 COMMENT '24小时最低价(USD)';

-- Create index for better query performance
CREATE INDEX IF NOT EXISTS idx_token_market_cache_high_24h ON token_market_cache(high_24h);
CREATE INDEX IF NOT EXISTS idx_token_market_cache_low_24h ON token_market_cache(low_24h);

-- Comments for documentation
COMMENT ON COLUMN token_market_cache.high_24h IS '24小时最高价格(USD),来自CoinGecko API或手动测试设置';
COMMENT ON COLUMN token_market_cache.low_24h IS '24小时最低价格(USD),来自CoinGecko API或手动测试设置';
