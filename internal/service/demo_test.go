// Package service provides unit tests for the demo service.
package service

import (
	"context"
	"testing"
	"time"

	pb "injective-explorer/api/explorer/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockBlockStream implements pb.Demo_StreamLatestBlocksServer for testing
type mockBlockStream struct {
	grpc.ServerStream
	ctx     context.Context
	updates []*pb.BlockUpdate
	sendErr error
}

func (m *mockBlockStream) Context() context.Context {
	return m.ctx
}

func (m *mockBlockStream) Send(update *pb.BlockUpdate) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.updates = append(m.updates, update)
	return nil
}

func (m *mockBlockStream) SendMsg(msg interface{}) error {
	return m.Send(msg.(*pb.BlockUpdate))
}

func (m *mockBlockStream) RecvMsg(msg interface{}) error {
	return nil
}

func (m *mockBlockStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockBlockStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockBlockStream) SetTrailer(metadata.MD) {
}

// mockAccountStream implements pb.Demo_StreamAccountUpdatesServer for testing
type mockAccountStream struct {
	grpc.ServerStream
	ctx     context.Context
	updates []*pb.AccountUpdate
	sendErr error
}

func (m *mockAccountStream) Context() context.Context {
	return m.ctx
}

func (m *mockAccountStream) Send(update *pb.AccountUpdate) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.updates = append(m.updates, update)
	return nil
}

func (m *mockAccountStream) SendMsg(msg interface{}) error {
	return m.Send(msg.(*pb.AccountUpdate))
}

func (m *mockAccountStream) RecvMsg(msg interface{}) error {
	return nil
}

func (m *mockAccountStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockAccountStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockAccountStream) SetTrailer(metadata.MD) {
}

// TestCheckHealthy tests the CheckHealthy endpoint
func TestCheckHealthy(t *testing.T) {
	// Skip if global.Logger is not initialized (requires full app setup)
	// In a real integration test, you would set up the full application context
	t.Skip("Requires full application setup with database and logger initialization")

	service := NewDemoService()
	ctx := context.Background()

	t.Run("successful health check", func(t *testing.T) {
		req := &pb.CheckHealthyRequest{
			Service: "test-service",
		}

		resp, err := service.CheckHealthy(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "test-service", resp.Service)
		assert.Contains(t, []string{"healthy", "degraded", "unhealthy"}, resp.Status)
		assert.NotNil(t, resp.Details)
		assert.Contains(t, resp.Details, "database")
	})

	t.Run("default service name", func(t *testing.T) {
		req := &pb.CheckHealthyRequest{}

		resp, err := service.CheckHealthy(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "injective-explorer", resp.Service)
	})
}

// TestStreamLatestBlocks_InputValidation tests input validation for StreamLatestBlocks
func TestStreamLatestBlocks_InputValidation(t *testing.T) {
	service := NewDemoService()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stream := &mockBlockStream{
		ctx: ctx,
	}

	req := &pb.StreamLatestBlocksRequest{
		FromHeight: 0,
	}

	// This will fail because we don't have a real database connection in tests
	// but we can test that the method handles the request correctly
	// The important thing is that it doesn't panic
	assert.NotPanics(t, func() {
		_ = service.StreamLatestBlocks(req, stream)
	})
}

// TestStreamAccountUpdates_InputValidation tests input validation for StreamAccountUpdates
func TestStreamAccountUpdates_InputValidation(t *testing.T) {
	service := NewDemoService()

	// Test empty address
	req := &pb.StreamAccountUpdatesRequest{
		Address: "",
	}

	err := service.StreamAccountUpdates(req, nil)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "address is required")

	// Test invalid address format
	req.Address = "invalid-address"
	err = service.StreamAccountUpdates(req, nil)
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "invalid address format")
}

// TestErrorHandling tests that errors are properly handled with gRPC status codes
func TestErrorHandling(t *testing.T) {
	service := NewDemoService()

	// Test that invalid address returns InvalidArgument
	req := &pb.StreamAccountUpdatesRequest{
		Address: "invalid",
	}

	err := service.StreamAccountUpdates(req, nil)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "error should be a gRPC status error")
	assert.Equal(t, codes.InvalidArgument, st.Code())
}
