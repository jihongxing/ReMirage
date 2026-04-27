# 实施计划：Phase 2 隐匿与数据面证据闭环

## 概述

按 M5→M6→M7→证据 四个里程碑递进实施。本 spec 不新增功能，只做实验设计、证据采集、PBT 验证和治理回写。

关键约束：
- 所有实验在受控本地环境（loopback/mock）执行，证据强度标注为"受控环境基线"
- 非 Linux 环境降级为 mock/simulation，标注"模拟环境参考"
- PBT 使用 `pgregory.net/rapid`（Go）和 `hypothesis`（Python），最少 100 次迭代
- eBPF PBT 采用 Mock 方式（Go 等价实现）作为主路径，Map 驱动作为集成验证
- 所有 PBT 子任务为必须项（Phase 1 经验教训）
- 证据产物按固定路径沉淀到 `artifacts/` 和 `deploy/evidence/`
- Drill 脚本必须包含完整步骤、证据捕获和报告生成（不只是 `go test` 包装）

## 任务

- [x] 1. M5：隐匿实验方案冻结
  - [x] 1.1 创建 `docs/reports/stealth-experiment-plan.md`
    - 定义四个检测面的实验方法论：握手指纹（JA3/JA4）、包长分布、时序分布（IAT）、简单分类器可分性
    - 握手指纹：对照组（真实 Chrome + 常见 uTLS）、采集条件（loopback、TLS 1.3）、比较维度（JA3 hash、JA4 fingerprint、TCP options、TLS extension 顺序、QUIC transport parameters）
    - 包长分布：采集规模 ≥1000 次连接、统计维度（前 10 包长度、方向、上下行比例、熵值）、对照基准（无填充流量）
    - 时序分布：采集维度（IAT、burst 结构）、对照条件（Jitter±VPC 的 2×2 矩阵）
    - 简单分类器：RandomForest + XGBoost、评估指标（AUC、F1、准确率）、训练/测试集划分
    - 明确标注实验环境约束：受控本地环境，证据强度"受控环境基线"
    - _需求: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6_

  - [x] 1.2 创建 `docs/reports/stealth-claims-boundary.md`（初始版本）
    - 基于当前代码能力和 capability-truth-source.md 验收标准，列出允许/不允许表述两个清单
    - 允许表述：协议主源和运行时锚点已建立、eBPF 编译回归通过、关键 .c 文件可真实编译
    - 不允许表述：DPI/ML 对抗效果达到 N 分（无独立实验结果）
    - 此文件在 M6/M7 完成后会根据实验结果更新
    - _需求: 1.7, 1.8, 1.9_

  - [x] 1.3 创建 `deploy/scripts/drill-m5-experiment-plan.sh`
    - M5 验证脚本：检查 stealth-experiment-plan.md 和 stealth-claims-boundary.md 是否存在且包含必要章节
    - 验证四个检测面方法论完整性（grep 关键章节标题）
    - 验证 claims-boundary 包含允许/不允许两个清单
    - 捕获验证日志到 `deploy/evidence/m5-experiment-plan-drill.log`
    - _需求: 1.1, 1.7_

- [x] 2. Checkpoint — M5 冻结确认
  - 确认实验方案和表述边界文档已创建，请用户确认是否有问题。


- [x] 3. M6：抓包样本生成编排
  - [x] 3.1 创建 `artifacts/dpi-audit/generate-samples.sh`
    - 抓包样本生成编排脚本（需要 Linux + root/CAP_NET_RAW）
    - 步骤：① 加载 eBPF 程序（通过 Go 测试 helper 或独立加载器）② 启动 tcpdump 抓包（loopback）③ 注入测试流量（TCP SYN / QUIC / TLS / HTTP 混合）④ 停止抓包 ⑤ 按检测面分组命名 pcapng 文件
    - 握手指纹组：生成 `artifacts/dpi-audit/handshake/remirage-syn.pcapng`（B-DNA 启用）、`chrome-syn.pcapng`（对照组，需预采集或使用公开数据集）、`utls-syn.pcapng`（uTLS 配置流量，本地使用 Go uTLS 库生成或从公开数据集获取，来源在 README 中标注）
    - 包长分布组：生成 `artifacts/dpi-audit/packet-length/mode-fixed-mtu.pcapng`、`mode-random-range.pcapng`、`mode-gaussian.pcapng`、`baseline-no-padding.pcapng`
    - 时序分布组：生成 `artifacts/dpi-audit/timing/config-none.pcapng`、`config-jitter-only.pcapng`、`config-vpc-only.pcapng`、`config-jitter-vpc.pcapng`
    - 非 Linux 环境：跳过真实抓包，标注"pcap 样本缺失，分析脚本使用模拟数据"
    - _需求: 2.6, 3.6, 4.6_

