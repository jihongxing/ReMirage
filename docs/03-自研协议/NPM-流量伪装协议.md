# NPM 流量伪装协议

## 一、协议定位

**形态维度防御**：10%-30% 概率填充，解决数据包长度特征泄露

- **技术栈**：C (eBPF XDP) 数据面 + Go 控制面
- **核心能力**：动态 Padding + 协议拟态 + 流量混淆
- **语言分工**：C 负责内核态零拷贝 Padding，Go 负责策略计算

---

## 二、协议架构

### 2.1 分层设计

```
┌─────────────────────────────────────────────────────────┐
│                    NPM 协议栈                            │
├─────────────────────────────────────────────────────────┤
│ 策略层 (Go)                                              │
│   ├─ 填充策略选择                                        │
│   ├─ 目标协议识别                                        │
│   └─ 概率分布控制                                        │
├─────────────────────────────────────────────────────────┤
│ 执行层 (eBPF XDP)                                        │
│   ├─ 包长度调整                                          │
│   ├─ 随机 Padding 注入                                   │
│   ├─ 协议头部伪装                                        │
│   └─ 零拷贝处理                                          │
├─────────────────────────────────────────────────────────┤
│ 网卡层                                                    │
│   ├─ XDP_TX（直接转发）                                 │
│   ├─ XDP_PASS（上送协议栈）                             │
│   └─ XDP_DROP（丢弃）                                    │
└─────────────────────────────────────────────────────────┘
```

---

## 三、核心算法

### 3.1 动态 Padding 策略

```go
type PaddingStrategy struct {
    MinPadding    int     // 最小填充：10 字节
    MaxPadding    int     // 最大填充：200 字节
    Probability   float64 // 填充概率：10%-30%
    TargetSize    int     // 目标包长（协议拟态）
    Distribution  string  // 分布类型：uniform/gaussian/exponential
}

// 计算填充大小
func (ps *PaddingStrategy) CalculatePadding(originalSize int) int {
    // 1. 概率判断
    if rand.Float64() > ps.Probability {
        return 0 // 不填充
    }
    
    // 2. 根据分布类型计算
    switch ps.Distribution {
    case "uniform":
        return ps.uniformPadding()
    case "gaussian":
        return ps.gaussianPadding(originalSize)
    case "exponential":
        return ps.exponentialPadding()
    case "target":
        return ps.targetPadding(originalSize)
    default:
        return ps.uniformPadding()
    }
}

// 均匀分布填充
func (ps *PaddingStrategy) uniformPadding() int {
    return ps.MinPadding + rand.Intn(ps.MaxPadding-ps.MinPadding)
}

// 高斯分布填充（模拟真实流量）
func (ps *PaddingStrategy) gaussianPadding(originalSize int) int {
    mean := float64(ps.MaxPadding) / 2
    stddev := float64(ps.MaxPadding) / 6
    
    padding := int(rand.NormFloat64()*stddev + mean)
    
    // 限制范围
    if padding < ps.MinPadding {
        padding = ps.MinPadding
    }
    if padding > ps.MaxPadding {
        padding = ps.MaxPadding
    }
    
    return padding
}

// 指数分布填充（模拟突发流量）
func (ps *PaddingStrategy) exponentialPadding() int {
    lambda := 1.0 / float64(ps.MaxPadding)
    padding := int(-math.Log(1-rand.Float64()) / lambda)
    
    if padding < ps.MinPadding {
        padding = ps.MinPadding
    }
    if padding > ps.MaxPadding {
        padding = ps.MaxPadding
    }
    
    return padding
}

// 目标对齐填充（协议拟态）
func (ps *PaddingStrategy) targetPadding(originalSize int) int {
    if originalSize >= ps.TargetSize {
        return 0
    }
    return ps.TargetSize - originalSize
}
```

### 3.2 协议拟态

