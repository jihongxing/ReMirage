package gtclient

import (
	"context"
	"fmt"
	"log"
	"phantom-client/pkg/token"
	"time"
)

// RecoveryPhase 恢复阶段
type RecoveryPhase int

const (
	PhaseJitter   RecoveryPhase = iota // 主链路抖动（< 5s）
	PhasePressure                      // 节点受压（5s-30s）
	PhaseDeath                         // 节点死亡（> 30s）
)

// String 返回阶段名称
func (p RecoveryPhase) String() string {
	switch p {
	case PhaseJitter:
		return "Jitter"
	case PhasePressure:
		return "Pressure"
	case PhaseDeath:
		return "Death"
	default:
		return "Unknown"
	}
}

// RecoveryResult 恢复结果
type RecoveryResult struct {
	Phase    RecoveryPhase
	Duration time.Duration
	Attempts int
	Success  bool
}

// RecoveryFSM 恢复状态机
type RecoveryFSM struct {
	phaseTimeout time.Duration // 每阶段超时（默认 15s）
	totalTimeout time.Duration // 总恢复超时（默认 60s）
}

// NewRecoveryFSM 创建恢复状态机
func NewRecoveryFSM() *RecoveryFSM {
	return &RecoveryFSM{
		phaseTimeout: 15 * time.Second,
		totalTimeout: 60 * time.Second,
	}
}

// Evaluate 根据断连时长判定恢复阶段
func (r *RecoveryFSM) Evaluate(disconnectDuration time.Duration) RecoveryPhase {
	switch {
	case disconnectDuration < 5*time.Second:
		return PhaseJitter
	case disconnectDuration < 30*time.Second:
		return PhasePressure
	default:
		return PhaseDeath
	}
}

// Execute 执行恢复流程
func (r *RecoveryFSM) Execute(ctx context.Context, phase RecoveryPhase, client *GTunnelClient) (*RecoveryResult, error) {
	start := time.Now()
	totalCtx, totalCancel := context.WithTimeout(ctx, r.totalTimeout)
	defer totalCancel()

	result := &RecoveryResult{Phase: phase}

	// 按阶段递进执行
	phases := []RecoveryPhase{PhaseJitter, PhasePressure, PhaseDeath}
	startIdx := 0
	for i, p := range phases {
		if p == phase {
			startIdx = i
			break
		}
	}

	for i := startIdx; i < len(phases); i++ {
		currentPhase := phases[i]
		phaseCtx, phaseCancel := context.WithTimeout(totalCtx, r.phaseTimeout)

		var err error
		switch currentPhase {
		case PhaseJitter:
			err = r.executeJitter(phaseCtx, client, result)
		case PhasePressure:
			err = r.executePressure(phaseCtx, client, result)
		case PhaseDeath:
			err = r.executeDeath(phaseCtx, client, result)
		}

		phaseCancel()

		if err == nil {
			result.Success = true
			result.Duration = time.Since(start)
			result.Phase = currentPhase
			log.Printf("[RecoveryFSM] ✅ 恢复成功: phase=%s, duration=%v, attempts=%d",
				currentPhase, result.Duration, result.Attempts)
			return result, nil
		}

		log.Printf("[RecoveryFSM] ⚠️ %s 阶段失败，升级到下一阶段", currentPhase)
	}

	result.Duration = time.Since(start)
	return result, fmt.Errorf("all recovery phases exhausted")
}

// executeJitter 主链路抖动恢复：在当前连接上重试 3 次，间隔 1s
func (r *RecoveryFSM) executeJitter(ctx context.Context, client *GTunnelClient, result *RecoveryResult) error {
	client.mu.RLock()
	currentGW := client.currentGW
	client.mu.RUnlock()

	for i := 0; i < 3; i++ {
		result.Attempts++
		engine, err := client.probe(ctx, currentGW)
		if err == nil {
			res := &probeResult{gw: currentGW, engine: engine}
			if err := client.switchWithTransaction(res, currentGW.IP); err == nil {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return fmt.Errorf("jitter retry exhausted")
}

// executePressure 节点受压恢复：触发拓扑刷新 + 同 Cell 切换
func (r *RecoveryFSM) executePressure(ctx context.Context, client *GTunnelClient, result *RecoveryResult) error {
	result.Attempts++

	// 触发拓扑刷新
	client.triggerImmediateTopoPull()

	// 等待一小段时间让拓扑刷新完成
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}

	client.mu.RLock()
	oldIP := client.currentGW.IP
	client.mu.RUnlock()

	// 尝试从 RuntimeTopology 切换到同 Cell 的其他节点
	if !client.runtimeTopo.IsEmpty() {
		node, err := client.runtimeTopo.NextByPriority(oldIP)
		if err == nil {
			gw := token.GatewayEndpoint{IP: node.IP, Port: node.Port, Region: node.Region}
			engine, probeErr := client.probe(ctx, gw)
			if probeErr == nil {
				res := &probeResult{gw: gw, engine: engine}
				if err := client.switchWithTransaction(res, oldIP); err == nil {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("pressure recovery failed")
}

// executeDeath 节点死亡恢复：执行现有 L1→L2→L3 降级
func (r *RecoveryFSM) executeDeath(ctx context.Context, client *GTunnelClient, result *RecoveryResult) error {
	result.Attempts++
	return client.doReconnect(ctx)
}
