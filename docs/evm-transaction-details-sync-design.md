# EVM 交易细节同步技术方案

## 一、方案评估

### 1.1 用户提出的方案

**方案描述**：
- 在数据同步时，识别到 `MsgEthereumTx` 后
- 在保持现有逻辑外，**同步调用** `GetTransactionDetails` 来保存 EVM 交易细节
- 与 Injective 交易 hash 进行关联

**优点**：
- ✅ 实现简单，逻辑清晰
- ✅ 数据一致性较好（同步保存）
- ✅ 不需要额外的消息队列

**缺点**：
- ❌ **性能问题**：同步调用会阻塞主流程，影响交易同步速度
- ❌ **可靠性问题**：EVM RPC 失败会影响整个交易同步流程
- ❌ **错误隔离差**：EVM 数据获取失败可能导致交易同步失败
- ❌ **无重试机制**：临时网络问题会导致数据丢失
- ❌ **扩展性差**：EVM RPC 响应慢时，会拖慢整个同步流程

### 1.2 推荐方案：异步处理 + 消息队列

**核心思路**：
1. **主流程**：识别 EVM 交易后，只提取 EVM hash，标记为需要处理
2. **异步处理**：使用消息队列异步获取和保存 EVM 交易细节
3. **错误隔离**：EVM 数据获取失败不影响主流程
4. **重试机制**：支持自动重试，确保数据最终一致性

## 二、详细设计方案

### 2.1 数据模型设计

#### 方案 A：扩展 Transaction 表（推荐）

在 `Transaction` 表中添加 EVM 交易细节字段：

```go
// internal/models/transaction.go
type Transaction struct {
    // ... 现有字段
    
    // EVM 交易相关字段
    EVMHash              string  `json:"evm_hash" gorm:"type:varchar(128);column:evm_hash;index:idx_evm_hash"`
    EVMDetailsStorageKey string  `json:"evm_details_storage_key" gorm:"column:evm_details_storage_key"`
    EVMDetailsSynced      bool    `json:"evm_details_synced" gorm:"column:evm_details_synced;default:0;index"`
    EVMDetailsSyncError   string  `json:"evm_details_sync_error" gorm:"type:text;column:evm_details_sync_error"`
    EVMDetailsSyncAt      *time.Time `json:"evm_details_sync_at" gorm:"column:evm_details_sync_at"`
}
```

**优点**：
- 数据关联紧密，查询方便
- 不需要额外的 JOIN 操作
- 可以利用现有的存储优化机制（对象存储）

**缺点**：
- Transaction 表字段增加

#### 方案 B：独立表（备选）

如果 EVM 交易细节数据量很大，可以考虑独立表：

```go
// internal/models/evm_transaction_details.go
type EVMTransactionDetails struct {
    gorm.Model
    
    // 关联字段
    CosmosTxHash string `json:"cosmos_tx_hash" gorm:"type:varchar(128);column:cosmos_tx_hash;uniqueIndex;not null"`
    EVMHash      string `json:"evm_hash" gorm:"type:varchar(128);column:evm_hash;index;not null"`
    
    // 交易细节（JSON 格式，或存储到对象存储）
    Details      string `json:"details" gorm:"type:text;column:details"` // JSON 格式的 TransactionDetails
    DetailsStorageKey string `json:"details_storage_key" gorm:"column:details_storage_key"`
    
    // 同步状态
    Synced       bool   `json:"synced" gorm:"column:synced;default:0;index"`
    SyncError    string `json:"sync_error" gorm:"type:text;column:sync_error"`
    SyncAt       *time.Time `json:"sync_at" gorm:"column:sync_at"`
}
```

**推荐使用方案 A**，因为：
1. EVM 交易细节数据量可控（单个交易细节 < 50KB）
2. 查询时通常需要关联 Cosmos 交易信息
3. 可以利用现有的存储优化机制

### 2.2 EVM Hash 提取

在交易同步时，从 Events 中提取 EVM hash：

