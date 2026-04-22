---
Status: derived
Target Truth: docs/protocols/jitter-lite.md
Migration: 当前有效协议语义已迁移到 docs/protocols/jitter-lite.md，本文降级为解释性输入材料
---

# Jitter-Lite 时域扰动协议

## 一、协议定位

**时间维度防御**：对抗 AI 统计分析（IAT），解决数据包到达间隔特征识别

- **技术栈**：C (eBPF TC) 数据面 + Go 控制面
- **核心能力**：全球拟态矩阵 + 流量自适应 + 物理噪音注入
- **语言分工**：C 负责控制 skb->tstamp 实现绝对精准 IAT，Go 负责模板选择与流量分类

---

## 二、协议架构

### 2.1 分层设计

```
┌─────────────────────────────────────────────────────────┐
│                  Jitter-Lite 协议栈                      │
├─────────────────────────────────────────────────────────┤
│ 策略层 (Go)                                              │
│   ├─ 拟态模板选择                                        │
│   ├─ 业务流识别                                          │
│   └─ 环境感知                                            │
├─────────────────────────────────────────────────────────┤
│ 执行层 (eBPF)                                            │
│   ├─ TC 队列延迟注入                                     │
│   ├─ 高斯分布采样                                        │
│   ├─ 物理噪音模拟                                        │
│   └─ 硬件 FQ 调度                                        │
├─────────────────────────────────────────────────────────┤
│ 内核层                                                    │
│   ├─ skb->tstamp 设置                                    │
│   ├─ bpf_ktime_get_ns()                                  │
│   └─ FQ (Fair Queuing) 调度器                           │
└─────────────────────────────────────────────────────────┘
```

---

## 三、全球拟态矩阵

### 3.1 模板定义

```go
// 拟态模板
type MimicryTemplate struct {
    ID          string
    Name        string
    Target      string  // 模拟对象
    MeanIAT     uint32  // 平均包间隔（微秒）
    StddevIAT   uint32  // 标准差
    BurstSize   uint32  // 突发包数量
    PacketSizeMin uint32
    PacketSizeMax uint32
}

// 全球拟态矩阵
var GlobalMimicryMatrix = map[string]MimicryTemplate{
    "Conference-Pro": {
        ID:            "conference_pro",
        Name:          "Conference-Pro",
        Target:        "Zoom / Teams",
        MeanIAT:       20000,  // 20ms
        StddevIAT:     5000,   // 5ms
        BurstSize:     1,
        PacketSizeMin: 100,
        PacketSizeMax: 200,
    },
    "Cinema-Ultra": {
        ID:            "cinema_ultra",
        Name:          "Cinema-Ultra",
        Target:        "Netflix / Disney+",
        MeanIAT:       50000,  // 50ms
        StddevIAT:     15000,  // 15ms
        BurstSize:     50,
        PacketSizeMin: 1200,
        PacketSizeMax: 1400,
    },
    "Social-Pulse": {
        ID:            "social_pulse",
        Name:          "Social-Pulse",
        Target:        "WhatsApp / Meta",
        MeanIAT:       100000, // 100ms
        StddevIAT:     30000,  // 30ms
        BurstSize:     3,
        PacketSizeMin: 50,
        PacketSizeMax: 150,
    },
    "Gamer-Zero": {
        ID:            "gamer_zero",
        Name:          "Gamer-Zero",
        Target:        "Steam / FPS Games",
        MeanIAT:       500,    // 0.5ms
        StddevIAT:     200,    // 0.2ms
        BurstSize:     1,
        PacketSizeMin: 80,
        PacketSizeMax: 120,
    },
    "Stream-Live": {
        ID:            "stream_live",
        Name:          "Stream-Live",
        Target:        "Twitch / YouTube Live",
        MeanIAT:       30000,  // 30ms
        StddevIAT:     10000,  // 10ms
        BurstSize:     20,
        PacketSizeMin: 800,
        PacketSizeMax: 1200,
    },
    "VoIP-Stable": {
        ID:            "voip_stable",
        Name:          "VoIP-Stable",
        Target:        "Skype / Discord",
        MeanIAT:       20000,  // 20ms
        StddevIAT:     2000,   // 2ms
        BurstSize:     1,
        PacketSizeMin: 160,
        PacketSizeMax: 160,
    },
}
```

### 3.2 模板选择策略

```go
type TemplateSelector struct {
    context *Context
}

func (ts *TemplateSelector) Select(user *User) string {
    location := user.GeoLocation
    hour := time.Now().Hour()
    
    // 地理位置 + 时间段
    switch {
    case location == "US" && hour >= 9 && hour <= 17:
        return "Conference-Pro" // 办公时间 → Zoom
    case location == "EU" && hour >= 20 && hour <= 23:
        return "Cinema-Ultra"   // 晚间 → Netflix
    case location == "ASIA" && user.IsMobile:
        return "Social-Pulse"   // 移动端 → WhatsApp
    case user.IsGaming:
        return "Gamer-Zero"     // 游戏 → Steam
    default:
        return "Conference-Pro"
    }
}
```

