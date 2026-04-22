# 任务清单：Mirage V1-V2 升级审计整改（代码审计增强版）

## 代码审计摘要

基于对仓库代码的逐文件审计，确认以下关键现状：

- **T01**：`L1Stats` 在 `types.go:137` 和 `manager.go:369` 重复定义，**编译阻断已确认**
- **T03**：V2 组件代码完整存在（orchestrator 7 个子模块 + stealth 4 个文件），但 `main.go` 中**零实例化**（grep 零匹配）
- **T04**：`PersistConfig` 有 `OSEndpoint` 字段，但 `provision.go` 的 `RunProvisioning()` **未写入** `OSEndpoint`（第 97 行构造 persistCfg 时缺失）
- **T05**：api-gateway `main.go` 中**未挂载**任何 query handler（仅挂载了 gRPC + provisioner + console）
- **T06**：`asnEntries` 在 `main.go:270` 初始化为空 slice `[]ebpf.ASNBlockEntry{}`，永远不进入 `SyncASNBlocklist`
- **T07**：`ReceiveLoop` 只消费 Scheme A，不消费 Scheme B；无通道时 `default` 分支 continue 热循环；`DrainQueue` 标注 "for testing"
- **T08**：OS 侧无 topology/entitlement handler，Client `topo.go` 指向不存在的 `/api/v1/topology`
- **T09**：`probe()` 构造 `QUICEngine` 时**不传递** `PinnedCertHash`（grep 零匹配）；`NICDetector` 创建后赋值给 `_ = detector` 未注入

---

## Phase 1：阻断修复

### T01：修复 Gateway 构建阻断
- [x] 1.1 删除 `mirage-gateway/pkg/ebpf/manager.go` 第 368-376 行重复的 `L1Stats` 结构体定义
  - 保留 `types.go:137` 中的权威版本（含 7 个字段：ASNDrops, RateDrops, SilentDrops, BlacklistDrops, SanityDrops, ProfileDrops, TotalChecked）
  - `manager.go` 中的版本也是 7 个字段但注释不同，删除即可
  - 确认 `manager.go` 的 `ReadL1Stats()` 方法（约第 380 行）引用的是 `types.go` 中的定义
- [x] 1.2 全局搜索 `ebpf.L1Stats` 引用点，确认无字段签名漂移
  - 已知引用点：`manager.go:ReadL1Stats()`、`cmd/gateway/main.go`（通过 `applier.ReadL1Stats()`）
- [x] 1.3 新增 `mirage-gateway/pkg/ebpf/compile_test.go`
  - 包含空测试函数 + `var _ L1Stats` 类型引用断言
  - 确保包可编译且无同名类型冲突
- [x] 1.4 验证 `go build ./...` 和 `go vet ./...` 在 `mirage-gateway` 下通过

### T02：统一 Mirage OS 数据库真相
- [x] 2.1 新增 `docs/adr/001-database-truth.md`
  - 声明 `mirage-os/pkg/models/db.go` 为运行时数据库真相
  - 已确认 db.go 包含完整模型：User, Cell, Gateway, BillingLog, ThreatIntel, Deposit, QuotaPurchase, Invitation, AuthChallenge + V2 模型（V2LinkState, V2SessionState, V2ControlState, V2PersonaState, V2CommitLog, V2BudgetEntry, V2AuditRecord）
- [x] 2.2 对齐 `users` 表
  - GORM `User` 主键为 `UserID string`（`gorm:"primaryKey;column:user_id"`）
  - Prisma `User` 主键为 `id String @id @default(uuid())`
  - 需确认两侧是否指向同一列，或需要迁移脚本对齐
- [x] 2.3 对齐 `gateways` 表
  - GORM `Gateway` 主键为 `GatewayID string`（`gorm:"primaryKey;column:gateway_id"`）
  - Prisma `Gateway` 主键为 `id String @id`
  - `gateway-bridge/pkg/topology/registry.go` 的 `Register()` SQL 使用 `$1` 作为 `id` 列
  - 需确认 GORM 的 `gateway_id` 列名与 bridge 的 `id` 列名是否一致
