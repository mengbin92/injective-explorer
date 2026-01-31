# 跨链交易数据同步设计方案

## 1. 概述

本文档说明如何从区块链同步跨链交易数据并保存到 `database_schema.sql` 中定义的跨链交易表。

## 2. 当前数据流程

### 2.1 交易同步流程

当前项目的交易同步流程如下：

```
区块同步 (SyncService)
    ↓
提取交易哈希 → 推送到 Redis 队列
    ↓
后台 Worker 处理
    ↓
调用 chainClient.GetTx() 获取交易详情
    ↓
解析交易 (txparser.go)
    ↓
保存到 transactions 表
```

### 2.2 当前解析能力

`txparser.go` 中已经实现了以下 message extractor：

1. **IBC 转账**：
   - Message Type: `/ibc.applications.transfer.v1.MsgTransfer`
   - Extractor: `ibcMsgTransferExtractor`
   - 当前提取：`sender`, `receiver`, `amount`, `denom`
   - **缺失**：`source_port`, `source_channel`, `destination_port`, `destination_channel`, `packet_sequence`, `timeout_height`, `timeout_timestamp` 等详细信息

2. **Peggy 发送到以太坊**：
   - Message Type: `/injective.peggy.v1.MsgSendToEth`
   - Extractor: `injectivePeggyMsgSendToEthExtractor`
   - 当前提取：`sender`, `eth_dest`, `amount`, `denom`, `bridge_fee`
   - **缺失**：`outgoing_tx_id`, `batch_timeout`, `batch_nonce`, `event_nonce`, `event_height` 等详细信息

3. **Peggy 存款**：
   - Message Type: `/injective.peggy.v1.MsgDepositClaim`
   - **未实现**：需要新增 extractor

### 2.3 问题分析

**当前状态**：
- `txparser.go` 的 extractor 只提取基本的转账信息（from/to/value），用于在 `transactions` 表中显示
- **没有**专门解析跨链交易的详细信息并保存到独立的跨链交易表

**需要实现**：
- 在交易处理流程中，检测到跨链交易时，额外解析交易的 events 和 messages
- 提取跨链交易的完整信息（channel、port、packet_sequence、event_nonce 等）
- 保存到 `ibc_transfer_txs`、`peggy_deposit_txs`、`peggy_withdrawal_txs` 表

## 3. 数据来源

跨链交易的详细信息主要来自两个地方：

### 3.1 Message 字段

Message 中包含跨链交易的基本信息：

**IBC MsgTransfer**：
```json
{
  "@type": "/ibc.applications.transfer.v1.MsgTransfer",
  "sender": "inj1...",
  "receiver": "cosmos1...",
  "token": {
    "denom": "inj",
    "amount": "1000000"
  },
  "source_port": "transfer",
  "source_channel": "channel-0",
  "timeout_height": {...},
  "timeout_timestamp": "1699999999000000000"
}
```

**Peggy MsgSendToEth**：
```json
{
  "@type": "/injective.peggy.v1.MsgSendToEth",
  "sender": "inj1...",
  "eth_dest": "0xabc...",
  "amount": {
    "denom": "peggy0x...",
    "amount": "1000000000000000000"
  },
  "bridge_fee": {
    "denom": "peggy0x...",
    "amount": "1000000000000000"
  }
}
```

**Peggy MsgDepositClaim**：
```json
{
  "@type": "/injective.peggy.v1.MsgDepositClaim",
  "event_nonce": "42",
  "eth_sender": "0xabc...",
  "injective_receiver": "inj1...",
  "amount": {
    "denom": "peggy0x...",
    "amount": "1000000000000000000"
  }
}
```

### 3.2 Events 字段

Events 中包含更详细的跨链信息，特别是：

**IBC Events**：
- `send_packet` - 包含 `packet_sequence`, `packet_data`, `packet_timeout_height`, `packet_timeout_timestamp`
- `ibc_transfer` - 包含 `sender`, `receiver`, `amount`
- `transfer` - 包含转账信息
- `recv_packet` - 包含 `packet_dst_port`, `packet_dst_channel`

