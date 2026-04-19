// Package ebpf - 威胁监控器
// 负责从内核 Ring Buffer 读取威胁事件并上报
package ebpf

import (
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"
)

// ThreatMonitor 威胁监控器
type ThreatMonitor struct {
	reader    *RingBufferReader
	stats     *MonitorStats
	stopCh    chan struct{}
	callbacks []ThreatCallback
	engine    ThreatEngine // 策略引擎接口
}

// ThreatEngine 策略引擎接口
type ThreatEngine interface {
	UpdateByThreat(threatType uint8, severity uint32)
}

// MonitorStats 监控统计
type MonitorStats struct {
	TotalEvents    uint64 // 总事件数
	ActiveProbing  uint64 // 主动探测
	ReplayAttack   uint64 // 重放攻击
	TimingAttack   uint64 // 时序攻击
	DPIDetection   uint64 // DPI 检测
	LastEventTime  int64  // 最后事件时间
	EventsPerMin   uint64 // 每分钟事件数
}

// ThreatCallback 威胁回调函数
type ThreatCallback func(*ThreatEvent)

// NewThreatMonitor 创建威胁监控器
func NewThreatMonitor(reader *RingBufferReader) *ThreatMonitor {
	return &ThreatMonitor{
		reader:    reader,
		stats:     &MonitorStats{},
		stopCh:    make(chan struct{}),
		callbacks: make([]ThreatCallback, 0),
		engine:    nil, // 稍后通过 SetEngine 设置
	}
}

// SetEngine 设置策略引擎
func (tm *ThreatMonitor) SetEngine(engine ThreatEngine) {
	tm.engine = engine
}

// Start 启动监控
func (tm *ThreatMonitor) Start() {
	// 启动 Ring Buffer 读取
	tm.reader.Start()

	// 启动统计输出
	go tm.statsReporter()

	log.Println("🔍 威胁监控器已启动")
}

// Stop 停止监控
func (tm *ThreatMonitor) Stop() error {
	close(tm.stopCh)
	return tm.reader.Stop()
}

// RegisterCallback 注册威胁回调
func (tm *ThreatMonitor) RegisterCallback(cb ThreatCallback) {
	tm.callbacks = append(tm.callbacks, cb)
}

// handleThreatEvent 处理威胁事件（实现 ThreatEventHandler 接口）
func (tm *ThreatMonitor) handleThreatEvent(event *ThreatEvent) error {
	// 更新统计
	atomic.AddUint64(&tm.stats.TotalEvents, 1)
	atomic.StoreInt64(&tm.stats.LastEventTime, time.Now().Unix())

	// 分类统计
	switch EventType(event.ThreatType) {
	case EventActiveProbing:
		atomic.AddUint64(&tm.stats.ActiveProbing, 1)
	case EventReplayAttack:
		atomic.AddUint64(&tm.stats.ReplayAttack, 1)
	case EventTimingAttack:
		atomic.AddUint64(&tm.stats.TimingAttack, 1)
	case EventDPIDetection:
		atomic.AddUint64(&tm.stats.DPIDetection, 1)
	}

	// 格式化输出
	tm.logThreatEvent(event)

	// 调用回调函数
	for _, cb := range tm.callbacks {
		cb(event)
	}

	return nil
}

// logThreatEvent 记录威胁事件
func (tm *ThreatMonitor) logThreatEvent(event *ThreatEvent) {
	ip := intToIP(event.SourceIP)
	eventType := tm.getEventTypeName(EventType(event.ThreatType))
	severity := tm.getSeverityLevel(event.Severity)

	// 红色高亮输出
	log.Printf("\033[1;31m[🚨 威胁预警]\033[0m 类型=%s | 来源=%s:%d | 严重程度=%s(%d) | 包计数=%d | 时间=%s",
		eventType,
		ip,
		event.SourcePort,
		severity,
		event.Severity,
		event.PacketCount,
		time.Unix(0, int64(event.Timestamp)).Format("15:04:05.000"),
	)

	// 高危威胁额外告警
	if event.Severity >= 8 {
		log.Printf("\033[1;31m🚨 [高危告警] 检测到严重威胁！建议立即提升防御等级\033[0m")
	}

	// 调用策略引擎自动调整
	if tm.engine != nil {
		tm.engine.UpdateByThreat(uint8(event.ThreatType), event.Severity)
	}
}

