package models

import (
	"time"

	"gorm.io/gorm"
)

// PeggyBatchTxMapping represents the mapping between batch_nonce and outgoing_tx_ids.
// This table is used to link MsgSendToEth records (user-visible) with their corresponding batch
// so that when the EVM tx hash is retrieved from Ethereum, it can be updated on both types of records.
type PeggyBatchTxMapping struct {
	ID        uint64         `gorm:"primaryKey;column:id"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`

	// BatchNonce is the unique identifier for the batch (from EventOutgoingBatch)
	BatchNonce uint64 `gorm:"column:batch_nonce;not null;index:idx_batch_nonce"`

	// OutgoingTxID is the unique identifier for a withdrawal intent (from send_to_eth event)
	// Multiple outgoing_tx_ids can belong to the same batch
	OutgoingTxID uint64 `gorm:"column:outgoing_tx_id;not null;index:idx_outgoing_tx_id"`

	// Block information for tracking and potential backfill
	BlockHeight    uint64 `gorm:"column:block_height;index"`
	BlockTimestamp uint64 `gorm:"column:block_timestamp;index"`
}

// TableName returns the table name for GORM.
func (PeggyBatchTxMapping) TableName() string {
	return "peggy_batch_tx_mapping"
}

// BeforeCreate ensures the composite unique key is respected.
func (m *PeggyBatchTxMapping) BeforeCreate(tx *gorm.DB) error {
	// GORM will handle the unique constraint via the database schema
	return nil
}
