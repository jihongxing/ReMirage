# 实施计划：Phase 3 可运营基线固化

## 概述

按 M8→M9→证据 三个里程碑递进实施。本 spec 不新增功能，只安排部署等级定义、联合演练、证据沉淀和 Runbook 对齐。

关键约束：
- 所有 PBT 子任务为必须项（Phase 1/2 经验教训）
- PBT 使用 `pgregory.net/rapid`，最少 100 次迭代
- PBT 复用现有测试文件，不创建新测试文件
- M8 基线检查需 Linux + 容器环境完整验证；非 Linux 降级为子集检查并标注
- M9 联合演练以现有 Go 测试为骨架，不依赖真实 eBPF 环境
- 证据强度："代码级验证 + 受控环境基线检查"
- 证据产物按固定路径沉淀
- Drill 脚本包含完整步骤（不只是 `go test` 包装）
- 术语与 requirements.md / design.md / capability-truth-source.md 保持一致

## 任务

- [x] 1. M8：部署等级定义与基线清单
  - [x] 1.1 创建 `docs/reports/deployment-tiers.md`
    - 定义三个部署等级：默认部署（Default）、加固部署（Hardened）、极限隐匿部署（Extreme Stealth）
    - 每个等级包含：适用场景、安全边界、配置要求
    - 每个配置项标注当前代码支持状态：已支持 / 需配置 / 不支持，引用具体文件路径作为证据
    - 配置项覆盖：RAM_Shield（mlock + core dump 禁用）、证书存储（磁盘 vs tmpfs）、Swap 状态、日志持久化、Emergency_Wipe 可用性、Cert_Rotate、只读根文件系统、密钥注入方式、持久化卷
    - 明确标注 `mirage-gateway/docker-compose.tmpfs.yml` 对应加固部署等级，不自动等于极限隐匿
    - 不支持的配置项标注为"当前不支持"并说明差距（如 Emergency_Wipe 自动触发、Vault + tmpfs Volume Mount）
    - 证书 ≤24h 有效期列为"候选强化项"（当前 cert-rotate.sh --days-before 是轮转预警阈值，非签发有效期；gen_gateway_cert.sh 签发 3 天；需新增签发策略），不作为当前 tier 既有配置要求
    - 引用 Runbook 交叉引用：Secret_Injection_Runbook（每等级推荐注入路径）、Least_Privilege_Runbook（权限矩阵验证）、Node_Replacement_Runbook（每等级替换注意事项）
    - 为加固部署和极限隐匿部署各提供配置示例片段，引用 Tmpfs_Compose 和 Cert_Rotate 实际参数
    - 标注 Runbook 不一致时以哪份文档为准
    - _需求: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 3.1, 3.2, 3.3, 3.4, 3.5_

  - [x] 1.2 创建 `docs/reports/deployment-baseline-checklist.md`
    - 为每个部署等级列出可执行检查项
    - 每项包含：检查项名称、检查命令或验证方法、预期结果、对应部署等级、自动化程度（可脚本化 / 需人工验证 / 需环境依赖）
    - RAM_Shield 检查项：mlock 生效（`/proc/<pid>/status` VmLck 非零）、core dump 禁用（`ulimit -c` 为 0）、swap 使用量为零（`/proc/meminfo` SwapTotal/SwapFree）
    - 证书检查项：存储路径在 tmpfs（`mount | grep tmpfs | grep certs`）、有效期符合等级要求（`openssl x509 -enddate`）、CA 私钥不在 Gateway 节点
    - 文件系统检查项：根文件系统只读（`mount | grep "ro,"`）、无非 tmpfs 可写挂载点、swap 分区禁用（`swapon --show` 为空）
    - Emergency_Wipe 检查项：脚本存在且可执行、依赖工具可用（`shred`、`bpftool`）、dry-run 等效验证
    - _需求: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6_


  - [x] 1.3 创建 `deploy/scripts/drill-m8-baseline.sh`
    - M8 基线验证演练脚本
    - 输入：部署等级参数（default|hardened|extreme）
    - 步骤：① 检查运行环境（Linux/容器/root 权限）② 按目标等级执行 Baseline_Checklist 中对应检查项 ③ RAM_Shield 状态检查（mlock、core dump、swap）④ 证书配置检查（tmpfs、有效期、CA 私钥位置）⑤ 文件系统检查（只读根、可写挂载点、swap 分区）⑥ Emergency_Wipe 可用性检查（脚本存在、依赖工具、dry-run）⑦ 汇总结果并生成报告
    - 非 Linux 环境：跳过 `/proc` 相关检查，标注"需 Linux 环境验证"
    - 容器外执行：跳过 tmpfs/只读根检查，标注"需容器环境验证"
    - 权限不足：提示需要 root/sudo，降级为可执行的检查子集
    - 捕获日志到 `deploy/evidence/m8-baseline-drill.log`
    - 脚本退出码反映检查结果
    - _需求: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6_

