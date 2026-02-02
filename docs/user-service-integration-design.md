# 用户详情服务集成技术方案（优化增强版）

## 1. 概述

### 1.1 背景
当前 `injective-explorer` 项目使用简化的认证机制，通过 `X-User-ID` Header 获取用户ID，但未验证用户状态。需要集成外部用户详情服务 (`/api/v1/client/user/detail`) 来验证用户身份和状态，提升系统安全性和用户信息完整性。

### 1.2 目标
- ✅ 集成用户详情服务接口，支持多环境配置
- ✅ 验证用户状态（status: 0=不可用, 1=正常）
- ✅ 获取用户扩展信息（account, source, action 等）
- ✅ 保持向后兼容性，支持优雅降级
- ✅ 实现高性能缓存机制，减少外部服务调用
- ✅ 添加熔断器，防止级联故障
- ✅ 完善监控和日志，便于问题排查

### 1.3 架构图

```
┌─────────────┐     ┌─────────────────────────────────────────┐
│   客户端     │────▶│      injective-explorer 认证中间件            │
│             │     │  ┌───────────────────────────────────┐ │
└─────────────┘     │  │  UserAuthMiddleware               │ │
                    │  │  - 提取 X-User-ID & Authorization │ │
                    │  │  - 缓存查询 (Redis)              │ │
                    │  │  - 用户状态验证                   │ │
                    │  └───────────────────────────────────┘ │
                    │              │                         │
                    │              ▼                         │
                    │  ┌───────────────────────────────────┐ │
                    │  │  UserServiceClient                 │ │
                    │  │  - HTTP 客户端                     │ │
                    │  │  - 重试机制 (指数退避)              │ │
                    │  │  - 熔断器 (Circuit Breaker)        │ │
                    │  └───────────────────────────────────┘ │
                    └──────────────┬──────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    │                             │
                    ▼                             ▼
            ┌──────────────┐            ┌─────────────────────┐
            │  Redis 缓存   │            │  用户详情服务        │
            │  (5分钟TTL)   │            │  (多环境支持)        │
            │  - 缓存命中    │            │  - 生产环境          │
            │  - 缓存穿透保护│            │  - 预发布环境        │
            └──────────────┘            └─────────────────────┘
```

### 1.4 关键特性

| 特性       | 说明                                | 优先级 |
| ---------- | ----------------------------------- | ------ |
| 多环境支持 | 支持生产/预发布环境配置             | 高     |
| 缓存机制   | Redis 缓存，5分钟 TTL，防止缓存穿透 | 高     |
| 熔断器     | 防止级联故障，自动恢复              | 高     |
| 重试机制   | 指数退避重试，最大3次               | 中     |
| 降级策略   | 服务故障时优雅降级                  | 高     |
| 监控指标   | Prometheus 指标，便于监控           | 中     |
| 错误分类   | 区分网络错误、业务错误、认证错误    | 中     |

---

## 2. 外部接口信息

### 2.1 基本信息
| 项目         | 值                                                                |
| ------------ | ----------------------------------------------------------------- |
| 生产环境     | `https://prv.gapi-adm-rwa.injective.network:7443/api/v1/client/user/detail` |
| 预发布环境   | `http://47.238.214.9:8175/api/v1/client/user/detail`              |
| 请求方法     | GET                                                               |
| 认证方式     | Bearer Token                                                      |
| Content-Type | application/json                                                  |

### 2.2 请求参数
| 参数名        | 位置   | 类型   | 必填 | 说明                                      |
| ------------- | ------ | ------ | ---- | ----------------------------------------- |
| id            | query  | string | 否   | 用户ID，如 "1"                            |
| Authorization | header | string | 是   | Bearer Token 认证，格式: `Bearer {token}` |

### 2.3 响应结构

**成功响应**:
```json
{
  "code": 200,
  "data": {
    "id": 12,
    "account": "24185456",
    "source": "explorer",
    "alternative_id": 85269316,
    "action": "register",
    "status": 1,
    "created_at": "2026-01-29 11:43:20",
    "updated_at": "2026-01-29 11:43:20"
  },
  "message": "success"
}
```

**错误响应**:
```json
{
  "code": 401,
  "data": null,
  "message": "Unauthorized"
}
```

### 2.4 字段说明
| 字段           | 类型        | 说明                                  |
| -------------- | ----------- | ------------------------------------- |
| code           | integer     | 响应码，200=成功，其他=错误           |
| data           | object/null | 用户详情数据，错误时为 null           |
| message        | string      | 响应消息                              |
| id             | integer     | 用户内部ID                            |
| account        | string      | 用户名/账号                           |
| source         | string      | 用户来源，如 "explorer"               |
| alternative_id | integer     | 其他系统ID                            |
| action         | string      | 行为类型: "login" / "register"        |
| status         | integer     | 用户状态: 0=不可用, 1=正常            |
| created_at     | string      | 创建时间，格式: "YYYY-MM-DD HH:mm:ss" |
| updated_at     | string      | 更新时间，格式: "YYYY-MM-DD HH:mm:ss" |

### 2.5 错误码说明
| 错误码 | 说明       | 处理策略             |
| ------ | ---------- | -------------------- |
| 200    | 成功       | 正常处理             |
| 401    | 未授权     | 认证失败，拒绝访问   |
| 404    | 用户不存在 | 认证失败，拒绝访问   |
| 500    | 服务器错误 | 可重试，或降级处理   |
| 503    | 服务不可用 | 触发熔断器，降级处理 |

---

## 3. 实现方案

### 3.1 配置文件扩展

**文件**: `internal/conf/conf.proto`

在 `APIManagement` 消息中添加用户服务配置：

