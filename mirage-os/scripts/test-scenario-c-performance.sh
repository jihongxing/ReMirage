#!/bin/bash
# 场景 C：时序性能压力测试
# 验证高频心跳下的数据库性能和计费流水写入延迟

set -e

echo "=========================================="
echo "场景 C：时序性能压力测试"
echo "=========================================="

# 配置
GRPC_HOST="localhost:50051"
DB_HOST="localhost"
DB_USER="postgres"
DB_NAME="mirage_os"
CONCURRENT_GATEWAYS=100
REQUESTS_PER_GATEWAY=10

echo ""
echo "📦 步骤 1: 准备测试环境..."

# 创建测试用户和 Gateway
psql -h $DB_HOST -U $DB_USER -d $DB_NAME <<EOF
-- 清理旧数据
DELETE FROM billing_logs WHERE gateway_id LIKE 'gw-perf-%';
DELETE FROM gateways WHERE gateway_id LIKE 'gw-perf-%';
DELETE FROM users WHERE user_id = 'user-perf-test';

-- 创建测试用户
INSERT INTO users (user_id, xmr_address, balance, balance_usd, remaining_quota, total_quota, cell_level)
VALUES ('user-perf-test', 'XMR_PERF_TEST', 100.0, 15000.0, 1099511627776, 1099511627776, 1);
EOF

echo "✅ 测试用户已创建"

echo ""
echo "📦 步骤 2: 创建 $CONCURRENT_GATEWAYS 个模拟 Gateway..."

for i in $(seq 1 $CONCURRENT_GATEWAYS); do
  GW_ID=$(printf "gw-perf-%03d" $i)
  
  psql -h $DB_HOST -U $DB_USER -d $DB_NAME <<EOF > /dev/null
INSERT INTO gateways (gateway_id, cell_id, user_id, ip_address, version, is_online)
VALUES ('$GW_ID', 'cell-us-west-01', 'user-perf-test', '192.168.1.$i', '1.0.0', false)
ON CONFLICT (gateway_id) DO NOTHING;
EOF
done

echo "✅ $CONCURRENT_GATEWAYS 个 Gateway 已创建"

echo ""
echo "📦 步骤 3: 启动压力测试（$CONCURRENT_GATEWAYS 并发 × $REQUESTS_PER_GATEWAY 请求）..."

START_TIME=$(date +%s)

# 并发发送请求
for i in $(seq 1 $CONCURRENT_GATEWAYS); do
  GW_ID=$(printf "gw-perf-%03d" $i)
  
  (
    for j in $(seq 1 $REQUESTS_PER_GATEWAY); do
      grpcurl -plaintext -d '{
        "gateway_id": "'$GW_ID'",
        "timestamp": '$(date +%s)',
        "base_traffic_bytes": 1048576,
        "defense_traffic_bytes": 524288,
        "cell_level": "standard"
      }' $GRPC_HOST mirage.gateway.v1.GatewayService/ReportTraffic > /dev/null 2>&1
    done
  ) &
done

# 等待所有后台任务完成
wait

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo "✅ 压力测试完成，耗时: ${DURATION}s"

echo ""
echo "📦 步骤 4: 统计计费流水写入性能..."

TOTAL_LOGS=$(psql -h $DB_HOST -U $DB_USER -d $DB_NAME -t -c \
  "SELECT COUNT(*) FROM billing_logs WHERE gateway_id LIKE 'gw-perf-%';")

echo "   总计费流水记录: $TOTAL_LOGS"
echo "   预期记录数: $((CONCURRENT_GATEWAYS * REQUESTS_PER_GATEWAY))"

if [ "$TOTAL_LOGS" -ge $((CONCURRENT_GATEWAYS * REQUESTS_PER_GATEWAY * 9 / 10)) ]; then
    echo "✅ 计费流水写入成功率 >= 90%"
else
    echo "⚠️  计费流水写入成功率 < 90%，可能存在性能瓶颈"
fi

# 计算平均延迟
AVG_LATENCY=$(echo "scale=2; $DURATION * 1000 / ($CONCURRENT_GATEWAYS * $REQUESTS_PER_GATEWAY)" | bc)
echo "   平均请求延迟: ${AVG_LATENCY}ms"

if (( $(echo "$AVG_LATENCY < 100" | bc -l) )); then
    echo "✅ 平均延迟 < 100ms"
else
    echo "⚠️  平均延迟 >= 100ms，需要优化"
fi

echo ""
echo "📦 步骤 5: 分析数据库性能指标..."

# 查询最近的计费流水
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c \
  "SELECT 
     COUNT(*) as total_logs,
     SUM(business_bytes) as total_business_bytes,
     SUM(defense_bytes) as total_defense_bytes,
     SUM(cost_usd) as total_cost_usd,
     MIN(created_at) as first_log,
     MAX(created_at) as last_log
   FROM billing_logs 
   WHERE gateway_id LIKE 'gw-perf-%';"

# 查询数据库连接数
echo ""
echo "   当前数据库连接数:"
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c \
  "SELECT COUNT(*) as active_connections 
   FROM pg_stat_activity 
   WHERE datname = '$DB_NAME';"

# 查询表大小
echo ""
echo "   billing_logs 表大小:"
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -c \
  "SELECT pg_size_pretty(pg_total_relation_size('billing_logs')) as table_size;"

echo ""
echo "📦 步骤 6: 验证配额扣减..."

REMAINING=$(psql -h $DB_HOST -U $DB_USER -d $DB_NAME -t -c \
  "SELECT remaining_quota FROM users WHERE user_id = 'user-perf-test';")

EXPECTED_CONSUMED=$((TOTAL_LOGS * (1048576 + 524288)))
ACTUAL_CONSUMED=$((1099511627776 - REMAINING))

echo "   预期消耗: $EXPECTED_CONSUMED 字节"
echo "   实际消耗: $ACTUAL_CONSUMED 字节"
echo "   剩余配额: $REMAINING 字节"

if [ "$ACTUAL_CONSUMED" -ge $((EXPECTED_CONSUMED * 9 / 10)) ]; then
    echo "✅ 配额扣减准确率 >= 90%"
else
    echo "⚠️  配额扣减准确率 < 90%，可能存在并发问题"
fi

echo ""
echo "=========================================="
echo "✅ 场景 C 验证完成"
echo "=========================================="
echo ""
echo "📋 性能指标:"
echo "   - 并发 Gateway 数: $CONCURRENT_GATEWAYS"
echo "   - 每个 Gateway 请求数: $REQUESTS_PER_GATEWAY"
echo "   - 总请求数: $((CONCURRENT_GATEWAYS * REQUESTS_PER_GATEWAY))"
echo "   - 总耗时: ${DURATION}s"
echo "   - 平均延迟: ${AVG_LATENCY}ms"
echo "   - 计费流水记录: $TOTAL_LOGS"
echo "   - 写入成功率: $(echo "scale=2; $TOTAL_LOGS * 100 / ($CONCURRENT_GATEWAYS * $REQUESTS_PER_GATEWAY)" | bc)%"
echo ""
echo "🔥 性能优化建议:"
echo "   1. 如果平均延迟 > 100ms，考虑增加数据库连接池大小"
echo "   2. 如果写入成功率 < 95%，检查数据库锁竞争"
echo "   3. 为 billing_logs 表添加分区（按时间）"
echo "   4. 为 gateways.gateway_id 和 users.user_id 添加索引"
echo "   5. 考虑使用 PostgreSQL 的 COPY 批量插入"
echo ""
