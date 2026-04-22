# 设计文档：Mirage V1-V2 升级审计整改

## 概述

本设计文档覆盖 T01~T10 共 10 项整改任务的技术方案。按三条主线组织：
1. Gateway 构建与 V2 编排链路（T01, T03, T06, T07）
2. OS 数据库真相与查询面（T02, T05, T08）
3. Client 配置契约与建链路径（T04, T09）
4. 安全声明收口（T10）

---

## 主线一：Gateway 构建与 V2 编排链路

### T01：修复 L1Stats 重复定义

**现状分析：**
- `mirage-gateway/pkg/ebpf/types.go:137` 和 `mirage-gateway/pkg/ebpf/manager.go:369` 各定义了一个 `L1Stats` 结构体
- 两处字段语义相同但注释不同，导致编译失败

**方案：**
1. 删除 `manager.go` 中的 `L1Stats` 定义，保留 `types.go` 中的权威版本（该文件是所有 eBPF 类型的统一定义点）
2. 确认 `manager.go` 中 `ReadL1Stats()` 方法引用的是 `types.go` 中的定义
3. 在 `pkg/ebpf` 下新增 `compile_test.go`，包含一个空测试函数确保包可编译

**涉及文件：**
- `mirage-gateway/pkg/ebpf/manager.go` — 删除重复 `L1Stats`
- `mirage-gateway/pkg/ebpf/types.go` — 保留为权威定义
- `mirage-gateway/pkg/ebpf/compile_test.go` — 新增编译回归测试

---

### T03：Gateway 接入 V2 编排链路

**现状分析：**
- `cmd/gateway/main.go` 已有 650+ 行启动逻辑，但未实例化 V2 orchestrator 组件
- V2 组件（`SurvivalOrchestrator`、`CommitEngine`、`EventDispatcher`、`TransportFabric`、`StealthControlPlane`）均已实现但未接入 `main.go`
- 控制命令仍通过 legacy `GatewayDownlink` 直接落入 `loader/blacklist/gswitch`

**方案：**
1. 在 `main.go` 的启动序列中新增 V2 组件初始化块：
   - 创建 `EventDispatcher` → `CommitEngine` → `TransportFabric` → `SurvivalOrchestrator` → `StealthControlPlane`
   - 依赖顺序：EventDispatcher 最先，StealthControlPlane 最后
2. 采用方案 B（适配转换）：保留 `GatewayDownlink` gRPC service，在入口层新增 `V2CommandAdapter`，将 `PushStrategy/PushQuota` 等 legacy 命令转换为 `ControlCommand`，再投递给 V2 编排链路
3. 新增 `pkg/api/v2_adapter.go`，实现 legacy → V2 命令转换
4. 新增 `tests/v2_e2e_test.go`，覆盖命令闭环

**组件接线图：**
```
GatewayDownlink (gRPC) → V2CommandAdapter → EventDispatcher → CommitEngine → TransportFabric
                                                                    ↓
                                                          SurvivalOrchestrator
                                                                    ↓
                                                        StealthControlPlane
```

**涉及文件：**
- `mirage-gateway/cmd/gateway/main.go` — 新增 V2 组件初始化
- `mirage-gateway/pkg/api/v2_adapter.go` — 新增命令适配层
- `mirage-gateway/tests/v2_e2e_test.go` — 新增端到端测试

---

### T06：L1 清洗命中数据面

**现状分析：**
- `pkg/threat/` 中 threat-intel provider 已加载 ASN/cloud 数据，但 `asnEntries` 初始化为空
- `LookupASN`/`IsCloudIP` 存在但无运行时调用点
- `DefenseApplier.SyncASNBlocklist()` 已实现，但上游从未调用

**方案：**
1. 在 threat-intel provider 初始化时，从 `configs/asn_database.json` 和 `configs/cloud_ranges.json` 加载数据填充 `asnEntries`
2. 在 Gateway 启动序列中，将 threat-intel provider 的产物通过 `DefenseApplier.SyncASNBlocklist()` 下发到 eBPF map
3. 在数据面入口（`pkg/api/handlers.go` 或 V2 adapter）增加 `LookupASN`/`IsCloudIP` 调用点
4. 空 entries 时记录 WARN 日志而非静默跳过