```protobuf
message APIManagement {
  RateLimit rate_limit = 1;
  UsageStats usage_stats = 2;
  IPLocation ip_location = 3;
  ArchiveUsage archive_usage = 4;
  IPWhitelist ip_whitelist = 5;
  bool test_mode = 6;
  UserService user_service = 7;  // 新增：用户服务配置
}

// 用户服务配置
message UserService {
  // 基础URL（支持多环境）
  // 生产环境: "https://prv.gapi-adm-rwa.injective.network:7443"
  // 预发布环境: "http://47.238.214.9:8175"
  string base_url = 1;
  
  // API版本路径，如 "api/v1"
  string api_version = 2;
  
  // 服务间调用密钥（可选，用于额外验证）
  // 从环境变量读取: ${USER_SERVICE_API_KEY}
  string api_key = 3;
  
  // HTTP 请求超时时间（默认: 5秒）
  google.protobuf.Duration timeout = 4;
  
  // 最大重试次数（默认: 3次）
  int32 max_retries = 5;
  
  // 重试配置
  RetryConfig retry = 6;
  
  // 熔断器配置
  CircuitBreakerConfig circuit_breaker = 7;
  
  // 缓存配置
  CacheConfig cache = 8;
  
  // 是否启用用户服务验证（默认: false，向后兼容）
  bool enabled = 9;
  
  // 降级模式：当用户服务不可用时是否允许访问（默认: true）
  bool fallback_enabled = 10;
}

// 重试配置
message RetryConfig {
  // 最大重试次数（默认: 3）
  int32 max_attempts = 1;
  
  // 初始重试延迟（默认: 100ms）
  google.protobuf.Duration initial_delay = 2;
  
  // 最大重试延迟（默认: 2s）
  google.protobuf.Duration max_delay = 3;
  
  // 重试延迟倍数（指数退避，默认: 2.0）
  double backoff_multiplier = 4;
  
  // 可重试的HTTP状态码列表（默认: 500, 502, 503, 504）
  repeated int32 retryable_status_codes = 5;
}

// 熔断器配置
message CircuitBreakerConfig {
  // 是否启用熔断器（默认: true）
  bool enabled = 1;
  
  // 失败阈值：连续失败多少次后打开熔断器（默认: 5）
  int32 failure_threshold = 2;
  
  // 恢复超时：熔断器打开后多久尝试恢复（默认: 30s）
  google.protobuf.Duration recovery_timeout = 3;
  
  // 半开状态最大请求数：半开状态下允许的最大请求数（默认: 3）
  int32 half_open_max_requests = 4;
}

// 缓存配置
message CacheConfig {
  // 缓存TTL（默认: 5分钟）
  google.protobuf.Duration ttl = 1;
  
  // 是否启用缓存（默认: true）
  bool enabled = 2;
  
  // 缓存键前缀（默认: "user:detail:"）
  string key_prefix = 3;
  
  // 防止缓存穿透：缓存空值（用户不存在）的TTL（默认: 1分钟）
  google.protobuf.Duration negative_cache_ttl = 4;
}
```

**配置文件示例** (`configs/config.yaml`):

```yaml
api_management:
  test_mode: false

  user_service:
    # 基础配置
    enabled: true  # 是否启用用户服务验证
    base_url: "${USER_SERVICE_BASE_URL:https://prv.gapi-adm-rwa.injective.network:7443}"  # 支持环境变量
    api_version: "api/v1"
    api_key: "${USER_SERVICE_API_KEY}"  # 从环境变量读取，可选
    
    # 超时和重试
    timeout: 5s
    max_retries: 3
    
    # 重试配置
    retry:
      max_attempts: 3
      initial_delay: 100ms
      max_delay: 2s
      backoff_multiplier: 2.0
      retryable_status_codes:
        - 500
        - 502
        - 503
        - 504
    
    # 熔断器配置
    circuit_breaker:
      enabled: true
      failure_threshold: 5
      recovery_timeout: 30s
      half_open_max_requests: 3
    
    # 缓存配置
    cache:
      enabled: true
      ttl: 5m
      key_prefix: "user:detail:"
      negative_cache_ttl: 1m
    
    # 降级配置
    fallback_enabled: true  # 用户服务故障时允许降级访问
```

---

### 3.2 用户服务客户端

**文件**: `internal/service/user_service_client.go`

