# 横向扩展技术方案 - QPS 提升方案

## 1. 当前架构分析

### 1.1 现状
- **服务架构**: Go + Kratos 框架，HTTP (8000) + gRPC (9000)
- **部署方式**: 单实例 Docker Compose 部署
- **代理层**: Envoy (gRPC-Web) + Nginx (可选)
- **数据层**: MySQL + Redis
- **当前限制**: 
  - Envoy 只配置单个后端服务
  - 数据库连接池: MaxOpenConns=50, MaxIdleConns=15
  - Redis 连接池: pool_size=100
  - 无负载均衡配置

### 1.2 瓶颈分析
1. **应用层**: 单实例，无法水平扩展
2. **代理层**: Envoy 未配置多后端负载均衡
3. **数据库**: 连接池较小，可能成为瓶颈
4. **同步服务**: 多实例会重复同步（需要单实例运行）

## 2. 横向扩展方案

### 2.1 架构设计

```
                    ┌─────────────┐
                    │   Nginx     │  (可选，用于 SSL 终止)
                    │  (443/80)   │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Envoy     │  (负载均衡 + gRPC-Web)
                    │   (9001)    │
                    └──────┬──────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
  ┌─────▼─────┐    ┌─────▼─────┐    ┌─────▼─────┐
  │ Instance 1 │    │ Instance 2 │    │ Instance N │
  │ :8000/:9000│    │ :8000/:9000│    │ :8000/:9000│
  └─────┬─────┘    └─────┬─────┘    └─────┬─────┘
        │                │                  │
        └────────────────┼──────────────────┘
                         │
        ┌────────────────┼────────────────┐
        │                │                │
  ┌─────▼─────┐    ┌─────▼─────┐    ┌─────▼─────┐
  │   MySQL   │    │   Redis   │    │   MinIO   │
  │  (主从)   │    │ (集群/哨兵)│    │ (可选)    │
  └───────────┘    └───────────┘    └───────────┘
```

### 2.2 核心原则

1. **无状态服务**: API 服务完全无状态，可任意扩展
2. **共享存储**: Redis 和 MySQL 作为共享存储
3. **单实例同步**: 区块同步服务只在一个实例运行
4. **健康检查**: 所有实例配置健康检查
5. **优雅关闭**: 支持优雅关闭，避免请求中断

## 3. 实施步骤

### 3.1 阶段一：Envoy 负载均衡配置

#### 3.1.1 更新 Envoy 配置支持多后端

**文件**: `envoy/envoy.yaml`

```yaml
static_resources:
  clusters:
    - name: grpc_service
      connect_timeout: 0.25s
      type: logical_dns
      lb_policy: round_robin  # 轮询负载均衡
      # 或者使用 least_request: 最少连接数
      # lb_policy: least_request
      http2_protocol_options:
        max_concurrent_streams: 100
        initial_stream_window_size: 65536
        initial_connection_window_size: 1048576
      upstream_connection_options:
        tcp_keepalive:
          keepalive_probes: 3
          keepalive_time: 30
          keepalive_interval: 5
      load_assignment:
        cluster_name: grpc_service
        endpoints:
          - lb_endpoints:
              # 多个后端实例
              - endpoint:
                  address:
                    socket_address:
                      address: injective-explorer-1
                      port_value: 9000
              - endpoint:
                  address:
                    socket_address:
                      address: injective-explorer-2
                      port_value: 9000
              - endpoint:
                  address:
                    socket_address:
                      address: injective-explorer-3
                      port_value: 9000
      # 健康检查配置
      health_checks:
        - timeout: 1s
          interval: 10s
          unhealthy_threshold: 3
          healthy_threshold: 2
          grpc_health_check:
            service_name: ""
```

#### 3.1.2 HTTP 负载均衡配置

**方案一：使用 Nginx（推荐）**

**文件**: `envoy/nginx-http-lb.conf`

```nginx
upstream explorer_http {
    least_conn;  # 最少连接数负载均衡
    server injective-explorer-1:8000 max_fails=3 fail_timeout=30s;
    server injective-explorer-2:8000 max_fails=3 fail_timeout=30s;
    server injective-explorer-3:8000 max_fails=3 fail_timeout=30s;
    keepalive 32;
}

server {
    listen 80;
    location / {
        proxy_pass http://explorer_http;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
    }
}
```

**方案二：使用 Envoy（统一代理）**

