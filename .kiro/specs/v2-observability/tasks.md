# 任务清单：V2 观测与审计

## 1. 数据结构定义
- [x] 1.1 创建 `pkg/orchestrator/audit/audit_record.go`，定义 AuditRecord 结构体（含 GORM tag 和 JSON tag snake_case）、TableName()、Validate() 方法
- [x] 1.2 创建 `pkg/orchestrator/audit/timeline_entries.go`，定义五类时间线条目结构体（SessionTimelineEntry、LinkHealthTimelineEntry、PersonaVersionTimelineEntry、SurvivalModeTimelineEntry、TransactionTimelineEntry）及各自 TableName()
- [x] 1.3 创建 `pkg/orchestrator/audit/diagnostic_views.go`，定义 SessionDiagnostic、SystemDiagnostic（含 ActiveTxInfo）、TransactionDiagnostic 结构体
- [x] 1.4 创建 `pkg/orchestrator/audit/errors.go`，定义 ErrAuditRecordNotFound、ErrSessionNotFound、ErrTransactionNotFound、ErrInvalidAuditRecord 及其 Error() 方法
- [x] 1.5 单元测试：验证所有错误类型的 Error() 方法包含关键字段信息

## 2. 存储接口与实现
- [x] 2.1 创建 `pkg/orchestrator/audit/audit_store.go`，定义 AuditFilter 结构体和 AuditStore 接口（Save/GetByTxID/List/Cleanup）
- [x] 2.2 创建 `pkg/orchestrator/audit/audit_store_impl.go`，实现基于 GORM 的 AuditStore（含过滤查询和按保留天数清理）
- [x] 2.3 创建 `pkg/orchestrator/audit/timeline_store.go`，定义 TimeRange 结构体和 TimelineStore 接口（五类条目的 Save/List + Cleanup）
- [x] 2.4 创建 `pkg/orchestrator/audit/timeline_store_impl.go`，实现基于 GORM 的 TimelineStore（含时间范围过滤和 timestamp 升序排列）

## 3. AuditCollector 审计采集器
- [x] 3.1 创建 `pkg/orchestrator/audit/audit_collector.go`，定义 TransactionProvider、BudgetDecisionProvider 接口和 AuditCollector 接口
- [x] 3.2 创建 `pkg/orchestrator/audit/audit_collector_impl.go`，实现 AuditCollector：OnTransactionFinished（终态→AuditRecord）、Handle（EventHandler 接口）、EventType()
- [x] 3.3 PBT: Property 1 — AuditRecord 字段派生正确性（终态映射 flip_success/rollback_triggered、deny verdict 映射 deny_reason）

## 4. TimelineCollector 时间线采集器
- [x] 4.1 创建 `pkg/orchestrator/audit/timeline_collector.go`，定义 TimelineCollector 接口
- [x] 4.2 创建 `pkg/orchestrator/audit/timeline_collector_impl.go`，实现 TimelineCollector 七个方法（生成 UUID entry_id、填充字段、写入 TimelineStore）
- [x] 4.3 PBT: Property 2 — Session 时间线条目生成完整性
- [x] 4.4 PBT: Property 3 — Link 健康时间线条目生成完整性
- [x] 4.5 PBT: Property 4 — Persona 版本时间线条目生成完整性
- [x] 4.6 PBT: Property 5 — Survival Mode 时间线条目生成完整性
- [x] 4.7 PBT: Property 6 — Transaction 时间线条目生成完整性

## 5. DiagnosticAggregator 诊断聚合器
- [x] 5.1 创建 `pkg/orchestrator/audit/diagnostic_aggregator.go`，定义 DiagnosticAggregator 接口
- [x] 5.2 创建 `pkg/orchestrator/audit/diagnostic_aggregator_impl.go`，实现 DiagnosticAggregator：从 SessionStateManager/LinkStateManager/ControlStateManager/CommitEngine/TimelineStore 聚合三种诊断视图
- [x] 5.3 PBT: Property 7 — Session 诊断视图聚合正确性
- [x] 5.4 PBT: Property 8 — 系统诊断视图聚合正确性
- [x] 5.5 PBT: Property 9 — 事务诊断视图与 stuck_duration 不变量

## 6. 数据保留与清理
- [x] 6.1 PBT: Property 10 — 数据保留清理正确性（AuditStore 和 TimelineStore 的 Cleanup 仅删除超期记录）

## 7. JSON 序列化
- [x] 7.1 PBT: Property 11 — 所有数据结构的 JSON round-trip（AuditRecord + 五类 TimelineEntry + 三种 Diagnostic，snake_case key，RFC 3339 时间戳）

## 8. DB 模型注册
- [x] 8.1 扩展 `mirage-os/pkg/models/db.go` 的 AutoMigrate，注册 AuditRecord、SessionTimelineEntry、LinkHealthTimelineEntry、PersonaVersionTimelineEntry、SurvivalModeTimelineEntry、TransactionTimelineEntry 六个模型

## 9. HTTP API 端点
- [x] 9.1 实现 GET `/api/v2/audit/records` 端点（支持 tx_type、时间范围、rollback_triggered 过滤）
- [x] 9.2 实现 GET `/api/v2/audit/records/{tx_id}` 端点（返回指定事务审计记录，不存在返回 404）
- [x] 9.3 实现 GET `/api/v2/timelines/sessions/{session_id}` 端点（支持时间范围过滤）
- [x] 9.4 实现 GET `/api/v2/timelines/links/{link_id}/health` 端点（支持时间范围过滤）
- [x] 9.5 实现 GET `/api/v2/timelines/personas/{session_id}` 端点（支持时间范围过滤）
- [x] 9.6 实现 GET `/api/v2/timelines/survival-modes` 端点（支持时间范围过滤）
- [x] 9.7 实现 GET `/api/v2/timelines/transactions/{tx_id}` 端点
- [x] 9.8 实现 GET `/api/v2/diagnostics/sessions/{session_id}` 端点（不存在返回 404）
- [x] 9.9 实现 GET `/api/v2/diagnostics/system` 端点
- [x] 9.10 实现 GET `/api/v2/diagnostics/transactions/{tx_id}` 端点（不存在返回 404）

## 10. 集成测试
- [x] 10.1 集成测试：GORM AutoMigrate 创建 6 张新表 + 索引验证
- [x] 10.2 集成测试：AuditStore CRUD（Save → GetByTxID → List 过滤 → Cleanup）
- [x] 10.3 集成测试：TimelineStore 五类条目 CRUD（Save → List 过滤 + timestamp 升序 → Cleanup）
- [x] 10.4 集成测试：AuditCollector 注册为 EventRollbackDone 和 EventBudgetReject 的 EventHandler
- [x] 10.5 集成测试：完整审计流程端到端（CommitTransaction 终态 → AuditCollector → AuditStore → API 查询）
- [x] 10.6 集成测试：完整时间线流程端到端（状态变更 → TimelineCollector → TimelineStore → API 查询）
- [x] 10.7 集成测试：诊断视图 API 端点请求/响应验证
