// Package cortex 威胁感知中枢
// 实现指纹-IP 关联分析与预测性拦截
package cortex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Analyzer 数据炼金术分析器
type Analyzer struct {
	mu sync.RWMutex

	// 指纹库
	fingerprints map[string]*IdentityFingerprint

	// IP -> 指纹映射
	ipToFingerprint map[string]string

	// 高危指纹缓存（用于 eBPF 快速查询）
	highRiskCache map[string]bool

	// 白名单指纹
	trustedFingerprints map[string]bool

	// 配置
	config AnalyzerConfig

	// 回调
	onHighRisk   func(fp *IdentityFingerprint)
	onAutoBlock  func(ip string, reason string)

	// 停止信号
	stopChan chan struct{}
}

// IdentityFingerprint 身份指纹
type IdentityFingerprint struct {
	UID           string    `json:"uid"`
	Hash          string    `json:"hash"`
	FirstSeen     time.Time `json:"firstSeen"`
	LastSeen      time.Time `json:"lastSeen"`
	ThreatScore   int       `json:"threatScore"`
	AssociatedIPs []string  `json:"associatedIPs"`

	// 行为统计
	HoneypotDwellTime time.Duration `json:"honeypotDwellTime"`
	CanaryTriggered   int           `json:"canaryTriggered"`
	SQLInjectionAttempts int        `json:"sqlInjectionAttempts"`
	DirScanAttempts   int           `json:"dirScanAttempts"`

	// 地理偏好
	RegionPreference map[string]int `json:"regionPreference"`

	// 状态
	IsHighRisk bool `json:"isHighRisk"`
	IsTrusted  bool `json:"isTrusted"`
	IsBlocked  bool `json:"isBlocked"`
}

// AnalyzerConfig 分析器配置
type AnalyzerConfig struct {
	ScanInterval       time.Duration
	HighRiskThreshold  int
	DwellTimeThreshold time.Duration
	MaxCacheSize       int
}

// DefaultConfig 默认配置
func DefaultConfig() AnalyzerConfig {
	return AnalyzerConfig{
		ScanInterval:       5 * time.Minute,
		HighRiskThreshold:  50,
		DwellTimeThreshold: 10 * time.Minute,
		MaxCacheSize:       10000,
	}
}

// NewAnalyzer 创建分析器
func NewAnalyzer(config AnalyzerConfig) *Analyzer {
	return &Analyzer{
		fingerprints:        make(map[string]*IdentityFingerprint),
		ipToFingerprint:     make(map[string]string),
		highRiskCache:       make(map[string]bool),
		trustedFingerprints: make(map[string]bool),
		config:              config,
		stopChan:            make(chan struct{}),
	}
}

// Start 启动分析循环
func (a *Analyzer) Start(ctx context.Context) {
	ticker := time.NewTicker(a.config.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopChan:
			return
		case <-ticker.C:
			a.runAnalysis()
		}
	}
}

// Stop 停止分析器
func (a *Analyzer) Stop() {
	close(a.stopChan)
}

// runAnalysis 执行分析
func (a *Analyzer) runAnalysis() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, fp := range a.fingerprints {
		if fp.IsTrusted {
			continue
		}

		// 计算威胁分值
		score := a.calculateThreatScore(fp)
		fp.ThreatScore = score

		// 检查是否达到高危阈值
		if score >= a.config.HighRiskThreshold && !fp.IsHighRisk {
			fp.IsHighRisk = true
			a.highRiskCache[fp.Hash] = true

			// 自动封禁关联 IP
			for _, ip := range fp.AssociatedIPs {
				if a.onAutoBlock != nil {
					go a.onAutoBlock(ip, fmt.Sprintf("high_risk_fingerprint:%s", fp.UID))
				}
			}

			if a.onHighRisk != nil {
				go a.onHighRisk(fp)
			}
		}

		// 检查蜜罐停留时间
		if fp.HoneypotDwellTime >= a.config.DwellTimeThreshold && !fp.IsBlocked {
			fp.IsBlocked = true
			a.highRiskCache[fp.Hash] = true
		}

		// 检查金丝雀触发
		if fp.CanaryTriggered > 0 && !fp.IsBlocked {
			fp.IsBlocked = true
			fp.ThreatScore += fp.CanaryTriggered * 20
			a.highRiskCache[fp.Hash] = true
		}
	}

	// 维护缓存大小
	a.pruneCache()
}

