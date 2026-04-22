# NPM

本文件是 NPM 的当前有效协议语义入口。它不再沿用旧 `NPM-流量伪装协议.md` 中“协议拟态库 + 理想化分布矩阵 + 性能表”的写法，而是只接管已经在当前代码中稳定存在的包长与形态伪装语义。

## 协议定位

NPM 是 ReMirage 的包长与流量形态伪装协议，负责在不改变上层业务语义的前提下，改变链路外显的长度特征、尾部形态和部分噪声包行为。

它当前稳定承担四类职责：

1. 在 XDP 入口统一执行入站剥离与出站填充。
2. 以概率控制和目标 MTU 控制包长扩展。
3. 为诱饵包和隐写就绪信号提供一个附属出口。
4. 作为伪装扰动层中“长度 / 形态维度”的正式语义入口。

## 主真相归属

- 协议语义入口：本文档
- 运行时主锚点：
  - `mirage-gateway/bpf/npm.c`
  - `mirage-gateway/bpf/common.h`
  - `mirage-gateway/pkg/ebpf/persona_updater.go`
  - `mirage-gateway/pkg/ebpf/types.go`
  - `mirage-gateway/pkg/mcc/signal.go`
  - `mirage-gateway/pkg/api/handlers.go`
- 相关控制与装载锚点：
  - `mirage-gateway/pkg/ebpf/loader.go`
  - `mirage-gateway/pkg/nerve/nerve_splice.go`

## 当前有效的配置语义

### 统一后的主配置面

当前 NPM 已经明确统一为：

- `enabled`
- `filling_rate`
- `global_mtu`
- `min_packet_size`
- `padding_mode`
- `decoy_rate`

它们现在由 `mirage-gateway/bpf/common.h` 中的 `struct npm_config` 与 `npm_config_map` 共同承载，`mirage-gateway/bpf/npm.c` 直接读取这套统一定义。

从协议视角看，当前 NPM 的最小正式配置面已经清楚收敛到：

1. 是否启用。
2. 填充概率。
3. 目标 MTU。
4. 小包跳过阈值。
5. 填充模式。
6. 诱饵包比例。

### 控制面当前已统一到同一结构

本轮收口之后，NPM 不再并存 `npm_global_map` / `npm_config_map` 两套主配置命名。

当前有效事实已经变成：

1. 数据面统一读取 `npm_config_map`。
2. Go 侧统一写入 `NPMConfig`。
3. 默认参数通过共享 helper 收敛，而不是在不同调用点各写一套匿名结构。

### Strategy / Persona / MCC 到 NPM 的接入

当前 Go 侧至少已经稳定暴露出三条接入路径：

1. `pkg/api/handlers.go` 的 `DesiredStatePayload.PaddingRate`。
2. `pkg/ebpf/persona_updater.go` 的 `PersonaParams.NPM` 与 `NPMConfig`。
3. `pkg/mcc/signal.go` 的 `syncNPMConfig()`。
4. `pkg/nerve/nerve_splice.go` 的 `MotorDownlink.ApplyDesiredState()`。

其中统一默认参数当前由：

- `mirage-gateway/pkg/ebpf/types.go` 中的 `NewDefaultNPMConfig()`

负责生成。

从语义上看，这三条链路共同说明：

- NPM 已经不是“只存在于 eBPF 里的孤立功能”。
- 它已经被纳入统一策略、画像切换和匿名信令调优。
- 且当前主配置命名和字段含义已经完成一轮统一。

## 当前有效的数据面语义

### 单一 XDP 挂载入口

当前 `npm_xdp_main` 是 NPM 的唯一 XDP 挂载入口，并按固定顺序执行：

1. `handle_l1_defense()`
2. `handle_npm_strip()`
3. `handle_npm_padding()`

这意味着 NPM 当前不是一个孤立的“纯 padding 程序”，而是嵌在入口纵深防御之后、与主入口共居的数据面协议。

### 入站剥离

`handle_npm_strip()` 当前已经稳定实现：

1. 解析以太网与 IPv4 头。
2. 计算帧长度与 IP 长度差。
3. 若检测到尾部填充，则用 `bpf_xdp_adjust_tail()` 直接剥离。

这说明 NPM 当前不仅负责“加 padding”，也负责把额外尾部长度在入口恢复掉。

### 出站填充

`handle_npm_padding()` 当前已经稳定实现：