如果希望使用 Envoy 同时处理 HTTP 和 gRPC，可以使用 `envoy/envoy-http-lb.yaml` 配置。

**访问方式**：
- HTTP API: `http://localhost:8000` （通过 Nginx 负载均衡）
- gRPC-Web: `http://localhost:9001` （通过 Envoy 负载均衡）

### 3.2 阶段二：Docker Compose 多实例配置

#### 3.2.1 创建生产环境多实例配置

**文件**: `deploy/docker-compose.scale.yml`

```yaml
version: '3.8'

services:
  # 实例 1
  injective-explorer-1:
    image: injective-explorer:${VERSION:-latest}
    container_name: injective-explorer-1
    restart: unless-stopped
    env_file:
      - .env.production
    environment:
      - INSTANCE_ID=1
      - ENABLE_SYNC=true  # 只有实例1运行同步服务
    ports:
      - "8001:8000"  # 不同端口避免冲突
      - "9001:9000"
    volumes:
      - ./configs:/data/conf:ro
    networks:
      - injective-explorer-network
    depends_on:
      - injective-explorer-mysql
      - injective-explorer-redis
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/api/v1/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G

  # 实例 2
  injective-explorer-2:
    image: injective-explorer:${VERSION:-latest}
    container_name: injective-explorer-2
    restart: unless-stopped
    env_file:
      - .env.production
    environment:
      - INSTANCE_ID=2
      - ENABLE_SYNC=false  # 其他实例不运行同步服务
    ports:
      - "8002:8000"
      - "9002:9000"
    volumes:
      - ./configs:/data/conf:ro
    networks:
      - injective-explorer-network
    depends_on:
      - injective-explorer-mysql
      - injective-explorer-redis
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/api/v1/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G

  # 实例 3
  injective-explorer-3:
    image: injective-explorer:${VERSION:-latest}
    container_name: injective-explorer-3
    restart: unless-stopped
    env_file:
      - .env.production
    environment:
      - INSTANCE_ID=3
      - ENABLE_SYNC=false
    ports:
      - "8003:8000"
      - "9003:9000"
    volumes:
      - ./configs:/data/conf:ro
    networks:
      - injective-explorer-network
    depends_on:
      - injective-explorer-mysql
      - injective-explorer-redis
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/api/v1/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G

  # Envoy 负载均衡器
  envoy:
    image: envoyproxy/envoy:v1.36-latest
    container_name: injective-explorer-envoy
    restart: unless-stopped
    ports:
      - "9001:9001"
      - "9901:9901"
    volumes:
      - ./envoy/envoy.yaml:/etc/envoy/envoy.yaml:ro
    networks:
      - injective-explorer-network
    depends_on:
      - injective-explorer-1
      - injective-explorer-2
      - injective-explorer-3
    command: /usr/local/bin/envoy -c /etc/envoy/envoy.yaml --service-cluster envoy --service-node envoy
    healthcheck:
      test: ["CMD-SHELL", "wget --spider -q http://localhost:9901/server_info || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3

  # Redis (可配置为集群或哨兵模式)
  injective-explorer-redis:
    image: redis:8.4
    container_name: injective-explorer-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - injective-explorer-redis:/data
    networks:
      - injective-explorer-network
    command: >
      redis-server
      --maxmemory 2gb
      --maxmemory-policy allkeys-lru
      --save 60 1000
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G

  # MySQL (建议配置主从)
  injective-explorer-mysql:
    image: mysql:8.0
    container_name: injective-explorer-mysql
    restart: unless-stopped
    ports:
      - "3306:3306"
    environment:
      - MYSQL_ROOT_PASSWORD=${DB_ROOT_PASSWORD}
      - MYSQL_DATABASE=${DB_NAME:-explorer}
      - MYSQL_CHARACTER_SET_SERVER=utf8mb4
      - MYSQL_COLLATION_SERVER=utf8mb4_unicode_ci
    volumes:
      - injective-explorer-mysql:/var/lib/mysql
      - ./database/my.cnf:/etc/mysql/conf.d/my.cnf:ro
    networks:
      - injective-explorer-network
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost", "-uroot", "-p${DB_ROOT_PASSWORD}"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 60s
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 4G

networks:
  injective-explorer-network:
    driver: bridge

volumes:
  injective-explorer-redis:
  injective-explorer-mysql:
```

### 3.3 阶段三：代码优化

#### 3.3.1 同步服务单实例控制

