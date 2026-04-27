// Package provisioning - 用户等级判定与路由下发
// 根据用户 CellLevel 分配对应等级的 Gateway IP 和证书
// Diamond: 物理独占节点（CN2 GIA/IPLC）| Platinum: 高优先级共享 | Standard: 共享池
// 跨级分配严厉禁止：Standard 用户被封不能连累 Diamond 用户
package provisioning

import (
	"fmt"
	"log"
	"mirage-os/pkg/models"
	"mirage-os/pkg/redact"
	"sort"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TierRouter 等级路由器
type TierRouter struct {
	db *gorm.DB
	mu sync.RWMutex

	// 缓存：cellLevel → 可用 Gateway 列表
	cache map[int][]GatewayRoute
}

// GatewayRoute 路由条目
type GatewayRoute struct {
	GatewayID   string `json:"gateway_id"`
	IPAddress   string `json:"ip_address"`
	CellID      string `json:"cell_id"`
	Region      string `json:"region"`
	Connections int    `json:"connections"`
	Phase       int    `json:"phase"` // 0:潜伏 1:校准 2:服役
}

// NewTierRouter 创建等级路由器
func NewTierRouter(db *gorm.DB) *TierRouter {
	return &TierRouter{
		db:    db,
		cache: make(map[int][]GatewayRoute),
	}
}

// DetermineUserTier 判定用户等级
// 规则：直接返回用户已购买的 cell_level，不再基于余额推导
// cell_level 仅通过 PurchaseTierSubscription 或到期降级任务变更
func DetermineUserTier(user *models.User) int {
	if user.CellLevel >= 1 && user.CellLevel <= 3 {
		return user.CellLevel
	}
	return 1 // Default: Standard
}

// TierLabel 等级标签
func TierLabel(level int) string {
	switch level {
	case 3:
		return "diamond"
	case 2:
		return "platinum"
	default:
		return "standard"
	}
}

// TierConfig 等级服务差异配置
type TierConfig struct {
	MaxLoadPercent   float32 // 分配时的负载阈值
	ConnectionRatio  float32 // 连接上限比例（相对默认值）
	RecoveryPriority int     // 恢复优先级（数字越大越优先）
}

var tierConfigs = map[int]TierConfig{
	1: {MaxLoadPercent: 80, ConnectionRatio: 1.0, RecoveryPriority: 1}, // Standard
	2: {MaxLoadPercent: 60, ConnectionRatio: 0.7, RecoveryPriority: 2}, // Platinum
	3: {MaxLoadPercent: 40, ConnectionRatio: 0.4, RecoveryPriority: 3}, // Diamond
}

// GetTierLoadThreshold 获取等级负载阈值
func GetTierLoadThreshold(level int) float32 {
	if cfg, ok := tierConfigs[level]; ok {
		return cfg.MaxLoadPercent
	}
	return 80
}

// GetTierConnectionRatio 获取等级连接上限比例
func GetTierConnectionRatio(level int) float32 {
	if cfg, ok := tierConfigs[level]; ok {
		return cfg.ConnectionRatio
	}
	return 1.0
}

// GetTierRecoveryPriority 获取等级恢复优先级
func GetTierRecoveryPriority(level int) int {
	if cfg, ok := tierConfigs[level]; ok {
		return cfg.RecoveryPriority
	}
	return 1
}

// AllocateGateway 为用户分配 Gateway（按等级路由，支持降级分配）
// Diamond: 独占节点（负载 < 40%，仅 cell_level=3 的蜂窝）
// Platinum: 低负载节点（负载 < 60%，仅 cell_level=2 的蜂窝）
// Standard: 共享池（负载 < 80%，仅 cell_level=1 的蜂窝）
// 降级分配：对应等级池无可用 Cell 时降级到低一级资源池
func (r *TierRouter) AllocateGateway(userID string) (*GatewayRoute, error) {
	var user models.User
	if err := r.db.Where("user_id = ?", userID).First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户不存在")
	}

	tier := DetermineUserTier(&user)

	// cell_level 不再自动更新，仅通过 PurchaseTierSubscription 或到期降级任务变更

	// 按等级查询可用 Gateway，支持降级分配
	var routes []GatewayRoute
	var allocatedTier int
	for level := tier; level >= 1; level-- {
		var err error
		routes, err = r.queryAvailableGateways(level)
		if err == nil && len(routes) > 0 {
			allocatedTier = level
			if level < tier {
				log.Printf("⚠️ [TierRouter] 用户等级 %s(%d) 资源池无可用节点，降级到 %s(%d) 资源池: user=%s",
					TierLabel(tier), tier, TierLabel(level), level, redact.Token(userID))
			}
			break
		}
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("无可用 %s 级别节点", TierLabel(tier))
	}

	// 选择最优节点
	best := r.selectBest(routes, allocatedTier)

	// 使用 FOR UPDATE 锁定 Gateway 行，防止并发分配同一节点
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var gw models.Gateway
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("gateway_id = ? AND is_online = true AND phase = 2", best.GatewayID).
			First(&gw).Error; err != nil {
			return fmt.Errorf("节点已被占用")
		}

		// Diamond 独占检查
		if allocatedTier == 3 && gw.ActiveConnections > 0 {
			return fmt.Errorf("Diamond 节点已被占用")
		}

		// 绑定
		return tx.Model(&gw).Updates(map[string]any{
			"user_id":            userID,
			"active_connections": gorm.Expr("active_connections + 1"),
		}).Error
	})

	if err != nil {
		return nil, err
	}

	log.Printf("✅ [TierRouter] 分配 %s 节点: user=%s, gw=%s, ip=%s",
		TierLabel(allocatedTier), redact.Token(userID), best.GatewayID, redact.IP(best.IPAddress))

	return best, nil
}

