// Package gtunnel - 0-RTT 伪装器
// 模拟浏览器预连接特征，注入假 Early Data
package gtunnel

import (
	"crypto/rand"
	"log"
	"sync"
	"time"
)

// ZeroRTTObfuscator 0-RTT 伪装器
type ZeroRTTObfuscator struct {
	mu sync.RWMutex

	// 配置
	enabled           bool
	earlyDataSize     int    // 假 Early Data 大小
	preconnectDelay   time.Duration // 预连接延迟

	// 区域配置
	region            string
	
	// 模板
	earlyDataTemplates map[string][]byte

	// 统计
	stats ZeroRTTStats
}

// ZeroRTTStats 统计
type ZeroRTTStats struct {
	EarlyDataInjections int64
	PreconnectSimulated int64
	BytesInjected       int64
}

// NewZeroRTTObfuscator 创建 0-RTT 伪装器
func NewZeroRTTObfuscator(region string) *ZeroRTTObfuscator {
	o := &ZeroRTTObfuscator{
		enabled:            true,
		earlyDataSize:      256,
		preconnectDelay:    50 * time.Millisecond,
		region:             region,
		earlyDataTemplates: make(map[string][]byte),
	}
	o.initTemplates()
	return o
}

// initTemplates 初始化 Early Data 模板
func (o *ZeroRTTObfuscator) initTemplates() {
	// 模拟不同服务的 Early Data 特征

	// YouTube - 视频预加载请求
	o.earlyDataTemplates["youtube"] = o.buildEarlyDataTemplate(
		"GET /api/stats/playback HTTP/1.1\r\n"+
			"Host: www.youtube.com\r\n"+
			"Accept: */*\r\n"+
			"X-YouTube-Client-Name: 1\r\n"+
			"X-YouTube-Client-Version: 2.20240101\r\n\r\n",
	)

	// Netflix - 流媒体预热
	o.earlyDataTemplates["netflix"] = o.buildEarlyDataTemplate(
		"GET /api/shakti/mre HTTP/1.1\r\n"+
			"Host: www.netflix.com\r\n"+
			"Accept: application/json\r\n"+
			"X-Netflix-Request-Id: 0\r\n\r\n",
	)

	// Zoom - 会议预连接
	o.earlyDataTemplates["zoom"] = o.buildEarlyDataTemplate(
		"GET /wc/ping HTTP/1.1\r\n"+
			"Host: zoom.us\r\n"+
			"Accept: */*\r\n"+
			"X-ZM-TRACKINGID: 0\r\n\r\n",
	)

	// Spotify - 音频预加载
	o.earlyDataTemplates["spotify"] = o.buildEarlyDataTemplate(
		"GET /v1/me/player HTTP/1.1\r\n"+
			"Host: api.spotify.com\r\n"+
			"Accept: application/json\r\n"+
			"Authorization: Bearer 0\r\n\r\n",
	)

	// 通用 HTTPS
	o.earlyDataTemplates["https"] = o.buildEarlyDataTemplate(
		"GET / HTTP/1.1\r\n"+
			"Host: example.com\r\n"+
			"Accept: */*\r\n"+
			"Connection: keep-alive\r\n\r\n",
	)
}

// buildEarlyDataTemplate 构建 Early Data 模板
func (o *ZeroRTTObfuscator) buildEarlyDataTemplate(httpRequest string) []byte {
	// 填充到目标大小
	template := []byte(httpRequest)
	if len(template) < o.earlyDataSize {
		padding := make([]byte, o.earlyDataSize-len(template))
		rand.Read(padding)
		template = append(template, padding...)
	}
	return template[:o.earlyDataSize]
}

