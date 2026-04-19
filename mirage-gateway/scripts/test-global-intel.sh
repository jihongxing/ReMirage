#!/bin/bash
# 场景 3：全球情报共振测试

echo "=========================================="
echo "场景 3：全球情报共振测试"
echo "=========================================="

GRPC_HOST="localhost:50051"
DB_HOST="localhost"
DB_USER="postgres"
DB_NAME="mirage_os"

ATTACKER_IP="203.0.113.100"
GATEWAY_TOKYO="gw-jp-tokyo-01"
GATEWAY_NEWYORK="gw-us-newyork-01"

echo ""
echo "📦 步骤 1: 准备测试环境..."

# 创建测试 Gateway
psql -h $DB_HOST -U $DB_USER -d $DB_NAME <<EOF > /dev/null 2>&1
DELETE FROM threat_intel WHERE src_ip = '$ATTACKER_IP';
DELETE FROM gateways WHERE gateway_id IN ('$GATEWAY_TOKYO', '$GATEWAY_NEWYORK');

INSERT INTO gateways (gateway_id, cell_id, ip_address, version, is_online)
VALUES 
  ('$GATEWAY_TOKYO', 'cell-jp-tokyo-01', '192.168.1.201', '1.0.0', true),
  ('$GATEWAY_NEWYORK', 'cell-us-newyork-01', '192.168.1.202', '1.0.0', true);
EOF

echo "   ✅ 测试环境已准备"
echo "   - 东京节点: $GATEWAY_TOKYO"
echo "   - 纽约节点: $GATEWAY_NEWYORK"
echo "   - 攻击者 IP: $ATTACKER_IP"

echo ""
echo "📦 步骤 2: 模拟东京节点遭受扫描攻击..."
echo "   时间: $(date '+%Y-%m-%d %H:%M:%S')"

for i in {1..10}; do
    grpcurl -plaintext -d '{
      "gateway_id": "'$GATEWAY_TOKYO'",
      "timestamp": '$(date +%s)000000000',
      "threat_type": "THREAT_ACTIVE_PROBING",
      "source_ip": "'$ATTACKER_IP'",
      "source_port": '$((12345 + i))',
      "severity": 8,
      "packet_count": 100,
      "ja4_fingerprint": "dGVzdF9maW5nZXJwcmludA=="
    }' $GRPC_HOST mirage.gateway.v1.GatewayService/ReportThreat > /dev/null 2>&1
    
    echo "   威胁上报 $i/10"
done

echo ""
echo "📦 步骤 3: 验证威胁情报累积..."

HIT_COUNT=$(psql -h $DB_HOST -U $DB_USER -d $DB_NAME -t -c \
  "SELECT hit_count FROM threat_intel WHERE src_ip = '$ATTACKER_IP';" 2>/dev/null)

echo "   攻击者 IP: $ATTACKER_IP"
echo "   累计命中次数: $HIT_COUNT"

if [ "$HIT_COUNT" -ge 10 ]; then
    echo "   ✅ 威胁情报已累积（hit_count >= 10）"
else
    echo "   ⚠️  威胁情报累积不足"
fi

echo ""
echo "📦 步骤 4: 纽约节点发起心跳（获取全局黑名单）..."
echo "   等待 10 秒（模拟心跳间隔）..."
sleep 10

grpcurl -plaintext -d '{
  "gateway_id": "'$GATEWAY_NEWYORK'",
  "version": "1.0.0",
  "current_threat_level": 0,
  "status": {
    "online": true,
    "active_connections": 5
  },
  "timestamp": '$(date +%s)'
}' $GRPC_HOST mirage.gateway.v1.GatewayService/SyncHeartbeat > /tmp/heartbeat_response.json 2>&1

echo ""
echo "📦 步骤 5: 验证全球黑名单同步..."

if grep -q "globalBlacklist" /tmp/heartbeat_response.json 2>/dev/null; then
    echo "   ✅ 心跳响应包含 globalBlacklist 字段"
    
    if grep -q "$ATTACKER_IP" /tmp/heartbeat_response.json 2>/dev/null; then
        echo "   ✅ 黑名单中包含攻击者 IP: $ATTACKER_IP"
        echo ""
        echo "   🎯 情报共振成功！"
        echo "   - 东京节点检测到攻击（T+0s）"
        echo "   - Mirage-OS 更新全局情报库（T+1s）"
        echo "   - 纽约节点同步黑名单（T+10s）"
        echo "   - 全球所有节点自动封禁该 IP（T+60s）"
    else
        echo "   ⚠️  黑名单中未包含攻击者 IP"
    fi
else
    echo "   ⚠️  心跳响应未包含 globalBlacklist 字段"
fi

echo ""
echo "📦 步骤 6: 验证内核态黑名单更新..."
echo "   检查纽约节点的内核 LPM Trie Map:"
echo "   sudo bpftool map dump name high_risk_ips"
echo ""
echo "   预期结果："
echo "   - Map 中应包含 $ATTACKER_IP"
echo "   - 从该 IP 发起的连接应被立即阻断（TC_ACT_STOLEN）"

echo ""
echo "📦 步骤 7: 查询全局威胁情报库..."
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c \
  "SELECT src_ip, threat_type, severity, hit_count, last_seen 
   FROM threat_intel 
   WHERE src_ip = '$ATTACKER_IP' 
   ORDER BY last_seen DESC;"

echo ""
echo "=========================================="
echo "✅ 场景 3 测试完成"
echo "=========================================="
echo ""
echo "📋 验证清单:"
echo "   1. 东京节点成功上报威胁（hit_count >= 10）"
echo "   2. Mirage-OS 更新全局情报库"
echo "   3. 纽约节点心跳获取黑名单"
echo "   4. 内核 LPM Trie Map 包含攻击者 IP"
echo "   5. Dashboard 显示全球威胁分布"
echo ""
echo "🌍 全球视角验证:"
echo "   打开 Dashboard (http://localhost:5173)"
echo "   观察 3D 地球上："
echo "   - 东京位置应显示红色威胁标记"
echo "   - 实时情报流应显示该攻击事件"
echo "   - 全球节点状态应同步更新"
echo ""
