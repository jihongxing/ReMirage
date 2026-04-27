#!/bin/bash
# udp-qualify.sh - UDP 网络资质检测脚本
# 用途：部署前验证服务器的 UDP 收发能力和性能是否达标
# 用法：sudo bash udp-qualify.sh [测试端口] [测试时长秒]

set -e

PORT=${1:-8443}
DURATION=${2:-10}
PASS=0
FAIL=0
WARN=0

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; ((++PASS)); }
fail() { echo -e "${RED}[FAIL]${NC} $1"; ((++FAIL)); }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; ((++WARN)); }

echo "============================================"
echo " Mirage UDP 网络资质检测"
echo " 测试端口: $PORT | 时长: ${DURATION}s"
echo "============================================"
echo ""

# 1. 内核版本检查
echo "--- 基础环境 ---"
KVER=$(uname -r)
KMAJOR=$(echo $KVER | cut -d. -f1)
KMINOR=$(echo $KVER | cut -d. -f2)
if [ "$KMAJOR" -gt 5 ] || ([ "$KMAJOR" -eq 5 ] && [ "$KMINOR" -ge 15 ]); then
    pass "内核版本: $KVER (>= 5.15)"
elif [ "$KMAJOR" -eq 4 ] && [ "$KMINOR" -ge 19 ]; then
    warn "内核版本: $KVER (>= 4.19, 建议升级到 5.15+)"
else
    fail "内核版本: $KVER (< 4.19, 不支持)"
fi

# 2. UDP socket 创建能力
echo ""
echo "--- UDP Socket 能力 ---"
if python3 -c "
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(('0.0.0.0', $PORT))
s.close()
sys.exit(0)
" 2>/dev/null; then
    pass "UDP $PORT 端口可绑定"
else
    fail "UDP $PORT 端口绑定失败（被占用或权限不足）"
fi

# 3. UDP 收发回环测试
echo ""
echo "--- UDP 回环测试 ---"
RECV_COUNT=0

# 启动 UDP 接收端
python3 -c "
import socket, sys, time, threading

s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(('127.0.0.1', $PORT))
s.settimeout(3)

count = 0
try:
    while True:
        data, addr = s.recvfrom(2048)
        count += 1
        if count >= 100:
            break
except:
    pass
s.close()
print(count)
" &
RECV_PID=$!

sleep 0.5

# 发送 100 个 UDP 包
python3 -c "
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
for i in range(100):
    s.sendto(b'MIRAGE_TEST_' + str(i).encode(), ('127.0.0.1', $PORT))
s.close()
"

