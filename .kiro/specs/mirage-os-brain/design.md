# 设计文档：Phase 2 — Mirage-OS 大脑 MVP

## 概述

本设计覆盖 Mirage-OS 控制中心的两个核心服务：
- **gateway-bridge**（Go）：gRPC 服务端 + 配额熔断 + 黑名单分发 + 策略下发
- **api-server**（NestJS + Prisma）：用户认证/管理、蜂窝管理、计费查询/充值、域名/威胁/节点查询

设计约束：
- Go gateway-bridge 直接写 PostgreSQL（billing_logs、quota 扣减、threat_intel）
- NestJS api-server 通过 Prisma ORM 读写 PostgreSQL（业务 CRUD）
- 共享数据库，Go 写流水，NestJS 读流水 + 写业务
- 所有货币字段使用 numeric(20,8)
- Redis 用于 Gateway 连接状态缓存和黑名单 Pub/Sub 分发
- Proto 文件复用 Phase 1 定义，OS 实现服务端

## 架构

### 系统架构

```mermaid
graph TD
    GW1[Gateway #1] -->|gRPC mTLS| GB[gateway-bridge<br/>Go :50051]
    GW2[Gateway #2] -->|gRPC mTLS| GB
    GWN[Gateway #N] -->|gRPC mTLS| GB

    GB -->|写: billing_logs, quota, threat_intel| PG[(PostgreSQL)]
    GB -->|读写: 连接状态, 黑名单缓存| RD[(Redis)]
    GB -->|PushStrategy/PushBlacklist| GW1
    GB -->|PushStrategy/PushBlacklist| GW2

    API[api-server<br/>NestJS :3000] -->|Prisma ORM| PG
    API -->|读: 连接状态| RD

    CLIENT[管理员/用户] -->|REST API + JWT| API
```

### 数据流：生死裁决闭环

```mermaid
sequenceDiagram
    participant GW as Gateway
    participant GB as gateway-bridge
    participant PG as PostgreSQL
    participant RD as Redis

    loop 每 30 秒
        GW->>GB: SyncHeartbeat(gateway_id, status, ...)
        GB->>PG: UPDATE gateways SET last_heartbeat=now()
        GB->>RD: SET gateway:{id}:status = ONLINE (TTL 60s)
        GB->>PG: SELECT remaining_quota FROM users WHERE ...
        GB-->>GW: HeartbeatResponse(remaining_quota)
    end

    loop 每 60 秒
        GW->>GB: ReportTraffic(business_bytes, defense_bytes)
        GB->>PG: BEGIN TX
        Note over GB,PG: 1. 计算费用<br/>2. UPDATE users SET remaining_quota -= cost<br/>3. INSERT billing_logs<br/>4. UPDATE users SET total_consumed += cost
        GB->>PG: COMMIT
        alt remaining_quota <= 0
            GB-->>GW: HeartbeatResponse(remaining_quota=0)
            Note over GW: 写入 quota_map=0<br/>触发 TC_ACT_STOLEN
        end
    end
```

### 数据流：全局黑名单分发

```mermaid
sequenceDiagram
    participant GW1 as Gateway #1
    participant GW2 as Gateway #2
    participant GB as gateway-bridge
    participant PG as PostgreSQL
    participant RD as Redis

    GW1->>GB: ReportThreat(source_ip=1.2.3.4)
    GB->>PG: UPSERT threat_intel SET hit_count += 1
    alt hit_count >= 100
        GB->>PG: UPDATE threat_intel SET is_banned=true
        GB->>RD: PUBLISH mirage:blacklist {ip: 1.2.3.4}
        GB->>RD: SADD mirage:blacklist:global 1.2.3.4
        GB-->>GW1: PushBlacklist([1.2.3.4/32])
        GB-->>GW2: PushBlacklist([1.2.3.4/32])
    end
```

## 组件与接口

### 1. gateway-bridge（Go 服务）

#### 目录结构

