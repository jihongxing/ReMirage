# 任务清单：V2 Survival Orchestrator

## 1. 枚举、常量与错误类型

- [x] 1.1 创建 `mirage-gateway/pkg/orchestrator/survival/types.go`，定义 SwitchAggressiveness 枚举（Conservative / Moderate / Aggressive）、SessionAdmissionPolicy 枚举（Open / RestrictNew / HighPriorityOnly / Closed）、TriggerSource 枚举（LinkHealth / EntryBurn / Budget / Policy）、ValidTransitions 映射表（6 个 key，15 条合法路径）、ModeSeverity 排序映射
- [x] 1.2 创建 `mirage-gateway/pkg/orchestrator/survival/errors.go`，定义 ErrInvalidTransition、ErrConstraintViolation、ErrAdmissionDenied 三个错误类型及其 Error() 方法
- [x] 1.3 创建 `mirage-gateway/pkg/orchestrator/survival/errors_test.go`，编写错误类型单元测试：验证 Error() 包含正确字段信息
- [x] 1.4 创建 `mirage-gateway/pkg/orchestrator/survival/types_test.go`，编写单元测试：ValidTransitions 包含 6 个 key 共 15 条路径、ModeSeverity 排序正确性、枚举值字符串表示

## 2. Survival Mode 状态机

- [x] 2.1 创建 `mirage-gateway/pkg/orchestrator/survival/state_machine.go`，实现 ValidateTransition 函数：检查 (from, to) 是否在 ValidTransitions 中，非法返回 ErrInvalidTransition，自迁移拒绝
- [x] 2.2 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/state_machine_prop_test.go`，Property 1：状态机转换合法性——生成随机 SurvivalMode 对，验证转换结果与 ValidTransitions 表一致

## 3. ModePolicy 绑定

- [x] 3.1 创建 `mirage-gateway/pkg/orchestrator/survival/mode_policy.go`，定义 ModePolicy 结构体（含 JSON snake_case 标签）、TransportPolicyName / PersonaPolicyName / BudgetPolicyName 类型、DefaultModePolicies 常量表（6 种模式的五维策略）
- [x] 3.2 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/mode_policy_prop_test.go`，Property 2：ModePolicy 绑定完整性与正确性——生成随机 SurvivalMode，验证五个字段非空且 switch_aggressiveness / session_admission_policy 与规格表一致

## 4. 触发因素评估

- [x] 4.1 创建 `mirage-gateway/pkg/orchestrator/survival/trigger.go`，定义 TriggerSignal、ModeTransitionAdvice 结构体和 TriggerEvaluator、LinkHealthTrigger、EntryBurnTrigger、BudgetTrigger、PolicyTrigger 接口
- [x] 4.2 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_link_health.go`，实现 LinkHealthTrigger：avg < 10 → Escape，avg < 30 → Degraded，avg < 60 → Hardened，avg ≥ 60 → nil
- [x] 4.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_link_health_prop_test.go`，Property 3：Link Health 触发阈值映射——生成随机链路集合，验证评估结果与阈值表一致
- [x] 4.4 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_entry_burn.go`，实现 EntryBurnTrigger：burnCount > threshold 时产生信号
- [x] 4.5 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_entry_burn_prop_test.go`，Property 4：Entry Burn 触发正确性——生成随机 burnCount 和 threshold，验证触发条件
- [x] 4.6 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_budget.go`，实现 BudgetTrigger：仅 deny_and_suspend 时建议 Degraded
- [x] 4.7 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_budget_prop_test.go`，Property 5：Budget 触发正确性——生成随机 BudgetVerdict，验证仅 deny_and_suspend 触发
- [x] 4.8 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_policy.go`，实现 PolicyTrigger：将外部指令转换为对应模式的触发信号
- [x] 4.9 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_evaluator.go`，实现 TriggerEvaluator：SubmitSignal 收集信号，Evaluate 合并所有信号取最高严重度
- [x] 4.10 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/trigger_evaluator_prop_test.go`，Property 6：触发因素合并取最高严重度——生成随机 TriggerSignal 集合，验证合并结果的 target_mode 为最高严重度且 triggers 包含所有输入信号

## 5. 迁移约束

- [x] 5.1 创建 `mirage-gateway/pkg/orchestrator/survival/constraint.go`，定义 TransitionConstraintConfig 结构体（含 JSON 标签）、DefaultTransitionConstraintConfig 常量、TransitionConstraint 接口
- [x] 5.2 创建 `mirage-gateway/pkg/orchestrator/survival/constraint_impl.go`，实现 TransitionConstraint：minimum_dwell_time 检查、upgrade cooldown 检查、downgrade hysteresis 检查、Policy_Trigger 绕过 cooldown/hysteresis
- [x] 5.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/constraint_prop_test.go`，Property 7：迁移约束综合判定——生成随机迁移场景（current, target, 时间点, triggers），验证约束判定结果正确且错误包含 ConstraintType 和 Remaining

## 6. Transport Fabric 路径管理

