# 实施计划：运营前安全加固（Pre-Launch Security Hardening）

## 概述

基于 18 项安全审计发现，按 P0 → P1 → P2 优先级排序，将设计方案转化为可执行的编码任务。P0 任务按依赖关系排序：先收口最高风险的控制面授权和资金安全，再逐步覆盖信令、证书、eBPF 和日志。

## 任务

---

### P0：首批客户前必须完成

---

- [x] 1. 控制面授权链收口（T13）
  - [x] 1.1 `breach.service.ts` 增加角色过滤，新增 `isOperator` 字段或 `OperatorKey` 模型
    - 修改 `findMany` 查询条件，只允许 operator/admin 角色用户通过 Breach 认证
    - 修改 `mirage-os/api-server/src/modules/auth/breach.service.ts`
    - 修改 `mirage-os/api-server/src/prisma/schema.prisma`（User 增加 `isOperator Boolean @default(false)`）
    - _需求: 9.1_

  - [x]* 1.2 编写 Breach 角色过滤属性测试
    - **Property 14: Breach 认证角色过滤**
    - 使用 fast-check 验证非 operator 用户即使有公钥也无法获取 admin token
    - **验证: 需求 9.1**

  - [x] 1.3 `gateway-bridge/pkg/grpc/server.go` TLS 改为 `tls.RequireAndVerifyClientCert`
    - 使用 `tls_manager.go` 的 `GetServerTLSConfig()` 方法
    - 修改 `mirage-os/gateway-bridge/pkg/grpc/server.go`
    - _需求: 9.2_

  - [x] 1.4 `config.go` 增加生产模式校验：`TLSEnabled` 为 false 时返回错误
    - 修改 `mirage-os/gateway-bridge/pkg/config/config.go`
    - 修改 `mirage-os/configs/mirage-os.yaml` 移除 `tls_enabled: false` 默认值
    - _需求: 9.3_

  - [x] 1.5 Gateway `CommandSecret` 为空时启动失败
    - 修改 `mirage-gateway/cmd/gateway/main.go`，移除 `if cfg.Security.CommandSecret != ""` 条件
    - _需求: 9.4_

  - [x] 1.6 `command_auth.go` HMAC 覆盖范围扩展 + nonce 重放缓存
    - HMAC 签名覆盖：`commandType + timestamp + nonce + SHA256(payload)`
    - 新增内存 LRU nonce 缓存，TTL 120s
    - 修改 `mirage-gateway/pkg/api/command_auth.go`
    - _需求: 9.5, 9.6_

  - [x]* 1.7 编写 Gateway 命令签名完整性与防重放属性测试
    - **Property 15: Gateway 命令签名完整性与防重放**
    - 使用 rapid 验证修改任意字段导致 HMAC 失败，同一 nonce 120s 内被拒绝
    - **验证: 需求 9.5, 9.6**

- [x] 2. 检查点 - 控制面授权链
  - 确保所有测试通过，如有疑问请向用户确认。

- [x] 3. 封死业务侧直接加额度接口（T14）
  - [x] 3.1 `billing.controller.ts` 增加 `@Roles('admin')` + `@UseGuards(RolesGuard)`
    - 修改 `mirage-os/api-server/src/modules/billing/billing.controller.ts`
    - _需求: 10.1, 10.4_

  - [x] 3.2 `billing.service.ts` 的 `recharge()` 限制为仅内部链路调用
    - 额度增加只能来自 `monero_manager.go` 的 `confirmDeposit()`
    - 修改 `mirage-os/api-server/src/modules/billing/billing.service.ts`
    - _需求: 10.2_

  - [x] 3.3 `schema.prisma` QuotaPurchase 增加 `depositId` 外键关联
    - 修改 `mirage-os/api-server/src/prisma/schema.prisma`
    - _需求: 10.3_

  - [x]* 3.4 编写 Recharge 端点角色限制属性测试
    - **Property 16: Recharge 端点角色限制**
    - 使用 fast-check 验证非 admin 用户调用返回 403
    - **验证: 需求 10.1**