- [x] 2. Checkpoint — M8 基线冻结确认
  - 确认部署等级文档、基线检查清单和 M8 演练脚本已创建，请用户确认是否有问题。

- [x] 3. M9：准入控制 PBT 与 Critical Tests
  - [x] 3.1 扩展 `mirage-gateway/pkg/api/quota_bucket_test.go` — Property 1: QuotaBucket 用户隔离 PBT
    - **Property 1: QuotaBucket 用户隔离**
    - 测试函数: `TestProperty_QuotaBucketIsolation`
    - 使用 `rapid` 生成随机 (quotaA ∈ [1,100000], quotaB ∈ [1,100000], consumeAmounts []uint64)
    - 创建两个用户，耗尽用户 A 的配额后验证：
      - 用户 B 的 `RemainingBytes` 不减少（除 B 自身消耗外）
      - 用户 B 的 `Exhausted` 标志保持为 0
      - 用户 B 的 `Consume()` 在自身配额范围内继续成功
    - 最少 100 次迭代
    - **验证: 需求 5.3, 6.1, 6.3**

  - [x] 3.2 扩展 `mirage-gateway/pkg/api/fuse_callback_test.go` — Property 2: FuseCallback 精确定向 PBT
    - **Property 2: FuseCallback 精确定向**
    - 测试函数: `TestProperty_FuseCallbackTargeting`
    - 使用 `rapid` 生成随机 (userCount ∈ [2,10], exhaustedUserIndex ∈ [0,userCount-1], quotas []uint64)
    - 注册多个用户，耗尽指定用户配额后验证：
      - `onQuotaExhausted` 回调仅以该用户 UID 触发
      - 其他未耗尽用户不触发回调
    - 最少 100 次迭代
    - **验证: 需求 6.2**

  - [x] 3.3 扩展 `mirage-gateway/pkg/redact/redact_test.go` — Property 3: IP 脱敏完整性 PBT
    - **Property 3: IP 脱敏完整性**
    - 测试函数: `TestProperty_IPRedactionCompleteness`
    - 使用 `rapid` 生成包含随机数量 IPv4 地址的随机文本（IP 数量 ∈ [0,10]，每个 IP 各段 ∈ [0,255]）
    - 调用 `RedactIPInText` 后验证：
      - 所有 IPv4 地址最后一段被替换为 `***`
      - 输出中不包含任何原始 IPv4 地址的完整最后一段
      - 非 IPv4 文本内容保持不变
    - 最少 100 次迭代
    - **验证: 需求 5.2, 6.4**

  - [x] 3.4 扩展 `mirage-gateway/pkg/api/integration_test.go` — Property 4: AddQuota 重新激活 PBT
    - **Property 4: AddQuota 重新激活**
    - 测试函数: `TestProperty_AddQuotaReactivation`
    - 使用 `rapid` 生成随机 (initialQuota ∈ [1,10000], additionalQuota ∈ [1,10000])
    - 先耗尽配额（Exhausted=1），再调用 `UpdateQuota(uid, additionalQuota)` 后验证：
      - `Exhausted` 标志重置为 0
      - `RemainingBytes` 等于 additionalQuota
      - 可继续消费（在新配额范围内）
    - 最少 100 次迭代
    - **验证: 需求 6.5**

  - [x] 3.5 扩展 `mirage-gateway/pkg/api/security_regression_test.go` — Property 5: HMAC 校验确定性 PBT
    - **Property 5: HMAC 校验确定性**
    - 测试函数: `TestProperty_HMACDeterminism`
    - 使用 `rapid` 生成随机 (commandType, timestamp, nonce, payloadHash, secret) 字符串组合
    - 对同一输入调用两次 `Verify()` 验证结果一致
    - 修改任一字段（commandType / timestamp / nonce / payloadHash）后验证签名不匹配
    - 最少 100 次迭代
    - **验证: 需求 5.1**

  - [x] 3.6 扩展 `mirage-gateway/pkg/api/integration_test.go` — 3 个 Critical Tests
    - 新增 `TestCritical_IllegalRequestNoQuotaImpact`：发送非法请求（无 HMAC）被拒绝后，验证合法用户配额 `RemainingBytes` 不变。所属部署等级：All
    - 新增 `TestCritical_FuseLogRedaction`：用户配额耗尽触发熔断后，验证熔断事件日志中 IP 已脱敏（`x.x.x.***` 格式）、不包含明文密钥或 token。所属部署等级：All
    - 新增 `TestCritical_QuotaReactivationE2E`：用户配额耗尽后调用 `AddQuota` 追加配额，验证 `Exhausted` 恢复为 0 且可继续消费。所属部署等级：All
    - 这些测试不依赖真实 eBPF 环境，可在 CI 中执行
    - _需求: 5.3, 6.4, 6.5, 7.3, 7.4_

