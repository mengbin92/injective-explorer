#!/bin/bash
# 测试运行脚本
# Quick Test Runner for Block Sync Optimization

set -e

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "======================================"
echo "  Block Sync Optimization Test"
echo "======================================"
echo ""

# 检查服务是否运行
check_service() {
    if curl -s http://localhost:8000/health > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} Service is running"
        return 0
    else
        echo -e "${RED}✗${NC} Service is not running"
        return 1
    fi
}

# 测试轮询间隔
test_poll_interval() {
    echo -e "\n${YELLOW}[Test 1] Testing poll interval...${NC}"

    if docker logs injective-explorer-test 2>&1 | grep -q "processing blocks from"; then
        echo -e "${GREEN}✓${NC} Block processing logs found"
        docker logs injective-explorer-test 2>&1 | grep "processing blocks from" | tail -3
    else
        echo -e "${YELLOW}!${NC} No processing logs yet (waiting for sync to start)"
    fi
}

# 测试 worker
test_workers() {
    echo -e "\n${YELLOW}[Test 2] Testing worker pool...${NC}"

    if docker logs injective-explorer-test 2>&1 | grep -q "starting.*workers"; then
        echo -e "${GREEN}✓${NC} Worker pool logs found"
        docker logs injective-explorer-test 2>&1 | grep "starting.*workers" | tail -1
    else
        echo -e "${RED}✗${NC} Worker pool logs not found"
    fi
}

# 测试 metrics
test_metrics() {
    echo -e "\n${YELLOW}[Test 3] Testing metrics endpoint...${NC}"

    if curl -s http://localhost:9090/metrics > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} Metrics endpoint accessible"

        # 查看 block 相关指标
        echo "Block-related metrics:"
        curl -s http://localhost:9090/metrics | grep "^block_" | head -5 || echo "No block metrics yet (waiting for sync)"
    else
        echo -e "${YELLOW}!${NC} Metrics endpoint not ready yet"
    fi
}

# 测试批量检查
test_batch_check() {
    echo -e "\n${YELLOW}[Test 4] Testing batch existence check...${NC}"

    if docker logs injective-explorer-test 2>&1 | grep -q "batch checked"; then
        echo -e "${GREEN}✓${NC} Batch check logs found"
        docker logs injective-explorer-test 2>&1 | grep "batch checked" | tail -1
    else
        echo -e "${YELLOW}!${NC} Batch check logs not yet (waiting for sync)"
    fi
}

# 测试处理统计
test_processing_stats() {
    echo -e "\n${YELLOW}[Test 5] Testing processing statistics...${NC}"

    if docker logs injective-explorer-test 2>&1 | grep -q "processed.*blocks.*avg"; then
        echo -e "${GREEN}✓${NC} Processing statistics found"
        docker logs injective-explorer-test 2>&1 | grep "processed.*blocks.*avg" | tail -3
    else
        echo -e "${YELLOW}!${NC} Processing statistics not yet (waiting for sync)"
    fi
}

# 运行性能测试
test_performance() {
    echo -e "\n${YELLOW}[Test 6] Running performance test...${NC}"

    if [ -x "./scripts/performance_test.sh" ]; then
        echo "Running performance test (5 minutes)..."
        ./scripts/performance_test.sh
    else
        echo -e "${RED}✗${NC} Performance test script not found or not executable"
    fi
}

# 主函数
main() {
    case "${1:-all}" in
        health)
            check_service
            ;;
        interval)
            test_poll_interval
            ;;
        workers)
            test_workers
            ;;
        metrics)
            test_metrics
            ;;
        batch)
            test_batch_check
            ;;
        stats)
            test_processing_stats
            ;;
        performance)
            test_performance
            ;;
        all)
            check_service
            test_poll_interval
            test_workers
            test_metrics
            test_batch_check
            test_processing_stats
            ;;
        *)
            echo "Usage: $0 {health|interval|workers|metrics|batch|stats|performance|all}"
            exit 1
            ;;
    esac

    echo ""
    echo "======================================"
    echo -e "${GREEN}Test completed!${NC}"
    echo ""
    echo "Quick commands:"
    echo "  View logs:   docker logs -f injective-explorer-test"
    echo "  Check lag:   curl -s http://localhost:9090/metrics | grep block_height_lag"
    echo "  Dashboard:   http://localhost:3000 (admin/admin)"
    echo ""
}

# 运行
main "$@"
