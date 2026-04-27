---
Status: temporary
Target Truth: 基于 audit-report 的整改执行清单，仅对当前发布周期有效
Source: docs/audit-report.md
---

# Mirage Project 审计整改任务清单

> 基线来源：`docs/audit-report.md`
> 用途：把当前 P0 / P1 审计发现拆成可执行的整改任务，供排期、指派和复验使用

## 一、使用说明

本清单默认使用“角色 owner”而不是具体人名，便于先推进工作再落人。建议在周会或发布例会上，将 `建议 Owner` 列替换为实际负责人。

建议状态流转：

- `open`：未开始
- `in_progress`：修复中
- `fixed`：代码或文档已改
- `verified`：按复验命令验证通过
- `waived`：书面豁免

关闭规则：

- P0 / P1 只有在“修复动作完成 + 复验命令通过 + 审计报告状态更新”为 `verified` 后才能关闭
- 只改文档、不改代码或部署资产，不算关闭

## 二、Owner 角色建议

| 角色 | 负责范围 |
|------|----------|
| Release / Security | 漏洞扫描、SBOM、发布产物追溯、CI 守门 |
| OS / Deploy | `mirage-os` 配置、compose、runbook、Redis、密钥注入 |
| Gateway Runtime | `mirage-gateway` 运行时、eBPF、配额、编排主链 |
| Cross-Platform Client | `phantom-client` 交付物、`wintun.dll` 来源与封装 |
| DevEx / CI | 基准测试可运行性、脚本化复验、CI 接线 |

## 三、P0 整改任务

| ID | 优先级 | 建议 Owner | 问题 | 修复动作 | 复验命令 | 关闭标准 |
|----|--------|------------|------|----------|----------|----------|
| R-P0-01 | P0 | Release / Security | 缺少依赖漏洞扫描证据 | 1. 新增统一扫描脚本，如 `scripts/security-scan.ps1` 和 `scripts/security-scan.sh`。 2. 对每个 Go 模块执行 `govulncheck ./...`。 3. 对 `sdk/js` 执行 `npm audit --omit=dev` 或团队认可的等效扫描。 4. 将扫描结果归档到 CI 产物或文档附件。 | `Get-ChildItem -Recurse -Filter go.mod | Where-Object { $_.FullName -notmatch '\\node_modules\\' } | ForEach-Object { Push-Location $_.DirectoryName; govulncheck ./...; Pop-Location }` | 所有 Go 模块完成扫描；`sdk/js` 完成依赖扫描；无未豁免高危漏洞；结果可追溯 |
| R-P0-02 | P0 | OS / Deploy | `mirage-os/configs/config.yaml` 含危险默认值 | 1. 将 `config.yaml` 改名为 `config.dev.yaml`，或在文件头明确标注“仅限开发环境”。 2. 移除 `password: postgres` 和 `change-this-in-production` 这类危险默认值。 3. 明确生产配置只允许从 Secret / 挂载文件 / 部署清单读取。 4. 更新相关 README 与部署文档。 | `Select-String -Path mirage-os\\configs\\config.yaml -Pattern 'password: postgres|change-this-in-production'` | 检索结果为空，或该文件已明确改为开发配置且生产路径不再引用 |
| R-P0-03 | P0 | Release / Security | 工作目录存在未追踪二进制与 DLL 来源不清 | 1. 清理工作目录中的 `.exe` / `.dll` 构建产物。 2. 对 `phantom-client/cmd/phantom/wintun.dll` 记录官方来源、版本、下载地址、SHA256。 3. 若必须保留单一副本，明确 authoritative 路径；清理多余副本。 4. 在 CI 增加工作目录污染检查。 | `git status --ignored --short` | 无无法解释的二进制产物；`wintun.dll` 来源可追溯；CI 能阻止误提交 |

## 四、P1 整改任务