- [x] 4. Checkpoint — M9 PBT 与 Critical Tests 确认
  - 确认 5 个 PBT 和 3 个 Critical Tests 已编写并通过，请用户确认是否有问题。

- [x] 5. M9：联合演练记录与演练脚本
  - [x] 5.1 创建 `docs/reports/access-control-joint-drill.md`
    - 场景 1（非法接入）完整记录：
      - 步骤 1a-1d：无 HMAC 签名 → 拒绝、过期时间戳 → 拒绝、重放 nonce → 拒绝、无 JWT WebSocket → 401
      - 日志脱敏验证：IP → `x.x.x.***`、token → `***`、secret → `[REDACTED]`
      - 配额不受损验证：合法用户 `RemainingBytes` 前后不变
      - `pkg/api` 包 HMAC 回归测试全部通过作为回归基线
    - 场景 2（配额耗尽）完整记录：
      - 步骤 4a-4d：GrantAccess(userA, userB) → userA 持续消耗至耗尽 → userA 被熔断（Exhausted=1）→ userB 不受影响
      - `onQuotaExhausted` 回调验证：参数为 userA UID
      - userB 状态不变验证：`RemainingBytes` 未减少、`Exhausted`=0
      - 熔断日志脱敏验证：IP 脱敏、无明文密钥/token
      - AddQuota 重新激活验证：userA 追加配额后 `Exhausted` 恢复为 0
    - 每个步骤记录实际输出（命令输出、日志片段、配额状态查询结果）
    - 异常现象完整记录和根因分析（如有）
    - Smoke test 入口汇总：引用现有测试作为基础（`pkg/api` 包 HMAC 回归、`services/ws-gateway` 包 JWT 鉴权、`pkg/api` 包配额隔离 `-count=10` 连续通过、`pkg/redact` 包 Gateway 侧 + OS 侧脱敏全部通过）
    - Critical test 入口列表：3 个新增 critical test 的测试名称、执行命令、预期结果、所属部署等级（All）、环境依赖
    - Redis 鉴权连通性 smoke 入口：复用功能验证清单中"生产配置鉴权闭环"口径（`Select-String -Path deploy/docker-compose.os.yml -Pattern 'requirepass|MIRAGE_REDIS_PASSWORD|redis://:'`）
    - _需求: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 7.1, 7.2, 7.3, 7.4_

  - [x] 5.2 创建 `deploy/scripts/drill-m9-joint-drill.sh`
    - M9 联合演练执行脚本
    - 步骤：
      - ① 检查环境（Go 编译器、mirage-gateway 和 mirage-os 目录存在）
      - ② 场景 1 — 非法接入回归：运行 `pkg/api` 包 HMAC 回归测试、`services/ws-gateway` 包 JWT 鉴权测试、`pkg/redact` 包 Gateway 侧 + OS 侧脱敏测试、`pkg/api` 包配额隔离测试（`-count=10`）
      - ③ 场景 2 — 配额耗尽回归：运行 `pkg/api` 包熔断回调测试、`pkg/api` 包集成测试（含现有 + 3 个 critical tests）、`pkg/api` 包 AddQuota 重置测试
      - ④ PBT 执行：运行 5 个 Property Tests（Property 1-5）
      - ⑤ Redis 鉴权连通性验证：检查 `deploy/docker-compose.os.yml` 中 requirepass / 连接串 / healthcheck 配置一致性（复用功能验证清单"生产配置鉴权闭环"口径）
      - ⑥ 汇总结果：统计通过/失败数量，生成联合演练摘要
    - 捕获日志到 `deploy/evidence/m9-joint-drill.log`
    - 脚本退出码反映测试结果
    - _需求: 5.4, 6.6, 7.1, 7.2_

- [x] 6. Checkpoint — M9 联合演练确认
  - 确认联合演练记录和演练脚本已完成，请用户确认是否有问题。

