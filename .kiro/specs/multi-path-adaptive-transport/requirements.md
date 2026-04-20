# 需求文档：多路径自适应传输（Multi-Path Adaptive Transport）

## 简介

在现有 QUIC(UDP) + WSS(TCP) 双通道基础上，扩展三种极端环境生存协议（WebRTC DataChannel、ICMP Tunnel、DNS Tunnel），并构建多路径自适应调度系统（Orchestrator）。新增传输协议统一实现 `TransportConn` 接口，与现有 FEC 和分片机制完全兼容，支持基于链路质量的无缝切换。

## 术语表

- **Orchestrator**：多路径自适应调度器，负责协议探测、优先级管理、链路审计与无缝切换
- **TransportConn**：统一传输连接接口，定义 Send/Recv/Close/Type/RTT/RemoteAddr/MaxDatagramSize 方法
- **TransportType**：传输类型枚举，标识不同传输协议（QUIC=0, WSS=1, WebRTC=2, ICMP=3, DNS=4）
- **TransportManager**：传输管理器，管理活跃连接与降级/升格切换
- **PathScheduler**：路径调度器，支持 round-robin、lowest-rtt、redundant 等调度策略
- **FECProcessor**：前向纠错处理器，8 数据分片 + 4 冗余分片
- **Happy_Eyeballs**：分阶段并行竞速探测模式，Phase 1 探测无信令依赖的协议，Phase 2 利用已建立通道拉起有依赖的协议
- **Epoch_Barrier**：纪元屏障，双发选收期间在 ShardHeader 中注入 Epoch 标识，接收端据此切断高延迟通道的拖尾污染
- **Dynamic_MTU**：动态 MTU 通知机制，各传输协议通过 MaxDatagramSize() 声明最大承载能力，路径切换时通知 FECProcessor 调整分片大小
- **Link_Auditor**：实时链路审计器，持续监控丢包率与延迟波动
- **Priority_Level**：传输协议权重优先级（Level 0~3）
- **SDP_Signaling**：WebRTC 会话描述协议信令，通过现有 WSS 通道交换完成 NAT 打洞
- **ICMP_Transport**：基于 ICMP Echo Request/Reply 自定义 Payload 的隧道传输
- **DNS_Transport**：基于 DNS 查询/响应（TXT/CNAME 记录）的隧道传输
- **WebRTC_Transport**：基于 WebRTC DataChannel（DTLS + SCTP）的隧道传输
- **eBPF_TC_Hook**：eBPF Traffic Control 钩子，用于内核态截获和处理 ICMP 数据包
- **Probe_Cycle**：心跳探测周期，默认 30 秒，对所有已知路径执行连通性检测
- **Promotion**：升格操作，当高优先级路径恢复可用时，将流量从低优先级路径迁移回高优先级路径
- **Demotion**：降格操作，当当前路径劣化时，将流量切换到下一可用低优先级路径

---

## 需求

### 需求 1：WebRTC DataChannel 传输协议

**用户故事：** 作为网关运维人员，我希望在 UDP 被封锁但 WebRTC 流量不受限的环境中，通过 WebRTC DataChannel 承载隧道流量，使 DPI 设备将流量识别为正常的实时视频通话。

#### 验收标准

1. THE WebRTC_Transport SHALL 实现 TransportConn 接口（Send、Recv、Close、Type、RTT、RemoteAddr）
2. THE WebRTC_Transport SHALL 使用 pion/webrtc 库建立 DTLS + SCTP 连接
3. WHEN WebRTC_Transport 需要建立连接时，THE WebRTC_Transport SHALL 通过现有 WSS 通道交换 SDP 信令完成 NAT 打洞
4. THE WebRTC_Transport SHALL 不参与 HappyEyeballs Phase 1 首发竞速，仅在 WSS 通道建立成功后的 Phase 2 阶段发起连接
5. THE WebRTC_Transport SHALL 将 IP 数据包封装进 WebRTC DataChannel 的不可靠交付模式传输
6. WHEN WebRTC_Transport 建立连接成功时，THE WebRTC_Transport SHALL 返回 TransportType 值为 2（TransportWebRTC）
7. THE WebRTC_Transport SHALL 与现有 FECProcessor 兼容，支持接收和发送 FEC 编码后的分片数据
8. IF WebRTC DataChannel 连接断开，THEN THE WebRTC_Transport SHALL 通过 Recv 方法返回 io.EOF 错误并释放底层资源

---

### 需求 2：ICMP Tunnel 传输协议

**用户故事：** 作为网关运维人员，我希望在 Captive Portal 或公共 WiFi 计费网关环境中，通过 ICMP Echo Request/Reply 的自定义 Payload 承载加密数据，使外界观察到的流量仅为普通 Ping 操作。

#### 验收标准

