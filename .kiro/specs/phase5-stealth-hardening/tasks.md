# 实施计划：Phase 5 流量隐匿加固

## 概述

按 M13→M14→M15→M16 四个里程碑递进实施。本 Spec 修复已识别的隐匿缺陷，采集真实对照基线，重跑分类器实验。

关键约束：
- M13 需要在对应原生 OS 上采集对照数据（Chrome-Win 在 Windows、Chrome-macOS 在 macOS、Firefox-Linux 在 Linux），不允许跨 OS 替代
- M14 的 eBPF 改动遵守 protocol-language-rules.md（C 做数据面，Go 做控制面，eBPF Map 通信）
- PBT 使用 `pgregory.net/rapid`，最少 100 次迭代
- 能力域状态升级条件：M14 完成 + M13 真实基线（非降级） + M15 分类器 AUC 达标（单维 < 0.75 + 联合 < 0.85），三者缺一不可
- Phase 出关（implementation exit）与能力状态升级（capability-upgrade gate）分离：Phase 可以降级出关，但降级/模拟数据不可作为能力状态升级依据
- 非 Linux 环境：M13 跳过真实采集（标注降级），M14 PBT 使用 Mock，M15 使用模拟数据标注"校准后模拟"

## 任务

- [ ] 1. M13：真实对照基线采集
  - [ ] 1.1 创建 `artifacts/dpi-audit/baseline/capture-baseline.sh`
    - 自动化采集脚本：启动 tcpdump → 用本机浏览器访问目标站点列表 → 停止抓包 → 按画像族分组
    - 目标站点：google.com、youtube.com、cloudflare.com、github.com、wikipedia.org（覆盖 CDN/直连/混合场景）
    - 画像族必须在对应原生 OS 上采集：
      - Chrome-Win：在 Windows 节点运行，使用 Windows 原生 Chrome
      - Chrome-macOS：在 macOS 节点运行，使用 macOS 原生 Chrome
      - Firefox-Linux：在 Linux 节点运行，使用 Linux 原生 Firefox
    - 不允许用 Linux 浏览器数据代表 Windows/macOS 画像族（TCP 栈、TLS 库、OS 指纹不同）
    - 若某 OS 节点不可用，该画像族标注"待采集"，不用其他 OS 替代
    - 每族至少 100 条独立 HTTPS 连接
    - 输出：`artifacts/dpi-audit/baseline/{chrome-win,chrome-macos,firefox-linux}/*.pcapng`
    - _需求: 1.1, 1.2_

  - [ ] 1.2 创建 `artifacts/dpi-audit/baseline/extract-baseline-stats.py`
    - 从真实 pcapng 提取统计数据：
      - 握手指纹：tcp_window / tcp_mss / tcp_wscale / tcp_sack / tcp_timestamps / tls_ext_count / tls_ext_order / JA4
      - 包长分布：前 10 包长度+方向、整体直方图（256 bin）、均值/标准差/熵值/上下行比例
      - IAT 分布：均值/标准差/P50/P95/P99/burst 结构
    - 输出：`artifacts/dpi-audit/baseline/baseline-stats.csv` + `baseline-distribution.json`（包长 CDF）
    - _需求: 1.1, 1.3_

  - [ ] 1.3 校验画像库一致性
    - 将真实采集的 TCP/TLS 指纹与 `configs/bdna/fingerprints.yaml` 和 `configs/bdna/profile-registry.v1.json` 逐字段比对
    - 发现偏差时更新画像库数据（tcp_window / tcp_mss / tcp_wscale / tls_ext_order 等）
    - 记录偏差清单到 `docs/reports/stealth-experiment-results.md` 的"画像库校准"章节
    - _需求: 1.4, 2.5_

  - [ ] 1.4 备份模拟数据并替换
    - 将现有 `artifacts/dpi-audit/{handshake,packet-length,timing}/*.pcapng` 重命名为 `*.simulated.pcapng`
    - 用真实采集数据替换（保持文件名不变，分析脚本无需修改）
    - 真实采集元数据存储为 `artifacts/dpi-audit/baseline/capture-metadata.json`（内核版本、浏览器版本、OS 版本、网络条件）
    - `simulation-metadata.json` 保持不变，仅用于模拟样本；两套元数据文件语义隔离
    - _需求: 1.3, 1.4_

