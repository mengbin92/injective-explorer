# 标签服务重构技术方案

## 📋 概述

本文档描述了如何重构和扩展injective-explorer的标签服务，以满足产品需求文档中的所有要求。

**设计原则：**
1. **向后兼容** - 保持现有API接口不变
2. **渐进式实施** - 分阶段实施，降低风险
3. **统一架构** - 地址标签和合约标签使用统一的标签系统
4. **性能优化** - 支持批量操作和缓存

**参考文档：**
- 产品需求：`~/Downloads/公链后台产品需求V0.0.1.md`
- 相关章节：地址标签管理、智能合约标签管理、INJ数据监控

---

## 🎯 需求分析

### 当前实现
1. **AddressLabel** - 地址标签
   - 表：`address_labels`
   - 字段：chain_id, address, label_type, label_value, label_color, description, created_by, created_at, updated_at
   - 唯一约束：(chain_id, address, label_type)
   - API：CreateAddressLabel, UpdateAddressLabel, GetAddressLabels, DeleteAddressLabel

2. **ContractTag** - 合约标签
   - 表：`contract_tags`
   - 字段：chain_id, contract_address, tag_type, tag_value, tag_color, description, created_by, created_at, updated_at
   - 唯一约束：(chain_id, contract_address, tag_type)
   - API：AddContractTag, RemoveContractTag, GetContractTags

### 产品需求

#### 1. 地址标签管理（第3.1节）
- ✅ 一个地址可以有多个标签
- ✅ 每个标签可以独立设置是否显示在前端
- ✅ 支持批量导入导出
- ✅ 支持统计指标（已标记地址数、显示/隐藏标签数）
- ✅ 支持简单CSV格式（兼容INJ数据监控）

#### 2. 智能合约标签管理（第3.2节）
- ✅ 标签分类：正常（绿色）、异常（红色）
- ✅ 从预设标签中选择
- ✅ 标签管理：预置标签列表，可统计使用次数
- ✅ 每个标签可以设置是否显示在前端

#### 3. INJ数据监控（第3.3节）
- ✅ CSV简单格式：地址,标签名称,备注
- ✅ 支持地址标签+备注一体化管理

### 差距分析

| 需求 | 当前实现 | 差距 | 优先级 |
|------|---------|------|--------|
| 一个地址多个标签 | ❌ 唯一约束限制 | P0 | 调整唯一约束 |
| 显示控制 | ❌ 无visible字段 | P0 | 添加visible字段 |
| 预设标签 | ❌ 无标签定义表 | P1 | 新建tag_definitions表 |
| 地址备注 | ❌ 无独立备注表 | P1 | 新建address_remarks表 |
| 批量导入导出 | ❌ 无批量API | P1 | 新增批量API |
| 统计指标 | ❌ 无统计API | P2 | 新增统计API |

---

## 🏗️ 技术方案

### 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                      标签服务架构                              │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────┐      ┌──────────────────┐            │
│  │ AddressLabelService│     │ ContractInfoService│          │
│  │  - 地址标签CRUD   │      │  - 合约标签CRUD   │            │
│  │  - 批量导入导出   │      │  - 预设标签选择   │            │
│  └────────┬─────────┘      └────────┬─────────┘            │
│           │                          │                       │
│           └──────────┬───────────────┘                       │
│                      ▼                                       │
│           ┌──────────────────────┐                          │
│           │ TagDefinitionService  │                          │
│           │  - 预设标签CRUD       │                          │
│           │  - 使用统计           │                          │
│           └──────────────────────┘                          │
│                                                               │
│  ┌──────────────────┐                                        │
│  │AddressRemarkService│                                       │
│  │  - 地址备注CRUD   │                                       │
│  └──────────────────┘                                        │
└─────────────────────────────────────────────────────────────┘
```

### 数据库设计

#### 1. 修改address_labels表

**重要说明：唯一性约束调整**

根据需求文档，一个地址可以添加多个标签（如"Binance"、"热钱包"、"内部备注"），这些标签通过`label_value`（标签名称）区分。当前实现的唯一约束是`(chain_id, address, label_type)`，需要调整为基于标签名称的唯一约束。

**方案选择：**
- **方案A（推荐）**：调整唯一约束为`(chain_id, address, label_value, deleted_at)`，允许同一地址有多个不同类型的标签
- **方案B**：保持现有约束，但将`label_type`设为固定值（如"custom"），通过`label_value`区分不同标签

考虑到向后兼容性，采用**方案A**，但需要数据迁移：

**变更：**
- 添加`visible`字段（显示控制）
- 调整唯一约束为`(chain_id, address, label_value, deleted_at)`

**原因：**
- `visible`字段实现显示控制
- 唯一约束改为`label_value`，允许一个地址有多个不同标签

**迁移SQL：**
```sql
-- migrations/001_alter_address_labels_add_visible_and_adjust_unique.sql

