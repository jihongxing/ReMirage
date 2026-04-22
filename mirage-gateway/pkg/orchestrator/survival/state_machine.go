package survival

import orchestrator "mirage-gateway/pkg/orchestrator"

// ValidateTransition 检查 (from, to) 是否为合法迁移
// 自迁移（from == to）被拒绝，非法迁移返回 ErrInvalidTransition
func ValidateTransition(from, to orchestrator.SurvivalMode) error {
	if from == to {
		return &ErrInvalidTransition{From: from, To: to}
	}
	targets, ok := ValidTransitions[from]
	if !ok {
		return &ErrInvalidTransition{From: from, To: to}
	}
	for _, t := range targets {
		if t == to {
			return nil
		}
	}
	return &ErrInvalidTransition{From: from, To: to}
}
