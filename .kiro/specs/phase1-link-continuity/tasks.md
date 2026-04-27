# 实施计划：Phase 1 链路连续性闭环

## 概述

按 M1→M2→M3→M4→证据 五个里程碑递进实施。所有代码级测试使用 mock transport，PBT 使用 `pgregory.net/rapid`，最少 100 次迭代。本 spec 不新增功能，只补测试、冻结文档、沉淀证据。

关键事实约束：
- Gateway 承载优先级：QUIC(0) > WebRTC(1) > WSS(2) > ICMP(3) = DNS(3)
- ClientOrchestrator 产品实际 FallbackTimeout = 10s（构造默认值 3s，ProbeAndConnect 传入 10s）
- switchWithTransaction 无 Rollback 调用路径（switchRollbackFn 已注册但未使用）
- adoptConnection 在有 ClientOrchestrator 时替换其内部 active，transport 本身不变
- Reconnect 中 disconnectStart 在进入 StateReconnecting 时记录，首次 Evaluate 几乎总是 PhaseJitter
- doReconnect 内部各级 probe 使用独立 5s context timeout，RecoveryFSM 总超时 60s / 单阶段 15s

## 任务

- [x] 1. M1：承载矩阵冻结
  - [x] 1.1 创建 `docs/governance/carrier-matrix.md`
    - 记录 QUIC/WSS/WebRTC/ICMP/DNS 五种承载的 Gateway 侧与 Client 侧实现状态
    - 标注"正式承诺"（QUIC、WSS）与"已接线待闭环"（WebRTC、ICMP、DNS）
    - 区分构造默认值与产品实际运行值：FallbackTimeout 构造默认 3s / 产品实际 10s，ProbeInterval 30s，PromoteThreshold 3
    - 承载优先级按真相源：QUIC > WebRTC > WSS > ICMP/DNS
    - _需求: 1.1, 1.2, 1.3, 1.4, 1.5_

  - [x] 1.2 回写 `docs/governance/capability-truth-source.md` 主证据锚点
    - 在"多承载编排与降级"能力域的主证据锚点列表中添加 `docs/governance/carrier-matrix.md`
    - _需求: 1.6, 9.1_

- [x] 2. M2：ClientOrchestrator 降级/回升测试
  - [x] 2.1 新建 `phantom-client/pkg/gtclient/client_orchestrator_test.go`
    - 创建 `mockTransport` 结构体（实现 Transport 接口：SendDatagram/ReceiveDatagram/IsConnected/Close）
    - 编写降级集成测试：QUIC 失败→WSS 降级→数据传输→QUIC 恢复→回升→数据传输，验证全链路不中断
    - 编写 QUIC+WSS 全失败测试：验证返回 "all transports failed" 错误
    - 编写降级/回升日志事件验证测试（基于当前日志内容断言：降级时验证"QUIC 拨号失败"和"WSS 降级路径已建立"消息存在；回升时验证"QUIC 探测成功"计数和"已回升到 QUIC 主路径"消息存在；不要求结构化 source/target 字段或切换耗时字段）
    - _需求: 2.1, 2.4, 2.5, 2.6, 2.7, 2.8_

  - [x] 2.2 编写 Property 2: ClientOrchestrator 降级正确性 PBT
    - **Property 2: ClientOrchestrator degradation correctness**
    - 测试函数: `TestProperty_DegradationCorrectness`
    - 使用 `rapid` 生成随机 FallbackTimeout（1ms-5s），QUIC 始终失败 + WSS 成功时 ActiveType 必须为 "wss"
    - **验证: 需求 2.1**

  - [x] 2.3 编写 Property 3: ClientOrchestrator 回升正确性 PBT
    - **Property 3: ClientOrchestrator promotion correctness**
    - 测试函数: `TestProperty_PromotionCorrectness`
    - 使用 `rapid` 生成随机 PromoteThreshold（1-10），WSS 降级后 QUIC 探测连续成功 N 次后 ActiveType 必须为 "quic"
    - **验证: 需求 2.3**

