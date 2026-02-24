package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/bytedance/sonic"

	"injective-explorer/internal/converter"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
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
	IBCTransfers       []*models.IBCTransferTx
	PeggyDeposits      []*models.PeggyDepositTx
	PeggyWithdrawals   []*models.PeggyWithdrawalTx
	PeggyBatchMappings []*models.PeggyBatchTxMapping // batch_nonce → outgoing_tx_ids mappings
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
		len(d.PeggyWithdrawals) == 0 &&
		len(d.PeggyBatchMappings) == 0
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

	// Step 5.5: Parse EventOutgoingBatch to establish batch → outgoing_tx_ids mappings
	batchMappings := parseEventOutgoingBatch(events, blockHeight, blockTimestamp)
	if len(batchMappings) > 0 {
		data.PeggyBatchMappings = append(data.PeggyBatchMappings, batchMappings...)
	}

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
		EthTxHash:      converter.NormalizeHash(evmHash), // Store EVM hash if this is an EVM transaction
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
		EthTxHash:      converter.NormalizeHash(evmHash), // Store EVM hash if this is an EVM transaction
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

// parseEventOutgoingBatch parses EventOutgoingBatch events and creates batch mappings.
// This function extracts batch_nonce → outgoing_tx_ids mappings.
func parseEventOutgoingBatch(events []*abci.Event, blockHeight, blockTimestamp uint64) []*models.PeggyBatchTxMapping {
	var mappings []*models.PeggyBatchTxMapping

	for _, event := range events {
		if event == nil {
			continue
		}

		// EventOutgoingBatch from Injective Peggy v1
		if event.Type == "injective.peggy.v1.EventOutgoingBatch" || event.Type == "EventOutgoingBatch" {
			global.Logger.Debugf("Found EventOutgoingBatch with %d attributes", len(event.Attributes))

			var batchNonce uint64
			var batchTxIds []uint64
			var denom string

			// Parse event attributes
			for _, attr := range event.Attributes {
				global.Logger.Debugf("  - %s = %s", attr.Key, attr.Value)

				switch attr.Key {
				case "batch_nonce", "batchNonce":
					if nonce, err := parseUint64(attr.Value); err == nil {
						batchNonce = nonce
						global.Logger.Debugf("Extracted batch_nonce: %d", nonce)
					} else {
						global.Logger.Warnf("Failed to parse batch_nonce: %v", err)
					}
				case "batch_tx_ids", "batchTxIds":
					// Parse comma-separated list of outgoing_tx_ids
					// Format: "1,2,3" or as JSON array
					ids := parseUint64Array(attr.Value)
					if len(ids) > 0 {
						batchTxIds = ids
						global.Logger.Debugf("Extracted %d batch_tx_ids", len(ids))
					}
				case "denom":
					denom = attr.Value
					global.Logger.Debugf("Extracted denom: %s", denom)
				}
			}

			// Create mapping records
			if batchNonce > 0 && len(batchTxIds) > 0 {
				for _, txID := range batchTxIds {
					mapping := &models.PeggyBatchTxMapping{
						BatchNonce:     batchNonce,
						OutgoingTxID:   txID,
						BlockHeight:    blockHeight,
						BlockTimestamp: blockTimestamp,
					}
					mappings = append(mappings, mapping)
				}
				global.Logger.Infof("Parsed EventOutgoingBatch: batch_nonce=%d, outgoing_tx_count=%d, denom=%s",
					batchNonce, len(batchTxIds), denom)
			} else {
				global.Logger.Warnf("EventOutgoingBatch missing required fields: batch_nonce=%d, tx_ids=%d",
					batchNonce, len(batchTxIds))
			}
		}
	}

	return mappings
}

// parseUint64Array parses a comma-separated string of uint64 values.
// Handles formats: "1,2,3" or "[1,2,3]" or JSON array.
func parseUint64Array(s string) []uint64 {
	if s == "" {
		return nil
	}

	// Trim whitespace and brackets
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "[]{}")

	// Split by comma
	parts := strings.Split(s, ",")
	var result []uint64

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if val, err := parseUint64(part); err == nil {
			result = append(result, val)
		}
	}

	return result
}

// parseMsgWithdrawClaim parses a Peggy MsgWithdrawClaim (withdrawal confirmation).
// This updates an existing PeggyWithdrawalTx record.
func parseMsgWithdrawClaim(
	msg *peggytypes.MsgWithdrawClaim,
	events []*abci.Event,
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
		EthTxHash:      converter.NormalizeHash(evmHash), // Store EVM hash if this is an EVM transaction
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

	// Extract token_contract from EventWithdrawClaim and build denom
	enrichPeggyWithdrawClaimFromEvents(withdrawal, events)

	// Note: We don't have sender/receiver/amount in MsgWithdrawClaim
	// These will be preserved from the original MsgSendToEth record via UPSERT

	return withdrawal, nil
}

