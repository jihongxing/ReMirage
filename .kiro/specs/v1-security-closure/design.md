# 设计文档：OS-Gateway 安全闭环

## 概述

本设计覆盖 OS 侧 5 项 + Gateway 侧 5 项两周内修整改。改动横跨 NestJS API Server、Go Gateway Bridge、Go Gateway 三个服务。

## 设计原则

1. **在 Spec 1-1/1-2 基础上增量改造**：复用已有 RolesGuard、BlacklistManager、CommandAuth 等模块
2. **C 做数据面，Go 做控制面**：安全状态机、联动逻辑、指标采集全部在 Go 层；eBPF 只负责黑名单命中丢包
3. **Go → C 通过 eBPF Map**：处置策略最终通过黑名单 LPM Map 生效，不新增数据面程序
4. **OS → Gateway 通过现有 gRPC 下行通道**：威胁情报、策略更新、状态强制切换复用 PushBlacklist/PushStrategy

---

## 模块 1：统一 RBAC 与资源归属（需求 1）

### 改动范围

- `mirage-os/api-server/src/modules/auth/roles.guard.ts`：升级为矩阵查询
- `mirage-os/api-server/src/modules/auth/rbac-matrix.ts`（新建）：角色权限矩阵
- `mirage-os/api-server/src/modules/auth/owner.guard.ts`（新建）：资源归属守卫
- 各 Controller：应用 @Roles + OwnerGuard

### 设计细节

#### 角色权限矩阵

```typescript
// rbac-matrix.ts
export enum Role {
  USER = 'user',
  ADMIN = 'admin',
  OPERATOR = 'operator',
  AUDITOR = 'auditor',
}

export enum Permission {
  USER_READ = 'user:read',
  USER_WRITE = 'user:write',
  GATEWAY_READ = 'gateway:read',
  GATEWAY_WRITE = 'gateway:write',
  CELL_READ = 'cell:read',
  CELL_WRITE = 'cell:write',
  THREAT_READ = 'threat:read',
  THREAT_WRITE = 'threat:write',
  BILLING_READ = 'billing:read',
  BILLING_WRITE = 'billing:write',
  AUDIT_READ = 'audit:read',
  SYSTEM_ADMIN = 'system:admin',
}

export const RBAC_MATRIX: Record<Role, Permission[]> = {
  [Role.ADMIN]: Object.values(Permission), // 全部权限
  [Role.OPERATOR]: [
    Permission.GATEWAY_READ, Permission.GATEWAY_WRITE,
    Permission.CELL_READ, Permission.CELL_WRITE,
    Permission.THREAT_READ, Permission.THREAT_WRITE,
  ],
  [Role.AUDITOR]: [
    Permission.USER_READ, Permission.GATEWAY_READ,
    Permission.CELL_READ, Permission.THREAT_READ,
    Permission.BILLING_READ, Permission.AUDIT_READ,
  ],
  [Role.USER]: [Permission.BILLING_READ],
};
```

#### RolesGuard 升级

```typescript
// roles.guard.ts — 改为查询 RBAC_MATRIX
canActivate(context: ExecutionContext): boolean {
  const requiredPermissions = this.reflector.getAllAndOverride<Permission[]>('permissions', [
    context.getHandler(), context.getClass(),
  ]);
  if (!requiredPermissions) return true;
  const { user } = context.switchToHttp().getRequest();
  const userPerms = RBAC_MATRIX[user.role] || [];
  return requiredPermissions.every(p => userPerms.includes(p));
}
```

#### OwnerGuard（资源归属）

```typescript
// owner.guard.ts
@Injectable()
export class OwnerGuard implements CanActivate {
  canActivate(context: ExecutionContext): boolean {
    const request = context.switchToHttp().getRequest();
    const user = request.user;
    // admin/operator/auditor 跳过 owner check
    if (['admin', 'operator', 'auditor'].includes(user.role)) return true;
    // user 角色：检查资源归属
    const resourceUserId = request.params.userId || request.body?.userId;
    if (resourceUserId && resourceUserId !== user.userId) return false;
    return true;
  }
}
```

---

## 模块 2：内部服务统一鉴权（需求 2）

### 改动范围

- `mirage-os/gateway-bridge/pkg/grpc/interceptor.go`（新建）：gRPC CN 白名单拦截器
- `mirage-os/cmd/ws-gateway/main.go`：增加 JWT 校验
- `mirage-os/gateway-bridge/pkg/rest/middleware.go`：增加访问日志

### 设计细节

#### gRPC CN 白名单

