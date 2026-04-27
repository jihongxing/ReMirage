# 需求文档：Phase 3 可运营基线固化

## 简介

本需求文档覆盖北极星实施计划 Phase 3（可运营基线固化），目标是将两个能力域从"强组件"推进为"可运营基线"——证明反取证与准入控制能力可以在真实运营场景下协同工作，而不仅仅是孤立组件。

Phase 3 包含两个里程碑（M8-M9），覆盖两个能力域：
- 反取证与最小运行痕迹（M8）
- 准入控制与防滥用（M9）

本 spec 严格遵守 `capability-truth-source.md` 的治理规则：只安排部署等级定义、联合演练、证据沉淀和 runbook 对齐，不扩展产品边界，不新增功能。

关键原则：
1. 本 spec 是关于基线固化，不是新功能开发
2. 只安排：部署等级定义、联合演练、证据积累、runbook 对齐
3. 不扩展产品边界
4. 所有演练结果必须诚实反映当前能力
5. 必须区分"组件存在"与"可运营基线已验证"
6. Phase 1/2 经验教训：只描述当前代码实际行为，包含具体证据产物和固定路径，跨文档术语对齐，诚实标注证据强度

## 术语表

- **Deployment_Tier_Document**: 部署等级说明文档，定义默认/加固/极限隐匿三种部署等级的边界与配置要求，产出路径 `docs/reports/deployment-tiers.md`
- **Baseline_Checklist**: 基线检查清单，为每个部署等级列出必须满足的检查项，产出路径 `docs/reports/deployment-baseline-checklist.md`
- **Joint_Drill_Record**: 准入控制联合演练记录，包含演练步骤、实际结果和证据截图/日志，产出路径 `docs/reports/access-control-joint-drill.md`
- **Capability_Truth_Source**: 能力真相源文档 `docs/governance/capability-truth-source.md`，定义验收标准与状态等级
- **Verification_Checklist**: 功能验证清单 `docs/Mirage 功能确认与功能验证任务清单.md`，当前发布周期验证材料
- **RAM_Shield**: 内存保护模块 `mirage-gateway/pkg/security/ram_shield.go`，提供 mlock 内存锁定、core dump 禁用、swap 检测
- **Emergency_Wipe**: 紧急擦除脚本 `deploy/scripts/emergency-wipe.sh`，焦土协议实现（3-pass 随机覆写 + 零化、eBPF 卸载、证书/密钥安全擦除）
- **Cert_Rotate**: 证书轮换脚本 `deploy/scripts/cert-rotate.sh`，支持本地 CA 签发和 OS API 签发两条路径，证书默认 3 天有效期
- **Tmpfs_Compose**: tmpfs 部署配置 `mirage-gateway/docker-compose.tmpfs.yml`，内存文件系统部署（只读根、swap 禁用、证书仅存内存）
- **Command_Auth**: HMAC 签名校验器 `mirage-gateway/pkg/api/command_auth.go`，覆盖 commandType + timestamp + nonce + SHA256(payload)，±60s 时间窗口
- **JWT_Auth**: WebSocket JWT 鉴权中间件 `mirage-os/services/ws-gateway/auth_test.go`，token 校验与 /health 旁路
- **Burn_Engine**: 实时烧录引擎 `mirage-gateway/pkg/ebpf/burn_engine.go`，eBPF 流量计费、per-user 配额隔离、配额耗尽自动熔断
- **Log_Redactor**: 日志脱敏模块 `mirage-gateway/pkg/redact/redact.go`，IP/Token/Secret 脱敏（IP → x.x.x.***，Token → ***，Secret → [REDACTED]）
- **Redis_Auth**: Redis 鉴权配置 `deploy/docker-compose.os.yml`，requirepass 启用 + 连接串带密码 + healthcheck 鉴权探活
- **Secret_Injection_Runbook**: 密钥注入 Runbook `deploy/runbooks/secret-injection.md`，K8s Secret / Vault + Compose 过渡方案
- **Least_Privilege_Runbook**: 最小权限模型 `deploy/runbooks/least-privilege-model.md`，按角色划分资源访问权限
- **Node_Replacement_Runbook**: 节点失陷替换 Runbook `deploy/runbooks/compromised-node-replacement.md`，< 30 分钟替换流程
- **Security_Regression_Tests**: 安全回归测试 `mirage-gateway/pkg/api/security_regression_test.go`，覆盖 HMAC、时间戳、nonce、高危命令、重放检测

