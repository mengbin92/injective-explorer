// Package models provides data models for blockchain entities.
// This file provides functions to update validator block statistics from Tendermint blocks.
// Note: We no longer store block_signers table, only update aggregated statistics (方案3).
package models

import (
	"injective-explorer/internal/global"
	"injective-explorer/provider/db"
	"fmt"
	"strings"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// ValidatorQueryClient defines the interface for querying validators from chain.
// This is used to fetch validator info in real-time when it's not in local DB.
type ValidatorQueryClient interface {
	// GetValidators queries all validators from chain
	GetValidators(ctx interface{}) ([]stakingtypes.Validator, error)
}

// validatorQueryClient is the global query client set by service layer
var validatorQueryClient ValidatorQueryClient

// SetValidatorQueryClient sets the global validator query client.
// This should be called once during service initialization.
func SetValidatorQueryClient(client ValidatorQueryClient) {
	validatorQueryClient = client
}

// hexToConsAddress converts hex consensus address to bech32 format.
// Block header proposer address is in hex format (uppercase), but DB stores bech32 format.
func hexToConsAddress(hexAddr string) string {
	// If already in bech32 format (contains "1" which is the separator), return as-is
	if strings.Contains(hexAddr, "1") {
		return hexAddr
	}

	// Remove "0x" prefix if present
	if len(hexAddr) >= 2 && (hexAddr[0:2] == "0x" || hexAddr[0:2] == "0X") {
		hexAddr = hexAddr[2:]
	}

	// Hex decode
	if len(hexAddr)%2 != 0 {
		return hexAddr // Invalid hex, return as-is
	}

	addrBytes := make([]byte, len(hexAddr)/2)
	for i := 0; i < len(hexAddr); i += 2 {
		var b byte
		if _, err := fmt.Sscanf(hexAddr[i:i+2], "%02X", &b); err != nil {
			// Try lowercase
			if _, err := fmt.Sscanf(hexAddr[i:i+2], "%02x", &b); err != nil {
				return hexAddr // Invalid hex, return as-is
			}
		}
		addrBytes[i/2] = b
	}

	// Convert to bech32 consensus address
	consAddr := sdk.ConsAddress(addrBytes)
	return consAddr.String()
}

// getOrFetchValidator gets validator from local DB, or fetches from chain if not found.
// It saves the fetched validator to local DB for future use.
// consensusAddress can be either hex format (from block header) or bech32 format.
func getOrFetchValidator(consensusAddress string) (*Validator, error) {
	// Convert hex address to bech32 format if needed
	bech32ConsAddress := hexToConsAddress(consensusAddress)

	// First try to get from local DB
	var validator Validator
	err := db.Get().Model(&Validator{}).Where("consensus_address = ?", bech32ConsAddress).First(&validator).Error
	if err == nil && validator.OperatorAddress != "" {
		return &validator, nil
	}

	// If not found in local DB and we have a query client, try to fetch from chain
	if validatorQueryClient != nil {
		global.Logger.Debugf("validator with consensus address %s not found in local DB, fetching from chain", consensusAddress)

		// Fetch all validators from chain
		validators, err := validatorQueryClient.GetValidators(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch validators from chain: %w", err)
		}

		// Find the validator with matching consensus address and save to DB
		for _, v := range validators {
			validatorModel := ValidatorFromStakingTypes(v)
			if validatorModel == nil {
				continue
			}

			// Save/update validator to local DB
			err := db.Get().Where("operator_address = ?", validatorModel.OperatorAddress).
				Assign(validatorModel).
				FirstOrCreate(&Validator{}).Error
			if err != nil {
				global.Logger.Warnf("failed to save validator %s to DB: %v", validatorModel.OperatorAddress, err)
				continue
			}

			// Check if this is the validator we're looking for
			// Compare with bech32 format address
			if validatorModel.ConsensusAddress == bech32ConsAddress {
				return validatorModel, nil
			}
		}
	}

	return nil, fmt.Errorf("validator with consensus address %s not found", consensusAddress)
}

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

	// Get proposer validator info - fetch from chain if not in local DB
	proposerValidator, err := getOrFetchValidator(proposerHex)
	if err != nil {
		global.Logger.Warnf("cannot find validator for proposer consensus address %s at height %d: %v", proposerHex, height, err)
		return nil // Skip this block if proposer not found
	}
	proposerOperatorAddress := proposerValidator.OperatorAddress

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

			// Convert hex consensus address to bech32 format for DB lookup
			signerBech32Addr := hexToConsAddress(signerConsensusHex)

			// Get validator operator address - fetch from chain if not in local DB
			signerValidator, err := getOrFetchValidator(signerBech32Addr)
			if err != nil {
				global.Logger.Warnf("cannot find validator for signer consensus address %s (bech32: %s) at height %d: %v", signerConsensusHex, signerBech32Addr, height, err)
				continue // Skip this signer if not found
			}
			signerOperatorAddress := signerValidator.OperatorAddress

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
