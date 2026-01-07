# injective-explorer 性能优化方案

## 测试结果分析

### 当前性能表现

| 测试场景 | QPS | 平均延迟 | 说明 |
|---------|-----|---------|------|
| 测试1: 健康检查 | 10,571 | 4.71ms | ✅ 基础性能优秀 |
| 测试2: 区块查询 | 398 | 104.69ms | ✅ 单接口性能良好 |
| 测试3: 混合测试 | 30 | 1,490.84ms | ❌ **性能严重下降** |

### 问题诊断

**核心问题**：混合测试时QPS从10,571急剧下降到30，延迟从4.71ms增加到1,490.84ms，说明某些接口在高并发下成为瓶颈。

**瓶颈分析**：
1. **认证中间件**：每个请求都要同步查询数据库验证token，没有缓存
2. **数据库连接池**：MaxOpenConns=50可能不够，导致连接等待
3. **账户相关接口**：复杂的数据库查询，如果缓存未命中需要查询链上数据
4. **限流中间件**：多次Redis查询（虽然已优化，但仍有一定开销）
5. **统计中间件**：每个请求创建goroutine，高并发下可能造成goroutine泄漏

---

## 优化方案

### 优先级1：认证中间件缓存（最关键）

**问题**：每个请求都要查询数据库验证token，这是最大的性能瓶颈。

**解决方案**：实现Token缓存机制

#### 1.1 添加Token缓存层

**文件**：`internal/middleware/auth.go`

**优化点**：
- 使用Redis缓存已验证的token信息（TTL: 5分钟）
- 缓存key格式：`token:hash:{tokenHash}`
- 缓存命中时直接返回，避免数据库查询
- 缓存未命中时查询数据库并写入缓存

**预期效果**：
- 减少数据库查询压力 90%+
- 认证延迟从 ~10-50ms 降低到 ~1-2ms（Redis查询）
- QPS提升 5-10倍

#### 1.2 实现代码

```go
// 在 validateToken 函数中添加缓存逻辑
func validateToken(ctx context.Context, token string) (*models.APIToken, error) {
    // Hash token
    hash := sha256.Sum256([]byte(token))
    tokenHash := hex.EncodeToString(hash[:])
    
    // 1. 先查Redis缓存
    redisClient := cache.GetRedisClient()
    if redisClient != nil {
        cacheKey := fmt.Sprintf("token:hash:%s", tokenHash)
        cachedData, err := redisClient.Get(ctx, cacheKey).Result()
        if err == nil {
            // 缓存命中，反序列化
            var tokenInfo models.APIToken
            if err := sonic.Unmarshal([]byte(cachedData), &tokenInfo); err == nil {
                return &tokenInfo, nil
            }
        }
    }
    
    // 2. 缓存未命中，查询数据库
    var tokenInfo models.APIToken
    err := db.Get().Where("token_hash = ?", tokenHash).First(&tokenInfo).Error
    if err == gorm.ErrRecordNotFound {
        return nil, errors.New(401, "TOKEN_INVALID", "Invalid API token")
    }
    if err != nil {
        return nil, errors.New(500, "DATABASE_ERROR", "Failed to validate token")
    }
    
    // 3. 写入缓存（异步，不阻塞请求）
    if redisClient != nil && tokenInfo.Status == "active" {
        go func() {
            cacheKey := fmt.Sprintf("token:hash:%s", tokenHash)
            tokenJSON, _ := sonic.Marshal(tokenInfo)
            redisClient.Set(context.Background(), cacheKey, tokenJSON, 5*time.Minute)
        }()
    }
    
    return &tokenInfo, nil
}
```

---

### 优先级2：数据库连接池优化

**问题**：MaxOpenConns=50在高并发下可能不够，导致连接等待。

**解决方案**：增加连接池大小

**文件**：`provider/db/db.go`

**优化点**：
- 将 MaxOpenConns 从 50 增加到 100（需要确保MySQL max_connections >= 150）
- 将 MaxIdleConns 从 15 增加到 30（30% of MaxOpenConns）
- 优化连接生命周期设置

**预期效果**：
- 减少连接等待时间
- 提升并发处理能力

**实现代码**：

```go
// 在 Init 函数中修改连接池配置
sqlDB.SetMaxIdleConns(30)                  // 增加到30 (30% of MaxOpenConns)
sqlDB.SetMaxOpenConns(100)                 // 增加到100 (需要MySQL max_connections >= 150)
sqlDB.SetConnMaxLifetime(5 * time.Minute)  // 保持5分钟
sqlDB.SetConnMaxIdleTime(3 * time.Minute)  // 保持3分钟
```

**注意**：需要确保MySQL配置：
```sql
-- 检查当前max_connections
SHOW VARIABLES LIKE 'max_connections';

-- 如果小于150，需要增加（建议设置为200）
SET GLOBAL max_connections = 200;
```

---

### 优先级3：账户接口查询优化

