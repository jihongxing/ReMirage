// Package health - App 模拟探测器
// 模拟主流 App 的业务特征，主动诱导并分析干扰行为
package health

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// ProbeResult 探测结果
type ProbeResult struct {
	AppName       string        `json:"app_name"`
	Success       bool          `json:"success"`
	RTT           time.Duration `json:"rtt"`
	ErrorType     string        `json:"error_type,omitempty"`
	BlockingType  string        `json:"blocking_type,omitempty"`
	ResponseCode  int           `json:"response_code,omitempty"`
	Timestamp     time.Time     `json:"timestamp"`
}

// AppSimulator App 模拟探测器
type AppSimulator struct {
	mu sync.RWMutex

	// 配置
	region        string
	probeInterval time.Duration
	timeout       time.Duration

	// 探测目标
	targetApps    []*AppProfile
	probeResults  map[string]*ProbeResult
	anomalyScores map[string]float64

	// HTTP 客户端
	httpClient *http.Client

	// 回调
	onAnomalyDetected func(appName string, blockingType string)

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAppSimulator 创建 App 模拟器
func NewAppSimulator(region string) *AppSimulator {
	ctx, cancel := context.WithCancel(context.Background())

	// 自定义 TLS 配置
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        100,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	return &AppSimulator{
		region:        region,
		probeInterval: 30 * time.Second,
		timeout:       10 * time.Second,
		targetApps:    GetRegionalApps(region, 0.5),
		probeResults:  make(map[string]*ProbeResult),
		anomalyScores: make(map[string]float64),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetAnomalyCallback 设置异常回调
func (as *AppSimulator) SetAnomalyCallback(callback func(string, string)) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.onAnomalyDetected = callback
}

// Start 启动模拟器
func (as *AppSimulator) Start() {
	as.wg.Add(1)
	go as.probeLoop()
	log.Printf("🎭 App 模拟探测器已启动 (区域: %s, 目标: %d 个 App)", as.region, len(as.targetApps))
}

// Stop 停止模拟器
func (as *AppSimulator) Stop() {
	as.cancel()
	as.wg.Wait()
	log.Println("🛑 App 模拟探测器已停止")
}

// probeLoop 探测循环
func (as *AppSimulator) probeLoop() {
	defer as.wg.Done()

	// 初始探测
	as.probeAllApps()

	ticker := time.NewTicker(as.probeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-as.ctx.Done():
			return
		case <-ticker.C:
			as.probeAllApps()
		}
	}
}

// probeAllApps 探测所有目标 App
func (as *AppSimulator) probeAllApps() {
	var wg sync.WaitGroup

	for _, app := range as.targetApps {
		wg.Add(1)
		go func(profile *AppProfile) {
			defer wg.Done()
			result := as.probeApp(profile)
			as.recordResult(result)
		}(app)
	}

	wg.Wait()
	as.analyzeAnomalies()
}

// probeApp 探测单个 App
func (as *AppSimulator) probeApp(profile *AppProfile) *ProbeResult {
	result := &ProbeResult{
		AppName:   profile.Name,
		Timestamp: time.Now(),
	}

	start := time.Now()

	switch profile.Category {
	case CategoryMessaging:
		as.probeMessagingApp(profile, result)
	case CategoryVideo:
		as.probeVideoApp(profile, result)
	case CategoryVoIP:
		as.probeVoIPApp(profile, result)
	default:
		as.probeGenericApp(profile, result)
	}

	result.RTT = time.Since(start)
	return result
}

// probeMessagingApp 探测即时通讯 App
func (as *AppSimulator) probeMessagingApp(profile *AppProfile, result *ProbeResult) {
	// 模拟协议握手
	if len(profile.Domains) == 0 {
		result.ErrorType = "no_domain"
		return
	}

	domain := profile.Domains[0]
	port := profile.Ports[0]
	addr := net.JoinHostPort(domain, fmt.Sprintf("%d", port))

	conn, err := net.DialTimeout("tcp", addr, as.timeout)
	if err != nil {
		result.ErrorType = "connect_failed"
		result.BlockingType = as.classifyError(err)
		return
	}
	defer conn.Close()

	// 发送特征握手包
	if len(profile.HandshakePattern) > 0 {
		// 构造模拟握手包
		handshake := as.buildHandshakePacket(profile)
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err = conn.Write(handshake)
		if err != nil {
			result.ErrorType = "write_failed"
			result.BlockingType = as.classifyError(err)
			return
		}

		// 读取响应
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			result.ErrorType = "read_failed"
			result.BlockingType = as.classifyError(err)
			return
		}

		// 检查是否为封锁响应
		if blockType := as.detectBlockingResponse(buf[:n]); blockType != "" {
			result.BlockingType = blockType
			return
		}
	}

	result.Success = true
}

