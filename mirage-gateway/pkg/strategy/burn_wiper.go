// Package strategy - BurnWiper: 暴力内存/磁盘擦除引擎
// 收到 0xDEADBEEF 信号后，用 /dev/urandom 把所有私钥切片覆写 3 遍
// 确保取证工具无法从进程内存或磁盘残留中恢复任何密钥材料
package strategy

import (
	"crypto/rand"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const (
	// DeadBeef 自毁魔数
	DeadBeef = uint32(0xDEADBEEF)

	// WipePasses 覆写遍数
	WipePasses = 3
)

// SecretSlice 可被暴力擦除的密钥切片
type SecretSlice struct {
	data []byte
	name string
}

// BurnWiper 暴力擦除引擎
type BurnWiper struct {
	mu      sync.Mutex
	secrets []SecretSlice
	paths   []string // 需要安全删除的文件/目录路径
	burned  bool
}

// NewBurnWiper 创建擦除引擎
func NewBurnWiper() *BurnWiper {
	return &BurnWiper{
		secrets: make([]SecretSlice, 0, 16),
		paths:   make([]string, 0, 16),
	}
}

// RegisterSecret 注册一个密钥切片（传入切片的指针，擦除时直接覆写底层数组）
func (bw *BurnWiper) RegisterSecret(name string, secret []byte) {
	if len(secret) == 0 {
		return
	}
	bw.mu.Lock()
	defer bw.mu.Unlock()

	bw.secrets = append(bw.secrets, SecretSlice{
		data: secret,
		name: name,
	})
}

// RegisterPath 注册需要安全删除的文件路径
func (bw *BurnWiper) RegisterPath(path string) {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	bw.paths = append(bw.paths, path)
}

// RegisterGlob 注册需要安全删除的 glob 模式
func (bw *BurnWiper) RegisterGlob(pattern string) {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	bw.paths = append(bw.paths, pattern)
}

// Burn 执行暴力擦除（0xDEADBEEF 触发）
// 顺序：内存密钥 → 磁盘文件 → 强制 GC → 返回
// 此函数执行后，所有注册的密钥切片内容变为随机垃圾
func (bw *BurnWiper) Burn() {
	bw.mu.Lock()
	if bw.burned {
		bw.mu.Unlock()
		return
	}
	bw.burned = true
	secrets := bw.secrets
	paths := bw.paths
	bw.mu.Unlock()

	log.Printf("🔥 [BurnWiper] 0x%X 触发，开始暴力擦除 (%d 密钥, %d 路径)",
		DeadBeef, len(secrets), len(paths))

	// Phase 1: 内存密钥 — 3 遍 /dev/urandom 覆写
	for _, s := range secrets {
		bw.wipeSlice(s)
	}

	// Phase 2: 磁盘文件 — 3 遍随机覆写后删除
	for _, p := range paths {
		bw.wipePath(p)
	}

	// Phase 3: 强制 GC，让 Go runtime 回收所有已清零的对象
	runtime.GC()
	runtime.GC() // 双重 GC 确保 finalizer 执行

	log.Println("🔥 [BurnWiper] 擦除完成，所有密钥材料已销毁")
}

// wipeSlice 对单个密钥切片执行 3 遍随机覆写
func (bw *BurnWiper) wipeSlice(s SecretSlice) {
	if len(s.data) == 0 {
		return
	}

	for pass := 0; pass < WipePasses; pass++ {
		// 直接覆写底层数组（绕过 Go 的 copy-on-write 语义）
		rand.Read(s.data)
	}

	// 最终一遍全零（确保不留随机残留作为"密钥存在"的证据）
	for i := range s.data {
		s.data[i] = 0
	}

	log.Printf("  🗑️  [BurnWiper] 已擦除: %s (%d bytes, %d passes)", s.name, len(s.data), WipePasses)
}

// wipePath 安全删除文件或 glob 匹配的文件
func (bw *BurnWiper) wipePath(pattern string) {
	// 尝试 glob 展开
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		// 不是 glob，当作单个路径处理
		matches = []string{pattern}
	}

	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue // 文件不存在，跳过
		}

		if info.IsDir() {
			// 递归删除目录中的所有文件
			filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
				if err != nil || fi.IsDir() {
					return nil
				}
				bw.secureOverwrite(p, fi.Size())
				return nil
			})
			os.RemoveAll(path)
		} else {
			bw.secureOverwrite(path, info.Size())
			os.Remove(path)
		}

		log.Printf("  🗑️  [BurnWiper] 已删除: %s", path)
	}
}

// secureOverwrite 对单个文件执行 3 遍随机覆写
func (bw *BurnWiper) secureOverwrite(path string, size int64) {
	if size == 0 {
		return
	}

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer f.Close()

	buf := make([]byte, 4096)

	for pass := 0; pass < WipePasses; pass++ {
		f.Seek(0, 0)
		remaining := size
		for remaining > 0 {
			n := int64(len(buf))
			if n > remaining {
				n = remaining
			}
			rand.Read(buf[:n])
			f.Write(buf[:n])
			remaining -= n
		}
		f.Sync()
	}
}

// WipeSecrets 实现 SensitiveData 接口（供 HeartbeatMonitor 调用）
func (bw *BurnWiper) WipeSecrets() {
	bw.Burn()
}
