// Package models provides utility functions for protobuf serialization and deserialization.
// This file contains shared functions to eliminate code duplication across model conversion functions.
package models

import (
	"encoding/base64"
	"fmt"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// MarshalProtoToBase64 marshals a protobuf message to a base64-encoded string.
// This function provides a consistent way to serialize protobuf messages for storage.
//
// Parameters:
//   - msg: The protobuf message to marshal (must implement proto.Message)
//
// Returns:
//   - string: The base64-encoded string representation of the protobuf message
//   - error: Error if marshaling fails
//
// Example:
//
//	encoded, err := MarshalProtoToBase64(protobufMessage)
//	if err != nil {
//	    // Handle error
//	}
func MarshalProtoToBase64(msg proto.Message) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("cannot marshal nil protobuf message")
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal protobuf message: %w", err)
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// UnmarshalProtoFromBase64 unmarshals a base64-encoded string back to a protobuf message.
// This function provides a consistent way to deserialize protobuf messages from storage.
//
// Parameters:
//   - encoded: The base64-encoded string representation of the protobuf message
//   - msg: The protobuf message to unmarshal into (must implement proto.Message)
//
// Returns:
//   - error: Error if decoding or unmarshaling fails
//
// Example:
//
//	var msg MyProtoMessage
//	err := UnmarshalProtoFromBase64(encoded, &msg)
//	if err != nil {
//	    // Handle error
//	}
func UnmarshalProtoFromBase64(encoded string, msg proto.Message) error {
	if encoded == "" {
		return fmt.Errorf("cannot unmarshal empty string")
	}

	if msg == nil {
		return fmt.Errorf("cannot unmarshal into nil protobuf message")
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("failed to decode base64 string: %w", err)
	}

	if err := proto.Unmarshal(data, msg); err != nil {
		return fmt.Errorf("failed to unmarshal protobuf message: %w", err)
	}

	return nil
}

// MarshalProtoSliceToBase64 marshals a slice of protobuf messages to a slice of base64-encoded strings.
// This function handles nil messages gracefully by using a fallback string representation.
//
// Parameters:
//   - msgs: The slice of protobuf messages to marshal
//
// Returns:
//   - []string: The slice of base64-encoded strings
//   - error: Error if any marshaling fails (returns first error encountered)
//
// Note: If marshaling fails for a message, it falls back to String() representation.
// This ensures backward compatibility with old data formats.
func MarshalProtoSliceToBase64(msgs []proto.Message) ([]string, error) {
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

		encoded, err := MarshalProtoToBase64(msg)
		if err != nil {
			// Fallback to text representation for backward compatibility
			textRepr := prototext.Format(msg)
			result = append(result, textRepr)
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to marshal message at index %d: %w", i, err)
			}
			continue
		}

		result = append(result, encoded)
	}

	return result, firstErr
}

// UnmarshalProtoSliceFromBase64 unmarshals a slice of base64-encoded strings back to protobuf messages.
// This function handles empty strings and decoding errors gracefully.
//
// Parameters:
//   - encoded: The slice of base64-encoded strings
//   - factory: A function that creates a new protobuf message instance for each element
//
// Returns:
//   - []proto.Message: The slice of unmarshaled protobuf messages
//   - error: Error if any unmarshaling fails (returns first error encountered)
//
// Note: If decoding fails for a string, it creates an empty message to maintain compatibility.
func UnmarshalProtoSliceFromBase64(encoded []string, factory func() proto.Message) ([]proto.Message, error) {
	if encoded == nil {
		return nil, nil
	}

	result := make([]proto.Message, 0, len(encoded))
	var firstErr error

	for i, enc := range encoded {
		if enc == "" {
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

		if err := UnmarshalProtoFromBase64(enc, msg); err != nil {
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
