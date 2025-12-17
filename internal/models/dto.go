// Package models provides Data Transfer Objects (DTOs) for API responses.
// DTOs exclude internal database fields like gorm.Model to prevent exposing
// sensitive database information to clients.
//
// Note: DTO types are now defined in proto files (api/explorer/v1/demo.proto)
// and generated as Go types. This file provides conversion functions from
// database models to proto DTO messages.
package models

import (
	"time"

	pb "injective-explorer/api/explorer/v1"

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
func ToTransactionDTO(tx *Transaction, isDetail bool) *pb.TransactionDTO {
	if tx == nil {
		return nil
	}

	// Helper function to decompress and format data for DTO
	// Returns the decompressed data as a string (base64 for binary, plain text for text)
	// This ensures that compressed data stored in the database is properly decompressed before returning to clients
	decompressForDTO := func(compressedData string) string {
		if compressedData == "" {
			return ""
		}
		// Decompress the data (handles both compressed and uncompressed data)
		decompressed, err := DecompressData(compressedData)
		if err != nil {
			// If decompression fails, the data might be in old format (uncompressed)
			// Try to decode as base64 if it looks like base64, otherwise return as-is
			decoded := decodeBinaryData(compressedData)
			if len(decoded) > 0 && len(decoded) != len(compressedData) {
				// Successfully decoded as base64 (size changed), check if it's text or binary
				if isValidUTF8(decoded) {
					return string(decoded)
				}
				// Binary data, return base64 encoded (original format)
				return compressedData
			}
			// Not base64 or same size, return as plain text (backward compatibility)
			return compressedData
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
		// Decompress compressed fields before returning
		dataStr := decompressForDTO(tx.Data)
		messagesStr := decompressForDTO(tx.Messages)
		logsStr := decompressForDTO(tx.Logs)

		// Addresses are already stored in Ethereum format (0x...) in the database, so no conversion needed
		return &pb.TransactionDTO{
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
	} else {
		// For non-detail mode, also decompress Messages if present
		messagesStr := decompressForDTO(tx.Messages)

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
