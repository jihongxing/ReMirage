---
Status: input
Target Truth: docs/governance/boundaries/component-responsibilities.md
Migration: OS 职责定义已迁移到 component-responsibilities.md，本文保留为组件输入材料
---

# Mirage-Brain 控制中心设计

## 项目定位

**项目二：Mirage-Brain（M.C.C. & Web 后台）**

- **任务**：全球域名流水线调度、蜂窝隔离算法、死因分析算法、Web 指挥中心
- **架构**：高性能微服务（Go/Rust）+ 前端看板
- **角色**：战区指挥部

---

## 一、核心任务

### 1.1 全球域名流水线调度

| 能力 | 技术实现 |
|------|---------|
| 域名自动采购 | Monero 混币 + 一次性身份 |
| 温储备池管理 | 10-15 个待命域名 |
| 司法管辖区轮转 | 避开 14 Eyes + 动态评分 |
| G-Switch 扩散 | Raft 一致性 + 秒级同步 |
| 死信开关 | 24 小时心跳 + 自动自毁 |

### 1.2 蜂窝隔离算法

| 能力 | 技术实现 |
|------|---------|
| 用户分组隔离 | 200 用户 / 10 蜂窝 |
| 资源池独立 | 域名/IP 不交叉 |
| 风险对冲 | 单蜂窝被渗透不影响其他 |
| 动态防火墙 | 蜂窝遭 DDoS 自动熔断 |
| 洁净区溢价 | 处女地群组商业化 |

### 1.3 死因分析算法

| 能力 | 技术实现 |
|------|---------|
| 封禁特征识别 | DNS 污染 / IP 封锁 / 证书吊销 |
| 存活周期建模 | 不同注册商/后缀平均寿命 |
| 对抗策略评估 | 策略 A vs B 存活率对比 |
| 自动优化建议 | 避开高风险注册商/增加轮换频率 |
| 情报库进化 | 群体免疫 + 持续学习 |

### 1.4 Web 指挥中心

| 能力 | 技术实现 |
|------|---------|
| 蜂窝可视化仪表盘 | 实时态势图 + 健康心跳 |
| 策略热下发 | 拟态切换 + NPM 填充控制 |
| 威胁情报中心 | 威胁溯源 + 黑名单分发 |
| 死因分析报告 | 自动生成 + 优化建议 |
| 用户管理 | 邀请制 + 连坐制 + 付费墙 |

---

## 二、技术架构

### 2.1 微服务架构

```
┌─────────────────────────────────────────────────────────┐
│              Mirage-Brain 微服务架构                     │
├─────────────────────────────────────────────────────────┤
│ API Gateway (Go + Gin)                                   │
│   ├─ RESTful API                                         │
│   ├─ WebSocket (实时推送)                                │
│   ├─ gRPC (Gateway 通信)                                 │
│   └─ GraphQL (前端查询)                                  │
├─────────────────────────────────────────────────────────┤
│ 核心服务                                                  │
│   ├─ Cellular Service (Go)      # 蜂窝管理              │
│   ├─ Domain Service (Go)        # 域名流水线            │
│   ├─ Strategy Service (Rust)    # 策略引擎              │
│   ├─ Intelligence Service (Go)  # 威胁情报              │
│   └─ Analytics Service (Rust)   # 死因分析              │
├─────────────────────────────────────────────────────────┤
│ 数据层                                                    │
│   ├─ PostgreSQL (用户/蜂窝/域名)                         │
│   ├─ Redis (缓存/会话/队列)                              │
│   ├─ TimescaleDB (时序数据)                              │
│   ├─ ClickHouse (日志分析)                               │
│   └─ IPFS (碎片化存储)                                   │
├─────────────────────────────────────────────────────────┤
│ 消息队列                                                  │
│   ├─ NATS (服务间通信)                                   │
│   └─ Kafka (事件流)                                      │
└─────────────────────────────────────────────────────────┘
```

### 2.2 代码结构

