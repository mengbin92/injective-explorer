// Package models provides message parsing utilities for transaction messages.
// This file contains functions to parse and format transaction messages according to the specified JSON format.
package models

import (
	"encoding/base64"
	"strings"

	"injective-explorer/internal/global"

	"github.com/bytedance/sonic"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/gogoproto/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ParseMessagesToFormattedJSON parses the Messages field and returns a formatted JSON string
// with decoded message contents according to the PDF specification.
//
// Parameters:
//   - messagesBytes: The raw Messages bytes (TxBody protobuf or TxRaw protobuf)
//
// Returns:
//   - string: Formatted JSON string with decoded messages, or original format if parsing fails
//
// The output format matches the PDF specification:
//
//	{
//	  "messages": [
//	    {
//	      "@type": "/cosmos.bank.v1beta1.MsgSend",
//	      "from_address": "0x...",
//	      "to_address": "0x...",
//	      "amount": [{"denom": "inj", "amount": "1000000"}]
//	    }
//	  ]
//	}
func ParseMessagesToFormattedJSON(messagesBytes []byte) string {
	if len(messagesBytes) == 0 {
		return ""
	}

	// Check if already JSON format - if so, try to enhance it
	if len(messagesBytes) > 0 && (messagesBytes[0] == '[' || messagesBytes[0] == '{') {
		// Already JSON, try to parse and enhance
		return enhanceExistingJSON(string(messagesBytes))
	}

	// Try parsing as TxRaw first (most common format from explorer API)
	var txRaw cosmostxpb.TxRaw
	if err := txRaw.Unmarshal(messagesBytes); err == nil && len(txRaw.BodyBytes) > 0 {
		// Successfully parsed as TxRaw, parse BodyBytes as TxBody
		var txBody cosmostxpb.TxBody
		if err := txBody.Unmarshal(txRaw.BodyBytes); err == nil {
			return formatTxBodyMessages(&txBody)
		}
	}

	// Try parsing as TxBody directly (fallback)
	var txBody cosmostxpb.TxBody
	if err := txBody.Unmarshal(messagesBytes); err == nil {
		return formatTxBodyMessages(&txBody)
	}

	// Both protobuf parsing failed, return base64 encoded (fallback)
	if global.Logger != nil {
		global.Logger.Warnf("WARN: failed to parse Messages for formatting, returning base64 encoded")
	}
	return base64.StdEncoding.EncodeToString(messagesBytes)
}

// formatTxBodyMessages formats TxBody messages into the specified JSON structure
func formatTxBodyMessages(txBody *cosmostxpb.TxBody) string {
	if txBody == nil || len(txBody.Messages) == 0 {
		return "{\"messages\":[]}"
	}

	messages := make([]map[string]interface{}, 0, len(txBody.Messages))
	for _, msgAny := range txBody.Messages {
		if msgAny == nil {
			continue
		}

		// Decode the message using dynamic protobuf decoding
		decodedMsg := decodeAnyMessage(msgAny)
		messages = append(messages, decodedMsg)
	}

	// Build the result JSON
	result := map[string]interface{}{
		"messages": messages,
	}

	// Add memo if present
	if txBody.Memo != "" {
		result["memo"] = txBody.Memo
	}

	// Marshal to JSON
	jsonBytes, err := sonic.Marshal(result)
	if err != nil {
		if global.Logger != nil {
			global.Logger.Warnf("WARN: failed to marshal formatted messages to JSON: %v", err)
		}
		return "{\"messages\":[]}"
	}

	return string(jsonBytes)
}

// decodeAnyMessage decodes a codectypes.Any message into a map with formatted fields
// Uses the registered interface registry to properly decode all message types
func decodeAnyMessage(msgAny *codectypes.Any) map[string]interface{} {
	if msgAny == nil {
		return map[string]interface{}{}
	}

	// Create the result map with @type field
	result := map[string]interface{}{
		"@type": msgAny.TypeUrl,
	}

	// Get the interface registry with all types registered
	registry := getInterfaceRegistry()

	// Try to unpack the message using the registry
	var msg sdk.Msg
	err := registry.UnpackAny(msgAny, &msg)
	if err != nil {
		// If unpacking fails, try using dynamic protobuf as fallback
		if global.Logger != nil {
			global.Logger.Debugf("Failed to unpack message %s with registry: %v, trying dynamic decoding", msgAny.TypeUrl, err)
		}
		return decodeAnyMessageDynamic(msgAny)
	}

	// Use the proto codec to marshal to JSON
	// This works for both gogoproto and standard protobuf messages
	codec := getProtoCodec()
	jsonBytes, err := codec.MarshalJSON(msg)
	if err != nil {
		if global.Logger != nil {
			global.Logger.Debugf("Failed to marshal message %s to JSON: %v, trying dynamic decoding", msgAny.TypeUrl, err)
		}
		return decodeAnyMessageDynamic(msgAny)
	}

	// Parse the JSON back into a map
	var msgMap map[string]interface{}
	if err := sonic.Unmarshal(jsonBytes, &msgMap); err != nil {
		if global.Logger != nil {
			global.Logger.Debugf("Failed to unmarshal JSON to map for %s: %v", msgAny.TypeUrl, err)
		}
		result["value"] = base64.StdEncoding.EncodeToString(msgAny.Value)
		return result
	}

	// Merge the fields into result (excluding @type from msgMap if present)
	for k, v := range msgMap {
		if k != "@type" {
			// Convert addresses to EVM format
			result[k] = convertAddressesInValue(v, k)
		}
	}

	return result
}

// decodeAnyMessageDynamic decodes a message using dynamic protobuf as fallback
// This is used when the message type is not registered in the interface registry
func decodeAnyMessageDynamic(msgAny *codectypes.Any) map[string]interface{} {
	result := map[string]interface{}{
		"@type": msgAny.TypeUrl,
	}

	// Extract the message type from TypeUrl (e.g., "/cosmos.bank.v1beta1.MsgSend" -> "cosmos.bank.v1beta1.MsgSend")
	messageType := strings.TrimPrefix(msgAny.TypeUrl, "/")

	// Try to find the message descriptor in the global registry
	msgDesc, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(messageType))
	if err != nil {
		// Message type not found in registry, return with base64-encoded value
		if global.Logger != nil {
			global.Logger.Debugf("Message type %s not found in global registry, returning base64 value", messageType)
		}
		result["value"] = base64.StdEncoding.EncodeToString(msgAny.Value)
		return result
	}

	// Create a dynamic message from the descriptor
	msg := dynamicpb.NewMessage(msgDesc.Descriptor())

	// Unmarshal the value bytes into the dynamic message
	if err := proto.Unmarshal(msgAny.Value, msg); err != nil {
		if global.Logger != nil {
			global.Logger.Debugf("Failed to unmarshal dynamic message %s: %v", messageType, err)
		}
		result["value"] = base64.StdEncoding.EncodeToString(msgAny.Value)
		return result
	}

	// Marshal the message to JSON using protojson
	marshaler := protojson.MarshalOptions{
		UseProtoNames:   true,  // Use proto field names (snake_case)
		EmitUnpopulated: false, // Don't include empty fields
		Indent:          "",    // No indentation for compact JSON
	}

	jsonBytes, err := marshaler.Marshal(msg)
	if err != nil {
		if global.Logger != nil {
			global.Logger.Debugf("Failed to marshal dynamic message %s to JSON: %v", messageType, err)
		}
		result["value"] = base64.StdEncoding.EncodeToString(msgAny.Value)
		return result
	}

	// Parse the JSON back into a map
	var msgMap map[string]interface{}
	if err := sonic.Unmarshal(jsonBytes, &msgMap); err != nil {
		if global.Logger != nil {
			global.Logger.Debugf("Failed to unmarshal JSON to map for %s: %v", messageType, err)
		}
		result["value"] = base64.StdEncoding.EncodeToString(msgAny.Value)
		return result
	}

	// Merge the fields into result (excluding @type from msgMap if present)
	for k, v := range msgMap {
		if k != "@type" {
			// Convert addresses to EVM format
			result[k] = convertAddressesInValue(v, k)
		}
	}

	return result
}

