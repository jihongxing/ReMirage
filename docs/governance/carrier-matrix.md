---
Status: authoritative
Frozen: true
FreezeDate: 2025-01-01
Target Truth: 五种承载协议的当前实现状态、承诺等级与降级参数的唯一真相源
---

# 承载矩阵（Carrier Matrix）

本文件冻结当前版本五种承载协议的实现状态，区分"正式承诺"与"已接线待闭环"，避免将设计入口误写成稳定承载能力。

## 一、承载优先级（真相源）

优先级按 Gateway 侧 `Orchestrator` 定义（`mirage-gateway/pkg/gtunnel/orchestrator.go`）：

| 优先级 | 承载协议 | PriorityLevel 值 |
|--------|----------|-------------------|
| 最高   | QUIC     | 0                 |
| 次高   | WebRTC   | 1                 |
| 中     | WSS      | 2                 |
| 最低   | ICMP     | 3                 |
| 最低   | DNS      | 3（与 ICMP 同级）  |

## 二、实现状态矩阵

| 承载协议 | Gateway 侧状态 | Client 侧状态 | 承诺等级 |
|----------|----------------|----------------|----------|
| QUIC     | ✅ Orchestrator 主路径，AdoptInboundConn 被动接入，probeLoop 探测，HappyEyeballs Phase 1 竞速 | ✅ ClientOrchestrator 主路径，Connect 首选 QUIC，降级后 probeAndPromote 持续探测回升 | **正式承诺** |
| WSS      | ✅ Orchestrator 备用路径，AdoptInboundConn 被动接入，ChameleonServerConnAdapter 适配 | ✅ ClientOrchestrator 降级路径，QUIC 超时后自动降级到 WSS | **正式承诺** |
| WebRTC   | ✅ Orchestrator HappyEyeballs Phase 2 拉起，依赖 WSS 信令通道，WSSSignaler + WebRTCTransport | ❌ ClientOrchestrator 未实现 WebRTC 拨号/接入 | **已接线待闭环** |
| ICMP     | ✅ Orchestrator 配置开关（默认关闭），ICMPTransportConfig 已定义，AdoptInboundConn 可接入 | ❌ ClientOrchestrator 未实现 ICMP 拨号/接入 | **已接线待闭环** |
| DNS      | ✅ Orchestrator 配置开关（默认关闭），DNSTransportConfig 已定义，dialDNS + NewDNSTransport 已实现 | ❌ ClientOrchestrator 未实现 DNS 拨号/接入 | **已接线待闭环** |

### 承诺等级判定规则

- **正式承诺**：Gateway 侧 Orchestrator 与 Client 侧 ClientOrchestrator 均有完整实现，具备端到端降级/回升能力
- **已接线待闭环**：仅 Gateway 侧有路径入口（配置、类型定义、接入函数），Client 侧未实现端到端闭环

## 三、QUIC→WSS 降级边界

### 参数对照表

| 参数 | 构造默认值 | 产品实际运行值 | 来源 |
|------|-----------|---------------|------|
| FallbackTimeout | 3s | 10s（由 `ProbeAndConnect` 传入） | `ClientOrchestratorConfig.FallbackTimeout` |
| ProbeInterval | 30s | 30s | `ClientOrchestratorConfig.ProbeInterval` |
| PromoteThreshold | 3 次连续成功 | 3 | `ClientOrchestratorConfig.PromoteThreshold` |

### 降级流程

1. `ClientOrchestrator.Connect` 以 `FallbackTimeout` 为超时尝试 QUIC 拨号
2. QUIC 拨号失败 → 尝试 WSS 拨号
3. WSS 拨号成功 → `activeType` 设为 `"wss"`，启动后台 `probeAndPromote` 协程

### 回升流程

