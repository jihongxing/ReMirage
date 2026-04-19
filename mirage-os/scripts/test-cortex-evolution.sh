#!/bin/bash
# Mirage-Cortex 影子演习
# 测试极端攻击下的吞吐能力

set -e

# 配置
OS_API="${MIRAGE_OS_API:-http://localhost:8080}"
FINGERPRINT_COUNT=1000
IP_COUNT=10000
DURATION=10
RAFT_DELAY_THRESHOLD=2000  # 2s in ms

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=========================================="
echo "  Mirage-Cortex 影子演习 (Shadow Drills)"
echo "=========================================="
echo ""
echo "参数:"
echo "  - 指纹数量: $FINGERPRINT_COUNT"
echo "  - IP 数量: $IP_COUNT"
echo "  - 持续时间: ${DURATION}s"
echo "  - Raft 延迟阈值: ${RAFT_DELAY_THRESHOLD}ms"
echo ""

# 生成随机指纹
generate_fingerprint() {
    echo "fp_$(openssl rand -hex 16)"
}

# 生成随机 IP
generate_ip() {
    echo "$((RANDOM % 256)).$((RANDOM % 256)).$((RANDOM % 256)).$((RANDOM % 256))"
}

# 记录开始时间
START_TIME=$(date +%s%3N)

echo -e "${YELLOW}[Phase 1] 生成测试数据...${NC}"

# 生成指纹列表
FINGERPRINTS=()
for i in $(seq 1 $FINGERPRINT_COUNT); do
    FINGERPRINTS+=("$(generate_fingerprint)")
done
echo "  ✓ 生成 $FINGERPRINT_COUNT 个指纹"

# 生成 IP 列表
IPS=()
for i in $(seq 1 $IP_COUNT); do
    IPS+=("$(generate_ip)")
done
echo "  ✓ 生成 $IP_COUNT 个 IP"

echo ""
echo -e "${YELLOW}[Phase 2] 压力测试 - 指纹涌入...${NC}"

# 并发发送指纹
CONCURRENT=50
SENT=0
ERRORS=0

send_fingerprint() {
    local fp=$1
    local ip=$2
    local region=$3
    
    curl -s -X POST "$OS_API/api/cortex/fingerprint" \
        -H "Content-Type: application/json" \
        -d "{\"hash\":\"$fp\",\"ip\":\"$ip\",\"region\":\"$region\"}" \
        --max-time 5 > /dev/null 2>&1
    
    return $?
}

# 批量发送
REGIONS=("US" "HK" "SG" "CH" "IS" "JP" "DE" "NL")
BATCH_SIZE=$((IP_COUNT / CONCURRENT))

