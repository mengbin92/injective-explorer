// Package models provides Data Transfer Objects (DTOs) for API responses.
// DTOs exclude internal database fields like gorm.Model to prevent exposing
// sensitive database information to clients.
//
// Note: DTO types are now defined in proto files (api/explorer/v1/explorer.proto)
// and generated as Go types. This file provides conversion functions from
// database models to proto DTO messages.
package models

import (
	"context"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/storage"

	"github.com/bytedance/sonic"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	DTOModeWithDetail    = true
	DTOModeWithoutDetail = false
)

// DTO types are now defined in proto files:
// - pb.AccountDTO
// - pb.AccountBalanceDTO
// - pb.BlockDTO
// - pb.TransactionDTO
// - pb.ContractInfoDTO
// - pb.ContractTagInfoDTO

// ToAccountDTO converts an Account model to proto AccountDTO.
// Addresses are already stored in Ethereum format (0x...) in the database, so no conversion needed.
func ToAccountDTO(account *Account) *pb.AccountDTO {
	if account == nil {
		return nil
	}

	return &pb.AccountDTO{
		Address:     account.Address,
		BaseAccount: account.BaseAccount,
		CodeHash:    account.CodeHash,
	}
}

// ToAccountBalanceDTO converts an AccountBalance model to proto AccountBalanceDTO.
// Addresses are already stored in Ethereum format (0x...) in the database, so no conversion needed.
func ToAccountBalanceDTO(balance *AccountBalance) *pb.AccountBalanceDTO {
	if balance == nil {
		return nil
	}

	return &pb.AccountBalanceDTO{
		Address: balance.Address,
		Denom:   balance.Denom,
		Amount:  balance.Amount,
	}
}

// ToAccountBalanceDTOs converts a slice of AccountBalance models to proto AccountBalanceDTOs.
func ToAccountBalanceDTOs(balances []AccountBalance) []*pb.AccountBalanceDTO {
	if balances == nil {
		return nil
	}
	dtos := make([]*pb.AccountBalanceDTO, 0, len(balances))
	for _, balance := range balances {
		dtos = append(dtos, ToAccountBalanceDTO(&balance))
	}
	return dtos
}

// ToBlockDTO converts a Block model to proto BlockDTO.
// Addresses are already stored in Ethereum format (0x...) in the database, so no conversion needed.
// Txs field is converted from JSON strings to JSON objects (google.protobuf.Struct) for frontend use.
func ToBlockDTO(block *Block, isDetail bool) *pb.BlockDTO {
	if block == nil {
		return nil
	}

	// Convert Txs from JSON strings to JSON objects
	var txs []*structpb.Struct
	if len(block.Txs) > 0 {
		txs = make([]*structpb.Struct, 0, len(block.Txs))
		for _, txStr := range block.Txs {
			if txStr == "" {
				continue
			}
			// Parse JSON string to structpb.Struct
			var txStruct structpb.Struct
			if err := protojson.Unmarshal([]byte(txStr), &txStruct); err == nil {
				txs = append(txs, &txStruct)
			}
			// If parsing fails, skip this transaction (graceful degradation)
		}
	}

	if isDetail {
		return &pb.BlockDTO{
			Height:             block.Height,
			Proposer:           block.Proposer,
			Moniker:            block.Moniker,
			BlockHash:          block.BlockHash,
			ParentHash:         block.ParentHash,
			NumPrecommits:      block.NumPrecommits,
			NumTxs:             block.NumTxs,
			TotalTxs:           block.TotalTxs,
			Txs:                txs,
			Timestamp:          block.Timestamp,
			BlockUnixTimestamp: block.BlockUnixTimestamp,
		}
	} else {
		return &pb.BlockDTO{
			Height:             block.Height,
			Proposer:           block.Proposer,
			BlockHash:          block.BlockHash,
			NumTxs:             block.NumTxs,
			BlockUnixTimestamp: block.BlockUnixTimestamp,
		}
	}
}

// ToBlockDTOs converts a slice of Block models to proto BlockDTOs.
func ToBlockDTOs(blocks []Block) []*pb.BlockDTO {
	if blocks == nil {
		return nil
	}
	dtos := make([]*pb.BlockDTO, 0, len(blocks))
	for _, block := range blocks {
		dtos = append(dtos, ToBlockDTO(&block, DTOModeWithoutDetail))
	}
	return dtos
}