---

## 四、核心算法

### 4.1 高斯分布采样

**Box-Muller 变换**：

```c
// eBPF 实现
static __always_inline __u64 gaussian_sample(__u32 mean, __u32 stddev) {
    // 1. 生成两个均匀分布随机数
    __u32 u1 = bpf_get_prandom_u32();
    __u32 u2 = bpf_get_prandom_u32();
    
    // 2. 归一化到 [0, 1]
    double u1_norm = (double)u1 / UINT32_MAX;
    double u2_norm = (double)u2 / UINT32_MAX;
    
    // 3. Box-Muller 变换
    double z = sqrt(-2.0 * log(u1_norm)) * cos(2.0 * M_PI * u2_norm);
    
    // 4. 缩放到目标分布
    __u64 result = (__u64)(mean + z * stddev);
    
    return result;
}
```

### 4.2 物理噪音注入

**模拟光缆抖动 + 路由器队列**：

```c
// 模拟光缆物理抖动（0.1ms - 2ms）
static __always_inline __u64 simulate_fiber_jitter() {
    // 基于泊松分布的微小抖动
    __u32 random = bpf_get_prandom_u32();
    return (random % 2000) + 100; // 0.1ms - 2ms
}

// 模拟路由器队列延迟（0 - 10ms）
static __always_inline __u64 simulate_router_queue() {
    // 基于当前网络负载的动态延迟
    __u32 load = get_network_load();
    return (load * 10000) / 100; // 负载越高延迟越大
}

// 综合延迟计算
static __always_inline __u64 calculate_total_delay(
    struct mimic_params *p
) {
    // 1. 基础延迟（高斯分布）
    __u64 base_delay = gaussian_sample(p->mean_iat_us, p->stddev_iat_us);
    
    // 2. 物理噪音
    __u64 physical_noise = simulate_fiber_jitter();
    
    // 3. 路由器队列
    __u64 queue_delay = simulate_router_queue();
    
    // 4. 总延迟
    return base_delay + physical_noise + queue_delay;
}
```

---

## 五、eBPF 实现

### 5.1 数据结构

```c
// bpf/jitter.h
struct mimic_params {
    __u32 mean_iat_us;      // 平均包间隔（微秒）
    __u32 stddev_iat_us;    // 标准差
    __u32 burst_size;       // 突发包数量
    __u32 packet_size_min;  // 最小包长
    __u32 packet_size_max;  // 最大包长
    __u32 physical_noise;   // 是否启用物理噪音
    __u32 router_queue;     // 是否启用路由器队列模拟
};

// BPF Map：存储拟态模板参数
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16);
    __type(key, __u32);   // 模板 ID
    __type(value, struct mimic_params);
} mimic_config_map SEC(".maps");

// BPF Map：存储流量类型映射
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u64);   // 流标识（src_ip:src_port:dst_ip:dst_port）
    __type(value, __u32); // 模板 ID
} flow_template_map SEC(".maps");
```

### 5.2 TC 程序

```c
// bpf/jitter.c
SEC("tc")
int apply_jitter(struct __sk_buff *skb) {
    // 1. 提取流标识
    __u64 flow_id = extract_flow_id(skb);
    
    // 2. 查询流对应的模板
    __u32 *template_id = bpf_map_lookup_elem(&flow_template_map, &flow_id);
    if (!template_id) {
        return TC_ACT_OK; // 无拟态
    }
    
    // 3. 查询模板参数
    struct mimic_params *p = bpf_map_lookup_elem(&mimic_config_map, template_id);
    if (!p) {
        return TC_ACT_OK;
    }
    
    // 4. 计算延迟
    __u64 delay_us = calculate_total_delay(p);
    
    // 5. 设置 skb 时间戳（硬件级定时）
    skb->tstamp = bpf_ktime_get_ns() + (delay_us * 1000);
    
    // 6. 调整包长（Padding）
    adjust_packet_size(skb, p->packet_size_min, p->packet_size_max);
    
    return TC_ACT_OK;
}

// 提取流标识
static __always_inline __u64 extract_flow_id(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return 0;
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return 0;
    
    // 组合流标识
    __u64 flow_id = 0;
    flow_id |= (__u64)ip->saddr << 32;
    flow_id |= (__u64)ip->daddr;
    
    return flow_id;
}
```

---

## 六、流量自适应

### 6.1 业务流识别

```go
type TrafficClassifier struct {
    patterns map[string]*Pattern
}

func (tc *TrafficClassifier) Classify(flow *Flow) string {
    // 1. 深度包检测（DPI）
    if flow.HasVideoCodec() {
        if flow.UploadRatio > 0.3 {
            return "Stream-Live" // 直播推流
        }
        return "Cinema-Ultra" // 视频点播
    }
    
    // 2. 端口特征
    if flow.DstPort == 443 && flow.SNI == "github.com" {
        return "SSH-Like" // 代码开发
    }
    
    // 3. 流量模式
    if flow.IsSymmetric() && flow.AvgPacketSize < 200 {
        return "VoIP-Stable" // 语音通话
    }
    
    // 4. 时序特征
    if flow.HasBurstPattern() && flow.PacketRate > 100 {
        return "Gamer-Zero" // 游戏
    }
    
    // 5. 默认
    return "Conference-Pro"
}
```

