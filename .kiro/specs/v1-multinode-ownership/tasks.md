# 任务清单：多节点架构 P0 — 归属与计费隔离

## 任务

- [x] 1. Proto 扩展：精确归属与会话事件
  - [x] 1.1 扩展 `TrafficRequest`：增加 `user_id`、`session_id`、`sequence_number` 字段
  - [x] 1.2 扩展 `QuotaPush`：增加 `user_id` 字段
  - [x] 1.3 扩展 `HeartbeatRequest`：增加 `repeated UserQuotaSummary user_quotas` 字段，新增 `UserQuotaSummary` 消息
  - [x] 1.4 新增 `SessionEventType` 枚举、`SessionEventRequest`/`SessionEventResponse` 消息
  - [x] 1.5 在 `GatewayUplink` service 中新增 `ReportSessionEvent` RPC
  - [x] 1.6 重新生成 Go 和 TypeScript 的 proto 代码

- [x] 2. DB Schema 扩展：会话模型与计费幂等
  - [x] 2.1 在 Prisma schema 中新增 `GatewaySession` 模型（session_id, gateway_id, user_id, client_id, status, connected_at, disconnected_at）
  - [x] 2.2 在 Prisma schema 中新增 `ClientSession` 模型（session_id, client_id, user_id, current_gateway_id, status）
  - [x] 2.3 在 `BillingLog` 模型中增加 `session_id` 和 `sequence_number` 字段，增加 `[gatewayId, sequenceNumber]` 唯一约束
  - [x] 2.4 在 `Gateway` 模型中增加 `sessions GatewaySession[]` 关系，在 `User` 模型中增加 `gatewaySessions` 和 `clientSessions` 关系
  - [x] 2.5 运行 Prisma migrate 生成数据库迁移并验证

- [x] 3. Gateway 配额隔离桶
  - [x] 3.1 新建 `pkg/api/quota_bucket.go`，实现 `QuotaBucketManager`（按 user_id 隔离的配额桶，原子操作消费/更新）
  - [x] 3.2 改造 `PushQuota` handler：按 `user_id` 更新对应配额桶，兼容旧模式（user_id 为空时更新全局桶）
  - [x] 3.3 将数据面配额检查从全局桶改为按连接所属 user_id 查找对应桶
  - [x] 3.4 实现配额耗尽回调：仅熔断对应用户的连接，不影响其他用户
  - [x] 3.5 实现 `GetSummaries()` 方法，供心跳上报使用
  - [x] 3.6 编写配额隔离单元测试：两个用户并发消费，一个耗尽不影响另一个

- [x] 4. Gateway 会话管理器
  - [x] 4.1 新建 `pkg/api/session_manager.go`，实现 `SessionManager`（session_id → user_id/client_id 映射）
  - [x] 4.2 在客户端连接建立时调用 `SessionManager.Register` 并通过 gRPC 上报 `ReportSessionEvent(CONNECTED)`
  - [x] 4.3 在客户端断开时调用 `SessionManager.Unregister` 并通过 gRPC 上报 `ReportSessionEvent(DISCONNECTED)`
  - [x] 4.4 将 `SessionManager` 集成到 Gateway 主流程，确保所有连接都经过注册

- [x] 5. Gateway 流量按用户统计与上报
  - [x] 5.1 新建 `pkg/api/traffic_counter.go`，实现 `UserTrafficCounter`（按 user_id + session_id 维度统计）
  - [x] 5.2 在数据面流量统计回调中，通过 `SessionManager.GetUserID` 查找用户并调用 `UserTrafficCounter.Add`
  - [x] 5.3 改造流量上报循环：调用 `Flush()` 获取各用户流量快照，为每个用户生成独立 `TrafficRequest` 并上报
  - [x] 5.4 实现 `sequence_number` 单调递增（Gateway 重启后从文件恢复）
  - [x] 5.5 改造心跳上报：在 `HeartbeatRequest` 中携带 `user_quotas` 摘要

- [x] 6. OS 归属映射服务
  - [x] 6.1 新建 `session.module.ts`、`session.service.ts`，实现会话建立/断开/超时处理逻辑
  - [x] 6.2 新建 `session.controller.ts`，实现归属查询 REST API（按 gateway_id/user_id/client_id 查询，仅 admin/operator 可访问）
  - [x] 6.3 在 gateway-bridge gRPC handler 中实现 `ReportSessionEvent` 处理，转发到 SessionService
  - [x] 6.4 在 Gateway 心跳超时处理中调用 `SessionService.onGatewayTimeout` 批量标记断开

- [x] 7. OS 计费改造
  - [x] 7.1 改造 gateway-bridge `ReportTraffic` handler：优先使用 `user_id` 直接定位用户，user_id 为空时回退 gateway_id 反推
  - [x] 7.2 实现幂等去重：基于 `gateway_id + sequence_number` 检查重复上报
  - [x] 7.3 改造计费写入：`BillingLog` 写入时携带 `session_id` 和 `sequence_number`
  - [x] 7.4 改造 `QuotaBridge`：配额消费和同步按 user_id 维度操作（复用现有逻辑，key 不变）

- [x] 8. OS 配额下发改造
  - [x] 8.1 新建 `gateway-bridge/pkg/dispatch/quota_dispatch.go`，实现按用户维度配额下发
  - [x] 8.2 改造配额下发流程：查询 Gateway 上活跃用户列表，为每个用户生成 `QuotaPush`（携带 user_id）
  - [x] 8.3 在配额变更事件（购买/消费/耗尽）时触发对应 Gateway 的配额下发

- [x] 9. 集成测试
  - [x] 9.1 编写多用户共享 Gateway 配额隔离测试：2 个用户并发跑流量，一个耗尽不影响另一个
  - [x] 9.2 编写计费精确归属测试：同一 Gateway 上多用户流量上报后，OS 能分别看到各自流量
  - [x] 9.3 编写幂等上报测试：重复 sequence_number 不重复计费
  - [x] 9.4 编写会话生命周期测试：连接建立 → 归属查询 → 断开 → 状态更新
