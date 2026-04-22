# 需求文档：V2 Budget Engine

## 简介

Budget Engine 是 Mirage V2 编排内核的横切引擎之一，负责将所有强化动作约束在有限预算范围内。其核心目标不是单纯记账，而是把 Mirage 从"无限强化"约束到"有限决策"——每一次编排动作（Persona 切换、Link 迁移、Gateway 重分配、Survival Mode 切换）都必须经过预算评估和服务等级校验后才能执行。

Budget Engine 采用双层模型：Internal Cost Model 计算内部资源消耗成本，External SLA / Service Policy 映射用户服务等级权限。两层结合产生五种判定结果（allow / allow_degraded / allow_with_charge / deny_and_hold / deny_and_suspend），替换 Commit Engine 中的 DefaultBudgetChecker 和 DefaultServiceClassChecker。

本模块全部为 Go 控制面代码，位于 `mirage-gateway/pkg/orchestrator/budget/`，数据库模型扩展位于 `mirage-os/pkg/models/`。

## 术语表

- **Budget_Engine**：预算引擎，负责编排动作的成本估算、服务等级校验和预算判定
- **BudgetProfile**：预算配置对象，定义单个会话或全局的资源预算上限和权限开关
- **Internal_Cost_Model**：内部成本模型，计算编排动作的带宽、延迟、切换、入口消耗、Gateway 负载五类成本
- **CostEstimate**：成本估算结果对象，包含五类成本分量和总成本
- **External_SLA_Policy**：外部服务等级策略，定义不同 ServiceClass 对应的权限和约束
- **ServiceClass**：服务等级枚举（Standard / Platinum / Diamond），定义于 Spec 4-1
- **BudgetVerdict**：预算判定结果枚举（allow / allow_degraded / allow_with_charge / deny_and_hold / deny_and_suspend）
- **BudgetDecision**：预算判定完整响应对象，包含 verdict、成本明细和拒绝原因
- **CommitTransaction**：提交事务对象，定义于 Spec 4-3，Budget_Engine 在 Validate Constraint 阶段对其进行预算校验
- **BudgetChecker**：预算校验接口，定义于 Spec 4-3 的 `validators.go`，Budget_Engine 实现该接口替换 DefaultBudgetChecker
- **ServiceClassChecker**：服务等级校验接口，定义于 Spec 4-3 的 `validators.go`，Budget_Engine 实现该接口替换 DefaultServiceClassChecker
- **TxType**：事务类型枚举（PersonaSwitch / LinkMigration / GatewayReassignment / SurvivalModeSwitch），定义于 Spec 4-3
- **SurvivalMode**：生存姿态枚举（Normal / LowNoise / Hardened / Degraded / Escape / LastResort），定义于 Spec 4-1
- **BudgetLedger**：预算账本，记录预算消耗历史，支持滑动窗口统计

## 需求

### 需求 1：BudgetProfile 定义与管理

**用户故事：** 作为编排系统运维人员，我希望为每个会话或全局定义预算配置，以便约束编排动作的资源消耗上限。

#### 验收标准

1. THE BudgetProfile SHALL 包含以下数值预算字段：latency_budget_ms（int64，延迟预算毫秒数）、bandwidth_budget_ratio（float64，带宽预算比率 0.0-1.0）、switch_budget_per_hour（int，每小时切换预算次数）、entry_burn_budget_per_day（int，每日入口消耗预算次数）、gateway_load_budget（float64，Gateway 负载预算比率 0.0-1.0）
2. THE BudgetProfile SHALL 包含以下布尔权限字段：hardened_allowed（是否允许 Hardened 模式）、escape_allowed（是否允许 Escape 模式）、last_resort_allowed（是否允许 LastResort 模式）
3. WHEN 创建 BudgetProfile 时，THE Budget_Engine SHALL 校验所有数值字段在合法范围内：latency_budget_ms 大于 0，bandwidth_budget_ratio 在 0.0 到 1.0 之间（含边界），switch_budget_per_hour 大于等于 0，entry_burn_budget_per_day 大于等于 0，gateway_load_budget 在 0.0 到 1.0 之间（含边界）
4. IF BudgetProfile 的任何数值字段超出合法范围，THEN THE Budget_Engine SHALL 返回包含字段名和合法范围的校验错误
5. THE Budget_Engine SHALL 提供 DefaultBudgetProfile 函数，返回适用于 Standard 服务等级的默认预算配置

### 需求 2：Internal Cost Model

**用户故事：** 作为编排引擎，我希望在执行动作前估算内部资源成本，以便判断该动作是否在预算范围内。

#### 验收标准

