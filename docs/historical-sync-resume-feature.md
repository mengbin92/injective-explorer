# 历史区块同步断点续传功能

## 问题描述

在之前的实现中，历史区块同步的进度（`historicalSyncHeight`）只存储在内存中。当服务重启时，这个内存变量会丢失，导致：

1. 服务重启后会从配置的 `SYNC_BLOCKS_EARLIEST_HEIGHT` 重新开始同步
2. 即使已经同步了数千个区块，重启后又会重新同步这些区块
3. 浪费时间和资源，降低同步效率

## 解决方案

将历史区块同步进度持久化到 Redis 中，这样服务重启后可以恢复进度，从上次中断的地方继续同步。

## 实现细节

### 1. 新增 Redis 缓存键

在 `internal/service/constants.go` 中新增了常量：

```go.go
CacheKeyHistoricalSyncHeight = "sync:historical:block:height"  // Historical block sync height persistence key
```

### 2. 修改同步进度管理函数

修改了 `internal/service/sync.go` 中的函数：

#### `getHistoricalSyncHeight()`
- 首先尝试从 Redis 加载持久化的同步进度
- 如果 Redis 中有值，更新内存中的值
- 返回当前同步高度

#### `setHistoricalSyncHeight(height uint64)`
- 更新内存中的同步高度
- 将同步高度持久化到 Redis（无过期时间）
- 确保服务重启后可以恢复进度

#### `clearHistoricalSyncHeight()` (新增)
- 清除 Redis 中持久化的同步进度
- 用于需要重新从头开始同步的场景

### 3. 修改历史区块同步初始化逻辑

在 `historicalBlockSync()` 函数中：

1. 启动时首先尝试从 Redis 恢复持久化的同步进度
2. 如果 Redis 中有持久化的进度，从该高度继续同步
3. 如果没有持久化的进度，按照原有逻辑确定起始高度
4. 每次更新同步进度时，同时更新 Redis 中的值

## 使用说明

### 正常使用

服务重启后会自动从上次中断的地方继续同步，无需任何额外操作。

日志示例：

```
historical block sync: recovered persisted sync height 6000 from Redis
historical block sync: resuming from persisted height 6000
```

### 重置同步进度

如果需要重新从头开始同步历史区块，可以使用以下方法：

#### 方法 1: 清除 Redis 中的同步进度

```bash
# 连接到 Redis
redis-cli

# 清除历史同步进度
DEL sync:historical:block:height
```

#### 方法 2: 重启服务

清除 Redis 中的同步进度后，重启服务即可从 `SYNC_BLOCKS_EARLIEST_HEIGHT` 重新开始同步。

## 配置说明

同步起始高度由配置文件中的 `sync.blocks.earliest_height` 参数控制：

```yaml
sync:
  blocks:
    earliest_height: ${SYNC_BLOCKS_EARLIEST_HEIGHT:1}  # 默认从区块 1 开始
```

可以通过环境变量覆盖：

```bash
export SYNC_BLOCKS_EARLIEST_HEIGHT=1000
```

## 技术优势

1. **断点续传**: 服务重启后自动恢复进度，避免重复同步
2. **高效持久化**: 使用 Redis 存储，读写速度快
3. **无过期时间**: 同步进度永久保存，直到手动清除
4. **向后兼容**: 如果 Redis 中没有持久化的进度，按照原有逻辑工作
5. **线程安全**: 使用互斥锁保护同步高度的读写操作

## 注意事项

1. **Redis 可用性**: 此功能依赖 Redis，如果 Redis 不可用，服务仍然可以运行，但无法持久化同步进度
2. **多实例部署**: 在多实例部署场景中，只有一个实例会执行历史区块同步（通过分布式锁控制），其他实例不会影响同步进度
3. **数据一致性**: 同步进度保存在 Redis 中，如果 Redis 数据丢失，服务会从数据库中的最小区块高度开始同步

## 测试建议

1. **正常重启测试**:
   - 启动服务，等待同步到某个高度（例如 6000）
   - 重启服务
   - 验证服务从 6000 继续同步，而不是从 1 开始

2. **重置测试**:
   - 启动服务，等待同步到某个高度
   - 清除 Redis 中的 `sync:historical:block:height`
   - 重启服务
   - 验证服务从 `SYNC_BLOCKS_EARLIEST_HEIGHT` 重新开始同步

3. **Redis 故障测试**:
   - 停止 Redis
   - 启动服务
   - 验证服务可以正常运行（但无法持久化进度）
   - 重启 Redis
   - 验证后续的同步进度可以正常持久化
