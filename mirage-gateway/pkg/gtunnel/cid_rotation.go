// Package gtunnel - CID 轮换管理器
// 动态 Connection ID 轮换，打破流量连续性统计
package gtunnel

import (
	"crypto/rand"
	"log"
	"sync"
	"time"
)

// CIDRotationManager CID 轮换管理器
type CIDRotationManager struct {
	mu sync.RWMutex

	// CID 池
	cidPool     [][]byte
	poolSize    int
	cidLength   int

	// 当前 CID
	currentCID  []byte
	currentIdx  int

	// 轮换策略
	rotateAfterPackets uint64  // 每 N 包轮换
	rotateAfterTime    time.Duration // 每 N 时间轮换
	rotateOnPathSwitch bool    // 路径切换时轮换

	// 计数器
	packetCount uint64
	lastRotate  time.Time

	// CID 映射同步回调（通知 eBPF）
	onCIDChange func(oldCID, newCID []byte)

	// 统计
	stats CIDRotationStats
}

// CIDRotationStats 统计
type CIDRotationStats struct {
	TotalRotations   int64
	PacketRotations  int64
	TimeRotations    int64
	PathRotations    int64
	CurrentCIDIndex  int
}

// CIDRotationConfig 配置
type CIDRotationConfig struct {
	PoolSize           int
	CIDLength          int
	RotateAfterPackets uint64
	RotateAfterTime    time.Duration
	RotateOnPathSwitch bool
}

// DefaultCIDRotationConfig 默认配置
func DefaultCIDRotationConfig() *CIDRotationConfig {
	return &CIDRotationConfig{
		PoolSize:           16,
		CIDLength:          8,
		RotateAfterPackets: 1000,
		RotateAfterTime:    5 * time.Minute,
		RotateOnPathSwitch: true,
	}
}

// NewCIDRotationManager 创建 CID 轮换管理器
func NewCIDRotationManager(config *CIDRotationConfig) *CIDRotationManager {
	if config == nil {
		config = DefaultCIDRotationConfig()
	}

	m := &CIDRotationManager{
		poolSize:           config.PoolSize,
		cidLength:          config.CIDLength,
		rotateAfterPackets: config.RotateAfterPackets,
		rotateAfterTime:    config.RotateAfterTime,
		rotateOnPathSwitch: config.RotateOnPathSwitch,
		lastRotate:         time.Now(),
	}

	m.initCIDPool()
	return m
}

// initCIDPool 初始化 CID 池
func (m *CIDRotationManager) initCIDPool() {
	m.cidPool = make([][]byte, m.poolSize)

	for i := 0; i < m.poolSize; i++ {
		cid := make([]byte, m.cidLength)
		rand.Read(cid)
		m.cidPool[i] = cid
	}

	m.currentCID = m.cidPool[0]
	m.currentIdx = 0

	log.Printf("🔄 [CID-Rotation] 初始化 CID 池: %d 个, 长度 %d 字节", m.poolSize, m.cidLength)
}

// GetCurrentCID 获取当前 CID
func (m *CIDRotationManager) GetCurrentCID() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cid := make([]byte, len(m.currentCID))
	copy(cid, m.currentCID)
	return cid
}

// OnPacketSent 包发送后调用，检查是否需要轮换
func (m *CIDRotationManager) OnPacketSent() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.packetCount++

	// 检查包计数轮换
	if m.rotateAfterPackets > 0 && m.packetCount >= m.rotateAfterPackets {
		m.rotateCID("packet")
		m.packetCount = 0
		m.stats.PacketRotations++
		return true
	}

	// 检查时间轮换
	if m.rotateAfterTime > 0 && time.Since(m.lastRotate) >= m.rotateAfterTime {
		m.rotateCID("time")
		m.stats.TimeRotations++
		return true
	}

	return false
}

// OnPathSwitch 路径切换时调用
func (m *CIDRotationManager) OnPathSwitch(oldPath, newPath string) bool {
	if !m.rotateOnPathSwitch {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.rotateCID("path")
	m.stats.PathRotations++

	log.Printf("🔄 [CID-Rotation] 路径切换触发 CID 轮换: %s → %s", oldPath, newPath)
	return true
}

// rotateCID 执行 CID 轮换
func (m *CIDRotationManager) rotateCID(reason string) {
	oldCID := m.currentCID

	// 选择下一个 CID
	m.currentIdx = (m.currentIdx + 1) % m.poolSize
	m.currentCID = m.cidPool[m.currentIdx]
	m.lastRotate = time.Now()
	m.stats.TotalRotations++
	m.stats.CurrentCIDIndex = m.currentIdx

	// 通知回调
	if m.onCIDChange != nil {
		go m.onCIDChange(oldCID, m.currentCID)
	}

	log.Printf("🔄 [CID-Rotation] CID 轮换 (%s): idx=%d", reason, m.currentIdx)
}

// ForceRotate 强制轮换
func (m *CIDRotationManager) ForceRotate() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rotateCID("force")
	
	cid := make([]byte, len(m.currentCID))
	copy(cid, m.currentCID)
	return cid
}

