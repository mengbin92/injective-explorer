# Peggy 跨链交易 EVM Hash 同步技术方案

## 一、背景与问题

### 1.1 现状

- Peggy 跨链交易表（`peggy_deposit_txs`、`peggy_withdrawal_txs`）中的 `eth_tx_hash` 字段目前均为空
- 当前 `evmHash` 从 ABCI Events 中提取，但 **Peggy 消息（MsgDepositClaim、MsgWithdrawClaim）是 Cosmos SDK 消息**，不会产生 `ethereum_tx_hash` 属性的 ABCI 事件
- 真正的 Ethereum 交易 hash 存在于 **Peggy 桥接的 EVM 链**（Sepolia 测试网 / Ethereum 主网）上，而非 Injective 链的 EVM 层

### 1.2 目标

- 从 Sepolia（测试网）或 Ethereum 主网获取 Peggy 跨链交易对应的 EVM 交易 hash
- 支持通过配置文件切换测试网 / 主网
- 采用异步补充模式，不影响主同步流程
- 支持历史数据回填和新数据实时同步

---

## 二、Peggy 合约事件结构

基于 Gravity Bridge 合约（Peggy 标准实现），Ethereum 链上关键事件：

### 2.1 SendToCosmosEvent（存款：Ethereum → Cosmos）

用户调用 `sendToCosmos()` 锁定资产时触发：

```solidity
event SendToCosmosEvent(
    address indexed _tokenContract,
    address indexed _sender,
    string _destination,      // Cosmos 接收地址（Bech32 或 hex）
    uint256 _amount,
    uint256 _eventNonce      // 全局递增，与 MsgDepositClaim.event_nonce 对应
);
```

**匹配维度**：`event_nonce`、`_sender`（对应 ethereum_sender）、`_tokenContract`（对应 denom 中的 0x...）、`_amount`

### 2.2 TransactionBatchExecutedEvent（提款完成：Cosmos → Ethereum）

Relayer 调用 `submitBatch()` 执行批量提款时触发：

```solidity
event TransactionBatchExecutedEvent(
    uint256 indexed _batchNonce,
    address indexed _token,
    uint256 _eventNonce
);
```

**匹配维度**：`batch_nonce`、`_token`（token contract，对应 denom）

**注意**：一个 batch 包含多笔提款，所有 `outgoing_tx_id` 在该 batch 内的提款共享同一个 EVM tx hash。

---

## 三、配置设计

### 3.1 Proto 定义扩展

在 `internal/conf/conf.proto` 的 `Chain` 中新增：

```protobuf
message Chain {
  // ... 现有字段
  EVM evm = 12;
  PeggyBridgeEvm peggy_bridge_evm = 13;  // 新增：Peggy 桥接 EVM 配置
}

// Peggy 桥接 EVM 配置（Sepolia / Ethereum 主网）
// 注意：此配置独立于 Chain.evm，后者用于查询 Injective 自身的 EVM 层
message PeggyBridgeEvm {
  bool enabled = 1;                    // 是否启用 Peggy EVM hash 同步
  string rpc_endpoint = 2;            // JSON-RPC 端点（以太坊测试网/主网）
  int64 chain_id = 3;                  // Chain ID（11155111=Sepolia, 1=Mainnet）
  string bridge_contract = 4;          // Peggy/Gravity 合约地址（可选，用于过滤）
  string queue_key = 5;                // Redis 队列 key，默认 "peggy:evm_hash:queue"
  google.protobuf.Duration timeout = 6;
  Retry retry = 7;

  int32 rpc_rate_limit = 8;            // RPC 限流（每秒请求数），默认 10
  int32 worker_count = 9;               // 并发 worker 数，默认 3
  int32 min_confirm_blocks = 10;       // 最小确认块数，默认 12
  int32 batch_update_size = 11;        // 分批更新大小，默认 100

  message Retry {
    int32 max_attempts = 1;
    google.protobuf.Duration initial_delay = 2;
  }
}
```

### 3.2 config.yaml 示例

```yaml
chain:
  # ... 现有 chain 配置

  # Peggy 桥接 EVM 配置（用于获取 eth_tx_hash）
  peggy_bridge_evm:
    enabled: ${PEGGY_BRIDGE_EVM_ENABLED:false}
    rpc_endpoint: ${PEGGY_BRIDGE_EVM_RPC_ENDPOINT:https://rpc.sepolia.org}
    chain_id: ${PEGGY_BRIDGE_EVM_CHAIN_ID:11155111}
    # 可选：指定合约地址以精确过滤，不填则通过 topic 过滤
    bridge_contract: ${PEGGY_BRIDGE_EVM_CONTRACT:}
    queue_key: ${PEGGY_BRIDGE_EVM_QUEUE_KEY:peggy:evm_hash:queue}
    timeout: 15s
    retry:
      max_attempts: 3
      initial_delay: 2s
    rpc_rate_limit: ${PEGGY_BRIDGE_EVM_RPC_RATE_LIMIT:10}     # 每秒请求数
    worker_count: ${PEGGY_BRIDGE_EVM_WORKER_COUNT:3}         # 并发 worker 数
    min_confirm_blocks: ${PEGGY_BRIDGE_EVM_MIN_CONFIRM_BLOCKS:12}  # 最小确认块数
    batch_update_size: ${PEGGY_BRIDGE_EVM_BATCH_UPDATE_SIZE:100}    # 分批更新大小
```

### 3.3 环境变量

