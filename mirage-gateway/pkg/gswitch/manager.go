// Package gswitch - G-Switch 域名转生协议
// 执行"壁虎断尾"，秒级切换通信域名
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

// DomainStatus 域名状态
type DomainStatus int

const (
	DomainActive  DomainStatus = 0 // 活跃
	DomainStandby DomainStatus = 1 // 热备
	DomainBurned  DomainStatus = 2 // 已战死
	DomainCooling DomainStatus = 3 // 冷却中
)

// Domain 域名实体
type Domain struct {
	Name       string       `json:"name"`
	IP         string       `json:"ip"`
	Status     DomainStatus `json:"status"`
	CreatedAt  time.Time    `json:"created_at"`
	LastUsed   time.Time    `json:"last_used"`
	UsageCount uint64       `json:"usage_count"`
	BurnedAt   *time.Time   `json:"burned_at,omitempty"`
	BurnReason string       `json:"burn_reason,omitempty"`
}

// SNIMapping SNI 映射（写入 eBPF）
type SNIMapping struct {
	OldSNI    [64]byte
	NewSNI    [64]byte
	Timestamp uint64
	Active    uint32
}

// GSwitchManager 域名转生管理器
type GSwitchManager struct {
	mu sync.RWMutex

	// 域名池
	activePool  []*Domain // 活跃域名
	standbyPool []*Domain // 热备域名
	burnedPool  []*Domain // 已战死域名

	// 当前活跃域名
	currentDomain *Domain

	// eBPF Map 引用
	sniMap              *ebpf.Map
	domainCtrl          *ebpf.Map
	bdnaProfileSwitcher BDNAProfileSwitcher

	// M.C.C. 通报回调
	onDomainBurned func(domain *Domain)
	onBDNAReset    func(reason string) // B-DNA 重置回调

	// 耗材冷却机制
	recentBurns       []time.Time   // 最近战死时间戳
	burnRateWindow    time.Duration // 统计窗口
	burnRateThreshold int           // 触发 B-DNA Reset 的阈值

	// 配置
	minStandbyCount int           // 最小热备数量
	cooldownPeriod  time.Duration // 冷却期

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewGSwitchManager 创建域名转生管理器
func NewGSwitchManager(sniMap, domainCtrl *ebpf.Map) *GSwitchManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &GSwitchManager{
		activePool:        make([]*Domain, 0),
		standbyPool:       make([]*Domain, 0),
		burnedPool:        make([]*Domain, 0),
		sniMap:            sniMap,
		domainCtrl:        domainCtrl,
		recentBurns:       make([]time.Time, 0),
		burnRateWindow:    5 * time.Minute,
		burnRateThreshold: 3, // 5 分钟内 3 次战死触发 B-DNA Reset
		minStandbyCount:   5,
		cooldownPeriod:    24 * time.Hour,
		ctx:               ctx,
		cancel:            cancel,
	}
}

// SetBDNAProfileSwitcher 设置 B-DNA 画像切换器。
func (gm *GSwitchManager) SetBDNAProfileSwitcher(switcher BDNAProfileSwitcher) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.bdnaProfileSwitcher = switcher
}

// SetJA4Map 兼容旧调用方，内部桥接到 active_profile_map 切换器。
func (gm *GSwitchManager) SetJA4Map(ja4Map *ebpf.Map) {
	gm.SetBDNAProfileSwitcher(&rawMapBDNAProfileSwitcher{activeProfileMap: ja4Map})
}

// SetBDNAResetCallback 设置 B-DNA 重置回调
func (gm *GSwitchManager) SetBDNAResetCallback(callback func(string)) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.onBDNAReset = callback
}

// SetBurnedCallback 设置域名战死回调
func (gm *GSwitchManager) SetBurnedCallback(callback func(*Domain)) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.onDomainBurned = callback
}

