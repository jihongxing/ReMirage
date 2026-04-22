package stego

import (
	"testing"

	"pgregory.net/rapid"
)

// TestProperty5_SilentDiscard verifies that random byte arrays (not produced by
// BuildStegoPayload) are silently discarded by StegoDecoder.Decode.
// **Validates: Requirements 3.9**
func TestProperty5_SilentDiscard(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := rapid.SliceOfN(rapid.Byte(), KeySize, KeySize).Draw(t, "key")
		randomData := rapid.SliceOfN(rapid.Byte(), 0, 2000).Draw(t, "random_data")

		decoder := NewStegoDecoder(key)
		cmd, err := decoder.Decode(randomData)
		if err != nil {
			t.Fatalf("expected nil error for random data, got: %v", err)
		}
		if cmd != nil {
			t.Fatalf("expected nil command for random data, got command")
		}
	})
}
