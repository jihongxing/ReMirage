#!/usr/bin/env bash
# M11 收敛验证 drill 脚本
# 按七域结构化核对表检查三份文档一致性
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$LOG_DIR/m11-convergence.log"
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

echo "=== M11 Convergence Verification Drill ===" | tee "$LOG_FILE"
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$LOG_FILE"

CTS="$PROJECT_ROOT/docs/governance/capability-truth-source.md"
MPS="$PROJECT_ROOT/docs/governance/market-positioning-scenarios.md"
DM="$PROJECT_ROOT/docs/暗网基础设施防御力评价矩阵.md"
CCR="$PROJECT_ROOT/docs/reports/cross-document-consistency.md"

# 文档存在性
check "cross-document-consistency.md exists" test -f "$CCR"

# Market_Positioning 对齐声明
check "MPS: alignment declaration" grep -q "Capability_Truth_Source 完成对齐" "$MPS"

# Defense_Matrix 对齐声明
check "DM: alignment declaration" grep -q "Alignment:" "$DM"

# 七域结构化核对：Status_Level 一致性
DOMAINS=("多承载编排与降级" "节点恢复与共振发现" "会话连续性与链路漂移" "流量整形与特征隐匿" "eBPF 深度参与" "反取证与最小运行痕迹" "准入控制与防滥用")

for domain in "${DOMAINS[@]}"; do
    check "CTS contains domain: $domain" grep -q "$domain" "$CTS"
done

# 限定语存在性检查（部分实现域）
check "DM: stealth marked as target score" grep -q "目标分值" "$DM"

# 绝对化违规词检查
check "DM: no '秒杀'" bash -c "! grep -q '秒杀' '$DM'"
check "DM: no '必然瘫痪'" bash -c "! grep -q '必然瘫痪' '$DM'"
check "DM: no '绝对优于'" bash -c "! grep -q '绝对优于' '$DM'"
check "MPS: no absolute claims" bash -c "! grep -q '100%' '$MPS'"

# 主证据锚点引用检查
check "CTS: phase4-evidence-audit referenced" grep -q "phase4-evidence-audit" "$CTS"

echo ""
echo "════════════════════════════════════════" | tee -a "$LOG_FILE"
echo "Result: $PASS passed, $FAIL failed" | tee -a "$LOG_FILE"
echo "════════════════════════════════════════" | tee -a "$LOG_FILE"

if [ "$FAIL" -gt 0 ]; then exit 1; else exit 0; fi