### 6.2 自动切换

```go
type JitterManager struct {
    ebpf       *ebpf.Manager
    classifier *TrafficClassifier
}

func (jm *JitterManager) AutoSwitch(flow *Flow) {
    // 1. 识别业务类型
    businessType := jm.classifier.Classify(flow)
    
    // 2. 获取模板 ID
    template := GlobalMimicryMatrix[businessType]
    
    // 3. 更新 eBPF Map（零中断）
    flowID := flow.ID()
    jm.ebpf.UpdateMap("flow_template_map", flowID, template.ID)
    
    // 4. 日志记录
    log.Printf("Flow %s switched to template %s", flowID, template.Name)
}
```

---

## 七、性能优化

### 7.1 硬件 FQ 调度

```c
// 利用内核 FQ (Fair Queuing) 调度器
SEC("tc")
int hardware_timing(struct __sk_buff *skb) {
    // 1. 获取纳秒级时间戳
    __u64 now_ns = bpf_ktime_get_ns();
    
    // 2. 计算目标发送时间
    __u64 delay_ns = calculate_jitter_ns(skb);
    __u64 target_ns = now_ns + delay_ns;
    
    // 3. 设置 skb->tstamp（网卡硬件负责延迟）
    skb->tstamp = target_ns;
    
    // 4. 交给 FQ 调度器（零 CPU 开销）
    return TC_ACT_OK;
}
```

### 7.2 批处理优化

```go
type BatchProcessor struct {
    batchSize int
    queue     chan *Packet
}

func (bp *BatchProcessor) Process() {
    batch := make([]*Packet, 0, bp.batchSize)
    
    for packet := range bp.queue {
        batch = append(batch, packet)
        
        if len(batch) >= bp.batchSize {
            // 批量处理
            bp.processBatch(batch)
            batch = batch[:0]
        }
    }
}

func (bp *BatchProcessor) processBatch(batch []*Packet) {
    // 1. 批量查询模板
    templates := bp.batchLookupTemplates(batch)
    
    // 2. 批量计算延迟
    delays := bp.batchCalculateDelays(templates)
    
    // 3. 批量更新 eBPF Map
    bp.batchUpdateMaps(batch, delays)
}
```

---

## 八、性能指标

### 8.1 延迟精度

| 实现方式 | 延迟精度 | CPU 开销 | 吞吐影响 |
|---------|---------|---------|---------|
| 用户态 sleep | 毫秒级 | 5%-8% | -15% |
| Go time.Sleep | 毫秒级 | 3%-5% | -10% |
| eBPF TC 队列 | 纳秒级 | < 1% | -0.5% |
| 硬件 FQ 调度 | 纳秒级 | < 0.1% | -0.1% |

### 8.2 拟态效果

| 模板 | IAT 方差 | 包长方差 | 识别难度 |
|------|---------|---------|---------|
| Conference-Pro | 5ms | 50 字节 | 极高 |
| Cinema-Ultra | 15ms | 200 字节 | 极高 |
| Gamer-Zero | 0.2ms | 20 字节 | 高 |

---

## 九、配置示例

```yaml
jitter_lite:
  # 全局开关
  enabled: true
  
  # 默认模板
  default_template: Conference-Pro
  
  # 自动切换
  auto_switch:
    enabled: true
    classifier: dpi_based
  
  # 物理噪音
  physical_noise:
    enabled: true
    fiber_jitter: true
    router_queue: true
  
  # 性能
  performance:
    hardware_fq: true
    batch_size: 64
    ebpf_offload: true
  
  # 模板覆盖
  templates:
    Conference-Pro:
      mean_iat_us: 20000
      stddev_iat_us: 5000
    Cinema-Ultra:
      mean_iat_us: 50000
      stddev_iat_us: 15000
```

---

## 十、实现参考

```go
// pkg/jitter/manager.go
package jitter

type Manager struct {
    ebpf       *ebpf.Manager
    classifier *TrafficClassifier
    templates  map[string]MimicryTemplate
}

func NewManager() *Manager {
    return &Manager{
        ebpf:       ebpf.NewManager(),
        classifier: NewTrafficClassifier(),
        templates:  GlobalMimicryMatrix,
    }
}

func (m *Manager) Start() error {
    // 1. 加载 eBPF 程序
    if err := m.ebpf.LoadProgram("jitter.o"); err != nil {
        return err
    }
    
    // 2. 挂载到 TC
    if err := m.ebpf.AttachTC("eth0", "egress"); err != nil {
        return err
    }
    
    // 3. 初始化模板
    for id, template := range m.templates {
        m.ebpf.UpdateMap("mimic_config_map", id, template)
    }
    
    return nil
}

func (m *Manager) UpdateTemplate(flowID uint64, templateName string) error {
    template := m.templates[templateName]
    return m.ebpf.UpdateMap("flow_template_map", flowID, template.ID)
}
```