- [x] 2.4 重写 `registry.go` 中 `Register()` 的 upsert SQL
  - 当前 SQL 使用 `INSERT INTO gateways (id, status, ...)` 
  - 需匹配 GORM 的列名（`gateway_id` vs `id`）
  - 同时检查 `SyncHeartbeat` 中的 upsert SQL（`server.go` 约第 100 行）
- [x] 2.5 对齐 `billing_logs` 表
  - GORM `BillingLog` 有 `LogType string` 字段
  - Prisma `BillingLog` 无 `logType` 字段但有 `businessBytes/defenseBytes/businessCost/defenseCost/totalCost`
  - 需确认是否为同一张表的不同视图，或需要合并字段
- [x] 2.6 在 `schema.prisma` 头部添加注释：`// ⚠️ 适配层：运行时数据库真相为 mirage-os/pkg/models/db.go`
- [x] 2.7 新增 `mirage-os/scripts/migrate-unified-schema.sql`
  - 处理主键列名对齐（`user_id` vs `id`）
  - 处理缺失字段补充
  - 处理类型不一致（如 Decimal vs float64）

---

## Phase 2：主链路打通（可并行）

### T03：Gateway 接入 V2 编排链路
- [x] 3.1 在 `mirage-gateway/cmd/gateway/main.go` 中新增 V2 组件初始化块
  - 当前 main.go 约 450 行，V2 组件**完全未实例化**
  - 需要在步骤 12（TPROXY）之后、步骤 13（gRPC 客户端）之前插入
  - 创建顺序（按依赖）：
    1. `events.NewDispatcher()` — 事件分发器
    2. `commit.NewCommitEngine(controlMgr, executor, cooldownMgr, conflictMgr, store)` — 需要实现 ControlStateManager、PhaseExecutor、CooldownManager、ConflictManager、TxStore 的具体实例
    3. `transport.NewTransportFabric(...)` — 需要 TransportScorer
    4. `survival.NewSurvivalOrchestrator(...)` — 需要 AdmissionChecker、ConstraintChecker、ModePolicy、TriggerEvaluator
    5. `stealth.NewStealthControlPlane(opts)` — 需要 ShadowStreamMux、StegoEncoder/Decoder、EventDispatcher
  - **关键依赖**：每个组件都需要具体的 Store/Manager 实现，需要确认 `*_impl.go` 文件中的构造函数签名
- [x] 3.2 新增 `mirage-gateway/pkg/api/v2_adapter.go`
  - 实现 `V2CommandAdapter` 结构体
  - `AdaptPushStrategy(req *pb.StrategyPush) *pb.ControlCommand` — 将 defense_level/jitter/noise 映射为 V2 ControlCommand
  - `AdaptPushQuota(req *pb.QuotaPush) *pb.ControlCommand` — 将 remaining_bytes/user_id 映射为 V2 ControlCommand
  - `AdaptPushBlacklist(req *pb.BlacklistPush) *pb.ControlCommand` — 将 entries 映射为 V2 ControlCommand
  - `AdaptPushReincarnation(req *pb.ReincarnationPush) *pb.ControlCommand` — 将 domain/ip 映射为 V2 ControlCommand
- [x] 3.3 修改 `handlers.go` 中的 `PushStrategy/PushQuota/PushBlacklist/PushReincarnation`
  - 在现有逻辑前增加 V2 适配层调用
  - 如果 V2 adapter 非 nil，优先走 V2 编排链路
  - 保留 legacy fallback 作为降级路径
- [x] 3.4 确认启动日志输出 V2 组件创建成功信息
  - 格式：`✅ V2 EventDispatcher 已创建`、`✅ V2 CommitEngine 已创建`、...
- [x] 3.5 新增端到端测试 `mirage-gateway/tests/v2_e2e_test.go`
  - 覆盖 PushStrategy 命令从 gRPC 入口 → V2 adapter → EventDispatcher → CommitEngine 的闭环

### T04：统一 Client 配置契约
- [x] 4.1 新增 `phantom-client/pkg/persist/contract.go`
  - 以文档注释定义配置契约：
    - 来自 token：`BootstrapPool`, `CertFingerprint`, `UserID`
    - 来自 provisioning 推导：`OSEndpoint`（从 token 的 gateway endpoint 推导或从 delivery response 获取）
    - 运行时缓存：`LastEntitlement`
