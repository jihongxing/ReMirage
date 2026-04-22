package events

// EventType 事件类型枚举
type EventType string

const (
	EventSessionMigrateRequest EventType = "session.migrate.request"
	EventSessionMigrateAck     EventType = "session.migrate.ack"
	EventPersonaPrepare        EventType = "persona.prepare"
	EventPersonaFlip           EventType = "persona.flip"
	EventSurvivalModeChange    EventType = "survival.mode.change"
	EventRollbackRequest       EventType = "rollback.request"
	EventRollbackDone          EventType = "rollback.done"
	EventBudgetReject          EventType = "budget.reject"
	// V2 Adapter 命令类型（legacy → V2 转换）
	EventTypeStrategyUpdate  EventType = "strategy.update"
	EventTypeQuotaUpdate     EventType = "quota.update"
	EventTypeBlacklistUpdate EventType = "blacklist.update"
	EventTypeReincarnation   EventType = "reincarnation.trigger"
)

// AllEventTypes 所有已定义的 EventType
var AllEventTypes = []EventType{
	EventSessionMigrateRequest,
	EventSessionMigrateAck,
	EventPersonaPrepare,
	EventPersonaFlip,
	EventSurvivalModeChange,
	EventRollbackRequest,
	EventRollbackDone,
	EventBudgetReject,
	EventTypeStrategyUpdate,
	EventTypeQuotaUpdate,
	EventTypeBlacklistUpdate,
	EventTypeReincarnation,
}

// EventScope 事件作用域枚举
type EventScope string

const (
	EventScopeSession EventScope = "Session"
	EventScopeLink    EventScope = "Link"
	EventScopeGlobal  EventScope = "Global"
)