// ToTransactionDTO converts a Transaction model to proto TransactionDTO.
// It automatically decompresses compressed data fields before returning.
// It also loads data from object storage if storage keys exist.
func ToTransactionDTO(tx *Transaction, isDetail bool) *pb.TransactionDTO {
	if tx == nil {
		return nil
	}

	// Helper function to load and decompress data from database or object storage
	// Returns the decompressed data as a string (base64 for binary, plain text for text)
	// This ensures that data stored in object storage or compressed data stored in the database
	// is properly loaded and decompressed before returning to clients
	loadAndDecompressForDTO := func(dbField, storageKey string) string {
		// If storage key exists, try to load from object storage first
		if storageKey != "" {
			storageClient := storage.Get()
			ctx := context.Background()
			if storageData, err := storageClient.GetObject(ctx, storageKey); err == nil {
				// Decompress data from object storage (may be compressed binary format)
				decompressed, decompErr := DecompressDataFromBytes(storageData)
				if decompErr != nil {
					// If decompression fails, assume it's uncompressed (backward compatibility)
					decompressed = storageData
				}
				// Successfully decompressed, check if it's text or binary
				if isValidUTF8(decompressed) {
					return string(decompressed)
				}
				// For binary data, encode to base64 for safe JSON transmission
				return encodeBinaryData(decompressed)
			}
			// If object storage fails, fall back to database field
		}

		// No storage key or object storage failed, decompress database field
		if dbField == "" {
			return ""
		}
		// Decompress the data (handles both compressed and uncompressed data)
		decompressed, err := DecompressData(dbField)
		if err != nil {
			// If decompression fails, the data might be in old format (uncompressed)
			// Try to decode as base64 if it looks like base64, otherwise return as-is
			decoded := decodeBinaryData(dbField)
			if len(decoded) > 0 && len(decoded) != len(dbField) {
				// Successfully decoded as base64 (size changed), check if it's text or binary
				if isValidUTF8(decoded) {
					return string(decoded)
				}
				// Binary data, return base64 encoded (original format)
				return dbField
			}
			// Not base64 or same size, return as plain text (backward compatibility)
			return dbField
		}
		// Successfully decompressed, check if it's text or binary
		if isValidUTF8(decompressed) {
			// Return as plain text
			return string(decompressed)
		}
		// For binary data, encode to base64 for safe JSON transmission
		// This matches the original behavior for binary data
		return encodeBinaryData(decompressed)
	}

	if isDetail {
		// Load and decompress fields (checking object storage first)
		dataStr := loadAndDecompressForDTO(tx.Data, tx.DataStorageKey)
		messagesStr := loadAndDecompressForDTO(tx.Messages, tx.MessagesStorageKey)
		logsStr := loadAndDecompressForDTO(tx.Logs, tx.LogsStorageKey)

		// Addresses are already stored in Ethereum format (0x...) in the database, so no conversion needed
		txDTO := &pb.TransactionDTO{
			Id:                 tx.Id,
			BlockNumber:        tx.BlockNumber,
			BlockTimestamp:     tx.BlockTimestamp,
			Hash:               tx.Hash,
			Code:               tx.Code,
			Data:               dataStr,
			Info:               tx.Info,
			GasWanted:          tx.GasWanted,
			GasUsed:            tx.GasUsed,
			GasFee:             tx.GasFee,
			Codespace:          tx.Codespace,
			Events:             tx.Events,
			TxType:             tx.TxType,
			Messages:           messagesStr,
			Signatures:         tx.Signatures,
			Memo:               tx.Memo,
			TxNumber:           tx.TxNumber,
			BlockUnixTimestamp: tx.BlockUnixTimestamp,
			ErrorLog:           tx.ErrorLog,
			Logs:               logsStr,
			ClaimIds:           tx.ClaimIds,
			Signer:             tx.Signer,
			To:                 tx.To,
			Value:              tx.Value,
		}

		// Load EVM transaction details if available
		if tx.EVMHash != "" {
			evmDetails, err := LoadEVMTransactionDetails(context.Background(), tx)
			if err == nil && evmDetails != nil {
				txDTO.EvmTransaction = ToEVMTransactionDTO(evmDetails, tx)
			}
		}

		return txDTO
	} else {
		// For non-detail mode, also decompress Messages if present
		// Load from object storage if storage key exists, otherwise from database
		messagesStr := loadAndDecompressForDTO(tx.Messages, tx.MessagesStorageKey)

		// Addresses are already stored in Ethereum format (0x...) in the database, so no conversion needed
		return &pb.TransactionDTO{
			BlockNumber:        tx.BlockNumber,
			BlockUnixTimestamp: tx.BlockUnixTimestamp,
			Hash:               tx.Hash,
			ErrorLog:           tx.ErrorLog,
			Messages:           messagesStr,
			Signer:             tx.Signer,
			To:                 tx.To,
			Value:              tx.Value,
		}
	}
}

