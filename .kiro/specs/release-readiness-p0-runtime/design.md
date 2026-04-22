# Release Readiness P0 Runtime Bugfix Design

## Overview

Mirage V2 存在 8 个 P0 级运行时阻断缺陷，涵盖编译失败（接口不兼容、未定义变量）、Topology API 契约不一致（字段结构 + HMAC 签名算法）、SQL 列名冲突、Provisioning 路由未挂载、以及集成测试路径不匹配。本设计采用最小化修复策略：适配器模式桥接接口差异、以 Client 端结构为权威统一 Topology 契约、以 GORM model 为权威统一 SQL 列名（ADR-001）、补充路由注册。

## Glossary

- **Bug_Condition (C)**: 触发 8 个 P0 缺陷中任意一个的输入条件集合
- **Property (P)**: 修复后各缺陷对应的正确行为
- **Preservation**: 修复不得改变的现有行为（事件分发、Client 运行时、REST API 等）
- **EventDispatcher**: `mirage-gateway/pkg/orchestrator/events/dispatcher.go` 中定义的事件分发接口，方法签名 `Dispatch(ctx, *ControlEvent)`
- **stealth.EventDispatcher**: `mirage-gateway/pkg/gtunnel/stealth/control_plane.go` 中定义的本地接口，方法签名 `Dispatch(ctx, interface{})`
- **GORM Gateway model**: `mirage-os/pkg/models/db.go` 中的 `Gateway` struct，为数据库列名的 Single Source of Truth（ADR-001）
- **RouteTableResponse (Client)**: `phantom-client/pkg/gtclient/topo.go` 中定义的路由表响应结构，含 `GatewayNode{ip, port, priority, region, cell_id}`、`Signature []byte`、`PublishedAt time.Time`
- **RouteTableResponse (OS)**: `mirage-os/services/topology/handler.go` 中定义的路由表响应结构，含 `GatewayEntry{gateway_id, ip_address, cell_id, status}`、`Signature string`、`PublishedAt string`

## Bug Details

### Bug Condition

8 个缺陷在以下条件下触发：

**Formal Specification:**
```
FUNCTION isBugCondition(input)
  INPUT: input of type BuildOrRuntimeEvent
  OUTPUT: boolean

  // 编译阻断
  IF input.action == "go build mirage-gateway/cmd/gateway"
     AND codeContains(stealth.StealthControlPlaneOpts{Dispatcher: events.EventDispatcher})
     THEN RETURN true  // Bug 1.1

  IF input.action == "go build phantom-client/cmd/phantom"
     AND onBannedCallbackReferences("kr")
     AND NOT variableDefined("kr", onBannedScope)
     THEN RETURN true  // Bug 1.2

  // Topology 契约不兼容
  IF input.action == "client.PullRouteTable"
     AND osResponse.gateways[*].fields != {"ip","port","priority","region","cell_id"}
     THEN RETURN true  // Bug 1.3

  IF input.action == "client.VerifyHMAC"
     AND osHMACMethod == "sprintf_concat"
     AND clientHMACMethod == "json_marshal"
     THEN RETURN true  // Bug 1.4

  // SQL 列名冲突
  IF input.action IN ("registry.Register", "registry.loadFromDB", "grpc.SyncHeartbeat", "rest.handleGateways")
     AND sqlColumnName("heartbeat") NOT CONSISTENT across modules
     THEN RETURN true  // Bug 1.5

  IF input.action IN ("registry.Register", "grpc.SyncHeartbeat")
     AND sqlPrimaryKeyName NOT CONSISTENT across modules
     THEN RETURN true  // Bug 1.6

  // Provisioning 路由未挂载
  IF input.action == "HTTP request to /internal/delivery/*"
     AND NOT routeRegistered("/internal/delivery/*", bridgeHTTPMux)
     THEN RETURN true  // Bug 1.7

  // 集成测试路径不匹配
  IF input.action == "integration_test topology fetch"
     AND testPath == "/api/v1/topology"
     AND osRegisteredPath == "/api/v2/topology"
     THEN RETURN true  // Bug 1.8

  RETURN false
END FUNCTION
```

### Examples

