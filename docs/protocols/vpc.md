# VPC

本文件是 VPC 的当前有效协议语义入口。它不是把旧 `VPC-噪声注入协议.md` 原样搬过来，而是把当前已经由代码承载的“背景噪声 + 威胁自适应”语义抽取出来。

## 协议定位

VPC 是 ReMirage 的背景噪声与威胁自适应协议，负责让链路在外显行为上更接近真实网络环境，同时在威胁压力升高时联动入口处置、时序扰动和噪声注入。

它当前稳定承担四类职责：

1. 将噪声参数写入 eBPF 数据面。
2. 以入口威胁与状态机驱动噪声和处置强度变化。
3. 为时域扰动、背景噪声和异常隔离提供联动上下文。
4. 作为伪装扰动层中“背景维度”的正式语义入口。

## 主真相归属

- 协议语义入口：本文档
- 运行时主锚点：
  - `mirage-gateway/pkg/ebpf/*`
  - `mirage-gateway/pkg/threat/*`
  - `mirage-gateway/pkg/cortex/*`
  - `mirage-gateway/bpf/jitter.c`
- 控制参数入口：
  - `mirage-gateway/pkg/api/handlers.go`

## 为什么 VPC 是跨层协议

和 `G-Tunnel`、`G-Switch` 不同，VPC 当前不是一个收敛在单目录里的模块。

它的当前有效语义横跨三层：

| 层级 | 当前职责 |
|------|----------|
| 感知层 | 感知威胁、异常、入口压力与行为风险 |
| 控制层 | 将威胁等级、噪声强度、入口状态映射为策略 |
| 数据面 | 通过 eBPF map 和 TC 程序施加延迟、噪声、丢弃与统计 |

所以当前 VPC 的正式语义不能只看某一个目录，必须以“跨层联动协议”理解。

## 当前有效的配置语义

### VPCConfig

当前写入数据面的核心结构已经稳定存在于 `mirage-gateway/pkg/ebpf/types.go`：

- `Enabled`
- `FiberJitterUs`
- `RouterDelayUs`
- `NoiseIntensity`

这说明当前 VPC 的最小协议面已经不是抽象的“噪声注入”，而是三个明确参数：

1. 光缆抖动量
2. 路由器延迟量
3. 总体噪声强度

### DefenseStrategy 到 VPC 的映射

当前控制面不会单独下发一个“VPC 命令”，而是通过 `DefenseStrategy` 统一写入：

- `JitterMeanUs`
- `JitterStddevUs`
- `FiberJitterUs`
- `RouterDelayUs`
- `NoiseIntensity`

其中 VPC 部分最终落到 `vpc_config_map`。

这条链路当前由以下位置承载：

- `mirage-gateway/pkg/api/handlers.go`
- `mirage-gateway/pkg/ebpf/loader.go`
- `mirage-gateway/pkg/ebpf/manager.go`

## 当前有效的控制链路

### 1. OS / 控制面参数下发

当前 `PushStrategy` 已经稳定携带与 VPC 相关的参数：

- `NoiseIntensity`
- `FiberJitterUs`
- `RouterDelayUs`

即使 legacy 分支里暂时没有把所有字段都完全用满，协议入口已经清楚了：VPC 当前是通过统一策略下发链路进入网关的。

### 2. eBPF Map 写入

`Loader.UpdateStrategy()` 当前已经明确执行：

1. 写入 `jitter_config_map`
2. 写入 `vpc_config_map`
3. 采用 write-all-or-rollback，确保多 map 更新时的一致性

这意味着当前 VPC 与 Jitter-Lite 之间是“共同更新、共同生效”的关系，而不是两套互相独立的配置平面。

### 3. Persona 双 Slot 联动

当前 `PersonaMapUpdater` 也会把 VPC 参数写入 `vpc_config_map` 的 shadow slot，并参与原子 flip。

这意味着在当前实现里，VPC 还承担一层“入口画像切换”的语义，不只是运行时即时调参。

## 当前有效的数据面语义

### Map 母体与程序挂载