- [ ] 2. Checkpoint — M13 基线采集确认
  - 确认真实对照基线已采集，画像库偏差已记录，请用户确认是否继续。

- [ ] 3. M14：隐匿缺陷修复
  - [ ] 3.1 B-DNA per-connection 画像选择 — C 侧
    - 在 `bdna.c` 中新增 `conn_profile_map`（`BPF_MAP_TYPE_LRU_HASH`，`max_entries=65536`，key=`conn_key`，value=`__u32 profile_id`）
    - 新增 `profile_weight_map`（`BPF_MAP_TYPE_ARRAY`，`max_entries=64`，value=`__u32 cumulative_weight`）和 `profile_count_map`（`BPF_MAP_TYPE_ARRAY`，`max_entries=1`）
    - 修改 `bdna_tcp_rewrite`：先查 `conn_profile_map`，命中则用返回的 profile_id；**未命中（SYN 首包）→ C 侧自选**：用 `bpf_get_prng_u32()` 按 `profile_weight_map` 权重选择 profile_id，写入 `conn_profile_map`，再查 `fingerprint_map`。仅当 `profile_weight_map` 也无数据时回退到 `active_profile_map[0]`
    - 修改 `bdna_quic_rewrite` 同理：使用相同的 per-connection 查询路径
    - 修改 `bdna_tls_rewrite` 同理：使用相同的 per-connection 查询路径（不能继续走全局 `active_profile_map`，否则三条路径画像不一致）
    - 确保编译回归通过
    - _需求: 2.1, 2.5_

  - [ ] 3.2 B-DNA per-connection 画像选择 — Go 侧
    - 在 `bdna_profile_updater.go` 中新增启动时初始化：将画像族权重写入 `profile_weight_map`（CDF 格式）和 `profile_count_map`
    - 新增 `OverrideConnectionProfile(saddr, daddr uint32, sport, dport uint16, profileID uint32) error`（策略调整用，覆写 C 侧自选结果）
    - 按 `gateway.yaml` 的 `bdna.profile_weights` 配置权重
    - 默认权重：Chrome 65%、Firefox 15%、Safari 10%、Edge 10%
    - 首包画像由 C 侧 `bpf_get_prng_u32()` + `profile_weight_map` 保证，Go 侧不参与首包选择
    - _需求: 2.2, 2.3, 2.4_

  - [ ] 3.3 编写 Property 1: per-connection 画像隔离 PBT
    - 测试函数: `TestProperty_PerConnectionProfileIsolation`
    - 文件: `mirage-gateway/pkg/ebpf/bdna_conn_property_test.go`
    - 使用 `rapid` 生成随机 conn_key 集合（数量 ∈ [2,100]），分配画像后验证：
      - 同一 conn_key 多次查询返回相同 profile_id
      - 不同 conn_key 的 profile_id 分布符合配置权重（χ² 检验）
    - 最少 100 次迭代
    - **验证: 需求 2.1, 2.3, 2.4**

  - [ ] 3.4 编写 Property 4: 画像族权重分布 PBT
    - 测试函数: `TestProperty_ProfileWeightDistribution`
    - 文件: `mirage-gateway/pkg/ebpf/bdna_conn_property_test.go`
    - 使用 `rapid` 生成随机权重配置（各族权重 ∈ [1,100]），分配 1000 条连接后验证各族占比与权重比例的偏差 < 10%
    - 最少 100 次迭代
    - **验证: 需求 2.3**

  - [ ] 3.5 NPM MIMIC 分布模式 — C 侧
    - 在 `npm.c` 中新增 `npm_target_distribution_map`（`BPF_MAP_TYPE_ARRAY`，`max_entries=256`）
    - value 结构：`struct { __u32 cumulative_prob; __u16 pkt_len_low; __u16 pkt_len_high; }`
    - 在 `calculate_padding` 中新增 `case 3`（MIMIC）：从 CDF 采样目标包长，计算 padding
    - 采样使用 `bpf_get_prng_u32()`，二分查找 CDF
    - 保持现有三种模式行为不变
    - 确保编译回归通过
    - _需求: 3.1, 3.2, 3.4_

  - [ ] 3.6 NPM MIMIC 分布模式 — Go 侧
    - 在 `npm_verifier.go` 或新建 `npm_distribution.go` 中实现 `LoadTargetDistribution(baselinePath string) error`
    - 从 `baseline-distribution.json` 读取包长直方图，生成 256-bin CDF，写入 `npm_target_distribution_map`
    - 在 Gateway 启动时调用，配合 `npm_config_map` 的 `padding_mode=3`
    - _需求: 3.3_

  - [ ] 3.7 编写 Property 2: NPM MIMIC 分布拟合 PBT
    - 测试函数: `TestProperty_NPMMimicDistributionFit`
    - 文件: `mirage-gateway/pkg/ebpf/npm_property_test.go`
    - 使用 `rapid` 生成随机目标 CDF（256 bin，单调递增），随机包序列（数量 ∈ [100,500]，大小 ∈ [64,1500]）经 MIMIC Mock 处理后：
      - 输出包长分布与目标 CDF 的 JS 散度 < 0.15
      - 小包（< min_packet_size）不填充
      - 大包（> target_mtu）不截断
    - 最少 100 次迭代
    - **验证: 需求 3.2, 3.5**

  - [ ] 3.8 Jitter IAT 校准 — Go 侧
    - 在 `dna_updater.go` 中新增 `CalibrateFromBaseline(baselineStatsPath string) error`
    - 从 `baseline-stats.csv` 读取真实 IAT 统计（iat_mean_us / iat_std_us），写入 `dna_template_map`
    - 在 Gateway 启动时调用（在 `LoadAndSyncFingerprints` 之后）
    - _需求: 4.1, 4.2_

  - [ ] 3.9 编写 Property 3: Jitter 校准后 IAT 分布 PBT
    - 测试函数: `TestProperty_JitterCalibratedIATDistribution`
    - 文件: `mirage-gateway/pkg/ebpf/gaussian_property_test.go`
    - 使用 `rapid` 生成随机 baseline IAT 参数（mean ∈ [500,5000]μs, std ∈ [50,1000]μs），校准后生成 200 个 IAT 样本：
      - 样本均值与目标均值偏差 < 20%
      - 样本标准差与目标标准差偏差 < 30%
    - 最少 100 次迭代
    - **验证: 需求 4.1, 4.3**