```
mirage-brain/
├── services/
│   ├── api-gateway/         # API 网关
│   ├── cellular/            # 蜂窝管理服务
│   ├── domain/              # 域名流水线服务
│   ├── strategy/            # 策略引擎服务 (Rust)
│   ├── intelligence/        # 威胁情报服务
│   └── analytics/           # 死因分析服务 (Rust)
├── web/
│   ├── frontend/            # React + TypeScript
│   │   ├── src/
│   │   │   ├── components/  # 组件
│   │   │   ├── pages/       # 页面
│   │   │   ├── hooks/       # Hooks
│   │   │   └── utils/       # 工具
│   │   └── public/
│   └── admin/               # 管理后台
├── pkg/
│   ├── models/              # 数据模型
│   ├── crypto/              # 加密工具
│   ├── tor/                 # Tor 集成
│   └── ipfs/                # IPFS 集成
├── configs/
│   └── brain.yaml           # 配置文件
└── scripts/
    ├── build.sh             # 编译脚本
    ├── deploy.sh            # 部署脚本
    └── migrate.sh           # 数据库迁移
```

---

## 三、核心服务实现

### 3.1 域名流水线服务（Go）

```go
// services/domain/pipeline.go
package domain

type DomainPipeline struct {
    purchaser *Purchaser
    warmPool  *WarmPool
    coldPool  *ColdPool
    gswitcher *GSwitcher
}

// 自动采购域名
func (dp *DomainPipeline) AutoPurchase() {
    ticker := time.NewTicker(6 * time.Hour)
    
    for range ticker.C {
        // 1. 检查温储备池
        if dp.warmPool.Size() < 10 {
            // 2. 从冷储备池激活
            domains := dp.coldPool.Activate(5)
            dp.warmPool.Add(domains)
        }
        
        // 3. 检查冷储备池
        if dp.coldPool.Size() < 20 {
            // 4. 自动采购新域名
            newDomains := dp.purchaser.Purchase(10)
            dp.coldPool.Add(newDomains)
        }
    }
}

// 域名采购器
type Purchaser struct {
    monero *MoneroWallet
    tor    *TorClient
}

func (p *Purchaser) Purchase(count int) []*Domain {
    domains := make([]*Domain, 0, count)
    
    for i := 0; i < count; i++ {
        // 1. 生成一次性身份
        identity := p.generateDisposableIdentity()
        
        // 2. 选择注册商（避开高风险）
        registrar := p.selectRegistrar()
        
        // 3. Monero 混币支付
        payment := p.monero.CreatePayment()
        
        // 4. 通过 Tor 注册
        domain := p.tor.RegisterDomain(registrar, identity, payment)
        
        domains = append(domains, domain)
    }
    
    return domains
}
```

### 3.2 蜂窝管理服务（Go）

```go
// services/cellular/manager.go
package cellular

type CellularManager struct {
    cells map[string]*Cell
    users map[string]*User
    db    *Database
}

// 蜂窝隔离算法
func (cm *CellularManager) IsolateCell(cellID string) error {
    cell := cm.cells[cellID]
    
    // 1. 资源池独立
    cell.DomainPool = cm.allocateDomainPool(cell)
    cell.IPPool = cm.allocateIPPool(cell)
    
    // 2. 网络隔离
    cell.VLANTag = cm.allocateVLAN(cell)
    
    // 3. 策略隔离
    cell.Strategy = cm.computeStrategy(cell)
    
    return nil
}

// 动态防火墙
func (cm *CellularManager) MonitorCells() {
    ticker := time.NewTicker(1 * time.Minute)
    
    for range ticker.C {
        for _, cell := range cm.cells {
            // 检测 DDoS
            if cell.IncomingTraffic > cell.Threshold {
                // 自动熔断
                cm.CircuitBreak(cell.ID)
            }
        }
    }
}
```

### 3.3 死因分析服务（Rust）

