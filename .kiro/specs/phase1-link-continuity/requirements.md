# 需求文档：Phase 1 链路连续性闭环

## 简介

本需求文档覆盖北极星实施计划 Phase 1（链路连续性闭环），目标是将三个能力域从"设计存在"推进为"链路连续性闭环"，为 `Hyper-Resilient Overlay Network` 北极星目标提供首批端到端验证证据。

Phase 1 包含四个里程碑（M1-M4），覆盖三个能力域：
- 多承载编排与降级（M1、M2）
- 节点恢复与共振发现（M3）
- 会话连续性与链路漂移（M4）

本 spec 严格遵守 `capability-truth-source.md` 的治理规则：只安排测试补齐、验证闭环、编排收口、证据沉淀和配置/部署基线固化，不扩展产品边界，不新增功能。

## 术语表

- **Orchestrator**: Gateway 侧多协议编排器，位于 `mirage-gateway/pkg/gtunnel/orchestrator.go`，支持 QUIC/WSS/WebRTC/ICMP/DNS 被动模式接入
- **ClientOrchestrator**: Client 侧多协议编排器，位于 `phantom-client/pkg/gtclient/client_orchestrator.go`，实现 QUIC→WSS 降级/回升
- **Transport**: 统一传输接口，定义于 `phantom-client/pkg/gtclient/client.go`，包含 SendDatagram/ReceiveDatagram/IsConnected/Close
- **RecoveryFSM**: 恢复状态机，位于 `phantom-client/pkg/gtclient/recovery_fsm.go`，实现 Jitter→Pressure→Death 三阶段恢复
- **Resolver**: 信令共振发现器，位于 `phantom-client/pkg/resonance/resolver.go`，支持 DoH/Gist/Mastodon 并发竞速（First-Win-Cancels-All）
- **switchWithTransaction**: 两阶段事务式切换（PreAdd→adoptConnection→Commit），位于 `GTunnelClient`。注意：当前实现无 Rollback 调用路径，switchRollbackFn 已注册但未被使用
- **Carrier_Matrix**: 承载矩阵，记录各承载协议的正式承诺状态与支持边界
- **Degradation_Level**: 降级等级，L1_Normal→L2_Degraded→L3_LastResort
- **RuntimeTopology**: 运行时拓扑池，由 OS 控制面下发，区别于静态 BootstrapPool
- **Capability_Truth_Source**: 能力真相源文档 `docs/governance/capability-truth-source.md`，定义验收标准与状态等级

## 需求

### 需求 1：承载矩阵冻结（M1）

**用户故事：** 作为项目治理者，我希望冻结当前版本的承载矩阵，以便明确哪些承载是正式承诺、哪些是"已接线待闭环"，避免把设计入口误写成稳定承载能力。

#### 验收标准

1. THE Carrier_Matrix SHALL 记录以下承载协议的当前状态：QUIC、WSS、WebRTC、ICMP、DNS
2. WHEN 一个承载协议在 Gateway 侧 Orchestrator 和 Client 侧 ClientOrchestrator 均有完整实现时，THE Carrier_Matrix SHALL 将该承载标注为"正式承诺"
3. WHEN 一个承载协议仅在 Gateway 侧有路径入口但 Client 侧未实现端到端闭环时，THE Carrier_Matrix SHALL 将该承载标注为"已接线待闭环"
4. THE Carrier_Matrix SHALL 包含 QUIC→WSS 降级边界的完整描述，区分构造默认值（FallbackTimeout 3s）与产品实际运行值（ProbeAndConnect 传入 FallbackTimeout 10s），回升探测间隔（30s）和回升阈值（连续 3 次成功）
5. THE Carrier_Matrix SHALL 为 WebRTC、ICMP、DNS 三个承载分别标注 Gateway 侧实现状态和 Client 侧实现状态
6. WHEN Carrier_Matrix 完成冻结后，THE Capability_Truth_Source SHALL 在"多承载编排与降级"能力域的主证据锚点中引用该矩阵文档

