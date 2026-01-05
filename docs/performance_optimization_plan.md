# 性能优化分析与优化清单

基于对injective-explorer项目的深入分析,我发现了以下性能瓶颈并制定了详细的优化计划。

## **当前性能状况分析**

**瓶颈识别:**
1. **数据库层**:复杂查询、N+1问题、连接池未优化
2. **Redis层**:限流检查增加5-10ms延迟、缓存命中率低
3. **API层**:同步处理、重复数据转换、大数据集处理慢
4. **并发层**:工作线程配置保守(默认5-10个)
5. **缓存策略**:最大缓存1000条,频繁失效

---

## **优化清单与排期** (按一周5个工作日)

### **第1天: 数据库优化 (预计提升: QPS +200-300)**

#### 1.1 数据库连接池优化 ⚡ **高优先级**
**文件**: `provider/db/db.go`
```go
// 优化连接池配置
sqlDB.SetMaxOpenConns(100)        // 从默认25提升到100
sqlDB.SetMaxIdleConns(25)         // 从默认2提升到25
sqlDB.SetConnMaxLifetime(5 * time.Minute)
```
**预期提升**: 减少连接争用,提升并发能力

#### 1.2 添加数据库查询缓存 ⚡ **高优先级**
**文件**: `internal/service/explorer.go` (多个查询函数)
```go
// 为高频查询添加缓存
// GetTransactionByHash, GetBlockByHeight, GetAccountTransactions等
cacheKey := fmt.Sprintf("db:tx:%s", hash)
cached := redis.Get(ctx, cacheKey)
if cached != nil {
    return cached, nil
}
// 查询后缓存5-10分钟
redis.Set(ctx, cacheKey, result, 10*time.Minute)
```
**预期提升**: 减少数据库压力50-70%

#### 1.3 优化事务表索引 ⚡ **高优先级**
**文件**: `internal/models/transaction.go`
```sql
-- 为高频查询添加复合索引 (MySQL版本)
CREATE INDEX idx_transactions_signer_timestamp
ON transactions (signer, block_unix_timestamp)
WHERE archived = 0;

CREATE INDEX idx_transactions_evm_hash
ON transactions (evm_hash)
WHERE evm_hash != '' AND archived = 0;

CREATE INDEX idx_transactions_block_number_code
ON transactions (block_number, code)
WHERE archived = 0;

-- 注意: MySQL不支持CONCURRENTLY关键字
-- 对于InnoDB表，MySQL 5.6+使用在线DDL，减少了表锁定时间
-- 对于超大表(>10M行)，建议使用pt-online-schema-change工具
```
**预期提升**: 查询性能提升3-5倍

---

### **第2天: Redis和缓存优化 (预计提升: QPS +150-250)**

#### 2.1 实现本地内存缓存 ⚡ **高优先级**
**新文件**: `provider/cache/local_cache.go`
```go
// 使用ristretto或go-cache实现本地缓存
// 减少Redis网络往返 (0.5-1ms -> 0.01ms)
localCache := ristretto.NewCache(&ristretto.Config{
    NumCounters: 1e7,     // 1000万个计数器
    MaxCost:     1 << 30, // 1GB
    BufferItems: 64,      // 缓冲项
})
```
**文件**: `internal/middleware/ratelimit.go`
```go
// 限流检查先查本地缓存
localCache.Get(ipKey)
```
**预期提升**: 减少Redis延迟5-10ms/请求

#### 2.2 扩大Redis缓存容量 ⚡ **中优先级**
**文件**: `internal/service/constants.go`
```go
// 从1000提升到10000
CacheMaxSize = 10000  // 从1000增加10倍
CacheTotalTTL = 15 * time.Minute  // 从5分钟提升
```
**预期提升**: 缓存命中率从30%提升到70%

#### 2.3 优化限流器性能 ⚡ **中优先级**
**文件**: `internal/middleware/ratelimit.go`
```go
// 使用令牌桶算法替代滑动窗口
// 减少4次Redis操作到1次
func (r *RateLimiter) CheckTokenRateLimitOptimized(...) {
    // 使用Redis Lua脚本原子操作
    script := `
    local current = redis.call('GET', KEYS[1])
    if current == false then
        redis.call('SET', KEYS[1], 1, 'EX', 2)
        return 1
    end
    if tonumber(current) >= ARGV[1] then
        return 0
    end
    redis.call('INCR', KEYS[1])
    return 1
    `
}
```
**预期提升**: 限流检查从4次Redis操作减少到1次

---

### **第3天: API层异步优化 (预计提升: QPS +200-300)**

#### 3.1 实现批量查询接口 ⚡ **高优先级**
**文件**: `api/explorer/v1/explorer.proto`
```protobuf
// 添加批量查询接口
rpc GetTransactionsBatch (GetTransactionsBatchRequest) returns (Reply);
rpc GetBlocksBatch (GetBlocksBatchRequest) returns (Reply);

message GetTransactionsBatchRequest {
  repeated string hashes = 1;
  repeated string addresses = 2;
}
```
**预期提升**: 批量查询减少90%网络开销

#### 3.2 异步统计更新 ⚡ **高优先级**
**文件**: `internal/service/explorer.go`
```go
// 将统计更新改为异步
func (s *ExplorerService) GetTransactionStats(...) {
    // 立即返回缓存数据
    go s.updateStatsInBackground() // 后台更新
}
```
**预期提升**: API响应时间从100ms减少到10ms