- [x] 4. OS 内部接口鉴权与网络边界隔离（T01）
  - [x] 4.1 新增 `InternalHMACGuard` 中间件
    - 校验 `HMAC-SHA256(timestamp + nonce + SHA256(body))`，5 分钟防重放窗口
    - 新增 `mirage-os/api-server/src/common/internal-hmac.guard.ts`
    - _需求: 1.1, 1.2, 1.3_

  - [x]* 4.2 编写 HMAC Guard 属性测试
    - **Property 1: HMAC Guard 拒绝无效请求**
    - 使用 fast-check 验证无效签名、过期时间戳、重复 nonce 均被拒绝
    - **验证: 需求 1.1, 1.2, 1.3**

  - [x] 4.3 给 `XMRWebhookController` 和 `DeliveryController` 挂载 `InternalHMACGuard`
    - 修改 `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts`
    - 修改 `mirage-os/api-server/src/modules/billing/delivery.controller.ts`
    - _需求: 1.1_

  - [x] 4.4 `DeliveryController` 内部 fetch 调用增加 HMAC 签名 header
    - 修改 `mirage-os/api-server/src/modules/billing/delivery.controller.ts`
    - _需求: 1.5_

  - [x] 4.5 `BridgeClientService` 的 `BRIDGE_INTERNAL_SECRET` 为空时启动抛异常
    - 修改 `mirage-os/api-server/src/modules/gateways/bridge-client.service.ts`
    - _需求: 1.6_

  - [x] 4.6 将 `/webhook/*` 和 `/delivery/*` 从公网 `/api` 前缀分离
    - 绑定到独立的内部监听端口或 Unix socket
    - 修改 `mirage-os/api-server/src/main.ts`
    - _需求: 1.4_

- [x] 5. XMR 充值结算单一真相源（T02）
  - [x] 5.1 `xmr-webhook.controller.ts` 废弃余额落账逻辑，只保留 WebSocket 通知
    - 修改 `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts`
    - _需求: 2.1_

  - [x] 5.2 `monero_manager.go` 的 `confirmDeposit()` 增加 `WHERE status = 'pending'` 条件
    - 确保只有 `PENDING -> CONFIRMED` 可落账
    - 修改 `mirage-os/services/billing/monero_manager.go`
    - _需求: 2.2, 2.3_

  - [x]* 5.3 编写充值落账幂等性属性测试
    - **Property 2: 充值落账幂等性**
    - 使用 rapid 验证 confirmDeposit 调用 N 次余额只增加一次
    - **验证: 需求 2.2, 2.3**

  - [x] 5.4 `schema.prisma` Deposit 增加 `transferIndex`，唯一键改为 `@@unique([txHash, transferIndex])`
    - 修改 `mirage-os/api-server/src/prisma/schema.prisma`
    - _需求: 2.4_

  - [x] 5.5 `GenerateDepositAddress()` 改为订单级子地址模型
    - 每次充值请求生成新子地址并关联到 Deposit 记录
    - 修改 `mirage-os/services/billing/monero_manager.go`
    - _需求: 2.5_

  - [x]* 5.6 编写充值子地址唯一性属性测试
    - **Property 3: 充值子地址唯一性**
    - 使用 rapid 验证同一用户 N 次充值请求生成 N 个不同子地址
    - **验证: 需求 2.5**

- [x] 6. 检查点 - 资金安全
  - 确保所有测试通过，如有疑问请向用户确认。

