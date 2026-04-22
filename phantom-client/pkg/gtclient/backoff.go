package gtclient

import (
	"math"
	"time"
)

// ExponentialBackoff 指数退避计算器
type ExponentialBackoff struct {
	Base      time.Duration // 基础间隔
	Max       time.Duration // 最大间隔
	FailCount int           // 当前连续失败次数
}

// NewExponentialBackoff creates a new ExponentialBackoff with the given base and max intervals.
func NewExponentialBackoff(base, max time.Duration) *ExponentialBackoff {
	return &ExponentialBackoff{
		Base: base,
		Max:  max,
	}
}

// Next returns the current backoff delay: min(base × 2^FailCount, max).
func (eb *ExponentialBackoff) Next() time.Duration {
	if eb.FailCount <= 0 {
		return eb.Base
	}
	delay := time.Duration(float64(eb.Base) * math.Pow(2, float64(eb.FailCount)))
	if delay > eb.Max || delay <= 0 { // overflow protection
		return eb.Max
	}
	return delay
}

// Record increments the fail count and returns the new backoff delay.
func (eb *ExponentialBackoff) Record() time.Duration {
	eb.FailCount++
	return eb.Next()
}

// Reset resets the fail count to zero.
func (eb *ExponentialBackoff) Reset() {
	eb.FailCount = 0
}
