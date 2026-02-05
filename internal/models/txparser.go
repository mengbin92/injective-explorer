// Package models provides a unified, event-first extractor for "from/to/value" from
// Injective / Cosmos SDK transactions. It supports bank, wasm, ibc, staking/gov (no value),
// and has hooks for Injective exchange/oracle types. The extractor prefers events/logs
// (coin_spent, coin_received, transfer, wasm execute) and falls back to message fields
// and signatures. It is resilient to simulation results and contract-internal transfers
// as long as events are emitted in logs.

package models

import (
	"encoding/json" // Only for json.RawMessage type
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"

	"injective-explorer/internal/global"
	"injective-explorer/internal/utils"
)

// Coin represents a token amount.
type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"` // keep as string to avoid precision/int issues
}

// ExtractResult is the normalized result the extractor returns.
type ExtractResult struct {
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Values []Coin `json:"values,omitempty"`
	// Additional debugging info
	MsgTypes []string `json:"msg_types,omitempty"`
	Notes    []string `json:"notes,omitempty"`
	// ShouldShowValue indicates whether this transaction should display to/value
	// Only true for actual asset transfers (A → B), not for instructions/state changes
	ShouldShowValue bool `json:"should_show_value,omitempty"`
	// Cross-chain transaction details
	IBCTransfer     *IBCTransferDetail     `json:"ibc_transfer,omitempty"`
	PeggyDeposit    *PeggyDepositDetail    `json:"peggy_deposit,omitempty"`
	PeggyWithdrawal *PeggyWithdrawalDetail `json:"peggy_withdrawal,omitempty"`
}

// IBCTransferDetail contains IBC transfer transaction details.
type IBCTransferDetail struct {
	SourcePort         string
	SourceChannel      string
	DestinationPort    string
	DestinationChannel string
	PacketSequence     uint64
	TimeoutHeight      string
	TimeoutTimestamp   uint64
	DataHex            []byte
}

// PeggyDepositDetail contains Peggy deposit transaction details.
type PeggyDepositDetail struct {
	EventNonce          uint64
	EventHeight         uint64
	OrchestratorAddress string
	ClaimType           int32
	EthTxHash           string
}

// PeggyWithdrawalDetail contains Peggy withdrawal transaction details.
type PeggyWithdrawalDetail struct {
	BridgeFee           string
	OutgoingTxId        uint64
	BatchTimeout        uint64
	BatchNonce          uint64
	OrchestratorAddress string
	EventNonce          uint64
	EventHeight         uint64
	ClaimType           int32
	EthTxHash           string
}

// Minimal structs matching the JSON layout. We only include the fields we need.
type TxJSON struct {
	Messages   []MessageRaw   `json:"messages"`
	Logs       []LogRaw       `json:"logs"`
	Signatures []SignatureRaw `json:"signatures"`
	GasFee     *GasFee        `json:"gas_fee,omitempty"`
}

type GasFee struct {
	Payer string `json:"payer"`
}

// MessageRaw holds type and raw message object
type MessageRaw struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// LogRaw holds msg_index and events
type LogRaw struct {
	MsgIndex string     `json:"msg_index"`
	Events   []EventRaw `json:"events"`
}

type EventRaw struct {
	Type       string         `json:"type"`
	Attributes []AttributeRaw `json:"attributes"`
}

type AttributeRaw struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"` // sometimes string, sometimes []object
}

type SignatureRaw struct {
	Address string `json:"address"`
}