- [x] 7. 敏感字段最小返回与审计脱敏（T15）
  - [x] 7.1 `users.service.ts` 的 `findOne()` 改为显式 `select` 白名单
    - 排除 `passwordHash`、`totpSecret`、`ed25519Pubkey`
    - 修改 `mirage-os/api-server/src/modules/users/users.service.ts`
    - _需求: 11.1_

  - [x]* 7.2 编写用户详情敏感字段排除属性测试
    - **Property 17: 用户详情敏感字段排除**
    - 使用 fast-check 验证 findOne 返回不包含敏感字段
    - **验证: 需求 11.1**

  - [x] 7.3 `audit-interceptor.ts` 增加路径级脱敏规则
    - 对 `/auth/*`、`/billing/*` 路径移除 password、totpCode、token、secret、signature 等键
    - 修改 `mirage-os/api-server/src/modules/audit/audit-interceptor.ts`
    - _需求: 11.2_

  - [x] 7.4 `audit.service.ts` 增加字段级红线过滤
    - `actionParams` 写入前扫描并移除敏感键
    - 修改 `mirage-os/api-server/src/modules/audit/audit.service.ts`
    - _需求: 11.3_

  - [x]* 7.5 编写审计日志敏感键过滤属性测试
    - **Property 18: 审计日志敏感键过滤**
    - 使用 fast-check 验证包含敏感键的请求体经处理后不再包含这些键
    - **验证: 需求 11.2, 11.3**

  - [x] 7.6 `command_audit.go` 的 `Params` 只记录摘要
    - 不记录原始参数全文，只保留 type 和 level
    - 修改 `mirage-gateway/pkg/api/command_audit.go`
    - _需求: 11.4_

  - [x]* 7.7 编写命令审计摘要化属性测试
    - **Property 19: 命令审计摘要化**
    - 使用 rapid 验证审计日志 Params 仅包含摘要
    - **验证: 需求 11.4**

- [x] 8. Client 首次启动信任链（T05）
  - [x] 8.1 新增 `embed/ca.pem` 内嵌 Root CA 证书
    - 新增 `phantom-client/embed/ca.pem`
    - _需求: 5.1_

  - [x] 8.2 `redeemFromURI()` 移除 `InsecureSkipVerify`，使用内嵌 CA 验签
    - 修改 `phantom-client/cmd/phantom/main.go`
    - _需求: 5.1, 5.3_

  - [x] 8.3 `QUICEngine` PinnedCertHash 为空时使用 CA 证书做标准 TLS 验证
    - 修改 `phantom-client/pkg/gtclient/quic_engine.go`
    - _需求: 5.2_

  - [x] 8.4 实现 `PullRouteTable()`：拉取签名路由表并验签
    - 修改 `phantom-client/pkg/gtclient/client.go`
    - _需求: 5.4_

  - [x]* 8.5 编写路由表签名验证属性测试
    - **Property 10: 路由表签名验证**
    - 使用 rapid 验证篡改载荷任意字节导致签名验证失败
    - **验证: 需求 5.4**

- [x] 9. 信令共振世代化防重放（T04）
  - [x] 9.1 `signal_crypto.go` SignalPayload 增加 Epoch、ManifestID、ExpireAt 字段
    - 更新 `SerializePayload` / `DeserializePayload`
    - 修改 `mirage-gateway/pkg/gswitch/signal_crypto.go`
    - _需求: 4.1_

  - [x] 9.2 `OpenSignal()` 增加 Epoch 校验和 ExpireAt 校验
    - Epoch < CurrentEpoch → 丢弃；同 Epoch 要求更高 Timestamp；过期丢弃
    - 修改 `mirage-gateway/pkg/gswitch/signal_crypto.go`
    - _需求: 4.2, 4.3, 4.4_

  - [x]* 9.3 编写信令世代化防重放属性测试
    - **Property 8: 信令世代化防重放**
    - 使用 rapid 验证旧 Epoch、同 Epoch 旧 Timestamp、过期信令均被丢弃
    - **验证: 需求 4.2, 4.3, 4.4**

  - [x] 9.4 Client 侧持久化 CurrentEpoch 到 `~/.phantom-client/epoch`
    - 新增 `phantom-client/pkg/persist/` 持久化模块
    - _需求: 4.5_

  - [x]* 9.5 编写 Epoch 持久化 Round-Trip 属性测试
    - **Property 9: Epoch Round-Trip**
    - 使用 rapid 验证写入后读取得到相同值
    - **验证: 需求 4.5**

  - [x] 9.6 `resolver.go` 增加 `validateEpoch()` 方法
    - 在 DoH、Gist、Mastodon 各通道验签后执行 Epoch 校验
    - 修改 `phantom-client/pkg/resonance/resolver.go`
    - 修改 `phantom-client/pkg/resonance/doh.go`
    - 修改 `phantom-client/pkg/resonance/gist.go`
    - 修改 `phantom-client/pkg/resonance/mastodon.go`
    - _需求: 4.6_

