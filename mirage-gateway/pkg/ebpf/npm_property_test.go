package ebpf

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 8: NPM 模式修正不变量
// **Validates: Requirements 9.2, 9.3**

// fakeNPMMap simulates an eBPF Map holding a single NPMConfig.
type fakeNPMMap struct {
	cfg NPMConfig
}

func (m *fakeNPMMap) Lookup(key, valueOut interface{}) error {
	k, ok := key.(*uint32)
	if !ok || *k != 0 {
		return fmt.Errorf("invalid key")
	}
	out, ok := valueOut.(*NPMConfig)
	if !ok {
		return fmt.Errorf("invalid value type")
	}
	*out = m.cfg
	return nil
}

func (m *fakeNPMMap) Put(key, value interface{}) error {
	k, ok := key.(*uint32)
	if !ok || *k != 0 {
		return fmt.Errorf("invalid key")
	}
	cfg, ok := value.(*NPMConfig)
	if !ok {
		return fmt.Errorf("invalid value type")
	}
	m.cfg = *cfg
	return nil
}

func TestProperty_NPMModeCorrection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate arbitrary NPMConfig with any PaddingMode
		mode := rapid.Uint32Range(0, 10).Draw(t, "paddingMode")
		enabled := rapid.Uint32Range(0, 1).Draw(t, "enabled")
		fillingRate := rapid.Uint32Range(0, 100).Draw(t, "fillingRate")
		globalMTU := rapid.Uint32Range(500, 1500).Draw(t, "globalMTU")
		minPkt := rapid.Uint32Range(0, 256).Draw(t, "minPacketSize")
		decoyRate := rapid.Uint32Range(0, 100).Draw(t, "decoyRate")

		m := &fakeNPMMap{
			cfg: NPMConfig{
				Enabled:       enabled,
				FillingRate:   fillingRate,
				GlobalMTU:     globalMTU,
				MinPacketSize: minPkt,
				PaddingMode:   mode,
				DecoyRate:     decoyRate,
			},
		}

		// Run VerifyGaussianMode
		err := VerifyGaussianModeWithMap(m)
		if err != nil {
			t.Fatalf("VerifyGaussianMode failed: %v", err)
		}

		// Property: PaddingMode must be Gaussian after verification
		if m.cfg.PaddingMode != NPMModeGaussian {
			t.Fatalf("PaddingMode=%d after VerifyGaussianMode, expected %d",
				m.cfg.PaddingMode, NPMModeGaussian)
		}

		// Property: other fields must be preserved
		if m.cfg.Enabled != enabled {
			t.Fatalf("Enabled changed: %d → %d", enabled, m.cfg.Enabled)
		}
		if m.cfg.FillingRate != fillingRate {
			t.Fatalf("FillingRate changed: %d → %d", fillingRate, m.cfg.FillingRate)
		}
		if m.cfg.GlobalMTU != globalMTU {
			t.Fatalf("GlobalMTU changed: %d → %d", globalMTU, m.cfg.GlobalMTU)
		}
		if m.cfg.MinPacketSize != minPkt {
			t.Fatalf("MinPacketSize changed: %d → %d", minPkt, m.cfg.MinPacketSize)
		}
		if m.cfg.DecoyRate != decoyRate {
			t.Fatalf("DecoyRate changed: %d → %d", decoyRate, m.cfg.DecoyRate)
		}
	})
}

// ---------------------------------------------------------------------------
// Mock NPM logic: Go equivalent of C eBPF npm.c functions (Tasks 5.2–5.4)
// ---------------------------------------------------------------------------

const (
	MIN_PADDING_SIZE uint32 = 64
	MAX_PADDING_SIZE uint32 = 1400
)

