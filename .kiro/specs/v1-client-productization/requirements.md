# 需求文档：Client 产品化（拓扑学习 + 订阅托管 + 后台服务化）

## 简介

本 Spec 对应 `Client 鲁棒性整改清单.md` 第二阶段（"把能不能自己活下去补齐"）和第三阶段（精选高价值项），目标是将 phantom-client 从"可运行的连接工具"升级为"用户几乎无感、可长期常驻、可自愈、可托管"的正式产品形态。

当前 Client 的核心缺陷：
1. **拓扑学习空壳**：`PullRouteTable()` 是空实现，Client 启动后无法学习运行时 Gateway 拓扑，只能依赖启动时的 Bootstrap Pool
2. **订阅无运行时托管**：有效期只在启动时检查一次，常驻后无法感知续费、降级、到期或封禁
3. **前台 CLI 形态**：依赖命令行参数、stdin、前台日志输出和信号退出，不适合长期常驻
4. **物理网卡探测硬编码**：QUIC 建链通过 `net.Dial("udp4", "8.8.8.8:53")` 探测出口，在受限网络中不可靠
5. **路由回滚不完整**：激活中途失败、切换中途失败、服务异常退出缺少系统性回滚保护

前置依赖：
- Spec 1-3（v1-client-fsm-route-fix）：提供统一连接状态机 FSM、单飞重连、原子事务切换
- Spec 2-3（v1-os-control-plane）：提供 OS 侧 Gateway 注册、拓扑索引、拓扑同步协议

## 术语表

- **GTunnelClient**：G-Tunnel 客户端主结构体，管理连接、FEC、重组器、路由表、当前网关
- **RouteTable**：内存网关节点列表，存储运行时学习到的 Gateway 拓扑信息
- **Bootstrap_Pool**：启动种子池，来自 token/URI 兑换的初始 Gateway 列表，仅用于首次建链和保底兜底
- **Runtime_Topology**：运行时拓扑池，通过周期性拉取 OS 控制面获得的动态 Gateway 列表
- **Route_Table_Protocol**：路由表拉取协议，Client 与 OS 之间通过 gRPC/HTTP 同步 Gateway 拓扑的正式协议
- **Entitlement**：订阅权益，包含有效期、剩余配额、服务等级、封禁状态等用户订阅信息
- **Service_Class**：服务等级，OS 下发的付费等级标识（Standard/Platinum/Diamond），决定 Client 运行时行为策略
- **Entitlement_Manager**：订阅托管管理器，负责周期性拉取和缓存用户订阅状态
- **Daemon_Manager**：后台服务管理器，负责将 Client 主进程注册为系统服务（systemd/Windows Service）
- **Provisioning_Flow**：首次开通流程，用户通过 token/URI 完成初始配置绑定的一次性流程
- **Runtime_Flow**：后台运行流程，Client 以 daemon 形态长期常驻、自动连接、自动恢复的持续运行流程
- **Health_Guardian**：健康守护器，负责进程级和功能级的持续健康检查与自恢复
- **Degradation_Level**：退化等级，定义 Client 在不同故障场景下的分级退化行为（L1 正常/L2 轻度退化/L3 绝境恢复）
- **Grace_Window**：离线宽限窗口，控制面失联后允许 Client 继续维持当前连接的最大时间
- **KillSwitch**：路由劫持管理器，实现 fail-closed 路由策略
- **QUICEngine**：QUIC Datagram 连接引擎，管理物理 NIC 绑定和数据报收发
- **Resonance_Resolver**：信令共振发现器，三通道（DoH/Gist/Mastodon）并发拉取加密信令
- **Physical_NIC_Detector**：物理网卡出口探测器，负责识别 QUIC 建链应绑定的物理出口 IP

## 需求

### 需求 1：实现真正可用的 Route Table 拉取（TOPO-1）

**用户故事：** 作为 Client，我需要在启动后能真正从 OS 控制面拉取运行时 Gateway 拓扑，以便不再只依赖启动时的 Bootstrap Pool 进行连接和切换。

#### 验收标准

1. THE GTunnelClient SHALL 实现 `PullRouteTable(ctx)` 方法，通过 Route_Table_Protocol 向 OS 控制面发起拓扑拉取请求
2. WHEN 拉取成功时，THE Route_Table_Protocol SHALL 返回包含以下字段的路由表：Gateway 列表（IP + Port）、每个 Gateway 的优先级、区域/Cell 标识、路由表版本号
3. WHEN 拉取成功时，THE GTunnelClient SHALL 将结果写入内存 RouteTable，并记录最近更新时间戳
4. IF 拉取失败（网络超时、OS 不可达、协议错误），THEN THE GTunnelClient SHALL 保留现有 RouteTable 不变，并记录失败原因
5. THE RouteTable SHALL 为每条 Gateway 记录维护优先级字段，重连时按优先级降序尝试

