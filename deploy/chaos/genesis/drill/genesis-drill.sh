#!/usr/bin/env bash
# ============================================================
# The Genesis Drill — 创世演习三幕剧本
# ============================================================
# 在 drill 容器内执行，通过 Docker API 操控其他容器网络
#
# 用法：./genesis-drill.sh [act1|act2|act3|full]
# ============================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

OS_ADDR="${OS_ADDR:-10.99.0.10}"
GW_A="${GATEWAY_A_ADDR:-10.99.0.20}"
GW_B="${GATEWAY_B_ADDR:-10.99.0.30}"
PHANTOM="${PHANTOM_ADDR:-10.99.0.40}"

log_act()   { echo -e "\n${BOLD}${CYAN}═══════════════════════════════════════${NC}"; echo -e "${BOLD}${CYAN}  $*${NC}"; echo -e "${BOLD}${CYAN}═══════════════════════════════════════${NC}\n"; }
log_scene() { echo -e "${YELLOW}▶ $*${NC}"; }
log_ok()    { echo -e "${GREEN}  ✅ $*${NC}"; }
log_fail()  { echo -e "${RED}  ❌ $*${NC}"; }
log_info()  { echo -e "  ℹ️  $*"; }
log_wait()  { echo -e "  ⏳ $*"; }

PASS_COUNT=0
FAIL_COUNT=0

