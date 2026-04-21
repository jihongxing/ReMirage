# 需求文档：多节点架构 P0 — 归属与计费隔离

## 简介

本 Spec 对应 `多节点架构整改清单.md` 中 P0 项，目标是让 Mirage 从"一个 Gateway 绑定一个用户"升级为"一个 Gateway 承载多个用户/会话"，解决共享 Gateway 场景下的配额串扰和计费归属错误。

当前核心问题：
1. **Proto 层**：`TrafficRequest` 只带 `gateway_id`，OS 无法知道流量属于哪个用户
2. **DB 层**：`gateways` 表无会话模型，无法表达"一个 Gateway 承载多个用户"
3. **Gateway 层**：`QuotaPush` 只有一个 `remaining_bytes`，配额是全局桶，一个用户耗尽影响整机
4. **OS 层**：计费通过 `gateway_id` 反推 `user_id`，共享 Gateway 下完全失效

本轮整改核心目标：**建立精确到用户/会话的归属、配额和计费模型。**

## 术语表

- **Session**：一个客户端到一个 Gateway 的活跃连接，由 `session_id` 唯一标识
- **归属映射**：Client → User → Gateway → Cell 的四层关系链
- **隔离桶**：按 user_id 或 session_id 隔离的配额桶，替代 Gateway 全局桶
- **精确归属**：流量上报携带 user_id + session_id，OS 不再通过 gateway_id 猜测用户
- **幂等上报**：流量上报携带序列号，重放不会重复计费
- **QuotaManager**：OS 侧内存态配额管理器，当前按 uid 管理，本轮保持不变
- **QuotaBridge**：OS 侧 PostgreSQL ↔ Redis 配额桥接器，本轮需适配多会话

## 需求

### 需求 1：TrafficRequest 支持精确归属（Proto-1）

**用户故事：** 作为计费系统，我需要流量上报携带精确的用户和会话身份，以便共享 Gateway 上的并发客户端都能独立计费。

#### 验收标准

1. THE `TrafficRequest` proto 消息 SHALL 增加 `user_id`（string）和 `session_id`（string）字段
2. THE `TrafficRequest` SHALL 增加 `sequence_number`（uint64）字段，用于幂等去重
3. THE Gateway SHALL 在上报流量时按 user_id + session_id 维度拆分统计，而不是整机汇总
4. THE OS 侧 `ReportTraffic` handler SHALL 使用 `user_id` 直接定位用户，不再通过 `gateway_id` 反推
5. IF `user_id` 为空，THEN OS SHALL 回退到通过 `gateway_id` 查找归属（向后兼容）
6. THE OS SHALL 基于 `gateway_id + sequence_number` 做幂等去重，重复上报不重复计费

### 需求 2：DB 从 Gateway 绑定 User 升级为 Gateway 承载多 Session（DB-1）

**用户故事：** 作为系统架构师，我需要数据模型支持一个 Gateway 同时承载多个用户的多个会话，以便共享 Gateway 架构可正确运行。

#### 验收标准

1. THE 数据库 SHALL 新增 `gateway_sessions` 表，记录：session_id、gateway_id、user_id、client_id、status（active/disconnected/migrating）、connected_at、disconnected_at
2. THE 数据库 SHALL 新增 `client_sessions` 表，记录：session_id、client_id、user_id、current_gateway_id、status、created_at、updated_at
3. THE `gateways` 表 SHALL 保持为纯节点信息表，不再承载用户归属语义
4. THE `gateway_sessions` 表 SHALL 支持同一 gateway_id 下多条 active 记录（多用户共享）
5. THE `client_sessions` 表 SHALL 支持 client_id 在不同 gateway_id 间迁移（记录 current_gateway_id 变更）
6. THE 所有现有依赖 `gateway_id → user_id` 反推的查询 SHALL 迁移为通过 `gateway_sessions` 或 `client_sessions` 显式查询

### 需求 3：计费日志可回溯到用户与会话（DB-2）

**用户故事：** 作为财务系统，我需要每条计费流水都能精确回溯到用户和会话，以便同一 Gateway 上多个用户的账单可独立核对。

#### 验收标准

1. THE `billing_logs` 表 SHALL 增加 `session_id`（string）字段
2. THE `billing_logs` 表 SHALL 增加 `sequence_number`（bigint）字段，用于幂等去重
3. THE 计费写入逻辑 SHALL 基于 `gateway_id + sequence_number` 做唯一约束，防止重复计费
4. THE 计费查询 API SHALL 支持按 user_id、gateway_id、session_id 三个维度筛选
5. THE 计费汇总 SHALL 按 user_id 聚合，不再按 gateway_id 聚合

### 需求 4：Gateway 配额从全局桶升级为用户/会话隔离桶（Gateway-1）

**用户故事：** 作为 Gateway 运维，我需要同一台 Gateway 上不同用户的配额互相隔离，以便一个用户耗尽配额不影响其他用户。

#### 验收标准

1. THE Gateway SHALL 维护按 user_id 隔离的配额桶（`map[string]*UserQuota`），替代当前的单一全局配额
2. THE `QuotaPush` proto 消息 SHALL 增加 `user_id`（string）字段，支持按用户下发配额
3. THE Gateway SHALL 在收到 `QuotaPush` 时按 `user_id` 更新对应用户的配额桶
4. THE Gateway 数据面配额检查 SHALL 按连接所属的 user_id 查找对应配额桶
5. IF 某用户配额耗尽，THEN 仅该用户的连接被熔断，其他用户不受影响
6. THE Gateway SHALL 在心跳中上报各用户配额摘要（user_id + remaining_bytes 列表）

### 需求 5：流量上报携带精确归属信息（Gateway-2）

**用户故事：** 作为 Gateway，我需要将流量统计拆分到用户/会话维度再上报，以便 OS 能精确计费。

#### 验收标准

1. THE Gateway SHALL 维护按 user_id + session_id 维度的流量统计计数器
2. THE Gateway SHALL 在上报周期到达时，为每个活跃 user_id 生成独立的 `TrafficRequest`
3. THE 每个 `TrafficRequest` SHALL 携带 user_id、session_id、business_bytes、defense_bytes、sequence_number
4. THE sequence_number SHALL 为每个 gateway_id 单调递增，Gateway 重启后从持久化存储恢复
5. THE Gateway SHALL 支持分批上报：单次 RPC 上报一个用户的流量，多用户分多次 RPC

### 需求 6：OS 建立 Client/User/Gateway 归属映射服务（OS-3）

**用户故事：** 作为 OS 控制面，我需要精确知道每个客户端当前接在哪个 Gateway、属于哪个用户，以便所有计费与配额操作不再依赖猜测。

#### 验收标准

1. THE OS SHALL 提供归属映射服务，支持以下查询：
   - 按 gateway_id 查询当前承载的所有 session 列表
   - 按 user_id 查询当前所有活跃 session 及其所在 Gateway
   - 按 client_id 查询当前 session 及所在 Gateway
2. THE 归属映射 SHALL 在 Gateway 上报会话建立/断开事件时实时更新
3. THE Gateway SHALL 在客户端连接建立时通过 gRPC 上报会话建立事件（session_id、user_id、client_id）
4. THE Gateway SHALL 在客户端断开时通过 gRPC 上报会话断开事件
5. THE OS SHALL 在 Gateway 心跳超时后自动将该 Gateway 下所有 session 标记为 disconnected
6. THE 归属映射查询 SHALL 通过 REST API 暴露（仅 admin/operator 可访问）
