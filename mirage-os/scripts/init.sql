-- Mirage-OS 全球资产数据库初始化脚本
-- PostgreSQL 14+ 兼容

-- ============================================
-- 扩展与配置
-- ============================================

-- 启用 UUID 扩展（用于生成唯一标识）
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 启用 TimescaleDB（可选，用于时序数据优化）
-- CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ============================================
-- 用户与资产核心
-- ============================================

-- 用户账户表
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(64) UNIQUE NOT NULL,           -- 用户唯一标识（匿名哈希）
    xmr_address VARCHAR(95) UNIQUE,                -- Monero 充值地址
    balance NUMERIC(20, 8) DEFAULT 0 NOT NULL,     -- 余额（XMR）
    balance_usd NUMERIC(20, 2) DEFAULT 0 NOT NULL, -- 余额（USD，用于计费）
    remaining_quota BIGINT DEFAULT 0 NOT NULL,     -- 剩余配额（字节）
    total_quota BIGINT DEFAULT 0 NOT NULL,         -- 总配额（字节）
    cell_level INTEGER DEFAULT 1 NOT NULL,         -- 蜂窝等级（1:标准, 2:白金, 3:钻石）
    auto_renew BOOLEAN DEFAULT false,              -- 自动续费
    quota_expires_at TIMESTAMP,                    -- 配额过期时间
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT check_balance CHECK (balance >= 0),
    CONSTRAINT check_quota CHECK (remaining_quota >= 0),
    CONSTRAINT check_cell_level CHECK (cell_level BETWEEN 1 AND 3)
);

-- 用户索引
CREATE INDEX idx_users_user_id ON users(user_id);
CREATE INDEX idx_users_xmr_address ON users(xmr_address);
CREATE INDEX idx_users_updated_at ON users(updated_at);

-- ============================================
-- 蜂窝与节点拓扑
-- ============================================

-- 蜂窝表
CREATE TABLE IF NOT EXISTS cells (
    id SERIAL PRIMARY KEY,
    cell_id VARCHAR(32) UNIQUE NOT NULL,           -- 蜂窝唯一标识
    cell_name VARCHAR(128) NOT NULL,               -- 蜂窝名称
    region_code VARCHAR(16) NOT NULL,              -- 地区代码（如 'US-West', 'HK-01'）
    country VARCHAR(2) NOT NULL,                   -- 国家代码（ISO 3166-1）
    city VARCHAR(64),                              -- 城市
    latitude NUMERIC(10, 7),                       -- 纬度
    longitude NUMERIC(10, 7),                      -- 经度
    jurisdiction VARCHAR(64),                      -- 司法管辖区
    cell_level INTEGER DEFAULT 1 NOT NULL,         -- 蜂窝等级
    cost_multiplier NUMERIC(5, 2) DEFAULT 1.0,     -- 成本倍率
    max_gateways INTEGER DEFAULT 100,              -- 最大 Gateway 数量
    status VARCHAR(16) DEFAULT 'active',           -- 状态（active/maintenance/offline）
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT check_cost_multiplier CHECK (cost_multiplier > 0),
    CONSTRAINT check_status CHECK (status IN ('active', 'maintenance', 'offline'))
);

-- 蜂窝索引
CREATE INDEX idx_cells_cell_id ON cells(cell_id);
CREATE INDEX idx_cells_region_code ON cells(region_code);
CREATE INDEX idx_cells_country ON cells(country);
CREATE INDEX idx_cells_status ON cells(status);

-- Gateway 节点表
CREATE TABLE IF NOT EXISTS gateways (
    id SERIAL PRIMARY KEY,
    gateway_id VARCHAR(32) UNIQUE NOT NULL,        -- Gateway 唯一标识
    cell_id VARCHAR(32) NOT NULL,                  -- 所属蜂窝
    user_id VARCHAR(64),                           -- 所属用户（可选）
    ip_address VARCHAR(45) NOT NULL,               -- IP 地址
    version VARCHAR(16),                           -- Gateway 版本
    current_threat_level INTEGER DEFAULT 0,        -- 当前威胁等级
    active_connections INTEGER DEFAULT 0,          -- 活跃连接数
    cpu_percent NUMERIC(5, 2) DEFAULT 0,           -- CPU 占用率
    memory_bytes BIGINT DEFAULT 0,                 -- 内存占用
    bandwidth_bps BIGINT DEFAULT 0,                -- 带宽使用
    is_online BOOLEAN DEFAULT false,               -- 是否在线
    last_heartbeat_at TIMESTAMP,                   -- 最后心跳时间
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT fk_gateway_cell FOREIGN KEY (cell_id) REFERENCES cells(cell_id) ON DELETE CASCADE
);

