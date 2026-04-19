#!/bin/bash
# 配额熔断测试脚本

echo "=========================================="
echo "配额熔断功能测试"
echo "=========================================="

if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 root 权限运行: sudo $0"
    exit 1
fi

# 1. 启动 Gateway（后台运行）
echo ""
echo "📦 步骤 1: 启动 Gateway..."
./bin/mirage-gateway -iface eth0 -defense 20 -traffic 100 &
GATEWAY_PID=$!

echo "   Gateway PID: $GATEWAY_PID"
sleep 3

# 2. 检查初始流量统计
echo ""
echo "📦 步骤 2: 检查初始流量统计..."
echo "   发送 10 个 ping 包..."
ping -c 10 8.8.8.8 > /dev/null 2>&1

sleep 2

# 3. 模拟配额耗尽
echo ""
echo "📦 步骤 3: 模拟配额耗尽..."
echo "   ⚠️  将配额设置为 0（触发熔断）"

# 使用 bpftool 修改 quota_map（如果可用）
if command -v bpftool &> /dev/null; then
    # 查找 quota_map 的 ID
    MAP_ID=$(bpftool map list | grep quota_map | awk '{print $1}' | cut -d: -f1)
    if [ -n "$MAP_ID" ]; then
        echo "   找到 quota_map ID: $MAP_ID"
        # 设置配额为 0
        bpftool map update id $MAP_ID key 0 0 0 0 value 0 0 0 0 0 0 0 0
        echo "   ✅ 配额已设置为 0"
    else
        echo "   ⚠️  未找到 quota_map"
    fi
else
    echo "   ⚠️  bpftool 不可用，跳过配额修改"
fi

# 4. 测试熔断效果
echo ""
echo "📦 步骤 4: 测试熔断效果..."
echo "   发送 10 个 ping 包（应该被阻断）..."
ping -c 10 -W 2 8.8.8.8

# 5. 恢复配额
echo ""
echo "📦 步骤 5: 恢复配额..."
if [ -n "$MAP_ID" ]; then
    # 设置配额为 100GB
    bpftool map update id $MAP_ID key 0 0 0 0 value 0 0 0 e8 d4 a5 10 0
    echo "   ✅ 配额已恢复为 100GB"
fi

# 6. 验证恢复
echo ""
echo "📦 步骤 6: 验证恢复..."
echo "   发送 10 个 ping 包（应该正常）..."
ping -c 10 8.8.8.8

# 7. 停止 Gateway
echo ""
echo "📦 步骤 7: 停止 Gateway..."
kill -SIGINT $GATEWAY_PID
wait $GATEWAY_PID 2>/dev/null

echo ""
echo "=========================================="
echo "✅ 测试完成"
echo "=========================================="
echo ""
echo "📋 预期结果："
echo "   1. 初始状态：ping 正常，有延迟抖动"
echo "   2. 配额耗尽：ping 失败或 100% 丢包"
echo "   3. 配额恢复：ping 恢复正常"
echo ""
echo "📊 查看日志："
echo "   - 应该看到 '🚨 [计费] 配额已耗尽，服务已熔断！'"
echo "   - 应该看到 '💰 [计费] 配额已更新: XXX GB'"
echo ""
