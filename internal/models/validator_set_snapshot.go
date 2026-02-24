// Package models provides data models for blockchain entities.
// This file defines the ValidatorSetSnapshot model for tracking validator set changes.
package models

import (
	"injective-explorer/internal/global"
	"injective-explorer/provider/db"
	"fmt"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ValidatorSetSnapshot represents a snapshot of the validator set at a specific block height.
// This is used to track which validators should sign blocks, enabling accurate missed block calculation.
type ValidatorSetSnapshot struct {
	gorm.Model

	BlockHeight            uint64 `json:"block_height" gorm:"column:block_height;index"`
	ValidatorOperatorAddress string `json:"validator_operator_address" gorm:"column:validator_operator_address;index"`
	ValidatorConsensusAddress string `json:"validator_consensus_address" gorm:"column:validator_consensus_address;index"`
	VotingPower           uint64 `json:"voting_power" gorm:"column:voting_power"`
	BlockUnixTimestamp    uint64 `json:"block_unix_timestamp" gorm:"column:block_unix_timestamp"`
}

// TableName returns the table name for ValidatorSetSnapshot model.
func (ValidatorSetSnapshot) TableName() string {
	return "validator_set_snapshots"
}

// SaveValidatorSetSnapshot saves a validator set snapshot at a specific block height.
// This should be called when the validator set changes, or periodically (e.g., every N blocks).
//
// Parameters:
//   - blockHeight: The block height for this snapshot
//   - validators: The validator set at this height
//   - blockUnixTimestamp: The block timestamp
//
// Returns:
//   - error: Error if saving fails
func SaveValidatorSetSnapshot(blockHeight uint64, validators *coretypes.ResultValidators, blockUnixTimestamp uint64) error {
	if validators == nil || len(validators.Validators) == 0 {
		return fmt.Errorf("validators is nil or empty")
	}

	var snapshots []ValidatorSetSnapshot

	for _, val := range validators.Validators {
		if val.Address == nil {
			continue
		}

		consensusHex := fmt.Sprintf("%X", val.Address)

		// Convert hex to bech32 for DB lookup - validators.consensus_address stores injvalcons1xxx
		consensusBech32, convErr := HexToValconsAddress(consensusHex)
		if convErr != nil {
			global.Logger.Warnf("failed to convert consensus hex %s to bech32: %v", consensusHex, convErr)
		}

		// Get validator operator address (validators table uses bech32 consensus_address)
		var validator Validator
		lookupAddr := consensusHex
		if convErr == nil {
			lookupAddr = consensusBech32
		}
		validatorErr := db.Get().Model(&Validator{}).Where("consensus_address = ?", lookupAddr).First(&validator).Error
		operatorAddress := consensusBech32 // Fallback: use bech32 if no validator found
		if validatorErr == nil && validator.OperatorAddress != "" {
			operatorAddress = validator.OperatorAddress
		} else if convErr != nil {
			operatorAddress = consensusHex // Last fallback when conversion failed
		}

		// Store bech32 for consistency with validators table
		consensusAddrToStore := consensusBech32
		if convErr != nil {
			consensusAddrToStore = consensusHex
		}
		snapshots = append(snapshots, ValidatorSetSnapshot{
			BlockHeight:            blockHeight,
			ValidatorOperatorAddress: operatorAddress,
			ValidatorConsensusAddress: consensusAddrToStore,
			VotingPower:           uint64(val.VotingPower),
			BlockUnixTimestamp:    blockUnixTimestamp,
		})
	}

	if len(snapshots) == 0 {
		return fmt.Errorf("no validators to save")
	}

	// Batch insert using ON DUPLICATE KEY UPDATE
	err := db.Get().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "block_height"}, {Name: "validator_operator_address"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"validator_consensus_address",
			"voting_power",
			"block_unix_timestamp",
			"updated_at",
		}),
	}).Create(&snapshots).Error

	if err != nil {
		global.Logger.Errorf("failed to save validator set snapshot for height %d: %v", blockHeight, err)
		return fmt.Errorf("failed to save validator set snapshot: %w", err)
	}

	global.Logger.Debugf("saved validator set snapshot for height %d: %d validators", blockHeight, len(snapshots))
	return nil
}