```rust
// services/analytics/death_analyzer.rs
use std::collections::HashMap;

pub struct DeathAnalyzer {
    db: Database,
    ml_model: MLModel,
}

impl DeathAnalyzer {
    // 分析域名死因
    pub fn analyze(&self, domain: &Domain) -> DeathReport {
        // 1. 收集特征
        let features = self.extract_features(domain);
        
        // 2. ML 分类
        let death_type = self.ml_model.predict(&features);
        
        // 3. 生成报告
        DeathReport {
            domain: domain.name.clone(),
            death_time: domain.death_time,
            survival_time: domain.survival_time(),
            death_type,
            probability: self.calculate_probability(&features),
            triggers: self.identify_triggers(domain),
            recommendations: self.generate_recommendations(domain),
        }
    }
    
    // 提取特征
    fn extract_features(&self, domain: &Domain) -> Features {
        Features {
            packet_loss_rate: domain.metrics.packet_loss,
            rtt_variance: domain.metrics.rtt_variance,
            icmp_unreachable: domain.metrics.icmp_count,
            dns_response_time: domain.metrics.dns_time,
            registrar: domain.registrar.clone(),
            tld: domain.tld.clone(),
        }
    }
    
    // 生成优化建议
    fn generate_recommendations(&self, domain: &Domain) -> Vec<String> {
        let mut recs = Vec::new();
        
        // 基于历史数据
        let stats = self.db.get_registrar_stats(&domain.registrar);
        
        if stats.avg_survival_time < 24.0 {
            recs.push(format!("避开该注册商（{}）", domain.registrar));
        }
        
        if domain.survival_time() < 12.0 {
            recs.push("提高域名轮换频率（24h → 12h）".to_string());
        }
        
        recs
    }
}
```

### 3.4 策略引擎服务（Rust）

```rust
// services/strategy/engine.rs
pub struct StrategyEngine {
    rules: Vec<Rule>,
    context: Context,
}

impl StrategyEngine {
    // 计算最优策略
    pub fn compute(&self, user: &User) -> Strategy {
        // 1. 环境感知
        let context = self.detect_context(user);
        
        // 2. 威胁评估
        let threat_level = self.assess_threat(&context);
        
        // 3. 策略选择
        Strategy {
            mimicry_template: self.select_mimicry(&context),
            jurisdiction_path: self.select_path(&context),
            domain_pool: self.select_domains(threat_level),
            padding_ratio: self.calculate_padding(threat_level),
            performance: self.select_congestion_control(&context),
        }
    }
    
    // 环境感知
    fn detect_context(&self, user: &User) -> Context {
        Context {
            geo_location: user.geo_location.clone(),
            time_of_day: chrono::Local::now().hour(),
            network_type: self.detect_network_type(user),
            business_type: self.detect_business_type(user),
        }
    }
}
```

---

## 四、Web 前端实现

### 4.1 蜂窝可视化仪表盘（React + TypeScript）

```typescript
// web/frontend/src/pages/Dashboard.tsx
import React, { useEffect, useState } from 'react';
import { useWebSocket } from '../hooks/useWebSocket';
import { CellMap } from '../components/CellMap';
import { HealthMonitor } from '../components/HealthMonitor';

export const Dashboard: React.FC = () => {
  const [cells, setCells] = useState<Cell[]>([]);
  const ws = useWebSocket('wss://brain.mirage.local/ws');
  
  useEffect(() => {
    ws.on('cell_update', (data) => {
      setCells(data.cells);
    });
  }, [ws]);
  
  return (
    <div className="dashboard">
      <CellMap cells={cells} />
      <HealthMonitor cells={cells} />
    </div>
  );
};
```

### 4.2 策略热下发控制台

```typescript
// web/frontend/src/pages/StrategyControl.tsx
import React, { useState } from 'react';
import { api } from '../utils/api';

export const StrategyControl: React.FC = () => {
  const [cellID, setCellID] = useState('');
  const [template, setTemplate] = useState('Conference-Pro');
  
  const handleSwitch = async () => {
    await api.post('/strategy/switch', {
      cell_id: cellID,
      template_id: template,
    });
  };
  
  return (
    <div className="strategy-control">
      <select onChange={(e) => setCellID(e.target.value)}>
        {/* 蜂窝列表 */}
      </select>
      
      <select onChange={(e) => setTemplate(e.target.value)}>
        <option value="Conference-Pro">Conference-Pro (Zoom)</option>
        <option value="Cinema-Ultra">Cinema-Ultra (Netflix)</option>
        <option value="Gamer-Zero">Gamer-Zero (Steam)</option>
      </select>
      
      <button onClick={handleSwitch}>立即切换</button>
    </div>
  );
};
```

---

## 五、部署架构

