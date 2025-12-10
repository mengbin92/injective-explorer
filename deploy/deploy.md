# Injective Explorer 部署文档

本文档提供了 Injective Explorer 区块链浏览器 API 服务的完整部署指南，包括开发环境、测试环境和生产环境的部署步骤。

## 目录

- [系统要求](#系统要求)
- [快速开始](#快速开始)
- [环境准备](#环境准备)
- [配置说明](#配置说明)
- [Docker 部署](#docker-部署)
- [手动部署](#手动部署)
- [生产环境部署](#生产环境部署)
- [数据库初始化](#数据库初始化)
- [监控和维护](#监控和维护)
- [故障排查](#故障排查)

## 系统要求

### 最低要求

- **CPU**: 2 核心
- **内存**: 4GB RAM
- **磁盘**: 50GB 可用空间（根据数据量调整）
- **网络**: 稳定的互联网连接（用于同步区块链数据）

### 推荐配置

- **CPU**: 4+ 核心
- **内存**: 8GB+ RAM
- **磁盘**: 200GB+ SSD（用于数据库和日志）
- **网络**: 100Mbps+ 带宽

### 软件依赖

#### 必需软件

- **操作系统**: Linux (Ubuntu 20.04+ / CentOS 7+ / Debian 10+) 或 macOS
- **Go**: 1.25.3 或更高版本
- **MySQL**: 8.0 或更高版本
- **Redis**: 6.0 或更高版本
- **Docker**: 20.10+ 和 Docker Compose 1.29+（可选，用于容器化部署）
- **Protocol Buffer Compiler (protoc)**: 3.20+（用于代码生成）

#### 可选软件

- **MinIO**: 用于对象存储（可选，用于存储大型交易数据）
- **Envoy**: 用于 gRPC-Web 支持（已包含在 Docker Compose 中）
- **Nginx**: 用于反向代理和负载均衡（生产环境推荐）

## 快速开始

### 使用 Docker Compose（推荐）

这是最简单的部署方式，适合开发和测试环境：

```bash
# 1. 克隆项目
git clone <repository-url>
cd injective-explorer

# 2. 复制环境变量文件
cp .env.example .env

# 3. 编辑配置文件（可选）
vim .env
vim configs/config.yaml

# 4. 启动所有服务
docker-compose up -d

# 5. 查看日志
docker-compose logs -f injective-explorer

# 6. 检查服务状态
docker-compose ps
```

服务将在以下端口启动：
- HTTP API: http://localhost:8000
- gRPC API: localhost:9000
- gRPC-Web (Envoy): http://localhost:9001
- Envoy Admin: http://localhost:9901
- MySQL: localhost:3306
- Redis: localhost:6379

### 验证部署

```bash
# 检查健康状态
curl http://localhost:8000/demo/health

# 查看最新区块
curl http://localhost:8000/demo/block/latest?page=1&page_size=5
```

## 环境准备

### 1. 安装 Go

```bash
# Ubuntu/Debian
wget https://go.dev/dl/go1.25.3.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.3.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# macOS (使用 Homebrew)
brew install go@1.25

# 验证安装
go version
```

### 2. 安装 Protocol Buffer Compiler

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y protobuf-compiler

# macOS
brew install protobuf

# 验证安装
protoc --version
```

### 3. 安装 MySQL

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y mysql-server
sudo systemctl start mysql
sudo systemctl enable mysql

# macOS
brew install mysql
brew services start mysql

# 创建数据库和用户
sudo mysql -e "CREATE DATABASE explorer CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
sudo mysql -e "CREATE USER 'explorer'@'localhost' IDENTIFIED BY 'your_password';"
sudo mysql -e "GRANT ALL PRIVILEGES ON explorer.* TO 'explorer'@'localhost';"
sudo mysql -e "FLUSH PRIVILEGES;"
```

### 4. 安装 Redis

```bash
# Ubuntu/Debian
sudo apt-get install -y redis-server
sudo systemctl start redis
sudo systemctl enable redis

# macOS
brew install redis
brew services start redis

# 验证安装
redis-cli ping
```

### 5. 安装 Docker（可选）

```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# macOS
brew install --cask docker

# 安装 Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# 验证安装
docker --version
docker-compose --version
```

## 配置说明

### 环境变量配置

项目支持通过环境变量进行配置，优先级高于 YAML 配置文件。

#### 创建环境变量文件

```bash
cp .env.example .env
vim .env
```

#### 主要环境变量

**服务器配置**
```bash
HTTP_PORT=8000              # HTTP 服务端口
GRPC_PORT=9000              # gRPC 服务端口
ENABLE_PPROF=false          # 生产环境必须设为 false
```

**数据库配置**
```bash
DB_USER=root                # 数据库用户名
DB_PASSWORD=your_password   # 数据库密码（生产环境使用强密码）
DB_HOST=localhost           # 数据库主机
DB_PORT=3306                # 数据库端口
DB_NAME=explorer            # 数据库名称
```

**Redis 配置**
```bash
REDIS_HOST=localhost        # Redis 主机
REDIS_PORT=6379             # Redis 端口
REDIS_PASSWORD=             # Redis 密码（可选）
REDIS_DB=0                  # Redis 数据库编号
```

**对象存储配置（可选）**
```bash
OBJECT_STORAGE_PROVIDER=minio        # 存储提供商: minio, s3, oss, cos
OBJECT_STORAGE_ENDPOINT=localhost:9000
OBJECT_STORAGE_ACCESS_KEY=minioadmin
OBJECT_STORAGE_SECRET_KEY=minioadmin
OBJECT_STORAGE_BUCKET=injective-explorer
OBJECT_STORAGE_ENABLED=false         # 是否启用对象存储
```

### YAML 配置文件

主配置文件位于 `configs/config.yaml`，支持环境变量占位符：

```yaml
server:
  http:
    addr: 0.0.0.0:${HTTP_PORT:8000}
    timeout: 30s
  grpc:
    addr: 0.0.0.0:${GRPC_PORT:9000}
    timeout: 30s
  enable_swagger: true
  enable_cors: true

data:
  database:
    driver: mysql
    source: ${DB_USER:root}:${DB_PASSWORD:password}@tcp(${DB_HOST:localhost}:${DB_PORT:3306})/${DB_NAME:explorer}?charset=utf8mb4&parseTime=True&loc=Local
  redis:
    addr: ${REDIS_HOST:localhost}:${REDIS_PORT:6379}
    password: ${REDIS_PASSWORD:}
    db: ${REDIS_DB:0}

chain:
  network: mainnet          # mainnet, testnet, 或 custom
  # 其他链配置...

sync:
  intervals:
    block_poll: 5s          # 区块轮询间隔
    tx_process: 1s          # 交易处理间隔
  concurrency:
    tx_processors: 5        # 并发交易处理器数量
```

## Docker 部署

### 使用 Docker Compose

#### 1. 准备配置文件

```bash
# 创建必要的目录
mkdir -p configs

# 复制配置文件
cp configs/config.yaml configs/config.yaml

# 创建 .env 文件
cp .env.example .env
vim .env
```

#### 2. 启动服务

```bash
# 构建并启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f
```

#### 3. 停止服务

```bash
# 停止所有服务
docker-compose stop

# 停止并删除容器
docker-compose down

# 停止并删除容器、卷和网络
docker-compose down -v
```

#### 4. 更新服务

```bash
# 拉取最新代码
git pull

# 重新构建镜像
docker-compose build

# 重启服务
docker-compose up -d
```

### 单独使用 Docker

```bash
# 构建镜像
docker build -t injective-explorer:latest .

# 运行容器
docker run -d \
  --name injective-explorer \
  -p 8000:8000 \
  -p 9000:9000 \
  -v $(pwd)/configs:/data/conf \
  -v $(pwd)/.env:/app/.env \
  --env-file .env \
  injective-explorer:latest
```

## 手动部署

### 1. 获取源代码

```bash
git clone <repository-url>
cd injective-explorer
```

### 2. 安装依赖工具

```bash
# 安装 Go 工具
make init

# 或手动安装
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest
go install github.com/google/wire/cmd/wire@latest
```

### 3. 生成代码

```bash
# 生成所有代码（proto, config等）
make all
```

### 4. 配置环境

```bash
# 复制环境变量文件
cp .env.example .env

# 编辑配置
vim .env
vim configs/config.yaml
```

### 5. 构建应用

```bash
# 构建二进制文件
make build

# 或手动构建
go build -ldflags "-X main.Version=$(git describe --tags --always)" -o ./bin/injective-explorer ./cmd/injective-explorer
```

### 6. 运行应用

```bash
# 直接运行
./bin/injective-explorer -conf ./configs

# 或使用 systemd 服务（见下方）
```

### 7. 创建 Systemd 服务（Linux）

创建 `/etc/systemd/system/injective-explorer.service`:

```ini
[Unit]
Description=Injective Explorer API Service
After=network.target mysql.service redis.service

[Service]
Type=simple
User=explorer
WorkingDirectory=/opt/injective-explorer
ExecStart=/opt/injective-explorer/bin/injective-explorer -conf /opt/injective-explorer/configs
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

# 环境变量
Environment="HTTP_PORT=8000"
Environment="GRPC_PORT=9000"
EnvironmentFile=/opt/injective-explorer/.env

# 资源限制
LimitNOFILE=65535
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

启用并启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable injective-explorer
sudo systemctl start injective-explorer
sudo systemctl status injective-explorer
```

## 生产环境部署

### 架构建议

```
                    ┌─────────────┐
                    │   Nginx     │  (负载均衡/SSL终止)
                    │  (443/80)   │
                    └──────┬──────┘
                           │
            ┌──────────────┼──────────────┐
            │              │              │
      ┌─────▼─────┐  ┌─────▼─────┐  ┌─────▼─────┐
      │  Envoy    │  │  Envoy    │  │  Envoy    │  (gRPC-Web代理)
      │  (9001)   │  │  (9001)   │  │  (9001)   │
      └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
            │              │              │
      ┌─────▼─────┐  ┌─────▼─────┐  ┌─────▼─────┐
      │ Explorer  │  │ Explorer  │  │ Explorer  │  (应用实例)
      │  (9000)   │  │  (9000)   │  │  (9000)   │
      └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
            │              │              │
            └──────────────┼──────────────┘
                           │
            ┌──────────────┼──────────────┐
            │              │              │
      ┌─────▼─────┐  ┌─────▼─────┐  ┌─────▼─────┐
      │  MySQL    │  │  Redis    │  │  MinIO    │  (数据存储)
      │  (主从)   │  │ (集群)    │  │ (可选)    │
      └───────────┘  └───────────┘  └───────────┘
```

### 安全配置

#### 1. 防火墙设置

```bash
# 只开放必要端口
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
sudo ufw enable
```

#### 2. SSL/TLS 配置

使用 Let's Encrypt 获取免费 SSL 证书：

```bash
# 安装 Certbot
sudo apt-get install certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d your-domain.com

# 自动续期
sudo certbot renew --dry-run
```

#### 3. Nginx 反向代理配置

创建 `/etc/nginx/sites-available/injective-explorer`:

```nginx
upstream explorer_http {
    least_conn;
    server localhost:8000 max_fails=3 fail_timeout=30s;
    # 添加更多实例
    # server localhost:8001 max_fails=3 fail_timeout=30s;
}

upstream explorer_grpc_web {
    least_conn;
    server localhost:9001 max_fails=3 fail_timeout=30s;
    # 添加更多实例
    # server localhost:9002 max_fails=3 fail_timeout=30s;
}

server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # HTTP API
    location / {
        proxy_pass http://explorer_http;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # gRPC-Web
    location /api.explorer.v1.Demo/ {
        grpc_pass grpc://explorer_grpc_web;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;
        grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        
        # 流式传输超时
        grpc_connect_timeout 0s;
        grpc_send_timeout 0s;
        grpc_read_timeout 0s;
    }

    # 日志
    access_log /var/log/nginx/injective-explorer-access.log;
    error_log /var/log/nginx/injective-explorer-error.log;

    # ========== CORS 统一由 Nginx 返回 ==========
    add_header Access-Control-Allow-Origin  "$http_origin" always;
    add_header Access-Control-Allow-Credentials true always;
    add_header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, OPTIONS" always;
    add_header Access-Control-Allow-Headers "keep-alive,user-agent,cache-control,content-type,content-transfer-encoding,x-accept-content-transfer-encoding,x-accept-response-streaming,x-user-agent,x-grpc-web,grpc-timeout" always;
    add_header Access-Control-Expose-Headers "grpc-status,grpc-message" always;
    add_header Access-Control-Max-Age 1728000 always;

    # 预检请求直接 204
    if ($request_method = OPTIONS) {
        return 204;
    }
}
```

启用配置：

```bash
sudo ln -s /etc/nginx/sites-available/injective-explorer /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

#### 4. 数据库安全

```sql
-- 创建只读用户（用于监控）
CREATE USER 'explorer_readonly'@'%' IDENTIFIED BY 'strong_password';
GRANT SELECT ON explorer.* TO 'explorer_readonly'@'%';

-- 限制连接数
SET GLOBAL max_connections = 500;

-- 启用慢查询日志
SET GLOBAL slow_query_log = 'ON';
SET GLOBAL long_query_time = 2;
```

#### 5. Redis 安全

```bash
# 设置密码
redis-cli CONFIG SET requirepass "strong_password"

# 在配置文件中设置
echo "requirepass strong_password" >> /etc/redis/redis.conf
```

### 性能优化

#### 1. 数据库优化

```sql
-- 创建索引
CREATE INDEX idx_block_height ON blocks(height);
CREATE INDEX idx_tx_hash ON transactions(hash);
CREATE INDEX idx_tx_block_number ON transactions(block_number);
CREATE INDEX idx_tx_signer ON transactions(signer);

-- 分区表（大量数据时）
-- 参考 database/partition_tables.sql
```

#### 2. Redis 优化

```bash
# 配置 Redis
vim /etc/redis/redis.conf

# 设置最大内存
maxmemory 2gb
maxmemory-policy allkeys-lru

# 启用持久化
save 900 1
save 300 10
save 60 10000
```

#### 3. 应用优化

```yaml
# configs/config.yaml
sync:
  concurrency:
    tx_processors: 10        # 根据 CPU 核心数调整
    backward_sync_workers: 5
    historical_sync_workers: 3
  batch_size:
    backward_sync: 20       # 增加批次大小
    historical_sync: 10
```

## 监控和维护

### 健康检查

```bash
# HTTP 健康检查
curl http://localhost:8000/demo/health

# 检查数据库连接
mysql -u root -p -e "SELECT 1"

# 检查 Redis 连接
redis-cli ping
```

### 日志管理

#### 应用日志

```bash
# Docker 日志
docker-compose logs -f injective-explorer

# Systemd 日志
journalctl -u injective-explorer -f

# 日志轮转配置
cat > /etc/logrotate.d/injective-explorer << 'EOF'
/var/log/injective-explorer/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0644 explorer explorer
}
EOF
```

#### 数据库日志

```sql
-- 查看慢查询
SELECT * FROM mysql.slow_log ORDER BY start_time DESC LIMIT 10;

-- 查看连接数
SHOW PROCESSLIST;
SHOW STATUS LIKE 'Threads_connected';
```

### 性能监控

#### 使用 Prometheus（可选）

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'injective-explorer'
    static_configs:
      - targets: ['localhost:8000']
    metrics_path: '/metrics'
```

#### 系统监控

```bash
# CPU 和内存使用
top -p $(pgrep injective-explorer)

# 磁盘使用
df -h
du -sh /var/lib/mysql/explorer

# 网络连接
netstat -an | grep -E '8000|9000'
```

### 定期维护任务

```bash
# 创建维护脚本
cat > /usr/local/bin/maintain-explorer.sh << 'EOF'
#!/bin/bash

# 1. 数据库优化
mysql -u root -p'password' explorer -e "OPTIMIZE TABLE transactions, blocks;"

# 2. 清理旧日志（保留 30 天）
find /var/log/injective-explorer -name "*.log" -mtime +30 -delete

# 3. 检查磁盘空间
df -h | awk '$5 > 80 {print "Warning: Disk usage > 80%"}'

# 4. 重启服务（如果需要）
# systemctl restart injective-explorer
EOF

chmod +x /usr/local/bin/maintain-explorer.sh

# 每周执行一次
echo "0 3 * * 0 /usr/local/bin/maintain-explorer.sh" | crontab -
```

## 故障排查

### 常见问题

#### 1. 服务无法启动

```bash
# 检查端口占用
sudo netstat -tulpn | grep -E '8000|9000'

# 检查配置文件
./bin/injective-explorer -conf ./configs --check-config

# 查看详细日志
journalctl -u injective-explorer -n 100
```

#### 2. 数据库连接失败

```bash
# 测试数据库连接
mysql -u root -p -h localhost -e "SELECT 1"

# 检查数据库服务
sudo systemctl status mysql

# 检查防火墙
sudo ufw status
```

#### 3. 同步速度慢

```yaml
# 增加并发数
sync:
  concurrency:
    tx_processors: 10
    backward_sync_workers: 5

# 检查网络连接
ping sentry.lcd.injective.network
```

#### 4. 内存不足

```bash
# 检查内存使用
free -h
ps aux | grep injective-explorer

# 调整 Redis 内存限制
redis-cli CONFIG SET maxmemory 2gb
```

#### 5. 磁盘空间不足

```bash
# 检查磁盘使用
df -h
du -sh /var/lib/mysql/explorer

# 清理旧数据（归档）
# 使用 ArchiveService 归档旧交易
```

### 调试模式

```bash
# 启用调试日志
export LOG_LEVEL=-1  # debug 模式

# 启用 pprof（仅开发环境）
export ENABLE_PPROF=true

# 访问 pprof
go tool pprof http://localhost:8000/debug/pprof/profile
```

### 获取支持

如果遇到问题，请提供以下信息：

1. 错误日志
2. 配置文件（隐藏敏感信息）
3. 系统信息（OS, Go 版本等）
4. 数据库版本
5. 问题复现步骤

## 升级指南

### 升级步骤

1. **备份数据**
   ```bash
   mysqldump -u root -p explorer > backup_$(date +%Y%m%d).sql
   ```

2. **停止服务**
   ```bash
   docker-compose stop
   # 或
   systemctl stop injective-explorer
   ```

3. **更新代码**
   ```bash
   git pull
   git checkout <new-version>
   ```

4. **重新生成代码**
   ```bash
   make all
   ```

5. **重新构建**
   ```bash
   make build
   # 或
   docker-compose build
   ```

6. **执行数据库迁移**（如果有）
   ```bash
   # 应用会自动执行迁移
   ```

7. **启动服务**
   ```bash
   docker-compose up -d
   # 或
   systemctl start injective-explorer
   ```

8. **验证**
   ```bash
   curl http://localhost:8000/demo/health
   ```

## 附录

### 端口说明

| 端口 | 服务        | 说明                       |
| ---- | ----------- | -------------------------- |
| 8000 | HTTP API    | RESTful API 服务           |
| 9000 | gRPC API    | gRPC 服务                  |
| 9001 | gRPC-Web    | Envoy 代理的 gRPC-Web 端口 |
| 9901 | Envoy Admin | Envoy 管理界面             |
| 3306 | MySQL       | 数据库服务                 |
| 6379 | Redis       | 缓存服务                   |
| 9000 | MinIO       | 对象存储服务（如果启用）   |

### 环境变量完整列表

参考 `.env.example` 文件获取完整的环境变量列表和说明。

---

**最后更新**: 2025-01-XX
**维护者**: Injective Explorer Team
