package gtclient

import "time"

// ConnState represents the connection state of the GTunnel client.
type ConnState int32

const (
	StateInit          ConnState = iota // 初始化
	StateBootstrapping                  // 正在探测 bootstrap 节点
	StateConnected                      // 已连接
	StateSuspicious                     // 链路可疑（暂停控制面写入）
	StateDegraded                       // 连接质量下降
	StateReconnecting                   // 正在重连
	StateExhausted                      // 所有重连策略耗尽
	StateStopped                        // 已停止
)

var stateNames = [...]string{
	"Init",
	"Bootstrapping",
	"Connected",
	"Suspicious",
	"Degraded",
	"Reconnecting",
	"Exhausted",
	"Stopped",
}

func (s ConnState) String() string {
	if int(s) >= 0 && int(s) < len(stateNames) {
		return stateNames[s]
	}
	return "Unknown"
}

// DegradationLevel 退化等级
type DegradationLevel int32

const (
	L1_Normal     DegradationLevel = iota // 使用 RuntimeTopology 正常连接
	L2_Degraded                           // 回退到 BootstrapPool
	L3_LastResort                         // 进入 Resonance 绝境发现
)

var degradationNames = [...]string{
	"L1_Normal",
	"L2_Degraded",
	"L3_LastResort",
}

func (d DegradationLevel) String() string {
	if int(d) >= 0 && int(d) < len(degradationNames) {
		return degradationNames[d]
	}
	return "Unknown"
}

// DegradationEvent 退化事件
type DegradationEvent struct {
	Level     DegradationLevel `json:"level"`
	Reason    string           `json:"reason"`
	EnteredAt time.Time        `json:"entered_at"`
	Attempts  int              `json:"attempts"`
	Duration  time.Duration    `json:"duration"` // 恢复耗时（仅从高等级恢复到低等级时填充）
}

// NewDegradationEvent creates a DegradationEvent for entering a new level.
func NewDegradationEvent(level DegradationLevel, reason string, attempts int) DegradationEvent {
	return DegradationEvent{
		Level:     level,
		Reason:    reason,
		EnteredAt: time.Now(),
		Attempts:  attempts,
	}
}

// NewRecoveryEvent creates a DegradationEvent for recovering to a lower level.
func NewRecoveryEvent(level DegradationLevel, reason string, attempts int, duration time.Duration) DegradationEvent {
	return DegradationEvent{
		Level:     level,
		Reason:    reason,
		EnteredAt: time.Now(),
		Attempts:  attempts,
		Duration:  duration,
	}
}
