// Package service provides business logic services for the explorer API.
package service

import (
	"context"

	pb "injective-explorer/api/explorer/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
