# 实现计划：多路径自适应传输（Multi-Path Adaptive Transport）

## 概述

在现有 G-Tunnel 双通道架构基础上，扩展 WebRTC DataChannel、ICMP Tunnel、DNS Tunnel 三种极端环境生存协议，构建统一的 Orchestrator 多路径自适应调度器。Go 控制面 + C 数据面（ICMP eBPF TC Hook）。

## 任务

- [x] 1. 扩展 TransportType 枚举与依赖准备
  - [x] 1.1 扩展 `mirage-gateway/pkg/gtunnel/transport.go` 中的 TransportType 枚举，新增 TransportWebRTC=2、TransportICMP=3、TransportDNS=4；在 TransportConn 接口中新增 `MaxDatagramSize() int` 方法
    - 保持现有 TransportQUIC=0、TransportWebSocket=1 不变
    - 现有 ChameleonClientConn 需补充 MaxDatagramSize() 实现（返回 65535）
    - _需求: 4.1, 4.3_
  - [x] 1.2 扩展 `mirage-gateway/pkg/gtunnel/fec.go` 中的 ShardHeader 结构体，新增 Epoch uint32 字段和 Reserved uint16 字段；更新 SerializeShard/DeserializeShard 函数以支持 Epoch
    - _需求: 5.9, 8.3, 8.4_
  - [x] 1.3 在 `mirage-gateway/pkg/ebpf/types.go` 中添加 ICMP 相关 Go 结构体（ICMPConfig、ICMPTxEntry、ICMPRxEvent），确保与 C 数据面结构体字节对齐
    - _需求: 2.5, 2.6, 7.2_
  - [x] 1.4 更新 `mirage-gateway/go.mod`，添加 `github.com/pion/webrtc/v4` 和 `github.com/miekg/dns` 依赖
    - _需求: 1.2, 3.8_

- [x] 2. 实现 LinkAuditor 链路审计器
  - [x] 2.1 创建 `mirage-gateway/pkg/gtunnel/link_auditor.go`，实现 LinkAuditor 结构体
    - 实现 PathMetrics、AuditThresholds 数据结构
    - 实现 RecordSample 方法：记录 RTT 和丢包采样
    - 实现 ShouldDegrade 方法：丢包率 > demoteLossRate 或 RTT > demoteRTTMultiple * baselineRTT 时返回 true
    - 实现 GetMetrics 方法
    - _需求: 5.4, 5.5_
  - [x]* 2.2 编写属性测试：降格判定阈值正确性
    - **Property 5: 降格判定阈值正确性**
    - 使用 `pgregory.net/rapid` 生成随机 PathMetrics（lossRate 0.0~1.0、随机 RTT、随机 baselineRTT）
    - 验证 ShouldDegrade 返回 true 当且仅当 lossRate > demoteLossRate 或 rtt > demoteRTTMultiple * baselineRTT
    - **验证: 需求 5.8**
  - [x]* 2.3 编写属性测试：升格判定连续成功计数
    - **Property 6: 升格判定连续成功计数**
    - 使用 rapid 生成随机布尔值探测结果序列
    - 验证升格触发当且仅当序列中存在连续 ≥ promoteThreshold 个 true 值
    - **验证: 需求 6.2**

- [x] 3. 实现 OrchestratorConfig 与配置解析
  - [x] 3.1 在 `mirage-gateway/pkg/gtunnel/orchestrator.go` 中定义 OrchestratorConfig、各协议独立配置结构体（WebRTCTransportConfig、ICMPTransportConfig、DNSTransportConfig）及默认值函数
    - _需求: 9.1, 9.2, 9.3_
  - [x] 3.2 扩展 `mirage-gateway/configs/gateway.yaml`，添加 orchestrator 和 transports 配置段
    - _需求: 9.1, 9.2_
  - [x] 3.3 实现配置解析逻辑：从 gateway.yaml 读取并填充 OrchestratorConfig，缺失字段使用默认值
    - _需求: 9.4, 9.5_
  - [x]* 3.4 编写属性测试：缺失配置使用默认值
    - **Property 8: 缺失配置使用默认值**
    - 使用 rapid 生成部分 OrchestratorConfig（随机缺失某些传输协议配置段）
    - 验证解析后缺失字段的值等于该协议的默认配置值
    - **验证: 需求 9.5**

- [x] 4. Checkpoint — 确保所有测试通过
  - 确保所有测试通过，如有疑问请询问用户。

- [x] 5. 实现 WebRTCTransport
  - [x] 5.1 创建 `mirage-gateway/pkg/gtunnel/webrtc_transport.go`，实现 WebRTCTransport 结构体
    - 使用 pion/webrtc 库建立 DTLS + SCTP 连接
    - 实现 SDPSignaler 接口，通过现有 WSS 通道交换 SDP 信令
    - 实现 TransportConn 接口：Send、Recv、Close、Type（返回 TransportWebRTC=2）、RTT、RemoteAddr、MaxDatagramSize（返回 16384）
    - DataChannel 使用不可靠交付模式
    - 连接断开时 Recv 返回 io.EOF 并释放资源
    - WebRTC 不参与 Phase 1 竞速，仅在 WSS 建立后的 Phase 2 阶段通过 SDPSignaler 拉起
    - _需求: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8_

