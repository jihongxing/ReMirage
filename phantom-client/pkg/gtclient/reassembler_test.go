package gtclient

import (
	"bytes"
	"crypto/rand"
	"testing"

	"pgregory.net/rapid"
)

// TestReassembler_FullPipeline verifies: Split → FEC Encode → Shard Header → Ingest → FEC Decode → Reassemble
func TestReassembler_FullPipeline(t *testing.T) {
	fec, err := NewFECCodec(8, 4)
	if err != nil {
		t.Fatal(err)
	}
	sampler := NewOverlapSampler()
	reasm := NewReassembler(fec, sampler)

	// Original IP packet
	original := make([]byte, 1200)
	rand.Read(original)

	// Sender side: Split → FEC Encode → build wire shards
	fragments := sampler.Split(original)
	fragCount := len(fragments)

	for _, frag := range fragments {
		shards, err := fec.Encode(frag.Data)
		if err != nil {
			t.Fatalf("fec encode: %v", err)
		}
		for shardIdx, shard := range shards {
			header := EncodeShardHeader(frag.SeqNum, len(frag.Data), frag.OverlapID, shardIdx, fragCount)
			payload := append(header, shard...)
			reasm.IngestShard(payload)
		}
	}

	// Should have a completed packet
	select {
	case pkt := <-reasm.Completed():
		if !bytes.Equal(pkt, original) {
			t.Fatalf("reassembled packet mismatch: got %d bytes, want %d", len(pkt), len(original))
		}
	default:
		t.Fatal("no completed packet after ingesting all shards")
	}
}

// TestReassembler_WithLoss verifies FEC recovery when some shards are lost.
func TestReassembler_WithLoss(t *testing.T) {
	fec, err := NewFECCodec(8, 4)
	if err != nil {
		t.Fatal(err)
	}
	sampler := NewOverlapSampler()
	reasm := NewReassembler(fec, sampler)

	// Small packet that fits in one fragment
	original := make([]byte, 200)
	rand.Read(original)

	fragments := sampler.Split(original)
	if len(fragments) != 1 {
		t.Fatalf("expected 1 fragment for 200 bytes, got %d", len(fragments))
	}

	frag := fragments[0]
	shards, err := fec.Encode(frag.Data)
	if err != nil {
		t.Fatal(err)
	}

	// Drop 4 shards (parity count) — should still recover with 8 data shards
	for shardIdx, shard := range shards {
		if shardIdx >= 8 { // skip all 4 parity shards
			continue
		}
		header := EncodeShardHeader(frag.SeqNum, len(frag.Data), frag.OverlapID, shardIdx, 1)
		payload := append(header, shard...)
		reasm.IngestShard(payload)
	}

	select {
	case pkt := <-reasm.Completed():
		if !bytes.Equal(pkt, original) {
			t.Fatalf("reassembled packet mismatch after loss recovery")
		}
	default:
		t.Fatal("FEC recovery failed: no completed packet")
	}
}

// TestProperty_ReassemblerRoundTrip property-based test for arbitrary packet sizes.
func TestProperty_ReassemblerRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		size := rapid.IntRange(1, 4000).Draw(t, "size")
		data := make([]byte, size)
		rand.Read(data)

		fec, _ := NewFECCodec(8, 4)
		sampler := NewOverlapSampler()
		reasm := NewReassembler(fec, sampler)

		fragments := sampler.Split(data)
		fragCount := len(fragments)
		for _, frag := range fragments {
			shards, err := fec.Encode(frag.Data)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			for idx, shard := range shards {
				header := EncodeShardHeader(frag.SeqNum, len(frag.Data), frag.OverlapID, idx, fragCount)
				reasm.IngestShard(append(header, shard...))
			}
		}

		select {
		case pkt := <-reasm.Completed():
			if !bytes.Equal(pkt, data) {
				t.Fatalf("mismatch: got %d bytes, want %d", len(pkt), size)
			}
		default:
			t.Fatalf("no completed packet for size=%d", size)
		}
	})
}
