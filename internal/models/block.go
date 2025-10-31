// Package models provides data models for blocks and transactions.
package models

import (
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"gorm.io/gorm"
)

// Block represents a blockchain block in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct is designed to store block information from the Injective blockchain explorer,
// including block height, proposer information, hash values, transaction counts, and timestamps.
type Block struct {
	gorm.Model

	// Height is the block height on the blockchain
	Height uint64 `json:"height" gorm:"column:height;index"`
	// Proposer is the address of the block proposer
	Proposer string `json:"proposer" gorm:"column:proposer"`
	// Moniker is the moniker (name) of the block proposer
	Moniker string `json:"moniker" gorm:"column:moniker"`
	// BlockHash is the hash of the current block
	BlockHash string `json:"block_hash" gorm:"column:block_hash"`
	// ParentHash is the hash of the parent block
	ParentHash string `json:"parent_hash" gorm:"column:parent_hash"`
	// NumPrecommits is the number of precommits for this block
	NumPrecommits int64 `json:"num_precommits" gorm:"column:num_precommits"`
	// NumTxs is the number of transactions in this block
	NumTxs int64 `json:"num_txs" gorm:"column:num_txs"`
	// TotalTxs is the total number of transactions in the blockchain up to this block
	TotalTxs int64 `json:"total_txs" gorm:"column:total_txs"`
	// Txs is a list of transactions in this block (not stored in database)
	Txs []*Transaction `json:"txs" gorm:"-"`
	// Timestamp is the human-readable timestamp of the block
	Timestamp string `json:"timestamp" gorm:"column:timestamp"`
	// BlockUnixTimestamp is the Unix timestamp of the block
	BlockUnixTimestamp uint64 `json:"block_unix_timestamp" gorm:"column:block_unix_timestamp"`
	// Validators is a list of validator addresses for this block (stored as JSONB)
	Validators []string `json:"validators" gorm:"column:validators;type:jsonb"`
}

// BlockFromProto converts a protobuf BlockDetailInfo to a Block model.
//
// Parameters:
//   - detail: The protobuf block detail information (note: parameter name corrected from "deatil")
//
// Returns:
//   - *Block: A new Block instance with data from the protobuf message
func BlockFromProto(detail *injective_explorer_rpcpb.BlockDetailInfo) *Block {
	if detail == nil {
		return nil
	}

	return &Block{
		Height:             detail.Height,
		Proposer:           detail.Proposer,
		Moniker:            detail.Moniker,
		BlockHash:          detail.BlockHash,
		ParentHash:         detail.ParentHash,
		NumPrecommits:      detail.NumPreCommits,
		NumTxs:             detail.NumTxs,
		TotalTxs:           detail.TotalTxs,
		Timestamp:          detail.Timestamp,
		BlockUnixTimestamp: detail.BlockUnixTimestamp,
		// Note: Txs field is not populated here as it requires additional processing
	}
}
