# Injective Explorer

A comprehensive blockchain explorer API service for the Injective network, built with the Kratos framework. This service provides RESTful and gRPC APIs for querying blockchain data including blocks, transactions, and account information.

## Features

- **Block Queries**: Get latest blocks with pagination, query blocks by height, and retrieve latest block height
- **Transaction Queries**: Get latest transactions with pagination and query transactions by hash
- **Account Queries**: Get account information, transactions with pagination, and balances
- **Streaming APIs**: Real-time streaming of blocks, transactions, and account updates via gRPC streaming
- **Health Check**: Comprehensive health check endpoint with component status and latency metrics
- **Database-First Strategy**: Prioritizes database queries over direct blockchain API calls for improved performance
- **Pagination Support**: Both page-based and cursor-based pagination for efficient data retrieval
- **Block Synchronization**: Real-time block synchronization with historical block backfill
- **Dual Protocol Support**: Both gRPC and HTTP/REST APIs
- **gRPC-Web Support**: Browser-compatible gRPC-Web API via Envoy Proxy
- **Multiple Database Support**: MySQL, PostgreSQL, and SQLite
- **Caching**: Redis support for improved performance
- **Structured Logging**: Comprehensive logging with Zap
- **Input Validation**: Built-in request validation and error handling
- **Type-Safe Responses**: Uses Protocol Buffer messages for type-safe API responses
- **Data Storage Optimization**: Advanced storage optimization features including:
  - **Data Compression**: Automatic compression of large transaction fields (>1KB) to reduce storage by 60-80%
  - **Data Archiving**: Automatic archiving of old transactions (>6 months) to separate archive tables
  - **Storage Separation**: Optional object storage (MinIO/S3/OSS/COS) for large binary fields (>10KB)
  - **Backward Compatibility**: Seamless support for both compressed and uncompressed data formats

## Architecture

This project follows the clean architecture pattern with clear separation of concerns:

```
injective-explorer/
├── api/              # API definitions (Protocol Buffers)
├── cmd/              # Application entry points
├── configs/          # Configuration files
├── internal/         # Internal application code
│   ├── biz/          # Business logic
│   ├── conf/         # Configuration definitions
│   ├── global/       # Global variables and initialization
│   ├── models/       # Data models
│   ├── server/       # gRPC and HTTP server setup
│   └── service/      # Service implementations
│       ├── clients.go           # Shared blockchain client initialization
│       ├── constants.go         # Service-level constants
│       ├── demo.go              # API service implementation
│       ├── sync.go              # Block synchronization service
│       ├── archive.go           # Transaction archiving service
│       └── storage_migration.go # Data migration to object storage service
└── provider/         # External service providers
    ├── cache/        # Redis cache provider
    ├── chain/        # Blockchain client providers
    ├── db/           # Database providers
    ├── logger/       # Logging provider
    └── storage/      # Object storage providers (MinIO, S3, OSS, COS)
```

## Prerequisites

- Go 1.19 or later
- Protocol Buffer Compiler (protoc)
- Make (optional, for using Makefile commands)

## Installation

### 1. Install Dependencies

```bash
# Install required Go tools
make init
```

Or manually install:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest
go install github.com/google/wire/cmd/wire@latest
```

### 2. Generate Code

Generate Protocol Buffer code and other necessary files:

```bash
make all
```

This will:
- Generate gRPC and HTTP code from proto files
- Generate OpenAPI specifications
- Generate configuration code

### 3. Build

```bash
make build
```

Or build manually:

```bash
go build -ldflags "-X main.Version=$(git describe --tags --always)" -o ./bin/injective-explorer ./cmd/injective-explorer
```

## Configuration

The application supports configuration through both YAML files and environment variables. Environment variables take precedence over YAML configuration values.

### Environment Variables

The application uses environment variables for flexible configuration across different environments (development, staging, production). 

#### Quick Start

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` with your actual configuration values:
   ```bash
   # Edit .env file
   vim .env
   ```

3. The application will automatically load environment variables when started.

#### Available Environment Variables

See `.env.example` for a complete list of available environment variables with detailed descriptions. Key variables include:

**Server Configuration:**
- `HTTP_PORT`: HTTP server port (default: 8000)
- `GRPC_PORT`: gRPC server port (default: 9000)
- `ENABLE_PPROF`: Enable pprof profiling endpoint (default: false, **disable in production**)

**Database Configuration:**
- `DB_USER`: Database username (default: root)
- `DB_PASSWORD`: Database password (**change in production**)
- `DB_HOST`: Database host address (default: localhost)
- `DB_PORT`: Database port (default: 3306)
- `DB_NAME`: Database name (default: explorer)

**Redis Configuration:**
- `REDIS_HOST`: Redis host address (default: localhost)
- `REDIS_PORT`: Redis port (default: 6379)
- `REDIS_PASSWORD`: Redis password (default: empty)
- `REDIS_DB`: Redis database number (default: 0)

**Object Storage Configuration:**
- `OBJECT_STORAGE_PROVIDER`: Object storage provider - minio, s3, oss, cos (default: minio)
- `OBJECT_STORAGE_ENDPOINT`: Object storage endpoint (default: localhost:9000)
- `OBJECT_STORAGE_ACCESS_KEY`: Access key ID for authentication
- `OBJECT_STORAGE_SECRET_KEY`: Secret access key for authentication
- `OBJECT_STORAGE_BUCKET`: Bucket name for storing objects (default: injective-explorer)
- `OBJECT_STORAGE_REGION`: Region for the bucket (default: us-east-1)
- `OBJECT_STORAGE_USE_SSL`: Use SSL/TLS for connections (default: false)
- `OBJECT_STORAGE_PATH_PREFIX`: Path prefix for objects (default: transactions/)
- `OBJECT_STORAGE_ENABLED`: Enable object storage (default: false)

#### Kratos Configuration System

The application uses Kratos configuration system with the following priority order (highest to lowest):

1. **Environment Variables** (highest priority)
2. **YAML Configuration File** (lowest priority)

Kratos supports environment variable override using dot-notation:
- `SERVER_HTTP_ADDR` overrides `server.http.addr`
- `DATA_DATABASE_SOURCE` overrides `data.database.source`
- `DATA_REDIS_ADDR` overrides `data.redis.addr`
- `LOG_LEVEL` overrides `log.level`
- `LOG_FORMAT` overrides `log.format`

**Example:**
```bash
# Override HTTP server address via environment variable
export SERVER_HTTP_ADDR="0.0.0.0:8080"

# Override database connection string
export DATA_DATABASE_SOURCE="user:password@tcp(host:port)/dbname"
```

**Note**: The YAML placeholder syntax (`${VAR:default}`) in `config.yaml` also supports by Kratos file source.

#### Docker Compose Integration

When using Docker Compose, environment variables are automatically loaded from the `environment` section in `docker-compose.yaml`. The application will use these values to override the default configuration.

**Example docker-compose.yaml:**
```yaml
services:
  injective-explorer:
    environment:
      - HTTP_PORT=8000
      - GRPC_PORT=9000
      - DB_HOST=injective-explorer-mysql
      - DB_PASSWORD=your_password
      - REDIS_HOST=injective-explorer-redis
```

### YAML Configuration

Create or modify `configs/config.yaml`:

```yaml
server:
  http:
    addr: 0.0.0.0:${HTTP_PORT:8000}
    timeout: 30s
  grpc:
    addr: 0.0.0.0:${GRPC_PORT:9000}
    timeout: 30s
  enable_swagger: true

data:
  database:
    driver: mysql
    source: ${DB_USER:root}:${DB_PASSWORD:jKBrZHGcsNG5fMc52EWz}@tcp(${DB_HOST:localhost}:${DB_PORT:3306})/${DB_NAME:explorer}?charset=utf8mb4&parseTime=True&loc=Local
  redis:
    addr: ${REDIS_HOST:localhost}:${REDIS_PORT:6379}
    password: ${REDIS_PASSWORD:}
    db: ${REDIS_DB:0}
    pool_size: 100
    read_timeout: 0.2s
    write_timeout: 0.2s
  object_storage:
    provider: ${OBJECT_STORAGE_PROVIDER:minio} # s3, oss, cos, minio
    endpoint: ${OBJECT_STORAGE_ENDPOINT:localhost:9000}
    access_key_id: ${OBJECT_STORAGE_ACCESS_KEY:minioadmin}
    secret_access_key: ${OBJECT_STORAGE_SECRET_KEY:minioadmin}
    bucket_name: ${OBJECT_STORAGE_BUCKET:injective-explorer}
    region: ${OBJECT_STORAGE_REGION:us-east-1}
    use_ssl: ${OBJECT_STORAGE_USE_SSL:false}
    path_prefix: ${OBJECT_STORAGE_PATH_PREFIX:transactions/}
    enabled: ${OBJECT_STORAGE_ENABLED:false} # Default disabled for backward compatibility

log:
  # 支持debug(-1)、info(0)、warn(1)、error(2)、dpanic(3)、panic(4)、fatal(5)
  level: -1
  # json or console
  format: console

chain:
  network: mainnet
  lcd_endpoint: https://sentry.lcd.injective.network
  tm_endpoint: https://sentry.tm.injective.network:443
  chain_grpc_endpoint: sentry.chain.grpc.injective.network:443
  chain_stream_grpc_endpoint: sentry.chain.stream.injective.network:443
  exchange_grpc_endpoint: sentry.exchange.grpc.injective.network:443
  explorer_grpc_endpoint: sentry.explorer.grpc.injective.network:443
  chain_id: injective-1
  fee_denom: inj
  name: mainnet
  official_tokens_list_url: https://github.com/InjectiveLabs/injective-lists/raw/master/json/tokens/testnet.json

sync:
  # Redis 队列配置
  redis:
    queue_key: "tx:hash:queue"

  # 时间间隔配置
  intervals:
    block_poll: 5s # 区块轮询间隔
    tx_process: 1s # 交易处理间隔
    historical_sync: 10s # 历史区块同步间隔
    historical_batch_delay: 1s # 历史同步批次延迟
    backward_sync_log: 10s # 向后同步进度日志时间间隔
    rpc_status_retry: 5s # RPC 状态获取失败重试延迟

  # 超时配置
  timeouts:
    block_query: 30s # 区块查询超时

  # 并发配置
  concurrency:
    tx_processors: 5 # 并发交易处理器数量
    backward_sync_workers: 5 # 向后同步工作线程数
    historical_sync_workers: 2 # 历史同步工作线程数

  # 批次大小配置
  batch_size:
    backward_sync: 10 # 向后同步批次大小
    historical_sync: 5 # 历史同步批次大小
    missing_block_check: 100 # 查找缺失区块的批次大小
    backward_sync_log: 100 # 向后同步进度日志间隔（每 N 个区块）

  # 重试配置
  retry:
    max_attempts: 3 # 最大重试次数
    initial_delay: 2s # 初始重试延迟

  # 区块配置
  blocks:
    earliest_height: 143726350 # 最早区块高度的默认值
```

### Network Configuration

The service supports three network modes:

1. **mainnet**: Pre-configured mainnet settings
2. **testnet**: Pre-configured testnet settings
3. **custom**: Custom network configuration using the provided endpoints

## API Documentation

### Block APIs

#### Get Latest Blocks

```http
GET demo/block/latest?page=1&page_size=1
```

**Query Parameters**:
- `page` (int32, optional): Page number (default: 1, min: 1)
- `page_size` (int32, optional): Page size (default: 20, min: 1, max: 100)
- `cursor` (string, optional): Cursor for cursor-based pagination (takes priority over page)