// Start 启动管理器
func (gm *GSwitchManager) Start() error {
	// 启动热备补充循环
	gm.wg.Add(1)
	go gm.standbyRefillLoop()

	// 启动冷却回收循环
	gm.wg.Add(1)
	go gm.cooldownRecycleLoop()

	log.Println("🦎 G-Switch 域名转生管理器已启动")
	return nil
}

// Stop 停止管理器
func (gm *GSwitchManager) Stop() {
	gm.cancel()
	gm.wg.Wait()
	log.Println("🛑 G-Switch 管理器已停止")
}

// AddDomain 添加域名到热备池
func (gm *GSwitchManager) AddDomain(name, ip string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	domain := &Domain{
		Name:      name,
		IP:        ip,
		Status:    DomainStandby,
		CreatedAt: time.Now(),
	}

	gm.standbyPool = append(gm.standbyPool, domain)
	log.Printf("📥 域名已加入热备池: %s", name)
	return nil
}

// ActivateDomain 激活域名
func (gm *GSwitchManager) ActivateDomain(domain *Domain) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	domain.Status = DomainActive
	domain.LastUsed = time.Now()
	gm.currentDomain = domain

	// 更新 eBPF SNI Map
	if err := gm.updateSNIMap(domain); err != nil {
		return fmt.Errorf("更新 SNI Map 失败: %w", err)
	}

	log.Printf("✅ 域名已激活: %s", domain.Name)
	return nil
}

// TriggerEscape 触发逃逸（Red Alert 时调用）
func (gm *GSwitchManager) TriggerEscape(reason string) error {
	gm.mu.Lock()

	// 记录战死时间
	gm.recentBurns = append(gm.recentBurns, time.Now())

	// 检查是否需要触发 B-DNA Reset
	needBDNAReset := gm.checkBurnRate()

	// 1. 标记当前域名为战死
	if gm.currentDomain != nil {
		now := time.Now()
		gm.currentDomain.Status = DomainBurned
		gm.currentDomain.BurnedAt = &now
		gm.currentDomain.BurnReason = reason
		gm.burnedPool = append(gm.burnedPool, gm.currentDomain)

		burnedDomain := gm.currentDomain
		callback := gm.onDomainBurned

		log.Printf("💀 域名已战死: %s (原因: %s)", burnedDomain.Name, reason)

		// 异步通报 M.C.C.
		if callback != nil {
			go callback(burnedDomain)
		}
	}

	// 2. 从热备池选取新域名
	if len(gm.standbyPool) == 0 {
		gm.mu.Unlock()
		return fmt.Errorf("热备池已空，无法逃逸")
	}

	newDomain := gm.standbyPool[0]
	gm.standbyPool = gm.standbyPool[1:]

	// 3. 激活新域名
	newDomain.Status = DomainActive
	newDomain.LastUsed = time.Now()
	gm.currentDomain = newDomain

	bdnaCallback := gm.onBDNAReset
	gm.mu.Unlock()

	// 4. 更新 eBPF（零中断迁移）
	if err := gm.updateSNIMap(newDomain); err != nil {
		return fmt.Errorf("更新 SNI Map 失败: %w", err)
	}

	log.Printf("🦎 域名转生完成: %s → %s",
		gm.burnedPool[len(gm.burnedPool)-1].Name, newDomain.Name)

	// 5. 如果战死频率过高，触发 B-DNA Reset
	if needBDNAReset {
		log.Printf("⚠️  域名战死频率过高，触发 B-DNA Reset")
		gm.triggerBDNAReset("high_burn_rate")
		if bdnaCallback != nil {
			go bdnaCallback("high_burn_rate")
		}
	}

	return nil
}