**涉及文件：**
- `mirage-gateway/cmd/gateway/main.go` — 启动时调用 SyncASNBlocklist
- `mirage-gateway/pkg/threat/` — 修复 provider 产物生成
- `mirage-gateway/pkg/ebpf/manager.go` — 确认 SyncASNBlocklist 路径正确

---

### T07：Stealth Control Plane 可靠性修复

**现状分析：**
- `StealthControlPlane.ReceiveLoop()` 只消费 Scheme A（`mux.ReadCommand`），不消费 Scheme B（`decoder`）
- 无通道可用时 `default` 分支直接 continue，形成热循环
- `cmdQueue` 有 `DrainQueue()` 方法但仅标注 "for testing"，无恢复后自动回放

**方案：**
1. 重写 `ReceiveLoop()`：
   - 使用 `select` 同时监听 Scheme A（mux channel）和 Scheme B（decoder channel）
   - 无通道可用时，使用 `time.After` 退避（初始 100ms，最大 5s，指数退避）
2. 新增 `drainOnRecovery()` 方法：
   - 当通道从 `ChannelQueued` 恢复到 `ChannelSchemeA/B` 时，自动 drain `cmdQueue`
   - 回放前检查命令时间戳，丢弃超过 TTL（默认 60s）的命令
3. 在 `SetSchemeAAvailable()` 和状态切换时触发 `drainOnRecovery()`

**涉及文件：**
- `mirage-gateway/pkg/gtunnel/stealth/control_plane.go` — 重写 ReceiveLoop、新增 drain 逻辑
- `mirage-gateway/pkg/gtunnel/stealth/control_plane_test.go` — 新增三场景测试

---

## 主线二：OS 数据库真相与查询面

### T02：统一数据库真相

**现状分析：**
- `mirage-os/pkg/models/db.go` 定义了 GORM models（User, Cell, Gateway, BillingLog, ThreatIntel）
- `mirage-os/api-server/src/prisma/schema.prisma` 定义了 Prisma models
- `mirage-os/gateway-bridge/` 使用 raw SQL upsert
- 三套定义的字段名、主键类型、时间字段语义不一致

**方案：**
1. 新增 `docs/adr/001-database-truth.md`，正式声明 GORM_Models 为运行时数据库真相
2. 逐表对齐：
   - `users`：以 `db.go` 的 `User` 为准，Prisma 的 `User` 改为只读视图
   - `gateways`：以 `db.go` 的 `Gateway` 为准，`gateway-bridge` 的 upsert SQL 改为匹配 GORM 字段
   - `billing_logs`：以 `db.go` 的 `BillingLog` 为准，NestJS billing service 改为调用 Go API
3. 重写 `gateway-bridge/pkg/topology/registry.go` 中的 `Register()` SQL，使用 GORM 字段名
4. 输出迁移脚本 `mirage-os/scripts/migrate-unified-schema.sql`

**涉及文件：**
- `docs/adr/001-database-truth.md` — 新增 ADR
- `mirage-os/pkg/models/db.go` — 权威定义，可能需要补充缺失字段
- `mirage-os/gateway-bridge/pkg/topology/registry.go` — 重写 upsert SQL
- `mirage-os/api-server/src/prisma/schema.prisma` — 标记为适配层
- `mirage-os/scripts/migrate-unified-schema.sql` — 新增迁移脚本

---

### T05：暴露 V2 查询面

**现状分析：**
- `mirage-os/services/` 下已有 `state-query`、`persona-query`、`transaction-query`、`observability-query` 四个 handler
- `mirage-os/services/api-gateway/main.go` 当前只注册了 gRPC service 和 provisioning/billing/console，未挂载 V2 query handlers
- `persona-query` 和 `state-query` 都注册了 `/api/v2/sessions/` 前缀，会导致 mux panic

**方案：**
1. 在 `api-gateway/main.go` 中新增 HTTP mux 挂载块，注册四个 query handler
2. 路由分配：
   - `state-query` → `/api/v2/links`, `/api/v2/sessions/{id}/state`
   - `persona-query` → `/api/v2/personas/`, `/api/v2/sessions/{id}/persona`
   - `transaction-query` → `/api/v2/transactions`
   - `observability-query` → `/api/v2/audit/records`
3. `session` 查询路由 owner 定为 `state-query`，`persona-query` 使用 `/api/v2/sessions/{id}/persona` 子路径
4. 确保 query handlers 使用 `database.GetDB()` 访问 GORM 数据