-- ⚠️ 重要：执行前请备份数据库

-- 1. 检查现有数据是否有冲突（同一地址同一label_type有多条记录）
-- 如果有，需要先处理数据
SELECT chain_id, address, label_type, COUNT(*) as cnt
FROM address_labels
WHERE deleted_at IS NULL
GROUP BY chain_id, address, label_type
HAVING cnt > 1;

-- 2. 删除旧的唯一约束
ALTER TABLE address_labels
DROP INDEX uk_address_label;

-- 3. 添加visible字段
ALTER TABLE address_labels
ADD COLUMN visible TINYINT(1) DEFAULT 1 COMMENT '是否在前端显示（1=显示，0=隐藏）'
AFTER description;

-- 4. 为现有数据设置默认值
UPDATE address_labels SET visible = 1 WHERE visible IS NULL;

-- 5. 添加新的唯一约束（基于标签名称）
ALTER TABLE address_labels
ADD UNIQUE KEY `uk_address_label_value` (`chain_id`, `address`, `label_value`, `deleted_at`);

-- 6. 添加索引
ALTER TABLE address_labels
ADD INDEX idx_visible (visible);
ALTER TABLE address_labels
ADD INDEX idx_label_value (label_value);
```

**字段说明：**
- `visible`：新增字段，控制该标签是否在区块链浏览器前端显示
  - 1：显示（默认）
  - 0：隐藏（仅后台可见）
- `label_value`：标签名称（如"Binance"、"热钱包"），用于区分同一地址的不同标签
- `label_type`：标签类型（如"exchange"、"wallet"），用于分类，不再作为唯一性约束的一部分

**数据迁移注意事项：**
- 如果现有数据中存在同一地址同一`label_type`的多条记录，需要合并或重命名`label_value`以确保唯一性
- 迁移前需要检查数据一致性

#### 2. 修改contract_tags表

**重要说明：与tag_definitions的关联**

根据需求文档，合约标签应该"从预设标签中选择"。为实现此功能，需要在`contract_tags`表中添加对`tag_definitions`的引用：

**变更：**
- 添加`visible`字段（显示控制）
- 添加`tag_definition_id`字段（关联预设标签）
- 添加`is_preset`字段（是否预设标签）

**迁移SQL：**
```sql
-- migrations/002_alter_contract_tags_add_preset_support.sql

-- 1. 添加visible字段
ALTER TABLE contract_tags
ADD COLUMN visible TINYINT(1) DEFAULT 1 COMMENT '是否在前端显示（1=显示，0=隐藏）'
AFTER description;

-- 2. 添加tag_definition_id字段（关联到tag_definitions表）
ALTER TABLE contract_tags
ADD COLUMN tag_definition_id BIGINT UNSIGNED COMMENT '关联的预置标签定义ID（可选，如果从预设标签选择则填写）'
AFTER visible;

-- 3. 添加is_preset字段（冗余字段，便于查询）
ALTER TABLE contract_tags
ADD COLUMN is_preset TINYINT(1) DEFAULT 0 COMMENT '是否为系统预置标签（1=是，0=否）'
AFTER tag_definition_id;