- [x] 4.2 修复 `provision.go` 第 97 行的 `persistCfg` 构造
  - **当前代码缺陷**：构造 `PersistConfig` 时只写入了 `BootstrapPool`、`CertFingerprint`、`UserID`，**遗漏了 `OSEndpoint`**
  - 修复方案：从 `bootstrapCfg` 中推导 `OSEndpoint`（如果 token 包含），或从 delivery response 的 endpoint 推导
  - 如果 token 不包含 os_endpoint，从 bootstrap pool 的第一个 gateway 地址推导（`https://<ip>:<port>`）
- [x] 4.3 在 `main.go` 中提取公共函数 `loadRuntimeConfig(configDir string) (*persist.PersistConfig, []byte, []byte, error)`
  - 当前 `runDaemonMode`（约第 100 行）和 `runForegroundWithConfig`（约第 260 行）各自独立装载配置
  - 统一为一个函数：加载 PersistConfig → 从 Keyring 加载 PSK/AuthKey → 校验必填字段 → 返回
- [x] 4.4 在 `loadRuntimeConfig` 中增加 `OSEndpoint` 缺失检查
  - 输出明确错误：`"os_endpoint missing in config.json — re-run provisioning with latest token"`
  - 同时检查 `BootstrapPool` 非空、`UserID` 非空
- [x] 4.5 验证 `TopoRefresher` 和 `EntitlementManager` 使用 `persistCfg.OSEndpoint`
  - 当前 `createTopoFetcher` 和 `createEntitlementFetcher` 接收 `osEndpoint string` 参数
  - 需确认 daemon 模式下该参数来自 `persistCfg.OSEndpoint`

### T05：暴露 OS V2 查询面
- [x] 5.1 在 `mirage-os/services/api-gateway/main.go` 中新增 HTTP mux 挂载块
  - 当前 main.go 只有 gRPC server（第 80 行）+ provisioner HTTP（第 100 行）+ console（第 120 行）
  - 需要在 provisioner HTTP 之后新增一个 query HTTP mux
  - 监听独立端口（如 `:8080`）或复用 provisioner 的 mux
- [x] 5.2 检查并修复 query handler 路由冲突
  - 需要读取 `state-query/handler.go` 和 `persona-query/handler.go` 确认各自注册的路由前缀
  - 如果都注册 `/api/v2/sessions/`，修改 persona-query 为 `/api/v2/sessions/{id}/persona`
- [x] 5.3 确认 state-query 为 `/api/v2/sessions/` 路由的唯一 owner
- [x] 5.4 确认四个 query handler 使用 `database.GetDB()` 访问 GORM 数据模型
  - 需要检查每个 handler 的构造函数是否接收 `*gorm.DB` 参数
- [x] 5.5 新增集成测试
  - 验证 api-gateway 启动不 panic
  - 验证 `/api/v2/links`、`/api/v2/sessions/`、`/api/v2/transactions`、`/api/v2/audit/records` 返回 2xx

---

## Phase 3：能力生效（可并行）

### T06：L1 清洗命中数据面
- [x] 6.1 修复 `cmd/gateway/main.go` 第 270 行的死代码
  - **当前代码**：`asnEntries := []ebpf.ASNBlockEntry{}` 初始化为空 slice
  - **修复**：从 `intelProvider` 获取真实 ASN entries
  - 需要确认 `ThreatIntelProvider` 是否有 `GetASNEntries() []ebpf.ASNBlockEntry` 方法
  - 如果没有，需要新增转换函数：将 `asn_database.json` 中的 CIDR+ASN 数据转换为 `[]ebpf.ASNBlockEntry`
- [x] 6.2 确认 `configs/asn_database.json` 数据格式
  - 已确认包含 16 个云厂商 CIDR 段（AWS, Azure, GCP, Aliyun, Tencent, Cloudflare 等）
  - 需要确认 JSON 结构是否可直接映射为 `ASNBlockEntry{CIDR, ASN}`
- [x] 6.3 在数据面入口增加 `LookupASN`/`IsCloudIP` 运行时调用点
  - 候选位置：`pkg/threat/l1_monitor.go` 的事件处理回调中
  - 或在 `pkg/api/handlers.go` 的 `ReportThreat` 中增加 ASN 检查
