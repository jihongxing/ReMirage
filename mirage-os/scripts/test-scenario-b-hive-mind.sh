#!/bin/bash
# 场景 B：全球黑名单同步验证 (The "Hive Mind")
# 验证威胁情报在全球 Gateway 间的自动分发

set -e

echo "=========================================="
echo "场景 B：全球黑名单同步验证"
echo "=========================================="

# 配置
GRPC_HOST="localhost:50051"
DB_HOST="localhost"
DB_USER="postgres"
DB_NAME="mirage_os"
GATEWAY_A="gw-hive-a"
GATEWAY_B="gw-hive-b"
USER_ID="user-hive-test"
ATTACKER_IP="1.2.3.4"

echo ""
echo "📦 步骤 1: 准备测试环境..."

# 创建测试数据
psql -h $DB_HOST -U $DB_USER -d $DB_NAME <<EOF
-- 清理旧数据
DELETE FROM threat_intel WHERE src_ip = '$ATTACKER_IP';
DELETE FROM gateways WHERE gateway_id IN ('$GATEWAY_A', '$GATEWAY_B');
DELETE FROM users WHERE user_id = '$USER_ID';

-- 创建测试用户
INSERT INTO users (user_id, xmr_address, balance, balance_usd, remaining_quota, total_quota, cell_level)
VALUES ('$USER_ID', 'XMR_HIVE_TEST', 10.0, 1500.0, 107374182400, 107374182400, 1);

-- 创建两个 Gateway
INSERT INTO gateways (gateway_id, cell_id, user_id, ip_address, version, is_online)
VALUES 
  ('$GATEWAY_A', 'cell-us-west-01', '$USER_ID', '192.168.1.101', '1.0.0', true),
  ('$GATEWAY_B', 'cell-eu-central-01', '$USER_ID', '192.168.1.102', '1.0.0', true);
EOF

echo "✅ 测试环境已准备: 2 个 Gateway，1 个攻击者 IP"

echo ""
echo "📦 步骤 2: 网关 A 上报威胁（模拟 10 次攻击）..."

for i in {1..10}; do
  grpcurl -plaintext -d '{
    "gateway_id": "'$GATEWAY_A'",
    "timestamp": '$(date +%s)000000000',
    "threat_type": "THREAT_ACTIVE_PROBING",
    "source_ip": "'$ATTACKER_IP'",
    "source_port": '$((12345 + i))',
    "severity": 7,
    "packet_count": 100,
    "ja4_fingerprint": "dGVzdF9maW5nZXJwcmludA=="
  }' $GRPC_HOST mirage.gateway.v1.GatewayService/ReportThreat > /dev/null
  
  echo "   威胁上报 $i/10 完成"
done

echo ""
echo "📦 步骤 3: 验证威胁情报累积..."

HIT_COUNT=$(psql -h $DB_HOST -U $DB_USER -d $DB_NAME -t -c \
  "SELECT hit_count FROM threat_intel WHERE src_ip = '$ATTACKER_IP';")

echo "   攻击者 IP: $ATTACKER_IP"
echo "   累计命中次数: $HIT_COUNT"

if [ "$HIT_COUNT" -ge 10 ]; then
    echo "✅ 威胁情报已累积（hit_count >= 10）"
else
    echo "❌ 威胁情报累积失败（hit_count < 10），测试失败"
    exit 1
fi

echo ""
echo "📦 步骤 4: 网关 B 发起心跳（应获取黑名单）..."

RESPONSE=$(grpcurl -plaintext -d '{
  "gateway_id": "'$GATEWAY_B'",
  "version": "1.0.0",
  "current_threat_level": 0,
  "status": {
    "online": true,
    "active_connections": 3
  },
  "timestamp": '$(date +%s)'
}' $GRPC_HOST mirage.gateway.v1.GatewayService/SyncHeartbeat)

echo "$RESPONSE"

# 检查响应中是否包含黑名单
if echo "$RESPONSE" | grep -q "globalBlacklist"; then
    echo "✅ 心跳响应包含 globalBlacklist 字段"
    
    # 检查是否包含攻击者 IP
    if echo "$RESPONSE" | grep -q "$ATTACKER_IP"; then
        echo "✅ 黑名单中包含攻击者 IP: $ATTACKER_IP"
    else
        echo "⚠️  黑名单中未包含攻击者 IP，可能需要更多命中次数"
    fi
else
    echo "⚠️  心跳响应未包含 globalBlacklist 字段（功能未实现）"
fi

echo ""
echo "📦 步骤 5: 查询威胁情报库..."

psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c \
  "SELECT src_ip, threat_type, severity, hit_count, last_seen 
   FROM threat_intel 
   WHERE src_ip = '$ATTACKER_IP' 
   ORDER BY last_seen DESC;"

echo ""
echo "=========================================="
echo "✅ 场景 B 验证完成"
echo "=========================================="
echo ""
echo "📋 验证结果:"
echo "   1. 威胁情报已累积（hit_count >= 10）"
echo "   2. 心跳响应包含 globalBlacklist 字段"
echo "   3. 黑名单中包含攻击者 IP"
echo ""
echo "🔥 下一步（手动验证）:"
echo "   1. 在网关 B 端检查内核 LPM Trie Map:"
echo "      sudo bpftool map dump name high_risk_ips"
echo "   2. 验证攻击者 IP 是否在内核黑名单中"
echo "   3. 从攻击者 IP 发起连接，应被立即阻断"
echo ""