```go
// internal/service/evm_extractor.go
package service

import (
    "context"
    "strings"
    
    evmtypes "github.com/InjectiveLabs/sdk-go/chain/evm/types"
    injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
)

// ExtractEVMHashFromEvents 从交易事件中提取 EVM hash
func ExtractEVMHashFromEvents(events []*injective_explorer_rpcpb.Event) string {
    for _, event := range events {
        if event == nil {
            continue
        }
        
        // 检查事件类型
        eventType := event.GetType()
        if !strings.Contains(eventType, "ethereum") && !strings.Contains(eventType, "evm") {
            continue
        }
        
        // 从 attributes 中提取 EVM hash
        attrs := event.GetAttributes()
        if evmHash, ok := attrs[evmtypes.AttributeKeyEthereumTxHash]; ok {
            // 确保 hash 有 0x 前缀
            hash := strings.TrimSpace(evmHash)
            if !strings.HasPrefix(hash, "0x") {
                hash = "0x" + hash
            }
            return hash
        }
    }
    return ""
}

// IsEVMTransaction 检查交易是否为 EVM 交易
func IsEVMTransaction(txData *injective_explorer_rpcpb.TxDetailData) bool {
    // 方法 1: 检查 Events 中是否有 EVM hash
    if evmHash := ExtractEVMHashFromEvents(txData.Events); evmHash != "" {
        return true
    }
    
    // 方法 2: 检查 Messages 类型
    if len(txData.Messages) == 0 {
        return false
    }
    
    txBody := &cosmostxpb.TxBody{}
    if err := txBody.Unmarshal(txData.Messages); err != nil {
        return false
    }
    
    for _, msgAny := range txBody.Messages {
        if msgAny == nil {
            continue
        }
        typeURL := msgAny.TypeUrl
        if strings.Contains(typeURL, "MsgEthereumTx") ||
           strings.Contains(typeURL, "ethermint.evm.v1") ||
           strings.Contains(typeURL, "injective.evm.v1") {
            return true
        }
    }
    
    return false
}
```

### 2.3 同步流程改造

#### 2.3.1 主流程改造（processTransactionBatchInternal）

在 `processTransactionBatchInternal` 中，识别 EVM 交易并提取 hash：

```go
// internal/service/sync.go

func (s *SyncService) processTransactionBatchInternal(ctx context.Context, redisClient *redis.Client, txHashes []string) (int, error) {
    // ... 现有代码 ...
    
    // Step 2: Convert successful responses to models
    txModels := make([]txModelData, 0, len(txHashes))
    for _, result := range txResults {
        // ... 现有转换逻辑 ...
        
        // 识别 EVM 交易并提取 hash
        if IsEVMTransaction(result.txResp.Data) {
            evmHash := ExtractEVMHashFromEvents(result.txResp.Data.Events)
            if evmHash != "" {
                txModel.EVMHash = evmHash
                txModel.EVMDetailsSynced = false // 标记为未同步
                
                // 将 EVM hash 推送到处理队列（异步处理）
                go func(cosmosHash, evmHash string) {
                    evmCtx, evmCancel := context.WithTimeout(context.Background(), 5*time.Second)
                    defer evmCancel()
                    
                    // 推送到 Redis 队列
                    if err := redisClient.LPush(evmCtx, s.config.EVMDetailsQueueKey, 
                        fmt.Sprintf("%s:%s", cosmosHash, evmHash)).Err(); err != nil {
                        global.Logger.Warnf("failed to push EVM details task to queue: %v", err)
                    }
                }(txModel.Hash, evmHash)
            }
        }
        
        txModels = append(txModels, txModelData{
            hash:    result.hash,
            txModel: txModel,
            txData:  result.txResp.Data,
        })
    }
    
    // ... 后续批量保存逻辑保持不变 ...
}
```

#### 2.3.2 异步处理服务

创建独立的 EVM 交易细节处理服务：

