// Package cortex eBPF 同步器
// 将高危指纹同步到 eBPF Map 实现 O(1) 查询
package cortex

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/cilium/ebpf"
)

// Syncer 高危指纹同步器
type Syncer struct {
	mu sync.RWMutex

	// eBPF Maps
	highRiskMap *ebpf.Map // hash -> threat_score
	trustedMap  *ebpf.Map // hash -> 1

	// 分析器引用
	analyzer *Analyzer

	// 同步状态
	lastSyncCount int
}

// NewSyncer 创建同步器
func NewSyncer(analyzer *Analyzer) *Syncer {
	return &Syncer{
		analyzer: analyzer,
	}
}

// SetMaps 设置 eBPF Maps
func (s *Syncer) SetMaps(highRisk, trusted *ebpf.Map) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.highRiskMap = highRisk
	s.trustedMap = trusted
}

// Sync 同步高危指纹到 eBPF
func (s *Syncer) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.highRiskMap == nil {
		return fmt.Errorf("high_risk_map not set")
	}

	// 获取所有高危指纹
	hashes := s.analyzer.GetHighRiskHashes()

	// 批量写入 eBPF Map
	for _, hash := range hashes {
		fp := s.analyzer.GetFingerprint(hash)
		if fp == nil {
			continue
		}

		// 将 hash 前 8 字节作为 key
		key := hashToKey(hash)
		value := uint32(fp.ThreatScore)

		if err := s.highRiskMap.Put(key, value); err != nil {
			return fmt.Errorf("failed to put high_risk_map: %w", err)
		}
	}

	s.lastSyncCount = len(hashes)
	return nil
}

// SyncTrusted 同步白名单指纹
func (s *Syncer) SyncTrusted(trustedHashes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.trustedMap == nil {
		return fmt.Errorf("trusted_map not set")
	}

	for _, hash := range trustedHashes {
		key := hashToKey(hash)
		value := uint32(1)

		if err := s.trustedMap.Put(key, value); err != nil {
			return fmt.Errorf("failed to put trusted_map: %w", err)
		}
	}

	return nil
}

// RemoveFromHighRisk 从高危列表移除
func (s *Syncer) RemoveFromHighRisk(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.highRiskMap == nil {
		return nil
	}

	key := hashToKey(hash)
	return s.highRiskMap.Delete(key)
}

// GetSyncCount 获取同步数量
func (s *Syncer) GetSyncCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncCount
}

// hashToKey 将 hash 字符串转换为 eBPF Map key
func hashToKey(hash string) uint64 {
	if len(hash) < 16 {
		return 0
	}
	// 取 hash 前 16 个字符（8 字节 hex）
	var key uint64
	for i := 0; i < 8 && i*2+1 < len(hash); i++ {
		b := hexToByte(hash[i*2], hash[i*2+1])
		key |= uint64(b) << (56 - i*8)
	}
	return key
}

func hexToByte(h, l byte) byte {
	return hexVal(h)<<4 | hexVal(l)
}

func hexVal(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	}
	return 0
}

// KeyToBytes 将 key 转换为字节数组（用于 eBPF）
func KeyToBytes(key uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, key)
	return b
}
