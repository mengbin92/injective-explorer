package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"injective-explorer/internal/converter"
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

// Global codec singleton for cross-chain message parsing.
// This avoids the expensive operation of creating a new codec for each transaction.
var (
	globalCodec     codec.Codec
	codecInitOnce   sync.Once
)

// initGlobalCodec initializes the global codec with interface registry.
// This is called once via sync.Once to ensure thread-safety.
func initGlobalCodec() {
	interfaceRegistry := types.NewInterfaceRegistry()
	// Register cross-chain message types
	ibctypes.RegisterInterfaces(interfaceRegistry)
	peggytypes.RegisterInterfaces(interfaceRegistry)
	globalCodec = codec.NewProtoCodec(interfaceRegistry)
}

// getCodec returns the singleton codec instance, initializing it if necessary.
func getCodec() codec.Codec {
	codecInitOnce.Do(initGlobalCodec)
	return globalCodec
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

	// Step 2: Get the singleton codec for unpacking
	cdc := getCodec()

	// Step 3: Extract events for enrichment
	events := extractEvents(txResp)

	// Step 4: Extract EVM hash if this is an EVM transaction
	// Convert []*abci.Event to []abci.Event for ExtractEVMHashFromABCIEvents
	var eventsSlice []abci.Event
	for _, e := range events {
		if e != nil {
			eventsSlice = append(eventsSlice, *e)
		}
	}
	evmHash := ExtractEVMHashFromABCIEvents(eventsSlice)

	// Step 5: Process messages and collect cross-chain data
	txHash := txResp.TxResponse.TxHash
	data := &CrossChainData{}

	for _, anyMsg := range anyMsgs {
		if anyMsg == nil {
			continue
		}

		// Dispatch to appropriate handler based on message type
		// Pass evmHash so it can be stored in cross-chain tables
		if err := processCrossChainMessage(anyMsg, cdc, events, txHash, evmHash, blockHeight, blockTimestamp, data); err != nil {
			global.Logger.Warnf("Failed to process cross-chain message in tx %s: %v", txHash, err)
		}
	}

	if data.IsEmpty() {
		return nil, nil
	}

	return data, nil
}

// extractEvents extracts events from transaction response for enrichment.
func extractEvents(txResp *tx.GetTxResponse) []*abci.Event {
	if txResp.TxResponse == nil || len(txResp.TxResponse.Events) == 0 {
		return nil
	}

	events := make([]*abci.Event, len(txResp.TxResponse.Events))
	for i := range txResp.TxResponse.Events {
		events[i] = &txResp.TxResponse.Events[i]
	}
	return events
}

// processCrossChainMessage dispatches message processing based on TypeUrl.
func processCrossChainMessage(
	anyMsg *types.Any,
	cdc codec.Codec,
	events []*abci.Event,
	txHash string,
	evmHash string,
	blockHeight, blockTimestamp uint64,
	data *CrossChainData,
) error {
	switch anyMsg.TypeUrl {
	case "/ibc.applications.transfer.v1.MsgTransfer":
		return handleMsgTransfer(anyMsg, cdc, events, txHash, evmHash, blockHeight, blockTimestamp, data)
	case "/injective.peggy.v1.MsgDepositClaim":
		return handleMsgDepositClaim(anyMsg, cdc, events, txHash, evmHash, blockHeight, blockTimestamp, data)
	case "/injective.peggy.v1.MsgSendToEth":
		return handleMsgSendToEth(anyMsg, cdc, events, txHash, evmHash, blockHeight, blockTimestamp, data)
	case "/injective.peggy.v1.MsgWithdrawClaim":
		return handleMsgWithdrawClaim(anyMsg, cdc, events, txHash, evmHash, blockHeight, blockTimestamp, data)
	default:
		// Not a cross-chain message, skip silently
		return nil
	}
}

// handleMsgTransfer processes IBC MsgTransfer messages.
func handleMsgTransfer(
	anyMsg *types.Any,
	cdc codec.Codec,
	events []*abci.Event,
	txHash string,
	evmHash string,
	blockHeight, blockTimestamp uint64,
	data *CrossChainData,
) error {
	var msg ibctypes.MsgTransfer
	if err := cdc.Unmarshal(anyMsg.Value, &msg); err != nil {
		return errors.Wrap(err, "failed to unmarshal MsgTransfer")
	}

	ibc, err := parseMsgTransfer(&msg, events, txHash, evmHash, blockHeight, blockTimestamp)
	if err != nil {
		return errors.Wrap(err, "failed to parse MsgTransfer")
	}

	if ibc != nil {
		data.IBCTransfers = append(data.IBCTransfers, ibc)
	}
	return nil
}

