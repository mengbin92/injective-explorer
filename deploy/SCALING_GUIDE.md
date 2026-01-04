# 横向扩展快速部署指南

## 概述

本指南介绍如何将 injective-explorer 从单实例部署扩展到多实例部署，以提升 QPS 和可用性。

## 架构说明

### 单实例架构（当前）
```
Client -> Envoy -> injective-explorer (单实例) -> MySQL/Redis
```

### 多实例架构（扩展后）
```
Client -> Envoy (负载均衡) -> [injective-explorer-1, injective-explorer-2, injective-explorer-3] -> MySQL/Redis
```

## 快速开始

### 1. 准备配置文件

确保以下文件存在：
- `deploy/docker-compose.scale.yml` - 多实例 Docker Compose 配置
- `envoy/envoy.scale.yaml` - Envoy 负载均衡配置
- `.env.production` - 生产环境变量配置

### 2. 构建镜像

```bash
# 构建应用镜像
docker build -t injective-explorer:latest .
```

### 3. 启动多实例服务

```bash
# 启动 3 个实例
docker-compose -f deploy/docker-compose.scale.yml up -d

# 查看服务状态
docker-compose -f deploy/docker-compose.scale.yml ps

# 查看日志
docker-compose -f deploy/docker-compose.scale.yml logs -f injective-explorer-1
```

### 4. 验证部署

```bash
# 检查健康状态
curl http://localhost:8001/api/v1/health  # 实例 1
curl http://localhost:8002/api/v1/health  # 实例 2
curl http://localhost:8003/api/v1/health  # 实例 3

# 检查 Envoy 负载均衡
curl http://localhost:9001/  # 通过 Envoy 访问
```

## 关键配置说明

### 1. 同步服务控制

**环境变量方式**（推荐）：
```yaml
environment:
  - ENABLE_SYNC=true   # 实例 1：运行同步服务
  - ENABLE_SYNC=false  # 实例 2,3：不运行同步服务
```

**分布式锁方式**（自动）：
- 即使多个实例设置 `ENABLE_SYNC=true`，也只有一个实例能获得锁并运行同步服务
- 锁基于 Redis，自动续期，实例停止时自动释放

### 2. 负载均衡策略

**Envoy 配置** (`envoy/envoy.scale.yaml`)：
- `lb_policy: round_robin` - 轮询负载均衡
- 可选：`lb_policy: least_request` - 最少连接数

**健康检查**：
- 自动检测不健康的实例
- 自动从负载均衡中移除故障实例

### 3. 数据库连接池

**当前配置**：
- `MaxOpenConns: 50` - 每个实例最大连接数
- `MaxIdleConns: 15` - 每个实例空闲连接数

**建议调整**（根据实例数）：
- 3 实例：每个实例 `MaxOpenConns: 30`（总计 90，MySQL max_connections 建议 150+）
- 5 实例：每个实例 `MaxOpenConns: 20`（总计 100，MySQL max_connections 建议 200+）

## 扩缩容操作

### 扩容

```bash
# 方式 1：使用 docker-compose scale（需要修改配置）
# 编辑 deploy/docker-compose.scale.yml，添加更多实例

# 方式 2：手动添加实例
docker run -d \
  --name injective-explorer-4 \
  --network injective-explorer-scale-network \
  -e INSTANCE_ID=4 \
  -e ENABLE_SYNC=false \
  injective-explorer:latest
```

### 缩容

```bash
# 停止并删除实例
docker stop injective-explorer-3
docker rm injective-explorer-3

# 更新 Envoy 配置，移除该实例
# 编辑 envoy/envoy.scale.yaml，删除对应 endpoint
docker restart injective-explorer-envoy-scale
```

## 监控和调试

### 1. 查看实例状态

```bash
# 查看所有实例
docker ps | grep injective-explorer

# 查看实例资源使用
docker stats injective-explorer-1 injective-explorer-2 injective-explorer-3
```

### 2. 查看日志

```bash
# 查看特定实例日志
docker logs -f injective-explorer-1

# 查看所有实例日志
docker-compose -f deploy/docker-compose.scale.yml logs -f
```

### 3. 检查同步服务状态

```bash
# 检查哪个实例持有同步锁
docker exec injective-explorer-redis-scale redis-cli GET sync:lock:blockchain

# 查看同步服务日志（应该在实例 1）
docker logs injective-explorer-1 | grep -i sync
```

