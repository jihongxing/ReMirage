// Package gtunnel - 转生协议（路径自动切换）
package gtunnel

import (
	"log"
	"time"
)

// ReincarnationManager 转生管理器
type ReincarnationManager struct {
	tunnel         *Tunnel
	healthChecker  *HealthChecker
	switchInterval time.Duration
}

// HealthChecker 健康检查器
type HealthChecker struct {
	paths      map[string]*PathHealth
	checkInterval time.Duration
}

// PathHealth 路径健康状态
type PathHealth struct {
	PathID       string
	IsHealthy    bool
	LastCheck    time.Time
	FailCount    int
	SuccessCount int
}

// NewReincarnationManager 创建转生管理器
func NewReincarnationManager(tunnel *Tunnel) *ReincarnationManager {
	return &ReincarnationManager{
		tunnel:         tunnel,
		healthChecker:  NewHealthChecker(),
		switchInterval: 5 * time.Second,
	}
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		paths:         make(map[string]*PathHealth),
		checkInterval: 3 * time.Second,
	}
}

// Start 启动转生管理器
func (rm *ReincarnationManager) Start() {
	go rm.monitorHealth()
	log.Println("🔄 [转生协议] 已启动")
}

// monitorHealth 监控路径健康
func (rm *ReincarnationManager) monitorHealth() {
	ticker := time.NewTicker(rm.healthChecker.checkInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		paths := rm.tunnel.scheduler.GetAllPaths()
		
		for _, path := range paths {
			health := rm.healthChecker.CheckPath(path)
			
			if !health.IsHealthy {
				log.Printf("⚠️  [转生协议] 路径 %s 不健康，准备切换", path.ID)
				rm.switchToHealthyPath(path.ID)
			}
		}
	}
}

// CheckPath 检查路径健康
func (hc *HealthChecker) CheckPath(path *Path) *PathHealth {
	health, exists := hc.paths[path.ID]
	if !exists {
		health = &PathHealth{
			PathID:    path.ID,
			IsHealthy: true,
		}
		hc.paths[path.ID] = health
	}
	
	// 检查丢包率
	if path.LossRate > 0.5 {
		health.FailCount++
		health.SuccessCount = 0
	} else {
		health.SuccessCount++
		health.FailCount = 0
	}
	
	// 连续失败 3 次标记为不健康
	if health.FailCount >= 3 {
		health.IsHealthy = false
	}
	
	// 连续成功 3 次恢复健康
	if health.SuccessCount >= 3 {
		health.IsHealthy = true
	}
	
	health.LastCheck = time.Now()
	
	return health
}

// switchToHealthyPath 切换到健康路径
func (rm *ReincarnationManager) switchToHealthyPath(failedPathID string) {
	paths := rm.tunnel.scheduler.GetAllPaths()
	
	for _, path := range paths {
		if path.ID == failedPathID {
			continue
		}
		
		health := rm.healthChecker.paths[path.ID]
		if health != nil && health.IsHealthy {
			err := rm.tunnel.scheduler.SwitchPath(path.ID)
			if err == nil {
				log.Printf("✅ [转生协议] 已切换到路径: %s", path.ID)
				return
			}
		}
	}
	
	log.Printf("❌ [转生协议] 无可用健康路径")
}
