# 实施计划：外部零特征消除

## 概述

按 P0（上线阻断）→ P1（短期修复）→ P2（中期修复）优先级分组，依赖关系排序。Go 控制面 + C eBPF 数据面双线推进，通过 eBPF Map 通信。属性测试使用 `pgregory.net/rapid`。

## 任务

### P0：上线阻断 — 架构级结构修复 + 核心被动特征消除

- [x] 1. Client QUIC ALPN 修正与指纹对齐
  - [x] 1.1 修改 `phantom-client/pkg/gtclient/quic_engine.go`，将 `NextProtos` 从 `["mirage-gtunnel"]` 改为 `["h3"]`，移除所有握手字段中的 `mirage`、`gtunnel` 等自定义标识
    - _需求: 2.1, 2.2, 2.3_
  - [x] 1.2 修改 `phantom-client/pkg/gtclient/quic_engine.go`，通过 quic-go `*quic.Config` 可用字段对齐 Chrome 140+：`MaxIdleTimeout=30s`、`InitialPacketSize=1200`、`InitialStreamReceiveWindow=6MB`、`InitialConnectionReceiveWindow=15MB`，CID 长度 8 字节。注意：`InitialMaxData`、`MaxUDPPayloadSize` 不是 quic-go Config 的可用字段，记录为技术债务
    - _需求: 3.1, 3.2, 3.3, 3.4, 3.5_
  - [x] 1.3 抓包验收当前 quic-go 默认 Transport Parameters 与 Chrome 的差异，记录差异清单到文档
    - _需求: 3.7_
  - [x]* 1.4 编写属性测试验证 QUIC 配置不含自定义协议标识
    - **Property 5: QUIC 配置不含自定义协议标识**
    - **验证: 需求 2.1, 2.2, 2.3**

- [x] 2. Client 源头指纹生成（A-06 主线 + 补线）
  - [x] 2.1 修改 `phantom-client/pkg/gtclient/quic_engine.go`，配置 `tls.Config.MinVersion = tls.VersionTLS13`、`NextProtos = ["h3"]`。记录"可控字段清单"和"不可控字段差异清单"（TLS 1.3 CipherSuites 不可配置、CurvePreferences 顺序不保证生效），通过 pcap 抓包验收确认实际差异并设定验收门槛
    - _需求: 7.1, 7.2, 7.4_
  - [x] 2.2 新建 `mirage-gateway/pkg/rewriter/nfqueue_rewriter.go`，实现基于 NFQUEUE 的用户态拦截层，读取 B-DNA `skb->mark` 标记，对 Gateway 出站 TCP/TLS 连接执行指纹重写
    - _需求: 7.4, 7.5, 7.6_
  - [x]* 2.3 编写 pcap 抓包验收脚本，对比 Client QUIC ClientHello 与 Chrome 的实际差异
    - _需求: 7.4_

- [x] 3. Send-Path Shim — Client 出网特征消除
  - [x] 3.1 新建 `phantom-client/pkg/gtclient/send_path_shim.go`，定义 `SendPathShim` 结构体（`sendFn`/`paddingMean`/`paddingStddev`/`maxMTU`/`iatMode`/`iatMeanUs`/`iatStddevUs`），定义 `IATMode` 类型和 `SendPathShimConfig` 结构体
    - _需求: 4.1, 4.2, 4.3, 4.4_
  - [x] 3.2 实现 `applyPadding(encrypted)` 方法：对加密后的 datagram 追加随机字节使包长符合 N(mean, stddev²)，超 MTU 截断；实现 `sampleIATDelay()` 方法：正态用 `NormFloat64()`，指数用 `ExpFloat64()`，负值归零
    - _需求: 4.2, 4.4, 4.5_
  - [x] 3.3 实现 `Send(encrypted)` 方法：先 Padding 再 IAT 延迟再 sendFn
    - _需求: 4.1_
  - [x] 3.4 修改 `phantom-client/pkg/gtclient/client.go` 的 `Send()` 方法，将 `transport.SendDatagram(encrypted)` 和 `quicEngine.SendDatagram(encrypted)` 替换为 `c.sendShim.Send(encrypted)`，确保 shim 作用在"加密后、真正发 datagram 前"的边界
    - _需求: 4.1, 4.6, 5.3_
  - [x]* 3.5 编写属性测试验证 Padding 不变量
    - **Property 6: SendPathShim Padding 不变量**
    - **验证: 需求 4.2, 4.5**
  - [x]* 3.6 编写属性测试验证 IAT 采样范围
    - **Property 7: SendPathShim IAT 采样范围**
    - **验证: 需求 4.4**

