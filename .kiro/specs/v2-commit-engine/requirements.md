# 需求文档：V2 State Commit Engine

## 简介

Mirage V2 编排内核的事务化状态提交引擎。负责所有关键状态变化（persona switch、link migration、gateway reassignment、survival mode switch）的事务化管理，通过标准六阶段提交流程（Prepare → Validate Constraint → Shadow Write → Flip → Acknowledge → Commit/Rollback）保证状态变更的原子性和可恢复性。State Commit Engine 是 Budget Engine（Spec 5-1）和 Survival Orchestrator（Spec 5-2）的前置依赖。

## 术语表

- **Commit_Engine**：提交引擎，mirage-gateway 中 `pkg/orchestrator/` 模块的子组件，负责所有受管变更的事务化提交流程
- **Commit_Transaction**：提交事务对象，记录一次完整状态变更的全部阶段状态和上下文
- **TX_ID**：事务唯一标识符，格式为 UUID v4 字符串
- **TX_Type**：事务类型枚举，取值为 PersonaSwitch / LinkMigration / GatewayReassignment / SurvivalModeSwitch
- **TX_Phase**：事务阶段枚举，取值为 Preparing / Validating / ShadowWriting / Flipping / Acknowledging / Committed / RolledBack / Failed
- **TX_Scope**：事务作用域枚举，取值为 Session（会话级）/ Link（链路级）/ Global（全局级）
- **Prepare_State**：准备阶段状态快照，包含事务开始时收集的当前 Session 状态、Link 状态、Persona 版本、Survival Mode、预算余量
- **Validate_State**：校验阶段结果，记录预算校验、服务等级校验、冷却时间校验、事务冲突校验的通过或拒绝结果
- **Shadow_State**：影子写入阶段的目标状态，包含新 persona manifest、新 route target、新 survival mode
- **Flip_State**：切换阶段结果，记录单点切换是否成功执行
- **Ack_State**：确认阶段结果，记录本地控制面确认、数据面版本确认、关键链路状态确认的结果
- **Commit_State**：最终提交状态，记录事务最终结果（Committed 或 RolledBack）及原因
- **Rollback_Marker**：回滚标记，指向上一个已确认稳定的 Epoch，事务失败或系统崩溃后据此恢复
- **Stable_Epoch**：稳定纪元，最近一次成功提交事务后的 Epoch 值，系统恢复的目标点
- **Cooldown_Period**：冷却时间，同类型事务两次执行之间的最小间隔，防止高频抖动
- **Session_State**：会话状态，由 Spec 4-1 定义
- **Link_State**：链路状态，由 Spec 4-1 定义
- **Control_State**：控制状态，由 Spec 4-1 定义，其 active_tx_id、epoch、rollback_marker 字段与 Commit_Engine 直接交互
- **Persona_Engine**：画像引擎，由 Spec 4-2 定义，Commit_Engine 在 PersonaSwitch 类型事务中调用其 SwitchPersona / Rollback 接口
- **Persona_Manifest**：画像清单，由 Spec 4-2 定义
- **Survival_Mode**：生存姿态枚举，由 Spec 4-1 定义
- **Service_Class**：服务等级，由 Spec 4-1 定义
- **Lock_Manager**：锁管理器，由 Spec 4-1 定义的并发控制组件，Commit_Engine 复用其细粒度锁机制

## 需求

### 需求 1：CommitTransaction 对象定义

**用户故事：** 作为编排器开发者，我需要一个完整的事务对象来记录每次状态变更的全部阶段信息，以便追踪事务进度、支持崩溃恢复和事后审计。

#### 验收标准

