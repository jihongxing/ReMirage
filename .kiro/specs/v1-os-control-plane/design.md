# 设计文档：多节点架构 P0 — OS 控制面补齐

## 概述

本设计覆盖 OS 控制面四项改造：Gateway 注册与拓扑索引、按维度下推、心跳拓扑语义、一致性验收。改动集中在 gateway-bridge（Go）和 proto 层，NestJS API Server 仅增加查询 API。

## 设计原则

1. **在现有 DownlinkService + StrategyDispatcher 基础上增量改造**：不重建下推链路
2. **拓扑索引双写 DB + Redis**：DB 为持久化真相源，Redis 为高频查询缓存
3. **心跳驱动拓扑刷新**：注册建立初始拓扑，心跳持续刷新，超时自动清理
4. **下推状态持久化**：每次下推写入 DB 审计表，支持事后追踪

---

## 模块 1：Gateway 注册与拓扑索引（需求 1）

### 改动范围

- `mirage-proto/mirage.proto`：新增 RegisterGateway RPC 和消息
- `mirage-os/gateway-bridge/pkg/grpc/server.go`：实现 RegisterGateway handler
- `mirage-os/gateway-bridge/pkg/topology/registry.go`（新建）：拓扑索引管理器
- `mirage-os/api-server/src/modules/gateways/gateways.controller.ts`：增加拓扑查询 API

### 设计细节

#### Proto 新增

```protobuf
service GatewayUplink {
    // ... 现有 RPC
    rpc RegisterGateway(RegisterRequest) returns (RegisterResponse);
}

message RegisterRequest {
    string gateway_id = 1;
    string cell_id = 2;
    string downlink_addr = 3;
    string version = 4;
    GatewayCapabilities capabilities = 5;
}

message GatewayCapabilities {
    bool ebpf_supported = 1;
    int32 max_connections = 2;
    int32 max_sessions = 3;
}

message RegisterResponse {
    bool success = 1;
    string message = 2;
    string assigned_cell_id = 3;  // OS 可能重新分配 Cell
}
```

#### 拓扑索引管理器