1. THE ICMP_Transport SHALL 实现 TransportConn 接口（Send、Recv、Close、Type、RTT、RemoteAddr）
2. THE ICMP_Transport 的数据面 SHALL 使用 C 语言在 eBPF TC Hook 层实现原始 ICMP 包的构造与截获
3. THE ICMP_Transport SHALL 使用 ICMP Echo Request（类型 8）发送数据，使用 ICMP Echo Reply（类型 0）接收数据
4. THE ICMP_Transport SHALL 将加密后的数据分片塞入 ICMP Payload 字段
5. THE ICMP_Transport 的 Go 控制面 SHALL 通过 eBPF Map 向 C 数据面下发配置指令
6. THE ICMP_Transport 的 C 数据面 SHALL 通过 Ring Buffer 向 Go 控制面上报接收到的数据
7. WHEN ICMP_Transport 建立连接成功时，THE ICMP_Transport SHALL 返回 TransportType 值为 3（TransportICMP）
8. THE ICMP_Transport 的网关侧进程 SHALL 要求 CAP_NET_RAW 权限
9. THE ICMP_Transport SHALL 与现有 FECProcessor 兼容，支持接收和发送 FEC 编码后的分片数据
10. IF ICMP 通道不可达，THEN THE ICMP_Transport SHALL 通过 Recv 方法返回错误并释放 eBPF 资源

---

### 需求 3：DNS Tunnel 传输协议

**用户故事：** 作为网关运维人员，我希望在几乎全封锁的极端环境中，通过 DNS 查询/响应承载紧急控制指令，只要目标网络能解析外部域名就能建立通信通道。

#### 验收标准

1. THE DNS_Transport SHALL 实现 TransportConn 接口（Send、Recv、Close、Type、RTT、RemoteAddr）
2. THE DNS_Transport 的客户端 SHALL 将数据使用 Base32 编码作为子域名发起 DNS 查询
3. THE DNS_Transport 的网关侧 SHALL 运行权威 DNS 服务器，解码子域名获取上行数据，并将下行数据编码进 TXT 或 CNAME 记录
4. WHEN DNS_Transport 建立连接成功时，THE DNS_Transport SHALL 返回 TransportType 值为 4（TransportDNS）
5. THE DNS_Transport SHALL 仅作为紧急控制指令通道使用，不承载高频大流量数据
6. THE DNS_Transport SHALL 与现有 FECProcessor 兼容，支持接收和发送 FEC 编码后的分片数据
7. THE DNS_Transport 的 MaxDatagramSize() SHALL 返回不超过 110 字节（受 DNS Label 63 字节 × 多 Label 拼接 - Base32 膨胀系数约束），FECProcessor 在 DNS 模式下 SHALL 据此动态缩小分片大小
8. IF DNS 查询超时或解析失败，THEN THE DNS_Transport SHALL 返回明确的错误信息
9. THE DNS_Transport 的控制面逻辑 SHALL 使用 Go 语言实现

---

### 需求 4：TransportType 扩展与接口标准化

**用户故事：** 作为开发人员，我希望新增的三种传输协议与现有 QUIC 和 WSS 通道使用统一的类型枚举和接口规范，确保调度器和管理器无需感知具体协议细节。

#### 验收标准

1. THE TransportType 枚举 SHALL 扩展为五个值：TransportQUIC=0、TransportWebSocket=1、TransportWebRTC=2、TransportICMP=3、TransportDNS=4
2. THE ICMPTransport、WebRTCTransport、DNSTransport SHALL 各自独立实现 TransportConn 接口
3. THE TransportConn 接口 SHALL 新增 MaxDatagramSize() int 方法，返回该传输协议单次 Send 可承载的最大字节数
4. THE Orchestrator SHALL 仅通过 TransportConn 接口与所有传输协议交互，不依赖具体实现类型
5. FOR ALL TransportConn 实现，Send 方法接受的 data 参数 SHALL 为经过 FEC 编码后的分片字节切片
6. FOR ALL TransportConn 实现，RTT 方法 SHALL 返回最近一次测量的往返时延
7. WHEN Orchestrator 切换活跃路径时，SHALL 读取新路径的 MaxDatagramSize() 并通知 FECProcessor 动态调整分片大小

---

### 需求 5：多路径自适应调度器（Orchestrator）

**用户故事：** 作为网关运维人员，我希望系统能自动探测所有可用传输路径，根据链路质量实时选择最优路径，在路径劣化时无缝切换，用户层完全无感知。

#### 验收标准

