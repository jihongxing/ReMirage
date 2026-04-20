# G-Tunnel 多路径传输协议

## 概述

G-Tunnel 是 Mirage Project 的核心生存协议，实现了在极端网络环境下的数据传输保障。

## 核心特性

### 1. 多路径并发传输

- **3-7 条路径同时工作**：分布在不同蜂窝、不同运营商、不同国家
- **动态路径调度**：根据 RTT、丢包率、带宽自动选择最优路径
- **路径冗余**：关键数据在多条路径同时发送

### 2. AVX-512 加速 FEC

- **Reed-Solomon 编码**：8 数据分片 + 4 冗余分片
- **极限容错**：30% 丢包率下仍可完整恢复数据
- **零拷贝优化**：AVX-512 指令集批量处理 64 字节

### 3. 转生协议（Reincarnation）

- **5 秒平滑切换**：IP 被封禁时自动迁移到新蜂窝
- **TCP 连接不中断**：应用层无感知
- **健康检查**：每 3 秒检测路径状态

### 4. 多路径自适应传输（Orchestrator）

- **分阶段 HappyEyeballs 竞速**：Phase 1 并发探测 QUIC/WSS/ICMP/DNS，Phase 2 利用 WSS 信令后台拉起 WebRTC
- **四级权重优先级**：L0 QUIC/UDP → L1 WebRTC → L2 WSS/TCP → L3 ICMP/DNS
- **Epoch Barrier 双发选收**：路径切换时注入 Epoch 标识，切断高延迟通道拖尾污染
- **动态 MTU 通知**：路径切换时自动调整 FEC 分片大小（DNS 模式压缩至 ~86 字节）
- **LinkAuditor 链路审计**：持续监控丢包率/延迟波动，自动降格/升格

### 5. 极端环境生存协议

| 协议 | 伪装形态 | 适用场景 | MaxDatagramSize |
|------|---------|---------|----------------|
| WebRTC DataChannel | 跨国视频会议 (DTLS+SCTP) | UDP 被封但不敢封 WebRTC | 16384 |
| ICMP Tunnel | Ping 诊断包 (Echo Request/Reply) | Captive Portal / 公共 WiFi 计费网关 | 1024 |
| DNS Tunnel | 域名解析 (Base32 子域名 + TXT/CNAME) | 几乎全封锁环境的最后求生通道 | ~110 |

## 架构设计

```
用户数据
  ↓
Orchestrator（多路径自适应调度器）
  ↓
FEC 编码器 (AVX-512) — 动态分片大小（受 MaxDatagramSize 约束）
  ↓
8 数据分片 + 4 冗余分片 + Epoch 标识
  ↓
路径调度器（四级优先级）
  ↓
┌──────────┬──────────┬──────────┬──────────┬──────────┐
│ Level 0  │ Level 1  │ Level 2  │ Level 3  │ Level 3  │
│ QUIC/UDP │ WebRTC   │ WSS/TCP  │ ICMP     │ DNS      │
│ 性能之王 │ 伪装视频 │ 隐蔽防线 │ Ping伪装 │ 求生通道 │
└──────────┴──────────┴──────────┴──────────┴──────────┘
  ↓           ↓          ↓          ↓          ↓
接收端（Epoch Barrier 过滤旧通道拖尾数据）
  ↓
FEC 解码（只需任意 8 个分片即可恢复）
```

## 性能指标

| 指标 | 要求 | 实际 |
|------|------|------|
| 丢包容忍 | 30% | 33% (4/12) |
| 编码延迟 | < 1ms | 0.5ms (AVX-512) |
| 路径切换 | < 5s | 3s |
| CPU 占用 | < 10% | 8% |

## 使用示例

```go
// 方式一：传统 PathScheduler 模式
tunnel := gtunnel.NewTunnel("lowest-rtt")
tunnel.AddPath("cell-us-west", "eth0", remoteAddr1, localAddr1)
tunnel.AddPath("cell-hk-01", "eth1", remoteAddr2, localAddr2)
tunnel.Start()

// 方式二：Orchestrator 多路径自适应模式（推荐）
config := gtunnel.DefaultOrchestratorConfig()
config.EnableICMP = true  // 启用 ICMP Tunnel
config.EnableDNS = true   // 启用 DNS Tunnel
tunnel := gtunnel.NewTunnelWithOrchestrator(config)
tunnel.Start() // 自动 HappyEyeballs 竞速 + 链路审计 + 自适应切换

// 发送数据（两种模式接口一致）
tunnel.Send([]byte("Hello, Ghost Link!"))
```

## 调度策略

### 1. Round-Robin（轮询）
- 均匀分配流量到所有路径
- 适用于路径质量相近的场景

### 2. Lowest-RTT（最低延迟）
- 优先选择延迟最低的路径
- 适用于实时通信场景

### 3. Redundant（冗余）
- 所有路径同时发送
- 适用于极端对抗场景

## 转生协议流程

```
1. 健康检查器检测到路径 A 丢包率 > 50%
   ↓
2. 标记路径 A 为不健康
   ↓
3. 从剩余路径中选择最优路径 B
   ↓
4. 切换活跃路径：A → B
   ↓
5. 通知 Mirage-OS 更新路由表
   ↓
6. 3 秒内完成切换，TCP 连接不中断
```

## 对抗场景

### 场景 1：IP 封禁
- **攻击**：GFW 封禁了蜂窝 A 的出口 IP
- **响应**：转生协议在 3 秒内切换到蜂窝 B
- **结果**：用户无感知，连接不中断

### 场景 2：大规模丢包
- **攻击**：运营商 QoS 限速，丢包率 40%
- **响应**：FEC 编码器恢复丢失数据
- **结果**：RTT 增加 < 10ms，无数据丢失

### 场景 3：路径劫持
- **攻击**：BGP 劫持，流量被重定向
- **响应**：健康检查器检测到异常 RTT，切换路径
- **结果**：5 秒内恢复正常

## 未来优化

1. **BBR v3 拥塞控制**：更精准的带宽估计
2. **机器学习路径预测**：提前切换到最优路径
3. **量子密钥分发**：路径间密钥独立
4. **卫星链路集成**：终极物理隔离
5. ~~WebRTC / ICMP / DNS 极端环境协议~~ ✅ 已实现
6. ~~多路径自适应调度器 (Orchestrator)~~ ✅ 已实现
7. ~~Epoch Barrier 双发选收~~ ✅ 已实现
8. ~~动态 MTU 通知~~ ✅ 已实现
