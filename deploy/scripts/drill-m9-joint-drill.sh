#!/bin/bash
# drill-m9-joint-drill.sh - M9 联合演练执行脚本
# 用途：执行准入控制联合演练（场景 1 非法接入 + 场景 2 配额耗尽）
# 输出：deploy/evidence/m9-joint-drill.log

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_FILE="$PROJECT_ROOT/deploy/evidence/m9-joint-drill.log"
PASS=0
FAIL=0
TOTAL=0

mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "$1" | tee -a "$LOG_FILE"
}

run_test() {
    local name="$1"
    local dir="$2"
    local pattern="$3"
    local extra_args="${4:-}"
    TOTAL=$((TOTAL + 1))

    log "  运行: $name"
    if (cd "$dir" && go test $extra_args -run "$pattern" -v -count=1 ./... >> "$LOG_FILE" 2>&1); then
        PASS=$((PASS + 1))
        log "  ✅ PASS: $name"
    else
        FAIL=$((FAIL + 1))
        log "  ❌ FAIL: $name"
    fi
}

# 清空日志
> "$LOG_FILE"

log "═══════════════════════════════════════════════════════════"
log "  M9 准入控制联合演练"
log "  时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date '+%Y-%m-%dT%H:%M:%S')"
log "═══════════════════════════════════════════════════════════"
log ""

# ─── 步骤 1: 环境检查 ───
log "--- 步骤 1: 环境检查 ---"

if ! command -v go &>/dev/null; then
    log "  ❌ Go 编译器未安装"
    exit 2
fi
log "  Go 版本: $(go version)"

GW_DIR="$PROJECT_ROOT/mirage-gateway"
OS_DIR="$PROJECT_ROOT/mirage-os"

if [ ! -d "$GW_DIR" ]; then
    log "  ❌ mirage-gateway 目录不存在: $GW_DIR"
    exit 2
fi
log "  mirage-gateway: $GW_DIR ✅"

if [ ! -d "$OS_DIR" ]; then
    log "  ⚠️  mirage-os 目录不存在: $OS_DIR（OS 侧测试将跳过）"
    OS_AVAILABLE=0
else
    log "  mirage-os: $OS_DIR ✅"
    OS_AVAILABLE=1
fi
log ""

# ─── 步骤 2: 场景 1 — 非法接入回归 ───
log "--- 步骤 2: 场景 1 — 非法接入回归 ---"

# 2a. HMAC 回归测试
run_test "HMAC 回归测试 (security_regression_test.go)" \
    "$GW_DIR" \
    "TestSecurityRegression_" \
    "-timeout 60s"

# 2b. JWT 鉴权测试
if [ "$OS_AVAILABLE" = "1" ]; then
    run_test "JWT 鉴权测试 (auth_test.go)" \
        "$OS_DIR" \
        "TestJWTAuth_" \
        "-timeout 60s"
else
    log "  ⏭️  SKIP: JWT 鉴权测试（mirage-os 不可用）"
fi

# 2c. 脱敏测试（Gateway 侧）
run_test "脱敏测试 Gateway 侧 (redact_test.go)" \
    "$GW_DIR" \
    "TestRedact" \
    "-timeout 60s"

# 2d. 脱敏测试（OS 侧）
if [ "$OS_AVAILABLE" = "1" ]; then
    run_test "脱敏测试 OS 侧 (redact_test.go)" \
        "$OS_DIR" \
        "TestRedact" \
        "-timeout 60s"
else
    log "  ⏭️  SKIP: 脱敏测试 OS 侧（mirage-os 不可用）"
fi

# 2e. 配额隔离测试（-count=10）
run_test "配额隔离测试 (-count=10)" \
    "$GW_DIR" \
    "TestQuotaBucket_IsolationTwoUsers|TestIntegration_MultiUserQuotaIsolation" \
    "-timeout 120s -count=10"

log ""

# ─── 步骤 3: 场景 2 — 配额耗尽回归 ───
log "--- 步骤 3: 场景 2 — 配额耗尽回归 ---"

# 3a. 熔断回调测试
run_test "熔断回调测试 (fuse_callback_test.go)" \
    "$GW_DIR" \
    "TestFuseCallback_" \
    "-timeout 60s"

