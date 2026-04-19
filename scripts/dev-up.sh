#!/bin/bash
# dev-up.sh - 一键拉起开发环境
# 包含: PostgreSQL + Redis + gateway-bridge + api-server + web
#
# 用法:
#   ./scripts/dev-up.sh        # 启动
#   ./scripts/dev-up.sh down   # 停止
#   ./scripts/dev-up.sh logs   # 查看日志

set -euo pipefail

COMPOSE_FILE="mirage-os/docker-compose.yaml"

case "${1:-up}" in
  up)
    echo "🚀 启动 Mirage-OS 开发环境..."
    docker compose -f "$COMPOSE_FILE" up -d --build

    echo ""
    echo "等待服务就绪..."
    sleep 5

    # 运行数据库迁移
    echo "📦 运行 Prisma 迁移..."
    docker compose -f "$COMPOSE_FILE" exec api-server npx prisma db push --skip-generate 2>/dev/null || true

    # 创建默认蜂窝
    echo "🐝 初始化默认蜂窝..."
    docker compose -f "$COMPOSE_FILE" exec -T postgres psql -U mirage -d mirage_os -c "
      INSERT INTO cells (id, name, region, level, cost_multiplier, max_users, max_domains, created_at)
      VALUES
        ('cell-sg-01', 'Singapore-Alpha', 'SG', 'STANDARD', 1.0, 50, 15, NOW()),
        ('cell-ch-01', 'Zurich-Bravo', 'CH', 'PLATINUM', 1.5, 30, 10, NOW()),
        ('cell-is-01', 'Reykjavik-Charlie', 'IS', 'DIAMOND', 2.0, 20, 8, NOW())
      ON CONFLICT (id) DO NOTHING;
    " 2>/dev/null || true

    # 创建测试邀请码
    echo "🎫 创建测试邀请码..."
    docker compose -f "$COMPOSE_FILE" exec -T postgres psql -U mirage -d mirage_os -c "
      INSERT INTO invite_codes (id, code, created_by, is_used, created_at)
      SELECT gen_random_uuid(), 'SMOKE_TEST', (SELECT id FROM users LIMIT 1), false, NOW()
      WHERE NOT EXISTS (SELECT 1 FROM invite_codes WHERE code = 'SMOKE_TEST');
    " 2>/dev/null || true

    echo ""
    echo "╔══════════════════════════════════════╗"
    echo "║   Mirage-OS 开发环境已就绪          ║"
    echo "╠══════════════════════════════════════╣"
    echo "║  Web:            http://localhost:8080 ║"
    echo "║  API:            http://localhost:3000 ║"
    echo "║  Gateway Bridge: localhost:50051       ║"
    echo "║  PostgreSQL:     localhost:5432        ║"
    echo "║  Redis:          localhost:6379        ║"
    echo "╚══════════════════════════════════════╝"
    echo ""
    echo "冒烟测试: ./scripts/smoke-test.sh"
    echo "模拟充值: ./scripts/mock-xmr-confirm.sh"
    ;;

  down)
    echo "🛑 停止开发环境..."
    docker compose -f "$COMPOSE_FILE" down -v
    echo "✅ 已停止"
    ;;

  logs)
    docker compose -f "$COMPOSE_FILE" logs -f --tail=50
    ;;

  *)
    echo "用法: $0 {up|down|logs}"
    exit 1
    ;;
esac
