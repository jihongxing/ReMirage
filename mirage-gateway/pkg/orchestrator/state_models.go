package orchestrator

import "time"

// ============================================
// 枚举定义
// ============================================

// LinkPhase 链路阶段枚举
type LinkPhase string

const (
	LinkPhaseProbing     LinkPhase = "Probing"
	LinkPhaseActive      LinkPhase = "Active"
	LinkPhaseDegrading   LinkPhase = "Degrading"
	LinkPhaseStandby     LinkPhase = "Standby"
	LinkPhaseUnavailable LinkPhase = "Unavailable"
)

// AllLinkPhases 所有合法 LinkPhase 值
var AllLinkPhases = []LinkPhase{
	LinkPhaseProbing, LinkPhaseActive, LinkPhaseDegrading,
	LinkPhaseStandby, LinkPhaseUnavailable,
}

// SessionPhase 会话阶段枚举
type SessionPhase string

const (
	SessionPhaseBootstrapping SessionPhase = "Bootstrapping"
	SessionPhaseActive        SessionPhase = "Active"
	SessionPhaseProtected     SessionPhase = "Protected"
	SessionPhaseMigrating     SessionPhase = "Migrating"
	SessionPhaseDegraded      SessionPhase = "Degraded"
	SessionPhaseSuspended     SessionPhase = "Suspended"
	SessionPhaseClosed        SessionPhase = "Closed"
)

// AllSessionPhases 所有合法 SessionPhase 值
var AllSessionPhases = []SessionPhase{
	SessionPhaseBootstrapping, SessionPhaseActive, SessionPhaseProtected,
	SessionPhaseMigrating, SessionPhaseDegraded, SessionPhaseSuspended,
	SessionPhaseClosed,
}

// ControlHealth 控制面健康枚举
type ControlHealth string

const (
	ControlHealthHealthy    ControlHealth = "Healthy"
	ControlHealthRecovering ControlHealth = "Recovering"
	ControlHealthFaulted    ControlHealth = "Faulted"
)

// ServiceClass 服务等级
type ServiceClass string

const (
	ServiceClassStandard ServiceClass = "Standard"
	ServiceClassPlatinum ServiceClass = "Platinum"
	ServiceClassDiamond  ServiceClass = "Diamond"
)

// SurvivalMode 生存姿态枚举（本 Spec 仅定义枚举，实现在 Spec 5-2）
type SurvivalMode string

const (
	SurvivalModeNormal     SurvivalMode = "Normal"
	SurvivalModeLowNoise   SurvivalMode = "LowNoise"
	SurvivalModeHardened   SurvivalMode = "Hardened"
	SurvivalModeDegraded   SurvivalMode = "Degraded"
	SurvivalModeEscape     SurvivalMode = "Escape"
	SurvivalModeLastResort SurvivalMode = "LastResort"
)

// ============================================
// 状态转换表
// ============================================

// linkTransitions 合法的 Link 状态转换
var linkTransitions = map[[2]LinkPhase]bool{
	{LinkPhaseProbing, LinkPhaseActive}:        true,
	{LinkPhaseProbing, LinkPhaseUnavailable}:   true,
	{LinkPhaseActive, LinkPhaseDegrading}:      true,
	{LinkPhaseActive, LinkPhaseStandby}:        true,
	{LinkPhaseDegrading, LinkPhaseActive}:      true,
	{LinkPhaseDegrading, LinkPhaseStandby}:     true,
	{LinkPhaseDegrading, LinkPhaseUnavailable}: true,
	{LinkPhaseStandby, LinkPhaseProbing}:       true,
	{LinkPhaseStandby, LinkPhaseUnavailable}:   true,
	{LinkPhaseUnavailable, LinkPhaseProbing}:   true,
}

// IsValidLinkTransition 检查 Link 状态转换是否合法
func IsValidLinkTransition(from, to LinkPhase) bool {
	return linkTransitions[[2]LinkPhase{from, to}]
}

// sessionTransitions 合法的 Session 状态转换
var sessionTransitions = map[[2]SessionPhase]bool{
	{SessionPhaseBootstrapping, SessionPhaseActive}: true,
	{SessionPhaseBootstrapping, SessionPhaseClosed}: true,
	{SessionPhaseActive, SessionPhaseProtected}:     true,
	{SessionPhaseActive, SessionPhaseMigrating}:     true,
	{SessionPhaseActive, SessionPhaseDegraded}:      true,
	{SessionPhaseActive, SessionPhaseSuspended}:     true,
	{SessionPhaseActive, SessionPhaseClosed}:        true,
	{SessionPhaseProtected, SessionPhaseActive}:     true,
	{SessionPhaseProtected, SessionPhaseMigrating}:  true,
	{SessionPhaseProtected, SessionPhaseDegraded}:   true,
	{SessionPhaseMigrating, SessionPhaseActive}:     true,
	{SessionPhaseMigrating, SessionPhaseDegraded}:   true,
	{SessionPhaseMigrating, SessionPhaseClosed}:     true,
	{SessionPhaseDegraded, SessionPhaseActive}:      true,
	{SessionPhaseDegraded, SessionPhaseSuspended}:   true,
	{SessionPhaseDegraded, SessionPhaseClosed}:      true,
	{SessionPhaseSuspended, SessionPhaseActive}:     true,
	{SessionPhaseSuspended, SessionPhaseClosed}:     true,
}