1. THE Commit_Engine SHALL 定义 Commit_Transaction 结构体，包含以下字段：tx_id（string，UUID v4 唯一标识）、tx_type（TX_Type 枚举）、tx_phase（TX_Phase 枚举，当前所处阶段）、tx_scope（TX_Scope 枚举，事务作用域）、target_session_id（string，目标会话 ID）、target_link_id（string，目标链路 ID）、target_persona_id（string，目标画像 ID）、target_survival_mode（string，目标生存姿态）、prepare_state（JSON 序列化的准备阶段快照）、validate_state（JSON 序列化的校验结果）、shadow_state（JSON 序列化的影子写入目标）、flip_state（JSON 序列化的切换结果）、ack_state（JSON 序列化的确认结果）、commit_state（JSON 序列化的最终提交状态）、rollback_marker（uint64，事务开始时的稳定 Epoch）、created_at（时间戳）、finished_at（时间戳，事务完成时间）
2. THE Commit_Engine SHALL 定义 TX_Type 枚举，包含四种受管变更类型：PersonaSwitch、LinkMigration、GatewayReassignment、SurvivalModeSwitch
3. THE Commit_Engine SHALL 定义 TX_Phase 枚举，包含八个阶段：Preparing、Validating、ShadowWriting、Flipping、Acknowledging、Committed、RolledBack、Failed
4. THE Commit_Engine SHALL 定义 TX_Scope 枚举，包含三个作用域：Session（会话级事务，如 PersonaSwitch）、Link（链路级事务，如 LinkMigration）、Global（全局事务，如 SurvivalModeSwitch）
5. WHEN 创建 Commit_Transaction 时，THE Commit_Engine SHALL 将 tx_phase 初始化为 Preparing，将 rollback_marker 设置为当前 Control_State 的 last_successful_epoch
6. THE Commit_Engine SHALL 确保 tx_id 在系统范围内唯一，禁止重复

### 需求 2：标准提交流程 — Prepare 阶段

**用户故事：** 作为编排器开发者，我需要事务在执行前收集完整的上下文快照，以便后续阶段能基于一致的起始状态进行校验和决策。

#### 验收标准

1. WHEN 事务进入 Prepare 阶段时，THE Commit_Engine SHALL 收集以下上下文并写入 prepare_state：当前 Session_State 快照（如果 target_session_id 非空）、当前 Link_State 快照（如果 target_link_id 非空）、当前 Persona_Manifest 版本号、当前 Survival_Mode、当前 Control_State 的 epoch
2. WHEN Prepare 阶段完成后，THE Commit_Engine SHALL 将 tx_phase 从 Preparing 转换为 Validating
3. IF Prepare 阶段收集上下文时发现目标 Session 或 Link 不存在，THEN THE Commit_Engine SHALL 将 tx_phase 设为 Failed，记录失败原因，终止事务
4. WHEN 事务进入 Prepare 阶段时，THE Commit_Engine SHALL 将 Control_State 的 active_tx_id 设置为当前 tx_id，标记事务开始

### 需求 3：标准提交流程 — Validate Constraint 阶段

**用户故事：** 作为编排器开发者，我需要事务在执行变更前通过约束校验，以便确保变更满足预算、服务等级、冷却时间和冲突规则。

#### 验收标准

1. WHEN 事务进入 Validate 阶段时，THE Commit_Engine SHALL 执行以下校验并将结果写入 validate_state：冷却时间校验（同类型事务距上次执行是否超过 Cooldown_Period）、事务冲突校验（是否违反并发事务规则）
2. WHEN 事务进入 Validate 阶段时，THE Commit_Engine SHALL 预留预算校验接口（budget_check），当前版本该接口默认返回通过，待 Spec 5-1 Budget Engine 实现后接入
3. WHEN 事务进入 Validate 阶段时，THE Commit_Engine SHALL 预留服务等级校验接口（service_class_check），当前版本该接口默认返回通过，待 Spec 5-1 实现后接入
4. WHEN Validate 阶段全部校验通过后，THE Commit_Engine SHALL 将 tx_phase 从 Validating 转换为 ShadowWriting
5. IF Validate 阶段任一校验失败，THEN THE Commit_Engine SHALL 将 tx_phase 设为 Failed，记录失败的校验项和原因，清空 Control_State 的 active_tx_id，终止事务

