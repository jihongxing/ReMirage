package gtclient

import (
	"fmt"

	"github.com/klauspost/reedsolomon"
)

// FECCodec provides Reed-Solomon forward error correction.
type FECCodec struct {
	dataShards   int
	parityShards int
	enc          reedsolomon.Encoder
}

// NewFECCodec creates a new FEC codec (default: 8 data + 4 parity).
func NewFECCodec(data, parity int) (*FECCodec, error) {
	enc, err := reedsolomon.New(data, parity)
	if err != nil {
		return nil, fmt.Errorf("reedsolomon.New: %w", err)
	}
	return &FECCodec{
		dataShards:   data,
		parityShards: parity,
		enc:          enc,
	}, nil
}

// Encode splits data into dataShards + parityShards shards.
func (f *FECCodec) Encode(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	// Calculate shard size (ceil division)
	shardSize := (len(data) + f.dataShards - 1) / f.dataShards

	// Pad data to fill all data shards evenly
	padded := make([]byte, shardSize*f.dataShards)
	copy(padded, data)

	// Split into data shards
	shards := make([][]byte, f.dataShards+f.parityShards)
	for i := 0; i < f.dataShards; i++ {
		shards[i] = padded[i*shardSize : (i+1)*shardSize]
	}
	// Allocate parity shards
	for i := f.dataShards; i < f.dataShards+f.parityShards; i++ {
		shards[i] = make([]byte, shardSize)
	}

	if err := f.enc.Encode(shards); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	// Prepend original data length as metadata in first shard header
	// We store the original length in a separate way: return it alongside
	// For simplicity, we encode the original length in the first 4 bytes of shard[0]
	// But that changes shard structure. Instead, we'll use a wrapper approach.
	// Store original length externally — caller tracks it.

	return shards, nil
}

// Decode reconstructs original data from shards (nil shards = lost).
// originalLen is the original data length before padding.
func (f *FECCodec) Decode(shards [][]byte, originalLen int) ([]byte, error) {
	if len(shards) != f.dataShards+f.parityShards {
		return nil, fmt.Errorf("expected %d shards, got %d", f.dataShards+f.parityShards, len(shards))
	}

	if err := f.enc.Reconstruct(shards); err != nil {
		return nil, fmt.Errorf("reconstruct: %w", err)
	}

	ok, err := f.enc.Verify(shards)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("verification failed")
	}

	// Concatenate data shards
	var result []byte
	for i := 0; i < f.dataShards; i++ {
		result = append(result, shards[i]...)
	}

	// Trim to original length
	if originalLen > 0 && originalLen <= len(result) {
		result = result[:originalLen]
	}

	return result, nil
}

// DataShards returns the number of data shards.
func (f *FECCodec) DataShards() int { return f.dataShards }

// ParityShards returns the number of parity shards.
func (f *FECCodec) ParityShards() int { return f.parityShards }

// TotalShards returns total shard count.
func (f *FECCodec) TotalShards() int { return f.dataShards + f.parityShards }
