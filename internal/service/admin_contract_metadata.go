// Package service provides Admin API for ERC20 contract metadata management.
package service

import (
	"context"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"gorm.io/gorm"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/internal/service/metrics"
	"injective-explorer/internal/utils"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/db"
)

// RefreshContractMetadata refreshes metadata for a single contract.
func (s *AdminService) RefreshContractMetadata(ctx context.Context, req *pb.RefreshContractMetadataRequest) (*pb.Reply, error) {
	if err := utils.ValidateRequiredFields("contract_address", req.GetContractAddress()); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}
	contractAddress := req.GetContractAddress()
	chainID := req.GetChainId()
	if chainID == "" {
		chainID = global.ChainConfig.ChainId
	}
	if chainID == "" {
		chainID = DefaultChainID
	}

	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := RefreshContractMetadataStandalone(queryCtx, contractAddress, chainID, nil, true)
	if err != nil {
		global.Logger.Warnf("failed to refresh metadata for contract_address=%s: %v", strings.ToLower(contractAddress), err)
		return ErrorReply(http.StatusInternalServerError, "failed to refresh metadata", err), err
	}

	var ci models.ContractInfo
	if err := db.Get().WithContext(ctx).Where("contract_address = ? AND chain_id = ?", contractAddress, chainID).First(&ci).Error; err != nil {
		return ErrorReply(http.StatusInternalServerError, "failed to query contract", err), err
	}

	data := map[string]interface{}{
		"contract_address":  ci.ContractAddress,
		"chain_id":          ci.ChainID,
		"token_name":        ci.TokenName,
		"token_symbol":      ci.TokenSymbol,
		"decimals":          ci.Decimals,
		"metadata_source":   ci.MetadataSource,
		"metadata_updated_at": ci.MetadataUpdatedAt,
		"updated":           true,
	}
	return MarshalAndReply(data, "marshal RefreshContractMetadata response error")
}

// BackfillContractMetadata batch backfills metadata for contracts with missing data.
// When onlyPeggy=true and sync mode, uses JSON-RPC batch for Ethereum to reduce round-trips.
func (s *AdminService) BackfillContractMetadata(ctx context.Context, req *pb.BackfillContractMetadataRequest) (*pb.Reply, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	chainID := req.GetChainId()
	if chainID == "" {
		chainID = global.ChainConfig.ChainId
	}
	async := req.GetAsync()
	onlyPeggy := req.GetOnlyPeggy()

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := db.Get().WithContext(queryCtx).Model(&models.ContractInfo{}).
		Where("contract_type IN ?", []string{"erc20", "solidity"}).
		Where("(token_name IS NULL OR token_name = '' OR token_symbol IS NULL OR token_symbol = '')").
		Where("(metadata_not_applicable = ? OR metadata_not_applicable IS NULL)", false)
	if chainID != "" {
		query = query.Where("chain_id = ?", chainID)
	}
	if onlyPeggy {
		query = query.Where("is_peggy_token = ?", true)
	}
	query = query.Limit(limit)

	selectFields := "contract_address, chain_id"
	if onlyPeggy {
		selectFields = "contract_address, chain_id, peggy_denom"
	}
	var contracts []models.ContractInfo
	if err := query.Select(selectFields).Find(&contracts).Error; err != nil {
		return ErrorReply(http.StatusInternalServerError, "failed to query contracts", err), err
	}

	if len(contracts) == 0 {
		data := &pb.BackfillContractMetadataDTO{Queued: 0, Processed: 0, Succeeded: 0, Failed: 0}
		return MarshalAndReply(data, "marshal BackfillContractMetadata response error")
	}

	if async {
		concurrency := 10
		if len(contracts) < concurrency {
			concurrency = len(contracts)
		}
		var wg sync.WaitGroup
		sem := make(chan struct{}, concurrency)
		for _, c := range contracts {
			wg.Add(1)
			sem <- struct{}{}
			go func(addr, cid string) {
				defer wg.Done()
				defer func() { <-sem }()
				ctx2, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_ = RefreshContractMetadataStandalone(ctx2, addr, cid, nil, true)
			}(c.ContractAddress, c.ChainID)
		}
		wg.Wait()
		data := &pb.BackfillContractMetadataDTO{Queued: int32(len(contracts)), Processed: int32(len(contracts)), Succeeded: 0, Failed: 0}
		return MarshalAndReply(data, "marshal BackfillContractMetadata response error")
	}

	// Sync mode: use batch for Peggy tokens when possible
	if onlyPeggy && len(contracts) > 1 {
		processed, succeeded, failed := backfillPeggyMetadataBatch(ctx, chainID, contracts)
		data := &pb.BackfillContractMetadataDTO{
			Queued:    int32(len(contracts)),
			Processed: int32(processed),
			Succeeded: int32(succeeded),
			Failed:    int32(failed),
		}
		return MarshalAndReply(data, "marshal BackfillContractMetadata response error")
	}

	// Sync mode: single contract or non-Peggy
	processed, succeeded, failed := 0, 0, 0
	refreshCtx, refreshCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer refreshCancel()

	for _, c := range contracts {
		err := RefreshContractMetadataStandalone(refreshCtx, c.ContractAddress, c.ChainID, nil, true)
		processed++
		if err != nil {
			failed++
			global.Logger.Warnf("backfill failed for %s: %v", c.ContractAddress, err)
		} else {
			succeeded++
		}
	}

	data := &pb.BackfillContractMetadataDTO{
		Queued:    int32(len(contracts)),
		Processed: int32(processed),
		Succeeded: int32(succeeded),
		Failed:    int32(failed),
	}
	return MarshalAndReply(data, "marshal BackfillContractMetadata response error")
}