// ToTransactionDTOs converts a slice of Transaction models to proto TransactionDTOs.
func ToTransactionDTOs(txs []Transaction) []*pb.TransactionDTO {
	if txs == nil {
		return nil
	}
	dtos := make([]*pb.TransactionDTO, 0, len(txs))
	for _, tx := range txs {
		dtos = append(dtos, ToTransactionDTO(&tx, DTOModeWithoutDetail))
	}
	return dtos
}

// ToContractTagInfoDTO converts a ContractTag model to ContractTagInfoDTO proto message.
func ToContractTagInfoDTO(tag *ContractTag) *pb.ContractTagInfoDTO {
	if tag == nil {
		return nil
	}

	var createdAt string
	if !tag.CreatedAt.IsZero() {
		createdAt = tag.CreatedAt.Format(time.RFC3339)
	}

	return &pb.ContractTagInfoDTO{
		Id:              uint64(tag.ID),
		ContractAddress: tag.ContractAddress,
		ChainId:         tag.ChainID,
		TagType:         tag.TagType,
		TagValue:        tag.TagValue,
		TagColor:        tag.TagColor,
		Description:     tag.Description,
		CreatedBy:       tag.CreatedBy,
		CreatedAt:       createdAt,
	}
}

// ToContractTagInfoDTOs converts a slice of ContractTag models to proto ContractTagInfoDTOs.
func ToContractTagInfoDTOs(tags []ContractTag) []*pb.ContractTagInfoDTO {
	if tags == nil {
		return nil
	}
	dtos := make([]*pb.ContractTagInfoDTO, 0, len(tags))
	for _, tag := range tags {
		dtos = append(dtos, ToContractTagInfoDTO(&tag))
	}
	return dtos
}

// ToContractInfoDTO converts a ContractInfo model to ContractInfoDTO proto message.
func ToContractInfoDTO(contract *ContractInfo) *pb.ContractInfoDTO {
	if contract == nil {
		return nil
	}

	// Convert tags
	tags := make([]*pb.ContractTagInfoDTO, 0, len(contract.Tags))
	for _, tag := range contract.Tags {
		tags = append(tags, ToContractTagInfoDTO(&tag))
	}

	// Convert metadata
	metadata := make(map[string]string)
	if contract.Metadata != nil {
		for k, v := range contract.Metadata {
			// Convert value to string
			if str, ok := v.(string); ok {
				metadata[k] = str
			} else {
				// Marshal non-string values to JSON string
				if jsonBytes, err := sonic.Marshal(v); err == nil {
					metadata[k] = string(jsonBytes)
				}
			}
		}
	}

	// Format timestamps
	var verifiedAt, createdAt, updatedAt string
	if contract.VerifiedAt != nil {
		verifiedAt = contract.VerifiedAt.Format(time.RFC3339)
	}
	if !contract.CreatedAt.IsZero() {
		createdAt = contract.CreatedAt.Format(time.RFC3339)
	}
	if !contract.UpdatedAt.IsZero() {
		updatedAt = contract.UpdatedAt.Format(time.RFC3339)
	}

	return &pb.ContractInfoDTO{
		Id:                 uint64(contract.ID),
		ContractAddress:    contract.ContractAddress,
		Name:               contract.Name,
		TokenName:          contract.TokenName,
		TokenSymbol:        contract.TokenSymbol,
		ContractType:       contract.ContractType,
		ChainId:            contract.ChainID,
		VerificationStatus: contract.VerificationStatus,
		IsVerified:         contract.IsVerified,
		VerifiedAt:         verifiedAt,
		VerifiedBy:         contract.VerifiedBy,
		Metadata:           metadata,
		Tags:               tags,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}
}

