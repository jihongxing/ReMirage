# 需求文档：运营前安全加固（Pre-Launch Security Hardening）

## 简介

基于 `docs/05-实施指南/运营前安全审计-可执行任务清单.md` 的 18 项安全审计发现（T01~T18），本文档将审计结论转化为可验证的安全需求。覆盖 9 类攻击面，按 P0（首批客户前必须完成，11 项）、P1（两周内完成，6 项）、P2（路线图级，1 项）三个优先级分层。

核心防御主线：
1. 资金与配额不可被重复核销或直接绕过（T02, T03, T14）
2. Client 首次启动与共振恢复不可被劫持（T04, T05）
3. 节点失陷、日志泄露、eBPF 异常不可将整套系统一起拖下水（T06, T07, T08, T13, T15）

## 术语表

- **OS**：Mirage OS 控制中心，负责用户管理、计费、网关编排等控制面逻辑
- **Gateway**：Mirage Gateway 融合网关，负责数据面流量处理、eBPF 程序挂载、隧道管理
- **Client**：Phantom Client 客户端，用户侧隧道终端
- **InternalHMACGuard**：内部接口 HMAC-SHA256 签名校验中间件
- **Epoch**：信令世代编号，单调递增，用于防重放
- **ManifestID**：信令清单标识，绑定特定路由表版本
- **CSR**：Certificate Signing Request，证书签名请求
- **Canary_Attach**：eBPF 金丝雀挂载，先在虚拟接口验证再挂生产网卡
- **Sanitizer**：日志脱敏处理器，截断或遮蔽敏感字段
- **Breach_Auth**：基于 Ed25519 签名的管理员认证流程
- **Invite_Tree**：邀请树，记录用户间邀请关系的有向图
- **Session_Shaper**：会话级流量整形器，在业务分帧层做时序去相关
- **Path_Health_Scorer**：路径健康评分器，维护 RTT 基线和异常检测
- **Release_Manifest**：发布清单，包含版本、构建哈希、Ed25519 签名
- **Fail_Closed**：安全默认策略，缺少必要配置时拒绝启动而非降级运行

---

## P0 需求（首批客户前必须完成）

---

### 需求 1：OS 内部接口鉴权与网络边界隔离（T01）

**用户故事：** 作为系统运维人员，我希望所有 OS 内部接口都有独立的鉴权和网络隔离，以防止外部攻击者伪造内部请求篡改用户余额或触发非授权操作。

#### 验收标准

1. WHEN 一个请求到达 `/webhook/*` 或 `/delivery/*` 端点且未携带有效 HMAC-SHA256 签名, THE OS SHALL 返回 401 或 403 状态码并拒绝处理该请求
2. WHEN 一个请求携带的 HMAC 签名中 timestamp 与服务器时间差超过 5 分钟, THE InternalHMACGuard SHALL 拒绝该请求并返回 403 状态码
3. WHEN 一个请求的 nonce 在 5 分钟窗口期内已被使用过, THE InternalHMACGuard SHALL 拒绝该请求以防止重放攻击
4. THE OS SHALL 将 `/webhook/*` 和 `/delivery/*` 路由从公网 `/api` 前缀分离，绑定到独立的内部监听端口或 Unix socket
5. WHEN DeliveryController 向 PROVISIONER_URL 发起内部调用, THE OS SHALL 在请求中附加 HMAC 签名 header
6. WHEN BridgeClientService 的 `BRIDGE_INTERNAL_SECRET` 环境变量为空, THE OS SHALL 在启动时抛出异常并拒绝启动

---

### 需求 2：XMR 充值结算单一真相源（T02）

**用户故事：** 作为系统运维人员，我希望 XMR 充值流程有唯一的落账路径和幂等保护，以防止重复调用导致余额被多次增加。

#### 验收标准

