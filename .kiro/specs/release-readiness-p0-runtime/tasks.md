# Implementation Plan

## Phase 1: Bug Condition Exploration Tests（修复前验证 Bug 存在）

- [x] 1. Write bug condition exploration tests — 编译阻断 + 运行时契约验证
  - **Property 1: Bug Condition** — P0 Runtime Blockers Exist
  - **CRITICAL**: This test MUST FAIL on unfixed code — failure confirms the bugs exist
  - **DO NOT attempt to fix the test or the code when it fails**
  - **NOTE**: This test encodes the expected behavior — it will validate the fix when it passes after implementation
  - **GOAL**: Surface counterexamples that demonstrate the 8 P0 bugs exist
  - **Scoped PBT Approach**: 针对确定性编译/运行时 bug，scope 到具体失败用例
  - Test 1.1: `cd mirage-gateway && go build ./cmd/gateway/` → 期望编译失败（`cannot use v2Dispatcher as stealth.EventDispatcher`）
  - Test 1.2: `cd phantom-client && go build ./cmd/phantom/` → 期望编译失败（`undefined: kr`）
  - Test 1.3: 构造 OS 端 `GatewayEntry{gateway_id, ip_address, cell_id, status}` JSON → 用 Client 端 `GatewayNode` 反序列化 → 期望 `IP==""`, `Port==0`（字段丢失）
  - Test 1.4: 用相同数据分别按 OS 端 `fmt.Sprintf` 和 Client 端 `json.Marshal(hmacBody{})` 计算 HMAC → 期望不匹配
  - Test 1.5: 静态检查 `grpc/server.go` SQL 中 `last_heartbeat` vs `registry.go` 中 `last_heartbeat_at` → 期望不一致
  - Test 1.6: 静态检查 `grpc/server.go` SQL 中 `ON CONFLICT (id)` vs `registry.go` 中 `ON CONFLICT (gateway_id)` → 期望不一致
  - Test 1.7: 检查 `bridge/main.go` 中是否注册了 `/internal/delivery/redeem` 路由 → 期望未注册
  - Test 1.8: 检查 `integration_test.go` 中 API 路径 `/api/v1/topology` vs OS 端 `/api/v2/topology` → 期望不一致
  - Run tests on UNFIXED code
  - **EXPECTED OUTCOME**: Tests FAIL（confirms all 8 bugs exist）
  - Document counterexamples found to understand root cause
  - Mark task complete when tests are written, run, and failure is documented
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8_

## Phase 2: Preservation Property Tests（修复前验证现有行为基线）

- [x] 2. Write preservation property tests (BEFORE implementing fix)
  - **Property 2: Preservation** — 现有行为不变性验证
  - **IMPORTANT**: Follow observation-first methodology
  - Observe: V2 EventDispatcher 分发流程（去重、epoch 校验、handler 路由）在未修复代码上正常工作
  - Observe: phantom-client 非 banned 路径功能（bootstrap pool、RuntimeTopo、degradation tracking）正常
  - Observe: Client 端 `TopoVerifier` HMAC 验签、版本单调递增、PublishedAt 反回滚逻辑正常
  - Observe: `registry.Register` 使用 `gateway_id` + `last_heartbeat_at` 的 SQL 正常执行
  - Observe: REST handler `handleGateways` 返回完整 gateway 列表 JSON 结构
  - Observe: provisioning handler 的 redeem/status/provision 请求处理逻辑正常
  - Write property-based tests:
    - PBT: 生成随机 `GatewayNode` 列表 → Client 端序列化 → 反序列化 → 验证往返一致（preservation of Client-side serialization）
    - PBT: 生成随机 `RouteTableResponse` → Client 端 `ComputeHMAC` → 验证 HMAC 确定性（preservation of HMAC computation）
    - PBT: 生成随机 `ControlEvent` → V2 EventDispatcher 分发 → 验证去重和 handler 路由行为不变
  - Verify tests pass on UNFIXED code
  - **EXPECTED OUTCOME**: Tests PASS（confirms baseline behavior to preserve）
  - Mark task complete when tests are written, run, and passing on unfixed code
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_

## Phase 3: Implementation — 编译阻断修复（Fix 1.1, 1.2）

- [x] 3. Fix 1.1 — EventDispatcher 接口适配器
  - [x] 3.1 在 `mirage-gateway/cmd/gateway/main.go` 中创建 `stealthDispatcherAdapter` struct
    - 实现 `stealth.EventDispatcher` 接口：`Dispatch(ctx context.Context, event interface{}) error`
    - 内部做类型断言 `event.(*events.ControlEvent)` 后调用 `events.EventDispatcher.Dispatch(ctx, ce)`
    - _Bug_Condition: `stealth.EventDispatcher.Dispatch(ctx, interface{})` ≠ `events.EventDispatcher.Dispatch(ctx, *ControlEvent)`_
    - _Expected_Behavior: 适配器桥接两个接口，`go build ./cmd/gateway/` 成功_
    - _Preservation: V2 EventDispatcher 完整分发流程（去重、epoch、handler 路由）不变_
    - _Requirements: 2.1, 3.1_
  - [x] 3.2 将 `StealthControlPlaneOpts{Dispatcher: v2Dispatcher}` 改为 `StealthControlPlaneOpts{Dispatcher: &stealthDispatcherAdapter{v2Dispatcher}}`
    - _Requirements: 2.1_
  - [x] 3.3 验证 `cd mirage-gateway && go build ./cmd/gateway/` 编译成功
    - _Requirements: 2.1_

