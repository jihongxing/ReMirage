# 审计整改 Bugfix 需求文档

## Introduction

本文档针对 Mirage Project 全项目上线审计报告（`docs/audit-report.md`）中发现的 3 项 P0 阻断项和 7 项 P1 必须修复项，定义缺陷行为、期望行为和回归保护条件。当前项目状态为 `release_blocked`，所有 P0/P1 问题必须关闭后方可解除阻断。

审计基线来源：
- `docs/audit-report.md`（审计报告 v3）
- `docs/audit-remediation-checklist.md`（整改执行清单）

---

## Bug Analysis

### Current Behavior (Defect)

**P0 阻断项**

1.1 WHEN 对项目中的 Go 模块和 Node 项目执行依赖漏洞扫描时 THEN 系统不存在任何扫描脚本、CI 集成或扫描结果记录，无法提供依赖安全证据（F-P0-01）

1.2 WHEN 读取 `mirage-os/configs/config.yaml` 配置文件时 THEN 系统包含硬编码的危险默认值 `password: postgres` 和 `jwt_secret: change-this-in-production`，若被误用于生产环境将导致数据库凭证暴露和 JWT 可伪造（F-P0-02）

1.3 WHEN 检查工作目录中的构建产物时 THEN 系统存在多份未追踪的 `.exe`/`.dll` 二进制文件，且 `phantom-client/cmd/phantom/wintun.dll`（被 `go:embed` 嵌入最终交付物）的来源版本、下载地址和 SHA256 未记录，无法追溯（F-P0-03）

**P1 必须修复项**

1.4 WHEN Gateway/OS/Client 在生产环境输出日志时 THEN 系统缺少统一的日志脱敏中间件，IP、user_id、token、Authorization、secret、password 等敏感字段可能以明文形式出现在日志中（F-P1-01）

1.5 WHEN Gateway 运行时处理传输编排时 THEN 系统同时存在 `Orchestrator` 和 `TransportManager` 两条并行主链，`source-of-truth-map` 标记为"待收敛 S-01"，调用路径未统一（F-P1-02）

1.6 WHEN 执行 eBPF 编译回归测试（`compile_test.go`）时 THEN 系统仅验证 `L1Stats` 结构体字段与 C 侧对齐，未真正编译任何 `.c` 文件，数据面编译回归无自动化保护（F-P1-03）

1.7 WHEN 生产环境部署 `deploy/docker-compose.os.yml` 中的 Redis 服务时 THEN Redis 仅配置 `redis-server --appendonly yes` 无 `requirepass`，消费方（gateway-bridge、api-server）连接 `redis://redis:6379` 均无密码，任何可达网络均可未授权访问（F-P1-04）

1.8 WHEN 对比 `deploy/docker-compose.os.yml` 与 `deploy/runbooks/secret-injection.md` 的密钥注入方式时 THEN compose 通过环境变量直接注入 DB 密码、JWT、HMAC 密钥，而 runbook 明确要求"密钥不通过普通环境变量长期保存"应走 K8s Secret/Vault，两者口径矛盾（F-P1-05）

1.9 WHEN 对 `pkg/api` 执行 `go test -count=10` 重复测试时 THEN `TestQuotaBucket_IsolationTwoUsers` 间歇性失败，`onExhausted` 回调未触发，说明 `quota_bucket.go` 在配额从正数变为 0 时存在并发竞态（F-P1-06）

1.10 WHEN 在 `benchmarks/` 目录执行 `go test ./...` 时 THEN 系统直接失败并提示 `go: updates to go.mod needed; to update it: go mod tidy`，基准测试无法直接运行（F-P1-08）


### Expected Behavior (Correct)

**P0 阻断项**

2.1 WHEN 对项目中的 Go 模块和 Node 项目执行依赖漏洞扫描时 THEN 系统 SHALL 提供跨平台统一扫描脚本（`scripts/security-scan.sh` + `scripts/security-scan.ps1`），对所有 Go 模块执行 `govulncheck ./...`，对 `sdk/js` 执行 `npm audit --omit=dev`，扫描结果归档到 CI 产物或文档附件，且无未豁免的高危漏洞（F-P0-01）

2.2 WHEN 读取 `mirage-os/configs/config.yaml`（或其替代文件）时 THEN 系统 SHALL 将该文件明确标注为"仅限开发环境"（或改名为 `config.dev.yaml`），移除 `password: postgres` 和 `change-this-in-production` 等危险默认值，且生产部署路径不引用此文件（F-P0-02）

2.3 WHEN 检查工作目录中的构建产物时 THEN 系统 SHALL 清理所有未追踪的 `.exe`/`.dll` 构建产物，对 `wintun.dll` 记录官方来源、版本号、下载地址和 SHA256，将 `phantom-client/cmd/phantom/wintun.dll` 确立为唯一 authoritative 副本（`go:embed` 源），删除 `phantom-client/wintun.dll` 和 `phantom-client/embed/wintun.dll` 多余副本，确认 `Dockerfile.chaos` 中 `RUN touch cmd/phantom/wintun.dll` 占位逻辑不受影响，且 CI 增加 `git status --porcelain` 防误提交检查（F-P0-03）

**P1 必须修复项**

