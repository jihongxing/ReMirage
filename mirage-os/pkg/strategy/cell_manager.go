// Package strategy - 蜂窝调度器
package strategy

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"mirage-os/pkg/models"

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