// probeVideoApp 探测视频 App
func (as *AppSimulator) probeVideoApp(profile *AppProfile, result *ProbeResult) {
	if len(profile.Domains) == 0 {
		result.ErrorType = "no_domain"
		return
	}

	// 模拟 Range 请求（视频片段）
	url := fmt.Sprintf("https://%s/", profile.Domains[0])
	req, _ := http.NewRequestWithContext(as.ctx, "GET", url, nil)
	
	// 添加视频流特征头
	req.Header.Set("Range", "bytes=0-1048576")
	req.Header.Set("Accept", "video/webm,video/mp4,video/*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := as.httpClient.Do(req)
	if err != nil {
		result.ErrorType = "request_failed"
		result.BlockingType = as.classifyError(err)
		return
	}
	defer resp.Body.Close()

	result.ResponseCode = resp.StatusCode

	// 检查响应
	if resp.StatusCode == 403 {
		result.BlockingType = "http_403_inject"
		return
	}
	if resp.StatusCode == 302 || resp.StatusCode == 301 {
		location := resp.Header.Get("Location")
		if as.isSuspiciousRedirect(location) {
			result.BlockingType = "http_302_redirect"
			return
		}
	}

	// 读取部分响应检查内容
	body := make([]byte, 1024)
	n, _ := io.ReadAtLeast(resp.Body, body, 100)
	if n > 0 {
		if blockType := as.detectBlockingResponse(body[:n]); blockType != "" {
			result.BlockingType = blockType
			return
		}
	}

	result.Success = true
}

// probeVoIPApp 探测 VoIP App
func (as *AppSimulator) probeVoIPApp(profile *AppProfile, result *ProbeResult) {
	if len(profile.Domains) == 0 || len(profile.Ports) < 2 {
		result.ErrorType = "no_domain"
		return
	}

	// VoIP 通常使用 UDP
	domain := profile.Domains[0]
	port := profile.Ports[1] // 通常第二个端口是媒体端口

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", domain, port))
	if err != nil {
		result.ErrorType = "resolve_failed"
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		result.ErrorType = "connect_failed"
		result.BlockingType = as.classifyError(err)
		return
	}
	defer conn.Close()

	// 发送模拟 RTP 包
	rtpPacket := as.buildRTPPacket()
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(rtpPacket)
	if err != nil {
		result.ErrorType = "write_failed"
		result.BlockingType = as.classifyError(err)
		return
	}

	// 尝试接收响应（可能没有）
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	_, err = conn.Read(buf)
	if err != nil {
		// UDP 无响应不一定是封锁
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			result.Success = true // 超时可能是正常的
			return
		}
		result.ErrorType = "read_failed"
		return
	}

	result.Success = true
}

