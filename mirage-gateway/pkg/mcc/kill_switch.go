// Package mcc - 紧急物理开关 (Physical Kill Switch)
// 最高优先级信令：全网静默
package mcc

import (
	"context"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"mirage-gateway/pkg/storage"
)

// KillSwitchPriority 信令优先级
const (
	PriorityNormal   = 0
	PriorityHigh     = 1
	PriorityCritical = 2
	PriorityKill     = 255 // 最高优先级 - 全网静默
)

// KillSwitchState 开关状态
type KillSwitchState int

const (
	StateActive   KillSwitchState = iota // 正常运行
	StateArmed                           // 已武装（准备触发）
	StateTriggered                       // 已触发
	StateSilent                          // 静默完成
)

// KillSwitch 紧急物理开关
type KillSwitch struct {
	mu            sync.RWMutex
	state         KillSwitchState
	armCode       string // 武装码（防止误触发）
	triggerCode   string // 触发码
	lastArmedTime time.Time
	armTimeout    time.Duration // 武装超时（自动解除）

	// Vault 持久化
	vault *storage.VaultStorage

	// 回调
	OnArmed     func()
	OnTriggered func()
	OnSilent    func()

	// 清理函数
	cleanupFuncs []func() error
}

// NewKillSwitch 创建紧急开关
func NewKillSwitch(armCode, triggerCode string) *KillSwitch {
	return &KillSwitch{
		state:        StateActive,
		armCode:      armCode,
		triggerCode:  triggerCode,
		armTimeout:   5 * time.Minute, // 5 分钟内未触发自动解除
		cleanupFuncs: make([]func() error, 0),
	}
}

// NewKillSwitchWithVault 创建带持久化的紧急开关
func NewKillSwitchWithVault(armCode, triggerCode string, vault *storage.VaultStorage) *KillSwitch {
	ks := &KillSwitch{
		state:        StateActive,
		armCode:      armCode,
		triggerCode:  triggerCode,
		armTimeout:   5 * time.Minute,
		vault:        vault,
		cleanupFuncs: make([]func() error, 0),
	}

	// 检查 Vault 中的状态（防止重启后自动拉起）
	if vault != nil {
		state := vault.GetSystemState()
		if state == storage.StateShutdown || state == storage.StateDead {
			log.Printf("💀 [KillSwitch] 检测到 SHUTDOWN/DEAD 状态，拒绝启动")
			ks.state = StateSilent
		}
	}

	return ks
}

// RegisterCleanup 注册清理函数
func (ks *KillSwitch) RegisterCleanup(fn func() error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.cleanupFuncs = append(ks.cleanupFuncs, fn)
}

// Arm 武装开关（两阶段触发的第一阶段）
func (ks *KillSwitch) Arm(code string) bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if code != ks.armCode {
		log.Printf("⚠️  [KillSwitch] 武装码错误")
		return false
	}

	if ks.state == StateTriggered || ks.state == StateSilent {
		log.Printf("⚠️  [KillSwitch] 已触发，无法重新武装")
		return false
	}

	ks.state = StateArmed
	ks.lastArmedTime = time.Now()

	log.Printf("🔴 [KillSwitch] 已武装，等待触发码...")

	if ks.OnArmed != nil {
		go ks.OnArmed()
	}

	// 启动超时自动解除
	go ks.autoDisarm()

	return true
}

// autoDisarm 自动解除武装
func (ks *KillSwitch) autoDisarm() {
	time.Sleep(ks.armTimeout)

	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.state == StateArmed {
		ks.state = StateActive
		log.Printf("🟢 [KillSwitch] 武装超时，已自动解除")
	}
}

// Trigger 触发开关（两阶段触发的第二阶段）
func (ks *KillSwitch) Trigger(code string) bool {
	ks.mu.Lock()

	if code != ks.triggerCode {
		ks.mu.Unlock()
		log.Printf("⚠️  [KillSwitch] 触发码错误")
		return false
	}

	if ks.state != StateArmed {
		ks.mu.Unlock()
		log.Printf("⚠️  [KillSwitch] 未武装，无法触发")
		return false
	}

	ks.state = StateTriggered
	ks.mu.Unlock()

	log.Printf("💀 [KillSwitch] 触发！开始全网静默...")

	if ks.OnTriggered != nil {
		go ks.OnTriggered()
	}

	// 执行静默流程
	go ks.executeSilence()

	return true
}