// statsReporter 统计报告器
func (tm *ThreatMonitor) statsReporter() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	lastTotal := uint64(0)

	for {
		select {
		case <-tm.stopCh:
			return

		case <-ticker.C:
			total := atomic.LoadUint64(&tm.stats.TotalEvents)
			eventsPerMin := total - lastTotal
			lastTotal = total

			atomic.StoreUint64(&tm.stats.EventsPerMin, eventsPerMin)

			if total > 0 {
				log.Printf("📊 [威胁统计] 总计=%d | 本分钟=%d | 主动探测=%d | 重放=%d | 时序=%d | DPI=%d",
					total,
					eventsPerMin,
					atomic.LoadUint64(&tm.stats.ActiveProbing),
					atomic.LoadUint64(&tm.stats.ReplayAttack),
					atomic.LoadUint64(&tm.stats.TimingAttack),
					atomic.LoadUint64(&tm.stats.DPIDetection),
				)

				// 威胁趋势分析
				if eventsPerMin > 100 {
					log.Println("🚨 [趋势告警] 威胁事件激增，可能正在遭受攻击！")
				}
			}
		}
	}
}

// GetStats 获取统计信息
func (tm *ThreatMonitor) GetStats() *MonitorStats {
	return &MonitorStats{
		TotalEvents:   atomic.LoadUint64(&tm.stats.TotalEvents),
		ActiveProbing: atomic.LoadUint64(&tm.stats.ActiveProbing),
		ReplayAttack:  atomic.LoadUint64(&tm.stats.ReplayAttack),
		TimingAttack:  atomic.LoadUint64(&tm.stats.TimingAttack),
		DPIDetection:  atomic.LoadUint64(&tm.stats.DPIDetection),
		LastEventTime: atomic.LoadInt64(&tm.stats.LastEventTime),
		EventsPerMin:  atomic.LoadUint64(&tm.stats.EventsPerMin),
	}
}

// getEventTypeName 获取事件类型名称
func (tm *ThreatMonitor) getEventTypeName(t EventType) string {
	switch t {
	case EventActiveProbing:
		return "主动探测"
	case EventReplayAttack:
		return "重放攻击"
	case EventTimingAttack:
		return "时序攻击"
	case EventDPIDetection:
		return "DPI检测"
	default:
		return fmt.Sprintf("未知(%d)", t)
	}
}

// getSeverityLevel 获取严重程度等级
func (tm *ThreatMonitor) getSeverityLevel(severity uint32) string {
	switch {
	case severity >= 9:
		return "🔴 极高"
	case severity >= 7:
		return "🟠 高"
	case severity >= 5:
		return "🟡 中"
	case severity >= 3:
		return "🟢 低"
	default:
		return "⚪ 极低"
	}
}

// intToIP 将 uint32 转换为 IP 地址
func intToIP(ip uint32) string {
	return net.IPv4(
		byte(ip),
		byte(ip>>8),
		byte(ip>>16),
		byte(ip>>24),
	).String()
}

// ExportToMirageOS 导出威胁数据到 Mirage-OS
// 这是为未来的 Mirage-OS 集成预留的接口
func (tm *ThreatMonitor) ExportToMirageOS() map[string]interface{} {
	stats := tm.GetStats()

	return map[string]interface{}{
		"total_events":    stats.TotalEvents,
		"active_probing":  stats.ActiveProbing,
		"replay_attack":   stats.ReplayAttack,
		"timing_attack":   stats.TimingAttack,
		"dpi_detection":   stats.DPIDetection,
		"last_event_time": stats.LastEventTime,
		"events_per_min":  stats.EventsPerMin,
		"threat_level":    tm.calculateThreatLevel(stats),
	}
}

// calculateThreatLevel 计算威胁等级
func (tm *ThreatMonitor) calculateThreatLevel(stats *MonitorStats) int {
	// 简单的威胁等级计算
	// 0: 无威胁
	// 1-3: 低威胁
	// 4-6: 中威胁
	// 7-9: 高威胁
	// 10: 极高威胁

	eventsPerMin := stats.EventsPerMin

	switch {
	case eventsPerMin == 0:
		return 0
	case eventsPerMin < 10:
		return 2
	case eventsPerMin < 50:
		return 5
	case eventsPerMin < 100:
		return 7
	default:
		return 10
	}
}
