// Package models provides data models for bank transfer records.
package models

import "time"

// BankTransfer represents a bank transfer record in the database.
// It stores information about native token transfers (e.g., UINJ),
// including sender, recipient, amounts, and transaction details.
type BankTransfer struct {
	ID uint64 `json:"id" gorm:"column:id;primaryKey"`

	// Transfer details
	TxHash    string `json:"tx_hash" gorm:"column:tx_hash;size:128;not null;index:idx_bank_transfers_tx_hash"`
	Sender    string `json:"sender" gorm:"column:sender;size:128;not null;index:idx_bank_transfers_sender;index:idx_bank_transfers_sender_time,priority:1"`
	Recipient string `json:"recipient" gorm:"column:recipient;size:128;not null;index:idx_bank_transfers_recipient;index:idx_bank_transfers_recipient_time,priority:1"`
	Amounts   string `json:"amounts" gorm:"column:amounts;type:text;not null"` // JSON: [{"denom":"uinj","amount":"1000"}]

	// Block info
	BlockHeight   int64 `json:"block_height" gorm:"column:block_height;not null;index:idx_bank_transfers_block_height"`
	UnixTimestamp int64 `json:"unix_timestamp" gorm:"column:unix_timestamp;not null;index:idx_bank_transfers_timestamp;index:idx_bank_transfers_sender_time,priority:2,sort:desc;index:idx_bank_transfers_recipient_time,priority:2,sort:desc"`

	// Status
	Success                bool       `json:"success" gorm:"column:success;not null;default:true"`
	IsCommunityPoolRelated bool       `json:"is_community_pool_related" gorm:"column:is_community_pool_related;default:false"`
	CreatedAt              *time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName returns the table name for BankTransfer model.
func (BankTransfer) TableName() string {
	return "bank_transfers"
}
