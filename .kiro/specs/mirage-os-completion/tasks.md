# 实现计划：Mirage-OS 控制面 P0 待办任务完成

## 概述

将 Mirage-OS 控制面 13 项 P0 待办任务从 TODO/mock 替换为生产级 Go 实现。按模块递进：先建接口与基础设施，再实现 gRPC 服务层，然后完成 Raft 安全层与威胁检测，最后集成 CellScheduler 通信和 Monero 支付。所有外部依赖通过接口注入，使用 `pgregory.net/rapid` 进行属性测试。

## Tasks

- [x] 1. GatewayDownlink 下行服务实现
  - [x] 1.1 实现 GatewayConnectionManager 和 DownlinkService（Desired State 模型）
    - 创建 `mirage-os/gateway-bridge/pkg/grpc/downlink.go`
    - 实现 `GatewayConnectionManager`：维护 `map[string]*grpc.ClientConn`，提供 `GetConn/CloseConn` 方法
    - 实现 Desired State 模型：每次策略/黑名单/配额变更时覆盖 Redis Hash `gateway:{id}:desired_state` 并计算 StateHash
    - SyncHeartbeat 时对比 Gateway 上报的 CurrentStateHash 与 Redis 中的 StateHash，不一致则下发全量 Desired State
    - 对于一次性事件（PushReincarnation），使用 Redis List `mirage:downlink:events:{gatewayID}` 带 TTL 和去重
    - 从 Redis `gateway:{id}:addr` 获取 Gateway gRPC 地址建立连接
    - 嵌入 `pb.UnimplementedGatewayDownlinkServer`，使用 `status.Error` 返回错误
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_

  - [x]* 1.2 编写 DownlinkService 单元测试
    - 创建 `mirage-os/gateway-bridge/pkg/grpc/downlink_test.go`
    - 测试推送成功路径、Gateway 不可达时加入重试队列、重试逻辑
    - 使用 mock gRPC 服务端验证推送内容
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_

- [x] 2. CellService gRPC 服务实现
  - [x] 2.1 实现 CellServiceImpl 全部 RPC 方法
    - 创建 `mirage-os/services/cellular/cell_service.go`
    - 实现 `RegisterCell`：验证 cell_id 非空、等级非 LEVEL_UNKNOWN、location.country 非空；GORM Create 持久化；cell_id 重复返回 AlreadyExists
    - 实现 `ListCells`：按等级/国家/online_only 筛选，查询 Gateway 数量和负载率
    - 实现 `AllocateGateway`：按 preferred_level + preferred_country 筛选，按 load_percent ASC 取最优蜂窝；`crypto/rand` 生成 32 字节 hex 连接令牌
    - 实现 `HealthCheck`：查询蜂窝 Gateway 数量、失败数、平均延迟、24h 威胁计数
    - 实现 `SwitchCell`：未指定 target_cell_id 时自动选择同等级、不同管辖区、负载最低的蜂窝
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_

  - [x]* 2.2 编写属性测试：RegisterCell 输入验证完整性
    - **Property 1: RegisterCell 输入验证完整性**
    - 创建 `mirage-os/services/cellular/cell_service_test.go`
    - 使用 rapid 生成随机 RegisterCellRequest，验证无效输入返回 InvalidArgument 且数据库无新增，有效输入返回 success 且数据库存在记录
    - **Validates: Requirements 2.1**

  - [x]* 2.3 编写属性测试：ListCells 筛选条件正确性
    - **Property 2: ListCells 筛选条件正确性**
    - 生成随机蜂窝数据集和筛选条件，验证返回的每个蜂窝都满足所有指定筛选条件
    - **Validates: Requirements 2.2**

  - [x]* 2.4 编写属性测试：AllocateGateway 最优选择
    - **Property 3: AllocateGateway 最优选择**
    - 生成随机蜂窝数据集和 AllocateRequest，验证分配结果是满足条件中负载最低的蜂窝
    - **Validates: Requirements 2.3**

  - [x]* 2.5 编写属性测试：SwitchCell 目标蜂窝约束
    - **Property 4: SwitchCell 目标蜂窝约束**
    - 生成随机蜂窝数据集和 SwitchCellRequest（未指定 target），验证目标蜂窝满足同等级、不同管辖区、active 状态、负载最低
    - **Validates: Requirements 2.5**

