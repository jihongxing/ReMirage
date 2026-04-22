---
Status: temporary
Target Truth: 跨组件整改，结论应回写到对应组件实现
Migration: V1-V2 升级整改清单，完成后归档
---

# Mirage V1-V2 升级审计整改任务清单

## 1. 文档说明

本文档将第一轮、第二轮、第三轮审计 Findings 收敛为一份可直接执行的整改任务清单，用于：

- 生成 `.kiro/specs` 下的正式 Spec 任务
- 指导 V1 -> V2 升级期间的代码整改顺序
- 作为 Gateway / Phantom Client / Mirage OS 三条主线的联调验收基线

每个任务包含：

- 对应 Findings 来源
- 明确的开发任务
- 涉及文件
- 验收标准

---

## 2. 前置收敛决策

在执行下述任务前，建议先锁定三条工程决策，否则后续整改会反复返工：

1. `mirage-os` 的运行时数据库真相优先定为 `GORM / Go models`
2. `mirage-gateway` 的真实运行时控制面优先定为 `V2 orchestrator / commit / transport / stealth` 链路
3. `phantom-client` 的配置模型拆成两层：
   - `BootstrapConfig`：一次性交付材料
   - `PersistConfig`：长期运行配置真相，必须包含 `os_endpoint`、证书钉扎输入、运行时拓扑刷新所需参数

---

## 3. 优先级定义

- `P0`：阻断构建、阻断运行、阻断 V1 -> V2 升级验证，必须优先完成
- `P1`：主链路可运行但能力未真正生效，必须在 V2 联调前完成
- `P2`：可信性、性能或安全声明与现实不一致，需在版本封板前收口

---

## 4. 任务总览

| 任务 ID | 优先级 | 目标 |
|--------|--------|------|
| T01 | P0 | 修复 Gateway 构建阻断，恢复基础可编译状态 |
| T02 | P0 | 确定 Mirage OS 单一数据库真相，并清理 Prisma / GORM / raw SQL 冲突 |
| T03 | P0 | 让 Gateway 真实运行入口接入 V2 编排与控制面链路 |
| T04 | P0 | 统一 Phantom Client 的 provisioned config 与运行时配置契约 |
| T05 | P0 | 正式暴露 Mirage OS 的 V2 查询面，并消除路由冲突 |
| T06 | P1 | 让 Gateway 的 ASN / Cloud L1 清洗真正命中数据面 |
| T07 | P1 | 修复 Stealth Control Plane 的 fallback、排队与恢复语义 |
| T08 | P1 | 对齐 Phantom Client 与 Mirage OS 的 topology / entitlement API 契约 |
| T09 | P1 | 让证书钉扎、NIC 检测、启动期拓扑同步进入真实建链路径 |
| T10 | P2 | 修复或降级 Constant-Time Lock 的不可信实现与声明 |

---

## 5. P0 任务

### T01 修复 Gateway 构建阻断，恢复基础可编译状态

#### 来源 Findings

- 第一轮 `R1-F1`：`mirage-gateway/pkg/ebpf/types.go` 与 `pkg/ebpf/manager.go` 重复声明 `L1Stats`

#### 开发任务

- [ ] T01-1：删除或合并重复的 `L1Stats` 定义，确保 `pkg/ebpf` 只有一个权威结构体
- [ ] T01-2：清理所有引用点，避免因为字段定义漂移引入二次编译错误
- [ ] T01-3：为 `pkg/ebpf` 增加最小编译回归测试，防止再次出现同名类型双定义
- [ ] T01-4：补一条 CI 级检查，至少覆盖 `mirage-gateway` 的编译阶段

#### 涉及文件

- `mirage-gateway/pkg/ebpf/types.go`
- `mirage-gateway/pkg/ebpf/manager.go`
- `mirage-gateway/pkg/ebpf/*`
- `mirage-gateway/go.mod`

#### 验收标准

- `mirage-gateway` 不再因 `L1Stats redeclared` 编译失败
- `go test ./...` 至少不再被 `pkg/ebpf` 的重复类型错误阻断
- 所有导入 `pkg/ebpf` 的包可以正常通过编译阶段

---

### T02 确定 Mirage OS 单一数据库真相，并清理 Prisma / GORM / raw SQL 冲突

