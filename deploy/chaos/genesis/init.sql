-- Genesis Drill 初始化数据库
-- 创建最小必要表结构

CREATE TABLE IF NOT EXISTS cells (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(64) NOT NULL,
    region VARCHAR(32) NOT NULL DEFAULT 'auto',
    level VARCHAR(16) NOT NULL DEFAULT 'STANDARD',
    cost_multiplier NUMERIC(4,2) NOT NULL DEFAULT 1.0,
    max_users INT NOT NULL DEFAULT 50,
    max_domains INT NOT NULL DEFAULT 15,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(64) PRIMARY KEY,
    cell_id UUID REFERENCES cells(id),
    is_active BOOLEAN NOT NULL DEFAULT true,
    remaining_quota NUMERIC(20,8) NOT NULL DEFAULT 0,
    total_deposit NUMERIC(20,8) NOT NULL DEFAULT 0,
    total_consumed NUMERIC(20,8) NOT NULL DEFAULT 0,
    ed25519_pubkey VARCHAR(128),
    hardware_fingerprint VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gateways (
    id VARCHAR(64) PRIMARY KEY,
    cell_id UUID REFERENCES cells(id),
    ip_address INET,
    port INT DEFAULT 443,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    last_heartbeat TIMESTAMPTZ,
    ebpf_loaded BOOLEAN NOT NULL DEFAULT false,
    threat_level INT NOT NULL DEFAULT 0,
    active_connections BIGINT NOT NULL DEFAULT 0,
    memory_usage_mb INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS billing_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(64) REFERENCES users(id),
    gateway_id VARCHAR(64) REFERENCES gateways(id),
    business_bytes BIGINT NOT NULL DEFAULT 0,
    defense_bytes BIGINT NOT NULL DEFAULT 0,
    business_cost NUMERIC(20,8) NOT NULL DEFAULT 0,
    defense_cost NUMERIC(20,8) NOT NULL DEFAULT 0,
    total_cost NUMERIC(20,8) NOT NULL DEFAULT 0,
    period_seconds INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS threat_intel (
    id BIGSERIAL PRIMARY KEY,
    source_ip INET NOT NULL,
    source_port INT,
    threat_type VARCHAR(32) NOT NULL,
    severity INT NOT NULL DEFAULT 0,
    hit_count INT NOT NULL DEFAULT 1,
    is_banned BOOLEAN NOT NULL DEFAULT false,
    reported_by_gateway VARCHAR(64),
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_ip, threat_type)
);

-- 插入测试数据
INSERT INTO cells (id, name, region, level, cost_multiplier)
VALUES ('00000000-0000-0000-0000-000000000001', 'chaos-cell', 'test', 'STANDARD', 1.0)
ON CONFLICT DO NOTHING;

INSERT INTO users (id, cell_id, is_active, remaining_quota)
VALUES ('chaos-user-001', '00000000-0000-0000-0000-000000000001', true, 0)
ON CONFLICT DO NOTHING;

INSERT INTO gateways (id, cell_id, ip_address, port, status)
VALUES
    ('gateway-alpha', '00000000-0000-0000-0000-000000000001', '10.99.0.20', 443, 'active'),
    ('gateway-bravo', '00000000-0000-0000-0000-000000000001', '10.99.0.30', 443, 'standby')
ON CONFLICT DO NOTHING;
