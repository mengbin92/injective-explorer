// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"net"
	"strings"
)

// IPWhitelistChecker checks if an IP address is in the whitelist.
// It supports both individual IP addresses and CIDR ranges (IPv4 and IPv6).
type IPWhitelistChecker struct {
	ips   []string
	nets  []*net.IPNet
	exact map[string]bool
}

// NewIPWhitelistChecker creates a new IP whitelist checker from a list of IP addresses and CIDR ranges.
//
// Parameters:
//   - ips: List of IP addresses or CIDR ranges (e.g., "127.0.0.1", "192.168.1.0/24", "::1", "2001:db8::/32")
//
// Returns:
//   - *IPWhitelistChecker: A new IP whitelist checker instance
func NewIPWhitelistChecker(ips []string) *IPWhitelistChecker {
	checker := &IPWhitelistChecker{
		ips:   ips,
		nets:  make([]*net.IPNet, 0),
		exact: make(map[string]bool),
	}

	// Parse IPs and CIDR ranges
	for _, ipStr := range ips {
		ipStr = strings.TrimSpace(ipStr)
		if ipStr == "" {
			continue
		}

		// Check if it's a CIDR range
		if strings.Contains(ipStr, "/") {
			_, ipNet, err := net.ParseCIDR(ipStr)
			if err == nil {
				checker.nets = append(checker.nets, ipNet)
			}
		} else {
			// Check if it's a valid IP address
			ip := net.ParseIP(ipStr)
			if ip != nil {
				checker.exact[ip.String()] = true
			}
		}
	}

	return checker
}

// IsWhitelisted checks if the given IP address is in the whitelist.
//
// Parameters:
//   - ip: The IP address to check (can be IPv4 or IPv6)
//
// Returns:
//   - bool: true if the IP is whitelisted, false otherwise
func (c *IPWhitelistChecker) IsWhitelisted(ip string) bool {
	if len(c.ips) == 0 {
		return false
	}

	// Remove brackets if present (for IPv6 addresses)
	ip = removeBrackets(ip)

	// Check exact match first
	if c.exact[ip] {
		return true
	}

	// Parse IP address
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check CIDR ranges
	for _, ipNet := range c.nets {
		if ipNet.Contains(parsedIP) {
			return true
		}
	}

	return false
}