**Peggy Events**：
- `deposit_claim` - 包含 `event_nonce`, `event_height`, `orchestrator_address`
- `withdrawal_claim` - 包含 `event_nonce`, `event_height`, `batch_nonce`
- `send_to_eth` - 包含 `outgoing_tx_id`, `batch_timeout`

## 4. 实现方案

### 4.1 架构设计（优化后）

**优化思路**：复用现有的 `txparser.go` 架构，扩展现有 extractor 而非创建独立解析器。

```
processTransaction (sync.go)
    ↓
保存到 transactions 表 (现有流程)
    ↓
txparser.ExtractCrossChainInfo (新增调用)
    ↓
保存到跨链交易表 (新增)
```

### 4.2 实现位置

**核心优化**：在 `internal/models/txparser.go` 中扩展现有 extractor，保持代码内聚性。

```go
// internal/models/txparser.go

// 扩展现有 ExtractResult 结构
type ExtractResult struct {
    From            string   `json:"from,omitempty"`
    To              string   `json:"to,omitempty"`
    Values          []Coin   `json:"values,omitempty"`
    MsgTypes        []string `json:"msg_types,omitempty"`
    ShouldShowValue bool     `json:"should_show_value,omitempty"`

    // 新增：跨链交易详细信息
    IBCTransfer     *IBCTransferDetail     `json:"ibc_transfer,omitempty"`
    PeggyDeposit    *PeggyDepositDetail    `json:"peggy_deposit,omitempty"`
    PeggyWithdrawal *PeggyWithdrawalDetail `json:"peggy_withdrawal,omitempty"`
}

// IBCTransferDetail 包含 IBC 转账详细信息
type IBCTransferDetail struct {
    SourcePort        string
    SourceChannel     string
    DestinationPort   string
    DestinationChannel string
    PacketSequence    uint64
    TimeoutHeight     string
    TimeoutTimestamp  uint64
    DataHex           []byte
}

// PeggyDepositDetail 包含 Peggy 存款详细信息
type PeggyDepositDetail struct {
    EventNonce         uint64
    EventHeight        uint64
    OrchestratorAddress string
    ClaimType          int32
}

// PeggyWithdrawalDetail 包含 Peggy 取款详细信息
type PeggyWithdrawalDetail struct {
    BridgeFee           string
    OutgoingTxId        uint64
    BatchTimeout        uint64
    BatchNonce          uint64
    OrchestratorAddress string
    EventNonce          uint64
    EventHeight         uint64
    ClaimType           int32
}
```

### 4.3 扩展现有 Extractor

**优化思路**：修改现有 extractor，在提取基础信息的同时提取跨链详细信息。