### 需求 2：自动降级/回升端到端验证（M2）

**用户故事：** 作为发布验证者，我希望至少有一条发布级的自动降级与回升链路通过端到端验证，以便证明多承载编排不只是"代码存在"而是"行为闭环"。

#### 验收标准

1. WHEN QUIC 主路径不可达时，THE ClientOrchestrator SHALL 在 FallbackTimeout（默认 3s）内自动降级到 WSS 路径
2. WHILE ClientOrchestrator 处于 WSS 降级状态时，THE ClientOrchestrator SHALL 以 ProbeInterval（默认 30s）间隔持续探测 QUIC 可用性
3. WHEN QUIC 探测连续成功达到 PromoteThreshold（默认 3 次）时，THE ClientOrchestrator SHALL 自动回升到 QUIC 主路径并关闭旧 WSS 连接
4. THE Degradation_Drill_Test SHALL 验证完整的降级→数据传输→回升→数据传输链路，确认降级和回升过程中数据通道不中断
5. WHEN 降级发生时，THE ClientOrchestrator SHALL 通过日志记录降级事件（当前实现打印粗粒度消息如"QUIC 拨号失败"和"WSS 降级路径已建立"，不含结构化 source/target 字段；验收基于当前日志内容断言，不要求新增字段）
6. WHEN 回升发生时，THE ClientOrchestrator SHALL 通过日志记录回升事件（当前实现打印探测成功计数如"QUIC 探测成功 (N/M)"和"已回升到 QUIC 主路径"，不含结构化切换耗时字段；验收基于当前日志内容断言）
7. THE Degradation_Drill_Test SHALL 产出可复验的运行日志，作为功能验证清单的回写证据
8. IF QUIC 和 WSS 均不可达，THEN THE ClientOrchestrator SHALL 返回明确错误（"all transports failed"），不进入静默失败状态

### 需求 3：Gateway 侧协议优先级与被动接入验证（M2 补充）

**用户故事：** 作为发布验证者，我希望验证 Gateway 侧 Orchestrator 的协议优先级排序和被动接入机制，以便确认 Gateway 侧多承载主链的正确性。

#### 验收标准

1. THE Orchestrator SHALL 按 QUIC > WebRTC > WSS > ICMP/DNS 的优先级排序管理活跃路径（QUIC=0 > WebRTC=1 > WSS=2 > ICMP=DNS=3）
2. WHEN 一个更高优先级的承载连接通过 AdoptInboundConn 接入时，THE Orchestrator SHALL 自动将活跃路径切换到该高优先级承载
3. WHEN auditor.ShouldDegrade 判定当前活跃路径质量不达标时，THE Orchestrator SHALL 通过 demote() 降级到下一个可用的低优先级承载（注意：当前实现中连接断开不会直接触发降级，降级入口是 probeLoop → auditor.ShouldDegrade → demote 链路）
4. THE Protocol_Priority_Test SHALL 验证从低优先级到高优先级的逐步接入场景，确认每次接入后活跃路径正确切换
5. THE Orchestrator SHALL 通过 Send 方法将数据路由到当前活跃路径，不泄漏到非活跃路径

### 需求 4：节点阵亡恢复演练（M3）

**用户故事：** 作为发布验证者，我希望证明"发现器存在"已升级为"节点阵亡后可恢复"，以便将"节点恢复与共振发现"能力域从"部分实现"推进为有实证闭环。

#### 验收标准

