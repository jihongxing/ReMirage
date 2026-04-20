package gtclient

import (
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"time"
)

// FragmentGroup collects shards belonging to one fragment (identified by SeqNum + OverlapID).
type FragmentGroup struct {
	SeqNum     int
	OverlapID  uint32
	DataLen    int // original fragment data length from header
	FragCount  int // total number of fragments in this packet
	Shards     [][]byte
	ShardCount int
	Decoded    []byte // decoded fragment data (nil until FEC succeeds)
	CreatedAt  time.Time
}

// Reassembler buffers incoming shards and produces complete IP packets.
type Reassembler struct {
	fec       *FECCodec
	sampler   *OverlapSampler
	mu        sync.Mutex
	groups    map[uint64]*FragmentGroup // key = overlapID<<16 | seqNum
	completed chan []byte               // reassembled IP packets ready for TUN
	ttl       time.Duration             // max age before eviction
}

// NewReassembler creates a reassembler with the given FEC codec.
func NewReassembler(fec *FECCodec, sampler *OverlapSampler) *Reassembler {
	r := &Reassembler{
		fec:       fec,
		sampler:   sampler,
		groups:    make(map[uint64]*FragmentGroup),
		completed: make(chan []byte, 256),
		ttl:       5 * time.Second,
	}
	go r.evictionLoop()
	return r
}

// groupKey computes a unique key for a fragment group.
func groupKey(overlapID uint32, seqNum int) uint64 {
	return uint64(overlapID)<<16 | uint64(uint16(seqNum))
}

// IngestShard processes a single decrypted datagram payload.
// Wire format: [2B seqNum][2B dataLen][4B overlapID][2B shardIndex][2B fragCount][shard...]
func (r *Reassembler) IngestShard(payload []byte) {
	if len(payload) < 12 {
		return
	}

	seqNum := int(binary.BigEndian.Uint16(payload[0:2]))
	dataLen := int(binary.BigEndian.Uint16(payload[2:4]))
	overlapID := binary.BigEndian.Uint32(payload[4:8])
	shardIdx := int(binary.BigEndian.Uint16(payload[8:10]))
	fragCount := int(binary.BigEndian.Uint16(payload[10:12]))
	shardData := payload[12:]

	totalShards := r.fec.TotalShards()
	if shardIdx >= totalShards {
		return
	}

	key := groupKey(overlapID, seqNum)

	r.mu.Lock()
	defer r.mu.Unlock()

	grp, exists := r.groups[key]
	if !exists {
		grp = &FragmentGroup{
			SeqNum:    seqNum,
			OverlapID: overlapID,
			DataLen:   dataLen,
			FragCount: fragCount,
			Shards:    make([][]byte, totalShards),
			CreatedAt: time.Now(),
		}
		r.groups[key] = grp
	}

	if grp.Decoded != nil {
		return // already decoded
	}

	if grp.Shards[shardIdx] == nil {
		copied := make([]byte, len(shardData))
		copy(copied, shardData)
		grp.Shards[shardIdx] = copied
		grp.ShardCount++
	}

	// Try FEC decode when we have enough shards
	if grp.ShardCount >= r.fec.DataShards() && grp.Decoded == nil {
		decoded, err := r.fec.Decode(grp.Shards, grp.DataLen)
		if err != nil {
			return
		}
		grp.Decoded = decoded
		// Free shard memory
		grp.Shards = nil

		// Try to assemble the full packet from all fragments of this overlapID
		r.tryAssemble(overlapID)
	}
}

// tryAssemble checks if all fragments for an overlapID are decoded and reassembles.
// Must be called with r.mu held.
func (r *Reassembler) tryAssemble(overlapID uint32) {
	var frags []Fragment
	expectedCount := 0

	for _, grp := range r.groups {
		if grp.OverlapID != overlapID {
			continue
		}
		if expectedCount == 0 {
			expectedCount = grp.FragCount
		}
		if grp.Decoded == nil {
			return // not all decoded yet
		}
		frags = append(frags, Fragment{
			Data:      grp.Decoded,
			SeqNum:    grp.SeqNum,
			OverlapID: grp.OverlapID,
		})
	}

	if len(frags) == 0 || len(frags) < expectedCount {
		return // still waiting for more fragments
	}

	// Sort by SeqNum
	sort.Slice(frags, func(i, j int) bool {
		return frags[i].SeqNum < frags[j].SeqNum
	})

	reassembled, err := r.sampler.Reassemble(frags)
	if err != nil || len(reassembled) == 0 {
		return
	}

	// Deliver
	select {
	case r.completed <- reassembled:
	default:
	}

	// Cleanup
	for key, grp := range r.groups {
		if grp.OverlapID == overlapID {
			delete(r.groups, key)
		}
	}
}

// Completed returns the channel of reassembled IP packets.
func (r *Reassembler) Completed() <-chan []byte {
	return r.completed
}

// evictionLoop removes stale fragment groups.
func (r *Reassembler) evictionLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		now := time.Now()
		for key, grp := range r.groups {
			if now.Sub(grp.CreatedAt) > r.ttl {
				delete(r.groups, key)
			}
		}
		r.mu.Unlock()
	}
}

// EncodeShardHeader creates the 12-byte extended header for wire transmission.
// Format: [2B seqNum][2B dataLen][4B overlapID][2B shardIndex][2B fragCount]
func EncodeShardHeader(seqNum int, dataLen int, overlapID uint32, shardIdx int, fragCount int) []byte {
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], uint16(seqNum))
	binary.BigEndian.PutUint16(header[2:4], uint16(dataLen))
	binary.BigEndian.PutUint32(header[4:8], overlapID)
	binary.BigEndian.PutUint16(header[8:10], uint16(shardIdx))
	binary.BigEndian.PutUint16(header[10:12], uint16(fragCount))
	return header
}

// DecodeShardMeta extracts metadata from the 12-byte shard header.
func DecodeShardMeta(header []byte) (seqNum, dataLen, shardIdx, fragCount int, overlapID uint32, err error) {
	if len(header) < 12 {
		return 0, 0, 0, 0, 0, fmt.Errorf("shard header too short: %d", len(header))
	}
	seqNum = int(binary.BigEndian.Uint16(header[0:2]))
	dataLen = int(binary.BigEndian.Uint16(header[2:4]))
	overlapID = binary.BigEndian.Uint32(header[4:8])
	shardIdx = int(binary.BigEndian.Uint16(header[8:10]))
	fragCount = int(binary.BigEndian.Uint16(header[10:12]))
	return
}
