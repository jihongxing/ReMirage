#!/bin/bash
# Mirage-OS 全场景测试脚本
# 一键运行所有压力验证场景

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="$SCRIPT_DIR/../logs"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 创建日志目录
mkdir -p "$LOG_DIR"

echo "=========================================="
echo "Mirage-OS 全场景压力验证"
echo "=========================================="
echo ""
echo "测试时间: $(date)"
echo "日志目录: $LOG_DIR"
echo ""

# 检查依赖
echo "📦 检查依赖..."
if ! command -v grpcurl &> /dev/null; then
    echo "❌ grpcurl 未安装"
    echo "   安装方法: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

if ! command -v psql &> /dev/null; then
    echo "❌ psql 未安装"
    exit 1
fi

if ! command -v bc &> /dev/null; then
    echo "❌ bc 未安装"
    exit 1
fi

echo "✅ 依赖检查通过"
echo ""

# 检查服务状态
echo "📦 检查服务状态..."
if ! nc -z localhost 50051 2>/dev/null; then
    echo "❌ Mirage-OS API Gateway 未运行（localhost:50051）"
    echo "   请先启动: cd mirage-os && go run services/api-gateway/main.go"
    exit 1
fi

if ! nc -z localhost 5432 2>/dev/null; then
    echo "❌ PostgreSQL 未运行（localhost:5432）"
    exit 1
fi

echo "✅ 服务状态正常"
echo ""

# 设置脚本可执行权限
chmod +x "$SCRIPT_DIR/test-scenario-a-quota-meltdown.sh"
chmod +x "$SCRIPT_DIR/test-scenario-b-hive-mind.sh"
chmod +x "$SCRIPT_DIR/test-scenario-c-performance.sh"

# 场景 A：欠费熔断验证
echo "=========================================="
echo "场景 A：欠费熔断验证"
echo "=========================================="
LOG_FILE="$LOG_DIR/scenario-a-quota-meltdown-$TIMESTAMP.log"
if bash "$SCRIPT_DIR/test-scenario-a-quota-meltdown.sh" 2>&1 | tee "$LOG_FILE"; then
    SCENARIO_A_RESULT="✅ 通过"
else
    SCENARIO_A_RESULT="❌ 失败"
fi
echo ""
sleep 2

# 场景 B：全球黑名单同步验证
echo "=========================================="
echo "场景 B：全球黑名单同步验证"
echo "=========================================="
LOG_FILE="$LOG_DIR/scenario-b-hive-mind-$TIMESTAMP.log"
if bash "$SCRIPT_DIR/test-scenario-b-hive-mind.sh" 2>&1 | tee "$LOG_FILE"; then
    SCENARIO_B_RESULT="✅ 通过"
else
    SCENARIO_B_RESULT="❌ 失败"
fi
echo ""
sleep 2

# 场景 C：时序性能压力测试
echo "=========================================="
echo "场景 C：时序性能压力测试"
echo "=========================================="
LOG_FILE="$LOG_DIR/scenario-c-performance-$TIMESTAMP.log"
if bash "$SCRIPT_DIR/test-scenario-c-performance.sh" 2>&1 | tee "$LOG_FILE"; then
    SCENARIO_C_RESULT="✅ 通过"
else
    SCENARIO_C_RESULT="❌ 失败"
fi
echo ""

# 生成总结报告
REPORT_FILE="$LOG_DIR/test-report-$TIMESTAMP.txt"

cat > "$REPORT_FILE" <<EOF
========================================
Mirage-OS 压力验证总结报告
========================================

测试时间: $(date)
测试环境:
  - Mirage-OS API Gateway: localhost:50051
  - PostgreSQL: localhost:5432

========================================
测试结果
========================================

场景 A：欠费熔断验证
  结果: $SCENARIO_A_RESULT
  验证点:
    - 配额耗尽时返回 remaining_quota=0
    - 计费流水正确记录
    - 内核态熔断逻辑触发

场景 B：全球黑名单同步验证
  结果: $SCENARIO_B_RESULT
  验证点:
    - 威胁情报累积（hit_count >= 10）
    - 心跳响应包含 globalBlacklist
    - 黑名单包含攻击者 IP

场景 C：时序性能压力测试
  结果: $SCENARIO_C_RESULT
  验证点:
    - 100 并发 Gateway × 10 请求
    - 平均延迟 < 100ms
    - 计费流水写入成功率 >= 90%
    - 配额扣减准确率 >= 90%

========================================
详细日志
========================================

场景 A: $LOG_DIR/scenario-a-quota-meltdown-$TIMESTAMP.log
场景 B: $LOG_DIR/scenario-b-hive-mind-$TIMESTAMP.log
场景 C: $LOG_DIR/scenario-c-performance-$TIMESTAMP.log

========================================
下一步行动
========================================

1. 在 Gateway 端验证内核态逻辑:
   - 检查 quota_map: sudo bpftool map dump name quota_map
   - 检查 high_risk_ips: sudo bpftool map dump name high_risk_ips
   - 验证 TC_ACT_STOLEN 熔断效果

2. 性能优化（如果场景 C 失败）:
   - 增加数据库连接池大小
   - 为 billing_logs 添加时间分区
   - 优化索引策略

3. 前端集成:
   - 实时显示配额消耗
   - 全球威胁地图
   - 计费流水图表

========================================
EOF

echo ""
echo "=========================================="
echo "全场景测试完成"
echo "=========================================="
echo ""
echo "📋 测试结果:"
echo "   场景 A（欠费熔断）: $SCENARIO_A_RESULT"
echo "   场景 B（全球黑名单）: $SCENARIO_B_RESULT"
echo "   场景 C（性能压力）: $SCENARIO_C_RESULT"
echo ""
echo "📄 总结报告已生成: $REPORT_FILE"
echo ""
cat "$REPORT_FILE"
