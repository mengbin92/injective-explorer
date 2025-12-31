# Redis and Cache Optimization

This document describes the Redis and cache optimization improvements implemented as Day 2 optimization tasks.

## Overview

The following optimizations have been implemented to improve Redis performance, reliability, and observability:

1. **Enhanced Connection Pool Configuration**
2. **Automatic Health Checking**
3. **Metrics Collection and Monitoring**
4. **Cache Warming Support**
5. **Retry and Backoff Policies**
6. **Metrics-Aware Cache Client**

## Configuration Changes

The Redis configuration in `internal/conf/conf.proto` has been extended with additional fields:

```protobuf
message Redis {
  string addr = 1;
  string password = 2;
  int32 db = 3;
  int32 pool_size = 4;
  google.protobuf.Duration read_timeout = 5;
  google.protobuf.Duration write_timeout = 6;

  // Redis connection pool advanced settings
  int32 min_idle_conns = 7;           // Minimum idle connections (default: 5)
  google.protobuf.Duration conn_max_lifetime = 8; // Connection max lifetime
  google.protobuf.Duration conn_max_idle_time = 9; // Connection max idle time
  google.protobuf.Duration dial_timeout = 10;     // Dial timeout
  google.protobuf.Duration pool_timeout = 11;     // Pool timeout
  int32 max_retries = 12;            // Max retries (default: 3)
  google.protobuf.Duration min_retry_backoff = 13; // Min retry backoff
  google.protobuf.Duration max_retry_backoff = 14; // Max retry backoff

  // Monitoring and health check settings
  bool enable_metrics = 15;          // Enable metrics collection
  google.protobuf.Duration health_check_interval = 16; // Health check interval

  // Cache warming settings
  bool enable_warming = 17;          // Enable cache warming on startup
  repeated string warm_keys = 18;    // Keys to warm on startup
  google.protobuf.Duration warm_ttl = 19; // Default TTL for warmed keys
}
```

## Implementation Details

### 1. Enhanced Connection Pool Configuration

The connection pool has been enhanced with the following improvements:

- **MinIdleConns**: Maintains a minimum number of idle connections for faster response times
- **ConnMaxLifetime**: Prevents connection leaks by limiting connection lifetime
- **ConnMaxIdleTime**: Closes idle connections to free resources
- **DialTimeout**: Prevents hanging on connection establishment
- **PoolTimeout**: Prevents waiting too long for a connection from the pool
- **Retry Policy**: Automatic retries with exponential backoff for transient failures

### 2. Automatic Health Checking

A background health check goroutine monitors Redis connectivity:

- Runs at configurable intervals (default: 30 seconds)
- Logs connectivity issues for monitoring
- Can be extended with alerting mechanisms

### 3. Metrics Collection and Monitoring

Comprehensive metrics are collected for cache operations:

**Cache Operation Metrics**:
- Total gets, sets, and deletes
- Cache hits and misses
- Hit rate percentage
- Error counts
- Average latency per operation

**Connection Pool Metrics**:
- Pool hits and misses
- Connection timeouts
- Total, idle, and stale connections

**Metrics Collection**:
- Automatic collection via Redis hooks
- Background metrics reporting every 60 seconds
- Programmatic access via `cache.GetMetrics()`

### 4. Cache Warming Support

Preloads frequently accessed data on application startup:

- Configurable list of keys to warm
- Extends TTL of existing keys
- Logs warming progress and statistics

### 5. Metrics-Aware Cache Client

A wrapper client (`cache.Client`) provides automatic metrics collection:

```go
// Example usage
client := cache.NewClient(ctx)

// Get operation - automatically records metrics
result := client.Get("key")

// Set operation - automatically records metrics
err := client.Set("key", "value", 5*time.Minute).Err()

// Get current metrics
metrics := cache.GetMetrics()
fmt.Printf("Hit rate: %.2f%%\n", cache.GetHitRate())
```

## Configuration Examples

### Production Configuration

```yaml
data:
  redis:
    addr: redis:6379
    password: "${REDIS_PASSWORD}"
    db: 0
    pool_size: 100
    min_idle_conns: 20
    conn_max_lifetime: 1h
    conn_max_idle_time: 30m
    dial_timeout: 5s
    pool_timeout: 5s
    max_retries: 3
    enable_metrics: true
    health_check_interval: 30s
    enable_warming: true
    warm_keys:
      - "price:inj"
      - "supply:inj"
      - "price:usdt"
    warm_ttl: 1h
```

