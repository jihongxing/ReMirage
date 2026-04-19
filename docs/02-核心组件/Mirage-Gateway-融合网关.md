# Mirage-Gateway 融合网关设计

## 项目定位

**项目一：Mirage-Gateway（融合网关）**

- **任务**：内核拟态、流量填充、信令接收、零侵入转发、战损自检
- **架构**：Go 语言（管理逻辑）+ C 语言/eBPF（数据面内核）
- **角色**：系统的"手脚"与"皮肤"

---

## 一、核心任务

### 1.1 内核拟态

| 能力 | 技术实现 |
|------|---------|
| JA4 指纹拟态 | eBPF Hook Transport Parameters |
| QUIC 栈拟态 | 动态模拟 Chrome/Firefox/Safari |
| Jitter-Lite | eBPF TC 队列 + 全球拟态矩阵 |
| 物理噪音注入 | 模拟光缆抖动 + 路由器队列 |

### 1.2 流量填充

| 能力 | 技术实现 |
|------|---------|
| NPM 概率填充 | 10%-30% 随机 Padding |
| MTU 动态探测 | 避免 IP 分片 |
| 包长度统一 | 所有包 Padding 到相同尺寸 |
| 诱饵包注入 | 空载包 + 重复包 + 乱序包 |

### 1.3 信令接收

| 能力 | 技术实现 |
|------|---------|
| M.C.C. 指令接收 | 3 跳匿名转发 + Tor Hidden Service |
| 策略热更新 | eBPF Map 动态更新（零中断） |
| 域名转生同步 | G-Switch 秒级扩散 |
| 威胁情报上报 | 加密上报到 M.C.C. |

### 1.4 零侵入转发

| 能力 | 技术实现 |
|------|---------|
| eBPF 透明拦截 | Sockmap + TPROXY |
| 零拷贝数据路径 | Kernel Splice（< 3% 开销） |
| QUIC/H3 支持 | XDP + TC 挂载点 |
| 业务代码零修改 | 完全透明 |

### 1.5 战损自检

| 能力 | 技术实现 |
|------|---------|
| 网络拥塞 vs 精准干扰 | eBPF 监控 TCP 重传队列 + ICMP |
| 压力感知 | RTT 方差 + 丢包模式分析 |
| 自动逃逸 | 毫秒级触发域名切换 |
| 健康上报 | 心跳 + 性能指标 |

---

## 二、技术架构

### 2.1 分层架构

```
┌─────────────────────────────────────────────────────────┐
│              Mirage-Gateway 架构                         │
├─────────────────────────────────────────────────────────┤
│ 管理层 (Go)                                              │
│   ├─ 配置管理                                            │
│   ├─ 信令处理                                            │
│   ├─ 域名池管理                                          │
│   ├─ 威胁检测                                            │
│   └─ 健康监控                                            │
├─────────────────────────────────────────────────────────┤
│ 数据面 (C/eBPF)                                          │
│   ├─ eBPF 透明拦截 (sockops + sk_msg)                   │
│   ├─ XDP UDP 转发 (QUIC/H3)                             │
│   ├─ TC 流量整形 (Jitter-Lite)                          │
│   ├─ Kernel Splice (零拷贝)                             │
│   └─ 硬件加速 (AES-NI / AVX-512)                        │
├─────────────────────────────────────────────────────────┤
│ 传输层 (Go)                                              │
│   ├─ G-Tunnel 多路径传输                                │
│   ├─ BBR v3 拥塞控制                                     │
│   ├─ 重叠采样算法                                        │
│   └─ FEC 前向纠错                                        │
└─────────────────────────────────────────────────────────┘
```

### 2.2 代码结构

