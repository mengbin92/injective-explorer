// Package main 提供HTTP接口并发压力测试工具
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
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

	// 统计信息
	totalRequests   int64
	successRequests int64
	failedRequests  int64
	totalLatency    int64 // 微秒
	minLatency      int64 = 1e9
	maxLatency      int64
	statusCodes     sync.Map // map[int]int64

	// 测试地址示例（用于需要address参数的接口）
	testAddresses = []string{
		"inj1lfycr67vur5gayrakw6xj06sx86f4zf7qex26d",
		"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z",
		"inj1ng84mfnq4z4tuh0cd7a28x0hxw75vxcm70ls9q",
		"inj1exyaj8xflhqvrs5x30lxms4egya4v527tzu2xq",
	}
)

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

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        concurrency * 2,
			MaxIdleConnsPerHost: concurrency,
			IdleConnTimeout:     90 * time.Second,
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
		for atomic.LoadInt64(&totalRequests) < int64(requests) {
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
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 如果指定了请求数，检查是否达到
			if requests > 0 && atomic.LoadInt64(&totalRequests) >= int64(requests) {
				return
			}

			// 随机选择一个接口进行测试
			ep := endpoints[workerID%len(endpoints)]

			// 对于需要address的接口，随机选择地址
			if ep.Path == "/demo/account/info" || ep.Path == "/demo/account/balances" || ep.Path == "/demo/account/transactions" {
				epCopy := ep
				epCopy.Params = make(map[string]string)
				for k, v := range ep.Params {
					epCopy.Params[k] = v
				}
				if k, ok := epCopy.Params["address"]; ok && k == testAddresses[0] {
					epCopy.Params["address"] = testAddresses[workerID%len(testAddresses)]
				}
				ep = epCopy
			}

			// 执行请求
			makeRequest(client, ep)

			// 如果指定了请求数，检查是否达到
			if requests > 0 && atomic.LoadInt64(&totalRequests) >= int64(requests) {
				return
			}
		}
	}
}

// makeRequest 执行单个HTTP请求
func makeRequest(client *http.Client, ep EndpointConfig) {
	// 构建URL
	u, err := url.Parse(baseURL + ep.Path)
	if err != nil {
		if verbose {
			color.Red("URL解析错误: %v\n", err)
		}
		atomic.AddInt64(&failedRequests, 1)
		atomic.AddInt64(&totalRequests, 1)
		return
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

	atomic.AddInt64(&totalRequests, 1)

	if err != nil {
		if verbose {
			color.Red("[%s] 请求失败: %v\n", ep.Name, err)
		}
		atomic.AddInt64(&failedRequests, 1)
		return
	}
	defer resp.Body.Close()

	// 记录状态码
	statusCode := resp.StatusCode
	if count, ok := statusCodes.Load(statusCode); ok {
		statusCodes.Store(statusCode, count.(int64)+1)
	} else {
		statusCodes.Store(statusCode, int64(1))
	}

	// 更新统计
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		atomic.AddInt64(&successRequests, 1)
	} else {
		atomic.AddInt64(&failedRequests, 1)
		if verbose {
			color.Yellow("[%s] HTTP %d\n", ep.Name, statusCode)
		}
	}

	// 更新延迟统计
	atomic.AddInt64(&totalLatency, latency)

	// 更新最小延迟
	for {
		old := atomic.LoadInt64(&minLatency)
		if latency >= old || atomic.CompareAndSwapInt64(&minLatency, old, latency) {
			break
		}
	}

	// 更新最大延迟
	for {
		old := atomic.LoadInt64(&maxLatency)
		if latency <= old || atomic.CompareAndSwapInt64(&maxLatency, old, latency) {
			break
		}
	}
}

// printStats 打印统计结果
func printStats(startTime, endTime time.Time, endpoints []EndpointConfig) {
	duration := endTime.Sub(startTime)
	total := atomic.LoadInt64(&totalRequests)
	success := atomic.LoadInt64(&successRequests)
	failed := atomic.LoadInt64(&failedRequests)
	totalLat := atomic.LoadInt64(&totalLatency)

	color.Cyan("\n=== 测试结果 ===\n")
	fmt.Printf("测试时长: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("总请求数: %d\n", total)
	fmt.Printf("成功请求: %d (%.2f%%)\n", success, float64(success)/float64(total)*100)
	fmt.Printf("失败请求: %d (%.2f%%)\n", failed, float64(failed)/float64(total)*100)

	if total > 0 {
		avgLatency := float64(totalLat) / float64(total) / 1000.0 // 转换为毫秒
		minLat := float64(atomic.LoadInt64(&minLatency)) / 1000.0
		maxLat := float64(atomic.LoadInt64(&maxLatency)) / 1000.0
		qps := float64(total) / duration.Seconds()

		fmt.Printf("\n性能指标:\n")
		fmt.Printf("  QPS (每秒请求数): %.2f\n", qps)
		fmt.Printf("  平均延迟: %.2f ms\n", avgLatency)
		fmt.Printf("  最小延迟: %.2f ms\n", minLat)
		fmt.Printf("  最大延迟: %.2f ms\n", maxLat)

		// 打印状态码分布
		fmt.Printf("\n状态码分布:\n")
		statusCodes.Range(func(key, value interface{}) bool {
			code := key.(int)
			count := value.(int64)
			percentage := float64(count) / float64(total) * 100
			statusColor := color.New(color.FgGreen)
			if code >= 400 {
				statusColor = color.New(color.FgRed)
			} else if code >= 300 {
				statusColor = color.New(color.FgYellow)
			}
			statusColor.Printf("  %d: %d (%.2f%%)\n", code, count, percentage)
			return true
		})
	}

	fmt.Printf("\n测试接口:\n")
	for _, ep := range endpoints {
		fmt.Printf("  - %s: %s\n", ep.Name, ep.Path)
	}
	fmt.Println()
}
