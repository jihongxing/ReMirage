---
Status: authoritative
Target Truth: ReMirage 能力目标、验收标准与当前真实能力的唯一治理入口
---

# Capability Truth Source

本文件是 ReMirage 对“我们想做到什么、什么算做到、现在实际做到哪里”的唯一真相源。

它解决三个长期容易混在一起的问题：

1. 北极星目标写得很强，但没有和当前实现状态分层。
2. 销售材料、路演矩阵、技术愿景容易被误当成当前已交付能力。
3. 验收缺少统一口径，导致“设计存在”“代码存在”“真实可验”被混为一谈。

从现在开始，凡是涉及以下问题，统一先看本文：

- 某项能力是不是项目的正式目标
- 某项能力达到什么程度才允许对外宣称
- 某项能力当前到底是已实现、部分实现，还是尚未闭环
- 一份商业文案是否可以作为当前能力描述

## 一、三层模型

本项目所有能力陈述都必须分成三层，禁止再混写：

### 1. 北极星目标

回答“我们最终想做到什么”。

特点：

- 可以比当前实现更强
- 可以作为改造和升级方向
- 可以用于路线牵引
- 不能自动等于当前真实能力

### 2. 验收标准

回答“做到什么程度，才允许把这项能力算作已交付”。

特点：

- 必须可验证
- 必须有明确证据锚点
- 必须能区分“代码存在”和“行为闭环”
- 不通过验收，不得按已实现对外承诺

### 3. 当前真实能力

回答“当前版本基于代码、测试、脚本和运行时证据，真实做到哪里”。

特点：

- 只能引用权威文档、运行时代码、测试资产、发布验证脚本
- 必须接受降级表述
- 允许写“部分实现”“未闭环”
- 不允许为了营销需要而抬高等级

## 二、能力状态等级

本文统一使用以下状态等级：

| 状态 | 含义 | 是否可作为当前对外能力承诺 |
|------|------|----------------------------|
| `已实现` | 已有明确验收入口，关键证据闭环 | 可以 |
| `已实现（限定表述）` | 能力存在且有证据，但表述边界必须收紧 | 可以，但不得夸大 |
| `部分实现` | 有设计、有代码锚点、但缺少完整闭环或端到端证据 | 不可以按满额承诺 |
| `未闭环` | 仅有方向或局部实现，不能作为当前能力输出 | 不可以 |

## 三、对外表述规则

### 1. 商业矩阵不是当前能力真相源

销售矩阵、评分表、路演文案、市场对比材料只能是派生材料，不能单独决定当前项目能力。

### 2. 对外比较必须条件化

凡是涉及与外部协议或产品比较的表述，必须满足：

1. 明确这是“商业定位判断”还是“实验结果”。
2. 若是实验结果，必须给出版本、部署前提、测量方法和证据来源。
3. 若没有统一测试前提，不允许使用“秒杀”“必然瘫痪”“绝对优于”这类绝对化词汇。

### 3. 禁止把目标态写成现状态

出现以下情况时，必须降级表述：

- 只有代码入口，没有端到端演练
- 只有 Gateway 侧能力，没有 Client/Gateway 双边闭环
- 只有测试桩，没有发布级验证
- 只有架构设计，没有运行时主链收口

## 四、北极星产品命名

在治理体系内，ReMirage 的正式北极星产品命名为：

- `Hyper-Resilient Overlay Network`
- `Zero-Trust SD-WAN 隐蔽数据链路`

“影子网络”只作为叙事标签使用，不作为当前能力验收名词。

### 1. 这些命名在这里代表什么

- `Hyper-Resilient Overlay Network` 强调多承载编排、节点恢复、链路漂移控制和业务连续性
- `Zero-Trust SD-WAN 隐蔽数据链路` 强调受控接入、策略治理、低暴露传输和可运营基线

### 2. 何时可以把北极星命名当作当前能力承诺

只有同时满足以下条件，才允许把上述命名从“目标态”升级为“当前可承诺能力”：

1. `多承载编排与降级` 达到 `已实现`
2. `节点恢复与共振发现` 达到 `已实现`
3. `会话连续性与链路漂移` 至少达到 `已实现（限定表述）`，且业务影响边界清晰
4. `准入控制与防滥用` 达到 `已实现`
5. 若要使用“隐蔽数据链路”表述，`流量整形与特征隐匿`、`eBPF 深度参与的数据面与防护`、`反取证与最小运行痕迹` 至少达到 `已实现（限定表述）`

在未满足上述条件前，这些命名只能作为北极星目标、路线牵引和派生叙事，不得直接当作当前版本的满额能力标题。

### 3. Phase 4 M10 升级条件评估结论

评估日期：2026-04-25

