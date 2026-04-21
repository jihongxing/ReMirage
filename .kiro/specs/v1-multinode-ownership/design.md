# 设计文档：多节点架构 P0 — 归属与计费隔离

## 概述

本设计覆盖 Proto/DB/Gateway/OS 四层改造，将 Mirage 从"Gateway 绑定 User"升级为"Gateway 承载多 Session"。改动横跨 proto 定义、Prisma schema、Go Gateway、Go gateway-bridge、NestJS API Server。

## 设计原则

1. **向后兼容**：新增字段均为 optional，旧版 Gateway 上报不带 user_id 时 OS 回退到 gateway_id 反推
2. **最小侵入**：复用现有 QuotaManager/QuotaBridge，只扩展 key 维度
3. **幂等优先**：所有上报携带 sequence_number，重放安全
4. **C 做数据面，Go 做控制面**：配额检查在 Go 层按 user_id 查桶，不改 eBPF 数据面

---

## 模块 1：Proto 扩展（需求 1）

### 改动范围

- `mirage-proto/mirage.proto`：扩展 TrafficRequest、QuotaPush、新增 SessionEvent

### 设计细节

#### TrafficRequest 扩展

```protobuf
message TrafficRequest {
    string gateway_id = 1;
    int64 timestamp = 2;
    uint64 business_bytes = 3;
    uint64 defense_bytes = 4;
    int32 period_seconds = 5;
    // 新增：精确归属
    string user_id = 6;
    string session_id = 7;
    uint64 sequence_number = 8;  // 幂等去重
}
```

#### QuotaPush 扩展

```protobuf
message QuotaPush {
    uint64 remaining_bytes = 1;
    // 新增：按用户下发
    string user_id = 2;
}
```

#### HeartbeatRequest 扩展

```protobuf
message HeartbeatRequest {
    string gateway_id = 1;
    int64 timestamp = 2;
    GatewayStatus status = 3;
    bool ebpf_loaded = 4;
    int32 threat_level = 5;
    int64 active_connections = 6;
    int32 memory_usage_mb = 7;
    // 新增：用户配额摘要
    repeated UserQuotaSummary user_quotas = 8;
}

message UserQuotaSummary {
    string user_id = 1;
    uint64 remaining_bytes = 2;
    int32 active_sessions = 3;
}
```

#### 新增：会话事件上报

```protobuf
service GatewayUplink {
    // ... 现有 RPC
    rpc ReportSessionEvent(SessionEventRequest) returns (SessionEventResponse);
}

message SessionEventRequest {
    string gateway_id = 1;
    string session_id = 2;
    string user_id = 3;
    string client_id = 4;
    SessionEventType event_type = 5;
    int64 timestamp = 6;
}

enum SessionEventType {
    SESSION_CONNECTED = 0;
    SESSION_DISCONNECTED = 1;
    SESSION_MIGRATING = 2;
}

message SessionEventResponse {
    bool ack = 1;
}
```

---

## 模块 2：DB Schema 扩展（需求 2、3）

### 改动范围

- `mirage-os/api-server/src/prisma/schema.prisma`：新增表 + 修改 billing_logs

### 设计细节

#### 新增 gateway_sessions 表

```prisma
model GatewaySession {
  id             String   @id @default(uuid())
  sessionId      String   @unique @map("session_id")
  gatewayId      String   @map("gateway_id")
  gateway        Gateway  @relation(fields: [gatewayId], references: [id])
  userId         String   @map("user_id")
  user           User     @relation(fields: [userId], references: [id])
  clientId       String?  @map("client_id")
  status         String   @default("active") // active / disconnected / migrating
  connectedAt    DateTime @default(now()) @map("connected_at")
  disconnectedAt DateTime? @map("disconnected_at")
  createdAt      DateTime @default(now()) @map("created_at")
  updatedAt      DateTime @updatedAt @map("updated_at")

  @@map("gateway_sessions")
  @@index([gatewayId, status])
  @@index([userId, status])
  @@index([clientId])
}
```

#### 新增 client_sessions 表

```prisma
model ClientSession {
  id               String   @id @default(uuid())
  sessionId        String   @unique @map("session_id")
  clientId         String   @map("client_id")
  userId           String   @map("user_id")
  user             User     @relation(fields: [userId], references: [id])
  currentGatewayId String?  @map("current_gateway_id")
  status           String   @default("active") // active / disconnected / migrated
  createdAt        DateTime @default(now()) @map("created_at")
  updatedAt        DateTime @updatedAt @map("updated_at")

  @@map("client_sessions")
  @@index([clientId])
  @@index([userId, status])
  @@index([currentGatewayId])
}
```

