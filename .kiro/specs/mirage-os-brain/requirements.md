# 需求文档：Phase 2 — Mirage-OS 大脑 MVP

## 简介

本阶段为 Mirage Project 四阶段实施的第二阶段，目标是构建最小可用控制中心（Mirage-OS），打通"Gateway 心跳/流量上报 → 结算扣费 → 配额熔断"的完整商业闭环，以及"威胁情报聚合 → 全局黑名单秒级分发"的安全闭环。

Mirage-OS 采用双服务架构：
- **gateway-bridge**（Go 服务）：实现 gRPC 服务端（接收 Gateway 上行数据）、配额熔断决策、全局黑名单聚合分发、策略下发
- **api-server**（NestJS + Prisma 服务）：实现用户认证（邀请制 + TOTP）、用户管理、蜂窝管理、计费查询/充值、域名状态查看、威胁情报查询、节点状态查询

两个服务共享同一个 PostgreSQL 数据库（Go 写流水/配额，NestJS 读流水 + 写业务数据），通过 Redis 进行 Gateway 连接状态缓存和黑名单分发 Pub/Sub。

Phase 1（gateway-closure）已完成 Gateway 侧的 gRPC 客户端/服务端、Protobuf 定义（proto/mirage.proto）。本阶段实现 OS 侧的对应服务端和客户端。

## 术语表

- **OS**：Mirage-OS，全局控制中心，本阶段构建的目标系统
- **Gateway**：Mirage-Gateway，运行于边缘节点的融合网关（Phase 1 已完成）
- **Gateway_Bridge**：Go 服务进程，负责 gRPC 通信和核心调度逻辑
- **API_Server**：NestJS 服务进程，负责业务 CRUD API
- **GRPC_Service**：Gateway_Bridge 中的 gRPC 服务端，实现 GatewayUplink 服务（接收心跳/流量/威胁上报）
- **Quota_Enforcer**：配额熔断器，在流量结算后判断剩余配额，配额归零时通过心跳响应通知 Gateway 阻断流量
- **Intel_Distributor**：威胁情报分发器，聚合所有 Gateway 上报的威胁 IP，生成全局黑名单并通过 Redis Pub/Sub 分发
- **Strategy_Dispatcher**：策略下发器，通过 GatewayDownlink 向 Gateway 推送防御等级和拟态模板参数
- **Billing_Module**：NestJS 计费模块，提供流量流水查询和配额充值 API
- **Auth_Module**：NestJS 认证模块，实现邀请制注册、TOTP 双因素认证、JWT 会话管理
- **Users_Module**：NestJS 用户管理模块，用户 CRUD 和影子认证公钥绑定
- **Cells_Module**：NestJS 蜂窝管理模块，蜂窝创建、用户分配到蜂窝
- **Gateways_Module**：NestJS 节点管理模块，Gateway 在线状态和健康度查询
- **Domains_Module**：NestJS 域名管理模块，域名温储备池状态查看
- **Threats_Module**：NestJS 威胁情报模块，威胁事件列表查询
- **Proto_Definition**：Phase 1 定义的 Protobuf 文件（proto/mirage.proto），OS 实现其服务端
- **Prisma_Schema**：NestJS 的数据模型定义文件，映射 PostgreSQL 表结构
- **numeric_20_8**：PostgreSQL 的 numeric(20,8) 精度类型，用于所有货币字段防止 XMR 换算精度丢失
- **TC_ACT_STOLEN**：eBPF 内核态流量阻断动作，当 Gateway 收到 remaining_quota=0 时触发
- **LPM_Trie**：最长前缀匹配 Trie，Gateway 侧 eBPF Map 类型，用于高效 IP 前缀匹配
- **TOTP**：基于时间的一次性密码算法，用于双因素认证
- **JWT**：JSON Web Token，用于 API 会话管理
- **Invite_Code**：邀请码，邀请制注册的凭证，每个邀请码只能使用一次

## 需求

### 需求 1：Protobuf 共享与 gRPC 服务端