```go
// pkg/topology/registry.go
package topology

import (
    "context"
    "fmt"
    "log"
    "sync"
    "time"

    goredis "github.com/redis/go-redis/v9"
    "database/sql"
)

type GatewayInfo struct {
    GatewayID      string
    CellID         string
    DownlinkAddr   string
    Status         string // ONLINE / DEGRADED / OFFLINE
    Version        string
    EBPFSupported  bool
    MaxConnections int32
    MaxSessions    int32
    ActiveSessions int32
    LastHeartbeat  time.Time
}

type Registry struct {
    gateways map[string]*GatewayInfo // gateway_id → info
    byCell   map[string][]string     // cell_id → []gateway_id
    mu       sync.RWMutex
    db       *sql.DB
    rdb      *goredis.Client
}

func NewRegistry(db *sql.DB, rdb *goredis.Client) *Registry {
    r := &Registry{
        gateways: make(map[string]*GatewayInfo),
        byCell:   make(map[string][]string),
        db:       db,
        rdb:      rdb,
    }
    r.loadFromDB()
    return r
}

// Register 注册 Gateway（DB + Redis + 内存）
func (r *Registry) Register(ctx context.Context, info *GatewayInfo) error {
    info.Status = "ONLINE"
    info.LastHeartbeat = time.Now()

    // 1. DB UPSERT
    _, err := r.db.ExecContext(ctx, `
        INSERT INTO gateways (id, cell_id, ip_address, status, ebpf_loaded, last_heartbeat, updated_at)
        VALUES ($1, $2, $3, 'ONLINE', $4, NOW(), NOW())
        ON CONFLICT (id) DO UPDATE SET
            cell_id = EXCLUDED.cell_id,
            ip_address = EXCLUDED.ip_address,
            status = 'ONLINE',
            ebpf_loaded = EXCLUDED.ebpf_loaded,
            last_heartbeat = NOW(),
            updated_at = NOW()
    `, info.GatewayID, info.CellID, info.DownlinkAddr, info.EBPFSupported)
    if err != nil {
        return fmt.Errorf("db upsert: %w", err)
    }

    // 2. Redis 拓扑索引
    pipe := r.rdb.Pipeline()
    pipe.HSet(ctx, fmt.Sprintf("topo:gw:%s", info.GatewayID), map[string]interface{}{
        "cell_id":       info.CellID,
        "downlink_addr": info.DownlinkAddr,
        "status":        "ONLINE",
        "version":       info.Version,
        "max_sessions":  info.MaxSessions,
    })
    pipe.Expire(ctx, fmt.Sprintf("topo:gw:%s", info.GatewayID), 10*time.Minute)
    pipe.SAdd(ctx, fmt.Sprintf("topo:cell:%s:gateways", info.CellID), info.GatewayID)
    pipe.Set(ctx, fmt.Sprintf("gateway:%s:addr", info.GatewayID), info.DownlinkAddr, 10*time.Minute)
    pipe.Set(ctx, fmt.Sprintf("gateway:%s:status", info.GatewayID), "ONLINE", 60*time.Second)
    pipe.Exec(ctx)

    // 3. 内存索引
    r.mu.Lock()
    defer r.mu.Unlock()
    // 如果 Cell 变更，从旧 Cell 移除
    if old, ok := r.gateways[info.GatewayID]; ok && old.CellID != info.CellID {
        r.removeCellIndex(old.CellID, info.GatewayID)
    }
    r.gateways[info.GatewayID] = info
    r.byCell[info.CellID] = appendUnique(r.byCell[info.CellID], info.GatewayID)

    log.Printf("[Registry] Gateway %s 注册成功 (cell=%s, addr=%s)", info.GatewayID, info.CellID, info.DownlinkAddr)
    return nil
}

// GetGatewaysByCell 查询 Cell 下所有在线 Gateway
func (r *Registry) GetGatewaysByCell(cellID string) []*GatewayInfo {
    r.mu.RLock()
    defer r.mu.RUnlock()
    var result []*GatewayInfo
    for _, gwID := range r.byCell[cellID] {
        if gw, ok := r.gateways[gwID]; ok && gw.Status == "ONLINE" {
            result = append(result, gw)
        }
    }
    return result
}

// GetAllOnline 查询所有在线 Gateway
func (r *Registry) GetAllOnline() []*GatewayInfo {
    r.mu.RLock()
    defer r.mu.RUnlock()
    var result []*GatewayInfo
    for _, gw := range r.gateways {
        if gw.Status == "ONLINE" {
            result = append(result, gw)
        }
    }
    return result
}

// UpdateHeartbeat 心跳刷新
func (r *Registry) UpdateHeartbeat(gatewayID string, activeSessions int32, stateHash string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    if gw, ok := r.gateways[gatewayID]; ok {
        gw.LastHeartbeat = time.Now()
        gw.ActiveSessions = activeSessions
        gw.Status = "ONLINE"
    }
}

// MarkOffline 标记下线
func (r *Registry) MarkOffline(ctx context.Context, gatewayID string) {
    r.mu.Lock()
    gw, ok := r.gateways[gatewayID]
    if ok {
        gw.Status = "OFFLINE"
    }
    r.mu.Unlock()

    if ok {
        r.db.ExecContext(ctx, `UPDATE gateways SET status = 'OFFLINE', updated_at = NOW() WHERE id = $1`, gatewayID)
        r.rdb.HSet(ctx, fmt.Sprintf("topo:gw:%s", gatewayID), "status", "OFFLINE")
        r.rdb.SRem(ctx, fmt.Sprintf("topo:cell:%s:gateways", gw.CellID), gatewayID)
        log.Printf("[Registry] Gateway %s 标记下线", gatewayID)
    }
}

// StartTimeoutChecker 启动心跳超时检查（每 60 秒）
func (r *Registry) StartTimeoutChecker(ctx context.Context, timeout time.Duration) {
    go func() {
        ticker := time.NewTicker(60 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done(): return
            case <-ticker.C:
                r.checkTimeouts(ctx, timeout)
            }
        }
    }()
}

func (r *Registry) checkTimeouts(ctx context.Context, timeout time.Duration) {
    r.mu.RLock()
    var expired []string
    for gwID, gw := range r.gateways {
        if gw.Status == "ONLINE" && time.Since(gw.LastHeartbeat) > timeout {
            expired = append(expired, gwID)
        }
    }
    r.mu.RUnlock()

    for _, gwID := range expired {
        r.MarkOffline(ctx, gwID)
    }
}
```

---

## 模块 2：按维度下推（需求 2）

### 改动范围

- `mirage-os/gateway-bridge/pkg/dispatch/fanout.go`（新建）：统一 fan-out 引擎
- `mirage-os/gateway-bridge/pkg/dispatch/push_log.go`（新建）：下推状态记录

### 设计细节

#### Fan-out 引擎

