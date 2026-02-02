package models

import (
	"time"

	v1 "injective-explorer/api/explorer/v1"

	"gorm.io/gorm"
)

// PeggyWithdrawalTx represents a Peggy withdrawal transaction in the database.
type PeggyWithdrawalTx struct {
	ID        uint64         `gorm:"primaryKey;column:id"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`

	Sender              string `gorm:"column:sender;size:128;not null;index"`
	Receiver            string `gorm:"column:receiver;size:128;not null;index"`
	Amount              string `gorm:"column:amount;size:100"`
	Denom               string `gorm:"column:denom;size:100"`
	BridgeFee           string `gorm:"column:bridge_fee;size:100"`
	OutgoingTxId        uint64 `gorm:"column:outgoing_tx_id;uniqueIndex"`
	BatchTimeout        uint64 `gorm:"column:batch_timeout"`
	BatchNonce          uint64 `gorm:"column:batch_nonce"`
	OrchestratorAddress string `gorm:"column:orchestrator_address;size:128"`
	EventNonce          uint64 `gorm:"column:event_nonce"`
	EventHeight         uint64 `gorm:"column:event_height"`
	State               string `gorm:"column:state;size:50;default:'pending'"`
	ClaimType           int32  `gorm:"column:claim_type;default:2"`

	TxHash         string `gorm:"column:tx_hash;size:128;not null;index"`
	EthTxHash      string `gorm:"column:eth_tx_hash;size:128;index"`
	BlockHeight    uint64 `gorm:"column:block_height;index"`
	BlockTimestamp uint64 `gorm:"column:block_timestamp;index"`
}

// TableName returns the table name for PeggyWithdrawalTx.
func (PeggyWithdrawalTx) TableName() string {
	return "peggy_withdrawal_txs"
}

// ToProto converts PeggyWithdrawalTx to proto DTO.
func (t *PeggyWithdrawalTx) ToProto() *v1.PeggyWithdrawalTxDTO {
	return &v1.PeggyWithdrawalTxDTO{
		Sender:              t.Sender,
		Receiver:            t.Receiver,
		Amount:              t.Amount,
		Denom:               t.Denom,
		BridgeFee:           t.BridgeFee,
		OutgoingTxId:        t.OutgoingTxId,
		BatchTimeout:        t.BatchTimeout,
		BatchNonce:          t.BatchNonce,
		OrchestratorAddress: t.OrchestratorAddress,
		EventNonce:          t.EventNonce,
		EventHeight:         t.EventHeight,
		State:               t.State,
		ClaimType:           t.ClaimType,
		TxHashes:            []string{t.TxHash},
		EthTxHash:           t.EthTxHash,
		CreatedAt:           t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           t.UpdatedAt.Format(time.RFC3339),
	}
}

// ToPeggyWithdrawalTxProtos converts slice of PeggyWithdrawalTx to proto DTOs.
func ToPeggyWithdrawalTxProtos(txs []*PeggyWithdrawalTx) []*v1.PeggyWithdrawalTxDTO {
	result := make([]*v1.PeggyWithdrawalTxDTO, len(txs))
	for i, tx := range txs {
		result[i] = tx.ToProto()
	}
	return result
}
