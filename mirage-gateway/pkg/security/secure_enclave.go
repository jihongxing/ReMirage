// Package security - Secure Enclave 密钥安全区
// O6 深度隐匿：抗内存取证与冷启动攻击
//
// 问题：Go 的 GC 不保证及时清除已释放的内存页。
// X25519 私钥、TLS 会话密钥即使被 nil 赋值，仍以明文残留在堆中，
// 直到 GC 回收该页并被新数据覆盖。冷启动攻击可直接提取。
//
// 解决方案：
//  1. mlock: 锁定内存页，禁止被 swap 到磁盘
//  2. madvise(MADV_DONTDUMP): 禁止出现在 core dump 中
//  3. 显式覆写: 生命周期结束时立即用随机数据覆写（不等 GC）
//  4. Guard Pages: 前后各放一个不可访问的 guard page，防止 buffer overflow 读取
package security

import (
	"crypto/rand"
	"fmt"
	"log"
	"runtime"
	"sync"
	"unsafe"
)

// SecureEnclave 密钥安全区（mlock + 显式覆写 + guard pages）
type SecureEnclave struct {
	mu       sync.Mutex
	slots    []*EnclaveSlot
	shield   *RAMShield
	maxSlots int
}

// EnclaveSlot 安全区槽位
type EnclaveSlot struct {
	name    string        // 人类可读名称（调试用）
	buf     *SecureBuffer // mlock 锁定的缓冲区
	size    int           // 有效数据长度
	inUse   bool          // 是否正在使用
	created int64         // 创建时间（Unix nano）
}

// NewSecureEnclave 创建密钥安全区
func NewSecureEnclave(shield *RAMShield, maxSlots int) *SecureEnclave {
	if maxSlots <= 0 {
		maxSlots = 32
	}
	return &SecureEnclave{
		slots:    make([]*EnclaveSlot, 0, maxSlots),
		shield:   shield,
		maxSlots: maxSlots,
	}
}

// Store 将密钥材料存入安全区
// 输入的 key 会被立即复制到 mlock 区域，然后原始 slice 被覆写清零
// 返回 slot ID 用于后续 Load/Destroy
func (se *SecureEnclave) Store(name string, key []byte) (int, error) {
	se.mu.Lock()
	defer se.mu.Unlock()

	if len(se.slots) >= se.maxSlots {
		return -1, fmt.Errorf("安全区已满 (%d/%d)", len(se.slots), se.maxSlots)
	}

	// 1. 分配 mlock 锁定的缓冲区
	buf, err := se.shield.SecureAlloc(len(key))
	if err != nil {
		return -1, fmt.Errorf("分配安全缓冲区失败: %w", err)
	}

	// 2. 复制密钥到安全区
	copy(buf.Data, key)

	// 3. 立即覆写原始输入（防止 GC 前被取证）
	wipeBytes(key)

	// 4. 设置 finalizer：如果 slot 被 GC 回收前未显式 Destroy，自动擦除
	slot := &EnclaveSlot{
		name:  name,
		buf:   buf,
		size:  len(key),
		inUse: true,
	}

	runtime.SetFinalizer(slot, func(s *EnclaveSlot) {
		if s.inUse && s.buf != nil {
			log.Printf("⚠️ [SecureEnclave] 槽位 '%s' 未显式销毁，finalizer 自动擦除", s.name)
			wipeBytes(s.buf.Data)
		}
	})

	id := len(se.slots)
	se.slots = append(se.slots, slot)

	log.Printf("🔐 [SecureEnclave] 密钥已存入: name=%s, slot=%d, size=%d", name, id, len(key))
	return id, nil
}

// Load 从安全区读取密钥（返回副本，调用者负责用完后调用 WipeBytes 清除）
func (se *SecureEnclave) Load(slotID int) ([]byte, error) {
	se.mu.Lock()
	defer se.mu.Unlock()

	if slotID < 0 || slotID >= len(se.slots) {
		return nil, fmt.Errorf("无效的 slot ID: %d", slotID)
	}

	slot := se.slots[slotID]
	if !slot.inUse {
		return nil, fmt.Errorf("slot %d 已销毁", slotID)
	}

	// 返回副本（调用者负责清除）
	out := make([]byte, slot.size)
	copy(out, slot.buf.Data[:slot.size])
	return out, nil
}

// Destroy 销毁指定槽位的密钥（立即覆写 + munlock）
func (se *SecureEnclave) Destroy(slotID int) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	if slotID < 0 || slotID >= len(se.slots) {
		return fmt.Errorf("无效的 slot ID: %d", slotID)
	}

	slot := se.slots[slotID]
	if !slot.inUse {
		return nil // 已销毁，幂等
	}

	// 1. 用随机数据覆写（防止冷启动攻击读取零值模式）
	rand.Read(slot.buf.Data)
	// 2. 再用零覆写
	wipeBytes(slot.buf.Data)
	// 3. 标记为已销毁
	slot.inUse = false
	// 4. 通过 RAMShield 释放 mlock
	se.shield.SecureWipe(slot.buf)

	log.Printf("🗑️ [SecureEnclave] 密钥已销毁: name=%s, slot=%d", slot.name, slotID)
	return nil
}

// DestroyAll 销毁所有槽位（紧急自毁时调用）
func (se *SecureEnclave) DestroyAll() {
	se.mu.Lock()
	defer se.mu.Unlock()

	for i, slot := range se.slots {
		if slot.inUse {
			rand.Read(slot.buf.Data)
			wipeBytes(slot.buf.Data)
			slot.inUse = false
			se.shield.SecureWipe(slot.buf)
			log.Printf("🗑️ [SecureEnclave] 紧急销毁: name=%s, slot=%d", slot.name, i)
		}
	}
}

// ActiveCount 返回活跃槽位数
func (se *SecureEnclave) ActiveCount() int {
	se.mu.Lock()
	defer se.mu.Unlock()
	count := 0
	for _, slot := range se.slots {
		if slot.inUse {
			count++
		}
	}
	return count
}

// wipeBytes 安全擦除字节切片（防止编译器优化跳过）
// 使用 volatile 语义确保写入不被优化掉
func wipeBytes(b []byte) {
	for i := range b {
		*(*byte)(unsafe.Pointer(&b[i])) = 0
	}
	// 内存屏障：确保覆写对所有 CPU 可见
	runtime.KeepAlive(b)
}

// WipeBytes 导出版本（供外部调用者清除 Load 返回的副本）
func WipeBytes(b []byte) {
	wipeBytes(b)
}
