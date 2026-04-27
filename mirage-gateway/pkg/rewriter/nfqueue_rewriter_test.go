package rewriter

import (
	"encoding/binary"
	"testing"
)

// buildTCPSYNPacket 构造一个最小的 IPv4 TCP SYN 包用于测试。
func buildTCPSYNPacket(srcIP, dstIP [4]byte, srcPort, dstPort, windowSize uint16, ttl uint8) []byte {
	// IPv4 header (20 bytes) + TCP header (20 bytes) = 40 bytes minimum
	pkt := make([]byte, 40)

	// IPv4 header
	pkt[0] = 0x45                            // Version=4, IHL=5 (20 bytes)
	pkt[1] = 0                               // DSCP/ECN
	binary.BigEndian.PutUint16(pkt[2:4], 40) // Total length
	pkt[8] = ttl                             // TTL
	pkt[9] = 6                               // Protocol = TCP
	copy(pkt[12:16], srcIP[:])
	copy(pkt[16:20], dstIP[:])

	// TCP header
	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	pkt[32] = 0x50 // Data offset = 5 (20 bytes), no options
	pkt[33] = 0x02 // SYN flag
	binary.BigEndian.PutUint16(pkt[34:36], windowSize)

	// Compute IP checksum
	recalcIPChecksum(pkt[:20])
	// Compute TCP checksum
	recalcTCPChecksum(pkt, 20)

	return pkt
}

func TestHandlePacket_NoMark(t *testing.T) {
	r, err := NewNFQueueRewriter(NFQueueRewriterConfig{QueueNum: 1})
	if err != nil {
		t.Fatal(err)
	}

	pkt := buildTCPSYNPacket([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 12345, 443, 65535, 64)
	result, modified := r.HandlePacket(pkt, 0)

	if modified {
		t.Error("mark=0 should not trigger rewrite")
	}
	if len(result) != len(pkt) {
		t.Error("packet length should be unchanged")
	}
}

func TestHandlePacket_WithFingerprint(t *testing.T) {
	fp := &TCPFingerprint{
		WindowSize: 32768,
		TTL:        128,
	}
	r, err := NewNFQueueRewriter(NFQueueRewriterConfig{
		QueueNum:      1,
		FingerprintDB: map[uint8]*TCPFingerprint{1: fp},
	})
	if err != nil {
		t.Fatal(err)
	}

	pkt := buildTCPSYNPacket([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 12345, 443, 65535, 64)
	result, modified := r.HandlePacket(pkt, 1) // mark=1 → template ID=1

	if !modified {
		t.Fatal("expected packet to be rewritten")
	}

	// Verify TTL was rewritten
	if result[8] != 128 {
		t.Errorf("TTL: got %d, want 128", result[8])
	}

	// Verify Window Size was rewritten
	gotWin := binary.BigEndian.Uint16(result[34:36])
	if gotWin != 32768 {
		t.Errorf("WindowSize: got %d, want 32768", gotWin)
	}
}

func TestHandlePacket_UnknownTemplate(t *testing.T) {
	r, err := NewNFQueueRewriter(NFQueueRewriterConfig{
		QueueNum:      1,
		FingerprintDB: map[uint8]*TCPFingerprint{},
	})
	if err != nil {
		t.Fatal(err)
	}

	pkt := buildTCPSYNPacket([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 12345, 443, 65535, 64)
	_, modified := r.HandlePacket(pkt, 99) // mark=99, no template

	if modified {
		t.Error("unknown template should not trigger rewrite")
	}
}

func TestHandlePacket_NonSYN(t *testing.T) {
	fp := &TCPFingerprint{WindowSize: 32768, TTL: 128}
	r, err := NewNFQueueRewriter(NFQueueRewriterConfig{
		QueueNum:      1,
		FingerprintDB: map[uint8]*TCPFingerprint{1: fp},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Build a non-SYN packet (ACK)
	pkt := buildTCPSYNPacket([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 12345, 443, 65535, 64)
	pkt[33] = 0x10 // ACK flag instead of SYN
	recalcTCPChecksum(pkt, 20)

	_, modified := r.HandlePacket(pkt, 1)
	if modified {
		t.Error("non-SYN packet should not be rewritten")
	}
}

func TestHandlePacket_TooShort(t *testing.T) {
	r, err := NewNFQueueRewriter(NFQueueRewriterConfig{
		QueueNum:      1,
		FingerprintDB: map[uint8]*TCPFingerprint{1: {WindowSize: 32768}},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, modified := r.HandlePacket([]byte{0x45, 0x00}, 1)
	if modified {
		t.Error("short packet should not be rewritten")
	}
}

func TestHandlePacket_NonTCP(t *testing.T) {
	r, err := NewNFQueueRewriter(NFQueueRewriterConfig{
		QueueNum:      1,
		FingerprintDB: map[uint8]*TCPFingerprint{1: {WindowSize: 32768}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Build a UDP packet (protocol=17)
	pkt := buildTCPSYNPacket([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 12345, 443, 65535, 64)
	pkt[9] = 17 // UDP
	recalcIPChecksum(pkt[:20])

	_, modified := r.HandlePacket(pkt, 1)
	if modified {
		t.Error("non-TCP packet should not be rewritten")
	}
}

func TestNewNFQueueRewriter_InvalidConfig(t *testing.T) {
	_, err := NewNFQueueRewriter(NFQueueRewriterConfig{QueueNum: 0})
	if err == nil {
		t.Error("expected error for QueueNum=0")
	}
}

func TestStats(t *testing.T) {
	fp := &TCPFingerprint{WindowSize: 32768, TTL: 128}
	r, err := NewNFQueueRewriter(NFQueueRewriterConfig{
		QueueNum:      1,
		FingerprintDB: map[uint8]*TCPFingerprint{1: fp},
	})
	if err != nil {
		t.Fatal(err)
	}

	pkt := buildTCPSYNPacket([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 12345, 443, 65535, 64)

	// Process with mark=0 (pass)
	r.HandlePacket(pkt, 0)
	// Process with mark=1 (rewrite)
	r.HandlePacket(pkt, 1)

	processed, rewritten, passed := r.Stats()
	if processed != 2 {
		t.Errorf("processed: got %d, want 2", processed)
	}
	if rewritten != 1 {
		t.Errorf("rewritten: got %d, want 1", rewritten)
	}
	if passed != 1 {
		t.Errorf("passed: got %d, want 1", passed)
	}
}
