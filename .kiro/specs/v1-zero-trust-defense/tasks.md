# 任务清单：零信任抗审计三层纵深防御

## 需求 1：静态 ASN/网段清洗（L1 - XDP 层）

- [x] 1. ASN/网段清洗
  - [x] 1.1 修改 `mirage-gateway/bpf/common.h`：增加 L1 防御相关结构体定义 — `struct rate_limit_config`（syn_pps_limit/conn_pps_limit/enabled）、`struct rate_counter`（syn_count/conn_count/window_start）、`struct rate_event`（timestamp/source_ip/trigger_type/current_rate）、`struct silent_config`（drop_icmp_unreachable/drop_tcp_rst/enabled）、`struct l1_stats`（asn_drops/rate_drops/silent_drops/total_checked）
  - [x] 1.2 修改 `mirage-gateway/bpf/common.h`：增加 L1 防御相关 eBPF Map 定义 — `asn_blocklist_lpm`（LPM_TRIE, max_entries=131072, key=struct lpm_key, value=__u32）、`rate_limit_map`（LRU_HASH, max_entries=65536, key=__u32, value=struct rate_counter）、`rate_config_map`（ARRAY, max_entries=1）、`silent_config_map`（ARRAY, max_entries=1）、`l1_defense_events`（RINGBUF, max_entries=256*1024）、`l1_stats_map`（PERCPU_ARRAY, max_entries=1）
  - [x] 1.3 新建 `mirage-gateway/bpf/l1_defense.c`：实现 `static __always_inline int handle_l1_asn_check(struct xdp_md *ctx)` 函数 — 解析 IP 头获取 saddr，构造 lpm_key（prefixlen=32, addr=saddr），调用 `bpf_map_lookup_elem(&asn_blocklist_lpm, &key)`，命中时递增 `l1_stats_map.asn_drops` 并返回 `XDP_DROP`，未命中返回 `XDP_PASS`
  - [x] 1.4 修改 `mirage-gateway/pkg/ebpf/types.go`：增加 Go 侧 L1 防御结构体 — `RateLimitConfig`（SynPPSLimit/ConnPPSLimit/Enabled uint32）、`RateEvent`（Timestamp uint64/SourceIP uint32/TriggerType uint32/CurrentRate uint64）、`SilentConfig`（DropICMPUnreachable/DropTCPRst/Enabled uint32）、`L1Stats`（ASNDrops/RateDrops/SilentDrops/TotalChecked uint64）
  - [x] 1.5 新建 `mirage-gateway/pkg/threat/intel_provider.go`：实现 `ThreatIntelProvider` 结构体 — `NewThreatIntelProvider(asnPath, cloudRangesPath string) (*ThreatIntelProvider, error)` 从本地文件加载；`LookupASN(ip string) *ASNInfo` 纯内存前缀匹配查询；`IsCloudIP(ip string) (bool, CloudProvider)` 检查云厂商网段；`Reload(asnPath, cloudRangesPath string) error` 热更新
  - [x] 1.6 新建 `mirage-gateway/pkg/threat/asn_database.go`：实现 `ASNDatabase` 结构体 — `LoadASNDatabase(path string) (*ASNDatabase, error)` 从本地 JSON 文件加载 ASN 条目到内存排序数组；`Lookup(ip net.IP) *ASNInfo` 使用二分查找进行前缀匹配
  - [x] 1.7 新建 `mirage-gateway/pkg/threat/cloud_ranges.go`：实现 `CloudRangeDB` 结构体 — `LoadCloudRanges(path string) (*CloudRangeDB, error)` 从本地 JSON 文件加载各云厂商 CIDR 列表；`Match(ip net.IP) (bool, CloudProvider)` 遍历匹配
  - [x] 1.8 修改 `mirage-gateway/pkg/ebpf/manager.go`：增加 `SyncASNBlocklist(entries []ASNBlockEntry) error` 方法 — 遍历 entries，将每个 CIDR 转换为 lpm_key 写入 `asn_blocklist_lpm` Map；增加 `ASNBlockEntry` 结构体（CIDR string, ASN uint32）

## 需求 2：速率限制（L1 - XDP 层）

