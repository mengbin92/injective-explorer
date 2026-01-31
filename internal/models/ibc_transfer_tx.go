package models

import (
	"time"

	v1 "injective-explorer/api/explorer/v1"
	"gorm.io/gorm"
)

// IBCTransferTx represents an IBC transfer transaction in the database.
type IBCTransferTx struct {
	ID                 uint64         `gorm:"primaryKey;column:id"`
	CreatedAt          time.Time      `gorm:"column:created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"column:deleted_at;index"`

	Sender             string         `gorm:"column:sender;size:128;not null;index"`
	Receiver           string         `gorm:"column:receiver;size:128;not null;index"`
	SourcePort         string         `gorm:"column:source_port;size:100"`
	SourceChannel      string         `gorm:"column:source_channel;size:100;index"`
	DestinationPort    string         `gorm:"column:destination_port;size:100"`
	DestinationChannel string         `gorm:"column:destination_channel;size:100;index"`
	Amount             string         `gorm:"column:amount;size:100"`
	Denom              string         `gorm:"column:denom;size:100"`
	TimeoutHeight      string         `gorm:"column:timeout_height;size:100"`
	TimeoutTimestamp   uint64         `gorm:"column:timeout_timestamp"`
	PacketSequence     uint64         `gorm:"column:packet_sequence"`
	DataHex            []byte         `gorm:"column:data_hex;type:blob"`
	State              string         `gorm:"column:state;size:50;default:'completed'"`

	TxHash             string         `gorm:"column:tx_hash;size:128;not null;index"`
	BlockHeight        uint64         `gorm:"column:block_height;index"`
	BlockTimestamp     uint64         `gorm:"column:block_timestamp;index"`
}

// TableName returns the table name for IBCTransferTx.
func (IBCTransferTx) TableName() string {
	return "ibc_transfer_txs"
}

// ToProto converts IBCTransferTx to proto DTO.
func (t *IBCTransferTx) ToProto() *v1.IBCTransferTxDTO {
	return &v1.IBCTransferTxDTO{
		Sender:             t.Sender,
		Receiver:           t.Receiver,
		SourcePort:         t.SourcePort,
		SourceChannel:      t.SourceChannel,
		DestinationPort:    t.DestinationPort,
		DestinationChannel: t.DestinationChannel,
		Amount:             t.Amount,
		Denom:              t.Denom,
		TimeoutHeight:      t.TimeoutHeight,
		TimeoutTimestamp:   t.TimeoutTimestamp,
		PacketSequence:     t.PacketSequence,
		DataHex:            t.DataHex,
		State:              t.State,
		TxHashes:           []string{t.TxHash},
		CreatedAt:          t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          t.UpdatedAt.Format(time.RFC3339),
	}
}

// ToIBCTransferTxProtos converts slice of IBCTransferTx to proto DTOs.
func ToIBCTransferTxProtos(txs []*IBCTransferTx) []*v1.IBCTransferTxDTO {
	result := make([]*v1.IBCTransferTxDTO, len(txs))
	for i, tx := range txs {
		result[i] = tx.ToProto()
	}
	return result
}
