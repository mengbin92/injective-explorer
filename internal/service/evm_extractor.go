// Package service provides EVM transaction extraction utilities.
package service

import (
	"strings"

	"injective-explorer/internal/global"

	evmtypes "github.com/InjectiveLabs/sdk-go/chain/evm/types"
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	abci "github.com/cometbft/cometbft/abci/types"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"
)

// ExtractEVMHashFromEvents extracts EVM transaction hash from transaction events.
//
// Parameters:
//   - events: List of events from the transaction
//
// Returns:
//   - string: EVM transaction hash with 0x prefix, or empty string if not found
func ExtractEVMHashFromEvents(events []*injective_explorer_rpcpb.Event) string {
	for _, event := range events {
		if event == nil {
			continue
		}

		// Check event type
		eventType := event.GetType()
		if !strings.Contains(eventType, "ethereum") && !strings.Contains(eventType, "evm") {
			continue
		}

		// Extract EVM hash from attributes
		attrs := event.GetAttributes()
		if evmHash, ok := attrs[evmtypes.AttributeKeyEthereumTxHash]; ok {
			// Ensure hash has 0x prefix
			hash := strings.TrimSpace(evmHash)
			if !strings.HasPrefix(hash, "0x") {
				hash = "0x" + hash
			}
			global.Logger.Debugf("found EVM hash in events: %s", hash)
			return hash
		}
	}
	return ""
}

// ExtractEVMHashFromLogs extracts EVM transaction hash from transaction logs (JSON format).
//
// Parameters:
//   - logsBytes: Logs data in JSON format
//
// Returns:
//   - string: EVM transaction hash with 0x prefix, or empty string if not found
func ExtractEVMHashFromLogs(logsBytes []byte) string {
	if len(logsBytes) == 0 {
		return ""
	}

	// Try to parse as JSON array of logs
	var logs []struct {
		MsgIndex string `json:"msg_index"`
		Events   []struct {
			Type       string `json:"type"`
			Attributes []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"attributes"`
		} `json:"events"`
	}

	if err := sonic.Unmarshal(logsBytes, &logs); err != nil {
		// Try single log object
		var singleLog struct {
			MsgIndex string `json:"msg_index"`
			Events   []struct {
				Type       string `json:"type"`
				Attributes []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"attributes"`
			} `json:"events"`
		}
		if err := sonic.Unmarshal(logsBytes, &singleLog); err != nil {
			return ""
		}
		logs = []struct {
			MsgIndex string `json:"msg_index"`
			Events   []struct {
				Type       string `json:"type"`
				Attributes []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"attributes"`
			} `json:"events"`
		}{singleLog}
	}

	// Search for EVM hash in logs
	for _, log := range logs {
		for _, event := range log.Events {
			eventType := strings.ToLower(event.Type)
			if strings.Contains(eventType, "ethereum") || strings.Contains(eventType, "evm") {
				for _, attr := range event.Attributes {
					if attr.Key == evmtypes.AttributeKeyEthereumTxHash {
						hash := strings.TrimSpace(attr.Value)
						if !strings.HasPrefix(hash, "0x") {
							hash = "0x" + hash
						}
						global.Logger.Debugf("found EVM hash in logs: %s", hash)
						return hash
					}
				}
			}
		}
	}

	return ""
}

// ExtractEVMHashFromABCIEvents extracts EVM transaction hash from abci.Event array.
// This is the format returned by chainClient.GetTx().
//
// Parameters:
//   - events: Array of abci.Event (from TxResponse.Events)
//
// Returns:
//   - string: EVM transaction hash with 0x prefix, or empty string if not found
func ExtractEVMHashFromABCIEvents(events []abci.Event) string {
	for _, event := range events {
		eventType := strings.ToLower(event.Type)
		if !strings.Contains(eventType, "ethereum") && !strings.Contains(eventType, "evm") {
			continue
		}

		// abci.Event has Attributes as []abci.EventAttribute
		for _, attr := range event.Attributes {
			if attr.Key == evmtypes.AttributeKeyEthereumTxHash {
				hash := strings.TrimSpace(attr.Value)
				if !strings.HasPrefix(hash, "0x") {
					hash = "0x" + hash
				}
				global.Logger.Debugf("found EVM hash in abci events: %s", hash)
				return hash
			}
		}
	}
	return ""
}

// ExtractEVMHash extracts EVM transaction hash from transaction data.
// It checks both Events and Logs fields.
//
// Parameters:
//   - txData: Transaction detail data from explorer
//
// Returns:
//   - string: EVM transaction hash with 0x prefix, or empty string if not found
func ExtractEVMHash(txData *injective_explorer_rpcpb.TxDetailData) string {
	if txData == nil {
		return ""
	}

	// Method 1: Check Events field (protobuf Event format)
	if evmHash := ExtractEVMHashFromEvents(txData.Events); evmHash != "" {
		return evmHash
	}

	// Method 2: Check Logs field (JSON format, may contain abci.Event format)
	if evmHash := ExtractEVMHashFromLogs(txData.Logs); evmHash != "" {
		return evmHash
	}

	return ""
}

// IsEVMTransaction checks if a transaction is an EVM transaction.
//
// Parameters:
//   - txData: Transaction detail data from explorer
//
// Returns:
//   - bool: true if the transaction is an EVM transaction, false otherwise
func IsEVMTransaction(txData *injective_explorer_rpcpb.TxDetailData) bool {
	if txData == nil {
		return false
	}

	// Method 1: Check if EVM hash exists in events or logs
	if evmHash := ExtractEVMHash(txData); evmHash != "" {
		return true
	}

	// Method 2: Check message types
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
