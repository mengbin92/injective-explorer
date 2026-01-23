# EVM交易同步改进方案

## 问题描述

1. **EVM交易的signatures和signer不能即时获得**：
   - 通过go-ethereum发送的EVM交易，在解析入库时signatures和signer不能像Injective原生交易那样即时获得
   - 导致查询时很多信息都有延迟

2. **EVM信息同步失败时会把交易清除**：
   - 测试发现evm信息同步失败时会把交易从Redis中清除掉
   - 导致后续无法再重试更新数据库中transactions信息

## 解决方案

### 1. 改进EVM details同步失败时的缓存处理

**问题**：当EVM details同步失败时，Redis缓存可能被清除，导致后续无法重试。

**解决方案**：
- 在`evm_details_sync.go`的`processTask`函数中，当EVM details获取失败或保存失败时，调用`preserveTransactionCache`函数
- `preserveTransactionCache`函数确保交易在Redis缓存中保留，即使同步失败也能被查询和后续更新

**代码位置**：
- `internal/service/evm_details_sync.go:277-295` - 获取EVM details失败时的处理
- `internal/service/evm_details_sync.go:297-302` - 保存EVM details失败时的处理
- `internal/service/evm_details_sync.go:708-732` - `preserveTransactionCache`函数实现

### 2. 改进signatures和signer更新后的缓存处理

**问题**：当signatures和signer更新成功后，需要确保Redis缓存也被更新。

**解决方案**：
- 在`saveEVMDetails`函数中，当成功更新signatures和signer后，重新加载交易并更新Redis缓存
- 如果更新失败，也保留缓存以便后续重试

**代码位置**：
- `internal/service/evm_details_sync.go:500-519` - 更新signatures和signer后的缓存处理

### 3. 添加定期检查机制

**问题**：即使EVM details同步成功，如果signatures和signer更新失败，这些信息可能仍然缺失。

**解决方案**：
- 添加`periodicSignerUpdateWorker`后台worker，每5分钟检查一次缺失signatures和signer的EVM交易
- `checkAndUpdateMissingSigners`函数查询数据库中已同步EVM details但signatures/signer为空的交易
- `updateSignerFromEVMDetails`函数从evm_transactions表中提取信息并更新transactions表

**代码位置**：
- `internal/service/evm_details_sync.go:98-105` - 启动定期检查worker
- `internal/service/evm_details_sync.go:734-752` - `periodicSignerUpdateWorker`实现
- `internal/service/evm_details_sync.go:754-793` - `checkAndUpdateMissingSigners`实现
- `internal/service/evm_details_sync.go:796-877` - `updateSignerFromEVMDetails`实现

## 改进效果

1. **Redis缓存保护**：
   - EVM details同步失败时，交易仍然保留在Redis缓存中
   - 确保查询时能够获取到交易数据，即使signatures和signer暂时缺失

2. **自动修复机制**：
   - 定期检查并更新缺失的signatures和signer
   - 即使初始同步失败，后续也能自动修复

3. **更好的错误处理**：
   - 失败时保留缓存，成功时更新缓存
   - 确保数据的一致性和可用性

## 配置说明

定期检查的间隔时间可以在代码中调整：
- 当前设置：每5分钟检查一次
- 位置：`internal/service/evm_details_sync.go:741`
- 可以根据实际需求调整检查频率

## 测试建议

1. **测试EVM details同步失败场景**：
   - 模拟EVM RPC失败
   - 验证Redis缓存是否保留
   - 验证后续重试是否能够成功

2. **测试定期检查机制**：
   - 创建一些signatures/signer为空的EVM交易
   - 等待定期检查执行
   - 验证是否自动更新

3. **测试缓存更新**：
   - 验证signatures和signer更新后，Redis缓存是否同步更新
   - 验证查询时是否能获取到最新的数据

## 注意事项

1. 定期检查机制会增加一定的数据库查询负载，但影响很小（每5分钟查询最多100条记录）
2. 如果EVM details同步持续失败，定期检查机制会持续尝试更新，直到成功
3. Redis缓存的TTL仍然是5分钟，但通过定期检查和失败时的缓存保留，确保数据可用性