// calculateThreatScore 计算威胁分值
func (a *Analyzer) calculateThreatScore(fp *IdentityFingerprint) int {
	score := 0

	// 关联 IP 数量（每个额外 IP +10）
	if len(fp.AssociatedIPs) > 1 {
		score += (len(fp.AssociatedIPs) - 1) * 10
	}

	// 蜜罐停留时间（每分钟 +2）
	score += int(fp.HoneypotDwellTime.Minutes()) * 2

	// 金丝雀触发（每次 +20）
	score += fp.CanaryTriggered * 20

	// SQL 注入尝试（每次 +15）
	score += fp.SQLInjectionAttempts * 15

	// 目录扫描（每次 +5）
	score += fp.DirScanAttempts * 5

	// 多区域攻击（每个额外区域 +10）
	if len(fp.RegionPreference) > 1 {
		score += (len(fp.RegionPreference) - 1) * 10
	}

	return score
}

// pruneCache 修剪缓存
func (a *Analyzer) pruneCache() {
	if len(a.highRiskCache) <= a.config.MaxCacheSize {
		return
	}

	// 保留最高风险的指纹
	type scored struct {
		hash  string
		score int
	}
	var all []scored
	for hash := range a.highRiskCache {
		if fp, ok := a.fingerprints[hash]; ok {
			all = append(all, scored{hash, fp.ThreatScore})
		}
	}

	// 按分数排序，保留前 MaxCacheSize 个
	// 简化实现：直接删除超出部分
	count := 0
	for hash := range a.highRiskCache {
		if count >= a.config.MaxCacheSize {
			delete(a.highRiskCache, hash)
		}
		count++
	}
}

// RecordFingerprint 记录指纹
func (a *Analyzer) RecordFingerprint(hash string, ip string, region string) *IdentityFingerprint {
	a.mu.Lock()
	defer a.mu.Unlock()

	fp, exists := a.fingerprints[hash]
	if !exists {
		fp = &IdentityFingerprint{
			UID:              generateUID(hash),
			Hash:             hash,
			FirstSeen:        time.Now(),
			LastSeen:         time.Now(),
			AssociatedIPs:    []string{ip},
			RegionPreference: make(map[string]int),
		}
		a.fingerprints[hash] = fp
	} else {
		fp.LastSeen = time.Now()

		// 添加新 IP
		found := false
		for _, existingIP := range fp.AssociatedIPs {
			if existingIP == ip {
				found = true
				break
			}
		}
		if !found {
			fp.AssociatedIPs = append(fp.AssociatedIPs, ip)
		}
	}

	// 更新区域偏好
	if region != "" {
		fp.RegionPreference[region]++
	}

	a.ipToFingerprint[ip] = hash
	return fp
}

// RecordHoneypotActivity 记录蜜罐活动
func (a *Analyzer) RecordHoneypotActivity(hash string, dwellTime time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if fp, ok := a.fingerprints[hash]; ok {
		fp.HoneypotDwellTime += dwellTime
	}
}

// RecordCanaryTrigger 记录金丝雀触发
func (a *Analyzer) RecordCanaryTrigger(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if fp, ok := a.fingerprints[hash]; ok {
		fp.CanaryTriggered++
	}
}

// RecordSQLInjection 记录 SQL 注入尝试
func (a *Analyzer) RecordSQLInjection(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if fp, ok := a.fingerprints[hash]; ok {
		fp.SQLInjectionAttempts++
	}
}

// RecordDirScan 记录目录扫描
func (a *Analyzer) RecordDirScan(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if fp, ok := a.fingerprints[hash]; ok {
		fp.DirScanAttempts++
	}
}