**涉及文件：**
- `mirage-os/services/api-gateway/main.go` — 挂载 query handlers
- `mirage-os/services/state-query/handler.go` — 确认路由前缀
- `mirage-os/services/persona-query/handler.go` — 修改路由前缀避免冲突
- `mirage-os/services/transaction-query/handler.go` — 确认路由
- `mirage-os/services/observability-query/handler.go` — 确认路由

---

### T08：对齐 topology/entitlement API 契约

**现状分析：**
- Client `pkg/gtclient/topo.go` 指向 `/api/v1/topology`，OS 侧无对应实现
- OS 侧 `gateways.controller.ts`（NestJS）有部分 gateway 查询，但非正式 topology API
- 缺少 entitlement API 的正式定义

**方案：**
1. 新增 `docs/api/topology-contract.md` 和 `docs/api/entitlement-contract.md`
2. 在 OS Go 侧新增 `services/topology/handler.go`：
   - `GET /api/v2/topology` — 返回 `RouteTableResponse`（含 `version`, `published_at`, `signature`, `gateways[]`）
   - 使用 GORM 查询 `gateways` 表，用 OS 私钥签名
3. 在 OS Go 侧新增 `services/entitlement/handler.go`：
   - `GET /api/v2/entitlement` — 返回用户权限与配额
4. 修改 Client `topo.go` 中的 URL 从 `/api/v1/topology` 改为 `/api/v2/topology`
5. 在 `api-gateway/main.go` 中挂载新 handler

**涉及文件：**
- `docs/api/topology-contract.md` — 新增契约文档
- `docs/api/entitlement-contract.md` — 新增契约文档
- `mirage-os/services/topology/handler.go` — 新增
- `mirage-os/services/entitlement/handler.go` — 新增
- `mirage-os/services/api-gateway/main.go` — 挂载新 handler
- `phantom-client/pkg/gtclient/topo.go` — 修改 URL

---

## 主线三：Client 配置契约与建链路径

### T04：统一配置契约

**现状分析：**
- `phantom-client/pkg/persist/store.go` 已定义 `PersistConfig`，包含 `OSEndpoint` 字段
- `phantom-client/cmd/phantom/provision.go` 的 `RunProvisioning()` 负责 token 解析与配置持久化
- `cmd/phantom/main.go` 中 `runDaemonMode` 和 `runForegroundMode` 使用不同的配置装载路径

**方案：**
1. 新增 `pkg/persist/contract.go`，定义配置契约文档注释，明确字段来源：
   - 来自 token：`BootstrapPool`, `CertFingerprint`, `UserID`
   - 来自 provisioning 推导：`OSEndpoint`
   - 运行时缓存：`LastEntitlement`
2. 在 `RunProvisioning()` 中确保 `OSEndpoint` 从 token 的 `os_endpoint` 字段或 gateway endpoint 推导后写入 `PersistConfig`
3. 统一 `runDaemonMode` 和 `runForegroundMode` 的配置装载：
   - 提取公共函数 `loadRuntimeConfig(configDir) (*PersistConfig, []byte, []byte, error)`
   - 两种模式都调用此函数
4. 配置缺失时输出明确错误：`"os_endpoint missing in config.json — re-run provisioning with latest token"`

**涉及文件：**
- `phantom-client/pkg/persist/contract.go` — 新增契约文档
- `phantom-client/cmd/phantom/provision.go` — 确保写入 OSEndpoint
- `phantom-client/cmd/phantom/main.go` — 统一配置装载逻辑

---

### T09：建链参数进入真实路径

**现状分析：**
- `pkg/gtclient/client.go` 中 `probe()` 构造 `QUICEngine` 时未传递 `PinnedCertHash`
- `pkg/nicdetect/` 存在但 `main.go` 中未注入正式 `NICDetector`
- `pkg/gtclient/topo.go` 中 `PullRouteTable()` 为空实现

**方案：**
1. 修改 `probe()` 调用链：
   - 从 `PersistConfig.CertFingerprint` 解码为 `[]byte`
   - 传递给 `QUICEngine` 的 TLS config 作为 `PinnedCertHash`
   - 在 TLS `VerifyPeerCertificate` 回调中校验 fingerprint
