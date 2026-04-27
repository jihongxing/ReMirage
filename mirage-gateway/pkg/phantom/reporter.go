package phantom

import (
	"log"
	"time"

	"mirage-gateway/pkg/cortex"
	"mirage-gateway/pkg/redact"
	"mirage-gateway/pkg/threat"
)

// HoneypotReporter 蜜罐命中事件上报器
type HoneypotReporter struct {
	bus *cortex.ThreatBus
}

// NewHoneypotReporter 创建蜜罐上报器
func NewHoneypotReporter(bus *cortex.ThreatBus) *HoneypotReporter {
	return &HoneypotReporter{bus: bus}
}

// ReportAccess 上报蜜罐命中事件到 ThreatBus
func (r *HoneypotReporter) ReportAccess(record *AccessRecord) {
	// 9.6: 递增蜜罐命中指标
	threat.HoneypotHitTotal.WithLabelValues(threat.GetGatewayID()).Inc()

	if r.bus == nil {
		return
	}

	event := &cortex.HighSeverityEvent{
		ID:         "hp_" + record.Timestamp.Format("20060102150405"),
		Timestamp:  record.Timestamp.UnixMilli(),
		ThreatType: cortex.EventHoneypot,
		Severity:   8,
		SourceIP:   record.RemoteAddr,
		Blocked:    false,
	}

	r.bus.EmitHighSeverityEvent(event)
	log.Printf("[HoneypotReporter] 蜜罐命中上报: IP=%s Path=%s", redact.RedactIP(record.RemoteAddr), record.Path)
}

// SyncEBPFStats 将 eBPF 数据面 STAT_REDIRECTED 同步到 Prometheus 指标
func (r *HoneypotReporter) SyncEBPFStats(mgr *Manager) {
	stats := mgr.GetPhantomStats()
	// 使用 Add 将 eBPF 侧累计重定向数同步到 mirage_honeypot_hit_total
	// 注意：Prometheus Counter 只能递增，这里用差值方式
	threat.HoneypotHitTotal.WithLabelValues(threat.GetGatewayID()).Add(float64(stats.Redirected))
}

// BindToHoneypot 将上报器绑定到蜜罐服务器的 onAccess 回调
func (r *HoneypotReporter) BindToHoneypot(server *HoneypotServer) {
	server.OnAccess(func(record *AccessRecord) {
		r.ReportAccess(record)
	})
}

// ReportCanaryTrigger 上报金丝雀触发事件
func (r *HoneypotReporter) ReportCanaryTrigger(token *CanaryToken, ip string) {
	if r.bus == nil {
		return
	}

	event := &cortex.HighSeverityEvent{
		ID:         "canary_" + token.ID,
		Timestamp:  time.Now().UnixMilli(),
		ThreatType: cortex.EventHoneypot,
		Severity:   9,
		SourceIP:   ip,
		Blocked:    false,
	}

	r.bus.EmitHighSeverityEvent(event)
	log.Printf("[HoneypotReporter] 金丝雀触发上报: IP=%s TokenID=%s", redact.RedactIP(ip), token.ID)
}