2.4 WHEN Gateway/OS/Client 在生产环境输出日志时 THEN 系统 SHALL 通过统一的 `redact`/`safe-log` 包对 IP、user_id、token、Authorization、secret、password 等敏感字段执行白名单输出规则，确保日志中不出现明文敏感信息（F-P1-01）

2.5 WHEN Gateway 运行时处理传输编排时 THEN 系统 SHALL 以 `Orchestrator` 为唯一主链，将 `TransportManager` 的调用点迁移至 Orchestrator，旧接口仅保留兼容层并显式标记 deprecated（F-P1-02）

2.6 WHEN 执行 eBPF 编译回归测试时 THEN 系统 SHALL 包含独立的 eBPF 编译测试或脚本，至少验证 `clang -target bpf` 可以将关键 `.c` 文件编译为目标产物，并将该测试纳入 CI（F-P1-03）

2.7 WHEN 生产环境部署 Redis 服务时 THEN 系统 SHALL 为 Redis 配置 `requirepass` 或 ACL 鉴权，消费方连接串统一改为 `redis://:${MIRAGE_REDIS_PASSWORD}@redis:6379`，healthcheck 同步改为鉴权探活（F-P1-04）

2.8 WHEN 对比 compose 与 runbook 的密钥注入方式时 THEN 系统 SHALL 统一密钥注入口径，runbook、compose、实际代码三者一致，无互相冲突的说明（F-P1-05）

2.9 WHEN 对 `pkg/api` 执行 `go test -count=10` 重复测试时 THEN 系统 SHALL 稳定通过所有测试，`quota_bucket.go` 在配额从正数变为 0 时正确触发 `onExhausted` 回调，不再出现间歇性失败（F-P1-06）。验证分两层：(a) 确定性单元测试验证"余额归零时必须置 Exhausted 并触发回调"；(b) 压力测试 `-count=10` 验证"不再 flaky"

2.10 WHEN 在 `benchmarks/` 目录执行 `go test ./...` 时 THEN 系统 SHALL 在干净环境下直接运行成功，模块元数据完整，依赖已锁定，不再提示 `go mod tidy`（F-P1-08）

### Unchanged Behavior (Regression Prevention)

3.1 WHEN 现有 CI 流水线执行常规构建和测试时 THEN 系统 SHALL CONTINUE TO 正常编译 Gateway/OS/Client/CLI 所有组件（`go build` + `go vet` 全部 EXIT_CODE=0）

3.2 WHEN 开发环境使用 `deploy/docker-compose.dev.yml` 启动服务时 THEN 系统 SHALL CONTINUE TO 正常启动所有开发服务，开发环境配置不受生产配置整改影响

3.3 WHEN Gateway 通过 Orchestrator 执行传输编排时 THEN 系统 SHALL CONTINUE TO 正常完成路径调度、BBR v3 控制和多路径传输功能

3.4 WHEN 执行现有的 P0 runtime 测试（`tests/p0_runtime/`）时 THEN 系统 SHALL CONTINUE TO 全部通过，包括 bug_exploration 和 preservation 测试

3.5 WHEN 执行 chaos 测试（`deploy/chaos/chaos_test.sh`）时 THEN 系统 SHALL CONTINUE TO 正常运行 genesis 环境和混沌测试

3.6 WHEN 使用 `deploy/release/manifest.go` + `verify.go` 进行发布签名验证时 THEN 系统 SHALL CONTINUE TO 正常执行 Ed25519 签名和验证流程

3.7 WHEN 执行证书轮转（`cert-rotate.sh`）和紧急擦除（`emergency-wipe.sh`）时 THEN 系统 SHALL CONTINUE TO 正常完成安全操作流程

3.8 WHEN `phantom-client` 在 Windows 环境运行时 THEN 系统 SHALL CONTINUE TO 通过 `go:embed` 正确嵌入和提取 `wintun.dll`（来源整改不影响嵌入机制）

3.9 WHEN 非零正数配额的用户执行正常 API 调用时 THEN 系统 SHALL CONTINUE TO 正确计算配额余额并在真正耗尽时触发回调

3.10 WHEN 生产环境启用 mTLS 强制模式（`MIRAGE_ENV=production`）时 THEN 系统 SHALL CONTINUE TO 强制 TLS 和 gRPC TLS，拒绝非加密连接

### Document Closure (审计文档闭环)

4.1 WHEN 每个 P0/P1 Finding 的代码/配置修复完成并通过复验后 THEN 系统 SHALL 将 `docs/audit-report.md` 中对应 Finding 的状态从 `open` 更新为 `verified`，并记录复验命令和结果

4.2 WHEN 所有 P0/P1 Finding 状态更新为 `verified` 后 THEN 系统 SHALL 将 `docs/audit-report.md` 的审计总结论从 `release_blocked` 更新为 `release_ready`

4.3 WHEN 每个整改任务完成后 THEN 系统 SHALL 在 `docs/audit-remediation-checklist.md` 的执行记录模板中回写：负责人（或角色）、修复 PR/Commit、复验命令、复验结果、审计状态（`verified`）

4.4 WHEN 整改涉及文档更新时 THEN 系统 SHALL 同步更新审计报告第九章"文档更新要求"中列出的所有关联文档（`secret-injection.md`、`docker-compose.os.yml`、`config.yaml`、wintun.dll 来源文档）
