package stego

import (
	"errors"
	"sync"
	"sync/atomic"

	pb "mirage-proto/gen"
)

// StegoEncoder is the steganography encoder for Scheme B.
type StegoEncoder struct {
	sessionKey []byte
	queue      chan *pb.ControlCommand
	maxRate    float64
	totalDummy atomic.Uint64
	stegoDummy atomic.Uint64
	mu         sync.Mutex
}

// NewStegoEncoder creates a new stego encoder.
// maxRate is the maximum stego replacement rate (e.g. 0.05 for 5%).
func NewStegoEncoder(sessionKey []byte, maxRate float64) *StegoEncoder {
	if maxRate <= 0 {
		maxRate = 0.05
	}
	return &StegoEncoder{
		sessionKey: sessionKey,
		queue:      make(chan *pb.ControlCommand, 64),
		maxRate:    maxRate,
	}
}

// Enqueue adds a control command to the pending send queue.
func (e *StegoEncoder) Enqueue(cmd *pb.ControlCommand) error {
	if cmd == nil {
		return errors.New("nil command")
	}
	select {
	case e.queue <- cmd:
		return nil
	default:
		return errors.New("stego encoder queue full")
	}
}

// Encode attempts to encode the head-of-queue command into a stego payload.
// dummyLen is the length of the dummy packet about to be sent.
// Returns nil if no command is pending, length is insufficient, or rate limit exceeded.
func (e *StegoEncoder) Encode(dummyLen int) ([]byte, error) {
	e.totalDummy.Add(1)

	// Check rate limit
	total := e.totalDummy.Load()
	stego := e.stegoDummy.Load()
	if total > 0 && float64(stego+1)/float64(total) > e.maxRate {
		return nil, nil
	}

	// Peek at queue
	select {
	case cmd := <-e.queue:
		payload, err := BuildStegoPayload(e.sessionKey, cmd, dummyLen)
		if err != nil {
			// Length insufficient or other error — re-queue the command
			select {
			case e.queue <- cmd:
			default:
				// queue full, drop oldest and re-add
			}
			return nil, nil
		}
		e.stegoDummy.Add(1)
		return payload, nil
	default:
		return nil, nil
	}
}

// GetRate returns the current stego replacement rate.
func (e *StegoEncoder) GetRate() float64 {
	total := e.totalDummy.Load()
	if total == 0 {
		return 0
	}
	return float64(e.stegoDummy.Load()) / float64(total)
}

// QueueLen returns the current queue length (for testing).
func (e *StegoEncoder) QueueLen() int {
	return len(e.queue)
}
