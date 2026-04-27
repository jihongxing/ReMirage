# 需求文档：外部零特征消除

## 简介

本需求文档覆盖 Mirage 项目外部零特征消除审计清单中所有仍开放的整改项。目标是消除被动观察者（pcap 分析）和主动探测者（发包探测）能识别的所有强特征，使 Mirage 流量在外部网络观察者视角下与正常浏览器流量不可区分。

整改范围涵盖三个层级：架构级结构阻断（Layer S）、外部被动观察强特征（Layer A）、外部主动探测强特征（Layer B）。按 P0（上线阻断）→ P1（短期修复）→ P2（中期修复）分优先级实施。

## 术语表

- **Orchestrator**：Gateway 侧多路径编排器，负责 QUIC/WSS/WebRTC/DNS/ICMP 五协议的统一调度与切换（`mirage-gateway/pkg/gtunnel/orchestrator.go`）
- **ClientOrchestrator**：Client 侧编排器，负责 Client 端多路径调度（`phantom-client/pkg/gtclient/client_orchestrator.go`）
- **MultiPathBuffer**：G-Tunnel 双发选收缓冲器，在路径切换期间同时向新旧路径发送数据并去重（`mirage-gateway/pkg/gtunnel/multipath.go`）
- **QUICEngine**：Client 侧 QUIC 连接引擎，管理到 Gateway 的 QUIC Datagram 连接（`phantom-client/pkg/gtclient/quic_engine.go`）
- **SessionShaper**：Client 侧会话级时序去相关器，当前仅做批量窗口发送（`phantom-client/pkg/gtclient/session_shaper.go`）
- **ChameleonListener**：Gateway 侧 WebSocket over TLS 降级监听器（`mirage-gateway/pkg/gtunnel/chameleon_fallback.go`）
- **StrategyEngine**：Gateway 侧策略引擎，根据威胁等级自动调整防御参数（`mirage-gateway/pkg/strategy/engine.go`）
- **GSwitchManager**：G-Switch 域名转生管理器，执行域名切换与热备管理（`mirage-gateway/pkg/gswitch/manager.go`）
- **B-DNA**：行为指纹识别协议，eBPF TC 层重写 TCP/QUIC 握手指纹（`mirage-gateway/bpf/bdna.c`）
- **NPM**：流量伪装协议，eBPF XDP 层追加 Padding 消除包长特征（`mirage-gateway/bpf/npm.c`）
- **Jitter-Lite**：时域扰动协议，eBPF TC 层控制 `skb->tstamp` 实现 IAT 控制（`mirage-gateway/bpf/jitter.c`）
- **VPC**：噪声注入协议，eBPF TC 层模拟物理网络延迟特征（`mirage-gateway/bpf/jitter.c` 内 VPC 相关函数）
- **ALPN**：Application-Layer Protocol Negotiation，TLS/QUIC 握手中的应用层协议协商字段
- **JA3/JA4/JA4Q**：TLS/QUIC 客户端指纹哈希算法，基于握手参数计算
- **uTLS**：Go 语言 TLS 指纹拟态库，可模拟真实浏览器的 TLS ClientHello
- **bearer listener**：承载真实用户数据面流量的网络监听器（区别于 gRPC 控制面监听器）
- **bearer path**：从进程 socket → 网卡 ifindex → hook 点的完整数据面路径
- **DPI**：深度包检测，通过分析数据包内容识别协议和应用
- **IAT**：Inter-Arrival Time，包间到达时间间隔
- **SPL**：Sequence of Packet Lengths，包长序列
- **epoch**：Orchestrator 路径切换的版本号，每次切换递增
- **CID rotation**：QUIC Connection ID 轮换，切换时更新连接标识

## 需求

### 需求 1：Orchestrator 原子切换事务（S-02）

**用户故事：** 作为 Gateway 运维人员，我希望 Orchestrator 的路径切换是原子化的事务操作，以确保切换期间不丢包且外部观察者无法通过流量中断识别切换行为。