- [x] 4. Orchestrator 原子切换事务
  - [x] 4.1 新建 `mirage-gateway/pkg/gtunnel/switch_buffer.go`，定义 `SwitchBuffer` 结构体（基于 `TransportConn` 接口，不依赖 `*Path`/`*net.UDPConn`），实现 `EnableDualSend(oldConn, newConn TransportConn, duration time.Duration)`、`SendDual(data []byte)`、`ReceiveAndDedupe(seq, data)`、`IsDualModeActive()` 方法
    - _需求: 1.1, 1.2, 1.3_
  - [x] 4.2 修改 `mirage-gateway/pkg/gtunnel/orchestrator.go`：在 `NewOrchestrator` 中初始化 `SwitchBuffer`（替代未初始化的 `mpBuffer`），实现 `executeSwitchTransaction(oldPath, newPath *ManagedPath)` 方法，将 `demote()`/`promote()` 改为调用该事务方法
    - 事务步骤：EnableDualSend(oldPath.Conn, newPath.Conn) → 双发窗口 → epoch barrier → notifyFECMTU → 收敛 activePath
    - CID rotation 降级为技术债务（quic-go 公开 API 不支持）
    - _需求: 1.1, 1.2, 1.4, 1.5_
  - [x] 4.3 实现回滚逻辑：双发期间新路径连续 N 次发送失败 → 回滚到旧路径，epoch 不变
    - _需求: 1.6_
  - [x]* 4.3 编写属性测试验证切换操作启动双发模式
    - **Property 1: 切换操作启动双发模式**
    - **验证: 需求 1.1, 1.2**
  - [x]* 4.4 编写属性测试验证切换事务 epoch 递增
    - **Property 3: 切换事务 epoch 递增**
    - **验证: 需求 1.4**
  - [x]* 4.5 编写属性测试验证切换事务回滚保持 epoch 不变
    - **Property 4: 切换事务回滚保持 epoch 不变**
    - **验证: 需求 1.7**

- [x] 5. SwitchBuffer 双发选收去重与时间随机化
  - [x] 5.1 在 `switch_buffer.go` 的 `EnableDualSend` 中使用 `crypto/rand` 生成 [80ms, 200ms] 随机双发时长
    - _需求: 18.1, 18.2, 18.3_
  - [x] 5.2 确认 `ReceiveAndDedupe` 去重逻辑正确：首次返回 `(data, true)`，重复 seq 返回 `(nil, false)`
    - _需求: 1.3_
  - [x]* 5.3 编写属性测试验证双发选收去重正确性
    - **Property 2: 双发选收去重正确性**
    - **验证: 需求 1.3**
  - [x]* 5.4 编写属性测试验证双发模式持续时间随机化范围
    - **Property 15: 双发模式持续时间随机化范围**
    - **验证: 需求 18.1, 18.2**

- [x] 6. Checkpoint — 确保所有测试通过
  - 确保所有测试通过，如有问题请向用户确认。

### P0：上线阻断 — Gateway 生产态 Bearer Listener + 主动探测防护

- [x] 7. 生产态 QUIC/H3 Bearer Listener + 两层探测防护
  - [x] 7.1 修改 `mirage-gateway/cmd/gateway/main.go`，在 `main()` 中创建 QUIC/H3 bearer listener（443/UDP），配置 `EnableDatagrams=true`、`MaxIdleTimeout=30s`，通过现有 `Orchestrator.StartPassive()` 模式绑定到 Orchestrator
    - _需求: 13.1, 13.2, 13.4_
  - [x] 7.2 实现 HTTP/3 请求处理：标准 HTTP/3 请求返回 403/404 合法响应
    - _需求: 13.3_
  - [x] 7.3 新建 `mirage-gateway/pkg/threat/quic_guard.go`，实现第一层 `UDPPreFilter`：在 `quic.Listener` 之前对 UDP 首包做 QUIC Initial 格式校验（Version/DCID 长度/Packet Type），不合法的直接丢弃不回包
    - _需求: 14.1_
  - [x] 7.4 在同文件实现第二层 `QUICPostAcceptValidator`：在 `quic.Listener.Accept()` 返回 `*quic.Conn` 后，检查 `ConnectionState().TLS.NegotiatedProtocol` 是否为 `h3`，非 h3 则快速关闭连接并与 `BlacklistManager`/`RiskScorer` 集成
    - _需求: 14.2, 14.3_
  - [x] 7.5 在 `main.go` 中将两层防护接入 QUIC/H3 bearer listener，保持现有 TCP `HandshakeGuard`/`ProtocolDetector` 继续挂在 gRPC listener 上不做修改
    - _需求: 14.4_
  - [x]* 7.6 编写集成测试验证 QUIC/H3 握手、HTTP/3 响应、非法 Initial 丢弃和协商异常快速关闭
    - _需求: 13.2, 13.3, 14.1, 14.2_