- [x] 3. BillingService gRPC 服务实现
  - [x] 3.1 实现 BillingServiceImpl 全部 RPC 方法
    - 创建 `mirage-os/services/billing/billing_service.go`
    - 定义流量包价格常量 map：`PackageType × CellLevel → (priceUSD, quotaBytes)`
    - 实现 `CreateAccount`：验证 user_id/public_key 唯一性，GORM Create 用户记录，重复返回 AlreadyExists；通过 monero-wallet-rpc `create_address` 为用户生成专属子地址并存储
    - 实现 `Deposit`：仅作为手动触发确认检查的辅助接口；主充值流程由 MoneroManager.MonitorDeposits 通过 `get_transfers` 自动检测子地址入账（Sub-address 隔离，阻断 TxHash 重放抢占）
    - 实现 `GetBalance`：查询用户余额、配额、蜂窝分配
    - 实现 `PurchaseQuota`：数据库事务中验证余额 → 扣减 → 创建 QuotaPurchase → 增加配额；余额不足返回 FailedPrecondition
    - 实现 `GetBillingLogs`：按时间范围和 limit 查询 billing_logs
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_

  - [x]* 3.2 编写属性测试：PurchaseQuota 余额不变量
    - **Property 5: PurchaseQuota 余额不变量**
    - 创建 `mirage-os/services/billing/billing_service_test.go`
    - 生成随机余额、流量包类型、蜂窝等级，验证购买后余额 = B - price 且配额增加，或余额不足时被拒绝且不变
    - **Validates: Requirements 3.4**

- [x] 4. Checkpoint - 确保 gRPC 服务层测试通过
  - 确保所有测试通过，如有问题请向用户确认。

- [x] 5. GatewayService gRPC 服务实现
  - [x] 5.1 实现 GatewayServiceImpl 全部 RPC 方法
    - 创建 `mirage-os/services/api-gateway/gateway_service.go`
    - 实现 `SyncHeartbeat`：GORM Upsert Gateway 状态 + Redis SET `gateway:{id}:status` TTL 60s；返回 DefenseConfig 和剩余配额
    - 实现 `ReportTraffic`：Redis DECRBY 原子扣减配额；计算 cost_usd；剩余 < 总量×10% 时 quota_warning=true
    - 实现 `ReportThreat`：持久化到 threat_intel 表；severity 映射：≥8→EMERGENCY_SHUTDOWN，≥6→SWITCH_CELL，≥4→BLOCK_IP，≥2→INCREASE_DEFENSE，<2→NONE
    - 实现 `GetQuota`：查询用户配额、过期时间、auto_renew、蜂窝配额
    - _Requirements: 4.1, 4.2, 4.3, 4.4_

  - [x]* 5.2 编写属性测试：流量配额扣减与告警
    - **Property 6: 流量配额扣减与告警**
    - 创建 `mirage-os/services/api-gateway/gateway_service_test.go`
    - 生成随机初始配额和流量值，验证扣减后剩余 = Q-T（不低于0），quota_warning 在 <10% 时为 true
    - **Validates: Requirements 4.2**

  - [x]* 5.3 编写属性测试：威胁严重程度到动作的映射
    - **Property 7: 威胁严重程度到动作的映射**
    - 生成随机 severity 0-10，验证返回的 ThreatAction 符合阈值映射规则
    - **Validates: Requirements 4.3**

