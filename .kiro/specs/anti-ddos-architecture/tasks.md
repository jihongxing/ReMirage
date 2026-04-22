# 任务清单：Mirage 抗 DDoS 架构整改

## 需求 1：多协议感知入口守卫完善

- [x] 1. 入口画像自动同步
  - [x] 1.1 修改 `mirage-gateway/pkg/ebpf/manager.go` DefenseApplier.Start 方法：在 `applyStrategy` 之后调用 `da.SyncIngressProfiles(DefaultIngressProfiles())`，失败时 log.Printf 警告但不阻断启动
  - [x] 1.2 修改 `mirage-gateway/pkg/ebpf/manager.go`：为 DefenseApplier 增加 `UpdateIngressProfiles(profiles []IngressProfile) error` 方法，支持运行时动态更新入口画像（通过 updateCh 异步执行）
  - [x] 1.3 修改 `mirage-gateway/bpf/l1_defense.c` handle_l1_ingress_profile 函数：在丢弃包时递增 `l1_stats.profile_drops` 计数器
  - [x] 1.4 修改 `mirage-gateway/bpf/l1_defense.c` handle_l1_packet_sanity 函数：在丢弃包时递增 `l1_stats.sanity_drops` 计数器
  - [x] 1.5 修改 `mirage-gateway/pkg/ebpf/manager.go`：增加 `ReadL1Stats() (*L1Stats, error)` 方法，从 `l1_stats_map` 读取统计数据（asn_drops / rate_drops / silent_drops / blacklist_drops / sanity_drops / profile_drops / total_checked）

## 需求 2：无状态准入能力

- [x] 2. 无状态准入
  - [x] 2.1 修改 `mirage-gateway/bpf/common.h`：增加 `syn_validation_map`（BPF_MAP_TYPE_LRU_HASH，key=源IP，value=struct syn_state { __u64 cookie; __u64 timestamp; __u8 validated; }，max_entries=131072）和 `syn_config_map`（BPF_MAP_TYPE_ARRAY，max_entries=1，value=struct syn_config { __u32 enabled; __u32 challenge_threshold; }）
  - [x] 2.2 修改 `mirage-gateway/bpf/l1_defense.c`：增加 `handle_l1_syn_validation` 函数 — 当 SYN 包来自未验证 IP 且该 IP 的 SYN 速率超过 challenge_threshold 时，在 `syn_validation_map` 中记录 cookie（基于 saddr+dport+timestamp 的 hash），后续收到该 IP 的 ACK 包时验证 cookie 是否匹配，匹配则标记 validated=1
  - [x] 2.3 修改 `mirage-gateway/bpf/l1_defense.c` handle_l1_defense 主入口：在速率限制检查之前调用 `handle_l1_syn_validation`
  - [x] 2.4 修改 `mirage-gateway/pkg/ebpf/manager.go`：增加 `SyncSynValidationConfig(enabled bool, threshold uint32) error` 方法，将配置写入 `syn_config_map`
  - [x] 2.5 修改 `mirage-gateway/pkg/ebpf/loader.go` sharedMapNames：增加 `"syn_validation_map"` 和 `"syn_config_map"`

## 需求 3：多维准入控制

- [x] 3. 多维准入评分器
  - [x] 3.1 创建 `mirage-gateway/pkg/threat/admission.go`：实现 `AdmissionScorer` 结构体 — 内部 `map[string]*IPScore`（IP → 评分），`IPScore` 包含 NewConnRate / ValidAuthRate / TokenValidRate / ProfileMatchRate / ActiveSessions / LastUpdate 字段；`Score()` 方法计算综合评分（0-100），CGNAT 感知：ActiveSessions > 1 时给予可信度加成
  - [x] 3.2 修改 `mirage-gateway/pkg/threat/admission.go`：为 AdmissionScorer 增加 `RecordNewConn(ip string)` / `RecordAuthResult(ip string, success bool)` / `RecordTokenCheck(ip string, valid bool)` / `RecordProfileMatch(ip string, matched bool)` 方法，每个方法更新对应维度的滑动窗口统计
  - [x] 3.3 修改 `mirage-gateway/pkg/threat/admission.go`：为 AdmissionScorer 增加 `Evaluate(ip string) IngressAction` 方法 — 评分 > 60 返回 ActionPass，30-60 返回 ActionThrottle，< 30 返回 ActionDrop
  - [x] 3.4 修改 `mirage-gateway/pkg/threat/policy.go`：在 IngressPolicy 中集成 AdmissionScorer — Evaluate 方法中当规则匹配结果为 ActionPass 时，额外检查 AdmissionScorer 的评分
  - [x] 3.5 修改 `mirage-gateway/pkg/threat/admission.go`：增加 Prometheus metrics 暴露 — `admission_score_histogram`（评分分布）、`admission_action_total`（各动作计数）

