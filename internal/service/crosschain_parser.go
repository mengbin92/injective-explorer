package service

import (
	"context"
	"fmt"
	"strconv"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	tx "github.com/cosmos/cosmos-sdk/types/tx"
	ibctypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	peggytypes "github.com/InjectiveLabs/sdk-go/chain/peggy/types"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/pkg/errors"
	"gorm.io/gorm/clause"
)

// CrossChainData contains parsed cross-chain transaction data.
type CrossChainData struct {
	IBCTransfers     []*models.IBCTransferTx
	PeggyDeposits    []*models.PeggyDepositTx
	PeggyWithdrawals []*models.PeggyWithdrawalTx
}

// IsEmpty returns true if no cross-chain data was parsed.
func (d *CrossChainData) IsEmpty() bool {
	return len(d.IBCTransfers) == 0 &&
		len(d.PeggyDeposits) == 0 &&
		len(d.PeggyWithdrawals) == 0
}

// ParseCrossChainTransaction detects and parses cross-chain transactions
// following the peggy.md approach: TxBytes → TxDecoder → sdk.Tx → GetMsgs
//
// Returns nil if no cross-chain messages are found (not an error).
// Returns error only for critical parsing failures.
//
// This function is temporarily exported for testing purposes.
func ParseCrossChainTransaction(
	ctx context.Context,
	txResp *tx.GetTxResponse,
	blockHeight, blockTimestamp uint64,
) (*CrossChainData, error) {
	if txResp == nil || txResp.Tx == nil || txResp.Tx.Body == nil {
		return nil, nil
	}

	// Step 1: Get the Any messages from transaction body
	anyMsgs := txResp.Tx.Body.Messages
	if len(anyMsgs) == 0 {
		return nil, nil
	}

	// Step 2: Create interface registry for unpacking
	interfaceRegistry := types.NewInterfaceRegistry()
	// Register cross-chain message types
	ibctypes.RegisterInterfaces(interfaceRegistry)
	peggytypes.RegisterInterfaces(interfaceRegistry)
	cdc := codec.NewProtoCodec(interfaceRegistry)

	// Step 3: Extract events for enrichment
	var events []*abci.Event
	if txResp.TxResponse != nil && len(txResp.TxResponse.Events) > 0 {
		events = make([]*abci.Event, len(txResp.TxResponse.Events))
		for i := range txResp.TxResponse.Events {
			events[i] = &txResp.TxResponse.Events[i]
		}
	}

	txHash := txResp.TxResponse.TxHash
	data := &CrossChainData{}

	// Step 4: Iterate through Any messages and unpack cross-chain types
	for _, anyMsg := range anyMsgs {
		if anyMsg == nil {
			continue
		}

		// Check message type by TypeUrl first (faster than unpacking)
		switch anyMsg.TypeUrl {
		case "/ibc.applications.transfer.v1.MsgTransfer":
			var msg ibctypes.MsgTransfer
			if err := cdc.Unmarshal(anyMsg.Value, &msg); err != nil {
				global.Logger.Warnf("failed to unmarshal MsgTransfer in tx %s: %v", txHash, err)
				continue
			}
			ibc, err := parseMsgTransfer(&msg, events, txHash, blockHeight, blockTimestamp)
			if err != nil {
				global.Logger.Warnf("failed to parse MsgTransfer in tx %s: %v", txHash, err)
				continue
			}
			if ibc != nil {
				data.IBCTransfers = append(data.IBCTransfers, ibc)
			}

		case "/injective.peggy.v1.MsgDepositClaim":
			var msg peggytypes.MsgDepositClaim
			if err := cdc.Unmarshal(anyMsg.Value, &msg); err != nil {
				global.Logger.Warnf("failed to unmarshal MsgDepositClaim in tx %s: %v", txHash, err)
				continue
			}
			deposit, err := parseMsgDepositClaim(&msg, events, txHash, blockHeight, blockTimestamp)
			if err != nil {
				global.Logger.Warnf("failed to parse MsgDepositClaim in tx %s: %v", txHash, err)
				continue
			}
			if deposit != nil {
				data.PeggyDeposits = append(data.PeggyDeposits, deposit)
			}

		case "/injective.peggy.v1.MsgSendToEth":
			var msg peggytypes.MsgSendToEth
			if err := cdc.Unmarshal(anyMsg.Value, &msg); err != nil {
				global.Logger.Warnf("failed to unmarshal MsgSendToEth in tx %s: %v", txHash, err)
				continue
			}
			withdrawal, err := parseMsgSendToEth(&msg, events, txHash, blockHeight, blockTimestamp)
			if err != nil {
				global.Logger.Warnf("failed to parse MsgSendToEth in tx %s: %v", txHash, err)
				continue
			}
			if withdrawal != nil {
				data.PeggyWithdrawals = append(data.PeggyWithdrawals, withdrawal)
			}

		case "/injective.peggy.v1.MsgWithdrawClaim":
			var msg peggytypes.MsgWithdrawClaim
			if err := cdc.Unmarshal(anyMsg.Value, &msg); err != nil {
				global.Logger.Warnf("failed to unmarshal MsgWithdrawClaim in tx %s: %v", txHash, err)
				continue
			}
			withdrawal, err := parseMsgWithdrawClaim(&msg, events, txHash, blockHeight, blockTimestamp)
			if err != nil {
				global.Logger.Warnf("failed to parse MsgWithdrawClaim in tx %s: %v", txHash, err)
				continue
			}
			if withdrawal != nil {
				data.PeggyWithdrawals = append(data.PeggyWithdrawals, withdrawal)
			}
		}
	}

	if data.IsEmpty() {
		return nil, nil
	}

	return data, nil
}