#### billing_logs 扩展

```prisma
model BillingLog {
  // ... 现有字段
  sessionId      String?  @map("session_id")
  sequenceNumber BigInt?  @map("sequence_number")

  @@unique([gatewayId, sequenceNumber]) // 幂等去重
}
```

#### Gateway 表增加关系

```prisma
model Gateway {
  // ... 现有字段
  sessions GatewaySession[]
}

model User {
  // ... 现有字段
  gatewaySessions GatewaySession[]
  clientSessions  ClientSession[]
}
```

---

## 模块 3：Gateway 配额隔离桶（需求 4）

### 改动范围

- `mirage-gateway/pkg/api/handlers.go`：PushQuota 按 user_id 分桶
- `mirage-gateway/pkg/api/quota_bucket.go`（新建）：用户隔离配额桶

### 设计细节

#### 用户隔离配额桶

```go
// pkg/api/quota_bucket.go
package api

import (
    "sync"
    "sync/atomic"
    "time"
)

type UserQuota struct {
    UserID         string
    RemainingBytes uint64
    TotalBytes     uint64
    Exhausted      uint32 // atomic: 0=正常, 1=耗尽
    UpdatedAt      time.Time
}

type QuotaBucketManager struct {
    buckets map[string]*UserQuota // user_id → quota
    mu      sync.RWMutex
    onExhausted func(userID string)
}

func NewQuotaBucketManager() *QuotaBucketManager {
    return &QuotaBucketManager{
        buckets: make(map[string]*UserQuota),
    }
}

// UpdateQuota OS 下发配额更新
func (m *QuotaBucketManager) UpdateQuota(userID string, remainingBytes uint64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    bucket, ok := m.buckets[userID]
    if !ok {
        bucket = &UserQuota{UserID: userID}
        m.buckets[userID] = bucket
    }
    atomic.StoreUint64(&bucket.RemainingBytes, remainingBytes)
    atomic.StoreUint32(&bucket.Exhausted, 0)
    bucket.UpdatedAt = time.Now()
}

// Consume 消费配额（原子操作），返回是否允许
func (m *QuotaBucketManager) Consume(userID string, bytes uint64) bool {
    m.mu.RLock()
    bucket, ok := m.buckets[userID]
    m.mu.RUnlock()
    if !ok {
        return false // 未知用户拒绝
    }
    if atomic.LoadUint32(&bucket.Exhausted) == 1 {
        return false
    }
    for {
        remaining := atomic.LoadUint64(&bucket.RemainingBytes)
        if remaining < bytes {
            atomic.StoreUint32(&bucket.Exhausted, 1)
            if m.onExhausted != nil {
                go m.onExhausted(userID)
            }
            return false
        }
        if atomic.CompareAndSwapUint64(&bucket.RemainingBytes, remaining, remaining-bytes) {
            return true
        }
    }
}

// GetSummaries 获取所有用户配额摘要（心跳上报用）
func (m *QuotaBucketManager) GetSummaries() []*UserQuotaSummary {
    m.mu.RLock()
    defer m.mu.RUnlock()
    var summaries []*UserQuotaSummary
    for _, b := range m.buckets {
        summaries = append(summaries, &UserQuotaSummary{
            UserID:         b.UserID,
            RemainingBytes: atomic.LoadUint64(&b.RemainingBytes),
        })
    }
    return summaries
}
```

#### PushQuota Handler 改造

```go
func (h *CommandHandler) PushQuota(ctx context.Context, req *pb.QuotaPush) (*pb.PushResponse, error) {
    // ... 签名校验、审计日志（Spec 1-2 已实现）

    if req.UserId != "" {
        // 新模式：按用户下发
        h.quotaBuckets.UpdateQuota(req.UserId, req.RemainingBytes)
    } else {
        // 兼容旧模式：全局配额（向后兼容）
        h.quotaBuckets.UpdateQuota("__global__", req.RemainingBytes)
    }
    return &pb.PushResponse{Success: true}, nil
}
```

---

## 模块 4：Gateway 流量按用户统计与上报（需求 5）

### 改动范围

- `mirage-gateway/pkg/api/traffic_counter.go`（新建）：按用户流量计数器
- `mirage-gateway/pkg/api/grpc_client.go`：改造 ReportTraffic 为按用户上报

