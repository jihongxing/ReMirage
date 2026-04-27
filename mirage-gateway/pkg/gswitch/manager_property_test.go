package gswitch

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 16: 域名生成格式多样性
// **Validates: Requirements 19.1, 19.2**

func TestProperty_DomainGenerationDiversity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		gm := &GSwitchManager{}

		const N = 100
		tldSet := make(map[string]bool)

		for i := 0; i < N; i++ {
			domain := gm.generateTempDomain()

			// Extract TLD + parent domain pattern
			parts := strings.Split(domain.Name, ".")
			if len(parts) < 2 {
				t.Fatalf("invalid domain: %s", domain.Name)
			}
			// Use last 2 parts as pattern key (e.g. "example.com", "example.net")
			pattern := strings.Join(parts[len(parts)-2:], ".")
			tldSet[pattern] = true
		}

		// Property: at least 3 different format patterns
		if len(tldSet) < 3 {
			t.Fatalf("only %d distinct domain patterns in %d generations, expected >= 3", len(tldSet), N)
		}
	})
}
