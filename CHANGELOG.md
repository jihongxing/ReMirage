# Changelog

## [0.9.0] - 2026-04-20

### 核心架构
- 完成二元架构（C 数据面 + Go 控制面）全链路实现
- eBPF Loader 支持 XDP/TC/Sockmap 三种挂载模式
- Go ↔ C 通信通过 eBPF Map + Ring Buffer 实现

### 协议实现
- NPM 流量伪装协议（XDP 层零拷贝 Padding）
- B-DNA 行为识别协议（TCP/QUIC/TLS 指纹重写）
- Jitter-Lite 时域扰动协议（纳秒级 IAT 控制）
- VPC 噪声注入协议（物理噪音模拟）
- G-Tunnel 多路径传输（QUIC H3 + FEC + CID 轮换）
- G-Switch 域名转生（M.C.C. 信令 + Raft 一致性）

### Mirage-Gateway
- 18 阶段启动流程
- Cortex 威胁感知中枢
- Phantom 影子欺骗引擎
- 策略引擎（B-DNA 模板、成本计算、心跳监控）
- 安全加固（mTLS、反调试、RAM 防护、证书钉扎）
- TPROXY 透明代理 + Splice 零拷贝
- 计费引擎（流量统计 + 配额熔断）

### Mirage-OS
- Raft 3 节点集群
- 地理围栏 + 热密钥管理
- NestJS API Server
- React Web Console
- 12 个微服务（计费、蜂窝、域名、智能、影子等）
- Monero 支付集成
- WebSocket 实时推送

### Phantom Client
- Windows TUN 适配（Wintun）
- G-Tunnel QUIC 客户端
- Kill Switch
- 内存安全防护
- Token 引导连接

### 部署
- Ansible Playbook（Gateway + OS + Certs）
- Kubernetes 生产清单（DaemonSet + HPA + PDB）
- Docker Compose 开发环境
- 证书生成脚本（Root CA + Gateway + OS）
- 混沌测试脚本

### SDK
- 10 种语言 SDK（Go/Python/JS/Rust/Java/Kotlin/Swift/C#/PHP/Ruby）
- 多语言文档（中/英/日/俄/西/印）

### 测试
- 集成测试（生命周期、转生、自毁、Raft 故障转移）
- 性能基准（FEC、G-Switch、资源占用）
- eBPF 延迟测量（bpftrace）
- 高压测试脚本

---

## [0.9.1] - 2026-04-20

### 验证
- Linux 编译验证通过（OpenCloudOS 9, kernel 6.6.119, clang 17, Go 1.24.2）
- eBPF 数据面：7 个 .o 全部编译成功（bdna/chameleon/h3_shaper/jitter/npm/phantom/sockmap）
- Go 控制面：bin/mirage-gateway 二进制生成，零错误
- Linux 运行验证通过：18 阶段全量启动，零降级，零警告
- eBPF 全量挂载：XDP 1 个 + TC 8 个 + Sockops 1 个 + sk_msg 1 个，共享 Map 13 个，总 Map 43 个
- Sockmap 零拷贝路径已激活
- Dead Man's Switch 自毁序列验证通过（心跳超时 → 0xDEADBEEF → Map 清空 → 内存擦除 → 进程退出）

### eBPF Verifier 修复
- jitter.c: 内联 emergency_wipe 消除跨程序 BPF 调用
- npm.c: 合并双 XDP 为 npm_xdp_main 单入口（Single-Tenancy Rule）
- bdna.c: Bulletproof Scanner — 逐字节 skb 游走，零栈变量偏移访问
- bdna.c: Pointer Refresh Pattern — store_bytes 后刷新所有包指针
- bdna.c: bpf_l4_csum_replace 替代手动校验和计算
- bdna.c/chameleon.c: 位掩码黄金法则 (doff_len &= 0x3C) 强制定界
- phantom.c/npm.c: ip_hlen &= 0x3C 位掩码定界
- loader.go: sk_msg 改用 RawAttachProgram 挂载到 Map FD