| 变量 | 说明 | 测试网示例 | 主网示例 | 默认值 |
|------|------|------------|----------|--------|
| `PEGGY_BRIDGE_EVM_ENABLED` | 是否启用 | `true` | `true` | `false` |
| `PEGGY_BRIDGE_EVM_RPC_ENDPOINT` | RPC 端点 | `https://rpc.sepolia.org` | `https://eth.llamarpc.com` | - |
| `PEGGY_BRIDGE_EVM_CHAIN_ID` | Chain ID | `11155111` | `1` | - |
| `PEGGY_BRIDGE_EVM_CONTRACT` | 合约地址（可选） | - | - | - |
| `PEGGY_BRIDGE_EVM_QUEUE_KEY` | Redis 队列 key | `peggy:evm_hash:queue` | `peggy:evm_hash:queue` | `peggy:evm_hash:queue` |
| `PEGGY_BRIDGE_EVM_RPC_RATE_LIMIT` | 每秒请求数 | `10` | `10` | `10` |
| `PEGGY_BRIDGE_EVM_WORKER_COUNT` | 并发 worker 数 | `3` | `3` | `3` |
| `PEGGY_BRIDGE_EVM_MIN_CONFIRM_BLOCKS` | 最小确认块数 | `12` | `12` | `12` |
| `PEGGY_BRIDGE_EVM_BATCH_UPDATE_SIZE` | 分批更新大小 | `100` | `100` | `100` |

### 3.4 .env.example 补充

```bash
# ========== Peggy Bridge EVM (Sepolia / Mainnet) ==========
# 用于从 Ethereum 链获取 Peggy 跨链交易的 evmHash
# 测试网：Sepolia
PEGGY_BRIDGE_EVM_ENABLED=true
PEGGY_BRIDGE_EVM_RPC_ENDPOINT=https://rpc.sepolia.org
PEGGY_BRIDGE_EVM_CHAIN_ID=11155111
PEGGY_BRIDGE_EVM_QUEUE_KEY=peggy:evm_hash:queue
PEGGY_BRIDGE_EVM_RPC_RATE_LIMIT=10
PEGGY_BRIDGE_EVM_WORKER_COUNT=3
PEGGY_BRIDGE_EVM_MIN_CONFIRM_BLOCKS=12
PEGGY_BRIDGE_EVM_BATCH_UPDATE_SIZE=100

# 主网配置示例（上线时切换）
# PEGGY_BRIDGE_EVM_ENABLED=true
# PEGGY_BRIDGE_EVM_RPC_ENDPOINT=https://eth.llamarpc.com
# PEGGY_BRIDGE_EVM_CHAIN_ID=1
# PEGGY_BRIDGE_EVM_RPC_RATE_LIMIT=10
# PEGGY_BRIDGE_EVM_WORKER_COUNT=3
# PEGGY_BRIDGE_EVM_MIN_CONFIRM_BLOCKS=12
# PEGGY_BRIDGE_EVM_BATCH_UPDATE_SIZE=100
```

---

## 四、架构设计

### 4.1 整体流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Sync Service (主流程)                                                        │
│  ParseCrossChainTransaction() → saveCrossChainTransaction()                   │
│  - 解析 Peggy 消息，evmHash 为空时仍正常保存                                    │
│  - 将 eth_tx_hash 为空的记录入队到 Redis                                         │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  PeggyEvmHashSyncService (异步后台服务)                                        │
│  - 从 Redis 队列消费任务                                                       │
│  - 调用 PeggyBridgeEVMClient 查询 Ethereum/Sepolia                            │
│  - 通过合约事件匹配并获取 tx hash                                               │
│  - 回写 peggy_deposit_txs / peggy_withdrawal_txs.eth_tx_hash                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 组件职责

| 组件 | 职责 |
|------|------|
| `PeggyBridgeEVMClient` | 连接 Sepolia/主网，查询区块、日志、交易 |
| `PeggyEvmHashSyncService` | 消费队列、调用 Client、更新 DB |
| Redis 队列 | 存储待补充 eth_tx_hash 的记录标识 |
| `global.PeggyBridgeEVMConfig` | 全局配置，供各模块使用 |

---

## 五、详细设计

### 5.1 事件 Topic 签名

用于 `eth_getLogs` 过滤：

```go
// SendToCosmosEvent(address,address,string,uint256,uint256)
var SendToCosmosEventSig = crypto.Keccak256Hash([]byte(
    "SendToCosmosEvent(address,address,string,uint256,uint256)",
))

// TransactionBatchExecutedEvent(uint256,address,uint256)
var TransactionBatchExecutedEventSig = crypto.Keccak256Hash([]byte(
    "TransactionBatchExecutedEvent(uint256,address,uint256)",
))
```

### 5.2 查询策略

#### Deposit (MsgDepositClaim)

1. 从 `peggy_deposit_txs` 读取：`event_nonce`、`event_height`、`sender`、`denom`、`amount`
2. 从 `denom` 解析 token contract：`peggy0x...` → `0x...`
3. 调用 `eth_getLogs`：
   - `topics[0]` = SendToCosmosEventSig
   - `topics[1]` = tokenContract (indexed)
   - `topics[2]` = sender (indexed)
4. 遍历 logs，解析 data 中的 `_eventNonce`、`_amount`
5. 精确匹配 `event_nonce`、`amount`（避免误匹配）
6. 取该 log 所属交易的 `transactionHash`

**区块范围**：
- `event_height` 为 Ethereum 区块高度（Peggy 事件中记录）
- `block_height` 为 Cosmos 区块高度（交易执行位置）
- 使用 `[event_height, event_height+10]` 作为搜索窗口（防止重放等边界情况）

