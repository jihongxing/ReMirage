# 需求文档：V2 观测与审计

## 简介

本文档定义 Mirage V2 编排内核的观测与审计层（Observability & Audit Layer）。该层为编排内核提供事务审计记录、五类状态时间线和最小诊断视图，使系统在 V2 上线后具备"可解释、可复盘、可诊断"的能力。

前置依赖：
- Spec 4-1（三层状态模型）：SessionState、LinkState、ControlState、三层状态管理器
- Spec 4-2（Persona Engine）：PersonaManifest（persona_id、version、lifecycle）
- Spec 4-3（Commit Engine）：CommitTransaction（tx_id、tx_type、tx_phase、各阶段状态 JSON）、CommitEngine
- Spec 5-1（Budget Engine）：BudgetDecision（verdict、cost_estimate、deny_reason）
- Spec 5-2（Survival Orchestrator）：SurvivalOrchestrator、TransitionRecord
- Spec 5-3（控制语义层）：ControlEvent、EventDispatcher、EventHandler 接口

涉及模块：mirage-gateway（pkg/orchestrator/audit/）+ mirage-os（DB + API）

## 术语表

- **Audit_Record**：事务审计记录，记录一次 CommitTransaction 的关键决策信息
- **Timeline_Entry**：时间线条目，记录某个观测对象在某一时刻的状态快照
- **Session_Timeline**：会话时间线，记录 Session 的状态变迁历史
- **Link_Health_Timeline**：链路健康时间线，记录 Link 的健康指标变化历史
- **Persona_Version_Timeline**：画像版本时间线，记录 Persona 的版本切换历史
- **Survival_Mode_Timeline**：生存姿态时间线，记录 Survival Mode 的迁移历史
- **Transaction_Timeline**：事务时间线，记录 CommitTransaction 的阶段推进历史
- **Diagnostic_View**：诊断视图，聚合当前系统关键状态的只读快照
- **Audit_Store**：审计存储接口，负责审计记录的持久化与查询
- **Timeline_Store**：时间线存储接口，负责时间线条目的持久化与查询
- **Audit_Collector**：审计采集器，监听 ControlEvent 和 CommitTransaction 生命周期事件并生成审计记录
- **Timeline_Collector**：时间线采集器，监听状态变更事件并生成时间线条目
- **Diagnostic_Aggregator**：诊断聚合器，从各状态管理器实时聚合诊断视图

## 需求

### 需求 1：事务审计记录

**用户故事：** 作为运维人员，我希望每次关键事务都留下完整的审计记录，以便事后复盘决策过程。

#### 验收标准

1. WHEN 一个 CommitTransaction 到达终态（Committed / RolledBack / Failed），THE Audit_Collector SHALL 生成一条 Audit_Record 并写入 Audit_Store
2. THE Audit_Record SHALL 包含以下字段：audit_id（UUID v4）、tx_id、tx_type、initiated_at（事务创建时间）、finished_at（事务完成时间）、initiation_reason（发起原因）、target_state（目标状态 JSON）、budget_verdict（预算判定结果）、flip_success（flip 是否成功）、rollback_triggered（rollback 是否触发）
3. WHEN budget_verdict 为 deny_and_hold 或 deny_and_suspend，THE Audit_Record SHALL 额外包含 deny_reason 字段
4. WHEN tx_phase 为 RolledBack，THE Audit_Record 的 rollback_triggered SHALL 为 true 且 flip_success SHALL 为 false
5. WHEN tx_phase 为 Committed，THE Audit_Record 的 flip_success SHALL 为 true 且 rollback_triggered SHALL 为 false
6. WHEN tx_phase 为 Failed，THE Audit_Record 的 flip_success SHALL 为 false 且 rollback_triggered SHALL 为 false
7. THE Audit_Store SHALL 支持按 tx_type、时间范围、rollback_triggered 过滤查询
8. THE Audit_Record SHALL 支持 JSON 序列化，时间戳格式为 RFC 3339