```
gateway-bridge/
├── cmd/bridge/main.go
├── pkg/
│   ├── grpc/
│   │   └── server.go           # GatewayUplink 服务端实现
│   ├── quota/
│   │   └── enforcer.go         # 配额熔断 + 流量结算
│   ├── intel/
│   │   └── distributor.go      # 全局黑名单聚合 + 分发
│   ├── dispatch/
│   │   └── strategy.go         # 策略下发（GatewayDownlink 客户端）
│   ├── config/
│   │   └── config.go           # YAML 配置加载
│   └── store/
│       └── postgres.go         # PostgreSQL 直连（database/sql + lib/pq）
├── proto/
│   └── mirage.proto            # 复用 Phase 1 定义
├── go.mod
└── Dockerfile
```

#### cmd/bridge/main.go

```go
func main() {
    // 1. 加载配置
    cfg := config.Load("configs/mirage-os.yaml")

    // 2. 连接 PostgreSQL
    db := store.NewPostgres(cfg.Database.DSN)

    // 3. 连接 Redis
    rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})

    // 4. 初始化核心模块
    enforcer := quota.NewEnforcer(db, cfg.Quota)
    distributor := intel.NewDistributor(db, rdb, cfg.Intel)
    dispatcher := dispatch.NewStrategyDispatcher(rdb)

    // 5. 启动 gRPC 服务
    grpcServer := grpc.NewServer(cfg.GRPC, enforcer, distributor, dispatcher, db, rdb)
    grpcServer.Start()

    // 6. 启动黑名单 Redis 订阅（用于多实例同步）
    distributor.StartSubscriber(ctx)

    // 7. 优雅退出
    gracefulShutdown(grpcServer, db, rdb)
}
```

#### pkg/grpc/server.go

```go
// Server 实现 GatewayUplink 服务端
type Server struct {
    proto.UnimplementedGatewayUplinkServer
    enforcer    *quota.Enforcer
    distributor *intel.Distributor
    dispatcher  *dispatch.StrategyDispatcher
    db          *sql.DB
    rdb         *redis.Client
    port        int
    tlsEnabled  bool
    tlsCert     string
    tlsKey      string
    tlsCA       string
}

func NewServer(cfg config.GRPCConfig, enforcer *quota.Enforcer,
    distributor *intel.Distributor, dispatcher *dispatch.StrategyDispatcher,
    db *sql.DB, rdb *redis.Client) *Server

func (s *Server) Start() error

func (s *Server) Stop()

// SyncHeartbeat 处理心跳
func (s *Server) SyncHeartbeat(ctx context.Context, req *proto.HeartbeatRequest) (*proto.HeartbeatResponse, error)
// - 校验 gateway_id 非空、timestamp 非零
// - UPDATE gateways SET status, last_heartbeat, ebpf_loaded, threat_level, active_connections, memory_usage_mb
// - SET Redis gateway:{id}:status = ONLINE (TTL 60s)
// - SELECT remaining_quota FROM users JOIN gateways
// - 返回 HeartbeatResponse{remaining_quota}

// ReportTraffic 处理流量上报
func (s *Server) ReportTraffic(ctx context.Context, req *proto.TrafficRequest) (*proto.TrafficResponse, error)
// - 校验 gateway_id 非空
// - 调用 enforcer.Settle(gateway_id, business_bytes, defense_bytes, period_seconds)
// - 返回 TrafficResponse{ack: true}

// ReportThreat 处理威胁上报
func (s *Server) ReportThreat(ctx context.Context, req *proto.ThreatRequest) (*proto.ThreatResponse, error)
// - 校验 gateway_id 非空
// - 遍历 events，UPSERT threat_intel
// - 调用 distributor.CheckAndBan(source_ip)
// - 返回 ThreatResponse{ack: true}
```

#### pkg/quota/enforcer.go