// probeGenericApp 通用探测
func (as *AppSimulator) probeGenericApp(profile *AppProfile, result *ProbeResult) {
	if len(profile.Domains) == 0 {
		result.ErrorType = "no_domain"
		return
	}

	url := fmt.Sprintf("https://%s/", profile.Domains[0])
	req, _ := http.NewRequestWithContext(as.ctx, "HEAD", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := as.httpClient.Do(req)
	if err != nil {
		result.ErrorType = "request_failed"
		result.BlockingType = as.classifyError(err)
		return
	}
	defer resp.Body.Close()

	result.ResponseCode = resp.StatusCode

	if resp.StatusCode >= 400 {
		result.BlockingType = fmt.Sprintf("http_%d", resp.StatusCode)
		return
	}

	result.Success = true
}

// buildHandshakePacket 构造握手包
func (as *AppSimulator) buildHandshakePacket(profile *AppProfile) []byte {
	// 基于 App 特征构造握手包
	packet := make([]byte, 64)
	copy(packet, profile.HandshakePattern)
	
	// 添加随机填充
	rand.Read(packet[len(profile.HandshakePattern):])
	
	return packet
}

// buildRTPPacket 构造 RTP 包
func (as *AppSimulator) buildRTPPacket() []byte {
	// RTP 头部 (12 字节) + 随机载荷
	packet := make([]byte, 172)
	
	// RTP Version 2, Padding, Extension, CSRC count
	packet[0] = 0x80
	// Marker, Payload Type (动态)
	packet[1] = 0x60
	// Sequence Number
	rand.Read(packet[2:4])
	// Timestamp
	rand.Read(packet[4:8])
	// SSRC
	rand.Read(packet[8:12])
	// 随机载荷
	rand.Read(packet[12:])
	
	return packet
}

// classifyError 分类错误
func (as *AppSimulator) classifyError(err error) string {
	errStr := err.Error()
	
	if bytes.Contains([]byte(errStr), []byte("connection refused")) {
		return "connection_refused"
	}
	if bytes.Contains([]byte(errStr), []byte("connection reset")) {
		return "rst_injection"
	}
	if bytes.Contains([]byte(errStr), []byte("timeout")) {
		return "timeout"
	}
	if bytes.Contains([]byte(errStr), []byte("no route")) {
		return "no_route"
	}
	if bytes.Contains([]byte(errStr), []byte("network unreachable")) {
		return "network_unreachable"
	}
	
	return "unknown"
}

// detectBlockingResponse 检测封锁响应
func (as *AppSimulator) detectBlockingResponse(data []byte) string {
	for _, sig := range KnownBlockingSignatures {
		if bytes.Contains(data, sig.Pattern) {
			return sig.Type
		}
	}
	return ""
}

// isSuspiciousRedirect 检测可疑重定向
func (as *AppSimulator) isSuspiciousRedirect(location string) bool {
	suspiciousPatterns := []string{
		"warning", "block", "forbidden", "denied",
		"gov.", "police", "security",
	}
	
	for _, pattern := range suspiciousPatterns {
		if bytes.Contains([]byte(location), []byte(pattern)) {
			return true
		}
	}
	return false
}

// recordResult 记录结果
func (as *AppSimulator) recordResult(result *ProbeResult) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.probeResults[result.AppName] = result
}

// analyzeAnomalies 分析异常
func (as *AppSimulator) analyzeAnomalies() {
	as.mu.Lock()
	defer as.mu.Unlock()

	for appName, result := range as.probeResults {
		profile := GetAppProfile(appName)
		if profile == nil {
			continue
		}

		score := as.calculateAnomalyScore(result, profile)
		as.anomalyScores[appName] = score

		// 触发回调
		if score > 0.7 && result.BlockingType != "" {
			callback := as.onAnomalyDetected
			if callback != nil {
				go callback(appName, result.BlockingType)
			}
			log.Printf("🚨 检测到 %s 异常: type=%s, score=%.2f", 
				appName, result.BlockingType, score)
		}
	}
}

// calculateAnomalyScore 计算异常分数
func (as *AppSimulator) calculateAnomalyScore(result *ProbeResult, profile *AppProfile) float64 {
	if result.Success {
		// 检查 RTT 是否异常
		if result.RTT > profile.RTTTolerance*2 {
			return 0.5 // 延迟异常
		}
		return 0.0
	}

	// 根据错误类型评分
	switch result.BlockingType {
	case "rst_injection":
		return 1.0 // 明确的封锁
	case "http_403_inject":
		return 0.95
	case "http_302_redirect":
		return 0.9
	case "tcp_zero_window":
		return 0.85
	case "connection_refused":
		return 0.7
	case "timeout":
		return 0.5
	default:
		return 0.3
	}
}

// GetAnomalyScores 获取异常分数
func (as *AppSimulator) GetAnomalyScores() map[string]float64 {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	scores := make(map[string]float64)
	for k, v := range as.anomalyScores {
		scores[k] = v
	}
	return scores
}

// GetProbeResults 获取探测结果
func (as *AppSimulator) GetProbeResults() map[string]*ProbeResult {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	results := make(map[string]*ProbeResult)
	for k, v := range as.probeResults {
		results[k] = v
	}
	return results
}

// GetBlockedApps 获取被封锁的 App 列表
func (as *AppSimulator) GetBlockedApps() []string {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	var blocked []string
	for appName, score := range as.anomalyScores {
		if score > 0.7 {
			blocked = append(blocked, appName)
		}
	}
	return blocked
}
