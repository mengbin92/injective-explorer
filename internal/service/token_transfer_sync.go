// Package service provides token transfer synchronization service.
package service

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/db"

	"github.com/ethereum/go-ethereum"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TokenTransferSyncService handles token transfer synchronization from EVM logs.
type TokenTransferSyncService struct {
	ctx              context.Context
	cancel           context.CancelFunc
	config           *TokenTransferSyncConfig
	evmClient        *chain.EVMClient
	wg               sync.WaitGroup
	syncMutex        sync.Mutex
	isRunning        bool
	cleanupTicker    *time.Ticker // Ticker for periodic cleanup
	retentionDays    int          // How many days to retain transfer data
}

// TokenTransferSyncConfig holds the token transfer sync service configuration.
type TokenTransferSyncConfig struct {
	WorkerCount        int
	BatchSize          int
	ProcessInterval    time.Duration
	Timeout            time.Duration
	StartBlock         uint64 // Optional: start from specific block
	EnableRealtimeSync bool
}

// DefaultTokenTransferSyncConfig returns default configuration for token transfer sync
func DefaultTokenTransferSyncConfig() *TokenTransferSyncConfig {
	return &TokenTransferSyncConfig{
		WorkerCount:        2,
		BatchSize:          100,
		ProcessInterval:    30 * time.Second,
		Timeout:            60 * time.Second,
		StartBlock:         0,
		EnableRealtimeSync: true,
	}
}

// LoadTokenTransferSyncConfig loads token transfer sync configuration from bootstrap config
func LoadTokenTransferSyncConfig(bc *conf.Bootstrap) *TokenTransferSyncConfig {
	config := DefaultTokenTransferSyncConfig()

	if bc == nil {
		return config
	}

	// Load from sync config if available
	if bc.Sync != nil {
		if bc.Sync.BatchSize != nil {
			if bc.Sync.BatchSize.HistoricalSync > 0 {
				config.BatchSize = int(bc.Sync.BatchSize.HistoricalSync)
			}
		}
		if bc.Sync.Concurrency != nil {
			if bc.Sync.Concurrency.HistoricalSyncWorkers > 0 {
				config.WorkerCount = int(bc.Sync.Concurrency.HistoricalSyncWorkers)
			}
		}
	}

	return config
}

// TransferEvent represents an ERC-20 Transfer event.
// Event signature: Transfer(address indexed from, address indexed to, uint256 value)
const (
	TransferEventSignature = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	TransferEventHash      = "Transfer(address,address,uint256)"
)

// NewTokenTransferSyncService creates a new instance of TokenTransferSyncService.
func NewTokenTransferSyncService(ctx context.Context, config *TokenTransferSyncConfig, bc *conf.Bootstrap) (*TokenTransferSyncService, error) {
	serviceCtx, cancel := context.WithCancel(ctx)

	// Create EVM client
	var evmClient *chain.EVMClient
	var err error

	if bc != nil && bc.Chain != nil && bc.Chain.Evm != nil && bc.Chain.Evm.Enabled {
		evmClient, err = createEVMClientFromConfig(bc.Chain.Evm)
		if err != nil {
			cancel()
			return nil, errors.Wrap(err, "failed to create EVM client from config")
		}
	} else {
		networkName := global.NetworkName
		if networkName == "" {
			networkName = "mainnet"
		}
		evmClient, err = chain.NewEVMClient(networkName)
		if err != nil {
			cancel()
			return nil, errors.Wrap(err, "failed to create EVM client")
		}
	}

	// Get retention days from environment or use default (90 days)
	retentionDays := 90
	if days := os.Getenv("TOKEN_TRANSFER_RETENTION_DAYS"); days != "" {
		if d, err := strconv.Atoi(days); err == nil && d > 0 {
			retentionDays = d
		}
	}

	return &TokenTransferSyncService{
		ctx:           serviceCtx,
		cancel:        cancel,
		config:        config,
		evmClient:     evmClient,
		isRunning:     false,
		cleanupTicker: time.NewTicker(24 * time.Hour), // Run cleanup every 24 hours
		retentionDays: retentionDays,
	}, nil
}

// Start starts the token transfer sync service.
func (s *TokenTransferSyncService) Start() {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	if s.isRunning {
		global.Logger.Warn("Token transfer sync service is already running")
		return
	}

	s.isRunning = true

	// Start historical sync worker
	s.wg.Add(1)
	go s.historicalSyncWorker()

	// Start realtime sync worker if enabled
	if s.config.EnableRealtimeSync {
		s.wg.Add(1)
		go s.realtimeSyncWorker()
	}

	// Start cleanup worker
	s.wg.Add(1)
	go s.cleanupWorker()

	global.Logger.Infof("Token transfer sync service started (retention: %d days)", s.retentionDays)
}