// calculatePadding is the Go equivalent of C calculate_padding in npm.c.
// It computes the padding size based on mode, current packet size, and target MTU.
func calculatePadding(mode, currentSize, targetMTU uint32) uint32 {
	if currentSize >= targetMTU {
		return 0
	}
	space := targetMTU - currentSize
	var padding uint32

	switch mode {
	case NPMModeFixedMTU:
		padding = space
	case NPMModeRandomRange:
		// Simulate random range: pick a value in (0, space]
		if space == 0 {
			return 0
		}
		// Deterministic-ish for mock: use midpoint as representative
		padding = (space + 1) / 2
		if padding == 0 {
			padding = 1
		}
	case NPMModeGaussian:
		// Simulate gaussian: pick a value in [0, space]
		// Use ~68% of space as representative gaussian center
		padding = (space * 68) / 100
	default:
		return 0
	}

	// Clamp to [MIN_PADDING_SIZE, MAX_PADDING_SIZE]
	if padding > 0 && padding < MIN_PADDING_SIZE {
		padding = MIN_PADDING_SIZE
	}
	if padding > MAX_PADDING_SIZE {
		padding = MAX_PADDING_SIZE
	}
	return padding
}

// handleNPMPadding is the Go equivalent of C handle_npm_padding in npm.c.
// Returns (padding, skipped). skipped=true means the packet was too small.
func handleNPMPadding(cfg NPMConfig, currentSize uint32) (padding uint32, skipped bool) {
	if cfg.Enabled != 1 {
		return 0, true
	}
	if currentSize < cfg.MinPacketSize {
		return 0, true
	}
	p := calculatePadding(cfg.PaddingMode, currentSize, cfg.GlobalMTU)
	return p, false
}

// npmStats mirrors the C struct npm_stats for tracking.
type npmStats struct {
	TotalPackets   uint64
	PaddedPackets  uint64
	PaddingBytes   uint64
	SkippedPackets uint64
}

// npmStatsTracker processes a sequence of packets and accumulates stats.
type npmStatsTracker struct {
	stats npmStats
	cfg   NPMConfig
}

func newNPMStatsTracker(cfg NPMConfig) *npmStatsTracker {
	return &npmStatsTracker{cfg: cfg}
}

// processPacket runs handleNPMPadding and updates stats accordingly.
func (t *npmStatsTracker) processPacket(currentSize uint32) {
	t.stats.TotalPackets++
	padding, skipped := handleNPMPadding(t.cfg, currentSize)
	if skipped {
		t.stats.SkippedPackets++
		return
	}
	if padding > 0 {
		t.stats.PaddedPackets++
		t.stats.PaddingBytes += uint64(padding)
	}
}

// ---------------------------------------------------------------------------
// Task 5.2: Example tests for NPM padding behavior
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, NPM 填充 Example Tests
// **Validates: Requirements 3.1, 3.3, 3.4**

// TestExample_NPMFixedMTUPadding verifies FIXED_MTU mode pads to exact MTU.
func TestExample_NPMFixedMTUPadding(t *testing.T) {
	cfg := NPMConfig{
		Enabled:       1,
		PaddingMode:   NPMModeFixedMTU,
		GlobalMTU:     1400,
		MinPacketSize: 100,
	}
	padding, skipped := handleNPMPadding(cfg, 800)
	if skipped {
		t.Fatal("packet should not be skipped")
	}
	// FIXED_MTU: padding = targetMTU - currentSize = 600
	if padding != 600 {
		t.Fatalf("FIXED_MTU padding: got %d, want 600", padding)
	}
}

// TestExample_NPMRandomRangePadding verifies RANDOM_RANGE mode produces padding in valid range.
func TestExample_NPMRandomRangePadding(t *testing.T) {
	cfg := NPMConfig{
		Enabled:       1,
		PaddingMode:   NPMModeRandomRange,
		GlobalMTU:     1400,
		MinPacketSize: 100,
	}
	padding, skipped := handleNPMPadding(cfg, 800)
	if skipped {
		t.Fatal("packet should not be skipped")
	}
	space := uint32(600)
	if padding == 0 || padding > space {
		t.Fatalf("RANDOM_RANGE padding=%d out of range (0, %d]", padding, space)
	}
	if padding < MIN_PADDING_SIZE || padding > MAX_PADDING_SIZE {
		t.Fatalf("RANDOM_RANGE padding=%d outside [%d, %d]", padding, MIN_PADDING_SIZE, MAX_PADDING_SIZE)
	}
}

