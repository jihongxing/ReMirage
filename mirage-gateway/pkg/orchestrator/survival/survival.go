package survival

import (
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
	"mirage-gateway/pkg/orchestrator/transport"
)

// SurvivalOrchestratorDeps 依赖注入
type SurvivalOrchestratorDeps struct {
	Evaluator      TriggerEvaluatorIface
	Constraint     TransitionConstraintIface
	Admission      SessionAdmissionControllerIface
	Fabric         transport.TransportFabricIface
	CommitEngine   commit.CommitEngine
	Persona        PersonaEngineIface
	GSwitchAdapter interface {
		TriggerEscape(reason string) error
	}
}

// NewSurvivalOrchestrator 创建 SurvivalOrchestrator
func NewSurvivalOrchestrator(deps SurvivalOrchestratorDeps) SurvivalOrchestratorIface {
	return &survivalOrchestrator{
		currentMode:    orchestrator.SurvivalModeNormal,
		currentPolicy:  DefaultModePolicies[orchestrator.SurvivalModeNormal],
		enteredAt:      time.Now(),
		lastUpgradeAt:  time.Time{},
		evaluator:      deps.Evaluator,
		constraint:     deps.Constraint,
		admission:      deps.Admission,
		fabric:         deps.Fabric,
		commitEngine:   deps.CommitEngine,
		persona:        deps.Persona,
		gswitchAdapter: deps.GSwitchAdapter,
		history:        make([]*TransitionRecord, 0),
	}
}