- [x] 2. 速率限制
  - [x] 2.1 修改 `mirage-gateway/bpf/l1_defense.c`：实现 `static __always_inline int handle_l1_rate_limit(struct xdp_md *ctx, __u32 saddr, __u8 is_syn)` 函数 — 从 `rate_config_map` 读取配置，从 `rate_limit_map` 查询/创建该 IP 的计数器，窗口过期（>1s）则重置，SYN 包超过 syn_pps_limit 或总连接超过 conn_pps_limit 时递增 `l1_stats_map.rate_drops`、通过 `l1_defense_events` Ring Buffer 上报 `rate_event`、返回 `XDP_DROP`
  - [x] 2.2 修改 `mirage-gateway/bpf/l1_defense.c`：实现 `static __always_inline int handle_l1_defense(struct xdp_md *ctx)` 主入口函数 — 解析以太网头和 IP 头，提取 saddr 和 TCP flags（判断 SYN），依次调用 `handle_l1_asn_check` → `handle_l1_rate_limit`，任一返回非 XDP_PASS 则直接返回
  - [x] 2.3 修改 `mirage-gateway/bpf/npm.c`：在 `npm_xdp_main` 函数最前面增加 `#include "l1_defense.c"` 并调用 `int l1_action = handle_l1_defense(ctx); if (l1_action != XDP_PASS) return l1_action;`（在 handle_npm_strip 之前）
  - [x] 2.4 修改 `mirage-gateway/pkg/ebpf/manager.go`：增加 `SyncRateLimitConfig(cfg *RateLimitConfig) error` 方法 — 将配置写入 `rate_config_map` eBPF Map
  - [x] 2.5 新建 `mirage-gateway/pkg/threat/l1_monitor.go`：实现 `L1Monitor` 结构体 — `NewL1Monitor(loader *ebpf.Loader, riskScorer *cortex.RiskScorer) *L1Monitor`；`StartEventLoop(ctx context.Context)` 从 `l1_defense_events` Ring Buffer 读取 `rate_event`，解析源 IP 后调用 `riskScorer.AddScore(ip, 20, "rate_limit")`；定期（每 5 秒）从 `l1_stats_map` 读取统计并更新 Prometheus 指标

## 需求 3：静默响应（L1 - TC 层）

- [x] 3. 静默响应
  - [x] 3.1 新建 `mirage-gateway/bpf/l1_silent.c`：实现 TC egress 程序 `l1_silent_egress` — 解析 IP 头，检查协议类型：ICMP（protocol=1）且 Type=3 时从 `silent_config_map` 读取配置，`drop_icmp_unreachable` 启用则返回 `TC_ACT_SHOT`；TCP（protocol=6）且 RST 标志位置位时，`drop_tcp_rst` 启用则返回 `TC_ACT_SHOT`；递增 `l1_stats_map.silent_drops`；其余返回 `TC_ACT_OK`
  - [x] 3.2 修改 `mirage-gateway/pkg/ebpf/manager.go`：增加 `SyncSilentConfig(cfg *SilentConfig) error` 方法 — 将配置写入 `silent_config_map` eBPF Map
  - [x] 3.3 修改 `mirage-gateway/pkg/ebpf/loader.go`：在 eBPF 程序加载流程中增加 `l1_silent.c` 的 TC egress 挂载逻辑（使用 netlink 将 `l1_silent_egress` 挂载到目标网卡的 TC egress 钩子）

## 需求 4：抗重放归因（L2 - Go 用户空间）

- [x] 4. 抗重放归因
  - [x] 4.1 新建 `mirage-gateway/pkg/threat/nonce_store.go`：实现 `NonceStore` 结构体 — `NewNonceStore(maxSize int, ttl time.Duration) *NonceStore`（maxSize=100000, ttl=5min）；内部使用 `map[string]*nonceEntry`（key 为 nonce hex 编码）；`nonceEntry` 包含 SourceIP/Timestamp/CreatedAt 字段
  - [x] 4.2 修改 `mirage-gateway/pkg/threat/nonce_store.go`：实现 `CheckAndStore(nonce []byte, sourceIP string, timestamp time.Time) (isDuplicate bool, originalIP string)` 方法 — hex 编码 nonce 作为 key 查询 map，存在则返回 (true, entry.SourceIP)，不存在则存储并返回 (false, "")；容量超过 maxSize 时执行 LRU 淘汰（删除最早的 10% 条目）
  - [x] 4.3 修改 `mirage-gateway/pkg/threat/nonce_store.go`：实现 `StartCleanup(ctx context.Context)` 方法 — 启动 goroutine，每 30 秒遍历 entries，删除 CreatedAt 超过 ttl 的条目
  - [x] 4.4 修改 `mirage-gateway/pkg/threat/nonce_store.go`：实现 `CheckTimestamp(timestamp time.Time, maxSkew time.Duration) bool` 方法 — 检查 timestamp 与 `time.Now()` 的偏差是否超过 maxSkew（默认 30s），超过返回 false