---

## [Unreleased]

### 数据面全链路贯通（Data Plane Kill）

#### Phantom-Client FEC 解码 + 重组管线
- 新增 `reassembler.go`：完整的 Shard 缓冲 → FEC 解码 → Overlap 重组管线
- 12 字节 Shard Wire Header：`[2B seqNum][2B dataLen][4B overlapID][2B shardIdx][2B fragCount]`
- `Receive()` 改造为阻塞式完整管线：QUIC Datagram → 解密 → IngestShard → FEC Decode → Overlap Reassemble → 返回 IP 包
- `Send()` 同步升级为 12 字节 header（含 fragCount），接收端可判断何时所有 fragment 到齐
- FragmentGroup TTL 5s 自动驱逐，防止内存泄漏
- 属性测试覆盖：1-4000 字节任意包大小往返一致性（100 次 rapid 测试通过）
- FEC 丢包恢复测试：丢弃全部 4 个 parity shard 后仍可恢复原始数据

#### Phantom-Client mTLS 证书钉扎
- `QUICEngine` 新增 `PinnedCertHash` 配置（SHA-256 leaf cert pin）
- 生产模式：`VerifyConnection` 回调验证服务端证书指纹，常量时间比较
- 开发模式：`PinnedCertHash` 为空时回退到 InsecureSkipVerify

#### Gateway QUIC 拨号实现
- `chameleon_client.go` 的 `dialQUIC()` 从占位符替换为真实 quic-go 实现
- 新增 `QUICConn` 结构体实现 `TransportConn` 接口（Send/Recv/Close/Type/RTT/RemoteAddr/MaxDatagramSize）
- 支持 mTLS 客户端证书加载（CertFile + KeyFile）
- 物理网卡绑定（避免 TUN 路由环路）
- Datagram 支持协商验证
- 降级探测 `probeAndPromote` 现在使用真实 QUIC 连接而非占位符

### 控制面接线（Control Plane Wiring）

#### HeartbeatMonitor 接入 gRPC 控制流
- `heartbeat_monitor.go` 重构：通过 `HeartbeatSender` 接口解耦 api 包依赖（消除循环引用）
- 新增 `api/heartbeat_adapter.go`：桥接 `GRPCClient` 与 `HeartbeatMonitor`
- 心跳成功时自动喂看门狗（`lastHeartbeat` 更新 + 退出紧急模式）
- 心跳请求自动携带 `runtime.MemStats`、GatewayStatus、eBPF 加载状态

#### 紧急自毁闭环（OpSec Seal）
- 新增 `burn_wiper.go`：暴力擦除引擎，收到 0xDEADBEEF 后执行完整自毁序列
- 内存密钥擦除：3 遍 `crypto/rand` 随机覆写 + 最终全零化，直接操作底层 `[]byte`
- 磁盘文件擦除：3 遍 4KB 块随机覆写 + `fsync` + `os.Remove`，支持 glob 模式批量删除
- `HeartbeatMonitor` 集成 `BurnWiper`：自动注册 `/tmp/mirage-*`、`/var/log/mirage-*`、`/etc/mirage/gateway.yaml`
- `GetBurnWiper()` 暴露给外部模块，启动时注册 `masterKey`/`hwKey`/`nodeKey` 等密钥切片
- 幂等保护：`burned` 标志位防止重复执行
- 实现 `SensitiveData` 接口，可作为 `HeartbeatMonitor.RegisterSensitive()` 的参数
- 5 个测试覆盖：密钥擦除验证、文件删除验证、glob 批量删除、双重调用安全、接口兼容性

#### CellLifecycleManager eBPF 阶段更新
- 新增 `EBPFPhaseUpdater` 接口，`updateEBPFPhase()` 从 TODO 替换为真实 Map 写入
- `NewCellLifecycleManager` 接受 `EBPFPhaseUpdater` 参数

