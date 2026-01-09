// Package service provides unit tests for the address label service.
package service

import (
	"context"
	"testing"

	pb "injective-explorer/api/explorer/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateAddressLabel_InputValidation tests input validation for CreateAddressLabel.
// Note: Full integration tests with database are skipped as they require DB initialization.
func TestCreateAddressLabel_InputValidation(t *testing.T) {
	service := NewAddressLabelService()
	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		resp, err := service.CreateAddressLabel(ctx, nil)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "request cannot be nil")
	})

	t.Run("missing address", func(t *testing.T) {
		req := &pb.CreateAddressLabelRequest{
			Address:   "",
			ChainId:   "injective-1",
			LabelType: "exchange",
		}

		resp, err := service.CreateAddressLabel(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "address is required")
	})

	t.Run("missing chain_id", func(t *testing.T) {
		req := &pb.CreateAddressLabelRequest{
			Address:   "inj1test1234567890123456789012345678901234567890",
			ChainId:   "",
			LabelType: "exchange",
		}

		resp, err := service.CreateAddressLabel(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "chain_id is required")
	})

	t.Run("missing label_type", func(t *testing.T) {
		req := &pb.CreateAddressLabelRequest{
			Address:   "inj1test1234567890123456789012345678901234567890",
			ChainId:   "injective-1",
			LabelType: "",
		}

		resp, err := service.CreateAddressLabel(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "label_type is required")
	})
}

// TestUpdateAddressLabel_InputValidation tests input validation for UpdateAddressLabel.
// Note: Full integration tests with database are skipped as they require DB initialization.
func TestUpdateAddressLabel_InputValidation(t *testing.T) {
	service := NewAddressLabelService()
	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		resp, err := service.UpdateAddressLabel(ctx, nil)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "request cannot be nil")
	})

	t.Run("missing address", func(t *testing.T) {
		req := &pb.UpdateAddressLabelRequest{
			Address:    "",
			ChainId:    "injective-1",
			LabelType:  "exchange",
			LabelValue: "Binance",
		}

		resp, err := service.UpdateAddressLabel(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "address is required")
	})

	t.Run("missing chain_id", func(t *testing.T) {
		req := &pb.UpdateAddressLabelRequest{
			Address:    "inj1test1234567890123456789012345678901234567890",
			ChainId:    "",
			LabelType:  "exchange",
			LabelValue: "Binance",
		}

		resp, err := service.UpdateAddressLabel(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "chain_id is required")
	})

	t.Run("missing label_type", func(t *testing.T) {
		req := &pb.UpdateAddressLabelRequest{
			Address:    "inj1test1234567890123456789012345678901234567890",
			ChainId:    "injective-1",
			LabelType:  "",
			LabelValue: "Binance",
		}

		resp, err := service.UpdateAddressLabel(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "label_type is required")
	})

	t.Run("no fields to update", func(t *testing.T) {
		req := &pb.UpdateAddressLabelRequest{
			Address:   "inj1test1234567890123456789012345678901234567890",
			ChainId:   "injective-1",
			LabelType: "exchange",
		}

		resp, err := service.UpdateAddressLabel(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "at least one field must be provided")
	})
}

// TestDeleteAddressLabel_InputValidation tests input validation for DeleteAddressLabel.
// Note: Full integration tests with database are skipped as they require DB initialization.
func TestDeleteAddressLabel_InputValidation(t *testing.T) {
	service := NewAddressLabelService()
	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		_, err := service.DeleteAddressLabel(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request cannot be nil")
	})

	t.Run("missing address", func(t *testing.T) {
		req := &pb.DeleteAddressLabelRequest{
			Address:   "",
			ChainId:   "injective-1",
			LabelType: "exchange",
		}

		_, err := service.DeleteAddressLabel(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "address is required")
	})

	t.Run("missing chain_id", func(t *testing.T) {
		req := &pb.DeleteAddressLabelRequest{
			Address:   "inj1test1234567890123456789012345678901234567890",
			ChainId:   "",
			LabelType: "exchange",
		}

		_, err := service.DeleteAddressLabel(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chain_id is required")
	})

	t.Run("missing label_type", func(t *testing.T) {
		req := &pb.DeleteAddressLabelRequest{
			Address:   "inj1test1234567890123456789012345678901234567890",
			ChainId:   "injective-1",
			LabelType: "",
		}

		_, err := service.DeleteAddressLabel(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "label_type is required")
	})
}