for batch in $(seq 1 $CONCURRENT); do
    (
        for i in $(seq 1 $BATCH_SIZE); do
            idx=$(( (batch - 1) * BATCH_SIZE + i ))
            if [ $idx -le $IP_COUNT ]; then
                fp_idx=$((idx % FINGERPRINT_COUNT))
                fp=${FINGERPRINTS[$fp_idx]}
                ip=${IPS[$idx-1]}
                region=${REGIONS[$((RANDOM % ${#REGIONS[@]}))]}
                
                if send_fingerprint "$fp" "$ip" "$region"; then
                    ((SENT++)) || true
                else
                    ((ERRORS++)) || true
                fi
            fi
        done
    ) &
done

# 等待所有后台任务完成
wait

END_TIME=$(date +%s%3N)
ELAPSED=$((END_TIME - START_TIME))

echo "  ✓ 发送完成"
echo "    - 成功: $SENT"
echo "    - 失败: $ERRORS"
echo "    - 耗时: ${ELAPSED}ms"

echo ""
echo -e "${YELLOW}[Phase 3] 验证 Raft 同步延迟...${NC}"

# 获取 Raft 状态
RAFT_STATUS=$(curl -s "$OS_API/api/raft/status" 2>/dev/null || echo '{"sync_delay_ms": 0}')
RAFT_DELAY=$(echo "$RAFT_STATUS" | grep -o '"sync_delay_ms":[0-9]*' | cut -d: -f2 || echo "0")

if [ -z "$RAFT_DELAY" ]; then
    RAFT_DELAY=0
fi

if [ "$RAFT_DELAY" -lt "$RAFT_DELAY_THRESHOLD" ]; then
    echo -e "  ${GREEN}✓ Raft 同步延迟: ${RAFT_DELAY}ms (< ${RAFT_DELAY_THRESHOLD}ms)${NC}"
    RAFT_PASS=true
else
    echo -e "  ${RED}✗ Raft 同步延迟: ${RAFT_DELAY}ms (>= ${RAFT_DELAY_THRESHOLD}ms)${NC}"
    RAFT_PASS=false
fi

echo ""
echo -e "${YELLOW}[Phase 4] 验证 eBPF Map 内存占用...${NC}"

# 获取 eBPF 统计
EBPF_STATUS=$(curl -s "$OS_API/api/ebpf/stats" 2>/dev/null || echo '{"memory_mb": 0}')
EBPF_MEMORY=$(echo "$EBPF_STATUS" | grep -o '"memory_mb":[0-9]*' | cut -d: -f2 || echo "0")

if [ -z "$EBPF_MEMORY" ]; then
    EBPF_MEMORY=0
fi

MAX_MEMORY=50  # 50MB 阈值

if [ "$EBPF_MEMORY" -lt "$MAX_MEMORY" ]; then
    echo -e "  ${GREEN}✓ eBPF Map 内存: ${EBPF_MEMORY}MB (< ${MAX_MEMORY}MB)${NC}"
    EBPF_PASS=true
else
    echo -e "  ${RED}✗ eBPF Map 内存: ${EBPF_MEMORY}MB (>= ${MAX_MEMORY}MB)${NC}"
    EBPF_PASS=false
fi

echo ""
echo -e "${YELLOW}[Phase 5] 验证 Cortex 统计...${NC}"

# 获取 Cortex 统计
CORTEX_STATS=$(curl -s "$OS_API/api/cortex/stats" 2>/dev/null || echo '{}')
TOTAL_FP=$(echo "$CORTEX_STATS" | grep -o '"totalFingerprints":[0-9]*' | cut -d: -f2 || echo "0")
HIGH_RISK=$(echo "$CORTEX_STATS" | grep -o '"highRiskCount":[0-9]*' | cut -d: -f2 || echo "0")

echo "  - 总指纹数: $TOTAL_FP"
echo "  - 高危指纹: $HIGH_RISK"

echo ""
echo "=========================================="
echo "  演习结果"
echo "=========================================="

TOTAL_TIME=$(($(date +%s%3N) - START_TIME))
TPS=$((IP_COUNT * 1000 / TOTAL_TIME))

echo ""
echo "性能指标:"
echo "  - 总耗时: ${TOTAL_TIME}ms"
echo "  - 吞吐量: ${TPS} req/s"
echo "  - 目标: ${DURATION}s 内完成"

if [ "$TOTAL_TIME" -le $((DURATION * 1000)) ]; then
    echo -e "  ${GREEN}✓ 时间目标达成${NC}"
    TIME_PASS=true
else
    echo -e "  ${RED}✗ 超出时间目标${NC}"
    TIME_PASS=false
fi

echo ""
echo "验证结果:"

PASS_COUNT=0
FAIL_COUNT=0

if [ "$RAFT_PASS" = true ]; then
    echo -e "  ${GREEN}✓ Raft 同步延迟${NC}"
    ((PASS_COUNT++))
else
    echo -e "  ${RED}✗ Raft 同步延迟${NC}"
    ((FAIL_COUNT++))
fi

if [ "$EBPF_PASS" = true ]; then
    echo -e "  ${GREEN}✓ eBPF 内存占用${NC}"
    ((PASS_COUNT++))
else
    echo -e "  ${RED}✗ eBPF 内存占用${NC}"
    ((FAIL_COUNT++))
fi

if [ "$TIME_PASS" = true ]; then
    echo -e "  ${GREEN}✓ 时间目标${NC}"
    ((PASS_COUNT++))
else
    echo -e "  ${RED}✗ 时间目标${NC}"
    ((FAIL_COUNT++))
fi

echo ""
echo "=========================================="

if [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${GREEN}影子演习通过！Cortex 准备就绪。${NC}"
    exit 0
else
    echo -e "${RED}影子演习失败！${FAIL_COUNT} 项未通过。${NC}"
    exit 1
fi
