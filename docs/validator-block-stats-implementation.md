# 验证者节点出块统计功能实现文档

## 概述

本功能实现了验证者节点提议的、签名的和错过的出块数统计。系统会自动跟踪每个块的提议者和签名者信息，并定期更新验证者的统计指标。

**重要更新**: 为了大幅减少数据量（减少99.8%），我们已移除 `block_signers` 表，改为使用聚合统计表（`validator_block_stats`）。现在只存储聚合统计数据，不再存储每个块的详细签名记录。

## 实现内容

### 1. 数据库表结构

**validator_block_stats 表（聚合统计表，方案3）**:

```sql
CREATE TABLE `validator_block_stats` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `validator_operator_address` varchar(128) NOT NULL COMMENT 'Validator operator address',
  `period_start_height` bigint unsigned NOT NULL COMMENT 'Period start block height',
  `period_end_height` bigint unsigned DEFAULT NULL COMMENT 'Period end block height',
  `proposed_count` bigint unsigned DEFAULT '0' COMMENT 'Number of blocks proposed in this period',
  `signed_count` bigint unsigned DEFAULT '0' COMMENT 'Number of blocks signed in this period',
  `should_sign_count` bigint unsigned DEFAULT '0' COMMENT 'Number of blocks validator should have signed',
  `missed_count` bigint unsigned DEFAULT '0' COMMENT 'Number of blocks missed in this period',
  ...
)
```

**注意**: `block_signers` 表已被移除，不再存储每个块的详细签名记录。

### 2. 数据模型

- **ValidatorBlockStats 模型** (`internal/models/validator_block_stats.go`): 聚合统计表模型
- **SaveBlockSigners 函数** (`internal/models/block_signer.go`): 从 Tendermint 块数据中提取签名者信息，直接更新聚合统计表（不再保存到 `block_signers` 表）

### 3. 同步服务集成

修改了块同步逻辑 (`internal/service/sync.go`)，在保存块时自动提取和保存签名者信息：

- `processBlockOptimizedSkipCheck`: 单个块处理时保存签名者
- `processBlocksBatch`: 批量块处理时保存签名者

### 4. 统计服务

创建了 `ValidatorStatsService` (`internal/service/validator_stats.go`)，定期更新验证者统计：

- **更新频率**: 每 10 分钟更新一次
- **统计指标**:
  - `proposed`: 提议的块数（从 `validator_block_stats` 聚合统计表汇总）
  - `signed`: 签名的块数（从 `validator_block_stats` 聚合统计表汇总）
  - `missed`: 错过的块数（从 `validator_block_stats` 聚合统计表汇总：`should_sign_count - signed_count`）

### 5. API 接口

添加了新的 API 接口查询验证者统计信息：

**接口定义** (`api/explorer/v1/explorer.proto`):
```protobuf
rpc GetValidatorBlockStats(GetValidatorBlockStatsRequest) returns (Reply) {
  option (google.api.http) = {
    get : "/api/v1/validator/block-stats"
  };
}
```

**请求参数**:
- `validator_address` (string, required): 验证者操作地址

**响应数据**:
```json
{
  "validator_address": "injvaloper1...",
  "moniker": "Validator Name",
  "proposed": 1234,
  "signed": 5678,
  "missed": 12,
  "uptime_percentage": 99.79
}
```

## 使用方法

### 1. 数据库初始化

`validator_block_stats` 表的创建已包含在 `init_database.sql` 中。执行数据库初始化脚本即可创建所有表：

```bash
mysql -u username -p database_name < database/init_database.sql
```

**注意**: `block_signers` 表已被移除，不再需要创建。

### 2. 启动服务

服务启动时会自动：
- 启动 `ValidatorStatsService` 后台服务
- 在同步新块时自动更新聚合统计表（不再保存到 `block_signers` 表）
- 每 10 分钟更新一次验证者统计

### 3. 查询验证者统计

通过 HTTP API 查询：

```bash
curl "http://localhost:8000/api/v1/validator/block-stats?validator_address=injvaloper1..."
```

## 统计逻辑说明

### 数据存储优化（方案3：聚合统计表）