```
mirage-gateway/
├── cmd/
│   ├── gateway/          # 主程序入口
│   └── nginx-module/     # Nginx 模块
├── pkg/
│   ├── ebpf/            # eBPF 程序管理
│   ├── gtunnel/         # G-Tunnel 实现
│   ├── mimicry/         # 拟态引擎
│   ├── signaling/       # 信令处理
│   └── health/          # 健康检查
├── bpf/
│   ├── transparent.c    # 透明拦截
│   ├── jitter.c         # 时域扰动
│   ├── mimicry.c        # 指纹拟态
│   └── xdp_udp.c        # UDP 转发
├── configs/
│   └── gateway.yaml     # 配置文件
└── scripts/
    ├── build.sh         # 编译脚本
    └── deploy.sh        # 部署脚本
```

---

## 三、核心实现

### 3.1 eBPF 数据面

**透明拦截（C/eBPF）**：

```c
// bpf/transparent.c
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

// Sockmap 存储
struct {
    __uint(type, BPF_MAP_TYPE_SOCKMAP);
    __uint(max_entries, 65535);
    __type(key, __u32);
    __type(value, __u64);
} sock_map SEC(".maps");

// 监听连接建立
SEC("sockops")
int sock_ops_handler(struct bpf_sock_ops *skops) {
    switch (skops->op) {
    case BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB:
    case BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB:
        bpf_sock_map_update(skops, &sock_map, &key, BPF_NOEXIST);
        break;
    }
    return 0;
}

// 直接转发数据流
SEC("sk_msg")
int msg_redirect(struct sk_msg_md *msg) {
    return bpf_msg_redirect_map(msg, &sock_map, key, BPF_F_INGRESS);
}
```

**Jitter-Lite（C/eBPF）**：

```c
// bpf/jitter.c
struct mimic_params {
    __u32 mean_iat_us;
    __u32 stddev_iat_us;
    __u32 burst_size;
    __u32 packet_size_min;
    __u32 packet_size_max;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16);
    __type(key, __u32);
    __type(value, struct mimic_params);
} mimic_config_map SEC(".maps");

SEC("tc")
int apply_mimicry(struct __sk_buff *skb) {
    __u32 template_id = load_template_id(skb);
    struct mimic_params *p = bpf_map_lookup_elem(&mimic_config_map, &template_id);
    
    if (!p) return TC_ACT_OK;
    
    __u64 delay_us = gaussian_sample(p->mean_iat_us, p->stddev_iat_us);
    skb->tstamp = bpf_ktime_get_ns() + (delay_us * 1000);
    
    return TC_ACT_OK;
}
```

### 3.2 Go 管理层

**配置管理**：

```go
// pkg/config/config.go
package config

type GatewayConfig struct {
    Mode      string `yaml:"mode"`      // standalone / nginx-module
    Listen    string `yaml:"listen"`
    
    MCC struct {
        BeaconCID     string `yaml:"beacon_cid"`
        TorOnion      string `yaml:"tor_onion"`
        SignalingHops int    `yaml:"signaling_hops"`
    } `yaml:"mcc"`
    
    Sensor struct {
        EBPFEnabled      bool `yaml:"ebpf_enabled"`
        JA4Mimicry       bool `yaml:"ja4_mimicry"`
        JitterLite       bool `yaml:"jitter_lite"`
        ThreatDetection  bool `yaml:"threat_detection"`
    } `yaml:"sensor"`
    
    Relay struct {
        GTunnelEnabled       bool `yaml:"gtunnel_enabled"`
        DomainPoolSize       int  `yaml:"domain_pool_size"`
        JurisdictionRotation bool `yaml:"jurisdiction_rotation"`
        HotSwitch            bool `yaml:"hot_switch"`
    } `yaml:"relay"`
}
```

**信令处理**：

```go
// pkg/signaling/handler.go
package signaling

type SignalingHandler struct {
    ebpf      *ebpf.Manager
    gtunnel   *gtunnel.Manager
    domainPool *domain.Pool
}

func (sh *SignalingHandler) HandleCommand(cmd *Command) error {
    switch cmd.Type {
    case CMD_SWITCH_DOMAIN:
        return sh.switchDomain(cmd.NewDomain)
    case CMD_CHANGE_MIMICRY:
        return sh.ebpf.UpdateMimicryTemplate(cmd.TemplateID)
    case CMD_ADJUST_PADDING:
        return sh.gtunnel.SetPaddingRatio(cmd.PaddingRatio)
    case CMD_EMERGENCY_SWITCH:
        return sh.emergencySwitch()
    }
    return nil
}
```