1. WHEN 主节点不可达时，THE GTunnelClient.Reconnect SHALL 创建 RecoveryFSM 并从当前断连时长对应的阶段开始执行恢复（注意：Reconnect 中 disconnectStart 在进入 StateReconnecting 时记录，因此首次 Evaluate 几乎总是从 PhaseJitter 起步，通过阶段递进最终到达 PhaseDeath 触发 doReconnect 执行 L1→L2→L3 降级恢复）
2. WHEN L1（RuntimeTopology）和 L2（BootstrapPool）均不可达时，THE GTunnelClient SHALL 调用 Resolver 执行信令共振发现（L3 绝境复活）
3. WHEN Resolver 通过 DoH、Gist 或 Mastodon 任一通道发现新入口时，THE GTunnelClient SHALL 使用发现的网关信息执行 switchWithTransaction 完成切换
4. WHEN 信令共振发现成功且建链成功后，THE GTunnelClient SHALL 立即触发一次 triggerImmediateTopoPull 刷新运行时拓扑
5. THE Node_Death_Recovery_Drill SHALL 模拟以下完整链路：主节点失效→RecoveryFSM 升级到 PhaseDeath→L1/L2 失败→Resolver 发现新入口→switchWithTransaction 完成切换→数据通道恢复
6. THE Node_Death_Recovery_Drill SHALL 产出终端日志和事件记录，包含每个恢复阶段的时间戳、尝试次数和结果
7. IF 所有恢复策略（L1/L2/L3）均失败，THEN THE GTunnelClient SHALL 返回明确错误（"all reconnection strategies exhausted"），不进入无限重试
8. THE Node_Death_Recovery_Drill SHALL 定义恢复成功、超时和失败回退的判定标准。注意超时口径：RecoveryFSM 总超时 60s（单阶段 15s），但 doReconnect 内部各级 probe 使用独立的 5s context timeout

### 需求 5：RecoveryFSM 三阶段恢复正确性（M3 补充）

**用户故事：** 作为发布验证者，我希望验证 RecoveryFSM 的三阶段恢复逻辑正确性，以便确认恢复状态机的阶段判定和递进执行符合设计。

#### 验收标准

1. WHEN 断连时长 < 5s 时，THE RecoveryFSM SHALL 判定为 PhaseJitter
2. WHEN 断连时长 >= 5s 且 < 30s 时，THE RecoveryFSM SHALL 判定为 PhasePressure
3. WHEN 断连时长 >= 30s 时，THE RecoveryFSM SHALL 判定为 PhaseDeath
4. WHEN PhaseJitter 恢复失败时，THE RecoveryFSM SHALL 自动升级到 PhasePressure 继续执行
5. WHEN PhasePressure 恢复失败时，THE RecoveryFSM SHALL 自动升级到 PhaseDeath 继续执行
6. WHEN 任一阶段恢复成功时，THE RecoveryFSM SHALL 返回 RecoveryResult，包含成功阶段、总耗时和尝试次数
7. FOR ALL 有效的断连时长值，RecoveryFSM.Evaluate 的阶段判定 SHALL 满足单调递增性：PhaseJitter < PhasePressure < PhaseDeath 对应的时长阈值严格递增

### 需求 6：Resolver 并发竞速发现正确性（M3 补充）

**用户故事：** 作为发布验证者，我希望验证 Resolver 的 First-Win-Cancels-All 竞速机制正确性，以便确认信令共振发现在节点阵亡场景下可靠工作。

#### 验收标准

1. WHEN 多个通道（DoH/Gist/Mastodon）同时启动时，THE Resolver SHALL 并发执行所有已配置通道
2. WHEN 任一通道率先返回有效信令时，THE Resolver SHALL 立即取消其余通道并返回该信令结果
3. THE ResolvedSignal SHALL 包含发现的网关列表（IP/Port/Priority）、域名列表、成功通道名称和延迟时间
4. IF 所有已配置通道均失败，THEN THE Resolver SHALL 返回包含所有通道错误信息的聚合错误
5. WHEN 仅配置了部分通道时，THE Resolver SHALL 只启动已配置的通道，不因未配置通道而失败
6. THE Resolver SHALL 为每个通道设置独立超时（默认 ChannelTimeout 10s），单通道超时不阻塞其余通道

### 需求 7：业务连续性样板验证（M4）

**用户故事：** 作为发布验证者，我希望至少有一条业务样板证明"业务连续"而不只是"连接存在"，以便将"会话连续性与链路漂移"能力域的验证从传输层推进到业务层。

