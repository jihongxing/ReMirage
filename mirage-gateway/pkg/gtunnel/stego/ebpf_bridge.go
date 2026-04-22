package stego

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"
)

// StegoReadyEvent mirrors the C struct stego_ready_event.
type StegoReadyEvent struct {
	Timestamp uint64
	DummyLen  uint32
	DummySeq  uint32
}

// StegoReadyEventSize is the size of StegoReadyEvent in bytes.
const StegoReadyEventSize = 16

// ParseStegoReadyEvent parses a raw ring buffer record into StegoReadyEvent.
func ParseStegoReadyEvent(data []byte) (*StegoReadyEvent, error) {
	if len(data) < StegoReadyEventSize {
		return nil, fmt.Errorf("data too short: %d < %d", len(data), StegoReadyEventSize)
	}
	return &StegoReadyEvent{
		Timestamp: binary.LittleEndian.Uint64(data[0:8]),
		DummyLen:  binary.LittleEndian.Uint32(data[8:12]),
		DummySeq:  binary.LittleEndian.Uint32(data[12:16]),
	}, nil
}

// StegoCommand mirrors the C struct stego_command.
type StegoCommand struct {
	Valid      uint32
	PayloadLen uint32
	Payload    [1400]byte
}

// StegoCommandSize is the size of StegoCommand in bytes.
const StegoCommandSize = int(unsafe.Sizeof(StegoCommand{}))

// MarshalStegoCommand serializes a StegoCommand for eBPF map write.
func MarshalStegoCommand(cmd *StegoCommand) ([]byte, error) {
	if cmd == nil {
		return nil, errors.New("nil StegoCommand")
	}
	buf := make([]byte, StegoCommandSize)
	binary.LittleEndian.PutUint32(buf[0:4], cmd.Valid)
	binary.LittleEndian.PutUint32(buf[4:8], cmd.PayloadLen)
	copy(buf[8:], cmd.Payload[:])
	return buf, nil
}

// EBPFBridge provides Go-side bridge for stego eBPF maps.
// In production, this wraps cilium/ebpf map operations.
// The interface is defined here for testability.
type EBPFBridge struct {
	// RingBufReader reads from stego_ready_map ring buffer.
	// In production: ringbuf.NewReader(stegoReadyMap)
	ReadEvent func() (*StegoReadyEvent, error)

	// WriteCommand writes a StegoCommand to stego_command_map.
	// In production: stegoCommandMap.Put(uint32(0), cmd)
	WriteCommand func(cmd *StegoCommand) error
}

// NewEBPFBridge creates a new eBPF bridge with the provided callbacks.
func NewEBPFBridge(readEvent func() (*StegoReadyEvent, error), writeCommand func(cmd *StegoCommand) error) *EBPFBridge {
	return &EBPFBridge{
		ReadEvent:    readEvent,
		WriteCommand: writeCommand,
	}
}

// WriteStegoPayload writes a stego payload to the eBPF command map.
func (b *EBPFBridge) WriteStegoPayload(payload []byte) error {
	if len(payload) > 1400 {
		return fmt.Errorf("payload too large: %d > 1400", len(payload))
	}
	cmd := &StegoCommand{
		Valid:      1,
		PayloadLen: uint32(len(payload)),
	}
	copy(cmd.Payload[:], payload)
	return b.WriteCommand(cmd)
}

// ClearStegoCommand clears the stego command map entry.
func (b *EBPFBridge) ClearStegoCommand() error {
	cmd := &StegoCommand{Valid: 0}
	return b.WriteCommand(cmd)
}
