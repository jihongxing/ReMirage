// Package phantom 混沌自愈与克苏鲁模式
// 影子无限再生 + 心理压制响应
package phantom

import (
	"bytes"
	"encoding/base64"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// ChaosEngine 混沌引擎
type ChaosEngine struct {
	mu sync.RWMutex

	// 影子实例池
	shadowPool map[string]*ShadowInstance

	// 攻击检测
	attackMetrics map[string]*AttackMetrics

	// 配置
	config ChaosConfig

	// 统计
	stats ChaosStats

	// 回调
	onReincarnation func(uid string, oldIP string, newIP string)
	onCthulhuMode   func(uid string, payload []byte)
}

// ShadowInstance 影子实例
type ShadowInstance struct {
	ID           string
	IP           string
	Port         int
	Fingerprint  string
	CreatedAt    time.Time
	Generation   int
	AssignedUIDs map[string]bool
}

// AttackMetrics 攻击指标
type AttackMetrics struct {
	UID              string
	RequestCount     int64
	BytesReceived    int64
	LastRequestTime  time.Time
	RequestRate      float64 // 请求/秒
	PayloadSignature []byte  // 捕获的攻击载荷
	AttackType       string
	Intensity        int // 1-10
}

// ChaosConfig 配置
type ChaosConfig struct {
	ReincarnationTimeMs int     // 重生时间（毫秒）
	CthulhuThreshold    float64 // 克苏鲁模式触发阈值（请求/秒）
	MaxShadowInstances  int
	PayloadMirrorRate   float32 // 载荷回显比例
}

// ChaosStats 统计
type ChaosStats struct {
	TotalReincarnations int64
	ActiveShadows       int
	CthulhuActivations  int64
	PayloadsMirrored    int64
	TotalAttackBytes    int64
}

// DefaultChaosConfig 默认配置
func DefaultChaosConfig() ChaosConfig {
	return ChaosConfig{
		ReincarnationTimeMs: 100,
		CthulhuThreshold:    100, // 100 req/s 触发
		MaxShadowInstances:  100,
		PayloadMirrorRate:   0.3,
	}
}

// NewChaosEngine 创建混沌引擎
func NewChaosEngine(config ChaosConfig) *ChaosEngine {
	return &ChaosEngine{
		shadowPool:    make(map[string]*ShadowInstance),
		attackMetrics: make(map[string]*AttackMetrics),
		config:        config,
	}
}

// RecordRequest 记录请求（用于攻击检测）
func (ce *ChaosEngine) RecordRequest(uid string, payload []byte) *AttackMetrics {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	metrics, exists := ce.attackMetrics[uid]
	if !exists {
		metrics = &AttackMetrics{
			UID:             uid,
			LastRequestTime: time.Now(),
		}
		ce.attackMetrics[uid] = metrics
	}

	now := time.Now()
	elapsed := now.Sub(metrics.LastRequestTime).Seconds()

	atomic.AddInt64(&metrics.RequestCount, 1)
	atomic.AddInt64(&metrics.BytesReceived, int64(len(payload)))
	ce.stats.TotalAttackBytes += int64(len(payload))

	// 计算请求速率
	if elapsed > 0 {
		metrics.RequestRate = float64(metrics.RequestCount) / elapsed
	}

	// 捕获攻击载荷特征
	if len(payload) > 0 && len(metrics.PayloadSignature) < 1024 {
		metrics.PayloadSignature = append(metrics.PayloadSignature, payload[:min(len(payload), 256)]...)
	}

	metrics.LastRequestTime = now

	// 检测攻击类型
	metrics.AttackType = ce.detectAttackType(payload)
	metrics.Intensity = ce.calculateIntensity(metrics)

	return metrics
}

// detectAttackType 检测攻击类型
func (ce *ChaosEngine) detectAttackType(payload []byte) string {
	if len(payload) == 0 {
		return "probe"
	}

	payloadStr := string(payload)

	// SQL 注入特征
	sqlPatterns := []string{"SELECT", "UNION", "DROP", "INSERT", "UPDATE", "DELETE", "OR 1=1", "' OR '", "--;"}
	for _, p := range sqlPatterns {
		if bytes.Contains(bytes.ToUpper(payload), []byte(p)) {
			return "sql_injection"
		}
	}

	// XSS 特征
	if bytes.Contains(payload, []byte("<script")) || bytes.Contains(payload, []byte("javascript:")) {
		return "xss"
	}

	// 路径遍历
	if bytes.Contains(payload, []byte("../")) || bytes.Contains(payload, []byte("..\\")) {
		return "path_traversal"
	}

	// 命令注入
	cmdPatterns := []string{"; ls", "| cat", "`id`", "$(whoami)", "&& rm"}
	for _, p := range cmdPatterns {
		if bytes.Contains(payload, []byte(p)) {
			return "command_injection"
		}
	}

	// DDoS 特征（大量重复数据）
	if len(payload) > 1000 && ce.isRepetitive(payload) {
		return "ddos"
	}

	// 扫描器特征
	if bytes.Contains(payload, []byte("nmap")) || bytes.Contains(payload, []byte("masscan")) {
		return "scanner"
	}

	_ = payloadStr
	return "unknown"
}

func (ce *ChaosEngine) isRepetitive(data []byte) bool {
	if len(data) < 100 {
		return false
	}

	// 检查前 100 字节的重复模式
	pattern := data[:10]
	count := 0
	for i := 0; i < len(data)-10; i += 10 {
		if bytes.Equal(data[i:i+10], pattern) {
			count++
		}
	}

	return float64(count) > float64(len(data)/10)*0.8
}

func (ce *ChaosEngine) calculateIntensity(metrics *AttackMetrics) int {
	intensity := 1

	// 基于请求速率
	if metrics.RequestRate > 10 {
		intensity += 2
	}
	if metrics.RequestRate > 50 {
		intensity += 2
	}
	if metrics.RequestRate > 100 {
		intensity += 2
	}

	// 基于数据量
	if metrics.BytesReceived > 1024*1024 {
		intensity += 1
	}
	if metrics.BytesReceived > 10*1024*1024 {
		intensity += 2
	}

	if intensity > 10 {
		intensity = 10
	}

	return intensity
}

// ShouldActivateCthulhu 判断是否激活克苏鲁模式
func (ce *ChaosEngine) ShouldActivateCthulhu(uid string) bool {
	ce.mu.RLock()
	metrics, exists := ce.attackMetrics[uid]
	ce.mu.RUnlock()

	if !exists {
		return false
	}

	return metrics.RequestRate >= ce.config.CthulhuThreshold
}

// GenerateCthulhuResponse 生成克苏鲁响应
func (ce *ChaosEngine) GenerateCthulhuResponse(uid string, seed int64) []byte {
	ce.mu.Lock()
	ce.stats.CthulhuActivations++
	metrics := ce.attackMetrics[uid]
	ce.mu.Unlock()

	rng := rand.New(rand.NewSource(seed))

	// 基础乱码（看似 Base64）
	baseSize := 4096 + rng.Intn(8192)
	response := make([]byte, baseSize)
	for i := range response {
		response[i] = byte(rng.Intn(256))
	}

	// 转换为 Base64 格式
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(response)))
	base64.StdEncoding.Encode(encoded, response)

	// 混入攻击者自己的载荷（心理压制）
	if metrics != nil && len(metrics.PayloadSignature) > 0 && rng.Float32() < ce.config.PayloadMirrorRate {
		ce.mu.Lock()
		ce.stats.PayloadsMirrored++
		ce.mu.Unlock()

		// 在随机位置插入攻击者的载荷
		insertPos := rng.Intn(len(encoded) - len(metrics.PayloadSignature))
		copy(encoded[insertPos:], metrics.PayloadSignature)

		if ce.onCthulhuMode != nil {
			go ce.onCthulhuMode(uid, metrics.PayloadSignature)
		}
	}

	// 添加一些"有意义"的假数据头
	headers := []string{
		"MIRAGE-ADAPTIVE-RESPONSE-v2.1\n",
		"THREAT-ANALYSIS-COMPLETE\n",
		"COUNTER-MEASURE-DEPLOYED\n",
		"FINGERPRINT-LOCKED\n",
	}
	header := []byte(headers[rng.Intn(len(headers))])

	return append(header, encoded...)
}