1. THE OS 的 XMR Webhook 端点 SHALL 仅保留通知功能（推送 WebSocket 事件），不再直接修改用户余额
2. WHEN monero_manager 的 confirmDeposit 被调用, THE Billing_Service SHALL 仅在 Deposit 状态为 PENDING 时执行 PENDING→CONFIRMED 状态转换并增加余额
3. WHEN 同一笔链上转账的 confirmDeposit 被重复调用, THE Billing_Service SHALL 因状态前置条件不满足而跳过落账，确保余额只增加一次
4. THE Database SHALL 使用 `(txHash, transferIndex)` 复合唯一键区分同一笔交易的多个 output
5. WHEN 用户发起充值请求, THE Billing_Service SHALL 为每次请求生成独立的订单级子地址并关联到 Deposit 记录

---

### 需求 3：邀请树与反白嫖治理（T03）

**用户故事：** 作为系统运维人员，我希望邀请制注册具备完整的邀请链追溯和反滥用能力，以防止批量注册薅取试用流量。

#### 验收标准

1. WHEN 一个邀请码被核销, THE Auth_Service SHALL 写入完整邀请链信息，包括 invited_by、invite_root 和 invite_depth（parent.depth + 1）
2. WHILE 新用户处于 7 天观察期内, THE Quota_Service SHALL 限制该用户配额上限为 1GB 且并发会话数为 1
3. WHEN 运维人员触发邀请树熔断操作, THE Admin_Service SHALL 按 invite_root 批量冻结整条邀请链上的所有用户
4. WHEN 同一邀请码来源 IP 在一小时内注册尝试超过 3 次, THE Auth_Service SHALL 拒绝后续注册请求
5. THE Database SHALL 在 User 模型上存储 invitedBy、inviteRoot、inviteDepth 和 observationEndsAt 字段以支持邀请链追溯

---

### 需求 4：信令共振世代化防重放（T04）

**用户故事：** 作为系统运维人员，我希望信令共振协议具备世代化防重放能力，以防止攻击者回放旧信令覆盖 Client 路由表。

#### 验收标准

1. THE Signal_Crypto SHALL 在 SignalPayload 中包含 Epoch（uint64）、ManifestID（16 字节）和 ExpireAt（int64）字段
2. WHEN Client 收到一条信令且其 Epoch 小于 Client 本地 CurrentEpoch, THE Client SHALL 丢弃该信令
3. WHEN Client 收到一条信令且其 Epoch 等于 Client 本地 CurrentEpoch, THE Client SHALL 要求该信令的 Timestamp 严格大于上一条已接受信令的 Timestamp
4. WHEN Client 收到一条信令且其 ExpireAt 早于当前时间, THE Client SHALL 丢弃该信令，即使验签通过
5. WHEN Client 重启, THE Client SHALL 从本地持久化文件加载 CurrentEpoch，确保重启后仍能拒绝旧世代信令
6. WHEN Resolver 通过任意通道（DoH、Gist、Mastodon）获取到信令, THE Resolver SHALL 在验签成功后执行 Epoch 校验

---

### 需求 5：Client 首次启动信任链（T05）

**用户故事：** 作为系统运维人员，我希望 Client 首次启动时不跳过 TLS 验证，以防止中间人攻击劫持 bootstrap 流程。

#### 验收标准

1. WHEN Client 执行 redeemFromURI 兑换 bootstrap 配置, THE Client SHALL 使用内嵌的 Root CA 证书验证服务端 TLS 证书，不使用 InsecureSkipVerify
2. WHEN QUICEngine 的 PinnedCertHash 为空, THE Client SHALL 使用 bootstrap 配置中的 CA 证书执行标准 TLS 验证，不跳过证书校验
3. WHEN 中间人替换了交付端点的 TLS 证书, THE Client SHALL 启动失败并报告证书验证错误
4. WHEN Client 通过已建立的 QUIC 连接拉取路由表, THE Client SHALL 验证路由表的签名，签名无效时拒绝更新 RuntimeTopology

---

### 需求 6：证书生命周期重构（T06）

**用户故事：** 作为系统运维人员，我希望 Gateway 节点不持有 CA 私钥且使用短期证书，以确保单节点失陷不会危及整个 PKI 体系。