### 需求 2：拓扑学习改成持续刷新（TOPO-2）

**用户故事：** 作为 Client，我需要在运行期间持续刷新 Gateway 拓扑，以便适应节点上下线、调度变化和控制面重分配。

#### 验收标准

1. THE GTunnelClient SHALL 启动一个后台 goroutine，按可配置的周期（默认 5 分钟）执行 PullRouteTable
2. WHEN 周期性拉取失败时，THE GTunnelClient SHALL 使用指数退避策略重试（初始间隔 30 秒，最大间隔 10 分钟），且保留现有拓扑不变
3. THE RouteTable SHALL 维护版本号字段，WHEN 收到新路由表时，THE GTunnelClient SHALL 比较版本号，仅当新版本大于当前版本时才更新
4. WHEN 连续 3 次拉取失败时，THE GTunnelClient SHALL 发出拓扑刷新告警事件（供日志和状态展示消费）
5. WHEN 拉取恢复成功时，THE GTunnelClient SHALL 重置退避计时器并恢复正常刷新周期

### 需求 3：绝境发现链路真正接入主流程（TOPO-3）

**用户故事：** 作为 Client，我需要在所有已知节点不可用时自动进入绝境发现流程，以便实现最末级自愈能力。

#### 验收标准

1. THE 主启动流程（main.go）SHALL 在初始化阶段构造 Resonance_Resolver 并通过 `SetResonanceResolver` 注入 GTunnelClient
2. WHEN Resonance_Resolver 未注入时，THE GTunnelClient SHALL 跳过 Level 3 绝境发现，仅使用 Level 1（RouteTable）和 Level 2（Bootstrap_Pool）
3. WHEN 绝境发现成功时，THE GTunnelClient SHALL 将发现到的新节点同时写入内存 RouteTable，使其进入后续可复用的拓扑池
4. THE 绝境发现 SHALL 使用独立超时（默认 15 秒），不受主重连超时（5 秒）限制
5. WHEN 绝境发现成功且建链成功时，THE GTunnelClient SHALL 立即触发一次 PullRouteTable 以获取完整运行时拓扑

### 需求 4：区分启动种子池和运行时拓扑池（TOPO-4）

**用户故事：** 作为 Client，我需要明确区分 Bootstrap_Pool 和 Runtime_Topology 的职责，以便主切换路径优先依赖运行时拓扑，Bootstrap 仅作为保底兜底。

#### 验收标准

1. THE GTunnelClient SHALL 维护两个独立的地址池：Bootstrap_Pool（来自 token/URI，只读）和 Runtime_Topology（来自 PullRouteTable，可更新）
2. WHEN 执行重连时，THE GTunnelClient SHALL 按以下优先级尝试：Level 1 Runtime_Topology → Level 2 Bootstrap_Pool → Level 3 Resonance_Resolver
3. THE Bootstrap_Pool SHALL 在整个 Client 生命周期内保持不变，不受 PullRouteTable 结果影响
4. THE Runtime_Topology SHALL 支持独立的优先级排序、最近更新时间和版本号管理
5. WHEN Runtime_Topology 为空（首次启动尚未拉取）时，THE GTunnelClient SHALL 回退到 Bootstrap_Pool 进行首次建链

### 需求 5：拓扑最小可信约束（TOPO-5）

**用户故事：** 作为 Client，我需要对拉取到的路由表进行签名和版本校验，以便防止学到错误或被篡改的拓扑。

#### 验收标准

1. THE Route_Table_Protocol 响应 SHALL 包含签名字段（HMAC-SHA256，使用 PSK 派生密钥签名）
2. WHEN 收到路由表响应时，THE GTunnelClient SHALL 验证签名，IF 签名校验失败，THEN THE GTunnelClient SHALL 拒绝该路由表并保留现有拓扑
3. THE GTunnelClient SHALL 验证路由表的发布时间戳，IF 发布时间早于当前拓扑的发布时间，THEN THE GTunnelClient SHALL 拒绝该路由表（防回滚）
4. IF 路由表中 Gateway 列表为空，THEN THE GTunnelClient SHALL 拒绝该路由表并记录异常事件
5. THE GTunnelClient SHALL 对连续 3 次签名校验失败执行保守策略：暂停拓扑刷新 30 分钟，并发出安全告警事件

### 需求 6：订阅校验升级为运行时托管（SUB-2）

**用户故事：** 作为用户，我需要 Client 在常驻运行期间持续同步我的订阅状态，以便续费后自动续得新状态、到期后执行受控降级。

#### 验收标准