## 需求 5：半开连接/慢速嗅探熔断（L2 - Go 用户空间）

- [x] 5. 半开连接熔断
  - [x] 5.1 新建 `mirage-gateway/pkg/threat/handshake_guard.go`：实现 `HandshakeGuard` 结构体 — `NewHandshakeGuard(timeout time.Duration, bl *BlacklistManager, rs *cortex.RiskScorer) *HandshakeGuard`（timeout=300ms）；内部维护 `ipCounters map[string]*hsCounter`（Count int, WindowStart time.Time）
  - [x] 5.2 修改 `mirage-gateway/pkg/threat/handshake_guard.go`：实现 `WrapListener(ln net.Listener) net.Listener` 方法 — 返回自定义 `guardedListener`，其 `Accept()` 方法在接受连接后调用 `conn.SetDeadline(time.Now().Add(hg.timeout))`，返回包装后的 `guardedConn`
  - [x] 5.3 修改 `mirage-gateway/pkg/threat/handshake_guard.go`：实现 `guardedConn` 结构体（嵌入 net.Conn）— 增加 `ClearDeadline()` 方法调用 `conn.SetDeadline(time.Time{})`，供 TLS 握手成功后调用
  - [x] 5.4 修改 `mirage-gateway/pkg/threat/handshake_guard.go`：实现 `onTimeout(sourceIP string)` 方法 — 递增 `ipCounters[sourceIP].Count`，窗口过期（>1min）则重置；Count > 5 时调用 `blacklist.Add(sourceIP+"/32", 1h, SourceLocal)` 和 `riskScorer.AddScore(sourceIP, 20, "handshake_timeout")`；递增 `HandshakeTimeoutTotal` Prometheus 指标

## 需求 6：非预期协议突发归因（L3 - Go 用户空间）

- [x] 6. 协议异常检测
  - [x] 6.1 新建 `mirage-gateway/pkg/threat/protocol_detector.go`：实现 `ProtocolDetector` 结构体 — `NewProtocolDetector(rs *cortex.RiskScorer, bl *BlacklistManager) *ProtocolDetector`；定义 `protocolSignatures` 变量：`map[string][]byte`（"ssh"→"SSH-", "http_get"→"GET ", "http_post"→"POST", "http_head"→"HEAD", "http_conn"→"CONN"）
  - [x] 6.2 修改 `mirage-gateway/pkg/threat/protocol_detector.go`：实现 `Detect(conn net.Conn) (isMalicious bool, protocolType string)` 方法 — 使用 `bufio.NewReader(conn)` 包装，调用 `Peek(8)` 读取前 8 字节（设置 100ms 读取超时），遍历 `protocolSignatures` 检查前缀匹配，匹配则返回 (true, protocolType)，不匹配返回 (false, "")；返回包装后的 buffered reader 供后续 TLS 握手使用
  - [x] 6.3 修改 `mirage-gateway/pkg/threat/protocol_detector.go`：实现 `HandleMalicious(sourceIP string, protocolType string)` 方法 — 调用 `riskScorer.AddScore(sourceIP, 40, "protocol_scan:"+protocolType)`；递增 `ProtocolScanTotal.WithLabelValues(gateway_id, protocolType)` Prometheus 指标

## 需求 7：时序与熵值行为基线监控（L3 - Go 用户空间）

