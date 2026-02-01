// Package service provides API implementation for on-chain price services.
package service

import (
	"context"
	"net/http"
	"time"

	v1 "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/models"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/emptypb"
)

// OnChainPriceService implements the OnChainPriceService gRPC/HTTP API.
type OnChainPriceService struct {
	v1.UnimplementedOnChainPriceServiceServer

	provider   *OnChainPriceProvider
	aggregator *PriceAggregator
	syncer     *OnChainPriceSyncer
	logger     *log.Helper
}

// NewOnChainPriceService creates a new on-chain price API service.
func NewOnChainPriceService(logger log.Logger) *OnChainPriceService {
	if logger == nil {
		logger = log.NewStdLogger(nil)
	}
	helper := log.NewHelper(logger)

	// Create provider with shared chain client
	provider, err := NewOnChainPriceProviderWithSharedClient(helper)
	if err != nil {
		helper.Warnw("failed to create on-chain price provider", "error", err)
		// Continue with nil provider, API will return errors
	}

	// Create aggregator
	aggregator := NewPriceAggregator(provider, helper)

	// Add CoinGecko as fallback source
	aggregator.AddSource(NewCoinGeckoPriceSource())

	// Create syncer for price history
	var syncer *OnChainPriceSyncer
	if provider != nil {
		syncer = NewOnChainPriceSyncer(provider, helper)
		// Start the syncer in background
		go func() {
			if err := syncer.Start(context.Background()); err != nil {
				helper.Warnw("failed to start price syncer", "error", err)
			}
		}()
		helper.Infow("on-chain price syncer started")
	}

	return &OnChainPriceService{
		provider:   provider,
		aggregator: aggregator,
		syncer:     syncer,
		logger:     helper,
	}
}

// GetOnChainPrice implements GetOnChainPrice API.
func (s *OnChainPriceService) GetOnChainPrice(ctx context.Context, req *v1.GetOnChainPriceRequest) (*v1.Reply, error) {
	if s.provider == nil {
		return ErrorReply(http.StatusInternalServerError, "on-chain price provider not initialized", nil), nil
	}

	denom := req.Denom
	if denom == "" {
		denom = symbolToDenom(req.Symbol)
	}

	priceInfo, err := s.provider.GetPriceByDenom(ctx, denom)
	if err != nil {
		s.logger.Warnw("failed to get on-chain price", "symbol", req.Symbol, "denom", denom, "error", err)
		return ErrorReply(http.StatusNotFound, "failed to get price", err), nil
	}

	// Convert to DTO
	dto := convertToOnChainPriceDTO(priceInfo)
	return MarshalAndReply(dto, "marshal price error")
}

// GetBatchOnChainPrices implements GetBatchOnChainPrices API.
func (s *OnChainPriceService) GetBatchOnChainPrices(ctx context.Context, req *v1.GetBatchOnChainPricesRequest) (*v1.Reply, error) {
	if s.provider == nil {
		return ErrorReply(http.StatusInternalServerError, "on-chain price provider not initialized", nil), nil
	}

	denoms := req.Denoms
	if len(denoms) == 0 {
		// Convert symbols to denoms
		for _, symbol := range req.Symbols {
			denoms = append(denoms, symbolToDenom(symbol))
		}
	}

	prices := s.provider.BatchGetPrices(ctx, denoms)

	var priceDTOs []*v1.OnChainPriceDTO
	successCount := 0
	failedCount := 0

	for _, price := range prices {
		if price != nil {
			priceDTOs = append(priceDTOs, convertToOnChainPriceDTO(price))
			successCount++
		} else {
			failedCount++
		}
	}

	// Build batch response DTO
	batchDTO := &v1.BatchOnChainPricesDTO{
		Prices:       priceDTOs,
		TotalCount:   int32(len(denoms)),
		SuccessCount: int32(successCount),
		FailedCount:  int32(failedCount),
	}

	return MarshalAndReply(batchDTO, "marshal batch prices error")
}