1. THE Internal_Cost_Model SHALL 为每个 CommitTransaction 计算五类成本分量：bandwidth_cost（带宽增量成本）、latency_cost（延迟增加成本）、switch_cost（切换次数成本）、entry_burn_cost（入口消耗成本）、gateway_load_cost（Gateway 负载成本）
2. WHEN 估算 PersonaSwitch 类型事务的成本时，THE Internal_Cost_Model SHALL 计算 bandwidth_cost 和 latency_cost 两个分量
3. WHEN 估算 LinkMigration 类型事务的成本时，THE Internal_Cost_Model SHALL 计算 switch_cost、latency_cost 和 bandwidth_cost 三个分量
4. WHEN 估算 GatewayReassignment 类型事务的成本时，THE Internal_Cost_Model SHALL 计算 gateway_load_cost、entry_burn_cost 和 switch_cost 三个分量
5. WHEN 估算 SurvivalModeSwitch 类型事务的成本时，THE Internal_Cost_Model SHALL 计算全部五类成本分量
6. THE CostEstimate SHALL 包含五类成本分量和一个 total_cost 字段，total_cost 等于五类分量之和
7. THE Internal_Cost_Model SHALL 保证所有成本分量为非负值

### 需求 3：External SLA / Service Policy

**用户故事：** 作为编排系统，我希望根据用户的服务等级映射对应的权限策略，以便在预算判定中考虑服务等级约束。

#### 验收标准

1. THE External_SLA_Policy SHALL 为 Standard 服务等级定义以下约束：hardened_allowed 为 false，escape_allowed 为 false，last_resort_allowed 为 false，max_switch_per_hour 为 5，max_entry_burn_per_day 为 2
2. THE External_SLA_Policy SHALL 为 Platinum 服务等级定义以下约束：hardened_allowed 为 true，escape_allowed 为 false，last_resort_allowed 为 false，max_switch_per_hour 为 15，max_entry_burn_per_day 为 5
3. THE External_SLA_Policy SHALL 为 Diamond 服务等级定义以下约束：hardened_allowed 为 true，escape_allowed 为 true，last_resort_allowed 为 true，max_switch_per_hour 为 30，max_entry_burn_per_day 为 10
4. WHEN 查询不存在的 ServiceClass 对应的策略时，THE External_SLA_Policy SHALL 返回 Standard 等级的默认策略
5. THE External_SLA_Policy SHALL 提供 GetPolicy 函数，接受 ServiceClass 参数并返回对应的策略对象

### 需求 4：预算判定流程

**用户故事：** 作为编排引擎，我希望在提出动作后获得明确的预算判定结果，以便决定是否执行、降级执行或拒绝该动作。

#### 验收标准

1. THE Budget_Engine SHALL 支持五种判定结果：allow（允许执行）、allow_degraded（允许但降级执行）、allow_with_charge（允许但额外计费）、deny_and_hold（拒绝并保持当前状态）、deny_and_suspend（拒绝并挂起会话）
2. WHEN 编排器提出动作时，THE Budget_Engine SHALL 按以下顺序执行判定：第一步估算内部成本，第二步检查服务等级权限，第三步比较成本与预算余量，第四步返回 BudgetDecision
3. WHEN 估算成本的全部分量均在预算范围内且服务等级权限允许时，THE Budget_Engine SHALL 返回 allow 判定
4. WHEN 估算成本超出预算但超出比例在 20% 以内且服务等级权限允许时，THE Budget_Engine SHALL 返回 allow_degraded 判定
5. WHEN 估算成本超出预算且超出比例超过 20% 时，THE Budget_Engine SHALL 返回 deny_and_hold 判定
6. WHEN 服务等级权限不允许目标 SurvivalMode 时，THE Budget_Engine SHALL 返回 deny_and_hold 判定，BudgetDecision 包含被拒绝的 SurvivalMode 名称和当前 ServiceClass
7. WHEN 会话的累计预算消耗超过日预算上限的 150% 时，THE Budget_Engine SHALL 返回 deny_and_suspend 判定
8. THE BudgetDecision SHALL 包含以下字段：verdict（判定结果）、cost_estimate（成本估算明细）、remaining_budget（剩余预算快照）、deny_reason（拒绝原因，仅在 deny 时非空）

### 需求 5：BudgetChecker 接口实现

**用户故事：** 作为 Commit Engine，我希望在 Validate Constraint 阶段调用 Budget Engine 进行预算校验，以便替换默认的始终通过校验器。

#### 验收标准

1. THE Budget_Engine SHALL 实现 Spec 4-3 定义的 BudgetChecker 接口，接受 context.Context 和 CommitTransaction 指针作为参数
2. WHEN BudgetChecker.Check 被调用时，THE Budget_Engine SHALL 从 CommitTransaction 中提取 tx_type、target_session_id、target_survival_mode 字段用于预算判定
3. WHEN 预算判定结果为 allow 或 allow_degraded 或 allow_with_charge 时，THE BudgetChecker SHALL 返回 nil 错误
4. WHEN 预算判定结果为 deny_and_hold 或 deny_and_suspend 时，THE BudgetChecker SHALL 返回包含 verdict 和 deny_reason 的 ErrBudgetDenied 错误
5. WHEN CommitTransaction 的 target_session_id 为空（全局事务）时，THE BudgetChecker SHALL 使用全局 BudgetProfile 进行判定

### 需求 6：ServiceClassChecker 接口实现