// parseMsgDepositClaim parses a Peggy MsgDepositClaim (Ethereum → Injective).
func parseMsgDepositClaim(
	msg *peggytypes.MsgDepositClaim,
	events []*abci.Event,
	txHash string,
	blockHeight, blockTimestamp uint64,
) (*models.PeggyDepositTx, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil MsgDepositClaim")
	}

	deposit := &models.PeggyDepositTx{
		Sender:         msg.EthereumSender,
		Receiver:       msg.CosmosReceiver,
		Amount:         msg.Amount.String(),
		Denom:          fmt.Sprintf("peggy%s", msg.TokenContract),
		EventNonce:     msg.EventNonce,
		EventHeight:    msg.BlockHeight,
		State:          "completed", // MsgDepositClaim means already completed
		ClaimType:      1,            // DepositClaim = 1
		TxHash:         txHash,
		BlockHeight:    blockHeight,
		BlockTimestamp: blockTimestamp,
	}

	// Extract orchestrator from message
	if msg.Orchestrator != "" {
		deposit.OrchestratorAddress = msg.Orchestrator
	}

	// Enrich with event data if available
	enrichPeggyDepositFromEvents(deposit, events)

	return deposit, nil
}

// enrichPeggyDepositFromEvents extracts additional fields from events.
func enrichPeggyDepositFromEvents(deposit *models.PeggyDepositTx, events []*abci.Event) {
	// Future: extract additional fields from events if needed
	// Currently all necessary fields are in the message itself
}

// parseMsgSendToEth parses a Peggy MsgSendToEth (Injective → Ethereum intent).
func parseMsgSendToEth(
	msg *peggytypes.MsgSendToEth,
	events []*abci.Event,
	txHash string,
	blockHeight, blockTimestamp uint64,
) (*models.PeggyWithdrawalTx, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil MsgSendToEth")
	}

	withdrawal := &models.PeggyWithdrawalTx{
		Sender:         msg.Sender,
		Receiver:       msg.EthDest,
		Amount:         msg.Amount.Amount.String(),
		Denom:          msg.Amount.Denom,
		BridgeFee:      msg.BridgeFee.Amount.String(),
		State:          "pending", // Intent, not yet completed
		ClaimType:      2,          // WithdrawalClaim = 2
		TxHash:         txHash,
		BlockHeight:    blockHeight,
		BlockTimestamp: blockTimestamp,
	}

	// Extract outgoing_tx_id and batch_timeout from events
	enrichPeggyWithdrawalFromEvents(withdrawal, events)

	return withdrawal, nil
}

// enrichPeggyWithdrawalFromEvents extracts outgoing_tx_id and batch_timeout from events.
func enrichPeggyWithdrawalFromEvents(withdrawal *models.PeggyWithdrawalTx, events []*abci.Event) {
	for _, event := range events {
		if event.Type == "send_to_eth" {
			for _, attr := range event.Attributes {
				switch attr.Key {
				case "outgoing_tx_id":
					if id, err := parseUint64(attr.Value); err == nil {
						withdrawal.OutgoingTxId = id
					}
				case "batch_timeout":
					if timeout, err := parseUint64(attr.Value); err == nil {
						withdrawal.BatchTimeout = timeout
					}
				}
			}
		}
	}
}

