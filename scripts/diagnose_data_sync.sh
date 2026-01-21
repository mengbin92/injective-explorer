#!/bin/bash

# 诊断脚本：检查数据同步问题
# 用于排查为什么只有blocks表有数据，其他表没有数据

echo "======================================"
echo "Injective Explorer 数据同步诊断工具"
echo "======================================"
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 从.env文件读取配置
if [ -f .env ]; then
    echo "✓ 读取.env配置文件"
    DB_HOST=$(grep DB_HOST .env | cut -d '=' -f2)
    DB_PORT=$(grep DB_PORT .env | cut -d '=' -f2)
    DB_USER=$(grep DB_USER .env | cut -d '=' -f2)
    DB_PASSWORD=$(grep DB_PASSWORD .env | cut -d '=' -f2)
    DB_NAME=$(grep DB_NAME .env | cut -d '=' -f2)
    REDIS_HOST=$(grep REDIS_HOST .env | cut -d '=' -f2)
    REDIS_PORT=$(grep REDIS_PORT .env | cut -d '=' -f2)
else
    echo -e "${RED}✗ .env文件不存在${NC}"
    exit 1
fi

# 1. 检查数据库连接
echo "======================================"
echo "1. 检查数据库连接"
echo "======================================"
MYSQL_CMD="mysql -h$DB_HOST -P$DB_PORT -u$DB_USER -p$DB_PASSWORD -D$DB_NAME -se"

# 测试数据库连接
if $MYSQL_CMD "SELECT 1" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 数据库连接成功${NC}"
else
    echo -e "${RED}✗ 数据库连接失败${NC}"
    echo "  请检查："
    echo "  - 数据库服务是否运行"
    echo "  - 网络连接是否正常"
    echo "  - 用户名密码是否正确"
    exit 1
fi
echo ""

# 2. 检查表是否存在和数据统计
echo "======================================"
echo "2. 检查数据库表和数据统计"
echo "======================================"

TABLES=("blocks" "transactions" "validators" "evm_transactions")
for table in "${TABLES[@]}"; do
    # 检查表是否存在
    EXISTS=$($MYSQL_CMD "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$DB_NAME' AND table_name='$table'")
    if [ "$EXISTS" -eq 1 ]; then
        # 统计行数
        COUNT=$($MYSQL_CMD "SELECT COUNT(*) FROM $table")
        printf "%-20s: %10d 行\n" "$table" "$COUNT"
    else
        echo -e "${YELLOW}⚠ 表 '$table' 不存在${NC}"
    fi
done
echo ""

# 3. 检查最新的区块和交易
echo "======================================"
echo "3. 检查最新数据"
echo "======================================"

LATEST_BLOCK=$($MYSQL_CMD "SELECT height FROM blocks ORDER BY height DESC LIMIT 1")
if [ -n "$LATEST_BLOCK" ]; then
    echo -e "${GREEN}最新区块高度: $LATEST_BLOCK${NC}"
else
    echo -e "${RED}没有找到区块数据${NC}"
fi

LATEST_TX=$($MYSQL_CMD "SELECT COUNT(*) FROM transactions")
echo "交易数量: $LATEST_TX"

# 检查最新区块是否有交易
if [ -n "$LATEST_BLOCK" ]; then
    BLOCK_TXS=$($MYSQL_CMD "SELECT num_txs FROM blocks WHERE height=$LATEST_BLOCK")
    echo "最新区块交易数: $BLOCK_TXS"
fi
echo ""

# 4. 检查Redis连接
echo "======================================"
echo "4. 检查Redis连接"
echo "======================================"

if command -v redis-cli &> /dev/null; then
    if redis-cli -h $REDIS_HOST -p $REDIS_PORT PING > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Redis连接成功${NC}"

        # 检查队列中的数据
        TX_QUEUE_COUNT=$(redis-cli -h $REDIS_HOST -p $REDIS_PORT LLEN "tx_hash_queue" 2>/dev/null)
        if [ -n "$TX_QUEUE_COUNT" ]; then
            echo "tx_hash_queue中的交易数: $TX_QUEUE_COUNT"
        fi

        TX_PROCESSING_COUNT=$(redis-cli -h $REDIS_HOST -p $REDIS_PORT LLEN "tx_hash_processing_queue" 2>/dev/null)
        if [ -n "$TX_PROCESSING_COUNT" ]; then
            echo "tx_hash_processing_queue中的交易数: $TX_PROCESSING_COUNT"
        fi

        EVM_QUEUE_COUNT=$(redis-cli -h $REDIS_HOST -p $REDIS_PORT LLEN "evm_details_queue" 2>/dev/null)
        if [ -n "$EVM_QUEUE_COUNT" ]; then
            echo "evm_details_queue中的任务数: $EVM_QUEUE_COUNT"
        fi
    else
        echo -e "${RED}✗ Redis连接失败${NC}"
        echo "  请检查："
        echo "  - Redis服务是否运行"
        echo "  - Redis主机和端口配置: $REDIS_HOST:$REDIS_PORT"
    fi
else
    echo -e "${YELLOW}⚠ redis-cli未安装，无法检查Redis${NC}"
fi
echo ""

# 5. 检查应用日志（最近的错误）
echo "======================================"
echo "5. 检查最近的应用日志（如有）"
echo "======================================"

if [ -f "logs/injective-explorer.log" ]; then
    echo "最近的错误日志:"
    tail -n 50 logs/injective-explorer.log | grep -i "error\|warn\|failed" | tail -n 10
elif [ -f "injective-explorer.log" ]; then
    echo "最近的错误日志:"
    tail -n 50 injective-explorer.log | grep -i "error\|warn\|failed" | tail -n 10
else
    echo -e "${YELLOW}未找到日志文件${NC}"
fi
echo ""

# 6. 给出诊断建议
echo "======================================"
echo "6. 诊断建议"
echo "======================================"

BLOCKS_COUNT=$($MYSQL_CMD "SELECT COUNT(*) FROM blocks")
TXS_COUNT=$($MYSQL_CMD "SELECT COUNT(*) FROM transactions")

if [ "$BLOCKS_COUNT" -gt 0 ] && [ "$TXS_COUNT" -eq 0 ]; then
    echo -e "${RED}问题：区块有数据但交易表为空${NC}"
    echo ""
    echo "可能原因："
    echo "1. Redis连接失败或未初始化"
    echo "2. 交易处理器（transaction processor）未启动"
    echo "3. Redis队列配置错误"
    echo ""
    echo "建议检查步骤："
    echo "1. 检查应用启动日志，确认 Redis 初始化成功"
    echo "2. 查找日志中的 'redis client is nil' 错误"
    echo "3. 查找日志中的 'transaction processor' 启动信息"
    echo "4. 检查 Redis 队列中是否有待处理的交易哈希"
    echo ""
    echo "快速修复命令："
    echo "  - 重启应用服务"
    echo "  - 检查.env中的Redis配置"
    echo "  - 确认Redis服务运行正常"
elif [ "$BLOCKS_COUNT" -eq 0 ]; then
    echo -e "${RED}问题：区块表也为空${NC}"
    echo "说明区块同步未开始或失败"
else
    echo -e "${GREEN}数据同步看起来正常${NC}"
fi

echo ""
echo "======================================"
echo "诊断完成"
echo "======================================"
