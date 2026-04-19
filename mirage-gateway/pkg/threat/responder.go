package threat

import (
	"context"
	"log"
	"sync"
	"time"

	"mirage-gateway/pkg/ebpf"
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
	grpcNotify    func(level ThreatLevel)
	cooldownUntil time.Time
	mu            sync.Mutex
}

// NewResponder 创建响应器
func NewResponder(engine *strategy.StrategyEngine, loader *ebpf.Loader) *Responder {
	return &Responder{
		currentLevel: LevelLow,
		engine:       engine,
		loader:       loader,
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
