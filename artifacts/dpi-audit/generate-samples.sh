#!/usr/bin/env bash
# M6 抓包样本生成编排脚本
# 证据强度：
#   - 受控环境基线（真实 Linux loopback 抓包）
#   - 模拟环境参考（自动降级，生成 synthetic pcapng + metadata）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EVIDENCE_DIR="$PROJECT_ROOT/deploy/evidence"
LOG_FILE="$EVIDENCE_DIR/m6-generate-samples.log"
REQUIREMENTS_FILE="$SCRIPT_DIR/requirements.txt"

source "$PROJECT_ROOT/deploy/scripts/common-go.sh"
source "$PROJECT_ROOT/deploy/scripts/common-python.sh"

HANDSHAKE_DIR="$SCRIPT_DIR/handshake"
PKTLEN_DIR="$SCRIPT_DIR/packet-length"
TIMING_DIR="$SCRIPT_DIR/timing"

mkdir -p "$EVIDENCE_DIR" "$HANDSHAKE_DIR" "$PKTLEN_DIR" "$TIMING_DIR"

DEGRADED=false
declare -a DEGRADE_REASONS=()

log() {
	echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*" | tee -a "$LOG_FILE"
}

add_degrade_reason() {
	DEGRADED=true
	DEGRADE_REASONS+=("$1")
}

has_cap_net_raw() {
	if [[ ${EUID:-1} -eq 0 ]]; then
		return 0
	fi
	command -v capsh >/dev/null 2>&1 && capsh --print 2>/dev/null | grep -q "cap_net_raw"
}

capture_helpers_ready() {
	local ebpf_dir="$PROJECT_ROOT/mirage-gateway/pkg/ebpf"
	local pattern
	for pattern in \
		"func TestBDNASYNCapture" \
		"func TestUTLSSYNCapture" \
		"func TestNPMCapture_Mode0" \
		"func TestNPMCapture_Mode1" \
		"func TestNPMCapture_Mode2" \
		"func TestNPMCapture_Baseline" \
		"func TestJitterCapture_none" \
		"func TestJitterCapture_jitter-only" \
		"func TestJitterCapture_vpc-only" \
		"func TestJitterCapture_jitter-vpc"; do
		if ! grep -R -q "$pattern" "$ebpf_dir"/*.go 2>/dev/null; then
			return 1
		fi
	done
	return 0
}

check_environment() {
	: > "$LOG_FILE"
	log "=========================================="
	log "M6 抓包样本生成编排"
	log "目标：优先真实抓包，失败时自动降级为模拟样本"
	log "=========================================="

	if [[ "$(uname -s)" != "Linux" ]]; then
		add_degrade_reason "非 Linux 环境"
	else
		log "运行环境: Linux $(uname -r)"
	fi

	if ! command -v tcpdump >/dev/null 2>&1; then
		add_degrade_reason "tcpdump 不可用"
	else
		log "tcpdump: $(tcpdump --version 2>&1 | head -1)"
	fi

	if ! has_cap_net_raw; then
		add_degrade_reason "缺少 root/CAP_NET_RAW，无法在 loopback 抓包"
	else
		log "抓包权限: 已满足"
	fi

	if init_go_runner "$PROJECT_ROOT" 23; then
		log_go_runner | tee -a "$LOG_FILE"
	else
		add_degrade_reason "Go >= 1.23 不可用，真实抓包 helper 无法执行"
	fi

	if init_python_runner "$PROJECT_ROOT" 10; then
		log_python_runner | tee -a "$LOG_FILE"
	else
		log "FATAL: 无可用 Python 运行器"
		exit 1
	fi

	if ensure_python_modules "$REQUIREMENTS_FILE" scapy; then
		log "Python 依赖: scapy 可用"
	else
		log "FATAL: 无法准备 scapy 依赖"
		exit 1
	fi

	if ! capture_helpers_ready; then
		add_degrade_reason "真实抓包 helper 测试尚未在仓库中落地"
	fi
}

generate_simulation_samples() {
	log ""
	log "切换到模拟模式：${DEGRADE_REASONS[*]}"
	run_python_script "artifacts/dpi-audit" "generate-simulated-samples.py" 2>&1 | tee -a "$LOG_FILE"

	for f in \
		"$HANDSHAKE_DIR/remirage-syn.pcapng" \
		"$HANDSHAKE_DIR/chrome-syn.pcapng" \
		"$HANDSHAKE_DIR/utls-syn.pcapng" \
		"$PKTLEN_DIR/baseline-no-padding.pcapng" \
		"$PKTLEN_DIR/mode-fixed-mtu.pcapng" \
		"$PKTLEN_DIR/mode-random-range.pcapng" \
		"$PKTLEN_DIR/mode-gaussian.pcapng" \
		"$TIMING_DIR/config-none.pcapng" \
		"$TIMING_DIR/config-jitter-only.pcapng" \
		"$TIMING_DIR/config-vpc-only.pcapng" \
		"$TIMING_DIR/config-jitter-vpc.pcapng"; do
		if [[ -f "$f" ]]; then
			log "  ✓ $(realpath --relative-to="$SCRIPT_DIR" "$f" 2>/dev/null || echo "$f")"
		else
			log "  ✗ 缺失: $f"
			return 1
		fi
	done

	log "模拟样本生成完成"
}

main() {
	check_environment

	if $DEGRADED; then
		generate_simulation_samples
		exit 0
	fi

	log ""
	log "真实抓包模式已满足，但当前仓库尚未启用该路径；为避免输出虚假证据，改为失败退出。"
	log "请在落地真实抓包 helper 后删除该保护分支。"
	exit 1
}

main "$@"
