// Package strategy - 心跳监控与紧急自毁
package strategy

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

const (
	// HeartbeatTimeout 心跳超时时间（死信开关）
	HeartbeatTimeout = 300 * time.Second // 5 分钟
	
	// HeartbeatInterval 心跳发送间隔
	HeartbeatInterval = 30 * time.Second
)

// HeartbeatMonitor 心跳监控器
type HeartbeatMonitor struct {
	osEndpoint      string
	lastHeartbeat   time.Time
	emergencyMgr    EmergencyManager // 紧急自毁管理器
	ramShield       *RAMShield       // 内存保护器
	ctx             context.Context
	cancel          context.CancelFunc
	emergencyMode   bool
}

// EmergencyManager 紧急自毁管理器接口
type EmergencyManager interface {
	TriggerWipe() error
}

// NewHeartbeatMonitor 创建心跳监控器
func NewHeartbeatMonitor(osEndpoint string, emergencyMgr EmergencyManager, ramShield *RAMShield) *HeartbeatMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &HeartbeatMonitor{
		osEndpoint:    osEndpoint,
		lastHeartbeat: time.Now(),
		emergencyMgr:  emergencyMgr,
		ramShield:     ramShield,
		ctx:           ctx,
		cancel:        cancel,
		emergencyMode: false,
	}
}

// Start 启动心跳监控
func (m *HeartbeatMonitor) Start() error {
	log.Println("[HeartbeatMonitor] 启动心跳监控")
	
	// 启动心跳发送
	go m.sendHeartbeat()
	
	// 启动超时检测
	go m.watchTimeout()
	
	return nil
}

// Stop 停止心跳监控
func (m *HeartbeatMonitor) Stop() {
	log.Println("[HeartbeatMonitor] 停止心跳监控")
	m.cancel()
}

// sendHeartbeat 发送心跳
func (m *HeartbeatMonitor) sendHeartbeat() {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if err := m.sendHeartbeatToOS(); err != nil {
				log.Printf("[HeartbeatMonitor] 心跳发送失败: %v", err)
			} else {
				m.lastHeartbeat = time.Now()
			}
		}
	}
}

// sendHeartbeatToOS 向 Mirage-OS 发送心跳
func (m *HeartbeatMonitor) sendHeartbeatToOS() error {
	// TODO: 实现 gRPC/HTTP 心跳请求
	// req := &pb.HeartbeatRequest{
	//     GatewayId: getGatewayID(),
	//     Timestamp: time.Now().Unix(),
	//     Status: "online",
	// }
	// resp, err := client.Heartbeat(ctx, req)
	
	log.Println("[HeartbeatMonitor] 发送心跳到 Mirage-OS")
	return nil
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

// triggerEmergencyShutdown 触发紧急自毁
func (m *HeartbeatMonitor) triggerEmergencyShutdown() {
	log.Println("🔥 [HeartbeatMonitor] ========== 紧急自毁程序启动 ==========")
	
	// 1. 清空 eBPF Map
	if err := m.wipeEBPFMaps(); err != nil {
		log.Printf("[HeartbeatMonitor] eBPF Map 清空失败: %v", err)
	}
	
	// 2. 清空内存敏感数据
	if err := m.wipeMemory(); err != nil {
		log.Printf("[HeartbeatMonitor] 内存清空失败: %v", err)
	}
	
	// 3. 删除临时文件
	if err := m.cleanupFiles(); err != nil {
		log.Printf("[HeartbeatMonitor] 文件清理失败: %v", err)
	}
	
	// 4. 进程自杀
	log.Println("🔥 [HeartbeatMonitor] 执行进程自杀")
	os.Exit(1)
}

// wipeEBPFMaps 清空所有 eBPF Map
func (m *HeartbeatMonitor) wipeEBPFMaps() error {
	log.Println("[HeartbeatMonitor] 清空 eBPF Map")
	
	if m.emergencyMgr == nil {
		return fmt.Errorf("紧急管理器未初始化")
	}
	
	// 触发 eBPF 紧急自毁
	if err := m.emergencyMgr.TriggerWipe(); err != nil {
		return fmt.Errorf("触发 eBPF 自毁失败: %w", err)
	}
	
	return nil
}

// wipeMemory 清空内存敏感数据
func (m *HeartbeatMonitor) wipeMemory() error {
	log.Println("[HeartbeatMonitor] 清空内存敏感数据")
	
	if m.ramShield == nil {
		log.Println("[HeartbeatMonitor] ⚠️ RAM Shield 未初始化，跳过内存清空")
		return nil
	}
	
	// 解锁所有锁定的内存
	if err := m.ramShield.UnlockAll(); err != nil {
		log.Printf("[HeartbeatMonitor] ⚠️ 解锁内存失败: %v", err)
	}
	
	// TODO: 清空具体的敏感数据结构
	// 1. G-Tunnel 私钥
	// 2. 用户会话数据
	// 3. 配置缓存
	
	return nil
}

// cleanupFiles 清理临时文件
func (m *HeartbeatMonitor) cleanupFiles() error {
	log.Println("[HeartbeatMonitor] 清理临时文件")
	
	// TODO: 实现文件清理
	// 删除:
	// - /tmp/mirage-*
	// - /var/log/mirage-*
	// - 配置文件备份
	
	// 示例代码:
	// os.RemoveAll("/tmp/mirage-gateway")
	// os.RemoveAll("/var/log/mirage-gateway.log")
	
	return nil
}

// UpdateHeartbeat 手动更新心跳时间（用于测试）
func (m *HeartbeatMonitor) UpdateHeartbeat() {
	m.lastHeartbeat = time.Now()
	log.Println("[HeartbeatMonitor] 手动更新心跳时间")
}

// GetTimeSinceLastHeartbeat 获取距离上次心跳的时间
func (m *HeartbeatMonitor) GetTimeSinceLastHeartbeat() time.Duration {
	return time.Since(m.lastHeartbeat)
}

// IsEmergencyMode 是否处于紧急模式
func (m *HeartbeatMonitor) IsEmergencyMode() bool {
	return m.emergencyMode
}
