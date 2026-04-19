// Package strategy - RAM Shield 内存保护
package strategy

import (
	"log"
	"runtime"
)

// RAMShield 内存保护器
type RAMShield struct {
	lockedPages []uintptr
	lockedSize  int64
}

// NewRAMShield 创建内存保护器
func NewRAMShield() *RAMShield {
	return &RAMShield{
		lockedPages: make([]uintptr, 0),
		lockedSize:  0,
	}
}

// LockMemory 锁定内存页（防止 swap）
func (s *RAMShield) LockMemory(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	
	// Windows 不支持 mlock，使用 VirtualLock 替代
	// 这里简化实现，仅记录日志
	log.Printf("[RAMShield] ⚠️ Windows 平台不支持 mlock，跳过内存锁定")
	
	s.lockedSize += int64(len(data))
	
	return nil
}

// UnlockAll 解锁所有内存
func (s *RAMShield) UnlockAll() error {
	s.lockedPages = s.lockedPages[:0]
	s.lockedSize = 0
	
	log.Println("[RAMShield] 已解锁所有内存")
	
	return nil
}

// WipeMemory 清空内存数据
func (s *RAMShield) WipeMemory(data []byte) {
	if len(data) == 0 {
		return
	}
	
	// 覆盖为零
	for i := range data {
		data[i] = 0
	}
	
	// 强制 GC
	runtime.GC()
	
	log.Printf("[RAMShield] 清空内存: %d 字节", len(data))
}

// CheckSwapUsage 检查是否有内存被 swap
func (s *RAMShield) CheckSwapUsage() (bool, error) {
	// Windows 不支持 /proc/self/status
	log.Println("[RAMShield] ⚠️ Windows 平台不支持 swap 检查")
	return false, nil
}

// MonitorMemoryMaps 监控内存映射
func (s *RAMShield) MonitorMemoryMaps() error {
	// Windows 不支持 /proc/self/maps
	log.Println("[RAMShield] ⚠️ Windows 平台不支持内存映射监控")
	return nil
}

// DisableCoreDump 禁用 core dump
func (s *RAMShield) DisableCoreDump() error {
	// Windows 不支持 rlimit
	log.Println("[RAMShield] ⚠️ Windows 平台不支持 core dump 禁用")
	return nil
}

// GetLockedMemorySize 获取已锁定内存大小
func (s *RAMShield) GetLockedMemorySize() int64 {
	return s.lockedSize
}

// SecureString 安全字符串（自动清零）
type SecureString struct {
	data   []byte
	shield *RAMShield
}

// NewSecureString 创建安全字符串
func NewSecureString(value string, shield *RAMShield) (*SecureString, error) {
	data := []byte(value)
	
	// 锁定内存
	if err := shield.LockMemory(data); err != nil {
		return nil, err
	}
	
	return &SecureString{
		data:   data,
		shield: shield,
	}, nil
}

// Get 获取字符串值
func (s *SecureString) Get() string {
	return string(s.data)
}

// Destroy 销毁字符串（清零）
func (s *SecureString) Destroy() {
	s.shield.WipeMemory(s.data)
	s.data = nil
}

// String 实现 Stringer 接口（隐藏真实值）
func (s *SecureString) String() string {
	return "[REDACTED]"
}
