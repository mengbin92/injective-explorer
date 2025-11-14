# gRPC-Web 和 gRPC-Web-Stream 迁移方案评估

## 项目现状分析

### 当前架构
- **框架**: Kratos v2.8.0
- **接口类型**: HTTP REST + gRPC
- **API 定义**: Protocol Buffers (demo.proto)
- **数据同步**: 后台 SyncService 持续同步区块链数据到数据库
- **前端交互**: 前端需要主动调用接口获取数据更新

### 当前问题
1. 数据更新需要前端主动轮询或调用接口
2. 无法实现实时数据推送
3. 前端需要维护轮询逻辑，增加复杂度

### 技术基础
- ✅ 已有 `github.com/improbable-eng/grpc-web` 依赖（间接依赖）
- ✅ 已有完整的 Protocol Buffers 定义
- ✅ 已有 gRPC 服务实现
- ❌ 目前没有流式接口（streaming）

---

## 方案一：在现有项目基础上新增 gRPC-Web 支持

### 工作量评估

#### 1. 服务端改造（预计 3-5 天）

**1.1 添加 gRPC-Web 服务器支持**
- 在 `internal/server/` 中新增 `grpcweb.go`
- 集成 `grpc-web` 中间件到 HTTP 服务器
- 配置 CORS 和跨域支持
- **工作量**: 1-2 天

**1.2 添加流式接口定义**
- 在 `api/explorer/v1/demo.proto` 中新增 streaming RPC
  ```protobuf
  rpc StreamLatestBlocks(StreamLatestBlocksRequest) returns (stream BlockUpdate) {
    option (google.api.http) = {
      get: "/demo/stream/blocks"
    };
  }
  
  rpc StreamLatestTransactions(StreamLatestTransactionsRequest) returns (stream TransactionUpdate) {
    option (google.api.http) = {
      get: "/demo/stream/transactions"
    };
  }
  
  rpc StreamAccountUpdates(StreamAccountUpdatesRequest) returns (stream AccountUpdate) {
    option (google.api.http) = {
      get: "/demo/stream/account"
    };
  }
  ```
- **工作量**: 0.5 天

**1.3 实现流式服务方法**
- 在 `internal/service/demo.go` 中实现 streaming 方法
- 需要与 SyncService 集成，监听数据变更事件
- 实现事件发布/订阅机制（可使用 channel 或 Redis pub/sub）
- **工作量**: 2-3 天

**1.4 配置更新**
- 更新 `internal/conf/conf.proto` 添加 gRPC-Web 配置
- 更新配置文件示例
- **工作量**: 0.5 天

#### 2. 前端改造（预计 2-3 天）

**2.1 引入 gRPC-Web 客户端库**
- 安装 `@grpc/grpc-web` 或 `@improbable-eng/grpc-web`
- 生成 TypeScript/JavaScript 客户端代码
- **工作量**: 0.5 天

**2.2 实现流式数据订阅**
- 创建 gRPC-Web 客户端连接
- 实现流式数据接收和处理
- 替换现有的轮询逻辑
- **工作量**: 1.5-2 天

**2.3 错误处理和重连机制**
- 实现连接断开重连
- 错误处理和降级策略
- **工作量**: 0.5-1 天

#### 3. 测试和文档（预计 1-2 天）
- 单元测试
- 集成测试
- API 文档更新
- **工作量**: 1-2 天

**总工作量**: 6-10 天

---

### 优点

1. **渐进式改造**
   - 不影响现有 HTTP 和 gRPC 接口
   - 可以逐步迁移，降低风险
   - 新旧接口可以并存

2. **向后兼容**
   - 保留现有 HTTP 接口，确保兼容性
   - 第三方服务仍可使用 HTTP 接口
   - 前端可以逐步迁移

3. **风险可控**
   - 出现问题可以快速回退
   - 不影响现有功能
   - 可以分阶段上线