// parseMsgWithdrawClaim parses a Peggy MsgWithdrawClaim (withdrawal confirmation).
// This updates an existing PeggyWithdrawalTx record.
func parseMsgWithdrawClaim(
	msg *peggytypes.MsgWithdrawClaim,
	events []*abci.Event,
	txHash string,
	blockHeight, blockTimestamp uint64,
) (*models.PeggyWithdrawalTx, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil MsgWithdrawClaim")
	}

	// MsgWithdrawClaim confirms a batch withdrawal
	// We create a record that will be upserted based on batch_nonce relation
	withdrawal := &models.PeggyWithdrawalTx{
		BatchNonce:     msg.BatchNonce,
		EventNonce:     msg.EventNonce,
		EventHeight:    msg.BlockHeight,
		State:          "completed",
		ClaimType:      2, // WithdrawalClaim = 2
		TxHash:         txHash,
		BlockHeight:    blockHeight,
		BlockTimestamp: blockTimestamp,
	}

	// Extract orchestrator if available
	if msg.Orchestrator != "" {
		withdrawal.OrchestratorAddress = msg.Orchestrator
	}

	// Note: We don't have sender/receiver/amount in MsgWithdrawClaim
	// These will be preserved from the original MsgSendToEth record via UPSERT

	return withdrawal, nil
}

// parseMsgTransfer parses an IBC MsgTransfer.
func parseMsgTransfer(
	msg *ibctypes.MsgTransfer,
	events []*abci.Event,
	txHash string,
	blockHeight, blockTimestamp uint64,
) (*models.IBCTransferTx, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil MsgTransfer")
	}

	transfer := &models.IBCTransferTx{
		Sender:           msg.Sender,
		Receiver:         msg.Receiver,
		SourcePort:       msg.SourcePort,
		SourceChannel:    msg.SourceChannel,
		Amount:           msg.Token.Amount.String(),
		Denom:            msg.Token.Denom,
		TimeoutTimestamp: msg.TimeoutTimestamp,
		State:            "completed",
		TxHash:           txHash,
		BlockHeight:      blockHeight,
		BlockTimestamp:   blockTimestamp,
	}

	// Parse timeout height if present (checking if not zero value)
	if msg.TimeoutHeight.RevisionNumber != 0 || msg.TimeoutHeight.RevisionHeight != 0 {
		transfer.TimeoutHeight = fmt.Sprintf("%d-%d", msg.TimeoutHeight.RevisionNumber, msg.TimeoutHeight.RevisionHeight)
	}

	// Enrich with packet_sequence and destination from events
	enrichIBCTransferFromEvents(transfer, events)

	return transfer, nil
}

// enrichIBCTransferFromEvents extracts packet_sequence and destination info from events.
func enrichIBCTransferFromEvents(transfer *models.IBCTransferTx, events []*abci.Event) {
	for _, event := range events {
		if event.Type == "send_packet" {
			for _, attr := range event.Attributes {
				switch attr.Key {
				case "packet_sequence":
					if seq, err := parseUint64(attr.Value); err == nil {
						transfer.PacketSequence = seq
					}
				case "packet_dst_port":
					transfer.DestinationPort = attr.Value
				case "packet_dst_channel":
					transfer.DestinationChannel = attr.Value
				case "packet_data":
					transfer.DataHex = []byte(attr.Value)
				}
			}
		}
	}
}

// parseUint64 safely parses a string to uint64.
func parseUint64(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	return strconv.ParseUint(s, 10, 64)
}

// SaveCrossChainTransactionTest saves cross-chain transaction data to database with UPSERT.
// This is exported for testing purposes.
func SaveCrossChainTransactionTest(ctx context.Context, data *CrossChainData) error {
	return saveCrossChainTransaction(ctx, data)
}

// saveCrossChainTransaction saves cross-chain transaction data to database with UPSERT.
func saveCrossChainTransaction(ctx context.Context, data *CrossChainData) error {
	if data == nil || data.IsEmpty() {
		return nil
	}

	// Save IBC transfers (UPSERT on tx_hash + packet_sequence)
	for _, ibc := range data.IBCTransfers {
		if err := db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tx_hash"}, {Name: "packet_sequence"}},
			DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
		}).Create(ibc).Error; err != nil {
			return errors.Wrap(err, "failed to save IBC transfer")
		}
	}

	// Save Peggy deposits (UPSERT on event_nonce)
	for _, deposit := range data.PeggyDeposits {
		if err := db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_nonce"}},
			DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
		}).Create(deposit).Error; err != nil {
			return errors.Wrap(err, "failed to save Peggy deposit")
		}
	}

	// Save Peggy withdrawals (UPSERT on outgoing_tx_id)
	// Note: MsgWithdrawClaim updates existing MsgSendToEth records
	for _, withdrawal := range data.PeggyWithdrawals {
		if err := db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "outgoing_tx_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"state", "batch_nonce", "event_nonce", "event_height",
				"orchestrator_address", "eth_tx_hash", "updated_at",
			}),
		}).Create(withdrawal).Error; err != nil {
			return errors.Wrap(err, "failed to save Peggy withdrawal")
		}
	}

	return nil
}
