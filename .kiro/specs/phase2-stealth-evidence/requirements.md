# 需求文档：Phase 2 隐匿与数据面证据闭环

## 简介

本需求文档覆盖北极星实施计划 Phase 2（隐匿与数据面证据闭环），目标是将两个能力域从"可讲故事"推进为"可拿证据"，为 `Zero-Trust SD-WAN 隐蔽数据链路` 北极星目标提供实证基础。

Phase 2 包含三个里程碑（M5-M7），覆盖两个能力域：
- 流量整形与特征隐匿（M5、M6）
- eBPF 深度参与的数据面与防护（M7）

本 spec 严格遵守 `capability-truth-source.md` 的治理规则：只安排实验设计、证据采集、验证闭环和表述边界回写，不扩展产品边界，不新增功能。

关键原则：
1. 本 spec 是关于证据采集，不是新功能开发
2. 所有实验结果必须诚实反映当前能力，不得膨胀表述
3. 若实验结果不支撑某项表述，该表述必须降级，不得伪造实验
4. 必须区分"代码存在"与"行为已验证有证据"
5. Phase 1 经验教训：只描述当前代码实际实现的行为，不写愿景性需求

## 术语表

- **Stealth_Experiment_Plan**: 隐匿实验方案文档，定义实验方法论、测试计划和对外表述边界，产出路径 `docs/reports/stealth-experiment-plan.md`
- **Experiment_Result_Report**: 最小实验结果报告，包含抓包数据、统计结果、实验结论与限制说明，产出路径 `docs/reports/stealth-experiment-results.md`
- **eBPF_Coverage_Map**: eBPF 覆盖图文档，记录 eBPF 在数据面的真实参与边界，产出路径 `docs/reports/ebpf-coverage-map.md`
- **Claims_Boundary_List**: 对外允许/不允许表述清单，基于实验证据划定表述边界，产出路径 `docs/reports/stealth-claims-boundary.md`
- **DPI_Risk_Audit_Checklist**: DPI 风险审计清单 `docs/governance/dpi-risk-audit-checklist.md`，定义各检测面的风险等级与验证动作
- **Capability_Truth_Source**: 能力真相源文档 `docs/governance/capability-truth-source.md`，定义验收标准与状态等级
- **Defense_Matrix**: 暗网基础设施防御力评价矩阵 `docs/暗网基础设施防御力评价矩阵.md`，派生商业叙事材料
- **NPM_Program**: NPM 协议 eBPF XDP 程序 `mirage-gateway/bpf/npm.c`，负责包长填充与剥离
- **BDNA_Program**: B-DNA 协议 eBPF TC 程序 `mirage-gateway/bpf/bdna.c`，负责 TCP SYN 重写、QUIC/TLS 协同标记、JA4 捕获
- **Jitter_Program**: Jitter-Lite/VPC 共享 eBPF TC 程序 `mirage-gateway/bpf/jitter.c`，负责时域扰动与背景噪声
- **Chameleon_Program**: Chameleon TLS 指纹 eBPF TC 程序 `mirage-gateway/bpf/chameleon.c`
- **L1_Defense_Program**: L1 防御 eBPF 程序 `mirage-gateway/bpf/l1_defense.c`
- **L1_Silent_Program**: L1 静默 eBPF 程序 `mirage-gateway/bpf/l1_silent.c`，TC egress 挂载
- **Sockmap_Program**: Sockmap 加速 eBPF 程序 `mirage-gateway/bpf/sockmap.c`
- **eBPF_Loader**: Go 侧 eBPF 加载器 `mirage-gateway/pkg/ebpf/loader.go`，负责加载、挂载和配置所有 eBPF 程序
- **Userspace_Path**: 用户态处理路径，指 Go 控制面中不经过 eBPF 的数据处理链路（如 G-Tunnel 分片重组、FEC、QUIC/TLS 协同重写等）

## 需求

### 需求 1：隐匿实验方案冻结（M5）

**用户故事：** 作为项目治理者，我希望在产出实验结果之前先统一实验方法论，以便确保后续实验结论可信、可复验，且对外表述有明确边界。

#### 验收标准

