---
Status: active
Source of Truth: 本文档为 Phase 2 M5 里程碑的主产出
Role: 隐匿实验方案，定义四个检测面的实验方法论
Evidence Strength: 受控环境基线
---

# 隐匿实验方案（Stealth Experiment Plan）

## 一、文档目的

本文档定义 ReMirage 隐匿能力验证的实验方法论，覆盖四个检测面：

1. 握手指纹（JA3/JA4）
2. 包长分布
3. 时序分布（IAT）
4. 简单分类器可分性

本文档是方法论冻结文档，不包含实验结果。实验结果将在 M6 阶段产出并记录到 `docs/reports/stealth-experiment-results.md`。

## 二、实验环境约束

| 约束项 | 说明 |
|--------|------|
| 执行环境 | 受控本地环境（loopback 接口或 mock 流量） |
| 网络条件 | 非真实对抗网络，无中间设备干扰 |
| 操作系统 | Linux（内核 ≥ 5.15），非 Linux 降级为 mock/simulation |
| 编译工具 | clang + BPF target |
| 抓包工具 | tcpdump / tshark（需 root 或 CAP_NET_RAW） |
| 证据强度 | **受控环境基线** — 不可外推为"可抵抗生产级 DPI/ML 系统" |
| 降级标注 | 非 Linux 环境产出的数据标注为"模拟环境参考"，不计入正式证据 |

### 环境检查清单

实验执行前必须通过以下检查：

- [ ] `uname -r` 确认内核版本 ≥ 5.15
- [ ] `clang --version` 确认 clang 可用
- [ ] `tcpdump --version` 确认抓包工具可用
- [ ] `python3 --version` 确认 Python ≥ 3.8（分类器实验）
- [ ] `pip3 list | grep scikit-learn` 确认 sklearn 可用
- [ ] `bpftrace --version` 确认 bpftrace 可用（性能采集，可选）
- [ ] 确认具备 root 或 CAP_NET_RAW 权限

## 三、检测面 1：握手指纹实验

### 3.1 实验目标

验证 B-DNA 的 TCP SYN 重写和 JA4 捕获链路在握手外观上的实际效果，确认重写后的握手指纹与目标画像的匹配程度，以及与真实浏览器流量的差异。

### 3.2 对照组定义

| 对照组 | 来源 | 说明 |
|--------|------|------|
| 真实 Chrome 浏览器 | 本地 Chrome 访问目标站点的抓包，或公开 pcap 数据集 | 作为"真实浏览器"基准 |
| 常见 uTLS 配置 | 本地使用 Go uTLS 库（`utls.HelloChrome_Auto`）生成的握手流量 | 作为"常见伪装库"基准 |
| ReMirage（B-DNA 启用） | B-DNA `bdna_tcp_rewrite` 启用 + `active_profile_map` 加载指纹模板 | 实验对象 |
| ReMirage（B-DNA 关闭） | B-DNA 未启用，使用 Go 默认 TCP 栈 | 作为"无伪装"基准 |

### 3.3 采集条件

| 条件 | 值 |
|------|-----|
| 网络接口 | loopback（`lo`） |
| TLS 版本 | TLS 1.3（优先）；若目标站点回退到 TLS 1.2 则同时记录 |
| 目标站点 | 本地 TLS 服务器（自签证书）或 `localhost:443` |
| 采集工具 | `tcpdump -i lo -w <output>.pcapng` |
| 每组采集量 | ≥ 100 次独立握手 |
| eBPF 配置 | `active_profile_map` 加载默认 Chrome 画像（`profile-registry.v1.json` 中的默认 profile） |

### 3.4 比较维度

| 维度 | 提取方式 | 比较方法 |
|------|----------|----------|
| JA3 hash | 从 TLS ClientHello 提取 → MD5 | 精确匹配：hash 是否一致 |
| JA4 fingerprint | `bdna_ja4_capture` 捕获 / Python scapy 提取 | 精确匹配 + 字段级差异分析 |
| TCP options | 从 TCP SYN 提取 MSS、Window Scale、SACK Permitted、Timestamps | 逐字段对比：值是否与目标模板匹配 |
| TCP Window Size | 从 TCP SYN 提取 | 精确匹配：是否等于 `fingerprint.tcp_window` |
| TLS extension 顺序 | 从 ClientHello 提取扩展列表 | 有序列表比较：顺序是否一致 |
| TLS extension 数量 | 从 ClientHello 提取 | 数值比较 |
| QUIC transport parameters | 从 QUIC Initial 提取（若有 QUIC 流量） | 逐参数对比：`max_idle_timeout`、`initial_max_data`、`initial_max_streams_bidi/uni`、`ack_delay_exponent` |

