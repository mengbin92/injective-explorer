-- ================================================================
-- API账户管理模块测试数据
-- 创建时间: 2026-02-02
-- 描述: 为API账户管理相关功能生成测试数据，包括API Token、用户使用量统计
-- ================================================================

-- ================================================================
-- 1. API Token测试数据 (api_tokens)
-- ================================================================

-- 用户1: 活跃账户，Free套餐，高使用量（>80%）
-- Token 1: 活跃，最近使用
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_001', 'Production API', 'a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456', 'byh_prod_a1...x7y8z9', 'free', 'active', DATE_SUB(NOW(), INTERVAL 2 HOUR), DATE_SUB(NOW(), INTERVAL 30 DAY));

-- Token 2: 活跃，最近使用
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_001', 'Development', 'd4e5f6a1b2c3789012345678901234567890abcdef1234567890abcdef123456', 'byh_dev_d4e...u1v2w3', 'free', 'active', DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 20 DAY));

-- 用户2: 活跃账户，Free套餐，低使用量（<50%）
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_002', 'Main Token', 'm1n2o3p4q5r6789012345678901234567890abcdef1234567890abcdef123456', 'byh_main_m1...p4q5r6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 5 DAY), DATE_SUB(NOW(), INTERVAL 60 DAY));

-- 用户3: 活跃账户，Free套餐，高使用量（>80%）
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_003', 'Production API', 'p1q2r3s4t5u6789012345678901234567890abcdef1234567890abcdef123456', 'byh_prod_p1...s4t5u6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 3 HOUR), DATE_SUB(NOW(), INTERVAL 45 DAY)),
('user_003', 'Testing', 't1u2v3w4x5y6789012345678901234567890abcdef1234567890abcdef123456', 'byh_test_t1...w4x5y6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 30 DAY)),
('user_003', 'Backup', 'b1c2d3e4f5g6789012345678901234567890abcdef1234567890abcdef123456', 'byh_bak_b1c...e4f5g6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 2 DAY), DATE_SUB(NOW(), INTERVAL 15 DAY));

-- 用户4: 已禁用账户，所有token都是disabled
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_004', 'Main Token', 'm4n5o6p7q8r9789012345678901234567890abcdef1234567890abcdef123456', 'byh_main_m4...p7q8r9', 'free', 'disabled', DATE_SUB(NOW(), INTERVAL 10 DAY), DATE_SUB(NOW(), INTERVAL 90 DAY)),
('user_004', 'Secondary', 's1t2u3v4w5x6789012345678901234567890abcdef1234567890abcdef123456', 'byh_sec_s1t...v4w5x6', 'free', 'disabled', DATE_SUB(NOW(), INTERVAL 15 DAY), DATE_SUB(NOW(), INTERVAL 80 DAY));

-- 用户5: 待激活账户（没有token，但会在user_usage_stats中有记录）
-- 不插入token数据

-- 用户6: 活跃账户，Paid套餐，中使用量（50-80%）
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_006', 'Production', 'p6q7r8s9t0u1789012345678901234567890abcdef1234567890abcdef123456', 'byh_prod_p6...s9t0u1', 'paid', 'active', DATE_SUB(NOW(), INTERVAL 4 HOUR), DATE_SUB(NOW(), INTERVAL 25 DAY));

-- 用户7: 活跃账户，Free套餐，中使用量（50-80%）
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_007', 'Main API', 'm7n8o9p0q1r2789012345678901234567890abcdef1234567890abcdef123456', 'byh_main_m7...p0q1r2', 'free', 'active', DATE_SUB(NOW(), INTERVAL 6 HOUR), DATE_SUB(NOW(), INTERVAL 40 DAY)),
('user_007', 'Dev API', 'd7e8f9g0h1i2789012345678901234567890abcdef1234567890abcdef123456', 'byh_dev_d7e...g0h1i2', 'free', 'active', DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 20 DAY));

-- 用户8: 活跃账户，Free套餐，低使用量，最近30天活跃
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_008', 'API Token', 'a8b9c0d1e2f3789012345678901234567890abcdef1234567890abcdef123456', 'byh_api_a8b...d1e2f3', 'free', 'active', DATE_SUB(NOW(), INTERVAL 5 DAY), DATE_SUB(NOW(), INTERVAL 50 DAY));

-- 用户9: 活跃账户，Free套餐，高使用量，最近30天活跃
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_009', 'Production', 'p9q0r1s2t3u4789012345678901234567890abcdef1234567890abcdef123456', 'byh_prod_p9...s2t3u4', 'free', 'active', DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_SUB(NOW(), INTERVAL 35 DAY)),
('user_009', 'Testing', 't9u0v1w2x3y4789012345678901234567890abcdef1234567890abcdef123456', 'byh_test_t9...w2x3y4', 'free', 'active', DATE_SUB(NOW(), INTERVAL 2 HOUR), DATE_SUB(NOW(), INTERVAL 20 DAY));

-- 用户10: 活跃账户，Paid套餐，低使用量
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
('user_010', 'Enterprise API', 'e0f1g2h3i4j5789012345678901234567890abcdef1234567890abcdef123456', 'byh_ent_e0f...h3i4j5', 'paid', 'active', DATE_SUB(NOW(), INTERVAL 3 DAY), DATE_SUB(NOW(), INTERVAL 70 DAY));

-- 用户11-20: 批量创建更多测试账户
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
-- 用户11: 活跃，最近使用
('user_011', 'Token 1', 't1o2k3e4n5t6789012345678901234567890abcdef1234567890abcdef123456', 'byh_tok_t1o...e4n5t6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_SUB(NOW(), INTERVAL 10 DAY)),
-- 用户12: 活跃，最近使用
('user_012', 'Main Token', 'm1a2i3n4t5o6789012345678901234567890abcdef1234567890abcdef123456', 'byh_main_m1...n4t5o6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 2 HOUR), DATE_SUB(NOW(), INTERVAL 12 DAY)),
-- 用户13: 活跃，最近使用
('user_013', 'API Key', 'a1p2i3k4e5y6789012345678901234567890abcdef1234567890abcdef123456', 'byh_api_a1p...k4e5y6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 3 HOUR), DATE_SUB(NOW(), INTERVAL 15 DAY)),
-- 用户14: 活跃，最近使用
('user_014', 'Production', 'p1r2o3d4u5c6789012345678901234567890abcdef1234567890abcdef123456', 'byh_prod_p1...d4u5c6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 4 HOUR), DATE_SUB(NOW(), INTERVAL 18 DAY)),
-- 用户15: 活跃，最近使用
('user_015', 'Development', 'd1e2v3e4l5o6789012345678901234567890abcdef1234567890abcdef123456', 'byh_dev_d1e...e4l5o6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 5 HOUR), DATE_SUB(NOW(), INTERVAL 20 DAY)),
-- 用户16: 活跃，最近使用
('user_016', 'Testing', 't1e2s3t4i5n6789012345678901234567890abcdef1234567890abcdef123456', 'byh_test_t1...t4i5n6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 6 HOUR), DATE_SUB(NOW(), INTERVAL 22 DAY)),
-- 用户17: 活跃，最近使用
('user_017', 'Staging', 's1t2a3g4i5n6789012345678901234567890abcdef1234567890abcdef123456', 'byh_stag_s1...g4i5n6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 7 HOUR), DATE_SUB(NOW(), INTERVAL 25 DAY)),
-- 用户18: 活跃，最近使用
('user_018', 'Backup', 'b1a2c3k4u5p6789012345678901234567890abcdef1234567890abcdef123456', 'byh_bak_b1a...k4u5p6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 8 HOUR), DATE_SUB(NOW(), INTERVAL 28 DAY)),
-- 用户19: 活跃，最近使用
('user_019', 'Primary', 'p1r2i3m4a5r6789012345678901234567890abcdef1234567890abcdef123456', 'byh_prim_p1...m4a5r6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 9 HOUR), DATE_SUB(NOW(), INTERVAL 30 DAY)),
-- 用户20: 活跃，最近使用
('user_020', 'Secondary', 's1e2c3o4n5d6789012345678901234567890abcdef1234567890abcdef123456', 'byh_sec_s1e...o4n5d6', 'free', 'active', DATE_SUB(NOW(), INTERVAL 10 HOUR), DATE_SUB(NOW(), INTERVAL 32 DAY));

-- 用户21-30: 更多账户，有些超过30天未使用（不算本月活跃）
INSERT INTO `api_tokens` (`user_id`, `name`, `token_hash`, `token_preview`, `plan_type`, `status`, `last_used_at`, `created_at`) VALUES
-- 用户21: 活跃但超过30天未使用
('user_021', 'Token 1', 't2o3k4e5n6t7789012345678901234567890abcdef1234567890abcdef123456', 'byh_tok_t2o...e5n6t7', 'free', 'active', DATE_SUB(NOW(), INTERVAL 35 DAY), DATE_SUB(NOW(), INTERVAL 100 DAY)),
-- 用户22: 活跃但超过30天未使用
('user_022', 'Main Token', 'm2a3i4n5t6o7789012345678901234567890abcdef1234567890abcdef123456', 'byh_main_m2...n5t6o7', 'free', 'active', DATE_SUB(NOW(), INTERVAL 40 DAY), DATE_SUB(NOW(), INTERVAL 110 DAY)),
-- 用户23: 活跃但超过30天未使用
('user_023', 'API Key', 'a2p3i4k5e6y7789012345678901234567890abcdef1234567890abcdef123456', 'byh_api_a2p...k5e6y7', 'free', 'active', DATE_SUB(NOW(), INTERVAL 45 DAY), DATE_SUB(NOW(), INTERVAL 120 DAY)),
-- 用户24: 禁用账户
('user_024', 'Production', 'p2r3o4d5u6c7789012345678901234567890abcdef1234567890abcdef123456', 'byh_prod_p2...d5u6c7', 'free', 'disabled', DATE_SUB(NOW(), INTERVAL 20 DAY), DATE_SUB(NOW(), INTERVAL 90 DAY)),
-- 用户25: 禁用账户
('user_025', 'Development', 'd2e3v4e5l6o7789012345678901234567890abcdef1234567890abcdef123456', 'byh_dev_d2e...e5l6o7', 'free', 'disabled', DATE_SUB(NOW(), INTERVAL 25 DAY), DATE_SUB(NOW(), INTERVAL 95 DAY)),
-- 用户26: 活跃，最近使用
('user_026', 'Testing', 't2e3s4t5i6n7789012345678901234567890abcdef1234567890abcdef123456', 'byh_test_t2...t5i6n7', 'free', 'active', DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 35 DAY)),
-- 用户27: 活跃，最近使用
('user_027', 'Staging', 's2t3a4g5i6n7789012345678901234567890abcdef1234567890abcdef123456', 'byh_stag_s2...g5i6n7', 'free', 'active', DATE_SUB(NOW(), INTERVAL 2 DAY), DATE_SUB(NOW(), INTERVAL 38 DAY)),
-- 用户28: 活跃，最近使用
('user_028', 'Backup', 'b2a3c4k5u6p7789012345678901234567890abcdef1234567890abcdef123456', 'byh_bak_b2a...k5u6p7', 'free', 'active', DATE_SUB(NOW(), INTERVAL 3 DAY), DATE_SUB(NOW(), INTERVAL 40 DAY)),
-- 用户29: 活跃，最近使用
('user_029', 'Primary', 'p2r3i4m5a6r7789012345678901234567890abcdef1234567890abcdef123456', 'byh_prim_p2...m5a6r7', 'free', 'active', DATE_SUB(NOW(), INTERVAL 4 DAY), DATE_SUB(NOW(), INTERVAL 42 DAY)),
-- 用户30: 活跃，最近使用
('user_030', 'Secondary', 's2e3c4o5n6d7789012345678901234567890abcdef1234567890abcdef123456', 'byh_sec_s2e...o5n6d7', 'free', 'active', DATE_SUB(NOW(), INTERVAL 5 DAY), DATE_SUB(NOW(), INTERVAL 45 DAY));

-- ================================================================
-- 2. 用户使用量统计测试数据 (user_usage_stats)
-- ================================================================

-- 用户1: 高使用量（68450/100000 = 68.45%，实际应该更高，设为85000/100000 = 85%）
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_001', 85000, 100000, 1245680, 1200000, 45680, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 30 DAY));

-- 用户2: 低使用量（23100/100000 = 23.1%）
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_002', 23100, 100000, 450000, 420000, 30000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 60 DAY));

-- 用户3: 高使用量（95200/100000 = 95.2%）
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_003', 95200, 100000, 2100000, 2000000, 100000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 45 DAY));

-- 用户4: 已禁用账户，中等使用量（45200/100000 = 45.2%）
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_004', 45200, 100000, 890000, 850000, 40000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 90 DAY));

-- 用户5: 待激活账户，无使用量（0/100000 = 0%）
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_005', 0, 100000, 0, 0, 0, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 5 DAY));

-- 用户6: Paid套餐，中使用量（65000/200000 = 32.5%，但相对于100000是65%）
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_006', 65000, 100000, 1500000, 1450000, 50000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 25 DAY));

-- 用户7: 中使用量（68000/100000 = 68%）
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_007', 68000, 100000, 980000, 950000, 30000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 40 DAY));

-- 用户8: 低使用量（15000/100000 = 15%），最近30天活跃
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_008', 15000, 100000, 320000, 300000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 50 DAY));

-- 用户9: 高使用量（88000/100000 = 88%），最近30天活跃
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_009', 88000, 100000, 1800000, 1750000, 50000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 35 DAY));

-- 用户10: Paid套餐，低使用量（12000/200000 = 6%，但相对于100000是12%）
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
('user_010', 12000, 100000, 250000, 240000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 70 DAY));

-- 用户11-20: 批量创建使用量统计
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
-- 用户11: 中使用量
('user_011', 55000, 100000, 450000, 430000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 10 DAY)),
-- 用户12: 低使用量
('user_012', 18000, 100000, 180000, 170000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 12 DAY)),
-- 用户13: 高使用量
('user_013', 92000, 100000, 920000, 900000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 15 DAY)),
-- 用户14: 中使用量
('user_014', 72000, 100000, 720000, 700000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 18 DAY)),
-- 用户15: 低使用量
('user_015', 25000, 100000, 250000, 240000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 20 DAY)),
-- 用户16: 高使用量
('user_016', 96000, 100000, 960000, 950000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 22 DAY)),
-- 用户17: 中使用量
('user_017', 58000, 100000, 580000, 560000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 25 DAY)),
-- 用户18: 低使用量
('user_018', 22000, 100000, 220000, 210000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 28 DAY)),
-- 用户19: 高使用量
('user_019', 89000, 100000, 890000, 880000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 30 DAY)),
-- 用户20: 中使用量
('user_020', 62000, 100000, 620000, 600000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 32 DAY));