- [x] 3. M2：Gateway Orchestrator 优先级测试
  - [x] 3.1 扩展 `mirage-gateway/pkg/gtunnel/orchestrator_test.go`
    - 添加从低到高逐步接入场景测试（DNS→ICMP→WSS→WebRTC→QUIC），验证每次接入后活跃路径正确切换
    - 添加 auditor.ShouldDegrade 触发降级到下一可用承载的测试（注意：当前实现中连接断开不直接触发降级，降级入口是 probeLoop → auditor.ShouldDegrade → demote 链路）
    - 优先级必须按 QUIC(0) > WebRTC(1) > WSS(2) > ICMP(3) = DNS(3)
    - _需求: 3.1, 3.2, 3.3, 3.4_

  - [x] 3.2 编写 Property 4: Orchestrator 优先级排序 PBT
    - **Property 4: Orchestrator priority ordering**
    - 测试函数: `TestProperty_PriorityOrdering`
    - 使用 `rapid` 生成随机协议接入顺序排列组合，AdoptInboundConn 后 GetActiveType 始终返回最高优先级类型
    - **验证: 需求 3.1, 3.2**

  - [x] 3.3 编写 Property 5: Orchestrator Send 路由正确性 PBT
    - **Property 5: Orchestrator Send routing correctness**
    - 测试函数: `TestProperty_SendRoutingCorrectness`
    - 使用 `rapid` 生成随机注入顺序和随机 payload，Send 后仅活跃路径 sendCount 增加，非活跃路径不变
    - **验证: 需求 3.5**

- [x] 4. M3：RecoveryFSM 测试
  - [x] 4.1 新建 `phantom-client/pkg/gtclient/recovery_fsm_test.go`
    - 编写阶段边界 example 测试：验证 < 5s → PhaseJitter，5s-30s → PhasePressure，≥ 30s → PhaseDeath
    - 编写阶段递进测试：PhaseJitter 失败→自动升级 PhasePressure→失败→自动升级 PhaseDeath
    - 编写恢复成功返回 RecoveryResult 测试：验证包含成功阶段、总耗时、尝试次数
    - 编写 Reconnect 首次 Evaluate 行为测试：验证 disconnectStart 在 StateReconnecting 时记录，首次几乎总是 PhaseJitter
    - _需求: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 4.1_

  - [x] 4.2 编写 Property 1: RecoveryFSM.Evaluate 单调性与边界正确性 PBT
    - **Property 1: RecoveryFSM.Evaluate monotonicity and boundary correctness**
    - 测试函数: `TestProperty_EvaluateMonotonicity`
    - 使用 `rapid` 生成随机 d1, d2（0-120s），若 d1 < d2 则 Evaluate(d1) ≤ Evaluate(d2)；同时验证三段边界
    - **验证: 需求 5.1, 5.2, 5.3, 5.7**

- [x] 5. M3：Resolver PBT 扩展
  - [x] 5.1 扩展 `phantom-client/pkg/resonance/resolver_test.go`
    - 添加仅配置部分通道时只启动已配置通道的测试
    - 添加单通道超时不阻塞其余通道的测试
    - _需求: 6.1, 6.5, 6.6_

  - [x] 5.2 编写 Property 6: Resolver First-Win 竞速 PBT
    - **Property 6: Resolver First-Win racing**
    - 测试函数: `TestProperty_FirstWinRacing`
    - 使用 `rapid` 生成随机通道延迟组合（至少一个成功），Resolve 返回成功且 ResolvedSignal 包含有效 Gateways 和 Channel
    - **验证: 需求 6.2, 6.3**

  - [x] 5.3 编写 Property 7: Resolver 全失败聚合错误 PBT
    - **Property 7: Resolver all-fail aggregated error**
    - 测试函数: `TestProperty_AllFailAggregatedError`
    - 使用 `rapid` 生成随机通道配置（全部失败），Resolve 返回 error 且包含所有通道错误信息
    - **验证: 需求 6.4**

