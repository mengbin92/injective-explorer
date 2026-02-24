// Package models provides data models for blockchain entities.
// This file provides functions to update validator block statistics from Tendermint blocks.
// Note: We no longer store block_signers table, only update aggregated statistics (方案3).
package models

import (
	"injective-explorer/internal/global"
	"injective-explorer/provider/db"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// ValidatorQueryClient defines the interface for querying validators from chain.
// This is used to fetch validator info in real-time when it's not in local DB.
type ValidatorQueryClient interface {
	// GetValidators queries all validators from chain (current height).
	GetValidators(ctx interface{}) ([]stakingtypes.Validator, error)
	// GetValidatorsAtHeight queries validators at a specific block height.
	// Use this for historical blocks where validators may have since unbonded.
	// Returns error if the chain doesn't support historical queries (e.g., pruned nodes).
	GetValidatorsAtHeight(ctx context.Context, height uint64) ([]stakingtypes.Validator, error)
	// GetValidatorByConsensusAddress queries a single validator by consensus address.
	// If not implemented, returns an error and the caller should fall back to GetValidators.
	GetValidatorByConsensusAddress(ctx context.Context, consAddr string) (*stakingtypes.Validator, error)
}

// BlockSignerService handles block signer operations with dependency injection.
// This service replaces the previous global variable approach for better testability and thread safety.
type BlockSignerService struct {
	validatorQueryClient ValidatorQueryClient
	// Cache for validators fetched from chain to avoid repeated expensive queries
	validatorCache  map[string]*Validator
	cacheMutex      sync.RWMutex
	cacheExpiry     time.Duration
	lastCacheUpdate time.Time
}

// NewBlockSignerService creates a new BlockSignerService instance.
//
// Parameters:
//   - validatorQueryClient: The client for querying validators from chain
//
// Returns:
//   - *BlockSignerService: A new service instance
func NewBlockSignerService(validatorQueryClient ValidatorQueryClient) *BlockSignerService {
	return &BlockSignerService{
		validatorQueryClient: validatorQueryClient,
		validatorCache:       make(map[string]*Validator),
		cacheExpiry:          5 * time.Minute, // Cache expires after 5 minutes
	}
}

// hexToConsAddress converts hex consensus address to bech32 format.
// Block header proposer address is in hex format (uppercase), but DB stores bech32 format (injvalcons1xxx).
// Note: Hex strings can contain "1" as a digit, so we must check for bech32 prefix, not just "1".
func hexToConsAddress(hexAddr string) string {
	// If already in bech32 format (starts with consensus prefix + "1" separator), return as-is
	// e.g. injvalcons1xxx - "1" is the Bech32 separator between HRP and data
	consensusPrefix := sdk.GetConfig().GetBech32ConsensusAddrPrefix()
	if strings.HasPrefix(hexAddr, consensusPrefix+"1") {
		return hexAddr
	}

	// Remove "0x" prefix if present
	hexAddr = strings.TrimPrefix(strings.TrimPrefix(hexAddr, "0x"), "0X")

	// Decode hex string using standard library
	addrBytes, err := hex.DecodeString(hexAddr)
	if err != nil {
		// Invalid hex, return as-is
		global.Logger.Warnf("failed to decode hex address %s: %v", hexAddr, err)
		return hexAddr
	}

	// Convert to bech32 consensus address
	consAddr := sdk.ConsAddress(addrBytes)
	return consAddr.String()
}

// getOrFetchValidator gets validator from local DB, or fetches from chain if not found.
// It saves the fetched validator to local DB for future use.
// consensusAddress can be either hex format (from block header) or bech32 format.
// blockHeight is used for historical query when validator not in current set (e.g., unbonded).
//
// This method uses caching to avoid repeated expensive chain queries for the same validator.
func (s *BlockSignerService) getOrFetchValidator(consensusAddress string, blockHeight uint64) (*Validator, error) {
	// Convert hex address to bech32 format if needed
	bech32ConsAddress := hexToConsAddress(consensusAddress)

	// First try to get from local DB
	var validator Validator
	err := db.Get().Model(&Validator{}).Where("consensus_address = ?", bech32ConsAddress).First(&validator).Error
	if err == nil && validator.OperatorAddress != "" {
		return &validator, nil
	}

	// Check cache first
	s.cacheMutex.RLock()
	cachedValidator, cached := s.validatorCache[bech32ConsAddress]
	cacheValid := time.Since(s.lastCacheUpdate) < s.cacheExpiry
	s.cacheMutex.RUnlock()

	if cached && cacheValid && cachedValidator != nil {
		global.Logger.Debugf("validator with consensus address %s found in cache", consensusAddress)
		return cachedValidator, nil
	}

	// If not found in local DB and we have a query client, try to fetch from chain
	if s.validatorQueryClient == nil {
		return nil, fmt.Errorf("validator query client not available")
	}

	global.Logger.Debugf("validator with consensus address %s not found in local DB, fetching from chain", consensusAddress)

	// Try to use optimized query method if available
	ctx := context.Background()
	stakingValidator, err := s.validatorQueryClient.GetValidatorByConsensusAddress(ctx, bech32ConsAddress)
	if err == nil && stakingValidator != nil {
		// Found via optimized query
		validatorModel := ValidatorFromStakingTypes(*stakingValidator)
		if validatorModel == nil {
			return nil, fmt.Errorf("failed to convert validator from staking types")
		}

		// Save/update validator to local DB
		err := db.Get().Where("operator_address = ?", validatorModel.OperatorAddress).
			Assign(validatorModel).
			FirstOrCreate(&Validator{}).Error
		if err != nil {
			global.Logger.Warnf("failed to save validator %s to DB: %v", validatorModel.OperatorAddress, err)
		}

		// Update cache
		s.cacheMutex.Lock()
		s.validatorCache[bech32ConsAddress] = validatorModel
		s.lastCacheUpdate = time.Now()
		s.cacheMutex.Unlock()

		return validatorModel, nil
	}

	// Fallback to fetching all validators (current height first)
	validators, err := s.validatorQueryClient.GetValidators(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch validators from chain: %w", err)
	}

	foundValidator := s.findAndPersistValidator(validators, bech32ConsAddress)
	if foundValidator != nil {
		return foundValidator, nil
	}

	// For historical blocks: validator may have unbonded, query at block height
	if blockHeight > 0 {
		global.Logger.Debugf("validator %s not in current set, trying historical query at height %d", consensusAddress, blockHeight)
		historicalValidators, histErr := s.validatorQueryClient.GetValidatorsAtHeight(ctx, blockHeight)
		if histErr != nil {
			return nil, fmt.Errorf("validator with consensus address %s not found (current set), historical query at height %d failed: %w", consensusAddress, blockHeight, histErr)
		}
		foundValidator = s.findAndPersistValidator(historicalValidators, bech32ConsAddress)
		if foundValidator != nil {
			global.Logger.Debugf("found validator %s via historical query at height %d", consensusAddress, blockHeight)
			return foundValidator, nil
		}
	}

	return nil, fmt.Errorf("validator with consensus address %s not found", consensusAddress)
}

// findAndPersistValidator finds validator by consensus address in the list, persists to DB and cache.
func (s *BlockSignerService) findAndPersistValidator(validators []stakingtypes.Validator, bech32ConsAddress string) *Validator {
	var foundValidator *Validator
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

		if validatorModel.ConsensusAddress == bech32ConsAddress {
			foundValidator = validatorModel
		}

		// Update cache
		s.cacheMutex.Lock()
		s.validatorCache[validatorModel.ConsensusAddress] = validatorModel
		s.lastCacheUpdate = time.Now()
		s.cacheMutex.Unlock()
	}
	return foundValidator
}