// convertAddressesInValue recursively converts all addresses in a value to EVM format
func convertAddressesInValue(value interface{}, fieldName string) interface{} {
	switch v := value.(type) {
	case string:
		// Check if this field should contain an address
		if isAddressFieldName(fieldName) || isAddressLikeValue(v) {
			return ConvertAddressToEthForStorage(v)
		}
		return v
	case map[string]interface{}:
		// Recursively convert addresses in nested maps
		result := make(map[string]interface{})
		for k, val := range v {
			result[k] = convertAddressesInValue(val, k)
		}
		return result
	case []interface{}:
		// Recursively convert addresses in arrays
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = convertAddressesInValue(val, fieldName)
		}
		return result
	default:
		return v
	}
}

// isAddressFieldName checks if a field name indicates it contains an address
func isAddressFieldName(fieldName string) bool {
	lowerField := strings.ToLower(fieldName)
	return strings.Contains(lowerField, "address") ||
		strings.HasSuffix(lowerField, "sender") ||
		strings.HasSuffix(lowerField, "receiver") ||
		strings.HasSuffix(lowerField, "delegator") ||
		strings.HasSuffix(lowerField, "validator") ||
		strings.HasSuffix(lowerField, "depositor") ||
		strings.HasSuffix(lowerField, "voter") ||
		strings.HasSuffix(lowerField, "proposer") ||
		strings.HasSuffix(lowerField, "granter") ||
		strings.HasSuffix(lowerField, "grantee") ||
		strings.HasSuffix(lowerField, "signer") ||
		strings.HasSuffix(lowerField, "creator") ||
		strings.HasSuffix(lowerField, "owner") ||
		strings.HasSuffix(lowerField, "operator")
}

