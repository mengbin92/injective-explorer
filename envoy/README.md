# Envoy Proxy gRPC-Web 配置

本项目使用 Envoy Proxy 将前端的 gRPC-Web 请求转发到后端的 gRPC 服务。

## 架构说明

```
前端 (浏览器)
    ↓ gRPC-Web 请求
Envoy Proxy (端口 9001)
    ↓ 转换为 gRPC
后端 gRPC 服务 (端口 9000)
```

## 配置说明

### Envoy 配置文件 (`envoy.yaml`)

- **监听端口**: 9001 (接收 gRPC-Web 请求)
- **Admin 端口**: 9901 (Envoy 管理界面)
- **后端服务**: `injective-explorer:9000` (gRPC 服务)

### 主要功能

1. **gRPC-Web 支持**: 自动将 gRPC-Web 请求转换为标准 gRPC 请求
2. **CORS 支持**: 配置了跨域资源共享，允许所有来源（生产环境建议限制）
3. **超时设置**: 请求超时时间为 30 秒
4. **负载均衡**: 使用轮询（ROUND_ROBIN）策略

## 使用方法

### 启动服务

使用 docker-compose 一键启动所有服务（包括 Envoy）：

```bash
docker-compose up -d
```

### 验证 Envoy 运行状态

1. **检查 Envoy 健康状态**:
   ```bash
   curl http://localhost:9901/server_info
   ```

2. **查看 Envoy 配置**:
   ```bash
   curl http://localhost:9901/config_dump
   ```

3. **查看 Envoy 统计信息**:
   ```bash
   curl http://localhost:9901/stats
   ```

### 前端连接配置

前端需要将 gRPC-Web 客户端指向 Envoy 代理：

```javascript
// 示例：使用 @improbable-eng/grpc-web
const client = new ExplorerServiceClient('http://localhost:9001');
```

## 端口说明

- **8000**: HTTP API 服务
- **9000**: gRPC 服务（内部使用，不对外暴露）
- **9001**: Envoy gRPC-Web 代理（前端连接此端口）
- **9901**: Envoy Admin 管理端口

## 与自实现方案的对比

### 之前（自实现）
- 使用 `github.com/improbable-eng/grpc-web` 库
- 在 Go 代码中实现 gRPC-Web 转换
- 需要维护额外的代码

### 现在（Envoy）
- 使用成熟的 Envoy Proxy
- 配置简单，性能更好
- 支持更多高级功能（限流、监控、路由等）
- 与后端服务解耦

## 自定义配置

如果需要修改 Envoy 配置：

1. 编辑 `envoy/envoy.yaml`
2. 重启 Envoy 服务：
   ```bash
   docker-compose restart envoy
   ```

### 常见配置修改

#### 限制 CORS 来源

修改 `envoy.yaml` 中的 CORS 配置：

```yaml
allow_origin_string_match:
  - exact: "https://yourdomain.com"
  - exact: "https://app.yourdomain.com"
```

#### 修改超时时间

修改 `route_config` 中的 `timeout` 值：

```yaml
timeout: 60s  # 改为 60 秒
```

#### 添加多个后端服务

在 `clusters` 中添加更多端点：

```yaml
endpoints:
  - lb_endpoints:
      - endpoint:
          address:
            socket_address:
              address: injective-explorer
              port_value: 9000
      - endpoint:
          address:
            socket_address:
              address: injective-explorer-2
              port_value: 9000
```

## 故障排查

### Envoy 无法启动

1. 检查配置文件语法：
   ```bash
   docker-compose exec envoy envoy --config-path /etc/envoy/envoy.yaml --mode validate
   ```

2. 查看 Envoy 日志：
   ```bash
   docker-compose logs envoy
   ```

### 无法连接到后端服务

1. 确保 `injective-explorer` 服务已启动：
   ```bash
   docker-compose ps
   ```

2. 检查网络连接：
   ```bash
   docker-compose exec envoy ping injective-explorer
   ```

3. 检查 gRPC 服务是否正常：
   ```bash
   docker-compose exec injective-explorer netstat -tlnp | grep 9000
   ```

### CORS 问题

如果前端遇到 CORS 错误：

1. 检查 Envoy 配置中的 `allow_origin_string_match`
2. 查看浏览器控制台的错误信息
3. 检查请求头是否正确设置

## 监控和日志

### 查看 Envoy 日志

```bash
docker-compose logs -f envoy
```

### 查看 Envoy 统计信息

访问 `http://localhost:9901/stats` 查看详细的统计信息。

### 性能监控

Envoy 提供了丰富的性能指标，可以通过 Admin API 获取：
- 请求数量
- 响应时间
- 错误率
- 连接数等

## 生产环境建议

1. **限制 CORS 来源**: 不要使用 `*`，指定具体的域名
2. **启用 TLS**: 配置 HTTPS 加密
3. **设置资源限制**: 在 docker-compose 中设置内存和 CPU 限制
4. **日志轮转**: 配置日志文件大小和保留策略
5. **监控告警**: 集成 Prometheus 等监控系统