4. **灵活性高**
   - 可以根据实际使用情况决定是否完全迁移
   - 不同客户端可以使用不同接口

### 缺点

1. **维护成本**
   - 需要同时维护 HTTP、gRPC 和 gRPC-Web 三套接口
   - 代码复杂度增加
   - 需要维护多套文档

2. **资源消耗**
   - 需要同时运行多个服务器
   - 内存和 CPU 占用增加

3. **代码重复**
   - 可能存在部分逻辑重复
   - 需要确保多套接口行为一致

---

## 方案二：直接重构项目，全面采用 gRPC-Web

### 工作量评估

#### 1. 服务端重构（预计 5-7 天）

**1.1 移除 HTTP 服务器**
- 删除 `internal/server/http.go` 中的 HTTP 服务器代码
- 移除 HTTP 相关的配置和依赖
- **工作量**: 0.5 天

**1.2 重构为纯 gRPC-Web**
- 将所有 HTTP 接口迁移到 gRPC-Web
- 移除 `google.api.http` 注解（或保留用于文档）
- **工作量**: 1-2 天

**1.3 添加流式接口**
- 同方案一，添加 streaming RPC
- 实现流式服务方法
- **工作量**: 2-3 天

**1.4 重构服务层**
- 统一服务实现，移除 HTTP 特定逻辑
- 简化代码结构
- **工作量**: 1-2 天

#### 2. 前端全面重构（预计 3-5 天）

**2.1 完全迁移到 gRPC-Web**
- 移除所有 HTTP 请求代码
- 重写所有 API 调用逻辑
- **工作量**: 2-3 天

**2.2 实现流式数据订阅**
- 同方案一
- **工作量**: 1-2 天

#### 3. 测试和文档（预计 2-3 天）
- 全面测试
- 更新所有文档
- 迁移指南编写
- **工作量**: 2-3 天

**总工作量**: 10-15 天

---

### 优点

1. **技术栈统一**
   - 前后端统一使用 gRPC
   - 接口定义更清晰
   - 减少技术栈复杂度

2. **性能提升**
   - gRPC 基于 HTTP/2 和 Protobuf
   - 更小的消息体积
   - 更好的压缩和序列化性能
   - 支持多路复用

3. **代码简化**
   - 移除 HTTP 相关代码
   - 减少维护成本
   - 代码更清晰

4. **更好的类型安全**
   - Protocol Buffers 提供强类型
   - 前后端类型一致
   - 减少类型错误

### 缺点

1. **高风险**
   - 全面重构可能引入新问题
   - 影响现有功能稳定性
   - 回退困难

2. **兼容性问题**
   - 移除 HTTP 接口可能导致第三方服务不兼容
   - 需要通知所有依赖方
   - 可能需要提供兼容层

3. **学习曲线**
   - 团队需要学习 gRPC-Web
   - 前端开发需要适应新的调用方式
   - 调试工具和生态不如 HTTP 成熟

4. **浏览器限制**
   - 某些旧浏览器可能不支持
   - 需要 polyfill 或降级方案

---

## 技术实现细节

### gRPC-Web 集成方案

#### 方案 A: 使用 Envoy 代理（推荐用于生产环境）
```
浏览器 -> Envoy (gRPC-Web) -> gRPC Server
```
- 优点: 生产级稳定性，功能完整
- 缺点: 需要额外部署 Envoy

#### 方案 B: 使用 grpc-web-go 中间件（适合当前项目）
```go
import (
    "github.com/improbable-eng/grpc-web/go/grpcweb"
)

func NewGRPCWebServer(grpcServer *grpc.Server) *http.Server {
    wrappedGrpc := grpcweb.WrapServer(grpcServer)
    
    httpServer := &http.Server{
        Handler: http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
            if wrappedGrpc.IsGrpcWebRequest(req) {
                wrappedGrpc.ServeHTTP(resp, req)
            } else {
                // 处理其他请求或返回 404
            }
        }),
    }
    
    return httpServer
}
```

