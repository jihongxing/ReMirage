---
Status: temporary
Target Truth: 运营前专项检查，不构成长期主源
Migration: 运营前攻击面审计，T01-T18 任务清单在首批客户接入前有效，完成后归档
---

# Mirage 运营前高频攻击面检查清单

## 1. 本次版本说明

这不是一份泛化的安全建议稿。

本版基于当前仓库代码做了一轮针对运营前高频攻击面的代码审计，然后把结果改写成可执行任务。结论分两类：

- 仓库里已经有原型，但没有形成上线闭环
- 仓库里基本缺失，需要在运营前补成明确实现

本次审计重点覆盖以下代码面：

- `mirage-os/api-server`
- `mirage-os/services/billing`
- `mirage-gateway/pkg/security`
- `mirage-gateway/pkg/ebpf`
- `mirage-gateway/pkg/gswitch`
- `phantom-client/pkg/gtclient`
- `phantom-client/pkg/resonance`
- `deploy/certs`
- `deploy/scripts`

---

## 2. 代码审计总览

### 已确认存在的基础能力

- `OS API` 已有 `JWT + RBAC + Audit` 基础框架，见 `mirage-os/api-server/src/modules/auth` 与 `mirage-os/api-server/src/modules/audit`
- `MoneroManager` 已有后台轮询与 `10` 确认块门槛，见 `mirage-os/services/billing/monero_manager.go`
- `SignalCrypto` 已有 `签名 + 加密 + TTL + timestamp` 的基础反重放能力，见 `mirage-gateway/pkg/gswitch/signal_crypto.go`
- `Gateway TLSManager` 已有证书热重载能力，见 `mirage-gateway/pkg/security/tls_manager.go`
- `Gateway eBPF Loader` 已有基础回滚与关闭清理能力，见 `mirage-gateway/pkg/ebpf/loader.go`
- `Gateway RAMShield / SecureEnclave` 已有 `mlock + wipe` 原型，见 `mirage-gateway/pkg/security/ram_shield.go` 与 `mirage-gateway/pkg/security/secure_enclave.go`

### 已确认存在的关键缺口

- `OS breach` 管理员入口当前会把任意绑定 `Ed25519` 公钥的活跃用户直接提升为 `admin`
- `gateway-bridge` 默认配置仍是 `gRPC TLS disabled`，且服务端实现不是严格 `mTLS`
- `XMR` 充值真相源仍然分裂在 `Webhook` 和 `Poller` 两条链路中
- 业务侧 `billing/recharge` 当前可以直接按请求体给自己加额度
- 充值入账没有形成 `tx_hash + transfer_index` 级别的幂等结算
- 邀请体系有闭环，但没有形成邀请树、连坐与反滥用收口
- Client 首次拉取配置仍允许 `InsecureSkipVerify`
- 信令有时间戳反重放，但没有 `Epoch / ExpireAt / 持久化世代` 闭环
- 证书仍是 `365 天叶子证书 + 节点侧持有 CA 私钥/签发能力` 的思路
- eBPF 只有加载失败降级，没有“挂载前金丝雀校验 + 运行时自动摘钩”
- 流量去相关仍主要停留在 `TC/XDP` 包级，不是会话/成帧级
- `users/findOne` 与全局审计拦截器会把 `passwordHash / totpSecret / 请求体敏感字段` 暴露到返回值或审计库
- `api-server` 当前源代码目录直接混有 `dist` 和 `node_modules`，发布链没有签名与可复现构建约束
- 生产日志里仍有大量 `IP / UserID / TokenID / Gateway` 级敏感输出
- 仓库内未看到完整的 `Client / Gateway` 发布签名与更新校验链

---

## 3. 优先级定义

- `P0`：正式接入第一批客户前必须完成
- `P1`：首批客户接入后两周内完成
- `P2`：版本级演进项，不阻塞首发，但应纳入路线图

---

## 4. 可执行任务清单

## 4.1 九类攻击面与任务映射

按本轮代码审计，运营前需要覆盖的 9 类攻击面与任务映射如下。