```go
// ibcMsgTransferExtractor 扩展示例
func (e *ibcMsgTransferExtractor) Extract(msg json.RawMessage, res *ExtractResult, txResp interface{}) bool {
    var data struct {
        Sender           string          `json:"sender"`
        Receiver         string          `json:"receiver"`
        Token            Token           `json:"token"`
        SourcePort       string          `json:"source_port"`
        SourceChannel    string          `json:"source_channel"`
        TimeoutHeight    json.RawMessage `json:"timeout_height"`
        TimeoutTimestamp string          `json:"timeout_timestamp"`
    }

    if err := json.Unmarshal(msg, &data); err != nil {
        return false
    }

    res.From = data.Sender
    res.To = data.Receiver
    res.Values = []Coin{{Denom: data.Token.Denom, Amount: data.Token.Amount}}

    // 提取跨链详细信息
    res.IBCTransfer = &IBCTransferDetail{
        SourcePort:    data.SourcePort,
        SourceChannel: data.SourceChannel,
    }

    // 解析 timeout_height
    if len(data.TimeoutHeight) > 0 {
        res.IBCTransfer.TimeoutHeight = string(data.TimeoutHeight)
    }

    // 解析 timeout_timestamp
    if data.TimeoutTimestamp != "" {
        if ts, err := strconv.ParseUint(data.TimeoutTimestamp, 10, 64); err == nil {
            res.IBCTransfer.TimeoutTimestamp = ts
        }
    }

    // 从 txResp 的 events 中提取更多信息
    if txResp != nil {
        if resp, ok := txResp.(*tx.GetTxResponse); ok {
            extractIBCInfoFromEvents(resp.TxResponse.Events, res.IBCTransfer)
        }
    }

    return true
}

// peggyMsgSendToEthExtractor 扩展示例
func (e *peggyMsgSendToEthExtractor) Extract(msg json.RawMessage, res *ExtractResult, txResp interface{}) bool {
    var data struct {
        Sender    string `json:"sender"`
        EthDest   string `json:"eth_dest"`
        Amount    Token  `json:"amount"`
        BridgeFee Token  `json:"bridge_fee"`
    }

    if err := json.Unmarshal(msg, &data); err != nil {
        return false
    }

    res.From = data.Sender
    res.To = data.EthDest
    res.Values = []Coin{{Denom: data.Amount.Denom, Amount: data.Amount.Amount}}

    // 提取跨链详细信息
    res.PeggyWithdrawal = &PeggyWithdrawalDetail{
        BridgeFee: data.BridgeFee.Amount,
    }

    // 从 txResp 的 events 中提取更多信息
    if txResp != nil {
        if resp, ok := txResp.(*tx.GetTxResponse); ok {
            extractPeggyWithdrawalFromEvents(resp.TxResponse.Events, res.PeggyWithdrawal)
        }
    }

    return true
}

// 新增：peggyMsgDepositClaimExtractor
func (e *peggyMsgDepositClaimExtractor) Extract(msg json.RawMessage, res *ExtractResult, txResp interface{}) bool {
    var data struct {
        EventNonce        string `json:"event_nonce"`
        EthSender         string `json:"eth_sender"`
        InjectiveReceiver string `json:"injective_receiver"`
        Amount            Token  `json:"amount"`
    }

    if err := json.Unmarshal(msg, &data); err != nil {
        return false
    }

    res.From = data.EthSender
    res.To = data.InjectiveReceiver
    res.Values = []Coin{{Denom: data.Amount.Denom, Amount: data.Amount.Amount}}

    // 提取跨链详细信息
    res.PeggyDeposit = &PeggyDepositDetail{
        ClaimType: 1, // DepositClaim
    }

    if data.EventNonce != "" {
        if nonce, err := strconv.ParseUint(data.EventNonce, 10, 64); err == nil {
            res.PeggyDeposit.EventNonce = nonce
        }
    }

    // 从 txResp 的 events 中提取更多信息
    if txResp != nil {
        if resp, ok := txResp.(*tx.GetTxResponse); ok {
            extractPeggyDepositFromEvents(resp.TxResponse.Events, res.PeggyDeposit)
        }
    }

    return true
}
```

### 4.4 Events 解析辅助函数

