package gtclient

import (
	"testing"

	"pgregory.net/rapid"
)

// Property 4: FEC 纠错能力 — up to parityShards lost shards recoverable
func TestProperty_FECErrorCorrection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		data := rapid.SliceOfN(rapid.Byte(), 1, 4096).Draw(t, "data")
		fec, err := NewFECCodec(8, 4)
		if err != nil {
			t.Fatal(err)
		}

		shards, err := fec.Encode(data)
		if err != nil {
			t.Fatal(err)
		}

		// Lose up to 4 shards (parityShards)
		numLost := rapid.IntRange(1, 4).Draw(t, "numLost")
		total := fec.TotalShards()
		lostIndices := make(map[int]bool)
		for len(lostIndices) < numLost {
			idx := rapid.IntRange(0, total-1).Draw(t, "lostIdx")
			lostIndices[idx] = true
		}
		for idx := range lostIndices {
			shards[idx] = nil
		}

		recovered, err := fec.Decode(shards, len(data))
		if err != nil {
			t.Fatalf("decode failed with %d lost shards: %v", numLost, err)
		}
		if string(recovered) != string(data) {
			t.Fatal("recovered data mismatch")
		}
	})
}

// Property 5: FEC 往返一致性 (no loss)
func TestProperty_FECRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		size := rapid.IntRange(1, 65536).Draw(t, "size")
		data := rapid.SliceOfN(rapid.Byte(), size, size).Draw(t, "data")

		fec, err := NewFECCodec(8, 4)
		if err != nil {
			t.Fatal(err)
		}

		shards, err := fec.Encode(data)
		if err != nil {
			t.Fatal(err)
		}

		recovered, err := fec.Decode(shards, len(data))
		if err != nil {
			t.Fatal(err)
		}

		if len(recovered) != len(data) {
			t.Fatalf("length mismatch: %d vs %d", len(recovered), len(data))
		}
		for i := range data {
			if recovered[i] != data[i] {
				t.Fatalf("byte %d mismatch", i)
			}
		}
	})
}

// Property 6: 分片重组一致性
func TestProperty_SamplerRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		size := rapid.IntRange(20, 65535).Draw(t, "size")
		data := rapid.SliceOfN(rapid.Byte(), size, size).Draw(t, "data")

		sampler := NewOverlapSampler()
		fragments := sampler.Split(data)
		if len(fragments) == 0 {
			t.Fatal("no fragments produced")
		}

		reassembled, err := sampler.Reassemble(fragments)
		if err != nil {
			t.Fatal(err)
		}

		if len(reassembled) != len(data) {
			t.Fatalf("length mismatch: %d vs %d", len(reassembled), len(data))
		}
		for i := range data {
			if reassembled[i] != data[i] {
				t.Fatalf("byte %d mismatch", i)
			}
		}
	})
}

// 边界条件：单字节
func TestFEC_SingleByte(t *testing.T) {
	fec, _ := NewFECCodec(8, 4)
	data := []byte{0x42}
	shards, err := fec.Encode(data)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := fec.Decode(shards, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0] != 0x42 {
		t.Fatalf("expected [0x42], got %v", recovered)
	}
}

// 边界条件：空数据
func TestFEC_EmptyData(t *testing.T) {
	fec, _ := NewFECCodec(8, 4)
	_, err := fec.Encode([]byte{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

// 边界条件：64KB 最大包
func TestFEC_MaxPacket(t *testing.T) {
	fec, _ := NewFECCodec(8, 4)
	data := make([]byte, 65536)
	for i := range data {
		data[i] = byte(i % 256)
	}
	shards, err := fec.Encode(data)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := fec.Decode(shards, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != len(data) {
		t.Fatalf("length mismatch: %d vs %d", len(recovered), len(data))
	}
}

// RouteTable 测试
func TestRouteTable(t *testing.T) {
	rt := &RouteTable{}
	if rt.Count() != 0 {
		t.Fatal("expected empty")
	}
}

// Sampler 边界：小数据
func TestSampler_SmallData(t *testing.T) {
	sampler := NewOverlapSampler()
	data := []byte("hello world - small")
	frags := sampler.Split(data)
	if len(frags) != 1 {
		t.Fatalf("expected 1 fragment for small data, got %d", len(frags))
	}
	reassembled, err := sampler.Reassemble(frags)
	if err != nil {
		t.Fatal(err)
	}
	if string(reassembled) != string(data) {
		t.Fatalf("mismatch: %q vs %q", reassembled, data)
	}
}