## 需求 4：OS 软防响应链路

- [x] 4. 软防响应闭环
  - [x] 4.1 创建 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：实现 `DDoSResponder` 结构体 — 持有 Registry / DownlinkService 引用；`HandleResourcePressure(ctx, gwID)` 方法调用 `registry.MarkUnderAttack` 并通过 Downlink 通知 Gateway 停止接受新连接
  - [x] 4.2 修改 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：增加 `HandleRecovery(ctx, gwID)` 方法 — 调用 `registry.RecoverFromAttack` 并通过 Downlink 通知 Gateway 恢复接受连接
  - [x] 4.3 修改 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：增加恢复计数器 — 为每个 UNDER_ATTACK 节点维护 `recoveryCounter map[string]int`，每次心跳正常时 +1，连续 3 次达标后触发 HandleRecovery
  - [x] 4.4 修改 `mirage-os/gateway-bridge/pkg/topology/registry.go` UpdateHeartbeatWithMetrics：当 evaluateDDoSState 返回 "UNDER_ATTACK" 时调用 DDoSResponder.HandleResourcePressure，返回 "ONLINE" 时调用 HandleRecovery（需要将 DDoSResponder 注入 Registry 或通过回调解耦）
  - [x] 4.5 修改 `mirage-os/services/provisioning/tier_router.go` AllocateGateway 查询条件：增加 `WHERE status NOT IN ('UNDER_ATTACK', 'DRAINING', 'DEAD')` 过滤
  - [x] 4.6 修改 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：增加 `recoveryLog []RecoveryEvent` 和 `RecoveryEvent` 结构体（GatewayID / EventType / Reason / Timestamp / Duration），所有状态变迁写入日志

## 需求 5：OS 硬断响应链路

- [x] 5. 硬断响应闭环
  - [x] 5.1 修改 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：增加 `HandleNodeDeath(ctx, gwID)` 方法 — 调用 `registry.MarkDead`，获取死亡节点的 CellID，调用 CellScheduler.ActivateStandby 激活替补
  - [x] 5.2 修改 `mirage-os/pkg/strategy/cell_manager.go`：增加 `ActivateStandby(ctx context.Context, cellID string) (*models.Gateway, error)` 方法 — 优先从 Phase=1（温备）选择，温备耗尽则从 Phase=0（冷备）选择，将选中节点 Phase 更新为 2
  - [x] 5.3 修改 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：在 HandleNodeDeath 中替补激活成功后，调用 `publishNewTopology(ctx, cellID)` 发布新路由表 — 查询该 Cell 下所有 ONLINE 节点，构造 RouteTableResponse 并通过 Downlink 推送给关联 Client
  - [x] 5.4 修改 `mirage-os/gateway-bridge/pkg/topology/registry.go` checkTimeouts：当检测到 DEAD 节点时，调用 DDoSResponder.HandleNodeDeath（通过回调或直接引用）
  - [x] 5.5 修改 `mirage-os/gateway-bridge/pkg/topology/registry.go` UpdateHeartbeat：当 DEAD 节点重新发送心跳时，将其状态设为 ONLINE 但 Phase 重置为 1（校准期），不立即分配用户

## 需求 6：Client 运行时拓扑持续学习