### 需求 4：标准提交流程 — Shadow Write 阶段

**用户故事：** 作为编排器开发者，我需要事务将目标状态写入影子区而非直接覆盖当前状态，以便在切换失败时能安全回滚。

#### 验收标准

1. WHEN 事务进入 Shadow Write 阶段时，THE Commit_Engine SHALL 根据 tx_type 将目标状态写入对应的影子区域：PersonaSwitch 类型调用 Persona_Engine 的 Shadow 写入接口、LinkMigration 类型将新 route target 写入 shadow 区、SurvivalModeSwitch 类型将新 survival mode 写入 shadow 区
2. WHEN Shadow Write 阶段完成后，THE Commit_Engine SHALL 将 rollback_marker 写入 Control_State，确保崩溃后能识别未完成的影子写入
3. WHEN Shadow Write 阶段完成后，THE Commit_Engine SHALL 将 tx_phase 从 ShadowWriting 转换为 Flipping
4. IF Shadow Write 阶段写入失败，THEN THE Commit_Engine SHALL 将 tx_phase 设为 RolledBack，清理已写入的影子数据，清空 Control_State 的 active_tx_id

### 需求 5：标准提交流程 — Flip 阶段

**用户故事：** 作为编排器开发者，我需要事务通过单点切换使新状态正式生效，以便将影子区的目标状态原子地提升为当前活跃状态。

#### 验收标准

1. WHEN 事务进入 Flip 阶段时，THE Commit_Engine SHALL 执行单点切换操作：PersonaSwitch 类型调用 Persona_Engine 的 Atomic_Flip、LinkMigration 类型更新 Session_State 的 current_link_id、GatewayReassignment 类型更新 Session_State 的 gateway_id、SurvivalModeSwitch 类型更新 Session_State 的 current_survival_mode
2. WHEN Flip 执行成功后，THE Commit_Engine SHALL 递增 Control_State 的 epoch，将 flip_state 记录为成功，将 tx_phase 从 Flipping 转换为 Acknowledging
3. IF Flip 执行失败，THEN THE Commit_Engine SHALL 触发回滚流程：恢复影子写入前的状态、将 tx_phase 设为 RolledBack、记录失败原因

### 需求 6：标准提交流程 — Acknowledge 阶段

**用户故事：** 作为编排器开发者，我需要事务在切换后等待关键参与方确认，以便验证新状态已在所有关键组件上生效。

#### 验收标准

1. WHEN 事务进入 Acknowledge 阶段时，THE Commit_Engine SHALL 执行以下确认检查并将结果写入 ack_state：本地控制面确认（Control_State 的 epoch 与预期一致）、数据面版本确认（PersonaSwitch 类型检查 eBPF Map 的 active_slot 指向正确 Slot）
2. WHEN 全部确认检查通过后，THE Commit_Engine SHALL 将 tx_phase 从 Acknowledging 转换为 Committed
3. IF 确认检查在超时时间内未全部通过，THEN THE Commit_Engine SHALL 触发回滚流程，将 tx_phase 设为 RolledBack，记录未通过的确认项

### 需求 7：标准提交流程 — Commit / Rollback 阶段

**用户故事：** 作为编排器开发者，我需要事务在最终阶段明确提交或回滚，以便系统状态始终处于已知的稳定点。

#### 验收标准

1. WHEN tx_phase 变为 Committed 时，THE Commit_Engine SHALL 执行以下操作：将 Control_State 的 last_successful_epoch 更新为当前 epoch、将 Control_State 的 rollback_marker 更新为当前 epoch、清空 Control_State 的 active_tx_id、记录 finished_at 时间戳
2. WHEN tx_phase 变为 RolledBack 时，THE Commit_Engine SHALL 执行以下操作：根据 tx_type 调用对应的回滚逻辑（PersonaSwitch 调用 Persona_Engine 的 Rollback）、将 Control_State 的 epoch 恢复到 rollback_marker 指向的值、清空 Control_State 的 active_tx_id、记录 finished_at 时间戳和回滚原因
3. THE Commit_Engine SHALL 确保 Committed 和 RolledBack 是终态，tx_phase 进入这两个状态后禁止再次转换
4. WHEN 事务完成（Committed 或 RolledBack）后，THE Commit_Engine SHALL 将完整的 Commit_Transaction 记录持久化到数据库，用于审计和诊断