```go
// pkg/grpc/interceptor.go
func CNWhitelistInterceptor(allowedCNs []string) grpc.UnaryServerInterceptor {
    cnSet := make(map[string]bool)
    for _, cn := range allowedCNs {
        cnSet[cn] = true
    }
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler) (interface{}, error) {
        p, ok := peer.FromContext(ctx)
        if !ok {
            return nil, status.Errorf(codes.Unauthenticated, "no peer info")
        }
        tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
        if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
            return nil, status.Errorf(codes.Unauthenticated, "no client cert")
        }
        cn := tlsInfo.State.PeerCertificates[0].Subject.CommonName
        if !cnSet[cn] {
            return nil, status.Errorf(codes.PermissionDenied, "CN not allowed: %s", cn)
        }
        return handler(ctx, req)
    }
}
```

#### WebSocket JWT 校验

ws-gateway 连接建立时从 query param 或 header 提取 JWT 并校验：

```go
func wsAuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := r.URL.Query().Get("token")
            if token == "" {
                token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            }
            if _, err := jwt.Parse(token, keyFunc(jwtSecret)); err != nil {
                http.Error(w, "unauthorized", 401)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## 模块 3：控制面审计日志（需求 3）

### 改动范围

- `mirage-os/api-server/src/modules/audit/audit.module.ts`（新建）
- `mirage-os/api-server/src/modules/audit/audit.service.ts`（新建）
- `mirage-os/api-server/src/modules/audit/audit.controller.ts`（新建）
- `mirage-os/api-server/src/modules/audit/audit-interceptor.ts`（新建）
- `mirage-os/api-server/src/prisma/schema.prisma`：增加 audit_logs 表
- `mirage-os/gateway-bridge/pkg/audit/audit.go`（新建）

### 设计细节

#### Prisma Schema

```prisma
model AuditLog {
  id          String   @id @default(uuid())
  operatorId  String   @map("operator_id")
  operatorRole String  @map("operator_role")
  sourceIp    String   @map("source_ip")
  targetResource String @map("target_resource")
  actionType  String   @map("action_type")
  actionParams Json?   @map("action_params")
  result      String   // success / failure / denied
  createdAt   DateTime @default(now()) @map("created_at")
  @@map("audit_logs")
  @@index([createdAt])
  @@index([operatorId])
  @@index([actionType])
}
```

#### NestJS 审计拦截器

```typescript
// audit-interceptor.ts
@Injectable()
export class AuditInterceptor implements NestInterceptor {
  constructor(private auditService: AuditService) {}

  intercept(context: ExecutionContext, next: CallHandler): Observable<any> {
    const request = context.switchToHttp().getRequest();
    const startTime = Date.now();
    return next.handle().pipe(
      tap((response) => {
        if (this.isSensitiveAction(request.method, request.url)) {
          this.auditService.log({
            operatorId: request.user?.userId,
            operatorRole: request.user?.role,
            sourceIp: request.ip,
            targetResource: request.url,
            actionType: `${request.method} ${request.route?.path}`,
            actionParams: request.body,
            result: 'success',
          });
        }
      }),
    );
  }
}
```

#### Gateway Bridge 审计

```go
// pkg/audit/audit.go
type AuditLogger struct {
    mu      sync.Mutex
    entries []AuditEntry
    file    *os.File
}

type AuditEntry struct {
    Timestamp   time.Time `json:"ts"`
    CommandType string    `json:"cmd"`
    SourceAddr  string    `json:"src"`
    TargetGW    string    `json:"target"`
    Params      string    `json:"params"`
    Result      string    `json:"result"`
}

func (al *AuditLogger) Log(entry AuditEntry) {
    entry.Timestamp = time.Now()
    al.mu.Lock()
    defer al.mu.Unlock()
    data, _ := json.Marshal(entry)
    al.file.Write(append(data, '\n'))
}
```

---

## 模块 4：威胁情报回路（需求 4）

### 改动范围

- `mirage-os/api-server/src/modules/threats/threat-intel.service.ts`（新建）
- `mirage-os/api-server/src/prisma/schema.prisma`：增加 threat_intel 表
- `mirage-os/gateway-bridge/pkg/dispatch/blacklist_sync.go`（新建）
- `mirage-os/gateway-bridge/pkg/grpc/handlers.go`：增加威胁上报处理

### 设计细节

#### Prisma Schema

```prisma
model ThreatIntel {
  id        String   @id @default(uuid())
  cidr      String
  riskLevel Int      @map("risk_level") // 1-5
  source    String   // manual / auto / gateway_report
  gatewayId String?  @map("gateway_id")
  reportCount Int    @default(1) @map("report_count")
  ttlSeconds Int     @map("ttl_seconds")
  expiresAt DateTime @map("expires_at")
  createdAt DateTime @default(now()) @map("created_at")
  updatedAt DateTime @updatedAt @map("updated_at")
  @@map("threat_intel")
  @@unique([cidr])
  @@index([expiresAt])
}
```

#### 威胁情报同步流程

```
Gateway 上报威胁事件 → gateway-bridge gRPC → ThreatIntelService
  ↓