// GetContractMetadataStats returns metadata statistics.
func (s *AdminService) GetContractMetadataStats(ctx context.Context, req *pb.GetContractMetadataStatsRequest) (*pb.Reply, error) {
	chainID := req.GetChainId()
	if chainID == "" {
		chainID = global.ChainConfig.ChainId
	}

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var missingCount, notERC20Count int64
	missingQ := db.Get().WithContext(queryCtx).Model(&models.ContractInfo{}).
		Where("contract_type IN ?", []string{"erc20", "solidity"}).
		Where("(token_name IS NULL OR token_name = '' OR token_symbol IS NULL OR token_symbol = '')").
		Where("(metadata_not_applicable = ? OR metadata_not_applicable IS NULL)", false)
	notERC20Q := db.Get().WithContext(queryCtx).Model(&models.ContractInfo{}).
		Where("contract_type IN ?", []string{"erc20", "solidity"}).
		Where("metadata_not_applicable = ?", true)
	if chainID != "" {
		missingQ = missingQ.Where("chain_id = ?", chainID)
		notERC20Q = notERC20Q.Where("chain_id = ?", chainID)
	}
	missingQ.Count(&missingCount)
	notERC20Q.Count(&notERC20Count)

	metrics.SetContractsMissingMetadata(float64(missingCount))
	metrics.SetContractsNotERC20(float64(notERC20Count))

	data := &pb.ContractMetadataStatsDTO{
		MissingCount:  missingCount,
		NotErc20Count: notERC20Count,
		ChainId:       chainID,
	}
	return MarshalAndReply(data, "marshal GetContractMetadataStats response error")
}

// UpdateContractMetadataMetrics updates Prometheus metrics (contracts_missing_metadata_total,
// contracts_not_erc20_total) from DB. Call after backfill or periodically. chainID empty = all chains.
func UpdateContractMetadataMetrics(ctx context.Context, chainID string) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var missingCount, notERC20Count int64
	missingQ := db.Get().WithContext(queryCtx).Model(&models.ContractInfo{}).
		Where("contract_type IN ?", []string{"erc20", "solidity"}).
		Where("(token_name IS NULL OR token_name = '' OR token_symbol IS NULL OR token_symbol = '')").
		Where("(metadata_not_applicable = ? OR metadata_not_applicable IS NULL)", false)
	notERC20Q := db.Get().WithContext(queryCtx).Model(&models.ContractInfo{}).
		Where("contract_type IN ?", []string{"erc20", "solidity"}).
		Where("metadata_not_applicable = ?", true)
	if chainID != "" {
		missingQ = missingQ.Where("chain_id = ?", chainID)
		notERC20Q = notERC20Q.Where("chain_id = ?", chainID)
	}
	_ = missingQ.Count(&missingCount)
	_ = notERC20Q.Count(&notERC20Count)
	metrics.SetContractsMissingMetadata(float64(missingCount))
	metrics.SetContractsNotERC20(float64(notERC20Count))
}

