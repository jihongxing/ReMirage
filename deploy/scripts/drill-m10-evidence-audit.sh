#!/usr/bin/env bash
# M10 证据审计 drill 脚本
# 检查七域证据文件存在性 + capability-truth-source.md 回写完整性
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$LOG_DIR/m10-evidence-audit.log"
PASS=0
FAIL=0

mkdir -p "$LOG_DIR"

check() {
    local name="$1"; shift
    printf "── %s ──\n" "$name" | tee -a "$LOG_FILE"
    if "$@" >> "$LOG_FILE" 2>&1; then
        echo "  ✅ PASS" | tee -a "$LOG_FILE"; ((PASS++))
    else
        echo "  ❌ FAIL" | tee -a "$LOG_FILE"; ((FAIL++))
    fi
}

echo "=== M10 Evidence Audit Drill ===" | tee "$LOG_FILE"
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$LOG_FILE"

# 七域证据文件存在性检查
check "Evidence: carrier-matrix (M1)" test -f "$PROJECT_ROOT/docs/governance/carrier-matrix.md"
check "Evidence: m2-degradation-drill (M2)" test -f "$PROJECT_ROOT/deploy/scripts/drill-m2-degradation.sh"
check "Evidence: m3-node-death-drill (M3)" test -f "$PROJECT_ROOT/deploy/scripts/drill-m3-node-death.sh"
check "Evidence: m4-continuity-report (M4)" test -f "$PROJECT_ROOT/deploy/evidence/m4-continuity-report.md"
check "Evidence: stealth-experiment-plan (M5)" test -f "$PROJECT_ROOT/docs/reports/stealth-experiment-plan.md"
check "Evidence: stealth-experiment-results (M6)" test -f "$PROJECT_ROOT/docs/reports/stealth-experiment-results.md"
check "Evidence: stealth-claims-boundary (M6)" test -f "$PROJECT_ROOT/docs/reports/stealth-claims-boundary.md"
check "Evidence: ebpf-coverage-map (M7)" test -f "$PROJECT_ROOT/docs/reports/ebpf-coverage-map.md"
check "Evidence: deployment-tiers (M8)" test -f "$PROJECT_ROOT/docs/reports/deployment-tiers.md"
check "Evidence: deployment-baseline-checklist (M8)" test -f "$PROJECT_ROOT/docs/reports/deployment-baseline-checklist.md"
check "Evidence: access-control-joint-drill (M9)" test -f "$PROJECT_ROOT/docs/reports/access-control-joint-drill.md"
check "Evidence: phase4-evidence-audit (M10)" test -f "$PROJECT_ROOT/docs/reports/phase4-evidence-audit.md"

# capability-truth-source.md 回写完整性检查
CTS="$PROJECT_ROOT/docs/governance/capability-truth-source.md"
check "CTS: M10 盘点结论存在" grep -q "Phase 4 M10 盘点结论" "$CTS"
check "CTS: 升级条件评估存在" grep -q "Phase 4 M10 升级条件评估结论" "$CTS"

echo ""
echo "════════════════════════════════════════" | tee -a "$LOG_FILE"
echo "Result: $PASS passed, $FAIL failed" | tee -a "$LOG_FILE"
echo "════════════════════════════════════════" | tee -a "$LOG_FILE"

if [ "$FAIL" -gt 0 ]; then exit 1; else exit 0; fi