// isAddressLikeValue checks if a string value looks like a bech32 address
func isAddressLikeValue(value string) bool {
	// Bech32 addresses typically start with a prefix and contain only lowercase alphanumeric characters
	// Common prefixes: inj, cosmos, osmo, etc.
	if len(value) < 20 || len(value) > 100 {
		return false
	}

	// Check if it starts with a common prefix followed by "1"
	parts := strings.Split(value, "1")
	if len(parts) < 2 {
		return false
	}

	prefix := parts[0]
	if len(prefix) < 2 || len(prefix) > 10 {
		return false
	}

	// Check if prefix is all lowercase letters
	for _, c := range prefix {
		if c < 'a' || c > 'z' {
			return false
		}
	}

	// Check if the rest contains only valid bech32 characters
	rest := strings.Join(parts[1:], "1")
	for _, c := range rest {
		valid := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !valid {
			return false
		}
	}

	return true
}

// enhanceExistingJSON tries to enhance existing JSON format by decoding base64 values
func enhanceExistingJSON(jsonStr string) string {
	var data map[string]interface{}
	if err := sonic.Unmarshal([]byte(jsonStr), &data); err != nil {
		return jsonStr
	}

	// Check if it has messages array
	messagesInterface, ok := data["messages"]
	if !ok {
		return jsonStr
	}

	messages, ok := messagesInterface.([]interface{})
	if !ok {
		return jsonStr
	}

	// Try to enhance each message
	enhanced := false
	for i, msgInterface := range messages {
		msgMap, ok := msgInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if it has a base64-encoded value field
		valueStr, ok := msgMap["value"].(string)
		if !ok || valueStr == "" {
			continue
		}

		// Try to decode the value
		typeURL, _ := msgMap["@type"].(string)
		if typeURL == "" {
			continue
		}

		valueBytes, err := base64.StdEncoding.DecodeString(valueStr)
		if err != nil {
			continue
		}

		// Try to decode using dynamic protobuf
		msgAny := &codectypes.Any{
			TypeUrl: typeURL,
			Value:   valueBytes,
		}

		decodedMsg := decodeAnyMessage(msgAny)
		// Check if decoding was successful (no "value" field means success)
		if _, hasValue := decodedMsg["value"]; !hasValue {
			messages[i] = decodedMsg
			enhanced = true
		}
	}

	if !enhanced {
		return jsonStr
	}

	// Re-marshal the enhanced data
	data["messages"] = messages
	jsonBytes, err := sonic.Marshal(data)
	if err != nil {
		return jsonStr
	}

	return string(jsonBytes)
}
