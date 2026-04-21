package cortex

import (
	"log"
	"time"
)

// FingerprintReporter 高危指纹检测事件上报器
type FingerprintReporter struct {
	bus *ThreatBus
}

// NewFingerprintReporter 创建指纹上报器
func NewFingerprintReporter(bus *ThreatBus) *FingerprintReporter {
	return &FingerprintReporter{bus: bus}
}

// ReportHighRisk 上报高危指纹检测事件到 ThreatBus
func (r *FingerprintReporter) ReportHighRisk(fp *IdentityFingerprint) {
	if r.bus == nil || fp == nil {
		return
	}

	sourceIP := ""
	if len(fp.AssociatedIPs) > 0 {
		sourceIP = fp.AssociatedIPs[len(fp.AssociatedIPs)-1]
	}

	event := &HighSeverityEvent{
		ID:          "fp_" + fp.UID,
		Timestamp:   time.Now().UnixMilli(),
		ThreatType:  EventFingerprint,
		Severity:    8,
		SourceIP:    sourceIP,
		Fingerprint: fp.Hash,
		Blocked:     false,
	}

	r.bus.EmitHighSeverityEvent(event)
	log.Printf("[FingerprintReporter] 高危指纹上报: hash=%s IP=%s score=%d",
		fp.Hash, sourceIP, fp.ThreatScore)
}

// BindToAnalyzer 将上报器绑定到 Analyzer 的 OnHighRisk 回调
func (r *FingerprintReporter) BindToAnalyzer(analyzer *Analyzer) {
	analyzer.OnHighRisk(func(fp *IdentityFingerprint) {
		r.ReportHighRisk(fp)
	})
}
