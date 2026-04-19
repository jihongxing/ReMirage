// Package ebpf - eBPF 管理器
// 负责 Map 的读写封装和策略应用
package ebpf

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// DefenseApplier 防御策略应用器
type DefenseApplier struct {
	loader   *Loader
	config   *DefenseConfig
	stopCh   chan struct{}
	updateCh chan *DefenseConfig
}

// DefenseConfig 防御配置
type DefenseConfig struct {
	Level          uint32        // 防御等级 (10/20/30)
	JitterMeanUs   uint32        // Jitter 平均值 (微秒)
	JitterStddevUs uint32        // Jitter 标准差 (微秒)
	PaddingRate    uint32        // NPM 填充率 (%)
	NoiseIntensity uint32        // VPC 噪声强度 (%)
	UpdateInterval time.Duration // 更新间隔
}

// NewDefenseApplier 创建防御应用器
func NewDefenseApplier(loader *Loader, config *DefenseConfig) *DefenseApplier {
	return &DefenseApplier{
		loader:   loader,
		config:   config,
		stopCh:   make(chan struct{}),
		updateCh: make(chan *DefenseConfig, 10),
	}
}

// Start 启动防御应用器
func (da *DefenseApplier) Start() {
	// 应用初始策略
	if err := da.applyStrategy(da.config); err != nil {
		log.Printf("❌ 应用初始策略失败: %v", err)
		return
	}

	// 启动定期更新
	go da.updateLoop()

	// 监听系统信号
	go da.signalHandler()

	log.Println("✅ 防御应用器已启动")
}

// Stop 停止防御应用器
func (da *DefenseApplier) Stop() {
	close(da.stopCh)
	log.Println("🛑 防御应用器已停止")
}

// UpdateConfig 更新配置
func (da *DefenseApplier) UpdateConfig(config *DefenseConfig) error {
	select {
	case da.updateCh <- config:
		log.Printf("📝 配置更新请求已提交: Level=%d%%", config.Level)
		return nil
	default:
		log.Println("⚠️  配置更新队列已满，跳过")
		return fmt.Errorf("配置更新队列已满")
	}
}

// updateLoop 定期更新循环
func (da *DefenseApplier) updateLoop() {
	ticker := time.NewTicker(da.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-da.stopCh:
			return

		case newConfig := <-da.updateCh:
			if err := da.applyStrategy(newConfig); err != nil {
				log.Printf("❌ 应用策略失败: %v", err)
			} else {
				da.config = newConfig
				log.Printf("✅ 策略已更新: Level=%d%%", newConfig.Level)
			}

		case <-ticker.C:
			// 定期刷新策略（防止 Map 被意外清空）
			if err := da.applyStrategy(da.config); err != nil {
				log.Printf("⚠️  策略刷新失败: %v", err)
			}
		}
	}
}

// applyStrategy 应用防御策略
func (da *DefenseApplier) applyStrategy(config *DefenseConfig) error {
	strategy := &DefenseStrategy{
		JitterMeanUs:    config.JitterMeanUs,
		JitterStddevUs:  config.JitterStddevUs,
		TemplateID:      1,
		FiberJitterUs:   config.JitterMeanUs / 5,
		RouterDelayUs:   config.JitterMeanUs / 10,
		NoiseIntensity:  config.NoiseIntensity,
	}

	return da.loader.UpdateStrategy(strategy)
}

// signalHandler 信号处理器
func (da *DefenseApplier) signalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		select {
		case <-da.stopCh:
			return

		case sig := <-sigCh:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Printf("🛑 收到退出信号: %v", sig)
				da.cleanup()
				os.Exit(0)

			case syscall.SIGHUP:
				log.Println("🔄 收到重载信号，刷新策略...")
				if err := da.applyStrategy(da.config); err != nil {
					log.Printf("❌ 策略刷新失败: %v", err)
				}
			}
		}
	}
}

// cleanup 清理资源
func (da *DefenseApplier) cleanup() {
	log.Println("🧹 开始清理资源...")

	// 关闭 Loader
	if err := da.loader.Close(); err != nil {
		log.Printf("⚠️  关闭 Loader 失败: %v", err)
	}

	// 清理 TC 钩子
	if err := da.cleanupTC(); err != nil {
		log.Printf("⚠️  清理 TC 钩子失败: %v", err)
	}

	log.Println("✅ 资源清理完成")
}

// cleanupTC 清理 TC 钩子
func (da *DefenseApplier) cleanupTC() error {
	// 注意：这里需要使用 netlink 或 tc 命令清理
	// 简化实现：记录日志
	log.Printf("🧹 清理 TC 钩子: %s", da.loader.iface)

	// TODO: 实现 TC 清理逻辑
	// 可以使用: tc filter del dev eth0 egress
	// 或使用 netlink 库

	return nil
}

// GetCurrentConfig 获取当前配置
func (da *DefenseApplier) GetCurrentConfig() *DefenseConfig {
	return da.config
}

// AdjustDefenseLevel 调整防御等级
func (da *DefenseApplier) AdjustDefenseLevel(level uint32) error {
	if level != 10 && level != 20 && level != 30 {
		return fmt.Errorf("无效的防御等级: %d (必须是 10/20/30)", level)
	}

	newConfig := &DefenseConfig{
		Level:          level,
		JitterMeanUs:   50000,                    // 50ms
		JitterStddevUs: 15000,                    // 15ms
		PaddingRate:    level,                    // 与防御等级一致
		NoiseIntensity: level,                    // 与防御等级一致
		UpdateInterval: da.config.UpdateInterval, // 保持不变
	}

	da.UpdateConfig(newConfig)
	return nil
}
