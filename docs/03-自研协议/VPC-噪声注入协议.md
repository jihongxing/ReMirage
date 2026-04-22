---
Status: derived
Target Truth: docs/protocols/vpc.md
Migration: 当前有效协议语义已迁移到 docs/protocols/vpc.md，本文降级为解释性输入材料
---

# VPC 噪声注入协议

## 一、协议定位

**背景维度防御**：物理链路背景噪音注入，解决用户行为画像建立

- **技术栈**：C (eBPF TC) 数据面 + Go 控制面
- **核心能力**：光缆抖动模拟 + 路由器队列模拟 + 威胁等级自适应
- **语言分工**：C 负责纳秒级定时器控制 skb->tstamp，Go 负责威胁检测与策略调整

---

## 二、协议架构

### 2.1 分层设计

```
┌─────────────────────────────────────────────────────────┐
│                    VPC 协议栈                            │
├─────────────────────────────────────────────────────────┤
│ 感知层 (Go)                                              │
│   ├─ TCP 重传队列监控                                    │
│   ├─ 威胁等级评估                                        │
│   └─ 自适应策略                                          │
├─────────────────────────────────────────────────────────┤
│ 决策层                                                    │
│   ├─ 噪声强度计算                                        │
│   ├─ 注入时机选择                                        │
│   └─ 流量分配                                            │
├─────────────────────────────────────────────────────────┤
│ 执行层 (eBPF)                                            │
│   ├─ 光缆抖动注入                                        │
│   ├─ 路由器队列模拟                                      │
│   ├─ 虚假重传                                            │
│   └─ 噪声流生成                                          │
└─────────────────────────────────────────────────────────┘
```

---

## 三、核心机制

### 3.1 威胁感知

```go
type ThreatDetector struct {
    tcpMonitor *TCPMonitor
    thresholds *ThreatThresholds
}

type ThreatThresholds struct {
    // TCP 重传队列阈值
    RetransmitRate float64 // 重传率：5%
    
    // RTT 方差阈值
    RTTVariance time.Duration // 100ms
    
    // ICMP 异常阈值
    ICMPCount int // 10 次/分钟
    
    // 丢包模式
    SelectiveLoss bool // 选择性丢包检测
}

// 威胁等级
type ThreatLevel int

const (
    THREAT_NONE     ThreatLevel = 0 // 无威胁
    THREAT_LOW      ThreatLevel = 1 // 低威胁：网络拥塞
    THREAT_MEDIUM   ThreatLevel = 2 // 中威胁：可疑探测
    THREAT_HIGH     ThreatLevel = 3 // 高威胁：精准干扰
    THREAT_CRITICAL ThreatLevel = 4 // 极高威胁：主动攻击
)

// 检测威胁等级
func (td *ThreatDetector) Detect(conn *Connection) ThreatLevel {
    metrics := td.tcpMonitor.GetMetrics(conn)
    
    // 1. 检查重传率
    if metrics.RetransmitRate > td.thresholds.RetransmitRate {
        // 2. 区分拥塞 vs 干扰
        if td.isNetworkCongestion(metrics) {
            return THREAT_LOW // 网络拥塞
        }
        
        // 3. 检查选择性丢包
        if td.isSelectiveLoss(metrics) {
            return THREAT_HIGH // 精准干扰
        }
        
        return THREAT_MEDIUM // 可疑探测
    }
    
    // 4. 检查 ICMP 异常
    if metrics.ICMPCount > td.thresholds.ICMPCount {
        return THREAT_CRITICAL // 主动攻击
    }
    
    return THREAT_NONE
}

// 判断是否为网络拥塞
func (td *ThreatDetector) isNetworkCongestion(m *Metrics) bool {
    // 拥塞特征：
    // 1. RTT 持续上升
    // 2. 丢包均匀分布
    // 3. 所有连接受影响
    
    return m.RTTTrend > 0 && 
           m.LossDistribution == "uniform" &&
           m.AffectedConnRatio > 0.8
}

// 判断是否为选择性丢包
func (td *ThreatDetector) isSelectiveLoss(m *Metrics) bool {
    // 精准干扰特征：
    // 1. RTT 稳定
    // 2. 丢包集中在特定端口/协议
    // 3. 仅部分连接受影响
    
    return m.RTTVariance < td.thresholds.RTTVariance &&
           m.LossDistribution == "selective" &&
           m.AffectedConnRatio < 0.3
}
```

