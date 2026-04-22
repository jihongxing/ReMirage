// Package topology - DDoS 响应协调器
// 软防（资源耗尽型）+ 硬断（体积型）响应闭环
package topology

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// DownlinkInterface Gateway 下行通信接口
type DownlinkInterface interface {
	PushDesiredState(ctx context.Context, gwID string, key string, value interface{}) error
}

// CellSchedulerInterface 蜂窝调度器接口
type CellSchedulerInterface interface {
	ActivateStandby(ctx context.Context, cellID string) (StandbyResult, error)
}

// StandbyResult 替补激活结果
type StandbyResult struct {
	GatewayID string
	IPAddress string
	CellID    string
}

// RecoveryEvent 状态变迁事件
type RecoveryEvent struct {
	GatewayID string
	EventType string // SOFT_DEFENSE_START / SOFT_DEFENSE_END / HARD_BREAK_REPLACED / NODE_DEATH
	Reason    string
	Timestamp time.Time
	Duration  time.Duration
}

// DDoSResponder DDoS 响应协调器
type DDoSResponder struct {
	registry        *Registry
	scheduler       CellSchedulerInterface
	downlink        DownlinkInterface
	commitEngine    CommitEngineInterface
	budgetChecker   BudgetCheckerInterface
	publisher       RecoveryPublisherInterface
	recoveryCounter map[string]int // gwID → 连续正常心跳计数
	recoveryLog     []RecoveryEvent
	mu              sync.Mutex
}

// CommitEngineInterface 事务化替补引擎接口
type CommitEngineInterface interface {
	ExecuteReplacement(ctx context.Context, oldGW, newGW, cellID string) error
}

// BudgetCheckerInterface 预算检查接口
type BudgetCheckerInterface interface {
	CheckBudget(cellLevel int, action string) (bool, error)
	ConsumeBudget(cellLevel int, action string, cost float64)
}

// RecoveryPublisherInterface 恢复发布平面接口
type RecoveryPublisherInterface interface {
	PublishReplacement(ctx context.Context, cellID string, newGateways []GatewayEndpointInfo) error
}

// GatewayEndpointInfo 网关端点信息
type GatewayEndpointInfo struct {
	IP   string
	Port int
}

// NewDDoSResponder 创建 DDoS 响应协调器
func NewDDoSResponder(registry *Registry) *DDoSResponder {
	return &DDoSResponder{
		registry:        registry,
		recoveryCounter: make(map[string]int),
		recoveryLog:     make([]RecoveryEvent, 0),
	}
}

// SetScheduler 注入蜂窝调度器
func (d *DDoSResponder) SetScheduler(s CellSchedulerInterface) { d.scheduler = s }

// SetDownlink 注入下行通信
func (d *DDoSResponder) SetDownlink(dl DownlinkInterface) { d.downlink = dl }

// SetCommitEngine 注入事务化替补引擎
func (d *DDoSResponder) SetCommitEngine(ce CommitEngineInterface) { d.commitEngine = ce }

// SetBudgetChecker 注入预算检查器
func (d *DDoSResponder) SetBudgetChecker(bc BudgetCheckerInterface) { d.budgetChecker = bc }

// SetPublisher 注入恢复发布器
func (d *DDoSResponder) SetPublisher(p RecoveryPublisherInterface) { d.publisher = p }

// HandleResourcePressure 软防响应：资源耗尽型攻击
func (d *DDoSResponder) HandleResourcePressure(ctx context.Context, gwID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.registry.MarkUnderAttack(ctx, gwID)

	// 通知 Gateway 停止接受新连接
	if d.downlink != nil {
		if err := d.downlink.PushDesiredState(ctx, gwID, "reject_new_connections", true); err != nil {
			log.Printf("[DDoSResponder] ⚠️ 通知 Gateway %s 停止接受连接失败: %v", gwID, err)
		}
	}

	// 重置恢复计数器
	d.recoveryCounter[gwID] = 0

	d.logEvent(gwID, "SOFT_DEFENSE_START", "resource_pressure")
	log.Printf("[DDoSResponder] 🔴 Gateway %s 进入软防模式", gwID)
}

// HandleRecovery 软防恢复：连续 N 次心跳正常
func (d *DDoSResponder) HandleRecovery(ctx context.Context, gwID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.registry.RecoverFromAttack(ctx, gwID)

	// 通知 Gateway 恢复接受连接
	if d.downlink != nil {
		if err := d.downlink.PushDesiredState(ctx, gwID, "reject_new_connections", false); err != nil {
			log.Printf("[DDoSResponder] ⚠️ 通知 Gateway %s 恢复接受连接失败: %v", gwID, err)
		}
	}

	delete(d.recoveryCounter, gwID)

	d.logEvent(gwID, "SOFT_DEFENSE_END", "recovered")
	log.Printf("[DDoSResponder] 🟢 Gateway %s 从软防模式恢复", gwID)
}

// IncrementRecoveryCounter 递增恢复计数器，返回是否达到恢复阈值（连续 3 次正常）
func (d *DDoSResponder) IncrementRecoveryCounter(gwID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.recoveryCounter[gwID]++
	return d.recoveryCounter[gwID] >= 3
}

