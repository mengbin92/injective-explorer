# ChainClientV2替换ExplorerClient技术方案

**文档版本**: 1.0  
**创建日期**: 2026-01-14  
**目标**: 使用ChainClientV2接口替换ExplorerClient接口

---

## 目录

- [一、接口映射关系](#一接口映射关系)
- [二、实现方案](#二实现方案)
- [三、数据格式转换](#三数据格式转换)
- [四、不可直接实现的功能](#四不可直接实现的功能)
- [五、实施步骤](#五实施步骤)
- [六、代码示例](#六代码示例)

---

## 一、接口映射关系

### 1.1 ExplorerClient接口列表

| 方法名          | 参数                                           | 返回值                                          | 功能说明                        |
| --------------- | ---------------------------------------------- | ----------------------------------------------- | ------------------------------- |
| GetBlock        | height string                                  | *injective_explorer_rpcpb.GetBlockResponse      | 根据高度获取区块信息            |
| GetTxByTxHash   | txHash string                                  | *injective_explorer_rpcpb.GetTxByTxHashResponse | 根据交易哈希获取交易详情        |
| GetAccountTxs   | *injective_explorer_rpcpb.GetAccountTxsRequest | *injective_explorer_rpcpb.GetAccountTxsResponse | 获取指定账户的交易列表          |
| GetBlocks       | 无                                             | *injective_explorer_rpcpb.GetBlocksResponse     | 获取区块列表                    |
| GetTxs          | *injective_explorer_rpcpb.GetTxsRequest        | *injective_explorer_rpcpb.GetTxsResponse        | 获取交易列表                    |
| StreamBlocks    | 无                                             | (StreamBlocksClient, error)                     | 创建区块流（WebSocket实时推送） |
| FetchValidators | 无                                             | *injective_explorer_rpcpb.GetValidatorsResponse | 获取验证者列表                  |
| FetchValidator  | validatorAddress string                        | *injective_explorer_rpcpb.GetValidatorResponse  | 获取指定验证者信息              |

### 1.2 ChainClientV2可用接口

| ChainClientV2方法              | 参数                          | 返回值                                                      | 说明               |
| ------------------------------ | ----------------------------- | ----------------------------------------------------------- | ------------------ |
| FetchBlockByHeight             | ctx, height uint64            | (*tmtypes.Block, error)                                     | 根据高度获取区块   |
| GetTx                          | ctx, hash string              | (*sdk.TxResponse, error)                                    | 根据哈希获取交易   |
| GetAccount                     | ctx, address string           | (*authtypes.BaseAccount, error)                             | 获取账户信息       |
| GetBankBalances                | ctx, address string           | (*banktypes.QueryAllBalancesResponse, error)                | 获取账户余额       |
| FetchValidatorSetByHeight      | ctx, height uint64, page *int | (*tmtypes.ResultValidators, error)                          | 获取验证者集合     |
| FetchValidatorDistributionInfo | ctx, validatorAddr string     | (*distrtypes.QueryValidatorDistributionInfoResponse, error) | 获取验证者分发信息 |
| LatestBlockHeight              | ctx                           | (uint64, error)                                             | 获取最新区块高度   |

### 1.3 接口映射表

| ExplorerClient方法 | ChainClientV2对应方法                  | 实现难度 | 备注                     |
| ------------------ | -------------------------------------- | -------- | ------------------------ |
| GetBlock           | FetchBlockByHeight                     | ⭐ 简单   | 需要数据格式转换         |
| GetTxByTxHash      | GetTx                                  | ⭐ 简单   | 需要数据格式转换         |
| GetAccountTxs      | ❌ 无直接对应                           | 🔴 困难   | 需要本地索引或全链扫描   |
| GetBlocks          | FetchBlockByHeight + LatestBlockHeight | 🟡 中等   | 需要循环调用             |
| GetTxs             | GetTx + 区块扫描                       | 🔴 困难   | 需要扫描区块获取交易     |
| StreamBlocks       | ❌ 无直接对应                           | 🟡 中等   | 需要轮询或Tendermint订阅 |
| FetchValidators    | FetchValidatorSetByHeight              | ⭐ 简单   | 需要数据格式转换         |
| FetchValidator     | FetchValidatorDistributionInfo         | ⭐ 简单   | 需要数据格式转换         |

---

## 二、实现方案

### 2.1 方案架构

```
┌─────────────────────────────────────────────────────────────┐
│                  ExplorerClient适配器层                        │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  ChainClientV2Adapter                                    │ │
│  │  - 实现ExplorerClient接口                                │ │
│  │  - 调用ChainClientV2方法                                 │ │
│  │  - 数据格式转换                                          │ │
│  └─────────────────────────────────────────────────────────┘ │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ↓
┌─────────────────────────────────────────────────────────────┐
│                  ChainClientV2                              │
│  - FetchBlockByHeight                                       │
│  - GetTx                                                    │
│  - GetAccount                                               │
│  - GetBankBalances                                          │
│  - FetchValidatorSetByHeight                                │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ↓
┌─────────────────────────────────────────────────────────────┐
│              Tendermint RPC / Cosmos SDK                    │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 适配器接口定义

```go
// ChainClientV2Adapter 实现ExplorerClient接口
type ChainClientV2Adapter struct {
    chainClient chainclient.ChainClientV2
    tmClient    tmclient.TendermintClient
    // 可选：本地缓存和索引
    cache       cache.Cache
    indexer     *AccountTxIndexer  // 用于账户交易历史
}

// 实现ExplorerClient接口
var _ explorerclient.ExplorerClient = (*ChainClientV2Adapter)(nil)
```

---

## 三、数据格式转换

### 3.1 区块数据转换

**ExplorerClient返回格式**: `*injective_explorer_rpcpb.GetBlockResponse`
**ChainClientV2返回格式**: `*tmtypes.Block`

```go
func (a *ChainClientV2Adapter) convertBlockToExplorerFormat(
    tmBlock *tmtypes.Block,
) *injective_explorer_rpcpb.GetBlockResponse {
    // 转换逻辑
    return &injective_explorer_rpcpb.GetBlockResponse{
        Data: &injective_explorer_rpcpb.Block{
            Height:    uint64(tmBlock.Height),
            Hash:      tmBlock.Hash().String(),
            Timestamp: tmBlock.Time.Unix(),
            TxCount:   uint64(len(tmBlock.Txs)),
            // ... 其他字段映射
        },
    }
}
```

### 3.2 交易数据转换

**ExplorerClient返回格式**: `*injective_explorer_rpcpb.GetTxByTxHashResponse`
**ChainClientV2返回格式**: `*sdk.TxResponse`

```go
func (a *ChainClientV2Adapter) convertTxToExplorerFormat(
    txResponse *sdk.TxResponse,
    tmBlock *tmtypes.Block,
) *injective_explorer_rpcpb.GetTxByTxHashResponse {
    // 转换逻辑
    return &injective_explorer_rpcpb.GetTxByTxHashResponse{
        Data: &injective_explorer_rpcpb.Tx{
            Hash:      txResponse.TxHash,
            Height:    uint64(txResponse.Height),
            Code:      txResponse.Code,
            GasUsed:   txResponse.GasUsed,
            GasWanted: txResponse.GasWanted,
            // ... 其他字段映射
        },
    }
}
```

### 3.3 验证者数据转换

**ExplorerClient返回格式**: `*injective_explorer_rpcpb.GetValidatorsResponse`
**ChainClientV2返回格式**: `*tmtypes.ResultValidators`

```go
func (a *ChainClientV2Adapter) convertValidatorsToExplorerFormat(
    validators *tmtypes.ResultValidators,
) *injective_explorer_rpcpb.GetValidatorsResponse {
    // 转换逻辑
    explorerValidators := make([]*injective_explorer_rpcpb.Validator, 0)
    for _, v := range validators.Validators {
        explorerValidators = append(explorerValidators, &injective_explorer_rpcpb.Validator{
            Address:     v.Address.String(),
            VotingPower: v.VotingPower,
            // ... 其他字段映射
        })
    }
    return &injective_explorer_rpcpb.GetValidatorsResponse{
        Data: explorerValidators,
    }
}
```

---

## 四、不可直接实现的功能

### 4.1 GetAccountTxs - 账户交易历史

**问题**: ChainClientV2无法直接查询账户的交易历史，因为区块链底层不维护"账户→交易"索引。

**解决方案**:

#### 方案A: 本地索引（推荐）

```go
// 1. 创建账户交易索引表
CREATE TABLE account_transaction_index (
    id BIGSERIAL PRIMARY KEY,
    address VARCHAR(42) NOT NULL,
    tx_hash VARCHAR(66) NOT NULL,
    block_height BIGINT NOT NULL,
    block_time TIMESTAMP NOT NULL,
    role VARCHAR(10) NOT NULL,  -- 'signer' or 'recipient'
    INDEX idx_address_block_time (address, block_time DESC),
    INDEX idx_tx_hash (tx_hash)
);

// 2. 实现索引服务
type AccountTxIndexer struct {
    db *gorm.DB
}

// 3. 增量同步：监听新区块，解析交易，更新索引
func (i *AccountTxIndexer) SyncBlock(ctx context.Context, block *tmtypes.Block) error {
    for _, tx := range block.Txs {
        txResponse, err := i.chainClient.GetTx(ctx, hex.EncodeToString(tx.Hash()))
        if err != nil {
            continue
        }
        
        // 提取交易中的地址
        signers := extractSigners(txResponse)
        recipients := extractRecipients(txResponse)
        
        // 保存到索引表
        for _, addr := range signers {
            i.db.Create(&AccountTransactionIndex{
                Address:    addr,
                TxHash:     txResponse.TxHash,
                BlockHeight: block.Height,
                BlockTime:   block.Time,
                Role:       "signer",
            })
        }
        // ... recipients类似处理
    }
    return nil
}

// 4. 查询接口
func (a *ChainClientV2Adapter) GetAccountTxs(
    ctx context.Context,
    req *injective_explorer_rpcpb.GetAccountTxsRequest,
) (*injective_explorer_rpcpb.GetAccountTxsResponse, error) {
    // 从索引表查询
    var indices []AccountTransactionIndex
    a.indexer.db.Where("address = ?", req.Address).
        Order("block_time DESC").
        Limit(int(req.Limit)).
        Offset(int(req.Skip)).
        Find(&indices)
    
    // 根据tx_hash获取完整交易信息
    txHashes := make([]string, len(indices))
    for i, idx := range indices {
        txHashes[i] = idx.TxHash
    }
    
    // 批量获取交易
    txs := a.batchGetTxs(ctx, txHashes)
    
    return &injective_explorer_rpcpb.GetAccountTxsResponse{
        Data: txs,
        Paging: &injective_explorer_rpcpb.Paging{
            Total: uint64(len(txs)),
        },
    }, nil
}
```

**工作量**: 2-3周
- 索引表设计: 2-3天
- 增量同步逻辑: 1周
- 查询接口: 2-3天
- 测试验证: 2-3天

#### 方案B: 全链扫描（不推荐）

```go
func (a *ChainClientV2Adapter) GetAccountTxs(
    ctx context.Context,
    req *injective_explorer_rpcpb.GetAccountTxsRequest,
) (*injective_explorer_rpcpb.GetAccountTxsResponse, error) {
    // ⚠️ 性能问题：需要扫描所有历史区块
    latestHeight, _ := a.chainClient.LatestBlockHeight(ctx)
    var matchingTxs []*injective_explorer_rpcpb.Tx
    
    // 从最新区块开始扫描（限制范围）
    startHeight := latestHeight - 10000  // 只扫描最近1万个区块
    for height := latestHeight; height >= startHeight && len(matchingTxs) < int(req.Limit); height-- {
        block, _ := a.chainClient.FetchBlockByHeight(ctx, height)
        for _, tx := range block.Txs {
            txResponse, _ := a.chainClient.GetTx(ctx, hex.EncodeToString(tx.Hash()))
            if isAccountRelated(txResponse, req.Address) {
                matchingTxs = append(matchingTxs, convertTx(txResponse))
            }
        }
    }
    
    return &injective_explorer_rpcpb.GetAccountTxsResponse{
        Data: matchingTxs,
    }, nil
}
```

**问题**: 
- 性能极差（扫描1万个区块需要数分钟）
- 无法获取完整历史
- 资源消耗大

### 4.2 GetBlocks - 区块列表

**问题**: ChainClientV2没有批量获取区块的接口。

**解决方案**: 循环调用 + 并发优化

```go
func (a *ChainClientV2Adapter) GetBlocks(
    ctx context.Context,
) (*injective_explorer_rpcpb.GetBlocksResponse, error) {
    // 1. 获取最新高度
    latestHeight, err := a.chainClient.LatestBlockHeight(ctx)
    if err != nil {
        return nil, err
    }
    
    // 2. 确定要获取的区块范围（例如最近20个）
    limit := 20
    startHeight := latestHeight - uint64(limit) + 1
    if startHeight < 1 {
        startHeight = 1
    }
    
    // 3. 并发获取区块
    type blockResult struct {
        height uint64
        block  *tmtypes.Block
        err    error
    }
    
    resultCh := make(chan blockResult, limit)
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, 10)  // 限制并发数为10
    
    for height := startHeight; height <= latestHeight; height++ {
        wg.Add(1)
        go func(h uint64) {
            defer wg.Done()
            semaphore <- struct{}{}
            defer func() { <-semaphore }()
            
            block, err := a.chainClient.FetchBlockByHeight(ctx, h)
            resultCh <- blockResult{height: h, block: block, err: err}
        }(height)
    }
    
    go func() {
        wg.Wait()
        close(resultCh)
    }()
    
    // 4. 收集结果并排序
    blocks := make([]*tmtypes.Block, 0, limit)
    results := make(map[uint64]*tmtypes.Block)
    for result := range resultCh {
        if result.err == nil {
            results[result.height] = result.block
        }
    }
    
    for height := startHeight; height <= latestHeight; height++ {
        if block, ok := results[height]; ok {
            blocks = append(blocks, block)
        }
    }
    
    // 5. 转换为Explorer格式
    explorerBlocks := make([]*injective_explorer_rpcpb.Block, len(blocks))
    for i, block := range blocks {
        explorerBlocks[i] = a.convertBlockToExplorerFormat(block).Data
    }
    
    return &injective_explorer_rpcpb.GetBlocksResponse{
        Data: explorerBlocks,
    }, nil
}
```

**性能影响**: 
- 顺序调用: 20个区块 × 200ms = 4秒
- 并发调用(10并发): (20/10) × 200ms = 400ms

### 4.3 GetTxs - 交易列表

**问题**: ChainClientV2没有直接获取交易列表的接口。

**解决方案**: 从区块中提取交易

```go
func (a *ChainClientV2Adapter) GetTxs(
    ctx context.Context,
    req *injective_explorer_rpcpb.GetTxsRequest,
) (*injective_explorer_rpcpb.GetTxsResponse, error) {
    // 1. 获取最新高度
    latestHeight, err := a.chainClient.LatestBlockHeight(ctx)
    if err != nil {
        return nil, err
    }
    
    // 2. 确定扫描范围
    limit := int(req.Limit)
    if limit > 100 {
        limit = 100  // 限制最大数量
    }
    
    // 3. 从最新区块开始扫描，收集交易
    var allTxs []*injective_explorer_rpcpb.Tx
    scannedBlocks := 0
    maxScanBlocks := 1000  // 最多扫描1000个区块
    
    for height := latestHeight; height > 0 && len(allTxs) < limit && scannedBlocks < maxScanBlocks; height-- {
        block, err := a.chainClient.FetchBlockByHeight(ctx, height)
        if err != nil {
            continue
        }
        
        // 提取区块中的交易
        for _, tx := range block.Txs {
            if len(allTxs) >= limit {
                break
            }
            
            txResponse, err := a.chainClient.GetTx(ctx, hex.EncodeToString(tx.Hash()))
            if err != nil {
                continue
            }
            
            // 时间过滤
            if req.StartTime > 0 && txResponse.Timestamp < req.StartTime {
                continue
            }
            if req.EndTime > 0 && txResponse.Timestamp > req.EndTime {
                continue
            }
            
            explorerTx := a.convertTxToExplorerFormat(txResponse, block)
            allTxs = append(allTxs, explorerTx.Data)
        }
        
        scannedBlocks++
    }
    
    return &injective_explorer_rpcpb.GetTxsResponse{
        Data: allTxs,
        Paging: &injective_explorer_rpcpb.Paging{
            Total: uint64(len(allTxs)),
        },
    }, nil
}
```

**性能影响**: 
- 扫描100个交易可能需要查询10-20个区块
- 每个区块200ms，总耗时: 2-4秒

### 4.4 StreamBlocks - 区块流式推送

**问题**: ChainClientV2本身没有流式API。

**答案**: ✅ **可以实现**，但需要通过底层的TendermintClient来实现。

**实现方式**: 使用Tendermint的WebSocket订阅功能

#### 方案A: Tendermint订阅（推荐，实时性好）

ChainClientV2虽然本身没有流式API，但可以通过TendermintClient订阅`NewBlock`事件来实现出块监听。

**实现原理**:
- ChainClientV2底层使用Tendermint RPC客户端
- Tendermint支持WebSocket订阅，可以订阅`tm.event='NewBlock'`事件
- 当新区块产生时，Tendermint会通过WebSocket推送事件

**代码实现**:

```go
func (a *ChainClientV2Adapter) StreamBlocks(
    ctx context.Context,
) (<-chan *injective_explorer_rpcpb.StreamBlocksResponse, error) {
    ch := make(chan *injective_explorer_rpcpb.StreamBlocksResponse)
    
    go func() {
        defer close(ch)
        
        // 获取Tendermint客户端（ChainClientV2内部已包含）
        // 或者使用独立的TendermintClient
        tmClient := a.tmClient  // 从适配器获取
        
        // 订阅NewBlock事件
        // 注意：需要检查TendermintClient是否支持Subscribe方法
        // 如果使用rpchttp客户端，需要转换为WebSocket客户端
        
        // 方式1: 使用Tendermint WebSocket客户端
        wsClient, err := rpcclient.NewWSClient(
            global.ChainConfig.TmEndpoint,
            "/websocket",
        )
        if err != nil {
            return
        }
        defer wsClient.Stop()
        
        // 订阅NewBlock事件
        query := "tm.event='NewBlock'"
        eventCh, err := wsClient.Subscribe(ctx, "block-stream", query)
        if err != nil {
            return
        }
        
        // 处理事件
        for {
            select {
            case <-ctx.Done():
                return
            case event, ok := <-eventCh:
                if !ok {
                    return
                }
                
                // 解析Tendermint事件
                block := extractBlockFromTendermintEvent(event)
                if block != nil {
                    // 使用ChainClientV2获取完整区块信息
                    fullBlock, err := a.chainClient.FetchBlockByHeight(
                        ctx,
                        uint64(block.Height),
                    )
                    if err != nil {
                        continue
                    }
                    
                    // 转换为Explorer格式
                    explorerBlock := a.convertBlockToExplorerFormat(fullBlock)
                    ch <- &injective_explorer_rpcpb.StreamBlocksResponse{
                        Block: explorerBlock.Data,
                        Type:  "new",
                    }
                }
            }
        }
    }()
    
    return ch, nil
}

// 从Tendermint事件中提取区块信息
func extractBlockFromTendermintEvent(event interface{}) *tmtypes.Block {
    // 解析Tendermint事件格式
    // 事件结构: {"type": "tendermint/event/NewBlock", "value": {...}}
    // 需要根据实际的SDK实现来解析
    // ...
}
```

**优势**:
- ✅ 实时性好：延迟 < 100ms
- ✅ 资源消耗低：事件驱动，无需轮询
- ✅ 稳定性高：Tendermint原生支持

**注意事项**:
1. 需要WebSocket连接（不是HTTP）
2. 需要处理连接断开和重连
3. 需要解析Tendermint事件格式

#### 方案B: 轮询（简单但延迟高）

```go
func (a *ChainClientV2Adapter) StreamBlocks(
    ctx context.Context,
) (<-chan *injective_explorer_rpcpb.StreamBlocksResponse, error) {
    ch := make(chan *injective_explorer_rpcpb.StreamBlocksResponse)
    
    go func() {
        defer close(ch)
        
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()
        
        var lastHeight uint64 = 0
        
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                // 获取最新高度
                latestHeight, err := a.chainClient.LatestBlockHeight(ctx)
                if err != nil {
                    continue
                }
                
                // 推送新区块
                if latestHeight > lastHeight {
                    for h := lastHeight + 1; h <= latestHeight; h++ {
                        block, err := a.chainClient.FetchBlockByHeight(ctx, h)
                        if err != nil {
                            continue
                        }
                        
                        explorerBlock := a.convertBlockToExplorerFormat(block)
                        ch <- &injective_explorer_rpcpb.StreamBlocksResponse{
                            Block: explorerBlock.Data,
                            Type:  "new",
                        }
                    }
                    lastHeight = latestHeight
                }
            }
        }
    }()
    
    return ch, nil
}
```

**问题**: 
- 延迟: 0-1秒（平均500ms）
- 资源浪费: 每秒查询一次
- 不适合生产环境

#### 方案C: 使用项目现有的EventBus机制（推荐用于内部服务）

```go
func (a *ChainClientV2Adapter) StreamBlocks(
    ctx context.Context,
) (<-chan *injective_explorer_rpcpb.StreamBlocksResponse, error) {
    ch := make(chan *injective_explorer_rpcpb.StreamBlocksResponse)
    
    go func() {
        defer close(ch)
        
        // 使用Tendermint客户端订阅新区块事件
        eventCh, err := a.tmClient.Subscribe(ctx, "tm.event='NewBlock'")
        if err != nil {
            return
        }
        
        for {
            select {
            case <-ctx.Done():
                return
            case event := <-eventCh:
                // 解析Tendermint事件
                block := extractBlockFromEvent(event)
                if block != nil {
                    explorerBlock := a.convertBlockToExplorerFormat(block)
                    ch <- &injective_explorer_rpcpb.StreamBlocksResponse{
                        Block: explorerBlock.Data,
                        Type:  "new",
                    }
                }
            }
        }
    }()
    
    return ch, nil
}
```

**优势**: 
- 延迟: <100ms
- 实时推送
- 资源消耗低

**注意**: 如果项目已经有SyncService在监听区块并发布到EventBus，可以直接使用EventBus：

```go
func (a *ChainClientV2Adapter) StreamBlocks(
    ctx context.Context,
) (<-chan *injective_explorer_rpcpb.StreamBlocksResponse, error) {
    ch := make(chan *injective_explorer_rpcpb.StreamBlocksResponse)
    
    go func() {
        defer close(ch)
        
        // 使用项目现有的EventBus
        eventBus := GetEventBus()
        blockChan := eventBus.SubscribeBlocks()
        defer eventBus.UnsubscribeBlocks(blockChan)
        
        for {
            select {
            case <-ctx.Done():
                return
            case blockEvent, ok := <-blockChan:
                if !ok {
                    return
                }
                
                if blockEvent != nil && blockEvent.Block != nil {
                    blockDTO := models.ToBlockDTO(blockEvent.Block, models.DTOModeWithoutDetail)
                    updateType := "new"
                    if blockEvent.Type == EventTypeBlockUpdate {
                        updateType = "update"
                    }
                    
                    ch <- &injective_explorer_rpcpb.StreamBlocksResponse{
                        Block: blockDTO,
                        Type:  updateType,
                    }
                }
            }
        }
    }()
    
    return ch, nil
}
```

这种方式的好处是：
- 复用现有的区块同步逻辑
- 不需要额外的Tendermint连接
- 与现有架构集成良好

---

## 五、实施步骤

### 阶段1: 基础接口实现（1周）

1. **创建适配器结构**
   - 定义 `ChainClientV2Adapter`
   - 实现基础接口：`GetBlock`, `GetTxByTxHash`, `FetchValidators`, `FetchValidator`

2. **数据格式转换**
   - 实现区块转换函数
   - 实现交易转换函数
   - 实现验证者转换函数

3. **单元测试**
   - 测试每个转换函数
   - 测试基础接口调用

### 阶段2: 批量查询实现（1-2周）

1. **GetBlocks实现**
   - 实现并发获取区块逻辑
   - 优化性能（连接池、缓存）

2. **GetTxs实现**
   - 实现区块扫描逻辑
   - 实现时间过滤

3. **性能优化**
   - 添加缓存层
   - 优化并发控制

### 阶段3: 账户交易历史（2-3周）

1. **索引表设计**
   - 设计数据库表结构
   - 创建索引

2. **增量同步服务**
   - 实现区块监听
   - 实现交易解析
   - 实现索引更新

3. **查询接口**
   - 实现 `GetAccountTxs`
   - 实现分页和过滤

### 阶段4: 流式推送（1-2周）

1. **Tendermint订阅**
   - 集成Tendermint客户端
   - 实现事件订阅

2. **数据转换**
   - 事件格式转换
   - 错误处理

### 阶段5: 集成测试（1周）

1. **功能测试**
   - 测试所有接口
   - 测试边界情况

2. **性能测试**
   - 压力测试
   - 性能对比

3. **生产部署**
   - 灰度发布
   - 监控告警

---

## 六、代码示例

### 6.1 完整适配器实现框架

```go
package adapter

import (
    "context"
    "encoding/hex"
    "sync"
    "time"
    
    chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
    tmclient "github.com/InjectiveLabs/sdk-go/client/tm"
    injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
)

// ChainClientV2Adapter 实现ExplorerClient接口
type ChainClientV2Adapter struct {
    chainClient chainclient.ChainClientV2
    tmClient    tmclient.TendermintClient
    indexer     *AccountTxIndexer  // 可选
    cache       Cache              // 可选
}

// NewChainClientV2Adapter 创建适配器实例
func NewChainClientV2Adapter(
    chainClient chainclient.ChainClientV2,
    tmClient tmclient.TendermintClient,
) *ChainClientV2Adapter {
    return &ChainClientV2Adapter{
        chainClient: chainClient,
        tmClient:    tmClient,
    }
}

// GetBlock 根据高度获取区块
func (a *ChainClientV2Adapter) GetBlock(
    ctx context.Context,
    height string,
) (*injective_explorer_rpcpb.GetBlockResponse, error) {
    // 解析高度
    blockHeight := parseHeight(height)
    
    // 调用ChainClientV2
    tmBlock, err := a.chainClient.FetchBlockByHeight(ctx, blockHeight)
    if err != nil {
        return nil, err
    }
    
    // 转换格式
    return a.convertBlockToExplorerFormat(tmBlock), nil
}

// GetTxByTxHash 根据交易哈希获取交易详情
func (a *ChainClientV2Adapter) GetTxByTxHash(
    ctx context.Context,
    txHash string,
) (*injective_explorer_rpcpb.GetTxByTxHashResponse, error) {
    // 清理哈希格式
    hash := cleanTxHash(txHash)
    
    // 调用ChainClientV2
    txResponse, err := a.chainClient.GetTx(ctx, hash)
    if err != nil {
        return nil, err
    }
    
    // 获取区块信息（用于补充数据）
    block, _ := a.chainClient.FetchBlockByHeight(ctx, uint64(txResponse.Height))
    
    // 转换格式
    return a.convertTxToExplorerFormat(txResponse, block), nil
}

// GetAccountTxs 获取指定账户的交易列表
func (a *ChainClientV2Adapter) GetAccountTxs(
    ctx context.Context,
    req *injective_explorer_rpcpb.GetAccountTxsRequest,
) (*injective_explorer_rpcpb.GetAccountTxsResponse, error) {
    // 如果有索引器，使用索引查询
    if a.indexer != nil {
        return a.indexer.GetAccountTxs(ctx, req)
    }
    
    // 否则使用全链扫描（性能较差）
    return a.scanAccountTxs(ctx, req)
}

// GetBlocks 获取区块列表
func (a *ChainClientV2Adapter) GetBlocks(
    ctx context.Context,
) (*injective_explorer_rpcpb.GetBlocksResponse, error) {
    // 实现见4.2节
    return a.getBlocksConcurrent(ctx)
}

// GetTxs 获取交易列表
func (a *ChainClientV2Adapter) GetTxs(
    ctx context.Context,
    req *injective_explorer_rpcpb.GetTxsRequest,
) (*injective_explorer_rpcpb.GetTxsResponse, error) {
    // 实现见4.3节
    return a.scanTxsFromBlocks(ctx, req)
}

// StreamBlocks 创建区块流
func (a *ChainClientV2Adapter) StreamBlocks(
    ctx context.Context,
) (<-chan *injective_explorer_rpcpb.StreamBlocksResponse, error) {
    // 实现见4.4节方案B
    return a.streamBlocksViaSubscription(ctx)
}

// FetchValidators 获取验证者列表
func (a *ChainClientV2Adapter) FetchValidators(
    ctx context.Context,
) (*injective_explorer_rpcpb.GetValidatorsResponse, error) {
    // 获取最新高度
    latestHeight, err := a.chainClient.LatestBlockHeight(ctx)
    if err != nil {
        return nil, err
    }
    
    // 获取验证者集合
    validators, err := a.chainClient.FetchValidatorSetByHeight(ctx, latestHeight, nil)
    if err != nil {
        return nil, err
    }
    
    // 转换格式
    return a.convertValidatorsToExplorerFormat(validators), nil
}

// FetchValidator 获取指定验证者信息
func (a *ChainClientV2Adapter) FetchValidator(
    ctx context.Context,
    validatorAddress string,
) (*injective_explorer_rpcpb.GetValidatorResponse, error) {
    // 获取验证者分发信息
    distInfo, err := a.chainClient.FetchValidatorDistributionInfo(ctx, validatorAddress)
    if err != nil {
        return nil, err
    }
    
    // 转换格式
    return a.convertValidatorToExplorerFormat(distInfo), nil
}

// 辅助函数
func (a *ChainClientV2Adapter) convertBlockToExplorerFormat(
    tmBlock *tmtypes.Block,
) *injective_explorer_rpcpb.GetBlockResponse {
    // 实现转换逻辑
    // ...
}

func (a *ChainClientV2Adapter) convertTxToExplorerFormat(
    txResponse *sdk.TxResponse,
    block *tmtypes.Block,
) *injective_explorer_rpcpb.GetTxByTxHashResponse {
    // 实现转换逻辑
    // ...
}
```

### 6.2 使用示例

```go
// 初始化适配器
chainClient, _ := GetSharedChainClient()
tmClient := GetSharedTmClient()
adapter := NewChainClientV2Adapter(chainClient, tmClient)

// 使用适配器（接口与ExplorerClient相同）
block, err := adapter.GetBlock(ctx, "12345")
tx, err := adapter.GetTxByTxHash(ctx, "0x...")
accountTxs, err := adapter.GetAccountTxs(ctx, &injective_explorer_rpcpb.GetAccountTxsRequest{
    Address: "inj1...",
    Limit:   20,
})
```

---

## 七、注意事项

### 7.1 性能考虑

1. **批量查询性能下降**
   - GetBlocks: 从1次调用变为N次调用（N=区块数量）
   - GetTxs: 需要扫描多个区块
   - 建议：添加缓存层，限制查询范围

2. **账户交易历史**
   - 必须建立本地索引
   - 全链扫描不可行

3. **流式推送**
   - 优先使用Tendermint订阅
   - 轮询方案延迟较高

### 7.2 数据一致性

1. **格式差异**
   - ExplorerClient和ChainClientV2返回的数据格式不同
   - 需要仔细实现转换逻辑，确保字段映射正确

2. **数据完整性**
   - 某些ExplorerClient返回的字段可能在ChainClientV2中不存在
   - 需要设置默认值或标记为不可用

### 7.3 错误处理

1. **网络错误**
   - ChainClientV2直接连接节点，网络错误更频繁
   - 需要实现重试机制

2. **数据缺失**
   - 某些历史数据可能无法获取
   - 需要优雅降级

---

## 八、总结

### 8.1 可行性评估

| 功能            | 可行性   | 工作量 | 性能影响       |
| --------------- | -------- | ------ | -------------- |
| GetBlock        | ✅ 可行   | 1-2天  | 无             |
| GetTxByTxHash   | ✅ 可行   | 1-2天  | 无             |
| GetAccountTxs   | ⚠️ 需索引 | 2-3周  | 中等           |
| GetBlocks       | ✅ 可行   | 3-5天  | 中等（需并发） |
| GetTxs          | ⚠️ 需扫描 | 1周    | 较大           |
| StreamBlocks    | ✅ 可行   | 1-2周  | 小（用订阅）   |
| FetchValidators | ✅ 可行   | 1-2天  | 无             |
| FetchValidator  | ✅ 可行   | 1-2天  | 无             |

### 8.2 推荐实施策略

1. **分阶段实施**
   - 先实现简单接口（GetBlock, GetTxByTxHash等）
   - 再实现批量查询（GetBlocks, GetTxs）
   - 最后实现复杂功能（GetAccountTxs, StreamBlocks）

2. **混合方案**
   - 保留ExplorerClient作为主要数据源
   - ChainClientV2作为降级方案
   - 逐步迁移

3. **性能优化**
   - 添加多级缓存
   - 实现本地索引
   - 优化并发控制

---

**文档结束**