#### 验收标准

1. THE OS SHALL 提供证书签发 API（`POST /internal/cert/sign`），接收 Gateway CSR 并签发有效期 24h~72h 的短期叶子证书
2. WHEN Gateway 需要续签证书, THE Gateway SHALL 生成 CSR 并向 OS 签发端提交请求，不在本地使用 CA 私钥签发
3. THE Gateway 部署产物 SHALL 不包含 ca.key 或 root-ca.key 文件
4. WHEN 节点证书自然过期, THE Gateway SHALL 自动向 OS 请求新证书，无需人工干预
5. IF Gateway 节点失陷, THEN THE PKI_System SHALL 依赖证书自然过期（不超过 72h）使失陷节点的证书失效，无需人工吊销

---

### 需求 7：eBPF 金丝雀挂载与自动摘钩（T07）

**用户故事：** 作为系统运维人员，我希望 eBPF 程序挂载前经过金丝雀验证且运行时有健康监测和自动摘钩能力，以防止异常 eBPF 程序拖垮整个数据面。

#### 验收标准

1. WHEN Gateway 启动并加载 eBPF 程序, THE Loader SHALL 先在 dummy0 虚拟接口执行 canary 挂载和自检，通过后再挂载到生产网卡
2. WHILE eBPF 程序运行中, THE Health_Checker SHALL 定期采样 SoftIRQ 统计、网卡丢包率和 ring buffer error 计数
3. WHEN 某个 eBPF 程序的健康指标超过预设阈值, THE Manager SHALL 自动 detach 该程序并切换到 iptables fallback，不影响其他程序
4. THE Manager SHALL 为每个 BPF 程序维护独立的健康状态标记，异常时只摘除单个程序
5. THE Gateway 启动日志 SHALL 明确输出 canary 校验结果，生产挂载前必须记录 canary passed

---

### 需求 8：生产日志脱敏与调试面收口（T08）

**用户故事：** 作为系统运维人员，我希望生产日志不包含完整的敏感信息且调试日志默认关闭，以防止日志泄露导致用户隐私暴露。

#### 验收标准

1. THE Gateway 和 OS SHALL 使用结构化日志库（Gateway 侧 zerolog 或 zap）替换所有 log.Printf 调用
2. THE Sanitizer SHALL 将 IP 地址截断为 /24 网段、userID 截断为前 8 位加省略号、token 和 address 只保留前 4 位
3. THE Logger SHALL 将日志分为三级：audit（审计日志，独立通道）、ops（运维日志，默认开启）、debug（仅开发模式开启）
4. WHILE 系统运行在生产模式, THE Logger SHALL 禁止 debug 级别日志落盘
5. WHEN monero_manager 输出日志, THE Sanitizer SHALL 处理所有包含用户 ID 和 XMR 地址的字段

---

### 需求 9：控制面授权链收口（T13）

**用户故事：** 作为系统运维人员，我希望控制面授权链严格区分普通用户和管理员，以防止任意绑定公钥的用户获取 admin 权限。

#### 验收标准

1. WHEN 用户通过 Breach 认证流程请求 admin token, THE Breach_Service SHALL 仅允许具有 operator 或 admin 角色的用户通过认证，普通用户即使绑定 Ed25519 公钥也被拒绝
2. THE Gateway_Bridge gRPC 服务 SHALL 使用 tls.RequireAndVerifyClientCert 强制 mTLS，不接受无客户端证书的连接
3. WHEN 生产环境的 grpc.tls_enabled 配置为 false, THE OS SHALL 在启动时返回配置校验错误并拒绝启动
4. WHEN Gateway 的 CommandSecret 为空, THE Gateway SHALL 在启动时直接失败，不允许无签名校验运行
5. THE Command_Auth SHALL 将 HMAC 签名覆盖范围扩展为 commandType + timestamp + nonce + SHA256(payload)
6. WHEN 同一条高危命令的 nonce 在 120 秒内已被使用, THE Command_Auth SHALL 拒绝该命令以防止重放

