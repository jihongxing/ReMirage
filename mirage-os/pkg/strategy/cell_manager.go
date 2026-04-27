// Package strategy - 蜂窝调度器
package strategy

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"mirage-os/pkg/models"
	"mirage-os/pkg/redact"

	"gorm.io/gorm"
)

// CellPhase 蜂窝生命周期阶段
type CellPhase int

const (
	PhaseIncubation  CellPhase = 0 // 潜伏期：VPC 噪声注入
	PhaseCalibration CellPhase = 1 // 校准期：网络质量测量
	PhaseActive      CellPhase = 2 // 服役期：承载真实流量
)

// 配置常量
const (
	IncubationDuration  = 1 * time.Hour
	CalibrationSamples  = 100
	ScaleOutThreshold   = 0.8
	ThreatLevelCritical = 3
)

// DownlinkClient Gateway 下行通信接口
type DownlinkClient interface {
	PushStrategy(gatewayID string, strategy any) error
	PushQuota(gatewayID string, remainingBytes uint64) error
}

// CellScheduler 蜂窝调度器
type CellScheduler struct {
	db             *gorm.DB
	mu             sync.RWMutex
	activeCells    map[string]*models.Cell
	shadowPool     []*models.Gateway
	downlinkClient DownlinkClient
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewCellScheduler 创建调度器
func NewCellScheduler(db *gorm.DB, dlClient DownlinkClient) *CellScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &CellScheduler{
		db:             db,
		activeCells:    make(map[string]*models.Cell),
		shadowPool:     make([]*models.Gateway, 0),
		downlinkClient: dlClient,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start 启动调度器
func (s *CellScheduler) Start() error {
	log.Println("[CellScheduler] 启动蜂窝调度器")
	if err := s.loadActiveCells(); err != nil {
		return fmt.Errorf("加载蜂窝失败: %w", err)
	}
	if err := s.loadShadowPool(); err != nil {
		return fmt.Errorf("加载影子池失败: %w", err)
	}
	go s.lifecycleManager()
	go s.autoScaler()
	return nil
}

// Stop 停止调度器
func (s *CellScheduler) Stop() {
	log.Println("[CellScheduler] 停止调度器")
	s.cancel()
}

// loadActiveCells 加载活跃蜂窝
func (s *CellScheduler) loadActiveCells() error {
	var cells []models.Cell
	if err := s.db.Where("status = ?", "active").Find(&cells).Error; err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range cells {
		s.activeCells[cells[i].CellID] = &cells[i]
	}
	log.Printf("[CellScheduler] 加载 %d 个活跃蜂窝", len(cells))
	return nil
}

// loadShadowPool 加载影子池
func (s *CellScheduler) loadShadowPool() error {
	var gateways []models.Gateway
	if err := s.db.Where("phase IN (?, ?) AND is_online = ?",
		PhaseIncubation, PhaseCalibration, true).Find(&gateways).Error; err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shadowPool = make([]*models.Gateway, len(gateways))
	for i := range gateways {
		s.shadowPool[i] = &gateways[i]
	}
	log.Printf("[CellScheduler] 加载 %d 个影子节点", len(gateways))
	return nil
}

// lifecycleManager 生命周期管理器
func (s *CellScheduler) lifecycleManager() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processLifecycle()
		}
	}
}

// processLifecycle 处理生命周期转换
func (s *CellScheduler) processLifecycle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, gw := range s.shadowPool {
		if gw.Phase == int(PhaseIncubation) && gw.IncubationStartedAt != nil {
			if now.Sub(*gw.IncubationStartedAt) >= IncubationDuration {
				if err := s.promoteToCalibration(gw); err != nil {
					log.Printf("[CellScheduler] 晋升到校准期失败 %s: %v", gw.GatewayID, err)
				}
			}
		}
		if gw.Phase == int(PhaseCalibration) && gw.NetworkQuality >= 70.0 {
			log.Printf("[CellScheduler] 节点 %s 校准完成，质量: %.2f", gw.GatewayID, gw.NetworkQuality)
		}
	}
}

