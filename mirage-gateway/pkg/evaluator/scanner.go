// Package evaluator - AI 审计评估器
package evaluator

import (
	"context"
	"log"
	"math"
	"sync"
	"time"
)

// ScanResult 扫描结果
type ScanResult struct {
	Timestamp      time.Time
	Confidence     float64 // 0-100
	Classification string
	Features       map[string]float64
	Anomalies      []string
}

// Scanner AI 审计评估器
type Scanner struct {
	mu              sync.RWMutex
	results         []ScanResult
	threshold       float64
	feedbackChannel chan<- FeedbackSignal
	ctx             context.Context
	cancel          context.CancelFunc
}

// FeedbackSignal 反馈信号
type FeedbackSignal struct {
	Type       string
	Confidence float64
	Action     string
	Params     map[string]interface{}
}

// NewScanner 创建扫描器
func NewScanner(threshold float64, feedbackCh chan<- FeedbackSignal) *Scanner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scanner{
		results:         make([]ScanResult, 0, 1000),
		threshold:       threshold,
		feedbackChannel: feedbackCh,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start 启动扫描器
func (s *Scanner) Start() {
	log.Println("[Scanner] 启动 AI 审计评估器")
	go s.continuousScan()
}

// Stop 停止扫描器
func (s *Scanner) Stop() {
	log.Println("[Scanner] 停止扫描器")
	s.cancel()
}

// continuousScan 持续扫描
func (s *Scanner) continuousScan() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.performScan()
		}
	}
}

// performScan 执行扫描
func (s *Scanner) performScan() {
	// 1. 捕获流量特征
	features := s.captureFeatures()
	
	// 2. 计算异常分数
	confidence := s.calculateAnomalyScore(features)
	
	// 3. 分类
	classification := s.classify(features, confidence)
	
	// 4. 检测异常
	anomalies := s.detectAnomalies(features)
	
	result := ScanResult{
		Timestamp:      time.Now(),
		Confidence:     confidence,
		Classification: classification,
		Features:       features,
		Anomalies:      anomalies,
	}
	
	// 5. 保存结果
	s.mu.Lock()
	s.results = append(s.results, result)
	if len(s.results) > 1000 {
		s.results = s.results[1:]
	}
	s.mu.Unlock()
	
	log.Printf("[Scanner] 扫描完成: 置信度=%.2f%%, 分类=%s", confidence, classification)
	
	// 6. 触发反馈
	if confidence > s.threshold {
		s.triggerFeedback(result)
	}
}

// captureFeatures 捕获流量特征
func (s *Scanner) captureFeatures() map[string]float64 {
	// TODO: 实际实现需要从 eBPF 或 pcap 捕获
	// 这里使用模拟数据
	
	features := map[string]float64{
		"packet_size_mean":     512.5,
		"packet_size_std":      128.3,
		"iat_mean":             45.2,  // 毫秒
		"iat_std":              12.8,
		"entropy":              7.85,  // 0-8
		"tls_version":          1.3,
		"cipher_suite_count":   7,
		"extension_count":      11,
		"tcp_window_size":      65535,
		"burst_size":           5,
		"flow_duration":        120.5, // 秒
	}
	
	return features
}

// calculateAnomalyScore 计算异常分数
func (s *Scanner) calculateAnomalyScore(features map[string]float64) float64 {
	// 使用简化的异常检测算法
	// 实际应该使用 ML 模型（随机森林/CNN）
	
	score := 0.0
	
	// 1. 熵检查（加密流量熵应该接近 8）
	entropy := features["entropy"]
	if entropy < 7.5 || entropy > 7.99 {
		score += 15.0
	}
	
	// 2. IAT 规律性检查
	iatMean := features["iat_mean"]
	iatStd := features["iat_std"]
	cv := iatStd / iatMean // 变异系数
	if cv < 0.1 || cv > 0.5 {
		score += 20.0
	}
	
	// 3. 包大小分布
	sizeMean := features["packet_size_mean"]
	sizeStd := features["packet_size_std"]
	if sizeStd < 50 || sizeMean > 1400 {
		score += 10.0
	}
	
	// 4. TLS 指纹异常
	cipherCount := features["cipher_suite_count"]
	extCount := features["extension_count"]
	if cipherCount < 5 || extCount < 8 {
		score += 25.0
	}
	
	// 5. 突发模式
	burstSize := features["burst_size"]
	if burstSize > 10 || burstSize < 2 {
		score += 10.0
	}
	
	return math.Min(score, 100.0)
}