2. 在 `main.go` 启动时创建正式 `NICDetector` 实例，注入到 `GTClient`
3. 实现 `PullRouteTable()`：
   - 调用 `GET /api/v2/topology`（T08 新增的接口）
   - 验签后返回 `RouteTableResponse`
4. 启动序列改为：`LoadConfig → PullRouteTable → probe() → 进入长期运行`
5. 各环节失败时定义退化行为：
   - fingerprint 不匹配 → 拒绝连接，返回错误
   - NIC 检测失败 → 使用系统默认路由，记录 WARN
   - 拓扑同步失败 → 重试 3 次，间隔 5s，仍失败则使用 bootstrap pool

**涉及文件：**
- `phantom-client/pkg/gtclient/client.go` — 传递 PinnedCertHash
- `phantom-client/pkg/gtclient/quic_engine.go` — TLS 验证回调
- `phantom-client/pkg/gtclient/topo.go` — 实现 PullRouteTable
- `phantom-client/cmd/phantom/main.go` — 注入 NICDetector，调整启动序列

---

## 主线四：安全声明收口

### T10：CTLock 降级或重写

**现状分析：**
- `mirage-gateway/pkg/gtunnel/ctlock/ctlock.go` 使用 `time.Now()` busy-wait 实现"恒定时间"
- property tests 已知失败，timing envelope 无法满足

**方案（推荐降级）：**
1. 将 `ctlock` 降级为普通 `sync.Mutex` 包装，删除恒定时间声明
2. 重命名为 `TimedLock`，保留基本的"最小持锁时间"语义（非恒定时间）
3. 修复 property tests 为验证"持锁时间 ≥ minDuration"而非"持锁时间 = constDuration"
4. 删除文档中的 constant-time 安全声明

**涉及文件：**
- `mirage-gateway/pkg/gtunnel/ctlock/ctlock.go` — 降级实现
- `mirage-gateway/pkg/gtunnel/ctlock/*_test.go` — 修复测试
- 相关设计文档 — 删除 constant-time 声明

---

## 执行顺序

```
Phase 1 (阻断修复):  T01 → T02
Phase 2 (主链路打通): T03 + T04 + T05 (并行)
Phase 3 (能力生效):   T06 + T07 + T08 + T09 (并行)
Phase 4 (收口):       T10
```

---

## 正确性属性

### P1: L1Stats 类型唯一性（T01）
- 属性：`pkg/ebpf` 包内 `L1Stats` 类型定义数量 = 1
- 验证：编译回归测试 + grep 检查

### P2: 数据库模型一致性（T02）
- 属性：`gateway-bridge Register()` 的 upsert 字段集 ⊆ GORM Gateway model 字段集
- 验证：集成测试 — Register 后 GORM 查询返回一致数据

### P3: V2 组件接线完整性（T03）
- 属性：Gateway 启动后，5 个 V2 组件均为非 nil 且处于 running 状态
- 验证：启动测试检查组件状态

### P4: 配置契约完整性（T04）
- 属性：对任意有效 token，`RunProvisioning()` 产出的 `PersistConfig` 包含非空 `OSEndpoint`
- 验证：property test — 生成随机 token → provision → 检查 OSEndpoint

### P5: 路由无冲突（T05）
- 属性：`api-gateway` 启动时 mux 注册不 panic，且四个 query 路由均可达
- 验证：集成测试 — 启动 + HTTP GET 各路由返回 2xx

### P6: L1 清洗数据面命中（T06）
- 属性：非空 ASN entries → SyncASNBlocklist 被调用 → eBPF map 非空
- 验证：集成测试

### P7: 命令队列恢复（T07）
- 属性：对任意命令序列，通道断开后恢复 → DrainQueue 返回所有未超时命令
- 验证：property test — 生成随机命令 → 断开 → 恢复 → 验证 drain 结果

### P8: topology 往返一致性（T08）
- 属性：OS 生成的 topology 响应通过 Client TopoVerifier 验签
- 验证：集成测试

### P9: 证书钉扎生效（T09）
- 属性：错误 fingerprint → 连接失败；正确 fingerprint → 连接成功
- 验证：单元测试

### P10: CTLock 测试稳定性（T10）
- 属性：`go test ./pkg/gtunnel/ctlock/...` 稳定通过
- 验证：CI 门禁
