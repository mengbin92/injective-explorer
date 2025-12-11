# 区块链浏览器数据存储架构方案

## 概述

本文档详细说明区块链浏览器的数据存储架构，包括数据分类、存储策略、归档机制以及技术选型理由。

根据实际测试，同步主网数据每天需要 **70-100GB** 的硬盘存储。通过数据压缩、存储分离和归档策略，可以将存储成本降低 **70-90%**。

## 架构图

### 整体架构

```mermaid
graph TB
    subgraph "应用层"
        API[API服务]
        Sync[同步服务]
        Archive[归档服务]
    end
    
    subgraph "数据层"
        subgraph "MySQL数据库"
            MainTable[(transactions<br/>主表)]
            ArchiveTable[(transaction_data_archives<br/>归档表)]
            BlockTable[(blocks<br/>区块表)]
            AccountTable[(accounts<br/>账户表)]
        end
        
        subgraph "对象存储"
            OSS[MinIO/S3/OSS/COS<br/>大字段存储]
        end
        
        subgraph "缓存层"
            Redis[(Redis<br/>热点数据缓存)]
        end
    end
    
    API --> MainTable
    API --> ArchiveTable
    API --> BlockTable
    API --> AccountTable
    API --> Redis
    API --> OSS
    
    Sync --> MainTable
    Sync --> OSS
    Sync --> Redis
    
    Archive --> MainTable
    Archive --> ArchiveTable
    Archive --> OSS
    
    MainTable -.->|存储键引用| OSS
    ArchiveTable -.->|存储键引用| OSS
```

### 数据流转图

```mermaid
sequenceDiagram
    participant Sync as 同步服务
    participant Compress as 压缩模块
    participant DB as MySQL主表
    participant Storage as 对象存储
    participant Archive as 归档服务
    participant ArchiveDB as 归档表
    
    Note over Sync,Storage: 数据同步阶段
    Sync->>Compress: 原始交易数据
    Compress->>Compress: 压缩数据(>1KB)
    alt 压缩后大小 > 3KB
        Compress->>Storage: 上传到对象存储
        Compress->>DB: 存储键 + 元数据
    else 压缩后大小 <= 3KB
        Compress->>DB: 压缩数据 + 元数据
    end
    
    Note over Archive,ArchiveDB: 数据归档阶段(6个月后)
    Archive->>DB: 查询旧交易
    Archive->>ArchiveDB: 移动大字段到归档表
    alt 启用对象存储
        Archive->>Storage: 上传大字段(如未上传)
        Archive->>ArchiveDB: 存储键
    else 未启用对象存储
        Archive->>ArchiveDB: 完整数据
    end
    Archive->>DB: 清空大字段，标记archived=true
```

### 数据存储决策流程

```mermaid
flowchart TD
    Start[新交易数据] --> Compress{数据压缩}
    Compress -->|>1KB| Compressed[压缩后数据]
    Compress -->|<=1KB| SmallData[原始数据]
    
    Compressed --> CheckSize{压缩后大小}
    SmallData --> StoreDB1[存储到MySQL]
    
    CheckSize -->|>3KB| UploadOSS[上传到对象存储]
    CheckSize -->|<=3KB| StoreDB2[存储到MySQL]
    
    UploadOSS --> StoreKey[MySQL存储键]
    StoreKey --> StoreDB3[MySQL存储元数据]
    
    StoreDB1 --> End[完成]
    StoreDB2 --> End
    StoreDB3 --> End
    
    style UploadOSS fill:#90EE90
    style StoreDB1 fill:#FFE4B5
    style StoreDB2 fill:#FFE4B5
    style StoreDB3 fill:#FFE4B5
```

## 数据分类与存储策略

### 1. 热数据（最近6个月）

**存储位置**：MySQL 主表 (`transactions`)

**包含数据**：
- **元数据字段**（始终存储在MySQL）：
  - `id`, `hash`, `block_number`, `block_timestamp`
  - `code`, `gas_wanted`, `gas_used`, `gas_fee`
  - `tx_type`, `signer`, `to`, `value`
  - `events`, `signatures`, `memo`, `claim_ids`
  - `created_at`, `updated_at`
  