```
┌─────────────────────────────────────────────────────────┐
│              Mirage-Brain 部署拓扑                       │
├─────────────────────────────────────────────────────────┤
│ 节点 #1: 冰岛（主节点）                                   │
│   ├─ API Gateway                                         │
│   ├─ 所有微服务                                          │
│   ├─ PostgreSQL (主)                                     │
│   ├─ Redis (主)                                          │
│   └─ Web 前端                                            │
├─────────────────────────────────────────────────────────┤
│ 节点 #2: 瑞士（热备）                                     │
│   ├─ API Gateway                                         │
│   ├─ 核心服务                                            │
│   ├─ PostgreSQL (从)                                     │
│   └─ Redis (从)                                          │
├─────────────────────────────────────────────────────────┤
│ 节点 #3: 新加坡（热备）                                   │
│   ├─ API Gateway                                         │
│   ├─ 核心服务                                            │
│   ├─ PostgreSQL (从)                                     │
│   └─ Redis (从)                                          │
└─────────────────────────────────────────────────────────┘
         ↓ Raft 一致性 + 自动故障转移
┌─────────────────────────────────────────────────────────┐
│              全球 Gateway 节点                           │
│   ├─ 100+ 边缘节点                                       │
│   └─ 通过 3 跳匿名 + Tor 连接 Brain                      │
└─────────────────────────────────────────────────────────┘
```

---

## 六、开发路线图

### Phase 1: 核心服务（6 周）

- 蜂窝管理服务
- 域名流水线服务
- 策略引擎服务
- 数据库设计

### Phase 2: Web 后台（4 周）

- 蜂窝可视化仪表盘
- 策略热下发控制台
- 威胁情报中心
- 用户管理

### Phase 3: 高级功能（4 周）

- 死因分析服务
- 威胁情报聚合
- 自动化运维
- 监控告警

### Phase 4: 优化与测试（2 周）

- 性能优化
- 压力测试
- 安全审计
- 文档完善

### 1.1 蜂窝可视化仪表盘 (Cellular Dashboard)

**实时态势图**：

```
┌─────────────────────────────────────────────────────────┐
│              全球蜂窝分布态势图                           │
├─────────────────────────────────────────────────────────┤
│  🟢 蜂窝 #1 (US-East)    ✅ 健康  15 域名  120 用户      │
│  🟢 蜂窝 #2 (EU-West)    ✅ 健康  12 域名   85 用户      │
│  🟡 蜂窝 #3 (ASIA-SG)    ⚠️  警告   8 域名   45 用户      │
│  🔴 蜂窝 #4 (US-West)    ❌ 故障   3 域名   20 用户      │
└─────────────────────────────────────────────────────────┘
```

**健康心跳可视化**：

| 域名 | 状态 | 存活时间 | 流量 | 威胁等级 |
|------|------|---------|------|---------|
| node-a1.example.com | 🟢 在线 | 72h | 1.2GB/h | 低 |
| node-b2.example.com | 🟡 警告 | 48h | 2.5GB/h | 中 |
| node-c3.example.com | 🔴 被封 | 12h | 0 | 高 |
| node-d4.example.com | 🔄 转生中 | - | - | - |

**转生动画**：

```
域名 node-c3.example.com 检测到封锁
    ↓ (< 5s)
从温储备池激活 node-e5.example.com
    ↓ (< 30s)
后台预握手 + 多路径重叠传输
    ↓ (< 60s)
逐步迁移流量 (0% → 100%)
    ↓
✅ 转生完成，用户无感知
```

**战损统计**：

```
过去 24 小时：
├─ 域名报废：8 个
├─ 平均存活时间：36.5 小时
├─ 转生成功率：99.2%
├─ 用户感知中断：0 次
└─ Survival ROI：92%
```

---

### 1.2 策略热下发 (Hot-Swapping)

**拟态切换器**：

```
┌─────────────────────────────────────────────────────────┐
│              拟态模板切换                                 │
├─────────────────────────────────────────────────────────┤
│ 蜂窝选择：[蜂窝 #3 (ASIA-SG)        ▼]                  │
│                                                          │
│ 当前模板：Conference-Pro (Zoom)                          │
│                                                          │
│ 切换至：                                                  │
│   ○ Conference-Pro (Zoom/Teams)                         │
│   ● Cinema-Ultra (Netflix/Disney+)                      │
│   ○ Social-Pulse (WhatsApp/Meta)                        │
│   ○ Gamer-Zero (Steam/FPS)                              │
│   ○ VoIP-Stable (Skype/Discord)                         │
│                                                          │
│ [立即切换]  [定时切换]  [自动适应]                        │
└─────────────────────────────────────────────────────────┘
```