// promoteToCalibration 晋升到校准期
func (s *CellScheduler) promoteToCalibration(gw *models.Gateway) error {
	log.Printf("[CellScheduler] 节点 %s 进入校准期", gw.GatewayID)
	gw.Phase = int(PhaseCalibration)
	if err := s.db.Model(gw).Updates(map[string]any{"phase": PhaseCalibration}).Error; err != nil {
		return err
	}

	// 通过 DownlinkClient 推送校准参数
	if s.downlinkClient != nil {
		calibrationParams := map[string]any{
			"type":            "calibration",
			"probe_targets":   []string{"1.1.1.1", "8.8.8.8"},
			"sample_count":    CalibrationSamples,
			"sample_interval": "1s",
		}
		if err := s.downlinkClient.PushStrategy(gw.GatewayID, calibrationParams); err != nil {
			log.Printf("[CellScheduler] ⚠️ 推送校准参数失败 %s: %v（Desired State 模型将在心跳时重试）", gw.GatewayID, err)
		}
	}
	return nil
}

// PromoteToActive 晋升到服役期
func (s *CellScheduler) PromoteToActive(gatewayID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var gw models.Gateway
	if err := s.db.Where("gateway_id = ?", gatewayID).First(&gw).Error; err != nil {
		return fmt.Errorf("节点不存在: %w", err)
	}
	if gw.Phase != int(PhaseCalibration) {
		return fmt.Errorf("节点未处于校准期，当前阶段: %d", gw.Phase)
	}

	log.Printf("[CellScheduler] 节点 %s 晋升为服役状态", gatewayID)
	gw.Phase = int(PhaseActive)
	if err := s.db.Model(&gw).Updates(map[string]any{"phase": PhaseActive}).Error; err != nil {
		return err
	}

	// 通过 DownlinkClient 下发 B-DNA 模板
	if s.downlinkClient != nil {
		templatePush := map[string]any{
			"type":        "bdna_template",
			"template_id": "default-v1",
		}
		if err := s.downlinkClient.PushStrategy(gatewayID, templatePush); err != nil {
			log.Printf("[CellScheduler] ⚠️ 推送 B-DNA 模板失败 %s: %v（Desired State 模型将在心跳时重试）", gatewayID, err)
		}
	}
	return nil
}

// RegisterGateway 注册新网关（进入潜伏期）
func (s *CellScheduler) RegisterGateway(gatewayID, cellID, ipAddress string) error {
	now := time.Now()
	gw := models.Gateway{
		GatewayID:           gatewayID,
		CellID:              cellID,
		IPAddress:           ipAddress,
		Phase:               int(PhaseIncubation),
		IncubationStartedAt: &now,
		IsOnline:            true,
		LastHeartbeatAt:     &now,
	}
	if err := s.db.Create(&gw).Error; err != nil {
		return fmt.Errorf("注册网关失败: %w", err)
	}

	log.Printf("[CellScheduler] 新网关 %s 进入潜伏期", gatewayID)

	s.mu.Lock()
	s.shadowPool = append(s.shadowPool, &gw)
	s.mu.Unlock()

	// 通过 DownlinkClient 推送 VPC 噪声注入启动指令
	if s.downlinkClient != nil {
		noiseParams := map[string]any{
			"type":            "vpc_noise",
			"noise_intensity": 80,
		}
		if err := s.downlinkClient.PushStrategy(gatewayID, noiseParams); err != nil {
			log.Printf("[CellScheduler] ⚠️ 推送噪声注入失败 %s: %v（Desired State 模型将在心跳时重试）", gatewayID, err)
		}
	}
	return nil
}

// autoScaler 自动伸缩器
func (s *CellScheduler) autoScaler() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkScaleOut()
		}
	}
}