- [x] 6.4 空 entries 时记录 WARN 日志：`"⚠️ ASN 黑名单为空，L1 清洗未生效"`
- [x] 6.5 新增集成测试，验证命中 ASN 规则的 IP 被下发到 eBPF map

### T07：修复 Stealth Control Plane 可靠性
- [x] 7.1 重写 `control_plane.go` 的 `ReceiveLoop()`（当前约第 100 行）
  - **当前问题**：
    - 只消费 Scheme A（`p.mux.ReadCommand()`），不消费 Scheme B（`p.decoder`）
    - 无通道时 `default` 分支直接 `continue`，形成热循环
  - **修复方案**：
    ```
    select {
    case <-ctx.Done(): return
    case cmd := <-schemeACh:  // mux.ReadCommand 包装为 channel
    case cmd := <-schemeBCh:  // decoder.Decode 包装为 channel
    case <-time.After(backoff): // 无通道时退避
    }
    ```
  - 需要将 `mux.ReadCommand()` 和 `decoder` 的阻塞调用包装为 goroutine + channel
- [x] 7.2 新增 `drainOnRecovery()` 方法
  - 当通道从 `ChannelQueued` 恢复到 `ChannelSchemeA/B` 时自动调用
  - 遍历 `cmdQueue`，检查每条命令的时间戳，丢弃超过 TTL（默认 60s）的命令
  - 未超时的命令通过恢复的通道重新发送
  - 当前 `DrainQueue()` 标注 "for testing"，需要改为正式方法
- [x] 7.3 在 `SetSchemeAAvailable()` 和 `updateState()` 中触发 `drainOnRecovery()`
  - 当状态从 `ChannelQueued` 变为 `ChannelSchemeA` 或 `ChannelSchemeB` 时触发
- [x] 7.4 新增测试覆盖三场景
  - 场景 1：双通道断开 → 命令进入 cmdQueue → 验证 QueueLen() 递增
  - 场景 2：单通道恢复 → drainOnRecovery 触发 → 验证队列被清空
  - 场景 3：恢复后回放 → 超时命令被丢弃 → 未超时命令被重发

### T08：对齐 topology/entitlement API 契约
- [x] 8.1 新增 `docs/api/topology-contract.md`
  - URL: `GET /api/v2/topology`
  - 鉴权: `Authorization: Bearer <base64(authKey)>` + `X-Client-ID: <userID>`
  - 响应结构: `RouteTableResponse{Version, PublishedAt, Gateways[], Signature}`
  - 签名: HMAC-SHA256(PSK, Version + PublishedAt + Gateways JSON)
- [x] 8.2 新增 `docs/api/entitlement-contract.md`
  - URL: `GET /api/v2/entitlement`
  - 鉴权: 同上
  - 响应结构: `Entitlement{ServiceClass, ExpiresAt, QuotaRemaining, Banned, FetchedAt}`
- [x] 8.3 新增 `mirage-os/services/topology/handler.go`
  - 实现 `GET /api/v2/topology`
  - 从 GORM 查询 `gateways` 表（status=ONLINE）
  - 用 OS 私钥签名响应
  - 支持 `If-None-Match` / `304 Not Modified`
- [x] 8.4 新增 `mirage-os/services/entitlement/handler.go`
  - 实现 `GET /api/v2/entitlement`
  - 从 GORM 查询用户配额、订阅状态、封禁状态
- [x] 8.5 在 `api-gateway/main.go` 中挂载 topology 和 entitlement handler
  - 与 T05 的 query mux 合并
- [x] 8.6 修改 `phantom-client/pkg/gtclient/topo.go`
  - `createTopoFetcher` 中 URL 从 `/api/v1/topology` 改为 `/api/v2/topology`
  - 同步修改 `phantom-client/cmd/phantom/main.go` 中的 `createEntitlementFetcher` URL
- [x] 8.7 新增集成测试
  - 验证 OS 返回的 topology 响应通过 `TopoVerifier.Verify()` 验签
  - 验证 `version` 单调递增

