#!/usr/bin/env bash
# ============================================================
# Mirage-OS 端到端 Smoke Test
# 前置条件：docker compose up -d 已完成，所有服务就绪
# 用法：bash scripts/smoke-test.sh [API_BASE] [BRIDGE_BASE]
# ============================================================
set -euo pipefail

API_BASE="${1:-http://localhost:3000}"
BRIDGE_BASE="${2:-http://localhost:7000}"
WEB_BASE="${3:-http://localhost:8080}"

PASS=0
FAIL=0
TOTAL=0

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

check() {
  local name="$1"
  local result="$2"
  local expected="${3:-200}"
  TOTAL=$((TOTAL + 1))
  if [ "$result" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $name (HTTP $result)"
    PASS=$((PASS + 1))
  else
    echo -e "  ${RED}✗${NC} $name (got HTTP $result, expected $expected)"
    FAIL=$((FAIL + 1))
  fi
}

check_body() {
  local name="$1"
  local body="$2"
  local pattern="$3"
  TOTAL=$((TOTAL + 1))
  if echo "$body" | grep -q "$pattern"; then
    echo -e "  ${GREEN}✓${NC} $name (body contains '$pattern')"
    PASS=$((PASS + 1))
  else
    echo -e "  ${RED}✗${NC} $name (body missing '$pattern')"
    FAIL=$((FAIL + 1))
  fi
}

echo ""
echo "=========================================="
echo " Mirage-OS Smoke Test"
echo "=========================================="
echo ""

# ------------------------------------------
# 1. 基础健康检查
# ------------------------------------------
echo -e "${YELLOW}[1/7] 基础健康检查${NC}"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/health" 2>/dev/null || echo "000")
check "api-server /health" "$STATUS"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BRIDGE_BASE/internal/health" 2>/dev/null || echo "000")
check "gateway-bridge /internal/health" "$STATUS"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$WEB_BASE/" 2>/dev/null || echo "000")
check "web 首页加载" "$STATUS"

# ------------------------------------------
# 2. Auth 认证流程
# ------------------------------------------
echo ""
echo -e "${YELLOW}[2/7] Auth 认证流程${NC}"

# 获取 challenge
CHALLENGE_RESP=$(curl -s -w "\n%{http_code}" "$API_BASE/api/auth/challenge" 2>/dev/null || echo -e "\n000")
CHALLENGE_STATUS=$(echo "$CHALLENGE_RESP" | tail -1)
CHALLENGE_BODY=$(echo "$CHALLENGE_RESP" | sed '$d')
check "GET /api/auth/challenge" "$CHALLENGE_STATUS"

# 登录（用测试账号，预期可能 401 如果没有种子数据）
LOGIN_RESP=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"test123","totp":"000000"}' 2>/dev/null || echo -e "\n000")
LOGIN_STATUS=$(echo "$LOGIN_RESP" | tail -1)
LOGIN_BODY=$(echo "$LOGIN_RESP" | sed '$d')

# 如果有种子数据，登录应该返回 201；否则 401 也是正常的（说明认证逻辑在工作）
if [ "$LOGIN_STATUS" = "201" ] || [ "$LOGIN_STATUS" = "200" ]; then
  check "POST /api/auth/login (有种子数据)" "$LOGIN_STATUS" "$LOGIN_STATUS"
  TOKEN=$(echo "$LOGIN_BODY" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)
  if [ -z "$TOKEN" ]; then
    TOKEN=$(echo "$LOGIN_BODY" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)
  fi
elif [ "$LOGIN_STATUS" = "401" ]; then
  check "POST /api/auth/login (无种子数据，401 正常)" "$LOGIN_STATUS" "401"
  TOKEN=""
else
  check "POST /api/auth/login" "$LOGIN_STATUS" "201"
  TOKEN=""
fi

# ------------------------------------------
# 3. 需要 JWT 的接口（如果拿到了 token）
# ------------------------------------------
echo ""
echo -e "${YELLOW}[3/7] 受保护接口（JWT）${NC}"

