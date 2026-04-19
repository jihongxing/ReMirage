#!/bin/bash
# 重新编译并测试计费功能

echo "=========================================="
echo "重新编译并测试计费功能"
echo "=========================================="

# 1. 清理并编译
echo ""
echo "📦 步骤 1: 清理并编译..."
make clean
make all

if [ $? -ne 0 ]; then
    echo "❌ 编译失败"
    exit 1
fi

echo "✅ 编译成功"

# 2. 检查 Map 是否存在于 eBPF 对象文件
echo ""
echo "📦 步骤 2: 检查 eBPF Map..."
if command -v llvm-objdump &> /dev/null; then
    echo "   检查 traffic_stats:"
    llvm-objdump -t bpf/jitter.o | grep traffic_stats || echo "   ⚠️  未找到"
    
    echo "   检查 quota_map:"
    llvm-objdump -t bpf/jitter.o | grep quota_map || echo "   ⚠️  未找到"
else
    echo "   ⚠️  llvm-objdump 不可用，跳过检查"
fi

# 3. 启动 Gateway（后台）
echo ""
echo "📦 步骤 3: 启动 Gateway（后台）..."
sudo ./bin/mirage-gateway -iface eth0 -defense 20 -traffic 100 > /tmp/gateway.log 2>&1 &
GATEWAY_PID=$!

echo "   Gateway PID: $GATEWAY_PID"
sleep 3

# 4. 检查日志
echo ""
echo "📦 步骤 4: 检查日志..."
echo "   前 20 行日志："
head -20 /tmp/gateway.log

echo ""
echo "   等待 15 秒，观察计费日志..."
sleep 15

echo ""
echo "   最新日志："
tail -10 /tmp/gateway.log

# 5. 检查是否有错误
echo ""
echo "📦 步骤 5: 检查错误..."
if grep -q "traffic_stats Map 不存在" /tmp/gateway.log; then
    echo "   ❌ traffic_stats Map 仍然不存在"
else
    echo "   ✅ traffic_stats Map 正常"
fi

if grep -q "quota_map 不存在" /tmp/gateway.log; then
    echo "   ❌ quota_map 仍然不存在"
else
    echo "   ✅ quota_map 正常"
fi

if grep -q "📊 \[计费\] 上报流量" /tmp/gateway.log; then
    echo "   ✅ 计费上报正常"
    grep "📊 \[计费\] 上报流量" /tmp/gateway.log | tail -1
else
    echo "   ⚠️  未看到计费上报"
fi

# 6. 停止 Gateway
echo ""
echo "📦 步骤 6: 停止 Gateway..."
sudo kill -SIGINT $GATEWAY_PID
wait $GATEWAY_PID 2>/dev/null

echo ""
echo "=========================================="
echo "测试完成"
echo "=========================================="
echo ""
echo "完整日志: /tmp/gateway.log"
echo ""