### 3.5 统计采集

从 `bdna_stats_map` 读取以下计数器：

- `tcp_rewritten` — TCP SYN 成功重写次数
- `quic_rewritten` — QUIC Initial 标记次数
- `tls_rewritten` — TLS ClientHello 标记次数
- `skipped` — 跳过（无模板或非匹配包）次数

### 3.6 已知限制标注

- B-DNA 对 QUIC Initial 和 TLS ClientHello 的处理方式为"内核标记（`skb->mark`）+ 用户态协同"，不是内核完整重写
- 本实验仅验证 TCP SYN 层面的内核态重写效果和 JA4 捕获准确性
- QUIC/TLS 的完整重写效果取决于用户态协同链路，本轮实验标注为"协同链路存在，完整效果待验证"

### 3.7 产出物

| 产出 | 路径 |
|------|------|
| ReMirage 握手抓包 | `artifacts/dpi-audit/handshake/remirage-syn.pcapng` |
| Chrome 对照抓包 | `artifacts/dpi-audit/handshake/chrome-syn.pcapng` |
| uTLS 对照抓包 | `artifacts/dpi-audit/handshake/utls-syn.pcapng` |
| 特征提取脚本 | `artifacts/dpi-audit/handshake/extract-features.py` |
| 对比结果 | `artifacts/dpi-audit/handshake/comparison.csv` |


## 四、检测面 2：包长分布实验

### 4.1 实验目标

验证 NPM 的 XDP 层填充在包长分布上的实际效果，确认三种填充模式（固定 MTU、随机区间、高斯分布）是否能有效改变包长外观，以及改变后的分布与无填充流量的统计差异。

### 4.2 对照组定义

| 对照组 | 配置 | 说明 |
|--------|------|------|
| 无填充基线 | NPM `enabled=0` | 原始流量包长分布，作为对照基准 |
| 固定 MTU 模式 | `padding_mode=NPM_MODE_FIXED_MTU(0)`, `global_mtu=1400` | 所有包对齐到目标 MTU |
| 随机区间模式 | `padding_mode=NPM_MODE_RANDOM_RANGE(1)`, `global_mtu=1400` | 在剩余空间内随机填充 |
| 高斯分布模式 | `padding_mode=NPM_MODE_GAUSSIAN(2)`, `global_mtu=1400` | 简化正态分布靠近目标区间 |

### 4.3 采集条件

| 条件 | 值 |
|------|-----|
| 采集规模 | **≥ 1000 次新建连接**（每种模式） |
| 网络接口 | loopback（`lo`） |
| 流量类型 | 混合 TCP/UDP 流量（HTTP GET、TLS 握手、短连接、长连接） |
| NPM 配置 | `filling_rate=100`（全量填充，消除概率采样干扰） |
| `min_packet_size` | 分两轮：第一轮 `min_packet_size=0`（全填充），第二轮 `min_packet_size=128`（验证小包跳过） |
| 抓包工具 | `tcpdump -i lo -w <output>.pcapng` |

### 4.4 统计维度

| 维度 | 计算方法 | 用途 |
|------|----------|------|
| 前 10 包长度 | 每次连接的前 10 个包的 IP 总长度 | 检测启动阶段特征 |
| 前 10 包方向 | 每次连接的前 10 个包的方向（上行/下行） | 检测方向模式 |
| 整体包长直方图 | 所有包的 IP 总长度分布（bin=10 bytes） | 检测整体分布形态 |
| 上下行比例 | 上行总字节 / 下行总字节 | 检测流量对称性 |
| 包长熵值 | Shannon 熵：$H = -\sum p_i \log_2 p_i$（$p_i$ 为各 bin 的概率） | 量化分布均匀度 |
| KL 散度 | $D_{KL}(P \| Q)$，P=填充后分布，Q=无填充分布 | 量化与对照组的分布距离 |
| JS 散度 | $D_{JS} = \frac{1}{2}D_{KL}(P \| M) + \frac{1}{2}D_{KL}(Q \| M)$，$M=\frac{P+Q}{2}$ | 对称化的分布距离 |