-- Gateway 索引
CREATE INDEX idx_gateways_gateway_id ON gateways(gateway_id);
CREATE INDEX idx_gateways_cell_id ON gateways(cell_id);
CREATE INDEX idx_gateways_user_id ON gateways(user_id);
CREATE INDEX idx_gateways_is_online ON gateways(is_online);
CREATE INDEX idx_gateways_last_heartbeat ON gateways(last_heartbeat_at);

-- ============================================
-- 计费流水与情报仓库
-- ============================================

-- 计费流水表（高频写入）
CREATE TABLE IF NOT EXISTS billing_logs (
    id BIGSERIAL PRIMARY KEY,
    log_id UUID DEFAULT uuid_generate_v4(),        -- 流水唯一标识
    gateway_id VARCHAR(32) NOT NULL,               -- Gateway ID
    user_id VARCHAR(64),                           -- 用户 ID
    cell_id VARCHAR(32),                           -- 蜂窝 ID
    business_bytes BIGINT DEFAULT 0,               -- 业务流量（字节）
    defense_bytes BIGINT DEFAULT 0,                -- 防御流量（字节）
    total_bytes BIGINT DEFAULT 0,                  -- 总流量
    cost_usd NUMERIC(20, 8) DEFAULT 0,             -- 费用（USD）
    cost_multiplier NUMERIC(5, 2) DEFAULT 1.0,     -- 成本倍率
    log_type VARCHAR(16) DEFAULT 'traffic',        -- 流水类型（traffic/deposit/purchase/refund）
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT check_bytes CHECK (business_bytes >= 0 AND defense_bytes >= 0)
);

-- 计费流水索引
CREATE INDEX idx_billing_logs_gateway_id ON billing_logs(gateway_id);
CREATE INDEX idx_billing_logs_user_id ON billing_logs(user_id);
CREATE INDEX idx_billing_logs_created_at ON billing_logs(created_at DESC);
CREATE INDEX idx_billing_logs_log_type ON billing_logs(log_type);

-- 转换为时序表（可选，需要 TimescaleDB）
-- SELECT create_hypertable('billing_logs', 'created_at', if_not_exists => TRUE);

-- 威胁情报表
CREATE TABLE IF NOT EXISTS threat_intel (
    id BIGSERIAL PRIMARY KEY,
    gateway_id VARCHAR(32),                        -- 检测到的 Gateway
    src_ip VARCHAR(45) NOT NULL,                   -- 源 IP
    src_port INTEGER,                              -- 源端口
    threat_type INTEGER NOT NULL,                  -- 威胁类型（1:探测, 2:扫描, 3:DPI, 4:时序攻击）
    severity INTEGER DEFAULT 5,                    -- 严重程度（0-10）
    ja4_fingerprint VARCHAR(64),                   -- JA4 指纹
    packet_count INTEGER DEFAULT 1,                -- 数据包计数
    hit_count BIGINT DEFAULT 1,                    -- 命中次数
    first_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT check_threat_type CHECK (threat_type BETWEEN 0 AND 10),
    CONSTRAINT check_severity CHECK (severity BETWEEN 0 AND 10)
);

-- 威胁情报索引
CREATE INDEX idx_threat_intel_src_ip ON threat_intel(src_ip);
CREATE INDEX idx_threat_intel_ja4 ON threat_intel(ja4_fingerprint);
CREATE INDEX idx_threat_intel_threat_type ON threat_intel(threat_type);
CREATE INDEX idx_threat_intel_last_seen ON threat_intel(last_seen DESC);
CREATE INDEX idx_threat_intel_gateway_id ON threat_intel(gateway_id);

-- 转换为时序表（可选）
-- SELECT create_hypertable('threat_intel', 'last_seen', if_not_exists => TRUE);

-- ============================================
-- 充值记录表
-- ============================================

CREATE TABLE IF NOT EXISTS deposits (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,                  -- 用户 ID
    tx_hash VARCHAR(64) UNIQUE NOT NULL,           -- Monero 交易哈希
    amount_xmr NUMERIC(20, 12) NOT NULL,           -- 充值金额（XMR）
    amount_usd NUMERIC(20, 2) NOT NULL,            -- 等值美元
    exchange_rate NUMERIC(20, 8) NOT NULL,         -- 汇率（XMR/USD）
    status VARCHAR(16) DEFAULT 'pending',          -- 状态（pending/confirmed/failed）
    confirmations INTEGER DEFAULT 0,               -- 确认数
    confirmed_at TIMESTAMP,                        -- 确认时间
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT check_amount CHECK (amount_xmr > 0),
    CONSTRAINT check_status CHECK (status IN ('pending', 'confirmed', 'failed'))
);