```go
package service

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "sync"
    "time"

    "injective-explorer/internal/conf"
    "injective-explorer/internal/global"
    "github.com/bytedance/sonic"
    "github.com/pkg/errors"
)

// UserDetail 用户详情响应
// 对应 /client/user/detail 接口的 data 字段
type UserDetail struct {
    ID             int64  `json:"id"`
    Account        string `json:"account"`
    Source         string `json:"source"`
    AlternativeID  int64  `json:"alternative_id"`
    Action         string `json:"action"`   // login/register
    Status         int    `json:"status"`   // 0-不可用 1-正常
    CreatedAt      string `json:"created_at"`
    UpdatedAt      string `json:"updated_at"`
}

// IsActive 检查用户是否可用
func (u *UserDetail) IsActive() bool {
    return u.Status == 1
}

// CircuitBreakerState 熔断器状态
type CircuitBreakerState int

const (
    CircuitBreakerClosed CircuitBreakerState = iota // 关闭：正常状态
    CircuitBreakerOpen                              // 打开：拒绝请求
    CircuitBreakerHalfOpen                          // 半开：尝试恢复
)

// CircuitBreaker 熔断器实现
type CircuitBreaker struct {
    mu                sync.RWMutex
    state             CircuitBreakerState
    failureCount      int
    lastFailureTime   time.Time
    failureThreshold  int
    recoveryTimeout   time.Duration
    halfOpenRequests  int
    halfOpenMaxReqs   int
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(failureThreshold int, recoveryTimeout time.Duration, halfOpenMaxReqs int) *CircuitBreaker {
    return &CircuitBreaker{
        state:            CircuitBreakerClosed,
        failureThreshold: failureThreshold,
        recoveryTimeout:  recoveryTimeout,
        halfOpenMaxReqs:  halfOpenMaxReqs,
    }
}

// Call 执行请求，根据熔断器状态决定是否允许
func (cb *CircuitBreaker) Call(fn func() error) error {
    cb.mu.RLock()
    state := cb.state
    cb.mu.RUnlock()

    switch state {
    case CircuitBreakerOpen:
        // 检查是否可以进入半开状态
        cb.mu.Lock()
        if time.Since(cb.lastFailureTime) >= cb.recoveryTimeout {
            cb.state = CircuitBreakerHalfOpen
            cb.halfOpenRequests = 0
            global.Logger.Infof("Circuit breaker entering half-open state")
        } else {
            cb.mu.Unlock()
            return errors.New("circuit breaker is open")
        }
        cb.mu.Unlock()
        fallthrough

    case CircuitBreakerHalfOpen:
        cb.mu.Lock()
        if cb.halfOpenRequests >= cb.halfOpenMaxReqs {
            cb.mu.Unlock()
            return errors.New("circuit breaker half-open max requests reached")
        }
        cb.halfOpenRequests++
        cb.mu.Unlock()
    }

    // 执行请求
    err := fn()
    
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if err != nil {
        cb.failureCount++
        cb.lastFailureTime = time.Now()

        // 检查是否需要打开熔断器
        if cb.failureCount >= cb.failureThreshold {
            cb.state = CircuitBreakerOpen
            global.Logger.Warnf("Circuit breaker opened after %d failures", cb.failureCount)
        } else if cb.state == CircuitBreakerHalfOpen {
            // 半开状态下失败，重新打开
            cb.state = CircuitBreakerOpen
            cb.halfOpenRequests = 0
        }
        return err
    }

    // 成功：重置失败计数
    if cb.state == CircuitBreakerHalfOpen {
        // 半开状态下成功，关闭熔断器
        cb.state = CircuitBreakerClosed
        cb.failureCount = 0
        cb.halfOpenRequests = 0
        global.Logger.Infof("Circuit breaker closed after successful request")
    } else {
        cb.failureCount = 0
    }

    return nil
}

// UserServiceClient 用户服务客户端
type UserServiceClient struct {
    baseURL         string
    apiKey          string
    timeout         time.Duration
    maxRetries      int
    httpClient      *http.Client
    circuitBreaker  *CircuitBreaker
    retryConfig     *conf.UserService_RetryConfig
}

// NewUserServiceClient 创建用户服务客户端
func NewUserServiceClient(cfg *conf.UserService) *UserServiceClient {
    timeout := cfg.Timeout.AsDuration()
    if timeout == 0 {
        timeout = 5 * time.Second
    }

    maxRetries := int(cfg.MaxRetries)
    if maxRetries == 0 {
        maxRetries = 3
    }

    client := &UserServiceClient{
        baseURL:    fmt.Sprintf("%s/%s", cfg.BaseUrl, cfg.ApiVersion),
        apiKey:     cfg.ApiKey,
        timeout:    timeout,
        maxRetries: maxRetries,
        httpClient: &http.Client{
            Timeout: timeout,
        },
        retryConfig: cfg.Retry,
    }

    // 初始化熔断器
    if cfg.CircuitBreaker != nil && cfg.CircuitBreaker.Enabled {
        failureThreshold := int(cfg.CircuitBreaker.FailureThreshold)
        if failureThreshold == 0 {
            failureThreshold = 5
        }
        recoveryTimeout := cfg.CircuitBreaker.RecoveryTimeout.AsDuration()
        if recoveryTimeout == 0 {
            recoveryTimeout = 30 * time.Second
        }
        halfOpenMaxReqs := int(cfg.CircuitBreaker.HalfOpenMaxRequests)
        if halfOpenMaxReqs == 0 {
            halfOpenMaxReqs = 3
        }
        client.circuitBreaker = NewCircuitBreaker(failureThreshold, recoveryTimeout, halfOpenMaxReqs)
    }

    return client
}

// GetUserDetail 获取用户详情（带重试和熔断器）
//
// 参数:
//   - ctx: 上下文
//   - userID: 用户ID
//   - token: Bearer Token (不含 "Bearer " 前缀)
//
// 返回:
//   - *UserDetail: 用户详情
//   - error: 调用错误
func (c *UserServiceClient) GetUserDetail(ctx context.Context, userID string, token string) (*UserDetail, error) {
    var lastErr error

    // 使用熔断器包装请求
    err := c.circuitBreaker.Call(func() error {
        // 重试逻辑
        for attempt := 0; attempt <= c.maxRetries; attempt++ {
            if attempt > 0 {
                // 计算重试延迟（指数退避）
                delay := c.calculateRetryDelay(attempt)
                global.Logger.Debugf("Retrying user detail request (attempt %d/%d) after %v", 
                    attempt, c.maxRetries, delay)
                
                select {
                case <-ctx.Done():
                    return ctx.Err()
                case <-time.After(delay):
                }
            }

            detail, err := c.doGetUserDetail(ctx, userID, token)
            if err == nil {
                return nil // 成功，重置错误
            }

            lastErr = err

            // 检查是否可重试
            if !c.isRetryableError(err) {
                return err // 不可重试的错误，直接返回
            }

            // 最后一次尝试失败
            if attempt == c.maxRetries {
                return err
            }
        }

        return lastErr
    })

    if err != nil {
        return nil, err
    }

    // 最后一次成功的结果
    return c.doGetUserDetail(ctx, userID, token)
}

// doGetUserDetail 执行实际的HTTP请求
func (c *UserServiceClient) doGetUserDetail(ctx context.Context, userID string, token string) (*UserDetail, error) {
    url := fmt.Sprintf("%s/client/user/detail?id=%s", c.baseURL, userID)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, errors.Wrap(err, "create request failed")
    }

    // 设置认证头
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
    if c.apiKey != "" {
        req.Header.Set("X-API-Key", c.apiKey)
    }
    req.Header.Set("Accept", "application/json")
    req.Header.Set("User-Agent", "injective-explorer/1.0")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, errors.Wrap(err, "request failed")
    }
    defer resp.Body.Close()

    // 读取响应体
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, errors.Wrap(err, "read response body failed")
    }

    var result struct {
        Code    int         `json:"code"`
        Data    UserDetail  `json:"data"`
        Message string      `json:"message"`
    }

    if err := sonic.Unmarshal(body, &result); err != nil {
        return nil, errors.Wrap(err, "decode response failed")
    }

    // 处理业务错误
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("http error: status=%d, code=%d, message=%s", 
            resp.StatusCode, result.Code, result.Message)
    }

    if result.Code != 200 {
        // 401/404 等认证错误不可重试
        if result.Code == 401 || result.Code == 404 {
            return nil, fmt.Errorf("auth error: %s (code: %d)", result.Message, result.Code)
        }
        // 其他错误可重试
        return nil, fmt.Errorf("api error: %s (code: %d)", result.Message, result.Code)
    }

    return &result.Data, nil
}

// calculateRetryDelay 计算重试延迟（指数退避）
func (c *UserServiceClient) calculateRetryDelay(attempt int) time.Duration {
    if c.retryConfig == nil {
        // 默认：100ms * 2^(attempt-1)
        return time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond
    }

    initialDelay := c.retryConfig.InitialDelay.AsDuration()
    if initialDelay == 0 {
        initialDelay = 100 * time.Millisecond
    }

    maxDelay := c.retryConfig.MaxDelay.AsDuration()
    if maxDelay == 0 {
        maxDelay = 2 * time.Second
    }

    multiplier := c.retryConfig.BackoffMultiplier
    if multiplier == 0 {
        multiplier = 2.0
    }

    delay := time.Duration(float64(initialDelay) * multiplier * float64(attempt-1))
    if delay > maxDelay {
        delay = maxDelay
    }

    return delay
}

// isRetryableError 判断错误是否可重试
func (c *UserServiceClient) isRetryableError(err error) bool {
    if err == nil {
        return false
    }

    // 检查是否是网络错误或超时
    if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
        return true
    }

    // 检查HTTP状态码
    errStr := err.Error()
    retryableCodes := []int{500, 502, 503, 504}
    if c.retryConfig != nil && len(c.retryConfig.RetryableStatusCodes) > 0 {
        retryableCodes = make([]int, len(c.retryConfig.RetryableStatusCodes))
        for i, code := range c.retryConfig.RetryableStatusCodes {
            retryableCodes[i] = int(code)
        }
    }

    for _, code := range retryableCodes {
        if fmt.Sprintf("status=%d", code) == errStr || 
           fmt.Sprintf("http error: status=%d", code) == errStr {
            return true
        }
    }

    // 401/404 等认证错误不可重试
    if fmt.Sprintf("auth error:") == errStr {
        return false
    }

    // 其他错误默认可重试
    return true
}
```

