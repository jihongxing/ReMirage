#!/usr/bin/env bash
# Mirage Project - 高压实弹演练
# 测试 Jitter-Lite + eBPF Padding 下的长连接稳定性
# 以及 G-Switch 5 秒转生时 WebSocket 的平滑重连

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_fail()  { echo -e "${RED}[FAIL]${NC} $*"; }
log_data()  { echo -e "${CYAN}[DATA]${NC} $*"; }

GATEWAY_HEALTH="http://127.0.0.1:8081"
RESULTS_DIR="/tmp/mirage-highpressure-$(date +%s)"
mkdir -p "$RESULTS_DIR"

# ============================================
# 测试 1: WebSocket 长连接延迟基线
# 在 Jitter-Lite 开启状态下测量 WebSocket 往返延迟
# ============================================
test_websocket_latency() {
    log_info "=== WebSocket 长连接延迟基线测试 ==="
    log_info "测量 Jitter-Lite + eBPF Padding 下的 WebSocket RTT"

    local ws_endpoint="${1:-wss://localhost:443/ws}"
    local duration="${2:-60}"
    local output="$RESULTS_DIR/ws_latency.csv"

    echo "timestamp,rtt_ms,jitter_ms" > "$output"

    # 使用 websocat 进行延迟测量
    if ! command -v websocat &>/dev/null; then
        log_warn "websocat 未安装，使用 curl 替代"
        # Fallback: 使用 HTTP 长轮询模拟
        local start_ts=$(date +%s)
        local count=0
        local total_rtt=0
        local prev_rtt=0

        while [ $(($(date +%s) - start_ts)) -lt "$duration" ]; do
            local req_start=$(date +%s%N)
            curl -sf -o /dev/null -w '%{time_total}' "$GATEWAY_HEALTH/healthz" 2>/dev/null || true
            local req_end=$(date +%s%N)
            local rtt_ms=$(( (req_end - req_start) / 1000000 ))

            local jitter_ms=0
            if [ $prev_rtt -gt 0 ]; then
                jitter_ms=$(( rtt_ms - prev_rtt ))
                [ $jitter_ms -lt 0 ] && jitter_ms=$(( -jitter_ms ))
            fi

            echo "$(date +%s),$rtt_ms,$jitter_ms" >> "$output"
            total_rtt=$((total_rtt + rtt_ms))
            count=$((count + 1))
            prev_rtt=$rtt_ms

            sleep 0.1
        done

        if [ $count -gt 0 ]; then
            local avg_rtt=$((total_rtt / count))
            log_data "采样数: $count, 平均 RTT: ${avg_rtt}ms"
        fi
    else
        # 使用 websocat 进行真实 WebSocket 测试
        log_info "使用 websocat 连接 $ws_endpoint"
        timeout "$duration" bash -c "
            seq 1 1000 | while read i; do
                echo '{\"type\":\"ping\",\"ts\":'$(date +%s%N)'}'
                sleep 0.1
            done | websocat -t '$ws_endpoint' 2>/dev/null | while read line; do
                echo \"\$(date +%s),\$line\" >> '$output'
            done
        " || true
    fi

    log_info "结果已保存: $output"

    # 分析结果
    if [ -f "$output" ] && [ "$(wc -l < "$output")" -gt 2 ]; then
        local p50=$(sort -t, -k2 -n "$output" | tail -n +2 | awk -F, '{print $2}' | \
            awk '{a[NR]=$1} END {print a[int(NR*0.5)]}')
        local p99=$(sort -t, -k2 -n "$output" | tail -n +2 | awk -F, '{print $2}' | \
            awk '{a[NR]=$1} END {print a[int(NR*0.99)]}')
        local max_jitter=$(awk -F, 'NR>1{print $3}' "$output" | sort -n | tail -1)

        log_data "P50 RTT: ${p50:-N/A}ms"
        log_data "P99 RTT: ${p99:-N/A}ms"
        log_data "最大 Jitter: ${max_jitter:-N/A}ms"

        # 判定
        if [ "${p99:-999}" -lt 200 ]; then
            log_info "✅ P99 < 200ms，延迟可接受"
        else
            log_warn "⚠️ P99 >= 200ms，可能影响实时业务"
        fi
    fi
}