- [x] 4. M6：握手指纹实验（检测面 1）
  - [x] 4.1 创建 `artifacts/dpi-audit/handshake/extract-features.py`
    - Python 脚本：从 pcapng 文件提取握手指纹特征
    - 提取维度：TCP Window Size、MSS、WScale、SACK、Timestamps、JA4 fingerprint、TLS extension 顺序
    - 输入：pcapng 文件路径；输出：comparison.csv
    - 依赖：scapy 或 pyshark
    - _需求: 2.1, 2.2, 2.6_

  - [x] 4.2 扩展 `mirage-gateway/pkg/ebpf/bdna_conn_property_test.go`（复用现有文件）
    - 现有文件已包含 `TestProperty_BDNANonSYNWindowConsistency`
    - 新增 B-DNA TCP SYN 重写匹配和统计一致性的 PBT
    - 编写 example 测试：验证 TCP SYN 重写后字段与模板匹配、非 SYN 包不触发重写、无模板时 skipped 递增
    - _需求: 2.1, 2.3, 2.4, 2.5_

  - [x] 4.3 编写 Property 3: B-DNA TCP SYN 重写匹配 PBT
    - **Property 3: B-DNA TCP SYN 重写匹配**
    - 测试函数: `TestProperty_BDNATCPSYNRewriteMatch`
    - 文件: `mirage-gateway/pkg/ebpf/bdna_conn_property_test.go`（复用现有文件）
    - 使用 `rapid` 生成随机 `stack_fingerprint`（tcp_window ∈ [1,65535], tcp_mss ∈ [536,1460], tcp_wscale ∈ [0,14]）
    - 验证：重写后 TCP SYN 包的 Window Size / MSS / WScale 与模板值完全匹配
    - **验证: 需求 2.1**

  - [x] 4.4 编写 Property 4: B-DNA 统计一致性 PBT
    - **Property 4: B-DNA 统计一致性**
    - 测试函数: `TestProperty_BDNAStatsConsistency`
    - 文件: `mirage-gateway/pkg/ebpf/bdna_conn_property_test.go`（复用现有文件）
    - 使用 `rapid` 生成随机包序列（TCP SYN / QUIC Initial / TLS ClientHello / 其他），验证：
      - TCP SYN 包：tcp_rewritten 或 skipped 递增
      - QUIC Initial 包：quic_rewritten 递增
      - TLS ClientHello 包：tls_rewritten 递增
      - 非匹配包：计数器不变
    - **验证: 需求 2.3**

