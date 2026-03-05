// Package service provides EVM transaction details synchronization service.
package service

import (
	"context"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"
)

// periodicContractTotalSupplyUpdateWorker periodically updates totalSupply for existing contracts
// that don't have it set yet.
func (s *EVMDetailsSyncService) periodicContractTotalSupplyUpdateWorker() {
	defer s.wg.Done()

	// Check every 10 minutes for contracts needing totalSupply update
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// Run immediately on startup
	s.updateContractTotalSupply()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.updateContractTotalSupply()
		}
	}
}

// updateContractTotalSupply queries contracts without totalSupply and updates them from the chain.
func (s *EVMDetailsSyncService) updateContractTotalSupply() {
	if s.evmClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	// Find contracts without totalSupply - limit to 50 per batch to avoid overloading
	var contracts []models.ContractInfo
	err := db.Get().WithContext(ctx).
		Where("total_supply IS NULL OR total_supply = ''").
		Where("contract_type = ?", "solidity").
		Limit(50).
		Find(&contracts).Error

	if err != nil {
		global.Logger.Warnf("failed to query contracts without totalSupply: %v", err)
		return
	}

	if len(contracts) == 0 {
		return
	}

	global.Logger.Infof("found %d contracts without totalSupply, attempting to update", len(contracts))

	// Update each contract
	successCount := 0
	for _, contract := range contracts {
		if err := s.updateContractTotalSupplyFromChain(ctx, &contract); err != nil {
			global.Logger.Warnf("failed to update totalSupply for contract %s: %v", contract.ContractAddress, err)
		} else {
			successCount++
		}
		// Small delay to avoid overwhelming the RPC
		time.Sleep(100 * time.Millisecond)
	}

	if successCount > 0 {
		global.Logger.Infof("successfully updated totalSupply for %d/%d contracts", successCount, len(contracts))
	}
}

// updateContractTotalSupplyFromChain fetches totalSupply from the contract and updates the database.
func (s *EVMDetailsSyncService) updateContractTotalSupplyFromChain(ctx context.Context, contract *models.ContractInfo) error {
	if s.evmClient == nil {
		return nil
	}

	// Query totalSupply from the contract
	totalSupply, err := s.evmClient.GetERC20TotalSupply(ctx, contract.ContractAddress)
	if err != nil {
		return err
	}

	if totalSupply == "" {
		return nil // Contract might not implement totalSupply
	}

	// Update the contract in database
	err = db.Get().WithContext(ctx).
		Model(contract).
		Where("id = ?", contract.ID).
		Update("total_supply", totalSupply).Error

	if err != nil {
		return err
	}

	global.Logger.Debugf("updated totalSupply for contract %s: %s", contract.ContractAddress, totalSupply)
	return nil
}