- **Bug 1.1**: `cd mirage-gateway && go build ./cmd/gateway/` → 编译错误 `cannot use v2Dispatcher as stealth.EventDispatcher value`
- **Bug 1.2**: `cd phantom-client && go build ./cmd/phantom/` → 编译错误 `undefined: kr`
- **Bug 1.3**: Client 收到 `{"gateway_id":"gw-1","ip_address":"1.2.3.4"}` → 期望 `{"ip":"1.2.3.4","port":443,"priority":0}` → 字段全部为零值
- **Bug 1.4**: OS 签名 `HMAC(sprintf("%d%s%s", 1, "2024-...", "[{...}]"))` ≠ Client 签名 `HMAC(json.Marshal({gateways,version,published_at}))` → 验签永远失败
- **Bug 1.5**: `registry.Register` 写 `last_heartbeat_at`，`grpc.SyncHeartbeat` 写 `last_heartbeat` → SQL error: column "last_heartbeat" does not exist
- **Bug 1.6**: `registry.Register` 用 `ON CONFLICT (gateway_id)`，`grpc.SyncHeartbeat` 用 `ON CONFLICT (id)` → UPSERT 行为不可预测
- **Bug 1.7**: `POST /internal/delivery/redeem` → 404 Not Found（路由未注册）
- **Bug 1.8**: 集成测试请求 `/api/v1/topology` → mock 返回 200，但真实 OS 注册的是 `/api/v2/topology`

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- V2 EventDispatcher 的完整分发流程（去重、epoch 校验、handler 路由）不变
- phantom-client 非 banned 状态下的所有功能（bootstrap、TUN 转发、拓扑刷新、权限检查）不变
- Client 端 TopoVerifier 的 HMAC 验签、版本单调递增、PublishedAt 反回滚逻辑不变
- gateway-bridge gRPC server 心跳处理中除列名修复外的所有字段写入不变
- gateway-bridge REST handler 返回的 gateway 列表 JSON 结构不变
- provisioning handler 的请求校验、错误处理、响应格式不变
- REST API 的 InternalAuthMiddleware 和 AccessLogMiddleware 对所有路由的保护不变

**Scope:**
所有不涉及上述 8 个缺陷触发条件的输入路径应完全不受影响。

## Hypothesized Root Cause

1. **Bug 1.1 — 接口签名不兼容**: `stealth.EventDispatcher` 定义 `Dispatch(ctx, interface{})`，而 `events.EventDispatcher` 定义 `Dispatch(ctx, *ControlEvent)`。两个接口方法名相同但参数类型不同，Go 类型系统不允许隐式转换。

2. **Bug 1.2 — 变量作用域缺失**: `OnBanned` 回调在 `runDaemonMode` 函数中定义为闭包，但 `kr` 变量仅在 `provision.go` 的 `loadRuntimeConfig` 中创建，未在 `runDaemonMode` 作用域中定义。

3. **Bug 1.3 — Topology 响应结构不一致**: OS 端 `GatewayEntry` 使用 `{gateway_id, ip_address, cell_id, status}`，Client 端 `GatewayNode` 期望 `{ip, port, priority, region, cell_id}`。字段名和语义完全不同。

4. **Bug 1.4 — HMAC 序列化方式不一致**: OS 端用 `fmt.Sprintf("%d%s%s", version, publishedAt, gwJSON)` 拼接，Client 端用 `json.Marshal(hmacBody{...})` 序列化。两种方式产生不同的字节序列。

5. **Bug 1.5 — Heartbeat 列名分裂**: `registry.go` 遵循 GORM model 使用 `last_heartbeat_at`，但 `grpc/server.go` 和 `rest/handler.go` 使用 `last_heartbeat`。根据 ADR-001，应统一为 GORM model 的 `last_heartbeat_at`。

6. **Bug 1.6 — 主键列名分裂**: `registry.go` 使用 `gateway_id` 作为 UPSERT 冲突键（与 GORM model 一致），`grpc/server.go` 使用 `id`。根据 ADR-001，应统一为 `gateway_id`。

7. **Bug 1.7 — 路由未注册**: `provisioning.HTTPHandler.RegisterRoutes` 已实现，但 `bridge/main.go` 中只创建了 `rest.Handler` 并注册其路由，未创建 `provisioning.HTTPHandler` 并注册到同一 mux。

8. **Bug 1.8 — 测试路径过时**: 集成测试 mock server 注册 `/api/v1/topology`，但 OS 端已升级为 `/api/v2/topology`。