1. THE Stealth_Experiment_Plan SHALL 定义以下四个检测面的实验方法论：握手指纹（JA3/JA4）、包长分布、时序分布（IAT）、简单分类器可分性
2. WHEN 定义握手指纹实验方法论时，THE Stealth_Experiment_Plan SHALL 指定对照组（至少包含真实 Chrome 浏览器流量和常见 uTLS 配置流量）、采集条件（网络环境、TLS 版本、目标站点）和比较维度（JA3 hash、JA4 fingerprint、TCP options、TLS extension 顺序、QUIC transport parameters）
3. WHEN 定义包长分布实验方法论时，THE Stealth_Experiment_Plan SHALL 指定采集规模（不少于 1000 次新建连接）、统计维度（前 10 包长度、方向、上下行比例、熵值）和对照基准（目标拟态流量的同维度分布）
4. WHEN 定义时序分布实验方法论时，THE Stealth_Experiment_Plan SHALL 指定采集维度（IAT、burst 结构、时段特征）和对照条件（启用/关闭 Jitter-Lite 和 VPC 的流量样本对比）
5. WHEN 定义简单分类器实验方法论时，THE Stealth_Experiment_Plan SHALL 指定分类器类型（优先 RandomForest 和 XGBoost；若 XGBoost 环境不可用，至少输出 RandomForest 并显式标注降级）、评估指标（AUC、F1、准确率）和训练/测试集划分方式
6. THE Stealth_Experiment_Plan SHALL 明确实验环境约束：本轮实验在受控本地环境（mock 或 loopback）执行，不要求真实对抗网络环境；实验结论的证据强度标注为"受控环境基线"
7. THE Claims_Boundary_List SHALL 基于当前代码能力和 Capability_Truth_Source 的验收标准，列出对外允许表述和不允许表述两个清单
8. WHEN 当前代码能力仅有 eBPF 编译回归和本地双证据、但无独立对抗实验结果时，THE Claims_Boundary_List SHALL 将"DPI/ML 对抗效果达到 N 分"列为不允许表述
9. THE Claims_Boundary_List SHALL 将"协议主源和运行时锚点已建立"、"eBPF 编译回归通过"、"关键 .c 文件可真实编译"列为允许表述

### 需求 2：握手指纹实验（M6 — 检测面 1）

**用户故事：** 作为发布验证者，我希望产出握手指纹维度的最小实验结果，以便确认 B-DNA 的 TCP SYN 重写和 JA4 捕获链路在握手外观上的实际效果。

#### 验收标准

1. WHEN BDNA_Program 的 `bdna_tcp_rewrite` 启用且 `active_profile_map` 已加载指纹模板时，THE Experiment_Result_Report SHALL 包含 TCP SYN 包的以下字段对比数据：Window Size、MSS、Window Scale、SACK Permitted、Timestamps
2. WHEN BDNA_Program 的 `bdna_ja4_capture` 捕获到 TLS ClientHello 时，THE Experiment_Result_Report SHALL 包含简化版 JA4 指纹与对照组（真实 Chrome）的差异分析
3. THE Experiment_Result_Report SHALL 记录 BDNA_Program 的 `bdna_stats_map` 统计数据：`tcp_rewritten`、`quic_rewritten`、`tls_rewritten`、`skipped` 计数
4. THE Experiment_Result_Report SHALL 明确标注当前 B-DNA 对 QUIC Initial 和 TLS ClientHello 的处理方式为"内核标记 + 用户态协同"（`skb->mark`），不是内核完整重写
5. IF 实验结果显示 TCP SYN 重写后的指纹与目标画像仍存在可区分差异，THEN THE Experiment_Result_Report SHALL 记录具体差异字段和差异幅度，不隐瞒结果
6. THE Experiment_Result_Report SHALL 附带可复验的抓包文件路径（`artifacts/dpi-audit/handshake/*.pcapng`）和特征提取脚本路径

### 需求 3：包长分布实验（M6 — 检测面 2）

**用户故事：** 作为发布验证者，我希望产出包长维度的最小实验结果，以便确认 NPM 的 XDP 层填充在包长分布上的实际效果。

#### 验收标准

