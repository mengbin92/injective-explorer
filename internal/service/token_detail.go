// Package service provides token detail service for the explorer.
package service

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/internal/utils"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

const (
	// Cache key constants for token detail
	CacheKeyTokenDetail       = "token_detail:%s:%s"        // chain_id:contract_address
	CacheKeyTokenTransfers    = "token_transfers:%s:%s:%d"  // chain_id:contract_address:page
	CacheKeyTokenHolders      = "token_holders:%s:%s:%d"    // chain_id:contract_address:page
	CacheKeyTokenHoldersStats = "token_holders_stats:%s:%s" // chain_id:contract_address
	CacheExpireTokenDetail    = 10 * time.Minute
	CacheExpireTokenTransfers = 2 * time.Minute
	CacheExpireTokenHolders   = 5 * time.Minute
)

// TokenDetailService provides detailed token information
type TokenDetailService struct {
	pb.UnimplementedTokenDetailServiceServer
	explorerService *ExplorerService
}

// NewTokenDetailService creates a new TokenDetailService
func NewTokenDetailService(explorerService *ExplorerService) *TokenDetailService {
	return &TokenDetailService{
		explorerService: explorerService,
	}
}

// GetTokenDetail returns detailed information for a specific token
func (s *TokenDetailService) GetTokenDetail(ctx context.Context, req *pb.GetTokenDetailRequest) (*pb.Reply, error) {
	if err := utils.ValidateNil(req); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}

	// At least one of contract_address or denom must be provided
	if req.GetContractAddress() == "" && req.GetDenom() == "" {
		return ErrorReply(http.StatusBadRequest, "contract_address or denom is required", errors.New("missing required field")), errors.New("missing required field")
	}

	chainID := req.GetChainId()
	if chainID == "" {
		chainID = DefaultChainID
	}

	// If denom is provided but not contract_address, resolve contract_address from denom
	contractAddress := req.GetContractAddress()
	if contractAddress == "" && req.GetDenom() != "" {
		var contractInfo models.ContractInfo
		err := db.Get().WithContext(ctx).
			Where("peggy_denom = ? AND chain_id = ?", req.GetDenom(), chainID).
			First(&contractInfo).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &pb.Reply{
					Code:    int32(http.StatusOK),
					Message: "contract not found",
					Data:    nil,
				}, nil
			}
			global.Logger.Errorf("failed to query contract by denom: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to query contract by denom", err), err
		}
		contractAddress = contractInfo.ContractAddress
	}

	// Try to get from cache
	cacheKey := fmt.Sprintf(CacheKeyTokenDetail, chainID, contractAddress)
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cachedData, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil && cachedData != "" {
			var cachedResponse pb.TokenDetailResponse
			if err := sonic.Unmarshal([]byte(cachedData), &cachedResponse); err == nil {
				return MarshalAndReply(&cachedResponse, "marshal cached token detail error")
			}
		}
	}

	// Query contract info
	var contract models.ContractInfo
	err := db.Get().WithContext(ctx).
		Preload("Tags").
		Where("contract_address = ? AND chain_id = ?", contractAddress, chainID).
		First(&contract).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: "contract not found",
				Data:    nil,
			}, nil
		}
		global.Logger.Errorf("failed to query contract: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query contract", err), err
	}

	// Get full market data from CoinGecko (including supply info)
	var marketData *TokenMarketData
	if contract.TokenSymbol != "" {
		tokenConfig, err := LoadTokenConfigFromConf(nil, contract.TokenSymbol)
		if err == nil && tokenConfig.CoinGeckoID != "" {
			marketData, _ = GetTokenFullMarketData(ctx, tokenConfig)
		}
	}

	// Get total transfer count
	var totalTransfers int64
	db.Get().WithContext(ctx).
		Model(&models.TokenTransfer{}).
		Where("contract_address = ?", strings.ToLower(contractAddress)).
		Where("deleted_at IS NULL").
		Count(&totalTransfers)

	// Build response
	response := &pb.TokenDetailResponse{
		Basic: &pb.TokenBasicInfo{
			Name:            contract.TokenName,
			Symbol:          contract.TokenSymbol,
			ContractAddress: contract.ContractAddress,
			TokenType:       contract.ContractType,
			Tags:            models.ToContractTagInfoDTOs(contract.Tags),
		},
		Overview: &pb.TokenOverviewInfo{},
		Market: &pb.TokenMarketInfo{
			Price:          formatPrice(0),
			Volume_24H:     formatLargeNumber(0),
			HoldersCount:   0,
			TotalTransfers: totalTransfers,
			High_24H:       "-",
			Low_24H:        "-",
		},
		Additional: &pb.TokenAdditionalInfo{
			TokenStandard: normalizeTokenStandard(contract.ContractType),
			Decimals:      18, // Default decimals, should be fetched from contract
			DeployDate:    formatDeploymentTime(contract.CreatedAt),
			CreatorShort:  contract.Creator,
		},
	}

	// Resolve denom and decimals
	// Priority: contract.PeggyDenom (chain denom) > "peggy" + contractAddress
	if contract.PeggyDenom != "" {
		response.Basic.Denom = contract.PeggyDenom
	} else if contract.ContractAddress != "" {
		response.Basic.Denom = contract.PeggyDenom
	}
	response.Basic.Decimals = int32(contract.Decimals)
	if response.Basic.Decimals == 0 {
		response.Basic.Decimals = 18
	}

	// Fill market data
	if marketData != nil {
		response.Market.Price = formatPrice(marketData.PriceUSD)
		response.Market.PriceChange_24H = marketData.PriceChange24h
		response.Market.Volume_24H = formatLargeNumber(marketData.TotalVolume24h)
		response.Market.HoldersCount = marketData.HoldersCount
		response.Market.High_24H = formatPrice(marketData.High24h)
		response.Market.Low_24H = formatPrice(marketData.Low24h)

		response.Overview.MarketCap = formatLargeNumber(marketData.MarketCapUSD)
	}

	// Fill supply data with full details
	if marketData != nil {
		if marketData.CirculatingSupply != nil {
			response.Overview.CirculatingSupply = formatTokenAmount(marketData.CirculatingSupply, 18)
		}
		if marketData.TotalSupply != nil {
			response.Overview.MaxSupply = formatTokenAmount(marketData.TotalSupply, 18)
		}
		if marketData.MaxSupply != nil {
			response.Overview.MaxSupply = formatTokenAmount(marketData.MaxSupply, 18)
		} else {
			response.Overview.MaxSupply = "无限制"
		}
		response.Overview.FullyDilutedMarketCap = formatLargeNumber(marketData.FullyDilutedMarketCap)
	}

	// Cache result
	if redisClient != nil {
		go func() {
			cacheJSON, _ := sonic.Marshal(response)
			redisClient.Set(context.Background(), cacheKey, string(cacheJSON), CacheExpireTokenDetail)
		}()
	}

	return MarshalAndReply(response, "marshal token detail error")
}

