# 需求文档：V2 Survival Orchestrator

## 简介

本文档定义 Mirage V2 编排内核的 Survival Orchestrator（生存编排器），负责将局部事件（入口失效、链路健康下降、预算耗尽、策略触发）升级为全局生存姿态，并驱动 Transport Fabric 和 Persona Engine 联动响应。Survival Orchestrator 取代原有 G-Switch 的单点域名切换角色，成为系统级姿态控制器。

本模块位于 `mirage-gateway/pkg/orchestrator/survival/`，Transport Fabric 位于 `mirage-gateway/pkg/orchestrator/transport/`，G-Switch 适配层保留在 `mirage-gateway/pkg/gswitch/`。

## 术语表

- **Survival_Orchestrator**：生存编排器，负责管理 Survival Mode 状态机、评估触发因素、执行模式迁移
- **Survival_Mode**：生存姿态枚举（Normal / LowNoise / Hardened / Degraded / Escape / LastResort），定义于 Spec 4-1
- **Mode_Policy**：每种 Survival Mode 绑定的编排策略集合，包含 transport_policy / persona_policy / budget_policy / switch_aggressiveness / session_admission_policy
- **Trigger_Evaluator**：触发因素评估器，负责将原始信号（链路健康、入口消耗、预算状态、策略指令）转换为模式迁移建议
- **Link_Health_Trigger**：基于链路健康分数的触发因素
- **Entry_Burn_Trigger**：基于入口（域名）战死频率的触发因素
- **Budget_Trigger**：基于预算耗尽状态的触发因素
- **Policy_Trigger**：基于外部策略指令的触发因素
- **Transition_Constraint**：状态迁移约束，包含 cooldown（冷却时间）、hysteresis（滞回阈值）、minimum_dwell_time（最小驻留时间）
- **Transport_Fabric**：传输织网层，负责承载路径选择、路径切换、多通道调度、退化路径启用
- **Transport_Policy**：按 Survival Mode 分级的传输策略
- **Path_Scorer**：路径评分器，根据链路健康指标和当前 Transport Policy 计算路径得分
- **Switch_Aggressiveness**：切换激进度，控制路径切换的触发灵敏度（Conservative / Moderate / Aggressive）
- **Session_Admission_Policy**：会话准入策略，控制新会话是否允许建立（Open / RestrictNew / HighPriorityOnly / Closed）
- **Commit_Engine**：事务化状态提交引擎（Spec 4-3），Survival Mode 切换通过 TxTypeSurvivalModeSwitch 事务执行
- **Budget_Engine**：预算引擎（Spec 5-1），提供 BudgetChecker 和 ServiceClassChecker
- **G-Switch_Adapter**：G-Switch 适配层，将现有域名转生能力纳入 Survival Orchestrator 的 Entry Burn 处理流程

## 需求

### 需求 1：Survival Mode 状态机

**用户故事：** 作为系统运维人员，我希望系统具备六种明确的生存姿态，以便根据威胁等级和资源状况自动调整全局编排策略。

#### 验收标准

1. THE Survival_Orchestrator SHALL 维护一个包含六种模式的状态机：Normal、LowNoise、Hardened、Degraded、Escape、LastResort
2. THE Survival_Orchestrator SHALL 在系统启动时将 Survival Mode 初始化为 Normal
3. WHEN Survival Mode 发生变更时，THE Survival_Orchestrator SHALL 通过 Commit_Engine 的 TxTypeSurvivalModeSwitch 事务执行变更
4. THE Survival_Orchestrator SHALL 维护合法的状态迁移路径表，仅允许预定义的模式迁移
5. THE Survival_Orchestrator SHALL 支持以下迁移路径：Normal↔LowNoise、Normal→Hardened、Normal→Degraded、LowNoise→Hardened、LowNoise→Degraded、Hardened→Escape、Hardened→Degraded、Hardened→Normal、Degraded→Normal、Degraded→Escape、Escape→LastResort、Escape→Hardened、Escape→Normal、LastResort→Escape、LastResort→Normal
6. IF 收到非法的模式迁移请求，THEN THE Survival_Orchestrator SHALL 返回包含当前模式和目标模式的错误

### 需求 2：Mode Policy 绑定