// handleMsgDepositClaim processes Peggy MsgDepositClaim messages.
func handleMsgDepositClaim(
	anyMsg *types.Any,
	cdc codec.Codec,
	events []*abci.Event,
	txHash string,
	evmHash string,
	blockHeight, blockTimestamp uint64,
	data *CrossChainData,
) error {
	var msg peggytypes.MsgDepositClaim
	if err := cdc.Unmarshal(anyMsg.Value, &msg); err != nil {
		return errors.Wrap(err, "failed to unmarshal MsgDepositClaim")
	}

	deposit, err := parseMsgDepositClaim(&msg, events, txHash, evmHash, blockHeight, blockTimestamp)
	if err != nil {
		return errors.Wrap(err, "failed to parse MsgDepositClaim")
	}

	if deposit != nil {
		data.PeggyDeposits = append(data.PeggyDeposits, deposit)
	}
	return nil
}

// handleMsgSendToEth processes Peggy MsgSendToEth messages.
func handleMsgSendToEth(
	anyMsg *types.Any,
	cdc codec.Codec,
	events []*abci.Event,
	txHash string,
	evmHash string,
	blockHeight, blockTimestamp uint64,
	data *CrossChainData,
) error {
	var msg peggytypes.MsgSendToEth
	if err := cdc.Unmarshal(anyMsg.Value, &msg); err != nil {
		return errors.Wrap(err, "failed to unmarshal MsgSendToEth")
	}

	withdrawal, err := parseMsgSendToEth(&msg, events, txHash, evmHash, blockHeight, blockTimestamp)
	if err != nil {
		return errors.Wrap(err, "failed to parse MsgSendToEth")
	}

	if withdrawal != nil {
		data.PeggyWithdrawals = append(data.PeggyWithdrawals, withdrawal)
	}
	return nil
}

// handleMsgWithdrawClaim processes Peggy MsgWithdrawClaim messages.
func handleMsgWithdrawClaim(
	anyMsg *types.Any,
	cdc codec.Codec,
	events []*abci.Event,
	txHash string,
	evmHash string,
	blockHeight, blockTimestamp uint64,
	data *CrossChainData,
) error {
	var msg peggytypes.MsgWithdrawClaim
	if err := cdc.Unmarshal(anyMsg.Value, &msg); err != nil {
		return errors.Wrap(err, "failed to unmarshal MsgWithdrawClaim")
	}

	withdrawal, err := parseMsgWithdrawClaim(&msg, events, txHash, evmHash, blockHeight, blockTimestamp)
	if err != nil {
		return errors.Wrap(err, "failed to parse MsgWithdrawClaim")
	}

	if withdrawal != nil {
		data.PeggyWithdrawals = append(data.PeggyWithdrawals, withdrawal)
	}
	return nil
}