1. WHEN NPM_Program 启用且 `npm_config_map` 已配置填充参数时，THE Experiment_Result_Report SHALL 包含以下三种填充模式的包长分布数据：`NPM_MODE_FIXED_MTU`、`NPM_MODE_RANDOM_RANGE`、`NPM_MODE_GAUSSIAN`
2. THE Experiment_Result_Report SHALL 对每种填充模式统计：前 10 包长度分布、整体包长直方图、与对照组（无填充流量）的 KL 散度或 JS 散度
3. THE Experiment_Result_Report SHALL 记录 NPM_Program 的 `npm_stats_map` 统计数据：`total_packets`、`padded_packets`、`padding_bytes`、`skipped_packets`
4. WHEN `min_packet_size` 配置为非零值时，THE Experiment_Result_Report SHALL 验证小于该阈值的控制性小包确实被跳过未填充
5. IF 实验结果显示某种填充模式的包长分布仍可被简单统计方法区分，THEN THE Experiment_Result_Report SHALL 记录该模式的可区分性结论和具体统计指标
6. THE Experiment_Result_Report SHALL 附带可复验的抓包文件路径（`artifacts/dpi-audit/packet-length/*.pcapng`）和统计分析脚本路径

### 需求 4：时序分布实验（M6 — 检测面 3）

**用户故事：** 作为发布验证者，我希望产出时序维度的最小实验结果，以便确认 Jitter-Lite 和 VPC 在 IAT 与节奏控制上的实际效果。

#### 验收标准

1. WHEN Jitter_Program 的 `jitter_lite_egress` 启用时，THE Experiment_Result_Report SHALL 包含以下对比数据：启用前/后的 IAT 分布（均值、标准差、P50/P95/P99）
2. WHEN Jitter_Program 的 VPC 噪声模型启用时，THE Experiment_Result_Report SHALL 包含以下对比数据：启用前/后的延迟分布、乱序率、丢包率
3. THE Experiment_Result_Report SHALL 对比以下四种配置组合的时序特征：(a) Jitter-Lite 关闭 + VPC 关闭，(b) Jitter-Lite 开启 + VPC 关闭，(c) Jitter-Lite 关闭 + VPC 开启，(d) Jitter-Lite 开启 + VPC 开启
4. THE Experiment_Result_Report SHALL 记录 `dna_template_map` 优先读取与 `jitter_config_map` 回退读取两条链路的实际生效情况
5. IF 实验结果显示启用扰动后的时序特征仍可被简单分类器（RandomForest/XGBoost）以 AUC > 0.8 区分，THEN THE Experiment_Result_Report SHALL 记录该结论并标注为"当前扰动强度不足以抵抗简单 ML 分类"
6. THE Experiment_Result_Report SHALL 附带可复验的抓包文件路径（`artifacts/dpi-audit/timing/*.pcapng`）和时序分析脚本路径

### 需求 5：简单分类器可分性实验（M6 — 检测面 4）

**用户故事：** 作为发布验证者，我希望用简单 ML 分类器对上述三个维度的综合特征做可分性测试，以便量化当前隐匿能力的实际效果边界。

#### 验收标准

1. THE Experiment_Result_Report SHALL 使用需求 2-4 产出的特征数据，训练至少 RandomForest 分类器区分 ReMirage 流量与对照组流量（优先同时训练 XGBoost；若 XGBoost 环境不可用，至少输出 RandomForest 并显式标注"XGBoost 不可用，仅 RandomForest 结果"）
2. THE Experiment_Result_Report SHALL 报告以下指标：AUC、F1、准确率、混淆矩阵
3. THE Experiment_Result_Report SHALL 分别报告单维度（仅握手特征、仅包长特征、仅时序特征）和多维度联合的分类结果
4. IF 任一维度的单独 AUC > 0.9，THEN THE Experiment_Result_Report SHALL 将该维度标注为"高可区分性风险"并建议后续优先改进方向
5. THE Experiment_Result_Report SHALL 明确标注实验限制：受控环境、样本规模、分类器复杂度，不将结论外推为"可抵抗生产级 DPI/ML 系统"
6. THE Experiment_Result_Report SHALL 附带可复验的训练脚本路径和模型评估脚本路径（`artifacts/dpi-audit/classifier/`）

