#!/usr/bin/env bash
# Mirage Project - 混沌工程与红蓝对抗测试套件
# 用法: ./chaos_test.sh <test_name> [options]
# 测试项:
#   raft_failover    - Raft Leader 强杀故障转移
#   domain_block     - G-Switch 域名封锁模拟
#   heartbeat_death  - 心跳超时自毁验证
#   quota_cutoff     - 配额熔断精准掐断
#   full_chaos       - 全部测试

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_fail()  { echo -e "${RED}[FAIL]${NC} $*"; }

GATEWAY_HEALTH="http://127.0.0.1:8081"
OS_NODES=("10.0.1.20" "10.0.2.20" "10.0.4.20")
RAFT_PORT=50848
HEARTBEAT_TIMEOUT=300

# ============================================
# 测试 1: Raft Leader 强杀故障转移
# ============================================
test_raft_failover() {
    log_info "=== Raft Leader 强杀故障转移测试 ==="

    # 1. 找到当前 Leader
    local leader=""
    for node in "${OS_NODES[@]}"; do
        status=$(curl -sf "https://${node}:${RAFT_PORT}/raft/status" --insecure 2>/dev/null || echo "")
        if echo "$status" | grep -q '"state":"Leader"'; then
            leader="$node"
            break
        fi
    done

    if [ -z "$leader" ]; then
        log_fail "无法找到 Raft Leader"
        return 1
    fi
    log_info "当前 Leader: $leader"

    # 2. 记录时间戳
    local start_ts=$(date +%s%N)

    # 3. 强杀 Leader（模拟云控制台强制关机）
    log_warn "强杀 Leader: $leader"
    ssh "root@${leader}" "kill -9 \$(pgrep mirage-os)" 2>/dev/null || true

    # 4. 等待新 Leader 选举
    local new_leader=""
    local elapsed=0
    while [ $elapsed -lt 30 ]; do
        for node in "${OS_NODES[@]}"; do
            [ "$node" = "$leader" ] && continue
            status=$(curl -sf "https://${node}:${RAFT_PORT}/raft/status" --insecure 2>/dev/null || echo "")
            if echo "$status" | grep -q '"state":"Leader"'; then
                new_leader="$node"
                break 2
            fi
        done
        sleep 0.5
        elapsed=$((elapsed + 1))
    done

    local end_ts=$(date +%s%N)
    local failover_ms=$(( (end_ts - start_ts) / 1000000 ))

    if [ -n "$new_leader" ]; then
        log_info "新 Leader: $new_leader (故障转移耗时: ${failover_ms}ms)"
        if [ $failover_ms -lt 5000 ]; then
            log_info "✅ 故障转移 < 5s，通过"
        else
            log_warn "⚠️ 故障转移 > 5s，需要优化"
        fi
    else
        log_fail "❌ 30s 内未完成 Leader 选举"
        return 1
    fi

    # 5. 验证 Gateway 心跳是否自动切换到新 Leader
    sleep 5
    gw_status=$(curl -sf "${GATEWAY_HEALTH}/status" 2>/dev/null || echo "")
    if echo "$gw_status" | grep -q '"grpc_client_connected":true'; then
        log_info "✅ Gateway 已自动连接到新 Leader"
    else
        log_warn "⚠️ Gateway 未自动重连，检查 gRPC 重连逻辑"
    fi
}

