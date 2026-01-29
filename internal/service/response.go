package service

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/utils"
)

// MarshalAndReply serializes the value and returns a success reply.
// The value is converted to google.protobuf.Struct, allowing direct JSON object response.
func MarshalAndReply(v interface{}, errorMsg string) (*pb.Reply, error) {
	var structData *structpb.Struct
	var err error

	// Check if v is a proto.Message, use protoMessageToStructWithOptions if so
	if msg, ok := v.(proto.Message); ok {
		structData, err = ProtoMessageToStructWithOptions(msg, nil)
	} else {
		// For non-proto types (maps, etc.), use toStruct
		structData, err = ToStruct(v)
	}

	if err != nil {
		global.Logger.Errorf("%s: %v", errorMsg, err)
		return ErrorReply(http.StatusInternalServerError, errorMsg, err), err
	}
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
		Data:    structData,
	}, nil
}

// ErrorReply returns an error reply with the given code, message, and error details.
func ErrorReply(code int, message string, err error) *pb.Reply {
	reply := &pb.Reply{
		Code: int32(code),
	}

	// Only append error details if err is not nil
	if err != nil {
		// Sanitize error message to prevent sensitive information leakage
		sanitizedErr := utils.SanitizeError(err, MaxErrorLength)
		reply.Message = fmt.Sprintf("%s: %s", message, sanitizedErr)
	} else {
		reply.Message = message
	}

	return reply
}

// ToStruct converts any value to google.protobuf.Struct.
// If the value is a slice/array, it will be automatically wrapped in a map with "data" key.
// This is because structpb.Struct can only represent JSON objects, not arrays.
func ToStruct(v interface{}) (*structpb.Struct, error) {
	// Check if v is a slice or array using reflection
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		// Wrap array/slice in a map with "data" key
		v = map[string]interface{}{
			"data": v,
		}
	}

	// Use sonic for JSON operations (high performance)
	// First marshal to JSON
	jsonBytes, err := sonic.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal to JSON")
	}

	// Unmarshal into structpb.Struct using standard protojson
	var structData structpb.Struct
	if err := protojson.Unmarshal(jsonBytes, &structData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON to struct")
	}

	return &structData, nil
}

// ProtoMessageToStructWithOptions converts a protobuf message to google.protobuf.Struct.
func ProtoMessageToStructWithOptions(msg proto.Message, options *protojson.MarshalOptions) (*structpb.Struct, error) {
	// Use default options or custom options
	if options == nil {
		options = &protojson.MarshalOptions{
			UseProtoNames:   true,  // Use proto field names instead of JSON names
			EmitUnpopulated: false, // Don't output unset fields
		}
	}

	jsonBytes, err := options.Marshal(msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal proto message")
	}

	var structData structpb.Struct
	if err := protojson.Unmarshal(jsonBytes, &structData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON to structpb.Struct")
	}

	return &structData, nil
}
