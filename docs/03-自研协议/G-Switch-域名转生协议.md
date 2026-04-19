# G-Switch 域名转生协议

## 一、协议定位

**存活维度防御**：秒级域名转生同步，解决基础设施被封锁后的快速恢复

- **技术栈**：Go 控制面（M.C.C.）
- **核心能力**：信令共振 + 盲寻址 + 热切换
- **语言分工**：纯 Go 实现（涉及 API 调用、数据库更新、Raft 一致性等高层逻辑）

---

## 二、协议架构

### 2.1 分层设计

```
┌─────────────────────────────────────────────────────────┐
│                   G-Switch 协议栈                        │
├─────────────────────────────────────────────────────────┤
│ 决策层 (M.C.C.)                                          │
│   ├─ 封锁检测                                            │
│   ├─ 域名选择                                            │
│   └─ 转生触发                                            │
├─────────────────────────────────────────────────────────┤
│ 一致性层 (Raft)                                          │
│   ├─ 提案 (Propose)                                      │
│   ├─ 投票 (Vote)                                         │
│   └─ 提交 (Commit)                                       │
├─────────────────────────────────────────────────────────┤
│ 扩散层                                                    │
│   ├─ 主信令通道（G-Tunnel 加密）                         │
│   ├─ 备用公告板（Twitter/GitHub/DNS TXT/IPFS）          │
│   └─ Tor Hidden Service                                 │
├─────────────────────────────────────────────────────────┤
│ 执行层 (Gateway)                                         │
│   ├─ 域名更新                                            │
│   ├─ eBPF SNI 切换                                       │
│   └─ 双栈共存                                            │
└─────────────────────────────────────────────────────────┘
```

---

## 三、核心机制

### 3.1 域名池管理

```go
type DomainPool struct {
    Active  []*Domain  // 活跃域名（3-5 个）
    Warm    []*Domain  // 温储备（10-15 个）
    Cold    []*Domain  // 冷储备（20-30 个）
    Queue   chan *Domain // 采购队列
}

// 域名状态
type DomainState int

const (
    STATE_COLD    DomainState = 0 // 已注册，未解析
    STATE_WARM    DomainState = 1 // 已解析，待命
    STATE_ACTIVE  DomainState = 2 // 正在使用
    STATE_DYING   DomainState = 3 // 即将报废
    STATE_DEAD    DomainState = 4 // 已报废
)

// 域名定义
type Domain struct {
    ID          string
    Name        string
    State       DomainState
    Registrar   string
    TLD         string
    CreatedAt   time.Time
    ActivatedAt time.Time
    DeathAt     time.Time
    HealthScore float64
    Metrics     *DomainMetrics
}

type DomainMetrics struct {
    PacketLoss   float64
    RTT          time.Duration
    RTTVariance  time.Duration
    ICMPCount    int
    DNSTime      time.Duration
}
```

### 3.2 封锁检测

```go
type BlockDetector struct {
    thresholds *Thresholds
}

type Thresholds struct {
    PacketLossRate float64       // 丢包率阈值：30%
    RTTVariance    time.Duration // RTT 方差阈值：100ms
    ICMPCount      int           // ICMP 不可达阈值：10 次
    Timeout        time.Duration // 超时阈值：5s
}

func (bd *BlockDetector) Detect(domain *Domain) BlockType {
    metrics := domain.Metrics
    
    // 1. DNS 污染
    if metrics.DNSTime > bd.thresholds.Timeout {
        return BLOCK_DNS_POISON
    }
    
    // 2. IP 封锁
    if metrics.PacketLoss > bd.thresholds.PacketLossRate {
        return BLOCK_IP_BLOCKED
    }
    
    // 3. 主动探测
    if metrics.ICMPCount > bd.thresholds.ICMPCount {
        return BLOCK_ACTIVE_PROBING
    }
    
    // 4. 选择性延迟
    if metrics.RTTVariance > bd.thresholds.RTTVariance && 
       metrics.PacketLoss < 0.05 {
        return BLOCK_SELECTIVE_DELAY
    }
    
    return BLOCK_NONE
}

type BlockType int

const (
    BLOCK_NONE             BlockType = 0
    BLOCK_DNS_POISON       BlockType = 1
    BLOCK_IP_BLOCKED       BlockType = 2
    BLOCK_ACTIVE_PROBING   BlockType = 3
    BLOCK_SELECTIVE_DELAY  BlockType = 4
)
```