- [x] 6. 拓扑持续学习
  - [x] 6.1 创建 `phantom-client/pkg/gtclient/topo_cache.go`：实现 `TopoCache` 结构体 — `Save(resp *RouteTableResponse) error` 将路由表 JSON 序列化后写入本地文件（路径通过构造函数传入）；`Load() (*RouteTableResponse, error)` 从文件读取并反序列化
  - [x] 6.2 修改 `phantom-client/pkg/gtclient/topo.go` TopoRefresher：增加 `cache *TopoCache` 字段，在 `PullOnce` 成功后调用 `cache.Save(resp)`
  - [x] 6.3 修改 `phantom-client/pkg/gtclient/client.go` NewGTunnelClient：在初始化 RuntimeTopology 时，尝试从 TopoCache.Load 加载缓存的路由表作为初始拓扑
  - [x] 6.4 修改 `phantom-client/pkg/gtclient/topo.go` TopoRefresher：确认已有指数退避重试（最大间隔 30 分钟）和连续 3 次失败告警

## 需求 7：Client 失联后原子恢复状态机

- [x] 7. 恢复状态机
  - [x] 7.1 创建 `phantom-client/pkg/gtclient/recovery_fsm.go`：实现 `RecoveryFSM` 结构体 — `RecoveryPhase` 枚举（PhaseJitter / PhasePressure / PhaseDeath）；`Evaluate(disconnectDuration time.Duration) RecoveryPhase` 方法（< 5s → Jitter，5-30s → Pressure，> 30s → Death）
  - [x] 7.2 修改 `phantom-client/pkg/gtclient/recovery_fsm.go`：增加 `Execute(ctx context.Context, phase RecoveryPhase, client *GTunnelClient) error` 方法 — PhaseJitter：在当前连接重试 3 次（间隔 1s）；PhasePressure：触发拓扑刷新 + 同 Cell 切换；PhaseDeath：调用 client.doReconnect（现有 L1→L2→L3）
  - [x] 7.3 修改 `phantom-client/pkg/gtclient/recovery_fsm.go`：每阶段超时 15 秒，总恢复时间不超过 60 秒
  - [x] 7.4 修改 `phantom-client/pkg/gtclient/client.go` Reconnect 方法：集成 RecoveryFSM — 计算断连时长，调用 `fsm.Evaluate` 确定恢复阶段，调用 `fsm.Execute` 执行恢复
  - [x] 7.5 修改 `phantom-client/pkg/gtclient/recovery_fsm.go`：恢复成功后记录 `RecoveryResult`（耗时、使用的恢复级别、尝试次数）

## 需求 8：Survival Orchestrator 集成

- [x] 8. 生存编排器
  - [x] 8.1 创建 `mirage-os/pkg/orchestrator/survival.go`：实现 `SurvivalOrchestrator` 结构体 — `SurvivalState` 枚举（Normal / Degraded / Critical / Emergency）；持有 Registry 引用
  - [x] 8.2 修改 `mirage-os/pkg/orchestrator/survival.go`：增加 `Evaluate()` 方法 — 统计 ONLINE / UNDER_ATTACK / DEAD 节点数，按比例判定系统生存状态
  - [x] 8.3 修改 `mirage-os/pkg/orchestrator/survival.go`：增加 `GetStatus() SurvivalStatus` API 方法 — 返回当前状态、各状态节点数、最近事件列表
  - [x] 8.4 修改 `mirage-os/pkg/orchestrator/survival.go`：增加 `PrioritizeRecovery()` 方法 — 当多节点同时受攻击时，按 Diamond > Platinum > Standard 优先级决定恢复顺序

## 需求 9：State Commit Engine