### 4.5 统计采集

从 `npm_stats_map` 读取以下计数器：

- `total_packets` — 总处理包数
- `padded_packets` — 成功填充包数
- `padding_bytes` — 总填充字节数
- `skipped_packets` — 跳过包数（小于 `min_packet_size` 或概率未命中）

验证点：
- `padded_packets + skipped_packets ≤ total_packets`
- `min_packet_size > 0` 时，小于阈值的控制性小包确实被跳过

### 4.6 已知限制标注

- NPM 当前仅处理 IPv4 流量
- `filling_rate=100` 的全量填充场景不代表生产环境配置
- 包长填充不改变包的数量和方向，仅改变长度外观
- 诱饵包（`decoy_rate`）不在本轮实验范围内

### 4.7 产出物

| 产出 | 路径 |
|------|------|
| 固定 MTU 模式抓包 | `artifacts/dpi-audit/packet-length/mode-fixed-mtu.pcapng` |
| 随机区间模式抓包 | `artifacts/dpi-audit/packet-length/mode-random-range.pcapng` |
| 高斯分布模式抓包 | `artifacts/dpi-audit/packet-length/mode-gaussian.pcapng` |
| 无填充基线抓包 | `artifacts/dpi-audit/packet-length/baseline-no-padding.pcapng` |
| 分布分析脚本 | `artifacts/dpi-audit/packet-length/analyze-distribution.py` |
| 统计结果 | `artifacts/dpi-audit/packet-length/distributions.csv` |

## 五、检测面 3：时序分布实验

### 5.1 实验目标

验证 Jitter-Lite 和 VPC 在 IAT（包间到达时间）与节奏控制上的实际效果，确认时域扰动是否能有效改变流量的时序特征，以及改变后的时序分布与原始流量的统计差异。

### 5.2 对照条件：Jitter ± VPC 的 2×2 矩阵

| 配置组合 | Jitter-Lite | VPC | 说明 |
|----------|-------------|-----|------|
| (a) 基线 | 关闭 | 关闭 | 原始时序，无任何扰动 |
| (b) 仅 Jitter | 开启 | 关闭 | 仅 IAT 扰动，无背景噪声 |
| (c) 仅 VPC | 关闭 | 开启 | 仅背景噪声，无 IAT 扰动 |
| (d) 联合 | 开启 | 开启 | IAT 扰动 + 背景噪声叠加 |

### 5.3 采集条件

| 条件 | 值 |
|------|-----|
| 采集规模 | 每种配置组合 ≥ 500 次连接 |
| 网络接口 | loopback（`lo`） |
| 流量类型 | 稳定速率 TCP 流量（消除应用层突发干扰） |
| Jitter 配置 | `enabled=1`, `mean_iat_us=1000`, `stddev_iat_us=200` |
| VPC 配置 | `enabled=1`, `fiber_jitter_us=50`, `router_delay_us=100`, `noise_intensity=50` |
| DNA 模板 | 第一轮：`dna_template_map` 有模板（验证模板优先级）；第二轮：`dna_template_map` 无模板（验证回退到 `jitter_config_map`） |
| 抓包工具 | `tcpdump -i lo -w <output>.pcapng -tt`（带时间戳） |

### 5.4 采集维度

| 维度 | 计算方法 | 用途 |
|------|----------|------|
| IAT 均值 | 相邻包时间戳差的算术平均 | 基本时序特征 |
| IAT 标准差 | 相邻包时间戳差的标准差 | 时序波动程度 |
| IAT P50 / P95 / P99 | 相邻包时间戳差的分位数 | 尾部延迟特征 |
| Burst 结构 | 连续 IAT < 阈值（如 100μs）的包组成一个 burst，统计 burst 大小和间隔 | 突发模式特征 |
| 时段特征 | 按时间窗口（如 1s）统计包速率变化 | 宏观节奏特征 |

### 5.5 配置优先级验证

验证 `jitter_lite_egress` 的配置读取优先级：