// checkBurnRate 检查战死频率
func (gm *GSwitchManager) checkBurnRate() bool {
	cutoff := time.Now().Add(-gm.burnRateWindow)

	// 清理过期记录
	var recent []time.Time
	for _, t := range gm.recentBurns {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	gm.recentBurns = recent

	return len(recent) >= gm.burnRateThreshold
}

// triggerBDNAReset 触发 B-DNA 重置
func (gm *GSwitchManager) triggerBDNAReset(reason string) {
	if gm.bdnaProfileSwitcher == nil {
		log.Printf("⚠️  B-DNA 画像切换器未设置，跳过画像重置")
		return
	}

	profileID := randomBDNAProfileID(gm.bdnaProfileSwitcher)

	if err := gm.bdnaProfileSwitcher.SetActiveProfile(profileID); err != nil {
		log.Printf("⚠️  B-DNA Reset 失败: %v", err)
		return
	}

	log.Printf("🎭 B-DNA 画像已切换: profile=%d (reason=%s)", profileID, reason)
}

// updateSNIMap 更新 eBPF SNI 映射
func (gm *GSwitchManager) updateSNIMap(domain *Domain) error {
	if gm.sniMap == nil {
		return nil
	}

	var mapping SNIMapping
	copy(mapping.NewSNI[:], domain.Name)
	mapping.Timestamp = uint64(time.Now().UnixNano())
	mapping.Active = 1

	key := uint32(0)
	return gm.sniMap.Put(&key, &mapping)
}

// standbyRefillLoop 热备补充循环
func (gm *GSwitchManager) standbyRefillLoop() {
	defer gm.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.refillStandby()
		}
	}
}

// refillStandby 补充热备域名
func (gm *GSwitchManager) refillStandby() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	deficit := gm.minStandbyCount - len(gm.standbyPool)
	if deficit <= 0 {
		return
	}

	// 生成临时域名（实际应从 M.C.C. 获取）
	for i := 0; i < deficit; i++ {
		domain := gm.generateTempDomain()
		gm.standbyPool = append(gm.standbyPool, domain)
	}

	log.Printf("📦 热备池已补充 %d 个域名，当前: %d", deficit, len(gm.standbyPool))
}

// generateTempDomain 生成临时域名
func (gm *GSwitchManager) generateTempDomain() *Domain {
	// 生成随机子域名
	randBytes := make([]byte, 8)
	rand.Read(randBytes)
	subdomain := hex.EncodeToString(randBytes)

	return &Domain{
		Name:      fmt.Sprintf("%s.cdn.example.com", subdomain),
		IP:        "0.0.0.0", // 待分配
		Status:    DomainStandby,
		CreatedAt: time.Now(),
	}
}

// cooldownRecycleLoop 冷却回收循环
func (gm *GSwitchManager) cooldownRecycleLoop() {
	defer gm.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.recycleCooledDomains()
		}
	}
}

// recycleCooledDomains 回收冷却完成的域名
func (gm *GSwitchManager) recycleCooledDomains() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	now := time.Now()
	var remaining []*Domain

	for _, domain := range gm.burnedPool {
		if domain.BurnedAt != nil && now.Sub(*domain.BurnedAt) > gm.cooldownPeriod {
			// 冷却完成，可以回收
			domain.Status = DomainCooling
			log.Printf("♻️  域名冷却完成，可回收: %s", domain.Name)
		} else {
			remaining = append(remaining, domain)
		}
	}

	gm.burnedPool = remaining
}

// GetCurrentDomain 获取当前活跃域名
func (gm *GSwitchManager) GetCurrentDomain() *Domain {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.currentDomain
}

// GetPoolStats 获取域名池统计
func (gm *GSwitchManager) GetPoolStats() map[string]int {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	return map[string]int{
		"active":  len(gm.activePool),
		"standby": len(gm.standbyPool),
		"burned":  len(gm.burnedPool),
	}
}

// ImportDomains 批量导入域名
func (gm *GSwitchManager) ImportDomains(domains []string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for _, name := range domains {
		domain := &Domain{
			Name:      name,
			Status:    DomainStandby,
			CreatedAt: time.Now(),
		}
		gm.standbyPool = append(gm.standbyPool, domain)
	}

	log.Printf("📥 批量导入 %d 个域名", len(domains))
}
