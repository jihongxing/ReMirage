package threat

import (
	"context"
	"log"
	"sync"
	"time"

	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/redact"
	"mirage-gateway/pkg/strategy"
)

// LevelParams 威胁等级对应的防御参数
type LevelParams struct {
	JitterMeanUs   uint32
	JitterStddevUs uint32
	NoiseIntensity uint32
	PaddingRate    uint32
}

// levelParamsMap 等级→参数映射（单调递增）
var levelParamsMap = map[ThreatLevel]*LevelParams{
	LevelLow:      {JitterMeanUs: 10000, JitterStddevUs: 3000, NoiseIntensity: 5, PaddingRate: 10},
	LevelMedium:   {JitterMeanUs: 30000, JitterStddevUs: 10000, NoiseIntensity: 15, PaddingRate: 20},
	LevelHigh:     {JitterMeanUs: 50000, JitterStddevUs: 15000, NoiseIntensity: 20, PaddingRate: 25},
	LevelCritical: {JitterMeanUs: 80000, JitterStddevUs: 25000, NoiseIntensity: 25, PaddingRate: 30},
	LevelExtreme:  {JitterMeanUs: 100000, JitterStddevUs: 30000, NoiseIntensity: 30, PaddingRate: 35},
}

// GetLevelParams 获取等级参数（导出供测试使用）
func GetLevelParams(level ThreatLevel) *LevelParams {
	if p, ok := levelParamsMap[level]; ok {
		return p
	}
	return levelParamsMap[LevelLow]
}

// Responder 威胁响应器
type Responder struct {
	currentLevel  ThreatLevel
	engine        *strategy.StrategyEngine
	loader        *ebpf.Loader
	blacklist     *BlacklistManager
	policy        *IngressPolicy
	ingressLogger *IngressLogger
	grpcNotify    func(level ThreatLevel)
	cooldownUntil time.Time
	fsm           *SecurityFSM
	mu            sync.Mutex
}

// NewResponder 创建响应器
func NewResponder(engine *strategy.StrategyEngine, loader *ebpf.Loader, blacklist *BlacklistManager) *Responder {
	return &Responder{
		currentLevel: LevelLow,
		engine:       engine,
		loader:       loader,
		blacklist:    blacklist,
	}
}

// Start 启动响应循环
func (r *Responder) Start(ctx context.Context, events <-chan *UnifiedThreatEvent) {
	go r.respondLoop(ctx, events)
	log.Println("[Responder] 威胁响应器已启动")
}

// SetGRPCNotify 设置 gRPC 上报回调
func (r *Responder) SetGRPCNotify(fn func(level ThreatLevel)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grpcNotify = fn
}

// SetPolicy 设置入口处置策略
func (r *Responder) SetPolicy(policy *IngressPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = policy
}

// SetIngressLogger 设置入口日志记录器
func (r *Responder) SetIngressLogger(logger *IngressLogger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ingressLogger = logger
}

// SetFSM 设置安全状态机
func (r *Responder) SetFSM(fsm *SecurityFSM) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fsm = fsm
}

// GetFSM 获取安全状态机
func (r *Responder) GetFSM() *SecurityFSM {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fsm
}

// EvaluateIngress 使用策略引擎评估入口流量并记录日志
func (r *Responder) EvaluateIngress(ctx *IngressContext) IngressAction {
	r.mu.Lock()
	policy := r.policy
	logger := r.ingressLogger
	r.mu.Unlock()

	if policy == nil {
		return ActionPass
	}

	action, condition := policy.EvaluateWithRule(ctx)

	// 6.5: 为每次处置动作记录结构化安全日志
	if action != ActionPass {
		LogPolicyAction(logger, ctx, action, condition)
		// 9.3: 递增入口拒绝指标
		IngressRejectTotal.WithLabelValues(GetGatewayID(), action.String()).Inc()
	}

	return action
}

// GetCurrentLevel 获取当前威胁等级
func (r *Responder) GetCurrentLevel() ThreatLevel {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentLevel
}

// respondLoop 响应循环
func (r *Responder) respondLoop(ctx context.Context, events <-chan *UnifiedThreatEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			r.handleEvent(event)
		}
	}
}