```go
// pkg/dispatch/fanout.go
package dispatch

import (
    "context"
    "fmt"
    "log"
    "time"
)

type FanoutScope int

const (
    ScopeSingle FanoutScope = iota // 单 Gateway
    ScopeCell                       // 按 Cell
    ScopeGlobal                     // 全局
)

type PushCommand struct {
    CommandType string      // strategy / quota / blacklist / reincarnation
    Payload     interface{}
    Scope       FanoutScope
    TargetID    string // gateway_id 或 cell_id
}

type FanoutEngine struct {
    registry   *topology.Registry
    dispatcher *StrategyDispatcher
    pushLog    *PushLog
}

func NewFanoutEngine(registry *topology.Registry, dispatcher *StrategyDispatcher, pushLog *PushLog) *FanoutEngine {
    return &FanoutEngine{registry: registry, dispatcher: dispatcher, pushLog: pushLog}
}

// Execute 执行下推
func (fe *FanoutEngine) Execute(ctx context.Context, cmd *PushCommand) error {
    targets := fe.resolveTargets(cmd)
    if len(targets) == 0 {
        return fmt.Errorf("no online gateways for scope=%d target=%s", cmd.Scope, cmd.TargetID)
    }

    var lastErr error
    for _, gwID := range targets {
        err := fe.pushToGateway(ctx, gwID, cmd)
        result := "success"
        if err != nil {
            result = err.Error()
            lastErr = err
            // 重试（最多 3 次，指数退避）
            for attempt := 1; attempt <= 3; attempt++ {
                time.Sleep(time.Duration(attempt*attempt) * time.Second)
                if retryErr := fe.pushToGateway(ctx, gwID, cmd); retryErr == nil {
                    result = fmt.Sprintf("success_after_retry_%d", attempt)
                    lastErr = nil
                    break
                }
            }
            if lastErr != nil {
                result = "failed_after_retries"
                log.Printf("[Fanout] ⚠️ 下推失败 gateway=%s cmd=%s: %v", gwID, cmd.CommandType, lastErr)
            }
        }
        fe.pushLog.Record(gwID, cmd.CommandType, result)
    }
    return lastErr
}

func (fe *FanoutEngine) resolveTargets(cmd *PushCommand) []string {
    switch cmd.Scope {
    case ScopeSingle:
        return []string{cmd.TargetID}
    case ScopeCell:
        gws := fe.registry.GetGatewaysByCell(cmd.TargetID)
        ids := make([]string, len(gws))
        for i, gw := range gws { ids[i] = gw.GatewayID }
        return ids
    case ScopeGlobal:
        gws := fe.registry.GetAllOnline()
        ids := make([]string, len(gws))
        for i, gw := range gws { ids[i] = gw.GatewayID }
        return ids
    }
    return nil
}
```

#### 下推状态记录

```go
// pkg/dispatch/push_log.go
type PushLogEntry struct {
    GatewayID   string    `json:"gateway_id"`
    CommandType string    `json:"command_type"`
    Result      string    `json:"result"`
    Timestamp   time.Time `json:"timestamp"`
}

type PushLog struct {
    entries []PushLogEntry
    mu      sync.Mutex
    db      *sql.DB
    maxSize int
}

func (pl *PushLog) Record(gatewayID, cmdType, result string) {
    entry := PushLogEntry{
        GatewayID: gatewayID, CommandType: cmdType,
        Result: result, Timestamp: time.Now(),
    }
    pl.mu.Lock()
    pl.entries = append(pl.entries, entry)
    if len(pl.entries) > pl.maxSize {
        pl.entries = pl.entries[len(pl.entries)-pl.maxSize:]
    }
    pl.mu.Unlock()

    // 异步写 DB
    go pl.persistToDB(entry)
}

func (pl *PushLog) persistToDB(entry PushLogEntry) {
    pl.db.Exec(`INSERT INTO push_logs (gateway_id, command_type, result, created_at) VALUES ($1, $2, $3, $4)`,
        entry.GatewayID, entry.CommandType, entry.Result, entry.Timestamp)
}

func (pl *PushLog) GetRecent(limit int) []PushLogEntry {
    pl.mu.Lock()
    defer pl.mu.Unlock()
    start := len(pl.entries) - limit
    if start < 0 { start = 0 }
    result := make([]PushLogEntry, len(pl.entries)-start)
    copy(result, pl.entries[start:])
    return result
}
```

---

## 模块 3：心跳拓扑语义扩展（需求 3）

### 改动范围

- `mirage-proto/mirage.proto`：扩展 HeartbeatRequest/Response
- `mirage-os/gateway-bridge/pkg/grpc/server.go`：改造 SyncHeartbeat handler

