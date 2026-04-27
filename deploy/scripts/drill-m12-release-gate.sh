#!/usr/bin/env bash
# M12 发布门禁 drill 脚本
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$LOG_DIR/m12-release-gate.log"
PASS=0
FAIL=0
SKIP=0

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

echo "=== M12 Release Gate Drill ===" | tee "$LOG_FILE"
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$LOG_FILE"

# 1. release-verify.ps1（如有 PowerShell 环境）
if command -v pwsh &>/dev/null; then
    check "release-verify.ps1" pwsh -ExecutionPolicy Bypass -File "$PROJECT_ROOT/scripts/release-verify.ps1"
else
    echo "── release-verify.ps1 ── ⏭️ SKIP: pwsh not found" | tee -a "$LOG_FILE"
    ((SKIP++))
fi

# 2. Go test deploy/release（含 PBT）
if command -v go &>/dev/null; then
    check "go test deploy/release" bash -c "cd '$PROJECT_ROOT/deploy/release' && go test -v -count=1 ./..."
else
    echo "── go test deploy/release ── ⏭️ SKIP: go not found" | tee -a "$LOG_FILE"
    ((SKIP++))
fi

# 3. Remediation_Roadmap Status 检查
ROADMAP="$PROJECT_ROOT/docs/governance/capability-gap-remediation-roadmap.md"
check "Remediation_Roadmap Status=completed" grep -q "Status: completed" "$ROADMAP"

echo ""
echo "════════════════════════════════════════" | tee -a "$LOG_FILE"
echo "Result: $PASS passed, $FAIL failed, $SKIP skipped" | tee -a "$LOG_FILE"
echo "════════════════════════════════════════" | tee -a "$LOG_FILE"

if [ "$FAIL" -gt 0 ]; then exit 1; else exit 0; fi