- [x] 8. WSS 降级路径接入 uTLS
  - [x] 8.1 修改 `mirage-gateway/pkg/gtunnel/chameleon_client.go`，将 `DialChameleon` 改为使用 `dialWithUTLS` 建立底层 TCP+TLS 连接，在 uTLS 连接之上建立 WebSocket
    - _需求: 6.1, 6.2, 6.3_
  - [x]* 8.2 编写单元测试验证 WSS 连接使用 uTLS 指纹
    - _需求: 6.2_

- [x] 9. Checkpoint — 确保所有测试通过
  - 确保所有测试通过，如有问题请向用户确认。

### P1：短期修复 — eBPF 数据面特征消除

- [x] 10. B-DNA 非 SYN 包一致性
  - [x] 10.1 修改 `mirage-gateway/bpf/bdna.c`，新增 `bdna_conn_map`（LRU_HASH, 65536 entries），定义 `conn_key` 和 `conn_state` 结构体
    - _需求: 10.2_
  - [x] 10.2 修改 `bdna_tcp_rewrite`：SYN 包重写后将 `target_window` 存入 `bdna_conn_map`；非 SYN 包前 N 个（默认 10）读取 Map 维护 Window Size 一致性并重算校验和
    - _需求: 10.1, 10.3_
  - [x]* 10.3 编写属性测试验证 B-DNA 非 SYN 包 Window Size 一致性（Go 用户态等价实现）
    - **Property 9: B-DNA 非 SYN 包 Window Size 一致性**
    - **验证: 需求 10.1, 10.2, 10.3**

- [x] 11. Jitter-Lite 高斯采样修正
  - [x] 11.1 修改 `mirage-gateway/bpf/common.h`，将 `gaussian_sample` 从均匀分布替换为 Irwin-Hall 近似（4 个 `bpf_get_prandom_u32` 求和再缩放），结果 <= 0 时返回 0
    - _需求: 11.1, 11.2_
  - [x]* 11.2 编写属性测试验证 Irwin-Hall 高斯采样统计特征（Go 用户态等价实现，1000 次采样验证均值和标准差）
    - **Property 10: Irwin-Hall 高斯采样统计特征**
    - **验证: 需求 11.1, 11.2**

- [x] 12. VPC 延迟分布模型修正
  - [x] 12.1 修改 `mirage-gateway/bpf/jitter.c`，将 `simulate_fiber_jitter_v2` 中的均匀分布替换为分段线性近似的指数分布
    - _需求: 12.1, 12.3_
  - [x] 12.2 修改 `mirage-gateway/bpf/jitter.c`，将 `simulate_submarine_cable` 中的三角波替换为 3 频率叠加伪随机波形
    - _需求: 12.2_
  - [x]* 12.3 编写属性测试验证 VPC 光缆抖动指数分布特征（Go 用户态等价实现）
    - **Property 11: VPC 光缆抖动指数分布特征**
    - **验证: 需求 12.1, 12.3**
  - [x]* 12.4 编写属性测试验证 VPC 跨洋模拟非周期性
    - **Property 12: VPC 跨洋模拟非周期性**
    - **验证: 需求 12.2**

- [x] 13. 配额熔断渐进式降级
  - [x] 13.1 修改 `mirage-gateway/bpf/jitter.c`，扩展 `quota_map` 为 2 entries（key=0 剩余，key=1 总量），将硬截断 `TC_ACT_STOLEN` 替换为概率降级：剩余 <10% 时 50% 通过，<1% 时 10% 通过
    - _需求: 16.1, 16.2, 16.3_
  - [x]* 13.2 编写属性测试验证配额降级概率正确性（Go 用户态等价实现）
    - **Property 13: 配额降级概率正确性**
    - **验证: 需求 16.1, 16.2, 16.3**

