package stealth

import (
	"fmt"
	"time"

	pb "mirage-proto/gen"

	"mirage-gateway/pkg/orchestrator/events"
)

// commandTypeToEventType maps ControlCommandType to EventType.
var commandTypeToEventType = map[pb.ControlCommandType]events.EventType{
	pb.ControlCommandType_PERSONA_FLIP:         events.EventPersonaFlip,
	pb.ControlCommandType_BUDGET_SYNC:          events.EventBudgetReject,
	pb.ControlCommandType_SURVIVAL_MODE_CHANGE: events.EventSurvivalModeChange,
	pb.ControlCommandType_ROLLBACK:             events.EventRollbackRequest,
	pb.ControlCommandType_SESSION_MIGRATE:      events.EventSessionMigrateRequest,
}

// eventTypeToCommandType maps EventType to ControlCommandType.
var eventTypeToCommandType = map[events.EventType]pb.ControlCommandType{
	events.EventPersonaFlip:           pb.ControlCommandType_PERSONA_FLIP,
	events.EventBudgetReject:          pb.ControlCommandType_BUDGET_SYNC,
	events.EventSurvivalModeChange:    pb.ControlCommandType_SURVIVAL_MODE_CHANGE,
	events.EventRollbackRequest:       pb.ControlCommandType_ROLLBACK,
	events.EventSessionMigrateRequest: pb.ControlCommandType_SESSION_MIGRATE,
}

// ToControlEvent converts a Protobuf ControlCommand to a ControlEvent.
func ToControlEvent(cmd *pb.ControlCommand) (*events.ControlEvent, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil ControlCommand")
	}
	et, ok := commandTypeToEventType[cmd.CommandType]
	if !ok {
		return nil, fmt.Errorf("unknown command_type: %v", cmd.CommandType)
	}

	sem := events.GetSemantics(et)
	scope := events.EventScopeSession
	priority := 5
	requiresAck := false
	if sem != nil {
		scope = sem.DefaultScope
		priority = sem.DefaultPriority
		requiresAck = sem.RequiresAck
	}

	return &events.ControlEvent{
		EventID:     cmd.CommandId,
		EventType:   et,
		Source:      "stealth-control-plane",
		TargetScope: scope,
		Priority:    priority,
		Epoch:       cmd.Epoch,
		PayloadRef:  string(cmd.Payload),
		RequiresAck: requiresAck,
		CreatedAt:   time.Unix(0, cmd.Timestamp),
	}, nil
}

// FromControlEvent converts a ControlEvent back to a Protobuf ControlCommand.
func FromControlEvent(event *events.ControlEvent) (*pb.ControlCommand, error) {
	if event == nil {
		return nil, fmt.Errorf("nil ControlEvent")
	}
	ct, ok := eventTypeToCommandType[event.EventType]
	if !ok {
		return nil, fmt.Errorf("unknown event_type: %v", event.EventType)
	}
	return &pb.ControlCommand{
		CommandId:   event.EventID,
		CommandType: ct,
		Epoch:       event.Epoch,
		Timestamp:   event.CreatedAt.UnixNano(),
		Payload:     []byte(event.PayloadRef),
	}, nil
}