// extractToAndValueFromProto extracts to and value from protobuf transaction data.
// It converts protobuf data to JSON format and uses event-first extraction logic.
// Returns the recipient address and an array of values for multi-currency transfers.
func extractToAndValueFromProto(messagesBytes []byte, events []*injective_explorer_rpcpb.Event, logsBytes []byte, signatures []*injective_explorer_rpcpb.Signature, gasFee *injective_explorer_rpcpb.GasFee) (to string, value []string) {
	// Build TxJSON structure from protobuf data
	txJSON := &TxJSON{}

	// Convert messages from TxBody
	// Note: Messages are protobuf-encoded, so we primarily rely on Events/Logs for extraction
	// Messages are included for fallback parsing when events don't contain enough info
	if len(messagesBytes) > 0 {
		txBody := &cosmostxpb.TxBody{}
		if err := txBody.Unmarshal(messagesBytes); err == nil {
			for _, msgAny := range txBody.Messages {
				if msgAny == nil {
					continue
				}
				typeURL := msgAny.TypeUrl
				msgValue := msgAny.GetValue()

				if len(msgValue) > 0 {
					// Try to unmarshal as JSON first (some indexers might return JSON)
					var jsonValue map[string]interface{}
					if err := sonic.Unmarshal(msgValue, &jsonValue); err == nil {
						// It's already JSON, use it directly
						if msgJSONBytes, err := sonic.Marshal(jsonValue); err == nil {
							txJSON.Messages = append(txJSON.Messages, MessageRaw{
								Type:    typeURL,
								Message: json.RawMessage(msgJSONBytes),
							})
						}
					} else {
						// It's protobuf binary - create a minimal structure with type URL
						// The message parser will use type URL to determine parsing strategy
						// For protobuf-encoded messages, we rely more on Events/Logs
						msgObj := map[string]interface{}{
							"@type": typeURL,
						}
						if msgJSONBytes, err := sonic.Marshal(msgObj); err == nil {
							txJSON.Messages = append(txJSON.Messages, MessageRaw{
								Type:    typeURL,
								Message: json.RawMessage(msgJSONBytes),
							})
						}
					}
				}
			}
		}
	}

	// Convert events to LogRaw format
	// Parse events from protobuf Event structure
	// First, try to convert events to JSON and parse them
	if len(events) > 0 {
		log := LogRaw{
			MsgIndex: "-1", // Use "-1" to mark events from protobuf (distinguish from logsBytes)
			Events:   []EventRaw{},
		}
		for _, event := range events {
			if event == nil {
				continue
			}

			// Event has Type (string) and Attributes (map[string]string)
			eventRaw := EventRaw{
				Type:       event.GetType(),
				Attributes: []AttributeRaw{},
			}

			// Convert map[string]string to []AttributeRaw
			attrs := event.GetAttributes()
			for key, value := range attrs {
				eventRaw.Attributes = append(eventRaw.Attributes, AttributeRaw{
					Key:   key,
					Value: value,
				})
			}

			log.Events = append(log.Events, eventRaw)
		}
		if len(log.Events) > 0 {
			txJSON.Logs = append(txJSON.Logs, log)
		}
	}

	// Also parse logs from logsBytes - this is often more reliable as it contains structured JSON
	// Logs typically contain the full event structure with proper JSON formatting
	if len(logsBytes) > 0 {
		// Try to parse as JSON array of logs
		var logs []LogRaw
		if err := sonic.Unmarshal(logsBytes, &logs); err == nil {
			txJSON.Logs = append(txJSON.Logs, logs...)
		} else {
			// Try to parse as single log object
			var singleLog LogRaw
			if err := sonic.Unmarshal(logsBytes, &singleLog); err == nil {
				txJSON.Logs = append(txJSON.Logs, singleLog)
			} else {
				// Try to find JSON array pattern in the string
				logsStr := string(logsBytes)
				if strings.Contains(logsStr, "[") && strings.Contains(logsStr, "events") {
					// Try to extract JSON array manually
					startIdx := strings.Index(logsStr, "[")
					endIdx := strings.LastIndex(logsStr, "]")
					if startIdx >= 0 && endIdx > startIdx {
						jsonArray := logsStr[startIdx : endIdx+1]
						if err := sonic.Unmarshal([]byte(jsonArray), &logs); err == nil {
							txJSON.Logs = append(txJSON.Logs, logs...)
						}
					}
				}
			}
		}
	}

	// Convert signatures
	for _, sig := range signatures {
		if sig != nil && sig.Address != "" {
			txJSON.Signatures = append(txJSON.Signatures, SignatureRaw{
				Address: sig.Address,
			})
		}
	}

	// Convert gasFee
	if gasFee != nil && gasFee.Payer != "" {
		txJSON.GasFee = &GasFee{
			Payer: gasFee.Payer,
		}
	}

	// Extract using event-first logic
	result, err := extractFromToValue(txJSON)
	if err != nil {
		// If extraction fails, return default values
		return "-", nil
	}

	// Only return to/value if this is a value transfer transaction
	// For non-transfer transactions (instructions, state changes), return default values
	if !result.ShouldShowValue {
		return "-", nil
	}

	// Convert result to to/value strings
	to = result.To
	if len(result.Values) > 0 {
		// Format value as array of "amountdenom" strings for multi-currency support
		value = make([]string, 0, len(result.Values))
		for _, coin := range result.Values {
			value = append(value, coin.Amount+coin.Denom)
		}
	} else {
		// If no values found even for transfer transaction, return nil
		value = nil
	}

	// If to is empty even for transfer transaction, return "-"
	if to == "" {
		to = "-"
	}

	return to, value
}

// IsValueTransfer determines if a message type represents an actual asset transfer (A → B)
// that should display to/value. Returns true only for genuine transfers, not for
// instructions, state changes, or module operations.
//
// This function implements the core rule: "Only transactions that represent actual asset
// transfers (A → B) should display to/value. Instructions, state changes, and module
// operations should not display these fields."
//
// Rules:
//   - Returns true for: MsgSend, MsgTransfer (IBC), MsgDeposit/Withdraw (Exchange), MsgExecuteContract (with funds > 0)
//   - Returns false for: orders, staking, governance, oracle, authz, etc. (these are instructions/state changes)
//
// Parameters:
//   - msgType: The message type URL (e.g., "/cosmos.bank.v1beta1.MsgSend")
//   - hasFunds: For MsgExecuteContract, whether the contract call includes funds
//
// Returns:
//   - bool: true if this message type represents an asset transfer, false otherwise
func IsValueTransfer(msgType string, hasFunds bool) bool {
	switch msgType {
	case "/cosmos.bank.v1beta1.MsgSend",
		"/ibc.applications.transfer.v1.MsgTransfer",
		"/injective.exchange.v1beta1.MsgDeposit",
		"/injective.exchange.v1beta1.MsgWithdraw",
		"/injective.exchange.v1beta1.MsgSubaccountTransfer",
		"/injective.exchange.v1beta1.MsgExternalTransfer",
		"/injective.insurance.v1beta1.MsgUnderwrite",
		"/injective.insurance.v1beta1.MsgRequestRedemption",
		"/injective.tokenfactory.v1beta1.MsgMint",
		"/injective.tokenfactory.v1beta1.MsgBurn",
		"/injective.peggy.v1.MsgSendToEth",
		"/injective.wasmx.v1.MsgExecuteContractCompat":
		return true
	case "/cosmwasm.wasm.v1.MsgExecuteContract":
		// Only show to/value if funds > 0 (actual transfer to contract)
		return hasFunds
	default:
		return false
	}
}