// queryAvailableGateways 查询指定等级的可用 Gateway（按等级应用不同负载阈值）
func (r *TierRouter) queryAvailableGateways(tier int) ([]GatewayRoute, error) {
	var gateways []models.Gateway

	// 获取等级对应的连接上限比例
	connRatio := GetTierConnectionRatio(tier)

	// 基础条件：在线 + 服役中 + 非受攻击/排空/死亡状态
	query := r.db.Where("gateways.is_online = ? AND gateways.phase = 2", true).
		Where("gateways.status NOT IN ?", []string{"UNDER_ATTACK", "DRAINING", "DEAD"})

	// 按等级筛选蜂窝 + 应用连接上限比例
	switch tier {
	case 3: // Diamond: 独占，仅 level=3 蜂窝，连接上限 * 0.4
		query = query.Where("gateways.active_connections = 0").
			Where("gateways.cell_id IN (SELECT cell_id FROM cells WHERE cell_level = 3 AND status = 'active')")
	case 2: // Platinum: 仅 level=2 蜂窝，连接上限 * 0.7
		query = query.Where("gateways.active_connections < (SELECT max_gateways FROM cells WHERE cells.cell_id = gateways.cell_id) * ?", connRatio).
			Where("gateways.cell_id IN (SELECT cell_id FROM cells WHERE cell_level = 2 AND status = 'active')")
	default: // Standard: 仅 level=1 蜂窝，连接上限 * 1.0
		query = query.Where("gateways.active_connections < (SELECT max_gateways FROM cells WHERE cells.cell_id = gateways.cell_id) * ?", connRatio).
			Where("gateways.cell_id IN (SELECT cell_id FROM cells WHERE cell_level = 1 AND status = 'active')")
	}

	if err := query.Order("active_connections ASC").Limit(10).Find(&gateways).Error; err != nil {
		return nil, err
	}

	routes := make([]GatewayRoute, 0, len(gateways))
	for _, gw := range gateways {
		routes = append(routes, GatewayRoute{
			GatewayID:   gw.GatewayID,
			IPAddress:   gw.IPAddress,
			CellID:      gw.CellID,
			Connections: gw.ActiveConnections,
			Phase:       gw.Phase,
		})
	}

	return routes, nil
}

// selectBest 选择最优节点
func (r *TierRouter) selectBest(routes []GatewayRoute, tier int) *GatewayRoute {
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Connections < routes[j].Connections
	})

	// Diamond: 必须独占（connections == 0）
	if tier == 3 {
		for i := range routes {
			if routes[i].Connections == 0 {
				return &routes[i]
			}
		}
	}

	return &routes[0]
}

// RefreshCache 刷新路由缓存
func (r *TierRouter) RefreshCache() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for tier := 1; tier <= 3; tier++ {
		routes, err := r.queryAvailableGateways(tier)
		if err == nil {
			r.cache[tier] = routes
		}
	}
}

// GetCachedRoutes 获取缓存的路由（快速路径，不查库）
func (r *TierRouter) GetCachedRoutes(tier int) []GatewayRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cache[tier]
}

// ReleaseGateway 释放 Gateway 绑定（用户断开时调用）
func (r *TierRouter) ReleaseGateway(gatewayID, userID string) error {
	return r.db.Model(&models.Gateway{}).
		Where("gateway_id = ? AND user_id = ?", gatewayID, userID).
		Updates(map[string]any{
			"user_id":            "",
			"active_connections": gorm.Expr("GREATEST(active_connections - 1, 0)"),
		}).Error
}