```go
// Enforcer 配额熔断器
type Enforcer struct {
    db            *sql.DB
    businessPrice float64  // 业务流量单价 $/GB，默认 0.10
    defensePrice  float64  // 防御流量单价 $/GB，默认 0.05
}

// PricingConfig 定价配置
type PricingConfig struct {
    BusinessPricePerGB float64 `yaml:"business_price_per_gb"`
    DefensePricePerGB  float64 `yaml:"defense_price_per_gb"`
}

func NewEnforcer(db *sql.DB, cfg PricingConfig) *Enforcer

// CalculateCost 计算费用（纯函数，可属性测试）
// cost = (businessBytes / 1e9) * businessPrice * multiplier + (defenseBytes / 1e9) * defensePrice * multiplier
func (e *Enforcer) CalculateCost(businessBytes, defenseBytes uint64, multiplier float64) (businessCost, defenseCost, totalCost float64)

// Settle 结算流量（事务原子操作）
// 1. 查询 gateway → cell → cost_multiplier
// 2. 查询 gateway → user
// 3. CalculateCost
// 4. BEGIN TX: UPDATE users remaining_quota -= totalCost, total_consumed += totalCost; INSERT billing_logs; COMMIT
// 5. 返回 (newRemainingQuota, error)
func (e *Enforcer) Settle(gatewayID string, businessBytes, defenseBytes uint64, periodSeconds int32) (remainingQuota float64, err error)

// GetRemainingQuota 查询用户剩余配额（通过 gateway_id 关联）
func (e *Enforcer) GetRemainingQuota(gatewayID string) (float64, error)
```

#### pkg/intel/distributor.go

```go
// Distributor 全局黑名单分发器
type Distributor struct {
    db           *sql.DB
    rdb          *redis.Client
    banThreshold int  // 默认 100
}

func NewDistributor(db *sql.DB, rdb *redis.Client, cfg config.IntelConfig) *Distributor

// RecordThreat 记录威胁事件（UPSERT threat_intel）
func (d *Distributor) RecordThreat(event *proto.ThreatEvent, gatewayID string) error

// CheckAndBan 检查 hit_count 是否达到阈值，达到则封禁并发布
func (d *Distributor) CheckAndBan(sourceIP string) (banned bool, err error)
// - SELECT hit_count FROM threat_intel WHERE source_ip = ?
// - if hit_count >= banThreshold: UPDATE is_banned=true, PUBLISH mirage:blacklist, SADD mirage:blacklist:global

// LoadBannedIPs 启动时从 PostgreSQL 加载所有已封禁 IP 到 Redis
func (d *Distributor) LoadBannedIPs() error

// GetGlobalBlacklist 获取当前全局黑名单（从 Redis 缓存读取）
func (d *Distributor) GetGlobalBlacklist() ([]string, error)

// StartSubscriber 启动 Redis Pub/Sub 订阅（多实例同步）
func (d *Distributor) StartSubscriber(ctx context.Context)

// Cleanup 清理旧记录（30 天前且 hit_count < 10）
func (d *Distributor) Cleanup() (int64, error)
```

#### pkg/dispatch/strategy.go

```go
// StrategyDispatcher 策略下发器
type StrategyDispatcher struct {
    rdb         *redis.Client
    connections map[string]*grpcConn  // gateway_id → gRPC 连接
    mu          sync.RWMutex
    pendingPush map[string]*proto.StrategyPush  // 待重推的策略
}

func NewStrategyDispatcher(rdb *redis.Client) *StrategyDispatcher

// RegisterGateway 注册 Gateway 的下行连接信息
func (sd *StrategyDispatcher) RegisterGateway(gatewayID, downlinkAddr string) error

// PushStrategyToCell 向蜂窝下所有在线 Gateway 推送策略
func (sd *StrategyDispatcher) PushStrategyToCell(cellID string, strategy *proto.StrategyPush) error
// - 从 Redis 获取该 cell 下所有在线 gateway
// - 逐个推送，失败的记录到 pendingPush

// PushBlacklistToAll 向所有在线 Gateway 推送黑名单
func (sd *StrategyDispatcher) PushBlacklistToAll(entries []*proto.BlacklistEntryProto) error

// PushQuotaToGateway 向指定 Gateway 推送配额
func (sd *StrategyDispatcher) PushQuotaToGateway(gatewayID string, remainingBytes uint64) error

// RetryPending 重试待推送的策略（在 Gateway 心跳时调用）
func (sd *StrategyDispatcher) RetryPending(gatewayID string) error
```

