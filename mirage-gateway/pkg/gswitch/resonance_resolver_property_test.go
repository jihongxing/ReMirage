package gswitch

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 18: 信令 UA 随机性与合法性
// **Validates: Requirements 21.1, 21.2, 21.3**

func TestProperty_SignalingUARandomnessAndLegitimacy(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		const N = 100
		uaSet := make(map[string]bool)

		for i := 0; i < N; i++ {
			ua := randomUA()

			// Property: no non-standard identifiers
			if strings.Contains(ua, "HealthCheck") {
				t.Fatalf("UA contains 'HealthCheck': %s", ua)
			}
			if strings.Contains(ua, "compatible; HealthCheck") {
				t.Fatalf("UA contains old hardcoded pattern: %s", ua)
			}

			// Property: must look like a real browser UA (starts with Mozilla/5.0)
			if !strings.HasPrefix(ua, "Mozilla/5.0") {
				t.Fatalf("UA does not start with 'Mozilla/5.0': %s", ua)
			}

			uaSet[ua] = true
		}

		// Property: at least 3 different UA strings in 100 calls
		if len(uaSet) < 3 {
			t.Fatalf("only %d distinct UAs in %d calls, expected >= 3", len(uaSet), N)
		}
	})
}

func TestBrowserUAPoolSize(t *testing.T) {
	if len(browserUAPool) < 10 {
		t.Fatalf("browserUAPool has %d entries, expected >= 10", len(browserUAPool))
	}
}