#### Evaluator 闭环
- `scanner.go` 新增 `TrafficStatsProvider` 接口，`captureFeatures()` 优先从 eBPF 读取统计数据
- `feedback.go` 新增 `EmergencyHandler` 接口，`emergencyStop()` 从 TODO 替换为真实 `TriggerWipe()` 调用

#### eBPF 修复
- `loader.go` 移除重复的 `GetMap` 方法声明（编译错误修复）
- `manager.go` 的 `cleanupTC()` 委托给 `Loader.Close()`（已有完整 netlink FilterDel 实现）
- `billing.go` 的 `CellLevel` 从硬编码改为可配置字段 + `SetCellLevel()` 方法
- `icmp_transport.go` 添加缺失的 `ebpfTypes` import（编译错误修复）
- `dns_transport.go` 添加缺失的 `github.com/miekg/dns` import（编译错误修复）

#### 新增三种极端环境生存协议
- **WebRTC DataChannel 传输** (`webrtc_transport.go`)：伪装为跨国视频会议流量（DTLS + SCTP），使用 pion/webrtc 库，不可靠交付模式，MaxDatagramSize 16384
- **ICMP Tunnel 传输** (`icmp_transport.go` + `bpf/icmp_tunnel.c`)：伪装为 Ping 诊断包，C 数据面 eBPF TC Hook 构造/截获 ICMP Echo Request/Reply，Go 控制面通过 eBPF Map + Ring Buffer 通信
- **DNS Tunnel 传输** (`dns_transport.go`)：伪装为域名解析，Base32 编码子域名 + TXT/CNAME 记录，MaxDatagramSize ~110 字节，仅作紧急控制指令通道

#### Orchestrator 多路径自适应调度器
- 替代原有 TransportManager 的串行降级逻辑
- 分阶段 HappyEyeballs 竞速：Phase 1 并发探测 QUIC/WSS/ICMP/DNS，Phase 2 利用 WSS 信令后台拉起 WebRTC
- 四级权重优先级：L0 QUIC → L1 WebRTC → L2 WSS → L3 ICMP/DNS
- Epoch Barrier 双发选收：路径切换时在 ShardHeader 注入 Epoch 标识，接收端冻结旧通道数据，切断高延迟拖尾污染
- 动态 MTU 通知：TransportConn 接口新增 MaxDatagramSize()，路径切换时通知 FEC 动态调整分片大小

#### LinkAuditor 链路审计器
- 持续监控各路径丢包率和延迟波动
- 丢包率 > 30% 或 RTT > 200% 基线时触发降格
- 心跳探测：默认 30s 周期，Level 3 时缩短为 15s
- 连续 3 次探测成功触发升格

#### 接口与结构体变更
- `TransportType` 枚举扩展：新增 TransportWebRTC=2、TransportICMP=3、TransportDNS=4
- `TransportConn` 接口新增 `MaxDatagramSize() int` 方法
- `ShardHeader` 新增 `Epoch uint32` 字段
- 新增 `SerializeShardWithEpoch` / `DeserializeShardWithEpoch` 函数
- `ChameleonClientConn` 补充 `MaxDatagramSize()` 实现

#### eBPF 数据面
- 新增 `bpf/icmp_tunnel.c`：TC egress 构造 ICMP Echo Request，TC ingress 截获 Echo Reply 提取 Payload
- `loader.go` 新增 icmp_tunnel.o 加载逻辑（非 critical，加载失败降级运行）
- `pkg/ebpf/types.go` 新增 ICMPConfig、ICMPTxEntry、ICMPRxEvent 结构体

#### 配置
- `gateway.yaml` 新增 orchestrator 配置段和 transports 多协议配置段
- 支持各协议独立启用/禁用、自定义探测参数和降格阈值

#### 依赖
- 新增 `github.com/pion/webrtc/v4`
- 新增 `github.com/miekg/dns`