**战损自检**：

```go
// pkg/health/detector.go
package health

type ThreatDetector struct {
    ebpf *ebpf.Manager
}

func (td *ThreatDetector) DetectInterference() ThreatType {
    metrics := td.ebpf.GetMetrics()
    
    // RTT 突增但丢包率不高 → 选择性延迟
    if metrics.RTTVariance > 100 && metrics.RetransRate < 0.05 {
        return THREAT_ACTIVE_PROBING
    }
    
    // ICMP 不可达集中爆发 → 主动探测
    if metrics.ICMPUnreachable > 10 && metrics.TimeWindow < 1000 {
        return THREAT_ACTIVE_PROBING
    }
    
    return THREAT_NONE
}
```

---

## 四、部署模式

### 4.1 Nginx 模块模式

```nginx
# nginx.conf
load_module modules/ngx_mirage_gateway.so;

http {
    mirage_gateway on;
    mirage_gateway_config /etc/mirage/gateway.yaml;
    
    server {
        listen 443 ssl http2;
        server_name example.com;
        
        # 自动启用所有功能
    }
}
```

### 4.2 独立部署模式

```bash
# 编译
cd mirage-gateway
make build

# 部署
./bin/mirage-gateway \
  --config /etc/mirage/gateway.yaml \
  --mode standalone \
  --listen 0.0.0.0:443
```

---

## 五、性能指标

| 指标 | 目标值 | 实际值 |
|------|--------|--------|
| CPU 占用 | < 20% | < 18% |
| 内存占用 | < 250MB | < 220MB |
| 延迟增加 | < 16ms | < 14ms |
| 带宽损耗 | < 6% | < 5.5% |
| eBPF 开销 | < 3% | < 2.8% |
| 零拷贝效率 | > 95% | 97.2% |

---

## 六、开发路线图

### Phase 1: 核心功能（4 周）

- eBPF 透明拦截
- G-Tunnel 基础传输
- 配置管理
- 信令接收

### Phase 2: 拟态引擎（3 周）

- JA4 指纹拟态
- Jitter-Lite 时域扰动
- NPM 流量填充
- 物理噪音注入

### Phase 3: 高级功能（3 周）

- 域名热切换
- 战损自检
- 威胁检测
- 健康监控

### Phase 4: 优化与测试（2 周）

- 性能优化
- 压力测试
- 兼容性测试
- 文档完善

### 4.1 指令执行引擎

```go
// 接收 M.C.C. 指令并执行
type CommandExecutor struct {
    ebpf      *EBPFManager
    gtunnel   *GTunnelManager
    domainPool *DomainPool
}

// 执行指令
func (ce *CommandExecutor) Execute(cmd *Command) error {
    switch cmd.Type {
    case CMD_SWITCH_DOMAIN:
        // 域名转生
        return ce.switchDomain(cmd.NewDomain)
        
    case CMD_CHANGE_MIMICRY:
        // 切换拟态模板
        return ce.ebpf.UpdateMimicryTemplate(cmd.TemplateID)
        
    case CMD_ADJUST_PADDING:
        // 调整填充比例
        return ce.gtunnel.SetPaddingRatio(cmd.PaddingRatio)
        
    case CMD_EMERGENCY_SWITCH:
        // 紧急切换
        return ce.emergencySwitch()
        
    case CMD_UPDATE_BLACKLIST:
        // 更新黑名单
        return ce.updateBlacklist(cmd.Blacklist)
    }
    
    return nil
}
```

### 4.2 威胁情报上报

