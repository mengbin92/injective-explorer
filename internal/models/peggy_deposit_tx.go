package models

import (
	"time"

	v1 "injective-explorer/api/explorer/v1"

	"gorm.io/gorm"
)

// PeggyDepositTx represents a Peggy deposit transaction in the database.
type PeggyDepositTx struct {
	ID        uint64         `gorm:"primaryKey;column:id"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`

	Sender              string `gorm:"column:sender;size:128;not null;index"`
	Receiver            string `gorm:"column:receiver;size:128;not null;index"`
	Amount              string `gorm:"column:amount;size:100"`
	Denom               string `gorm:"column:denom;size:100"`
	EventNonce          uint64 `gorm:"column:event_nonce;uniqueIndex"`
	EventHeight         uint64 `gorm:"column:event_height"`
	OrchestratorAddress string `gorm:"column:orchestrator_address;size:128"`
	State               string `gorm:"column:state;size:50;default:'completed'"`
	ClaimType           int32  `gorm:"column:claim_type;default:1"`

	TxHash         string `gorm:"column:tx_hash;size:128;not null;index"`
	EthTxHash      string `gorm:"column:eth_tx_hash;size:128;index"`
	BlockHeight    uint64 `gorm:"column:block_height;index"`
	BlockTimestamp uint64 `gorm:"column:block_timestamp;index"`
}

// TableName returns the table name for PeggyDepositTx.
func (PeggyDepositTx) TableName() string {
	return "peggy_deposit_txs"
}

// ToProto converts PeggyDepositTx to proto DTO.
func (t *PeggyDepositTx) ToProto() *v1.PeggyDepositTxDTO {
	return &v1.PeggyDepositTxDTO{
		Sender:              t.Sender,
		Receiver:            t.Receiver,
		Amount:              t.Amount,
		Denom:               t.Denom,
		EventNonce:          t.EventNonce,
		EventHeight:         t.EventHeight,
		OrchestratorAddress: t.OrchestratorAddress,
		State:               t.State,
		ClaimType:           t.ClaimType,
		TxHashes:            []string{t.TxHash},
		EthTxHash:           t.EthTxHash,
		CreatedAt:           t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           t.UpdatedAt.Format(time.RFC3339),
	}
}

// ToPeggyDepositTxProtos converts slice of PeggyDepositTx to proto DTOs.
func ToPeggyDepositTxProtos(txs []*PeggyDepositTx) []*v1.PeggyDepositTxDTO {
	result := make([]*v1.PeggyDepositTxDTO, len(txs))
	for i, tx := range txs {
		result[i] = tx.ToProto()
	}
	return result
}