// checkScaleOut 检查是否需要扩容
func (s *CellScheduler) checkScaleOut() {
	var activeGateways []models.Gateway
	if err := s.db.Where("phase = ? AND is_online = ?", PhaseActive, true).
		Find(&activeGateways).Error; err != nil {
		log.Printf("[CellScheduler] 查询活跃节点失败: %v", err)
		return
	}

	// 收集所有活跃 Cell，确保温备池充足
	cellsSeen := make(map[string]bool)
	for _, gw := range activeGateways {
		if !cellsSeen[gw.CellID] {
			cellsSeen[gw.CellID] = true
			go s.EnsureStandbyPool(s.ctx, gw.CellID)
		}
	}

	for _, gw := range activeGateways {
		if gw.CurrentThreatLevel >= ThreatLevelCritical {
			log.Printf("[CellScheduler] 检测到高威胁节点 %s (等级: %d)，触发扩容",
				gw.GatewayID, gw.CurrentThreatLevel)
			s.scaleOutForRegion(gw.CellID)
			return
		}
	}
	for _, gw := range activeGateways {
		maxConnections := 10000
		loadRatio := float64(gw.ActiveConnections) / float64(maxConnections)
		if loadRatio >= ScaleOutThreshold {
			log.Printf("[CellScheduler] 节点 %s 负载过高 (%.2f%%)，触发扩容",
				gw.GatewayID, loadRatio*100)
			s.scaleOutForRegion(gw.CellID)
			return
		}
	}
}

// scaleOutForRegion 为指定区域扩容
func (s *CellScheduler) scaleOutForRegion(cellID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bestGateway *models.Gateway
	var bestQuality float64
	for _, gw := range s.shadowPool {
		if gw.CellID == cellID && gw.Phase == int(PhaseCalibration) {
			if gw.NetworkQuality > bestQuality {
				bestQuality = gw.NetworkQuality
				bestGateway = gw
			}
		}
	}
	if bestGateway == nil {
		log.Printf("[CellScheduler] 区域 %s 无可用影子节点", cellID)
		return fmt.Errorf("无可用影子节点")
	}
	log.Printf("[CellScheduler] 选中节点 %s (质量: %.2f) 进行扩容",
		bestGateway.GatewayID, bestQuality)
	return s.PromoteToActive(bestGateway.GatewayID)
}

// UpdateNetworkQuality 更新网络质量
func (s *CellScheduler) UpdateNetworkQuality(gatewayID string, rtt int, packetLoss float64) error {
	rttScore := float64(rtt) / 1000.0
	if rttScore > 1.0 {
		rttScore = 1.0
	}
	quality := 100.0 - (50.0 * rttScore) - (50.0 * packetLoss)
	if quality < 0 {
		quality = 0
	}
	if err := s.db.Model(&models.Gateway{}).
		Where("gateway_id = ?", gatewayID).
		Updates(map[string]any{
			"baseline_rtt":         rtt,
			"baseline_packet_loss": packetLoss,
			"network_quality":      quality,
		}).Error; err != nil {
		return err
	}
	log.Printf("[CellScheduler] 更新节点 %s 网络质量: %.2f (RTT: %dμs, 丢包: %.2f%%)",
		gatewayID, quality, rtt, packetLoss*100)
	return nil
}

// GetShadowPoolStatus 获取影子池状态
func (s *CellScheduler) GetShadowPoolStatus() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	incubationCount := 0
	calibrationCount := 0
	for _, gw := range s.shadowPool {
		switch gw.Phase {
		case int(PhaseIncubation):
			incubationCount++
		case int(PhaseCalibration):
			calibrationCount++
		}
	}
	return map[string]any{
		"total":       len(s.shadowPool),
		"incubation":  incubationCount,
		"calibration": calibrationCount,
	}
}

// ============================================
// 恢复优先级差异（Task 6）
// ============================================

// GatewaySessionWithLevel 带用户等级的会话信息
type GatewaySessionWithLevel struct {
	SessionID string
	UserID    string
	GatewayID string
	CellLevel int
}

