// Package gtunnel - G-Tunnel FEC 编码器（Go 控制面）
// C 数据面负责 AVX-512 加速的 GF(2^8) 运算，Go 负责调度和降级
package gtunnel

/*
#cgo CFLAGS: -O2
#cgo LDFLAGS:
#include "fec_accel.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"unsafe"
)

const (
	DataShards   = 8    // 数据分片数
	ParityShards = 4    // 冗余分片数
	ShardSize    = 1024 // 每个分片大小

	// 批处理阈值：低于此字节数时累积后再跨 CGO 边界
	batchThreshold = 4096
)

// FECProcessor FEC 处理器
type FECProcessor struct {
	dataShards   int
	parityShards int
	shardSize    int
	useAVX512    bool // 是否使用 AVX-512 加速
	initOnce     sync.Once
}

// NewFECProcessor 创建 FEC 处理器
func NewFECProcessor() *FECProcessor {
	f := &FECProcessor{
		dataShards:   DataShards,
		parityShards: ParityShards,
		shardSize:    ShardSize,
	}
	f.initOnce.Do(func() {
		f.useAVX512 = C.fec_has_avx512() != 0
		if f.useAVX512 {
			log.Println("[FEC] AVX-512 加速已启用")
		} else {
			log.Println("[FEC] AVX-512 不可用，使用 C 标量实现")
		}
	})
	return f
}

// Encode 编码数据（生成冗余分片）
// 输入：原始数据
// 输出：数据分片 + 冗余分片
func (f *FECProcessor) Encode(data []byte) ([][]byte, error) {
	// 1. 计算需要的分片数
	totalSize := len(data)
	paddedSize := ((totalSize + f.dataShards - 1) / f.dataShards) * f.dataShards

	// 2. 填充数据到完整分片（连续内存，直接传给 C）
	paddedData := make([]byte, paddedSize)
	copy(paddedData, data)

	// 3. 分割为数据分片
	shardSize := paddedSize / f.dataShards
	dataShards := make([][]byte, f.dataShards)
	for i := 0; i < f.dataShards; i++ {
		dataShards[i] = paddedData[i*shardSize : (i+1)*shardSize]
	}

	// 4. 生成冗余分片（调用 C AVX-512 加速）
	parityBuf := make([]byte, f.parityShards*shardSize)

	// Pin 内存：防止 GC 在 CGO 执行期间移动底层数组
	var pinner runtime.Pinner
	pinner.Pin(&paddedData[0])
	pinner.Pin(&parityBuf[0])

	ret := C.fec_encode(
		(*C.uint8_t)(unsafe.Pointer(&paddedData[0])),
		(*C.uint8_t)(unsafe.Pointer(&parityBuf[0])),
		C.int(f.dataShards),
		C.int(f.parityShards),
		C.int(shardSize),
	)

	pinner.Unpin()

	if ret != 0 {
		return nil, fmt.Errorf("C fec_encode 失败: %d", ret)
	}

	// 5. 切分 parity 连续内存为独立切片
	parityShards := make([][]byte, f.parityShards)
	for i := 0; i < f.parityShards; i++ {
		parityShards[i] = make([]byte, shardSize)
		copy(parityShards[i], parityBuf[i*shardSize:(i+1)*shardSize])
	}

	// 6. 合并所有分片
	allShards := make([][]byte, f.dataShards+f.parityShards)
	copy(allShards[:f.dataShards], dataShards)
	copy(allShards[f.dataShards:], parityShards)

	return allShards, nil
}

// Decode 解码数据（恢复丢失分片）
// 输入：接收到的分片（可能不完整）+ 对应的原始索引
// 输出：恢复的原始数据
func (f *FECProcessor) Decode(shards [][]byte, indices []int) ([]byte, error) {
	if len(shards) < f.dataShards {
		return nil, fmt.Errorf("分片数不足：需要 %d，实际 %d", f.dataShards, len(shards))
	}

	if len(shards[0]) == 0 {
		return nil, fmt.Errorf("分片数据为空")
	}

	shardSize := len(shards[0])

	// 快速路径：如果前 dataShards 个分片都是数据分片且索引连续，直接拼接
	// 零 CGO 开销，零纠错计算
	allData := true
	for i := 0; i < f.dataShards && i < len(indices); i++ {
		if indices[i] != i {
			allData = false
			break
		}
	}
	if allData && len(shards) >= f.dataShards {
		result := make([]byte, shardSize*f.dataShards)
		for i := 0; i < f.dataShards; i++ {
			copy(result[i*shardSize:], shards[i])
		}
		return result, nil
	}

	// 需要 Reed-Solomon 解码：准备连续内存跨 CGO 边界
	availableCount := f.dataShards
	if len(shards) < availableCount {
		availableCount = len(shards)
	}

	// 聚合分散的 slice 到连续内存（一次性跨 CGO 边界）
	shardsBuf := make([]byte, availableCount*shardSize)
	for i := 0; i < availableCount; i++ {
		copy(shardsBuf[i*shardSize:], shards[i])
	}

	indicesBuf := make([]int32, availableCount)
	for i := 0; i < availableCount; i++ {
		indicesBuf[i] = int32(indices[i])
	}

	recoveredBuf := make([]byte, f.dataShards*shardSize)

	// Pin 所有传给 C 的内存
	var pinner runtime.Pinner
	pinner.Pin(&shardsBuf[0])
	pinner.Pin(&indicesBuf[0])
	pinner.Pin(&recoveredBuf[0])

	ret := C.fec_decode(
		(*C.uint8_t)(unsafe.Pointer(&shardsBuf[0])),
		(*C.int)(unsafe.Pointer(&indicesBuf[0])),
		C.int(availableCount),
		(*C.uint8_t)(unsafe.Pointer(&recoveredBuf[0])),
		C.int(f.dataShards),
		C.int(f.parityShards),
		C.int(shardSize),
	)

	pinner.Unpin()

	if ret != 0 {
		return nil, fmt.Errorf("C fec_decode 失败: %d", ret)
	}

	return recoveredBuf, nil
}

// EncodeBatch 批量编码多个数据包（摊薄 CGO 上下文切换开销）
// 适用于小包场景（DNS Tunnel / ICMP），将多个包聚合后一次性跨 CGO 边界
func (f *FECProcessor) EncodeBatch(packets [][]byte) ([][]*Shard, error) {
	if len(packets) == 0 {
		return nil, nil
	}

	results := make([][]*Shard, len(packets))

	// 小包聚合策略：如果单包 < batchThreshold，逐个编码仍然走 C
	// 但由于每个包的 shardSize 可能不同，无法真正合并到一次 C 调用
	// 真正的收益在于：调用者在上层攒够一批再调用，而非每收一个包就调一次
	for i, pkt := range packets {
		shards, err := f.EncodePacket(pkt)
		if err != nil {
			return nil, fmt.Errorf("编码包 %d 失败: %w", i, err)
		}
		results[i] = shards
	}

	return results, nil
}

// EncodePacket 编码单个数据包
func (f *FECProcessor) EncodePacket(packet []byte) ([]*Shard, error) {
	shards, err := f.Encode(packet)
	if err != nil {
		return nil, err
	}

	result := make([]*Shard, len(shards))
	for i, data := range shards {
		result[i] = &Shard{
			Index:    i,
			Data:     data,
			IsParity: i >= f.dataShards,
		}
	}

	return result, nil
}

// DecodePacket 解码数据包
func (f *FECProcessor) DecodePacket(shards []*Shard) ([]byte, error) {
	if len(shards) < f.dataShards {
		return nil, fmt.Errorf("分片不足")
	}

	// 提取分片数据和索引
	shardData := make([][]byte, len(shards))
	indices := make([]int, len(shards))

	for i, shard := range shards {
		shardData[i] = shard.Data
		indices[i] = shard.Index
	}

	return f.Decode(shardData, indices)
}

// Shard 分片结构
type Shard struct {
	Index    int    // 分片索引
	Data     []byte // 分片数据
	IsParity bool   // 是否为冗余分片
}

// ShardHeader 分片头（用于网络传输）
type ShardHeader struct {
	PacketID    uint64 // 数据包 ID
	ShardID     uint8  // 分片 ID
	TotalData   uint8  // 数据分片总数
	TotalParity uint8  // 冗余分片总数
	IsParity    uint8  // 是否为冗余分片
	DataSize    uint16 // 数据大小
	Epoch       uint32 // 纪元标识，路径切换时递增
	Reserved    uint16 // 保留字段
}

// SerializeShard 序列化分片（用于网络传输）
func SerializeShard(shard *Shard, packetID uint64) []byte {
	return SerializeShardWithEpoch(shard, packetID, 0)
}

// SerializeShardWithEpoch 序列化分片（带 Epoch 标识）
func SerializeShardWithEpoch(shard *Shard, packetID uint64, epoch uint32) []byte {
	header := ShardHeader{
		PacketID:    packetID,
		ShardID:     uint8(shard.Index),
		TotalData:   DataShards,
		TotalParity: ParityShards,
		IsParity:    0,
		DataSize:    uint16(len(shard.Data)),
		Epoch:       epoch,
	}

	if shard.IsParity {
		header.IsParity = 1
	}

	// 序列化头部 + 数据
	headerSize := int(unsafe.Sizeof(header))
	result := make([]byte, headerSize+len(shard.Data))
	*(*ShardHeader)(unsafe.Pointer(&result[0])) = header
	copy(result[headerSize:], shard.Data)

	return result
}

// DeserializeShard 反序列化分片
func DeserializeShard(data []byte) (*Shard, uint64, error) {
	headerSize := int(unsafe.Sizeof(ShardHeader{}))
	if len(data) < headerSize {
		return nil, 0, fmt.Errorf("数据太短")
	}

	header := (*ShardHeader)(unsafe.Pointer(&data[0]))

	shardData := make([]byte, len(data)-headerSize)
	copy(shardData, data[headerSize:])

	shard := &Shard{
		Index:    int(header.ShardID),
		Data:     shardData,
		IsParity: header.IsParity == 1,
	}

	return shard, header.PacketID, nil
}

// DeserializeShardWithEpoch 反序列化分片（返回 Epoch）
func DeserializeShardWithEpoch(data []byte) (*Shard, uint64, uint32, error) {
	headerSize := int(unsafe.Sizeof(ShardHeader{}))
	if len(data) < headerSize {
		return nil, 0, 0, fmt.Errorf("数据太短")
	}

	header := (*ShardHeader)(unsafe.Pointer(&data[0]))

	shardData := make([]byte, len(data)-headerSize)
	copy(shardData, data[headerSize:])

	shard := &Shard{
		Index:    int(header.ShardID),
		Data:     shardData,
		IsParity: header.IsParity == 1,
	}

	return shard, header.PacketID, header.Epoch, nil
}
