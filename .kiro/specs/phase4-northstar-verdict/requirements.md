# 需求文档：Phase 4 北极星升级判定

## 简介

本需求文档覆盖北极星实施计划 Phase 4（北极星升级判定），目标是将 Phase 1-3 产出的局部证据收敛为正式能力升级判定。

Phase 4 包含三个里程碑（M10-M12），覆盖全部七个能力域：
- 多承载编排与降级
- 节点恢复与共振发现
- 会话连续性与链路漂移
- 流量整形与特征隐匿
- eBPF 深度参与的数据面与防护
- 反取证与最小运行痕迹
- 准入控制与防滥用

Phase 4 不新增能力域，不扩展产品边界，只负责三件事：
1. M10：根据真实证据回写主文档状态
2. M11：让派生材料与主文档边界对齐
3. M12：把验证入口固化为可复用的发布级基础设施

关键原则：
1. 状态升级只能基于 Phase 1-3 实际产出的证据，不能基于叙事强度
2. 证据不足 → 维持当前状态，记录差距，不得提前升级
3. 派生材料不得超出 `capability-truth-source.md` 的边界
4. 所有验证入口必须可复现、可在下一发布周期直接复用
5. Phase 1-3 经验教训：不做满额承诺、诚实标注证据强度、包级测试引用（不写具体测试数量）、固定产物路径

## 术语表

- **Capability_Truth_Source**: 能力真相源文档 `docs/governance/capability-truth-source.md`，七个能力域的北极星目标、验收标准与当前真实能力的唯一治理入口
- **Remediation_Roadmap**: 北极星实施计划 `docs/governance/capability-gap-remediation-roadmap.md`，当前升级周期的阶段推进 companion
- **Source_of_Truth_Map**: 真相源地图 `docs/governance/source-of-truth-map.md`，问题域到主真相源的映射登记
- **Market_Positioning**: 市场定位场景文档 `docs/governance/market-positioning-scenarios.md`，商业场景与交付叙事的派生材料
- **Defense_Matrix**: 暗网基础设施防御力评价矩阵 `docs/暗网基础设施防御力评价矩阵.md`，商业定位与销售矩阵的派生材料
- **Verification_Checklist**: 功能验证清单 `docs/Mirage 功能确认与功能验证任务清单.md`，当前发布周期验证材料
- **Release_Verify_Script**: 发布验证脚本 `scripts/release-verify.ps1`，15 项检查的本地复验入口
- **Verify_Go**: 发布产物签名与验证 `deploy/release/verify.go`，manifest 签名校验与二进制 hash 验证
- **Manifest_Go**: 发布清单 `deploy/release/manifest.go`，版本、构建时间、Git commit、SHA-256、Ed25519 签名
- **Evidence_Anchor**: 证据锚点，指向具体文件路径、测试包路径或运行日志的链接，用于证明某项能力状态的依据
- **Status_Level**: 能力状态等级，取值为 `已实现`、`已实现（限定表述）`、`部分实现`、`未闭环` 四级
- **North_Star_Name**: 北极星产品命名，即 `Hyper-Resilient Overlay Network` 和 `Zero-Trust SD-WAN 隐蔽数据链路`
- **Upgrade_Condition**: 升级条件，`capability-truth-source.md` 第四节定义的五项条件，满足后才允许将 North_Star_Name 从目标态升级为当前可承诺能力
- **Phase1_Evidence**: Phase 1 产出证据，包括承载矩阵（`docs/governance/carrier-matrix.md`）、降级/回升演练日志、节点恢复演练记录、业务连续性样板
- **Phase2_Evidence**: Phase 2 产出证据，包括隐匿实验方案（`docs/reports/stealth-experiment-plan.md`）、实验结果（`docs/reports/stealth-experiment-results.md`）、表述边界（`docs/reports/stealth-claims-boundary.md`）、eBPF 覆盖图（`docs/reports/ebpf-coverage-map.md`）
- **Phase3_Evidence**: Phase 3 产出证据，包括部署等级说明（`docs/reports/deployment-tiers.md`）、基线检查清单（`docs/reports/deployment-baseline-checklist.md`）、联合演练记录（`docs/reports/access-control-joint-drill.md`）
- **Drill_Logs**: 演练日志目录 `deploy/evidence/`，Phase 1-3 各里程碑的运行日志存档
- **DPI_Audit_Artifacts**: DPI 实验数据目录 `artifacts/dpi-audit/`，M6 抓包与统计结果
- **eBPF_Perf_Artifacts**: eBPF 性能数据目录 `artifacts/ebpf-perf/`，M7 benchmark 与观测数据