- [x] 10. 证书生命周期重构（T06）
  - [x] 10.1 OS 侧新增证书签发 API `POST /internal/cert/sign`
    - 接收 Gateway CSR，签发 24h~72h 短期叶子证书
    - 新增 `mirage-os/api-server/src/modules/certs/cert-sign.controller.ts`
    - 挂载 `InternalHMACGuard` + mTLS
    - _需求: 6.1_

  - [x]* 10.2 编写短期证书有效期约束属性测试
    - **Property 11: 短期证书有效期约束**
    - 使用 fast-check 验证签发证书有效期在 24h~72h 之间
    - **验证: 需求 6.1**

  - [x] 10.3 `tls_manager.go` 增加 `GenerateCSR()` 和 `RequestCertFromOS()` 方法
    - 修改 `mirage-gateway/pkg/security/tls_manager.go`
    - _需求: 6.2, 6.4_

  - [x] 10.4 `gen_gateway_cert.sh` / `gen_os_cert.sh` 有效期改为 72h，标记仅限开发环境
    - 修改 `deploy/certs/gen_gateway_cert.sh`
    - 修改 `deploy/certs/gen_os_cert.sh`
    - _需求: 6.3_

  - [x] 10.5 `cert-rotate.sh` 改为 CSR 模式，移除本地 `root-ca.key` 依赖
    - 修改 `deploy/scripts/cert-rotate.sh`
    - _需求: 6.2, 6.3_

  - [x] 10.6 Gateway 部署清单排除 `ca.key` / `root-ca.key`，增加部署前检查
    - 修改 `mirage-gateway/deployments/production_ready_manifest.yaml`
    - _需求: 6.3, 6.5_

- [x] 11. 检查点 - 信任链与证书
  - 确保所有测试通过，如有疑问请向用户确认。

- [x] 12. 邀请树与反白嫖治理（T03）
  - [x] 12.1 `schema.prisma` User 增加邀请链字段
    - 增加 `invitedBy`、`inviteRoot`、`inviteDepth`、`observationEndsAt`
    - 修改 `mirage-os/api-server/src/prisma/schema.prisma`
    - _需求: 3.5_

  - [x] 12.2 `invitation_service.go` 核销时写入完整邀请链
    - `invited_by`、`invite_root`、`invite_depth = parent.depth + 1`
    - 修改 `mirage-os/services/billing/invitation_service.go`
    - _需求: 3.1_

  - [x]* 12.3 编写邀请链深度正确性属性测试
    - **Property 4: 邀请链深度正确性**
    - 使用 rapid 验证 invite_depth = parent.depth + 1 且 invite_root 一致
    - **验证: 需求 3.1**

  - [x] 12.4 新增观察期逻辑：7 天观察期，限制配额 1GB、并发会话 1
    - 修改 `mirage-os/api-server/src/modules/auth/auth.service.ts`
    - _需求: 3.2_

  - [x]* 12.5 编写观察期配额限制属性测试
    - **Property 5: 观察期配额限制**
    - 使用 rapid 验证观察期内配额不超过 1GB 且并发不超过 1
    - **验证: 需求 3.2**

  - [x] 12.6 新增邀请树熔断 API：按 `invite_root` 批量冻结
    - _需求: 3.3_

  - [x]* 12.7 编写邀请树熔断完整性属性测试
    - **Property 6: 邀请树熔断完整性**
    - 使用 rapid 验证熔断后同一 invite_root 的所有用户被冻结
    - **验证: 需求 3.3**

  - [x] 12.8 注册接口增加速率限制：同一邀请码来源 IP 每小时最多 3 次
    - 修改 `mirage-os/api-server/src/modules/auth/auth.service.ts`
    - _需求: 3.4_

  - [x]* 12.9 编写注册速率限制属性测试
    - **Property 7: 注册速率限制**
    - 使用 fast-check 验证第 4 次及后续注册被拒绝
    - **验证: 需求 3.4**

