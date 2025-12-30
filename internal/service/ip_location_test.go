package service

import (
	"context"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Private IP ranges
		{"Class A private", "10.0.0.1", true},
		{"Class A private", "10.255.255.255", true},
		{"Class B private", "172.16.0.1", true},
		{"Class B private", "172.31.255.255", true},
		{"Class C private", "192.168.0.1", true},
		{"Class C private", "192.168.255.255", true},
		{"Loopback", "127.0.0.1", true},
		{"Loopback", "127.255.255.255", true},
		{"Link-local", "169.254.0.1", true},
		{"Link-local", "169.254.255.255", true},

		// Public IP addresses
		{"Public IP", "8.8.8.8", false},
		{"Public IP", "172.15.0.1", false},
		{"Public IP", "172.32.0.1", false},
		{"Public IP", "192.167.255.255", false},
		{"Public IP", "192.169.0.0", false},
		{"Invalid IP", "invalid", false},
		{"Empty IP", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrivateIP(tt.ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIPLocationService_GetLocation_PrivateIP(t *testing.T) {
	// Create service without database (graceful degradation)
	service := &IPLocationService{}

	// Test various private IPs return "Private Network, Local"
	privateIPs := []string{
		"192.168.1.1",
		"10.0.0.1",
		"172.16.0.1",
		"127.0.0.1",
		"169.254.1.1",
	}

	for _, ip := range privateIPs {
		location := service.GetLocation(context.Background(), ip)
		if location != "Private Network, Local" {
			t.Errorf("GetLocation(%s) = %s, want 'Private Network, Local'", ip, location)
		}
	}

	// Test that public IPs return empty (since no database)
	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
	}

	for _, ip := range publicIPs {
		location := service.GetLocation(context.Background(), ip)
		if location != "" {
			t.Errorf("GetLocation(%s) = %s, want empty string (no database)", ip, location)
		}
	}
}
