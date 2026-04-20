#!/usr/bin/env bash
# ============================================================
# Genesis Drill 快速启动
# ============================================================
# 用法:
#   ./run.sh up       — 启动混沌实验室
#   ./run.sh drill    — 执行完整三幕演习
#   ./run.sh act1     — 仅执行第一幕
#   ./run.sh act2     — 仅执行第二幕
#   ./run.sh act3     — 仅执行第三幕
#   ./run.sh down     — 销毁实验室
#   ./run.sh logs     — 查看日志
# ============================================================

set -euo pipefail

COMPOSE_FILE="../docker-compose.genesis.yml"
PROJECT_NAME="genesis"

case "${1:-help}" in
    up)
        echo "🚀 启动混沌实验室..."
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" up -d --build
        echo ""
        echo "⏳ 等待服务就绪 (30s)..."
        sleep 30
        echo "✅ 实验室就绪"
        echo ""
        echo "执行演习: $0 drill"
        ;;
    drill|full)
        echo "🎬 执行创世演习 (完整三幕)..."
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" exec drill /scripts/genesis-drill.sh full
        ;;
    act1|act2|act3)
        echo "🎬 执行第 ${1#act} 幕..."
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" exec drill /scripts/genesis-drill.sh "$1"
        ;;
    down)
        echo "💀 销毁混沌实验室..."
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" down -v --remove-orphans
        echo "✅ 已清理"
        ;;
    logs)
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs -f "${2:-}"
        ;;
    status)
        docker compose -f "$COMPOSE_FILE" -p "$PROJECT_NAME" ps
        ;;
    *)
        echo "用法: $0 [up|drill|act1|act2|act3|down|logs|status]"
        echo ""
        echo "  up       启动混沌实验室 (构建所有容器)"
        echo "  drill    执行完整三幕演习"
        echo "  act1     第一幕：创世流转 (商业闭环)"
        echo "  act2     第二幕：协议绞杀 (多路径降级)"
        echo "  act3     第三幕：焦土与复活 (信令共振)"
        echo "  down     销毁实验室"
        echo "  logs     查看日志 (可选: logs <service>)"
        echo "  status   查看容器状态"
        ;;
esac
