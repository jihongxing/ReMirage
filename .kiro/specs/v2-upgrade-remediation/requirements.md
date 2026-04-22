# 需求文档：Mirage V1-V2 升级审计整改

## 简介

本文档覆盖 Mirage V1 到 V2 升级过程中三轮审计发现的 10 项整改任务（T01~T10），按优先级分为 P0（阻断构建/运行/升级验证）、P1（主链路能力未生效）、P2（安全声明与现实不一致）三个层级。

整改围绕三条主线展开：
1. Gateway 构建阻断与 V2 编排链路未接入（T01, T03）
2. 数据库真相分裂与 API 契约不对齐（T02, T05, T08）
3. Client 配置契约分裂与建链参数丢失（T04, T09）

三条工程决策前提：
1. `mirage-os` 运行时数据库真相优先定为 GORM/Go models
2. `mirage-gateway` 真实运行时控制面优先定为 V2 orchestrator/commit/transport/stealth 链路
3. `phantom-client` 配置模型拆成 `BootstrapConfig` + `PersistConfig` 两层

## 术语表

- **Gateway**：`mirage-gateway` 融合网关进程，负责数据面转发与协议编排
- **OS**：`mirage-os` 控制中心，负责用户管理、拓扑分发、计费与查询面
- **Client**：`phantom-client` 客户端，负责建链、拓扑同步与隧道维持
- **Orchestrator**：V2 编排内核，包含 `SurvivalOrchestrator`、`CommitEngine`、`EventDispatcher`、`TransportFabric`
- **StealthControlPlane**：隐蔽控制面，双通道（Scheme A / Scheme B）命令传输
- **BootstrapConfig**：Client 一次性交付材料，来自 provisioning token
- **PersistConfig**：Client 长期运行配置真相，包含 `os_endpoint`、证书钉扎输入、拓扑刷新参数
- **L1Stats**：eBPF 层统计结构体，用于 L1 清洗数据上报
- **GORM_Models**：`mirage-os/pkg/models/db.go` 中定义的 Go 数据库模型，作为运行时数据库真相
- **QueryHandler**：OS 侧 V2 查询面处理器，包含 state-query、persona-query、transaction-query、observability-query
- **TopoVerifier**：Client 侧拓扑响应验签组件
- **QUICEngine**：Client 侧 QUIC 连接引擎
- **NICDetector**：Client 侧物理网卡探测组件
- **CTLock**：`mirage-gateway/pkg/gtunnel/ctlock` 中的恒定时间锁实现

---

## 需求

### 需求 1：修复 Gateway 构建阻断（T01）

**用户故事：** 作为开发者，我需要 Gateway 恢复基础可编译状态，以便后续所有整改任务可以在可构建的代码基础上推进。

#### 验收标准

1. WHEN `pkg/ebpf` 包中存在同名类型双定义时，THE Gateway 构建系统 SHALL 在合并后仅保留一个权威的 `L1Stats` 结构体定义
2. WHEN `L1Stats` 定义被合并后，THE Gateway 构建系统 SHALL 更新所有引用点以匹配权威定义的字段签名
3. THE Gateway 编译流程 SHALL 对 `mirage-gateway` 执行 `go build ./...` 成功完成，无类型重复声明错误
4. WHEN 新代码引入同名类型双定义时，THE CI 检查 SHALL 在编译阶段检测并报告该冲突
5. THE `pkg/ebpf` 包 SHALL 包含最小编译回归测试，验证包内无同名类型冲突

---

### 需求 2：统一 Mirage OS 数据库真相（T02）

**用户故事：** 作为开发者，我需要 Mirage OS 只存在一套被正式声明的运行时数据库模型定义，以便消除 Prisma/GORM/raw SQL 三套真相的冲突。

#### 验收标准

1. THE 仓库 SHALL 包含一份数据库 ADR 文档，正式声明 GORM_Models 为运行时数据库真相
2. THE GORM_Models SHALL 作为 `users`、`gateways`、`billing_logs`、`invite_codes` 表的唯一权威字段定义
3. WHEN `gateway-bridge` 执行 `Register()` 时，THE `gateway-bridge` SHALL 使用与 GORM_Models 一致的表结构成功完成 upsert 操作
4. THE 各服务模块 SHALL 对 `users`、`gateways`、`billing_logs` 表使用统一的主键、业务 ID、金额字段与时间字段语义
5. IF Prisma 模型与 GORM_Models 存在字段冲突，THEN THE 迁移方案 SHALL 将 Prisma 改造为适配层或只读层
6. THE 仓库 SHALL 包含数据迁移脚本，保证线上已有数据可平滑过渡到统一模型