- [x] 5. M6：包长分布实验（检测面 2）
  - [x] 5.1 创建 `artifacts/dpi-audit/packet-length/analyze-distribution.py`
    - Python 脚本：从 pcapng 文件分析包长分布
    - 统计维度：前 10 包长度分布、整体包长直方图、KL 散度、JS 散度
    - 输入：多个 pcapng 文件（各填充模式 + 对照组）；输出：distributions.csv + 直方图
    - _需求: 3.2, 3.5, 3.6_

  - [x] 5.2 扩展 `mirage-gateway/pkg/ebpf/npm_property_test.go`（复用现有文件）
    - 现有文件已包含 `TestProperty_NPMModeCorrection`
    - 新增 NPM 填充正确性和统计一致性的 PBT
    - 编写 example 测试：验证三种模式的填充行为、小包跳过、大包不填充
    - _需求: 3.1, 3.3, 3.4_

  - [x] 5.3 编写 Property 1: NPM 填充正确性 PBT
    - **Property 1: NPM 填充正确性**
    - 测试函数: `TestProperty_NPMPaddingCorrectness`
    - 文件: `mirage-gateway/pkg/ebpf/npm_property_test.go`（复用现有文件）
    - 使用 `rapid` 生成随机 `npm_config`（enabled=1, padding_mode ∈ {0,1,2}, global_mtu ∈ [500,1500], min_packet_size ∈ [0,256]）和随机 current_size ∈ [0,1600]
    - 验证：
      - current_size < min_packet_size → 不填充
      - current_size >= target_mtu → padding = 0
      - FIXED_MTU: padding = target_mtu - current_size
      - RANDOM_RANGE: 0 < padding ≤ target_mtu - current_size
      - GAUSSIAN: padding ∈ [0, target_mtu - current_size]
      - padding > 0 时：MIN_PADDING_SIZE(64) ≤ padding ≤ MAX_PADDING_SIZE(1400)
    - **验证: 需求 3.1, 3.4**

  - [x] 5.4 编写 Property 2: NPM 统计一致性 PBT
    - **Property 2: NPM 统计一致性**
    - 测试函数: `TestProperty_NPMStatsConsistency`
    - 文件: `mirage-gateway/pkg/ebpf/npm_property_test.go`（复用现有文件）
    - 使用 `rapid` 生成随机包序列（大小 ∈ [0,1600]，数量 ∈ [1,200]），经过 `handle_npm_padding` Mock 处理后验证：
      - padded_packets + skipped_packets ≤ total_packets
      - padding_bytes > 0 当且仅当 padded_packets > 0
      - padded_packets ≤ total_packets - skipped_packets
    - **验证: 需求 3.3**


- [x] 6. M6：时序分布实验（检测面 3）
  - [x] 6.1 创建 `artifacts/dpi-audit/timing/analyze-timing.py`
    - Python 脚本：从 pcapng 文件分析 IAT 分布
    - 统计维度：IAT 均值、标准差、P50/P95/P99、burst 结构
    - 输入：四种配置组合的 pcapng 文件；输出：iat-stats.csv
    - _需求: 4.1, 4.2, 4.3, 4.6_

  - [x] 6.2 扩展 `mirage-gateway/pkg/ebpf/gaussian_property_test.go` 和 `mirage-gateway/pkg/ebpf/vpc_property_test.go`（复用现有文件）
    - gaussian_property_test.go 已包含 `TestProperty_IrwinHallGaussianStatistics`
    - vpc_property_test.go 已包含 `TestProperty_VPCFiberJitterExponentialDistribution` 和 `TestProperty_VPCSubmarineCableNonPeriodic`
    - 新增 Jitter IAT 方差增加和配置优先级的 PBT
    - 编写 example 测试：验证模板优先级、回退行为、IAT 方差变化
    - _需求: 4.1, 4.4_

  - [x] 6.3 编写 Property 5: Jitter IAT 方差增加 PBT
    - **Property 5: Jitter IAT 方差增加**
    - 测试函数: `TestProperty_JitterIATVarianceIncrease`
    - 文件: `mirage-gateway/pkg/ebpf/gaussian_property_test.go`（复用现有文件）
    - 使用 `rapid` 生成随机 `jitter_config`（enabled=1, mean_iat_us ∈ [100,10000], stddev_iat_us ∈ [10,2000]）
    - 生成 N=200 个包的 IAT 序列，比较启用/关闭 jitter 的方差
    - 验证：启用后 IAT 方差 > 关闭时 IAT 方差
    - **验证: 需求 4.1**

  - [x] 6.4 编写 Property 6: Jitter 配置优先级 PBT
    - **Property 6: Jitter 配置优先级**
    - 测试函数: `TestProperty_JitterConfigPriority`
    - 文件: `mirage-gateway/pkg/ebpf/gaussian_property_test.go`（复用现有文件）
    - 使用 `rapid` 生成随机 `dna_template`（target_iat_mu ∈ [100,10000], target_iat_sigma ∈ [10,2000]）和随机 `jitter_config`
    - 场景 A：dna_template_map 有模板 → 验证使用 get_mimic_delay 路径，delay_ns > 0
    - 场景 B：dna_template_map 无模板 → 验证回退到 gaussian_sample 路径，delay_ns > 0
    - **验证: 需求 4.4**

