# 需求文档：Phase 1 — Gateway 闭环

## 简介

本阶段为 Mirage Project 四阶段实施的第一阶段，目标是补全 Mirage-Gateway 缺失的三个关键模块（`pkg/security`、`pkg/threat`、`pkg/api`），使 Gateway 成为可独立运行、可与 Mirage-OS 双向通信的完整战斗单元。

当前 Gateway 已具备完整的 eBPF 数据面（C）和 Go 控制面（策略引擎、G-Tunnel、G-Switch、Phantom、Cortex、评估器等），但缺少安全认证、威胁编排和 gRPC 通信能力，无法与 OS 控制中心对接。

本阶段为纯 Go 控制面开发，不涉及 C/eBPF 数据面变更。

## 术语表

- **Gateway**：Mirage-Gateway，运行于节点上的融合网关，包含 eBPF 数据面和 Go 控制面
- **OS**：Mirage-OS，全局控制中心（本阶段尚未构建，但需定义通信契约）
- **TLS_Manager**：mTLS 证书管理器，负责加载、验证和热重载 TLS 证书
- **Shadow_Auth**：影子认证模块，基于 Ed25519 挑战-响应机制验证用户合法性
- **Threat_Aggregator**：威胁事件聚合器，统一收集来自 Cortex、eBPF Monitor、Evaluator 的威胁事件
- **Threat_Responder**：威胁响应器，将威胁等级映射为协议参数调整动作
- **Blacklist_Manager**：黑名单管理器，维护本地 IP 黑名单并同步到 eBPF LPM Trie Map
- **GRPC_Client**：gRPC 上行客户端，负责向 OS 发送心跳、流量上报和威胁上报
- **GRPC_Server**：gRPC 下行服务端，接收 OS 下发的策略、黑名单、配额和转生指令
- **Command_Handler**：下行指令处理器，将 OS 指令转化为本地操作（写入 eBPF Map、触发 G-Switch 等）
- **Proto_Definition**：Protobuf 服务定义文件，Gateway↔OS 通信契约的唯一真相源
- **Strategy_Engine**：策略引擎（已有模块 `pkg/strategy/engine.go`），根据威胁等级调整防御参数
- **eBPF_Loader**：eBPF 加载器（已有模块 `pkg/ebpf/loader.go`），管理 eBPF 程序和 Map
- **Cortex_Analyzer**：Cortex 分析器（已有模块 `pkg/cortex/analyzer.go`），指纹-IP 关联分析
- **Threat_Bus**：威胁情报总线（已有模块 `pkg/cortex/threat_bus.go`），高危事件广播
- **Threat_Monitor**：eBPF 威胁监控器（已有模块 `pkg/ebpf/monitor.go`），从 Ring Buffer 读取内核态威胁事件
- **Evaluator_Scanner**：审计评估器（已有模块 `pkg/evaluator/scanner.go`），流量特征扫描与异常检测
- **GSwitch_Manager**：G-Switch 管理器（已有模块 `pkg/gswitch/manager.go`），域名转生执行
- **LPM_Trie**：最长前缀匹配 Trie，eBPF Map 类型，用于高效 IP 前缀匹配
- **Ring_Buffer**：eBPF Ring Buffer，C 数据面向 Go 控制面上报事件的通道
- **mTLS**：双向 TLS 认证，Gateway 和 OS 互相验证对方证书
- **Ed25519**：Edwards-curve 数字签名算法，用于影子认证的挑战-响应签名
- **Mirage_CLI**：用户端命令行工具（已有模块 `mirage-cli/`），生成 Ed25519 签名

## 需求

### 需求 1：mTLS 证书管理

**用户故事：** 作为 Gateway 运维人员，我希望 Gateway 能加载和管理 mTLS 证书，以便与 OS 建立双向认证的安全通信通道。

#### 验收标准