**技术约束**：
- 现有 `MultiPathBuffer.EnableDualSend` 接受 `*Path`（含 `*net.UDPConn`），而 Orchestrator 使用 `ManagedPath`（含 `TransportConn` 接口）。两套类型体系不兼容，不能直接调用。
- Orchestrator 的 `mpBuffer` 字段虽然存在但从未初始化（`NewOrchestrator` 中未赋值）。
- quic-go 公开 API 中没有 `RetireConnectionID` 方法，CID rotation 在当前栈中不可直接实现。
- 实施路径二选一：(a) 新建基于 `TransportConn`/`ManagedPath` 的 `SwitchBuffer` 专门服务编排事务；(b) 先将 `MultiPathBuffer` 抽象成不依赖 `*Path`/`*net.UDPConn` 的接口层，再让 Orchestrator 接入。

#### 验收标准

1. WHEN Orchestrator 执行 demote 操作, THE Orchestrator SHALL 启动双发选收，在新旧路径上同时发送数据
2. WHEN Orchestrator 执行 promote 操作, THE Orchestrator SHALL 启动双发选收，在新旧路径上同时发送数据
3. WHILE 双发选收模式处于活跃状态, THE Orchestrator SHALL 对两条路径的接收数据进行去重后交付
4. WHEN 双发选收窗口结束, THE Orchestrator SHALL 执行 epoch barrier 递增，将 Send/Recv 收敛到新 activePath
5. WHEN 路径切换事务完成, THE Orchestrator SHALL 通知 FEC 层调整 MTU 参数以匹配新路径特性
6. IF 双发选收期间新路径不可用, THEN THE Orchestrator SHALL 回滚到旧路径并保持 epoch 不变
7. CID rotation 记录为技术债务，不在当前 spec 中承诺实现（quic-go 公开 API 不支持）

### 需求 2：Client QUIC ALPN 修正（A-02a）

**用户故事：** 作为安全工程师，我希望 Client 的 QUIC 握手 ALPN 字段使用标准浏览器值，以确保被动观察者无法通过单包 ALPN 检测识别 Mirage 流量。

#### 验收标准

1. THE QUICEngine SHALL 在 TLS 配置中将 NextProtos 设置为 `["h3"]`
2. WHEN QUICEngine 建立 QUIC 连接, THE QUICEngine SHALL 在 QUIC Initial 包中携带 ALPN 值 `h3`
3. THE QUICEngine SHALL 不在任何握手字段中包含 `mirage`、`gtunnel` 或其他自定义协议标识符

### 需求 3：Client QUIC 指纹完整对齐（A-02b）

**用户故事：** 作为安全工程师，我希望 Client 的 QUIC Transport Parameters 与真实浏览器一致，以确保被动观察者无法通过 QUIC 参数差异识别 Mirage 流量。

**技术约束**：当前 Client 使用 `quic.Dial(ctx, udpConn, remoteAddr, *tls.Config, *quic.Config)` 建立连接，不是基于 TCP/TLS 的握手栈。uTLS 库（`utls.UClient`）仅适用于 TLS over TCP，不能直接用于 QUIC over UDP。因此 QUIC 指纹对齐必须通过 quic-go 原生 API 实现，分为"当前栈可做"和"当前栈做不了"两层。

#### 验收标准 — 当前栈可做（立即）

1. THE QUICEngine SHALL 将 `quic.Config.MaxIdleTimeout` 设置为 30s（Chrome 140+ 一致）
2. THE QUICEngine SHALL 将 `quic.Config.InitialPacketSize` 设置为 1200（Chrome 标准）
3. THE QUICEngine SHALL 将 `quic.Config.InitialStreamReceiveWindow` 和 `InitialConnectionReceiveWindow` 设置为与 Chrome 140+ 一致的值
4. WHEN QUICEngine 建立连接, THE QUICEngine SHALL 通过 quic-go 的 `quic.Config` 可用字段对齐 Chrome 行为
5. WHEN QUICEngine 建立连接, THE QUICEngine SHALL 模拟真实浏览器的 CID 更新频率和长度

#### 验收标准 — 当前栈做不了（需要上游支持或替换方案，记录为技术债务）

