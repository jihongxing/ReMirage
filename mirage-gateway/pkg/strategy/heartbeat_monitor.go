// Package strategy - 心跳监控与紧急自毁
package strategy

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"
)

const (
	// HeartbeatTimeout 心跳超时时间（死信开关）
	HeartbeatTimeout = 300 * time.Second // 5 分钟

	// HeartbeatInterval 心跳发送间隔
	HeartbeatInterval = 30 * time.Second
)

// EmergencyManager 紧急自毁管理器接口
type EmergencyManager interface {
	TriggerWipe() error
}

// SensitiveData 可擦除的敏感数据持有者
type SensitiveData interface {
	WipeSecrets()
}

// HeartbeatSender 心跳发送接口（解耦 api 包依赖）
type HeartbeatSender interface {
	// StartHeartbeatLoop 启动心跳循环，成功时调用 onSuccess
	StartHeartbeatLoop(ctx context.Context, onSuccess func())
	// IsConnected 是否已连接到 OS
	IsConnected() bool
}

// HeartbeatMonitor 心跳监控器
type HeartbeatMonitor struct {
	sender        HeartbeatSender
	gatewayID     string
	lastHeartbeat time.Time
	emergencyMgr  EmergencyManager
	ramShield     *RAMShield
	burnWiper     *BurnWiper
	sensitives    []SensitiveData
	ctx           context.Context
	cancel        context.CancelFunc
	emergencyMode bool
}

// NewHeartbeatMonitor 创建心跳监控器
func NewHeartbeatMonitor(
	sender HeartbeatSender,
	gatewayID string,
	emergencyMgr EmergencyManager,
	ramShield *RAMShield,
) *HeartbeatMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	wiper := NewBurnWiper()

	// 注册默认的敏感文件路径
	wiper.RegisterGlob("/tmp/mirage-*")
	wiper.RegisterGlob("/var/log/mirage-*")
	wiper.RegisterPath("/tmp/mirage-gateway")
	wiper.RegisterPath("/etc/mirage/gateway.yaml")

	return &HeartbeatMonitor{
		sender:        sender,
		gatewayID:     gatewayID,
		lastHeartbeat: time.Now(),
		emergencyMgr:  emergencyMgr,
		ramShield:     ramShield,
		burnWiper:     wiper,
		ctx:           ctx,
		cancel:        cancel,
		emergencyMode: false,
	}
}

// GetBurnWiper 获取擦除引擎（供外部模块注册密钥）
func (m *HeartbeatMonitor) GetBurnWiper() *BurnWiper {
	return m.burnWiper
}

// RegisterSensitive 注册敏感数据持有者（自毁时擦除）
func (m *HeartbeatMonitor) RegisterSensitive(s SensitiveData) {
	m.sensitives = append(m.sensitives, s)
}

// Start 启动心跳监控
func (m *HeartbeatMonitor) Start() error {
	log.Println("[HeartbeatMonitor] 启动心跳监控")

	if m.sender != nil {
		// 通过 HeartbeatSender 启动心跳循环，成功时喂看门狗
		m.sender.StartHeartbeatLoop(m.ctx, func() {
			m.lastHeartbeat = time.Now()
			m.emergencyMode = false
		})
	}

	// 启动超时检测（死信开关）
	go m.watchTimeout()

	return nil
}

// buildHeartbeatRequest 已移除 — 由外部 adapter 实现

// Stop 停止心跳监控
func (m *HeartbeatMonitor) Stop() {
	log.Println("[HeartbeatMonitor] 停止心跳监控")
	m.cancel()
}

// watchTimeout 监控心跳超时
func (m *HeartbeatMonitor) watchTimeout() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			elapsed := time.Since(m.lastHeartbeat)

			if elapsed >= HeartbeatTimeout {
				log.Printf("[HeartbeatMonitor] ⚠️ 心跳超时 %v，触发紧急自毁", elapsed)
				m.triggerEmergencyShutdown()
				return
			}

			// 警告阈值（超时前 1 分钟）
			if elapsed >= HeartbeatTimeout-60*time.Second && !m.emergencyMode {
				log.Printf("[HeartbeatMonitor] ⚠️ 心跳即将超时 (%v)，进入紧急模式", elapsed)
				m.emergencyMode = true
			}
		}
	}
}

// triggerEmergencyShutdown 触发紧急自毁（0xDEADBEEF 序列）
func (m *HeartbeatMonitor) triggerEmergencyShutdown() {
	log.Printf("🔥 [HeartbeatMonitor] ========== 0x%X 紧急自毁程序启动 ==========", DeadBeef)

	// 1. 写入 eBPF 自毁魔数（内核态立即停止转发）
	if err := m.wipeEBPFMaps(); err != nil {
		log.Printf("[HeartbeatMonitor] eBPF Map 清空失败: %v", err)
	}

	// 2. BurnWiper 暴力擦除（内存密钥 3 遍 urandom + 磁盘 3 遍覆写）
	if m.burnWiper != nil {
		m.burnWiper.Burn()
	}

	// 3. 通知其他敏感数据持有者
	for _, s := range m.sensitives {
		s.WipeSecrets()
	}

	// 4. RAM Shield 解锁 + 强制 GC
	if m.ramShield != nil {
		m.ramShield.UnlockAll()
	}
	runtime.GC()

	// 5. 进程自杀
	log.Println("🔥 [HeartbeatMonitor] 执行进程自杀")
	os.Exit(1)
}

// wipeEBPFMaps 清空所有 eBPF Map
func (m *HeartbeatMonitor) wipeEBPFMaps() error {
	log.Println("[HeartbeatMonitor] 清空 eBPF Map")

	if m.emergencyMgr == nil {
		return fmt.Errorf("紧急管理器未初始化")
	}

	if err := m.emergencyMgr.TriggerWipe(); err != nil {
		return fmt.Errorf("触发 eBPF 自毁失败: %w", err)
	}

	return nil
}

// wipeMemory 清空内存敏感数据（备用路径，主路径由 BurnWiper.Burn() 执行）
func (m *HeartbeatMonitor) wipeMemory() {
	for _, s := range m.sensitives {
		s.WipeSecrets()
	}
	if m.ramShield != nil {
		m.ramShield.UnlockAll()
	}
	runtime.GC()
}

// UpdateHeartbeat 手动更新心跳时间（用于测试）
func (m *HeartbeatMonitor) UpdateHeartbeat() {
	m.lastHeartbeat = time.Now()
}

// GetTimeSinceLastHeartbeat 获取距离上次心跳的时间
func (m *HeartbeatMonitor) GetTimeSinceLastHeartbeat() time.Duration {
	return time.Since(m.lastHeartbeat)
}

// IsEmergencyMode 是否处于紧急模式
func (m *HeartbeatMonitor) IsEmergencyMode() bool {
	return m.emergencyMode
}
