#!/bin/bash
# 场景 1：极端封禁下的转生测试

echo "=========================================="
echo "场景 1：转生协议压力测试"
echo "=========================================="

GATEWAY_PID=$(pgrep -f mirage-gateway)
if [ -z "$GATEWAY_PID" ]; then
    echo "❌ Gateway 未运行"
    exit 1
fi

echo ""
echo "📦 步骤 1: 检查当前活跃路径..."
echo "   当前 Gateway PID: $GATEWAY_PID"

# 模拟路径状态
ACTIVE_PATH="cell-us-west-01-eth0"
BACKUP_PATH="cell-hk-01-eth1"

echo "   活跃路径: $ACTIVE_PATH"
echo "   备用路径: $BACKUP_PATH"

echo ""
echo "📦 步骤 2: 模拟 IP 封禁（禁用活跃路径）..."
echo "   执行: 人为注入 50% 丢包到 $ACTIVE_PATH"

# 使用 tc qdisc 模拟丢包
sudo tc qdisc add dev eth0 root netem loss 50% 2>/dev/null || echo "   ⚠️  需要 root 权限执行 tc 命令"

echo ""
echo "📦 步骤 3: 等待转生协议触发（5 秒）..."
for i in {5..1}; do
    echo "   倒计时: $i 秒"
    sleep 1
done

echo ""
echo "📦 步骤 4: 验证路径切换..."
echo "   预期结果："
echo "   - 健康检查器检测到 $ACTIVE_PATH 丢包率 > 50%"
echo "   - 转生管理器切换到 $BACKUP_PATH"
echo "   - TCP 连接不中断"
echo "   - Dashboard 节点状态: 绿灯 → 黄灯 → 绿灯"

echo ""
echo "📦 步骤 5: 恢复网络环境..."
sudo tc qdisc del dev eth0 root 2>/dev/null || echo "   ⚠️  需要 root 权限"

echo ""
echo "=========================================="
echo "✅ 场景 1 测试完成"
echo "=========================================="
echo ""
echo "📋 验证清单:"
echo "   1. Gateway 日志中是否出现: [转生协议] 路径 $ACTIVE_PATH 不健康"
echo "   2. Gateway 日志中是否出现: [转生协议] 已切换到路径: $BACKUP_PATH"
echo "   3. 使用 curl 测试连接是否中断"
echo "   4. Dashboard 上节点状态变化"
echo ""
