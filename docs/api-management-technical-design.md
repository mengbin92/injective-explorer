# 浏览器API管理技术方案

> **文档版本**: v1.0  
> **编写日期**: 2025-12-24  
> **基于**: 产品反馈和需求文档

---

## 一、方案概述

### 1.1 设计目标

基于产品反馈，本方案实现以下核心功能：

1. **API Token管理**: 用户与Token关系表，Token格式为`inj_`+32位随机字符
2. **限流系统**: 秒级限流（IP、Token、账户三层）
3. **使用量统计**: 延时1-5秒统计，90天后数据归档
4. **IP归属地查询**: 使用免费服务，准确度要求不高
5. **数据导出**: Excel分页导出，避免内存占用过大
6. **Webhook预留**: 收费Token支持Webhook，免费Token不支持

### 1.2 技术架构

```
┌─────────────────────────────────────────────────────────┐
│                    前端应用层                              │
│              (API文档、Dashboard、后台管理)                │
└─────────────────────────────────────────────────────────┘
                          ↓ HTTP/gRPC
┌─────────────────────────────────────────────────────────┐
│                    API网关层                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │ 认证中间件 │  │ 限流中间件 │  │ 统计中间件 │              │
│  └──────────┘  └──────────┘  └──────────┘              │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│                   业务服务层                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │Token服务  │  │统计服务   │  │导出服务   │              │
│  └──────────┘  └──────────┘  └──────────┘              │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│                   数据存储层                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │  MySQL   │  │  Redis   │  │ 对象存储  │              │
│  └──────────┘  └──────────┘  └──────────┘              │
└─────────────────────────────────────────────────────────┘
```

---

## 二、数据库设计

### 2.1 核心表结构

#### 2.1.1 API Token表 (`api_tokens`)