**问题**: 如果有100个验证者节点，每个块都记录所有验证者会导致数据量巨大（50,000条记录 vs 100条记录）。

**解决方案**: 
- **validator_block_stats 表（聚合统计表）**: 只存储聚合统计数据
  - 每个验证者每个周期只有1条记录
  - 100个验证者 × 1个周期 = 100条记录
  - 相比 `block_signers` 表的50,000条记录，**减少了99.8%**
  - 查询性能最优（直接查询聚合值，无需 COUNT）

**注意**: `block_signers` 表已被移除，不再存储每个块的详细签名记录。

### 提议块数 (Proposed)
- 从 `validator_block_stats` 表聚合计算：`SUM(proposed_count)` 跨所有周期
- 表示该验证者作为提议者出块的次数

### 签名块数 (Signed)
- 从 `validator_block_stats` 表聚合计算：`SUM(signed_count)` 跨所有周期
- 包括作为提议者的块和作为签名者参与的块

### 错过块数 (Missed)
- 从 `validator_block_stats` 表聚合计算：`SUM(missed_count)` 跨所有周期
- `missed_count = should_sign_count - signed_count`（每个周期内）
- **注意**: 当前 `should_sign_count` 只对实际签名的验证者设置为 true，未来需要获取验证者集合来准确计算所有活跃验证者的 `should_sign_count`

### 在线率 (Uptime Percentage)
- 计算公式: `(signed / (signed + missed)) * 100%`
- 表示验证者的签名参与率

## 未来改进

1. **精确的错过块数计算**: 
   - 需要存储每个块高度时的验证者集合
   - 可以查询 Tendermint 的验证者集合历史记录

2. **时间范围统计**:
   - 支持查询特定时间范围内的统计（如最近24小时、最近7天等）

3. **批量查询**:
   - 支持一次查询多个验证者的统计信息

4. **历史趋势**:
   - 存储历史统计数据，支持查看验证者表现趋势

## 数据量优化说明

### 采用方案3：聚合统计表（最优化，已实施）

假设：
- 100个验证者节点
- 每个块平均50个验证者参与签名
- 1000个块（1个统计周期）

**validator_block_stats 表（方案3，当前使用）**: 
- 100个验证者 × 1个周期 = 100条记录
- 相比 block_signers 表的50,000条记录，**减少了99.8%**
- 相比记录所有100个验证者（100,000条），**减少了99.9%**
- 查询性能最优（直接查询聚合值，无需 COUNT）

**block_signers 表（已移除）**: 
- ~~1000个块 × 50个验证者 = 50,000条记录~~
- **已移除**: 不再存储每个块的详细签名记录，大幅减少数据量

### 优势

1. **数据量最小**: 只存储聚合统计数据，不存储详细记录
2. **查询性能最优**: 直接查询聚合值，无需 COUNT 操作
3. **代码更简单**: 无需维护详细记录表

### 未来改进

1. **准确的 should_sign_count**: 需要定期获取验证者集合，准确计算所有活跃验证者应该签名的块数
2. **时间范围统计**: 支持查询特定时间范围内的统计（如最近24小时、最近7天等）
3. **历史趋势**: 存储历史统计数据，支持查看验证者表现趋势

## 相关文件

- `database/init_database.sql`: 数据库初始化脚本（包含 `validator_block_stats` 表创建，`block_signers` 表已移除）
- `internal/models/block_signer.go`: 签名者信息提取和聚合统计更新逻辑（不再保存到 `block_signers` 表）
- `internal/models/validator_block_stats.go`: ValidatorBlockStats 模型和聚合统计逻辑（方案3）
- `internal/service/validator_stats.go`: 验证者统计服务（使用聚合统计表）
- `internal/service/sync.go`: 块同步服务（已集成签名者信息提取和统计更新）
- `internal/service/explorer.go`: API 服务实现（使用聚合统计表查询）
- `api/explorer/v1/explorer.proto`: API 定义
- `internal/server/http.go`: HTTP 服务器启动（已集成统计服务）
- `docs/block-signers-table-analysis.md`: `block_signers` 表移除分析文档