---

### 3.3 增强认证中间件

**文件**: `internal/middleware/user_auth.go` (修改)

```go
package middleware

import (
    "context"
    "fmt"
    "strings"
    "time"

    "injective-explorer/internal/conf"
    "injective-explorer/internal/global"
    "injective-explorer/internal/service"
    "injective-explorer/provider/cache"

    "github.com/bytedance/sonic"
    "github.com/go-kratos/kratos/v2/errors"
    "github.com/go-kratos/kratos/v2/middleware"
    khttp "github.com/go-kratos/kratos/v2/transport/http"
    "github.com/redis/go-redis/v9"
)

// ContextKey 上下文键类型
type ContextKey string

const (
    ContextKeyUserID     ContextKey = "user_id"
    ContextKeyUserDetail ContextKey = "user_detail"
    ContextKeyAuthToken  ContextKey = "auth_token"
)

// 缓存键常量
const (
    CacheKeyUserDetailPrefix = "user:detail:"
    CacheKeyUserDetailNegativePrefix = "user:detail:negative:"
)

// UserAuthMiddleware 简单用户认证（保持向后兼容）
// 从 X-User-ID Header 获取用户ID，不验证用户详情
func UserAuthMiddleware() middleware.Middleware {
    return func(handler middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            if httpReq, ok := khttp.RequestFromServerContext(ctx); ok {
                userID := httpReq.Header.Get("X-User-ID")
                if userID == "" {
                    return nil, errors.New(401, "UNAUTHORIZED", "Missing X-User-ID header")
                }
                ctx = context.WithValue(ctx, ContextKeyUserID, userID)
            }
            return handler(ctx, req)
        }
    }
}

// EnhancedUserAuthMiddleware 增强用户认证（调用用户详情接口）
//
// 功能:
//   1. 从 X-User-ID Header 获取用户ID
//   2. 从 Authorization Header 获取 Bearer Token
//   3. 调用用户详情接口验证用户（带缓存）
//   4. 检查用户状态，status=1 才允许访问
//   5. 支持降级模式（用户服务故障时）
//
// 参数:
//   - userClient: 用户服务客户端
//   - cfg: 用户服务配置
func EnhancedUserAuthMiddleware(userClient *service.UserServiceClient, cfg *conf.UserService) middleware.Middleware {
    fallbackEnabled := cfg == nil || cfg.FallbackEnabled
    
    return func(handler middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            httpReq, ok := khttp.RequestFromServerContext(ctx)
            if !ok {
                return handler(ctx, req)
            }

            // 1. 获取用户ID
            userID := httpReq.Header.Get("X-User-ID")
            if userID == "" {
                return nil, errors.New(401, "UNAUTHORIZED", "Missing X-User-ID header")
            }

            // 2. 获取 Bearer Token
            authHeader := httpReq.Header.Get("Authorization")
            if authHeader == "" {
                return nil, errors.New(401, "UNAUTHORIZED", "Missing Authorization header")
            }

            token, err := extractBearerToken(authHeader)
            if err != nil {
                return nil, errors.New(401, "UNAUTHORIZED", "Invalid Authorization header format")
            }

            // 3. 获取用户详情（带缓存）
            userDetail, err := getUserDetailWithCache(ctx, userClient, userID, token, cfg)
            if err != nil {
                global.Logger.Warnf("user detail verification failed for %s: %v", userID, err)

                if fallbackEnabled {
                    // 降级处理：使用默认用户信息
                    global.Logger.Warnf("falling back to default user info for %s", userID)
                    userDetail = &service.UserDetail{
                        ID:      0,
                        Account: userID,
                        Source:  "unknown",
                        Status:  1, // 默认允许
                        Action:  "unknown",
                    }
                } else {
                    return nil, errors.New(401, "UNAUTHORIZED", "User verification failed")
                }
            }

            // 4. 检查用户状态
            if !userDetail.IsActive() {
                return nil, errors.New(403, "FORBIDDEN", "User account is disabled")
            }

            // 5. 将用户信息存入 context
            ctx = context.WithValue(ctx, ContextKeyUserID, userID)
            ctx = context.WithValue(ctx, ContextKeyUserDetail, userDetail)
            ctx = context.WithValue(ctx, ContextKeyAuthToken, token)

            return handler(ctx, req)
        }
    }
}

// extractBearerToken 从 Authorization header 提取 token
func extractBearerToken(authHeader string) (string, error) {
    const prefix = "Bearer "
    if !strings.HasPrefix(authHeader, prefix) {
        return "", fmt.Errorf("invalid authorization format")
    }
    return strings.TrimPrefix(authHeader, prefix), nil
}

// getUserDetailWithCache 带缓存的用户详情获取
//
// 缓存策略:
//   - Key: user:detail:{userID}
//   - TTL: 配置的缓存TTL（默认5分钟）
//   - 防止缓存穿透：缓存空值（用户不存在）
//   - 缓存未命中时调用用户服务接口
func getUserDetailWithCache(
    ctx context.Context,
    client *service.UserServiceClient,
    userID, token string,
    cfg *conf.UserService,
) (*service.UserDetail, error) {
    // 检查缓存是否启用
    cacheEnabled := cfg == nil || cfg.Cache == nil || cfg.Cache.Enabled
    if !cacheEnabled {
        // 缓存未启用，直接调用服务
        return client.GetUserDetail(ctx, userID, token)
    }

    cacheKey := CacheKeyUserDetailPrefix + userID
    negativeCacheKey := CacheKeyUserDetailNegativePrefix + userID
    redisClient := cache.GetRedisClient()

    // 1. 检查负缓存（防止缓存穿透）
    if redisClient != nil {
        negativeCached, err := redisClient.Get(ctx, negativeCacheKey).Result()
        if err == nil && negativeCached == "1" {
            global.Logger.Debugf("user detail negative cache hit for %s", userID)
            return nil, errors.New(404, "USER_NOT_FOUND", "User not found")
        }
    }

    // 2. 尝试从缓存获取
    if redisClient != nil {
        cached, err := redisClient.Get(ctx, cacheKey).Result()
        if err == nil && cached != "" {
            var detail service.UserDetail
            if err := sonic.UnmarshalString(cached, &detail); err == nil {
                global.Logger.Debugf("user detail cache hit for %s", userID)
                // 更新指标：缓存命中
                // metrics.UserServiceCacheHit.Inc()
                return &detail, nil
            }
        } else if err != redis.Nil {
            global.Logger.Warnf("failed to get user detail from cache: %v", err)
        }
    }

    // 3. 缓存未命中，调用服务
    global.Logger.Debugf("user detail cache miss for %s, calling user service", userID)
    detail, err := client.GetUserDetail(ctx, userID, token)
    
    // 更新指标：缓存未命中
    // metrics.UserServiceCacheMiss.Inc()
    
    if err != nil {
        // 用户不存在：设置负缓存
        if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "USER_NOT_FOUND") {
            if redisClient != nil && cfg.Cache != nil {
                negativeTTL := cfg.Cache.NegativeCacheTtl.AsDuration()
                if negativeTTL == 0 {
                    negativeTTL = 1 * time.Minute
                }
                if setErr := redisClient.Set(ctx, negativeCacheKey, "1", negativeTTL).Err(); setErr != nil {
                    global.Logger.Warnf("failed to set negative cache: %v", setErr)
                }
            }
        }
        return nil, err
    }

    // 4. 写入缓存
    if redisClient != nil {
        ttl := 5 * time.Minute // 默认TTL
        if cfg.Cache != nil && cfg.Cache.Ttl != nil {
            ttl = cfg.Cache.Ttl.AsDuration()
        }
        
        if data, err := sonic.MarshalString(detail); err == nil {
            if setErr := redisClient.Set(ctx, cacheKey, data, ttl).Err(); setErr != nil {
                global.Logger.Warnf("failed to cache user detail: %v", setErr)
            } else {
                global.Logger.Debugf("user detail cached for %s (TTL: %v)", userID, ttl)
            }
        }
    }

    return detail, nil
}
```

