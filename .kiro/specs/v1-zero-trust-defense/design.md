# 设计文档：零信任抗审计三层纵深防御

## 概述

本设计实现三层纵深防御矩阵（L1 物理层 / L2 密码学层 / L3 行为层）及威胁情报本地化富化。严格遵循语言分工铁律：L1 层数据包处理用 C（eBPF XDP/TC），L2/L3 层业务逻辑用 Go，通信通过 eBPF Map + Ring Buffer。

## 设计原则

1. **C 做数据面，Go 做控制面**：ASN 清洗、速率限制、静默响应在 XDP/TC 层执行；mTLS 抗重放、行为基线在 Go 用户空间执行
2. **Go → C 通过 eBPF Map**：ASN 网段通过 LPM Trie Map 下发，速率配置通过 Hash Map 下发
3. **C → Go 通过 Ring Buffer**：速率限制触发事件、ASN 命中事件通过 Ring Buffer 上报
4. **Default-Drop**：所有拒绝动作均为静默丢弃，禁止返回任何可识别系统的信息
5. **数据本地化闭环**：所有富化数据在本地内存完成查询，禁止数据面向外查询

---

## 模块 1：L1 纵深防御 eBPF 程序（需求 1/2/3）

### 改动范围

- 新建 `mirage-gateway/bpf/l1_defense.c`：L1 层 XDP 防御程序
- 修改 `mirage-gateway/bpf/common.h`：增加 L1 防御相关 Map 和结构体定义
- 修改 `mirage-gateway/bpf/npm.c`：在 XDP 入口集成 L1 防御检查
- 修改 `mirage-gateway/pkg/ebpf/types.go`：增加 L1 防御相关 Go 结构体
- 修改 `mirage-gateway/pkg/ebpf/manager.go`：增加 L1 防御 Map 管理

### 设计细节

#### common.h 新增结构体和 Map

```c
/* L1 防御：速率限制配置（Go → C） */
struct rate_limit_config {
    __u32 syn_pps_limit;        // SYN 包每秒上限（默认 50）
    __u32 conn_pps_limit;       // 总连接每秒上限（默认 200）
    __u32 enabled;              // 是否启用
};

/* L1 防御：每 IP 速率计数器 */
struct rate_counter {
    __u64 syn_count;            // SYN 包计数
    __u64 conn_count;           // 总连接计数
    __u64 window_start;         // 窗口起始时间（纳秒）
};

/* L1 防御：速率限制触发事件（C → Go） */
struct rate_event {
    __u64 timestamp;
    __u32 source_ip;
    __u32 trigger_type;         // 0=SYN, 1=CONN
    __u64 current_rate;
};

/* L1 防御：静默响应配置（Go → C） */
struct silent_config {
    __u32 drop_icmp_unreachable;  // 拦截 ICMP Unreachable
    __u32 drop_tcp_rst;           // 拦截非法 TCP RST
    __u32 enabled;
};
```

新增 eBPF Map：

```c
// ASN 黑名单 LPM Trie（Go → C，与 blacklist_lpm 独立）
struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __uint(max_entries, 131072);  // 128K 条目，覆盖主流云厂商网段
    __uint(map_flags, BPF_F_NO_PREALLOC);
    __type(key, struct lpm_key);
    __type(value, __u32);         // ASN 编号（用于统计归因）
} asn_blocklist_lpm SEC(".maps");

// 速率限制计数器（Per-IP LRU）
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 65536);
    __type(key, __u32);           // 源 IP
    __type(value, struct rate_counter);
} rate_limit_map SEC(".maps");

// 速率限制配置（Go → C）
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct rate_limit_config);
} rate_config_map SEC(".maps");

// 静默响应配置（Go → C）
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct silent_config);
} silent_config_map SEC(".maps");

// L1 防御事件 Ring Buffer（C → Go）
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} l1_defense_events SEC(".maps");

// L1 统计计数器
struct l1_stats {
    __u64 asn_drops;
    __u64 rate_drops;
    __u64 silent_drops;
    __u64 total_checked;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct l1_stats);
} l1_stats_map SEC(".maps");
```

#### l1_defense.c XDP 程序

```c
SEC("xdp")
int l1_defense_main(struct xdp_md *ctx) {
    // 1. 解析 IP 头
    // 2. ASN 黑名单查询 → XDP_DROP
    // 3. 速率限制检查 → XDP_DROP + Ring Buffer 上报
    // 4. XDP_PASS（放行到下一层）
}
```