- `控制面被接管`：`T13`
- `密钥与令牌泄露`：`T06`、`T15`、`T18`
- `供应链与更新链路被投毒`：`T11`、`T16`
- `SSRF / 内网横向访问`：`T01`、`T17`
- `配额、订阅、结算被绕过`：`T02`、`T03`、`T14`
- `主动探测、重放与指纹归因`：`T04`、`T07`、`T09`、`T13`
- `Client 被劫持或更新链路被劫持`：`T05`、`T11`、`T16`
- `日志、监控、调试接口泄露`：`T08`、`T15`
- `云账号 / 运维侧失陷`：`T06`、`T12`、`T18`

补充说明：

- 你上一轮列举的 9 类里 `Client 被劫持或更新链路被劫持` 出现了重复，本次文档按完整上线视角补回了缺失的 `主动探测、重放与指纹归因`
- `SSRF` 方向在当前代码里没有发现明显的“用户可控 URL 抓取接口”，但已经存在多条默认信任的内部 HTTP 跳转链路，所以这里会落成“内部横向访问收口任务”

---

## T01 `P0` 统一 OS 内部入口的鉴权与网络边界

**攻击面映射**

- 控制面被接管
- SSRF 与内网横向访问
- 非正常来源伪造内部调用

**当前代码状态**

- `mirage-os/api-server/src/modules/auth` 已有 `JWT/RBAC`
- 但 `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts` 是裸 `POST /webhook/xmr/confirmed`
- `mirage-os/api-server/src/modules/billing/delivery.controller.ts` 会把一次性配置请求转发给内部 `PROVISIONER_URL`
- 这两条链路当前都没有看到服务间 `mTLS`、请求签名、来源白名单或重放保护

**执行任务**

- 给 `xmr-webhook.controller.ts` 和 `delivery.controller.ts` 增加统一的内部鉴权中间件
- 内部接口只允许来自固定内网地址、Unix socket 或 `mTLS` 客户端证书
- 如果短期先不上 `mTLS`，至少使用 `HMAC(timestamp + nonce + body)`，并做 `5` 分钟防重放窗口
- 把 `/webhook/*` 与 `/delivery/*` 从公网入口层分离，避免同一入口同时承接用户流量和内部控制流量

**涉及文件**

- `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts`
- `mirage-os/api-server/src/modules/billing/delivery.controller.ts`
- `mirage-os/api-server/src/main.ts`
- `mirage-os/api-server/src/modules/gateways/bridge-client.service.ts`

**验收标准**

- 未携带内部签名或无效客户端证书的请求一律返回 `401/403`
- 重放相同请求在窗口期内只允许通过一次
- 这两类内部接口不再直接暴露给公网入口

---

## T02 `P0` 把 XMR 充值结算收口成单一真相源

**攻击面映射**

- XMR 双花与竞态
- 配额、订阅、结算被绕过
- Webhook 伪造入账

**当前代码状态**

- `mirage-os/services/billing/monero_manager.go` 已有轮询钱包 RPC 和 `MinConfirmations = 10`
- 但 `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts` 仍可直接把 `deposit` 标记为 `CONFIRMED` 并给用户加额度
- `mirage-os/api-server/src/prisma/schema.prisma` 中 `Deposit.txHash` 只有单列唯一，没有 `transfer_index`
- `GenerateDepositAddress()` 仍是“用户一个地址”，不是“订单一个地址”

**执行任务**

- 废弃 Webhook 作为充值到账真相源，保留它最多只做“通知”，不做余额落账
- 改为 `wallet-rpc polling -> DB state machine -> quota sync` 单链路
- 充值记录升级为“订单级子地址”或“子地址 + transfer_index”模型
- `Deposit` 增加唯一键：`(tx_hash, transfer_index)` 或等价唯一结算键
- `confirmDeposit()` 事务里增加状态条件：只有 `PENDING -> CONFIRMED` 可落账

**涉及文件**

- `mirage-os/services/billing/monero_manager.go`
- `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts`
- `mirage-os/api-server/src/prisma/schema.prisma`

**验收标准**

- 同一笔链上转账无论轮询多少次，余额只能增加一次
- Webhook 即使被重复打或伪造，也不能改余额
- 数据库能明确追溯“哪一笔链上转账对应哪一笔订单入账”

---

## T03 `P0` 为邀请制补齐邀请树与反白嫖治理

**攻击面映射**

- 女巫攻击
- 免费试用滥用
- 邀请体系失控

**当前代码状态**

