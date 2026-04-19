-- 影子蜂窝数据库迁移脚本
-- 为 Gateway 表添加生命周期字段

-- 添加生命周期字段
ALTER TABLE gateways ADD COLUMN IF NOT EXISTS phase INTEGER DEFAULT 0 CHECK (phase BETWEEN 0 AND 2);
ALTER TABLE gateways ADD COLUMN IF NOT EXISTS incubation_started_at TIMESTAMP;
ALTER TABLE gateways ADD COLUMN IF NOT EXISTS network_quality NUMERIC(5,2) DEFAULT 0;
ALTER TABLE gateways ADD COLUMN IF NOT EXISTS baseline_rtt INTEGER DEFAULT 0;
ALTER TABLE gateways ADD COLUMN IF NOT EXISTS baseline_packet_loss NUMERIC(5,2) DEFAULT 0;

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_gateways_phase ON gateways(phase);
CREATE INDEX IF NOT EXISTS idx_gateways_incubation_started_at ON gateways(incubation_started_at);

-- 更新现有节点为服役状态
UPDATE gateways SET phase = 2 WHERE phase IS NULL OR phase = 0;

-- 添加注释
COMMENT ON COLUMN gateways.phase IS '生命周期阶段: 0=潜伏, 1=校准, 2=服役';
COMMENT ON COLUMN gateways.incubation_started_at IS '潜伏期开始时间';
COMMENT ON COLUMN gateways.network_quality IS '网络质量分数 (0-100)';
COMMENT ON COLUMN gateways.baseline_rtt IS '基准 RTT (微秒)';
COMMENT ON COLUMN gateways.baseline_packet_loss IS '基准丢包率 (0-1)';
