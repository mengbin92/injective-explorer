// Package models provides data models for blockchain entities.
// This file defines the ValidatorBlockStats model for aggregated validator statistics.
package models

import (
	"injective-explorer/internal/global"
	"injective-explorer/provider/db"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// ValidatorBlockStats represents aggregated validator block statistics for a period.
// This is the most optimized approach: only stores aggregated counts, not individual block records.
type ValidatorBlockStats struct {
	gorm.Model

	ValidatorOperatorAddress string `json:"validator_operator_address" gorm:"column:validator_operator_address;index"`
	PeriodStartHeight        uint64 `json:"period_start_height" gorm:"column:period_start_height"`
	PeriodEndHeight          uint64 `json:"period_end_height" gorm:"column:period_end_height"`
	ProposedCount            uint64 `json:"proposed_count" gorm:"column:proposed_count"`
	SignedCount              uint64 `json:"signed_count" gorm:"column:signed_count"`
	ShouldSignCount          uint64 `json:"should_sign_count" gorm:"column:should_sign_count"`
	MissedCount              uint64 `json:"missed_count" gorm:"column:missed_count"`
}

// TableName returns the table name for ValidatorBlockStats model.
func (ValidatorBlockStats) TableName() string {
	return "validator_block_stats"
}

// PeriodSize defines the number of blocks per statistics period.
// Statistics are aggregated every PeriodSize blocks.
const PeriodSize = 1000

// GetCurrentPeriodStartHeight calculates the start height of the current period for a given block height.
func GetCurrentPeriodStartHeight(blockHeight uint64) uint64 {
	return (blockHeight / PeriodSize) * PeriodSize
}

// GetCurrentPeriodEndHeight calculates the end height of the current period for a given block height.
func GetCurrentPeriodEndHeight(blockHeight uint64) uint64 {
	return GetCurrentPeriodStartHeight(blockHeight) + PeriodSize - 1
}

// UpdateValidatorStatsForBlock updates validator statistics for a single block.
// This increments the counters for the validator in the current period.
//
// Parameters:
//   - validatorOperatorAddress: Validator operator address
//   - blockHeight: Block height
//   - isProposer: Whether validator is the proposer
//   - signed: Whether validator signed the block
//   - shouldSign: Whether validator should have signed (was in active validator set)
//
// Returns:
//   - error: Error if update fails
// UpdateValidatorStatsForBlock updates validator statistics for a single block.
// This increments the counters for the validator in the current period.
// Only accepts bech32 format validator operator addresses (e.g., injvaloper1...).
func UpdateValidatorStatsForBlock(validatorOperatorAddress string, blockHeight uint64, isProposer, signed, shouldSign bool) error {
	if validatorOperatorAddress == "" {
		return fmt.Errorf("validator operator address is required")
	}

	// Validate address format - must be bech32 operator address, not hex
	valoperPrefix := sdk.GetConfig().GetBech32ValidatorAddrPrefix()
	if !strings.HasPrefix(validatorOperatorAddress, valoperPrefix) {
		return fmt.Errorf("invalid validator operator address format: %s (must be bech32 format starting with '%s')", validatorOperatorAddress, valoperPrefix)
	}

	periodStart := GetCurrentPeriodStartHeight(blockHeight)
	periodEnd := GetCurrentPeriodEndHeight(blockHeight)

	// First, try to get existing record
	var existingStats ValidatorBlockStats
	err := db.Get().
		Where("validator_operator_address = ? AND period_start_height = ?", validatorOperatorAddress, periodStart).
		First(&existingStats).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check existing stats: %w", err)
	}

	// Prepare updates
	updates := map[string]interface{}{
		"period_end_height": periodEnd,
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new record
		newStats := ValidatorBlockStats{
			ValidatorOperatorAddress: validatorOperatorAddress,
			PeriodStartHeight:        periodStart,
			PeriodEndHeight:          periodEnd,
			ProposedCount:            0,
			SignedCount:              0,
			ShouldSignCount:          0,
			MissedCount:              0,
		}

		if isProposer {
			newStats.ProposedCount = 1
		}
		if signed {
			newStats.SignedCount = 1
		}
		if shouldSign {
			newStats.ShouldSignCount = 1
			// Calculate missed: should_sign - signed
			if signed {
				newStats.MissedCount = 0
			} else {
				newStats.MissedCount = 1
			}
		}

		err = db.Get().Create(&newStats).Error
		if err != nil {
			// Handle duplicate entry race condition - another goroutine may have created it
			if isDuplicateEntryError(err) {
				// Try to update existing record instead
				return updateExistingStats(db.Get(), validatorOperatorAddress, periodStart, periodEnd, isProposer, signed, shouldSign)
			}
			global.Logger.Errorf("failed to create validator stats for %s at height %d: %v", validatorOperatorAddress, blockHeight, err)
			return fmt.Errorf("failed to create validator stats: %w", err)
		}
		return nil
	}

	// Update existing record
	if isProposer {
		updates["proposed_count"] = gorm.Expr("proposed_count + ?", 1)
	}
	if signed {
		updates["signed_count"] = gorm.Expr("signed_count + ?", 1)
	}
	if shouldSign {
		updates["should_sign_count"] = gorm.Expr("should_sign_count + ?", 1)
	}
	// Calculate missed count: should_sign_count - signed_count
	updates["missed_count"] = gorm.Expr("should_sign_count - signed_count")

	err = db.Get().Model(&existingStats).Updates(updates).Error
	if err != nil {
		global.Logger.Errorf("failed to update validator stats for %s at height %d: %v", validatorOperatorAddress, blockHeight, err)
		return fmt.Errorf("failed to update validator stats: %w", err)
	}

	return nil
}