// RefreshPeggyMetadata refreshes metadata for a Peggy token specifically.
func (s *AdminService) RefreshPeggyMetadata(ctx context.Context, req *pb.RefreshPeggyMetadataRequest) (*pb.Reply, error) {
	if err := utils.ValidateRequiredFields("contract_address", req.GetContractAddress()); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}
	contractAddress := req.GetContractAddress()
	chainID := global.ChainConfig.ChainId
	if chainID == "" {
		chainID = DefaultChainID
	}

	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := RefreshContractMetadataStandalone(queryCtx, contractAddress, chainID, nil, true)
	if err != nil {
		global.Logger.Warnf("failed to refresh Peggy metadata for contract_address=%s: %v", strings.ToLower(contractAddress), err)
		return ErrorReply(http.StatusInternalServerError, "failed to refresh Peggy metadata", err), err
	}

	var ci models.ContractInfo
	if err := db.Get().WithContext(ctx).Where("contract_address = ? AND chain_id = ?", contractAddress, chainID).First(&ci).Error; err != nil {
		return ErrorReply(http.StatusInternalServerError, "failed to query contract", err), err
	}

	data := &pb.RefreshPeggyMetadataDTO{
		ContractAddress:  ci.ContractAddress,
		TokenName:        ci.TokenName,
		TokenSymbol:      ci.TokenSymbol,
		Decimals:         int32(ci.Decimals),
		MetadataSource:   ci.MetadataSource,
		IsPeggy:          ci.IsPeggyToken,
		OriginalDenom:    ci.PeggyDenom,
	}
	return MarshalAndReply(data, "marshal RefreshPeggyMetadata response error")
}

// ClearContractMetadataCache clears all contract metadata Redis cache (for periodic full refresh).
func (s *AdminService) ClearContractMetadataCache(ctx context.Context, _ *pb.ClearContractMetadataCacheRequest) (*pb.Reply, error) {
	deleted, err := DeleteContractMetadataCacheByPrefix(ctx)
	if err != nil {
		global.Logger.Warnf("failed to clear contract metadata cache: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to clear metadata cache", err), err
	}
	data := map[string]interface{}{
		"deleted_keys": deleted,
	}
	return MarshalAndReply(data, "marshal ClearContractMetadataCache response error")
}

// backfillPeggyMetadataBatch uses GetERC20MetadataBatch to fetch multiple Peggies in one RPC batch.
func backfillPeggyMetadataBatch(ctx context.Context, chainID string, contracts []models.ContractInfo) (processed, succeeded, failed int) {
	bc := global.BootstrapConfig
	if bc == nil || bc.Chain == nil || bc.Chain.Ethereum == nil || !bc.Chain.Ethereum.Enabled {
		for range contracts {
			processed++
			failed++
		}
		return processed, succeeded, failed
	}
	endpoints := getEthereumRpcEndpoints(bc)
	if len(endpoints) == 0 {
		for range contracts {
			processed++
			failed++
		}
		return processed, succeeded, failed
	}
	// Pre-allocate slices with known capacity to avoid multiple allocations
	originalAddrs := make([]string, 0, len(contracts))
	contractIndex := make([]int, 0, len(contracts))
	for i, c := range contracts {
		denom := c.PeggyDenom
		if denom == "" && c.Metadata != nil {
			if d, ok := c.Metadata["bank_denom"].(string); ok {
				denom = d
			}
		}
		orig := strings.TrimPrefix(denom, "peggy")
		if orig == "" || len(orig) < 42 {
			// No valid Ethereum address in denom, fallback to standard flow
			processed++
			if err := RefreshContractMetadataStandalone(ctx, c.ContractAddress, chainID, nil, true); err != nil {
				failed++
			} else {
				succeeded++
			}
			continue
		}
		if !strings.HasPrefix(orig, "0x") {
			orig = "0x" + orig
		}
		originalAddrs = append(originalAddrs, orig)
		contractIndex = append(contractIndex, i)
	}
	if len(originalAddrs) == 0 {
		return processed, succeeded, failed
	}
	cfg := bc.Chain.Ethereum
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = cfg.Timeout.AsDuration()
	}
	chainIDBig := big.NewInt(cfg.ChainId)
	if chainIDBig.Int64() == 0 {
		chainIDBig = big.NewInt(1)
	}
	client, err := chain.NewEVMClientWithConfig(&chain.EVMConfig{
		RPCURL:  endpoints[0],
		ChainID: chainIDBig,
		Network: "ethereum",
		Timeout: timeout,
	})
	if err != nil {
		global.Logger.Warnf("backfill Peggy batch: failed to create Ethereum client: %v", err)
		return len(contracts), 0, len(contracts)
	}
	defer client.Close()
	results, err := client.GetERC20MetadataBatch(ctx, originalAddrs)
	if err != nil {
		global.Logger.Warnf("backfill Peggy batch failed: %v", err)
		return len(contracts), 0, len(contracts)
	}
	for i, r := range results {
		processed++
		ci := contracts[contractIndex[i]]
		if r.Error != nil || r.Name == "" || r.Symbol == "" {
			metrics.RecordContractMetadataFetch("failure", "batch")
			failed++
			global.Logger.Debugf("batch fetch failed for contract_address=%s, falling back to standard", strings.ToLower(ci.ContractAddress))
			if fallbackErr := RefreshContractMetadataStandalone(ctx, ci.ContractAddress, chainID, nil, true); fallbackErr == nil {
				failed--
				succeeded++
			}
			continue
		}
		name, symbol, validateErr := validateERC20Metadata(r.Name, r.Symbol, r.Decimals)
		if validateErr != nil {
			failed++
			continue
		}
		updates := map[string]interface{}{
			"name":                 name,
			"token_name":           name,
			"token_symbol":         symbol,
			"decimals":             r.Decimals,
			"metadata_source":      "ethereum",
			"metadata_updated_at":  time.Now(),
			"version":              gorm.Expr("version + 1"),
		}
		var cur models.ContractInfo
		if dbErr := db.Get().WithContext(ctx).Select("version").Where("contract_address = ? AND chain_id = ?", ci.ContractAddress, chainID).First(&cur).Error; dbErr != nil {
			failed++
			continue
		}
		result := db.Get().WithContext(ctx).Model(&models.ContractInfo{}).
			Where("contract_address = ? AND chain_id = ? AND version = ?", ci.ContractAddress, chainID, cur.Version).
			Updates(updates)
		if result.Error != nil {
			global.Logger.Warnf("backfill batch update failed for contract_address=%s: %v", strings.ToLower(ci.ContractAddress), result.Error)
			failed++
			continue
		}
		if result.RowsAffected == 0 {
			failed++
			continue
		}
		setContractMetadataToCache(ctx, chainID, ci.ContractAddress, &ERC20MetadataCached{
			Name: name, Symbol: symbol, Decimals: r.Decimals, Source: "ethereum",
		})
		metrics.RecordContractMetadataFetch("success", "ethereum")
		succeeded++
	}
	return processed, succeeded, failed
}