- [ ] 4. Checkpoint — M14 缺陷修复确认
  - 确认 B-DNA 动态化、NPM MIMIC、Jitter 校准已实现，PBT 通过，编译回归通过。

- [ ] 5. M15：指纹审计与分类器实验迭代
  - [ ] 5.1 TLS/QUIC 指纹审计
    - 审计 `chameleon_client.go` 的 `dialWithUTLS`：检查 uTLS HelloChrome spec 的 extension 列表是否与画像库 `tls_ext_order` 一致
    - 审计 `quic_engine.go`：检查 QUIC transport parameters 是否与画像库 `quic_*` 字段一致
    - 审计 WebSocket upgrade 路径：检查 HTTP headers（User-Agent / Accept / Sec-WebSocket-*）
    - 发现不一致时修复代码，确保三条路径与选定画像族一致
    - 审计结论记录到 `docs/reports/stealth-experiment-results.md` 的"指纹审计"章节
    - _需求: 5.1, 5.2, 5.3, 5.4_

  - [ ] 5.2 重新生成实验样本
    - 使用 M14 修复后的代码 + M13 真实对照基线，重新生成 ReMirage 侧实验样本
    - 更新 `artifacts/dpi-audit/{handshake,packet-length,timing}/` 中的 remirage 样本
    - 非 Linux 环境：使用更新后的模拟参数重新生成模拟样本，标注"校准后模拟"
    - _需求: 6.1_

  - [ ] 5.3 重跑分类器实验
    - 执行 `artifacts/dpi-audit/classifier/train-classifier.py`
    - 记录四个检测面的 AUC/F1/Accuracy
    - 与修复前数据（全部 1.0）对比，量化改善幅度
    - 若 XGBoost 可用，同时报告 XGBoost 结果
    - _需求: 6.2, 6.3, 6.4_

  - [ ] 5.4 更新实验结论文档
    - 更新 `docs/reports/stealth-experiment-results.md`：
      - 修复前 AUC（全部 1.0，模拟数据）
      - 修复后 AUC（真实基线 / 校准后模拟）
      - 各检测面效果评级
      - 画像库校准偏差清单
      - 指纹审计结论
      - 实验限制说明
    - 标注证据强度：真实基线（Linux 采集）/ 校准后模拟（非 Linux）
    - _需求: 6.5_

