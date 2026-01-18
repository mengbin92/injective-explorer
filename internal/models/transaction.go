// Package models provides data models for blocks and transactions.
package models

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"injective-explorer/provider/storage"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// Transaction represents a blockchain transaction in the database.
// The composite index idx_block_enhance on (block_number, hash, signer, "to", value) is created
// via GORM's AutoMigrate for optimizing the enhanceBlockDTOWithTransactions function.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct is designed to store transaction information from the Injective blockchain explorer,
// including transaction hash, block information, gas usage, events, messages, and execution status.
type Transaction struct {
	gorm.Model

	Id string `json:"id" gorm:"column:tx_data_id"` // Id is the transaction data ID

	BlockNumber          uint64     `json:"block_number" gorm:"column:block_number"`                               // BlockNumber is the block number containing this transaction
	BlockTimestamp       string     `json:"block_timestamp" gorm:"column:block_timestamp"`                         // BlockTimestamp is the timestamp of the block containing this transaction
	Hash                 string     `json:"hash" gorm:"column:hash"`                                               // Hash is the transaction hash
	Code                 uint32     `json:"code" gorm:"column:code"`                                               // Code is the transaction execution code (0 = success, non-zero = error)
	Data                 string     `json:"data" gorm:"column:data"`                                               // Data contains the transaction data (stored as string, originally []byte)
	Info                 string     `json:"info" gorm:"column:info"`                                               // Info contains additional transaction information
	GasWanted            int64      `json:"gas_wanted" gorm:"column:gas_wanted"`                                   // GasWanted is the maximum gas requested for the transaction
	GasUsed              int64      `json:"gas_used" gorm:"column:gas_used"`                                       // GasUsed is the actual gas used by the transaction
	GasFee               string     `json:"gas_fee" gorm:"column:gas_fee"`                                         // GasFee is the gas fee paid for the transaction
	Codespace            string     `json:"codespace" gorm:"column:codespace"`                                     // Codespace is the error codespace if the transaction failed
	Events               []string   `json:"events" gorm:"column:events;serializer:json"`                           // Events is a list of events emitted by the transaction (stored as JSON)
	TxType               string     `json:"tx_type" gorm:"column:tx_type"`                                         // TxType is the type of the transaction
	MessageTypes         []string   `json:"message_types" gorm:"column:message_types;serializer:json"`             // MessageTypes is a list of message type URLs extracted from Messages (e.g., ["/cosmos.bank.v1beta1.MsgSend", "/injective.exchange.v1beta1.MsgDeposit"]), stored as JSON array for efficient filtering
	Messages             string     `json:"messages" gorm:"column:messages"`                                       // Messages contains the transaction messages (stored as string, originally []byte)
	Signatures           []string   `json:"signatures" gorm:"column:signatures;serializer:json"`                   // Signatures is a list of transaction signatures (stored as JSON)
	Memo                 string     `json:"memo" gorm:"column:memo"`                                               // Memo is the memo field of the transaction
	TxNumber             uint64     `json:"tx_number" gorm:"column:tx_number"`                                     // TxNumber is the transaction number
	BlockUnixTimestamp   uint64     `json:"block_unix_timestamp" gorm:"column:block_unix_timestamp"`               // BlockUnixTimestamp is the Unix timestamp of the block containing this transaction
	ErrorLog             string     `json:"error_log" gorm:"column:error_log"`                                     // ErrorLog contains error information if the transaction failed
	Logs                 string     `json:"logs" gorm:"column:logs"`                                               // Logs contains execution logs (stored as string, originally []byte)
	ClaimIds             []int64    `json:"claim_ids" gorm:"column:claim_ids;serializer:json"`                     // ClaimIds is a list of claim IDs associated with this transaction (stored as JSONB)
	Signer               string     `json:"signer" gorm:"type:varchar(128);column:signer"`                         // Signer is the transaction initiator address (first signer)
	To                   string     `json:"to" gorm:"type:varchar(128);column:to"`                                 // To is the recipient address for ethereum transactions (corresponds to 'to' field in ethereum tx)
	Value                string     `json:"value" gorm:"type:varchar(128);column:value"`                           // Value is the transaction value for ethereum transactions (corresponds to 'value' field in ethereum tx)
	Archived             bool       `json:"archived" gorm:"column:archived;default:0"`                             // Archived indicates whether this transaction has been archived
	DataStorageKey       string     `json:"data_storage_key" gorm:"column:data_storage_key"`                       // DataStorageKey is the object storage key for Data field (no index: only used in migration with IS NULL checks)
	MessagesStorageKey   string     `json:"messages_storage_key" gorm:"column:messages_storage_key"`               // MessagesStorageKey is the object storage key for Messages field (no index: only used in migration with IS NULL checks)
	LogsStorageKey       string     `json:"logs_storage_key" gorm:"column:logs_storage_key"`                       // LogsStorageKey is the object storage key for Logs field (no index: only used in migration with IS NULL checks)

	// Composite index for performance optimization
	// This field is not a real column, just used to define the composite index
	_ BlockEnhanceIndex `gorm:"-"` // Ignore this field in CRUD operations
}