**Response**:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "data": [
            {
                "block_hash": "0xd666e1d52bbf7bb482e86a9e13139af6df45f6362d3b8db5ee4c298595e523ba",
                "block_unix_timestamp": 1763085718628,
                "height": 141514703,
                "num_txs": 12,
                "proposer": "injvalcons1rtejpld5nf6qnvtn2uhumrenqu9hmc06sv6xuq"
            },
            {
                "block_hash": "0x262deb1cadb33939be19d246af0de8859e69fbd78105c5e4f34d460d4a8772d3",
                "block_unix_timestamp": 1763085717529,
                "height": 141514702,
                "num_txs": 27,
                "proposer": "injvalcons1euzqu2apctue3mzdz9qr3cu5v6yefp3w9ffhkg"
            },
            {
                "block_hash": "0x916ef620bdff1b5f4293080973192fce5d39741ae2ef47b8f522a526dc4e2eb3",
                "block_unix_timestamp": 1763085716893,
                "height": 141514701,
                "num_txs": 16,
                "proposer": "injvalcons1vzrkqls72mmwu7f54whktq6vjttpsyzvfu0l8l"
            },
            {
                "block_hash": "0x6863af32fffd06e2ed75ee941b2b295142feb19fa0af4d7df81d3c87fbda9222",
                "block_unix_timestamp": 1763085716306,
                "height": 141514700,
                "num_txs": 16,
                "proposer": "injvalcons1d3tqsnsulge3v2xgavw6d7r9jgj902t4mv9u5l"
            },
            {
                "block_hash": "0x80345004bd65b2c8c26480e6bc632b76c3cf138d60f9f7083b2fb5838562f17d",
                "block_unix_timestamp": 1763085715717,
                "height": 141514699,
                "num_txs": 11,
                "proposer": "injvalcons175ctmkculqfkwzn0du09w3r56ny38p04w8fen0"
            }
        ],
        "pagination": {
            "has_next": true,
            "has_prev": false,
            "page": 1,
            "page_size": 5,
            "total": 1825,
            "total_pages": 365
        }
    }
}
```

Returns the latest blocks from the blockchain with pagination support. The service uses a database-first strategy, querying from the local database first and falling back to the blockchain API if needed.

#### Get Block by Height
```http
GET /demo/block/by-height?height={height}
```

Parameters:
- `height` (int64): Block height

Returns block information for the specified height.

**Response**:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "block_hash": "0xb66050282da5383cf53bd916a6c24ecbb5209f6a7e6d5930b46d16f7783f33bc",
        "block_unix_timestamp": 1763032074424,
        "height": 141431945,
        "moniker": "Everstake",
        "num_txs": 4,
        "parent_hash": "0x43f1243d0b59dda6ba4291bb96eedc9d2877e0ef0d844f94cf5ecf21e9af635c",
        "proposer": "injvalcons175ctmkculqfkwzn0du09w3r56ny38p04w8fen0",
        "timestamp": "2025-11-13 11:07:54.424 +0000 UTC",
        "txs": [
            "{\"block_number\":141431945,\"block_timestamp\":\"2025-11-13 11:07:54.424 +0000 UTC\",\"hash\":\"0x25225b053c8b760fbf5d1ab9a8775296105617533217cf1d541a956bf1b4bbf5\",\"messages\":\"W3sidHlwZSI6Ii9jb3Ntd2FzbS53YXNtLnYxLk1zZ0V4ZWN1dGVDb250cmFjdCIsInZhbHVlIjp7InR5cGUiOiIvY29zbXdhc20ud2FzbS52MS5Nc2dFeGVjdXRlQ29udHJhY3QiLCJ2YWx1ZSI6eyIkYmluYXJ5Ijp7ImJhc2U2NCI6IkNpcHBibW94TXpScVkyMXlPRGN6TURKdWVYcG1lVFZ5TWpkamNuaHFOV1IyWjJkbWVYaHFPV2M1TlhZU0ttbHVhakUyTkhKMFpEQjNabXRtWlc0d2RuRmtPVGhrTkRSa056UjBkVGMwYUdFMWQzQmhkbTQ1WkJvU2V5SnRZWEpyWlhSZmJXRnJaU0k2ZTMxOSIsInN1YlR5cGUiOiIwMCJ9fX19XQ==\",\"tx_number\":2650610892,\"tx_msg_types\":\"WyIvY29zbXdhc20ud2FzbS52MS5Nc2dFeGVjdXRlQ29udHJhY3QiXQ==\",\"logs\":\"W3sibXNnX2luZGV4IjoiMCIsImV2ZW50cyI6W3sidHlwZSI6Im1lc3NhZ2UiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJhY3Rpb24iLCJ2YWx1ZSI6Ii9jb3Ntd2FzbS53YXNtLnYxLk1zZ0V4ZWN1dGVDb250cmFjdCIsImluZGV4Ijp0cnVlfSx7ImtleSI6InNlbmRlciIsInZhbHVlIjoiaW5qMTM0amNtcjg3MzAybnl6Znk1cjI3Y3J4ajVkdmdnZnl4ajlnOTV2IiwiaW5kZXgiOnRydWV9LHsia2V5IjoibW9kdWxlIiwidmFsdWUiOiJ3YXNtIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibXNnX2luZGV4IiwidmFsdWUiOiIwIiwiaW5kZXgiOnRydWV9XX0seyJ0eXBlIjoiZXhlY3V0ZSIsImF0dHJpYnV0ZXMiOlt7ImtleSI6Il9jb250cmFjdF9hZGRyZXNzIiwidmFsdWUiOiJpbmoxNjRydGQwd2ZrZmVuMHZxZDk4ZDQ0ZDc0dHU3NGhhNXdwYXZuOWQiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJ3YXNtLW1hcmtldF9tYWtlIiwiYXR0cmlidXRlcyI6W3sia2V5IjoiX2NvbnRyYWN0X2FkZHJlc3MiLCJ2YWx1ZSI6ImluajE2NHJ0ZDB3ZmtmZW4wdnFkOThkNDRkNzR0dTc0aGE1d3Bhdm45ZCIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImNvbnRyYWN0X3R5cGUiLCJ2YWx1ZSI6ImNyYXRlcy5pbzp2YXVsdC1jcG1tLXNwb3QiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJjb250cmFjdF92ZXJzaW9uIiwidmFsdWUiOiIwLjIuNyIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19XX1d\",\"signatures\":[{\"pubkey\":\"0326f26bde5c9335580d2595ebcdf4cd33fc2ea44e0d1d9642b09c1c6ec90d8571\",\"address\":\"inj134jcmr87302nyzfy5r27crxj5dvggfyxj9g95v\",\"sequence\":3706,\"signature\":\"CbomG8o5Q3KEvfNIk5OFXv6z/Ga7ynj7nAd5z5JWh3VA/cUr2BXXgQC1iUXD8O5qvkvSlGHI1r0NGLgFGZK0RQ==\"}],\"block_unix_timestamp\":1763032074424}",
            "{\"block_number\":141431945,\"block_timestamp\":\"2025-11-13 11:07:54.424 +0000 UTC\",\"hash\":\"0xe5bb3c497a55b14e8435b94df272929587578c25523100ea8156269774049f5c\",\"messages\":\"W3sidHlwZSI6Ii9jb3Ntb3MuYXV0aHoudjFiZXRhMS5Nc2dFeGVjIiwidmFsdWUiOnsidHlwZSI6Ii9jb3Ntb3MuYXV0aHoudjFiZXRhMS5Nc2dFeGVjIiwidmFsdWUiOnsiJGJpbmFyeSI6eyJiYXNlNjQiOiJDaXBwYm1veE4zRnFOR1pyTlRSNk5qTm5jM2N6ZG1SeE5IRjNlWG8wTUhrNWRXVTFhemxoYm1aMWVHZ1NpUVlLTUM5cGJtcGxZM1JwZG1VdVpYaGphR0Z1WjJVdWRqRmlaWFJoTVM1TmMyZENZWFJqYUZWd1pHRjBaVTl5WkdWeWN4TFVCUW9xYVc1cU1XcGtaR2h4YXpabFozZGpkM0ozZW1WMk5YZHFaVzVsYUhRd2VXdHRiSGRvWTJWb1l6UjZFa0l3ZURrek5XSTNNRFZpTlRrME0ySXdaVEZpT0RVNU5qVXhaREpqWTJZek56VmlZemsyWkdaa1pEY3dNREF3TURBd01EQXdNREF3TURBd01EQXdNREF3TWpjaVFqQjRaV1l3WkdRMk16TmtZVFV5WTJaak1qRmtZamcyTm1Jd05UUTJNMk00WWpRell6QXlNalkxWWpnMlltWTVNalUzWVRWa016azRORGhqWkRKbFpXRXhNVUtOQWdwQ01IaGxaakJrWkRZek0yUmhOVEpqWm1NeU1XUmlPRFkyWWpBMU5EWXpZemhpTkROak1ESXlOalZpT0RaaVpqa3lOVGRoTldRek9UZzBPR05rTW1WbFlURXhFcU1CQ2tJd2VEa3pOV0kzTURWaU5UazBNMkl3WlRGaU9EVTVOalV4WkRKalkyWXpOelZpWXprMlpHWmtaRGN3TURBd01EQXdNREF3TURBd01EQXdNREF3TURBd01qY1NLbWx1YWpFM2NXbzBabXMxTkhvMk0yZHpkek4yWkhFMGNYZDVlalF3ZVRsMVpUVnJPV0Z1Wm5WNGFCb2JORGsyTXpVd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXdJaFF5TWpVMU9EQXdNREF3TURBd01EQXdNREF3TUJnQklod3lOems1TURBd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXdLZ0V3UW8wQ0NrSXdlR1ZtTUdSa05qTXpaR0UxTW1ObVl6SXhaR0k0TmpaaU1EVTBOak5qT0dJME0yTXdNakkyTldJNE5tSm1PVEkxTjJFMVpETTVPRFE0WTJReVpXVmhNVEVTb3dFS1FqQjRPVE0xWWpjd05XSTFPVFF6WWpCbE1XSTROVGsyTlRGa01tTmpaak0zTldKak9UWmtabVJrTnpBd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXlOeElxYVc1cU1UZHhhalJtYXpVMGVqWXpaM04zTTNaa2NUUnhkM2w2TkRCNU9YVmxOV3M1WVc1bWRYaG9HaHMxTURBMk5qQXdNREF3TURBd01EQXdNREF3TURBd01EQXdNREFpRkRJeU5UVTRNREF3TURBd01EQXdNREF3TURBd0dBSWlIREk0TWpNd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXdNREFxQVRBPSIsInN1YlR5cGUiOiIwMCJ9fX19XQ==\",\"tx_number\":2650610891,\"tx_msg_types\":\"WyIvY29zbW9zLmF1dGh6LnYxYmV0YTEuTXNnRXhlYyJd\",\"logs\":\"W3sibXNnX2luZGV4IjoiMCIsImV2ZW50cyI6W3sidHlwZSI6Im1lc3NhZ2UiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJhY3Rpb24iLCJ2YWx1ZSI6Ii9jb3Ntb3MuYXV0aHoudjFiZXRhMS5Nc2dFeGVjIiwiaW5kZXgiOnRydWV9LHsia2V5Ijoic2VuZGVyIiwidmFsdWUiOiJpbmoxN3FqNGZrNTR6NjNnc3czdmRxNHF3eXo0MHk5dWU1azlhbmZ1eGgiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtb2R1bGUiLCJ2YWx1ZSI6ImF1dGh6IiwiaW5kZXgiOnRydWV9LHsia2V5IjoibXNnX2luZGV4IiwidmFsdWUiOiIwIiwiaW5kZXgiOnRydWV9XX0seyJ0eXBlIjoiaW5qZWN0aXZlLmV4Y2hhbmdlLnYyLkV2ZW50Q2FuY2VsRGVyaXZhdGl2ZU9yZGVyIiwiYXR0cmlidXRlcyI6W3sia2V5IjoiaXNMaW1pdENhbmNlbCIsInZhbHVlIjoidHJ1ZSIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImxpbWl0X29yZGVyIiwidmFsdWUiOiJ7XCJvcmRlcl9pbmZvXCI6e1wic3ViYWNjb3VudF9pZFwiOlwiMHg5MzViNzA1YjU5NDNiMGUxYjg1OTY1MWQyY2NmMzc1YmM5NmRmZGQ3MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDI3XCIsXCJmZWVfcmVjaXBpZW50XCI6XCJpbmoxN3FqNGZrNTR6NjNnc3czdmRxNHF3eXo0MHk5dWU1azlhbmZ1eGhcIixcInByaWNlXCI6XCI0OTYuNDIwMDAwMDAwMDAwMDAwMDAwXCIsXCJxdWFudGl0eVwiOlwiMjIuNTU1MDAwMDAwMDAwMDAwMDAwXCIsXCJjaWRcIjpcIlwifSxcIm9yZGVyX3R5cGVcIjpcIkJVWVwiLFwibWFyZ2luXCI6XCIyNzk5LjAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmlsbGFibGVcIjpcIjIyLjU1NTAwMDAwMDAwMDAwMDAwMFwiLFwidHJpZ2dlcl9wcmljZVwiOlwiMC4wMDAwMDAwMDAwMDAwMDAwMDBcIixcIm9yZGVyX2hhc2hcIjpcIitsZ0p0Q0pRZjU2bU9tZG5tbjlmM0N1KzVuYzlSNU1PZSt5cmVNVkdhQ1k9XCIsXCJleHBpcmF0aW9uX2Jsb2NrXCI6XCIwXCJ9IiwiaW5kZXgiOnRydWV9LHsia2V5IjoibWFya2V0X2lkIiwidmFsdWUiOiJcIjB4ZWYwZGQ2MzNkYTUyY2ZjMjFkYjg2NmIwNTQ2M2M4YjQzYzAyMjY1Yjg2YmY5MjU3YTVkMzk4NDhjZDJlZWExMVwiIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibWFya2V0X29yZGVyX2NhbmNlbCIsInZhbHVlIjoibnVsbCIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImF1dGh6X21zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6ImluamVjdGl2ZS5leGNoYW5nZS52MWJldGExLkV2ZW50Q2FuY2VsRGVyaXZhdGl2ZU9yZGVyIiwiYXR0cmlidXRlcyI6W3sia2V5IjoiaXNMaW1pdENhbmNlbCIsInZhbHVlIjoidHJ1ZSIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImxpbWl0X29yZGVyIiwidmFsdWUiOiJ7XCJvcmRlcl9pbmZvXCI6e1wic3ViYWNjb3VudF9pZFwiOlwiMHg5MzViNzA1YjU5NDNiMGUxYjg1OTY1MWQyY2NmMzc1YmM5NmRmZGQ3MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDI3XCIsXCJmZWVfcmVjaXBpZW50XCI6XCJpbmoxN3FqNGZrNTR6NjNnc3czdmRxNHF3eXo0MHk5dWU1azlhbmZ1eGhcIixcInByaWNlXCI6XCI0OTY0MjAwMDAuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJxdWFudGl0eVwiOlwiMjIuNTU1MDAwMDAwMDAwMDAwMDAwXCIsXCJjaWRcIjpcIlwifSxcIm9yZGVyX3R5cGVcIjpcIkJVWVwiLFwibWFyZ2luXCI6XCIyNzk5MDAwMDAwLjAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmlsbGFibGVcIjpcIjIyLjU1NTAwMDAwMDAwMDAwMDAwMFwiLFwidHJpZ2dlcl9wcmljZVwiOlwiMC4wMDAwMDAwMDAwMDAwMDAwMDBcIixcIm9yZGVyX2hhc2hcIjpcIitsZ0p0Q0pRZjU2bU9tZG5tbjlmM0N1KzVuYzlSNU1PZSt5cmVNVkdhQ1k9XCJ9IiwiaW5kZXgiOnRydWV9LHsia2V5IjoibWFya2V0X2lkIiwidmFsdWUiOiJcIjB4ZWYwZGQ2MzNkYTUyY2ZjMjFkYjg2NmIwNTQ2M2M4YjQzYzAyMjY1Yjg2YmY5MjU3YTVkMzk4NDhjZDJlZWExMVwiIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibWFya2V0X29yZGVyX2NhbmNlbCIsInZhbHVlIjoibnVsbCIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImF1dGh6X21zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6ImluamVjdGl2ZS5leGNoYW5nZS52Mi5FdmVudENhbmNlbERlcml2YXRpdmVPcmRlciIsImF0dHJpYnV0ZXMiOlt7ImtleSI6ImlzTGltaXRDYW5jZWwiLCJ2YWx1ZSI6InRydWUiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJsaW1pdF9vcmRlciIsInZhbHVlIjoie1wib3JkZXJfaW5mb1wiOntcInN1YmFjY291bnRfaWRcIjpcIjB4OTM1YjcwNWI1OTQzYjBlMWI4NTk2NTFkMmNjZjM3NWJjOTZkZmRkNzAwMDAwMDAwMDAwMDAwMDAwMDAwMDAyN1wiLFwiZmVlX3JlY2lwaWVudFwiOlwiaW5qMTdxajRmazU0ejYzZ3N3M3ZkcTRxd3l6NDB5OXVlNWs5YW5mdXhoXCIsXCJwcmljZVwiOlwiNTAwLjczMDAwMDAwMDAwMDAwMDAwMFwiLFwicXVhbnRpdHlcIjpcIjIyLjU1NTAwMDAwMDAwMDAwMDAwMFwiLFwiY2lkXCI6XCJcIn0sXCJvcmRlcl90eXBlXCI6XCJTRUxMXCIsXCJtYXJnaW5cIjpcIjI4MjMuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJmaWxsYWJsZVwiOlwiMjIuNTU1MDAwMDAwMDAwMDAwMDAwXCIsXCJ0cmlnZ2VyX3ByaWNlXCI6XCIwLjAwMDAwMDAwMDAwMDAwMDAwMFwiLFwib3JkZXJfaGFzaFwiOlwiVXRMWGxtYXpjZkpKejkwZS9zakl4VGRTa2dkV3lhOW9WRGpBeWNJNE01dz1cIixcImV4cGlyYXRpb25fYmxvY2tcIjpcIjBcIn0iLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtYXJrZXRfaWQiLCJ2YWx1ZSI6IlwiMHhlZjBkZDYzM2RhNTJjZmMyMWRiODY2YjA1NDYzYzhiNDNjMDIyNjViODZiZjkyNTdhNWQzOTg0OGNkMmVlYTExXCIiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtYXJrZXRfb3JkZXJfY2FuY2VsIiwidmFsdWUiOiJudWxsIiwiaW5kZXgiOnRydWV9LHsia2V5IjoiYXV0aHpfbXNnX2luZGV4IiwidmFsdWUiOiIwIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibXNnX2luZGV4IiwidmFsdWUiOiIwIiwiaW5kZXgiOnRydWV9XX0seyJ0eXBlIjoiaW5qZWN0aXZlLmV4Y2hhbmdlLnYxYmV0YTEuRXZlbnRDYW5jZWxEZXJpdmF0aXZlT3JkZXIiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJpc0xpbWl0Q2FuY2VsIiwidmFsdWUiOiJ0cnVlIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibGltaXRfb3JkZXIiLCJ2YWx1ZSI6IntcIm9yZGVyX2luZm9cIjp7XCJzdWJhY2NvdW50X2lkXCI6XCIweDkzNWI3MDViNTk0M2IwZTFiODU5NjUxZDJjY2YzNzViYzk2ZGZkZDcwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMjdcIixcImZlZV9yZWNpcGllbnRcIjpcImluajE3cWo0Zms1NHo2M2dzdzN2ZHE0cXd5ejQweTl1ZTVrOWFuZnV4aFwiLFwicHJpY2VcIjpcIjUwMDczMDAwMC4wMDAwMDAwMDAwMDAwMDAwMDBcIixcInF1YW50aXR5XCI6XCIyMi41NTUwMDAwMDAwMDAwMDAwMDBcIixcImNpZFwiOlwiXCJ9LFwib3JkZXJfdHlwZVwiOlwiU0VMTFwiLFwibWFyZ2luXCI6XCIyODIzMDAwMDAwLjAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmlsbGFibGVcIjpcIjIyLjU1NTAwMDAwMDAwMDAwMDAwMFwiLFwidHJpZ2dlcl9wcmljZVwiOlwiMC4wMDAwMDAwMDAwMDAwMDAwMDBcIixcIm9yZGVyX2hhc2hcIjpcIlV0TFhsbWF6Y2ZKSno5MGUvc2pJeFRkU2tnZFd5YTlvVkRqQXljSTRNNXc9XCJ9IiwiaW5kZXgiOnRydWV9LHsia2V5IjoibWFya2V0X2lkIiwidmFsdWUiOiJcIjB4ZWYwZGQ2MzNkYTUyY2ZjMjFkYjg2NmIwNTQ2M2M4YjQzYzAyMjY1Yjg2YmY5MjU3YTVkMzk4NDhjZDJlZWExMVwiIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibWFya2V0X29yZGVyX2NhbmNlbCIsInZhbHVlIjoibnVsbCIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImF1dGh6X21zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19XX1d\",\"signatures\":[{\"pubkey\":\"039b40a63c5169055893f22c18262c2f4a702270876ba1eda821bac9dbecf4cdf6\",\"address\":\"inj17qj4fk54z63gsw3vdq4qwyz40y9ue5k9anfuxh\",\"sequence\":1376431,\"signature\":\"Q9f+rfKwEAG5scRFpnITN9x40C+ayKP/fV2fjFCugQRjxxYoR+0pIUgtQcXPVo/vQ4WBdcCWAtZeFBPb5CqRNQ==\"}],\"block_unix_timestamp\":1763032074424}",
            "{\"block_number\":141431945,\"block_timestamp\":\"2025-11-13 11:07:54.424 +0000 UTC\",\"hash\":\"0xef083021b0a79aa5ce4107a5e2b314ee87e812c3a6ba7c3d4c95d95752c47967\",\"messages\":\"W3sidHlwZSI6Ii9pbmplY3RpdmUuZXhjaGFuZ2UudjFiZXRhMS5Nc2dCYXRjaFVwZGF0ZU9yZGVycyIsInZhbHVlIjp7InR5cGUiOiIvaW5qZWN0aXZlLmV4Y2hhbmdlLnYxYmV0YTEuTXNnQmF0Y2hVcGRhdGVPcmRlcnMiLCJ2YWx1ZSI6eyIkYmluYXJ5Ijp7ImJhc2U2NCI6IkNpcHBibW94YUc1NU9ISnJORFExTlhKcmVXNXFORFJzTkRKcVpITnpOWE5xZG1jemEzWjJOVGgwWkdneW93RUtRakI0TlRSa05EVXdOV0ZrWldZMllUVmpaV1l5Tm1Kak5EQXpZVE16WkRVNU5UWXlNR1JsWkRSbE1UVmlPV1V5WW1NelpHUTBPRGxpTnpFME9ERXpNelkyWVJKQ01IaGlZMk00TnpGa1lXSTFZVFV3TnpZeU5HVTFOV0ZtWldGaE9UTTJNVEJoTkRJMFl6UTBObU5qTURBd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXdJQUVxRnpJMU1URXhNMmhsYkdsNE4ySTVaVGcyT0RVd01XWmsiLCJzdWJUeXBlIjoiMDAifX19fV0=\",\"tx_number\":2650610890,\"tx_msg_types\":\"WyIvaW5qZWN0aXZlLmV4Y2hhbmdlLnYxYmV0YTEuTXNnQmF0Y2hVcGRhdGVPcmRlcnMiXQ==\",\"logs\":\"W3sibXNnX2luZGV4IjoiMCIsImV2ZW50cyI6W3sidHlwZSI6Im1lc3NhZ2UiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJhY3Rpb24iLCJ2YWx1ZSI6Ii9pbmplY3RpdmUuZXhjaGFuZ2UudjFiZXRhMS5Nc2dCYXRjaFVwZGF0ZU9yZGVycyIsImluZGV4Ijp0cnVlfSx7ImtleSI6InNlbmRlciIsInZhbHVlIjoiaW5qMWhueThyazQ0NTVya3luajQ0bDQyamRzczVzanZnM2t2djU4dGRoIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibW9kdWxlIiwidmFsdWUiOiJleGNoYW5nZSIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6ImNvaW5fc3BlbnQiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJzcGVuZGVyIiwidmFsdWUiOiJpbmoxNHZubXcyd2VlM3h0cnNxZnZwY3FnMzVqZzl2N2oydmRwengwa2siLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJhbW91bnQiLCJ2YWx1ZSI6Ijk5MDAwMDAwcGVnZ3kweGRBQzE3Rjk1OEQyZWU1MjNhMjIwNjIwNjk5NDU5N0MxM0Q4MzFlYzciLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJjb2luX3JlY2VpdmVkIiwiYXR0cmlidXRlcyI6W3sia2V5IjoicmVjZWl2ZXIiLCJ2YWx1ZSI6ImluajFobnk4cms0NDU1cmt5bmo0NGw0Mmpkc3M1c2p2ZzNrdnY1OHRkaCIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImFtb3VudCIsInZhbHVlIjoiOTkwMDAwMDBwZWdneTB4ZEFDMTdGOTU4RDJlZTUyM2EyMjA2MjA2OTk0NTk3QzEzRDgzMWVjNyIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6InRyYW5zZmVyIiwiYXR0cmlidXRlcyI6W3sia2V5IjoicmVjaXBpZW50IiwidmFsdWUiOiJpbmoxaG55OHJrNDQ1NXJreW5qNDRsNDJqZHNzNXNqdmcza3Z2NTh0ZGgiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJzZW5kZXIiLCJ2YWx1ZSI6ImluajE0dm5tdzJ3ZWUzeHRyc3FmdnBjcWczNWpnOXY3ajJ2ZHB6eDBrayIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImFtb3VudCIsInZhbHVlIjoiOTkwMDAwMDBwZWdneTB4ZEFDMTdGOTU4RDJlZTUyM2EyMjA2MjA2OTk0NTk3QzEzRDgzMWVjNyIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6Im1lc3NhZ2UiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJzZW5kZXIiLCJ2YWx1ZSI6ImluajE0dm5tdzJ3ZWUzeHRyc3FmdnBjcWczNWpnOXY3ajJ2ZHB6eDBrayIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6ImluamVjdGl2ZS5leGNoYW5nZS52Mi5FdmVudENhbmNlbERlcml2YXRpdmVPcmRlciIsImF0dHJpYnV0ZXMiOlt7ImtleSI6ImlzTGltaXRDYW5jZWwiLCJ2YWx1ZSI6InRydWUiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJsaW1pdF9vcmRlciIsInZhbHVlIjoie1wib3JkZXJfaW5mb1wiOntcInN1YmFjY291bnRfaWRcIjpcIjB4YmNjODcxZGFiNWE1MDc2MjRlNTVhZmVhYTkzNjEwYTQyNGM0NDZjYzAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmVlX3JlY2lwaWVudFwiOlwiaW5qMWhueThyazQ0NTVya3luajQ0bDQyamRzczVzanZnM2t2djU4dGRoXCIsXCJwcmljZVwiOlwiMzUxNS40MDAwMDAwMDAwMDAwMDAwMDBcIixcInF1YW50aXR5XCI6XCIwLjI4MDAwMDAwMDAwMDAwMDAwMFwiLFwiY2lkXCI6XCIyNTExMTNoZWxpeDdiOWU4Njg1MDFmZFwifSxcIm9yZGVyX3R5cGVcIjpcIlNFTExfUE9cIixcIm1hcmdpblwiOlwiOTkuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJmaWxsYWJsZVwiOlwiMC4yODAwMDAwMDAwMDAwMDAwMDBcIixcInRyaWdnZXJfcHJpY2VcIjpcIjAuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJvcmRlcl9oYXNoXCI6XCI1SEtwdUpsclQwcDJ5dDdIK2kxd2haZ1BjZHMrWVpWcjBuTHgyamNUbkZ3PVwiLFwiZXhwaXJhdGlvbl9ibG9ja1wiOlwiMFwifSIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1hcmtldF9pZCIsInZhbHVlIjoiXCIweDU0ZDQ1MDVhZGVmNmE1Y2VmMjZiYzQwM2EzM2Q1OTU2MjBkZWQ0ZTE1YjllMmJjM2RkNDg5YjcxNDgxMzM2NmFcIiIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1hcmtldF9vcmRlcl9jYW5jZWwiLCJ2YWx1ZSI6Im51bGwiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJpbmplY3RpdmUuZXhjaGFuZ2UudjFiZXRhMS5FdmVudENhbmNlbERlcml2YXRpdmVPcmRlciIsImF0dHJpYnV0ZXMiOlt7ImtleSI6ImlzTGltaXRDYW5jZWwiLCJ2YWx1ZSI6InRydWUiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJsaW1pdF9vcmRlciIsInZhbHVlIjoie1wib3JkZXJfaW5mb1wiOntcInN1YmFjY291bnRfaWRcIjpcIjB4YmNjODcxZGFiNWE1MDc2MjRlNTVhZmVhYTkzNjEwYTQyNGM0NDZjYzAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmVlX3JlY2lwaWVudFwiOlwiaW5qMWhueThyazQ0NTVya3luajQ0bDQyamRzczVzanZnM2t2djU4dGRoXCIsXCJwcmljZVwiOlwiMzUxNTQwMDAwMC4wMDAwMDAwMDAwMDAwMDAwMDBcIixcInF1YW50aXR5XCI6XCIwLjI4MDAwMDAwMDAwMDAwMDAwMFwiLFwiY2lkXCI6XCIyNTExMTNoZWxpeDdiOWU4Njg1MDFmZFwifSxcIm9yZGVyX3R5cGVcIjpcIlNFTExfUE9cIixcIm1hcmdpblwiOlwiOTkwMDAwMDAuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJmaWxsYWJsZVwiOlwiMC4yODAwMDAwMDAwMDAwMDAwMDBcIixcInRyaWdnZXJfcHJpY2VcIjpcIjAuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJvcmRlcl9oYXNoXCI6XCI1SEtwdUpsclQwcDJ5dDdIK2kxd2haZ1BjZHMrWVpWcjBuTHgyamNUbkZ3PVwifSIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1hcmtldF9pZCIsInZhbHVlIjoiXCIweDU0ZDQ1MDVhZGVmNmE1Y2VmMjZiYzQwM2EzM2Q1OTU2MjBkZWQ0ZTE1YjllMmJjM2RkNDg5YjcxNDgxMzM2NmFcIiIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1hcmtldF9vcmRlcl9jYW5jZWwiLCJ2YWx1ZSI6Im51bGwiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfV19XQ==\",\"signatures\":[{\"pubkey\":\"020752c76eac2b27b5d22c84199e81af48973d49a7a6fe0477c3c6e96129035de8\",\"address\":\"inj1hny8rk4455rkynj44l42jdss5sjvg3kvv58tdh\",\"sequence\":156838658,\"signature\":\"cZatHs/Vfkm+3crarBgkQSE6TrH7VhvDi7WgKsDiHyh3vyM01ZkS+sFPmbdiEh7bzYLc+HeKMSWKZO3MobEAlA==\"}],\"block_unix_timestamp\":1763032074424}",
            "{\"block_number\":141431945,\"block_timestamp\":\"2025-11-13 11:07:54.424 +0000 UTC\",\"hash\":\"0x768c4b5efa2c04484c714927ffe291f5e0c72b05e2c08f0c79d1643c466b1bc0\",\"messages\":\"W3sidHlwZSI6Ii9pbmplY3RpdmUuZXhjaGFuZ2UudjFiZXRhMS5Nc2dCYXRjaFVwZGF0ZU9yZGVycyIsInZhbHVlIjp7InR5cGUiOiIvaW5qZWN0aXZlLmV4Y2hhbmdlLnYxYmV0YTEuTXNnQmF0Y2hVcGRhdGVPcmRlcnMiLCJ2YWx1ZSI6eyIkYmluYXJ5Ijp7ImJhc2U2NCI6IkNpcHBibW94YUc1NU9ISnJORFExTlhKcmVXNXFORFJzTkRKcVpITnpOWE5xZG1jemEzWjJOVGgwWkdneW93RUtRakI0TlRSa05EVXdOV0ZrWldZMllUVmpaV1l5Tm1Kak5EQXpZVE16WkRVNU5UWXlNR1JsWkRSbE1UVmlPV1V5WW1NelpHUTBPRGxpTnpFME9ERXpNelkyWVJKQ01IaGlZMk00TnpGa1lXSTFZVFV3TnpZeU5HVTFOV0ZtWldGaE9UTTJNVEJoTkRJMFl6UTBObU5qTURBd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXdJQUVxRnpJMU1URXhNMmhsYkdsNFl6YzVZV00wTlRZM04ySTVNcU1CQ2tJd2VEVTBaRFExTURWaFpHVm1ObUUxWTJWbU1qWmlZelF3TTJFek0yUTFPVFUyTWpCa1pXUTBaVEUxWWpsbE1tSmpNMlJrTkRnNVlqY3hORGd4TXpNMk5tRVNRakI0WW1Oak9EY3haR0ZpTldFMU1EYzJNalJsTlRWaFptVmhZVGt6TmpFd1lUUXlOR00wTkRaall6QXdNREF3TURBd01EQXdNREF3TURBd01EQXdNREF3TUNBQktoY3lOVEV4TVROb1pXeHBlR015Wm1Zell6WTJPREEzWlVLakFncENNSGcxTkdRME5UQTFZV1JsWmpaaE5XTmxaakkyWW1NME1ETmhNek5rTlRrMU5qSXdaR1ZrTkdVeE5XSTVaVEppWXpOa1pEUTRPV0kzTVRRNE1UTXpOalpoRXJzQkNrSXdlR0pqWXpnM01XUmhZalZoTlRBM05qSTBaVFUxWVdabFlXRTVNell4TUdFME1qUmpORFEyWTJNd01EQXdNREF3TURBd01EQXdNREF3TURBd01EQXdNREFTS21sdWFqRm9ibms0Y21zME5EVTFjbXQ1Ym1vME5HdzBNbXBrYzNNMWMycDJaek5yZG5ZMU9IUmthQm9jTXpVeE5UUXdNREF3TURBd01EQXdNREF3TURBd01EQXdNREF3TUNJU01qZ3dNREF3TURBd01EQXdNREF3TURBd0toY3lOVEV4TVROb1pXeHBlRGRpT1dVNE5qZzFNREZtWkJnSUlobzVPVEF3TURBd01EQXdNREF3TURBd01EQXdNREF3TURBd01Db0JNQT09Iiwic3ViVHlwZSI6IjAwIn19fX1d\",\"tx_number\":2650610889,\"tx_msg_types\":\"WyIvaW5qZWN0aXZlLmV4Y2hhbmdlLnYxYmV0YTEuTXNnQmF0Y2hVcGRhdGVPcmRlcnMiXQ==\",\"logs\":\"W3sibXNnX2luZGV4IjoiMCIsImV2ZW50cyI6W3sidHlwZSI6Im1lc3NhZ2UiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJhY3Rpb24iLCJ2YWx1ZSI6Ii9pbmplY3RpdmUuZXhjaGFuZ2UudjFiZXRhMS5Nc2dCYXRjaFVwZGF0ZU9yZGVycyIsImluZGV4Ijp0cnVlfSx7ImtleSI6InNlbmRlciIsInZhbHVlIjoiaW5qMWhueThyazQ0NTVya3luajQ0bDQyamRzczVzanZnM2t2djU4dGRoIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibW9kdWxlIiwidmFsdWUiOiJleGNoYW5nZSIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6ImNvaW5fc3BlbnQiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJzcGVuZGVyIiwidmFsdWUiOiJpbmoxNHZubXcyd2VlM3h0cnNxZnZwY3FnMzVqZzl2N2oydmRwengwa2siLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJhbW91bnQiLCJ2YWx1ZSI6Ijk5MDAwMDAwcGVnZ3kweGRBQzE3Rjk1OEQyZWU1MjNhMjIwNjIwNjk5NDU5N0MxM0Q4MzFlYzciLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJjb2luX3JlY2VpdmVkIiwiYXR0cmlidXRlcyI6W3sia2V5IjoicmVjZWl2ZXIiLCJ2YWx1ZSI6ImluajFobnk4cms0NDU1cmt5bmo0NGw0Mmpkc3M1c2p2ZzNrdnY1OHRkaCIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImFtb3VudCIsInZhbHVlIjoiOTkwMDAwMDBwZWdneTB4ZEFDMTdGOTU4RDJlZTUyM2EyMjA2MjA2OTk0NTk3QzEzRDgzMWVjNyIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6InRyYW5zZmVyIiwiYXR0cmlidXRlcyI6W3sia2V5IjoicmVjaXBpZW50IiwidmFsdWUiOiJpbmoxaG55OHJrNDQ1NXJreW5qNDRsNDJqZHNzNXNqdmcza3Z2NTh0ZGgiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJzZW5kZXIiLCJ2YWx1ZSI6ImluajE0dm5tdzJ3ZWUzeHRyc3FmdnBjcWczNWpnOXY3ajJ2ZHB6eDBrayIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImFtb3VudCIsInZhbHVlIjoiOTkwMDAwMDBwZWdneTB4ZEFDMTdGOTU4RDJlZTUyM2EyMjA2MjA2OTk0NTk3QzEzRDgzMWVjNyIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6Im1lc3NhZ2UiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJzZW5kZXIiLCJ2YWx1ZSI6ImluajE0dm5tdzJ3ZWUzeHRyc3FmdnBjcWczNWpnOXY3ajJ2ZHB6eDBrayIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1zZ19pbmRleCIsInZhbHVlIjoiMCIsImluZGV4Ijp0cnVlfV19LHsidHlwZSI6ImluamVjdGl2ZS5leGNoYW5nZS52Mi5FdmVudENhbmNlbERlcml2YXRpdmVPcmRlciIsImF0dHJpYnV0ZXMiOlt7ImtleSI6ImlzTGltaXRDYW5jZWwiLCJ2YWx1ZSI6InRydWUiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJsaW1pdF9vcmRlciIsInZhbHVlIjoie1wib3JkZXJfaW5mb1wiOntcInN1YmFjY291bnRfaWRcIjpcIjB4YmNjODcxZGFiNWE1MDc2MjRlNTVhZmVhYTkzNjEwYTQyNGM0NDZjYzAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmVlX3JlY2lwaWVudFwiOlwiaW5qMWhueThyazQ0NTVya3luajQ0bDQyamRzczVzanZnM2t2djU4dGRoXCIsXCJwcmljZVwiOlwiMzUxNS40MDAwMDAwMDAwMDAwMDAwMDBcIixcInF1YW50aXR5XCI6XCIwLjI4MDAwMDAwMDAwMDAwMDAwMFwiLFwiY2lkXCI6XCIyNTExMTNoZWxpeGM3OWFjNDU2NzdiOVwifSxcIm9yZGVyX3R5cGVcIjpcIlNFTExfUE9cIixcIm1hcmdpblwiOlwiOTkuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJmaWxsYWJsZVwiOlwiMC4yODAwMDAwMDAwMDAwMDAwMDBcIixcInRyaWdnZXJfcHJpY2VcIjpcIjAuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJvcmRlcl9oYXNoXCI6XCJXcGhocDk0amtYZEF5bjJkSmM1NC9wajlnbnlUMC9LMHFUV3R1TjJPelVVPVwiLFwiZXhwaXJhdGlvbl9ibG9ja1wiOlwiMFwifSIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1hcmtldF9pZCIsInZhbHVlIjoiXCIweDU0ZDQ1MDVhZGVmNmE1Y2VmMjZiYzQwM2EzM2Q1OTU2MjBkZWQ0ZTE1YjllMmJjM2RkNDg5YjcxNDgxMzM2NmFcIiIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1hcmtldF9vcmRlcl9jYW5jZWwiLCJ2YWx1ZSI6Im51bGwiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJpbmplY3RpdmUuZXhjaGFuZ2UudjFiZXRhMS5FdmVudENhbmNlbERlcml2YXRpdmVPcmRlciIsImF0dHJpYnV0ZXMiOlt7ImtleSI6ImlzTGltaXRDYW5jZWwiLCJ2YWx1ZSI6InRydWUiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJsaW1pdF9vcmRlciIsInZhbHVlIjoie1wib3JkZXJfaW5mb1wiOntcInN1YmFjY291bnRfaWRcIjpcIjB4YmNjODcxZGFiNWE1MDc2MjRlNTVhZmVhYTkzNjEwYTQyNGM0NDZjYzAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmVlX3JlY2lwaWVudFwiOlwiaW5qMWhueThyazQ0NTVya3luajQ0bDQyamRzczVzanZnM2t2djU4dGRoXCIsXCJwcmljZVwiOlwiMzUxNTQwMDAwMC4wMDAwMDAwMDAwMDAwMDAwMDBcIixcInF1YW50aXR5XCI6XCIwLjI4MDAwMDAwMDAwMDAwMDAwMFwiLFwiY2lkXCI6XCIyNTExMTNoZWxpeGM3OWFjNDU2NzdiOVwifSxcIm9yZGVyX3R5cGVcIjpcIlNFTExfUE9cIixcIm1hcmdpblwiOlwiOTkwMDAwMDAuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJmaWxsYWJsZVwiOlwiMC4yODAwMDAwMDAwMDAwMDAwMDBcIixcInRyaWdnZXJfcHJpY2VcIjpcIjAuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJvcmRlcl9oYXNoXCI6XCJXcGhocDk0amtYZEF5bjJkSmM1NC9wajlnbnlUMC9LMHFUV3R1TjJPelVVPVwifSIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1hcmtldF9pZCIsInZhbHVlIjoiXCIweDU0ZDQ1MDVhZGVmNmE1Y2VmMjZiYzQwM2EzM2Q1OTU2MjBkZWQ0ZTE1YjllMmJjM2RkNDg5YjcxNDgxMzM2NmFcIiIsImluZGV4Ijp0cnVlfSx7ImtleSI6Im1hcmtldF9vcmRlcl9jYW5jZWwiLCJ2YWx1ZSI6Im51bGwiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJpbmplY3RpdmUuZXhjaGFuZ2UudjIuRXZlbnRPcmRlckNhbmNlbEZhaWwiLCJhdHRyaWJ1dGVzIjpbeyJrZXkiOiJjaWQiLCJ2YWx1ZSI6IlwiMjUxMTEzaGVsaXhjMmZmM2M2NjgwN2VcIiIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImRlc2NyaXB0aW9uIiwidmFsdWUiOiJcImludmFsaWQgb3JkZXIgaWRlbnRpZmllciBzdHJpbmc6IHN0cnVjdCBmaWVsZCBlcnJvclwiIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibWFya2V0X2lkIiwidmFsdWUiOiJcIjB4NTRkNDUwNWFkZWY2YTVjZWYyNmJjNDAzYTMzZDU5NTYyMGRlZDRlMTViOWUyYmMzZGQ0ODliNzE0ODEzMzY2YVwiIiwiaW5kZXgiOnRydWV9LHsia2V5Ijoib3JkZXJfaGFzaCIsInZhbHVlIjoiXCJcIiIsImluZGV4Ijp0cnVlfSx7ImtleSI6InN1YmFjY291bnRfaWQiLCJ2YWx1ZSI6IlwiMHhiY2M4NzFkYWI1YTUwNzYyNGU1NWFmZWFhOTM2MTBhNDI0YzQ0NmNjMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwXCIiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJpbmplY3RpdmUuZXhjaGFuZ2UudjFiZXRhMS5FdmVudE9yZGVyQ2FuY2VsRmFpbCIsImF0dHJpYnV0ZXMiOlt7ImtleSI6ImNpZCIsInZhbHVlIjoiXCJcIiIsImluZGV4Ijp0cnVlfSx7ImtleSI6ImRlc2NyaXB0aW9uIiwidmFsdWUiOiJcIlwiIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibWFya2V0X2lkIiwidmFsdWUiOiJcIjB4NTRkNDUwNWFkZWY2YTVjZWYyNmJjNDAzYTMzZDU5NTYyMGRlZDRlMTViOWUyYmMzZGQ0ODliNzE0ODEzMzY2YVwiIiwiaW5kZXgiOnRydWV9LHsia2V5Ijoib3JkZXJfaGFzaCIsInZhbHVlIjoiXCJcIiIsImluZGV4Ijp0cnVlfSx7ImtleSI6InN1YmFjY291bnRfaWQiLCJ2YWx1ZSI6IlwiXCIiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJjb2luX3NwZW50IiwiYXR0cmlidXRlcyI6W3sia2V5Ijoic3BlbmRlciIsInZhbHVlIjoiaW5qMWhueThyazQ0NTVya3luajQ0bDQyamRzczVzanZnM2t2djU4dGRoIiwiaW5kZXgiOnRydWV9LHsia2V5IjoiYW1vdW50IiwidmFsdWUiOiI5OTAwMDAwMHBlZ2d5MHhkQUMxN0Y5NThEMmVlNTIzYTIyMDYyMDY5OTQ1OTdDMTNEODMxZWM3IiwiaW5kZXgiOnRydWV9LHsia2V5IjoibXNnX2luZGV4IiwidmFsdWUiOiIwIiwiaW5kZXgiOnRydWV9XX0seyJ0eXBlIjoiY29pbl9yZWNlaXZlZCIsImF0dHJpYnV0ZXMiOlt7ImtleSI6InJlY2VpdmVyIiwidmFsdWUiOiJpbmoxNHZubXcyd2VlM3h0cnNxZnZwY3FnMzVqZzl2N2oydmRwengwa2siLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJhbW91bnQiLCJ2YWx1ZSI6Ijk5MDAwMDAwcGVnZ3kweGRBQzE3Rjk1OEQyZWU1MjNhMjIwNjIwNjk5NDU5N0MxM0Q4MzFlYzciLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJ0cmFuc2ZlciIsImF0dHJpYnV0ZXMiOlt7ImtleSI6InJlY2lwaWVudCIsInZhbHVlIjoiaW5qMTR2bm13MndlZTN4dHJzcWZ2cGNxZzM1amc5djdqMnZkcHp4MGtrIiwiaW5kZXgiOnRydWV9LHsia2V5Ijoic2VuZGVyIiwidmFsdWUiOiJpbmoxaG55OHJrNDQ1NXJreW5qNDRsNDJqZHNzNXNqdmcza3Z2NTh0ZGgiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJhbW91bnQiLCJ2YWx1ZSI6Ijk5MDAwMDAwcGVnZ3kweGRBQzE3Rjk1OEQyZWU1MjNhMjIwNjIwNjk5NDU5N0MxM0Q4MzFlYzciLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJtZXNzYWdlIiwiYXR0cmlidXRlcyI6W3sia2V5Ijoic2VuZGVyIiwidmFsdWUiOiJpbmoxaG55OHJrNDQ1NXJreW5qNDRsNDJqZHNzNXNqdmcza3Z2NTh0ZGgiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtc2dfaW5kZXgiLCJ2YWx1ZSI6IjAiLCJpbmRleCI6dHJ1ZX1dfSx7InR5cGUiOiJpbmplY3RpdmUuZXhjaGFuZ2UudjIuRXZlbnROZXdEZXJpdmF0aXZlT3JkZXJzIiwiYXR0cmlidXRlcyI6W3sia2V5IjoiYnV5X29yZGVycyIsInZhbHVlIjoiW10iLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtYXJrZXRfaWQiLCJ2YWx1ZSI6IlwiMHg1NGQ0NTA1YWRlZjZhNWNlZjI2YmM0MDNhMzNkNTk1NjIwZGVkNGUxNWI5ZTJiYzNkZDQ4OWI3MTQ4MTMzNjZhXCIiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJzZWxsX29yZGVycyIsInZhbHVlIjoiW3tcIm9yZGVyX2luZm9cIjp7XCJzdWJhY2NvdW50X2lkXCI6XCIweGJjYzg3MWRhYjVhNTA3NjI0ZTU1YWZlYWE5MzYxMGE0MjRjNDQ2Y2MwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDBcIixcImZlZV9yZWNpcGllbnRcIjpcImluajFobnk4cms0NDU1cmt5bmo0NGw0Mmpkc3M1c2p2ZzNrdnY1OHRkaFwiLFwicHJpY2VcIjpcIjM1MTUuNDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJxdWFudGl0eVwiOlwiMC4yODAwMDAwMDAwMDAwMDAwMDBcIixcImNpZFwiOlwiMjUxMTEzaGVsaXg3YjllODY4NTAxZmRcIn0sXCJvcmRlcl90eXBlXCI6XCJTRUxMX1BPXCIsXCJtYXJnaW5cIjpcIjk5LjAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmlsbGFibGVcIjpcIjAuMjgwMDAwMDAwMDAwMDAwMDAwXCIsXCJ0cmlnZ2VyX3ByaWNlXCI6XCIwLjAwMDAwMDAwMDAwMDAwMDAwMFwiLFwib3JkZXJfaGFzaFwiOlwiNUhLcHVKbHJUMHAyeXQ3SCtpMXdoWmdQY2RzK1laVnIwbkx4MmpjVG5Gdz1cIixcImV4cGlyYXRpb25fYmxvY2tcIjpcIjBcIn1dIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibXNnX2luZGV4IiwidmFsdWUiOiIwIiwiaW5kZXgiOnRydWV9XX0seyJ0eXBlIjoiaW5qZWN0aXZlLmV4Y2hhbmdlLnYxYmV0YTEuRXZlbnROZXdEZXJpdmF0aXZlT3JkZXJzIiwiYXR0cmlidXRlcyI6W3sia2V5IjoiYnV5X29yZGVycyIsInZhbHVlIjoiW10iLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJtYXJrZXRfaWQiLCJ2YWx1ZSI6IlwiMHg1NGQ0NTA1YWRlZjZhNWNlZjI2YmM0MDNhMzNkNTk1NjIwZGVkNGUxNWI5ZTJiYzNkZDQ4OWI3MTQ4MTMzNjZhXCIiLCJpbmRleCI6dHJ1ZX0seyJrZXkiOiJzZWxsX29yZGVycyIsInZhbHVlIjoiW3tcIm9yZGVyX2luZm9cIjp7XCJzdWJhY2NvdW50X2lkXCI6XCIweGJjYzg3MWRhYjVhNTA3NjI0ZTU1YWZlYWE5MzYxMGE0MjRjNDQ2Y2MwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDBcIixcImZlZV9yZWNpcGllbnRcIjpcImluajFobnk4cms0NDU1cmt5bmo0NGw0Mmpkc3M1c2p2ZzNrdnY1OHRkaFwiLFwicHJpY2VcIjpcIjM1MTU0MDAwMDAuMDAwMDAwMDAwMDAwMDAwMDAwXCIsXCJxdWFudGl0eVwiOlwiMC4yODAwMDAwMDAwMDAwMDAwMDBcIixcImNpZFwiOlwiMjUxMTEzaGVsaXg3YjllODY4NTAxZmRcIn0sXCJvcmRlcl90eXBlXCI6XCJTRUxMX1BPXCIsXCJtYXJnaW5cIjpcIjk5MDAwMDAwLjAwMDAwMDAwMDAwMDAwMDAwMFwiLFwiZmlsbGFibGVcIjpcIjAuMjgwMDAwMDAwMDAwMDAwMDAwXCIsXCJ0cmlnZ2VyX3ByaWNlXCI6XCIwLjAwMDAwMDAwMDAwMDAwMDAwMFwiLFwib3JkZXJfaGFzaFwiOlwiNUhLcHVKbHJUMHAyeXQ3SCtpMXdoWmdQY2RzK1laVnIwbkx4MmpjVG5Gdz1cIn1dIiwiaW5kZXgiOnRydWV9LHsia2V5IjoibXNnX2luZGV4IiwidmFsdWUiOiIwIiwiaW5kZXgiOnRydWV9XX1dfV0=\",\"signatures\":[{\"pubkey\":\"020752c76eac2b27b5d22c84199e81af48973d49a7a6fe0477c3c6e96129035de8\",\"address\":\"inj1hny8rk4455rkynj44l42jdss5sjvg3kvv58tdh\",\"sequence\":156838657,\"signature\":\"zUlh3t8I+dTFJedJheG/qt/fWYD1IPUmavdTxYMAIlZwwL0BsfYu0OIqHD981g7Gy7srpB36uDProDystKWiTw==\"}],\"block_unix_timestamp\":1763032074424}"
        ]
    }
}
```

