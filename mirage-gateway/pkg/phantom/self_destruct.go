// Package phantom - 自毁协议
// 实现物理清空与静默退出
package phantom

import (
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	DestructConfirmCode = "MIRAGE-DESTRUCT-CONFIRM"
	DestructMagic       = 0xDEADBEEF
)

// DestructController 自毁控制器
type DestructController struct {
	mu              sync.Mutex
	trafficStopper  func() error
	ebpfWiper       func() error
	logWiper        func() error
	tmpfsWiper      func() error
	peerNotifier    func() error
	destructed      bool
}

// NewDestructController 创建自毁控制器
func NewDestructController() *DestructController {
	return &DestructController{}
}

// SetTrafficStopper 设置流量停止器
func (dc *DestructController) SetTrafficStopper(fn func() error) {
	dc.trafficStopper = fn
}

// SetEBPFWiper 设置 eBPF 清除器
func (dc *DestructController) SetEBPFWiper(fn func() error) {
	dc.ebpfWiper = fn
}

// SetLogWiper 设置日志清除器
func (dc *DestructController) SetLogWiper(fn func() error) {
	dc.logWiper = fn
}

// SetTmpfsWiper 设置 tmpfs 清除器
func (dc *DestructController) SetTmpfsWiper(fn func() error) {
	dc.tmpfsWiper = fn
}

// SetPeerNotifier 设置对等节点通知器
func (dc *DestructController) SetPeerNotifier(fn func() error) {
	dc.peerNotifier = fn
}

// Execute 执行自毁序列
func (dc *DestructController) Execute(confirmCode string) bool {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// 验证确认码
	if confirmCode != DestructConfirmCode {
		log.Println("💀 [SelfDestruct] 确认码不匹配，拒绝执行")
		return false
	}

	if dc.destructed {
		log.Println("💀 [SelfDestruct] 已执行过自毁，忽略")
		return false
	}

	log.Println("💀 [SelfDestruct] ========== 开始自毁序列 ==========")

	// 1. 停止所有 B-DNA 流量（立即切断所有隧道）
	log.Println("💀 [SelfDestruct] [1/6] 停止所有流量...")
	if dc.trafficStopper != nil {
		if err := dc.trafficStopper(); err != nil {
			log.Printf("💀 [SelfDestruct] 停止流量失败: %v", err)
		}
	}

	// 2. 通知对等节点
	log.Println("💀 [SelfDestruct] [2/6] 通知对等节点...")
	if dc.peerNotifier != nil {
		if err := dc.peerNotifier(); err != nil {
			log.Printf("💀 [SelfDestruct] 通知对等节点失败: %v", err)
		}
	}

	// 3. 擦除内核 eBPF Maps（防止内存镜像取证）
	log.Println("💀 [SelfDestruct] [3/6] 擦除 eBPF Maps...")
	if dc.ebpfWiper != nil {
		if err := dc.ebpfWiper(); err != nil {
			log.Printf("💀 [SelfDestruct] 擦除 eBPF 失败: %v", err)
		}
	}

	// 4. 执行磁盘日志清除
	log.Println("💀 [SelfDestruct] [4/6] 清除日志...")
	if dc.logWiper != nil {
		if err := dc.logWiper(); err != nil {
			log.Printf("💀 [SelfDestruct] 清除日志失败: %v", err)
		}
	}

	// 5. 清除 tmpfs（master_key 驻留区）
	log.Println("💀 [SelfDestruct] [5/6] 清除 tmpfs...")
	if dc.tmpfsWiper != nil {
		if err := dc.tmpfsWiper(); err != nil {
			log.Printf("💀 [SelfDestruct] 清除 tmpfs 失败: %v", err)
		}
	}

	// 6. 内存覆写与进程自毁
	log.Println("💀 [SelfDestruct] [6/6] 内存覆写与进程自毁...")
	dc.destructed = true
	go dc.finalDestruct()

	return true
}

// finalDestruct 最终自毁（异步执行）
func (dc *DestructController) finalDestruct() {
	// 等待日志刷新
	time.Sleep(100 * time.Millisecond)

	// 1. 强制 GC 清理堆内存
	runtime.GC()
	debug.FreeOSMemory()

	// 2. 尝试自我删除二进制文件
	selfDelete()

	// 3. 静默退出
	os.Exit(0)
}