- [x] 7. 证据沉淀：验证清单与治理回写
  - [x] 7.1 回写 `docs/Mirage 功能确认与功能验证任务清单.md`
    - 在功能验证任务表中新增"部署等级与基线清单冻结"条目：
      - 复验命令指向 `bash deploy/scripts/drill-m8-baseline.sh`
      - 通过标准：部署等级文档和基线检查清单已完成且检查项可执行
      - 证据文件路径：`docs/reports/deployment-tiers.md`、`docs/reports/deployment-baseline-checklist.md`
      - 执行负责人：`Security / Platform`
    - 新增"准入控制联合演练"条目：
      - 复验命令指向 `bash deploy/scripts/drill-m9-joint-drill.sh`
      - 通过标准：两个演练场景关键验证点全部通过、5 个 PBT 各 100 次通过、3 个 Critical Tests 通过、Redis 鉴权配置一致
      - 证据文件路径：`docs/reports/access-control-joint-drill.md`、`deploy/evidence/m9-joint-drill.log`
      - Smoke test 入口列表：HMAC 回归 / JWT 回归 / 脱敏回归 / 配额隔离 / Redis 鉴权连通性
      - Critical test 入口列表：3 个跨组件测试（含所属部署等级 All）
      - 执行负责人：`Gateway Runtime` + `Security / Platform`
    - _需求: 4.1, 7.5, 8.1_

  - [x] 7.2 回写 `docs/governance/capability-truth-source.md`（反取证能力域）
    - 回写"反取证与最小运行痕迹"能力域的"当前真实能力"描述
    - 增加部署等级定义和基线清单的证据锚点链接：`docs/reports/deployment-tiers.md`、`docs/reports/deployment-baseline-checklist.md`
    - 更新表述边界：允许"支持默认/加固/极限隐匿三种部署等级，加固部署已有完整配置锚点"；不允许"所有部署都是无盘化运行"
    - 若极限隐匿部署某些配置项不支持 → 维持"已实现（限定表述）"状态，记录差距说明
    - _需求: 4.2, 4.3, 4.4_

  - [x] 7.3 回写 `docs/governance/capability-truth-source.md`（准入控制能力域）
    - 回写"准入控制与防滥用"能力域的"当前真实能力"描述
    - 增加联合演练记录和 smoke/critical test 入口的证据锚点链接：`docs/reports/access-control-joint-drill.md`
    - 评估是否满足维持"已实现"状态的条件，增加联合演练证据
    - 若联合演练发现跨组件协同缺陷（如熔断后日志未脱敏、配额隔离串扰）→ 降级为"已实现（限定表述）"并记录具体缺陷
    - 确保两个能力域表述与 Deployment_Tier_Document 和 Joint_Drill_Record 实际结论一致
    - _需求: 8.1, 8.2, 8.3, 8.4_

- [x] 8. 最终 Checkpoint — Phase 3 出关确认
  - 确认所有里程碑完成、治理回写一致、证据沉淀完整，请用户确认是否有问题。

## 备注

- 所有 PBT 子任务为必须项（非可选），Phase 1/2 经验教训
- PBT 使用 `pgregory.net/rapid`（Go），最少 100 次迭代
- PBT 复用现有测试文件，不创建新测试文件：
  - `mirage-gateway/pkg/api/quota_bucket_test.go` — Property 1
  - `mirage-gateway/pkg/api/fuse_callback_test.go` — Property 2
  - `mirage-gateway/pkg/redact/redact_test.go` — Property 3
  - `mirage-gateway/pkg/api/integration_test.go` — Property 4 + 3 Critical Tests
  - `mirage-gateway/pkg/api/security_regression_test.go` — Property 5
- 每个 PBT 任务标注对应 design property 编号和验证的需求编号
- Critical Tests 添加到 `integration_test.go`，不创建新文件
- M8 基线检查需 Linux + 容器环境完整验证；非 Linux 降级为子集检查并标注
- M9 联合演练以现有 Go 测试为骨架，不依赖真实 eBPF 环境
- 证据强度："代码级验证 + 受控环境基线检查"
- 证据产物固定路径：`docs/reports/`（文档）、`deploy/scripts/`（演练脚本）、`deploy/evidence/`（drill 日志）
- Drill 脚本包含完整步骤：环境检查 → 测试执行 → 证据捕获 → 报告生成
- 术语一致性：Deployment_Tier_Document / Baseline_Checklist / Joint_Drill_Record / Capability_Truth_Source / Verification_Checklist 在所有引用中保持一致