// GetTokenTransfers returns transfer transactions for a token
func (s *TokenDetailService) GetTokenTransfers(ctx context.Context, req *pb.GetTokenTransfersRequest) (*pb.Reply, error) {
	if err := utils.ValidateNil(req); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}
	if err := utils.ValidateRequiredFields("contract_address", req.GetContractAddress()); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}

	chainID := req.GetChainId()
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Normalize pagination
	var pagination *pb.PaginationRequest
	if req.GetPagination() != nil {
		pagination = req.GetPagination()
	} else {
		pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, _, _ := normalizePagination(
		pagination.GetPage(),
		pagination.GetPageSize(),
		pagination.GetCursor(),
		MaxPageSize,
	)

	// Query token transfers from database
	contractAddress := strings.ToLower(req.GetContractAddress())

	queryCtx, queryCancel := utils.WithDBTimeoutContext(ctx)
	defer queryCancel()

	// Build query
	query := db.Get().WithContext(queryCtx).
		Model(&models.TokenTransfer{}).
		Where("contract_address = ?", contractAddress).
		Where("deleted_at IS NULL")

	// Apply address filter (from or to)
	if req.GetAddress() != "" {
		addr := strings.ToLower(req.GetAddress())
		query = query.Where("(from_address = ? OR to_address = ?)", addr, addr)
	}

	// Apply method filter
	if req.GetMethod() != "" {
		query = query.Where("method = ?", req.GetMethod())
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		global.Logger.Errorf("failed to count token transfers: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query transfers", err), err
	}

	// Get contract info for token symbol and decimals
	var contract models.ContractInfo
	err := db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ?", req.GetContractAddress(), chainID).
		First(&contract).Error

	tokenSymbol := ""
	if err == nil {
		tokenSymbol = contract.TokenSymbol
	}

	// Fetch transfers with pagination
	var transfers []models.TokenTransfer
	err = query.
		Order("block_timestamp DESC, log_index DESC").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Find(&transfers).Error

	if err != nil {
		global.Logger.Errorf("failed to query token transfers: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query transfers", err), err
	}

	// Convert to response items
	items := make([]*pb.TokenTransferInfo, 0, len(transfers))
	for _, transfer := range transfers {
		item := &pb.TokenTransferInfo{
			TxHash:      transfer.TransactionHash,
			Method:      transfer.Method,
			Timestamp:   int64(transfer.BlockTimestamp),
			TimeAgo:     formatTimeAgo(transfer.BlockTimestamp),
			FromAddress: transfer.FromAddress,
			ToAddress:   transfer.ToAddress,
			Amount:      formatHolderAmountWithDenom(transfer.Amount, tokenSymbol),
		}
		items = append(items, item)
	}

	// Build pagination info
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	response := &pb.TokenTransfersResponse{
		Items: items,
		Pagination: &pb.PaginationInfo{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
			HasPrev:    page > 1,
			HasNext:    page < totalPages,
		},
	}

	return MarshalAndReply(response, "marshal token transfers error")
}