- [x] 7. M6：简单分类器实验（检测面 4）
  - [x] 7.1 创建 `artifacts/dpi-audit/classifier/train-classifier.py`
    - Python 脚本：训练 RandomForest + XGBoost 分类器
    - 依赖声明：`artifacts/dpi-audit/classifier/requirements.txt`（scikit-learn, xgboost, pandas, numpy）
    - XGBoost 降级策略：若 `import xgboost` 失败，降级为仅使用 RandomForest，在结果中标注"XGBoost 不可用，仅 RandomForest 结果"
    - 输入：检测面 1-3 的特征数据（features.csv）
    - 输出：results.json（AUC、F1、准确率、混淆矩阵）
    - 分别报告单维度（仅握手 / 仅包长 / 仅时序）和多维度联合分类结果
    - 单维度 AUC > 0.9 时标注"高可区分性风险"
    - 明确标注实验限制：受控环境、样本规模、分类器复杂度
    - _需求: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_

- [x] 8. M6 收口：实验结论与治理回写
  - [x] 8.1 创建 `docs/reports/stealth-experiment-results.md`
    - 综合四个检测面的实验结果
    - 各检测面效果评级（有效/部分有效/无效）
    - 当前隐匿能力整体边界描述
    - 明确限制说明：受控环境基线，不外推为"可抵抗生产级 DPI/ML 系统"
    - 附带所有抓包文件路径和分析脚本路径
    - _需求: 2.5, 2.6, 3.5, 3.6, 4.5, 4.6, 5.5, 6.1_

  - [x] 8.2 更新 `docs/reports/stealth-claims-boundary.md`
    - 根据实验结果更新允许/不允许表述清单
    - 若实验证明某维度有效 → 限定表述可升级为允许
    - 若实验证明某维度无效或部分有效 → 满额表述维持不允许
    - _需求: 6.2_

  - [x] 8.3 回写 `docs/暗网基础设施防御力评价矩阵.md`
    - 回写"特征隐匿极限"维度评分说明
    - 将评分依据从"概念描述"更新为"受控环境实验结论"
    - 标注证据强度等级
    - 若实验结果不支撑当前评分 → 降低评分或增加限定说明
    - _需求: 6.3, 6.5_

  - [x] 8.4 回写 `docs/governance/capability-truth-source.md`（M6 部分）
    - 回写"流量整形与特征隐匿"能力域的"当前真实能力"描述
    - 增加实验证据锚点链接：`docs/reports/stealth-experiment-results.md`、`artifacts/dpi-audit/`
    - _需求: 6.4_

  - [x] 8.5 创建 `deploy/scripts/drill-m6-experiment.sh`
    - M6 实验执行编排脚本
    - 步骤：① 检查环境（Linux/clang/Python/tcpdump/root 或 CAP_NET_RAW/pip 依赖含 scikit-learn 和可选 xgboost）② 执行抓包样本生成（`bash artifacts/dpi-audit/generate-samples.sh`）③ 执行 eBPF PBT（`go test -run TestProperty_NPM -v` + `go test -run TestProperty_BDNA -v` + `go test -run TestProperty_Jitter -v`）④ 运行分析脚本（extract-features.py、analyze-distribution.py、analyze-timing.py）⑤ 运行分类器训练（train-classifier.py）⑥ 生成实验结果摘要
    - 非 Linux 环境：跳过真实 eBPF 实验和抓包，仅执行 Mock PBT，标注降级
    - 捕获日志到 `deploy/evidence/m6-experiment-drill.log`
    - 脚本退出码反映测试结果
    - _需求: 2.6, 3.6, 4.6, 5.6_

- [x] 9. Checkpoint — M6 实验结果确认
  - 确认四个检测面实验结果和治理回写已完成，请用户确认是否有问题。