### 需求 6：实验结论与 Defense_Matrix 回写（M6 收口）

**用户故事：** 作为项目治理者，我希望将实验结论诚实回写到治理文档和派生材料，以便确保对外表述与实际证据一致。

#### 验收标准

1. WHEN 所有四个检测面实验完成后，THE Experiment_Result_Report SHALL 产出一份综合结论，包含：各检测面的效果评级（有效/部分有效/无效）、当前隐匿能力的整体边界描述、明确的限制说明
2. THE Claims_Boundary_List SHALL 根据实验结果更新允许/不允许表述清单：若实验证明某维度有效，该维度的限定表述可升级为允许；若实验证明某维度无效或部分有效，对应的满额表述维持不允许
3. WHEN 实验结论产出后，THE Defense_Matrix SHALL 回写"特征隐匿极限"维度的评分说明，将当前评分依据从"概念描述"更新为"受控环境实验结论"，并标注证据强度等级
4. WHEN 实验结论产出后，THE Capability_Truth_Source SHALL 回写"流量整形与特征隐匿"能力域的"当前真实能力"描述，增加实验证据锚点链接
5. IF 实验结果不支撑 Defense_Matrix 中"特征隐匿极限"的当前评分，THEN THE Defense_Matrix SHALL 降低该维度评分或增加限定说明，不维持无证据支撑的满额评分

### 需求 7：eBPF 覆盖图文档（M7 — 覆盖边界）

**用户故事：** 作为项目治理者，我希望产出一份 eBPF 覆盖图，明确 eBPF 在数据面的真实参与边界，以便统一"eBPF 深度参与"的表述口径，避免把"深度参与"误写成"全链路零拷贝"。

#### 验收标准

1. THE eBPF_Coverage_Map SHALL 列出当前所有 eBPF 程序及其挂载点，区分三个层次：源码定义（source-defined）、编译产物（compiled）、运行态挂载（loader-attached）。运行态挂载以 loader.go 为唯一真相源。当前 loader 实际挂载的程序包括：NPM_Program（XDP）、BDNA_Program（TC egress + TC ingress）、Jitter_Program（TC egress + TC ingress）、Chameleon_Program（TC egress）、L1_Silent_Program（TC egress）、Sockmap_Program（sockops/sk_msg），以及 Phantom、H3_Shaper、ICMP_Tunnel 等非 critical 程序
2. THE eBPF_Coverage_Map SHALL 为每个 eBPF 程序标注以下属性：挂载类型（XDP/TC/sockops/sk_msg）、处理方向（ingress/egress）、处理的协议层（L2/L3/L4/L7）、是否需要用户态协同（独立完成/需要 `skb->mark` 协同）
3. THE eBPF_Coverage_Map SHALL 明确标注以下数据路径为 Userspace_Path（不经过 eBPF）：G-Tunnel 分片与重组、FEC 编解码、QUIC 握手与传输参数协商、TLS ClientHello 完整重写、HTTP/2 SETTINGS 与请求序列、G-Switch 域名转生逻辑
4. THE eBPF_Coverage_Map SHALL 产出一份"eBPF 参与 vs 用户态处理"的路径对照表，标注每条关键数据路径的处理位置（eBPF 内核态 / Go 用户态 / 混合协同）
5. THE eBPF_Coverage_Map SHALL 基于路径对照表计算 eBPF 参与度的定性结论：哪些路径是"eBPF 独立完成"、哪些是"eBPF 标记 + 用户态完成"、哪些是"纯用户态"
6. IF 路径对照表显示用户态处理路径占比显著，THEN THE eBPF_Coverage_Map SHALL 明确结论为"eBPF 深度参与关键路径，但非全链路零拷贝"，不使用"全流量全链路零拷贝"表述

### 需求 8：关键路径性能证据（M7 — 性能基线）

**用户故事：** 作为发布验证者，我希望为 eBPF 关键路径产出最小性能观测数据，以便为"eBPF 深度参与"的表述提供量化证据支撑。

#### 验收标准