6. THE QUICEngine SHOULD 在 quic-go 支持 Transport Parameters Hook 后，注入完整的 Chrome QUIC Transport Parameters（`initial_max_data`、`max_udp_payload_size` 等非 Config 字段）
7. 在上游支持到位前，THE QUICEngine SHALL 通过抓包验收确认当前 quic-go 默认 Transport Parameters 与 Chrome 的差异程度，并记录差异清单

### 需求 4：Client 用户态源头特征消除（A-03）

**用户故事：** 作为安全工程师，我希望 Client 在用户态从源头消除流量特征，以确保在没有 eBPF 的终端设备上也能实现包长和 IAT 分布拟态。

**技术约束**：当前 `client.Send()` 的出网路径是 `Split → FEC Encode → encrypt → transport.SendDatagram(encrypted)`。SessionShaper 当前未被实例化使用。外部观察者看到的是加密后的 QUIC Datagram，因此 Padding 和 IAT 控制必须作用在"加密后、真正调用 `transport.SendDatagram()` 前"的边界，而不是在 SessionShaper 内部对明文操作。

#### 验收标准

1. THE Client SHALL 在 `client.Send()` 内部的 `transport.SendDatagram(encrypted)` 调用前，插入统一的 send-path shim 层，对加密后的 datagram 进行 Padding 和 IAT 控制
2. THE send-path shim SHALL 对加密后的 datagram 追加随机字节 Padding，使包长分布符合目标正态分布
3. THE send-path shim SHALL 支持可配置的目标包长分布参数（均值、标准差）
4. THE send-path shim SHALL 支持可控 IAT 分布注入（正态分布和指数分布两种模式）
5. IF Padding 导致数据包超过 QUIC Datagram MTU, THEN THE send-path shim SHALL 将 Padding 截断到 MTU 上限
6. THE send-path shim SHALL 同时适用于 `transport.SendDatagram` 和 `quicEngine.SendDatagram` 两条发送路径

### 需求 5：协同链闭环验收（A-04）

**用户故事：** 作为系统架构师，我希望数据流经过完整的协议协同链 `B-DNA → G-Tunnel → NPM → Jitter-Lite → VPC`，以确保端到端的零特征目标可验收。

#### 验收标准

1. WHEN Gateway 接收到 Client 数据, THE Gateway SHALL 使数据依次经过 B-DNA、G-Tunnel、NPM、Jitter-Lite、VPC 处理链
2. THE Gateway SHALL 在生产态启动真实公网 QUIC/H3 bearer listener，使默认主路径的 bearer path 固化到可验收的进程/socket/ifindex/hook
3. WHEN Client 发送数据, THE Client SHALL 使数据经过用户态 Padding 和 IAT 控制后再调用 transport.SendDatagram
4. THE Gateway SHALL 确保 NPM/B-DNA/Jitter-Lite/VPC 的 eBPF 程序挂载在正确的 bearer path 上（与公网 QUIC/H3 listener 对应的网卡 ifindex）

### 需求 6：WSS 降级路径接入 uTLS（A-05）

**用户故事：** 作为安全工程师，我希望 WSS 降级路径使用 uTLS 建立 TLS 连接，以确保被动观察者无法通过 JA3/JA4 指纹识别 Go 原生栈。

#### 验收标准

1. WHEN DialChameleon 建立 WSS 连接, THE ChameleonClient SHALL 使用 dialWithUTLS 函数建立底层 TCP+TLS 连接
2. WHEN dialWithUTLS 建立连接, THE ChameleonClient SHALL 生成与 Chrome 一致的 TLS ClientHello 指纹
3. THE ChameleonClient SHALL 在 uTLS 连接之上建立 WebSocket 协议

### 需求 7：Client 源头指纹生成（A-06）

**用户故事：** 作为安全工程师，我希望 Client 从源头生成正确的 TLS/QUIC 指纹，以确保靠近 Client 一侧的被动观察者无法识别 Go 原生栈特征。

**技术约束**：uTLS 仅适用于 TLS over TCP（如 WSS 降级路径），不能直接用于 QUIC over UDP。Client QUIC 指纹对齐需通过 quic-go 原生 Config 字段实现（见需求 3）。A-06 拆为两条独立任务线：Client 源头指纹生成（主线）和 Gateway NFQUEUE/用户态补充重写（补线）。