// handleEvent 处理事件
func (r *Responder) handleEvent(event *UnifiedThreatEvent) {
	newLevel := r.severityToLevel(event.Severity)

	r.mu.Lock()
	defer r.mu.Unlock()

	// 使用策略引擎评估处置动作
	if r.policy != nil {
		ctx := &IngressContext{
			SourceIP:    event.SourceIP,
			ThreatLevel: newLevel,
		}
		action, condition := r.policy.EvaluateWithRule(ctx)
		if action != ActionPass {
			LogPolicyAction(r.ingressLogger, ctx, action, condition)
			// 9.3: 递增入口拒绝指标
			IngressRejectTotal.WithLabelValues(GetGatewayID(), action.String()).Inc()
		}

		// 策略驱动的自动封禁：Drop 动作触发黑名单添加
		if action == ActionDrop && r.blacklist != nil && event.SourceIP != "" && event.SourceIP != "0.0.0.0" {
			ttl := time.Hour
			if newLevel >= LevelCritical {
				ttl = 24 * time.Hour
			}
			if err := r.blacklist.Add(event.SourceIP+"/32", time.Now().Add(ttl), SourceLocal); err != nil {
				log.Printf("[Responder] 自动封禁失败: %s: %v", redact.RedactIP(event.SourceIP), err)
			} else {
				log.Printf("[Responder] 策略封禁: %s (action=%s, condition=%s)", redact.RedactIP(event.SourceIP), action, condition)
			}
		}
	} else {
		// 回退：无策略时使用原有硬编码逻辑
		if newLevel >= LevelHigh && r.blacklist != nil && event.SourceIP != "" && event.SourceIP != "0.0.0.0" {
			ttl := time.Hour
			if newLevel >= LevelCritical {
				ttl = 24 * time.Hour
			}
			if err := r.blacklist.Add(event.SourceIP+"/32", time.Now().Add(ttl), SourceLocal); err != nil {
				log.Printf("[Responder] 自动封禁失败: %s: %v", redact.RedactIP(event.SourceIP), err)
			} else {
				log.Printf("[Responder] 自动封禁: %s (TTL=%v, level=%d)", redact.RedactIP(event.SourceIP), ttl, newLevel)
			}
		}
	}

	// 升级：立即执行
	if newLevel > r.currentLevel {
		r.applyLevel(newLevel)
		return
	}

	// 降级：检查冷却期（120 秒）
	if newLevel < r.currentLevel {
		if time.Now().Before(r.cooldownUntil) {
			return // 冷却期内不降级
		}
		r.applyLevel(newLevel)
	}
}

// applyLevel 应用新等级
func (r *Responder) applyLevel(level ThreatLevel) {
	old := r.currentLevel
	r.currentLevel = level

	// 设置降级冷却期
	r.cooldownUntil = time.Now().Add(120 * time.Second)

	// 调用策略引擎
	if r.engine != nil {
		r.engine.UpdateByThreat(uint8(level), uint32(level)*2)
	}

	// 写入 eBPF Map
	if r.loader != nil {
		params := GetLevelParams(level)
		strat := &ebpf.DefenseStrategy{
			JitterMeanUs:   params.JitterMeanUs,
			JitterStddevUs: params.JitterStddevUs,
			NoiseIntensity: params.NoiseIntensity,
		}
		if err := r.loader.UpdateStrategy(strat); err != nil {
			log.Printf("[Responder] eBPF Map 写入失败: %v", err)
		}
	}

	log.Printf("[Responder] 威胁等级变化: %d → %d", old, level)

	// 8.4: 威胁等级变化后驱动安全状态机迁移
	if r.fsm != nil {
		r.fsm.Evaluate(&SecurityMetrics{
			ThreatLevel: level,
		})
	}

	// 严重/极限等级通知 gRPC
	if level >= LevelCritical && r.grpcNotify != nil {
		go r.grpcNotify(level)
	}
}

// severityToLevel 严重程度映射到威胁等级
func (r *Responder) severityToLevel(severity int) ThreatLevel {
	switch {
	case severity >= 9:
		return LevelExtreme
	case severity >= 7:
		return LevelCritical
	case severity >= 5:
		return LevelHigh
	case severity >= 3:
		return LevelMedium
	default:
		return LevelLow
	}
}