```go
// extractIBCInfoFromEvents extracts IBC-specific information from events.
func extractIBCInfoFromEvents(events []abci.Event, detail *IBCTransferDetail) {
    for _, event := range events {
        switch event.Type {
        case "send_packet":
            for _, attr := range event.Attributes {
                key := string(attr.Key)
                value := string(attr.Value)

                switch key {
                case "packet_sequence":
                    if seq, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.PacketSequence = seq
                    }
                case "packet_timeout_height":
                    detail.TimeoutHeight = value
                case "packet_timeout_timestamp":
                    if ts, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.TimeoutTimestamp = ts
                    }
                case "packet_data":
                    detail.DataHex = []byte(value)
                    // 解析 packet_data 获取 destination_port 和 destination_channel
                    extractDestinationFromPacketData(value, detail)
                }
            }

        case "recv_packet":
            for _, attr := range event.Attributes {
                key := string(attr.Key)
                value := string(attr.Value)

                if key == "packet_dst_port" {
                    detail.DestinationPort = value
                } else if key == "packet_dst_channel" {
                    detail.DestinationChannel = value
                }
            }
        }
    }
}

// extractPeggyDepositFromEvents extracts Peggy deposit information from events.
func extractPeggyDepositFromEvents(events []abci.Event, detail *PeggyDepositDetail) {
    for _, event := range events {
        if event.Type == "deposit_claim" {
            for _, attr := range event.Attributes {
                key := string(attr.Key)
                value := string(attr.Value)

                switch key {
                case "event_nonce":
                    if nonce, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.EventNonce = nonce
                    }
                case "event_height":
                    if height, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.EventHeight = height
                    }
                case "orchestrator_address":
                    detail.OrchestratorAddress = value
                }
            }
        }
    }
}

// extractPeggyWithdrawalFromEvents extracts Peggy withdrawal information from events.
func extractPeggyWithdrawalFromEvents(events []abci.Event, detail *PeggyWithdrawalDetail) {
    for _, event := range events {
        switch event.Type {
        case "send_to_eth":
            for _, attr := range event.Attributes {
                key := string(attr.Key)
                value := string(attr.Value)

                switch key {
                case "outgoing_tx_id":
                    if id, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.OutgoingTxId = id
                    }
                case "batch_timeout":
                    if ts, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.BatchTimeout = ts
                    }
                }
            }

        case "withdrawal_claim":
            for _, attr := range event.Attributes {
                key := string(attr.Key)
                value := string(attr.Value)

                switch key {
                case "event_nonce":
                    if nonce, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.EventNonce = nonce
                    }
                case "event_height":
                    if height, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.EventHeight = height
                    }
                case "batch_nonce":
                    if nonce, err := strconv.ParseUint(value, 10, 64); err == nil {
                        detail.BatchNonce = nonce
                    }
                case "orchestrator_address":
                    detail.OrchestratorAddress = value
                }
            }
        }
    }
}

// extractDestinationFromPacketData parses packet_data JSON to extract destination info.
func extractDestinationFromPacketData(packetData string, detail *IBCTransferDetail) {
    var data struct {
        Receiver string `json:"receiver"`
        // 其他字段...
    }
    if err := json.Unmarshal([]byte(packetData), &data); err != nil {
        return
    }
    // 如有需要提取更多信息
}
```

### 4.5 在 SyncService 中集成

在 `internal/service/sync.go` 的 `processTransaction` 方法中，保存交易到数据库后，添加跨链交易解析逻辑：

