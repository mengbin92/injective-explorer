// Package service provides token list service for the explorer.
package service

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
)

const (
	// Cache key constants for token list
	CacheKeyTokenList    = "token_list:%s:%s:%d" // chain_id:filters_hash:page (page_size included in filters_hash)
	CacheExpireTokenList = 5 * time.Minute
)

// TokenListService provides token list with market data
type TokenListService struct {
	pb.UnimplementedTokenListServiceServer
	explorerService *ExplorerService
}

// NewTokenListService creates a new TokenListService
func NewTokenListService(explorerService *ExplorerService) *TokenListService {
	return &TokenListService{
		explorerService: explorerService,
	}
}

// ListTokens returns a paginated list of tokens with market data
func (s *TokenListService) ListTokens(ctx context.Context, req *pb.TokenListRequest) (*pb.Reply, error) {
	// 1. Parameter validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), nil
	}

	// 2. Set default values
	chainID := req.ChainId
	if chainID == "" {
		chainID = DefaultChainID
	}

	sortField := req.SortBy
	if sortField == "" {
		sortField = "market_cap" // Default sort by market cap
	}

	sortOrder := req.SortOrder
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc" // Default descending order
	}

	// 3. Normalize pagination parameters
	var pagination *pb.PaginationRequest
	if req.Pagination != nil {
		pagination = req.Pagination
	} else {
		pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, _, _ := normalizePagination(
		pagination.Page,
		pagination.PageSize,
		pagination.Cursor,
		MaxPageSize,
	)

	// 4. Try to get from cache
	cacheKey := fmt.Sprintf(CacheKeyTokenList, chainID, buildCacheKeySuffix(req, pageSize), page)
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cachedData, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil && cachedData != "" {
			var cachedResponse pb.TokenListResponse
			if err := sonic.Unmarshal([]byte(cachedData), &cachedResponse); err == nil {
				global.Logger.Debugf("returning cached token list for page %d", page)
				return MarshalAndReply(&cachedResponse, "marshal cached token list error")
			}
		}
	}

	// 5. Build query
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).
		Model(&models.ContractInfo{}).
		Preload("Tags").
		Where("token_symbol IS NOT NULL AND token_symbol != ''").
		Group("contract_infos.id, contract_infos.contract_address, contract_infos.chain_id")

	// Apply filter conditions
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	if req.ContractType != "" {
		query = query.Where("contract_type = ?", req.ContractType)
	}

	if req.VerificationStatus != "" {
		query = query.Where("verification_status = ?", req.VerificationStatus)
	}

	if req.Search != "" {
		search := strings.TrimSpace(req.Search)

		// If search starts with 0x, treat it as an address
		if strings.HasPrefix(search, "0x") {
			// Full address: exact match (42 chars for address, 66 for tx hash)
			if len(search) == 42 || len(search) == 66 {
				query = query.Where("contract_address = ?", search)
			} else {
				// Address prefix: prefix match
				query = query.Where("contract_address LIKE ?", search+"%")
			}
		} else {
			// Normal search: fuzzy match on name and symbol
			searchPattern := "%" + search + "%"
			query = query.Where(
				"token_name LIKE ? OR token_symbol LIKE ? OR contract_address LIKE ?",
				searchPattern, searchPattern, searchPattern,
			)
		}
	}

	if req.TagType != "" {
		query = query.Joins(
			"INNER JOIN contract_tags ON contract_tags.contract_address = contract_infos.contract_address AND contract_tags.chain_id = contract_infos.chain_id",
		).Where("contract_tags.tag_type = ? AND contract_tags.visible = ?", req.TagType, true)
	}

	// 6. Get total count
	var total int64
	var countErr error
	total, _, countErr = getApproximateCountWithError(query, &models.ContractInfo{}, queryCtx)
	if countErr != nil {
		global.Logger.Warnf("failed to get count for tokens: %v", countErr)
		total = ApproximateCountFallback
	}

	// 7. Execute query (fetch more data if sorting by market data)
	fetchSize := int64(pageSize)
	if sortField == "market_cap" || sortField == "volume_24h" || sortField == "price" || sortField == "holders" {
		fetchSize = int64(pageSize * 3) // Fetch 3x data for sorting
	}

	query = query.Offset(int(offset)).Limit(int(fetchSize)).Order("created_at DESC")

	var contracts []models.ContractInfo
	err := query.Find(&contracts).Error
	if err != nil {
		global.Logger.Errorf("failed to query tokens: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query tokens", err), err
	}

	// 8. Batch get market data
	tokenSymbols := extractTokenSymbols(contracts)
	marketDataMap := s.batchGetTokenMarketData(ctx, tokenSymbols)

	// 9. Assemble response data
	items := make([]*pb.TokenListItemDTO, 0, len(contracts))
	for _, contract := range contracts {
		item := &pb.TokenListItemDTO{
			Id:                   uint64(contract.ID),
			ContractAddress:      contract.ContractAddress,
			ContractAddressShort: formatAddressShort(contract.ContractAddress),
			TokenName:            contract.TokenName,
			TokenSymbol:          contract.TokenSymbol,
			Tags:                 models.ToContractTagInfoDTOs(contract.Tags),
			FirstDeploymentTime:  formatDeploymentTime(contract.CreatedAt),
			CreatedAt:            contract.CreatedAt.Format(time.RFC3339),
		}

		// Fill market data (use uppercase symbol for lookup)
		if marketData, ok := marketDataMap[strings.ToUpper(contract.TokenSymbol)]; ok {
			item.PriceUsd = marketData.PriceUSD
			item.PriceChange_24H = marketData.PriceChange24h
			item.TotalVolume_24H = marketData.TotalVolume24h
			item.MarketCapUsd = marketData.MarketCapUSD
			item.HoldersCount = marketData.HoldersCount
		}

		// Get token config for denom and decimals
		tokenConfig := GetTokenConfigOrCreate(contract.TokenSymbol)
		if tokenConfig != nil {
			item.Denom = tokenConfig.Denom
			item.Decimals = tokenConfig.Decimals
		} else {
			// Fallback: use contract address as denom and default decimals
			item.Denom = contract.ContractAddress
			item.Decimals = 18
		}

		items = append(items, item)
	}

	// 10. Sort if by market data
	// When sorting by market data, try to use pre-computed cache for better performance
	if sortField == "market_cap" || sortField == "volume_24h" || sortField == "price" || sortField == "holders" {
		// Try to get sorted data from cache first
		cacheItems, err := s.getTokensSortedFromCache(ctx, chainID, sortField, sortOrder, int(offset), int(pageSize))
		if err == nil && len(cacheItems) > 0 {
			items = cacheItems
		} else {
			// Fallback to in-memory sorting
			items = sortTokenItems(items, sortField, sortOrder)
			if len(items) > int(pageSize) {
				items = items[:pageSize]
			}
		}
	}

	// 11. Build pagination info
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}
	paginationInfo := &pb.PaginationInfo{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
	}

	// 12. Build pagination response
	response := &pb.TokenListResponse{
		Items:      items,
		Pagination: paginationInfo,
	}

	// 13. Cache result
	if redisClient != nil {
		go func() {
			cacheJSON, _ := sonic.Marshal(response)
			redisClient.Set(context.Background(), cacheKey, string(cacheJSON), CacheExpireTokenList)
		}()
	}

	return MarshalAndReply(response, "marshal token list error")
}