#### pkg/config/config.go

```go
type Config struct {
    GRPC     GRPCConfig     `yaml:"grpc"`
    Database DatabaseConfig `yaml:"database"`
    Redis    RedisConfig    `yaml:"redis"`
    Quota    PricingConfig  `yaml:"quota"`
    Intel    IntelConfig    `yaml:"intel"`
}

type GRPCConfig struct {
    Port       int    `yaml:"port"`        // 默认 50051
    TLSEnabled bool   `yaml:"tls_enabled"`
    CertFile   string `yaml:"cert_file"`
    KeyFile    string `yaml:"key_file"`
    CAFile     string `yaml:"ca_file"`
}

type DatabaseConfig struct {
    DSN string `yaml:"dsn"`
}

type RedisConfig struct {
    Addr     string `yaml:"addr"`
    Password string `yaml:"password"`
    DB       int    `yaml:"db"`
}

type IntelConfig struct {
    BanThreshold   int `yaml:"ban_threshold"`    // 默认 100
    CleanupDays    int `yaml:"cleanup_days"`     // 默认 30
    CleanupMinHits int `yaml:"cleanup_min_hits"` // 默认 10
}

func Load(path string) *Config
```

### 2. api-server（NestJS 服务）

#### 目录结构

```
api-server/
├── src/
│   ├── modules/
│   │   ├── auth/
│   │   │   ├── auth.module.ts
│   │   │   ├── auth.controller.ts
│   │   │   ├── auth.service.ts
│   │   │   ├── jwt.strategy.ts
│   │   │   └── jwt-auth.guard.ts
│   │   ├── users/
│   │   │   ├── users.module.ts
│   │   │   ├── users.controller.ts
│   │   │   └── users.service.ts
│   │   ├── cells/
│   │   │   ├── cells.module.ts
│   │   │   ├── cells.controller.ts
│   │   │   └── cells.service.ts
│   │   ├── billing/
│   │   │   ├── billing.module.ts
│   │   │   ├── billing.controller.ts
│   │   │   └── billing.service.ts
│   │   ├── domains/
│   │   │   ├── domains.module.ts
│   │   │   ├── domains.controller.ts
│   │   │   └── domains.service.ts
│   │   ├── threats/
│   │   │   ├── threats.module.ts
│   │   │   ├── threats.controller.ts
│   │   │   └── threats.service.ts
│   │   └── gateways/
│   │       ├── gateways.module.ts
│   │       ├── gateways.controller.ts
│   │       └── gateways.service.ts
│   ├── prisma/
│   │   ├── schema.prisma
│   │   └── prisma.service.ts
│   ├── app.module.ts
│   └── main.ts
├── package.json
├── tsconfig.json
├── nest-cli.json
└── Dockerfile
```

#### prisma/schema.prisma