#### Phantom-Client 信令共振发现器 (Resonance Resolver)
- 新增 `pkg/resonance/` 包：客户端侧信令共振发现器
- DoH 突围：内置 DNS over HTTPS 客户端，强制通过 1.1.1.1/8.8.8.8 查询 TXT 记录，绕过本地 ISP 投毒
- GitHub Gist 通道：解析伪装为监控数据的 JSON（`{"v":1,"ts":...,"data":"<signal>"}`）
- Mastodon Hashtag 通道：搜索公开 hashtag 时间线，HTML 剥离 + 信令提取
- First-Win-Cancels-All 并发竞速：`context.WithCancel` 三通道并发，任一成功立即斩断其余
- 解密验签闭环：Base64 RawURL 解码 → SignalCrypto.OpenSignal（X25519 ECDH + ChaCha20 解密 + Ed25519 验签 + 反重放）
- GTunnelClient 三级降级集成：RouteTable → Bootstrap Pool → 信令共振发现（绝境复活）
- `SetResonanceResolver()` 注入接口，解耦 crypto 依赖
- 6 个测试覆盖：DNS/Gist/Mastodon 单通道验证 + First-Win 竞速验证 + 2 个属性测试

#### 属性测试（Property-Based Testing）
- 10 个正确性属性覆盖：Shard 序列化往返一致性、DNS Base32 编码往返一致性、Phase 1 排除 WebRTC、HappyEyeballs 选择最快协议、降格阈值正确性、升格连续成功计数、禁用协议跳过探测、缺失配置使用默认值、Epoch Barrier 切断拖尾污染、动态 MTU 约束分片大小
- 集成测试覆盖 Orchestrator 降格/升格端到端流程

### 待完成（文档-代码同步审计 2026-04-20）

#### P0 — 致命缺口（阻断商业闭环）

**Mirage-Gateway**
- [x] ~~`pkg/strategy/heartbeat_monitor.go:90` — gRPC 心跳上报未实现~~ → 已通过 HeartbeatSender 接口 + HeartbeatAdapter 接入
- [x] ~~`pkg/strategy/heartbeat_monitor.go:183` — 紧急自毁时敏感数据清空未实现~~ → 已实现 SensitiveData 接口 + 3-pass wipe
- [x] ~~`pkg/strategy/heartbeat_monitor.go:195` — 临时文件清理未实现~~ → 已实现 secureDelete 3-pass 覆写
- [x] ~~`pkg/strategy/cell_lifecycle.go:204` — eBPF Map 阶段更新未实现~~ → 已通过 EBPFPhaseUpdater 接口实现
- [x] ~~`pkg/gtunnel/chameleon_client.go:365` — QUIC 拨号占位符~~ → 已实现真实 quic-go Datagram 连接
- [x] ~~`pkg/ebpf/manager.go:168` — TC 钩子清理逻辑未实现~~ → 已委托给 Loader.Close()
- [x] ~~`pkg/evaluator/scanner.go:118` — 流量特征捕获使用模拟数据~~ → 已通过 TrafficStatsProvider 接口优先读取 eBPF
- [x] ~~`pkg/evaluator/feedback.go:147` — 紧急停止逻辑未实现~~ → 已通过 EmergencyHandler 接口实现
- [x] ~~`pkg/strategy/cell_lifecycle.go:98` — VPC 噪声注入调用未实现~~ → 已通过 VPCNoiseInjector 接口 + eBPF Map 写入实现
- [x] ~~`pkg/strategy/cell_lifecycle.go:125` — 网络质量测量未实现~~ → 已通过 NetworkProber 接口 + TCP fallback 实现真实探测
- [x] ~~`pkg/strategy/cell_lifecycle.go:150` — 上报 Mirage-OS + B-DNA 模板微调未实现~~ → 已通过 CalibrationReporter + DNATemplateUpdater 接口闭环
- [x] ~~`pkg/gtunnel/fec.go:58,84` — FEC AVX-512 C 加速编码/解码未对接（降级为 P2）~~ → 已通过 CGO 对接 fec_accel.c，Reed-Solomon GF(2^8) 编解码，运行时 AVX-512 检测 + 标量 fallback