---

## 四、Raft 一致性

### 4.1 提案流程

```go
type RaftProposal struct {
    Type      string    // "DOMAIN_SWITCH"
    OldDomain string
    NewDomain string
    Timestamp time.Time
    Reason    BlockType
}

func (m *MCC) switchDomain(trigger DomainSwitchTrigger) error {
    // 1. 从温储备池选择新域名
    newDomain := m.warmPool.SelectBest()
    
    // 2. 创建 Raft 提案
    proposal := RaftProposal{
        Type:      "DOMAIN_SWITCH",
        OldDomain: m.activeDomain.Name,
        NewDomain: newDomain.Name,
        Timestamp: time.Now(),
        Reason:    trigger.BlockType,
    }
    
    // 3. 提交到 Raft 集群
    future := m.raft.Apply(marshal(proposal), 5*time.Second)
    
    // 4. 等待确认（< 100ms）
    if err := future.Error(); err != nil {
        return err
    }
    
    // 5. 推送到所有 Gateway
    return m.broadcastToEdges(newDomain)
}
```

### 4.2 扩散性能

| 节点数 | 扩散延迟 | 一致性保证 | 网络开销 |
|--------|---------|-----------|---------|
| 10 个 | < 200ms | Raft Quorum | 10 KB |
| 100 个 | < 800ms | 最终一致性 | 100 KB |
| 1000 个 | < 3s | 分层扩散 | 1 MB |

---

## 五、信令共振

### 5.1 多路广播

```go
type SignalingBroadcaster struct {
    primary   *PrimaryChannel   // G-Tunnel 加密
    fallbacks []FallbackChannel // 备用公告板
}

// 主信令通道
type PrimaryChannel struct {
    gtunnel *GTunnel
    cipher  cipher.AEAD
}

func (pc *PrimaryChannel) Broadcast(domain *Domain) error {
    // 1. 加密域名信息
    plaintext := marshal(domain)
    encrypted := pc.cipher.Seal(nil, nonce, plaintext, nil)
    
    // 2. 通过 G-Tunnel 发送
    return pc.gtunnel.Send(encrypted)
}

// 备用公告板
type FallbackChannel interface {
    Publish(domain *Domain) error
    Fetch() (*Domain, error)
}

// Twitter/X 公告板
type TwitterBoard struct {
    client *twitter.Client
    account string
}

func (tb *TwitterBoard) Publish(domain *Domain) error {
    // 1. 加密域名
    encrypted := encrypt(domain)
    
    // 2. Base64 编码
    encoded := base64.StdEncoding.EncodeToString(encrypted)
    
    // 3. 更新个人简介
    return tb.client.UpdateBio(tb.account, encoded)
}

// GitHub 公告板
type GitHubBoard struct {
    client *github.Client
    repo   string
}

func (gb *GitHubBoard) Publish(domain *Domain) error {
    // 1. 加密域名
    encrypted := encrypt(domain)
    
    // 2. 创建 Commit
    message := fmt.Sprintf("Update config: %s", 
        base64.StdEncoding.EncodeToString(encrypted))
    
    return gb.client.CreateCommit(gb.repo, message, nil)
}

// DNS TXT 公告板
type DNSTXTBoard struct {
    domains []string
}

func (db *DNSTXTBoard) Publish(domain *Domain) error {
    // 1. 加密域名
    encrypted := encrypt(domain)
    
    // 2. 分片（DNS TXT 限制 255 字节）
    fragments := splitToFragments(encrypted, 255)
    
    // 3. 更新 TXT 记录
    for i, frag := range fragments {
        record := fmt.Sprintf("v=mirage%d %s", i, frag)
        db.updateTXT(db.domains[i], record)
    }
    
    return nil
}

// IPFS 公告板
type IPFSBoard struct {
    ipfs *ipfs.Client
    cid  string
}

func (ib *IPFSBoard) Publish(domain *Domain) error {
    // 1. 加密域名
    encrypted := encrypt(domain)
    
    // 2. 上传到 IPFS
    newCID, err := ib.ipfs.Add(encrypted)
    if err != nil {
        return err
    }
    
    // 3. 更新 CID（通过 IPNS）
    return ib.ipfs.Publish(ib.cid, newCID)
}
```

### 5.2 Gateway 扫描