// Reincarnate 影子重生
func (ce *ChaosEngine) Reincarnate(uid string, oldInstanceID string) *ShadowInstance {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	// 获取旧实例
	oldInstance := ce.shadowPool[oldInstanceID]
	generation := 1
	if oldInstance != nil {
		generation = oldInstance.Generation + 1
		delete(ce.shadowPool, oldInstanceID)
	}

	// 生成新实例
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	newInstance := &ShadowInstance{
		ID:           generateInstanceID(rng),
		IP:           generateRandomIP(rng),
		Port:         443 + rng.Intn(1000),
		Fingerprint:  generateFingerprint(rng),
		CreatedAt:    time.Now(),
		Generation:   generation,
		AssignedUIDs: make(map[string]bool),
	}
	newInstance.AssignedUIDs[uid] = true

	ce.shadowPool[newInstance.ID] = newInstance
	ce.stats.TotalReincarnations++
	ce.stats.ActiveShadows = len(ce.shadowPool)

	if ce.onReincarnation != nil && oldInstance != nil {
		go ce.onReincarnation(uid, oldInstance.IP, newInstance.IP)
	}

	return newInstance
}

func generateInstanceID(rng *rand.Rand) string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	id := make([]byte, 12)
	for i := range id {
		id[i] = chars[rng.Intn(len(chars))]
	}
	return "shadow_" + string(id)
}