**修改**: `internal/service/sync.go`

添加基于 Redis 的分布式锁，确保只有一个实例运行同步服务：

```go
// 使用 Redis 分布式锁确保只有一个实例运行同步服务
func (s *SyncService) acquireSyncLock(ctx context.Context) (bool, error) {
    redisClient := cache.GetRedisClient()
    if redisClient == nil {
        return false, errors.New("redis client not available")
    }
    
    lockKey := "sync:lock:blockchain"
    lockValue := fmt.Sprintf("%s-%d", os.Hostname(), os.Getpid())
    lockTTL := 30 * time.Second
    
    // 尝试获取锁
    acquired, err := redisClient.SetNX(ctx, lockKey, lockValue, lockTTL).Result()
    if err != nil {
        return false, err
    }
    
    if acquired {
        // 启动续期 goroutine
        go s.renewSyncLock(ctx, lockKey, lockValue, lockTTL)
        return true, nil
    }
    
    return false, nil
}

func (s *SyncService) renewSyncLock(ctx context.Context, lockKey, lockValue string, ttl time.Duration) {
    ticker := time.NewTicker(ttl / 2) // 每 TTL/2 续期一次
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            redisClient := cache.GetRedisClient()
            if redisClient == nil {
                return
            }
            
            // 检查锁是否仍然属于当前实例
            currentValue, err := redisClient.Get(ctx, lockKey).Result()
            if err == redis.Nil || currentValue != lockValue {
                // 锁已丢失，停止续期
                return
            }
            
            // 续期
            redisClient.Expire(ctx, lockKey, ttl)
        }
    }
}
```

#### 3.3.2 数据库连接池优化

**修改**: `provider/db/db.go`

根据实例数量动态调整连接池大小：

```go
func Init(ctx context.Context, cfg *conf.Data_Database, logKratos log.Logger) error {
    // ... 现有代码 ...
    
    sqlDB, err := gdb.DB()
    if err != nil {
        return errors.Wrap(err, "get sql db error")
    }
    
    // 根据实例数量动态调整连接池
    // 假设 MySQL max_connections = 500
    // 每个实例使用: (500 * 0.6) / 实例数
    instanceCount := getInstanceCount() // 从环境变量或配置获取
    if instanceCount == 0 {
        instanceCount = 1 // 默认单实例
    }
    
    maxOpenConns := (500 * 60 / 100) / instanceCount  // 每个实例最多 300/实例数
    if maxOpenConns < 10 {
        maxOpenConns = 10 // 最小 10
    }
    if maxOpenConns > 100 {
        maxOpenConns = 100 // 最大 100
    }
    
    maxIdleConns := maxOpenConns / 3  // 空闲连接数 = 1/3 最大连接数
    if maxIdleConns < 5 {
        maxIdleConns = 5
    }
    
    sqlDB.SetMaxIdleConns(maxIdleConns)
    sqlDB.SetMaxOpenConns(maxOpenConns)
    sqlDB.SetConnMaxLifetime(5 * time.Minute)
    sqlDB.SetConnMaxIdleTime(3 * time.Minute)
    
    // ... 其余代码 ...
}
```

#### 3.3.3 健康检查端点

**修改**: `internal/server/http.go` 或创建健康检查服务

确保健康检查端点快速响应：

```go
// 注册健康检查路由
r.GET("/health", func(ctx khttp.Context) error {
    // 快速健康检查：只检查关键依赖
    checks := map[string]string{
        "status": "ok",
        "timestamp": time.Now().Format(time.RFC3339),
    }
    
    // 可选：检查数据库连接
    if db.Get() != nil {
        sqlDB, err := db.Get().DB()
        if err == nil {
            if err := sqlDB.PingContext(ctx.Request().Context()); err == nil {
                checks["database"] = "ok"
            } else {
                checks["database"] = "error"
            }
        }
    }
    
    // 可选：检查 Redis 连接
    if redisClient := cache.GetRedisClient(); redisClient != nil {
        if err := redisClient.Ping(ctx.Request().Context()).Err(); err == nil {
            checks["redis"] = "ok"
        } else {
            checks["redis"] = "error"
        }
    }
    
    ctx.Response().Header().Set("Content-Type", "application/json")
    return json.NewEncoder(ctx.Response()).Encode(checks)
})

r.GET("/api/v1/health", func(ctx khttp.Context) error {
    // 快速健康检查
    ctx.Response().Header().Set("Content-Type", "application/json")
    ctx.Response().WriteHeader(http.StatusOK)
    ctx.Response().Write([]byte(`{"status":"ok"}`))
    return nil
})
```

