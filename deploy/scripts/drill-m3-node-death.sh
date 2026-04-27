#!/usr/bin/env bash
# M3 节点阵亡恢复受控演练脚本
# 证据强度：代码级模拟（mock transport），非真实网络演练
# 用途：执行节点阵亡恢复演练测试，捕获 RecoveryFSM 各阶段日志
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EVIDENCE_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$EVIDENCE_DIR/m3-node-death-drill.log"

source "$SCRIPT_DIR/common-go.sh"

mkdir -p "$EVIDENCE_DIR"
init_go_runner "$PROJECT_ROOT" 23

echo "========================================" | tee "$LOG_FILE"
echo "M3 节点阵亡恢复受控演练" | tee -a "$LOG_FILE"
echo "时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ')" | tee -a "$LOG_FILE"
echo "证据强度: 代码级模拟（mock transport）" | tee -a "$LOG_FILE"
log_go_runner | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 1] 执行节点阵亡恢复演练测试..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestNodeDeathDrill" \
  -v -timeout 120s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 2] 执行 RecoveryFSM 阶段测试..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestEvaluate_PhaseBoundaries|TestExecute_PhaseProgression|TestReconnect_FirstEvaluate" \
  -v -timeout 60s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 3] 执行 RecoveryFSM 单调性 PBT (Property 1)..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestProperty_EvaluateMonotonicity" \
  -v -timeout 60s ./pkg/gtclient/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 4] 执行 Resolver PBT (Property 6, 7)..." | tee -a "$LOG_FILE"
run_go_cmd "phantom-client" test -run "TestProperty_FirstWinRacing|TestProperty_AllFailAggregatedError" \
  -v -timeout 120s ./pkg/resonance/ 2>&1 | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
echo "M3 演练完成" | tee -a "$LOG_FILE"
echo "证据文件: $LOG_FILE" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