**防御强度调节（安全 vs 成本）**：

```
┌─────────────────────────────────────────────────────────┐
│              防御强度调节                                 │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  安全优先 ←─────────●─────────→ 成本优先                │
│           [━━━━━━━━━━━━━━━━━━━━]                        │
│            10%  15%  20%  25%  30%                       │
│                                                          │
│  当前配置：平衡模式（20%）                                │
│                                                          │
│  ├─ NPM 填充：20%                                        │
│  ├─ 流量开销：+20%                                       │
│  ├─ 隐蔽性：高                                           │
│  └─ 月流量成本：+$30（假设 150GB 基础 × $0.10/GB）       │
│                                                          │
│  预设模式：                                               │
│  ○ 经济模式（10%）- 流量受限环境                          │
│  ● 平衡模式（20%）- 日常使用（推荐）                      │
│  ○ 极限模式（30%）- 高风险环境                            │
│                                                          │
│  [应用配置]  [恢复默认]                                   │
└─────────────────────────────────────────────────────────┘
```

**蜂窝级别精细控制**：

```
┌─────────────────────────────────────────────────────────┐
│              蜂窝防御强度配置                             │
├─────────────────────────────────────────────────────────┤
│ 蜂窝 #1 (US-East)：  [━━━━━━━━━━━━━━━━━━━━] 15%         │
│   └─ 月成本：+$22.5 | 120 用户 | 低风险区域              │
│                                                          │
│ 蜂窝 #2 (EU-West)：  [━━━━━━━━━━━━━━━━━━━━] 20%         │
│   └─ 月成本：+$25.5 | 85 用户 | 中风险区域               │
│                                                          │
│ 蜂窝 #3 (ASIA-SG)：  [━━━━━━━━━━━━━━━━━━━━] 30%         │
│   └─ 月成本：+$40.5 | 45 用户 | 高风险区域               │
│                                                          │
│ 总流量成本：+$88.5/月                                     │
│                                                          │
│ [批量应用]  [按风险自动调整]                              │
└─────────────────────────────────────────────────────────┘
```

**手动转生开关**：

```
┌─────────────────────────────────────────────────────────┐
│              紧急转生控制                                 │
├─────────────────────────────────────────────────────────┤
│ 域名：node-b2.example.com                                │
│ 状态：🟡 警告（丢包率 15%）                               │
│                                                          │
│ 转生原因：                                                │
│   ● 手动触发                                              │
│   ○ 丢包率超阈值                                          │
│   ○ 定期轮换                                              │
│                                                          │
│ 新域名池：                                                │
│   ● 从温储备池自动选择                                     │
│   ○ 指定域名：[___________________]                      │
│                                                          │
│ [⚠️  立即转生]  [取消]                                    │
└─────────────────────────────────────────────────────────┘
```

---

### 1.3 情报与死因分析 (Intelligence Center)

**威胁溯源**：

```
┌─────────────────────────────────────────────────────────┐
│              威胁情报中心                                 │
├─────────────────────────────────────────────────────────┤
│ 过去 24 小时拦截：                                        │
│                                                          │
│ 🔴 JA4 指纹扫描：1,247 次                                │
│    └─ 主要来源：AS13335 (Cloudflare)                     │
│                                                          │
│ 🟡 SNI 探测：856 次                                      │
│    └─ 主要来源：AS15169 (Google)                         │
│                                                          │
│ 🟢 主动探测：342 次                                      │
│    └─ 主要来源：AS16509 (AWS)                            │
│                                                          │
│ 🔵 DPI 深度检测：128 次                                  │
│    └─ 主要来源：AS4134 (ChinaNet)                        │
└─────────────────────────────────────────────────────────┘
```

**黑名单分发**：