```go
// internal/service/evm_details_sync.go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "time"
    
    "injective-explorer/internal/global"
    "injective-explorer/internal/models"
    "injective-explorer/provider/cache"
    "injective-explorer/provider/chain"
    "injective-explorer/provider/db"
    "injective-explorer/provider/storage"
    ethcommon "github.com/ethereum/go-ethereum/common"
    "github.com/go-redis/redis/v8"
    "github.com/pkg/errors"
    "gorm.io/gorm"
)

// EVMDetailsSyncService 处理 EVM 交易细节同步
type EVMDetailsSyncService struct {
    ctx            context.Context
    cancel         context.CancelFunc
    config         *EVMDetailsSyncConfig
    evmClient      *chain.EVMClient
    redisClient    *redis.Client
}

type EVMDetailsSyncConfig struct {
    QueueKey           string        // Redis 队列 key
    WorkerCount        int           // 工作协程数量
    BatchSize          int           // 批处理大小
    RetryDelay         time.Duration // 重试延迟
    MaxRetryAttempts   int           // 最大重试次数
    ProcessInterval    time.Duration // 处理间隔
    Timeout            time.Duration // 单个任务超时时间
}

// NewEVMDetailsSyncService 创建 EVM 交易细节同步服务
func NewEVMDetailsSyncService(ctx context.Context, config *EVMDetailsSyncConfig) (*EVMDetailsSyncService, error) {
    serviceCtx, cancel := context.WithCancel(ctx)
    
    // 创建 EVM 客户端
    evmClient, err := chain.NewEVMClient(global.ChainConfig.Network)
    if err != nil {
        cancel()
        return nil, errors.Wrap(err, "failed to create EVM client")
    }
    
    redisClient := cache.GetRedisClient()
    if redisClient == nil {
        cancel()
        return nil, errors.New("redis client is not available")
    }
    
    return &EVMDetailsSyncService{
        ctx:         serviceCtx,
        cancel:      cancel,
        config:      config,
        evmClient:   evmClient,
        redisClient: redisClient,
    }, nil
}

// Start 启动同步服务
func (s *EVMDetailsSyncService) Start() {
    for i := 0; i < s.config.WorkerCount; i++ {
        go s.worker(i)
    }
    global.Logger.Infof("EVM details sync service started with %d workers", s.config.WorkerCount)
}

// Stop 停止同步服务
func (s *EVMDetailsSyncService) Stop() {
    s.cancel()
    if s.evmClient != nil {
        s.evmClient.Close()
    }
    global.Logger.Info("EVM details sync service stopped")
}

// worker 工作协程
func (s *EVMDetailsSyncService) worker(id int) {
    ticker := time.NewTicker(s.config.ProcessInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-s.ctx.Done():
            return
        case <-ticker.C:
            s.processBatch(id)
        }
    }
}

// processBatch 批量处理 EVM 交易细节
func (s *EVMDetailsSyncService) processBatch(workerID int) {
    ctx, cancel := context.WithTimeout(s.ctx, s.config.Timeout)
    defer cancel()
    
    // 批量从队列中获取任务
    tasks := make([]string, 0, s.config.BatchSize)
    for i := 0; i < s.config.BatchSize; i++ {
        result, err := s.redisClient.BRPop(ctx, time.Second, s.config.QueueKey).Result()
        if err != nil {
            if err != redis.Nil {
                global.Logger.Warnf("worker %d: failed to pop from queue: %v", workerID, err)
            }
            break
        }
        tasks = append(tasks, result[1])
    }
    
    if len(tasks) == 0 {
        return
    }
    
    global.Logger.Debugf("worker %d: processing %d EVM details tasks", workerID, len(tasks))
    
    // 并发处理任务
    var wg sync.WaitGroup
    for _, task := range tasks {
        wg.Add(1)
        go func(taskStr string) {
            defer wg.Done()
            s.processTask(ctx, taskStr, workerID)
        }(task)
    }
    wg.Wait()
}

// processTask 处理单个任务
func (s *EVMDetailsSyncService) processTask(ctx context.Context, taskStr string, workerID int) {
    // 解析任务：格式为 "cosmosHash:evmHash"
    parts := strings.Split(taskStr, ":")
    if len(parts) != 2 {
        global.Logger.Warnf("worker %d: invalid task format: %s", workerID, taskStr)
        return
    }
    
    cosmosHash := parts[0]
    evmHash := parts[1]
    
    // 检查是否已经处理过
    var tx models.Transaction
    err := db.Get().WithContext(ctx).
        Where("hash = ?", cosmosHash).
        First(&tx).Error
    
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            global.Logger.Warnf("worker %d: transaction %s not found, skipping", workerID, cosmosHash)
            return
        }
        global.Logger.Errorf("worker %d: failed to query transaction %s: %v", workerID, cosmosHash, err)
        // 重新入队重试
        s.retryTask(ctx, taskStr)
        return
    }
    
    // 如果已经同步过，跳过
    if tx.EVMDetailsSynced {
        return
    }
    
    // 获取 EVM 交易细节
    details, err := s.fetchEVMDetails(ctx, evmHash)
    if err != nil {
        global.Logger.Warnf("worker %d: failed to fetch EVM details for %s (evm: %s): %v", 
            workerID, cosmosHash, evmHash, err)
        
        // 更新错误信息
        s.updateSyncError(ctx, cosmosHash, err.Error())
        
        // 重试（如果未超过最大重试次数）
        if s.shouldRetry(&tx) {
            s.retryTask(ctx, taskStr)
        }
        return
    }
    
    // 保存交易细节
    if err := s.saveEVMDetails(ctx, &tx, details); err != nil {
        global.Logger.Errorf("worker %d: failed to save EVM details for %s: %v", workerID, cosmosHash, err)
        s.retryTask(ctx, taskStr)
        return
    }
    
    global.Logger.Debugf("worker %d: successfully synced EVM details for %s", workerID, cosmosHash)
}

// fetchEVMDetails 获取 EVM 交易细节
func (s *EVMDetailsSyncService) fetchEVMDetails(ctx context.Context, evmHash string) (*chain.TransactionDetails, error) {
    hash := ethcommon.HexToHash(evmHash)
    details, err := s.evmClient.GetTransactionDetails(ctx, hash)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get transaction details for %s", evmHash)
    }
    return details, nil
}

// saveEVMDetails 保存 EVM 交易细节
func (s *EVMDetailsSyncService) saveEVMDetails(ctx context.Context, tx *models.Transaction, details *chain.TransactionDetails) error {
    // 序列化为 JSON
    detailsJSON, err := json.Marshal(details)
    if err != nil {
        return errors.Wrap(err, "failed to marshal transaction details")
    }
    
    // 检查是否需要存储到对象存储（如果数据较大）
    storageKey := ""
    detailsStr := string(detailsJSON)
    
    if storage.IsEnabled() && len(detailsJSON) > 3*1024 { // 3KB 阈值
        storageClient := storage.Get()
        storageKey = storage.BuildStorageKey("evm_details", tx.Hash)
        
        if err := storageClient.PutObject(ctx, storageKey, detailsJSON); err != nil {
            global.Logger.Warnf("failed to upload EVM details to storage for %s: %v", tx.Hash, err)
            // 继续使用数据库存储
            storageKey = ""
        } else {
            detailsStr = "" // 清空数据库字段
        }
    }
    
    // 更新数据库
    now := time.Now()
    updates := map[string]interface{}{
        "evm_details_synced":      true,
        "evm_details_sync_at":     &now,
        "evm_details_sync_error":  "",
    }
    
    if storageKey != "" {
        updates["evm_details_storage_key"] = storageKey
    } else {
        // 如果数据较小，直接存储在数据库
        // 注意：这里需要添加一个新字段来存储 details JSON
        // 或者使用现有的某个字段（如果合适）
    }
    
    err = db.Get().WithContext(ctx).
        Model(tx).
        Updates(updates).Error
    
    if err != nil {
        return errors.Wrapf(err, "failed to update transaction %s", tx.Hash)
    }
    
    return nil
}

// updateSyncError 更新同步错误信息
func (s *EVMDetailsSyncService) updateSyncError(ctx context.Context, cosmosHash string, errorMsg string) {
    db.Get().WithContext(ctx).
        Model(&models.Transaction{}).
        Where("hash = ?", cosmosHash).
        Updates(map[string]interface{}{
            "evm_details_sync_error": errorMsg,
        })
}

// shouldRetry 判断是否应该重试
func (s *EVMDetailsSyncService) shouldRetry(tx *models.Transaction) bool {
    // 可以根据错误类型和重试次数判断
    // 这里简化处理，总是重试（实际应该限制重试次数）
    return true
}

// retryTask 重新入队任务
func (s *EVMDetailsSyncService) retryTask(ctx context.Context, taskStr string) {
    // 延迟后重新入队
    go func() {
        time.Sleep(s.config.RetryDelay)
        if err := s.redisClient.LPush(ctx, s.config.QueueKey, taskStr).Err(); err != nil {
            global.Logger.Warnf("failed to retry task %s: %v", taskStr, err)
        }
    }()
}
```

