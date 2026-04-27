#!/usr/bin/env bash
# M2 降级/回升受控演练脚本
# 证据强度：代码级模拟（mock transport），非真实网络演练
# 用途：执行 ClientOrchestrator 降级/回升测试，捕获运行日志作为治理证据
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EVIDENCE_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$EVIDENCE_DIR/m2-degradation-drill.log"

source "$SCRIPT_DIR/common-go.sh"

mkdir -p "$EVIDENCE_DIR"
init_go_runner "$PROJECT_ROOT" 23

echo "========================================" | tee "$LOG_FILE"
echo "M2 降级/回升受控演练" | tee -a "$LOG_FILE"
echo "时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ')" | tee -a "$LOG_FILE"
echo "证据强度: 代码级模拟（mock transport）" | tee -a "$LOG_FILE"
log_go_runner | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 1] 执行降级/回升集成测试..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestDegradationAndPromotion_FullChain|TestAllTransportsFailed|TestDegradationAndPromotionLogs" \
  -v -timeout 60s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 2] 执行降级正确性 PBT (Property 2)..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestProperty_DegradationCorrectness" \
  -v -timeout 120s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 3] 执行回升正确性 PBT (Property 3)..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestProperty_PromotionCorrectness" \
  -v -timeout 120s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
echo "M2 演练完成" | tee -a "$LOG_FILE"
echo "证据文件: $LOG_FILE" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