- [x] 6. M3：节点阵亡恢复演练测试
  - [x] 6.1 新建 `phantom-client/pkg/gtclient/node_death_drill_test.go`
    - 模拟完整链路：主节点失效→Reconnect 创建 RecoveryFSM→从 PhaseJitter 起步→阶段递进到 PhaseDeath→doReconnect L1(RuntimeTopo) 失败→L2(BootstrapPool) 失败→L3 Resolver 发现新入口→switchWithTransaction 完成切换→数据通道恢复
    - 验证恢复后立即触发 triggerImmediateTopoPull
    - 验证所有策略失败时返回 "all reconnection strategies exhausted"
    - 验证超时口径：RecoveryFSM 总 60s / 单阶段 15s，doReconnect 内部 probe 5s
    - 使用 mock transport + mock HTTP server 模拟 Resolver 通道
    - _需求: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_

- [x] 7. M4：switchWithTransaction + 业务连续性样板测试
  - [x] 7.1 新建 `phantom-client/pkg/gtclient/business_continuity_test.go`
    - 编写同 IP 切换测试：newIP == oldIP 时直接 adoptConnection，不触发 PreAdd/Commit
    - 编写 PreAdd 失败测试：关闭新 engine，返回错误，不修改当前活跃连接（注意：无 Rollback 调用路径）
    - 编写 PreAdd→adoptConnection→Commit 正常流程测试
    - 编写 adoptConnection 在有 ClientOrchestrator 时的行为测试：替换 Orchestrator 内部 active，transport 本身不变
    - 编写业务连续性样板测试：模拟持续请求流，记录切换前后发送/接收字节数、请求序号，区分"传输层切换成功"与"业务层影响量化"（不预设"业务层无感"结论，由实际测试数据决定边界）
    - 编写切换前后 currentGW 状态一致性验证
    - _需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 8.1, 8.2, 8.3, 8.4, 8.5_

  - [x] 7.2 编写 Property 8: switchWithTransaction 同 IP 幂等性 PBT
    - **Property 8: switchWithTransaction same-IP idempotency**
    - 测试函数: `TestProperty_SameIPIdempotency`
    - 使用 `rapid` 生成随机 IP/Port/Region，当 newIP == oldIP 时验证不调用 switchPreAddFn/switchCommitFn
    - **验证: 需求 8.1**

  - [x] 7.3 编写 Property 9: switchWithTransaction 后状态一致性 PBT
    - **Property 9: switchWithTransaction post-state consistency**
    - 测试函数: `TestProperty_PostStateConsistency`
    - 使用 `rapid` 生成随机网关信息，成功切换后 currentGW 的 IP/Port/Region 与输入一致；若 transport 为 ClientOrchestrator 则验证 Orchestrator 内部 active 已替换但 transport 本身不变
    - **验证: 需求 8.4, 8.5**

