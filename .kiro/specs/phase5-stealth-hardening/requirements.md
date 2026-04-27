# 需求文档：Phase 5 流量隐匿加固

## 背景

Phase 2 M6 分类器实验结果显示四个检测面全部 AUC=1.0 / F1=1.0 / Accuracy=1.0，ReMirage 流量特征被简单 RandomForest 完全分开。

边界说明：这些结果来自模拟样本（`generate-simulated-samples.py`），不是真实 Linux 抓包基线。不能断言"真实网络里也一定 100% 可区分"，但可以断言：当前隐匿实验暴露出严重可分性风险，且没有证据支持对外宣称抗 DPI/ML。

本 Spec 目标：修复已识别的隐匿缺陷，采集真实对照基线，重跑分类器实验，用数据决定能力域状态是否可升级。

## 约束

- 本 Spec 不新增协议，只修复现有协议的隐匿效果
- 能力域状态升级条件：第一层实现完成 + 原生 OS 真实抓包基线采集完成 + 单维/联合 AUC 达到目标后，才可从"部分实现"升级为"已实现（限定表述）"
- 不追求"完全匹配某一个 Chrome 常量"，目标是"按目标画像族生成一致指纹"
- H2 SETTINGS、H3/QUIC、WebSocket upgrade 三条路径分别审计，不混为一谈
- eBPF 数据面改动遵守 protocol-language-rules.md 铁律
- 所有实验结论必须标注证据强度等级

## 需求

### 1. 真实对照基线采集

- 1.1 采集真实浏览器访问主流 HTTPS 站点的流量，提取握手指纹（TCP SYN 字段 + TLS ClientHello extension 列表 + JA4）、包长分布（前 N 包长度/方向/上下行比例/熵值）、IAT 分布（均值/标准差/P50/P95/P99/burst 结构）
- 1.2 采集至少 3 个目标画像族的对照数据，每族至少 100 条连接。画像族必须在对应原生 OS 上采集：Chrome-Win 在 Windows 节点采集、Chrome-macOS 在 macOS 节点采集、Firefox-Linux 在 Linux 节点采集。不允许用 Linux 浏览器数据代表 Windows/macOS 画像族（TCP 栈、TLS 库、OS 指纹不同）。若某 OS 节点不可用，该画像族标注"待采集"而非用其他 OS 替代
- 1.3 采集结果替换 `artifacts/dpi-audit/` 中的模拟 pcapng 文件，保留模拟数据为 `*.simulated.pcapng` 后缀备份
- 1.4 真实采集元数据存储为 `artifacts/dpi-audit/baseline/capture-metadata.json`（内核版本、浏览器版本、OS 版本、网络条件），不复用 `simulation-metadata.json`。`simulation-metadata.json` 仅保留给模拟样本，两套元数据文件语义隔离

### 2. B-DNA 指纹动态化

- 2.1 `bdna.c` 从 per-global `active_profile_map` 改为 per-connection 画像选择：每条新连接根据 `conn_key` 查 `conn_profile_map` 获取 profile_id，再从 `fingerprint_map` 取对应模板。**首包时序要求**：TCP SYN 是第一个可观测指纹，`conn_profile_map` 必须在 SYN 到达 `bdna_tcp_rewrite` 时已有值。实现方式二选一：（A）C 侧首包自选——SYN 首包未命中 `conn_profile_map` 时，由 eBPF 侧用 `bpf_get_prng_u32() % profile_count` 选择 profile_id 并写入 `conn_profile_map`，后续包直接查表；（B）Go 侧预注册——在 listener accept 之前通过 eBPF Map 预写 profile_id（仅适用于 Gateway 主动监听场景）。推荐方案 A，因为 SYN 到达时 Go 侧尚未感知连接
- 2.2 Go 控制面负责维护 `profile_select_map`（`BPF_MAP_TYPE_ARRAY`，`max_entries=64`，value=`struct { __u32 cumulative_weight; __u32 profile_id; }`）和 `profile_count_map`（当前可用画像数量）。C 侧首包自选时遍历 `profile_select_map` 按 cumulative_weight 采样，返回对应的真实 `profile_id`。这样画像 ID 不需要连续，registry 中禁用/待采集的画像不写入 `profile_select_map` 即可排除
- 2.3 权重配置可通过 `gateway.yaml` 调整，默认权重按全球浏览器市场份额分配
- 2.4 确保同一连接生命周期内画像不变（TCP SYN 重写和后续 QUIC/TLS 参数使用同一 profile）
- 2.5 `bdna_tls_rewrite` 和 `bdna_quic_rewrite` 必须与 `bdna_tcp_rewrite` 使用相同的 per-connection 画像查询路径（先查 `conn_profile_map`，未命中回退 `active_profile_map[0]`），确保三条重写路径画像一致
- 2.6 画像库数据（`fingerprints.yaml` / `profile-registry.v1.json`）中的值必须与真实采集的对照基线一致，发现偏差时更新画像库