```prisma
generator client {
  provider = "prisma-client-js"
}

datasource db {
  provider = "postgresql"
  url      = env("DATABASE_URL")
}

enum CellLevel {
  STANDARD
  PLATINUM
  DIAMOND
}

enum GatewayStatus {
  ONLINE
  DEGRADED
  OFFLINE
}

enum DepositStatus {
  PENDING
  CONFIRMED
  FAILED
}

model User {
  id              String    @id @default(uuid())
  username        String    @unique
  passwordHash    String    @map("password_hash")
  ed25519Pubkey   String?   @map("ed25519_pubkey")
  totpSecret      String?   @map("totp_secret")
  remainingQuota  Decimal   @default(0) @map("remaining_quota") @db.Decimal(20, 8)
  totalDeposit    Decimal   @default(0) @map("total_deposit") @db.Decimal(20, 8)
  totalConsumed   Decimal   @default(0) @map("total_consumed") @db.Decimal(20, 8)
  cellId          String?   @map("cell_id")
  cell            Cell?     @relation(fields: [cellId], references: [id])
  inviteCodeUsed  String?   @map("invite_code_used")
  isActive        Boolean   @default(true) @map("is_active")
  createdAt       DateTime  @default(now()) @map("created_at")
  updatedAt       DateTime  @updatedAt @map("updated_at")

  billingLogs     BillingLog[]
  deposits        Deposit[]
  quotaPurchases  QuotaPurchase[]
  createdInvites  InviteCode[]  @relation("CreatedBy")
  usedInvite      InviteCode?   @relation("UsedBy")

  @@map("users")
}

model Cell {
  id             String    @id @default(uuid())
  name           String    @unique
  region         String
  level          CellLevel @default(STANDARD)
  costMultiplier Decimal   @default(1.0) @map("cost_multiplier") @db.Decimal(20, 8)
  maxUsers       Int       @default(50) @map("max_users")
  maxDomains     Int       @default(15) @map("max_domains")
  createdAt      DateTime  @default(now()) @map("created_at")

  users    User[]
  gateways Gateway[]

  @@map("cells")
}

model Gateway {
  id                String        @id
  cellId            String?       @map("cell_id")
  cell              Cell?         @relation(fields: [cellId], references: [id])
  ipAddress         String?       @map("ip_address")
  status            GatewayStatus @default(OFFLINE)
  lastHeartbeat     DateTime?     @map("last_heartbeat")
  ebpfLoaded        Boolean       @default(false) @map("ebpf_loaded")
  threatLevel       Int           @default(0) @map("threat_level")
  activeConnections BigInt        @default(0) @map("active_connections")
  memoryUsageMb     Int           @default(0) @map("memory_usage_mb")
  createdAt         DateTime      @default(now()) @map("created_at")
  updatedAt         DateTime      @updatedAt @map("updated_at")

  billingLogs BillingLog[]

  @@map("gateways")
}

model BillingLog {
  id            String   @id @default(uuid())
  userId        String   @map("user_id")
  user          User     @relation(fields: [userId], references: [id])
  gatewayId     String   @map("gateway_id")
  gateway       Gateway  @relation(fields: [gatewayId], references: [id])
  businessBytes BigInt   @map("business_bytes")
  defenseBytes  BigInt   @map("defense_bytes")
  businessCost  Decimal  @map("business_cost") @db.Decimal(20, 8)
  defenseCost   Decimal  @map("defense_cost") @db.Decimal(20, 8)
  totalCost     Decimal  @map("total_cost") @db.Decimal(20, 8)
  periodSeconds Int      @map("period_seconds")
  createdAt     DateTime @default(now()) @map("created_at")

  @@map("billing_logs")
}

model ThreatIntel {
  id                String   @id @default(uuid())
  sourceIp          String   @map("source_ip")
  sourcePort        Int?     @map("source_port")
  threatType        String   @map("threat_type")
  severity          Int      @default(0)
  hitCount          Int      @default(1) @map("hit_count")
  isBanned          Boolean  @default(false) @map("is_banned")
  firstSeen         DateTime @default(now()) @map("first_seen")
  lastSeen          DateTime @default(now()) @map("last_seen")
  reportedByGateway String?  @map("reported_by_gateway")

  @@unique([sourceIp, threatType])
  @@map("threat_intel")
}

model Deposit {
  id        String        @id @default(uuid())
  userId    String        @map("user_id")
  user      User          @relation(fields: [userId], references: [id])
  amount    Decimal       @db.Decimal(20, 8)
  currency  String        @default("USD")
  txHash    String?       @map("tx_hash")
  status    DepositStatus @default(PENDING)
  createdAt DateTime      @default(now()) @map("created_at")

  @@map("deposits")
}

model QuotaPurchase {
  id        String   @id @default(uuid())
  userId    String   @map("user_id")
  user      User     @relation(fields: [userId], references: [id])
  quotaGb   Decimal  @map("quota_gb") @db.Decimal(20, 8)
  price     Decimal  @db.Decimal(20, 8)
  cellLevel String   @map("cell_level")
  createdAt DateTime @default(now()) @map("created_at")

  @@map("quota_purchases")
}

model InviteCode {
  id        String    @id @default(uuid())
  code      String    @unique
  createdBy String    @map("created_by")
  creator   User      @relation("CreatedBy", fields: [createdBy], references: [id])
  usedBy    String?   @unique @map("used_by")
  usedUser  User?     @relation("UsedBy", fields: [usedBy], references: [id])
  isUsed    Boolean   @default(false) @map("is_used")
  createdAt DateTime  @default(now()) @map("created_at")
  usedAt    DateTime? @map("used_at")

  @@map("invite_codes")
}
```

