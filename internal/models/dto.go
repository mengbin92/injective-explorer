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
)

const (
	DTOModeWithDetail    = true
	DTOModeWithoutDetail = false
)

// convertSignaturesToDTO converts database signature JSON strings to proto TransactionSignature objects
func convertSignaturesToDTO(sigStrings []string) []*pb.TransactionSignature {
	if len(sigStrings) == 0 {
		return nil
	}

	signatures := make([]*pb.TransactionSignature, 0, len(sigStrings))
	for _, sigStr := range sigStrings {
		if sigStr == "" {
			continue
		}

		// Try to parse as JSON object (new format with address)
		var sigData map[string]interface{}
		if err := sonic.Unmarshal([]byte(sigStr), &sigData); err == nil {
			sig := &pb.TransactionSignature{}

			// Extract pubkey
			if v, ok := sigData["pubkey"].(string); ok {
				sig.Pubkey = v
			}

			// Extract address
			if v, ok := sigData["address"].(string); ok {
				sig.Address = v
			}

			// Extract sequence
			switch v := sigData["sequence"].(type) {
			case float64:
				sig.Sequence = uint64(v)
			case int64:
				sig.Sequence = uint64(v)
			case uint64:
				sig.Sequence = v
			}

			// Extract signature
			if v, ok := sigData["signature"].(string); ok {
				sig.Signature = v
			}

			// Only add if we have at least pubkey or signature
			if sig.Pubkey != "" || sig.Signature != "" {
				signatures = append(signatures, sig)
			}
		}
	}

	if len(signatures) == 0 {
		return nil
	}
	return signatures
}

// DTO types are now defined in proto files:
// - pb.AccountDTO
// - pb.AccountBalanceDTO
// - pb.BlockDTO
// - pb.TransactionDTO
// - pb.ContractInfoDTO
// - pb.ContractTagInfoDTO

// ToAccountDTO converts an Account model to proto AccountDTO.
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
// The isDetail parameter controls whether to include all fields or only summary fields.
func ToBlockDTO(block *Block, isDetail bool) *pb.BlockDTO {
	if block == nil {
		return nil
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
			Txs:                block.Txs,
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
// The isDetail parameter controls whether to include all fields or only summary fields.
// Automatically decompresses compressed data and loads from object storage if available.
func ToTransactionDTO(tx *Transaction, isDetail bool) *pb.TransactionDTO {
	if tx == nil {
		return nil
	}

	// Helper function to load and decompress data from database or object storage.
	loadAndDecompressForDTO := func(dbField, storageKey string) string {
		// Try object storage first if storage key exists
		if storageKey != "" {
			storageClient := storage.Get()
			ctx := context.Background()
			if storageData, err := storageClient.GetObject(ctx, storageKey); err == nil {
				// Try to decompress (may already be uncompressed for backward compatibility)
				decompressed, decompErr := DecompressDataFromBytes(storageData)
				if decompErr != nil {
					// Decompression failed, assume uncompressed (backward compatibility)
					decompressed = storageData
				}
				// Check if data is text or binary
				if isValidUTF8(decompressed) {
					return string(decompressed)
				}
				// Binary data, encode to base64 for safe JSON transmission
				return encodeBinaryData(decompressed)
			}
			// Object storage failed or no storage key, try database field
		}

		// No storage key or object storage failed, use database field
		if dbField == "" {
			return ""
		}
		// Decompress data (handles both compressed and uncompressed)
		decompressed, err := DecompressData(dbField)
		if err != nil {
			// Decompression failed, might be old format (uncompressed or base64)
			decoded := decodeBinaryData(dbField)
			if len(decoded) > 0 && len(decoded) != len(dbField) {
				// Successfully decoded as base64, check if text or binary
				if isValidUTF8(decoded) {
					return string(decoded)
				}
				// Binary data, return base64 encoded (original format)
				return dbField
			}
			// Not base64 or same size, return as plain text (backward compatibility)
			return dbField
		}
		// Successfully decompressed, check if text or binary
		if isValidUTF8(decompressed) {
			return string(decompressed)
		}
		// Binary data, encode to base64 for safe JSON transmission
		return encodeBinaryData(decompressed)
	}

	if isDetail {
		// Load and decompress fields (object storage first, then database)
		dataStr := loadAndDecompressForDTO(tx.Data, tx.DataStorageKey)
		messagesStr := loadAndDecompressForDTO(tx.Messages, tx.MessagesStorageKey)
		logsStr := loadAndDecompressForDTO(tx.Logs, tx.LogsStorageKey)
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
			Signatures:         convertSignaturesToDTO(tx.Signatures),
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

		// Note: EVM transaction details are loaded separately by the service layer

		return txDTO
	} else {
		// For non-detail mode, load and decompress Messages field
		messagesStr := loadAndDecompressForDTO(tx.Messages, tx.MessagesStorageKey)
		return &pb.TransactionDTO{
			BlockNumber:        tx.BlockNumber,
			BlockUnixTimestamp: tx.BlockUnixTimestamp,
			Hash:               tx.Hash,
			ErrorLog:           tx.ErrorLog,
			Messages:           messagesStr,
			GasUsed:            tx.GasUsed,
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

// ToAddressLabelDTO converts an AddressLabel model to AddressLabelDTO proto message.
func ToAddressLabelDTO(label *AddressLabel) *pb.AddressLabelDTO {
	if label == nil {
		return nil
	}

	var createdAt string
	if !label.CreatedAt.IsZero() {
		createdAt = label.CreatedAt.Format(time.RFC3339)
	}

	var updatedAt string
	if !label.UpdatedAt.IsZero() {
		updatedAt = label.UpdatedAt.Format(time.RFC3339)
	}

	return &pb.AddressLabelDTO{
		Id:          uint64(label.ID),
		Address:     label.Address,
		ChainId:     label.ChainID,
		LabelType:   label.LabelType,
		LabelValue:  label.LabelValue,
		LabelColor:  label.LabelColor,
		Description: label.Description,
		CreatedBy:   label.CreatedBy,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

// ToAddressLabelDTOs converts a slice of AddressLabel models to proto AddressLabelDTOs.
func ToAddressLabelDTOs(labels []AddressLabel) []*pb.AddressLabelDTO {
	if labels == nil {
		return nil
	}
	dtos := make([]*pb.AddressLabelDTO, 0, len(labels))
	for _, label := range labels {
		dtos = append(dtos, ToAddressLabelDTO(&label))
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
		Accounts:          accounts,
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

// ToTokenInfoDTO converts an APIToken model to TokenInfo proto message.
func ToTokenInfoDTO(token *APIToken) *pb.TokenInfo {
	if token == nil {
		return nil
	}

	lastUsedAt := ""
	if token.LastUsedAt != nil {
		lastUsedAt = token.LastUsedAt.Format(time.RFC3339)
	}

	return &pb.TokenInfo{
		Id:           int64(token.ID),
		Name:         token.Name,
		TokenPreview: token.TokenPreview,
		PlanType:     token.PlanType,
		Status:       token.Status,
		CreatedAt:    token.CreatedAt.Format(time.RFC3339),
		LastUsedAt:   lastUsedAt,
	}
}

// ToTokenDTOs converts a slice of APIToken models to TokenInfo proto messages.
func ToTokenDTOs(tokens []APIToken) []*pb.TokenInfo {
	if tokens == nil {
		return nil
	}
	dtos := make([]*pb.TokenInfo, 0, len(tokens))
	for _, token := range tokens {
		dtos = append(dtos, ToTokenInfoDTO(&token))
	}
	return dtos
}