-- 用户21-30: 更多使用量统计
INSERT INTO `user_usage_stats` (`user_id`, `monthly_used`, `monthly_limit`, `total_usage`, `success_count`, `failed_count`, `last_reset_at`, `created_at`) VALUES
-- 用户21: 低使用量，超过30天未使用
('user_021', 30000, 100000, 500000, 480000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 100 DAY)),
-- 用户22: 中使用量，超过30天未使用
('user_022', 70000, 100000, 800000, 780000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 110 DAY)),
-- 用户23: 高使用量，超过30天未使用
('user_023', 85000, 100000, 1200000, 1180000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 120 DAY)),
-- 用户24: 禁用账户，中使用量
('user_024', 50000, 100000, 600000, 580000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 90 DAY)),
-- 用户25: 禁用账户，低使用量
('user_025', 20000, 100000, 300000, 290000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 95 DAY)),
-- 用户26: 活跃，低使用量
('user_026', 28000, 100000, 280000, 270000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 35 DAY)),
-- 用户27: 活跃，中使用量
('user_027', 66000, 100000, 660000, 640000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 38 DAY)),
-- 用户28: 活跃，高使用量
('user_028', 91000, 100000, 910000, 900000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 40 DAY)),
-- 用户29: 活跃，中使用量
('user_029', 59000, 100000, 590000, 570000, 20000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 42 DAY)),
-- 用户30: 活跃，低使用量
('user_030', 32000, 100000, 320000, 310000, 10000, DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 45 DAY));