#### auth.service.ts

```typescript
@Injectable()
export class AuthService {
  constructor(
    private prisma: PrismaService,
    private jwtService: JwtService,
  ) {}

  // 邀请制注册
  // 1. 验证 invite_code 存在且未使用
  // 2. bcrypt hash 密码
  // 3. 生成 TOTP secret
  // 4. 创建用户 + 标记邀请码已使用（事务）
  // 5. 返回 { user, totpUri }
  async register(dto: RegisterDto): Promise<{ user: User; totpUri: string }>

  // 三因素登录
  // 1. 验证用户名 + 密码（bcrypt compare）
  // 2. 验证 TOTP 验证码（speakeasy）
  // 3. 签发 JWT（24h 有效期，含 user_id + cell_id）
  async login(dto: LoginDto): Promise<{ accessToken: string }>

  // JWT 验证
  async validateToken(payload: JwtPayload): Promise<User>
}

// RegisterDto
interface RegisterDto {
  username: string;
  password: string;
  inviteCode: string;
}

// LoginDto
interface LoginDto {
  username: string;
  password: string;
  totpCode: string;
}
```

#### billing.service.ts

```typescript
@Injectable()
export class BillingService {
  constructor(private prisma: PrismaService) {}

  // 查询流量流水（分页 + 时间范围过滤）
  async getLogs(userId: string, query: LogQueryDto): Promise<PaginatedResult<BillingLog>>

  // 查询配额余额
  async getQuota(userId: string): Promise<QuotaInfo>
  // 返回 { remainingQuota, totalDeposit, totalConsumed }

  // 充值（事务：创建 quota_purchases + 增加 remaining_quota）
  async recharge(userId: string, dto: RechargeDto): Promise<QuotaPurchase>
  // 1. 验证 quotaGb > 0, price > 0
  // 2. BEGIN TX: INSERT quota_purchases; UPDATE users remaining_quota += price; total_deposit += price; COMMIT

  // 查询充值记录
  async getPurchases(userId: string): Promise<QuotaPurchase[]>
}
```

#### cells.service.ts

```typescript
@Injectable()
export class CellsService {
  constructor(private prisma: PrismaService) {}

  // 创建蜂窝
  // 根据 level 自动设置 cost_multiplier: STANDARD=1.0, PLATINUM=1.5, DIAMOND=2.0
  async create(dto: CreateCellDto): Promise<Cell>

  // 蜂窝列表（含用户数和 Gateway 数统计）
  async findAll(): Promise<CellWithStats[]>

  // 分配用户到蜂窝
  // 检查蜂窝是否已满（当前用户数 >= max_users → 409）
  async assignUser(cellId: string, userId: string): Promise<void>
}
```

#### gateways.service.ts

```typescript
@Injectable()
export class GatewaysService {
  constructor(private prisma: PrismaService) {}

  // 节点列表（支持 cell_id 和 status 过滤）
  async findAll(query: GatewayQueryDto): Promise<Gateway[]>

  // 节点详情
  async findOne(id: string): Promise<Gateway>

  // 标记超时节点为 OFFLINE（last_heartbeat > 300s）
  // 由定时任务调用
  async markOfflineGateways(): Promise<number>
}
```

### 3. Docker Compose