```go
// 协议特征库
type ProtocolProfile struct {
    Name           string
    TypicalSizes   []int     // 典型包长
    SizeDistribution map[int]float64 // 包长分布
    HeaderPattern  []byte    // 头部特征
}

var ProtocolProfiles = map[string]ProtocolProfile{
    "HTTP/3": {
        Name: "HTTP/3",
        TypicalSizes: []int{64, 128, 256, 512, 1024, 1400},
        SizeDistribution: map[int]float64{
            64:   0.15,
            128:  0.20,
            256:  0.25,
            512:  0.20,
            1024: 0.15,
            1400: 0.05,
        },
    },
    "WebRTC": {
        Name: "WebRTC",
        TypicalSizes: []int{100, 150, 200},
        SizeDistribution: map[int]float64{
            100: 0.30,
            150: 0.40,
            200: 0.30,
        },
    },
    "TLS": {
        Name: "TLS",
        TypicalSizes: []int{517, 1024, 1400},
        SizeDistribution: map[int]float64{
            517:  0.40, // TLS ClientHello
            1024: 0.30,
            1400: 0.30,
        },
    },
}

// 选择目标包长
func selectTargetSize(profile ProtocolProfile) int {
    r := rand.Float64()
    cumulative := 0.0
    
    for size, prob := range profile.SizeDistribution {
        cumulative += prob
        if r <= cumulative {
            return size
        }
    }
    
    return profile.TypicalSizes[0]
}
```

---

## 四、eBPF 实现

### 4.1 数据结构

```c
// bpf/npm.h
struct padding_config {
    __u32 min_padding;      // 最小填充
    __u32 max_padding;      // 最大填充
    __u32 probability;      // 填充概率（0-100）
    __u32 target_size;      // 目标包长
    __u32 distribution;     // 分布类型
};

// BPF Map：存储填充配置
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u64);   // 流标识
    __type(value, struct padding_config);
} padding_config_map SEC(".maps");

// BPF Map：统计信息
struct padding_stats {
    __u64 total_packets;
    __u64 padded_packets;
    __u64 total_padding_bytes;
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct padding_stats);
} padding_stats_map SEC(".maps");
```

### 4.2 XDP 程序

```c
// bpf/npm.c
SEC("xdp")
int apply_padding(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    
    // 1. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;
    
    // 2. 解析 IP 头
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;
    
    // 3. 提取流标识
    __u64 flow_id = extract_flow_id(ip);
    
    // 4. 查询填充配置
    struct padding_config *cfg = bpf_map_lookup_elem(&padding_config_map, &flow_id);
    if (!cfg)
        return XDP_PASS;
    
    // 5. 概率判断
    __u32 random = bpf_get_prandom_u32() % 100;
    if (random > cfg->probability)
        return XDP_PASS;
    
    // 6. 计算填充大小
    __u32 original_size = data_end - data;
    __u32 padding_size = calculate_padding(cfg, original_size);
    
    if (padding_size == 0)
        return XDP_PASS;
    
    // 7. 扩展包空间
    if (bpf_xdp_adjust_tail(ctx, padding_size) < 0)
        return XDP_PASS;
    
    // 8. 填充随机数据
    data = (void *)(long)ctx->data;
    data_end = (void *)(long)ctx->data_end;
    fill_random_padding(data + original_size, padding_size);
    
    // 9. 更新统计
    update_stats(padding_size);
    
    return XDP_PASS;
}

// 计算填充大小
static __always_inline __u32 calculate_padding(
    struct padding_config *cfg,
    __u32 original_size
) {
    __u32 padding = 0;
    
    switch (cfg->distribution) {
    case DIST_UNIFORM:
        padding = cfg->min_padding + 
                  (bpf_get_prandom_u32() % (cfg->max_padding - cfg->min_padding));
        break;
    
    case DIST_TARGET:
        if (original_size < cfg->target_size)
            padding = cfg->target_size - original_size;
        break;
    
    case DIST_GAUSSIAN:
        padding = gaussian_sample(cfg->max_padding / 2, cfg->max_padding / 6);
        if (padding < cfg->min_padding)
            padding = cfg->min_padding;
        if (padding > cfg->max_padding)
            padding = cfg->max_padding;
        break;
    }
    
    return padding;
}

// 填充随机数据
static __always_inline void fill_random_padding(void *start, __u32 size) {
    __u8 *ptr = start;
    
    for (__u32 i = 0; i < size && i < 256; i++) {
        ptr[i] = bpf_get_prandom_u32() & 0xFF;
    }
}

// 更新统计
static __always_inline void update_stats(__u32 padding_size) {
    __u32 key = 0;
    struct padding_stats *stats = bpf_map_lookup_elem(&padding_stats_map, &key);
    
    if (stats) {
        __sync_fetch_and_add(&stats->total_packets, 1);
        __sync_fetch_and_add(&stats->padded_packets, 1);
        __sync_fetch_and_add(&stats->total_padding_bytes, padding_size);
    }
}
```