**用户故事：** 作为系统运维人员，我希望每种生存姿态绑定一组明确的编排策略，以便系统在模式切换后自动调整所有子系统行为。

#### 验收标准

1. THE Survival_Orchestrator SHALL 为每种 Survival Mode 绑定一个 Mode_Policy，包含 transport_policy、persona_policy、budget_policy、switch_aggressiveness、session_admission_policy 五个维度
2. WHEN Survival Mode 切换成功后，THE Survival_Orchestrator SHALL 将新模式对应的 Mode_Policy 下发到 Transport_Fabric 和 Persona Engine
3. THE Survival_Orchestrator SHALL 为 Normal 模式绑定策略：switch_aggressiveness=Conservative、session_admission_policy=Open
4. THE Survival_Orchestrator SHALL 为 LowNoise 模式绑定策略：switch_aggressiveness=Conservative、session_admission_policy=Open
5. THE Survival_Orchestrator SHALL 为 Hardened 模式绑定策略：switch_aggressiveness=Moderate、session_admission_policy=Open
6. THE Survival_Orchestrator SHALL 为 Degraded 模式绑定策略：switch_aggressiveness=Conservative、session_admission_policy=RestrictNew
7. THE Survival_Orchestrator SHALL 为 Escape 模式绑定策略：switch_aggressiveness=Aggressive、session_admission_policy=HighPriorityOnly
8. THE Survival_Orchestrator SHALL 为 LastResort 模式绑定策略：switch_aggressiveness=Aggressive、session_admission_policy=Closed

### 需求 3：触发因素评估

**用户故事：** 作为系统运维人员，我希望系统能够根据四类触发因素自动评估是否需要切换生存姿态，以便及时响应威胁变化。

#### 验收标准

1. THE Trigger_Evaluator SHALL 支持四类触发因素：Link_Health_Trigger、Entry_Burn_Trigger、Budget_Trigger、Policy_Trigger
2. WHEN 所有活跃链路的平均健康分数低于 60 时，THE Link_Health_Trigger SHALL 建议从 Normal 升级到 Hardened
3. WHEN 所有活跃链路的平均健康分数低于 30 时，THE Link_Health_Trigger SHALL 建议升级到 Degraded
4. WHEN 所有活跃链路的平均健康分数低于 10 时，THE Link_Health_Trigger SHALL 建议升级到 Escape
5. WHEN 过去 1 小时内入口战死次数超过 entry_burn_threshold 时，THE Entry_Burn_Trigger SHALL 建议升级生存姿态
6. WHEN Budget_Engine 返回 deny_and_suspend 判定时，THE Budget_Trigger SHALL 建议降级到 Degraded
7. WHEN 收到外部 Policy_Trigger 指令时，THE Trigger_Evaluator SHALL 将指令转换为对应的模式迁移建议
8. THE Trigger_Evaluator SHALL 将多个触发因素的建议合并，选择最高严重度的建议作为最终迁移目标
9. THE Trigger_Evaluator SHALL 在评估结果中包含触发源标识和触发原因描述


### 需求 4：状态迁移约束

**用户故事：** 作为系统运维人员，我希望状态迁移受到冷却时间、滞回阈值和最小驻留时间的约束，以便防止高频抖动和反复横跳。

#### 验收标准

1. THE Transition_Constraint SHALL 为每种 Survival Mode 定义 minimum_dwell_time（最小驻留时间），在驻留时间未满前拒绝降级迁移
2. THE Transition_Constraint SHALL 为升级迁移定义 cooldown 时间，同类型升级在冷却期内被拒绝
3. THE Transition_Constraint SHALL 为降级迁移定义 hysteresis 阈值，触发因素必须持续改善超过阈值才允许降级
4. THE Transition_Constraint SHALL 使用以下默认最小驻留时间：Normal=0s、LowNoise=30s、Hardened=60s、Degraded=120s、Escape=30s、LastResort=60s
5. THE Transition_Constraint SHALL 使用默认升级冷却时间 60 秒
6. THE Transition_Constraint SHALL 使用默认降级滞回阈值：触发因素改善幅度超过升级阈值的 20%
7. IF 迁移请求违反任一约束，THEN THE Transition_Constraint SHALL 返回包含违反约束类型和剩余等待时间的错误
8. THE Transition_Constraint SHALL 允许 Policy_Trigger 类型的迁移绕过 cooldown 和 hysteresis 约束，但仍受 minimum_dwell_time 约束

