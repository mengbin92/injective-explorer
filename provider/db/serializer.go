// Package db provides custom serializers for GORM to optimize JSON operations.
package db

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

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
		if len(bytes) == 0 {
			field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
			return nil
		}

		// Special handling for []string fields: convert numbers to strings
		if field.FieldType.Kind() == reflect.Slice && field.FieldType.Elem().Kind() == reflect.String {
			jsonStr := string(bytes)

			// Handle empty/null cases
			trimmed := strings.TrimSpace(jsonStr)
			if trimmed == "" || trimmed == "null" || trimmed == "NULL" {
				fieldValue.Elem().Set(reflect.ValueOf([]string{}))
				field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
				return nil
			}

			// Try to unmarshal as array first
			var rawArray []interface{}
			if err := json.Unmarshal(bytes, &rawArray); err == nil {
				stringArray := make([]string, len(rawArray))
				for i, elem := range rawArray {
					stringArray[i] = toString(elem)
				}
				fieldValue.Elem().Set(reflect.ValueOf(stringArray))
				field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
				return nil
			}

			// Try to unmarshal as single value (e.g., 100000000 or "value")
			var singleValue interface{}
			if err := json.Unmarshal(bytes, &singleValue); err == nil {
				fieldValue.Elem().Set(reflect.ValueOf([]string{toString(singleValue)}))
				field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
				return nil
			}

			// Last resort: use raw string as single element
			// This handles unquoted strings in database
			fieldValue.Elem().Set(reflect.ValueOf([]string{jsonStr}))
			field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
			return nil
		}

		// For other types, use Sonic for performance
		if err = sonic.Unmarshal(bytes, fieldValue.Interface()); err != nil {
			return err
		}
	}
	field.ReflectValueOf(ctx, dst).Set(fieldValue.Elem())
	return nil
}

// toString converts any value to string, handling numeric types properly
func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Use %.0f to avoid decimal points for whole numbers
		return fmt.Sprintf("%.0f", val)
	case json.Number:
		return string(val)
	case int64:
		return fmt.Sprintf("%d", val)
	case int:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// Value implements the serialization process using Sonic.
// It marshals Go values into JSON bytes for database storage.
func (SonicJSONSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	if fieldValue == nil {
		return nil, nil
	}
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
