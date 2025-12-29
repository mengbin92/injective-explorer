package middleware

import (
	"testing"
)

func TestIPWhitelistChecker(t *testing.T) {
	// Test with individual IPs and CIDR ranges
	ips := []string{
		"127.0.0.1",
		"192.168.1.0/24",
		"::1",
		"2001:db8::/32",
	}

	checker := NewIPWhitelistChecker(ips)

	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},
		{"192.168.1.1", true},
		{"192.168.1.100", true},
		{"192.168.1.255", true},
		{"192.168.2.1", false},
		{"::1", true},
		{"2001:db8::1", true},
		{"2001:db8:ffff::1", true},
		{"2001:db9::1", false},
		{"10.0.0.1", false},
		{"[::1]", true}, // Test with brackets
		{"[2001:db8::1]", true},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := checker.IsWhitelisted(tt.ip)
			if result != tt.expected {
				t.Errorf("IsWhitelisted(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIPWhitelistCheckerEmpty(t *testing.T) {
	checker := NewIPWhitelistChecker([]string{})
	if checker.IsWhitelisted("127.0.0.1") {
		t.Error("Empty whitelist should not match any IP")
	}
}

func TestIPWhitelistCheckerInvalidIP(t *testing.T) {
	checker := NewIPWhitelistChecker([]string{"127.0.0.1"})
	if checker.IsWhitelisted("invalid-ip") {
		t.Error("Invalid IP should not match")
	}
}

