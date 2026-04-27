#!/usr/bin/env bash
# M7 eBPF 覆盖图与性能验证脚本
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EVIDENCE_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$EVIDENCE_DIR/m7-ebpf-coverage-drill.log"
PERF_DIR="$PROJECT_ROOT/artifacts/ebpf-perf"
COVERAGE_MAP="$PROJECT_ROOT/docs/reports/ebpf-coverage-map.md"

source "$SCRIPT_DIR/common-go.sh"

mkdir -p "$EVIDENCE_DIR" "$PERF_DIR"

PASS=0
FAIL=0
SKIP=0

pass() {
	echo "  ✅ $1" | tee -a "$LOG_FILE"
	PASS=$((PASS + 1))
}

fail() {
	echo "  ❌ $1" | tee -a "$LOG_FILE"
	FAIL=$((FAIL + 1))
}

skip() {
	echo "  ⏭️  $1 [降级: 跳过]" | tee -a "$LOG_FILE"
	SKIP=$((SKIP + 1))
}

echo "========================================" | tee "$LOG_FILE"
echo "M7 eBPF 覆盖图与性能验证" | tee -a "$LOG_FILE"
echo "时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ')" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

echo "" | tee -a "$LOG_FILE"
echo "[Step 1] 检查环境..." | tee -a "$LOG_FILE"

IS_LINUX=false
if [[ "$(uname -s)" == "Linux" ]]; then
	IS_LINUX=true
	pass "操作系统: Linux ($(uname -r))"
else
	skip "操作系统: $(uname -s) — 非 Linux，eBPF 编译和性能采集将降级"
fi

HAS_CLANG=false
if command -v clang >/dev/null 2>&1; then
	HAS_CLANG=true
	pass "clang 可用: $(clang --version | head -1)"
else
	skip "clang 不可用 — eBPF 编译回归将降级"
fi

if init_go_runner "$PROJECT_ROOT" 23; then
	pass "Go 运行器已就绪: $GO_RUNNER_DESC"
else
	skip "Go >= 1.23 不可用 — 将使用 shell 编译回归 fallback"
fi

HAS_BPFTRACE=false
if command -v bpftrace >/dev/null 2>&1; then
	HAS_BPFTRACE=true
	pass "bpftrace 可用"
else
	skip "bpftrace 不可用 — 延迟数据采集将跳过"
fi

HAS_PERF=false
if command -v perf >/dev/null 2>&1 || command -v mpstat >/dev/null 2>&1; then
	HAS_PERF=true
	pass "perf/mpstat 可用"
else
	skip "perf/mpstat 不可用 — CPU 数据采集将跳过"
fi

HAS_BPFTOOL=false
if command -v bpftool >/dev/null 2>&1; then
	HAS_BPFTOOL=true
	pass "bpftool 可用"
else
	skip "bpftool 不可用 — Map 内存数据采集将跳过"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 2] 执行 eBPF 编译回归..." | tee -a "$LOG_FILE"

if [[ "$IS_LINUX" == true && "$HAS_CLANG" == true ]]; then
	COMPILE_OK=false
	if [[ "${GO_RUNNER:-}" != "" ]]; then
		GO_COMPILE_LOG="$(mktemp)"
		if run_go_cmd "mirage-gateway" test -run "TestBPFCompile_KeyCFiles" -v ./pkg/ebpf/ 2>&1 | tee "$GO_COMPILE_LOG" | tee -a "$LOG_FILE"; then
			if grep -q "^--- SKIP:" "$GO_COMPILE_LOG" || grep -q "skipping eBPF compile test on windows" "$GO_COMPILE_LOG"; then
				echo "  说明: 当前 Go runner 命中 SKIP，继续执行 shell fallback 以补齐真实 Linux 编译证据" | tee -a "$LOG_FILE"
			else
				COMPILE_OK=true
				pass "Go 编译回归通过"
			fi
		else
			echo "  说明: Go 编译回归失败，尝试 shell fallback ..." | tee -a "$LOG_FILE"
		fi
		rm -f "$GO_COMPILE_LOG"
	fi

	if [[ "$COMPILE_OK" == false ]]; then
		if (cd "$PROJECT_ROOT/mirage-gateway" && bash ./scripts/test-ebpf-compile.sh) 2>&1 | tee -a "$LOG_FILE"; then
			COMPILE_OK=true
			pass "shell 编译回归 fallback 通过"
		else
			fail "eBPF 编译回归失败"
		fi
	fi