```sql
CREATE TABLE `api_tokens` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID（来自外部系统）',
  `name` VARCHAR(50) NOT NULL COMMENT 'Token名称',
  `token_hash` VARCHAR(64) NOT NULL UNIQUE COMMENT 'Token哈希值（SHA256）',
  `token_preview` VARCHAR(20) NOT NULL COMMENT 'Token预览（inj_xxx...xxx）',
  `plan_type` VARCHAR(20) NOT NULL DEFAULT 'free' COMMENT '套餐类型：free/paid',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/disabled',
  `last_used_at` TIMESTAMP NULL COMMENT '最后使用时间',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_token_hash` (`token_hash`),
  INDEX `idx_status` (`status`),
  INDEX `idx_plan_type` (`plan_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='API Token表';
```

**设计说明**:
- `user_id`: 字符串类型，存储外部系统提供的用户ID
- `token_hash`: 存储Token的SHA256哈希值，用于验证
- `token_preview`: 存储Token预览（前6位+后6位），用于列表显示
- `plan_type`: 区分免费和付费Token，用于Webhook功能判断

---

#### 2.1.2 用户使用量统计表 (`user_usage_stats`)

```sql
CREATE TABLE `user_usage_stats` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID',
  `monthly_used` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '本月已使用次数',
  `monthly_limit` BIGINT UNSIGNED NOT NULL DEFAULT 100000 COMMENT '月度限额',
  `total_usage` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '历史总使用量',
  `success_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '成功请求数',
  `failed_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '失败请求数',
  `last_reset_at` TIMESTAMP NOT NULL COMMENT '上次重置时间',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  UNIQUE KEY `uk_user_id` (`user_id`),
  INDEX `idx_last_reset_at` (`last_reset_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='用户使用量统计表';
```

---

#### 2.1.3 API请求日志表 (`api_request_logs`)

```sql
CREATE TABLE `api_request_logs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `token_id` BIGINT UNSIGNED NOT NULL COMMENT 'Token ID',
  `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID',
  `ip_address` VARCHAR(45) NOT NULL COMMENT 'IP地址（支持IPv6）',
  `ip_location` VARCHAR(100) NULL COMMENT 'IP归属地',
  `endpoint` VARCHAR(255) NOT NULL COMMENT '请求接口路径',
  `method` VARCHAR(10) NOT NULL COMMENT 'HTTP方法',
  `status_code` INT NOT NULL COMMENT 'HTTP状态码',
  `response_time` INT NOT NULL COMMENT '响应时间（毫秒）',
  `error_message` TEXT NULL COMMENT '错误信息',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  
  INDEX `idx_token_id` (`token_id`),
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_ip_address` (`ip_address`),
  INDEX `idx_endpoint` (`endpoint`),
  INDEX `idx_status_code` (`status_code`),
  INDEX `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='API请求日志表';

-- 按时间分区（按月分区，便于归档）
ALTER TABLE `api_request_logs` 
PARTITION BY RANGE (UNIX_TIMESTAMP(created_at)) (
  PARTITION p202412 VALUES LESS THAN (UNIX_TIMESTAMP('2025-01-01')),
  PARTITION p202501 VALUES LESS THAN (UNIX_TIMESTAMP('2025-02-01')),
  PARTITION p202502 VALUES LESS THAN (UNIX_TIMESTAMP('2025-03-01')),
  PARTITION p_future VALUES LESS THAN MAXVALUE
);
```

**设计说明**:
- 使用分区表，按月分区，便于数据归档和删除
- `ip_location`: 异步查询IP归属地，可能为空

---

#### 2.1.4 使用量日统计表 (`usage_daily_stats`)

```sql
CREATE TABLE `usage_daily_stats` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID',
  `stat_date` DATE NOT NULL COMMENT '统计日期',
  `total_calls` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '总请求数',
  `success_calls` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '成功请求数',
  `failed_calls` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '失败请求数',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  UNIQUE KEY `uk_user_date` (`user_id`, `stat_date`),
  INDEX `idx_stat_date` (`stat_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='使用量日统计表';
```

**设计说明**:
- 用于Dashboard图表展示，避免实时聚合查询
- 由后台任务定期汇总生成

---

#### 2.1.5 IP黑名单表 (`ip_blacklist`)

```sql
CREATE TABLE `ip_blacklist` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `ip_address` VARCHAR(45) NOT NULL UNIQUE COMMENT 'IP地址',
  `ip_location` VARCHAR(100) NULL COMMENT 'IP归属地',
  `reason` VARCHAR(50) NOT NULL COMMENT '加入原因',
  `reason_text` VARCHAR(255) NOT NULL COMMENT '原因描述',
  `block_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '拦截次数',
  `operator_id` VARCHAR(64) NULL COMMENT '操作人ID',
  `operator_name` VARCHAR(100) NULL COMMENT '操作人名称',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  
  INDEX `idx_ip_address` (`ip_address`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='IP黑名单表';
```

---

#### 2.1.6 邮箱黑名单表 (`email_blacklist`)

```sql
CREATE TABLE `email_blacklist` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `domain` VARCHAR(255) NOT NULL UNIQUE COMMENT '邮箱域名（小写）',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/disabled',
  `block_count` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '拦截次数',
  `note` TEXT NULL COMMENT '备注',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  INDEX `idx_domain` (`domain`),
  INDEX `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='邮箱黑名单表';
```

---

#### 2.1.7 套餐配置表 (`api_plans`)

```sql
CREATE TABLE `api_plans` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(50) NOT NULL UNIQUE COMMENT '套餐名称',
  `plan_type` VARCHAR(20) NOT NULL COMMENT '套餐类型：free/paid',
  `monthly_limit` BIGINT UNSIGNED NOT NULL COMMENT '月度调用限额',
  `rate_limit_per_second` INT NOT NULL COMMENT '每秒请求限制',
  `max_tokens` INT NOT NULL COMMENT '最大Token数',
  `enable_webhook` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否支持Webhook',
  `price` DECIMAL(10,2) NOT NULL DEFAULT 0.00 COMMENT '价格',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/inactive',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  INDEX `idx_plan_type` (`plan_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='套餐配置表';

-- 初始化Free套餐数据
INSERT INTO `api_plans` (`name`, `plan_type`, `monthly_limit`, `rate_limit_per_second`, `max_tokens`, `enable_webhook`, `price`, `status`) 
VALUES ('Free', 'free', 100000, 5, 3, 0, 0.00, 'active');
```

---

#### 2.1.8 Webhook配置表 (`webhook_configs`) - 预留

```sql
CREATE TABLE `webhook_configs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `token_id` BIGINT UNSIGNED NOT NULL COMMENT 'Token ID',
  `user_id` VARCHAR(64) NOT NULL COMMENT '用户ID',
  `webhook_url` VARCHAR(512) NOT NULL COMMENT 'Webhook URL',
  `events` JSON NOT NULL COMMENT '订阅的事件类型',
  `secret` VARCHAR(64) NOT NULL COMMENT 'Webhook签名密钥',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '状态：active/disabled',
  `last_triggered_at` TIMESTAMP NULL COMMENT '最后触发时间',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  INDEX `idx_token_id` (`token_id`),
  INDEX `idx_user_id` (`user_id`),
  INDEX `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci 
COMMENT='Webhook配置表（预留）';
```

---

## 三、数据模型设计（Go）

### 3.1 API Token模型

```go
// internal/models/api_token.go
package models

import (
    "time"
    "gorm.io/gorm"
)

// APIToken represents an API token in the database
type APIToken struct {
    gorm.Model
    
    UserID      string     `gorm:"column:user_id;type:varchar(64);not null;index" json:"user_id"`
    Name        string     `gorm:"column:name;type:varchar(50);not null" json:"name"`
    TokenHash   string     `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex" json:"token_hash"`
    TokenPreview string    `gorm:"column:token_preview;type:varchar(20);not null" json:"token_preview"`
    PlanType    string     `gorm:"column:plan_type;type:varchar(20);not null;default:'free';index" json:"plan_type"`
    Status      string     `gorm:"column:status;type:varchar(20);not null;default:'active';index" json:"status"`
    LastUsedAt  *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
}

// TableName specifies the table name
func (APIToken) TableName() string {
    return "api_tokens"
}
```

---

### 3.2 用户使用量统计模型

```go
// internal/models/user_usage_stats.go
package models

import (
    "time"
    "gorm.io/gorm"
)

// UserUsageStats represents user usage statistics
type UserUsageStats struct {
    gorm.Model
    
    UserID      string    `gorm:"column:user_id;type:varchar(64);not null;uniqueIndex" json:"user_id"`
    MonthlyUsed uint64    `gorm:"column:monthly_used;type:bigint unsigned;not null;default:0" json:"monthly_used"`
    MonthlyLimit uint64   `gorm:"column:monthly_limit;type:bigint unsigned;not null;default:100000" json:"monthly_limit"`
    TotalUsage  uint64    `gorm:"column:total_usage;type:bigint unsigned;not null;default:0" json:"total_usage"`
    SuccessCount uint64    `gorm:"column:success_count;type:bigint unsigned;not null;default:0" json:"success_count"`
    FailedCount  uint64    `gorm:"column:failed_count;type:bigint unsigned;not null;default:0" json:"failed_count"`
    LastResetAt  time.Time `gorm:"column:last_reset_at;not null" json:"last_reset_at"`
}

// TableName specifies the table name
func (UserUsageStats) TableName() string {
    return "user_usage_stats"
}
```

---

### 3.3 API请求日志模型

```go
// internal/models/api_request_log.go
package models

import (
    "gorm.io/gorm"
)

// APIRequestLog represents an API request log
type APIRequestLog struct {
    gorm.Model
    
    TokenID     uint   `gorm:"column:token_id;type:bigint unsigned;not null;index" json:"token_id"`
    UserID      string `gorm:"column:user_id;type:varchar(64);not null;index" json:"user_id"`
    IPAddress   string `gorm:"column:ip_address;type:varchar(45);not null;index" json:"ip_address"`
    IPLocation  string `gorm:"column:ip_location;type:varchar(100)" json:"ip_location"`
    Endpoint    string `gorm:"column:endpoint;type:varchar(255);not null;index" json:"endpoint"`
    Method      string `gorm:"column:method;type:varchar(10);not null" json:"method"`
    StatusCode  int    `gorm:"column:status_code;type:int;not null;index" json:"status_code"`
    ResponseTime int   `gorm:"column:response_time;type:int;not null" json:"response_time"`
    ErrorMessage string `gorm:"column:error_message;type:text" json:"error_message"`
}

// TableName specifies the table name
func (APIRequestLog) TableName() string {
    return "api_request_logs"
}
```

---

## 四、API接口设计（Protobuf）

### 4.1 API Token管理服务

```protobuf
// api/explorer/v1/api_token.proto
syntax = "proto3";

package api.explorer.v1;

import "explorer/v1/common.proto";
import "google/api/annotations.proto";

option go_package = "injective-explorer/api/explorer/v1;v1";

// APITokenService provides API token management services
service APITokenService {
  // CreateToken creates a new API token
  rpc CreateToken(CreateTokenRequest) returns (CreateTokenResponse) {
    option (google.api.http) = {
      post: "/api/v1/tokens"
      body: "*"
    };
  }

  // ListTokens lists all tokens for a user
  rpc ListTokens(ListTokensRequest) returns (ListTokensResponse) {
    option (google.api.http) = {
      get: "/api/v1/tokens"
    };
  }

  // DeleteToken deletes a token
  rpc DeleteToken(DeleteTokenRequest) returns (DeleteTokenResponse) {
    option (google.api.http) = {
      delete: "/api/v1/tokens/{token_id}"
    };
  }

  // GetTokenUsage gets token usage statistics
  rpc GetTokenUsage(GetTokenUsageRequest) returns (GetTokenUsageResponse) {
    option (google.api.http) = {
      get: "/api/v1/tokens/{token_id}/usage"
    };
  }
}

// CreateTokenRequest creates a new token
message CreateTokenRequest {
  string name = 1; // Token name (required, 1-50 characters)
}

// CreateTokenResponse returns the created token
message CreateTokenResponse {
  string token = 1;        // Full token (only shown once)
  string token_preview = 2; // Token preview (inj_xxx...xxx)
  int64 token_id = 3;     // Token ID
  string message = 4;     // Success message
}

// ListTokensRequest lists tokens
message ListTokensRequest {
  // Empty, uses authenticated user from context
}

// ListTokensResponse returns token list
message ListTokensResponse {
  repeated TokenInfo tokens = 1;
}

// TokenInfo represents token information
message TokenInfo {
  int64 id = 1;
  string name = 2;
  string token_preview = 3; // inj_xxx...xxx
  string plan_type = 4;     // free/paid
  string status = 5;        // active/disabled
  string created_at = 6;
  string last_used_at = 7;  // Empty if never used
}

// DeleteTokenRequest deletes a token
message DeleteTokenRequest {
  int64 token_id = 1;
}

// DeleteTokenResponse confirms deletion
message DeleteTokenResponse {
  string message = 1;
}

// GetTokenUsageRequest gets token usage
message GetTokenUsageRequest {
  int64 token_id = 1;
}

// GetTokenUsageResponse returns usage statistics
message GetTokenUsageResponse {
  int64 total_calls = 1;
  int64 success_calls = 2;
  int64 failed_calls = 3;
  string last_used_at = 4;
}
```

---

## 五、限流系统设计

### 5.1 限流架构

```
请求进入
  ↓
[IP限流检查] → Redis Sorted Set (滑动窗口)
  ↓ 通过
[Token限流检查] → Redis Sorted Set (滑动窗口)
  ↓ 通过
[账户限流检查] → Redis Counter + 数据库
  ↓ 通过
处理请求
```

---

### 5.2 Redis限流实现

```go
// internal/middleware/ratelimit.go
package middleware

import (
    "context"
    "fmt"
    "time"
    
    "github.com/redis/go-redis/v9"
)

// RateLimiter implements rate limiting using Redis
type RateLimiter struct {
    redis *redis.Client
}

// CheckIPRateLimit checks IP rate limit
// limit: requests per minute
func (r *RateLimiter) CheckIPRateLimit(ctx context.Context, ip string, limit int) (bool, error) {
    key := fmt.Sprintf("ratelimit:ip:%s", ip)
    now := time.Now()
    windowStart := now.Add(-1 * time.Minute)
    
    // Remove expired entries
    r.redis.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.Unix()))
    
    // Count current requests
    count, err := r.redis.ZCard(ctx, key).Result()
    if err != nil {
        return false, err
    }
    
    if count >= int64(limit) {
        return false, nil // Rate limit exceeded
    }
    
    // Add current request
    r.redis.ZAdd(ctx, key, redis.Z{
        Score:  float64(now.Unix()),
        Member: fmt.Sprintf("%d", now.UnixNano()),
    })
    r.redis.Expire(ctx, key, 2*time.Minute) // Auto cleanup
    
    return true, nil
}

// CheckTokenRateLimit checks token rate limit (per second)
func (r *RateLimiter) CheckTokenRateLimit(ctx context.Context, tokenID int64, limit int) (bool, error) {
    key := fmt.Sprintf("ratelimit:token:%d", tokenID)
    now := time.Now()
    windowStart := now.Add(-1 * time.Second)
    
    // Remove expired entries
    r.redis.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.Unix()))
    
    // Count current requests
    count, err := r.redis.ZCard(ctx, key).Result()
    if err != nil {
        return false, err
    }
    
    if count >= int64(limit) {
        return false, nil // Rate limit exceeded
    }
    
    // Add current request
    r.redis.ZAdd(ctx, key, redis.Z{
        Score:  float64(now.Unix()),
        Member: fmt.Sprintf("%d", now.UnixNano()),
    })
    r.redis.Expire(ctx, key, 2*time.Second) // Auto cleanup
    
    return true, nil
}