#### 验收标准 — 主线：Client 源头指纹生成

1. THE QUICEngine SHALL 通过 quic-go 原生 `*tls.Config` 和 `*quic.Config` 的可用字段，尽可能对齐 Chrome 的 QUIC 参数
2. THE QUICEngine SHALL 记录"可控字段清单"（如 `NextProtos`、`MinVersion`、`MaxVersion`）和"不可控字段差异清单"（如 TLS 1.3 CipherSuites 顺序不可配置、`CurvePreferences` 顺序不保证生效），不再使用"与 Chrome 对齐"的绝对表述
3. THE ChameleonClient SHALL 在 WSS 降级路径使用 uTLS 生成 Chrome 一致的 TLS ClientHello（此处 uTLS 可用，因为 WSS 是 TLS over TCP）
4. THE QUICEngine SHALL 通过 pcap 抓包验收确认当前 QUIC ClientHello 与 Chrome 的实际差异，并记录验收门槛

#### 验收标准 — 补线：Gateway NFQUEUE/用户态补充重写

4. THE Gateway SHALL 实现基于 NFQUEUE 或 iptables REDIRECT 的用户态拦截层，对 Gateway 出站 TCP/TLS 流量进行指纹重写
5. THE Gateway 用户态拦截层 SHALL 读取 B-DNA 的 `skb->mark` 标记，对标记的连接执行实际指纹重写
6. 补线仅覆盖 Gateway 出站方向，不覆盖 Client 出站方向

### 需求 8：B-DNA 指纹模板扩展（A-07）

**用户故事：** 作为安全工程师，我希望 B-DNA 指纹模板覆盖更多浏览器版本，以确保长期观察者无法通过聚类分析识别有限的指纹集合。

#### 验收标准

1. THE B-DNA SHALL 将 eBPF 指纹模板从当前 6 个扩展到至少 30 个
2. THE B-DNA SHALL 覆盖 Chrome、Firefox、Safari、Edge 各主要版本的指纹
3. THE B-DNA SHALL 支持通过 eBPF Map 动态更新指纹模板，无需重新加载 eBPF 程序

### 需求 9：NPM 默认模式固化与运行时断言（A-08）

**用户故事：** 作为运维人员，我希望 NPM 的运行时默认值始终为 Gaussian 模式，并有断言防止回落到 fixed MTU 模式。

**技术约束**：`npm_config_map` 的 value 类型是完整的 `NPMConfig` 结构体（包含 `Enabled`、`FillingRate`、`GlobalMTU`、`MinPacketSize`、`PaddingMode`、`DecoyRate` 六个字段），不是单独的 `uint32` mode。断言逻辑必须读取完整结构体后检查 `PaddingMode` 字段。

#### 验收标准

1. THE NPM Go 控制面 SHALL 保持 `NewDefaultNPMConfig()` 的 PaddingMode 为 `NPMModeGaussian`
2. WHEN NPM eBPF 程序加载完成, THE NPM Go 控制面 SHALL 从 eBPF Map 中读取完整 `NPMConfig` 结构体，检查其 `PaddingMode` 字段是否为 `NPMModeGaussian`
3. IF `NPMConfig.PaddingMode` 不是 `NPMModeGaussian`, THEN THE NPM Go 控制面 SHALL 记录错误日志，将 `PaddingMode` 修正为 `NPMModeGaussian` 后写回完整结构体

### 需求 10：B-DNA 非 SYN 包一致性（A-09）

**用户故事：** 作为安全工程师，我希望 B-DNA 对非 SYN 包也维护 Window Size 一致性，以确保高级 DPI 无法通过 SYN 声明与实际行为的差异识别伪装。

#### 验收标准

1. THE B-DNA eBPF 程序 SHALL 对连接建立后的前 N 个数据包（N 可配置，默认 10）维护 Window Size 与 SYN 声明的一致性
2. WHEN B-DNA 重写 SYN 包的 Window Size, THE B-DNA SHALL 将目标 Window Size 存入 per-connection eBPF Map
3. WHILE 连接处于前 N 个包阶段, THE B-DNA SHALL 读取 per-connection Map 中的目标 Window Size 并应用到数据包