// GetTokenHolders returns holder rankings for a token
func (s *TokenDetailService) GetTokenHolders(ctx context.Context, req *pb.GetTokenHoldersRequest) (*pb.Reply, error) {
	if err := utils.ValidateNil(req); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}
	if err := utils.ValidateRequiredFields("contract_address", req.GetContractAddress()); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}

	chainID := req.GetChainId()
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Normalize pagination
	var pagination *pb.PaginationRequest
	if req.GetPagination() != nil {
		pagination = req.GetPagination()
	} else {
		pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, _, _ := normalizePagination(
		pagination.GetPage(),
		pagination.GetPageSize(),
		pagination.GetCursor(),
		MaxPageSize,
	)

	sortBy := req.GetSortBy()
	if sortBy == "" {
		sortBy = "balance" // Default sort by balance
	}

	sortOrder := req.GetSortOrder()
	if sortOrder != "" && sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// Get contract info to find token denom
	var contract models.ContractInfo
	err := db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ?", req.GetContractAddress(), chainID).
		First(&contract).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: "contract not found",
				Data:    nil,
			}, nil
		}
		global.Logger.Errorf("failed to query contract: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query contract", err), err
	}

	// Use PeggyDenom (chain denom like "peggy0x...") for account_balances query
	// account_balances.denom stores chain denom format, not token symbol
	denom := contract.PeggyDenom
	if denom == "" {
		// Fallback to TokenSymbol for backward compatibility
		denom = contract.TokenSymbol
	}
	if denom == "" {
		denom = req.GetContractAddress()
	}

	// Query account balances for this denom
	var balances []models.AccountBalance
	query := db.Get().WithContext(ctx).
		Model(&models.AccountBalance{}).
		Where("denom = ?", denom).
		Where("amount != ?", "0") // Exclude zero balances

	// Get total count
	var total int64
	query.Count(&total)

	// Apply pagination and sorting
	if sortBy == "balance" {
		if sortOrder == "desc" {
			query = query.Order("amount DESC")
		} else {
			query = query.Order("amount ASC")
		}
	}

	err = query.Offset(int(offset)).Limit(int(pageSize)).Find(&balances).Error
	if err != nil {
		global.Logger.Errorf("failed to query balances: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query balances", err), err
	}

	// Get token price for USD value calculation
	var tokenPrice float64
	if contract.TokenSymbol != "" {
		tokenConfig, err := LoadTokenConfigFromConf(nil, contract.TokenSymbol)
		if err == nil {
			marketData, _ := GetTokenMarketDataFromCoinGecko(ctx, tokenConfig)
			if marketData != nil {
				tokenPrice = marketData.PriceUSD
			}
		}
	}

	// Calculate total supply for percentage calculation
	totalSupply := calculateTotalSupply(ctx, denom)

	// Batch fetch address labels
	addresses := extractAddressesFromBalances(balances)
	addressLabelsMap := s.batchGetAddressLabels(ctx, chainID, addresses)

	// Convert to holder info
	items := make([]*pb.TokenHolderInfo, 0, len(balances))
	for i, balance := range balances {
		holderInfo := &pb.TokenHolderInfo{
			Rank:       int32(offset) + int32(i) + 1,
			Address:    balance.Address,
			Balance:    formatHolderAmountWithDenom(balance.Amount, contract.TokenSymbol),
			Percentage: calculateHolderPercentage(balance.Amount, totalSupply),
			ValueUsd:   calculateHolderValueUSD(balance.Amount, tokenPrice),
		}

		// Set label information if available
		if label, ok := addressLabelsMap[balance.Address]; ok {
			holderInfo.Label = label.LabelValue
			holderInfo.LabelType = label.LabelType
		}

		items = append(items, holderInfo)
	}

	// Build pagination info
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	response := &pb.TokenHoldersResponse{
		Items: items,
		Pagination: &pb.PaginationInfo{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
			HasPrev:    page > 1,
			HasNext:    page < totalPages,
		},
	}

	return MarshalAndReply(response, "marshal token holders error")
}

