#!/usr/bin/env bash
# M5 隐匿实验方案冻结验证脚本
# 证据强度：文档完整性检查（非实验执行）
# 用途：验证 stealth-experiment-plan.md 和 stealth-claims-boundary.md 是否存在且包含必要章节
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EVIDENCE_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$EVIDENCE_DIR/m5-experiment-plan-drill.log"

mkdir -p "$EVIDENCE_DIR"

PASS=0
FAIL=0

pass() {
  echo "  ✅ $1" | tee -a "$LOG_FILE"
  PASS=$((PASS + 1))
}

fail() {
  echo "  ❌ $1" | tee -a "$LOG_FILE"
  FAIL=$((FAIL + 1))
}

echo "========================================" | tee "$LOG_FILE"
echo "M5 隐匿实验方案冻结验证" | tee -a "$LOG_FILE"
echo "时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ')" | tee -a "$LOG_FILE"
echo "证据强度: 文档完整性检查" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

EXPERIMENT_PLAN="$PROJECT_ROOT/docs/reports/stealth-experiment-plan.md"
CLAIMS_BOUNDARY="$PROJECT_ROOT/docs/reports/stealth-claims-boundary.md"

# ── Step 1: 检查 stealth-experiment-plan.md 存在性 ──
echo "" | tee -a "$LOG_FILE"
echo "[Step 1] 检查 stealth-experiment-plan.md 是否存在..." | tee -a "$LOG_FILE"
if [ -f "$EXPERIMENT_PLAN" ]; then
  pass "stealth-experiment-plan.md 存在"
else
  fail "stealth-experiment-plan.md 不存在"
fi

# ── Step 2: 验证四个检测面方法论完整性 ──
echo "" | tee -a "$LOG_FILE"
echo "[Step 2] 验证四个检测面方法论章节..." | tee -a "$LOG_FILE"

if [ -f "$EXPERIMENT_PLAN" ]; then
  for section in "握手指纹" "包长分布" "时序分布" "简单分类器"; do
    if grep -q "$section" "$EXPERIMENT_PLAN"; then
      pass "检测面方法论章节存在: $section"
    else
      fail "检测面方法论章节缺失: $section"
    fi
  done
else
  fail "无法验证检测面章节（文件不存在）"
fi

# ── Step 3: 检查 stealth-claims-boundary.md 存在性 ──
echo "" | tee -a "$LOG_FILE"
echo "[Step 3] 检查 stealth-claims-boundary.md 是否存在..." | tee -a "$LOG_FILE"
if [ -f "$CLAIMS_BOUNDARY" ]; then
  pass "stealth-claims-boundary.md 存在"
else
  fail "stealth-claims-boundary.md 不存在"
fi

# ── Step 4: 验证 claims-boundary 包含允许/不允许表述清单 ──
echo "" | tee -a "$LOG_FILE"
echo "[Step 4] 验证允许/不允许表述清单..." | tee -a "$LOG_FILE"

if [ -f "$CLAIMS_BOUNDARY" ]; then
  if grep -q "允许表述" "$CLAIMS_BOUNDARY"; then
    pass "允许表述清单存在"
  else
    fail "允许表述清单缺失"
  fi

  if grep -q "不允许表述" "$CLAIMS_BOUNDARY"; then
    pass "不允许表述清单存在"
  else
    fail "不允许表述清单缺失"
  fi
else
  fail "无法验证表述清单（文件不存在）"
fi

# ── 汇总 ──
echo "" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
echo "M5 验证完成" | tee -a "$LOG_FILE"
echo "通过: $PASS  失败: $FAIL" | tee -a "$LOG_FILE"
echo "证据文件: $LOG_FILE" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0