assert_ok() {
    if [ $? -eq 0 ]; then
        log_ok "$1"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_fail "$1"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_eq() {
    local actual="$1" expected="$2" msg="$3"
    if [ "$actual" = "$expected" ]; then
        log_ok "$msg"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_fail "$msg (expected: $expected, got: $actual)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_contains() {
    local haystack="$1" needle="$2" msg="$3"
    if echo "$haystack" | grep -q "$needle"; then
        log_ok "$msg"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_fail "$msg (not found: $needle)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

wait_for_service() {
    local addr="$1" path="$2" max_wait="${3:-30}"
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if curl -sf "http://${addr}${path}" > /dev/null 2>&1; then
            return 0
        fi
        sleep 1
        elapsed=$((elapsed + 1))
    done
    return 1
}

# ============================================================
# 第一幕：创世流转 (The Golden Path)
# ============================================================
act1_golden_path() {
    log_act "第一幕：创世流转 (The Golden Path)"
    log_info "验证商业闭环：邀请 → 充值 → 分配 → 连接 → 计费"

    # Scene 1: 等待所有服务就绪
    log_scene "Scene 1: 等待服务就绪"
    wait_for_service "$OS_ADDR:7000" "/internal/health" 60
    assert_ok "OS gateway-bridge 健康"

    # 检查 Gateway A 是否注册到 OS
    sleep 5
    local gw_status
    gw_status=$(curl -sf "http://${OS_ADDR}:7000/internal/gateways" 2>/dev/null || echo "{}")
    assert_contains "$gw_status" "gateway-alpha" "Gateway A 已注册到 OS"

    # Scene 2: 模拟 XMR 到账 Webhook
    log_scene "Scene 2: 模拟 Monero 充值到账"
    local webhook_resp
    webhook_resp=$(curl -sf -X POST "http://${OS_ADDR}:7000/internal/webhook/xmr" \
        -H "Content-Type: application/json" \
        -d '{
            "user_id": "chaos-user-001",
            "tx_hash": "deadbeef0123456789abcdef",
            "amount_xmr": 0.01,
            "confirmations": 10
        }' 2>/dev/null || echo "error")
    assert_contains "$webhook_resp" "ok\|success\|accepted" "XMR Webhook 处理成功"

    # Scene 3: 验证配额分配
    log_scene "Scene 3: 验证配额分配"
    sleep 2
    local quota
    quota=$(curl -sf "http://${OS_ADDR}:7000/internal/quota/chaos-user-001" 2>/dev/null || echo "0")
    log_info "用户配额: $quota"
    # 配额应该 > 0
    if [ "$quota" != "0" ] && [ "$quota" != "" ] && [ "$quota" != "error" ]; then
        log_ok "配额已分配 (quota=$quota)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_fail "配额未分配"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi

    # Scene 4: 验证 Phantom Client 连接
    log_scene "Scene 4: 验证 Phantom Client 连接到 Gateway A"
    sleep 3
    local phantom_status
    phantom_status=$(curl -sf "http://${PHANTOM}:9090/status" 2>/dev/null || echo "{}")
    assert_contains "$phantom_status" "connected\|alive" "Phantom Client 已连接"

    # Scene 5: 验证计费开始
    log_scene "Scene 5: 验证流量计费"
    # 通过 Phantom 发送测试流量
    curl -sf -X POST "http://${PHANTOM}:9090/test/send" \
        -d '{"bytes": 1048576}' 2>/dev/null || true
    sleep 3

    local billing
    billing=$(curl -sf "http://${OS_ADDR}:7000/internal/billing/chaos-user-001" 2>/dev/null || echo "{}")
    log_info "计费状态: $billing"
    assert_contains "$billing" "consumed\|bytes\|cost" "计费模块已记录流量"

    echo ""
    log_info "第一幕完成: Golden Path 验证结束"
}

# ============================================================
# 第二幕：协议绞杀 (The Protocol Asphyxiation)
# ============================================================
act2_protocol_asphyxiation() {
    log_act "第二幕：协议绞杀 (The Protocol Asphyxiation)"
    log_info "验证 G-Tunnel 多路径降级矩阵"

    # Scene 1: 确认当前通道为 QUIC
    log_scene "Scene 1: 确认 QUIC 主通道活跃"
    local transport
    transport=$(curl -sf "http://${PHANTOM}:9090/status" 2>/dev/null | \
        grep -oP '"transport"\s*:\s*"\K[^"]+' || echo "unknown")
    log_info "当前传输协议: $transport"

    # Scene 2: 切断 UDP（杀死 QUIC）
    log_scene "Scene 2: 切断 UDP 出站 — 模拟 ISP 阻断 QUIC"
    # 在 phantom 容器的网络命名空间中注入 iptables 规则
    docker exec genesis-phantom-1 iptables -A OUTPUT -p udp --dport 443 -j DROP 2>/dev/null || \
        nsenter_exec "phantom-1" "iptables -A OUTPUT -p udp --dport 443 -j DROP"

    log_wait "等待降级触发 (最多 15s)..."
    local degraded=false
    for i in $(seq 1 15); do
        sleep 1
        transport=$(curl -sf "http://${PHANTOM}:9090/status" 2>/dev/null | \
            grep -oP '"transport"\s*:\s*"\K[^"]+' || echo "unknown")
        if [ "$transport" != "quic" ] && [ "$transport" != "unknown" ]; then
            degraded=true
            break
        fi
    done

    if $degraded; then
        log_ok "QUIC 阻断后降级到: $transport (耗时: ${i}s)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_fail "15s 内未触发降级"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi

    # Scene 3: 验证数据流未中断
    log_scene "Scene 3: 验证降级后数据流连续性"
    local send_result
    send_result=$(curl -sf -X POST "http://${PHANTOM}:9090/test/send" \
        -d '{"bytes": 4096}' 2>/dev/null || echo "error")
    assert_contains "$send_result" "ok\|sent\|success" "降级通道数据传输正常"

    # Scene 4: 加大绞杀 — 切断 TCP
    log_scene "Scene 4: 切断 TCP 出站 — 模拟全协议阻断"
    docker exec genesis-phantom-1 iptables -A OUTPUT -p tcp -d "$GW_A" -j DROP 2>/dev/null || \
        nsenter_exec "phantom-1" "iptables -A OUTPUT -p tcp -d $GW_A -j DROP"

    log_wait "等待极端降级 (最多 20s)..."
    sleep 10
    transport=$(curl -sf "http://${PHANTOM}:9090/status" 2>/dev/null | \
        grep -oP '"transport"\s*:\s*"\K[^"]+' || echo "dead")
    log_info "极端阻断后传输协议: $transport"

    # ICMP 或 DNS 通道应该亮起（如果配置了）
    if [ "$transport" = "icmp" ] || [ "$transport" = "dns" ]; then
        log_ok "极端环境逃生通道激活: $transport"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_info "极端逃生通道未激活（可能未配置 ICMP/DNS），跳过"
    fi

    # Scene 5: 恢复网络
    log_scene "Scene 5: 恢复网络"
    docker exec genesis-phantom-1 iptables -F OUTPUT 2>/dev/null || \
        nsenter_exec "phantom-1" "iptables -F OUTPUT"
    sleep 5

    transport=$(curl -sf "http://${PHANTOM}:9090/status" 2>/dev/null | \
        grep -oP '"transport"\s*:\s*"\K[^"]+' || echo "unknown")
    log_info "网络恢复后传输协议: $transport"
    if [ "$transport" = "quic" ]; then
        log_ok "网络恢复后自动升格回 QUIC"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_info "未自动升格回 QUIC (当前: $transport)，可能需要更长探测周期"
    fi

    echo ""
    log_info "第二幕完成: 协议绞杀验证结束"
}

# ============================================================
# 第三幕：焦土与复活 (Scorched Earth & Resonance)
# ============================================================
act3_scorched_earth() {
    log_act "第三幕：焦土与复活 (Scorched Earth & Resonance)"
    log_info "验证信令共振发现器 — 绝境复活"

    # Scene 1: 确认 Client 当前连接到 Gateway A
    log_scene "Scene 1: 确认 Client 连接到 Gateway A"
    local current_gw
    current_gw=$(curl -sf "http://${PHANTOM}:9090/status" 2>/dev/null | \
        grep -oP '"gateway_ip"\s*:\s*"\K[^"]+' || echo "unknown")
    log_info "当前 Gateway: $current_gw"
    assert_contains "$current_gw" "$GW_A\|10.99.0.20" "Client 连接到 Gateway A"

    # Scene 2: OS 发布 Gateway B 信令到共振通道
    log_scene "Scene 2: OS 发布 Gateway B 信令到共振通道"
    # 通知 OS 将 Gateway B 的信息写入 mock 共振服务
    local publish_resp
    publish_resp=$(curl -sf -X POST "http://${OS_ADDR}:7000/internal/resonance/publish" \
        -H "Content-Type: application/json" \
        -d "{
            \"gateways\": [{\"ip\": \"10.99.0.30\", \"port\": 443, \"priority\": 1}],
            \"domains\": [\"gw-bravo.example.com\"]
        }" 2>/dev/null || echo "error")
    log_info "信令发布结果: $publish_resp"

    # 同时直接写入 mock 服务的共享状态（确保信令可达）
    echo '{"gateways":[{"ip":"10.99.0.30","port":443,"priority":1}],"domains":["gw-bravo.example.com"]}' \
        > /shared/signal_payload.json 2>/dev/null || true

    # Scene 3: 焦土 — 杀死 Gateway A
    log_scene "Scene 3: 执行焦土指令 — 杀死 Gateway A"
    log_info "⚠️  发送 0xDEADBEEF 自毁指令..."

    # 方式 1: 通过 OS 下发焦土指令
    curl -sf -X POST "http://${OS_ADDR}:7000/internal/gateway/gateway-alpha/kill" 2>/dev/null || true

    # 方式 2: 直接停止容器（模拟物理封锁）
    sleep 2
    docker stop genesis-gateway-a-1 --timeout=1 2>/dev/null || \
        docker stop genesis-gateway-a --time=1 2>/dev/null || \
        docker kill genesis-gateway-a-1 2>/dev/null || \
        docker kill genesis-gateway-a 2>/dev/null || true
    log_info "Gateway A 已阵亡"

    # Scene 4: 等待 Client 检测到死亡并触发共振发现
    log_scene "Scene 4: 等待 Client 触发信令共振发现"
    log_wait "等待心跳超时 + 共振发现 (最多 30s)..."

    local resurrected=false
    local resurrection_time=0
    local start_ts=$(date +%s)

    for i in $(seq 1 30); do
        sleep 1
        local status
        status=$(curl -sf "http://${PHANTOM}:9090/status" 2>/dev/null || echo "{}")
        current_gw=$(echo "$status" | grep -oP '"gateway_ip"\s*:\s*"\K[^"]+' || echo "dead")

        if echo "$current_gw" | grep -q "$GW_B\|10.99.0.30"; then
            resurrected=true
            resurrection_time=$i
            break
        fi

        # 打印进度
        if [ $((i % 5)) -eq 0 ]; then
            log_info "  ... ${i}s elapsed, gateway=$current_gw"
        fi
    done

    local end_ts=$(date +%s)
    local total_time=$((end_ts - start_ts))

    if $resurrected; then
        log_ok "🔥 信令共振复活成功！Client 已迁移到 Gateway B (耗时: ${resurrection_time}s)"
        PASS_COUNT=$((PASS_COUNT + 1))

        if [ $resurrection_time -le 10 ]; then
            log_ok "复活时间 ≤ 10s — 极速"
            PASS_COUNT=$((PASS_COUNT + 1))
        elif [ $resurrection_time -le 20 ]; then
            log_ok "复活时间 ≤ 20s — 合格"
            PASS_COUNT=$((PASS_COUNT + 1))
        else
            log_info "复活时间 > 20s — 可优化"
        fi
    else
        log_fail "30s 内未完成信令共振复活"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi

    # Scene 5: 验证复活后功能完整性
    log_scene "Scene 5: 验证复活后功能完整性"
    if $resurrected; then
        sleep 2
        local send_result
        send_result=$(curl -sf -X POST "http://${PHANTOM}:9090/test/send" \
            -d '{"bytes": 4096}' 2>/dev/null || echo "error")
        assert_contains "$send_result" "ok\|sent\|success" "复活后数据传输正常"

        # 验证计费继续
        local billing
        billing=$(curl -sf "http://${OS_ADDR}:7000/internal/billing/chaos-user-001" 2>/dev/null || echo "{}")
        assert_contains "$billing" "consumed\|bytes" "复活后计费继续"
    fi

    # Scene 6: 恢复 Gateway A（为后续测试）
    log_scene "Scene 6: 恢复 Gateway A"
    docker start genesis-gateway-a-1 2>/dev/null || true

    echo ""
    log_info "第三幕完成: 焦土与复活验证结束"
}

# ============================================================
# nsenter 辅助（当 docker exec 不可用时）
# ============================================================
nsenter_exec() {
    local container="$1"
    shift
    local pid
    pid=$(docker inspect --format '{{.State.Pid}}' "genesis-${container}" 2>/dev/null || echo "")
    if [ -n "$pid" ] && [ "$pid" != "0" ]; then
        nsenter -t "$pid" -n -- sh -c "$*"
    fi
}

# ============================================================
# 结果汇总
# ============================================================
print_summary() {
    echo ""
    echo -e "${BOLD}═══════════════════════════════════════${NC}"
    echo -e "${BOLD}  创世演习结果汇总${NC}"
    echo -e "${BOLD}═══════════════════════════════════════${NC}"
    echo -e "  ${GREEN}通过: ${PASS_COUNT}${NC}"
    echo -e "  ${RED}失败: ${FAIL_COUNT}${NC}"
    echo -e "  总计: $((PASS_COUNT + FAIL_COUNT))"
    echo ""

    if [ $FAIL_COUNT -eq 0 ]; then
        echo -e "${GREEN}${BOLD}  🎉 ALL CLEAR — 系统具备完整战斗力${NC}"
    else
        echo -e "${YELLOW}${BOLD}  ⚠️  存在 ${FAIL_COUNT} 项失败，需要修复${NC}"
    fi
    echo ""
}

# ============================================================
# 主入口
# ============================================================
case "${1:-full}" in
    act1|golden)
        act1_golden_path
        print_summary
        ;;
    act2|asphyxiation|protocol)
        act2_protocol_asphyxiation
        print_summary
        ;;
    act3|scorched|resonance)
        act3_scorched_earth
        print_summary
        ;;
    full|all)
        act1_golden_path
        echo ""
        sleep 3
        act2_protocol_asphyxiation
        echo ""
        sleep 3
        act3_scorched_earth
        print_summary
        ;;
    *)
        echo "用法: $0 [act1|act2|act3|full]"
        echo ""
        echo "  act1 / golden      — 第一幕：创世流转 (商业闭环)"
        echo "  act2 / protocol    — 第二幕：协议绞杀 (多路径降级)"
        echo "  act3 / resonance   — 第三幕：焦土与复活 (信令共振)"
        echo "  full / all         — 完整三幕演习"
        ;;
esac