// SignerInfo represents a validator's participation in a block (used internally, not stored in DB).
type SignerInfo struct {
	ValidatorOperatorAddress string
	IsProposer               bool
}

// SaveBlockSigners extracts validator signer information from a Tendermint block
// and updates aggregated statistics (方案3: 聚合统计表).
// This function no longer saves to block_signers table to reduce data volume by 99.8%.
//
// Parameters:
//   - block: The Tendermint ResultBlock containing block and commit information
//
// Returns:
//   - error: Error if update fails. Returns an error if proposer validator cannot be found,
//     as this is critical information that should not be silently ignored.
func (s *BlockSignerService) SaveBlockSigners(block *coretypes.ResultBlock) error {
	if block == nil || block.Block == nil {
		return fmt.Errorf("block is nil")
	}

	height := uint64(block.Block.Height)
	proposerHex := block.Block.Header.ProposerAddress.String()

	// Get proposer validator info - fetch from chain if not in local DB
	// This is critical information - if we can't find the proposer, we should return an error
	// Pass block height for historical query when proposer may have unbonded
	proposerValidator, err := s.getOrFetchValidator(proposerHex, height)
	if err != nil {
		return fmt.Errorf("cannot find validator for proposer consensus address %s at height %d: %w", proposerHex, height, err)
	}
	proposerOperatorAddress := proposerValidator.OperatorAddress

	// Collect signers information
	signers := make([]SignerInfo, 0)

	// Add proposer
	signers = append(signers, SignerInfo{
		ValidatorOperatorAddress: proposerOperatorAddress,
		IsProposer:               true,
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
			// For signers, we log a warning but continue processing other signers
			// as missing a signer is less critical than missing the proposer
			// Pass block height for historical query when signer may have unbonded
			signerValidator, err := s.getOrFetchValidator(signerBech32Addr, height)
			if err != nil {
				global.Logger.Warnf("cannot find validator for signer consensus address %s (bech32: %s) at height %d: %v", signerConsensusHex, signerBech32Addr, height, err)
				continue // Skip this signer if not found
			}
			signerOperatorAddress := signerValidator.OperatorAddress

			// Only add if signature is not nil (validator actually signed)
			if sig.Signature != nil {
				signers = append(signers, SignerInfo{
					ValidatorOperatorAddress: signerOperatorAddress,
					IsProposer:               false,
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
					true, // signed=true (they're in the signers list)
					true, // shouldSign=true (they signed, so they should have signed)
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

// SaveBlockSigners is a convenience function that uses a default service instance.
// This maintains backward compatibility but is deprecated. Use BlockSignerService instead.
//
// Deprecated: Use BlockSignerService.SaveBlockSigners instead for better dependency injection.
var defaultBlockSignerService *BlockSignerService
var defaultServiceMutex sync.Mutex

// SetDefaultBlockSignerService sets the default service instance for backward compatibility.
func SetDefaultBlockSignerService(service *BlockSignerService) {
	defaultServiceMutex.Lock()
	defer defaultServiceMutex.Unlock()
	defaultBlockSignerService = service
}

// SaveBlockSigners is the legacy function signature maintained for backward compatibility.
// It uses the default service instance if available, otherwise returns an error.
//
// Deprecated: Use BlockSignerService.SaveBlockSigners instead.
func SaveBlockSigners(block *coretypes.ResultBlock) error {
	defaultServiceMutex.Lock()
	service := defaultBlockSignerService
	defaultServiceMutex.Unlock()

	if service == nil {
		return fmt.Errorf("default block signer service not initialized, use BlockSignerService directly")
	}

	return service.SaveBlockSigners(block)
}