### 需求 11：Jitter-Lite 高斯采样修正（A-10）

**用户故事：** 作为安全工程师，我希望 Jitter-Lite 的 IAT 采样使用真正的高斯近似，以确保被动观察者无法通过 IAT 分布的均匀特征识别 Mirage 流量。

#### 验收标准

1. THE Jitter-Lite eBPF 程序 SHALL 将 `common.h` 中的 `gaussian_sample` 函数从当前的均匀分布实现替换为 Irwin-Hall 近似（4 个均匀分布求和再缩放）
2. WHEN gaussian_sample 被调用, THE Jitter-Lite SHALL 返回符合正态分布特征的采样值（均值和标准差与参数一致）

### 需求 12：VPC 延迟分布模型修正（A-11）

**用户故事：** 作为安全工程师，我希望 VPC 的光缆抖动和跨洋模拟使用正确的统计分布，以确保被动观察者无法通过延迟分布特征识别模拟流量。

#### 验收标准

1. THE VPC eBPF 程序 SHALL 将 `simulate_fiber_jitter_v2` 中的均匀分布替换为指数分布近似
2. THE VPC eBPF 程序 SHALL 将 `simulate_submarine_cable` 中的三角波替换为多频率叠加伪随机波形
3. WHEN VPC 计算光缆抖动, THE VPC SHALL 生成符合指数分布特征的延迟值

### 需求 13：生产态 QUIC/H3 Bearer Listener（B-01b）

**用户故事：** 作为 Gateway 运维人员，我希望 Gateway 在生产态具备真实的 QUIC/H3 数据面监听器，以确保主动探测者发送标准 HTTP/3 请求时 Gateway 能正确响应。

#### 验收标准

1. THE Gateway SHALL 在 `main.go` 中创建生产态 QUIC/H3 bearer listener，监听 443/UDP
2. WHEN 外部客户端发送 QUIC Initial with ALPN=h3, THE Gateway SHALL 正确完成 QUIC 握手
3. WHEN 外部客户端发送标准 HTTP/3 请求, THE Gateway SHALL 返回合法 HTTP 响应（403 或 404）
4. THE Orchestrator SHALL 将数据面绑定到此 QUIC/H3 bearer listener

### 需求 14：UDP/QUIC 公网数据面主动探测防护（B-01c）

**用户故事：** 作为安全工程师，我希望公网 QUIC/H3 数据面具备主动探测防护能力，以确保主动探测者无法通过异常握手行为识别 Gateway。

**技术约束**：现有 `HandshakeGuard`（`WrapListener(net.Listener)`）和 `ProtocolDetector`（`Detect(net.Conn)`）是 TCP `net.Listener`/`net.Conn` 包装器，依赖 `Accept()` 后的流式读首字节。QUIC/H3 公网入口是 UDP 包，不是 TCP 连接，不能直接复用现有防护模块。

防护分两层：
- **第一层：UDP 首包预过滤**（目标：非法 Initial 不回包）— 在 `quic.Listener` 之前，对 UDP 首包做轻量级 QUIC Initial 格式校验，不合法的直接丢弃
- **第二层：Accept 后 ConnectionState 校验**（目标：协商异常快速关闭 + 风险评分）— 在 `quic.Listener.Accept()` 返回 `*quic.Conn` 后，检查 `ConnectionState().TLS.NegotiatedProtocol` 等协商结果

#### 验收标准

1. THE Gateway SHALL 新建第一层 UDP 首包预过滤模块，在 `quic.Listener` 之前对 UDP 首包做 QUIC Initial 格式校验（Version/DCID 长度/Token 合理性），不合法的直接丢弃不回包
2. THE Gateway SHALL 新建第二层 Accept 后校验模块，在 `quic.Listener.Accept()` 返回 `*quic.Conn` 后，检查 `ConnectionState().TLS.NegotiatedProtocol` 是否为 `h3`，非 h3 则快速关闭连接
3. WHEN 第二层检测到协商异常, THE Gateway SHALL 与现有 `BlacklistManager` 和 `RiskScorer` 集成，对频繁异常的 IP 进行风险评分和黑名单处理
4. THE Gateway SHALL 保持现有 TCP `HandshakeGuard`/`ProtocolDetector` 继续挂在 gRPC 控制面 listener 上，不做修改

