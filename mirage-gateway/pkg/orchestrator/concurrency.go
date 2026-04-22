package orchestrator

import (
	"context"
	"sync"
)

// LockManager 细粒度锁管理器
type LockManager struct {
	locks sync.Map // key -> *sync.Mutex
}

// NewLockManager 创建 LockManager 实例
func NewLockManager() *LockManager {
	return &LockManager{}
}

// Lock 获取指定 key 的锁，返回 unlock 函数。支持 context 超时控制。
func (lm *LockManager) Lock(ctx context.Context, key string) (unlock func(), err error) {
	val, _ := lm.locks.LoadOrStore(key, &sync.Mutex{})
	mu := val.(*sync.Mutex)

	// 使用 channel 实现可取消的锁获取
	done := make(chan struct{})
	go func() {
		mu.Lock()
		close(done)
	}()

	select {
	case <-done:
		return func() { mu.Unlock() }, nil
	case <-ctx.Done():
		// 如果 context 取消，需要等锁获取后再释放，避免泄漏
		go func() {
			<-done
			mu.Unlock()
		}()
		return nil, ctx.Err()
	}
}
