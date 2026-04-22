# 设计文档：Mirage 抗 DDoS 架构整改

## 概述

本设计覆盖抗 DDoS 架构的完整四层防御：Gateway 入口准入层、OS 编排裁决层、Client 生存恢复层、V2 编排内核集成。遵循"C 做数据面，Go 做控制面"的铁律。

## 设计原则

1. **数据包处理 = C**（XDP/TC 层），**业务逻辑 = Go**（API/数据库/Raft）
2. **通信 = eBPF Map + Ring Buffer**，禁止直接函数调用
3. **尽可能前移到 XDP**：能在 XDP 做的不拖到 TC，能在 TC 做的不拖到用户态
4. **体积型攻击下优先保全系统，而不是保全节点**
5. **不做单 IP 粗暴限流**：考虑 CGNAT/企业出口场景

---

## 模块 1：多协议感知入口守卫完善（需求 1）

### 改动范围
- `mirage-gateway/pkg/ebpf/manager.go`：启动时自动同步默认入口画像
- `mirage-gateway/cmd/gateway/main.go`：在 DefenseApplier 启动后调用 SyncIngressProfiles

### 设计细节

上一轮已在 `l1_defense.c` 中实现了 `ingress_profile_map` 查询和 `handle_l1_ingress_profile` 函数。本轮需要在 Go 控制面启动时自动同步默认配置：

```go
// DefenseApplier.Start() 中增加
func (da *DefenseApplier) Start() {
    // ... 现有逻辑
    // 同步默认入口画像
    if err := da.SyncIngressProfiles(DefaultIngressProfiles()); err != nil {
        log.Printf("⚠️ 入口画像同步失败（降级）: %v", err)
    }
}
```

OS 下发动态更新通过 Downlink 的 PushStrategy 扩展，携带入口画像配置。

---

## 模块 2：多维准入控制（需求 3）

### 改动范围
- `mirage-gateway/pkg/threat/admission.go`（新建）：多维准入评分器
- `mirage-gateway/pkg/threat/policy.go`：IngressPolicy 集成多维评分

### 设计细节

```go
// pkg/threat/admission.go
type AdmissionScorer struct {
    mu       sync.RWMutex
    scores   map[string]*IPScore // IP → 评分
    window   time.Duration       // 评分窗口（1 分钟）
}

type IPScore struct {
    NewConnRate     float64   // 新建连接速率
    ValidAuthRate   float64   // 有效验证通过率
    TokenValidRate  float64   // 会话令牌有效率
    ProfileMatchRate float64  // 入口画像匹配率
    ActiveSessions  int       // 活跃会话数（CGNAT 感知）
    LastUpdate      time.Time
}

// Score 计算综合评分（0-100，越高越可信）
func (s *IPScore) Score() float64 {
    // CGNAT 感知：同一 IP 下有多个有效会话时提高可信度
    cgnatBonus := math.Min(float64(s.ActiveSessions)*5, 20)
    return s.ValidAuthRate*30 + s.TokenValidRate*30 + 
           s.ProfileMatchRate*20 + cgnatBonus - 
           math.Min(s.NewConnRate/10, 30)
}
```

评分器在 IngressPolicy.Evaluate 中作为额外维度参与决策。

---

## 模块 3：OS 软防响应链路（需求 4）

### 改动范围
- `mirage-os/gateway-bridge/pkg/topology/registry.go`：已有 MarkUnderAttack/RecoverFromAttack
- `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`（新建）：软防/硬断响应协调器
- `mirage-os/services/provisioning/tier_router.go`：跳过 UNDER_ATTACK 节点

### 设计细节

```go
// pkg/topology/ddos_responder.go
type DDoSResponder struct {
    registry     *Registry
    scheduler    CellSchedulerInterface
    downlink     DownlinkInterface
    recoveryLog  []RecoveryEvent
    mu           sync.Mutex
}

// 软防响应：资源耗尽型攻击
func (d *DDoSResponder) HandleResourcePressure(ctx context.Context, gwID string) {
    d.registry.MarkUnderAttack(ctx, gwID)
    // 通知 Gateway 停止接受新连接
    d.downlink.PushDesiredState(ctx, gwID, "reject_new_connections", true)
    d.logEvent(gwID, "SOFT_DEFENSE_START", "resource_pressure")
}

// 软防恢复：连续 N 次心跳正常
func (d *DDoSResponder) HandleRecovery(ctx context.Context, gwID string) {
    d.registry.RecoverFromAttack(ctx, gwID)
    d.downlink.PushDesiredState(ctx, gwID, "reject_new_connections", false)
    d.logEvent(gwID, "SOFT_DEFENSE_END", "recovered")
}
```

TierRouter 修改：