// SortSessionsByPriority 按等级优先级排序（纯函数，用于属性测试）
// Diamond(3) 优先 → Platinum(2) → Standard(1)
func SortSessionsByPriority(sessions []GatewaySessionWithLevel) {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CellLevel > sessions[j].CellLevel
	})
}

// getTierConnectionRatioLocal 本地等级连接上限比例（避免循环导入 provisioning 包）
func getTierConnectionRatioLocal(level int) float32 {
	switch level {
	case 3:
		return 0.4
	case 2:
		return 0.7
	default:
		return 1.0
	}
}

// RecoverUsers 故障恢复时按等级优先级排序迁移
func (s *CellScheduler) RecoverUsers(failedGatewayID string) error {
	// 查询该 Gateway 上所有活跃用户及其等级
	var sessions []GatewaySessionWithLevel
	s.db.Raw(`
		SELECT g.gateway_id, g.user_id, u.cell_level
		FROM gateways g
		JOIN users u ON g.user_id = u.user_id
		WHERE g.gateway_id = ? AND g.is_online = true AND g.user_id != ''
	`, failedGatewayID).Scan(&sessions)

	if len(sessions) == 0 {
		return nil
	}

	// 按等级降序排序：Diamond(3) > Platinum(2) > Standard(1)
	SortSessionsByPriority(sessions)

	for _, session := range sessions {
		target := s.selectRecoveryTarget(session.CellLevel)
		if target != nil {
			s.migrateSession(session.UserID, session.GatewayID, target.GatewayID)
		} else {
			log.Printf("[CellScheduler] 无法为用户 %s (等级 %d) 找到恢复目标", redact.Token(session.UserID), session.CellLevel)
		}
	}
	return nil
}

// selectRecoveryTarget 选择恢复目标 Gateway
// Diamond 用户优先选择网络质量最高的备选 Gateway
func (s *CellScheduler) selectRecoveryTarget(userLevel int) *models.Gateway {
	var candidates []models.Gateway

	// 查询同等级或更低等级的可用 Gateway
	for level := userLevel; level >= 1; level-- {
		s.db.Where(`
			is_online = true AND phase = 2 AND active_connections <
			(SELECT max_gateways FROM cells WHERE cells.cell_id = gateways.cell_id) * ? AND
			cell_id IN (SELECT cell_id FROM cells WHERE cell_level = ? AND status = 'active')
		`, getTierConnectionRatioLocal(level), level).
			Find(&candidates)
		if len(candidates) > 0 {
			break
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Diamond 用户优先选择网络质量最高的备选 Gateway
	if userLevel == 3 {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].NetworkQuality > candidates[j].NetworkQuality
		})
	} else {
		// 其他等级按连接数最少排序
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].ActiveConnections < candidates[j].ActiveConnections
		})
	}

	return &candidates[0]
}

// migrateSession 迁移用户会话
func (s *CellScheduler) migrateSession(userID, fromGatewayID, toGatewayID string) {
	// 释放旧 Gateway
	s.db.Model(&models.Gateway{}).Where("gateway_id = ?", fromGatewayID).
		Updates(map[string]any{
			"user_id":            "",
			"active_connections": gorm.Expr("GREATEST(active_connections - 1, 0)"),
		})

	// 绑定新 Gateway
	s.db.Model(&models.Gateway{}).Where("gateway_id = ?", toGatewayID).
		Updates(map[string]any{
			"user_id":            userID,
			"active_connections": gorm.Expr("active_connections + 1"),
		})

	log.Printf("[CellScheduler] 迁移用户 %s: %s → %s", redact.Token(userID), fromGatewayID, toGatewayID)
}

