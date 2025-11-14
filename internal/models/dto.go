// Package models provides Data Transfer Objects (DTOs) for API responses.
// DTOs exclude internal database fields like gorm.Model to prevent exposing
// sensitive database information to clients.
package models

// AccountDTO represents account information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type AccountDTO struct {
	Address     string `json:"address,omitempty"`
	BaseAccount string `json:"base_account,omitempty"`
	CodeHash    string `json:"code_hash,omitempty"`
}

// AccountBalanceDTO represents account balance information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type AccountBalanceDTO struct {
	Address string `json:"address,omitempty"`
	Denom   string `json:"denom,omitempty"`
	Amount  string `json:"amount,omitempty"`
}

// BlockDTO represents block information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type BlockDTO struct {
	Height             uint64   `json:"height,omitempty"`
	Proposer           string   `json:"proposer,omitempty"`
	Moniker            string   `json:"moniker,omitempty"`
	BlockHash          string   `json:"block_hash,omitempty"`
	ParentHash         string   `json:"parent_hash,omitempty"`
	NumPrecommits      int64    `json:"num_precommits,omitempty"`
	NumTxs             int64    `json:"num_txs,omitempty"`
	TotalTxs           int64    `json:"total_txs,omitempty"`
	Txs                []string `json:"txs,omitempty"`
	Timestamp          string   `json:"timestamp,omitempty"`
	BlockUnixTimestamp uint64   `json:"block_unix_timestamp,omitempty"`
}

// TransactionDTO represents transaction information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type TransactionDTO struct {
	Id                 string   `json:"id,omitempty"`
	BlockNumber        uint64   `json:"block_number,omitempty"`
	BlockTimestamp     string   `json:"block_timestamp,omitempty"`
	Hash               string   `json:"hash,omitempty"`
	Code               uint32   `json:"code,omitempty"`
	Data               string   `json:"data,omitempty"`
	Info               string   `json:"info,omitempty"`
	GasWanted          int64    `json:"gas_wanted,omitempty"`
	GasUsed            int64    `json:"gas_used,omitempty"`
	GasFee             string   `json:"gas_fee,omitempty"`
	Codespace          string   `json:"codespace,omitempty"`
	Events             []string `json:"events,omitempty"`
	TxType             string   `json:"tx_type,omitempty"`
	Messages           string   `json:"messages,omitempty"`
	Signatures         []string `json:"signatures,omitempty"`
	Memo               string   `json:"memo,omitempty"`
	TxNumber           uint64   `json:"tx_number,omitempty"`
	BlockUnixTimestamp uint64   `json:"block_unix_timestamp,omitempty"`
	ErrorLog           string   `json:"error_log"`
	Logs               string   `json:"logs,omitempty"`
	ClaimIds           []int64  `json:"claim_ids,omitempty"`
	Payer              string   `json:"payer,omitempty"`
}

// ValidatorDTO represents validator information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type ValidatorDTO struct {
	Id                      string   `json:"id,omitempty"`
	Moniker                 string   `json:"moniker,omitempty"`
	OperatorAddress         string   `json:"operator_address,omitempty"`
	ConsensusAddress        string   `json:"consensus_address,omitempty"`
	Jailed                  bool     `json:"jailed,omitempty"`
	Status                  int32    `json:"status,omitempty"`
	Tokens                  string   `json:"tokens,omitempty"`
	DelegatorShares         string   `json:"delegator_shares,omitempty"`
	Description             string   `json:"description,omitempty"`
	UnbondingHeight         int64    `json:"unbonding_height,omitempty"`
	UnbondingTime           string   `json:"unbonding_time,omitempty"`
	CommissionRate          string   `json:"commission_rate,omitempty"`
	CommissionMaxRate       string   `json:"commission_max_rate,omitempty"`
	CommissionMaxChangeRate string   `json:"commission_max_change_rate,omitempty"`
	CommissionUpdateTime    string   `json:"commission_update_time,omitempty"`
	Proposed                uint64   `json:"proposed,omitempty"`
	Signed                  uint64   `json:"signed,omitempty"`
	Missed                  uint64   `json:"missed,omitempty"`
	Timestamp               string   `json:"timestamp,omitempty"`
	Uptimes                 []string `json:"uptimes,omitempty"`
	SlashingEvents          []string `json:"slashing_events,omitempty"`
	UptimePercentage        float64  `json:"uptime_percentage,omitempty"`
	ImageUrl                string   `json:"image_url,omitempty"`
}

