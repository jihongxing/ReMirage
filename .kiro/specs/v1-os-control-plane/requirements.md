# 需求文档：多节点架构 P0 — OS 控制面补齐

## 简介

本 Spec 对应 `多节点架构整改清单.md` 中 OS-1、OS-2、Proto-2、OS-4，目标是让 Mirage OS 从"能收心跳"升级为"能注册、能索引、能按维度下推、能一致性验收"的真正控制平面。

当前状态（Spec 2-2 完成后）：
1. OS 已有归属映射服务（gateway_sessions/client_sessions），能精确知道谁在哪
2. gateway-bridge 已有 StrategyDispatcher，能向 Gateway 推送策略/黑名单/配额
3. DownlinkService 已有 Desired State 模型（Redis 存储期望状态 + hash 对齐）
4. 心跳处理已有 UPSERT gateways 表 + Redis 在线状态缓存

但仍缺少：
1. **统一注册流程**：Gateway 接入时没有正式注册，拓扑信息散落在 Redis 各 key 中
2. **拓扑索引**：OS 无法一次性查询某 Cell 下所有在线 Gateway 及其承载能力
3. **按维度下推**：PushStrategyToCell 依赖 Redis SCAN 模式匹配，不可靠且无法扩展到按用户下推
4. **下推状态追踪**：下推失败只有 pendingPush 简单重试，无审计、无追踪、无告警
5. **心跳缺少拓扑语义**：心跳只表达"我活着"，不表达"我的下行地址、Cell 归属、承载能力"

## 术语表

- **Gateway 注册**：Gateway 首次接入 OS 时的正式登记流程，写入拓扑索引
- **拓扑索引**：OS 内部维护的 Gateway 全量视图（gateway_id → cell_id/addr/status/capacity）
- **Fan-out**：按维度（单 Gateway/Cell/全局）将指令分发到多个 Gateway
- **下推状态**：每次下推操作的记录（目标、内容、结果、时间戳）
- **Desired State**：Gateway 的期望状态，OS 写入 Redis，Gateway 通过 hash 对齐
- **状态对齐**：Gateway 心跳时上报当前 state_hash，OS 比对后决定是否全量下发

## 需求

### 需求 1：建立真实的 Gateway 注册与拓扑索引（OS-1）

**用户故事：** 作为 OS 控制面，我需要每个 Gateway 接入时有正式的注册流程，以便持有完整的 Gateway 拓扑视图。

#### 验收标准

1. THE OS SHALL 提供 Gateway 注册 RPC（`RegisterGateway`），Gateway 启动时必须先注册再发心跳
2. THE 注册请求 SHALL 包含：gateway_id、cell_id、downlink_addr（下行可达地址）、version（Gateway 版本）、capabilities（eBPF 支持、最大连接数等）
3. THE OS SHALL 在注册成功后将 Gateway 信息写入 `gateways` 表（DB）和拓扑索引缓存（Redis）
4. THE 拓扑索引 SHALL 支持以下查询：
   - 按 cell_id 查询所有在线 Gateway 列表
   - 按 gateway_id 查询完整注册信息（cell_id、downlink_addr、status、capabilities）
   - 查询全局在线 Gateway 数量和分布
5. THE OS SHALL 在 Gateway 心跳超时（300 秒无心跳）后将其标记为 OFFLINE 并从拓扑索引中移除
6. THE OS SHALL 在 Gateway 重新注册时更新拓扑索引（支持 downlink_addr 变更、Cell 漂移）

### 需求 2：把按节点下推与按 Cell 下推做成正式能力（OS-2）

**用户故事：** 作为 OS 控制面，我需要可靠的按维度下推能力，以便对 N 个 Gateway 进行统一控制。

#### 验收标准

1. THE OS SHALL 支持三种下推模式：
   - 按单个 Gateway 下推（指定 gateway_id）
   - 按 Cell 下推（指定 cell_id，fan-out 到该 Cell 下所有在线 Gateway）
   - 按全局下推（fan-out 到所有在线 Gateway）
2. THE 下推能力 SHALL 覆盖四种指令类型：策略下发（PushStrategy）、配额下发（PushQuota）、黑名单下发（PushBlacklist）、转生指令（PushReincarnation）
3. THE 按 Cell 下推 SHALL 从拓扑索引中查询目标 Gateway 列表，而不是通过 Redis SCAN 模式匹配
4. THE 每次下推 SHALL 记录下推状态：目标 gateway_id、指令类型、下推时间、结果（成功/失败/超时）
5. THE 下推失败 SHALL 自动重试（最多 3 次，指数退避），重试仍失败则记录告警日志
6. THE OS SHALL 提供下推状态查询 API（仅 admin/operator），可查看最近 N 次下推记录

### 需求 3：Heartbeat 与状态同步消息补齐拓扑语义（Proto-2）

**用户故事：** 作为 OS 控制面，我需要心跳不只表达"我活着"，还要表达拓扑信息，以便不再依赖外部猜测重建 Gateway 拓扑。

#### 验收标准

1. THE `HeartbeatRequest` proto 消息 SHALL 增加以下字段：
   - `downlink_addr`（string）：Gateway 下行可达地址
   - `cell_id`（string）：Gateway 所属 Cell
   - `active_sessions`（int32）：当前承载的活跃会话数
   - `state_hash`（string）：当前 Desired State 的 hash（用于状态对齐）
   - `version`（string）：Gateway 版本号
2. THE `HeartbeatResponse` proto 消息 SHALL 增加以下字段：
   - `needs_full_sync`（bool）：是否需要全量状态同步
   - `desired_state_hash`（string）：OS 侧期望的 state_hash
3. THE OS 心跳处理 SHALL 在收到心跳时更新拓扑索引中的 Gateway 信息（status、active_sessions、last_heartbeat）
4. THE OS 心跳处理 SHALL 比对 `state_hash`，不一致时在响应中设置 `needs_full_sync=true`
5. THE Gateway 收到 `needs_full_sync=true` 时 SHALL 主动拉取全量 Desired State 并对齐

### 需求 4：多网关控制面一致性验收（OS-4）

**用户故事：** 作为运维工程师，我需要多 Gateway 架构下的控制面行为可验证，以便确认 OS 能稳定管理 N 个 Gateway。

#### 验收标准

1. THE 测试套件 SHALL 包含：Gateway 注册测试（新 Gateway 注册后拓扑索引可查到）
2. THE 测试套件 SHALL 包含：按 Cell 下推测试（指定 Cell 下所有 Gateway 收到策略，Cell 外 Gateway 不受影响）
3. THE 测试套件 SHALL 包含：Gateway 下线测试（心跳超时后拓扑索引移除、会话标记断开）
4. THE 测试套件 SHALL 包含：Gateway 重注册测试（downlink_addr 变更后拓扑索引更新）
5. THE 测试套件 SHALL 包含：下推失败重试测试（目标 Gateway 不可达时重试 3 次后记录告警）
6. THE 测试套件 SHALL 包含：状态对齐测试（Gateway 重启后通过心跳 state_hash 触发全量同步）
7. THE 所有控制面测试 SHALL 可通过 `make test-control-plane` 一键执行