ThreatIntelService 评估：
  - 同一来源被 >= 3 个 Gateway 上报 → 全局封禁
  - 手动添加 → 直接全局封禁
  ↓
gateway-bridge 通过 PushBlacklist 下发到所有在线 Gateway
  ↓
Gateway BlacklistManager.MergeGlobal → syncToEBPF → 数据面生效
```

#### 心跳黑名单摘要

Gateway 心跳增加字段：

```protobuf
message HeartbeatRequest {
  // ... 现有字段
  int32 blacklist_count = 10;
  int64 blacklist_updated_at = 11; // Unix timestamp
}
```

OS 收到心跳后比对摘要，不一致时触发全量同步。

---

## 模块 5：安全基线（需求 5）

### 改动范围

- `mirage-os/api-server/src/main.ts`：ValidationPipe + Helmet + CORS
- `mirage-os/api-server/src/filters/http-exception.filter.ts`（新建）
- `mirage-os/gateway-bridge/cmd/bridge/main.go`：禁用 reflection

### 设计细节

#### NestJS 安全基线

```typescript
// main.ts
import helmet from 'helmet';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);

  // 输入校验
  app.useGlobalPipes(new ValidationPipe({
    whitelist: true,        // 剥离未定义字段
    forbidNonWhitelisted: true,
    transform: true,
  }));

  // 安全 Header
  app.use(helmet({
    contentSecurityPolicy: false, // API 不需要 CSP
    hsts: process.env.NODE_ENV === 'production',
  }));

  // CORS
  app.enableCors({
    origin: process.env.NODE_ENV === 'production'
      ? (process.env.ALLOWED_ORIGINS || '').split(',')
      : true,
    credentials: true,
  });

  // 全局异常过滤器（脱敏）
  app.useGlobalFilters(new HttpExceptionFilter());
}
```

#### 错误响应脱敏

```typescript
// http-exception.filter.ts
@Catch()
export class HttpExceptionFilter implements ExceptionFilter {
  catch(exception: unknown, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const response = ctx.getResponse();
    const status = exception instanceof HttpException
      ? exception.getStatus() : 500;

    const body: any = { statusCode: status, message: 'Internal server error' };
    if (exception instanceof HttpException) {
      body.message = exception.message;
    }
    // 生产模式不暴露堆栈
    if (process.env.NODE_ENV !== 'production' && exception instanceof Error) {
      body.stack = exception.stack;
    }
    response.status(status).json(body);
  }
}
```

#### gRPC Reflection 禁用

```go
// gateway-bridge main.go
if os.Getenv("MIRAGE_ENV") != "production" {
    reflection.Register(grpcServer)
}
```

---

## 模块 6：标准入口处置策略（需求 6）

### 改动范围

- `mirage-gateway/pkg/threat/policy.go`（新建）：处置策略引擎
- `mirage-gateway/pkg/threat/policy_config.go`（新建）：策略配置结构
- `mirage-gateway/configs/gateway.yaml`：增加 ingress_policy 配置段

### 设计细节

#### 策略配置

```yaml
# gateway.yaml
ingress_policy:
  rules:
    - condition: blacklist_hit
      action: drop
      priority: 100
    - condition: threat_level_critical
      action: drop
      priority: 90
    - condition: threat_level_high
      action: throttle
      params: { pps: 10 }
      priority: 80
    - condition: honeypot_hit
      action: trap
      priority: 70
    - condition: suspicious_fingerprint
      action: observe
      priority: 60
    - condition: rate_exceeded
      action: throttle
      params: { pps: 50 }
      priority: 50
```

#### 策略引擎

```go
// pkg/threat/policy.go
type IngressPolicy struct {
    rules []PolicyRule
    mu    sync.RWMutex
}

type PolicyRule struct {
    Condition string            `yaml:"condition"`
    Action    IngressAction     `yaml:"action"`
    Params    map[string]int    `yaml:"params"`
    Priority  int               `yaml:"priority"`
}

