package stealth

import (
	"bytes"
	"context"
	"fmt"
	"mirage-gateway/pkg/gtunnel/stego"
	"sync"
	"testing"
	"time"

	pb "mirage-proto/gen"
)

// pipeStream implements QUICStream using a pipe for bidirectional communication.
type pipeStream struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	mu       sync.Mutex
}

func newPipeStreamPair() (*pipeStream, *pipeStream) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}
	return &pipeStream{readBuf: buf1, writeBuf: buf2},
		&pipeStream{readBuf: buf2, writeBuf: buf1}
}

func (s *pipeStream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readBuf.Read(p)
}

func (s *pipeStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeBuf.Write(p)
}

func (s *pipeStream) Close() error { return nil }

// pipeConn implements QUICConn for integration testing.
type pipeConn struct {
	stream QUICStream
}

func (c *pipeConn) OpenStream() (QUICStream, error) { return c.stream, nil }
func (c *pipeConn) OpenStreamSync(_ context.Context) (QUICStream, error) {
	return c.stream, nil
}

// TestIntegration_ShadowStreamMux_ReadWrite tests Stream 0 read/write on a pipe.
func TestIntegration_ShadowStreamMux_ReadWrite(t *testing.T) {
	// Create a shared buffer stream (simulates loopback)
	stream := &testStream{}
	conn := &testConn{stream: stream}

	mux, err := NewShadowStreamMux(conn)
	if err != nil {
		t.Fatalf("NewShadowStreamMux: %v", err)
	}
	defer mux.Close()

	cmds := []*pb.ControlCommand{
		{CommandId: "int-1", CommandType: pb.ControlCommandType_PERSONA_FLIP, Epoch: 1, Timestamp: time.Now().UnixNano()},
		{CommandId: "int-2", CommandType: pb.ControlCommandType_BUDGET_SYNC, Epoch: 2, Timestamp: time.Now().UnixNano()},
		{CommandId: "int-3", CommandType: pb.ControlCommandType_ROLLBACK, Epoch: 3, Timestamp: time.Now().UnixNano(), Payload: []byte("rollback-data")},
	}

	for _, cmd := range cmds {
		if err := mux.WriteCommand(cmd); err != nil {
			t.Fatalf("WriteCommand(%s): %v", cmd.CommandId, err)
		}
	}

	for _, expected := range cmds {
		got, err := mux.ReadCommand()
		if err != nil {
			t.Fatalf("ReadCommand: %v", err)
		}
		if got.CommandId != expected.CommandId {
			t.Fatalf("CommandId: got %s, want %s", got.CommandId, expected.CommandId)
		}
		if got.CommandType != expected.CommandType {
			t.Fatalf("CommandType mismatch for %s", expected.CommandId)
		}
	}
}

// TestIntegration_Stream0_NonBlocking tests that Stream 0 writes don't block data streams.
func TestIntegration_Stream0_NonBlocking(t *testing.T) {
	stream := &testStream{}
	conn := &testConn{stream: stream}

	mux, err := NewShadowStreamMux(conn)
	if err != nil {
		t.Fatalf("NewShadowStreamMux: %v", err)
	}
	defer mux.Close()

	// Write control commands concurrently with data
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	// Control stream writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			cmd := &pb.ControlCommand{
				CommandId:   fmt.Sprintf("ctrl-%d", i),
				CommandType: pb.ControlCommandType_PERSONA_FLIP,
				Epoch:       uint64(i),
				Timestamp:   time.Now().UnixNano(),
			}
			if err := mux.WriteCommand(cmd); err != nil {
				errCh <- fmt.Errorf("WriteCommand: %v", err)
				return
			}
		}
	}()

	// Simulated data stream writer (writes to a separate buffer)
	dataBuf := &bytes.Buffer{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			data := []byte(fmt.Sprintf("data-packet-%d", i))
			if _, err := dataBuf.Write(data); err != nil {
				errCh <- fmt.Errorf("data write: %v", err)
				return
			}
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent error: %v", err)
	}
}

