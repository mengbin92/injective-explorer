// Package main 提供HTTP接口并发压力测试工具
package main

import (
	"context"
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

// 常量定义
const (
	// DefaultMinLatency 默认最小延迟值（微秒），用于初始化
	DefaultMinLatency = int64(1e9)
	// DefaultMaxIdleConns 默认最大空闲连接数（已优化为200）
	DefaultMaxIdleConns = 200
	// DefaultMaxIdleConnsPerHost 每个host的最大空闲连接数（已优化为100）
	DefaultMaxIdleConnsPerHost = 100
	// DefaultIdleConnTimeout 空闲连接超时时间
	DefaultIdleConnTimeout = 90 * time.Second
	// DefaultTLSHandshakeTimeout TLS握手超时时间
	DefaultTLSHandshakeTimeout = 10 * time.Second
)

var (
	// 测试配置
	baseURL      string
	concurrency  int
	duration     time.Duration
	requests     int
	timeout      time.Duration
	verbose      bool
	testEndpoint string

	// 测试地址示例（用于需要address参数的接口）
	testAddresses = []string{
		"inj1lfycr67vur5gayrakw6xj06sx86f4zf7qex26d",
		"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z",
		"inj1ng84mfnq4z4tuh0cd7a28x0hxw75vxcm70ls9q",
		"inj1exyaj8xflhqvrs5x30lxms4egya4v527tzu2xq",
	}
)

// Stats 封装所有统计信息
type Stats struct {
	mu           sync.RWMutex
	total        int64
	success      int64
	failed       int64
	totalLatency int64 // 微秒
	minLatency   int64
	maxLatency   int64
	statusCodes  map[int]int64
}

// NewStats 创建新的统计对象
func NewStats() *Stats {
	return &Stats{
		minLatency:  DefaultMinLatency,
		statusCodes: make(map[int]int64),
	}
}

// RecordRequest 记录请求
func (s *Stats) RecordRequest() {
	atomic.AddInt64(&s.total, 1)
}

// RecordSuccess 记录成功请求
func (s *Stats) RecordSuccess() {
	atomic.AddInt64(&s.success, 1)
}

// RecordFailure 记录失败请求
func (s *Stats) RecordFailure() {
	atomic.AddInt64(&s.failed, 1)
}

// RecordLatency 记录延迟
func (s *Stats) RecordLatency(latency int64) {
	atomic.AddInt64(&s.totalLatency, latency)

	// 更新最小延迟
	for {
		old := atomic.LoadInt64(&s.minLatency)
		if latency >= old {
			break
		}
		if atomic.CompareAndSwapInt64(&s.minLatency, old, latency) {
			break
		}
	}

	// 更新最大延迟
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

// RecordStatus 记录状态码
func (s *Stats) RecordStatus(code int) {
	s.mu.Lock()
	s.statusCodes[code]++
	s.mu.Unlock()
}

// GetStats 获取统计信息快照
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

// 全局统计对象
var stats = NewStats()

func init() {
	flag.StringVar(&baseURL, "url", "http://localhost:8000", "服务器基础URL")
	flag.IntVar(&concurrency, "c", 10, "并发数")
	flag.DurationVar(&duration, "d", 0, "测试持续时间（如：30s, 5m）")
	flag.IntVar(&requests, "n", 0, "总请求数（0表示使用duration）")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "请求超时时间")
	flag.BoolVar(&verbose, "v", false, "详细输出")
	flag.StringVar(&testEndpoint, "endpoint", "", "指定测试的接口路径（空表示测试所有接口）")
}

// EndpointConfig 接口配置
type EndpointConfig struct {
	Path   string
	Method string
	Params map[string]string
	Name   string
}

// 所有可测试的接口
var endpoints = []EndpointConfig{
	{
		Name:   "健康检查",
		Path:   "/demo/health",
		Method: "GET",
		Params: map[string]string{},
	},
	{
		Name:   "最新区块高度",
		Path:   "/demo/block/latest-height",
		Method: "GET",
		Params: map[string]string{},
	},
	{
		Name:   "最新区块列表",
		Path:   "/demo/block/latest",
		Method: "GET",
		Params: map[string]string{
			"pagination.page":      "1",
			"pagination.page_size": "20",
		},
	},
	{
		Name:   "按高度获取区块",
		Path:   "/demo/block/by-height",
		Method: "GET",
		Params: map[string]string{
			"height": "1",
		},
	},
	{
		Name:   "最新交易列表",
		Path:   "/demo/transaction/latest",
		Method: "GET",
		Params: map[string]string{
			"pagination.page":      "1",
			"pagination.page_size": "20",
		},
	},
	{
		Name:   "交易统计",
		Path:   "/demo/transaction/stats",
		Method: "GET",
		Params: map[string]string{},
	},
	{
		Name:   "24小时失败交易",
		Path:   "/demo/transaction/failed-24h",
		Method: "GET",
		Params: map[string]string{
			"pagination.page":      "1",
			"pagination.page_size": "20",
		},
	},
	{
		Name:   "账户信息",
		Path:   "/demo/account/info",
		Method: "GET",
		Params: map[string]string{
			"address": testAddresses[0],
		},
	},
	{
		Name:   "账户余额",
		Path:   "/demo/account/balances",
		Method: "GET",
		Params: map[string]string{
			"address": testAddresses[0],
		},
	},
	{
		Name:   "账户交易",
		Path:   "/demo/account/transactions",
		Method: "GET",
		Params: map[string]string{
			"address":              testAddresses[0],
			"pagination.page":      "1",
			"pagination.page_size": "20",
		},
	},
}

func main() {
	flag.Parse()

	// 验证参数
	if duration == 0 && requests == 0 {
		fmt.Fprintf(os.Stderr, "错误: 必须指定 -d (持续时间) 或 -n (请求数)\n")
		flag.Usage()
		os.Exit(1)
	}

	if concurrency <= 0 {
		fmt.Fprintf(os.Stderr, "错误: 并发数必须大于0\n")
		os.Exit(1)
	}

	// 选择要测试的接口
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
			fmt.Fprintf(os.Stderr, "警告: 未找到接口 %s，将测试所有接口\n", testEndpoint)
		}
	}

	// 显示测试配置
	color.Cyan("=== HTTP接口压力测试 ===\n")
	fmt.Printf("服务器URL: %s\n", baseURL)
	fmt.Printf("并发数: %d\n", concurrency)
	if duration > 0 {
		fmt.Printf("持续时间: %v\n", duration)
	} else {
		fmt.Printf("总请求数: %d\n", requests)
	}
	fmt.Printf("请求超时: %v\n", timeout)
	fmt.Printf("测试接口数: %d\n", len(testEndpoints))
	fmt.Println()

	// 创建HTTP客户端（优化配置）
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        200,                    // 增加总连接数
			MaxIdleConnsPerHost: 100,                    // 增加每个host的连接数
			IdleConnTimeout:     DefaultIdleConnTimeout,
			DisableCompression:  false,                  // 启用压缩
			TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
			ForceAttemptHTTP2:   true,                   // 尝试使用 HTTP/2（如果服务器支持）
		},
	}

	// 启动测试
	ctx, cancel := context.WithCancel(context.Background())
	if duration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), duration)
	}
	defer cancel()

	var wg sync.WaitGroup
	startTime := time.Now()

	// 启动并发worker
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			worker(ctx, client, testEndpoints, workerID)
		}(i)
	}

	// 如果指定了请求数，等待完成
	if requests > 0 {
		for atomic.LoadInt64(&stats.total) < int64(requests) {
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}

	// 等待所有worker完成
	wg.Wait()
	endTime := time.Now()

	// 打印统计结果
	printStats(startTime, endTime, testEndpoints)
}