### 4. 性能测试

```bash
# 使用 ab 工具测试
ab -n 10000 -c 100 http://localhost:9001/api/v1/health

# 使用 wrk 工具测试
wrk -t4 -c100 -d30s http://localhost:9001/api/v1/health
```

## 故障处理

### 实例故障

1. **自动处理**：
   - Envoy 健康检查自动检测故障
   - 自动从负载均衡中移除故障实例
   - 请求自动路由到健康实例

2. **手动处理**：
   ```bash
   # 重启故障实例
   docker restart injective-explorer-1
   
   # 如果无法恢复，移除实例
   docker stop injective-explorer-1
   docker rm injective-explorer-1
   ```

### 同步服务故障

如果运行同步服务的实例故障：

1. **自动恢复**：
   - 锁会在 30 秒后过期
   - 其他实例可以自动获取锁并启动同步服务

2. **手动恢复**：
   ```bash
   # 释放锁（如果需要）
   docker exec injective-explorer-redis-scale redis-cli DEL sync:lock:blockchain
   
   # 重启实例或手动设置 ENABLE_SYNC=true
   ```

### 数据库连接耗尽

```bash
# 检查 MySQL 连接数
docker exec injective-explorer-mysql-scale mysql -uroot -p -e "SHOW STATUS LIKE 'Threads_connected';"

# 检查最大连接数
docker exec injective-explorer-mysql-scale mysql -uroot -p -e "SHOW VARIABLES LIKE 'max_connections';"
```

## 性能优化建议

### 1. 数据库优化

```sql
-- 增加最大连接数
SET GLOBAL max_connections = 500;

-- 优化 InnoDB 缓冲池
SET GLOBAL innodb_buffer_pool_size = 2147483648; -- 2GB
```

### 2. Redis 优化

```bash
# 检查 Redis 内存使用
docker exec injective-explorer-redis-scale redis-cli INFO memory

# 检查连接数
docker exec injective-explorer-redis-scale redis-cli INFO clients
```

### 3. 应用优化

- 调整连接池大小（根据实例数）
- 启用缓存预热
- 优化慢查询

## 预期性能提升

| 实例数 | 理论 QPS | 实际 QPS | 提升比例 |
|--------|----------|----------|----------|
| 1      | 1000     | 1000     | 1x       |
| 2      | 2000     | 1800     | 1.8x     |
| 3      | 3000     | 2400     | 2.4x     |
| 5      | 5000     | 3500     | 3.5x     |
| 10     | 10000    | 6000     | 6x       |

**注意**：实际性能受限于数据库、Redis 和网络带宽。

## 生产环境建议

1. **使用 Kubernetes**：
   - 自动扩缩容
   - 滚动更新
   - 更好的资源管理

2. **数据库主从**：
   - 主库处理写操作
   - 从库处理读操作
   - 提高可用性

3. **Redis 集群**：
   - 高可用性
   - 数据分片
   - 自动故障转移

4. **监控告警**：
   - Prometheus + Grafana
   - 关键指标告警
   - 日志聚合分析

## 常见问题

### Q: 如何确保只有一个实例运行同步服务？

A: 两种方式：
1. 环境变量：设置 `ENABLE_SYNC=true` 只在一个实例
2. 分布式锁：自动确保只有一个实例获得锁（即使多个实例设置 `ENABLE_SYNC=true`）

### Q: 如何添加更多实例？

A: 
1. 复制 `injective-explorer-3` 配置，创建 `injective-explorer-4`
2. 更新 Envoy 配置，添加新实例 endpoint
3. 重启 Envoy 服务

### Q: 如何实现零停机部署？

A:
1. 先启动新版本实例
2. 等待健康检查通过
3. 逐步停止旧版本实例
4. 使用滚动更新策略

### Q: 数据库连接数如何计算？

A: 
- 总连接数 = 实例数 × 每实例 MaxOpenConns
- MySQL max_connections 应 >= 总连接数 × 1.2（保留余量）
- 例如：3 实例 × 30 = 90，MySQL max_connections 建议 150+

## 参考文档

- [横向扩展技术方案](./docs/horizontal-scaling-plan.md)
- [部署文档](./deploy.md)
- [性能优化文档](./docs/performance-bottleneck-analysis.md)