| 条件 | 要求 | 当前状态 | 是否满足 |
|------|------|----------|----------|
| 条件 1 | 多承载编排与降级 达到 `已实现` | `已实现（限定表述）` | 未满足 |
| 条件 2 | 节点恢复与共振发现 达到 `已实现` | `已实现（限定表述）` | 未满足 |
| 条件 3 | 会话连续性与链路漂移 至少达到 `已实现（限定表述）` | `已实现（限定表述）` | 满足 |
| 条件 4 | 准入控制与防滥用 达到 `已实现` | `已实现` | 满足 |
| 条件 5 | 隐蔽三域至少达到 `已实现（限定表述）` | 流量整形为 `部分实现` | 未满足 |

**结论：North_Star_Name 仍为目标态，不可升级为当前可承诺能力。**

未满足条件：条件 1（多承载缺真实网络演练）、条件 2（节点恢复缺真实演练）、条件 5（流量整形缺实验结果）。

## 五、能力真相矩阵

| 能力域 | 北极星目标 | 验收标准 | 当前真实能力 | 当前状态 | 主证据锚点 |
|--------|------------|----------|--------------|----------|------------|
| 多承载编排与降级 | 在复杂网络环境下形成统一多承载主链，按 `QUIC -> WebRTC -> WSS -> ICMP/DNS` 组织主通道与回退通道 | 1. Gateway 与 Client 均有统一主链。 2. 至少完成主通道失败后的自动降级验证。 3. 回升路径有可复验证据。 4. 未经验证的承载不得按稳定能力宣称 | Gateway 侧 Orchestrator 已作为唯一多协议主链接线；Client 侧 ClientOrchestrator QUIC→WSS 降级/回升已通过代码级模拟验证（含 PBT Property 2/3 各 100 次迭代）；Gateway 侧优先级排序已通过 PBT Property 4/5 验证；承载矩阵已冻结，QUIC/WSS 为正式承诺，WebRTC/ICMP/DNS 为已接线待闭环。**Phase 4 M10 盘点结论**：验收标准 4 项全部达成，但当前证据为代码级模拟（mock transport），尚无真实网络演练证据，不满足升级为"已实现"的条件。维持当前状态。差距：需补齐真实网络环境下的端到端降级/回升演练证据 | `已实现（限定表述）` | `docs/governance/carrier-matrix.md`、`docs/protocols/gtunnel.md`、`mirage-gateway/cmd/gateway/main.go`、`phantom-client/pkg/gtclient/client.go`、`phantom-client/pkg/gtclient/client_orchestrator.go`、`phantom-client/pkg/gtclient/client_orchestrator_test.go`、`mirage-gateway/pkg/gtunnel/orchestrator_test.go`、`deploy/evidence/m2-degradation-drill.log`、`deploy/scripts/drill-m2-degradation.sh`、`docs/governance/source-of-truth-map.md`、`docs/reports/phase4-evidence-audit.md` |
| 节点恢复与共振发现 | 节点被封锁或失联后，客户端可通过外部信令源自动发现新入口并完成恢复 | 1. DoH / Gist / Mastodon 等至少一条恢复链路有自动化验证。 2. 真实演练能证明节点阵亡后可恢复。 3. 恢复失败会进入明确定义的降级路径 | RecoveryFSM 三阶段恢复已通过代码级模拟验证（含 PBT Property 1 单调性 100 次迭代）；Resolver First-Win 竞速已通过 PBT Property 6/7 验证；节点阵亡恢复链路已通过集成测试验证；恢复失败返回明确错误。**Phase 4 M10 盘点结论**：验收标准 3 项中 2 项达成、1 项部分达成（真实演练），维持当前状态。差距：当前证据为代码级模拟，尚无真实节点阵亡演练证据，需下一周期补齐 | `已实现（限定表述）` | `phantom-client/pkg/resonance/resolver.go`、`phantom-client/pkg/resonance/resolver_test.go`、`phantom-client/pkg/gtclient/recovery_fsm_test.go`、`phantom-client/pkg/gtclient/node_death_drill_test.go`、`deploy/evidence/m3-node-death-drill.log`、`deploy/scripts/drill-m3-node-death.sh`、`docs/reports/phase4-evidence-audit.md` |
| 会话连续性与链路漂移 | 底层承载切换时，上层业务会话尽可能无感，至少不把“连接存在”误写成“业务完全无感” | 1. 至少对一个受支持路径对完成长连接业务演练。 2. 证明链路切换不会造成目标业务中断。 3. 若只能保证传输层切换，不得对外写成“所有 TCP 业务无感” | switchWithTransaction 事务式切换已通过代码级模拟验证（含 PBT Property 8/9 各 100 次迭代）；传输层切换在 mock 环境下正确执行；业务连续性样板测试记录了切换前后数据流状态。**Phase 4 M10 盘点结论**：验收标准 3 项中 2 项达成、1 项部分达成，维持当前状态。差距：mock 环境丢包为 0，不能直接推导为业务层无感；真实网络环境下的业务层影响需下一周期单独演练量化 | `已实现（限定表述）` | `docs/protocols/gtunnel.md`、`phantom-client/pkg/gtclient/client.go`、`phantom-client/pkg/gtclient/business_continuity_test.go`、`deploy/evidence/m4-continuity-report.md`、`deploy/scripts/drill-m4-continuity.sh`、`docs/reports/phase4-evidence-audit.md` |
| 流量整形与特征隐匿 | NPM / B-DNA / Jitter-Lite / VPC 协同工作，在包长、时序、画像层面降低可识别性 | 1. 配置主源、加载路径、编译路径、运行时挂载全部存在。 2. 关键 `.c` 文件可真实编译。 3. 若要宣称 DPI/ML 对抗效果，必须有独立实验或基准证据 | 协议主源和运行时锚点已建立；eBPF 编译回归与本地双证据已补齐；M6 隐匿实验已设计（四个检测面：握手指纹、包长分布、时序分布、简单分类器），受控环境基线待采集；目前没有可作为权威依据的 JA3/JA4/DPI/ML 对抗实验结果，因此不能按“10 分隐匿效果”对外宣称。Phase 2 出关判定：实验框架已建立（方案冻结、脚本就绪、PBT 通过），受控环境基线待 Linux 环境实际采集。证据不足以升级为'已实现（限定表述）'，维持'部分实现'。差距：缺少实际抓包数据和分类器实验结果，需下一周期在 Linux 环境采集受控环境基线 | `部分实现` | `docs/protocols/source-of-truth.md`、`docs/protocols/npm.md`、`bdna.md`、`jitter-lite.md`、`vpc.md`、`mirage-gateway/pkg/ebpf/bpf_compile_test.go`、`mirage-gateway/scripts/test-ebpf-compile.sh`、`docs/reports/stealth-experiment-results.md`（M6 实验结论待采集）、`docs/reports/stealth-experiment-plan.md`、`docs/reports/stealth-claims-boundary.md`、`artifacts/dpi-audit/`、`deploy/scripts/drill-m6-experiment.sh`、`docs/reports/phase4-evidence-audit.md` |
| eBPF 深度参与的数据面与防护 | 让 eBPF 深度承担关键防护、观测和部分数据面处理职责，形成性能与防护护城河 | 1. 关键 BPF 程序可编译、可挂载、可接线。 2. 关键 Map / Ring Buffer / Threat 回调闭环存在。 3. 若要宣称“全流量全链路零拷贝”，必须有更高等级证据 | eBPF 加载、Threat 事件、B-DNA / Jitter / L1 防护、编译回归均已具备证据；M7 覆盖图已产出（`ebpf-coverage-map.md`），区分运行态挂载（15 个程序）、源码未挂载（9 个 SEC 函数）、纯用户态路径（7 条关键路径），结论为“eBPF 深度参与关键路径（XDP 包长控制、TC 指纹重写、TC 时域扰动、sockmap 加速），但非全链路零拷贝”；性能证据待 Linux 环境实际采集（延迟/CPU/内存占位文件已创建于 `artifacts/ebpf-perf/`）；项目仍保留大量用户态处理链路（G-Tunnel/FEC/QUIC/TLS/G-Switch），因此只能宣称“eBPF 深度参与关键路径”，不能宣称“所有流量都在驱动层零拷贝完成”。当前维持“已实现（限定表述）”：覆盖图证据已闭环，性能证据待采集，不满足升级为“已实现”的条件。Phase 2 出关判定：覆盖图证据已闭环（15 个运行态程序、路径对照表、参与度结论），性能证据待 Linux 环境采集。维持'已实现（限定表述）'。 | `已实现（限定表述）` | `mirage-gateway/cmd/gateway/main.go`、`mirage-gateway/pkg/ebpf/*`、`docs/audit-report.md`、`docs/Mirage 功能确认与功能验证任务清单.md`、`docs/reports/ebpf-coverage-map.md`、`artifacts/ebpf-perf/`、`deploy/scripts/drill-m7-ebpf-coverage.sh`、`docs/reports/phase4-evidence-audit.md` |
| 反取证与最小运行痕迹 | 在节点失陷、物理查扣、紧急退出场景下，尽量减少密钥和运行痕迹残留 | 1. RAM Shield、自毁/紧急擦除、密钥轮转/擦除有脚本和配置证据。 2. 若要宣称“无盘化运行”，必须有对应部署基线而不是个别模式 | RAM Shield、紧急擦除、证书擦除、tmpfs 部署资产已存在。支持默认/加固/极限隐匿三种部署等级，加固部署已有完整配置锚点（只读根、tmpfs 证书、swap 禁用、Emergency_Wipe 预装）。极限隐匿部署部分配置项当前不支持：Emergency_Wipe 自动触发需新增代码、证书 ≤24h 有效期为候选强化项需新增签发策略。当前 docker-compose.tmpfs.yml 对应加固部署等级，不自动等于极限隐匿部署。“所有部署都无盘化”“所有密钥都是极短生命周期”这类表述仍不能按普遍事实输出 | `已实现（限定表述）` | `mirage-gateway/cmd/gateway/main.go`、`deploy/scripts/emergency-wipe.sh`、`deploy/scripts/cert-rotate.sh`、`mirage-gateway/docker-compose.tmpfs.yml`、`docs/audit-report.md`、`docs/reports/deployment-tiers.md`、`docs/reports/deployment-baseline-checklist.md`、`deploy/scripts/drill-m8-baseline.sh`、`docs/reports/phase4-evidence-audit.md` |
| 准入控制与防滥用 | 在接入、认证、计费、熔断、日志治理层面形成可运营的 Zero-Trust 基线 | 1. 关键鉴权链路有回归测试。 2. Redis 鉴权、JWT/HMAC/mTLS、配额熔断、日志脱敏有发布级验证。 3. 对外宣称必须与当前运行脚本一致 | 命令鉴权、WebSocket JWT、Redis 鉴权、配额隔离与熔断、日志脱敏、发布验证脚本都已建立并有复验证据。M9 联合演练已通过：非法接入链路（HMAC/时间戳/nonce/JWT 拒绝 → 日志脱敏 → 配额不受损）和配额耗尽链路（隔离 → 精确熔断 → 日志脱敏 → AddQuota 重新激活）均验证通过。**Phase 4 M10 盘点结论**：验收标准 3 项全部达成，确认维持"已实现"状态。无差距 | `已实现` | `mirage-gateway/pkg/api/security_regression_test.go`、`mirage-os/services/ws-gateway/auth_test.go`、`deploy/docker-compose.os.yml`、`docs/Mirage 功能确认与功能验证任务清单.md`、`docs/audit-report.md`、`docs/reports/access-control-joint-drill.md`、`deploy/scripts/drill-m9-joint-drill.sh`、`docs/reports/phase4-evidence-audit.md` |