#### Withdrawal (MsgWithdrawClaim)

1. 从任务或 `peggy_withdrawal_txs` 读取：`batch_nonce`、`denom`（解析 token contract）
2. 调用 `eth_getLogs`：
   - `topics[0]` = TransactionBatchExecutedEventSig
   - `topics[1]` = batchNonce (indexed)
   - `topics[2]` = token (indexed)
3. 找到匹配的 log 后，取其 `transactionHash`
4. 更新**该 batch 下所有 withdrawal 记录**的 `eth_tx_hash`：
   - **MsgWithdrawClaim 记录**：`WHERE batch_nonce = ? AND denom LIKE ?`（需从 EventWithdrawClaim 解析 token_contract 并写入 denom）
   - **MsgSendToEth 记录**：`WHERE outgoing_tx_id IN (batch_tx_ids)`（需解析 EventOutgoingBatch 建立 batch_nonce → outgoing_tx_ids 映射，见 §11.1）

### 5.3 队列设计

**队列 Key**：`peggy:evm_hash:queue`（可配置）

**任务格式**：

- Deposit：`deposit:{block_height}:{event_nonce}:{tx_hash}`
- Withdrawal：`withdrawal:{block_height}:{batch_nonce}:{tx_hash}`

或简化为 JSON：`{"type":"deposit","block_height":123,"event_nonce":456,"tx_hash":"xxx"}`

**去重**：任务入队前检查 `eth_tx_hash` 是否已填充，避免重复入队。

### 5.4 数据模型与 DB 更新

`saveCrossChainTransaction` 中，OnConflict 的 `DoUpdates` 需包含 `eth_tx_hash`，以便异步服务更新时能正确回写：

```go
// saveCrossChainTransaction - Peggy deposit
DoUpdates: clause.AssignmentColumns([]string{
    "state", "updated_at",
    "sender", "receiver", "amount", "denom", "orchestrator_address",
    "block_height", "block_timestamp",
    "eth_tx_hash",  // 确保异步更新可写入
}),

// savePeggyWithdrawal - MsgWithdrawClaim
DoUpdates: clause.AssignmentColumns([]string{
    "state", "updated_at",
    "event_nonce", "event_height", "orchestrator_address",
    "eth_tx_hash",
}),
```

### 5.5 性能与并发处理

#### 5.5.1 Withdrawal Batch 更新优化

当一个 batch 包含大量提款记录时（可能上百笔），单次更新可能导致：
- DB 事务超时
- 锁表时间过长

**优化方案**：

```go
// 分批更新，每批 100 条
const batchUpdateSize = 100

func UpdateWithdrawalBatchTxHash(ctx context.Context, db *gorm.DB, batchNonce uint64, tokenContract, txHash string) error {
    var txIDs []uint64

    // 1. 查询该 batch 下所有待更新的记录 ID（tokenContract 需 normalize 为小写以匹配 denom 格式）
    db.Model(&PeggyWithdrawalTx{}).
        Where("batch_nonce = ? AND denom LIKE ?", batchNonce, "peggy"+tokenContract+"%").
        Where("eth_tx_hash = '' OR eth_tx_hash IS NULL").
        Pluck("id", &txIDs)

    // 2. 分批更新
    for i := 0; i < len(txIDs); i += batchUpdateSize {
        end := i + batchUpdateSize
        if end > len(txIDs) {
            end = len(txIDs)
        }

        batchIDs := txIDs[i:end]
        if err := db.Model(&PeggyWithdrawalTx{}).
            Where("id IN ?", batchIDs).
            Update("eth_tx_hash", txHash).Error; err != nil {
            return err
        }
    }

    return nil
}
```

#### 5.5.2 RPC 限流与并发控制

```go
type PeggyBridgeEVMClient struct {
    client   *ethclient.Client
    chainID  *big.Int
    timeout  time.Duration
    rateLimit *rate.Limiter  // 新增：限流器
}

func NewPeggyBridgeEVMClient(rpcEndpoint string, chainID int64, timeout time.Duration, rps int) (*PeggyBridgeEVMClient, error) {
    client, err := ethclient.Dial(rpcEndpoint)
    if err != nil {
        return nil, err
    }

    return &PeggyBridgeEVMClient{
        client:    client,
        chainID:   big.NewInt(chainID),
        timeout:   timeout,
        rateLimit: rate.NewLimiter(rate.Limit(rps), rps),  // rps: 每秒请求数
    }, nil
}

func (c *PeggyBridgeEVMClient) GetDepositTxHash(ctx context.Context, ...) (string, error) {
    // 等待限流许可
    if err := c.rateLimit.Wait(ctx); err != nil {
        return "", err
    }
    // ... 原有逻辑
}
```

#### 5.5.3 Redis 队列并发消费

```go
// worker 并发数可配置
func (s *PeggyEvmHashSyncService) Run(ctx context.Context, workerCount int) {
    var wg sync.WaitGroup

    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            s.worker(ctx, workerID)
        }(i)
    }

    wg.Wait()
}

func (s *PeggyEvmHashSyncService) worker(ctx context.Context, workerID int) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            task := s.dequeueTask(ctx)
            if task == nil {
                time.Sleep(time.Second)
                continue
            }
            s.processTask(ctx, task)
        }
    }
}
```

### 5.6 EventOutgoingBatch 解析与映射表

由于业务需要用户看到 MsgSendToEth 的 evmHash，必须建立 `batch_nonce -> outgoing_tx_ids` 映射。