- [x] 8. 证据沉淀：受控演练产物
  - [x] 8.1 创建 `deploy/scripts/drill-m2-degradation.sh`
    - M2 降级/回升受控演练脚本
    - 步骤：① 启动 mock gateway（或使用 docker-compose 启动真实 gateway + client 对）② 触发 QUIC 不可达条件 ③ 验证 WSS 降级 ④ 恢复 QUIC ⑤ 验证回升
    - 同时执行代码级 `go test -run TestDegradation -v` 作为基线证据
    - 捕获运行日志到 `deploy/evidence/m2-degradation-drill.log`
    - 脚本退出码反映测试结果，可直接用于 CI gate
    - 注意：当前版本的受控演练仍基于 mock transport，证据强度为"代码级模拟"；若后续需要升级为真实网络演练，需单独立任务
    - _需求: 2.4, 2.7_

  - [x] 8.2 创建 `deploy/scripts/drill-m3-node-death.sh`
    - M3 节点阵亡恢复受控演练脚本
    - 步骤：① 执行 `go test -run TestNodeDeathDrill -v` ② 捕获 RecoveryFSM 各阶段日志 ③ 输出恢复成功/超时/失败回退判定结果
    - 捕获运行日志到 `deploy/evidence/m3-node-death-drill.log`
    - 注意：同上，当前为代码级模拟证据
    - _需求: 4.5, 4.6, 4.8_

  - [x] 8.3 创建 `deploy/scripts/drill-m4-continuity.sh`
    - M4 业务连续性样板受控演练脚本
    - 步骤：① 执行 `go test -run TestBusinessContinuity -v` ② 捕获切换前后数据流状态 ③ 生成分层结论报告到 `deploy/evidence/m4-continuity-report.md`
    - 捕获运行日志到 `deploy/evidence/m4-continuity-drill.log`
    - 注意：同上，当前为代码级模拟证据
    - _需求: 7.5, 7.6_

  - [x] 8.4 创建 `deploy/evidence/m4-continuity-report.md`
    - M4 业务连续性样板受控演练报告（由 drill-m4-continuity.sh 生成或手动补充）
    - 内容：传输层切换结果、业务层影响量化数据（丢包数、恢复时间）、当前版本业务连续性边界说明
    - 此文件为 capability-truth-source 回写"会话连续性与链路漂移"能力域的直接证据锚点
    - _需求: 7.5, 7.6_

  - [x] 8.5 创建 `deploy/evidence/README.md`
    - 说明 evidence 目录结构、各日志文件用途、各报告文件用途、复验命令
    - 作为 Phase 1 受控演练报告的索引入口
    - 明确标注当前证据强度等级：代码级模拟（mock transport），非真实网络演练

- [x] 9. 证据沉淀：验证清单回写
  - [x] 9.1 回写 `docs/Mirage 功能确认与功能验证任务清单.md`
    - 在功能验证任务表中新增"自动降级/回升演练"条目（复验命令指向 drill-m2-degradation.sh、通过标准、证据文件路径 deploy/evidence/m2-*.log）
    - 新增"节点阵亡恢复演练"条目（复验命令指向 drill-m3-node-death.sh）
    - 新增"业务连续性样板"条目（复验命令指向 drill-m4-continuity.sh，证据文件包含 deploy/evidence/m4-continuity-drill.log 和 deploy/evidence/m4-continuity-report.md）
    - 每个条目必须包含可独立执行的复验命令和证据文件路径
    - _需求: 9.2, 9.3, 9.4_

  - [x] 9.2 回写 `docs/governance/capability-truth-source.md`
    - 根据 M2/M3/M4 实际验证结果，评估"多承载编排与降级"和"节点恢复与共振发现"两个能力域是否满足状态升级条件
    - 若验证通过，更新当前真实能力描述和状态等级
    - 若不足以升级，维持当前状态并在主证据锚点中记录差距说明
    - 回写"会话连续性与链路漂移"能力域的当前真实能力描述（基于 M4 业务连续性样板结果，明确标注"传输层切换"与"业务层影响量化"的边界，不预设"业务层无感"结论）
    - _需求: 7.7, 9.5, 9.6_

## 备注

- 所有 PBT 子任务为必须项（非可选），确保 requirements/design 的 traceability 完整
- 每个任务引用具体需求编号，确保可追溯
- PBT 任务标注对应 design property 编号
- Property test 使用 `pgregory.net/rapid`，最少 100 次迭代
- 所有代码级测试使用 mock transport，不依赖真实网络连接
- Gateway 侧测试落到 `mirage-gateway/pkg/gtunnel/orchestrator_test.go`，不再往 `cmd/gateway/protocol_chain_integration_test.go` 堆
- Gateway 侧降级验收基于 auditor.ShouldDegrade → demote 链路，不假设连接断开自动触发降级
- ClientOrchestrator 日志验收基于当前粗粒度消息断言，不要求新增结构化字段
- M4 不预设"业务层无感"结论，由实际测试数据决定边界描述
- 受控演练产物（drill script / evidence logs / report / README）为治理状态升级的必须交付物
- 当前证据强度等级为"代码级模拟"（mock transport），capability-truth-source 回写时必须如实标注此边界；若需升级为真实网络演练证据，应单独立后续任务
