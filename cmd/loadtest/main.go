// Package main provides an HTTP interface concurrent stress testing tool.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
)

// Constant definitions
const (
	// DefaultMinLatency is the default minimum latency value (microseconds) for initialization
	DefaultMinLatency = int64(1e9)
	// DefaultMaxIdleConns is the default maximum number of idle connections (optimized to 200)
	DefaultMaxIdleConns = 200
	// DefaultMaxIdleConnsPerHost is the maximum number of idle connections per host (optimized to 100)
	DefaultMaxIdleConnsPerHost = 100
	// DefaultIdleConnTimeout is the idle connection timeout
	DefaultIdleConnTimeout = 90 * time.Second
	// DefaultTLSHandshakeTimeout is the TLS handshake timeout
	DefaultTLSHandshakeTimeout = 10 * time.Second
)

var (
	// Test configuration
	baseURL      string
	concurrency  int
	duration     time.Duration
	requests     int
	timeout      time.Duration
	verbose      bool
	testEndpoint string
	skipDataLoad bool // Whether to skip data preloading

	// Dynamic test data
	latestBlockHeight int64      // Latest block height
	testAddresses     []string   // Real test addresses
	validBlockRange   BlockRange // Valid block range

	// Default test addresses (for fallback use)
	defaultAddresses = []string{
		"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7",
		"0x9a0f5da660a8aabe5df86fbaa399f733bd461b1b",
		"0xc989d91cc9fdc0c1c2868bfe6dc2b9413b56515e",
	}
)

// BlockRange represents a block range
type BlockRange struct {
	Min int64
	Max int64
}

// Stats encapsulates all statistical information
type Stats struct {
	mu           sync.RWMutex
	total        int64
	success      int64
	failed       int64
	totalLatency int64 // microseconds
	minLatency   int64
	maxLatency   int64
	statusCodes  map[int]int64
}

// NewStats creates a new statistics object
func NewStats() *Stats {
	return &Stats{
		minLatency:  DefaultMinLatency,
		statusCodes: make(map[int]int64),
	}
}

// RecordRequest records a request
func (s *Stats) RecordRequest() {
	atomic.AddInt64(&s.total, 1)
}

// RecordSuccess records a successful request
func (s *Stats) RecordSuccess() {
	atomic.AddInt64(&s.success, 1)
}

// RecordFailure records a failed request
func (s *Stats) RecordFailure() {
	atomic.AddInt64(&s.failed, 1)
}

// RecordLatency records latency
func (s *Stats) RecordLatency(latency int64) {
	atomic.AddInt64(&s.totalLatency, latency)

	// Update minimum latency
	for {
		old := atomic.LoadInt64(&s.minLatency)
		if latency >= old {
			break
		}
		if atomic.CompareAndSwapInt64(&s.minLatency, old, latency) {
			break
		}
	}

	// Update maximum latency
	for {
		old := atomic.LoadInt64(&s.maxLatency)
		if latency <= old {
			break
		}
		if atomic.CompareAndSwapInt64(&s.maxLatency, old, latency) {
			break
		}
	}
}

// RecordStatus records a status code
func (s *Stats) RecordStatus(code int) {
	s.mu.Lock()
	s.statusCodes[code]++
	s.mu.Unlock()
}

// GetStats gets a snapshot of the statistics
func (s *Stats) GetStats() (total, success, failed, totalLat, minLat, maxLat int64, statusCodes map[int]int64) {
	total = atomic.LoadInt64(&s.total)
	success = atomic.LoadInt64(&s.success)
	failed = atomic.LoadInt64(&s.failed)
	totalLat = atomic.LoadInt64(&s.totalLatency)
	minLat = atomic.LoadInt64(&s.minLatency)
	maxLat = atomic.LoadInt64(&s.maxLatency)

	s.mu.RLock()
	statusCodes = make(map[int]int64, len(s.statusCodes))
	for k, v := range s.statusCodes {
		statusCodes[k] = v
	}
	s.mu.RUnlock()

	return
}

