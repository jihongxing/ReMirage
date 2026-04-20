// Package raft - 地理围栏与威胁检测（分层架构）
package raft

import (
	"context"
	"log"
	"math"
	"time"
)

// ThreatLevel 威胁等级
type ThreatLevel int

const (
	ThreatLevelNone     ThreatLevel = 0
	ThreatLevelLow      ThreatLevel = 3
	ThreatLevelMedium   ThreatLevel = 5
	ThreatLevelHigh     ThreatLevel = 7
	ThreatLevelCritical ThreatLevel = 9
)

// ThreatIndicator 威胁指标
type ThreatIndicator struct {
	Type      string
	Severity  ThreatLevel
	Timestamp time.Time
	Details   map[string]any
}

// ============================================
// Provider 接口
// ============================================

// NetworkStatsProvider 网络统计数据提供者
type NetworkStatsProvider interface {
	GetConnectionsBySourceIP() (map[string]int, error)
	GetTrafficRateBps() (uint64, error)
	GetSYNRate() (uint64, error)
	GetUDPRate() (uint64, error)
	GetConnectionSources() ([]ConnectionSource, error)
	GetRouteHops() ([]RouteHop, error)
}

// GovernmentIPChecker 政府 IP 段检查器
type GovernmentIPChecker interface {
	IsGovernmentIP(ip string) bool
}

// DataCenterAccessProvider 数据中心访问监控
type DataCenterAccessProvider interface {
	GetUnplannedAccessEvents(since time.Time) ([]AccessEvent, error)
}

// ConnectionSource 连接来源
type ConnectionSource struct {
	IP      string
	Country string
	Count   int
}

// RouteHop 路由跳数
type RouteHop struct {
	Target       string
	CurrentHops  int
	BaselineHops int
}

// AccessEvent 物理访问事件
type AccessEvent struct {
	Timestamp time.Time
	Location  string
	Planned   bool
}

// ============================================
// 分层威胁分析器
// ============================================

// ControlPlaneThreatAnalyzer 控制面威胁分析器（触发 Raft 退位）
type ControlPlaneThreatAnalyzer struct {
	govIPChecker GovernmentIPChecker
	dcAccess     DataCenterAccessProvider
	netStats     NetworkStatsProvider
}

// NewControlPlaneThreatAnalyzer 创建控制面威胁分析器
func NewControlPlaneThreatAnalyzer(gov GovernmentIPChecker, dc DataCenterAccessProvider, net NetworkStatsProvider) *ControlPlaneThreatAnalyzer {
	return &ControlPlaneThreatAnalyzer{govIPChecker: gov, dcAccess: dc, netStats: net}
}

// GatewayThreatAnalyzer Gateway 级威胁分析器（触发蜂窝调度）
type GatewayThreatAnalyzer struct {
	netStats NetworkStatsProvider
}

// NewGatewayThreatAnalyzer 创建 Gateway 级威胁分析器
func NewGatewayThreatAnalyzer(net NetworkStatsProvider) *GatewayThreatAnalyzer {
	return &GatewayThreatAnalyzer{netStats: net}
}

// ============================================
// 纯函数：检测逻辑（用于属性测试）
// ============================================

// DetectGovernmentAudit 检测政府审计（纯函数）
// 当存在政府 IP 连接 OR 非计划物理访问 OR 路由跳数异常(差异>2)时返回 true
func DetectGovernmentAudit(connections map[string]int, govChecker GovernmentIPChecker, hasUnplannedAccess bool, routeHops []RouteHop) bool {
	// 检测政府 IP 段连接
	for ip := range connections {
		if govChecker != nil && govChecker.IsGovernmentIP(ip) {
			return true
		}
	}
	// 检测非计划物理访问
	if hasUnplannedAccess {
		return true
	}
	// 检测路由跳数异常（与基线差异 > 2 跳）
	for _, hop := range routeHops {
		diff := hop.CurrentHops - hop.BaselineHops
		if diff < 0 {
			diff = -diff
		}
		if diff > 2 {
			return true
		}
	}
	return false
}

// DetectDDoS 检测 DDoS 攻击（纯函数）
// current > baseline×5 OR synRate > 10000 OR udpRate > 50000
func DetectDDoS(baseline, current, synRate, udpRate uint64) bool {
	if baseline > 0 && current > baseline*5 {
		return true
	}
	if synRate > 10000 {
		return true
	}
	if udpRate > 50000 {
		return true
	}
	return false
}

// TrafficBaseline 流量基线统计
type TrafficBaseline struct {
	Mean   float64
	StdDev float64
}

// DetectAnomalousTraffic 检测异常流量（纯函数）
// |v - μ| > 3σ OR 单 IP > 1000 连接 OR 非预期地理区域连接超阈值
func DetectAnomalousTraffic(baseline TrafficBaseline, currentTraffic float64, connByIP map[string]int, geoSources []ConnectionSource, expectedCountries map[string]bool, unexpectedThreshold int) bool {
	// 3σ 偏离检测
	if baseline.StdDev > 0 {
		deviation := math.Abs(currentTraffic - baseline.Mean)
		if deviation > 3*baseline.StdDev {
			return true
		}
	}
	// 单 IP 连接数 > 1000
	for _, count := range connByIP {
		if count > 1000 {
			return true
		}
	}
	// 非预期地理区域连接
	unexpectedCount := 0
	for _, src := range geoSources {
		if !expectedCountries[src.Country] {
			unexpectedCount += src.Count
		}
	}
	if unexpectedThreshold > 0 && unexpectedCount > unexpectedThreshold {
		return true
	}
	return false
}

// ShouldStepDown 判断是否应触发退位（纯函数）
func ShouldStepDown(level int, isLeader bool) bool {
	return level >= 8 && isLeader
}