## 需求

### 需求 1：七域证据盘点与升级判定（M10 — 证据审计）

**用户故事：** 作为项目治理者，我希望对七个能力域逐一盘点 Phase 1-3 产出的证据，判定每个域是否满足升级条件，以便基于事实而非叙事做出状态回写决策。

#### 验收标准

1. THE Capability_Truth_Source SHALL 对七个能力域逐一执行证据盘点，每个域的盘点结果包含：当前 Status_Level、Phase 1-3 产出的 Evidence_Anchor 列表、验收标准逐项达成情况（达成/未达成/部分达成）、升级判定结论（升级/维持/降级）
2. WHEN 盘点"多承载编排与降级"时，THE Capability_Truth_Source SHALL 检查以下证据：承载矩阵是否冻结（Phase1_Evidence）、自动降级/回升演练是否通过（Drill_Logs）、Client/Gateway 双边闭环是否完整；IF 证据满足验收标准，THEN 状态从 `部分实现` 升级为 `已实现` 或 `已实现（限定表述）`
3. WHEN 盘点"节点恢复与共振发现"时，THE Capability_Truth_Source SHALL 检查以下证据：节点阵亡恢复演练是否通过（Drill_Logs）、恢复成功/超时/失败回退判定标准是否明确；IF 证据满足验收标准，THEN 状态从 `部分实现` 升级为 `已实现` 或 `已实现（限定表述）`
4. WHEN 盘点"会话连续性与链路漂移"时，THE Capability_Truth_Source SHALL 检查以下证据：业务连续性样板是否通过（Drill_Logs）、"传输层切换"与"业务层无感"是否有分层结论；IF 证据满足验收标准，THEN 状态从 `部分实现` 升级为至少 `已实现（限定表述）`，且业务影响边界清晰
5. WHEN 盘点"流量整形与特征隐匿"时，THE Capability_Truth_Source SHALL 检查以下证据：隐匿实验方案是否冻结（Phase2_Evidence）、最小实验结果是否产出（DPI_Audit_Artifacts）、对外表述边界是否明确；IF 证据满足验收标准，THEN 状态从 `部分实现` 升级为 `已实现（限定表述）`
6. WHEN 盘点"eBPF 深度参与的数据面与防护"时，THE Capability_Truth_Source SHALL 检查以下证据：eBPF 覆盖图是否完成（Phase2_Evidence）、关键路径性能证据是否存在（eBPF_Perf_Artifacts）；IF 证据充分，THEN 维持或确认 `已实现（限定表述）` 状态
7. WHEN 盘点"反取证与最小运行痕迹"时，THE Capability_Truth_Source SHALL 检查以下证据：部署等级说明是否完成（Phase3_Evidence）、基线检查清单是否可执行；IF 证据充分，THEN 维持或确认 `已实现（限定表述）` 状态
8. WHEN 盘点"准入控制与防滥用"时，THE Capability_Truth_Source SHALL 检查以下证据：联合演练记录是否完整（Phase3_Evidence）、smoke/critical test 入口是否固化；IF 证据充分，THEN 维持或确认 `已实现` 状态
9. IF 某个能力域的 Phase 1-3 证据不足以满足验收标准，THEN THE Capability_Truth_Source SHALL 维持该域当前 Status_Level 不变，并在主证据锚点中记录具体差距说明

### 需求 2：主文档状态回写（M10 — 回写执行）

**用户故事：** 作为项目治理者，我希望根据需求 1 的盘点结论，将状态变更正式写入 Capability_Truth_Source，以便主文档反映真实能力而非历史快照。

#### 验收标准

1. THE Capability_Truth_Source 第五节"能力真相矩阵"SHALL 根据需求 1 的盘点结论更新每个能力域的以下字段：`当前真实能力`（描述文本）、`当前状态`（Status_Level）、`主证据锚点`（Evidence_Anchor 列表）
2. WHEN 某个能力域状态发生升级时，THE Capability_Truth_Source SHALL 在该域的 `主证据锚点` 中增加 Phase 1-3 产出的具体文件路径链接，每个链接指向实际存在的文件或测试包
3. WHEN 某个能力域状态维持不变时，THE Capability_Truth_Source SHALL 在该域的 `主证据锚点` 中增加差距说明，标注缺少哪些证据、需要在下一周期补齐
4. THE Capability_Truth_Source 第四节"北极星产品命名"SHALL 根据七域最终状态评估 Upgrade_Condition 是否满足：IF 五项条件全部满足，THEN 标注 North_Star_Name 可从目标态升级为当前可承诺能力；IF 任一条件未满足，THEN 标注 North_Star_Name 仍为目标态，并列出未满足的条件
5. THE Capability_Truth_Source 回写后的所有表述 SHALL 不超出 Phase 1-3 证据支撑的边界，不使用"全部""所有""100%"等绝对化词汇（除非证据确实支撑）
6. THE Capability_Truth_Source 回写 SHALL 遵循变更规则（第八节）：先改主文档，再改派生材料