### 3.2 eBPF 监控

```c
// bpf/vpc_monitor.c
struct tcp_metrics {
    __u64 retransmit_count;
    __u64 total_packets;
    __u64 rtt_sum;
    __u64 rtt_count;
    __u64 icmp_count;
};

// BPF Map：存储 TCP 指标
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u64);   // 连接标识
    __type(value, struct tcp_metrics);
} tcp_metrics_map SEC(".maps");

// 监控 TCP 重传
SEC("kprobe/tcp_retransmit_skb")
int monitor_tcp_retransmit(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    
    // 1. 提取连接标识
    __u64 conn_id = extract_conn_id(sk);
    
    // 2. 更新重传计数
    struct tcp_metrics *metrics = bpf_map_lookup_elem(&tcp_metrics_map, &conn_id);
    if (metrics) {
        __sync_fetch_and_add(&metrics->retransmit_count, 1);
    }
    
    return 0;
}

// 监控 RTT
SEC("kprobe/tcp_ack")
int monitor_tcp_rtt(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct tcp_sock *tp = tcp_sk(sk);
    
    __u64 conn_id = extract_conn_id(sk);
    __u32 rtt = tp->srtt_us >> 3; // 平滑 RTT
    
    struct tcp_metrics *metrics = bpf_map_lookup_elem(&tcp_metrics_map, &conn_id);
    if (metrics) {
        __sync_fetch_and_add(&metrics->rtt_sum, rtt);
        __sync_fetch_and_add(&metrics->rtt_count, 1);
    }
    
    return 0;
}

// 监控 ICMP
SEC("xdp")
int monitor_icmp(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    
    struct iphdr *ip = data + sizeof(struct ethhdr);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;
    
    if (ip->protocol == IPPROTO_ICMP) {
        struct icmphdr *icmp = (void *)(ip + 1);
        if ((void *)(icmp + 1) > data_end)
            return XDP_PASS;
        
        // ICMP Destination Unreachable
        if (icmp->type == 3) {
            __u64 conn_id = extract_conn_id_from_icmp(icmp);
            
            struct tcp_metrics *metrics = bpf_map_lookup_elem(&tcp_metrics_map, &conn_id);
            if (metrics) {
                __sync_fetch_and_add(&metrics->icmp_count, 1);
            }
        }
    }
    
    return XDP_PASS;
}
```

---

## 四、噪声注入

### 4.1 光缆抖动模拟

```go
type FiberJitterSimulator struct {
    baseJitter    time.Duration // 基础抖动：0.1ms
    maxJitter     time.Duration // 最大抖动：2ms
    distribution  string        // 分布类型：poisson
}

// 计算光缆抖动
func (fjs *FiberJitterSimulator) Calculate() time.Duration {
    switch fjs.distribution {
    case "poisson":
        return fjs.poissonJitter()
    case "gaussian":
        return fjs.gaussianJitter()
    case "uniform":
        return fjs.uniformJitter()
    default:
        return fjs.poissonJitter()
    }
}

// 泊松分布抖动（模拟光子到达）
func (fjs *FiberJitterSimulator) poissonJitter() time.Duration {
    lambda := float64(fjs.maxJitter) / 2
    jitter := -lambda * math.Log(1-rand.Float64())
    
    if jitter < float64(fjs.baseJitter) {
        jitter = float64(fjs.baseJitter)
    }
    if jitter > float64(fjs.maxJitter) {
        jitter = float64(fjs.maxJitter)
    }
    
    return time.Duration(jitter)
}

// 高斯分布抖动
func (fjs *FiberJitterSimulator) gaussianJitter() time.Duration {
    mean := float64(fjs.maxJitter) / 2
    stddev := float64(fjs.maxJitter) / 6
    
    jitter := rand.NormFloat64()*stddev + mean
    
    if jitter < float64(fjs.baseJitter) {
        jitter = float64(fjs.baseJitter)
    }
    if jitter > float64(fjs.maxJitter) {
        jitter = float64(fjs.maxJitter)
    }
    
    return time.Duration(jitter)
}
```

