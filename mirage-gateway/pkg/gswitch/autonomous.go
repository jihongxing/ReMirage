// Package gswitch - 自适应逃逸决策引擎
// 基于信誉分系统实现静默切换，无需知道具体封锁原因
package gswitch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cilium/ebpf"
)

// SwitchReason 切换原因
type SwitchReason int

const (
	ReasonNone           SwitchReason = 0
	ReasonLowReputation  SwitchReason = 1 // 信誉度下降
	ReasonHTTPError      SwitchReason = 2 // HTTP 错误集中爆发
	ReasonRSTFlood       SwitchReason = 3 // RST 洪水
	ReasonJA4Fingerprint SwitchReason = 4 // JA4 指纹暴露
	ReasonManual         SwitchReason = 5 // 手动触发
	ReasonScheduled      SwitchReason = 6 // 定时轮换
)

// ShadowDomain 影子域名
type ShadowDomain struct {
	Name        string    `json:"name"`
	IP          string    `json:"ip"`
	CreatedAt   time.Time `json:"created_at"`
	WarmupAt    *time.Time `json:"warmup_at"`
	IsWarmedUp  bool      `json:"is_warmed_up"`
	UsageCount  int       `json:"usage_count"`
}

// SwitchEvent 切换事件
type SwitchEvent struct {
	OldDomain   string       `json:"old_domain"`
	NewDomain   string       `json:"new_domain"`
	Reason      SwitchReason `json:"reason"`
	ReputationScore float64  `json:"reputation_score"`
	Timestamp   time.Time    `json:"timestamp"`
}

// ErrorDistribution 错误分布
type ErrorDistribution struct {
	HTTP403Count  int       `json:"http_403_count"`
	HTTP451Count  int       `json:"http_451_count"`
	HTTP503Count  int       `json:"http_503_count"`
	RSTCount      int       `json:"rst_count"`
	TimeoutCount  int       `json:"timeout_count"`
	WindowStart   time.Time `json:"window_start"`
}

// AutonomousGSwitch 自适应逃逸引擎
type AutonomousGSwitch struct {
	mu sync.RWMutex

	// 当前活跃域名
	activeDomain string

	// 影子域名池（预热状态）
	shadowPool []*ShadowDomain

	// 已废弃域名
	burnedDomains []string

	// 错误分布统计
	errorDist *ErrorDistribution

	// 切换历史
	switchHistory []*SwitchEvent

	// eBPF Map 引用
	sniMap     *ebpf.Map
	ja4Map     *ebpf.Map

	// 回调
	onSwitch func(event *SwitchEvent)

	// 配置
	reputationThreshold float64       // 信誉阈值
	errorBurstThreshold int           // 错误爆发阈值
	warmupInterval      time.Duration // 预热间隔
	rotationInterval    time.Duration // 定时轮换间隔
	shadowPoolSize      int           // 影子池大小

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAutonomousGSwitch 创建自适应逃逸引擎
func NewAutonomousGSwitch(sniMap, ja4Map *ebpf.Map) *AutonomousGSwitch {
	ctx, cancel := context.WithCancel(context.Background())

	return &AutonomousGSwitch{
		shadowPool:          make([]*ShadowDomain, 0),
		burnedDomains:       make([]string, 0),
		errorDist:           &ErrorDistribution{WindowStart: time.Now()},
		switchHistory:       make([]*SwitchEvent, 0),
		sniMap:              sniMap,
		ja4Map:              ja4Map,
		reputationThreshold: 40.0,
		errorBurstThreshold: 10,
		warmupInterval:      5 * time.Minute,
		rotationInterval:    24 * time.Hour,
		shadowPoolSize:      3,
		ctx:                 ctx,
		cancel:              cancel,
	}
}

// SetSwitchCallback 设置切换回调
func (ag *AutonomousGSwitch) SetSwitchCallback(callback func(*SwitchEvent)) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.onSwitch = callback
}

// Start 启动引擎
func (ag *AutonomousGSwitch) Start(initialDomain string) error {
	ag.mu.Lock()
	ag.activeDomain = initialDomain
	ag.mu.Unlock()

	// 启动影子域名预热
	ag.wg.Add(1)
	go ag.warmupLoop()

	// 启动错误监控
	ag.wg.Add(1)
	go ag.errorMonitorLoop()

	// 启动定时轮换
	ag.wg.Add(1)
	go ag.scheduledRotationLoop()

	log.Printf("🤖 自适应逃逸引擎已启动 (active: %s)", initialDomain)
	return nil
}

