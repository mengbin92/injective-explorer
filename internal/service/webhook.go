// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"fmt"
	"net/http"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/utils"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// WebhookService provides webhook management (for paid tokens only).
// This service is reserved for future implementation.
type WebhookService struct {
	pb.UnimplementedWebhookServiceServer
}

// NewWebhookService creates a new webhook service instance.
//
// Returns:
//   - *WebhookService: A new service instance
func NewWebhookService() *WebhookService {
	return &WebhookService{}
}

// CreateWebhook creates a webhook configuration.
// Reserved for future implementation.
func (s *WebhookService) CreateWebhook(ctx context.Context, req *pb.CreateWebhookRequest) (*pb.Reply, error) {
	return nil, status.Error(codes.Unimplemented, "Webhook functionality is reserved for future implementation")
}

// ListWebhooks lists webhook configurations.
// Reserved for future implementation.
func (s *WebhookService) ListWebhooks(ctx context.Context, req *pb.ListWebhooksRequest) (*pb.Reply, error) {
	return nil, status.Error(codes.Unimplemented, "Webhook functionality is reserved for future implementation")
}

// UpdateWebhook updates webhook configuration.
// Reserved for future implementation.
func (s *WebhookService) UpdateWebhook(ctx context.Context, req *pb.UpdateWebhookRequest) (*pb.Reply, error) {
	return nil, status.Error(codes.Unimplemented, "Webhook functionality is reserved for future implementation")
}

// DeleteWebhook deletes a webhook.
// Reserved for future implementation.
func (s *WebhookService) DeleteWebhook(ctx context.Context, req *pb.DeleteWebhookRequest) (*pb.Reply, error) {
	return nil, status.Error(codes.Unimplemented, "Webhook functionality is reserved for future implementation")
}

// errorReply creates a standardized error reply with sanitized error messages.
func (s *WebhookService) errorReply(code int, message string, err error) *pb.Reply {
	sanitizedErr := utils.SanitizeError(err, 200)
	return &pb.Reply{
		Code:    int32(code),
		Message: fmt.Sprintf("%s: %s", message, sanitizedErr),
	}
}

// marshalAndReply serializes the value and returns a success reply.
// The value is converted to google.protobuf.Struct, allowing direct JSON object response.
func (s *WebhookService) marshalAndReply(v interface{}, errorMsg string) (*pb.Reply, error) {
	var structData *structpb.Struct
	var err error

	// Check if v is a proto.Message, use protoMessageToStructWithOptions if so
	if msg, ok := v.(proto.Message); ok {
		structData, err = s.protoMessageToStructWithOptions(msg, nil)
	} else {
		// For non-proto types (maps, etc.), use toStruct
		structData, err = s.toStruct(v)
	}

	if err != nil {
		global.Logger.Errorf("%s: %v", errorMsg, err)
		return s.errorReply(http.StatusInternalServerError, errorMsg, err), err
	}
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
		Data:    structData,
	}, nil
}

// toStruct converts any value to google.protobuf.Struct.
func (s *WebhookService) toStruct(v interface{}) (*structpb.Struct, error) {
	jsonBytes, err := sonic.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal to JSON")
	}

	var structData structpb.Struct
	if err := protojson.Unmarshal(jsonBytes, &structData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON to struct")
	}

	return &structData, nil
}

// protoMessageToStructWithOptions converts a protobuf message to google.protobuf.Struct.
func (s *WebhookService) protoMessageToStructWithOptions(msg proto.Message, options *protojson.MarshalOptions) (*structpb.Struct, error) {
	if options == nil {
		options = &protojson.MarshalOptions{
			UseProtoNames:   true,
			EmitUnpopulated: false,
		}
	}

	jsonBytes, err := options.Marshal(msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal proto message")
	}

	var structData structpb.Struct
	if err := protojson.Unmarshal(jsonBytes, &structData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON to struct")
	}

	return &structData, nil
}

