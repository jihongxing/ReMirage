# G-Tunnel

本文件是 G-Tunnel 的当前有效协议语义入口。它不是旧 `G-Tunnel-多路径传输协议.md` 的整篇搬运，而是从现有实现中抽取仍然有效、已经被代码承载的协议语义。

## 协议定位

G-Tunnel 是 ReMirage 的主传输承载协议，负责把客户端与网关之间的连接维持成一条“可恢复、可降级、可切换”的连续链路。

它当前稳定承担四类职责：

1. 数据分片、纠错、重组。
2. 主承载与回退承载的统一抽象。
3. 多路径或多通道之间的调度与升降级。
4. 断连后的分阶段恢复。

## 主真相归属

- 协议语义入口：本文档
- 运行时主锚点：
  - `phantom-client/pkg/gtclient/*`
  - `mirage-gateway/pkg/gtunnel/*`
- 共享控制语义：
  - `mirage-proto/mirage.proto`
  - `mirage-proto/control_command.proto`

## 当前有效的协议结构

### 1. 发送流水线

客户端当前稳定存在的发送流水线是：

`IP packet -> overlap split -> FEC encode -> shard header -> encrypt -> transport send`

对应实现可见于 `phantom-client/pkg/gtclient/client.go`。

### 2. 接收流水线

接收侧当前稳定存在的还原流水线是：

`transport recv -> decrypt -> shard ingest -> FEC decode -> overlap reassemble -> IP packet`

对应实现可见于：

- `phantom-client/pkg/gtclient/reassembler.go`
- `phantom-client/pkg/gtclient/sampler.go`

### 3. 统一承载抽象

G-Tunnel 当前以统一 `TransportConn` 抽象管理承载类型。已经登记在代码中的承载类型包括：

| 承载类型 | 作用 |
|----------|------|
| `QUIC` | 主通道 |
| `WebSocket` | TCP 降级通道 |
| `WebRTC` | 受限环境下的回退承载 |
| `ICMP` | 极限回退承载 |
| `DNS` | 求生通道 |

这层抽象当前定义在 `mirage-gateway/pkg/gtunnel/transport.go`。

## 当前有效的报文语义

### Overlap Fragment

客户端的 overlap sampler 当前采用以下默认参数：

- `ChunkSize = 400`
- `OverlapSize = 100`

分片语义当前由 `phantom-client/pkg/gtclient/sampler.go` 定义。

每个 fragment 至少携带：

- `SeqNum`
- `Data`
- `OverlapID`

其中 `OverlapID` 当前通过对原始数据做 `CRC32` 计算得到，用于同一包片段归组与一致性校验。

### Shard Header

FEC 分片进入线传输前，当前会附加一个 12 字节 header，格式为：

| 字段 | 长度 |
|------|------|
| `seqNum` | 2B |
| `dataLen` | 2B |
| `overlapID` | 4B |
| `shardIndex` | 2B |
| `fragCount` | 2B |

这一定义当前由 `phantom-client/pkg/gtclient/reassembler.go` 中的 `EncodeShardHeader` / `DecodeShardMeta` 承担。

### FEC 语义

当前客户端默认使用：

- `dataShards = 8`
- `parityShards = 4`

这意味着接收端只要达到数据分片阈值，就会尝试重建缺失分片，再进入 overlap reassemble。

## 当前有效的状态与恢复语义

### 连接状态

客户端当前登记的连接状态包括：

- `Init`
- `Bootstrapping`
- `Connected`
- `Suspicious`
- `Degraded`
- `Reconnecting`
- `Exhausted`
- `Stopped`

状态定义位于 `phantom-client/pkg/gtclient/state.go`。

### 退化等级

G-Tunnel 当前有一套清晰的三级退化语义：

| 等级 | 含义 |
|------|------|
| `L1_Normal` | 使用运行时拓扑池 |
| `L2_Degraded` | 回退到 bootstrap pool |
| `L3_LastResort` | 进入 resonance 绝境发现 |

### 恢复阶段

恢复状态机当前按断连时长分三阶段执行：

| 阶段 | 含义 | 当前动作 |
|------|------|----------|
| `Jitter` | 主链路短抖动 | 在当前节点上快速重试 |
| `Pressure` | 节点受压 | 触发拓扑刷新并尝试同优先级切换 |
| `Death` | 节点死亡 | 执行完整的 L1 -> L2 -> L3 降级 |

对应实现位于 `phantom-client/pkg/gtclient/recovery_fsm.go`。

## 当前有效的拓扑与切换语义

### 双池分离

G-Tunnel 当前已经稳定采用“双池分离”：

- `bootstrapPool`
  来自 token/URI，生命周期内只读。
- `runtimeTopo`
  来自动态拉取，可更新。

恢复时优先使用运行时拓扑，再回退到启动种子池，最后进入共振发现。

### 事务式切换

客户端当前切换连接时采用事务式顺序：

1. 预加新路由。
2. 接管新连接。
3. 提交切换并清理旧路由。

这比旧文档中泛化的“热切换”更接近当前有效实现。

## 当前有效的调度语义

网关侧当前存在两类调度结构：

### PathScheduler

`mirage-gateway/pkg/gtunnel/multipath.go` 中的 `PathScheduler` 已经稳定暴露以下策略：

- `round-robin`
- `lowest-rtt`
- `redundant`

它当前关注的是路径选择和发送，而不是旧文档中那套完整的理想化评分体系。

### Orchestrator

`mirage-gateway/pkg/gtunnel/orchestrator.go` 中的 `Orchestrator` 当前已经明确承载：

- 协议优先级
- 探测与升降级
- 双发选收的 epoch 语义
- 不同回退承载的启用开关

当前优先级从高到低是：

`QUIC -> WebRTC -> WSS -> ICMP/DNS`

## 本轮明确采纳的内容

以下旧文档语义，已经被新目录明确接管：

- 多路径并不是抽象口号，而是分片、FEC、重组、路径调度和恢复流程的组合。
- 回退承载不是“额外协议集合”，而是 G-Tunnel 统一承载抽象中的分支。
- 恢复不再用笼统的“自愈”描述，而是以 `Jitter / Pressure / Death` 三阶段来表达。

## 本轮明确不采纳为当前有效语义的内容

以下内容在旧文档中存在，但本轮没有作为当前正式语义接管，因为它们尚未形成稳定主源：

- `BBR v3` 作为正式协议语义
- `AVX-512` 优化作为协议级承诺
- 旧文档中的大段性能指标表
- 所有愿景化的“五种协议并存效果”表述

这些内容要么属于实现细节，要么尚未形成稳定的运行时约束。

## 与旧文档的关系

- 旧 `docs/03-自研协议/G-Tunnel-多路径传输协议.md` 继续保留，但默认视为输入材料。
- 后续若需要迁移更细的报文或状态字段，应优先回写本文档或对应运行时实现，而不是继续扩写旧文档。
