-- ============================================
-- ADR-001 统一数据库模型迁移脚本
-- 将 Prisma 主导的表结构对齐到 GORM Models 真相
-- ============================================

BEGIN;

-- ============================================
-- 1. gateways 表：添加 gateway_id 列（如果不存在），建立唯一索引
-- GORM 使用 gateway_id 作为业务主键，Prisma/Bridge 之前使用 id
-- ============================================

-- 如果 gateway_id 列不存在，从 id 列复制
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'gateways' AND column_name = 'gateway_id'
    ) THEN
        ALTER TABLE gateways ADD COLUMN gateway_id VARCHAR(32);
        UPDATE gateways SET gateway_id = id WHERE gateway_id IS NULL;
        ALTER TABLE gateways ALTER COLUMN gateway_id SET NOT NULL;
        CREATE UNIQUE INDEX IF NOT EXISTS idx_gateways_gateway_id ON gateways(gateway_id);
    END IF;
END $$;

-- 添加 GORM 模型中存在但 Prisma 缺失的列
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'user_id') THEN
        ALTER TABLE gateways ADD COLUMN user_id VARCHAR(64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'phase') THEN
        ALTER TABLE gateways ADD COLUMN phase INT DEFAULT 0 CHECK (phase BETWEEN 0 AND 2);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'incubation_started_at') THEN
        ALTER TABLE gateways ADD COLUMN incubation_started_at TIMESTAMPTZ;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'network_quality') THEN
        ALTER TABLE gateways ADD COLUMN network_quality NUMERIC(5,2) DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'baseline_rtt') THEN
        ALTER TABLE gateways ADD COLUMN baseline_rtt INT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'baseline_packet_loss') THEN
        ALTER TABLE gateways ADD COLUMN baseline_packet_loss NUMERIC(5,2) DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'is_online') THEN
        ALTER TABLE gateways ADD COLUMN is_online BOOLEAN DEFAULT false;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'cpu_percent') THEN
        ALTER TABLE gateways ADD COLUMN cpu_percent NUMERIC(5,2) DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'memory_bytes') THEN
        ALTER TABLE gateways ADD COLUMN memory_bytes BIGINT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'bandwidth_bps') THEN
        ALTER TABLE gateways ADD COLUMN bandwidth_bps BIGINT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'current_threat_level') THEN
        ALTER TABLE gateways ADD COLUMN current_threat_level INT DEFAULT 0;
    END IF;
END $$;

-- 重命名 last_heartbeat → last_heartbeat_at（匹配 GORM tag）
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'last_heartbeat')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gateways' AND column_name = 'last_heartbeat_at') THEN
        ALTER TABLE gateways RENAME COLUMN last_heartbeat TO last_heartbeat_at;
    END IF;
END $$;

-- ============================================
-- 2. users 表：添加 GORM 模型中存在但 Prisma 缺失的列
-- ============================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'user_id') THEN
        ALTER TABLE users ADD COLUMN user_id VARCHAR(64);
        UPDATE users SET user_id = id WHERE user_id IS NULL;
        ALTER TABLE users ALTER COLUMN user_id SET NOT NULL;
        CREATE UNIQUE INDEX IF NOT EXISTS idx_users_user_id ON users(user_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'hardware_public_key') THEN
        ALTER TABLE users ADD COLUMN hardware_public_key VARCHAR(128) DEFAULT '';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'hardware_fingerprint') THEN
        ALTER TABLE users ADD COLUMN hardware_fingerprint VARCHAR(64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'xmr_address') THEN
        ALTER TABLE users ADD COLUMN xmr_address VARCHAR(95);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'balance') THEN
        ALTER TABLE users ADD COLUMN balance NUMERIC(20,8) DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'balance_usd') THEN
        ALTER TABLE users ADD COLUMN balance_usd NUMERIC(20,2) DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'trust_score') THEN
        ALTER TABLE users ADD COLUMN trust_score INT DEFAULT 50 CHECK (trust_score BETWEEN 0 AND 100);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'status') THEN
        ALTER TABLE users ADD COLUMN status VARCHAR(16) DEFAULT 'active' CHECK (status IN ('active','suspended','banned'));
    END IF;
END $$;

-- ============================================
-- 3. billing_logs 表：添加 GORM 模型中存在但 Prisma 缺失的列
-- ============================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'billing_logs' AND column_name = 'log_type') THEN
        ALTER TABLE billing_logs ADD COLUMN log_type VARCHAR(16) DEFAULT 'traffic';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'billing_logs' AND column_name = 'log_id') THEN
        ALTER TABLE billing_logs ADD COLUMN log_id UUID DEFAULT uuid_generate_v4();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'billing_logs' AND column_name = 'cell_id') THEN
        ALTER TABLE billing_logs ADD COLUMN cell_id VARCHAR(32);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'billing_logs' AND column_name = 'total_bytes') THEN
        ALTER TABLE billing_logs ADD COLUMN total_bytes BIGINT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'billing_logs' AND column_name = 'cost_usd') THEN
        ALTER TABLE billing_logs ADD COLUMN cost_usd NUMERIC(20,8) DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'billing_logs' AND column_name = 'cost_multiplier') THEN
        ALTER TABLE billing_logs ADD COLUMN cost_multiplier NUMERIC(5,2) DEFAULT 1.0;
    END IF;
END $$;

COMMIT;
