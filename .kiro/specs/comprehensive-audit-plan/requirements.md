# 需求文档：Mirage Project 全项目上线审计计划

## 简介

本规格定义 Mirage Project 面向正式上线的完整审计计划。目标不是再开发一套“审计平台”，而是基于当前仓库真实结构、现有发布资料和已存在的 release-readiness spec，对整个项目执行一轮可落地、可追踪、可阻断发布的全面审计，最终给出是否达到“可上线”标准的结论。

本计划覆盖以下代码与交付面：

- `mirage-gateway/`
- `mirage-os/`
- `phantom-client/`
- `mirage-cli/`
- `mirage-proto/`
- `sdk/`
- `deploy/`
- `tests/`
- `benchmarks/`
- `docs/`

本计划与当前发布周期的 `docs/release-readiness-traceability-index.md` 协同工作：

- 当前周期的 P0/P1 发布阻断项，以 traceability index 和对应 release-readiness spec 为准
- 本计划作为“全项目上线审计总纲”，补足组件、运行时、运维、供应链和文档治理的完整闭环
- 本计划不得与 `docs/governance/source-of-truth-map.md` 冲突，若发现冲突，以真相源地图定义的 authoritative 入口为准

## 审计目标

1. 确认项目在构建、部署、认证、协议实现、数据安全、运维恢复、供应链和文档层面满足生产发布最低要求
2. 把现有零散的发布清单、专项整改 spec、runbook 和代码验证动作收敛为同一套审计闭环
3. 对所有发现形成分级、证据、负责人、修复期限和复验状态，确保审计结果可以真实驱动上线决策
4. 给出明确的 release ready / release blocked 结论，而不是只产出“建议”

## 非目标

- 不在本规格范围内开发新的通用审计引擎、审计服务或审计 DSL
- 不替代现有专项 spec 的实现细节，只定义它们如何纳入总审计闭环
- 不把临时发布索引误写成长期真相源

## 术语表

- **Audit Plan**: 本次全项目上线审计的总计划
- **Audit Finding**: 审计发现，必须带有证据、风险等级、归属模块和整改建议
- **Evidence**: 证明某项检查通过或失败的客观材料，例如命令输出、测试结果、代码位置、配置对照、截图
- **Release Gate**: 发布门禁项，未通过时必须阻断上线
- **Truth Source**: 真相源，定义某一问题域应以哪个文件或代码位置为准
- **P0**: 阻断上线，必须在发布前修复并复验
- **P1**: 上线前必须修复，否则不能宣布 release ready
- **P2**: 允许上线后限期修复，但必须有明确 owner 和 deadline
- **P3**: 建议改进项，不阻断当前发布

## 需求

### 需求 1：审计治理与发布门禁

**用户故事：** 作为项目负责人，我需要一套与当前发布周期一致的审计治理规则，以便所有人知道什么会阻断上线、什么属于后续整改。

#### 验收标准

1. WHEN 本轮审计启动时, THE Audit Plan SHALL 将 `docs/release-readiness-traceability-index.md` 视为当前发布周期 P0/P1 阶段的唯一发布索引
2. THE Audit Plan SHALL 对齐 `docs/governance/source-of-truth-map.md`，为每一个检查项指定对应的 authoritative 入口
3. THE Audit Plan SHALL 定义统一风险分级：P0、P1、P2、P3，并明确 P0/P1 阻断上线
4. THE Audit Plan SHALL 为每一个 Audit Finding 记录模块、证据、负责人、修复方案、目标日期和复验状态
5. WHEN 任意发布门禁项未通过时, THE Audit Plan SHALL 将总体状态标记为 `release_blocked`
6. THE Audit Plan SHALL 区分“本轮上线必须完成”的阻断审计与“上线后继续收敛”的长期治理项

### 需求 2：范围盘点与基线可安装性审计

**用户故事：** 作为发布负责人，我需要先确认全仓库的构建、依赖和部署基线可工作，避免在安全和协议审计前就存在基础失效。

#### 验收标准

1. THE Audit Plan SHALL 覆盖 `mirage-gateway`、`mirage-os`、`phantom-client`、`mirage-cli`、`mirage-proto`、`sdk`、`deploy`、`tests`、`benchmarks` 和 `docs`
2. THE Audit Plan SHALL 验证核心 Go 模块至少具备可解析依赖、可编译或可测试的基本能力
3. THE Audit Plan SHALL 验证 `mirage-os` 的 Docker Compose、数据库迁移、Redis 依赖和 Web/API 入口具备最小启动路径
4. THE Audit Plan SHALL 验证 `mirage-proto` 的协议文件可以生成或与当前生成产物保持一致
5. THE Audit Plan SHALL 检查 `sdk/` 中各语言 SDK 是否至少具备版本清单、依赖文件或 README 级别的可交付边界说明
6. IF 核心组件无法构建、无法启动或依赖无法解析, THEN THE Audit Plan SHALL 至少标记为 P1 风险

### 需求 3：认证、授权与密钥管理审计

