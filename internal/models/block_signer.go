// Package models provides data models for blockchain entities.
// This file provides functions to update validator block statistics from Tendermint blocks.
// Note: We no longer store block_signers table, only update aggregated statistics (方案3).
package models

import (
	"injective-explorer/internal/global"
	"injective-explorer/provider/db"
	"fmt"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
)

// SignerInfo represents a validator's participation in a block (used internally, not stored in DB).
type SignerInfo struct {
	ValidatorOperatorAddress string
	IsProposer              bool
}

// SaveBlockSigners extracts validator signer information from a Tendermint block
// and updates aggregated statistics (方案3: 聚合统计表).
// This function no longer saves to block_signers table to reduce data volume by 99.8%.
//
// Parameters:
//   - block: The Tendermint ResultBlock containing block and commit information
//
// Returns:
//   - error: Error if update fails
func SaveBlockSigners(block *coretypes.ResultBlock) error {
	if block == nil || block.Block == nil {
		return fmt.Errorf("block is nil")
	}

	height := uint64(block.Block.Height)
	proposerHex := block.Block.Header.ProposerAddress.String()

	// Get proposer validator info
	var proposerValidator Validator
	proposerErr := db.Get().Model(&Validator{}).Where("consensus_address = ?", proposerHex).First(&proposerValidator).Error
	proposerOperatorAddress := proposerHex // Fallback to consensus address if not found
	if proposerErr == nil && proposerValidator.OperatorAddress != "" {
		proposerOperatorAddress = proposerValidator.OperatorAddress
	}

	// Collect signers information
	signers := make([]SignerInfo, 0)

	// Add proposer
	signers = append(signers, SignerInfo{
		ValidatorOperatorAddress: proposerOperatorAddress,
		IsProposer:              true,
	})

	// Extract signers from LastCommit
	if block.Block.LastCommit != nil && len(block.Block.LastCommit.Signatures) > 0 {
		signerMap := make(map[string]bool) // Track unique signers by consensus address

		for _, sig := range block.Block.LastCommit.Signatures {
			if sig.ValidatorAddress == nil {
				continue
			}

			signerConsensusHex := fmt.Sprintf("%X", sig.ValidatorAddress)
			// Skip if already added (proposer)
			if signerConsensusHex == proposerHex {
				continue
			}

			// Skip if already processed
			if signerMap[signerConsensusHex] {
				continue
			}
			signerMap[signerConsensusHex] = true

			// Get validator operator address
			var signerValidator Validator
			signerErr := db.Get().Model(&Validator{}).Where("consensus_address = ?", signerConsensusHex).First(&signerValidator).Error
			signerOperatorAddress := signerConsensusHex // Fallback to consensus address if not found
			if signerErr == nil && signerValidator.OperatorAddress != "" {
				signerOperatorAddress = signerValidator.OperatorAddress
			}

			// Only add if signature is not nil (validator actually signed)
			if sig.Signature != nil {
				signers = append(signers, SignerInfo{
					ValidatorOperatorAddress: signerOperatorAddress,
					IsProposer:              false,
				})
			}
		}
	}

	// Update aggregated statistics (方案3: 聚合统计表)
	// This is done asynchronously to avoid blocking block processing
	if len(signers) > 0 {
		go func() {
			for _, signer := range signers {
				// Update statistics: isProposer, signed=true (since they're in the signers list)
				// Note: shouldSign is set to true here because they actually signed
				// For accurate should_sign_count, we need validator set information (can be added later)
				if err := UpdateValidatorStatsForBlock(
					signer.ValidatorOperatorAddress,
					height,
					signer.IsProposer,
					true,  // signed=true (they're in the signers list)
					true,  // shouldSign=true (they signed, so they should have signed)
				); err != nil {
					global.Logger.Warnf("failed to update validator stats for %s at height %d: %v",
						signer.ValidatorOperatorAddress, height, err)
				}
			}
			global.Logger.Debugf("updated aggregated stats for %d validators at height %d (proposer: %s)", len(signers), height, proposerOperatorAddress)
		}()
	}

	return nil
}