#### Get Latest Block Height
```http
GET /demo/block/latest-height
```

Returns the height of the latest block.

**Response**:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "height": 141142400
    }
}
```

**Note**: The response uses the `LatestBlockHeightResponse` proto message type for type safety.

### Transaction APIs

#### Get Latest Transactions
```http
GET /demo/transaction/latest?page=1&page_size=5
```

**Query Parameters**:
- `page` (int32, optional): Page number (default: 1, min: 1)
- `page_size` (int32, optional): Page size (default: 20, min: 1, max: 100)
- `cursor` (string, optional): Cursor for cursor-based pagination (takes priority over page)

Returns the latest transactions from the blockchain with pagination support. The service uses a database-first strategy for improved performance.

**Response**:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "data": [
            {
                "block_number": 141512064,
                "block_unix_timestamp": 1763084056386,
                "error_log": "",
                "hash": "0x75664123c37c239d9d5c14d476b2e16a3ed73b29caad8251f7425fe82d9f8dbf",
                "messages": "[{\"type\":\"/cosmos.distribution.v1beta1.MsgSetWithdrawAddress\",\"value\":{\"delegator_address\":\"inj1s7kvd0ql49xt37nx2y4f90zy3hld5t6xpmdxhe\",\"withdraw_address\":\"inj1s7kvd0ql49xt37nx2y4f90zy3hld5t6xpmdxhe\"}},{\"type\":\"/cosmos.authz.v1beta1.MsgExec\",\"value\":{\"grantee\":\"inj1s7kvd0ql49xt37nx2y4f90zy3hld5t6xpmdxhe\",\"msgs\":[{\"@type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"sender\":\"inj18cajq5agkjfgwp3dlsek3w8vuzawqu3uaztpen\",\"subaccount_id\":\"\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[{\"market_id\":\"0x9b9980167ecc3645ff1a5517886652d94a0825e54a77d2057cbbe3ebee015963\",\"subaccount_id\":\"0x3e3b2053a8b49287062dfc3368b8ece0bae0723c000000000000000000000003\",\"order_hash\":\"\",\"order_mask\":0,\"cid\":\"#00000006200dc438e54a65d80000bfce\"}],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}]}}]",
                "payer": "inj1s7kvd0ql49xt37nx2y4f90zy3hld5t6xpmdxhe"
            },
            {
                "block_number": 141512064,
                "block_unix_timestamp": 1763084056386,
                "error_log": "",
                "hash": "0xd021ba51359aa8bd3611cce3128fdf5938a081be54ab1394184627311375f46a",
                "messages": "[{\"type\":\"/cosmwasm.wasm.v1.MsgExecuteContract\",\"value\":{\"sender\":\"inj1s9hhrdgzerf79w963gv6kez4hvp3jds5avsc6e\",\"contract\":\"inj1zen7yh00gffkvsxcptqzaslu27msrw0ee933vf\",\"msg\":{\"execute_orders_multiple\":{\"subaccounts\":[\"0xe148ccd3d2a676075a5f26dbd04deec1e031ad430000006b756a692d75736474\",\"0xf59acebba0d19094ef532db7e6a8f7d844bc02fc0000006b756a692d75736474\"]}},\"funds\":[]}}]",
                "payer": "inj1s9hhrdgzerf79w963gv6kez4hvp3jds5avsc6e"
            },
            {
                "block_number": 141512064,
                "block_unix_timestamp": 1763084056386,
                "error_log": "",
                "hash": "0x656d1f3afde84de64a8b97287354418954e9d4de276aa78b82a4c49dd06a8f74",
                "messages": "[{\"type\":\"/cosmos.distribution.v1beta1.MsgSetWithdrawAddress\",\"value\":{\"delegator_address\":\"inj1s9sparu5ksj9f2n36ltnfgh8zm0p028a4xrgpc\",\"withdraw_address\":\"inj1s9sparu5ksj9f2n36ltnfgh8zm0p028a4xrgpc\"}},{\"type\":\"/cosmos.authz.v1beta1.MsgExec\",\"value\":{\"grantee\":\"inj1s9sparu5ksj9f2n36ltnfgh8zm0p028a4xrgpc\",\"msgs\":[{\"@type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"sender\":\"inj18cajq5agkjfgwp3dlsek3w8vuzawqu3uaztpen\",\"subaccount_id\":\"\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[{\"market_id\":\"0x9b9980167ecc3645ff1a5517886652d94a0825e54a77d2057cbbe3ebee015963\",\"subaccount_id\":\"0x3e3b2053a8b49287062dfc3368b8ece0bae0723c000000000000000000000003\",\"order_hash\":\"\",\"order_mask\":0,\"cid\":\"#00000006200d5cd1ee8cd8520000bf8c\"}],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}]}}]",
                "payer": "inj1s9sparu5ksj9f2n36ltnfgh8zm0p028a4xrgpc"
            },
            {
                "block_number": 141512064,
                "block_unix_timestamp": 1763084056386,
                "error_log": "",
                "hash": "0xc518ba6cbfd787bbd8f5a7030f6629ebc53d9690ef794fa889d97b40bab8d05f",
                "messages": "[{\"type\":\"/cosmos.authz.v1beta1.MsgExec\",\"value\":{\"grantee\":\"inj1xzk3uyxk7n9n4m5hczvdmmc34p8cwfcpst7nju\",\"msgs\":[{\"@type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"sender\":\"inj18cajq5agkjfgwp3dlsek3w8vuzawqu3uaztpen\",\"subaccount_id\":\"\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[{\"market_id\":\"0x9b9980167ecc3645ff1a5517886652d94a0825e54a77d2057cbbe3ebee015963\",\"subaccount_id\":\"0x3e3b2053a8b49287062dfc3368b8ece0bae0723c000000000000000000000003\",\"order_hash\":\"\",\"order_mask\":0,\"cid\":\"#00000006200d5cd1ee8cd8520000bf8c\"}],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[{\"market_id\":\"0x9b9980167ecc3645ff1a5517886652d94a0825e54a77d2057cbbe3ebee015963\",\"order_info\":{\"subaccount_id\":\"0x3e3b2053a8b49287062dfc3368b8ece0bae0723c000000000000000000000003\",\"fee_recipient\":\"inj18cajq5agkjfgwp3dlsek3w8vuzawqu3uaztpen\",\"price\":\"7002000.000000000000000000\",\"quantity\":\"1428.571000000000000000\",\"cid\":\"#00000006200dc438e54a65d80000bfce\"},\"order_type\":\"BUY_PO\",\"margin\":\"2000570828.399999982138979249\",\"trigger_price\":null}],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}]}}]",
                "payer": "inj1xzk3uyxk7n9n4m5hczvdmmc34p8cwfcpst7nju"
            },
            {
                "block_number": 141512064,
                "block_unix_timestamp": 1763084056386,
                "error_log": "",
                "hash": "0xb447c86a67a35b5b6c4ede728d8ceedab7b24664fd2e812db97fba23563e55aa",
                "messages": "[{\"type\":\"/cosmos.authz.v1beta1.MsgExec\",\"value\":{\"grantee\":\"inj14sph79aemnd0jsmgx0kpfl2tsutjfjaxwjf5x4\",\"msgs\":[{\"@type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"sender\":\"inj14k7wgey85pt226ugr9sq4h2ya774n82hzy26nf\",\"subaccount_id\":\"0xadbce46487a056a56b8819600add44efbd599d57000000000000000000000001\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[\"0xf12a831d2901a1c1130fb81c88c3b443c62be3cd14547625f2db8a5f2b187d2c\"],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[{\"market_id\":\"0xf12a831d2901a1c1130fb81c88c3b443c62be3cd14547625f2db8a5f2b187d2c\",\"order_info\":{\"subaccount_id\":\"0xadbce46487a056a56b8819600add44efbd599d57000000000000000000000001\",\"fee_recipient\":\"inj14k7wgey85pt226ugr9sq4h2ya774n82hzy26nf\",\"price\":\"579420000.000000000000000000\",\"quantity\":\"7.094000000000000000\",\"cid\":\"019a7fff-b558-755d-952c-8f69a049cac2\"},\"order_type\":\"SELL_PO\",\"margin\":\"411110000.000000000000000000\",\"trigger_price\":\"0.000000000000000000\"},{\"market_id\":\"0xf12a831d2901a1c1130fb81c88c3b443c62be3cd14547625f2db8a5f2b187d2c\",\"order_info\":{\"subaccount_id\":\"0xadbce46487a056a56b8819600add44efbd599d57000000000000000000000001\",\"fee_recipient\":\"inj14k7wgey85pt226ugr9sq4h2ya774n82hzy26nf\",\"price\":\"571360000.000000000000000000\",\"quantity\":\"7.194000000000000000\",\"cid\":\"019a7fff-b559-755d-952c-93fcf628730f\"},\"order_type\":\"BUY_PO\",\"margin\":\"411110000.000000000000000000\",\"trigger_price\":\"0.000000000000000000\"}],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}]}}]",
                "payer": "inj14sph79aemnd0jsmgx0kpfl2tsutjfjaxwjf5x4"
            }
        ],
        "pagination": {
            "has_next": true,
            "has_prev": false,
            "page": 1,
            "page_size": 5,
            "total": 2463,
            "total_pages": 493
        }
    }
}
```

