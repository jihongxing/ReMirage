# 需求文档：OS-Gateway 安全闭环

## 简介

本 Spec 对应 `OS-Gateway 安全整改清单.md` 中两周内修部分（七~九），目标是将 Spec 1-1（OS 紧急封洞）和 Spec 1-2（Gateway 紧急封洞）建立的单点安全能力，升级为 OS 与 Gateway 之间的完整安全闭环。

当前状态（Spec 1-1/1-2 完成后）：
1. OS 侧已有 RBAC 基础（RolesGuard + @Roles 装饰器），但角色模型不统一，缺少 operator/auditor 等角色
2. 内部服务已收口到 127.0.0.1 + shared secret，但各服务鉴权标准不一致
3. Gateway 已有黑名单 → eBPF 同步链路，但 OS 侧威胁情报无法统一下发封禁
4. Gateway 已有威胁等级 → 自动封禁，但缺少本地安全状态机驱动整体行为
5. 两侧均缺少控制面审计日志和安全观测指标

本轮整改核心目标：**把散落的安全能力收敛成统一的安全运行模型。**

## 术语表

- **RBAC**：基于角色的访问控制，本轮扩展为 user/admin/operator/auditor 四角色
- **资源归属**：每个资源（Gateway/Cell/User/Session）都有明确的 owner，非 owner 不可访问
- **控制面审计日志**：记录所有敏感操作（封禁/解封/kill/策略下发/等级切换）的结构化日志
- **威胁情报回路**：OS 识别高风险来源 → 生成封禁指令 → 下发 Gateway → Gateway 写入 eBPF → 数据面生效
- **安全基线**：输入校验、限流、安全 Header、CORS 收敛、错误响应脱敏等基础防护
- **入口处置策略**：Gateway 对异常流量的标准化处置动作（Pass/Observe/Throttle/Trap/Drop）
- **安全状态机**：Gateway 本地运行态（Normal/Alert/HighPressure/Isolated/Silent），不同状态对应不同防御强度
- **联动**：蜜罐命中 → 反哺 Cortex → 更新黑名单 → 数据面生效的完整链路

## 需求

### 需求 1：统一 RBAC 与资源归属模型（OS-2W-1）

**用户故事：** 作为安全工程师，我需要系统有统一的角色模型和资源归属校验，以便权限策略一致且不可绕过。

#### 验收标准

1. THE OS API Server SHALL 支持四种角色：`user`（普通用户）、`admin`（管理员）、`operator`（运维）、`auditor`（审计只读）
2. THE `RolesGuard` SHALL 从统一的角色权限矩阵中查询权限，而不是在每个 Controller 硬编码
3. THE 角色权限矩阵 SHALL 定义为：admin 拥有全部权限；operator 拥有 Gateway/Cell 管理权限但无用户管理权限；auditor 仅拥有只读权限；user 仅可访问自己的资源
4. THE API Server SHALL 为所有资源类接口增加 owner check：用户只能访问 `userId` 匹配的资源（billing/quota/session）
5. THE `JwtStrategy.validate` SHALL 从 JWT payload 提取 role 并附加到 request.user，role 缺失时默认为 `user`
6. IF 用户角色不满足接口要求，THEN THE API SHALL 返回 HTTP 403 并记录拒绝日志（角色、接口、时间戳）

### 需求 2：内部服务统一鉴权标准（OS-2W-2）

**用户故事：** 作为安全工程师，我需要 OS 内部所有服务间调用都有统一的身份校验，以便不存在裸内部服务。

#### 验收标准

1. THE gateway-bridge REST API SHALL 继续使用 `X-Internal-Secret` 共享密钥校验（Spec 1-1 已实现）
2. THE gateway-bridge gRPC 服务 SHALL 在生产模式下强制 mTLS（Spec 1-1 已实现），并增加客户端证书 CN 白名单校验
3. THE API Server 到 gateway-bridge 的内部调用 SHALL 统一携带 `X-Internal-Secret` Header
4. THE WebSocket Gateway（ws-gateway）SHALL 增加 JWT 校验，拒绝无效 token 的连接
5. THE 所有内部 HTTP 服务 SHALL 记录访问日志（来源 IP、请求路径、鉴权结果、时间戳）
6. IF 内部服务收到未鉴权请求，THEN SHALL 返回 401 并记录安全告警

### 需求 3：控制面审计日志（OS-2W-3）