#### 5.6.1 映射表设计

```sql
-- peggy_batch_tx_mapping 表
CREATE TABLE peggy_batch_tx_mapping (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    batch_nonce BIGINT UNSIGNED NOT NULL,
    outgoing_tx_id BIGINT UNSIGNED NOT NULL,
    block_height BIGINT UNSIGNED NOT NULL,
    block_timestamp TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_batch_outgoing (batch_nonce, outgoing_tx_id),
    KEY idx_batch_nonce (batch_nonce),
    KEY idx_outgoing_tx_id (outgoing_tx_id)
) COMMENT='Peggy Batch -> OutgoingTxIDs 映射表';
```

#### 5.6.2 EventOutgoingBatch 事件结构

```go
// EventOutgoingBatch Cosmos 事件
type EventOutgoingBatch struct {
    BatchNonce    uint64   `json:"batch_nonce"`
    BatchTxIDs    []uint64 `json:"batch_tx_ids"`  // outgoing_tx_id 列表
    BlockHeight   uint64   `json:"block_height"`
    BlockTimestamp int64   `json:"block_timestamp"`
}
```

#### 5.6.3 解析逻辑（在区块处理中）

```go
// provider/chain/crosschain_parser.go

func parseEventOutgoingBatch(events []abci.Event, blockHeight int64, blockTime time.Time) ([]*PeggyBatchTxMapping, error) {
    var mappings []*PeggyBatchTxMapping

    for _, event := range events {
        if event.Type == "EventOutgoingBatch" {
            var batchEvent EventOutgoingBatch
            if err := parseEventAttributes(event.Attributes, &batchEvent); err != nil {
                continue
            }

            // 为每个 outgoing_tx_id 创建映射记录
            for _, txID := range batchEvent.BatchTxIDs {
                mapping := &PeggyBatchTxMapping{
                    BatchNonce:     batchEvent.BatchNonce,
                    OutgoingTxID:   txID,
                    BlockHeight:    blockHeight,
                    BlockTimestamp: blockTime,
                }
                mappings = append(mappings, mapping)
            }
        }
    }

    return mappings, nil
}
```

#### 5.6.4 更新 Withdrawal 的两类记录

```go
// 更新一个 batch 的所有相关记录
func UpdateWithdrawalBatchTxHash(ctx context.Context, db *gorm.DB, batchNonce uint64, tokenContract, txHash string) error {
    // 1. 查询映射表获取 outgoing_tx_ids
    var outgoingTxIDs []uint64
    db.Model(&PeggyBatchTxMapping{}).
        Where("batch_nonce = ?", batchNonce).
        Pluck("outgoing_tx_id", &outgoingTxIDs)

    // 2. 更新 MsgSendToEth 记录（用户可见）
    if len(outgoingTxIDs) > 0 {
        db.Model(&PeggyWithdrawalTx{}).
            Where("outgoing_tx_id IN ?", outgoingTxIDs).
            Where("eth_tx_hash = '' OR eth_tx_hash IS NULL").
            Update("eth_tx_hash", txHash)
    }

    // 3. 更新 MsgWithdrawClaim 记录（attestation）
    db.Model(&PeggyWithdrawalTx{}).
        Where("batch_nonce = ? AND denom LIKE ?", batchNonce, "peggy"+tokenContract+"%").
        Where("eth_tx_hash = '' OR eth_tx_hash IS NULL").
        Update("eth_tx_hash", txHash)

    return nil
}
```

### 5.7 Denom 与 Token Contract 映射

Peggy denom 格式：`peggy{ContractAddress}`，如 `peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238`。

```go
func ParseTokenContractFromDenom(denom string) string {
    if strings.HasPrefix(denom, "peggy") {
        return converter.NormalizeHash(strings.TrimPrefix(denom, "peggy"))
    }
    return ""
}
```

---

## 六、接口与实现要点

### 6.1 PeggyBridgeEVMClient

```go
// provider/chain/peggy_bridge_evm_client.go

type PeggyBridgeEVMClient struct {
    client   *ethclient.Client
    chainID  *big.Int
    timeout  time.Duration
}

// GetDepositTxHash 根据 event_nonce, sender, tokenContract, amount 查询 SendToCosmosEvent 对应的 tx hash
func (c *PeggyBridgeEVMClient) GetDepositTxHash(ctx context.Context, eventNonce uint64, eventHeight uint64, sender, tokenContract, amount string) (string, error)

// GetWithdrawalBatchTxHash 根据 batch_nonce, tokenContract 查询 TransactionBatchExecutedEvent 对应的 tx hash
func (c *PeggyBridgeEVMClient) GetWithdrawalBatchTxHash(ctx context.Context, batchNonce uint64, tokenContract string) (string, error)
```

### 6.2 PeggyEvmHashSyncService

```go
// internal/service/peggy_evm_hash_sync.go

type PeggyEvmHashSyncService struct {
    config       *PeggyEvmHashSyncConfig
    redisClient  *redis.Client
    evmClient    *chain.PeggyBridgeEVMClient
}

// Run 启动 worker，循环从 Redis 拉取任务并处理
func (s *PeggyEvmHashSyncService) Run(ctx context.Context)
```

### 6.3 边界情况处理

#### 6.3.1 Denom 解析边界

```go
func ParseTokenContractFromDenom(denom string) (string, error) {
    // 非 peggy 前缀
    if !strings.HasPrefix(denom, "peggy") {
        return "", ErrNotPeggyDenom
    }

    addr := strings.TrimPrefix(denom, "peggy")

    // 验证地址格式
    if !common.IsHexAddress(addr) {
        return "", ErrInvalidAddress
    }

    return converter.NormalizeHash(addr), nil
}
```

