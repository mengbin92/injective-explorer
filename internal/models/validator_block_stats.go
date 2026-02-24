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
	"gorm.io/gorm/clause"
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

	// Build record with increments (1 or 0 for new record)
	var proposedInc, signedInc, shouldSignInc uint64
	if isProposer {
		proposedInc = 1
	}
	if signed {
		signedInc = 1
	}
	if shouldSign {
		shouldSignInc = 1
	}
	missedInc := uint64(0)
	if shouldSign && !signed {
		missedInc = 1
	}

	newStats := ValidatorBlockStats{
		ValidatorOperatorAddress: validatorOperatorAddress,
		PeriodStartHeight:        periodStart,
		PeriodEndHeight:          periodEnd,
		ProposedCount:            proposedInc,
		SignedCount:              signedInc,
		ShouldSignCount:          shouldSignInc,
		MissedCount:              missedInc,
	}

	// Use upsert to avoid duplicate key errors when multiple workers process the same period
	err := db.Get().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "validator_operator_address"}, {Name: "period_start_height"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"period_end_height":   periodEnd,
			"proposed_count":     gorm.Expr("proposed_count + ?", proposedInc),
			"signed_count":       gorm.Expr("signed_count + ?", signedInc),
			"should_sign_count":  gorm.Expr("should_sign_count + ?", shouldSignInc),
			"missed_count":       gorm.Expr("should_sign_count - signed_count"),
			"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}),
	}).Create(&newStats).Error

	if err != nil {
		global.Logger.Errorf("failed to upsert validator stats for %s at height %d: %v", validatorOperatorAddress, blockHeight, err)
		return fmt.Errorf("failed to upsert validator stats: %w", err)
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

