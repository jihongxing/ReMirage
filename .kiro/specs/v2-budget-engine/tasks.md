# 任务清单：V2 Budget Engine

## 1. 枚举、常量与错误类型

- [x] 1.1 创建 `mirage-gateway/pkg/orchestrator/budget/types.go`，定义 BudgetVerdict 枚举（allow / allow_degraded / allow_with_charge / deny_and_hold / deny_and_suspend）和阈值常量（OverBudgetThreshold=0.20, DailySuspendThreshold=1.50）
- [x] 1.2 创建 `mirage-gateway/pkg/orchestrator/budget/errors.go`，定义 ErrBudgetDenied、ErrServiceClassDenied、ErrInvalidBudgetProfile 三个错误类型及其 Error() 方法
- [x] 1.3 创建 `mirage-gateway/pkg/orchestrator/budget/errors_test.go`，编写错误类型单元测试：验证 Error() 包含正确字段信息
- [x] 1.4 [PBT] 创建 `mirage-gateway/pkg/orchestrator/budget/errors_prop_test.go`，Property 9：错误消息内容正确性——生成随机 ErrBudgetDenied 和 ErrServiceClassDenied，验证 Error() 包含所有必要字段值

## 2. BudgetProfile 定义与校验

- [x] 2.1 创建 `mirage-gateway/pkg/orchestrator/budget/profile.go`，定义 BudgetProfile 结构体（含 GORM 标签和 JSON 标签 snake_case）、TableName()、Validate() 方法、DefaultBudgetProfile() 函数
- [x] 2.2 创建 `mirage-gateway/pkg/orchestrator/budget/profile_test.go`，编写单元测试：DefaultBudgetProfile 默认值、Validate 边界值（0、1.0 边界）
- [x] 2.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/budget/profile_prop_test.go`，Property 1：BudgetProfile 校验正确性——生成随机 BudgetProfile，验证合法值通过、非法值返回含字段名的错误

## 3. CostEstimate 与 InternalCostModel

- [x] 3.1 创建 `mirage-gateway/pkg/orchestrator/budget/cost.go`，定义 CostEstimate 结构体和 InternalCostModel 接口
- [x] 3.2 创建 `mirage-gateway/pkg/orchestrator/budget/cost_model.go`，实现 DefaultCostModel：按事务类型计算对应成本分量，total_cost = 五类分量之和
- [x] 3.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/budget/cost_prop_test.go`，Property 2：成本分量矩阵正确性——生成随机 CommitTransaction，验证各 TxType 仅对应分量非零且全部非负
- [x] 3.4 [PBT] 在 `cost_prop_test.go` 中追加 Property 3：CostEstimate 总成本不变量——验证 total_cost == 五类分量之和

## 4. ExternalSLAPolicy

- [x] 4.1 创建 `mirage-gateway/pkg/orchestrator/budget/sla.go`，定义 SLAPolicy 结构体和 ExternalSLAPolicy 接口
- [x] 4.2 创建 `mirage-gateway/pkg/orchestrator/budget/sla_impl.go`，实现 DefaultSLAPolicy：Standard / Platinum / Diamond 三档策略常量表，未知 ServiceClass 返回 Standard
- [x] 4.3 创建 `mirage-gateway/pkg/orchestrator/budget/sla_test.go`，编写单元测试：三档策略精确值验证、未知 ServiceClass 回退到 Standard

## 5. BudgetLedger 滑动窗口

- [x] 5.1 创建 `mirage-gateway/pkg/orchestrator/budget/ledger.go`，定义 LedgerEntry 结构体和 BudgetLedger 接口
- [x] 5.2 创建 `mirage-gateway/pkg/orchestrator/budget/ledger_impl.go`，实现 InMemoryLedger：sync.Mutex 保护的内存账本，Record / SwitchCountInLastHour / EntryBurnCountInLastDay / Cleanup 方法
- [x] 5.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/budget/ledger_prop_test.go`，Property 6：滑动窗口计数正确性——生成随机条目集合，验证 SwitchCountInLastHour 和 EntryBurnCountInLastDay 返回正确计数
- [x] 5.4 [PBT] 在 `ledger_prop_test.go` 中追加 Property 7：账本清理正确性——生成随机条目，Cleanup 后无超过 24h 的条目且 24h 内条目保留

## 6. BudgetDecision 与判定逻辑

- [x] 6.1 创建 `mirage-gateway/pkg/orchestrator/budget/decision.go`，定义 BudgetDecision 结构体
- [x] 6.2 创建 `mirage-gateway/pkg/orchestrator/budget/checker.go`，实现 BudgetCheckerImpl：Evaluate 方法（完整判定流程）和 Check 方法（实现 commit.BudgetChecker 接口）
- [x] 6.3 [PBT] 创建 `mirage-gateway/pkg/orchestrator/budget/checker_prop_test.go`，Property 4：预算判定决策树正确性——生成随机事务/预算/账本组合，验证 verdict 符合决策树规则

## 7. ServiceClassChecker 实现

- [x] 7.1 创建 `mirage-gateway/pkg/orchestrator/budget/service_class_checker.go`，定义 SessionGetter 接口，实现 ServiceClassCheckerImpl（实现 commit.ServiceClassChecker 接口）
- [x] 7.2 [PBT] 创建 `mirage-gateway/pkg/orchestrator/budget/service_class_checker_prop_test.go`，Property 5：ServiceClassChecker 权限校验正确性——生成随机 ServiceClass 和 SurvivalMode 组合，验证校验结果与 SLAPolicy 一致

## 8. JSON 序列化

- [x] 8.1 [PBT] 创建 `mirage-gateway/pkg/orchestrator/budget/json_prop_test.go`，Property 8：JSON round-trip——生成随机 BudgetProfile、BudgetDecision、CostEstimate，验证 marshal/unmarshal 等价且 BudgetProfile 使用 snake_case 键名

## 9. BudgetProfileStore 持久化

- [x] 9.1 创建 `mirage-gateway/pkg/orchestrator/budget/store.go`，定义 BudgetProfileStore 接口
- [x] 9.2 创建 `mirage-gateway/pkg/orchestrator/budget/store_impl.go`，实现 GormBudgetProfileStore：Get（不存在返回 Default）、Save、LoadAll 方法
- [x] 9.3 在 `mirage-os/pkg/models/db.go` 的 AutoMigrate 中注册 BudgetProfile 模型

## 10. 集成与接线

- [x] 10.1 创建 `mirage-gateway/pkg/orchestrator/budget/budget.go`，提供 NewBudgetEngine 工厂函数，组装 BudgetCheckerImpl 和 ServiceClassCheckerImpl 及其依赖
- [x] 10.2 创建 `mirage-gateway/pkg/orchestrator/budget/ledger_concurrent_test.go`，编写 BudgetLedger 并发安全集成测试（多 goroutine 读写无 data race）