### 需求 15：DNS/ICMP/WebRTC 接入生产启动链（B-01d）

**用户故事：** 作为 Gateway 运维人员，我希望 DNS、ICMP、WebRTC 传输层接入生产启动链，以确保冷备协议在需要时可用。

**技术约束**：三类冷备协议在 Gateway 侧的形态完全不同：
- **DNS**：Gateway 侧是 `DNSServer`（被动监听，接口为 `NewDNSServer` → `SetRecvCallback` → `Start` → `SendToClient`），不是客户端侧的 `DNSTransport`
- **WebRTC**：Gateway 侧是 `WebRTCAnswerer`（被动应答，依赖 WSS 信令通道回传 SDP Answer/ICE Candidate），不能在启动时立即创建，必须在 WSS ServerConn 就绪后通过控制帧触发
- **ICMP**：`ICMPTransport` 是主动 transport（Go Raw Socket 发送 + eBPF Ring Buffer 接收），不是被动 listener，需要 eBPF Map 注入和 Raw Socket 权限

#### 验收标准

1. THE Gateway SHALL 在 `main.go` 的生产启动链中调用 `NewDNSServer(domain, listenAddr)` 创建 DNS 服务端，通过 `SetRecvCallback` 注册收包回调将上行数据喂给 Orchestrator 的 `FeedInboundPacket`，调用 `Start()` 启动监听
2. THE Gateway SHALL 在 `main.go` 中预注册 WebRTC WSS 控制帧处理器，在 ChameleonListener/WSS ServerConn 就绪后，通过 WSS 控制帧触发 `NewWebRTCAnswerer(config, sendCtrl)` → `HandleOffer` → `HandleRemoteCandidate` → `WaitReady` 流程
3. THE Gateway SHALL 在 `main.go` 中按 `NewICMPTransport(configMap, txMap, rxRingbuf, config)` 初始化 ICMP 主动 transport，通过 `Orchestrator.AdoptInboundConn` 注册为可用路径
4. WHEN 上述任一冷备协议启动失败, THE Gateway SHALL 记录告警但不阻断 Gateway 启动

### 需求 16：配额熔断渐进式降级（B-02）

**用户故事：** 作为安全工程师，我希望配额耗尽时流量渐进衰减而非瞬间归零，以确保主动探测者无法通过精确到字节的流量截断识别 Gateway。

#### 验收标准

1. WHEN 剩余配额低于总配额的 10%, THE Jitter-Lite eBPF 程序 SHALL 将通过概率降低到 50%
2. WHEN 剩余配额低于总配额的 1%, THE Jitter-Lite eBPF 程序 SHALL 将通过概率降低到 10%
3. THE Jitter-Lite eBPF 程序 SHALL 使用概率丢弃（`bpf_get_prandom_u32`）而非硬截断实现降级
4. WHEN 配额从充足状态进入降级状态, THE Jitter-Lite SHALL 确保流量衰减过渡期大于 5 秒

### 需求 17：策略引擎参数随机偏移（B-03）

**用户故事：** 作为安全工程师，我希望策略引擎的防御参数在每个等级内引入随机偏移，以确保主动探测者无法通过离散阶梯变化精确判断防御等级。

**技术约束**：当前 `GetParams()` 被状态切换日志直接调用（`engine.go:74`）。随机化不能放在 `GetParams()` 读接口上（否则每次读取都不同，难以验证且引入额外噪声），应改为"状态生成一次，状态内复用"模式。

#### 验收标准

1. WHEN StrategyEngine 切换防御等级, THE StrategyEngine SHALL 在切换时一次性生成带 ±20% 随机偏移的 DefenseParams 并缓存
2. WHEN GetParams 被调用, THE StrategyEngine SHALL 返回当前缓存的已偏移参数（同一等级内多次调用返回相同值）
3. THE StrategyEngine SHALL 确保随机偏移后的参数值不低于 0
4. WHEN StrategyEngine 再次切换等级（即使切换到相同等级）, THE StrategyEngine SHALL 重新生成随机偏移值

