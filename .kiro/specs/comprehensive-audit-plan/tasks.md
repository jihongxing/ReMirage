# Execution Plan: Mirage Project 全项目上线审计

## Overview

本任务清单描述的是"执行全项目审计并拿到上线结论"的工作，而不是开发新的审计程序。任务按阻断优先、证据优先、复验闭环优先排序。

## Tasks

- [x] 1. 建立审计基线与单一入口
  - [x] 1.1 锁定本轮发布判定入口
    - 确认 `docs/release-readiness-traceability-index.md` 作为当前周期 P0/P1 的唯一发布索引
    - 记录本计划与 `release-readiness-p0-runtime`、`release-readiness-p1-gateway-control`、`release-readiness-p1-os-release-closure` 的关系
    - 明确"当前必须关闭"和"上线后继续收敛"的边界
    - **证据**: traceability index Status: temporary，3 个 spec 全部 tasks 已完成
    - _Requirements: 1.1, 1.6, 12.3_

  - [x] 1.2 建立真相源映射
    - 对照 `docs/governance/source-of-truth-map.md`
    - 为 Gateway、OS、Client、CLI、Proto、SDK、Deploy、Docs 分配审计入口
    - 标记所有"旧资料仅迁移用途、不作为发布依据"的文档
    - **证据**: 审计报告 2.2 真相源映射确认表
    - _Requirements: 1.2, 10.3_

  - [x] 1.3 建立统一 findings / evidence 模板
    - 统一字段：ID、组件、审计域、证据、风险等级、负责人、截止日期、复验方式、状态
    - 定义 `passed / failed / blocked / waived / verified` 状态
    - 约定 P0/P1 未 verified 前一律阻断上线
    - **证据**: 审计报告 Findings 汇总表
    - _Requirements: 1.3, 1.4, 11.1, 11.2, 11.5_

- [x] 2. 收敛当前发布周期阻断项
  - [x] 2.1 复核 P0 runtime 阻断项
    - 复核 Gateway 编译、OS 编译、Proto 兼容性等当前索引中的 P0 项
    - 保存实际验证命令与结果
    - 对未关闭项补充明确 owner 和复验动作
    - **证据**: `go build` 全部 EXIT_CODE=0，`go vet` 全部通过
    - _Requirements: 1.5, 2.2, 12.1_

  - [x] 2.2 复核 P1 Gateway 控制面阻断项
    - 对照 `.kiro/specs/release-readiness-p1-gateway-control/`
    - 检查控制面认证、配置、命令入口和部署相关问题是否已闭环
    - 将复核结果写回统一审计总表
    - **证据**: 10/10 tasks 已完成，5 个检查点全部通过
    - _Requirements: 3.3, 5.4, 12.1_

  - [x] 2.3 复核 P1 OS 发布收口项
    - 对照 `.kiro/specs/release-readiness-p1-os-release-closure/`
    - 复核证书签发、轮转、entitlement/query surface、compose/manifest 对齐、发布文档完整性
    - 确认 `mirage-os/RELEASE_CHECKLIST.md` 的关键项有实际验证证据
    - **证据**: 6/6 tasks 已完成，5 个检查点全部通过
    - _Requirements: 3.5, 8.3, 10.1, 12.1_

- [x] 3. 组件基线可构建与可启动审计
  - [x] 3.1 审计 Gateway 基线
    - 验证 `mirage-gateway/` 的依赖、构建、关键测试、配置文件和最小启动路径
    - 对齐 `cmd/gateway/main.go` 的 `GatewayConfig` 与 `configs/`、`deployments/`、runbook
    - **证据**: `go build ./cmd/gateway/` EXIT_CODE=0，`go vet ./...` 通过，Makefile/Dockerfile 完整
    - **发现**: F-P1-03 eBPF compile_test.go 仅验证结构体字段对齐，未真正编译 eBPF；F-P1-06 TestQuotaBucket_IsolationTwoUsers 失败
    - _Requirements: 2.2, 5.4_

  - [x] 3.2 审计 OS 基线
    - 验证 `mirage-os/` 的 Go 服务、`api-server/`、`web/`、`gateway-bridge/`、数据库迁移和 compose
    - 复核 `RELEASE_CHECKLIST.md` 的 10 项主干能力
    - **证据**: `go build ./...` EXIT_CODE=0，docker-compose.yaml 完整，RELEASE_CHECKLIST 10 项 + smoke-test.sh
    - _Requirements: 2.3, 8.4, 10.4_

  - [x] 3.3 审计 Phantom Client 基线
    - 验证 `phantom-client/` 的构建、关键模块、交付文件和最小运行边界
    - 检查 Windows 相关依赖如 `wintun.dll` 的交付合理性和版本来源
    - **证据**: `go build ./cmd/phantom/` EXIT_CODE=0，wintun 通过 go:embed 嵌入
    - **发现**: wintun.dll 存在 3 份副本，来源版本未记录（纳入 F-P0-03）
    - _Requirements: 2.2, 4.2_

  - [x] 3.4 审计 CLI / Proto / SDK 基线
    - 验证 `mirage-cli/` 的构建和命令边界
    - 验证 `mirage-proto/` 的生成链路或生成产物一致性
    - 盘点 `sdk/` 各语言包的状态、依赖文件和交付边界
    - **证据**: CLI `go build ./...` 通过，Proto 2 个 .proto 文件完整，SDK 9 语言均有依赖文件
    - **发现**: F-P2-01 SDK 无统一版本号
    - _Requirements: 2.4, 2.5, 9.5_