### 需求 3：Market_Positioning 收敛回写（M11 — 派生材料 1）

**用户故事：** 作为项目治理者，我希望市场定位文档与主文档对齐，以便商业叙事不再包含超出当前真实能力的承诺。

#### 验收标准

1. WHEN Capability_Truth_Source 回写完成后，THE Market_Positioning SHALL 审查全文，确保以下内容与 Capability_Truth_Source 第五节一致：核心商业场景描述中引用的能力不超出对应域的 Status_Level、交付形态描述不暗示尚未实现的能力
2. WHEN 某个商业场景引用了 Status_Level 为 `部分实现` 的能力域时，THE Market_Positioning SHALL 在该场景描述中增加限定语，标注该能力的当前边界（例如"当前支持 QUIC → WSS 降级，WebRTC/ICMP/DNS 路径待闭环"）
3. THE Market_Positioning SHALL 在文件头部的 `Role` 字段中确认：本文件已于当前升级周期与 Capability_Truth_Source 完成对齐，对齐日期为 M11 完成日期
4. IF Market_Positioning 中存在 Capability_Truth_Source 未登记的能力表述，THEN THE Market_Positioning SHALL 删除该表述或将其标注为"候选方向，当前未实现"
5. THE Market_Positioning 回写后 SHALL 不包含任何满额承诺超出 Capability_Truth_Source 主证据锚点支撑范围的表述

### 需求 4：Defense_Matrix 收敛回写（M11 — 派生材料 2）

**用户故事：** 作为项目治理者，我希望防御力评价矩阵的评分和话术与主文档对齐，以便销售材料不再包含无证据支撑的满分评价。

#### 验收标准

1. WHEN Capability_Truth_Source 回写完成后，THE Defense_Matrix SHALL 审查核心能力评分表中 Mirage V2 列的每项评分，确保评分依据与 Capability_Truth_Source 第五节的 Status_Level 和主证据锚点一致
2. WHEN 某项评分对应的能力域 Status_Level 为 `部分实现` 时，THE Defense_Matrix SHALL 将该项评分标注为"目标分值"而非"当前分值"，或将评分调整为反映当前真实能力的分值，并在备注中说明调整原因
3. WHEN 某项评分对应的能力域 Status_Level 为 `已实现（限定表述）` 时，THE Defense_Matrix SHALL 在该项评分旁增加限定说明，引用 Capability_Truth_Source 中的表述边界
4. THE Defense_Matrix 的"矩阵解读与话术支撑"章节 SHALL 审查每条话术，确保不包含以下违规表述：(a) 将 `部分实现` 的能力写成已完全实现、(b) 使用"秒杀""必然瘫痪""绝对优于"等绝对化词汇且无实验证据支撑、(c) 将目标态能力写成当前已交付能力
5. THE Defense_Matrix SHALL 在文件头部增加对齐声明：本文件已于当前升级周期与 Capability_Truth_Source 完成对齐，评分和话术均受主文档边界约束
6. IF Defense_Matrix 中存在 Capability_Truth_Source 未登记的能力维度或评分项，THEN THE Defense_Matrix SHALL 删除该项或标注为"候选评估维度，当前无主文档锚点"

### 需求 5：跨文档一致性验证（M11 — 收敛闭环）

**用户故事：** 作为项目治理者，我希望验证 Capability_Truth_Source、Market_Positioning、Defense_Matrix 三份文档在回写后保持一致，以便不再出现文档间的能力表述冲突。

#### 验收标准

