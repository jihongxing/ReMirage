# Mirage 功能确认与功能验证任务清单

> 用途：仅用于当前发布周期的功能确认与功能验证，不包含任何新功能开发。
> 范围：只验证当前仓库中已经存在、已经声明可发布的能力。
> 原则：优先验证主链路、拒绝新增功能、优先复用现有测试与脚本。

## 一、使用方式

本清单分为两类目标：

- 功能确认：确认“这个能力是否已存在、是否属于本次发布承诺、是否有唯一验证入口”
- 功能验证：确认“这个能力在当前代码与配置下是否可通过复验命令稳定通过”

建议执行顺序：

1. 先执行“发布基线可用”
2. 再执行所有 `P0` 项，确认上线主链路
3. 最后执行所有 `P1` 项，补齐稳定性与防回归证据

建议负责人角色说明：

- `Release / QA`：发布前总体验证、证据归档、结果汇总
- `Gateway Runtime`：`mirage-gateway` 相关功能与数据面验证
- `OS / Control Plane`：`mirage-os`、`gateway-bridge`、控制面与配置验证
- `Security / Platform`：鉴权、日志脱敏、扫描门禁、发布门禁验证

## 二、功能确认与功能验证任务表

| 功能 | 确认目标 | 复验命令 | 通过标准 | 风险等级 | 证据文件 | 执行负责人 |
|------|----------|----------|----------|----------|----------|------------|
| 发布基线可用 | 当前发布承诺的 Gateway、OS、Client、CLI 均可构建并通过主回归 | `powershell -ExecutionPolicy Bypass -File .\scripts\release-verify.ps1` | 15 项检查全部 `PASS`，无 `FAIL` | `P0` | `scripts/release-verify.ps1` | `Release / QA` |
| Gateway 注册与重注册 | Gateway 首次注册、重复注册更新、离线判定链路可用 | `cd mirage-os/gateway-bridge && go test ./pkg/topology -run "TestGatewayRegister_MemoryQueryable|TestGatewayReRegister_DownlinkAddrUpdate|TestGatewayOffline_HeartbeatTimeout" -v` | 3 个测试全部通过；注册信息可查询；重注册可刷新地址与 Cell；超时网关会转离线 | `P0` | `mirage-os/gateway-bridge/pkg/topology/control_plane_test.go` | `OS / Control Plane` |
| 心跳与威胁上报入口 | 心跳上报、流量上报、威胁上报至少具备输入校验与拒绝能力 | `cd mirage-os/gateway-bridge && go test ./pkg/grpc -run "TestProperty_InvalidRequestRejection|TestReportTraffic_EmptyGatewayID|TestReportThreat_EmptyGatewayID" -v` | 非法请求与空 `gateway_id` 请求被拒绝，测试全部通过 | `P0` | `mirage-os/gateway-bridge/pkg/grpc/server_test.go` | `OS / Control Plane` |
| 内部接口鉴权 | OS 内部 REST Secret 校验与访问日志行为正确 | `cd mirage-os/gateway-bridge && go test ./pkg/rest -run "TestInternalAuthMiddleware_ValidSecret|TestInternalAuthMiddleware_InvalidSecret|TestAccessLogMiddleware" -v` | 正确 secret 放行；错误 secret 返回未授权；访问日志链路正常 | `P0` | `mirage-os/gateway-bridge/pkg/rest/middleware_test.go` | `Security / Platform` |
| Gateway 命令鉴权 | HMAC、时间戳、Nonce、高风险命令校验均有效 | `cd mirage-gateway && go test ./pkg/api -run "TestSecurityRegression_" -v` | 无签名、坏签名、过期时间戳、缺失 nonce、重放请求均被拒绝；合法请求通过 | `P0` | `mirage-gateway/pkg/api/security_regression_test.go` | `Security / Platform` |
| 配额扣减与告警 | 配额计算与威胁严重度动作映射正确 | `cd mirage-os && go test ./services/api-gateway -run "TestProperty_QuotaDeductionAndWarning|TestProperty_SeverityToActionMapping" -v` | 扣减结果与期望一致；严重度到动作映射稳定正确 | `P1` | `mirage-os/services/api-gateway/gateway_service_test.go` | `OS / Control Plane` |
| 配额隔离与熔断 | 单用户耗尽不会影响其他用户，熔断仅作用目标用户 | `cd mirage-gateway && go test -count=10 ./pkg/api -run "TestQuotaBucket_IsolationTwoUsers|TestFuseCallback_ExhaustedTriggersDisconnect|TestFuseCallback_OnlyAffectsTargetUser|TestIntegration_MultiUserQuotaIsolation" -v` | 连续 10 次通过；无用户串扰；耗尽回调稳定触发 | `P0` | `mirage-gateway/pkg/api/quota_bucket_test.go`、`mirage-gateway/pkg/api/fuse_callback_test.go`、`mirage-gateway/pkg/api/integration_test.go` | `Gateway Runtime` |
| 配额下发到网关 | OS 下发 quota 后 Gateway Desired State 正确更新 | `cd mirage-os/gateway-bridge && go test ./pkg/grpc -run "TestPushQuota_UpdatesDesiredState|TestPushQuota_PreservesOtherFields" -v` | 配额字段被更新，其他状态字段不被破坏 | `P1` | `mirage-os/gateway-bridge/pkg/grpc/downlink_test.go` | `OS / Control Plane` |
| Provisioning 阅后即焚 | 交付链接存在性、过期、清理、一致性逻辑正确 | `cd mirage-os && go test ./services/provisioning -run "TestBurnLink_|TestCleanExpiredLinks" -v` | 链接存在时可查询；过期会失效；清理后仅移除过期项 | `P0` | `mirage-os/services/provisioning/provisioner_test.go` | `OS / Control Plane` |
| WebSocket JWT 鉴权 | 缺失 token、坏 token、空 secret、健康检查旁路行为正确 | `cd mirage-os && go test ./services/ws-gateway -run "TestJWTAuth_" -v` | `/ws` 无 token 或坏 token 返回 `401`；`/health` 可旁路 | `P0` | `mirage-os/services/ws-gateway/auth_test.go` | `Security / Platform` |
| 订阅到期处理 | 到期降级、余额不足降级、自动续费保级逻辑正确 | `cd mirage-os && go test ./services/billing -run "TestProperty_Expired" -v` | 到期且不续费时降级；余额不足时降级；自动续费且余额充足时保持等级 | `P1` | `mirage-os/services/billing/subscription_manager_test.go` | `OS / Control Plane` |
| eBPF 编译回归 | 关键 `.c` 文件在 Linux + clang 环境可真实编译 | `cd mirage-gateway && go test -run TestBPFCompile_KeyCFiles ./pkg/ebpf -v` | Linux 下真实编译通过；非 Linux 环境正确 `SKIP` | `P0` | `mirage-gateway/pkg/ebpf/bpf_compile_test.go` | `Gateway Runtime` |
| eBPF 本地双证据 | 除 Go 测试外，本地脚本也可独立验证关键文件编译 | `bash ./mirage-gateway/scripts/test-ebpf-compile.sh` | `npm.c`、`bdna.c`、`jitter.c`、`l1_defense.c`、`l1_silent.c` 全部编译成功，退出码为 0 | `P1` | `mirage-gateway/scripts/test-ebpf-compile.sh` | `Gateway Runtime` |
| 日志脱敏功能边界 | 关键日志脱敏能力与回归测试仍成立 | `cd mirage-gateway && go test -v ./pkg/redact/ && cd ../mirage-os && go test -v ./pkg/redact/` | Gateway 与 OS 两侧 redact 测试全部通过，无明文泄露回归 | `P1` | `mirage-gateway/pkg/redact/log_redaction_test.go`、`mirage-gateway/pkg/redact/redact_test.go`、`mirage-os/pkg/redact/redact_test.go` | `Security / Platform` |
| 生产配置鉴权闭环 | Redis 鉴权、连接串、健康检查配置一致 | `Select-String -Path deploy\\docker-compose.os.yml -Pattern 'requirepass|MIRAGE_REDIS_PASSWORD|redis://:'` | Redis 启用 `requirepass`；消费方连接串带密码；healthcheck 使用鉴权探活 | `P0` | `deploy/docker-compose.os.yml`、`deploy/runbooks/secret-injection.md` | `OS / Control Plane` |
| 自动降级/回升演练 | ClientOrchestrator QUIC→WSS 降级/回升端到端验证 | `bash deploy/scripts/drill-m2-degradation.sh` | 降级集成测试、全失败测试、日志验证测试、Property 2/3 PBT 全部通过 | `P0` | `deploy/evidence/m2-degradation-drill.log`、`phantom-client/pkg/gtclient/client_orchestrator_test.go` | `Gateway Runtime` |
| 节点阵亡恢复演练 | 节点阵亡→RecoveryFSM→L1/L2/L3 恢复链路验证 | `bash deploy/scripts/drill-m3-node-death.sh` | 节点阵亡演练测试、RecoveryFSM 阶段测试、Property 1/6/7 PBT 全部通过 | `P0` | `deploy/evidence/m3-node-death-drill.log`、`phantom-client/pkg/gtclient/node_death_drill_test.go`、`phantom-client/pkg/gtclient/recovery_fsm_test.go` | `Gateway Runtime` |
| 业务连续性样板 | switchWithTransaction 事务式切换 + 业务数据流连续性验证 | `bash deploy/scripts/drill-m4-continuity.sh` | switchWithTransaction 测试、业务连续性样板测试、Property 8/9 PBT 全部通过 | `P0` | `deploy/evidence/m4-continuity-drill.log`、`deploy/evidence/m4-continuity-report.md`、`phantom-client/pkg/gtclient/business_continuity_test.go` | `Gateway Runtime` |
| 隐匿实验方案冻结 | M5 实验方案和表述边界文档完整性 | `bash deploy/scripts/drill-m5-experiment-plan.sh` | 8 项检查全部通过 | `P1` | `docs/reports/stealth-experiment-plan.md` + `docs/reports/stealth-claims-boundary.md` | `Gateway Runtime` |
| 隐匿实验结果 | M6 四个检测面实验执行与结果产出 | `bash deploy/scripts/drill-m6-experiment.sh` | PBT 全部通过，分析脚本可运行 | `P1` | `docs/reports/stealth-experiment-results.md` + `artifacts/dpi-audit/` | `Gateway Runtime` |
| eBPF 覆盖图与性能证据 | M7 覆盖图文档完整性与性能数据采集 | `bash deploy/scripts/drill-m7-ebpf-coverage.sh` | 覆盖图章节完整，关键程序已列出 | `P1` | `docs/reports/ebpf-coverage-map.md` + `artifacts/ebpf-perf/` | `Gateway Runtime` |
| 部署等级与基线清单冻结 | 部署等级文档和基线检查清单已完成且检查项可执行 | `bash deploy/scripts/drill-m8-baseline.sh` | 部署等级文档和基线检查清单已完成且检查项可执行 | `P1` | `docs/reports/deployment-tiers.md`、`docs/reports/deployment-baseline-checklist.md` | `Security / Platform` |
| 准入控制联合演练 | 两个演练场景关键验证点全部通过、5 个 PBT 各 100 次通过、3 个 Critical Tests 通过、Redis 鉴权配置一致 | `bash deploy/scripts/drill-m9-joint-drill.sh` | 两个演练场景关键验证点全部通过、5 个 PBT 各 100 次通过、3 个 Critical Tests 通过、Redis 鉴权配置一致。Smoke test 入口：HMAC 回归 / JWT 回归 / 脱敏回归 / 配额隔离 / Redis 鉴权连通性。Critical test 入口：TestCritical_IllegalRequestNoQuotaImpact / TestCritical_FuseLogRedaction / TestCritical_QuotaReactivationE2E（所属部署等级 All） | `P1` | `docs/reports/access-control-joint-drill.md`、`deploy/evidence/m9-joint-drill.log` | `Gateway Runtime` + `Security / Platform` |
| M10 证据审计报告 | 七域证据盘点报告和 capability-truth-source.md 回写完成 | `bash deploy/scripts/drill-m10-evidence-audit.sh` | 七域证据文件存在性检查通过、CTS 回写完整性检查通过 | `P1` | `docs/reports/phase4-evidence-audit.md`、`docs/governance/capability-truth-source.md` | `Release / QA` |
| M11 跨文档一致性 | cross-document-consistency.md 存在、Market_Positioning 和 Defense_Matrix 对齐声明存在 | `bash deploy/scripts/drill-m11-convergence.sh` | 七域一致性核对通过、对齐声明存在、无绝对化违规词 | `P1` | `docs/reports/cross-document-consistency.md`、`docs/governance/market-positioning-scenarios.md`、`docs/暗网基础设施防御力评价矩阵.md` | `Release / QA` |
| M12 发布门禁 | evidence.go 和 evidence_test.go 存在、release-verify.ps1 新增 Gate 可执行、Remediation_Roadmap Status 为 completed | `bash deploy/scripts/drill-m12-release-gate.sh` | Go 测试通过（含 PBT）、Remediation_Roadmap Status 为 completed | `P1` | `deploy/release/evidence.go`、`deploy/release/evidence_test.go`、`scripts/release-verify.ps1`、`docs/governance/capability-gap-remediation-roadmap.md` | `Release / QA` |