// batchGetTokenMarketData fetches market data for multiple tokens in parallel.
// It first tries to get data from token_market_cache table, then falls back to
// fetching from external API via token config.
func (s *TokenListService) batchGetTokenMarketData(ctx context.Context, symbols []string) map[string]*TokenMarketData {
	result := make(map[string]*TokenMarketData)

	// Step 1: Try to get data from token_market_cache table first
	// Normalize symbols to lowercase for case-insensitive matching
	lowerSymbols := make([]string, len(symbols))
	for i, s := range symbols {
		lowerSymbols[i] = strings.ToLower(s)
	}

	var cacheResults []models.TokenMarketCache
	err := db.Get().WithContext(ctx).
		Model(&models.TokenMarketCache{}).
		Where("LOWER(token_symbol) IN ?", lowerSymbols).
		Where("deleted_at IS NULL").
		Find(&cacheResults).Error

	if err != nil {
		global.Logger.Warnf("failed to query token_market_cache: %v", err)
	} else {
		// Build result from cache data using original symbol case as key
		for _, cache := range cacheResults {
			result[strings.ToUpper(cache.TokenSymbol)] = &TokenMarketData{
				PriceUSD:              cache.PriceUSD,
				PriceChange24h:        cache.PriceChange24h,
				MarketCapUSD:          cache.MarketCapUSD,
				TotalVolume24h:        cache.TotalVolume24h,
				HoldersCount:          cache.HoldersCount,
				TotalSupply:           parseBigIntPtr(cache.TotalSupply),
				CirculatingSupply:     parseBigIntPtr(cache.CirculatingSupply),
				MaxSupply:             parseBigIntPtr(cache.MaxSupply),
				FullyDilutedMarketCap: cache.FullyDilutedMarketCap,
			}
		}
		global.Logger.Debugf("loaded %d tokens from token_market_cache", len(result))
	}

	// Step 2: For tokens not in cache, try to fetch via API using token config
	tokensNeedingFetch := make([]string, 0)
	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}
		// Use uppercase for lookup
		if _, ok := result[strings.ToUpper(symbol)]; !ok {
			tokensNeedingFetch = append(tokensNeedingFetch, symbol)
		}
	}

	if len(tokensNeedingFetch) > 0 {
		// Build token config map for tokens not in cache
		tokenConfigs := make(map[string]*TokenConfig)
		for _, symbol := range tokensNeedingFetch {
			config := GetTokenConfigOrCreate(symbol)
			if config == nil {
				global.Logger.Debugf("no token config for %s: unable to create from symbol", symbol)
				continue
			}
			tokenConfigs[symbol] = config
		}

		// Fetch market data from API for remaining tokens
		if len(tokenConfigs) > 0 {
			apiResults := BatchGetTokenMarketData(ctx, tokenConfigs)
			for symbol, data := range apiResults {
				// Store with uppercase key
				result[strings.ToUpper(symbol)] = data
			}
		}
	}

	return result
}

