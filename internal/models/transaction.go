// Package models provides data models for blocks and transactions.
package models

import (
	"context"
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"injective-explorer/provider/storage"

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
	Archived           bool     `json:"archived" gorm:"column:archived;default:0;index"`               // Archived indicates whether this transaction has been archived
	DataStorageKey     string   `json:"data_storage_key" gorm:"column:data_storage_key;index"`         // DataStorageKey is the object storage key for Data field
	MessagesStorageKey string   `json:"messages_storage_key" gorm:"column:messages_storage_key;index"` // MessagesStorageKey is the object storage key for Messages field
	LogsStorageKey     string   `json:"logs_storage_key" gorm:"column:logs_storage_key;index"`         // LogsStorageKey is the object storage key for Logs field
}

// TransactionFromProto converts a protobuf TxDetailData to a Transaction model.
// It compresses large fields (>1KB) and optionally uploads very large fields (>10KB) to object storage.
//
// Parameters:
//   - detail: The protobuf transaction detail data
//
// Returns:
//   - *Transaction: A new Transaction instance with data from the protobuf message
//
// Behavior:
//   - Compresses Data, Messages, and Logs fields if they exceed 1KB
//   - Uploads to object storage if enabled and data exceeds 10KB
//   - Sets storage keys and clears database fields after successful upload
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

	// Compress binary data directly, then encode to base64 for safe storage
	// This approach provides better compression ratio than compressing base64-encoded strings
	// The CompressData function handles:
	// - Compression of large data (>1KB)
	// - Base64 encoding of compressed binary data for safe database storage
	// - Base64 encoding of small binary data
	// - Plain text for small text data
	dataStr, err := CompressData(detail.Data)
	if err != nil {
		// If compression fails, fallback to base64 encoding for safety
		dataStr = encodeBinaryData(detail.Data)
	}

	messagesStr, err := CompressData(detail.Messages)
	if err != nil {
		messagesStr = encodeBinaryData(detail.Messages)
	}

	logsStr, err := CompressData(detail.Logs)
	if err != nil {
		logsStr = encodeBinaryData(detail.Logs)
	}

	// Try to upload large fields to object storage
	// Threshold: 10KB - only upload if data is large enough to benefit from object storage
	const storageThreshold = 10 * 1024
	storageClient := storage.Get()

	tx := &Transaction{
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

	// Upload to object storage if enabled and data is large enough
	ctx := context.Background()
	if len(detail.Data) > storageThreshold {
		key := buildStorageKey("data", detail.Hash)
		if err := storageClient.PutObject(ctx, key, detail.Data); err == nil {
			tx.DataStorageKey = key
			tx.Data = "" // Clear data field after successful upload
		}
	}

	if len(detail.Messages) > storageThreshold {
		key := buildStorageKey("messages", detail.Hash)
		if err := storageClient.PutObject(ctx, key, detail.Messages); err == nil {
			tx.MessagesStorageKey = key
			tx.Messages = "" // Clear messages field after successful upload
		}
	}

	if len(detail.Logs) > storageThreshold {
		key := buildStorageKey("logs", detail.Hash)
		if err := storageClient.PutObject(ctx, key, detail.Logs); err == nil {
			tx.LogsStorageKey = key
			tx.Logs = "" // Clear logs field after successful upload
		}
	}

	return tx
}

// encodeBinaryData encodes binary data to base64 if it contains invalid UTF-8 characters.
// This ensures the data can be safely stored in a UTF-8 database column.
//
// Parameters:
//   - data: Binary data to encode
//
// Returns:
//   - string: Base64-encoded string if data contains invalid UTF-8, otherwise plain string
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
//
// Parameters:
//   - data: Byte slice to check
//
// Returns:
//   - bool: True if data is valid UTF-8, false otherwise
func isValidUTF8(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	return utf8.Valid(data)
}

