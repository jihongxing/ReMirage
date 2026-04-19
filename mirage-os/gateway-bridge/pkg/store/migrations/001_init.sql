-- Mirage-OS 初始化 Schema（与 Prisma Schema 对齐）

CREATE TYPE cell_level AS ENUM ('STANDARD', 'PLATINUM', 'DIAMOND');
CREATE TYPE gateway_status AS ENUM ('ONLINE', 'DEGRADED', 'OFFLINE');
CREATE TYPE deposit_status AS ENUM ('PENDING', 'CONFIRMED', 'FAILED');

CREATE TABLE IF NOT EXISTS cells (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL,
    region VARCHAR(255) NOT NULL,
    level cell_level NOT NULL DEFAULT 'STANDARD',
    cost_multiplier NUMERIC(20,8) NOT NULL DEFAULT 1.0,
    max_users INT NOT NULL DEFAULT 50,
    max_domains INT NOT NULL DEFAULT 15,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    ed25519_pubkey VARCHAR(255),
    totp_secret VARCHAR(255),
    remaining_quota NUMERIC(20,8) NOT NULL DEFAULT 0,
    total_deposit NUMERIC(20,8) NOT NULL DEFAULT 0,
    total_consumed NUMERIC(20,8) NOT NULL DEFAULT 0,
    cell_id UUID REFERENCES cells(id),
    invite_code_used VARCHAR(255),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gateways (
    id VARCHAR(255) PRIMARY KEY,
    cell_id UUID REFERENCES cells(id),
    ip_address VARCHAR(255),
    status gateway_status NOT NULL DEFAULT 'OFFLINE',
    last_heartbeat TIMESTAMPTZ,
    ebpf_loaded BOOLEAN NOT NULL DEFAULT FALSE,
    threat_level INT NOT NULL DEFAULT 0,
    active_connections BIGINT NOT NULL DEFAULT 0,
    memory_usage_mb INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS billing_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    gateway_id VARCHAR(255) NOT NULL REFERENCES gateways(id),
    business_bytes BIGINT NOT NULL,
    defense_bytes BIGINT NOT NULL,
    business_cost NUMERIC(20,8) NOT NULL,
    defense_cost NUMERIC(20,8) NOT NULL,
    total_cost NUMERIC(20,8) NOT NULL,
    period_seconds INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS threat_intel (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_ip VARCHAR(255) NOT NULL,
    source_port INT,
    threat_type VARCHAR(255) NOT NULL,
    severity INT NOT NULL DEFAULT 0,
    hit_count INT NOT NULL DEFAULT 1,
    is_banned BOOLEAN NOT NULL DEFAULT FALSE,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reported_by_gateway VARCHAR(255),
    UNIQUE(source_ip, threat_type)
);

CREATE TABLE IF NOT EXISTS deposits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    amount NUMERIC(20,8) NOT NULL,
    currency VARCHAR(10) NOT NULL DEFAULT 'USD',
    tx_hash VARCHAR(255),
    status deposit_status NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quota_purchases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    quota_gb NUMERIC(20,8) NOT NULL,
    price NUMERIC(20,8) NOT NULL,
    cell_level VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invite_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(255) UNIQUE NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    used_by UUID UNIQUE REFERENCES users(id),
    is_used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    used_at TIMESTAMPTZ
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_billing_logs_user_id ON billing_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_billing_logs_created_at ON billing_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_threat_intel_source_ip ON threat_intel(source_ip);
CREATE INDEX IF NOT EXISTS idx_threat_intel_is_banned ON threat_intel(is_banned);
CREATE INDEX IF NOT EXISTS idx_gateways_cell_id ON gateways(cell_id);
CREATE INDEX IF NOT EXISTS idx_gateways_status ON gateways(status);
CREATE INDEX IF NOT EXISTS idx_users_cell_id ON users(cell_id);