---

### 需求 10：封死业务侧直接加额度接口（T14）

**用户故事：** 作为系统运维人员，我希望普通用户无法通过业务 API 直接增加额度，以确保所有额度增长都可追溯到真实支付。

#### 验收标准

1. THE OS SHALL 将 `POST /billing/recharge` 端点限制为仅 admin 角色可调用，或直接下线该公开端点
2. THE Billing_Service SHALL 确保额度增加只能来自 monero_manager 的 confirmDeposit 内部链路
3. THE Database SHALL 在 QuotaPurchase 模型上增加 depositId 外键关联到 Deposit 记录
4. WHEN 普通用户调用 billing/recharge 端点, THE OS SHALL 返回 403 状态码

---

### 需求 11：敏感字段最小返回与审计脱敏（T15）

**用户故事：** 作为系统运维人员，我希望 API 响应不返回认证敏感字段且审计库不存储明文口令，以防止数据泄露扩大影响面。

#### 验收标准

1. THE Users_Service 的 findOne 方法 SHALL 使用显式 select 白名单，排除 passwordHash、totpSecret 和 ed25519Pubkey 字段
2. WHEN 审计拦截器记录 `/auth/*` 或 `/billing/*` 路径的请求体, THE Audit_Interceptor SHALL 移除 password、totpCode、token、secret、signature 等键后再写入 actionParams
3. THE Audit_Service SHALL 在 actionParams 写入前扫描并移除所有包含 password、token、totp、secret、key 的键
4. THE Command_Audit SHALL 仅记录命令摘要（如 type 和 level），不记录原始参数全文

---

## P1 需求（首批客户接入后两周内完成）

---

### 需求 12：会话级时序去相关（T09）

**用户故事：** 作为系统运维人员，我希望流量去相关在会话/成帧层实现而非仅在包级处理，以提升抗流量分析能力且不破坏传输层语义。

#### 验收标准

1. THE Session_Shaper SHALL 支持小批量聚合和定时 flush，窗口可配置（10-50ms）
2. WHEN Client 调用 GTunnelClient.Send, THE Session_Shaper SHALL 在发送前对业务片段输出节奏进行去相关处理
3. THE Gateway SHALL 实现对称的 Session_Shaper 以保持双向去相关一致性
4. WHERE 产品层级为 Standard, THE Session_Shaper SHALL 不启用去相关；WHERE 产品层级为 Platinum, THE Session_Shaper SHALL 使用 30ms 窗口；WHERE 产品层级为 Diamond, THE Session_Shaper SHALL 使用 50ms 窗口
5. THE Session_Shaper SHALL 不在 UDP/IP 层做随机乱序，避免触发 QUIC 重传

---

### 需求 13：Client 路径健康评分与劫持可疑判定（T10）

**用户故事：** 作为系统运维人员，我希望 Client 能持续评估链路健康状态并在可疑时采取防御动作，以在链路被劫持但未断开时提供早期预警。

#### 验收标准

1. THE Path_Health_Scorer SHALL 维护 RTT 基线（EWMA）、连续失败计数和切换频率
2. WHEN QUICEngine 建立连接, THE QUICEngine SHALL 从 QUIC ConnectionState 采样 SmoothedRTT 并上报给 Path_Health_Scorer
3. THE State_Machine SHALL 支持 StateSuspicious 状态，介于正常和断开之间
4. WHILE Client 处于 StateSuspicious 状态, THE Client SHALL 暂停敏感控制面写入（如 topo refresh）但不立刻切断业务流
5. WHEN 异常分数越过阈值, THE Client SHALL 触发候补路径评估并尝试切换到备用链路

---

### 需求 14：Client / Gateway 发布签名链（T11）

**用户故事：** 作为系统运维人员，我希望所有发布产物都有签名校验，以防止供应链攻击投毒二进制文件。

#### 验收标准