- [x] 4. 认证、授权与密钥管理审计
  - [x] 4.1 审计密钥注入路径
    - 对照 `deploy/runbooks/secret-injection.md`
    - 检查 JWT、internal HMAC、command secret、TLS 证书的来源、默认值、示例值和运行时注入方式
    - 扫描仓库、compose、示例配置和脚本中的明文密钥
    - **证据**: 生产 compose 使用 `${ENV_VAR}` 无默认值，Gateway 启动强制 CommandSecret 非空
    - **发现**: F-P0-02 config.yaml 硬编码 postgres 密码和 JWT secret；F-P1-05 compose 与 runbook 密钥注入口径矛盾
    - _Requirements: 3.1, 3.2, 3.6_

  - [x] 4.2 审计 Gateway 与 OS 认证链路
    - 验证 command auth、internal-hmac、JWT、WebSocket 鉴权、管理 API 鉴权
    - **证据**: P1 spec 已覆盖 command HMAC 收紧、rate limiter 修复、query surface 认证
    - _Requirements: 3.3, 3.4, 3.6_

  - [x] 4.3 审计证书签发与轮转一致性
    - 对照 `deploy/certs/`、`deploy/scripts/cert-rotate.sh`、`deploy/docker-compose.os.yml`、Gateway/OS 配置
    - 验证目录口径、签发接口、轮转脚本、证书用途和回滚说明一致
    - **证据**: cert-rotate.sh 区分 api/local 模式，证书目录统一，compose 挂载一致
    - _Requirements: 3.5, 8.2, 8.3_

- [x] 5. 运行时安全与数据安全审计
  - [x] 5.1 审计日志与错误回显
    - 检查 Gateway、OS、Client 的日志是否脱敏
    - **证据**: 生产代码无 panic 调用，log.Fatal 仅在启动阶段
    - **发现**: F-P1-01 未发现统一日志脱敏中间件
    - _Requirements: 4.1, 4.4, 4.5_

  - [x] 5.2 审计敏感数据存储与缓存
    - 审计 PostgreSQL、Redis、BoltDB 或同类存储的敏感数据处理
    - **证据**: 生产 compose 无默认 DB 密码，tmpfs 部署可用
    - **发现**: F-P1-04 deploy/docker-compose.os.yml Redis 无 requirepass
    - _Requirements: 4.3, 4.6_

  - [x] 5.3 审计 Client / Gateway 运行时防护
    - 检查 self-destruct、burn、killswitch、memsafe、secure wipe 等能力
    - **证据**: RAM Shield 启用，反调试实现，emergency-wipe.sh 7 步完整擦除
    - _Requirements: 4.2, 4.6_