### 3. NPM 拟态分布模式

- 3.1 在 `npm.c` 的 `calculate_padding` 中新增第四种模式 `NPM_MODE_MIMIC`（mode=3）
- 3.2 MIMIC 模式从 `npm_target_distribution_map`（BPF_MAP_TYPE_ARRAY，256 个 bin）中采样目标包长，每个 bin 存储该包长区间的累积概率
- 3.3 Go 控制面从真实对照基线的包长直方图生成 CDF，写入 `npm_target_distribution_map`
- 3.4 MIMIC 模式保留小包不填充（< min_packet_size）、大包不截断（> target_mtu）的现有行为
- 3.5 MIMIC 模式的填充目标是让填充后的包长分布与目标流量的 JS 散度 < 0.1

### 4. Jitter IAT 拟态校准

- 4.1 `dna_template_map` 中的 `TargetIATMu` / `TargetIATSigma` 必须从真实对照基线的 IAT 统计值校准，不能使用任意配置值
- 4.2 Go 控制面在启动时从对照基线数据加载 IAT 参数，写入 `dna_template_map`
- 4.3 Jitter 扰动后的 IAT 分布验收指标：均值偏差 < 20%、标准差偏差 < 30%、P95 偏差 < 50%。KS 检验 p-value > 0.05 作为远期设计目标，不作为本轮升级门禁（真实 IAT 往往是重尾/突发/多峰分布，单一 gaussian 参数难以满足 KS 检验；若需通过 KS，需后续引入经验 CDF 或混合分布模型）
- 4.4 `jitter.c` 的 `jitter_lite_egress` 在无 `dna_template` 时的回退行为保持不变（使用 `jitter_config` 的 gaussian_sample）

### 5. TLS/QUIC 指纹对齐

- 5.1 审计 `chameleon_client.go` 的 `dialWithUTLS` 使用的 uTLS ClientHello spec，确保 TLS extension 列表与选定画像族一致（extension 数量、顺序、内容）
- 5.2 审计 `quic_engine.go` 的 QUIC transport parameters，确保与选定画像族的 QUIC 参数一致（max_idle_timeout、initial_max_data、initial_max_streams_bidi/uni、ack_delay_exponent）
- 5.3 分别审计三条握手路径的指纹一致性：
  - H2 over TLS（WSS 路径）：TLS ClientHello + HTTP/2 SETTINGS
  - H3 over QUIC（主路径）：QUIC Initial + QUIC transport parameters
  - WebSocket upgrade（降级路径）：TLS ClientHello + HTTP/1.1 Upgrade headers
- 5.4 每条路径的审计结论记录到 `docs/reports/stealth-experiment-results.md`

### 6. 分类器实验迭代

- 6.1 在第一层修复（需求 2/3/4）完成后，使用真实对照基线（需求 1）重新生成实验样本
- 6.2 重跑四个检测面的分类器实验（握手指纹、包长分布、时序分布、联合分类）
- 6.3 记录修复前后的 AUC/F1 对比，量化改善幅度
- 6.4 单维度 AUC 设计目标 < 0.75，联合 AUC 设计目标 < 0.85（实际值由实验决定，不达标不伪造）
- 6.5 实验结论更新到 `docs/reports/stealth-experiment-results.md`，标注证据强度为"受控环境真实基线"

### 7. 治理回写

- 7.1 根据实验结果更新 `docs/reports/stealth-claims-boundary.md` 的允许/不允许表述
- 7.2 根据实验结果评估 `docs/governance/capability-truth-source.md` 中"流量整形与特征隐匿"能力域是否满足从"部分实现"升级为"已实现（限定表述）"的条件
- 7.3 能力域状态升级条件（三档）：
  - **可升级为"已实现（限定表述）"**：需求 2/3/4 实现完成 + 需求 1 真实基线采集完成（非降级/非模拟） + 单维 AUC 均 < 0.75 + 联合 AUC < 0.85
  - **记录为"风险已下降但不升级"**：实现完成 + 真实基线 + 单维 AUC ∈ [0.75, 0.9) 或联合 AUC ∈ [0.85, 0.9)，维持"部分实现"但在 claims-boundary 中增加限定允许表述
  - **维持"部分实现"无变更**：AUC ≥ 0.9 或真实基线未采集（降级/模拟数据不可作为升级依据）
- 7.4 不满足升级条件时维持"部分实现"，记录实际 AUC 值和差距说明
