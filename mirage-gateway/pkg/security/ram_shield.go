// Package security - RAM Shield 增强版内存保护
package security

import (
	"fmt"
	"log"
	"runtime"
	"sync"
)

// SecureBuffer mlock 锁定的安全缓冲区
type SecureBuffer struct {
	Data   []byte
	locked bool
}

// RAMShield 增强版内存保护器
type RAMShield struct {
	mu             sync.Mutex
	registeredBufs []*SecureBuffer
	totalLocked    int64
}

// NewRAMShield 创建增强版内存保护器
func NewRAMShield() *RAMShield {
	return &RAMShield{
		registeredBufs: make([]*SecureBuffer, 0),
	}
}

// SecureAlloc 分配 mlock 锁定的内存缓冲区
func (rs *RAMShield) SecureAlloc(size int) (*SecureBuffer, error) {
	if size <= 0 {
		return nil, fmt.Errorf("无效的缓冲区大小: %d", size)
	}

	buf := &SecureBuffer{
		Data: make([]byte, size),
	}

	if err := mlockBuffer(buf.Data); err != nil {
		log.Printf("[RAMShield] ⚠️ mlock 失败（降级运行）: %v", err)
	} else {
		buf.locked = true
	}

	rs.mu.Lock()
	rs.registeredBufs = append(rs.registeredBufs, buf)
	rs.totalLocked += int64(size)
	rs.mu.Unlock()

	return buf, nil
}

// SecureWipe 安全擦除指定缓冲区
func (rs *RAMShield) SecureWipe(buf *SecureBuffer) error {
	if buf == nil {
		return fmt.Errorf("缓冲区为 nil")
	}

	// 逐字节覆写零值（防止编译器优化跳过）
	for i := range buf.Data {
		buf.Data[i] = 0
	}

	// 解锁内存页
	if buf.locked {
		if err := munlockBuffer(buf.Data); err != nil {
			log.Printf("[RAMShield] ⚠️ munlock 失败: %v", err)
		}
		buf.locked = false
	}

	// 触发 GC
	runtime.GC()

	// 从注册列表中移除
	rs.mu.Lock()
	for i, b := range rs.registeredBufs {
		if b == buf {
			rs.registeredBufs = append(rs.registeredBufs[:i], rs.registeredBufs[i+1:]...)
			rs.totalLocked -= int64(len(buf.Data))
			break
		}
	}
	rs.mu.Unlock()

	return nil
}

// DisableCoreDump 禁用 core dump
func (rs *RAMShield) DisableCoreDump() error {
	return disableCoreDump()
}

// CheckSwapUsage 检测内存是否被交换到磁盘
func (rs *RAMShield) CheckSwapUsage() (int64, error) {
	return checkSwapUsage()
}

// WipeAll 擦除所有已注册的敏感缓冲区
func (rs *RAMShield) WipeAll() error {
	rs.mu.Lock()
	bufs := make([]*SecureBuffer, len(rs.registeredBufs))
	copy(bufs, rs.registeredBufs)
	rs.mu.Unlock()

	var lastErr error
	for _, buf := range bufs {
		if err := rs.SecureWipe(buf); err != nil {
			lastErr = err
			log.Printf("[RAMShield] ⚠️ 擦除缓冲区失败: %v", err)
		}
	}

	runtime.GC()
	log.Println("[RAMShield] ✅ 所有敏感缓冲区已擦除")
	return lastErr
}

// RegisteredCount 返回已注册缓冲区数量
func (rs *RAMShield) RegisteredCount() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.registeredBufs)
}

// ContainsBuffer 检查缓冲区是否已注册
func (rs *RAMShield) ContainsBuffer(buf *SecureBuffer) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, b := range rs.registeredBufs {
		if b == buf {
			return true
		}
	}
	return false
}