- [ ] 6. Checkpoint — M15 实验结果确认
  - 确认分类器实验已重跑，AUC 数据已记录，请用户确认是否继续。

- [ ] 7. M16：治理回写与状态评估
  - [ ] 7.1 更新 `docs/reports/stealth-claims-boundary.md`
    - 根据新 AUC 数据更新允许/不允许表述
    - 若单维 AUC < 0.9 → 对应限定表述可升级为允许（附带 AUC 值）
    - 若单维 AUC ≥ 0.9 → 维持不允许，标注"高可区分性风险仍存在"
    - _需求: 7.1_

  - [ ] 7.2 评估能力域状态升级
    - 检查升级条件（三档）：
      - **可升级为"已实现（限定表述）"**：M14 代码实现完成 + M13 真实基线采集完成（非降级/非模拟） + 单维 AUC 均 < 0.75 + 联合 AUC < 0.85
      - **记录为"风险已下降但不升级"**：实现完成 + 真实基线 + 单维 AUC ∈ [0.75, 0.9) 或联合 AUC ∈ [0.85, 0.9)，维持"部分实现"但在 claims-boundary 中增加限定允许表述
      - **维持"部分实现"无变更**：AUC ≥ 0.9 或真实基线未采集（降级/模拟数据不可作为升级依据）
    - _需求: 7.2, 7.3, 7.4_

  - [ ] 7.3 回写 `docs/governance/capability-truth-source.md`
    - 更新"流量整形与特征隐匿"能力域的"当前真实能力"描述
    - 更新主证据锚点列表
    - 更新 Phase 5 实验结论
    - _需求: 7.2_

  - [ ] 7.4 回写 `docs/Mirage 功能确认与功能验证任务清单.md`
    - 新增"隐匿加固实验"条目：复验命令、通过标准、证据文件路径
    - _需求: 7.1_

- [ ] 8. Exit：Phase 5 出关判定
  - **Implementation Exit**（Phase 完成判定）：
    - M13 真实对照基线已采集（或标注降级，降级不阻塞 Phase 出关）
    - M14 三项修复已实现 + PBT 通过 + 编译回归通过
    - M15 分类器实验已重跑 + AUC 数据已记录
    - M16 治理回写完成
    - stealth-experiment-results.md 已更新
    - stealth-claims-boundary.md 已更新
    - capability-truth-source.md 已回写
  - **Capability-Upgrade Gate**（能力状态升级判定，与 Implementation Exit 分离）：
    - 仅当 M13 使用真实基线（非降级/非模拟）+ 单维 AUC 均 < 0.75 + 联合 AUC < 0.85 时，才可将能力域从"部分实现"升级为"已实现（限定表述）"
    - 降级/模拟数据通过 Implementation Exit 但不通过 Capability-Upgrade Gate

## 备注

- PBT 使用 `pgregory.net/rapid`（Go），最少 100 次迭代
- PBT 复用现有测试文件：`bdna_conn_property_test.go`、`npm_property_test.go`、`gaussian_property_test.go`
- eBPF 改动遵守 protocol-language-rules.md：C 做数据面，Go 做控制面，eBPF Map 通信
- M13 需要在对应原生 OS 上采集（Chrome-Win 在 Windows、Chrome-macOS 在 macOS、Firefox-Linux 在 Linux）；某 OS 不可用时该画像族标注"待采集"
- 画像库更新不追求"完全匹配某一个 Chrome 版本常量"，而是"按画像族生成一致指纹"
- 分类器 AUC 设计目标：单维 < 0.75、联合 < 0.85；升级门禁与设计目标一致，不设宽松中间档
- Implementation Exit 与 Capability-Upgrade Gate 分离：Phase 可降级出关，但降级/模拟数据不可作为能力状态升级依据
- 真实采集元数据存储为 `capture-metadata.json`，`simulation-metadata.json` 仅保留给模拟样本
- 证据产物路径：`artifacts/dpi-audit/baseline/`（真实基线）、`docs/reports/`（报告）