**用户故事：** 作为 Mirage-OS 系统，我需要实现 Phase 1 定义的 GatewayUplink gRPC 服务端，以便接收所有 Gateway 的心跳、流量上报和威胁上报。

#### 验收标准

1. THE Gateway_Bridge SHALL 复用 Phase 1 定义的 proto/mirage.proto 文件，实现 GatewayUplink 服务的服务端（SyncHeartbeat、ReportTraffic、ReportThreat）
2. WHEN 收到 SyncHeartbeat 请求时，THE GRPC_Service SHALL 更新该 Gateway 的在线状态和最后心跳时间到 PostgreSQL gateways 表，并将连接状态缓存到 Redis
3. WHEN 收到 ReportTraffic 请求时，THE GRPC_Service SHALL 将流量数据传递给 Quota_Enforcer 进行结算扣费
4. WHEN 收到 ReportThreat 请求时，THE GRPC_Service SHALL 将威胁事件写入 PostgreSQL threat_intel 表，并传递给 Intel_Distributor 进行聚合
5. THE GRPC_Service SHALL 在配置的端口上监听，支持 mTLS 双向认证（生产环境）和非 TLS 模式（开发环境）
6. IF 收到的 gRPC 请求中 gateway_id 为空或 timestamp 为 0，THEN THE GRPC_Service SHALL 返回 gRPC InvalidArgument 错误码

### 需求 2：配额熔断与流量结算

**用户故事：** 作为 Mirage-OS 系统，我需要对 Gateway 上报的流量进行实时结算和配额扣减，以便在用户配额耗尽时通知 Gateway 阻断流量。

#### 验收标准

1. WHEN 收到 ReportTraffic 请求时，THE Quota_Enforcer SHALL 根据业务流量字节数和防御流量字节数计算费用（业务流量 × 业务单价 + 防御流量 × 防御单价），费用精度为 numeric(20,8)
2. THE Quota_Enforcer SHALL 在单个 PostgreSQL 事务中原子执行以下三步操作：扣减用户 remaining_quota、插入 billing_logs 流水记录、更新用户 total_consumed 累计消费
3. WHEN 用户 remaining_quota 扣减后小于等于 0 时，THE Quota_Enforcer SHALL 在下一次 SyncHeartbeat 响应中返回 remaining_quota=0，触发 Gateway 内核态 TC_ACT_STOLEN 阻断
4. THE Quota_Enforcer SHALL 根据用户所属蜂窝的级别（标准/白金/钻石）应用对应的流量单价倍率
5. IF 流量上报中的 period_seconds 为 0 或 business_bytes 和 defense_bytes 均为 0，THEN THE Quota_Enforcer SHALL 跳过结算并返回成功响应
6. FOR ALL 有效的流量上报，结算前用户 remaining_quota 减去结算后用户 remaining_quota SHALL 等于本次计算的费用值（结算精度一致性）

### 需求 3：全局黑名单聚合与分发

**用户故事：** 作为 Mirage-OS 系统，我需要聚合所有 Gateway 上报的威胁 IP，生成全局黑名单并分发到所有在线 Gateway，以便实现全网秒级免疫。

#### 验收标准

1. WHEN 同一源 IP 在 threat_intel 表中的累计 hit_count 达到 100 次时，THE Intel_Distributor SHALL 将该 IP 标记为全局封禁（is_banned=true）
2. WHEN 有新的 IP 被标记为全局封禁时，THE Intel_Distributor SHALL 在 5 秒内通过 Redis Pub/Sub 发布黑名单更新事件到 mirage:blacklist 频道
3. THE Intel_Distributor SHALL 维护一个全局黑名单缓存（Redis），包含所有 is_banned=true 的 IP 及其 CIDR 前缀和过期时间
4. WHEN Gateway_Bridge 启动时，THE Intel_Distributor SHALL 从 PostgreSQL 加载所有已封禁 IP 到 Redis 缓存
5. WHEN 新 Gateway 连接并发送首次心跳时，THE GRPC_Service SHALL 通过 GatewayDownlink.PushBlacklist 将当前全局黑名单完整下发给该 Gateway
6. IF threat_intel 表中的记录超过 100 万条，THEN THE Intel_Distributor SHALL 自动清理 30 天前且 hit_count 小于 10 的记录