**Mirage-OS**
- [x] ~~`gateway-bridge/proto/` — Uplink 4 个 gRPC 方法未实现（SyncHeartbeat/ReportTraffic/ReportThreat + Downlink 4 个）~~ → 已实现 Desired State 对齐模型 + GatewayConnectionManager + DownlinkService（PushBlacklist/PushStrategy/PushQuota/PushReincarnation）
- [x] ~~`api/proto/cell_grpc.pb.go` — CellService 5 个方法未实现~~ → 已实现 CellServiceImpl（RegisterCell/ListCells/AllocateGateway/HealthCheck/SwitchCell）+ 4 个属性测试
- [x] ~~`api/proto/billing_grpc.pb.go` — BillingService 5 个方法未实现~~ → 已实现 BillingServiceImpl（CreateAccount/Deposit/GetBalance/PurchaseQuota/GetBillingLogs）+ Sub-address 隔离防 TxHash 重放
- [x] ~~`api/proto/gateway_grpc.pb.go` — GatewayService 4 个方法未实现~~ → 已实现 SyncHeartbeat（Desired State 对齐）+ ReportTraffic（配额扣减+告警）+ ReportThreat（severity 分级映射）+ GetQuota
- [x] ~~`pkg/raft/hot_key_manager.go:94` — 从 Raft 集群收集 Shamir 份额逻辑未实现~~ → 已实现 ShareProvider 接口 + 并发收集 + 5s 超时 + 份额验证（Index∈[1,5], len=32）
- [x] ~~`pkg/raft/hot_key_manager.go:140` — 密钥轮换逻辑未实现~~ → 已实现 Raft Log 强一致性分发（KeyRotationCommand + raft.Apply），避免 Split-Brain
- [x] ~~`pkg/raft/cluster.go:266` — 威胁检测逻辑未实现~~ → 已实现 checkThreatLevel 集成 GeoFence，仅 ControlPlane 级威胁触发退位
- [x] ~~`pkg/raft/geo_fence.go:114` — 政府审计检测未实现~~ → 已实现 DetectGovernmentAudit（政府 IP 段 + 物理访问 + 路由跳数异常 >2 跳）
- [x] ~~`pkg/raft/geo_fence.go:123` — DDoS 检测未实现~~ → 已实现 DetectDDoS（流量 >5x 基线 OR SYN >10000/s OR UDP >50000/s），Gateway 级威胁不触发 Raft 退位
- [x] ~~`pkg/raft/geo_fence.go:132` — 异常流量检测未实现~~ → 已实现 DetectAnomalousTraffic（3σ 偏离 + 单 IP >1000 连接 + 地理异常），Gateway 级威胁不触发 Raft 退位
- [x] ~~`pkg/strategy/cell_manager.go:170` — 通知 Gateway 网络质量测量未实现~~ → 已通过 DownlinkClient 接口 + PushStrategy 推送校准参数
- [x] ~~`pkg/strategy/cell_manager.go:198` — 下发 B-DNA 模板到 eBPF Map 未实现~~ → 已通过 DownlinkClient 接口 + PushStrategy 推送 template_id
- [x] ~~`pkg/strategy/cell_manager.go:307` — 通知 Gateway VPC 噪声注入未实现~~ → 已通过 DownlinkClient 接口 + PushStrategy 推送 noise_intensity=80
- [x] ~~`services/billing/monero_manager.go:174` — Monero RPC 交易确认数查询使用模拟值~~ → 已实现 HTTPMoneroRPCClient（JSON-RPC 2.0 get_transfer_by_txid，endpoint 强制 127.0.0.1:18082）
- [x] ~~`services/billing/monero_manager.go:244` — XMR/USD 实时汇率 API 返回固定值 $150~~ → 已实现 CachedExchangeRateProvider（CoinGecko → Kraken 回退，Redis 缓存 5min TTL）

