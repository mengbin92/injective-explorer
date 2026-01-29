# Injective Explorer 技术方案文档

> **文档版本**: v2.0
> **更新日期**: 2026-01-29
> **项目**: Injective Explorer - Injective区块链浏览器API服务
> **更新内容**: 基于实际代码实现的完整技术方案

---

## 目录

1. [项目概述](#项目概述)
2. [技术栈](#技术栈)
3. [系统架构](#系统架构)
4. [核心服务](#核心服务)
5. [数据库设计](#数据库设计)
6. [中间件与基础设施](#中间件与基础设施)
7. [核心流程](#核心流程)
8. [API设计](#api设计)
9. [性能优化](#性能优化)
10. [部署架构](#部署架构)
11. [监控与运维](#监控与运维)

---

## 项目概述

Injective Explorer 是一个基于 Go-Kratos v2 框架构建的企业级区块链浏览器 API 服务，专门为 Injective 网络提供高性能的数据查询和分析功能。项目采用微服务架构，支持 HTTP/REST 和 gRPC 双协议，提供完整的区块链数据查询、实时同步、数据归档、API管理等功能。

### 核心功能

#### ✅ 已实现功能

**区块链数据查询**
- 区块查询：最新区块列表、区块详情、按高度查询、批量查询
- 交易查询：最新交易、交易详情、按哈希查询、批量查询、失败交易查询
- EVM交易：EVM交易详情查询、异步同步、批量处理
- 账户查询：账户信息、交易历史、余额查询

**实时同步**
- WebSocket订阅Tendermint新块事件
- 历史数据回填和缺失检测
- 分布式锁确保单实例同步
- 工作线程池并发处理
- EVM交易详情异步同步

**数据归档**
- 自动归档历史区块和交易数据
- 对象存储集成(S3/MinIO)
- 数据压缩(Gzip)
- 大字段分离存储

**合约与代币**
- 合约信息管理：创建、更新、查询、删除
- 合约标签管理
- 合约文件上传和导入导出(Excel)
- 代币列表和详情
- 代币转账历史
- 代币市场价格缓存同步

**验证者服务**
- 验证者信息查询
- 验证者出块统计
- 验证者签名统计
- 实时验证者集合更新

**标签管理**
- 地址标签管理(exchange, project, scam等类型)
- 标签定义管理
- 地址备注功能
- 批量标签操作

**API管理**
- API Token管理(创建、列表、删除)
- Token认证(SHA256哈希存储)
- 速率限制(Token桶算法)
- 月度配额管理
- IP白名单/黑名单
- 使用量统计和Dashboard
- API计划管理(Free/Pro/Premium)

**Webhook服务**
- Webhook配置管理
- 事件推送机制

**性能监控**
- Prometheus指标收集
- 缓存命中率统计
- API响应时间分布
- 性能分析工具(pprof)

---

## 技术栈

### 编程语言与框架

| 技术         | 版本     | 用途                          |
| ------------ | -------- | ----------------------------- |
| **Go**       | 1.25.3   | 主要编程语言                  |
| **Kratos**   | v2.8.0   | 微服务框架，提供HTTP/gRPC服务 |
| **Protobuf** | v1.36.8  | API定义和序列化               |
| **gRPC**     | v1.71.0  | RPC通信协议                   |

### 数据库与存储

| 技术         | 版本      | 用途                              |
| ------------ | --------- | --------------------------------- |
| **MySQL**    | 8.0+      | 主数据库（推荐）                  |
| **PostgreSQL** | -       | 可选数据库                        |
| **SQLite**   | -         | 开发测试数据库                    |
| **Redis**    | v9.16.0   | 缓存、队列、限流、分布式锁        |
| **GORM**     | v1.31.0   | ORM框架                           |
| **MinIO**    | v7.0.97   | 对象存储(开发)                    |
| **AWS S3**   | v1.93.2   | 对象存储(生产)                    |

### 区块链集成

| 技术                       | 版本     | 用途                          |
| -------------------------- | -------- | ----------------------------- |
| **Injective SDK**          | v1.59.0  | Injective区块链交互           |
| **Cosmos SDK**             | v0.50.12 | Cosmos生态支持                |
| **CometBFT**               | v1.0.1   | Tendermint共识引擎            |
| **Ethereum Go**            | v1.15.7  | EVM支持(Injective EVM)        |

### 工具库

| 技术                | 版本      | 用途                          |
| ------------------- | --------- | ----------------------------- |
| **Zap**             | v1.27.0   | 结构化日志                    |
| **Sonic**           | v1.14.2   | 高性能JSON编解码              |
| **Ristretto**       | v2.1.0    | 高性能本地缓存(L1缓存)        |
| **Prometheus**      | v1.23.2   | 性能指标收集                  |
| **Excelize**        | v2.10.0   | Excel导入导出                 |
| **GeoIP2**          | v1.13.0   | IP地理位置查询                |
| **Swagger/OpenAPI** | -         | API文档生成                   |

---

## 系统架构

### 整体架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                        客户端层                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │ Web前端  │  │ 移动端   │  │ API调用  │  │ gRPC客户端│        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                       网关/代理层                                │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              Envoy Proxy (gRPC-Web)                      │  │
│  │  - gRPC-Web转换  - CORS处理  - 负载均衡                   │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              Nginx (可选，生产环境)                        │  │
│  │  - HTTP反向代理  - SSL终止  - 静态资源                    │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                     应用服务层 (Kratos)                          │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    HTTP Server (8000)                    │  │
│  │  - RESTful API  - Swagger UI  - CORS支持                  │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    gRPC Server (9000)                    │  │
│  │  - gRPC API  - Streaming RPC  - 高性能RPC                │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                   中间件层                                │  │
│  │  - Recovery  - Logging  - CORS  - Auth & RateLimit       │  │
│  │  - IP Whitelist  - Stats Collection                      │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                   业务服务层 (22个服务)                    │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │  │
│  │  │同步服务  │  │EVM同步   │  │浏览器服务│  │合约服务  │    │  │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │  │
│  │  │代币服务  │  │验证者服务│  │API管理   │  │标签服务  │    │  │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │  │
│  │  │归档服务  │  │Webhook   │  │监控服务  │                  │  │
│  │  └──────────┘  └──────────┘  └──────────┘                  │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                       数据访问层                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │  GORM    │  │TieredCache│ │ Storage  │  │Chain SDK │        │
│  │ (MySQL)  │  │ L1+L2    │  │(S3/MinIO)│  │(Injective)│        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                       数据存储层                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │  MySQL   │  │  Redis   │  │  MinIO  │  │ Injective │        │
│  │          │  │          │  │   S3    │  │ Network  │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└─────────────────────────────────────────────────────────────────┘
```

### 服务分层架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Presentation Layer                       │
│  HTTP/gRPC Server + Middleware Stack                        │
│  (Recovery, Logging, CORS, Auth, RateLimit, Stats)         │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                    Business Logic Layer                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ SyncService  │  │ EVMDetailsSync│ │ExplorerService│     │
│  │              │  │ Service      │  │              │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ContractInfo  │  │TokenList/    │  │Validator     │     │
│  │Service       │  │DetailService │  │StatsService  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │APIToken      │  │AddressLabel  │  │ArchiveService│     │
│  │Service       │  │Service       │  │              │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                    Data Access Layer                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Database   │  │  TieredCache │  │   Storage    │     │
│  │   (GORM)     │  │  (L1+Redis)  │  │  (MinIO/S3)  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ EVM Client   │  │Explorer SDK  │  │  GeoIP2      │     │
│  │              │  │              │  │              │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                    External Services                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ Injective    │  │  Tendermint  │  │  Prometheus  │     │
│  │ Blockchain   │  │  WebSocket   │  │  Monitoring  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

---

## 核心服务

### 1. SyncService (区块链同步服务)

**位置**: `internal/service/sync.go`

**职责**:
- 实时订阅区块链新块事件
- 历史数据回填和缺失检测
- 并发处理区块和交易数据
- 账户信息和余额更新

**关键特性**:
- WebSocket实时订阅 (Tendermint RPC)
- 分布式锁确保单实例运行 (Redis SetNX)
- 工作线程池并发处理
- 批量事务处理队列
- 自动故障恢复机制

**处理流程**:
```
1. 启动服务
   ├─ 获取分布式锁 (Redis)
   ├─ 初始化区块链客户端
   ├─ 验证当前块高度
   └─ 启动工作线程池

2. 实时同步
   ├─ WebSocket订阅新块事件
   ├─ 工作线程处理块数据
   ├─ 提取交易哈希到队列
   └─ 批量处理交易

3. 历史同步
   ├─ 检测缺失块
   ├─ 向后回填数据
   └─ 批量处理优化
```

### 2. EVMDetailsSyncService (EVM交易详情同步)

**位置**: `internal/service/evm_details_sync.go`

**职责**:
- 异步获取EVM交易详情
- 批量处理和缓冲优化
- 请求合并避免重复

**关键特性**:
- 请求合并机制 (避免重复查询)
- 批量更新缓冲 (定期刷新)
- 电路断路器模式 (故障保护)
- 重试机制 (指数退避)

**配置**:
```yaml
evm_details_sync:
  queue_key: "evm:sync:queue"
  worker_count: 5
  batch_size: 100
  batch_update_size: 50
  flush_interval: 5s
```

### 3. ExplorerService (浏览器查询服务)

**位置**: `internal/service/explorer.go`

**核心API**:
- `GetLatestBlocks()` - 获取最新区块
- `GetBlockByHeight()` - 按高度查询区块
- `GetLatestTransactions()` - 获取最新交易
- `GetTransactionByHash()` - 按哈希查询交易
- `GetAccountInfo()` - 获取账户信息
- `GetAccountTransactions()` - 获取账户交易
- `GetAccountBalances()` - 获取账户余额
- `GetEVMTransaction()` - EVM交易查询
- `BatchGetBlocks()` - 批量查询区块
- `BatchGetTransactions()` - 批量查询交易

### 4. ContractInfoService (合约信息服务)

**位置**: `internal/service/contract_info.go`

**功能**:
- 合约信息CRUD操作
- 合约标签管理
- 合约文件上传和管理
- Excel导入导出
- 合约搜索和过滤

### 5. TokenListService & TokenDetailService (代币服务)

**位置**: `internal/service/token_list.go`, `internal/service/token_detail.go`

**功能**:
- 代币列表查询 (分页、排序)
- 代币详情查询
- 代币转账历史
- 市场价格信息
- 代币持有者统计

### 6. ValidatorStatsService (验证者统计服务)

**位置**: `internal/service/validator_stats.go`

**功能**:
- 验证者出块统计
- 验证者签名统计
- 实时验证者集合更新
- 历史统计数据

### 7. APITokenService (API管理服务)

**位置**: `internal/service/api_token.go`

**功能**:
- API Token创建和管理
- Token认证 (SHA256哈希)
- 使用配额统计
- Dashboard数据
- Token使用历史

### 8. AddressLabelService (地址标签服务)

**位置**: `internal/service/address_label.go`

**功能**:
- 地址标签CRUD
- 标签类型管理 (exchange, project, scam等)
- 批量标签操作
- 标签可见性控制

### 9. ArchiveService (数据归档服务)

**位置**: `internal/service/archive.go`

**功能**:
- 自动归档历史数据
- 数据压缩 (Gzip)
- 对象存储上传
- 归档调度管理

---

## 数据库设计

### 核心表结构 (30+个表)

#### 1. blocks (区块表)
```sql
CREATE TABLE `blocks` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `height` BIGINT UNSIGNED NOT NULL UNIQUE,
  `proposer` VARCHAR(255),
  `block_hash` VARCHAR(64) NOT NULL,
  `parent_hash` VARCHAR(64),
  `num_precommits` INT DEFAULT 0,
  `num_txs` INT DEFAULT 0,
  `total_txs` BIGINT DEFAULT 0,
  `timestamp` DATETIME,
  `block_unix_timestamp` BIGINT,
  `archived` BOOLEAN DEFAULT FALSE,
  `txs_storage_key` VARCHAR(512),
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_height` (`height`),
  INDEX `idx_height_desc` (`height` DESC),
  INDEX `idx_blocks_archived_timestamp` (`archived`, `block_unix_timestamp`)
);
```

#### 2. transactions (交易表)
```sql
CREATE TABLE `transactions` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `block_number` BIGINT UNSIGNED NOT NULL,
  `hash` VARCHAR(64) NOT NULL UNIQUE,
  `code` INT DEFAULT 0,
  `gas_wanted` BIGINT DEFAULT 0,
  `gas_used` BIGINT DEFAULT 0,
  `gas_fee` VARCHAR(255),
  `signer` TEXT,
  `to` TEXT,
  `messages` LONGBLOB,
  `messages_storage_key` VARCHAR(512),
  `events` LONGBLOB,
  `events_storage_key` VARCHAR(512),
  `signatures` LONGBLOB,
  `values` JSON,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_block_number` (`block_number`),
  INDEX `idx_hash` (`hash`),
  INDEX `idx_block_enhance` (`block_number`, `hash`, `signer`(100), `to`(100), `values`(100))
);
```

#### 3. evm_transactions (EVM交易表)
```sql
CREATE TABLE `evm_transactions` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `transaction_hash` VARCHAR(66) NOT NULL UNIQUE,
  `evm_details_synced` BOOLEAN DEFAULT FALSE,
  `evm_from_address` VARCHAR(42),
  `evm_to_address` VARCHAR(42),
  `evm_details_storage_key` VARCHAR(512),
  `raw_details_json` LONGBLOB,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_transaction_hash` (`transaction_hash`),
  INDEX `idx_evm_from` (`evm_from_address`),
  INDEX `idx_evm_to` (`evm_to_address`)
);
```

#### 4. api_tokens (API Token表)
```sql
CREATE TABLE `api_tokens` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `user_id` VARCHAR(64) NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `token_hash` VARCHAR(64) NOT NULL UNIQUE,
  `token_preview` VARCHAR(20) NOT NULL,
  `plan_id` BIGINT UNSIGNED,
  `plan_type` VARCHAR(20) DEFAULT 'free',
  `status` VARCHAR(20) DEFAULT 'active',
  `last_used_at` TIMESTAMP NULL,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_token_hash` (`token_hash`),
  INDEX `idx_status` (`status`)
);
```

#### 5. api_request_logs (API请求日志表)
```sql
CREATE TABLE `api_request_logs` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `token_id` BIGINT UNSIGNED,
  `user_id` VARCHAR(64),
  `ip_address` VARCHAR(45) NOT NULL,
  `ip_location` VARCHAR(100),
  `endpoint` VARCHAR(255) NOT NULL,
  `method` VARCHAR(10) NOT NULL,
  `status_code` INT NOT NULL,
  `response_time` INT NOT NULL,
  `error_message` TEXT,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_api_logs_token_time` (`token_id`, `created_at`),
  INDEX `idx_api_logs_ip_time` (`ip_address`, `created_at`),
  INDEX `idx_api_logs_user_time` (`user_id`, `created_at`)
);
```

#### 6. address_labels (地址标签表)
```sql
CREATE TABLE `address_labels` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `chain_id` VARCHAR(50) NOT NULL,
  `address` VARCHAR(100) NOT NULL,
  `label_type` VARCHAR(50) NOT NULL,
  `label_value` VARCHAR(200) NOT NULL,
  `label_color` VARCHAR(20),
  `description` TEXT,
  `visible` BOOLEAN DEFAULT TRUE,
  `created_by` VARCHAR(64),
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_address_label_value` (`chain_id`, `address`, `label_value`)
);
```

#### 其他重要表
- `accounts` - 账户信息
- `account_balances` - 账户余额
- `contract_infos` - 合约信息
- `contract_tags` - 合约标签
- `validators` - 验证者信息
- `validator_block_stats` - 验证者出块统计
- `token_market_cache` - 代币市场价格
- `token_transfers` - 代币转账
- `block_data_archives` - 块归档
- `transaction_data_archives` - 交易归档
- `webhooks` - Webhook配置
- `api_plans` - API计划
- `usage_daily_stats` - 日使用统计
- `ip_blacklist/whitelist` - IP黑白名单

---

## 中间件与基础设施

### 1. Redis 分层缓存

**架构**:
```
L1 缓存 (本地内存 - Ristretto)
├─ 响应时间: µs级
├─ 容量: 可配置 (默认1000条目)
└─ TTL: 可配置

L2 缓存 (Redis)
├─ 响应时间: ms级
├─ 分布式共享
├─ 持久化支持
└─ 高可用 (Sentinel/Cluster)
```

**缓存策略**:
```
请求 → L1缓存 → L2缓存 → 数据库
       ↑ 命中    ↑ 命中    ↑ 更新L1+L2
```

**用途**:
- 分层缓存: L1(Ristretto) + L2(Redis)
- 消息队列: EVM同步队列、交易处理队列
- 分布式锁: 同步服务锁
- 限流计数: Token桶算法
- 请求合并: 避免重复查询

**配置**:
```yaml
redis:
  addr: localhost:6379
  password: ""
  db: 0
  pool_size: 10
  read_timeout: 3s
  write_timeout: 3s
  enable_metrics: true
```

### 2. 中间件栈

**SelectiveAuthAndRateLimitMiddleware**
- Token认证 (SHA256哈希验证)
- IP白名单检查
- 速率限制 (Token桶算法)
- 月度配额检查
- IP地理位置查询 (GeoIP2)
- 使用量统计

**请求处理流程**:
```
HTTP/gRPC请求
  ↓
Recovery中间件 (异常恢复)
  ↓
Logging中间件 (请求日志)
  ↓
CORS中间件 (跨域处理)
  ↓
SelectiveAuthAndRateLimitMiddleware
  ├─ API Token验证
  ├─ IP白名单检查
  ├─ 速率限制
  ├─ 配额检查
  └─ IP地理位置
  ↓
业务服务处理
  ↓
数据访问层
  ↓
返回响应
  ↓
统计收集 (异步)
```

### 3. 对象存储

**支持提供商**:
- MinIO (开发/测试)
- AWS S3
- 阿里云 OSS
- 腾讯云 COS

**用途**:
- 归档历史交易数据
- 存储大字段 (>3KB)
- 降低数据库存储成本

**配置**:
```yaml
object_storage:
  provider: s3  # s3, minio, oss, cos
  endpoint: https://s3.amazonaws.com
  access_key_id: ""
  secret_access_key: ""
  bucket_name: injective-explorer
  region: us-east-1
  use_ssl: true
  enabled: true
```

---

## 核心流程

### 1. 区块同步流程

```
┌─────────────────────────────────────────────────────────────┐
│                    区块同步流程                              │
└─────────────────────────────────────────────────────────────┘

启动服务
  ↓
尝试获取分布式锁 (Redis SetNX)
  ├─ 成功 → 继续同步
  └─ 失败 → 跳过同步(其他实例运行中)
  ↓
初始化区块链客户端
  ├─ Tendermint RPC客户端
  ├─ Injective Explorer SDK
  └─ EVM客户端
  ↓
初始化并验证当前高度
  ├─ 查询数据库最后块高
  ├─ 查询链上最新块高
  └─ 块哈希验证确保一致性
  ↓
┌─────────────────┐
│ WebSocket订阅   │ ← 订阅Tendermint新块事件
└─────────────────┘
  ↓
启动工作线程池 (可配置数量)
  ↓
┌─────────────────┐
│ 接收新块事件     │
└─────────────────┘
  ↓
processBlock(height)
  ├─ 获取块详情
  ├─ 提取交易哈希
  ├─ 保存块数据到数据库
  ├─ 更新账户余额
  └─ 发送交易到处理队列
  ↓
┌─────────────────┐
│ 交易处理线程     │ ← 从Redis队列拉取
└─────────────────┘
  ├─ 批量查询交易详情
  ├─ 解析交易消息
  ├─ 更新账户信息
  ├─ 压缩大字段 (>1KB)
  └─ 保存到数据库
  ↓
┌─────────────────┐
│ 历史块同步       │ ← 后台补充缺失块
└─────────────────┘
  ├─ 检测缺失块范围
  ├─ 批量处理
  └─ 继续填补缺口
```

### 2. EVM交易详情同步流程

```
┌─────────────────────────────────────────────────────────────┐
│                 EVM交易详情同步流程                          │
└─────────────────────────────────────────────────────────────┘

SyncService发现EVM交易
  ↓
发送交易哈希到Redis队列 ("evm:sync:queue")
  ↓
EVMDetailsSyncService监听队列
  ↓
┌─────────────────┐
│ 请求合并检查     │ ← 避免重复查询
└─────────────────┘
  ├─ 检查是否已在处理中
  └─ 合并重复请求
  ↓
通过EVM RPC获取交易详情
  ├─ eth_getTransactionReceipt
  ├─ eth_getTransactionByHash
  └─ 电路断路器保护
  ↓
批量缓冲更新 (达到阈值或超时)
  ↓
判断数据大小
  ├─ < 3KB → 直接存储到数据库
  └─ >= 3KB → 压缩后上传到对象存储
  ↓
更新evm_transactions表
  ├─ evm_details_synced = true
  ├─ evm_from_address
  ├─ evm_to_address
  └─ raw_details_json 或 storage_key
  ↓
更新缓存
```

### 3. API请求处理流程

```
┌─────────────────────────────────────────────────────────────┐
│                  API请求处理流程                             │
└─────────────────────────────────────────────────────────────┘

HTTP/gRPC请求
  ↓
Recovery中间件 (Panic捕获)
  ↓
Logging中间件 (请求ID生成)
  ↓
CORS中间件 (跨域头设置)
  ↓
SelectiveAuthAndRateLimitMiddleware
  ↓
┌─────────────────┐
│ 1. Token验证     │
└─────────────────┘
  ├─ 从Header或Query提取Token
  ├─ SHA256哈希验证
  ├─ 检查Token状态 (active/suspended)
  └─ 更新last_used_at
  ↓
┌─────────────────┐
│ 2. IP检查        │
└─────────────────┘
  ├─ IP白名单验证 (如启用)
  ├─ IP黑名单检查
  └─ GeoIP地理位置查询
  ↓
┌─────────────────┐
│ 3. 速率限制      │
└─────────────────┘
  ├─ Token桶算法
  ├─ 每秒限制检查
  ├─ 月度配额检查
  └─ 触发限制 → HTTP 429
  ↓
业务服务处理
  ↓
┌─────────────────┐
│ 4. 数据访问      │
└─────────────────┘
  ├─ L1缓存查询 (Ristretto)
  ├─ L2缓存查询 (Redis)
  └─ 数据库查询
  ↓
返回响应
  ↓
┌─────────────────┐
│ 5. 异步统计      │
└─────────────────┘
  ├─ 记录请求日志
  ├─ 更新使用量统计
  └─ 更新缓存
```

### 4. 数据归档流程

```
┌─────────────────────────────────────────────────────────────┐
│                    数据归档流程                              │
└─────────────────────────────────────────────────────────────┘

定时任务触发 (Cron调度)
  ↓
查询需归档的数据
  ├─ 块: archived=false AND age>阈值
  └─ 交易: 关联块已归档
  ↓
批量处理 (每批1000条)
  ↓
┌─────────────────┐
│ 数据压缩         │ ← Gzip压缩
└─────────────────┘
  ├─ 压缩messages字段
  ├─ 压缩events字段
  └─ 压缩raw_details_json
  ↓
判断存储策略
  ├─ 压缩后 < 3KB → 存储到归档表
  └─ 压缩后 >= 3KB ↓
      上传到对象存储 (S3/MinIO)
      ├─ 生成存储key
      ├─ 上传压缩数据
      └─ 记录storage_key
  ↓
更新原表记录
  ├─ archived = true
  ├─ 清空原始大字段
  └─ 设置storage_key
  ↓
记录归档元数据
  ├─ storage_provider
  ├─ storage_key
  ├─ data_size
  └─ archived_at
  ↓
继续下一批
```

---

## API设计

### API架构
- **协议**: gRPC + REST (HTTP网关)
- **传输**: HTTP/2 (gRPC), HTTP/1.1 (REST)
- **API版本**: v1 (`/api/v1/`)
- **认证**: API Token (Bearer Token)
- **文档**: OpenAPI/Swagger

### 核心API端点

#### 块相关
```
GET  /api/v1/block/latest                - 获取最新区块
GET  /api/v1/block/by-height             - 按高度查询区块
GET  /api/v1/block/latest-height         - 获取最新块高
GET  /api/v1/block/transactions          - 获取块内交易
GET  /api/v1/block/gas-utilization       - Gas利用率
POST /api/v1/blocks/batch                - 批量查询区块
```

#### 交易相关
```
GET  /api/v1/transaction/latest          - 最新交易
GET  /api/v1/transaction/by-hash         - 按哈希查询
GET  /api/v1/transaction/stats           - 交易统计
GET  /api/v1/transaction/failed-24h      - 24小时失败交易
GET  /api/v1/transaction/failed-1000     - 最新1000个失败交易
POST /api/v1/transactions/batch          - 批量查询交易
```

#### 账户相关
```
GET /api/v1/account/info                 - 账户信息
GET /api/v1/account/transactions         - 账户交易历史
GET /api/v1/account/balances             - 账户余额
```

#### EVM相关
```
GET /api/v1/evm/transaction              - EVM交易查询
GET /api/v1/evm/transactions             - EVM交易列表
```

#### 代币相关
```
GET /api/v1/token/list                   - 代币列表
GET /api/v1/token/details                - 代币详情
GET /api/v1/token/transfers              - 代币转账历史
```

#### API管理
```
POST   /api/v1/api-token/create          - 创建API Token
GET    /api/v1/api-token/list            - Token列表
DELETE /api/v1/api-token/{id}            - 删除Token
GET    /api/v1/api-token/usage           - Token使用统计
GET    /api/v1/api-token/dashboard       - 仪表板统计
```

#### 合约相关
```
POST   /api/v1/contract/create           - 创建合约信息
GET    /api/v1/contract/{address}        - 合约信息查询
PUT    /api/v1/contract/{address}        - 更新合约信息
DELETE /api/v1/contract/{address}        - 删除合约信息
POST   /api/v1/contract/tags/add         - 添加标签
POST   /api/v1/contract/upload           - 文件上传
GET    /api/v1/contract/export           - 导出Excel
POST   /api/v1/contract/import           - 导入Excel
```

#### 标签管理
```
GET    /api/v1/address-label              - 获取地址标签
POST   /api/v1/address-label              - 创建标签
DELETE /api/v1/address-label              - 删除标签
POST   /api/v1/tag-definition             - 标签定义管理
```

### 通用响应格式

```protobuf
message Reply {
  int32 code = 1;                      // 状态码 (0=成功)
  string message = 2;                  // 响应消息
  string request_id = 3;               // 请求追踪ID
  google.protobuf.Struct data = 4;     // 响应数据
}
```

---

## 性能优化

### 1. 数据库优化

**连接池配置**:
```go
MaxOpenConns:     100  // 最大打开连接数
MaxIdleConns:     30   // 最大空闲连接数
ConnMaxLifetime:  5m   // 连接最大生命周期
ConnMaxIdleTime:  3m   // 连接最大空闲时间
```

**索引优化**:
- 复合索引: `idx_block_enhance(block_number, hash, signer, to, value)`
- 时间索引: `idx_blocks_archived_timestamp(archived, block_unix_timestamp)`
- API日志: `idx_api_logs_token_time(token_id, created_at)`
- 唯一索引: `uk_address_label_value(chain_id, address, label_value)`

**查询优化**:
- 分页查询避免大量数据返回
- 使用覆盖索引减少回表
- 避免SELECT *,只查询需要的字段
- 批量操作减少数据库往返

### 2. 缓存优化

**分层缓存**:
```
L1 (Ristretto)
├─ 容量: 1000条目
├─ TTL: 5分钟
└─ 命中率: >90%

L2 (Redis)
├─ TTL: 1小时
├─ 持久化: 是
└─ 命中率: >80%
```

**缓存策略**:
- Cache-Aside模式
- 缓存预热 (启动时加载热点数据)
- 缓存失效策略 (TTL + 主动更新)
- 缓存穿透保护 (空值缓存)

### 3. 同步优化

**并发处理**:
- 工作线程池 (可配置worker_count)
- 批量处理 (减少数据库操作)
- 请求合并 (避免重复查询)
- 异步处理 (EVM详情同步)

**数据压缩**:
- Gzip压缩大字段 (>1KB)
- 对象存储迁移 (>3KB)
- 可配置压缩阈值

### 4. API优化

**响应优化**:
- 分页查询 (防止大量数据返回)
- 字段选择 (按需返回)
- 批量接口 (减少往返)
- 响应压缩 (gzip)

**速率限制**:
- Token桶算法 (精确控制)
- 分级限额 (不同计划)
- 月度配额 (防止过度使用)

---

## 部署架构

### Docker Compose部署

```yaml
version: '3.8'

services:
  injective-explorer:
    image: injective-explorer:latest
    ports:
      - "8000:8000"  # HTTP
      - "9000:9000"  # gRPC
    environment:
      - DATA_DATABASE_SOURCE=root:password@tcp(mysql:3306)/injective_explorer
      - DATA_REDIS_ADDR=redis:6379
      - NODE_TM_RPC=http://tendermint:26657
      - SERVICE_ENABLE_SYNC=true
    depends_on:
      - mysql
      - redis
      - minio

  mysql:
    image: mysql:8.0
    ports:
      - "3306:3306"
    environment:
      - MYSQL_ROOT_PASSWORD=password
      - MYSQL_DATABASE=injective_explorer

  redis:
    image: redis:latest
    ports:
      - "6379:6379"

  minio:
    image: minio/minio:latest
    ports:
      - "19000:9000"
      - "19001:9001"
    command: server /data --console-address ":9001"

  envoy:
    image: envoyproxy/envoy:v1.25-latest
    ports:
      - "9001:9001"  # gRPC-Web
      - "9901:9901"  # Admin
    volumes:
      - ./envoy/envoy.yaml:/etc/envoy/envoy.yaml
```

### 端口分配

| 服务               | 端口  | 说明             |
| ------------------ | ----- | ---------------- |
| injective-explorer HTTP | 8000  | RESTful API      |
| injective-explorer gRPC | 9000  | gRPC服务         |
| Envoy gRPC-Web     | 9001  | 浏览器gRPC访问   |
| Envoy Admin        | 9901  | Envoy管理界面    |
| MySQL              | 3306  | 数据库           |
| Redis              | 6379  | 缓存             |
| MinIO API          | 19000 | 对象存储API      |
| MinIO Console      | 19001 | 对象存储管理界面 |
| pprof              | 6060  | 性能分析(可选)   |

### 生产环境部署

```
┌─────────────────────────────────────────────────────────────┐
│          Load Balancer (Nginx/HAProxy)                      │
│          - SSL终止  - 负载均衡  - 健康检查                   │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│          API Gateway (Envoy)                                │
│          - gRPC-Web  - 路由  - 限流                          │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│          Application Servers (多实例)                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │Instance 1│  │Instance 2│  │Instance N│                  │
│  │(SYNC=ON) │  │(SYNC=OFF)│  │(SYNC=OFF)│                  │
│  └──────────┘  └──────────┘  └──────────┘                  │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│          Data Layer (高可用)                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │MySQL         │  │Redis Cluster │  │S3/OSS        │     │
│  │(Master-Slave)│  │(Sentinel)    │  │(对象存储)    │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

**关键配置**:
- 只有一个实例: `enable_sync=true`
- 其他实例: 通过分布式锁自动禁用同步
- 所有实例共享: 数据库、Redis、对象存储
- 负载均衡器分发API请求

### 水平扩展

**扩展策略**:
1. **无状态设计**: 所有实例无状态,易于扩展
2. **分布式锁**: 确保只有一个实例执行同步
3. **共享存储**: 数据库、Redis、对象存储集中管理
4. **负载均衡**: Nginx/HAProxy分发请求
5. **缓存预热**: 启动时预热缓存,提升性能

---

## 监控与运维

### Prometheus指标

**暴露端点**: `GET {http_addr}/metrics`

**核心指标**:
```
# 业务指标
injective_explorer_block_height                  - 当前块高度
injective_explorer_transaction_total             - 总交易数
injective_explorer_sync_duration_seconds         - 同步耗时
injective_explorer_api_requests_total            - API请求总数
injective_explorer_cache_hit_rate                - 缓存命中率

# 性能指标
injective_explorer_api_response_time_seconds     - API响应时间
injective_explorer_db_query_duration_seconds     - 数据库查询耗时
injective_explorer_redis_operation_duration      - Redis操作耗时

# Go运行时指标
go_goroutines                               - Goroutine数量
go_memstats_alloc_bytes                     - 内存分配
process_cpu_seconds_total                   - CPU使用
```

### 健康检查

**端点**:
- HTTP: `GET /health`
- gRPC: `grpc.health.v1.Health/Check`

**检查项**:
- 数据库连接
- Redis连接
- 区块链RPC连接
- 同步状态

### 日志管理

**日志格式**: JSON (生产) / Console (开发)

**日志级别**:
- DEBUG: 详细调试信息
- INFO: 一般信息
- WARN: 警告信息
- ERROR: 错误信息

**日志内容**:
- 请求追踪ID
- 响应时间
- 错误堆栈
- 业务事件

### 性能分析

**pprof端点** (如启用):
```
GET /debug/pprof/          - 性能分析首页
GET /debug/pprof/heap      - 堆内存分析
GET /debug/pprof/goroutine - Goroutine分析
GET /debug/pprof/profile   - CPU分析
```

**配置**:
```yaml
service:
  enable_pprof: true
  pprof_addr: localhost:6060
```

### 故障排查

**常见问题**:

| 问题 | 原因 | 解决方案 |
|-----|------|---------|
| 块同步停止 | 分布式锁被占用 | 检查Redis锁,手动释放 |
| 高内存占用 | 缓存过大 | 调整缓存大小,检查泄漏 |
| 数据库连接拒绝 | 连接数过多 | 增加max_connections |
| API超时 | 慢查询 | 检查索引,优化查询 |
| 同步延迟高 | 处理速度不足 | 增加worker_count |

---

## 总结

### 技术亮点

1. **高性能**:
   - 分层缓存 (L1内存 + L2 Redis)
   - 批量处理和并发优化
   - 数据库索引优化
   - 请求合并机制

2. **高可用**:
   - 分布式锁确保单实例同步
   - 自动故障恢复
   - 健康检查和监控
   - 水平扩展支持

3. **可扩展**:
   - 微服务架构
   - 多数据库支持
   - 对象存储分离
   - 模块化设计

4. **企业级**:
   - API Token管理
   - 速率限制和配额
   - 使用量统计
   - 审计日志

5. **开发友好**:
   - Protobuf类型安全
   - Swagger API文档
   - 完整错误处理
   - 性能分析工具

### 项目特色

- ✅ **完整实现**: 22个核心服务,30+数据库表
- ✅ **性能优化**: 分层缓存、批量处理、请求合并
- ✅ **企业功能**: API管理、限流、统计、监控
- ✅ **生产就绪**: Docker部署、监控告警、故障恢复
- ✅ **文档完善**: 技术方案、API文档、部署指南

---

**文档结束**