# ============================================
# 测试 2: G-Switch 转生时长连接平滑度
# 在 WebSocket 活跃期间触发域名转生，测量断连时间
# ============================================
test_gswitch_reconnect() {
    log_info "=== G-Switch 转生平滑重连测试 ==="

    local output="$RESULTS_DIR/gswitch_reconnect.log"
    > "$output"

    # 1. 启动后台 WebSocket 心跳（模拟活跃长连接）
    log_info "启动后台心跳..."
    local heartbeat_pid=""
    (
        while true; do
            local ts=$(date +%s%N)
            local result=$(curl -sf -o /dev/null -w '%{http_code}:%{time_total}' \
                "$GATEWAY_HEALTH/healthz" 2>/dev/null || echo "000:0")
            echo "$ts,$result" >> "$output"
            sleep 0.5
        done
    ) &
    heartbeat_pid=$!

    sleep 5  # 等待基线稳定

    # 2. 触发 G-Switch 强制转生
    log_info "触发 G-Switch 强制转生..."
    local switch_ts=$(date +%s%N)

    # 通过 API 触发转生
    curl -sf -X POST "$GATEWAY_HEALTH/debug/gswitch/force" 2>/dev/null || \
        log_warn "API 触发失败，尝试 gRPC"

    # 3. 等待转生完成
    sleep 10

    # 4. 停止心跳
    kill $heartbeat_pid 2>/dev/null || true
    wait $heartbeat_pid 2>/dev/null || true

    # 5. 分析断连窗口
    log_info "分析断连窗口..."

    local total_probes=$(wc -l < "$output")
    local failed_probes=$(grep -c "000:" "$output" 2>/dev/null || echo "0")
    local switch_ts_sec=$((switch_ts / 1000000000))

    # 找到转生后第一个失败和最后一个失败
    local first_fail=""
    local last_fail=""
    while IFS=, read -r ts result; do
        local ts_sec=$((ts / 1000000000))
        if [ $ts_sec -ge $switch_ts_sec ] && echo "$result" | grep -q "^000:"; then
            [ -z "$first_fail" ] && first_fail=$ts
            last_fail=$ts
        fi
    done < "$output"

    if [ -n "$first_fail" ] && [ -n "$last_fail" ]; then
        local gap_ms=$(( (last_fail - first_fail) / 1000000 ))
        log_data "断连窗口: ${gap_ms}ms"
        log_data "总探测: $total_probes, 失败: $failed_probes"

        if [ $gap_ms -le 5000 ]; then
            log_info "✅ 断连窗口 ≤ 5s，G-Switch 转生平滑"
        else
            log_fail "❌ 断连窗口 > 5s (${gap_ms}ms)，需要优化重连逻辑"
        fi
    else
        if [ "$failed_probes" -eq 0 ]; then
            log_info "✅ 零断连，G-Switch 转生完全透明"
        else
            log_warn "⚠️ 无法确定断连窗口"
        fi
    fi

    log_info "结果已保存: $output"
}