**问题**：账户相关接口（info, balances, transactions）查询复杂，如果缓存未命中需要查询链上数据，非常耗时。

**解决方案**：增强缓存策略和查询优化

#### 3.1 账户信息缓存

**文件**：`internal/service/explorer.go`

**优化点**：
- 账户信息缓存（TTL: 5分钟）
- 账户余额缓存（TTL: 1分钟）
- 账户交易列表缓存（TTL: 30秒）

**预期效果**：
- 减少链上查询 80%+
- 账户接口延迟降低 50-70%

#### 3.2 数据库查询优化

**优化点**：
- 确保账户相关表有正确的索引
- 使用批量查询减少数据库往返

**检查索引**：
```sql
-- 检查账户表索引
SHOW INDEX FROM accounts;
SHOW INDEX FROM account_balances;

-- 如果没有address索引，需要添加
CREATE INDEX idx_accounts_address ON accounts(address);
CREATE INDEX idx_account_balances_address ON account_balances(address);
```

---

### 优先级4：限流中间件优化

**问题**：限流中间件需要多次Redis查询。

**解决方案**：使用Redis Pipeline批量查询

**文件**：`internal/middleware/ratelimit_middleware.go`

**优化点**：
- 将IP限流、Token限流、用户月度限额的Redis查询合并为Pipeline
- 减少Redis往返次数从3次到1次

**预期效果**：
- 限流检查延迟降低 60-70%

---

### 优先级5：统计中间件优化

**问题**：每个请求创建goroutine，高并发下可能造成goroutine泄漏。

**解决方案**：使用工作池模式

**文件**：`internal/middleware/stats_middleware.go`

**优化点**：
- 使用有界goroutine池处理统计记录
- 避免无限制创建goroutine

**预期效果**：
- 减少goroutine数量
- 降低内存使用

---

### 优先级6：健康检查接口优化

**问题**：健康检查接口虽然性能很好，但可以进一步优化。

**解决方案**：跳过不必要的中间件

**文件**：`internal/middleware/filter.go`

**优化点**：
- 健康检查接口跳过认证、限流、统计中间件
- 直接返回，减少处理时间

**实现代码**：

```go
// 在 SelectiveAuthAndRateLimitMiddleware 中添加
path := request.URL.Path
if path == "/api/v1/health" {
    // 健康检查接口跳过所有中间件
    return handler(ctx, req)
}
```

---

## 实施计划

### 阶段1：快速优化（1-2天）
1. ✅ 实现Token缓存（优先级1）
2. ✅ 优化数据库连接池（优先级2）
3. ✅ 健康检查接口优化（优先级6）

**预期效果**：QPS从30提升到200-300

### 阶段2：深度优化（3-5天）
4. ✅ 账户接口缓存优化（优先级3）
5. ✅ 限流中间件Pipeline优化（优先级4）
6. ✅ 统计中间件工作池优化（优先级5）

**预期效果**：QPS提升到500-800

### 阶段3：监控和调优（持续）
7. 添加性能监控指标
8. 根据实际负载调优参数
9. 数据库查询优化

**预期效果**：QPS稳定在800-1000+

---

## 预期效果总结

| 优化项 | 当前QPS | 预期QPS | 提升倍数 |
|--------|---------|---------|---------|
| 混合测试（优化前） | 30 | - | - |
| 阶段1优化后 | 30 | 200-300 | 6-10x |
| 阶段2优化后 | 200-300 | 500-800 | 2-3x |
| 阶段3优化后 | 500-800 | 800-1000+ | 1.5-2x |

**总体提升**：从30 QPS提升到800-1000+ QPS，**提升26-33倍**

---

## 监控指标

优化后需要监控以下指标：

1. **QPS**：每秒请求数
2. **延迟分布**：P50, P95, P99延迟
3. **错误率**：4xx, 5xx错误比例
4. **数据库连接池使用率**：MaxOpenConns使用情况
5. **Redis连接池使用率**：连接数使用情况
6. **Goroutine数量**：避免goroutine泄漏

---

## 注意事项

1. **Token缓存失效**：当token状态变更（禁用/启用）时，需要清除缓存
2. **数据库连接数**：增加MaxOpenConns时，确保MySQL max_connections足够大
3. **缓存一致性**：账户信息缓存需要考虑数据一致性
4. **压力测试**：每个优化后都要进行压力测试验证效果

---

## 回滚方案

如果优化后出现问题，可以快速回滚：

1. Token缓存：如果Redis故障，代码会自动降级到数据库查询
2. 连接池配置：可以快速调整回原来的值
3. 中间件优化：可以通过配置开关控制

---

## 后续优化方向

1. **读写分离**：数据库主从分离，读操作使用从库
2. **CDN缓存**：静态数据和热点数据使用CDN
3. **API网关**：使用Nginx/Envoy做限流和缓存
4. **数据库分片**：如果数据量继续增长，考虑分片
5. **微服务拆分**：将账户服务、区块服务等拆分为独立服务