### 需求 4：策略下发

**用户故事：** 作为 Mirage-OS 系统，我需要向 Gateway 下发防御策略参数，以便根据全局态势调整各节点的防御强度。

#### 验收标准

1. THE Strategy_Dispatcher SHALL 实现 GatewayDownlink 服务的客户端，支持向指定 Gateway 推送 PushStrategy、PushBlacklist、PushQuota、PushReincarnation 指令
2. WHEN 管理员通过 API_Server 修改某蜂窝的防御等级时，THE Strategy_Dispatcher SHALL 在 10 秒内将新策略推送到该蜂窝下所有在线 Gateway
3. THE Strategy_Dispatcher SHALL 从 Redis 获取 Gateway 连接状态，仅向在线状态的 Gateway 推送策略
4. IF Gateway 在策略推送时不可达，THEN THE Strategy_Dispatcher SHALL 记录推送失败日志，并在该 Gateway 下次心跳时重新推送

### 需求 5：PostgreSQL 数据模型

**用户故事：** 作为系统架构师，我需要定义 Mirage-OS 的数据库表结构，以便 Go 服务和 NestJS 服务共享同一数据源。

#### 验收标准

1. THE Prisma_Schema SHALL 定义 users 表，包含以下字段：id（UUID 主键）、username（唯一）、password_hash、ed25519_pubkey（影子认证公钥）、totp_secret、remaining_quota（numeric_20_8）、total_deposit（numeric_20_8）、total_consumed（numeric_20_8）、cell_id（外键）、invite_code_used、is_active、created_at、updated_at
2. THE Prisma_Schema SHALL 定义 cells 表，包含以下字段：id（UUID 主键）、name（唯一）、region、level（枚举：STANDARD/PLATINUM/DIAMOND）、cost_multiplier（numeric_20_8）、max_users、max_domains、created_at
3. THE Prisma_Schema SHALL 定义 gateways 表，包含以下字段：id（字符串主键，即 gateway_id）、cell_id（外键）、ip_address、status（枚举：ONLINE/DEGRADED/OFFLINE）、last_heartbeat、ebpf_loaded、threat_level、active_connections、memory_usage_mb、created_at、updated_at
4. THE Prisma_Schema SHALL 定义 billing_logs 表，包含以下字段：id（UUID 主键）、user_id（外键）、gateway_id、business_bytes（bigint）、defense_bytes（bigint）、business_cost（numeric_20_8）、defense_cost（numeric_20_8）、total_cost（numeric_20_8）、period_seconds、created_at
5. THE Prisma_Schema SHALL 定义 threat_intel 表，包含以下字段：id（UUID 主键）、source_ip、source_port、threat_type、severity、hit_count、is_banned、first_seen、last_seen、reported_by_gateway
6. THE Prisma_Schema SHALL 定义 deposits 表，包含以下字段：id（UUID 主键）、user_id（外键）、amount（numeric_20_8）、currency、tx_hash、status（枚举：PENDING/CONFIRMED/FAILED）、created_at
7. THE Prisma_Schema SHALL 定义 quota_purchases 表，包含以下字段：id（UUID 主键）、user_id（外键）、quota_gb（numeric_20_8）、price（numeric_20_8）、cell_level、created_at
8. THE Prisma_Schema SHALL 定义 invite_codes 表，包含以下字段：id（UUID 主键）、code（唯一）、created_by（外键）、used_by（外键，可空）、is_used、created_at、used_at（可空）
9. THE Prisma_Schema SHALL 为所有货币字段使用 Decimal 类型映射到 PostgreSQL 的 numeric(20,8)

### 需求 6：用户认证（邀请制 + TOTP）