// IsValidSessionTransition 检查 Session 状态转换是否合法
func IsValidSessionTransition(from, to SessionPhase) bool {
	return sessionTransitions[[2]SessionPhase{from, to}]
}

// ============================================
// 结构体定义
// ============================================

// LinkState 链路状态
type LinkState struct {
	LinkID           string     `gorm:"column:link_id;primaryKey;size:64" json:"link_id"`
	TransportType    string     `gorm:"column:transport_type;size:32;not null" json:"transport_type"`
	GatewayID        string     `gorm:"column:gateway_id;index;size:32;not null" json:"gateway_id"`
	HealthScore      float64    `gorm:"column:health_score;type:numeric(5,2);default:0;check:health_score >= 0 AND health_score <= 100" json:"health_score"`
	RttMs            int64      `gorm:"column:rtt_ms;default:0" json:"rtt_ms"`
	LossRate         float64    `gorm:"column:loss_rate;type:numeric(5,4);default:0;check:loss_rate >= 0 AND loss_rate <= 1" json:"loss_rate"`
	JitterMs         int64      `gorm:"column:jitter_ms;default:0" json:"jitter_ms"`
	Phase            LinkPhase  `gorm:"column:phase;size:16;not null;check:phase IN ('Probing','Active','Degrading','Standby','Unavailable')" json:"phase"`
	Available        bool       `gorm:"column:available;default:false" json:"available"`
	Degraded         bool       `gorm:"column:degraded;default:false" json:"degraded"`
	LastProbeAt      *time.Time `gorm:"column:last_probe_at" json:"last_probe_at"`
	LastSwitchReason string     `gorm:"column:last_switch_reason;size:256;default:''" json:"last_switch_reason"`
	CreatedAt        time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (LinkState) TableName() string { return "link_states" }

// SessionState 会话状态
type SessionState struct {
	SessionID           string       `gorm:"column:session_id;primaryKey;size:64" json:"session_id"`
	UserID              string       `gorm:"column:user_id;index;size:64;not null" json:"user_id"`
	ClientID            string       `gorm:"column:client_id;size:64;not null" json:"client_id"`
	GatewayID           string       `gorm:"column:gateway_id;index;size:32;not null" json:"gateway_id"`
	ServiceClass        ServiceClass `gorm:"column:service_class;size:16;not null;check:service_class IN ('Standard','Platinum','Diamond')" json:"service_class"`
	Priority            int          `gorm:"column:priority;default:50;check:priority >= 0 AND priority <= 100" json:"priority"`
	CurrentPersonaID    string       `gorm:"column:current_persona_id;size:64;default:''" json:"current_persona_id"`
	CurrentLinkID       string       `gorm:"column:current_link_id;index;size:64" json:"current_link_id"`
	CurrentSurvivalMode SurvivalMode `gorm:"column:current_survival_mode;size:16;default:'Normal'" json:"current_survival_mode"`
	BillingMode         string       `gorm:"column:billing_mode;size:32;default:''" json:"billing_mode"`
	State               SessionPhase `gorm:"column:state;size:16;not null;check:state IN ('Bootstrapping','Active','Protected','Migrating','Degraded','Suspended','Closed')" json:"state"`
	MigrationPending    bool         `gorm:"column:migration_pending;default:false" json:"migration_pending"`
	CreatedAt           time.Time    `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time    `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (SessionState) TableName() string { return "session_states" }

// ControlState 控制状态
type ControlState struct {
	GatewayID           string        `gorm:"column:gateway_id;primaryKey;size:32" json:"gateway_id"`
	Epoch               uint64        `gorm:"column:epoch;not null;default:0" json:"epoch"`
	PersonaVersion      uint64        `gorm:"column:persona_version;default:0" json:"persona_version"`
	RouteGeneration     uint64        `gorm:"column:route_generation;default:0" json:"route_generation"`
	ActiveTxID          string        `gorm:"column:active_tx_id;size:64;default:''" json:"active_tx_id"`
	RollbackMarker      uint64        `gorm:"column:rollback_marker;default:0" json:"rollback_marker"`
	LastSuccessfulEpoch uint64        `gorm:"column:last_successful_epoch;default:0" json:"last_successful_epoch"`
	LastSwitchReason    string        `gorm:"column:last_switch_reason;size:256;default:''" json:"last_switch_reason"`
	ControlHealth       ControlHealth `gorm:"column:control_health;size:16;default:'Healthy';check:control_health IN ('Healthy','Recovering','Faulted')" json:"control_health"`
	UpdatedAt           time.Time     `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ControlState) TableName() string { return "control_states" }