// hasAnyValueTransferMessage checks if any message in the transaction is a value transfer type.
// This is used to determine if the transaction should show to/value fields.
//
// The function iterates through all messages in the transaction and checks if any of them
// represent actual asset transfers. For MsgExecuteContract, it also checks if funds are present.
//
// Parameters:
//   - tx: The transaction JSON structure containing messages
//
// Returns:
//   - bool: true if at least one message is a value transfer type, false otherwise
func hasAnyValueTransferMessage(tx *TxJSON) bool {
	for _, m := range tx.Messages {
		// For MsgExecuteContract, need to check if it has funds
		if m.Type == "/cosmwasm.wasm.v1.MsgExecuteContract" {
			var tmp struct {
				Funds []Coin `json:"funds"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				if len(tmp.Funds) > 0 {
					return true
				}
			} else {
				global.Logger.Debugf("Failed to unmarshal /cosmwasm.wasm.v1.MsgExecuteContract in hasAnyValueTransferMessage: %v", err)
			}
		} else if m.Type == "/injective.wasmx.v1.MsgExecuteContractCompat" {
			// For MsgExecuteContractCompat, check if funds string is not empty
			var tmp struct {
				Funds string `json:"funds"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				if tmp.Funds != "" {
					return true
				}
			} else {
				global.Logger.Debugf("Failed to unmarshal /injective.wasmx.v1.MsgExecuteContractCompat in hasAnyValueTransferMessage: %v", err)
			}
		} else if IsValueTransfer(m.Type, false) {
			return true
		}
	}
	return false
}

// extractFromToValue accepts a TxJSON structure and returns a best-effort normalized ExtractResult using event-first logic.
// It only extracts to/value for transactions that represent actual asset transfers (A → B),
// not for instructions, state changes, or module operations.
func extractFromToValue(tx *TxJSON) (*ExtractResult, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}

	res := &ExtractResult{}

	// Collect all message types for context
	for _, m := range tx.Messages {
		res.MsgTypes = append(res.MsgTypes, m.Type)
	}

	// Check if this transaction contains any value transfer messages
	// If not, we should not extract to/value (it's an instruction/state change, not a transfer)
	hasTransferMsg := hasAnyValueTransferMessage(tx)
	hasNonTransferMsg := false
	// Check if there are any known non-transfer message types
	for _, m := range tx.Messages {
		if m.Type == "" {
			continue // Skip empty message types
		}
		// For MsgExecuteContract, we need to check funds
		if m.Type == "/cosmwasm.wasm.v1.MsgExecuteContract" {
			var tmp struct {
				Funds []Coin `json:"funds"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				if len(tmp.Funds) == 0 {
					hasNonTransferMsg = true
					break
				}
			} else {
				global.Logger.Debugf("Failed to unmarshal /cosmwasm.wasm.v1.MsgExecuteContract in extractFromToValue: %v", err)
			}
		} else if m.Type == "/injective.wasmx.v1.MsgExecuteContractCompat" {
			// For MsgExecuteContractCompat, check if funds string is empty
			var tmp struct {
				Funds string `json:"funds"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				if tmp.Funds == "" {
					hasNonTransferMsg = true
					break
				}
			} else {
				global.Logger.Debugf("Failed to unmarshal /injective.wasmx.v1.MsgExecuteContractCompat in extractFromToValue: %v", err)
			}
		} else if !IsValueTransfer(m.Type, false) {
			// Known non-transfer message type
			hasNonTransferMsg = true
			break
		}
	}

	// 1) Event-first pass: parse logs for coin_spent / coin_received / transfer / wasm events
	// Events are the most reliable indicator of actual transfers
	// BUT: We must check message types - even if events exist (e.g., gas fee events),
	// we should only show to/value if the message type is actually a transfer type
	if eventsFound := extractFromEventsJSON(tx, res); eventsFound {
		// Decision logic:
		// 1. If there are transfer message types -> show to/value (even if only events found)
		// 2. If there are non-transfer message types -> don't show to/value (events are likely gas fee)
		// 3. If no messages at all -> show to/value (rely on events, assume they're valid transfers)
		if hasTransferMsg {
			// Transfer message type exists, show to/value
			res.ShouldShowValue = true
		} else if hasNonTransferMsg {
			// Non-transfer message type exists, don't show to/value even if events found
			// (those events are likely gas fee related, not the transaction's purpose)
			res.To = ""
			res.Values = nil
			res.ShouldShowValue = false
			res.Notes = append(res.Notes, "transfer events found but message type is not a transfer - ignoring to/value")
		} else {
			// No messages or unknown message types, rely on events
			// This handles cases where transactions only have events (e.g., contract internal transfers)
			res.ShouldShowValue = true
			res.Notes = append(res.Notes, "no known message types found, relying on events for to/value")
		}
		// fill missing 'from' from signatures if absent
		if res.From == "" {
			if len(tx.Signatures) > 0 {
				res.From = tx.Signatures[0].Address
				res.Notes = append(res.Notes, "from filled from first signature")
			}
		}
		return res, nil
	}

	// 2) Message-level fallback: inspect each message for known transfer fields
	// Only extract if this is a value transfer message type
	if hasTransferMsg {
		extracted := extractFromMessagesJSON(tx, res)
		if extracted {
			res.ShouldShowValue = true
			// if still no from, try signatures/gasFee.payer
			if res.From == "" {
				if len(tx.Signatures) > 0 {
					res.From = tx.Signatures[0].Address
					res.Notes = append(res.Notes, "from filled from first signature")
				} else if tx.GasFee != nil && tx.GasFee.Payer != "" {
					res.From = tx.GasFee.Payer
					res.Notes = append(res.Notes, "from filled from gasFee.payer")
				}
			}
			return res, nil
		}
	}

	// 3) For non-transfer transactions (instructions, state changes), we still extract signer
	// but mark that to/value should not be shown
	res.ShouldShowValue = false
	if len(tx.Signatures) > 0 {
		res.From = tx.Signatures[0].Address
		res.Notes = append(res.Notes, "non-transfer transaction: signer only, no to/value")
		return res, nil
	}

	if tx.GasFee != nil && tx.GasFee.Payer != "" {
		res.From = tx.GasFee.Payer
		res.Notes = append(res.Notes, "non-transfer transaction: payer only, no to/value")
		return res, nil
	}

	// If we can't determine anything, still return a result (empty but valid)
	res.Notes = append(res.Notes, "cannot determine transaction details")
	return res, nil
}