1. THE Release_System SHALL 为每次构建生成 ReleaseManifest，包含版本、构建时间、二进制 SHA-256 和 Ed25519 签名
2. WHEN Gateway 或 Client 启动, THE Binary SHALL 加载 manifest 并校验本地二进制 hash 与签名，校验失败时拒绝启动
3. WHEN 自动升级机制下载新版本, THE Updater SHALL 先验签再落盘，未签名版本拒绝加载
4. THE CI_Pipeline SHALL 在构建完成后自动生成 manifest 并用 Ed25519 私钥签名

---

### 需求 15：干净构建与供应链整洁化（T16）

**用户故事：** 作为系统运维人员，我希望发布流程基于干净构建且源码仓不混放产物，以确保发布可复现且不被本地开发环境污染。

#### 验收标准

1. THE Repository SHALL 在 .gitignore 中排除 `mirage-os/api-server/dist/` 和 `mirage-os/api-server/node_modules/`，并清理已提交的产物
2. THE Dockerfile SHALL 使用多阶段构建：builder 阶段 `npm ci --production`，运行阶段只拷贝 dist 和生产依赖
3. THE CI_Pipeline SHALL 使用干净环境和锁文件驱动构建，不允许开发机产物直接进入发布
4. THE Release_System SHALL 生成发布 manifest，记录源码提交 hash、package-lock.json hash 和产物 hash

---

### 需求 16：内部横向链路收口（T17）

**用户故事：** 作为系统运维人员，我希望内部服务间 HTTP 调用有严格的 host 白名单和重定向限制，以防止 SSRF 攻击利用内部服务做跳板。

#### 验收标准

1. WHEN 内部服务发起 HTTP 调用, THE Internal_HTTP_Client SHALL 校验目标 URL host 是否在白名单内（仅允许 127.0.0.1、localhost、::1 和明确配置的内网地址）
2. THE Internal_HTTP_Client SHALL 在所有内部 fetch 调用中设置 `redirect: 'error'` 禁止跟随重定向
3. WHEN rest/middleware 的 internal secret 为空, THE Middleware SHALL 拒绝所有请求，不允许降级放行
4. WHEN 任意 `*_URL` 环境变量的 host 不在白名单内, THE Service SHALL 在启动时校验失败并拒绝启动

---

### 需求 17：生产默认值 Fail-Closed（T18）

**用户故事：** 作为系统运维人员，我希望生产配置模板不包含开发回退值且缺少关键配置时服务拒绝启动，以防止因配置遗漏导致安全降级。

#### 验收标准

1. WHEN JWT_SECRET 环境变量未设置, THE Auth_Module SHALL 在模块初始化时直接抛出异常，不使用硬编码开发密钥回退
2. THE OS 配置 SHALL 将 grpc.tls_enabled 设为必填项，移除 false 默认值
3. THE Docker_Compose 生产模板 SHALL 将端口绑定限制为 127.0.0.1（如 `127.0.0.1:3000:3000`），不暴露到宿主机所有网卡
4. THE Deploy_System SHALL 提供独立的 docker-compose.dev.yml 保留开发便利配置，生产模板移除所有开发回退

---

## P2 需求（版本级演进项）

---

### 需求 18：云与运维安全 Runbook（T12）

**用户故事：** 作为系统运维人员，我希望有完整的生产运维安全 Runbook，以确保节点失陷时能在限定时间内完成替换且不扩大影响面。

#### 验收标准

1. THE Operations_Team SHALL 拥有生产节点密钥注入 Runbook，密钥不通过镜像固化或普通环境变量长期保存
2. THE Operations_Team SHALL 拥有云控制台、对象存储、CI、镜像仓库的最小权限模型文档
3. THE Operations_Team SHALL 拥有"节点失陷后替换流程"可演练 Runbook，可在限定时间内替换单个失陷节点
4. IF 单台 Gateway 失陷, THEN THE PKI_System SHALL 确保该节点不持有 Root 级签发材料，失陷不扩散到其他节点
