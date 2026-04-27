# 实施计划：Phase 4 北极星升级判定

## 概述

按 M10→M11→M12→Exit 四个里程碑递进实施。本 spec 不新增能力域，只负责将 Phase 1-3 产出的局部证据收敛为正式能力升级判定。

关键约束：
- Phase 4 不新增功能，不扩展产品边界
- 状态升级只能基于 Phase 1-3 实际产出的证据，不能基于叙事强度
- 证据不足 → 维持当前状态，记录差距，不得提前升级
- 派生材料不得超出 `capability-truth-source.md` 的边界
- PBT 仅 1 个（Property 1: VerifyEvidenceCompleteness 正确性），为必须项
- PBT 使用 `pgregory.net/rapid`，最少 100 次迭代
- 诚实标注证据强度，包级测试引用（不写具体测试数量）
- 固定产物路径：`docs/reports/`（报告）、`deploy/scripts/`（drill 脚本）、`deploy/release/`（Go 代码）

## 任务

- [x] 1. M10：证据审计与主文档回写
  - [x] 1.1 创建 `docs/reports/phase4-evidence-audit.md` — 七域证据盘点报告
    - 对七个能力域逐一盘点 Phase 1-3 产出的证据
    - 每个域包含：当前 Status_Level、Phase 1-3 产出的 Evidence_Anchor 列表、验收标准逐项达成情况（达成/未达成/部分达成）、升级判定结论（升级/维持/降级）
    - 多承载编排与降级：检查承载矩阵冻结、降级/回升演练日志、Client/Gateway 双边闭环
    - 节点恢复与共振发现：检查节点阵亡恢复演练日志、恢复判定标准
    - 会话连续性与链路漂移：检查业务连续性样板、传输层/业务层分层结论
    - 流量整形与特征隐匿：检查隐匿实验方案、实验结果、表述边界
    - eBPF 深度参与：检查覆盖图、性能证据
    - 反取证与最小运行痕迹：检查部署等级说明、基线检查清单
    - 准入控制与防滥用：检查联合演练记录、smoke/critical test 入口
    - 证据不足的域维持当前状态，记录具体差距说明
    - _需求: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9_

  - [x] 1.2 回写 `docs/governance/capability-truth-source.md` — 七域状态回写
    - 根据 1.1 盘点结论更新第五节"能力真相矩阵"每个域的字段：`当前真实能力`（描述文本）、`当前状态`（Status_Level）、`主证据锚点`（Evidence_Anchor 列表）
    - 状态升级的域：在主证据锚点中增加 Phase 1-3 产出的具体文件路径链接
    - 状态维持的域：在主证据锚点中增加差距说明，标注缺少哪些证据
    - 所有表述不超出 Phase 1-3 证据支撑的边界，不使用绝对化词汇（除非证据确实支撑）
    - _需求: 2.1, 2.2, 2.3, 2.5, 2.6_

  - [x] 1.3 回写 `docs/governance/capability-truth-source.md` — 北极星命名升级条件评估
    - 根据七域最终状态评估第四节 Upgrade_Condition 五项条件
    - 五项条件全部满足 → 标注 North_Star_Name 可从目标态升级为当前可承诺能力
    - 任一条件未满足 → 标注 North_Star_Name 仍为目标态，列出未满足的条件
    - _需求: 2.4_

  - [x] 1.4 创建 `deploy/scripts/drill-m10-evidence-audit.sh` — M10 证据审计 drill 脚本
    - 检查七域证据文件存在性（遍历 Phase 1-3 关键产物路径）
    - 检查 `capability-truth-source.md` 回写完整性（grep 七域状态字段是否已更新）
    - 非 Linux 环境正确降级
    - 捕获日志到 `deploy/evidence/m10-evidence-audit.log`
    - 脚本退出码反映检查结果
    - _需求: 1.1, 2.1_

- [x] 2. Checkpoint — M10 证据审计与回写确认
  - 确认七域证据盘点报告和主文档回写已完成，请用户确认是否有问题。