#### 来源 Findings

- 第二轮 `R2-F1`：`gateway-bridge` 写入的 `gateways` 表结构与 Go migrator 创建的结构不一致
- 第二轮 `R2-F7`：Prisma 的 `user / billing / gateway` 模型与 Go 侧迁移表结构不兼容
- 第三轮 `R3-F1`：`mirage-os` 同时存在 Prisma、GORM、raw SQL 三套数据库真相

#### 开发任务

- [ ] T02-1：在仓库内新增一份数据库 ADR，正式声明运行时数据库真相是 `GORM/Go models` 还是 `Prisma`
- [ ] T02-2：如果选择 `GORM/Go models` 作为真相，统一以 `pkg/models/db.go` 为权威定义，逐步改造 Prisma 为适配层或只读层
- [ ] T02-3：重写 `gateway-bridge` 的 `gateways` upsert SQL，使其严格匹配最终真相表结构
- [ ] T02-4：对 `users`、`gateways`、`billing_logs`、`invite_codes/invitations` 做逐表字段对齐，明确主键、业务 ID、金额字段、时间字段的唯一含义
- [ ] T02-5：停掉或迁移仍基于冲突字段假设的 NestJS 服务，尤其是 billing / gateways / users 相关模块
- [ ] T02-6：输出一份数据迁移脚本或兼容视图方案，避免线上已有数据无法平滑过渡

#### 涉及文件

- `mirage-os/pkg/database/database.go`
- `mirage-os/pkg/models/db.go`
- `mirage-os/gateway-bridge/pkg/topology/registry.go`
- `mirage-os/api-server/src/prisma/schema.prisma`
- `mirage-os/api-server/src/modules/billing/billing.service.ts`
- `mirage-os/api-server/src/modules/gateways/*`
- `mirage-os/api-server/src/modules/users/*`
- 新增数据库 ADR / 迁移文档

#### 验收标准

- 仓库中只存在一套被正式声明为“运行时数据库真相”的模型定义
- `gateway-bridge Register()` 能对同一套 `gateways` 表成功 upsert
- `billing / user / gateway` 相关服务不再混用冲突主键或冲突字段语义
- 新老数据有明确迁移路径，不能依赖人工修库

---

### T03 让 Gateway 真实运行入口接入 V2 编排与控制面链路

#### 来源 Findings

- 第二轮 `R2-F2`：Gateway 启动时仍只接 legacy downlink，V2 orchestrator / stealth control plane 不可达
- 第三轮 `R3-F2`：运行时入口、proto 定义与 V2 编排链路未对齐

#### 开发任务

- [ ] T03-1：在 `cmd/gateway/main.go` 中正式实例化 `SurvivalOrchestrator`、`CommitEngine`、`EventDispatcher`、`TransportFabric`、`StealthControlPlane`
- [ ] T03-2：明确 legacy `GatewayDownlink` 与 V2 `ControlCommand` 的关系，二选一：
- [ ] T03-3：方案 A，新增正式的 `ControlCommand` service，并让 OS / Gateway 都走 V2 命令语义
- [ ] T03-4：方案 B，保留 `PushStrategy/PushQuota/...`，但在 Gateway 入口层做适配，转换为 V2 orchestrator 内部命令
- [ ] T03-5：让实际接收到的控制命令进入 commit / event / transport / stealth 整条链路，而不是直接落 `loader / blacklist / gswitch`
- [ ] T03-6：为 V2 控制面补一条端到端联调测试，覆盖“OS 发命令 -> Gateway 编排执行 -> Ack / 状态回传”

#### 涉及文件

- `mirage-gateway/cmd/gateway/main.go`
- `mirage-gateway/pkg/api/handlers.go`
- `mirage-gateway/pkg/orchestrator/survival/survival.go`
- `mirage-gateway/pkg/orchestrator/commit/engine.go`
- `mirage-gateway/pkg/orchestrator/events/dispatcher.go`
- `mirage-gateway/pkg/orchestrator/transport/fabric.go`
- `mirage-gateway/pkg/orchestrator/transport/fabric_impl.go`
- `mirage-gateway/pkg/gtunnel/stealth/control_plane.go`
- `mirage-proto/mirage.proto`
- `mirage-proto/control_command.proto`
- `mirage-proto/gen/*`