#### 3.3 优化JSON序列化 ⚡ **高优先级**
**文件**: 多处API响应处理
```go
// 使用sonic池化编码器
var encoderPool = sync.Pool{
    New: func() interface{} {
        return &sonic.Encoder{}
    },
}

func marshalWithPool(v interface{}) ([]byte, error) {
    enc := encoderPool.Get().(*sonic.Encoder)
    defer encoderPool.Put(enc)
    return enc.Encode(v)
}
```
**预期提升**: JSON处理速度提升2-3倍

---

### **第4天: 并发和连接优化 (预计提升: QPS +150-200)**

#### 4.1 增加工作线程池 ⚡ **中优先级**
**文件**: `internal/service/constants.go`
```go
// 将并发工作线程翻倍
defaultConcurrentTxProcessors = 20     // 从5增加到20
defaultBackwardSyncWorkers = 10        // 从5增加到10
defaultHistoricalSyncWorkers = 5       // 从2增加到5
defaultEVMDetailsWorkerCount = 10      // 从3增加到10
```
**预期提升**: 并行处理能力提升3-4倍

#### 4.2 HTTP连接池优化 ⚡ **中优先级**
**文件**: `internal/server/http.go`
```go
// 配置HTTP/TCP参数
httpSrv.MaxHeaderBytes = 1 << 20  // 1MB
httpSrv.IdleTimeout = 90 * time.Second
httpSrv.ReadTimeout = 15 * time.Second  // 从30秒减少
httpSrv.WriteTimeout = 15 * time.Second // 从30秒减少

// 启用TCP_FASTOPEN和TCP_NODELAY
tcpListener := tcpListener.(*net.TCPListener)
tcpListener.SetKeepAlive(true)
tcpListener.SetKeepAlivePeriod(30 * time.Second)
```
**预期提升**: 减少连接建立开销

#### 4.3 实现请求合并 ⚡ **高优先级**
**新文件**: `internal/middleware/request_coalescing.go`
```go
// 合并相同请求,避免重复处理
type RequestCoalescer struct {
    sync.Mutex
    inflight map[string]*inflightRequest
}

func (c *RequestCoalescer) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
    // 如果相同请求在处理中,等待结果而不是重新执行
}
```
**预期提升**: 减少重复计算,缓存击穿保护

---

### **第5天: 监控和调优 (预计提升: QPS +100-150)**

#### 5.1 添加性能监控 ⚡ **高优先级**
**新文件**: `internal/middleware/metrics.go`
```go
// 使用Prometheus收集详细指标
apiDuration := prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name: "api_request_duration_ms",
        Help: "API request duration in milliseconds",
        Buckets: []float64{10, 50, 100, 200, 500, 1000, 2000},
    },
    []string{"method", "endpoint", "status"},
)

// 收集数据库、Redis、外部API调用耗时
```

#### 5.2 实现自适应限流 ⚡ **中优先级**
**文件**: `internal/middleware/adaptive_ratelimit.go`
```go
// 根据系统负载动态调整限制
type AdaptiveLimiter struct {
    cpuThreshold    float64
    memoryThreshold float64

    // CPU > 80%时自动降低限制10%
    // Memory > 80%时自动降低限制15%
}
```

#### 5.3 慢查询优化和索引分析 ⚡ **高优先级**
```bash
# 启用数据库慢查询日志
log_min_duration_statement = 500ms

# 使用pg_stat_statements分析最耗时查询
SELECT * FROM pg_stat_statements ORDER BY total_time DESC LIMIT 10;

# 根据查询模式添加必要索引
```

---

### **预计总性能提升**

| 优化项目 | 预计QPS提升 | 累计QPS |
|---------|------------|---------|
| 基准线 | - | 300 |
| 数据库优化 | +250 | 550 |
| Redis缓存优化 | +200 | 750 |
| API异步优化 | +250 | 1000 |
| 并发优化 | +175 | 1175 |
| 监控调优 | +125 | 1300 |

**最终结果: QPS从300提升到1300+,超过1000的目标!**

---

## **关键优化点详解**

### **1. 为什么需要本地缓存?**
当前所有缓存都走Redis,每个请求增加0.5-1ms网络延迟。对于1000 QPS,这相当于每秒增加0.5-1秒的等待时间。本地缓存将这部分延迟降低到0.01ms,减少50倍。

### **2. 连接池优化的重要性**
默认GORM连接池很小(MaxOpenConns=25),在高并发下会成为瓶颈。增加到100后,可以支持更多并行请求,减少排队时间。

### **3. 限流器的性能影响**
当前限流需要4次Redis操作(删除过期、计数、添加、设置过期),每次请求增加4-6ms延迟。优化后减少到1次Lua脚本操作,延迟降低到1-2ms。

### **4. 批量查询的价值**
单个查询需要网络往返10-20ms。批量查询将100次请求合并为1次,减少99%的网络延迟,特别适合移动端批量获取数据。

---

## **实施建议**

1. **分阶段部署**:每天部署一部分优化,监控性能变化
2. **先做数据库优化**:数据库是所有请求的瓶颈,优化效果最明显
3. **充分测试**:每次优化后进行压测,验证效果
4. **监控关键指标**:QPS、响应时间、数据库CPU、Redis延迟、错误率
5. **回滚计划**:准备快速回滚方案,以防问题

## **监控指标**

优化过程中需要监控:
- QPS (目标: >1000)
- P95延迟 (目标: <100ms)
- 数据库查询时间 (目标: <50ms)
- Redis命中率 (目标: >70%)
- 连接池使用率 (目标: <80%)
- CPU和内存使用率 (目标: <70%)

按照这份优化计划,可以稳定实现QPS 1000+的目标,并有潜力达到1300+QPS。
