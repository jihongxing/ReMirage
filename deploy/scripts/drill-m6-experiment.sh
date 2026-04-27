#!/usr/bin/env bash
# M6 隐匿实验执行编排脚本
# 证据强度：
#   - 受控环境基线（真实 Linux 抓包）
#   - 模拟环境参考（自动降级，保证链路可复验）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EVIDENCE_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$EVIDENCE_DIR/m6-experiment-drill.log"
DPI_AUDIT_DIR="$PROJECT_ROOT/artifacts/dpi-audit"
CORE_REQUIREMENTS="$DPI_AUDIT_DIR/requirements.txt"

source "$SCRIPT_DIR/common-go.sh"
source "$SCRIPT_DIR/common-python.sh"

mkdir -p "$EVIDENCE_DIR"

PASS=0
FAIL=0
SKIP=0
DEGRADED=false

pass() {
	echo "  ✅ $1" | tee -a "$LOG_FILE"
	PASS=$((PASS + 1))
}

fail() {
	echo "  ❌ $1" | tee -a "$LOG_FILE"
	FAIL=$((FAIL + 1))
}

skip() {
	echo "  ⏭️  $1 [降级跳过]" | tee -a "$LOG_FILE"
	SKIP=$((SKIP + 1))
}

echo "========================================" | tee "$LOG_FILE"
echo "M6 隐匿实验执行编排" | tee -a "$LOG_FILE"
echo "时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ')" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 1] 环境检查..." | tee -a "$LOG_FILE"

if init_go_runner "$PROJECT_ROOT" 23; then
	log_go_runner | tee -a "$LOG_FILE"
	pass "Go 运行器已就绪"
else
	fail "Go 运行器不可用，无法执行 PBT"
fi

if init_python_runner "$PROJECT_ROOT" 10; then
	log_python_runner | tee -a "$LOG_FILE"
	pass "Python 运行器已就绪"
else
	fail "Python 运行器不可用"
fi

if ensure_python_modules "$CORE_REQUIREMENTS" scapy numpy pandas; then
	pass "分析依赖已就绪（scapy/numpy/pandas）"
else
	fail "无法准备分析依赖"
fi

if ensure_python_modules "$CORE_REQUIREMENTS" sklearn; then
	pass "分类器依赖已就绪（scikit-learn）"
else
	skip "scikit-learn 不可用，分类器训练将跳过"
fi

if [[ "$(uname -s)" != "Linux" ]]; then
	DEGRADED=true
	echo "  操作系统: $(uname -s) — 仅能生成模拟样本" | tee -a "$LOG_FILE"
else
	echo "  操作系统: Linux ($(uname -r))" | tee -a "$LOG_FILE"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 2] 抓包/模拟样本生成..." | tee -a "$LOG_FILE"
if bash "$DPI_AUDIT_DIR/generate-samples.sh" 2>&1 | tee -a "$LOG_FILE"; then
	if [[ -f "$DPI_AUDIT_DIR/simulation-metadata.json" ]]; then
		DEGRADED=true
		pass "样本生成完成（模拟环境参考）"
	else
		pass "样本生成完成（受控环境基线）"
	fi
else
	fail "样本生成失败"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 3] 执行 eBPF Mock PBT..." | tee -a "$LOG_FILE"
if [[ "$FAIL" -eq 0 ]]; then
	if run_go_cmd "mirage-gateway" test -run "TestProperty_NPM" -v -timeout 120s ./pkg/ebpf/ 2>&1 | tee -a "$LOG_FILE"; then
		pass "NPM PBT 通过"
	else
		fail "NPM PBT 失败"
	fi

	if run_go_cmd "mirage-gateway" test -run "TestProperty_BDNA" -v -timeout 120s ./pkg/ebpf/ 2>&1 | tee -a "$LOG_FILE"; then
		pass "B-DNA PBT 通过"
	else
		fail "B-DNA PBT 失败"
	fi

	if run_go_cmd "mirage-gateway" test -run "TestProperty_Jitter" -v -timeout 120s ./pkg/ebpf/ 2>&1 | tee -a "$LOG_FILE"; then
		pass "Jitter PBT 通过"
	else
		fail "Jitter PBT 失败"
	fi
else
	skip "由于前置失败，PBT 未执行"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 4] 运行分析脚本与特征汇总..." | tee -a "$LOG_FILE"
if [[ "$FAIL" -eq 0 ]]; then
	if run_python_script "artifacts/dpi-audit/handshake" "extract-features.py" 2>&1 | tee -a "$LOG_FILE"; then
		pass "握手指纹特征提取完成"
	else
		fail "握手指纹特征提取失败"
	fi

	if run_python_script "artifacts/dpi-audit/packet-length" "analyze-distribution.py" 2>&1 | tee -a "$LOG_FILE"; then
		pass "包长分布分析完成"
	else
		fail "包长分布分析失败"
	fi

	if run_python_script "artifacts/dpi-audit/timing" "analyze-timing.py" 2>&1 | tee -a "$LOG_FILE"; then
		pass "时序分布分析完成"
	else
		fail "时序分布分析失败"
	fi

	if run_python_script "artifacts/dpi-audit/classifier" "build-features.py" 2>&1 | tee -a "$LOG_FILE"; then
		pass "分类器特征集已生成"
	else
		fail "分类器特征集生成失败"
	fi
else
	skip "由于前置失败，分析脚本未执行"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 5] 运行分类器训练..." | tee -a "$LOG_FILE"
if [[ "$FAIL" -eq 0 ]]; then
	if python_has_modules sklearn; then
		if run_python_script "artifacts/dpi-audit/classifier" "train-classifier.py" 2>&1 | tee -a "$LOG_FILE"; then
			pass "分类器训练完成"
		else
			fail "分类器训练失败"
		fi
	else
		skip "scikit-learn 不可用，分类器训练跳过"
	fi
else
	skip "由于前置失败，分类器训练未执行"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 6] 验证关键产物..." | tee -a "$LOG_FILE"
for output in \
	"docs/reports/stealth-experiment-results.md" \
	"docs/reports/stealth-experiment-plan.md" \
	"docs/reports/stealth-claims-boundary.md" \
	"artifacts/dpi-audit/handshake/comparison.csv" \
	"artifacts/dpi-audit/packet-length/distributions.csv" \
	"artifacts/dpi-audit/timing/iat-stats.csv" \
	"artifacts/dpi-audit/classifier/features.csv"; do
	if [[ -f "$PROJECT_ROOT/$output" ]]; then
		pass "产物存在: $output"
	else
		fail "产物缺失: $output"
	fi
done

if [[ -f "$PROJECT_ROOT/artifacts/dpi-audit/classifier/results.json" ]]; then
	pass "产物存在: artifacts/dpi-audit/classifier/results.json"
else
	skip "results.json 缺失（分类器训练未执行或失败）"
fi

echo "" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
echo "M6 实验编排完成" | tee -a "$LOG_FILE"
echo "通过: $PASS  失败: $FAIL  跳过: $SKIP" | tee -a "$LOG_FILE"
if $DEGRADED; then
	echo "模式: 降级（模拟环境参考）" | tee -a "$LOG_FILE"
else
	echo "模式: 完整（受控环境基线）" | tee -a "$LOG_FILE"
fi
echo "证据文件: $LOG_FILE" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

if [[ "$FAIL" -gt 0 ]]; then
	exit 1
fi
exit 0
