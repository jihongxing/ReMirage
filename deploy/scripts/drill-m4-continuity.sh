#!/usr/bin/env bash
# M4 业务连续性样板受控演练脚本
# 证据强度：代码级模拟（mock transport），非真实网络演练
# 用途：执行 switchWithTransaction 和业务连续性测试，生成分层结论报告
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EVIDENCE_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$EVIDENCE_DIR/m4-continuity-drill.log"
REPORT_FILE="$EVIDENCE_DIR/m4-continuity-report.md"

source "$SCRIPT_DIR/common-go.sh"

mkdir -p "$EVIDENCE_DIR"
init_go_runner "$PROJECT_ROOT" 23

echo "========================================" | tee "$LOG_FILE"
echo "M4 业务连续性样板受控演练" | tee -a "$LOG_FILE"
echo "时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ')" | tee -a "$LOG_FILE"
echo "证据强度: 代码级模拟（mock transport）" | tee -a "$LOG_FILE"
log_go_runner | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 1] 执行 switchWithTransaction 测试..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestSwitchWithTransaction|TestAdoptConnection|TestBusinessContinuity" \
  -v -timeout 60s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 2] 执行同 IP 幂等性 PBT (Property 8)..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestProperty_SameIPIdempotency" \
  -v -timeout 60s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 3] 执行状态一致性 PBT (Property 9)..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestProperty_PostStateConsistency" \
  -v -timeout 60s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
echo "M4 演练完成" | tee -a "$LOG_FILE"
echo "证据文件: $LOG_FILE" | tee -a "$LOG_FILE"
echo "报告文件: $REPORT_FILE" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
