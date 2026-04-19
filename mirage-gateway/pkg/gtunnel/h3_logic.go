// Package gtunnel - H3 协议逻辑控制器 (Go 控制面)
// 负责握手构造、CID 策略、eBPF Map 下发
package gtunnel

import (
	"crypto/rand"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/cilium/ebpf"
)

// H3LogicController H3 逻辑控制器
type H3LogicController struct {
	mu sync.RWMutex

	// 区域配置
	region string

	// CID 管理
	cidManager *CIDRotationManager

	// H3 帧生成器
	framer *H3FrameGenerator

	// 0-RTT 伪装器
	zeroRTT *ZeroRTTObfuscator

	// eBPF Maps
	cidRotationMap  *ebpf.Map // CID 轮换映射
	h3ConfigMap     *ebpf.Map // H3 配置映射
	noiseConfigMap  *ebpf.Map // 噪声配置映射

	// 重连模拟
	reconnectInterval time.Duration
	lastReconnect     time.Time

	// 统计
	stats H3LogicStats

	// 停止信号
	stopChan chan struct{}
}

// H3LogicStats 统计
type H3LogicStats struct {
	HandshakesGenerated int64
	CIDRotations        int64
	ReconnectSimulated  int64
	MapUpdates          int64
}

// CIDMapEntry eBPF CID Map 条目（支持双 CID 静默期）
type CIDMapEntry struct {
	ActiveCID      [8]byte
	GracefulCID    [8]byte
	GracefulExpire uint64 // 纳秒时间戳
	NoiseRate      uint8
	GracefulEnabled uint8
	Padding        [6]byte
}

// H3ConfigEntry eBPF H3 配置条目
type H3ConfigEntry struct {
	FrameType     uint8
	MimicryType   uint8  // 0=YouTube, 1=Netflix, 2=Zoom
	PaddingMin    uint16
	PaddingMax    uint16
	MTUTarget     uint16
	DeepMimicry   uint8  // 前 N 包深度拟态
	Reserved      uint8
}

// NoiseConfigEntry eBPF 噪声配置条目
type NoiseConfigEntry struct {
	Enabled     uint8
	Rate        uint8  // 0-100
	MinSize     uint16
	MaxSize     uint16
	Reserved    [2]byte
}

// NewH3LogicController 创建 H3 逻辑控制器
func NewH3LogicController(region string) *H3LogicController {
	ctrl := &H3LogicController{
		region:            region,
		cidManager:        NewCIDRotationManager(nil),
		framer:            NewH3FrameGenerator(region, ""),
		zeroRTT:           NewZeroRTTObfuscator(region),
		reconnectInterval: 7 * time.Minute, // 5-10 分钟随机
		lastReconnect:     time.Now(),
		stopChan:          make(chan struct{}),
	}

	// 设置 CID 变更回调
	ctrl.cidManager.SetOnCIDChange(ctrl.onCIDChange)

	return ctrl
}

// SetEBPFMaps 设置 eBPF Maps
func (c *H3LogicController) SetEBPFMaps(cidMap, h3Map, noiseMap *ebpf.Map) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cidRotationMap = cidMap
	c.h3ConfigMap = h3Map
	c.noiseConfigMap = noiseMap

	log.Printf("🔗 [H3-Logic] eBPF Maps 已绑定")
}

// Start 启动控制器
func (c *H3LogicController) Start() {
	go c.runReconnectSimulator()
	go c.runCIDRotationMonitor()
	log.Printf("🚀 [H3-Logic] 控制器已启动 (region: %s)", c.region)
}

// Stop 停止控制器
func (c *H3LogicController) Stop() {
	close(c.stopChan)
}

// runReconnectSimulator 运行重连模拟器
func (c *H3LogicController) runReconnectSimulator() {
	for {
		// 随机间隔 5-10 分钟
		randVal, _ := rand.Int(rand.Reader, big.NewInt(int64(5*time.Minute)))
		interval := 5*time.Minute + time.Duration(randVal.Int64())

		select {
		case <-c.stopChan:
			return
		case <-time.After(interval):
			c.simulateReconnect()
		}
	}
}

// runCIDRotationMonitor 运行 CID 轮换监控
func (c *H3LogicController) runCIDRotationMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.checkCIDRotation()
		}
	}
}

// simulateReconnect 模拟断连重连
func (c *H3LogicController) simulateReconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. 强制 CID 轮换
	newCID := c.cidManager.ForceRotate()

	// 2. 重置 H3 帧计数（触发深度拟态）
	c.framer.ResetFrameCount()

	// 3. 更新 eBPF Map
	c.updateCIDMap(newCID)

	c.stats.ReconnectSimulated++
	c.lastReconnect = time.Now()

	log.Printf("🔄 [H3-Logic] 模拟重连: 新 CID 已下发, 深度拟态已重置")
}

// checkCIDRotation 检查 CID 轮换
func (c *H3LogicController) checkCIDRotation() {
	// 由 CIDRotationManager 内部处理
	// 这里只做额外的策略检查
}

