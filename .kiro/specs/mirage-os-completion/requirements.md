# 需求文档：Mirage-OS 控制面 P0 待办任务完成

## 简介

本文档定义 Mirage-OS 控制面所有 P0 级待办任务的需求。这些任务阻断商业闭环，涵盖 gRPC 服务实现、Raft 集群安全机制、蜂窝调度器 Gateway 通信、以及 Monero 支付集成。所有实现均为 Go 控制面代码，遵循 C 数据面 + Go 控制面的二元架构原则。

## 术语表

- **Mirage-OS**: 系统控制中心，Go 后端，负责 API、Raft 共识、微服务编排
- **Gateway**: 融合网关节点，运行 eBPF 数据面 + Go 控制面，通过 gRPC 与 Mirage-OS 通信
- **Gateway_Bridge**: Mirage-OS 侧的 gRPC 桥接服务，处理 Gateway 上行请求并提供下行推送
- **CellService**: 蜂窝管理 gRPC 服务，负责蜂窝注册、查询、分配、健康检查、切换
- **BillingService**: 计费 gRPC 服务，负责账户创建、Monero 充值、余额查询、流量包购买、流水查询
- **GatewayService**: Gateway 心跳 gRPC 服务，负责心跳同步、流量上报、威胁上报、配额查询
- **GatewayDownlink**: OS→Gateway 下行 gRPC 服务，负责推送黑名单、策略、配额、转生指令
- **HotKeyManager**: 热密钥管理器，管理 Shamir 秘密分享的主密钥生命周期
- **GeoFence**: 地理围栏模块，检测政府审计、DDoS、异常流量等威胁
- **CellScheduler**: 蜂窝调度器，管理 Gateway 生命周期（潜伏→校准→服役）
- **MoneroManager**: Monero 充值管理器，处理 XMR 交易确认和汇率查询
- **Shamir_Secret_Sharing**: 3-of-5 秘密分享方案，将主密钥分割为 5 份，任意 3 份可恢复
- **Raft_Cluster**: 基于 hashicorp/raft 的 3 节点共识集群
- **B-DNA**: 行为识别协议指纹模板，用于 eBPF 层 TCP/QUIC/TLS 指纹重写
- **VPC_Noise**: VPC 噪声注入协议，在潜伏期模拟物理噪音
- **Monero_RPC**: Monero 节点 JSON-RPC 接口，用于查询交易和钱包操作

## 需求

### 需求 1：GatewayDownlink 下行 gRPC 服务实现

**用户故事：** 作为 Mirage-OS 运维人员，我希望控制面能主动向 Gateway 推送黑名单、防御策略、配额更新和转生指令，以便实现实时的安全响应和资源管理。

#### 验收标准

1. WHEN Mirage-OS 检测到需要封禁的 IP 列表时，THE Gateway_Bridge SHALL 通过 PushBlacklist RPC 将黑名单条目（CIDR、过期时间、来源类型）推送到目标 Gateway
2. WHEN 防御等级或策略参数发生变更时，THE Gateway_Bridge SHALL 通过 PushStrategy RPC 将新的防御配置（defense_level、jitter 参数、noise_intensity、padding_rate、template_id）推送到目标 Gateway
3. WHEN 用户配额发生变更时，THE Gateway_Bridge SHALL 通过 PushQuota RPC 将剩余配额字节数推送到目标 Gateway
4. WHEN 域名转生触发时，THE Gateway_Bridge SHALL 通过 PushReincarnation RPC 将新域名、新 IP、原因和截止时间推送到目标 Gateway
5. IF Gateway 不可达或推送失败，THEN THE Gateway_Bridge SHALL 将推送任务加入重试队列，并在 Gateway 下次心跳时重试


### 需求 2：CellService gRPC 服务实现

**用户故事：** 作为 Mirage-OS 管理员，我希望通过 gRPC 接口管理蜂窝拓扑（注册、查询、分配、健康检查、切换），以便实现蜂窝网络的动态编排。

#### 验收标准

