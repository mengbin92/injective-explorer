# 监控指标指南

本文档描述了如何使用 Prometheus 监控 Injective Explorer 项目的资源使用情况，以检测内存泄漏和 goroutine 泄漏。

## 新增监控指标

### 1. 数据库连接池指标

新增的数据库连接池指标可以帮助你监控数据库连接的使用情况：

```
# 数据库连接数
database_connections{state="idle"}        # 空闲连接数
database_connections{state="in_use"}      # 正在使用的连接数
database_connections{state="open"}        # 总打开连接数

# 数据库连接统计
database_connection_stats{stat_type="wait_count"}         # 等待连接的总次数
database_connection_stats{stat_type="wait_duration_ns"}   # 等待连接的总时长（纳秒）
database_connection_stats{stat_type="max_idle_closed"}    # 因超过最大空闲数关闭的连接数
database_connection_stats{stat_type="max_lifetime_closed"} # 因超过最大生命周期关闭的连接数
```

### 2. Redis 连接池指标

新增的 Redis 连接池指标可以帮助你监控 Redis 连接的健康状况：

```
redis_connections{state="hits"}          # 命中连接池的次数
redis_connections{state="misses"}        # 未命中连接池的次数
redis_connections{state="timeouts"}      # 连接池超时次数
redis_connections{state="total_conns"}   # 总连接数
redis_connections{state="idle_conns"}    # 空闲连接数
redis_connections{state="stale_conns"}   # 过期连接数
```

### 3. 运行时指标（已存在）

```
# Goroutine 数量
app_goroutines

# 内存使用（按类型）
app_memory_bytes{type="heap_alloc"}  # 堆内存分配
app_memory_bytes{type="heap_sys"}    # 堆内存系统
app_memory_bytes{type="stack_inuse"} # 栈内存使用
app_memory_bytes{type="sys_total"}   # 总内存

# GC 统计
app_gc_stats{type="num_gc"}           # GC 次数
app_gc_stats{type="pause_total_ns"}   # GC 总暂停时间
```

## 验证指标

### 本地测试

启动服务后，使用以下命令查看指标：

```bash
# 查看所有指标
curl http://localhost:8000/metrics

# 查看特定的数据库连接指标
curl http://localhost:8000/metrics | grep database_connections

# 查看特定的 Redis 连接指标
curl http://localhost:8000/metrics | grep redis_connections

# 查看 goroutine 数量
curl http://localhost:8000/metrics | grep app_goroutines

# 查看内存使用情况
curl http://localhost:8000/metrics | grep app_memory_bytes
```

### 预期输出示例

```
# 数据库连接池指标
database_connections{state="idle"} 25
database_connections{state="in_use"} 5
database_connections{state="open"} 30

database_connection_stats{stat_type="wait_count"} 0
database_connection_stats{stat_type="wait_duration_ns"} 0
database_connection_stats{stat_type="max_idle_closed"} 2
database_connection_stats{stat_type="max_lifetime_closed"} 1

# Redis 连接池指标
redis_connections{state="hits"} 1523
redis_connections{state="misses"} 12
redis_connections{state="timeouts"} 0
redis_connections{state="total_conns"} 10
redis_connections{state="idle_conns"} 8
redis_connections{state="stale_conns"} 0

# 运行时指标
app_goroutines 45
app_memory_bytes{type="heap_alloc"} 52428800
app_memory_bytes{type="heap_sys"} 67108864
app_gc_stats{type="num_gc"} 12
```

## Prometheus 告警规则

### Goroutine 泄漏检测

```yaml
groups:
  - name: goroutine_leaks
    interval: 30s
    rules:
      # Goroutine 数量持续增长（可能存在泄漏）
      - alert: GoroutineLeakDetected
        expr: rate(app_goroutines[5m]) > 0.5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "检测到 Goroutine 泄漏"
          description: "Goroutine 数量在过去 10 分钟内持续增长，当前值: {{ $value }}"

      # Goroutine 数量异常高
      - alert: HighGoroutineCount
        expr: app_goroutines > 5000
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Goroutine 数量过高"
          description: "当前 Goroutine 数量为 {{ $value }}，超过阈值 5000"
```

### 内存泄漏检测

```yaml
  - name: memory_leaks
    interval: 30s
    rules:
      # 堆内存持续增长
      - alert: HeapMemoryLeaking
        expr: rate(app_memory_bytes{type="heap_alloc"}[10m]) > 1048576
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "堆内存持续增长"
          description: "堆内存分配速率超过 1MB/s，持续 15 分钟"

      # GC 压力过大
      - alert: HighGCPause
        expr: rate(app_gc_stats{type="pause_total_ns"}[5m]) > 10000000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "GC 压力过大"
          description: "GC 暂停时间速率超过 10ms/s"
```

### 数据库连接池告警