- [x] 4. Fix 1.2 — OnBanned 回调中补充 kr 变量
  - [x] 4.1 在 `phantom-client/cmd/phantom/main.go` 的 `runDaemonMode` 函数中，`OnBanned` 闭包之前创建 `kr := persist.NewKeyring()`
    - _Bug_Condition: `OnBanned` 回调引用 `kr` 但 `kr` 未在 `runDaemonMode` 作用域中定义_
    - _Expected_Behavior: `kr` 在闭包词法作用域内可见，`go build ./cmd/phantom/` 成功_
    - _Preservation: phantom-client 非 banned 路径功能不变_
    - _Requirements: 2.2, 3.2_
  - [x] 4.2 验证 `cd phantom-client && go build ./cmd/phantom/` 编译成功
    - _Requirements: 2.2_

## Phase 4: Implementation — Topology 契约修复（Fix 1.3, 1.4）

- [x] 5. Fix 1.3 — OS 端 Topology 响应结构对齐 Client
  - [x] 5.1 在 `mirage-os/services/topology/handler.go` 中将 `GatewayEntry` 替换为与 Client 端 `GatewayNode` 一致的结构
    - 字段映射：`ip` ← `gw.IPAddress`，`port` ← 443（默认），`priority` ← 0（默认），`region` ← `gw.CellID`，`cell_id` ← `gw.CellID`
    - `PublishedAt` 从 `string` 改为 `time.Time`（JSON 序列化自动为 RFC3339，与 Client 端 `time.Time` 兼容）
    - `Signature` 从 hex string 改为 `[]byte`（JSON 序列化为 base64，与 Client 端 `[]byte` 兼容）
    - _Bug_Condition: OS 端 `GatewayEntry{gateway_id, ip_address, cell_id, status}` ≠ Client 端 `GatewayNode{ip, port, priority, region, cell_id}`_
    - _Expected_Behavior: Client 端 `json.Unmarshal` 后 `GatewayNode.IP` 非空、`Port` 非零_
    - _Preservation: OS 端 ETag 支持、错误处理不变_
    - _Requirements: 2.3, 3.3_

- [x] 6. Fix 1.4 — OS 端 HMAC 签名算法对齐 Client
  - [x] 6.1 在 `mirage-os/services/topology/handler.go` 中定义 `hmacBody` struct（与 Client 端 `topo.go` 中的 `hmacBody` 一致）
    - 字段：`Gateways []GatewayNode`、`Version uint64`、`PublishedAt time.Time`（JSON tag 对齐）
    - _Requirements: 2.4_
  - [x] 6.2 将 HMAC 计算从 `fmt.Sprintf("%d%s%s", version, publishedAt, gwJSON)` 改为 `json.Marshal(hmacBody{...})`
    - 签名结果直接存为 `[]byte`（不再 hex encode）
    - _Bug_Condition: OS 端 `sprintf_concat` ≠ Client 端 `json_marshal`_
    - _Expected_Behavior: 同一数据两端 HMAC 计算结果相同_
    - _Preservation: Client 端 `ComputeHMAC` 逻辑不变_
    - _Requirements: 2.4, 3.3_
  - [x] 6.3 编写 OS→Client 序列化往返测试 + HMAC 一致性测试
    - PBT: 生成随机 GatewayNode 列表 → OS 端序列化 → Client 端反序列化 → 验证字段完整
    - PBT: 生成随机 RouteTableResponse → 两端 HMAC 计算 → 验证结果相同
    - _Requirements: 2.3, 2.4_

## Phase 5: Implementation — SQL 列名修复（Fix 1.5, 1.6）

- [x] 7. Fix 1.5 — Heartbeat 列名统一为 `last_heartbeat_at`
  - [x] 7.1 `mirage-os/gateway-bridge/pkg/grpc/server.go` `SyncHeartbeat` 方法：`last_heartbeat` → `last_heartbeat_at`
    - _Bug_Condition: `grpc/server.go` 写 `last_heartbeat`，`registry.go` 写 `last_heartbeat_at`_
    - _Expected_Behavior: 所有模块统一使用 GORM model 的 `last_heartbeat_at`_
    - _Preservation: 心跳处理中除列名外的所有字段写入不变_
    - _Requirements: 2.5, 3.4_
  - [x] 7.2 `mirage-os/gateway-bridge/pkg/rest/handler.go` `handleGateways` 方法：`last_heartbeat` → `last_heartbeat_at`
    - _Requirements: 2.5, 3.5_

