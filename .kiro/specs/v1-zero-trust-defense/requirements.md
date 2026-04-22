# 需求文档：零信任抗审计三层纵深防御

## 简介

本 Spec 对应 `零信任抗审与防守侧归因架构.md` 中三层纵深防御矩阵（L1/L2/L3）及威胁情报富化模块的完整实现。

当前 Gateway 已具备基础安全骨架（SecurityFSM、BlacklistManager、RiskScorer、IngressPolicy），但缺乏系统性的纵深防御能力：

1. L1 层无 ASN/网段级清洗，无 SYN Flood 速率限制，ICMP Unreachable 未被抑制
2. L2 层 mTLS 已强制（Spec 1-1），但缺乏抗重放归因和半开连接熔断
3. L3 层无协议异常检测，无时序/熵值行为基线监控
4. 威胁情报富化数据（ASN 库、云厂商 IP 库）未接入，无本地化闭环

核心原则：**Behavior Over Identity（行为优于身份）& Default-Drop（默认静默丢弃）**。

## 术语表

- **XDP**：eXpress Data Path，Linux 内核网卡驱动层的高性能数据包处理框架
- **TC**：Traffic Control，Linux 内核流量控制层，支持 ingress/egress 钩子
- **ASN**：Autonomous System Number，自治系统编号，用于标识网络运营商
- **LPM_Trie**：Longest Prefix Match Trie，eBPF Map 类型，用于 CIDR 前缀匹配
- **SYN_Flood**：利用 TCP 三次握手的半开连接耗尽服务器资源的 DDoS 攻击
- **mTLS**：双向 TLS 认证，Gateway 与 Client 之间必须互相验证证书
- **Nonce**：一次性随机数，用于防止重放攻击
- **Ring_Buffer**：eBPF 环形缓冲区，C 数据面向 Go 控制面上报事件的通道
- **SecurityFSM**：安全状态机，根据 SecurityMetrics 驱动 Normal/Alert/HighPressure/Isolated/Silent 状态迁移
- **RiskScorer**：IP 风险评分器，评分累加 + 时间衰减 + 自动封禁
- **BlacklistManager**：黑名单管理器，维护 CIDR → 过期时间映射，同步到 eBPF LPM Trie
- **DefenseApplier**：防御策略应用器，通过 eBPF Map 下发配置到数据面
- **Markov_Model**：马尔可夫链模型，用于建模合法流量的时序特征基线
- **Entropy**：信息熵，衡量数据随机性的指标，用于检测异常流量模式

## 需求

### 需求 1：静态 ASN/网段清洗（L1 - XDP 层）

**用户故事：** 作为安全工程师，我需要在网卡驱动层直接丢弃已知敌对 ASN 和云厂商数据中心 IP 的流量，以便在最早阶段消除资源枯竭攻击。

#### 验收标准

1. THE Gateway SHALL 在启动时从本地 ASN 离线库加载云厂商数据中心 IP 网段（AWS/Azure/GCP/Aliyun/Tencent Cloud）到 eBPF LPM Trie Map（`asn_blocklist_lpm`）
2. WHEN 入站数据包的源 IP 命中 `asn_blocklist_lpm` 时，THE XDP 程序 SHALL 执行 `XDP_DROP`，不产生任何响应
3. THE Go 控制面 SHALL 通过 eBPF Map 动态更新 `asn_blocklist_lpm` 条目，支持热加载新网段
4. THE ASN 离线库 SHALL 以本地文件形式存储（JSON/二进制），禁止在数据面处理连接时向外发起网络查询
5. THE Gateway SHALL 通过 Prometheus 指标 `mirage_asn_drop_total` 记录 ASN 清洗丢弃的数据包总数

### 需求 2：速率限制（L1 - XDP 层）

**用户故事：** 作为安全工程师，我需要在 XDP 层实现 SYN Flood 和高频连接的速率限制，以便在内核层阻止资源枯竭攻击。

#### 验收标准

1. THE XDP 程序 SHALL 维护每源 IP 的连接速率计数器（`rate_limit_map`，类型 LRU_HASH）
2. WHEN 单个源 IP 在 1 秒内的 SYN 包数量超过配置阈值（默认 50）时，THE XDP 程序 SHALL 对该 IP 后续 SYN 包执行 `XDP_DROP`
3. WHEN 单个源 IP 在 1 秒内的总连接尝试超过配置阈值（默认 200）时，THE XDP 程序 SHALL 对该 IP 后续包执行 `XDP_DROP`
4. THE Go 控制面 SHALL 通过 eBPF Map（`rate_config_map`）动态调整速率限制阈值
5. WHEN 速率限制触发时，THE XDP 程序 SHALL 通过 Ring Buffer 上报事件（源 IP、触发类型、当前速率），Go 控制面接收后累加 RiskScorer 评分
6. THE Gateway SHALL 通过 Prometheus 指标 `mirage_ratelimit_drop_total` 记录速率限制丢弃的数据包总数

### 需求 3：静默响应（L1 - XDP 层）

**用户故事：** 作为安全工程师，我需要 Gateway 禁止产生任何 ICMP Unreachable 回包，以便让攻击者的扫描器陷入等待超时的黑洞。

#### 验收标准

1. THE XDP 程序 SHALL 拦截所有出站 ICMP Destination Unreachable（Type 3）报文并执行 `XDP_DROP`
2. THE XDP 程序 SHALL 拦截所有出站 ICMP Port Unreachable（Type 3, Code 3）报文并执行 `XDP_DROP`
3. THE XDP 程序 SHALL 拦截所有出站 TCP RST 报文（对非已建立连接的 RST）并执行 `XDP_DROP`
4. THE Go 控制面 SHALL 通过 eBPF Map（`silent_config_map`）控制静默响应的启用/禁用
5. THE Gateway SHALL 通过 Prometheus 指标 `mirage_silent_drop_total` 记录静默丢弃的响应包总数

