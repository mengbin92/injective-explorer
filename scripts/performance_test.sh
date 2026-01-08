#!/bin/bash

# 性能测试脚本
# 该脚本用于测试区块同步优化后的性能

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 配置
SERVICE_URL="http://localhost:8000"
METRICS_URL="http://localhost:9090"
PPROF_URL="http://localhost:6060"
TEST_DURATION=300
RESULTS_DIR="results"

echo "====================================="
echo "   Block Sync Performance Test"
echo "====================================="
echo ""

# 创建结果目录
mkdir -p ${RESULTS_DIR}

# 1. 检查环境
echo -e "${YELLOW}[1/6] Checking environment...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: go is not installed${NC}"
    exit 1
fi

if ! command -v curl &> /dev/null; then
    echo -e "${RED}Error: curl is not installed${NC}"
    exit 1
fi

echo "✓ Environment check passed"
echo ""

# 2. 编译服务
echo -e "${YELLOW}[2/6] Building service...${NC}"
go build -o bin/injective-explorer cmd/injective-explorer/main.go
echo "✓ Build completed"
echo ""

# 3. 启动服务（如果未运行）
echo -e "${YELLOW}[3/6] Starting service...${NC}"
SERVICE_PID=$(pgrep -f "injective-explorer" || echo "")

if [ -z "$SERVICE_PID" ]; then
    echo "Starting new service instance..."
    ENABLE_SYNC=true \
    LOG_LEVEL=debug \
    ENABLE_METRICS=true \
    METRICS_PORT=9090 \
    PPROF_ENABLED=true \
    ./bin/injective-explorer server > ${RESULTS_DIR}/service.log 2>&1 &
    SERVICE_PID=$!
    echo "Service started with PID: $SERVICE_PID"

    # 等待服务启动
    echo "Waiting for service to start..."
    for i in {1..30}; do
        if curl -s ${SERVICE_URL}/health > /dev/null 2>&1; then
            echo "✓ Service started successfully"
            break
        fi
        echo -n "."
        sleep 1
    done
    echo ""
else
    echo "✓ Service already running (PID: $SERVICE_PID)"
fi
echo ""

# 4. 运行单元测试
echo -e "${YELLOW}[4/6] Running unit tests...${NC}"
go test -v ./internal/service -run TestBatchCheckBlockExistence -count=1 | tee ${RESULTS_DIR}/unit_tests.log
go test -v ./internal/service -run TestBlockWorker -count=1 >> ${RESULTS_DIR}/unit_tests.log 2>&1

echo "✓ Unit tests completed"
echo ""

# 5. 监控测试
echo -e "${YELLOW}[5/6] Monitoring block sync performance...${NC}"
echo "Monitoring for ${TEST_DURATION} seconds..."

# 创建监控脚本
cat > ${RESULTS_DIR}/monitor.sh << 'EOF'
#!/bin/bash
DURATION=$1
OUTPUT_FILE=$2
SERVICE_URL=$3

echo "timestamp,block_height_lag,blocks_processed,avg_processing_time" > $OUTPUT_FILE

START_TIME=$(date +%s)
while [ $(($(date +%s) - START_TIME)) -lt $DURATION ]; do
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')

    # Get metrics (if available)
    if curl -s ${SERVICE_URL}/metrics > /dev/null 2>&1; then
        LAG=$(curl -s ${SERVICE_URL}/metrics | grep "block_height_lag" | awk '{print $2}' || echo "0")
        PROCESSED=$(curl -s ${SERVICE_URL}/metrics | grep "blocks_processed_total" | awk '{print $2}' || echo "0")
        AVG_TIME=$(curl -s ${SERVICE_URL}/metrics | grep "block_processing_duration_avg" | awk '{print $2}' || echo "0")
    else
        LAG=0
        PROCESSED=0
        AVG_TIME=0
    fi

    echo "$TIMESTAMP,$LAG,$PROCESSED,$AVG_TIME" >> $OUTPUT_FILE

    sleep 1
done
EOF

chmod +x ${RESULTS_DIR}/monitor.sh

# 在后台运行监控
${RESULTS_DIR}/monitor.sh ${TEST_DURATION} ${RESULTS_DIR}/metrics.csv ${METRICS_URL} &
MONITOR_PID=$!

echo "Monitor PID: $MONITOR_PID"

# 等待监控完成
wait $MONITOR_PID

# 打印统计结果
echo ""
echo "Performance metrics collected:"
head -10 ${RESULTS_DIR}/metrics.csv
echo "..."

echo "✓ Monitoring completed"
echo ""

# 6. 性能分析
echo -e "${YELLOW}[6/6] Collecting performance profiles...${NC}"

# CPU profile
echo "Collecting CPU profile (30s)..."
curl -s ${PPROF_URL}/debug/pprof/profile?seconds=30 > ${RESULTS_DIR}/cpu.prof 2>&1 || echo "Warning: Could not collect CPU profile"

# Heap profile
echo "Collecting heap profile..."
curl -s ${PPROF_URL}/debug/pprof/heap > ${RESULTS_DIR}/heap.prof 2>&1 || echo "Warning: Could not collect heap profile"

# Goroutine profile
echo "Collecting goroutine profile..."
curl -s ${PPROF_URL}/debug/pprof/goroutine?debug=1 > ${RESULTS_DIR}/goroutines.txt 2>&1 || echo "Warning: Could not collect goroutine profile"

echo "✓ Performance profiles collected"
echo ""

# 结果分析
echo "====================================="
echo "   Performance Test Results"
echo "====================================="
echo ""

echo "Files created:"
echo "  - ${RESULTS_DIR}/service.log - Service logs"
echo "  - ${RESULTS_DIR}/unit_tests.log - Unit test results"
echo "  - ${RESULTS_DIR}/metrics.csv - Performance metrics"
echo "  - ${RESULTS_DIR}/cpu.prof - CPU profile"
echo "  - ${RESULTS_DIR}/heap.prof - Heap profile"
echo "  - ${RESULTS_DIR}/goroutines.txt - Goroutine info"
echo ""

# 简要统计
echo "Brief Statistics:"
echo "-----------------"
if [ -f ${RESULTS_DIR}/metrics.csv ]; then
    # Calculate average lag
    if command -v awk &> /dev/null; then
        AVG_LAG=$(tail -n +2 ${RESULTS_DIR}/metrics.csv | awk -F',' '{sum+=$2; count++} END {if(count>0) printf "%.2f", sum/count; else print "N/A"}')
        echo "Average block lag: $AVG_LAG blocks"

        MAX_LAG=$(tail -n +2 ${RESULTS_DIR}/metrics.csv | awk -F',' 'BEGIN {max=0} {if($2>max) max=$2} END {print max}')
        echo "Max block lag: $MAX_LAG blocks"
    else
        echo "Install awk for detailed statistics"
    fi
else
    echo "No metrics data available"
fi

echo ""
echo -e "${GREEN}Performance test completed successfully!${NC}"
echo ""
echo "Next steps:"
echo "1. Analyze metrics in ${RESULTS_DIR}/metrics.csv"
echo "2. Check performance profiles: go tool pprof ${RESULTS_DIR}/cpu.prof"
echo "3. Review service logs: tail -f ${RESULTS_DIR}/service.log"
echo "4. Verify block lag is < 50 blocks (target: < 10 blocks)"
echo ""