func (p *IngressPolicy) Evaluate(ctx *IngressContext) IngressAction {
    p.mu.RLock()
    defer p.mu.RUnlock()
    bestAction := ActionPass
    bestPriority := -1
    for _, rule := range p.rules {
        if rule.matches(ctx) && rule.Priority > bestPriority {
            bestAction = rule.Action
            bestPriority = rule.Priority
        }
    }
    return bestAction
}

type IngressContext struct {
    SourceIP       string
    BlacklistHit   bool
    ThreatLevel    ThreatLevel
    HoneypotHit    bool
    FingerprintRisk int
    ConnectionRate float64
}
```

---

## 模块 7：蜜罐-指纹-黑名单联动（需求 7）

### 改动范围

- `mirage-gateway/pkg/cortex/risk_scorer.go`（新建）：IP 风险评分器
- `mirage-gateway/pkg/cortex/threat_bus.go`：增加蜜罐/指纹事件订阅
- `mirage-gateway/pkg/phantom/reporter.go`（新建）：蜜罐命中上报
- `mirage-gateway/pkg/cortex/analyzer.go`：增加指纹事件处理

### 设计细节

#### IP 风险评分器

```go
// pkg/cortex/risk_scorer.go
type RiskScorer struct {
    scores    map[string]*IPScore // IP → score
    blacklist *threat.BlacklistManager
    mu        sync.RWMutex
    decayTicker *time.Ticker
}

type IPScore struct {
    Score     int
    UpdatedAt time.Time
    Sources   []string // honeypot / fingerprint / threat
}

const (
    HoneypotHitScore    = 30
    DangerousFPScore    = 40
    ThreatEventScore    = 20
    AutoBanThreshold    = 70
    DecayPerHour        = 10
    AutoBanTTL          = 2 * time.Hour
)

func (rs *RiskScorer) AddScore(ip string, delta int, source string) {
    rs.mu.Lock()
    defer rs.mu.Unlock()
    s, ok := rs.scores[ip]
    if !ok {
        s = &IPScore{}
        rs.scores[ip] = s
    }
    s.Score += delta
    if s.Score > 100 { s.Score = 100 }
    s.UpdatedAt = time.Now()
    s.Sources = append(s.Sources, source)

    if s.Score >= AutoBanThreshold && rs.blacklist != nil {
        rs.blacklist.Add(ip+"/32", time.Now().Add(AutoBanTTL), threat.SourceLocal)
        log.Printf("[RiskScorer] 自动封禁: %s (score=%d)", ip, s.Score)
    }
}

func (rs *RiskScorer) startDecay(ctx context.Context) {
    rs.decayTicker = time.NewTicker(time.Hour)
    go func() {
        for {
            select {
            case <-ctx.Done(): return
            case <-rs.decayTicker.C:
                rs.mu.Lock()
                for ip, s := range rs.scores {
                    s.Score -= DecayPerHour
                    if s.Score <= 0 { delete(rs.scores, ip) }
                }
                rs.mu.Unlock()
            }
        }
    }()
}
```

#### 联动事件总线

Cortex ThreatBus 增加事件类型：

```go
const (
    EventThreat     = "threat"
    EventHoneypot   = "honeypot"
    EventFingerprint = "fingerprint"
)
```

Phantom 模块命中时发送事件到 ThreatBus，Cortex 订阅后调用 RiskScorer.AddScore。

---

## 模块 8：Gateway 安全状态机（需求 8）

### 改动范围

- `mirage-gateway/pkg/threat/security_fsm.go`（新建）
- `mirage-gateway/pkg/threat/responder.go`：集成状态机
- `mirage-gateway/pkg/api/handlers.go`：支持 OS 强制切换

### 设计细节

#### 状态机定义

```go
// pkg/threat/security_fsm.go
type SecurityState int

const (
    StateNormal       SecurityState = iota // 默认策略
    StateAlert                             // 收紧 Throttle
    StateHighPressure                      // 主动 Drop
    StateIsolated                          // 仅白名单
    StateSilent                            // 最小暴露
)

type SecurityFSM struct {
    current       SecurityState
    cooldownUntil time.Time
    policy        *IngressPolicy
    mu            sync.Mutex
    onStateChange func(SecurityState) // 回调：上报 OS + 更新指标
}

func (fsm *SecurityFSM) Evaluate(metrics *SecurityMetrics) {
    fsm.mu.Lock()
    defer fsm.mu.Unlock()

    target := fsm.computeTarget(metrics)

    // 升级立即执行
    if target > fsm.current {
        fsm.transition(target)
        return
    }
    // 降级需冷却 300s
    if target < fsm.current && time.Now().After(fsm.cooldownUntil) {
        fsm.transition(target)
    }
}

