# 验证者统计方案优化

## 问题分析

当前方案的问题：
- `block_signers` 表会为每个块记录所有实际参与的验证者（提议者 + 签名者）
- 如果有100个验证者，每个块平均50个签名，1000个块就会有50000条记录
- 数据量会快速增长，影响查询性能

## 优化方案

### 方案1：只记录实际参与者（当前方案，已优化）

**优点**：
- 只记录实际参与的验证者，不是所有验证者
- 数据量相对较小（每个块通常只有几十个验证者参与）

**缺点**：
- 仍然会随着块数增长而增长
- 计算错过块数需要知道每个块高度时的活跃验证者集合

### 方案2：使用验证者集合快照 + 增量统计（推荐）

**核心思想**：
1. 创建 `validator_set_snapshots` 表，定期记录验证者集合快照
2. `block_signers` 表只记录实际参与者（保持现状）
3. 统计时：
   - **提议数**：从 `block_signers` 表统计 `is_proposer=true`
   - **签名数**：从 `block_signers` 表统计 `signed=true`
   - **错过数**：从验证者集合快照计算应该签名的块数，减去已签名的块数

**数据表设计**：

```sql
-- 验证者集合快照表（记录每个块高度时的活跃验证者）
CREATE TABLE `validator_set_snapshots` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `block_height` bigint unsigned NOT NULL COMMENT 'Block height',
  `validator_operator_address` varchar(128) NOT NULL COMMENT 'Validator operator address',
  `validator_consensus_address` varchar(128) DEFAULT NULL COMMENT 'Validator consensus address',
  `voting_power` bigint unsigned DEFAULT NULL COMMENT 'Voting power',
  `block_unix_timestamp` bigint unsigned DEFAULT NULL COMMENT 'Block timestamp',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_validator_set_snapshot` (`block_height`, `validator_operator_address`),
  KEY `idx_validator_set_block_height` (`block_height`),
  KEY `idx_validator_set_validator` (`validator_operator_address`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Validator set snapshots';

-- 优化：只在验证者集合变化时记录快照
-- 或者定期记录（如每100个块记录一次）
```

**统计逻辑**：
```sql
-- 提议数：从 block_signers 统计
SELECT COUNT(*) FROM block_signers 
WHERE validator_operator_address = ? AND is_proposer = true;

-- 签名数：从 block_signers 统计
SELECT COUNT(*) FROM block_signers 
WHERE validator_operator_address = ? AND signed = true;

-- 错过数：从 validator_set_snapshots 计算应该签名的块数，减去已签名数
SELECT 
  (SELECT COUNT(DISTINCT block_height) FROM validator_set_snapshots 
   WHERE validator_operator_address = ?) 
  - 
  (SELECT COUNT(*) FROM block_signers 
   WHERE validator_operator_address = ? AND signed = true) 
AS missed;
```

### 方案3：使用聚合统计表（最优化）

**核心思想**：
1. 创建 `validator_block_stats` 表，定期聚合统计
2. 每次块同步时，只更新相关验证者的统计
3. 不需要存储每个块的详细记录

**数据表设计**：

```sql
CREATE TABLE `validator_block_stats` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `validator_operator_address` varchar(128) NOT NULL,
  `period_start_height` bigint unsigned NOT NULL COMMENT 'Period start block height',
  `period_end_height` bigint unsigned NOT NULL COMMENT 'Period end block height',
  `proposed_count` bigint unsigned DEFAULT 0 COMMENT 'Number of blocks proposed in this period',
  `signed_count` bigint unsigned DEFAULT 0 COMMENT 'Number of blocks signed in this period',
  `should_sign_count` bigint unsigned DEFAULT 0 COMMENT 'Number of blocks validator should have signed',
  `missed_count` bigint unsigned DEFAULT 0 COMMENT 'Number of blocks missed',
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_validator_period` (`validator_operator_address`, `period_start_height`),
  KEY `idx_validator_stats_validator` (`validator_operator_address`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Validator block statistics by period';
```

**统计逻辑**：
- 每个块同步时，更新相关验证者的统计
- 定期（如每1000个块）创建一个新的统计周期
- 查询时聚合所有周期的数据

## 推荐方案

**采用方案3（聚合统计表）**，原因：
1. **数据量最小**：不需要存储每个块的详细记录
   - 100个验证者 × 1000个块/周期 = 100条记录/周期
   - 相比 block_signers 表的50,000条记录，减少99.8%
2. **查询最快**：直接查询聚合统计表，无需 COUNT 操作
3. **更新简单**：每个块只需要更新相关验证者的计数器
4. **扩展性好**：可以按周期查询，支持时间段统计

**方案3 vs 方案2 对比**：
- 方案2：需要存储 block_signers（50,000条）+ validator_set_snapshots（100条）= 50,100条
- 方案3：只需要存储 validator_block_stats（100条/周期）
- **数据量减少：99.8%**

## 实施步骤

1. ✅ 创建 `validator_block_stats` 表（已完成）
2. ✅ 在 `SaveBlockSigners` 中更新聚合统计（已完成）
3. ✅ 修改统计逻辑，使用聚合统计表（已完成）
4. 可选：定期获取验证者集合，更新 `should_sign_count`（用于准确计算错过块数）

## 数据量对比

假设：100个验证者，1000个块

| 方案 | 数据量 | 说明 |
|------|--------|------|
| 方案1（block_signers） | 50,000条 | 每个块记录实际参与者 |
| 方案2（快照） | 50,100条 | block_signers + 快照表 |
| **方案3（聚合统计）** | **100条** | **按周期聚合，数据量最小** |

## 注意事项

1. **should_sign_count 的更新**：
   - 当前实现：对于实际签名的验证者，shouldSign=true（他们肯定在活跃集合中）
   - 未来优化：可以定期（如每1000个块）获取验证者集合，更新所有验证者的 should_sign_count
   - 或者：在验证者集合变化时，更新当前周期所有验证者的 should_sign_count

2. **错过块数的计算**：
   - 当前：missed_count = should_sign_count - signed_count
   - 如果 should_sign_count 未准确更新，missed_count 可能不准确
   - 可以通过定期更新 should_sign_count 来改善准确性