```go
// processTransaction 扩展示例
func (s *SyncService) processTransaction(ctx context.Context, txHash string, blockHeight, blockTimestamp uint64) error {
    // ... 现有逻辑：获取交易详情 ...

    // 解析交易（扩展后的 ExtractTransaction）
    extractResult := models.ExtractTransactionWithCrossChain(txResp, blockHeight, blockTimestamp)

    // 保存到 transactions 表（现有逻辑）
    txModel := extractResult.ToTransactionModel()
    if err := s.saveTransaction(ctx, txModel); err != nil {
        return err
    }

    // 新增：保存跨链交易信息
    if err := s.saveCrossChainTransaction(ctx, extractResult, txModel); err != nil {
        global.Logger.Warnf("failed to save cross-chain transaction %s: %v", txHash, err)
        // 不阻塞主流程，只记录警告
    }

    return nil
}

// saveCrossChainTransaction saves cross-chain transaction details to dedicated tables.
func (s *SyncService) saveCrossChainTransaction(
    ctx context.Context,
    result *models.ExtractResult,
    txModel *models.Transaction,
) error {
    // 1. 保存 IBC 转账
    if result.IBCTransfer != nil {
        ibcTx := &models.IBCTransferTx{
            Sender:             result.From,
            Receiver:           result.To,
            SourcePort:         result.IBCTransfer.SourcePort,
            SourceChannel:      result.IBCTransfer.SourceChannel,
            DestinationPort:    result.IBCTransfer.DestinationPort,
            DestinationChannel: result.IBCTransfer.DestinationChannel,
            Amount:             result.Values[0].Amount,
            Denom:              result.Values[0].Denom,
            TimeoutHeight:      result.IBCTransfer.TimeoutHeight,
            TimeoutTimestamp:   result.IBCTransfer.TimeoutTimestamp,
            PacketSequence:     result.IBCTransfer.PacketSequence,
            DataHex:            result.IBCTransfer.DataHex,
            State:              "completed",
            TxHash:             txModel.Hash,
            BlockHeight:        txModel.BlockNumber,
            BlockTimestamp:     txModel.BlockUnixTimestamp,
        }
        if err := s.saveIBCTransfer(ctx, ibcTx); err != nil {
            return errors.Wrap(err, "failed to save IBC transfer")
        }
    }

    // 2. 保存 Peggy 存款
    if result.PeggyDeposit != nil {
        deposit := &models.PeggyDepositTx{
            Sender:              result.From,
            Receiver:            result.To,
            Amount:              result.Values[0].Amount,
            Denom:               result.Values[0].Denom,
            EventNonce:          result.PeggyDeposit.EventNonce,
            EventHeight:         result.PeggyDeposit.EventHeight,
            OrchestratorAddress: result.PeggyDeposit.OrchestratorAddress,
            State:               "completed",
            ClaimType:           result.PeggyDeposit.ClaimType,
            TxHash:              txModel.Hash,
            BlockHeight:         txModel.BlockNumber,
            BlockTimestamp:      txModel.BlockUnixTimestamp,
        }
        if err := s.savePeggyDeposit(ctx, deposit); err != nil {
            return errors.Wrap(err, "failed to save Peggy deposit")
        }
    }

    // 3. 保存 Peggy 取款
    if result.PeggyWithdrawal != nil {
        withdrawal := &models.PeggyWithdrawalTx{
            Sender:              result.From,
            Receiver:            result.To,
            Amount:              result.Values[0].Amount,
            Denom:               result.Values[0].Denom,
            BridgeFee:           result.PeggyWithdrawal.BridgeFee,
            OutgoingTxId:        result.PeggyWithdrawal.OutgoingTxId,
            BatchTimeout:        result.PeggyWithdrawal.BatchTimeout,
            BatchNonce:          result.PeggyWithdrawal.BatchNonce,
            OrchestratorAddress: result.PeggyWithdrawal.OrchestratorAddress,
            EventNonce:          result.PeggyWithdrawal.EventNonce,
            EventHeight:         result.PeggyWithdrawal.EventHeight,
            State:               determineWithdrawalState(result.PeggyWithdrawal),
            ClaimType:           result.PeggyWithdrawal.ClaimType,
            TxHash:              txModel.Hash,
            BlockHeight:         txModel.BlockNumber,
            BlockTimestamp:      txModel.BlockUnixTimestamp,
        }
        if err := s.savePeggyWithdrawal(ctx, withdrawal); err != nil {
            return errors.Wrap(err, "failed to save Peggy withdrawal")
        }
    }

    return nil
}

// determineWithdrawalState determines the state of a Peggy withdrawal.
func determineWithdrawalState(detail *models.PeggyWithdrawalDetail) string {
    if detail.BatchNonce > 0 {
        return "completed"
    }
    if detail.BatchTimeout > 0 {
        return "batched"
    }
    return "pending"
}
```

### 4.6 批量插入优化

**优化点**：高并发同步时逐条插入跨链表会有性能问题，使用批量 upsert：

```go
// saveIBCTransfer saves IBC transfer transaction to database.
func (s *SyncService) saveIBCTransfer(ctx context.Context, ibcTx *models.IBCTransferTx) error {
    return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
        Columns: []clause.Column{{Name: "tx_hash"}, {Name: "packet_sequence"}},
        DoUpdates: clause.AssignmentColumns([]string{
            "sender", "receiver", "source_port", "source_channel",
            "destination_port", "destination_channel", "amount", "denom",
            "timeout_height", "timeout_timestamp", "state", "updated_at",
        }),
    }).Create(ibcTx).Error
}

// savePeggyDeposit saves Peggy deposit transaction to database.
func (s *SyncService) savePeggyDeposit(ctx context.Context, deposit *models.PeggyDepositTx) error {
    return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
        Columns: []clause.Column{{Name: "event_nonce"}},
        DoUpdates: clause.AssignmentColumns([]string{
            "sender", "receiver", "amount", "denom",
            "event_height", "orchestrator_address", "state", "updated_at",
        }),
    }).Create(deposit).Error
}

// savePeggyWithdrawal saves Peggy withdrawal transaction to database.
func (s *SyncService) savePeggyWithdrawal(ctx context.Context, withdrawal *models.PeggyWithdrawalTx) error {
    return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
        Columns: []clause.Column{{Name: "outgoing_tx_id"}},
        DoUpdates: clause.AssignmentColumns([]string{
            "sender", "receiver", "amount", "denom", "bridge_fee",
            "batch_timeout", "batch_nonce", "orchestrator_address",
            "event_nonce", "event_height", "state", "updated_at",
        }),
    }).Create(withdrawal).Error
}

// BatchSaveCrossChainTxs 批量保存跨链交易（用于优化性能）
func (s *SyncService) BatchSaveCrossChainTxs(ctx context.Context, ibcTxs []*models.IBCTransferTx, deposits []*models.PeggyDepositTx, withdrawals []*models.PeggyWithdrawalTx) error {
    return db.Get().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if len(ibcTxs) > 0 {
            if err := tx.Clauses(clause.OnConflict{
                Columns:   []clause.Column{{Name: "tx_hash"}, {Name: "packet_sequence"}},
                DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
            }).CreateInBatches(ibcTxs, 100).Error; err != nil {
                return err
            }
        }

        if len(deposits) > 0 {
            if err := tx.Clauses(clause.OnConflict{
                Columns:   []clause.Column{{Name: "event_nonce"}},
                DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
            }).CreateInBatches(deposits, 100).Error; err != nil {
                return err
            }
        }

        if len(withdrawals) > 0 {
            if err := tx.Clauses(clause.OnConflict{
                Columns:   []clause.Column{{Name: "outgoing_tx_id"}},
                DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
            }).CreateInBatches(withdrawals, 100).Error; err != nil {
                return err
            }
        }

        return nil
    })
}
```