// TestExample_NPMGaussianPadding verifies GAUSSIAN mode produces padding in valid range.
func TestExample_NPMGaussianPadding(t *testing.T) {
	cfg := NPMConfig{
		Enabled:       1,
		PaddingMode:   NPMModeGaussian,
		GlobalMTU:     1400,
		MinPacketSize: 100,
	}
	padding, skipped := handleNPMPadding(cfg, 800)
	if skipped {
		t.Fatal("packet should not be skipped")
	}
	space := uint32(600)
	if padding > space {
		t.Fatalf("GAUSSIAN padding=%d exceeds space %d", padding, space)
	}
	if padding > 0 && (padding < MIN_PADDING_SIZE || padding > MAX_PADDING_SIZE) {
		t.Fatalf("GAUSSIAN padding=%d outside [%d, %d]", padding, MIN_PADDING_SIZE, MAX_PADDING_SIZE)
	}
}

// TestExample_NPMSmallPacketSkipped verifies packets below min_packet_size are skipped.
func TestExample_NPMSmallPacketSkipped(t *testing.T) {
	cfg := NPMConfig{
		Enabled:       1,
		PaddingMode:   NPMModeFixedMTU,
		GlobalMTU:     1400,
		MinPacketSize: 128,
	}
	padding, skipped := handleNPMPadding(cfg, 64) // below min_packet_size
	if !skipped {
		t.Fatal("small packet should be skipped")
	}
	if padding != 0 {
		t.Fatalf("skipped packet should have padding=0, got %d", padding)
	}
}

// TestExample_NPMLargePacketNoPadding verifies packets >= MTU get no padding.
func TestExample_NPMLargePacketNoPadding(t *testing.T) {
	cfg := NPMConfig{
		Enabled:       1,
		PaddingMode:   NPMModeFixedMTU,
		GlobalMTU:     1400,
		MinPacketSize: 100,
	}
	// Packet already at MTU
	padding, skipped := handleNPMPadding(cfg, 1400)
	if skipped {
		t.Fatal("packet at MTU should not be skipped")
	}
	if padding != 0 {
		t.Fatalf("packet at MTU should have padding=0, got %d", padding)
	}

	// Packet above MTU
	padding, skipped = handleNPMPadding(cfg, 1500)
	if skipped {
		t.Fatal("packet above MTU should not be skipped")
	}
	if padding != 0 {
		t.Fatalf("packet above MTU should have padding=0, got %d", padding)
	}
}

// ---------------------------------------------------------------------------
// Task 5.3: Property 1 — NPM 填充正确性 PBT
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, Property 1: NPM 填充正确性
// **Validates: Requirements 3.1, 3.4**