- [x] 14. 社交时钟渐变过渡
  - [x] 14.1 修改 `mirage-gateway/bpf/jitter.c`，将 `get_social_clock_factor` 中的硬切换替换为 sigmoid 渐变：30 分钟过渡窗口，使用整数近似 `sigmoid(x) ≈ x/(1+|x|)`
    - 扩展 `social_clock_config` 新增 `transition_window` 字段
    - _需求: 23.1, 23.2, 23.3_
  - [x]* 14.2 编写属性测试验证社交时钟渐变连续性（Go 用户态等价实现）
    - **Property 20: 社交时钟渐变连续性**
    - **验证: 需求 23.1, 23.2, 23.3**

- [x] 15. B-DNA 指纹模板扩展
  - [x] 15.1 修改 `mirage-gateway/bpf/bdna.c`，将 `fingerprint_map` 的 `max_entries` 从 16 扩展到 64
    - _需求: 8.1_
  - [x] 15.2 创建指纹模板配置文件（YAML），包含 30+ 模板覆盖 Chrome 130-140+、Firefox 120-130+、Safari 17-18、Edge 120-130+；在 Go 控制面加载器中实现启动时从配置文件读取并写入 eBPF Map
    - 定义 `BrowserFingerprint` Go 结构体
    - _需求: 8.1, 8.2, 8.3_
  - [x]* 15.3 编写单元测试验证模板数量 >= 30 且覆盖 4 种浏览器
    - _需求: 8.1, 8.2_

- [x] 16. NPM 默认模式固化与运行时断言
  - [x] 16.1 在 NPM Go 控制面加载器中实现 `VerifyGaussianMode()`：启动时从 eBPF Map 读取完整 `NPMConfig` 结构体，检查 `PaddingMode` 字段是否为 Gaussian，非 Gaussian 时记录错误、修正 `PaddingMode` 后写回完整结构体
    - _需求: 9.1, 9.2, 9.3_
  - [x]* 16.2 编写属性测试验证 NPM 模式修正不变量
    - **Property 8: NPM 模式修正不变量**
    - **验证: 需求 9.2, 9.3**

- [x] 17. Checkpoint — 确保所有测试通过
  - 确保所有测试通过，如有问题请向用户确认。

### P2：中期修复 — 主动探测参数随机化 + 信令修正

- [x] 18. 策略引擎参数随机偏移与调整间隔随机化
  - [x] 18.1 修改 `mirage-gateway/pkg/strategy/engine.go`，新增 `cachedParams *DefenseParams` 字段，实现 `applyRandomOffset(params, ratio)` 和 `regenerateParams()` 函数。在 `NewStrategyEngine` 初始化时和 `UpdateByThreat` 等级变化时调用 `regenerateParams()` 一次性生成带 ±20% 偏移的参数并缓存。`GetParams()` 返回缓存值（同一等级内多次调用返回相同值）
    - _需求: 17.1, 17.2, 17.3, 17.4_
  - [x] 18.2 修改 `mirage-gateway/pkg/strategy/engine.go`，新增 `adjustInterval time.Duration` 字段，实现 `randomAdjustInterval()` 返回 [8s, 15s] 随机间隔，替换 `UpdateByThreat` 中的固定 `10*time.Second`，每次等级调整后重新生成
    - _需求: 22.1, 22.2_
  - [x]* 18.3 编写属性测试验证策略引擎参数随机偏移范围
    - **Property 14: 策略引擎参数随机偏移范围**
    - **验证: 需求 17.1, 17.2, 17.3**
  - [x]* 18.4 编写属性测试验证策略调整间隔随机化范围
    - **Property 19: 策略调整间隔随机化范围**
    - **验证: 需求 22.1, 22.2**

- [x] 19. G-Switch 域名格式多样化
  - [x] 19.1 修改 `mirage-gateway/pkg/gswitch/manager.go`，定义 5+ 种 `domainPatterns`（不同 TLD/子域/编码），实现辅助函数 `base32Encode`/`alphanumeric`/`wordPair`，修改 `generateTempDomain` 随机选择模式
    - _需求: 19.1, 19.2, 19.3_
  - [x]* 19.2 编写属性测试验证域名生成格式多样性
    - **Property 16: 域名生成格式多样性**
    - **验证: 需求 19.1, 19.2**