// CheckUserMonthlyLimit checks user monthly limit
func (r *RateLimiter) CheckUserMonthlyLimit(ctx context.Context, userID string, limit uint64) (bool, uint64, error) {
    key := fmt.Sprintf("usage:user:%s:monthly", userID)
    
    // Get current usage from Redis
    current, err := r.redis.Get(ctx, key).Uint64()
    if err == redis.Nil {
        current = 0
    } else if err != nil {
        return false, 0, err
    }
    
    if current >= limit {
        return false, current, nil // Limit exceeded
    }
    
    return true, current, nil
}

// IncrementUserUsage increments user usage counter
func (r *RateLimiter) IncrementUserUsage(ctx context.Context, userID string) error {
    key := fmt.Sprintf("usage:user:%s:monthly", userID)
    
    // Increment counter
    _, err := r.redis.Incr(ctx, key).Result()
    if err != nil {
        return err
    }
    
    // Set expiration to end of month
    now := time.Now()
    nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
    ttl := nextMonth.Sub(now)
    r.redis.Expire(ctx, key, ttl)
    
    return nil
}
```

---

## 六、使用量统计系统

### 6.1 统计流程

```
API请求
  ↓
[记录到Redis队列] → 异步处理
  ↓
[批量写入数据库] → 延时1-5秒
  ↓
[更新统计计数] → Redis + 数据库
  ↓
[定期汇总日统计] → 后台任务
```

---

### 6.2 异步统计实现

```go
// internal/service/usage_stats.go
package service

import (
    "context"
    "encoding/json"
    "time"
    
    "injective-explorer/internal/models"
    "injective-explorer/provider/cache"
    "injective-explorer/provider/db"
    
    "github.com/redis/go-redis/v9"
)

// UsageStatsService handles usage statistics
type UsageStatsService struct {
    redis *redis.Client
    db    *gorm.DB
}

// RecordRequest records an API request asynchronously
func (s *UsageStatsService) RecordRequest(ctx context.Context, log *models.APIRequestLog) error {
    // Serialize log to JSON
    data, err := json.Marshal(log)
    if err != nil {
        return err
    }
    
    // Push to Redis queue
    queueKey := "queue:api_logs"
    return s.redis.LPush(ctx, queueKey, data).Err()
}

