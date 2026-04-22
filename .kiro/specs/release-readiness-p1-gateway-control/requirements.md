# 需求文档：Release Readiness P1 Gateway Control Hardening

## 简介

本文档收敛 Gateway 审计中不会立刻阻止编译，但会在上线后让防护链路、控制面安全和运行观测失真的问题。目标是让 Gateway 的“已声明能力”真正进入运行主路径。

---

## 需求

### 需求 1：L2 防护组件必须进入真实运行路径

**用户故事：** 作为防护系统，我需要 `NonceStore`、`HandshakeGuard`、`ProtocolDetector` 真正参与入口处理，而不是只在启动时被实例化。

#### 验收标准

1. WHEN Gateway 启动时，THE L2 组件 SHALL 被接入实际 listener、session 或 command 入口
2. THE 运行路径 SHALL 存在对 `NonceStore`、`HandshakeGuard`、`ProtocolDetector` 的真实调用点
3. THE 主程序 SHALL 不再通过 `_ = handshakeGuard` 等方式丢弃关键防护组件

---

### 需求 2：L1 SYN 验证不得被伪造 ACK 绕过

**用户故事：** 作为数据面守卫，我需要 SYN challenge 机制真正校验握手关联性，以便攻击者不能仅凭伪造 ACK 完成验证。

#### 验收标准

1. WHEN 源 IP 超过 challenge 阈值时，THE L1 程序 SHALL 生成可验证 challenge
2. THE ACK 验证 SHALL 绑定足够的 TCP 语义，而不是仅比较同源 IP 上缓存的 hash
3. IF ACK 未满足 challenge 校验，THEN THE 源地址 SHALL 不能被标记为 validated
4. THE 仓库 SHALL 包含针对 challenge/ack 绕过的回归测试

---

### 需求 3：高危控制命令必须强制完整签名与真实限流

**用户故事：** 作为控制面，我需要高危命令的鉴权和限流真正覆盖 payload 与来源主机，以便减少重放、payload 替换和端口轮换绕过。

#### 验收标准

1. WHEN 校验高危命令时，THE Gateway SHALL 强制要求 `x-mirage-nonce` 与 `x-mirage-payload-hash`
2. THE HMAC 校验 SHALL 对具体 payload 绑定，而不是允许仅对 `commandType + timestamp` 签名
3. THE rate limiter SHALL 以源 IP 或等价主机标识限流，而不是 `ip:port`
4. THE 安全回归测试 SHALL 拒绝缺失 nonce、缺失 payload-hash、修改 payload、重复 nonce 的请求

---

### 需求 4：V2 控制链路必须真正有已注册处理器

**用户故事：** 作为编排内核，我需要 EventDispatcher 在运行态拥有真正的 handler 注册与状态回执能力，而不是把 V2 adapter 变成空壳。

#### 验收标准

1. WHEN Gateway 初始化 V2 组件时，THE EventRegistry SHALL 注册生产可用的 handler
2. WHEN V2 adapter 投递控制事件后，THE EventDispatcher SHALL 不因 `ErrHandlerNotRegistered` 失败
3. THE 威胁事件缓存 SHALL 在重连后 flush，而不是永久滞留在 buffer
4. THE V2 相关测试 SHALL 反映真实运行接线，而不是只依赖测试桩 handler

---

### 需求 5：审计、指标与测试门禁必须可信

**用户故事：** 作为运维和审计方，我需要 Gateway 的日志、指标和测试结果能够真实反映系统状态。

#### 验收标准

1. THE 审计日志 SHALL 不得对同一请求同时记录成功和失败两种结论
2. THE L1 指标同步 SHALL 上报增量或显式 reset 后的值，避免重复累计放大
3. THE `pkg/orchestrator/events` 测试资产 SHALL 与当前 `EventType` 定义保持一致
4. THE 目标范围测试 SHALL 在合并前保持绿色