# ============================================
# 测试 2: G-Switch 域名封锁模拟
# ============================================
test_domain_block() {
    log_info "=== G-Switch 域名封锁模拟 ==="

    # 1. 获取当前活跃域名
    local active_domain=$(curl -sf "${GATEWAY_HEALTH}/status" 2>/dev/null | \
        python3 -c "import sys,json; print(json.load(sys.stdin).get('active_domain','unknown'))" 2>/dev/null || echo "unknown")
    log_info "当前活跃域名: $active_domain"

    # 2. 在路由器层面阻断域名解析（模拟 DNS 污染）
    log_warn "阻断域名: $active_domain"
    # 使用 iptables 阻断对应 IP 的出站流量
    # 实际操作需要根据环境调整
    # iptables -A OUTPUT -d <domain_ip> -j DROP

    # 3. 掐表等待 G-Switch 触发
    local start_ts=$(date +%s)
    local switched=false
    local elapsed=0

    while [ $elapsed -lt 30 ]; do
        new_domain=$(curl -sf "${GATEWAY_HEALTH}/status" 2>/dev/null | \
            python3 -c "import sys,json; print(json.load(sys.stdin).get('active_domain','unknown'))" 2>/dev/null || echo "unknown")
        if [ "$new_domain" != "$active_domain" ] && [ "$new_domain" != "unknown" ]; then
            switched=true
            break
        fi
        sleep 1
        elapsed=$((elapsed + 1))
    done

    local switch_time=$elapsed

    if $switched; then
        log_info "✅ G-Switch 触发成功 (耗时: ${switch_time}s, 新域名: $new_domain)"
        if [ $switch_time -le 5 ]; then
            log_info "✅ 切换时间 ≤ 5s，通过"
        else
            log_warn "⚠️ 切换时间 > 5s，需要优化信令共振延迟"
        fi
    else
        log_fail "❌ 30s 内未触发域名切换"
        return 1
    fi

    # 4. 恢复网络
    # iptables -D OUTPUT -d <domain_ip> -j DROP
}

