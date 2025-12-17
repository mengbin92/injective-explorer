#!/bin/bash

# HTTP接口压力测试快速脚本
# 使用方法: ./scripts/loadtest.sh [选项]

# 默认配置
DEFAULT_URL="http://localhost:8000"
DEFAULT_CONCURRENCY=10
DEFAULT_DURATION="30s"

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOADTEST_BIN="$PROJECT_ROOT/bin/loadtest"

# 检查loadtest是否已编译
if [ ! -f "$LOADTEST_BIN" ]; then
    echo "正在编译loadtest工具..."
    cd "$PROJECT_ROOT"
    go build -o bin/loadtest ./cmd/loadtest
    if [ $? -ne 0 ]; then
        echo "编译失败！"
        exit 1
    fi
fi

# 解析参数
URL="${1:-$DEFAULT_URL}"
CONCURRENCY="${2:-$DEFAULT_CONCURRENCY}"
DURATION="${3:-$DEFAULT_DURATION}"

# 运行测试
echo "开始压力测试..."
echo "URL: $URL"
echo "并发数: $CONCURRENCY"
echo "持续时间: $DURATION"
echo ""

"$LOADTEST_BIN" -url "$URL" -c "$CONCURRENCY" -d "$DURATION"