// cleanupWorker periodically cleans up old transfer records.
func (s *TokenTransferSyncService) cleanupWorker() {
	defer s.wg.Done()

	// Run cleanup immediately on start
	s.cleanupOldTransfers()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.cleanupTicker.C:
			s.cleanupOldTransfers()
		}
	}
}

// cleanupOldTransfers removes transfer records older than retention period.
func (s *TokenTransferSyncService) cleanupOldTransfers() {
	if s.retentionDays <= 0 {
		return // No cleanup if retention is disabled
	}

	cutoffTime := time.Now().AddDate(0, 0, -s.retentionDays)

	result := db.Get().Exec(
		"DELETE FROM token_transfers WHERE block_timestamp < ? AND deleted_at IS NULL",
		cutoffTime.Unix(),
	)

	if result.Error != nil {
		global.Logger.Errorf("Failed to cleanup old token transfers: %v", result.Error)
	} else if result.RowsAffected > 0 {
		global.Logger.Infof("Cleaned up %d old token transfer records (older than %s)",
			result.RowsAffected, cutoffTime.Format("2006-01-02"))
	}
}

// Stop stops the token transfer sync service.
func (s *TokenTransferSyncService) Stop() error {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	if !s.isRunning {
		return nil
	}

	s.cancel()

	// Stop cleanup ticker
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}

	s.wg.Wait()
	s.isRunning = false

	global.Logger.Info("Token transfer sync service stopped")
	return nil
}

// historicalSyncWorker processes historical blocks for token transfers.
func (s *TokenTransferSyncService) historicalSyncWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.ProcessInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.processHistoricalBlocks(); err != nil {
				global.Logger.Errorf("Failed to process historical blocks: %v", err)
			}
		}
	}
}

// realtimeSyncWorker processes new blocks in realtime.
func (s *TokenTransferSyncService) realtimeSyncWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Second) // Check for new blocks every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.processNewBlocks(); err != nil {
				global.Logger.Errorf("Failed to process new blocks: %v", err)
			}
		}
	}
}

// processHistoricalBlocks processes historical blocks for token transfers.
func (s *TokenTransferSyncService) processHistoricalBlocks() error {
	// Get last processed block
	lastProcessedBlock, err := s.getLastProcessedBlock()
	if err != nil {
		return errors.Wrap(err, "failed to get last processed block")
	}

	// If configured to start from specific block
	if s.config.StartBlock > 0 && lastProcessedBlock < s.config.StartBlock {
		lastProcessedBlock = s.config.StartBlock
	}

	// Get current block number
	currentBlockBig, err := s.evmClient.GetBlockNumber(s.ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current block number")
	}
	currentBlock := currentBlockBig.Uint64()

	// If caught up, skip
	if lastProcessedBlock >= currentBlock {
		return nil
	}

	// Process blocks in batches
	endBlock := lastProcessedBlock + uint64(s.config.BatchSize)
	if endBlock > currentBlock {
		endBlock = currentBlock
	}

	global.Logger.Debugf("Processing historical blocks: %d to %d", lastProcessedBlock+1, endBlock)

	// Fetch and process logs for Transfer events
	if err := s.processTransferLogs(lastProcessedBlock+1, endBlock); err != nil {
		return errors.Wrap(err, "failed to process transfer logs")
	}

	// Update last processed block
	if err := s.updateLastProcessedBlock(endBlock); err != nil {
		global.Logger.Warnf("Failed to update last processed block: %v", err)
	}

	return nil
}

// processNewBlocks processes new blocks for token transfers.
func (s *TokenTransferSyncService) processNewBlocks() error {
	// Get last synced block from database
	lastSyncedBlock, err := s.getLastProcessedBlock()
	if err != nil {
		return errors.Wrap(err, "failed to get last synced block")
	}

	// Get current block number from chain
	currentBlockBig, err := s.evmClient.GetBlockNumber(s.ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current block number")
	}
	currentBlock := currentBlockBig.Uint64()

	// If no new blocks, skip
	if lastSyncedBlock >= currentBlock {
		return nil
	}

	global.Logger.Debugf("Processing new blocks: %d to %d", lastSyncedBlock+1, currentBlock)

	// Process logs for new blocks
	if err := s.processTransferLogs(lastSyncedBlock+1, currentBlock); err != nil {
		return errors.Wrap(err, "failed to process transfer logs")
	}

	// Update last processed block
	if err := s.updateLastProcessedBlock(currentBlock); err != nil {
		global.Logger.Warnf("Failed to update last processed block: %v", err)
	}

	return nil
}