### 需求 2：Session 时间线

**用户故事：** 作为运维人员，我希望查看某个 Session 的完整状态变迁历史，以便追踪会话生命周期。

#### 验收标准

1. WHEN SessionStateManager.TransitionState 成功执行，THE Timeline_Collector SHALL 生成一条 Session_Timeline 条目
2. THE Session_Timeline 条目 SHALL 包含：entry_id（UUID v4）、session_id、from_state（变更前 SessionPhase）、to_state（变更后 SessionPhase）、reason（变更原因）、link_id（当时绑定的 Link）、persona_id（当时使用的 Persona）、survival_mode（当时的 Survival Mode）、timestamp
3. THE Timeline_Store SHALL 支持按 session_id 查询该 Session 的全部时间线条目，按 timestamp 升序排列
4. THE Timeline_Store SHALL 支持按 session_id 和时间范围过滤查询

### 需求 3：Link 健康时间线

**用户故事：** 作为运维人员，我希望查看某条 Link 的健康指标变化历史，以便分析链路质量趋势。

#### 验收标准

1. WHEN LinkStateManager.UpdateHealth 成功执行，THE Timeline_Collector SHALL 生成一条 Link_Health_Timeline 条目
2. WHEN LinkStateManager.TransitionPhase 成功执行，THE Timeline_Collector SHALL 生成一条 Link_Health_Timeline 条目
3. THE Link_Health_Timeline 条目 SHALL 包含：entry_id（UUID v4）、link_id、health_score、rtt_ms、loss_rate、jitter_ms、phase（当时的 LinkPhase）、event_type（"health_update" 或 "phase_transition"）、timestamp
4. THE Timeline_Store SHALL 支持按 link_id 查询该 Link 的全部健康时间线条目，按 timestamp 升序排列
5. THE Timeline_Store SHALL 支持按 link_id 和时间范围过滤查询

### 需求 4：Persona 版本时间线

**用户故事：** 作为运维人员，我希望查看 Persona 版本切换历史，以便追踪画像变更轨迹。

#### 验收标准

1. WHEN PersonaEngine.SwitchPersona 成功执行，THE Timeline_Collector SHALL 生成一条 Persona_Version_Timeline 条目
2. WHEN PersonaEngine.Rollback 成功执行，THE Timeline_Collector SHALL 生成一条 Persona_Version_Timeline 条目
3. THE Persona_Version_Timeline 条目 SHALL 包含：entry_id（UUID v4）、session_id、persona_id、from_version（切换前版本号）、to_version（切换后版本号）、event_type（"switch" 或 "rollback"）、timestamp
4. THE Timeline_Store SHALL 支持按 session_id 或 persona_id 查询 Persona 版本时间线条目，按 timestamp 升序排列

### 需求 5：Survival Mode 时间线

**用户故事：** 作为运维人员，我希望查看 Survival Mode 的迁移历史，以便分析系统姿态变化趋势。

#### 验收标准

1. WHEN SurvivalOrchestrator.RequestTransition 成功执行模式迁移，THE Timeline_Collector SHALL 生成一条 Survival_Mode_Timeline 条目
2. THE Survival_Mode_Timeline 条目 SHALL 包含：entry_id（UUID v4）、from_mode、to_mode、triggers（触发因素列表 JSON）、tx_id（关联事务 ID）、timestamp
3. THE Timeline_Store SHALL 支持查询全部 Survival Mode 时间线条目，按 timestamp 升序排列
4. THE Timeline_Store SHALL 支持按时间范围过滤查询

### 需求 6：Transaction 时间线

**用户故事：** 作为运维人员，我希望查看某次事务的阶段推进历史，以便定位事务卡在哪个阶段。

#### 验收标准

