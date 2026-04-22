package threat

import (
	"log"
	"sync"
	"time"
)

// SecurityState 安全状态（供 FSM 集成）
type SecurityState int

const (
	StateNormal       SecurityState = iota // 默认策略
	StateAlert                             // 收紧 Throttle
	StateHighPressure                      // 主动 Drop
	StateIsolated                          // 仅白名单
	StateSilent                            // 最小暴露
)

// IngressContext 入口处置上下文
type IngressContext struct {
	SourceIP        string
	BlacklistHit    bool
	ThreatLevel     ThreatLevel
	HoneypotHit     bool
	FingerprintRisk int
	ConnectionRate  float64
}

// IngressPolicy 入口处置策略引擎
type IngressPolicy struct {
	rules           []PolicyRule
	admissionScorer *AdmissionScorer
	mu              sync.RWMutex
}

// NewIngressPolicy 创建策略引擎
func NewIngressPolicy(rules []PolicyRule) *IngressPolicy {
	p := &IngressPolicy{
		rules: make([]PolicyRule, len(rules)),
	}
	copy(p.rules, rules)
	return p
}

// SetAdmissionScorer 注入多维准入评分器
func (p *IngressPolicy) SetAdmissionScorer(scorer *AdmissionScorer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.admissionScorer = scorer
}

// Evaluate 评估入口上下文，返回最高优先级匹配动作
func (p *IngressPolicy) Evaluate(ctx *IngressContext) IngressAction {
	p.mu.RLock()
	defer p.mu.RUnlock()

	bestAction := ActionPass
	bestPriority := -1

	for _, rule := range p.rules {
		if rule.matches(ctx) && rule.Priority > bestPriority {
			bestAction = rule.Action
			bestPriority = rule.Priority
		}
	}

	// 规则匹配结果为 Pass 时，额外检查 AdmissionScorer 的评分
	if bestAction == ActionPass && p.admissionScorer != nil && ctx.SourceIP != "" {
		admissionAction := p.admissionScorer.Evaluate(ctx.SourceIP)
		if admissionAction > bestAction {
			bestAction = admissionAction
		}
	}

	return bestAction
}

// EvaluateWithRule 评估并返回匹配的规则条件名（用于日志）
func (p *IngressPolicy) EvaluateWithRule(ctx *IngressContext) (IngressAction, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	bestAction := ActionPass
	bestPriority := -1
	bestCondition := ""

	for _, rule := range p.rules {
		if rule.matches(ctx) && rule.Priority > bestPriority {
			bestAction = rule.Action
			bestPriority = rule.Priority
			bestCondition = rule.Condition
		}
	}

	return bestAction, bestCondition
}

// UpdateRules 热更新策略规则
func (p *IngressPolicy) UpdateRules(rules []PolicyRule) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rules = make([]PolicyRule, len(rules))
	copy(p.rules, rules)
	log.Printf("[IngressPolicy] 策略规则已更新，共 %d 条", len(rules))
}

// ApplyStateOverride 根据安全状态覆盖策略
func (p *IngressPolicy) ApplyStateOverride(state SecurityState) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var overrides []PolicyRule

	switch state {
	case StateAlert:
		// 收紧 Throttle 阈值
		overrides = append(p.rules, PolicyRule{
			Condition: "rate_exceeded",
			Action:    ActionThrottle,
			Params:    map[string]int{"pps": 5},
			Priority:  95,
		})
	case StateHighPressure:
		// 主动 Drop 可疑流量
		overrides = append(p.rules, PolicyRule{
			Condition: "suspicious_fingerprint",
			Action:    ActionDrop,
			Priority:  95,
		}, PolicyRule{
			Condition: "rate_exceeded",
			Action:    ActionDrop,
			Priority:  95,
		})
	case StateIsolated:
		// 仅白名单放行，其余全部 Drop（通过极高优先级 rate_exceeded 规则）
		overrides = []PolicyRule{
			{Condition: "blacklist_hit", Action: ActionDrop, Priority: 200},
			{Condition: "threat_level_critical", Action: ActionDrop, Priority: 190},
			{Condition: "threat_level_high", Action: ActionDrop, Priority: 180},
			{Condition: "honeypot_hit", Action: ActionDrop, Priority: 170},
			{Condition: "suspicious_fingerprint", Action: ActionDrop, Priority: 160},
			{Condition: "rate_exceeded", Action: ActionDrop, Priority: 150},
		}
	case StateSilent:
		// 最小暴露：全部 Drop
		overrides = []PolicyRule{
			{Condition: "blacklist_hit", Action: ActionDrop, Priority: 200},
			{Condition: "threat_level_critical", Action: ActionDrop, Priority: 200},
			{Condition: "threat_level_high", Action: ActionDrop, Priority: 200},
			{Condition: "honeypot_hit", Action: ActionDrop, Priority: 200},
			{Condition: "suspicious_fingerprint", Action: ActionDrop, Priority: 200},
			{Condition: "rate_exceeded", Action: ActionDrop, Priority: 200},
		}
	default:
		// StateNormal: 不覆盖
		return
	}

	p.rules = overrides
	log.Printf("[IngressPolicy] 安全状态覆盖: state=%d, rules=%d", state, len(p.rules))
}

// GetRules 获取当前规则（只读副本）
func (p *IngressPolicy) GetRules() []PolicyRule {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]PolicyRule, len(p.rules))
	copy(result, p.rules)
	return result
}

// PolicyRule 策略规则
type PolicyRule struct {
	Condition string         `yaml:"condition"`
	Action    IngressAction  `yaml:"action"`
	Params    map[string]int `yaml:"params,omitempty"`
	Priority  int            `yaml:"priority"`
}

// matches 检查规则是否匹配上下文
func (r *PolicyRule) matches(ctx *IngressContext) bool {
	switch r.Condition {
	case "blacklist_hit":
		return ctx.BlacklistHit
	case "threat_level_critical":
		return ctx.ThreatLevel >= LevelCritical
	case "threat_level_high":
		return ctx.ThreatLevel >= LevelHigh
	case "honeypot_hit":
		return ctx.HoneypotHit
	case "suspicious_fingerprint":
		return ctx.FingerprintRisk >= 70
	case "rate_exceeded":
		pps, ok := r.Params["pps"]
		if !ok {
			pps = 50 // 默认阈值
		}
		return ctx.ConnectionRate > float64(pps)
	default:
		return false
	}
}

// LogPolicyAction 记录策略处置动作的结构化安全日志
func LogPolicyAction(logger *IngressLogger, ctx *IngressContext, action IngressAction, condition string) {
	if logger == nil {
		return
	}
	logger.Log(IngressLog{
		Timestamp:   time.Now(),
		SourceIP:    ctx.SourceIP,
		Action:      action,
		Reason:      condition,
		ThreatLevel: int(ctx.ThreatLevel),
	})
}
