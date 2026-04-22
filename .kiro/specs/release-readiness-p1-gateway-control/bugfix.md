# Bugfix Requirements Document

## Introduction

Mirage V2 上线阻断修复 P1 级别（Gateway 控制面方向）。本轮修复覆盖 5 个已验证的 Bug，涉及 L2 防护组件未接入、HMAC nonce 可绕过、Rate Limiter key 粒度错误、V2 EventDispatcher 无 handler、以及 Prometheus 指标高基数标签。这些问题共同导致 Gateway 的防护与控制面在运行态形同虚设。

## Bug Analysis

### Current Behavior (Defect)

1.1 WHEN Gateway 启动时，`NonceStore` 和 `HandshakeGuard` 被实例化后通过 `_ = nonceStore` 和 `_ = handshakeGuard` 丢弃（`main.go` 第 368-369 行），THEN L2 层的抗重放检测和半开连接熔断完全不生效，gRPC server listener 未经 `HandshakeGuard.WrapListener()` 包装

1.2 WHEN 客户端发送 HMAC 签名命令时不携带 `x-mirage-nonce` metadata，THEN `command_auth.go` 中 `if len(nonce) > 0 { ... }` 条件跳过重放检测，攻击者可无限重放已截获的合法签名请求

1.3 WHEN gRPC 请求到达 `CommandHandler` 时，`peerAddr(ctx)` 返回 `p.Addr.String()` 即 `ip:port` 格式，Rate Limiter 以此为 key，THEN 攻击者通过更换源端口即可绕过每分钟 10 次的速率限制

1.4 WHEN V2 编排链路收到 ControlEvent 时，`v2EventRegistry` 创建后未调用 `Register()` 注册任何 `EventHandler`，THEN 所有 V2 事件分发均返回 `ErrHandlerNotRegistered`，strategy/blacklist/quota/reincarnation 四种 V2 编排链路完全不通

1.5 WHEN 不同源 IP 触发准入评分时，`admissionActionTotal` 指标使用 `[]string{"ip", "action"}` 标签，THEN 大量不同 IP 导致 Prometheus 时间序列膨胀，Gateway 内存持续增长

### Expected Behavior (Correct)

2.1 WHEN Gateway 启动时，THEN `NonceStore` SHALL 接入 HMAC 校验链路用于 nonce 去重，`HandshakeGuard` SHALL 通过 `WrapListener()` 包装 gRPC server 的 `net.Listener`，使半开连接熔断和握手超时检测在所有入站连接上生效

2.2 WHEN 客户端发送 HMAC 签名命令时缺少 `x-mirage-nonce` metadata，THEN 系统 SHALL 拒绝该请求并返回认证错误，nonce 为必填字段

2.3 WHEN gRPC 请求到达 `CommandHandler` 时，Rate Limiter SHALL 以纯 IP（不含端口）为 key 进行速率限制，通过 `net.SplitHostPort()` 从 `peerAddr` 中提取 host 部分

2.4 WHEN V2 编排链路初始化时，`v2EventRegistry` SHALL 注册至少 `strategy.update`、`blacklist.update`、`quota.update`、`reincarnation.trigger` 四种 EventType 的 handler，使 V2 ControlEvent 分发正常工作

2.5 WHEN 准入评分触发时，`admissionActionTotal` 指标 SHALL 移除 `ip` 标签，仅保留 `action` 标签（或替换为低基数标签如 `gateway_id`），避免时间序列膨胀

### Unchanged Behavior (Regression Prevention)

3.1 WHEN gRPC server 正常接收合法连接时，THEN 系统 SHALL CONTINUE TO 正常处理 gRPC 请求，`HandshakeGuard` 包装不影响已完成握手的连接性能

3.2 WHEN 客户端携带有效 nonce 发送 HMAC 签名命令时，THEN 系统 SHALL CONTINUE TO 正常通过认证校验，HMAC 签名验证逻辑不变

3.3 WHEN 同一 IP 的合法请求在速率限制阈值内时，THEN 系统 SHALL CONTINUE TO 正常处理请求，速率限制阈值（每分钟 10 次）不变

3.4 WHEN legacy 下行命令链路（非 V2）处理 PushStrategy/PushBlacklist/PushQuota/PushReincarnation 时，THEN 系统 SHALL CONTINUE TO 通过 legacy 路径正常执行，V2 adapter 失败时降级到 legacy 的行为不变

3.5 WHEN 其他 Prometheus 指标（如 `IngressRejectTotal`、`AuthFailureTotal` 等）正常上报时，THEN 系统 SHALL CONTINUE TO 保持现有标签结构和计数行为不变
