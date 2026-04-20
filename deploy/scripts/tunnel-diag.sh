#!/bin/bash
# tunnel-diag.sh - G-Tunnel 多路径诊断脚本
# 用途：诊断各传输路径的连通性和性能
# 用法：bash tunnel-diag.sh [gateway_addr] [--full]

set -e

GATEWAY_ADDR=${1:-"127.0.0.1:9090"}
FULL_TEST=0

shift 2>/dev/null || true
while [ $# -gt 0 ]; do
    case "$1" in
        --full) FULL_TEST=1 ;;
    esac
    shift
done

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

echo "============================================"
echo " G-Tunnel 多路径诊断"
echo " Gateway: $GATEWAY_ADDR"
echo "============================================"
echo ""

# ─── 1. Gateway 隧道状态 ───
echo -e "${CYAN}--- 隧道状态 ---${NC}"
TUNNEL_RESP=$(curl -s --connect-timeout 3 --max-time 5 "http://${GATEWAY_ADDR}/api/tunnel/status" 2>/dev/null)
if [ -z "$TUNNEL_RESP" ]; then
    echo -e "${RED}  ❌ 无法连接 Gateway${NC}"
    exit 1
fi

ACTIVE=$(echo "$TUNNEL_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('active', False))" 2>/dev/null || echo "False")
if [ "$ACTIVE" = "True" ]; then
    echo -e "  ${GREEN}🟢 隧道已激活${NC}"
else
    echo -e "  ${RED}⚫ 隧道未激活${NC}"
fi
echo ""

# ─── 2. 各路径状态 ───
echo -e "${CYAN}--- 传输路径 ---${NC}"
PATHS_RESP=$(curl -s --connect-timeout 3 --max-time 5 "http://${GATEWAY_ADDR}/api/tunnel/paths" 2>/dev/null)
if [ -n "$PATHS_RESP" ]; then
    echo "$PATHS_RESP" | python3 -c "
import sys, json
try:
    paths = json.load(sys.stdin)
    if not isinstance(paths, list):
        paths = []
    icons = {'active': '🟢', 'standby': '🟡', 'degraded': '🟠', 'dead': '🔴'}
    print(f'  {\"Level\":<6} {\"Type\":<8} {\"Status\":<10} {\"RTT\":<10} {\"Loss\":<8} Endpoint')
    print('  ' + '─' * 65)
    for p in paths:
        icon = icons.get(p.get('status', ''), '❓')
        loss = f\"{p.get('loss_rate', 0)*100:.1f}%\"
        print(f'  {icon} L{p.get(\"level\", \"?\"):<4} {p.get(\"type\", \"?\"):<8} {p.get(\"status\", \"?\"):<10} {p.get(\"rtt\", \"—\"):<10} {loss:<8} {p.get(\"endpoint\", \"\")}')
except:
    print('  解析失败')
" 2>/dev/null || echo "  解析失败"
else
    echo "  无法获取路径信息"
fi
echo ""

# ─── 3. QUIC 连通性测试 ───
echo -e "${CYAN}--- QUIC (UDP 443) 连通性 ---${NC}"

