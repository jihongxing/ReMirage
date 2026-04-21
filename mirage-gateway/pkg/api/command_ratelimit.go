package api

import (
	"fmt"
	"sync"
	"time"
)

// rateBucket 速率桶
type rateBucket struct {
	count       int
	windowStart time.Time
}

// CommandRateLimiter 命令速率限制器
type CommandRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	limit   int
	window  time.Duration
}

// NewCommandRateLimiter 创建速率限制器（每分钟 10 次）
func NewCommandRateLimiter() *CommandRateLimiter {
	rl := &CommandRateLimiter{
		buckets: make(map[string]*rateBucket),
		limit:   10,
		window:  time.Minute,
	}
	go rl.cleanup()
	return rl
}

// Check 检查是否超出速率限制
func (rl *CommandRateLimiter) Check(sourceAddr string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, ok := rl.buckets[sourceAddr]
	if !ok {
		rl.buckets[sourceAddr] = &rateBucket{count: 1, windowStart: now}
		return nil
	}

	// 窗口过期，重置
	if now.Sub(bucket.windowStart) > rl.window {
		bucket.count = 1
		bucket.windowStart = now
		return nil
	}

	bucket.count++
	if bucket.count > rl.limit {
		return fmt.Errorf("rate limit exceeded: %s (%d/%d per minute)", sourceAddr, bucket.count, rl.limit)
	}

	return nil
}

// cleanup 定期清理过期 bucket
func (rl *CommandRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for addr, bucket := range rl.buckets {
			if now.Sub(bucket.windowStart) > rl.window*2 {
				delete(rl.buckets, addr)
			}
		}
		rl.mu.Unlock()
	}
}