- [x] 9. 事务化替补引擎
  - [x] 9.1 创建 `mirage-os/pkg/orchestrator/commit_engine.go`：实现 `CommitEngine` 结构体和 `ReplacementTx` 事务结构 — TxID / OldGateway / NewGateway / CellID / Steps / Status / CreatedAt
  - [x] 9.2 修改 `mirage-os/pkg/orchestrator/commit_engine.go`：实现 `ExecuteReplacement(ctx, tx)` 方法 — 顺序执行 Steps，任一步骤失败时逆序回滚已执行步骤
  - [x] 9.3 修改 `mirage-os/pkg/orchestrator/commit_engine.go`：实现 `BuildReplacementTx(oldGW, newGW, cellID)` 方法 — 构造标准替换事务（步骤：摘除旧节点 → 激活新节点 → 更新拓扑 → 发布路由），每步附带回滚函数
  - [x] 9.4 修改 `mirage-os/pkg/orchestrator/commit_engine.go`：增加 `RetryTx(txID)` 方法 — 从日志中加载失败事务并重新执行
  - [x] 9.5 修改 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go` HandleNodeDeath：将直接替补逻辑改为通过 CommitEngine.ExecuteReplacement 执行

## 需求 10：抗 DDoS 预算模型

- [x] 10. 预算模型
  - [x] 10.1 创建 `mirage-os/pkg/orchestrator/budget.go`：实现 `DDoSBudget` 结构体 — `TierBudget` 包含 MaxHotStandby / MaxSwitchPerHour / GuardIntensity / RecoveryPriority / CurrentUsage 字段
  - [x] 10.2 修改 `mirage-os/pkg/orchestrator/budget.go`：实现 `CheckBudget(cellLevel int, action string) (bool, error)` 方法 — 检查指定等级的预算是否允许执行指定动作
  - [x] 10.3 修改 `mirage-os/pkg/orchestrator/budget.go`：实现 `ConsumeBudget(cellLevel int, action string, cost float64)` 方法 — 扣减预算
  - [x] 10.4 修改 `mirage-os/pkg/orchestrator/budget.go`：实现 `GetBudgetStatus() map[int]*TierBudget` API 方法
  - [x] 10.5 修改 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：在 HandleNodeDeath 中调用 CheckBudget 检查是否允许激活替补

## 需求 11：独立恢复发布平面

- [x] 11. 恢复发布平面
  - [x] 11.1 创建 `mirage-os/pkg/recovery/publisher.go`：实现 `RecoveryPublisher` 结构体 — 持有多个 `PublishChannel` 接口（DNS TXT / Gist / Mastodon）
  - [x] 11.2 修改 `mirage-os/pkg/recovery/publisher.go`：实现 `PublishReplacement(ctx, cellID, newGateways)` 方法 — 构造 Resonance Signal 并通过所有通道并发发布
  - [x] 11.3 修改 `mirage-os/pkg/recovery/publisher.go`：实现 `PublishChannel` 接口和 `DNSTXTChannel` / `GistChannel` 实现
  - [x] 11.4 修改 `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go` HandleNodeDeath：在替补激活后调用 RecoveryPublisher.PublishReplacement 发布到恢复平面

## 需求 12：Gateway 预热池与容量分层

- [x] 12. 预热池管理
  - [x] 12.1 修改 `mirage-os/pkg/strategy/cell_manager.go`：实现 `ActivateStandby(ctx, cellID)` 方法 — 优先 Phase=1 温备，其次 Phase=0 冷备，激活后 Phase 更新为 2
  - [x] 12.2 修改 `mirage-os/pkg/strategy/cell_manager.go`：增加 `EnsureStandbyPool(ctx, cellID)` 方法 — 检查温备池节点数，低于阈值（默认 2）时从冷备池补充
  - [x] 12.3 修改 `mirage-os/pkg/strategy/cell_manager.go` checkScaleOut：在现有扩容逻辑中集成 EnsureStandbyPool 调用
  - [x] 12.4 修改 `mirage-os/pkg/strategy/cell_manager.go`：增加 `GetPoolStats(cellID) PoolStats` 方法 — 返回各级池的节点数（active / warm / cold）
  - [x] 12.5 修改 `mirage-os/pkg/strategy/cell_manager.go`：增加 Prometheus metrics — `gateway_pool_size{cell_id, phase}` gauge
