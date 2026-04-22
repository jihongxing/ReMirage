package stealth

import (
	"bytes"
	"context"
	"testing"

	pb "mirage-proto/gen"
)

// testStream implements QUICStream for testing.
type testStream struct {
	buf    bytes.Buffer
	closed bool
}

func (s *testStream) Read(p []byte) (int, error)  { return s.buf.Read(p) }
func (s *testStream) Write(p []byte) (int, error) { return s.buf.Write(p) }
func (s *testStream) Close() error                { s.closed = true; return nil }

// testConn implements QUICConn for testing.
type testConn struct {
	stream *testStream
}

func (c *testConn) OpenStream() (QUICStream, error) { return c.stream, nil }
func (c *testConn) OpenStreamSync(_ context.Context) (QUICStream, error) {
	return c.stream, nil
}

func TestShadowStreamMux_WriteReadCommand(t *testing.T) {
	stream := &testStream{}
	conn := &testConn{stream: stream}

	mux, err := NewShadowStreamMux(conn)
	if err != nil {
		t.Fatalf("NewShadowStreamMux: %v", err)
	}

	cmd := &pb.ControlCommand{
		CommandId:   "test-cmd-001",
		CommandType: pb.ControlCommandType_PERSONA_FLIP,
		Epoch:       42,
		Timestamp:   1234567890,
		Payload:     []byte("test-payload"),
	}

	if err := mux.WriteCommand(cmd); err != nil {
		t.Fatalf("WriteCommand: %v", err)
	}

	restored, err := mux.ReadCommand()
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}

	if cmd.CommandId != restored.CommandId {
		t.Fatalf("CommandId mismatch")
	}
	if cmd.CommandType != restored.CommandType {
		t.Fatalf("CommandType mismatch")
	}
	if cmd.Epoch != restored.Epoch {
		t.Fatalf("Epoch mismatch")
	}
}

func TestShadowStreamMux_IsAvailable(t *testing.T) {
	stream := &testStream{}
	conn := &testConn{stream: stream}

	mux, err := NewShadowStreamMux(conn)
	if err != nil {
		t.Fatalf("NewShadowStreamMux: %v", err)
	}

	if !mux.IsAvailable() {
		t.Fatal("expected available")
	}

	mux.Close()
	if mux.IsAvailable() {
		t.Fatal("expected not available after close")
	}
}

func TestShadowStreamMux_NilConn(t *testing.T) {
	_, err := NewShadowStreamMux(nil)
	if err == nil {
		t.Fatal("expected error for nil connection")
	}
}