### 流式接口实现示例

```go
func (s *DemoService) StreamLatestBlocks(req *pb.StreamLatestBlocksRequest, stream pb.Demo_StreamLatestBlocksServer) error {
    // 订阅区块更新事件
    blockChan := make(chan *models.Block, 100)
    
    // 注册监听器（与 SyncService 集成）
    syncService.RegisterBlockListener(blockChan)
    defer syncService.UnregisterBlockListener(blockChan)
    
    for {
        select {
        case block := <-blockChan:
            // 转换为 protobuf 并发送
            blockProto := models.ToBlockProto(block)
            if err := stream.Send(blockProto); err != nil {
                return err
            }
        case <-stream.Context().Done():
            return stream.Context().Err()
        }
    }
}
```

### 前端使用示例

```typescript
import { BlockUpdate } from './generated/demo_pb';
import { DemoClient } from './generated/demo_grpc_web_pb';

const client = new DemoClient('http://localhost:8000');

const stream = client.streamLatestBlocks(new StreamLatestBlocksRequest(), {});

stream.on('data', (block: BlockUpdate) => {
  // 处理新的区块数据
  updateUI(block);
});

stream.on('error', (err) => {
  // 处理错误
  console.error('Stream error:', err);
  // 实现重连逻辑
});
```

---

## 推荐方案

### 建议采用方案一：在现有项目基础上新增

**理由：**

1. **风险更低**
   - 不影响现有功能
   - 可以逐步验证和优化
   - 出现问题可以快速回退

2. **工作量更少**
   - 预计 6-10 天 vs 10-15 天
   - 可以分阶段实施
   - 不需要大规模重构

3. **更灵活**
   - 可以根据实际使用情况决定是否完全迁移
   - 保留 HTTP 接口作为备选
   - 支持渐进式迁移

4. **符合当前项目状态**
   - 项目已有完整的 HTTP 接口
   - 可能有第三方依赖
   - 团队可能更熟悉 HTTP

### 实施建议

#### 第一阶段：基础支持（2-3 天）
1. 集成 gRPC-Web 服务器
2. 添加一个简单的流式接口进行验证
3. 前端实现基础连接测试

#### 第二阶段：核心功能（3-4 天）
1. 实现主要的流式接口（区块、交易、账户）
2. 与 SyncService 集成
3. 前端实现完整的数据订阅

#### 第三阶段：优化和完善（1-2 天）
1. 错误处理和重连机制
2. 性能优化
3. 文档和测试

#### 第四阶段（可选）：完全迁移
- 如果 gRPC-Web 使用良好，可以考虑移除 HTTP 接口
- 但建议保留一段时间作为备选

---

## 注意事项

1. **浏览器兼容性**
   - 确保目标浏览器支持 gRPC-Web
   - 准备降级方案（如 WebSocket 或轮询）

2. **网络环境**
   - 某些网络环境可能限制 HTTP/2
   - 需要测试各种网络条件

3. **调试工具**
   - gRPC-Web 的调试工具不如 HTTP 成熟
   - 需要准备相应的调试方案

4. **性能监控**
   - 监控流式连接的性能
   - 注意连接数和资源消耗

5. **安全性**
   - 确保 CORS 配置正确
   - 注意认证和授权机制

---

## 总结

| 对比项 | 方案一：新增 | 方案二：重构 |
|--------|------------|------------|
| **工作量** | 6-10 天 | 10-15 天 |
| **风险** | 低 | 高 |
| **兼容性** | 高（保留 HTTP） | 低（移除 HTTP） |
| **维护成本** | 中（多套接口） | 低（单一接口） |
| **性能** | 相同 | 相同 |
| **灵活性** | 高 | 低 |
| **推荐度** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |

**最终建议：采用方案一，在现有项目基础上新增 gRPC-Web 和 gRPC-Web-Stream 支持。**