// extractFromEventsJSON scans logs/events and tries to populate res. It returns true if
// any transfer-related event was found and applied.
// It prioritizes transfer events with valid msg_index (actual message transfers)
// and excludes events without msg_index (typically gas fee transfers).
func extractFromEventsJSON(tx *TxJSON, res *ExtractResult) bool {
	transferFound := false

	// Collect candidate transfers across all logs; prefer explicit transfer or coin_received
	type transferCandidate struct {
		From        string
		To          string
		Amount      []Coin
		Priority    int    // lower is higher priority
		MsgIndex    string // msg_index from log, empty means gas fee or transaction-level event
		HasMsgIndex bool   // true if msg_index is valid (not empty and not "0" for gas fee)
	}

	cands := []transferCandidate{}

	for _, log := range tx.Logs {
		// Process events in this log
		// Gas fee events typically don't have msg_index or have empty msg_index
		// Actual message transfer events have msg_index like "0", "1", etc.
		// Note: log.MsgIndex might be "0" for protobuf events (hardcoded), so we need to check
		// if the event itself has msg_index attribute, or if log.MsgIndex is from logsBytes (structured)
		for _, ev := range log.Events {
			// Check if event has msg_index attribute (more reliable indicator)
			eventMsgIndex := ""
			for _, attr := range ev.Attributes {
				if attr.Key == "msg_index" {
					eventMsgIndex = stringifyAttrValue(attr.Value)
					break
				}
			}

			// Use event-level msg_index if available, otherwise use log-level msg_index
			effectiveMsgIndex := eventMsgIndex
			if effectiveMsgIndex == "" {
				effectiveMsgIndex = log.MsgIndex
			}

			// Consider msg_index valid based on the source:
			// 1. If event has msg_index attribute (explicit), it's valid
			// 2. If event is from logsBytes (log.MsgIndex != "-1"), it's valid (even without msg_index attribute)
			// 3. If event is from protobuf (log.MsgIndex == "-1") and has no msg_index attribute, it's invalid (gas fee)
			eventHasValidMsgIndex := (eventMsgIndex != "" && eventMsgIndex != "-1") || (log.MsgIndex != "" && log.MsgIndex != "-1")

			switch ev.Type {
			case "transfer":
				// attributes: recipient, sender, amount
				var t transferCandidate
				t.MsgIndex = effectiveMsgIndex
				t.HasMsgIndex = eventHasValidMsgIndex
				for _, attr := range ev.Attributes {
					k := attr.Key
					v := stringifyAttrValue(attr.Value)
					switch k {
					case "recipient":
						t.To = v
					case "sender":
						t.From = v
					case "amount":
						t.Amount = append(t.Amount, parseCoinStrList(v)...) // parse like "8inj" or "8inj,3atom"
					}
				}
				if len(t.Amount) > 0 {
					t.Priority = 10
					cands = append(cands, t)
					transferFound = true
				}

			case "coin_spent":
				var t transferCandidate
				t.MsgIndex = effectiveMsgIndex
				t.HasMsgIndex = eventHasValidMsgIndex
				for _, attr := range ev.Attributes {
					k := attr.Key
					v := stringifyAttrValue(attr.Value)
					switch k {
					case "spender":
						t.From = v
					case "amount":
						t.Amount = append(t.Amount, parseCoinStrList(v)...)
					}
				}
				if len(t.Amount) > 0 {
					t.Priority = 20
					cands = append(cands, t)
					transferFound = true
				}

			case "coin_received":
				var t transferCandidate
				t.MsgIndex = effectiveMsgIndex
				t.HasMsgIndex = eventHasValidMsgIndex
				for _, attr := range ev.Attributes {
					k := attr.Key
					v := stringifyAttrValue(attr.Value)
					switch k {
					case "receiver":
						t.To = v
					case "amount":
						t.Amount = append(t.Amount, parseCoinStrList(v)...)
					}
				}
				if len(t.Amount) > 0 {
					t.Priority = 20
					cands = append(cands, t)
					transferFound = true
				}

			case "execute":
				// wasm execute indicates contract being called — may include _contract_address attr
				var contract string
				for _, attr := range ev.Attributes {
					if attr.Key == "_contract_address" {
						contract = stringifyAttrValue(attr.Value)
					}
				}
				if contract != "" {
					// mark in notes but don't treat as transfer by itself
					res.Notes = append(res.Notes, "execute contract: "+contract)
				}
			}
		}
	}

	if !transferFound {
		return false
	}

	// Filter out gas fee transfers: prefer events with valid msg_index
	// If we have events with msg_index, only consider those
	hasMsgIndexEvents := false
	for _, c := range cands {
		if c.HasMsgIndex {
			hasMsgIndexEvents = true
			break
		}
	}

	// If we have events with msg_index, filter to only those
	// This is critical: if there are events with msg_index, events without msg_index are gas fees
	// Backward compatibility: if ALL events lack msg_index (old format), keep all events
	if hasMsgIndexEvents {
		filteredCands := []transferCandidate{}
		for _, c := range cands {
			if c.HasMsgIndex {
				filteredCands = append(filteredCands, c)
			}
		}
		if len(filteredCands) > 0 {
			cands = filteredCands
		}
	}

	// Choose highest priority candidate (smallest Priority value) and prefer the one with both from & to.
	if len(cands) == 0 {
		return false
	}

	sort.SliceStable(cands, func(i, j int) bool {
		// First priority: prefer events with valid msg_index
		if cands[i].HasMsgIndex != cands[j].HasMsgIndex {
			return cands[i].HasMsgIndex // true (has msg_index) comes before false (no msg_index)
		}
		// Second priority: smaller Priority value (transfer=10 < coin_spent/coin_received=20)
		if cands[i].Priority != cands[j].Priority {
			return cands[i].Priority < cands[j].Priority
		}
		// Third priority: prefer entries that have both from&to
		iScore := 0
		jScore := 0
		if cands[i].From != "" && cands[i].To != "" {
			iScore += 2
		}
		if cands[j].From != "" && cands[j].To != "" {
			jScore += 2
		}
		return iScore > jScore
	})

	// Use the best candidate for from/to
	best := cands[0]
	if best.From != "" {
		res.From = best.From
	}
	if best.To != "" {
		res.To = best.To
	}

	// Merge amounts from multiple messages (different msg_index)
	// Avoid double-counting: for each msg_index, only use the best candidate
	// Special case: if ALL candidates lack msg_index (old format), merge all candidates (backward compatibility)

	// Check if we have any candidates with msg_index
	hasAnyMsgIndex := false
	for _, c := range cands {
		if c.HasMsgIndex {
			hasAnyMsgIndex = true
			break
		}
	}

	if hasAnyMsgIndex {
		// Modern format: group by msg_index and take the best from each group
		msgIndexCandidates := map[string]*transferCandidate{} // msg_index -> best candidate for that msg_index
		for _, c := range cands {
			if !c.HasMsgIndex {
				continue // Skip gas fee events when we have msg_index events
			}
			if existing, ok := msgIndexCandidates[c.MsgIndex]; !ok {
				msgIndexCandidates[c.MsgIndex] = &c
			} else {
				// Prefer the one with lower Priority (transfer over coin_spent/coin_received)
				if c.Priority < existing.Priority {
					msgIndexCandidates[c.MsgIndex] = &c
				}
			}
		}

		// Merge amounts from the best candidate of each msg_index
		for _, c := range msgIndexCandidates {
			res.Values = mergeCoins(res.Values, c.Amount)
		}
	} else {
		// Old format (all candidates lack msg_index): merge all candidates to avoid double-counting
		// Group by (from, to, denom) to avoid counting duplicate events (transfer + coin_spent + coin_received)
		uniqueTransfers := map[string]Coin{} // key: "from:to:denom" -> coin
		for _, c := range cands {
			for _, coin := range c.Amount {
				key := fmt.Sprintf("%s:%s:%s", c.From, c.To, coin.Denom)
				if existing, ok := uniqueTransfers[key]; ok {
					// Sum amounts for same denom
					existing.Amount = addDecimalStrings(existing.Amount, coin.Amount)
					uniqueTransfers[key] = existing
				} else {
					uniqueTransfers[key] = coin
				}
			}
		}

		// Merge unique transfers into result
		for _, coin := range uniqueTransfers {
			res.Values = mergeCoins(res.Values, []Coin{coin})
		}
	}

	res.Notes = append(res.Notes, "extracted from events (transfer/coin_spent/coin_received)")
	if best.HasMsgIndex {
		res.Notes = append(res.Notes, "selected event with msg_index="+best.MsgIndex)
	} else {
		res.Notes = append(res.Notes, "selected event without msg_index (may be gas fee)")
	}
	return true
}