```
┌─────────────────────────────────────────────────────────┐
│              全局黑名单管理                               │
├─────────────────────────────────────────────────────────┤
│ IP 段黑名单：                                             │
│   ├─ 185.220.0.0/16 (Tor Exit)                          │
│   ├─ 104.16.0.0/12 (Cloudflare)                         │
│   └─ 8.8.8.0/24 (Google DNS)                            │
│                                                          │
│ ASN 黑名单：                                              │
│   ├─ AS13335 (Cloudflare)                               │
│   ├─ AS15169 (Google)                                   │
│   └─ AS4134 (ChinaNet)                                  │
│                                                          │
│ JA4 指纹黑名单：                                          │
│   ├─ t13d1516h2_8daaf6152771_e5627efa2ab1               │
│   └─ t13d1517h2_9daaf6152772_f5627efa2ab2               │
│                                                          │
│ [添加规则]  [批量导入]  [同步到全网]                      │
└─────────────────────────────────────────────────────────┘
```

**死因分析**：

```
┌─────────────────────────────────────────────────────────┐
│              域名死因分析报告                             │
├─────────────────────────────────────────────────────────┤
│ 域名：node-c3.example.com                                │
│ 报废时间：2026-02-04 14:32:18 UTC                        │
│ 存活时长：12 小时 45 分钟                                 │
│                                                          │
│ 死因分类：                                                │
│   ● DNS 污染（概率 85%）                                  │
│   ○ IP 封锁（概率 10%）                                   │
│   ○ 证书吊销（概率 5%）                                   │
│                                                          │
│ 触发特征：                                                │
│   ├─ 丢包率突增：5% → 95% (30 秒内)                      │
│   ├─ RTT 异常：50ms → 5000ms                             │
│   └─ ICMP 不可达：连续 15 次                              │
│                                                          │
│ 建议措施：                                                │
│   ├─ 避开该注册商（Namecheap）                           │
│   ├─ 增加 DNS-less 连接比例                              │
│   └─ 提高域名轮换频率（24h → 12h）                        │
│                                                          │
│ [导出报告]  [应用建议]                                    │
└─────────────────────────────────────────────────────────┘
```

---

## 二、核心功能模块

### 2.1 用户蜂窝管理

```go
// 蜂窝管理器
type CellularManager struct {
    cells map[string]*Cell
    users map[string]*User
}

// 蜂窝定义
type Cell struct {
    ID            string
    Name          string
    Region        string
    SecurityLevel int
    MaxDomains    int
    MaxUsers      int
    Domains       []*Domain
    Users         []*User
    HealthScore   float64
}

// 创建蜂窝
func (cm *CellularManager) CreateCell(name, region string, secLevel int) *Cell {
    cell := &Cell{
        ID:            uuid.New().String(),
        Name:          name,
        Region:        region,
        SecurityLevel: secLevel,
        MaxDomains:    15,
        MaxUsers:      50,
        HealthScore:   1.0,
    }
    
    cm.cells[cell.ID] = cell
    return cell
}

// 分配用户到蜂窝
func (cm *CellularManager) AssignUser(userID, cellID string) error {
    cell := cm.cells[cellID]
    user := cm.users[userID]
    
    if len(cell.Users) >= cell.MaxUsers {
        return errors.New("蜂窝已满")
    }
    
    cell.Users = append(cell.Users, user)
    user.CellID = cellID
    
    return nil
}
```

### 2.2 策略引擎

```go
// 策略引擎
type StrategyEngine struct {
    mcc *MCC
}

// 计算最优策略
func (se *StrategyEngine) ComputeStrategy(user *User) *Strategy {
    // 1. 环境感知
    context := se.detectContext(user)
    
    // 2. 威胁评估
    threatLevel := se.assessThreat(context)
    
    // 3. 策略选择
    return &Strategy{
        MimicryTemplate:  se.selectMimicry(context),
        JurisdictionPath: se.selectPath(context),
        DomainPool:       se.selectDomains(threatLevel),
        PaddingRatio:     se.calculatePadding(threatLevel, user.DefenseLevel),
        Performance:      se.selectCongestionControl(context),
    }
}

// 计算填充比例（考虑用户偏好）
func (se *StrategyEngine) calculatePadding(threatLevel int, defenseLevel string) float64 {
    // 基础填充比例
    basePadding := map[int]float64{
        THREAT_NONE:     0.10,
        THREAT_LOW:      0.15,
        THREAT_MEDIUM:   0.20,
        THREAT_HIGH:     0.25,
        THREAT_CRITICAL: 0.30,
    }[threatLevel]
    
    // 用户偏好调整
    switch defenseLevel {
    case "economy":
        return basePadding * 0.5  // 经济模式：减半
    case "balanced":
        return basePadding        // 平衡模式：默认
    case "extreme":
        return basePadding * 1.5  // 极限模式：1.5 倍
    default:
        return basePadding
    }
}

// 热下发策略
func (se *StrategyEngine) PushStrategy(userID string, strategy *Strategy) error {
    // 1. 查找用户所在蜂窝
    cell := se.mcc.FindCellByUser(userID)
    
    // 2. 推送到该蜂窝的所有 Gateway
    for _, gateway := range cell.Gateways {
        gateway.UpdateStrategy(strategy)
    }
    
    return nil
}
```