#### 6.3.2 Amount 精度匹配

```go
func MatchAmount(expected, actual string) (bool, error) {
    // 转换为 big.Int 进行精确比较
    expectedInt, ok := new(big.Int).SetString(expected, 10)
    if !ok {
        return false, fmt.Errorf("invalid expected amount: %s", expected)
    }

    actualInt, ok := new(big.Int).SetString(actual, 10)
    if !ok {
        return false, fmt.Errorf("invalid actual amount: %s", actual)
    }

    // 精确匹配（不使用误差，避免误匹配）
    return expectedInt.Cmp(actualInt) == 0, nil
}
```

#### 6.3.3 错误分类与处理

| 错误类型 | 处理策略 | 示例 |
|---------|---------|------|
| 网络超时 | 重试（指数退避） | RPC 连接超时 |
| RPC 限流 | 延迟重试 | 429 Too Many Requests |
| 无匹配日志 | 记录警告，不重试 | event_nonce 错误或数据不一致 |
| DB 更新失败 | 重试 | 临时性锁冲突 |
| Denom 解析失败 | 跳过，记录日志 | 非 peggy denom |

### 6.4 监控与可观测性

#### 6.4.1 指标收集

```go
// internal/metrics/peggy_evm_sync.go

var (
    syncQueueLength = prometheus.NewGaugeVec(prometheus.GaugeOpts{
        Name: "peggy_evm_sync_queue_length",
        Help: "Number of tasks waiting for EVM hash sync",
    }, []string{"chain"})

    syncDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "peggy_evm_sync_duration_seconds",
        Help:    "Duration of EVM hash sync operations",
        Buckets: prometheus.DefBuckets,
    }, []string{"operation", "status"})

    syncResult = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "peggy_evm_sync_result_total",
        Help: "Total number of sync operations",
    }, []string{"operation", "status"})
)

func RecordSync(operation, status string, duration time.Duration) {
    syncDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
    syncResult.WithLabelValues(operation, status).Inc()
}
```

#### 6.4.2 告警规则

```yaml
# prometheus/alerts.yml
groups:
  - name: peggy_evm_sync
    rules:
      - alert: PeggyEvmSyncQueueBacklog
        expr: peggy_evm_sync_queue_length > 1000
        for: 10m
        annotations:
          summary: "Peggy EVM sync queue backlog"
          description: "Queue length {{ $value }} exceeds threshold"

      - alert: PeggyEvmSyncHighFailureRate
        expr: rate(peggy_evm_sync_result_total{status="error"}[5m]) / rate(peggy_evm_sync_result_total[5m]) > 0.1
        for: 5m
        annotations:
          summary: "Peggy EVM sync high failure rate"
          description: "Failure rate {{ $value | humanizePercentage }} exceeds 10%"

      - alert: PeggyEvmSyncRPCDown
        expr: rate(peggy_evm_sync_result_total{operation="rpc_call",status="error"}[1m]) > 5
        for: 2m
        annotations:
          summary: "Peggy EVM RPC connection issues"
          description: "High RPC error rate detected"
```

#### 6.4.3 日志记录

```go
func (s *PeggyEvmHashSyncService) processTask(ctx context.Context, task *SyncTask) {
    start := time.Now()
    log := s.log.With(
        "task_type", task.Type,
        "event_nonce", task.EventNonce,
        "tx_hash", task.CosmosTxHash,
    )

    txHash, err := s.evmClient.GetDepositTxHash(ctx, task.EventNonce, ...)
    if err != nil {
        log.Error("Failed to get EVM tx hash", "error", err, "duration", time.Since(start))
        metrics.RecordSync("deposit", "error", time.Since(start))
        s.retryTask(ctx, task)
        return
    }

    if err := s.updateDB(ctx, task, txHash); err != nil {
        log.Error("Failed to update database", "error", err)
        metrics.RecordSync("deposit", "db_error", time.Since(start))
        return
    }

    log.Info("Successfully synced EVM hash", "evm_tx_hash", txHash, "duration", time.Since(start))
    metrics.RecordSync("deposit", "success", time.Since(start))
}
```

### 6.5 集成点

1. **Sync Service**：在 `saveCrossChainTransaction` 之后，对 `eth_tx_hash` 为空的 deposit/withdrawal 入队
2. **wire.go**：注入 `PeggyEvmHashSyncService`，随应用启动
3. **global**：增加 `PeggyBridgeEVMConfig`，在 `Init()` 中加载

---

## 七、时序与边界处理

### 7.1 Deposit

- `event_height` 为 **Ethereum 区块高度**
- 若配置的 RPC 为 archival 节点，可立即查询；否则需等待区块确认后再查
- 建议：`event_height` 落后当前区块 N 个块以上再查询（如 N=12）

### 7.2 Withdrawal

- `MsgWithdrawClaim` 在 Cosmos 上执行时，Ethereum 上的 `submitBatch` 通常已先执行
- 查询窗口：可根据 `block_timestamp` 推算大致 Ethereum 区块范围，或从最新区块向前扫描

### 7.3 失败与重试

- RPC 失败、无匹配 log：写入 `sync_error` 字段（若扩展）、重新入队延迟重试
- 重试次数、延迟由配置的 `retry` 控制

### 7.4 MsgSendToEth