// stringifyAttrValue converts AttributeRaw.Value to string if possible.
// It handles strings and arrays, using sonic.Marshal for complex types.
func stringifyAttrValue(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		// sometimes value is an array of objects (e.g. in some indexers)
		// try to marshal simple patterns like [{"price_id":...}]
		b, _ := sonic.Marshal(t)
		return string(b)
	default:
		b, _ := sonic.Marshal(t)
		return string(b)
	}
}

// parseCoinStrList parses strings like "8inj" or "8inj,3atom" into []Coin
func parseCoinStrList(s string) []Coin {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := []Coin{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		c := parseSingleCoinStr(p)
		if c != nil {
			out = append(out, *c)
		}
	}
	return out
}

var coinSplitRe = regexp.MustCompile(`^([0-9]+)([a-zA-Z0-9/_-]+)$`)

// parseSingleCoinStr parses a single coin string like "8inj" or returns nil.
func parseSingleCoinStr(s string) *Coin {
	m := coinSplitRe.FindStringSubmatch(s)
	if len(m) == 3 {
		return &Coin{Denom: m[2], Amount: m[1]}
	}
	// fallback: try separating numeric prefix and non-numeric suffix
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9') {
		i++
	}
	if i == 0 || i >= len(s) {
		return nil
	}
	return &Coin{Amount: s[:i], Denom: s[i:]}
}