// onCIDChange CID 变更回调
func (c *H3LogicController) onCIDChange(oldCID, newCID []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 使用带静默期的更新
	c.updateCIDMapWithGraceful(oldCID, newCID)
	c.stats.CIDRotations++
}

// updateCIDMap 更新 CID Map
func (c *H3LogicController) updateCIDMap(newCID []byte) {
	if c.cidRotationMap == nil {
		return
	}

	key := uint32(0)
	entry := CIDMapEntry{
		NoiseRate:       3, // 3% 噪声率
		GracefulEnabled: 0,
	}
	copy(entry.ActiveCID[:], newCID)

	if err := c.cidRotationMap.Put(key, entry); err != nil {
		log.Printf("⚠️ [H3-Logic] CID Map 更新失败: %v", err)
		return
	}

	c.stats.MapUpdates++
}

// updateCIDMapWithGraceful 更新 CID Map（带静默期）
func (c *H3LogicController) updateCIDMapWithGraceful(oldCID, newCID []byte) {
	if c.cidRotationMap == nil {
		return
	}

	key := uint32(0)
	
	// 静默期 300ms
	gracefulExpire := uint64(time.Now().UnixNano()) + 300_000_000

	entry := CIDMapEntry{
		GracefulExpire:  gracefulExpire,
		NoiseRate:       3,
		GracefulEnabled: 1,
	}
	copy(entry.ActiveCID[:], newCID)
	copy(entry.GracefulCID[:], oldCID)

	if err := c.cidRotationMap.Put(key, entry); err != nil {
		log.Printf("⚠️ [H3-Logic] CID Map 更新失败: %v", err)
		return
	}

	c.stats.MapUpdates++
	log.Printf("🔄 [H3-Logic] CID 轮换: 静默期 300ms 已启用")
}

// UpdateH3Config 更新 H3 配置到 eBPF
func (c *H3LogicController) UpdateH3Config(mimicryType uint8, deepMimicryPackets uint8) {
	if c.h3ConfigMap == nil {
		return
	}

	key := uint32(0)
	entry := H3ConfigEntry{
		FrameType:   0, // DATA
		MimicryType: mimicryType,
		PaddingMin:  64,
		PaddingMax:  256,
		MTUTarget:   1200, // QUIC 标准 MTU
		DeepMimicry: deepMimicryPackets,
	}

	if err := c.h3ConfigMap.Put(key, entry); err != nil {
		log.Printf("⚠️ [H3-Logic] H3 Config Map 更新失败: %v", err)
		return
	}

	c.stats.MapUpdates++
}

// UpdateNoiseConfig 更新噪声配置到 eBPF
func (c *H3LogicController) UpdateNoiseConfig(enabled bool, rate uint8) {
	if c.noiseConfigMap == nil {
		return
	}

	key := uint32(0)
	entry := NoiseConfigEntry{
		Rate:    rate,
		MinSize: 32,
		MaxSize: 128,
	}
	if enabled {
		entry.Enabled = 1
	}

	if err := c.noiseConfigMap.Put(key, entry); err != nil {
		log.Printf("⚠️ [H3-Logic] Noise Config Map 更新失败: %v", err)
		return
	}

	c.stats.MapUpdates++
}

// GenerateInitialPacket 生成 QUIC Initial 包
func (c *H3LogicController) GenerateInitialPacket(dcid, scid []byte) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	// QUIC Initial 包格式
	packet := make([]byte, 0, 1200)

	// Long Header: 1100 0000 = 0xC0 (Initial)
	packet = append(packet, 0xC0)

	// Version: QUIC v1
	packet = append(packet, 0x00, 0x00, 0x00, 0x01)

	// DCID Length + DCID
	packet = append(packet, byte(len(dcid)))
	packet = append(packet, dcid...)

	// SCID Length + SCID
	packet = append(packet, byte(len(scid)))
	packet = append(packet, scid...)

	// Token Length (0)
	packet = append(packet, 0x00)

	// Length (varint) - 占位
	lengthPos := len(packet)
	packet = append(packet, 0x40, 0x00) // 2 字节 varint 占位

	// Packet Number (1 byte)
	packet = append(packet, 0x00)

	// Crypto Frame (SETTINGS)
	cryptoFrame := c.buildCryptoFrame()
	packet = append(packet, cryptoFrame...)

	// 填充到 1200 字节
	if len(packet) < 1200 {
		padding := make([]byte, 1200-len(packet))
		packet = append(packet, padding...)
	}

	// 更新 Length 字段
	payloadLen := len(packet) - lengthPos - 2
	packet[lengthPos] = byte(0x40 | (payloadLen >> 8))
	packet[lengthPos+1] = byte(payloadLen)

	c.stats.HandshakesGenerated++

	return packet
}

