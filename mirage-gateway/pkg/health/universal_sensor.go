// Package health - 通用业务回显探测器
// 基于响应头异动和内容熵增检测，无需适配特定 App
package health

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"mirage-gateway/pkg/storage"
)

// ProbeNode 探测节点
type ProbeNode struct {
	Name     string `json:"name"`
	Region   string `json:"region"`
	Endpoint string `json:"endpoint"` // 代理端点
	Weight   float64 `json:"weight"`
}

// UserAgentProfile UA 配置
type UserAgentProfile struct {
	Name string
	UA   string
}

// ResponseFingerprint 响应指纹
type ResponseFingerprint struct {
	ContentLength int64             `json:"content_length"`
	ContentType   string            `json:"content_type"`
	StatusCode    int               `json:"status_code"`
	HeaderHash    string            `json:"header_hash"`
	BodyHash      string            `json:"body_hash"`
	BodyEntropy   float64           `json:"body_entropy"`
	Timestamp     time.Time         `json:"timestamp"`
}

// HijackingSignature 劫持指纹
type HijackingSignature struct {
	Type            string    `json:"type"` // content_injection, redirect, block_page
	ExpectedLength  int64     `json:"expected_length"`
	ActualLength    int64     `json:"actual_length"`
	ExpectedType    string    `json:"expected_type"`
	ActualType      string    `json:"actual_type"`
	EntropyDelta    float64   `json:"entropy_delta"`
	DetectedAt      time.Time `json:"detected_at"`
	DetectedRegion  string    `json:"detected_region"`
}

// DomainReputation 域名信誉
type DomainReputation struct {
	Domain          string                 `json:"domain"`
	Score           float64                `json:"score"` // 0-100
	BaselineHash    string                 `json:"baseline_hash"`
	BaselineLength  int64                  `json:"baseline_length"`
	BaselineEntropy float64                `json:"baseline_entropy"`
	LastCheck       time.Time              `json:"last_check"`
	FailCount       int                    `json:"fail_count"`
	HijackCount     int                    `json:"hijack_count"`
	RegionalScores  map[string]float64     `json:"regional_scores"`
	Signatures      []*HijackingSignature  `json:"signatures"`
}