- [x] 6. 实现 DNSTransport
  - [x] 6.1 创建 `mirage-gateway/pkg/gtunnel/dns_transport.go`，实现 DNSTransport 客户端和 DNSServer 网关侧权威服务器
    - 使用 miekg/dns 库
    - 客户端：Base32 编码数据为子域名发起 DNS 查询
    - 服务端：解码子域名获取上行数据，下行数据编码进 TXT/CNAME 记录
    - 实现 TransportConn 接口：Send、Recv、Close、Type（返回 TransportDNS=4）、RTT、RemoteAddr、MaxDatagramSize（返回 ~110）
    - DNS 查询超时或解析失败返回明确错误
    - Send 方法在数据超过 MaxDatagramSize 时返回错误（由上层 FEC 保证分片不超限）
    - _需求: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9_
  - [x]* 6.2 编写属性测试：DNS Base32 编码往返一致性
    - **Property 2: DNS Base32 编码往返一致性**
    - 使用 rapid 生成长度不超过 DNS 子域名限制的随机字节切片
    - 验证 Base32 编码为子域名后再解码还原出原始字节切片
    - **验证: 需求 3.2**

- [x] 7. 实现 ICMP eBPF 数据面（C）
  - [x] 7.1 创建 `mirage-gateway/bpf/icmp_tunnel.c`，实现 ICMP Tunnel eBPF 程序
    - 定义 icmp_config、icmp_tx_entry、icmp_rx_event 结构体
    - 定义 eBPF Maps：icmp_config_map（HASH）、icmp_tx_map（QUEUE）、icmp_data_events（RINGBUF）
    - 实现 TC egress 函数 `icmp_tunnel_egress`：从 icmp_tx_map 读取数据，构造 ICMP Echo Request 包
    - 实现 TC ingress 函数 `icmp_tunnel_ingress`：截获匹配的 ICMP Echo Reply，提取 Payload 通过 Ring Buffer 上报
    - 无匹配配置时返回 TC_ACT_OK 放行
    - _需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.7_

- [x] 8. 实现 ICMPTransport Go 控制面
  - [x] 8.1 创建 `mirage-gateway/pkg/gtunnel/icmp_transport.go`，实现 ICMPTransport 结构体
    - 通过 eBPF Loader 加载 icmp_tunnel.o 并获取 Map 引用
    - Send 方法：将数据写入 icmp_tx_map
    - Recv 方法：从 icmp_data_events Ring Buffer 读取数据
    - Close 方法：清理 eBPF 资源
    - 实现 TransportConn 接口：Type（返回 TransportICMP=3）、RTT、RemoteAddr、MaxDatagramSize（返回 1024）
    - ICMP 通道不可达时返回错误并释放 eBPF 资源
    - _需求: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 2.10_

- [x] 9. Checkpoint — 确保所有测试通过
  - 确保所有测试通过，如有疑问请询问用户。