### 设计细节

#### 按用户流量计数器

```go
// pkg/api/traffic_counter.go
type UserTrafficCounter struct {
    counters map[string]*TrafficStats // user_id → stats
    mu       sync.Mutex
    seqNum   uint64 // 全局单调递增序列号
}

type TrafficStats struct {
    UserID        string
    SessionID     string
    BusinessBytes uint64
    DefenseBytes  uint64
}

func (tc *UserTrafficCounter) Add(userID, sessionID string, bizBytes, defBytes uint64) {
    tc.mu.Lock()
    defer tc.mu.Unlock()
    key := userID
    s, ok := tc.counters[key]
    if !ok {
        s = &TrafficStats{UserID: userID, SessionID: sessionID}
        tc.counters[key] = s
    }
    atomic.AddUint64(&s.BusinessBytes, bizBytes)
    atomic.AddUint64(&s.DefenseBytes, defBytes)
}

// Flush 返回所有用户的流量快照并重置计数器
func (tc *UserTrafficCounter) Flush() []*TrafficStats {
    tc.mu.Lock()
    defer tc.mu.Unlock()
    var result []*TrafficStats
    for _, s := range tc.counters {
        result = append(result, &TrafficStats{
            UserID:        s.UserID,
            SessionID:     s.SessionID,
            BusinessBytes: atomic.SwapUint64(&s.BusinessBytes, 0),
            DefenseBytes:  atomic.SwapUint64(&s.DefenseBytes, 0),
        })
    }
    return result
}

func (tc *UserTrafficCounter) NextSeqNum() uint64 {
    return atomic.AddUint64(&tc.seqNum, 1)
}
```

#### 上报改造

```go
func (c *GRPCClient) ReportTrafficByUser(stats []*TrafficStats, gatewayID string) {
    for _, s := range stats {
        if s.BusinessBytes == 0 && s.DefenseBytes == 0 {
            continue
        }
        req := &pb.TrafficRequest{
            GatewayId:      gatewayID,
            Timestamp:      time.Now().Unix(),
            BusinessBytes:  s.BusinessBytes,
            DefenseBytes:   s.DefenseBytes,
            PeriodSeconds:  int32(reportInterval.Seconds()),
            UserId:         s.UserID,
            SessionId:      s.SessionID,
            SequenceNumber: c.trafficCounter.NextSeqNum(),
        }
        c.uplink.ReportTraffic(ctx, req)
    }
}
```

---

## 模块 5：Gateway 会话管理与事件上报（需求 4、5、6）

### 改动范围

- `mirage-gateway/pkg/api/session_manager.go`（新建）：连接 → 用户映射
- `mirage-gateway/pkg/api/grpc_client.go`：增加 ReportSessionEvent

### 设计细节

#### 会话管理器

```go
// pkg/api/session_manager.go
type SessionManager struct {
    sessions map[string]*SessionInfo // session_id → info
    byUser   map[string][]string    // user_id → []session_id
    mu       sync.RWMutex
}

type SessionInfo struct {
    SessionID   string
    UserID      string
    ClientID    string
    ConnectedAt time.Time
}

func (sm *SessionManager) Register(sessionID, userID, clientID string) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    sm.sessions[sessionID] = &SessionInfo{
        SessionID: sessionID, UserID: userID,
        ClientID: clientID, ConnectedAt: time.Now(),
    }
    sm.byUser[userID] = append(sm.byUser[userID], sessionID)
}

func (sm *SessionManager) Unregister(sessionID string) *SessionInfo {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    info, ok := sm.sessions[sessionID]
    if !ok { return nil }
    delete(sm.sessions, sessionID)
    // 从 byUser 中移除
    sids := sm.byUser[info.UserID]
    for i, s := range sids {
        if s == sessionID {
            sm.byUser[info.UserID] = append(sids[:i], sids[i+1:]...)
            break
        }
    }
    return info
}

func (sm *SessionManager) GetUserID(sessionID string) string {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    if info, ok := sm.sessions[sessionID]; ok {
        return info.UserID
    }
    return ""
}

func (sm *SessionManager) ActiveSessionCount() int {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    return len(sm.sessions)
}
```

---

## 模块 6：OS 归属映射服务（需求 6）

### 改动范围

- `mirage-os/api-server/src/modules/sessions/session.module.ts`（新建）
- `mirage-os/api-server/src/modules/sessions/session.service.ts`（新建）
- `mirage-os/api-server/src/modules/sessions/session.controller.ts`（新建）
- `mirage-os/gateway-bridge/pkg/grpc/handlers.go`：增加 ReportSessionEvent 处理