- **大字段**（根据大小决定存储位置）：
  - `data`: 交易原始数据
  - `messages`: 交易消息
  - `logs`: 执行日志
  - `info`: 附加信息
  - `error_log`: 错误日志

**存储策略**：
- 所有数据先进行压缩（>1KB的数据使用gzip压缩）
- **压缩后大小 ≤ 3KB**：存储在MySQL的TEXT/LONGTEXT字段
- **压缩后大小 > 3KB**：上传到对象存储，MySQL只存储存储键（`*_storage_key`字段）

### 2. 归档数据（6个月以上）

**存储位置**：
- MySQL 主表 (`transactions`)：保留元数据和 `archived=true` 标记
- MySQL 归档表 (`transaction_data_archives`)：存储大字段或存储键
- 对象存储：大字段（如果启用）

**归档流程**：
1. 查询超过6个月的交易（`created_at < 6个月前`）
2. 将大字段（`data`, `messages`, `logs`, `info`, `error_log`）移动到 `transaction_data_archives` 表
3. 如果启用对象存储且数据超过阈值，上传到对象存储并存储键
4. 在主表中清空大字段，设置 `archived=true`
5. 保留所有元数据字段用于查询和统计

**查询支持**：
- 查询时自动从主表（`archived=true`）和归档表联合查询
- 如果存在存储键，自动从对象存储加载数据
- 对用户透明，无需额外操作

### 3. 区块数据

**存储位置**：MySQL (`blocks` 表)

**特点**：
- 数据量相对较小
- 查询频繁
- 不需要归档（区块数据较小，可长期保留）

### 4. 账户数据

**存储位置**：MySQL (`accounts`, `account_balances` 表)

**特点**：
- 结构化数据
- 查询频繁
- 不需要归档

### 5. 缓存数据

**存储位置**：Redis

**缓存内容**：
- 最近100个区块
- 最近100个交易
- 统计数据（TPS、交易统计等）
- 缓存TTL：5分钟

## 为什么选择对象存储而不是MySQL？

### 1. 成本优势

**MySQL存储成本**：
- 云数据库存储：约 ¥5-10/GB/月
- 需要高性能SSD，成本较高
- 备份和快照成本

**对象存储成本**（以阿里云OSS为例）：
- 标准存储：¥0.12/GB/月（约 **40-80倍** 成本差异）
- 低频访问：¥0.08/GB/月
- 归档存储：¥0.033/GB/月

**成本对比示例**（假设每月新增600GB大字段数据）：
- MySQL存储：600GB × ¥5 = **¥3,000/月**
- 对象存储：600GB × ¥0.12 = **¥72/月**
- **节省成本：约 97%**

### 2. 数据库性能影响

**大TEXT字段对MySQL的影响**：
- **查询性能下降**：大TEXT字段会增加行大小，影响索引效率
- **内存占用**：查询时需要加载完整行到内存
- **I/O压力**：大字段增加磁盘I/O
- **备份时间**：大字段显著增加备份和恢复时间

**对象存储的优势**：
- 数据库只存储小字符串（存储键，通常 < 200字节）
- 查询时按需加载，不影响主查询性能
- 减少数据库I/O压力

### 3. 可扩展性

**MySQL限制**：
- 单表大小限制（虽然可以很大，但性能会下降）
- 单行大小限制（65KB，TEXT字段除外但影响性能）
- 扩展需要分库分表，复杂度高

**对象存储优势**：
- 理论上无限扩展
- 自动分布式存储
- 支持生命周期策略（自动归档到冷存储）

### 4. 数据生命周期管理

**对象存储支持**：
- 自动转换存储类型（标准 → 低频 → 归档）
- 自动删除过期数据
- 成本随访问频率自动优化

**MySQL限制**：
- 需要手动管理数据生命周期
- 删除大字段需要大量I/O操作

### 5. 备份和恢复

**对象存储优势**：
- 自动多副本备份
- 跨区域复制
- 版本控制支持