1. THE Entitlement_Manager SHALL 按可配置的周期（默认 10 分钟）向 OS 控制面拉取用户订阅状态
2. THE 订阅状态 SHALL 包含以下字段：有效期（expires_at）、剩余配额（quota_remaining_bytes）、服务等级（service_class）、封禁状态（banned）
3. WHEN 订阅状态发生变化时，THE Entitlement_Manager SHALL 发出状态变更事件，供 GTunnelClient 和日志系统消费
4. IF 拉取失败，THEN THE Entitlement_Manager SHALL 使用指数退避重试，并保留最近一次成功拉取的状态
5. WHEN 检测到 banned=true 时，THE Entitlement_Manager SHALL 立即通知 GTunnelClient 执行受控断开

### 需求 7：服务等级策略做成 Client 运行时能力（SUB-3）

**用户故事：** 作为运营方，我需要不同服务等级的用户在 Client 行为上存在稳定、可验证的差异，以便服务分层落到产品行为上。

#### 验收标准

1. THE GTunnelClient SHALL 维护当前 Service_Class 字段，初始值从 Entitlement_Manager 获取
2. WHEN Service_Class 变更时，THE GTunnelClient SHALL 即时调整以下运行时行为：可用 Gateway 池范围、重连积极度（退避参数）、绝境发现是否启用、心跳频率
3. THE Service_Class SHALL 支持至少三个等级：Standard（标准）、Platinum（高级）、Diamond（钻石）
4. WHEN Service_Class 为 Standard 时，THE GTunnelClient SHALL 禁用 Resonance_Resolver（绝境发现仅对高级及以上开放）
5. WHEN Service_Class 变更时，THE GTunnelClient SHALL 不要求用户重启或重新配置，变更即时生效

### 需求 8：前台 CLI 改成正式后台服务（SVC-1）

**用户故事：** 作为用户，我需要 Client 安装并开通后默认以后台服务运行，以便终端关闭不影响服务持续工作。

#### 验收标准

1. THE Daemon_Manager SHALL 在 Linux 上生成并注册 systemd service unit 文件，支持 `systemctl start/stop/status phantom-client`
2. THE Daemon_Manager SHALL 在 Windows 上注册为 Windows Service，支持通过服务管理器启停
3. THE Client 主进程 SHALL 支持 `--daemon` 启动参数，以后台服务模式运行（无 stdin 依赖、无前台日志输出）
4. WHEN 以 daemon 模式运行时，THE Client SHALL 将日志输出到文件或系统日志（Linux: journald，Windows: Event Log），不再输出到 stderr
5. THE Client SHALL 保留 `--foreground` 模式作为调试入口，行为与当前 CLI 模式一致

### 需求 9：首次开通与长期运行拆成两个阶段（SVC-2）

**用户故事：** 作为用户，我需要完成首次开通后，后续使用不再依赖重复输入 token 或 URI，以便运行态与配置态职责清晰分离。

#### 验收标准

1. THE Provisioning_Flow SHALL 定义为一次性操作：接受 token/URI → 兑换配置 → 验证连通性 → 持久化非敏感配置 → 注册系统服务
2. THE Runtime_Flow SHALL 定义为持续操作：读取已持久化配置 → 创建 TUN → 建链 → 转发 → 周期性拓扑刷新 → 周期性订阅同步
3. THE Provisioning_Flow SHALL 将以下非敏感配置持久化到本地文件：Bootstrap_Pool、CertFingerprint、UserID、OS 控制面地址
4. THE Provisioning_Flow SHALL 将敏感材料（PSK、AuthKey）存储到操作系统密钥管理（Linux: keyring，Windows: Credential Manager），不落盘明文
5. WHEN Runtime_Flow 启动时发现本地无有效配置，THE Client SHALL 提示用户执行 Provisioning_Flow，不尝试无配置启动

### 需求 10：分级退化策略显式行为（FSM-4）

**用户故事：** 作为运维人员，我需要准确知道 Client 当前是在正常连接、轻度退化还是绝境恢复，以便故障复盘时能还原完整重连路径。

#### 验收标准

1. THE GTunnelClient SHALL 定义三个退化等级：L1_Normal（使用 Runtime_Topology 正常连接）、L2_Degraded（回退到 Bootstrap_Pool）、L3_LastResort（进入 Resonance_Resolver 绝境发现）
2. WHEN 进入每个退化等级时，THE GTunnelClient SHALL 记录进入条件、进入时间戳和触发原因
3. WHEN 从高退化等级恢复到低退化等级时，THE GTunnelClient SHALL 记录恢复耗时和恢复路径
4. THE GTunnelClient SHALL 提供 `DegradationLevel()` 方法，返回当前退化等级
5. WHEN 退化等级发生变化时，THE GTunnelClient SHALL 发出退化事件，包含等级、原因、尝试次数和耗时