// extractTokenSymbols extracts unique token symbols from contracts
func extractTokenSymbols(contracts []models.ContractInfo) []string {
	symbols := make([]string, 0, len(contracts))
	seen := make(map[string]bool)

	for _, contract := range contracts {
		if contract.TokenSymbol != "" && !seen[contract.TokenSymbol] {
			symbols = append(symbols, contract.TokenSymbol)
			seen[contract.TokenSymbol] = true
		}
	}

	return symbols
}

// sortTokenItems sorts token items by specified field
func sortTokenItems(items []*pb.TokenListItemDTO, sortField, sortOrder string) []*pb.TokenListItemDTO {
	sort.Slice(items, func(i, j int) bool {
		var iValue, jValue float64

		switch sortField {
		case "market_cap":
			iValue = items[i].MarketCapUsd
			jValue = items[j].MarketCapUsd
		case "volume_24h":
			iValue = items[i].TotalVolume_24H
			jValue = items[j].TotalVolume_24H
		case "price":
			iValue = items[i].PriceUsd
			jValue = items[j].PriceUsd
		case "holders":
			iValue = float64(items[i].HoldersCount)
			jValue = float64(items[j].HoldersCount)
		default:
			return false
		}

		if sortOrder == "asc" {
			return iValue < jValue
		}
		return iValue > jValue
	})

	return items
}

// getTokensSortedFromCache retrieves tokens sorted by market data from pre-computed cache.
// This is more efficient than in-memory sorting when dealing with large datasets.
func (s *TokenListService) getTokensSortedFromCache(ctx context.Context, chainID, sortField, sortOrder string, offset, limit int) ([]*pb.TokenListItemDTO, error) {
	// Query the token_market_cache table
	query := &models.TokenMarketCacheQuery{
		ChainID:   chainID,
		SortBy:    sortField,
		SortOrder: sortOrder,
		Page:      offset/limit + 1,
		PageSize:  limit,
	}

	cacheResults, total, err := ListTokenMarketsFromCache(ctx, query)
	if err != nil {
		return nil, err
	}

	if total == 0 {
		return nil, nil
	}

	// Get contract info for these tokens
	// Normalize symbols to lowercase for case-insensitive matching
	lowerSymbols := make([]string, 0, len(cacheResults))
	for _, cache := range cacheResults {
		lowerSymbols = append(lowerSymbols, strings.ToLower(cache.TokenSymbol))
	}

	var contracts []models.ContractInfo
	err = db.Get().WithContext(ctx).
		Preload("Tags").
		Where("LOWER(token_symbol) IN ? AND chain_id = ?", lowerSymbols, chainID).
		Where("deleted_at IS NULL").
		Find(&contracts).Error
	if err != nil {
		return nil, err
	}

	// Build contract lookup map using uppercase symbol as key
	contractMap := make(map[string]*models.ContractInfo)
	for i := range contracts {
		contractMap[strings.ToUpper(contracts[i].TokenSymbol)] = &contracts[i]
	}

	// Build cache lookup map using uppercase symbol as key
	cacheMap := make(map[string]*models.TokenMarketCache)
	for i := range cacheResults {
		cacheMap[strings.ToUpper(cacheResults[i].TokenSymbol)] = cacheResults[i]
	}

	// Assemble response items maintaining sort order from cache
	items := make([]*pb.TokenListItemDTO, 0, len(cacheResults))
	for _, cache := range cacheResults {
		contract, ok := contractMap[strings.ToUpper(cache.TokenSymbol)]
		if !ok {
			continue
		}

		item := &pb.TokenListItemDTO{
			Id:                   uint64(contract.ID),
			ContractAddress:      contract.ContractAddress,
			ContractAddressShort: formatAddressShort(contract.ContractAddress),
			TokenName:            contract.TokenName,
			TokenSymbol:          contract.TokenSymbol,
			Tags:                 models.ToContractTagInfoDTOs(contract.Tags),
			FirstDeploymentTime:  formatDeploymentTime(contract.CreatedAt),
			CreatedAt:            contract.CreatedAt.Format(time.RFC3339),
			PriceUsd:             cache.PriceUSD,
			PriceChange_24H:      cache.PriceChange24h,
			TotalVolume_24H:      cache.TotalVolume24h,
			MarketCapUsd:         cache.MarketCapUSD,
			HoldersCount:         cache.HoldersCount,
		}

		// Get token config for denom and decimals
		tokenConfig := GetTokenConfigOrCreate(contract.TokenSymbol)
		if tokenConfig != nil {
			item.Denom = tokenConfig.Denom
			item.Decimals = tokenConfig.Decimals
		} else {
			// Fallback: use contract address as denom and default decimals
			item.Denom = contract.ContractAddress
			item.Decimals = 18
		}

		items = append(items, item)
	}

	return items, nil
}