// mergeCoins merges coin slices, summing amounts for same denom (string-sum, not big.Int)
func mergeCoins(a, b []Coin) []Coin {
	m := map[string]string{}
	for _, c := range a {
		m[c.Denom] = addDecimalStrings(m[c.Denom], c.Amount)
	}
	for _, c := range b {
		m[c.Denom] = addDecimalStrings(m[c.Denom], c.Amount)
	}
	out := []Coin{}
	for denom, amt := range m {
		out = append(out, Coin{Denom: denom, Amount: amt})
	}
	// Sort coins: native token (inj) first, then others alphabetically
	sort.Slice(out, func(i, j int) bool {
		// Native token "inj" always comes first
		if out[i].Denom == "inj" && out[j].Denom != "inj" {
			return true
		}
		if out[i].Denom != "inj" && out[j].Denom == "inj" {
			return false
		}
		// For non-native tokens, sort alphabetically
		return out[i].Denom < out[j].Denom
	})
	return out
}

// addDecimalStrings naively adds two integer amounts represented as decimal strings.
func addDecimalStrings(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	// reverse strings
	ra := utils.ReverseString(a)
	rb := utils.ReverseString(b)
	carry := 0
	res := []byte{}
	n := utils.Max(len(ra), len(rb))
	for i := 0; i < n; i++ {
		d1 := 0
		if i < len(ra) {
			d1 = int(ra[i] - '0')
		}
		d2 := 0
		if i < len(rb) {
			d2 = int(rb[i] - '0')
		}
		sum := d1 + d2 + carry
		res = append(res, byte('0'+(sum%10)))
		carry = sum / 10
	}
	if carry > 0 {
		res = append(res, byte('0'+carry))
	}
	return utils.ReverseString(string(res))
}

// MessageExtractor defines the interface for extracting transfer information from a message.
type MessageExtractor interface {
	// Extract parses the message and updates the ExtractResult if transfer information is found.
	// Returns true if transfer information was extracted, false otherwise.
	Extract(msg json.RawMessage, res *ExtractResult) bool
}

// messageExtractorRegistry stores registered message extractors by message type.
var (
	messageExtractors     = make(map[string]MessageExtractor)
	messageExtractorsOnce sync.Once
)

// registerMessageExtractor registers a message extractor for a specific message type.
func registerMessageExtractor(msgType string, extractor MessageExtractor) {
	messageExtractors[msgType] = extractor
}