### 4.2 路由器队列模拟

```go
type RouterQueueSimulator struct {
    minDelay time.Duration // 最小延迟：0ms
    maxDelay time.Duration // 最大延迟：10ms
    loadFactor float64     // 负载因子：0.0-1.0
}

// 计算队列延迟
func (rqs *RouterQueueSimulator) Calculate(threatLevel ThreatLevel) time.Duration {
    // 1. 基于威胁等级调整负载因子
    adjustedLoad := rqs.loadFactor
    
    switch threatLevel {
    case THREAT_NONE:
        adjustedLoad *= 0.3 // 低负载
    case THREAT_LOW:
        adjustedLoad *= 0.5
    case THREAT_MEDIUM:
        adjustedLoad *= 0.7
    case THREAT_HIGH:
        adjustedLoad *= 0.9
    case THREAT_CRITICAL:
        adjustedLoad = 1.0 // 满负载
    }
    
    // 2. 计算延迟（负载越高延迟越大）
    delay := float64(rqs.minDelay) + 
             adjustedLoad * float64(rqs.maxDelay-rqs.minDelay)
    
    return time.Duration(delay)
}
```

### 4.3 eBPF 注入

```c
// bpf/vpc_inject.c
struct noise_config {
    __u32 fiber_jitter_us;   // 光缆抖动（微秒）
    __u32 router_delay_us;   // 路由器延迟（微秒）
    __u32 threat_level;      // 威胁等级
    __u32 noise_intensity;   // 噪声强度（0-100）
};

// BPF Map：存储噪声配置
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u64);   // 连接标识
    __type(value, struct noise_config);
} noise_config_map SEC(".maps");

// TC 程序：注入噪声
SEC("tc")
int inject_noise(struct __sk_buff *skb) {
    // 1. 提取连接标识
    __u64 conn_id = extract_conn_id_from_skb(skb);
    
    // 2. 查询噪声配置
    struct noise_config *cfg = bpf_map_lookup_elem(&noise_config_map, &conn_id);
    if (!cfg)
        return TC_ACT_OK;
    
    // 3. 计算总延迟
    __u64 total_delay_us = cfg->fiber_jitter_us + cfg->router_delay_us;
    
    // 4. 设置 skb 时间戳
    skb->tstamp = bpf_ktime_get_ns() + (total_delay_us * 1000);
    
    // 5. 随机丢包（模拟拥塞）
    if (cfg->threat_level >= THREAT_HIGH) {
        __u32 random = bpf_get_prandom_u32() % 100;
        if (random < cfg->noise_intensity / 10) {
            return TC_ACT_SHOT; // 丢弃
        }
    }
    
    return TC_ACT_OK;
}
```

---

## 五、自适应策略

### 5.1 威胁等级映射

```go
type AdaptiveStrategy struct {
    threatLevel ThreatLevel
}

// 计算噪声强度
func (as *AdaptiveStrategy) CalculateIntensity() int {
    switch as.threatLevel {
    case THREAT_NONE:
        return 10 // 10% 强度
    case THREAT_LOW:
        return 30 // 30% 强度
    case THREAT_MEDIUM:
        return 50 // 50% 强度
    case THREAT_HIGH:
        return 70 // 70% 强度
    case THREAT_CRITICAL:
        return 90 // 90% 强度
    default:
        return 10
    }
}

// 计算注入频率
func (as *AdaptiveStrategy) CalculateFrequency() time.Duration {
    switch as.threatLevel {
    case THREAT_NONE:
        return 10 * time.Second // 低频
    case THREAT_LOW:
        return 5 * time.Second
    case THREAT_MEDIUM:
        return 2 * time.Second
    case THREAT_HIGH:
        return 1 * time.Second
    case THREAT_CRITICAL:
        return 500 * time.Millisecond // 高频
    default:
        return 10 * time.Second
    }
}
```