// processTransferLogs fetches and processes Transfer event logs for a block range.
func (s *TokenTransferSyncService) processTransferLogs(fromBlock, toBlock uint64) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.Timeout)
	defer cancel()

	// Build query for Transfer events
	// Transfer event signature: keccak256("Transfer(address,address,uint256)")
	topic := ethcommon.HexToHash(TransferEventSignature)

	// Create filter query
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(fromBlock)),
		ToBlock:   big.NewInt(int64(toBlock)),
		Topics:    [][]ethcommon.Hash{{topic}},
	}

	// Fetch logs
	logs, err := s.evmClient.GetLogs(ctx, query)
	if err != nil {
		return errors.Wrap(err, "failed to fetch logs")
	}

	if len(logs) == 0 {
		return nil
	}

	global.Logger.Debugf("Found %d Transfer events in blocks %d-%d", len(logs), fromBlock, toBlock)

	// Process logs into token transfers
	transfers := s.parseTransferLogs(logs)
	if len(transfers) == 0 {
		return nil
	}

	// Save transfers to database
	if err := s.saveTokenTransfers(transfers); err != nil {
		return errors.Wrap(err, "failed to save token transfers")
	}

	global.Logger.Debugf("Saved %d token transfers", len(transfers))

	return nil
}

// parseTransferLogs parses EVM logs into TokenTransfer models.
func (s *TokenTransferSyncService) parseTransferLogs(logs []ethtypes.Log) []*models.TokenTransfer {
	transfers := make([]*models.TokenTransfer, 0, len(logs))

	for _, log := range logs {
		// Skip if not enough topics (Transfer event has 3 topics: event signature + 2 indexed addresses)
		if len(log.Topics) < 3 {
			continue
		}

		// Parse from address (topic 1)
		fromAddress := ethcommon.HexToAddress(log.Topics[1].Hex()).Hex()

		// Parse to address (topic 2)
		toAddress := ethcommon.HexToAddress(log.Topics[2].Hex()).Hex()

		// Parse amount (data field)
		var amount string
		if len(log.Data) >= 32 {
			amountValue := new(big.Int).SetBytes(log.Data)
			amount = amountValue.String()
		} else {
			amount = "0"
		}

		// Determine method
		method := "Transfer"
		if fromAddress == "0x0000000000000000000000000000000000000000" {
			method = "Mint"
		} else if toAddress == "0x0000000000000000000000000000000000000000" {
			method = "Burn"
		}

		transfer := &models.TokenTransfer{
			ContractAddress: strings.ToLower(log.Address.Hex()),
			FromAddress:     strings.ToLower(fromAddress),
			ToAddress:       strings.ToLower(toAddress),
			Amount:          amount,
			Method:          method,
			BlockNumber:     log.BlockNumber,
			LogIndex:        uint32(log.Index),
			ChainID:         DefaultChainID,
		}

		// Get transaction hash
		if log.TxHash != (ethcommon.Hash{}) {
			transfer.EVMHash = strings.ToLower(log.TxHash.Hex())
		}

		transfers = append(transfers, transfer)
	}

	return transfers
}

// saveTokenTransfers saves token transfers to database in batches.
func (s *TokenTransferSyncService) saveTokenTransfers(transfers []*models.TokenTransfer) error {
	if len(transfers) == 0 {
		return nil
	}

	// Look up Cosmos transaction hashes for EVM hashes
	if err := s.enrichTransactionHashes(transfers); err != nil {
		global.Logger.Warnf("Failed to enrich transaction hashes: %v", err)
	}

	// Batch insert with upsert (ignore duplicates)
	return db.Get().Transaction(func(tx *gorm.DB) error {
		for _, transfer := range transfers {
			// Use clause.OnConflict to ignore duplicates based on unique index
			err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "transaction_hash"}, {Name: "log_index"}},
				DoNothing: true,
			}).Create(transfer).Error

			if err != nil {
				global.Logger.Warnf("Failed to insert transfer: %v", err)
			}
		}
		return nil
	})
}

// enrichTransactionHashes looks up Cosmos transaction hashes for EVM transaction hashes.
func (s *TokenTransferSyncService) enrichTransactionHashes(transfers []*models.TokenTransfer) error {
	// Collect EVM hashes
	evmHashes := make([]string, 0, len(transfers))
	for _, transfer := range transfers {
		if transfer.EVMHash != "" {
			evmHashes = append(evmHashes, transfer.EVMHash)
		}
	}

	if len(evmHashes) == 0 {
		return nil
	}

	// Query evm_transactions table
	var evmTxs []models.EVMTransaction
	err := db.Get().Model(&models.EVMTransaction{}).
		Where("evm_hash IN ?", evmHashes).
		Find(&evmTxs).Error

	if err != nil {
		return err
	}

	// Build lookup map
	hashMap := make(map[string]string)
	for _, evmTx := range evmTxs {
		hashMap[strings.ToLower(evmTx.EVMHash)] = evmTx.TransactionHash
	}

	// Enrich transfers
	for _, transfer := range transfers {
		if cosmosHash, ok := hashMap[strings.ToLower(transfer.EVMHash)]; ok {
			transfer.TransactionHash = cosmosHash
		} else {
			// If no Cosmos hash found, use EVM hash as fallback
			transfer.TransactionHash = transfer.EVMHash
		}
	}

	return nil
}