### 需求 4：抗重放归因（L2 - Go 用户空间）

**用户故事：** 作为安全工程师，我需要检测并封锁 DPI 截获合法握手包后的异地重放探测，以便阻止密码学层面的主动探测攻击。

#### 验收标准

1. THE Gateway SHALL 在 TLS 握手阶段记录每个连接的 Nonce 和 Timestamp 到本地缓存（`NonceStore`，容量上限 100000 条，LRU 淘汰）
2. WHEN 收到的握手 Nonce 在 NonceStore 中已存在时，THE Gateway SHALL 立即静默关闭连接并将源 IP 加入黑名单（TTL 2 小时）
3. WHEN 收到的握手 Timestamp 与本地时钟偏差超过 30 秒时，THE Gateway SHALL 静默关闭连接并通过 RiskScorer 累加 30 分
4. THE NonceStore SHALL 自动清理超过 5 分钟的过期条目
5. WHEN 重放攻击被检测时，THE Gateway SHALL 通过 Ring Buffer 上报 `ThreatReplayAttack` 事件，包含源 IP、重复 Nonce 前 8 字节、时间戳

### 需求 5：半开连接/慢速嗅探熔断（L2 - Go 用户空间）

**用户故事：** 作为安全工程师，我需要对未能在 300ms 内完成安全握手的连接执行静默 RST，以便防止半开连接资源耗尽和框架指纹泄漏。

#### 验收标准

1. THE Gateway SHALL 为每个新入站连接启动 300ms 握手超时计时器
2. WHEN 连接在 300ms 内未完成完整 TLS 握手时，THE Gateway SHALL 在 TCP 层静默发送 RST 并关闭连接
3. THE Gateway SHALL 在握手超时时不返回任何 TLS Alert 报文、HTTP 错误码或应用层错误信息
4. WHEN 同一源 IP 在 1 分钟内触发 5 次以上握手超时时，THE Gateway SHALL 将该 IP 加入黑名单（TTL 1 小时）并通过 RiskScorer 累加 20 分
5. THE Gateway SHALL 通过 Prometheus 指标 `mirage_handshake_timeout_total` 记录握手超时总数

### 需求 6：非预期协议突发归因（L3 - Go 用户空间）

**用户故事：** 作为安全工程师，我需要检测 443 端口上的非 TLS 协议探测报文（SSH 握手、HTTP 明文等），以便标记和封锁协议扫描器。

#### 验收标准

1. THE Gateway SHALL 在接受连接后、TLS 握手前，读取前 8 字节进行协议指纹检测
2. WHEN 前 8 字节匹配 SSH 协议签名（`SSH-`）时，THE Gateway SHALL 标记源 IP 为"协议扫描器"并静默关闭连接
3. WHEN 前 8 字节匹配 HTTP 明文请求（`GET ` / `POST` / `HEAD` / `CONN`）时，THE Gateway SHALL 标记源 IP 为"协议扫描器"并静默关闭连接
4. WHEN 源 IP 被标记为"协议扫描器"时，THE RiskScorer SHALL 累加 40 分（等同 DangerousFPScore）
5. THE Gateway SHALL 通过 Prometheus 指标 `mirage_protocol_scan_total` 按协议类型（ssh/http/unknown）记录协议扫描检测总数

### 需求 7：时序与熵值行为基线监控（L3 - Go 用户空间）

**用户故事：** 作为安全工程师，我需要对已建立连接的流量进行持续性行为画像，以便检测偏离正常模式的自动化扫描和异常流量。

#### 验收标准

1. THE Gateway SHALL 为每个活跃连接维护行为画像（`ConnProfile`），记录：包长分布直方图、收发比例、平均包间隔、数据熵值
2. WHILE 连接处于活跃状态时，THE Gateway SHALL 每 10 秒计算一次行为偏离度，与内置马尔可夫链基线模型比较
3. WHEN 行为偏离度超过阈值（默认 0.7）时，THE Gateway SHALL 在下一心跳周期强制断开连接并通过 RiskScorer 累加 25 分
4. WHEN 连接表现出"只发不收"模式（收发比例 < 0.05 持续 30 秒）时，THE Gateway SHALL 标记为自动化扫描并强制断开
5. THE Gateway SHALL 通过 Prometheus 指标 `mirage_behavior_anomaly_total` 记录行为异常检测总数

### 需求 8：威胁情报本地化富化

**用户故事：** 作为安全工程师，我需要将云厂商公开网段和 ASN/WHOIS 离线库加载到网关本地内存，以便在不发起外部查询的前提下对入站流量进行风险评分富化。

#### 验收标准

1. THE Gateway SHALL 在启动时从本地文件加载云厂商公开网段库（`cloud_ranges.json`）和 ASN 离线库（`asn_database.bin`）到内存
2. THE Gateway SHALL 提供 `ThreatIntelProvider` 接口，支持 `LookupASN(ip) → ASNInfo` 和 `IsCloudIP(ip) → (bool, CloudProvider)` 查询
3. WHEN 入站连接的源 IP 属于云厂商数据中心网段时，THE RiskScorer SHALL 自动累加 20 分（标记为高风险异常）
4. THE Gateway SHALL 禁止在数据面处理连接时向外发起任何同步网络查询（DNS/HTTP/WHOIS）
5. THE Gateway SHALL 支持通过控制面安全通道（OS 下发）热更新威胁情报库，更新后自动刷新内存缓存和 eBPF Map
6. THE Gateway SHALL 通过 Prometheus 指标 `mirage_threat_intel_lookup_total` 按结果类型（cloud_hit/asn_hit/clean）记录查询总数