### 需求 5：Transport Fabric 路径管理

**用户故事：** 作为系统运维人员，我希望 Transport Fabric 能够根据当前 Survival Mode 的传输策略管理承载路径，以便在不同生存姿态下选择最优路径。

#### 验收标准

1. THE Transport_Fabric SHALL 维护所有可用链路的路径列表，每条路径关联一个 LinkState（Spec 4-1）
2. THE Transport_Fabric SHALL 使用 Path_Scorer 根据链路健康指标（health_score、rtt_ms、loss_rate、jitter_ms）和当前 Transport_Policy 计算每条路径的综合得分
3. WHEN 需要选择路径时，THE Transport_Fabric SHALL 返回得分最高的可用路径
4. THE Transport_Fabric SHALL 在路径切换时通过 Commit_Engine 的 TxTypeLinkMigration 事务执行
5. THE Transport_Fabric SHALL 在路径切换后保持 Session 层语义不变
6. THE Transport_Fabric SHALL 在路径切换后保持 Persona 稳定，除非 Survival Orchestrator 要求联动切换
7. IF 所有主路径不可用，THEN THE Transport_Fabric SHALL 启用退化路径（degraded path）

### 需求 6：Transport Policy 分级

**用户故事：** 作为系统运维人员，我希望传输策略按 Survival Mode 分级，以便在不同生存姿态下采用不同的路径选择和切换策略。

#### 验收标准

1. WHILE 处于 Normal 模式时，THE Transport_Policy SHALL 优先选择高性能路径，限制无意义切换（switch_aggressiveness=Conservative）
2. WHILE 处于 LowNoise 模式时，THE Transport_Policy SHALL 维持当前路径，降低切换频率，避免产生额外流量特征
3. WHILE 处于 Hardened 模式时，THE Transport_Policy SHALL 允许更高容错成本，提高切换敏感度（switch_aggressiveness=Moderate）
4. WHILE 处于 Degraded 模式时，THE Transport_Policy SHALL 降低激进切换，保守维持当前有效路径
5. WHILE 处于 Escape 模式时，THE Transport_Policy SHALL 允许使用高成本入口，优先存活而不是性能（switch_aggressiveness=Aggressive）
6. WHILE 处于 LastResort 模式时，THE Transport_Policy SHALL 仅维持高优先级会话的路径，释放低优先级会话占用的路径资源
7. THE Transport_Policy SHALL 定义路径切换触发阈值：Conservative 模式下健康分数低于 40 才触发切换，Moderate 模式下低于 60 触发，Aggressive 模式下低于 80 触发

### 需求 7：G-Switch 适配与升级

**用户故事：** 作为系统运维人员，我希望现有 G-Switch 域名转生能力被纳入 Survival Orchestrator 的统一管理，以便域名切换不再是孤立动作而是生存姿态的一部分。

#### 验收标准

1. THE G-Switch_Adapter SHALL 将 GSwitchManager 的 TriggerEscape 能力封装为 Entry_Burn_Trigger 的信号源
2. WHEN GSwitchManager 报告域名战死时，THE G-Switch_Adapter SHALL 将战死事件转换为 Entry_Burn_Trigger 信号并提交给 Trigger_Evaluator
3. THE G-Switch_Adapter SHALL 在 Survival Mode 切换到 Escape 或 LastResort 时，自动调用 GSwitchManager 的 TriggerEscape 执行域名转生
4. THE G-Switch_Adapter SHALL 将 GSwitchManager 的域名池状态（active/standby/burned 数量）暴露给 Trigger_Evaluator 作为评估输入
5. IF 热备域名池为空且当前模式低于 Escape，THEN THE G-Switch_Adapter SHALL 向 Trigger_Evaluator 提交升级到 Escape 的建议

### 需求 8：多通道调度

**用户故事：** 作为系统运维人员，我希望 Transport Fabric 支持多通道并行调度，以便在高威胁场景下通过多路径提高存活率。

#### 验收标准