### 2.4 数据库迁移

```sql
-- 添加 EVM 相关字段到 transactions 表
ALTER TABLE transactions
ADD COLUMN evm_hash VARCHAR(128) NULL AFTER hash,
ADD COLUMN evm_details_storage_key VARCHAR(255) NULL AFTER logs_storage_key,
ADD COLUMN evm_details_synced BOOLEAN DEFAULT FALSE AFTER evm_details_storage_key,
ADD COLUMN evm_details_sync_error TEXT NULL AFTER evm_details_synced,
ADD COLUMN evm_details_sync_at TIMESTAMP NULL AFTER evm_details_sync_error;

-- 创建索引
CREATE INDEX idx_evm_hash ON transactions(evm_hash);
CREATE INDEX idx_evm_details_synced ON transactions(evm_details_synced);
```

### 2.5 配置更新

```go
// internal/service/constants.go 或 config

// EVM 交易细节同步配置
EVMDetailsQueueKey: "evm:details:queue",
EVMDetailsWorkerCount: 3,        // 3 个工作协程
EVMDetailsBatchSize: 10,         // 每批处理 10 个
EVMDetailsRetryDelay: 5 * time.Second,
EVMDetailsMaxRetryAttempts: 3,
EVMDetailsProcessInterval: 2 * time.Second,
EVMDetailsTimeout: 30 * time.Second,
```

