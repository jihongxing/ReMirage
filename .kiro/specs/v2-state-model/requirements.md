# 需求文档：V2 三层状态模型

## 简介

Mirage V2 编排内核的基础状态建模层。将系统状态严格拆分为 Link State（链路状态）、Session State（会话状态）、Control State（控制状态）三层，实现状态定义、生命周期管理、持久化存储和查询 API。三层状态模型是后续 Persona Engine、State Commit Engine、Budget Engine、Survival Orchestrator 的基础依赖。

## 术语表

- **Orchestrator**：编排器，mirage-gateway 中新建的 `pkg/orchestrator/` 模块，负责 V2 状态管理与编排逻辑
- **Link_State**：链路状态对象，描述底层承载路径的健康与可用性，不包含用户业务语义
- **Session_State**：会话状态对象，描述业务会话的生命周期与当前承载映射
- **Control_State**：控制状态对象，描述系统当前正在执行的切换或提交操作，支持崩溃恢复
- **Link_Phase**：链路阶段枚举，取值为 Probing / Active / Degrading / Standby / Unavailable
- **Session_Phase**：会话阶段枚举，取值为 Bootstrapping / Active / Protected / Migrating / Degraded / Suspended / Closed
- **Control_Health**：控制面健康枚举，取值为 Healthy / Recovering / Faulted
- **Epoch**：全局递增的逻辑时钟，标识控制状态的版本，用于崩溃恢复和回滚判定
- **Rollback_Marker**：回滚标记，指向上一个已确认稳定的 Epoch，系统崩溃后据此恢复
- **Service_Class**：服务等级，对应用户付费等级（Standard / Platinum / Diamond）
- **Survival_Mode**：生存姿态枚举（Normal / LowNoise / Hardened / Degraded / Escape / LastResort），由后续 Spec 5-2 实现
- **State_Store**：状态持久化层，基于 PostgreSQL + GORM，位于 mirage-os 数据库
- **State_Query_API**：状态查询接口，Go HTTP API，提供三层状态的读取能力

## 需求

### 需求 1：Link State 定义与生命周期

**用户故事：** 作为编排器开发者，我需要一个独立的链路状态模型，以便将链路健康与用户会话解耦，实现链路失效不直接导致会话消失。

#### 验收标准

1. THE Orchestrator SHALL 定义 Link_State 结构体，包含以下字段：link_id（string，唯一标识）、transport_type（string，传输类型）、gateway_id（string，所属网关）、health_score（float64，0-100 健康分）、rtt_ms（int64，往返延迟毫秒）、loss_rate（float64，0-1 丢包率）、jitter_ms（int64，抖动毫秒）、phase（Link_Phase 枚举）、available（bool）、degraded（bool）、last_probe_at（时间戳）、last_switch_reason（string）、created_at（时间戳）、updated_at（时间戳）
2. THE Orchestrator SHALL 定义 Link_Phase 枚举，包含五个状态：Probing、Active、Degrading、Standby、Unavailable
3. WHEN Link_State 的 phase 从一个值转换到另一个值时，THE Orchestrator SHALL 校验转换合法性，仅允许以下转换路径：Probing→Active、Probing→Unavailable、Active→Degrading、Active→Standby、Degrading→Active、Degrading→Standby、Degrading→Unavailable、Standby→Probing、Standby→Unavailable、Unavailable→Probing
4. IF Link_State 的 phase 转换请求不在合法路径中，THEN THE Orchestrator SHALL 返回包含当前状态和目标状态的错误信息，拒绝该转换
5. WHEN Link_State 的 phase 变为 Unavailable 时，THE Orchestrator SHALL 将 available 设为 false，将 health_score 设为 0
6. WHEN Link_State 的 phase 变为 Active 时，THE Orchestrator SHALL 将 available 设为 true，将 degraded 设为 false
7. WHEN Link_State 的 phase 变为 Degrading 时，THE Orchestrator SHALL 将 degraded 设为 true

### 需求 2：Session State 定义与生命周期

**用户故事：** 作为编排器开发者，我需要一个独立的会话状态模型，以便将业务会话作为编排器的主要服务对象，使会话能承受底层链路漂移。

#### 验收标准

