package models

import (
	"encoding/hex"

	github_com_cosmos_cosmos_sdk_types "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"gorm.io/gorm"
)

type Account struct {
	gorm.Model

	Address string `json:"address" gorm:"column:address;index"`
	TypeUrl string `json:"type_url" gorm:"column:type_url"`
	Value   string `json:"value" gorm:"column:value"`
}

func AccountFromProto(account *authtypes.QueryAccountResponse, address string) *Account {
	return &Account{
		Address: address,
		TypeUrl: account.Account.TypeUrl,
		Value:   hex.EncodeToString(account.Account.Value),
	}
}

type AccountBalance struct {
	gorm.Model

	Address string `json:"address" gorm:"column:address"`
	Denom   string `json:"denom" gorm:"column:denom"`
	Amount  string `json:"amount" gorm:"column:amount"`
}

func AccountBalanceFromProto(coin github_com_cosmos_cosmos_sdk_types.Coin, address string) *AccountBalance {
	return &AccountBalance{
		Address: address,
		Denom:   coin.Denom,
		Amount:  coin.Amount.String(),
	}
}