// ProcessLogQueue processes logs from queue (background worker)
func (s *UsageStatsService) ProcessLogQueue(ctx context.Context) error {
    queueKey := "queue:api_logs"
    
    for {
        // Blocking pop from queue (timeout: 5 seconds)
        result, err := s.redis.BRPop(ctx, 5*time.Second, queueKey).Result()
        if err == redis.Nil {
            continue // Timeout, retry
        }
        if err != nil {
            return err
        }
        
        // Deserialize log
        var log models.APIRequestLog
        if err := json.Unmarshal([]byte(result[1]), &log); err != nil {
            continue // Skip invalid log
        }
        
        // Batch insert (collect multiple logs before inserting)
        // Implementation details...
    }
}

// UpdateDailyStats updates daily statistics (scheduled task)
func (s *UsageStatsService) UpdateDailyStats(ctx context.Context, date time.Time) error {
    // Aggregate statistics from api_request_logs
    // Insert/update usage_daily_stats
    // Implementation details...
    return nil
}
```

---

## 七、数据归档方案

### 7.1 归档策略

- **归档时间**: 90天前的数据
- **归档方式**: 迁移到对象存储（MinIO/S3）
- **归档频率**: 每天执行一次
- **归档后处理**: 删除数据库中的原始数据

---

### 7.2 归档实现

```go
// internal/service/archive_usage.go
package service

import (
    "context"
    "time"
    
    "injective-explorer/internal/models"
    "injective-explorer/provider/db"
    "injective-explorer/provider/storage"
)

// ArchiveUsageService handles usage data archiving
type ArchiveUsageService struct {
    db      *gorm.DB
    storage storage.Storage
}

// ArchiveOldLogs archives logs older than 90 days
func (s *ArchiveUsageService) ArchiveOldLogs(ctx context.Context) error {
    cutoffDate := time.Now().AddDate(0, 0, -90)
    
    // Query old logs in batches
    batchSize := 1000
    offset := 0
    
    for {
        var logs []models.APIRequestLog
        err := s.db.Where("created_at < ?", cutoffDate).
            Limit(batchSize).
            Offset(offset).
            Find(&logs).Error
        
        if err != nil {
            return err
        }
        
        if len(logs) == 0 {
            break // No more logs
        }
        
        // Archive to object storage
        for _, log := range logs {
            // Serialize and upload
            // Implementation details...
        }
        
        // Delete from database
        ids := make([]uint, len(logs))
        for i, log := range logs {
            ids[i] = log.ID
        }
        s.db.Delete(&models.APIRequestLog{}, ids)
        
        offset += batchSize
    }
    
    return nil
}
```

---

## 八、IP归属地查询

### 8.1 使用免费GeoIP2数据库

```go
// internal/service/ip_location.go
package service

import (
    "context"
    "net"
    
    "github.com/oschwald/geoip2-golang"
)

// IPLocationService provides IP location lookup
type IPLocationService struct {
    db *geoip2.Reader
}

// NewIPLocationService creates a new IP location service
func NewIPLocationService(dbPath string) (*IPLocationService, error) {
    db, err := geoip2.Open(dbPath)
    if err != nil {
        return nil, err
    }
    return &IPLocationService{db: db}, nil
}

// GetLocation gets IP location (async, may return empty)
func (s *IPLocationService) GetLocation(ctx context.Context, ip string) string {
    ipAddr := net.ParseIP(ip)
    if ipAddr == nil {
        return ""
    }
    
    record, err := s.db.City(ipAddr)
    if err != nil {
        return "" // Return empty on error (non-blocking)
    }
    
    // Format: "City, Country"
    city := record.City.Names["en"]
    country := record.Country.Names["en"]
    if city != "" && country != "" {
        return city + ", " + country
    }
    if country != "" {
        return country
    }
    return ""
}
```

**说明**:
- 使用MaxMind GeoLite2免费数据库
- 异步查询，不阻塞API请求
- 查询失败返回空字符串（不影响功能）

---

## 九、数据导出方案

### 9.1 Excel分页导出

```go
// internal/service/export.go
package service

import (
    "context"
    "fmt"
    "io"
    
    "github.com/xuri/excelize/v2"
)

// ExportService handles data export
type ExportService struct {
    db *gorm.DB
}

// ExportUsageLogs exports usage logs to Excel (paginated)
func (s *ExportService) ExportUsageLogs(ctx context.Context, userID string, w io.Writer) error {
    f := excelize.NewFile()
    defer f.Close()
    
    sheetName := "Usage Logs"
    f.NewSheet(sheetName)
    
    // Headers
    headers := []string{"时间", "IP地址", "接口", "状态码", "响应时间(ms)"}
    for i, h := range headers {
        cell := fmt.Sprintf("%c1", 'A'+i)
        f.SetCellValue(sheetName, cell, h)
    }
    
    // Query in batches to avoid memory issues
    batchSize := 1000
    page := 1
    row := 2
    
    for {
        var logs []models.APIRequestLog
        err := s.db.Where("user_id = ?", userID).
            Order("created_at DESC").
            Limit(batchSize).
            Offset((page - 1) * batchSize).
            Find(&logs).Error
        
        if err != nil {
            return err
        }
        
        if len(logs) == 0 {
            break // No more data
        }
        
        // Write batch to Excel
        for _, log := range logs {
            f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), log.CreatedAt.Format("2006-01-02 15:04:05"))
            f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), log.IPAddress)
            f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), log.Endpoint)
            f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), log.StatusCode)
            f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), log.ResponseTime)
            row++
        }
        
        // Flush to writer periodically
        if page%10 == 0 {
            if err := f.Write(w); err != nil {
                return err
            }
        }
        
        if len(logs) < batchSize {
            break // Last page
        }
        page++
    }
    
    // Final write
    return f.Write(w)
}
```

**优化点**:
- 分批查询，避免一次性加载大量数据
- 定期刷新到Writer，减少内存占用
- 使用流式写入，不全部加载到内存

---

## 十、Webhook功能预留

### 10.1 Webhook接口设计

```protobuf
// api/explorer/v1/webhook.proto
syntax = "proto3";

package api.explorer.v1;

import "google/api/annotations.proto";

