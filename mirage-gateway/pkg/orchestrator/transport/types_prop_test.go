package transport

import (
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

var allSurvivalModes = []orchestrator.SurvivalMode{
	orchestrator.SurvivalModeNormal,
	orchestrator.SurvivalModeLowNoise,
	orchestrator.SurvivalModeHardened,
	orchestrator.SurvivalModeDegraded,
	orchestrator.SurvivalModeEscape,
	orchestrator.SurvivalModeLastResort,
}

// Property 10: Transport Policy 分级正确性
func TestProperty10_TransportPolicyGrading(t *testing.T) {
	type expected struct {
		threshold   float64
		maxParallel int
		prewarm     bool
	}
	spec := map[orchestrator.SurvivalMode]expected{
		orchestrator.SurvivalModeNormal:     {40.0, 1, false},
		orchestrator.SurvivalModeLowNoise:   {40.0, 1, false},
		orchestrator.SurvivalModeHardened:   {60.0, 2, true},
		orchestrator.SurvivalModeDegraded:   {40.0, 1, false},
		orchestrator.SurvivalModeEscape:     {80.0, 3, true},
		orchestrator.SurvivalModeLastResort: {80.0, 2, false},
	}

	rapid.Check(t, func(t *rapid.T) {
		mode := allSurvivalModes[rapid.IntRange(0, len(allSurvivalModes)-1).Draw(t, "mode")]
		policy := DefaultTransportPolicies[mode]

		if policy == nil {
			t.Fatalf("no transport policy for mode %s", mode)
		}

		exp := spec[mode]
		if policy.SwitchThreshold != exp.threshold {
			t.Fatalf("mode %s: expected threshold %.1f, got %.1f", mode, exp.threshold, policy.SwitchThreshold)
		}
		if policy.MaxParallelPaths != exp.maxParallel {
			t.Fatalf("mode %s: expected max_parallel %d, got %d", mode, exp.maxParallel, policy.MaxParallelPaths)
		}
		if policy.PrewarmBackup != exp.prewarm {
			t.Fatalf("mode %s: expected prewarm %v, got %v", mode, exp.prewarm, policy.PrewarmBackup)
		}
	})
}