func (fsm *SecurityFSM) computeTarget(m *SecurityMetrics) SecurityState {
    switch {
    case m.ControlPlaneDown:
        return StateSilent
    case m.ThreatLevel >= LevelExtreme || m.RejectRate > 0.8:
        return StateIsolated
    case m.ThreatLevel >= LevelCritical || m.RejectRate > 0.5:
        return StateHighPressure
    case m.ThreatLevel >= LevelHigh || m.RejectRate > 0.2:
        return StateAlert
    default:
        return StateNormal
    }
}

func (fsm *SecurityFSM) transition(target SecurityState) {
    old := fsm.current
    fsm.current = target
    fsm.cooldownUntil = time.Now().Add(300 * time.Second)
    fsm.policy.ApplyStateOverride(target)
    if fsm.onStateChange != nil {
        go fsm.onStateChange(target)
    }
    log.Printf("[SecurityFSM] 状态迁移: %d → %d", old, target)
}

// ForceState OS 强制切换
func (fsm *SecurityFSM) ForceState(state SecurityState) {
    fsm.mu.Lock()
    defer fsm.mu.Unlock()
    fsm.transition(state)
}

type SecurityMetrics struct {
    ThreatLevel      ThreatLevel
    RejectRate       float64 // 最近 1 分钟入口拒绝率
    BlacklistHitRate float64
    ControlPlaneDown bool
}
```

---

## 模块 9：Prometheus 观测指标（需求 9）

### 改动范围

- `mirage-gateway/pkg/threat/metrics.go`（新建）
- `mirage-gateway/cmd/gateway/main.go`：启动 metrics HTTP server

### 设计细节

```go
// pkg/threat/metrics.go
import "github.com/prometheus/client_golang/prometheus"

var (
    IngressRejectTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_ingress_reject_total",
    }, []string{"gateway_id", "action"})

    HoneypotHitTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_honeypot_hit_total",
    }, []string{"gateway_id"})

    BlacklistHitTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_blacklist_hit_total",
    }, []string{"gateway_id"})

    ThreatEscalationTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_threat_escalation_total",
    }, []string{"gateway_id"})

    AuthFailureTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_auth_failure_total",
    }, []string{"gateway_id"})

    MTLSErrorTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "mirage_mtls_error_total",
    }, []string{"gateway_id"})

    SecurityStateGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
        Name: "mirage_security_state",
    }, []string{"gateway_id"})
)

func RegisterMetrics() {
    prometheus.MustRegister(
        IngressRejectTotal, HoneypotHitTotal, BlacklistHitTotal,
        ThreatEscalationTotal, AuthFailureTotal, MTLSErrorTotal,
        SecurityStateGauge,
    )
}
```

Metrics HTTP server 绑定 `127.0.0.1:9090`。

---

## 模块 10：安全回归测试（需求 10）

### 改动范围

- `mirage-gateway/pkg/threat/security_test.go`（新建）
- `mirage-os/tests/security_regression_test.go`（新建）
- `Makefile`：增加 `test-security` target

### 测试矩阵

| 测试项 | 模块 | 验证点 |
|--------|------|--------|
| mTLS 拒绝 | Gateway gRPC | 无证书连接返回错误 |
| 黑名单生效 | BlacklistManager | Add → SyncStats 一致 |
| 入口 Drop | IngressPolicy | 黑名单 IP → ActionDrop |
| RBAC 越权 | API Server | user 访问 admin 接口 → 403 |
| 审计完整性 | AuditService | 敏感操作后 audit_logs 有记录 |
| 状态机迁移 | SecurityFSM | 威胁升级 → 状态变化 |
| 联动链路 | RiskScorer | 蜜罐命中 → 评分 → 自动封禁 |

---

## 配置变更汇总

### gateway.yaml 新增

```yaml
ingress_policy:
  rules: [...]  # 见模块 6

security:
  grpc_allowed_cns: ["mirage-os", "mirage-bridge"]
```

### mirage-os.yaml 新增

```yaml
audit:
  enabled: true
  retention_days: 90

threat_intel:
  auto_global_ban_threshold: 3  # 同一来源被 N 个 Gateway 上报后全局封禁
  default_ttl_seconds: 3600

cors:
  allowed_origins: "${ALLOWED_ORIGINS}"
```

## 不在本次范围内

- 版本级安全改造（统一安全控制平面、跨节点威胁情报中心）
- Phantom 蜜罐收敛（由 Spec 2-4 覆盖）
- 零信任三层纵深防御（由 Spec 3-3 覆盖）
- Proto 文件修改（心跳字段扩展可在实现时按需添加）