---

### 3.4 HTTP Server 集成

**文件**: `internal/server/http.go` (修改 NewHTTPServer 函数)

```go
func NewHTTPServer(c *conf.Server, logger log.Logger, bootstrap *conf.Bootstrap) *khttp.Server {
    // ... 现有代码（rateLimiter, statsService等）...

    // 创建用户服务客户端（如果配置了）
    var userClient *service.UserServiceClient
    var userAuthMiddleware middleware.Middleware

    if bootstrap != nil && 
       bootstrap.ApiManagement != nil && 
       bootstrap.ApiManagement.UserService != nil &&
       bootstrap.ApiManagement.UserService.Enabled {
        // 配置了用户服务且启用，使用增强认证
        userClient = service.NewUserServiceClient(bootstrap.ApiManagement.UserService)
        userAuthMiddleware = middleware.EnhancedUserAuthMiddleware(
            userClient, 
            bootstrap.ApiManagement.UserService,
        )
        log.NewHelper(logger).Info("Enhanced user authentication enabled with user service")
    } else {
        // 未配置用户服务或未启用，使用简单认证
        userAuthMiddleware = middleware.UserAuthMiddleware()
        log.NewHelper(logger).Info("Simple user authentication enabled (no user service)")
    }

    // 修改 SelectiveAuthAndRateLimitMiddleware 以支持用户认证
    // 注意：需要在 filter.go 中修改，让 /api/v1/tokens 路径使用用户认证中间件
    // ... 其他代码 ...
}
```

**文件**: `internal/middleware/filter.go` (修改 SelectiveAuthAndRateLimitMiddleware)