// BlockEnhanceIndex is a dummy struct used to define composite index
type BlockEnhanceIndex struct{}

// TransactionFromProto converts a protobuf TxDetailData to a Transaction model.
// Compresses large fields (>1KB) and optionally uploads large fields to object storage.
//
// The storageThreshold parameter is based on compressed data size (actual database storage size).
// If 0 or negative, uses default 3KB (3072 bytes). This threshold balances database performance
// and storage costs (object storage is typically cheaper than database storage for large fields).
//
// Behavior:
//   - Compresses Data, Messages, and Logs fields if they exceed 1KB
//   - Uploads to object storage if enabled and compressed data exceeds storageThreshold
//   - Extracts signer from first signature's address, falls back to GasFee.Payer
//   - Converts addresses to Ethereum format (0x...) for database storage
func TransactionFromProto(detail *injective_explorer_rpcpb.TxDetailData, storageThreshold int) *Transaction {
	return TransactionFromProtoWithPool(detail, storageThreshold, nil)
}

// TransactionFromProtoWithPool converts a protobuf TxDetailData to a Transaction model,
// optionally reusing a Transaction from the object pool to reduce allocations.
//
// Parameters:
//   - detail: The protobuf transaction detail data
//   - storageThreshold: Storage threshold in bytes for compressed data (0 or negative uses default 3KB)
//   - poolTx: Optional Transaction instance from object pool (can be nil)
//
// Behavior is the same as TransactionFromProto, but reuses poolTx if provided to reduce allocations.
func TransactionFromProtoWithPool(detail *injective_explorer_rpcpb.TxDetailData, storageThreshold int, poolTx *Transaction) *Transaction {
	if detail == nil {
		return nil
	}

	// Store events as JSON strings
	eventMessages := make([]proto.Message, 0, len(detail.Events))
	for _, event := range detail.Events {
		eventMessages = append(eventMessages, event)
	}
	events, _ := MarshalProtoSliceToJSON(eventMessages)

	// Store signatures as JSON strings
	sigMessages := make([]proto.Message, 0, len(detail.Signatures))
	for _, sig := range detail.Signatures {
		sigMessages = append(sigMessages, sig)
	}
	signatures, _ := MarshalProtoSliceToJSON(sigMessages)

	// Store GasFee as JSON string
	gasFee := ""
	signer := ""
	if detail.GasFee != nil {
		encoded, err := MarshalProtoToJSON(detail.GasFee)
		if err == nil {
			gasFee = encoded
		} else {
			gasFee = ""
		}
	}

	// Extract signer (transaction initiator) from first signature
	if len(detail.Signatures) > 0 && detail.Signatures[0] != nil {
		if detail.Signatures[0].Address != "" {
			signer = detail.Signatures[0].Address
		}
	}

	// Fallback to GasFee.Payer if no signature address found
	if signer == "" && detail.GasFee != nil {
		signer = detail.GasFee.Payer
	}

	// Extract to and value from transaction
	to, value := extractToAndValueFromProto(detail.Messages, detail.Events, detail.Logs, detail.Signatures, detail.GasFee)

	// Extract message types from Messages for efficient filtering (before compressing)
	messageTypes := extractMessageTypesFromBytes(detail.Messages)
	if len(messageTypes) == 0 && len(detail.Messages) > 0 {
		log.Printf("WARN: failed to extract message types for transaction %s, messages length: %d", detail.Hash, len(detail.Messages))
	}

	// Compress and encode data for storage
	dataStr, err := CompressData(detail.Data)
	if err != nil {
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

	var tx *Transaction
	if poolTx != nil {
		tx = poolTx
		*tx = Transaction{}
	} else {
		tx = &Transaction{}
	}

	// Populate transaction fields
	tx.Id = detail.Id
	tx.BlockNumber = detail.BlockNumber
	tx.BlockTimestamp = detail.BlockTimestamp
	tx.Hash = detail.Hash
	tx.Code = detail.Code
	tx.Data = dataStr
	tx.Info = detail.Info
	tx.GasWanted = detail.GasWanted
	tx.GasUsed = detail.GasUsed
	tx.GasFee = gasFee
	tx.Codespace = detail.Codespace
	tx.Events = events
	tx.TxType = detail.TxType
	tx.MessageTypes = messageTypes
	tx.Messages = messagesStr
	tx.Signatures = signatures
	tx.Memo = detail.Memo
	tx.TxNumber = detail.TxNumber
	tx.BlockUnixTimestamp = detail.BlockUnixTimestamp
	tx.ErrorLog = detail.ErrorLog
	tx.Logs = logsStr
	tx.ClaimIds = detail.ClaimIds
	// Convert addresses to Ethereum format for database storage
	tx.Signer = ConvertAddressToEthForStorage(signer)
	tx.To = ConvertAddressToEthForStorage(to)
	tx.Value = value

	// Upload large fields to object storage if enabled
	if storage.IsEnabled() {
		// Calculate threshold (use provided value or default 3KB)
		threshold := storageThreshold
		if threshold <= 0 {
			threshold = 3 * 1024
		}

		storageClient := storage.Get()
		ctx := context.Background()

		// Process Data field
		if storageKey := processLargeField(ctx, storageClient, "data", detail.Hash, detail.Data, dataStr, threshold); storageKey != "" {
			tx.DataStorageKey = storageKey
			tx.Data = ""
		}

		// Process Messages field
		if storageKey := processLargeField(ctx, storageClient, "messages", detail.Hash, detail.Messages, messagesStr, threshold); storageKey != "" {
			tx.MessagesStorageKey = storageKey
			tx.Messages = ""
		}

		// Process Logs field
		if storageKey := processLargeField(ctx, storageClient, "logs", detail.Hash, detail.Logs, logsStr, threshold); storageKey != "" {
			tx.LogsStorageKey = storageKey
			tx.Logs = ""
		}
	}

	return tx
}

// processLargeField processes a large field by compressing and optionally uploading it to object storage.
// This function encapsulates the repeated logic for handling Data, Messages, and Logs fields.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - storageClient: Object storage client
//   - fieldName: Name of the field (e.g., "data", "messages", "logs") for building storage key and logging
//   - txHash: Transaction hash for building storage key
//   - originalData: Original binary data to compress and upload
//   - compressedStr: Compressed string representation (for checking size against threshold)
//   - threshold: Storage threshold in bytes (compressed data size threshold)
//
// Returns:
//   - string: Storage key if upload was successful, empty string otherwise
//
// Behavior:
//   - Checks if compressed data size exceeds threshold
//   - Compresses original data for object storage (binary format, no base64)
//   - Uploads to object storage if compression succeeds
//   - Logs warnings for compression or upload failures
//   - Returns storage key on success, empty string on failure (fallback to database storage)
func processLargeField(ctx context.Context, storageClient storage.Storage, fieldName, txHash string, originalData []byte, compressedStr string, threshold int) string {
	// Only process if compressed size exceeds threshold
	if len(compressedStr) <= threshold {
		return ""
	}

	// Build storage key
	key := storage.BuildStorageKey(fieldName, txHash)

	// Compress original data for object storage (binary format, no base64)
	// CompressDataToBytes returns compressed data if beneficial, otherwise original data
	dataToUpload, _, compressErr := CompressDataToBytes(originalData)
	if compressErr != nil {
		// Log compression error - this is critical as it means large data will remain in database
		log.Printf("WARN: failed to compress %s field for transaction %s: %v. Data will remain in database, which may cause database bloat.", fieldName, txHash, compressErr)
		return ""
	}

	// Upload to object storage
	if err := storageClient.PutObject(ctx, key, dataToUpload); err != nil {
		// Log upload error - fallback to database storage
		log.Printf("WARN: failed to upload %s field to object storage for transaction %s: %v. Falling back to database storage.", fieldName, txHash, err)
		return ""
	}

	// Upload successful, return storage key
	return key
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
// Loads data from object storage if storage keys exist, otherwise uses database fields.
// Automatically decompresses compressed data and handles backward compatibility.
func TransactionToProto(tx *Transaction) *injective_explorer_rpcpb.TxDetailData {
	if tx == nil {
		return nil
	}

	storageClient := storage.Get()
	ctx := context.Background()

	// Load data from object storage if storage key exists, otherwise use database field
	var dataBytes []byte
	if tx.DataStorageKey != "" {
		if storageData, err := storageClient.GetObject(ctx, tx.DataStorageKey); err == nil {
			var decompressErr error
			dataBytes, decompressErr = DecompressDataFromBytes(storageData)
			if decompressErr != nil {
				// Decompression failed, assume uncompressed (backward compatibility)
				dataBytes = storageData
			}
		} else {
			// Object storage failed, fall back to database field
			dataBytes, _ = DecompressData(tx.Data)
			if len(dataBytes) == 0 {
				dataBytes = decodeBinaryData(tx.Data)
			}
		}
	} else {
		// Decompress data from database field
		var err error
		dataBytes, err = DecompressData(tx.Data)
		if err != nil {
			// Decompression failed, try base64 decode (backward compatibility)
			dataBytes = decodeBinaryData(tx.Data)
		}
	}

	var messagesBytes []byte
	if tx.MessagesStorageKey != "" {
		if storageMessages, err := storageClient.GetObject(ctx, tx.MessagesStorageKey); err == nil {
			var decompressErr error
			messagesBytes, decompressErr = DecompressDataFromBytes(storageMessages)
			if decompressErr != nil {
				// Decompression failed, assume uncompressed (backward compatibility)
				messagesBytes = storageMessages
			}
		} else {
			// Object storage failed, fall back to database field
			messagesBytes, _ = DecompressData(tx.Messages)
			if len(messagesBytes) == 0 {
				messagesBytes = decodeBinaryData(tx.Messages)
			}
		}
	} else {
		var err error
		messagesBytes, err = DecompressData(tx.Messages)
		if err != nil {
			messagesBytes = decodeBinaryData(tx.Messages)
		}
	}

	var logsBytes []byte
	if tx.LogsStorageKey != "" {
		if storageLogs, err := storageClient.GetObject(ctx, tx.LogsStorageKey); err == nil {
			var decompressErr error
			logsBytes, decompressErr = DecompressDataFromBytes(storageLogs)
			if decompressErr != nil {
				// Decompression failed, assume uncompressed (backward compatibility)
				logsBytes = storageLogs
			}
		} else {
			// Object storage failed, fall back to database field
			logsBytes, _ = DecompressData(tx.Logs)
			if len(logsBytes) == 0 {
				logsBytes = decodeBinaryData(tx.Logs)
			}
		}
	} else {
		var err error
		logsBytes, err = DecompressData(tx.Logs)
		if err != nil {
			logsBytes = decodeBinaryData(tx.Logs)
		}
	}

	// Convert events from JSON strings back to protobuf format
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

		// JSON unmarshal failed, create empty Event for backward compatibility
		events = append(events, &injective_explorer_rpcpb.Event{})
	}

	// Convert signatures from JSON strings back to protobuf format
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

// ExtractMessageTypes extracts all message types (TypeUrl) from a Transaction's Messages field.
// Decompresses and parses the Messages field to extract TypeUrl from each message.
func ExtractMessageTypes(tx *Transaction) []string {
	if tx == nil {
		return nil
	}

	// Get messages bytes
	var messagesBytes []byte
	storageClient := storage.Get()
	ctx := context.Background()

	if tx.MessagesStorageKey != "" {
		if storageMessages, err := storageClient.GetObject(ctx, tx.MessagesStorageKey); err == nil {
			var decompressErr error
			messagesBytes, decompressErr = DecompressDataFromBytes(storageMessages)
			if decompressErr != nil {
				messagesBytes = storageMessages
			}
		} else {
			messagesBytes, _ = DecompressData(tx.Messages)
			if len(messagesBytes) == 0 {
				messagesBytes = decodeBinaryData(tx.Messages)
			}
		}
	} else {
		var err error
		messagesBytes, err = DecompressData(tx.Messages)
		if err != nil {
			messagesBytes = decodeBinaryData(tx.Messages)
		}
	}

	if len(messagesBytes) == 0 {
		return nil
	}

	// Extract message types (handles TxRaw, TxBody, and JSON formats)
	return extractMessageTypesFromBytes(messagesBytes)
}

// extractMessageTypesFromBytes extracts message types from Messages bytes.
// Supports multiple formats:
//   - Protobuf-encoded TxRaw (contains BodyBytes which is TxBody)
//   - Protobuf-encoded TxBody directly
//   - JSON format (array of message objects with "type" field)
func extractMessageTypesFromBytes(messagesBytes []byte) []string {
	if len(messagesBytes) == 0 {
		return nil
	}

	// Try parsing as TxRaw first (most common format from explorer API)
	var txRaw cosmostxpb.TxRaw
	if err := txRaw.Unmarshal(messagesBytes); err == nil && len(txRaw.BodyBytes) > 0 {
		// Successfully parsed as TxRaw, parse BodyBytes as TxBody
		var txBody cosmostxpb.TxBody
		if err := txBody.Unmarshal(txRaw.BodyBytes); err == nil {
			if len(txBody.Messages) == 0 {
				return nil
			}

			messageTypes := make([]string, 0, len(txBody.Messages))
			for _, msgAny := range txBody.Messages {
				if msgAny != nil && msgAny.TypeUrl != "" {
					messageTypes = append(messageTypes, msgAny.TypeUrl)
				}
			}

			// Log warning if no message types extracted
			if len(messageTypes) == 0 && len(txBody.Messages) > 0 {
				log.Printf("WARN: extracted 0 message types from %d messages in TxRaw->TxBody, all messages have empty TypeUrl", len(txBody.Messages))
			}

			return messageTypes
		}
	}

	// Try parsing as TxBody directly (fallback)
	txBody := &cosmostxpb.TxBody{}
	if err := txBody.Unmarshal(messagesBytes); err == nil {
		// Successfully parsed as TxBody
		if len(txBody.Messages) == 0 {
			return nil
		}

		messageTypes := make([]string, 0, len(txBody.Messages))
		for _, msgAny := range txBody.Messages {
			if msgAny != nil && msgAny.TypeUrl != "" {
				messageTypes = append(messageTypes, msgAny.TypeUrl)
			}
		}

		// Log warning if no message types extracted
		if len(messageTypes) == 0 && len(txBody.Messages) > 0 {
			log.Printf("WARN: extracted 0 message types from %d protobuf messages, all messages have empty TypeUrl", len(txBody.Messages))
		}

		return messageTypes
	}

	// Protobuf parsing failed, try JSON format
	if len(messagesBytes) > 0 && (messagesBytes[0] == '[' || messagesBytes[0] == '{') {
		var jsonMessages []map[string]interface{}
		if err := sonic.Unmarshal(messagesBytes, &jsonMessages); err == nil {
			// Successfully parsed as JSON array
			messageTypes := make([]string, 0, len(jsonMessages))
			for _, msg := range jsonMessages {
				if msgType, ok := msg["type"].(string); ok && msgType != "" {
					messageTypes = append(messageTypes, msgType)
				}
			}
			if len(messageTypes) > 0 {
				return messageTypes
			}
		} else {
			// Try parsing as single JSON object
			var singleMsg map[string]interface{}
			if err := sonic.Unmarshal(messagesBytes, &singleMsg); err == nil {
				if msgType, ok := singleMsg["type"].(string); ok && msgType != "" {
					return []string{msgType}
				}
			}
		}
	}

	// Both protobuf and JSON parsing failed
	log.Printf("WARN: failed to extract message types: protobuf unmarshal error, messages length: %d, first bytes: %v", len(messagesBytes), getFirstBytes(messagesBytes, 10))
	return nil
}

// getFirstBytes returns the first n bytes of a byte slice for debugging
func getFirstBytes(data []byte, n int) []byte {
	if len(data) < n {
		return data
	}
	return data[:n]
}

// TransactionFromGetTxResponse converts a chainClient.GetTx response to a Transaction model.
// Converts GetTxResponse to TxDetailData format, then uses TransactionFromProto.
func TransactionFromGetTxResponse(getTxResp *cosmostxpb.GetTxResponse, storageThreshold int) *Transaction {
	if getTxResp == nil {
		return nil
	}

	// Convert GetTxResponse to TxDetailData
	txDetailData := ConvertGetTxResponseToTxDetailData(getTxResp)
	if txDetailData == nil {
		return nil
	}

	// Use existing TransactionFromProto function
	return TransactionFromProto(txDetailData, storageThreshold)
}

// extractSignerFromEvents attempts to extract signer address from transaction events.
// Common event types that may contain signer information include:
// - "message" event with "sender" attribute
// - "tx" event with "acc_seq" or "signer" attribute
func extractSignerFromEvents(events []*injective_explorer_rpcpb.Event) string {
	if len(events) == 0 {
		return ""
	}

	// Look for "message" event with "sender" attribute
	for _, event := range events {
		if event == nil {
			continue
		}
		if event.Type == "message" {
			if attrs := event.GetAttributes(); attrs != nil {
				if sender, ok := attrs["sender"]; ok && sender != "" {
					return sender
				}
			}
		}
		// Also check for "tx" event
		if event.Type == "tx" {
			if attrs := event.GetAttributes(); attrs != nil {
				if signer, ok := attrs["signer"]; ok && signer != "" {
					return signer
				}
			}
		}
	}

	return ""
}

// ConvertGetTxResponseToTxDetailData converts *tx.GetTxResponse to *injective_explorer_rpcpb.TxDetailData.
// Extracts data from GetTxResponse.Tx and GetTxResponse.TxResponse fields.
func ConvertGetTxResponseToTxDetailData(getTxResp *cosmostxpb.GetTxResponse) *injective_explorer_rpcpb.TxDetailData {
	if getTxResp == nil {
		return nil
	}

	txResp := getTxResp.TxResponse
	if txResp == nil {
		return nil
	}

	// Extract data from Tx field
	var dataBytes []byte
	var messagesBytes []byte
	var logsBytes []byte
	var signatures []*injective_explorer_rpcpb.Signature
	var events []*injective_explorer_rpcpb.Event

	if getTxResp.Tx != nil {
		// Marshal Tx to bytes for Data field
		if txBytes, err := getTxResp.Tx.Marshal(); err == nil {
			dataBytes = txBytes
		}

		// Extract messages from Tx.Body
		if getTxResp.Tx.Body != nil {
			if bodyBytes, err := getTxResp.Tx.Body.Marshal(); err == nil {
				messagesBytes = bodyBytes
			}
		}

		// Extract signatures from AuthInfo.SignerInfo and Signatures
		if getTxResp.Tx.AuthInfo != nil && len(getTxResp.Tx.AuthInfo.SignerInfos) > 0 {
			signatures = make([]*injective_explorer_rpcpb.Signature, 0, len(getTxResp.Tx.AuthInfo.SignerInfos))
			for i, signerInfo := range getTxResp.Tx.AuthInfo.SignerInfos {
				sigData := &injective_explorer_rpcpb.Signature{}

				// Extract public key (if available)
				if signerInfo.PublicKey != nil && len(signerInfo.PublicKey.Value) > 0 {
					sigData.Pubkey = base64.StdEncoding.EncodeToString(signerInfo.PublicKey.Value)
				}

				// Extract sequence number
				sigData.Sequence = signerInfo.Sequence

				// Extract signature data from Signatures slice (if available)
				if i < len(getTxResp.Tx.Signatures) && len(getTxResp.Tx.Signatures[i]) > 0 {
					sigData.Signature = base64.StdEncoding.EncodeToString(getTxResp.Tx.Signatures[i])
				}

				// Address will be extracted from events as fallback
				signatures = append(signatures, sigData)
			}
		}
	}

	// Extract events from TxResponse
	if len(txResp.Events) > 0 {
		events = make([]*injective_explorer_rpcpb.Event, 0, len(txResp.Events))
		for _, event := range txResp.Events {
			attrs := make(map[string]string)
			for _, attr := range event.Attributes {
				attrs[string(attr.Key)] = string(attr.Value)
			}
			events = append(events, &injective_explorer_rpcpb.Event{
				Type:       event.Type,
				Attributes: attrs,
			})
		}
	}

	// Extract signer address from events as fallback
	signerFromEvents := extractSignerFromEvents(events)

	// Update signatures with address from events if not already set
	if signerFromEvents != "" && len(signatures) > 0 {
		for _, sig := range signatures {
			if sig != nil && sig.Address == "" {
				sig.Address = signerFromEvents
				break // Only set the first signature's address
			}
		}
	}

	// Extract logs from TxResponse
	if len(txResp.Logs) > 0 {
		if logsJSON, err := sonic.Marshal(txResp.Logs); err == nil {
			logsBytes = logsJSON
		}
	}

	// Extract gas fee from TxResponse
	var gasFee *injective_explorer_rpcpb.GasFee
	if getTxResp.Tx != nil && getTxResp.Tx.AuthInfo != nil && getTxResp.Tx.AuthInfo.Fee != nil {
		fee := getTxResp.Tx.AuthInfo.Fee
		amounts := make([]*injective_explorer_rpcpb.CosmosCoin, 0, len(fee.Amount))
		for _, amt := range fee.Amount {
			amounts = append(amounts, &injective_explorer_rpcpb.CosmosCoin{
				Denom:  amt.Denom,
				Amount: amt.Amount.String(),
			})
		}
		gasFee = &injective_explorer_rpcpb.GasFee{
			Amount:   amounts,
			GasLimit: fee.GasLimit,
			Payer:    "",
		}
	}

	// Extract block timestamp from TxResponse
	var blockTimestamp string
	var blockUnixTimestamp uint64
	if txResp.Timestamp != "" {
		blockTimestamp = txResp.Timestamp
		// Parse timestamp to get unix timestamp
		if t, err := time.Parse(time.RFC3339, txResp.Timestamp); err == nil {
			blockUnixTimestamp = uint64(t.Unix())
		}
	}

	// Build TxDetailData
	return &injective_explorer_rpcpb.TxDetailData{
		Id:                 "",
		BlockNumber:        uint64(txResp.Height),
		BlockTimestamp:     blockTimestamp,
		Hash:               fmt.Sprintf("0x%s", strings.ToLower(txResp.TxHash)),
		Code:               txResp.Code,
		Data:               dataBytes,
		Info:               txResp.Info,
		GasWanted:          int64(txResp.GasWanted),
		GasUsed:            int64(txResp.GasUsed),
		GasFee:             gasFee,
		Codespace:          txResp.Codespace,
		Events:             events,
		TxType:             "",
		Messages:           messagesBytes,
		Signatures:         signatures,
		Memo:               getTxResp.Tx.Body.Memo,
		TxNumber:           0,
		BlockUnixTimestamp: blockUnixTimestamp,
		ErrorLog:           txResp.RawLog,
		Logs:               logsBytes,
		ClaimIds:           nil,
	}
}
