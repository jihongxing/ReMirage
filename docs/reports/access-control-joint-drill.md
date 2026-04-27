# 准入控制联合演练记录 (Joint Drill Record)

> 本文档记录 Phase 3 M9 准入控制联合演练的完整过程和结果。
> 包含两个演练场景：非法接入（场景 1）和配额耗尽（场景 2）。
> 每个步骤记录实际输出，不仅记录"通过/失败"。

## 场景 1：非法接入 → 拒绝 → 日志脱敏 → 配额不受损

### 步骤 1a：无 HMAC 签名请求 → Command_Auth 拒绝

| 属性 | 值 |
|------|-----|
| 操作 | 发送无 gRPC metadata 的请求 |
| 组件 | `Command_Auth`（`pkg/api/command_auth.go`） |
| 预期 | 拒绝，返回 "missing gRPC metadata" |
| 测试入口 | `TestSecurityRegression_mTLS_RejectWithoutSignature` |
| 结果 | **PASS** — 无 metadata 时请求被拒绝 |

### 步骤 1b：过期时间戳请求 → Command_Auth 拒绝

| 属性 | 值 |
|------|-----|
| 操作 | 发送 120 秒前时间戳的请求（超过 ±60s 窗口） |
| 组件 | `Command_Auth` |
| 预期 | 拒绝，返回 "timestamp expired" |
| 测试入口 | `TestSecurityRegression_mTLS_RejectExpiredTimestamp` |
| 结果 | **PASS** — 过期时间戳被拒绝 |

### 步骤 1c：重放 nonce 请求 → Command_Auth 拒绝

| 属性 | 值 |
|------|-----|
| 操作 | 使用已消费的 nonce 发送第二次请求 |
| 组件 | `Command_Auth` → `nonceCache` |
| 预期 | 第一次通过，第二次拒绝（"nonce replay detected"） |
| 测试入口 | `TestSecurityRegression_NonceReplayDetected` |
| 结果 | **PASS** — 重放 nonce 被检测并拒绝 |

### 步骤 1d：无 JWT WebSocket 连接 → JWT_Auth 拒绝

| 属性 | 值 |
|------|-----|
| 操作 | 发送无 JWT token 的 WebSocket 连接到 `/ws` |
| 组件 | `JWT_Auth`（`services/ws-gateway/auth_test.go`） |
| 预期 | 返回 HTTP 401 |
| 测试入口 | `TestJWTAuth_MissingToken` |
| 结果 | **PASS** — 无 token 返回 401 |

### 日志脱敏验证

| 验证项 | 方法 | 结果 |
|--------|------|------|
| IP 脱敏 → `x.x.x.***` | `RedactIPInText` 替换所有 IPv4 最后一段 | **PASS** — `TestRedactIPInText` + `TestRedactIPInText_NoLeakLastOctet` + PBT Property 3 (100 次) |
| Token 脱敏 → `***` | `RedactToken` 替换为 `***` | **PASS** — `TestRedactToken_Standard` |
| Secret 脱敏 → `[REDACTED]` | `RedactSecret` 替换为 `[REDACTED]` | **PASS** — `TestRedactSecret_Standard` |

### 配额不受损验证

| 验证项 | 方法 | 结果 |
|--------|------|------|
| 合法用户 `RemainingBytes` 不变 | `TestCritical_IllegalRequestNoQuotaImpact` | **PASS** — 非法请求被拒绝后合法用户配额不变 |
| 合法用户 `Exhausted` 保持为 0 | 同上 | **PASS** |

### 回归基线

`pkg/api` 包 HMAC 回归测试（`security_regression_test.go`）全部通过：
- `TestSecurityRegression_mTLS_RejectWithoutSignature` ✅
- `TestSecurityRegression_mTLS_RejectInvalidHMAC` ✅
- `TestSecurityRegression_mTLS_AcceptValidHMAC` ✅
- `TestSecurityRegression_mTLS_RejectExpiredTimestamp` ✅
- `TestSecurityRegression_RejectMissingNonce` ✅
- `TestSecurityRegression_RejectHighRiskWithoutPayloadHash` ✅
- `TestSecurityRegression_NonceReplayDetected` ✅
- `TestProperty_HMACDeterminism` (PBT, 100 次) ✅

---

## 场景 2：配额耗尽 → 目标用户熔断 → 隔离验证

### 步骤 4a：GrantAccess(userA, userB)

| 属性 | 值 |
|------|-----|
| 操作 | 为 userA 和 userB 分别授权配额 |
| 组件 | `QuotaBucketManager.UpdateQuota` |
| 测试入口 | `TestIntegration_MultiUserQuotaIsolation` |
| 结果 | **PASS** — 两用户配额独立分配 |

### 步骤 4b：userA 持续消耗至耗尽

| 属性 | 值 |
|------|-----|
| 操作 | userA 并发消费直至配额耗尽 |
| 组件 | `QuotaBucketManager.Consume` (CAS 原子操作) |
| 结果 | **PASS** — userA 消费不超过分配配额 |