# 从配置获取 QUIC 地址
QUIC_ADDR=$(echo "$PATHS_RESP" | python3 -c "
import sys, json
try:
    paths = json.load(sys.stdin)
    for p in paths:
        if p.get('type') == 'quic':
            print(p.get('endpoint', ''))
            break
except:
    pass
" 2>/dev/null)

if [ -n "$QUIC_ADDR" ]; then
    QUIC_HOST=$(echo "$QUIC_ADDR" | cut -d: -f1)
    QUIC_PORT=$(echo "$QUIC_ADDR" | cut -d: -f2)
    QUIC_PORT=${QUIC_PORT:-443}

    # DNS 解析
    QUIC_IP=$(dig +short "$QUIC_HOST" A 2>/dev/null | head -1)
    if [ -n "$QUIC_IP" ]; then
        echo "  DNS: $QUIC_HOST → $QUIC_IP"
    else
        echo -e "  ${YELLOW}⚠️  DNS 解析失败: $QUIC_HOST${NC}"
    fi

    # UDP 端口可达性
    if command -v nc &>/dev/null; then
        if nc -zu -w3 "$QUIC_HOST" "$QUIC_PORT" 2>/dev/null; then
            echo -e "  ${GREEN}✅ UDP $QUIC_HOST:$QUIC_PORT 可达${NC}"
        else
            echo -e "  ${RED}❌ UDP $QUIC_HOST:$QUIC_PORT 不可达${NC}"
        fi
    fi

    # ICMP ping
    if ping -c 3 -W 2 "$QUIC_HOST" >/dev/null 2>&1; then
        PING_RTT=$(ping -c 3 -W 2 "$QUIC_HOST" 2>/dev/null | tail -1 | awk -F'/' '{print $5}')
        echo "  Ping RTT: ${PING_RTT}ms"
    else
        echo -e "  ${YELLOW}⚠️  ICMP 不可达（可能被过滤）${NC}"
    fi
else
    echo "  未配置 QUIC 端点"
fi
echo ""

# ─── 4. WSS 连通性测试 ───
echo -e "${CYAN}--- WSS (TCP 443) 连通性 ---${NC}"

WSS_ENDPOINT=$(echo "$PATHS_RESP" | python3 -c "
import sys, json
try:
    paths = json.load(sys.stdin)
    for p in paths:
        if p.get('type') == 'wss':
            print(p.get('endpoint', ''))
            break
except:
    pass
" 2>/dev/null)

if [ -n "$WSS_ENDPOINT" ]; then
    # 提取 host
    WSS_HOST=$(echo "$WSS_ENDPOINT" | sed 's|wss://||' | cut -d: -f1 | cut -d/ -f1)
    WSS_PORT=$(echo "$WSS_ENDPOINT" | grep -oP ':\d+' | tr -d ':' || echo "443")

    # TCP 连接测试
    if command -v nc &>/dev/null; then
        if nc -z -w3 "$WSS_HOST" "$WSS_PORT" 2>/dev/null; then
            echo -e "  ${GREEN}✅ TCP $WSS_HOST:$WSS_PORT 可达${NC}"
        else
            echo -e "  ${RED}❌ TCP $WSS_HOST:$WSS_PORT 不可达${NC}"
        fi
    fi

    # TLS 握手测试
    if command -v openssl &>/dev/null; then
        TLS_INFO=$(echo | openssl s_client -connect "$WSS_HOST:$WSS_PORT" -servername "$WSS_HOST" 2>/dev/null | grep -E "Protocol|Cipher" | head -2)
        if [ -n "$TLS_INFO" ]; then
            echo "  TLS: $(echo $TLS_INFO | tr '\n' ' ')"
        fi
    fi
else
    echo "  未配置 WSS 端点"
fi
echo ""

# ─── 5. FEC 状态 ───
echo -e "${CYAN}--- FEC 前向纠错 ---${NC}"
echo "$TUNNEL_RESP" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    fec = d.get('fec', {})
    print(f'  启用:     {fec.get(\"enabled\", False)}')
    print(f'  冗余率:   {fec.get(\"ratio\", 0):.2f}')
    print(f'  已恢复:   {fec.get(\"recovered\", 0)} 包')
    print(f'  数据分片: {fec.get(\"data_shards\", 0)}')
except:
    print('  无法获取 FEC 信息')
" 2>/dev/null
echo ""

# ─── 6. 吞吐量 ───
echo -e "${CYAN}--- 吞吐量 ---${NC}"
echo "$TUNNEL_RESP" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    tp = d.get('throughput', {})
    up = tp.get('up_bytes_per_sec', 0)
    down = tp.get('down_bytes_per_sec', 0)
    def fmt(b):
        if b >= 1073741824: return f'{b/1073741824:.1f} GB/s'
        if b >= 1048576: return f'{b/1048576:.1f} MB/s'
        if b >= 1024: return f'{b/1024:.1f} KB/s'
        return f'{b} B/s'
    print(f'  上行: {fmt(up)}')
    print(f'  下行: {fmt(down)}')
except:
    print('  无法获取吞吐量信息')
" 2>/dev/null
echo ""

# ─── 7. 完整测试（可选）───
if [ "$FULL_TEST" = "1" ]; then
    echo -e "${CYAN}--- 扩展诊断 ---${NC}"

    # MTR 路由追踪
    if [ -n "$QUIC_HOST" ] && command -v mtr &>/dev/null; then
        echo "  路由追踪 ($QUIC_HOST):"
        mtr -r -c 5 "$QUIC_HOST" 2>/dev/null | tail -n +2 | while read -r line; do
            echo "    $line"
        done
    fi
    echo ""

    # 本地 eBPF 状态
    echo "  eBPF 程序:"
    if command -v bpftool &>/dev/null; then
        bpftool prog list 2>/dev/null | grep -A1 "mirage\|npm\|bdna\|jitter\|vpc\|phantom\|chameleon\|sockmap\|h3_shaper" | while read -r line; do
            echo "    $line"
        done
    else
        echo "    (bpftool 未安装)"
    fi
    echo ""

    # 网络接口统计
    echo "  接口统计:"
    for iface in $(ip -o link show up | awk -F': ' '{print $2}' | grep -v "^lo$" | head -3); do
        RX=$(cat /sys/class/net/$iface/statistics/rx_bytes 2>/dev/null || echo 0)
        TX=$(cat /sys/class/net/$iface/statistics/tx_bytes 2>/dev/null || echo 0)
        DROPS=$(cat /sys/class/net/$iface/statistics/rx_dropped 2>/dev/null || echo 0)
        echo "    $iface: RX=$(numfmt --to=iec $RX 2>/dev/null || echo ${RX}B) TX=$(numfmt --to=iec $TX 2>/dev/null || echo ${TX}B) drops=$DROPS"
    done
fi

echo ""
echo "============================================"
echo " 诊断完成"
echo "============================================"