1. WHEN CommitTransaction 的 tx_phase 发生变更，THE Timeline_Collector SHALL 生成一条 Transaction_Timeline 条目
2. THE Transaction_Timeline 条目 SHALL 包含：entry_id（UUID v4）、tx_id、from_phase（变更前 TxPhase）、to_phase（变更后 TxPhase）、phase_data（该阶段的状态 JSON，如 prepare_state / validate_state 等）、timestamp
3. THE Timeline_Store SHALL 支持按 tx_id 查询该事务的全部阶段推进历史，按 timestamp 升序排列
4. WHEN 查询某个 tx_id 的 Transaction_Timeline，THE Timeline_Store SHALL 返回从 Preparing 到终态的完整阶段序列


### 需求 7：最小诊断视图 — Session 诊断

**用户故事：** 作为运维人员，我希望一次查询就能看到某个 Session 当前挂在哪个 Link、使用哪个 Persona、处于哪个 Survival Mode，以便快速定位问题。

#### 验收标准

1. WHEN 请求某个 session_id 的诊断视图，THE Diagnostic_Aggregator SHALL 返回一个 Session_Diagnostic 对象
2. THE Session_Diagnostic SHALL 包含：session_id、current_link_id、current_link_phase、current_persona_id、current_persona_version、current_survival_mode、session_state、last_switch_reason（最近一次切换原因）、last_rollback_reason（最近一次回滚原因）
3. THE Diagnostic_Aggregator SHALL 从 SessionStateManager、LinkStateManager、ControlStateManager 实时聚合数据，返回当前最新状态
4. IF 请求的 session_id 不存在，THEN THE Diagnostic_Aggregator SHALL 返回 session not found 错误

### 需求 8：最小诊断视图 — 系统诊断

**用户故事：** 作为运维人员，我希望一次查询就能看到系统当前的 Survival Mode 和最近一次切换原因，以便快速了解系统整体状态。

#### 验收标准

1. WHEN 请求系统诊断视图，THE Diagnostic_Aggregator SHALL 返回一个 System_Diagnostic 对象
2. THE System_Diagnostic SHALL 包含：current_survival_mode、last_mode_switch_reason（最近一次模式切换原因）、last_mode_switch_time（最近一次模式切换时间）、active_session_count（活跃会话数）、active_link_count（活跃链路数）、active_transaction（当前活跃事务信息，无则为空）
3. THE Diagnostic_Aggregator SHALL 从 SurvivalOrchestrator、SessionStateManager、LinkStateManager、ControlStateManager 实时聚合数据

### 需求 9：最小诊断视图 — 事务诊断

**用户故事：** 作为运维人员，我希望查看某次事务当前卡在哪个阶段，以便快速定位事务阻塞原因。

#### 验收标准

1. WHEN 请求某个 tx_id 的事务诊断视图，THE Diagnostic_Aggregator SHALL 返回一个 Transaction_Diagnostic 对象
2. THE Transaction_Diagnostic SHALL 包含：tx_id、tx_type、current_phase、phase_durations（每个已完成阶段的耗时）、stuck_duration（如果事务在非终态，当前阶段已持续时间）、target_session_id、target_survival_mode
3. IF 请求的 tx_id 不存在，THEN THE Diagnostic_Aggregator SHALL 返回 transaction not found 错误
4. IF 事务处于终态（Committed / RolledBack / Failed），THE Transaction_Diagnostic 的 stuck_duration SHALL 为零值

### 需求 10：审计采集器与事件集成

**用户故事：** 作为开发者，我希望审计采集器通过 ControlEvent 事件机制自动触发，无需手动调用。

#### 验收标准

1. THE Audit_Collector SHALL 实现 Spec 5-3 的 EventHandler 接口
2. WHEN EventDispatcher 分发 EventRollbackDone 事件，THE Audit_Collector SHALL 查询关联的 CommitTransaction 并生成 rollback_triggered=true 的 Audit_Record
3. WHEN EventDispatcher 分发 EventBudgetReject 事件，THE Audit_Collector SHALL 查询关联的 CommitTransaction 并生成包含 budget_verdict 和 deny_reason 的 Audit_Record
4. THE Audit_Collector SHALL 在 EventRegistry 中注册为 EventRollbackDone 和 EventBudgetReject 的处理器