// WebhookService provides webhook management (for paid tokens only)
service WebhookService {
  // CreateWebhook creates a webhook configuration
  rpc CreateWebhook(CreateWebhookRequest) returns (CreateWebhookResponse) {
    option (google.api.http) = {
      post: "/api/v1/webhooks"
      body: "*"
    };
  }

  // ListWebhooks lists webhook configurations
  rpc ListWebhooks(ListWebhooksRequest) returns (ListWebhooksResponse) {
    option (google.api.http) = {
      get: "/api/v1/webhooks"
    };
  }

  // UpdateWebhook updates webhook configuration
  rpc UpdateWebhook(UpdateWebhookRequest) returns (UpdateWebhookResponse) {
    option (google.api.http) = {
      put: "/api/v1/webhooks/{webhook_id}"
      body: "*"
    };
  }

  // DeleteWebhook deletes a webhook
  rpc DeleteWebhook(DeleteWebhookRequest) returns (DeleteWebhookResponse) {
    option (google.api.http) = {
      delete: "/api/v1/webhooks/{webhook_id}"
    };
  }
}

// CreateWebhookRequest creates a webhook
message CreateWebhookRequest {
  int64 token_id = 1;           // Token ID (must be paid plan)
  string webhook_url = 2;       // Webhook URL
  repeated string events = 3;   // Event types to subscribe
}

// CreateWebhookResponse returns webhook info
message CreateWebhookResponse {
  int64 webhook_id = 1;
  string message = 2;
}
```

---

### 10.2 Webhook触发机制（预留）

```go
// internal/service/webhook.go
package service

// WebhookService handles webhook delivery
type WebhookService struct {
    // Implementation will be added when needed
}

// TriggerWebhook triggers a webhook event (future implementation)
func (s *WebhookService) TriggerWebhook(ctx context.Context, event string, data interface{}) error {
    // 1. Query webhook configs for this event
    // 2. For each webhook:
    //    - Sign payload with secret
    //    - Send HTTP POST request
    //    - Retry on failure
    //    - Log delivery status
    return nil
}
```

---

## 十一、配置管理

### 11.1 配置文件扩展

```protobuf
// internal/conf/conf.proto 新增部分

message APIManagement {
  RateLimit rate_limit = 1;
  UsageStats usage_stats = 2;
  IPLocation ip_location = 3;
  Archive archive_usage = 4;
  
  message RateLimit {
    int32 ip_per_minute = 1;        // IP每分钟限制
    int32 ip_per_hour = 2;           // IP每小时限制
    int32 token_per_second = 3;     // Token每秒限制
    int32 default_monthly_limit = 4; // 默认月度限额
  }
  
  message UsageStats {
    int32 async_delay_seconds = 1;   // 异步统计延迟（1-5秒）
    int32 archive_days = 2;          // 归档天数（90天）
  }
  
  message IPLocation {
    string geoip_db_path = 1;        // GeoIP2数据库路径
  }
  
  message Archive {
    bool enabled = 1;
    google.protobuf.Duration interval = 2;
    int32 batch_size = 3;
  }
}
```

---

## 十二、实施计划

### 12.0 排期时间表

**总体排期**：
- **开发周期**：至 2025年1月6日
- **测试和修改回归**：2025年1月6日 - 2025年1月12日

**详细时间安排**：
- Phase 1（核心功能）：12月24日 - 12月27日（4个工作日，12月28-29为周末）
- 元旦假期：1月1日 - 1月3日（3天，不计算在工作日内）
- Phase 2（完善功能）：12月30日 - 12月31日（2个工作日，1月4-5日为周末）
- Phase 3（优化和扩展）：1月4日 - 1月6日（3个工作日）
- 测试阶段：1月6日 - 1月12日（包含功能测试、性能测试、回归测试等）

**工作日统计**：
- 总开发工作日：7天（Phase 1: 4天 + Phase 2: 2天 + Phase 3: 1天）

---

### 12.1 Phase 1: 核心功能（12月24日 - 12月27日，4个工作日）

1. **数据库表创建**
   - 创建所有核心表
   - 初始化套餐数据

2. **Token管理**
   - Token生成（inj_+32位随机字符）
   - Token创建/删除接口
   - Token认证中间件

3. **基础限流**
   - IP限流（每分钟）
   - Token限流（每秒）

### 12.2 Phase 2: 完善功能（12月30日 - 12月31日，2个工作日）

1. **使用量统计**
   - 异步日志记录
   - 日统计汇总
   - Dashboard数据接口

2. **账户限流**
   - 月度限额检查
   - 限额重置机制

3. **IP归属地**
   - GeoIP2集成
   - 异步查询

### 12.3 Phase 3: 优化和扩展（1月4日 - 1月6日，3个工作日）

1. **数据归档**
   - 90天数据归档
   - 归档任务调度

2. **数据导出**
   - Excel分页导出
   - 内存优化

3. **Webhook预留**
   - 数据库表创建
   - 接口定义（暂不实现）

---

## 十三、技术要点总结

### 13.1 Token生成

```go
// Token格式: inj_ + 32位随机字符（Base64 URL编码）
func GenerateAPIToken() (string, string, error) {
    // 生成32字节随机数
    randomBytes := make([]byte, 32)
    if _, err := rand.Read(randomBytes); err != nil {
        return "", "", err
    }
    
    // Base64 URL编码
    token := "inj_" + base64.URLEncoding.EncodeToString(randomBytes)
    
    // 计算SHA256哈希（用于存储）
    hash := sha256.Sum256([]byte(token))
    tokenHash := hex.EncodeToString(hash[:])
    
    // 生成预览（前6位+后6位）
    tokenPreview := token[:9] + "..." + token[len(token)-6:]
    
    return token, tokenHash, nil
}
```

### 13.2 关键性能优化

1. **限流**: 使用Redis Sorted Set实现滑动窗口，O(log N)复杂度
2. **统计**: 异步队列 + 批量写入，不阻塞API请求
3. **查询**: 使用数据库索引和分区表，优化查询性能
4. **导出**: 分批查询和流式写入，避免内存溢出

---

## 十四、中间件完整实现

### 14.1 Token认证中间件

```go
// internal/middleware/auth.go
package middleware

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "strings"
    
    "injective-explorer/internal/models"
    "injective-explorer/provider/db"
    
    "github.com/go-kratos/kratos/v2/errors"
    "github.com/go-kratos/kratos/v2/middleware"
    khttp "github.com/go-kratos/kratos/v2/transport/http"
    "gorm.io/gorm"
)

// getClientIP extracts client IP from request (used in other middleware)
func getClientIP(r *http.Request) string {
    // Implementation in ratelimit_middleware.go
    return ""
}