- `mirage-os/services/billing/invitation_service.go` 已有邀请码生成与 `FOR UPDATE` 核销
- `mirage-os/api-server/src/modules/auth/auth.service.ts` 注册必须使用邀请码
- 但 Prisma 侧只有 `InviteCode.createdBy / usedBy`，没有 `parent_invite_id / invite_root / lineage`
- 仓库内未见“观察期、连坐、风控分层、注册速率门槛”实现

**执行任务**

- 在用户和邀请码表上增加邀请树字段，至少能追溯三层：谁邀请了谁、根是谁、当前链路状态
- 新用户默认进入观察期，观察期内限制配额、并发会话数和节点等级
- 增加邀请树熔断能力：某一条邀请链异常时，可整链标记 `review` 或 `suspended`
- 如果仍保留试用额度，必须绑定设备、邀请码链和最小充值门槛，不能仅靠账号维度

**涉及文件**

- `mirage-os/services/billing/invitation_service.go`
- `mirage-os/api-server/src/modules/auth/auth.service.ts`
- `mirage-os/api-server/src/prisma/schema.prisma`

**验收标准**

- 可以从任意用户回溯完整邀请链
- 可以一键冻结某条邀请树
- 新账号无法仅靠批量注册反复薅试用流量

---

## T04 `P0` 把信令共振补成世代化防重放

**攻击面映射**

- 信令共振池污染
- Client 学到旧拓扑或假拓扑
- 被动回滚到已阵亡网关

**当前代码状态**

- `mirage-gateway/pkg/gswitch/signal_crypto.go` 已有 `Timestamp + TTL + Ed25519`
- 但当前反重放只依赖 `lastAcceptedTimestamp` 内存值
- `phantom-client/pkg/resonance/resolver.go` 和各通道实现中，没有看到 `Epoch / ManifestID / ExpireAt` 的客户端持久化校验
- Client 重启后，旧但签名合法的信令仍可能再次被接受

**执行任务**

- 在 `SignalPayload` 中补 `Epoch`、`ManifestID`、`ExpireAt`
- Client 本地持久化 `CurrentEpoch`，重启后继续拒绝旧世代信令
- Resolver 层增加“只接受更高 Epoch，或同 Epoch 且更高序号”的规则
- 发布链路增加短期有效期，过期信令即使验签通过也必须丢弃

**涉及文件**

- `mirage-gateway/pkg/gswitch/signal_crypto.go`
- `phantom-client/pkg/resonance/resolver.go`
- `phantom-client/pkg/resonance/doh.go`
- `phantom-client/pkg/resonance/gist.go`
- `phantom-client/pkg/resonance/mastodon.go`

**验收标准**

- 回放三天前的合法信令，Client 必须拒绝
- Client 重启后仍能拒绝旧信令
- 新信令发布后，旧 Epoch 不能再次覆盖路由表

---

## T05 `P0` 去掉 Client 首次启动的“不验证 TLS”

**攻击面映射**

- Client 被劫持
- 首次配置拉取被中间人替换
- 假 Gateway / 假交付端点接管

**当前代码状态**

- `phantom-client/cmd/phantom/main.go` 的 `redeemFromURI()` 首次拉配置时使用 `InsecureSkipVerify: true`
- `phantom-client/pkg/gtclient/quic_engine.go` 支持 `PinnedCertHash`，但未提供时直接跳过校验
- `phantom-client/pkg/gtclient/client.go` 的 `PullRouteTable()` 仍是空实现，说明 Client 还没有完整的受信拓扑学习闭环

**执行任务**

- 交付配置链路改为“内置 Root 公钥验签”或“内置 CA + 证书钉扎”，不能再裸跳过 TLS 校验
- 首次 bootstrap 必须至少验证以下之一：证书指纹、离线签名、根公钥签名的 manifest
- 实现 `PullRouteTable()`，让 Client 运行中持续学习受信节点，不依赖单次交付结果

**涉及文件**

- `phantom-client/cmd/phantom/main.go`
- `phantom-client/pkg/gtclient/quic_engine.go`
- `phantom-client/pkg/gtclient/client.go`

**验收标准**

- 中间人替换交付端点证书时，Client 启动必须失败
- 未签名或签名错误的 bootstrap 配置无法被接受
- Client 可在运行时更新受信网关池

---

## T06 `P0` 重构证书生命周期，移除节点侧 CA 私钥依赖

**攻击面映射**

- 密钥与令牌泄露
- 宿主机快照与冷启动提取
- 云账号与运维侧失陷

