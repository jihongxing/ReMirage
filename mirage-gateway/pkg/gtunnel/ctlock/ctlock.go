// Package ctlock — TimedLock：最小持锁时间语义（非恒定时间）
//
// 降级说明（ADR）：
// 原 ConstantTimeLock 使用 time.Now() busy-wait 试图实现恒定时间语义，
// 但 Go 运行时调度器和 OS 定时器精度无法保证纳秒级恒定时间。
// 降级为 sync.Mutex + time.Sleep(minDuration) 的最小持锁时间语义。
// 不再声明 constant-time 安全属性。
package ctlock

import (
	"sync"
	"sync/atomic"
	"time"
)

// AuditCollector is a local interface to avoid circular imports.
type AuditCollector interface {
	OnOverflow(elapsed time.Duration, slotNs int64)
}

// TimedLock 最小持锁时间锁（降级自 ConstantTimeLock）。
// 保证 ProcessControl/ProcessStego 的执行时间 ≥ minDuration。
// 不保证恒定时间。
type TimedLock struct {
	mu          sync.Mutex
	minDuration time.Duration
	audit       AuditCollector
	overflows   atomic.Int64
}

// ConstantTimeLock 保留旧名称作为别名，兼容现有引用
type ConstantTimeLock = TimedLock

// NewConstantTimeLock 创建 TimedLock（保留旧构造函数名兼容）。
// slotNs 现在语义为最小持锁时间（纳秒），不再是恒定时间槽。
func NewConstantTimeLock(slotNs int64, audit AuditCollector) *TimedLock {
	if slotNs <= 0 {
		slotNs = 250_000
	}
	return &TimedLock{
		minDuration: time.Duration(slotNs),
		audit:       audit,
	}
}

// ProcessControl 在互斥锁保护下执行 handler，确保总时间 ≥ minDuration。
func (t *TimedLock) ProcessControl(handler func() error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	start := time.Now()
	err := handler()
	elapsed := time.Since(start)

	if elapsed < t.minDuration {
		time.Sleep(t.minDuration - elapsed)
	} else if elapsed > t.minDuration*2 {
		// handler 执行时间远超 minDuration，记录溢出
		t.overflows.Add(1)
		if t.audit != nil {
			t.audit.OnOverflow(elapsed, int64(t.minDuration))
		}
	}

	return err
}

// ProcessStego 在互斥锁保护下处理 stego 包，确保总时间 ≥ minDuration。
func (t *TimedLock) ProcessStego(isStego bool, handler func() error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	start := time.Now()
	var err error
	if isStego {
		err = handler()
	}
	// 不再执行 FakeCryptoWork — 降级后不声明 timing 等价性
	elapsed := time.Since(start)

	if elapsed < t.minDuration {
		time.Sleep(t.minDuration - elapsed)
	} else if elapsed > t.minDuration*2 {
		t.overflows.Add(1)
		if t.audit != nil {
			t.audit.OnOverflow(elapsed, int64(t.minDuration))
		}
	}

	return err
}

// Overflows returns the number of times processing exceeded 2x minDuration.
func (t *TimedLock) Overflows() int64 {
	return t.overflows.Load()
}
