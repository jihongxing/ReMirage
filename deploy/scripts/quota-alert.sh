#!/bin/bash
# quota-alert.sh - 配额告警脚本
# 用途：监控 Gateway 配额使用情况，低于阈值时发送告警
# 用法：bash quota-alert.sh [gateway_addr] [--threshold 80] [--webhook URL]
# 建议 cron: */10 * * * * /opt/mirage/scripts/quota-alert.sh

set -e

GATEWAY_ADDR=${1:-"127.0.0.1:9090"}
THRESHOLD=80        # 使用率告警阈值 (%)
CRITICAL=95         # 使用率严重告警阈值 (%)
WEBHOOK=""
TELEGRAM_BOT=""
TELEGRAM_CHAT=""
STATE_FILE="/tmp/mirage-quota-alert-state"

# 解析参数
shift 2>/dev/null || true
while [ $# -gt 0 ]; do
    case "$1" in
        --threshold) THRESHOLD="$2"; shift ;;
        --critical) CRITICAL="$2"; shift ;;
        --webhook) WEBHOOK="$2"; shift ;;
        --telegram-bot) TELEGRAM_BOT="$2"; shift ;;
        --telegram-chat) TELEGRAM_CHAT="$2"; shift ;;
    esac
    shift
done

# 获取配额信息
RESPONSE=$(curl -s --connect-timeout 3 --max-time 5 "http://${GATEWAY_ADDR}/api/quota" 2>/dev/null)
if [ -z "$RESPONSE" ]; then
    echo "[$(date -u '+%H:%M:%S')] ❌ 无法连接 Gateway ($GATEWAY_ADDR)"
    exit 1
fi

# 解析 JSON
USAGE_PCT=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('usage_percent', 0))" 2>/dev/null || echo "0")
REMAINING=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('remaining_bytes', 0))" 2>/dev/null || echo "0")
TOTAL=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('total_bytes', 0))" 2>/dev/null || echo "0")
THROTTLED=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('throttled', False))" 2>/dev/null || echo "False")
DEFENSE_OVERHEAD=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(round(d.get('defense_overhead', 0)*100, 1))" 2>/dev/null || echo "0")

# 格式化字节
format_bytes() {
    local bytes=$1
    if [ "$bytes" -ge 1073741824 ]; then
        echo "$(echo "scale=1; $bytes/1073741824" | bc) GB"
    elif [ "$bytes" -ge 1048576 ]; then
        echo "$(echo "scale=1; $bytes/1048576" | bc) MB"
    else
        echo "${bytes} B"
    fi
}

REMAINING_FMT=$(format_bytes "$REMAINING")
TOTAL_FMT=$(format_bytes "$TOTAL")
USAGE_INT=$(printf "%.0f" "$USAGE_PCT")

# 判断告警级别
ALERT_LEVEL="none"
if [ "$THROTTLED" = "True" ]; then
    ALERT_LEVEL="critical"
elif [ "$USAGE_INT" -ge "$CRITICAL" ]; then
    ALERT_LEVEL="critical"
elif [ "$USAGE_INT" -ge "$THRESHOLD" ]; then
    ALERT_LEVEL="warning"
fi

# 防止重复告警（同级别 1 小时内不重复）
LAST_ALERT=""
if [ -f "$STATE_FILE" ]; then
    LAST_ALERT=$(cat "$STATE_FILE" 2>/dev/null)
fi

CURRENT_HOUR=$(date +%Y%m%d%H)
ALERT_KEY="${ALERT_LEVEL}_${CURRENT_HOUR}"

should_alert() {
    if [ "$ALERT_LEVEL" = "none" ]; then
        return 1
    fi
    if [ "$LAST_ALERT" = "$ALERT_KEY" ]; then
        return 1  # 同小时已告警
    fi
    return 0
}

# 发送告警
send_alert() {
    local msg=$1
    local level=$2

    # Webhook (Slack/Discord/自定义)
    if [ -n "$WEBHOOK" ]; then
        local color="warning"
        [ "$level" = "critical" ] && color="danger"
        curl -s -X POST "$WEBHOOK" \
            -H "Content-Type: application/json" \
            -d "{\"text\":\"$msg\",\"level\":\"$level\"}" \
            >/dev/null 2>&1 || true
    fi

    # Telegram
    if [ -n "$TELEGRAM_BOT" ] && [ -n "$TELEGRAM_CHAT" ]; then
        local encoded_msg
        encoded_msg=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$msg'))" 2>/dev/null || echo "$msg")
        curl -s "https://api.telegram.org/bot${TELEGRAM_BOT}/sendMessage?chat_id=${TELEGRAM_CHAT}&text=${encoded_msg}" \
            >/dev/null 2>&1 || true
    fi

    echo "$ALERT_KEY" > "$STATE_FILE"
}

# 输出状态
TIMESTAMP=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
case "$ALERT_LEVEL" in
    critical)
        echo "[$TIMESTAMP] 🔴 配额严重不足: ${USAGE_PCT}% 已用 (剩余 $REMAINING_FMT / $TOTAL_FMT)"
        [ "$THROTTLED" = "True" ] && echo "  ⚠️  流量已被限速"
        echo "  防御开销: ${DEFENSE_OVERHEAD}%"
        if should_alert; then
            MSG="🔴 Mirage 配额告警 [${GATEWAY_ADDR}]: ${USAGE_PCT}% 已用，剩余 ${REMAINING_FMT}。"
            [ "$THROTTLED" = "True" ] && MSG="${MSG} 流量已限速！"
            send_alert "$MSG" "critical"
            echo "  📢 告警已发送"
        fi
        ;;
    warning)
        echo "[$TIMESTAMP] 🟡 配额预警: ${USAGE_PCT}% 已用 (剩余 $REMAINING_FMT / $TOTAL_FMT)"
        echo "  防御开销: ${DEFENSE_OVERHEAD}%"
        if should_alert; then
            MSG="🟡 Mirage 配额预警 [${GATEWAY_ADDR}]: ${USAGE_PCT}% 已用，剩余 ${REMAINING_FMT}。建议充值。"
            send_alert "$MSG" "warning"
            echo "  📢 告警已发送"
        fi
        ;;
    none)
        echo "[$TIMESTAMP] 🟢 配额正常: ${USAGE_PCT}% 已用 (剩余 $REMAINING_FMT / $TOTAL_FMT)"
        # 恢复正常时清除状态
        rm -f "$STATE_FILE"
        ;;
esac

exit 0