### 需求 11：观测数据持久化

**用户故事：** 作为运维人员，我希望审计记录和时间线数据持久化到数据库，以便长期保存和查询。

#### 验收标准

1. THE Audit_Store SHALL 将 Audit_Record 持久化到 PostgreSQL 的 audit_records 表
2. THE Timeline_Store SHALL 将五类时间线条目持久化到 PostgreSQL 的对应表（session_timeline、link_health_timeline、persona_version_timeline、survival_mode_timeline、transaction_timeline）
3. THE audit_records 表 SHALL 包含索引：tx_id、tx_type、initiated_at、rollback_triggered
4. THE 各时间线表 SHALL 包含索引：主查询字段（如 session_id、link_id、tx_id）和 timestamp

### 需求 12：观测数据查询 API

**用户故事：** 作为运维人员，我希望通过 HTTP API 查询审计记录、时间线和诊断视图。

#### 验收标准

1. THE mirage-os SHALL 提供 GET /api/v2/audit/records 端点，支持按 tx_type、时间范围、rollback_triggered 过滤
2. THE mirage-os SHALL 提供 GET /api/v2/audit/records/{tx_id} 端点，返回指定事务的审计记录
3. THE mirage-os SHALL 提供 GET /api/v2/timelines/sessions/{session_id} 端点，返回 Session 时间线
4. THE mirage-os SHALL 提供 GET /api/v2/timelines/links/{link_id}/health 端点，返回 Link 健康时间线
5. THE mirage-os SHALL 提供 GET /api/v2/timelines/personas/{session_id} 端点，返回 Persona 版本时间线
6. THE mirage-os SHALL 提供 GET /api/v2/timelines/survival-modes 端点，返回 Survival Mode 时间线
7. THE mirage-os SHALL 提供 GET /api/v2/timelines/transactions/{tx_id} 端点，返回 Transaction 时间线
8. THE mirage-os SHALL 提供 GET /api/v2/diagnostics/sessions/{session_id} 端点，返回 Session 诊断视图
9. THE mirage-os SHALL 提供 GET /api/v2/diagnostics/system 端点，返回系统诊断视图
10. THE mirage-os SHALL 提供 GET /api/v2/diagnostics/transactions/{tx_id} 端点，返回事务诊断视图
11. 所有 API 响应 SHALL 为 JSON 格式，时间戳使用 RFC 3339
12. IF 请求的资源不存在，THE API SHALL 返回 HTTP 404

### 需求 13：数据保留与清理

**用户故事：** 作为运维人员，我希望观测数据有合理的保留策略，避免无限增长。

#### 验收标准

1. THE Audit_Store SHALL 支持按保留天数清理过期的 Audit_Record，默认保留 90 天
2. THE Timeline_Store SHALL 支持按保留天数清理过期的时间线条目，默认保留 30 天
3. WHEN 执行清理操作，THE Audit_Store 和 Timeline_Store SHALL 仅删除超过保留期限的记录，保留期限内的记录保持不变

### 需求 14：JSON 序列化

**用户故事：** 作为开发者，我希望所有观测数据结构支持 JSON round-trip，以便在 API 和存储之间无损传输。

#### 验收标准

1. THE Audit_Record SHALL 支持 JSON 序列化后再反序列化产生等价对象
2. THE 五类 Timeline_Entry SHALL 各自支持 JSON 序列化后再反序列化产生等价对象
3. THE Session_Diagnostic、System_Diagnostic、Transaction_Diagnostic SHALL 各自支持 JSON 序列化后再反序列化产生等价对象
4. 所有 JSON 输出 SHALL 使用 snake_case 字段命名
5. 所有时间戳字段 SHALL 格式化为 RFC 3339