-- 4. 添加索引
ALTER TABLE contract_tags
ADD INDEX idx_visible (visible);
ALTER TABLE contract_tags
ADD INDEX idx_is_preset (is_preset);
ALTER TABLE contract_tags
ADD INDEX idx_tag_definition_id (tag_definition_id);

-- 5. 添加外键约束（可选，如果使用外键）
-- ALTER TABLE contract_tags
-- ADD CONSTRAINT fk_contract_tag_definition FOREIGN KEY (tag_definition_id) REFERENCES tag_definitions(id);
```

**字段说明：**
- `visible`：新增字段，控制该标签是否在区块链浏览器前端显示
- `tag_definition_id`：关联到`tag_definitions`表的ID，如果标签是从预设标签中选择的，则填写此字段
- `is_preset`：冗余字段，标识是否为系统预置标签（可通过`tag_definition_id IS NOT NULL`判断）
- `tag_value`：标签名称，如果`tag_definition_id`存在，则应该与`tag_definitions.tag_name`保持一致

**使用规则：**
1. **从预设标签选择**：填写`tag_definition_id`，`tag_value`自动从`tag_definitions`获取，`tag_color`和`tag_category`也自动继承
2. **自定义标签**：不填写`tag_definition_id`，手动填写`tag_value`、`tag_color`等字段

#### 3. 创建tag_definitions表

**用途：** 管理预设标签定义（管理员在后台预定义常用标签）

**表结构：**
```sql
-- migrations/003_create_tag_definitions.sql

