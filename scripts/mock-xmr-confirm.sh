#!/bin/bash
# mock-xmr-confirm.sh - 模拟 XMR 充值确认回调
# 用于本地冒烟测试和 CI/CD E2E 测试
#
# 用法:
#   ./scripts/mock-xmr-confirm.sh [user_id] [amount_xmr]
#
# 示例:
#   ./scripts/mock-xmr-confirm.sh user-001 0.05

set -euo pipefail

API_BASE="${API_BASE:-http://localhost:3000/api}"
USER_ID="${1:-test-user-001}"
AMOUNT_XMR="${2:-0.05}"
TX_HASH="mock-$(date +%s)-$(head -c 8 /dev/urandom | xxd -p)"
AMOUNT_USD=$(echo "$AMOUNT_XMR * 150" | bc -l 2>/dev/null || echo "7.50")

echo "=== Mirage XMR Mock Confirm ==="
echo "  API:     $API_BASE"
echo "  User:    $USER_ID"
echo "  Amount:  $AMOUNT_XMR XMR (~\$$AMOUNT_USD)"
echo "  TxHash:  $TX_HASH"
echo ""

# 1. 模拟 XMR 确认回调
echo "[1/3] 发送 XMR 确认回调..."
CONFIRM_RESP=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/webhook/xmr/confirmed" \
  -H "Content-Type: application/json" \
  -d "{
    \"userId\": \"$USER_ID\",
    \"txHash\": \"$TX_HASH\",
    \"amountXmr\": $AMOUNT_XMR,
    \"amountUsd\": $AMOUNT_USD,
    \"confirmations\": 10
  }")

HTTP_CODE=$(echo "$CONFIRM_RESP" | tail -1)
BODY=$(echo "$CONFIRM_RESP" | head -n -1)

if [ "$HTTP_CODE" = "200" ]; then
  echo "  ✅ 确认成功: $BODY"
else
  echo "  ❌ 确认失败 (HTTP $HTTP_CODE): $BODY"
  exit 1
fi

# 2. 验证用户余额
echo ""
echo "[2/3] 验证用户余额..."
sleep 1

# 通过 gateway-bridge REST API 查询（如果可用）
BRIDGE_URL="${BRIDGE_URL:-http://localhost:7000}"
QUOTA_RESP=$(curl -s -w "\n%{http_code}" "$BRIDGE_URL/internal/quota/$USER_ID" 2>/dev/null || echo -e "{}\n000")
QUOTA_CODE=$(echo "$QUOTA_RESP" | tail -1)
QUOTA_BODY=$(echo "$QUOTA_RESP" | head -n -1)

if [ "$QUOTA_CODE" = "200" ]; then
  echo "  ✅ 配额查询: $QUOTA_BODY"
else
  echo "  ⚠️  配额查询不可用 (HTTP $QUOTA_CODE)，跳过"
fi

# 3. 验证 Gateway 列表
echo ""
echo "[3/3] 验证 Gateway 状态..."
GW_RESP=$(curl -s -w "\n%{http_code}" "$BRIDGE_URL/internal/gateways" 2>/dev/null || echo -e "[]\\n000")
GW_CODE=$(echo "$GW_RESP" | tail -1)
GW_BODY=$(echo "$GW_RESP" | head -n -1)

if [ "$GW_CODE" = "200" ]; then
  echo "  ✅ Gateway 列表: $GW_BODY"
else
  echo "  ⚠️  Gateway 查询不可用 (HTTP $GW_CODE)，跳过"
fi

echo ""
echo "=== Mock 完成 ==="