## 需求

### 需求 1：部署等级定义（M8 — 等级划分）

**用户故事：** 作为运维人员，我希望有明确的部署等级定义，以便根据不同场景选择合适的部署配置，清楚知道每个等级的安全边界。

#### 验收标准

1. THE Deployment_Tier_Document SHALL 定义三个部署等级：默认部署（Default）、加固部署（Hardened）、极限隐匿部署（Extreme Stealth），每个等级包含适用场景、安全边界和配置要求
2. WHEN 定义默认部署等级时，THE Deployment_Tier_Document SHALL 标注以下配置项的状态：RAM_Shield 启用状态（mlock + core dump 禁用）、证书存储位置（磁盘 vs tmpfs）、swap 状态（允许 vs 禁用）、日志持久化（允许）、Emergency_Wipe 可用性（可选）
3. WHEN 定义加固部署等级时，THE Deployment_Tier_Document SHALL 标注以下配置项的状态：RAM_Shield 强制启用、证书存储在 tmpfs、swap 禁用（`mem_swappiness: 0`）、Cert_Rotate 启用（证书有效期 ≤ 72h）、只读根文件系统、Emergency_Wipe 预装
4. WHEN 定义极限隐匿部署等级时，THE Deployment_Tier_Document SHALL 标注以下配置项的状态：加固部署的全部要求 + 日志不持久化（`max-file: 1` + 内存日志）+ 无持久化卷挂载 + Emergency_Wipe 自动触发条件预配置（当前不支持，需新增代码）。证书有效期 ≤24h 列为"候选强化项"（当前 cert-rotate.sh 的 `--days-before` 是轮转预警阈值，不是签发有效期；gen_gateway_cert.sh 签发有效期为 3 天；若需 ≤24h 签发策略需新增签发参数），不作为当前 tier 的既有配置要求
5. THE Deployment_Tier_Document SHALL 为每个等级标注当前代码和配置的支持状态：已支持（有代码/配置锚点）、需配置（代码存在但需手动配置）、不支持（需新增代码），并引用具体文件路径作为证据
6. THE Deployment_Tier_Document SHALL 明确标注：当前 Tmpfs_Compose 配置（`mirage-gateway/docker-compose.tmpfs.yml`）对应加固部署等级，不自动等于极限隐匿部署
7. IF 某个等级的某项配置当前不支持，THEN THE Deployment_Tier_Document SHALL 标注为"当前不支持"并说明差距，不将未实现的配置写成已支持

### 需求 2：基线检查清单（M8 — 检查项）

**用户故事：** 作为运维人员，我希望有一份可执行的基线检查清单，以便在部署时逐项验证当前部署是否满足目标等级的要求。

#### 验收标准