---

## 五、流量混淆

### 5.1 协议头部伪装

```c
// 伪装 HTTP/3 头部
static __always_inline void mimic_http3_header(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    
    // 1. 定位 UDP 载荷
    struct udphdr *udp = locate_udp_header(data, data_end);
    if (!udp)
        return;
    
    void *payload = (void *)(udp + 1);
    if ((void *)(payload + 16) > data_end)
        return;
    
    // 2. 伪装 QUIC 头部
    __u8 *quic_header = payload;
    quic_header[0] = 0xC0 | 0x03; // Long Header + Version 1
    quic_header[1] = 0x00;
    quic_header[2] = 0x00;
    quic_header[3] = 0x01;
    
    // 3. 随机 Connection ID
    for (int i = 4; i < 12; i++) {
        quic_header[i] = bpf_get_prandom_u32() & 0xFF;
    }
}

// 伪装 TLS 头部
static __always_inline void mimic_tls_header(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    
    struct tcphdr *tcp = locate_tcp_header(data, data_end);
    if (!tcp)
        return;
    
    void *payload = (void *)(tcp + 1);
    if ((void *)(payload + 5) > data_end)
        return;
    
    // TLS Record Header
    __u8 *tls_header = payload;
    tls_header[0] = 0x17; // Application Data
    tls_header[1] = 0x03; // TLS 1.2
    tls_header[2] = 0x03;
    
    // Length (Big Endian)
    __u16 length = data_end - payload - 5;
    tls_header[3] = (length >> 8) & 0xFF;
    tls_header[4] = length & 0xFF;
}
```

### 5.2 流量注入

```go
type TrafficInjector struct {
    interval time.Duration
    size     int
}

// 空闲时注入噪声流量
func (ti *TrafficInjector) InjectNoise(conn *Connection) {
    ticker := time.NewTicker(ti.interval)
    
    for range ticker.C {
        // 1. 生成随机数据
        noise := make([]byte, ti.size)
        rand.Read(noise)
        
        // 2. 伪装成正常业务流量
        packet := ti.wrapAsHTTP3(noise)
        
        // 3. 发送
        conn.Send(packet)
    }
}

// 伪装成 HTTP/3 请求
func (ti *TrafficInjector) wrapAsHTTP3(data []byte) []byte {
    // QUIC Header + HTTP/3 HEADERS Frame
    header := []byte{
        0xC0, 0x00, 0x00, 0x01, // QUIC Long Header
        0x08, // DCID Length
    }
    
    // Random Connection ID
    cid := make([]byte, 8)
    rand.Read(cid)
    header = append(header, cid...)
    
    // HTTP/3 HEADERS Frame
    frame := []byte{
        0x01, // HEADERS Frame Type
        0x00, // Length (placeholder)
    }
    
    return append(append(header, frame...), data...)
}
```

---

## 六、性能优化

### 6.1 零拷贝