1. WHEN Gateway 启动时，THE TLS_Manager SHALL 从 gateway.yaml 的 mcc.tls 配置段读取证书路径（cert_file、key_file、ca_file）并加载证书文件
2. WHEN 证书文件路径有效且证书格式正确时，THE TLS_Manager SHALL 返回可用于 gRPC 连接的 tls.Config 对象，其中包含客户端证书和 CA 根证书池
3. IF 证书文件不存在或格式无效，THEN THE TLS_Manager SHALL 返回包含具体文件路径和错误原因的错误信息
4. WHEN mcc.tls.enabled 配置为 false 时，THE TLS_Manager SHALL 返回不启用 TLS 的配置（用于开发环境）
5. THE TLS_Manager SHALL 使用 gateway.yaml 中 mcc.tls.server_name 字段作为 TLS ServerName 验证值
6. WHEN 证书文件在运行时被更新时，THE TLS_Manager SHALL 在 60 秒内检测到变更并重新加载证书，且不中断现有连接


### 需求 2：Ed25519 影子认证

**用户故事：** 作为系统管理员，我希望 Gateway 能通过 Ed25519 挑战-响应机制验证用户合法性，以便只有持有正确私钥的用户才能通过认证。

#### 验收标准

1. WHEN 收到认证请求时，THE Shadow_Auth SHALL 生成一个包含随机 nonce 和当前时间戳的挑战字符串
2. WHEN 收到签名响应时，THE Shadow_Auth SHALL 使用用户注册时提交的 Ed25519 公钥验证签名的有效性
3. IF 签名验证失败，THEN THE Shadow_Auth SHALL 返回认证失败错误且不泄露失败的具体原因
4. IF 挑战字符串的时间戳与当前时间差超过 300 秒，THEN THE Shadow_Auth SHALL 拒绝该签名响应并返回挑战过期错误
5. THE Shadow_Auth SHALL 支持从 hex 编码的字符串解析 Ed25519 公钥（与 Mirage_CLI keygen 输出格式兼容）
6. THE Shadow_Auth SHALL 为每次认证请求生成独立的挑战字符串，确保同一挑战不可重复使用

### 需求 3：威胁事件聚合

**用户故事：** 作为 Gateway 系统，我需要将来自多个检测源的威胁事件统一聚合，以便形成完整的威胁态势感知。

#### 验收标准

1. THE Threat_Aggregator SHALL 从以下三个事件源收集威胁事件：Threat_Monitor（eBPF Ring Buffer 事件）、Cortex_Analyzer（指纹关联分析事件）、Evaluator_Scanner（流量异常检测事件）
2. WHEN 收到来自任意事件源的威胁事件时，THE Threat_Aggregator SHALL 将事件标准化为统一的内部事件格式，包含时间戳、事件类型、源 IP、严重程度和来源标识
3. WHEN 同一源 IP 在 60 秒内产生多个相同类型的威胁事件时，THE Threat_Aggregator SHALL 将这些事件合并为一个聚合事件，并记录事件计数
4. THE Threat_Aggregator SHALL 通过 Go channel 将聚合后的事件分发给 Threat_Responder 和 GRPC_Client
5. IF 事件队列深度超过 10000，THEN THE Threat_Aggregator SHALL 丢弃最旧的事件并记录丢弃计数

### 需求 4：威胁响应联动

**用户故事：** 作为 Gateway 系统，我需要根据威胁等级自动调整防御参数，以便在检测到威胁时实时提升防御强度。

#### 验收标准

1. WHEN Threat_Aggregator 输出的聚合事件严重程度达到阈值时，THE Threat_Responder SHALL 调用 Strategy_Engine 的 UpdateByThreat 方法更新防御等级
2. WHEN 防御等级发生变化时，THE Threat_Responder SHALL 通过 eBPF_Loader 的 UpdateStrategy 方法将新的防御参数写入 eBPF Map（jitter_config_map、vpc_config_map）
3. THE Threat_Responder SHALL 支持五个威胁等级（低/中/高/严重/极限），每个等级对应一组预定义的协议参数（Jitter 均值/标准差、噪声强度、填充率）
4. WHILE 威胁等级为严重或极限时，THE Threat_Responder SHALL 将当前威胁状态同步上报给 GRPC_Client 以通知 OS
5. WHEN 威胁等级从高等级降回低等级时，THE Threat_Responder SHALL 等待至少 120 秒的冷却期后再降低防御参数，防止抖动

### 需求 5：本地黑名单管理