### 5.2 动态调整

```go
type NoiseController struct {
    detector  *ThreatDetector
    strategy  *AdaptiveStrategy
    simulator *NoiseSimulator
    ebpf      *ebpf.Manager
}

// 自动调整噪声
func (nc *NoiseController) AutoAdjust(conn *Connection) {
    ticker := time.NewTicker(1 * time.Second)
    
    for range ticker.C {
        // 1. 检测威胁等级
        threatLevel := nc.detector.Detect(conn)
        
        // 2. 更新策略
        nc.strategy.threatLevel = threatLevel
        
        // 3. 计算噪声参数
        intensity := nc.strategy.CalculateIntensity()
        frequency := nc.strategy.CalculateFrequency()
        
        // 4. 更新 eBPF Map
        config := NoiseConfig{
            FiberJitterUs:  nc.simulator.fiber.Calculate().Microseconds(),
            RouterDelayUs:  nc.simulator.router.Calculate(threatLevel).Microseconds(),
            ThreatLevel:    uint32(threatLevel),
            NoiseIntensity: uint32(intensity),
        }
        
        nc.ebpf.UpdateMap("noise_config_map", conn.ID, config)
        
        // 5. 调整注入频率
        ticker.Reset(frequency)
    }
}
```

---

## 六、噪声流生成

### 6.1 虚假重传

```go
type FakeRetransmitter struct {
    conn *Connection
}

// 生成虚假重传包
func (fr *FakeRetransmitter) Generate() []byte {
    // 1. 克隆真实数据包
    realPacket := fr.conn.GetLastPacket()
    fakePacket := make([]byte, len(realPacket))
    copy(fakePacket, realPacket)
    
    // 2. 修改序列号（模拟重传）
    tcp := parseTCPHeader(fakePacket)
    tcp.SeqNum -= uint32(len(tcp.Payload))
    
    // 3. 重新计算校验和
    tcp.Checksum = calculateChecksum(fakePacket)
    
    return fakePacket
}

// 注入虚假重传
func (fr *FakeRetransmitter) Inject(intensity int) {
    // 基于强度决定注入频率
    probability := float64(intensity) / 100.0
    
    if rand.Float64() < probability {
        fakePacket := fr.Generate()
        fr.conn.SendRaw(fakePacket)
    }
}
```

### 6.2 背景流量

```go
type BackgroundTrafficGenerator struct {
    targets []string
    interval time.Duration
}

// 生成背景流量
func (btg *BackgroundTrafficGenerator) Generate() {
    ticker := time.NewTicker(btg.interval)
    
    for range ticker.C {
        // 1. 随机选择目标
        target := btg.targets[rand.Intn(len(btg.targets))]
        
        // 2. 发起合规请求
        go btg.makeRequest(target)
    }
}

// 发起合规请求
func (btg *BackgroundTrafficGenerator) makeRequest(target string) {
    // 1. 构造 HTTP/3 请求
    client := &http.Client{
        Transport: &http3.RoundTripper{},
    }
    
    // 2. 请求主流网站
    resp, err := client.Get(target)
    if err != nil {
        return
    }
    defer resp.Body.Close()
    
    // 3. 读取部分内容（模拟浏览）
    io.CopyN(io.Discard, resp.Body, 1024)
}
```

---

## 七、性能指标

### 7.1 检测性能

| 指标 | 响应时间 | 准确率 | CPU 开销 |
|------|---------|--------|---------|
| 威胁检测 | < 100ms | 95% | < 2% |
| 拥塞识别 | < 50ms | 98% | < 1% |
| 干扰识别 | < 200ms | 92% | < 3% |