1. WHEN 收到 RegisterCell 请求时，THE CellService SHALL 验证蜂窝参数（cell_id 唯一性、等级有效性、地理位置完整性），将蜂窝记录持久化到 PostgreSQL，并返回注册结果
2. WHEN 收到 ListCells 请求时，THE CellService SHALL 根据筛选条件（等级、国家、是否仅在线）从数据库查询蜂窝列表，并返回包含状态、负载率、Gateway 数量的完整蜂窝信息
3. WHEN 收到 AllocateGateway 请求时，THE CellService SHALL 根据用户偏好（等级、国家）选择负载最低的可用蜂窝，生成连接令牌，并返回分配结果
4. WHEN 收到 HealthCheck 请求时，THE CellService SHALL 检查指定蜂窝（或全部蜂窝）的健康状态，包括 Gateway 数量、失败 Gateway 数量、平均延迟、24 小时威胁计数
5. WHEN 收到 SwitchCell 请求时，THE CellService SHALL 根据切换原因（用户请求、威胁检测、过载、离线、管辖区变更）选择目标蜂窝，执行 Gateway 迁移，并返回新蜂窝信息和连接令牌

### 需求 3：BillingService gRPC 服务实现

**用户故事：** 作为 Mirage-OS 计费模块，我希望通过 gRPC 接口提供完整的计费功能（账户创建、充值、余额查询、流量包购买、流水查询），以便支撑 Monero 匿名支付闭环。

#### 验收标准

1. WHEN 收到 CreateAccount 请求时，THE BillingService SHALL 验证 user_id 和 public_key 的唯一性，创建用户账户记录，并返回 account_id
2. WHEN 收到 Deposit 请求时，THE BillingService SHALL 验证 Monero 交易哈希的有效性，创建待确认充值记录，查询实时汇率计算 USD 等值，并返回余额和汇率信息
3. WHEN 收到 GetBalance 请求时，THE BillingService SHALL 查询用户余额（USD）、配额信息（总量、已用、剩余、过期时间）和蜂窝分配详情
4. WHEN 收到 PurchaseQuota 请求时，THE BillingService SHALL 验证用户余额充足，根据流量包类型和蜂窝等级计算费用，扣减余额，分配配额，并返回购买结果
5. WHEN 收到 GetBillingLogs 请求时，THE BillingService SHALL 根据时间范围和条数限制查询计费流水记录，并返回包含流量明细和费用的流水列表

### 需求 4：GatewayService gRPC 服务实现

**用户故事：** 作为 Mirage-OS 网关管理模块，我希望通过 gRPC 接口处理 Gateway 的心跳同步、流量上报、威胁上报和配额查询，以便实时掌握 Gateway 状态并做出响应。

#### 验收标准

1. WHEN 收到 SyncHeartbeat 请求时，THE GatewayService SHALL 更新 Gateway 状态（在线状态、威胁等级、资源使用、活跃连接数）到数据库和 Redis 缓存，并返回防御配置和剩余配额
2. WHEN 收到 ReportTraffic 请求时，THE GatewayService SHALL 记录流量数据（业务流量、防御流量、流量细分），执行配额扣减，计算当前费用，并在配额低于 10% 时设置告警标志
3. WHEN 收到 ReportThreat 请求时，THE GatewayService SHALL 将威胁事件（类型、源 IP、严重程度、JA4 指纹）持久化到威胁情报库，评估威胁动作（提升防御、封禁 IP、切换蜂窝、紧急关闭），并返回建议动作和新防御等级
4. WHEN 收到 GetQuota 请求时，THE GatewayService SHALL 查询用户的剩余配额、总配额、过期时间、自动续费状态和蜂窝配额详情


### 需求 5：Shamir 份额收集实现

**用户故事：** 作为 Raft 集群热密钥管理器，我希望在冷启动时能从在线 Raft 节点收集 Shamir 秘密份额，以便恢复主密钥并激活热密钥模式。

#### 验收标准

