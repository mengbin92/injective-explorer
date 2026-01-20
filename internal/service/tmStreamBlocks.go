package service

import (
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"
	"context"
	"encoding/hex"
	"time"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/types/tx"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/pkg/errors"
	"gorm.io/gorm/clause"
)

func (s *SyncService) tmStreamBlocks(ctx context.Context) {
	defer s.wg.Done()

	global.Logger.Info("starting real-time block stream (WSS) with Tendermint client")

	sub, err := s.rpcClient.Subscribe(ctx, "injective-block-listener", "tm.event='NewBlockHeader'")
	if err != nil {
		global.Logger.Errorf("failed to subscribe to TM client: %v", err)
		return
	}

	for {
		select {
		case msg := <-sub:
			ev, ok := msg.Data.(tmtypes.EventDataNewBlockHeader)
			if !ok {
				continue
			}

			h := ev.Header

			global.Logger.Infof("height=%d hash=%X time=%s proposer=%s", h.Height, h.Hash(), h.Time.Format(time.RFC3339), h.ProposerAddress.String())
			blockResp, err := s.tmProcessBlock(ctx, int64(h.Height))
			if err != nil {
				global.Logger.Errorf("failed to process block at height %d: %v", h.Height, err)
				continue
			}

			// Save block to database using upsert for optimal performance
			blockModel := models.BlockFromTM(blockResp)
			if err := s.saveBlockToDB(blockModel); err != nil {
				global.Logger.Errorf("failed to save block at height %d: %v", h.Height, err)
			}

			// Extract transaction hashes and push to Redis queue for processing
			if len(blockResp.Block.Data.Txs) > 0 {
				redisClient := cache.GetRedisClient()
				if redisClient != nil {
					// Use Pipeline for batch operations to reduce network round trips
					pipe := redisClient.Pipeline()
					for _, tx := range blockResp.Block.Data.Txs {
						pipe.LPush(ctx, s.config.TxHashQueueKey, hex.EncodeToString(tx.Hash()))
					}
					_, err := pipe.Exec(ctx)
					if err != nil {
						global.Logger.Warnf("failed to batch push tx hashes to queue: %v", err)
						// Continue even if Redis push fails (eventual consistency)
					} else {
						global.Logger.Debugf("pushed %d transaction hashes to queue for height %d", len(blockResp.Block.Data.Txs), h.Height)
					}
				} else {
					global.Logger.Warnf("redis client is nil, skipping tx hash queue for height %d", h.Height)
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func (s *SyncService) tmProcessBlock(ctx context.Context, height int64) (*coretypes.ResultBlock, error) {
	global.Logger.Infof("processing block with cometbft RPC client at height %d", height)
	blockResp, err := s.rpcClient.Block(ctx, &height)
	if err != nil {
		global.Logger.Errorf("failed to get block at height %d: %v", height, err)
		return nil, errors.Wrap(err, "failed to get block at height")
	}
	return blockResp, nil
}

func (s *SyncService) getAllValidators(ctx context.Context) error {
	// Create Staking query client using ChainClientV2's QueryClient
	// This ensures TLS configuration is handled correctly
	stakingClient := stakingtypes.NewQueryClient(s.chainClient.QueryClient())
	validatorsResp, err := stakingClient.Validators(ctx, &stakingtypes.QueryValidatorsRequest{
		// Status: stakingtypes.BondStatusBonded, // Query bonded validators
		Pagination: &query.PageRequest{
			Limit: 1000, // Query up to 1000 validators
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to query validators from chain")
	}

	count := 0
	for _, validator := range validatorsResp.Validators {
		validatorModel := models.ValidatorFromStakingTypes(validator)
		saveToDB(validatorModel, "create validator error")
		count++
	}
	global.Logger.Infof("synced %d validators from chain", count)
	return nil
}

// periodicValidatorUpdate periodically updates validator information
func (s *SyncService) periodicValidatorUpdate(ctx context.Context) {
	ticker := time.NewTicker(ValidatorUpdateInterval) // Update validators periodically
	defer ticker.Stop()

	global.Logger.Infof("starting periodic validator update (interval: 10m)")

	for {
		select {
		case <-ctx.Done():
			global.Logger.Infof("periodic validator update stopped")
			return
		case <-ticker.C:
			if err := s.getAllValidators(ctx); err != nil {
				global.Logger.Errorf("failed to update validators: %v", err)
				// Continue even if update fails
			}
		}
	}
}

func (s *SyncService) getTx(ctx context.Context, hash string) (*tx.GetTxResponse, error) {
	txResp, err := s.chainClient.GetTx(ctx,hash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tx from chain")
	}
	return txResp, nil
}

// saveBlockToDB saves a block to the database using upsert for optimal performance.
// It uses clause.OnConflict to reduce database round trips from 2 to 1.
func (s *SyncService) saveBlockToDB(blockModel *models.Block) error {
	if blockModel == nil {
		return errors.New("block model is nil")
	}

	err := db.Get().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "height"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"proposer", "moniker", "block_hash", "parent_hash",
			"num_precommits", "num_txs", "total_txs", "txs",
			"timestamp", "block_unix_timestamp", "updated_at",
		}),
	}).Create(blockModel).Error

	if err != nil {
		return errors.Wrapf(err, "failed to upsert block at height %d", blockModel.Height)
	}

	// Publish block event for streaming
	eventBus := GetEventBus()
	eventBus.PublishBlock(&BlockEvent{
		Block: blockModel,
		Type:  EventTypeBlockNew,
	})

	// Cache block to Redis
	cacheBlock(context.Background(), blockModel)

	return nil
}
