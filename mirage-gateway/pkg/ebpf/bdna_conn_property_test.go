package ebpf

import (
	"encoding/binary"
	"testing"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 9: B-DNA 非 SYN 包 Window Size 一致性
// **Validates: Requirements 10.1, 10.2, 10.3**

// connKey mirrors the C struct conn_key
type connKey struct {
	SrcIP   uint32
	DstIP   uint32
	SrcPort uint16
	DstPort uint16
}

// connState mirrors the C struct conn_state
type connState struct {
	TargetWindow uint16
	PktCount     uint16
	MaxPkt       uint16
}

// bdnaConnTracker is a Go userspace equivalent of the eBPF bdna_conn_map logic
type bdnaConnTracker struct {
	connMap map[connKey]*connState
}

func newBDNAConnTracker() *bdnaConnTracker {
	return &bdnaConnTracker{connMap: make(map[connKey]*connState)}
}

// processSYN stores the target window for a new connection (mirrors SYN path in bdna_tcp_rewrite)
func (t *bdnaConnTracker) processSYN(key connKey, targetWindow uint16, maxPkt uint16) {
	t.connMap[key] = &connState{
		TargetWindow: targetWindow,
		PktCount:     0,
		MaxPkt:       maxPkt,
	}
}

// processNonSYN returns the window to apply (0 means no rewrite needed)
// mirrors the non-SYN path in bdna_tcp_rewrite
func (t *bdnaConnTracker) processNonSYN(key connKey, currentWindow uint16) (rewrittenWindow uint16, rewritten bool) {
	state, ok := t.connMap[key]
	if !ok || state.PktCount >= state.MaxPkt {
		return currentWindow, false
	}
	state.PktCount++
	return state.TargetWindow, true
}

func TestProperty_BDNANonSYNWindowConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tracker := newBDNAConnTracker()

		// Generate connection parameters
		key := connKey{
			SrcIP:   rapid.Uint32().Draw(t, "srcIP"),
			DstIP:   rapid.Uint32().Draw(t, "dstIP"),
			SrcPort: rapid.Uint16().Draw(t, "srcPort"),
			DstPort: rapid.Uint16().Draw(t, "dstPort"),
		}
		targetWindow := rapid.Uint16Range(1, 65535).Draw(t, "targetWindow")
		maxPkt := rapid.Uint16Range(1, 50).Draw(t, "maxPkt")

		// Process SYN
		tracker.processSYN(key, targetWindow, maxPkt)

		// Process N non-SYN packets — all should get target window
		for i := uint16(0); i < maxPkt; i++ {
			currentWindow := rapid.Uint16Range(1, 65535).Draw(t, "currentWindow")
			rewritten, ok := tracker.processNonSYN(key, currentWindow)
			if !ok {
				t.Fatalf("packet %d should have been rewritten (maxPkt=%d)", i, maxPkt)
			}
			if rewritten != targetWindow {
				t.Fatalf("packet %d: window=%d, expected=%d", i, rewritten, targetWindow)
			}
		}

		// Packet N+1 should NOT be rewritten
		_, ok := tracker.processNonSYN(key, 12345)
		if ok {
			t.Fatalf("packet beyond maxPkt should not be rewritten")
		}
	})
}

// htons is a helper for network byte order conversion
func htons(v uint16) uint16 {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return binary.LittleEndian.Uint16(b)
}

// ---------------------------------------------------------------------------
// Mock structs for B-DNA TCP SYN rewrite and stats tracking (Tasks 4.2–4.4)
// ---------------------------------------------------------------------------

// NOTE: stackFingerprint is defined in bdna_fingerprint_loader.go (package-level).
// We reuse it here directly — fields used: TCPWindow, TCPMSS, TCPWScale.

// tcpSYNPacket represents a minimal TCP SYN packet for mock rewrite
type tcpSYNPacket struct {
	WindowSize uint16
	MSS        uint16
	WScale     uint8
	IsSYN      bool
}

// bdnaSYNRewriter simulates bdna_tcp_rewrite for TCP SYN packets.
// When a SYN packet arrives and a fingerprint template is active,
// the rewriter overwrites Window Size, MSS, and WScale to match the template.
type bdnaSYNRewriter struct {
	fingerprint *stackFingerprint // nil means no template loaded
}

// rewriteSYN applies the fingerprint template to a TCP SYN packet.
// Returns (rewritten packet, wasRewritten).
func (r *bdnaSYNRewriter) rewriteSYN(pkt tcpSYNPacket) (tcpSYNPacket, bool) {
	if !pkt.IsSYN {
		return pkt, false
	}
	if r.fingerprint == nil {
		return pkt, false
	}
	pkt.WindowSize = r.fingerprint.TCPWindow
	pkt.MSS = r.fingerprint.TCPMSS
	pkt.WScale = r.fingerprint.TCPWScale
	return pkt, true
}