- [x] 6. HotKeyManager Shamir 份额收集与密钥轮换
  - [x] 6.1 定义 ShareProvider/ShareDistributor 接口并实现 collectShares
    - 修改 `mirage-os/pkg/raft/hot_key_manager.go`
    - 定义 `ShareProvider` 接口（`RequestShare`、`GetOnlineNodes`）
    - 定义 `ShareDistributor` 接口（`DistributeShare`）
    - 实现 `RaftShareProvider`：通过 Raft Transport 请求份额
    - 重写 `collectShares`：并发请求，单节点超时 5s，验证份额 Index∈[1,5] 且 Value 长度 32 字节，收集 ≥3 个有效份额
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5_

  - [x] 6.2 实现 RefreshHotKey 密钥轮换逻辑（Raft Log 强一致性分发）
    - 修改 `mirage-os/pkg/raft/hot_key_manager.go` 的 `RefreshHotKey`
    - 定义 `KeyRotationCommand` 结构体（type、encrypted_shares map、epoch）
    - 流程：备份旧份额 → crypto/rand 生成 32 字节新密钥 → SplitSecret 3-of-5 → 用各节点公钥加密份额 → 打包为 KeyRotationCommand → raft.Apply(command) 提交到 Raft 日志
    - FSM Apply 时各节点解密并存储自己的份额
    - 仅当过半节点 commit 后才生效，raft.Apply 失败则保持旧密钥不变
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6_

  - [x]* 6.3 编写属性测试：Shamir 份额验证
    - **Property 8: Shamir 份额验证**
    - 创建 `mirage-os/pkg/raft/hot_key_manager_test.go`
    - 生成随机 Index 和 Value，验证仅当 Index∈[1,5] 且 len(Value)==32 时判定有效
    - **Validates: Requirements 5.4**

  - [x]* 6.4 编写属性测试：Shamir 秘密分享 Round-Trip
    - **Property 9: Shamir 秘密分享 Round-Trip**
    - 生成随机 32 字节密钥，SplitSecret 后任取 3 份 CombineShares，验证恢复结果等于原始密钥
    - **Validates: Requirements 6.3**

- [x] 7. GeoFence 威胁检测实现
  - [x] 7.1 定义 Provider 接口并实现威胁分层架构
    - 修改 `mirage-os/pkg/raft/geo_fence.go`
    - 定义 `NetworkStatsProvider`、`GovernmentIPChecker`、`DataCenterAccessProvider` 接口
    - 拆分为 `ControlPlaneThreatAnalyzer`（政府审计/物理入侵/路由异常 → 触发 Raft 退位）和 `GatewayThreatAnalyzer`（DDoS/SYN Flood/异常连接 → 触发蜂窝调度）
    - 实现 `detectGovernmentAudit`：检测政府 IP 段连接 + 非计划物理访问 + 路由跳数异常（与基线差异 >2 跳）
    - 仅 ControlPlane 级威胁触发 Raft LeadershipTransfer，Gateway 级威胁仅触发蜂窝调度
    - _Requirements: 8.1, 8.2, 8.3, 8.4_

  - [x] 7.2 实现 detectDDoS（Gateway 级威胁，不触发 Raft 退位）
    - 实现 `detectDDoS`：获取流量基线，current > baseline×5 或 SYN 速率 >10000/s 或 UDP 速率 >50000/s 时返回 true
    - 检测到时通过 CellScheduler 触发蜂窝熔断/IP 切换/防御升级，不影响 Raft 集群
    - _Requirements: 9.1, 9.2, 9.3, 9.4_

  - [x] 7.3 实现 detectAnomalousTraffic（Gateway 级威胁，不触发 Raft 退位）
    - 实现 `detectAnomalousTraffic`：流量偏差 >3σ 或单 IP 连接 >1000 或非预期地理区域连接超阈值时返回 true
    - 检测到时通过 CellScheduler 触发蜂窝调度，不影响 Raft 集群
    - _Requirements: 10.1, 10.2, 10.3, 10.4_

  - [x] 7.4 完善 checkThreatLevel 集成（仅 ControlPlane 级威胁触发退位）
    - 修改 `mirage-os/pkg/raft/cluster.go` 的 `checkThreatLevel`
    - 仅使用 ControlPlaneThreatAnalyzer 的结果决定是否触发 Raft LeadershipTransfer
    - GatewayThreatAnalyzer 的结果通过事件通知 CellScheduler 处理，不影响 Raft 集群
    - level≥8 且 IsLeader 时触发 StepDown（仅限政府审计/物理入侵等控制面威胁）
    - _Requirements: 7.1, 7.2, 7.3_

  - [x]* 7.5 编写属性测试：威胁等级触发退位
    - **Property 10: 威胁等级触发退位**
    - 创建 `mirage-os/pkg/raft/geo_fence_test.go`
    - 生成随机 level 和 isLeader，验证仅当 level≥8 且 isLeader=true 时触发退位
    - **Validates: Requirements 7.2**

  - [x]* 7.6 编写属性测试：综合威胁等级计算
    - **Property 11: 综合威胁等级计算**
    - 生成随机 ThreatIndicator 列表，验证 calculateOverallThreat 返回最近 5 分钟内最高 Severity
    - **Validates: Requirements 7.3**

  - [x]* 7.7 编写属性测试：政府审计网络检测
    - **Property 12: 政府审计网络检测**
    - 生成随机连接列表和政府 IP 段，验证检测逻辑正确性
    - **Validates: Requirements 8.1, 8.3**

  - [x]* 7.8 编写属性测试：DDoS 攻击检测
    - **Property 13: DDoS 攻击检测**
    - 生成随机 baseline/current/synRate/udpRate，验证检测阈值逻辑
    - **Validates: Requirements 9.1, 9.2, 9.3**

  - [x]* 7.9 编写属性测试：异常流量检测
    - **Property 14: 异常流量检测**
    - 生成随机基线统计和当前值，验证 3σ 偏离、单 IP >1000、地理异常检测
    - **Validates: Requirements 10.1, 10.2, 10.3**