// enrichPeggyWithdrawClaimFromEvents extracts token_contract from EventWithdrawClaim
// and builds the denom field (peggy0x...).
func enrichPeggyWithdrawClaimFromEvents(withdrawal *models.PeggyWithdrawalTx, events []*abci.Event) {
	for _, event := range events {
		if event == nil {
			continue
		}

		// EventWithdrawClaim from Injective Peggy v1
		if event.Type == "injective.peggy.v1.EventWithdrawClaim" || event.Type == "EventWithdrawClaim" {
			global.Logger.Debugf("Found EventWithdrawClaim with %d attributes", len(event.Attributes))

			for _, attr := range event.Attributes {
				global.Logger.Debugf("  - %s = %s", attr.Key, attr.Value)

				if attr.Key == "token_contract" || attr.Key == "tokenContract" {
					// Build denom: peggy0x...
					tokenContract := attr.Value
					// Normalize to lowercase with 0x prefix
					if len(tokenContract) > 0 && tokenContract[0] == '0' && tokenContract[1] == 'x' {
						// Already has 0x prefix
					} else if len(tokenContract) > 0 && tokenContract[0] != '0' {
						tokenContract = "0x" + tokenContract
					}
					withdrawal.Denom = "peggy" + converter.NormalizeHash(tokenContract)
					global.Logger.Infof("Extracted token_contract from EventWithdrawClaim: %s → denom: %s",
						attr.Value, withdrawal.Denom)
					break
				}
			}
		}
	}

	// If no denom was found, log a warning
	if withdrawal.Denom == "" {
		global.Logger.Warnf("No token_contract found in EventWithdrawClaim for batch_nonce=%d", withdrawal.BatchNonce)
	}
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
// Handles Cosmos event attribute values that may be JSON-encoded with surrounding quotes (e.g. "\"12\"").
func parseUint64(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'")
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

	// Save Peggy deposits (UPSERT on composite unique key)
	// Note: MsgDepositClaim is NOT a transfer transaction - it's a declaration/attestation
	// Multiple transactions (votes) can have the same event_nonce
	// We now allow multiple txs with same event_nonce (Cosmos + EVM representations)
	// using composite unique key: (block_height, event_nonce, tx_hash)
	for _, deposit := range data.PeggyDeposits {
		if err := db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "block_height"}, {Name: "event_nonce"}, {Name: "tx_hash"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"state", "updated_at",
				"sender",      // Update sender
				"receiver",    // Update receiver
				"amount",      // Update amount
				"denom",       // Update denom
				"orchestrator_address", // Update orchestrator
				"block_height", "block_timestamp", // Update block info
			}),
		}).Create(deposit).Error; err != nil {
			return errors.Wrap(err, "failed to save Peggy deposit")
		}
		if deposit.EventNonce > 0 {
			global.Logger.Debugf("saved Peggy deposit: event_nonce=%d, block_height=%d, tx_hash=%s",
				deposit.EventNonce, deposit.BlockHeight, deposit.TxHash)
		} else {
			global.Logger.Warnf("Peggy deposit with event_nonce=0 in tx %s", deposit.TxHash)
		}
	}

	// Save Peggy withdrawals - MsgSendToEth: Create via outgoing_tx_id
	// - MsgWithdrawClaim: Update existing record via batch_nonce lookup
	for _, withdrawal := range data.PeggyWithdrawals {
		if err := savePeggyWithdrawal(ctx, withdrawal); err != nil {
			return errors.Wrap(err, "failed to save Peggy withdrawal")
		}
	}

	// Save Peggy batch mappings (batch_nonce → outgoing_tx_ids)
	// Use ON CONFLICT to handle duplicates (idempotent)
	for _, mapping := range data.PeggyBatchMappings {
		if err := db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "batch_nonce"}, {Name: "outgoing_tx_id"}},
			DoNothing: true, // Skip if already exists (idempotent)
		}).Create(mapping).Error; err != nil {
			// Log error but don't fail the entire transaction
			global.Logger.Warnf("Failed to save Peggy batch mapping (batch_nonce=%d, outgoing_tx_id=%d): %v",
				mapping.BatchNonce, mapping.OutgoingTxID, err)
		}
	}

	// Enqueue sync tasks for Peggy transactions with empty eth_tx_hash
	// This is done asynchronously after successful save
	enqueuePeggyEvmHashSyncTasks(ctx, data)

	return nil
}

