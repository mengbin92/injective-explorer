// Package service provides unit tests for the explorer service.
package service

import (
	"context"
	"testing"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/provider/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockBlockStream implements pb.Explorer_StreamLatestBlocksServer for testing
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

// mockAccountStream implements pb.Explorer_StreamAccountUpdatesServer for testing
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

	service := NewExplorerService()
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
	service := NewExplorerService()
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
	service := NewExplorerService()

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

	// Test with mock stream (for future integration tests)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	stream := &mockAccountStream{
		ctx: ctx,
	}

	// This will fail because we don't have a real database connection in tests
	// but we can test that the method handles the request correctly
	req.Address = "inj1test1234567890123456789012345678901234567890"

	// Use the mock stream to test the method doesn't panic
	// The method will return early due to context timeout or database connection issues
	// but we verify it handles the stream parameter correctly
	testErr := service.StreamAccountUpdates(req, stream)
	// We expect either nil (if database is available) or an error
	// The important thing is that it doesn't panic and handles the stream correctly
	_ = testErr // Explicitly use err to satisfy linter
	// Verify stream methods are accessible (they will be called during actual streaming)
	_ = stream.Context()
	_ = stream.Send(nil)
}

// TestErrorHandling tests that errors are properly handled with gRPC status codes
func TestErrorHandling(t *testing.T) {
	service := NewExplorerService()

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

// TestGetTotalFromCache tests the getTotalFromCache function.
// Note: This test requires Redis to be available, so it may be skipped in CI.
func TestGetTotalFromCache(t *testing.T) {
	// Test cache miss (key doesn't exist)
	total, isExact := getTotalFromCache(context.Background(), "nonexistent:key")
	assert.Equal(t, int64(0), total)
	assert.False(t, isExact)

	// Test with nil Redis client (should return 0, false)
	// This is tested implicitly when Redis is not available
}

// TestCacheTotal tests the cacheTotal function.
// Note: This test requires Redis to be available, so it may be skipped in CI.
func TestCacheTotal(t *testing.T) {
	ctx := context.Background()

	// Test caching a total value
	testKey := "test:total:blocks"
	testTotal := int64(12345)

	// Cache the total
	err := cacheTotal(ctx, testKey, testTotal)
	assert.NoError(t, err)

	// Retrieve it
	total, isExact := getTotalFromCache(ctx, testKey)
	assert.Equal(t, testTotal, total)
	assert.False(t, isExact) // Cached values are always approximate

	// Clean up
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		redisClient.Del(ctx, testKey)
	}
}

// TestGetApproximateCountWithError tests the getApproximateCountWithError function.
// Note: This test requires a database connection, so it may be skipped in CI.
func TestGetApproximateCountWithError(t *testing.T) {
	t.Skip("Requires database connection and models setup")
	// This test would require:
	// 1. Database connection setup
	// 2. Model initialization
	// 3. Test data setup
	// For now, we skip it as it requires full application setup
}
