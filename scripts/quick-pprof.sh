#!/bin/bash
# 快速 pprof 分析脚本 - 用于快速查看性能瓶颈

set -e

PPROF_HOST="${PPROF_HOST:-localhost:6060}"
PROFILE_DURATION="${PROFILE_DURATION:-30}"

echo "=========================================="
echo "快速 pprof 性能分析"
echo "=========================================="
echo "服务器: $PPROF_HOST"
echo "分析时长: ${PROFILE_DURATION}秒"
echo "=========================================="
echo ""

# 检查 graphviz（可选）
if ! command -v dot &> /dev/null; then
    echo "💡 提示: 未检测到 graphviz，但 Web 界面可以正常使用（推荐）"
    echo ""
fi

# 检查连接
if ! curl -s "http://$PPROF_HOST/debug/pprof/" > /dev/null; then
    echo "❌ 无法连接到 $PPROF_HOST/debug/pprof/"
    echo ""
    echo "请确保:"
    echo "  1. 服务器正在运行"
    echo "  2. 已设置 ENABLE_PPROF=true"
    echo "  3. 服务器监听在 $PPROF_HOST"
    exit 1
fi

echo "✓ 服务器连接正常"
echo ""

# 选择分析类型
echo "请选择分析类型:"
echo "  1) CPU 性能分析 (推荐)"
echo "  2) 内存分析"
echo "  3) Goroutine 分析"
echo "  4) 阻塞分析"
echo "  5) 互斥锁分析"
echo "  6) 查看所有类型"
echo ""
read -p "请输入选项 [1-6] (默认: 1): " choice
choice=${choice:-1}

case $choice in
    1)
        echo ""
        echo "正在采集 CPU profile (${PROFILE_DURATION}秒)..."
        go tool pprof -http=:8080 "http://$PPROF_HOST/debug/pprof/profile?seconds=$PROFILE_DURATION"
        ;;
    2)
        echo ""
        echo "正在采集内存 profile..."
        go tool pprof -http=:8080 "http://$PPROF_HOST/debug/pprof/heap"
        ;;
    3)
        echo ""
        echo "正在采集 Goroutine profile..."
        go tool pprof -http=:8080 "http://$PPROF_HOST/debug/pprof/goroutine"
        ;;
    4)
        echo ""
        echo "正在采集阻塞 profile..."
        go tool pprof -http=:8080 "http://$PPROF_HOST/debug/pprof/block"
        ;;
    5)
        echo ""
        echo "正在采集互斥锁 profile..."
        go tool pprof -http=:8080 "http://$PPROF_HOST/debug/pprof/mutex"
        ;;
    6)
        echo ""
        echo "正在采集所有类型的 profile..."
        
        # CPU
        echo "  → CPU profile..."
        go tool pprof -proto -output=/tmp/cpu_$$.pb.gz \
          "http://$PPROF_HOST/debug/pprof/profile?seconds=$PROFILE_DURATION" &
        CPU_PID=$!
        
        # 等待 CPU profile 完成
        wait $CPU_PID
        
        # 其他 profiles
        echo "  → Heap profile..."
        go tool pprof -proto -output=/tmp/heap_$$.pb.gz \
          "http://$PPROF_HOST/debug/pprof/heap" &
        HEAP_PID=$!
        
        echo "  → Goroutine profile..."
        go tool pprof -proto -output=/tmp/goroutine_$$.pb.gz \
          "http://$PPROF_HOST/debug/pprof/goroutine" &
        GOROUTINE_PID=$!
        
        wait $HEAP_PID $GOROUTINE_PID
        
        echo ""
        echo "✓ 所有 profile 已采集完成"
        echo ""
        echo "查看 CPU 分析:"
        echo "  go tool pprof -http=:8080 /tmp/cpu_$$.pb.gz"
        echo ""
        echo "查看内存分析:"
        echo "  go tool pprof -http=:8081 /tmp/heap_$$.pb.gz"
        echo ""
        echo "查看 Goroutine 分析:"
        echo "  go tool pprof -http=:8082 /tmp/goroutine_$$.pb.gz"
        ;;
    *)
        echo "无效选项"
        exit 1
        ;;
esac

echo ""
echo "=========================================="
echo "分析完成！"
echo "=========================================="
echo ""
echo "Web 界面已启动，请在浏览器中打开:"
echo "  http://localhost:8080"
echo ""
echo "常用操作:"
echo "  - 查看火焰图: 点击 'Flame Graph'"
echo "  - 查看调用图: 点击 'Graph'"
echo "  - 查看 Top 函数: 点击 'Top'"
echo ""