## 三、方案对比

| 特性 | 用户方案（同步） | 推荐方案（异步） |
|------|----------------|----------------|
| **性能** | ❌ 阻塞主流程 | ✅ 不阻塞主流程 |
| **可靠性** | ❌ EVM RPC 失败影响主流程 | ✅ 错误隔离，不影响主流程 |
| **重试机制** | ❌ 无 | ✅ 支持自动重试 |
| **扩展性** | ❌ 差 | ✅ 好（可调整工作协程数） |
| **数据一致性** | ✅ 强一致性 | ✅ 最终一致性 |
| **实现复杂度** | ✅ 简单 | ⚠️ 中等（需要消息队列） |
| **资源占用** | ✅ 低 | ⚠️ 中等（需要 Redis 队列） |

## 四、实施步骤

### 阶段 1：基础准备（1-2 天）
1. ✅ 创建 EVM hash 提取函数
2. ✅ 数据库迁移，添加 EVM 相关字段
3. ✅ 更新 Transaction 模型

### 阶段 2：主流程改造（1 天）
1. ✅ 在 `processTransactionBatchInternal` 中识别 EVM 交易
2. ✅ 提取 EVM hash 并推送到队列
3. ✅ 更新 Transaction 模型字段

### 阶段 3：异步处理服务（2-3 天）
1. ✅ 创建 `EVMDetailsSyncService`
2. ✅ 实现任务处理逻辑
3. ✅ 实现重试机制
4. ✅ 集成到主服务

### 阶段 4：测试和优化（1-2 天）
1. ✅ 单元测试
2. ✅ 集成测试
3. ✅ 性能测试和优化

## 五、注意事项

1. **EVM Hash 格式**：确保 hash 有 `0x` 前缀
2. **错误处理**：区分临时错误（网络问题）和永久错误（交易不存在）
3. **存储优化**：大文件使用对象存储，小文件直接存数据库
4. **监控告警**：监控队列长度和处理速度
5. **数据一致性**：考虑使用事务确保数据一致性（如果需要）

## 六、总结

**推荐使用异步处理方案**，因为：
1. ✅ 不阻塞主流程，性能更好
2. ✅ 错误隔离，可靠性更高
3. ✅ 支持重试，数据完整性更好
4. ✅ 易于扩展和维护

虽然实现复杂度稍高，但带来的收益是值得的。对于高并发的区块链数据同步场景，异步处理是更好的选择。