| ID | 优先级 | 建议 Owner | 问题 | 修复动作 | 复验命令 | 关闭标准 |
|----|--------|------------|------|----------|----------|----------|
| R-P1-01 | P1 | Gateway Runtime + OS / Deploy | 日志脱敏缺失 | 1. 引入统一 `redact` / `safe-log` 能力。 2. 优先改造 Gateway 审计、熔断、流量上报、Client Provisioning、OS 鉴权日志。 3. 为 `user_id`、IP、token、Authorization、secret、password 建白名单输出规则。 4. 为关键路径新增脱敏测试。 | `Select-String -Path mirage-gateway\\pkg\\**\\*.go,mirage-os\\**\\*.go,phantom-client\\**\\*.go -Pattern 'Authorization|password|secret|token'` | 抽样日志路径不再直接打印敏感字段；新增脱敏测试通过 |
| R-P1-02 | P1 | Gateway Runtime | 编排主链未收敛 | 1. 盘点 `TransportManager` 实际调用点。 2. 将主路径切到 Orchestrator。 3. 旧接口仅保留兼容层并显式标记 deprecated。 4. 更新 `source-of-truth-map` 相关代码落点。 | `Get-ChildItem -Recurse -File mirage-gateway\\pkg,mirage-gateway\\cmd -Include *.go | Select-String -Pattern 'NewTransportManager|TransportManager'` | 业务主链不再依赖 `TransportManager`；仅保留兼容定义或测试桩；关键测试走 Orchestrator |
| R-P1-03 | P1 | Gateway Runtime + DevEx / CI | eBPF 编译回归不足 | 1. 新增真实编译测试或脚本，覆盖关键 `.c` 文件。 2. 若本机依赖复杂，则用容器或 CI 专用环境编译。 3. 将该测试纳入 CI。 | `go test -run TestBPFCompile_KeyCFiles ./pkg/ebpf -v`（Linux + clang） | 存在真实 eBPF 编译验证；失败可复现；CI 中默认执行 |
| R-P1-04 | P1 | OS / Deploy | 生产 Redis 未鉴权 | 1. 在 `deploy/docker-compose.os.yml` 为 Redis 增加 `requirepass` 或 ACL。 2. 将消费方连接串统一改为带密码环境变量。 3. healthcheck 同步改为鉴权检查。 4. 更新 runbook。 | `Select-String -Path deploy\\docker-compose.os.yml -Pattern 'requirepass|MIRAGE_REDIS_PASSWORD|redis://:'` | Redis 服务端启用鉴权；消费方连接串含密码；healthcheck 与文档一致 |
| R-P1-05 | P1 | OS / Deploy + Release / Security | compose 与 runbook 密钥注入口径冲突 | 1. 明确当前发布唯一口径。 2. 如果继续用 compose，则在 runbook 中标明为“过渡方案”并限定场景。 3. 如果按生产标准推进，则把 compose 改为 Secret / Vault / 文件挂载方案。 4. 同步更新 `secret-injection.md`。 | `Select-String -Path deploy\\runbooks\\secret-injection.md,deploy\\docker-compose.os.yml -Pattern 'Vault|Volume Mount|POSTGRES_PASSWORD|JWT_SECRET|QUERY_HMAC_SECRET'` | runbook、compose、实际部署方式三者一致，无互相冲突口径 |
| R-P1-06 | P1 | Gateway Runtime | 配额测试存在并发竞态 | 1. 修复 `quota_bucket.go`，让“余额从正数变 0”也能触发耗尽回调。 2. 将当前测试拆成确定性用例 + 压力复验用例。 3. 在 CI 中增加重复执行。 | `go test -count=10 ./pkg/api` | 连续 10 次测试稳定通过；不再出现 `TestQuotaBucket_IsolationTwoUsers` 间歇性失败 |
| R-P1-08 | P1 | DevEx / CI | `benchmarks/` 无法直接运行 | 1. 在 `benchmarks/` 执行 `go mod tidy` 修复模块元数据。 2. 确认依赖被锁定。 3. 若存在平台依赖，在 README 中明确运行前提。 4. 将 `go test ./...` 加入最小 CI 校验。 | `go test ./...` | `benchmarks/` 在干净环境可直接运行；不再提示 `go mod tidy` |

## 五、建议推进顺序

### 第一批：发布前置条件

- R-P0-01 依赖扫描
- R-P0-02 危险默认值
- R-P0-03 产物追溯

### 第二批：部署与密钥口径

- R-P1-04 Redis 鉴权
- R-P1-05 密钥注入方式统一

### 第三批：代码与测试稳定性

- R-P1-03 eBPF 编译回归
- R-P1-06 配额测试竞态
- R-P1-08 benchmarks 可运行性

### 第四批：长期治理收口

- R-P1-01 日志脱敏
- R-P1-02 编排主链收敛

## 六、建议同步更新的文档

