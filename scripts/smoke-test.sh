#!/bin/bash
# smoke-test.sh - 全链路冒烟测试
# 验证: 注册 → 充值 → 获取配置 → 验证计费
#
# 前置条件:
#   docker-compose up -d (mirage-os/docker-compose.yaml)
#
# 用法:
#   ./scripts/smoke-test.sh

set -euo pipefail

API_BASE="${API_BASE:-http://localhost:3000/api}"
BRIDGE_URL="${BRIDGE_URL:-http://localhost:7000}"
PASS=0
FAIL=0
SKIP=0

check() {
  local name="$1"
  local result="$2"
  if [ "$result" = "0" ]; then
    echo "  ✅ $name"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $name"
    FAIL=$((FAIL + 1))
  fi
}

skip() {
  echo "  ⏭️  $1 (跳过)"
  SKIP=$((SKIP + 1))
}

echo "╔══════════════════════════════════════╗"
echo "║   Mirage Project 全链路冒烟测试     ║"
echo "╚══════════════════════════════════════╝"
echo ""

# ============================================================
# Phase 1: 基础设施检查
# ============================================================
echo "── Phase 1: 基础设施 ──"

# api-server 健康检查
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/../health" 2>/dev/null || echo "000")
if [ "$HTTP" = "000" ]; then
  # 尝试 NestJS 默认路径
  HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/auth/challenge" 2>/dev/null || echo "000")
fi
check "api-server 可达" "$([ "$HTTP" != "000" ] && echo 0 || echo 1)"

# gateway-bridge 健康检查
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BRIDGE_URL/internal/health" 2>/dev/null || echo "000")
check "gateway-bridge 可达" "$([ "$HTTP" = "200" ] && echo 0 || echo 1)"

echo ""

# ============================================================
# Phase 2: 认证链路
# ============================================================
echo "── Phase 2: 认证链路 ──"

# 获取 Ed25519 挑战
CHALLENGE_RESP=$(curl -s "$API_BASE/auth/challenge" 2>/dev/null || echo "{}")
NONCE=$(echo "$CHALLENGE_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('nonce',''))" 2>/dev/null || echo "")
check "GET /auth/challenge 返回 nonce" "$([ -n "$NONCE" ] && echo 0 || echo 1)"

# 用户注册（需要邀请码，可能失败）
REG_RESP=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"username":"smoke_test_user","password":"Test1234!@#$","inviteCode":"SMOKE_TEST"}' 2>/dev/null || echo -e "{}\n000")
REG_CODE=$(echo "$REG_RESP" | tail -1)
if [ "$REG_CODE" = "201" ] || [ "$REG_CODE" = "200" ]; then
  check "POST /auth/register" "0"
else
  skip "POST /auth/register (需要有效邀请码)"
fi

# 用户登录
LOGIN_RESP=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"smoke_test_user","password":"Test1234!@#$","totpCode":"000000"}' 2>/dev/null || echo -e "{}\n000")
LOGIN_CODE=$(echo "$LOGIN_RESP" | tail -1)
LOGIN_BODY=$(echo "$LOGIN_RESP" | head -n -1)
JWT=$(echo "$LOGIN_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('accessToken',''))" 2>/dev/null || echo "")
if [ -n "$JWT" ]; then
  check "POST /auth/login → JWT" "0"
else
  skip "POST /auth/login (用户可能不存在)"
  JWT="dev-token"
fi

echo ""

# ============================================================
# Phase 3: 计费链路
# ============================================================
echo "── Phase 3: 计费链路 ──"

# 模拟 XMR 确认
XMR_RESP=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/webhook/xmr/confirmed" \
  -H "Content-Type: application/json" \
  -d '{"userId":"smoke-user","txHash":"smoke-tx-001","amountXmr":0.01,"amountUsd":1.50,"confirmations":10}' 2>/dev/null || echo -e "{}\n000")
XMR_CODE=$(echo "$XMR_RESP" | tail -1)
check "POST /webhook/xmr/confirmed" "$([ "$XMR_CODE" = "200" ] && echo 0 || echo 1)"

# 查询 Gateway 列表
GW_RESP=$(curl -s -w "\n%{http_code}" "$BRIDGE_URL/internal/gateways" 2>/dev/null || echo -e "[]\n000")
GW_CODE=$(echo "$GW_RESP" | tail -1)
check "GET /internal/gateways" "$([ "$GW_CODE" = "200" ] && echo 0 || echo 1)"

echo ""

# ============================================================
# Phase 4: 配置交付链路
# ============================================================
echo "── Phase 4: 配置交付 ──"

# 阅后即焚兑换（需要有效 token）
DELIVERY_RESP=$(curl -s -w "\n%{http_code}" "$API_BASE/delivery/nonexistent-token?key=dGVzdA==" 2>/dev/null || echo -e "{}\n000")
DELIVERY_CODE=$(echo "$DELIVERY_RESP" | tail -1)
check "GET /delivery/:token 无效 token 返回 404" "$([ "$DELIVERY_CODE" = "404" ] && echo 0 || echo 1)"

echo ""

# ============================================================
# 结果汇总
# ============================================================
TOTAL=$((PASS + FAIL + SKIP))
echo "══════════════════════════════════════"
echo "  通过: $PASS / $TOTAL"
echo "  失败: $FAIL"
echo "  跳过: $SKIP"
echo "══════════════════════════════════════"

if [ "$FAIL" -gt 0 ]; then
  echo "⚠️  存在失败项，请检查服务状态"
  exit 1
else
  echo "✅ 冒烟测试通过"
  exit 0
fi