当前 `jitter.o` 是共享 Map 的母体程序，负责创建包括 `vpc_config_map` 在内的共享 map，并挂载：

- `jitter_lite_egress`
- `vpc_ingress_detect`

因此，VPC 当前并不是一个单独 `vpc.o` 程序，而是和 Jitter-Lite 共处在同一个数据面母体里。

### 背景噪声模型

`mirage-gateway/bpf/jitter.c` 当前已经存在一套增强版 VPC 噪声模型，主要包括：

- `vpc_noise_profile`
- `active_noise_profile`
- `vpc_noise_stats`

以及以下物理噪声构件：

- 光缆抖动模拟
- 路由器队列延迟模拟
- 跨洋光缆周期特征
- 数据中心微突发抖动
- 乱序与丢包模拟

从协议语义上看，当前 VPC 已经不只是“加点随机延迟”，而是把背景网络特征作为一类可配置的伪装对象。

### 数据面输出

当前数据面至少已经稳定输出两类结果：

1. 对 `skb->tstamp` 的延迟调整。
2. 对 `vpc_noise_stats` 的噪声统计。

这说明 VPC 的协议结果既包括“行为改变”，也包括“观测结果”。

## 当前有效的威胁联动语义

### Threat -> FSM

当前 `threat.SecurityFSM` 会根据：

- `ThreatLevel`
- `RejectRate`
- `ControlPlaneDown`

把节点状态迁移到：

- `Normal`
- `Alert`
- `HighPressure`
- `Isolated`
- `Silent`

虽然这部分不是 VPC 独占，但它构成了 VPC 当前的上游压力语义。

### FSM -> IngressPolicy

当前状态切换后，`IngressPolicy.ApplyStateOverride()` 会直接改变入口处置规则，例如：

- 收紧 throttle
- 主动 drop
- 仅白名单放行
- 最小暴露

这意味着 VPC 当前不仅影响“链路长什么样”，也影响“入口对流量怎么处置”。

### Threat / Cortex -> Jitter

`cortex.QuarantineManager` 当前已经存在针对隔离对象的抖动注入能力：

- 为隔离对象生成随机 jitter delay
- 记录 jitter 次数
- 通过观察期决定晋升或释放

这条链路说明当前 VPC 和 Jitter-Lite 在实践里已经通过 `Cortex` 侧的异常观察机制发生联动。

## 当前有效的协议边界

VPC 当前应该被理解成“背景维度协议”，而不是以下任何一种东西：

- 不是单独的内核模块。
- 不是单独的威胁检测器。
- 不是单独的入口策略引擎。
- 也不是单纯的 jitter 参数集合。

它真正拥有的是：把威胁上下文映射成背景噪声、入口收敛和外显行为变化。

## 本轮明确采纳的内容

以下旧文档语义，已经被新目录明确接管：

- VPC 的主职责是“背景噪声与威胁自适应”，不是泛化的神秘协议名词。
- 当前 VPC 与 Jitter-Lite 在数据面上高度耦合，共享 map 母体和策略更新链路。
- 当前 VPC 的核心参数是 `FiberJitterUs`、`RouterDelayUs`、`NoiseIntensity`。
- 当前 VPC 的控制结果既会进 eBPF map，也会通过 `SecurityFSM` / `IngressPolicy` 改变入口处置强度。

## 本轮明确不采纳为当前有效语义的内容

以下内容在旧文档中出现，但本轮不作为正式语义接管：

- `TCPMonitor` / `kprobe tcp_retransmit` 那套完整检测链作为当前已落地事实
- 旧文档中的精细威胁分类阈值表
- 所有未在现有运行时锚点中收敛的“选择性丢包识别”细节
- 把 VPC 说成一个完全独立、边界封闭的协议模块

这些内容要么仍偏设想，要么还没有形成单一稳定主源。

## 与旧文档的关系

- 旧 `docs/03-自研协议/VPC-噪声注入协议.md` 继续保留，但默认视为解释性输入材料。
- 后续若需要补更细的 map 字段、检测语义或 profile 语义，应优先回写本文档和对应运行时实现，而不是继续扩写旧文档。