// Stop 停止引擎
func (ag *AutonomousGSwitch) Stop() {
	ag.cancel()
	ag.wg.Wait()
	log.Println("🛑 自适应逃逸引擎已停止")
}

// AddShadowDomain 添加影子域名
func (ag *AutonomousGSwitch) AddShadowDomain(name, ip string) {
	ag.mu.Lock()
	defer ag.mu.Unlock()

	shadow := &ShadowDomain{
		Name:      name,
		IP:        ip,
		CreatedAt: time.Now(),
	}

	ag.shadowPool = append(ag.shadowPool, shadow)
	log.Printf("👻 影子域名已添加: %s", name)
}

// ReportError 上报错误
func (ag *AutonomousGSwitch) ReportError(errorType string) {
	ag.mu.Lock()
	defer ag.mu.Unlock()

	// 重置窗口（每 5 分钟）
	if time.Since(ag.errorDist.WindowStart) > 5*time.Minute {
		ag.errorDist = &ErrorDistribution{WindowStart: time.Now()}
	}

	switch errorType {
	case "http_403":
		ag.errorDist.HTTP403Count++
	case "http_451":
		ag.errorDist.HTTP451Count++
	case "http_503":
		ag.errorDist.HTTP503Count++
	case "rst":
		ag.errorDist.RSTCount++
	case "timeout":
		ag.errorDist.TimeoutCount++
	}
}

// CheckAndSwitch 检查并切换（基于信誉分）
func (ag *AutonomousGSwitch) CheckAndSwitch(domain string, reputationScore float64) bool {
	ag.mu.Lock()

	if domain != ag.activeDomain {
		ag.mu.Unlock()
		return false
	}

	if reputationScore >= ag.reputationThreshold {
		ag.mu.Unlock()
		return false
	}

	ag.mu.Unlock()

	// 触发切换
	return ag.executeSwitch(ReasonLowReputation, reputationScore)
}

// executeSwitch 执行切换
func (ag *AutonomousGSwitch) executeSwitch(reason SwitchReason, score float64) bool {
	ag.mu.Lock()

	// 获取预热完成的影子域名
	var newDomain *ShadowDomain
	for i, shadow := range ag.shadowPool {
		if shadow.IsWarmedUp {
			newDomain = shadow
			ag.shadowPool = append(ag.shadowPool[:i], ag.shadowPool[i+1:]...)
			break
		}
	}

	if newDomain == nil {
		// 没有预热域名，紧急生成
		newDomain = ag.generateEmergencyDomain()
	}

	oldDomain := ag.activeDomain

	// 记录废弃域名
	ag.burnedDomains = append(ag.burnedDomains, oldDomain)

	// 切换活跃域名
	ag.activeDomain = newDomain.Name
	newDomain.UsageCount++

	// 记录事件
	event := &SwitchEvent{
		OldDomain:       oldDomain,
		NewDomain:       newDomain.Name,
		Reason:          reason,
		ReputationScore: score,
		Timestamp:       time.Now(),
	}
	ag.switchHistory = append(ag.switchHistory, event)

	callback := ag.onSwitch
	ag.mu.Unlock()

	// 更新 eBPF SNI Map
	if err := ag.updateSNIMap(newDomain.Name); err != nil {
		log.Printf("⚠️  更新 SNI Map 失败: %v", err)
	}

	// 如果是 JA4 指纹暴露，同时重置 JA4
	if reason == ReasonJA4Fingerprint {
		ag.resetJA4Template()
	}

	log.Printf("🔄 域名切换完成: %s → %s (reason=%d, score=%.1f)",
		oldDomain, newDomain.Name, reason, score)

	// 触发回调
	if callback != nil {
		go callback(event)
	}

	return true
}

// updateSNIMap 更新 SNI Map
func (ag *AutonomousGSwitch) updateSNIMap(domain string) error {
	if ag.sniMap == nil {
		return nil
	}

	type SNIEntry struct {
		SNI       [64]byte
		Timestamp uint64
		Active    uint32
	}

	var entry SNIEntry
	copy(entry.SNI[:], domain)
	entry.Timestamp = uint64(time.Now().UnixNano())
	entry.Active = 1

	key := uint32(0)
	return ag.sniMap.Put(&key, &entry)
}

// resetJA4Template 重置 JA4 模板
func (ag *AutonomousGSwitch) resetJA4Template() {
	if ag.ja4Map == nil {
		return
	}

	// 随机选择新的 JA4 模板
	templates := []uint32{0, 1, 2, 3, 4} // 预设模板 ID
	randBytes := make([]byte, 1)
	rand.Read(randBytes)
	templateID := templates[int(randBytes[0])%len(templates)]

	key := uint32(0)
	if err := ag.ja4Map.Put(&key, &templateID); err != nil {
		log.Printf("⚠️  重置 JA4 模板失败: %v", err)
	} else {
		log.Printf("🎭 JA4 模板已重置: template=%d", templateID)
	}
}