// TestGetAddressLabels_InputValidation tests input validation for GetAddressLabels.
// Note: Full integration tests with database are skipped as they require DB initialization.
func TestGetAddressLabels_InputValidation(t *testing.T) {
	service := NewAddressLabelService()
	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		resp, err := service.GetAddressLabels(ctx, nil)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "request cannot be nil")
	})

	t.Run("missing address", func(t *testing.T) {
		req := &pb.GetAddressLabelsRequest{
			Address: "",
			ChainId: "injective-1",
		}

		resp, err := service.GetAddressLabels(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "address is required")
	})

	t.Run("missing chain_id", func(t *testing.T) {
		req := &pb.GetAddressLabelsRequest{
			Address: "inj1test1234567890123456789012345678901234567890",
			ChainId: "",
		}

		resp, err := service.GetAddressLabels(ctx, req)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "chain_id is required")
	})
}

// TestListAddressLabels_InputValidation tests input validation for ListAddressLabels.
// Note: Full integration tests with database are skipped as they require DB initialization.
func TestListAddressLabels_InputValidation(t *testing.T) {
	service := NewAddressLabelService()
	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		resp, err := service.ListAddressLabels(ctx, nil)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(400), resp.Code)
		assert.Contains(t, resp.Message, "request cannot be nil")
	})

	t.Run("valid request without filters", func(t *testing.T) {
		t.Skip("Skipping - requires database connection")
		req := &pb.ListAddressLabelsRequest{}

		_, _ = service.ListAddressLabels(ctx, req)
	})

	t.Run("valid request with pagination", func(t *testing.T) {
		t.Skip("Skipping - requires database connection")
		req := &pb.ListAddressLabelsRequest{
			ChainId:   "injective-1",
			LabelType: "exchange",
			Pagination: &pb.PaginationRequest{
				Page:     1,
				PageSize: 10,
			},
		}

		_, _ = service.ListAddressLabels(ctx, req)
	})

	t.Run("valid request with cursor pagination", func(t *testing.T) {
		t.Skip("Skipping - requires database connection")
		req := &pb.ListAddressLabelsRequest{
			ChainId: "injective-1",
			Pagination: &pb.PaginationRequest{
				Cursor:   "100",
				PageSize: 10,
			},
		}

		_, _ = service.ListAddressLabels(ctx, req)
	})
}

// TestAddressLabelService_StatusCodes tests that appropriate HTTP status codes are returned.
func TestAddressLabelService_StatusCodes(t *testing.T) {
	service := NewAddressLabelService()
	ctx := context.Background()

	t.Run("CreateAddressLabel returns 400 for missing address", func(t *testing.T) {
		req := &pb.CreateAddressLabelRequest{
			ChainId:   "injective-1",
			LabelType: "exchange",
		}

		resp, _ := service.CreateAddressLabel(ctx, req)
		assert.Equal(t, int32(400), resp.Code)
	})

	t.Run("UpdateAddressLabel returns 400 for missing fields", func(t *testing.T) {
		req := &pb.UpdateAddressLabelRequest{
			Address:   "inj1test1234567890123456789012345678901234567890",
			ChainId:   "injective-1",
			LabelType: "exchange",
		}

		resp, _ := service.UpdateAddressLabel(ctx, req)
		assert.Equal(t, int32(400), resp.Code)
	})

	t.Run("GetAddressLabels returns 400 for missing address", func(t *testing.T) {
		req := &pb.GetAddressLabelsRequest{
			ChainId: "injective-1",
		}

		resp, _ := service.GetAddressLabels(ctx, req)
		assert.Equal(t, int32(400), resp.Code)
	})

	t.Run("ListAddressLabels returns 400 for nil request", func(t *testing.T) {
		resp, _ := service.ListAddressLabels(ctx, nil)
		assert.Equal(t, int32(400), resp.Code)
	})
}

