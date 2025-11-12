// Package models provides data models for blocks and transactions.
package models

import (
	"encoding/base64"
	"unicode/utf8"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// Transaction represents a blockchain transaction in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct is designed to store transaction information from the Injective blockchain explorer,
// including transaction hash, block information, gas usage, events, messages, and execution status.
type Transaction struct {
	gorm.Model

	Id string `json:"id" gorm:"column:tx_data_id"` // Id is the transaction data ID

	BlockNumber        uint64   `json:"block_number" gorm:"column:block_number;index:idx_block_tx"`                     // BlockNumber is the block number containing this transaction (indexed for pagination)
	BlockTimestamp     string   `json:"block_timestamp" gorm:"column:block_timestamp"`                                  // BlockTimestamp is the timestamp of the block containing this transaction
	Hash               string   `json:"hash" gorm:"column:hash;index"`                                                  // Hash is the transaction hash (indexed for faster lookups)
	Code               uint32   `json:"code" gorm:"column:code"`                                                        // Code is the transaction execution code (0 = success, non-zero = error)
	Data               string   `json:"data" gorm:"column:data"`                                                        // Data contains the transaction data (stored as string, originally []byte)
	Info               string   `json:"info" gorm:"column:info"`                                                        // Info contains additional transaction information
	GasWanted          int64    `json:"gas_wanted" gorm:"column:gas_wanted"`                                            // GasWanted is the maximum gas requested for the transaction
	GasUsed            int64    `json:"gas_used" gorm:"column:gas_used"`                                                // GasUsed is the actual gas used by the transaction
	GasFee             string   `json:"gas_fee" gorm:"column:gas_fee"`                                                  // GasFee is the gas fee paid for the transaction
	Codespace          string   `json:"codespace" gorm:"column:codespace"`                                              // Codespace is the error codespace if the transaction failed
	Events             []string `json:"events" gorm:"column:events;serializer:json"`                                    // Events is a list of events emitted by the transaction (stored as JSON)
	TxType             string   `json:"tx_type" gorm:"column:tx_type"`                                                  // TxType is the type of the transaction
	Messages           string   `json:"messages" gorm:"column:messages"`                                                // Messages contains the transaction messages (stored as string, originally []byte)
	Signatures         []string `json:"signatures" gorm:"column:signatures;serializer:json"`                            // Signatures is a list of transaction signatures (stored as JSON)
	Memo               string   `json:"memo" gorm:"column:memo"`                                                        // Memo is the memo field of the transaction
	TxNumber           uint64   `json:"tx_number" gorm:"column:tx_number;index:idx_tx_number;index:idx_block_tx"`       // TxNumber is the transaction number (indexed for pagination)
	BlockUnixTimestamp uint64   `json:"block_unix_timestamp" gorm:"column:block_unix_timestamp;index:idx_timestamp_tx"` // BlockUnixTimestamp is the Unix timestamp of the block containing this transaction (indexed for sorting)
	ErrorLog           string   `json:"error_log" gorm:"column:error_log"`                                              // ErrorLog contains error information if the transaction failed
	Logs               string   `json:"logs" gorm:"column:logs"`                                                        // Logs contains execution logs (stored as string, originally []byte)
	ClaimIds           []int64  `json:"claim_ids" gorm:"column:claim_ids;serializer:json"`                              // ClaimIds is a list of claim IDs associated with this transaction (stored as JSONB)
	Payer              string   `json:"payer" gorm:"type:varchar(128);column:payer"`
}

// TransactionFromProto converts a protobuf TxDetailData to a Transaction model.
//
// Parameters:
//   - detail: The protobuf transaction detail data (note: parameter name corrected from "deatil")
//
// Returns:
//   - *Transaction: A new Transaction instance with data from the protobuf message
func TransactionFromProto(detail *injective_explorer_rpcpb.TxDetailData) *Transaction {
	if detail == nil {
		return nil
	}

	// Store events as JSON strings for reversible conversion
	// Use shared utility function to eliminate code duplication
	eventMessages := make([]proto.Message, 0, len(detail.Events))
	for _, event := range detail.Events {
		eventMessages = append(eventMessages, event)
	}
	events, _ := MarshalProtoSliceToJSON(eventMessages)

	// Store signatures as JSON strings for reversible conversion
	// Use shared utility function to eliminate code duplication
	sigMessages := make([]proto.Message, 0, len(detail.Signatures))
	for _, sig := range detail.Signatures {
		sigMessages = append(sigMessages, sig)
	}
	signatures, _ := MarshalProtoSliceToJSON(sigMessages)

	// Store GasFee as JSON string for reversible conversion
	gasFee := ""
	payer := ""
	if detail.GasFee != nil {
		// Use shared utility function to eliminate code duplication
		encoded, err := MarshalProtoToJSON(detail.GasFee)
		if err == nil {
			gasFee = encoded
		} else {
			// Fallback to empty string if marshaling fails
			// In production, you might want to log this error
			gasFee = ""
		}
		payer = detail.GasFee.Payer
	}

	// Convert binary data to base64 to avoid encoding issues
	// Data, Messages, and Logs may contain binary data that cannot be stored as UTF-8 strings
	dataStr := encodeBinaryData(detail.Data)
	messagesStr := encodeBinaryData(detail.Messages)
	logsStr := encodeBinaryData(detail.Logs)

	return &Transaction{
		Id:                 detail.Id,
		BlockNumber:        detail.BlockNumber,
		BlockTimestamp:     detail.BlockTimestamp,
		Hash:               detail.Hash,
		Code:               detail.Code,
		Data:               dataStr,
		Info:               detail.Info,
		GasWanted:          detail.GasWanted,
		GasUsed:            detail.GasUsed,
		GasFee:             gasFee,
		Codespace:          detail.Codespace,
		Events:             events,
		TxType:             detail.TxType,
		Messages:           messagesStr,
		Signatures:         signatures,
		Memo:               detail.Memo,
		TxNumber:           detail.TxNumber,
		BlockUnixTimestamp: detail.BlockUnixTimestamp,
		ErrorLog:           detail.ErrorLog,
		Logs:               logsStr,
		ClaimIds:           detail.ClaimIds,
		Payer:              payer,
	}
}

// encodeBinaryData encodes binary data to base64 if it contains invalid UTF-8 characters.
// This ensures the data can be safely stored in a UTF-8 database column.
func encodeBinaryData(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// Check if data is valid UTF-8
	if !isValidUTF8(data) {
		// Encode as base64 if it contains binary data
		return base64.StdEncoding.EncodeToString(data)
	}

	// If it's valid UTF-8, return as string
	return string(data)
}

// isValidUTF8 checks if the byte slice contains valid UTF-8 encoded text.
func isValidUTF8(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	return utf8.Valid(data)
}

// decodeBinaryData decodes base64-encoded data back to bytes if needed.
func decodeBinaryData(dataStr string) []byte {
	if dataStr == "" {
		return nil
	}

	// Try to decode as base64 first
	if decoded, err := base64.StdEncoding.DecodeString(dataStr); err == nil {
		return decoded
	}

	// If not base64, return as bytes
	return []byte(dataStr)
}

// TransactionToProto converts a Transaction model back to a protobuf TxDetailData.
//
// Parameters:
//   - tx: The Transaction model from database
//
// Returns:
//   - *injective_explorer_rpcpb.TxDetailData: A new TxDetailData instance with data from the Transaction model
func TransactionToProto(tx *Transaction) *injective_explorer_rpcpb.TxDetailData {
	if tx == nil {
		return nil
	}

	// Decode binary data fields
	dataBytes := decodeBinaryData(tx.Data)
	messagesBytes := decodeBinaryData(tx.Messages)
	logsBytes := decodeBinaryData(tx.Logs)

	// Convert events from JSON strings back to protobuf format
	// Use shared utility function to eliminate code duplication
	events := make([]*injective_explorer_rpcpb.Event, 0, len(tx.Events))
	for _, eventStr := range tx.Events {
		if eventStr == "" {
			continue
		}

		var event injective_explorer_rpcpb.Event
		if err := UnmarshalProtoFromJSON(eventStr, &event); err == nil {
			events = append(events, &event)
			continue
		}

		// If JSON unmarshal fails, the string might be in old format
		// from old data. We'll create an empty Event to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
		events = append(events, &injective_explorer_rpcpb.Event{})
	}

	// Convert signatures from JSON strings back to protobuf format
	// Use shared utility function to eliminate code duplication
	signatures := make([]*injective_explorer_rpcpb.Signature, 0, len(tx.Signatures))
	for _, sigStr := range tx.Signatures {
		if sigStr == "" {
			continue
		}

		var sig injective_explorer_rpcpb.Signature
		if err := UnmarshalProtoFromJSON(sigStr, &sig); err == nil {
			signatures = append(signatures, &sig)
			continue
		}

		// If JSON unmarshal fails, the string might be in old format
		// from old data. We'll create an empty Signature to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
		signatures = append(signatures, &injective_explorer_rpcpb.Signature{})
	}

	// Parse GasFee from JSON string back to protobuf format
	// Use shared utility function to eliminate code duplication
	var gasFee *injective_explorer_rpcpb.GasFee
	if tx.GasFee != "" {
		var gasFeeData injective_explorer_rpcpb.GasFee
		if err := UnmarshalProtoFromJSON(tx.GasFee, &gasFeeData); err == nil {
			gasFee = &gasFeeData
		}
		// If JSON unmarshal fails, the string might be in old format
		// from old data. We'll leave gasFee as nil to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
	}

	return &injective_explorer_rpcpb.TxDetailData{
		Id:                 tx.Id,
		BlockNumber:        tx.BlockNumber,
		BlockTimestamp:     tx.BlockTimestamp,
		Hash:               tx.Hash,
		Code:               tx.Code,
		Data:               dataBytes,
		Info:               tx.Info,
		GasWanted:          tx.GasWanted,
		GasUsed:            tx.GasUsed,
		GasFee:             gasFee,
		Codespace:          tx.Codespace,
		Events:             events,
		TxType:             tx.TxType,
		Messages:           messagesBytes,
		Signatures:         signatures,
		Memo:               tx.Memo,
		TxNumber:           tx.TxNumber,
		BlockUnixTimestamp: tx.BlockUnixTimestamp,
		ErrorLog:           tx.ErrorLog,
		Logs:               logsBytes,
		ClaimIds:           tx.ClaimIds,
	}
}