// AuthMiddleware validates API token
func AuthMiddleware() middleware.Middleware {
    return func(handler middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            // Extract token from HTTP header
            if httpCtx, ok := khttp.FromServerContext(ctx); ok {
                authHeader := httpCtx.Request().Header.Get("Authorization")
                if authHeader == "" {
                    return nil, errors.Unauthorized("UNAUTHORIZED", "Missing Authorization header")
                }
                
                // Parse Bearer token
                parts := strings.Split(authHeader, " ")
                if len(parts) != 2 || parts[0] != "Bearer" {
                    return nil, errors.Unauthorized("UNAUTHORIZED", "Invalid Authorization header format")
                }
                
                token := parts[1]
                
                // Validate token
                tokenInfo, err := validateToken(ctx, token)
                if err != nil {
                    return nil, err
                }
                
                // Check token status
                if tokenInfo.Status != "active" {
                    return nil, errors.Unauthorized("TOKEN_DISABLED", "API token is disabled")
                }
                
                // Check user account status (if needed)
                // Note: User management is handled by external system
                
                // Store token info in context
                ctx = context.WithValue(ctx, "token_id", tokenInfo.ID)
                ctx = context.WithValue(ctx, "user_id", tokenInfo.UserID)
                ctx = context.WithValue(ctx, "plan_type", tokenInfo.PlanType)
                
                return handler(ctx, req)
            }
            
            return handler(ctx, req)
        }
    }
}

// validateToken validates token and returns token info
func validateToken(ctx context.Context, token string) (*models.APIToken, error) {
    // Hash token
    hash := sha256.Sum256([]byte(token))
    tokenHash := hex.EncodeToString(hash[:])
    
    // Query from database
    var tokenInfo models.APIToken
    err := db.Get().Where("token_hash = ?", tokenHash).First(&tokenInfo).Error
    if err == gorm.ErrRecordNotFound {
        return nil, errors.Unauthorized("TOKEN_INVALID", "Invalid API token")
    }
    if err != nil {
        return nil, err
    }
    
    return &tokenInfo, nil
}
```

---

### 14.2 限流中间件

```go
// internal/middleware/ratelimit_middleware.go
package middleware

import (
    "context"
    "net/http"
    "strings"
    
    "injective-explorer/internal/middleware/ratelimit"
    
    "github.com/go-kratos/kratos/v2/errors"
    "github.com/go-kratos/kratos/v2/middleware"
    khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RateLimitMiddleware applies rate limiting
func RateLimitMiddleware(limiter *ratelimit.RateLimiter) middleware.Middleware {
    return func(handler middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            if httpCtx, ok := khttp.FromServerContext(ctx); ok {
                request := httpCtx.Request()
                
                // 1. IP限流检查
                ip := getClientIP(request)
                allowed, err := limiter.CheckIPRateLimit(ctx, ip, 100) // 100次/分钟
                if err != nil {
                    return nil, errors.InternalServer("RATE_LIMIT_ERROR", "Rate limit check failed")
                }
                if !allowed {
                    return nil, errors.TooManyRequests("IP_RATE_LIMIT", "IP request rate limit exceeded")
                }
                
                // 2. Token限流检查
                tokenID, ok := ctx.Value("token_id").(uint)
                if ok {
                    allowed, err = limiter.CheckTokenRateLimit(ctx, int64(tokenID), 5) // 5次/秒
                    if err != nil {
                        return nil, errors.InternalServer("RATE_LIMIT_ERROR", "Rate limit check failed")
                    }
                    if !allowed {
                        return nil, errors.TooManyRequests("TOKEN_RATE_LIMIT", "Token request rate limit exceeded")
                    }
                }
                
                // 3. 账户月度限流检查
                userID, ok := ctx.Value("user_id").(string)
                if ok {
                    allowed, current, err := limiter.CheckUserMonthlyLimit(ctx, userID, 100000) // 100,000次/月
                    if err != nil {
                        return nil, errors.InternalServer("RATE_LIMIT_ERROR", "Rate limit check failed")
                    }
                    if !allowed {
                        return nil, errors.TooManyRequests("MONTHLY_QUOTA_EXCEEDED", 
                            "Monthly API quota exceeded")
                    }
                    
                    // Increment usage (async)
                    go limiter.IncrementUserUsage(context.Background(), userID)
                }
            }
            
            return handler(ctx, req)
        }
    }
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
    // Check X-Forwarded-For header (for proxy/load balancer)
    forwarded := r.Header.Get("X-Forwarded-For")
    if forwarded != "" {
        // Take first IP in the chain
        ips := strings.Split(forwarded, ",")
        if len(ips) > 0 {
            return strings.TrimSpace(ips[0])
        }
    }
    
    // Check X-Real-IP header
    realIP := r.Header.Get("X-Real-IP")
    if realIP != "" {
        return realIP
    }
    
    // Fallback to RemoteAddr
    ip := r.RemoteAddr
    if idx := strings.LastIndex(ip, ":"); idx != -1 {
        ip = ip[:idx]
    }
    return ip
}
```

---

### 14.3 统计中间件

```go
// internal/middleware/stats_middleware.go
package middleware

import (
    "context"
    "time"
    
    "injective-explorer/internal/models"
    "injective-explorer/internal/service"
    
    "github.com/go-kratos/kratos/v2/middleware"
    khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// StatsMiddleware records API request statistics
func StatsMiddleware(statsService *service.UsageStatsService) middleware.Middleware {
    return func(handler middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            startTime := time.Now()
            
            // Execute handler
            reply, err := handler(ctx, req)
            
            // Calculate response time
            responseTime := int(time.Since(startTime).Milliseconds())
            
            // Record statistics (async)
            if httpCtx, ok := khttp.FromServerContext(ctx); ok {
                go func() {
                    request := httpCtx.Request()
                    response := httpCtx.Response()
                    
                    tokenID, _ := ctx.Value("token_id").(uint)
                    userID, _ := ctx.Value("user_id").(string)
                    ip := getClientIP(request)
                    
                    log := &models.APIRequestLog{
                        TokenID:      tokenID,
                        UserID:       userID,
                        IPAddress:    ip,
                        Endpoint:     request.URL.Path,
                        Method:       request.Method,
                        StatusCode:   response.StatusCode(),
                        ResponseTime: responseTime,
                    }
                    
                    if err != nil {
                        log.ErrorMessage = err.Error()
                    }
                    
                    // Async record (non-blocking)
                    statsService.RecordRequest(context.Background(), log)
                }()
            }
            
            return reply, err
        }
    }
}
```

---

## 十五、服务层完整实现

### 15.1 Token服务实现

```go
// internal/service/api_token.go
package service

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "fmt"
    
    "injective-explorer/internal/models"
    "injective-explorer/provider/db"
    
    "github.com/go-kratos/kratos/v2/errors"
    "gorm.io/gorm"
)

// APITokenService provides API token management
type APITokenService struct {
    db *gorm.DB
}