// GenerateEarlyData 生成假 Early Data
func (o *ZeroRTTObfuscator) GenerateEarlyData(service string) []byte {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.enabled {
		return nil
	}

	// 选择模板
	template, ok := o.earlyDataTemplates[service]
	if !ok {
		template = o.earlyDataTemplates["https"]
	}

	// 复制并添加随机性
	earlyData := make([]byte, len(template))
	copy(earlyData, template)

	// 在末尾添加随机字节，增加熵
	randomSuffix := make([]byte, 32)
	rand.Read(randomSuffix)
	if len(earlyData) > 32 {
		copy(earlyData[len(earlyData)-32:], randomSuffix)
	}

	o.stats.EarlyDataInjections++
	o.stats.BytesInjected += int64(len(earlyData))

	return earlyData
}

// WrapInitialWithEarlyData 在 Initial 包后注入 Early Data
func (o *ZeroRTTObfuscator) WrapInitialWithEarlyData(initialPacket []byte, service string) []byte {
	earlyData := o.GenerateEarlyData(service)
	if earlyData == nil {
		return initialPacket
	}

	// 构建 0-RTT 包
	zeroRTTPacket := o.buildZeroRTTPacket(earlyData)

	// 合并 Initial + 0-RTT
	combined := make([]byte, 0, len(initialPacket)+len(zeroRTTPacket))
	combined = append(combined, initialPacket...)
	combined = append(combined, zeroRTTPacket...)

	log.Printf("🚀 [0-RTT] 注入 Early Data: %d bytes (service: %s)", len(earlyData), service)

	return combined
}

// buildZeroRTTPacket 构建 0-RTT 包
func (o *ZeroRTTObfuscator) buildZeroRTTPacket(earlyData []byte) []byte {
	// QUIC 0-RTT 包格式（简化）
	// Header Form (1) = 1 (Long Header)
	// Fixed Bit (1) = 1
	// Long Packet Type (2) = 01 (0-RTT)
	// Reserved Bits (2) = 00
	// Packet Number Length (2) = 00 (1 byte)

	packet := make([]byte, 0, len(earlyData)+32)

	// Long Header: 1101 0000 = 0xD0 (0-RTT)
	packet = append(packet, 0xD0)

	// Version: QUIC v1 (0x00000001)
	packet = append(packet, 0x00, 0x00, 0x00, 0x01)

	// DCID Length + DCID (8 bytes)
	dcid := make([]byte, 8)
	rand.Read(dcid)
	packet = append(packet, 0x08)
	packet = append(packet, dcid...)

	// SCID Length + SCID (0 bytes)
	packet = append(packet, 0x00)

	// Packet Number (1 byte)
	packet = append(packet, 0x00)

	// Payload (加密的 Early Data)
	// 实际应该加密，这里简化处理
	packet = append(packet, earlyData...)

	return packet
}

// SimulatePreconnect 模拟浏览器预连接
func (o *ZeroRTTObfuscator) SimulatePreconnect(service string) []byte {
	o.mu.Lock()
	o.stats.PreconnectSimulated++
	o.mu.Unlock()

	// 模拟预连接延迟
	time.Sleep(o.preconnectDelay)

	// 生成预连接 Early Data
	return o.GenerateEarlyData(service)
}

// SetEnabled 设置启用状态
func (o *ZeroRTTObfuscator) SetEnabled(enabled bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enabled = enabled
}

// SetEarlyDataSize 设置 Early Data 大小
func (o *ZeroRTTObfuscator) SetEarlyDataSize(size int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.earlyDataSize = size
	o.initTemplates() // 重新初始化模板
}

// SetRegion 设置区域
func (o *ZeroRTTObfuscator) SetRegion(region string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.region = region
}

// GetStats 获取统计
func (o *ZeroRTTObfuscator) GetStats() ZeroRTTStats {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.stats
}

// GetServiceForRegion 根据区域获取推荐服务
func (o *ZeroRTTObfuscator) GetServiceForRegion() string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	switch o.region {
	case "asia_pacific":
		return "youtube"
	case "europe":
		return "spotify"
	case "north_america":
		return "netflix"
	case "middle_east":
		return "https"
	case "china":
		return "https"
	default:
		return "https"
	}
}