#### Get Transaction by Hash
```http
GET /demo/transaction/by-hash?hash={hash}
```

Parameters:
- `hash` (string): Transaction hash

Returns transaction details for the specified hash.

**Response**:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "block_number": 141277444,
        "block_timestamp": "2025-11-12 07:41:06.809 +0000 UTC",
        "block_unix_timestamp": 1762933266809,
        "data": "EsUCCiUvY29zbW9zLmF1dGh6LnYxYmV0YTEuTXNnRXhlY1Jlc3BvbnNlEpsCCpgCIkIweGJmN2Q3NTJmZDE1NTU3N2ZlMDU0NTk5N2RmMDk5OGVjNDRiMGQyYTNhMzQwZTZiOWFiZDI5NjllNDkwYWNlNzUiQjB4YmJjZjVjOGYxOTBlZTdjZGVlZWU0MmQxYjVmZWJlOWU1OGJhN2NjYzA5NzgyNjVjOWVhODU4NTBkMDI2MDg0OSJCMHgzZGYyZjBlYWU3MzUxMmJiMGNhNjU1NTVmMmFjNjhhOTA4MjgwMTNhMDIwMzFjM2YxYjZmODc1MDA2MTA1ZDlmIkIweGVkMjJkMmViZGM4YjM2NjUyMDZmZmIxOGYwNmFmZDNkMjFiNTI1MDQ3ZmU4YjNlN2M4NjJkZjdlOWExMDAxYjNKAEoASgBKAA==",
        "error_log": "",
        "gas_fee": "{\"amount\":[{\"denom\":\"inj\",\"amount\":\"76000000000000\"}],\"gas_limit\":475000,\"payer\":\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\"}",
        "gas_used": 430964,
        "gas_wanted": 475000,
        "hash": "0x724770374909822d4f3802ef1b3ea287232f8aa92e588334bc24c6dc7c376706",
        "logs": "[{\"msg_index\":\"0\",\"events\":[{\"type\":\"message\",\"attributes\":[{\"key\":\"action\",\"value\":\"/cosmos.authz.v1beta1.MsgExec\",\"index\":true},{\"key\":\"sender\",\"value\":\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\",\"index\":true},{\"key\":\"module\",\"value\":\"authz\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v2.EventCancelDerivativeOrder\",\"attributes\":[{\"key\":\"isLimitCancel\",\"value\":\"true\",\"index\":true},{\"key\":\"limit_order\",\"value\":\"{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4117.950000000000000000\\\",\\\"quantity\\\":\\\"54.576200000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"BUY_PO\\\",\\\"margin\\\":\\\"5619.000000000000000000\\\",\\\"fillable\\\":\\\"54.576200000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"64H3d6MS6tnZ2d2u7xy/1HYpHj7xIRu4u3tM4pLImjM=\\\",\\\"expiration_block\\\":\\\"0\\\"}\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"market_order_cancel\",\"value\":\"null\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v1beta1.EventCancelDerivativeOrder\",\"attributes\":[{\"key\":\"isLimitCancel\",\"value\":\"true\",\"index\":true},{\"key\":\"limit_order\",\"value\":\"{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4117950000.000000000000000000\\\",\\\"quantity\\\":\\\"54.576200000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"BUY_PO\\\",\\\"margin\\\":\\\"5619000000.000000000000000000\\\",\\\"fillable\\\":\\\"54.576200000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"64H3d6MS6tnZ2d2u7xy/1HYpHj7xIRu4u3tM4pLImjM=\\\"}\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"market_order_cancel\",\"value\":\"null\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v2.EventCancelDerivativeOrder\",\"attributes\":[{\"key\":\"isLimitCancel\",\"value\":\"true\",\"index\":true},{\"key\":\"limit_order\",\"value\":\"{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4117.960000000000000000\\\",\\\"quantity\\\":\\\"54.576200000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"SELL_PO\\\",\\\"margin\\\":\\\"5619.000000000000000000\\\",\\\"fillable\\\":\\\"54.576200000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"06IRlf0j+3AYtxWrZ5ZBwo/nUrEpdlzQG2egyXp9OnY=\\\",\\\"expiration_block\\\":\\\"0\\\"}\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"market_order_cancel\",\"value\":\"null\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v1beta1.EventCancelDerivativeOrder\",\"attributes\":[{\"key\":\"isLimitCancel\",\"value\":\"true\",\"index\":true},{\"key\":\"limit_order\",\"value\":\"{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4117960000.000000000000000000\\\",\\\"quantity\\\":\\\"54.576200000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"SELL_PO\\\",\\\"margin\\\":\\\"5619000000.000000000000000000\\\",\\\"fillable\\\":\\\"54.576200000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"06IRlf0j+3AYtxWrZ5ZBwo/nUrEpdlzQG2egyXp9OnY=\\\"}\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"market_order_cancel\",\"value\":\"null\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v2.EventCancelDerivativeOrder\",\"attributes\":[{\"key\":\"isLimitCancel\",\"value\":\"true\",\"index\":true},{\"key\":\"limit_order\",\"value\":\"{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4115.740000000000000000\\\",\\\"quantity\\\":\\\"3.898300000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"BUY\\\",\\\"margin\\\":\\\"401.000000000000000000\\\",\\\"fillable\\\":\\\"3.898300000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"uP3acUgeHuh0XdsNKVhrn3taJZFkOkEcx1v4xH/0hvE=\\\",\\\"expiration_block\\\":\\\"0\\\"}\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"market_order_cancel\",\"value\":\"null\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v1beta1.EventCancelDerivativeOrder\",\"attributes\":[{\"key\":\"isLimitCancel\",\"value\":\"true\",\"index\":true},{\"key\":\"limit_order\",\"value\":\"{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4115740000.000000000000000000\\\",\\\"quantity\\\":\\\"3.898300000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"BUY\\\",\\\"margin\\\":\\\"401000000.000000000000000000\\\",\\\"fillable\\\":\\\"3.898300000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"uP3acUgeHuh0XdsNKVhrn3taJZFkOkEcx1v4xH/0hvE=\\\"}\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"market_order_cancel\",\"value\":\"null\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v2.EventCancelDerivativeOrder\",\"attributes\":[{\"key\":\"isLimitCancel\",\"value\":\"true\",\"index\":true},{\"key\":\"limit_order\",\"value\":\"{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4120.160000000000000000\\\",\\\"quantity\\\":\\\"3.898300000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"SELL\\\",\\\"margin\\\":\\\"402.000000000000000000\\\",\\\"fillable\\\":\\\"3.898300000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"kmfW1dTpaxnG5Q4NNfiTFTuuTiZKo6MH211WKLQc+88=\\\",\\\"expiration_block\\\":\\\"0\\\"}\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"market_order_cancel\",\"value\":\"null\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v1beta1.EventCancelDerivativeOrder\",\"attributes\":[{\"key\":\"isLimitCancel\",\"value\":\"true\",\"index\":true},{\"key\":\"limit_order\",\"value\":\"{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4120160000.000000000000000000\\\",\\\"quantity\\\":\\\"3.898300000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"SELL\\\",\\\"margin\\\":\\\"402000000.000000000000000000\\\",\\\"fillable\\\":\\\"3.898300000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"kmfW1dTpaxnG5Q4NNfiTFTuuTiZKo6MH211WKLQc+88=\\\"}\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"market_order_cancel\",\"value\":\"null\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v2.EventNewDerivativeOrders\",\"attributes\":[{\"key\":\"buy_orders\",\"value\":\"[{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4117.940000000000000000\\\",\\\"quantity\\\":\\\"54.576300000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"BUY_PO\\\",\\\"margin\\\":\\\"5619.000000000000000000\\\",\\\"fillable\\\":\\\"54.576300000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"v311L9FVV3/gVFmX3wmY7ESw0qOjQOa5q9KWnkkKznU=\\\",\\\"expiration_block\\\":\\\"0\\\"}]\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"sell_orders\",\"value\":\"[]\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v1beta1.EventNewDerivativeOrders\",\"attributes\":[{\"key\":\"buy_orders\",\"value\":\"[{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4117940000.000000000000000000\\\",\\\"quantity\\\":\\\"54.576300000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"BUY_PO\\\",\\\"margin\\\":\\\"5619000000.000000000000000000\\\",\\\"fillable\\\":\\\"54.576300000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"v311L9FVV3/gVFmX3wmY7ESw0qOjQOa5q9KWnkkKznU=\\\"}]\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"sell_orders\",\"value\":\"[]\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v2.EventNewDerivativeOrders\",\"attributes\":[{\"key\":\"buy_orders\",\"value\":\"[]\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"sell_orders\",\"value\":\"[{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4117.950000000000000000\\\",\\\"quantity\\\":\\\"54.576300000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"SELL_PO\\\",\\\"margin\\\":\\\"5619.000000000000000000\\\",\\\"fillable\\\":\\\"54.576300000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"PfLw6uc1ErsMplVV8qxoqQgoAToCAxw/G2+HUAYQXZ8=\\\",\\\"expiration_block\\\":\\\"0\\\"}]\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]},{\"type\":\"injective.exchange.v1beta1.EventNewDerivativeOrders\",\"attributes\":[{\"key\":\"buy_orders\",\"value\":\"[]\",\"index\":true},{\"key\":\"market_id\",\"value\":\"\\\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\\\"\",\"index\":true},{\"key\":\"sell_orders\",\"value\":\"[{\\\"order_info\\\":{\\\"subaccount_id\\\":\\\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\\\",\\\"fee_recipient\\\":\\\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\\\",\\\"price\\\":\\\"4117950000.000000000000000000\\\",\\\"quantity\\\":\\\"54.576300000000000000\\\",\\\"cid\\\":\\\"\\\"},\\\"order_type\\\":\\\"SELL_PO\\\",\\\"margin\\\":\\\"5619000000.000000000000000000\\\",\\\"fillable\\\":\\\"54.576300000000000000\\\",\\\"trigger_price\\\":\\\"0.000000000000000000\\\",\\\"order_hash\\\":\\\"PfLw6uc1ErsMplVV8qxoqQgoAToCAxw/G2+HUAYQXZ8=\\\"}]\",\"index\":true},{\"key\":\"authz_msg_index\",\"value\":\"0\",\"index\":true},{\"key\":\"msg_index\",\"value\":\"0\",\"index\":true}]}]}]",
        "messages": "[{\"type\":\"/cosmos.authz.v1beta1.MsgExec\",\"value\":{\"grantee\":\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\",\"msgs\":[{\"@type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"sender\":\"inj1kgjmntg9zkz6lr4y3vpm5xwvm5c4csaz9t0w2c\",\"subaccount_id\":\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\"],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[{\"market_id\":\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\",\"order_info\":{\"subaccount_id\":\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\",\"fee_recipient\":\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\",\"price\":\"4117940000.000000000000000000\",\"quantity\":\"54.576300000000000000\",\"cid\":\"\"},\"order_type\":\"BUY_PO\",\"margin\":\"5619000000.000000000000000000\",\"trigger_price\":\"0.000000000000000000\"},{\"market_id\":\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\",\"order_info\":{\"subaccount_id\":\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\",\"fee_recipient\":\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\",\"price\":\"4115730000.000000000000000000\",\"quantity\":\"3.898300000000000000\",\"cid\":\"\"},\"order_type\":\"BUY\",\"margin\":\"401000000.000000000000000000\",\"trigger_price\":\"0.000000000000000000\"},{\"market_id\":\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\",\"order_info\":{\"subaccount_id\":\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\",\"fee_recipient\":\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\",\"price\":\"4117950000.000000000000000000\",\"quantity\":\"54.576300000000000000\",\"cid\":\"\"},\"order_type\":\"SELL_PO\",\"margin\":\"5619000000.000000000000000000\",\"trigger_price\":\"0.000000000000000000\"},{\"market_id\":\"0x0160a0c8ecbf5716465b9fc22bceeedf6e92dcdc688e823bbe1af3b22a84e5b5\",\"order_info\":{\"subaccount_id\":\"0xb225b9ad051585af8ea48b03ba19ccdd315c43a2000000000000000000000012\",\"fee_recipient\":\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\",\"price\":\"4120150000.000000000000000000\",\"quantity\":\"3.898300000000000000\",\"cid\":\"\"},\"order_type\":\"SELL\",\"margin\":\"402000000.000000000000000000\",\"trigger_price\":\"0.000000000000000000\"}],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}]}}]",
        "payer": "inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37",
        "signatures": [
            "{\"pubkey\":\"0270042a0c18ef5a03bb92f65cc7d9d5458f224bd51b3d8a0bd6c0754f7ae8cd93\",\"address\":\"inj1hy63tfvfyxmr4j4pxk02q5hd3uy2j3w76kgd37\",\"sequence\":3222977,\"signature\":\"jkB+MfDv9BqzTsI65bVYYh1qCWiuJDw6PlLpQRaMDEImzLsRjv5MInpV9tNzz4iylb/nTIA9j3NKb6sDALem9A==\"}"
        ],
        "tx_number": 2645266264,
        "tx_type": "injective"
    }
}
```

### Account APIs

#### Get Account Info
```http
GET /demo/account/info?address={address}
```

Parameters:
- `address` (string): Account address

Returns account information.

**Response**:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "address": "inj1lfycr67vur5gayrakw6xj06sx86f4zf7qex26d",
        "base_account": "{\"address\":\"inj1lfycr67vur5gayrakw6xj06sx86f4zf7qex26d\",\"pub_key\":\"{\\\"type\\\":\\\"eth_secp256k1\\\",\\\"value\\\":\\\"02e02897801ddf2e94a815fc9b69122d0bd521abe4459c1431d7698010a8c13bf3\\\",\\\"evm_address\\\":\\\"0xfa4981ebcce0e88e907db3b4693f5031f49a893e\\\"}\",\"account_number\":1191995,\"sequence\":96378280}",
        "code_hash": "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
    }
}
```