```go
// SelectiveAuthAndRateLimitMiddleware 修改为支持用户认证中间件
func SelectiveAuthAndRateLimitMiddleware(
    rateLimiter *RateLimiter,
    statsService *service.UsageStatsService,
    ipLocationService *service.IPLocationService,
    ipWhitelistChecker *IPWhitelistChecker,
    testMode bool,
    userAuthMiddleware middleware.Middleware, // 新增参数
) middleware.Middleware {
    authMiddleware := AuthMiddleware()
    rateLimitMiddleware := RateLimitMiddleware(rateLimiter)
    statsMiddleware := StatsMiddleware(statsService, ipLocationService)

    return func(handler middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            if testMode {
                return handler(ctx, req)
            }

            request, ok := khttp.RequestFromServerContext(ctx)
            if !ok {
                return handler(ctx, req)
            }

            path := request.URL.Path

            if path == "/api/v1/health" {
                return handler(ctx, req)
            }

            // Token 管理路径使用用户认证中间件
            if strings.HasPrefix(path, "/api/v1/tokens") {
                if userAuthMiddleware != nil {
                    userAuthHandler := userAuthMiddleware(handler)
                    return userAuthHandler(ctx, req)
                }
                // 如果没有用户认证中间件，直接通过（向后兼容）
                return handler(ctx, req)
            }

            // ... 其他路径的处理（IP白名单、API Token认证等）...
        }
    }
}
```

---

### 3.5 在服务中使用用户信息

**示例**: 在 `APITokenService` 中获取用户详情

```go
func (s *APITokenService) CreateToken(ctx context.Context, req *pb.CreateTokenRequest) (*pb.Reply, error) {
    // 获取用户ID（两种认证模式都支持）
    userID := s.getUserIDFromContext(ctx)

    // 获取用户详情（仅在增强认证模式下可用）
    if userDetail, ok := ctx.Value(middleware.ContextKeyUserDetail).(*service.UserDetail); ok {
        // 使用扩展信息
        log.Printf("Creating token for user: %s (ID: %d, Source: %s)",
            userDetail.Account, userDetail.ID, userDetail.Source)

        // 可以根据 source 做不同处理
        if userDetail.Source == "explorer" {
            // 特定逻辑
        }
    }

    // 继续业务逻辑...
}
```

---

## 4. 降级策略

### 4.1 降级场景和处理策略

| 场景             | 错误类型   | 行为                                       | 日志级别 |
| ---------------- | ---------- | ------------------------------------------ | -------- |
| 用户服务超时     | 网络错误   | fallback=true 时允许访问，使用默认用户信息 | WARN     |
| 用户服务返回 5xx | 服务器错误 | fallback=true 时允许访问，使用默认用户信息 | WARN     |
| 用户服务返回 401 | 认证错误   | 拒绝访问，返回 401                         | ERROR    |
| 用户服务返回 404 | 用户不存在 | 拒绝访问，返回 401                         | WARN     |
| 用户 status=0    | 业务错误   | 返回 403 Forbidden                         | WARN     |
| Redis 缓存故障   | 缓存错误   | 直接调用用户服务，不影响认证               | DEBUG    |
| 熔断器打开       | 熔断保护   | fallback=true 时允许访问，使用默认用户信息 | WARN     |

### 4.2 降级开关

通过配置 `fallback_enabled` 控制：

**配置项**: `api_management.user_service.fallback_enabled`

- `fallback_enabled=true` (推荐): 
  - 用户服务故障时允许访问（使用默认用户信息）
  - 适用于生产环境，保证服务可用性
  - 日志记录降级事件，便于监控

- `fallback_enabled=false` (严格模式):
  - 用户服务故障时拒绝访问
  - 适用于安全要求极高的场景
  - 可能导致服务不可用

### 4.3 降级时的默认用户信息

```go
// 降级时使用的默认用户信息
userDetail := &service.UserDetail{
    ID:      0,                    // 无效ID
    Account: userID,               // 使用请求中的 userID
    Source:  "unknown",            // 未知来源
    Status:  1,                    // 默认允许访问
    Action:  "unknown",            // 未知操作
}
```

**注意**: 降级模式下，用户详情信息不完整，业务逻辑中需要判断 `ID == 0` 来识别降级用户。

### 4.4 降级监控

降级事件会触发以下监控指标：
- `user_auth_fallback_total`: 降级触发次数
- 日志记录：`WARN` 级别，包含错误信息

建议设置告警：
- 降级次数 > 10次/分钟：告警通知
- 降级持续时间 > 5分钟：升级告警

---

## 5. 安全考虑

### 5.1 认证头传递
- 客户端传入的 `Authorization` Header 会被转发到用户服务
- 服务间调用使用 `X-API-Key` 进行额外验证

### 5.2 敏感信息保护
- `api_key` 从环境变量读取，不写入配置文件
- 日志中不输出完整 token

### 5.3 超时控制
- 用户服务调用默认 5 秒超时
- 重试次数默认 3 次
- 防止级联故障

---

## 6. 实施步骤

### Phase 1: 基础功能（2-3天）

#### Step 1: 配置扩展
1. 修改 `internal/conf/conf.proto`，添加 `UserService` 配置消息
2. 重新生成 Go 代码: `make generate` 或 `protoc ...`
3. 验证配置结构正确

**验收标准**: 
- 配置可以正确解析
- 所有字段都有合理的默认值

#### Step 2: 创建客户端
1. 创建 `internal/service/user_service_client.go`
2. 实现 `GetUserDetail` 方法（基础版本，无重试和熔断器）
3. 添加单元测试（Mock HTTP 客户端）

**验收标准**:
- 可以成功调用用户服务接口
- 正确处理成功和错误响应
- 单元测试覆盖率 > 80%

#### Step 3: 中间件增强
1. 修改 `internal/middleware/user_auth.go`
2. 添加 `EnhancedUserAuthMiddleware`
3. 实现基础缓存逻辑（无负缓存）

**验收标准**:
- 可以正确提取用户ID和Token
- 缓存可以正常工作
- 用户状态验证正确

### Phase 2: 高级功能（2-3天）

#### Step 4: 重试和熔断器
1. 在 `UserServiceClient` 中添加重试机制（指数退避）
2. 实现 `CircuitBreaker`
3. 集成到客户端

**验收标准**:
- 重试机制正常工作
- 熔断器可以正确打开/关闭
- 测试各种故障场景