CREATE TABLE tag_definitions (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  tag_type VARCHAR(20) NOT NULL COMMENT 'Tag type: contract, address',
  tag_name VARCHAR(50) NOT NULL COMMENT 'Tag name',
  tag_category VARCHAR(20) NOT NULL COMMENT 'Tag category: normal, abnormal',
  tag_color VARCHAR(20) COMMENT 'Tag color (hex code)',
  description TEXT COMMENT 'Tag description',
  is_system TINYINT(1) DEFAULT 1 COMMENT 'Is system preset tag (1: yes, 0: no)',
  usage_count INT DEFAULT 0 COMMENT 'Usage count (how many times this tag is used)',
  sort_order INT DEFAULT 0 COMMENT 'Sort order (for display ordering)',
  created_by VARCHAR(128) COMMENT 'Creator',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME DEFAULT NULL,
  UNIQUE KEY uk_tag_type_name (tag_type, tag_name, deleted_at),
  KEY idx_tag_type (tag_type),
  KEY idx_tag_category (tag_category),
  KEY idx_is_system (is_system)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Preset tag definitions';
```

**预设数据：**
```sql
-- 合约标签 - 正常（绿色）
INSERT INTO tag_definitions (tag_type, tag_name, tag_category, tag_color, description, is_system, sort_order) VALUES
('contract', '官方', 'normal', '#52c41a', '官方项目标签', 1, 1),
('contract', '稳定币', 'normal', '#52c41a', '稳定币标签', 1, 2),
('contract', 'DeFi', 'normal', '#52c41a', 'DeFi协议标签', 1, 3),
('contract', 'CEX', 'normal', '#52c41a', '中心化交易所标签', 1, 4),
('contract', '跨链桥', 'normal', '#52c41a', '跨链桥标签', 1, 5),
('contract', 'DEX', 'normal', '#52c41a', '去中心化交易所标签', 1, 6),
('contract', 'NFT', 'normal', '#52c41a', 'NFT相关标签', 1, 7);

-- 合约标签 - 异常（红色）
INSERT INTO tag_definitions (tag_type, tag_name, tag_category, tag_color, description, is_system, sort_order) VALUES
('contract', '诈骗', 'abnormal', '#ff4d4f', '诈骗项目标签', 1, 100),
('contract', '洗钱', 'abnormal', '#ff4d4f', '洗钱相关标签', 1, 101),
('contract', '高风险', 'abnormal', '#ff4d4f', '高风险项目标签', 1, 102),
('contract', '已废弃', 'abnormal', '#ff4d4f', '已废弃项目标签', 1, 103);

-- 地址标签
INSERT INTO tag_definitions (tag_type, tag_name, tag_category, tag_color, description, is_system, sort_order) VALUES
('address', '交易所', 'normal', '#1890ff', '交易所地址', 1, 1),
('address', '做市商', 'normal', '#1890ff', '做市商地址', 1, 2),
('address', '巨鲸', 'normal', '#1890ff', '巨鲸地址', 1, 3),
('address', '内部账户', 'normal', '#1890ff', '内部账户', 1, 4);
```

#### 4. 创建address_remarks表

**用途：** 地址备注（仅后台管理系统可见，前端浏览器不显示）

**表结构：**
```sql
-- migrations/004_create_address_remarks.sql

CREATE TABLE address_remarks (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  address VARCHAR(128) NOT NULL COMMENT 'Address',
  chain_id VARCHAR(50) NOT NULL COMMENT 'Chain ID',
  remark TEXT NOT NULL COMMENT 'Remark content',
  created_by VARCHAR(128) COMMENT 'Creator',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME DEFAULT NULL,
  UNIQUE KEY uk_address_remark (chain_id, address, deleted_at),
  KEY idx_address (chain_id, address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Address remarks (backend only)';
```

---

## 📡 API设计

### 1. TagDefinitionService (新服务)

**proto文件：** `api/explorer/v1/tag_definition.proto`

**API列表：**
- `ListTagDefinitions` - 列表查询标签定义
- `CreateTagDefinition` - 创建标签定义
- `UpdateTagDefinition` - 更新标签定义
- `DeleteTagDefinition` - 删除标签定义
- `GetTagUsageStats` - 获取使用统计
- `GetTagDefinitionsForSelection` - 获取可选标签（前端下拉框）

**HTTP路由：**
```
GET    /api/v1/tag/definitions                 # 列表查询
POST   /api/v1/tag/definition                  # 创建
PUT    /api/v1/tag/definition                  # 更新
DELETE /api/v1/tag/definition                  # 删除
GET    /api/v1/tag/usage-stats                 # 使用统计
GET    /api/v1/tag/definitions/selection       # 可选标签列表
```

### 2. AddressRemarkService (新服务)

**proto文件：** `api/explorer/v1/address_remark.proto`

**API列表：**
- `SetAddressRemark` - 设置/更新地址备注
- `GetAddressRemark` - 获取地址备注
- `DeleteAddressRemark` - 删除地址备注

**HTTP路由：**
```
POST   /api/v1/address/remark                  # 设置/更新
GET    /api/v1/address/remark                  # 获取
DELETE /api/v1/address/remark                  # 删除
```

### 3. AddressLabelService (扩展)

**新增API：**
- `GetAddressLabelStats` - 获取地址标签统计
- `BatchImportAddressLabels` - 批量导入地址标签
- `BatchExportAddressLabels` - 批量导出地址标签

**HTTP路由：**
```
GET    /api/v1/address/labels/stats            # 统计
POST   /api/v1/address/labels/batch-import     # 批量导入
POST   /api/v1/address/labels/batch-export     # 批量导出
```

**统计指标实现：**

`GetAddressLabelStats`返回以下统计信息：
- `total_labeled_addresses`：已标记地址数（去重）
- `visible_labels_count`：显示中的标签数
- `hidden_labels_count`：隐藏的标签数
- `total_labels_count`：标签总数

**实现SQL示例：**
```sql
-- 已标记地址数（去重）
SELECT COUNT(DISTINCT CONCAT(chain_id, ':', address)) as total_labeled_addresses
FROM address_labels
WHERE chain_id = ? AND deleted_at IS NULL;

-- 显示中的标签数
SELECT COUNT(*) as visible_labels_count
FROM address_labels
WHERE chain_id = ? AND visible = 1 AND deleted_at IS NULL;

-- 隐藏的标签数
SELECT COUNT(*) as hidden_labels_count
FROM address_labels
WHERE chain_id = ? AND visible = 0 AND deleted_at IS NULL;

-- 标签总数
SELECT COUNT(*) as total_labels_count
FROM address_labels
WHERE chain_id = ? AND deleted_at IS NULL;
```

**CSV格式支持：**

**简单格式（兼容INJ数据监控）：**
```csv
地址,标签名称,备注
0x742d35Bd6113c38B7a4D2949EA87A3A0A5A0Df3c,Binance热钱包,主要充值地址
0x5d3a78Bc9f8e12Ef3a4b5C6d7E8f9A0b1C2d3E4f,做市商A,主要做市商
```

**完整格式：**
```csv
地址,标签名称,标签类型,标签颜色,描述,是否显示,备注
0x1234567890abcdef1234567890abcdef12345678,Binance,exchange,#FF5733,币安交易所,true,主要充值地址
0x1234567890abcdef1234567890abcdef12345678,热钱包,wallet,#ffc107,热钱包,true,主要充值地址
0xabcdef1234567890abcdef1234567890abcdef12,项目方,project,#28a745,项目方地址,true,官方团队
0x567890abcdef1234567890abcdef1234567890ab,内部备注,custom,#6c757d,内部备注,false,内部编号001
```

**说明：**
- 简单格式：默认`label_type`为"custom"，`visible`为true，`label_color`为空
- 完整格式：包含所有字段，支持更精细的控制
- 一个地址可以有多行（多个标签）
- 备注信息单独存储到`address_remarks`表

**导入逻辑：**
1. 检测CSV格式（通过列数或表头判断）
2. 简单格式：默认`label_type`为"custom"，`visible`为true，`label_color`为空
3. 完整格式：按列解析所有字段
4. 如果地址已存在标签，根据`import_mode`决定是覆盖、跳过还是更新
5. 备注信息单独存储到`address_remarks`表

---

## 🔄 实施计划

### Phase 1: 数据库迁移 (低风险)

**任务：**
1. 执行`migrations/003_create_tag_definitions.sql`
2. 执行`migrations/004_create_address_remarks.sql`

**验证：**
```sql
SHOW TABLES LIKE 'tag_definitions';
SHOW TABLES LIKE 'address_remarks';
SELECT COUNT(*) FROM tag_definitions;  -- 应该有15条预设数据
```

**回滚：**
```sql
DROP TABLE IF EXISTS address_remarks;
DROP TABLE IF EXISTS tag_definitions;
```

### Phase 2: 修改现有表 (中风险)

**任务：**
1. 执行`migrations/001_alter_address_labels_add_visible_and_adjust_unique.sql`
2. 执行`migrations/002_alter_contract_tags_add_preset_support.sql`

**验证：**
```sql
DESCRIBE address_labels;  -- 应该有visible字段
DESCRIBE contract_tags;   -- 应该有visible, tag_definition_id, is_preset字段
SHOW INDEX FROM address_labels WHERE Key = 'uk_address_label_visible';
SHOW INDEX FROM contract_tags WHERE Key = 'idx_tag_definition_id';
```

**回滚：**
```sql
-- 无法完全回滚（唯一约束变更已生效）
-- 备份数据：mysqldump before migration
```

### Phase 3: API实现 (低风险)

**任务：**
1. 实现TagDefinitionService
   - 创建`internal/service/tag_definition.go`
   - 实现CRUD操作
   - 实现使用统计功能
   - 实现`GetTagDefinitionsForSelection`方法（用于前端标签选择器）

2. 实现AddressRemarkService
   - 创建`internal/service/address_remark.go`
   - 实现CRUD操作

3. 扩展AddressLabelService
   - 更新`internal/service/address_label.go`
     - 添加visible字段支持
     - 调整唯一性约束逻辑（基于label_value）
     - 实现批量导入导出（支持简单和完整CSV格式）
     - 实现统计API（GetAddressLabelStats）

4. 更新ContractInfoService
   - 更新`internal/service/contract_info.go`
     - 添加visible字段支持
     - 添加tag_definition_id字段支持
     - 实现从预设标签选择的功能（自动继承tag_definitions的属性）
     - 添加使用统计逻辑（更新tag_definitions.usage_count）
     - 在RemoveContractTag中实现usage_count递减

**注册服务：**
```go
// internal/server/grpc.go
tagDefinitionService := service.NewTagDefinitionService()
v1.RegisterTagDefinitionServiceServer(srv, tagDefinitionService)

addressRemarkService := service.NewAddressRemarkService()
v1.RegisterAddressRemarkServiceServer(srv, addressRemarkService)

// internal/server/http.go
tagDefinitionService := service.NewTagDefinitionService()
v1.RegisterTagDefinitionServiceHTTPServer(srv, tagDefinitionService)

addressRemarkService := service.NewAddressRemarkService()
v1.RegisterAddressRemarkServiceHTTPServer(srv, addressRemarkService)
```

### Phase 4: 前端集成 (中风险)

**任务：**
1. 添加预设标签管理页面
2. 更新合约标签添加页面（从预设选择）
3. 添加地址备注管理页面
4. 添加批量导入导出功能
5. 添加统计指标显示

---

## ⚠️ 风险评估

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|----------|
| 唯一约束变更导致数据冲突 | 高 | 中 | 执行前检查重复数据 |
| 现有API兼容性破坏 | 高 | 低 | 保持字段向后兼容（visible默认true） |
| 迁移脚本执行失败 | 高 | 低 | 先在测试环境验证，备份数据 |
| 性能下降 | 中 | 低 | 添加索引，优化查询 |

---

## 📊 测试计划

### 单元测试

**TagDefinitionService:**
```go
func TestTagDefinitionService_CreateTagDefinition(t *testing.T)
func TestTagDefinitionService_UpdateTagDefinition(t *testing.T)
func TestTagDefinitionService_DeleteTagDefinition(t *testing.T)
func TestTagDefinitionService_GetTagUsageStats(t *testing.T)
```

**AddressRemarkService:**
```go
func TestAddressRemarkService_SetAddressRemark(t *testing.T)
func TestAddressRemarkService_GetAddressRemark(t *testing.T)
func TestAddressRemarkService_DeleteAddressRemark(t *testing.T)
```

### 集成测试

**批量导入导出：**
```go
func TestAddressLabelService_BatchImportAddressLabels(t *testing.T)
func TestAddressLabelService_BatchExportAddressLabels(t *testing.T)
```

**预设标签选择：**
```go
func TestContractInfoService_AddContractTag_Preset(t *testing.T)
```

### 性能测试

**批量导入性能：**
- 测试10,000条地址标签导入
- 目标：< 5秒

**统计查询性能：**
- 测试GetAddressLabelStats
- 目标：< 100ms

---

## 📝 回滚计划

### 如果Phase 1失败
```sql
DROP TABLE IF EXISTS address_remarks;
DROP TABLE IF EXISTS tag_definitions;
```

### 如果Phase 2失败
```sql
-- 无法完全回滚
-- 建议：从备份恢复
mysqldump database_name < backup_before_migration.sql
```

### 如果Phase 3失败
```go
// 注释掉新服务注册
// internal/server/grpc.go
// tagDefinitionService := service.NewTagDefinitionService()
// v1.RegisterTagDefinitionServiceServer(s, tagDefinitionService)
```

### 如果Phase 4失败
```bash
git revert <commit-hash>
```

---

## 🎯 成功标准

1. ✅ 所有数据库迁移成功执行
2. ✅ 新API可正常调用并返回预期结果
3. ✅ 现有API保持向后兼容
4. ✅ 批量导入10,000条记录 < 5秒
5. ✅ 统计查询响应时间 < 100ms
6. ✅ 单元测试覆盖率 > 80%
7. ✅ 无性能回归

---

## 📚 附录

### A. 预设标签列表

**合约标签 - 正常：**
1. 官方 (#52c41a)
2. 稳定币 (#52c41a)
3. DeFi (#52c41a)
4. CEX (#52c41a)
5. 跨链桥 (#52c41a)
6. DEX (#52c41a)
7. NFT (#52c41a)

**合约标签 - 异常：**
1. 诈骗 (#ff4d4f)
2. 洗钱 (#ff4d4f)
3. 高风险 (#ff4d4f)
4. 已废弃 (#ff4d4f)

**地址标签：**
1. 交易所 (#1890ff)
2. 做市商 (#1890ff)
3. 巨鲸 (#1890ff)
4. 内部账户 (#1890ff)

### B. CSV导入导出格式示例

#### B.1 地址标签CSV格式

**简单格式（兼容INJ数据监控）：**
```csv
地址,标签名称,备注
0x742d35Bd6113c38B7a4D2949EA87A3A0A5A0Df3c,Binance热钱包,主要充值地址
0x5d3a78Bc9f8e12Ef3a4b5C6d7E8f9A0b1C2d3E4f,做市商A,主要做市商
```

**完整格式：**
```csv
地址,标签名称,标签类型,标签颜色,描述,是否显示,备注
0x1234567890abcdef1234567890abcdef12345678,Binance,exchange,#FF5733,币安交易所,true,主要充值地址
0x1234567890abcdef1234567890abcdef12345678,热钱包,wallet,#ffc107,热钱包,true,主要充值地址
0xabcdef1234567890abcdef1234567890abcdef12,项目方,project,#28a745,项目方地址,true,官方团队
0x567890abcdef1234567890abcdef1234567890ab,内部备注,custom,#6c757d,内部备注,false,内部编号001
```

**说明：**
- 简单格式：默认`label_type`为"custom"，`visible`为true，`label_color`为空
- 完整格式：包含所有字段，支持更精细的控制
- 一个地址可以有多行（多个标签）
- 备注信息单独存储到`address_remarks`表

#### B.2 合约标签CSV格式

```csv
contract_address,chain_id,标签名称,标签定义ID,标签颜色,描述,是否显示
0x1111111111111111111111111111111111111111,injective-1,官方,1,#28a745,官方代币,true
0x2222222222222222222222222222222222222222,injective-1,稳定币,2,#28a745,稳定币,true
0x3333333333333333333333333333333333333333,injective-1,诈骗,8,#dc3545,已确认的诈骗,false
```

**说明：**
- `标签定义ID`：如果填写，表示从预设标签选择，会自动继承`tag_definitions`中的颜色和分类
- 如果不填写`标签定义ID`，则为自定义标签，需要手动填写`标签颜色`和`描述`

### C. API调用示例

#### C.1 创建地址标签（支持visible字段）

```bash
curl -X POST http://localhost:8000/api/v1/address/label \
  -H "Content-Type: application/json" \
  -d '{
    "address": "0x1234567890abcdef1234567890abcdef12345678",
    "chain_id": "injective-1",
    "label_type": "exchange",
    "label_value": "Binance",
    "label_color": "#FF5733",
    "description": "币安交易所",
    "visible": true,
    "created_by": "admin"
  }'
```

#### C.2 设置地址备注

```bash
curl -X POST http://localhost:8000/api/v1/address/remark \
  -H "Content-Type: application/json" \
  -d '{
    "address": "0x1234567890abcdef1234567890abcdef12345678",
    "chain_id": "injective-1",
    "remark": "主要充值地址，日交易量大",
    "created_by": "admin"
  }'
```

#### C.3 创建预置标签

```bash
curl -X POST http://localhost:8000/api/v1/tag/definition \
  -H "Content-Type: application/json" \
  -d '{
    "tag_type": "contract",
    "tag_name": "DeFi 2.0",
    "tag_category": "normal",
    "tag_color": "#6f42c1",
    "description": "DeFi 2.0相关项目",
    "sort_order": 10,
    "created_by": "admin"
  }'
