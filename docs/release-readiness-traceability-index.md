---
Status: temporary
Target Truth: 本轮发布周期有效，不自动成为长期真相源
Migration: 发布就绪索引，仅对当前发布周期有效
---

# Release Readiness Traceability Index

> 本文档为本轮发布的唯一真相索引。所有发布结论以此文档中的检查点为准。

## 适用范围

本轮发布仅以 P0/P1 检查点为准。以下 spec 为本轮有效修复拆解：

| 优先级 | Spec | 状态 | 说明 |
|--------|------|------|------|
| P0 | `release-readiness-p0-runtime` | 有效 | 运行时阻断修复 |
| P1 | `release-readiness-p1-gateway-control` | 有效 | Gateway 控制面收口 |
| P1 | `release-readiness-p1-os-release-closure` | 有效 | OS 发布面收口 |

## 旧 Spec 适用状态

以下 spec 在本轮发布中**不作为上线依据**。其完成状态可能与当前实现不一致：

| Spec | 状态 | 说明 |
|------|------|------|
| `pre-launch-security-hardening` | ⚠️ 部分失效 | 部分任务已被 P0/P1 spec 重新拆解 |
| `core-hardening` | ⚠️ 部分失效 | 安全加固已在 P0 中重新验证 |
| `gateway-closure` | ⚠️ 部分失效 | Gateway 收口已在 P1-gateway-control 中重新定义 |
| `production-delivery` | ⚠️ 失效 | 交付标准已被本轮 P0/P1 检查点替代 |
| `v1-*` / `v2-*` | ✅ 功能 spec | 功能实现 spec，不影响发布判定 |
| `anti-ddos-architecture` | ✅ 功能 spec | 架构设计，不影响发布判定 |
| `mirage-os-brain` | ✅ 功能 spec | 功能实现 spec |
| `mirage-os-completion` | ✅ 功能 spec | 功能实现 spec |
| `phantom-client` | ✅ 功能 spec | 功能实现 spec |

## Findings → Spec → Task → Verification 映射

### P0: 运行时阻断

| Finding | Spec Task | 验证命令 |
|---------|-----------|----------|
| Gateway 编译失败 | P0 Task 1 | `cd mirage-gateway && go build ./cmd/gateway/` |
| OS 编译失败 | P0 Task 2 | `cd mirage-os && go build ./services/api-gateway/` |
| Proto 不兼容 | P0 Task 3 | `cd mirage-proto && protoc --go_out=. *.proto` |

### P1-A: 证书链路收口

| Finding | Spec Task | 验证命令 |
|---------|-----------|----------|
| `/internal/cert/sign` 返回占位 PEM | P1-OS Task 1 | `curl -X POST https://os:3000/internal/cert/sign -d '{"csr":"...","gatewayId":"gw-1"}'` |
| cert-rotate.sh 口径矛盾 | P1-OS Task 2 | `bash deploy/scripts/cert-rotate.sh --check-only` |
| 证书目录不一致 | P1-OS Task 2.2 | 检查 `deploy/certs/`, `deploy/scripts/cert-rotate.sh`, `deploy/docker-compose.os.yml` 中证书路径一致 |

### P1-B: 查询面与部署资产

| Finding | Spec Task | 验证命令 |
|---------|-----------|----------|
| entitlement 仅信任 X-Client-ID | P1-OS Task 3 | `curl -H 'X-Client-ID: fake' http://os:8080/api/v2/entitlement` 应返回 401 |
| persona-query 路由未挂载 | P1-OS Task 4 | `curl http://os:8080/api/v2/sessions/s1/persona` 应返回 200 或 404（非 405） |
| docker-compose.os.yml build context 错误 | P1-OS Task 5.1 | `cd deploy && docker compose -f docker-compose.os.yml config` |
| manifest 与 GatewayConfig 不对齐 | P1-OS Task 5.2 | 对比 `mirage-gateway/deployments/production_ready_manifest.yaml` 与 `cmd/gateway/main.go` GatewayConfig 结构 |
| 缺少 command_secret 等必需配置 | P1-OS Task 5.3 | 检查 manifest Secret 包含 `command_secret`，ConfigMap 包含 `mcc.tls.enabled: true` |

### P1-C: 发布文档

| Finding | Spec Task | 验证命令 |
|---------|-----------|----------|
| 缺少 traceability index | P1-OS Task 6 | 本文档即为验证产物 |
| 旧 spec 完成状态失真 | P1-OS Task 6.2 | 见上方"旧 Spec 适用状态"表 |

### P1-Gateway: Gateway 控制面

| Finding | Spec Task | 验证命令 |
|---------|-----------|----------|
| Gateway 控制面安全收口 | P1-GW 全部任务 | 见 `.kiro/specs/release-readiness-p1-gateway-control/tasks.md` |

## 发布检查点

以下检查点全部通过方可宣布 release ready：

| ID | 检查点 | 验证方式 | 状态 |
|----|--------|----------|------|
| C1 | 证书签发与轮转路径一致 | cert-rotate.sh --check-only + API 签发测试 | ✅ |
| C2 | query surface 认证真实生效 | 裸 X-Client-ID 请求返回 401 | ✅ |
| C3 | persona-query 测试通过 | `go test ./services/persona-query -v` | ✅ |
| C4 | compose/manifest 可被真实验证 | `docker compose config` + manifest 结构对比 | ✅ |
| C5 | release readiness index 发布完成 | 本文档存在且内容完整 | ✅ |

## 结论

本轮发布以 P0/P1 spec 为唯一有效修复拆解。旧 spec 的完成勾选不作为发布依据。