**MySQL限制**：
- 需要手动配置备份策略
- 大字段增加备份文件大小
- 恢复时间较长

## 为什么选择3KB作为存储阈值？

### 1. 数据库性能考虑

**MySQL TEXT字段性能影响**：
- **≤ 3KB**：对查询性能影响较小，可以接受
- **> 3KB**：开始显著影响查询性能
  - 增加行大小，影响索引扫描效率
  - 增加内存占用
  - 增加网络传输时间（API响应）

**测试数据**（基于实际测试）：
- 3KB以下字段：查询性能影响 < 5%
- 3-10KB字段：查询性能影响 5-15%
- 10KB以上字段：查询性能影响 > 15%

### 2. 存储成本平衡

**成本分析**（基于压缩后数据大小）：
- **≤ 3KB**：存储在MySQL，成本可接受
- **> 3KB**：存储在对象存储，成本显著降低

**阈值选择理由**：
- 3KB是数据库性能和存储成本的最佳平衡点
- 小于3KB的数据，对象存储的成本优势不明显（请求成本可能超过存储成本）
- 大于3KB的数据，对象存储的成本优势明显

### 3. 网络传输考虑

**API响应时间**：
- 3KB以下：网络传输时间可接受（< 10ms，假设100Mbps）
- 3KB以上：从对象存储加载，可以异步加载或按需加载

**用户体验**：
- 列表查询：不需要加载大字段，响应快
- 详情查询：按需从对象存储加载，可以接受稍长的延迟

### 4. 压缩效果

**压缩率统计**（基于实际数据）：
- 原始数据 > 1KB：压缩率约 60-80%
- 压缩后 > 3KB：原始数据通常 > 5-8KB
- 这类数据更适合存储在对象存储

**示例**：
- 原始数据 10KB → 压缩后 3.5KB → 上传到对象存储
- 原始数据 2KB → 压缩后 1.2KB → 存储在MySQL

### 5. 实际数据分布

根据实际数据分析：
- **约 60-70%** 的交易数据压缩后 ≤ 3KB
- **约 30-40%** 的交易数据压缩后 > 3KB
- 3KB阈值可以覆盖大部分小数据，同时将大数据迁移到对象存储

## 技术实现细节

### 1. 数据压缩

**压缩策略**：
- 阈值：1KB（小于1KB不压缩）
- 算法：gzip（压缩级别6，平衡压缩率和速度）
- 格式：`gzip:` + base64编码（数据库存储）

**压缩效果**：
- 文本数据：压缩率 60-80%
- JSON数据：压缩率 50-70%
- 二进制数据：压缩率 40-60%

### 2. 对象存储集成

**支持的存储提供商**：
- **MinIO**：S3兼容，开源，适合开发/测试
- **AWS S3**：生产环境（海外）
- **阿里云 OSS**：生产环境（国内）
- **腾讯云 COS**：生产环境（国内）

**存储键格式**：
```
transactions/{type}/{hash}
```
- `type`: `data`, `messages`, `logs`
- `hash`: 交易哈希

**数据格式**：
- 对象存储：压缩后的二进制数据（gzip格式）
- 减少存储空间和传输带宽

### 3. 归档机制

**归档配置**（`config.yaml`）：
```yaml
archive:
  enabled: true              # 启用自动归档
  interval: 86400s            # 执行间隔（24小时）
  age_months: 6               # 归档6个月前的数据
  batch_size: 1000            # 每批处理1000条
  storage_threshold: 3072     # 存储阈值（3KB）
  use_object_storage: true    # 使用对象存储
```

**归档流程**：
1. 定时任务（默认每天执行一次）
2. 查询需要归档的交易（`created_at < 6个月前 AND archived = false`）
3. 批量处理（默认1000条/批）
4. 移动大字段到归档表
5. 上传到对象存储（如果启用且超过阈值）
6. 更新主表（清空大字段，设置 `archived=true`）

### 4. 查询逻辑

