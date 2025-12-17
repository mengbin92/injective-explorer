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
	"regexp"
	"sort"
	"strings"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"

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
}

// Minimal structs matching the JSON layout. We only include the fields we need.
type TxJSON struct {
	Messages   []MessageRaw   `json:"messages"`
	Logs       []LogRaw       `json:"logs"`
	Signatures []SignatureRaw `json:"signatures"`
	GasFee     *GasFee        `json:"gasFee,omitempty"`
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
func extractToAndValueFromProto(messagesBytes []byte, events []*injective_explorer_rpcpb.Event, logsBytes []byte, signatures []*injective_explorer_rpcpb.Signature, gasFee *injective_explorer_rpcpb.GasFee) (to, value string) {
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
			MsgIndex: "0",
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
		return "-", "0"
	}

	// Only return to/value if this is a value transfer transaction
	// For non-transfer transactions (instructions, state changes), return default values
	if !result.ShouldShowValue {
		return "-", "0"
	}

	// Convert result to to/value strings
	to = result.To
	if len(result.Values) > 0 {
		// Format value as "amount1denom1,amount2denom2" or just "amountdenom"
		valueParts := make([]string, 0, len(result.Values))
		for _, coin := range result.Values {
			valueParts = append(valueParts, coin.Amount+coin.Denom)
		}
		value = strings.Join(valueParts, ",")
	} else {
		// If no values found even for transfer transaction, return "0"
		value = "0"
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
// Rules:
// - Only true for: MsgSend, MsgTransfer (IBC), MsgDeposit/Withdraw (Exchange), MsgExecuteContract (with funds)
// - False for: orders, staking, governance, oracle, authz, etc. (these are instructions/state changes)
func IsValueTransfer(msgType string, hasFunds bool) bool {
	switch msgType {
	case "/cosmos.bank.v1beta1.MsgSend",
		"/ibc.applications.transfer.v1.MsgTransfer",
		"/injective.exchange.v1beta1.MsgDeposit",
		"/injective.exchange.v1beta1.MsgWithdraw":
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
func extractFromEventsJSON(tx *TxJSON, res *ExtractResult) bool {
	transferFound := false

	// Collect candidate transfers across all logs; prefer explicit transfer or coin_received
	type transferCandidate struct {
		From     string
		To       string
		Amount   []Coin
		Priority int // lower is higher priority
	}

	cands := []transferCandidate{}

	for _, log := range tx.Logs {
		for _, ev := range log.Events {
			switch ev.Type {
			case "transfer":
				// attributes: recipient, sender, amount
				var t transferCandidate
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

	// Choose highest priority candidate (smallest Priority value) and prefer the one with both from & to.
	if len(cands) == 0 {
		return false
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Priority != cands[j].Priority {
			return cands[i].Priority < cands[j].Priority
		}
		// prefer entries that have both from&to
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

	best := cands[0]
	if best.From != "" {
		res.From = best.From
	}
	if best.To != "" {
		res.To = best.To
	}
	res.Values = mergeCoins(res.Values, best.Amount)
	res.Notes = append(res.Notes, "extracted from events (transfer/coin_spent/coin_received)")
	return true
}

// stringifyAttrValue converts AttributeRaw.Value to string if possible
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
	sort.Slice(out, func(i, j int) bool {
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

// extractFromMessagesJSON inspects the message array for known transfer Msg shapes and fills res.
// Only processes messages that represent actual asset transfers (A → B).
// Returns true if any transfer information was extracted.
func extractFromMessagesJSON(tx *TxJSON, res *ExtractResult) bool {
	found := false
	for _, m := range tx.Messages {
		// Only process value transfer message types
		// Non-transfer messages (orders, staking, governance, etc.) are skipped
		switch m.Type {
		case "/cosmos.bank.v1beta1.MsgSend":
			// shape: {"from_address":"...","to_address":"...","amount":[{"denom":"...","amount":"..."}]}
			var tmp struct {
				From   string `json:"from_address"`
				To     string `json:"to_address"`
				Amount []Coin `json:"amount"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				res.From = utils.FirstNonEmpty(res.From, tmp.From)
				res.To = utils.FirstNonEmpty(res.To, tmp.To)
				res.Values = mergeCoins(res.Values, tmp.Amount)
				found = true
			}

		case "/cosmwasm.wasm.v1.MsgExecuteContract":
			// shape: {"sender":"...","contract":"...","msg":{...},"funds":[{denom,amount}]}
			// Only extract if funds > 0 (actual transfer to contract)
			var tmp struct {
				Sender   string `json:"sender"`
				Contract string `json:"contract"`
				Funds    []Coin `json:"funds"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				// Only treat as transfer if funds are present
				if len(tmp.Funds) > 0 {
					res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
					res.To = utils.FirstNonEmpty(res.To, tmp.Contract)
					res.Values = mergeCoins(res.Values, tmp.Funds)
					found = true
				} else {
					// Contract call without funds - not a transfer, just record the contract
					res.Notes = append(res.Notes, "contract call without funds: "+tmp.Contract)
				}
			}

		case "/ibc.applications.transfer.v1.MsgTransfer":
			var tmp struct {
				Sender   string `json:"sender"`
				Receiver string `json:"receiver"`
				Token    struct {
					Denom  string `json:"denom"`
					Amount string `json:"amount"`
				} `json:"token"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
				res.To = utils.FirstNonEmpty(res.To, tmp.Receiver)
				if tmp.Token.Amount != "" {
					res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Token.Denom, Amount: tmp.Token.Amount}})
				}
				found = true
			}

		case "/injective.exchange.v1beta1.MsgDeposit":
			// Exchange deposit: user deposits funds to exchange module
			var tmp struct {
				Sender       string `json:"sender"`
				SubaccountId string `json:"subaccount_id"`
				Amount       struct {
					Denom  string `json:"denom"`
					Amount string `json:"amount"`
				} `json:"amount"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				res.From = utils.FirstNonEmpty(res.From, tmp.Sender)
				// Exchange module doesn't have a single "to" address, use subaccount or mark as exchange
				if tmp.SubaccountId != "" {
					res.To = tmp.SubaccountId
				}
				if tmp.Amount.Amount != "" {
					res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
				}
				found = true
			}

		case "/injective.exchange.v1beta1.MsgWithdraw":
			// Exchange withdraw: user withdraws funds from exchange module
			var tmp struct {
				Sender       string `json:"sender"`
				SubaccountId string `json:"subaccount_id"`
				Amount       struct {
					Denom  string `json:"denom"`
					Amount string `json:"amount"`
				} `json:"amount"`
			}
			if err := sonic.Unmarshal(m.Message, &tmp); err == nil {
				// For withdraw, from is the exchange module/subaccount, to is the sender
				if tmp.SubaccountId != "" {
					res.From = tmp.SubaccountId
				}
				res.To = utils.FirstNonEmpty(res.To, tmp.Sender)
				if tmp.Amount.Amount != "" {
					res.Values = mergeCoins(res.Values, []Coin{{Denom: tmp.Amount.Denom, Amount: tmp.Amount.Amount}})
				}
				found = true
			}

		default:
			// All other message types are NOT value transfers:
			// - Exchange orders (CreateSpotOrder, CancelSpotOrder, etc.) - instructions, not transfers
			// - Staking (Delegate, Undelegate, etc.) - state changes, not transfers
			// - Governance (Vote, Deposit, etc.) - instructions/config, not transfers
			// - Oracle (RelayPriceFeedPrice, etc.) - data submission, not transfers
			// - Authz/Feegrant - permissions, not transfers
			// - IBC control messages - channel management, not transfers
			// These are already recorded in res.MsgTypes, but we don't extract to/value for them
		}
	}
	return found
}