func generateRandomIP(rng *rand.Rand) string {
	// 生成看似真实的公网 IP
	ranges := []struct{ start, end int }{
		{1, 126},   // Class A
		{128, 191}, // Class B
		{192, 223}, // Class C
	}
	r := ranges[rng.Intn(len(ranges))]
	first := r.start + rng.Intn(r.end-r.start)

	return string(rune('0'+first/100)) + string(rune('0'+(first/10)%10)) + string(rune('0'+first%10)) +
		"." + string(rune('0'+rng.Intn(256)/100)) + string(rune('0'+(rng.Intn(256)/10)%10)) + string(rune('0'+rng.Intn(256)%10)) +
		"." + string(rune('0'+rng.Intn(256)/100)) + string(rune('0'+(rng.Intn(256)/10)%10)) + string(rune('0'+rng.Intn(256)%10)) +
		"." + string(rune('0'+rng.Intn(256)/100)) + string(rune('0'+(rng.Intn(256)/10)%10)) + string(rune('0'+rng.Intn(256)%10))
}

func generateFingerprint(rng *rand.Rand) string {
	chars := "0123456789abcdef"
	fp := make([]byte, 32)
	for i := range fp {
		fp[i] = chars[rng.Intn(len(chars))]
	}
	return string(fp)
}

// GetShadowForUID 获取 UID 对应的影子实例
func (ce *ChaosEngine) GetShadowForUID(uid string) *ShadowInstance {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	for _, instance := range ce.shadowPool {
		if instance.AssignedUIDs[uid] {
			return instance
		}
	}
	return nil
}