// UniversalSensor 通用传感器
type UniversalSensor struct {
	mu sync.RWMutex

	// 探测节点
	probeNodes []*ProbeNode

	// UA 配置
	userAgents []*UserAgentProfile

	// 域名信誉库
	reputations map[string]*DomainReputation

	// 劫持指纹库
	hijackSignatures []*HijackingSignature

	// App 探测池
	probePool []string

	// HTTP 客户端
	httpClient *http.Client

	// Vault 持久化
	vault *storage.VaultStorage

	// IP 指标批量写入通道
	ipMetricsChan chan *ipMetricEntry
	ipBatchSize   int
	ipFlushTicker *time.Ticker

	// 回调
	onReputationDrop func(domain string, score float64, reason string)
	onHijackDetected func(domain string, sig *HijackingSignature)

	// 配置
	probeInterval    time.Duration
	reputationDecay  float64 // 信誉衰减系数
	hijackThreshold  float64 // 劫持检测阈值

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ipMetricEntry IP 指标条目（用于批量写入）
type ipMetricEntry struct {
	IP      string
	Latency float64
	Success bool
	Region  string
}

// NewUniversalSensor 创建通用传感器
func NewUniversalSensor() *UniversalSensor {
	ctx, cancel := context.WithCancel(context.Background())

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        100,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}

	return &UniversalSensor{
		probeNodes:       getDefaultProbeNodes(),
		userAgents:       getDefaultUserAgents(),
		reputations:      make(map[string]*DomainReputation),
		hijackSignatures: make([]*HijackingSignature, 0),
		probePool:        getDefaultProbePool(),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
		ipMetricsChan:   make(chan *ipMetricEntry, 1000), // 缓冲 1000 条
		ipBatchSize:     100,                             // 每批 100 条
		probeInterval:   60 * time.Second,
		reputationDecay: 0.95,
		hijackThreshold: 0.3,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// NewUniversalSensorWithVault 创建带持久化的传感器
func NewUniversalSensorWithVault(vault *storage.VaultStorage) *UniversalSensor {
	us := NewUniversalSensor()
	us.vault = vault
	// 启动批量写入协程
	us.wg.Add(1)
	go us.ipMetricsBatchWriter()
	return us
}

// ipMetricsBatchWriter 批量写入 IP 指标（Write-Coalescing）
func (us *UniversalSensor) ipMetricsBatchWriter() {
	defer us.wg.Done()

	batch := make([]*ipMetricEntry, 0, us.ipBatchSize)
	flushTicker := time.NewTicker(5 * time.Second) // 5 秒强制刷新
	defer flushTicker.Stop()

	flush := func() {
		if len(batch) == 0 || us.vault == nil {
			return
		}

		// 批量写入
		for _, entry := range batch {
			record := &storage.IPReputationRecord{
				IP:      entry.IP,
				Latency: entry.Latency,
				Region:  entry.Region,
			}
			if entry.Success {
				record.ReputationScore = 80
				record.SuccessCount = 1
			} else {
				record.ReputationScore = 50
				record.FailCount = 1
			}
			us.vault.SaveIPReputation(record)
		}

		log.Printf("📦 批量写入 %d 条 IP 指标", len(batch))
		batch = batch[:0] // 清空
	}

	for {
		select {
		case <-us.ctx.Done():
			flush() // 退出前刷新
			return
		case entry := <-us.ipMetricsChan:
			batch = append(batch, entry)
			if len(batch) >= us.ipBatchSize {
				flush()
			}
		case <-flushTicker.C:
			flush()
		}
	}
}

// PersistIPMetrics 持久化 IP 指标到 Vault（非阻塞）
func (us *UniversalSensor) PersistIPMetrics(ip string, latency float64, success bool, region string) {
	if us.vault == nil {
		return
	}

	// 非阻塞发送到批量写入通道
	select {
	case us.ipMetricsChan <- &ipMetricEntry{
		IP:      ip,
		Latency: latency,
		Success: success,
		Region:  region,
	}:
	default:
		// 通道满，丢弃（避免阻塞）
		log.Println("⚠️ IP 指标通道已满，丢弃")
	}
}

// GetBestExitIPs 获取最佳出口 IP（基于历史数据）
func (us *UniversalSensor) GetBestExitIPs(limit int) ([]*storage.IPReputationRecord, error) {
	if us.vault == nil {
		return nil, fmt.Errorf("vault 未初始化")
	}
	return us.vault.GetTopIPs(limit)
}

// getDefaultProbeNodes 默认探测节点
func getDefaultProbeNodes() []*ProbeNode {
	return []*ProbeNode{
		{Name: "sg-primary", Region: "sg", Endpoint: "", Weight: 1.0},
		{Name: "de-frankfurt", Region: "de", Endpoint: "", Weight: 1.0},
		{Name: "us-west", Region: "us", Endpoint: "", Weight: 1.0},
		{Name: "ch-zurich", Region: "ch", Endpoint: "", Weight: 1.2}, // 瑞士权重更高
		{Name: "jp-tokyo", Region: "jp", Endpoint: "", Weight: 0.9},
	}
}

// getDefaultUserAgents 默认 UA 配置
func getDefaultUserAgents() []*UserAgentProfile {
	return []*UserAgentProfile{
		{Name: "chrome_win", UA: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0"},
		{Name: "safari_mac", UA: "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 Safari/605.1.15"},
		{Name: "chrome_android", UA: "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/120.0.0.0 Mobile"},
		{Name: "safari_ios", UA: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15"},
		{Name: "bot_google", UA: "Googlebot/2.1 (+http://www.google.com/bot.html)"},
	}
}

// getDefaultProbePool 默认探测池
func getDefaultProbePool() []string {
	return []string{
		"https://www.google.com/generate_204",
		"https://www.gstatic.com/generate_204",
		"https://cp.cloudflare.com/",
		"https://www.apple.com/library/test/success.html",
		"https://detectportal.firefox.com/success.txt",
	}
}

// SetCallbacks 设置回调
func (us *UniversalSensor) SetCallbacks(
	onDrop func(string, float64, string),
	onHijack func(string, *HijackingSignature),
) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.onReputationDrop = onDrop
	us.onHijackDetected = onHijack
}

// Start 启动传感器
func (us *UniversalSensor) Start() {
	us.wg.Add(1)
	go us.probeLoop()
	log.Println("🌐 通用业务回显探测器已启动")
}

// Stop 停止传感器
func (us *UniversalSensor) Stop() {
	us.cancel()
	us.wg.Wait()
	log.Println("🛑 通用传感器已停止")
}

// RegisterDomain 注册域名监控
func (us *UniversalSensor) RegisterDomain(domain string) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	if _, exists := us.reputations[domain]; exists {
		return nil
	}

	// 建立基线
	baseline, err := us.establishBaseline(domain)
	if err != nil {
		return fmt.Errorf("建立基线失败: %w", err)
	}

	us.reputations[domain] = &DomainReputation{
		Domain:          domain,
		Score:           100.0,
		BaselineHash:    baseline.BodyHash,
		BaselineLength:  baseline.ContentLength,
		BaselineEntropy: baseline.BodyEntropy,
		LastCheck:       time.Now(),
		RegionalScores:  make(map[string]float64),
	}

	log.Printf("📝 域名已注册监控: %s (baseline: %s)", domain, baseline.BodyHash[:16])
	return nil
}

// establishBaseline 建立基线
func (us *UniversalSensor) establishBaseline(domain string) (*ResponseFingerprint, error) {
	url := fmt.Sprintf("https://%s/", domain)
	return us.probeURL(url, us.userAgents[0].UA)
}

// probeLoop 探测循环
func (us *UniversalSensor) probeLoop() {
	defer us.wg.Done()

	ticker := time.NewTicker(us.probeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-us.ctx.Done():
			return
		case <-ticker.C:
			us.probeAllDomains()
		}
	}
}

// probeAllDomains 探测所有域名
func (us *UniversalSensor) probeAllDomains() {
	us.mu.RLock()
	domains := make([]string, 0, len(us.reputations))
	for domain := range us.reputations {
		domains = append(domains, domain)
	}
	us.mu.RUnlock()

	for _, domain := range domains {
		us.crossValidateDomain(domain)
	}
}

// crossValidateDomain 交叉验证域名
func (us *UniversalSensor) crossValidateDomain(domain string) {
	us.mu.RLock()
	rep, exists := us.reputations[domain]
	if !exists {
		us.mu.RUnlock()
		return
	}
	baseline := rep.BaselineHash
	baselineLen := rep.BaselineLength
	baselineEntropy := rep.BaselineEntropy
	us.mu.RUnlock()

	url := fmt.Sprintf("https://%s/", domain)
	results := make(map[string]*ResponseFingerprint)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 多 UA 探测
	for _, ua := range us.userAgents {
		wg.Add(1)
		go func(uaProfile *UserAgentProfile) {
			defer wg.Done()
			fp, err := us.probeURL(url, uaProfile.UA)
			if err != nil {
				fp = &ResponseFingerprint{StatusCode: -1}
			}
			mu.Lock()
			results[uaProfile.Name] = fp
			mu.Unlock()
		}(ua)
	}

	wg.Wait()

	// 分析结果
	us.analyzeProbeResults(domain, baseline, baselineLen, baselineEntropy, results)
}

// probeURL 探测 URL
func (us *UniversalSensor) probeURL(url, userAgent string) (*ResponseFingerprint, error) {
	req, err := http.NewRequestWithContext(us.ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := us.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体（限制大小）
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	// 计算指纹
	fp := &ResponseFingerprint{
		ContentLength: resp.ContentLength,
		ContentType:   resp.Header.Get("Content-Type"),
		StatusCode:    resp.StatusCode,
		HeaderHash:    us.hashHeaders(resp.Header),
		BodyHash:      us.hashBody(body),
		BodyEntropy:   us.calculateEntropy(body),
		Timestamp:     time.Now(),
	}

	return fp, nil
}

// hashHeaders 计算头部哈希
func (us *UniversalSensor) hashHeaders(headers http.Header) string {
	// 只哈希关键头部
	keyHeaders := []string{"Content-Type", "Server", "X-Powered-By"}
	var buf bytes.Buffer
	for _, key := range keyHeaders {
		if val := headers.Get(key); val != "" {
			buf.WriteString(key + ":" + val + "\n")
		}
	}
	hash := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(hash[:8])
}

// hashBody 计算响应体哈希
func (us *UniversalSensor) hashBody(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

// calculateEntropy 计算信息熵
func (us *UniversalSensor) calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	// 统计字节频率
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}

	// 计算香农熵
	var entropy float64
	total := float64(len(data))
	for _, count := range freq {
		p := float64(count) / total
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

// analyzeProbeResults 分析探测结果
func (us *UniversalSensor) analyzeProbeResults(
	domain, baseline string,
	baselineLen int64,
	baselineEntropy float64,
	results map[string]*ResponseFingerprint,
) {
	us.mu.Lock()
	defer us.mu.Unlock()

	rep, exists := us.reputations[domain]
	if !exists {
		return
	}

	successCount := 0
	hijackCount := 0
	failCount := 0

	for uaName, fp := range results {
		if fp.StatusCode == -1 {
			failCount++
			continue
		}

		if fp.StatusCode >= 400 {
			failCount++
			continue
		}

		successCount++

		// 检测劫持
		if sig := us.detectHijacking(domain, baseline, baselineLen, baselineEntropy, fp, uaName); sig != nil {
			hijackCount++
			rep.Signatures = append(rep.Signatures, sig)
			rep.HijackCount++

			// 触发回调
			if us.onHijackDetected != nil {
				go us.onHijackDetected(domain, sig)
			}
		}
	}

	// 更新信誉分
	totalProbes := len(results)
	if totalProbes > 0 {
		// 基础分数衰减
		rep.Score *= us.reputationDecay

		// 成功率加分
		successRate := float64(successCount) / float64(totalProbes)
		rep.Score += successRate * 10

		// 劫持扣分
		if hijackCount > 0 {
			rep.Score -= float64(hijackCount) * 15
		}

		// 失败扣分
		if failCount > 0 {
			rep.Score -= float64(failCount) * 5
			rep.FailCount += failCount
		}

		// 限制范围
		if rep.Score > 100 {
			rep.Score = 100
		}
		if rep.Score < 0 {
			rep.Score = 0
		}
	}

	rep.LastCheck = time.Now()

	// 信誉度下降告警
	if rep.Score < 50 && us.onReputationDrop != nil {
		reason := fmt.Sprintf("fail=%d, hijack=%d", failCount, hijackCount)
		go us.onReputationDrop(domain, rep.Score, reason)
	}

	log.Printf("📊 域名信誉更新: %s score=%.1f (success=%d, fail=%d, hijack=%d)",
		domain, rep.Score, successCount, failCount, hijackCount)
}

// detectHijacking 检测劫持
func (us *UniversalSensor) detectHijacking(
	domain, baseline string,
	baselineLen int64,
	baselineEntropy float64,
	fp *ResponseFingerprint,
	uaName string,
) *HijackingSignature {
	// 内容长度异常
	if baselineLen > 0 && fp.ContentLength > 0 {
		lengthDelta := math.Abs(float64(fp.ContentLength-baselineLen)) / float64(baselineLen)
		if lengthDelta > us.hijackThreshold {
			return &HijackingSignature{
				Type:           "content_injection",
				ExpectedLength: baselineLen,
				ActualLength:   fp.ContentLength,
				DetectedAt:     time.Now(),
				DetectedRegion: uaName,
			}
		}
	}

	// 内容类型异常
	if fp.ContentType != "" && !us.isExpectedContentType(fp.ContentType) {
		return &HijackingSignature{
			Type:         "content_type_change",
			ExpectedType: "application/json or text/plain",
			ActualType:   fp.ContentType,
			DetectedAt:   time.Now(),
			DetectedRegion: uaName,
		}
	}

	// 熵值异常（注入了大量 HTML）
	if baselineEntropy > 0 {
		entropyDelta := math.Abs(fp.BodyEntropy - baselineEntropy)
		if entropyDelta > 2.0 { // 熵值变化超过 2
			return &HijackingSignature{
				Type:         "entropy_anomaly",
				EntropyDelta: entropyDelta,
				DetectedAt:   time.Now(),
				DetectedRegion: uaName,
			}
		}
	}

	// 哈希不匹配
	if baseline != "" && fp.BodyHash != baseline {
		return &HijackingSignature{
			Type:         "content_modified",
			DetectedAt:   time.Now(),
			DetectedRegion: uaName,
		}
	}

	return nil
}

// isExpectedContentType 检查内容类型
func (us *UniversalSensor) isExpectedContentType(contentType string) bool {
	expected := []string{
		"application/json",
		"text/plain",
		"application/octet-stream",
		"text/html", // 某些 App 返回 HTML
	}
	for _, e := range expected {
		if bytes.Contains([]byte(contentType), []byte(e)) {
			return true
		}
	}
	return false
}

// GetReputation 获取域名信誉
func (us *UniversalSensor) GetReputation(domain string) *DomainReputation {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.reputations[domain]
}

// GetAllReputations 获取所有域名信誉
func (us *UniversalSensor) GetAllReputations() map[string]*DomainReputation {
	us.mu.RLock()
	defer us.mu.RUnlock()

	result := make(map[string]*DomainReputation)
	for k, v := range us.reputations {
		result[k] = v
	}
	return result
}

// GetLowReputationDomains 获取低信誉域名
func (us *UniversalSensor) GetLowReputationDomains(threshold float64) []string {
	us.mu.RLock()
	defer us.mu.RUnlock()

	var domains []string
	for domain, rep := range us.reputations {
		if rep.Score < threshold {
			domains = append(domains, domain)
		}
	}
	return domains
}