ASN 查询逻辑：
```c
struct lpm_key key = { .prefixlen = 32, .addr = ip->saddr };
__u32 *asn = bpf_map_lookup_elem(&asn_blocklist_lpm, &key);
if (asn) {
    // 更新统计
    stats->asn_drops++;
    return XDP_DROP;
}
```

速率限制逻辑：
```c
__u64 now = bpf_ktime_get_ns();
struct rate_counter *counter = bpf_map_lookup_elem(&rate_limit_map, &ip->saddr);
if (counter) {
    // 窗口过期则重置
    if (now - counter->window_start > 1000000000ULL) { // 1秒
        counter->syn_count = 0;
        counter->conn_count = 0;
        counter->window_start = now;
    }
    // 检查 SYN 限制
    if (is_syn && counter->syn_count >= cfg->syn_pps_limit) {
        // 上报事件到 Ring Buffer
        return XDP_DROP;
    }
    // 检查总连接限制
    if (counter->conn_count >= cfg->conn_pps_limit) {
        return XDP_DROP;
    }
    // 递增计数
    if (is_syn) counter->syn_count++;
    counter->conn_count++;
}
```

#### 静默响应（TC egress）

静默响应需要在 TC egress 层拦截出站的 ICMP Unreachable 和非法 TCP RST：

```c
SEC("tc")
int l1_silent_egress(struct __sk_buff *skb) {
    // 1. 解析 IP 头
    // 2. 检查 ICMP Type 3 → TC_ACT_SHOT
    // 3. 检查 TCP RST（非已建立连接） → TC_ACT_SHOT
    // 4. TC_ACT_OK
}
```

#### npm.c 集成

在 `npm_xdp_main` 入口最前面调用 L1 防御检查。由于 Linux 内核限制一个网卡只能挂载一个 XDP 程序，L1 防御逻辑需要集成到 npm.c 的 XDP 入口中：

```c
SEC("xdp")
int npm_xdp_main(struct xdp_md *ctx) {
    // L1 防御检查（最高优先级）
    int l1_action = handle_l1_defense(ctx);
    if (l1_action != XDP_PASS)
        return l1_action;

    // 原有 NPM 逻辑
    int action = handle_npm_strip(ctx);
    if (action != XDP_PASS)
        return action;
    return handle_npm_padding(ctx);
}
```

`handle_l1_defense` 作为 `static __always_inline` 函数定义在 `l1_defense.c` 中，通过 `#include` 引入 npm.c，或直接在 common.h 中定义。

---

## 模块 2：抗重放归因（需求 4）

### 改动范围

- 新建 `mirage-gateway/pkg/threat/nonce_store.go`：Nonce 存储与重放检测
- 修改 `mirage-gateway/pkg/threat/types.go`：增加 `ThreatReplayAttack` 相关常量（已存在）
- 修改 `mirage-gateway/pkg/cortex/risk_scorer.go`：集成重放检测评分

### 设计细节

#### NonceStore

```go
// pkg/threat/nonce_store.go
type NonceStore struct {
    mu       sync.RWMutex
    entries  map[string]*nonceEntry  // nonce_hex → entry
    maxSize  int                      // 上限 100000
    ttl      time.Duration            // 5 分钟
}

type nonceEntry struct {
    SourceIP  string
    Timestamp time.Time
    CreatedAt time.Time
}

func NewNonceStore(maxSize int, ttl time.Duration) *NonceStore

// CheckAndStore 检查 Nonce 是否重复，不重复则存储
// 返回 (isDuplicate bool, originalIP string)
func (ns *NonceStore) CheckAndStore(nonce []byte, sourceIP string, timestamp time.Time) (bool, string)

// StartCleanup 启动过期清理循环（每 30 秒清理一次）
func (ns *NonceStore) StartCleanup(ctx context.Context)
```

集成点：在 TLS 握手回调中（`VerifyPeerCertificate` 或自定义握手拦截器），提取 ClientHello 的 Random 字段作为 Nonce，调用 `NonceStore.CheckAndStore`。

重放检测触发后：
1. 静默关闭连接（不发送 TLS Alert）
2. `BlacklistManager.Add(sourceIP+"/32", 2h, SourceLocal)`
3. `RiskScorer.AddScore(sourceIP, 30, "replay_attack")`
4. 通过 `threat_events` Ring Buffer 上报 `ThreatReplayAttack` 事件

---

## 模块 3：半开连接熔断（需求 5）

### 改动范围

- 新建 `mirage-gateway/pkg/threat/handshake_guard.go`：握手超时熔断器
- 修改 `mirage-gateway/pkg/threat/metrics.go`：增加 `mirage_handshake_timeout_total` 指标

### 设计细节

#### HandshakeGuard

