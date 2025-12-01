// Package models provides data models for blockchain entities.
// This file defines archive models for transaction data archiving.
package models

import (
	"gorm.io/gorm"
)

// TransactionArchive represents archived transaction metadata in the database.
// It stores only essential fields for querying archived transactions.
// The detailed data (Messages, Logs, Data) is stored in TransactionDataArchive.
type TransactionArchive struct {
	gorm.Model

	// Id is the transaction data ID (matches Transaction.Id)
	Id string `json:"id" gorm:"column:tx_data_id;index"`

	// BlockNumber is the block number containing this transaction (indexed for pagination)
	BlockNumber uint64 `json:"block_number" gorm:"column:block_number;index:idx_block_number"`
	// BlockTimestamp is the timestamp of the block containing this transaction
	BlockTimestamp string `json:"block_timestamp" gorm:"column:block_timestamp"`
	// Hash is the transaction hash (indexed for faster lookups)
	Hash string `json:"hash" gorm:"column:hash;index"`
	// Code is the transaction execution code (0 = success, non-zero = error)
	Code uint32 `json:"code" gorm:"column:code"`
	// GasWanted is the maximum gas requested for the transaction
	GasWanted int64 `json:"gas_wanted" gorm:"column:gas_wanted"`
	// GasUsed is the actual gas used by the transaction
	GasUsed int64 `json:"gas_used" gorm:"column:gas_used"`
	// GasFee is the gas fee paid for the transaction (stored as JSON string)
	GasFee string `json:"gas_fee" gorm:"column:gas_fee"`
	// Codespace is the error codespace if the transaction failed
	Codespace string `json:"codespace" gorm:"column:codespace"`
	// Events is a list of events emitted by the transaction (stored as JSON)
	Events []string `json:"events" gorm:"column:events;serializer:json"`
	// TxType is the type of the transaction
	TxType string `json:"tx_type" gorm:"column:tx_type"`
	// Signatures is a list of transaction signatures (stored as JSON)
	Signatures []string `json:"signatures" gorm:"column:signatures;serializer:json"`
	// Memo is the memo field of the transaction
	Memo string `json:"memo" gorm:"column:memo"`
	// TxNumber is the transaction number (indexed for pagination)
	TxNumber uint64 `json:"tx_number" gorm:"column:tx_number;index:idx_tx_number;index:idx_block_tx"`
	// BlockUnixTimestamp is the Unix timestamp of the block containing this transaction (indexed for sorting)
	BlockUnixTimestamp uint64 `json:"block_unix_timestamp" gorm:"column:block_unix_timestamp;index:idx_timestamp_tx"`
	// ClaimIds is a list of claim IDs associated with this transaction (stored as JSON)
	ClaimIds []int64 `json:"claim_ids" gorm:"column:claim_ids;serializer:json"`
	// Payer is the payer address
	Payer string `json:"payer" gorm:"type:varchar(128);column:payer"`
	// ArchivedAt is the timestamp when this transaction was archived
	ArchivedAt int64 `json:"archived_at" gorm:"column:archived_at;index"`
}

// TransactionDataArchive represents archived transaction detailed data.
// It stores the large fields (Data, Messages, Logs) that are moved to archive storage.
type TransactionDataArchive struct {
	// TransactionID is the primary key, referencing TransactionArchive.ID
	TransactionID uint `json:"transaction_id" gorm:"primaryKey;column:transaction_id"`

	// Data contains the transaction data (compressed, stored as LONGBLOB/TEXT)
	Data string `json:"data" gorm:"column:data;type:longtext"`
	// Info contains additional transaction information
	Info string `json:"info" gorm:"column:info"`
	// Messages contains the transaction messages (compressed, stored as LONGBLOB/TEXT)
	Messages string `json:"messages" gorm:"column:messages;type:longtext"`
	// Logs contains execution logs (compressed, stored as LONGBLOB/TEXT)
	Logs string `json:"logs" gorm:"column:logs;type:longtext"`
	// ErrorLog contains error information if the transaction failed
	ErrorLog string `json:"error_log" gorm:"column:error_log"`
}

// TableName specifies the table name for TransactionArchive.
func (TransactionArchive) TableName() string {
	return "transaction_archives"
}

// TableName specifies the table name for TransactionDataArchive.
func (TransactionDataArchive) TableName() string {
	return "transaction_data_archives"
}
