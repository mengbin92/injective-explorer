package service

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestGenerateAPIToken(t *testing.T) {
	// Test token generation
	token, tokenHash, tokenPreview, err := generateAPIToken()

	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Verify token format
	if !strings.HasPrefix(token, "byh_") {
		t.Errorf("Token should start with 'byh_', got: %s", token)
	}

	// Verify token length (4 prefix + 64 hex chars = 68)
	expectedLength := 68
	if len(token) != expectedLength {
		t.Errorf("Token should have length %d, got: %d", expectedLength, len(token))
	}

	// Verify hex encoding after prefix
	tokenPart := strings.TrimPrefix(token, "byh_")
	if len(tokenPart) != 64 {
		t.Errorf("Token part should have 64 hex characters, got: %d", len(tokenPart))
	}

	// Verify all characters are valid hex
	if _, err := hex.DecodeString(tokenPart); err != nil {
		t.Errorf("Token part is not valid hex: %v", err)
	}

	// Verify token hash is valid hex
	if _, err := hex.DecodeString(tokenHash); err != nil {
		t.Errorf("Token hash is not valid hex: %v", err)
	}

	// Verify token preview format
	if !strings.HasPrefix(tokenPreview, "byh_") {
		t.Errorf("Token preview should start with 'byh_', got: %s", tokenPreview)
	}

	// Verify preview contains "..."
	if !strings.Contains(tokenPreview, "...") {
		t.Errorf("Token preview should contain '...', got: %s", tokenPreview)
	}

	// Test multiple generations to ensure uniqueness
	token2, _, _, err := generateAPIToken()
	if err != nil {
		t.Fatalf("Failed to generate second token: %v", err)
	}

	if token == token2 {
		t.Error("Generated tokens should be unique")
	}
}

func TestTokenFormatDetails(t *testing.T) {
	token, _, _, err := generateAPIToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Check that token contains only valid characters
	validChars := "0123456789abcdef"
	tokenPart := strings.TrimPrefix(token, "byh_")

	for _, char := range tokenPart {
		found := false
		for _, valid := range validChars {
			if char == valid {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Token contains invalid character: %c", char)
		}
	}
}