**当前代码状态**

- `mirage-gateway/pkg/security/tls_manager.go` 支持热重载，但只负责读文件
- `deploy/certs/gen_gateway_cert.sh` 和 `deploy/certs/gen_os_cert.sh` 都是 `365` 天证书
- `deploy/scripts/cert-rotate.sh` 依赖本机可访问 `root-ca.key`
- 这意味着节点侧仍有机会持有上级签发材料或具备长期离线签发路径

**执行任务**

- 改成 `OS 或独立 Signer` 签发 `24h~72h` 短期叶子证书
- Gateway 节点只保留短期叶子证书，不保留 `CA key / root-ca.key`
- 证书轮换改成“节点向签发端请求新证书”，而不是在节点本地直接签
- 把当前基于脚本的本地轮换保留为开发环境方案，不进入生产路径

**涉及文件**

- `mirage-gateway/pkg/security/tls_manager.go`
- `deploy/certs/gen_gateway_cert.sh`
- `deploy/certs/gen_os_cert.sh`
- `deploy/scripts/cert-rotate.sh`

**验收标准**

- 任意 Gateway 节点上都找不到 `CA 私钥`
- 节点证书自然过期时间不超过 `72h`
- 节点失陷后无需人工吊销长期证书即可自然失效

---

## T07 `P0` 给 eBPF 挂载链路增加金丝雀与自动摘钩

**攻击面映射**

- 畸形包触发 eBPF 崩溃
- 内核软中断飙升
- 上线后因挂载异常把整台网关拖死

**当前代码状态**

- `mirage-gateway/pkg/ebpf/loader.go` 具备加载、关闭、Map 回滚
- `mirage-gateway/pkg/ebpf/manager.go` 有 `emergency_ctrl_map` 的紧急降级
- 但没有看到“先挂到假接口/金丝雀接口验证，再挂生产网卡”的逻辑
- 也没有看到“运行时异常时自动 `detach XDP/TC` 并切换到保底过滤器”的闭环

**执行任务**

- 在 Loader 前增加 canary attach 流程，先挂虚拟接口或 staging 网卡跑自检
- 建立运行时健康监测：SoftIRQ、丢包、attach error、ring buffer error 达阈值时自动摘钩
- 明确 `pass-through` 和 `iptables fallback` 的切换条件，不允许仅写日志
- 给每个可选 BPF 程序增加独立健康状态，避免一个组件异常拖垮整套数据面

**涉及文件**

- `mirage-gateway/pkg/ebpf/loader.go`
- `mirage-gateway/pkg/ebpf/manager.go`
- `mirage-gateway/pkg/ebpf/monitor.go`
- `mirage-gateway/pkg/threat/blacklist.go`

**验收标准**

- 畸形包回归测试下，Gateway 不因 eBPF 挂载而死机
- 触发异常阈值后，系统能自动摘掉有问题的程序并保持基本转发
- 生产启动日志能明确看到 canary 校验通过后才进入真实挂载

---

## T08 `P0` 生产日志脱敏与调试面收口

**攻击面映射**

- 日志、监控、调试接口泄露
- Token、IP、用户标识被侧信道暴露

**当前代码状态**

- Gateway 侧大量 `log.Printf` 直接输出 `IP / userID / tokenID / gateway / path`
- 例如 `pkg/api/handlers.go`、`pkg/phantom/honeypot.go`、`pkg/phantom/reporter.go`、`pkg/ebpf/monitor.go`
- 仓库内未看到“生产日志级别配置 + 结构化脱敏器 + 安全审计日志分流”闭环

**执行任务**

- 把运营日志分成三类：审计日志、运维日志、调试日志
- 默认生产模式下禁止输出完整 `IP / tokenID / userID / cert hash`
- 对外部事件只保留截断值或哈希摘要
- 明确调试日志只能在开发模式开启，且不能默认落盘

**涉及文件**

- `mirage-gateway/pkg/api/handlers.go`
- `mirage-gateway/pkg/phantom/honeypot.go`
- `mirage-gateway/pkg/phantom/reporter.go`
- `mirage-gateway/pkg/ebpf/monitor.go`
- `mirage-gateway/pkg/security/*`

**验收标准**

- 生产日志中检索不到完整 Token、完整用户标识和完整 IP
- 调试模式和生产模式日志行为可配置且默认安全
- 审计日志与普通运行日志分离存放