wait $RECV_PID 2>/dev/null
RECV_COUNT=$(python3 -c "
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(('127.0.0.1', $(($PORT+1))))
s.settimeout(1)
# 已经在上面的进程中完成
print(100)
" 2>/dev/null || echo "0")

# 重新做一次干净的测试
RESULT=$(python3 -c "
import socket, threading, time

received = []
def server():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.bind(('127.0.0.1', $PORT + 10))
    s.settimeout(3)
    try:
        while len(received) < 100:
            data, _ = s.recvfrom(2048)
            received.append(data)
    except:
        pass
    s.close()

t = threading.Thread(target=server)
t.start()
time.sleep(0.2)

s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
for i in range(100):
    s.sendto(b'TEST' + str(i).encode(), ('127.0.0.1', $PORT + 10))
s.close()

t.join(timeout=5)
print(len(received))
" 2>/dev/null)

if [ "$RESULT" -ge 99 ]; then
    pass "回环收发: $RESULT/100 包 (丢包率 $((100-RESULT))%)"
elif [ "$RESULT" -ge 90 ]; then
    warn "回环收发: $RESULT/100 包 (丢包率 $((100-RESULT))%)"
else
    fail "回环收发: $RESULT/100 包 (丢包率 $((100-RESULT))%)"
fi

# 4. UDP 吞吐量测试
echo ""
echo "--- UDP 吞吐量测试 (${DURATION}s) ---"

THROUGHPUT=$(python3 -c "
import socket, threading, time

DURATION = $DURATION
PKT_SIZE = 1200  # QUIC MTU
total_bytes = 0
total_pkts = 0
running = True

def receiver():
    global total_bytes, total_pkts, running
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.bind(('127.0.0.1', $PORT + 20))
    s.settimeout(1)
    while running:
        try:
            data, _ = s.recvfrom(2048)
            total_bytes += len(data)
            total_pkts += 1
        except:
            pass
    s.close()

t = threading.Thread(target=receiver)
t.start()
time.sleep(0.2)

s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
payload = b'X' * PKT_SIZE
start = time.time()
sent = 0
while time.time() - start < DURATION:
    try:
        s.sendto(payload, ('127.0.0.1', $PORT + 20))
        sent += 1
    except:
        pass
s.close()
elapsed = time.time() - start
running = False
t.join(timeout=3)

mbps = (total_bytes * 8) / (elapsed * 1000000)
pps = total_pkts / elapsed
print(f'{mbps:.1f} {pps:.0f} {total_pkts} {sent}')
" 2>/dev/null)

if [ -n "$THROUGHPUT" ]; then
    MBPS=$(echo $THROUGHPUT | awk '{print $1}')
    PPS=$(echo $THROUGHPUT | awk '{print $2}')
    RECV_PKTS=$(echo $THROUGHPUT | awk '{print $3}')
    SENT_PKTS=$(echo $THROUGHPUT | awk '{print $4}')
    LOSS_PCT=$(python3 -c "print(f'{(1-$RECV_PKTS/$SENT_PKTS)*100:.1f}')" 2>/dev/null || echo "?")

    if (( $(echo "$MBPS > 100" | bc -l 2>/dev/null || echo 0) )); then
        pass "吞吐量: ${MBPS} Mbps (${PPS} pps)"
    elif (( $(echo "$MBPS > 10" | bc -l 2>/dev/null || echo 0) )); then
        warn "吞吐量: ${MBPS} Mbps (${PPS} pps) — 建议 > 100 Mbps"
    else
        fail "吞吐量: ${MBPS} Mbps (${PPS} pps) — 严重不足"
    fi
    echo "     发送: ${SENT_PKTS} 包 | 接收: ${RECV_PKTS} 包 | 丢包: ${LOSS_PCT}%"
else
    fail "吞吐量测试执行失败"
fi

# 5. UDP 延迟测试
echo ""
echo "--- UDP 延迟测试 ---"

LATENCY=$(python3 -c "
import socket, time, statistics

s_recv = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s_recv.bind(('127.0.0.1', $PORT + 30))
s_recv.settimeout(1)

s_send = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

latencies = []
for i in range(100):
    ts = time.perf_counter_ns()
    s_send.sendto(ts.to_bytes(8, 'big'), ('127.0.0.1', $PORT + 30))
    try:
        data, _ = s_recv.recvfrom(64)
        rtt = (time.perf_counter_ns() - int.from_bytes(data, 'big')) / 1000  # 微秒
        latencies.append(rtt)
    except:
        pass

s_recv.close()
s_send.close()

if latencies:
    avg = statistics.mean(latencies)
    p99 = sorted(latencies)[int(len(latencies)*0.99)]
    print(f'{avg:.1f} {p99:.1f}')
else:
    print('0 0')
" 2>/dev/null)

if [ -n "$LATENCY" ]; then
    AVG_US=$(echo $LATENCY | awk '{print $1}')
    P99_US=$(echo $LATENCY | awk '{print $2}')

    if (( $(echo "$AVG_US < 100" | bc -l 2>/dev/null || echo 1) )); then
        pass "延迟: avg=${AVG_US}μs p99=${P99_US}μs"
    elif (( $(echo "$AVG_US < 1000" | bc -l 2>/dev/null || echo 1) )); then
        warn "延迟: avg=${AVG_US}μs p99=${P99_US}μs — 偏高"
    else
        fail "延迟: avg=${AVG_US}μs p99=${P99_US}μs — 严重偏高"
    fi
else
    fail "延迟测试执行失败"
fi

# 6. 防火墙/安全组检查
echo ""
echo "--- 防火墙检查 ---"

# iptables INPUT 规则
if command -v iptables &>/dev/null; then
    UDP_RULES=$(iptables -L INPUT -n 2>/dev/null | grep -c "udp" || echo "0")
    if [ "$UDP_RULES" -gt 0 ]; then
        pass "iptables 存在 UDP 规则 ($UDP_RULES 条)"
    else
        warn "iptables 无显式 UDP 放行规则（依赖默认策略）"
    fi
fi

# 检查 conntrack
if [ -f /proc/sys/net/netfilter/nf_conntrack_udp_timeout ]; then
    UDP_TIMEOUT=$(cat /proc/sys/net/netfilter/nf_conntrack_udp_timeout)
    if [ "$UDP_TIMEOUT" -ge 30 ]; then
        pass "conntrack UDP 超时: ${UDP_TIMEOUT}s"
    else
        warn "conntrack UDP 超时: ${UDP_TIMEOUT}s (建议 >= 30s)"
    fi
fi

# 7. 系统 UDP 缓冲区
echo ""
echo "--- 系统 UDP 缓冲区 ---"

RMEM_MAX=$(cat /proc/sys/net/core/rmem_max 2>/dev/null || echo "0")
WMEM_MAX=$(cat /proc/sys/net/core/wmem_max 2>/dev/null || echo "0")

if [ "$RMEM_MAX" -ge 16777216 ]; then
    pass "rmem_max: $(($RMEM_MAX/1048576))MB"
elif [ "$RMEM_MAX" -ge 1048576 ]; then
    warn "rmem_max: $(($RMEM_MAX/1048576))MB (建议 >= 16MB)"
else
    fail "rmem_max: $(($RMEM_MAX/1024))KB (严重不足，建议 >= 16MB)"
fi

if [ "$WMEM_MAX" -ge 16777216 ]; then
    pass "wmem_max: $(($WMEM_MAX/1048576))MB"
elif [ "$WMEM_MAX" -ge 1048576 ]; then
    warn "wmem_max: $(($WMEM_MAX/1048576))MB (建议 >= 16MB)"
else
    fail "wmem_max: $(($WMEM_MAX/1024))KB (严重不足，建议 >= 16MB)"
fi

# 汇总
echo ""
echo "============================================"
echo " 检测结果汇总"
echo "============================================"
echo -e " ${GREEN}通过: $PASS${NC} | ${YELLOW}警告: $WARN${NC} | ${RED}失败: $FAIL${NC}"
echo ""

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}✅ 服务器 UDP 资质合格，可部署 Mirage-Gateway${NC}"
    exit 0
elif [ "$FAIL" -le 2 ] && [ "$WARN" -le 3 ]; then
    echo -e "${YELLOW}⚠️  服务器存在问题，建议修复后再部署${NC}"
    exit 1
else
    echo -e "${RED}❌ 服务器 UDP 资质不合格，无法部署${NC}"
    exit 2
fi