// savePeggyWithdrawal saves a single Peggy withdrawal with proper logic for MsgSendToEth vs MsgWithdrawClaim.
func savePeggyWithdrawal(ctx context.Context, withdrawal *models.PeggyWithdrawalTx) error {
	// MsgWithdrawClaim: has batch_nonce but no outgoing_tx_id (or with outgoing_tx_id after completion)
	// MsgSendToEth: has outgoing_tx_id but no batch_nonce (initially)

	if withdrawal.OutgoingTxId > 0 {
		// This is a MsgSendToEth - create or update by outgoing_tx_id
		// Note: outgoing_tx_id is unique per withdrawal intent
		return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "outgoing_tx_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"state", "batch_nonce", "event_nonce", "event_height",
				"orchestrator_address", "eth_tx_hash", "denom", "updated_at", // Include denom for EventWithdrawClaim updates
			}),
		}).Create(withdrawal).Error

	} else if withdrawal.BatchNonce > 0 {
		// This is a MsgWithdrawClaim - batch completion confirmation
		// Note: MsgWithdrawClaim is an attestation/vote, not the actual withdrawal
		// Multiple transactions (Cosmos + EVM) can have the same batch_nonce
		// Use composite unique key: (block_height, batch_nonce, tx_hash)
		return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "block_height"}, {Name: "batch_nonce"}, {Name: "tx_hash"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"state", "updated_at",
				"event_nonce", "event_height",
				"orchestrator_address", "eth_tx_hash", "denom", // Include denom for EventWithdrawClaim updates
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
				"orchestrator_address", "eth_tx_hash", "denom", "updated_at",
			}),
		}).Create(withdrawal).Error
	}
}

// enqueuePeggyEvmHashSyncTasks enqueues sync tasks for Peggy transactions with empty eth_tx_hash.
// This is called after saving Peggy deposits/withdrawals to trigger async EVM hash synchronization.
func enqueuePeggyEvmHashSyncTasks(ctx context.Context, data *CrossChainData) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		global.Logger.Warn("Redis client not available for Peggy EVM hash sync task enqueuing")
		return
	}

	const queueKey = "peggy:evm_hash:queue"
	var tasksEnqueued int

	// Enqueue deposit tasks
	for _, deposit := range data.PeggyDeposits {
		// Only enqueue if eth_tx_hash is empty
		if deposit.EthTxHash != "" {
			continue
		}

		task := map[string]interface{}{
			"type":         "deposit",
			"block_height": deposit.EventHeight, // Use event_height (Ethereum height)
			"event_nonce":  deposit.EventNonce,
			"tx_hash":      deposit.TxHash,
			"denom":        deposit.Denom,
			"amount":       deposit.Amount,
			"sender":       deposit.Sender,
		}

		taskData, err := sonic.Marshal(task)
		if err != nil {
			global.Logger.Errorf("Failed to marshal deposit sync task: %v", err)
			continue
		}

		// Use LPUSH to add to queue (RPOP on worker side)
		if err := redisClient.LPush(ctx, queueKey, taskData).Err(); err != nil {
			global.Logger.Errorf("Failed to enqueue deposit sync task: %v", err)
			continue
		}

		tasksEnqueued++
		global.Logger.Debugf("Enqueued deposit sync task: event_nonce=%d, event_height=%d, tx_hash=%s",
			deposit.EventNonce, deposit.EventHeight, deposit.TxHash)
	}

	// Enqueue withdrawal tasks
	for _, withdrawal := range data.PeggyWithdrawals {
		// Only enqueue if eth_tx_hash is empty
		if withdrawal.EthTxHash != "" {
			continue
		}

		// Only enqueue withdrawals with batch_nonce (MsgWithdrawClaim)
		if withdrawal.BatchNonce == 0 {
			continue
		}

		task := map[string]interface{}{
			"type":         "withdrawal",
			"block_height": withdrawal.EventHeight,
			"batch_nonce":  withdrawal.BatchNonce,
			"tx_hash":      withdrawal.TxHash,
			"denom":        withdrawal.Denom,
		}

		taskData, err := sonic.Marshal(task)
		if err != nil {
			global.Logger.Errorf("Failed to marshal withdrawal sync task: %v", err)
			continue
		}

		// Use LPUSH to add to queue (RPOP on worker side)
		if err := redisClient.LPush(ctx, queueKey, taskData).Err(); err != nil {
			global.Logger.Errorf("Failed to enqueue withdrawal sync task: %v", err)
			continue
		}

		tasksEnqueued++
		global.Logger.Debugf("Enqueued withdrawal sync task: batch_nonce=%d, tx_hash=%s",
			withdrawal.BatchNonce, withdrawal.TxHash)
	}

	if tasksEnqueued > 0 {
		global.Logger.Infof("Enqueued %d Peggy EVM hash sync tasks", tasksEnqueued)
	}
}