---

### 需求 3：Gateway 接入 V2 编排与控制面链路（T03）

**用户故事：** 作为开发者，我需要 Gateway 运行入口真正接入 V2 编排内核，以便控制命令不再绕过 orchestrator/commit/transport/stealth 链路。

#### 验收标准

1. WHEN Gateway 启动时，THE `cmd/gateway/main.go` SHALL 实例化 `SurvivalOrchestrator`、`CommitEngine`、`EventDispatcher`、`TransportFabric`、`StealthControlPlane` 五个 V2 组件
2. THE Gateway 启动日志 SHALL 包含 V2 orchestrator、transport、stealth 组件创建成功的确认信息
3. WHEN 控制命令从入口接收后，THE Gateway SHALL 将命令路由至 V2 编排链路，而非直接落入 legacy `loader/blacklist/gswitch`
4. THE Gateway SHALL 明确 legacy `GatewayDownlink` 与 V2 `ControlCommand` 的关系，选择适配转换或完全替换方案
5. THE Gateway SHALL 对至少一个真实命令类型完成"接收 → 编排 → 提交 → 回执"的完整闭环
6. THE 仓库 SHALL 包含端到端联调测试，覆盖"OS 发命令 → Gateway 编排执行 → Ack/状态回传"路径

---

### 需求 4：统一 Client 配置契约（T04）

**用户故事：** 作为开发者，我需要 Phantom Client 的 provisioned config 与运行时配置契约统一，以便 daemon 和 foreground 模式都能加载完整运行时依赖。

#### 验收标准

1. THE Client SHALL 定义一份正式的运行时配置契约文档，明确哪些字段来自 token、哪些字段来自持久化存储
2. WHEN provisioning 完成后，THE Client SHALL 将 `os_endpoint` 写入持久化配置文件
3. THE 持久化配置文件 SHALL 包含 `os_endpoint`、证书钉扎输入、运行时拓扑刷新所需的全部参数
4. WHEN Client 以 daemon 模式启动时，THE Client SHALL 通过与 foreground 模式相同的配置装载逻辑构建运行时依赖
5. IF 配置文件缺少必要字段，THEN THE Client SHALL 输出明确的错误信息与升级指引，而非静默失效
6. THE `TopoRefresher` 与 `EntitlementManager` SHALL 在默认安装路径下能成功读取 `os_endpoint` 并正常工作

---

### 需求 5：暴露 OS V2 查询面并消除路由冲突（T05）

**用户故事：** 作为开发者，我需要 Mirage OS 的 V2 查询面被正式暴露且路由无冲突，以便 Gateway 和 Client 可以调用查询接口。

#### 验收标准

1. WHEN `api-gateway` 启动时，THE `api-gateway` SHALL 挂载 `state-query`、`persona-query`、`transaction-query`、`observability-query` 四个 QueryHandler
2. THE `api-gateway` SHALL 在启动时不因重复路由注册而 panic
3. THE `session` 相关查询路由 SHALL 只有一个明确的 owner 路由前缀，其他模块使用子路径或不同资源名
4. WHEN 请求到达 `/api/v2/links`、`/api/v2/personas/*`、`/api/v2/transactions*`、`/api/v2/audit/records` 时，THE `api-gateway` SHALL 返回对应 QueryHandler 的正式响应
5. THE QueryHandler SHALL 访问与 GORM_Models 一致的数据模型
6. THE 仓库 SHALL 包含查询面集成测试，覆盖启动期 mux 注册与基础路由可达性

---

### 需求 6：L1 清洗命中数据面（T06）

**用户故事：** 作为开发者，我需要 Gateway 的 ASN/Cloud L1 清洗规则真正命中数据面，以便威胁情报不再只是被加载到内存而不产生实际清洗行为。

#### 验收标准

1. WHEN threat-intel provider 加载 ASN/cloud 匹配结果后，THE Gateway SHALL 将匹配结果转换为可下发的数据面结构
2. THE Gateway SHALL 为 `LookupASN` 和 `IsCloudIP` 提供至少一个运行时主路径调用点
3. WHEN `asnEntries` 非空时，THE Gateway SHALL 将匹配规则同步到数据面（eBPF map 或 blacklist）
4. IF `asnEntries` 为空，THEN THE Gateway SHALL 记录警告日志，而非静默跳过同步
5. THE 仓库 SHALL 包含集成测试，验证命中 ASN/cloud IP 规则的源地址会被下发到数据面并产生清洗行为

