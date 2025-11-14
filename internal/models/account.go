// Package models provides data models for blockchain entities.
// It defines structures for accounts, blocks, transactions, and their conversions
// between protobuf and database representations.
package models

import (
	"encoding/hex"
	"strings"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	"github.com/bytedance/sonic"
	"github.com/cosmos/cosmos-sdk/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	github_com_cosmos_cosmos_sdk_types "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// Account represents a blockchain account in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores account information from the Injective blockchain,
// including the account address, base account details, and code hash.
type Account struct {
	gorm.Model

	// Address is the blockchain address (indexed for faster lookups)
	Address string `json:"address" gorm:"column:address;index"`
	// BaseAccount contains serialized base account information (stored as JSON)
	BaseAccount string `json:"base_account" gorm:"column:base_account"`
	// CodeHash is the hash of the account's code (if it's a contract account)
	CodeHash string `json:"code_hash" gorm:"column:code_hash"`
}

// BaseAccount represents the base account information.
// It contains the core account details that are stored as JSON in the Account model.
type BaseAccount struct {
	// Address is the account address
	Address string `json:"address" gorm:"column:address"`
	// PubKey is the public key of the account
	PubKey string `json:"pub_key" gorm:"column:pub_key"`
	// AccountNumber is the account number assigned by the blockchain
	AccountNumber uint64 `json:"account_number" gorm:"column:account_number"`
	// Sequence is the account sequence number for transaction ordering
	Sequence uint64 `json:"sequence" gorm:"column:sequence"`
}

type PubKey struct {
	Type    string `json:"type" gorm:"column:type"`
	Value   string `json:"value" gorm:"column:value"`
	Address string `json:"evm_address" gorm:"column:evm_address"`
}

// AccountFromProto converts a protobuf EthAccount to an Account model.
//
// Parameters:
//   - account: The protobuf account object to convert
//   - address: The account address string
//
// Returns:
//   - *Account: A new Account instance with data from the protobuf message
func AccountFromProto(account *types.EthAccount, address string, codec codec.Codec) *Account {
	if account == nil {
		return nil
	}

	// Handle case where PubKey might be nil
	var pub *PubKey
	if account.PubKey != nil {
		var pubKey cryptotypes.PubKey
		err := codec.UnpackAny(account.PubKey, &pubKey)
		if err != nil {
			return nil
		}

		// Safely check if pubKey is nil and extract values
		// Use recover to handle panics when calling methods on nil interface values
		var pubType string
		var pubValue string
		var pubAddress string

		func() {
			defer func() {
				if r := recover(); r != nil {
					// pubKey is nil or invalid, use empty values
					pubType = ""
					pubValue = ""
					pubAddress = ""
				}
			}()
			pubType = pubKey.Type()
			pubValue = hex.EncodeToString(pubKey.Bytes())
			pubAddress = "0x" + strings.ToLower(pubKey.Address().String())
		}()

		pub = &PubKey{
			Type:    pubType,
			Value:   pubValue,
			Address: pubAddress,
		}
	} else {
		// Create a PubKey with empty values when PubKey is nil
		pub = &PubKey{
			Type:    "",
			Value:   "",
			Address: "",
		}
	}

	pubKeyJSON, _ := sonic.Marshal(pub)
	baseAccount := &BaseAccount{
		Address:       address,
		PubKey:        string(pubKeyJSON),
		AccountNumber: account.AccountNumber,
		Sequence:      account.Sequence,
	}
	baseAccountJSON, _ := sonic.Marshal(baseAccount)
	return &Account{
		Address:     address,
		BaseAccount: string(baseAccountJSON),
		CodeHash:    hex.EncodeToString(account.CodeHash),
	}
}

// AccountBalance represents an account balance for a specific denomination in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores balance information for accounts, including the address,
// denomination (token type), and amount.
type AccountBalance struct {
	gorm.Model

	// Address is the account address (indexed for faster lookups)
	Address string `json:"address" gorm:"column:address;index"`
	// Denom is the denomination (token type) of the balance (indexed for faster lookups)
	Denom string `json:"denom" gorm:"column:denom;index"`
	// Amount is the balance amount as a string (supports large numbers)
	Amount string `json:"amount" gorm:"column:amount"`
}

// AccountBalanceFromProto converts a Cosmos SDK Coin to an AccountBalance model.
//
// Parameters:
//   - coin: The Cosmos SDK coin object to convert
//   - address: The account address string
//
// Returns:
//   - *AccountBalance: A new AccountBalance instance with data from the coin
func AccountBalanceFromProto(coin github_com_cosmos_cosmos_sdk_types.Coin, address string) *AccountBalance {
	return &AccountBalance{
		Address: address,
		Denom:   coin.Denom,
		Amount:  coin.Amount.String(),
	}
}

// AccountToEthAccount converts an Account model back to an EthAccount.
// Note: This requires the chain client's codec to properly marshal the account.
// The function returns the JSON representation that can be used to reconstruct the account.
//
// Parameters:
//   - account: The Account model from database
//
// Returns:
//   - JSON string representation of the account (for compatibility with existing API)
func AccountToEthAccountJSON(account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}

	// Parse BaseAccount from JSON
	var baseAccount BaseAccount
	if err := sonic.Unmarshal([]byte(account.BaseAccount), &baseAccount); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal base account")
	}

	// Create a simplified account structure for JSON output
	accountJSON := map[string]interface{}{
		"address":      account.Address,
		"base_account": baseAccount,
		"code_hash":    account.CodeHash,
	}

	jsonBytes, err := sonic.Marshal(accountJSON)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal account to JSON")
	}

	return string(jsonBytes), nil
}
