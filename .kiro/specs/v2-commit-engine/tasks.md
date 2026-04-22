# 任务清单：V2 State Commit Engine

## 任务

- [x] 1 枚举与类型定义
  - [x] 1.1 在 `mirage-gateway/pkg/orchestrator/commit/types.go` 中定义 TxType、TxPhase、TxScope 枚举常量
  - [x] 1.2 在 `mirage-gateway/pkg/orchestrator/commit/types.go` 中定义 TxTypeScopeMap 和 TxScopePriority 映射
  - [x] 1.3 在 `mirage-gateway/pkg/orchestrator/commit/types.go` 中定义 TerminalPhases 集合和 IsTerminal 辅助函数

- [x] 2 CommitTransaction 结构体与持久化
  - [x] 2.1 在 `mirage-gateway/pkg/orchestrator/commit/transaction.go` 中定义 CommitTransaction 结构体（含 GORM 标签和 JSON 标签）
  - [x] 2.2 在 `mirage-os/pkg/models/db.go` 的 AutoMigrate 中注册 CommitTransaction 模型
  - [x] 2.3 实现 NewCommitTransaction 工厂函数：生成 UUID v4 tx_id、设置 tx_phase=Preparing、设置 rollback_marker=last_successful_epoch、设置 tx_scope=TxTypeScopeMap[tx_type]
  - [x] 2.4 🧪 Property 10: CommitTransaction JSON round-trip 属性测试

- [x] 3 TX_Phase 状态机
  - [x] 3.1 在 `mirage-gateway/pkg/orchestrator/commit/phase_machine.go` 中定义 ValidTransitions 表
  - [x] 3.2 实现 TransitionPhase 函数：校验转换合法性、返回转换时间戳、终态拒绝转换
  - [x] 3.3 🧪 Property 2: TX_Phase 状态机转换合法性属性测试

- [x] 4 冷却时间管理器
  - [x] 4.1 在 `mirage-gateway/pkg/orchestrator/commit/cooldown.go` 中定义 CooldownConfig 和 DefaultCooldownConfig
  - [x] 4.2 实现 CooldownManager：CheckCooldown（比较时间差与冷却期）、RecordCompletion（记录完成时间）
  - [x] 4.3 🧪 Property 5: 冷却时间判定属性测试

- [x] 5 事务冲突管理器
  - [x] 5.1 在 `mirage-gateway/pkg/orchestrator/commit/conflict.go` 中实现 ConflictManager：CheckConflict、RegisterActive、UnregisterActive
  - [x] 5.2 实现优先级抢占逻辑：高优先级事务回滚低优先级事务后注册自身
  - [x] 5.3 🧪 Property 3: 作用域活跃事务唯一性属性测试
  - [x] 5.4 🧪 Property 4: 优先级抢占正确性属性测试

- [x] 6 预留校验接口
  - [x] 6.1 在 `mirage-gateway/pkg/orchestrator/commit/validators.go` 中定义 BudgetChecker 和 ServiceClassChecker 接口
  - [x] 6.2 实现 DefaultBudgetChecker 和 DefaultServiceClassChecker（始终返回 nil）

- [x] 7 阶段执行器
  - [x] 7.1 在 `mirage-gateway/pkg/orchestrator/commit/phases.go` 中定义 PhaseExecutor 接口
  - [x] 7.2 实现 Prepare 阶段：收集 Session/Link/Persona/SurvivalMode/Epoch 快照，写入 prepare_state，目标不存在时设为 Failed
  - [x] 7.3 实现 Validate 阶段：调用 CooldownManager + ConflictManager + BudgetChecker + ServiceClassChecker，写入 validate_state
  - [x] 7.4 实现 ShadowWrite 阶段：根据 tx_type 分派影子写入（PersonaSwitch→PersonaEngine.WriteShadow、LinkMigration→shadow route、SurvivalModeSwitch→shadow mode），写入 rollback_marker 到 ControlState
  - [x] 7.5 实现 Flip 阶段：根据 tx_type 分派切换操作（PersonaSwitch→Atomic_Flip、LinkMigration→UpdateLink、GatewayReassignment→更新 gateway_id、SurvivalModeSwitch→更新 survival_mode），递增 epoch
  - [x] 7.6 实现 Acknowledge 阶段：校验 ControlState epoch 一致、PersonaSwitch 校验 active_slot
  - [x] 7.7 实现 Commit 阶段：更新 last_successful_epoch、rollback_marker、清空 active_tx_id、记录 finished_at
  - [x] 7.8 实现 Rollback 逻辑：根据 tx_type 调用对应回滚（PersonaSwitch→PersonaEngine.Rollback）、恢复 epoch 到 rollback_marker、清空 active_tx_id、记录 finished_at
  - [x] 7.9 🧪 Property 1: 事务创建初始状态不变量属性测试
  - [x] 7.10 🧪 Property 9: Prepare 阶段快照完整性属性测试
  - [x] 7.11 🧪 Property 6: Committed 后 ControlState 一致性属性测试
  - [x] 7.12 🧪 Property 7: RolledBack 后 ControlState 一致性属性测试

- [x] 8 CommitEngine 主体
  - [x] 8.1 在 `mirage-gateway/pkg/orchestrator/commit/engine.go` 中定义 CommitEngine 接口和 BeginTxRequest、TxFilter 结构体
  - [x] 8.2 实现 commitEngineImpl：注入 ControlStateManager、SessionStateManager、LinkStateManager、PersonaEngine、LockManager、CooldownManager、ConflictManager、BudgetChecker、ServiceClassChecker、GORM DB
  - [x] 8.3 实现 BeginTransaction：冲突检查 → 冷却时间检查 → 创建 CommitTransaction → 持久化 → 设置 active_tx_id
  - [x] 8.4 实现 ExecuteTransaction：按顺序执行 Prepare → Validate → ShadowWrite → Flip → Acknowledge → Commit，任一阶段失败触发 Rollback
  - [x] 8.5 实现 GetTransaction、ListTransactions、GetActiveTransactions 查询方法

- [x] 9 崩溃恢复
  - [x] 9.1 实现 RecoverOnStartup：查询未完成事务 → 逐个回滚 → 恢复 epoch 到 rollback_marker → 清空 active_tx_id → 设 control_health=Recovering
  - [x] 9.2 实现 rollback_marker 不可恢复时设 control_health=Faulted 的错误处理
  - [x] 9.3 🧪 Property 8: 崩溃恢复正确性属性测试

- [x] 10 错误类型定义
  - [x] 10.1 在 `mirage-gateway/pkg/orchestrator/commit/errors.go` 中定义所有错误类型：ErrTxConflict、ErrCooldownActive、ErrInvalidPhaseTransition、ErrTerminalPhase、ErrSessionNotFound、ErrLinkNotFound

- [x] 11 Transaction Query API
  - [x] 11.1 在 mirage-os 中实现 `GET /api/v2/transactions/{tx_id}` 端点
  - [x] 11.2 在 mirage-os 中实现 `GET /api/v2/transactions` 端点（支持 tx_type、tx_phase、target_session_id、时间范围过滤）
  - [x] 11.3 在 mirage-os 中实现 `GET /api/v2/transactions/active` 端点
  - [x] 11.4 实现 404 错误处理和 JSON 响应格式（时间戳 RFC 3339）
