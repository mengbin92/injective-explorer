// Package db provides custom serializers for GORM to optimize JSON operations.
package db

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/bytedance/sonic"
	"gorm.io/gorm/schema"
)

// SonicJSONSerializer is a custom GORM serializer that uses Sonic for high-performance JSON operations.
// It replaces the default JSON serializer to reduce CPU usage and improve serialization speed.
type SonicJSONSerializer struct{}

// Scan implements the deserialization process using Sonic.
// It unmarshals JSON data from the database into Go values.
// For []string fields, it handles type conversion from numbers to strings for backward compatibility.
func (SonicJSONSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) (err error) {
	fieldValue := reflect.New(field.FieldType)
	if dbValue != nil {
		var bytes []byte
		switch v := dbValue.(type) {
		case []byte:
			bytes = v
		case string:
			bytes = []byte(v)
		default:
			return fmt.Errorf("failed to unmarshal JSON value: %#v, value is %T", dbValue, dbValue)
		}
		if len(bytes) > 0 {
			// Special handling for []string fields: convert numbers to strings
			if field.FieldType.Kind() == reflect.Slice && field.FieldType.Elem().Kind() == reflect.String {
				// Helper function to convert a value to string
				valueToString := func(v interface{}) string {
					switch val := v.(type) {
					case string:
						return val
					case float64:
						// Convert number to string (handles integers and floats)
						// Use %.0f to avoid decimal points for whole numbers
						return fmt.Sprintf("%.0f", val)
					case json.Number:
						// json.Number is used when UseNumber is enabled
						return string(val)
					case int64:
						return fmt.Sprintf("%d", val)
					case int:
						return fmt.Sprintf("%d", val)
					case int32:
						return fmt.Sprintf("%d", val)
					case uint64:
						return fmt.Sprintf("%d", val)
					case uint:
						return fmt.Sprintf("%d", val)
					default:
						// Fallback: convert to string using fmt
						return fmt.Sprintf("%v", val)
					}
				}

				jsonStr := string(bytes)

				// First, try to unmarshal as array
				var rawArray []interface{}
				if err := json.Unmarshal(bytes, &rawArray); err == nil {
					// Successfully unmarshaled as array, convert each element to string
					stringArray := make([]string, 0, len(rawArray))
					for _, elem := range rawArray {
						stringArray = append(stringArray, valueToString(elem))
					}
					fieldValue.Elem().Set(reflect.ValueOf(stringArray))
				} else {
					// Failed to unmarshal as array, try as single value
					// This handles cases where database stores a single number like 100 instead of [100]
					var singleValue interface{}
					if err2 := json.Unmarshal(bytes, &singleValue); err2 == nil {
						// Successfully unmarshaled as single value, convert to array with one element
						stringArray := []string{valueToString(singleValue)}
						fieldValue.Elem().Set(reflect.ValueOf(stringArray))
					} else {
						// If both json.Unmarshal attempts failed, the data might be malformed
						// Try to handle edge cases: empty string, null, etc.
						trimmed := jsonStr
						if trimmed == "" || trimmed == "null" || trimmed == "NULL" {
							// Empty or null value, return empty array
							fieldValue.Elem().Set(reflect.ValueOf([]string{}))
						} else {
							// Last resort: try Sonic as fallback (for valid string arrays only)
							if err3 := sonic.Unmarshal(bytes, fieldValue.Interface()); err3 != nil {
								// All methods failed, return error with context
								return fmt.Errorf("failed to unmarshal JSON to []string (tried array, single value, and sonic): array error: %v, single value error: %v, sonic error: %v, data: %q", err, err2, err3, jsonStr)
							}
						}
					}
				}
			} else {
				// For other types, use Sonic for performance
				if err = sonic.Unmarshal(bytes, fieldValue.Interface()); err != nil {
					return err
				}
			}
		}
	}
	field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
	return nil
}

// Value implements the serialization process using Sonic.
// It marshals Go values into JSON bytes for database storage.
func (SonicJSONSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	// Handle nil values
	if fieldValue == nil {
		return nil, nil
	}

	// Use Sonic for high-performance JSON marshaling
	result, err := sonic.Marshal(fieldValue)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON value: %w", err)
	}

	return result, nil
}

// RegisterSonicJSONSerializer registers the Sonic JSON serializer with GORM.
// This should be called during database initialization to replace the default JSON serializer.
func RegisterSonicJSONSerializer() {
	// Register as "sonic_json" for explicit use
	schema.RegisterSerializer("sonic_json", SonicJSONSerializer{})
	// Also register as "json" to replace the default JSON serializer
	// This allows existing "serializer:json" tags to automatically use Sonic
	schema.RegisterSerializer("json", SonicJSONSerializer{})
}