1. THE Transport_Fabric SHALL 支持为同一 Session 同时维护主路径和备用路径
2. WHILE 处于 Hardened 或更高模式时，THE Transport_Fabric SHALL 为高优先级 Session 预热备用路径
3. WHEN 主路径健康分数低于切换阈值时，THE Transport_Fabric SHALL 将流量切换到已预热的备用路径
4. THE Transport_Fabric SHALL 限制同一 Session 的并行路径数量上限为 3

### 需求 9：Session 准入控制

**用户故事：** 作为系统运维人员，我希望在高威胁模式下限制新会话建立，以便保护现有高优先级会话的资源。

#### 验收标准

1. WHILE session_admission_policy 为 Open 时，THE Survival_Orchestrator SHALL 允许所有新会话建立
2. WHILE session_admission_policy 为 RestrictNew 时，THE Survival_Orchestrator SHALL 仅允许 service_class 为 Platinum 或 Diamond 的新会话建立
3. WHILE session_admission_policy 为 HighPriorityOnly 时，THE Survival_Orchestrator SHALL 仅允许 service_class 为 Diamond 的新会话建立
4. WHILE session_admission_policy 为 Closed 时，THE Survival_Orchestrator SHALL 拒绝所有新会话建立
5. THE Survival_Orchestrator SHALL 在拒绝新会话时返回包含当前准入策略和会话 service_class 的错误

### 需求 10：模式迁移事务集成

**用户故事：** 作为系统运维人员，我希望所有 Survival Mode 切换都通过 Commit Engine 事务执行，以便保证切换的原子性和可回滚性。

#### 验收标准

1. WHEN 执行 Survival Mode 切换时，THE Survival_Orchestrator SHALL 调用 Commit_Engine 的 BeginTransaction 创建 TxTypeSurvivalModeSwitch 事务
2. THE Survival_Orchestrator SHALL 在事务的 Validate 阶段调用 Budget_Engine 的 BudgetChecker 和 ServiceClassChecker 进行预算和服务等级校验
3. THE Survival_Orchestrator SHALL 在事务的 Shadow Write 阶段将新模式对应的 Mode_Policy 写入影子区
4. THE Survival_Orchestrator SHALL 在事务的 Flip 阶段原子切换当前 Survival Mode 和关联的 Mode_Policy
5. THE Survival_Orchestrator SHALL 在事务的 Acknowledge 阶段确认 Transport_Fabric 和 Persona Engine 已应用新策略
6. IF 事务在任一阶段失败，THEN THE Survival_Orchestrator SHALL 回滚到切换前的 Survival Mode 和 Mode_Policy
7. THE Survival_Orchestrator SHALL 在所有 Session 的 current_survival_mode 字段中更新新模式

### 需求 11：恢复与诊断

**用户故事：** 作为系统运维人员，我希望系统重启后能恢复到上一个稳定的生存姿态，并能诊断最近的模式切换历史。

#### 验收标准

1. WHEN 系统重启时，THE Survival_Orchestrator SHALL 从 ControlState 的 last_successful_epoch 恢复上一个稳定的 Survival Mode
2. IF 恢复时发现未完成的 SurvivalModeSwitch 事务，THEN THE Survival_Orchestrator SHALL 通过 Commit_Engine 的 RecoverOnStartup 回滚该事务
3. THE Survival_Orchestrator SHALL 记录每次模式迁移的触发源、触发原因、源模式、目标模式和迁移时间戳
4. THE Survival_Orchestrator SHALL 提供查询接口返回最近 N 次模式迁移历史记录

### 需求 12：数据序列化

**用户故事：** 作为系统运维人员，我希望所有核心数据结构支持 JSON 序列化和反序列化，以便通过 API 查询和持久化。

#### 验收标准

1. THE Mode_Policy SHALL 支持 JSON 序列化和反序列化，序列化后再反序列化产生等价对象
2. THE Transport_Policy SHALL 支持 JSON 序列化和反序列化，序列化后再反序列化产生等价对象
3. THE Transition_Constraint 配置 SHALL 支持 JSON 序列化和反序列化，序列化后再反序列化产生等价对象
4. 所有 JSON 输出 SHALL 使用 snake_case 字段命名