#### Get Account Transactions
```http
GET /demo/account/transactions?address={address}&pagination.page=1&pagination.page_size=20
```

**Query Parameters**:
- `address` (string, required): Account address
- `pagination.page` (int32, optional): Page number (default: 1, min: 1)
- `pagination.page_size` (int32, optional): Page size (default: 20, min: 1, max: 100)
- `pagination.cursor` (string, optional): Cursor for cursor-based pagination (takes priority over page)

Returns transactions associated with the account with pagination support. The service uses a database-first strategy for improved performance.

**Response**:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "data": [
            {
                "block_number": 141477796,
                "block_unix_timestamp": 1763062378197,
                "error_log": "",
                "hash": "0x8edc21fd00721bb06c77ed7187fe85c62690cc866afab51e50ced82faf9edf8d",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgIncreasePositionMargin\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"source_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000013\",\"destination_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000005\",\"market_id\":\"0xb7b7946c5152b045c453769a0b442b8a42415ed9bb95ba5f59203bf40c5f2f03\",\"amount\":\"56270000.000000000000000000\"}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141477796,
                "block_unix_timestamp": 1763062378197,
                "error_log": "",
                "hash": "0xc4a7ecb66936c3f8acf35962e50d73a7b91b40dc2137f797b8a75c1137b22cd8",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000013\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[\"0xb7b7946c5152b045c453769a0b442b8a42415ed9bb95ba5f59203bf40c5f2f03\"],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141468934,
                "block_unix_timestamp": 1763056286156,
                "error_log": "",
                "hash": "0x4c8a925075dfc5cfe12e38661bc7c21e1e5da73f2d5c32288d30a8bf8ab1719a",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgIncreasePositionMargin\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"source_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000015\",\"destination_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd700000000000000000000000c\",\"market_id\":\"0x30a1463cfb4c393c80e257ab93118cecd73c1e632dc4d2d31c12a51bc0a70bd7\",\"amount\":\"142920000.000000000000000000\"}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141468932,
                "block_unix_timestamp": 1763056284285,
                "error_log": "",
                "hash": "0x09b4c2a98306c60ca0208f71d58d05e99bf0b8942b5b355a5acb1f338f9f3682",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000015\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[\"0x30a1463cfb4c393c80e257ab93118cecd73c1e632dc4d2d31c12a51bc0a70bd7\"],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141464451,
                "block_unix_timestamp": 1763053308789,
                "error_log": "",
                "hash": "0x8d677dc4e46b94f385c324613861b0f044bf17956c6f142f9de82b496ac3fa2f",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgIncreasePositionMargin\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"source_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000013\",\"destination_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd700000000000000000000000f\",\"market_id\":\"0xa51b0df7a1ac2bea408fdc4f84f3d7060b62f976ab659b329b00065e5c8f2c37\",\"amount\":\"516580000.000000000000000000\"}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141464447,
                "block_unix_timestamp": 1763053306654,
                "error_log": "",
                "hash": "0xb51ae3679de1a561cb3fddd069dd28451d06b60b0c7bfb051f7949f92e28183f",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000013\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[\"0xa51b0df7a1ac2bea408fdc4f84f3d7060b62f976ab659b329b00065e5c8f2c37\"],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141452097,
                "block_unix_timestamp": 1763044644405,
                "error_log": "",
                "hash": "0xca4293a6629e5616964397f05a321d9fef05810484d51784dc7d23969be52c01",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgIncreasePositionMargin\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"source_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000013\",\"destination_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd700000000000000000000000c\",\"market_id\":\"0x30a1463cfb4c393c80e257ab93118cecd73c1e632dc4d2d31c12a51bc0a70bd7\",\"amount\":\"111330000.000000000000000000\"}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141452097,
                "block_unix_timestamp": 1763044644405,
                "error_log": "",
                "hash": "0xa9f1dbf2aa67ab2815d3b3441f06838fb941ba62f1336f176b01fd0cc42b17aa",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000013\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[\"0x30a1463cfb4c393c80e257ab93118cecd73c1e632dc4d2d31c12a51bc0a70bd7\"],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141392582,
                "block_unix_timestamp": 1763007645601,
                "error_log": "",
                "hash": "0xcc1a4abf6588749df65742854038fd336b15c1948eebaa6f834ef49884d3e2b9",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgIncreasePositionMargin\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"source_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000015\",\"destination_subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd700000000000000000000000c\",\"market_id\":\"0x30a1463cfb4c393c80e257ab93118cecd73c1e632dc4d2d31c12a51bc0a70bd7\",\"amount\":\"1650000.000000000000000000\"}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            },
            {
                "block_number": 141392582,
                "block_unix_timestamp": 1763007645601,
                "error_log": "",
                "hash": "0x671e85e26e966a9cd8c77dea4009c4bcb2c9f6aa3abb8b22619a3958cbfdca1e",
                "messages": "[{\"type\":\"/injective.exchange.v1beta1.MsgBatchUpdateOrders\",\"value\":{\"sender\":\"inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z\",\"subaccount_id\":\"0x935b705b5943b0e1b859651d2ccf375bc96dfdd7000000000000000000000015\",\"spot_market_ids_to_cancel_all\":[],\"derivative_market_ids_to_cancel_all\":[\"0x30a1463cfb4c393c80e257ab93118cecd73c1e632dc4d2d31c12a51bc0a70bd7\"],\"spot_orders_to_cancel\":[],\"derivative_orders_to_cancel\":[],\"spot_orders_to_create\":[],\"derivative_orders_to_create\":[],\"binary_options_orders_to_cancel\":[],\"binary_options_market_ids_to_cancel_all\":[],\"binary_options_orders_to_create\":[]}}]",
                "payer": "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z"
            }
        ],
        "pagination": {
            "has_next": false,
            "has_prev": false,
            "page": 1,
            "page_size": 10,
            "total": 0,
            "total_pages": 1
        }
    }
}
```

#### Get Account Balances
```http
GET /demo/account/balances?address={address}
```

Parameters:
- `address` (string): Account address

Returns account balances.

**Response**:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "balances": [
            {
                "amount": "6415196",
                "denom": "factory/inj1n636d9gzrqggdk66n2f97th0x8yuhfrtx520e7/ausd"
            },
            {
                "amount": "6415196",
                "denom": "factory/inj1n636d9gzrqggdk66n2f97th0x8yuhfrtx520e7/ausd"
            },
            {
                "amount": "6415196",
                "denom": "factory/inj1n636d9gzrqggdk66n2f97th0x8yuhfrtx520e7/ausd"
            },
            {
                "amount": "6415196",
                "denom": "factory/inj1n636d9gzrqggdk66n2f97th0x8yuhfrtx520e7/ausd"
            },
            {
                "amount": "6415196",
                "denom": "factory/inj1n636d9gzrqggdk66n2f97th0x8yuhfrtx520e7/ausd"
            },
            {
                "amount": "55561902679606140413",
                "denom": "inj"
            },
            {
                "amount": "55561332069038140413",
                "denom": "inj"
            },
            {
                "amount": "55561332069038140413",
                "denom": "inj"
            },
            {
                "amount": "55561332069038140413",
                "denom": "inj"
            },
            {
                "amount": "55561332069038140413",
                "denom": "inj"
            },
            {
                "amount": "1812809122",
                "denom": "peggy0xdAC17F958D2ee523a2206206994597C13D831ec7"
            },
            {
                "amount": "1812375667",
                "denom": "peggy0xdAC17F958D2ee523a2206206994597C13D831ec7"
            },
            {
                "amount": "1812375667",
                "denom": "peggy0xdAC17F958D2ee523a2206206994597C13D831ec7"
            },
            {
                "amount": "1812375667",
                "denom": "peggy0xdAC17F958D2ee523a2206206994597C13D831ec7"
            }
        ]
    }
}
```

### Health Check API

#### Check Service Health
```http
GET /demo/health?service={service_name}
```

**Query Parameters**:
- `service` (string, optional): Service name to check (default: "injective-explorer")

Returns the health status of the service and its components (database, Redis, etc.).

**Response**:

```json
{
    "status": "healthy",
    "service": "injective-explorer",
    "timestamp": 1763085718628,
    "details": {
        "database": {
            "status": "healthy",
            "latency_ms": 2.5
        },
        "redis": {
            "status": "healthy",
            "latency_ms": 1.2
        }
    }
}
```

**Status Values**:
- `healthy`: All critical components are operational
- `degraded`: Some non-critical components are unavailable
- `unhealthy`: Critical components (e.g., database) are unavailable

**HealthDetails Fields**:
- `status`: Component status ("healthy", "unhealthy", "not_configured")
- `error`: Error message if component is unhealthy (optional)
- `latency_ms`: Component response latency in milliseconds (optional)

### Streaming APIs

The service provides real-time streaming APIs for blocks, transactions, and account updates. These APIs use gRPC streaming and are also accessible via gRPC-Web for browser clients.

#### Stream Latest Blocks
```http
GET /demo/stream/blocks?from_height={height}
```

**Query Parameters**:
- `from_height` (int64, optional): Start streaming from a specific block height. If 0 or not provided, streams from the latest blocks.

Streams block updates as they are added to the blockchain. Returns a stream of `BlockUpdate` messages.

**Response Format** (streaming):

Each message in the stream:
```json
{
    "block": {
        "height": 141514703,
        "proposer": "injvalcons1rtejpld5nf6qnvtn2uhumrenqu9hmc06sv6xuq",
        "block_hash": "0xd666e1d52bbf7bb482e86a9e13139af6df45f6362d3b8db5ee4c298595e523ba",
        "num_txs": 12,
        "block_unix_timestamp": 1763085718628
    },
    "update_type": "new",
    "timestamp": 1763085718628
}
```

**Update Types**:
- `new`: A new block was added
- `update`: An existing block was updated

#### Stream Latest Transactions
```http
GET /demo/stream/transactions?types={type1,type2}
```

**Query Parameters**:
- `types` (repeated string, optional): Filter by transaction types. If empty, streams all transaction types.

Streams transaction updates as they are added to the blockchain. Returns a stream of `TransactionUpdate` messages.

**Response Format** (streaming):

Each message in the stream:
```json
{
    "transaction": {
        "hash": "0x75664123c37c239d9d5c14d476b2e16a3ed73b29caad8251f7425fe82d9f8dbf",
        "block_number": 141512064,
        "block_unix_timestamp": 1763084056386,
        "payer": "inj1s7kvd0ql49xt37nx2y4f90zy3hld5t6xpmdxhe",
        "messages": "...",
        "error_log": ""
    },
    "update_type": "new",
    "timestamp": 1763084056386
}
```

#### Stream Account Updates
```http
GET /demo/stream/account?address={address}&from_timestamp={timestamp}
```

**Query Parameters**:
- `address` (string, required): Account address to monitor
- `from_timestamp` (int64, optional): Start streaming from a specific timestamp. If 0 or not provided, streams from recent updates.

Streams account-related updates (transactions, balance changes, etc.) for a specific account. Returns a stream of `AccountUpdate` messages.

**Response Format** (streaming):

Each message in the stream:
```json
{
    "address": "inj1lfycr67vur5gayrakw6xj06sx86f4zf7qex26d",
    "update_type": "transaction",
    "data": {
        "transaction": {
            "hash": "0x8edc21fd00721bb06c77ed7187fe85c62690cc866afab51e50ced82faf9edf8d",
            "block_number": 141477796,
            "payer": "inj1lfycr67vur5gayrakw6xj06sx86f4zf7qex26d"
        },
        "metadata": {}
    },
    "timestamp": 1763062378197
}
```

**Update Types**:
- `transaction`: A new transaction involving the account
- `balance`: Account balance changed
- `account_update`: Other account-related updates

**Note**: Streaming APIs use Protocol Buffer messages (`BlockDTO`, `TransactionDTO`, `AccountUpdateData`) for type-safe data structures. For gRPC-Web clients, the data is automatically serialized/deserialized.

### Response Format

All APIs return a standard response format:

```json
{
  "code": 0,
  "message": "success",
  "data": "..." // JSON string containing the actual data
}
```

The `data` field contains a JSON string that, when parsed, typically has this structure:

```json
{
  "data": [...], // Array of items
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 1000,
      "total_pages": 50,
      "has_prev": false,
      "has_next": true,
      "approximate": false // true if total is an approximation
    }
  }
}
```

- `code`: Status code, same as HTTP Status code
- `message`: Response message
- `data`: Response data as JSON string or object (for paginated endpoints)

## gRPC API

The service provides gRPC APIs on the configured gRPC port (default: 9000). You can use any gRPC client to connect to the service.

Service definition: `api/explorer/v1/demo.proto`

### gRPC-Web Support

The service supports gRPC-Web through **Envoy Proxy**, enabling browser-based clients to interact with gRPC services. gRPC-Web requests are handled by Envoy on port 9001, which automatically converts them to standard gRPC requests and forwards them to the backend gRPC service.

**Architecture**:
```
Browser (gRPC-Web) → Envoy Proxy (9001) → gRPC Service (9000)
```

**Features**:
- Browser-compatible gRPC calls via Envoy proxy
- Support for both unary and streaming RPCs
- CORS support configured in Envoy
- Automatic Protocol Buffer serialization/deserialization
- Production-ready proxy solution with advanced features

**Configuration**:
See `envoy/envoy.yaml` for Envoy configuration. The Envoy proxy is automatically started via docker-compose.

**Usage Example** (JavaScript):
```javascript
// Using gRPC-Web client library, connect to Envoy proxy
const client = new DemoClient('http://localhost:9001');
const response = await client.checkHealthy({ service: 'injective-explorer' });
```

**Note**: For streaming RPCs via gRPC-Web, Envoy automatically handles the conversion. See `envoy/README.md` for detailed configuration.

## Architecture Details

### Database-First Strategy

The service implements a database-first query strategy for improved performance:

1. **Query from Database**: First attempts to retrieve data from the local database
2. **Fallback to Chain**: If data is not found in the database, queries the blockchain API
3. **Automatic Caching**: Data fetched from the chain is automatically saved to the database for future queries

This approach significantly reduces external API calls and improves response times.

### Block Synchronization

The service includes a comprehensive block synchronization system:

- **Real-time Sync**: Continuously synchronizes new blocks as they are produced
- **Backward Sync**: Efficiently syncs missing blocks when the service starts
- **Historical Sync**: Gradually syncs historical blocks in the background without affecting new block synchronization
- **Gap Detection**: Automatically detects and fills gaps in block data
- **Concurrent Processing**: Uses worker pools for efficient parallel block processing

### Pagination

The service supports two pagination strategies:

1. **Page-based Pagination**: Traditional offset-based pagination using `page` and `page_size`
2. **Cursor-based Pagination**: More efficient for large datasets using `cursor` (recommended for datasets with >10,000 records)

### Type-Safe API Responses

The service uses Protocol Buffer messages for all API responses, providing:

- **Type Safety**: Strongly-typed data structures defined in `api/explorer/v1/demo.proto`
- **Consistency**: Same message types used across gRPC, HTTP/REST, and gRPC-Web APIs
- **Performance**: Efficient binary serialization for gRPC, with automatic JSON conversion for HTTP
- **Documentation**: Proto definitions serve as API documentation and contract

**Key Message Types**:
- `BlockDTO`: Block information
- `TransactionDTO`: Transaction details
- `AccountDTO`: Account information
- `AccountBalanceDTO`: Account balance
- `HealthDetails`: Component health status
- `AccountUpdateData`: Account update information
- `LatestBlockHeightResponse`: Latest block height

### Error Handling

The service uses gRPC status codes for consistent error handling:

- `OK` (0): Success
- `InvalidArgument` (3): Invalid request parameters
- `Internal` (13): Internal server error
- `NotFound` (5): Resource not found

All errors include descriptive messages and are properly serialized for both gRPC and HTTP responses.

### Context Management

The service uses independent contexts for database queries and RPC calls to avoid HTTP timeout issues:

- **Database Queries**: Use independent context with `DBQueryTimeout` to prevent HTTP request cancellation from affecting database operations
- **RPC Calls**: Use independent context with `GRPCTimeout` for blockchain API calls

This ensures that long-running database queries or external API calls complete successfully even if the HTTP client disconnects.

## Data Storage Optimization

The service includes comprehensive data storage optimization features to handle large-scale blockchain data efficiently:

### Data Compression

**Automatic Compression**: Large transaction fields (Data, Messages, Logs) are automatically compressed using gzip when they exceed 1KB, reducing storage by 60-80%.

**Features**:
- Compression threshold: 1KB (configurable)
- Compression algorithm: gzip (level 6, balanced compression ratio and speed)
- Automatic decompression: Data is automatically decompressed when read
- Backward compatibility: Supports both compressed and uncompressed data formats
- Zero downtime: No migration required, new data is automatically compressed

**Usage**: Enabled by default, no configuration required. Compression happens automatically during data storage.

### Data Archiving

**Automatic Archiving**: Old transactions (>6 months) can be automatically archived to separate archive tables, keeping the main transactions table lean and fast.

**Features**:
- Configurable archive age (default: 6 months)
- Batch archiving for efficient processing
- Archive tables: `transaction_archives` and `transaction_data_archives`
- Automatic retrieval: Archived transactions are automatically retrieved when queried
- Dry-run mode: Test archiving without actually moving data

**Usage**: Use the `ArchiveService` to archive old transactions:

```go
archiveService := service.NewArchiveService()
config := service.DefaultArchiveConfig()
config.ArchiveAgeMonths = 6
err := archiveService.ArchiveOldTransactions(ctx, config)
```

### Storage Separation (Object Storage)

**Optional Object Storage**: Large binary fields (>10KB) can be stored in object storage services (MinIO, AWS S3, Alibaba OSS, Tencent COS) instead of the database.

**Features**:
- Multiple provider support: MinIO, AWS S3, Alibaba OSS, Tencent COS
- Automatic upload: Large fields are automatically uploaded to object storage
- Fallback mechanism: Falls back to database if object storage fails
- Storage keys: Only storage keys are stored in the database
- Backward compatibility: Supports both object-stored and database-stored data

**Configuration**: Enable object storage in `config.yaml`:

```yaml
data:
  object_storage:
    provider: minio  # or s3, oss, cos
    endpoint: localhost:9000
    access_key_id: minioadmin
    secret_access_key: minioadmin
    bucket_name: injective-explorer
    enabled: true  # Set to true to enable
```

**Migration**: Use `StorageMigrationService` to migrate existing data to object storage:

```go
migrationService := service.NewStorageMigrationService()
config := service.DefaultMigrationConfig()
config.Threshold = 10 * 1024  // 10KB threshold
err := migrationService.MigrateTransactions(ctx, config)
```

### Storage Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Transaction Storage                   │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │   Main DB    │  │ Archive DB  │  │ Object Store │ │
│  │              │  │             │  │              │ │
│  │ • Recent TX  │  │ • Old TX    │  │ • Large Data │ │
│  │ • Metadata   │  │ • Metadata  │  │ • Messages  │ │
│  │ • Compressed │  │ • Compressed│  │ • Logs      │ │
│  │   Data       │  │   Data      │  │             │ │
│  └──────────────┘  └──────────────┘  └──────────────┘ │
│                                                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │         Automatic Data Flow                       │  │
│  │  New TX → Compress → Store in Main DB            │  │
│  │  Old TX → Archive → Move to Archive DB           │  │
│  │  Large TX → Upload → Store in Object Storage     │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### Benefits

1. **Reduced Storage**: Compression reduces storage by 60-80%
2. **Improved Performance**: Smaller main table improves query performance
3. **Cost Efficiency**: Object storage is cheaper than database storage for large files
4. **Scalability**: Can handle billions of transactions efficiently
5. **Flexibility**: Can use different storage backends based on requirements

### Documentation

For detailed implementation documentation, see:
- `docs/storage-optimization.md`: Complete storage optimization strategy
- `docs/storage-separation-resources.md`: Object storage implementation details
- `docs/compression-implementation.md`: Compression feature documentation