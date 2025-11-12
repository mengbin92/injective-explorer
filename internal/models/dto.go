// Package models provides Data Transfer Objects (DTOs) for API responses.
// DTOs exclude internal database fields like gorm.Model to prevent exposing
// sensitive database information to clients.
package models

// AccountDTO represents account information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type AccountDTO struct {
	Address     string `json:"address"`
	BaseAccount string `json:"base_account"`
	CodeHash    string `json:"code_hash"`
}

// AccountBalanceDTO represents account balance information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type AccountBalanceDTO struct {
	Address string `json:"address"`
	Denom   string `json:"denom"`
	Amount  string `json:"amount"`
}

// BlockDTO represents block information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type BlockDTO struct {
	Height             uint64   `json:"height"`
	Proposer           string   `json:"proposer"`
	Moniker            string   `json:"moniker"`
	BlockHash          string   `json:"block_hash"`
	ParentHash         string   `json:"parent_hash"`
	NumPrecommits      int64    `json:"num_precommits"`
	NumTxs             int64    `json:"num_txs"`
	TotalTxs           int64    `json:"total_txs"`
	Txs                []string `json:"txs"`
	Timestamp          string   `json:"timestamp"`
	BlockUnixTimestamp uint64   `json:"block_unix_timestamp"`
}

// TransactionDTO represents transaction information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type TransactionDTO struct {
	Id                 string   `json:"id"`
	BlockNumber        uint64   `json:"block_number"`
	BlockTimestamp     string   `json:"block_timestamp"`
	Hash               string   `json:"hash"`
	Code               uint32   `json:"code"`
	Data               string   `json:"data"`
	Info               string   `json:"info"`
	GasWanted          int64    `json:"gas_wanted"`
	GasUsed            int64    `json:"gas_used"`
	GasFee             string   `json:"gas_fee"`
	Codespace          string   `json:"codespace"`
	Events             []string `json:"events"`
	TxType             string   `json:"tx_type"`
	Messages           string   `json:"messages"`
	Signatures         []string `json:"signatures"`
	Memo               string   `json:"memo"`
	TxNumber           uint64   `json:"tx_number"`
	BlockUnixTimestamp uint64   `json:"block_unix_timestamp"`
	ErrorLog           string   `json:"error_log"`
	Logs               string   `json:"logs"`
	ClaimIds           []int64  `json:"claim_ids"`
	Payer              string   `json:"payer"`
}

// ValidatorDTO represents validator information for API responses.
// It excludes gorm.Model fields (ID, CreatedAt, UpdatedAt, DeletedAt).
type ValidatorDTO struct {
	Id                      string   `json:"id"`
	Moniker                 string   `json:"moniker"`
	OperatorAddress         string   `json:"operator_address"`
	ConsensusAddress        string   `json:"consensus_address"`
	Jailed                  bool     `json:"jailed"`
	Status                  int32    `json:"status"`
	Tokens                  string   `json:"tokens"`
	DelegatorShares         string   `json:"delegator_shares"`
	Description             string   `json:"description"`
	UnbondingHeight         int64    `json:"unbonding_height"`
	UnbondingTime           string   `json:"unbonding_time"`
	CommissionRate          string   `json:"commission_rate"`
	CommissionMaxRate       string   `json:"commission_max_rate"`
	CommissionMaxChangeRate string   `json:"commission_max_change_rate"`
	CommissionUpdateTime    string   `json:"commission_update_time"`
	Proposed                uint64   `json:"proposed"`
	Signed                  uint64   `json:"signed"`
	Missed                  uint64   `json:"missed"`
	Timestamp               string   `json:"timestamp"`
	Uptimes                 []string `json:"uptimes"`
	SlashingEvents          []string `json:"slashing_events"`
	UptimePercentage        float64  `json:"uptime_percentage"`
	ImageUrl                string   `json:"image_url"`
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
func ToBlockDTO(block *Block) *BlockDTO {
	if block == nil {
		return nil
	}
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
}

// ToBlockDTOs converts a slice of Block models to BlockDTOs.
func ToBlockDTOs(blocks []Block) []BlockDTO {
	if blocks == nil {
		return nil
	}
	dtos := make([]BlockDTO, 0, len(blocks))
	for _, block := range blocks {
		dtos = append(dtos, *ToBlockDTO(&block))
	}
	return dtos
}

// ToTransactionDTO converts a Transaction model to TransactionDTO.
func ToTransactionDTO(tx *Transaction) *TransactionDTO {
	if tx == nil {
		return nil
	}
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
}

// ToTransactionDTOs converts a slice of Transaction models to TransactionDTOs.
func ToTransactionDTOs(txs []Transaction) []TransactionDTO {
	if txs == nil {
		return nil
	}
	dtos := make([]TransactionDTO, 0, len(txs))
	for _, tx := range txs {
		dtos = append(dtos, *ToTransactionDTO(&tx))
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