## 5. 数据模型

### 5.1 GORM 模型

需要在 `internal/models/` 中创建模型文件：

**`internal/models/ibc_transfer_tx.go`**：
```go
package models

import (
    "time"
    "gorm.io/gorm"
)

// IBCTransferTx represents an IBC transfer transaction in the database.
type IBCTransferTx struct {
    ID                  uint64         `gorm:"primaryKey;column:id"`
    CreatedAt           time.Time      `gorm:"column:created_at"`
    UpdatedAt           time.Time      `gorm:"column:updated_at"`
    DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at;index"`

    Sender              string         `gorm:"column:sender;size:128;not null;index"`
    Receiver            string         `gorm:"column:receiver;size:128;not null;index"`
    SourcePort          string         `gorm:"column:source_port;size:100"`
    SourceChannel       string         `gorm:"column:source_channel;size:100;index"`
    DestinationPort     string         `gorm:"column:destination_port;size:100"`
    DestinationChannel  string         `gorm:"column:destination_channel;size:100;index"`
    Amount              string         `gorm:"column:amount;size:100"`
    Denom               string         `gorm:"column:denom;size:100"`
    TimeoutHeight       string         `gorm:"column:timeout_height;size:100"`
    TimeoutTimestamp    uint64         `gorm:"column:timeout_timestamp"`
    PacketSequence      uint64         `gorm:"column:packet_sequence"`
    DataHex             []byte         `gorm:"column:data_hex;type:blob"`
    State               string         `gorm:"column:state;size:50;default:'completed'"`

    TxHash              string         `gorm:"column:tx_hash;size:128;not null;index"`
    BlockHeight         uint64         `gorm:"column:block_height;index"`
    BlockTimestamp      uint64         `gorm:"column:block_timestamp;index"`
}

func (IBCTransferTx) TableName() string {
    return "ibc_transfer_txs"
}