// GetValidatorStatsAggregated retrieves aggregated statistics for a validator across all periods.
//
// Parameters:
//   - validatorOperatorAddress: Validator operator address
//
// Returns:
//   - proposed: Total proposed blocks
//   - signed: Total signed blocks
//   - missed: Total missed blocks
//   - error: Error if query fails
func GetValidatorStatsAggregated(validatorOperatorAddress string) (proposed, signed, missed uint64, err error) {
	var result struct {
		Proposed uint64
		Signed   uint64
		Missed   uint64
	}

	err = db.Get().Model(&ValidatorBlockStats{}).
		Select("SUM(proposed_count) as proposed, SUM(signed_count) as signed, SUM(missed_count) as missed").
		Where("validator_operator_address = ?", validatorOperatorAddress).
		Scan(&result).Error

	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get aggregated validator stats: %w", err)
	}

	return result.Proposed, result.Signed, result.Missed, nil
}

// GetValidatorStatsForPeriod retrieves statistics for a validator in a specific period.
//
// Parameters:
//   - validatorOperatorAddress: Validator operator address
//   - periodStartHeight: Period start height
//
// Returns:
//   - *ValidatorBlockStats: Statistics for the period, or nil if not found
//   - error: Error if query fails
func GetValidatorStatsForPeriod(validatorOperatorAddress string, periodStartHeight uint64) (*ValidatorBlockStats, error) {
	var stats ValidatorBlockStats
	err := db.Get().
		Where("validator_operator_address = ? AND period_start_height = ?", validatorOperatorAddress, periodStartHeight).
		First(&stats).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get validator stats for period: %w", err)
	}

	return &stats, nil
}

// isDuplicateEntryError checks if an error is a MySQL duplicate entry error.
func isDuplicateEntryError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "Error 1062") || strings.Contains(errStr, "Duplicate entry")
}

// updateExistingStats updates an existing validator stats record.
// This is used when a duplicate entry race condition occurs.
func updateExistingStats(tx *gorm.DB, validatorOperatorAddress string, periodStart, periodEnd uint64, isProposer, signed, shouldSign bool) error {
	updates := map[string]interface{}{
		"period_end_height": periodEnd,
	}

	if isProposer {
		updates["proposed_count"] = gorm.Expr("proposed_count + ?", 1)
	}
	if signed {
		updates["signed_count"] = gorm.Expr("signed_count + ?", 1)
	}
	if shouldSign {
		updates["should_sign_count"] = gorm.Expr("should_sign_count + ?", 1)
	}
	// Calculate missed count: should_sign_count - signed_count
	updates["missed_count"] = gorm.Expr("should_sign_count - signed_count")

	result := tx.Model(&ValidatorBlockStats{}).
		Where("validator_operator_address = ? AND period_start_height = ?", validatorOperatorAddress, periodStart).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update validator stats: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no validator stats record found for update")
	}
	return nil
}
