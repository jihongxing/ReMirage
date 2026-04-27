package gtclient

import (
	"crypto/rand"
	"math"
	"math/big"
	mathrand "math/rand"
	"sync"
	"time"
)

// IATMode defines the inter-arrival time distribution mode.
type IATMode int

const (
	// IATModeNormal uses normal (Gaussian) distribution for IAT sampling.
	IATModeNormal IATMode = iota
	// IATModeExponential uses exponential distribution for IAT sampling.
	IATModeExponential
)

// SendPathShimConfig holds configuration for the send-path shim layer.
type SendPathShimConfig struct {
	// PaddingMean is the target packet length mean (bytes).
	PaddingMean int
	// PaddingStddev is the target packet length standard deviation (bytes).
	PaddingStddev int
	// MaxMTU is the QUIC Datagram MTU upper bound (bytes).
	MaxMTU int
	// IATMode selects the IAT distribution (Normal or Exponential).
	IATMode IATMode
	// IATMeanUs is the IAT mean in microseconds.
	IATMeanUs int64
	// IATStddevUs is the IAT standard deviation in microseconds (used for Normal mode).
	IATStddevUs int64
}

// SendPathShim is the unified send-path shim layer that applies padding and
// IAT control to encrypted datagrams before the actual transport send.
// It sits at the boundary: "after encryption, before SendDatagram".
type SendPathShim struct {
	mu            sync.Mutex
	sendFn        func([]byte) error
	paddingMean   int
	paddingStddev int
	maxMTU        int
	iatMode       IATMode
	iatMeanUs     int64
	iatStddevUs   int64
	rng           *mathrand.Rand
}

// NewSendPathShim creates a SendPathShim with the given config and send function.
func NewSendPathShim(cfg SendPathShimConfig, sendFn func([]byte) error) *SendPathShim {
	// Seed from crypto/rand for unpredictability
	seed, _ := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	return &SendPathShim{
		sendFn:        sendFn,
		paddingMean:   cfg.PaddingMean,
		paddingStddev: cfg.PaddingStddev,
		maxMTU:        cfg.MaxMTU,
		iatMode:       cfg.IATMode,
		iatMeanUs:     cfg.IATMeanUs,
		iatStddevUs:   cfg.IATStddevUs,
		rng:           mathrand.New(mathrand.NewSource(seed.Int64())),
	}
}

// applyPadding appends random bytes to the encrypted datagram so that the
// total packet length follows N(paddingMean, paddingStddev²). If the result
// exceeds maxMTU, it is truncated to maxMTU.
func (s *SendPathShim) applyPadding(encrypted []byte) []byte {
	s.mu.Lock()
	targetLen := int(s.rng.NormFloat64()*float64(s.paddingStddev) + float64(s.paddingMean))
	s.mu.Unlock()

	currentLen := len(encrypted)

	// If target is not larger than current, no padding needed
	if targetLen <= currentLen {
		return encrypted
	}

	// Cap at MTU
	if s.maxMTU > 0 && targetLen > s.maxMTU {
		targetLen = s.maxMTU
	}

	// Still no room to pad after MTU cap
	if targetLen <= currentLen {
		return encrypted
	}

	padLen := targetLen - currentLen
	padding := make([]byte, padLen)
	rand.Read(padding) // crypto/rand for random padding bytes

	result := make([]byte, targetLen)
	copy(result, encrypted)
	copy(result[currentLen:], padding)
	return result
}

// sampleIATDelay returns the IAT delay to apply before sending.
// Normal mode: N(iatMeanUs, iatStddevUs²) using NormFloat64().
// Exponential mode: Exp(1/iatMeanUs) using ExpFloat64().
// Negative values are clamped to 0.
func (s *SendPathShim) sampleIATDelay() time.Duration {
	s.mu.Lock()
	var delayUs float64
	switch s.iatMode {
	case IATModeNormal:
		delayUs = s.rng.NormFloat64()*float64(s.iatStddevUs) + float64(s.iatMeanUs)
	case IATModeExponential:
		delayUs = s.rng.ExpFloat64() * float64(s.iatMeanUs)
	default:
		delayUs = 0
	}
	s.mu.Unlock()

	if delayUs < 0 {
		delayUs = 0
	}
	return time.Duration(delayUs) * time.Microsecond
}

// Send applies padding, then IAT delay, then calls the underlying sendFn.
func (s *SendPathShim) Send(encrypted []byte) error {
	padded := s.applyPadding(encrypted)
	delay := s.sampleIATDelay()
	if delay > 0 {
		time.Sleep(delay)
	}
	return s.sendFn(padded)
}