// Global statistics object
var stats = NewStats()

func init() {
	flag.StringVar(&baseURL, "url", "http://localhost:8000", "Server base URL")
	flag.IntVar(&concurrency, "c", 10, "Number of concurrent workers")
	flag.DurationVar(&duration, "d", 0, "Test duration (e.g., 30s, 5m)")
	flag.IntVar(&requests, "n", 0, "Total number of requests (0 means use duration)")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "Request timeout")
	flag.BoolVar(&verbose, "v", false, "Verbose output")
	flag.StringVar(&testEndpoint, "endpoint", "", "Specify the endpoint path to test (empty means test all endpoints)")
	flag.BoolVar(&skipDataLoad, "skip-data-load", false, "Skip data preloading and use default test data")
}

// EndpointConfig represents endpoint configuration
type EndpointConfig struct {
	Path         string
	Method       string
	Params       map[string]string
	Name         string
	ParamBuilder func(rng *rand.Rand, addresses []string) map[string]string // Dynamic parameter builder function (using ParamBuilder pattern to avoid static dependency on global state)
}

// All testable endpoints
var endpoints = []EndpointConfig{
	{
		Name:   "Health Check",
		Path:   "/api/v1/health",
		Method: "GET",
		Params: map[string]string{},
	},
	{
		Name:   "Latest Block Height",
		Path:   "/api/v1/block/latest-height",
		Method: "GET",
		Params: map[string]string{},
	},
	{
		Name:         "Latest Block List",
		Path:         "/api/v1/block/latest",
		Method:       "GET",
		Params:       map[string]string{},
		ParamBuilder: buildPaginationParams,
	},
	{
		Name:         "Get Block by Height",
		Path:         "/api/v1/block/by-height",
		Method:       "GET",
		Params:       map[string]string{},
		ParamBuilder: buildBlockHeightParams,
	},
	{
		Name:         "Latest Transaction List",
		Path:         "/api/v1/transaction/latest",
		Method:       "GET",
		Params:       map[string]string{},
		ParamBuilder: buildPaginationParams,
	},
	{
		Name:   "Transaction Statistics",
		Path:   "/api/v1/transaction/stats",
		Method: "GET",
		Params: map[string]string{},
	},
	{
		Name:         "24h Failed Transactions",
		Path:         "/api/v1/transaction/failed-24h",
		Method:       "GET",
		Params:       map[string]string{},
		ParamBuilder: buildPaginationParams,
	},
	{
		Name:         "Account Information",
		Path:         "/api/v1/account/info",
		Method:       "GET",
		Params:       map[string]string{},
		ParamBuilder: buildAddressParams,
	},
	{
		Name:         "Account Balances",
		Path:         "/api/v1/account/balances",
		Method:       "GET",
		Params:       map[string]string{},
		ParamBuilder: buildAddressParams,
	},
	{
		Name:         "Account Transactions",
		Path:         "/api/v1/account/transactions",
		Method:       "GET",
		Params:       map[string]string{},
		ParamBuilder: buildAddressWithPaginationParams,
	},
}

// buildPaginationParams builds random pagination parameters
func buildPaginationParams(rng *rand.Rand, _ []string) map[string]string {
	pageSizes := []int{10, 20, 50, 100}
	page := rng.Intn(10) + 1          // 1-10 pages
	pageSize := pageSizes[rng.Intn(len(pageSizes))]
	return map[string]string{
		"pagination.page":      fmt.Sprintf("%d", page),
		"pagination.page_size": fmt.Sprintf("%d", pageSize),
	}
}

// buildBlockHeightParams builds random block height parameters
func buildBlockHeightParams(rng *rand.Rand, _ []string) map[string]string {
	height := int64(1)
	// Use latestBlockHeight directly to avoid issues with validBlockRange global variable
	if latestBlockHeight > 0 {
		// Randomly select block height from 1 to latestBlockHeight
		// Add safety check to ensure height is reasonable
		maxHeight := latestBlockHeight
		if maxHeight > 100000000 { // Sanity check: shouldn't exceed 100M
			maxHeight = 100000000
		}
		height = 1 + rng.Int63n(maxHeight)
	}
	return map[string]string{
		"height": fmt.Sprintf("%d", height),
	}
}