- [x] 3. M11：派生材料收敛与跨文档一致性
  - [x] 3.1 回写 `docs/governance/market-positioning-scenarios.md` — Market_Positioning 收敛
    - 审查全文，确保核心商业场景描述中引用的能力不超出 Capability_Truth_Source 第五节对应域的 Status_Level
    - `部分实现` 的能力域：在场景描述中增加限定语，标注当前边界
    - 在文件头部 `Role` 字段中确认已于当前升级周期与 Capability_Truth_Source 完成对齐
    - 删除或标注 Capability_Truth_Source 未登记的能力表述为"候选方向，当前未实现"
    - 回写后不包含任何满额承诺超出主证据锚点支撑范围的表述
    - _需求: 3.1, 3.2, 3.3, 3.4, 3.5_

  - [x] 3.2 回写 `docs/暗网基础设施防御力评价矩阵.md` — Defense_Matrix 收敛
    - 审查核心能力评分表中 Mirage V2 列的每项评分，确保评分依据与 Capability_Truth_Source 一致
    - `部分实现` 的域：将评分标注为"目标分值"或调整为反映当前真实能力的分值，备注调整原因
    - `已实现（限定表述）` 的域：在评分旁增加限定说明，引用表述边界
    - 审查"矩阵解读与话术支撑"章节，确保不包含违规表述（将部分实现写成已完全实现、无证据的绝对化词汇、目标态写成已交付）
    - 在文件头部增加对齐声明
    - 删除或标注 Capability_Truth_Source 未登记的评分项为"候选评估维度，当前无主文档锚点"
    - _需求: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6_

  - [x] 3.3 创建 `docs/reports/cross-document-consistency.md` — 跨文档一致性报告
    - 逐域比对三份文档的能力表述：Capability_Truth_Source 第五节（Status_Level + 主证据锚点）、Market_Positioning 中对应场景描述、Defense_Matrix 中对应评分与话术
    - 不一致项记录修正动作
    - 确认 Source_of_Truth_Map 中 Market_Positioning 和 Defense_Matrix 登记状态为"派生材料"
    - 发现未登记的派生材料引用能力表述 → 在 Source_of_Truth_Map 新增登记条目
    - _需求: 5.1, 5.2, 5.3, 5.4_

  - [x] 3.4 回写 `docs/governance/source-of-truth-map.md` — 派生材料登记确认
    - 确认 Market_Positioning 和 Defense_Matrix 的登记状态为"派生材料"，来源指向 Capability_Truth_Source
    - 如有新发现的未登记派生材料，新增登记条目
    - _需求: 5.3, 5.4_

  - [x] 3.5 回写 `docs/governance/capability-gap-remediation-roadmap.md` — M11 即时记账
    - 在 Phase 4 / M11 章节标注"派生材料收敛完成"
    - 记录跨文档一致性检查结果和不一致修正动作
    - _需求: 5.5_

  - [x] 3.6 创建 `deploy/scripts/drill-m11-convergence.sh` — M11 收敛验证 drill 脚本
    - 跨文档一致性检查：按七个能力域出结构化核对表（Status_Level 一致性 / 限定语存在性 / 主证据锚点引用 / 绝对化违规词检查），不仅仅是关键词 grep
    - 检查 Market_Positioning 和 Defense_Matrix 头部对齐声明是否存在
    - 检查 cross-document-consistency.md 是否已创建
    - 捕获日志到 `deploy/evidence/m11-convergence.log`
    - 脚本退出码反映检查结果
    - _需求: 5.1, 5.5_

- [x] 4. Checkpoint — M11 派生材料收敛确认
  - 确认 Market_Positioning、Defense_Matrix 收敛回写和跨文档一致性报告已完成，请用户确认是否有问题。

