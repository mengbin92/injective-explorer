// Package models provides utility functions for protobuf serialization using JSON.
// This file contains shared functions to eliminate code duplication across model conversion functions.
package models

import (
	"fmt"

	"github.com/bytedance/sonic"
	"google.golang.org/protobuf/proto"
)

// MarshalProtoToJSON marshals a protobuf message to a JSON string.
// This function provides a consistent way to serialize protobuf messages for storage.
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

	data, err := sonic.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal protobuf message to JSON: %w", err)
	}

	return string(data), nil
}

// UnmarshalProtoFromJSON unmarshals a JSON string back to a protobuf message.
// This function provides a consistent way to deserialize protobuf messages from storage.
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

	if err := sonic.Unmarshal([]byte(jsonStr), msg); err != nil {
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