**用户故事：** 作为安全负责人，我需要确认系统的认证链路、内部鉴权和密钥注入符合生产要求，避免未授权访问和密钥暴露。

#### 验收标准

1. THE Audit Plan SHALL 审计 Gateway、OS、Client 的密钥来源，确认运行所需密钥未硬编码在仓库和镜像中
2. THE Audit Plan SHALL 对照 `deploy/runbooks/secret-injection.md` 验证 JWT、HMAC、command secret、TLS 证书等密钥的注入路径
3. THE Audit Plan SHALL 验证 Gateway 的 command 认证、OS 的内部接口鉴权、WebSocket 鉴权和管理 API 认证真实生效
4. THE Audit Plan SHALL 验证裸请求、伪造头、缺失 token、过期 token、错误签名等绕过场景被拒绝
5. THE Audit Plan SHALL 验证 mTLS、证书签发、轮转和证书目录口径在脚本、compose、配置和代码中一致
6. IF 发现硬编码密钥、认证缺失或可绕过的授权链路, THEN THE Audit Plan SHALL 标记为 P0 风险

### 需求 4：运行时安全与敏感数据审计

**用户故事：** 作为安全负责人，我需要确认系统在日志、内存、存储和自毁机制方面不会泄露敏感信息。

#### 验收标准

1. THE Audit Plan SHALL 审计日志脱敏、日志存储策略和敏感字段过滤，覆盖 Gateway、OS 和 Client
2. THE Audit Plan SHALL 审计内存保护、自毁、擦除、killswitch、burn、memsafe 等运行时安全能力
3. THE Audit Plan SHALL 验证 PostgreSQL、Redis、BoltDB 或等效存储中的敏感字段处理是否符合设计预期
4. THE Audit Plan SHALL 检查 panic、fatal、调试日志和错误回显中是否泄露内部实现或凭证片段
5. THE Audit Plan SHALL 检查发布产物、默认配置和示例文件中是否残留敏感数据
6. IF 敏感数据以明文持久化、日志泄露密钥或擦除机制失效, THEN THE Audit Plan SHALL 标记为 P0 风险

### 需求 5：协议实现、架构边界与运行时一致性审计

**用户故事：** 作为架构负责人，我需要确认协议实现、组件职责和跨组件契约与真相源一致，避免系统“看起来能跑、实际上边界混乱”。

#### 验收标准

1. THE Audit Plan SHALL 对照 `docs/governance/boundaries/` 和 `docs/protocols/` 审计 Gateway、OS、Client 的职责归属
2. THE Audit Plan SHALL 审计 NPM、B-DNA、Jitter-Lite、G-Tunnel、G-Switch、VPC 的代码落点与协议文档一致性
3. THE Audit Plan SHALL 审计 `mirage-proto/*.proto` 与 Go/TS 侧调用是否存在接口漂移
4. THE Audit Plan SHALL 检查 `mirage-gateway/cmd/gateway/main.go` 中 `GatewayConfig` 与部署 manifest、默认配置、runbook 是否一致
5. THE Audit Plan SHALL 检查当前治理文档中标记“待收敛”的运行时主链是否已在代码上收敛或被纳入明确整改项
6. IF 发现协议主权混乱、配置语义漂移或跨组件契约不兼容, THEN THE Audit Plan SHALL 标记为 P1 风险

### 需求 6：代码质量与测试充分性审计

**用户故事：** 作为开发负责人，我需要确认关键代码路径具备足够的静态检查、测试覆盖和高风险行为约束。

#### 验收标准

1. THE Audit Plan SHALL 覆盖 Gateway、OS、Client、CLI 和关键辅助模块的静态质量检查
2. THE Audit Plan SHALL 检查 Go 代码中的错误处理、goroutine 生命周期、data race、panic 使用和资源释放
3. THE Audit Plan SHALL 检查 eBPF/C 数据面代码的 verifier 友好性、边界检查和失败安全回退
4. THE Audit Plan SHALL 检查 NestJS/前端代码的输入校验、异常处理和关键安全编码实践
5. THE Audit Plan SHALL 统计或抽样确认单元测试、集成测试、P0 runtime、chaos/genesis、基准测试对关键路径的覆盖情况
6. WHEN 关键发布路径没有有效测试保护时, THE Audit Plan SHALL 至少标记为 P1 风险

### 需求 7：性能、容量与稳定性审计

**用户故事：** 作为性能与运维负责人，我需要确认系统在核心路径上的性能、恢复和稳定性符合上线最低标准。

#### 验收标准

1. THE Audit Plan SHALL 审计 `benchmarks/`、`tests/p0_runtime/` 和 `deploy/chaos/` 对核心链路的验证能力
2. THE Audit Plan SHALL 定义并验证 Gateway、OS、Client 的最低性能和资源阈值，至少覆盖启动、核心 API、关键转发路径和异常恢复
3. THE Audit Plan SHALL 验证故障恢复、服务重启恢复、节点替换和证书轮转具备演练或脚本级证据
4. THE Audit Plan SHALL 检查是否具备最小可用的健康检查、日志、指标或故障定位入口
5. WHEN 核心恢复链路、压测结果或关键健康检查缺失时, THE Audit Plan SHALL 至少标记为 P1 风险