- 仅表示提款意图，Ethereum 上尚无交易
- **不**为其查询 eth_tx_hash，保持为空
- 对应的 `eth_tx_hash` 在后续 `MsgWithdrawClaim` 关联的 batch 执行时，通过 `TransactionBatchExecutedEvent` 统一填充到该 batch 内所有 withdrawal

---

## 八、配置加载与代码生成

### 8.1 Proto 生成

修改 `conf.proto` 后需执行：

```bash
make config
# 或
buf generate
```

### 8.2 配置解析

Kratos 配置框架会根据 YAML 结构自动映射到 proto 定义；需确认生成的 Go 结构体包含 `PeggyBridgeEvm` 字段。

### 8.3 环境变量覆盖

Kratos 支持 `${VAR:default}` 语法，环境变量会覆盖 YAML 中的默认值，便于部署时切换测试网/主网。

---

## 九、实施步骤建议

| 阶段 | 内容 | 预估工作量 |
|------|------|-----------|
| 0 | **验证**：确认 event_height 语义、denom 格式、合约地址、事件签名、EventOutgoingBatch 事件 | 1-2 天 |
| 1 | 扩展 `conf.proto`、`config.yaml`、`.env.example`，增加 `peggy_bridge_evm` 配置 | 0.5 天 |
| 2 | **Withdrawal 前置改造**（必须）：创建 `peggy_batch_tx_mapping` 表、解析 EventOutgoingBatch、解析 EventWithdrawClaim 补充 denom | 1-2 天 |
| 3 | 实现 `PeggyBridgeEVMClient`，支持 `GetDepositTxHash`、`GetWithdrawalBatchTxHash`，含限流 | 1-2 天 |
| 4 | 实现 `PeggyEvmHashSyncService`，Redis 队列消费、DB 更新（同时更新两类 withdrawal 记录）、错误分类与重试 | 2-3 天 |
| 5 | 在 `saveCrossChainTransaction` 后入队 eth_tx_hash 为空的记录 | 0.5 天 |
| 6 | 更新 `saveCrossChainTransaction` 的 DoUpdates 包含 `eth_tx_hash` | 0.5 天 |
| 7 | 实现监控指标、告警规则、结构化日志 | 1 天 |
| 8 | wire 注入、global 配置加载、启动服务 | 0.5 天 |
| 9 | 单元测试与集成测试 | 1-2 天 |
| 10 | 历史数据回填脚本（可选） | 1 天 |

### 阶段 0：验证清单

```sql
-- 1. 确认 event_height 是 Ethereum 高度
SELECT event_height, block_height, block_timestamp
FROM peggy_deposit_txs
LIMIT 10;

-- 2. 确认 denom 格式
SELECT DISTINCT denom
FROM peggy_deposit_txs
WHERE denom LIKE 'peggy%'
LIMIT 10;

-- 3. 确认 event_nonce 范围
SELECT MIN(event_nonce), MAX(event_nonce), COUNT(*)
FROM peggy_deposit_txs;

-- 4. 确认待补充数据量
SELECT COUNT(*)
FROM peggy_deposit_txs
WHERE eth_tx_hash = '' OR eth_tx_hash IS NULL;

-- 5. 确认 EventOutgoingBatch 事件是否存在及格式
SELECT type, attributes
FROM abci_events
WHERE type LIKE '%OutgoingBatch%'
LIMIT 10;

-- 6. 分析 withdrawal 记录的两类分布
SELECT
    CASE WHEN outgoing_tx_id > 0 THEN 'MsgSendToEth' ELSE 'MsgWithdrawClaim' END AS record_type,
    COUNT(*) AS count,
    COUNT(CASE WHEN eth_tx_hash != '' THEN 1 END) AS has_evm_hash
FROM peggy_withdrawal_txs
GROUP BY record_type;
```

### A. Injective 与 Injective Peggy 差异

### A. Injective 与 Injective Peggy 差异

若 Injective 使用 Injective 的 Peggy  fork，事件结构应与 Gravity 一致。若存在差异，可通过配置 `bridge_contract` 或扩展事件签名列表适配。

### B. 参考合约地址

- Injective Mainnet Peggy: `0xF955C57f9EA9Dc8781965FEaE0b6A2acE2BAD6f3`（Ethereum 主网）
- Sepolia 测试网合约需根据实际部署确认

### C. 事件解析示例（Go）

```go
// 解析 SendToCosmosEvent 的 data 字段
// data: _destination (string) + _amount (uint256) + _eventNonce (uint256)
// 注意：string 为动态类型，需按 ABI 规范解析
```

具体 ABI 解析可使用 `github.com/ethereum/go-ethereum/accounts/abi`。

### D. 配置对比：两个 EVM 的区别

| 配置 | 用途 | 链类型 | 示例 Chain ID |
|------|------|--------|--------------|
| `Chain.evm` | Injective 自身的 EVM 层 | Injective EVM | 根据实际配置 |
| `Chain.peggy_bridge_evm` | Peggy 桥接的外部 EVM 链 | Sepolia / Ethereum | 11155111 / 1 |

**关键区别**：
- `Chain.evm`：查询 Injective 链上智能合约调用的 EVM 交易 hash
- `Chain.peggy_bridge_evm`：查询跨链桥接时，在以太坊测试网/主网上的原始交易 hash

### E. 历史数据回填策略