```yaml
  - name: database_connection_pool
    interval: 30s
    rules:
      # 连接池使用率过高
      - alert: DatabasePoolHighUsage
        expr: |
          database_connections{state="in_use"} / database_connections{state="open"} > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "数据库连接池使用率过高"
          description: "连接池使用率为 {{ $value | humanizePercentage }}"

      # 连接等待次数过多
      - alert: DatabaseConnectionWaits
        expr: rate(database_connection_stats{stat_type="wait_count"}[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "数据库连接等待次数过多"
          description: "连接等待速率为 {{ $value }}/s"

      # 连接被频繁关闭
      - alert: DatabaseConnectionsClosing
        expr: |
          rate(database_connection_stats{stat_type="max_lifetime_closed"}[5m]) > 1
        for: 10m
        labels:
          severity: info
        annotations:
          summary: "数据库连接因生命周期到期而被关闭"
          description: "关闭速率: {{ $value }}/s，可能需要增加 ConnMaxLifetime"
```

### Redis 连接池告警

```yaml
  - name: redis_connection_pool
    interval: 30s
    rules:
      # 连接池超时
      - alert: RedisPoolTimeouts
        expr: rate(redis_connections{state="timeouts"}[5m]) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Redis 连接池超时"
          description: "连接池超时速率为 {{ $value }}/s"

      # 连接池未命中率高
      - alert: RedisPoolMissRate
        expr: |
          rate(redis_connections{state="misses"}[5m]) /
          (rate(redis_connections{state="hits"}[5m]) + rate(redis_connections{state="misses"}[5m])) > 0.1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Redis 连接池未命中率过高"
          description: "未命中率为 {{ $value | humanizePercentage }}"
```

## Grafana 仪表板建议

### 核心监控面板

1. **Goroutine 趋势图**
   - 指标: `app_goroutines`
   - 显示: 时间序列图
   - 告警线: 5000

2. **内存使用趋势**
   - 指标: `app_memory_bytes{type="heap_alloc"}`
   - 显示: 时间序列图
   - 增长率: `rate(app_memory_bytes{type="heap_alloc"}[5m])`

3. **GC 性能**
   - 指标: `rate(app_gc_stats{type="pause_total_ns"}[5m])`
   - 显示: 时间序列图
   - 单位: ms/s

4. **数据库连接池使用率**
   - 指标: `database_connections{state="in_use"} / database_connections{state="open"}`
   - 显示: 百分比仪表
   - 告警阈值: 80%

5. **Redis 连接池健康度**
   - 指标: `redis_connections{state="idle_conns"}` / `redis_connections{state="total_conns"}`
   - 显示: 百分比仪表

## 故障排查指南

### Goroutine 数量持续增长

**可能原因:**
1. Fire-and-forget goroutines 未正确管理
2. Context 未正确传播
3. 等待通道的 goroutine 未被唤醒

**排查步骤:**
```bash
# 1. 查看 goroutine 数量
curl http://localhost:8000/metrics | grep app_goroutines

# 2. 启用 pprof（仅开发环境）
ENABLE_PPROF=true ./injective-explorer

# 3. 获取 goroutine profile
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/goroutine
```

### 内存持续增长

**可能原因:**
1. 对象池未正确归还
2. 缓存无限增长
3. 切片/Map 未清理

**排查步骤:**
```bash
# 1. 查看堆内存使用
curl http://localhost:8000/metrics | grep heap_alloc

# 2. 使用 pprof 分析内存
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/heap

# 3. 查看内存分配热点
go tool pprof -alloc_space http://localhost:6060/debug/pprof/heap
```

### 数据库连接池耗尽

**可能原因:**
1. 连接未正确释放
2. 长事务占用连接
3. 连接池配置过小

**排查步骤:**
```bash
# 1. 查看连接池使用情况
curl http://localhost:8000/metrics | grep database_connections

# 2. 检查 wait_count 是否增长
curl http://localhost:8000/metrics | grep wait_count

# 3. 查看当前数据库配置
SHOW VARIABLES LIKE 'max_connections';
SHOW STATUS LIKE 'Threads_connected';
```

## 定期健康检查

建议设置以下定期检查：

1. **每日检查**
   - Goroutine 数量趋势
   - 堆内存使用趋势
   - GC 频率和暂停时间

2. **每周检查**
   - 数据库连接池使用率峰值
   - Redis 连接池健康状况
   - 长期资源使用趋势

3. **每月检查**
   - 审查告警规则有效性
   - 优化监控阈值
   - 更新仪表板

## 改进建议

基于监控数据，可以考虑以下优化：

1. **如果连接池经常耗尽:**
   - 增加 `MaxOpenConns`
   - 减少 `ConnMaxLifetime`
   - 优化慢查询

2. **如果 GC 压力过大:**
   - 增加对象池使用
   - 减少小对象分配
   - 优化热点代码路径

3. **如果 Redis 未命中率高:**
   - 增加连接池大小
   - 优化缓存策略
   - 检查网络延迟