// buildAddressParams builds random address parameters
// Uses ParamBuilder pattern to dynamically generate test parameters, avoiding static dependency on global state
func buildAddressParams(rng *rand.Rand, addresses []string) map[string]string {
	if len(addresses) == 0 {
		addresses = defaultAddresses
	}
	addr := addresses[rng.Intn(len(addresses))]
	return map[string]string{
		"address": addr,
	}
}

// buildAddressWithPaginationParams builds random address and pagination parameters
// Uses ParamBuilder pattern to dynamically generate test parameters, avoiding static dependency on global state
func buildAddressWithPaginationParams(rng *rand.Rand, addresses []string) map[string]string {
	params := buildAddressParams(rng, addresses)
	paginationParams := buildPaginationParams(rng, addresses)
	for k, v := range paginationParams {
		params[k] = v
	}
	return params
}

// loadTestDataFromAPI loads real test data from API
func loadTestDataFromAPI(client *http.Client) error {
	color.Cyan("Loading test data from API...\n")

	// 1. Get latest block height
	if err := loadLatestBlockHeight(client); err != nil {
		color.Yellow("Warning: Failed to get latest block height: %v, will use default value\n", err)
		// Use default value
		validBlockRange = BlockRange{Min: 1, Max: 148025427}
	} else {
		color.Green("✓ Latest block height: %d\n", latestBlockHeight)
	}

	// 2. Extract real addresses from latest transactions
	if err := loadRealAddresses(client); err != nil {
		color.Yellow("Warning: Failed to get real addresses: %v, will use default addresses\n", err)
		testAddresses = defaultAddresses
	} else {
		color.Green("✓ Got %d real addresses\n", len(testAddresses))
	}

	fmt.Println()
	return nil
}

// loadLatestBlockHeight gets the latest block height
func loadLatestBlockHeight(client *http.Client) error {
	u := baseURL + "/api/v1/block/latest-height"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse response (format is {"code":200, "message":"success", "data":{"height":"123456"}})
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	// Extract height from nested data field
	if data, ok := result["data"].(map[string]interface{}); ok {
		// Height is returned as a string (e.g., "39242")
		if hStr, ok := data["height"].(string); ok {
			var height int64
			if _, err := fmt.Sscanf(hStr, "%d", &height); err == nil {
				latestBlockHeight = height
				validBlockRange = BlockRange{Min: 1, Max: latestBlockHeight}
			}
		}
	}

	return nil
}

// loadRealAddresses extracts real addresses from latest transactions
func loadRealAddresses(client *http.Client) error {
	u := baseURL + "/api/v1/transaction/latest?pagination.page=1&pagination.page_size=50"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse response and extract addresses
	// Format is {"code":200, "message":"success", "data":{"data":[...], "pagination":{...}}}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	// Extract all unique addresses
	addressMap := make(map[string]bool)
	// Access nested data.data field for transactions
	if data, ok := result["data"].(map[string]interface{}); ok {
		if transactions, ok := data["data"].([]interface{}); ok {
			for _, tx := range transactions {
				if txMap, ok := tx.(map[string]interface{}); ok {
					// Try to extract from address
					if from, ok := txMap["from"].(string); ok && from != "" {
						addressMap[from] = true
					}
					// Try to extract to address
					if to, ok := txMap["to"].(string); ok && to != "" {
						addressMap[to] = true
					}
					// Try to extract signer address
					if signer, ok := txMap["signer"].(string); ok && signer != "" {
						addressMap[signer] = true
					}
				}
			}
		}
	}

	// Convert to slice
	if len(addressMap) > 0 {
		testAddresses = make([]string, 0, len(addressMap))
		for addr := range addressMap {
			testAddresses = append(testAddresses, addr)
		}
	}

	return nil
}