- [x] 5. M12：验证入口固化与周期收口
  - [x] 5.1 回写 `docs/Mirage 功能确认与功能验证任务清单.md` — Verification_Checklist 新增 M1-M12 条目
    - 新增 Phase 1-3 各里程碑产出的验证条目，每个条目包含：功能名称、确认目标、复验命令、通过标准、风险等级（P0/P1）、证据文件路径、执行负责人
    - M1-M2 条目：承载矩阵冻结确认、自动降级/回升演练复验（引用 drill 脚本路径）
    - M3 条目：节点阵亡恢复演练复验（引用 drill 脚本路径）
    - M4 条目：业务连续性样板复验（引用 drill 脚本路径）
    - M5-M7 条目：隐匿实验复验入口、eBPF 覆盖与性能复验入口
    - M8-M9 条目：部署基线检查复验、联合演练 smoke/critical test 复验
    - M10 条目：证据审计报告存在性、capability-truth-source.md 七域回写完成
    - M11 条目：cross-document-consistency.md 存在性、Market_Positioning 和 Defense_Matrix 对齐声明存在
    - M12 条目：evidence.go 和 evidence_test.go 存在性、release-verify.ps1 新增 Gate 可执行、Remediation_Roadmap Status 为 completed
    - 只引用实际存在的文件路径和测试包路径；未产出可复验脚本的条目标注"待补齐"
    - _需求: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 6.7, 6.8_

  - [x] 5.2 回写 `scripts/release-verify.ps1` — Release_Verify_Script 扩展 Phase 1-3 Gates
    - 在现有 15 项 Gate 之后追加新 Gate
    - Gate 6: Phase 1-3 关键测试包回归（多承载、节点恢复、会话连续性、隐匿协议编译、eBPF 编译回归、配额隔离、鉴权回归、日志脱敏）
    - Gate 7: 证据文件存在性检查（关键证据文件是否存在于预期路径）
    - 新增 Gate 在非 Linux 环境 SKIP（检测 `$IsLinux`）
    - 引用路径不存在时 SKIP 而非 FAIL（`Test-Path` 前置检查）
    - 引入三态结果模型：新增 `$Skip` 变量（初始化为 0），新增 Gate 条件不满足时 `$Skip++`；Result 行格式更新为 `$Pass passed, $Fail failed, $Skip skipped`
    - 现有 15 项检查行为不变（$Skip 始终为 0，向后兼容），新增检查项追加在现有 Gate 之后
    - _需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6_

  - [x] 5.3 创建 `deploy/release/evidence.go` — EvidenceManifest 结构体与默认清单
    - 定义 `EvidenceItem` 结构体：Domain、Milestone、Path、Required
    - 定义 `EvidenceManifest` 结构体：Items []EvidenceItem
    - 定义 `EvidenceResult` 结构体：MissingRequired、MissingOptional
    - 实现 `VerifyEvidenceCompleteness(manifest *EvidenceManifest, rootDir string) (*EvidenceResult, error)` 函数
    - 函数行为：遍历 manifest.Items，检查 `filepath.Join(rootDir, item.Path)` 是否存在；required 缺失收集到 MissingRequired；optional 缺失收集到 MissingOptional；`len(MissingRequired) > 0` 时返回 error（包含所有缺失 required 文件路径和所属能力域）；`len(MissingRequired) == 0` 时返回 nil error
    - nil/空 manifest → 返回空 result + nil error
    - rootDir 不存在 → 所有文件视为缺失
    - os.Stat 返回非 NotExist 错误 → 视为缺失
    - 实现 `DefaultEvidenceManifest() *EvidenceManifest` 返回覆盖 M1-M12 全部里程碑的默认清单（包括 M10 证据审计报告、M11 一致性报告、M12 evidence.go/evidence_test.go 自身）
    - 现有 `VerifyManifest` 函数不变
    - _需求: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

  - [x] 5.4 创建 `deploy/release/evidence_test.go` — VerifyEvidenceCompleteness PBT + Example Tests
    - **Property 1: VerifyEvidenceCompleteness 正确性**
    - 使用 `pgregory.net/rapid` 生成随机 EvidenceManifest（items 数量 ∈ [0,20]，每个 item 随机 required/optional）
    - 使用临时目录 + 随机创建/不创建文件模拟文件系统状态
    - 验证：返回 error 当且仅当存在至少一个 required 文件缺失；error message 包含所有缺失 required 文件路径；optional 缺失不导致 error
    - 最少 100 次迭代
    - **验证: 需求 8.1, 8.3, 8.4**
    - Example tests: AllPresent → nil error、RequiredMissing → error 包含路径、OptionalMissing → nil error、EmptyManifest → nil error、DefaultEvidenceManifest 覆盖 M1-M12
    - _需求: 8.1, 8.2, 8.3, 8.4, 8.6_

  - [x] 5.5 创建 `deploy/scripts/drill-m12-release-gate.sh` — M12 发布门禁 drill 脚本
    - 执行 `release-verify.ps1`（如有 PowerShell 环境）或跳过并标注
    - 执行 `go test` 运行 `deploy/release/` 包测试（含 VerifyEvidenceCompleteness PBT）
    - 检查 Remediation_Roadmap Status 字段是否已更新为 `completed`
    - 捕获日志到 `deploy/evidence/m12-release-gate.log`
    - 脚本退出码反映检查结果
    - _需求: 7.1, 8.1_