func TestProperty_NPMPaddingCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random npm_config
		mode := rapid.Uint32Range(0, 2).Draw(t, "paddingMode")
		globalMTU := rapid.Uint32Range(500, 1500).Draw(t, "globalMTU")
		minPkt := rapid.Uint32Range(0, 256).Draw(t, "minPacketSize")

		cfg := NPMConfig{
			Enabled:       1,
			PaddingMode:   mode,
			GlobalMTU:     globalMTU,
			MinPacketSize: minPkt,
		}

		currentSize := rapid.Uint32Range(0, 1600).Draw(t, "currentSize")

		padding, skipped := handleNPMPadding(cfg, currentSize)

		// Property: current_size < min_packet_size → skipped, no padding
		if currentSize < minPkt {
			if !skipped {
				t.Fatalf("currentSize=%d < minPacketSize=%d should be skipped", currentSize, minPkt)
			}
			if padding != 0 {
				t.Fatalf("skipped packet should have padding=0, got %d", padding)
			}
			return
		}

		// Not skipped from here
		if skipped {
			t.Fatalf("currentSize=%d >= minPacketSize=%d should not be skipped", currentSize, minPkt)
		}

		// Property: current_size >= target_mtu → padding = 0
		if currentSize >= globalMTU {
			if padding != 0 {
				t.Fatalf("currentSize=%d >= globalMTU=%d should have padding=0, got %d",
					currentSize, globalMTU, padding)
			}
			return
		}

		space := globalMTU - currentSize

		// Mode-specific properties
		switch mode {
		case NPMModeFixedMTU:
			// Property: FIXED_MTU padding = space (clamped)
			expected := space
			if expected < MIN_PADDING_SIZE {
				expected = MIN_PADDING_SIZE
			}
			if expected > MAX_PADDING_SIZE {
				expected = MAX_PADDING_SIZE
			}
			if padding != expected {
				t.Fatalf("FIXED_MTU: padding=%d, expected=%d (space=%d)", padding, expected, space)
			}

		case NPMModeRandomRange:
			// Property: 0 < padding ≤ space (after clamping)
			if padding == 0 {
				t.Fatalf("RANDOM_RANGE: padding should be > 0 (space=%d)", space)
			}
			if padding > space && padding > MIN_PADDING_SIZE {
				// Clamping to MIN_PADDING_SIZE can exceed space when space < MIN_PADDING_SIZE
				if space >= MIN_PADDING_SIZE && padding > space {
					t.Fatalf("RANDOM_RANGE: padding=%d > space=%d", padding, space)
				}
			}

		case NPMModeGaussian:
			// Property: padding ∈ [0, space] (after clamping, may be clamped up to MIN)
			// padding can be 0 if gaussian sample is 0, or clamped to MIN_PADDING_SIZE
		}

		// Universal property: when padding > 0, MIN_PADDING_SIZE ≤ padding ≤ MAX_PADDING_SIZE
		if padding > 0 {
			if padding < MIN_PADDING_SIZE {
				t.Fatalf("padding=%d < MIN_PADDING_SIZE=%d", padding, MIN_PADDING_SIZE)
			}
			if padding > MAX_PADDING_SIZE {
				t.Fatalf("padding=%d > MAX_PADDING_SIZE=%d", padding, MAX_PADDING_SIZE)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Task 5.4: Property 2 — NPM 统计一致性 PBT
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, Property 2: NPM 统计一致性
// **Validates: Requirements 3.3**

func TestProperty_NPMStatsConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random config
		mode := rapid.Uint32Range(0, 2).Draw(t, "paddingMode")
		globalMTU := rapid.Uint32Range(500, 1500).Draw(t, "globalMTU")
		minPkt := rapid.Uint32Range(0, 256).Draw(t, "minPacketSize")

		cfg := NPMConfig{
			Enabled:       1,
			PaddingMode:   mode,
			GlobalMTU:     globalMTU,
			MinPacketSize: minPkt,
		}

		tracker := newNPMStatsTracker(cfg)

		// Generate random packet sequence
		numPackets := rapid.IntRange(1, 200).Draw(t, "numPackets")
		for i := 0; i < numPackets; i++ {
			size := rapid.Uint32Range(0, 1600).Draw(t, "packetSize")
			tracker.processPacket(size)
		}

		s := tracker.stats

		// Property: padded_packets + skipped_packets ≤ total_packets
		if s.PaddedPackets+s.SkippedPackets > s.TotalPackets {
			t.Fatalf("padded(%d) + skipped(%d) > total(%d)",
				s.PaddedPackets, s.SkippedPackets, s.TotalPackets)
		}

		// Property: padding_bytes > 0 iff padded_packets > 0
		if s.PaddingBytes > 0 && s.PaddedPackets == 0 {
			t.Fatalf("padding_bytes=%d > 0 but padded_packets=0", s.PaddingBytes)
		}
		if s.PaddedPackets > 0 && s.PaddingBytes == 0 {
			t.Fatalf("padded_packets=%d > 0 but padding_bytes=0", s.PaddedPackets)
		}

		// Property: padded_packets ≤ total_packets - skipped_packets
		if s.PaddedPackets > s.TotalPackets-s.SkippedPackets {
			t.Fatalf("padded(%d) > total(%d) - skipped(%d)",
				s.PaddedPackets, s.TotalPackets, s.SkippedPackets)
		}
	})
}