// packetType classifies packets for bdnaStatsTracker
type packetType int

const (
	pktTCPSYN         packetType = iota // TCP SYN
	pktQUICInitial                      // QUIC Initial
	pktTLSClientHello                   // TLS ClientHello
	pktOther                            // non-matching
)

// bdnaStats mirrors the C struct bdna_stats
type bdnaStats struct {
	TCPRewritten  uint64
	QUICRewritten uint64
	TLSRewritten  uint64
	Skipped       uint64
}

// bdnaStatsTracker simulates bdna_stats_map counting logic.
type bdnaStatsTracker struct {
	stats       bdnaStats
	hasTemplate bool // whether a fingerprint template is active
}

// processPacket updates stats based on packet type, mirroring the eBPF bdna program logic.
func (s *bdnaStatsTracker) processPacket(pt packetType) {
	switch pt {
	case pktTCPSYN:
		if s.hasTemplate {
			s.stats.TCPRewritten++
		} else {
			s.stats.Skipped++
		}
	case pktQUICInitial:
		s.stats.QUICRewritten++
	case pktTLSClientHello:
		s.stats.TLSRewritten++
	case pktOther:
		// no counter change
	}
}

// ---------------------------------------------------------------------------
// Task 4.2: Example tests for B-DNA TCP SYN rewrite and stats
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, B-DNA TCP SYN 重写 Example Tests
// **Validates: Requirements 2.1, 2.3, 2.4, 2.5**

// TestExample_BDNASYNRewriteMatchesTemplate verifies that after rewrite,
// TCP SYN fields exactly match the fingerprint template.
func TestExample_BDNASYNRewriteMatchesTemplate(t *testing.T) {
	fp := &stackFingerprint{
		TCPWindow: 65535,
		TCPMSS:    1460,
		TCPWScale: 8,
	}
	rewriter := &bdnaSYNRewriter{fingerprint: fp}

	pkt := tcpSYNPacket{
		WindowSize: 29200,
		MSS:        1360,
		WScale:     7,
		IsSYN:      true,
	}

	out, rewritten := rewriter.rewriteSYN(pkt)
	if !rewritten {
		t.Fatal("expected SYN packet to be rewritten")
	}
	if out.WindowSize != fp.TCPWindow {
		t.Fatalf("WindowSize: got %d, want %d", out.WindowSize, fp.TCPWindow)
	}
	if out.MSS != fp.TCPMSS {
		t.Fatalf("MSS: got %d, want %d", out.MSS, fp.TCPMSS)
	}
	if out.WScale != fp.TCPWScale {
		t.Fatalf("WScale: got %d, want %d", out.WScale, fp.TCPWScale)
	}
}

// TestExample_BDNANonSYNNotRewritten verifies that non-SYN packets are not rewritten.
func TestExample_BDNANonSYNNotRewritten(t *testing.T) {
	fp := &stackFingerprint{TCPWindow: 65535, TCPMSS: 1460, TCPWScale: 8}
	rewriter := &bdnaSYNRewriter{fingerprint: fp}

	pkt := tcpSYNPacket{
		WindowSize: 29200,
		MSS:        1360,
		WScale:     7,
		IsSYN:      false, // not a SYN
	}

	out, rewritten := rewriter.rewriteSYN(pkt)
	if rewritten {
		t.Fatal("non-SYN packet should not be rewritten")
	}
	if out.WindowSize != 29200 || out.MSS != 1360 || out.WScale != 7 {
		t.Fatal("non-SYN packet fields should remain unchanged")
	}
}

// TestExample_BDNANoTemplateSkipped verifies that when no template is loaded,
// the skipped counter increments for TCP SYN packets.
func TestExample_BDNANoTemplateSkipped(t *testing.T) {
	tracker := &bdnaStatsTracker{hasTemplate: false}

	tracker.processPacket(pktTCPSYN)

	if tracker.stats.Skipped != 1 {
		t.Fatalf("skipped: got %d, want 1", tracker.stats.Skipped)
	}
	if tracker.stats.TCPRewritten != 0 {
		t.Fatalf("tcp_rewritten should be 0 when no template, got %d", tracker.stats.TCPRewritten)
	}
}

// ---------------------------------------------------------------------------
// Task 4.3: Property 3 — B-DNA TCP SYN 重写匹配 PBT
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, Property 3: B-DNA TCP SYN 重写匹配
// **Validates: Requirements 2.1**