1. THE Baseline_Checklist SHALL 为每个部署等级列出可执行的检查项，每项包含：检查项名称、检查命令或验证方法、预期结果、对应部署等级（Default/Hardened/Extreme Stealth）
2. WHEN 检查 RAM_Shield 状态时，THE Baseline_Checklist SHALL 包含以下检查项：(a) mlock 是否生效（验证 `/proc/<pid>/status` 中 `VmLck` 字段非零）、(b) core dump 是否禁用（验证 `ulimit -c` 为 0 或 `/proc/sys/kernel/core_pattern` 为空）、(c) swap 使用量是否为零（验证 `/proc/meminfo` 中 `SwapTotal` 或 `SwapFree`）
3. WHEN 检查证书配置时，THE Baseline_Checklist SHALL 包含以下检查项：(a) 证书存储路径是否在 tmpfs 上（`mount | grep tmpfs | grep certs`）、(b) 证书有效期是否符合等级要求（`openssl x509 -enddate -noout -in <cert>`）、(c) CA 私钥是否不在 Gateway 节点上
4. WHEN 检查文件系统时，THE Baseline_Checklist SHALL 包含以下检查项：(a) 根文件系统是否只读（`mount | grep "ro,"`）、(b) 是否存在非 tmpfs 的可写挂载点、(c) swap 分区是否禁用（`swapon --show` 为空）
5. WHEN 检查 Emergency_Wipe 可用性时，THE Baseline_Checklist SHALL 包含以下检查项：(a) 脚本是否存在且可执行、(b) 脚本依赖工具是否可用（`shred`、`bpftool`）、(c) 擦除验证步骤是否可执行（`--check-only` 模式的 dry-run 等效）
6. THE Baseline_Checklist SHALL 标注每个检查项的自动化程度：可脚本化（提供命令）、需人工验证（说明验证步骤）、需环境依赖（标注依赖条件）

### 需求 3：部署等级与 Runbook 对齐（M8 — 配置一致性）

**用户故事：** 作为运维人员，我希望部署等级定义与现有 Runbook 保持一致，以便在实际操作中不出现配置冲突或遗漏。

#### 验收标准

1. THE Deployment_Tier_Document SHALL 引用 Secret_Injection_Runbook 中的密钥注入方式，并标注每个部署等级的推荐注入路径：默认部署（Compose 环境变量）、加固部署（Docker Secrets 或 Vault）、极限隐匿部署（Vault + tmpfs Volume Mount）
2. THE Deployment_Tier_Document SHALL 引用 Least_Privilege_Runbook 中的权限矩阵，并验证每个部署等级的权限配置不超出矩阵定义
3. THE Deployment_Tier_Document SHALL 引用 Node_Replacement_Runbook 中的替换流程，并标注每个部署等级下节点替换的特殊注意事项：默认部署（标准流程）、加固部署（证书 72h 自然过期，与 compromised-node-replacement.md 一致）、极限隐匿部署（证书 72h 过期 + Emergency_Wipe 优先执行；≤24h 证书有效期为候选强化项，当前签发脚本不支持）
4. WHEN Deployment_Tier_Document 中的配置要求与现有 Runbook 存在不一致时，THE Deployment_Tier_Document SHALL 明确标注差异并说明以哪份文档为准
5. THE Deployment_Tier_Document SHALL 为加固部署和极限隐匿部署各提供一份配置示例片段，引用 Tmpfs_Compose 和 Cert_Rotate 的实际配置参数

### 需求 4：M8 证据沉淀与基线冻结

**用户故事：** 作为项目治理者，我希望 M8 的产出按治理规则沉淀，以便后续阶段和审计可追溯。

#### 验收标准

1. WHEN M8 完成后，THE Verification_Checklist SHALL 新增"部署等级与基线清单冻结"条目，包含 Deployment_Tier_Document 和 Baseline_Checklist 的文件路径、执行负责人和通过标准
2. WHEN M8 完成后，THE Capability_Truth_Source SHALL 回写"反取证与最小运行痕迹"能力域的"当前真实能力"描述，增加部署等级定义和基线清单的证据锚点链接
3. IF Deployment_Tier_Document 显示极限隐匿部署的某些配置项当前不支持，THEN THE Capability_Truth_Source SHALL 维持"已实现（限定表述）"状态，并在主证据锚点中记录差距说明
4. THE Capability_Truth_Source 中"反取证与最小运行痕迹"的表述边界 SHALL 更新为：允许"支持默认/加固/极限隐匿三种部署等级，加固部署已有完整配置锚点"；不允许"所有部署都是无盘化运行"（除非 Baseline_Checklist 证明默认部署也满足无盘化要求）

### 需求 5：非法接入联合演练（M9 — 演练场景 1）

