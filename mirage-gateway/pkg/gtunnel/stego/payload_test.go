package stego

import (
	"testing"

	pb "mirage-proto/gen"

	"pgregory.net/rapid"
)

func genControlCommand() *rapid.Generator[*pb.ControlCommand] {
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

// TestProperty3_StegoPayloadLengthInvariant verifies that BuildStegoPayload output
// length is strictly equal to targetLen.
// **Validates: Requirements 3.3, 4.1**
func TestProperty3_StegoPayloadLengthInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := rapid.SliceOfN(rapid.Byte(), KeySize, KeySize).Draw(t, "key")
		cmd := genControlCommand().Draw(t, "cmd")

		// Compute minimum target length for this command
		serialized, err := pb.MarshalControlCommand(cmd)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		minTarget := MinStegoPayloadOverhead + len(serialized)
		extra := rapid.IntRange(0, 512).Draw(t, "extra")
		targetLen := minTarget + extra

		payload, err := BuildStegoPayload(key, cmd, targetLen)
		if err != nil {
			t.Fatalf("BuildStegoPayload failed: %v", err)
		}

		if len(payload) != targetLen {
			t.Fatalf("length mismatch: got %d, want %d", len(payload), targetLen)
		}
	})
}

// TestProperty4_StegoRoundTrip verifies that BuildStegoPayload followed by
// ParseStegoPayload produces an equivalent ControlCommand.
// **Validates: Requirements 3.2, 3.4, 3.5, 3.7, 3.8**
func TestProperty4_StegoRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := rapid.SliceOfN(rapid.Byte(), KeySize, KeySize).Draw(t, "key")
		cmd := genControlCommand().Draw(t, "cmd")

		serialized, err := pb.MarshalControlCommand(cmd)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		minTarget := MinStegoPayloadOverhead + len(serialized)
		extra := rapid.IntRange(0, 256).Draw(t, "extra")
		targetLen := minTarget + extra

		payload, err := BuildStegoPayload(key, cmd, targetLen)
		if err != nil {
			t.Fatalf("BuildStegoPayload failed: %v", err)
		}

		restored, err := ParseStegoPayload(key, payload)
		if err != nil {
			t.Fatalf("ParseStegoPayload failed: %v", err)
		}
		if restored == nil {
			t.Fatalf("ParseStegoPayload returned nil")
		}

		if cmd.CommandId != restored.CommandId {
			t.Fatalf("CommandId mismatch: %q vs %q", cmd.CommandId, restored.CommandId)
		}
		if cmd.CommandType != restored.CommandType {
			t.Fatalf("CommandType mismatch")
		}
		if cmd.Epoch != restored.Epoch {
			t.Fatalf("Epoch mismatch")
		}
		if cmd.Timestamp != restored.Timestamp {
			t.Fatalf("Timestamp mismatch")
		}
		if len(cmd.Payload) == 0 && len(restored.Payload) == 0 {
			// ok
		} else if string(cmd.Payload) != string(restored.Payload) {
			t.Fatalf("Payload mismatch")
		}
	})
}
