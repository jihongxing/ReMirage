package cortex

import (
	"context"
	"math"
	"mirage-gateway/pkg/threat"
	"sync"
	"time"
)

const maxPacketSizes = 1000

// ConnProfile 单个连接的行为画像
type ConnProfile struct {
	ConnID       string
	SourceIP     string
	StartTime    time.Time
	PacketSizes  []int // 包长序列（最近 1000 个）
	SendBytes    uint64
	RecvBytes    uint64
	PacketCount  uint64
	LastPacketAt time.Time
	IntervalSum  time.Duration
}

// SendRecvRatio 计算收发比例 RecvBytes/SendBytes
// SendBytes=0 时返回 0
func (cp *ConnProfile) SendRecvRatio() float64 {
	if cp.SendBytes == 0 {
		return 0
	}
	return float64(cp.RecvBytes) / float64(cp.SendBytes)
}

// DataEntropy 计算 PacketSizes 分布的 Shannon 熵
func (cp *ConnProfile) DataEntropy() float64 {
	if len(cp.PacketSizes) == 0 {
		return 0
	}
	freq := make(map[int]int)
	for _, s := range cp.PacketSizes {
		freq[s]++
	}
	total := float64(len(cp.PacketSizes))
	var entropy float64
	for _, count := range freq {
		p := float64(count) / total
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

// BehaviorMonitor 行为基线监控器
type BehaviorMonitor struct {
	mu         sync.RWMutex
	profiles   map[string]*ConnProfile // connID → profile
	baseline   *MarkovModel
	threshold  float64 // 偏离度阈值（默认 0.7）
	riskScorer *RiskScorer
	onKick     func(connID string) // 踢下线回调
}

// NewBehaviorMonitor 创建行为监控器
func NewBehaviorMonitor(baseline *MarkovModel, threshold float64, rs *RiskScorer) *BehaviorMonitor {
	return &BehaviorMonitor{
		profiles:   make(map[string]*ConnProfile),
		baseline:   baseline,
		threshold:  threshold,
		riskScorer: rs,
	}
}

// SetOnKick 设置踢下线回调
func (bm *BehaviorMonitor) SetOnKick(fn func(connID string)) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.onKick = fn
}

// RecordPacket 记录数据包
// direction: 0=send, 1=recv
func (bm *BehaviorMonitor) RecordPacket(connID, sourceIP string, size int, direction int) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	p, ok := bm.profiles[connID]
	if !ok {
		now := time.Now()
		p = &ConnProfile{
			ConnID:       connID,
			SourceIP:     sourceIP,
			StartTime:    now,
			LastPacketAt: now,
		}
		bm.profiles[connID] = p
	}

	// 更新包间隔
	now := time.Now()
	if p.PacketCount > 0 {
		p.IntervalSum += now.Sub(p.LastPacketAt)
	}
	p.LastPacketAt = now
	p.PacketCount++

	// 记录包长，cap at maxPacketSizes
	if len(p.PacketSizes) >= maxPacketSizes {
		p.PacketSizes = p.PacketSizes[1:]
	}
	p.PacketSizes = append(p.PacketSizes, size)

	// 更新收发字节
	if direction == 0 {
		p.SendBytes += uint64(size)
	} else {
		p.RecvBytes += uint64(size)
	}
}

// RemoveConn 清理断开的连接
func (bm *BehaviorMonitor) RemoveConn(connID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	delete(bm.profiles, connID)
}

// StartMonitoring 启动监控循环（每 10 秒评估一次）
func (bm *BehaviorMonitor) StartMonitoring(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				bm.evaluate()
			}
		}
	}()
}

// evaluate 评估所有活跃连接
func (bm *BehaviorMonitor) evaluate() {
	bm.mu.RLock()
	// 收集需要踢出的连接（避免在读锁下修改 map）
	type kickTarget struct {
		connID   string
		sourceIP string
		reason   string
	}
	var kicks []kickTarget

	now := time.Now()
	for connID, p := range bm.profiles {
		// 1. 马尔可夫偏离度检查
		deviation := bm.baseline.Deviation(p.PacketSizes)
		if deviation > bm.threshold {
			kicks = append(kicks, kickTarget{connID, p.SourceIP, "behavior_anomaly"})
			continue
		}

		// 2. 只发不收模式检查：SendRecvRatio < 0.05 且持续 > 30s
		duration := now.Sub(p.StartTime)
		if duration > 30*time.Second && p.SendBytes > 0 && p.SendRecvRatio() < 0.05 {
			kicks = append(kicks, kickTarget{connID, p.SourceIP, "send_only_pattern"})
		}
	}
	bm.mu.RUnlock()

	// 执行踢出操作
	for _, k := range kicks {
		if bm.onKick != nil {
			bm.onKick(k.connID)
		}
		if bm.riskScorer != nil {
			bm.riskScorer.AddScore(k.sourceIP, 25, "behavior_anomaly")
		}
		threat.BehaviorAnomalyTotal.WithLabelValues(threat.GetGatewayID()).Inc()
	}
}