// classify 分类
func (s *Scanner) classify(features map[string]float64, confidence float64) string {
	if confidence < 20 {
		return "normal_traffic"
	} else if confidence < 50 {
		return "suspicious_traffic"
	} else if confidence < 80 {
		return "likely_vpn"
	} else {
		return "encrypted_tunnel"
	}
}

// detectAnomalies 检测异常
func (s *Scanner) detectAnomalies(features map[string]float64) []string {
	anomalies := make([]string, 0)
	
	// 熵异常
	if entropy := features["entropy"]; entropy > 7.95 {
		anomalies = append(anomalies, "high_entropy")
	}
	
	// IAT 规律性异常
	iatMean := features["iat_mean"]
	iatStd := features["iat_std"]
	if cv := iatStd / iatMean; cv < 0.15 {
		anomalies = append(anomalies, "regular_iat_pattern")
	}
	
	// 包大小异常
	if sizeMean := features["packet_size_mean"]; sizeMean > 1200 {
		anomalies = append(anomalies, "large_packet_size")
	}
	
	// TLS 指纹异常
	if cipherCount := features["cipher_suite_count"]; cipherCount < 6 {
		anomalies = append(anomalies, "limited_cipher_suites")
	}
	
	return anomalies
}

// triggerFeedback 触发反馈
func (s *Scanner) triggerFeedback(result ScanResult) {
	log.Printf("[Scanner] ⚠️ 检测到异常流量 (置信度: %.2f%%), 触发反馈", result.Confidence)
	
	signal := FeedbackSignal{
		Type:       "anomaly_detected",
		Confidence: result.Confidence,
		Action:     "adjust_parameters",
		Params: map[string]interface{}{
			"anomalies":      result.Anomalies,
			"classification": result.Classification,
		},
	}
	
	// 非阻塞发送
	select {
	case s.feedbackChannel <- signal:
		log.Println("[Scanner] 反馈信号已发送")
	default:
		log.Println("[Scanner] ⚠️ 反馈通道已满，跳过")
	}
}

// GetRecentResults 获取最近结果
func (s *Scanner) GetRecentResults(count int) []ScanResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if count > len(s.results) {
		count = len(s.results)
	}
	
	start := len(s.results) - count
	return s.results[start:]
}

// GetAverageConfidence 获取平均置信度
func (s *Scanner) GetAverageConfidence(duration time.Duration) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	cutoff := time.Now().Add(-duration)
	sum := 0.0
	count := 0
	
	for i := len(s.results) - 1; i >= 0; i-- {
		if s.results[i].Timestamp.Before(cutoff) {
			break
		}
		sum += s.results[i].Confidence
		count++
	}
	
	if count == 0 {
		return 0
	}
	
	return sum / float64(count)
}

// GetStats 获取统计信息
func (s *Scanner) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if len(s.results) == 0 {
		return map[string]interface{}{
			"total_scans": 0,
		}
	}
	
	// 计算统计
	totalScans := len(s.results)
	avgConfidence := 0.0
	maxConfidence := 0.0
	anomalyCount := 0
	
	for _, result := range s.results {
		avgConfidence += result.Confidence
		if result.Confidence > maxConfidence {
			maxConfidence = result.Confidence
		}
		if len(result.Anomalies) > 0 {
			anomalyCount++
		}
	}
	
	avgConfidence /= float64(totalScans)
	
	return map[string]interface{}{
		"total_scans":      totalScans,
		"avg_confidence":   avgConfidence,
		"max_confidence":   maxConfidence,
		"anomaly_count":    anomalyCount,
		"anomaly_rate":     float64(anomalyCount) / float64(totalScans) * 100,
		"last_scan":        s.results[len(s.results)-1].Timestamp,
	}
}