```go
// pkg/threat/handshake_guard.go
type HandshakeGuard struct {
    timeout     time.Duration          // 300ms
    mu          sync.Mutex
    ipCounters  map[string]*hsCounter  // IP → 超时计数
    blacklist   *BlacklistManager
    riskScorer  *cortex.RiskScorer
}

type hsCounter struct {
    Count     int
    WindowStart time.Time
}

func NewHandshakeGuard(timeout time.Duration, bl *BlacklistManager, rs *cortex.RiskScorer) *HandshakeGuard

// WrapListener 包装 net.Listener，为每个连接注入握手超时
func (hg *HandshakeGuard) WrapListener(ln net.Listener) net.Listener

// onTimeout 握手超时回调
// 1. 静默 RST（不发送 TLS Alert）
// 2. 递增 IP 超时计数
// 3. 同一 IP 1 分钟内 > 5 次 → 黑名单 1h + RiskScorer +20
func (hg *HandshakeGuard) onTimeout(sourceIP string)
```

集成点：在 TLS Listener 创建后，用 `HandshakeGuard.WrapListener` 包装，对每个 Accept 的连接设置 `SetDeadline(time.Now().Add(300ms))`，握手完成后清除 Deadline。

---

## 模块 4：协议异常检测（需求 6）

### 改动范围

- 新建 `mirage-gateway/pkg/threat/protocol_detector.go`：协议指纹检测器
- 修改 `mirage-gateway/pkg/threat/metrics.go`：增加 `mirage_protocol_scan_total` 指标

### 设计细节

#### ProtocolDetector

```go
// pkg/threat/protocol_detector.go
type ProtocolDetector struct {
    riskScorer *cortex.RiskScorer
    blacklist  *BlacklistManager
}

// 协议签名常量
var protocolSignatures = map[string][]byte{
    "ssh":  []byte("SSH-"),
    "http_get":  []byte("GET "),
    "http_post": []byte("POST"),
    "http_head": []byte("HEAD"),
    "http_conn": []byte("CONN"),
}

// Detect 读取连接前 8 字节，检测非 TLS 协议
// 返回 (isMalicious bool, protocolType string)
// 使用 io.ReadFull + 超时，不消耗 TLS 握手数据（Peek 模式）
func (pd *ProtocolDetector) Detect(conn net.Conn) (bool, string)

// HandleMalicious 处理恶意协议检测
// 1. 静默关闭连接
// 2. RiskScorer.AddScore(ip, 40, "protocol_scan:"+protocolType)
// 3. 递增 Prometheus 指标
func (pd *ProtocolDetector) HandleMalicious(sourceIP string, protocolType string)
```

集成点：在 Accept 连接后、TLS 握手前，先用 `bufio.NewReader` 包装连接，`Peek(8)` 读取前 8 字节进行协议检测。检测通过后将 buffered reader 传递给 TLS 握手。

---

## 模块 5：行为基线监控（需求 7）

### 改动范围

- 新建 `mirage-gateway/pkg/cortex/behavior_monitor.go`：行为基线监控器
- 新建 `mirage-gateway/pkg/cortex/markov_model.go`：马尔可夫链基线模型
- 修改 `mirage-gateway/pkg/threat/metrics.go`：增加 `mirage_behavior_anomaly_total` 指标

### 设计细节

#### ConnProfile

```go
// pkg/cortex/behavior_monitor.go
type ConnProfile struct {
    ConnID        string
    SourceIP      string
    StartTime     time.Time
    PacketSizes   []int          // 包长序列（最近 1000 个）
    SendBytes     uint64
    RecvBytes     uint64
    PacketCount   uint64
    LastPacketAt  time.Time
    IntervalSum   time.Duration  // 包间隔累加
}

// SendRecvRatio 计算收发比例
func (cp *ConnProfile) SendRecvRatio() float64

// AvgPacketInterval 计算平均包间隔
func (cp *ConnProfile) AvgPacketInterval() time.Duration

// DataEntropy 计算数据熵值（Shannon Entropy）
func (cp *ConnProfile) DataEntropy() float64
```

#### BehaviorMonitor

```go
type BehaviorMonitor struct {
    mu          sync.RWMutex
    profiles    map[string]*ConnProfile  // connID → profile
    baseline    *MarkovModel
    threshold   float64                   // 偏离度阈值（默认 0.7）
    riskScorer  *cortex.RiskScorer
    onKick      func(connID string)       // 踢下线回调
}

func NewBehaviorMonitor(baseline *MarkovModel, threshold float64, rs *cortex.RiskScorer) *BehaviorMonitor

// RecordPacket 记录数据包（每个包调用一次）
func (bm *BehaviorMonitor) RecordPacket(connID string, size int, direction int)

// StartMonitoring 启动监控循环（每 10 秒评估一次）
func (bm *BehaviorMonitor) StartMonitoring(ctx context.Context)

// evaluate 评估所有活跃连接
// 1. 计算行为偏离度
// 2. 偏离度 > threshold → 踢下线 + RiskScorer +25
// 3. 只发不收（ratio < 0.05 持续 30s） → 踢下线
func (bm *BehaviorMonitor) evaluate()
```

