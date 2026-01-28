// Package service provides validator statistics calculation services.
// This file implements validator block statistics (proposed, signed, missed blocks).
package service

import (
	"context"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/pkg/errors"
)

// ValidatorStatsService handles validator statistics calculation and updates.
type ValidatorStatsService struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// NewValidatorStatsService creates a new validator statistics service.
func NewValidatorStatsService(ctx context.Context) *ValidatorStatsService {
	statsCtx, cancel := context.WithCancel(ctx)
	return &ValidatorStatsService{
		ctx:    statsCtx,
		cancel: cancel,
	}
}

// Start starts the validator statistics update service.
// It periodically updates validator statistics (proposed, signed, missed blocks).
func (s *ValidatorStatsService) Start() {
	global.Logger.Infof("starting validator statistics service")

	// Run initial update
	go s.updateValidatorStats()

	// Schedule periodic updates (every 10 minutes)
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			global.Logger.Infof("validator statistics service stopped")
			return
		case <-ticker.C:
			go s.updateValidatorStats()
		}
	}
}

// Stop stops the validator statistics service.
func (s *ValidatorStatsService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// updateValidatorStats updates validator statistics from aggregated statistics table.
// It calculates proposed, signed, and missed blocks for each validator.
func (s *ValidatorStatsService) updateValidatorStats() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	global.Logger.Debugf("starting validator statistics update")

	// Get all active validators
	var validators []models.Validator
	err := db.Get().WithContext(ctx).Where("deleted_at IS NULL").Find(&validators).Error
	if err != nil {
		global.Logger.Errorf("failed to get validators for statistics update: %v", err)
		return
	}

	if len(validators) == 0 {
		global.Logger.Debugf("no validators found for statistics update")
		return
	}

	global.Logger.Debugf("updating statistics for %d validators", len(validators))

	// Update statistics for each validator
	for _, validator := range validators {
		if err := s.updateValidatorStat(ctx, validator.OperatorAddress); err != nil {
			global.Logger.Warnf("failed to update statistics for validator %s: %v", validator.OperatorAddress, err)
			// Continue with other validators
		}
	}

	global.Logger.Debugf("completed validator statistics update")
}

// updateValidatorStat updates statistics for a single validator.
// Uses aggregated statistics table (solution 3) for optimal performance.
func (s *ValidatorStatsService) updateValidatorStat(ctx context.Context, operatorAddress string) error {
	// Get aggregated statistics from validator_block_stats table (solution 3: aggregated statistics table)
	proposed, signed, missed, err := models.GetValidatorStatsAggregated(operatorAddress)
	if err != nil {
		// If aggregated stats not available, use validator table stats (updated by this service)
		global.Logger.Warnf("failed to get aggregated stats for %s: %v, using validator table stats", operatorAddress, err)
		
		// Get validator to use its cached statistics
		var validator models.Validator
		err = db.Get().WithContext(ctx).
			Where("operator_address = ?", operatorAddress).
			First(&validator).Error
		if err != nil {
			return errors.Wrapf(err, "failed to get validator %s", operatorAddress)
		}

		proposed = validator.Proposed
		signed = validator.Signed
		missed = validator.Missed
	}

	// Update validator statistics
	err = db.Get().WithContext(ctx).Model(&models.Validator{}).
		Where("operator_address = ?", operatorAddress).
		Updates(map[string]interface{}{
			"proposed": proposed,
			"signed":   signed,
			"missed":   missed,
			"updated_at": time.Now(),
		}).Error

	if err != nil {
		return errors.Wrapf(err, "failed to update validator statistics for %s", operatorAddress)
	}

	global.Logger.Debugf("updated statistics for validator %s: proposed=%d, signed=%d, missed=%d",
		operatorAddress, proposed, signed, missed)

	return nil
}

// CalculateMissedBlocksForValidator is a placeholder for accurate missed block calculation.
// Accurate calculation requires validator set information at each block height,
// which is not yet implemented. Currently always returns 0.
func (s *ValidatorStatsService) CalculateMissedBlocksForValidator(ctx context.Context, operatorAddress string, startHeight, endHeight uint64) (uint64, error) {
	// Accurate missed block calculation requires:
	// 1. Validator set at each block height
	// 2. Check if validator was in active set
	// 3. Count blocks where validator was in set but didn't sign
	// This functionality is not yet implemented; returns 0 as placeholder.
	return 0, nil
}