**用户故事：** 作为 Gateway 系统，我需要维护一个本地 IP 黑名单并同步到 eBPF 数据面，以便在内核态高效拦截恶意 IP。

#### 验收标准

1. THE Blacklist_Manager SHALL 维护一个本地 IP 黑名单，支持 IPv4 地址和 CIDR 前缀（如 192.168.1.0/24）
2. WHEN 添加一个 IP 或 CIDR 到黑名单时，THE Blacklist_Manager SHALL 在 1 秒内将该条目同步到 eBPF LPM_Trie Map
3. WHEN 从 OS 收到全局黑名单更新时，THE Blacklist_Manager SHALL 将全局黑名单与本地黑名单合并，全局条目优先级高于本地条目
4. THE Blacklist_Manager SHALL 为每个黑名单条目记录添加时间、过期时间和来源（本地/全局）
5. WHEN 黑名单条目达到过期时间时，THE Blacklist_Manager SHALL 自动移除该条目并从 eBPF LPM_Trie Map 中删除对应条目
6. IF eBPF LPM_Trie Map 容量达到上限（65536 条目），THEN THE Blacklist_Manager SHALL 按过期时间优先淘汰最早过期的条目


### 需求 6：Protobuf 通信契约定义

**用户故事：** 作为系统架构师，我需要定义 Gateway 与 OS 之间的 gRPC 通信契约，以便双方基于统一的接口规范进行开发。

#### 验收标准

1. THE Proto_Definition SHALL 定义 GatewayUplink 服务，包含以下 RPC 方法：SyncHeartbeat（心跳同步）、ReportTraffic（流量上报）、ReportThreat（威胁上报）
2. THE Proto_Definition SHALL 定义 GatewayDownlink 服务，包含以下 RPC 方法：PushBlacklist（黑名单下发）、PushStrategy（策略下发）、PushQuota（配额下发）、PushReincarnation（转生指令下发）
3. THE Proto_Definition SHALL 为 SyncHeartbeat 请求定义以下字段：gateway_id（字符串）、timestamp（int64）、status（枚举：ONLINE/DEGRADED/EMERGENCY）、ebpf_loaded（布尔）、threat_level（int32）、active_connections（int64）、memory_usage_mb（int32）
4. THE Proto_Definition SHALL 为 ReportTraffic 请求定义以下字段：gateway_id（字符串）、timestamp（int64）、business_bytes（uint64）、defense_bytes（uint64）、period_seconds（int32）
5. THE Proto_Definition SHALL 为 ReportThreat 请求定义以下字段：gateway_id（字符串）、events（威胁事件列表），每个事件包含 timestamp、threat_type（枚举）、source_ip、source_port、severity、packet_count
6. THE Proto_Definition SHALL 为 PushBlacklist 请求定义以下字段：entries（黑名单条目列表），每个条目包含 cidr（字符串）、expire_at（int64）、source（枚举：LOCAL/GLOBAL）
7. THE Proto_Definition SHALL 为 PushStrategy 请求定义以下字段：defense_level（int32）、jitter_mean_us（uint32）、jitter_stddev_us（uint32）、noise_intensity（uint32）、padding_rate（uint32）、template_id（uint32）
8. THE Proto_Definition SHALL 为 PushReincarnation 请求定义以下字段：new_domain（字符串）、new_ip（字符串）、reason（字符串）、deadline_seconds（int32）
9. FOR ALL 有效的 Protobuf 消息，使用 proto.Marshal 序列化后再使用 proto.Unmarshal 反序列化 SHALL 产生与原始消息等价的对象（往返一致性）

### 需求 7：gRPC 上行通道

**用户故事：** 作为 Gateway 系统，我需要通过 gRPC 向 OS 发送心跳、流量统计和威胁事件，以便 OS 掌握 Gateway 的实时状态。

#### 验收标准

