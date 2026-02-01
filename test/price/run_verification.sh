#!/bin/bash
# Phase 0 SDK API 验证脚本
# 运行所有 API 验证测试并生成报告

set -e

echo "=========================================="
echo "Phase 0: SDK API Verification"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查是否在项目根目录
if [ ! -f "go.mod" ]; then
    echo -e "${RED}Error: 请在项目根目录运行此脚本${NC}"
    exit 1
fi

# 创建输出目录
mkdir -p test/price/results

# 获取当前时间戳
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
REPORT_FILE="test/price/results/verification_${TIMESTAMP}.log"

echo "验证报告将保存到: $REPORT_FILE"
echo ""

# 运行测试的函数
run_test() {
    local test_name=$1
    local test_pattern=$2

    echo "----------------------------------------"
    echo "运行: $test_name"
    echo "----------------------------------------"

    if go test -v ./test/price/... -run "$test_pattern" 2>&1 | tee -a "$REPORT_FILE"; then
        echo -e "${GREEN}✓ $test_name 通过${NC}"
        return 0
    else
        echo -e "${RED}✗ $test_name 失败或跳过${NC}"
        return 1
    fi
}

# 1. Oracle API 验证
echo ""
echo "=========================================="
echo "1. Oracle 模块 API 验证"
echo "=========================================="

run_test "Oracle Params API" "TestOracleParamsAPI"
run_test "Oracle Price Feeds API" "TestOraclePriceFeedsAPI"
run_test "Oracle Price By Symbol" "TestOraclePriceBySymbolAPI"
run_test "Oracle Price By Denom" "TestOraclePriceByDenomAPI"
run_test "Oracle Band Prices" "TestOracleBandPricesAPI"
run_test "Oracle Price Precision" "TestOraclePricePrecision"
run_test "Oracle Module State" "TestOracleModuleStateAPI"

# 2. Spot Market API 验证
echo ""
echo "=========================================="
echo "2. Spot Market 模块 API 验证"
echo "=========================================="

run_test "Spot Markets List" "TestSpotMarketsListAPI"
run_test "Spot Market Detail" "TestSpotMarketDetailAPI"
run_test "Spot Orderbook" "TestSpotOrderbookAPI"
run_test "Spot Price From Orderbook" "TestSpotMarketPriceFromOrderbook"
run_test "Spot Markets By Denom" "TestSpotMarketsByDenom"
run_test "Spot Market Statistics" "TestSpotMarketStatistics"
run_test "Spot Market Price Precision" "TestSpotMarketPricePrecision"

# 3. Explorer API 验证
echo ""
echo "=========================================="
echo "3. Explorer API 验证"
echo "=========================================="

run_test "Explorer Spot Markets" "TestExplorerSpotMarketsAPI"
run_test "Explorer Market Detail" "TestExplorerSpotMarketDetailAPI"
run_test "Explorer Market Summary" "TestExplorerMarketSummaryAPI"
run_test "Explorer Market Trades" "TestExplorerMarketTradesAPI"
run_test "Explorer All Markets Summary" "TestExplorerAllMarketsSummaryAPI"
run_test "Explorer Price Precision" "TestExplorerPricePrecision"
run_test "Explorer Oracle Price" "TestExplorerOraclePriceAPI"
run_test "Explorer Network Comparison" "TestExplorerNetworkComparison"

# 生成汇总
echo ""
echo "=========================================="
echo "验证完成"
echo "=========================================="
echo ""
echo "详细报告: $REPORT_FILE"
echo ""

# 检查是否有未实现的 API
echo "检查未实现的 API..."
if grep -q "not implemented" "$REPORT_FILE" 2>/dev/null || grep -q "API not available" "$REPORT_FILE" 2>/dev/null; then
    echo -e "${YELLOW}⚠ 发现未实现或不可用的 API，请查看报告详情${NC}"
fi

# 提示下一步
echo ""
echo "下一步:"
echo "1. 查看详细报告: cat $REPORT_FILE"
echo "2. 根据验证结果更新 docs/onchain-price-phase0-sdk-verification.md"
echo "3. 更新技术方案中的 TODO 部分"
echo ""