- [x] 10. M7：eBPF 覆盖图文档
  - [x] 10.1 创建 `docs/reports/ebpf-coverage-map.md`
    - 列出所有 eBPF 程序，区分三个层次：源码定义（source-defined）/ 编译产物（compiled）/ 运行态挂载（loader-attached），以 loader.go 为运行态真相源
    - 运行态挂载程序：NPM(XDP)、BDNA(TC egress+ingress)、Jitter(TC egress+ingress)、Chameleon(TC egress)、L1_Silent(TC egress)、Sockmap(sockops/sk_msg)
    - 源码存在但 loader 未挂载的函数单独列出（bdna_tls_rewrite、jitter_lite_adaptive/physical/social、emergency_wipe 等）
    - 每个程序标注：挂载类型、处理方向（ingress/egress）、协议层（L2-L7）、用户态协同方式
    - 明确标注用户态处理路径（不经过 eBPF）：G-Tunnel 分片重组、FEC 编解码、QUIC 握手、TLS ClientHello 完整重写、HTTP/2 SETTINGS、G-Switch 域名转生
    - 产出"eBPF 参与 vs 用户态处理"路径对照表
    - 计算 eBPF 参与度定性结论：独立完成 / 标记+用户态 / 纯用户态
    - 若用户态路径占比显著 → 结论为"eBPF 深度参与关键路径，但非全链路零拷贝"
    - _需求: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6_

  - [x] 10.2 补充 `docs/reports/ebpf-coverage-map.md` 性能证据章节
    - 记录 NPM(XDP)、BDNA(TC)、Jitter(TC) 的 P50/P95/P99 延迟值
    - 记录 eBPF 程序 CPU 占用百分比（perf top / mpstat 采集）
    - 记录 eBPF Map 内存占用（bpftool map show 采集）
    - 采集工具：延迟用 `benchmarks/ebpf_latency.bt`，CPU 用 `perf top`，内存用 `bpftool map show`
    - 与 protocol-language-rules.md 性能要求对照（C 数据面延迟 < 1ms、CPU < 5%、内存 < 50MB）
    - 标注采集环境（内核版本、CPU 型号、网络负载）和证据强度等级
    - 若 P99 > 1ms 或 CPU > 5% 或内存 > 50MB → 记录异常并分析原因
    - 附带原始数据路径 `artifacts/ebpf-perf/`（latency-report.txt、cpu-report.txt、memory-report.txt）和采集脚本路径
    - _需求: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

- [x] 11. M7 收口：eBPF 表述统一与治理回写
  - [x] 11.1 更新 `docs/reports/stealth-claims-boundary.md`（eBPF 部分）
    - 增加 eBPF 相关允许表述："eBPF 深度参与关键路径（XDP 包长控制、TC 指纹重写、TC 时域扰动、sockmap 加速）"
    - 增加 eBPF 相关不允许表述："所有流量全链路零拷贝"（除非覆盖图证明用户态路径占比可忽略）
    - _需求: 9.1_

  - [x] 11.2 回写 `docs/governance/capability-truth-source.md`（M7 部分）
    - 回写"eBPF 深度参与的数据面与防护"能力域的"当前真实能力"描述
    - 增加覆盖图和性能证据锚点链接：`docs/reports/ebpf-coverage-map.md`、`artifacts/ebpf-perf/`
    - 评估是否满足从"已实现（限定表述）"维持或升级的条件
    - _需求: 9.2, 9.3_

  - [x] 11.3 回写 `docs/暗网基础设施防御力评价矩阵.md`（延迟维度）
    - 回写"协议栈处理延迟"维度评分说明
    - 将评分依据从"架构描述"更新为"受控环境性能观测"
    - 标注证据强度等级
    - 若性能观测数据与当前评分不一致 → 调整评分或增加限定说明
    - _需求: 9.4, 9.5_

  - [x] 11.4 创建 `deploy/scripts/drill-m7-ebpf-coverage.sh`
    - M7 覆盖图与性能验证脚本
    - 步骤：① 检查环境（Linux/clang/bpftrace/perf/bpftool）② 执行 eBPF 编译回归（`go test -run TestBPFCompile_KeyCFiles`）③ 若有 bpftrace → 执行 `benchmarks/ebpf_latency.bt` 采集延迟数据，保存到 `artifacts/ebpf-perf/latency-report.txt` ④ 若有 perf/mpstat → 采集 CPU 占用数据，保存到 `artifacts/ebpf-perf/cpu-report.txt` ⑤ 若有 bpftool → 执行 `bpftool map show` 采集 Map 内存占用，保存到 `artifacts/ebpf-perf/memory-report.txt` ⑥ 验证 ebpf-coverage-map.md 存在且包含必要章节 ⑦ 生成覆盖图验证摘要
    - 非 Linux 环境：跳过真实编译和性能采集，标注降级
    - 缺少 perf/bpftool 时：跳过对应采集步骤，在日志中标注"CPU/内存数据缺失"
    - 捕获日志到 `deploy/evidence/m7-ebpf-coverage-drill.log`
    - _需求: 8.6, 9.1_