1. WHEN Gateway 启动并完成 mTLS 初始化后，THE GRPC_Client SHALL 使用 TLS_Manager 提供的 tls.Config 建立到 OS 的 gRPC 连接
2. WHILE Gateway 处于运行状态，THE GRPC_Client SHALL 每 30 秒向 OS 发送一次 SyncHeartbeat 请求，包含当前 Gateway 状态信息
3. WHILE Gateway 处于运行状态，THE GRPC_Client SHALL 每 60 秒向 OS 发送一次 ReportTraffic 请求，包含该周期内的业务流量和防御流量字节数
4. WHEN Threat_Aggregator 产生新的聚合威胁事件时，THE GRPC_Client SHALL 在 5 秒内将事件通过 ReportThreat 发送给 OS
5. IF gRPC 连接断开，THEN THE GRPC_Client SHALL 使用指数退避策略（初始 1 秒，最大 60 秒）进行重连，重连期间在本地缓存待发送的事件（最多 1000 条）
6. IF OS 端点不可达超过 300 秒，THEN THE GRPC_Client SHALL 将 Gateway 状态标记为 DEGRADED 并记录告警日志

### 需求 8：gRPC 下行通道

**用户故事：** 作为 Gateway 系统，我需要接收 OS 下发的策略、黑名单、配额和转生指令，以便根据全局决策调整本地行为。

#### 验收标准

1. WHEN Gateway 启动后，THE GRPC_Server SHALL 在配置的端口上启动 gRPC 服务端，使用 TLS_Manager 提供的 tls.Config 进行 mTLS 认证
2. WHEN 收到 PushStrategy 请求时，THE Command_Handler SHALL 将策略参数通过 eBPF_Loader 的 UpdateStrategy 方法写入 eBPF Map，并在 100 毫秒内完成
3. WHEN 收到 PushBlacklist 请求时，THE Command_Handler SHALL 将黑名单条目传递给 Blacklist_Manager 进行合并和 eBPF Map 同步
4. WHEN 收到 PushQuota 请求时，THE Command_Handler SHALL 将剩余配额值写入 eBPF quota_map，当配额为 0 时触发内核态流量阻断
5. WHEN 收到 PushReincarnation 请求时，THE Command_Handler SHALL 调用 GSwitch_Manager 的 TriggerEscape 方法执行域名转生
6. IF 收到的下行指令格式无效或参数越界，THEN THE Command_Handler SHALL 返回 gRPC InvalidArgument 错误码并记录告警日志
7. THE GRPC_Server SHALL 验证所有入站连接的客户端证书，拒绝未通过 mTLS 认证的连接


### 需求 9：main.go 启动序列集成

**用户故事：** 作为 Gateway 运维人员，我希望 Gateway 按照正确的依赖顺序启动所有模块，以便系统能可靠地初始化并进入运行状态。

#### 验收标准

1. WHEN Gateway 进程启动时，THE Gateway SHALL 按以下顺序初始化模块：mTLS 初始化 → eBPF 加载 → 策略引擎 → 威胁编排（Aggregator + Responder + Blacklist） → gRPC 客户端（上行） → gRPC 服务端（下行） → 健康检查
2. IF 任一关键模块（mTLS、eBPF 加载）初始化失败，THEN THE Gateway SHALL 记录错误日志并终止进程，退出码为非零值
3. IF 非关键模块（gRPC 客户端、gRPC 服务端）初始化失败，THEN THE Gateway SHALL 记录告警日志并以降级模式继续运行
4. WHEN 收到 SIGINT 或 SIGTERM 信号时，THE Gateway SHALL 按启动的逆序依次关闭所有模块，关闭超时为 30 秒
5. WHILE Gateway 处于运行状态，THE Gateway 的 Go 控制面响应时间 SHALL 保持在 100 毫秒以内，内存占用 SHALL 保持在 200MB 以内

### 需求 10：健康检查增强

**用户故事：** 作为 Gateway 运维人员，我希望健康检查端点能反映所有新模块的状态，以便准确判断 Gateway 的整体健康度。

#### 验收标准

1. WHEN 收到 /status HTTP 请求时，THE Gateway SHALL 返回包含以下字段的 JSON 响应：ebpf_loaded、grpc_client_connected、grpc_server_running、threat_level、blacklist_count、uptime
2. WHEN gRPC 客户端与 OS 的连接断开时，THE Gateway SHALL 将 /readyz 端点返回 503 状态码，表示 Gateway 处于降级状态
3. WHEN 所有关键模块正常运行且 gRPC 连接正常时，THE Gateway SHALL 将 /healthz 和 /readyz 端点均返回 200 状态码
