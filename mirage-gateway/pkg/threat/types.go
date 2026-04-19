// Package threat - 威胁编排模块
package threat

import "time"

// ThreatEventType 威胁事件类型
type ThreatEventType int

const (
	ThreatActiveProbing ThreatEventType = iota + 1
	ThreatReplayAttack
	ThreatTimingAttack
	ThreatDPIDetection
	ThreatJA4Scan
	ThreatSNIProbe
	ThreatHighRiskFingerprint
	ThreatAnomalyDetected
)

// EventSource 事件来源
type EventSource int

const (
	SourceEBPF EventSource = iota + 1
	SourceCortex
	SourceEvaluator
)

// ThreatLevel 威胁等级
type ThreatLevel int

const (
	LevelLow      ThreatLevel = 1
	LevelMedium   ThreatLevel = 2
	LevelHigh     ThreatLevel = 3
	LevelCritical ThreatLevel = 4
	LevelExtreme  ThreatLevel = 5
)

// UnifiedThreatEvent 统一威胁事件格式
type UnifiedThreatEvent struct {
	Timestamp  time.Time
	EventType  ThreatEventType
	SourceIP   string
	SourcePort uint16
	Severity   int // 0-10
	Source     EventSource
	Count      int // 聚合计数
	RawData    interface{}
}

// BlacklistSource 黑名单来源
type BlacklistSource int

const (
	SourceLocal BlacklistSource = iota
	SourceGlobal
)

// BlacklistEntry 黑名单条目
type BlacklistEntry struct {
	CIDR     string
	AddedAt  time.Time
	ExpireAt time.Time
	Source   BlacklistSource
}