- [x] 12. Checkpoint — M7 覆盖图与性能证据确认
  - 确认 eBPF 覆盖图、性能证据和治理回写已完成，请用户确认是否有问题。

- [x] 13. 证据沉淀：验证清单与治理回写
  - [x] 13.1 回写 `docs/Mirage 功能确认与功能验证任务清单.md`
    - 新增"隐匿实验方案冻结"条目：复验命令指向 `drill-m5-experiment-plan.sh`、证据文件路径 `docs/reports/stealth-experiment-plan.md` + `docs/reports/stealth-claims-boundary.md`
    - 新增"隐匿实验结果"条目：复验命令指向 `drill-m6-experiment.sh`、证据文件路径 `docs/reports/stealth-experiment-results.md` + `artifacts/dpi-audit/`
    - 新增"eBPF 覆盖图与性能证据"条目：复验命令指向 `drill-m7-ebpf-coverage.sh`、证据文件路径 `docs/reports/ebpf-coverage-map.md` + `artifacts/ebpf-perf/`
    - _需求: 10.1, 10.2, 10.3_

  - [x] 13.2 回写 `docs/governance/capability-truth-source.md`（出关判定）
    - 根据 M6/M7 实际证据评估两个能力域状态变更条件：
      - "流量整形与特征隐匿"（当前"部分实现"）→ 是否可升级
      - "eBPF 深度参与的数据面与防护"（当前"已实现（限定表述）"）→ 是否可维持或升级
    - 若证据不足以升级 → 维持当前状态，记录实验结论和差距说明
    - 确保不出现超出实验证据边界的隐匿效果表述
    - _需求: 10.4, 10.5, 10.6_

  - [x] 13.3 创建 `deploy/evidence/README.md`（Phase 2 部分）
    - 说明 Phase 2 证据目录结构、各日志文件用途、各报告文件用途、复验命令
    - 标注证据强度等级：受控环境基线（Linux）/ 模拟环境参考（非 Linux）
    - _需求: 10.1, 10.2, 10.3_

- [x] 14. 最终 Checkpoint — Phase 2 出关确认
  - 确认所有里程碑完成、治理回写一致、证据沉淀完整，请用户确认是否有问题。

## 备注

- 所有 PBT 子任务为必须项（非可选），Phase 1 经验教训
- PBT 使用 `pgregory.net/rapid`（Go），最少 100 次迭代
- eBPF PBT 采用 Mock 方式（Go 等价实现 C 逻辑）作为主路径
- 每个 PBT 任务标注对应 design property 编号和验证的需求编号
- PBT 复用现有测试文件：`npm_property_test.go`、`bdna_conn_property_test.go`、`gaussian_property_test.go`、`vpc_property_test.go`，不另建平行测试体系
- M7 覆盖图区分三个层次：source-defined / compiled / loader-attached，以 loader.go 为运行态真相源
- L1_Silent 挂载类型为 TC egress（非 XDP），以 l1_silent.c SEC("tc") 和 loader.go attachL1Silent 为准
- 分类器依赖声明：`artifacts/dpi-audit/classifier/requirements.txt`；XGBoost 不可用时降级为仅 RandomForest + 标注
- 抓包样本生成由 `artifacts/dpi-audit/generate-samples.sh` 统一编排，需 Linux + root/CAP_NET_RAW
- 性能证据覆盖延迟（bpftrace）、CPU（perf top）、内存（bpftool map show）三个维度
- Drill 脚本包含完整步骤：环境检查 → 测试执行 → 证据捕获 → 报告生成
- 证据产物固定路径：`artifacts/dpi-audit/`（实验数据）、`artifacts/ebpf-perf/`（性能数据）、`deploy/evidence/`（drill 日志）
- 非 Linux 环境所有 eBPF 相关任务降级为 mock/simulation，标注"模拟环境参考"
- 术语一致性：stealth-experiment-plan / stealth-claims-boundary / stealth-experiment-results / ebpf-coverage-map 四个文档名称在所有引用中保持一致
