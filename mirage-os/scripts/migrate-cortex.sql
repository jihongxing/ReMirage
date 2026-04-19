-- Mirage-Cortex 数据库迁移
-- 指纹-IP 关联映射表

-- 指纹主表
CREATE TABLE IF NOT EXISTS fingerprints (
    id SERIAL PRIMARY KEY,
    uid VARCHAR(32) UNIQUE NOT NULL,
    hash VARCHAR(64) UNIQUE NOT NULL,
    first_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    threat_score INTEGER DEFAULT 0,
    
    -- 行为统计
    honeypot_dwell_seconds INTEGER DEFAULT 0,
    canary_triggered INTEGER DEFAULT 0,
    sql_injection_attempts INTEGER DEFAULT 0,
    dir_scan_attempts INTEGER DEFAULT 0,
    
    -- 状态标记
    is_high_risk BOOLEAN DEFAULT FALSE,
    is_trusted BOOLEAN DEFAULT FALSE,
    is_blocked BOOLEAN DEFAULT FALSE,
    
    -- 原始特征（加密存储）
    canvas_hash VARCHAR(64),
    webgl_hash VARCHAR(64),
    audio_hash VARCHAR(64),
    fonts_hash VARCHAR(64),
    user_agent TEXT,
    platform VARCHAR(64),
    language VARCHAR(16),
    screen_res VARCHAR(32),
    timezone INTEGER,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 指纹-IP 关联表（多对多）
CREATE TABLE IF NOT EXISTS fingerprint_ips (
    id SERIAL PRIMARY KEY,
    fingerprint_id INTEGER REFERENCES fingerprints(id) ON DELETE CASCADE,
    ip_address INET NOT NULL,
    first_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    request_count INTEGER DEFAULT 1,
    region VARCHAR(8),
    country VARCHAR(64),
    city VARCHAR(128),
    asn INTEGER,
    asn_org VARCHAR(256),
    
    UNIQUE(fingerprint_id, ip_address)
);

-- 区域偏好表
CREATE TABLE IF NOT EXISTS fingerprint_regions (
    id SERIAL PRIMARY KEY,
    fingerprint_id INTEGER REFERENCES fingerprints(id) ON DELETE CASCADE,
    region VARCHAR(8) NOT NULL,
    attack_count INTEGER DEFAULT 1,
    last_attack TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    UNIQUE(fingerprint_id, region)
);

-- 高危指纹缓存表（用于快速查询）
CREATE TABLE IF NOT EXISTS high_risk_fingerprints (
    hash VARCHAR(64) PRIMARY KEY,
    threat_score INTEGER NOT NULL,
    blocked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    reason TEXT
);

-- 白名单指纹表
CREATE TABLE IF NOT EXISTS trusted_fingerprints (
    hash VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64),
    device_name VARCHAR(128),
    trusted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    trusted_by VARCHAR(64)
);

-- 攻击事件日志
CREATE TABLE IF NOT EXISTS attack_events (
    id SERIAL PRIMARY KEY,
    fingerprint_id INTEGER REFERENCES fingerprints(id),
    ip_address INET,
    event_type VARCHAR(32) NOT NULL,
    event_data JSONB,
    gateway_id VARCHAR(64),
    region VARCHAR(8),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 索引优化
CREATE INDEX IF NOT EXISTS idx_fingerprints_hash ON fingerprints(hash);
CREATE INDEX IF NOT EXISTS idx_fingerprints_threat_score ON fingerprints(threat_score DESC);
CREATE INDEX IF NOT EXISTS idx_fingerprints_is_high_risk ON fingerprints(is_high_risk) WHERE is_high_risk = TRUE;
CREATE INDEX IF NOT EXISTS idx_fingerprint_ips_ip ON fingerprint_ips(ip_address);
CREATE INDEX IF NOT EXISTS idx_fingerprint_ips_fingerprint ON fingerprint_ips(fingerprint_id);
CREATE INDEX IF NOT EXISTS idx_attack_events_fingerprint ON attack_events(fingerprint_id);
CREATE INDEX IF NOT EXISTS idx_attack_events_created ON attack_events(created_at DESC);

-- 自动更新 updated_at
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER fingerprints_updated_at
    BEFORE UPDATE ON fingerprints
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

-- 统计视图
CREATE OR REPLACE VIEW cortex_stats AS
SELECT
    (SELECT COUNT(*) FROM fingerprints) AS total_fingerprints,
    (SELECT COUNT(*) FROM fingerprints WHERE is_high_risk = TRUE) AS high_risk_count,
    (SELECT COUNT(*) FROM trusted_fingerprints) AS trusted_count,
    (SELECT COUNT(DISTINCT ip_address) FROM fingerprint_ips) AS total_ips,
    (SELECT ROUND(COUNT(*) FILTER (WHERE is_high_risk) * 100.0 / NULLIF(COUNT(*), 0), 2) FROM fingerprints) AS auto_block_rate;

-- 指纹血缘查询函数
CREATE OR REPLACE FUNCTION get_fingerprint_lineage(fp_hash VARCHAR(64))
RETURNS TABLE (
    ip_address INET,
    region VARCHAR(8),
    country VARCHAR(64),
    first_seen TIMESTAMP WITH TIME ZONE,
    last_seen TIMESTAMP WITH TIME ZONE,
    request_count INTEGER
) AS $$
BEGIN
    RETURN QUERY
    SELECT fi.ip_address, fi.region, fi.country, fi.first_seen, fi.last_seen, fi.request_count
    FROM fingerprint_ips fi
    JOIN fingerprints f ON f.id = fi.fingerprint_id
    WHERE f.hash = fp_hash
    ORDER BY fi.first_seen;
END;
$$ LANGUAGE plpgsql;