### 需求 8：事务阶段状态机

**用户故事：** 作为编排器开发者，我需要事务阶段转换有严格的状态机约束，以便防止事务跳过关键阶段或进入非法状态。

#### 验收标准

1. THE Commit_Engine SHALL 定义 TX_Phase 状态机，仅允许以下转换路径：Preparing→Validating、Preparing→Failed、Validating→ShadowWriting、Validating→Failed、ShadowWriting→Flipping、ShadowWriting→RolledBack、Flipping→Acknowledging、Flipping→RolledBack、Acknowledging→Committed、Acknowledging→RolledBack
2. IF TX_Phase 转换请求不在合法路径中，THEN THE Commit_Engine SHALL 返回包含当前阶段和目标阶段的错误信息，拒绝该转换
3. THE Commit_Engine SHALL 确保每次 TX_Phase 转换时记录转换时间戳，用于诊断事务在哪个阶段耗时过长

### 需求 9：事务冲突规则

**用户故事：** 作为编排器开发者，我需要系统在同一时刻限制并发事务数量，以便防止多个事务同时修改相关状态导致不一致。

#### 验收标准

1. THE Commit_Engine SHALL 同一时刻最多允许一个 Session 级事务（TX_Scope=Session）处于活跃状态（tx_phase 不是 Committed、RolledBack 或 Failed）
2. THE Commit_Engine SHALL 同一时刻最多允许一个 Link 级事务（TX_Scope=Link）处于活跃状态
3. THE Commit_Engine SHALL 同一时刻最多允许一个 Global 级事务（TX_Scope=Global）处于活跃状态
4. WHEN 新事务请求与已有活跃事务的 TX_Scope 相同时，THE Commit_Engine SHALL 比较两个事务的优先级：Global > Link > Session，高优先级事务可以抢占低优先级事务
5. IF 新事务的优先级不高于已有活跃事务，THEN THE Commit_Engine SHALL 拒绝新事务并返回事务冲突错误，包含已有活跃事务的 tx_id 和 tx_type
6. WHEN 高优先级事务抢占低优先级事务时，THE Commit_Engine SHALL 先将被抢占事务回滚到 RolledBack 状态，再启动新事务

### 需求 10：冷却时间规则

**用户故事：** 作为编排器开发者，我需要同类型事务之间有最小间隔限制，以便防止系统在短时间内反复执行同类变更导致高频抖动。

#### 验收标准

1. THE Commit_Engine SHALL 为每种 TX_Type 维护一个可配置的 Cooldown_Period（默认值：PersonaSwitch=30 秒、LinkMigration=10 秒、GatewayReassignment=60 秒、SurvivalModeSwitch=60 秒）
2. WHEN 创建新事务时，THE Commit_Engine SHALL 检查同一 TX_Type 的上一次成功提交（Committed）的 finished_at 距当前时间是否超过 Cooldown_Period
3. IF 距上一次同类型事务的 finished_at 未超过 Cooldown_Period，THEN THE Commit_Engine SHALL 拒绝新事务并返回冷却时间未到的错误，包含剩余冷却秒数

### 需求 11：崩溃恢复规则

**用户故事：** 作为编排器开发者，我需要系统重启后能自动检测并处理未完成的事务，以便系统始终回到上一个稳定点而非停在中间态。

#### 验收标准