**用户故事：** 作为安全工程师，我需要所有敏感操作都有审计日志，以便事后可追溯关键操作链路。

#### 验收标准

1. THE OS SHALL 记录以下敏感操作的审计日志：用户封禁/解封、Gateway kill、策略下发、威胁等级切换、高危指令触发、角色变更、配置修改
2. THE 审计日志 SHALL 包含：操作者 ID、操作者角色、时间戳、来源 IP、目标资源、动作类型、动作参数、执行结果（成功/失败/拒绝）
3. THE 审计日志 SHALL 写入独立的审计表（`audit_logs`），不与业务日志混合
4. THE 审计日志 SHALL 不可被普通 API 删除，仅 auditor 角色可查询
5. THE gateway-bridge SHALL 为所有下行指令（PushStrategy/PushBlacklist/PushQuota/PushReincarnation）记录审计日志
6. THE 审计日志 SHALL 支持按时间范围、操作者、动作类型查询的 REST API（仅 admin/auditor 可访问）

### 需求 4：异常来源到 Gateway 封禁的统一回路（OS-2W-4）

**用户故事：** 作为安全工程师，我需要 OS 能把识别到的高风险来源稳定下发到 Gateway 并在数据面生效，以便封禁不是一次性动作而是可管理状态。

#### 验收标准

1. THE OS SHALL 维护全局威胁情报表（`threat_intel`），记录高风险 IP/CIDR、风险等级、来源（手动/自动/Gateway 上报）、TTL
2. THE OS SHALL 通过 gateway-bridge gRPC 下行通道将威胁情报同步到所有在线 Gateway（使用现有 PushBlacklist 指令）
3. THE Gateway BlacklistManager.MergeGlobal SHALL 正确合并 OS 下发的全局黑名单，全局条目优先级高于本地条目
4. THE OS SHALL 支持封禁 TTL 管理：条目到期自动解封，支持人工提前解除
5. THE Gateway SHALL 在封禁条目变更时通过心跳上报当前黑名单摘要（条目数 + 最新更新时间戳），OS 可校验一致性
6. THE OS SHALL 在收到 Gateway 威胁事件上报后，自动评估是否需要全局封禁（单 Gateway 上报 >= 3 次同一来源 → 全局封禁）

### 需求 5：安全基线（OS-2W-5）

**用户故事：** 作为安全工程师，我需要 API 服务具备基础安全防护，以便常见攻击手段无法轻易得逞。

#### 验收标准

1. THE API Server SHALL 对所有请求体启用输入校验（使用 NestJS `class-validator` + `ValidationPipe`），拒绝不符合 DTO 定义的请求
2. THE API Server SHALL 设置安全 HTTP Header：`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Strict-Transport-Security`（生产模式）
3. THE API Server SHALL 收敛 CORS 配置：生产模式下只允许指定 origin，禁止 `*`
4. THE API Server SHALL 对错误响应脱敏：生产模式下不返回堆栈信息、不暴露内部路径、数据库错误统一返回 500
5. THE API Server SHALL 对所有 GET 列表接口增加分页限制（默认 20，最大 100），防止大量数据泄露
6. THE gateway-bridge SHALL 在生产模式下禁用 gRPC reflection

### 需求 6：标准入口处置策略（Gateway-2W-1）

**用户故事：** 作为安全工程师，我需要 Gateway 对异常流量有标准化的处置策略，以便面对异常来源时系统行为一致。

#### 验收标准

1. THE Gateway SHALL 定义入口处置策略配置结构，支持按触发条件映射到处置动作
2. THE 处置动作 SHALL 包含 5 级：`Pass`（放行）、`Observe`（记录但不阻断）、`Throttle`（限速到 N pps）、`Trap`（引流蜜罐）、`Drop`（静默丢弃）
3. THE 触发条件 SHALL 包含：黑名单命中、威胁等级阈值、蜜罐命中、异常指纹匹配、连接速率超限
4. THE 处置策略 SHALL 可通过 `gateway.yaml` 配置，支持热加载（OS 下发更新）
5. THE Gateway SHALL 为每次处置动作记录结构化日志（源 IP、动作、触发条件、时间戳）
6. THE 处置策略 SHALL 支持优先级：多条件同时命中时执行最严格的动作

### 需求 7：蜜罐、指纹、黑名单三者联动（Gateway-2W-2）