- [x] 8. Fix 1.6 — 主键列名统一为 `gateway_id`
  - [x] 8.1 `grpc/server.go` `SyncHeartbeat`：`INSERT INTO gateways (id, ...)` → `INSERT INTO gateways (gateway_id, ...)`，`ON CONFLICT (id)` → `ON CONFLICT (gateway_id)`
    - _Bug_Condition: `grpc/server.go` 用 `id`，`registry.go` 用 `gateway_id`_
    - _Expected_Behavior: 所有模块统一使用 GORM model 的 `gateway_id` 作为业务唯一键_
    - _Preservation: UPSERT 语义不变_
    - _Requirements: 2.6, 3.4_
  - [x] 8.2 `grpc/server.go` `markStaleGatewaySessions`：`SELECT id FROM gateways` → `SELECT gateway_id FROM gateways`
    - _Requirements: 2.6_
  - [x] 8.3 `rest/handler.go` `handleGatewayAction`：`WHERE id = $1` → `WHERE gateway_id = $1`
    - _Requirements: 2.6, 3.5_
  - [x] 8.4 同步修正 GORM model 中 `SyncHeartbeat` 写入的其他列名（`threat_level` → `current_threat_level`，`memory_usage_mb` → `memory_bytes`，`ebpf_loaded` 保持）
    - 参照 GORM Gateway model：`CurrentThreatLevel`、`MemoryBytes`、`ActiveConnections`
    - _Requirements: 2.5, 2.6, 3.4_

## Phase 6: Implementation — Provisioning 路由挂载（Fix 1.7）

- [x] 9. Fix 1.7 — Provisioning 路由注册到 bridge HTTP mux
  - [x] 9.1 在 `mirage-os/gateway-bridge/cmd/bridge/main.go` 中导入 `mirage-os/services/provisioning` 包
    - _Requirements: 2.7_
  - [x] 9.2 在 REST server 启动前，创建 GORM DB 实例（使用相同 DSN）并初始化 `provisioning.Provisioner` + `provisioning.HTTPHandler`
    - _Bug_Condition: `provisioning.HTTPHandler.RegisterRoutes` 已实现但未被调用_
    - _Expected_Behavior: `/internal/delivery/redeem`、`/internal/delivery/status/`、`/internal/provision` 路由可达_
    - _Preservation: REST API 的 InternalAuthMiddleware 和 AccessLogMiddleware 对所有路由的保护不变_
    - _Requirements: 2.7, 3.6, 3.7_
  - [x] 9.3 调用 `provisioningHandler.RegisterRoutes(mux)` 注册到同一 mux
    - _Requirements: 2.7_

## Phase 7: Implementation — 集成测试路径修正（Fix 1.8）

- [x] 10. Fix 1.8 — 集成测试 API 路径对齐 OS 端
  - [x] 10.1 `phantom-client/pkg/gtclient/integration_test.go`：`case "/api/v1/topology":` → `case "/api/v2/topology":`
    - _Bug_Condition: 测试桩路径 `/api/v1/topology` ≠ OS 端注册路径 `/api/v2/topology`_
    - _Expected_Behavior: 测试桩与 OS 端路径一致_
    - _Requirements: 2.8_
  - [x] 10.2 `integration_test.go`：`case "/api/v1/entitlement":` → `case "/api/v2/entitlement":`
    - _Requirements: 2.8_
  - [x] 10.3 `integration_test.go`：fetcher 中 `server.URL+"/api/v1/topology"` → `server.URL+"/api/v2/topology"`
    - _Requirements: 2.8_
  - [x] 10.4 `integration_test.go`：fetcher 中 `server.URL+"/api/v1/entitlement"` → `server.URL+"/api/v2/entitlement"`
    - _Requirements: 2.8_

## Phase 8: Verification — 修复后验证

- [x] 11. Verify bug condition exploration tests now pass
  - [x] 11.1 Re-run exploration tests from task 1
    - **Property 1: Expected Behavior** — P0 Runtime Blockers Fixed
    - **IMPORTANT**: Re-run the SAME tests from task 1 — do NOT write new tests
    - The tests from task 1 encode the expected behavior
    - When these tests pass, it confirms the expected behavior is satisfied
    - **EXPECTED OUTCOME**: Tests PASS（confirms all 8 bugs are fixed）
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8_

- [x] 12. Verify preservation tests still pass
  - [x] 12.1 Re-run preservation tests from task 2
    - **Property 2: Preservation** — 现有行为不变性验证
    - **IMPORTANT**: Re-run the SAME tests from task 2 — do NOT write new tests
    - **EXPECTED OUTCOME**: Tests PASS（confirms no regressions）
    - Confirm all preservation tests still pass after fix
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_

## Phase 9: Checkpoint — 端到端验证

- [x] 13. Checkpoint — 全量编译 + 测试通过
  - `cd mirage-gateway && go build ./cmd/gateway/` 成功
  - `cd phantom-client && go build ./cmd/phantom/` 成功
  - `cd mirage-os && go build ./...` 成功
  - `cd phantom-client && go test ./pkg/gtclient/ -run TestIntegration` 通过
  - OS topology 响应能通过 Client `TopoVerifier.Verify()` 验签
  - gateway-bridge heartbeat SQL 列名与 GORM model 一致
  - `/internal/delivery/redeem` 路由可达（非 404）
  - Ensure all tests pass, ask the user if questions arise.