## Correctness Properties

Property 1: Bug Condition — 编译阻断修复

_For any_ Go build action targeting `mirage-gateway/cmd/gateway/` 或 `phantom-client/cmd/phantom/`，修复后的代码 SHALL 成功编译，不产生类型不兼容或未定义变量的编译错误。

**Validates: Requirements 2.1, 2.2**

Property 2: Bug Condition — Topology 契约一致性

_For any_ Client 端 topology 拉取请求，OS 端返回的 `RouteTableResponse` SHALL 包含与 Client 端 `GatewayNode` 结构一致的字段（`ip`, `port`, `priority`, `region`, `cell_id`），且 `Signature` 和 `PublishedAt` 类型两端一致，Client 能正确反序列化并通过 HMAC 验签。

**Validates: Requirements 2.3, 2.4**

Property 3: Bug Condition — SQL 列名一致性

_For any_ gateway-bridge 模块对 `gateways` 表的 INSERT/UPDATE/SELECT 操作，所有 SQL 语句 SHALL 使用与 GORM Gateway model 一致的列名（主键 `gateway_id`，心跳时间 `last_heartbeat_at`），不产生 SQL 列不存在错误。

**Validates: Requirements 2.5, 2.6**

Property 4: Bug Condition — Provisioning 路由可达性

_For any_ HTTP 请求到 `/internal/delivery/redeem`、`/internal/delivery/status/`、`/internal/provision`，gateway-bridge SHALL 正确路由到 provisioning handler 并返回预期响应（非 404）。

**Validates: Requirements 2.7**

Property 5: Bug Condition — 集成测试路径一致性

_For any_ Client 集成测试中的 topology API 请求，测试桩 SHALL 使用与 OS 端一致的路径 `/api/v2/topology`。

**Validates: Requirements 2.8**

Property 6: Preservation — 现有行为不变

_For any_ 不涉及上述 bug condition 的输入（V2 事件分发、Client 非 banned 运行、REST API 查询、provisioning 请求处理），修复后的代码 SHALL 产生与修复前完全相同的行为。

**Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7**

## Fix Implementation

### Changes Required

Assuming our root cause analysis is correct:


**Fix 1.1 — EventDispatcher 接口适配器**

**File**: `mirage-gateway/pkg/gtunnel/stealth/control_plane.go`

**Changes**:
1. 保持 `stealth.EventDispatcher` 接口定义不变（`Dispatch(ctx, interface{})`)
2. 在 `mirage-gateway/cmd/gateway/main.go` 中创建一个轻量适配器 struct，将 `events.EventDispatcher.Dispatch(ctx, *ControlEvent)` 包装为 `stealth.EventDispatcher.Dispatch(ctx, interface{})`
3. 适配器内部做类型断言 `event.(*events.ControlEvent)` 后调用真实 dispatcher

**Rationale**: 适配器模式避免修改 `stealth` 包或 `events` 包的接口定义，影响范围最小。

---

**Fix 1.2 — OnBanned 回调中补充 kr 变量**

**File**: `phantom-client/cmd/phantom/main.go`

**Function**: `runDaemonMode`

**Changes**:
1. 在 `OnBanned` 回调闭包之前，创建 `kr := persist.NewKeyring()` 变量
2. 确保 `kr` 在 `OnBanned` 闭包的词法作用域内可见

**Rationale**: `keyringService`、`keyringPSK`、`keyringAuthKey` 常量已在 `provision.go` 中定义且同包可见，只需补充 `kr` 实例。

---

**Fix 1.3 — OS 端 Topology 响应结构对齐 Client**

**File**: `mirage-os/services/topology/handler.go`

**Changes**:
1. 将 `GatewayEntry` 替换为与 Client 端 `GatewayNode` 一致的结构：`{ip, port, priority, region, cell_id}`
2. `ip` 从 GORM model 的 `IPAddress` 映射
3. `port` 默认 443（当前 Gateway 注册时未存储端口，使用默认值）
4. `priority` 默认 0（后续可从 Cell 配置中读取）
5. `region` 从 Cell 的 `RegionCode` 映射（当前可用 `CellID` 临时替代）
6. 将 `PublishedAt` 从 `string`（RFC3339）改为直接输出 `time.Time` 的 JSON 序列化（`time.Time` 默认 JSON 序列化即为 RFC3339，与 Client 端 `time.Time` 反序列化兼容）
7. 将 `Signature` 从 hex string 改为 `[]byte` 的 JSON 序列化（base64 编码，与 Client 端 `[]byte` 反序列化兼容）