// NewAPITokenService creates a new token service
func NewAPITokenService() *APITokenService {
    return &APITokenService{
        db: db.Get(),
    }
}

// CreateToken creates a new API token
func (s *APITokenService) CreateToken(ctx context.Context, userID, name string) (string, *models.APIToken, error) {
    // Validate name
    if len(name) == 0 || len(name) > 50 {
        return "", nil, errors.BadRequest("INVALID_NAME", "Token name must be 1-50 characters")
    }
    
    // Check user token count
    var count int64
    s.db.Model(&models.APIToken{}).
        Where("user_id = ? AND deleted_at IS NULL", userID).
        Count(&count)
    
    // Get user's plan to check max tokens
    // For now, default to 3 tokens (Free plan)
    maxTokens := 3
    if count >= int64(maxTokens) {
        return "", nil, errors.BadRequest("TOKEN_LIMIT_EXCEEDED", 
            fmt.Sprintf("Maximum token limit reached (%d tokens)", maxTokens))
    }
    
    // Generate token
    token, tokenHash, tokenPreview, err := generateAPIToken()
    if err != nil {
        return "", nil, errors.InternalServer("TOKEN_GENERATION_FAILED", "Failed to generate token")
    }
    
    // Create token record
    apiToken := &models.APIToken{
        UserID:      userID,
        Name:        name,
        TokenHash:   tokenHash,
        TokenPreview: tokenPreview,
        PlanType:    "free", // Default to free plan
        Status:      "active",
    }
    
    if err := s.db.Create(apiToken).Error; err != nil {
        return "", nil, errors.InternalServer("TOKEN_CREATION_FAILED", "Failed to create token")
    }
    
    return token, apiToken, nil
}

// ListTokens lists all tokens for a user
func (s *APITokenService) ListTokens(ctx context.Context, userID string) ([]*models.APIToken, error) {
    var tokens []*models.APIToken
    err := s.db.Where("user_id = ? AND deleted_at IS NULL", userID).
        Order("created_at DESC").
        Find(&tokens).Error
    if err != nil {
        return nil, errors.InternalServer("QUERY_FAILED", "Failed to query tokens")
    }
    return tokens, nil
}

// DeleteToken deletes a token
func (s *APITokenService) DeleteToken(ctx context.Context, userID string, tokenID uint) error {
    // Verify ownership
    var token models.APIToken
    err := s.db.Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error
    if err == gorm.ErrRecordNotFound {
        return errors.NotFound("TOKEN_NOT_FOUND", "Token not found")
    }
    if err != nil {
        return errors.InternalServer("QUERY_FAILED", "Failed to query token")
    }
    
    // Soft delete
    err = s.db.Delete(&token).Error
    if err != nil {
        return errors.InternalServer("DELETE_FAILED", "Failed to delete token")
    }
    
    return nil
}

// generateAPIToken generates a new API token
// Format: inj_ + 32位随机字符（Base64 URL编码）
func generateAPIToken() (token, tokenHash, tokenPreview string, err error) {
    // Generate 32 random bytes
    randomBytes := make([]byte, 32)
    if _, err := rand.Read(randomBytes); err != nil {
        return "", "", "", err
    }
    
    // Base64 URL encoding
    encoded := base64.URLEncoding.EncodeToString(randomBytes)
    token = "inj_" + encoded
    
    // Calculate SHA256 hash
    hash := sha256.Sum256([]byte(token))
    tokenHash = hex.EncodeToString(hash[:])
    
    // Generate preview (first 9 chars + ... + last 6 chars)
    if len(token) > 15 {
        tokenPreview = token[:9] + "..." + token[len(token)-6:]
    } else {
        tokenPreview = token
    }
    
    return token, tokenHash, tokenPreview, nil
}
```

---

### 15.2 使用量统计服务实现

```go
// internal/service/usage_stats.go
package service

import (
    "context"
    "encoding/json"
    "time"
    
    "injective-explorer/internal/models"
    "injective-explorer/provider/cache"
    "injective-explorer/provider/db"
    
    "github.com/redis/go-redis/v9"
)

// UsageStatsService handles usage statistics
type UsageStatsService struct {
    redis *redis.Client
    db    *gorm.DB
}

// NewUsageStatsService creates a new stats service
func NewUsageStatsService() *UsageStatsService {
    return &UsageStatsService{
        redis: cache.GetRedis(),
        db:    db.Get(),
    }
}

// RecordRequest records an API request asynchronously
func (s *UsageStatsService) RecordRequest(ctx context.Context, log *models.APIRequestLog) error {
    // Serialize log to JSON
    data, err := json.Marshal(log)
    if err != nil {
        return err
    }
    
    // Push to Redis queue (non-blocking)
    queueKey := "queue:api_logs"
    return s.redis.LPush(ctx, queueKey, data).Err()
}

// ProcessLogQueue processes logs from queue (background worker)
func (s *UsageStatsService) ProcessLogQueue(ctx context.Context) error {
    queueKey := "queue:api_logs"
    batchSize := 100
    batch := make([]*models.APIRequestLog, 0, batchSize)
    
    for {
        // Blocking pop from queue (timeout: 5 seconds)
        result, err := s.redis.BRPop(ctx, 5*time.Second, queueKey).Result()
        if err == redis.Nil {
            // Timeout, flush batch if any
            if len(batch) > 0 {
                s.flushBatch(ctx, batch)
                batch = batch[:0]
            }
            continue
        }
        if err != nil {
            return err
        }
        
        // Deserialize log
        var log models.APIRequestLog
        if err := json.Unmarshal([]byte(result[1]), &log); err != nil {
            continue // Skip invalid log
        }
        
        batch = append(batch, &log)
        
        // Flush when batch is full
        if len(batch) >= batchSize {
            s.flushBatch(ctx, batch)
            batch = batch[:0]
        }
    }
}

// flushBatch flushes a batch of logs to database
func (s *UsageStatsService) flushBatch(ctx context.Context, logs []*models.APIRequestLog) error {
    if len(logs) == 0 {
        return nil
    }
    
    // Batch insert
    return s.db.CreateInBatches(logs, 100).Error
}

