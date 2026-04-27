package stealth

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	pb "mirage-proto/gen"

	"google.golang.org/protobuf/proto"
)

// QUICStream abstracts a QUIC stream for testability.
type QUICStream interface {
	io.Reader
	io.Writer
	Close() error
}

// QUICConn abstracts a QUIC connection for testability.
type QUICConn interface {
	OpenStream() (QUICStream, error)
	OpenStreamSync(ctx context.Context) (QUICStream, error)
}

// ShadowStreamMux is Scheme A: QUIC covert stream multiplexer.
// Stream 0 is the privileged control stream.
type ShadowStreamMux struct {
	conn       QUICConn
	ctrlStream QUICStream
	mu         sync.Mutex
	closed     atomic.Bool
}

// NewShadowStreamMux creates a covert stream multiplexer on a QUIC connection.
// Opens Stream 0 as the privileged control stream.
func NewShadowStreamMux(conn QUICConn) (*ShadowStreamMux, error) {
	if conn == nil {
		return nil, errors.New("nil QUICConn")
	}
	stream, err := conn.OpenStream()
	if err != nil {
		return nil, err
	}
	return &ShadowStreamMux{
		conn:       conn,
		ctrlStream: stream,
	}, nil
}

// WriteCommand serializes a ControlCommand to Protobuf and writes to Stream 0.
// Format: [4-byte big-endian length][protobuf data]
func (m *ShadowStreamMux) WriteCommand(cmd *pb.ControlCommand) error {
	if m.closed.Load() {
		return errors.New("mux closed")
	}
	data, err := proto.Marshal(cmd)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := m.ctrlStream.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := m.ctrlStream.Write(data); err != nil {
		return err
	}
	return nil
}

// ReadCommand reads and deserializes a ControlCommand from Stream 0.
func (m *ShadowStreamMux) ReadCommand() (*pb.ControlCommand, error) {
	if m.closed.Load() {
		return nil, errors.New("mux closed")
	}

	var lenBuf [4]byte
	if _, err := io.ReadFull(m.ctrlStream, lenBuf[:]); err != nil {
		return nil, err
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	if msgLen > 65536 {
		return nil, errors.New("message too large")
	}

	data := make([]byte, msgLen)
	if _, err := io.ReadFull(m.ctrlStream, data); err != nil {
		return nil, err
	}

	cmd := &pb.ControlCommand{}
	if err := proto.Unmarshal(data, cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}

// IsAvailable checks if Stream 0 is available.
func (m *ShadowStreamMux) IsAvailable() bool {
	return !m.closed.Load() && m.ctrlStream != nil
}

// Close closes the control stream (does not close the QUIC connection).
func (m *ShadowStreamMux) Close() error {
	if m.closed.Swap(true) {
		return nil
	}
	if m.ctrlStream != nil {
		return m.ctrlStream.Close()
	}
	return nil
}
