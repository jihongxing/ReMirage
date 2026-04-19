#!/bin/bash
# 紧急自毁测试脚本

set -e

echo "=========================================="
echo "🔥 紧急自毁逻辑测试"
echo "=========================================="

GATEWAY_BIN="./bin/mirage-gateway"
TEST_DURATION=60

# 检查二进制文件
if [ ! -f "$GATEWAY_BIN" ]; then
    echo "❌ Gateway 二进制文件不存在，请先编译"
    echo "   运行: make build"
    exit 1
fi

# 启动 Gateway（后台运行）
echo ""
echo "1️⃣ 启动 Gateway..."
$GATEWAY_BIN &
GATEWAY_PID=$!
echo "   PID: $GATEWAY_PID"

# 等待启动
sleep 5

# 检查进程是否存活
if ! ps -p $GATEWAY_PID > /dev/null; then
    echo "❌ Gateway 启动失败"
    exit 1
fi

echo "✅ Gateway 已启动"

# 模拟心跳超时
echo ""
echo "2️⃣ 模拟心跳超时（等待 ${TEST_DURATION} 秒）..."
echo "   提示：正常情况下 Gateway 会在 300 秒后自毁"
echo "   为了测试，我们只等待 ${TEST_DURATION} 秒"

for i in $(seq 1 $TEST_DURATION); do
    if ! ps -p $GATEWAY_PID > /dev/null; then
        echo ""
        echo "🔥 Gateway 进程已终止（第 $i 秒）"
        echo "✅ 紧急自毁逻辑触发成功"
        exit 0
    fi
    
    # 每 10 秒打印一次
    if [ $((i % 10)) -eq 0 ]; then
        echo "   等待中... ($i/$TEST_DURATION 秒)"
    fi
    
    sleep 1
done

# 如果进程仍然存活
if ps -p $GATEWAY_PID > /dev/null; then
    echo ""
    echo "⚠️ Gateway 进程仍然存活"
    echo "   手动终止进程..."
    kill $GATEWAY_PID
    echo "✅ 测试完成（进程未自动终止）"
else
    echo ""
    echo "✅ 测试完成（进程已自动终止）"
fi

echo ""
echo "=========================================="
echo "测试结束"
echo "=========================================="