1. WHEN HotKeyManager 执行冷启动时，THE HotKeyManager SHALL 查询 Raft 集群中所有在线节点的地址列表
2. WHEN 在线节点列表获取成功时，THE HotKeyManager SHALL 向每个在线节点发送份额请求，并收集至少 3 个有效份额
3. IF 收集到的有效份额数量少于 3 个，THEN THE HotKeyManager SHALL 返回错误并记录日志，指明可用节点数量和所需最小份额数
4. WHEN 收集到足够份额时，THE HotKeyManager SHALL 验证每个份额的索引有效性（1-5 范围）和数据长度（32 字节），丢弃无效份额
5. IF 单个节点的份额请求超时（超过 5 秒），THEN THE HotKeyManager SHALL 跳过该节点并继续请求其他节点

### 需求 6：密钥轮换实现

**用户故事：** 作为安全管理员，我希望热密钥能定期轮换（生成新主密钥、重新分割、分发到节点），以便降低密钥泄露风险。

#### 验收标准

1. WHEN RefreshHotKey 被调用时，THE HotKeyManager SHALL 停用当前热密钥（清零内存）
2. WHEN 旧密钥停用完成后，THE HotKeyManager SHALL 生成新的 32 字节 AES-256 主密钥
3. WHEN 新主密钥生成后，THE HotKeyManager SHALL 使用 3-of-5 Shamir 方案将新密钥分割为 5 个份额
4. WHEN 份额分割完成后，THE HotKeyManager SHALL 将新份额分发到 Raft 集群的各个节点
5. WHEN 份额分发完成后，THE HotKeyManager SHALL 使用收集到的份额激活新的热密钥
6. IF 密钥轮换过程中任何步骤失败，THEN THE HotKeyManager SHALL 尝试使用旧份额恢复热密钥，确保服务不中断

### 需求 7：威胁检测逻辑实现

**用户故事：** 作为 Raft 集群节点，我希望能检测异常网络流量、政府审计行为和 DDoS 攻击，以便在威胁等级过高时触发 Leader 退位和蜂窝迁移。

#### 验收标准

1. WHILE Raft 集群运行中，THE Cluster SHALL 每 30 秒执行一次威胁检测，综合评估网络流量、审计行为和攻击指标
2. WHEN 威胁等级达到 8 或以上且当前节点为 Leader 时，THE Cluster SHALL 触发 Leader 退位（LeadershipTransfer），将领导权转移到其他司法管辖区的节点
3. THE GeoFence SHALL 通过以下指标综合计算威胁等级：政府审计检测结果、DDoS 攻击检测结果、异常流量检测结果，取最近 5 分钟内最高威胁等级

### 需求 8：政府审计检测实现

**用户故事：** 作为地理围栏模块，我希望能检测法律传票、数据中心访问异常和网络监控设备接入，以便在政府审计发生时触发最高级别威胁响应。

#### 验收标准

1. THE GeoFence SHALL 监控 Raft 集群节点的网络连接模式，检测来自已知政府 IP 段的连接尝试
2. THE GeoFence SHALL 监控数据中心物理访问指标（通过 API 或传感器数据），检测非计划内的物理访问事件
3. THE GeoFence SHALL 检测网络路径中新增的中间设备（TTL 变化、路由跳数异常），识别可能的网络监控设备
4. WHEN 检测到政府审计指标时，THE GeoFence SHALL 将威胁等级设置为 CRITICAL（9），触发最高级别响应

### 需求 9：DDoS 检测实现

**用户故事：** 作为地理围栏模块，我希望能检测 DDoS 攻击（流量峰值、SYN Flood、UDP Flood），以便及时触发防御响应。

#### 验收标准

1. THE GeoFence SHALL 维护流量基线统计（滑动窗口平均值），检测当前流量是否超过基线的 5 倍
2. THE GeoFence SHALL 监控 TCP SYN 包速率，检测 SYN Flood 攻击（SYN 速率超过每秒 10000 个）
3. THE GeoFence SHALL 监控 UDP 包速率，检测 UDP Flood 攻击（UDP 速率超过每秒 50000 个）
4. WHEN 检测到 DDoS 攻击指标时，THE GeoFence SHALL 将威胁等级设置为 HIGH（7）