- [x] 10. 实现 Orchestrator 调度器核心
  - [x] 10.1 在 `mirage-gateway/pkg/gtunnel/orchestrator.go` 中实现 Orchestrator 结构体核心逻辑
    - 定义 OrchestratorState 状态机（Probing、Active、Degrading、Promoting）
    - 定义 ManagedPath（含 Phase 字段）、PriorityLevel 结构体
    - 新增 epoch uint32 字段，每次路径切换递增
    - 维护四级权重优先级：Level 0=QUIC, Level 1=WebRTC, Level 2=WSS, Level 3=ICMP/DNS
    - 实现 Start 方法：分阶段 HappyEyeballs 探测
    - 实现 Send/Recv 方法：通过活跃路径收发数据，集成 FECProcessor 和 PathScheduler
    - 实现 notifyFECMTU 方法：路径切换时读取 MaxDatagramSize() 通知 FEC 调整分片大小
    - 实现 Close 方法
    - _需求: 5.1, 5.2, 5.5, 5.6, 5.10_
  - [x] 10.2 实现降格逻辑（demote 方法）
    - 集成 LinkAuditor，丢包率 > 30% 或 RTT > 200% 基线时触发降格
    - 降格时递增 epoch，在 ShardHeader 中注入新 Epoch 标识
    - 降格期间使用带 Epoch Barrier 的双发选收模式，接收端收到新 Epoch 后冻结旧通道数据
    - 降格完成后调用 notifyFECMTU 通知 FEC 调整分片大小
    - _需求: 5.8, 5.9_
  - [x] 10.3 实现心跳探测与升格逻辑（probeLoop、promote 方法）
    - 默认 30s 探测周期，Level 3 时缩短为 15s
    - 高优先级路径连续 3 次探测成功触发升格
    - 升格时递增 epoch，使用带 Epoch Barrier 的双发选收模式
    - 升格完成后调用 notifyFECMTU 通知 FEC 调整分片大小
    - 升格失败立即回退原路径并重置探测计数器
    - _需求: 6.1, 6.2, 6.3, 6.4, 6.5_
  - [x] 10.4 实现分阶段 happyEyeballs 方法
    - Phase 1：并发 Goroutine 竞速探测无信令依赖的协议（QUIC、WSS、ICMP、DNS），WebRTC 不参与
    - Phase 2：WSS 探测成功后，后台利用 WSS 作为信令通道拉起 WebRTC 连通性测试
    - WebRTC 打洞成功且优先级高于当前活跃路径时，通过 Promote 升格
    - 仅探测配置中启用的协议，跳过禁用协议
    - _需求: 5.1, 5.2, 5.3, 5.4, 9.4_
  - [x]* 10.5 编写属性测试：HappyEyeballs Phase 1 不包含 WebRTC
    - **Property 3: HappyEyeballs Phase 1 不包含 WebRTC**
    - 使用 rapid 生成随机传输协议启用/禁用配置组合
    - 验证 Phase 1 实际探测的协议集合中不包含 WebRTC
    - **验证: 需求 1.4, 5.2**
  - [x]* 10.6 编写属性测试：HappyEyeballs Phase 1 选择最快协议
    - **Property 4: HappyEyeballs Phase 1 选择最快协议**
    - 使用 rapid 生成一组启用的 mock TransportConn（排除 WebRTC，各自具有随机正延迟）
    - 验证竞速完成后选择的协议为延迟最小的那个
    - **验证: 需求 5.5**
  - [x]* 10.7 编写属性测试：禁用协议跳过探测
    - **Property 7: 禁用协议跳过探测**
    - 使用 rapid 生成随机传输协议启用/禁用配置组合
    - 验证 Orchestrator 实际探测的协议集合严格等于配置中启用的协议集合（WebRTC 仅在 Phase 2）
    - **验证: 需求 9.4**
  - [x]* 10.8 编写属性测试：Epoch Barrier 切断拖尾污染
    - **Property 9: Epoch Barrier 切断拖尾污染**
    - 使用 rapid 生成随机 epoch 值和混合新旧 epoch 的 Shard 序列
    - 验证接收端收到新 epoch 后，所有 epoch ≤ 旧值的 Shard 被丢弃
    - **验证: 需求 5.9**
  - [x]* 10.9 编写属性测试：动态 MTU 约束分片大小
    - **Property 10: 动态 MTU 约束分片大小**
    - 使用 rapid 生成随机原始数据和随机 MaxDatagramSize 值
    - 验证 FECProcessor 生成的每个 Shard 序列化后字节长度不超过 MaxDatagramSize
    - **验证: 需求 4.7**

- [x] 11. Shard 序列化兼容性验证
  - [x]* 11.1 编写属性测试：Shard 序列化往返一致性
    - **Property 1: Shard 序列化往返一致性**
    - 使用 rapid 生成合法的 Shard 对象（随机 Index、Data、IsParity）和任意 packetID
    - 验证 SerializeShard 后再 DeserializeShard 还原出等价的 Shard 和相同的 packetID
    - **验证: 需求 8.3, 8.4**

- [x] 12. 集成与接线
  - [x] 12.1 将 Orchestrator 集成到现有 Tunnel 启动流程
    - 在 `tunnel.go` 中引入 Orchestrator 替代直接使用 PathScheduler
    - Orchestrator 通过 TransportConn 接口与所有传输协议交互
    - _需求: 4.3, 4.4, 4.5, 8.1, 8.2_
  - [x] 12.2 将 ICMP eBPF 程序集成到 Loader 加载链
    - 在 `loader.go` 中添加 icmp_tunnel.o 的加载逻辑（作为 follower program，非 critical）
    - 通过 MapReplacements 共享必要的共享 Map
    - 加载失败时标记 ICMP 不可用，降级运行
    - _需求: 2.2, 7.1, 7.2_
  - [x]* 12.3 编写集成测试：Orchestrator 降格/升格端到端流程
    - 使用 mock TransportConn 模拟路径劣化触发降格、恢复触发升格
    - 验证状态机转换正确性
    - _需求: 5.5, 5.6, 6.2, 6.3, 6.5_

- [x] 13. 最终 Checkpoint — 确保所有测试通过
  - 确保所有测试通过，如有疑问请询问用户。

## 备注

- 标记 `*` 的子任务为可选，可跳过以加速 MVP 交付
- 每个任务引用了具体的需求条款以确保可追溯性
- Checkpoint 任务确保增量验证
- 属性测试验证设计文档中定义的 10 个正确性属性
- 单元测试验证具体示例和边界条件
- ICMP 数据面使用 C (eBPF TC Hook)，控制面使用 Go，通过 eBPF Map + Ring Buffer 通信