#### 验收标准

1. THE Business_Continuity_Sample SHALL 选择至少一种长连接业务模式（TCP 持续请求流或 WebSocket 长连接）作为验证样板
2. WHEN 底层承载发生 switchWithTransaction 切换时，THE Business_Continuity_Sample SHALL 记录切换前后的业务数据流状态（发送字节数、接收字节数、请求序号）
3. THE Business_Continuity_Sample SHALL 区分并记录"传输层切换成功"和"业务层影响量化"两个独立判定结果（不预设"业务层无感"结论，由实际测试数据决定边界）
4. WHEN switchWithTransaction 的 PreAdd 阶段失败时，THE GTunnelClient SHALL 关闭新连接的 QUICEngine 并返回错误，不修改当前活跃连接，业务数据流不中断（注意：当前实现无 Commit 失败回滚路径，switchRollbackFn 已注册但未被调用）
5. THE Business_Continuity_Sample SHALL 产出中断/不中断判定日志，包含切换时间戳、切换耗时、丢包数量和业务恢复时间
6. THE Business_Continuity_Sample SHALL 在结论中明确标注当前版本的业务连续性边界：哪些场景可做到传输层切换，业务层影响的量化数据（丢包数、恢复时间），以及哪些场景的影响边界尚未验证
7. WHEN Business_Continuity_Sample 完成验证后，THE Capability_Truth_Source SHALL 根据实际验证结果回写"会话连续性与链路漂移"能力域的当前真实能力描述

### 需求 8：事务式切换正确性（M4 补充）

**用户故事：** 作为发布验证者，我希望验证 switchWithTransaction 的三阶段事务式切换在各种场景下的正确性，以便确认链路漂移控制机制可靠。

#### 验收标准

1. WHEN 新旧 IP 相同时，THE switchWithTransaction SHALL 直接执行 adoptConnection 而不触发 PreAdd/Commit 流程
2. WHEN PreAdd 阶段失败时，THE switchWithTransaction SHALL 关闭新连接的 QUICEngine 并返回错误，不修改当前活跃连接
3. WHEN PreAdd 成功后，THE switchWithTransaction SHALL 执行 adoptConnection 接管新连接，然后调用 Commit 删除旧路由
4. FOR ALL switchWithTransaction 调用，切换完成后 GTunnelClient 的 currentGW SHALL 更新为新网关信息
5. FOR ALL switchWithTransaction 调用，切换完成后 GTunnelClient 的内部 quic 引用 SHALL 指向新 QUICEngine。若 transport 为 ClientOrchestrator，则 Orchestrator 内部 active 被替换为新 QUICTransportAdapter，transport 本身仍为 ClientOrchestrator 实例不变

### 需求 9：证据沉淀与治理回写（Phase 1 收口）

**用户故事：** 作为项目治理者，我希望 Phase 1 的所有验证结果按治理规则沉淀为正式证据，以便支撑能力状态升级判定和后续阶段推进。

#### 验收标准

1. WHEN M1 完成后，THE Carrier_Matrix 文档 SHALL 被添加到 Capability_Truth_Source 的"多承载编排与降级"主证据锚点列表
2. WHEN M2 完成后，THE 功能验证清单 SHALL 新增"自动降级/回升演练"条目，包含复验命令、通过标准和证据文件路径
3. WHEN M3 完成后，THE 功能验证清单 SHALL 新增"节点阵亡恢复演练"条目，包含复验命令、通过标准和证据文件路径
4. WHEN M4 完成后，THE 功能验证清单 SHALL 新增"业务连续性样板"条目，包含复验命令、通过标准和证据文件路径
5. WHEN Phase 1 全部里程碑完成后，THE Capability_Truth_Source SHALL 根据实际验证结果评估"多承载编排与降级"和"节点恢复与共振发现"两个能力域是否满足状态升级条件
6. IF 验证结果不足以支撑状态升级，THEN THE Capability_Truth_Source SHALL 维持当前状态不变，并在主证据锚点中记录差距说明