#### 验收标准

- Gateway 运行时启动日志中能确认 V2 orchestrator / transport / stealth 组件被实际创建并接线
- 控制命令从入口进入后，不再直接绕过 V2 编排链路
- 至少存在一个真实命令类型完成“接收、编排、提交、回执”的闭环
- V2 控制链路不是库代码孤岛，而是运行态主路径的一部分

---

### T04 统一 Phantom Client 的 provisioned config 与运行时配置契约

#### 来源 Findings

- 第二轮 `R2-F3`：provisioning 未持久化 `os_endpoint`
- 第三轮 `R3-F3`：`BootstrapConfig` 与 `PersistConfig` 契约分裂，运行时配置来源不统一

#### 开发任务

- [ ] T04-1：定义一份正式的“客户端运行时配置契约”，明确哪些字段来自 token，哪些字段必须持久化
- [ ] T04-2：在 provisioning 阶段把 `os_endpoint` 写入持久化配置
- [ ] T04-3：决定 `OSEndpoint` 是否进入 `BootstrapConfig`，或通过单独的 `RuntimeConfig` 承载；不能继续依赖散落字段拼装
- [ ] T04-4：让 daemon 模式和 foreground 模式都通过同一套配置装载逻辑构建运行时依赖
- [ ] T04-5：对配置缺失场景补明确错误信息与升级指引，避免后台任务静默失效

#### 涉及文件

- `phantom-client/cmd/phantom/provision.go`
- `phantom-client/cmd/phantom/main.go`
- `phantom-client/pkg/persist/store.go`
- `phantom-client/pkg/token/token.go`
- `phantom-client/pkg/*`

#### 验收标准

- 新 provision 的 `config.json` 中包含 `os_endpoint`
- foreground / daemon 两种启动方式都能加载完整运行时配置
- `TopoRefresher` 与 `EntitlementManager` 不再因为默认安装路径缺少 `os_endpoint` 而直接失效
- 配置真相只有一套，不再需要在运行时用多处对象拼字段

---

### T05 正式暴露 Mirage OS 的 V2 查询面，并消除路由冲突

#### 来源 Findings

- 第一轮 `R1-F5`：V2 query handlers 已实现但未暴露
- 第二轮 `R2-F6`：`persona-query` 与 `state-query` 重复注册 `/api/v2/sessions/`，直接挂同一 mux 会 panic

#### 开发任务

- [ ] T05-1：在 `services/api-gateway/main.go` 中正式挂载 `state-query`、`persona-query`、`transaction-query`、`observability-query`
- [ ] T05-2：梳理四个 query handler 的路由归属，消除重复的 `/api/v2/sessions/` 注册
- [ ] T05-3：明确 `session` 查询路由的 owner，其他模块改用子路径、不同资源名或内部调用
- [ ] T05-4：补充 query 面的集成测试，覆盖启动期 mux 注册与基础路由可达性
- [ ] T05-5：结合 T02 的数据库真相决策，确保 query handlers 访问的是同一套真实数据模型

#### 涉及文件

- `mirage-os/services/api-gateway/main.go`
- `mirage-os/services/state-query/handler.go`
- `mirage-os/services/persona-query/handler.go`
- `mirage-os/services/transaction-query/handler.go`
- `mirage-os/services/observability-query/handler.go`
- `mirage-os/services/*`

#### 验收标准

- `api-gateway` 启动时不再因为重复路由注册 panic
- `/api/v2/links`、`/api/v2/personas/*`、`/api/v2/transactions*`、`/api/v2/audit/records` 至少有一条可达的正式入口
- `session` 相关查询只有一个明确 owner 路由前缀
- 查询面启动后可被联调，而不是“代码存在但进程不提供服务”

---

## 6. P1 任务

### T06 让 Gateway 的 ASN / Cloud L1 清洗真正命中数据面

#### 来源 Findings

- 第一轮 `R1-F2`：本地 threat-intel provider 被加载，但 `asnEntries` 初始化为空且从未真正下发；`LookupASN` / `IsCloudIP` 没有运行时调用点

#### 开发任务

