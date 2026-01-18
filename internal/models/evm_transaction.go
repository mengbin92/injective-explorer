// Package models provides data models for EVM transactions.
package models

import (
	"time"

	"gorm.io/gorm"
)

// EVMTransaction represents an EVM transaction in the database.
// Stores EVM-specific information separately from the main Transaction table
// to avoid affecting signatures and signer fields during updates.
//
// The relationship is: one Transaction can have one EVMTransaction (one-to-one)
// linked by transaction_hash (cosmos hash).
type EVMTransaction struct {
	gorm.Model

	// TransactionHash is the Cosmos transaction hash (foreign key to transactions.hash)
	TransactionHash string `json:"transaction_hash" gorm:"type:varchar(128);column:transaction_hash;uniqueIndex;not null"`

	// EVMHash is the EVM transaction hash
	EVMHash string `json:"evm_hash" gorm:"type:varchar(128);column:evm_hash;index;not null"`

	// EVMFromAddress is the from address for EVM transactions
	EVMFromAddress string `json:"evm_from_address" gorm:"type:varchar(128);column:evm_from_address;index"`

	// EVMToAddress is the to address for EVM transactions
	EVMToAddress string `json:"evm_to_address" gorm:"type:varchar(128);column:evm_to_address;index"`

	// EVMDetailsStorageKey is the object storage key for EVM transaction details (format: evm_details/{hash[:8]}/{hash})
	EVMDetailsStorageKey string `json:"evm_details_storage_key" gorm:"column:evm_details_storage_key"`

	// EVMDetailsSynced indicates whether EVM transaction details have been synced
	EVMDetailsSynced bool `json:"evm_details_synced" gorm:"column:evm_details_synced;default:0;index"`

	// EVMDetailsSyncError contains error information if EVM details sync failed
	EVMDetailsSyncError string `json:"evm_details_sync_error" gorm:"type:text;column:evm_details_sync_error"`

	// EVMDetailsSyncAt is the timestamp when EVM details were synced
	EVMDetailsSyncAt *time.Time `json:"evm_details_sync_at" gorm:"column:evm_details_sync_at"`

	// RawDetailsJSON contains EVM transaction details JSON
	// - When object storage is enabled and data is large (>3KB): stores raw RPC response JSON if parsing fails
	// - When object storage is not enabled: stores parsed transaction details JSON
	// Format varies: Parsed (chain.TransactionDetails) or Raw (fallback with transaction and receipt)
	RawDetailsJSON string `json:"raw_details_json" gorm:"type:longtext;column:raw_details_json"`
}

// TableName returns the table name for EVMTransaction model.
func (EVMTransaction) TableName() string {
	return "evm_transactions"
}