-- ================================================================
-- 3. 验证数据插入结果
-- ================================================================

-- 查看账户统计
SELECT
    '总账户数' AS metric,
    COUNT(DISTINCT user_id) AS count
FROM api_tokens
WHERE deleted_at IS NULL
UNION ALL
SELECT
    '活跃账户数',
    COUNT(DISTINCT user_id)
FROM api_tokens
WHERE status = 'active' AND deleted_at IS NULL
UNION ALL
SELECT
    'Token总数',
    COUNT(*)
FROM api_tokens
WHERE deleted_at IS NULL
UNION ALL
SELECT
    '本月活跃账户数（最近30天有使用）',
    COUNT(DISTINCT user_id)
FROM api_tokens
WHERE last_used_at >= DATE_SUB(NOW(), INTERVAL 30 DAY) AND deleted_at IS NULL;

-- 查看账户状态分布
SELECT
    CASE
        WHEN COUNT(*) = 0 THEN 'pending'
        WHEN SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) > 0 THEN 'active'
        ELSE 'disabled'
    END AS account_status,
    COUNT(DISTINCT user_id) AS account_count
FROM api_tokens
WHERE deleted_at IS NULL
GROUP BY user_id
ORDER BY account_status;

-- 查看使用量分布
SELECT
    CASE
        WHEN monthly_limit = 0 THEN 'N/A'
        WHEN (monthly_used * 100.0 / monthly_limit) > 80 THEN 'high (>80%)'
        WHEN (monthly_used * 100.0 / monthly_limit) >= 50 THEN 'medium (50-80%)'
        ELSE 'low (<50%)'
    END AS usage_level,
    COUNT(*) AS account_count