### T09：建链参数进入真实路径
- [x] 9.1 修改 `phantom-client/pkg/gtclient/client.go` 中 `probe()` 函数（约第 200 行）
  - **当前代码**：`engine := NewQUICEngine(&QUICEngineConfig{GatewayAddr: addr})` — 只传了地址，**未传 PinnedCertHash**
  - **修复**：从 `c.config.CertFingerprint` 解码为 `[]byte`，传递给 `QUICEngineConfig.PinnedCertHash`
  - 需要在 `GTunnelClient` 中存储解码后的 `pinnedCertHash []byte`，在 `NewGTunnelClient` 中初始化
- [x] 9.2 确认 `quic_engine.go` 的 TLS 验证逻辑
  - 当前代码（约第 60 行）：`PinnedCertHash` 长度为 32 时启用 `VerifyConnection` 回调
  - 逻辑正确，只需确保 `probe()` 传递了正确的 hash
- [x] 9.3 修复 `main.go` 中 `NICDetector` 注入
  - **当前代码**（约第 183 行）：`detector := nicdetect.NewDetector()` 然后 `_ = detector` — 创建了但**未注入**
  - **修复**：需要在 `GTunnelClient` 上增加 `SetNICDetector(d NICDetector)` 方法
  - 或在 `NewGTunnelClient` 的 config 中增加 `NICDetector` 字段
  - 然后在 `probe()` 中传递给 `QUICEngineConfig.NICDetector`
- [x] 9.4 实现 `PullRouteTable()`
  - **当前代码**：`func (c *GTunnelClient) PullRouteTable(ctx context.Context) error { return nil }` — 空实现
  - **修复**：调用 `createTopoFetcher` 返回的 fetcher 函数，获取 `RouteTableResponse`
  - 验签后更新 `RuntimeTopology`
  - 需要在 `GTunnelClient` 中注入 fetcher 函数或 OS endpoint
- [x] 9.5 调整启动序列
  - 当前：`bootstrapWithRetry → PullRouteTable（空）→ 双向转发`
  - 修改为：`LoadConfig → PullRouteTable（真实拉取）→ bootstrapWithRetry → 双向转发`
  - 如果 PullRouteTable 成功，用返回的节点列表替代 bootstrap pool 进行 probe
- [x] 9.6 定义退化行为
  - fingerprint 不匹配 → 拒绝连接，返回 `"certificate pin mismatch"` 错误
  - NIC 检测失败 → 使用 `legacyDetectOutbound()` 作为 fallback，记录 WARN
  - 拓扑同步失败 → 重试 3 次（间隔 5s），仍失败则使用 bootstrap pool，记录 WARN

---

## Phase 4：收口

### T10：修复或降级 CTLock
- [x] 10.1 降级 `ctlock.go` 为普通 `sync.Mutex` 包装 + 最小持锁时间语义
  - 当前实现使用 `time.Now()` busy-wait + `FakeCryptoWork`
  - 降级方案：保留 `Lock()/Unlock()` 接口，内部用 `sync.Mutex` + `time.Sleep(minDuration)` 替代 busy-wait
  - 重命名为 `TimedLock`（可选）
- [x] 10.2 修复 property tests
  - 当前测试验证 "持锁时间 = constDuration"（已知失败）
  - 改为验证 "持锁时间 ≥ minDuration"
- [x] 10.3 删除文档和代码注释中的 constant-time 安全声明
  - 搜索 `constant.time`、`恒定时间`、`timing` 相关注释
- [x] 10.4 验证 `go test ./pkg/gtunnel/ctlock/...` 稳定通过

---

## 检查点

- [x] Phase 1 检查点：`mirage-gateway` 可编译，数据库 ADR 已发布
- [x] Phase 2 检查点：V2 组件在 Gateway 启动日志中可见，Client provision 写入 OSEndpoint，query 面可达
- [x] Phase 3 检查点：L1 清洗命中数据面，Stealth 双通道可用，topology API 端到端通过
- [x] Phase 4 检查点：CTLock 测试稳定通过，`go test ./...` 全绿

## 备注

- 每个任务标注了精确的代码行号和当前代码缺陷描述
- T03 是最复杂的任务，V2 组件的构造函数依赖链较深，建议先读 `*_impl.go` 确认签名
- T04 和 T09 有依赖关系：T04 写入 OSEndpoint 后，T09 的 PullRouteTable 才能使用
- T05 和 T08 有依赖关系：T08 新增的 topology/entitlement handler 需要通过 T05 的 mux 暴露