// getLastProcessedBlock gets the last processed block number.
func (s *TokenTransferSyncService) getLastProcessedBlock() (uint64, error) {
	// Try to get from database
	var result struct {
		LastBlock uint64
	}

	err := db.Get().Raw(`
		SELECT COALESCE(MAX(block_number), 0) as last_block
		FROM token_transfers
		WHERE deleted_at IS NULL
	`).Scan(&result).Error

	if err != nil {
		return 0, err
	}

	// If no records, return config start block or 0
	if result.LastBlock == 0 && s.config.StartBlock > 0 {
		return s.config.StartBlock, nil
	}

	return result.LastBlock, nil
}

// updateLastProcessedBlock updates the last processed block number.
func (s *TokenTransferSyncService) updateLastProcessedBlock(blockNumber uint64) error {
	// Store in a simple key-value table or use Redis
	// For now, we'll just log it
	global.Logger.Debugf("Updated last processed block to %d", blockNumber)
	return nil
}

// SyncTransfersForContract manually syncs transfers for a specific contract.
func (s *TokenTransferSyncService) SyncTransfersForContract(contractAddress string, fromBlock, toBlock uint64) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.Timeout)
	defer cancel()

	// Normalize contract address
	contractAddr := ethcommon.HexToAddress(contractAddress)

	// Build query for Transfer events from specific contract
	topic := ethcommon.HexToHash(TransferEventSignature)

	// Create filter query
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(fromBlock)),
		ToBlock:   big.NewInt(int64(toBlock)),
		Addresses: []ethcommon.Address{contractAddr},
		Topics:    [][]ethcommon.Hash{{topic}},
	}

	// Fetch logs
	logs, err := s.evmClient.GetLogs(ctx, query)
	if err != nil {
		return errors.Wrap(err, "failed to fetch logs")
	}

	if len(logs) == 0 {
		return nil
	}

	// Parse and save
	transfers := s.parseTransferLogs(logs)
	if err := s.saveTokenTransfers(transfers); err != nil {
		return errors.Wrap(err, "failed to save token transfers")
	}

	return nil
}

// GetTokenTransferCount returns the total number of transfers for a contract.
func GetTokenTransferCount(contractAddress string) (int64, error) {
	var count int64
	err := db.Get().Model(&models.TokenTransfer{}).
		Where("contract_address = ?", strings.ToLower(contractAddress)).
		Where("deleted_at IS NULL").
		Count(&count).Error

	return count, err
}

// GetTokenTransferVolume calculates the total transfer volume for a contract in a time range.
func GetTokenTransferVolume(contractAddress string, startTime, endTime *time.Time) (*big.Int, error) {
	query := db.Get().Model(&models.TokenTransfer{}).
		Where("contract_address = ?", strings.ToLower(contractAddress)).
		Where("deleted_at IS NULL")

	if startTime != nil {
		query = query.Where("block_timestamp >= ?", startTime.Unix())
	}
	if endTime != nil {
		query = query.Where("block_timestamp <= ?", endTime.Unix())
	}

	var transfers []models.TokenTransfer
	if err := query.Find(&transfers).Error; err != nil {
		return nil, err
	}

	totalVolume := big.NewInt(0)
	for _, transfer := range transfers {
		amount, ok := new(big.Int).SetString(transfer.Amount, 10)
		if ok {
			totalVolume.Add(totalVolume, amount)
		}
	}

	return totalVolume, nil
}

// FormatTokenAmount formats a token amount with proper decimals.
func FormatTokenAmount(amount string, decimals int) string {
	amountInt, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return amount
	}

	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	quotient := new(big.Int).Div(amountInt, divisor)
	remainder := new(big.Int).Mod(amountInt, divisor)

	if remainder.Sign() == 0 {
		return quotient.String()
	}

	// Format with decimals
	remainderStr := remainder.String()
	for len(remainderStr) < decimals {
		remainderStr = "0" + remainderStr
	}
	remainderStr = strings.TrimRight(remainderStr, "0")
	if remainderStr == "" {
		return quotient.String()
	}

	return fmt.Sprintf("%s.%s", quotient.String(), remainderStr)
}
