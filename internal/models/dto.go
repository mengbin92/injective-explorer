// Package models provides Data Transfer Objects (DTOs) for API responses.
// DTOs exclude internal database fields like gorm.Model to prevent exposing
// sensitive database information to clients.
//
// Note: DTO types are now defined in proto files (api/explorer/v1/demo.proto)
// and generated as Go types. This file provides conversion functions from
// database models to proto DTO messages.
package models

import (
	pb "injective-explorer/api/explorer/v1"
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
		}
	} else {
		// For non-detail mode, also decompress Messages if present
		messagesStr := decompressForDTO(tx.Messages)

		return &pb.TransactionDTO{
			BlockNumber:        tx.BlockNumber,
			BlockUnixTimestamp: tx.BlockUnixTimestamp,
			Hash:               tx.Hash,
			ErrorLog:           tx.ErrorLog,
			Messages:           messagesStr,
			Signer:             tx.Signer,
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
