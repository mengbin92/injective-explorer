# Injective Explorer 技术方案文档

> **文档版本**: v1.0  
> **编写日期**: 2025-01-XX  
> **项目**: Injective Explorer - Injective区块链浏览器API服务

---

## 目录

1. [项目概述](#项目概述)
2. [技术栈](#技术栈)
3. [中间件与基础设施](#中间件与基础设施)
4. [数据库设计](#数据库设计)
5. [系统架构](#系统架构)
6. [核心流程](#核心流程)
7. [API管理方案（规划中）](#api管理方案规划中)
8. [部署架构](#部署架构)

---

## 项目概述

Injective Explorer 是一个基于 Kratos 框架构建的区块链浏览器 API 服务，专门为 Injective 网络提供数据查询和分析功能。项目采用微服务架构，支持 HTTP/REST 和 gRPC 双协议，提供高性能的区块链数据查询服务。

### 核心功能

- ✅ **区块查询**: 最新区块列表、区块详情、区块高度查询、批量查询
- ✅ **交易查询**: 最新交易列表、交易详情、交易哈希查询、批量查询、失败交易查询
- ✅ **EVM交易**: EVM交易详情查询、异步同步、批量处理
- ✅ **账户查询**: 账户信息、账户交易、账户余额
- ✅ **实时同步**: WebSocket订阅新块、历史数据回填、分布式锁机制
- ✅ **数据归档**: 自动归档历史交易数据到对象存储(S3/MinIO)
- ✅ **统计服务**: 交易统计、Gas统计、活跃地址统计
- ✅ **合约信息**: 智能合约查询、标签管理、导入导出
- ✅ **代币服务**: 代币列表、代币详情、代币转账历史、市场价格缓存
- ✅ **验证者服务**: 验证者信息、出块统计、签名统计
- ✅ **标签管理**: 地址标签、标签定义、地址备注
- ✅ **API管理**: Token管理、限流(Token桶算法)、使用量统计、IP白名单/黑名单
- ✅ **Webhook服务**: Webhook配置管理、事件推送
- ✅ **性能监控**: Prometheus指标、缓存命中率、性能分析

---

## 技术栈

### 编程语言与框架

| 技术         | 版本    | 用途                          |
| ------------ | ------- | ----------------------------- |
| **Go**       | 1.25.3  | 主要编程语言                  |
| **Kratos**   | v2.8.0  | 微服务框架，提供HTTP/gRPC服务 |
| **Protobuf** | v1.36.6 | API定义和序列化               |
| **gRPC**     | v1.71.0 | RPC通信协议                   |

### 数据库与存储

| 技术         | 版本/类型 | 用途                              |
| ------------ | --------- | --------------------------------- |
| **MySQL**    | 8.0       | 主数据库（支持PostgreSQL/SQLite） |
| **Redis**    | latest    | 缓存、队列、限流                  |
| **GORM**     | v1.31.0   | ORM框架                           |
| **MinIO/S3** | -         | 对象存储（归档数据）              |

### 区块链SDK

| 技术                 | 版本     | 用途                          |
| -------------------- | -------- | ----------------------------- |
| **Injective SDK**    | v1.59.0  | Injective区块链交互           |
| **Cosmos SDK**       | v0.50.12 | Cosmos生态支持                |
| **CometBFT**         | v1.0.1   | Tendermint共识引擎            |
| **Ethereum Go**      | v1.15.7  | EVM支持(Injective EVM)        |
| **Injective Explorer SDK** | - | Injective Explorer集成        |

### 工具库

| 技术                   | 版本      | 用途                          |
| ---------------------- | --------- | ----------------------------- |
| **Zap**                | v1.27.0   | 结构化日志                    |
| **Sonic**              | v1.14.2   | 高性能JSON编解码              |
| **Redis Go Client**    | v9.16.0   | Redis客户端(go-redis)         |
| **Ristretto**          | v2.1.0    | 高性能本地缓存(L1缓存)        |
| **Prometheus**         | v1.23.2   | 性能指标收集                  |
| **AWS SDK v2**         | v1.41.0   | AWS S3集成                    |
| **MinIO SDK**          | v7.0.97   | MinIO对象存储                 |
| **Excelize**           | v2.10.0   | Excel导入导出                 |
| **GeoIP2**             | v1.13.0   | IP地理位置查询                |
| **Swagger/OpenAPI**    | -         | API文档生成                   |

---

## 中间件与基础设施

### 1. Redis

**用途**:
- **分层缓存**: L1(本地内存Ristretto) + L2(Redis)双层缓存架构
- **缓存热点数据**: 区块、交易、账户信息、代币价格等
- **消息队列**: EVM交易详情同步队列、交易处理队列
- **分布式锁**: 同步服务单实例锁、防止重复处理
- **限流计数器**: Token桶算法限流、IP限流、月度配额管理
- **请求合并**: 避免重复的EVM查询,提升吞吐量

**配置**:
```yaml
redis:
  addr: localhost:6379
  password: ""
  db: 0
  pool_size: 10
  read_timeout: 3s
  write_timeout: 3s
  enable_metrics: true  # Prometheus指标
```

**缓存架构**:
```
请求 → L1缓存(Ristretto) → L2缓存(Redis) → 数据库
       ↑ 命中返回(µs级)   ↑ 命中返回(ms级)  ↑ 更新L1+L2
```

**关键特性**:
- 连接池优化,支持健康检查
- 自动断线重连机制
- 缓存失效策略(TTL)
- 性能指标收集(命中率、延迟)

### 2. Envoy Proxy

**用途**:
- gRPC-Web代理（浏览器兼容）
- 负载均衡
- CORS处理
- 请求路由

**端口**:
- `9001`: gRPC-Web端口
- `9901`: Envoy管理端口

### 3. 对象存储

**支持提供商**:
- MinIO（开发/测试）
- AWS S3

**用途**:
- 归档历史交易数据
- 存储大字段（压缩后的交易数据）
- 降低数据库存储成本

### 4. Swagger/OpenAPI

**用途**:
- API文档自动生成
- 交互式API测试
- API规范定义

**访问路径**:
- `/swagger/`: Swagger UI
- `/openapi.yaml`: OpenAPI规范
- `/openapi.json`: OpenAPI JSON格式

---

## 数据库设计

### 核心表结构

#### 1. Blocks（区块表）
```sql
CREATE TABLE `blocks` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `height` BIGINT UNSIGNED NOT NULL UNIQUE,
  `hash` VARCHAR(64) NOT NULL UNIQUE,
  `time` TIMESTAMP NOT NULL,
  `num_txs` INT NOT NULL DEFAULT 0,
  `gas_used` BIGINT UNSIGNED DEFAULT 0,
  `gas_wanted` BIGINT UNSIGNED DEFAULT 0,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_height` (`height`),
  INDEX `idx_time` (`time`)
);
```

#### 2. Transactions（交易表）
```sql
CREATE TABLE `transactions` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `hash` VARCHAR(64) NOT NULL UNIQUE,
  `block_height` BIGINT UNSIGNED NOT NULL,
  `block_time` TIMESTAMP NOT NULL,
  `fee` DECIMAL(36,18) DEFAULT 0,
  `gas_used` BIGINT UNSIGNED DEFAULT 0,
  `gas_wanted` BIGINT UNSIGNED DEFAULT 0,
  `memo` TEXT,
  `messages` JSON,
  `archived` BOOLEAN DEFAULT FALSE,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_hash` (`hash`),
  INDEX `idx_block_height` (`block_height`),
  INDEX `idx_block_time` (`block_time`),
  INDEX `idx_archived` (`archived`)
);
```

#### 3. TransactionDataArchive（交易归档表）
```sql
CREATE TABLE `transaction_data_archive` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `transaction_id` BIGINT UNSIGNED NOT NULL,
  `storage_key` VARCHAR(512),
  `storage_provider` VARCHAR(50),
  `data_size` BIGINT UNSIGNED DEFAULT 0,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (`transaction_id`) REFERENCES `transactions`(`id`) ON DELETE CASCADE,
  INDEX `idx_transaction_id` (`transaction_id`)
);
```

#### 4. Accounts（账户表）
```sql
CREATE TABLE `accounts` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `address` VARCHAR(64) NOT NULL UNIQUE,
  `account_number` BIGINT UNSIGNED DEFAULT 0,
  `sequence` BIGINT UNSIGNED DEFAULT 0,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_address` (`address`)
);
```

#### 5. ContractInfo（合约信息表）
```sql
CREATE TABLE `contract_info` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `contract_address` VARCHAR(64) NOT NULL UNIQUE,
  `code_id` BIGINT UNSIGNED,
  `creator` VARCHAR(64),
  `admin` VARCHAR(64),
  `label` VARCHAR(255),
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_contract_address` (`contract_address`)
);
```

### 数据库特性

- **多数据库支持**: MySQL、PostgreSQL、SQLite
- **自动迁移**: GORM AutoMigrate
- **索引优化**: 关键字段建立索引
- **分区表**: 支持按时间分区（归档场景）
- **软删除**: 使用`deleted_at`字段

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
│  │  - Recovery  - Logging  - CORS  - 认证(规划中)            │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                   业务服务层                              │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │  │
│  │  │Demo服务  │  │合约服务  │  │同步服务  │  │归档服务  │    │  │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │  │
│  │  │价格服务  │  │统计服务  │  │验证服务  │                  │  │
│  │  └──────────┘  └──────────┘  └──────────┘                  │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                       数据访问层                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │  GORM    │  │  Redis   │  │  Storage │  │ Chain SDK│        │
│  │ (MySQL)  │  │ (Cache)  │  │ (MinIO)  │  │(Injective)│        │
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
│  HTTP/gRPC Server + Middleware (Recovery, Logging, CORS)    │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                    Business Logic Layer                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ DemoService  │  │ ContractInfo │  │ SyncService  │     │
│  │              │  │ Service      │  │              │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ PriceService │  │ ArchiveService│ │ ValidatorService│   │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                    Data Access Layer                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Database   │  │    Cache     │  │   Storage    │     │
│  │   (GORM)     │  │   (Redis)    │  │  (MinIO/S3)  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                    External Services                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ Injective    │  │  CoinGecko   │  │   (Future)   │     │
│  │ Blockchain   │  │  Price API   │  │  API Mgmt    │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
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
初始化区块链客户端 (Injective SDK)
  ↓
获取当前数据库最新区块高度
  ↓
┌─────────────────┐
│ 向后同步缺失区块 │ ← 检测缺失区块
└─────────────────┘
  ↓
┌─────────────────┐
│ 实时监控新区块   │ ← 轮询Tendermint RPC
└─────────────────┘
  ↓
获取区块数据
  ↓
┌─────────────────┐
│ 并发处理交易     │ ← Worker Pool (5个并发)
└─────────────────┘
  ↓
保存到数据库
  ↓
更新Redis缓存
  ↓
继续监控下一个区块
```

### 2. 交易查询流程

```
┌─────────────────────────────────────────────────────────────┐
│                    交易查询流程                              │
└─────────────────────────────────────────────────────────────┘

客户端请求交易数据
  ↓
检查Redis缓存
  ↓
┌─────────────┐
│ 缓存命中？   │
└─────────────┘
  ├─ 是 → 返回缓存数据
  │
  └─ 否 ↓
      查询数据库
      ↓
      ┌─────────────┐
      │ 数据库存在？ │
      └─────────────┘
        ├─ 是 → 返回数据 + 更新缓存
        │
        └─ 否 ↓
            查询区块链API (Injective)
            ↓
            保存到数据库
            ↓
            更新缓存
            ↓
            返回数据
```

### 3. 数据归档流程

```
┌─────────────────────────────────────────────────────────────┐
│                    数据归档流程                              │
└─────────────────────────────────────────────────────────────┘

定时任务触发 (每天执行)
  ↓
查询6个月前的交易数据
  ↓
批量处理 (每批1000条)
  ↓
┌─────────────────┐
│ 压缩大字段数据   │ ← 使用Gzip压缩
└─────────────────┘
  ↓
┌─────────────────┐
│ 判断存储方式     │
└─────────────────┘
  ├─ 小于阈值 → 存储到数据库归档表
  │
  └─ 大于阈值 → 上传到对象存储 (MinIO/S3)
      ↓
      更新交易记录 (archived=true)
      ↓
      删除原始大字段数据
      ↓
      继续下一批
```

### 4. API请求处理流程

```
┌─────────────────────────────────────────────────────────────┐
│                  API请求处理流程                             │
└─────────────────────────────────────────────────────────────┘

HTTP/gRPC请求
  ↓
Recovery中间件 (异常恢复)
  ↓
Logging中间件 (请求日志)
  ↓
CORS中间件 (跨域处理)
  ↓
┌─────────────────┐
│ 认证中间件       │ ← (规划中: Token验证)
└─────────────────┘
  ↓
┌─────────────────┐
│ 限流中间件       │ ← (规划中: IP/Token/账户限流)
└─────────────────┘
  ↓
业务服务处理
  ↓
数据访问层 (Database/Cache/Storage)
  ↓
返回响应
  ↓
统计中间件 (规划中: 使用量统计)
```

---

## API管理方案（规划中）

### 功能概述

根据 `docs/api-management-technical-design.md` 文档，计划实现以下功能：

#### 1. API Token管理
- Token格式: `inj_` + 32位随机字符（Base64 URL编码）
- Token存储: SHA256哈希存储
- Token预览: 前6位 + "..." + 后6位
- 套餐类型: Free/Paid

#### 2. 限流系统
- **IP限流**: 每分钟限制（滑动窗口）
- **Token限流**: 每秒限制（滑动窗口）
- **账户限流**: 月度限额（Redis计数器）

#### 3. 使用量统计
- 异步统计（1-5秒延迟）
- 请求日志记录
- 日统计汇总
- 90天数据归档

#### 4. IP归属地查询
- 使用MaxMind GeoLite2免费数据库
- 异步查询，不阻塞API

#### 5. 数据导出
- Excel分页导出
- 流式写入，避免内存溢出

#### 6. Webhook（预留）
- 仅付费Token支持
- 事件订阅机制

### 数据库设计（规划中）

#### API Token表
```sql
CREATE TABLE `api_tokens` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `user_id` VARCHAR(64) NOT NULL,
  `name` VARCHAR(50) NOT NULL,
  `token_hash` VARCHAR(64) NOT NULL UNIQUE,
  `token_preview` VARCHAR(20) NOT NULL,
  `plan_type` VARCHAR(20) NOT NULL DEFAULT 'free',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active',
  `last_used_at` TIMESTAMP NULL,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_token_hash` (`token_hash`)
);
```

#### API请求日志表
```sql
CREATE TABLE `api_request_logs` (
  `id` BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  `token_id` BIGINT UNSIGNED NOT NULL,
  `user_id` VARCHAR(64) NOT NULL,
  `ip_address` VARCHAR(45) NOT NULL,
  `ip_location` VARCHAR(100),
  `endpoint` VARCHAR(255) NOT NULL,
  `method` VARCHAR(10) NOT NULL,
  `status_code` INT NOT NULL,
  `response_time` INT NOT NULL,
  `error_message` TEXT,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_token_id` (`token_id`),
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_created_at` (`created_at`)
) PARTITION BY RANGE (UNIX_TIMESTAMP(created_at));
```

### 实施计划

**Phase 1: 核心功能（2周）**
- 数据库表创建
- Token管理（创建/删除/列表）
- Token认证中间件
- 基础限流（IP + Token）

**Phase 2: 完善功能（2周）**
- 使用量统计
- 账户月度限流
- IP归属地查询
- Dashboard数据接口

**Phase 3: 优化扩展（1-2周）**
- 数据归档
- Excel导出
- Webhook预留

---

## 部署架构

### Docker Compose部署

```
┌─────────────────────────────────────────────────────────────┐
│                    Docker Network                            │
│                  (injective-explorer-network)                     │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ injective-explorer│  │    Envoy     │  │    MySQL     │     │
│  │   :8000      │  │   :9001      │  │   :3306      │     │
│  │   :9000      │  │   :9901      │  │              │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │    Redis     │  │    MinIO     │                        │
│  │   :6379      │  │  :19000      │                        │
│  │              │  │  :19001      │                        │
│  └──────────────┘  └──────────────┘                        │
└─────────────────────────────────────────────────────────────┘
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

### 生产环境部署建议

```
┌─────────────────────────────────────────────────────────────┐
│                    Load Balancer (Nginx/HAProxy)            │
│                    - SSL终止  - 负载均衡                      │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                    API Gateway (Envoy)                       │
│                    - gRPC-Web  - 路由                        │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│               Application Servers (多实例)                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ Instance 1   │  │ Instance 2    │  │ Instance N   │     │
│  │ injective-explorer│  │ injective-explorer│  │ injective-explorer│     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│                    Data Layer                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ MySQL        │  │ Redis Cluster│  │ S3/OSS       │     │
│  │ (Master-Slave)│ │ (Sentinel)   │  │ (对象存储)   │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

### 配置管理

- **环境变量**: 支持环境变量覆盖配置
- **配置文件**: YAML格式，支持多环境配置
- **配置优先级**: 环境变量 > 配置文件

### 监控与日志

- **日志**: Zap结构化日志，支持JSON/Console格式
- **指标**: Prometheus指标（规划中）
- **追踪**: OpenTelemetry（规划中）
- **健康检查**: `/health` 端点

---

## 总结

### 技术特点

1. **高性能**: 
   - 数据库优先策略，减少外部API调用
   - Redis缓存热点数据
   - 并发处理交易数据

2. **可扩展性**:
   - 微服务架构，易于水平扩展
   - 支持多数据库
   - 对象存储分离大字段

3. **可靠性**:
   - 自动区块同步和缺失检测
   - 数据归档机制
   - 异常恢复中间件

4. **开发友好**:
   - Protobuf类型安全
   - Swagger API文档
   - 完整的错误处理

### 未来规划

- ✅ 核心功能已实现
- ⏳ API管理功能开发中
- 📋 性能优化和监控完善
- 📋 更多区块链网络支持

---

**文档结束**