// executeSilence 执行静默流程
func (ks *KillSwitch) executeSilence() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 0. 持久化状态为 SHUTDOWN（防止重启后自动拉起）
	if ks.vault != nil {
		log.Printf("💀 [KillSwitch] 持久化 SHUTDOWN 状态...")
		ks.vault.SetSystemState(storage.StateShutdown)
	}

	// 1. 执行所有注册的清理函数
	log.Printf("💀 [KillSwitch] 执行清理函数...")
	for i, fn := range ks.cleanupFuncs {
		if err := fn(); err != nil {
			log.Printf("⚠️  [KillSwitch] 清理函数 %d 失败: %v", i, err)
		}
	}

	// 2. 卸载 eBPF 程序
	log.Printf("💀 [KillSwitch] 卸载 eBPF 程序...")
	ks.unloadEBPF(ctx)

	// 3. 清空 tmpfs
	log.Printf("💀 [KillSwitch] 清空内存数据...")
	ks.clearTmpfs()

	// 4. 清空内存缓存
	log.Printf("💀 [KillSwitch] 清空系统缓存...")
	ks.dropCaches()

	// 5. 更新状态
	ks.mu.Lock()
	ks.state = StateSilent
	ks.mu.Unlock()

	log.Printf("💀 [KillSwitch] 静默完成")

	if ks.OnSilent != nil {
		go ks.OnSilent()
	}
}

// ExecutePhysicalWipe 执行物理抹除（自毁级别）
func (ks *KillSwitch) ExecutePhysicalWipe() {
	log.Printf("💀💀💀 [KillSwitch] 执行物理抹除...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 卸载 eBPF 程序
	ks.unloadEBPF(ctx)

	// 2. 清空 eBPF Map（bpf_map_delete_elem）
	log.Printf("💀 [KillSwitch] 清空 eBPF Map...")
	exec.CommandContext(ctx, "bpftool", "map", "delete", "name", "threat_level_map").Run()
	exec.CommandContext(ctx, "bpftool", "map", "delete", "name", "dna_config_map").Run()

	// 3. 物理抹除 Vault
	if ks.vault != nil {
		log.Printf("💀 [KillSwitch] 物理抹除 Vault...")
		ks.vault.PhysicalWipe()
	}

	// 4. 清空 tmpfs
	ks.clearTmpfs()

	// 5. 清空系统缓存
	ks.dropCaches()

	log.Printf("💀💀💀 [KillSwitch] 物理抹除完成，系统已死亡")
}

// unloadEBPF 卸载 eBPF 程序
func (ks *KillSwitch) unloadEBPF(ctx context.Context) {
	// 卸载 XDP 程序
	exec.CommandContext(ctx, "ip", "link", "set", "dev", "eth0", "xdp", "off").Run()

	// 卸载 TC 程序
	exec.CommandContext(ctx, "tc", "filter", "del", "dev", "eth0", "ingress").Run()
	exec.CommandContext(ctx, "tc", "filter", "del", "dev", "eth0", "egress").Run()

	// 使用 bpftool 清理残留
	exec.CommandContext(ctx, "bpftool", "prog", "detach", "xdp", "dev", "eth0").Run()
}

// clearTmpfs 清空 tmpfs
func (ks *KillSwitch) clearTmpfs() {
	paths := []string{
		"/opt/mirage/data",
		"/opt/mirage/logs",
	}

	for _, path := range paths {
		os.RemoveAll(path)
		os.MkdirAll(path, 0700)
	}
}

// dropCaches 清空系统缓存
func (ks *KillSwitch) dropCaches() {
	// sync 确保数据写入
	exec.Command("sync").Run()

	// 清空页面缓存、dentries 和 inodes
	f, err := os.OpenFile("/proc/sys/vm/drop_caches", os.O_WRONLY, 0)
	if err == nil {
		f.WriteString("3")
		f.Close()
	}
}

// GetState 获取当前状态
func (ks *KillSwitch) GetState() KillSwitchState {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.state
}

// IsActive 检查是否正常运行
func (ks *KillSwitch) IsActive() bool {
	return ks.GetState() == StateActive
}

// IsSilent 检查是否已静默
func (ks *KillSwitch) IsSilent() bool {
	state := ks.GetState()
	return state == StateTriggered || state == StateSilent
}

// --- M.C.C. 信令集成 ---

// KillSwitchSignal 全网静默信令
type KillSwitchSignal struct {
	Priority    int    `json:"priority"`
	ArmCode     string `json:"arm_code,omitempty"`
	TriggerCode string `json:"trigger_code,omitempty"`
	Timestamp   int64  `json:"timestamp"`
	Source      string `json:"source"`
}

// ProcessKillSwitchSignal 处理全网静默信令
func (ks *KillSwitch) ProcessKillSwitchSignal(signal *KillSwitchSignal) bool {
	// 验证优先级
	if signal.Priority != PriorityKill {
		return false
	}

	// 验证时间戳（防止重放攻击）
	signalTime := time.Unix(signal.Timestamp, 0)
	if time.Since(signalTime) > 5*time.Minute {
		log.Printf("⚠️  [KillSwitch] 信令过期，忽略")
		return false
	}

	// 两阶段触发
	if signal.ArmCode != "" && signal.TriggerCode == "" {
		return ks.Arm(signal.ArmCode)
	}

	if signal.TriggerCode != "" {
		return ks.Trigger(signal.TriggerCode)
	}

	return false
}