// initMessageExtractors initializes all message extractors.
// This function is called once using sync.Once to ensure thread-safe initialization.
func initMessageExtractors() {
	messageExtractorsOnce.Do(func() {
		// Cosmos Bank
		registerMessageExtractor("/cosmos.bank.v1beta1.MsgSend", &cosmosBankMsgSendExtractor{})

		// CosmWasm
		registerMessageExtractor("/cosmwasm.wasm.v1.MsgExecuteContract", &cosmwasmMsgExecuteContractExtractor{})

		// Injective Exchange
		registerMessageExtractor("/injective.exchange.v1beta1.MsgDeposit", &injectiveExchangeMsgDepositExtractor{})
		registerMessageExtractor("/injective.exchange.v1beta1.MsgWithdraw", &injectiveExchangeMsgWithdrawExtractor{})
		registerMessageExtractor("/injective.exchange.v1beta1.MsgSubaccountTransfer", &injectiveExchangeMsgSubaccountTransferExtractor{})
		registerMessageExtractor("/injective.exchange.v1beta1.MsgExternalTransfer", &injectiveExchangeMsgExternalTransferExtractor{})

		// Injective Insurance
		registerMessageExtractor("/injective.insurance.v1beta1.MsgUnderwrite", &injectiveInsuranceMsgUnderwriteExtractor{})
		registerMessageExtractor("/injective.insurance.v1beta1.MsgRequestRedemption", &injectiveInsuranceMsgRequestRedemptionExtractor{})

		// Injective TokenFactory
		registerMessageExtractor("/injective.tokenfactory.v1beta1.MsgMint", &injectiveTokenFactoryMsgMintExtractor{})
		registerMessageExtractor("/injective.tokenfactory.v1beta1.MsgBurn", &injectiveTokenFactoryMsgBurnExtractor{})

		// Injective WasmX
		registerMessageExtractor("/injective.wasmx.v1.MsgExecuteContractCompat", &injectiveWasmxMsgExecuteContractCompatExtractor{})
	})
}

// extractFromMessagesJSON inspects the message array for known transfer Msg shapes and fills res.
// Only processes messages that represent actual asset transfers (A → B).
// Returns true if any transfer information was extracted.
func extractFromMessagesJSON(tx *TxJSON, res *ExtractResult) bool {
	// Initialize extractors on first call
	initMessageExtractors()

	found := false
	for _, m := range tx.Messages {
		// Look up extractor for this message type
		extractor, ok := messageExtractors[m.Type]
		if !ok {
			// Unknown message type - skip (non-transfer messages like orders, staking, etc.)
			continue
		}

		// Extract transfer information using the registered extractor
		if extractor.Extract(m.Message, res) {
			found = true
		}
	}
	return found
}

// Message extractor implementations

// cosmosBankMsgSendExtractor extracts transfer info from Cosmos Bank MsgSend.
type cosmosBankMsgSendExtractor struct{}

func (e *cosmosBankMsgSendExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		From   string `json:"from_address"`
		To     string `json:"to_address"`
		Amount []Coin `json:"amount"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /cosmos.bank.v1beta1.MsgSend: %v", err)
		return false
	}
	res.From = utils.FirstNonEmpty(res.From, tmp.From)
	res.To = utils.FirstNonEmpty(res.To, tmp.To)
	res.Values = mergeCoins(res.Values, tmp.Amount)
	return true
}

// cosmwasmMsgExecuteContractExtractor extracts transfer info from CosmWasm MsgExecuteContract.
type cosmwasmMsgExecuteContractExtractor struct{}

func (e *cosmwasmMsgExecuteContractExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender   string `json:"sender"`
		Contract string `json:"contract"`
		Funds    []Coin `json:"funds"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /cosmwasm.wasm.v1.MsgExecuteContract: %v", err)
		return false
	}
	// Only treat as transfer if funds are present
	if len(tmp.Funds) > 0 {
		res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
		res.To = utils.FirstNonEmpty(res.To, tmp.Contract)
		res.Values = mergeCoins(res.Values, tmp.Funds)
		return true
	}
	// Contract call without funds - not a transfer, just record the contract
	res.Notes = append(res.Notes, "contract call without funds: "+tmp.Contract)
	return false
}

// injectiveExchangeMsgDepositExtractor extracts transfer info from Injective Exchange MsgDeposit.
type injectiveExchangeMsgDepositExtractor struct{}

func (e *injectiveExchangeMsgDepositExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender       string `json:"sender"`
		SubaccountId string `json:"subaccount_id"`
		Amount       struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.exchange.v1beta1.MsgDeposit: %v", err)
		return false
	}
	res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
	// Exchange module doesn't have a single "to" address, use subaccount or mark as exchange
	if tmp.SubaccountId != "" {
		res.To = tmp.SubaccountId
	}
	if tmp.Amount.Amount != "" {
		res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
	}
	return true
}

// injectiveExchangeMsgWithdrawExtractor extracts transfer info from Injective Exchange MsgWithdraw.
type injectiveExchangeMsgWithdrawExtractor struct{}

func (e *injectiveExchangeMsgWithdrawExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender       string `json:"sender"`
		SubaccountId string `json:"subaccount_id"`
		Amount       struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.exchange.v1beta1.MsgWithdraw: %v", err)
		return false
	}
	// For withdraw, from is the exchange module/subaccount, to is the sender
	if tmp.SubaccountId != "" {
		res.From = tmp.SubaccountId
	}
	res.To = utils.FirstNonEmpty(res.To, tmp.Sender)
	if tmp.Amount.Amount != "" {
		res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
	}
	return true
}

// injectiveExchangeMsgSubaccountTransferExtractor extracts transfer info from Injective Exchange MsgSubaccountTransfer.
type injectiveExchangeMsgSubaccountTransferExtractor struct{}

