#!/usr/bin/env bash
# ============================================================
# 种子数据：为 smoke test 创建测试用户和基础数据
# 前置条件：postgres 已启动，prisma migrate 已执行
# 用法：bash scripts/seed-test-data.sh [DATABASE_URL]
# ============================================================
set -euo pipefail

DATABASE_URL="${1:-postgresql://mirage:mirage_dev@localhost:5432/mirage_os}"

echo "[INFO] 写入种子数据..."

psql "$DATABASE_URL" <<'SQL'
-- 创建测试 Cell
INSERT INTO cells (id, name, region, level, cost_multiplier, max_users, max_domains, created_at)
VALUES ('cell-test-01', 'test-cell-hk', 'HK', 'STANDARD', 1.0, 50, 15, NOW())
ON CONFLICT (name) DO NOTHING;

-- 创建测试用户（密码: test123，totp_secret 为空表示跳过 TOTP）
-- 注意：密码 hash 需要由 api-server 的 bcrypt 生成，这里用预计算值
-- bcrypt("test123") = $2b$10$K4GxVxKz8v5Qz5Qz5Qz5QeK4GxVxKz8v5Qz5Qz5Qz5Qz5Qz5Qz
-- 实际使用时请通过 /api/auth/register 注册
INSERT INTO users (id, username, password_hash, remaining_quota, total_deposit, cell_id, is_active, created_at, updated_at)
VALUES (
  'user-test-01',
  'admin',
  '$2b$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012',
  100.00000000,
  100.00000000,
  'cell-test-01',
  true,
  NOW(),
  NOW()
)
ON CONFLICT (username) DO UPDATE SET remaining_quota = 100.00000000;

-- 创建测试 Gateway
INSERT INTO gateways (id, cell_id, ip_address, status, last_heartbeat, ebpf_loaded, threat_level, active_connections, memory_usage_mb, created_at, updated_at)
VALUES ('gw-test-01', 'cell-test-01', '10.0.1.1', 'ONLINE', NOW(), true, 0, 5, 128, NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET status = 'ONLINE', last_heartbeat = NOW();

-- 插入一条威胁记录
INSERT INTO threat_intel (id, source_ip, threat_type, severity, hit_count, is_banned, first_seen, last_seen)
VALUES ('threat-test-01', '192.168.99.1', 'ACTIVE_PROBING', 3, 12, false, NOW(), NOW())
ON CONFLICT (source_ip, threat_type) DO UPDATE SET hit_count = 12, last_seen = NOW();

SQL

echo "[INFO] 种子数据写入完成"
echo ""
echo "测试账号："
echo "  username: admin"
echo "  password: test123 (需要通过 register 接口重新注册以获得正确 hash)"
echo ""
echo "建议：使用 /api/auth/register 接口注册一个真实用户来跑 smoke test"