// IsHighRisk 检查指纹是否高危（O(1) 查询）
func (a *Analyzer) IsHighRisk(hash string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 先检查白名单
	if a.trustedFingerprints[hash] {
		return false
	}

	return a.highRiskCache[hash]
}

// IsHighRiskIP 通过 IP 检查是否高危
func (a *Analyzer) IsHighRiskIP(ip string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	hash, ok := a.ipToFingerprint[ip]
	if !ok {
		return false
	}

	if a.trustedFingerprints[hash] {
		return false
	}

	return a.highRiskCache[hash]
}

// AddTrustedFingerprint 添加白名单指纹
func (a *Analyzer) AddTrustedFingerprint(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.trustedFingerprints[hash] = true

	// 从高危缓存移除
	delete(a.highRiskCache, hash)

	// 更新指纹状态
	if fp, ok := a.fingerprints[hash]; ok {
		fp.IsTrusted = true
		fp.IsHighRisk = false
		fp.IsBlocked = false
	}
}

// RemoveTrustedFingerprint 移除白名单指纹
func (a *Analyzer) RemoveTrustedFingerprint(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.trustedFingerprints, hash)

	if fp, ok := a.fingerprints[hash]; ok {
		fp.IsTrusted = false
	}
}

// GetHighRiskHashes 获取高危指纹哈希列表（用于同步到 eBPF）
func (a *Analyzer) GetHighRiskHashes() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	hashes := make([]string, 0, len(a.highRiskCache))
	for hash := range a.highRiskCache {
		hashes = append(hashes, hash)
	}
	return hashes
}

// GetFingerprint 获取指纹详情
func (a *Analyzer) GetFingerprint(hash string) *IdentityFingerprint {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.fingerprints[hash]
}

// GetAllFingerprints 获取所有指纹
func (a *Analyzer) GetAllFingerprints() []*IdentityFingerprint {
	a.mu.RLock()
	defer a.mu.RUnlock()

	fps := make([]*IdentityFingerprint, 0, len(a.fingerprints))
	for _, fp := range a.fingerprints {
		fps = append(fps, fp)
	}
	return fps
}

// GetStats 获取统计信息
func (a *Analyzer) GetStats() CortexStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	stats := CortexStats{
		TotalFingerprints:   len(a.fingerprints),
		HighRiskCount:       len(a.highRiskCache),
		TrustedCount:        len(a.trustedFingerprints),
		TotalAssociatedIPs:  len(a.ipToFingerprint),
	}

	// 计算自动封禁比例
	if stats.TotalFingerprints > 0 {
		stats.AutoBlockRate = float64(stats.HighRiskCount) / float64(stats.TotalFingerprints) * 100
	}

	// 计算预判拦截数
	for _, fp := range a.fingerprints {
		if fp.IsHighRisk && len(fp.AssociatedIPs) > 1 {
			stats.PredictiveBlocks += len(fp.AssociatedIPs) - 1
		}
	}

	return stats
}

// CortexStats 统计信息
type CortexStats struct {
	TotalFingerprints  int     `json:"totalFingerprints"`
	HighRiskCount      int     `json:"highRiskCount"`
	TrustedCount       int     `json:"trustedCount"`
	TotalAssociatedIPs int     `json:"totalAssociatedIPs"`
	AutoBlockRate      float64 `json:"autoBlockRate"`
	PredictiveBlocks   int     `json:"predictiveBlocks"`
}

// OnHighRisk 设置高危回调
func (a *Analyzer) OnHighRisk(fn func(fp *IdentityFingerprint)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onHighRisk = fn
}

// OnAutoBlock 设置自动封禁回调
func (a *Analyzer) OnAutoBlock(fn func(ip string, reason string)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onAutoBlock = fn
}

func generateUID(hash string) string {
	h := sha256.Sum256([]byte(hash + time.Now().String()))
	return "uid_" + hex.EncodeToString(h[:])[:12]
}