func main() {
	flag.Parse()

	// Validate parameters
	if duration == 0 && requests == 0 {
		fmt.Fprintf(os.Stderr, "Error: Must specify -d (duration) or -n (number of requests)\n")
		flag.Usage()
		os.Exit(1)
	}

	if concurrency <= 0 {
		fmt.Fprintf(os.Stderr, "Error: Concurrent workers must be greater than 0\n")
		os.Exit(1)
	}

	// Select endpoints to test
	testEndpoints := endpoints
	if testEndpoint != "" {
		found := false
		for _, ep := range endpoints {
			if ep.Path == testEndpoint {
				testEndpoints = []EndpointConfig{ep}
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "Warning: Endpoint %s not found, will test all endpoints\n", testEndpoint)
		}
	}

	// Display test configuration
	color.Cyan("=== HTTP Interface Stress Test ===\n")
	fmt.Printf("Server URL: %s\n", baseURL)
	fmt.Printf("Concurrent workers: %d\n", concurrency)
	if duration > 0 {
		fmt.Printf("Duration: %v\n", duration)
	} else {
		fmt.Printf("Total requests: %d\n", requests)
	}
	fmt.Printf("Request timeout: %v\n", timeout)
	fmt.Printf("Number of endpoints to test: %d\n", len(testEndpoints))
	fmt.Println()

	// Create HTTP client (optimized configuration)
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        200, // Increase total connections
			MaxIdleConnsPerHost: 100, // Increase connections per host
			IdleConnTimeout:     DefaultIdleConnTimeout,
			DisableCompression:  false, // Enable compression
			TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
			ForceAttemptHTTP2:   true, // Try to use HTTP/2 (if server supports it)
		},
	}

	// Load test data from API
	if !skipDataLoad {
		if err := loadTestDataFromAPI(client); err != nil {
			color.Red("Error: Failed to load test data: %v\n", err)
			os.Exit(1)
		}
	} else {
		color.Yellow("Skipping data preloading, using default test data\n")
		testAddresses = defaultAddresses
		validBlockRange = BlockRange{Min: 1, Max: 148025427}
		fmt.Println()
	}

	// Start test
	ctx, cancel := context.WithCancel(context.Background())
	if duration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), duration)
	}
	defer cancel()

	var wg sync.WaitGroup
	startTime := time.Now()

	// Start concurrent workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Pass testAddresses to avoid worker dependency on global state
			worker(ctx, client, testEndpoints, workerID, testAddresses)
		}(i)
	}

	// If request count is specified, wait for completion
	if requests > 0 {
		for atomic.LoadInt64(&stats.total) < int64(requests) {
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}

	// Wait for all workers to complete
	wg.Wait()
	endTime := time.Now()

	// Print statistics
	printStats(startTime, endTime, testEndpoints)
}

// worker is the worker function that executes requests
func worker(ctx context.Context, client *http.Client, endpoints []EndpointConfig, workerID int, addresses []string) {
	// Create independent random number generator for each worker
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// If request count is specified, check if reached
			if requests > 0 && atomic.LoadInt64(&stats.total) >= int64(requests) {
				return
			}

			// Randomly select an endpoint to test
			ep := selectRandomEndpoint(endpoints, rng)

			// Execute request (pass addresses to avoid dependency on global state)
			if err := makeRequest(client, ep, rng, addresses); err != nil {
				if verbose {
					color.Red("[Worker %d] Request failed: %v\n", workerID, err)
				}
			}

			// If request count is specified, check if reached
			if requests > 0 && atomic.LoadInt64(&stats.total) >= int64(requests) {
				return
			}
		}
	}
}

// selectRandomEndpoint randomly selects an endpoint
func selectRandomEndpoint(endpoints []EndpointConfig, rng *rand.Rand) EndpointConfig {
	if len(endpoints) == 0 {
		return EndpointConfig{}
	}
	return endpoints[rng.Intn(len(endpoints))]
}