1. `probeAndPromote` 以 `ProbeInterval`（30s）间隔探测 QUIC 可用性
2. 探测成功 → `successCount++`，打印 `"QUIC 探测成功 (N/M)"`
3. 探测失败 → `successCount` 重置为 0
4. `successCount >= PromoteThreshold` → 替换 `active` 为新 QUIC transport，关闭旧 WSS 连接，打印 `"已回升到 QUIC 主路径"`

### 全失败行为

QUIC 和 WSS 均不可达时，`Connect` 返回 `"all transports failed"` 错误，不进入静默失败。

## 四、Gateway 侧各承载详细状态

### QUIC

- 优先级：0（最高）
- 实现：`TransportQUIC` 类型，Phase 1 HappyEyeballs 竞速参与
- 被动接入：`AdoptInboundConn` 支持
- 降级触发：`probeLoop` → `auditor.ShouldDegrade` → `demote()`（非连接断开直接触发）
- 默认启用：是

### WSS

- 优先级：2
- 实现：`TransportWebSocket` 类型，Phase 1 HappyEyeballs 竞速参与，`ChameleonClientConn` / `ChameleonServerConnAdapter` 适配
- 被动接入：`AdoptInboundConn` 支持
- 附加角色：作为 WebRTC Phase 2 的信令通道（`wssConn` 引用）
- 默认启用：是

### WebRTC

- 优先级：1
- 实现：`TransportWebRTC` 类型，Phase 2 HappyEyeballs 后台拉起，依赖 WSS 信令通道
- 被动接入：`AdoptInboundConn` 支持
- 拨号：`dialWebRTC` → `WSSSignaler` → `NewWebRTCTransport`
- 默认启用：是
- **闭环缺口**：Client 侧无 WebRTC 实现

### ICMP

- 优先级：3（最低，与 DNS 同级）
- 实现：`TransportICMP` 类型，`ICMPTransportConfig` 已定义（TargetIP / GatewayIP / Identifier / MaxPayload）
- 被动接入：`AdoptInboundConn` 支持
- 默认启用：**否**（`EnableICMP: false`）
- **闭环缺口**：Client 侧无 ICMP 实现，Gateway 侧默认关闭

### DNS

- 优先级：3（最低，与 ICMP 同级）
- 实现：`TransportDNS` 类型，`DNSTransportConfig` 已定义（Domain / Resolver / QueryType / MaxLabelLen），`dialDNS` → `NewDNSTransport`
- 被动接入：`AdoptInboundConn` 支持
- 默认启用：**否**（`EnableDNS: false`）
- **闭环缺口**：Client 侧无 DNS 实现，Gateway 侧默认关闭

## 五、Gateway 侧降级机制

Gateway 侧降级入口为 `probeLoop` → `auditor.ShouldDegrade` → `demote()` 链路：

- `probeLoop` 按 `ProbeCycle`（默认 30s，Level 3 路径缩短为 15s）周期探测所有路径
- `auditor.ShouldDegrade` 基于丢包率（默认阈值 0.30）和 RTT 倍数（默认 2.0）判定
- `demote()` 查找下一可用低优先级路径，通过 `executeSwitchTransaction` 执行双发选收切换
- **注意**：连接断开不直接触发降级，降级仅通过 auditor 质量判定触发

## 六、代码锚点

| 组件 | 文件路径 |
|------|----------|
| Gateway Orchestrator | `mirage-gateway/pkg/gtunnel/orchestrator.go` |
| Client ClientOrchestrator | `phantom-client/pkg/gtclient/client_orchestrator.go` |
| 优先级常量 | `mirage-gateway/pkg/gtunnel/orchestrator.go` → `PriorityQUIC` / `PriorityWebRTC` / `PriorityWSS` / `PriorityICMP` / `PriorityDNS` |
| 默认配置 | `mirage-gateway/pkg/gtunnel/orchestrator.go` → `DefaultOrchestratorConfig()` |
| Client 配置 | `phantom-client/pkg/gtclient/client_orchestrator.go` → `ClientOrchestratorConfig` |
