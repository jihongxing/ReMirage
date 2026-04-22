// Package commit - TX_Phase 状态机
package commit

import "time"

// ValidTransitions 合法的阶段转换路径
var ValidTransitions = map[TxPhase][]TxPhase{
	TxPhasePreparing:     {TxPhaseValidating, TxPhaseFailed},
	TxPhaseValidating:    {TxPhaseShadowWriting, TxPhaseFailed},
	TxPhaseShadowWriting: {TxPhaseFlipping, TxPhaseRolledBack},
	TxPhaseFlipping:      {TxPhaseAcknowledging, TxPhaseRolledBack},
	TxPhaseAcknowledging: {TxPhaseCommitted, TxPhaseRolledBack},
}

// TransitionPhase 执行阶段转换，返回转换时间戳
func TransitionPhase(current, target TxPhase) (time.Time, error) {
	if IsTerminal(current) {
		return time.Time{}, &ErrTerminalPhase{Phase: current}
	}
	targets, ok := ValidTransitions[current]
	if !ok {
		return time.Time{}, &ErrInvalidPhaseTransition{From: current, To: target}
	}
	for _, t := range targets {
		if t == target {
			return time.Now().UTC(), nil
		}
	}
	return time.Time{}, &ErrInvalidPhaseTransition{From: current, To: target}
}