// ResetRecoveryCounter 重置恢复计数器
func (d *DDoSResponder) ResetRecoveryCounter(gwID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recoveryCounter[gwID] = 0
}

// HandleNodeDeath 硬断响应：体积型攻击
func (d *DDoSResponder) HandleNodeDeath(ctx context.Context, gwID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.registry.MarkDead(ctx, gwID)
	d.logEvent(gwID, "NODE_DEATH", "heartbeat_timeout_3x")

	// 获取死亡节点信息
	d.registry.mu.RLock()
	gw, ok := d.registry.gateways[gwID]
	var cellID string
	if ok {
		cellID = gw.CellID
	}
	d.registry.mu.RUnlock()

	if !ok || cellID == "" {
		log.Printf("[DDoSResponder] ❌ 死亡节点 %s 信息不存在", gwID)
		return
	}

	// 预算检查
	if d.budgetChecker != nil {
		allowed, err := d.budgetChecker.CheckBudget(0, "activate_standby")
		if err != nil || !allowed {
			log.Printf("[DDoSResponder] ⚠️ 预算不足，无法激活替补: cell=%s: %v", cellID, err)
			return
		}
	}

	// 通过 CommitEngine 执行事务化替补（如果可用）
	if d.commitEngine != nil {
		if d.scheduler == nil {
			log.Printf("[DDoSResponder] ❌ 无调度器，无法激活替补")
			return
		}
		replacement, err := d.scheduler.ActivateStandby(ctx, cellID)
		if err != nil {
			log.Printf("[DDoSResponder] ❌ 无可用替补节点: cell=%s: %v", cellID, err)
			return
		}
		if err := d.commitEngine.ExecuteReplacement(ctx, gwID, replacement.GatewayID, cellID); err != nil {
			log.Printf("[DDoSResponder] ❌ 事务化替补失败: %v", err)
			return
		}
		d.logEvent(gwID, "HARD_BREAK_REPLACED", fmt.Sprintf("replacement=%s", replacement.GatewayID))
	} else if d.scheduler != nil {
		// 直接替补（无事务引擎时的降级路径）
		replacement, err := d.scheduler.ActivateStandby(ctx, cellID)
		if err != nil {
			log.Printf("[DDoSResponder] ❌ 无可用替补节点: cell=%s: %v", cellID, err)
			return
		}
		d.logEvent(gwID, "HARD_BREAK_REPLACED", fmt.Sprintf("replacement=%s", replacement.GatewayID))
		log.Printf("[DDoSResponder] ✅ 替补节点已激活: %s → %s", gwID, replacement.GatewayID)
	}

	// 发布新拓扑
	d.publishNewTopology(ctx, cellID)

	// 通过恢复发布平面发布
	if d.publisher != nil {
		d.publishToRecoveryPlane(ctx, cellID)
	}

	// 消耗预算
	if d.budgetChecker != nil {
		d.budgetChecker.ConsumeBudget(0, "activate_standby", 1.0)
	}
}

// publishNewTopology 发布新路由表给关联 Client
func (d *DDoSResponder) publishNewTopology(ctx context.Context, cellID string) {
	onlineGWs := d.registry.GetGatewaysByCell(cellID)
	if len(onlineGWs) == 0 {
		log.Printf("[DDoSResponder] ⚠️ Cell %s 无在线节点，无法发布拓扑", cellID)
		return
	}
	log.Printf("[DDoSResponder] 📡 发布新拓扑: cell=%s, nodes=%d", cellID, len(onlineGWs))
}

// publishToRecoveryPlane 通过恢复发布平面发布替补拓扑
func (d *DDoSResponder) publishToRecoveryPlane(ctx context.Context, cellID string) {
	onlineGWs := d.registry.GetGatewaysByCell(cellID)
	endpoints := make([]GatewayEndpointInfo, 0, len(onlineGWs))
	for _, gw := range onlineGWs {
		endpoints = append(endpoints, GatewayEndpointInfo{
			IP:   gw.DownlinkAddr,
			Port: 443,
		})
	}
	if err := d.publisher.PublishReplacement(ctx, cellID, endpoints); err != nil {
		log.Printf("[DDoSResponder] ⚠️ 恢复平面发布失败: %v", err)
	}
}

// logEvent 记录状态变迁事件
func (d *DDoSResponder) logEvent(gwID, eventType, reason string) {
	event := RecoveryEvent{
		GatewayID: gwID,
		EventType: eventType,
		Reason:    reason,
		Timestamp: time.Now(),
	}
	d.recoveryLog = append(d.recoveryLog, event)
	// 保留最近 1000 条
	if len(d.recoveryLog) > 1000 {
		d.recoveryLog = d.recoveryLog[len(d.recoveryLog)-500:]
	}
}

// GetRecoveryLog 获取恢复日志
func (d *DDoSResponder) GetRecoveryLog() []RecoveryEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]RecoveryEvent, len(d.recoveryLog))
	copy(result, d.recoveryLog)
	return result
}
