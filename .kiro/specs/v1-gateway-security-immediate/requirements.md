# 需求文档：Gateway 安全紧急封洞

## 简介

本 Spec 对应 `OS-Gateway 安全整改清单.md` 中 Gateway 立刻修部分（Gateway-Immediate-1 ~ 5），目标是封堵 Gateway 侧最高风险的安全漏洞。

当前 Gateway 已具备 mTLS、证书钉扎、黑名单、威胁感知等安全模块的基础骨架，但存在以下高危问题：

1. mTLS 可被配置禁用，生产环境可能回退到明文 gRPC
2. 证书钉扎已初始化但未接入连接校验链（`_ = certPin`）
3. 入口异常流量只上报不阻断
4. 黑名单写入 eBPF Map 的链路存在断点（`blacklist_lpm` Map 可能未被数据面程序引用）
5. 高危命令（kill/焦土/模式切换/转生）无源验证、无签名、无审计日志

本轮整改的核心原则：**先封洞，不扩展功能。**

## 术语表

- **mTLS**：双向 TLS 认证，Gateway 与 OS 之间必须互相验证证书
- **CertPin**：证书钉扎，对 OS 证书的 SHA-256 指纹进行固定校验，防止中间人
- **BlacklistManager**：Go 侧黑名单管理器，维护 CIDR → 过期时间映射
- **blacklist_lpm**：eBPF LPM Trie Map，数据面用于入口 IP 匹配
- **CommandHandler**：gRPC 下行指令处理器，处理 PushStrategy/PushBlacklist/PushQuota/PushReincarnation
- **MotorDownlink**：下行状态映射器，通过幂等 Hash 校验写入 eBPF Map
- **ScorchedEarth**：焦土协议执行器，负责紧急自毁流程
- **ThreatResponder**：威胁响应器，根据威胁等级自动调整防御策略
- **Cortex**：威胁感知中枢，指纹-IP 关联分析引擎

## 需求

### 需求 1：生产模式强制 mTLS，禁止明文回退

**用户故事：** 作为安全工程师，我需要 Gateway 在生产模式下强制使用 mTLS，以便非可信 OS 无法接入 Gateway 控制链路。

#### 验收标准

1. WHEN `gateway.yaml` 中 `mcc.tls.enabled` 为 `false` 且环境变量 `MIRAGE_ENV` 为 `production` 时，THE Gateway SHALL 拒绝启动并输出明确错误信息
2. WHEN `mcc.tls.enabled` 为 `true` 但证书文件缺失或无效时，THE Gateway SHALL 拒绝启动并输出证书路径和具体错误
3. THE gRPC Client（上行连接 OS）SHALL 在 `tlsConfig` 为 nil 时拒绝建立连接，而不是回退到 `insecure.NewCredentials()`
4. THE gRPC Server（下行接收 OS 指令）SHALL 在 `tlsConfig` 为 nil 时拒绝启动
5. WHEN mTLS 握手失败时，THE gRPC Server SHALL 静默关闭连接，不返回 TLS Alert（保持现有行为）

### 需求 2：证书钉扎接入连接校验链

**用户故事：** 作为安全工程师，我需要证书钉扎在 gRPC 连接建立后真正生效，以便中间人证书无法通过握手后的业务校验。

#### 验收标准

1. WHEN `security.cert_pinning.enabled` 为 `true` 且 `preset_hash` 非空时，THE Gateway SHALL 在 gRPC Client 连接 OS 后对 OS 证书执行 SHA-256 指纹校验
2. IF 证书指纹校验失败，THEN THE Gateway SHALL 立即断开连接并记录安全告警日志（包含预期指纹前16位和实际指纹前16位）
3. THE `certPin` 变量 SHALL 不再被 `_ =` 忽略，而是传入 gRPC Client 的连接校验链
4. WHEN 证书钉扎启用但 `preset_hash` 为空时，THE Gateway SHALL 在首次连接成功后自动钉扎 OS 证书（Trust-On-First-Use），并记录日志
5. THE CertPin SHALL 支持通过 OS 下发指令更新钉扎指纹（为证书轮换预留能力）

### 需求 3：入口异常流量可拒绝

**用户故事：** 作为安全工程师，我需要 Gateway 对明显的扫描流量直接丢弃，而不是只上报后继续放行。

#### 验收标准

1. WHEN ThreatResponder 检测到威胁等级 >= 3（HIGH）时，THE Gateway SHALL 将触发源 IP 自动加入黑名单（TTL 1小时）
2. WHEN Cortex 自动封禁回调触发时，THE blacklist.Add SHALL 确保条目同步写入 eBPF `blacklist_lpm` Map 并返回成功
3. THE Gateway SHALL 为入口处置定义 5 级动作枚举：`ActionPass / ActionObserve / ActionThrottle / ActionTrap / ActionDrop`
4. WHEN 威胁等级 >= 4（CRITICAL）时，THE Gateway SHALL 对触发源执行 `ActionDrop`（静默丢弃）
5. THE Gateway SHALL 记录每次入口处置动作到安全日志（源 IP、动作类型、触发原因、时间戳）

### 需求 4：黑名单到数据面生效链路修复

**用户故事：** 作为安全工程师，我需要黑名单写入后能在数据面立即命中，以便封禁的 IP 真正被阻断。

#### 验收标准

1. THE `blacklist_lpm` eBPF Map SHALL 在 eBPF 加载阶段被正确创建并 Pin 到 bpffs
2. THE 数据面入口程序（jitter.c TC ingress 或独立入口检查程序）SHALL 在处理每个入站包时查询 `blacklist_lpm` Map
3. IF 源 IP 命中 `blacklist_lpm`，THEN THE 数据面 SHALL 执行 `TC_ACT_SHOT`（静默丢弃），不返回任何响应
4. THE BlacklistManager.syncToEBPF SHALL 在写入失败时记录错误日志并返回错误（当前静默忽略）
5. THE BlacklistManager SHALL 在启动时验证 `blacklist_lpm` Map 存在且可写，不存在时记录错误并标记降级状态
6. THE BlacklistManager SHALL 提供 `SyncStats()` 方法，返回当前 Go 侧条目数与 eBPF 侧条目数，用于一致性校验

### 需求 5：高危命令收敛到唯一可信来源

**用户故事：** 作为安全工程师，我需要 kill/焦土/模式切换/转生等高危命令只能来自唯一可信控制面，以便 Gateway 不再接受来源不明的高危控制动作。

#### 验收标准

1. THE CommandHandler SHALL 为所有高危命令（PushReincarnation、PushStrategy defense_level >= 4、PushQuota remaining_bytes = 0）增加命令签名校验
2. THE 命令签名 SHALL 使用 HMAC-SHA256，密钥通过 `gateway.yaml` 中 `security.command_secret` 配置，签名字段通过 gRPC metadata 传递
3. IF 命令签名校验失败，THEN THE CommandHandler SHALL 返回 `codes.PermissionDenied` 并记录安全告警日志（包含来源 IP、命令类型、时间戳）
4. THE CommandHandler SHALL 为所有命令增加审计日志，记录：命令类型、来源信息、关键参数、执行结果、时间戳
5. THE CommandHandler SHALL 为高危命令增加速率限制：同一来源每分钟最多 10 次高危命令，超出时返回 `codes.ResourceExhausted`
6. THE 焦土协议触发路径 SHALL 仅限：心跳超时（Watchdog）、OS 下发 emergency_ctrl_map 指令码、调试器检测，禁止其他路径触发