```yaml
# docker-compose.yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: mirage_os
      POSTGRES_USER: mirage
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-mirage_dev}
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  gateway-bridge:
    build:
      context: ./gateway-bridge
      dockerfile: Dockerfile
    ports:
      - "50051:50051"
    depends_on:
      - postgres
      - redis
    environment:
      DATABASE_DSN: postgres://mirage:${POSTGRES_PASSWORD:-mirage_dev}@postgres:5432/mirage_os?sslmode=disable
      REDIS_ADDR: redis:6379
    volumes:
      - ./configs:/app/configs

  api-server:
    build:
      context: ./api-server
      dockerfile: Dockerfile
    ports:
      - "3000:3000"
    depends_on:
      - postgres
      - redis
    environment:
      DATABASE_URL: postgresql://mirage:${POSTGRES_PASSWORD:-mirage_dev}@postgres:5432/mirage_os
      REDIS_URL: redis://redis:6379
      JWT_SECRET: ${JWT_SECRET:-dev_jwt_secret_change_in_production}
      PORT: 3000

volumes:
  pgdata:
```

### 4. configs/mirage-os.yaml

```yaml
grpc:
  port: 50051
  tls_enabled: false
  cert_file: ""
  key_file: ""
  ca_file: ""

database:
  dsn: "postgres://mirage:mirage_dev@localhost:5432/mirage_os?sslmode=disable"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0

quota:
  business_price_per_gb: 0.10
  defense_price_per_gb: 0.05

intel:
  ban_threshold: 100
  cleanup_days: 30
  cleanup_min_hits: 10
```

## 数据模型

### 蜂窝级别 → 单价倍率映射

| 级别 | cost_multiplier | 业务单价 ($/GB) | 防御单价 ($/GB) |
|------|----------------|----------------|----------------|
| STANDARD | 1.0 | 0.10 | 0.05 |
| PLATINUM | 1.5 | 0.15 | 0.075 |
| DIAMOND | 2.0 | 0.20 | 0.10 |

### 费用计算公式

```
businessCost = (businessBytes / 1,000,000,000) × businessPricePerGB × costMultiplier
defenseCost  = (defenseBytes / 1,000,000,000) × defensePricePerGB × costMultiplier
totalCost    = businessCost + defenseCost
```

### Redis Key 设计

| Key | 类型 | TTL | 用途 |
|-----|------|-----|------|
| `gateway:{id}:status` | STRING | 60s | Gateway 在线状态 |
| `gateway:{id}:cell` | STRING | 60s | Gateway 所属蜂窝 |
| `mirage:blacklist:global` | SET | 无 | 全局黑名单 IP 集合 |
| `mirage:blacklist` | PUBSUB | - | 黑名单更新事件频道 |

## 正确性属性

### Property 1: 流量费用计算精度

*For any* 有效的 businessBytes（uint64）、defenseBytes（uint64）和 costMultiplier（1.0/1.5/2.0），CalculateCost 返回的 totalCost SHALL 等于 businessCost + defenseCost，且 businessCost = (businessBytes / 1e9) × businessPricePerGB × costMultiplier，defenseCost = (defenseBytes / 1e9) × defensePricePerGB × costMultiplier。

**Validates: Requirements 2.1, 2.4**

### Property 2: 结算精度一致性（往返属性）

*For any* 有效的初始 remainingQuota（> 0）和流量上报，结算前 remainingQuota 减去结算后 remainingQuota SHALL 等于 CalculateCost 计算的 totalCost。

**Validates: Requirements 2.6**

### Property 3: 配额归零触发阻断

*For any* 用户初始 remainingQuota 和一系列流量上报，当累计 totalCost >= 初始 remainingQuota 时，Settle 返回的 remainingQuota SHALL 小于等于 0。

**Validates: Requirements 2.3**

### Property 4: 蜂窝级别倍率单调性

*For any* 相同的流量数据，DIAMOND 级别的费用 SHALL 大于 PLATINUM 级别的费用，PLATINUM 级别的费用 SHALL 大于 STANDARD 级别的费用。

**Validates: Requirements 2.4**

### Property 5: 黑名单封禁阈值

*For any* 源 IP，当 hit_count < banThreshold 时 is_banned SHALL 为 false，当 hit_count >= banThreshold 时 is_banned SHALL 为 true。

**Validates: Requirements 3.1**

### Property 6: 邀请码一次性使用

