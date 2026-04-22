# B-DNA

本文件是 B-DNA 的当前有效协议语义入口。它不再沿用旧 `B-DNA-行为识别协议.md` 中“全球浏览器指纹库 + 理想化动态 TLS/QUIC 栈”的整包叙述，而是只接管已经由当前实现承载的指纹、握手与 JA4 风险链路。

## 协议定位

B-DNA 是 ReMirage 的指纹与握手行为伪装协议，负责让流量在 TCP SYN、TLS ClientHello、QUIC Initial 和 JA4 外显行为上更接近目标画像，并把高风险指纹暴露纳入联动处置。

它当前稳定承担四类职责：

1. 在 TC 数据面重写 TCP SYN 的部分选项与窗口语义。
2. 为 QUIC Initial 和 TLS ClientHello 建立“内核标记 + 用户态配合重写”的协同入口。
3. 捕获 JA4 指纹并进入分析、上报与高风险处置链路。
4. 作为伪装扰动层中“指纹 / 握手维度”的正式语义入口。

## 主真相归属

- 协议语义入口：本文档
- 运行时主锚点：
  - `mirage-gateway/bpf/bdna.c`
  - `mirage-gateway/pkg/ebpf/bdna_profile_updater.go`
  - `mirage-gateway/configs/bdna/profile-registry.v1.json`
  - `mirage-gateway/pkg/ebpf/dna_updater.go`
  - `mirage-gateway/pkg/ebpf/persona_updater.go`
  - `mirage-gateway/pkg/cortex/analyzer.go`
  - `mirage-gateway/pkg/cortex/fingerprint_reporter.go`
  - `mirage-gateway/pkg/gswitch/manager.go`
  - `mirage-gateway/pkg/gswitch/autonomous.go`

## 当前有效的配置语义

### 数据面活跃画像

`mirage-gateway/bpf/bdna.c` 当前已经稳定定义两类核心 map：

- `fingerprint_map`
- `active_profile_map`

它们共同表达当前最直接的数据面事实：

1. 存在一组可被选择的协议栈指纹模板。
2. 当前数据面会读取一个激活中的 profile。
3. TCP / QUIC / TLS 分支都围绕这个 active profile 运转。

### 版本化画像 registry

本轮之后，B-DNA 默认画像库已经不再以内嵌 Go 常量充当主源，而是外推到版本化 registry：

- `mirage-gateway/configs/bdna/profile-registry.v1.json`

当前 registry 至少明确承载四类事实：

1. `schema_version`
2. `registry_version`
3. `default_active_profile_id`
4. `profiles[]`

其中每个 profile 都直接表达当前 `stack_fingerprint` 需要的 TCP / QUIC / TLS 字段。`pkg/ebpf/bdna_profile_updater.go` 负责：

1. 读取 registry 文件。
2. 做 schema、唯一性和默认画像校验。
3. 将 profile 写入 `fingerprint_map`。
4. 暴露当前 registry 的 profile ID 列表给 G-Switch 等联动方使用。

### 指纹模板当前覆盖的字段

`stack_fingerprint` 当前已经稳定包含三类参数：

1. TCP：
   - `tcp_window`
   - `tcp_wscale`
   - `tcp_mss`
   - `tcp_sack_ok`
   - `tcp_timestamps`
2. QUIC：
   - `quic_max_idle`
   - `quic_max_data`
   - `quic_max_streams_bi`
   - `quic_max_streams_uni`
   - `quic_ack_delay_exp`
3. TLS：
   - `tls_version`
   - `tls_ext_order`
   - `tls_ext_count`

这意味着 B-DNA 当前不是抽象的“浏览器拟态”口号，而是已经有一份明确落到 eBPF 侧的数据结构。

### 当前已经明确区分的两类控制面

本轮收口之后，B-DNA 相关控制面不再被混写成一份模糊的“DNA map”。

当前已经明确分成两条语义不同的链路：

1. 握手 / 指纹画像控制面：
   - `fingerprint_map`
   - `active_profile_map`
   - `configs/bdna/profile-registry.v1.json`
   - `pkg/ebpf/bdna_profile_updater.go`
2. 跨协议协同模板控制面：
   - `dna_template_map`
   - `pkg/ebpf/dna_updater.go`
   - `pkg/ebpf/persona_updater.go`

这意味着当前 B-DNA 的“握手画像主权”和 `dna_template_map` 的“跨协议协同模板语义”已经被正式拆开。

### DNA 模板更新链路

`pkg/ebpf/dna_updater.go` 当前已经稳定提供 `UpdateDNATemplate()`，向 `dna_template_map` 写入：

- `TargetIATMu`
- `TargetIATSigma`
- `PaddingStrategy`
- `TargetMTU`
- `BurstSize`
- `BurstInterval`

这条链路说明 `dna_template_map` 当前仍然与 B-DNA 有强耦合来源，但它承载的已经不是纯握手画像，而是：

- 时域参数
- 包长目标
- 突发模式

也就是一条跨 `B-DNA` / `Jitter-Lite` / `NPM` 的协同模板链路。

但从主权边界上看：

- `dna_template_map` 更接近跨协议协同模板。
- `active_profile_map` / `fingerprint_map` 才是 B-DNA 当前最直接的握手指纹锚点。

## 当前有效的数据面语义

### TCP SYN 重写

`bdna_tcp_rewrite` 当前已经稳定实现：

1. 只处理 IPv4 TCP SYN。
2. 从 `active_profile_map` 读取当前 profile。
3. 从 `fingerprint_map` 读取具体模板。
4. 精准扫描 TCP options。
5. 重写 MSS 与窗口缩放。
6. 重写 TCP Window 并同步修正校验和。
7. 记录 `tcp_rewritten` 统计。