// CalculateOverallThreat 计算综合威胁等级（纯函数）
// 返回最近 windowDuration 内最高 Severity
func CalculateOverallThreat(indicators []ThreatIndicator, now time.Time, windowDuration time.Duration) ThreatLevel {
	maxThreat := ThreatLevelNone
	for _, ind := range indicators {
		if now.Sub(ind.Timestamp) <= windowDuration {
			if ind.Severity > maxThreat {
				maxThreat = ind.Severity
			}
		}
	}
	return maxThreat
}

// ============================================
// GeoFence 主结构
// ============================================

// GeoFence 地理围栏
type GeoFence struct {
	cluster    *Cluster
	cpAnalyzer *ControlPlaneThreatAnalyzer
	gwAnalyzer *GatewayThreatAnalyzer
	indicators []ThreatIndicator
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewGeoFence 创建地理围栏
func NewGeoFence(cluster *Cluster) *GeoFence {
	ctx, cancel := context.WithCancel(context.Background())
	return &GeoFence{
		cluster:    cluster,
		indicators: make([]ThreatIndicator, 0),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// SetAnalyzers 设置分析器（依赖注入）
func (gf *GeoFence) SetAnalyzers(cp *ControlPlaneThreatAnalyzer, gw *GatewayThreatAnalyzer) {
	gf.cpAnalyzer = cp
	gf.gwAnalyzer = gw
}

// Start 启动地理围栏监控
func (gf *GeoFence) Start() {
	log.Println("[GeoFence] 启动地理围栏监控")
	go gf.monitorThreats()
}

// Stop 停止地理围栏监控
func (gf *GeoFence) Stop() {
	log.Println("[GeoFence] 停止地理围栏监控")
	gf.cancel()
}

// monitorThreats 监控威胁
func (gf *GeoFence) monitorThreats() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-gf.ctx.Done():
			return
		case <-ticker.C:
			gf.detectThreats()
		}
	}
}

// detectThreats 检测威胁
func (gf *GeoFence) detectThreats() {
	// ControlPlane 级威胁
	if gf.cpAnalyzer != nil {
		if gf.detectGovernmentAudit() {
			gf.addIndicator(ThreatIndicator{
				Type:      "government_audit",
				Severity:  ThreatLevelCritical,
				Timestamp: time.Now(),
				Details:   map[string]any{"jurisdiction": gf.cluster.config.Jurisdiction},
			})
		}
	}

	// Gateway 级威胁
	if gf.gwAnalyzer != nil {
		if gf.detectDDoS() {
			gf.addIndicator(ThreatIndicator{
				Type:      "ddos_attack",
				Severity:  ThreatLevelHigh,
				Timestamp: time.Now(),
			})
		}
		if gf.detectAnomalousTraffic() {
			gf.addIndicator(ThreatIndicator{
				Type:      "anomalous_traffic",
				Severity:  ThreatLevelMedium,
				Timestamp: time.Now(),
			})
		}
	}

	// 仅 ControlPlane 级威胁影响 Raft 集群
	overallThreat := gf.calculateOverallThreat()
	gf.cluster.SetThreatLevel(int(overallThreat))
}

// detectGovernmentAudit 检测政府审计
func (gf *GeoFence) detectGovernmentAudit() bool {
	if gf.cpAnalyzer == nil {
		return false
	}
	connections, err := gf.cpAnalyzer.netStats.GetConnectionsBySourceIP()
	if err != nil {
		return false
	}
	hops, _ := gf.cpAnalyzer.netStats.GetRouteHops()
	hasUnplanned := false
	if gf.cpAnalyzer.dcAccess != nil {
		events, err := gf.cpAnalyzer.dcAccess.GetUnplannedAccessEvents(time.Now().Add(-1 * time.Hour))
		if err == nil {
			for _, e := range events {
				if !e.Planned {
					hasUnplanned = true
					break
				}
			}
		}
	}
	return DetectGovernmentAudit(connections, gf.cpAnalyzer.govIPChecker, hasUnplanned, hops)
}

// detectDDoS 检测 DDoS 攻击
func (gf *GeoFence) detectDDoS() bool {
	if gf.gwAnalyzer == nil {
		return false
	}
	baseline, err := gf.gwAnalyzer.netStats.GetTrafficRateBps()
	if err != nil {
		return false
	}
	current := baseline // 实际应从实时统计获取
	synRate, _ := gf.gwAnalyzer.netStats.GetSYNRate()
	udpRate, _ := gf.gwAnalyzer.netStats.GetUDPRate()
	return DetectDDoS(baseline, current, synRate, udpRate)
}

// detectAnomalousTraffic 检测异常流量
func (gf *GeoFence) detectAnomalousTraffic() bool {
	if gf.gwAnalyzer == nil {
		return false
	}
	connByIP, err := gf.gwAnalyzer.netStats.GetConnectionsBySourceIP()
	if err != nil {
		return false
	}
	for _, count := range connByIP {
		if count > 1000 {
			return true
		}
	}
	return false
}

// addIndicator 添加威胁指标
func (gf *GeoFence) addIndicator(indicator ThreatIndicator) {
	gf.indicators = append(gf.indicators, indicator)
	if len(gf.indicators) > 100 {
		gf.indicators = gf.indicators[1:]
	}
	log.Printf("[GeoFence] 检测到威胁: %s (严重程度: %d)", indicator.Type, indicator.Severity)
}

// calculateOverallThreat 计算总体威胁等级
func (gf *GeoFence) calculateOverallThreat() ThreatLevel {
	return CalculateOverallThreat(gf.indicators, time.Now(), 5*time.Minute)
}

// GetThreatIndicators 获取威胁指标
func (gf *GeoFence) GetThreatIndicators() []ThreatIndicator {
	return gf.indicators
}