// worker 执行请求的worker函数
func worker(ctx context.Context, client *http.Client, endpoints []EndpointConfig, workerID int) {
	// 为每个worker创建独立的随机数生成器
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 如果指定了请求数，检查是否达到
			if requests > 0 && atomic.LoadInt64(&stats.total) >= int64(requests) {
				return
			}

			// 随机选择一个接口进行测试（改进：使用真正的随机选择）
			ep := selectRandomEndpoint(endpoints, rng)

			// 对于需要address的接口，随机选择地址
			ep = randomizeAddressIfNeeded(ep, rng)

			// 执行请求
			if err := makeRequest(client, ep); err != nil {
				if verbose {
					color.Red("[Worker %d] 请求失败: %v\n", workerID, err)
				}
			}

			// 如果指定了请求数，检查是否达到
			if requests > 0 && atomic.LoadInt64(&stats.total) >= int64(requests) {
				return
			}
		}
	}
}

// selectRandomEndpoint 随机选择一个endpoint
func selectRandomEndpoint(endpoints []EndpointConfig, rng *rand.Rand) EndpointConfig {
	if len(endpoints) == 0 {
		return EndpointConfig{}
	}
	return endpoints[rng.Intn(len(endpoints))]
}

// randomizeAddressIfNeeded 如果需要，随机化地址参数
func randomizeAddressIfNeeded(ep EndpointConfig, rng *rand.Rand) EndpointConfig {
	addressEndpoints := map[string]bool{
		"/demo/account/info":         true,
		"/demo/account/balances":     true,
		"/demo/account/transactions": true,
	}

	if !addressEndpoints[ep.Path] {
		return ep
	}

	epCopy := ep
	epCopy.Params = make(map[string]string)
	for k, v := range ep.Params {
		epCopy.Params[k] = v
	}

	if addr, ok := epCopy.Params["address"]; ok && addr == testAddresses[0] {
		epCopy.Params["address"] = testAddresses[rng.Intn(len(testAddresses))]
	}

	return epCopy
}