- [x] 20. 速率限制阈值随机化
  - [x] 20.1 修改 `mirage-gateway/cmd/gateway/main.go`，将默认 SYN 速率从 50 提高到 200、CONN 速率从 200 提高到 500，实现 `applyRateOffset(base, 0.15)` 每次启动生成 ±15% 随机偏移
    - _需求: 20.1, 20.2, 20.3, 20.4_
  - [x]* 20.2 编写属性测试验证速率限制随机偏移范围
    - **Property 17: 速率限制随机偏移范围**
    - **验证: 需求 20.3**

- [x] 21. 信令 UA 修正
  - [x] 21.1 修改 `mirage-gateway/pkg/gswitch/resonance_resolver.go`，定义 10+ 真实浏览器 UA 池（Chrome/Firefox/Safari/Edge 多平台），实现 `randomUA()` 使用 `crypto/rand` 随机选择，替换所有 HTTP 请求中的硬编码 UA
    - _需求: 21.1, 21.2, 21.3_
  - [x]* 21.2 编写属性测试验证信令 UA 随机性与合法性
    - **Property 18: 信令 UA 随机性与合法性**
    - **验证: 需求 21.1, 21.2, 21.3**

- [x] 22. DNSServer 接入 Gateway 生产启动链
  - [x] 22.1 修改 `mirage-gateway/cmd/gateway/main.go`，在生产启动链中调用 `NewDNSServer(domain, listenAddr)` 创建 DNS 服务端，调用 `SetRecvCallback` 注册收包回调（将上行数据喂给 Orchestrator 的 `FeedInboundPacket`），调用 `Start()` 启动 DNS 监听。启动失败记录告警但不阻断 Gateway
    - _需求: 15.1, 15.4_

- [x] 23. WebRTCAnswerer 接入 Gateway 生产启动链
  - [x] 23.1 修改 `mirage-gateway/cmd/gateway/main.go`，在生产启动链中预注册 WebRTC 控制帧处理器。`WebRTCAnswerer` 依赖 WSS 信令通道（`sendCtrlFrame` 回调），因此不能在启动时立即创建，而是在 ChameleonListener/WSS ServerConn 就绪后，通过 WSS 控制帧触发 `NewWebRTCAnswerer(config, sendCtrl)` → `HandleOffer` → `HandleRemoteCandidate` → `WaitReady` 流程。在 `main.go` 中注册 WSS 控制帧路由，将 `CtrlTypeSDP_Offer`/`CtrlTypeICE_Candidate` 路由到 WebRTCAnswerer 实例。启动失败记录告警但不阻断
    - _需求: 15.3, 15.4_

- [x] 24. ICMPTransport 接入 Gateway 生产启动链
  - [x] 24.1 `ICMPTransport` 是主动 transport（Go Raw Socket 发送 + eBPF Ring Buffer 接收），不是被动 listener。在 `main.go` 中按 `NewICMPTransport(configMap, txMap, rxRingbuf, config)` 初始化（需要 eBPF Map 注入和 Raw Socket 权限），通过 `Orchestrator.AdoptInboundConn` 注册为可用路径。启动失败记录告警但不阻断
    - _需求: 15.2, 15.4_
  - [x]* 24.2 编写集成测试验证三个冷备协议（DNSServer / WebRTCAnswerer / ICMPTransport）启动和注册
    - _需求: 15.4_

- [x] 25. Checkpoint — 确保所有测试通过
  - 确保所有测试通过，如有问题请向用户确认。

### 端到端验收

- [x] 26. 协同链闭环集成验证
  - [x] 26.1 验证 Client 数据流经过完整协议协同链：Client SendPathShim (Padding/IAT) → QUIC (ALPN h3) → Bearer Listener → B-DNA → NPM → Jitter-Lite → VPC，确保 eBPF 程序挂载在正确的 bearer path（公网 QUIC/H3 listener 对应的网卡 ifindex）
    - _需求: 5.1, 5.2, 5.3, 5.4_
  - [x]* 26.2 编写端到端集成测试验证协议链完整性
    - _需求: 5.1, 5.2, 5.3, 5.4_

- [x] 27. 最终 Checkpoint — 确保所有测试通过
  - 确保所有测试通过，如有问题请向用户确认。

## 备注

- 标记 `*` 的子任务为可选，可跳过以加速 MVP
- 每个任务引用具体需求编号以确保可追溯
- eBPF 数据面属性测试使用 Go 用户态等价实现调用
- Go 属性测试使用 `pgregory.net/rapid` 库
- Checkpoint 确保增量验证