// RefreshPool 刷新 CID 池
func (m *CIDRotationManager) RefreshPool() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := 0; i < m.poolSize; i++ {
		rand.Read(m.cidPool[i])
	}

	m.currentCID = m.cidPool[m.currentIdx]
	log.Printf("🔄 [CID-Rotation] CID 池已刷新")
}

// SetOnCIDChange 设置 CID 变更回调
func (m *CIDRotationManager) SetOnCIDChange(fn func(oldCID, newCID []byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onCIDChange = fn
}

// GetStats 获取统计
func (m *CIDRotationManager) GetStats() CIDRotationStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// BuildNewConnectionIDFrame 构建 NEW_CONNECTION_ID 帧
func (m *CIDRotationManager) BuildNewConnectionIDFrame(sequenceNum uint64) []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// QUIC NEW_CONNECTION_ID 帧格式:
	// Type (0x18) + Sequence Number + Retire Prior To + Length + Connection ID + Stateless Reset Token

	frame := make([]byte, 0, 64)

	// 帧类型: NEW_CONNECTION_ID (0x18)
	frame = append(frame, 0x18)

	// Sequence Number (varint)
	frame = appendVarint(frame, sequenceNum)

	// Retire Prior To (varint) - 通常为 0
	frame = appendVarint(frame, 0)

	// Connection ID Length
	frame = append(frame, byte(m.cidLength))

	// Connection ID
	frame = append(frame, m.currentCID...)

	// Stateless Reset Token (16 bytes)
	resetToken := make([]byte, 16)
	rand.Read(resetToken)
	frame = append(frame, resetToken...)

	return frame
}

// BuildRetireConnectionIDFrame 构建 RETIRE_CONNECTION_ID 帧
func (m *CIDRotationManager) BuildRetireConnectionIDFrame(sequenceNum uint64) []byte {
	// QUIC RETIRE_CONNECTION_ID 帧格式:
	// Type (0x19) + Sequence Number

	frame := make([]byte, 0, 16)

	// 帧类型: RETIRE_CONNECTION_ID (0x19)
	frame = append(frame, 0x19)

	// Sequence Number (varint)
	frame = appendVarint(frame, sequenceNum)

	return frame
}

// SimulateMobileHandoff 模拟移动端切换（Wi-Fi → 4G）
func (m *CIDRotationManager) SimulateMobileHandoff() (newCIDFrame, retireFrame []byte) {
	m.mu.Lock()
	oldIdx := m.currentIdx
	m.rotateCID("mobile_handoff")
	newIdx := m.currentIdx
	m.mu.Unlock()

	// 构建 NEW_CONNECTION_ID 帧
	newCIDFrame = m.BuildNewConnectionIDFrame(uint64(newIdx))

	// 构建 RETIRE_CONNECTION_ID 帧
	retireFrame = m.BuildRetireConnectionIDFrame(uint64(oldIdx))

	log.Printf("📱 [CID-Rotation] 模拟移动端切换: CID %d → %d", oldIdx, newIdx)

	return newCIDFrame, retireFrame
}

// GetCIDFromPool 从池中获取指定索引的 CID
func (m *CIDRotationManager) GetCIDFromPool(idx int) []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if idx < 0 || idx >= m.poolSize {
		return nil
	}

	cid := make([]byte, len(m.cidPool[idx]))
	copy(cid, m.cidPool[idx])
	return cid
}

// SetRotationPolicy 设置轮换策略
func (m *CIDRotationManager) SetRotationPolicy(packets uint64, duration time.Duration, onPathSwitch bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rotateAfterPackets = packets
	m.rotateAfterTime = duration
	m.rotateOnPathSwitch = onPathSwitch

	log.Printf("🔄 [CID-Rotation] 策略更新: packets=%d, time=%v, pathSwitch=%v",
		packets, duration, onPathSwitch)
}

// GetPoolSize 获取池大小
func (m *CIDRotationManager) GetPoolSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.poolSize
}

// GetCIDLength 获取 CID 长度
func (m *CIDRotationManager) GetCIDLength() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cidLength
}
