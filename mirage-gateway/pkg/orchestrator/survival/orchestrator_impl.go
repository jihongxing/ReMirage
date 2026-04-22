package survival

import (
	"context"
	"fmt"
	"sync"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
	"mirage-gateway/pkg/orchestrator/transport"
)

// PersonaEngineIface Persona Engine 接口（简化）
type PersonaEngineIface interface {
	ApplyPolicy(policyName string) error
}

type survivalOrchestrator struct {
	mu sync.RWMutex

	currentMode   orchestrator.SurvivalMode
	currentPolicy *ModePolicy
	enteredAt     time.Time
	lastUpgradeAt time.Time

	evaluator      TriggerEvaluatorIface
	constraint     TransitionConstraintIface
	admission      SessionAdmissionControllerIface
	fabric         transport.TransportFabricIface
	commitEngine   commit.CommitEngine
	persona        PersonaEngineIface
	gswitchAdapter interface {
		TriggerEscape(reason string) error
	}

	history []*TransitionRecord
}

func (o *survivalOrchestrator) GetCurrentMode() orchestrator.SurvivalMode {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.currentMode
}

func (o *survivalOrchestrator) GetCurrentPolicy() *ModePolicy {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.currentPolicy
}

func (o *survivalOrchestrator) RequestTransition(ctx context.Context, target orchestrator.SurvivalMode, triggers []TriggerSignal) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	current := o.currentMode

	// 1. 验证迁移合法性
	if err := ValidateTransition(current, target); err != nil {
		return err
	}

	// 2. 检查约束
	if err := o.constraint.Check(current, target, o.enteredAt, o.lastUpgradeAt, triggers); err != nil {
		return err
	}

	// 3. CommitEngine 事务
	tx, err := o.commitEngine.BeginTransaction(ctx, &commit.BeginTxRequest{
		TxType:             commit.TxTypeSurvivalModeSwitch,
		TargetSurvivalMode: string(target),
	})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := o.commitEngine.ExecuteTransaction(ctx, tx.TxID); err != nil {
		// 回滚：保持当前模式
		return fmt.Errorf("execute transaction: %w", err)
	}

	// 4. 更新内部状态
	newPolicy := DefaultModePolicies[target]
	o.currentMode = target
	o.currentPolicy = newPolicy
	now := time.Now()
	o.enteredAt = now

	if ModeSeverity[target] > ModeSeverity[current] {
		o.lastUpgradeAt = now
	}

	// 5. 下发策略
	if o.fabric != nil {
		tp := transport.DefaultTransportPolicies[target]
		if tp != nil {
			o.fabric.ApplyPolicy(tp)
		}
	}
	if o.persona != nil {
		_ = o.persona.ApplyPolicy(string(newPolicy.PersonaPolicy))
	}
	if o.admission != nil {
		o.admission.UpdatePolicy(newPolicy.SessionAdmissionPolicy)
	}

	// 6. Escape/LastResort 自动触发 G-Switch
	if (target == orchestrator.SurvivalModeEscape || target == orchestrator.SurvivalModeLastResort) && o.gswitchAdapter != nil {
		_ = o.gswitchAdapter.TriggerEscape(fmt.Sprintf("survival mode → %s", target))
	}

	// 7. 记录历史
	o.history = append(o.history, &TransitionRecord{
		FromMode:  current,
		ToMode:    target,
		Triggers:  triggers,
		TxID:      tx.TxID,
		Timestamp: now,
	})

	return nil
}

func (o *survivalOrchestrator) EvaluateAndTransition(ctx context.Context) error {
	advice, err := o.evaluator.Evaluate(ctx, o.GetCurrentMode())
	if err != nil {
		return err
	}
	if advice == nil {
		return nil
	}
	return o.RequestTransition(ctx, advice.TargetMode, advice.Triggers)
}

func (o *survivalOrchestrator) CheckAdmission(serviceClass orchestrator.ServiceClass) error {
	return o.admission.Check(serviceClass)
}

func (o *survivalOrchestrator) GetTransitionHistory(n int) []*TransitionRecord {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if n <= 0 || n > len(o.history) {
		n = len(o.history)
	}
	start := len(o.history) - n
	result := make([]*TransitionRecord, n)
	copy(result, o.history[start:])
	return result
}

func (o *survivalOrchestrator) RecoverOnStartup(ctx context.Context) error {
	if err := o.commitEngine.RecoverOnStartup(ctx); err != nil {
		// 回退到 Normal
		o.mu.Lock()
		o.currentMode = orchestrator.SurvivalModeNormal
		o.currentPolicy = DefaultModePolicies[orchestrator.SurvivalModeNormal]
		o.enteredAt = time.Now()
		o.mu.Unlock()
		return err
	}
	return nil
}
