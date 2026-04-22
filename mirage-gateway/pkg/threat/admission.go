// Package threat - 多维准入评分器
// CGNAT 感知的多维准入控制，替代粗粒度 IP 限流
package threat

import (
	"math"
	"sync"
	"time"
)

// AdmissionScorer 多维准入评分器
type AdmissionScorer struct {
	mu     sync.RWMutex
	scores map[string]*IPScore
	window time.Duration // 评分窗口（默认 1 分钟）
}

// IPScore 单 IP 多维评分
type IPScore struct {
	NewConnRate      float64   // 新建连接速率（每秒）
	ValidAuthRate    float64   // 有效验证通过率（0-1）
	TokenValidRate   float64   // 会话令牌有效率（0-1）
	ProfileMatchRate float64   // 入口画像匹配率（0-1）
	ActiveSessions   int       // 活跃会话数（CGNAT 感知）
	LastUpdate       time.Time // 最后更新时间

	// 滑动窗口内部计数
	connCount    int
	authSuccess  int
	authTotal    int
	tokenValid   int
	tokenTotal   int
	profileMatch int
	profileTotal int
	windowStart  time.Time
}

// NewAdmissionScorer 创建评分器
func NewAdmissionScorer() *AdmissionScorer {
	return &AdmissionScorer{
		scores: make(map[string]*IPScore),
		window: 60 * time.Second,
	}
}

// getOrCreate 获取或创建 IP 评分（调用方需持有写锁）
func (as *AdmissionScorer) getOrCreate(ip string) *IPScore {
	s, ok := as.scores[ip]
	if !ok {
		s = &IPScore{
			windowStart: time.Now(),
			LastUpdate:  time.Now(),
		}
		as.scores[ip] = s
	}
	// 窗口过期则重置计数
	if time.Since(s.windowStart) > as.window {
		s.connCount = 0
		s.authSuccess = 0
		s.authTotal = 0
		s.tokenValid = 0
		s.tokenTotal = 0
		s.profileMatch = 0
		s.profileTotal = 0
		s.windowStart = time.Now()
	}
	return s
}

// Score 计算综合评分（0-100，越高越可信）
func (s *IPScore) Score() float64 {
	// 计算各维度比率
	if s.authTotal > 0 {
		s.ValidAuthRate = float64(s.authSuccess) / float64(s.authTotal)
	}
	if s.tokenTotal > 0 {
		s.TokenValidRate = float64(s.tokenValid) / float64(s.tokenTotal)
	}
	if s.profileTotal > 0 {
		s.ProfileMatchRate = float64(s.profileMatch) / float64(s.profileTotal)
	}

	// CGNAT 感知：同一 IP 下有多个有效会话时提高可信度
	cgnatBonus := math.Min(float64(s.ActiveSessions)*5, 20)

	// 连接速率惩罚
	connPenalty := math.Min(s.NewConnRate/10, 30)

	score := s.ValidAuthRate*30 + s.TokenValidRate*30 +
		s.ProfileMatchRate*20 + cgnatBonus - connPenalty

	// 钳位到 [0, 100]
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// RecordNewConn 记录新连接
func (as *AdmissionScorer) RecordNewConn(ip string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	s := as.getOrCreate(ip)
	s.connCount++
	elapsed := time.Since(s.windowStart).Seconds()
	if elapsed > 0 {
		s.NewConnRate = float64(s.connCount) / elapsed
	}
	s.LastUpdate = time.Now()
}

// RecordAuthResult 记录验证结果
func (as *AdmissionScorer) RecordAuthResult(ip string, success bool) {
	as.mu.Lock()
	defer as.mu.Unlock()
	s := as.getOrCreate(ip)
	s.authTotal++
	if success {
		s.authSuccess++
	}
	s.LastUpdate = time.Now()
}

// RecordTokenCheck 记录令牌检查结果
func (as *AdmissionScorer) RecordTokenCheck(ip string, valid bool) {
	as.mu.Lock()
	defer as.mu.Unlock()
	s := as.getOrCreate(ip)
	s.tokenTotal++
	if valid {
		s.tokenValid++
	}
	s.LastUpdate = time.Now()
}

// RecordProfileMatch 记录入口画像匹配结果
func (as *AdmissionScorer) RecordProfileMatch(ip string, matched bool) {
	as.mu.Lock()
	defer as.mu.Unlock()
	s := as.getOrCreate(ip)
	s.profileTotal++
	if matched {
		s.profileMatch++
	}
	s.LastUpdate = time.Now()
}

// RecordActiveSession 更新活跃会话数
func (as *AdmissionScorer) RecordActiveSession(ip string, count int) {
	as.mu.Lock()
	defer as.mu.Unlock()
	s := as.getOrCreate(ip)
	s.ActiveSessions = count
	s.LastUpdate = time.Now()
}

// Evaluate 评估 IP 的准入动作
func (as *AdmissionScorer) Evaluate(ip string) IngressAction {
	as.mu.RLock()
	defer as.mu.RUnlock()

	s, ok := as.scores[ip]
	if !ok {
		return ActionPass // 未知 IP 默认放行
	}

	score := s.Score()
	admissionActionTotal.WithLabelValues(scoreToAction(score).String()).Inc()
	admissionScoreHistogram.Observe(score)

	return scoreToAction(score)
}

func scoreToAction(score float64) IngressAction {
	switch {
	case score > 60:
		return ActionPass
	case score > 30:
		return ActionThrottle
	default:
		return ActionDrop
	}
}

// GetScore 获取指定 IP 的当前评分（用于监控）
func (as *AdmissionScorer) GetScore(ip string) float64 {
	as.mu.RLock()
	defer as.mu.RUnlock()
	s, ok := as.scores[ip]
	if !ok {
		return 100 // 未知 IP 默认满分
	}
	return s.Score()
}
