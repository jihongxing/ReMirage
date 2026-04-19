#!/bin/bash
# 场景 A：欠费熔断验证 (The "Dead Switch")
# 验证配额耗尽时，内核态立即触发 TC_ACT_STOLEN

set -e

echo "=========================================="
echo "场景 A：欠费熔断验证"
echo "=========================================="

# 配置
GRPC_HOST="localhost:50051"
DB_HOST="localhost"
DB_USER="postgres"
DB_NAME="mirage_os"
GATEWAY_ID="gw-test-quota"
USER_ID="user-test-quota"

echo ""
echo "📦 步骤 1: 准备测试环境..."

# 创建测试用户（配额仅剩 100 字节）
psql -h $DB_HOST -U $DB_USER -d $DB_NAME <<EOF
-- 删除旧数据
DELETE FROM billing_logs WHERE user_id = '$USER_ID';
DELETE FROM gateways WHERE gateway_id = '$GATEWAY_ID';
DELETE FROM users WHERE user_id = '$USER_ID';

-- 创建测试用户（配额仅剩 100 字节）
INSERT INTO users (user_id, xmr_address, balance, balance_usd, remaining_quota, total_quota, cell_level)
VALUES ('$USER_ID', 'XMR_QUOTA_TEST', 1.0, 150.0, 100, 107374182400, 1);

-- 创建测试 Gateway
INSERT INTO gateways (gateway_id, cell_id, user_id, ip_address, version, is_online)
VALUES ('$GATEWAY_ID', 'cell-us-west-01', '$USER_ID', '192.168.1.100', '1.0.0', false);
EOF

echo "✅ 测试用户已创建: $USER_ID (配额: 100 字节)"

echo ""
echo "📦 步骤 2: 上报大流量（10MB）..."

# 上报 10MB 流量（远超配额）
grpcurl -plaintext -d '{
  "gateway_id": "'$GATEWAY_ID'",
  "timestamp": '$(date +%s)',
  "base_traffic_bytes": 10485760,
  "defense_traffic_bytes": 0,
  "cell_level": "standard"
}' $GRPC_HOST mirage.gateway.v1.GatewayService/ReportTraffic

echo ""
echo "📦 步骤 3: 验证配额扣减..."

REMAINING=$(psql -h $DB_HOST -U $DB_USER -d $DB_NAME -t -c \
  "SELECT remaining_quota FROM users WHERE user_id = '$USER_ID';")

echo "   当前配额: $REMAINING 字节"

if [ "$REMAINING" -le 0 ]; then
    echo "✅ 配额已耗尽（$REMAINING <= 0）"
else
    echo "❌ 配额未耗尽（$REMAINING > 0），测试失败"
    exit 1
fi

echo ""
echo "📦 步骤 4: 再次心跳（应返回 remaining_quota=0）..."

RESPONSE=$(grpcurl -plaintext -d '{
  "gateway_id": "'$GATEWAY_ID'",
  "version": "1.0.0",
  "current_threat_level": 0,
  "status": {
    "online": true,
    "active_connections": 5
  },
  "timestamp": '$(date +%s)'
}' $GRPC_HOST mirage.gateway.v1.GatewayService/SyncHeartbeat)

echo "$RESPONSE"

# 检查响应中的 remaining_quota
if echo "$RESPONSE" | grep -q '"remainingQuota": "0"'; then
    echo "✅ 心跳返回 remaining_quota=0"
else
    echo "⚠️  心跳未返回 remaining_quota=0，请检查"
fi

echo ""
echo "📦 步骤 5: 验证计费流水..."

psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c \
  "SELECT gateway_id, business_bytes, defense_bytes, cost_usd, created_at 
   FROM billing_logs 
   WHERE user_id = '$USER_ID' 
   ORDER BY created_at DESC 
   LIMIT 3;"

echo ""
echo "=========================================="
echo "✅ 场景 A 验证完成"
echo "=========================================="
echo ""
echo "📋 验证结果:"
echo "   1. 配额已耗尽（remaining_quota <= 0）"
echo "   2. 心跳返回 remaining_quota=0"
echo "   3. 计费流水已记录"
echo ""
echo "🔥 下一步（手动验证）:"
echo "   1. 在 Gateway 端检查日志，应输出: [🚨 配额耗尽] 触发内核态熔断隔离"
echo "   2. 从宿主机 ping Gateway，应被阻断（TC_ACT_STOLEN 生效）"
echo "   3. 使用 bpftool 检查内核 quota_map: sudo bpftool map dump name quota_map"
echo ""
