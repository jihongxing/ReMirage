# Protocol Stack

本文件定义 ReMirage 协议栈的首轮正式分层，用来替代旧 `协议协同矩阵.md` 与 `语言分工架构.md` 中混杂的总览性描述。

## 分层原则

ReMirage 的协议栈按“控制契约、传输承载、伪装扰动、回退承载”四层理解。

| 层级 | 作用 | 主要主源 |
|------|------|----------|
| 控制契约层 | 定义 Gateway、OS、Client 之间交换的控制和状态语义 | `mirage-proto/*.proto` |
| 传输承载层 | 定义数据如何分片、纠错、重组、切换与回退 | `mirage-gateway/pkg/gtunnel/*`、`phantom-client/pkg/gtclient/*` |
| 伪装扰动层 | 定义包长、指纹、时序、背景噪声等伪装和扰动能力 | `mirage-gateway/bpf/*` 与相关控制代码 |
| 回退承载层 | 定义在受限环境中可启用的替代传输方式 | `mirage-gateway/pkg/gtunnel/*`、`phantom-client/pkg/resonance/*` |

## 协议族划分

### 1. 控制与信令协议族

这一层不是“某个六大协议之一”，而是 ReMirage 内部所有跨组件共享语义的底座。

当前首要入口：

- `mirage-proto/mirage.proto`
- `mirage-proto/control_command.proto`

它们负责定义：

- Gateway 与 OS 的上行/下行消息。
- 策略、配额、转生、会话事件等共享控制语义。
- 后续命令总线需要保持兼容的消息边界。

### 2. 传输承载协议族

传输承载协议族负责“连接怎样活下去、怎样在多路径下持续工作”。

当前主协议是 `G-Tunnel`，它覆盖：

- 多路径发送与路径选择。
- FEC 与重组。
- 会话级恢复与降级。
- WebRTC、DNS、ICMP 等替代承载的接入点。

当前运行时锚点主要在：

- `mirage-gateway/pkg/gtunnel/*`
- `phantom-client/pkg/gtclient/*`

正式语义入口：

- `docs/protocols/gtunnel.md`

### 3. 存活与切换协议族

这一层负责入口变化、域名转生、切换和逃逸动作。当前主协议是 `G-Switch`。

它与传输承载层相关，但不拥有数据承载主权。它负责：

- 转生指令的消费与执行。
- DNS-less、域名切换、逃逸路径协同。
- 与控制层的转生信令对接。

当前运行时锚点主要在：

- `mirage-gateway/pkg/gswitch/*`
- `mirage-proto/mirage.proto` 中的 `ReincarnationPush`

正式语义入口：

- `docs/protocols/gswitch.md`

### 4. 伪装与扰动协议族

这一层负责降低链路行为被固定识别的概率。它不是单一协议，而是一组协同能力：

| 协议 | 主要目标 |
|------|----------|
| `NPM` | 包长与流量形态伪装 |
| `B-DNA` | 指纹与握手行为伪装 |
| `Jitter-Lite` | 时序扰动与节奏拟态 |
| `VPC` | 背景噪声与威胁自适应注入 |

这组协议的共同特点是：

- 经常由 eBPF 或网关侧控制逻辑共同实现。
- 参数可由控制层下发，但行为主权不在文档。
- 很容易与策略、实现、性能假设混写，因此必须和控制契约分开。

当前正式语义入口：

- `docs/protocols/npm.md`
- `docs/protocols/bdna.md`
- `docs/protocols/jitter-lite.md`
- `docs/protocols/vpc.md`

### 5. 回退承载族

当主承载不可用时，协议栈允许落入回退承载，包括：

- WebRTC
- DNS Tunnel
- ICMP Tunnel

这部分当前更接近 `G-Tunnel` 族的承载分支，而不是完全独立的顶层协议。

## 当前协同规则

1. 控制契约层负责发出共享语义，不直接规定具体算法。
2. 传输承载层负责连接连续性。
3. 存活与切换层负责入口和路径切换。
4. 伪装扰动层负责外显行为控制。
5. 回退承载族只在主承载不足时接管部分能力。

## 首轮迁移后的边界收敛

这次迁移先做了三个收敛：

1. 旧 `协议协同矩阵` 不再作为长期主源，它的协同关系被压缩进本文件。
2. 旧 `语言分工架构` 不再作为长期主源，语言与实现归属只在需要时作为实现说明出现。
3. 单协议的运行时主权统一转到各自的代码或 `.proto`，而不是继续停留在旧文档中。