// BackfillOptions configures metadata backfill execution (for CLI/scheduler).
type BackfillOptions struct {
	Limit   int           // max contracts to process (default 50)
	ChainID string        // empty = use global.ChainConfig.ChainId
	DryRun  bool          // if true, only count and log, no actual refresh
	Delay   time.Duration // delay between each contract for RPC rate limiting
}

// RunBackfillMetadata runs metadata backfill with given options.
// Returns processed, succeeded, failed counts.
func RunBackfillMetadata(ctx context.Context, opt BackfillOptions) (processed, succeeded, failed int) {
	if opt.Limit <= 0 {
		opt.Limit = 50
	}
	chainID := opt.ChainID
	if chainID == "" {
		chainID = global.ChainConfig.ChainId
	}
	if chainID == "" {
		chainID = DefaultChainID
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var contracts []models.ContractInfo
	if err := db.Get().WithContext(queryCtx).Model(&models.ContractInfo{}).
		Where("contract_type IN ?", []string{"erc20", "solidity"}).
		Where("(token_name IS NULL OR token_name = '' OR token_symbol IS NULL OR token_symbol = '')").
		Where("(metadata_not_applicable = ? OR metadata_not_applicable IS NULL)", false).
		Where("chain_id = ?", chainID).
		Select("contract_address, chain_id, peggy_denom").
		Limit(opt.Limit).
		Find(&contracts).Error; err != nil {
		return 0, 0, 0
	}
	if len(contracts) == 0 {
		return 0, 0, 0
	}
	if opt.DryRun {
		return len(contracts), len(contracts), 0
	}
	// Use batch path only when all contracts have peggy_denom/bank_denom for CoinGecko lookup.
	onlyPeggy := true
	for _, c := range contracts {
		denom := c.PeggyDenom
		if denom == "" && c.Metadata != nil {
			if d, ok := c.Metadata["bank_denom"].(string); ok {
				denom = d
			}
		}
		if denom == "" {
			onlyPeggy = false
			break
		}
	}
	if onlyPeggy && len(contracts) > 1 {
		return backfillPeggyMetadataBatch(ctx, chainID, contracts)
	}
	refreshCtx, refreshCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer refreshCancel()
	for _, c := range contracts {
		if opt.Delay > 0 {
			select {
			case <-ctx.Done():
				return processed, succeeded, failed
			case <-time.After(opt.Delay):
			}
		}
		processed++
		if err := RefreshContractMetadataStandalone(refreshCtx, c.ContractAddress, c.ChainID, nil, true); err != nil {
			failed++
		} else {
			succeeded++
		}
	}
	return processed, succeeded, failed
}

// RunScheduledMetadataBackfill runs metadata backfill for the scheduler (convenience wrapper).
func RunScheduledMetadataBackfill(ctx context.Context, limit int) (processed, succeeded, failed int) {
	return RunBackfillMetadata(ctx, BackfillOptions{Limit: limit})
}