这说明 B-DNA 当前最成熟、最直接落地的能力，是 TCP 握手初始外观重写。

### QUIC Initial 协同重写

`bdna_quic_rewrite` 当前已经稳定实现：

1. 识别 UDP/443 的 QUIC Initial。
2. 校验其长包头与版本特征。
3. 读取当前 active profile。
4. 不在内核里做完整重写，而是通过 `skb->mark` 标记给后续用户态链路。
5. 记录 `quic_rewritten` 统计。

因此，B-DNA 当前对 QUIC 的正式语义不是“内核里已完整克隆所有 transport parameters”，而是：

- 内核已完成识别与改写触发。
- 复杂重写仍依赖用户态协作。

### TLS ClientHello 协同重写

`bdna_tls_rewrite` 当前已经稳定实现：

1. 识别 TCP payload 中的 TLS ClientHello。
2. 读取当前 active profile。
3. 通过 `skb->mark` 建立用户态协同重写入口。
4. 记录 `tls_rewritten` 统计。

因此，B-DNA 当前对 TLS 的正式语义同样应理解为：

- 已有稳定识别和协同触发链路。
- 尚不是“所有扩展顺序都已经在 eBPF 中完整改写”。

### JA4 捕获链路

`bdna_ja4_capture` 当前已经稳定实现：

1. 捕获 TLS ClientHello。
2. 构造简化版 JA4 指纹。
3. 写入 `ja4_cache`。
4. 通过 `ja4_events` 上报事件。

这说明 B-DNA 当前不仅负责“伪装”，也负责“识别现在暴露成了什么样子”。

### 统计面

`bdna_stats_map` 当前已经稳定记录：

- `tcp_rewritten`
- `quic_rewritten`
- `tls_rewritten`
- `skipped`

这意味着 B-DNA 当前已同时拥有：

1. 指纹行为变更语义。
2. 指纹路径观测语义。

## 当前有效的分析与处置链路

### Cortex 高风险分析

`pkg/cortex/analyzer.go` 当前已经稳定提供：

- 指纹与 IP 关联
- 高危缓存
- 自动提分
- 高危 / 封禁状态推进

从协议语义上看，这意味着 B-DNA 已经不是“只改包、不看后果”的协议，而是带有一条指纹风险分析闭环。

### 高风险指纹上报

`pkg/cortex/fingerprint_reporter.go` 当前已经稳定把高危指纹转换成 `ThreatBus` 高严重度事件。

这说明 B-DNA 当前与整体威胁感知系统已经发生正式联动，而不是只在局部模块里自转。

### G-Switch 压力联动

`pkg/gswitch/manager.go` 与 `pkg/gswitch/autonomous.go` 当前都已经稳定存在“JA4 暴露 -> 切换 B-DNA 握手画像”的联动语义：

1. 域名战死频率过高可触发 B-DNA reset。
2. JA4 指纹暴露可联动切换并重置模板。
3. 当前 reset 动作通过 B-DNA 画像切换器写入 `active_profile_map`，其可选 profile 集优先来自已加载 registry。

这说明 B-DNA 当前已经参与“入口逃逸压力导致指纹画像重置”的跨协议联动。

## 当前有效的协议边界

B-DNA 当前应该被理解成“指纹 / 握手协议”，而不是以下任何一种东西：

- 不是包长协议。
- 不是时域协议。
- 不是背景噪声协议。
- 也不是一整套已完备的浏览器运行时栈替身。

它真正拥有的是：决定链路在握手参数、栈指纹和 JA4 外显上应该如何表现。

## 与 NPM / Jitter-Lite / VPC 的边界

| 协议 | 主权 |
|------|------|
| `B-DNA` | TCP/TLS/QUIC 握手外观、指纹模板、JA4 暴露与联动处置 |
| `NPM` | 包长与尾部形态 |
| `Jitter-Lite` | IAT、节奏与发送时序 |
| `VPC` | 背景噪声与威胁环境外观 |

当前关系应理解为：

- B-DNA 与 Jitter-Lite 通过 `dna_template_map` 发生协同。
- B-DNA 与 G-Switch 通过 JA4 暴露与 reset 发生联动。
- B-DNA 不拥有包长主权，那属于 NPM。

## 本轮明确采纳的内容

以下旧文档语义，已经被新目录明确接管：

- B-DNA 的主职责是指纹与握手行为伪装。
- 当前 TCP SYN 重写是最成熟的数据面能力。
- QUIC / TLS 当前已经有稳定的识别与用户态协同重写入口。
- JA4 捕获、分析、上报与 reset 联动已经构成一条真实存在的闭环。

## 本轮明确不采纳为当前有效语义的内容

以下内容在旧文档中出现，但本轮不作为正式语义接管：

- 旧文档中的完整全球浏览器指纹库作为当前已固化事实
- 用动态 TLS/QUIC/HTTP2 栈完整克隆浏览器行为作为当前已落地事实
- 所有未在当前代码锚点中收敛的 ALPN、HTTP/2 SETTINGS 和请求序列拟态细节
- 旧文档中的匹配度评分和性能开销表

这些内容要么仍偏设想，要么没有形成当前单一主源。

## 与旧文档的关系

- 旧 `docs/03-自研协议/B-DNA-行为识别协议.md` 继续保留，但默认视为解释性输入材料。
- 后续若需要继续扩充画像库、发布新的 registry 版本或调整默认画像，应优先回写本文档、`configs/bdna/profile-registry.v*.json` 和对应运行时实现，而不是继续扩写旧文档。
