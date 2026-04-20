#!/bin/bash
# gateway-healthcheck.sh - Gateway 健康巡检脚本
# 用途：定时巡检 Gateway 各模块状态，异常时输出告警
# 用法：bash gateway-healthcheck.sh [gateway_addr] [--json] [--alert-webhook URL]
# 建议 cron: */5 * * * * /opt/mirage/scripts/gateway-healthcheck.sh >> /var/log/mirage-healthcheck.log 2>&1

set -e

GATEWAY_ADDR=${1:-"127.0.0.1:9090"}
OUTPUT_JSON=0
ALERT_WEBHOOK=""
ALERT_THRESHOLD=2  # 连续失败次数触发告警

STATE_FILE="/tmp/mirage-healthcheck-state"

# 解析参数
shift 2>/dev/null || true
while [ $# -gt 0 ]; do
    case "$1" in
        --json) OUTPUT_JSON=1 ;;
        --alert-webhook) ALERT_WEBHOOK="$2"; shift ;;
        --threshold) ALERT_THRESHOLD="$2"; shift ;;
    esac
    shift
done

TIMESTAMP=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
CHECKS=()
OVERALL="healthy"

# 执行检查
check() {
    local name=$1
    local url=$2
    local expect_field=$3
    local expect_value=$4

    local result
    local http_code
    local body

    result=$(curl -s -o /tmp/mirage-hc-body -w "%{http_code}" --connect-timeout 3 --max-time 5 "http://${GATEWAY_ADDR}${url}" 2>/dev/null) || result="000"
    body=$(cat /tmp/mirage-hc-body 2>/dev/null || echo "")

    local status="ok"
    local detail=""

    if [ "$result" = "000" ]; then
        status="unreachable"
        detail="连接超时"
        OVERALL="critical"
    elif [ "$result" != "200" ]; then
        status="error"
        detail="HTTP $result"
        OVERALL="degraded"
    elif [ -n "$expect_field" ] && [ -n "$body" ]; then
        # 简单 JSON 字段检查
        local actual
        actual=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('$expect_field',''))" 2>/dev/null || echo "")
        if [ -n "$expect_value" ] && [ "$actual" != "$expect_value" ]; then
            status="warning"
            detail="${expect_field}=${actual} (期望: ${expect_value})"
            if [ "$OVERALL" = "healthy" ]; then OVERALL="degraded"; fi
        else
            detail="${expect_field}=${actual}"
        fi
    fi

    CHECKS+=("{\"name\":\"$name\",\"status\":\"$status\",\"detail\":\"$detail\"}")

    if [ "$OUTPUT_JSON" = "0" ]; then
        local icon="✅"
        case "$status" in
            warning) icon="⚠️ " ;;
            error) icon="❌" ;;
            unreachable) icon="⚫" ;;
        esac
        echo "  $icon $name: $detail"
    fi
}

if [ "$OUTPUT_JSON" = "0" ]; then
    echo "[$TIMESTAMP] Mirage Gateway 健康巡检 ($GATEWAY_ADDR)"
fi

# ─── 执行各项检查 ───
check "health" "/health" "status" "online"
check "ebpf" "/health" "ebpf_loaded" "True"
check "grpc_uplink" "/health" "grpc_uplink" "connected"
check "grpc_downlink" "/health" "grpc_downlink" "running"
check "tunnel" "/api/tunnel/status" "active" ""
check "threat_level" "/api/threat/summary" "current_level" ""
check "quota" "/api/quota" "throttled" "False"

# ─── 系统资源检查 ───
if [ "$OUTPUT_JSON" = "0" ]; then
    echo ""
    echo "  系统资源:"
fi

# CPU
CPU_USAGE=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' 2>/dev/null || echo "0")
if [ "$OUTPUT_JSON" = "0" ]; then
    echo "    CPU: ${CPU_USAGE}%"
fi

# 内存
MEM_INFO=$(free -m 2>/dev/null | awk '/Mem:/ {printf "%d/%d MB (%.0f%%)", $3, $2, $3/$2*100}')
if [ "$OUTPUT_JSON" = "0" ]; then
    echo "    内存: $MEM_INFO"
fi

# Gateway 进程
GW_PID=$(pgrep -f "mirage-gateway" 2>/dev/null | head -1)
if [ -n "$GW_PID" ]; then
    GW_MEM=$(ps -o rss= -p "$GW_PID" 2>/dev/null | awk '{printf "%.0f", $1/1024}')
    GW_CPU=$(ps -o %cpu= -p "$GW_PID" 2>/dev/null | tr -d ' ')
    if [ "$OUTPUT_JSON" = "0" ]; then
        echo "    Gateway 进程: PID=$GW_PID MEM=${GW_MEM}MB CPU=${GW_CPU}%"
    fi
    CHECKS+=("{\"name\":\"process\",\"status\":\"ok\",\"detail\":\"PID=$GW_PID MEM=${GW_MEM}MB\"}")
else
    if [ "$OUTPUT_JSON" = "0" ]; then
        echo "    ❌ Gateway 进程未找到"
    fi
    CHECKS+=("{\"name\":\"process\",\"status\":\"error\",\"detail\":\"进程不存在\"}")
    OVERALL="critical"
fi

# ─── 输出结果 ───
if [ "$OUTPUT_JSON" = "1" ]; then
    CHECKS_JSON=$(printf '%s,' "${CHECKS[@]}" | sed 's/,$//')
    echo "{\"timestamp\":\"$TIMESTAMP\",\"gateway\":\"$GATEWAY_ADDR\",\"overall\":\"$OVERALL\",\"checks\":[$CHECKS_JSON]}"
else
    echo ""
    case "$OVERALL" in
        healthy)  echo "  🟢 总体状态: 健康" ;;
        degraded) echo "  🟡 总体状态: 降级" ;;
        critical) echo "  🔴 总体状态: 异常" ;;
    esac
fi

# ─── 连续失败告警 ───
if [ "$OVERALL" != "healthy" ]; then
    FAIL_COUNT=1
    if [ -f "$STATE_FILE" ]; then
        PREV_COUNT=$(cat "$STATE_FILE" 2>/dev/null || echo "0")
        FAIL_COUNT=$((PREV_COUNT + 1))
    fi
    echo "$FAIL_COUNT" > "$STATE_FILE"

    if [ "$FAIL_COUNT" -ge "$ALERT_THRESHOLD" ] && [ -n "$ALERT_WEBHOOK" ]; then
        ALERT_MSG="⚠️ Mirage Gateway 异常 (${GATEWAY_ADDR}): ${OVERALL}, 连续 ${FAIL_COUNT} 次失败"
        curl -s -X POST "$ALERT_WEBHOOK" \
            -H "Content-Type: application/json" \
            -d "{\"text\":\"$ALERT_MSG\",\"timestamp\":\"$TIMESTAMP\"}" \
            >/dev/null 2>&1 || true
        if [ "$OUTPUT_JSON" = "0" ]; then
            echo "  📢 告警已发送 (连续 ${FAIL_COUNT} 次)"
        fi
    fi
else
    # 恢复正常，清除状态
    rm -f "$STATE_FILE"
fi

# 退出码
case "$OVERALL" in
    healthy)  exit 0 ;;
    degraded) exit 1 ;;
    critical) exit 2 ;;
esac