1. WHEN M11 回写全部完成后，THE Remediation_Roadmap SHALL 记录一次跨文档一致性检查，逐域比对以下三份文档的能力表述：Capability_Truth_Source 第五节（Status_Level + 主证据锚点）、Market_Positioning 中对应场景描述、Defense_Matrix 中对应评分与话术
2. WHEN 跨文档比对发现不一致时（某份派生材料的表述超出 Capability_Truth_Source 边界），THE Remediation_Roadmap SHALL 记录不一致项并标注修正动作
3. THE Source_of_Truth_Map SHALL 确认 Market_Positioning 和 Defense_Matrix 的登记状态为"派生材料"，且来源指向 Capability_Truth_Source
4. IF 跨文档一致性检查发现 Source_of_Truth_Map 中未登记的派生材料引用了能力表述，THEN THE Source_of_Truth_Map SHALL 新增该材料的登记条目，标注为派生材料
5. WHEN 跨文档一致性检查全部通过后，THE Remediation_Roadmap SHALL 在 Phase 4 / M11 章节标注"派生材料收敛完成"

### 需求 6：Verification_Checklist 回写（M12 — 验证入口 1）

**用户故事：** 作为发布负责人，我希望功能验证清单反映 M1-M12 全部里程碑新增的验证入口，以便下一发布周期可以直接复用。

#### 验收标准

1. THE Verification_Checklist SHALL 新增 M1-M12 各里程碑产出的验证条目，每个条目包含：功能名称、确认目标、复验命令、通过标准、风险等级（P0/P1）、证据文件路径、执行负责人
2. WHEN 新增 M1-M2 验证条目时，THE Verification_Checklist SHALL 包含：承载矩阵冻结确认、自动降级/回升演练复验命令（引用 Drill_Logs 中的脚本路径）
3. WHEN 新增 M3 验证条目时，THE Verification_Checklist SHALL 包含：节点阵亡恢复演练复验命令（引用 Drill_Logs 中的脚本路径）
4. WHEN 新增 M4 验证条目时，THE Verification_Checklist SHALL 包含：业务连续性样板复验命令（引用 Drill_Logs 中的脚本路径）
5. WHEN 新增 M5-M7 验证条目时，THE Verification_Checklist SHALL 包含：隐匿实验复验入口（引用 DPI_Audit_Artifacts）、eBPF 覆盖与性能复验入口（引用 eBPF_Perf_Artifacts）
6. WHEN 新增 M8-M9 验证条目时，THE Verification_Checklist SHALL 包含：部署基线检查复验命令、联合演练 smoke/critical test 复验命令（引用 Phase3_Evidence）
7. THE Verification_Checklist 新增条目 SHALL 只引用实际存在的文件路径和测试包路径，不引用尚未创建的文件
8. IF Phase 1-3 某个里程碑未产出可复验的脚本或测试，THEN THE Verification_Checklist SHALL 在对应条目中标注"待补齐"并说明缺失内容

### 需求 7：Release_Verify_Script 扩展（M12 — 验证入口 2）

**用户故事：** 作为发布负责人，我希望发布验证脚本覆盖 Phase 1-3 新增的关键验证点，以便发布门禁自动检查北极星相关能力的回归状态。

#### 验收标准

1. THE Release_Verify_Script SHALL 新增 Phase 1-3 关键验证点的检查项（Gate），每个 Gate 包含：检查名称、检查命令、通过/失败判定逻辑
2. WHEN 新增检查项时，THE Release_Verify_Script SHALL 只添加可在本地环境执行的检查（不依赖远程服务或真实网络环境），对于需要特定环境的检查（如 eBPF 需要 Linux + clang），SHALL 在非目标环境下正确 SKIP 而非 FAIL
3. THE Release_Verify_Script SHALL 新增以下类别的检查项：(a) Phase 1 关键测试包回归（多承载、节点恢复、会话连续性相关测试包）、(b) Phase 2 关键测试包回归（隐匿协议编译、eBPF 编译回归）、(c) Phase 3 关键测试包回归（配额隔离、鉴权回归、日志脱敏）、(d) 证据文件存在性检查（关键证据文件是否存在于预期路径）
4. THE Release_Verify_Script 新增检查项后，结果统计 SHALL 采用三态模型（PASS/FAIL/SKIP）：`$Pass passed, $Fail failed, $Skip skipped`，总和等于实际检查项数。现有 15 项 Gate 保持二态（PASS/FAIL），新增 Gate 引入 SKIP 态用于环境不满足或路径缺失的场景。Result 行格式更新为包含 SKIP 计数
5. IF 新增检查项引用的测试包或文件路径在当前代码库中不存在，THEN THE Release_Verify_Script SHALL 将该检查项标注为 SKIP 并输出原因，不将其计为 FAIL
6. THE Release_Verify_Script 的修改 SHALL 保持向后兼容：现有 15 项检查的行为不变，新增检查项追加在现有 Gate 之后

### 需求 8：Verify_Go 能力域覆盖扩展（M12 — 验证入口 3）