// TestAddressLabelService_ResponseStructure tests that responses have proper structure.
func TestAddressLabelService_ResponseStructure(t *testing.T) {
	service := NewAddressLabelService()
	ctx := context.Background()

	t.Run("CreateAddressLabel returns error response with proper fields", func(t *testing.T) {
		req := &pb.CreateAddressLabelRequest{
			Address: "",
		}

		resp, err := service.CreateAddressLabel(ctx, req)

		require.Error(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.Message)
		assert.Equal(t, int32(400), resp.Code)
	})

	t.Run("UpdateAddressLabel returns error response with proper fields", func(t *testing.T) {
		req := &pb.UpdateAddressLabelRequest{
			Address: "",
		}

		resp, err := service.UpdateAddressLabel(ctx, req)

		require.Error(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.Message)
		assert.Equal(t, int32(400), resp.Code)
	})

	t.Run("GetAddressLabels returns error response with proper fields", func(t *testing.T) {
		req := &pb.GetAddressLabelsRequest{
			Address: "",
		}

		resp, err := service.GetAddressLabels(ctx, req)

		require.Error(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.Message)
		assert.Equal(t, int32(400), resp.Code)
	})

	t.Run("ListAddressLabels returns error response with proper fields", func(t *testing.T) {
		resp, err := service.ListAddressLabels(ctx, nil)

		require.Error(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.Message)
		assert.Equal(t, int32(400), resp.Code)
	})
}

// TestAddressLabelService_ErrorMessages tests that error messages are descriptive.
func TestAddressLabelService_ErrorMessages(t *testing.T) {
	service := NewAddressLabelService()
	ctx := context.Background()

	tests := []struct {
		name        string
		method      string
		req         interface{}
		expectedMsg string
	}{
		{
			name:        "CreateAddressLabel - missing address",
			method:      "CreateAddressLabel",
			req: &pb.CreateAddressLabelRequest{
				ChainId:   "injective-1",
				LabelType: "exchange",
			},
			expectedMsg: "address is required",
		},
		{
			name:        "CreateAddressLabel - missing chain_id",
			method:      "CreateAddressLabel",
			req: &pb.CreateAddressLabelRequest{
				Address:   "inj1test1234567890123456789012345678901234567890",
				LabelType: "exchange",
			},
			expectedMsg: "chain_id is required",
		},
		{
			name:        "CreateAddressLabel - missing label_type",
			method:      "CreateAddressLabel",
			req: &pb.CreateAddressLabelRequest{
				Address: "inj1test1234567890123456789012345678901234567890",
				ChainId: "injective-1",
			},
			expectedMsg: "label_type is required",
		},
		{
			name:        "UpdateAddressLabel - no fields to update",
			method:      "UpdateAddressLabel",
			req: &pb.UpdateAddressLabelRequest{
				Address:   "inj1test1234567890123456789012345678901234567890",
				ChainId:   "injective-1",
				LabelType: "exchange",
			},
			expectedMsg: "at least one field must be provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *pb.Reply
			var err error

			switch tt.method {
			case "CreateAddressLabel":
				resp, err = service.CreateAddressLabel(ctx, tt.req.(*pb.CreateAddressLabelRequest))
			case "UpdateAddressLabel":
				resp, err = service.UpdateAddressLabel(ctx, tt.req.(*pb.UpdateAddressLabelRequest))
			}

			require.Error(t, err)
			require.NotNil(t, resp)
			assert.Contains(t, resp.Message, tt.expectedMsg)
		})
	}
}

// TestAddressLabelService_Integration tests that require full application setup.
// These are skipped by default and should be run in integration test environment.
func TestAddressLabelService_Integration(t *testing.T) {
	t.Skip("Requires full application setup with database and Redis")

	// This test would run when database is available
	// It would test the complete cycle:
	// - Create label
	// - Get label
	// - Update label
	// - List labels
	// - Delete label
	// - Verify cache behavior
}

// BenchmarkAddressLabelOperations provides benchmarks for address label operations.
// Note: These benchmarks only test input validation, not full database operations.
func BenchmarkAddressLabelOperations(b *testing.B) {
	service := NewAddressLabelService()
	ctx := context.Background()

	b.Run("CreateAddressLabel input validation", func(b *testing.B) {
		req := &pb.CreateAddressLabelRequest{
			Address: "",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = service.CreateAddressLabel(ctx, req)
		}
	})

	b.Run("UpdateAddressLabel input validation", func(b *testing.B) {
		req := &pb.UpdateAddressLabelRequest{
			Address: "",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = service.UpdateAddressLabel(ctx, req)
		}
	})

	b.Run("GetAddressLabels input validation", func(b *testing.B) {
		req := &pb.GetAddressLabelsRequest{
			Address: "",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = service.GetAddressLabels(ctx, req)
		}
	})

	b.Run("ListAddressLabels input validation", func(b *testing.B) {
		// Use nil request to test input validation without database access
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = service.ListAddressLabels(ctx, nil)
		}
	})
}