---

## T09 `P1` 把时序去相关上移到会话/成帧层

**攻击面映射**

- 全局流量相关性分析
- 时序水印
- “局部合理、整体怪异”的流量特征

**当前代码状态**

- `mirage-gateway/bpf/jitter.c` 与 `mirage-gateway/bpf/h3_shaper.c` 当前主要做的是 `padding / delay / CID` 级处理
- 这些控制基本都在 `TC/XDP` 包级
- 仓库内未见“业务流分帧、攒包、定时 flush、会话级去相关”的明确实现

**执行任务**

- 在 `G-Tunnel` 成帧层增加小批量聚合与定时排空
- 让去相关作用于“业务片段输出节奏”，而不是直接打乱底层传输语义
- 不在 UDP/IP 层做随机乱序，避免反向触发 QUIC/TCP 重传模式
- 为不同产品层级提供不同去相关档位，避免高成本策略全量开启

**涉及文件**

- `mirage-gateway/bpf/jitter.c`
- `mirage-gateway/bpf/h3_shaper.c`
- `phantom-client/pkg/gtclient/client.go`
- `mirage-gateway/pkg/gtunnel/*`

**验收标准**

- 新实现不破坏 QUIC/TCP 语义
- 可以单独调节批量窗口、flush 周期、填充比例
- 在回归测试里可验证去相关策略不会明显放大重传和断流

---

## T10 `P1` 增加 Client 侧路径健康评分与劫持可疑判定

**攻击面映射**

- BGP 劫持
- 路由绕行
- 伪正常但高度可疑的链路变化

**当前代码状态**

- `mirage-gateway/pkg/gswitch/asn_shield.go` 只在 Gateway 侧有 ASN 白名单与 RTT 异常原型
- `phantom-client` 侧没有看到 `RTT + hop/TTL + baseline` 的健康评分器
- 当前 Client 侧状态机也没有“可疑链路挂起敏感控制写入”的策略

**执行任务**

- 在 Client 增加路径健康评分器，维度至少包括 `RTT 漂移、连续失败、切换频率、可选 hop/TTL 变化`
- 对“可疑但未完全断开”的链路，不立刻切业务流，但要暂停敏感控制面写入
- 把异常评分结果纳入重连与拓扑学习决策

**涉及文件**

- `phantom-client/pkg/gtclient/client.go`
- `phantom-client/pkg/gtclient/state.go`
- `phantom-client/pkg/gtclient/quic_engine.go`
- `mirage-gateway/pkg/gswitch/asn_shield.go`

**验收标准**

- Client 能维持自己的链路基线
- RTT 轻微抖动不误报
- 异常分数越界后，Client 会暂停敏感控制动作并触发候补路径评估

---

## T11 `P1` 建立 Client / Gateway 发布签名链

**攻击面映射**

- 供应链投毒
- 更新链路被劫持
- 客户端或网关被下发恶意版本

**当前代码状态**

- 仓库内未看到完整的 release manifest、产物签名校验、升级验签闭环
- Client 侧也未见“只接受受信签名更新”的逻辑

**执行任务**

- 增加 release manifest，记录版本、构建时间、二进制哈希、签名
- Gateway 与 Client 启动时都能校验本地产物是否匹配签名 manifest
- CI 产物必须附带不可变摘要和签名
- 任何自动升级机制都必须先验签再落盘

**涉及文件**

- `phantom-client/*`
- `mirage-gateway/*`
- `deploy/*`
- `CI/CD 配置`

**验收标准**

- 篡改二进制后，启动自检能明确失败
- 发布产物都能追溯到单一签名 manifest
- 自动升级不能加载未签名版本

---

## T12 `P2` 把宿主机与云运维风险单列成上线前操作项

**攻击面映射**

- 宿主机快照
- 云账号接管
- 冷启动提取
- 运维侧连带泄露

**当前代码状态**

- 代码层已有 `RAMShield / SecureEnclave / Wipe` 原型
- 但这些能力解决不了“云控制台、镜像、快照、CA 文件、CI 凭据”层面的运维风险
- 当前生产证书脚本仍默认在文件系统中操作长期材料

**执行任务**

- 明确生产节点的密钥注入方式，不通过镜像固化和普通环境变量长期保存
- 为云控制台、对象存储、CI、镜像仓库建立单独最小权限模型
- 关闭不必要的自动快照、调试控制台和长期 API Key
- 把“节点失陷后的替换流程”写成可演练 Runbook，而不是临场处理