- [x] 13. eBPF 金丝雀挂载与自动摘钩（T07）
  - [x] 13.1 `loader.go` 增加 `canaryAttach()` 方法
    - 创建 `dummy0` 虚拟接口，先挂载所有程序跑自检，通过后再挂生产网卡
    - 新增 `mirage-gateway/pkg/ebpf/canary.go`
    - 修改 `mirage-gateway/pkg/ebpf/loader.go`
    - _需求: 7.1, 7.5_

  - [x] 13.2 新增 `health_checker.go`：定期采样健康指标
    - 采样 `/proc/net/softnet_stat`、`/proc/net/dev` 丢包率、ring buffer error
    - 新增 `mirage-gateway/pkg/ebpf/health_checker.go`
    - _需求: 7.2_

  - [x] 13.3 达阈值时自动 detach 对应程序，切换 iptables fallback
    - 每个 BPF 程序独立健康状态标记
    - 修改 `mirage-gateway/pkg/ebpf/manager.go`
    - _需求: 7.3, 7.4_

  - [x]* 13.4 编写 eBPF 程序隔离摘钩属性测试
    - **Property 12: eBPF 程序隔离摘钩**
    - 使用 rapid 验证单个程序异常时只摘该程序，其余正常
    - **验证: 需求 7.3, 7.4**

- [x] 14. 生产日志脱敏与调试面收口（T08）
  - [x] 14.1 引入结构化日志库（zerolog 或 zap），替换所有 `log.Printf`
    - 修改 `mirage-gateway/pkg/api/handlers.go`
    - 修改 `mirage-gateway/pkg/api/command_audit.go`
    - 修改 `mirage-gateway/pkg/ebpf/monitor.go`
    - 修改 `mirage-gateway/pkg/phantom/honeypot.go`
    - 修改 `mirage-gateway/pkg/phantom/reporter.go`
    - _需求: 8.1_

  - [x] 14.2 新增 `pkg/logger/sanitizer.go`：IP /24 截断、userID 前 8 位、token 前 4 位
    - 新增 `mirage-gateway/pkg/logger/sanitizer.go`
    - _需求: 8.2_

  - [x]* 14.3 编写日志脱敏格式正确性属性测试
    - **Property 13: 日志脱敏格式正确性**
    - 使用 rapid 验证 SanitizeIP/SanitizeUserID/SanitizeToken 输出格式
    - **验证: 需求 8.2**

  - [x] 14.4 日志分三级：audit / ops / debug，生产禁止 debug 落盘
    - 通过 `LOG_LEVEL` 环境变量控制
    - _需求: 8.3, 8.4_

  - [x] 14.5 `monero_manager.go` 所有日志使用 sanitizer 处理敏感字段
    - 修改 `mirage-os/services/billing/monero_manager.go`
    - _需求: 8.5_

- [x] 15. 检查点 - P0 完成
  - 确保所有 P0 任务的测试通过，如有疑问请向用户确认。

---

### P1：首批客户接入后两周内完成

---

