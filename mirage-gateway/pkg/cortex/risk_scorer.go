package cortex

import (
	"context"
	"log"
	"sync"
	"time"

	"mirage-gateway/pkg/threat"
)

// 风险评分常量
const (
	HoneypotHitScore = 30
	DangerousFPScore = 40
	ThreatEventScore = 20
	AutoBanThreshold = 70
	DecayPerHour     = 10
	AutoBanTTL       = 2 * time.Hour
	MaxScore         = 100
)

// IPScore 单个 IP 的风险评分
type IPScore struct {
	Score     int
	UpdatedAt time.Time
	Sources   []string // honeypot / fingerprint / threat
}

// RiskScorer IP 风险评分器（评分累加 + 时间衰减 + 自动封禁）
type RiskScorer struct {
	mu        sync.RWMutex
	scores    map[string]*IPScore
	blacklist *threat.BlacklistManager
}

// NewRiskScorer 创建风险评分器
func NewRiskScorer(blacklist *threat.BlacklistManager) *RiskScorer {
	return &RiskScorer{
		scores:    make(map[string]*IPScore),
		blacklist: blacklist,
	}
}

// AddScore 累加 IP 风险评分，达到阈值自动封禁
func (rs *RiskScorer) AddScore(ip string, delta int, source string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	s, ok := rs.scores[ip]
	if !ok {
		s = &IPScore{}
		rs.scores[ip] = s
	}
	s.Score += delta
	if s.Score > MaxScore {
		s.Score = MaxScore
	}
	s.UpdatedAt = time.Now()
	s.Sources = append(s.Sources, source)

	if s.Score >= AutoBanThreshold && rs.blacklist != nil {
		_ = rs.blacklist.Add(ip+"/32", time.Now().Add(AutoBanTTL), threat.SourceLocal)
		log.Printf("[RiskScorer] 自动封禁: %s (score=%d)", ip, s.Score)
	}
}

// GetScore 获取 IP 当前评分
func (rs *RiskScorer) GetScore(ip string) int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if s, ok := rs.scores[ip]; ok {
		return s.Score
	}
	return 0
}

// GetIPScore 获取 IP 评分详情
func (rs *RiskScorer) GetIPScore(ip string) *IPScore {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.scores[ip]
}

// StartDecay 启动评分时间衰减（每小时 -DecayPerHour）
func (rs *RiskScorer) StartDecay(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rs.decay()
			}
		}
	}()
}

// decay 执行一次衰减
func (rs *RiskScorer) decay() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for ip, s := range rs.scores {
		s.Score -= DecayPerHour
		if s.Score <= 0 {
			delete(rs.scores, ip)
		}
	}
}
