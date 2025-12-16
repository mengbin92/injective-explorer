// Package db provides custom serializers for GORM to optimize JSON operations.
package db

import (
	"context"
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
			if err = sonic.Unmarshal(bytes, fieldValue.Interface()); err != nil {
				return err
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
