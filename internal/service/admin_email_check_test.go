package service

import (
	"context"
	"testing"

	pb "injective-explorer/api/explorer/v1"
)

func TestCheckEmailBlacklist(t *testing.T) {
	// This is a unit test placeholder
	// In production, you would set up a test database with known blacklist entries

	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{
			name:    "valid email format",
			email:   "test@example.com",
			wantErr: false,
		},
		{
			name:    "yopmail domain",
			email:   "test@yopmail.net",
			wantErr: false,
		},
		{
			name:    "uppercase email",
			email:   "TEST@YOPMAIL.NET",
			wantErr: false,
		},
		{
			name:    "invalid email - no @",
			email:   "invalid-email",
			wantErr: false, // returns error in response, not as error
		},
		{
			name:    "empty email",
			email:   "",
			wantErr: false, // returns error in response, not as error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &pb.CheckEmailBlacklistRequest{
				Email: tt.email,
			}

			// Note: This test requires a real AdminService with database connection
			// In production, you would mock the database or use a test database
			t.Skip("Requires database connection - run integration test separately")

			_, err := adminService.CheckEmailBlacklist(context.Background(), req)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckEmailBlacklist() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
