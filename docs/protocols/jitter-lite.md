# Jitter-Lite

本文件是 Jitter-Lite 的当前有效协议语义入口。它不再沿用旧 `Jitter-Lite-时域扰动协议.md` 中“全球拟态矩阵 + 大量理想化模板”的叙述方式，而是从当前实现里抽取已经稳定落地的时域语义。

## 协议定位

Jitter-Lite 是 ReMirage 的时域扰动协议，负责通过控制包间到达节奏、时间戳和时段画像，让链路在时间维度上更接近真实应用流量。

它当前稳定承担四类职责：

1. 通过 `skb->tstamp` 实现发送侧时域延迟控制。
2. 以 `jitter_config_map` / `dna_template_map` 驱动时域参数。
3. 为区域画像、社交时钟和拟态权重提供上层语义锚点。
4. 与 VPC 共用数据面母体，但拥有独立的“时域主权”。

## 主真相归属

- 协议语义入口：本文档
- 运行时主锚点：
  - `mirage-gateway/bpf/jitter.c`
  - `mirage-gateway/pkg/jitter/*`
  - `mirage-gateway/pkg/ebpf/*`
- 控制参数入口：
  - `mirage-gateway/pkg/api/handlers.go`

## 当前有效的配置语义

### JitterConfig

当前 Jitter-Lite 最小配置面已经稳定存在于 `mirage-gateway/pkg/ebpf/types.go`：

- `Enabled`
- `MeanIATUs`
- `StddevIATUs`
- `TemplateID`

这意味着当前 Jitter-Lite 的正式配置语义已经明确收敛到：

1. 平均时延间隔
2. 时延标准差
3. 当前模板编号

### DefenseStrategy 到 Jitter 的映射

当前控制面不会单独下发一个“Jitter-Lite 命令”，而是通过 `DefenseStrategy` 统一写入：

- `JitterMeanUs`
- `JitterStddevUs`
- `TemplateID`

最终落到：

- `jitter_config_map`

这条链路当前由以下位置承载：

- `mirage-gateway/pkg/api/handlers.go`
- `mirage-gateway/pkg/ebpf/loader.go`
- `mirage-gateway/pkg/ebpf/manager.go`

### DNA 模板协同

Jitter-Lite 当前还有一条更高优先级的时域协同入口：

- `dna_template_map`

在 `jitter_lite_egress` 中，数据面会优先读取 `dna_template_map`；若没有 DNA 模板，再回退到 `jitter_config_map`。

这意味着当前实现里，Jitter-Lite 已经不是“只用简单高斯采样”的协议，而是可以被 B-DNA 模板进一步接管时域行为。

## 当前有效的数据面语义

### 核心执行动作

Jitter-Lite 当前最核心的协议动作是：

`skb->tstamp = now + delay`

也就是说，它真正控制的是包在时域上的发出节奏，而不是单纯写一份配置。

### 主程序与分支程序

当前 `mirage-gateway/bpf/jitter.c` 中已经存在几条明确的时域执行路径：

- `jitter_lite_egress`
- `jitter_lite_adaptive`
- `jitter_lite_physical`
- `jitter_lite_social`

它们共同说明：当前 Jitter-Lite 不只是一个单函数协议，而是一组围绕时域控制展开的执行模式。

### jitter_lite_egress

这是当前最核心的时域主路径，已经稳定承载：

1. 配额熔断前置检查。
2. 业务流量统计。
3. DNA 模板优先读取。
4. `jitter_config_map` 回退读取。
5. 基于结果设置 `skb->tstamp`。
6. 与 NPM padding 的协同。

从协议边界看，这意味着当前 Jitter-Lite 已经和：

- 配额熔断
- B-DNA 模板
- NPM padding

发生真实耦合，但其主权仍然是“时间行为控制”。

### jitter_lite_adaptive

这条分支当前已经明确体现一个有效语义：

- Jitter-Lite 会根据流量类型选择模板，而不是所有流量都使用同一时域参数。

目前代码中已经登记了按端口和协议做粗粒度分类的能力，例如：

- `443/80`
- `22`
- 游戏端口区间

### jitter_lite_social

这条分支说明 Jitter-Lite 当前还具有“时段感知”能力：

- 根据社交时钟对 delay 做倍率调整。
- 让同一条链路在工作时段、休闲时段、睡眠时段表现不同。

### jitter_lite_physical

虽然它和 VPC 高度耦合，但这条分支也说明一个重要事实：

- 当前 Jitter-Lite 能把基础 IAT 模型与物理噪声叠加在同一条时域链路上。

这也是为什么 Jitter-Lite 和 VPC 必须并列建正式入口，而不能互相吞并。

## 当前有效的上层画像语义

### 区域画像

`mirage-gateway/pkg/jitter/regional_profiles.go` 当前已经稳定提供：

- 区域 ID
- 区域拟态权重覆盖
- TLS 指纹偏好
- 噪声域名
- NTP 服务器
- 分时段覆盖

这说明当前 Jitter-Lite 不只是“数学延迟分布”，而是带有地缘画像上下文的时域协议。

### 社交时钟

`mirage-gateway/pkg/jitter/social_clock.go` 当前已经稳定提供：

- 工作 / 休闲 / 睡眠时段
- 时段切换
- 拟态权重更新
- 噪声注入回调
- 区域配置融合

这意味着当前 Go 侧已经有一套围绕“何时该像什么流量”的上层时钟语义。

## 当前有效的协议边界

Jitter-Lite 当前应该被理解成“时域协议”，而不是以下任何一种东西：

- 不是单纯的高斯采样函数。
- 不是 `VPC` 的一个子章节。
- 不是 `B-DNA` 的附属参数。
- 也不是单纯的 padding 辅助器。

它真正拥有的是：决定链路在时间维度上应该如何表现。

## 与 VPC 的边界

Jitter-Lite 和 VPC 当前共享 `jitter.c` 这个数据面母体，但主权不同：

| 协议 | 主权 |
|------|------|
| `Jitter-Lite` | 时域节奏、IAT、时段画像、模板化 delay |
| `VPC` | 背景噪声、物理扰动、入口压力联动、噪声强度 |

两者当前关系应理解为：

- 数据面共居
- 控制面联动
- 语义主权分离

## 本轮明确采纳的内容

以下旧文档语义，已经被新目录明确接管：

- Jitter-Lite 的主职责是时域扰动，而不是笼统的“AI 统计对抗”口号。
- 当前 Jitter-Lite 已经有 `jitter_config_map`、`dna_template_map` 和 `skb->tstamp` 三个稳定锚点。
- 区域画像和社交时钟已经是 Jitter-Lite 的正式上层语义，而不只是附属想法。
- Jitter-Lite 与 VPC 虽然共享数据面母体，但不是同一个协议。

## 本轮明确不采纳为当前有效语义的内容

以下内容在旧文档中出现，但本轮不作为正式语义接管：

- 旧文档中的整套“全球拟态矩阵”名称表作为当前运行时事实
- DPI 级业务流识别作为当前已完备事实
- 所有未在当前代码锚点中固化的精细模板参数表
- 把硬件 FQ 调度直接当成当前协议的强承诺

这些内容要么仍偏设想，要么尚未形成稳定主源。

## 与旧文档的关系

- 旧 `docs/03-自研协议/Jitter-Lite-时域扰动协议.md` 继续保留，但默认视为解释性输入材料。
- 后续若需要补更细的模板语义、区域画像映射或数据面分支，应优先回写本文档和对应运行时实现，而不是继续扩写旧文档。