### 需求 11：离线宽限与受控退化策略（SUB-4）

**用户故事：** 作为用户，我需要控制面短时故障不会把我瞬间踢下线，以便正常在线用户不受控制面波动影响。

#### 验收标准

1. THE Entitlement_Manager SHALL 维护一个 Grace_Window（默认 24 小时），WHEN 控制面失联时，THE Client SHALL 在 Grace_Window 内继续维持当前连接
2. THE Client SHALL 区分四类离线场景并执行不同策略：
   - 控制面失联：维持当前连接，禁止服务等级升级，Grace_Window 到期后进入只读模式
   - 配额耗尽：允许维持当前连接但禁止新建连接，提示用户充值
   - 订阅到期：Grace_Window 到期后执行受控断开，保留本地配置以便续费后快速恢复
   - 账号停用（banned）：立即执行受控断开，清除本地敏感材料
3. WHEN Grace_Window 到期且控制面仍不可达时，THE Client SHALL 进入只读模式（维持已有连接但不执行拓扑刷新和服务等级变更）
4. WHEN 控制面恢复可达时，THE Client SHALL 立即执行一次订阅状态同步并恢复正常运行
5. THE Entitlement_Manager SHALL 在本地缓存最近一次成功拉取的订阅状态和拉取时间戳，用于 Grace_Window 计算

### 需求 12：后台健康守护与自恢复（SVC-3）

**用户故事：** 作为用户，我需要 Client 在进程崩溃、网络瞬断、短时依赖异常后能自动回到稳定态，以便不遗留半残路由或僵尸状态。

#### 验收标准

1. THE Health_Guardian SHALL 按可配置的周期（默认 30 秒）执行以下健康检查：TUN 设备是否存在且可读写、QUIC 连接是否存活、KillSwitch 路由是否与当前 Gateway 一致、Entitlement 是否在有效期内
2. WHEN 健康检查发现 TUN 设备丢失时，THE Health_Guardian SHALL 尝试重新创建 TUN 设备并重新激活 KillSwitch
3. WHEN 健康检查发现路由不一致时，THE Health_Guardian SHALL 尝试修复路由使其指向当前 Gateway
4. THE Daemon_Manager SHALL 配置系统服务的自动重启策略：Linux systemd Restart=on-failure + RestartSec=5s，Windows Recovery 设置为自动重启
5. WHEN Client 异常退出时，THE KillSwitch SHALL 执行 Deactivate 恢复原始路由（通过 defer 和 signal handler 双重保护）

### 需求 13：去掉 8.8.8.8:53 硬编码物理网卡探测（ROUTE-3）

**用户故事：** 作为用户，我需要在禁止访问 8.8.8.8 的受限网络环境中仍可完成建链，以便出口接口选择不依赖外部公共地址。

#### 验收标准

1. THE Physical_NIC_Detector SHALL 替换当前 `net.Dial("udp4", "8.8.8.8:53")` 探测逻辑，改为基于系统路由表查询默认网关出口接口
2. THE Physical_NIC_Detector SHALL 在 Linux 上通过解析 `ip route get <gateway_ip>` 获取出口接口和源 IP
3. THE Physical_NIC_Detector SHALL 在 Windows 上通过 `route print` 或 Win32 API `GetBestRoute2` 获取出口接口
4. IF 系统路由表查询失败，THEN THE Physical_NIC_Detector SHALL 回退到枚举非 loopback、非 TUN 的网络接口，选择第一个具有有效 IPv4 地址的接口
5. THE Physical_NIC_Detector SHALL 不向任何外部地址发送探测包

### 需求 14：启动/异常退出/切换失败时路由回滚保护（ROUTE-4）

**用户故事：** 作为用户，我需要大多数异常退出场景都能自动恢复路由，以便设备不会因为 Client 异常而长期停留在错误路由状态。

#### 验收标准

1. WHEN KillSwitch Activate 中途失败（步骤 2-4 任一失败）时，THE KillSwitch SHALL 回滚已执行的步骤，恢复到 Activate 前的路由状态
2. WHEN 网关切换事务中途失败时，THE KillSwitch SHALL 回滚预加的路由，保持切换前的路由状态不变
3. WHEN Client 进程收到 SIGKILL 或异常崩溃时，THE Daemon_Manager SHALL 在下次启动时检测残留路由（通过检查 TUN 设备和 /32 host route 是否存在），并执行清理
4. THE KillSwitch SHALL 在 Activate 成功后将当前路由状态（原始网关、原始接口、当前 Gateway IP）持久化到临时文件，用于崩溃恢复
5. WHEN 残留路由清理完成后，THE Client SHALL 从正常启动流程继续，不要求用户手动重启设备