// ToContractInfoDTOs converts a slice of ContractInfo models to proto ContractInfoDTOs.
func ToContractInfoDTOs(contracts []ContractInfo) []*pb.ContractInfoDTO {
	if contracts == nil {
		return nil
	}
	dtos := make([]*pb.ContractInfoDTO, 0, len(contracts))
	for _, contract := range contracts {
		dtos = append(dtos, ToContractInfoDTO(&contract))
	}
	return dtos
}

// LoadEVMTransactionDetails loads EVM transaction details from cache, storage, or database.
// The loading order is: Redis cache -> object storage -> database fallback.
//
// Parameters:
//   - ctx: Context for cancellation
//   - tx: Transaction model
//
// Returns:
//   - *chain.TransactionDetails: EVM transaction details, or nil if not found or not synced
//   - error: Error if the operation fails
func LoadEVMTransactionDetails(ctx context.Context, tx *Transaction) (*chain.TransactionDetails, error) {
	if tx == nil {
		return nil, nil
	}

	// Check if EVM details are synced
	if !tx.EVMDetailsSynced {
		return nil, nil
	}

	evmHash := tx.EVMHash
	if evmHash == "" {
		return nil, nil
	}

	// Ensure EVM hash has 0x prefix for cache key
	if len(evmHash) < 2 || evmHash[:2] != "0x" {
		evmHash = "0x" + evmHash
	}

	// 1. Try to load from Redis cache first (fastest)
	// Note: Import cycle prevention - we use evmcache package directly in services
	// This function is called by services which can import both models and evmcache

	// 2. Load from object storage if storage key exists
	if tx.EVMDetailsStorageKey != "" {
		storageClient := storage.Get()
		if storageClient != nil {
			detailsBytes, err := storageClient.GetObject(ctx, tx.EVMDetailsStorageKey)
			if err == nil {
				var details chain.TransactionDetails
				if err := sonic.Unmarshal(detailsBytes, &details); err == nil {
					return &details, nil
				}
			}
		}
	}

	return nil, nil
}

// ToEVMTransactionDTO converts TransactionDetails to EVMTransactionDTO.
//
// Parameters:
//   - details: EVM transaction details
//   - tx: Transaction model (for block timestamp)
//
// Returns:
//   - *pb.EVMTransactionDTO: EVM transaction DTO
func ToEVMTransactionDTO(details *chain.TransactionDetails, tx *Transaction) *pb.EVMTransactionDTO {
	if details == nil {
		return nil
	}

	// Convert logs
	logs := make([]*pb.EVMLogDTO, 0, len(details.Logs))
	for _, log := range details.Logs {
		logs = append(logs, &pb.EVMLogDTO{
			Address:     log.Address,
			Topics:      log.Topics,
			Data:        log.Data,
			BlockNumber: details.BlockNumber,
			TxHash:      details.Hash,
			TxIndex:     uint32(details.TransactionIndex),
			LogIndex:    uint32(log.Index),
		})
	}

	// Determine if contract call or creation
	isContractCall := details.To != "" && details.ContractAddress == ""
	isContractCreate := details.ContractAddress != ""

	// Use transaction block timestamp if available, otherwise use details timestamp
	blockTimestamp := details.Timestamp.Format(time.RFC3339)
	if tx != nil && tx.BlockTimestamp != "" {
		blockTimestamp = tx.BlockTimestamp
	}

	return &pb.EVMTransactionDTO{
		Hash:             details.Hash,
		BlockNumber:      details.BlockNumber,
		BlockTimestamp:   blockTimestamp,
		From:             details.From,
		To:               details.To,
		Value:            details.Value,
		Nonce:            details.Nonce,
		GasPrice:         details.GasPrice,
		GasLimit:         details.GasLimit,
		GasUsed:          details.GasUsed,
		InputData:        details.Input,
		ContractAddress:  details.ContractAddress,
		MethodId:         details.Method,
		Status:           details.Status,
		Logs:             logs,
		TransactionFee:   details.TransactionFee,
		IsContractCall:   isContractCall,
		IsContractCreate: isContractCreate,
	}
}