#### Step 5: 缓存优化
1. 添加负缓存（防止缓存穿透）
2. 优化缓存键设计
3. 添加缓存预热（可选）

**验收标准**:
- 缓存穿透问题解决
- 缓存命中率 > 80%

#### Step 6: Server 集成
1. 修改 `internal/server/http.go`
2. 修改 `internal/middleware/filter.go`
3. 根据配置选择中间件

**验收标准**:
- `/api/v1/tokens` 路径使用用户认证
- 配置未启用时保持向后兼容

### Phase 3: 监控和优化（1-2天）

#### Step 7: 监控指标
1. 创建 `internal/metrics/user_service_metrics.go`
2. 在关键位置添加指标记录
3. 配置 Grafana 监控面板

**验收标准**:
- 所有关键指标都已记录
- Grafana 面板可以正常显示

#### Step 8: 测试验证
1. 单元测试：覆盖所有关键路径
2. 集成测试：测试完整流程
3. 压力测试：验证性能和稳定性
4. 故障测试：测试各种故障场景

**测试场景**:
- ✅ 正常流程：用户认证成功
- ✅ 缓存命中：减少外部调用
- ✅ 缓存未命中：调用用户服务
- ✅ 用户服务超时：触发降级
- ✅ 用户服务5xx错误：触发降级
- ✅ 用户不存在（404）：拒绝访问
- ✅ 用户被禁用（status=0）：返回403
- ✅ 熔断器打开：触发降级
- ✅ Redis故障：直接调用用户服务

**验收标准**:
- 所有测试场景通过
- 性能指标满足要求（P99 < 100ms，缓存命中率 > 80%）

### Phase 4: 部署和上线（1天）

#### Step 9: 部署配置
1. 更新 `configs/config.yaml`，添加用户服务配置（默认禁用）
2. 配置环境变量：
   - `USER_SERVICE_BASE_URL`
   - `USER_SERVICE_API_KEY`
3. 更新部署文档

**配置示例**:
```yaml
api_management:
  user_service:
    enabled: false  # 默认禁用，灰度时启用
    base_url: "${USER_SERVICE_BASE_URL}"
    # ... 其他配置
```

#### Step 10: 灰度发布
1. **阶段1**: 在测试环境验证
   - 启用用户服务
   - 验证所有功能正常
   - 监控指标正常

2. **阶段2**: 小流量灰度（10%）
   - 启用用户服务，`fallback_enabled=true`
   - 监控错误率和性能
   - 观察1-2天

3. **阶段3**: 全量发布
   - 逐步提升流量到100%
   - 持续监控
   - 准备回滚方案

**回滚方案**:
- 设置 `enabled: false` 即可回滚到简单认证
- 无需代码回滚

### 6.1 实施检查清单

- [ ] 配置扩展完成
- [ ] 用户服务客户端实现完成
- [ ] 中间件增强完成
- [ ] 重试机制实现完成
- [ ] 熔断器实现完成
- [ ] 缓存优化完成
- [ ] Server集成完成
- [ ] 监控指标添加完成
- [ ] 单元测试通过
- [ ] 集成测试通过
- [ ] 压力测试通过
- [ ] 故障测试通过
- [ ] 配置文档更新
- [ ] 部署文档更新
- [ ] 监控面板配置完成
- [ ] 灰度发布计划制定

---

## 7. 监控与日志

### 7.1 Prometheus 监控指标

**文件**: `internal/metrics/user_service_metrics.go` (新增)

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // 用户服务请求总数（按状态码分类）
    UserServiceRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "user_service_requests_total",
            Help: "Total number of user service requests",
        },
        []string{"status_code", "method"},
    )

    // 用户服务请求耗时（直方图）
    UserServiceRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "user_service_request_duration_seconds",
            Help:    "User service request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method"},
    )

    // 缓存命中/未命中
    UserServiceCacheHit = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "user_service_cache_hit_total",
            Help: "Total number of user detail cache hits",
        },
    )

    UserServiceCacheMiss = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "user_service_cache_miss_total",
            Help: "Total number of user detail cache misses",
        },
    )

    // 熔断器状态
    CircuitBreakerState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "user_service_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"service"},
    )

    // 认证失败次数
    UserAuthFailuresTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "user_auth_failures_total",
            Help: "Total number of user authentication failures",
        },
        []string{"reason"}, // reason: "missing_header", "invalid_token", "user_disabled", "service_error"
    )

    // 降级触发次数
    UserAuthFallbackTotal = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "user_auth_fallback_total",
            Help: "Total number of fallback to simple auth",
        },
    )
)
```

### 7.2 关键指标说明

| 指标                                    | 类型      | 说明                         | 告警阈值       |
| --------------------------------------- | --------- | ---------------------------- | -------------- |
| `user_service_requests_total`           | Counter   | 用户服务请求总数（按状态码） | -              |
| `user_service_request_duration_seconds` | Histogram | 用户服务请求耗时             | P99 > 3s       |
| `user_service_cache_hit_total`          | Counter   | 缓存命中次数                 | -              |
| `user_service_cache_miss_total`         | Counter   | 缓存未命中次数               | -              |
| `user_service_circuit_breaker_state`    | Gauge     | 熔断器状态                   | State=1 (打开) |
| `user_auth_failures_total`              | Counter   | 认证失败次数                 | > 100/min      |
| `user_auth_fallback_total`              | Counter   | 降级触发次数                 | > 10/min       |

### 7.3 日志输出

```go
// 成功获取用户详情
global.Logger.Infof("User authenticated: %s (ID: %d, Source: %s)", 
    userDetail.Account, userDetail.ID, userDetail.Source)

// 缓存命中
global.Logger.Debugf("User detail cache hit for %s", userID)

// 缓存未命中
global.Logger.Debugf("User detail cache miss for %s, calling user service", userID)

// 降级警告
global.Logger.Warnf("User service unavailable, falling back to simple auth for %s: %v", userID, err)

// 熔断器打开
global.Logger.Warnf("Circuit breaker opened after %d failures", failureCount)

// 熔断器关闭
global.Logger.Infof("Circuit breaker closed after successful request")

// 认证失败
global.Logger.Warnf("User authentication failed for %s: %v", userID, err)