#### MarkovModel

```go
// pkg/cortex/markov_model.go
type MarkovModel struct {
    TransitionMatrix map[int]map[int]float64  // state → state → probability
    States           []int                     // 离散化的包长区间
}

// DefaultBaseline 返回内置的合法流量基线模型
func DefaultBaseline() *MarkovModel

// Deviation 计算观测序列与基线的偏离度（0.0 ~ 1.0）
// 使用 KL 散度归一化
func (m *MarkovModel) Deviation(observed []int) float64
```

---

## 模块 6：威胁情报本地化富化（需求 8）

### 改动范围

- 新建 `mirage-gateway/pkg/threat/intel_provider.go`：威胁情报提供器
- 新建 `mirage-gateway/pkg/threat/asn_database.go`：ASN 离线数据库
- 新建 `mirage-gateway/pkg/threat/cloud_ranges.go`：云厂商网段库
- 修改 `mirage-gateway/pkg/threat/metrics.go`：增加 `mirage_threat_intel_lookup_total` 指标

### 设计细节

#### ThreatIntelProvider

```go
// pkg/threat/intel_provider.go
type ThreatIntelProvider struct {
    mu           sync.RWMutex
    asnDB        *ASNDatabase
    cloudRanges  *CloudRangeDB
}

type ASNInfo struct {
    ASN         uint32
    Org         string
    Country     string
    IsDataCenter bool
}

type CloudProvider string

const (
    CloudAWS     CloudProvider = "aws"
    CloudAzure   CloudProvider = "azure"
    CloudGCP     CloudProvider = "gcp"
    CloudAliyun  CloudProvider = "aliyun"
    CloudTencent CloudProvider = "tencent"
)

func NewThreatIntelProvider(asnPath, cloudRangesPath string) (*ThreatIntelProvider, error)

// LookupASN 查询 IP 的 ASN 信息（纯内存查询，O(log n)）
func (tip *ThreatIntelProvider) LookupASN(ip string) *ASNInfo

// IsCloudIP 检查 IP 是否属于云厂商数据中心
func (tip *ThreatIntelProvider) IsCloudIP(ip string) (bool, CloudProvider)

// Reload 热更新数据库（OS 下发新数据后调用）
func (tip *ThreatIntelProvider) Reload(asnPath, cloudRangesPath string) error
```

#### ASNDatabase

```go
// pkg/threat/asn_database.go
type ASNDatabase struct {
    trie *net.IPNet  // 内存中的前缀树
    entries []asnEntry
}

type asnEntry struct {
    Network  *net.IPNet
    ASN      uint32
    Org      string
    Country  string
}

// LoadFromFile 从本地 JSON 文件加载
func LoadASNDatabase(path string) (*ASNDatabase, error)

// Lookup 前缀匹配查询
func (db *ASNDatabase) Lookup(ip net.IP) *ASNInfo
```

#### CloudRangeDB

```go
// pkg/threat/cloud_ranges.go
type CloudRangeDB struct {
    ranges map[CloudProvider][]*net.IPNet
}

// LoadFromFile 从本地 JSON 文件加载
func LoadCloudRanges(path string) (*CloudRangeDB, error)

// Match 检查 IP 是否命中云厂商网段
func (db *CloudRangeDB) Match(ip net.IP) (bool, CloudProvider)
```

数据文件格式（`configs/cloud_ranges.json`）：
```json
{
  "aws": ["3.0.0.0/15", "13.32.0.0/15", ...],
  "azure": ["13.64.0.0/11", ...],
  "gcp": ["8.8.4.0/24", "8.8.8.0/24", ...],
  "aliyun": ["47.52.0.0/16", ...],
  "tencent": ["49.51.0.0/16", ...]
}
```

集成点：
1. 启动时 `NewThreatIntelProvider` 加载本地文件
2. 在入口处置链中，`IngressPolicy.Evaluate` 前调用 `IsCloudIP`，命中则 `RiskScorer.AddScore(ip, 20, "cloud_datacenter")`
3. ASN 网段同步到 `asn_blocklist_lpm` eBPF Map，由 L1 层直接丢弃

---

## 模块 7：Prometheus 指标扩展

### 改动范围