// ActivateStandby 从温备池激活替补节点
// 优先 Phase=1（温备），其次 Phase=0（冷备），激活后 Phase 更新为 2
func (s *CellScheduler) ActivateStandby(ctx context.Context, cellID string) (*models.Gateway, error) {
	// 优先从 Phase=1（温备/校准期）中选择
	var standby models.Gateway
	err := s.db.Where("cell_id = ? AND phase = 1 AND is_online = true", cellID).
		Order("baseline_rtt ASC").First(&standby).Error
	if err != nil {
		// 温备池耗尽，从 Phase=0（冷备/潜伏期）中选择
		err = s.db.Where("cell_id = ? AND phase = 0 AND is_online = true", cellID).
			Order("baseline_rtt ASC").First(&standby).Error
		if err != nil {
			return nil, fmt.Errorf("no standby available in cell %s", cellID)
		}
	}

	// 激活：Phase → 2（服役期）
	if err := s.db.Model(&standby).Update("phase", int(PhaseActive)).Error; err != nil {
		return nil, fmt.Errorf("activate standby %s: %w", standby.GatewayID, err)
	}

	log.Printf("[CellScheduler] ✅ 替补节点已激活: %s (cell=%s, phase→2)", standby.GatewayID, cellID)

	// 补充温备池
	go s.EnsureStandbyPool(ctx, cellID)

	return &standby, nil
}

// EnsureStandbyPool 确保温备池节点数不低于阈值（默认 2）
func (s *CellScheduler) EnsureStandbyPool(ctx context.Context, cellID string) {
	const minWarmStandby = 2

	var warmCount int64
	s.db.Model(&models.Gateway{}).
		Where("cell_id = ? AND phase = 1 AND is_online = true", cellID).
		Count(&warmCount)

	if warmCount >= int64(minWarmStandby) {
		return
	}

	needed := int64(minWarmStandby) - warmCount

	// 从冷备池（Phase=0）补充到温备池（Phase=1）
	var coldGateways []models.Gateway
	s.db.Where("cell_id = ? AND phase = 0 AND is_online = true", cellID).
		Order("baseline_rtt ASC").
		Limit(int(needed)).
		Find(&coldGateways)

	for _, gw := range coldGateways {
		if err := s.db.Model(&gw).Update("phase", int(PhaseCalibration)).Error; err != nil {
			log.Printf("[CellScheduler] ⚠️ 补充温备池失败: %s: %v", gw.GatewayID, err)
			continue
		}
		log.Printf("[CellScheduler] 温备池补充: %s (cell=%s, phase 0→1)", gw.GatewayID, cellID)
	}
}

// PoolStats 各级池的节点统计
type PoolStats struct {
	Active int64 `json:"active"` // Phase=2
	Warm   int64 `json:"warm"`   // Phase=1
	Cold   int64 `json:"cold"`   // Phase=0
}

// GetPoolStats 返回指定 Cell 各级池的节点数
func (s *CellScheduler) GetPoolStats(cellID string) PoolStats {
	var stats PoolStats
	s.db.Model(&models.Gateway{}).
		Where("cell_id = ? AND phase = 2 AND is_online = true", cellID).
		Count(&stats.Active)
	s.db.Model(&models.Gateway{}).
		Where("cell_id = ? AND phase = 1 AND is_online = true", cellID).
		Count(&stats.Warm)
	s.db.Model(&models.Gateway{}).
		Where("cell_id = ? AND phase = 0 AND is_online = true", cellID).
		Count(&stats.Cold)
	return stats
}

// GetAllPoolStats 返回所有 Cell 的池统计（用于 Prometheus metrics 暴露）
func (s *CellScheduler) GetAllPoolStats() map[string]PoolStats {
	s.mu.RLock()
	cellIDs := make([]string, 0, len(s.activeCells))
	for id := range s.activeCells {
		cellIDs = append(cellIDs, id)
	}
	s.mu.RUnlock()

	result := make(map[string]PoolStats, len(cellIDs))
	for _, cellID := range cellIDs {
		result[cellID] = s.GetPoolStats(cellID)
	}
	return result
}