### 7.2 注入性能

| 威胁等级 | 噪声强度 | 延迟增加 | 带宽开销 |
|---------|---------|---------|---------|
| NONE | 10% | < 1ms | 2% |
| LOW | 30% | < 3ms | 5% |
| MEDIUM | 50% | < 5ms | 10% |
| HIGH | 70% | < 8ms | 15% |
| CRITICAL | 90% | < 12ms | 20% |

---

## 八、配置示例

```yaml
vpc:
  # 全局开关
  enabled: true
  
  # 威胁检测
  threat_detection:
    enabled: true
    retransmit_threshold: 0.05  # 5%
    rtt_variance_threshold: 100ms
    icmp_threshold: 10
    check_interval: 1s
  
  # 光缆抖动
  fiber_jitter:
    enabled: true
    base_jitter: 100us
    max_jitter: 2ms
    distribution: poisson
  
  # 路由器队列
  router_queue:
    enabled: true
    min_delay: 0ms
    max_delay: 10ms
    load_factor: 0.5
  
  # 自适应策略
  adaptive:
    enabled: true
    auto_adjust: true
    adjustment_interval: 1s
  
  # 噪声流
  noise_traffic:
    fake_retransmit: true
    background_traffic: true
    targets:
      - https://www.google.com
      - https://www.youtube.com
      - https://www.facebook.com
    interval: 5s
```

---

## 九、实现参考

```go
// pkg/vpc/controller.go
package vpc

type Controller struct {
    detector   *ThreatDetector
    strategy   *AdaptiveStrategy
    fiber      *FiberJitterSimulator
    router     *RouterQueueSimulator
    ebpf       *ebpf.Manager
    retransmit *FakeRetransmitter
    background *BackgroundTrafficGenerator
}

func NewController() *Controller {
    return &Controller{
        detector: NewThreatDetector(),
        strategy: &AdaptiveStrategy{},
        fiber: &FiberJitterSimulator{
            baseJitter:   100 * time.Microsecond,
            maxJitter:    2 * time.Millisecond,
            distribution: "poisson",
        },
        router: &RouterQueueSimulator{
            minDelay:   0,
            maxDelay:   10 * time.Millisecond,
            loadFactor: 0.5,
        },
        ebpf: ebpf.NewManager(),
    }
}

func (c *Controller) Start() error {
    // 1. 加载 eBPF 程序
    if err := c.ebpf.LoadProgram("vpc.o"); err != nil {
        return err
    }
    
    // 2. 挂载监控程序
    if err := c.ebpf.AttachKprobe("tcp_retransmit_skb", "monitor_tcp_retransmit"); err != nil {
        return err
    }
    
    // 3. 挂载注入程序
    if err := c.ebpf.AttachTC("eth0", "egress", "inject_noise"); err != nil {
        return err
    }
    
    // 4. 启动背景流量
    go c.background.Generate()
    
    return nil
}

func (c *Controller) Monitor(conn *Connection) {
    go c.AutoAdjust(conn)
}
```

---

## 十、闭环验证

### 10.1 完整流程

```
感知（< 100ms）
    ↓
eBPF 监控 TCP 重传队列
    ↓
区分：网络拥塞 vs 精准干扰
    ↓
防御（< 1s）
    ↓
注入物理链路背景噪音
    ↓
光缆抖动 + 路由器队列
    ↓
威胁等级自适应
    ↓
逃逸（< 60s）
    ↓
触发 G-Switch 域名转生
    ↓
热切换零感知
```

### 10.2 性能保证

| 阶段 | 响应时间 | 用户感知 | 自动化程度 |
|------|---------|---------|-----------|
| 感知 | < 100ms | 无感知 | 100% |
| 防御 | < 1s | 无感知 | 100% |
| 逃逸 | < 60s | 无感知 | 100% |
| **总计** | **< 60s** | **无感知** | **100%** |