// buildCryptoFrame 构建 Crypto 帧
func (c *H3LogicController) buildCryptoFrame() []byte {
	frame := make([]byte, 0, 256)

	// Frame Type: CRYPTO (0x06)
	frame = append(frame, 0x06)

	// Offset (varint): 0
	frame = append(frame, 0x00)

	// TLS ClientHello (简化)
	clientHello := c.buildTLSClientHello()

	// Length (varint)
	frame = appendVarint(frame, uint64(len(clientHello)))

	// Data
	frame = append(frame, clientHello...)

	return frame
}

// buildTLSClientHello 构建 TLS ClientHello
func (c *H3LogicController) buildTLSClientHello() []byte {
	hello := make([]byte, 0, 512)

	// Handshake Type: ClientHello (0x01)
	hello = append(hello, 0x01)

	// Length 占位
	lengthPos := len(hello)
	hello = append(hello, 0x00, 0x00, 0x00)

	// Version: TLS 1.2 (兼容)
	hello = append(hello, 0x03, 0x03)

	// Random (32 bytes)
	random := make([]byte, 32)
	rand.Read(random)
	hello = append(hello, random...)

	// Session ID Length (0)
	hello = append(hello, 0x00)

	// Cipher Suites
	cipherSuites := []uint16{
		0x1301, // TLS_AES_128_GCM_SHA256
		0x1302, // TLS_AES_256_GCM_SHA384
		0x1303, // TLS_CHACHA20_POLY1305_SHA256
	}
	hello = append(hello, byte(len(cipherSuites)*2>>8), byte(len(cipherSuites)*2))
	for _, suite := range cipherSuites {
		hello = append(hello, byte(suite>>8), byte(suite))
	}

	// Compression Methods
	hello = append(hello, 0x01, 0x00)

	// Extensions
	extensions := c.buildTLSExtensions()
	hello = append(hello, byte(len(extensions)>>8), byte(len(extensions)))
	hello = append(hello, extensions...)

	// 更新 Length
	length := len(hello) - lengthPos - 3
	hello[lengthPos] = byte(length >> 16)
	hello[lengthPos+1] = byte(length >> 8)
	hello[lengthPos+2] = byte(length)

	return hello
}

// buildTLSExtensions 构建 TLS 扩展
func (c *H3LogicController) buildTLSExtensions() []byte {
	ext := make([]byte, 0, 256)

	// Supported Versions (0x002b)
	ext = append(ext, 0x00, 0x2b, 0x00, 0x03, 0x02, 0x03, 0x04)

	// Key Share (0x0033) - 简化
	ext = append(ext, 0x00, 0x33, 0x00, 0x26, 0x00, 0x24)
	ext = append(ext, 0x00, 0x1d, 0x00, 0x20) // x25519
	keyShare := make([]byte, 32)
	rand.Read(keyShare)
	ext = append(ext, keyShare...)

	// ALPN (0x0010) - h3
	ext = append(ext, 0x00, 0x10, 0x00, 0x05, 0x00, 0x03, 0x02)
	ext = append(ext, 'h', '3')

	// QUIC Transport Parameters (0x0039)
	ext = append(ext, 0x00, 0x39)
	params := c.buildQUICTransportParams()
	ext = append(ext, byte(len(params)>>8), byte(len(params)))
	ext = append(ext, params...)

	return ext
}

// buildQUICTransportParams 构建 QUIC 传输参数
func (c *H3LogicController) buildQUICTransportParams() []byte {
	params := make([]byte, 0, 64)

	// initial_max_data (0x04)
	params = append(params, 0x04, 0x04)
	params = appendVarint(params, 1048576) // 1MB

	// initial_max_stream_data_bidi_local (0x05)
	params = append(params, 0x05, 0x04)
	params = appendVarint(params, 262144) // 256KB

	// initial_max_streams_bidi (0x08)
	params = append(params, 0x08, 0x02)
	params = appendVarint(params, 100)

	// max_idle_timeout (0x01)
	params = append(params, 0x01, 0x04)
	params = appendVarint(params, 30000) // 30s

	return params
}

// SetRegion 设置区域
func (c *H3LogicController) SetRegion(region string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.region = region
	c.framer.SetRegion(region)
	c.zeroRTT.SetRegion(region)

	// 根据区域更新拟态类型
	var mimicryType uint8
	switch region {
	case "asia_pacific":
		mimicryType = 0 // YouTube
	case "europe":
		mimicryType = 3 // Spotify
	case "north_america":
		mimicryType = 1 // Netflix
	default:
		mimicryType = 2 // Zoom
	}

	c.UpdateH3Config(mimicryType, 20)
}

// GetStats 获取统计
func (c *H3LogicController) GetStats() H3LogicStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// GetCIDManager 获取 CID 管理器
func (c *H3LogicController) GetCIDManager() *CIDRotationManager {
	return c.cidManager
}

// GetFramer 获取 H3 帧生成器
func (c *H3LogicController) GetFramer() *H3FrameGenerator {
	return c.framer
}

// GetZeroRTT 获取 0-RTT 伪装器
func (c *H3LogicController) GetZeroRTT() *ZeroRTTObfuscator {
	return c.zeroRTT
}