- [x] 16. 生产默认值 Fail-Closed（T18）
  - [x] 16.1 `auth.module.ts` 移除开发密钥 fallback
    - `JWT_SECRET` 未设置时模块初始化直接抛异常
    - 修改 `mirage-os/api-server/src/modules/auth/auth.module.ts`
    - _需求: 17.1_

  - [x] 16.2 `docker-compose.os.yml` 端口绑定改为 `127.0.0.1`
    - `127.0.0.1:3000:3000` 和 `127.0.0.1:50847:50847`
    - 修改 `deploy/docker-compose.os.yml`
    - _需求: 17.3_

  - [x] 16.3 新增 `docker-compose.dev.yml`，生产模板移除所有开发回退
    - 新增 `deploy/docker-compose.dev.yml`
    - 修改 `deploy/docker-compose.os.yml`
    - _需求: 17.4_

  - [x] 16.4 与 T06 对齐，删除生产节点依赖本地 CA key 的脚本路径
    - 修改 `deploy/certs/gen_gateway_cert.sh`
    - 修改 `deploy/certs/gen_os_cert.sh`
    - 修改 `deploy/scripts/cert-rotate.sh`
    - _需求: 17.2, 17.4_

- [x] 17. 内部横向链路收口（T17）
  - [x] 17.1 新增 `validateInternalURL()` 工具函数
    - 校验 URL host 白名单（127.0.0.1、localhost、::1、配置内网地址）
    - _需求: 16.1_

  - [x]* 17.2 编写内部 HTTP 白名单过滤属性测试
    - **Property 23: 内部 HTTP 白名单过滤**
    - 使用 fast-check 验证非白名单 host 被拒绝
    - **验证: 需求 16.1**

  - [x] 17.3 所有内部 fetch 调用增加 `redirect: 'error'`
    - 修改 `mirage-os/api-server/src/modules/billing/xmr-webhook.controller.ts`
    - 修改 `mirage-os/api-server/src/modules/billing/delivery.controller.ts`
    - 修改 `mirage-os/api-server/src/modules/gateways/bridge-client.service.ts`
    - _需求: 16.2_

  - [x] 17.4 `rest/middleware.go` secret 为空时拒绝所有请求
    - 修改 `mirage-os/gateway-bridge/pkg/rest/middleware.go`
    - _需求: 16.3_

  - [x] 17.5 所有 `*_URL` 环境变量启动时校验 host 白名单
    - 不在白名单内直接启动失败
    - _需求: 16.4_

  - [x]* 17.6 编写启动时 URL 环境变量白名单校验属性测试
    - **Property 24: 启动时 URL 环境变量白名单校验**
    - 使用 fast-check 验证非白名单 host 导致启动失败
    - **验证: 需求 16.4**

- [x] 18. 检查点 - Fail-Closed 与横向收口
  - 确保所有测试通过，如有疑问请向用户确认。

- [x] 19. 干净构建与供应链整洁化（T16）
  - [x] 19.1 `.gitignore` 增加排除项并清理已提交产物
    - 增加 `mirage-os/api-server/dist/` 和 `mirage-os/api-server/node_modules/`
    - 修改 `.gitignore`
    - _需求: 15.1_

  - [x] 19.2 `api-server/Dockerfile` 改为多阶段构建
    - builder 阶段 `npm ci --production`，运行阶段只拷贝 dist + 生产依赖
    - 修改 `mirage-os/api-server/Dockerfile`
    - _需求: 15.2_

  - [x] 19.3 生成发布 manifest：源码提交 hash、lockfile hash、产物 hash
    - _需求: 15.4_

- [x] 20. Client / Gateway 发布签名链（T11）
  - [x] 20.1 新增 `ReleaseManifest` 结构体和签名验证逻辑
    - 新增 `deploy/release/manifest.go`
    - 新增 `deploy/release/verify.go`
    - _需求: 14.1_

  - [x] 20.2 Gateway 和 Client `main.go` 增加启动自检
    - 加载 manifest → 计算本地二进制 hash → 验签
    - 修改 `phantom-client/cmd/phantom/main.go`
    - 修改 `mirage-gateway/cmd/gateway/main.go`
    - _需求: 14.2_

  - [x]* 20.3 编写发布产物签名验证属性测试
    - **Property 22: 发布产物签名验证**
    - 使用 rapid 验证篡改任意单字节导致验证失败
    - **验证: 需求 14.2**

  - [x] 20.4 自动升级机制先验签再落盘，未签名版本拒绝加载
    - _需求: 14.3_

