// Package gtunnel - H3 帧伪装器
// 将加密负载封装进伪造的 HTTP/3 帧结构
package gtunnel

import (
	"crypto/rand"
	"sync"
	"sync/atomic"
)

// 语义填充常量
const (
	MinPayloadConstraint    = 800  // 最小负载约束
	MaxPayloadConstraint    = 1200 // 最大负载约束
	SemanticPaddingFrames   = 5    // 前 N 个 DATA 帧强制语义填充
)

// H3FrameType HTTP/3 帧类型
type H3FrameType uint64

const (
	H3FrameDATA          H3FrameType = 0x00
	H3FrameHEADERS       H3FrameType = 0x01
	H3FrameCANCEL_PUSH   H3FrameType = 0x03
	H3FrameSETTINGS      H3FrameType = 0x04
	H3FramePUSH_PROMISE  H3FrameType = 0x05
	H3FrameGOAWAY        H3FrameType = 0x07
	H3FrameMAX_PUSH_ID   H3FrameType = 0x0D
	H3FramePRIORITY_UPDATE H3FrameType = 0x0F
)

// H3FrameGenerator H3 帧生成器
type H3FrameGenerator struct {
	mu sync.RWMutex

	// 区域配置
	Region        string
	TargetService string // YouTube, Zoom, Netflix 等

	// 帧计数
	frameCount    uint64
	dataFrameCount uint64 // DATA 帧计数（用于语义填充）
	deepMimicry   bool   // 深度拟态模式（前 20 包）
	
	// 区域 Headers 模板
	regionalHeaders map[string]*H3HeadersTemplate

	// 语义填充模板
	semanticPaddings [][]byte
}

// H3HeadersTemplate 区域 Headers 模板
type H3HeadersTemplate struct {
	Authority   string
	Path        string
	Method      string
	Scheme      string
	UserAgent   string
	Accept      string
	AcceptLang  string
	Origin      string
	Referer     string
}

// NewH3FrameGenerator 创建 H3 帧生成器
func NewH3FrameGenerator(region, targetService string) *H3FrameGenerator {
	g := &H3FrameGenerator{
		Region:          region,
		TargetService:   targetService,
		deepMimicry:     true,
		regionalHeaders: make(map[string]*H3HeadersTemplate),
	}
	g.initRegionalHeaders()
	g.initSemanticPaddings()
	return g
}

// initSemanticPaddings 初始化语义填充模板
func (g *H3FrameGenerator) initSemanticPaddings() {
	g.semanticPaddings = [][]byte{
		[]byte("cache-control: private, max-age=0\r\ncontent-type: video/mp4\r\n"),
		[]byte("content-type: application/octet-stream\r\ncontent-length: 1048576\r\n"),
		[]byte("x-content-type-options: nosniff\r\nstrict-transport-security: max-age=31536000\r\n"),
		[]byte("accept-ranges: bytes\r\ncontent-range: bytes 0-1048575/1048576\r\n"),
		[]byte("vary: Accept-Encoding\r\ncontent-encoding: gzip\r\n"),
		[]byte("etag: \"5f3b8c2d-100000\"\r\nlast-modified: Thu, 01 Jan 2025 00:00:00 GMT\r\n"),
		[]byte("x-served-by: cache-sin18034-SIN\r\nx-cache: HIT\r\n"),
		[]byte("alt-svc: h3=\":443\"; ma=86400\r\nserver: cloudflare\r\n"),
	}
}