// 用户被禁用
global.Logger.Warnf("User account disabled: %s (ID: %d)", userID, userDetail.ID)
```

### 7.4 Grafana 监控面板

建议创建以下监控面板：

1. **用户服务健康度**
   - 请求成功率（200/total）
   - 平均响应时间
   - P95/P99 响应时间
   - 错误率（4xx+5xx/total）

2. **缓存性能**
   - 缓存命中率（hit/(hit+miss)）
   - 缓存命中/未命中趋势

3. **熔断器状态**
   - 熔断器状态变化
   - 失败次数趋势

4. **认证统计**
   - 认证成功/失败次数
   - 降级触发次数
   - 用户禁用次数

---

## 8. 风险评估与缓解措施

| 风险         | 影响 | 概率 | 缓解措施                                                                      | 监控指标                                |
| ------------ | ---- | ---- | ----------------------------------------------------------------------------- | --------------------------------------- |
| 用户服务故障 | 高   | 中   | 1. 启用 fallback 模式<br>2. 使用缓存减少调用<br>3. 熔断器防止级联故障         | `user_auth_fallback_total`              |
| 性能下降     | 中   | 低   | 1. 5分钟缓存，命中率>80%<br>2. 异步缓存更新<br>3. 连接池复用                  | `user_service_request_duration_seconds` |
| 配置泄露     | 中   | 低   | 1. API Key 从环境变量读取<br>2. 不在日志中输出敏感信息<br>3. 使用密钥管理服务 | -                                       |
| 向后不兼容   | 低   | 低   | 1. 默认禁用，需显式启用<br>2. 无配置时保持现有行为<br>3. 提供平滑迁移方案     | -                                       |
| 缓存穿透     | 中   | 中   | 1. 负缓存（缓存空值）<br>2. 布隆过滤器（可选）                                | `user_service_cache_miss_total`         |
| 缓存雪崩     | 中   | 低   | 1. 缓存TTL添加随机值<br>2. 预热热点数据                                       | -                                       |
| 认证绕过     | 高   | 低   | 1. 严格验证用户状态<br>2. 记录所有认证失败<br>3. 定期审计日志                 | `user_auth_failures_total`              |
| 级联故障     | 高   | 低   | 1. 熔断器机制<br>2. 超时控制<br>3. 限流保护                                   | `circuit_breaker_state`                 |

### 8.1 安全考虑

1. **Token 验证**
   - 客户端传入的 `Authorization` Header 会被转发到用户服务
   - 服务间调用使用 `X-API-Key` 进行额外验证（可选）

2. **敏感信息保护**
   - `api_key` 从环境变量读取，不写入配置文件
   - 日志中不输出完整 token，只记录前4位和后4位
   - 用户详情信息仅在必要时记录

3. **超时控制**
   - 用户服务调用默认 5 秒超时
   - 重试次数默认 3 次
   - 防止级联故障

4. **请求限流**
   - 结合现有的 RateLimitMiddleware
   - 防止恶意请求导致用户服务压力过大

---

## 9. 附录

### 9.1 相关文件清单

| 文件                                      | 操作 | 说明                  |
| ----------------------------------------- | ---- | --------------------- |
| `internal/conf/conf.proto`                | 修改 | 添加 UserService 配置 |
| `internal/service/user_service_client.go` | 新增 | 用户服务客户端        |
| `internal/middleware/user_auth.go`        | 修改 | 增强认证中间件        |
| `internal/server/http.go`                 | 修改 | 集成中间件            |
| `configs/config.yaml`                     | 修改 | 添加配置示例          |
| `docs/user-service-integration-design.md` | 新增 | 本文档                |

### 9.2 环境变量

| 变量名                 | 说明           | 示例        |
| ---------------------- | -------------- | ----------- |
| `USER_SERVICE_API_KEY` | 服务间调用密钥 | `sk_xxx...` |

---

---

## 10. 优化总结

### 10.1 本次优化增强的关键点

1. **多环境支持**
   - 支持生产/预发布环境配置
   - 通过环境变量灵活切换

2. **熔断器机制**
   - 防止级联故障
   - 自动恢复机制
   - 半开状态保护

3. **重试策略优化**
   - 指数退避算法
   - 可配置的重试参数
   - 智能错误分类

4. **缓存策略增强**
   - 负缓存防止缓存穿透
   - 可配置的TTL
   - 缓存键规范化

5. **监控体系完善**
   - Prometheus 指标
   - Grafana 监控面板
   - 告警规则建议

6. **降级策略细化**
   - 详细的降级场景分类
   - 降级监控和告警
   - 默认用户信息规范

7. **安全性增强**
   - Token 安全处理
   - 敏感信息保护
   - 请求限流集成

8. **实施步骤细化**
   - 分阶段实施计划
   - 详细的验收标准
   - 灰度发布方案

### 10.2 性能优化建议

1. **缓存优化**
   - 目标缓存命中率 > 80%
   - 使用本地缓存（可选，如 go-cache）作为二级缓存
   - 预热热点用户数据

2. **连接池优化**
   - HTTP 客户端连接池大小：10-20
   - Keep-Alive 连接复用
   - 连接超时设置合理

3. **异步处理**
   - 缓存更新异步化
   - 监控指标记录异步化
   - 日志记录异步化（可选）

### 10.3 后续优化方向

1. **功能增强**
   - [ ] 支持批量查询用户详情（减少请求数）
   - [ ] 支持用户信息变更通知（主动失效缓存）
   - [ ] 支持多级缓存（本地缓存 + Redis）

2. **性能优化**
   - [ ] 使用 gRPC 替代 HTTP（如果用户服务支持）
   - [ ] 实现请求合并（相同用户ID的并发请求合并）
   - [ ] 实现智能预取（预测可能访问的用户）

3. **可观测性**
   - [ ] 分布式追踪（OpenTelemetry）
   - [ ] 详细的错误分类和统计
   - [ ] 用户行为分析

---

**文档版本**: 2.0（优化增强版）
**创建日期**: 2026-02-02
**最后更新**: 2026-02-02
**作者**: Claude Code