## 六、项目改造优先级

围绕北极星与现状差距，当前最值得优先推进的不是继续扩词，而是把“部分实现”项逐步打实：

1. **先闭环多承载端到端证据**
   - 重点不是再写承载类型，而是让 `QUIC -> WSS`、再到 `WebRTC / ICMP / DNS` 的真实验证链条落地。

2. **再闭环节点恢复演练**
   - 把 `DoH / Gist / Mastodon` 从“发现器存在”推进到“节点阵亡恢复已实测”。

3. **再补会话连续性证据**
   - 明确哪些业务是“传输层切换”，哪些业务能做到“业务层无感”，避免继续混写。

4. **最后补实验级隐匿结论**
   - 在没有独立实验结果之前，严禁把“有协议与扰动能力”写成“对抗 ML/DPI 已显著领先”。

## 七、与派生材料的关系

以下材料只能作为派生材料使用，不得再承担当前能力真相源职责：

| 文件 | 角色 | 使用规则 |
|------|------|----------|
| `docs/governance/market-positioning-scenarios.md` | 商业场景 / 交付叙事 | 可用于定位、路演和售前，不可单独作为当前能力证明 |
| `docs/暗网基础设施防御力评价矩阵.md` | 商业定位 / 销售矩阵 | 可作为北极星叙事输入，不可单独作为当前能力证明 |
| `docs/Mirage 功能确认与功能验证任务清单.md` | 当前发布周期验证材料 | 可作为当前版本证据，但只在该发布周期内有效 |
| `docs/audit-report.md` | 当前发布周期审计材料 | 可作为本轮修复与发布判断证据，但不是长期能力定义主源 |

## 八、变更规则

后续若要改变项目能力表述，必须遵守以下顺序：

1. 先改本文中的北极星目标、验收标准或当前真实能力
2. 再改派生材料（销售矩阵、路演稿、官网话术、发布说明）
3. 若运行时事实发生变化，优先更新对应代码锚点和真相源地图

若有新的矩阵、评分表、对比表出现，且未登记到本文与 `source-of-truth-map.md`，默认视为派生材料，不得用于项目验收或能力结论。

## 九、配套实施计划

为避免本文既承担“能力定义”又承担“执行拆解”，当前升级周期的北极星实施计划单独维护在：

- `docs/governance/capability-gap-remediation-roadmap.md`

使用顺序为：

1. 先看本文，确认目标、验收标准和当前真实能力
2. 再看实施计划，确认阶段推进、里程碑和验收证据