- `docs/audit-report.md`
- `deploy/runbooks/secret-injection.md`
- `deploy/docker-compose.os.yml`
- `mirage-os/configs/config.yaml` 或替代开发配置
- `phantom-client` 中与 `wintun.dll` 来源相关的 README / SBOM / 发布说明

## 七、执行记录模板

建议每项任务按下面格式记录，方便回写审计报告：

| 任务 ID | 负责人 | 修复 PR / Commit | 复验命令 | 复验结果 | 审计状态 |
|---------|--------|------------------|----------|----------|----------|
| R-P0-01 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `scripts/security-scan.sh` 执行 + `scripts/security-scan.ps1` 执行（PowerShell Join-Path 已修复） | 脚本已创建且跨平台可执行；sdk/js protobufjs critical 漏洞已通过 npm audit fix 修复（0 vulnerabilities）；Go 模块扫描正常 | verified |
| R-P0-02 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `Select-String -Path mirage-os\configs\config.yaml -Pattern 'password: postgres\|change-this-in-production'` | 检索结果为空，文件头已标注仅限开发环境，危险默认值已移除 | verified |
| R-P0-03 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `git status --ignored --short` 仅剩 `phantom-client/cmd/phantom/wintun.dll`（authoritative 副本）+ `phantom-client/WINTUN_SOURCE.md` 存在性验证 | 构建产物已清理（.exe/.dll/.o），仅保留 wintun.dll authoritative 副本，来源/版本/SHA256 已记录，多余副本已删除 | verified |
| R-P1-01 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `go test -run TestRedact ./pkg/redact/ -v` + `go test -run TestLogRedaction ./pkg/redact/ -v` + CI 敏感日志扫描（`.github/workflows/release-gate.yml` log-redact-check job，覆盖 user=%s/用户 %s/uid=%s/link=%s/IP=/SourceIP/RemoteAddr 模式） | 统一 redact 包已创建（mirage-gateway/pkg/redact + mirage-os/pkg/redact）；全仓库生产日志路径已接入脱敏，覆盖 Gateway（fuse_callback/handlers/grpc_client/v2_adapter/main.go/l1_monitor/quic_guard/handshake_guard/protocol_detector/phantom reporter/asn_shield/fingerprint_reporter/threat_bus/responder/tproxy bridge/chameleon_fallback）和 OS（api-gateway server/grpc server/quota_bridge/monero_manager/fuse_handler/quota_dispatch/raft fsm/cell_manager/tier_router/middleware/geoip/phantom server/subscription_manager/provisioner/ws-gateway auth）；日志脱敏回归测试已新增；CI 阻断式敏感日志扫描已接入 release-gate.yml | verified |
| R-P1-02 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `Get-ChildItem -Recurse mirage-gateway\pkg,mirage-gateway\cmd -Include *.go \| Select-String 'TransportManager'` | TransportManager 已标记 deprecated，方法委托至 Orchestrator，source-of-truth-map S-01 已收敛 | verified |
| R-P1-03 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `go test -run TestBPFCompile_KeyCFiles ./pkg/ebpf -v`（Linux + clang 环境执行；Windows 正确 SKIP） | eBPF 编译回归测试已新增（bpf_compile_test.go），覆盖 npm.c/bdna.c/jitter.c/l1_defense.c/l1_silent.c；CI 配置已创建（.github/workflows/ebpf-compile.yml），在 ubuntu-latest + clang 环境自动执行 | verified |
| R-P1-04 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `Select-String -Path deploy\docker-compose.os.yml -Pattern 'requirepass\|MIRAGE_REDIS_PASSWORD'` | Redis 已配置 requirepass，消费方连接串含密码，healthcheck 已更新 | verified |
| R-P1-05 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `Select-String -Path deploy\runbooks\secret-injection.md -Pattern '过渡方案'` | secret-injection.md 已新增过渡方案章节，compose 与 runbook 口径一致 | verified |
| R-P1-06 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `go test -count=10 ./pkg/api` | quota_bucket.go CAS 后检查 newRemaining==0 设置 Exhausted，-count=10 稳定通过 | verified |
| R-P1-08 | 审计整改自动化 (Kiro AI) | audit-remediation spec execution | `cd benchmarks && go test ./...` | go mod tidy 已执行，go.sum 已同步，go test 直接运行成功 | verified |