// parseMsgDepositClaim parses a Peggy MsgDepositClaim (Ethereum → Injective).
func parseMsgDepositClaim(
	msg *peggytypes.MsgDepositClaim,
	events []*abci.Event,
	txHash string,
	evmHash string,
	blockHeight, blockTimestamp uint64,
) (*models.PeggyDepositTx, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil MsgDepositClaim")
	}

	// Convert receiver from Bech32 to EVM format for consistent storage
	receiverEVM := msg.CosmosReceiver
	if evmAddr, err := converter.InjectiveToEthAddress(msg.CosmosReceiver); err == nil {
		receiverEVM = evmAddr
	} else {
		global.Logger.Debugf("Failed to convert receiver %s to EVM format: %v, using original", msg.CosmosReceiver, err)
	}

	// Ensure sender (Ethereum address) is normalized
	senderEVM := converter.NormalizeHash(msg.EthereumSender)

	// Use EVM hash if available, otherwise use Cosmos hash and normalize it
	finalTxHash := txHash
	if evmHash != "" {
		finalTxHash = converter.NormalizeHash(evmHash)
	} else {
		finalTxHash = converter.NormalizeHash(txHash)
	}

	deposit := &models.PeggyDepositTx{
		Sender:         senderEVM,
		Receiver:       receiverEVM,
		Amount:         msg.Amount.String(),
		Denom:          fmt.Sprintf("peggy%s", msg.TokenContract),
		EventNonce:     msg.EventNonce,
		EventHeight:    msg.BlockHeight,
		State:          "completed", // MsgDepositClaim means already completed
		ClaimType:      1,            // DepositClaim = 1
		TxHash:         finalTxHash,
		BlockHeight:    blockHeight,
		BlockTimestamp: blockTimestamp,
	}

	// Extract orchestrator from message and convert to EVM format
	if msg.Orchestrator != "" {
		orchestratorEVM := msg.Orchestrator
		if evmAddr, err := converter.InjectiveToEthAddress(msg.Orchestrator); err == nil {
			orchestratorEVM = evmAddr
		} else {
			global.Logger.Debugf("Failed to convert orchestrator %s to EVM format: %v, using original", msg.Orchestrator, err)
		}
		deposit.OrchestratorAddress = orchestratorEVM
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
	evmHash string,
	blockHeight, blockTimestamp uint64,
) (*models.PeggyWithdrawalTx, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil MsgSendToEth")
	}

	// Convert sender from Bech32 to EVM format for consistent storage
	senderEVM := msg.Sender
	if evmAddr, err := converter.InjectiveToEthAddress(msg.Sender); err == nil {
		senderEVM = evmAddr
	} else {
		global.Logger.Debugf("Failed to convert sender %s to EVM format: %v, using original", msg.Sender, err)
	}

	// Ensure receiver (Ethereum address) is normalized
	receiverEVM := converter.NormalizeHash(msg.EthDest)

	// Use EVM hash if available, otherwise use Cosmos hash and normalize it
	finalTxHash := txHash
	if evmHash != "" {
		finalTxHash = converter.NormalizeHash(evmHash)
	} else {
		finalTxHash = converter.NormalizeHash(txHash)
	}

	withdrawal := &models.PeggyWithdrawalTx{
		Sender:         senderEVM,
		Receiver:       receiverEVM,
		Amount:         msg.Amount.Amount.String(),
		Denom:          msg.Amount.Denom,
		BridgeFee:      msg.BridgeFee.Amount.String(),
		State:          "pending", // Intent, not yet completed
		ClaimType:      2,          // WithdrawalClaim = 2
		TxHash:         finalTxHash,
		BlockHeight:    blockHeight,
		BlockTimestamp: blockTimestamp,
	}

	// Extract outgoing_tx_id and batch_timeout from events
	enrichPeggyWithdrawalFromEvents(withdrawal, events)

	return withdrawal, nil
}

// enrichPeggyWithdrawalFromEvents extracts outgoing_tx_id and batch_timeout from events.
func enrichPeggyWithdrawalFromEvents(withdrawal *models.PeggyWithdrawalTx, events []*abci.Event) {
	// Debug: log all event types to understand what events are actually present
	eventTypes := make(map[string]int)
	for _, event := range events {
		if event == nil {
			continue // Defensive: skip nil events
		}
		eventTypes[event.Type]++
		// Log send_to_eth events for debugging
		if event.Type == "send_to_eth" {
			global.Logger.Debugf("Found send_to_eth event with %d attributes", len(event.Attributes))
			for _, attr := range event.Attributes {
				global.Logger.Debugf("  - %s = %s", attr.Key, attr.Value)
				switch attr.Key {
				case "outgoing_tx_id":
					if id, err := parseUint64(attr.Value); err == nil {
						withdrawal.OutgoingTxId = id
						global.Logger.Debugf("Extracted outgoing_tx_id: %d", id)
					} else {
						global.Logger.Warnf("Failed to parse outgoing_tx_id: %v", err)
					}
				case "batch_timeout":
					if timeout, err := parseUint64(attr.Value); err == nil {
						withdrawal.BatchTimeout = timeout
						global.Logger.Debugf("Extracted batch_timeout: %d", timeout)
					} else {
						global.Logger.Warnf("Failed to parse batch_timeout: %v", err)
					}
				}
			}
		}
	}

	// If outgoing_tx_id is still 0, log event summary for debugging
	if withdrawal.OutgoingTxId == 0 && len(events) > 0 {
		global.Logger.Debugf("No outgoing_tx_id found for withdrawal. Event types present: %v", eventTypes)
	}
}

