package security

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AntiDebug 反调试检测器
type AntiDebug struct {
	mu         sync.RWMutex
	silentMode bool
	lastCheck  time.Time
	interval   time.Duration
	onSilent   func()
	onRecover  func()
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewAntiDebug 创建反调试检测器
func NewAntiDebug(interval time.Duration) *AntiDebug {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &AntiDebug{
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// StartMonitor 启动检测循环
func (ad *AntiDebug) StartMonitor(ctx context.Context) error {
	ad.mu.Lock()
	ad.ctx, ad.cancel = context.WithCancel(ctx)
	ad.mu.Unlock()

	ad.wg.Add(1)
	go func() {
		defer ad.wg.Done()
		ticker := time.NewTicker(ad.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ad.ctx.Done():
				return
			case <-ticker.C:
				detected := ad.IsDebuggerPresent()
				ad.mu.Lock()
				wasSilent := ad.silentMode
				if detected && !wasSilent {
					ad.silentMode = true
					cb := ad.onSilent
					ad.mu.Unlock()
					log.Println("[AntiDebug] 🚨 检测到调试器，进入静默模式")
					if cb != nil {
						cb()
					}
				} else if !detected && wasSilent {
					ad.silentMode = false
					cb := ad.onRecover
					ad.mu.Unlock()
					log.Println("[AntiDebug] ✅ 调试器已脱离，恢复正常模式")
					if cb != nil {
						cb()
					}
				} else {
					ad.mu.Unlock()
				}
				ad.mu.Lock()
				ad.lastCheck = time.Now()
				ad.mu.Unlock()
			}
		}
	}()

	log.Println("[AntiDebug] ✅ 反调试检测循环已启动")
	return nil
}

// IsDebuggerPresent 返回当前是否检测到调试器
func (ad *AntiDebug) IsDebuggerPresent() bool {
	// 检查 TracerPid
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		log.Printf("[AntiDebug] ⚠️ 读取 /proc/self/status 失败: %v", err)
		return false
	}

	pid, err := ParseTracerPid(string(data))
	if err == nil && pid != 0 {
		return true
	}

	// 扫描调试器进程
	return scanDebuggerProcesses()
}

// EnterSilentMode 进入静默模式
func (ad *AntiDebug) EnterSilentMode() {
	ad.mu.Lock()
	ad.silentMode = true
	cb := ad.onSilent
	ad.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// ExitSilentMode 退出静默模式
func (ad *AntiDebug) ExitSilentMode() {
	ad.mu.Lock()
	ad.silentMode = false
	cb := ad.onRecover
	ad.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// IsSilent 返回是否处于静默模式
func (ad *AntiDebug) IsSilent() bool {
	ad.mu.RLock()
	defer ad.mu.RUnlock()
	return ad.silentMode
}

// SetCallbacks 设置静默/恢复回调
func (ad *AntiDebug) SetCallbacks(onSilent, onRecover func()) {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	ad.onSilent = onSilent
	ad.onRecover = onRecover
}

// Stop 停止检测循环
func (ad *AntiDebug) Stop() {
	ad.cancel()
	ad.wg.Wait()
	log.Println("[AntiDebug] 🛑 反调试检测已停止")
}

// ParseTracerPid 解析 /proc/self/status 中的 TracerPid（纯函数，可测试）
func ParseTracerPid(statusContent string) (int, error) {
	scanner := bufio.NewScanner(strings.NewReader(statusContent))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "TracerPid:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, fmt.Errorf("TracerPid 字段格式错误")
			}
			pid, err := strconv.Atoi(fields[1])
			if err != nil {
				return 0, fmt.Errorf("解析 TracerPid 失败: %w", err)
			}
			return pid, nil
		}
	}
	return 0, fmt.Errorf("未找到 TracerPid 字段")
}

// scanDebuggerProcesses 扫描常见调试器进程
func scanDebuggerProcesses() bool {
	debuggers := []string{"gdb", "strace", "ltrace", "perf", "lldb", "radare2"}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// 只检查数字目录（PID）
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		commPath := filepath.Join("/proc", entry.Name(), "comm")
		data, err := os.ReadFile(commPath)
		if err != nil {
			continue
		}

		comm := strings.TrimSpace(string(data))
		for _, dbg := range debuggers {
			if comm == dbg {
				log.Printf("[AntiDebug] 🚨 检测到调试器进程: %s (PID: %s)", comm, entry.Name())
				return true
			}
		}
	}

	return false
}
