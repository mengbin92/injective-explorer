// Package models provides data models for contract execution records.
package models

import (
	"time"
)

// ContractExecution represents a contract execution record in the database.
// It stores information about each contract execution, including sender,
// execution message, attached funds, and success status.
type ContractExecution struct {
	ID uint `json:"id" gorm:"column:id;primaryKey"`

	// Execution details
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null;index:idx_contract_executions_contract_address;index:idx_contract_executions_address_time,priority:1"`
	TxHash          string `json:"tx_hash" gorm:"column:tx_hash;size:128;not null;index:idx_contract_executions_tx_hash"`
	Sender          string `json:"sender" gorm:"column:sender;size:128;not null;index:idx_contract_executions_sender"`
	Msg             string `json:"msg" gorm:"column:msg;type:text;not null"`
	Funds           string `json:"funds" gorm:"column:funds;type:text"`

	// Block info
	BlockHeight   int64 `json:"block_height" gorm:"column:block_height;not null;index:idx_contract_executions_block_height"`
	UnixTimestamp int64 `json:"unix_timestamp" gorm:"column:unix_timestamp;not null;index:idx_contract_executions_timestamp;index:idx_contract_executions_address_time,priority:2,sort:desc"`

	// Status
	Success   bool       `json:"success" gorm:"column:success;not null;index:idx_contract_executions_success"`
	CreatedAt *time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName returns the table name for ContractExecution model.
func (ContractExecution) TableName() string {
	return "contract_executions"
}
