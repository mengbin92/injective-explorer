package models

import (
	"sync"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

var (
	// interfaceRegistry is a global registry for unpacking protobuf Any types
	interfaceRegistry codectypes.InterfaceRegistry
	registryOnce      sync.Once
)

// getInterfaceRegistry returns a configured interface registry for unpacking validator public keys
func getInterfaceRegistry() codectypes.InterfaceRegistry {
	registryOnce.Do(func() {
		interfaceRegistry = codectypes.NewInterfaceRegistry()
		// Register crypto types (public keys) so we can unpack validator consensus public keys
		cryptocodec.RegisterInterfaces(interfaceRegistry)
	})
	return interfaceRegistry
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