-- 充值记录索引
CREATE INDEX idx_deposits_user_id ON deposits(user_id);
CREATE INDEX idx_deposits_tx_hash ON deposits(tx_hash);
CREATE INDEX idx_deposits_status ON deposits(status);
CREATE INDEX idx_deposits_created_at ON deposits(created_at DESC);

-- ============================================
-- 流量包购买记录
-- ============================================

CREATE TABLE IF NOT EXISTS quota_purchases (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,                  -- 用户 ID
    package_type VARCHAR(16) NOT NULL,             -- 流量包类型（10GB/50GB/100GB/500GB/1TB）
    quota_bytes BIGINT NOT NULL,                   -- 购买流量（字节）
    cost_usd NUMERIC(20, 2) NOT NULL,              -- 费用（USD）
    cell_level INTEGER DEFAULT 1,                  -- 蜂窝等级
    expires_at TIMESTAMP,                          -- 过期时间
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT check_quota_bytes CHECK (quota_bytes > 0),
    CONSTRAINT check_cost CHECK (cost_usd > 0)
);

-- 流量包购买索引
CREATE INDEX idx_quota_purchases_user_id ON quota_purchases(user_id);
CREATE INDEX idx_quota_purchases_created_at ON quota_purchases(created_at DESC);

-- ============================================
-- 触发器：自动更新 updated_at
-- ============================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_cells_updated_at BEFORE UPDATE ON cells
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_gateways_updated_at BEFORE UPDATE ON gateways
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================
-- 初始化数据
-- ============================================

-- 插入默认蜂窝
INSERT INTO cells (cell_id, cell_name, region_code, country, city, cell_level, cost_multiplier, status)
VALUES 
    ('cell-us-west-01', 'US West Standard', 'US-West', 'US', 'Los Angeles', 1, 1.0, 'active'),
    ('cell-hk-01', 'Hong Kong Platinum', 'HK-01', 'HK', 'Hong Kong', 2, 1.5, 'active'),
    ('cell-sg-diamond-01', 'Singapore Diamond', 'SG-01', 'SG', 'Singapore', 3, 2.0, 'active')
ON CONFLICT (cell_id) DO NOTHING;

-- ============================================
-- 视图：实时统计
-- ============================================

-- 用户流量统计视图
CREATE OR REPLACE VIEW user_traffic_stats AS
SELECT 
    user_id,
    COUNT(*) as log_count,
    SUM(business_bytes) as total_business_bytes,
    SUM(defense_bytes) as total_defense_bytes,
    SUM(total_bytes) as total_bytes,
    SUM(cost_usd) as total_cost_usd,
    MAX(created_at) as last_activity
FROM billing_logs
WHERE log_type = 'traffic'
GROUP BY user_id;

-- Gateway 健康状态视图
CREATE OR REPLACE VIEW gateway_health AS
SELECT 
    g.gateway_id,
    g.cell_id,
    g.is_online,
    g.current_threat_level,
    g.last_heartbeat_at,
    EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - g.last_heartbeat_at)) as seconds_since_heartbeat,
    CASE 
        WHEN g.is_online AND EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - g.last_heartbeat_at)) < 60 THEN 'healthy'
        WHEN g.is_online AND EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - g.last_heartbeat_at)) < 120 THEN 'warning'
        ELSE 'critical'
    END as health_status
FROM gateways g;

-- 蜂窝负载统计视图
CREATE OR REPLACE VIEW cell_load_stats AS
SELECT 
    c.cell_id,
    c.cell_name,
    c.max_gateways,
    COUNT(g.gateway_id) as current_gateways,
    COUNT(CASE WHEN g.is_online THEN 1 END) as online_gateways,
    ROUND(COUNT(g.gateway_id)::NUMERIC / c.max_gateways * 100, 2) as load_percent
FROM cells c
LEFT JOIN gateways g ON c.cell_id = g.cell_id
GROUP BY c.cell_id, c.cell_name, c.max_gateways;

-- ============================================
-- 完成
-- ============================================

COMMENT ON TABLE users IS '用户账户与配额表';
COMMENT ON TABLE cells IS '蜂窝拓扑表';
COMMENT ON TABLE gateways IS 'Gateway 节点状态表';
COMMENT ON TABLE billing_logs IS '计费流水表（高频写入）';
COMMENT ON TABLE threat_intel IS '威胁情报库';
COMMENT ON TABLE deposits IS 'Monero 充值记录';
COMMENT ON TABLE quota_purchases IS '流量包购买记录';
