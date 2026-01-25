// Package models provides utility functions for protobuf serialization using JSON.
// This file contains shared functions to eliminate code duplication across model conversion functions.
package models

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// MarshalProtoToJSON marshals a protobuf message to a JSON string.
// This function provides a consistent way to serialize protobuf messages for storage.
// It uses protojson with EmitUnpopulated option to ensure all fields are included,
// even if they have zero/empty values.
//
// Parameters:
//   - msg: The protobuf message to marshal (must implement proto.Message)
//
// Returns:
//   - string: The JSON string representation of the protobuf message
//   - error: Error if marshaling fails
//
// Example:
//
//	jsonStr, err := MarshalProtoToJSON(protobufMessage)
//	if err != nil {
//	    // Handle error
//	}
func MarshalProtoToJSON(msg proto.Message) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("cannot marshal nil protobuf message")
	}

	// Use protojson with EmitUnpopulated to ensure all fields are included
	// This is important for Signature fields where sequence=0 or address="" should still be visible
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true, // Include all fields, even zero/empty values
	}

	data, err := marshaler.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal protobuf message to JSON: %w", err)
	}

	return string(data), nil
}

// UnmarshalProtoFromJSON unmarshals a JSON string back to a protobuf message.
// This function provides a consistent way to deserialize protobuf messages from storage.
// It uses protojson for consistency with MarshalProtoToJSON.
//
// Parameters:
//   - jsonStr: The JSON string representation of the protobuf message
//   - msg: The protobuf message to unmarshal into (must implement proto.Message)
//
// Returns:
//   - error: Error if decoding or unmarshaling fails
//
// Example:
//
//	var msg MyProtoMessage
//	err := UnmarshalProtoFromJSON(jsonStr, &msg)
//	if err != nil {
//	    // Handle error
//	}
func UnmarshalProtoFromJSON(jsonStr string, msg proto.Message) error {
	if jsonStr == "" {
		return fmt.Errorf("cannot unmarshal empty string")
	}

	if msg == nil {
		return fmt.Errorf("cannot unmarshal into nil protobuf message")
	}

	// Use protojson for consistency with MarshalProtoToJSON
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: false, // Keep unknown fields for safety
	}

	if err := unmarshaler.Unmarshal([]byte(jsonStr), msg); err != nil {
		return fmt.Errorf("failed to unmarshal JSON to protobuf message: %w", err)
	}

	return nil
}

// MarshalProtoSliceToJSON marshals a slice of protobuf messages to a slice of JSON strings.
// This function handles nil messages gracefully.
//
// Parameters:
//   - msgs: The slice of protobuf messages to marshal
//
// Returns:
//   - []string: The slice of JSON strings
//   - error: Error if any marshaling fails (returns first error encountered)
//
// Note: If marshaling fails for a message, it will be skipped and an error will be returned.
func MarshalProtoSliceToJSON(msgs []proto.Message) ([]string, error) {
	if msgs == nil {
		return nil, nil
	}

	result := make([]string, 0, len(msgs))
	var firstErr error

	for i, msg := range msgs {
		if msg == nil {
			result = append(result, "")
			continue
		}

		jsonStr, err := MarshalProtoToJSON(msg)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to marshal message at index %d: %w", i, err)
			}
			// Skip this message but continue processing others
			result = append(result, "")
			continue
		}

		result = append(result, jsonStr)
	}

	return result, firstErr
}

// UnmarshalProtoSliceFromJSON unmarshals a slice of JSON strings back to protobuf messages.
// This function handles empty strings and decoding errors gracefully.
//
// Parameters:
//   - jsonStrs: The slice of JSON strings
//   - factory: A function that creates a new protobuf message instance for each element
//
// Returns:
//   - []proto.Message: The slice of unmarshaled protobuf messages
//   - error: Error if any unmarshaling fails (returns first error encountered)
//
// Note: If decoding fails for a string, it creates an empty message to maintain compatibility.
func UnmarshalProtoSliceFromJSON(jsonStrs []string, factory func() proto.Message) ([]proto.Message, error) {
	if jsonStrs == nil {
		return nil, nil
	}

	result := make([]proto.Message, 0, len(jsonStrs))
	var firstErr error

	for i, jsonStr := range jsonStrs {
		if jsonStr == "" {
			// Create empty message for empty string
			if factory != nil {
				result = append(result, factory())
			}
			continue
		}

		msg := factory()
		if msg == nil {
			result = append(result, nil)
			continue
		}

		if err := UnmarshalProtoFromJSON(jsonStr, msg); err != nil {
			// Create empty message for backward compatibility
			result = append(result, factory())
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to unmarshal message at index %d: %w", i, err)
			}
			continue
		}

		result = append(result, msg)
	}

	return result, firstErr
}

// MarshalStringSliceToJSON marshals a slice of strings to a JSON string.
// This is useful for storing simple string slices (like transaction hashes) in the database.
//
// Parameters:
//   - strs: The slice of strings to marshal
//
// Returns:
//   - string: The JSON string representation (e.g., `["hash1","hash2"]`)
//   - error: Error if marshaling fails
func MarshalStringSliceToJSON(strs []string) (string, error) {
	if strs == nil {
		return "[]", nil
	}

	// Simple JSON marshaling for string slices
	result := "["
	for i, str := range strs {
		if i > 0 {
			result += ","
		}
		// Escape quotes in string
		escaped := strings.ReplaceAll(str, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		result += "\"" + escaped + "\""
	}
	result += "]"

	return result, nil
}

// UnmarshalStringSliceFromJSON unmarshals a JSON string back to a slice of strings.
//
// Parameters:
//   - jsonStr: The JSON string representation (e.g., `["hash1","hash2"]`)
//
// Returns:
//   - []string: The slice of strings
//   - error: Error if unmarshaling fails
func UnmarshalStringSliceFromJSON[T string](jsonStr string) ([]T, error) {
	if jsonStr == "" || jsonStr == "[]" {
		return nil, nil
	}

	// Simple JSON parsing for string slices
	// This is a simplified parser that works for our use case
	var result []T
	jsonStr = strings.TrimSpace(jsonStr)

	if !strings.HasPrefix(jsonStr, "[") || !strings.HasSuffix(jsonStr, "]") {
		return nil, fmt.Errorf("invalid JSON array format")
	}

	// Remove brackets
	inner := jsonStr[1 : len(jsonStr)-1]
	if inner == "" {
		return result, nil
	}

	// Parse comma-separated values
	var current strings.Builder
	inString := false
	escaped := false

	for _, r := range inner {
		switch r {
		case '\\':
			if escaped {
				current.WriteRune(r)
				escaped = false
			} else {
				escaped = true
			}
		case '"':
			if escaped {
				current.WriteRune(r)
				escaped = false
			} else {
				inString = !inString
			}
		case ',':
			if !inString {
				result = append(result, T(current.String()))
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			if inString || escaped {
				current.WriteRune(r)
			}
		}
	}

	// Add the last element
	if current.Len() > 0 || inString {
		result = append(result, T(current.String()))
	}

	return result, nil
}