// makeRequest 执行单个HTTP请求
// 返回错误以便调用者可以处理
func makeRequest(client *http.Client, ep EndpointConfig) error {
	// 构建URL
	u, err := url.Parse(baseURL + ep.Path)
	if err != nil {
		stats.RecordRequest()
		stats.RecordFailure()
		return fmt.Errorf("URL解析失败: %w", err)
	}

	// 添加查询参数
	q := u.Query()
	for k, v := range ep.Params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	// 记录开始时间
	start := time.Now()

	// 发送请求
	resp, err := client.Get(u.String())
	latency := time.Since(start).Microseconds()

	stats.RecordRequest()

	if err != nil {
		if verbose {
			color.Red("[%s] 请求失败: %v\n", ep.Name, err)
		}
		stats.RecordFailure()
		stats.RecordLatency(latency)
		return fmt.Errorf("HTTP请求失败: %w", err)
	}

	// 确保响应体被关闭（修复资源泄漏）
	// 关键修复：读取并丢弃响应体，确保连接可以复用
	// HTTP/1.1 要求必须读取完响应体才能复用连接
	// 如果不读取，连接会被关闭，每个请求都需要建立新连接，导致性能急剧下降
	// 这在本地测试时影响不大（连接建立很快），但在服务器测试时影响显著（网络延迟高）
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// 记录状态码
	stats.RecordStatus(resp.StatusCode)

	// 更新统计
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		stats.RecordSuccess()
	} else {
		stats.RecordFailure()
		if verbose {
			color.Yellow("[%s] HTTP %d\n", ep.Name, resp.StatusCode)
		}
	}

	// 更新延迟统计
	stats.RecordLatency(latency)

	return nil
}

// printStats 打印统计结果
func printStats(startTime, endTime time.Time, endpoints []EndpointConfig) {
	duration := endTime.Sub(startTime)
	total, success, failed, totalLat, minLat, maxLat, statusCodes := stats.GetStats()

	color.Cyan("\n=== 测试结果 ===\n")
	fmt.Printf("测试时长: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("总请求数: %d\n", total)
	fmt.Printf("成功请求: %d (%.2f%%)\n", success, float64(success)/float64(total)*100)
	fmt.Printf("失败请求: %d (%.2f%%)\n", failed, float64(failed)/float64(total)*100)

	if total > 0 {
		avgLatency := float64(totalLat) / float64(total) / 1000.0 // 转换为毫秒
		minLatMs := float64(minLat) / 1000.0
		maxLatMs := float64(maxLat) / 1000.0
		qps := float64(total) / duration.Seconds()

		fmt.Printf("\n性能指标:\n")
		fmt.Printf("  QPS (每秒请求数): %.2f\n", qps)
		fmt.Printf("  平均延迟: %.2f ms\n", avgLatency)
		fmt.Printf("  最小延迟: %.2f ms\n", minLatMs)
		fmt.Printf("  最大延迟: %.2f ms\n", maxLatMs)

		// 打印状态码分布
		fmt.Printf("\n状态码分布:\n")
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

	fmt.Printf("\n测试接口:\n")
	for _, ep := range endpoints {
		fmt.Printf("  - %s: %s\n", ep.Name, ep.Path)
	}
	fmt.Println()
}