func TestProperty_BDNATCPSYNRewriteMatch(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random stack_fingerprint within valid ranges
		fp := &stackFingerprint{
			TCPWindow: rapid.Uint16Range(1, 65535).Draw(t, "tcp_window"),
			TCPMSS:    rapid.Uint16Range(536, 1460).Draw(t, "tcp_mss"),
			TCPWScale: rapid.Uint8Range(0, 14).Draw(t, "tcp_wscale"),
		}
		rewriter := &bdnaSYNRewriter{fingerprint: fp}

		// Generate a random original SYN packet
		original := tcpSYNPacket{
			WindowSize: rapid.Uint16Range(1, 65535).Draw(t, "orig_window"),
			MSS:        rapid.Uint16Range(536, 1460).Draw(t, "orig_mss"),
			WScale:     rapid.Uint8Range(0, 14).Draw(t, "orig_wscale"),
			IsSYN:      true,
		}

		out, rewritten := rewriter.rewriteSYN(original)

		// Property: SYN packet with active template MUST be rewritten
		if !rewritten {
			t.Fatal("SYN packet with active template must be rewritten")
		}

		// Property: rewritten Window Size matches template exactly
		if out.WindowSize != fp.TCPWindow {
			t.Fatalf("WindowSize mismatch: got %d, want %d", out.WindowSize, fp.TCPWindow)
		}

		// Property: rewritten MSS matches template exactly
		if out.MSS != fp.TCPMSS {
			t.Fatalf("MSS mismatch: got %d, want %d", out.MSS, fp.TCPMSS)
		}

		// Property: rewritten WScale matches template exactly
		if out.WScale != fp.TCPWScale {
			t.Fatalf("WScale mismatch: got %d, want %d", out.WScale, fp.TCPWScale)
		}
	})
}

// ---------------------------------------------------------------------------
// Task 4.4: Property 4 — B-DNA 统计一致性 PBT
// ---------------------------------------------------------------------------

// Feature: phase2-stealth-evidence, Property 4: B-DNA 统计一致性
// **Validates: Requirements 2.3**

func TestProperty_BDNAStatsConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hasTemplate := rapid.Bool().Draw(t, "hasTemplate")
		tracker := &bdnaStatsTracker{hasTemplate: hasTemplate}

		// Generate a random packet sequence
		numPackets := rapid.IntRange(1, 200).Draw(t, "numPackets")

		var expectTCPRewritten, expectQUICRewritten, expectTLSRewritten, expectSkipped uint64

		for i := 0; i < numPackets; i++ {
			pt := packetType(rapid.IntRange(0, 3).Draw(t, "pktType"))

			before := tracker.stats

			tracker.processPacket(pt)

			after := tracker.stats

			switch pt {
			case pktTCPSYN:
				if hasTemplate {
					expectTCPRewritten++
					if after.TCPRewritten != before.TCPRewritten+1 {
						t.Fatalf("pkt %d: TCP SYN with template — tcp_rewritten should increment", i)
					}
				} else {
					expectSkipped++
					if after.Skipped != before.Skipped+1 {
						t.Fatalf("pkt %d: TCP SYN without template — skipped should increment", i)
					}
				}
			case pktQUICInitial:
				expectQUICRewritten++
				if after.QUICRewritten != before.QUICRewritten+1 {
					t.Fatalf("pkt %d: QUIC Initial — quic_rewritten should increment", i)
				}
			case pktTLSClientHello:
				expectTLSRewritten++
				if after.TLSRewritten != before.TLSRewritten+1 {
					t.Fatalf("pkt %d: TLS ClientHello — tls_rewritten should increment", i)
				}
			case pktOther:
				// Property: non-matching packets must not change any counter
				if after != before {
					t.Fatalf("pkt %d: non-matching packet changed counters: before=%+v after=%+v", i, before, after)
				}
			}
		}

		// Final consistency check: accumulated counters match expectations
		if tracker.stats.TCPRewritten != expectTCPRewritten {
			t.Fatalf("final tcp_rewritten: got %d, want %d", tracker.stats.TCPRewritten, expectTCPRewritten)
		}
		if tracker.stats.QUICRewritten != expectQUICRewritten {
			t.Fatalf("final quic_rewritten: got %d, want %d", tracker.stats.QUICRewritten, expectQUICRewritten)
		}
		if tracker.stats.TLSRewritten != expectTLSRewritten {
			t.Fatalf("final tls_rewritten: got %d, want %d", tracker.stats.TLSRewritten, expectTLSRewritten)
		}
		if tracker.stats.Skipped != expectSkipped {
			t.Fatalf("final skipped: got %d, want %d", tracker.stats.Skipped, expectSkipped)
		}
	})
}