- [x] 6. Checkpoint — M12 验证入口固化确认
  - 确认 Verification_Checklist 回写、release-verify.ps1 扩展、evidence.go 实现和测试已完成，请用户确认是否有问题。

- [x] 7. M12：周期收口与归档
  - [x] 7.1 回写 `docs/governance/capability-gap-remediation-roadmap.md` — Remediation_Roadmap 收口
    - 文件头部 `Status` 字段从 `temporary` 更新为 `completed`，增加完成日期
    - 第六节 Phase 4 出关标准下记录实际达成情况：M10-M12 完成状态、三者一致性、命名冲突状态
    - 新增"第八节：本轮总结"：七域最终状态汇总表（域名 + 起始状态 + 最终状态 + 是否升级）、North_Star_Name 升级判定结论、遗留差距清单
    - 诚实记录未完成项，不将未完成项标记为已完成
    - _需求: 9.1, 9.2, 9.3, 9.5_

  - [x] 7.2 回写 `docs/governance/source-of-truth-map.md` — 归档登记
    - 将 Remediation_Roadmap 状态从"临时有效材料"更新为"已归档"，标注有效范围为本升级周期
    - _需求: 9.4_

- [x] 8. Exit：Phase 4 出关判定
  - 逐项检查出关标准：
    - Capability_Truth_Source 第五节已完成全部七域状态回写，Evidence_Anchor 列表已更新（需求 10.1）
    - Market_Positioning 和 Defense_Matrix 已完成对齐回写，跨文档一致性检查已通过（需求 10.2）
    - Verification_Checklist 已新增 M1-M12 对应验证条目（需求 10.3）
    - Release_Verify_Script 已扩展 Phase 1-3 关键检查项，当前环境无非预期 FAIL（需求 10.4）
    - Verify_Go 模块已新增 VerifyEvidenceCompleteness，Evidence Manifest 覆盖 M1-M12（需求 10.5）
    - Remediation_Roadmap 已标记为 completed 并包含本轮总结（需求 10.6）
    - 三者一致性确认：Capability_Truth_Source / 派生叙事文档 / 验证入口无冲突（需求 10.7）
    - North_Star_Name 与当前真实能力不再冲突（需求 10.8）
  - 如有不一致 → 标记为未出关，需修正后重新判定
  - _需求: 10.1, 10.2, 10.3, 10.4, 10.5, 10.6, 10.7, 10.8_

## 备注

- PBT 仅 1 个（Property 1），为必须项（非可选），使用 `pgregory.net/rapid`，最少 100 次迭代
- PBT 测试文件：`deploy/release/evidence_test.go`
- Phase 4 不新增功能，不扩展产品边界
- 状态升级只能基于 Phase 1-3 实际产出的证据
- 证据产物固定路径：`docs/reports/`（报告）、`deploy/scripts/`（drill 脚本）、`deploy/release/`（Go 代码）、`deploy/evidence/`（drill 日志）
- 术语一致性：Capability_Truth_Source / Market_Positioning / Defense_Matrix / Verification_Checklist / Release_Verify_Script / Remediation_Roadmap / Source_of_Truth_Map 在所有引用中保持一致