// decodeBinaryData decodes base64-encoded data back to bytes if needed.
// If the string is not base64-encoded, returns it as bytes.
//
// Parameters:
//   - dataStr: String that may be base64-encoded or plain text
//
// Returns:
//   - []byte: Decoded bytes, or original string as bytes if not base64
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
// It loads data from object storage if storage keys exist, otherwise uses database fields.
// Automatically decompresses compressed data and handles backward compatibility.
//
// Parameters:
//   - tx: The Transaction model from database
//
// Returns:
//   - *injective_explorer_rpcpb.TxDetailData: A new TxDetailData instance with data from the Transaction model
//
// Behavior:
//   - First tries to load from object storage if storage keys exist
//   - Falls back to database fields if object storage fails or keys are empty
//   - Automatically decompresses compressed data (supports both compressed and uncompressed formats)
//   - Handles backward compatibility with old data formats
func TransactionToProto(tx *Transaction) *injective_explorer_rpcpb.TxDetailData {
	if tx == nil {
		return nil
	}

	storageClient := storage.Get()
	ctx := context.Background()

	// Load data from object storage if storage key exists, otherwise use database field
	var dataBytes []byte
	var err error
	if tx.DataStorageKey != "" {
		if data, err := storageClient.GetObject(ctx, tx.DataStorageKey); err == nil {
			dataBytes = data
		} else {
			// Fallback to database field if object storage fails
			dataBytes, _ = DecompressData(tx.Data)
			if len(dataBytes) == 0 {
				dataBytes = decodeBinaryData(tx.Data)
			}
		}
	} else {
		// Decompress and decode binary data fields
		// The DecompressData function handles both compressed (with prefix) and uncompressed data
		// For uncompressed data, it may be base64 encoded (old format) or plain text
		dataBytes, err = DecompressData(tx.Data)
		if err != nil {
			// If decompression fails, try to decode as base64 (backward compatibility with old format)
			dataBytes = decodeBinaryData(tx.Data)
		}
	}

	var messagesBytes []byte
	if tx.MessagesStorageKey != "" {
		if messages, err := storageClient.GetObject(ctx, tx.MessagesStorageKey); err == nil {
			messagesBytes = messages
		} else {
			// Fallback to database field if object storage fails
			messagesBytes, _ = DecompressData(tx.Messages)
			if len(messagesBytes) == 0 {
				messagesBytes = decodeBinaryData(tx.Messages)
			}
		}
	} else {
		messagesBytes, err = DecompressData(tx.Messages)
		if err != nil {
			messagesBytes = decodeBinaryData(tx.Messages)
		}
	}

	var logsBytes []byte
	if tx.LogsStorageKey != "" {
		if logs, err := storageClient.GetObject(ctx, tx.LogsStorageKey); err == nil {
			logsBytes = logs
		} else {
			// Fallback to database field if object storage fails
			logsBytes, _ = DecompressData(tx.Logs)
			if len(logsBytes) == 0 {
				logsBytes = decodeBinaryData(tx.Logs)
			}
		}
	} else {
		logsBytes, err = DecompressData(tx.Logs)
		if err != nil {
			logsBytes = decodeBinaryData(tx.Logs)
		}
	}

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

// buildStorageKey generates a storage key for object storage.
// Format: {type}/{hash[:8]}/{hash} or {type}/{hash} if hash is too short.
// Removes "0x" prefix from hash if present.
//
// Parameters:
//   - dataType: Type of data (e.g., "data", "messages", "logs")
//   - hash: Transaction hash (with or without "0x" prefix)
//
// Returns:
//   - string: Storage key for the object
func buildStorageKey(dataType, hash string) string {
	// Remove "0x" prefix if present
	if len(hash) > 2 && hash[:2] == "0x" {
		hash = hash[2:]
	}
	// Use first 8 characters for directory structure
	if len(hash) > 8 {
		return fmt.Sprintf("%s/%s/%s", dataType, hash[:8], hash)
	}
	return fmt.Sprintf("%s/%s", dataType, hash)
}