### 3.4 阶段四：配置优化

#### 3.4.1 MySQL 配置优化

**文件**: `database/my.cnf`

```ini
[mysqld]
# 连接数配置
max_connections = 500
max_connect_errors = 10000

# 缓冲池配置（根据内存调整）
innodb_buffer_pool_size = 2G
innodb_buffer_pool_instances = 4

# 日志配置
innodb_log_file_size = 256M
innodb_log_buffer_size = 64M

# 查询缓存（MySQL 8.0 已移除，但保留配置兼容性）
query_cache_type = 0

# 慢查询日志
slow_query_log = 1
slow_query_log_file = /var/log/mysql/slow.log
long_query_time = 1

# 连接超时
wait_timeout = 600
interactive_timeout = 600
```

#### 3.4.2 Redis 配置优化

**文件**: `database/redis.conf`

```conf
# 内存配置
maxmemory 2gb
maxmemory-policy allkeys-lru

# 持久化配置（根据需求选择）
save 900 1
save 300 10
save 60 10000

# 网络配置
tcp-backlog 511
timeout 0
tcp-keepalive 300

# 客户端配置
maxclients 10000
```

## 4. 性能优化建议

### 4.1 应用层优化

1. **连接池调优**
   - 数据库连接池：根据实例数动态调整
   - Redis 连接池：保持足够大小（100+）
   - HTTP 客户端连接池：复用连接

2. **缓存策略**
   - 热点数据缓存：区块、交易详情
   - 缓存预热：启动时加载热点数据
   - 缓存穿透防护：布隆过滤器或空值缓存

3. **异步处理**
   - 统计信息异步更新
   - 日志异步写入
   - 非关键路径异步处理

### 4.2 数据库优化

1. **索引优化**
   - 为常用查询字段添加索引
   - 定期分析慢查询日志
   - 使用覆盖索引减少回表

2. **读写分离**
   - 主库处理写操作
   - 从库处理读操作
   - 使用中间件（如 ProxySQL）自动路由

3. **分库分表**（长期方案）
   - 按区块高度分表
   - 按时间分库
   - 使用分片中间件

### 4.3 Redis 优化

1. **数据结构选择**
   - 使用 Hash 存储对象
   - 使用 Sorted Set 存储排行榜
   - 避免大 Key

2. **Pipeline 批量操作**
   - 批量读取减少网络往返
   - 批量写入提高吞吐量

3. **Redis 集群**（高可用方案）
   - 3 主 3 从配置
   - 自动故障转移
   - 数据分片

## 5. 监控和指标

### 5.1 关键指标

1. **应用指标**
   - QPS/TPS
   - 响应时间（P50/P95/P99）
   - 错误率
   - 并发连接数

2. **系统指标**
   - CPU 使用率
   - 内存使用率
   - 网络 I/O
   - 磁盘 I/O

3. **数据库指标**
   - 连接数
   - 慢查询数
   - QPS
   - 锁等待时间

4. **Redis 指标**
   - 连接数
   - 内存使用率
   - 命中率
   - 命令延迟

### 5.2 监控工具推荐

1. **Prometheus + Grafana**
   - 指标收集和可视化
   - 告警规则配置

2. **ELK Stack**
   - 日志收集和分析
   - 错误追踪

3. **APM 工具**
   - Jaeger（分布式追踪）
   - SkyWalking（应用性能监控）

## 6. 部署指南

### 6.1 快速部署（3 实例）

```bash
# 1. 构建镜像
docker build -t injective-explorer:latest .

# 2. 启动多实例
docker-compose -f deploy/docker-compose.scale.yml up -d

# 3. 检查状态
docker-compose -f deploy/docker-compose.scale.yml ps

# 4. 查看日志
docker-compose -f deploy/docker-compose.scale.yml logs -f injective-explorer-1
```

### 6.2 动态扩缩容

```bash
# 扩容到 5 个实例
docker-compose -f deploy/docker-compose.scale.yml up -d --scale injective-explorer=5

# 缩容到 2 个实例
docker-compose -f deploy/docker-compose.scale.yml up -d --scale injective-explorer=2
```

### 6.3 Kubernetes 部署（可选）

