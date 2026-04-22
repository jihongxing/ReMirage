package events

// EventSemantics 事件语义属性
type EventSemantics struct {
	DefaultScope    EventScope `json:"default_scope"`
	DefaultPriority int        `json:"default_priority"`
	RequiresAck     bool       `json:"requires_ack"`
	Idempotent      bool       `json:"idempotent"`
	Replayable      bool       `json:"replayable"`
	CarriesEpoch    bool       `json:"carries_epoch"`
}

// EventSemanticsMap 事件类型到语义属性的映射
var EventSemanticsMap = map[EventType]*EventSemantics{
	EventSessionMigrateRequest: {
		DefaultScope: EventScopeSession, DefaultPriority: 5,
		RequiresAck: true, Idempotent: false, Replayable: false, CarriesEpoch: true,
	},
	EventSessionMigrateAck: {
		DefaultScope: EventScopeSession, DefaultPriority: 5,
		RequiresAck: false, Idempotent: true, Replayable: true, CarriesEpoch: true,
	},
	EventPersonaPrepare: {
		DefaultScope: EventScopeSession, DefaultPriority: 4,
		RequiresAck: true, Idempotent: true, Replayable: true, CarriesEpoch: true,
	},
	EventPersonaFlip: {
		DefaultScope: EventScopeSession, DefaultPriority: 7,
		RequiresAck: true, Idempotent: false, Replayable: false, CarriesEpoch: true,
	},
	EventSurvivalModeChange: {
		DefaultScope: EventScopeGlobal, DefaultPriority: 9,
		RequiresAck: true, Idempotent: false, Replayable: false, CarriesEpoch: true,
	},
	EventRollbackRequest: {
		DefaultScope: EventScopeSession, DefaultPriority: 8,
		RequiresAck: true, Idempotent: true, Replayable: true, CarriesEpoch: true,
	},
	EventRollbackDone: {
		DefaultScope: EventScopeSession, DefaultPriority: 8,
		RequiresAck: false, Idempotent: true, Replayable: true, CarriesEpoch: true,
	},
	EventBudgetReject: {
		DefaultScope: EventScopeSession, DefaultPriority: 6,
		RequiresAck: false, Idempotent: true, Replayable: true, CarriesEpoch: false,
	},
	// V2 Adapter 命令语义（legacy → V2 转换）
	EventTypeStrategyUpdate: {
		DefaultScope: EventScopeGlobal, DefaultPriority: 5,
		RequiresAck: false, Idempotent: true, Replayable: true, CarriesEpoch: false,
	},
	EventTypeQuotaUpdate: {
		DefaultScope: EventScopeSession, DefaultPriority: 5,
		RequiresAck: false, Idempotent: true, Replayable: true, CarriesEpoch: false,
	},
	EventTypeBlacklistUpdate: {
		DefaultScope: EventScopeGlobal, DefaultPriority: 7,
		RequiresAck: false, Idempotent: true, Replayable: true, CarriesEpoch: false,
	},
	EventTypeReincarnation: {
		DefaultScope: EventScopeGlobal, DefaultPriority: 9,
		RequiresAck: true, Idempotent: false, Replayable: false, CarriesEpoch: false,
	},
}

// GetSemantics 查询指定 EventType 的语义属性，未定义返回 nil
func GetSemantics(et EventType) *EventSemantics {
	return EventSemanticsMap[et]
}
