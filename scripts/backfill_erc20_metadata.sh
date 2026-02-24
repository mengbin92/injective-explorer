#!/bin/bash
# ERC20 元数据回填脚本（设计文档 4.2.2 方案 B）
# 包装 go run ./cmd/backfill-metadata
#
# 用法：
#   ./scripts/backfill_erc20_metadata.sh [options]
#   ./scripts/backfill_erc20_metadata.sh -limit 100 -delay 500
#   ./scripts/backfill_erc20_metadata.sh -dry-run
#
# 参数（与 cmd/backfill-metadata 一致）：
#   -conf     配置路径（默认 ./configs）
#   -limit    每批处理数量（默认 50）
#   -delay    每个合约间隔毫秒，RPC 限速（默认 0）
#   -chain-id 链 ID（可选）
#   -dry-run  模拟不实际刷新

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

CONF="${CONF:-./configs}"

# -batch-size 作为 -limit 的别名以兼容设计文档表述
ARGS=()
while [[ $# -gt 0 ]]; do
  case $1 in
    -batch-size)
      ARGS+=("-limit" "$2")
      shift 2
      ;;
    *)
      ARGS+=("$1")
      shift
      ;;
  esac
done

go run ./cmd/backfill-metadata -conf "$CONF" "${ARGS[@]}"