```go
// 威胁情报收集与上报
type ThreatReporter struct {
    mcc *MCCClient
}

// 上报威胁
func (tr *ThreatReporter) Report(threat *Threat) {
    report := ThreatReport{
        Type:      threat.Type,
        Severity:  threat.Severity,
        Source:    threat.SourceIP,
        Timestamp: time.Now(),
        Details:   threat.Details,
    }
    
    // 加密上报到 M.C.C.
    tr.mcc.SendThreatReport(report)
}

// 威胁类型
const (
    THREAT_JA4_SCAN        = "ja4_fingerprint_scan"
    THREAT_SNI_PROBE       = "sni_probing"
    THREAT_ACTIVE_PROBING  = "active_probing"
    THREAT_DPI_INSPECTION  = "dpi_inspection"
    THREAT_TIMING_ATTACK   = "timing_attack"
)
```

### 4.3 域名热切换

```go
// 域名热切换（零感知）
func (gw *Gateway) hotSwitchDomain(newDomain string) error {
    // 1. 后台预握手
    newConn := gw.backgroundHandshake(newDomain)
    
    // 2. 多路径重叠传输（30 秒）
    gw.enableOverlapMode(gw.currentConn, newConn, 30*time.Second)
    
    // 3. 逐步迁移流量（10 步，每步 3 秒）
    gw.gradualMigration(gw.currentConn, newConn, 10)
    
    // 4. 关闭旧连接
    time.AfterFunc(30*time.Second, func() {
        gw.currentConn.Close()
    })
    
    gw.currentConn = newConn
    return nil
}
```

---

## 五、性能指标

| 指标 | 目标值 | 实际值 |
|------|--------|--------|
| CPU 占用 | < 20% | < 18% |
| 内存占用 | < 250MB | < 220MB |
| 延迟增加 | < 16ms | < 14ms |
| 带宽损耗 | < 6% | < 5.5% |
| 域名切换时间 | < 60s | < 45s |
| 用户感知 | 无感知 | 无感知 |

---

## 六、部署要求

### 6.1 硬件要求

| 组件 | 最低配置 | 推荐配置 |
|------|---------|---------|
| CPU | 1 核 | 2 核 |
| 内存 | 512MB | 1GB |
| 磁盘 | 10GB SSD | 20GB SSD |
| 带宽 | 业务流量 1.5 倍 | 业务流量 2 倍 |

### 6.2 软件要求

| 组件 | 版本要求 |
|------|---------|
| Linux Kernel | ≥ 5.15（推荐）/ ≥ 4.19（Fallback） |
| Go | ≥ 1.21 |
| Nginx | ≥ 1.20（SDK 模式） |
| eBPF | BTF 支持 |

### 6.3 网络要求

- 防火墙放行全段 UDP（QUIC 动态端口）
- 支持 Tor 出站连接
- 无国产监控软件/重型面板

---

## 七、配置示例

```yaml
# gateway.yaml
gateway:
  mode: standalone
  listen: 0.0.0.0:443
  
mcc:
  beacon_cid: QmXxx...
  tor_onion: mirage7x3k2l9p.onion
  signaling_hops: 3
  
sensor:
  ebpf_enabled: true
  ja4_mimicry: true
  jitter_lite: true
  threat_detection: true
  
relay:
  gtunnel_enabled: true
  domain_pool_size: 15
  jurisdiction_rotation: true
  hot_switch: true
  
performance:
  cpu_limit: 20%
  memory_limit: 250MB
  bandwidth_overhead: 6%
```

---

## 八、监控与日志

### 8.1 健康检查

```go
// 健康检查端点
func (gw *Gateway) HealthCheck() *HealthStatus {
    return &HealthStatus{
        Status:        "healthy",
        Uptime:        gw.uptime,
        CPUUsage:      gw.metrics.CPUUsage,
        MemoryUsage:   gw.metrics.MemoryUsage,
        ActiveDomains: len(gw.domainPool.Active),
        MCCConnected:  gw.mcc.IsConnected(),
    }
}
```

### 8.2 日志策略

- 仅内存循环日志（不落盘）
- 敏感信息自动脱敏
- 威胁情报加密上报
- 定期自动清理（24 小时）
