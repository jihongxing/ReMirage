package stego

import (
	"testing"

	pb "mirage-proto/gen"

	"pgregory.net/rapid"
)

func genCmd() *rapid.Generator[*pb.ControlCommand] {
	return rapid.Custom(func(t *rapid.T) *pb.ControlCommand {
		return &pb.ControlCommand{
			CommandId:   rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}`).Draw(t, "command_id"),
			CommandType: pb.ControlCommandType(rapid.IntRange(1, 5).Draw(t, "command_type")),
			Epoch:       rapid.Uint64Range(1, 1<<62).Draw(t, "epoch"),
			Timestamp:   rapid.Int64Range(1, 1<<62).Draw(t, "timestamp"),
			Payload:     rapid.SliceOfN(rapid.Byte(), 0, 128).Draw(t, "payload"),
		}
	})
}

// TestProperty6_LengthReject verifies that when dummyLen is too small to hold
// HMAC_Tag + Ciphertext, Encode returns nil.
// **Validates: Requirements 4.2**
func TestProperty6_LengthReject(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := rapid.SliceOfN(rapid.Byte(), KeySize, KeySize).Draw(t, "key")
		cmd := genCmd().Draw(t, "cmd")

		enc := NewStegoEncoder(key, 1.0) // no rate limit for this test
		if err := enc.Enqueue(cmd); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}

		// Use a very small dummyLen that can't fit the payload
		dummyLen := rapid.IntRange(0, MinStegoPayloadOverhead-1).Draw(t, "dummyLen")
		result, err := enc.Encode(dummyLen)
		if err != nil {
			t.Fatalf("Encode error: %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil for dummyLen=%d, got %d bytes", dummyLen, len(result))
		}
	})
}

// TestProperty7_RateLimitInvariant verifies that GetRate() never exceeds maxRate.
// **Validates: Requirements 4.3**
func TestProperty7_RateLimitInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := rapid.SliceOfN(rapid.Byte(), KeySize, KeySize).Draw(t, "key")
		maxRate := rapid.Float64Range(0.01, 0.2).Draw(t, "maxRate")
		enc := NewStegoEncoder(key, maxRate)

		numOps := rapid.IntRange(20, 200).Draw(t, "numOps")
		for i := 0; i < numOps; i++ {
			// Enqueue a command for some iterations
			if rapid.Bool().Draw(t, "enqueue") {
				cmd := genCmd().Draw(t, "cmd")
				enc.Enqueue(cmd) // ignore error if queue full
			}
			// Large enough dummyLen to always fit
			enc.Encode(2000)

			rate := enc.GetRate()
			if rate > maxRate+0.01 { // small tolerance for integer rounding
				t.Fatalf("rate %f exceeds maxRate %f", rate, maxRate)
			}
		}
	})
}
