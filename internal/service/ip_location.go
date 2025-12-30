// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"net"
	"sync"

	"injective-explorer/internal/global"

	"github.com/oschwald/geoip2-golang"
	"github.com/pkg/errors"
)

// IPLocationService provides IP geolocation lookup using GeoIP2 database.
// It uses MaxMind GeoLite2 free database for IP location queries.
type IPLocationService struct {
	db   *geoip2.Reader
	mu   sync.RWMutex
	path string

	// 预解析的私有IP网络
	privateIPNets []*net.IPNet
}

// NewIPLocationService creates a new IP location service instance.
// If dbPath is empty or the database cannot be opened, the service will still
// be created but will return empty strings for location queries (graceful degradation).
//
// Parameters:
//   - dbPath: Path to the GeoIP2 database file (GeoLite2-City.mmdb)
//
// Returns:
//   - *IPLocationService: A new service instance
//   - error: Error if database cannot be opened (non-critical, service still usable)
func NewIPLocationService(dbPath string) (*IPLocationService, error) {
	service := &IPLocationService{
		path: dbPath,
	}

	if dbPath == "" {
		global.Logger.Warn("GeoIP2 database path not configured, IP location will be disabled")
		return service, nil
	}

	db, err := geoip2.Open(dbPath)
	if err != nil {
		global.Logger.Warnf("failed to open GeoIP2 database at %s: %v, IP location will be disabled", dbPath, err)
		return service, err
	}

	service.db = db

	privateCIDRs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	privateIPNets := make([]*net.IPNet, 0, len(privateCIDRs))
	for _, cidr := range privateCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// panic if the private CIDR is invalid
			global.Logger.Errorf("invalid private CIDR: %s, error: %v", cidr, err)
			return nil, errors.New("invalid private CIDR: " + cidr + ", error: " + err.Error())
		}
		privateIPNets = append(privateIPNets, ipNet)
	}

	service.privateIPNets = privateIPNets
	return service, nil
}

// GetLocation gets IP location information asynchronously.
// Returns empty string if lookup fails (non-blocking, graceful degradation).
// For private/internal IPs, returns "Private Network, Local".
//
// Parameters:
//   - ctx: Context for the operation
//   - ip: The IP address to lookup
//
// Returns:
//   - string: Location string in format "City, Country" or "Private Network, Local" for private IPs
func (s *IPLocationService) GetLocation(ctx context.Context, ip string) string {
	// Check if IP is a private/internal IP address
	if isPrivateIP(ip) {
		return "Private Network, Local"
	}

	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()

	if db == nil {
		return "" // Database not available, return empty
	}

	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return "" // Invalid IP, return empty
	}

	record, err := db.City(ipAddr)
	if err != nil {
		return "" // Lookup failed, return empty (non-blocking)
	}

	// Format: "City, Country"
	city := record.City.Names["en"]
	country := record.Country.Names["en"]
	if city != "" && country != "" {
		return city + ", " + country
	}
	if country != "" {
		return country
	}
	return ""
}

// isPrivateIP checks if the given IP address is a private/internal IP.
// Private IP ranges:
//   - 10.0.0.0/8
//   - 172.16.0.0/12
//   - 192.168.0.0/16
//   - 127.0.0.0/8 (loopback)
//   - 169.254.0.0/16 (link-local)
//
// Parameters:
//   - ip: The IP address to check
//
// Returns:
//   - bool: true if the IP is private, false otherwise
func isPrivateIP(ip string) bool {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return false
	}

	// Check if IP is a loopback/link-local/private/internal IP address
	if ipAddr.IsLoopback() || ipAddr.IsLinkLocalUnicast() || ipAddr.IsPrivate() {
		return true
	}

	return false
}

// Reload reloads the GeoIP2 database from the configured path.
// This can be used to update the database without restarting the service.
//
// Parameters:
//   - ctx: Context for the operation
//
// Returns:
//   - error: Error if reload fails
func (s *IPLocationService) Reload(ctx context.Context) error {
	if s.path == "" {
		return nil
	}

	db, err := geoip2.Open(s.path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	oldDB := s.db
	s.db = db
	s.mu.Unlock()

	// Close old database
	if oldDB != nil {
		oldDB.Close()
	}

	return nil
}

// Close closes the GeoIP2 database connection.
func (s *IPLocationService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}