## 三、执行记录

| 功能 | 执行日期 | 执行人 | 执行结果 | 证据链接/附件 | 备注 |
|------|----------|--------|----------|---------------|------|
| 发布基线可用 | 2026-04-24 | Release / QA | PASS | `scripts/release-verify.ps1` | 15 项检查全部 PASS，无 FAIL |
| Gateway 注册与重注册 | 2026-04-24 | OS / Control Plane | PASS | `mirage-os/gateway-bridge/pkg/topology/control_plane_test.go` | 3 个测试全部通过 |
| 心跳与威胁上报入口 | 2026-04-24 | OS / Control Plane | PASS | `mirage-os/gateway-bridge/pkg/grpc/server_test.go` | 100 次 property test + 2 个单元测试全部通过 |
| 内部接口鉴权 | 2026-04-24 | Security / Platform | PASS | `mirage-os/gateway-bridge/pkg/rest/middleware_test.go` | 4 个测试全部通过，含访问日志链路 |
| Gateway 命令鉴权 | 2026-04-24 | Security / Platform | PASS | `mirage-gateway/pkg/api/security_regression_test.go` | 7 个安全回归测试全部通过 |
| 配额隔离与熔断 | 2026-04-24 | Gateway Runtime | PASS | `mirage-gateway/pkg/api/quota_bucket_test.go` 等 | -count=10 连续 40 次全部通过，无串扰 |
| Provisioning 阅后即焚 | 2026-04-24 | OS / Control Plane | PASS | `mirage-os/services/provisioning/provisioner_test.go` | 4 个测试全部通过 |
| WebSocket JWT 鉴权 | 2026-04-24 | Security / Platform | PASS | `mirage-os/services/ws-gateway/auth_test.go` | 4 个测试全部通过，含 /health 旁路 |
| eBPF 编译回归 | 2026-04-24 | Gateway Runtime | SKIP | `mirage-gateway/pkg/ebpf/bpf_compile_test.go` | Windows 环境正确 SKIP，需 Linux 环境复验 |
| 生产配置鉴权闭环 | 2026-04-24 | OS / Control Plane | PASS | `deploy/docker-compose.os.yml` | Redis requirepass 已启用，连接串带密码，healthcheck 使用鉴权探活 |
| 配额扣减与告警 | 2026-04-24 | OS / Control Plane | PASS | `mirage-os/services/api-gateway/gateway_service_test.go` | 2 个 property test 各 100 次全部通过 |
| 配额下发到网关 | 2026-04-24 | OS / Control Plane | PASS | `mirage-os/gateway-bridge/pkg/grpc/downlink_test.go` | 2 个测试全部通过，状态字段不被破坏 |
| 订阅到期处理 | 2026-04-24 | OS / Control Plane | PASS | `mirage-os/services/billing/subscription_manager_test.go` | 3 个 property test 各 100 次全部通过 |
| 日志脱敏功能边界 | 2026-04-24 | Security / Platform | PASS | `mirage-gateway/pkg/redact/` + `mirage-os/pkg/redact/` | Gateway 14 个 + OS 8 个测试全部通过 |
| eBPF 本地双证据 | 2026-04-24 | Gateway Runtime | PASS | `mirage-gateway/scripts/test-ebpf-compile.sh` | 修复 3 个编译问题后 5/5 全部通过：bdna.c 16-bit atomic→32-bit；jitter.c/common.h sdiv→手动符号+udiv；l1_silent.c 内联 icmphdr 替代 linux/icmp.h |
| 自动降级/回升演练 | 2026-04-25 | Gateway Runtime | PASS | `deploy/evidence/m2-degradation-drill.log`、`phantom-client/pkg/gtclient/client_orchestrator_test.go` | bash 演练已通过；集成测试、日志断言、Property 2/3 PBT 各 100 次通过；脚本已兼容 Windows Go fallback |
| 节点阵亡恢复演练 | 2026-04-25 | Gateway Runtime | PASS | `deploy/evidence/m3-node-death-drill.log`、`phantom-client/pkg/gtclient/node_death_drill_test.go`、`phantom-client/pkg/gtclient/recovery_fsm_test.go` | bash 演练已通过；节点阵亡演练、RecoveryFSM 阶段测试、Property 1/6/7 PBT 各 100 次通过；脚本已兼容 Windows Go fallback |
| 业务连续性样板 | 2026-04-25 | Gateway Runtime | PASS | `deploy/evidence/m4-continuity-drill.log`、`deploy/evidence/m4-continuity-report.md`、`phantom-client/pkg/gtclient/business_continuity_test.go` | bash 演练已通过；switchWithTransaction、业务连续性测试、Property 8/9 PBT 与 WithOrchestrator PBT 各 100 次通过；脚本已兼容 Windows Go fallback |
| 隐匿实验方案冻结 | — | Gateway Runtime | 待执行 | `docs/reports/stealth-experiment-plan.md` + `docs/reports/stealth-claims-boundary.md` | M5 文档已创建，drill 脚本已通过；待 Linux 环境复验 |
| 隐匿实验结果 | — | Gateway Runtime | 待执行 | `docs/reports/stealth-experiment-results.md` + `artifacts/dpi-audit/` | M6 实验设计完成，PBT 已通过（mock）；受控环境抓包与分析待 Linux 环境执行 |
| eBPF 覆盖图与性能证据 | — | Gateway Runtime | 待执行 | `docs/reports/ebpf-coverage-map.md` + `artifacts/ebpf-perf/` | M7 覆盖图已产出；性能数据占位文件已创建，待 Linux 环境实际采集 |
| 部署等级与基线清单冻结 | 2026-04-24 | Security / Platform | PASS | `docs/reports/deployment-tiers.md`、`docs/reports/deployment-baseline-checklist.md` | M8 部署等级文档和基线检查清单已完成，三个等级定义完整，检查项可执行 |
| 准入控制联合演练 | 2026-04-24 | Gateway Runtime + Security / Platform | PASS | `docs/reports/access-control-joint-drill.md`、`deploy/evidence/m9-joint-drill.log` | M9 两个演练场景全部通过；5 个 PBT 各 100 次通过；3 个 Critical Tests 通过；Redis 鉴权配置一致 |

## 四、收口建议

- `P0` 项全部通过后，才能进入发布候选确认
- `P1` 项若失败，不建议直接发布，应先判断是否影响当前发布承诺
- 本清单只允许补测试、补验证、补文档、补脚本，不允许借机扩展功能范围
- 若某项验证入口发生变化，应优先更新本清单，而不是靠口头同步
