# G-Switch

本文件是 G-Switch 的当前有效协议语义入口。它把旧 `G-Switch-域名转生协议.md` 里仍被现有实现承载的部分抽出来，并明确区分“逃逸切换”“共振发现”“DNS-less 劫持”三种不同职责。

## 协议定位

G-Switch 是 ReMirage 的存活与切换协议，负责在入口受损、域名战死或链路持续失败时，把系统从当前入口切换到新的可用入口。

它当前稳定承担四类职责：

1. 域名池管理。
2. 域名战死与逃逸切换。
3. eBPF SNI / 目标入口更新。
4. 与共振发现结果对接。

## 主真相归属

- 协议语义入口：本文档
- 运行时主锚点：
  - `mirage-gateway/pkg/gswitch/*`
- 共享控制语义：
  - `mirage-proto/mirage.proto` 中的 `ReincarnationPush`

## 当前有效的域名池语义

### 域名状态

当前代码中已经稳定登记的域名状态为：

| 状态 | 含义 |
|------|------|
| `DomainActive` | 当前活跃入口 |
| `DomainStandby` | 热备入口 |
| `DomainBurned` | 已战死 |
| `DomainCooling` | 冷却中 |

状态定义位于 `mirage-gateway/pkg/gswitch/manager.go`。

### 域名实体

当前域名记录至少包含：

- `Name`
- `IP`
- `Status`
- `CreatedAt`
- `LastUsed`
- `UsageCount`
- `BurnedAt`
- `BurnReason`

这说明当前 G-Switch 的主语义不是“抽象域名轮转”，而是对入口耗材进行状态化管理。

## 当前有效的逃逸语义

### TriggerEscape

当前最核心的切换动作由 `TriggerEscape(reason string)` 承担，执行顺序已经比较明确：

1. 记录一次战死事件。
2. 检查近期战死频率。
3. 将当前活跃域名标记为 `burned`。
4. 从 `standbyPool` 选取新的热备域名。
5. 激活新域名并更新 eBPF SNI 映射。
6. 在高战死频率下触发一次 `B-DNA Reset`。

这套动作比旧文档里的“提案、投票、广播、热切换”更能代表当前已落地的协议行为。

### 战死频率与联动

G-Switch 当前会在一个时间窗口内记录战死次数。若达到阈值，会联动触发 `B-DNA Reset`，通过切换当前握手画像降低特征固化风险。

也就是说，当前 G-Switch 与 B-DNA 的协同点已经存在，但协同边界是：

- `G-Switch` 负责判断是否需要重置。
- 当前握手画像的具体语义不由 G-Switch 定义。

## 当前有效的入口更新语义

### SNI Mapping

G-Switch 当前已经实现了面向 eBPF 的 `SNIMapping` 写入。核心字段包括：

- `OldSNI`
- `NewSNI`
- `Timestamp`
- `Active`

在当前实现里，切换后的新域名会写入 eBPF map，用于让网关侧入口更新尽量接近零中断。

### DNS-less Hijack

Linux 下当前已经有一条明确的 DNS-less 劫持链路：

1. 挂载 `sock_addr` eBPF 程序到 cgroup。
2. 通过 `gw_ip_map` 写入真实目标 Gateway IP / Port。
3. 让发往固定占位地址的流量在内核侧被透明改写。

这部分语义当前由 `mirage-gateway/pkg/gswitch/dnsless_hijack_linux.go` 承担。

## 当前有效的共振语义

### 共振发现不是逃逸本身

当前实现已经把“共振发现”和“逃逸切换”拆开了：

- `ResonanceResolver`
  负责从多通道发现新的入口信息。
- `ResonanceBridge`
  负责把发现结果导入 `GSwitchManager`。
- `GSwitchManager`
  负责真正激活或切换域名。

这比旧文档里把发现、扩散、执行混成一层要清楚很多。

### 当前已实现的发现通道

当前 `ResonanceResolver` 明确实现了三条并发竞速通道：

- `DoH DNS TXT`
- `CF Worker`
- `Mastodon`

任一通道先成功解密验签，即返回结果并取消其他通道。

### 桥接动作

`ResonanceBridge` 当前会：

1. 将解析出的域名导入热备池。
2. 将解析出的网关转换为可激活入口。
3. 在当前无活跃域名时，直接激活优先结果。

## 当前有效的池维护语义

G-Switch 当前还包含两条后台维护循环：

- 热备补充循环
- 冷却回收循环

这意味着域名池不是一次性静态表，而是一个持续维护的耗材池。

## 当前有效的对外抽象

`GSwitchAdapter` 当前向外暴露的有效控制面包括：

- 获取时间窗口内的战死次数
- 获取池统计
- 触发逃逸
- 监听域名战死事件
- 判断热备池是否为空

这可以视为当前 G-Switch 被其他模块消费的最小协议面。

## 本轮明确采纳的内容

以下旧文档语义，已经被新目录明确接管：

- “域名转生”在当前实现里首先是一个状态化入口切换机制。
- G-Switch 与 B-DNA 的协同已经存在，但以重置触发为边界。
- 共振发现是入口获取机制，不等同于切换执行机制。
- DNS-less 是一条独立的入口更新链路，而不是宣传性术语。

## 本轮明确不采纳为当前有效语义的内容

以下内容在旧文档中出现，但本轮不作为正式语义接管：

- `Raft` 一致性作为当前必经链路
- `Twitter / GitHub / IPFS / Tor` 全套公告板作为当前已实现承诺
- 旧文档中的扩散性能表
- 所有没有对应实现锚点的“秒级全网同步”表述

这些内容可以保留为历史设想或未来方向，但不应继续当作当前协议事实。

## 与旧文档的关系

- 旧 `docs/03-自研协议/G-Switch-域名转生协议.md` 继续保留，但默认视为输入材料。
- 后续若需要补更细的转生命令或事件语义，应优先回写本文档和 `mirage-proto/mirage.proto`，而不是继续扩写旧文档。