// ToProto converts to proto DTO
func (t *IBCTransferTx) ToProto() *pb.IBCTransferTxDTO {
    return &pb.IBCTransferTxDTO{
        Sender:             t.Sender,
        Receiver:           t.Receiver,
        SourcePort:         t.SourcePort,
        SourceChannel:      t.SourceChannel,
        DestinationPort:    t.DestinationPort,
        DestinationChannel: t.DestinationChannel,
        Amount:             t.Amount,
        Denom:              t.Denom,
        TimeoutHeight:      t.TimeoutHeight,
        TimeoutTimestamp:   t.TimeoutTimestamp,
        PacketSequence:     t.PacketSequence,
        DataHex:            t.DataHex,
        State:              t.State,
        TxHashes:           []string{t.TxHash},
        CreatedAt:          t.CreatedAt.Format(time.RFC3339),
        UpdatedAt:          t.UpdatedAt.Format(time.RFC3339),
    }
}
```

类似地创建 `peggy_deposit_tx.go` 和 `peggy_withdrawal_tx.go`。

## 6. 实现步骤

### 6.1 第一阶段：扩展 txparser

1. 在 `internal/models/txparser.go` 中扩展 `ExtractResult` 结构
2. 扩展现有 extractor（ibcMsgTransferExtractor, peggyMsgSendToEthExtractor）
3. 新增 peggyMsgDepositClaimExtractor
4. 实现 Events 解析辅助函数
5. 添加单元测试验证解析逻辑

### 6.2 第二阶段：创建数据模型

1. 创建 `internal/models/ibc_transfer_tx.go`
2. 创建 `internal/models/peggy_deposit_tx.go`
3. 创建 `internal/models/peggy_withdrawal_tx.go`
4. 在模型中实现 `ToProto()` 转换方法
5. 运行数据库迁移（如果表不存在）

### 6.3 第三阶段：集成到同步流程

1. 在 `sync.go` 中修改 `processTransaction` 方法
2. 添加 `saveCrossChainTransaction` 方法
3. 实现三个保存方法（`saveIBCTransfer`, `savePeggyDeposit`, `savePeggyWithdrawal`）
4. 实现批量保存优化（`BatchSaveCrossChainTxs`）
5. 更新同步状态表 `cross_chain_sync_state`

### 6.4 第四阶段：测试与验证

1. 单元测试：测试扩展后的 extractor
2. 集成测试：测试完整的数据同步流程
3. 验证数据正确性：对比链上数据和数据库数据
4. 性能测试：测试高并发下的批量插入性能

## 7. 注意事项

### 7.1 Events 格式不确定性

**风险**：Injective 链的 event 格式可能与标准 Cosmos SDK 有差异。

**应对措施**：
- 在实际数据上测试解析逻辑
- 添加防御性编程，容忍字段缺失
- 记录 warn 日志以便排查问题

```go
// 防御性解析示例
func safeParseUint64(value string, defaultVal uint64) uint64 {
    if value == "" {
        return defaultVal
    }
    if result, err := strconv.ParseUint(value, 10, 64); err == nil {
        return result
    }
    return defaultVal
}
```

### 7.2 数据量大

**风险**：跨链交易表可能快速增长。

**应对措施**：
- 已添加索引优化查询性能
- 考虑未来添加分区策略（按时间或区块高度）
- 考虑归档策略（参考项目的 archiving 实现）

### 7.3 状态更新

**风险**：IBC 交易状态可能从 `pending` 变为 `completed` 或 `timeout`。

**应对措施**：
- 使用 UPSERT 更新现有记录
- 定期检查并更新 pending 交易的状态
- 考虑添加状态监控和告警

### 7.4 Peggy 事件关联

**风险**：存款/取款的事件可能分散在多个交易中。

**应对措施**：
- 使用 `event_nonce` 关联相关事件
- 对于取款，先保存 `MsgSendToEth`，后续用 `MsgWithdrawClaim` 更新
- 使用状态机管理交易生命周期

### 7.5 错误处理

**原则**：跨链交易解析失败不应阻塞主交易处理流程。

```go
// 不阻塞主流程
if err := s.saveCrossChainTransaction(ctx, extractResult, txModel); err != nil {
    global.Logger.Warnf("failed to save cross-chain transaction %s: %v", txHash, err)
    // 只记录警告，不返回错误
}
```

## 8. 参考资源

### 8.1 Injective Explorer RPC

参考 `injective_explorer_rpc_interfaces.md` 了解接口规范。

### 8.2 Cosmos SDK Events

- IBC Events: https://github.com/cosmos/ibc-go/blob/main/docs/architecture/adr-001-coin-source-tracing.md
- Peggy Events: Injective 链特定事件

### 8.3 现有代码参考

- `internal/models/txparser.go` - 交易解析器实现
- `internal/service/sync.go` - 交易同步流程
- `internal/service/evm_extractor.go` - EVM 交易提取器（类似实现）

---

**文档版本**: 2.0
**更新日期**: 2026-01-31
**作者**: Injective Explorer Team