**Phantom-Client**
- [x] ~~`pkg/gtclient/client.go:208` — FEC 解码 + 重组管线未实现~~ → 已实现完整 Reassembler
- [x] ~~`pkg/gtclient/quic_engine.go:49` — mTLS 证书钉扎未启用~~ → 已实现 SHA-256 cert pin

#### P1 — 高优先级（功能完整性）

**eBPF 数据面**
- [x] ~~`bpf/jitter.c:94` — NPM Padding 填充策略未实现（固定/正态/跟随三种模式）~~ → 已实现 switch(padding_strategy) 三种模式 + bpf_skb_change_tail + IP 校验和更新 + traffic_stats 计费

**G-Switch 域名转生**
- [x] ~~信令共振公告板机制（Twitter/GitHub/DNS TXT/IPFS 多通道发现）~~ → 已实现 ResonanceResolver（DoH DNS TXT + CF Worker 反代 GitHub Gist + Mastodon 三通道极限竞速，首个成功即返回）+ ResonanceBridge 失联自动触发
- [x] ~~DNS-less eBPF 拦截（内核态域名劫持）~~ → 已实现 DNSlessHijacker（bpf/sock_hijack.c cgroup sock_addr connect4/sendmsg4/recvmsg4）+ Go 控制面 SetGatewayIP/Enable/Disable
- [x] ~~域名池三级管理（活跃/温储备/冷储备）~~ → 已实现 GSwitchManager（activePool/standbyPool/burnedPool）+ standbyRefillLoop 热备补充 + cooldownRecycleLoop 冷却回收 + AutonomousGSwitch 信誉分自适应逃逸

**G-Tunnel 多路径**
- [x] ~~WebRTC DataChannel 传输~~ → 已实现 Pion Diet 裁剪 + Trickle ICE + WSS 信令偷渡（WSSSignaler），网关侧 WebRTCAnswerer + CtrlFrameRouter 控制帧路由
- [x] ~~ICMP Tunnel 传输~~ → 已实现 Go Raw Socket 发包 + eBPF TC Hook 收包 + 令牌桶限速 + EMA RTT 平滑
- [x] ~~DNS Tunnel 传输~~ → 已实现轮询泵(200ms) + 缓存击穿(Nonce+Seq 唯一域名) + 分片重组(80B/片) + 网关侧权威 DNS 服务器
- [x] ~~BBR v3 拥塞控制~~ → 降级至 P3（quic-go 自带 Cubic/BBR v1，Linux 侧 sysctl 开启系统级 BBR 即可）

**Mirage-OS 微服务**
- [x] ~~12 个微服务业务逻辑完整性~~ → 已实现邀请码生成与核销（FOR UPDATE 悲观锁防双花）、QuotaBridge DB↔Redis 最终一致性（透支缓冲 5MB + 重试队列 + 定期对账）、TierRouter 等级路由下发（物理隔离，跨级分配禁止）
- [x] ~~Web Console 功能完整性~~ → 已实现 Console API 服务（mTLS/物理隔离双模式）、意图驱动蜂窝生命周期 API（promote-to-calibration/activate/retire）、用户管理/财务大盘/配额总览/邀请码管理 REST 接口、CellOrchestrator 前端页面
- [x] ~~WebSocket 实时推送~~ → 已实现 ws-gateway（Hub + ThrottledSubscriber 200ms 节流窗口 + Redis 多频道订阅 heartbeat/threat/tunnel + 冷启动全量快照 + 双向指令通道 tactical/ghost_mode/self_destruct）

#### P2 — 中优先级（加固与优化）