// AssignShadow 分配影子实例给 UID
func (ce *ChaosEngine) AssignShadow(uid string) *ShadowInstance {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	// 检查是否已分配
	for _, instance := range ce.shadowPool {
		if instance.AssignedUIDs[uid] {
			return instance
		}
	}

	// 找一个负载最低的实例
	var bestInstance *ShadowInstance
	minLoad := int(^uint(0) >> 1)

	for _, instance := range ce.shadowPool {
		load := len(instance.AssignedUIDs)
		if load < minLoad {
			minLoad = load
			bestInstance = instance
		}
	}

	// 如果没有可用实例或负载过高，创建新实例
	if bestInstance == nil || minLoad > 100 {
		if len(ce.shadowPool) < ce.config.MaxShadowInstances {
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			bestInstance = &ShadowInstance{
				ID:           generateInstanceID(rng),
				IP:           generateRandomIP(rng),
				Port:         443 + rng.Intn(1000),
				Fingerprint:  generateFingerprint(rng),
				CreatedAt:    time.Now(),
				Generation:   1,
				AssignedUIDs: make(map[string]bool),
			}
			ce.shadowPool[bestInstance.ID] = bestInstance
			ce.stats.ActiveShadows = len(ce.shadowPool)
		}
	}

	if bestInstance != nil {
		bestInstance.AssignedUIDs[uid] = true
	}

	return bestInstance
}

// GetAttackMetrics 获取攻击指标
func (ce *ChaosEngine) GetAttackMetrics(uid string) *AttackMetrics {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.attackMetrics[uid]
}

// GetAllAttackMetrics 获取所有攻击指标
func (ce *ChaosEngine) GetAllAttackMetrics() map[string]*AttackMetrics {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	result := make(map[string]*AttackMetrics)
	for k, v := range ce.attackMetrics {
		result[k] = v
	}
	return result
}

// GetStats 获取统计
func (ce *ChaosEngine) GetStats() ChaosStats {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	stats := ce.stats
	stats.ActiveShadows = len(ce.shadowPool)
	return stats
}

// OnReincarnation 设置重生回调
func (ce *ChaosEngine) OnReincarnation(fn func(uid string, oldIP string, newIP string)) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.onReincarnation = fn
}

// OnCthulhuMode 设置克苏鲁模式回调
func (ce *ChaosEngine) OnCthulhuMode(fn func(uid string, payload []byte)) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.onCthulhuMode = fn
}

// CleanupStale 清理过期数据
func (ce *ChaosEngine) CleanupStale(maxAge time.Duration) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	now := time.Now()

	// 清理过期攻击指标
	for uid, metrics := range ce.attackMetrics {
		if now.Sub(metrics.LastRequestTime) > maxAge {
			delete(ce.attackMetrics, uid)
		}
	}

	// 清理空闲影子实例
	for id, instance := range ce.shadowPool {
		if len(instance.AssignedUIDs) == 0 && now.Sub(instance.CreatedAt) > maxAge {
			delete(ce.shadowPool, id)
		}
	}

	ce.stats.ActiveShadows = len(ce.shadowPool)
}

// ============== Self-Destruct 物理清空逻辑 ==============

// SelfDestructConfig 自毁配置
type SelfDestructConfig struct {
	WipeMemory      bool          // 清空内存
	WipeEBPFMaps    bool          // 清空 eBPF Maps
	WipeLogs        bool          // 清空日志
	WipeTmpfs       bool          // 清空 tmpfs
	NotifyPeers     bool          // 通知对等节点
	GracePeriodMs   int           // 宽限期（毫秒）
	ConfirmCode     string        // 确认码（防误触）
}

// SelfDestructResult 自毁结果
type SelfDestructResult struct {
	Success       bool
	MemoryWiped   bool
	EBPFWiped     bool
	LogsWiped     bool
	TmpfsWiped    bool
	PeersNotified int
	Errors        []string
}