// initRegionalHeaders 初始化区域 Headers 模板
func (g *H3FrameGenerator) initRegionalHeaders() {
	// 亚太区 - YouTube/Bilibili
	g.regionalHeaders["asia_pacific"] = &H3HeadersTemplate{
		Authority:  "www.youtube.com",
		Path:       "/watch",
		Method:     "GET",
		Scheme:     "https",
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Accept:     "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		AcceptLang: "en-US,en;q=0.9,zh-CN;q=0.8",
		Origin:     "https://www.youtube.com",
		Referer:    "https://www.youtube.com/",
	}

	// 欧洲区 - Spotify/BBC
	g.regionalHeaders["europe"] = &H3HeadersTemplate{
		Authority:  "open.spotify.com",
		Path:       "/track/",
		Method:     "GET",
		Scheme:     "https",
		UserAgent:  "Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
		Accept:     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLang: "en-GB,en;q=0.9,de;q=0.8",
		Origin:     "https://open.spotify.com",
		Referer:    "https://open.spotify.com/",
	}

	// 北美区 - Netflix/Zoom
	g.regionalHeaders["north_america"] = &H3HeadersTemplate{
		Authority:  "www.netflix.com",
		Path:       "/browse",
		Method:     "GET",
		Scheme:     "https",
		UserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Accept:     "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		AcceptLang: "en-US,en;q=0.9",
		Origin:     "https://www.netflix.com",
		Referer:    "https://www.netflix.com/",
	}

	// 中东区 - Telegram
	g.regionalHeaders["middle_east"] = &H3HeadersTemplate{
		Authority:  "web.telegram.org",
		Path:       "/k/",
		Method:     "GET",
		Scheme:     "https",
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Accept:     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLang: "ar,en;q=0.9",
		Origin:     "https://web.telegram.org",
		Referer:    "https://web.telegram.org/",
	}

	// 中国区 - WeChat/Bilibili
	g.regionalHeaders["china"] = &H3HeadersTemplate{
		Authority:  "www.bilibili.com",
		Path:       "/video/",
		Method:     "GET",
		Scheme:     "https",
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
		Accept:     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLang: "zh-CN,zh;q=0.9,en;q=0.8",
		Origin:     "https://www.bilibili.com",
		Referer:    "https://www.bilibili.com/",
	}

	// 全球默认
	g.regionalHeaders["global"] = &H3HeadersTemplate{
		Authority:  "www.google.com",
		Path:       "/search",
		Method:     "GET",
		Scheme:     "https",
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Accept:     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLang: "en-US,en;q=0.9",
		Origin:     "https://www.google.com",
		Referer:    "https://www.google.com/",
	}
}

// WrapPayload 封装负载为 H3 帧
func (g *H3FrameGenerator) WrapPayload(payload []byte) []byte {
	count := atomic.AddUint64(&g.frameCount, 1)

	// 前 20 包深度拟态
	if count <= 20 && g.deepMimicry {
		return g.wrapWithDeepMimicry(payload, count)
	}

	// 之后简单 DATA 帧封装
	return g.wrapAsDataFrame(payload)
}

// wrapWithDeepMimicry 深度拟态封装
func (g *H3FrameGenerator) wrapWithDeepMimicry(payload []byte, seq uint64) []byte {
	switch seq {
	case 1:
		// 第一包：SETTINGS 帧
		return g.buildSettingsFrame()
	case 2, 3:
		// 第二三包：HEADERS 帧
		return g.buildHeadersFrame(payload)
	case 4, 5, 6:
		// 第四五六包：PRIORITY_UPDATE
		return g.buildPriorityUpdateFrame(payload)
	default:
		// 其余：DATA 帧
		return g.wrapAsDataFrame(payload)
	}
}

// wrapAsDataFrame 封装为 DATA 帧
func (g *H3FrameGenerator) wrapAsDataFrame(payload []byte) []byte {
	dataCount := atomic.AddUint64(&g.dataFrameCount, 1)

	// 前 5 个 DATA 帧强制语义填充
	if dataCount <= SemanticPaddingFrames {
		payload = g.applySemanticPadding(payload)
	}

	// H3 DATA 帧格式: Type(varint) + Length(varint) + Payload
	frame := make([]byte, 0, len(payload)+16)

	// 帧类型: DATA (0x00)
	frame = appendVarint(frame, uint64(H3FrameDATA))

	// 帧长度
	frame = appendVarint(frame, uint64(len(payload)))

	// 负载
	frame = append(frame, payload...)

	return frame
}