### 需求 18：双发模式时间随机化（B-04）

**用户故事：** 作为安全工程师，我希望 G-Tunnel 双发模式的持续时间随机化，以确保主动探测者无法通过固定 100ms 的流量突增窗口识别域名切换。

#### 验收标准

1. WHEN MultiPathBuffer 启用双发选收模式, THE MultiPathBuffer SHALL 将 dualModeDuration 设置为 [80ms, 200ms] 范围内的随机值
2. THE MultiPathBuffer SHALL 在每次启用双发模式时重新生成随机持续时间
3. THE MultiPathBuffer SHALL 使用加密安全的随机数生成器（`crypto/rand`）

### 需求 19：G-Switch 域名格式多样化（B-05）

**用户故事：** 作为安全工程师，我希望 G-Switch 生成的临时域名使用多种格式，以确保 DNS 监控无法通过固定的 `{16位hex}.cdn.example.com` 模式识别域名切换。

#### 验收标准

1. THE GSwitchManager SHALL 支持至少 5 种不同的域名生成模式（不同 TLD、不同子域结构、不同长度、混合字母数字）
2. WHEN GSwitchManager 生成临时域名, THE GSwitchManager SHALL 从可用模式中随机选择一种
3. THE GSwitchManager SHALL 支持从 M.C.C.（管理控制中心）获取真实域名池，优先使用真实域名而非本地生成

### 需求 20：速率限制阈值随机化（B-06）

**用户故事：** 作为安全工程师，我希望速率限制的默认阈值提高并引入随机偏移，以确保攻击者无法精确计算触发阈值。

#### 验收标准

1. THE Gateway SHALL 将默认 SYN 速率限制从 50/s 提高到至少 200/s
2. THE Gateway SHALL 将默认 CONN 速率限制从 200/s 提高到至少 500/s
3. WHEN Gateway 应用速率限制配置, THE Gateway SHALL 对阈值引入 ±15% 的随机偏移
4. THE Gateway SHALL 在每次启动时重新生成随机偏移值

### 需求 21：信令 UA 修正（B-07）

**用户故事：** 作为安全工程师，我希望信令共振 CF Worker 的 User-Agent 使用真实浏览器值，以确保 DPI 无法通过非标准 UA 识别信令通道。

#### 验收标准

1. THE ResonanceResolver SHALL 将 User-Agent 从 `Mozilla/5.0 (compatible; HealthCheck/1.0)` 替换为真实浏览器 UA 字符串
2. THE ResonanceResolver SHALL 维护一个包含至少 10 个真实浏览器 UA 的池
3. WHEN ResonanceResolver 发送 HTTP 请求, THE ResonanceResolver SHALL 从 UA 池中随机选择一个

### 需求 22：策略引擎调整间隔随机化（B-08）

**用户故事：** 作为安全工程师，我希望策略引擎的调整间隔引入随机抖动，以确保主动探测者无法通过固定 10 秒间隔的时序分析识别策略引擎。

#### 验收标准

1. WHEN StrategyEngine 判断是否允许等级调整, THE StrategyEngine SHALL 使用 [8s, 15s] 范围内的随机间隔替代固定 10 秒
2. THE StrategyEngine SHALL 在每次等级调整后重新生成下一次的随机间隔

### 需求 23：社交时钟渐变过渡（B-09）

**用户故事：** 作为安全工程师，我希望社交时钟的时段边界使用渐变过渡，以确保长期观察者无法通过整点附近的延迟阶跃变化识别 Mirage 流量。

#### 验收标准

1. THE Jitter-Lite eBPF 程序 SHALL 将 `get_social_clock_factor` 中的硬切换逻辑替换为 sigmoid 渐变过渡
2. WHEN 当前时间处于时段边界前后 30 分钟内, THE Jitter-Lite SHALL 使用 sigmoid 函数在两个倍数之间平滑过渡
3. THE Jitter-Lite SHALL 确保过渡期内的倍数变化是连续的，不出现阶跃