**用户故事：** 作为安全验证者，我希望通过联合演练证明"非法接入 → 拒绝 → 日志脱敏 → 配额不受损"这条链路在真实场景下可以协同工作。

#### 验收标准

1. THE Joint_Drill_Record SHALL 包含以下演练步骤的完整记录：(a) 发送无 HMAC 签名的请求 → Command_Auth 拒绝、(b) 发送过期时间戳的请求 → Command_Auth 拒绝、(c) 发送重放 nonce 的请求 → Command_Auth 拒绝、(d) 发送无 JWT token 的 WebSocket 连接 → JWT_Auth 拒绝
2. WHEN 非法请求被拒绝后，THE Joint_Drill_Record SHALL 验证 Log_Redactor 对拒绝日志的脱敏效果：请求来源 IP 被脱敏为 `x.x.x.***` 格式、token 值被替换为 `***`、密钥信息被替换为 `[REDACTED]`
3. WHEN 非法请求被拒绝后，THE Joint_Drill_Record SHALL 验证合法用户的 Burn_Engine 配额未受影响：合法用户的 `RemainingBytes` 在非法请求前后保持不变、合法用户的 `Exhausted` 标志保持为 0
4. THE Joint_Drill_Record SHALL 验证 Security_Regression_Tests（`pkg/api` 包 HMAC 回归测试）在演练环境下全部通过，作为演练的回归基线
5. THE Joint_Drill_Record SHALL 记录每个演练步骤的实际输出（命令输出、日志片段、配额状态查询结果），不仅记录"通过/失败"
6. IF 演练中发现任何环节未按预期工作（如日志未脱敏、配额被错误扣减），THEN THE Joint_Drill_Record SHALL 完整记录异常现象和根因分析，不隐瞒结果

### 需求 6：配额耗尽联合演练（M9 — 演练场景 2）

**用户故事：** 作为安全验证者，我希望通过联合演练证明"合法接入 → 配额耗尽 → 目标用户熔断"这条链路在真实场景下可以协同工作，且熔断仅影响目标用户。

#### 验收标准

1. THE Joint_Drill_Record SHALL 包含以下演练步骤的完整记录：(a) 为用户 A 和用户 B 分别授权配额（`Burn_Engine.GrantAccess`）、(b) 用户 A 持续消耗流量直至配额耗尽、(c) 验证用户 A 被熔断（`Exhausted` 标志为 1，从 eBPF 白名单移除）、(d) 验证用户 B 配额和访问不受影响
2. WHEN 用户 A 配额耗尽时，THE Joint_Drill_Record SHALL 验证 Burn_Engine 的 `onQuotaExhausted` 回调被触发，且回调参数为用户 A 的 UID
3. WHEN 用户 A 被熔断后，THE Joint_Drill_Record SHALL 验证用户 B 的以下状态不变：`RemainingBytes` 未减少（除用户 B 自身消耗外）、`Exhausted` 标志为 0、eBPF 白名单中用户 B 的条目仍为 allowed
4. THE Joint_Drill_Record SHALL 验证熔断后的日志输出经过 Log_Redactor 脱敏：用户 A 的 IP 地址被脱敏、配额耗尽事件日志不包含明文密钥或 token
5. THE Joint_Drill_Record SHALL 验证 `AddQuota` 可以为已熔断用户追加配额并重新激活：用户 A 追加配额后 `Exhausted` 标志恢复为 0、eBPF 白名单重新授权
6. THE Joint_Drill_Record SHALL 记录每个演练步骤的实际输出（Burn_Engine 状态查询、eBPF Map 查询、日志片段），不仅记录"通过/失败"

### 需求 7：长期验证入口固化（M9 — 持续验证）

**用户故事：** 作为项目治理者，我希望联合演练的关键验证点被固化为可重复执行的测试入口，以便后续发布周期可以持续回归。

#### 验收标准

