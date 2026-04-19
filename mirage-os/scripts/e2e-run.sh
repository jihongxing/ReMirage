#!/usr/bin/env bash
# ============================================================
# 一键端到端试运行
# 启动全栈 → 等待就绪 → 跑 smoke test → 输出结果
# 用法：bash scripts/e2e-run.sh
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "============================================"
echo " Mirage-OS 端到端试运行"
echo "============================================"
echo ""

# ------------------------------------------
# Step 1: 启动服务
# ------------------------------------------
echo "[1/5] 启动 docker compose..."
docker compose up -d --build

# ------------------------------------------
# Step 2: 等待服务就绪
# ------------------------------------------
echo "[2/5] 等待服务就绪..."

wait_for() {
  local name="$1"
  local url="$2"
  local max_wait="${3:-30}"
  local elapsed=0

  while [ $elapsed -lt $max_wait ]; do
    if curl -sf "$url" > /dev/null 2>&1; then
      echo "  ✓ $name 就绪 (${elapsed}s)"
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "  ✗ $name 超时 (${max_wait}s)"
  return 1
}

wait_for "PostgreSQL (via api-server)" "http://localhost:3000/health" 60
wait_for "gateway-bridge" "http://localhost:7000/internal/health" 30
wait_for "web" "http://localhost:8080/" 30

# ------------------------------------------
# Step 3: 执行 Prisma Migrate
# ------------------------------------------
echo ""
echo "[3/5] 执行 Prisma Migrate..."
docker compose exec -T api-server npx prisma migrate deploy 2>/dev/null || {
  echo "  ⚠ prisma migrate 跳过（可能已是最新）"
}

# ------------------------------------------
# Step 4: 种子数据（可选）
# ------------------------------------------
echo ""
echo "[4/5] 检查种子数据..."
# 如果 psql 可用，尝试写入种子数据
if command -v psql &> /dev/null; then
  bash "$SCRIPT_DIR/seed-test-data.sh" "postgresql://mirage:mirage_dev@localhost:5432/mirage_os" 2>/dev/null || {
    echo "  ⚠ 种子数据跳过"
  }
else
  echo "  ⚠ psql 不可用，跳过种子数据"
fi

# ------------------------------------------
# Step 5: 跑 Smoke Test
# ------------------------------------------
echo ""
echo "[5/5] 执行 Smoke Test..."
echo ""
bash "$SCRIPT_DIR/smoke-test.sh"

echo ""
echo "============================================"
echo " 试运行完成。查看上方结果。"
echo " 停止服务：docker compose down"
echo "============================================"