func (e *injectiveExchangeMsgSubaccountTransferExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender                  string `json:"sender"`
		SourceSubaccountID      string `json:"source_subaccount_id"`
		DestinationSubaccountID string `json:"destination_subaccount_id"`
		Amount                  struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.exchange.v1beta1.MsgSubaccountTransfer: %v", err)
		return false
	}
	res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
	// Use destination subaccount as "to"
	if tmp.DestinationSubaccountID != "" {
		res.To = utils.FirstNonEmpty(res.To, tmp.DestinationSubaccountID)
	}
	if tmp.Amount.Amount != "" {
		res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
	}
	return true
}

// injectiveExchangeMsgExternalTransferExtractor extracts transfer info from Injective Exchange MsgExternalTransfer.
type injectiveExchangeMsgExternalTransferExtractor struct{}

func (e *injectiveExchangeMsgExternalTransferExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender       string `json:"sender"`
		SubaccountID string `json:"subaccount_id"`
		Recipient    string `json:"recipient"`
		Amount       struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.exchange.v1beta1.MsgExternalTransfer: %v", err)
		return false
	}
	res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
	if tmp.Recipient != "" {
		res.To = utils.FirstNonEmpty(res.To, tmp.Recipient)
	}
	if tmp.Amount.Amount != "" {
		res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
	}
	return true
}

// injectiveInsuranceMsgUnderwriteExtractor extracts transfer info from Injective Insurance MsgUnderwrite.
type injectiveInsuranceMsgUnderwriteExtractor struct{}

func (e *injectiveInsuranceMsgUnderwriteExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender  string `json:"sender"`
		Deposit struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"deposit"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.insurance.v1beta1.MsgUnderwrite: %v", err)
		return false
	}
	res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
	// For underwrite, to is the insurance module (no specific address)
	res.To = utils.FirstNonEmpty(res.To, "insurance")
	if tmp.Deposit.Amount != "" {
		res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Deposit.Denom, Amount: tmp.Deposit.Amount}})
	}
	return true
}

// injectiveInsuranceMsgRequestRedemptionExtractor extracts transfer info from Injective Insurance MsgRequestRedemption.
type injectiveInsuranceMsgRequestRedemptionExtractor struct{}

func (e *injectiveInsuranceMsgRequestRedemptionExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender string `json:"sender"`
		Amount struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.insurance.v1beta1.MsgRequestRedemption: %v", err)
		return false
	}
	res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
	res.To = utils.FirstNonEmpty(res.To, "insurance")
	if tmp.Amount.Amount != "" {
		res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
	}
	return true
}

// injectiveTokenFactoryMsgMintExtractor extracts transfer info from Injective TokenFactory MsgMint.
type injectiveTokenFactoryMsgMintExtractor struct{}

func (e *injectiveTokenFactoryMsgMintExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender string `json:"sender"`
		Amount struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
		MintToAddress string `json:"mint_to_address"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.tokenfactory.v1beta1.MsgMint: %v", err)
		return false
	}
	res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
	if tmp.MintToAddress != "" {
		res.To = utils.FirstNonEmpty(res.To, tmp.MintToAddress)
	}
	if tmp.Amount.Amount != "" {
		res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
	}
	return true
}

// injectiveTokenFactoryMsgBurnExtractor extracts transfer info from Injective TokenFactory MsgBurn.
type injectiveTokenFactoryMsgBurnExtractor struct{}

func (e *injectiveTokenFactoryMsgBurnExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender string `json:"sender"`
		Amount struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
		BurnFromAddress string `json:"burn_from_address"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.tokenfactory.v1beta1.MsgBurn: %v", err)
		return false
	}
	// For burn, from is the address being burned from (or sender)
	if tmp.BurnFromAddress != "" {
		res.From = utils.FirstNonEmpty(res.From, tmp.BurnFromAddress)
	} else {
		res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
	}
	res.To = utils.FirstNonEmpty(res.To, "burn")
	if tmp.Amount.Amount != "" {
		res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
	}
	return true
}

// injectiveWasmxMsgExecuteContractCompatExtractor extracts transfer info from Injective WasmX MsgExecuteContractCompat.
type injectiveWasmxMsgExecuteContractCompatExtractor struct{}

func (e *injectiveWasmxMsgExecuteContractCompatExtractor) Extract(msg json.RawMessage, res *ExtractResult) bool {
	var tmp struct {
		Sender   string `json:"sender"`
		Contract string `json:"contract"`
		Funds    string `json:"funds"`
	}
	if err := sonic.Unmarshal(msg, &tmp); err != nil {
		global.Logger.Debugf("Failed to unmarshal /injective.wasmx.v1.MsgExecuteContractCompat: %v", err)
		return false
	}
	// Only treat as transfer if funds are present
	if tmp.Funds != "" {
		res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
		res.To = utils.FirstNonEmpty(res.To, tmp.Contract)
		// Parse funds string like "1000inj,500usdt"
		coins := parseCoinStrList(tmp.Funds)
		res.Values = mergeCoins(res.Values, coins)
		return true
	}
	return false
}


// parseUint64 safely parses a uint64 from string with proper error handling.
// Returns 0 for empty strings without error.
// Returns wrapped error for invalid numeric strings.
func parseUint64(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	result, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid uint64 value '%s': %w", s, err)
	}
	return result, nil
}
