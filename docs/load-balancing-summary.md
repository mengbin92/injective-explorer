# 负载均衡配置总结

## 当前架构

```
客户端请求
    │
    ├─ HTTP API (8000) ──> Nginx 负载均衡 ──> [实例1, 实例2, 实例3]
    │
    └─ gRPC-Web (9001) ──> Envoy 负载均衡 ──> [实例1, 实例2, 实例3]
```

## HTTP 负载均衡

### 配置方式

**使用 Nginx**（当前配置）：
- 配置文件：`envoy/nginx-http-lb.conf`
- 监听端口：`8000`
- 负载均衡策略：`least_conn`（最少连接数）
- 健康检查：自动检测故障实例并移除

**访问方式**：
```bash
# HTTP API 通过 Nginx 负载均衡访问
curl http://localhost:8000/api/v1/health

# 直接访问实例（不推荐，仅用于调试）
curl http://localhost:8001/api/v1/health  # 实例1
curl http://localhost:8002/api/v1/health  # 实例2
curl http://localhost:8003/api/v1/health  # 实例3
```

### 负载均衡策略

当前使用 `least_conn`（最少连接数），可选策略：

1. **least_conn**（当前）：将请求分发到连接数最少的服务器
   - 适合：请求处理时间差异较大的场景
   - 配置：`least_conn;`

2. **round_robin**：轮询分发请求
   - 适合：服务器性能相近的场景
   - 配置：`round_robin;` 或删除 `least_conn;`

3. **ip_hash**：基于客户端 IP 的会话保持
   - 适合：需要会话保持的场景
   - 配置：`ip_hash;`

### 健康检查

Nginx 自动检测后端实例健康状态：
- `max_fails=3`：连续失败 3 次后标记为不健康
- `fail_timeout=30s`：30 秒后重新尝试
- 不健康的实例自动从负载均衡中移除

## gRPC 负载均衡

### 配置方式

**使用 Envoy**：
- 配置文件：`envoy/envoy.scale.yaml`
- 监听端口：`9001`
- 负载均衡策略：`round_robin`（轮询）
- 健康检查：gRPC 健康检查

**访问方式**：
```bash
# gRPC-Web 通过 Envoy 负载均衡访问
curl http://localhost:9001/

# 直接访问实例（不推荐，仅用于调试）
# 注意：gRPC 端口不直接暴露，需要通过 Envoy
```

### 负载均衡策略

当前使用 `round_robin`，可选策略：

1. **round_robin**（当前）：轮询分发
   - 配置：`lb_policy: round_robin`

2. **least_request**：最少请求数
   - 配置：`lb_policy: least_request`

3. **ring_hash**：基于一致性哈希
   - 适合：需要会话保持的场景
   - 配置：`lb_policy: ring_hash`

## 统一入口方案（可选）

如果希望使用 Envoy 同时处理 HTTP 和 gRPC，可以使用 `envoy/envoy-http-lb.yaml`：

```yaml
# HTTP 监听器：8000
# gRPC 监听器：9001
# 统一通过 Envoy 代理
```

**优点**：
- 统一代理，配置集中
- 支持 HTTP/2 和 gRPC

**缺点**：
- 配置相对复杂
- Nginx 对 HTTP 支持更成熟

## 验证负载均衡

### 1. 检查 HTTP 负载均衡

```bash
# 多次请求，观察请求分发到不同实例
for i in {1..10}; do
  curl -s http://localhost:8000/api/v1/health | jq .
done

# 查看 Nginx 日志
docker logs injective-explorer-nginx-lb
```

### 2. 检查 gRPC 负载均衡

```bash
# 查看 Envoy 统计信息
curl http://localhost:9901/stats | grep cluster.grpc_service

# 查看健康检查状态
curl http://localhost:9901/clusters | grep grpc_service
```

### 3. 测试故障转移

```bash
# 停止一个实例
docker stop injective-explorer-2

# 验证请求仍然可以正常处理
curl http://localhost:8000/api/v1/health

# 重启实例
docker start injective-explorer-2
```

## 性能优化建议

### 1. 连接池配置

**Nginx**：
```nginx
keepalive 32;  # 保持 32 个空闲连接
```

**Envoy**：
```yaml
upstream_connection_options:
  tcp_keepalive:
    keepalive_probes: 3
    keepalive_time: 30
    keepalive_interval: 5
```

### 2. 超时配置

**Nginx**：
```nginx
proxy_connect_timeout 30s;
proxy_send_timeout 30s;
proxy_read_timeout 30s;
```

**Envoy**：
```yaml
timeout: 30s
```

### 3. 缓冲配置

**Nginx**：
```nginx
proxy_buffering on;
proxy_buffer_size 4k;
proxy_buffers 8 4k;
proxy_busy_buffers_size 8k;
```

## 常见问题

### Q: HTTP 请求会负载均衡吗？

A: **是的**，通过 Nginx 负载均衡器（端口 8000）统一入口，请求会自动分发到多个实例。

### Q: 如何查看请求分发到哪个实例？

A: 
1. 查看 Nginx 访问日志：`docker logs injective-explorer-nginx-lb`
2. 查看应用日志：每个实例的日志会显示处理的请求
3. 在响应头中添加实例标识（需要代码支持）

### Q: 如何添加更多实例？

A:
1. 在 `docker-compose.scale.yml` 中添加新实例配置
2. 更新 Nginx 配置，添加新实例到 upstream
3. 更新 Envoy 配置，添加新实例到 cluster
4. 重启负载均衡器

### Q: 负载均衡策略如何选择？

A:
- **least_conn**：适合请求处理时间差异大
- **round_robin**：适合服务器性能相近
- **ip_hash**：适合需要会话保持

### Q: 如何实现会话保持？

A:
1. 使用 `ip_hash` 策略（Nginx）
2. 使用 `ring_hash` 策略（Envoy）
3. 使用 Cookie 或 Header 进行会话标识

## 参考文档

- [横向扩展技术方案](./horizontal-scaling-plan.md)
- [快速部署指南](../deploy/SCALING_GUIDE.md)
- [Nginx 官方文档](https://nginx.org/en/docs/)
- [Envoy 官方文档](https://www.envoyproxy.io/docs)