- [x] 8. Checkpoint - 确保 Raft 安全层和威胁检测测试通过
  - 确保所有测试通过，如有问题请向用户确认。

- [x] 9. CellScheduler DownlinkClient 集成
  - [x] 9.1 定义 DownlinkClient 接口并集成到 CellScheduler
    - 修改 `mirage-os/pkg/strategy/cell_manager.go`
    - 定义 `DownlinkClient` 接口（`PushStrategy`、`PushQuota`）
    - 为 CellScheduler 注入 DownlinkClient
    - `RegisterGateway` → `PushStrategy(noise_intensity=80)` 启动 VPC 噪声注入
    - `promoteToCalibration` → `PushStrategy(探测参数)` 启动网络质量测量
    - `PromoteToActive` → `PushStrategy(template_id=X)` 下发 B-DNA 模板
    - 推送失败时加入待推送队列
    - _Requirements: 11.1, 11.2, 11.3, 11.4_

- [x] 10. Monero RPC 客户端与汇率查询
  - [x] 10.1 实现 MoneroRPCClient 接口和 HTTPMoneroRPCClient
    - 修改 `mirage-os/services/billing/monero_manager.go`
    - 定义 `MoneroRPCClient` 接口（`GetTransferByTxID`）
    - 实现 `HTTPMoneroRPCClient`：net/http 发送 JSON-RPC 2.0 请求 `get_transfer_by_txid`，解析 confirmations
    - 重写 `getTransactionConfirmations` 使用注入的 MoneroRPCClient
    - 重写 `GetExchangeRate` 使用注入的 ExchangeRateProvider
    - _Requirements: 12.1, 12.2, 12.3, 12.4_

  - [x] 10.2 实现 ExchangeRateProvider 和 CachedExchangeRateProvider
    - 创建 `mirage-os/services/billing/exchange_rate.go`
    - 定义 `ExchangeRateProvider` 接口
    - 实现 `CoinGeckoProvider`：GET `/api/v3/simple/price?ids=monero&vs_currencies=usd`
    - 实现 `KrakenProvider`：GET `/0/public/Ticker?pair=XMRUSD`
    - 实现 `CachedExchangeRateProvider`：Redis 缓存 key `mirage:xmr_usd_rate` TTL 5min，primary→fallback 回退
    - _Requirements: 13.1, 13.2, 13.3, 13.4, 13.5_

  - [x]* 10.3 编写属性测试：Monero RPC 响应解析
    - **Property 15: Monero RPC 响应解析**
    - 创建 `mirage-os/services/billing/monero_rpc_test.go`
    - 生成随机 JSON-RPC 2.0 响应，验证解析出的 confirmations 与响应中字段一致
    - **Validates: Requirements 12.2**

  - [x]* 10.4 编写汇率查询单元测试
    - 创建 `mirage-os/services/billing/exchange_rate_test.go`
    - 测试缓存命中/未命中、CoinGecko→Kraken 回退、两者都失败返回错误
    - _Requirements: 13.1, 13.2, 13.3, 13.4, 13.5_

- [x] 11. Final Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请向用户确认。

## Notes

- 标记 `*` 的子任务为可选，可跳过以加速 MVP
- 每个任务引用具体需求编号，确保可追溯性
- 属性测试使用 `pgregory.net/rapid`，每个属性至少 100 次迭代
- 所有 gRPC 服务嵌入 `UnimplementedXxxServer`，错误使用 `status.Error`
- 数据库操作使用 GORM，Redis 使用 `github.com/redis/go-redis/v9`
- 所有外部依赖通过接口注入，测试时使用 mock 实现