// ToEVMTransactionDTOs converts a slice of TransactionDetails to EVMTransactionDTOs.
//
// Parameters:
//   - detailsList: Slice of EVM transaction details
//   - txMap: Map of transaction hash to Transaction model (for block timestamps)
//
// Returns:
//   - []*pb.EVMTransactionDTO: Slice of EVM transaction DTOs
func ToEVMTransactionDTOs(detailsList []*chain.TransactionDetails, txMap map[string]*Transaction) []*pb.EVMTransactionDTO {
	if detailsList == nil {
		return nil
	}
	dtos := make([]*pb.EVMTransactionDTO, 0, len(detailsList))
	for _, details := range detailsList {
		var tx *Transaction
		if txMap != nil {
			tx = txMap[details.Hash]
		}
		if dto := ToEVMTransactionDTO(details, tx); dto != nil {
			dtos = append(dtos, dto)
		}
	}
	return dtos
}

// ToEmailBlacklistDTO converts an EmailBlacklist model to EmailBlacklistDTO proto message.
func ToEmailBlacklistDTO(blacklist *EmailBlacklist) *pb.EmailBlacklistDTO {
	if blacklist == nil {
		return nil
	}

	var createdAt, updatedAt string
	if !blacklist.CreatedAt.IsZero() {
		createdAt = blacklist.CreatedAt.Format(time.RFC3339)
	}
	if !blacklist.UpdatedAt.IsZero() {
		updatedAt = blacklist.UpdatedAt.Format(time.RFC3339)
	}

	return &pb.EmailBlacklistDTO{
		Id:         uint64(blacklist.ID),
		Domain:     blacklist.Domain,
		Status:     blacklist.Status,
		BlockCount: blacklist.BlockCount,
		Note:       blacklist.Note,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
}

// ToEmailBlacklistDTOs converts a slice of EmailBlacklist models to EmailBlacklistDTO proto messages.
func ToEmailBlacklistDTOs(blacklist []EmailBlacklist) []*pb.EmailBlacklistDTO {
	if blacklist == nil {
		return nil
	}
	dtos := make([]*pb.EmailBlacklistDTO, 0, len(blacklist))
	for _, b := range blacklist {
		dtos = append(dtos, ToEmailBlacklistDTO(&b))
	}
	return dtos
}

// ToPlanDTO converts an APIPlan model to PlanDTO proto message.
func ToPlanDTO(plan *APIPlan, tokenCount, userCount int64) *pb.PlanDTO {
	if plan == nil {
		return nil
	}

	var createdAt, updatedAt string
	if !plan.CreatedAt.IsZero() {
		createdAt = plan.CreatedAt.Format(time.RFC3339)
	}
	if !plan.UpdatedAt.IsZero() {
		updatedAt = plan.UpdatedAt.Format(time.RFC3339)
	}

	return &pb.PlanDTO{
		Id:                 uint64(plan.ID),
		Name:               plan.Name,
		PlanType:           plan.PlanType,
		Description:        plan.Description,
		MonthlyLimit:       plan.MonthlyLimit,
		RateLimitPerSecond: int32(plan.RateLimitPerSecond),
		MaxTokens:          int32(plan.MaxTokens),
		Price:              plan.Price,
		Status:             plan.Status,
		TokenCount:         tokenCount,
		UserCount:          userCount,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}
}

// ToPlanDTOs converts a slice of APIPlan models to PlanDTO proto messages.
// It requires a function to get stats for each plan.
func ToPlanDTOs(plans []APIPlan, getStats func(uint) (int64, int64, error)) []*pb.PlanDTO {
	if plans == nil {
		return nil
	}
	dtos := make([]*pb.PlanDTO, 0, len(plans))
	for _, plan := range plans {
		tokenCount, userCount := int64(0), int64(0)
		if getStats != nil {
			tokenCount, userCount, _ = getStats(plan.ID)
		}
		dtos = append(dtos, ToPlanDTO(&plan, tokenCount, userCount))
	}
	return dtos
}

// ToIPBlacklistDTO converts an IPBlacklist model to IPBlacklistDTO proto message.
func ToIPBlacklistDTO(blacklist *IPBlacklist) *pb.IPBlacklistDTO {
	if blacklist == nil {
		return nil
	}

	var createdAt string
	if !blacklist.CreatedAt.IsZero() {
		createdAt = blacklist.CreatedAt.Format(time.RFC3339)
	}

	return &pb.IPBlacklistDTO{
		Id:           uint64(blacklist.ID),
		IpAddress:    blacklist.IPAddress,
		IpLocation:   blacklist.IPLocation,
		Reason:       blacklist.Reason,
		ReasonText:   blacklist.ReasonText,
		BlockCount:   blacklist.BlockCount,
		OperatorName: blacklist.OperatorName,
		BlockType:    blacklist.BlockType,
		CreatedAt:    createdAt,
	}
}

// ToIPBlacklistDTOs converts a slice of IPBlacklist models to IPBlacklistDTO proto messages.
func ToIPBlacklistDTOs(blacklist []IPBlacklist) []*pb.IPBlacklistDTO {
	if blacklist == nil {
		return nil
	}
	dtos := make([]*pb.IPBlacklistDTO, 0, len(blacklist))
	for _, b := range blacklist {
		dtos = append(dtos, ToIPBlacklistDTO(&b))
	}
	return dtos
}

// ToIPRegistrationRecordDTO converts IP registration record data to IPRegistrationRecordDTO proto message.
func ToIPRegistrationRecordDTO(ipAddress, ipLocation string, registerCount int64, lastRegisterTime time.Time, status string) *pb.IPRegistrationRecordDTO {
	var lastRegisterTimeStr string
	if !lastRegisterTime.IsZero() {
		lastRegisterTimeStr = lastRegisterTime.Format(time.RFC3339)
	}

	return &pb.IPRegistrationRecordDTO{
		IpAddress:        ipAddress,
		IpLocation:       ipLocation,
		RegisterCount:    registerCount,
		LastRegisterTime: lastRegisterTimeStr,
		Status:           status,
	}
}

// ToIPDetailDTO converts IP detail data to IPDetailDTO proto message.
func ToIPDetailDTO(ipAddress, ipLocation, status string, firstRegisterTime time.Time, totalRegisters int64, accounts []*pb.IPRegistrationAccountDTO) *pb.IPDetailDTO {
	var firstRegisterTimeStr string
	if !firstRegisterTime.IsZero() {
		firstRegisterTimeStr = firstRegisterTime.Format(time.RFC3339)
	}

	return &pb.IPDetailDTO{
		IpAddress:         ipAddress,
		IpLocation:        ipLocation,
		Status:            status,
		FirstRegisterTime: firstRegisterTimeStr,
		TotalRegisters:    totalRegisters,
		Accounts:         accounts,
	}
}

// ToIPRegistrationAccountDTO converts IP registration record to IPRegistrationAccountDTO proto message.
func ToIPRegistrationAccountDTO(userID, email string, registerTime time.Time) *pb.IPRegistrationAccountDTO {
	var registerTimeStr string
	if !registerTime.IsZero() {
		registerTimeStr = registerTime.Format(time.RFC3339)
	}

	return &pb.IPRegistrationAccountDTO{
		UserId:       userID,
		Email:        email,
		Username:     "", // Username not available in IPRegistrationRecord model
		RegisterTime: registerTimeStr,
	}
}

// ToUsageErrorDTO converts an APIRequestLog model to UsageErrorDTO proto message.
func ToUsageErrorDTO(log *APIRequestLog) *pb.UsageErrorDTO {
	if log == nil {
		return nil
	}

	var timeStr string
	if !log.CreatedAt.IsZero() {
		timeStr = log.CreatedAt.Format(time.RFC3339)
	}

	return &pb.UsageErrorDTO{
		Time:         timeStr,
		IpAddress:    log.IPAddress,
		Endpoint:     log.Endpoint,
		Method:       log.Method,
		StatusCode:   int32(log.StatusCode),
		ErrorMessage: log.ErrorMessage,
		ResponseTime: int32(log.ResponseTime),
	}
}

// ToUsageErrorDTOs converts a slice of APIRequestLog models to UsageErrorDTO proto messages.
func ToUsageErrorDTOs(logs []APIRequestLog) []*pb.UsageErrorDTO {
	if logs == nil {
		return nil
	}
	dtos := make([]*pb.UsageErrorDTO, 0, len(logs))
	for _, log := range logs {
		dtos = append(dtos, ToUsageErrorDTO(&log))
	}
	return dtos
}