### 设计细节

#### Proto 扩展

```protobuf
message HeartbeatRequest {
    string gateway_id = 1;
    int64 timestamp = 2;
    GatewayStatus status = 3;
    bool ebpf_loaded = 4;
    int32 threat_level = 5;
    int64 active_connections = 6;
    int32 memory_usage_mb = 7;
    repeated UserQuotaSummary user_quotas = 8;  // Spec 2-2
    // 新增：拓扑语义
    string downlink_addr = 9;
    string cell_id = 10;
    int32 active_sessions = 11;
    string state_hash = 12;
    string version = 13;
}

message HeartbeatResponse {
    bool ack = 1;
    int64 server_time = 2;
    double remaining_quota = 3;
    // 新增：状态对齐
    bool needs_full_sync = 4;
    string desired_state_hash = 5;
}
```

#### SyncHeartbeat 改造

```go
func (s *Server) SyncHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
    // ... 现有校验

    // 1. 更新拓扑索引
    s.registry.UpdateHeartbeat(req.GatewayId, req.ActiveSessions, req.StateHash)

    // 2. 更新 DB（现有逻辑保持）
    s.db.ExecContext(ctx, `...UPSERT...`)

    // 3. 状态对齐检查
    needsSync := false
    desiredHash := ""
    if req.StateHash != "" {
        _, expectedHash, err := s.downlink.GetDesiredState(ctx, req.GatewayId)
        if err == nil && expectedHash != req.StateHash {
            needsSync = true
            desiredHash = expectedHash
        }
    }

    // 4. 查询配额
    remainingQuota, _ := s.enforcer.GetRemainingQuota(req.GatewayId)

    // 5. 重试待推送
    s.dispatcher.RetryPending(req.GatewayId)

    return &pb.HeartbeatResponse{
        Ack:               true,
        ServerTime:         time.Now().Unix(),
        RemainingQuota:     remainingQuota,
        NeedsFullSync:      needsSync,
        DesiredStateHash:   desiredHash,
    }, nil
}
```

---

## 模块 4：DB 扩展

### 改动范围

- `mirage-os/api-server/src/prisma/schema.prisma`：新增 push_logs 表，扩展 gateways 表

### 设计细节

#### push_logs 表

```prisma
model PushLog {
  id          String   @id @default(uuid())
  gatewayId   String   @map("gateway_id")
  commandType String   @map("command_type")
  result      String
  createdAt   DateTime @default(now()) @map("created_at")

  @@map("push_logs")
  @@index([gatewayId])
  @@index([createdAt])
  @@index([commandType])
}
```

#### gateways 表扩展

```prisma
model Gateway {
  // ... 现有字段
  downlinkAddr  String?  @map("downlink_addr")
  version       String?
  maxSessions   Int?     @map("max_sessions")
  activeSessions Int     @default(0) @map("active_sessions")
}
```

---

## 模块 5：NestJS 查询 API

### 改动范围

- `mirage-os/api-server/src/modules/gateways/gateways.controller.ts`：增加拓扑查询
- `mirage-os/api-server/src/modules/gateways/gateways.service.ts`：增加查询方法

### 设计细节

```typescript
// gateways.controller.ts 新增
@Get('topology/by-cell/:cellId')
@Permissions(Permission.GATEWAY_READ)
async getGatewaysByCell(@Param('cellId') cellId: string) {
  return this.gatewaysService.findByCellOnline(cellId);
}

@Get('topology/online')
@Permissions(Permission.GATEWAY_READ)
async getOnlineGateways() {
  return this.gatewaysService.findAllOnline();
}

@Get('push-logs')
@Permissions(Permission.GATEWAY_READ)
async getPushLogs(@Query('limit') limit = 50) {
  return this.gatewaysService.getRecentPushLogs(limit);
}
```

---

## 配置变更

### mirage-os.yaml 新增

```yaml
topology:
  heartbeat_timeout_seconds: 300
  timeout_check_interval_seconds: 60

push:
  max_retries: 3
  retry_backoff_base_seconds: 1
  log_retention_days: 30
```

### gateway.yaml 变更

Gateway 启动时需先调用 `RegisterGateway`，配置中需确保 `mcc.endpoint` 正确。

## 不在本次范围内

- Client 运行时拓扑学习（Spec 3-1）
- 路由表/拓扑同步协议 Proto-3（Spec 3-1）
- 跨节点威胁情报中心（版本级改造）
- Gateway 漂移自动重分配（版本级改造）
