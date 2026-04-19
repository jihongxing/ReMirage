#!/bin/bash
# Mirage-OS 心跳测试脚本

echo "=========================================="
echo "Mirage-OS 心跳功能测试"
echo "=========================================="

# 检查依赖
if ! command -v grpcurl &> /dev/null; then
    echo "❌ grpcurl 未安装"
    echo "   安装方法: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

if ! command -v psql &> /dev/null; then
    echo "❌ psql 未安装"
    exit 1
fi

# 配置
GRPC_HOST="localhost:50051"
DB_HOST="localhost"
DB_USER="postgres"
DB_NAME="mirage_os"

echo ""
echo "📦 步骤 1: 初始化数据库..."
psql -h $DB_HOST -U $DB_USER -d postgres -c "DROP DATABASE IF EXISTS $DB_NAME;"
psql -h $DB_HOST -U $DB_USER -d postgres -c "CREATE DATABASE $DB_NAME;"
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -f scripts/init.sql

echo ""
echo "📦 步骤 2: 创建测试用户..."
psql -h $DB_HOST -U $DB_USER -d $DB_NAME <<EOF
INSERT INTO users (user_id, xmr_address, balance, balance_usd, remaining_quota, total_quota, cell_level)
VALUES ('test-user-001', 'XMR_TEST_ADDRESS', 10.0, 1500.0, 107374182400, 107374182400, 1);
EOF

echo ""
echo "📦 步骤 3: 创建测试 Gateway..."
psql -h $DB_HOST -U $DB_USER -d $DB_NAME <<EOF
INSERT INTO gateways (gateway_id, cell_id, user_id, ip_address, version, is_online)
VALUES ('gw-test-001', 'cell-us-west-01', 'test-user-001', '192.168.1.100', '1.0.0', false);
EOF

echo ""
echo "📦 步骤 4: 启动 API Gateway 服务..."
echo "   请在另一个终端运行: cd mirage-os && go run services/api-gateway/main.go"
echo "   按 Enter 继续..."
read

echo ""
echo "📦 步骤 5: 测试心跳同步..."
grpcurl -plaintext -d '{
  "gateway_id": "gw-test-001",
  "version": "1.0.0",
  "current_threat_level": 2,
  "status": {
    "online": true,
    "active_connections": 10
  },
  "resource": {
    "cpu_percent": 25.5,
    "memory_bytes": 1073741824,
    "bandwidth_bps": 10485760
  },
  "timestamp": '$(date +%s)'
}' $GRPC_HOST mirage.gateway.v1.GatewayService/SyncHeartbeat

echo ""
echo "📦 步骤 6: 验证数据库更新..."
echo "   检查 Gateway 状态:"
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT gateway_id, is_online, last_heartbeat_at, current_threat_level FROM gateways WHERE gateway_id = 'gw-test-001';"

echo ""
echo "📦 步骤 7: 测试流量上报（1GB）..."
grpcurl -plaintext -d '{
  "gateway_id": "gw-test-001",
  "timestamp": '$(date +%s)',
  "base_traffic_bytes": 1073741824,
  "defense_traffic_bytes": 0,
  "cell_level": "standard"
}' $GRPC_HOST mirage.gateway.v1.GatewayService/ReportTraffic

echo ""
echo "📦 步骤 8: 验证配额扣减..."
echo "   检查用户配额:"
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT user_id, remaining_quota, total_quota FROM users WHERE user_id = 'test-user-001';"

echo "   检查计费流水:"
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT gateway_id, business_bytes, defense_bytes, cost_usd, created_at FROM billing_logs ORDER BY created_at DESC LIMIT 5;"

echo ""
echo "📦 步骤 9: 模拟配额耗尽..."
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "UPDATE users SET remaining_quota = -1 WHERE user_id = 'test-user-001';"

echo "   再次上报流量（应触发熔断）:"
grpcurl -plaintext -d '{
  "gateway_id": "gw-test-001",
  "timestamp": '$(date +%s)',
  "base_traffic_bytes": 1024,
  "defense_traffic_bytes": 0,
  "cell_level": "standard"
}' $GRPC_HOST mirage.gateway.v1.GatewayService/ReportTraffic

echo ""
echo "📦 步骤 10: 测试威胁上报..."
grpcurl -plaintext -d '{
  "gateway_id": "gw-test-001",
  "timestamp": '$(date +%s)000000000',
  "threat_type": "THREAT_ACTIVE_PROBING",
  "source_ip": "192.168.1.200",
  "source_port": 12345,
  "severity": 7,
  "packet_count": 100,
  "ja4_fingerprint": "dGVzdF9maW5nZXJwcmludA=="
}' $GRPC_HOST mirage.gateway.v1.GatewayService/ReportThreat

echo ""
echo "📦 步骤 11: 验证威胁情报..."
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c "SELECT src_ip, threat_type, severity, hit_count, last_seen FROM threat_intel ORDER BY last_seen DESC LIMIT 5;"

echo ""
echo "=========================================="
echo "✅ 测试完成"
echo "=========================================="
echo ""
echo "📋 验证清单:"
echo "   1. Gateway 状态已更新（is_online=true, last_heartbeat_at 为最新时间）"
echo "   2. 用户配额已扣减（remaining_quota 减少 1GB）"
echo "   3. 计费流水已记录（billing_logs 有新记录）"
echo "   4. 配额耗尽时返回 remaining_quota=0 和 quota_warning=true"
echo "   5. 威胁情报已记录（threat_intel 有新记录）"
echo ""