---

**Fix 1.4 — OS 端 HMAC 签名算法对齐 Client**

**File**: `mirage-os/services/topology/handler.go`

**Changes**:
1. 将 HMAC 签名计算方式从 `fmt.Sprintf("%d%s%s", version, publishedAt, gwJSON)` 改为与 Client 端一致的 `json.Marshal(hmacBody{Gateways, Version, PublishedAt})`
2. 定义与 Client 端相同的 `hmacBody` struct（含 `gateways`、`version`、`published_at` 字段）
3. 签名结果直接存为 `[]byte`（不再 hex encode）

**Rationale**: 以 Client 端为权威，因为 Client 已有完整的 HMAC 验签和拓扑管理逻辑。

---

**Fix 1.5 — Heartbeat 列名统一为 `last_heartbeat_at`**

**Files**:
- `mirage-os/gateway-bridge/pkg/grpc/server.go` — `SyncHeartbeat` 方法
- `mirage-os/gateway-bridge/pkg/rest/handler.go` — `handleGateways` 方法

**Changes**:
1. `grpc/server.go`: 将 UPSERT SQL 中的 `last_heartbeat` 替换为 `last_heartbeat_at`
2. `rest/handler.go`: 将 SELECT SQL 中的 `last_heartbeat` 替换为 `last_heartbeat_at`

**Rationale**: GORM Gateway model 定义为 `LastHeartbeatAt *time.Time \`gorm:"index" json:"last_heartbeat_at"\``，根据 ADR-001 以 GORM model 为准。

---

**Fix 1.6 — 主键列名统一为 `gateway_id`**

**File**: `mirage-os/gateway-bridge/pkg/grpc/server.go`

**Changes**:
1. `SyncHeartbeat`: 将 `INSERT INTO gateways (id, ...)` 改为 `INSERT INTO gateways (gateway_id, ...)`
2. `SyncHeartbeat`: 将 `ON CONFLICT (id)` 改为 `ON CONFLICT (gateway_id)`
3. `markStaleGatewaySessions`: 确认 `SELECT id FROM gateways` 中的 `id` 是否应为 `gateway_id`（根据 GORM model，`id` 是自增主键，`gateway_id` 是业务唯一键；此处查询目的是获取 gateway 标识用于后续 UPDATE，应改为 `SELECT gateway_id`）
4. `handleGatewayAction` in `rest/handler.go`: 将 `WHERE id = $1` 改为 `WHERE gateway_id = $1`

---

**Fix 1.7 — Provisioning 路由挂载**

**File**: `mirage-os/gateway-bridge/cmd/bridge/main.go`

**Changes**:
1. 导入 `mirage-os/services/provisioning` 包
2. 在 REST server 启动前，创建 `provisioning.Provisioner` 实例（需要 GORM DB，当前 bridge 使用 `*sql.DB`，需要评估是否可以创建 GORM 实例或使用适配方案）
3. 创建 `provisioning.NewHTTPHandler(provisioner)` 并调用 `RegisterRoutes(mux)` 注册到同一 mux
4. 这样 provisioning 路由自动受到已有的 `InternalAuthMiddleware` 和 `AccessLogMiddleware` 保护

**Note**: `provisioning.NewProvisioner` 依赖 `*gorm.DB`，而 bridge 当前使用 `*sql.DB`。需要在 bridge 中额外初始化一个 GORM DB 实例（使用相同 DSN），或将 Provisioner 改为接受 `*sql.DB`。最小化修复方案：在 bridge 中创建 GORM DB wrapper。

---

**Fix 1.8 — 集成测试路径修正**

**File**: `phantom-client/pkg/gtclient/integration_test.go`

**Changes**:
1. 将 mock server 中 `case "/api/v1/topology":` 改为 `case "/api/v2/topology":`
2. 将 mock server 中 `case "/api/v1/entitlement":` 改为 `case "/api/v2/entitlement":`
3. 将 fetcher 中 `server.URL+"/api/v1/topology"` 改为 `server.URL+"/api/v2/topology"`
4. 将 fetcher 中 `server.URL+"/api/v1/entitlement"` 改为 `server.URL+"/api/v2/entitlement"`