// GetValidatorSetAtHeight retrieves the validator set snapshot at a specific block height.
// If exact height not found, returns the most recent snapshot before the height.
//
// Parameters:
//   - height: The block height to query
//
// Returns:
//   - []ValidatorSetSnapshot: List of validators in the snapshot
//   - error: Error if query fails
func GetValidatorSetAtHeight(height uint64) ([]ValidatorSetSnapshot, error) {
	var snapshots []ValidatorSetSnapshot

	// Find the most recent snapshot at or before this height
	var snapshotHeight uint64
	err := db.Get().Model(&ValidatorSetSnapshot{}).
		Where("block_height <= ?", height).
		Order("block_height DESC").
		Limit(1).
		Pluck("block_height", &snapshotHeight).Error

	if err != nil {
		return nil, fmt.Errorf("failed to find validator set snapshot: %w", err)
	}

	// Get all validators at this snapshot height
	err = db.Get().Where("block_height = ?", snapshotHeight).Find(&snapshots).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get validator set snapshot: %w", err)
	}

	return snapshots, nil
}

// CheckIfValidatorSetChanged checks if the validator set has changed since the last snapshot.
// This helps determine if we need to save a new snapshot.
//
// Parameters:
//   - currentHeight: Current block height
//   - currentValidators: Current validator set
//
// Returns:
//   - bool: true if validator set changed, false otherwise
//   - error: Error if check fails
func CheckIfValidatorSetChanged(currentHeight uint64, currentValidators *coretypes.ResultValidators) (bool, error) {
	if currentValidators == nil {
		return false, fmt.Errorf("validators is nil or empty")
	}

	// Get the most recent snapshot
	var lastSnapshot ValidatorSetSnapshot
	err := db.Get().Order("block_height DESC").First(&lastSnapshot).Error

	// If no snapshot exists, validator set has "changed" (first snapshot)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get last snapshot: %w", err)
	}

	// Get validators from last snapshot
	var lastSnapshotValidators []ValidatorSetSnapshot
	err = db.Get().Where("block_height = ?", lastSnapshot.BlockHeight).Find(&lastSnapshotValidators).Error
	if err != nil {
		return false, fmt.Errorf("failed to get last snapshot validators: %w", err)
	}

	// Compare validator sets
	if len(currentValidators.Validators) != len(lastSnapshotValidators) {
		return true, nil // Different number of validators
	}

	// Create map of current validators by consensus address (both hex and bech32 for backward compat)
	currentMap := make(map[string]bool)
	for _, val := range currentValidators.Validators {
		if val.Address != nil && len(val.Address) > 0 {
			consensusHex := fmt.Sprintf("%X", val.Address)
			currentMap[consensusHex] = true
			consensusBech32, err := HexToValconsAddress(consensusHex)
			if err == nil {
				currentMap[consensusBech32] = true
			}
		}
	}

	// Check if all validators in last snapshot exist in current set
	for _, snapshotVal := range lastSnapshotValidators {
		if !currentMap[snapshotVal.ValidatorConsensusAddress] {
			return true, nil // Validator removed
		}
	}

	// Check if all validators in current set exist in last snapshot
	for _, val := range currentValidators.Validators {
		if val.Address != nil && len(val.Address) > 0 {
			consensusHex := fmt.Sprintf("%X", val.Address)
			consensusBech32, _ := HexToValconsAddress(consensusHex)
			found := false
			for _, snapshotVal := range lastSnapshotValidators {
				// Snapshot may store hex (old) or bech32 (new)
				if snapshotVal.ValidatorConsensusAddress == consensusHex || snapshotVal.ValidatorConsensusAddress == consensusBech32 {
					found = true
					break
				}
			}
			if !found {
				return true, nil // New validator added
			}
		}
	}

	return false, nil // Validator set unchanged
}