1. THE Joint_Drill_Record SHALL 从演练场景中提取可自动化的验证点，形成 smoke test 入口，覆盖以下关键链路：(a) 非法请求拒绝 + 日志脱敏验证、(b) 配额隔离 + 熔断验证、(c) Redis 鉴权连通性验证（复用功能验证清单中"生产配置鉴权闭环"的 Redis requirepass / 连接串 / healthcheck 校验口径）
2. WHEN 提取 smoke test 入口时，THE Joint_Drill_Record SHALL 引用现有测试作为基础：Security_Regression_Tests（`pkg/api` 包 HMAC 回归测试）、JWT_Auth 测试（`services/ws-gateway` 包鉴权测试）、配额隔离测试（`pkg/api` 包 `-count=10` 连续通过无串扰）、日志脱敏测试（`pkg/redact` 包 Gateway 侧 + OS 侧全部通过）
3. THE Joint_Drill_Record SHALL 为联合演练中发现的跨组件验证点（非单组件测试可覆盖的）定义新的 critical test 入口，包含：测试名称、执行命令、预期结果、所属部署等级（Default/Hardened/Extreme Stealth 或 All）、环境依赖
4. WHEN 定义新的 critical test 入口时，THE Joint_Drill_Record SHALL 确保测试可在 CI 环境中执行（不依赖真实 eBPF 环境时使用 mock），并标注环境依赖
5. THE Verification_Checklist SHALL 新增"准入控制联合演练"条目，包含 Joint_Drill_Record 的文件路径、smoke test 入口列表（含 Redis 鉴权）和 critical test 入口列表（含所属部署等级）

### 需求 8：M9 证据沉淀与治理回写

**用户故事：** 作为项目治理者，我希望 M9 的产出按治理规则沉淀，并完成准入控制能力域的证据闭环。

#### 验收标准

1. WHEN M9 完成后，THE Capability_Truth_Source SHALL 回写"准入控制与防滥用"能力域的"当前真实能力"描述，增加联合演练记录和 smoke/critical test 入口的证据锚点链接
2. WHEN 联合演练全部通过后，THE Capability_Truth_Source SHALL 评估"准入控制与防滥用"能力域是否满足维持"已实现"状态的条件，并在主证据锚点中增加联合演练证据
3. IF 联合演练发现跨组件协同存在缺陷（如熔断后日志未脱敏、配额隔离存在串扰），THEN THE Capability_Truth_Source SHALL 将该能力域降级为"已实现（限定表述）"并记录具体缺陷
4. WHEN Phase 3 全部里程碑完成后，THE Capability_Truth_Source 中"反取证与最小运行痕迹"和"准入控制与防滥用"两个能力域的表述 SHALL 与 Deployment_Tier_Document 和 Joint_Drill_Record 的实际结论一致，不出现超出证据边界的表述

### 需求 9：Phase 3 出关判定

**用户故事：** 作为项目治理者，我希望 Phase 3 有明确的出关标准，以便判定是否可以进入 Phase 4 北极星升级判定。

#### 验收标准

1. WHEN Phase 3 出关判定时，THE Deployment_Tier_Document 和 Baseline_Checklist SHALL 已完成且经过至少一次人工审查
2. WHEN Phase 3 出关判定时，THE Joint_Drill_Record SHALL 包含两个演练场景（非法接入 + 配额耗尽）的完整记录，且关键验证点全部通过
3. WHEN Phase 3 出关判定时，THE Verification_Checklist SHALL 已新增 M8 和 M9 对应的条目，且所有条目的执行结果已填写
4. WHEN Phase 3 出关判定时，THE Capability_Truth_Source 中"反取证与最小运行痕迹"和"准入控制与防滥用"两个能力域的证据锚点 SHALL 已更新
5. IF Phase 3 出关判定时任一里程碑未完成或关键验证点未通过，THEN Phase 3 SHALL 标记为未出关，不得进入 Phase 4
6. WHEN Phase 3 出关后，部署基线与联合演练 SHALL 可进入长期运维验证流程，关键 runbook、验证清单、配置示例已完成对齐