**用户故事：** 作为系统管理员，我需要实现邀请制注册和 TOTP 双因素认证，以便只有受邀用户才能访问系统，且登录过程具备双重安全保障。

#### 验收标准

1. WHEN 用户提交注册请求时，THE Auth_Module SHALL 验证邀请码的有效性（存在、未使用），验证通过后创建用户并将邀请码标记为已使用
2. IF 邀请码不存在或已被使用，THEN THE Auth_Module SHALL 返回 HTTP 400 错误，错误信息为"邀请码无效"
3. WHEN 用户首次注册成功后，THE Auth_Module SHALL 生成 TOTP 密钥并返回 TOTP 配置 URI（兼容 Google Authenticator 等标准 TOTP 应用）
4. WHEN 用户提交登录请求时，THE Auth_Module SHALL 验证用户名、密码和 TOTP 验证码三个因素，全部通过后签发 JWT 令牌
5. IF 用户名、密码或 TOTP 验证码任一验证失败，THEN THE Auth_Module SHALL 返回 HTTP 401 错误，错误信息不区分具体失败原因
6. THE Auth_Module SHALL 签发的 JWT 令牌有效期为 24 小时，包含 user_id 和 cell_id 声明
7. THE Auth_Module SHALL 对所有需要认证的 API 端点使用 JWT Guard 进行令牌验证

### 需求 7：用户管理

**用户故事：** 作为系统管理员，我需要管理用户信息和影子认证公钥绑定，以便用户能通过 Gateway 的 Ed25519 影子认证。

#### 验收标准

1. THE Users_Module SHALL 提供用户列表查询 API（GET /api/users），支持分页参数（page、limit）
2. THE Users_Module SHALL 提供用户详情查询 API（GET /api/users/:id），返回用户基本信息、所属蜂窝、配额余额
3. WHEN 用户提交 Ed25519 公钥绑定请求时，THE Users_Module SHALL 将 hex 编码的公钥存储到 users 表的 ed25519_pubkey 字段
4. THE Users_Module SHALL 提供用户停用 API（PATCH /api/users/:id/deactivate），将 is_active 设为 false
5. IF 请求的用户 ID 不存在，THEN THE Users_Module SHALL 返回 HTTP 404 错误

### 需求 8：计费模块

**用户故事：** 作为用户，我需要查询流量消费流水和充值配额，以便了解消费情况并在配额不足时及时充值。

#### 验收标准

1. THE Billing_Module SHALL 提供流量流水查询 API（GET /api/billing/logs），支持按时间范围和用户 ID 过滤，返回分页结果
2. THE Billing_Module SHALL 提供配额余额查询 API（GET /api/billing/quota），返回用户的 remaining_quota、total_deposit、total_consumed
3. WHEN 用户提交充值请求时（POST /api/billing/recharge），THE Billing_Module SHALL 在单个事务中创建 quota_purchases 记录并增加用户 remaining_quota
4. THE Billing_Module SHALL 提供充值记录查询 API（GET /api/billing/purchases），返回用户的历史充值记录
5. THE Billing_Module SHALL 在充值 API 中验证充值金额大于 0 且充值的流量包 quota_gb 大于 0
6. FOR ALL 充值操作，充值前 remaining_quota 加上充值 quota_gb 对应的金额 SHALL 等于充值后 remaining_quota（充值精度一致性）

### 需求 9：蜂窝管理

**用户故事：** 作为系统管理员，我需要创建和管理蜂窝，并将用户分配到蜂窝中，以便实现用户隔离和差异化服务。

#### 验收标准

1. THE Cells_Module SHALL 提供蜂窝创建 API（POST /api/cells），接受 name、region、level、max_users、max_domains 参数
2. THE Cells_Module SHALL 提供蜂窝列表查询 API（GET /api/cells），返回所有蜂窝及其用户数和 Gateway 数统计
3. WHEN 管理员将用户分配到蜂窝时（POST /api/cells/:id/assign），THE Cells_Module SHALL 更新用户的 cell_id 字段
4. IF 蜂窝的当前用户数已达到 max_users，THEN THE Cells_Module SHALL 返回 HTTP 409 错误，错误信息为"蜂窝已满"
5. THE Cells_Module SHALL 根据蜂窝 level 自动设置 cost_multiplier（STANDARD=1.0、PLATINUM=1.5、DIAMOND=2.0）