### Development Configuration

```yaml
data:
  redis:
    addr: localhost:6379
    pool_size: 10
    enable_metrics: true
    enable_warming: false
```

## Monitoring and Observability

### Log Output

When metrics collection is enabled, you'll see periodic log entries:

```
INFO msg="Cache metrics report" total_gets=1250 total_sets=320 total_deletes=45 hits=950 misses=300 errors=0 hit_rate_percent="76.00" avg_latency="2.35ms"
INFO msg="Redis pool stats" pool_hits=5000 pool_misses=45 pool_timeouts=0 total_conns=15 idle_conns=10 stale_conns=0
```

### Accessing Metrics Programmatically

```go
// Get all metrics
metrics := cache.GetMetrics()

// Get hit rate
hitRate := cache.GetHitRate()

// Get average latency
avgLatency := cache.GetAverageLatency()

// Get pool statistics
poolStats := cache.GetPoolStats()

// Get health status
err := cache.GetHealthStatus(ctx)
```

## Performance Benefits

### Connection Pool Optimization
- Reduced connection establishment overhead
- Better resource utilization
- Improved response times under load

### Metrics and Monitoring
- Real-time visibility into cache performance
- Early detection of issues
- Data-driven optimization decisions

### Auto-Recovery and Resilience
- Automatic retry for transient failures
- Health monitoring prevents cascading failures
- Better fault tolerance

## Migration Guide

### Existing Code Compatibility

The existing `cache.GetRedisClient()` function continues to work without changes. For automatic metrics collection, use the new `cache.Client`:

**Before:**
```go
redisClient := cache.GetRedisClient()
val, err := redisClient.Get(ctx, "key").Result()
```

**After (with metrics):**
```go
client := cache.NewClient(ctx)
val, err := client.Get("key").Result()
```

### Configuration Migration

Add the new configuration options to your config file:

```yaml
data:
  redis:
    # Existing options
    addr: localhost:6379
    pool_size: 10

    # New options (add these)
    min_idle_conns: 5
    conn_max_lifetime: 1h
    conn_max_idle_time: 30m
    dial_timeout: 5s
    pool_timeout: 5s
    max_retries: 3
    enable_metrics: true
    health_check_interval: 30s
    enable_warming: false
```

## Testing

### Unit Tests
Run the existing test suite to ensure backward compatibility:

```bash
go test ./provider/cache/...
```

### Integration Tests
Test with actual Redis connection:

```bash
# Start Redis
docker run -d -p 6379:6379 redis:alpine

# Run tests
REDIS_HOST=localhost go test ./provider/cache/...
```

## Future Enhancements

Potential future improvements include:

1. **Distributed Caching**: Support for Redis Cluster
2. **Advanced Cache Policies**: LRU, LFU eviction strategies
3. **Cache Analytics Dashboard**: Web UI for metrics visualization
4. **Smart Cache Warming**: ML-based prediction of hot keys
5. **Cache Prefetching**: Proactive loading of related data
6. **Redis Sentinel Support**: High availability configuration

## Troubleshooting

### Common Issues

1. **High Connection Pool Miss Rate**
   - Increase `min_idle_conns` and `pool_size`
   - Check `conn_max_idle_time` is not too aggressive

2. **Cache Hit Rate Too Low**
   - Review cache TTL settings
   - Consider cache warming for frequently accessed data
   - Analyze access patterns to optimize key design

3. **High Latency**
   - Enable metrics to identify slow operations
   - Check Redis server performance
   - Review network connectivity

### Metrics Interpretation

- **Hit Rate < 70%**: Consider increasing cache TTL or warming more keys
- **Pool Misses > 100/hour**: Increase pool size or min idle connections
- **Timeouts > 0**: Check Redis server load and network, increase timeouts

## References

- [Redis Configuration](https://redis.io/docs/manual/config/)
- [Go Redis Client](https://github.com/redis/go-redis)
- [Kratos Framework](https://go-kratos.dev/)