1. THE Orchestrator SHALL 定义 Session_State 结构体，包含以下字段：session_id（string，唯一标识）、user_id（string，所属用户）、client_id（string，客户端标识）、gateway_id（string，所属网关）、service_class（Service_Class 枚举）、priority（int，0-100 优先级）、current_persona_id（string，当前画像 ID）、current_link_id（string，当前链路 ID）、current_survival_mode（Survival_Mode 枚举）、billing_mode（string）、state（Session_Phase 枚举）、migration_pending（bool）、created_at（时间戳）、updated_at（时间戳）
2. THE Orchestrator SHALL 定义 Session_Phase 枚举，包含七个状态：Bootstrapping、Active、Protected、Migrating、Degraded、Suspended、Closed
3. WHEN Session_State 的 state 从一个值转换到另一个值时，THE Orchestrator SHALL 校验转换合法性，仅允许以下转换路径：Bootstrapping→Active、Bootstrapping→Closed、Active→Protected、Active→Migrating、Active→Degraded、Active→Suspended、Active→Closed、Protected→Active、Protected→Migrating、Protected→Degraded、Migrating→Active、Migrating→Degraded、Migrating→Closed、Degraded→Active、Degraded→Suspended、Degraded→Closed、Suspended→Active、Suspended→Closed
4. IF Session_State 的 state 转换请求不在合法路径中，THEN THE Orchestrator SHALL 返回包含当前状态和目标状态的错误信息，拒绝该转换
5. WHEN Session_State 的 state 变为 Migrating 时，THE Orchestrator SHALL 将 migration_pending 设为 true
6. WHEN Session_State 的 state 从 Migrating 转换到其他状态时，THE Orchestrator SHALL 将 migration_pending 设为 false
7. WHEN Session_State 的 current_link_id 发生变更时，THE Orchestrator SHALL 保持 session_id、current_persona_id、service_class、priority 不变，仅更新承载映射


### 需求 3：Control State 定义与生命周期

**用户故事：** 作为编排器开发者，我需要一个独立的控制状态模型，以便系统能解释"当前为什么是这个状态"，并在崩溃后恢复到上一个稳定点。

#### 验收标准

1. THE Orchestrator SHALL 定义 Control_State 结构体，包含以下字段：gateway_id（string，所属网关）、epoch（uint64，全局递增逻辑时钟）、persona_version（uint64，当前画像版本号）、route_generation（uint64，路由代次）、active_tx_id（string，当前活跃事务 ID，空字符串表示无活跃事务）、rollback_marker（uint64，上一个稳定 Epoch）、last_successful_epoch（uint64，最近一次成功提交的 Epoch）、last_switch_reason（string，最近一次切换原因）、control_health（Control_Health 枚举）、updated_at（时间戳）
2. THE Orchestrator SHALL 定义 Control_Health 枚举，包含三个状态：Healthy、Recovering、Faulted
3. WHEN Epoch 递增时，THE Orchestrator SHALL 确保新 Epoch 严格大于当前 Epoch，禁止回退或重复
4. WHEN active_tx_id 从空字符串变为非空字符串时，THE Orchestrator SHALL 将 control_health 保持为当前值，记录事务开始
5. WHEN 一次事务成功提交时，THE Orchestrator SHALL 将 last_successful_epoch 更新为当前 Epoch，将 rollback_marker 更新为当前 Epoch，将 active_tx_id 清空
6. IF 系统启动时检测到 active_tx_id 非空（存在未完成事务），THEN THE Orchestrator SHALL 将 control_health 设为 Recovering，将 Epoch 回退到 rollback_marker 指向的值，将 active_tx_id 清空
7. IF 系统启动时检测到 epoch 大于 last_successful_epoch 且 active_tx_id 非空，THEN THE Orchestrator SHALL 丢弃未提交的中间状态，恢复到 last_successful_epoch 对应的稳定状态

### 需求 4：三层状态关系约束

**用户故事：** 作为编排器开发者，我需要三层状态之间有明确的关系约束，以便保证状态一致性，防止出现孤立或矛盾的状态组合。

#### 验收标准

1. THE Orchestrator SHALL 确保每个 Session_State 的 current_link_id 引用一个已存在的 Link_State 的 link_id
2. IF Session_State 引用的 Link_State 的 phase 变为 Unavailable，THEN THE Orchestrator SHALL 将该 Session_State 的 state 转换为 Degraded（如果当前 state 允许该转换），而非直接转换为 Closed
3. THE Orchestrator SHALL 确保同一个 gateway_id 下只存在一个 Control_State 实例
4. WHEN 创建新的 Session_State 时，THE Orchestrator SHALL 校验 current_link_id 引用的 Link_State 的 available 字段为 true
5. IF 创建 Session_State 时引用的 Link_State 的 available 为 false，THEN THE Orchestrator SHALL 拒绝创建并返回链路不可用的错误信息