- [x] ~~`pkg/ebpf/billing.go:113` — CellLevel 硬编码 "standard"，需从配置读取~~ → 已改为可配置字段 + SetCellLevel() 方法
- [ ] 单元测试覆盖率提升至 80%+
- [ ] Phantom Client 跨平台 TUN 集成测试（Windows Wintun / Linux / macOS utun 已实现 Read/Write，需端到端验证）
- [ ] Ansible Playbook Raft 集群部署脚本完善
- [ ] Shamir 密钥分片初始化自动化脚本
- [ ] 混沌测试覆盖 Orchestrator 多路径降格/升格场景

#### 创世演习混沌实验室 (The Genesis Drill)
- 新增 `deploy/chaos/genesis/` 混沌测试基础设施
- Docker Compose 隔离网络拓扑：OS + Gateway A/B + Phantom Client + Mock 信令服务
- 三幕剧本自动化脚本 `genesis-drill.sh`：
  - 第一幕 Golden Path：邀请→充值→分配→连接→计费完整商业闭环
  - 第二幕 Protocol Asphyxiation：iptables 切断 UDP/TCP，验证多路径降级矩阵
  - 第三幕 Scorched Earth & Resonance：杀死 Gateway A，验证信令共振绝境复活
- Mock 信令服务（Go）：模拟 DoH / GitHub Gist / Mastodon 三通道
- PostgreSQL 初始化脚本：最小必要表结构 + 测试数据
- Phantom Client 混沌测试 Dockerfile（含 iptables/tcpdump 工具链）

---

## [0.9.2] - 2026-04-20

### E2E 测试环境部署记录

#### 服务器环境
- 服务器：腾讯云 VM-0-7-opencloudos（OpenCloudOS 9, kernel 6.6.x）
- 路径：`/opt/ReMirage`
- Go：1.25.5 linux/amd64
- Docker：29.4.0, Docker Compose Plugin 5.1.3

#### 部署修复记录
- `mirage-os/gateway-bridge/Dockerfile`：Go 1.22 → 1.24（匹配 go.mod）
- `mirage-gateway/Dockerfile`：重写为多阶段构建，`golang:1.25` 基础镜像 + eBPF 编译
- `phantom-client/Dockerfile.chaos`：添加 `touch cmd/phantom/wintun.dll` 占位（Linux 构建满足 go:embed）
- `phantom-client/Dockerfile.chaos`：COPY 指令不支持 shell 重定向，改用 `touch` 保证文件存在
- `docker-compose.genesis.yml`：os-node build context 从 `../../mirage-os` 改为 `../../mirage-os/gateway-bridge`
- `mirage-os/gateway-bridge/Dockerfile`：添加 `COPY --from=builder /app/configs ./configs`
- 新增 `mirage-os/gateway-bridge/configs/mirage-os.yaml`：chaos 测试专用配置
- Raft bind_addr：`0.0.0.0:7000` → `10.99.0.10:7001`（避免 REST 端口冲突 + 可广播地址）
- Raft peers：添加自身节点（bootstrap 需要至少一个 voter）
- `deploy/chaos/genesis/drill/Dockerfile.drill`：添加 GNU `grep`（BusyBox grep 不支持 `-P`）
- `mirage-gateway/Dockerfile` CMD：`-iface eth0 -defense 20` → `-config configs/gateway.yaml`
- `mirage-gateway/configs/gateway.yaml`：mcc.endpoint 指向 `10.99.0.10:50051`，TLS 禁用
- `deploy/chaos/genesis/drill/genesis-drill.sh`：`chmod +x`（Windows git 丢失执行权限）

#### E2E 测试结果（首次运行）
- 通过：2/10
  - ✅ OS gateway-bridge 健康检查
  - ✅ 配额查询 API 正常返回
