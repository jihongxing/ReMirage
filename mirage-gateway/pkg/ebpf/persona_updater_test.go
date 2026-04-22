package ebpf

import (
	"testing"

	"pgregory.net/rapid"
)

// ============================================
// Property 7: Shadow Slot 写入 round-trip
// ============================================

func TestProperty7_ShadowSlotWriteRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mock := NewMockPersonaMapUpdater()

		params := &PersonaParams{
			DNA: DNATemplateEntry{
				TargetIATMu:     rapid.Uint32().Draw(t, "iat_mu"),
				TargetIATSigma:  rapid.Uint32().Draw(t, "iat_sigma"),
				PaddingStrategy: rapid.Uint32Range(0, 2).Draw(t, "padding"),
				TargetMTU:       rapid.Uint16().Draw(t, "mtu"),
				BurstSize:       rapid.Uint32().Draw(t, "burst_size"),
				BurstInterval:   rapid.Uint32().Draw(t, "burst_interval"),
			},
			Jitter: JitterConfig{
				Enabled:     rapid.Uint32Range(0, 1).Draw(t, "j_enabled"),
				MeanIATUs:   rapid.Uint32().Draw(t, "j_mean"),
				StddevIATUs: rapid.Uint32().Draw(t, "j_stddev"),
				TemplateID:  rapid.Uint32().Draw(t, "j_template"),
			},
			VPC: VPCConfig{
				Enabled:        rapid.Uint32Range(0, 1).Draw(t, "v_enabled"),
				FiberJitterUs:  rapid.Uint32().Draw(t, "v_fiber"),
				RouterDelayUs:  rapid.Uint32().Draw(t, "v_router"),
				NoiseIntensity: rapid.Uint32Range(0, 100).Draw(t, "v_noise"),
			},
			NPM: NewDefaultNPMConfig(rapid.Uint32Range(0, 100).Draw(t, "npm_rate")),
		}

		slotID, err := mock.WriteShadow(params)
		if err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 7: WriteShadow failed: %v", err)
		}

		err = mock.VerifyShadow(slotID, params)
		if err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 7: VerifyShadow failed: %v", err)
		}
	})
}