// SelfDestruct 执行物理清空
func (ce *ChaosEngine) SelfDestruct(config SelfDestructConfig, confirmCode string) *SelfDestructResult {
	result := &SelfDestructResult{Errors: make([]string, 0)}

	// 验证确认码
	if config.ConfirmCode != "" && config.ConfirmCode != confirmCode {
		result.Errors = append(result.Errors, "确认码不匹配")
		return result
	}

	// 宽限期
	if config.GracePeriodMs > 0 {
		time.Sleep(time.Duration(config.GracePeriodMs) * time.Millisecond)
	}

	// 1. 清空内存数据
	if config.WipeMemory {
		ce.wipeMemory()
		result.MemoryWiped = true
	}

	// 2. 清空 eBPF Maps（通过写入 0xDEADBEEF 触发内核清理）
	if config.WipeEBPFMaps {
		if err := ce.wipeEBPFMaps(); err != nil {
			result.Errors = append(result.Errors, "eBPF 清空失败: "+err.Error())
		} else {
			result.EBPFWiped = true
		}
	}

	// 3. 清空日志
	if config.WipeLogs {
		if err := ce.wipeLogs(); err != nil {
			result.Errors = append(result.Errors, "日志清空失败: "+err.Error())
		} else {
			result.LogsWiped = true
		}
	}

	// 4. 清空 tmpfs
	if config.WipeTmpfs {
		if err := ce.wipeTmpfs(); err != nil {
			result.Errors = append(result.Errors, "tmpfs 清空失败: "+err.Error())
		} else {
			result.TmpfsWiped = true
		}
	}

	result.Success = len(result.Errors) == 0
	return result
}

// wipeMemory 安全清空内存
func (ce *ChaosEngine) wipeMemory() {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	// 清空影子池
	for id, instance := range ce.shadowPool {
		ce.secureWipeInstance(instance)
		delete(ce.shadowPool, id)
	}

	// 清空攻击指标
	for uid, metrics := range ce.attackMetrics {
		ce.secureWipeMetrics(metrics)
		delete(ce.attackMetrics, uid)
	}

	// 重置统计
	ce.stats = ChaosStats{}
}

// secureWipeInstance 安全擦除实例
func (ce *ChaosEngine) secureWipeInstance(instance *ShadowInstance) {
	if instance == nil {
		return
	}
	// 覆盖敏感字段
	instance.ID = ""
	instance.IP = ""
	instance.Fingerprint = ""
	instance.AssignedUIDs = nil
}

// secureWipeMetrics 安全擦除指标
func (ce *ChaosEngine) secureWipeMetrics(metrics *AttackMetrics) {
	if metrics == nil {
		return
	}
	metrics.UID = ""
	metrics.AttackType = ""
	// 覆盖载荷签名
	for i := range metrics.PayloadSignature {
		metrics.PayloadSignature[i] = 0
	}
	metrics.PayloadSignature = nil
}

// wipeEBPFMaps 清空 eBPF Maps
func (ce *ChaosEngine) wipeEBPFMaps() error {
	// 写入自毁魔数到 emergency_ctrl_map
	// 内核层检测到 0xDEADBEEF 后会清空所有 Map
	return writeEmergencySignal(0xDEADBEEF)
}

// wipeLogs 清空日志文件
func (ce *ChaosEngine) wipeLogs() error {
	logPaths := []string{
		"/var/log/mirage/",
		"/tmp/mirage-logs/",
	}

	for _, path := range logPaths {
		if err := secureDeleteDir(path); err != nil {
			// 继续尝试其他路径
			continue
		}
	}
	return nil
}

// wipeTmpfs 清空 tmpfs
func (ce *ChaosEngine) wipeTmpfs() error {
	tmpfsPaths := []string{
		"/dev/shm/mirage/",
		"/run/mirage/",
	}

	for _, path := range tmpfsPaths {
		if err := secureDeleteDir(path); err != nil {
			continue
		}
	}
	return nil
}

// writeEmergencySignal 写入紧急信号（通过 sysfs 或 procfs）
func writeEmergencySignal(signal uint32) error {
	// 实际实现需要通过 eBPF Map 写入
	// 这里是占位符，实际由 ebpf.Loader 处理
	_ = signal
	return nil
}

// secureDeleteDir 安全删除目录
func secureDeleteDir(path string) error {
	// 1. 先覆盖文件内容
	// 2. 再删除文件
	// 3. 最后删除目录
	// 实际实现需要遍历目录
	_ = path
	return nil
}