### 需求 10：Gateway 节点管理

**用户故事：** 作为系统管理员，我需要查看所有 Gateway 节点的在线状态和健康度，以便监控全网节点运行情况。

#### 验收标准

1. THE Gateways_Module SHALL 提供节点列表查询 API（GET /api/gateways），返回所有 Gateway 的状态信息，支持按 cell_id 和 status 过滤
2. THE Gateways_Module SHALL 提供节点详情查询 API（GET /api/gateways/:id），返回 Gateway 的完整状态信息（包含 last_heartbeat、threat_level、active_connections、memory_usage_mb）
3. WHEN Gateway 的 last_heartbeat 超过 300 秒未更新时，THE Gateways_Module SHALL 将该 Gateway 的 status 标记为 OFFLINE

### 需求 11：域名状态查看

**用户故事：** 作为系统管理员，我需要查看域名温储备池的状态，以便了解可用域名资源。

#### 验收标准

1. THE Domains_Module SHALL 提供域名列表查询 API（GET /api/domains），返回域名列表及其状态（WARM/ACTIVE/RETIRED），支持按状态过滤
2. THE Domains_Module SHALL 提供域名统计 API（GET /api/domains/stats），返回各状态域名的数量统计

### 需求 12：威胁情报查询

**用户故事：** 作为系统管理员，我需要查询威胁情报列表，以便了解全网威胁态势。

#### 验收标准

1. THE Threats_Module SHALL 提供威胁情报列表查询 API（GET /api/threats），返回威胁事件列表，支持按 threat_type、is_banned、severity 过滤，返回分页结果
2. THE Threats_Module SHALL 提供威胁统计 API（GET /api/threats/stats），返回各类型威胁的数量统计和已封禁 IP 总数

### 需求 13：Docker Compose 编排

**用户故事：** 作为运维人员，我需要通过 docker-compose 一键启动整个 Mirage-OS 系统，以便快速部署和开发调试。

#### 验收标准

1. THE docker-compose.yaml SHALL 定义以下服务：gateway-bridge（Go）、api-server（NestJS）、postgres（PostgreSQL 15）、redis（Redis 7）
2. THE docker-compose.yaml SHALL 配置 PostgreSQL 使用持久化卷存储数据，并设置初始数据库名称、用户名和密码
3. THE docker-compose.yaml SHALL 配置服务间依赖关系：gateway-bridge 和 api-server 依赖 postgres 和 redis
4. THE docker-compose.yaml SHALL 为 gateway-bridge 暴露 gRPC 端口（50051），为 api-server 暴露 HTTP 端口（3000）
5. THE docker-compose.yaml SHALL 通过环境变量将 PostgreSQL 和 Redis 连接信息传递给 gateway-bridge 和 api-server

### 需求 14：配置管理

**用户故事：** 作为运维人员，我需要通过统一的配置文件管理 Mirage-OS 的运行参数，以便灵活调整系统行为。

#### 验收标准

1. THE Gateway_Bridge SHALL 从 configs/mirage-os.yaml 读取配置，包含以下配置段：grpc（端口、TLS 配置）、database（PostgreSQL 连接串）、redis（连接地址）、quota（业务单价、防御单价）、intel（封禁阈值、清理周期）
2. THE API_Server SHALL 从环境变量读取配置，包含：DATABASE_URL（PostgreSQL 连接串）、REDIS_URL（Redis 连接地址）、JWT_SECRET、PORT
3. WHEN 配置文件中的必填字段缺失时，THE Gateway_Bridge SHALL 在启动时输出明确的错误信息并终止进程
