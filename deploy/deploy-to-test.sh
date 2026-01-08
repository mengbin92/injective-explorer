#!/bin/bash
# 部署到测试环境的脚本
# Deploy Block Sync Optimization to Test Environment

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
COMPOSE_FILE="docker-compose.test.yml"
PROJECT_NAME="injective-test"

# 函数定义
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 打印头信息
print_header() {
    echo ""
    echo "======================================"
    echo "  Injective Explorer - Test Environment"
    echo "  Block Sync Optimization Deployment"
    echo "======================================"
    echo ""
}

# 检查命令是否存在
check_command() {
    if ! command -v $1 > /dev/null; then
        log_error "$1 is not installed"
        exit 1
    fi
}

# 检查环境
pre_check() {
    log_info "Checking prerequisites..."

    check_command docker
    check_command docker-compose

    # 检查 Docker 是否运行
    if ! docker info > /dev/null 2>&1; then
        log_error "Docker is not running"
        exit 1
    fi

    log_success "Prerequisites check passed"
    echo ""
}

# 构建服务
build_service() {
    log_info "Building service..."

    # 编译代码
    log_info "Compiling Go binary..."
    go build -o bin/injective-explorer cmd/injective-explorer/main.go

    log_success "Build completed"
    echo ""
}

# 停止现有服务
stop_existing() {
    log_info "Stopping existing services..."

    if docker-compose -f ${COMPOSE_FILE} -p ${PROJECT_NAME} ps | grep -q "Up"; then
        docker-compose -f ${COMPOSE_FILE} -p ${PROJECT_NAME} down
        log_success "Existing services stopped"
    else
        log_info "No existing services running"
    fi

    echo ""
}

# 启动服务
start_services() {
    log_info "Starting test environment..."

    # 启动所有服务
    docker-compose -f ${COMPOSE_FILE} -p ${PROJECT_NAME} up -d

    # 等待服务健康
    log_info "Waiting for services to be healthy..."

    # 等待 PostgreSQL
    log_info "Waiting for PostgreSQL..."
    timeout 60s bash -c 'until docker exec injective-postgres-test pg_isready -U injective; do sleep 2; done'

    # 等待 Redis
    log_info "Waiting for Redis..."
    timeout 30s bash -c 'until docker exec injective-redis-test redis-cli ping; do sleep 2; done'

    # 等待 Injective Explorer
    log_info "Waiting for Injective Explorer..."
    timeout 120s bash -c 'until curl -s http://localhost:8000/health > /dev/null; do sleep 5; done'

    log_success "All services are healthy"
    echo ""
}

# 运行数据库迁移
run_migrations() {
    log_info "Running database migrations..."

    # 运行迁移（假设服务支持 migrate 命令）
    if docker exec injective-explorer-test ./injective-explorer migrate > /dev/null 2>&1; then
        log_success "Migrations completed"
    else
        log_warning "Migration command failed or not available, continuing..."
    fi

    echo ""
}

# 验证部署
verify_deployment() {
    log_info "Verifying deployment..."

    # 检查服务健康
    if curl -s http://localhost:8000/health > /dev/null; then
        log_success "API service is healthy"
    else
        log_error "API service health check failed"
        return 1
    fi

    # 检查 metrics
    if curl -s http://localhost:9090/metrics > /dev/null; then
        log_success "Metrics endpoint is accessible"
    else
        log_warning "Metrics endpoint is not accessible"
    fi

    # 检查 pprof
    if curl -s http://localhost:6060/debug/pprof/ > /dev/null; then
        log_success "Pprof endpoint is accessible"
    else
        log_warning "Pprof endpoint is not accessible"
    fi

    echo ""
}

# 输出访问信息
print_access_info() {
    log_info "Access Information:"
    echo "  API Service:        http://localhost:8000"
    echo "  Health Check:       http://localhost:8000/health"
    echo "  Metrics (Prom):     http://localhost:9090"
    echo "  Pprof:              http://localhost:6060"
    echo "  Grafana:            http://localhost:3000 (admin/admin)"
    echo "  PostgreSQL:         localhost:5432 (injective/injective_test_password)"
    echo "  Redis:              localhost:6379"
    echo ""
}

# 输出配置信息
print_config_info() {
    log_info "Block Sync Configuration:"
    echo "  Block Poll Interval:    500ms (optimized from 5s)"
    echo "  Backward Sync Workers:  5 workers (concurrent processing)"
    echo "  Tx Processors:          10 processors"
    echo "  EVM Workers:            3 workers"
    echo "  Batch Sizes:            10-50 (optimized)"
    echo ""
}

# 等待同步
wait_for_sync() {
    log_info "Waiting for block sync to start..."

    # 等待 60 秒让同步开始
    sleep 60

    # 检查日志
    log_info "Checking sync logs..."
    docker logs injective-explorer-test --tail 50 | grep "processing blocks" || log_warning "No sync logs found yet"

    echo ""
}

# 显示监控命令
show_monitoring_commands() {
    log_info "Useful Monitoring Commands:"
    echo ""
    echo "  # 查看实时日志"
    echo "  docker logs -f injective-explorer-test"
    echo ""
    echo "  # 查看区块同步日志"
    echo "  docker logs injective-explorer-test 2>&1 | grep 'processing blocks'"
    echo ""
    echo "  # 查看错误日志"
    echo "  docker logs injective-explorer-test 2>&1 | grep 'ERROR'"
    echo ""
    echo "  # 查看性能指标"
    echo "  curl -s http://localhost:9090/metrics | grep block_height_lag"
    echo ""
    echo "  # 检查 worker 数量"
    echo "  docker logs injective-explorer-test 2>&1 | grep 'starting.*workers'"
    echo ""
    echo "  # 运行性能测试"
    echo "  ./scripts/performance_test.sh"
    echo ""
    echo "  # 查看 Grafana 仪表盘"
    echo "  open http://localhost:3000"
    echo ""
}

# 主流程
main() {
    print_header

    # 步骤 1: 环境检查
    pre_check

    # 步骤 2: 构建服务
    build_service

    # 步骤 3: 停止现有服务
    stop_existing

    # 步骤 4: 启动服务
    start_services

    # 步骤 5: 运行迁移
    run_migrations

    # 步骤 6: 验证部署
    verify_deployment

    # 步骤 7: 等待同步
    wait_for_sync

    # 步骤 8: 输出信息
    print_access_info
    print_config_info
    show_monitoring_commands

    # 完成
    log_success "Deployment completed successfully!"
    echo ""
    echo "Next steps:"
    echo "  1. Monitor block sync: docker logs -f injective-explorer-test"
    echo "  2. Check Grafana dashboard: http://localhost:3000"
    echo "  3. Run performance tests: ./scripts/performance_test.sh"
    echo "  4. Verify block lag < 50 blocks (target: < 10)"
    echo ""
}

# 运行主流程
main "$@"