**用户故事：** 作为发布负责人，我希望 Go 语言的发布验证模块能够校验北极星相关的证据完整性，以便 CI/CD 流水线可以自动判定证据是否齐全。

#### 验收标准

1. THE Verify_Go 模块 SHALL 新增一个 `VerifyEvidenceCompleteness` 函数，接受证据清单（Evidence Manifest）作为输入，逐项检查清单中列出的文件是否存在于预期路径
2. THE Evidence Manifest SHALL 定义为结构化数据（Go struct 或 JSON），包含以下字段：能力域名称、里程碑编号（M1-M12）、证据文件路径列表、每个文件的预期状态（required/optional）
3. WHEN `VerifyEvidenceCompleteness` 检查到 required 文件缺失时，SHALL 返回错误信息，包含缺失文件路径和所属能力域
4. WHEN `VerifyEvidenceCompleteness` 检查到 optional 文件缺失时，SHALL 返回警告信息但不视为验证失败
5. THE Verify_Go 模块 SHALL 保持现有 `VerifyManifest` 函数不变，`VerifyEvidenceCompleteness` 作为独立函数新增
6. THE Evidence Manifest 的默认内容 SHALL 覆盖 M1-M12 全部里程碑的关键证据文件路径，路径与 Capability_Truth_Source 中的主证据锚点一致

### 需求 9：Remediation_Roadmap 收口与归档标记（M12 — 周期收口）

**用户故事：** 作为项目治理者，我希望本轮实施计划在 Phase 4 完成后正式收口，以便下一轮升级周期有清晰的起点。

#### 验收标准

1. WHEN M10-M12 全部完成后，THE Remediation_Roadmap SHALL 在文件头部的 `Status` 字段从 `temporary` 更新为 `completed`，并增加完成日期
2. THE Remediation_Roadmap SHALL 在第六节"阶段出关标准"的 Phase 4 出关标准下记录实际达成情况：(a) M10-M12 是否全部完成、(b) Capability_Truth_Source / 派生叙事文档 / 验证入口三者是否一致、(c) 对外命名与当前真实能力是否不再冲突
3. THE Remediation_Roadmap SHALL 新增"第八节：本轮总结"，包含：(a) 七域最终状态汇总表（域名 + 起始状态 + 最终状态 + 是否升级）、(b) North_Star_Name 升级判定结论（可升级/不可升级 + 原因）、(c) 遗留差距清单（未满足升级条件的域 + 具体差距 + 建议下一周期动作）
4. WHEN 本轮总结完成后，THE Source_of_Truth_Map SHALL 将 Remediation_Roadmap 的状态从"临时有效材料"更新为"已归档"，标注有效范围为本升级周期
5. IF 本轮存在未完成的里程碑或未满足的升级条件，THEN THE Remediation_Roadmap 的总结 SHALL 诚实记录，不将未完成项标记为已完成

### 需求 10：Phase 4 出关判定

**用户故事：** 作为项目治理者，我希望 Phase 4 有明确的出关标准，以便判定本轮北极星升级周期是否正式结束。

#### 验收标准

1. WHEN Phase 4 出关判定时，THE Capability_Truth_Source 第五节 SHALL 已完成全部七域的状态回写，每个域的 Evidence_Anchor 列表已更新
2. WHEN Phase 4 出关判定时，THE Market_Positioning 和 Defense_Matrix SHALL 已完成与 Capability_Truth_Source 的对齐回写，跨文档一致性检查已通过
3. WHEN Phase 4 出关判定时，THE Verification_Checklist SHALL 已新增 M1-M12 对应的验证条目
4. WHEN Phase 4 出关判定时，THE Release_Verify_Script SHALL 已扩展 Phase 1-3 关键检查项，且在当前环境下执行无非预期 FAIL
5. WHEN Phase 4 出关判定时，THE Verify_Go 模块 SHALL 已新增 `VerifyEvidenceCompleteness` 函数，且 Evidence Manifest 覆盖 M1-M12
6. WHEN Phase 4 出关判定时，THE Remediation_Roadmap SHALL 已标记为 `completed` 并包含本轮总结
7. IF Phase 4 出关判定时 Capability_Truth_Source、派生叙事文档、验证入口三者存在不一致，THEN Phase 4 SHALL 标记为未出关，需修正后重新判定
8. WHEN Phase 4 出关后，对外命名（North_Star_Name）与当前真实能力 SHALL 不再冲突：IF 升级条件满足则命名可作为当前能力承诺；IF 升级条件未满足则命名仍标注为目标态
