// Package commit - V2 State Commit Engine 类型定义
package commit

// TxType 事务类型枚举
type TxType string

const (
	TxTypePersonaSwitch       TxType = "PersonaSwitch"
	TxTypeLinkMigration       TxType = "LinkMigration"
	TxTypeGatewayReassignment TxType = "GatewayReassignment"
	TxTypeSurvivalModeSwitch  TxType = "SurvivalModeSwitch"
)

// AllTxTypes 所有合法 TxType 值
var AllTxTypes = []TxType{
	TxTypePersonaSwitch, TxTypeLinkMigration,
	TxTypeGatewayReassignment, TxTypeSurvivalModeSwitch,
}

// TxPhase 事务阶段枚举
type TxPhase string

const (
	TxPhasePreparing     TxPhase = "Preparing"
	TxPhaseValidating    TxPhase = "Validating"
	TxPhaseShadowWriting TxPhase = "ShadowWriting"
	TxPhaseFlipping      TxPhase = "Flipping"
	TxPhaseAcknowledging TxPhase = "Acknowledging"
	TxPhaseCommitted     TxPhase = "Committed"
	TxPhaseRolledBack    TxPhase = "RolledBack"
	TxPhaseFailed        TxPhase = "Failed"
)

// AllTxPhases 所有合法 TxPhase 值
var AllTxPhases = []TxPhase{
	TxPhasePreparing, TxPhaseValidating, TxPhaseShadowWriting,
	TxPhaseFlipping, TxPhaseAcknowledging, TxPhaseCommitted,
	TxPhaseRolledBack, TxPhaseFailed,
}

// TxScope 事务作用域枚举
type TxScope string

const (
	TxScopeSession TxScope = "Session"
	TxScopeLink    TxScope = "Link"
	TxScopeGlobal  TxScope = "Global"
)

// TxTypeScopeMap 事务类型到作用域的映射
var TxTypeScopeMap = map[TxType]TxScope{
	TxTypePersonaSwitch:       TxScopeSession,
	TxTypeLinkMigration:       TxScopeLink,
	TxTypeGatewayReassignment: TxScopeSession,
	TxTypeSurvivalModeSwitch:  TxScopeGlobal,
}

// TxScopePriority 作用域优先级（数值越大优先级越高）
var TxScopePriority = map[TxScope]int{
	TxScopeSession: 1,
	TxScopeLink:    2,
	TxScopeGlobal:  3,
}

// TerminalPhases 终态集合
var TerminalPhases = map[TxPhase]bool{
	TxPhaseCommitted:  true,
	TxPhaseRolledBack: true,
	TxPhaseFailed:     true,
}

// IsTerminal 判断阶段是否为终态
func IsTerminal(phase TxPhase) bool {
	return TerminalPhases[phase]
}