// GetTokenHoldersStats returns holder statistics
func (s *TokenDetailService) GetTokenHoldersStats(ctx context.Context, req *pb.GetTokenHoldersStatsRequest) (*pb.Reply, error) {
	if err := utils.ValidateNil(req); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}
	if err := utils.ValidateRequiredFields("contract_address", req.GetContractAddress()); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}

	chainID := req.GetChainId()
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Get contract info to find token denom
	var contract models.ContractInfo
	err := db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ?", req.GetContractAddress(), chainID).
		First(&contract).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: "contract not found",
				Data:    nil,
			}, nil
		}
		global.Logger.Errorf("failed to query contract: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query contract", err), err
	}

	// Use PeggyDenom (chain denom like "peggy0x...") for account_balances query
	// account_balances.denom stores chain denom format, not token symbol
	denom := contract.PeggyDenom
	if denom == "" {
		// Fallback to TokenSymbol for backward compatibility
		denom = contract.TokenSymbol
	}
	if denom == "" {
		denom = req.GetContractAddress()
	}

	// Get total holder count
	var totalHolders int64
	db.Get().WithContext(ctx).
		Model(&models.AccountBalance{}).
		Where("denom = ?", denom).
		Where("amount != ?", "0").
		Count(&totalHolders)

	// Get top 10 and top 100 holders for percentage calculation
	var top10Balances []models.AccountBalance
	var top100Balances []models.AccountBalance

	db.Get().WithContext(ctx).
		Where("denom = ?", denom).
		Where("amount != ?", "0").
		Order("amount DESC").
		Limit(100).
		Find(&top100Balances)

	top10Balances = top100Balances
	if len(top100Balances) > 10 {
		top10Balances = top100Balances[:10]
	}

	// Calculate total supply
	var totalSupply *big.Int
	for _, balance := range top100Balances {
		amount, ok := new(big.Int).SetString(balance.Amount, 10)
		if ok {
			if totalSupply == nil {
				totalSupply = new(big.Int)
			}
			totalSupply.Add(totalSupply, amount)
		}
	}

	// Calculate top 10 and top 100 percentage
	top10Percentage := "0"
	top100Percentage := "0"
	circulatingSupply := int64(0)

	if totalSupply != nil && totalSupply.Sign() > 0 {
		circulatingSupply = totalSupply.Int64()

		// Calculate percentages with 2 decimal places
		if top10Balances != nil {
			top10Amount := new(big.Int)
			for _, balance := range top10Balances {
				amount, _ := new(big.Int).SetString(balance.Amount, 10)
				top10Amount.Add(top10Amount, amount)
			}

			if top10Amount.Sign() > 0 {
				top10Ratio := new(big.Float).Quo(
					new(big.Float).SetInt(top10Amount),
					new(big.Float).SetInt(totalSupply),
				)
				top10Ratio.Mul(top10Ratio, big.NewFloat(100))
				top10Percentage = fmt.Sprintf("%.2f", top10Ratio)
			}
		}

		if top100Balances != nil {
			top100Amount := new(big.Int)
			for _, balance := range top100Balances {
				amount, _ := new(big.Int).SetString(balance.Amount, 10)
				top100Amount.Add(top100Amount, amount)
			}

			if top100Amount.Sign() > 0 {
				top100Ratio := new(big.Float).Quo(
					new(big.Float).SetInt(top100Amount),
					new(big.Float).SetInt(totalSupply),
				)
				top100Ratio.Mul(top100Ratio, big.NewFloat(100))
				top100Percentage = fmt.Sprintf("%.2f", top100Ratio)
			}
		}
	}

	response := &pb.TokenHoldersStatsResponse{
		TotalHolders:      totalHolders,
		Top10Percentage:   top10Percentage,
		Top100Percentage:  top100Percentage,
		CirculatingSupply: circulatingSupply,
	}

	return MarshalAndReply(response, "marshal token holders stats error")
}

