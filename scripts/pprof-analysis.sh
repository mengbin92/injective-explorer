#!/bin/bash
# pprof 性能分析脚本
# 用于分析 injective-explorer 的性能瓶颈

set -e

# 检查 graphviz（可选，仅用于命令行图形生成）
check_graphviz() {
    if ! command -v dot &> /dev/null; then
        echo "⚠️  警告: 未检测到 graphviz (dot 命令)"
        echo "   这将影响命令行图形生成（web/png 命令）"
        echo "   但 Web 界面（-http 参数）仍然可以正常使用"
        echo ""
        echo "   安装 graphviz:"
        echo "   macOS:    brew install graphviz"
        echo "   Ubuntu:   sudo apt-get install graphviz"
        echo "   CentOS:   sudo yum install graphviz"
        echo ""
    fi
}

# 配置
PPROF_HOST="${PPROF_HOST:-localhost:6060}"
PROFILE_DURATION="${PROFILE_DURATION:-30}"
OUTPUT_DIR="${OUTPUT_DIR:-./pprof-reports}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

echo "=========================================="
echo "pprof 性能分析工具"
echo "=========================================="
echo "目标服务器: $PPROF_HOST"
echo "分析时长: ${PROFILE_DURATION}秒"
echo "输出目录: $OUTPUT_DIR"
echo "=========================================="
echo ""

# 检查 graphviz
check_graphviz

# 检查服务器是否可访问
echo "检查服务器连接..."
if ! curl -s "http://$PPROF_HOST/debug/pprof/" > /dev/null; then
    echo "错误: 无法连接到 $PPROF_HOST/debug/pprof/"
    echo "请确保:"
    echo "  1. 服务器正在运行"
    echo "  2. 已设置环境变量 ENABLE_PPROF=true"
    echo "  3. 服务器监听在 $PPROF_HOST"
    exit 1
fi
echo "✓ 服务器连接正常"
echo ""

# 函数：生成性能报告
generate_report() {
    local profile_type=$1
    local endpoint=$2
    local output_file="$OUTPUT_DIR/${profile_type}_${TIMESTAMP}.pb.gz"
    
    echo "正在生成 $profile_type 性能分析..."
    
    if [ "$profile_type" = "cpu" ]; then
        # CPU profile 需要指定时长
        go tool pprof -proto -output="$output_file" "http://$PPROF_HOST/debug/pprof/profile?seconds=$PROFILE_DURATION" &
        local pprof_pid=$!
        echo "  采集 CPU 数据中... (${PROFILE_DURATION}秒)"
        wait $pprof_pid
    else
        go tool pprof -proto -output="$output_file" "http://$PPROF_HOST/debug/pprof/$endpoint"
    fi
    
    if [ -f "$output_file" ]; then
        echo "  ✓ 已保存到: $output_file"
        
        # 生成文本报告
        local txt_file="${output_file%.pb.gz}.txt"
        go tool pprof -top -cum "$output_file" > "$txt_file" 2>&1 || true
        echo "  ✓ 文本报告: $txt_file"
    else
        echo "  ✗ 生成失败"
    fi
    echo ""
}

# 生成各种性能分析报告
echo "开始性能分析..."
echo ""

# 1. CPU 性能分析
generate_report "cpu" "profile"

# 2. 内存分析
generate_report "heap" "heap"

# 3. Goroutine 分析
generate_report "goroutine" "goroutine"

# 4. 阻塞分析
generate_report "block" "block"

# 5. 互斥锁分析
generate_report "mutex" "mutex"

# 生成汇总报告
echo "生成汇总报告..."
SUMMARY_FILE="$OUTPUT_DIR/summary_${TIMESTAMP}.md"
cat > "$SUMMARY_FILE" << EOF
# 性能分析报告

**生成时间**: $(date)
**服务器**: $PPROF_HOST
**分析时长**: ${PROFILE_DURATION}秒

## 报告文件

- CPU 分析: \`cpu_${TIMESTAMP}.pb.gz\`
- 内存分析: \`heap_${TIMESTAMP}.pb.gz\`
- Goroutine 分析: \`goroutine_${TIMESTAMP}.pb.gz\`
- 阻塞分析: \`block_${TIMESTAMP}.pb.gz\`
- 互斥锁分析: \`mutex_${TIMESTAMP}.pb.gz\`

## 查看报告

### 交互式查看
\`\`\`bash
go tool pprof $OUTPUT_DIR/cpu_${TIMESTAMP}.pb.gz
\`\`\`

### 生成火焰图
\`\`\`bash
go tool pprof -http=:8080 $OUTPUT_DIR/cpu_${TIMESTAMP}.pb.gz
\`\`\`
然后在浏览器中打开 http://localhost:8080

### 查看文本报告
\`\`\`bash
cat $OUTPUT_DIR/cpu_${TIMESTAMP}.txt
\`\`\`

## 常用命令

### 查看 top 10 最耗时的函数
\`\`\`bash
go tool pprof -top $OUTPUT_DIR/cpu_${TIMESTAMP}.pb.gz
\`\`\`

### 查看调用图
\`\`\`bash
go tool pprof -web $OUTPUT_DIR/cpu_${TIMESTAMP}.pb.gz
\`\`\`

### 查看特定函数
\`\`\`bash
go tool pprof $OUTPUT_DIR/cpu_${TIMESTAMP}.pb.gz
(pprof) list functionName
\`\`\`
EOF

echo "✓ 汇总报告: $SUMMARY_FILE"
echo ""

echo "=========================================="
echo "性能分析完成！"
echo "=========================================="
echo "报告保存在: $OUTPUT_DIR"
echo ""
echo "快速查看 CPU 分析:"
echo "  go tool pprof -http=:8080 $OUTPUT_DIR/cpu_${TIMESTAMP}.pb.gz"
echo ""
echo "查看汇总报告:"
echo "  cat $SUMMARY_FILE"
echo ""