### 需求 10：异常流量检测实现

**用户故事：** 作为地理围栏模块，我希望能检测流量模式异常、连接数异常和地理位置异常，以便识别潜在的网络监控或攻击行为。

#### 验收标准

1. THE GeoFence SHALL 分析流量时间分布模式，检测与历史基线偏差超过 3 个标准差的异常
2. THE GeoFence SHALL 监控单 IP 连接数，检测单个 IP 建立超过 1000 个并发连接的异常行为
3. THE GeoFence SHALL 分析连接来源的地理分布，检测来自非预期地理区域的大量连接
4. WHEN 检测到异常流量指标时，THE GeoFence SHALL 将威胁等级设置为 MEDIUM（5）


### 需求 11：蜂窝调度器 Gateway 通信实现

**用户故事：** 作为蜂窝调度器，我希望在 Gateway 生命周期的各个阶段能通过 gRPC 下行通道向 Gateway 发送控制指令（VPC 噪声注入、网络质量测量、B-DNA 模板下发），以便完成影子蜂窝的完整生命周期管理。

#### 验收标准

1. WHEN 新 Gateway 注册并进入潜伏期时，THE CellScheduler SHALL 通过 GatewayDownlink 的 PushStrategy RPC 向 Gateway 发送 VPC 噪声注入启动指令（包含 noise_intensity 参数）
2. WHEN Gateway 从潜伏期晋升到校准期时，THE CellScheduler SHALL 通过 GatewayDownlink 的 PushStrategy RPC 向 Gateway 发送网络质量测量启动指令（包含探测目标和采样参数）
3. WHEN Gateway 从校准期晋升到服役期时，THE CellScheduler SHALL 通过 GatewayDownlink 的 PushStrategy RPC 向 Gateway 发送 B-DNA 指纹模板（包含 template_id），指示 Gateway 将模板加载到 eBPF Map
4. IF Gateway 在接收指令时不可达，THEN THE CellScheduler SHALL 将指令加入待推送队列，在 Gateway 下次心跳时重试

### 需求 12：Monero RPC 交易确认数查询实现

**用户故事：** 作为 Monero 充值管理器，我希望能通过 Monero 节点 RPC 查询真实的交易确认数，以便准确判断充值是否达到最小确认数（10 个确认块）。

#### 验收标准

1. WHEN 查询交易确认数时，THE MoneroManager SHALL 调用 Monero 节点的 `get_transfer_by_txid` JSON-RPC 方法，传入交易哈希
2. WHEN Monero RPC 返回成功响应时，THE MoneroManager SHALL 从响应中提取 `confirmations` 字段并返回
3. IF Monero RPC 调用失败（网络错误、超时、节点不可用），THEN THE MoneroManager SHALL 返回错误信息，不使用模拟值
4. IF Monero RPC 返回交易不存在，THEN THE MoneroManager SHALL 返回 0 确认数和描述性错误

### 需求 13：XMR/USD 实时汇率查询实现

**用户故事：** 作为 Monero 充值管理器，我希望能从外部 API 获取 XMR/USD 实时汇率，以便准确计算充值的 USD 等值。

#### 验收标准

1. WHEN 查询汇率时，THE MoneroManager SHALL 优先从 Redis 缓存读取汇率（缓存有效期 5 分钟）
2. WHEN 缓存未命中或已过期时，THE MoneroManager SHALL 调用 CoinGecko API（`/api/v3/simple/price?ids=monero&vs_currencies=usd`）获取实时汇率
3. IF CoinGecko API 不可用，THEN THE MoneroManager SHALL 回退到 Kraken API（`/0/public/Ticker?pair=XMRUSD`）作为备用数据源
4. WHEN 成功获取汇率时，THE MoneroManager SHALL 将汇率缓存到 Redis（key: `mirage:xmr_usd_rate`，TTL: 5 分钟）
5. IF 所有外部 API 均不可用，THEN THE MoneroManager SHALL 返回错误，不使用硬编码的固定汇率