1. THE eBPF_Coverage_Map SHALL 包含以下 eBPF 程序的性能观测数据：NPM_Program（XDP 处理延迟）、BDNA_Program（TC 处理延迟）、Jitter_Program（TC 处理延迟）
2. WHEN 使用 `benchmarks/ebpf_latency.bt` 或等效 bpftrace 脚本采集延迟数据时，THE eBPF_Coverage_Map SHALL 记录 XDP 和 TC 程序的 P50、P95、P99 延迟值
3. THE eBPF_Coverage_Map SHALL 将观测到的延迟值与 `protocol-language-rules.md` 中的性能要求（C 数据面延迟 < 1ms、CPU < 5%、内存 < 50MB）进行对照
4. THE eBPF_Coverage_Map SHALL 明确标注性能数据的采集环境（内核版本、CPU 型号、网络负载条件）和证据强度等级（受控环境基线 / 生产环境观测）
5. IF 性能观测数据显示某个 eBPF 程序的 P99 延迟超过 1ms、CPU 占用超过 5% 或 Map 内存占用超过 50MB，THEN THE eBPF_Coverage_Map SHALL 记录该异常并分析可能原因，不隐瞒结果
6. THE eBPF_Coverage_Map SHALL 附带性能观测的原始数据路径（`artifacts/ebpf-perf/`）和采集脚本路径

### 需求 9：eBPF 对外表述统一与治理回写（M7 收口）

**用户故事：** 作为项目治理者，我希望基于覆盖图和性能证据统一 eBPF 的对外表述，并回写治理文档，以便确保"eBPF 深度参与"的表述有证据支撑且边界清晰。

#### 验收标准

1. WHEN eBPF_Coverage_Map 和性能证据完成后，THE Claims_Boundary_List SHALL 增加 eBPF 相关的允许/不允许表述：允许"eBPF 深度参与关键路径（XDP 包长控制、TC 指纹重写、TC 时域扰动、sockmap 加速）"；不允许"所有流量全链路零拷贝"（除非覆盖图证明用户态路径占比可忽略）
2. WHEN eBPF_Coverage_Map 完成后，THE Capability_Truth_Source SHALL 回写"eBPF 深度参与的数据面与防护"能力域的"当前真实能力"描述，增加覆盖图和性能证据的锚点链接
3. WHEN 性能证据满足 `protocol-language-rules.md` 的性能要求时，THE Capability_Truth_Source SHALL 评估该能力域是否满足从"已实现（限定表述）"维持或升级的条件
4. THE Defense_Matrix SHALL 回写"协议栈处理延迟"维度的评分说明，将当前评分依据从"架构描述"更新为"受控环境性能观测"，并标注证据强度等级
5. IF Defense_Matrix 中"协议栈处理延迟"的当前评分与性能观测数据不一致，THEN THE Defense_Matrix SHALL 调整评分或增加限定说明

### 需求 10：Phase 2 证据沉淀与出关判定

**用户故事：** 作为项目治理者，我希望 Phase 2 的所有证据按治理规则沉淀，并完成出关判定，以便支撑后续阶段推进和北极星升级评估。

#### 验收标准

1. WHEN M5 完成后，THE 功能验证清单 SHALL 新增"隐匿实验方案冻结"条目，包含 Stealth_Experiment_Plan 和 Claims_Boundary_List 的文件路径
2. WHEN M6 完成后，THE 功能验证清单 SHALL 新增"隐匿实验结果"条目，包含 Experiment_Result_Report 的文件路径、抓包证据路径和分析脚本路径
3. WHEN M7 完成后，THE 功能验证清单 SHALL 新增"eBPF 覆盖图与性能证据"条目，包含 eBPF_Coverage_Map 的文件路径和性能数据路径
4. WHEN Phase 2 全部里程碑完成后，THE Capability_Truth_Source SHALL 根据实际证据评估以下两个能力域是否满足状态变更条件："流量整形与特征隐匿"（当前"部分实现"）和"eBPF 深度参与的数据面与防护"（当前"已实现（限定表述）"）
5. WHEN Phase 2 出关判定完成后，THE Defense_Matrix 和 Capability_Truth_Source 中不得再出现超出实验证据边界的隐匿效果表述
6. IF Phase 2 实验结果不足以支撑能力域状态升级，THEN THE Capability_Truth_Source SHALL 维持当前状态不变，并在主证据锚点中记录实验结论和差距说明
