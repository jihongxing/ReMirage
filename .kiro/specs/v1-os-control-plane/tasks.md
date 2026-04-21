# 任务清单：多节点架构 P0 — OS 控制面补齐

## 任务

- [x] 1. Proto 扩展：注册 RPC 与心跳拓扑语义
  - [x] 1.1 新增 `RegisterRequest`、`RegisterResponse`、`GatewayCapabilities` 消息定义
  - [x] 1.2 在 `GatewayUplink` service 中新增 `RegisterGateway` RPC
  - [x] 1.3 扩展 `HeartbeatRequest`：增加 `downlink_addr`、`cell_id`、`active_sessions`、`state_hash`、`version` 字段
  - [x] 1.4 扩展 `HeartbeatResponse`：增加 `needs_full_sync`、`desired_state_hash` 字段
  - [x] 1.5 重新生成 Go proto 代码

- [x] 2. 拓扑索引管理器
  - [x] 2.1 新建 `gateway-bridge/pkg/topology/registry.go`，实现 Registry（内存索引 + DB 持久化 + Redis 缓存三写）
  - [x] 2.2 实现 `Register` 方法：DB UPSERT + Redis 拓扑索引写入 + 内存索引更新
  - [x] 2.3 实现 `GetGatewaysByCell` 和 `GetAllOnline` 查询方法
  - [x] 2.4 实现 `UpdateHeartbeat` 方法：刷新 LastHeartbeat 和 ActiveSessions
  - [x] 2.5 实现 `MarkOffline` 方法：DB + Redis + 内存三处标记下线
  - [x] 2.6 实现 `StartTimeoutChecker`：每 60 秒检查心跳超时（300 秒），超时自动 MarkOffline
  - [x] 2.7 实现 `loadFromDB`：启动时从 DB 加载已有 Gateway 信息到内存索引

- [x] 3. Gateway 注册 RPC 实现
  - [x] 3.1 在 `gateway-bridge/pkg/grpc/server.go` 中实现 `RegisterGateway` handler，调用 Registry.Register
  - [x] 3.2 在 Gateway 侧 `cmd/gateway/main.go` 中增加启动时注册逻辑：先 RegisterGateway 再启动心跳循环
  - [x] 3.3 在 Gateway 侧 `pkg/api/grpc_client.go` 中增加 `Register` 方法

- [x] 4. 心跳处理改造
  - [x] 4.1 改造 `SyncHeartbeat` handler：调用 Registry.UpdateHeartbeat 更新拓扑索引
  - [x] 4.2 增加状态对齐检查：比对 `state_hash`，不一致时设置 `needs_full_sync=true`
  - [x] 4.3 在 Gateway 侧心跳上报中携带 `downlink_addr`、`cell_id`、`active_sessions`、`state_hash`、`version`
  - [x] 4.4 在 Gateway 侧处理心跳响应：收到 `needs_full_sync=true` 时拉取全量 Desired State 并对齐
  - [x] 4.5 在心跳超时处理中调用 SessionService.onGatewayTimeout 批量标记会话断开（复用 Spec 2-2）

- [-] 5. 统一 Fan-out 引擎
  - [~] 5.1 新建 `gateway-bridge/pkg/dispatch/fanout.go`，实现 FanoutEngine（支持 Single/Cell/Global 三种 scope）
  - [~] 5.2 实现 `resolveTargets`：从 Registry 查询目标 Gateway 列表
  - [~] 5.3 实现下推重试逻辑：最多 3 次指数退避重试，失败后记录告警
  - [~] 5.4 改造现有 `PushStrategyToCell`：从 Redis SCAN 改为通过 Registry.GetGatewaysByCell 查询
  - [~] 5.5 改造现有 `PushBlacklistToAll`：从遍历 connections map 改为通过 Registry.GetAllOnline 查询

- [-] 6. 下推状态记录
  - [~] 6.1 新建 `gateway-bridge/pkg/dispatch/push_log.go`，实现 PushLog（内存环形缓冲 + 异步 DB 持久化）
  - [~] 6.2 在 Prisma schema 中新增 `push_logs` 表并运行迁移
  - [~] 6.3 在 FanoutEngine 每次下推后调用 PushLog.Record 记录结果
  - [~] 6.4 实现 `GetRecent` 方法供查询 API 使用

- [ ] 7. DB Schema 扩展
  - [~] 7.1 在 `Gateway` 模型中增加 `downlink_addr`、`version`、`max_sessions`、`active_sessions` 字段
  - [~] 7.2 运行 Prisma migrate 生成数据库迁移

- [ ] 8. NestJS 查询 API
  - [~] 8.1 在 GatewaysController 中增加 `GET /gateways/topology/by-cell/:cellId` 接口（按 Cell 查在线 Gateway）
  - [~] 8.2 在 GatewaysController 中增加 `GET /gateways/topology/online` 接口（查所有在线 Gateway）
  - [~] 8.3 在 GatewaysController 中增加 `GET /gateways/push-logs` 接口（查最近下推记录）
  - [~] 8.4 为以上接口增加 RBAC 权限校验（仅 admin/operator 可访问）

- [ ] 9. 控制面一致性测试
  - [~] 9.1 编写 Gateway 注册测试：注册后 Registry 可查到，拓扑 API 返回正确
  - [~] 9.2 编写按 Cell 下推测试：Cell 内 Gateway 收到策略，Cell 外不受影响
  - [~] 9.3 编写 Gateway 下线测试：心跳超时后 MarkOffline + 会话标记断开
  - [~] 9.4 编写 Gateway 重注册测试：downlink_addr 变更后拓扑索引更新
  - [~] 9.5 编写下推失败重试测试：目标不可达时重试 3 次后记录告警
  - [~] 9.6 编写状态对齐测试：state_hash 不一致时触发全量同步
  - [~] 9.7 在 Makefile 增加 `test-control-plane` target
