#!/bin/bash
# WebSocket 实时推送测试脚本

echo "=========================================="
echo "Mirage-OS WebSocket 实时推送测试"
echo "=========================================="

# 检查依赖
if ! command -v redis-cli &> /dev/null; then
    echo "❌ redis-cli 未安装"
    exit 1
fi

echo ""
echo "📦 步骤 1: 检查 Redis 服务..."
if ! redis-cli ping &> /dev/null; then
    echo "❌ Redis 未运行"
    exit 1
fi
echo "✅ Redis 运行正常"

echo ""
echo "📦 步骤 2: 检查 WebSocket 服务..."
if ! nc -z localhost 8080 2>/dev/null; then
    echo "❌ WebSocket Gateway 未运行（localhost:8080）"
    echo "   请先启动: cd mirage-os && go run services/ws-gateway/main.go"
    exit 1
fi
echo "✅ WebSocket Gateway 运行正常"

echo ""
echo "📦 步骤 3: 发送测试事件..."

# 测试威胁事件（纽约）
echo "   发送威胁事件（纽约）..."
redis-cli PUBLISH mirage:events:all '{
  "type": "threat",
  "timestamp": '$(date +%s)',
  "data": {
    "lat": 40.71,
    "lng": -74.00,
    "intensity": 8,
    "label": "USA-NewYork",
    "srcIp": "192.168.1.100",
    "threatType": "ACTIVE_PROBING"
  }
}'

sleep 1

# 测试威胁事件（伦敦）
echo "   发送威胁事件（伦敦）..."
redis-cli PUBLISH mirage:events:all '{
  "type": "threat",
  "timestamp": '$(date +%s)',
  "data": {
    "lat": 51.51,
    "lng": -0.13,
    "intensity": 6,
    "label": "UK-London",
    "srcIp": "10.0.0.50",
    "threatType": "DPI_INSPECTION"
  }
}'

sleep 1

# 测试流量事件
echo "   发送流量事件..."
redis-cli PUBLISH mirage:events:all '{
  "type": "traffic",
  "timestamp": '$(date +%s)',
  "data": {
    "gatewayId": "gw-us-west-01",
    "lat": 34.05,
    "lng": -118.24,
    "businessBytes": 10485760,
    "defenseBytes": 5242880
  }
}'

sleep 1

# 测试配额事件
echo "   发送配额事件..."
redis-cli PUBLISH mirage:events:all '{
  "type": "quota",
  "timestamp": '$(date +%s)',
  "data": {
    "userId": "user-test-001",
    "remainingBytes": 1073741824,
    "totalBytes": 10737418240,
    "usagePercent": 90
  }
}'

echo ""
echo "=========================================="
echo "✅ 测试完成"
echo "=========================================="
echo ""
echo "📋 验证清单:"
echo "   1. 打开 Dashboard (http://localhost:5173)"
echo "   2. 观察 3D 地球上是否出现红色闪烁（纽约和伦敦）"
echo "   3. 观察情报流是否显示新的威胁事件"
echo "   4. 观察流量图表是否更新"
echo ""
echo "🔥 持续测试（每 3 秒发送一次）:"
echo "   while true; do"
echo "     redis-cli PUBLISH mirage:events:all '{\"type\":\"threat\",\"timestamp\":'$(date +%s)',\"data\":{\"lat\":40.71,\"lng\":-74.00,\"intensity\":8,\"label\":\"Test\",\"srcIp\":\"1.2.3.4\",\"threatType\":\"ACTIVE_PROBING\"}}'"
echo "     sleep 3"
echo "   done"
echo ""
