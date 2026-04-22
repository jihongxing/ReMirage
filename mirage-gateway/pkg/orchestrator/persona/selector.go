// Package persona - PersonaSelector 三重约束选择器
package persona

import (
	"context"
	"fmt"
	"mirage-gateway/pkg/orchestrator"
)

// SelectionConstraints 选择约束
type SelectionConstraints struct {
	ServiceClass orchestrator.ServiceClass
	LinkHealth   float64 // 0-100
	SurvivalMode orchestrator.SurvivalMode
}

// PersonaCandidate 候选 Persona（带元数据）
type PersonaCandidate struct {
	Manifest        *PersonaManifest
	ServiceClasses  []orchestrator.ServiceClass // 兼容的服务等级
	DefenseStrength int                         // 防御强度 0-100
	ResourceCost    int                         // 资源消耗 0-100
}

// PersonaSelector 三重约束选择器
type PersonaSelector struct {
	candidates []*PersonaCandidate
}

// NewPersonaSelector 创建选择器
func NewPersonaSelector(candidates []*PersonaCandidate) *PersonaSelector {
	return &PersonaSelector{candidates: candidates}
}

// Select 按三重约束选择 Persona
func (s *PersonaSelector) Select(_ context.Context, constraints *SelectionConstraints) (*PersonaManifest, error) {
	var filtered []*PersonaCandidate

	// 1. 按 ServiceClass 过滤
	for _, c := range s.candidates {
		if c.Manifest.Lifecycle != LifecycleActive && c.Manifest.Lifecycle != LifecyclePrepared {
			continue
		}
		if matchServiceClass(c.ServiceClasses, constraints.ServiceClass) {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		return nil, &ErrNoMatchingPersona{
			Constraints: fmt.Sprintf("ServiceClass=%s", constraints.ServiceClass),
		}
	}

	// 2. 按 LinkHealth 过滤资源消耗
	if constraints.LinkHealth < 50 {
		var lowCost []*PersonaCandidate
		for _, c := range filtered {
			if c.ResourceCost <= 50 {
				lowCost = append(lowCost, c)
			}
		}
		if len(lowCost) > 0 {
			filtered = lowCost
		}
	}

	// 3. 按 SurvivalMode 排序防御强度
	if isHighDefenseMode(constraints.SurvivalMode) {
		// 优先高防御强度
		best := filtered[0]
		for _, c := range filtered[1:] {
			if c.DefenseStrength > best.DefenseStrength {
				best = c
			}
		}
		return best.Manifest, nil
	}

	// 默认返回第一个匹配
	return filtered[0].Manifest, nil
}

func matchServiceClass(classes []orchestrator.ServiceClass, target orchestrator.ServiceClass) bool {
	for _, c := range classes {
		if c == target {
			return true
		}
	}
	return false
}

func isHighDefenseMode(mode orchestrator.SurvivalMode) bool {
	return mode == orchestrator.SurvivalModeHardened ||
		mode == orchestrator.SurvivalModeEscape ||
		mode == orchestrator.SurvivalModeLastResort
}
