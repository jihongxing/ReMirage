// Package security - 心跳超时看门狗
// 独立于 GracefulShutdown，当心跳超时时自动触发自毁
package security

import (
	"context"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Watchdog 心跳超时看门狗
type Watchdog struct {
	mu sync.Mutex

	// 最后一次心跳时间（原子操作）
	lastHeartbeat atomic.Int64

	// 超时阈值
	timeout time.Duration

	// 自毁组件
	ramShield    *RAMShield
	emergencyMgr EmergencyWiper

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWatchdog 创建看门狗
func NewWatchdog(timeout time.Duration, ramShield *RAMShield, emergencyMgr EmergencyWiper) *Watchdog {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watchdog{
		timeout:      timeout,
		ramShield:    ramShield,
		emergencyMgr: emergencyMgr,
		ctx:          ctx,
		cancel:       cancel,
	}
	w.lastHeartbeat.Store(time.Now().UnixNano())
	return w
}

// Feed 喂狗（收到心跳时调用）
func (w *Watchdog) Feed() {
	w.lastHeartbeat.Store(time.Now().UnixNano())
}

// Start 启动看门狗
func (w *Watchdog) Start() {
	w.wg.Add(1)
	go w.watchLoop()
	log.Printf("[Watchdog] 已启动，超时阈值: %v", w.timeout)
}

// Stop 停止看门狗
func (w *Watchdog) Stop() {
	w.cancel()
	w.wg.Wait()
	log.Println("[Watchdog] 已停止")
}

func (w *Watchdog) watchLoop() {
	defer w.wg.Done()

	// 检查间隔 = 超时的 1/10，确保及时发现
	checkInterval := w.timeout / 10
	if checkInterval < 5*time.Second {
		checkInterval = 5 * time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			last := time.Unix(0, w.lastHeartbeat.Load())
			elapsed := time.Since(last)

			if elapsed >= w.timeout {
				log.Printf("[Watchdog] 🚨 心跳超时 (%v >= %v)，触发自毁", elapsed, w.timeout)
				w.selfDestruct()
				return
			}

			// 80% 超时时发出警告
			if elapsed >= w.timeout*8/10 {
				log.Printf("[Watchdog] ⚠️ 心跳即将超时: %v / %v", elapsed, w.timeout)
			}
		}
	}
}

// selfDestruct 自毁序列
func (w *Watchdog) selfDestruct() {
	log.Println("[Watchdog] 🔥 === 自毁序列启动 ===")

	// 1. 清空 eBPF Map（原子操作，内核态数据优先）
	if w.emergencyMgr != nil {
		if err := w.emergencyMgr.TriggerWipe(); err != nil {
			log.Printf("[Watchdog] ⚠️ eBPF Map 清空失败: %v", err)
		} else {
			log.Println("[Watchdog] ✅ eBPF Map 已清空")
		}
	}

	// 2. 擦除所有敏感内存
	if w.ramShield != nil {
		if err := w.ramShield.WipeAll(); err != nil {
			log.Printf("[Watchdog] ⚠️ 内存擦除失败: %v", err)
		} else {
			log.Println("[Watchdog] ✅ 敏感内存已擦除")
		}
	}

	log.Println("[Watchdog] 🔥 === 自毁完成，进程退出 ===")

	// 3. 强制退出，不留痕迹
	os.Exit(0)
}

// TimeSinceLastHeartbeat 返回距上次心跳的时间
func (w *Watchdog) TimeSinceLastHeartbeat() time.Duration {
	last := time.Unix(0, w.lastHeartbeat.Load())
	return time.Since(last)
}