```go
// Gateway 失联时自动扫描公告板
func (gw *Gateway) scanPublicBoards() (*Domain, error) {
    // 1. Twitter/X 个人简介
    if domain := gw.scanTwitterProfile("@mirage_sys"); domain != nil {
        return domain, nil
    }
    
    // 2. GitHub Commit 历史
    if domain := gw.scanGitHubCommits("mirage-project/public"); domain != nil {
        return domain, nil
    }
    
    // 3. DNS TXT 记录
    if domain := gw.scanDNSTXT([]string{
        "fallback1.example.com",
        "fallback2.example.com",
    }); domain != nil {
        return domain, nil
    }
    
    // 4. IPFS 公开 CID
    return gw.scanIPFS("QmXxx...")
}

// 解密并重组域名碎片
func (gw *Gateway) reassembleDomain(fragments []string) *Domain {
    // 1. Shamir 重组（3-of-5）
    combined := shamir.Combine(fragments)
    
    // 2. ChaCha20 解密
    plaintext := gw.cipher.Decrypt(combined)
    
    // 3. 反序列化
    var domain Domain
    unmarshal(plaintext, &domain)
    
    return &domain
}
```

---

## 六、DNS-less 连接

### 6.1 eBPF 拦截

```c
// eBPF 程序：拦截 getaddrinfo 系统调用
SEC("uprobe/libc:getaddrinfo")
int intercept_getaddrinfo(struct pt_regs *ctx) {
    char *hostname = (char *)PT_REGS_PARM1(ctx);
    
    // 1. 查询 M.C.C. 下发的 IP 映射表
    struct ip_mapping *mapping = bpf_map_lookup_elem(&dns_override_map, hostname);
    
    if (mapping) {
        // 2. 直接返回预置 IP（绕过 DNS）
        struct addrinfo *result = (struct addrinfo *)PT_REGS_PARM4(ctx);
        result->ai_addr = &mapping->ip;
        
        // 3. 跳过原始 DNS 查询
        bpf_override_return(ctx, 0);
    }
    
    return 0;
}

// BPF Map：存储域名 → IP 映射
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10000);
    __type(key, char[256]);   // 域名
    __type(value, struct ip_mapping);
} dns_override_map SEC(".maps");

struct ip_mapping {
    __u32 ip;
    __u64 timestamp;
    __u32 ttl;
};
```

### 6.2 Go 实现

```go
type DNSlessResolver struct {
    ipMapping map[string][]net.IP
    cipher    cipher.AEAD
    ebpf      *ebpf.Manager
}

// M.C.C. 下发加密映射
func (m *MCC) pushIPMapping(domain string, ips []net.IP) {
    mapping := IPMapping{
        Domain:    domain,
        IPs:       ips,
        Timestamp: time.Now(),
        TTL:       3600, // 1 小时
    }
    
    // 1. 加密
    plaintext := marshal(mapping)
    encrypted := m.cipher.Seal(nil, nonce, plaintext, nil)
    
    // 2. 通过 G-Tunnel 信令通道下发
    m.signalChannel.Broadcast(encrypted)
}

// Gateway 本地解析（绕过系统 DNS）
func (gw *Gateway) resolve(domain string) net.IP {
    // 1. 查询本地加密映射表
    if ips, ok := gw.dnsless.ipMapping[domain]; ok {
        return ips[rand.Intn(len(ips))] // 随机负载均衡
    }
    
    // 2. Fallback：通过 DoH (DNS over HTTPS) 查询
    return gw.dohResolver.Lookup(domain)
}

// 更新 eBPF Map
func (gw *Gateway) updateDNSOverride(domain string, ip net.IP) error {
    mapping := IPMapping{
        IP:        ip,
        Timestamp: time.Now().Unix(),
        TTL:       3600,
    }
    
    return gw.ebpf.UpdateMap("dns_override_map", domain, mapping)
}
```

---

## 七、热切换

### 7.1 后台预握手

```go
func (gw *Gateway) backgroundHandshake(domain string) *Connection {
    // 1. DNS-less 解析
    ip := gw.dnsless.Resolve(domain)
    
    // 2. QUIC 0-RTT 连接
    conn := gw.quic.Connect(ip, &quic.Config{
        Use0RTT: true,
    })
    
    // 3. 预热连接（发送心跳）
    conn.SendHeartbeat()
    
    return conn
}
```

