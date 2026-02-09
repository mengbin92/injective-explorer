// Package models provides data models for CW20 token balances and transfers.
package models

import (
	"time"
)

// CW20Balance represents a CW20 token balance in the database.
// It stores the current balance of a token holder for a specific CW20 contract.
type CW20Balance struct {
	ID uint `json:"id" gorm:"column:id;primaryKey"`

	// Balance identification
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null;uniqueIndex:uk_cw20_contract_owner,priority:1;index:idx_cw20_balances_contract_address"`
	OwnerAddress    string `json:"owner_address" gorm:"column:owner_address;size:128;not null;uniqueIndex:uk_cw20_contract_owner,priority:2;index:idx_cw20_balances_owner_address"`

	// Balance data
	Balance   string     `json:"balance" gorm:"column:balance;size:255;not null"`
	UpdatedAt *time.Time `json:"updated_at" gorm:"column:updated_at;autoUpdateTime:milli;index:idx_cw20_balances_updated_at"`
}

// TableName returns the table name for CW20Balance model.
func (CW20Balance) TableName() string {
	return "cw20_balances"
}

// CW20Transfer represents a CW20 token transfer in the database.
// It stores transfer history for CW20 tokens, including sender, receiver,
// amount, and transaction details.
type CW20Transfer struct {
	ID uint `json:"id" gorm:"column:id;primaryKey"`

	// Transfer details
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null;index:idx_cw20_transfers_contract_address"`
	FromAddress     string `json:"from_address" gorm:"column:from_address;size:128;not null;index:idx_cw20_transfers_from_address"`
	ToAddress       string `json:"to_address" gorm:"column:to_address;size:128;not null;index:idx_cw20_transfers_to_address"`
	Amount          string `json:"amount" gorm:"column:amount;size:255;not null"`

	// Transaction info
	TxHash      string `json:"tx_hash" gorm:"column:tx_hash;size:128;not null;index:idx_cw20_transfers_tx_hash"`
	BlockHeight int64  `json:"block_height" gorm:"column:block_height;not null;index:idx_cw20_transfers_block_height"`

	// Timestamp
	UnixTimestamp int64      `json:"unix_timestamp" gorm:"column:unix_timestamp;not null;index:idx_cw20_transfers_timestamp"`
	CreatedAt     *time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName returns the table name for CW20Transfer model.
func (CW20Transfer) TableName() string {
	return "cw20_transfers"
}