```

#### C.4 从预设标签选择添加合约标签

```bash
curl -X POST http://localhost:8000/api/v1/contract/info/tag \
  -H "Content-Type: application/json" \
  -d '{
    "contract_address": "0x1111111111111111111111111111111111111111",
    "chain_id": "injective-1",
    "tag_definition_id": 1,
    "visible": true,
    "created_by": "admin"
  }'
```

#### C.5 获取标签使用统计

```bash
curl -X GET "http://localhost:8000/api/v1/tag/usage-stats?tag_type=contract"
```

**响应示例：**
```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "tag_name": "官方",
      "usage_count": 156,
      "tag_category": "normal"
    },
    {
      "tag_name": "稳定币",
      "usage_count": 89,
      "tag_category": "normal"
    },
    {
      "tag_name": "DeFi",
      "usage_count": 234,
      "tag_category": "normal"
    },
    {
      "tag_name": "诈骗",
      "usage_count": 12,
      "tag_category": "abnormal"
    }
  ]
}
```

---

---

## 🔍 实现细节补充

### 1. 合约标签从预设标签选择的实现逻辑

**服务层实现（AddContractTag）：**

当`tag_definition_id`存在时：
1. 从`tag_definitions`表查询预设标签定义
2. 自动继承预设标签的属性（`tag_name`、`tag_color`、`tag_category`、`description`）
3. 设置`is_preset = true`
4. 创建标签后，更新`tag_definitions.usage_count`（只有在创建新标签时更新）

### 2. 统计指标实现SQL

**地址标签统计：**

```sql
-- 已标记地址数（去重）
SELECT COUNT(DISTINCT CONCAT(chain_id, ':', address)) as total_labeled_addresses
FROM address_labels
WHERE chain_id = ? AND deleted_at IS NULL;

