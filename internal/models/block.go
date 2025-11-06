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

	Height             uint64   `json:"height" gorm:"column:height;index:idx_height;index:idx_height_desc"`               // Height is the block height on the blockchain (indexed for pagination)
	Proposer           string   `json:"proposer" gorm:"column:proposer"`                                                  // Proposer is the address of the block proposer
	Moniker            string   `json:"moniker" gorm:"column:moniker"`                                                    // Moniker is the moniker (name) of the block proposer
	BlockHash          string   `json:"block_hash" gorm:"column:block_hash;index"`                                        // BlockHash is the hash of the current block
	ParentHash         string   `json:"parent_hash" gorm:"column:parent_hash"`                                            // ParentHash is the hash of the parent block
	NumPrecommits      int64    `json:"num_precommits" gorm:"column:num_precommits"`                                      // NumPrecommits is the number of precommits for this block
	NumTxs             int64    `json:"num_txs" gorm:"column:num_txs"`                                                    // NumTxs is the number of transactions in this block
	TotalTxs           int64    `json:"total_txs" gorm:"column:total_txs"`                                                // TotalTxs is the total number of transactions in the blockchain up to this block
	Txs                []string `json:"txs" gorm:"column:txs;serializer:json"`                                            // Txs is a list of transactions in this block (not stored in database)
	Timestamp          string   `json:"timestamp" gorm:"column:timestamp"`                                                // Timestamp is the human-readable timestamp of the block
	BlockUnixTimestamp uint64   `json:"block_unix_timestamp" gorm:"column:block_unix_timestamp;index:idx_timestamp_desc"` // BlockUnixTimestamp is the Unix timestamp of the block (indexed for sorting)                            // Validators is a list of validator addresses for this block (stored as JSONB)
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

	txs := make([]string, len(detail.Txs))
	for i, tx := range detail.Txs {
		txs[i] = tx.String()
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
		Txs:                txs,
	}
}

// BlockToProto converts a Block model back to a protobuf BlockDetailInfo.
//
// Parameters:
//   - block: The Block model from database
//
// Returns:
//   - *injective_explorer_rpcpb.BlockDetailInfo: A new BlockDetailInfo instance with data from the Block model
func BlockToProto(block *Block) *injective_explorer_rpcpb.BlockDetailInfo {
	if block == nil {
		return nil
	}

	// Convert Txs from string slice back to protobuf format
	// Note: Since Txs are stored as strings, we need to parse them back
	// For simplicity, we'll create empty Txs array as the original format is lost
	txs := make([]*injective_explorer_rpcpb.TxData, 0, len(block.Txs))
	// Note: We cannot fully reconstruct the original TxData from strings,
	// so we'll leave Txs empty or minimal

	return &injective_explorer_rpcpb.BlockDetailInfo{
		Height:             block.Height,
		Proposer:           block.Proposer,
		Moniker:            block.Moniker,
		BlockHash:          block.BlockHash,
		ParentHash:         block.ParentHash,
		NumPreCommits:      block.NumPrecommits,
		NumTxs:             block.NumTxs,
		TotalTxs:           block.TotalTxs,
		Timestamp:          block.Timestamp,
		BlockUnixTimestamp: block.BlockUnixTimestamp,
		Txs:                txs,
	}
}
