// Package evaluator - 闭环反馈控制器
package evaluator

import (
	"context"
	"log"
)

// FeedbackController 反馈控制器
type FeedbackController struct {
	signalChannel <-chan FeedbackSignal
	strategyMgr   StrategyManager
	ctx           context.Context
	cancel        context.CancelFunc
	adjustCount   int
}

// StrategyManager 策略管理器接口
type StrategyManager interface {
	AdjustDNAParameters(mu, sigma float64) error
	SwitchChameleonProfile(profile string) error
	GetCurrentProfile() string
}

// NewFeedbackController 创建反馈控制器
func NewFeedbackController(signalCh <-chan FeedbackSignal, strategyMgr StrategyManager) *FeedbackController {
	ctx, cancel := context.WithCancel(context.Background())
	return &FeedbackController{
		signalChannel: signalCh,
		strategyMgr:   strategyMgr,
		ctx:           ctx,
		cancel:        cancel,
		adjustCount:   0,
	}
}

// Start 启动反馈控制器
func (fc *FeedbackController) Start() {
	log.Println("[Feedback] 启动闭环反馈控制器")
	go fc.processFeedback()
}

// Stop 停止反馈控制器
func (fc *FeedbackController) Stop() {
	log.Println("[Feedback] 停止反馈控制器")
	fc.cancel()
}

// processFeedback 处理反馈
func (fc *FeedbackController) processFeedback() {
	for {
		select {
		case <-fc.ctx.Done():
			return
		case signal := <-fc.signalChannel:
			fc.handleSignal(signal)
		}
	}
}

// handleSignal 处理信号
func (fc *FeedbackController) handleSignal(signal FeedbackSignal) {
	log.Printf("[Feedback] 收到反馈信号: 类型=%s, 置信度=%.2f%%", signal.Type, signal.Confidence)
	
	switch signal.Action {
	case "adjust_parameters":
		fc.adjustParameters(signal)
	case "switch_profile":
		fc.switchProfile(signal)
	case "emergency_stop":
		fc.emergencyStop(signal)
	default:
		log.Printf("[Feedback] ⚠️ 未知动作: %s", signal.Action)
	}
	
	fc.adjustCount++
}

// adjustParameters 调整参数
func (fc *FeedbackController) adjustParameters(signal FeedbackSignal) {
	log.Println("[Feedback] 调整 B-DNA 参数")
	
	// 根据置信度调整参数
	// 置信度越高，说明越容易被识别，需要增加随机性
	
	var muAdjust, sigmaAdjust float64
	
	if signal.Confidence > 80 {
		// 高置信度：大幅调整
		muAdjust = 1.2
		sigmaAdjust = 1.5
		log.Println("[Feedback] 高置信度检测，大幅增加随机性")
	} else if signal.Confidence > 50 {
		// 中等置信度：中等调整
		muAdjust = 1.1
		sigmaAdjust = 1.3
		log.Println("[Feedback] 中等置信度检测，适度增加随机性")
	} else {
		// 低置信度：小幅调整
		muAdjust = 1.05
		sigmaAdjust = 1.1
		log.Println("[Feedback] 低置信度检测，轻微调整")
	}
	
	// 应用调整
	if err := fc.strategyMgr.AdjustDNAParameters(muAdjust, sigmaAdjust); err != nil {
		log.Printf("[Feedback] ❌ 调整参数失败: %v", err)
		return
	}
	
	log.Printf("[Feedback] ✅ 参数已调整: μ×%.2f, σ×%.2f", muAdjust, sigmaAdjust)
}

// switchProfile 切换配置文件
func (fc *FeedbackController) switchProfile(signal FeedbackSignal) {
	log.Println("[Feedback] 切换 Chameleon 配置文件")
	
	currentProfile := fc.strategyMgr.GetCurrentProfile()
	
	// 配置文件轮换策略
	profiles := []string{"zoom-windows", "chrome-windows", "teams-windows"}
	nextProfile := ""
	
	for i, p := range profiles {
		if p == currentProfile {
			nextProfile = profiles[(i+1)%len(profiles)]
			break
		}
	}
	
	if nextProfile == "" {
		nextProfile = profiles[0]
	}
	
	if err := fc.strategyMgr.SwitchChameleonProfile(nextProfile); err != nil {
		log.Printf("[Feedback] ❌ 切换配置文件失败: %v", err)
		return
	}
	
	log.Printf("[Feedback] ✅ 已切换配置文件: %s → %s", currentProfile, nextProfile)
}

// emergencyStop 紧急停止
func (fc *FeedbackController) emergencyStop(signal FeedbackSignal) {
	log.Println("[Feedback] 🚨 触发紧急停止")
	
	// TODO: 实现紧急停止逻辑
	// 1. 停止所有流量
	// 2. 触发自毁程序
	// 3. 通知 Mirage-OS
	
	log.Println("[Feedback] ⚠️ 紧急停止功能未完全实现")
}

// GetAdjustCount 获取调整次数
func (fc *FeedbackController) GetAdjustCount() int {
	return fc.adjustCount
}
