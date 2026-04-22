package gtclient

import (
	"sync"
	"time"
)

// ProductTier 产品层级
type ProductTier int

const (
	TierStandard ProductTier = iota // 无去相关
	TierPlatinum                    // 30ms 窗口
	TierDiamond                     // 50ms 窗口
)

// TierWindow 返回产品层级对应的去相关窗口
func TierWindow(tier ProductTier) time.Duration {
	switch tier {
	case TierPlatinum:
		return 30 * time.Millisecond
	case TierDiamond:
		return 50 * time.Millisecond
	default:
		return 0 // Standard: 不启用
	}
}

// SessionShaper 会话级时序去相关器
type SessionShaper struct {
	mu         sync.Mutex
	window     time.Duration
	buffer     [][]byte
	flushTimer *time.Timer
	tier       ProductTier
	sendFn     func([]byte) error
}

// NewSessionShaper 创建 Session Shaper
func NewSessionShaper(tier ProductTier, sendFn func([]byte) error) *SessionShaper {
	return &SessionShaper{
		window: TierWindow(tier),
		tier:   tier,
		sendFn: sendFn,
	}
}

// Shape 对数据进行时序去相关处理
func (ss *SessionShaper) Shape(data []byte) error {
	if ss.window == 0 {
		// Standard 层级：直接发送，不做去相关
		return ss.sendFn(data)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	// 缓冲数据
	copied := make([]byte, len(data))
	copy(copied, data)
	ss.buffer = append(ss.buffer, copied)

	// 设置定时 flush
	if ss.flushTimer == nil {
		ss.flushTimer = time.AfterFunc(ss.window, ss.flush)
	}

	return nil
}

// flush 批量发送缓冲数据
func (ss *SessionShaper) flush() {
	ss.mu.Lock()
	buf := ss.buffer
	ss.buffer = nil
	ss.flushTimer = nil
	ss.mu.Unlock()

	for _, data := range buf {
		ss.sendFn(data)
	}
}

// SetTier 动态更新产品层级
func (ss *SessionShaper) SetTier(tier ProductTier) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.tier = tier
	ss.window = TierWindow(tier)
}

// Window 返回当前窗口
func (ss *SessionShaper) Window() time.Duration {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.window
}
