package gtclient

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"sort"
)

// OverlapSampler implements G-Tunnel's overlap sampling fragmentation.
type OverlapSampler struct {
	ChunkSize   int // default 400
	OverlapSize int // default 100
}

// Fragment represents a single overlap-sampled fragment.
type Fragment struct {
	Data      []byte
	SeqNum    int
	OverlapID uint32 // CRC32 checksum for validation
}

// NewOverlapSampler creates a sampler with default parameters.
func NewOverlapSampler() *OverlapSampler {
	return &OverlapSampler{
		ChunkSize:   400,
		OverlapSize: 100,
	}
}

// Split fragments data using overlap sampling.
func (s *OverlapSampler) Split(data []byte) []Fragment {
	if len(data) == 0 {
		return nil
	}

	stride := s.ChunkSize - s.OverlapSize
	if stride <= 0 {
		stride = 1
	}

	var fragments []Fragment
	seq := 0

	for offset := 0; offset < len(data); offset += stride {
		end := offset + s.ChunkSize
		if end > len(data) {
			end = len(data)
		}

		chunk := make([]byte, end-offset)
		copy(chunk, data[offset:end])

		fragments = append(fragments, Fragment{
			Data:      chunk,
			SeqNum:    seq,
			OverlapID: crc32.ChecksumIEEE(data),
		})
		seq++

		if end == len(data) {
			break
		}
	}

	return fragments
}

// Reassemble reconstructs original data from fragments.
func (s *OverlapSampler) Reassemble(fragments []Fragment) ([]byte, error) {
	if len(fragments) == 0 {
		return nil, fmt.Errorf("no fragments")
	}

	// Sort by SeqNum
	sorted := make([]Fragment, len(fragments))
	copy(sorted, fragments)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].SeqNum < sorted[j].SeqNum
	})

	// Validate OverlapID consistency
	overlapID := sorted[0].OverlapID
	for _, f := range sorted {
		if f.OverlapID != overlapID {
			return nil, fmt.Errorf("overlap ID mismatch: %d vs %d", f.OverlapID, overlapID)
		}
	}

	stride := s.ChunkSize - s.OverlapSize
	if stride <= 0 {
		stride = 1
	}

	// Calculate total size from fragment layout
	if len(sorted) == 1 {
		return sorted[0].Data, nil
	}

	// Total = stride * (n-1) + len(last fragment)
	totalSize := stride*(len(sorted)-1) + len(sorted[len(sorted)-1].Data)
	result := make([]byte, totalSize)

	for i, f := range sorted {
		offset := i * stride
		copy(result[offset:], f.Data)
	}

	return result, nil
}

// EncodeFragmentHeader serializes fragment metadata for wire format.
func EncodeFragmentHeader(f *Fragment) []byte {
	header := make([]byte, 8)
	binary.BigEndian.PutUint16(header[0:2], uint16(f.SeqNum))
	binary.BigEndian.PutUint16(header[2:4], uint16(len(f.Data)))
	binary.BigEndian.PutUint32(header[4:8], f.OverlapID)
	return header
}

// DecodeFragmentHeader deserializes fragment metadata.
func DecodeFragmentHeader(header []byte) (seqNum int, dataLen int, overlapID uint32, err error) {
	if len(header) < 8 {
		return 0, 0, 0, fmt.Errorf("header too short")
	}
	seqNum = int(binary.BigEndian.Uint16(header[0:2]))
	dataLen = int(binary.BigEndian.Uint16(header[2:4]))
	overlapID = binary.BigEndian.Uint32(header[4:8])
	return
}