### 需求 8：部署安全、运维可执行性与回滚审计

**用户故事：** 作为运维负责人，我需要确保部署资产是真实可执行的，而不是文档里“看起来完整”。

#### 验收标准

1. THE Audit Plan SHALL 覆盖 `deploy/certs/`、`deploy/scripts/`、`deploy/runbooks/`、`deploy/docker-compose.*.yml` 和相关组件内 compose/manifests
2. THE Audit Plan SHALL 检查证书生成、轮转、替换、节点失陷处理和紧急擦除脚本是否与 runbook 一致
3. THE Audit Plan SHALL 检查 compose、Dockerfile、镜像构建上下文、挂载目录、tmpfs、端口和环境变量是否一致且可解释
4. THE Audit Plan SHALL 检查数据库迁移、初始化、回滚和服务重启后的恢复路径是否有执行说明或自动化脚本
5. THE Audit Plan SHALL 检查生产部署最小权限模型是否在脚本、文档和服务配置中闭环
6. IF 部署资产不可执行、权限过宽或回滚路径不存在, THEN THE Audit Plan SHALL 标记为 P0 或 P1 风险

### 需求 9：供应链、发布产物与可追溯性审计

**用户故事：** 作为发布与安全负责人，我需要确认依赖、构建产物和发布流程可追溯，避免把无法证明来源的产物推向生产。

#### 验收标准

1. THE Audit Plan SHALL 扫描 Go、Node、SDK 等依赖的已知漏洞和锁文件完整性
2. THE Audit Plan SHALL 检查 Docker 基础镜像、构建脚本、lockfile、release manifest 和签名验证流程
3. THE Audit Plan SHALL 检查仓库中是否误提交可执行产物、临时文件或无法追溯来源的二进制
4. THE Audit Plan SHALL 要求本轮发布结论能够映射到具体 commit、具体验证命令和具体产物
5. THE Audit Plan SHALL 检查许可证、第三方依赖约束和 SDK 交付面是否存在阻断性缺口
6. IF 发现高危依赖漏洞、签名链失效或不可追溯发布产物, THEN THE Audit Plan SHALL 标记为 P0 风险

### 需求 10：文档、Runbook 与操作面完备性审计

**用户故事：** 作为项目负责人，我需要确认文档、runbook 和发布清单真实反映当前实现，这样团队才能按文档完成部署、排障和交接。

#### 验收标准

1. THE Audit Plan SHALL 对照 `docs/`、`deploy/runbooks/`、`DEPLOYMENT.md`、组件内 `DEPLOYMENT.md` 和 `RELEASE_CHECKLIST.md`
2. THE Audit Plan SHALL 检查架构文档、协议文档、API 契约、发布清单和部署步骤与当前代码是否一致
3. THE Audit Plan SHALL 检查是否存在多个相互冲突的说明文件，并按 `source-of-truth-map.md` 给出去留结论
4. THE Audit Plan SHALL 检查一线运维至少具备启动、验证、证书轮转、节点替换、故障恢复和回滚所需说明
5. WHEN 文档与代码存在重大漂移且会影响真实部署或误导上线判断时, THE Audit Plan SHALL 标记为 P1 或 P2 风险

### 需求 11：审计产物与复验闭环

**用户故事：** 作为项目负责人，我需要审计不是“一次性看一眼”，而是形成可复验、可交接、可继续推进的闭环。

#### 验收标准

1. THE Audit Plan SHALL 产出统一的审计总表，至少包含检查项、状态、证据链接、发现编号和修复归属
2. THE Audit Plan SHALL 区分 `not_started`、`in_progress`、`blocked`、`failed`、`passed`、`waived`、`verified` 等状态
3. THE Audit Plan SHALL 为每个 P0/P1 发现安排复验动作和通过标准
4. THE Audit Plan SHALL 要求最终结论包含：剩余风险、豁免项、上线建议、上线后追踪项
5. THE Audit Plan SHALL 在所有 P0/P1 关闭或经书面豁免前维持 `release_blocked`

### 需求 12：最终上线判定

**用户故事：** 作为决策者，我需要一个清晰且可解释的“是否可上线”判定标准，而不是泛泛的主观评价。

#### 验收标准

1. THE Audit Plan SHALL 只有在所有 P0/P1 关闭、关键构建与启动链路通过、关键认证链路通过、关键部署与恢复链路有证据时，才可标记为 `release_ready`
2. THE Audit Plan SHALL 在结论中明确列出通过项、阻断项、残余风险和后续整改承诺
3. THE Audit Plan SHALL 将当前发布周期结论回写或引用到 `docs/release-readiness-traceability-index.md`
4. IF 任一关键组件仅有“文档声明通过”而无可验证证据, THEN THE Audit Plan SHALL 视为未通过