### 7.2 逐步迁移

```go
func (gw *Gateway) gradualMigration(old, new *Connection, steps int) {
    for i := 0; i <= steps; i++ {
        ratio := float64(i) / float64(steps) // 0.0 → 1.0
        
        // 1. 调整流量分配比例
        gw.setTrafficRatio(old, 1.0-ratio)
        gw.setTrafficRatio(new, ratio)
        
        // 2. 每步间隔 3 秒
        time.Sleep(3 * time.Second)
    }
}

func (gw *Gateway) setTrafficRatio(conn *Connection, ratio float64) {
    // 更新 eBPF Map 控制流量分配
    gw.ebpf.UpdateMap("traffic_ratio_map", conn.ID, uint32(ratio*100))
}
```

### 7.3 双栈共存

```go
func (gw *Gateway) onDomainUpdate(newDomain string) {
    // 1. 添加新域名（双栈共存 60 秒）
    gw.addDomain(newDomain, ttl=60*time.Second)
    
    // 2. eBPF 内核态无缝切换 SNI
    gw.ebpf.UpdateSNITarget(newDomain)
    
    // 3. 60 秒后移除旧域名
    time.AfterFunc(60*time.Second, func() {
        gw.removeDomain(gw.oldDomain)
    })
}
```

---

## 八、性能指标

### 8.1 切换性能

| 阶段 | 延迟增加 | 丢包率 | 用户感知 |
|------|---------|--------|---------|
| 预握手（0-5s） | 0ms | 0% | 无感知 |
| 重叠传输（5-35s） | < 10ms | 0% | 无感知 |
| 逐步迁移（35-65s） | < 5ms | 0% | 无感知 |
| 完成切换（65s+） | 0ms | 0% | 无感知 |

### 8.2 DNS-less 性能

| 方案 | DNS 查询延迟 | 被污染风险 | 隐蔽性 |
|------|-------------|-----------|--------|
| 系统 DNS | 50-200ms | 高 | 低 |
| DoH/DoT | 100-300ms | 中 | 中 |
| eBPF 内核拦截 | < 1ms | 无 | 极高 |

---

## 九、配置示例

```yaml
gswitch:
  # 域名池
  domain_pool:
    active_size: 5
    warm_size: 15
    cold_size: 30
    auto_purchase: true
  
  # 封锁检测
  block_detection:
    packet_loss_threshold: 0.30
    rtt_variance_threshold: 100ms
    icmp_count_threshold: 10
    timeout_threshold: 5s
  
  # Raft 一致性
  raft:
    cluster_size: 5
    election_timeout: 1s
    heartbeat_interval: 500ms
  
  # 信令共振
  signaling:
    primary: gtunnel
    fallbacks:
      - twitter
      - github
      - dns_txt
      - ipfs
  
  # DNS-less
  dnsless:
    enabled: true
    ebpf_intercept: true
    doh_fallback: true
  
  # 热切换
  hot_switch:
    enabled: true
    background_handshake: true
    gradual_migration_steps: 10
    dual_stack_duration: 60s
```

---

## 十、实现参考

```go
// pkg/gswitch/switcher.go
package gswitch

type Switcher struct {
    mcc        *MCC
    raft       *Raft
    domainPool *DomainPool
    detector   *BlockDetector
    broadcaster *SignalingBroadcaster
}

func NewSwitcher(mcc *MCC) *Switcher {
    return &Switcher{
        mcc:        mcc,
        raft:       NewRaft(mcc.Nodes),
        domainPool: NewDomainPool(),
        detector:   NewBlockDetector(),
        broadcaster: NewSignalingBroadcaster(),
    }
}

func (s *Switcher) Start() {
    // 1. 启动封锁检测
    go s.monitorDomains()
    
    // 2. 启动域名采购
    go s.domainPool.AutoPurchase()
    
    // 3. 启动 Raft 集群
    s.raft.Start()
}

func (s *Switcher) monitorDomains() {
    ticker := time.NewTicker(10 * time.Second)
    
    for range ticker.C {
        for _, domain := range s.domainPool.Active {
            // 检测封锁
            blockType := s.detector.Detect(domain)
            
            if blockType != BLOCK_NONE {
                // 触发转生
                s.switchDomain(DomainSwitchTrigger{
                    Type:      "block_detected",
                    BlockType: blockType,
                })
            }
        }
    }
}
```