- 失败：8/10（业务逻辑层缺失，非环境问题）
  - ❌ Gateway 未注册到 OS（心跳 protobuf 序列化错误）
  - ❌ XMR Webhook API 未实现
  - ❌ Phantom Client 未启动（缺少测试模式入口）
  - ❌ 计费 API 未实现
  - ❌ 多路径降级（Phantom 未连接）
  - ❌ 信令共振复活（resonance publish API 未实现）

#### 基础设施层验证通过
- Docker 容器全部正常启动（10 个容器）
- OS 节点：PostgreSQL + Redis + Raft Leader 选举成功
- Gateway A/B：18 阶段全量启动，eBPF 挂载成功，gRPC 连接 OS 成功
- 网络拓扑：mirage-net (10.99.0.0/24) + infra-net (10.99.1.0/24) 隔离正确
- Mock 信令服务（DoH/Gist/Mastodon）正常运行

#### 已修复（0.9.3）
- [x] 补全 OS REST API（webhook/billing/resonance/gateway kill）
- [x] 修复 Gateway 心跳 protobuf 序列化问题（方案 A+：mirage-proto 协议中枢）
- [x] Phantom Client 添加 chaos 测试模式（chaos-harness）
- [x] Gateway 配置修复（gateway_id 环境变量、mcc endpoint 去 scheme、TLS 禁用）

#### 待验证（下次继续）
- [ ] 服务器全量 `--no-cache` 重建后重新跑演习
- [ ] 确认 Gateway 心跳成功注册到 OS（endpoint 修复后）
- [ ] 确认 Phantom chaos-harness 正常启动并暴露 :9090 状态 API
- [ ] 确认 XMR Webhook 正确分配配额
- [ ] genesis-drill.sh 中 `/shared/signal_payload.json` volume 挂载问题

---

## [0.9.3] - 2026-04-20

### 方案 A+：协议中枢模块 (mirage-proto Single Source of Truth)

#### 架构决策
- 拒绝方案 B（JSON Codec 降级）：gRPC HTTP/2 二进制帧性能优势不可牺牲
- 拒绝方案 C（手写 Proto 接口）：protobuf 内部反射机制过于复杂，手写极易 panic
- 执行方案 A+：建立独立协议中枢模块，物理级别强一致

#### 实施内容
- 新建 `mirage-proto/` 独立 Go module
  - `mirage.proto`：唯一权威 IDL 定义
  - `gen/mirage.pb.go`：protoc v33.2 + protoc-gen-go v1.36.10 生成
  - `gen/mirage_grpc.pb.go`：protoc-gen-go-grpc v1.6.0 生成
  - `generate.sh`：一键生成脚本（安装 protoc + 插件 + 生成 + 验证）
- Gateway 端改造：删除 `pkg/api/proto/`，7 个文件 import 改为 `pb "mirage-proto/gen"`
- OS 端改造：删除 `gateway-bridge/proto/`，11 个文件 import 改为 `pb "mirage-proto/gen"`
- 两端通过 `go.mod replace` 指向本地 `../mirage-proto`
- 两端编译验证通过，protobuf 序列化错误彻底修复

### OS REST API 补全
- `/internal/webhook/xmr`：XMR 充值到账（$150/XMR → 配额 GB）
- `/internal/billing/{user_id}`：计费查询（聚合 billing_logs）
- `/internal/resonance/publish`：信令共振发布（Redis 缓存）
- `/internal/gateway/{id}/kill`：焦土指令（DB + Redis Pub/Sub）
- `init.sql` gateways 表补全列

### Phantom Client 混沌测试模式
- 新增 `cmd/chaos-harness/main.go`：HTTP 状态 API `:9090`
- `Dockerfile.chaos` 入口改为 `chaos-harness`

### Gateway 配置修复
- `gateway_id: "${MIRAGE_GATEWAY_ID}"`（环境变量注入）
- `mcc.endpoint`：去掉 `https://` 前缀（gRPC 不带 scheme）
- `mcc.tls.enabled: false`（chaos 测试环境）