// makeRequest executes a single HTTP request
// Returns error so caller can handle it
func makeRequest(client *http.Client, ep EndpointConfig, rng *rand.Rand, addresses []string) error {
	// Build URL
	u, err := url.Parse(baseURL + ep.Path)
	if err != nil {
		stats.RecordRequest()
		stats.RecordFailure()
		return fmt.Errorf("URL parsing failed: %w", err)
	}

	// Add query parameters
	q := u.Query()
	// First add static parameters
	for k, v := range ep.Params {
		q.Set(k, v)
	}
	// If there's a dynamic parameter builder, use it to build dynamic parameters (pass addresses to avoid dependency on global state)
	if ep.ParamBuilder != nil {
		dynamicParams := ep.ParamBuilder(rng, addresses)
		for k, v := range dynamicParams {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()

	// Record start time
	start := time.Now()

	// Send request
	resp, err := client.Get(u.String())
	latency := time.Since(start).Microseconds()

	stats.RecordRequest()

	if err != nil {
		if verbose {
			color.Red("[%s] Request failed: %v\n", ep.Name, err)
		}
		stats.RecordFailure()
		stats.RecordLatency(latency)
		return fmt.Errorf("HTTP request failed: %w", err)
	}

	// Ensure response body is closed (fix resource leak)
	// Key fix: Read and discard response body to ensure connection can be reused
	// HTTP/1.1 requires reading the entire response body to reuse connections
	// If not read, the connection will be closed and each request will need to establish a new connection, causing severe performance degradation
	// This has little impact in local testing (connection establishment is fast), but significant impact in server testing (high network latency)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// Record status code
	stats.RecordStatus(resp.StatusCode)

	// Update statistics
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		stats.RecordSuccess()
	} else {
		stats.RecordFailure()
		if verbose {
			color.Yellow("[%s] HTTP %d\n", ep.Name, resp.StatusCode)
		}
	}

	// Update latency statistics
	stats.RecordLatency(latency)

	return nil
}

// printStats prints test results
func printStats(startTime, endTime time.Time, endpoints []EndpointConfig) {
	duration := endTime.Sub(startTime)
	total, success, failed, totalLat, minLat, maxLat, statusCodes := stats.GetStats()

	color.Cyan("\n=== Test Results ===\n")
	fmt.Printf("Test duration: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("Total requests: %d\n", total)
	fmt.Printf("Successful requests: %d (%.2f%%)\n", success, float64(success)/float64(total)*100)
	fmt.Printf("Failed requests: %d (%.2f%%)\n", failed, float64(failed)/float64(total)*100)

	if total > 0 {
		avgLatency := float64(totalLat) / float64(total) / 1000.0 // Convert to milliseconds
		minLatMs := float64(minLat) / 1000.0
		maxLatMs := float64(maxLat) / 1000.0
		qps := float64(total) / duration.Seconds()

		fmt.Printf("\nPerformance metrics:\n")
		fmt.Printf("  QPS (requests per second): %.2f\n", qps)
		fmt.Printf("  Average latency: %.2f ms\n", avgLatency)
		fmt.Printf("  Minimum latency: %.2f ms\n", minLatMs)
		fmt.Printf("  Maximum latency: %.2f ms\n", maxLatMs)

		// Print status code distribution
		fmt.Printf("\nStatus code distribution:\n")
		for code, count := range statusCodes {
			percentage := float64(count) / float64(total) * 100
			statusColor := color.New(color.FgGreen)
			if code >= 400 {
				statusColor = color.New(color.FgRed)
			} else if code >= 300 {
				statusColor = color.New(color.FgYellow)
			}
			statusColor.Printf("  %d: %d (%.2f%%)\n", code, count, percentage)
		}
	}

	fmt.Printf("\nTested endpoints:\n")
	for _, ep := range endpoints {
		fmt.Printf("  - %s: %s\n", ep.Name, ep.Path)
	}
	fmt.Println()
}
