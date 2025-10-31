// Package models provides data models for blocks and transactions.
package models

import (
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"gorm.io/gorm"
)

// Transaction represents a blockchain transaction in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct is designed to store transaction information from the Injective blockchain explorer,
// including transaction hash, block information, gas usage, events, messages, and execution status.
type Transaction struct {
	gorm.Model

	// Id is the transaction data ID
	Id string `json:"id" gorm:"column:tx_data_id"`
	// BlockNumber is the block number containing this transaction
	BlockNumber uint64 `json:"block_number" gorm:"column:block_number"`
	// BlockTimestamp is the timestamp of the block containing this transaction
	BlockTimestamp string `json:"block_timestamp" gorm:"column:block_timestamp"`
	// Hash is the transaction hash (indexed for faster lookups)
	Hash string `json:"hash" gorm:"column:hash;index"`
	// Code is the transaction execution code (0 = success, non-zero = error)
	Code uint32 `json:"code" gorm:"column:code"`
	// Data contains the transaction data (stored as string, originally []byte)
	Data string `json:"data" gorm:"column:data"`
	// Info contains additional transaction information
	Info string `json:"info" gorm:"column:info"`
	// GasWanted is the maximum gas requested for the transaction
	GasWanted int64 `json:"gas_wanted" gorm:"column:gas_wanted"`
	// GasUsed is the actual gas used by the transaction
	GasUsed int64 `json:"gas_used" gorm:"column:gas_used"`
	// GasFee is the gas fee paid for the transaction
	GasFee string `json:"gas_fee" gorm:"column:gas_fee"`
	// Codespace is the error codespace if the transaction failed
	Codespace string `json:"codespace" gorm:"column:codespace"`
	// Events is a list of events emitted by the transaction
	Events []string `json:"events" gorm:"column:events"`
	// TxType is the type of the transaction
	TxType string `json:"tx_type" gorm:"column:tx_type"`
	// Messages contains the transaction messages (stored as string, originally []byte)
	Messages string `json:"messages" gorm:"column:messages"`
	// Signatures is a list of transaction signatures
	Signatures []string `json:"signatures" gorm:"column:signatures"`
	// Memo is the memo field of the transaction
	Memo string `json:"memo" gorm:"column:memo"`
	// TxNumber is the transaction number
	TxNumber uint64 `json:"tx_number" gorm:"column:tx_number"`
	// BlockUnixTimestamp is the Unix timestamp of the block containing this transaction
	BlockUnixTimestamp uint64 `json:"block_unix_timestamp" gorm:"column:block_unix_timestamp"`
	// ErrorLog contains error information if the transaction failed
	ErrorLog string `json:"error_log" gorm:"column:error_log"`
	// Logs contains execution logs (stored as string, originally []byte)
	Logs string `json:"logs" gorm:"column:logs"`
	// ClaimIds is a list of claim IDs associated with this transaction (stored as JSONB)
	ClaimIds []int64 `json:"claim_ids" gorm:"column:claim_ids;type:jsonb"`
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

	// Convert events from protobuf format to string slice
	events := make([]string, 0, len(detail.Events))
	for _, event := range detail.Events {
		events = append(events, event.String())
	}

	// Convert signatures from protobuf format to string slice
	signatures := make([]string, 0, len(detail.Signatures))
	for _, sig := range detail.Signatures {
		signatures = append(signatures, sig.String())
	}

	return &Transaction{
		Id:                 detail.Id,
		BlockNumber:        detail.BlockNumber,
		BlockTimestamp:     detail.BlockTimestamp,
		Hash:               detail.Hash,
		Code:               detail.Code,
		Data:               string(detail.Data),
		Info:               detail.Info,
		GasWanted:          detail.GasWanted,
		GasUsed:            detail.GasUsed,
		GasFee:             detail.GasFee.String(),
		Codespace:          detail.Codespace,
		Events:             events,
		TxType:             detail.TxType,
		Messages:           string(detail.Messages),
		Signatures:         signatures,
		Memo:               detail.Memo,
		TxNumber:           detail.TxNumber,
		BlockUnixTimestamp: detail.BlockUnixTimestamp,
		ErrorLog:           detail.ErrorLog,
		Logs:               string(detail.Logs),
		ClaimIds:           detail.ClaimIds,
	}
}
