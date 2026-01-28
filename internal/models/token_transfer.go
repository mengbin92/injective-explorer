// Package models provides data models for token transfers.
package models

import (
	"time"

	"gorm.io/gorm"
)

// TokenTransfer represents a token transfer event in the database.
// Stores ERC-20/ERC-721 Transfer events parsed from EVM transaction logs.
type TokenTransfer struct {
	gorm.Model

	// Transaction info
	TransactionHash string `json:"transaction_hash" gorm:"column:transaction_hash;size:128;not null;index"` // Cosmos transaction hash
	EVMHash         string `json:"evm_hash" gorm:"column:evm_hash;size:128;index"`                           // EVM transaction hash

	// Transfer details
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null;index"` // Token contract address
	FromAddress     string `json:"from_address" gorm:"column:from_address;size:128;not null;index"`         // Sender address
	ToAddress       string `json:"to_address" gorm:"column:to_address;size:128;not null;index"`             // Receiver address
	Amount          string `json:"amount" gorm:"column:amount;size:128;not null"`                           // Transfer amount (raw units)
	Method          string `json:"method" gorm:"column:method;size:50;default:'Transfer'"`                    // Transfer type: Transfer, Mint, Burn

	// Block info
	BlockNumber     uint64 `json:"block_number" gorm:"column:block_number;index"`       // Block height
	BlockTimestamp  uint64 `json:"block_timestamp" gorm:"column:block_timestamp;index"` // Block Unix timestamp
	LogIndex        uint32 `json:"log_index" gorm:"column:log_index"`                   // Log index in transaction

	// Chain info
	ChainID string `json:"chain_id" gorm:"column:chain_id;size:50;default:'injective-1'"` // Chain ID
}

// TableName returns the table name for TokenTransfer model.
func (TokenTransfer) TableName() string {
	return "token_transfers"
}

// IsMint returns true if this is a mint operation (from address is zero address)
func (t *TokenTransfer) IsMint() bool {
	return t.FromAddress == "0x0000000000000000000000000000000000000000" ||
		t.FromAddress == "0x0000000000000000000000000000000000000000000000000000000000000000"
}

// IsBurn returns true if this is a burn operation (to address is zero address)
func (t *TokenTransfer) IsBurn() bool {
	return t.ToAddress == "0x0000000000000000000000000000000000000000" ||
		t.ToAddress == "0x0000000000000000000000000000000000000000000000000000000000000000"
}

// GetMethod returns the transfer method based on from/to addresses
func (t *TokenTransfer) GetMethod() string {
	if t.IsMint() {
		return "Mint"
	}
	if t.IsBurn() {
		return "Burn"
	}
	return "Transfer"
}

// TokenTransferQuery represents query parameters for token transfers
type TokenTransferQuery struct {
	ContractAddress string
	FromAddress     string
	ToAddress       string
	Address         string // Filter by either from or to
	Method          string
	ChainID         string
	StartTime       *time.Time
	EndTime         *time.Time
	Page            int
	PageSize        int
}