*For any* 邀请码，注册成功后该邀请码的 is_used SHALL 为 true，再次使用同一邀请码注册 SHALL 返回错误。

**Validates: Requirements 6.1, 6.2**

### Property 7: 登录认证统一拒绝

*For any* 登录请求，当用户名、密码或 TOTP 验证码任一错误时，返回的错误信息 SHALL 不包含具体失败原因（统一返回 401）。

**Validates: Requirements 6.5**

### Property 8: 充值精度一致性（往返属性）

*For any* 有效的充值金额 price（> 0），充值前 remainingQuota + price SHALL 等于充值后 remainingQuota。

**Validates: Requirements 8.6**

### Property 9: 蜂窝容量限制

*For any* 蜂窝，当已分配用户数等于 max_users 时，再次分配用户 SHALL 返回 409 错误。

**Validates: Requirements 9.4**

### Property 10: 无效 gRPC 请求拒绝

*For any* gateway_id 为空或 timestamp 为 0 的 gRPC 请求，GRPC_Service SHALL 返回 InvalidArgument 错误码。

**Validates: Requirements 1.6**

## 错误处理

### 分级错误策略

| 模块 | 错误类型 | 处理方式 |
|------|---------|---------|
| GRPC_Service | 无效请求参数 | 返回 gRPC InvalidArgument |
| GRPC_Service | 数据库连接失败 | 返回 gRPC Internal，记录错误日志 |
| Quota_Enforcer | 事务失败 | 回滚事务，返回错误，不扣减配额 |
| Quota_Enforcer | 用户不存在 | 返回 gRPC NotFound |
| Intel_Distributor | Redis 发布失败 | 记录错误日志，不影响数据库写入 |
| Strategy_Dispatcher | Gateway 不可达 | 记录到 pendingPush，下次心跳重试 |
| Auth_Module | 认证失败 | 统一返回 401，不泄露具体原因 |
| Auth_Module | 邀请码无效 | 返回 400 |
| Billing_Module | 充值金额无效 | 返回 400 |
| Cells_Module | 蜂窝已满 | 返回 409 |
| gateway-bridge 启动 | 配置缺失 | 输出错误信息，终止进程 |
| gateway-bridge 启动 | PostgreSQL 连接失败 | 重试 3 次后终止进程 |
| gateway-bridge 启动 | Redis 连接失败 | 记录告警，降级运行（无缓存/Pub/Sub） |

## 测试策略

### 属性测试（Property-Based Testing）

**Go gateway-bridge**：使用 `pgregory.net/rapid` 进行属性测试。

- Property 1-4: pkg/quota/enforcer_test.go — 费用计算精度、结算一致性、配额归零、倍率单调性
- Property 5: pkg/intel/distributor_test.go — 封禁阈值
- Property 10: pkg/grpc/server_test.go — 无效请求拒绝

每个属性测试最少运行 100 次迭代。注释标注对应属性：
```go
// Feature: mirage-os-brain, Property 1: 流量费用计算精度
func TestProperty_CostCalculation(t *testing.T) { ... }
```

**NestJS api-server**：使用 `fast-check` 进行属性测试。

- Property 6-7: auth.service.spec.ts — 邀请码一次性使用、登录统一拒绝
- Property 8: billing.service.spec.ts — 充值精度一致性
- Property 9: cells.service.spec.ts — 蜂窝容量限制

```typescript
// Feature: mirage-os-brain, Property 6: 邀请码一次性使用
it('should reject reused invite codes', () => { ... })
```

### 单元测试

- Go: 配置加载（有效/无效 YAML）、空流量跳过结算、Redis key TTL
- NestJS: JWT 签发/验证、TOTP 生成/验证、分页查询、Gateway 超时标记

### 集成测试

- 完整生死裁决链路：ReportTraffic → 结算 → 配额归零 → HeartbeatResponse(remaining_quota=0)
- 黑名单分发链路：ReportThreat × 100 → 封禁 → Redis Pub/Sub → PushBlacklist
- 认证链路：注册（邀请码）→ TOTP 设置 → 登录 → JWT 访问 API