// applySemanticPadding 应用语义填充
func (g *H3FrameGenerator) applySemanticPadding(payload []byte) []byte {
	currentLen := len(payload)
	if currentLen >= MinPayloadConstraint {
		return payload
	}

	// 计算目标长度 (800-1200 随机)
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	targetLen := MinPayloadConstraint + int(randBytes[0])%400

	paddingNeeded := targetLen - currentLen
	if paddingNeeded <= 0 {
		return payload
	}

	// 构建语义填充
	result := make([]byte, 0, targetLen)
	result = append(result, payload...)

	// 混入语义头部字符串
	for paddingNeeded > 0 {
		// 随机选择语义模板
		idx := int(randBytes[1]) % len(g.semanticPaddings)
		template := g.semanticPaddings[idx]

		if len(template) <= paddingNeeded {
			result = append(result, template...)
			paddingNeeded -= len(template)
		} else {
			result = append(result, template[:paddingNeeded]...)
			paddingNeeded = 0
		}

		// 更新随机索引
		rand.Read(randBytes[1:])
	}

	return result
}

// buildSettingsFrame 构建 SETTINGS 帧
func (g *H3FrameGenerator) buildSettingsFrame() []byte {
	// SETTINGS 参数
	settings := []struct {
		ID    uint64
		Value uint64
	}{
		{0x01, 4096},   // QPACK_MAX_TABLE_CAPACITY
		{0x06, 100},    // MAX_HEADER_LIST_SIZE
		{0x07, 0},      // QPACK_BLOCKED_STREAMS
	}

	// 构建 settings payload
	settingsPayload := make([]byte, 0, 32)
	for _, s := range settings {
		settingsPayload = appendVarint(settingsPayload, s.ID)
		settingsPayload = appendVarint(settingsPayload, s.Value)
	}

	// 构建帧
	frame := make([]byte, 0, len(settingsPayload)+8)
	frame = appendVarint(frame, uint64(H3FrameSETTINGS))
	frame = appendVarint(frame, uint64(len(settingsPayload)))
	frame = append(frame, settingsPayload...)

	return frame
}

// buildHeadersFrame 构建 HEADERS 帧
func (g *H3FrameGenerator) buildHeadersFrame(payload []byte) []byte {
	g.mu.RLock()
	template := g.regionalHeaders[g.Region]
	if template == nil {
		template = g.regionalHeaders["global"]
	}
	g.mu.RUnlock()

	// 简化的 QPACK 编码 Headers
	headers := g.encodeQPACKHeaders(template)

	// 混合真实负载
	combined := make([]byte, 0, len(headers)+len(payload))
	combined = append(combined, headers...)
	combined = append(combined, payload...)

	// 构建帧
	frame := make([]byte, 0, len(combined)+8)
	frame = appendVarint(frame, uint64(H3FrameHEADERS))
	frame = appendVarint(frame, uint64(len(combined)))
	frame = append(frame, combined...)

	return frame
}

// encodeQPACKHeaders 简化的 QPACK 编码
func (g *H3FrameGenerator) encodeQPACKHeaders(t *H3HeadersTemplate) []byte {
	// QPACK 编码前缀
	encoded := []byte{0x00, 0x00} // Required Insert Count = 0, Delta Base = 0

	// 伪头部（使用静态表索引）
	// :method GET = index 17
	encoded = append(encoded, 0xd1) // 11010001 = indexed, static, index 17

	// :scheme https = index 23
	encoded = append(encoded, 0xd7) // 11010111 = indexed, static, index 23

	// :path - 字面量
	encoded = append(encoded, 0x51) // 01010001 = literal with name ref, static, index 1
	encoded = append(encoded, byte(len(t.Path)))
	encoded = append(encoded, []byte(t.Path)...)

	// :authority - 字面量
	encoded = append(encoded, 0x50) // 01010000 = literal with name ref, static, index 0
	encoded = append(encoded, byte(len(t.Authority)))
	encoded = append(encoded, []byte(t.Authority)...)

	// user-agent - 字面量
	encoded = append(encoded, 0x5f, 0x1d) // literal with name ref, static, index 29+
	encoded = append(encoded, byte(len(t.UserAgent)))
	encoded = append(encoded, []byte(t.UserAgent)...)

	return encoded
}

