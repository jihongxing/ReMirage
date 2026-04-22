package survival

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	orchestrator "mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/transport"

	"pgregory.net/rapid"
)

var snakeCaseRegex = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func checkSnakeCaseKeys(t *rapid.T, data []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return // 非对象类型跳过
	}
	for key := range raw {
		if !snakeCaseRegex.MatchString(key) {
			t.Fatalf("JSON key %q is not snake_case", key)
		}
	}
}

// Property 13: JSON round-trip for ModePolicy
func TestProperty13_ModePolicyJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mode := genSurvivalMode(t)
		original := DefaultModePolicies[mode]

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		checkSnakeCaseKeys(t, data)

		var decoded ModePolicy
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if decoded.TransportPolicy != original.TransportPolicy {
			t.Fatalf("transport_policy mismatch")
		}
		if decoded.PersonaPolicy != original.PersonaPolicy {
			t.Fatalf("persona_policy mismatch")
		}
		if decoded.BudgetPolicy != original.BudgetPolicy {
			t.Fatalf("budget_policy mismatch")
		}
		if decoded.SwitchAggressiveness != original.SwitchAggressiveness {
			t.Fatalf("switch_aggressiveness mismatch")
		}
		if decoded.SessionAdmissionPolicy != original.SessionAdmissionPolicy {
			t.Fatalf("session_admission_policy mismatch")
		}
	})
}

// Property 13: JSON round-trip for TransportPolicy
func TestProperty13_TransportPolicyJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mode := genSurvivalMode(t)
		original := transport.DefaultTransportPolicies[mode]

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		checkSnakeCaseKeys(t, data)

		var decoded transport.TransportPolicy
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if decoded.SwitchThreshold != original.SwitchThreshold {
			t.Fatalf("switch_threshold mismatch")
		}
		if decoded.MaxParallelPaths != original.MaxParallelPaths {
			t.Fatalf("max_parallel_paths mismatch")
		}
		if decoded.PrewarmBackup != original.PrewarmBackup {
			t.Fatalf("prewarm_backup mismatch")
		}
	})
}

// Property 13: JSON round-trip for TransitionConstraintConfig
func TestProperty13_ConstraintConfigJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		config := TransitionConstraintConfig{
			MinDwellTimes: map[orchestrator.SurvivalMode]time.Duration{
				orchestrator.SurvivalModeNormal:   0,
				orchestrator.SurvivalModeHardened: time.Duration(rapid.IntRange(0, 300).Draw(t, "dwell")) * time.Second,
			},
			UpgradeCooldown:  time.Duration(rapid.IntRange(0, 120).Draw(t, "cooldown")) * time.Second,
			HysteresisMargin: rapid.Float64Range(0, 1).Draw(t, "margin"),
		}

		data, err := json.Marshal(config)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		checkSnakeCaseKeys(t, data)

		var decoded TransitionConstraintConfig
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if decoded.UpgradeCooldown != config.UpgradeCooldown {
			t.Fatalf("upgrade_cooldown mismatch: %v vs %v", decoded.UpgradeCooldown, config.UpgradeCooldown)
		}
		if decoded.HysteresisMargin != config.HysteresisMargin {
			t.Fatalf("hysteresis_margin mismatch")
		}
	})
}