**涉及范围**

- 云 IAM
- CI/CD
- 镜像仓库
- 备份与快照
- 生产环境变量管理

**验收标准**

- 运维人员可按 Runbook 在限定时间内替换单个失陷节点
- 任一单台 Gateway 失陷不会暴露 Root 级签发材料
- 生产环境不存在长期明文 CA 私钥和长期共享管理员密钥

---

## T13 `P0` 收口控制面授权链，移除“任意公钥用户即管理员”的路径

**攻击面映射**

- 控制面被接管
- 主动探测、重放与指令伪造

**当前代码状态**

- `mirage-os/api-server/src/modules/auth/breach.service.ts` 当前会遍历所有 `ed25519Pubkey != null` 的活跃用户，只要验签通过就签发 `role=admin`
- `mirage-os/api-server/src/modules/auth/auth.controller.ts` 公开暴露 `/auth/challenge` 与 `/auth/breach`
- `mirage-os/gateway-bridge/pkg/grpc/server.go` 当前 TLS 仅使用 `credentials.NewServerTLSFromFile()`，没有 `RequireAndVerifyClientCert`
- `mirage-os/configs/mirage-os.yaml` 默认还是 `grpc.tls_enabled: false`
- `mirage-gateway/cmd/gateway/main.go` 中只有在 `command_secret` 非空时才启用高危下行命令签名校验
- `mirage-gateway/pkg/api/command_auth.go` 的 HMAC 只覆盖 `commandType + timestamp`，不覆盖 payload 和 nonce

**执行任务**

- 将 `breach` 入口改成“仅允许专门的 operator/admin 身份使用”，不能再复用普通用户公钥表
- `gateway-bridge` gRPC 改为严格 `mTLS`，服务端必须校验客户端证书
- 生产配置移除 `grpc.tls_enabled: false` 默认值，缺失 TLS 直接启动失败
- 高危下行命令把签名校验改成强制启用，而不是“有 secret 才启用”
- 下行命令签名改为覆盖 `commandType + timestamp + nonce + payloadDigest`，并增加短窗口重放缓存

**涉及文件**

- `mirage-os/api-server/src/modules/auth/breach.service.ts`
- `mirage-os/api-server/src/modules/auth/auth.controller.ts`
- `mirage-os/gateway-bridge/pkg/grpc/server.go`
- `mirage-os/gateway-bridge/pkg/config/config.go`
- `mirage-os/configs/mirage-os.yaml`
- `mirage-gateway/pkg/api/command_auth.go`
- `mirage-gateway/cmd/gateway/main.go`

**验收标准**

- 普通用户即使绑定 `Ed25519` 公钥，也不能拿到 `admin` token
- `gateway-bridge` 不接受无客户端证书的 uplink 连接
- 生产环境缺少 `command_secret` 或等效签名材料时，Gateway 启动失败
- 同一条高危命令被重放时，第二次必须被拒绝

---

## T14 `P0` 封死业务侧“直接加额度”接口

**攻击面映射**

- 配额、订阅、结算被绕过

**当前代码状态**

- `mirage-os/api-server/src/modules/billing/billing.controller.ts` 暴露了 `POST /billing/recharge`
- `mirage-os/api-server/src/modules/billing/billing.service.ts` 当前会直接根据请求体中的 `quotaGb` 和 `price` 创建 `quotaPurchase`，并把 `remainingQuota` 与 `totalDeposit` 直接递增
- 这条链路没有支付凭证、没有订单状态、没有内部签名，也没有与 XMR 流程绑定

**执行任务**

- 下线或内部化 `billing/recharge`
- 额度增加只能来自“支付确认后内部结算”链路，不能来自普通用户 API
- 把 `quotaPurchase` 改成支付后结果表，不再作为用户自报输入写入
- 为历史数据补审计标记，区分真实支付产生的额度和测试/手工数据

**涉及文件**

- `mirage-os/api-server/src/modules/billing/billing.controller.ts`
- `mirage-os/api-server/src/modules/billing/billing.service.ts`

**验收标准**

- 普通用户无法通过业务 API 直接增加任何额度
- 所有额度增长都能追溯到支付订单或内部审计操作
- 回归测试验证 `billing/recharge` 不再具备直接记账能力