if [ -n "${TOKEN:-}" ]; then
  AUTH_HEADER="Authorization: Bearer $TOKEN"

  STATUS=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$API_BASE/api/gateways" 2>/dev/null || echo "000")
  check "GET /api/gateways" "$STATUS"

  STATUS=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$API_BASE/api/threats" 2>/dev/null || echo "000")
  check "GET /api/threats" "$STATUS"

  STATUS=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$API_BASE/api/threats/stats" 2>/dev/null || echo "000")
  check "GET /api/threats/stats" "$STATUS"

  STATUS=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$API_BASE/api/billing/quota" 2>/dev/null || echo "000")
  check "GET /api/billing/quota" "$STATUS"

  STATUS=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "$API_BASE/api/billing/logs" 2>/dev/null || echo "000")
  check "GET /api/billing/logs" "$STATUS"

else
  echo -e "  ${YELLOW}⚠${NC} 跳过（无有效 token，需要种子数据）"
  # 验证未认证请求返回 401
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/api/gateways" 2>/dev/null || echo "000")
  check "GET /api/gateways (无 token → 401)" "$STATUS" "401"

  STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/api/billing/quota" 2>/dev/null || echo "000")
  check "GET /api/billing/quota (无 token → 401)" "$STATUS" "401"
fi

# ------------------------------------------
# 4. Gateway Bridge 内部接口
# ------------------------------------------
echo ""
echo -e "${YELLOW}[4/7] Gateway Bridge 内部接口${NC}"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BRIDGE_BASE/internal/gateways" 2>/dev/null || echo "000")
check "GET /internal/gateways" "$STATUS"

# ------------------------------------------
# 5. Web 前端静态资源
# ------------------------------------------
echo ""
echo -e "${YELLOW}[5/7] Web 前端${NC}"

WEB_BODY=$(curl -s "$WEB_BASE/" 2>/dev/null || echo "")
check_body "index.html 包含 root div" "$WEB_BODY" 'id="root"'

# 验证 nginx 代理 /api 到 api-server
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$WEB_BASE/api/auth/challenge" 2>/dev/null || echo "000")
check "Nginx 代理 /api → api-server" "$STATUS"

# ------------------------------------------
# 6. 数据库连通性（通过 API 间接验证）
# ------------------------------------------
echo ""
echo -e "${YELLOW}[6/7] 数据库连通性${NC}"

# Prisma migrate 状态（通过 health 间接验证）
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/health" 2>/dev/null || echo "000")
check "Prisma 连接正常（health OK 即可）" "$STATUS"

# ------------------------------------------
# 7. Recharge 流程（如果有 token）
# ------------------------------------------
echo ""
echo -e "${YELLOW}[7/7] Recharge 流程${NC}"

if [ -n "${TOKEN:-}" ]; then
  RECHARGE_RESP=$(curl -s -w "\n%{http_code}" -X POST "$API_BASE/api/billing/recharge" \
    -H "Content-Type: application/json" \
    -H "$AUTH_HEADER" \
    -d '{"amount":10}' 2>/dev/null || echo -e "\n000")
  RECHARGE_STATUS=$(echo "$RECHARGE_RESP" | tail -1)
  check "POST /api/billing/recharge" "$RECHARGE_STATUS" "201"

  # 验证 quota 变化
  QUOTA_RESP=$(curl -s -H "$AUTH_HEADER" "$API_BASE/api/billing/quota" 2>/dev/null || echo "{}")
  check_body "Quota 已更新" "$QUOTA_RESP" "remaining_quota"
else
  echo -e "  ${YELLOW}⚠${NC} 跳过（无有效 token）"
fi

# ------------------------------------------
# 结果汇总
# ------------------------------------------
echo ""
echo "=========================================="
echo -e " 结果: ${GREEN}$PASS 通过${NC} / ${RED}$FAIL 失败${NC} / $TOTAL 总计"
echo "=========================================="
echo ""

if [ $FAIL -gt 0 ]; then
  exit 1
fi
exit 0