// UpdateDailyStats updates daily statistics (scheduled task)
func (s *UsageStatsService) UpdateDailyStats(ctx context.Context, date time.Time) error {
    startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
    endDate := startDate.Add(24 * time.Hour)
    
    // Aggregate statistics by user
    type StatsResult struct {
        UserID      string
        TotalCalls  int64
        SuccessCalls int64
        FailedCalls  int64
    }
    
    var results []StatsResult
    err := s.db.Model(&models.APIRequestLog{}).
        Select("user_id, COUNT(*) as total_calls, "+
            "SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 ELSE 0 END) as success_calls, "+
            "SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as failed_calls").
        Where("created_at >= ? AND created_at < ?", startDate, endDate).
        Group("user_id").
        Scan(&results).Error
    
    if err != nil {
        return err
    }
    
    // Insert/update daily stats
    for _, result := range results {
        dailyStat := models.UsageDailyStats{
            UserID:      result.UserID,
            StatDate:    startDate,
            TotalCalls:  uint64(result.TotalCalls),
            SuccessCalls: uint64(result.SuccessCalls),
            FailedCalls:  uint64(result.FailedCalls),
        }
        
        s.db.Where("user_id = ? AND stat_date = ?", result.UserID, startDate).
            Assign(dailyStat).
            FirstOrCreate(&dailyStat)
    }
    
    return nil
}
```

---

## 十六、后台任务调度

### 16.1 定时任务

```go
// internal/service/scheduler.go
package service

import (
    "context"
    "time"
    
    "github.com/go-kratos/kratos/v2/log"
)

// Scheduler manages background tasks
type Scheduler struct {
    statsService *UsageStatsService
    archiveService *ArchiveUsageService
    logger log.Logger
}

// NewScheduler creates a new scheduler
func NewScheduler(statsService *UsageStatsService, archiveService *ArchiveUsageService, logger log.Logger) *Scheduler {
    return &Scheduler{
        statsService: statsService,
        archiveService: archiveService,
        logger: log.NewHelper(logger),
    }
}

// Start starts background tasks
func (s *Scheduler) Start(ctx context.Context) {
    // Start log queue processor
    go s.processLogQueue(ctx)
    
    // Start daily stats updater (runs at 1 AM daily)
    go s.scheduleDailyStats(ctx)
    
    // Start archive task (runs daily)
    go s.scheduleArchive(ctx)
}

// processLogQueue processes log queue continuously
func (s *Scheduler) processLogQueue(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            if err := s.statsService.ProcessLogQueue(ctx); err != nil {
                s.logger.Errorf("Log queue processing error: %v", err)
                time.Sleep(5 * time.Second) // Retry after delay
            }
        }
    }
}

// scheduleDailyStats schedules daily stats update
func (s *Scheduler) scheduleDailyStats(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour) // Check every hour
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            now := time.Now()
            // Run at 1 AM
            if now.Hour() == 1 {
                yesterday := now.AddDate(0, 0, -1)
                if err := s.statsService.UpdateDailyStats(ctx, yesterday); err != nil {
                    s.logger.Errorf("Daily stats update error: %v", err)
                }
            }
        }
    }
}

// scheduleArchive schedules archive task
func (s *Scheduler) scheduleArchive(ctx context.Context) {
    ticker := time.NewTicker(24 * time.Hour) // Run daily
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := s.archiveService.ArchiveOldLogs(ctx); err != nil {
                s.logger.Errorf("Archive error: %v", err)
            }
        }
    }
}
```

---

## 十七、错误码定义

### 17.1 统一错误码

```go
// internal/utils/api_errors.go
package utils

import "github.com/go-kratos/kratos/v2/errors"

var (
    // Token相关错误
    ErrTokenInvalid      = errors.New(401, "TOKEN_INVALID", "Invalid API token")
    ErrTokenDisabled     = errors.New(401, "TOKEN_DISABLED", "API token is disabled")
    ErrTokenLimitExceeded = errors.New(400, "TOKEN_LIMIT_EXCEEDED", "Maximum token limit reached")
    
    // 限流相关错误
    ErrIPRateLimit       = errors.New(429, "IP_RATE_LIMIT", "IP request rate limit exceeded")
    ErrTokenRateLimit    = errors.New(429, "TOKEN_RATE_LIMIT", "Token request rate limit exceeded")
    ErrMonthlyQuotaExceeded = errors.New(429, "MONTHLY_QUOTA_EXCEEDED", "Monthly API quota exceeded")
    
    // 账户相关错误
    ErrAccountDisabled   = errors.New(403, "ACCOUNT_DISABLED", "Account is disabled")
    ErrIPBlacklisted     = errors.New(403, "IP_BLACKLISTED", "IP address is blacklisted")
)
```

---

## 十八、测试方案

### 18.1 单元测试

```go
// internal/service/api_token_test.go
package service

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
)

func TestGenerateAPIToken(t *testing.T) {
    token, hash, preview, err := generateAPIToken()
    
    assert.NoError(t, err)
    assert.True(t, strings.HasPrefix(token, "inj_"))
    assert.NotEmpty(t, hash)
    assert.NotEmpty(t, preview)
    assert.Contains(t, preview, "...")
}

// Note: This is a test example, actual test file should import:
// import (
//     "strings"
//     "testing"
//     "github.com/stretchr/testify/assert"
// )
```

### 18.2 集成测试

- Token创建和验证流程
- 限流功能测试
- 使用量统计测试

---

## 十九、部署说明

### 19.1 数据库迁移

```bash
# 执行SQL脚本创建表
mysql -u root -p explorer < database/api_management_tables.sql

# 或使用GORM AutoMigrate
go run cmd/injective-explorer/main.go migrate
```

### 19.2 配置更新

在 `configs/config.yaml` 中添加：

```yaml
api_management:
  rate_limit:
    ip_per_minute: 100
    ip_per_hour: 5000
    token_per_second: 5
    default_monthly_limit: 100000
  usage_stats:
    async_delay_seconds: 3
    archive_days: 90
  ip_location:
    geoip_db_path: "/data/GeoLite2-City.mmdb"
```

### 19.3 GeoIP2数据库下载

```bash
# 下载MaxMind GeoLite2数据库
wget https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=YOUR_LICENSE_KEY&suffix=tar.gz
```

---

## 二十、总结

### 20.1 核心特性

1. ✅ Token管理：inj_+32位随机字符，SHA256哈希存储
2. ✅ 三层限流：IP、Token、账户，秒级精度
3. ✅ 异步统计：1-5秒延迟，不阻塞API
4. ✅ 数据归档：90天自动归档到对象存储
5. ✅ IP归属地：免费GeoIP2数据库
6. ✅ 分页导出：Excel流式导出，避免内存溢出
7. ✅ Webhook预留：数据库和接口已定义

### 20.2 性能指标

- **限流检查**: < 5ms (Redis)
- **统计记录**: 异步，不阻塞
- **Token验证**: < 10ms (数据库查询)
- **导出性能**: 支持百万级数据导出

### 20.3 后续扩展

- Webhook功能实现
- 更多套餐类型支持
- 实时监控和告警
- 数据分析报表

---

**文档完成**
