package stealth

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pb "mirage-proto/gen"

	"mirage-gateway/pkg/gtunnel/stego"

	"pgregory.net/rapid"
)

type mockDispatcher struct {
	mu     sync.Mutex
	events []interface{}
}

func (d *mockDispatcher) Dispatch(_ context.Context, event interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, event)
	return nil
}

type mockTimeline struct {
	switches []string
}

func (t *mockTimeline) OnChannelSwitch(from, to ChannelState, reason string) {
	t.switches = append(t.switches, fmt.Sprintf("%s->%s:%s", from, to, reason))
}

func makeTestKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

// TestProperty11_ChannelSelection verifies channel selection invariants.
// **Validates: Requirements 8.1, 8.2, 8.3, 8.5**
func TestProperty11_ChannelSelection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hasMux := rapid.Bool().Draw(t, "hasMux")
		hasEncoder := rapid.Bool().Draw(t, "hasEncoder")

		var mux *ShadowStreamMux
		var enc *stego.StegoEncoder

		if hasMux {
			stream := &testStream{}
			conn := &testConn{stream: stream}
			var err error
			mux, err = NewShadowStreamMux(conn)
			if err != nil {
				t.Fatalf("NewShadowStreamMux: %v", err)
			}
		}
		if hasEncoder {
			enc = stego.NewStegoEncoder(makeTestKey(), 0.05)
		}

		cp := NewStealthControlPlane(StealthControlPlaneOpts{
			Mux:     mux,
			Encoder: enc,
		})

		state := cp.GetChannelState()

		switch {
		case hasMux:
			if state != ChannelSchemeA {
				t.Fatalf("expected SchemeA when mux available, got %s", state)
			}
		case hasEncoder:
			if state != ChannelSchemeB {
				t.Fatalf("expected SchemeB when only encoder available, got %s", state)
			}
		default:
			if state != ChannelQueued {
				t.Fatalf("expected Queued when nothing available, got %s", state)
			}
		}
	})
}

// TestProperty11_QueueCapacity verifies queue capacity is 64.
func TestProperty11_QueueCapacity(t *testing.T) {
	cp := NewStealthControlPlane(StealthControlPlaneOpts{})

	for i := 0; i < 64; i++ {
		cmd := &pb.ControlCommand{
			CommandId:   fmt.Sprintf("cmd-%d", i),
			CommandType: pb.ControlCommandType_PERSONA_FLIP,
			Epoch:       1,
			Timestamp:   time.Now().UnixNano(),
		}
		cp.SendCommand(context.Background(), cmd)
	}

	if cp.QueueLen() != 64 {
		t.Fatalf("expected queue len 64, got %d", cp.QueueLen())
	}

	// 65th command should succeed (drops oldest)
	cmd65 := &pb.ControlCommand{
		CommandId:   "cmd-65",
		CommandType: pb.ControlCommandType_PERSONA_FLIP,
		Epoch:       1,
		Timestamp:   time.Now().UnixNano(),
	}
	err := cp.SendCommand(context.Background(), cmd65)
	if err != nil {
		t.Fatalf("65th SendCommand failed: %v", err)
	}
}

// TestProperty13_DedupIdempotency verifies that duplicate command_ids are silently discarded.
// **Validates: Requirements 9.5**
func TestProperty13_DedupIdempotency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := NewStealthControlPlane(StealthControlPlaneOpts{})

		cmdID := rapid.StringMatching(`[a-f0-9]{8}`).Draw(t, "cmdID")
		cmd := &pb.ControlCommand{
			CommandId:   cmdID,
			CommandType: pb.ControlCommandType(rapid.IntRange(1, 5).Draw(t, "type")),
			Epoch:       1,
			Timestamp:   time.Now().UnixNano(),
		}

		// First send
		err := cp.SendCommand(context.Background(), cmd)
		if err != nil {
			t.Fatalf("first SendCommand: %v", err)
		}

		queueBefore := cp.QueueLen()

		// Second send with same ID
		err = cp.SendCommand(context.Background(), cmd)
		if err != nil {
			t.Fatalf("second SendCommand: %v", err)
		}

		queueAfter := cp.QueueLen()
		if queueAfter != queueBefore {
			t.Fatalf("queue grew after duplicate: before=%d, after=%d", queueBefore, queueAfter)
		}
	})
}

// TestControlPlane_ChannelSwitch verifies TimelineCollector is called on switch.
func TestControlPlane_ChannelSwitch(t *testing.T) {
	stream := &testStream{}
	conn := &testConn{stream: stream}
	mux, _ := NewShadowStreamMux(conn)
	tl := &mockTimeline{}

	cp := NewStealthControlPlane(StealthControlPlaneOpts{
		Mux:      mux,
		Encoder:  stego.NewStegoEncoder(makeTestKey(), 0.05),
		Timeline: tl,
	})

	if cp.GetChannelState() != ChannelSchemeA {
		t.Fatal("expected SchemeA initially")
	}

	// Close mux to trigger fallback
	mux.Close()
	cp.SetSchemeAAvailable(false)

	if cp.GetChannelState() != ChannelSchemeB {
		t.Fatalf("expected SchemeB after mux close, got %s", cp.GetChannelState())
	}

	if len(tl.switches) == 0 {
		t.Fatal("expected timeline switch event")
	}
}

// TestControlPlane_QueueDropOldest verifies oldest command is dropped when queue is full.
func TestControlPlane_QueueDropOldest(t *testing.T) {
	cp := NewStealthControlPlane(StealthControlPlaneOpts{})

	// Fill queue
	for i := 0; i < 64; i++ {
		cp.SendCommand(context.Background(), &pb.ControlCommand{
			CommandId:   fmt.Sprintf("old-%d", i),
			CommandType: pb.ControlCommandType_PERSONA_FLIP,
			Epoch:       1,
			Timestamp:   time.Now().UnixNano(),
		})
	}

	// Add one more
	cp.SendCommand(context.Background(), &pb.ControlCommand{
		CommandId:   "new-cmd",
		CommandType: pb.ControlCommandType_PERSONA_FLIP,
		Epoch:       1,
		Timestamp:   time.Now().UnixNano(),
	})

	cmds := cp.DrainQueue()
	found := false
	for _, c := range cmds {
		if c.CommandId == "new-cmd" {
			found = true
		}
	}
	if !found {
		t.Fatal("new command not found in queue after overflow")
	}
}