### 步骤 4c：验证 userA 被熔断（Exhausted=1）

| 属性 | 值 |
|------|-----|
| 操作 | 检查 userA 的 `Exhausted` 标志 |
| 组件 | `QuotaBucketManager.IsExhausted` |
| 结果 | **PASS** — `Exhausted=1`，后续 `Consume` 返回 false |

### `onQuotaExhausted` 回调验证

| 验证项 | 结果 |
|--------|------|
| 回调参数为 userA UID | **PASS** — `TestFuseCallback_ExhaustedTriggersDisconnect` |
| 回调仅触发一次 | **PASS** — PBT Property 2 (100 次) |
| 其他用户不触发回调 | **PASS** — `TestFuseCallback_OnlyAffectsTargetUser` |

### 步骤 4d：验证 userB 不受影响

| 验证项 | 方法 | 结果 |
|--------|------|------|
| userB `RemainingBytes` 未减少 | PBT Property 1 (100 次) | **PASS** |
| userB `Exhausted` = 0 | `TestQuotaBucket_IsolationTwoUsers` | **PASS** |
| userB `Consume()` 继续成功 | `TestFuseCallback_OnlyAffectsTargetUser` | **PASS** |

### 熔断日志脱敏验证

| 验证项 | 方法 | 结果 |
|--------|------|------|
| IP 脱敏 | `TestCritical_FuseLogRedaction` | **PASS** — 熔断日志中 IP 已脱敏为 `x.x.x.***` |
| 无明文密钥/token | `TestCritical_FuseLogRedaction` | **PASS** — Token → `***`，Secret → `[REDACTED]` |

### AddQuota 重新激活验证

| 验证项 | 方法 | 结果 |
|--------|------|------|
| `Exhausted` 恢复为 0 | `TestCritical_QuotaReactivationE2E` + PBT Property 4 (100 次) | **PASS** |
| `RemainingBytes` 等于追加量 | PBT Property 4 (100 次) | **PASS** |
| 可继续消费 | `TestCritical_QuotaReactivationE2E` | **PASS** |

---

## Smoke Test 入口汇总

| 入口 | 执行命令 | 覆盖范围 |
|------|----------|----------|
| HMAC 回归 | `go test ./pkg/api -run "TestSecurityRegression_\|TestProperty_HMACDeterminism" -v` | 场景 1 鉴权链路 |
| JWT 回归 | `go test ./services/ws-gateway -run "TestJWTAuth_" -v`（mirage-os） | 场景 1 WebSocket 鉴权 |
| 脱敏回归（Gateway） | `go test ./pkg/redact/ -v`（mirage-gateway） | 场景 1/2 日志脱敏 |
| 脱敏回归（OS） | `go test ./pkg/redact/ -v`（mirage-os） | OS 侧脱敏 |
| 配额隔离 | `go test ./pkg/api -run "TestQuotaBucket_\|TestProperty_QuotaBucketIsolation" -count=10 -v` | 场景 2 隔离 |
| Redis 鉴权连通性 | `Select-String -Path deploy/docker-compose.os.yml -Pattern 'requirepass\|MIRAGE_REDIS_PASSWORD\|redis://:'` | 生产配置鉴权闭环 |

## Critical Test 入口列表

| 测试名称 | 执行命令 | 预期结果 | 所属部署等级 | 环境依赖 |
|----------|----------|----------|-------------|----------|
| `TestCritical_IllegalRequestNoQuotaImpact` | `go test ./pkg/api -run TestCritical_IllegalRequestNoQuotaImpact -v` | 非法请求被拒绝，合法用户配额不变 | All | 无 eBPF 依赖 |
| `TestCritical_FuseLogRedaction` | `go test ./pkg/api -run TestCritical_FuseLogRedaction -v` | 熔断日志中 IP/Token/Secret 已脱敏 | All | 无 eBPF 依赖 |
| `TestCritical_QuotaReactivationE2E` | `go test ./pkg/api -run TestCritical_QuotaReactivationE2E -v` | AddQuota 后 Exhausted=0 且可继续消费 | All | 无 eBPF 依赖 |

## Redis 鉴权连通性 Smoke 入口

复用功能验证清单中"生产配置鉴权闭环"口径：

```powershell
Select-String -Path deploy/docker-compose.os.yml -Pattern 'requirepass|MIRAGE_REDIS_PASSWORD|redis://:'
```

验证项：
- Redis 启用 `requirepass`
- 消费方连接串带密码（`redis://:${MIRAGE_REDIS_PASSWORD}@redis:6379`）
- healthcheck 使用鉴权探活

## 异常现象记录

本次联合演练未发现异常现象。所有验证点均按预期工作：
- 非法请求被正确拒绝
- 日志脱敏完整
- 配额隔离无串扰
- 熔断回调精确定向
- AddQuota 重新激活正常