**用户故事：** 作为安全工程师，我需要蜜罐命中能反哺威胁分析并自动更新黑名单，以便同一异常来源在多个模块间形成统一风险视图。

#### 验收标准

1. THE Phantom（蜜罐）模块 SHALL 在命中蜜罐时向 Cortex（威胁分析）发送事件（源 IP、命中类型、时间戳、请求特征）
2. THE Cortex SHALL 在收到蜜罐命中事件后更新该 IP 的风险评分，蜜罐命中 +30 分（满分 100）
3. THE Cortex SHALL 在 IP 风险评分 >= 70 时自动触发 BlacklistManager.Add（TTL 2 小时）
4. THE B-DNA（指纹识别）SHALL 在检测到高危指纹（已知扫描器/爬虫指纹）时向 Cortex 发送事件
5. THE Cortex SHALL 维护 IP → 风险评分 的内存映射，评分随时间衰减（每小时 -10 分）
6. THE 联动链路 SHALL 有断路器保护：Cortex 处理队列满时丢弃低优先级事件，不阻塞蜜罐/指纹模块

### 需求 8：Gateway 本地安全状态机（Gateway-2W-3）

**用户故事：** 作为安全工程师，我需要 Gateway 在压力上升时能自动进入更保守的运行状态，以便高对抗能力体现为整体行为。

#### 验收标准

1. THE Gateway SHALL 实现本地安全状态机，包含 5 种状态：`Normal`（正常）、`Alert`（警戒）、`HighPressure`（高压）、`Isolated`（隔离）、`Silent`（静默）
2. THE 状态迁移 SHALL 由以下因素驱动：威胁等级变化、入口拒绝率、黑名单命中率、控制面连接状态、OS 下发指令
3. THE 每种状态 SHALL 对应不同的入口策略：Normal 使用默认策略；Alert 收紧 Throttle 阈值；HighPressure 启用主动 Drop；Isolated 仅允许白名单流量；Silent 最小化对外暴露
4. THE 状态迁移 SHALL 有冷却期（升级立即执行，降级需等待 300 秒无新威胁）
5. THE 状态机 SHALL 将当前状态通过心跳上报 OS，OS 可在控制台展示
6. THE 状态机 SHALL 支持 OS 强制切换状态（通过 PushStrategy 指令）

### 需求 9：关键安全动作观测指标（Gateway-2W-4）

**用户故事：** 作为运维工程师，我需要安全能力可量化，以便明确看到安全改造收益。

#### 验收标准

1. THE Gateway SHALL 暴露以下 Prometheus 指标：`mirage_ingress_reject_total`（入口拒绝数，按动作类型分标签）、`mirage_honeypot_hit_total`（蜜罐引流数）、`mirage_blacklist_hit_total`（黑名单命中数）、`mirage_threat_escalation_total`（威胁升级次数）、`mirage_auth_failure_total`（控制面鉴权失败数）、`mirage_mtls_error_total`（mTLS 异常次数）
2. THE 指标 SHALL 通过 `/metrics` HTTP 端点暴露（仅绑定 127.0.0.1）
3. THE 指标 SHALL 包含 Gateway ID 标签，支持多 Gateway 聚合
4. THE Gateway SHALL 在安全状态机状态变化时记录 `mirage_security_state` gauge 指标（0=Normal, 1=Alert, 2=HighPressure, 3=Isolated, 4=Silent）

### 需求 10：安全回归测试（Gateway-2W-5）

**用户故事：** 作为开发工程师，我需要关键安全能力可通过自动化验证，以便后续改动不会把安全链路改坏。

#### 验收标准

1. THE 测试套件 SHALL 包含：未授权控制面接入测试（无 mTLS 连接被拒绝）
2. THE 测试套件 SHALL 包含：异常流量入口拒绝测试（黑名单 IP 被 Drop）
3. THE 测试套件 SHALL 包含：黑名单生效测试（Go 侧 Add → eBPF Map 可查到）
4. THE 测试套件 SHALL 包含：mTLS 缺失与证书错误测试（错误证书连接被拒绝）
5. THE 测试套件 SHALL 包含：RBAC 越权测试（user 角色访问 admin 接口返回 403）
6. THE 测试套件 SHALL 包含：审计日志完整性测试（敏感操作后审计表有对应记录）
7. THE 所有安全回归测试 SHALL 可通过 `make test-security` 一键执行