// GetTokenContractCode returns contract bytecode and source code
func (s *TokenDetailService) GetTokenContractCode(ctx context.Context, req *pb.GetTokenContractCodeRequest) (*pb.Reply, error) {
	if err := utils.ValidateNil(req); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}
	if err := utils.ValidateRequiredFields("contract_address", req.GetContractAddress()); err != nil {
		return ErrorReply(err.Code, err.Message, err.Err), err.Err
	}

	chainID := req.GetChainId()
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Query contract source files
	var sourceFiles []models.ContractSourceFile
	err := db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ?", req.GetContractAddress(), chainID).
		Order("created_at DESC").
		Find(&sourceFiles).Error

	if err != nil {
		global.Logger.Errorf("failed to query contract source files: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query contract code", err), err
	}

	response := &pb.TokenContractCodeResponse{
		ContractName: "",
		Bytecode:     "",
		SourceCode:   "",
		Verified:     len(sourceFiles) > 0,
	}

	// If source files exist, return the first source file
	if len(sourceFiles) > 0 {
		for _, file := range sourceFiles {
			if file.FileType == "source" {
				response.ContractName = file.FileName
				response.SourceCode = file.FileContent
				break
			}
		}
	}

	return MarshalAndReply(response, "marshal token contract code error")
}

// Helper functions

func formatPrice(price float64) string {
	if price >= 1000 {
		return fmt.Sprintf("$%s", formatLargeNumber(price))
	} else if price >= 1 {
		return fmt.Sprintf("$%.2f", price)
	} else if price >= 0.01 {
		return fmt.Sprintf("$%.4f", price)
	} else if price > 0 {
		return fmt.Sprintf("$%.8f", price)
	}
	return "-"
}

func formatLargeNumber(value float64) string {
	if value >= 1e9 {
		return fmt.Sprintf("$%.1fB", value/1e9)
	} else if value >= 1e6 {
		return fmt.Sprintf("$%.1fM", value/1e6)
	} else if value >= 1e3 {
		return fmt.Sprintf("$%.1fK", value/1e3)
	} else if value > 0 {
		return fmt.Sprintf("$%.2f", value)
	}
	return "-"
}

// formatHolderAmount formats token amount from base units to human-readable format
func formatHolderAmount(amount string) string {
	// Default 18 decimals for ERC-20
	decimals := int32(18)

	// Parse amount string to big.Int
	amountInt, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return amount
	}

	// Divide by 10^decimals to get token amount
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	quotient := new(big.Int).Div(amountInt, divisor)
	remainder := new(big.Int).Mod(amountInt, divisor)

	if remainder.Sign() == 0 {
		return quotient.String()
	}

	// Format with decimals
	remainderStr := remainder.String()
	// Pad with zeros if needed
	for len(remainderStr) < int(decimals) {
		remainderStr = "0" + remainderStr
	}
	// Remove trailing zeros
	remainderStr = strings.TrimRight(remainderStr, "0")
	if remainderStr == "" {
		return quotient.String()
	}

	return fmt.Sprintf("%s.%s", quotient.String(), remainderStr)
}