// GetOnChainPriceHistory implements GetOnChainPriceHistory API.
func (s *OnChainPriceService) GetOnChainPriceHistory(ctx context.Context, req *v1.GetOnChainPriceHistoryRequest) (*v1.Reply, error) {
	if s.syncer == nil {
		return ErrorReply(http.StatusInternalServerError, "price syncer not initialized", nil), nil
	}

	startTime := req.StartTime
	endTime := req.EndTime

	if endTime == 0 {
		endTime = time.Now().Unix()
	}
	if startTime == 0 {
		// Default to 24 hours ago
		startTime = endTime - 86400
	}

	limit := int(req.Limit)
	if limit == 0 || limit > 1000 {
		limit = 100
	}

	history, err := s.syncer.GetPriceHistory(ctx, req.Symbol, startTime, endTime, limit)
	if err != nil {
		s.logger.Warnw("failed to get price history", "symbol", req.Symbol, "error", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get price history", err), nil
	}

	var data []*v1.PriceHistoryPointDTO
	for _, item := range history {
		data = append(data, &v1.PriceHistoryPointDTO{
			Timestamp:  item.Timestamp,
			PriceUsd:   item.PriceUSD,
			Confidence: item.Confidence,
		})
	}

	// Build history DTO
	historyDTO := &v1.OnChainPriceHistoryDTO{
		Symbol: req.Symbol,
		Data:   data,
		Count:  int32(len(data)),
	}

	return MarshalAndReply(historyDTO, "marshal price history error")
}

// GetAggregatedPrice implements GetAggregatedPrice API.
func (s *OnChainPriceService) GetAggregatedPrice(ctx context.Context, req *v1.GetAggregatedPriceRequest) (*v1.Reply, error) {
	if s.aggregator == nil {
		return ErrorReply(http.StatusInternalServerError, "price aggregator not initialized", nil), nil
	}

	aggPrice, err := s.aggregator.GetAggregatedPrice(ctx, req.Symbol)
	if err != nil {
		s.logger.Warnw("failed to get aggregated price", "symbol", req.Symbol, "error", err)
		return ErrorReply(http.StatusNotFound, "failed to get aggregated price", err), nil
	}

	// Convert to DTO
	dto := convertToAggregatedPriceDTO(aggPrice)
	return MarshalAndReply(dto, "marshal aggregated price error")
}

// GetSyncStatus implements GetSyncStatus API.
func (s *OnChainPriceService) GetSyncStatus(ctx context.Context, _ *emptypb.Empty) (*v1.Reply, error) {
	if s.syncer == nil {
		// Return default status if syncer not initialized
		statusDTO := &v1.SyncStatusDTO{
			IsEnabled:          false,
			NativeInterval:     "30s",
			AllMarketsInterval: "60s",
			NativeTokens:       []string{"INJ"},
			BatchSize:          50,
			IsSyncing:          false,
		}
		return MarshalAndReply(statusDTO, "marshal sync status error")
	}

	status := s.syncer.GetSyncStatus()

	// Build status DTO
	statusDTO := &v1.SyncStatusDTO{
		IsEnabled:          getBoolFromMap(status, "enabled"),
		NativeInterval:     getStringFromMap(status, "native_interval"),
		AllMarketsInterval: getStringFromMap(status, "all_markets_interval"),
		BatchSize:          int32(getIntFromMap(status, "batch_size")),
		IsSyncing:          getBoolFromMap(status, "is_syncing"),
	}

	// Extract native tokens if available
	if tokens, ok := status["native_tokens"].([]string); ok {
		statusDTO.NativeTokens = tokens
	}
	if lastSync, ok := status["last_sync_time"].(int64); ok {
		statusDTO.LastSyncTime = lastSync
	}

	return MarshalAndReply(statusDTO, "marshal sync status error")
}

// TriggerSync implements TriggerSync API.
func (s *OnChainPriceService) TriggerSync(ctx context.Context, _ *emptypb.Empty) (*v1.Reply, error) {
	if s.syncer == nil {
		return ErrorReply(http.StatusInternalServerError, "price syncer not initialized", nil), nil
	}

	// Start sync in background
	go func() {
		if err := s.syncer.ManualSync(context.Background()); err != nil {
			s.logger.Warnw("manual sync failed", "error", err)
		}
	}()

	// Build trigger response DTO
	triggerDTO := &v1.TriggerSyncDTO{
		Success:     true,
		TriggeredAt: time.Now().Unix(),
	}

	return MarshalAndReply(triggerDTO, "marshal trigger sync response error")
}

// GetPriceSources implements GetPriceSources API.
func (s *OnChainPriceService) GetPriceSources(ctx context.Context, _ *emptypb.Empty) (*v1.Reply, error) {
	sources := []*v1.PriceSourceInfoDTO{
		{
			Name:        "onchain",
			Description: "On-chain spot market price from Injective DEX",
			IsAvailable: s.provider != nil,
			Priority:    1,
			LastUpdated: time.Now().Unix(),
		},
		{
			Name:        "coingecko",
			Description: "CoinGecko API price",
			IsAvailable: true,
			Priority:    2,
			LastUpdated: time.Now().Unix(),
		},
	}

	// Build sources DTO
	sourcesDTO := &v1.PriceSourcesDTO{
		Sources: sources,
	}

	return MarshalAndReply(sourcesDTO, "marshal price sources error")
}

// Helper functions to convert internal models to DTO messages

func convertToOnChainPriceDTO(price *models.PriceInfo) *v1.OnChainPriceDTO {
	if price == nil {
		return nil
	}
	return &v1.OnChainPriceDTO{
		Symbol:      price.Symbol,
		Denom:       price.Denom,
		PriceUsd:    price.PriceUSD,
		PriceRaw:    price.PriceRaw,
		BestBuy:     price.BestBuy,
		BestSell:    price.BestSell,
		Source:      price.Source,
		MarketId:    price.MarketID,
		Timestamp:   price.Timestamp,
		Confidence:  price.Confidence,
		BlockHeight: price.BlockHeight,
	}
}

func convertToAggregatedPriceDTO(price *AggregatedPrice) *v1.AggregatedPriceDTO {
	if price == nil {
		return nil
	}
	return &v1.AggregatedPriceDTO{
		Symbol:        price.Symbol,
		PriceUsd:      price.PriceUSD,
		Sources:       price.Sources,
		BestSource:    price.BestSource,
		Confidence:    price.Confidence,
		Timestamp:     price.Timestamp,
		IsOutlier:     price.IsOutlier,
		OutlierReason: price.OutlierReason,
	}
}

// Helper functions for map access

func getBoolFromMap(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func getIntFromMap(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		}
	}
	return 0
}