- [ ] T06-1：梳理 L1 清洗的真实入口，明确是写 blacklist、写 eBPF map，还是进入独立的 prefilter 数据面
- [ ] T06-2：把 threat-intel provider 的 ASN / cloud 匹配结果真正转换为可下发的数据面结构
- [ ] T06-3：去掉“slice 为空就完全不 sync”的死条件，或者在 provider 层补足产物生成
- [ ] T06-4：为 `LookupASN` / `IsCloudIP` 增加真实运行时调用点，不允许只存在库函数
- [ ] T06-5：补一条集成测试，验证命中的 ASN / cloud IP 会被下发到数据面并产生实际清洗行为

#### 涉及文件

- `mirage-gateway/cmd/gateway/main.go`
- `mirage-gateway/pkg/threat/*`
- `mirage-gateway/pkg/ebpf/*`
- `mirage-gateway/pkg/api/*`

#### 验收标准

- 命中 ASN / 云厂商规则的源地址能够进入真实 L1 清洗路径
- `LookupASN` / `IsCloudIP` 至少存在一个运行时主路径调用点
- 威胁情报不再只是被加载到内存里，而是能影响实际数据面决策

---

### T07 修复 Stealth Control Plane 的 fallback、排队与恢复语义

#### 来源 Findings

- 第一轮 `R1-F3`：双通道都不可用时命令进入 `cmdQueue`，恢复后没有 drain 路径；`ReceiveLoop` 不消费 Scheme B，且无通道时会热循环

#### 开发任务

- [ ] T07-1：实现 `cmdQueue` 的恢复后回放机制，保证 bearer 恢复后积压命令能被真正发送
- [ ] T07-2：为 `ReceiveLoop` 补齐 Scheme B 的接收与投递路径
- [ ] T07-3：在“无通道可用”状态下增加阻塞、退避或条件变量，避免空转热循环
- [ ] T07-4：明确命令幂等、超时和失效语义，避免恢复后回放旧命令破坏状态
- [ ] T07-5：补测试覆盖三种场景：通道全断、单通道恢复、队列回放

#### 涉及文件

- `mirage-gateway/pkg/gtunnel/stealth/control_plane.go`
- `mirage-gateway/pkg/gtunnel/stealth/*`
- `mirage-gateway/pkg/orchestrator/transport/*`

#### 验收标准

- 通道恢复后，排队命令可以被自动 drain，而不是永久滞留
- Scheme A / Scheme B 都具备真实的接收投递能力
- 两通道都不可用时不再出现明显热循环
- fallback 行为满足“暂存并恢复后继续传输”的设计预期

---

### T08 对齐 Phantom Client 与 Mirage OS 的 topology / entitlement API 契约

#### 来源 Findings

- 第二轮 `R2-F4`：Client 指向 `/api/v1/topology`，仓库内未找到对应服务实现
- 第三轮 `R3-F4`：Client 的 API 路径、响应结构与 OS 现有实现不对齐

#### 开发任务

- [ ] T08-1：正式定义 topology API 契约，明确 URL、鉴权方式、响应字段、签名字段、版本语义
- [ ] T08-2：正式定义 entitlement API 契约，明确 URL、鉴权方式、响应结构、缓存与错误码
- [ ] T08-3：在 Mirage OS 中实现与契约一致的正式 handler，不能继续依赖不存在的 `/api/v1/topology`
- [ ] T08-4：让 Client fetcher 指向真实存在的 OS 路由，而不是测试桩接口
- [ ] T08-5：保证 topology 响应满足 `RouteTableResponse` 的验签要求，包括 `version`、`published_at`、`signature`

#### 涉及文件

- `phantom-client/cmd/phantom/main.go`
- `phantom-client/pkg/gtclient/topo.go`
- `mirage-os/api-server/src/modules/gateways/gateways.controller.ts`
- `mirage-os/services/api-gateway/main.go`
- `mirage-os/services/*`
- 新增或更新 topology / entitlement API 契约文档

#### 验收标准

- Client 指向的 topology / entitlement URL 在 OS 侧真实存在
- topology 响应能通过 `TopoVerifier`
- `version` 与 `published_at` 满足单调递增约束
- fresh provision 后，Client 可以完成至少一次成功的 topology 拉取与 entitlement 拉取