## Testing Strategy

### Validation Approach

测试策略分两阶段：先在未修复代码上验证 bug 确实存在（探索性测试），再在修复后验证正确性和行为保持。

### Exploratory Bug Condition Checking

**Goal**: 在修复前确认 bug 存在，验证根因分析。

**Test Plan**: 对每个 bug 编写最小复现测试，在未修复代码上运行观察失败。

**Test Cases**:
1. **编译测试 1.1**: `cd mirage-gateway && go build ./cmd/gateway/` → 期望编译失败（will fail on unfixed code）
2. **编译测试 1.2**: `cd phantom-client && go build ./cmd/phantom/` → 期望编译失败（will fail on unfixed code）
3. **Topology 反序列化测试**: 构造 OS 端格式的 JSON，用 Client 端 `RouteTableResponse` 反序列化 → 期望字段丢失（will fail on unfixed code）
4. **HMAC 一致性测试**: 用相同数据分别按 OS 端和 Client 端方式计算 HMAC → 期望不匹配（will fail on unfixed code）
5. **SQL 列名测试**: 检查 `grpc/server.go` 和 `registry.go` 中的 SQL 列名是否一致 → 期望不一致（will fail on unfixed code）
6. **Provisioning 路由测试**: 启动 bridge 后请求 `/internal/delivery/redeem` → 期望 404（will fail on unfixed code）
7. **集成测试路径测试**: 检查测试代码中的 API 路径 → 期望与 OS 端不一致（will fail on unfixed code）

**Expected Counterexamples**:
- 编译器报错信息明确指出类型不兼容和未定义变量
- JSON 反序列化后 `GatewayNode.IP` 为空字符串、`Port` 为 0
- 两端 HMAC 值不同

### Fix Checking

**Goal**: 验证修复后所有 bug condition 输入产生正确行为。

**Pseudocode:**
```
FOR ALL input WHERE isBugCondition(input) DO
  result := fixedSystem(input)
  ASSERT expectedBehavior(result)
END FOR
```

### Preservation Checking

**Goal**: 验证修复后所有非 bug condition 输入行为不变。

**Pseudocode:**
```
FOR ALL input WHERE NOT isBugCondition(input) DO
  ASSERT originalSystem(input) = fixedSystem(input)
END FOR
```

**Testing Approach**: 属性基测试（PBT）适用于 Topology 契约和 SQL 列名一致性验证，因为可以生成大量随机 gateway 数据验证序列化/反序列化往返一致性和 HMAC 匹配。

**Test Cases**:
1. **EventDispatcher 保持**: 验证 V2 事件分发流程（去重、epoch、handler 路由）在适配器引入后行为不变
2. **Client 运行时保持**: 验证 phantom-client 非 banned 路径功能不变
3. **REST API 保持**: 验证 gateway 列表查询返回完整字段
4. **Provisioning 保持**: 验证 redeem/status/provision 请求处理逻辑不变
5. **中间件保持**: 验证新挂载的 provisioning 路由受 InternalAuthMiddleware 保护

### Unit Tests

- 编译验证：`go build` 各模块成功
- Topology 序列化往返测试：OS 端序列化 → Client 端反序列化 → 字段完整
- HMAC 一致性测试：同一数据两端计算结果相同
- SQL 列名静态检查：所有 raw SQL 中 gateways 表列名与 GORM model 一致
- Provisioning 路由注册测试：mux 包含 `/internal/delivery/*` 路由

### Property-Based Tests

- 生成随机 GatewayNode 列表 → OS 端序列化 → Client 端反序列化 → 验证往返一致
- 生成随机 RouteTableResponse → 两端 HMAC 计算 → 验证结果相同
- 生成随机 gateway 状态 → 通过 registry 和 grpc server 写入 → 验证 SQL 不报错

### Integration Tests

- 端到端 Topology 拉取：OS 端返回 → Client 端解析 + 验签 → 成功
- Provisioning 端到端：bridge 启动 → 请求 `/internal/delivery/redeem` → 正确响应
- 集成测试路径一致性：运行 `integration_test.go` → 全部通过