### 需求 5：状态持久化（DB Schema）

**用户故事：** 作为运维人员，我需要三层状态持久化到 PostgreSQL 数据库，以便系统重启后能恢复状态，支持状态审计和历史查询。

#### 验收标准

1. THE State_Store SHALL 在 mirage-os 数据库中创建 `link_states` 表，包含 Link_State 的所有字段，以 link_id 为主键，gateway_id 建立索引
2. THE State_Store SHALL 在 mirage-os 数据库中创建 `session_states` 表，包含 Session_State 的所有字段，以 session_id 为主键，user_id 和 gateway_id 和 current_link_id 建立索引
3. THE State_Store SHALL 在 mirage-os 数据库中创建 `control_states` 表，包含 Control_State 的所有字段，以 gateway_id 为主键
4. THE State_Store SHALL 使用 GORM AutoMigrate 机制，将三张新表纳入现有的 `AutoMigrate` 函数
5. WHEN Link_State 或 Session_State 的状态发生变更时，THE State_Store SHALL 在同一数据库事务中更新状态字段和 updated_at 时间戳
6. THE State_Store SHALL 为 `session_states` 表的 state 字段添加 CHECK 约束，仅允许 Session_Phase 枚举中定义的值
7. THE State_Store SHALL 为 `link_states` 表的 phase 字段添加 CHECK 约束，仅允许 Link_Phase 枚举中定义的值
8. THE State_Store SHALL 为 `link_states` 表的 health_score 字段添加 CHECK 约束，限制范围为 0 到 100

### 需求 6：状态查询 API

**用户故事：** 作为编排器和运维工具的调用方，我需要通过 HTTP API 查询三层状态，以便实时了解系统运行状况和进行故障诊断。

#### 验收标准

1. THE State_Query_API SHALL 提供 `GET /api/v2/links` 端点，返回指定 gateway_id 下所有 Link_State 列表
2. THE State_Query_API SHALL 提供 `GET /api/v2/links/{link_id}` 端点，返回指定 link_id 的 Link_State 详情
3. THE State_Query_API SHALL 提供 `GET /api/v2/sessions` 端点，支持按 gateway_id、user_id、state 参数过滤，返回 Session_State 列表
4. THE State_Query_API SHALL 提供 `GET /api/v2/sessions/{session_id}` 端点，返回指定 session_id 的 Session_State 详情
5. THE State_Query_API SHALL 提供 `GET /api/v2/control/{gateway_id}` 端点，返回指定 gateway_id 的 Control_State 详情
6. IF 查询的资源不存在，THEN THE State_Query_API SHALL 返回 HTTP 404 状态码和包含资源类型与标识的错误消息
7. THE State_Query_API SHALL 以 JSON 格式返回所有响应，时间戳字段使用 RFC 3339 格式
8. THE State_Query_API SHALL 提供 `GET /api/v2/sessions/{session_id}/topology` 端点，返回该会话关联的 Link_State 和 Control_State 的聚合视图，用于快速诊断

### 需求 7：状态变更序列化与并发安全

**用户故事：** 作为编排器开发者，我需要状态变更操作是并发安全的，以便在多个 goroutine 同时操作状态时不会出现数据竞争或状态不一致。

#### 验收标准

1. THE Orchestrator SHALL 对同一个 Link_State 的 phase 变更操作进行序列化，同一时刻只允许一个 goroutine 修改同一个 link_id 的状态
2. THE Orchestrator SHALL 对同一个 Session_State 的 state 变更操作进行序列化，同一时刻只允许一个 goroutine 修改同一个 session_id 的状态
3. THE Orchestrator SHALL 对同一个 gateway_id 的 Control_State 的 epoch 递增操作进行序列化
4. WHEN 并发状态变更发生冲突时，THE Orchestrator SHALL 使用数据库乐观锁（基于 updated_at 字段）检测冲突，冲突时返回重试错误
5. IF 状态变更的数据库事务失败，THEN THE Orchestrator SHALL 保持内存状态与数据库状态一致，不允许出现内存已更新但数据库未更新的情况