---

### T09 让证书钉扎、NIC 检测、启动期拓扑同步进入真实建链路径

#### 来源 Findings

- 第一轮 `R1-F4`：`probe()` 构造 `QUICEngine` 时丢弃证书钉扎与 NIC 检测输入，`PullRouteTable` 仍是 stub
- 第二轮 `R2-F5`：真实连接路径仍未传递 `PinnedCertHash` 与 `NICDetector`
- 第三轮 `R3-F4`：建链参数传递与运行时启动同步未对齐，`8.8.8.8:53` fallback 仍可达

#### 开发任务

- [ ] T09-1：把 `BootstrapConfig.CertFingerprint` 或其替代字段解码为 `PinnedCertHash`，在真实 `probe()` 路径传给 `QUICEngine`
- [ ] T09-2：为 Client 注入正式 `NICDetector`，不允许正常路径继续落到 `legacyDetectOutbound()`
- [ ] T09-3：实现 `PullRouteTable()`，或移除空实现并用等价的正式启动期拓扑同步替代
- [ ] T09-4：让启动阶段至少完成一次可信拓扑同步，再进入长期运行状态
- [ ] T09-5：为错误 fingerprint、缺失 NIC 信息、拓扑同步失败分别定义显式退化行为

#### 涉及文件

- `phantom-client/pkg/gtclient/client.go`
- `phantom-client/pkg/gtclient/quic_engine.go`
- `phantom-client/pkg/gtclient/topo.go`
- `phantom-client/pkg/token/token.go`
- `phantom-client/cmd/phantom/main.go`
- `phantom-client/pkg/*`

#### 验收标准

- 错误证书指纹会导致连接失败，而不是静默跳过钉扎
- 正常运行路径下不再依赖 `8.8.8.8:53` 做物理网卡探测
- Client 启动后可完成至少一次正式的拓扑同步
- 证书钉扎、NIC 检测、路由表拉取都位于真实连接主路径，而不是旁路代码

---

## 7. P2 任务

### T10 修复或降级 Constant-Time Lock 的不可信实现与声明

#### 来源 Findings

- 第一轮 `R1-F6`：`ctlock` 依赖 `time.Now()` busy-wait + 未校准假工作，自测已经无法满足承诺的 timing envelope

#### 开发任务

- [ ] T10-1：先明确需求：该锁是否真的必须提供“恒定时间”语义，还是只需要“弱化时序差异”
- [ ] T10-2：如果保留恒定时间承诺，重写实现并引入校准逻辑，避免依赖不稳定的 busy-wait
- [ ] T10-3：如果做不到可验证的恒定时间语义，降级为普通锁实现，并同步删除相关安全声明与 spec 假设
- [ ] T10-4：修复 property tests，使其成为正式门禁，而不是已知失败但继续保留

#### 涉及文件

- `mirage-gateway/pkg/gtunnel/ctlock/ctlock.go`
- `mirage-gateway/pkg/gtunnel/ctlock/*_test.go`
- 相关设计文档 / spec

#### 验收标准

- `ctlock` 的测试在目标平台上稳定通过，或该功能被正式降级并删除错误声明
- 仓库中不再保留“代码做不到、文档仍宣称做到”的 timing contract
- `go test ./...` 不再被 `ctlock` 相关失败阻断

---

## 8. 建议执行顺序

建议按以下顺序推进，避免返工：

1. 先做 `T01`，恢复 Gateway 基础可编译状态
2. 再做 `T02`，先锁定 Mirage OS 的数据库真相
3. 并行推进 `T03`、`T04`、`T05`
4. 在运行主链路打通后，推进 `T06`、`T07`、`T08`、`T09`
5. 最后完成 `T10`，收口 V2 安全声明与测试可信性

---

## 9. Spec 拆分建议

如果要转成 `.kiro/specs`，建议最少拆成以下 6 个 Spec：

- `v2-gateway-runtime-control-plane`
- `v2-gateway-l1-cleaning-and-stealth-reliability`
- `v2-phantom-client-runtime-contract`
- `v2-phantom-client-topology-and-quic-hardening`
- `v2-mirage-os-db-truth-and-query-plane`
- `v2-ctlock-contract-closure`