FROM user_usage_stats
GROUP BY usage_level
ORDER BY usage_level;

-- 查看套餐类型分布
SELECT
    plan_type,
    COUNT(DISTINCT user_id) AS user_count,
    COUNT(*) AS token_count
FROM api_tokens
WHERE deleted_at IS NULL
GROUP BY plan_type;

-- ================================================================
-- 完成
-- ================================================================
-- 测试数据插入完成！
--
-- 数据概览:
-- - API Token: ~50个token，覆盖30个用户
-- - 用户使用量统计: 30条记录
--
-- 测试场景覆盖:
-- 1. 活跃账户（有active token）
-- 2. 禁用账户（所有token都是disabled）
-- 3. 待激活账户（user_005，有usage_stats但没有token）
-- 4. 高使用量账户（>80%）
-- 5. 中使用量账户（50-80%）
-- 6. 低使用量账户（<50%）
-- 7. Free套餐和Paid套餐
-- 8. 本月活跃账户（最近30天有使用）
-- 9. 超过30天未使用的账户
--
-- 接下来可以测试以下接口:
-- 1. GET /api/v1/admin/accounts - 账户列表（支持搜索、状态过滤、使用量过滤）
-- 2. GET /api/v1/admin/accounts/detail?user_id=user_001 - 账户详情
-- 3. GET /api/v1/admin/accounts/stats - 账户统计
-- 4. PUT /api/v1/admin/accounts/status - 禁用/启用账户
-- 5. PUT /api/v1/admin/tokens/status - 禁用/启用Token
-- 6. GET /api/v1/admin/accounts/export - 导出账户列表
-- ================================================================