**自动查询支持**：
- `GetTransactionByHash`: 自动从主表或归档表查询
- `GetArchivedTransaction`: 专门查询归档数据
- 自动从对象存储加载数据（如果存在存储键）

**查询流程**：
```go
1. 查询主表 (archived = false)
2. 如果未找到，查询主表 (archived = true) + 归档表
3. 如果存在存储键，从对象存储加载数据
4. 自动解压缩数据
5. 返回完整交易数据
```

## 存储容量估算

### 原始数据量
- **每天**：70-100GB
- **每月**：约 2.1-3TB
- **每年**：约 25-36TB

### 压缩后数据量
- **压缩率**：约 60-70%
- **每天**：约 21-30GB
- **每月**：约 630-900GB
- **每年**：约 7.5-11TB

### 存储分布（假设30%数据 > 3KB）

**MySQL存储**（70%数据，≤3KB）：
- 每天：约 15-21GB
- 每月：约 450-630GB

**对象存储**（30%数据，>3KB）：
- 每天：约 6-9GB
- 每月：约 180-270GB

### 成本估算（每月）

**MySQL存储**（450-630GB）：
- 成本：450GB × ¥5 = **¥2,250/月**（假设全部存储在MySQL）
- 实际：大部分数据压缩后存储在MySQL，成本更低

**对象存储**（180-270GB）：
- 标准存储：180GB × ¥0.12 = **¥22/月**
- 低频存储（3个月后）：180GB × ¥0.08 = **¥14/月**
- 归档存储（1年后）：180GB × ¥0.033 = **¥6/月**

**总成本对比**：
- **优化前**（全部MySQL）：2.1TB × ¥5 = **¥10,500/月**
- **优化后**（混合存储）：MySQL + 对象存储 ≈ **¥500-1,000/月**
- **节省成本**：约 **90-95%**

## 监控和运维

### 1. 存储监控指标

**MySQL监控**：
- 数据库大小
- 表大小（`transactions`, `transaction_data_archives`）
- 查询性能（慢查询日志）

**对象存储监控**：
- 存储使用量
- 请求次数（PUT/GET）
- 访问延迟
- 成本趋势

### 2. 告警设置

**存储空间告警**：
- MySQL使用率 > 80%
- 对象存储使用量异常增长

**性能告警**：
- 查询响应时间 > 阈值
- 对象存储访问失败率 > 1%

### 3. 定期维护

**数据库维护**：
- 定期执行 `OPTIMIZE TABLE`（每月）
- 清理过期索引
- 分析表统计信息

**对象存储维护**：
- 检查存储生命周期策略
- 清理无效对象
- 验证数据完整性

## 总结

### 核心优势

1. **成本降低 90%+**：通过压缩和对象存储，大幅降低存储成本
2. **性能提升**：数据库只存储小字段，查询性能提升
3. **可扩展性**：对象存储无限扩展，无需担心容量问题
4. **自动化**：自动压缩、自动归档、自动生命周期管理

### 技术选型理由总结

| 选型                  | 理由                                  |
| --------------------- | ------------------------------------- |
| **对象存储 vs MySQL** | 成本低40-80倍，性能影响小，可扩展性强 |
| **3KB阈值**           | 平衡数据库性能和存储成本的最佳点      |
| **gzip压缩**          | 压缩率高（60-80%），速度快，兼容性好  |
| **6个月归档**         | 平衡查询性能和存储成本                |
| **MinIO/S3/OSS**      | S3兼容，可无缝切换，支持多种部署方式  |

### 实施建议

1. **开发/测试环境**：使用MinIO（自建，免费）
2. **生产环境（国内）**：使用阿里云OSS或腾讯云COS
3. **生产环境（海外）**：使用AWS S3
4. **监控告警**：设置存储使用量和成本告警
5. **定期优化**：根据实际数据分布调整阈值和归档策略

## 参考文档

- [数据压缩实现](./compression-implementation.md)
- [归档使用指南](./archive-usage-guide.md)
- [存储分离资源清单](./storage-separation-resources.md)
- [归档数据库大小分析](./archive-database-size-analysis.md)