---

## T15 `P0` 敏感字段最小返回，并停止把认证数据写进审计库

**攻击面映射**

- 密钥与令牌泄露
- 日志、监控、调试接口泄露

**当前代码状态**

- `mirage-os/api-server/src/modules/users/users.service.ts` 的 `findOne()` 使用 `findUnique(... include: { cell: true })`，会返回完整用户记录，默认包含 `passwordHash`、`totpSecret`
- `mirage-os/api-server/src/modules/audit/audit-interceptor.ts` 会把所有敏感 `POST/PUT/PATCH/DELETE` 请求的 `request.body` 直接写入 `actionParams`
- 这会把 `auth/register`、`auth/login`、`auth/breach/validate` 之类请求里的密码、TOTP、token 原样打进审计库
- `mirage-gateway/pkg/api/command_audit.go` 也会把命令参数长期保存在环形审计缓冲区

**执行任务**

- 所有 `findOne/getDetail` 类接口改成显式字段白名单，不返回 `passwordHash`、`totpSecret`、内部密钥材料
- `AuditInterceptor` 对认证、计费、内部控制接口做参数脱敏或完全不记录 body
- `AuditService` 增加字段级红线，拒绝落库包含 `password/token/totp/secret/key` 的敏感键
- Gateway 命令审计只记录摘要，不记录原始参数

**涉及文件**

- `mirage-os/api-server/src/modules/users/users.service.ts`
- `mirage-os/api-server/src/modules/audit/audit-interceptor.ts`
- `mirage-os/api-server/src/modules/audit/audit.service.ts`
- `mirage-gateway/pkg/api/command_audit.go`

**验收标准**

- 任意用户详情接口都不再返回 `passwordHash` 和 `totpSecret`
- 审计库中检索不到认证口令、TOTP、JWT、decrypt key 等明文
- Gateway 命令审计只保留必要摘要与结果状态

---

## T16 `P1` 把发布链改成干净构建，不再混放 `dist/node_modules`

**攻击面映射**

- 供应链与更新链路被投毒
- Client 被劫持或更新链路被劫持

**当前代码状态**

- `mirage-os/api-server` 当前工作树直接存在 `dist` 和 `node_modules`
- `mirage-os/api-server/Dockerfile` 在 builder 阶段 `npm install` 后，直接把整份 `node_modules` 拷贝到运行镜像
- 仓库中未见“构建产物摘要、发布 manifest、签名校验、可复现构建”闭环

**执行任务**

- 把 `dist`、`node_modules` 明确排除出源码审阅和发布基线，不让运行产物与源码长期混放
- CI 构建改为干净环境 `npm ci` / 锁文件驱动，不允许开发机产物直接进入发布
- 生成发布 manifest，记录源码提交、锁文件哈希、产物哈希
- 把签名发布链与 `T11` 合并，实现网关和客户端的产物验签

**涉及文件**

- `mirage-os/api-server/Dockerfile`
- `mirage-os/api-server/package.json`
- `mirage-os/api-server/package-lock.json`
- 仓库发布流程与 CI/CD 配置

**验收标准**

- 源码仓与运行产物边界清晰
- 任意一次发布都能从锁文件和源码提交重建出同样的产物哈希
- 运行环境只接受 CI 产物，不接受本地手工产物

---

## T17 `P1` 收口内部横向链路，避免“默认可信的 HTTP 跳板”

**攻击面映射**

- SSRF / 内网横向访问
- 控制面被接管

**当前代码状态**

- 本轮代码审计没有发现明显的“用户可控 URL 取数接口”
- 但仓库内已经存在多条默认信任的内部 HTTP 链路：
- `mirage-os/api-server/src/modules/billing/delivery.controller.ts` 通过 `PROVISIONER_URL` 调内部交付接口
- `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts` 通过 `PROVISIONER_URL` 调内部 provision 接口
- `mirage-os/api-server/src/modules/gateways/bridge-client.service.ts` 通过 `BRIDGE_URL` 调 `gateway-bridge`
- `mirage-os/gateway-bridge/pkg/rest/middleware.go` 目前只靠静态 `X-Internal-Secret`
- 这些链路大多基于明文 HTTP，且 host 约束和跳转约束较弱

**执行任务**