1. **有 DNA 模板时**：`dna_template_map` 中存在对应模板 → 使用 `get_mimic_delay(tpl)` 计算延迟
2. **无 DNA 模板时**：`dna_template_map` 中不存在模板 → 回退到 `jitter_config_map` 使用 `gaussian_sample` 计算延迟
3. **两条路径**都应导致 `skb->tstamp` 被修改为 `now + delay_ns`（`delay_ns > 0`）

验证方法：
- 第一轮采集：写入 `dna_template_map`，观察 IAT 分布是否符合模板参数
- 第二轮采集：清空 `dna_template_map`，观察 IAT 分布是否符合 `jitter_config_map` 参数
- 对比两轮 IAT 分布差异

### 5.6 VPC 噪声统计采集

从 `vpc_noise_stats` 读取以下计数器：

- `total_packets` — 总处理包数
- `delayed_packets` — 被延迟的包数
- `total_delay_us` — 总延迟微秒数
- `dropped_packets` — 被丢弃的包数
- `reordered_packets` — 被乱序的包数
- `duplicated_packets` — 被复制的包数

### 5.7 已知限制标注

- Jitter-Lite 通过 `skb->tstamp` 控制发送时机，实际效果受内核 FQ 调度器影响
- VPC 的物理噪声模型（光缆抖动、路由器延迟等）在 loopback 环境下为模拟值，不代表真实网络特征
- `jitter_lite_adaptive`、`jitter_lite_physical`、`jitter_lite_social` 在 loader 中未挂载，本轮实验仅验证 `jitter_lite_egress` 主路径

### 5.8 产出物

| 产出 | 路径 |
|------|------|
| 基线抓包（无扰动） | `artifacts/dpi-audit/timing/config-none.pcapng` |
| 仅 Jitter 抓包 | `artifacts/dpi-audit/timing/config-jitter-only.pcapng` |
| 仅 VPC 抓包 | `artifacts/dpi-audit/timing/config-vpc-only.pcapng` |
| 联合抓包 | `artifacts/dpi-audit/timing/config-jitter-vpc.pcapng` |
| 时序分析脚本 | `artifacts/dpi-audit/timing/analyze-timing.py` |
| IAT 统计结果 | `artifacts/dpi-audit/timing/iat-stats.csv` |


## 六、检测面 4：简单分类器可分性实验

### 6.1 实验目标

使用简单 ML 分类器对检测面 1-3 产出的特征数据做可分性测试，量化当前隐匿能力的实际效果边界。回答核心问题：**一个简单分类器能否区分 ReMirage 流量与对照组流量？**

### 6.2 分类器选择

| 分类器 | 优先级 | 说明 |
|--------|--------|------|
| RandomForest | **必须** | sklearn `RandomForestClassifier`，默认参数（`n_estimators=100`） |
| XGBoost | **优先** | `xgboost.XGBClassifier`，默认参数；若 `import xgboost` 失败，降级为仅 RandomForest 并在结果中显式标注"XGBoost 不可用，仅 RandomForest 结果" |

不使用深度学习模型（CNN/RNN），原因：
- 本轮目标是建立"简单分类器基线"，不是追求最优分类性能
- 简单分类器的可分性结论更保守：若简单分类器即可区分，说明特征差异显著

### 6.3 特征工程

#### 6.3.1 握手特征集（来自检测面 1）

| 特征 | 类型 | 说明 |
|------|------|------|
| `tcp_window` | 数值 | TCP SYN Window Size |
| `tcp_mss` | 数值 | MSS Option 值 |
| `tcp_wscale` | 数值 | Window Scale 值 |
| `tcp_sack` | 布尔 | SACK Permitted 是否存在 |
| `tcp_timestamps` | 布尔 | Timestamps 是否存在 |
| `tls_ext_count` | 数值 | TLS 扩展数量 |
| `ja4_hash` | 类别 | JA4 指纹（编码为类别特征） |

#### 6.3.2 包长特征集（来自检测面 2）

| 特征 | 类型 | 说明 |
|------|------|------|
| `pkt_len_1` ~ `pkt_len_10` | 数值 | 前 10 包长度 |
| `pkt_dir_1` ~ `pkt_dir_10` | 布尔 | 前 10 包方向（0=上行, 1=下行） |
| `up_down_ratio` | 数值 | 上下行字节比例 |
| `pkt_len_entropy` | 数值 | 包长 Shannon 熵 |
| `pkt_len_mean` | 数值 | 包长均值 |
| `pkt_len_std` | 数值 | 包长标准差 |