```go
// cmd/backfill-peggy-evm/main.go

func BackfillHistoricalData(ctx context.Context, db *gorm.DB, evmClient *PeggyBridgeEVMClient) error {
    var records []PeggyDepositTx

    // 查询所有待补充的记录
    db.Where("eth_tx_hash = '' OR eth_tx_hash IS NULL").Find(&records)

    log.Info("Found records to backfill", "count", len(records))

    for _, record := range records {
        tokenContract, err := ParseTokenContractFromDenom(record.Denom)
        if err != nil {
            log.Warn("Skip invalid denom", "denom", record.Denom)
            continue
        }

        txHash, err := evmClient.GetDepositTxHash(ctx, record.EventNonce, record.EventHeight, record.Sender, tokenContract, record.Amount)
        if err != nil {
            log.Error("Failed to get tx hash", "event_nonce", record.EventNonce, "error", err)
            continue
        }

        if err := db.Model(&record).Update("eth_tx_hash", txHash).Error; err != nil {
            log.Error("Failed to update DB", "id", record.ID, "error", err)
            continue
        }

        log.Info("Backfilled", "id", record.ID, "evm_tx_hash", txHash)
    }

    return nil
}
```

### F. 配置完整性检查清单

#### 配置文件
- [ ] `conf.proto` 已添加 `PeggyBridgeEvm` 消息定义（包含 4 个新增配置项）
- [ ] `config.yaml` 已添加 `peggy_bridge_evm` 配置块
- [ ] `.env.example` 已添加环境变量说明

#### 代码实现
- [ ] `global.go` 已添加 `PeggyBridgeEVMConfig` 全局变量
- [ ] `wire.go` 已注入 `PeggyEvmHashSyncService`
- [ ] `provider/chain/peggy_bridge_evm_client.go`：实现 RPC 限流
- [ ] `internal/service/peggy_evm_hash_sync.go`：实现并发 worker、分批更新

#### Withdrawal 前置改造（阶段 2，必须）
- [ ] 创建 `peggy_batch_tx_mapping` 表
- [ ] `crosschain_parser.go`：实现 `parseEventOutgoingBatch` 解析
- [ ] `crosschain_parser.go`：`parseMsgWithdrawClaim` 增加从 EventWithdrawClaim 解析 `token_contract` 并写入 Denom
- [ ] `savePeggyBatchTxMapping`：保存 batch → outgoing_tx_ids 映射

#### Withdrawal 更新逻辑
- [ ] `UpdateWithdrawalBatchTxHash`：同时更新两类记录
  - `WHERE outgoing_tx_id IN (?)`（MsgSendToEth，用户可见）
  - `WHERE batch_nonce = ? AND denom LIKE ?`（MsgWithdrawClaim，attestation）

#### 监控
- [ ] Prometheus 指标已注册（`internal/metrics/peggy_evm_sync.go`）
- [ ] 告警规则已配置（`prometheus/alerts.yml`）

### G. 故障排查指南

| 问题 | 可能原因 | 排查步骤 |
|------|---------|---------|
| 队列积压 | RPC 限流/故障、worker 数量不足 | 1. 检查 RPC 可用性 <br> 2. 查看 worker 日志 <br> 3. 检查限流配置 |
| 无匹配日志 | event_nonce 错误、合约地址错误、网络不匹配 | 1. 验证合约地址 <br> 2. 检查 chain_id 配置 <br> 3. 手动查询 eth_getLogs 验证 |
| DB 更新失败 | 锁冲突、字段类型不匹配 | 1. 检查 DoUpdates 包含 eth_tx_hash <br> 2. 查看慢查询日志 |
| denom 解析失败 | 非 peggy 跨链、格式异常 | 1. 确认 denom 前缀 <br> 2. 检查地址格式 |
| MsgSendToEth 未更新 | 缺少 batch 映射、EventOutgoingBatch 未解析 | 1. 检查 `peggy_batch_tx_mapping` 表 <br> 2. 确认 `parseEventOutgoingBatch` 已实现 <br> 3. 验证 outgoing_tx_id 是否在映射表中 |

---

## 十二、EventOutgoingBatch 事件详解

### 12.1 事件作用

`EventOutgoingBatch` 在 Relayer 向 Ethereum 提交 batch 前在 Cosmos 链上发出，用于：
1. 通知即将提交的 batch 信息
2. 提供 `batch_nonce` → `outgoing_tx_ids` 的映射关系
3. 允许索引器预先建立映射，便于后续关联 EVM tx hash

### 12.2 事件结构（基于 Gravity Bridge）

```go
// Cosmos ABCI Event
type EventOutgoingBatch struct {
    // Batch 唯一标识，与 MsgWithdrawClaim 中的 batch_nonce 对应
    BatchNonce uint64 `json:"batch_nonce"`

    // 该 batch 包含的所有提款交易 ID（即 outgoing_tx_id）
    BatchTxIDs []uint64 `json:"batch_tx_ids"`

    // Batch 中的 token contract 地址
    TokenContract string `json:"token_contract"`

    //区块信息
    BlockHeight   uint64 `json:"block_height"`
    BlockTimestamp int64 `json:"block_timestamp"`
}
```

### 12.3 事件类型名称

可能的变体（需在阶段 0 确认）：
- `EventOutgoingBatch`（标准）
- `OutgoingBatchTx`
- `batch_created`
- 或其他自定义名称

**验证方法**：
```sql
SELECT DISTINCT type FROM abci_events WHERE type LIKE '%batch%' OR type LIKE '%Outgoing%';
```

### 12.4 与其他事件的关系

```
MsgSendToEth (用户发起提款)
    ↓
EventOutgoingBatch (Relayer 组装 batch)
    ↓
submitBatch (Ethereum 上执行) → TransactionBatchExecutedEvent
    ↓
MsgWithdrawClaim (Cosmos 上确认)
```