// formatDeploymentTime formats deployment time to "YYYY/MM/DD" format
func formatDeploymentTime(t time.Time) string {
	return t.Format("2006/01/02")
}

// formatAddressShort formats address to short form (e.g.=, "0x8a3d...7f2e")
func formatAddressShort(address string) string {
	if len(address) <= 10 {
		return address
	}
	return fmt.Sprintf("%s...%s", address[:6], address[len(address)-4:])
}

// buildCacheKeySuffix builds a cache key suffix from request parameters.
// Uses strings.Builder for efficient string concatenation, minimizing memory allocations.
func buildCacheKeySuffix(req *pb.TokenListRequest, pageSize int32) string {
	// Pre-calculate approximate capacity to reduce reallocations
	var sb strings.Builder
	// Estimate capacity: each field can be ~20 chars on average, plus separators
	estimatedCap := 0
	if req.ContractType != "" {
		estimatedCap += len(req.ContractType) + 1
	}
	if req.VerificationStatus != "" {
		estimatedCap += len(req.VerificationStatus) + 1
	}
	if req.Search != "" {
		estimatedCap += len(req.Search) + 1
	}
	if req.TagType != "" {
		estimatedCap += len(req.TagType) + 1
	}
	if req.SortBy != "" {
		estimatedCap += len(req.SortBy) + 1
	}
	if req.SortOrder != "" {
		estimatedCap += len(req.SortOrder) + 1
	}
	// Page size field: "ps" + digits + separator
	estimatedCap += 16
	sb.Grow(estimatedCap)

	first := true
	writeSeparator := func() {
		if first {
			first = false
		} else {
			sb.WriteByte(':')
		}
	}

	if req.ContractType != "" {
		writeSeparator()
		sb.WriteString(req.ContractType)
	}
	if req.VerificationStatus != "" {
		writeSeparator()
		sb.WriteString(req.VerificationStatus)
	}
	if req.Search != "" {
		writeSeparator()
		sb.WriteString(req.Search)
	}
	if req.TagType != "" {
		writeSeparator()
		sb.WriteString(req.TagType)
	}
	if req.SortBy != "" {
		writeSeparator()
		sb.WriteString(req.SortBy)
	}
	if req.SortOrder != "" {
		writeSeparator()
		sb.WriteString(req.SortOrder)
	}
	// Include page size in cache key
	writeSeparator()
	fmt.Fprintf(&sb, "ps%d", pageSize)

	if sb.Len() == 0 {
		return "default"
	}
	return sb.String()
}

// parseBigInt parses a string to *big.Int, returns nil if empty or invalid.
func parseBigInt(s string) *big.Int {
	if s == "" {
		return nil
	}
	n := new(big.Int)
	if _, ok := n.SetString(s, 10); !ok {
		return nil
	}
	return n
}

func parseBigIntPtr(s *string) *big.Int {
	if s == nil || *s == "" {
		return nil
	}
	n := new(big.Int)
	if _, ok := n.SetString(*s, 10); !ok {
		return nil
	}
	return n
}