# ============================================
# 测试 3: 高并发吞吐量（eBPF 数据面压力）
# ============================================
test_throughput() {
    log_info "=== eBPF 数据面吞吐量压力测试 ==="

    local duration="${1:-30}"
    local connections="${2:-100}"
    local output="$RESULTS_DIR/throughput.log"

    # 使用 iperf3 或 curl 并发
    if command -v iperf3 &>/dev/null; then
        log_info "使用 iperf3 测试 (${connections} 并发, ${duration}s)"
        iperf3 -c 127.0.0.1 -p 5201 -t "$duration" -P "$connections" \
            --json > "$output" 2>/dev/null || {
            log_warn "iperf3 连接失败，使用 curl 替代"
        }
    fi

    # Fallback: curl 并发下载
    log_info "使用 curl 并发测试 (${connections} 连接)"
    local start_ts=$(date +%s)
    local pids=()

    for i in $(seq 1 "$connections"); do
        (
            local bytes=0
            while [ $(($(date +%s) - start_ts)) -lt "$duration" ]; do
                local size=$(curl -sf -o /dev/null -w '%{size_download}' \
                    "$GATEWAY_HEALTH/status" 2>/dev/null || echo "0")
                bytes=$((bytes + size))
            done
            echo "$i,$bytes" >> "${output}.csv"
        ) &
        pids+=($!)
    done

    # 等待所有连接完成
    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    # 统计
    if [ -f "${output}.csv" ]; then
        local total_bytes=$(awk -F, '{sum+=$2} END {print sum}' "${output}.csv")
        local total_mb=$((total_bytes / 1024 / 1024))
        local mbps=$((total_mb * 8 / duration))
        log_data "总传输: ${total_mb}MB, 吞吐: ${mbps}Mbps"
    fi

    # 检查 eBPF 统计
    log_info "eBPF 数据面统计:"
    bpftool map dump name sockmap_stats 2>/dev/null | head -20 || \
        log_warn "无法读取 eBPF 统计"
}

# ============================================
# 测试 4: 防御开销测量
# 对比开启/关闭 Jitter+Padding 的延迟差异
# ============================================
test_defense_overhead() {
    log_info "=== 防御开销测量 ==="

    local samples=100
    local output="$RESULTS_DIR/defense_overhead.csv"
    echo "sample,defense_level,rtt_ms" > "$output"

    for level in 0 10 20 30; do
        log_info "测试防御等级: $level"

        # 设置防御等级
        curl -sf -X POST "$GATEWAY_HEALTH/debug/defense/$level" 2>/dev/null || true
        sleep 2  # 等待策略生效

        for i in $(seq 1 $samples); do
            local rtt=$(curl -sf -o /dev/null -w '%{time_total}' \
                "$GATEWAY_HEALTH/healthz" 2>/dev/null || echo "0")
            local rtt_ms=$(echo "$rtt * 1000" | bc 2>/dev/null || echo "0")
            echo "$i,$level,$rtt_ms" >> "$output"
        done
    done

    # 分析
    log_info "防御开销分析:"
    for level in 0 10 20 30; do
        local avg=$(awk -F, -v l="$level" '$2==l{sum+=$3;n++} END{if(n>0)print sum/n;else print 0}' "$output")
        log_data "  等级 $level: 平均 RTT = ${avg}ms"
    done

    # 恢复默认等级
    curl -sf -X POST "$GATEWAY_HEALTH/debug/defense/20" 2>/dev/null || true

    log_info "结果已保存: $output"
}

# ============================================
# 主入口
# ============================================
case "${1:-help}" in
    ws_latency)       test_websocket_latency "${2:-}" "${3:-60}" ;;
    gswitch_reconnect) test_gswitch_reconnect ;;
    throughput)       test_throughput "${2:-30}" "${3:-100}" ;;
    defense_overhead) test_defense_overhead ;;
    full)
        test_websocket_latency "" 30
        echo ""
        test_defense_overhead
        echo ""
        test_throughput 15 50
        echo ""
        test_gswitch_reconnect
        echo ""
        log_info "=== 全部测试完成 ==="
        log_info "结果目录: $RESULTS_DIR"
        ;;
    *)
        echo "用法: $0 <test_name> [options]"
        echo ""
        echo "  ws_latency [endpoint] [duration_sec]"
        echo "    WebSocket 长连接延迟基线（Jitter-Lite 开启）"
        echo ""
        echo "  gswitch_reconnect"
        echo "    G-Switch 转生时长连接平滑重连测试"
        echo ""
        echo "  throughput [duration_sec] [connections]"
        echo "    eBPF 数据面吞吐量压力测试"
        echo ""
        echo "  defense_overhead"
        echo "    防御开销测量（对比不同等级延迟差异）"
        echo ""
        echo "  full"
        echo "    全部测试"
        ;;
esac