### 12.5 历史数据回填

如果链上已有历史 EventOutgoingBatch 事件，需回填映射表：

```go
// cmd/backfill-batch-mapping/main.go

func BackfillBatchMapping(ctx context.Context, db *gorm.DB, chainProvider *ChainProvider) error {
    // 从最早区块开始扫描
    startHeight := uint64(1)  // 或从最早的 peggy_withdrawal_txs.block_height 开始

    for currentHeight := startHeight; ; currentHeight++ {
        block, err := chainProvider.Block(ctx, currentHeight)
        if err != nil {
            break  // 到达最新区块
        }

        // 解析 EventOutgoingBatch
        for _, tx := range block.Txs {
            for _, event := range tx.Log.Events {
                if event.Type == "EventOutgoingBatch" {
                    mapping := parseEventOutgoingBatch(event, currentHeight, block.Time)
                    if err := db.Create(mapping).Error; err != nil {
                        // 处理唯一键冲突（已存在）
                    }
                }
            }
        }

        log.Info("Backfilled batch mapping", "height", currentHeight)
    }

    return nil
}
```

---

## 十一、方案评审与遗漏补充

### 11.1 Withdrawal 更新逻辑的关键遗漏

**问题**：`peggy_withdrawal_txs` 中存在两类记录，更新策略需区分：

| 记录来源 | 唯一键 | 有 denom | 有 batch_nonce | 用户可见 |
|---------|-------|----------|----------------|---------|
| MsgSendToEth | outgoing_tx_id | ✅ | ❌（通常为 0） | ✅ |
| MsgWithdrawClaim | (block_height, batch_nonce, tx_hash) | ❌ | ✅ |  attestation 记录 |

当前设计中的 `WHERE batch_nonce = ? AND denom LIKE ?` 无法同时匹配两类记录：
- MsgSendToEth 有 denom 但**无** batch_nonce（无消息会写入该字段）
- MsgWithdrawClaim 有 batch_nonce 但**无** denom（`parseMsgWithdrawClaim` 未解析）

**实施方案**：

> ✅ 业务确认：用户需要看到 MsgSendToEth 的 evmHash，需实施方案 2

1. **MsgWithdrawClaim 补充 token_contract**：从 EventWithdrawClaim 的 `token_contract` 解析，写入 Denom（`peggy0x...`），以便用 `batch_nonce + denom` 更新 attestation 记录。

2. **解析 EventOutgoingBatch 建立映射**（必须）：EventOutgoingBatch 包含 `batch_tx_ids`（即 `outgoing_tx_id` 列表）。
   - 在区块处理时解析 EventOutgoingBatch，维护 `batch_nonce -> [outgoing_tx_ids]` 映射表（`peggy_batch_tx_mapping`）
   - 获取 EVM tx hash 后：**同时**更新两类记录
     - `WHERE outgoing_tx_id IN (batch_tx_ids)`（MsgSendToEth 记录，用户可见）
     - `WHERE batch_nonce = ? AND denom LIKE ?`（MsgWithdrawClaim 记录，attestation）

### 11.2 配置项说明

新增配置项已整合到 Proto 定义（§3.1）和 config.yaml（§3.2）中：

| 配置项 | 字段 | 默认值 | 说明 |
|-------|------|--------|------|
| `rpc_rate_limit` | `int32 rpc_rate_limit = 8` | 10 | RPC 限流（§5.5.2），每秒请求数 |
| `worker_count` | `int32 worker_count = 9` | 3 | worker 并发数（§5.5.3） |
| `min_confirm_blocks` | `int32 min_confirm_blocks = 10` | 12 | Deposit 确认块数（§7.1） |
| `batch_update_size` | `int32 batch_update_size = 11` | 100 | 分批更新大小（§5.5.1） |

### 11.3 任务去重与幂等

- **重复入队**：同一记录可能多次入队（重试、多 worker）。可参考 `evm_details_sync` 的 `RequestCoalescer` 或使用 Redis Set 记录处理中的任务，防止并发重复处理。
- **更新幂等**：更新时加 `WHERE eth_tx_hash = '' OR eth_tx_hash IS NULL`，仅更新未填充记录，避免覆盖已有值。

### 11.4 优雅停机

- 停止时等待当前任务完成或重新入队，避免数据处于中间状态。
- 可在 `Run()` 中监听 `ctx.Done()`，收到信号后停止拉取新任务、等待进行中任务完成再退出。

### 11.5 bridge_contract 过滤

当 `bridge_contract` 非空时，`eth_getLogs` 应增加 `address` 参数，仅查询该合约日志，缩小搜索范围并提高准确性。

### 11.6 环境变量表补充

| 变量 | 说明 |
|------|------|
| `PEGGY_BRIDGE_EVM_QUEUE_KEY` | Redis 队列 key，默认 `peggy:evm_hash:queue` |

### 11.7 与 EventOutgoingBatch 的依赖

✅ 业务确认需要实施方案 2，以下验证为**必须**：

**阶段 0 验证清单**：
- [ ] Injective 链是否会发出 `EventOutgoingBatch`（或等价事件）
- [ ] 该事件的 `batch_tx_ids` 字段格式（是否为 `outgoing_tx_id` 列表）
- [ ] 该事件的 `batch_nonce` 字段与 MsgWithdrawClaim 中的 `batch_nonce` 是否一致
- [ ] 当前链上是否存在 EventOutgoingBatch 历史数据（用于回填映射表）
