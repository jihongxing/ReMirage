package stealth

import (
	"testing"

	pb "mirage-proto/gen"

	"pgregory.net/rapid"
)

// genControlCommandType generates a valid ControlCommandType (1-5).
func genControlCommandType() *rapid.Generator[pb.ControlCommandType] {
	return rapid.Custom(func(t *rapid.T) pb.ControlCommandType {
		return pb.ControlCommandType(rapid.IntRange(1, 5).Draw(t, "command_type"))
	})
}

// genControlCommand generates a valid ControlCommand with non-empty command_id,
// command_type in [1,5], non-zero epoch and timestamp.
func genControlCommand() *rapid.Generator[*pb.ControlCommand] {
	return rapid.Custom(func(t *rapid.T) *pb.ControlCommand {
		return &pb.ControlCommand{
			CommandId:   rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}`).Draw(t, "command_id"),
			CommandType: genControlCommandType().Draw(t, "command_type"),
			Epoch:       rapid.Uint64Range(1, 1<<62).Draw(t, "epoch"),
			Timestamp:   rapid.Int64Range(1, 1<<62).Draw(t, "timestamp"),
			Payload:     rapid.SliceOfN(rapid.Byte(), 0, 512).Draw(t, "payload"),
		}
	})
}

// TestProperty1_ProtobufRoundTrip verifies that for any valid ControlCommand,
// Marshal followed by Unmarshal produces an equivalent object.
// **Validates: Requirements 2.2, 2.3**
func TestProperty1_ProtobufRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cmd := genControlCommand().Draw(t, "cmd")

		data, err := pb.MarshalControlCommand(cmd)
		if err != nil {
			t.Fatalf("MarshalControlCommand failed: %v", err)
		}

		restored, err := pb.UnmarshalControlCommand(data)
		if err != nil {
			t.Fatalf("UnmarshalControlCommand failed: %v", err)
		}

		if cmd.CommandId != restored.CommandId {
			t.Fatalf("CommandId mismatch: %q vs %q", cmd.CommandId, restored.CommandId)
		}
		if cmd.CommandType != restored.CommandType {
			t.Fatalf("CommandType mismatch: %v vs %v", cmd.CommandType, restored.CommandType)
		}
		if cmd.Epoch != restored.Epoch {
			t.Fatalf("Epoch mismatch: %d vs %d", cmd.Epoch, restored.Epoch)
		}
		if cmd.Timestamp != restored.Timestamp {
			t.Fatalf("Timestamp mismatch: %d vs %d", cmd.Timestamp, restored.Timestamp)
		}
		if len(cmd.Payload) == 0 && len(restored.Payload) == 0 {
			// both empty, ok
		} else if string(cmd.Payload) != string(restored.Payload) {
			t.Fatalf("Payload mismatch")
		}
	})
}

// TestProperty2_ControlCommandEventRoundTrip verifies that ToControlEvent followed
// by FromControlEvent produces an equivalent ControlCommand.
// **Validates: Requirements 1.3, 2.2**
func TestProperty2_ControlCommandEventRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cmd := genControlCommand().Draw(t, "cmd")

		event, err := ToControlEvent(cmd)
		if err != nil {
			t.Fatalf("ToControlEvent failed: %v", err)
		}

		if event.EventID != cmd.CommandId {
			t.Fatalf("EventID mismatch: %q vs %q", event.EventID, cmd.CommandId)
		}

		restored, err := FromControlEvent(event)
		if err != nil {
			t.Fatalf("FromControlEvent failed: %v", err)
		}

		if cmd.CommandId != restored.CommandId {
			t.Fatalf("CommandId mismatch: %q vs %q", cmd.CommandId, restored.CommandId)
		}
		if cmd.CommandType != restored.CommandType {
			t.Fatalf("CommandType mismatch: %v vs %v", cmd.CommandType, restored.CommandType)
		}
		if cmd.Epoch != restored.Epoch {
			t.Fatalf("Epoch mismatch: %d vs %d", cmd.Epoch, restored.Epoch)
		}
	})
}