#### 6.3.3 时序特征集（来自检测面 3）

| 特征 | 类型 | 说明 |
|------|------|------|
| `iat_mean` | 数值 | IAT 均值 |
| `iat_std` | 数值 | IAT 标准差 |
| `iat_p50` | 数值 | IAT P50 |
| `iat_p95` | 数值 | IAT P95 |
| `iat_p99` | 数值 | IAT P99 |
| `burst_count` | 数值 | Burst 数量 |
| `burst_mean_size` | 数值 | 平均 Burst 大小 |
| `burst_mean_interval` | 数值 | 平均 Burst 间隔 |

### 6.4 标签定义

| 标签 | 值 | 说明 |
|------|-----|------|
| ReMirage 流量 | 1 | B-DNA + NPM + Jitter/VPC 启用 |
| 对照组流量 | 0 | 真实 Chrome / uTLS / 无伪装流量 |

### 6.5 训练/测试集划分

| 参数 | 值 |
|------|-----|
| 划分方式 | 分层随机划分（`stratified split`） |
| 训练集比例 | 70% |
| 测试集比例 | 30% |
| 交叉验证 | 5-fold 交叉验证（报告训练集和测试集指标，防止过拟合） |
| 随机种子 | `random_state=42`（可复现） |

### 6.6 评估指标

| 指标 | 计算方式 | 判定阈值 |
|------|----------|----------|
| AUC | ROC 曲线下面积 | > 0.9 标注为"高可区分性风险" |
| F1 | 精确率与召回率的调和平均 | > 0.85 标注为"显著可区分" |
| 准确率 | 正确分类样本 / 总样本 | 参考指标 |
| 混淆矩阵 | TP / FP / TN / FN | 分析误分类模式 |

### 6.7 实验矩阵

分别报告以下分类任务的结果：

| 实验编号 | 特征集 | 说明 |
|----------|--------|------|
| C1 | 仅握手特征 | 单维度：握手指纹可分性 |
| C2 | 仅包长特征 | 单维度：包长分布可分性 |
| C3 | 仅时序特征 | 单维度：时序分布可分性 |
| C4 | 握手 + 包长 + 时序 | 多维度联合：综合可分性 |

### 6.8 风险标注规则

- 任一单维度 AUC > 0.9 → 该维度标注为 **"高可区分性风险"**，建议后续优先改进
- 任一单维度 AUC ∈ [0.7, 0.9] → 该维度标注为 **"中等可区分性"**，需关注
- 任一单维度 AUC < 0.7 → 该维度标注为 **"低可区分性"**，当前伪装效果可接受
- 联合 AUC > 0.9 → 标注为 **"综合高可区分性风险"**

### 6.9 已知限制标注

- 分类器复杂度有限（RandomForest / XGBoost），不代表高级 DPI/ML 系统的检测能力
- 受控环境样本不包含真实网络噪声、中间设备干扰和多跳路由影响
- 样本规模受限于本地采集能力，不代表大规模流量场景
- 本实验结论不可外推为"可抵抗生产级 DPI/ML 系统"

### 6.10 产出物

| 产出 | 路径 |
|------|------|
| 分类器训练脚本 | `artifacts/dpi-audit/classifier/train-classifier.py` |
| 依赖声明 | `artifacts/dpi-audit/classifier/requirements.txt` |
| 特征数据 | `artifacts/dpi-audit/classifier/features.csv` |
| 分类结果 | `artifacts/dpi-audit/classifier/results.json` |
| 混淆矩阵 | `artifacts/dpi-audit/classifier/confusion-matrices/` |

## 七、实验执行流水线

### 7.1 统一流水线

每个检测面实验遵循统一流水线：

```
1. 环境检查 → 确认 Linux / clang / tcpdump / Python 可用
2. 加载 eBPF 程序 → 通过 Go 测试 helper 或独立加载器
3. 写入配置 Map → 设置对应检测面的实验参数
4. 启动抓包 → tcpdump -i lo -w <output>.pcapng
5. 注入测试流量 → loopback 流量生成
6. 停止抓包 → 采集 pcapng + 读取 stats Map
7. 特征提取 → Python 脚本处理 pcapng
8. 统计分析 → 计算分布指标 / 训练分类器
9. 生成报告 → 写入 CSV / JSON 结果文件
```