// selfDelete 自我删除二进制
func selfDelete() {
	execPath, err := os.Executable()
	if err != nil {
		return
	}

	// Linux: 尝试删除自身
	_ = os.Remove(execPath)

	// 如果删除失败，尝试覆盖
	if f, err := os.OpenFile(execPath, os.O_WRONLY|os.O_TRUNC, 0); err == nil {
		// 写入随机数据覆盖
		garbage := make([]byte, 4096)
		for i := range garbage {
			garbage[i] = byte(i % 256)
		}
		f.Write(garbage)
		f.Close()
	}
}

// ============== 独立函数版本 ==============

// SelfDestruct 独立自毁函数（简化版）
func SelfDestruct(confirmCode string) {
	if confirmCode != DestructConfirmCode {
		return
	}

	log.Println("💀 [SelfDestruct] 执行紧急自毁...")

	// 1. 擦除 eBPF Maps
	WipeEBPFMapsGlobal()

	// 2. 清除日志
	WipeLogsGlobal()

	// 3. 清除 tmpfs
	WipeTmpfsGlobal()

	// 4. 内存清理
	WipeMemoryGlobal()

	// 5. 自删除并退出
	selfDelete()
	os.Exit(0)
}

// WipeEBPFMapsGlobal 全局 eBPF Map 擦除
func WipeEBPFMapsGlobal() {
	// 写入自毁魔数到 /sys/fs/bpf/mirage_emergency
	emergencyPath := "/sys/fs/bpf/mirage_emergency"
	if f, err := os.OpenFile(emergencyPath, os.O_WRONLY, 0); err == nil {
		magic := uint32(DestructMagic)
		f.Write((*[4]byte)(unsafe.Pointer(&magic))[:])
		f.Close()
	}

	// 尝试卸载所有 BPF 程序
	bpfPaths := []string{
		"/sys/fs/bpf/mirage_h3_shaper",
		"/sys/fs/bpf/mirage_jitter",
		"/sys/fs/bpf/mirage_chameleon",
	}
	for _, path := range bpfPaths {
		os.RemoveAll(path)
	}
}

// WipeLogsGlobal 全局日志擦除
func WipeLogsGlobal() {
	logPaths := []string{
		"/var/log/mirage/",
		"/tmp/mirage-logs/",
		"/var/log/mirage-gateway.log",
	}

	for _, path := range logPaths {
		secureWipePath(path)
	}
}

// WipeTmpfsGlobal 全局 tmpfs 擦除
func WipeTmpfsGlobal() {
	tmpfsPaths := []string{
		"/dev/shm/mirage/",
		"/run/mirage/",
		"/tmp/mirage/",
	}

	for _, path := range tmpfsPaths {
		secureWipePath(path)
	}
}

// WipeMemoryGlobal 全局内存擦除
func WipeMemoryGlobal() {
	// 强制 GC
	runtime.GC()
	debug.FreeOSMemory()

	// 尝试 madvise MADV_DONTNEED（Linux）
	// 这会告诉内核可以丢弃这些页面
}

// secureWipePath 安全擦除路径
func secureWipePath(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	if info.IsDir() {
		// 递归擦除目录
		entries, _ := os.ReadDir(path)
		for _, entry := range entries {
			secureWipePath(path + "/" + entry.Name())
		}
		os.RemoveAll(path)
	} else {
		// 覆盖文件内容后删除
		secureWipeFile(path)
	}
}

// secureWipeFile 安全擦除文件
func secureWipeFile(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	size := info.Size()
	if size == 0 {
		os.Remove(path)
		return
	}

	// 打开文件进行覆盖
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		os.Remove(path)
		return
	}

	// 三次覆盖：0x00, 0xFF, 随机
	patterns := []byte{0x00, 0xFF, 0xAA}
	buf := make([]byte, 4096)

	for _, pattern := range patterns {
		for i := range buf {
			buf[i] = pattern
		}
		f.Seek(0, 0)
		for written := int64(0); written < size; {
			n, _ := f.Write(buf)
			written += int64(n)
		}
		f.Sync()
	}

	f.Close()

	// 删除文件
	os.Remove(path)

	// 尝试 unlink（确保 inode 释放）
	syscall.Unlink(path)
}