### 2.3 成本计算器

```go
// 成本计算器
type CostCalculator struct {
    pricePerGB float64 // $0.10/GB
}

// 计算月流量成本
func (cc *CostCalculator) CalculateMonthlyCost(
    baseTraffic float64,  // GB
    paddingRatio float64, // 0.10 - 0.30
) float64 {
    extraTraffic := baseTraffic * paddingRatio
    return extraTraffic * cc.pricePerGB
}

// 实时成本预估
func (cc *CostCalculator) EstimateCost(user *User) CostEstimate {
    // 1. 获取用户历史流量
    avgMonthlyTraffic := user.GetAvgMonthlyTraffic() // GB
    
    // 2. 获取当前填充比例
    paddingRatio := user.GetPaddingRatio()
    
    // 3. 计算成本
    baseCost := avgMonthlyTraffic * cc.pricePerGB
    extraCost := cc.CalculateMonthlyCost(avgMonthlyTraffic, paddingRatio)
    
    return CostEstimate{
        BaseTraffic:   avgMonthlyTraffic,
        PaddingRatio:  paddingRatio,
        BaseCost:      baseCost,
        ExtraCost:     extraCost,
        TotalCost:     baseCost + extraCost,
        Savings:       cc.calculateSavings(paddingRatio),
    }
}

// 计算节省空间
func (cc *CostCalculator) calculateSavings(currentRatio float64) float64 {
    maxRatio := 0.30
    if currentRatio < maxRatio {
        return (maxRatio - currentRatio) * 100 // 百分比
    }
    return 0
}
```

### 2.3 威胁情报聚合

```go
// 威胁情报聚合器
type ThreatIntelligence struct {
    reports chan *ThreatReport
    db      *Database
}

// 聚合威胁情报
func (ti *ThreatIntelligence) Aggregate() {
    for report := range ti.reports {
        // 1. 存储原始报告
        ti.db.SaveReport(report)
        
        // 2. 更新黑名单
        if report.Severity >= SEVERITY_HIGH {
            ti.updateBlacklist(report)
        }
        
        // 3. 触发自动响应
        if report.Type == THREAT_ACTIVE_PROBING {
            ti.triggerEmergencySwitch(report.CellID)
        }
        
        // 4. 生成情报摘要
        ti.generateIntelligenceSummary()
    }
}

// 分发黑名单到全网
func (ti *ThreatIntelligence) DistributeBlacklist() {
    blacklist := ti.db.GetBlacklist()
    
    // 加密分发到所有 Gateway
    for _, gateway := range ti.getAllGateways() {
        gateway.UpdateBlacklist(blacklist)
    }
}
```

---

## 三、技术架构

### 3.1 后端架构

```
┌─────────────────────────────────────────────────────────┐
│                    Mirage-OS 后端                        │
├─────────────────────────────────────────────────────────┤
│ API 层 (Go + Gin)                                        │
│   ├─ RESTful API                                         │
│   ├─ WebSocket (实时推送)                                │
│   └─ gRPC (Gateway 通信)                                 │
├─────────────────────────────────────────────────────────┤
│ 业务层                                                    │
│   ├─ 蜂窝管理器                                           │
│   ├─ 策略引擎                                             │
│   ├─ 威胁情报聚合                                         │
│   └─ 域名生命周期管理                                     │
├─────────────────────────────────────────────────────────┤
│ 数据层                                                    │
│   ├─ PostgreSQL (用户/蜂窝/域名)                         │
│   ├─ Redis (缓存/会话)                                   │
│   ├─ TimescaleDB (时序数据)                              │
│   └─ IPFS (碎片化存储)                                   │
└─────────────────────────────────────────────────────────┘
```