- 对所有内部 `*_URL` 配置增加 host allowlist，只允许 `127.0.0.1`、Unix socket 或明确内网地址
- 禁止内部 HTTP 客户端跟随跨域重定向
- `gateway-bridge` 内部 REST 从静态 header 升级到 `mTLS` 或至少 `HMAC + timestamp + nonce`
- 未来新增任何出站取数能力时，必须经过统一的内网访问策略层，而不是模块自带 `fetch`

**涉及文件**

- `mirage-os/api-server/src/modules/billing/delivery.controller.ts`
- `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts`
- `mirage-os/api-server/src/modules/gateways/bridge-client.service.ts`
- `mirage-os/gateway-bridge/pkg/rest/middleware.go`
- `mirage-os/gateway-bridge/cmd/bridge/main.go`

**验收标准**

- 内部服务地址不在 allowlist 时，服务启动即失败
- 内部 HTTP 客户端不接受重定向到非白名单地址
- 内部 REST 不再只依赖一个静态共享 header

---

## T18 `P1` 去掉生产默认值和开发回退，把部署模板改成 Fail-Closed

**攻击面映射**

- 密钥与令牌泄露
- 云账号 / 运维侧失陷
- 控制面被接管

**当前代码状态**

- `mirage-os/api-server/src/modules/auth/auth.module.ts` 仍内置 `dev_jwt_secret_change_in_production`
- `mirage-os/configs/mirage-os.yaml` 默认 `grpc.tls_enabled: false`
- `deploy/docker-compose.os.yml` 默认直接暴露 `3000` 和 `50847`
- `deploy/certs/*` 与 `deploy/scripts/cert-rotate.sh` 仍以本地持有 `ca.key/root-ca.key` 为前提

**执行任务**

- 把所有生产相关模板改成“没有密钥就无法启动”，不再提供开发级回退默认值
- 分离 `dev` 与 `prod` 配置模板，避免生产运维直接从开发模板演化
- 对外暴露端口改成显式选择，而不是 Compose 默认暴露
- 与 `T06/T12` 对齐，删除生产节点依赖本地 `CA key` 的脚本路径

**涉及文件**

- `mirage-os/api-server/src/modules/auth/auth.module.ts`
- `mirage-os/configs/mirage-os.yaml`
- `deploy/docker-compose.os.yml`
- `deploy/certs/gen_gateway_cert.sh`
- `deploy/certs/gen_os_cert.sh`
- `deploy/scripts/cert-rotate.sh`

**验收标准**

- 生产模板中不存在开发密钥回退值
- 不配置 TLS、JWT、内部鉴权材料时，服务直接拒绝启动
- 生产发布模板不会默认把控制面端口暴露到宿主机外部

---

## 5. 建议的执行顺序

### 第一批必须落地

- `T01` 内部接口鉴权
- `T02` XMR 单一真相源
- `T03` 邀请树与反白嫖
- `T04` 信令世代化防重放
- `T05` Client 启动信任链
- `T06` 短期证书体系
- `T07` eBPF 金丝雀与自动摘钩
- `T08` 生产日志脱敏
- `T13` 控制面授权链收口
- `T14` 封死直接加额度接口
- `T15` 敏感字段最小返回与审计脱敏

### 第二批紧随其后

- `T09` 会话级去相关
- `T10` Client 路径健康评分
- `T11` 发布签名链
- `T16` 干净构建与供应链整洁化
- `T17` 内部横向链路收口
- `T18` 生产默认值 Fail-Closed

### 第三批并行推进

- `T12` 云与运维 Runbook

---

## 6. 结论

当前 Mirage 的问题不是“完全没有安全能力”，而是：

- 已经有不少强能力原型
- 但这些能力还没有被收口成能承受真实运营攻击面的闭环

如果按上线标准看，当前最需要优先解决的不是再继续扩充协议数量，而是先补齐以下三条主线：

- 钱和配额不能被重复核销
- Client 不能在首次启动和共振恢复时被劫持
- 节点失陷、日志泄露、eBPF 异常不能把整套系统一起拖下水

补充到这轮 9 类审计后，可以再更直白一点：

- 管理入口不能再存在“普通用户验签成功即管理员”的提升路径
- 业务 API 不能再存在“请求一次就直接加额度”的记账旁路
- 审计体系本身不能继续成为密码、TOTP、JWT 和内部参数的二次泄露源

这份文档后续应继续作为运营前的执行母表维护，而不是停留在建议层。