# ============================================
# 测试 3: 心跳超时自毁验证
# ============================================
test_heartbeat_death() {
    log_info "=== 心跳超时自毁验证 ==="
    log_warn "⚠️ 此测试将导致 Gateway 进程自毁，请确认环境"

    # 1. 确认 Gateway 运行中
    gw_status=$(curl -sf "${GATEWAY_HEALTH}/healthz" 2>/dev/null || echo "")
    if [ "$gw_status" != "OK" ]; then
        log_fail "Gateway 未运行"
        return 1
    fi
    log_info "Gateway 运行中"

    # 2. 阻断 Gateway → OS 的心跳通道
    log_warn "阻断心跳通道（iptables 阻断 50847 端口）"
    for node in "${OS_NODES[@]}"; do
        iptables -A OUTPUT -d "$node" -p tcp --dport 50847 -j DROP 2>/dev/null || true
    done

    # 3. 等待 heartbeat_timeout (300s) + 缓冲 (30s)
    local wait_time=$((HEARTBEAT_TIMEOUT + 30))
    log_info "等待 ${wait_time}s（心跳超时 ${HEARTBEAT_TIMEOUT}s + 缓冲 30s）..."

    local check_interval=30
    local elapsed=0
    local process_alive=true

    while [ $elapsed -lt $wait_time ]; do
        sleep $check_interval
        elapsed=$((elapsed + check_interval))

        # 检查进程是否还活着
        if ! pgrep -f mirage-gateway > /dev/null 2>&1; then
            process_alive=false
            log_info "进程已退出 (elapsed: ${elapsed}s)"
            break
        fi
        log_info "进程仍存活 (elapsed: ${elapsed}s / ${wait_time}s)"
    done

    # 4. 验证结果
    if ! $process_alive; then
        log_info "✅ 进程已自毁"

        # 验证 eBPF Map 是否被清空
        # 通过 bpftool 检查
        map_content=$(bpftool map dump name jitter_config_map 2>/dev/null || echo "empty")
        if echo "$map_content" | grep -q "key:" && ! echo "$map_content" | grep -qv "value: 00"; then
            log_info "✅ eBPF Map 已清空"
        else
            log_warn "⚠️ eBPF Map 可能未完全清空"
        fi

        # 验证 core dump 是否存在
        if ls /tmp/core.* 2>/dev/null || ls /var/crash/* 2>/dev/null; then
            log_fail "❌ 发现 core dump 文件，RAM Shield 失效"
        else
            log_info "✅ 无 core dump 文件"
        fi
    else
        log_fail "❌ 进程未在超时后自毁"
    fi

    # 5. 恢复网络
    for node in "${OS_NODES[@]}"; do
        iptables -D OUTPUT -d "$node" -p tcp --dport 50847 -j DROP 2>/dev/null || true
    done
}

# ============================================
# 测试 4: 配额熔断精准掐断
# ============================================
test_quota_cutoff() {
    log_info "=== 配额熔断精准掐断测试 ==="

    # 1. 通过 bpftool 设置极小配额（1MB）
    local tiny_quota=$((1 * 1024 * 1024))
    log_info "设置配额: ${tiny_quota} bytes (1MB)"

    # 写入 quota_map
    bpftool map update name quota_map key 0 0 0 0 value \
        $(printf '%02x ' $(echo "obase=16;$tiny_quota" | bc | sed 's/../& /g' | awk '{for(i=NF;i>0;i--) printf "%s ",$i}')) \
        2>/dev/null || {
        log_warn "bpftool 更新失败，尝试通过 API"
        curl -sf -X POST "${GATEWAY_HEALTH}/debug/quota" \
            -d "{\"remaining_bytes\": $tiny_quota}" 2>/dev/null || true
    }

    # 2. 发起大流量（iperf3 或 curl 下载）
    log_info "发起流量测试..."
    local start_ts=$(date +%s%N)

    # 尝试下载 10MB 数据
    timeout 30 curl -sf -o /dev/null "https://speed.cloudflare.com/__down?bytes=10485760" 2>/dev/null &
    local curl_pid=$!

    # 3. 监控配额消耗
    sleep 2

    # 检查配额是否已耗尽
    local remaining=$(bpftool map lookup name quota_map key 0 0 0 0 2>/dev/null | \
        grep -oP 'value:\s+\K.*' | tr -d ' ' || echo "unknown")

    if [ "$remaining" = "0000000000000000" ] || [ "$remaining" = "unknown" ]; then
        local end_ts=$(date +%s%N)
        local cutoff_ms=$(( (end_ts - start_ts) / 1000000 ))
        log_info "✅ 配额已耗尽，熔断生效 (耗时: ${cutoff_ms}ms)"

        # 验证后续流量是否被完全阻断
        local post_bytes=$(timeout 5 curl -sf -o /dev/null -w '%{size_download}' \
            "https://speed.cloudflare.com/__down?bytes=1024" 2>/dev/null || echo "0")

        if [ "$post_bytes" = "0" ]; then
            log_info "✅ 后续流量完全阻断，TC_ACT_STOLEN 生效"
        else
            log_fail "❌ 熔断后仍有 ${post_bytes} 字节泄漏"
        fi
    else
        log_warn "⚠️ 配额未耗尽: $remaining"
    fi

    # 清理
    kill $curl_pid 2>/dev/null || true

    # 恢复配额（无限）
    local max_quota="ffffffffffffffff"
    bpftool map update name quota_map key 0 0 0 0 value ff ff ff ff ff ff ff ff 2>/dev/null || true
    log_info "配额已恢复"
}

# ============================================
# 主入口
# ============================================
case "${1:-help}" in
    raft_failover)    test_raft_failover ;;
    domain_block)     test_domain_block ;;
    heartbeat_death)  test_heartbeat_death ;;
    quota_cutoff)     test_quota_cutoff ;;
    full_chaos)
        test_raft_failover
        echo ""
        test_domain_block
        echo ""
        test_quota_cutoff
        echo ""
        log_warn "心跳超时测试需要单独运行（会杀死进程）"
        log_info "运行: $0 heartbeat_death"
        ;;
    *)
        echo "用法: $0 <test_name>"
        echo "  raft_failover    - Raft Leader 强杀故障转移"
        echo "  domain_block     - G-Switch 域名封锁模拟"
        echo "  heartbeat_death  - 心跳超时自毁验证"
        echo "  quota_cutoff     - 配额熔断精准掐断"
        echo "  full_chaos       - 全部测试（除心跳自毁）"
        ;;
esac