# 3b. 集成测试（含 Critical Tests）
run_test "集成测试 + Critical Tests (integration_test.go)" \
    "$GW_DIR" \
    "TestIntegration_|TestCritical_" \
    "-timeout 120s"

# 3c. AddQuota 重置测试
run_test "AddQuota 重置测试 (quota_bucket_test.go)" \
    "$GW_DIR" \
    "TestQuotaBucket_UpdateResetsExhausted" \
    "-timeout 60s"

log ""

# ─── 步骤 4: PBT 执行 ───
log "--- 步骤 4: PBT 执行（5 个 Property Tests） ---"

# Property 1: QuotaBucket 用户隔离
run_test "Property 1: QuotaBucket 用户隔离 PBT" \
    "$GW_DIR" \
    "TestProperty_QuotaBucketIsolation" \
    "-timeout 60s"

# Property 2: FuseCallback 精确定向
run_test "Property 2: FuseCallback 精确定向 PBT" \
    "$GW_DIR" \
    "TestProperty_FuseCallbackTargeting" \
    "-timeout 60s"

# Property 3: IP 脱敏完整性
run_test "Property 3: IP 脱敏完整性 PBT" \
    "$GW_DIR" \
    "TestProperty_IPRedactionCompleteness" \
    "-timeout 60s"

# Property 4: AddQuota 重新激活
run_test "Property 4: AddQuota 重新激活 PBT" \
    "$GW_DIR" \
    "TestProperty_AddQuotaReactivation" \
    "-timeout 60s"

# Property 5: HMAC 校验确定性
run_test "Property 5: HMAC 校验确定性 PBT" \
    "$GW_DIR" \
    "TestProperty_HMACDeterminism" \
    "-timeout 60s"

log ""

# ─── 步骤 5: Redis 鉴权连通性验证 ───
log "--- 步骤 5: Redis 鉴权连通性验证 ---"

COMPOSE_FILE="$PROJECT_ROOT/deploy/docker-compose.os.yml"
TOTAL=$((TOTAL + 1))

if [ -f "$COMPOSE_FILE" ]; then
    REDIS_CHECKS=0

    if grep -q "requirepass" "$COMPOSE_FILE"; then
        log "  ✅ requirepass 已配置"
        REDIS_CHECKS=$((REDIS_CHECKS + 1))
    else
        log "  ❌ requirepass 未配置"
    fi

    if grep -q "MIRAGE_REDIS_PASSWORD" "$COMPOSE_FILE"; then
        log "  ✅ MIRAGE_REDIS_PASSWORD 环境变量已引用"
        REDIS_CHECKS=$((REDIS_CHECKS + 1))
    else
        log "  ❌ MIRAGE_REDIS_PASSWORD 未引用"
    fi

    if grep -q "redis://:" "$COMPOSE_FILE" || grep -q "REDIS_PASSWORD" "$COMPOSE_FILE"; then
        log "  ✅ 连接串带密码"
        REDIS_CHECKS=$((REDIS_CHECKS + 1))
    else
        log "  ❌ 连接串未带密码"
    fi

    if [ "$REDIS_CHECKS" -ge 2 ]; then
        PASS=$((PASS + 1))
        log "  ✅ PASS: Redis 鉴权配置一致 ($REDIS_CHECKS/3 项通过)"
    else
        FAIL=$((FAIL + 1))
        log "  ❌ FAIL: Redis 鉴权配置不一致 ($REDIS_CHECKS/3 项通过)"
    fi
else
    log "  ⏭️  SKIP: docker-compose.os.yml 不存在"
fi

log ""

# ─── 步骤 6: 汇总结果 ───
log "═══════════════════════════════════════════════════════════"
log "  M9 联合演练结果汇总"
log "  总测试组: $TOTAL"
log "  通过: $PASS"
log "  失败: $FAIL"
log "═══════════════════════════════════════════════════════════"

if [ "$FAIL" -gt 0 ]; then
    log ""
    log "⚠️  存在 $FAIL 个测试组未通过，请排查后重新验证。"
    exit 1
else
    log ""
    log "✅ 联合演练全部通过。"
    exit 0
fi