-- 显示中的标签数
SELECT COUNT(*) as visible_labels_count
FROM address_labels
WHERE chain_id = ? AND visible = 1 AND deleted_at IS NULL;

-- 隐藏的标签数
SELECT COUNT(*) as hidden_labels_count
FROM address_labels
WHERE chain_id = ? AND visible = 0 AND deleted_at IS NULL;

-- 标签总数
SELECT COUNT(*) as total_labels_count
FROM address_labels
WHERE chain_id = ? AND deleted_at IS NULL;
```

### 3. CSV导入实现逻辑

**简单格式解析：**

```go
func parseSimpleCSVFormat(content string) ([]AddressLabelImport, error) {
    reader := csv.NewReader(strings.NewReader(content))
    records, err := reader.ReadAll()
    if err != nil {
        return nil, err
    }
    
    var labels []AddressLabelImport
    for i, record := range records {
        // 跳过表头
        if i == 0 && (strings.Contains(record[0], "地址") || strings.Contains(record[0], "address")) {
            continue
        }
        
        if len(record) < 3 {
            continue
        }
        
        label := AddressLabelImport{
            Address:    record[0],
            LabelValue: record[1],
            Remark:     record[2],
            LabelType:  "custom",  // 默认类型
            Visible:    true,      // 默认显示
        }
        labels = append(labels, label)
    }
    
    return labels, nil
}
```

### 4. 兼容性保证

**向后兼容策略：**

**新增字段采用默认值：**
- `visible`字段默认为`true`（显示），不影响现有数据
- `is_preset`字段默认为`false`（非预置），不影响现有数据

**现有API调用不受影响：**
- 不传`visible`字段时，使用默认值`true`
- 不传`is_preset`字段时，使用默认值`false`

**示例：**
```json
// 旧API调用（继续有效）
POST /api/v1/address/label
{
  "address": "0x1234...",
  "chain_id": "injective-1",
  "label_type": "exchange",
  "label_value": "Binance"
}

// 新API调用（支持新字段）
POST /api/v1/address/label
{
  "address": "0x1234...",
  "chain_id": "injective-1",
  "label_type": "exchange",
  "label_value": "Binance",
  "visible": false,  // 新增字段
  "created_by": "admin"
}
```

---

**文档版本：** v2.0（优化版）
**创建日期：** 2026-01-26
**最后更新：** 2026-01-26
**状态：** 已优化，待评审
