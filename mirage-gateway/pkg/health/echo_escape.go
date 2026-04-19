// Package health - 回声逃逸测试
// 在触发 G-Switch 前，最后确认是否为"全域封锁"
package health

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// EscapeVerdict 逃逸判定结果
type EscapeVerdict int

const (
	VerdictUnknown       EscapeVerdict = 0
	VerdictDomainBlocked EscapeVerdict = 1 // 域名被定点封锁 → G-Switch
	VerdictProtocolBlock EscapeVerdict = 2 // 协议栈被识别 → B-DNA Reset
	VerdictRegionalBlock EscapeVerdict = 3 // 区域性断网
	VerdictAllClear      EscapeVerdict = 4 // 全部通畅
)

// DarkChannel 暗通道
type DarkChannel struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` // cdn, direct, tor, ipfs
	Endpoint string   `json:"endpoint"`
	Priority int      `json:"priority"`
}

// VerificationResult 验证结果
type VerificationResult struct {
	Channel     *DarkChannel  `json:"channel"`
	Success     bool          `json:"success"`
	RTT         time.Duration `json:"rtt"`
	ErrorType   string        `json:"error_type,omitempty"`
	Timestamp   time.Time     `json:"timestamp"`
}

// EscapeVerifier 逃逸验证器
type EscapeVerifier struct {
	mu sync.RWMutex

	// 暗通道列表
	darkChannels []*DarkChannel

	// 验证结果
	results map[string]*VerificationResult

	// HTTP 客户端
	httpClient *http.Client

	// 回调
	onVerdictReady func(EscapeVerdict, DamageType)

	// 配置
	timeout       time.Duration
	retryCount    int
	parallelProbe bool

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewEscapeVerifier 创建逃逸验证器
func NewEscapeVerifier() *EscapeVerifier {
	ctx, cancel := context.WithCancel(context.Background())

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        50,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}

	return &EscapeVerifier{
		darkChannels: getDefaultDarkChannels(),
		results:      make(map[string]*VerificationResult),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
		timeout:       10 * time.Second,
		retryCount:    2,
		parallelProbe: true,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// getDefaultDarkChannels 获取默认暗通道
func getDefaultDarkChannels() []*DarkChannel {
	return []*DarkChannel{
		// CDN 边缘节点
		{Name: "cloudflare_cdn", Type: "cdn", Endpoint: "https://1.1.1.1/cdn-cgi/trace", Priority: 1},
		{Name: "google_cdn", Type: "cdn", Endpoint: "https://www.gstatic.com/generate_204", Priority: 2},
		{Name: "akamai_cdn", Type: "cdn", Endpoint: "https://www.akamai.com/", Priority: 3},
		
		// 直连测试
		{Name: "cloudflare_direct", Type: "direct", Endpoint: "https://cloudflare.com/", Priority: 4},
		{Name: "fastly_direct", Type: "direct", Endpoint: "https://www.fastly.com/", Priority: 5},
		
		// 备用 DNS
		{Name: "google_dns", Type: "dns", Endpoint: "8.8.8.8:53", Priority: 6},
		{Name: "cloudflare_dns", Type: "dns", Endpoint: "1.1.1.1:53", Priority: 7},
	}
}

// SetVerdictCallback 设置判定回调
func (ev *EscapeVerifier) SetVerdictCallback(callback func(EscapeVerdict, DamageType)) {
	ev.mu.Lock()
	defer ev.mu.Unlock()
	ev.onVerdictReady = callback
}

// AddDarkChannel 添加暗通道
func (ev *EscapeVerifier) AddDarkChannel(channel *DarkChannel) {
	ev.mu.Lock()
	defer ev.mu.Unlock()
	ev.darkChannels = append(ev.darkChannels, channel)
}

// VerifyBeforeEscape 逃逸前验证
func (ev *EscapeVerifier) VerifyBeforeEscape(
	blockedApp string,
	damageType DamageType,
) (EscapeVerdict, *EscapeAction) {
	log.Printf("🔍 开始回声逃逸验证: app=%s, damage=%d", blockedApp, damageType)

	// 并行探测所有暗通道
	results := ev.probeAllChannels()

	// 分析结果
	verdict := ev.analyzeResults(results, damageType)

	// 生成逃逸动作
	action := ev.generateAction(verdict, damageType)

	// 触发回调
	ev.mu.RLock()
	callback := ev.onVerdictReady
	ev.mu.RUnlock()
	if callback != nil {
		go callback(verdict, damageType)
	}

	log.Printf("✅ 回声逃逸验证完成: verdict=%d, action=%s", verdict, action.Type)
	return verdict, action
}

// probeAllChannels 探测所有暗通道
func (ev *EscapeVerifier) probeAllChannels() map[string]*VerificationResult {
	ev.mu.RLock()
	channels := ev.darkChannels
	ev.mu.RUnlock()

	results := make(map[string]*VerificationResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, channel := range channels {
		wg.Add(1)
		go func(ch *DarkChannel) {
			defer wg.Done()
			result := ev.probeChannel(ch)
			mu.Lock()
			results[ch.Name] = result
			mu.Unlock()
		}(channel)
	}

	wg.Wait()
	return results
}

// probeChannel 探测单个暗通道
func (ev *EscapeVerifier) probeChannel(channel *DarkChannel) *VerificationResult {
	result := &VerificationResult{
		Channel:   channel,
		Timestamp: time.Now(),
	}

	var err error
	start := time.Now()

	switch channel.Type {
	case "cdn", "direct":
		err = ev.probeHTTP(channel.Endpoint)
	case "dns":
		err = ev.probeDNS(channel.Endpoint)
	default:
		err = ev.probeHTTP(channel.Endpoint)
	}

	result.RTT = time.Since(start)

	if err != nil {
		result.Success = false
		result.ErrorType = ev.classifyError(err)
	} else {
		result.Success = true
	}

	return result
}

// probeHTTP HTTP 探测
func (ev *EscapeVerifier) probeHTTP(endpoint string) error {
	req, err := http.NewRequestWithContext(ev.ctx, "HEAD", endpoint, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := ev.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("http_%d", resp.StatusCode)
	}

	return nil
}

// probeDNS DNS 探测
func (ev *EscapeVerifier) probeDNS(endpoint string) error {
	conn, err := net.DialTimeout("udp", endpoint, ev.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 发送简单 DNS 查询 (A 记录 for example.com)
	query := []byte{
		0x00, 0x01, // Transaction ID
		0x01, 0x00, // Flags: Standard query
		0x00, 0x01, // Questions: 1
		0x00, 0x00, // Answer RRs: 0
		0x00, 0x00, // Authority RRs: 0
		0x00, 0x00, // Additional RRs: 0
		// Query: example.com
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm',
		0x00,       // Null terminator
		0x00, 0x01, // Type: A
		0x00, 0x01, // Class: IN
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(query)
	if err != nil {
		return err
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 512)
	_, err = conn.Read(buf)
	return err
}

// classifyError 分类错误
func (ev *EscapeVerifier) classifyError(err error) string {
	errStr := err.Error()

	switch {
	case contains(errStr, "connection refused"):
		return "connection_refused"
	case contains(errStr, "connection reset"):
		return "rst_injection"
	case contains(errStr, "timeout"):
		return "timeout"
	case contains(errStr, "no route"):
		return "no_route"
	case contains(errStr, "network unreachable"):
		return "network_unreachable"
	case contains(errStr, "403"):
		return "http_403"
	default:
		return "unknown"
	}
}

// analyzeResults 分析结果
func (ev *EscapeVerifier) analyzeResults(
	results map[string]*VerificationResult,
	damageType DamageType,
) EscapeVerdict {
	successCount := 0
	failCount := 0
	cdnSuccess := false
	directSuccess := false

	for _, result := range results {
		if result.Success {
			successCount++
			switch result.Channel.Type {
			case "cdn":
				cdnSuccess = true
			case "direct":
				directSuccess = true
			}
		} else {
			failCount++
		}
	}

	totalChannels := len(results)

	// 全部失败 → 区域性断网或协议栈被识别
	if successCount == 0 {
		if damageType == DamageProtocolBlock {
			return VerdictProtocolBlock
		}
		return VerdictRegionalBlock
	}

	// 全部成功 → 可能是误报
	if failCount == 0 {
		return VerdictAllClear
	}

	// CDN 通但直连不通 → 域名被定点封锁
	if cdnSuccess && !directSuccess {
		return VerdictDomainBlocked
	}

	// 大部分失败 → 协议栈被识别
	if float64(failCount)/float64(totalChannels) > 0.7 {
		return VerdictProtocolBlock
	}

	// 部分失败 → 域名被定点封锁
	return VerdictDomainBlocked
}

// EscapeAction 逃逸动作
type EscapeAction struct {
	Type           string `json:"type"`            // gswitch, bdna_reset, wait, none
	ResetJA4       bool   `json:"reset_ja4"`       // 是否重置 JA4 模板
	ResetSNI       bool   `json:"reset_sni"`       // 是否重置 SNI
	ResetProtocol  bool   `json:"reset_protocol"`  // 是否重置协议栈
	WaitDuration   time.Duration `json:"wait_duration"`
	Reason         string `json:"reason"`
}

// generateAction 生成逃逸动作
func (ev *EscapeVerifier) generateAction(
	verdict EscapeVerdict,
	damageType DamageType,
) *EscapeAction {
	switch verdict {
	case VerdictDomainBlocked:
		return &EscapeAction{
			Type:     "gswitch",
			ResetSNI: true,
			ResetJA4: damageType == DamageProtocolBlock,
			Reason:   "域名被定点封锁，执行域名转生",
		}

	case VerdictProtocolBlock:
		return &EscapeAction{
			Type:          "bdna_reset",
			ResetJA4:      true,
			ResetSNI:      true,
			ResetProtocol: true,
			Reason:        "协议栈被深度指纹识别，执行全面重塑",
		}

	case VerdictRegionalBlock:
		return &EscapeAction{
			Type:         "wait",
			WaitDuration: 5 * time.Minute,
			Reason:       "区域性断网，等待恢复",
		}

	case VerdictAllClear:
		return &EscapeAction{
			Type:   "none",
			Reason: "暗通道全部通畅，可能是误报",
		}

	default:
		return &EscapeAction{
			Type:     "gswitch",
			ResetSNI: true,
			Reason:   "未知情况，保守执行域名转生",
		}
	}
}

// Close 关闭验证器
func (ev *EscapeVerifier) Close() {
	ev.cancel()
}

// GetResults 获取验证结果
func (ev *EscapeVerifier) GetResults() map[string]*VerificationResult {
	ev.mu.RLock()
	defer ev.mu.RUnlock()

	results := make(map[string]*VerificationResult)
	for k, v := range ev.results {
		results[k] = v
	}
	return results
}

// contains 字符串包含检查
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