// ToAccountDTO converts an Account model to AccountDTO.
func ToAccountDTO(account *Account) *AccountDTO {
	if account == nil {
		return nil
	}
	return &AccountDTO{
		Address:     account.Address,
		BaseAccount: account.BaseAccount,
		CodeHash:    account.CodeHash,
	}
}

// ToAccountBalanceDTO converts an AccountBalance model to AccountBalanceDTO.
func ToAccountBalanceDTO(balance *AccountBalance) *AccountBalanceDTO {
	if balance == nil {
		return nil
	}
	return &AccountBalanceDTO{
		Address: balance.Address,
		Denom:   balance.Denom,
		Amount:  balance.Amount,
	}
}

// ToAccountBalanceDTOs converts a slice of AccountBalance models to AccountBalanceDTOs.
func ToAccountBalanceDTOs(balances []AccountBalance) []AccountBalanceDTO {
	if balances == nil {
		return nil
	}
	dtos := make([]AccountBalanceDTO, 0, len(balances))
	for _, balance := range balances {
		dtos = append(dtos, *ToAccountBalanceDTO(&balance))
	}
	return dtos
}

// ToBlockDTO converts a Block model to BlockDTO.
func ToBlockDTO(block *Block, isDetail bool) *BlockDTO {
	if block == nil {
		return nil
	}
	if isDetail {
		return &BlockDTO{
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
		return &BlockDTO{
			Height:             block.Height,
			Proposer:           block.Proposer,
			BlockHash:          block.BlockHash,
			NumTxs:             block.NumTxs,
			BlockUnixTimestamp: block.BlockUnixTimestamp,
		}
	}

}

// ToBlockDTOs converts a slice of Block models to BlockDTOs.
func ToBlockDTOs(blocks []Block) []*BlockDTO {
	if blocks == nil {
		return nil
	}
	dtos := make([]*BlockDTO, 0, len(blocks))
	for _, block := range blocks {
		dtos = append(dtos, ToBlockDTO(&block, false))
	}
	return dtos
}

// ToTransactionDTO converts a Transaction model to TransactionDTO.
func ToTransactionDTO(tx *Transaction, isDetail bool) *TransactionDTO {
	if tx == nil {
		return nil
	}
	if isDetail {
		return &TransactionDTO{
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
		return &TransactionDTO{
			BlockNumber:        tx.BlockNumber,
			BlockUnixTimestamp: tx.BlockUnixTimestamp,
			Hash:               tx.Hash,
			ErrorLog:           tx.ErrorLog,
			Messages:           tx.Messages,
			Payer:              tx.Payer,
		}
	}
}

// ToTransactionDTOs converts a slice of Transaction models to TransactionDTOs.
func ToTransactionDTOs(txs []Transaction) []*TransactionDTO {
	if txs == nil {
		return nil
	}
	dtos := make([]*TransactionDTO, 0, len(txs))
	for _, tx := range txs {
		dtos = append(dtos, ToTransactionDTO(&tx, false))
	}
	return dtos
}

// ToValidatorDTO converts a Validator model to ValidatorDTO.
func ToValidatorDTO(validator *Validator) *ValidatorDTO {
	if validator == nil {
		return nil
	}
	return &ValidatorDTO{
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

// ToValidatorDTOs converts a slice of Validator models to ValidatorDTOs.
func ToValidatorDTOs(validators []Validator) []ValidatorDTO {
	if validators == nil {
		return nil
	}
	dtos := make([]ValidatorDTO, 0, len(validators))
	for _, validator := range validators {
		dtos = append(dtos, *ToValidatorDTO(&validator))
	}
	return dtos
}