// generateEmergencyDomain 紧急生成域名
func (ag *AutonomousGSwitch) generateEmergencyDomain() *ShadowDomain {
	randBytes := make([]byte, 8)
	rand.Read(randBytes)
	subdomain := hex.EncodeToString(randBytes)

	return &ShadowDomain{
		Name:       fmt.Sprintf("%s.emergency.cdn.example.com", subdomain),
		IP:         "0.0.0.0",
		CreatedAt:  time.Now(),
		IsWarmedUp: true, // 紧急域名直接标记为预热完成
	}
}

// warmupLoop 预热循环
func (ag *AutonomousGSwitch) warmupLoop() {
	defer ag.wg.Done()

	ticker := time.NewTicker(ag.warmupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ag.ctx.Done():
			return
		case <-ticker.C:
			ag.warmupShadowDomains()
		}
	}
}

// warmupShadowDomains 预热影子域名
func (ag *AutonomousGSwitch) warmupShadowDomains() {
	ag.mu.Lock()
	defer ag.mu.Unlock()

	// 补充影子池
	deficit := ag.shadowPoolSize - len(ag.shadowPool)
	for i := 0; i < deficit; i++ {
		shadow := ag.generateEmergencyDomain()
		shadow.IsWarmedUp = false
		ag.shadowPool = append(ag.shadowPool, shadow)
	}

	// 预热未预热的域名
	for _, shadow := range ag.shadowPool {
		if !shadow.IsWarmedUp {
			// 模拟预热（实际应发送测试请求）
			now := time.Now()
			shadow.WarmupAt = &now
			shadow.IsWarmedUp = true
			log.Printf("🔥 影子域名预热完成: %s", shadow.Name)
		}
	}
}

// errorMonitorLoop 错误监控循环
func (ag *AutonomousGSwitch) errorMonitorLoop() {
	defer ag.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ag.ctx.Done():
			return
		case <-ticker.C:
			ag.checkErrorBurst()
		}
	}
}

// checkErrorBurst 检查错误爆发
func (ag *AutonomousGSwitch) checkErrorBurst() {
	ag.mu.RLock()
	dist := ag.errorDist
	threshold := ag.errorBurstThreshold
	ag.mu.RUnlock()

	// HTTP 403/451 集中爆发 → 域名被封
	if dist.HTTP403Count+dist.HTTP451Count > threshold {
		ag.executeSwitch(ReasonHTTPError, 0)
		return
	}

	// RST 洪水 → 可能是 JA4 指纹暴露
	if dist.RSTCount > threshold*2 {
		ag.executeSwitch(ReasonJA4Fingerprint, 0)
		return
	}
}

// scheduledRotationLoop 定时轮换循环
func (ag *AutonomousGSwitch) scheduledRotationLoop() {
	defer ag.wg.Done()

	ticker := time.NewTicker(ag.rotationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ag.ctx.Done():
			return
		case <-ticker.C:
			ag.executeSwitch(ReasonScheduled, 100)
		}
	}
}

// ForceSwitch 强制切换
func (ag *AutonomousGSwitch) ForceSwitch(reason SwitchReason) bool {
	return ag.executeSwitch(reason, 0)
}

// GetActiveDomain 获取当前活跃域名
func (ag *AutonomousGSwitch) GetActiveDomain() string {
	ag.mu.RLock()
	defer ag.mu.RUnlock()
	return ag.activeDomain
}

// GetShadowPoolStatus 获取影子池状态
func (ag *AutonomousGSwitch) GetShadowPoolStatus() []*ShadowDomain {
	ag.mu.RLock()
	defer ag.mu.RUnlock()

	result := make([]*ShadowDomain, len(ag.shadowPool))
	copy(result, ag.shadowPool)
	return result
}

// GetSwitchHistory 获取切换历史
func (ag *AutonomousGSwitch) GetSwitchHistory() []*SwitchEvent {
	ag.mu.RLock()
	defer ag.mu.RUnlock()

	result := make([]*SwitchEvent, len(ag.switchHistory))
	copy(result, ag.switchHistory)
	return result
}

// GetErrorDistribution 获取错误分布
func (ag *AutonomousGSwitch) GetErrorDistribution() *ErrorDistribution {
	ag.mu.RLock()
	defer ag.mu.RUnlock()
	return ag.errorDist
}