- [x] 6.1 创建 `mirage-gateway/pkg/orchestrator/transport/types.go`，定义 PathScore 结构体、TransportPolicy 结构体（含 JSON 标签）、DefaultTransportPolicies 常量表
- [x] 6.2 创建 `mirage-gateway/pkg/orchestrator/transport/scorer.go`，定义 PathScorer 接口，实现 DefaultPathScorer：按权重公式计算路径得分
- [x] 6.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/transport/scorer_prop_test.go`，Property 8：路径评分公式正确性——生成随机 LinkState 和 TransportPolicy，验证得分与公式一致（误差 < 0.01）
- [x] 6.4 创建 `mirage-gateway/pkg/orchestrator/transport/fabric.go`，定义 TransportFabric 接口
- [x] 6.5 创建 `mirage-gateway/pkg/orchestrator/transport/fabric_impl.go`，实现 TransportFabric：SelectBestPath（取最高分）、SwitchPath（通过 CommitEngine）、ApplyPolicy、PrewarmBackup、GetActivePaths（限制 max_parallel_paths）
- [x] 6.6 [PBT] 创建 `mirage-gateway/pkg/orchestrator/transport/fabric_prop_test.go`，Property 9：最优路径选择——生成随机路径集合，验证 SelectBestPath 返回最高分路径
- [x] 6.7 [PBT] 在 `fabric_prop_test.go` 中追加 Property 14：并行路径数量上限——生成随机路径添加序列，验证并行路径数不超过 max_parallel_paths
- [x] 6.8 [PBT] 创建 `mirage-gateway/pkg/orchestrator/transport/types_prop_test.go`，Property 10：Transport Policy 分级正确性——生成随机 SurvivalMode，验证 DefaultTransportPolicies 的 switch_threshold、max_parallel_paths、prewarm_backup 与规格表一致

## 7. Session 准入控制

- [x] 7.1 创建 `mirage-gateway/pkg/orchestrator/survival/admission.go`，定义 SessionAdmissionController 接口
- [x] 7.2 创建 `mirage-gateway/pkg/orchestrator/survival/admission_impl.go`，实现 SessionAdmissionController：按准入矩阵判定（Open 全允许、RestrictNew 仅 Platinum/Diamond、HighPriorityOnly 仅 Diamond、Closed 全拒绝），拒绝返回 ErrAdmissionDenied
- [x] 7.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/admission_prop_test.go`，Property 11：Session 准入矩阵正确性——生成随机 SessionAdmissionPolicy 和 ServiceClass 组合，验证判定结果与矩阵一致且拒绝错误包含正确字段

## 8. G-Switch 适配层

- [x] 8.1 创建 `mirage-gateway/pkg/gswitch/adapter.go`，定义 GSwitchAdapter 接口
- [x] 8.2 创建 `mirage-gateway/pkg/gswitch/adapter_impl.go`，实现 GSwitchAdapter：封装 GSwitchManager，GetEntryBurnCount 统计战死次数，OnDomainBurned 转换为 TriggerSignal，TriggerEscape 委托给 GSwitchManager，IsStandbyPoolEmpty 检查热备池
- [x] 8.3 创建 `mirage-gateway/pkg/gswitch/adapter_test.go`，编写单元测试：域名战死事件转换、热备池为空时建议 Escape、TriggerEscape 委托调用

## 9. SurvivalOrchestrator 核心

- [x] 9.1 创建 `mirage-gateway/pkg/orchestrator/survival/orchestrator.go`，定义 SurvivalOrchestrator 接口和 TransitionRecord 结构体
- [x] 9.2 创建 `mirage-gateway/pkg/orchestrator/survival/orchestrator_impl.go`，实现 SurvivalOrchestrator：GetCurrentMode、GetCurrentPolicy、RequestTransition（ValidateTransition → CheckConstraints → CommitEngine 事务 → 下发 Policy）、EvaluateAndTransition、CheckAdmission、GetTransitionHistory、RecoverOnStartup
- [x] 9.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/orchestrator_prop_test.go`，Property 12：迁移历史记录完整性——生成随机成功迁移，验证 TransitionRecord 包含所有必要字段

## 10. JSON 序列化

- [x] 10.1 [PBT] 创建 `mirage-gateway/pkg/orchestrator/survival/json_prop_test.go`，Property 13：JSON round-trip——生成随机 ModePolicy、TransportPolicy、TransitionConstraintConfig，验证 marshal/unmarshal 等价且所有 JSON key 为 snake_case

## 11. 集成与接线

- [x] 11.1 创建 `mirage-gateway/pkg/orchestrator/survival/survival.go`，提供 NewSurvivalOrchestrator 工厂函数，组装所有依赖（TriggerEvaluator、TransitionConstraint、SessionAdmissionController、TransportFabric、CommitEngine、PersonaEngine、GSwitchAdapter）
- [x] 11.2 创建 `mirage-gateway/pkg/orchestrator/survival/orchestrator_integration_test.go`，编写集成测试：完整模式切换流程（Mock CommitEngine）、策略下发验证、系统重启恢复流程、Escape/LastResort 自动触发 GSwitchManager.TriggerEscape