// buildPriorityUpdateFrame 构建 PRIORITY_UPDATE 帧
func (g *H3FrameGenerator) buildPriorityUpdateFrame(payload []byte) []byte {
	// PRIORITY_UPDATE 帧
	priorityPayload := make([]byte, 0, 16)

	// Stream ID (varint)
	streamID := uint64(4 + (g.frameCount * 4)) // 模拟递增的 stream ID
	priorityPayload = appendVarint(priorityPayload, streamID)

	// Priority Field Value (简化)
	priorityPayload = append(priorityPayload, []byte("u=3")...) // urgency=3

	// 混合真实负载
	combined := make([]byte, 0, len(priorityPayload)+len(payload))
	combined = append(combined, priorityPayload...)
	combined = append(combined, payload...)

	// 构建帧
	frame := make([]byte, 0, len(combined)+8)
	frame = appendVarint(frame, uint64(H3FramePRIORITY_UPDATE))
	frame = appendVarint(frame, uint64(len(combined)))
	frame = append(frame, combined...)

	return frame
}

// SetRegion 设置区域
func (g *H3FrameGenerator) SetRegion(region string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Region = region
}

// SetTargetService 设置目标服务
func (g *H3FrameGenerator) SetTargetService(service string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.TargetService = service
}

// ResetFrameCount 重置帧计数（新连接时调用）
func (g *H3FrameGenerator) ResetFrameCount() {
	atomic.StoreUint64(&g.frameCount, 0)
	atomic.StoreUint64(&g.dataFrameCount, 0)
}

// SetDeepMimicry 设置深度拟态模式
func (g *H3FrameGenerator) SetDeepMimicry(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.deepMimicry = enabled
}

// GetFrameCount 获取帧计数
func (g *H3FrameGenerator) GetFrameCount() uint64 {
	return atomic.LoadUint64(&g.frameCount)
}

// appendVarint 追加变长整数
func appendVarint(b []byte, v uint64) []byte {
	if v < 64 {
		return append(b, byte(v))
	} else if v < 16384 {
		return append(b, byte(0x40|(v>>8)), byte(v))
	} else if v < 1073741824 {
		return append(b, byte(0x80|(v>>24)), byte(v>>16), byte(v>>8), byte(v))
	} else {
		return append(b, byte(0xc0|(v>>56)), byte(v>>48), byte(v>>40), byte(v>>32),
			byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}

// UnwrapPayload 解封装 H3 帧，提取负载
func (g *H3FrameGenerator) UnwrapPayload(frame []byte) ([]byte, error) {
	if len(frame) < 2 {
		return frame, nil
	}

	// 读取帧类型
	_, typeLen := readVarint(frame)
	if typeLen == 0 {
		return frame, nil
	}

	// 读取帧长度
	length, lengthLen := readVarint(frame[typeLen:])
	if lengthLen == 0 {
		return frame, nil
	}

	headerLen := typeLen + lengthLen
	if uint64(len(frame)) < uint64(headerLen)+length {
		return frame, nil
	}

	// 返回负载
	return frame[headerLen : uint64(headerLen)+length], nil
}

// readVarint 读取变长整数
func readVarint(b []byte) (uint64, int) {
	if len(b) == 0 {
		return 0, 0
	}

	prefix := b[0] >> 6
	switch prefix {
	case 0:
		return uint64(b[0]), 1
	case 1:
		if len(b) < 2 {
			return 0, 0
		}
		return uint64(b[0]&0x3f)<<8 | uint64(b[1]), 2
	case 2:
		if len(b) < 4 {
			return 0, 0
		}
		return uint64(b[0]&0x3f)<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3]), 4
	case 3:
		if len(b) < 8 {
			return 0, 0
		}
		return uint64(b[0]&0x3f)<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
			uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7]), 8
	}
	return 0, 0
}