```go
// AllocateGateway 中增加状态检查
func (r *TierRouter) AllocateGateway(...) {
    // 现有查询条件增加：AND status NOT IN ('UNDER_ATTACK', 'DRAINING', 'DEAD')
    query := r.db.Where("gateways.is_online = ? AND gateways.phase = 2", true).
        Where("gateways.status NOT IN ?", []string{"UNDER_ATTACK", "DRAINING", "DEAD"})
}
```

---

## 模块 4：OS 硬断响应链路（需求 5）

### 改动范围
- `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：硬断处理
- `mirage-os/pkg/strategy/cell_manager.go`：替补节点激活

### 设计细节

```go
// 硬断响应：体积型攻击
func (d *DDoSResponder) HandleNodeDeath(ctx context.Context, gwID string) {
    d.registry.MarkDead(ctx, gwID)
    
    // 获取死亡节点信息
    gw := d.registry.GetGateway(gwID)
    if gw == nil { return }
    
    // 从同 Cell 影子池选择替补
    replacement, err := d.scheduler.ActivateStandby(ctx, gw.CellID)
    if err != nil {
        log.Printf("[DDoSResponder] ❌ 无可用替补节点: cell=%s: %v", gw.CellID, err)
        return
    }
    
    // 发布新路由表
    d.publishNewTopology(ctx, gw.CellID)
    d.logEvent(gwID, "HARD_BREAK_REPLACED", replacement.GatewayID)
}
```

---

## 模块 5：Client 运行时拓扑持续学习（需求 6）

### 改动范围
- `phantom-client/pkg/gtclient/topo.go`：TopoRefresher 已有基础实现
- `phantom-client/pkg/gtclient/topo_cache.go`（新建）：本地持久化缓存

### 设计细节

TopoRefresher 已实现 5 分钟周期刷新和指数退避。需要补齐：

```go
// pkg/gtclient/topo_cache.go
type TopoCache struct {
    path string // 本地缓存文件路径
}

func (tc *TopoCache) Save(resp *RouteTableResponse) error {
    data, _ := json.Marshal(resp)
    return os.WriteFile(tc.path, data, 0600)
}

func (tc *TopoCache) Load() (*RouteTableResponse, error) {
    data, err := os.ReadFile(tc.path)
    if err != nil { return nil, err }
    var resp RouteTableResponse
    return &resp, json.Unmarshal(data, &resp)
}
```

在 TopoRefresher 成功拉取后调用 `cache.Save()`，启动时尝试 `cache.Load()` 作为初始拓扑。

---

## 模块 6：Client 失联后原子恢复状态机（需求 7）

### 改动范围
- `phantom-client/pkg/gtclient/recovery_fsm.go`（新建）：恢复状态机
- `phantom-client/pkg/gtclient/client.go`：doReconnect 集成恢复状态机

### 设计细节

```go
// pkg/gtclient/recovery_fsm.go
type RecoveryPhase int
const (
    PhaseJitter    RecoveryPhase = iota // 主链路抖动（< 5s）
    PhasePressure                       // 节点受压（5s-30s）
    PhaseDeath                          // 节点死亡（> 30s）
)

type RecoveryFSM struct {
    phase       RecoveryPhase
    startTime   time.Time
    attempts    int
    maxPerPhase int           // 每阶段最大重试次数
    phaseTimeout time.Duration // 每阶段超时
}

func (r *RecoveryFSM) Evaluate(disconnectDuration time.Duration) RecoveryPhase {
    switch {
    case disconnectDuration < 5*time.Second:
        return PhaseJitter
    case disconnectDuration < 30*time.Second:
        return PhasePressure
    default:
        return PhaseDeath
    }
}
```

集成到 doReconnect：
- PhaseJitter：在当前连接上重试 3 次，间隔 1s
- PhasePressure：触发拓扑刷新 + 同 Cell 切换
- PhaseDeath：执行现有 L1→L2→L3 降级

---

## 模块 7：Survival Orchestrator 集成（需求 8）

### 改动范围
- `mirage-os/pkg/orchestrator/survival.go`（新建）：生存编排器
- `mirage-os/gateway-bridge/pkg/topology/ddos_responder.go`：接入编排器

### 设计细节

```go
// pkg/orchestrator/survival.go
type SurvivalState int
const (
    SurvivalNormal    SurvivalState = iota
    SurvivalDegraded                       // 部分节点受压
    SurvivalCritical                       // 多节点受攻击
    SurvivalEmergency                      // 系统级危机
)

type SurvivalOrchestrator struct {
    registry    *topology.Registry
    responder   *topology.DDoSResponder
    state       SurvivalState
    mu          sync.RWMutex
}

