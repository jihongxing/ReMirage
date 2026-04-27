#!/bin/bash
# release-verify.sh — 发布前本地复验脚本
# 把所有"靠人记得"的复验命令沉淀到一个可执行入口
# 用法: ./scripts/release-verify.sh
# 退出码: 0=全部通过, 非零=有失败项
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
FAIL=0
PASS=0

check() {
  local name="$1"
  shift
  echo "── $name ──"
  if "$@" >/dev/null 2>&1; then
    echo "  ✅ PASS"
    PASS=$((PASS+1))
  else
    echo "  ❌ FAIL"
    FAIL=$((FAIL+1))
  fi
}

echo "╔══════════════════════════════════════╗"
echo "║   Mirage Release Verification       ║"
echo "╚══════════════════════════════════════╝"
echo ""

# Gate 1: 构建
check "Gateway go vet"    bash -c "cd $PROJECT_ROOT/mirage-gateway && go vet ./..."
check "OS go vet"         bash -c "cd $PROJECT_ROOT/mirage-os && go vet ./..."
check "Gateway build"     bash -c "cd $PROJECT_ROOT/mirage-gateway && go build ./cmd/gateway/"
check "OS build"          bash -c "cd $PROJECT_ROOT/mirage-os && go build ./..."
check "Client build"      bash -c "cd $PROJECT_ROOT/phantom-client && go build ./cmd/phantom/"
check "CLI build"         bash -c "cd $PROJECT_ROOT/mirage-cli && go build ./..."

# Gate 2: 关键测试
check "Gateway tests"     bash -c "cd $PROJECT_ROOT/mirage-gateway && go test ./..."
check "OS tests"          bash -c "cd $PROJECT_ROOT/mirage-os && go test ./..."
check "Benchmarks"        bash -c "cd $PROJECT_ROOT/benchmarks && go test ./..."
check "Quota -count=10"   bash -c "cd $PROJECT_ROOT/mirage-gateway && go test -count=10 ./pkg/api/"

# Gate 3: 配置安全
check "No dangerous defaults" bash -c "! grep -q 'password: postgres\|change-this-in-production' $PROJECT_ROOT/mirage-os/configs/config.yaml"
check "Redis requirepass"     bash -c "grep -q 'requirepass' $PROJECT_ROOT/deploy/docker-compose.os.yml"

# Gate 4: 产物清洁
check "No tracked binaries" bash -c "test -z \"\$(git -C $PROJECT_ROOT ls-files '*.exe' '*.dll' | grep -v wintun.dll)\""

# Gate 5: 文档一致性
check "Audit release_ready" bash -c "grep -q 'release_ready' $PROJECT_ROOT/docs/audit-report.md"
check "No open findings"    bash -c "! grep -q '| open |' $PROJECT_ROOT/docs/audit-report.md"

echo ""
echo "════════════════════════════════════════"
echo "Result: $PASS passed, $FAIL failed"
echo "════════════════════════════════════════"

[ "$FAIL" -eq 0 ] && exit 0 || exit 1