- [x] 21. 会话级时序去相关（T09）
  - [x] 21.1 新增 `session_shaper.go`（Client 侧）
    - 实现 `SessionShaper`：小批量聚合 + 定时 flush，窗口 10-50ms
    - 新增 `phantom-client/pkg/gtclient/session_shaper.go`
    - _需求: 12.1_

  - [x] 21.2 在 `GTunnelClient.Send()` 前插入 `SessionShaper` 层
    - 修改 `phantom-client/pkg/gtclient/client.go`
    - _需求: 12.2_

  - [x] 21.3 Gateway 侧增加对称的 `SessionShaper` 实现
    - 修改 `mirage-gateway/pkg/gtunnel/` 相关文件
    - _需求: 12.3_

  - [x] 21.4 不同产品层级配置不同去相关档位
    - Standard: 无、Platinum: 30ms、Diamond: 50ms
    - _需求: 12.4_

  - [x]* 21.5 编写 Session Shaper 产品层级窗口映射属性测试
    - **Property 20: Session Shaper 产品层级窗口映射**
    - 使用 rapid 验证各层级窗口值正确
    - **验证: 需求 12.4**

- [x] 22. Client 路径健康评分与劫持可疑判定（T10）
  - [x] 22.1 新增 `path_health.go`：PathHealthScorer
    - 维护 RTT 基线（EWMA）、连续失败计数、切换频率
    - 新增 `phantom-client/pkg/gtclient/path_health.go`
    - _需求: 13.1_

  - [x] 22.2 `quic_engine.go` 增加 RTT 采样接口
    - 从 QUIC ConnectionState 获取 SmoothedRTT
    - 修改 `phantom-client/pkg/gtclient/quic_engine.go`
    - _需求: 13.2_

  - [x] 22.3 `state.go` 增加 `StateSuspicious` 状态
    - 可疑链路暂停敏感控制面写入，不立刻切业务流
    - 修改 `phantom-client/pkg/gtclient/state.go`
    - 修改 `phantom-client/pkg/gtclient/client.go`
    - _需求: 13.3, 13.4_

  - [x] 22.4 异常分数越界后触发候补路径评估
    - 调用 `doReconnect` 的 L1 层
    - 修改 `phantom-client/pkg/gtclient/client.go`
    - _需求: 13.5_

  - [x]* 22.5 编写路径健康评分驱动状态转换属性测试
    - **Property 21: 路径健康评分驱动状态转换**
    - 使用 rapid 验证评分越过阈值进入 StateSuspicious，恢复后退出
    - **验证: 需求 13.4, 13.5**

- [x] 23. 检查点 - P1 完成
  - 确保所有 P1 任务的测试通过，如有疑问请向用户确认。

---

### P2：版本级演进项

---

- [x] 24. 云与运维安全 Runbook（T12）
  - [x] 24.1 编写生产节点密钥注入 Runbook
    - 不通过镜像固化，不通过普通环境变量长期保存
    - _需求: 18.1_

  - [x] 24.2 编写云控制台、对象存储、CI、镜像仓库最小权限模型文档
    - _需求: 18.2_

  - [x] 24.3 编写"节点失陷后替换流程"可演练 Runbook
    - 限定时间内替换单个失陷节点
    - _需求: 18.3_

- [x] 25. 最终检查点
  - 确保所有任务完成，全部测试通过，如有疑问请向用户确认。

## 备注

- 标记 `*` 的子任务为可选测试任务，可跳过以加速 MVP
- 每个任务关联到具体需求编号以确保可追溯
- 检查点确保增量验证
- 属性测试验证设计文档中定义的 24 个正确性属性
- Go 侧属性测试使用 `pgregory.net/rapid`，TypeScript 侧使用 `fast-check`