1. 读取全局配置。
2. 统计总包数。
3. 仅处理 IPv4。
4. 跳过小于 `min_packet_size` 的控制性小包。
5. 按 `filling_rate` 做概率决策。
6. 根据 `padding_mode` 计算 padding 大小。
7. 通过 `bpf_xdp_adjust_tail()` 扩展尾部。
8. 更新 `ip->tot_len` 和 IP 校验和。
9. 写入填充统计。

这条链路定义了 NPM 当前最核心的正式动作：

- 改变包长外观。
- 保持 IPv4 长度和校验和自洽。
- 在 XDP 层完成高频数据面操作。

### 当前已经落地的三种填充模式

`calculate_padding()` 当前明确支持：

- `NPM_MODE_FIXED_MTU`
- `NPM_MODE_RANDOM_RANGE`
- `NPM_MODE_GAUSSIAN`

它们对应的正式语义分别是：

1. 固定对齐到目标 MTU。
2. 在剩余空间内随机填充。
3. 用简化正态扰动靠近目标区间。

旧文档中的更多理想化分布模型，本轮不作为正式语义接管。

### 统计面

`npm_stats_map` 当前已经稳定记录：

- `total_packets`
- `padded_packets`
- `padding_bytes`
- `decoy_packets`
- `skipped_packets`

这意味着 NPM 当前已经同时拥有：

1. 行为改变语义。
2. 观测与回报语义。

## 当前有效的附属语义

### 诱饵包标记

`npm_decoy_marker` 当前已经稳定表明，NPM 不只有“包长补齐”这一件事，还承担一条附属噪声出口：

1. 依据 `decoy_rate` 做概率决策。
2. 通过 `skb->mark` 标记诱饵包。
3. 向 `stego_ready_map` 发布隐写就绪事件。
4. 在 `stego_command_map` 有效时执行载荷替换。

这条链路目前已经是有效实现，但它仍然是 NPM 的附属能力，不应反向吞并 NPM 的主定义。

### MTU 探测

`npm_mtu_probe` 当前还定义了一条辅助链路：

- 监听 ICMP Fragmentation Needed。
- 更新 `mtu_probe_map`。
- 通过 `mtu_events` 上报探测结果。

这说明 NPM 当前具备“围绕目标包长做动态探测”的现实能力，但它是辅助能力，不是协议主权本身。

## 当前有效的协议边界

NPM 当前应该被理解成“长度 / 形态协议”，而不是以下任何一种东西：

- 不是 TLS 指纹协议。
- 不是时域扰动协议。
- 不是背景噪声协议。
- 也不是泛化的所有伪装能力总称。

它真正拥有的是：决定链路在包长与尾部形态上应该如何表现。

## 与 B-DNA / Jitter-Lite / VPC 的边界

| 协议 | 主权 |
|------|------|
| `NPM` | 包长、尾部填充、长度外观、诱饵包附属出口 |
| `B-DNA` | TCP/TLS/QUIC 握手与指纹外观 |
| `Jitter-Lite` | 时域节奏、IAT 与发送时机 |
| `VPC` | 背景噪声、压力联动与入口环境外观 |

当前关系应理解为：

- NPM 与 Jitter-Lite 在数据面上存在直接协同。
- NPM 与 VPC 同属伪装扰动层，但主权不同。
- NPM 不拥有握手指纹语义，那是 B-DNA 的职责。

## 本轮明确采纳的内容

以下旧文档语义，已经被新目录明确接管：

- NPM 的主职责是包长与流量形态伪装。
- 当前 NPM 已经存在单一 XDP 入口、入站剥离、出站填充和统计链路。
- `filling_rate`、`global_mtu`、`min_packet_size`、`padding_mode` 已经是稳定配置锚点。
- 诱饵包与隐写就绪信号当前已经是 NPM 的附属实现语义。

## 本轮明确不采纳为当前有效语义的内容

以下内容在旧文档中出现，但本轮不作为正式语义接管：

- 旧文档里的完整协议画像库与目标协议分布矩阵
- 用户态批处理 padder 作为当前主实现事实
- 所有未在当前运行时代码中稳定固化的“协议头部拟态”细节
- 旧文档中的性能指标表和隐蔽性评分表

这些内容要么仍偏设想，要么没有形成当前单一主源。

## 与旧文档的关系

- 旧 `docs/03-自研协议/NPM-流量伪装协议.md` 继续保留，但默认视为解释性输入材料。
- 后续若需要继续细化 NPM 的 profile 选择、诱饵策略或 MTU 自适应，应优先回写本文档和对应运行时实现，而不是继续扩写旧文档。