### 设计细节

#### SessionService

```typescript
// session.service.ts
@Injectable()
export class SessionService {
  constructor(private prisma: PrismaService) {}

  // Gateway 上报会话建立
  async onSessionConnected(data: {
    sessionId: string; gatewayId: string;
    userId: string; clientId: string;
  }) {
    await this.prisma.gatewaySession.upsert({
      where: { sessionId: data.sessionId },
      create: {
        sessionId: data.sessionId,
        gatewayId: data.gatewayId,
        userId: data.userId,
        clientId: data.clientId,
        status: 'active',
      },
      update: {
        gatewayId: data.gatewayId,
        status: 'active',
        disconnectedAt: null,
      },
    });
    await this.prisma.clientSession.upsert({
      where: { sessionId: data.sessionId },
      create: {
        sessionId: data.sessionId,
        clientId: data.clientId,
        userId: data.userId,
        currentGatewayId: data.gatewayId,
        status: 'active',
      },
      update: {
        currentGatewayId: data.gatewayId,
        status: 'active',
      },
    });
  }

  // Gateway 上报会话断开
  async onSessionDisconnected(sessionId: string) {
    await this.prisma.gatewaySession.update({
      where: { sessionId },
      data: { status: 'disconnected', disconnectedAt: new Date() },
    });
    await this.prisma.clientSession.update({
      where: { sessionId },
      data: { status: 'disconnected' },
    });
  }

  // Gateway 心跳超时 → 批量标记断开
  async onGatewayTimeout(gatewayId: string) {
    await this.prisma.gatewaySession.updateMany({
      where: { gatewayId, status: 'active' },
      data: { status: 'disconnected', disconnectedAt: new Date() },
    });
  }

  // 查询：按 gateway_id 查当前会话
  async getSessionsByGateway(gatewayId: string) {
    return this.prisma.gatewaySession.findMany({
      where: { gatewayId, status: 'active' },
    });
  }

  // 查询：按 user_id 查当前会话
  async getSessionsByUser(userId: string) {
    return this.prisma.clientSession.findMany({
      where: { userId, status: 'active' },
    });
  }
}
```

#### OS 计费改造

ReportTraffic handler 改为直接使用 `user_id`：

```go
// gateway-bridge ReportTraffic handler
func (h *Handler) ReportTraffic(ctx context.Context, req *pb.TrafficRequest) (*pb.TrafficResponse, error) {
    userID := req.UserId
    if userID == "" {
        // 向后兼容：通过 gateway_id 查找
        userID = h.lookupUserByGateway(req.GatewayId)
    }

    // 幂等去重
    if h.isDuplicateTraffic(req.GatewayId, req.SequenceNumber) {
        return &pb.TrafficResponse{Ack: true}, nil
    }

    // 写入计费日志
    h.billingService.RecordTraffic(userID, req.GatewayId, req.SessionId,
        req.BusinessBytes, req.DefenseBytes, req.SequenceNumber)

    return &pb.TrafficResponse{Ack: true}, nil
}
```

---

## 模块 7：OS 配额下发改造

### 改动范围

- `mirage-os/gateway-bridge/pkg/dispatch/quota_dispatch.go`（新建）

### 设计细节

OS 下发配额时按用户维度：

```go
// 为某个 Gateway 上的所有活跃用户下发配额
func (d *QuotaDispatcher) PushQuotaToGateway(gatewayID string) error {
    sessions, _ := d.sessionService.GetActiveSessionsByGateway(gatewayID)
    userIDs := uniqueUserIDs(sessions)

    for _, uid := range userIDs {
        quota := d.quotaManager.GetQuota(uid)
        if quota == nil { continue }
        d.downlink.PushQuota(gatewayID, &pb.QuotaPush{
            UserId:         uid,
            RemainingBytes: quota.RemainingBytes,
        })
    }
    return nil
}
```

---

## 配置变更

无新增配置项。Proto 字段扩展向后兼容，旧版 Gateway 不发送新字段时 OS 自动回退。

## 不在本次范围内

- OS 控制面补齐（Gateway 注册、拓扑索引、按 Cell 下推）→ Spec 2-3
- Client 运行时拓扑学习 → Spec 3-1
- Client 稳定身份标识 → Spec 3-1
- eBPF 数据面配额检查改造（本轮配额检查在 Go 层，不改 eBPF）
