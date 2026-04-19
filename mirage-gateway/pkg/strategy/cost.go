// Package strategy - 策略计算器
// 负责防御等级到实际开销的数学转换
package strategy

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"
)

// CostCalculator 成本计算器
type CostCalculator struct {
	defenseLevel   uint32  // 防御等级 (10/20/30)
	baseTrafficGB  float64 // 基础流量 (GB/月)
	pricePerGB     float64 // 单价 ($/GB)
	bytesProcessed uint64  // 已处理字节数
	startTime      time.Time
	stopCh         chan struct{}
}

// CostReport 成本报告
type CostReport struct {
	DefenseLevel      uint32  // 防御等级
	BaseTrafficGB     float64 // 基础流量
	PaddingTrafficGB  float64 // 填充流量
	TotalTrafficGB    float64 // 总流量
	BaseCost          float64 // 基础成本
	PaddingCost       float64 // 填充成本
	TotalCost         float64 // 总成本
	CostIncrease      float64 // 成本增加 (%)
	BytesProcessed    uint64  // 已处理字节数
	EstimatedMonthly  float64 // 预估月成本
}

// NewCostCalculator 创建成本计算器
func NewCostCalculator(defenseLevel uint32, baseTrafficGB float64) *CostCalculator {
	return &CostCalculator{
		defenseLevel:  defenseLevel,
		baseTrafficGB: baseTrafficGB,
		pricePerGB:    0.10, // $0.10/GB
		startTime:     time.Now(),
		stopCh:        make(chan struct{}),
	}
}

// Start 启动成本计算器
func (cc *CostCalculator) Start() {
	go cc.reportLoop()
	log.Println("💰 成本计算器已启动")
}

// Stop 停止成本计算器
func (cc *CostCalculator) Stop() {
	close(cc.stopCh)
	log.Println("💰 成本计算器已停止")
}

// UpdateDefenseLevel 更新防御等级
func (cc *CostCalculator) UpdateDefenseLevel(level uint32) {
	atomic.StoreUint32(&cc.defenseLevel, level)
	log.Printf("💰 防御等级已更新: %d%%", level)
}

// RecordBytes 记录处理的字节数
func (cc *CostCalculator) RecordBytes(bytes uint64) {
	atomic.AddUint64(&cc.bytesProcessed, bytes)
}

// Calculate 计算成本
func (cc *CostCalculator) Calculate() *CostReport {
	level := atomic.LoadUint32(&cc.defenseLevel)
	processed := atomic.LoadUint64(&cc.bytesProcessed)

	// 转换为 GB
	processedGB := float64(processed) / (1024 * 1024 * 1024)

	// 计算填充流量
	// NPM 填充率 = 防御等级
	// 例如：20% 防御等级 = 20% 额外流量
	paddingRate := float64(level) / 100.0
	paddingTrafficGB := processedGB * paddingRate

	// 总流量
	totalTrafficGB := processedGB + paddingTrafficGB

	// 计算成本
	baseCost := processedGB * cc.pricePerGB
	paddingCost := paddingTrafficGB * cc.pricePerGB
	totalCost := totalTrafficGB * cc.pricePerGB

	// 成本增加百分比
	costIncrease := 0.0
	if baseCost > 0 {
		costIncrease = (paddingCost / baseCost) * 100
	}

	// 预估月成本
	elapsed := time.Since(cc.startTime)
	monthlyMultiplier := float64(30*24*time.Hour) / float64(elapsed)
	estimatedMonthly := totalCost * monthlyMultiplier

	return &CostReport{
		DefenseLevel:     level,
		BaseTrafficGB:    processedGB,
		PaddingTrafficGB: paddingTrafficGB,
		TotalTrafficGB:   totalTrafficGB,
		BaseCost:         baseCost,
		PaddingCost:      paddingCost,
		TotalCost:        totalCost,
		CostIncrease:     costIncrease,
		BytesProcessed:   processed,
		EstimatedMonthly: estimatedMonthly,
	}
}

// reportLoop 报告循环
func (cc *CostCalculator) reportLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cc.stopCh:
			return

		case <-ticker.C:
			report := cc.Calculate()
			cc.printReport(report)
		}
	}
}

// printReport 打印报告
func (cc *CostCalculator) printReport(report *CostReport) {
	if report.BytesProcessed == 0 {
		return
	}

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("💰 战损报告 (Cost Report)")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("🛡️  防御等级: %d%%", report.DefenseLevel)
	log.Printf("📊 基础流量: %.3f GB", report.BaseTrafficGB)
	log.Printf("🎭 填充流量: %.3f GB (+%.1f%%)", report.PaddingTrafficGB, report.CostIncrease)
	log.Printf("📈 总流量:   %.3f GB", report.TotalTrafficGB)
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("💵 基础成本: $%.4f", report.BaseCost)
	log.Printf("💸 填充成本: $%.4f", report.PaddingCost)
	log.Printf("💰 总成本:   $%.4f", report.TotalCost)
	log.Printf("📅 预估月成本: $%.2f", report.EstimatedMonthly)
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 成本告警
	if report.EstimatedMonthly > 100 {
		log.Println("⚠️  [成本告警] 预估月成本超过 $100，建议降低防御等级")
	}
}

// GetReport 获取报告
func (cc *CostCalculator) GetReport() *CostReport {
	return cc.Calculate()
}

// FormatReport 格式化报告（用于 API 输出）
func (cc *CostCalculator) FormatReport() string {
	report := cc.Calculate()

	return fmt.Sprintf(`
防御等级: %d%%
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
流量统计:
  基础流量: %.3f GB
  填充流量: %.3f GB (+%.1f%%)
  总流量:   %.3f GB

成本统计:
  基础成本: $%.4f
  填充成本: $%.4f
  总成本:   $%.4f
  预估月成本: $%.2f
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`,
		report.DefenseLevel,
		report.BaseTrafficGB,
		report.PaddingTrafficGB,
		report.CostIncrease,
		report.TotalTrafficGB,
		report.BaseCost,
		report.PaddingCost,
		report.TotalCost,
		report.EstimatedMonthly,
	)
}

// CalculateOptimalLevel 计算最优防御等级
// 根据预算和威胁等级推荐防御等级
func CalculateOptimalLevel(monthlyBudget float64, threatLevel int) uint32 {
	// 威胁等级 0-10
	// 预算 $0-1000

	switch {
	case threatLevel >= 8:
		// 高威胁：优先安全
		return 30
	case threatLevel >= 5:
		// 中威胁：平衡模式
		if monthlyBudget >= 200 {
			return 30
		}
		return 20
	case threatLevel >= 2:
		// 低威胁：经济模式
		if monthlyBudget >= 100 {
			return 20
		}
		return 10
	default:
		// 无威胁：最低防御
		return 10
	}
}

// EstimateMonthlyCost 预估月成本
func EstimateMonthlyCost(baseTrafficGB float64, defenseLevel uint32) float64 {
	pricePerGB := 0.10
	paddingRate := float64(defenseLevel) / 100.0
	totalTrafficGB := baseTrafficGB * (1 + paddingRate)
	return totalTrafficGB * pricePerGB
}
