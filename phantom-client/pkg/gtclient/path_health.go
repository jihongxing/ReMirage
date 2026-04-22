package gtclient

import (
	"sync"
	"time"
)

// PathHealthScorer 路径健康评分器
type PathHealthScorer struct {
	mu               sync.Mutex
	rttBaseline      float64 // EWMA RTT 基线 (ms)
	ewmaAlpha        float64 // EWMA 衰减系数
	failCount        int     // 连续失败计数
	switchFreq       int     // 切换频率
	suspiciousScore  float64 // 异常分数
	threshold        float64 // 进入 Suspicious 的阈值
	recoverThreshold float64 // 退出 Suspicious 的阈值
	lastSample       time.Time
}

// NewPathHealthScorer 创建路径健康评分器
func NewPathHealthScorer(threshold, recoverThreshold float64) *PathHealthScorer {
	return &PathHealthScorer{
		ewmaAlpha:        0.3,
		threshold:        threshold,
		recoverThreshold: recoverThreshold,
	}
}

// RecordRTT 记录 RTT 样本
func (p *PathHealthScorer) RecordRTT(rttMs float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.rttBaseline == 0 {
		p.rttBaseline = rttMs
	} else {
		p.rttBaseline = p.ewmaAlpha*rttMs + (1-p.ewmaAlpha)*p.rttBaseline
	}

	// RTT 偏离基线越大，异常分数越高
	if p.rttBaseline > 0 {
		deviation := (rttMs - p.rttBaseline) / p.rttBaseline
		if deviation > 0.5 {
			p.suspiciousScore += deviation * 10
		} else if p.suspiciousScore > 0 {
			p.suspiciousScore -= 1 // 缓慢恢复
			if p.suspiciousScore < 0 {
				p.suspiciousScore = 0
			}
		}
	}

	p.failCount = 0 // RTT 成功意味着连接正常
	p.lastSample = time.Now()
}

// RecordFailure 记录连接失败
func (p *PathHealthScorer) RecordFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failCount++
	p.suspiciousScore += float64(p.failCount) * 5
}

// RecordSwitch 记录路径切换
func (p *PathHealthScorer) RecordSwitch() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.switchFreq++
	p.suspiciousScore += 3
}

// IsSuspicious 是否应进入 Suspicious 状态
func (p *PathHealthScorer) IsSuspicious() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.suspiciousScore >= p.threshold
}

// IsRecovered 是否已恢复（可退出 Suspicious）
func (p *PathHealthScorer) IsRecovered() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.suspiciousScore < p.recoverThreshold
}

// Score 返回当前异常分数
func (p *PathHealthScorer) Score() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.suspiciousScore
}

// RTTBaseline 返回当前 RTT 基线
func (p *PathHealthScorer) RTTBaseline() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rttBaseline
}