// parseMsgWithdrawClaim parses a Peggy MsgWithdrawClaim (withdrawal confirmation).
// This updates an existing PeggyWithdrawalTx record.
func parseMsgWithdrawClaim(
	msg *peggytypes.MsgWithdrawClaim,
	_ []*abci.Event,
	txHash string,
	evmHash string,
	blockHeight, blockTimestamp uint64,
) (*models.PeggyWithdrawalTx, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil MsgWithdrawClaim")
	}

	// Use EVM hash if available, otherwise use Cosmos hash and normalize it
	finalTxHash := txHash
	if evmHash != "" {
		finalTxHash = converter.NormalizeHash(evmHash)
	} else {
		finalTxHash = converter.NormalizeHash(txHash)
	}

	// MsgWithdrawClaim confirms a batch withdrawal
	// We create a record that will be upserted based on batch_nonce relation
	withdrawal := &models.PeggyWithdrawalTx{
		BatchNonce:     msg.BatchNonce,
		EventNonce:     msg.EventNonce,
		EventHeight:    msg.BlockHeight,
		State:          "completed",
		ClaimType:      2, // WithdrawalClaim = 2
		TxHash:         finalTxHash,
		BlockHeight:    blockHeight,
		BlockTimestamp: blockTimestamp,
	}

	// Extract orchestrator if available and convert to EVM format
	if msg.Orchestrator != "" {
		orchestratorEVM := msg.Orchestrator
		if evmAddr, err := converter.InjectiveToEthAddress(msg.Orchestrator); err == nil {
			orchestratorEVM = evmAddr
		} else {
			global.Logger.Debugf("Failed to convert orchestrator %s to EVM format: %v, using original", msg.Orchestrator, err)
		}
		withdrawal.OrchestratorAddress = orchestratorEVM
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
	evmHash string,
	blockHeight, blockTimestamp uint64,
) (*models.IBCTransferTx, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil MsgTransfer")
	}

	// Convert sender from Bech32 to EVM format if it's a Injective address
	senderEVM := msg.Sender
	if evmAddr, err := converter.InjectiveToEthAddress(msg.Sender); err == nil {
		senderEVM = evmAddr
	} else {
		// For IBC, sender might be from another chain, keep original if conversion fails
		global.Logger.Debugf("Sender %s is not a Injective address or conversion failed: %v, using original", msg.Sender, err)
	}

	// Convert receiver from Bech32 to EVM format if it's a Injective address
	receiverEVM := msg.Receiver
	if evmAddr, err := converter.InjectiveToEthAddress(msg.Receiver); err == nil {
		receiverEVM = evmAddr
	} else {
		// For IBC, receiver might be from another chain, keep original if conversion fails
		global.Logger.Debugf("Receiver %s is not a Injective address or conversion failed: %v, using original", msg.Receiver, err)
	}

	// Use EVM hash if available, otherwise use Cosmos hash and normalize it
	finalTxHash := txHash
	if evmHash != "" {
		finalTxHash = converter.NormalizeHash(evmHash)
	} else {
		finalTxHash = converter.NormalizeHash(txHash)
	}

	transfer := &models.IBCTransferTx{
		Sender:           senderEVM,
		Receiver:         receiverEVM,
		SourcePort:       msg.SourcePort,
		SourceChannel:    msg.SourceChannel,
		Amount:           msg.Token.Amount.String(),
		Denom:            msg.Token.Denom,
		TimeoutTimestamp: msg.TimeoutTimestamp,
		State:            "completed",
		TxHash:           finalTxHash,
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
		if event == nil {
			continue // Defensive: skip nil events
		}
		if event.Type == "send_packet" {
			for _, attr := range event.Attributes {
				switch attr.Key {
				case "packet_sequence":
					if seq, err := parseUint64(attr.Value); err == nil {
						transfer.PacketSequence = seq
					} else {
						global.Logger.Warnf("Failed to parse packet_sequence: %v", err)
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

	// Save Peggy deposits (UPSERT on event_nonce if non-zero, otherwise use tx_hash)
	for _, deposit := range data.PeggyDeposits {
		if deposit.EventNonce > 0 {
			// Normal case: use event_nonce as unique key
			if err := db.Get().WithContext(ctx).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "event_nonce"}},
				DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
			}).Create(deposit).Error; err != nil {
				return errors.Wrap(err, "failed to save Peggy deposit")
			}
		} else {
			// Fallback: use tx_hash as unique key when event_nonce is 0
			global.Logger.Warnf("MsgDepositClaim with event_nonce=0 in tx %s, using tx_hash as unique key", deposit.TxHash)
			if err := db.Get().WithContext(ctx).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "tx_hash"}},
				DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
			}).Create(deposit).Error; err != nil {
				return errors.Wrap(err, "failed to save Peggy deposit with zero event_nonce")
			}
		}
	}

	// Save Peggy withdrawals
	// Strategy:
	// - MsgSendToEth: Create new record with outgoing_tx_id (UPSERT on outgoing_tx_id)
	// - MsgWithdrawClaim: Update existing record via batch_nonce lookup
	for _, withdrawal := range data.PeggyWithdrawals {
		if err := savePeggyWithdrawal(ctx, withdrawal); err != nil {
			return errors.Wrap(err, "failed to save Peggy withdrawal")
		}
	}

	return nil
}

