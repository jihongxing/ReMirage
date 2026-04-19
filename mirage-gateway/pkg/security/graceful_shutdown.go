package security

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// ShutdownModule 可关闭模块接口
type ShutdownModule interface {
	Name() string
	Shutdown(ctx context.Context) error
}

// EmergencyWiper 紧急擦除接口
type EmergencyWiper interface {
	TriggerWipe() error
}

// GracefulShutdown 优雅关闭管理器
type GracefulShutdown struct {
	mu               sync.Mutex
	modules          []ShutdownModule
	sensitiveBuffers []*SecureBuffer
	ramShield        *RAMShield
	emergencyMgr     EmergencyWiper
	timeout          time.Duration
}

// NewGracefulShutdown 创建优雅关闭管理器
func NewGracefulShutdown(ramShield *RAMShield, emergencyMgr EmergencyWiper, timeout time.Duration) *GracefulShutdown {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &GracefulShutdown{
		modules:          make([]ShutdownModule, 0),
		sensitiveBuffers: make([]*SecureBuffer, 0),
		ramShield:        ramShield,
		emergencyMgr:     emergencyMgr,
		timeout:          timeout,
	}
}

// RegisterModule 注册可关闭模块
func (gs *GracefulShutdown) RegisterModule(module ShutdownModule) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.modules = append(gs.modules, module)
	log.Printf("[GracefulShutdown] 已注册模块: %s", module.Name())
}

// RegisterSensitiveBuffer 注册敏感内存区域
func (gs *GracefulShutdown) RegisterSensitiveBuffer(buf *SecureBuffer) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.sensitiveBuffers = append(gs.sensitiveBuffers, buf)
}

// Shutdown 执行优雅关闭
func (gs *GracefulShutdown) Shutdown() error {
	gs.mu.Lock()
	modules := make([]ShutdownModule, len(gs.modules))
	copy(modules, gs.modules)
	gs.mu.Unlock()

	log.Println("[GracefulShutdown] 🛑 开始优雅关闭...")

	done := make(chan error, 1)
	go func() {
		done <- gs.executeShutdown(modules)
	}()

	timer := time.NewTimer(gs.timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("[GracefulShutdown] ⚠️ 关闭过程中出现错误: %v", err)
		}
		log.Println("[GracefulShutdown] ✅ 优雅关闭完成")
		return err
	case <-timer.C:
		log.Println("[GracefulShutdown] ⚠️ 关闭超时，强制退出")
		os.Exit(1)
		return fmt.Errorf("关闭超时")
	}
}

func (gs *GracefulShutdown) executeShutdown(modules []ShutdownModule) error {
	ctx, cancel := context.WithTimeout(context.Background(), gs.timeout-2*time.Second)
	defer cancel()

	// 1. 按逆序关闭所有模块
	for i := len(modules) - 1; i >= 0; i-- {
		m := modules[i]
		log.Printf("[GracefulShutdown] 关闭模块: %s", m.Name())
		if err := m.Shutdown(ctx); err != nil {
			log.Printf("[GracefulShutdown] ⚠️ 模块 %s 关闭失败: %v", m.Name(), err)
		}
	}

	// 2. 调用 EmergencyWiper 清空 eBPF Map
	if gs.emergencyMgr != nil {
		log.Println("[GracefulShutdown] 清空 eBPF Map...")
		if err := gs.emergencyMgr.TriggerWipe(); err != nil {
			log.Printf("[GracefulShutdown] ⚠️ eBPF Map 清空失败: %v", err)
		}
	}

	// 3. 擦除所有敏感内存
	if gs.ramShield != nil {
		log.Println("[GracefulShutdown] 擦除敏感内存...")
		if err := gs.ramShield.WipeAll(); err != nil {
			log.Printf("[GracefulShutdown] ⚠️ 内存擦除失败: %v", err)
		}
	}

	return nil
}

// GetModules 返回已注册模块列表（测试用）
func (gs *GracefulShutdown) GetModules() []ShutdownModule {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	result := make([]ShutdownModule, len(gs.modules))
	copy(result, gs.modules)
	return result
}