- [x] 6. 协议、边界与跨组件契约审计
  - [x] 6.1 审计治理边界与职责归属
    - 对照 `docs/governance/boundaries/` 审计职责分层
    - **证据**: boundaries/ 6 份文档全部 authoritative
    - **发现**: F-P1-02 Orchestrator vs TransportManager 并存
    - _Requirements: 5.1, 5.5_

  - [x] 6.2 审计协议实现与文档一致性
    - 对照 `docs/protocols/*.md` 审计 6 大协议代码落点
    - **证据**: 协议语言分工正确，7 个 bpf/*.c 文件，Go-C 通过 Map/RingBuffer 通信
    - _Requirements: 5.2, 10.2_

  - [x] 6.3 审计跨组件契约
    - 检查 `mirage-proto/*.proto` 与 Go/TS 调用的一致性
    - **证据**: mirage.proto + control_command.proto 定义完整，API 契约文档 authoritative
    - _Requirements: 5.3, 10.2_

- [x] 7. 代码质量、静态检查与测试保护审计
  - [x] 7.1 审计 Go / TypeScript / C 关键代码质量
    - **证据**: 4 个 Go 模块 go vet 全部通过，无 panic 使用
    - **发现**: F-P1-06 TestQuotaBucket_IsolationTwoUsers 失败（配额耗尽回调逻辑缺陷）
    - _Requirements: 6.1, 6.2, 6.3, 6.4_

  - [x] 7.2 盘点关键测试覆盖
    - **证据**: p0_runtime 测试通过，OS 含 6 个场景测试，chaos 含 genesis 环境
    - **发现**: F-P1-08 benchmarks go.mod 需要 tidy，不可直接运行
    - _Requirements: 6.5, 7.1_

  - [x] 7.3 标记测试缺口与未被保护的上线路径
    - **证据**: F-P1-03 eBPF 编译测试仅验证结构体；F-P1-06 配额/熔断路径回归失败
    - _Requirements: 6.6, 11.4_

- [x] 8. 性能、稳定性与恢复能力审计
  - [x] 8.1 审计性能与资源基线
    - **证据**: benchmarks/ 含 eBPF 延迟、FEC、GSwitch、资源基准脚本
    - **发现**: F-P1-08 benchmarks 不可直接运行
    - _Requirements: 7.1, 7.2_

  - [x] 8.2 审计运行时恢复路径
    - **证据**: RELEASE_CHECKLIST 第 10 项覆盖重启恢复，3 份 runbook authoritative，cert-rotate.sh 完整
    - _Requirements: 7.3, 7.4, 8.4_

  - [x] 8.3 审计最小可观测与排障能力
    - **证据**: gateway-healthcheck.sh 完整实现，CLI 含 diag/health/status/tunnel 命令
    - _Requirements: 7.4, 10.4_

- [x] 9. 部署资产、最小权限与回滚审计
  - [x] 9.1 审计 compose / Dockerfile / 环境变量一致性
    - **证据**: deploy/docker-compose.os.yml build context 正确，证书挂载 :ro，healthcheck 完整
    - **发现**: F-P1-04 Redis 无鉴权；F-P1-05 密钥注入口径矛盾
    - _Requirements: 8.1, 8.3_

  - [x] 9.2 审计 runbook 与脚本闭环
    - **证据**: 3 份 runbook 全部 authoritative，对应脚本存在且完整
    - _Requirements: 8.2, 8.5, 10.1_

  - [x] 9.3 审计回滚与应急动作
    - **证据**: emergency-wipe.sh 7 步焦土协议，cert-rotate.sh 含备份+回滚
    - _Requirements: 8.4, 8.6, 10.4_

- [x] 10. 供应链、依赖与发布产物审计
  - [x] 10.1 审计依赖安全与锁文件
    - **证据**: 各组件 go.sum 存在，sdk/js/package-lock.json 存在
    - **发现**: F-P0-01 无 govulncheck/npm audit 执行记录
    - _Requirements: 9.1, 9.5, 9.6_

  - [x] 10.2 审计发布产物与签名链
    - **证据**: deploy/release/manifest.go + verify.go 实现 Ed25519 签名
    - **发现**: F-P0-03 工作目录存在多份不可追溯构建产物，wintun.dll 来源未记录
    - _Requirements: 9.2, 9.3, 9.4, 9.6_

  - [x] 10.3 审计许可证与第三方交付约束
    - **证据**: 根目录 LICENSE 文件存在
    - _Requirements: 9.5_

- [x] 11. 文档、部署说明与操作面收口
  - [x] 11.1 审计架构与协议文档
    - **证据**: protocols/ 6 个协议文档全部收敛，source-of-truth.md 明确各协议主源
    - _Requirements: 10.2, 10.5_

  - [x] 11.2 审计 API 契约与发布资料
    - **证据**: entitlement-contract.md + topology-contract.md authoritative
    - **发现**: F-P0-02 + F-P1-05 + F-P1-07 配置/compose 与 runbook 口径矛盾
    - _Requirements: 10.1, 10.2, 10.3_

  - [x] 11.3 审计一线运维可执行性
    - **证据**: DEPLOYMENT.md 完整，RELEASE_CHECKLIST 10 项 + smoke-test.sh，3 份 runbook 完整
    - _Requirements: 10.4, 10.5_

- [x] 12. 统一出具 Findings、复验和上线结论
  - [x] 12.1 汇总所有 Findings
    - **证据**: `docs/audit-report.md` Findings 汇总表（3 P0 + 8 P1 + 4 P2 + 2 P3）
    - _Requirements: 11.1, 11.2_

  - [x] 12.2 执行 P0/P1 复验
    - **证据**: 3 个 release-readiness spec 全部复验通过，编译/vet 全部 EXIT_CODE=0
    - **注意**: 新发现的 F-P0-01~03 和 F-P1-01~08 为本轮审计新增，尚未修复
    - _Requirements: 11.3, 11.5_

  - [x] 12.3 形成最终上线判定
    - **结论**: `release_blocked`（3 P0 + 8 P1 未关闭）
    - **产物**: `docs/audit-report.md` (v2，已修正初版误报/漏报/重复计数)
    - _Requirements: 12.1, 12.2, 12.3, 12.4_

## Notes

- 本计划优先服务"当前版本是否可上线"的决策
- 现有 `release-readiness-*` spec 属于本计划的 Phase 1 输入
- `docs/governance/source-of-truth-map.md` 是所有文档对齐检查的入口
- 对没有证据的"通过"一律按未通过处理
- 只要存在未关闭的 P0/P1，最终结论必须是 `release_blocked`
- v2 修正：补充了已提交二进制、Redis 未鉴权、compose 密钥注入矛盾、JWT 默认值、配额测试失败、benchmarks 不可运行等遗漏；修正了 SUPPLY-01 误报和 eBPF 测试重复计数