// savePeggyWithdrawal saves a single Peggy withdrawal with proper logic for MsgSendToEth vs MsgWithdrawClaim.
func savePeggyWithdrawal(ctx context.Context, withdrawal *models.PeggyWithdrawalTx) error {
	// MsgWithdrawClaim: has batch_nonce but no outgoing_tx_id
	// MsgSendToEth: has outgoing_tx_id but no batch_nonce (initially)

	if withdrawal.BatchNonce > 0 && withdrawal.OutgoingTxId == 0 {
		// This is a MsgWithdrawClaim - update existing record by batch_nonce
		// Use a direct UPDATE query to avoid SELECT roundtrip

		updates := map[string]interface{}{
			"state":                "completed",
			"batch_nonce":          withdrawal.BatchNonce,
			"event_nonce":          withdrawal.EventNonce,
			"event_height":         withdrawal.EventHeight,
			"orchestrator_address": withdrawal.OrchestratorAddress,
			"updated_at":           time.Now(),
		}
		if withdrawal.EthTxHash != "" {
			updates["eth_tx_hash"] = withdrawal.EthTxHash
		}

		// Execute UPDATE and check RowsAffected to determine if record exists
		result := db.Get().WithContext(ctx).
			Model(&models.PeggyWithdrawalTx{}).
			Where("batch_nonce = ?", withdrawal.BatchNonce).
			Updates(updates)

		if result.Error != nil {
			return errors.Wrap(result.Error, "failed to update withdrawal by batch_nonce")
		}

		// If no rows were affected, the record doesn't exist yet
		// This might be a MsgWithdrawClaim arriving before MsgSendToEth
		if result.RowsAffected == 0 {
			global.Logger.Warnf("MsgWithdrawClaim with batch_nonce=%d has no corresponding MsgSendToEth, creating new record for tx %s",
				withdrawal.BatchNonce, withdrawal.TxHash)
			// Use UPSERT with batch_nonce as the unique key
			return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "batch_nonce"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"state", "event_nonce", "event_height",
					"orchestrator_address", "eth_tx_hash", "updated_at",
				}),
			}).Create(withdrawal).Error
		}

		return nil

	} else if withdrawal.OutgoingTxId > 0 {
		// This is a MsgSendToEth - create or update by outgoing_tx_id
		return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "outgoing_tx_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"state", "batch_nonce", "event_nonce", "event_height",
				"orchestrator_address", "eth_tx_hash", "updated_at",
			}),
		}).Create(withdrawal).Error

	} else {
		// Both outgoing_tx_id and batch_nonce are 0 - this shouldn't happen
		// Fall back to using tx_hash as unique key
		global.Logger.Warnf("Peggy withdrawal with both outgoing_tx_id=0 and batch_nonce=0 in tx %s, using tx_hash as unique key", withdrawal.TxHash)
		return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tx_hash"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"state", "batch_nonce", "event_nonce", "event_height",
				"orchestrator_address", "eth_tx_hash", "updated_at",
			}),
		}).Create(withdrawal).Error
	}
}
