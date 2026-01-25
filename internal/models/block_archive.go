// Package models provides data models for block archiving.
package models

import (
	"time"

	"gorm.io/gorm"
)

// BlockDataArchive represents archived block data in the database.
// Stores the txs field that has been moved from the main blocks table.
type BlockDataArchive struct {
	BlockID       uint64    `json:"block_id" gorm:"primaryKey;column:block_id"` // Block ID (references blocks.id)
	Txs           string    `json:"txs" gorm:"column:txs"` // Compressed transaction list (JSON)
	TxsStorageKey string    `json:"txs_storage_key" gorm:"column:txs_storage_key"` // Object storage key if uploaded
	ArchivedAt    time.Time `json:"archived_at" gorm:"column:archived_at"` // Archive timestamp
}

// TableName specifies the table name for BlockDataArchive model.
func (BlockDataArchive) TableName() string {
	return "block_data_archives"
}

// BeforeCreate - GORM hook called before creating a new record.
func (bda *BlockDataArchive) BeforeCreate(tx *gorm.DB) error {
	// Set archived_at timestamp if not already set
	if bda.ArchivedAt.IsZero() {
		bda.ArchivedAt = time.Now()
	}
	return nil
}