### 3.2 前端架构

```
┌─────────────────────────────────────────────────────────┐
│                    Mirage-OS 前端                        │
├─────────────────────────────────────────────────────────┤
│ React + TypeScript                                       │
│   ├─ 蜂窝可视化仪表盘                                     │
│   ├─ 策略热下发控制台                                     │
│   ├─ 威胁情报中心                                         │
│   └─ 用户管理                                             │
├─────────────────────────────────────────────────────────┤
│ 实时通信                                                  │
│   ├─ WebSocket (实时态势)                                │
│   └─ Server-Sent Events (日志流)                         │
├─────────────────────────────────────────────────────────┤
│ 可视化                                                    │
│   ├─ ECharts (态势图)                                    │
│   ├─ D3.js (拓扑图)                                      │
│   └─ React Flow (流程图)                                 │
└─────────────────────────────────────────────────────────┘
```

---

## 四、安全设计

### 4.1 访问控制

```go
// 多因素认证
type AuthManager struct {
    mfa *MFAService
}

// 登录流程
func (am *AuthManager) Login(username, password, totpCode string) (*Session, error) {
    // 1. 验证用户名密码
    user := am.verifyCredentials(username, password)
    
    // 2. 验证 TOTP
    if !am.mfa.VerifyTOTP(user.ID, totpCode) {
        return nil, errors.New("TOTP 验证失败")
    }
    
    // 3. 检测环境
    if am.isHeadlessBrowser() {
        return nil, errors.New("检测到自动化工具")
    }
    
    // 4. 创建会话
    return am.createSession(user)
}
```

### 4.2 数据脱敏

```go
// 数据脱敏
func (mcc *MCC) GetDomainList(userID string) []*DomainView {
    domains := mcc.db.GetDomains(userID)
    
    // 脱敏处理
    views := make([]*DomainView, len(domains))
    for i, domain := range domains {
        views[i] = &DomainView{
            ID:     hashID(domain.ID),      // 映射 ID
            Status: domain.Status,
            Health: domain.HealthScore,
            // 不返回真实域名/IP
        }
    }
    
    return views
}
```

### 4.3 审计日志

```go
// 审计日志
type AuditLogger struct {
    db *Database
}

// 记录操作
func (al *AuditLogger) Log(action, userID, details string) {
    log := &AuditLog{
        Timestamp: time.Now(),
        Action:    action,
        UserID:    userID,
        Details:   details,
        IP:        getCurrentIP(),
    }
    
    al.db.SaveAuditLog(log)
}
```

---

## 五、部署架构

```
┌─────────────────────────────────────────────────────────┐
│              Mirage-OS 部署拓扑                          │
├─────────────────────────────────────────────────────────┤
│ 节点 #1: 冰岛（主节点）                                   │
│   ├─ API 服务                                            │
│   ├─ Web 前端                                            │
│   ├─ PostgreSQL (主)                                     │
│   └─ Redis (主)                                          │
├─────────────────────────────────────────────────────────┤
│ 节点 #2: 瑞士（热备）                                     │
│   ├─ API 服务                                            │
│   ├─ PostgreSQL (从)                                     │
│   └─ Redis (从)                                          │
├─────────────────────────────────────────────────────────┤
│ 节点 #3: 新加坡（热备）                                   │
│   ├─ API 服务                                            │
│   ├─ PostgreSQL (从)                                     │
│   └─ Redis (从)                                          │
└─────────────────────────────────────────────────────────┘
         ↓ Raft 一致性 + 自动故障转移
┌─────────────────────────────────────────────────────────┐
│              全球 Gateway 节点                           │
│   ├─ 100+ 边缘节点                                       │
│   └─ 通过 3 跳匿名 + Tor 连接 M.C.C.                     │
└─────────────────────────────────────────────────────────┘
```

---

## 六、商业模型

| 等级 | 月费 | 能力 | Web 功能 |
|------|------|------|---------|
| Escort | $50 | 基础拦截 | 基础仪表盘 |
| Guardian | $200 | 深度指纹库 | 策略热下发 + 威胁情报 |
| Sovereign | $1500+ | 全协议栈混淆 | 完整功能 + 死因分析 + API 访问 |

**目标规模**：200 高净值用户，月营收 $300,000