**文件**: `deploy/k8s/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: injective-explorer
spec:
  replicas: 3
  selector:
    matchLabels:
      app: injective-explorer
  template:
    metadata:
      labels:
        app: injective-explorer
    spec:
      containers:
      - name: injective-explorer
        image: injective-explorer:latest
        ports:
        - containerPort: 8000
        - containerPort: 9000
        env:
        - name: INSTANCE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: ENABLE_SYNC
          value: "false"  # 使用 initContainer 或 Job 运行同步服务
        resources:
          requests:
            memory: "1Gi"
            cpu: "1"
          limits:
            memory: "2Gi"
            cpu: "2"
        livenessProbe:
          httpGet:
            path: /api/v1/health
            port: 8000
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/v1/health
            port: 8000
          initialDelaySeconds: 10
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: injective-explorer
spec:
  selector:
    app: injective-explorer
  ports:
  - name: http
    port: 8000
    targetPort: 8000
  - name: grpc
    port: 9000
    targetPort: 9000
  type: ClusterIP
```

## 7. 预期 QPS 提升

### 7.1 理论计算

假设单实例 QPS = 1000：

| 实例数 | 理论 QPS | 实际 QPS（考虑开销） | 提升比例 |
| ------ | -------- | -------------------- | -------- |
| 1      | 1000     | 1000                 | 1x       |
| 2      | 2000     | 1800                 | 1.8x     |
| 3      | 3000     | 2400                 | 2.4x     |
| 5      | 5000     | 3500                 | 3.5x     |
| 10     | 10000    | 6000                 | 6x       |

**注意**：实际 QPS 受限于：
- 数据库连接数和性能
- Redis 性能
- 网络带宽
- 业务逻辑复杂度

### 7.2 瓶颈分析

1. **数据库瓶颈**（最常见）
   - 解决方案：读写分离、连接池优化、索引优化

2. **Redis 瓶颈**
   - 解决方案：Redis 集群、Pipeline 优化

3. **网络瓶颈**
   - 解决方案：CDN、多地域部署

4. **CPU/内存瓶颈**
   - 解决方案：垂直扩展、代码优化

## 8. 故障处理

### 8.1 实例故障

1. **自动故障转移**
   - Envoy/Nginx 健康检查自动剔除故障实例
   - 新请求路由到健康实例

2. **手动处理**
   ```bash
   # 停止故障实例
   docker stop injective-explorer-1
   
   # 重启实例
   docker restart injective-explorer-1
   ```

### 8.2 数据库故障

1. **主从切换**
   - 自动切换到从库（需要配置）
   - 或手动切换

2. **连接池耗尽**
   - 检查慢查询
   - 优化查询
   - 增加连接数

### 8.3 Redis 故障

1. **降级策略**
   - 限流降级：允许所有请求
   - 缓存降级：直接查询数据库

2. **Redis 集群故障**
   - 自动故障转移（哨兵模式）
   - 手动切换主节点

## 9. 最佳实践

### 9.1 部署建议

1. **渐进式扩展**
   - 先扩展到 2-3 个实例
   - 监控性能指标
   - 根据实际需求逐步扩展

2. **资源预留**
   - 保留 20-30% 的资源余量
   - 应对突发流量

3. **灰度发布**
   - 新版本先部署到 1 个实例
   - 验证无问题后全量发布

### 9.2 运维建议

1. **定期检查**
   - 每日检查慢查询日志
   - 每周分析性能指标
   - 每月评估容量规划

2. **备份策略**
   - 数据库每日全量备份
   - Redis 定期 RDB 备份
   - 配置文件版本控制

3. **文档维护**
   - 更新部署文档
   - 记录故障处理经验
   - 分享最佳实践

## 10. 总结

通过横向扩展方案，可以实现：

1. **QPS 提升**：3 实例可提升 2-3 倍，10 实例可提升 5-6 倍
2. **高可用性**：单实例故障不影响服务
3. **弹性扩展**：根据负载动态调整实例数
4. **资源优化**：合理分配资源，提高利用率

**关键成功因素**：
- 无状态服务设计
- 共享存储（Redis + MySQL）
- 负载均衡配置
- 健康检查和监控
- 渐进式扩展策略

**下一步行动**：
1. 实施 Envoy 负载均衡配置
2. 部署多实例测试环境
3. 压力测试验证性能提升
4. 生产环境灰度发布
5. 持续监控和优化