func (so *SurvivalOrchestrator) Evaluate() {
    online := so.registry.GetAllOnline()
    underAttack := so.registry.GetByStatus("UNDER_ATTACK")
    dead := so.registry.GetByStatus("DEAD")
    
    totalActive := len(online) + len(underAttack)
    if totalActive == 0 {
        so.transition(SurvivalEmergency)
    } else if float64(len(underAttack)+len(dead))/float64(totalActive) > 0.5 {
        so.transition(SurvivalCritical)
    } else if len(underAttack) > 0 || len(dead) > 0 {
        so.transition(SurvivalDegraded)
    } else {
        so.transition(SurvivalNormal)
    }
}
```

---

## 模块 8：State Commit Engine（需求 9）

### 改动范围
- `mirage-os/pkg/orchestrator/commit_engine.go`（新建）：事务化替补引擎

### 设计细节

```go
type ReplacementTx struct {
    TxID        string
    OldGateway  string
    NewGateway  string
    CellID      string
    Steps       []TxStep
    Status      TxStatus // Pending/Committed/RolledBack/Failed
    CreatedAt   time.Time
}

type TxStep struct {
    Name    string
    Action  func(ctx context.Context) error
    Rollback func(ctx context.Context) error
    Done    bool
}

func (e *CommitEngine) ExecuteReplacement(ctx context.Context, tx *ReplacementTx) error {
    for i, step := range tx.Steps {
        if err := step.Action(ctx); err != nil {
            // 回滚已执行的步骤
            for j := i - 1; j >= 0; j-- {
                if tx.Steps[j].Rollback != nil {
                    tx.Steps[j].Rollback(ctx)
                }
            }
            tx.Status = TxStatusRolledBack
            return err
        }
        tx.Steps[i].Done = true
    }
    tx.Status = TxStatusCommitted
    return nil
}
```

---

## 模块 9：预算模型（需求 10）

### 改动范围
- `mirage-os/pkg/orchestrator/budget.go`（新建）：抗 DDoS 预算模型

### 设计细节

```go
type DDoSBudget struct {
    TierBudgets map[int]*TierBudget // cellLevel → budget
}

type TierBudget struct {
    MaxHotStandby    int     // 最大热备节点数
    MaxSwitchPerHour int     // 每小时最大切换次数
    GuardIntensity   int     // 入口守卫强度（1-10）
    RecoveryPriority int     // 恢复优先级
    CurrentUsage     float64 // 当前预算使用率
}

var defaultBudgets = map[int]*TierBudget{
    3: {MaxHotStandby: 3, MaxSwitchPerHour: 20, GuardIntensity: 10, RecoveryPriority: 3},
    2: {MaxHotStandby: 2, MaxSwitchPerHour: 10, GuardIntensity: 7, RecoveryPriority: 2},
    1: {MaxHotStandby: 1, MaxSwitchPerHour: 5, GuardIntensity: 5, RecoveryPriority: 1},
}
```

---

## 模块 10：独立恢复发布平面（需求 11）

### 改动范围
- `mirage-os/pkg/recovery/publisher.go`（新建）：恢复拓扑发布器
- `phantom-client/pkg/resonance/resolver.go`：已有多通道发现

### 设计细节

OS 侧在节点 DEAD 后通过多通道发布替补拓扑：

```go
type RecoveryPublisher struct {
    channels []PublishChannel // DNS TXT / Gist / Mastodon
}

func (rp *RecoveryPublisher) PublishReplacement(ctx context.Context, cellID string, newGateways []GatewayEndpoint) error {
    signal := &resonance.Signal{
        Gateways: newGateways,
        Version:  time.Now().UnixNano(),
    }
    for _, ch := range rp.channels {
        go ch.Publish(ctx, signal)
    }
    return nil
}
```

Client 侧已有 Resonance Resolver 作为 L3 恢复通道，无需额外修改。

---

## 模块 11：Gateway 预热池（需求 12）

### 改动范围
- `mirage-os/pkg/strategy/cell_manager.go`：已有影子池基础，需要完善三级池管理

### 设计细节

CellScheduler 已有 `loadShadowPool` 和 Phase 概念（0=潜伏, 1=校准, 2=服役）。需要补齐：

```go
// ActivateStandby 从温备池激活替补节点
func (s *CellScheduler) ActivateStandby(ctx context.Context, cellID string) (*models.Gateway, error) {
    // 优先从 Phase=1（温备）中选择
    var standby models.Gateway
    err := s.db.Where("cell_id = ? AND phase = 1 AND is_online = true", cellID).
        Order("baseline_rtt ASC").First(&standby).Error
    if err != nil {
        // 温备池耗尽，从 Phase=0（冷备）中选择
        err = s.db.Where("cell_id = ? AND phase = 0 AND is_online = true", cellID).
            Order("baseline_rtt ASC").First(&standby).Error
        if err != nil {
            return nil, fmt.Errorf("no standby available in cell %s", cellID)
        }
    }
    // 激活：Phase → 2
    s.db.Model(&standby).Update("phase", 2)
    return &standby, nil
}
```

---

## 不在本次范围内

- 上游/物理层防护（Layer 1）— 依赖云厂商或运营商
- eBPF SYN Cookie 的完整内核实现（需要 Linux 5.15+ 特定内核支持，本轮先做用户态降级方案）
- 跨区域容灾（需要多 OS 集群支持）