// sortHolders sorts holders by specified field
func sortHolders(items []*pb.TokenHolderInfo, sortBy, sortOrder string) []*pb.TokenHolderInfo {
	sort.Slice(items, func(i, j int) bool {
		var iValue, jValue float64

		switch sortBy {
		case "balance":
			// Parse balance string to float64 (simple version)
			iValue, _ = strconv.ParseFloat(strings.Trim(items[i].Balance, " $MK"), 64)
			jValue, _ = strconv.ParseFloat(strings.Trim(items[j].Balance, " $MK"), 64)
		case "percentage":
			iValue, _ = strconv.ParseFloat(items[i].Percentage, 64)
			jValue, _ = strconv.ParseFloat(items[j].Percentage, 64)
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

// batchGetAddressLabels fetches address labels for multiple addresses in batch
func (s *TokenDetailService) batchGetAddressLabels(ctx context.Context, chainID string, addresses []string) map[string]*models.AddressLabel {
	result := make(map[string]*models.AddressLabel)

	if len(addresses) == 0 {
		return result
	}

	// Convert addresses to Ethereum format for database query
	ethAddresses := make([]string, len(addresses))
	for i, addr := range addresses {
		ethAddresses[i] = models.ConvertAddressToEthForStorage(addr)
	}

	// Query address labels from database
	var labels []models.AddressLabel
	err := db.Get().WithContext(ctx).
		Where("chain_id = ? AND address IN ?", chainID, ethAddresses).
		Where("visible = ?", true).
		Find(&labels).Error

	if err != nil {
		global.Logger.Warnf("failed to query address labels: %v", err)
		return result
	}

	// Build map: use original address as key
	for _, label := range labels {
		// Find the original address corresponding to the converted address
		for i, ethAddr := range ethAddresses {
			if ethAddr == label.Address {
				result[addresses[i]] = &label
				break
			}
		}
	}

	return result
}

// extractAddressesFromBalances extracts addresses from account balances
func extractAddressesFromBalances(balances []models.AccountBalance) []string {
	addresses := make([]string, 0, len(balances))
	for _, balance := range balances {
		addresses = append(addresses, balance.Address)
	}
	return addresses
}

// calculateTotalSupply calculates the total supply for a denom
func calculateTotalSupply(ctx context.Context, denom string) *big.Int {
	var totalSupply *big.Int

	// Query all non-zero balances for this denom
	var balances []models.AccountBalance
	db.Get().WithContext(ctx).
		Where("denom = ?", denom).
		Where("amount != ?", "0").
		Find(&balances)

	// Sum up all balances
	for _, balance := range balances {
		amount, ok := new(big.Int).SetString(balance.Amount, 10)
		if ok {
			if totalSupply == nil {
				totalSupply = new(big.Int)
			}
			totalSupply.Add(totalSupply, amount)
		}
	}

	return totalSupply
}

// calculateHolderPercentage calculates the percentage of total supply held by a holder
func calculateHolderPercentage(amount string, totalSupply *big.Int) string {
	if totalSupply == nil || totalSupply.Sign() == 0 {
		return "0"
	}

	holderAmount, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return "0"
	}

	if holderAmount.Sign() == 0 {
		return "0"
	}

	// Calculate percentage: (holderAmount / totalSupply) * 100
	percentage := new(big.Float).Quo(
		new(big.Float).SetInt(holderAmount),
		new(big.Float).SetInt(totalSupply),
	)
	percentage.Mul(percentage, big.NewFloat(100))

	// Format with 2 decimal places
	result, _ := percentage.Float64()
	return fmt.Sprintf("%.2f", result)
}

// calculateHolderValueUSD calculates the USD value of a holder's balance
func calculateHolderValueUSD(amount string, priceUSD float64) string {
	if priceUSD <= 0 {
		return "-"
	}

	// Parse amount
	amountInt, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return "-"
	}

	// Convert to token amount (divide by 10^18)
	decimals := int64(18)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil)
	tokenAmount := new(big.Float).Quo(
		new(big.Float).SetInt(amountInt),
		new(big.Float).SetInt(divisor),
	)

	// Calculate USD value
	valueUSD := new(big.Float).Mul(tokenAmount, big.NewFloat(priceUSD))

	result, _ := valueUSD.Float64()
	return formatPrice(result)
}

// formatHolderAmountWithDenom formats token amount with symbol
func formatHolderAmountWithDenom(amount, symbol string) string {
	formatted := formatHolderAmount(amount)
	if symbol != "" {
		return fmt.Sprintf("%s %s", formatted, symbol)
	}
	return formatted
}

// normalizeTokenStandard converts contract type to standard format with hyphen
// e.g., "erc20" -> "erc-20", "erc721" -> "erc-721"
func normalizeTokenStandard(contractType string) string {
	if contractType == "" {
		return ""
	}
	// Convert known contract types to standard format with hyphen
	switch strings.ToLower(contractType) {
	case "erc20":
		return "erc-20"
	case "erc721":
		return "erc-721"
	case "erc1155":
		return "erc-1155"
	default:
		return contractType
	}
}

// formatTimeAgo formats a Unix timestamp to "X minutes ago" format
func formatTimeAgo(timestamp uint64) string {
	t := time.Unix(int64(timestamp), 0)
	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else if duration < 30*24*time.Hour {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	} else {
		return t.Format("2006-01-02")
	}
}