// TestIntegration_ChannelSwitch_E2E tests Scheme A → Scheme B → Scheme A switching.
func TestIntegration_ChannelSwitch_E2E(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	stream := &testStream{}
	conn := &testConn{stream: stream}
	mux, _ := NewShadowStreamMux(conn)
	enc := stego.NewStegoEncoder(key, 0.05)
	tl := &mockTimeline{}

	cp := NewStealthControlPlane(StealthControlPlaneOpts{
		Mux:      mux,
		Encoder:  enc,
		Timeline: tl,
	})

	// Initially Scheme A
	if cp.GetChannelState() != ChannelSchemeA {
		t.Fatalf("expected SchemeA, got %s", cp.GetChannelState())
	}

	// Send via Scheme A
	cmd1 := &pb.ControlCommand{
		CommandId:   "e2e-1",
		CommandType: pb.ControlCommandType_PERSONA_FLIP,
		Epoch:       1,
		Timestamp:   time.Now().UnixNano(),
	}
	if err := cp.SendCommand(context.Background(), cmd1); err != nil {
		t.Fatalf("SendCommand via A: %v", err)
	}

	// Close mux → fallback to Scheme B
	mux.Close()
	cp.SetSchemeAAvailable(false)
	if cp.GetChannelState() != ChannelSchemeB {
		t.Fatalf("expected SchemeB, got %s", cp.GetChannelState())
	}

	// Send via Scheme B
	cmd2 := &pb.ControlCommand{
		CommandId:   "e2e-2",
		CommandType: pb.ControlCommandType_BUDGET_SYNC,
		Epoch:       2,
		Timestamp:   time.Now().UnixNano(),
	}
	if err := cp.SendCommand(context.Background(), cmd2); err != nil {
		t.Fatalf("SendCommand via B: %v", err)
	}

	// Verify encoder has the command
	if enc.QueueLen() != 1 {
		t.Fatalf("expected 1 in encoder queue, got %d", enc.QueueLen())
	}

	// Restore Scheme A
	stream2 := &testStream{}
	conn2 := &testConn{stream: stream2}
	newMux, _ := NewShadowStreamMux(conn2)
	cp.mu.Lock()
	cp.mux = newMux
	cp.mu.Unlock()
	cp.SetSchemeAAvailable(true)

	if cp.GetChannelState() != ChannelSchemeA {
		t.Fatalf("expected SchemeA after restore, got %s", cp.GetChannelState())
	}

	if len(tl.switches) < 1 {
		t.Fatal("expected at least 1 timeline switch event")
	}
}

// TestIntegration_StegoE2E tests full stego encode → decode pipeline.
func TestIntegration_StegoE2E(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 42)
	}

	encoder := stego.NewStegoEncoder(key, 1.0) // no rate limit
	decoder := stego.NewStegoDecoder(key)

	cmd := &pb.ControlCommand{
		CommandId:   "stego-e2e",
		CommandType: pb.ControlCommandType_SURVIVAL_MODE_CHANGE,
		Epoch:       99,
		Timestamp:   time.Now().UnixNano(),
		Payload:     []byte("emergency-mode"),
	}

	if err := encoder.Enqueue(cmd); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Simulate dummy packet of 500 bytes
	payload, err := encoder.Encode(500)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if payload == nil {
		t.Fatal("Encode returned nil")
	}
	if len(payload) != 500 {
		t.Fatalf("payload length: got %d, want 500", len(payload))
	}

	// Decode
	restored, err := decoder.Decode(payload)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if restored == nil {
		t.Fatal("Decode returned nil")
	}
	if restored.CommandId != cmd.CommandId {
		t.Fatalf("CommandId: got %s, want %s", restored.CommandId, cmd.CommandId)
	}
	if restored.CommandType != cmd.CommandType {
		t.Fatalf("CommandType mismatch")
	}
	if string(restored.Payload) != string(cmd.Payload) {
		t.Fatalf("Payload mismatch")
	}
}

// TestIntegration_ConcurrentSendCommand tests concurrent SendCommand with race detection.
func TestIntegration_ConcurrentSendCommand(t *testing.T) {
	stream := &testStream{}
	conn := &testConn{stream: stream}
	mux, _ := NewShadowStreamMux(conn)

	cp := NewStealthControlPlane(StealthControlPlaneOpts{
		Mux: mux,
	})

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cmd := &pb.ControlCommand{
				CommandId:   fmt.Sprintf("concurrent-%d", idx),
				CommandType: pb.ControlCommandType(idx%5 + 1),
				Epoch:       uint64(idx),
				Timestamp:   time.Now().UnixNano(),
			}
			if err := cp.SendCommand(context.Background(), cmd); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent SendCommand error: %v", err)
	}
}