1. WHEN 系统启动时，THE Commit_Engine SHALL 查询数据库中所有 tx_phase 不是 Committed、RolledBack 或 Failed 的 Commit_Transaction 记录
2. WHEN 发现未完成事务时，THE Commit_Engine SHALL 将该事务的 tx_phase 设为 RolledBack，记录回滚原因为"系统重启恢复"
3. WHEN 发现未完成事务时，THE Commit_Engine SHALL 检查 Control_State 的 rollback_marker，将 epoch 恢复到 rollback_marker 指向的值
4. WHEN 发现未完成事务时，THE Commit_Engine SHALL 根据事务的 tx_type 执行对应的回滚清理：PersonaSwitch 类型调用 Persona_Engine 的 Rollback 恢复到 Cooling Slot、LinkMigration 类型恢复 Session_State 的 current_link_id 为 prepare_state 中记录的值
5. WHEN 恢复完成后，THE Commit_Engine SHALL 将 Control_State 的 active_tx_id 清空，control_health 设为 Recovering
6. IF 恢复过程中发现 rollback_marker 指向的状态不存在或不可恢复，THEN THE Commit_Engine SHALL 将 Control_State 的 control_health 设为 Faulted，记录错误日志，等待人工介入

### 需求 12：事务持久化

**用户故事：** 作为运维人员，我需要所有事务记录持久化到数据库，以便进行事务审计、故障诊断和历史查询。

#### 验收标准

1. THE Commit_Engine SHALL 在 mirage-os 数据库中创建 `commit_transactions` 表，包含 Commit_Transaction 的所有字段，以 tx_id 为主键
2. THE Commit_Engine SHALL 为 `commit_transactions` 表的 tx_type 字段添加 CHECK 约束，仅允许 TX_Type 枚举中定义的值
3. THE Commit_Engine SHALL 为 `commit_transactions` 表的 tx_phase 字段添加 CHECK 约束，仅允许 TX_Phase 枚举中定义的值
4. THE Commit_Engine SHALL 为 `commit_transactions` 表的 target_session_id 和 created_at 字段建立索引，支持按会话和时间范围查询
5. WHEN tx_phase 发生转换时，THE Commit_Engine SHALL 在同一数据库事务中更新 tx_phase 和对应的阶段状态字段
6. THE Commit_Engine SHALL 使用 GORM AutoMigrate 机制，将 `commit_transactions` 表纳入现有的 AutoMigrate 函数

### 需求 13：事务查询 API

**用户故事：** 作为编排器和运维工具的调用方，我需要通过 HTTP API 查询事务状态，以便了解当前活跃事务、历史事务和事务卡在哪个阶段。

#### 验收标准

1. THE Commit_Engine SHALL 提供 `GET /api/v2/transactions/{tx_id}` 端点，返回指定 tx_id 的 Commit_Transaction 详情
2. THE Commit_Engine SHALL 提供 `GET /api/v2/transactions` 端点，支持按 tx_type、tx_phase、target_session_id、时间范围参数过滤，返回 Commit_Transaction 列表
3. THE Commit_Engine SHALL 提供 `GET /api/v2/transactions/active` 端点，返回当前所有活跃（tx_phase 不是 Committed、RolledBack 或 Failed）的事务列表
4. IF 查询的事务不存在，THEN THE Commit_Engine SHALL 返回 HTTP 404 状态码和包含 tx_id 的错误消息
5. THE Commit_Engine SHALL 以 JSON 格式返回所有响应，时间戳字段使用 RFC 3339 格式

### 需求 14：Commit_Transaction JSON 序列化 round-trip

**用户故事：** 作为编排器开发者，我需要 Commit_Transaction 的 JSON 序列化和反序列化是无损的，以便 API 响应、数据库存储和日志记录中的事务数据保持一致。

#### 验收标准

1. FOR ALL 合法的 Commit_Transaction 对象，JSON 序列化后再反序列化 SHALL 产生等价对象，所有字段值保持不变
2. THE Commit_Engine SHALL 确保 JSON 序列化结果中的 created_at 和 finished_at 字段符合 RFC 3339 格式
3. THE Commit_Engine SHALL 确保 JSON 序列化结果中的 prepare_state、validate_state、shadow_state、flip_state、ack_state、commit_state 字段为合法的 JSON 对象或空对象