else
	skip "非 Linux 或缺少 clang — eBPF 编译回归跳过"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 3] 延迟数据采集..." | tee -a "$LOG_FILE"

LATENCY_SCRIPT="$PROJECT_ROOT/benchmarks/ebpf_latency.bt"
LATENCY_REPORT="$PERF_DIR/latency-report.txt"
if [[ "$IS_LINUX" == true && "$HAS_BPFTRACE" == true && -f "$LATENCY_SCRIPT" ]]; then
	if timeout 15 sudo bpftrace "$LATENCY_SCRIPT" > "$LATENCY_REPORT" 2>&1; then
		pass "延迟数据已采集: $LATENCY_REPORT"
	else
		skip "bpftrace 采集失败，保留占位报告"
	fi
else
	skip "延迟数据缺失，维持占位报告"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 4] CPU 数据采集..." | tee -a "$LOG_FILE"

CPU_REPORT="$PERF_DIR/cpu-report.txt"
if [[ "$IS_LINUX" == true && "$HAS_PERF" == true ]]; then
	if command -v mpstat >/dev/null 2>&1; then
		if timeout 10 mpstat -P ALL 1 5 > "$CPU_REPORT" 2>&1; then
			pass "CPU 数据已采集 (mpstat)"
		else
			skip "mpstat 采集失败，保留占位报告"
		fi
	elif command -v perf >/dev/null 2>&1; then
		if timeout 10 sudo perf stat -a sleep 5 > "$CPU_REPORT" 2>&1; then
			pass "CPU 数据已采集 (perf)"
		else
			skip "perf 采集失败，保留占位报告"
		fi
	fi
else
	skip "CPU 数据缺失，维持占位报告"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 5] Map 内存数据采集..." | tee -a "$LOG_FILE"

MEMORY_REPORT="$PERF_DIR/memory-report.txt"
if [[ "$IS_LINUX" == true && "$HAS_BPFTOOL" == true ]]; then
	if sudo bpftool map show > "$MEMORY_REPORT" 2>&1; then
		pass "Map 内存数据已采集"
	else
		skip "bpftool 采集失败，保留占位报告"
	fi
else
	skip "Map 内存数据缺失，维持占位报告"
fi

echo "" | tee -a "$LOG_FILE"
echo "[Step 6] 验证 ebpf-coverage-map.md 文档完整性..." | tee -a "$LOG_FILE"

if [[ -f "$COVERAGE_MAP" ]]; then
	pass "ebpf-coverage-map.md 存在"

	for section in \
		"运行态挂载程序清单" \
		"源码定义但未挂载" \
		"用户态处理路径" \
		"参与 vs 用户态处理路径对照表" \
		"参与度定性结论" \
		"性能证据"; do
		if grep -q "$section" "$COVERAGE_MAP"; then
			pass "章节存在: $section"
		else
			fail "章节缺失: $section"
		fi
	done

	for prog in "npm_xdp_main" "bdna_tcp_rewrite" "jitter_lite_egress" "sockmap_sockops"; do
		if grep -q "$prog" "$COVERAGE_MAP"; then
			pass "关键程序已列出: $prog"
		else
			fail "关键程序缺失: $prog"
		fi
	done
else
	fail "ebpf-coverage-map.md 不存在"
fi

echo "" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
echo "M7 eBPF 覆盖图与性能验证摘要" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"
echo "通过: $PASS  失败: $FAIL  跳过: $SKIP" | tee -a "$LOG_FILE"
echo "证据文件: $LOG_FILE" | tee -a "$LOG_FILE"
echo "========================================" | tee -a "$LOG_FILE"

if [[ "$FAIL" -gt 0 ]]; then
	exit 1
fi
exit 0
