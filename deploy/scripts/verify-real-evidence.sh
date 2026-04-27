#!/usr/bin/env bash
# Strict evidence gate for upgrading capability claims beyond simulated/reference status.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

FAIL=0
PASS=0

pass() {
	echo "  PASS: $1"
	PASS=$((PASS + 1))
}

fail() {
	echo "  FAIL: $1"
	FAIL=$((FAIL + 1))
}

require_file() {
	local path="$1"
	if [[ -f "$PROJECT_ROOT/$path" ]]; then
		pass "file exists: $path"
	else
		fail "file missing: $path"
	fi
}

reject_pattern() {
	local path="$1"
	local pattern="$2"
	local reason="$3"
	if [[ ! -f "$PROJECT_ROOT/$path" ]]; then
		fail "cannot inspect missing file: $path"
		return
	fi
	if grep -Eqi "$pattern" "$PROJECT_ROOT/$path"; then
		fail "$reason: $path"
	else
		pass "$reason cleared: $path"
	fi
}

echo "========================================"
echo "Real Evidence Gate"
echo "========================================"

if [[ "$(uname -s)" != "Linux" ]]; then
	fail "must run on Linux for eBPF/DPI evidence collection"
else
	pass "running on Linux ($(uname -r))"
fi

for cmd in clang tcpdump python3; do
	if command -v "$cmd" >/dev/null 2>&1; then
		pass "$cmd available"
	else
		fail "$cmd missing"
	fi
done

for optional_cmd in bpftrace bpftool perf; do
	if command -v "$optional_cmd" >/dev/null 2>&1; then
		pass "$optional_cmd available"
	else
		fail "$optional_cmd missing"
	fi
done

require_file "docs/reports/stealth-experiment-results.md"
require_file "deploy/evidence/m6-experiment-drill.log"
require_file "deploy/evidence/m7-ebpf-coverage-drill.log"
require_file "artifacts/dpi-audit/classifier/results.json"
require_file "artifacts/ebpf-perf/latency-report.txt"
require_file "artifacts/ebpf-perf/cpu-report.txt"
require_file "artifacts/ebpf-perf/memory-report.txt"

if [[ -f "$PROJECT_ROOT/artifacts/dpi-audit/simulation-metadata.json" ]]; then
	fail "simulation metadata is present; M6 evidence is simulated"
else
	pass "no simulation metadata present"
fi

reject_pattern "docs/reports/stealth-experiment-results.md" "simulated-reference|模拟环境参考" "M6 report is not simulated"
reject_pattern "deploy/evidence/m6-experiment-drill.log" "降级|模拟环境参考|simulation" "M6 drill ran without degradation"
reject_pattern "deploy/evidence/m7-ebpf-coverage-drill.log" "跳过|SKIP|占位|待采集" "M7 drill ran without skipped evidence"
reject_pattern "artifacts/ebpf-perf/latency-report.txt" "待采集|placeholder|占位" "latency report has real data"
reject_pattern "artifacts/ebpf-perf/cpu-report.txt" "待采集|placeholder|占位" "CPU report has real data"
reject_pattern "artifacts/ebpf-perf/memory-report.txt" "待采集|placeholder|占位" "memory report has real data"

echo "========================================"
echo "Result: $PASS passed, $FAIL failed"
echo "========================================"

if [[ "$FAIL" -gt 0 ]]; then
	exit 1
fi
