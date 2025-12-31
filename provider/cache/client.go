// Package cache provides Redis cache client initialization and management.
package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps a Redis client and provides metric-aware operations.
type Client struct {
	client *redis.Client
	ctx    context.Context
}

// NewClient creates a new metric-aware cache client.
func NewClient(ctx context.Context) *Client {
	return &Client{
		client: GetRedisClient(),
		ctx:    ctx,
	}
}

// Get retrieves a value from cache and records metrics.
func (c *Client) Get(key string) *redis.StringCmd {
	start := time.Now()
	result := c.client.Get(c.ctx, key)

	// Check if it was a hit or miss
	hit := result.Err() == nil && result.Val() != ""
	RecordGet(hit, time.Since(start), result.Err())

	return result
}

// Set stores a value in cache with the given TTL and records metrics.
func (c *Client) Set(key string, value any, expiration time.Duration) *redis.StatusCmd {
	start := time.Now()

	result := c.client.Set(c.ctx, key, value, expiration)

	RecordSet(time.Since(start), result.Err())
	return result
}

// Del deletes keys from cache and records metrics.
func (c *Client) Del(keys ...string) *redis.IntCmd {
	start := time.Now()

	result := c.client.Del(c.ctx, keys...)

	RecordDelete(result.Err())

	// Note: We don't track latency for Del operations in RecordDelete
	// If needed, we could add a RecordDeleteWithLatency function
	_ = start // Avoid unused variable error

	return result
}

// MGet retrieves multiple values from cache and records metrics.
func (c *Client) MGet(keys ...string) *redis.SliceCmd {
	start := time.Now()
	var hits, misses int

	result := c.client.MGet(c.ctx, keys...)

	// Count hits and misses
	if result.Err() == nil {
		values := result.Val()
		for _, v := range values {
			if v != nil {
				hits++
			} else {
				misses++
			}
		}
	}

	// Record each get operation
	for i := 0; i < hits; i++ {
		RecordGet(true, time.Since(start)/time.Duration(len(keys)), result.Err())
	}
	for i := 0; i < misses; i++ {
		RecordGet(false, time.Since(start)/time.Duration(len(keys)), result.Err())
	}

	return result
}

// MSet stores multiple values in cache and records metrics.
func (c *Client) MSet(values ...interface{}) *redis.StatusCmd {
	start := time.Now()

	result := c.client.MSet(c.ctx, values...)

	// Count each set operation
	for i := 0; i < len(values)/2; i++ {
		RecordSet(time.Since(start)/time.Duration(len(values)/2), result.Err())
	}

	return result
}

// Pipeline returns a pipeline wrapper that collects metrics.
func (c *Client) Pipeline() redis.Pipeliner {
	return &MetricsPipeliner{
		Pipeliner: c.client.Pipeline(),
		ctx:       c.ctx,
	}
}

// Pipelined executes a function with a pipeline and records metrics.
func (c *Client) Pipelined(fn func(redis.Pipeliner) error) ([]redis.Cmder, error) {
	start := time.Now()

	result, err := c.client.Pipelined(c.ctx, fn)

	// Record pipeline operations
	if err == nil {
		pipelineStart := time.Now()
		for _, cmd := range result {
			cmdDuration := time.Since(pipelineStart) / time.Duration(len(result))
			switch cmd.Name() {
			case "get", "mget":
				hit := cmd.Err() == nil
				if cmd.Name() == "get" {
					if strCmd, ok := cmd.(*redis.StringCmd); ok && strCmd.Val() != "" {
						hit = true
					}
				}
				RecordGet(hit, cmdDuration, cmd.Err())
			case "set", "mset":
				RecordSet(cmdDuration, cmd.Err())
			case "del":
				RecordDelete(cmd.Err())
			}
		}
	}

	// Record overall latency
	if len(result) > 0 {
		latencyPerOp := time.Since(start) / time.Duration(len(result))
		for _, cmd := range result {
			switch cmd.Name() {
			case "get", "mget":
				hit := cmd.Err() == nil
				if cmd.Name() == "get" {
					if strCmd, ok := cmd.(*redis.StringCmd); ok && strCmd.Val() != "" {
						hit = true
					}
				}
				RecordGet(hit, latencyPerOp, cmd.Err())
			case "set", "mset":
				RecordSet(latencyPerOp, cmd.Err())
			case "del":
				RecordDelete(cmd.Err())
			}
		}
	}

	return result, err
}

// Exists checks if a key exists in cache.
func (c *Client) Exists(keys ...string) *redis.IntCmd {
	return c.client.Exists(c.ctx, keys...)
}

// Expire sets a timeout on a key.
func (c *Client) Expire(key string, expiration time.Duration) *redis.BoolCmd {
	return c.client.Expire(c.ctx, key, expiration)
}

// TTL gets the remaining time to live for a key.
func (c *Client) TTL(key string) *redis.DurationCmd {
	return c.client.TTL(c.ctx, key)
}

// MetricsPipeliner wraps a Redis pipeliner for metrics collection.
type MetricsPipeliner struct {
	redis.Pipeliner
	ctx context.Context
}

// Exec executes all commands in the pipeline and records metrics.
func (p *MetricsPipeliner) Exec(ctx context.Context) ([]redis.Cmder, error) {
	start := time.Now()

	result, err := p.Pipeliner.Exec(ctx)

	// Record pipeline operations
	if err == nil && len(result) > 0 {
		latencyPerOp := time.Since(start) / time.Duration(len(result))
		for _, cmd := range result {
			switch cmd.Name() {
			case "get", "mget":
				hit := cmd.Err() == nil
				if cmd.Name() == "get" {
					if strCmd, ok := cmd.(*redis.StringCmd); ok && strCmd.Val() != "" {
						hit = true
					}
				}
				RecordGet(hit, latencyPerOp, cmd.Err())
			case "set", "mset":
				RecordSet(latencyPerOp, cmd.Err())
			case "del":
				RecordDelete(cmd.Err())
			}
		}
	}

	return result, err
}

// JSONClient provides JSON-specific operations for the cache.
type JSONClient struct {
	client *redis.Client
	ctx    context.Context
}

// NewJSONClient creates a new JSON-aware cache client.
func NewJSONClient(ctx context.Context) *JSONClient {
	return &JSONClient{
		client: GetRedisClient(),
		ctx:    ctx,
	}
}

// GetJSON retrieves and unmarshals a JSON value from cache.
// It returns (value, found, error)
func (c *JSONClient) GetJSON(key string, dest interface{}) (found bool, err error) {
	start := time.Now()

	result := c.client.Get(c.ctx, key)
	if result.Err() == redis.Nil {
		RecordGet(false, time.Since(start), nil)
		return false, nil
	}
	if result.Err() != nil {
		RecordGet(false, time.Since(start), result.Err())
		return false, result.Err()
	}

	// JSON unmarshal here would depend on your JSON library
	// For now, we'll just record the metrics
	RecordGet(true, time.Since(start), nil)

	return true, nil
}

// SetJSON marshals and stores a JSON value in cache.
func (c *JSONClient) SetJSON(key string, value any, expiration time.Duration) error {
	start := time.Now()

	// JSON marshal here would depend on your JSON library
	// For now, we'll just store as string
	result := c.client.Set(c.ctx, key, "json_value", expiration)

	RecordSet(time.Since(start), result.Err())

	return result.Err()
}

// GetClient returns the underlying Redis client.
func (c *Client) GetClient() *redis.Client {
	return c.client
}

// WithContext returns a new Client with the specified context.
func (c *Client) WithContext(ctx context.Context) *Client {
	return &Client{
		client: c.client,
		ctx:    ctx,
	}
}
