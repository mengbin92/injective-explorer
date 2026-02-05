package models

import (
	"sync"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/std"

	// Injective chain modules - provides comprehensive type registration
	auctiontypes "github.com/InjectiveLabs/sdk-go/chain/auction/types"
	exchangetypes "github.com/InjectiveLabs/sdk-go/chain/exchange/types"
	insurancetypes "github.com/InjectiveLabs/sdk-go/chain/insurance/types"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	peggytypes "github.com/InjectiveLabs/sdk-go/chain/peggy/types"
	permissionstypes "github.com/InjectiveLabs/sdk-go/chain/permissions/types"
	tokenfactorytypes "github.com/InjectiveLabs/sdk-go/chain/tokenfactory/types"
	wasmxtypes "github.com/InjectiveLabs/sdk-go/chain/wasmx/types"
)

var (
	// interfaceRegistry is a global registry for unpacking protobuf Any types
	interfaceRegistry codectypes.InterfaceRegistry
	registryOnce      sync.Once
)

// getInterfaceRegistry returns a configured interface registry with all message types registered.
// This uses the standard Cosmos SDK registration pattern via std.RegisterInterfaces,
// which automatically registers all standard Cosmos SDK modules (bank, staking, distribution, gov, etc..)
// plus Injective-specific modules.
func getInterfaceRegistry() codectypes.InterfaceRegistry {
	registryOnce.Do(func() {
		interfaceRegistry = codectypes.NewInterfaceRegistry()

		// Register crypto types (public keys, signatures)
		cryptocodec.RegisterInterfaces(interfaceRegistry)

		// Register all standard Cosmos SDK module types (bank, staking, distribution, gov, authz, etc.)
		// This is the recommended way from Cosmos SDK - covers all standard modules automatically
		std.RegisterInterfaces(interfaceRegistry)

		// Register Injective-specific module types
		exchangetypes.RegisterInterfaces(interfaceRegistry)
		auctiontypes.RegisterInterfaces(interfaceRegistry)
		insurancetypes.RegisterInterfaces(interfaceRegistry)
		oracletypes.RegisterInterfaces(interfaceRegistry)
		peggytypes.RegisterInterfaces(interfaceRegistry)
		wasmxtypes.RegisterInterfaces(interfaceRegistry)
		tokenfactorytypes.RegisterInterfaces(interfaceRegistry)
		permissionstypes.RegisterInterfaces(interfaceRegistry)
	})
	return interfaceRegistry
}

// GetInterfaceRegistry returns the global interface registry (exported for use in other packages).
func GetInterfaceRegistry() codectypes.InterfaceRegistry {
	return getInterfaceRegistry()
}

// getProtoCodec returns a codec configured with the interface registry
func getProtoCodec() codec.Codec {
	return codec.NewProtoCodec(getInterfaceRegistry())
}

// unpackConsensusPubKey unpacks a validator's consensus public key from the Any type
func unpackConsensusPubKey(pubkeyAny *codectypes.Any) (cryptotypes.PubKey, error) {
	if pubkeyAny == nil {
		return nil, nil
	}

	var pk cryptotypes.PubKey
	err := getInterfaceRegistry().UnpackAny(pubkeyAny, &pk)
	if err != nil {
		return nil, err
	}
	return pk, nil
}

