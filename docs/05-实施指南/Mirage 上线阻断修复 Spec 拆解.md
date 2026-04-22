---
Status: temporary
Target Truth: 上线阻断修复专项，结论应回写到对应实现
Migration: 上线阻断修复 Spec，完成后归档
---

# Mirage 上线阻断修复 Spec 拆解

## 1. 目的

本文档将最新一轮 Mirage V2 审计结果整理为一组新的 `P0/P1` 修复 Spec，用于替代那些“代码、文档、勾选状态已经失真”的旧发布判断。

这份拆解只做两件事：

1. 把审计发现映射为新的可执行 Spec
2. 给出严格的执行顺序和验收口径

本轮不处理 `P2` 收口项，它们会在 `P0/P1` 完成后再进入下一轮。

---

## 2. 结论

当前仓库状态不能作为“稳定可上线运营”交付。

阻断原因主要集中在三层：

1. **运行时主链路未闭环**
   - Gateway 与 Client 都存在直接编译阻断
   - OS / Client 的 topology 契约不兼容
   - heartbeat 仍写旧 schema
   - delivery / provision 闭环不通
2. **防护与控制面声明未真正生效**
   - Gateway 的 L2 防护组件未接入真实入口
   - L1 SYN 验证存在绕过
   - 高危控制命令签名与限流仍有缺口
   - V2 dispatcher 在运行态缺少 handler
3. **发布与运维真相未收口**
   - 证书签发/轮转路径不真实
   - query surface 认证不足
   - compose / manifest / 配置模型不一致
   - docs 与旧 spec 的完成状态不可直接用作发布依据

---

## 3. 新 Spec 列表

### Spec A：`release-readiness-p0-runtime`

路径：`.kiro/specs/release-readiness-p0-runtime`

处理范围：

- Gateway 主程序构建阻断
- phantom-client 主程序构建阻断
- topology API 与 Client 契约不兼容
- gateway-bridge heartbeat/stale cleanup 仍写旧 schema
- delivery / provisioning 交付链路不通

对应 Findings：

- Finding 1
- Finding 2
- Finding 3
- Finding 4
- Finding 5

### Spec B：`release-readiness-p1-gateway-control`

路径：`.kiro/specs/release-readiness-p1-gateway-control`

处理范围：

- Gateway L2 防护组件未接入真实运行路径
- L1 SYN validation 绕过
- 高危控制命令 HMAC 与 rate limiter 收口
- V2 dispatcher 运行态 handler 缺失
- threat buffer flush、审计日志、指标、测试门禁可信化

对应 Findings：

- Finding 6
- Gateway 二轮审计的 P1 项

### Spec C：`release-readiness-p1-os-release-closure`

路径：`.kiro/specs/release-readiness-p1-os-release-closure`

处理范围：

- 证书签发/轮转路径收口
- query surface 认证与 persona-query 路由缺口
- compose / manifest / 配置模型一致性
- docs / spec / 发布 gate 的 traceability 收口

对应 Findings：

- OS / deploy / docs 审计中的 P1 项

---

## 4. Findings 到 Spec 的映射

| Finding | 问题摘要 | 目标 Spec |
|--------|----------|-----------|
| F1 | Gateway entrypoint does not build | `release-readiness-p0-runtime` |
| F2 | phantom-client main package does not build | `release-readiness-p0-runtime` |
| F3 | Topology API contract incompatible with client | `release-readiness-p0-runtime` |
| F4 | Heartbeats still write pre-migration gateway columns | `release-readiness-p0-runtime` |
| F5 | Public delivery endpoint is guarded like an internal API | `release-readiness-p0-runtime` |
| F6 | L2 defense components instantiated but never wired | `release-readiness-p1-gateway-control` |

补充映射：

- Gateway 审计中的 `HMAC 重放/替换`、`V2 handler 缺失`、`rate limit 以 ip:port 计`、`审计日志双写`、`threat buffer 不回放`、`指标重复累计` 统一归入 `release-readiness-p1-gateway-control`
- OS / Client / Deploy 审计中的 `证书假签发`、`query 面伪认证`、`persona route 未挂载`、`compose build context 错误`、`docs/spec 完成状态冲突` 统一归入 `release-readiness-p1-os-release-closure`

---

## 5. 执行顺序

### 第一阶段：只做 P0

先完成 `release-readiness-p0-runtime`，因为它决定：

1. 产物能不能构建
2. Client 能不能拿到配置
3. OS / Client 能不能在 topology 上说同一种语言
4. Gateway 存活判断是否可信

P0 未完成前，不应继续宣称“进入上线前联调”。

### 第二阶段：并行推进两条 P1

P0 关闭后，并行推进：

1. `release-readiness-p1-gateway-control`
2. `release-readiness-p1-os-release-closure`

这样可以把：

- Gateway 防护与控制面能力
- OS/query/release/证书/文档收口

分成两条互相依赖较少的工作线。

---

## 6. 验收口径

本轮发布结论只以以下内容为准：

1. 新 P0/P1 Spec 的检查点是否通过
2. 对应命令是否可执行通过
3. 对应契约测试是否真实通过

不再以下列内容作为单独依据：

1. 旧 `.kiro/specs` 中已打勾的任务
2. 旧整改清单中的“建议已完成”表述
3. 未绑定验证命令的口头结论

---

## 7. 建议的验证命令

P0 完成后至少应通过：

- `cd mirage-gateway && go test ./... -run '^$'`
- `cd phantom-client && go test ./...`
- `cd mirage-os && go test ./services/persona-query -v`
- `cd mirage-os && go test ./services/topology ./services/entitlement ./services/state-query ./services/transaction-query`

P1 完成后至少应补通过：

- `cd mirage-gateway && go test ./pkg/api ./pkg/threat ./pkg/orchestrator/events -v`
- `cd mirage-os && docker compose -f deploy/docker-compose.os.yml build`
- topology / delivery / cert sign 的契约级或集成级验证

---

## 8. P2 延后项

暂不并入本轮 Spec 的问题：

- recovery FSM 断连时长判定失真
- CTLock 能力声明收口
- 其余不阻断本轮 P0/P1 发布结论的长期优化项

这些问题应在本轮 P0/P1 关闭后，再进入下一轮独立 Spec。