### 7.2 编排脚本

| 脚本 | 路径 | 职责 |
|------|------|------|
| 样本生成 | `artifacts/dpi-audit/generate-samples.sh` | 统一编排四个检测面的抓包样本生成 |
| M6 实验 | `deploy/scripts/drill-m6-experiment.sh` | 端到端实验执行：环境检查 → 样本生成 → PBT → 分析 → 分类器 |

### 7.3 非 Linux 降级策略

| 场景 | 降级行为 |
|------|----------|
| 非 Linux 系统 | 跳过真实 eBPF 加载和抓包，使用模拟数据运行分析脚本 |
| 无 root 权限 | 跳过 tcpdump 抓包，仅使用 eBPF Map 统计数据 |
| 无 Python | 跳过分类器实验，仅产出抓包和 Map 统计 |
| 无 XGBoost | 降级为仅 RandomForest，标注"XGBoost 不可用" |

## 八、证据强度等级定义

| 等级 | 含义 | 适用场景 |
|------|------|----------|
| 受控环境基线 | 在 loopback / mock 环境下产出的实验数据 | Linux + eBPF 真实加载 |
| 模拟环境参考 | 在非 Linux 或 mock 环境下产出的模拟数据 | 非 Linux / 无 root |
| 生产环境观测 | 在真实网络环境下产出的观测数据 | **本轮不涉及** |

本轮所有实验结论的证据强度标注为 **"受控环境基线"**。

任何基于本轮实验的对外表述，必须附带以下限定：
- "基于受控本地环境实验"
- "不代表真实对抗网络环境下的效果"
- "不可外推为可抵抗生产级 DPI/ML 系统"

## 九、与 DPI 风险审计清单的对应关系

| DPI 审计清单检测面 | 本实验覆盖 | 说明 |
|-------------------|-----------|------|
| 握手与栈指纹（P0） | ✅ 检测面 1 | 覆盖 JA3/JA4、TCP options、TLS extension |
| 首包与启动阶段统计特征（P0） | ✅ 检测面 2 | 覆盖前 10 包长度、方向、熵值 |
| 时序 / IAT / 节奏识别（P0） | ✅ 检测面 3 | 覆盖 IAT、burst 结构、Jitter±VPC 矩阵 |
| 应用层行为拟态缺口（P0） | ❌ 不覆盖 | 需要 WSS/HTTPS 全会话抓包，超出本轮范围 |
| 主动探测 / 重放（P0） | ❌ 不覆盖 | 需要多源 IP 探测，超出本轮范围 |
| 多承载降级过程（P0） | ❌ 不覆盖 | 需要 G-Tunnel 全链路，超出本轮范围 |
| 背景噪声真实性（P1） | 部分覆盖 | 检测面 3 的 VPC 部分涉及，但不含真实业务流量对照 |

## 十、术语与引用

| 术语 | 定义 |
|------|------|
| JA3 | TLS 客户端指纹哈希，基于 TLS 版本、密码套件、扩展列表等 |
| JA4 | JA3 的增强版，增加 ALPN、签名算法等维度 |
| IAT | Inter-Arrival Time，包间到达时间 |
| KL 散度 | Kullback-Leibler 散度，衡量两个概率分布的差异 |
| JS 散度 | Jensen-Shannon 散度，KL 散度的对称化版本 |
| AUC | Area Under the ROC Curve，分类器性能指标 |
| F1 | 精确率与召回率的调和平均，分类器性能指标 |
| Shannon 熵 | 信息熵，衡量分布的不确定性 |
| Burst | 连续快速到达的包组，IAT 低于阈值的连续包序列 |

### 引用文档

- `docs/governance/dpi-risk-audit-checklist.md` — DPI 风险审计清单
- `docs/governance/capability-truth-source.md` — 能力真相源
- `docs/protocols/npm.md` — NPM 协议语义入口
- `docs/protocols/bdna.md` — B-DNA 协议语义入口
- `docs/protocols/jitter-lite.md` — Jitter-Lite 协议语义入口
- `docs/protocols/vpc.md` — VPC 协议语义入口
- `docs/reports/stealth-claims-boundary.md` — 对外表述边界清单
- `docs/reports/stealth-experiment-results.md` — 实验结果报告（M6 产出）