1. THE Orchestrator SHALL 在启动时使用分阶段 Happy_Eyeballs 模式探测传输协议
2. THE Happy_Eyeballs Phase 1 SHALL 并行探测无信令依赖的协议（QUIC、WSS、ICMP、DNS），WebRTC 不参与 Phase 1
3. WHEN Phase 1 中 WSS 探测成功时（无论是否成为活跃路径），THE Orchestrator SHALL 立即进入 Phase 2，利用 WSS 作为信令通道在后台拉起 WebRTC 连通性测试
4. IF WebRTC 在 Phase 2 打洞成功且其优先级高于当前活跃路径，THEN THE Orchestrator SHALL 通过 Promote 机制将流量升格到 WebRTC
5. WHEN 多个协议同时探测成功时，THE Orchestrator SHALL 选择优先级最高且最先完成握手的协议作为初始活跃路径
6. THE Orchestrator SHALL 维护四级权重优先级：Level 0 为 QUIC/UDP，Level 1 为 WebRTC，Level 2 为 WSS/TCP，Level 3 为 ICMP 和 DNS
7. WHILE 存在活跃连接时，THE Link_Auditor SHALL 持续监控当前路径的丢包率和延迟波动
8. WHEN 当前路径丢包率超过 30% 或延迟波动超过基线 RTT 的 200% 时，THE Orchestrator SHALL 触发降格切换到下一可用优先级路径
9. THE Orchestrator SHALL 在降格切换期间使用带 Epoch_Barrier 的双发选收模式（DualSendMode），在 ShardHeader 中注入 Epoch 标识，接收端收到新 Epoch 后冻结旧通道数据解析，从新通道缓冲区推进流水线，确保切换过程零丢包且无乱序污染
10. THE Orchestrator SHALL 替代现有 TransportManager 的 ConnectWithFallback 方法，将串行尝试改为分阶段并发 Goroutine 竞速模式

---

### 需求 6：心跳探测与路径升格

**用户故事：** 作为网关运维人员，我希望系统在降格到低优先级路径后，持续探测高优先级路径的恢复状态，一旦恢复立即升格回高性能路径。

#### 验收标准

1. THE Orchestrator SHALL 以 30 秒为默认周期对所有已知路径执行心跳探测（Probe_Cycle）
2. WHEN 高优先级路径连续 3 次探测成功时，THE Orchestrator SHALL 触发升格操作将流量迁移回该路径
3. THE Orchestrator SHALL 在升格过程中使用带 Epoch_Barrier 的双发选收模式，确保迁移过程零丢包且无乱序污染
4. WHILE 处于 Level 3（ICMP/DNS）路径时，THE Orchestrator SHALL 将探测周期缩短为 15 秒以加速恢复
5. IF 升格过程中新路径再次失败，THEN THE Orchestrator SHALL 立即回退到原路径并重置探测计数器

---

### 需求 7：ICMP 数据面 eBPF 程序

**用户故事：** 作为开发人员，我希望 ICMP Tunnel 的数据面在内核态高效处理原始 ICMP 包，通过 eBPF TC Hook 实现零拷贝截获和构造，满足 C 数据面延迟小于 1ms 的性能要求。

#### 验收标准

1. THE ICMP eBPF 程序 SHALL 使用 C 语言编写，挂载到 TC（Traffic Control）Hook 点
2. THE ICMP eBPF 程序 SHALL 从 eBPF Map（HASH 类型）读取 Go 控制面下发的目标 IP 和加密密钥配置
3. THE ICMP eBPF 程序 SHALL 在 TC egress 方向构造 ICMP Echo Request 包，将加密数据写入 Payload 字段
4. THE ICMP eBPF 程序 SHALL 在 TC ingress 方向截获匹配的 ICMP Echo Reply 包，提取 Payload 数据
5. THE ICMP eBPF 程序 SHALL 通过 Ring Buffer 将截获的 Payload 数据上报给 Go 控制面
6. THE ICMP eBPF 程序处理单个 ICMP 包的延迟 SHALL 小于 1ms
7. IF eBPF Map 中无匹配配置，THEN THE ICMP eBPF 程序 SHALL 返回 TC_ACT_OK 放行数据包

---

### 需求 8：分片序列化与反序列化兼容性

**用户故事：** 作为开发人员，我希望所有新增传输协议能正确序列化和反序列化现有的 Shard/ShardHeader 结构，确保 FEC 编解码流程在任意传输通道上均能正常工作。

#### 验收标准

1. FOR ALL TransportConn 实现，通过 Send 方法发送的数据 SHALL 为 SerializeShard 函数输出的字节切片
2. FOR ALL TransportConn 实现，通过 Recv 方法接收的数据 SHALL 能被 DeserializeShard 函数正确解析
3. THE SerializeShard 函数 SHALL 对任意合法 Shard 输入产生确定性输出
4. FOR ALL 合法 Shard 对象，执行 SerializeShard 后再执行 DeserializeShard SHALL 还原出等价的 Shard 对象（往返一致性）
5. IF 接收到的数据长度小于 ShardHeader 大小，THEN THE DeserializeShard 函数 SHALL 返回明确的错误信息

---

### 需求 9：配置与运行时管理

**用户故事：** 作为网关运维人员，我希望通过配置文件控制各传输协议的启用状态、优先级和探测参数，支持运行时动态调整。

#### 验收标准

1. THE Orchestrator SHALL 从 gateway.yaml 配置文件读取各传输协议的启用状态和参数
2. THE 配置 SHALL 支持为每种传输协议独立设置启用/禁用开关
3. THE 配置 SHALL 支持自定义 Probe_Cycle 周期、降格阈值（丢包率和延迟倍数）、升格连续成功次数
4. WHEN 配置中某传输协议被禁用时，THE Orchestrator SHALL 跳过该协议的探测和连接尝试
5. IF 配置文件缺少某传输协议的配置段，THEN THE Orchestrator SHALL 使用该协议的默认配置值
