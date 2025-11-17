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
// - pb.ValidatorDTO

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
func ToTransactionDTO(tx *Transaction, isDetail bool) *pb.TransactionDTO {
	if tx == nil {
		return nil
	}
	if isDetail {
		return &pb.TransactionDTO{
			Id:                 tx.Id,
			BlockNumber:        tx.BlockNumber,
			BlockTimestamp:     tx.BlockTimestamp,
			Hash:               tx.Hash,
			Code:               tx.Code,
			Data:               tx.Data,
			Info:               tx.Info,
			GasWanted:          tx.GasWanted,
			GasUsed:            tx.GasUsed,
			GasFee:             tx.GasFee,
			Codespace:          tx.Codespace,
			Events:             tx.Events,
			TxType:             tx.TxType,
			Messages:           tx.Messages,
			Signatures:         tx.Signatures,
			Memo:               tx.Memo,
			TxNumber:           tx.TxNumber,
			BlockUnixTimestamp: tx.BlockUnixTimestamp,
			ErrorLog:           tx.ErrorLog,
			Logs:               tx.Logs,
			ClaimIds:           tx.ClaimIds,
			Payer:              tx.Payer,
		}
	} else {
		return &pb.TransactionDTO{
			BlockNumber:        tx.BlockNumber,
			BlockUnixTimestamp: tx.BlockUnixTimestamp,
			Hash:               tx.Hash,
			ErrorLog:           tx.ErrorLog,
			Messages:           tx.Messages,
			Payer:              tx.Payer,
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

// ToValidatorDTO converts a Validator model to proto ValidatorDTO.
func ToValidatorDTO(validator *Validator) *pb.ValidatorDTO {
	if validator == nil {
		return nil
	}
	return &pb.ValidatorDTO{
		Id:                      validator.Id,
		Moniker:                 validator.Moniker,
		OperatorAddress:         validator.OperatorAddress,
		ConsensusAddress:        validator.ConsensusAddress,
		Jailed:                  validator.Jailed,
		Status:                  validator.Status,
		Tokens:                  validator.Tokens,
		DelegatorShares:         validator.DelegatorShares,
		Description:             validator.Description,
		UnbondingHeight:         validator.UnbondingHeight,
		UnbondingTime:           validator.UnbondingTime,
		CommissionRate:          validator.CommissionRate,
		CommissionMaxRate:       validator.CommissionMaxRate,
		CommissionMaxChangeRate: validator.CommissionMaxChangeRate,
		CommissionUpdateTime:    validator.CommissionUpdateTime,
		Proposed:                validator.Proposed,
		Signed:                  validator.Signed,
		Missed:                  validator.Missed,
		Timestamp:               validator.Timestamp,
		Uptimes:                 validator.Uptimes,
		SlashingEvents:          validator.SlashingEvents,
		UptimePercentage:        validator.UptimePercentage,
		ImageUrl:                validator.ImageUrl,
	}
}

// ToValidatorDTOs converts a slice of Validator models to proto ValidatorDTOs.
func ToValidatorDTOs(validators []Validator) []*pb.ValidatorDTO {
	if validators == nil {
		return nil
	}
	dtos := make([]*pb.ValidatorDTO, 0, len(validators))
	for _, validator := range validators {
		dtos = append(dtos, ToValidatorDTO(&validator))
	}
	return dtos
}
