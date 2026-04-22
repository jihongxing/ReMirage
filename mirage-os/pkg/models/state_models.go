// Package models - V2 三层状态模型 GORM 定义
package models

import "time"

// ============================================
// 枚举常量（与 orchestrator 侧一致）
// ============================================

const (
	LinkPhaseProbing     = "Probing"
	LinkPhaseActive      = "Active"
	LinkPhaseDegrading   = "Degrading"
	LinkPhaseStandby     = "Standby"
	LinkPhaseUnavailable = "Unavailable"

	SessionPhaseBootstrapping = "Bootstrapping"
	SessionPhaseActive        = "Active"
	SessionPhaseProtected     = "Protected"
	SessionPhaseMigrating     = "Migrating"
	SessionPhaseDegraded      = "Degraded"
	SessionPhaseSuspended     = "Suspended"
	SessionPhaseClosed        = "Closed"

	ControlHealthHealthy    = "Healthy"
	ControlHealthRecovering = "Recovering"
	ControlHealthFaulted    = "Faulted"
)

// ============================================
// V2 三层状态结构体
// ============================================

// V2LinkState 链路状态（mirage-os 侧 GORM 模型）
type V2LinkState struct {
	LinkID           string     `gorm:"column:link_id;primaryKey;size:64" json:"link_id"`
	TransportType    string     `gorm:"column:transport_type;size:32;not null" json:"transport_type"`
	GatewayID        string     `gorm:"column:gateway_id;index;size:32;not null" json:"gateway_id"`
	HealthScore      float64    `gorm:"column:health_score;type:numeric(5,2);default:0;check:health_score >= 0 AND health_score <= 100" json:"health_score"`
	RttMs            int64      `gorm:"column:rtt_ms;default:0" json:"rtt_ms"`
	LossRate         float64    `gorm:"column:loss_rate;type:numeric(5,4);default:0;check:loss_rate >= 0 AND loss_rate <= 1" json:"loss_rate"`
	JitterMs         int64      `gorm:"column:jitter_ms;default:0" json:"jitter_ms"`
	Phase            string     `gorm:"column:phase;size:16;not null;check:phase IN ('Probing','Active','Degrading','Standby','Unavailable')" json:"phase"`
	Available        bool       `gorm:"column:available;default:false" json:"available"`
	Degraded         bool       `gorm:"column:degraded;default:false" json:"degraded"`
	LastProbeAt      *time.Time `gorm:"column:last_probe_at" json:"last_probe_at"`
	LastSwitchReason string     `gorm:"column:last_switch_reason;size:256;default:''" json:"last_switch_reason"`
	CreatedAt        time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (V2LinkState) TableName() string { return "link_states" }

// V2SessionState 会话状态（mirage-os 侧 GORM 模型）
type V2SessionState struct {
	SessionID           string    `gorm:"column:session_id;primaryKey;size:64" json:"session_id"`
	UserID              string    `gorm:"column:user_id;index;size:64;not null" json:"user_id"`
	ClientID            string    `gorm:"column:client_id;size:64;not null" json:"client_id"`
	GatewayID           string    `gorm:"column:gateway_id;index;size:32;not null" json:"gateway_id"`
	ServiceClass        string    `gorm:"column:service_class;size:16;not null;check:service_class IN ('Standard','Platinum','Diamond')" json:"service_class"`
	Priority            int       `gorm:"column:priority;default:50;check:priority >= 0 AND priority <= 100" json:"priority"`
	CurrentPersonaID    string    `gorm:"column:current_persona_id;size:64;default:''" json:"current_persona_id"`
	CurrentLinkID       string    `gorm:"column:current_link_id;index;size:64" json:"current_link_id"`
	CurrentSurvivalMode string    `gorm:"column:current_survival_mode;size:16;default:'Normal'" json:"current_survival_mode"`
	BillingMode         string    `gorm:"column:billing_mode;size:32;default:''" json:"billing_mode"`
	State               string    `gorm:"column:state;size:16;not null;check:state IN ('Bootstrapping','Active','Protected','Migrating','Degraded','Suspended','Closed')" json:"state"`
	MigrationPending    bool      `gorm:"column:migration_pending;default:false" json:"migration_pending"`
	CreatedAt           time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (V2SessionState) TableName() string { return "session_states" }

// V2ControlState 控制状态（mirage-os 侧 GORM 模型）
type V2ControlState struct {
	GatewayID           string    `gorm:"column:gateway_id;primaryKey;size:32" json:"gateway_id"`
	Epoch               uint64    `gorm:"column:epoch;not null;default:0" json:"epoch"`
	PersonaVersion      uint64    `gorm:"column:persona_version;default:0" json:"persona_version"`
	RouteGeneration     uint64    `gorm:"column:route_generation;default:0" json:"route_generation"`
	ActiveTxID          string    `gorm:"column:active_tx_id;size:64;default:''" json:"active_tx_id"`
	RollbackMarker      uint64    `gorm:"column:rollback_marker;default:0" json:"rollback_marker"`
	LastSuccessfulEpoch uint64    `gorm:"column:last_successful_epoch;default:0" json:"last_successful_epoch"`
	LastSwitchReason    string    `gorm:"column:last_switch_reason;size:256;default:''" json:"last_switch_reason"`
	ControlHealth       string    `gorm:"column:control_health;size:16;default:'Healthy';check:control_health IN ('Healthy','Recovering','Faulted')" json:"control_health"`
	UpdatedAt           time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (V2ControlState) TableName() string { return "control_states" }