---

### 需求 7：修复 Stealth Control Plane 可靠性（T07）

**用户故事：** 作为开发者，我需要 Stealth Control Plane 的 fallback、排队与恢复语义正确工作，以便双通道断连后命令不会永久丢失。

#### 验收标准

1. WHEN 双通道都不可用时，THE StealthControlPlane SHALL 将命令暂存到 `cmdQueue`
2. WHEN bearer 通道恢复后，THE StealthControlPlane SHALL 自动 drain `cmdQueue` 中的积压命令
3. THE `ReceiveLoop` SHALL 同时消费 Scheme A 和 Scheme B 两个通道的消息
4. WHILE 无通道可用时，THE `ReceiveLoop` SHALL 使用阻塞、退避或条件变量等待，而非空转热循环
5. WHEN 恢复后回放积压命令时，THE StealthControlPlane SHALL 检查命令的幂等性与超时状态，丢弃已失效命令
6. THE 仓库 SHALL 包含测试覆盖三种场景：通道全断、单通道恢复、队列回放

---

### 需求 8：对齐 Client 与 OS 的 topology/entitlement API 契约（T08）

**用户故事：** 作为开发者，我需要 Client 与 OS 的 topology/entitlement API 契约对齐，以便 Client 可以成功完成拓扑拉取与权限验证。

#### 验收标准

1. THE 仓库 SHALL 包含 topology API 契约文档，明确 URL、鉴权方式、响应字段、签名字段、版本语义
2. THE 仓库 SHALL 包含 entitlement API 契约文档，明确 URL、鉴权方式、响应结构、缓存策略与错误码
3. WHEN Client 请求 topology 接口时，THE OS SHALL 在契约定义的 URL 上返回符合 `RouteTableResponse` 结构的响应
4. THE topology 响应 SHALL 包含 `version`、`published_at`、`signature` 字段，且 `version` 与 `published_at` 满足单调递增约束
5. THE topology 响应 SHALL 通过 Client 侧 `TopoVerifier` 的验签检查
6. WHEN fresh provision 完成后，THE Client SHALL 能成功完成至少一次 topology 拉取与 entitlement 拉取

---

### 需求 9：建链参数进入真实连接路径（T09）

**用户故事：** 作为开发者，我需要证书钉扎、NIC 检测、启动期拓扑同步进入真实建链路径，以便安全参数不再被静默跳过。

#### 验收标准

1. WHEN `probe()` 构造 `QUICEngine` 时，THE Client SHALL 将 `BootstrapConfig.CertFingerprint` 解码为 `PinnedCertHash` 并传递给 `QUICEngine`
2. IF 证书指纹与服务端不匹配，THEN THE Client SHALL 拒绝连接并返回明确错误，而非静默跳过钉扎
3. THE Client SHALL 注入正式 `NICDetector`，正常路径不再依赖 `legacyDetectOutbound()` 或 `8.8.8.8:53` fallback
4. WHEN Client 启动时，THE Client SHALL 完成至少一次可信拓扑同步后再进入长期运行状态
5. IF 拓扑同步失败，THEN THE Client SHALL 定义显式退化行为（重试或降级），而非静默继续
6. THE `PullRouteTable()` SHALL 具有正式实现，或被等价的启动期拓扑同步逻辑替代，不允许保留空实现

---

### 需求 10：修复或降级 Constant-Time Lock（T10）

**用户故事：** 作为开发者，我需要 CTLock 的实现与其安全声明一致，以便仓库中不再保留"代码做不到、文档仍宣称做到"的 timing contract。

#### 验收标准

1. THE 仓库 SHALL 明确 CTLock 的需求定位：恒定时间语义或弱化时序差异语义
2. IF 保留恒定时间承诺，THEN THE CTLock SHALL 重写实现并引入校准逻辑，不依赖不稳定的 `time.Now()` busy-wait
3. IF 无法实现可验证的恒定时间语义，THEN THE CTLock SHALL 降级为普通锁实现，并同步删除相关安全声明
4. THE CTLock 测试 SHALL 在目标平台上稳定通过，成为正式门禁而非已知失败项
5. THE `go test ./...` SHALL 不被 CTLock 相关测试失败阻断