**用户故事：** 作为 Commit Engine，我希望在 Validate Constraint 阶段调用服务等级校验，以便确保编排动作符合用户的服务等级权限。

#### 验收标准

1. THE Budget_Engine SHALL 实现 Spec 4-3 定义的 ServiceClassChecker 接口，接受 context.Context 和 CommitTransaction 指针作为参数
2. WHEN ServiceClassChecker.Check 被调用时，THE Budget_Engine SHALL 从 CommitTransaction 的 target_session_id 获取对应 Session 的 ServiceClass
3. WHEN CommitTransaction 的 tx_type 为 SurvivalModeSwitch 且目标 SurvivalMode 为 Hardened 时，THE ServiceClassChecker SHALL 校验当前 ServiceClass 的 hardened_allowed 权限
4. WHEN CommitTransaction 的 tx_type 为 SurvivalModeSwitch 且目标 SurvivalMode 为 Escape 时，THE ServiceClassChecker SHALL 校验当前 ServiceClass 的 escape_allowed 权限
5. WHEN CommitTransaction 的 tx_type 为 SurvivalModeSwitch 且目标 SurvivalMode 为 LastResort 时，THE ServiceClassChecker SHALL 校验当前 ServiceClass 的 last_resort_allowed 权限
6. WHEN 服务等级校验通过时，THE ServiceClassChecker SHALL 返回 nil 错误
7. WHEN 服务等级校验失败时，THE ServiceClassChecker SHALL 返回包含 ServiceClass 和被拒绝的 SurvivalMode 的 ErrServiceClassDenied 错误

### 需求 7：预算账本与滑动窗口统计

**用户故事：** 作为 Budget Engine，我希望记录每次预算消耗并支持滑动窗口统计，以便准确判断切换频率和入口消耗是否超出预算。

#### 验收标准

1. THE BudgetLedger SHALL 记录每次预算消耗事件，包含：session_id、tx_type、cost_estimate、timestamp
2. WHEN 计算 switch_budget_per_hour 消耗时，THE BudgetLedger SHALL 统计过去 1 小时内该 session 的 LinkMigration 和 GatewayReassignment 事务数量
3. WHEN 计算 entry_burn_budget_per_day 消耗时，THE BudgetLedger SHALL 统计过去 24 小时内该 session 的 GatewayReassignment 事务数量
4. THE BudgetLedger SHALL 自动清理超过 24 小时的历史记录，防止内存无限增长
5. THE BudgetLedger SHALL 支持并发安全的读写操作

### 需求 8：BudgetProfile 持久化

**用户故事：** 作为系统运维人员，我希望 BudgetProfile 持久化到数据库，以便在系统重启后恢复预算配置。

#### 验收标准

1. THE Budget_Engine SHALL 将 BudgetProfile 持久化到 PostgreSQL 的 budget_profiles 表
2. THE budget_profiles 表 SHALL 包含以下字段：profile_id（主键）、session_id（可选，为空表示全局配置）、所有 BudgetProfile 数值和布尔字段、created_at、updated_at
3. WHEN 系统启动时，THE Budget_Engine SHALL 从数据库加载所有 BudgetProfile
4. WHEN BudgetProfile 被创建或更新时，THE Budget_Engine SHALL 同步持久化到数据库
5. WHEN 查询不存在的 session_id 对应的 BudgetProfile 时，THE Budget_Engine SHALL 返回 DefaultBudgetProfile

### 需求 9：BudgetProfile JSON 序列化

**用户故事：** 作为 API 层，我希望 BudgetProfile 和 BudgetDecision 支持 JSON 序列化和反序列化，以便通过 HTTP API 传输。

#### 验收标准

1. FOR ALL 合法的 BudgetProfile 对象，JSON 序列化后再反序列化 SHALL 产生等价对象（所有字段值保持不变）
2. FOR ALL 合法的 BudgetDecision 对象，JSON 序列化后再反序列化 SHALL 产生等价对象
3. FOR ALL 合法的 CostEstimate 对象，JSON 序列化后再反序列化 SHALL 产生等价对象
4. THE BudgetProfile 的 JSON 序列化 SHALL 使用 snake_case 字段命名

### 需求 10：错误类型定义

**用户故事：** 作为调用方，我希望 Budget Engine 返回结构化的错误类型，以便精确处理不同的拒绝原因。

#### 验收标准

1. THE Budget_Engine SHALL 定义 ErrBudgetDenied 错误类型，包含 Verdict（BudgetVerdict）和 Reason（string）字段
2. THE Budget_Engine SHALL 定义 ErrServiceClassDenied 错误类型，包含 ServiceClass（string）和 DeniedMode（string）字段
3. THE Budget_Engine SHALL 定义 ErrInvalidBudgetProfile 错误类型，包含 Field（string）和 Message（string）字段
4. WHEN ErrBudgetDenied 的 Error() 方法被调用时，THE 错误消息 SHALL 包含 verdict 和 reason 信息
5. WHEN ErrServiceClassDenied 的 Error() 方法被调用时，THE 错误消息 SHALL 包含 service_class 和 denied_mode 信息
