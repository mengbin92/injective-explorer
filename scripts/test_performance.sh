#!/bin/bash

# 性能测试脚本 - 验证不同配置下的QPS
# 使用方法: ./test_performance.sh

echo "=== injective-explorer 性能测试 ==="
echo ""

# 编译loadtest工具
echo "1. 编译loadtest工具..."
go build -o ./bin/loadtest cmd/loadtest/main.go

if [ ! -f "bin/loadtest" ]; then
    echo "错误: loadtest编译失败"
    exit 1
fi

echo "✓ loadtest编译成功"
echo ""

SERVER_URL="http://localhost:8000"
CONCURRENCY=300
DURATION="60s"

echo "服务器地址: $SERVER_URL"
echo "并发数: $CONCURRENCY"
echo "测试时长: $DURATION"
echo ""

# 测试1: 健康检查接口（最轻量级）
echo "=== 测试1: 健康检查接口 ==="
echo "这个测试只调用 /api/v1/health 接口，验证基础性能"
bin/loadtest -url $SERVER_URL -endpoint /api/v1/health -c $CONCURRENCY -d $DURATION

echo ""
echo "按Enter继续下一个测试..."
read

# 测试2: 单个区块查询（中等负载）
echo "=== 测试2: 区块查询接口 ==="
echo "测试 /api/v1/block/by-height 接口"
bin/loadtest -url $SERVER_URL -endpoint /api/v1/block/by-height -c $CONCURRENCY -d $DURATION

echo ""
echo "按Enter继续下一个测试..."
read

# 测试3: 所有接口（真实场景）
echo "=== 测试3: 所有接口混合测试 ==="
echo "模拟真实场景，测试所有接口的混合性能"
bin/loadtest -url $SERVER_URL -c $CONCURRENCY -d $DURATION

echo ""
echo "=== 测试完成 ==="
echo ""
echo "分析建议:"
echo "1. 如果测试1的QPS很高 (>500)，但测试3很低 (<100)，说明中间件是瓶颈"
echo "2. 如果测试1和测试3的QPS都很低，说明数据库或应用逻辑是瓶颈"
echo "3. 对比测试2和测试1的差异，可以估算业务逻辑的处理时间"
echo ""
