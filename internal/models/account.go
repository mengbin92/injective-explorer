// Package models provides data models for blockchain entities.
// It defines structures for accounts, blocks, transactions, and their conversions
// between protobuf and database representations.
package models

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/bytedance/sonic"
	"github.com/cosmos/cosmos-sdk/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	github_com_cosmos_cosmos_sdk_types "github.com/cosmos/cosmos-sdk/types"
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
			// 预先计算小写地址，避免重复调用 ToLower
			addrStr := pubKey.Address().String()
			lowerAddr := strings.ToLower(addrStr)
			pubAddress = fmt.Sprintf("0x%s", lowerAddr)
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
	// Convert address to Ethereum format for storage
	// Database stores addresses in Ethereum format (0x...) for consistency with browser display
	ethAddress := ConvertAddressToEthForStorage(address)
	return &Account{
		Address:     ethAddress,
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
	// Convert address to Ethereum format for storage
	// Database stores addresses in Ethereum format (0x...) for consistency with browser display
	ethAddress := ConvertAddressToEthForStorage(address)
	return &AccountBalance{
		Address: ethAddress,
		Denom:   coin.Denom,
		Amount:  coin.Amount.String(),
	}
}

// ConvertAddressToEthForStorage converts an address to Ethereum format (0x...) for database storage.
// This function ensures all addresses are stored in a consistent Ethereum format (0x...) in the database,
// regardless of the input format (Injective inj1... or Ethereum 0x...).
//
// Storage Policy:
//   - All addresses in the database are stored in Ethereum format (0x...) for consistency
//   - This simplifies queries and ensures uniform display in the browser
//   - Injective addresses (inj1...) are converted to Ethereum format (0x...)
//   - Ethereum addresses are validated and normalized to lowercase
//
// Parameters:
//   - address: The address to convert (can be inj1... or 0x... format)
//
// Returns:
//   - string: The address in Ethereum format (0x...), or original if conversion fails
//
// Note: If conversion fails, the original address is returned for backward compatibility
// with existing data that might be in unexpected formats.
func ConvertAddressToEthForStorage(address string) string {
	if address == "" {
		return address
	}

	// If already Ethereum format, validate and normalize (lowercase)
	if strings.HasPrefix(address, "0x") {
		// Normalize to lowercase for consistency
		return strings.ToLower(address)
	}

	// Try to convert from Injective format
	ethAddr, err := InjToEthAddress(address)
	if err != nil {
		// If conversion fails, return original address (for backward compatibility)
		// This handles edge cases where address might be in an unexpected format
		// In production, you might want to log this error for monitoring
		return address
	}

	return ethAddr
}

// 错误定义
var (
	ErrInvalidAddress = fmt.Errorf("invalid address format")
	ErrConversion     = fmt.Errorf("address conversion failed")
)

// IsValidInjectiveAddress 验证 Injective 地址格式
// 检查地址是否符合 inj1... 格式且能正确解码
func IsValidInjectiveAddress(addr string) bool {
	if addr == "" {
		return false
	}
	if len(addr) < 10 || !strings.HasPrefix(addr, "inj1") {
		return false
	}
	_, _, err := bech32.Decode(addr)
	return err == nil
}

// InjToEthAddress converts an Injective address (inj1...) to Ethereum address (0x...).
// This is the reverse operation of EthToInjAddress.
//
// Parameters:
//   - injAddr: The Injective address string (inj1...) or Ethereum address (0x...)
//
// Returns:
//   - string: The Ethereum address (0x...)
//   - error: Error if conversion fails
func InjToEthAddress(injAddr string) (string, error) {
	if injAddr == "" {
		return "", fmt.Errorf("%w: empty address", ErrInvalidAddress)
	}

	// If it's already an Ethereum address, validate and return as-is
	if strings.HasPrefix(injAddr, "0x") {
		// Basic validation: should be 42 characters (0x + 40 hex chars)
		if len(injAddr) != 42 {
			return "", fmt.Errorf("%w: invalid Ethereum address length", ErrInvalidAddress)
		}
		// Validate hex characters
		for i := 2; i < len(injAddr); i++ {
			c := injAddr[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return "", fmt.Errorf("%w: invalid hex character in Ethereum address", ErrInvalidAddress)
			}
		}
		return strings.ToLower(injAddr), nil
	}

	// Validate Injective address format
	if !IsValidInjectiveAddress(injAddr) {
		return "", fmt.Errorf("%w: invalid Injective address format: %s", ErrInvalidAddress, injAddr)
	}

	// 1. bech32 decode
	hrp, data, err := bech32.Decode(injAddr)
	if err != nil {
		return "", fmt.Errorf("%w: failed to decode bech32 address: %v", ErrConversion, err)
	}
	if hrp != "inj" {
		return "", fmt.Errorf("%w: invalid HRP, expected 'inj', got '%s'", ErrConversion, hrp)
	}

	// 2. 5-bit → 8-bit
	conv, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return "", fmt.Errorf("%w: failed to convert bits: %v", ErrConversion, err)
	}
	if len(conv) != 20 {
		return "", fmt.Errorf("%w: invalid address length: expected 20 bytes, got %d", ErrConversion, len(conv))
	}

	// 3. bytes → hex with 0x prefix (lowercase for consistency)
	return fmt.Sprintf("0x%s", strings.ToLower(hex.EncodeToString(conv))), nil
}