- 修改 `mirage-gateway/pkg/threat/metrics.go`：增加所有新指标

### 新增指标

```go
var (
    ASNDropTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_asn_drop_total",
        Help: "Total ASN blocklist drops at XDP layer",
    }, []string{"gateway_id"})

    RateLimitDropTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_ratelimit_drop_total",
        Help: "Total rate limit drops at XDP layer",
    }, []string{"gateway_id", "trigger_type"})

    SilentDropTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_silent_drop_total",
        Help: "Total silent response drops at TC layer",
    }, []string{"gateway_id"})

    HandshakeTimeoutTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_handshake_timeout_total",
        Help: "Total handshake timeouts",
    }, []string{"gateway_id"})

    ProtocolScanTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_protocol_scan_total",
        Help: "Total protocol scan detections",
    }, []string{"gateway_id", "protocol"})

    BehaviorAnomalyTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_behavior_anomaly_total",
        Help: "Total behavior anomaly detections",
    }, []string{"gateway_id"})

    ThreatIntelLookupTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_threat_intel_lookup_total",
        Help: "Total threat intel lookups",
    }, []string{"gateway_id", "result"})
)
```

---

## 模块 8：主程序集成

### 改动范围

- 修改 `mirage-gateway/cmd/gateway/main.go`：初始化并串联所有 L1/L2/L3 模块

### 初始化顺序

```
1. 加载配置
2. 加载威胁情报库（ThreatIntelProvider）
3. 初始化 eBPF Loader（加载 L1 防御 Map）
4. 同步 ASN 网段到 asn_blocklist_lpm Map
5. 同步速率限制配置到 rate_config_map
6. 同步静默响应配置到 silent_config_map
7. 启动 L1 Ring Buffer 监听（l1_defense_events）
8. 初始化 NonceStore（L2 抗重放）
9. 初始化 HandshakeGuard（L2 半开熔断）
10. 初始化 ProtocolDetector（L3 协议检测）
11. 初始化 BehaviorMonitor（L3 行为基线）
12. 包装 TLS Listener：ProtocolDetector → HandshakeGuard → TLS Handshake
13. 启动 BehaviorMonitor 监控循环
```

---

## 配置变更

`gateway.yaml` 新增：

```yaml
defense:
  l1:
    asn_blocklist_path: "/etc/mirage/asn_database.bin"
    cloud_ranges_path: "/etc/mirage/cloud_ranges.json"
    rate_limit:
      syn_pps: 50
      conn_pps: 200
      enabled: true
    silent_response:
      drop_icmp_unreachable: true
      drop_tcp_rst: true
      enabled: true
  l2:
    nonce_store_size: 100000
    nonce_ttl_seconds: 300
    handshake_timeout_ms: 300
    replay_ban_ttl_hours: 2
  l3:
    behavior_check_interval_seconds: 10
    deviation_threshold: 0.7
    send_recv_ratio_min: 0.05
    send_recv_timeout_seconds: 30
```

---

## 不在本次范围内

- RPKI 路由源认证（后续版本）
- 互联网背景噪音库（GreyNoise 类）集成（后续版本）
- 跨节点威胁情报同步（由 Spec 2-1 覆盖）
- OS 侧威胁情报推送协议（由 Spec 2-3 覆盖）

---

## 正确性属性

### P1：ASN 黑名单 LPM Trie 一致性（不变量）

对于所有通过 Go 控制面写入 `asn_blocklist_lpm` 的 CIDR 网段，eBPF Map 中的条目数量应等于 Go 侧维护的条目数量。写入后立即查询应命中。

### P2：NonceStore 抗重放（幂等性）

对于任意 Nonce 值 N，`CheckAndStore(N)` 首次调用返回 `isDuplicate=false`，后续所有调用返回 `isDuplicate=true`，直到 N 过期被清理。

### P3：速率限制窗口重置（状态机属性）

对于任意源 IP，当窗口时间（1 秒）过期后，该 IP 的速率计数器应重置为 0，不影响后续合法流量。

### P4：行为偏离度范围（不变量）

`MarkovModel.Deviation()` 的返回值始终在 [0.0, 1.0] 区间内，对于完全匹配基线的序列返回 0.0。

### P5：威胁情报查询纯本地（约束属性）

`ThreatIntelProvider.LookupASN` 和 `IsCloudIP` 的执行路径中不包含任何网络 I/O 调用（net.Dial / http.Get / dns.Lookup 等）。

### P6：协议检测不消耗数据（往返属性）

`ProtocolDetector.Detect` 使用 Peek 模式读取前 8 字节后，后续 TLS 握手仍能读取到完整的 ClientHello 数据。
