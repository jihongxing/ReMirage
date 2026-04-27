// Package gtunnel - SwitchBuffer 基于 TransportConn 的原子切换缓冲器
// 在路径切换期间实现双发选收与去重，不依赖 *Path / *net.UDPConn。
package gtunnel

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// 双发时长随机范围
	minDualDuration = 80 * time.Millisecond
	maxDualDuration = 200 * time.Millisecond

	// 去重窗口大小
	dedupeWindowSize = 1024

	// 回滚阈值：新路径连续发送失败次数
	rollbackThreshold = 5
)

// SwitchBuffer 基于 TransportConn 接口的切换缓冲器。
// 在 demote/promote 事务期间提供双发选收与去重能力。
type SwitchBuffer struct {
	mu sync.RWMutex

	oldConn   TransportConn
	newConn   TransportConn
	dualMode  bool
	duration  time.Duration
	startTime time.Time

	// 去重：seq → 是否已交付
	seqSeen   map[uint64]bool
	seqWindow []uint64 // 环形窗口，用于淘汰旧 seq
	windowIdx int

	// 回滚计数
	newPathFailCount int32
}

// NewSwitchBuffer 创建 SwitchBuffer 实例
func NewSwitchBuffer() *SwitchBuffer {
	return &SwitchBuffer{
		seqSeen:   make(map[uint64]bool, dedupeWindowSize),
		seqWindow: make([]uint64, dedupeWindowSize),
	}
}

// EnableDualSend 启动双发选收模式。
// oldConn/newConn 为 TransportConn 接口，duration 为双发持续时间。
// 若 duration <= 0，使用 crypto/rand 生成 [80ms, 200ms] 随机时长。
func (sb *SwitchBuffer) EnableDualSend(oldConn, newConn TransportConn, duration time.Duration) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.dualMode {
		return fmt.Errorf("dual-send already active")
	}
	if oldConn == nil || newConn == nil {
		return fmt.Errorf("oldConn and newConn must not be nil")
	}

	if duration <= 0 {
		var err error
		duration, err = randomDualDuration()
		if err != nil {
			return fmt.Errorf("generate random duration: %w", err)
		}
	}

	sb.oldConn = oldConn
	sb.newConn = newConn
	sb.dualMode = true
	sb.duration = duration
	sb.startTime = time.Now()
	atomic.StoreInt32(&sb.newPathFailCount, 0)

	// 重置去重窗口
	sb.seqSeen = make(map[uint64]bool, dedupeWindowSize)
	sb.seqWindow = make([]uint64, dedupeWindowSize)
	sb.windowIdx = 0

	return nil
}

// SendDual 向新旧两条路径同时发送数据。
// 返回 nil 只要至少一条路径成功。
// 新路径连续失败计数用于回滚判断。
func (sb *SwitchBuffer) SendDual(data []byte) error {
	sb.mu.RLock()
	if !sb.dualMode {
		sb.mu.RUnlock()
		return fmt.Errorf("dual-send not active")
	}
	oldConn := sb.oldConn
	newConn := sb.newConn
	sb.mu.RUnlock()

	errOld := oldConn.Send(data)
	errNew := newConn.Send(data)

	if errNew != nil {
		atomic.AddInt32(&sb.newPathFailCount, 1)
	} else {
		atomic.StoreInt32(&sb.newPathFailCount, 0)
	}

	if errOld != nil && errNew != nil {
		return fmt.Errorf("dual-send both failed: old=%v, new=%v", errOld, errNew)
	}
	return nil
}

// ReceiveAndDedupe 接收去重。
// 首次见到的 seq 返回 (data, true)；重复 seq 返回 (nil, false)。
func (sb *SwitchBuffer) ReceiveAndDedupe(seq uint64, data []byte) ([]byte, bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.seqSeen[seq] {
		return nil, false
	}

	// 淘汰环形窗口中最旧的 seq
	oldSeq := sb.seqWindow[sb.windowIdx]
	if oldSeq != 0 || sb.seqSeen[0] {
		delete(sb.seqSeen, oldSeq)
	}

	sb.seqSeen[seq] = true
	sb.seqWindow[sb.windowIdx] = seq
	sb.windowIdx = (sb.windowIdx + 1) % dedupeWindowSize

	return data, true
}

// IsDualModeActive 返回双发模式是否处于活跃状态
func (sb *SwitchBuffer) IsDualModeActive() bool {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.dualMode
}

// ShouldRollback 返回新路径是否连续失败达到回滚阈值
func (sb *SwitchBuffer) ShouldRollback() bool {
	return atomic.LoadInt32(&sb.newPathFailCount) >= rollbackThreshold
}

// DisableDualSend 关闭双发模式
func (sb *SwitchBuffer) DisableDualSend() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.dualMode = false
	sb.oldConn = nil
	sb.newConn = nil
}

// Duration 返回当前双发持续时间
func (sb *SwitchBuffer) Duration() time.Duration {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.duration
}

// randomDualDuration 使用 crypto/rand 生成 [80ms, 200ms] 随机双发时长
func randomDualDuration() (time.Duration, error) {
	rangeMs := int64(maxDualDuration-minDualDuration) / int64(time.Millisecond) // 120
	n, err := rand.Int(rand.Reader, big.NewInt(rangeMs+1))
	if err != nil {
		return 0, err
	}
	return minDualDuration + time.Duration(n.Int64())*time.Millisecond, nil
}