- [x] 7. 行为基线监控
  - [x] 7.1 新建 `mirage-gateway/pkg/cortex/markov_model.go`：实现 `MarkovModel` 结构体 — `TransitionMatrix map[int]map[int]float64`（state → state → probability）；`States []int`（离散化包长区间：[0,64], [65,256], [257,512], [513,1024], [1025,1500]）；`DefaultBaseline() *MarkovModel` 返回内置合法流量基线
  - [x] 7.2 修改 `mirage-gateway/pkg/cortex/markov_model.go`：实现 `Deviation(observed []int) float64` 方法 — 将 observed 包长序列离散化为状态序列，统计观测转移概率矩阵，计算与基线的 KL 散度，归一化到 [0.0, 1.0] 区间（使用 `math.Min(klDiv/maxKL, 1.0)`）
  - [x] 7.3 新建 `mirage-gateway/pkg/cortex/behavior_monitor.go`：实现 `ConnProfile` 结构体 — ConnID/SourceIP/StartTime/PacketSizes([]int, 最近 1000 个)/SendBytes/RecvBytes/PacketCount/LastPacketAt/IntervalSum；`SendRecvRatio() float64` 返回 RecvBytes/SendBytes（SendBytes=0 时返回 0）；`DataEntropy() float64` 计算 PacketSizes 的 Shannon 熵
  - [x] 7.4 修改 `mirage-gateway/pkg/cortex/behavior_monitor.go`：实现 `BehaviorMonitor` 结构体 — `NewBehaviorMonitor(baseline *MarkovModel, threshold float64, rs *RiskScorer) *BehaviorMonitor`；`RecordPacket(connID, sourceIP string, size int, direction int)` 更新对应 ConnProfile；`RemoveConn(connID string)` 清理断开的连接
  - [x] 7.5 修改 `mirage-gateway/pkg/cortex/behavior_monitor.go`：实现 `StartMonitoring(ctx context.Context)` 方法 — 启动 goroutine，每 10 秒调用 `evaluate()`；`evaluate()` 遍历所有活跃 ConnProfile，调用 `baseline.Deviation(profile.PacketSizes)` 计算偏离度，超过 threshold 则调用 `onKick(connID)` + `riskScorer.AddScore(ip, 25, "behavior_anomaly")`；检查只发不收模式（SendRecvRatio < 0.05 且持续 > 30s）则同样踢下线

## 需求 8：威胁情报本地化富化

- [x] 8. 威胁情报富化
  - [x] 8.1 新建 `mirage-gateway/configs/cloud_ranges.json`：创建云厂商公开网段示例文件 — JSON 格式，包含 aws/azure/gcp/aliyun/tencent 五个 key，每个 key 对应 CIDR 数组（各放入 3-5 个代表性网段作为初始数据）
  - [x] 8.2 新建 `mirage-gateway/configs/asn_database.json`：创建 ASN 离线库示例文件 — JSON 数组格式，每个条目包含 network（CIDR）、asn（uint32）、org（string）、country（string）字段（放入 10-20 个代表性条目作为初始数据）

## 需求横切：Prometheus 指标与主程序集成

- [x] 9. 指标与集成
  - [x] 9.1 修改 `mirage-gateway/pkg/threat/metrics.go`：增加 7 个新 Prometheus 指标变量 — `ASNDropTotal`（counter, labels: gateway_id）、`RateLimitDropTotal`（counter, labels: gateway_id, trigger_type）、`SilentDropTotal`（counter, labels: gateway_id）、`HandshakeTimeoutTotal`（counter, labels: gateway_id）、`ProtocolScanTotal`（counter, labels: gateway_id, protocol）、`BehaviorAnomalyTotal`（counter, labels: gateway_id）、`ThreatIntelLookupTotal`（counter, labels: gateway_id, result）；在 `RegisterMetrics()` 中注册
  - [x] 9.2 修改 `mirage-gateway/configs/gateway.yaml`：增加 `defense` 配置段 — `l1.asn_blocklist_path`、`l1.cloud_ranges_path`、`l1.rate_limit`（syn_pps/conn_pps/enabled）、`l1.silent_response`（drop_icmp_unreachable/drop_tcp_rst/enabled）、`l2.nonce_store_size`（100000）、`l2.nonce_ttl_seconds`（300）、`l2.handshake_timeout_ms`（300）、`l3.behavior_check_interval_seconds`（10）、`l3.deviation_threshold`（0.7）
  - [x] 9.3 修改 `mirage-gateway/cmd/gateway/main.go`：在启动流程中按顺序初始化所有防御模块 — ① `NewThreatIntelProvider` 加载本地威胁情报库 ② `SyncASNBlocklist` 同步 ASN 网段到 eBPF Map ③ `SyncRateLimitConfig` 同步速率限制配置 ④ `SyncSilentConfig` 同步静默响应配置 ⑤ `NewL1Monitor` 启动 L1 事件监听 ⑥ `NewNonceStore` + `StartCleanup` ⑦ `NewHandshakeGuard` ⑧ `NewProtocolDetector` ⑨ `NewBehaviorMonitor` + `StartMonitoring` ⑩ 用 `HandshakeGuard.WrapListener` 包装 TLS Listener