```c
// XDP 零拷贝 Padding
SEC("xdp")
int zero_copy_padding(struct xdp_md *ctx) {
    // 1. 直接在网卡 DMA 缓冲区操作
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    
    __u32 original_size = data_end - data;
    __u32 padding_size = 100;
    
    // 2. 扩展尾部（零拷贝）
    if (bpf_xdp_adjust_tail(ctx, padding_size) < 0)
        return XDP_PASS;
    
    // 3. 更新指针
    data_end = (void *)(long)ctx->data_end;
    
    // 4. 填充（直接写入 DMA 缓冲区）
    __u8 *padding_start = data + original_size;
    for (int i = 0; i < padding_size && i < 256; i++) {
        padding_start[i] = 0xFF;
    }
    
    return XDP_TX; // 直接从网卡发送
}
```

### 6.2 批处理

```go
type BatchPadder struct {
    batchSize int
    queue     chan *Packet
}

func (bp *BatchPadder) Process() {
    batch := make([]*Packet, 0, bp.batchSize)
    
    for packet := range bp.queue {
        batch = append(batch, packet)
        
        if len(batch) >= bp.batchSize {
            bp.processBatch(batch)
            batch = batch[:0]
        }
    }
}

func (bp *BatchPadder) processBatch(batch []*Packet) {
    // 1. 批量计算填充大小
    paddings := make([]int, len(batch))
    for i, pkt := range batch {
        paddings[i] = bp.calculatePadding(pkt.Size)
    }
    
    // 2. 批量更新 eBPF Map
    for i, pkt := range batch {
        bp.ebpf.UpdateMap("padding_config_map", pkt.FlowID, paddings[i])
    }
}
```

---

## 七、性能指标

### 7.1 延迟开销

| 实现方式 | 延迟增加 | CPU 开销 | 吞吐影响 |
|---------|---------|---------|---------|
| 用户态 Padding | 5-10ms | 8%-12% | -20% |
| 内核态 iptables | 2-5ms | 5%-8% | -10% |
| eBPF XDP | < 0.5ms | < 2% | -3% |
| 硬件卸载 | < 0.1ms | < 0.5% | -0.5% |

### 7.2 带宽开销

| 填充概率 | 平均填充 | 带宽增加 | 隐蔽性提升 |
|---------|---------|---------|-----------|
| 10% | 50 字节 | 5% | 中 |
| 20% | 100 字节 | 12% | 高 |
| 30% | 150 字节 | 18% | 极高 |

---

## 八、配置示例

```yaml
npm:
  # 全局开关
  enabled: true
  
  # 填充策略
  padding:
    min_size: 10
    max_size: 200
    probability: 0.20  # 20%
    distribution: gaussian
  
  # 协议拟态
  protocol_mimic:
    enabled: true
    target: HTTP/3
    profiles:
      - HTTP/3
      - WebRTC
      - TLS
  
  # 流量注入
  traffic_injection:
    enabled: true
    interval: 5s
    size: 100
  
  # 性能
  performance:
    xdp_mode: native  # native/offload/generic
    zero_copy: true
    batch_size: 64
```

---

## 九、实现参考

```go
// pkg/npm/padder.go
package npm

type Padder struct {
    ebpf     *ebpf.Manager
    strategy *PaddingStrategy
    profiles map[string]ProtocolProfile
}

func NewPadder() *Padder {
    return &Padder{
        ebpf: ebpf.NewManager(),
        strategy: &PaddingStrategy{
            MinPadding:   10,
            MaxPadding:   200,
            Probability:  0.20,
            Distribution: "gaussian",
        },
        profiles: ProtocolProfiles,
    }
}

func (p *Padder) Start() error {
    // 1. 加载 eBPF 程序
    if err := p.ebpf.LoadProgram("npm.o"); err != nil {
        return err
    }
    
    // 2. 挂载到 XDP
    return p.ebpf.AttachXDP("eth0", XDP_MODE_NATIVE)
}

func (p *Padder) UpdateConfig(flowID uint64, cfg PaddingConfig) error {
    return p.ebpf.UpdateMap("padding_config_map", flowID, cfg)
}
```

