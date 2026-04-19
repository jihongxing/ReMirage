// Package strategy - 协议变色龙
package strategy

import (
	"crypto/rand"
	"fmt"
	"log"
)

// ChameleonProfile 变色龙配置文件
type ChameleonProfile struct {
	Name           string
	TLSFingerprint TLSFingerprint
	QUICConfig     QUICConfig
	HTTPHeaders    []HTTPHeader
}

// TLSFingerprint TLS 指纹
type TLSFingerprint struct {
	Version       uint16   // TLS 版本
	CipherSuites  []uint16 // 密码套件
	Extensions    []uint16 // 扩展
	EllipticCurves []uint16 // 椭圆曲线
	ECPointFormats []uint8  // EC 点格式
	ALPN          []string // ALPN 协议
	SignatureAlgs []uint16 // 签名算法
}

// QUICConfig QUIC 配置
type QUICConfig struct {
	Version           uint32 // QUIC 版本
	ConnectionIDLen   uint8  // 连接 ID 长度
	InitialPacketSize uint16 // 初始包大小
	ACKFrequency      uint8  // ACK 频率
}

// HTTPHeader HTTP 头
type HTTPHeader struct {
	Name  string
	Value string
}

// Chameleon 协议变色龙
type Chameleon struct {
	profiles map[string]*ChameleonProfile
	active   *ChameleonProfile
}

// NewChameleon 创建协议变色龙
func NewChameleon() *Chameleon {
	c := &Chameleon{
		profiles: make(map[string]*ChameleonProfile),
	}
	
	// 加载预设配置文件
	c.loadProfiles()
	
	return c
}

// loadProfiles 加载预设配置文件
func (c *Chameleon) loadProfiles() {
	// Zoom Windows 客户端
	c.profiles["zoom-windows"] = &ChameleonProfile{
		Name: "Zoom Windows Client",
		TLSFingerprint: TLSFingerprint{
			Version: 0x0303, // TLS 1.2
			CipherSuites: []uint16{
				0x1301, // TLS_AES_128_GCM_SHA256
				0x1302, // TLS_AES_256_GCM_SHA384
				0x1303, // TLS_CHACHA20_POLY1305_SHA256
				0xc02b, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
				0xc02f, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
				0xc02c, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
				0xc030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
			},
			Extensions: []uint16{
				0x0000, // server_name
				0x0017, // extended_master_secret
				0x0023, // session_ticket
				0x000d, // signature_algorithms
				0x0005, // status_request
				0x000b, // ec_point_formats
				0x000a, // supported_groups
				0x0010, // application_layer_protocol_negotiation
				0x0012, // signed_certificate_timestamp
				0x002b, // supported_versions
				0x0033, // key_share
			},
			EllipticCurves: []uint16{
				0x001d, // x25519
				0x0017, // secp256r1
				0x0018, // secp384r1
			},
			ECPointFormats: []uint8{0x00}, // uncompressed
			ALPN:           []string{"h2", "http/1.1"},
			SignatureAlgs: []uint16{
				0x0403, // ecdsa_secp256r1_sha256
				0x0804, // rsa_pss_rsae_sha256
				0x0401, // rsa_pkcs1_sha256
				0x0503, // ecdsa_secp384r1_sha384
				0x0805, // rsa_pss_rsae_sha384
				0x0501, // rsa_pkcs1_sha384
			},
		},
		QUICConfig: QUICConfig{
			Version:           0x00000001, // QUIC v1
			ConnectionIDLen:   8,
			InitialPacketSize: 1200,
			ACKFrequency:      2,
		},
		HTTPHeaders: []HTTPHeader{
			{Name: "User-Agent", Value: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
			{Name: "Accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			{Name: "Accept-Language", Value: "en-US,en;q=0.5"},
			{Name: "Accept-Encoding", Value: "gzip, deflate, br"},
		},
	}
	
	// Chrome 浏览器
	c.profiles["chrome-windows"] = &ChameleonProfile{
		Name: "Chrome Windows",
		TLSFingerprint: TLSFingerprint{
			Version: 0x0303,
			CipherSuites: []uint16{
				0x1301, 0x1302, 0x1303,
				0xc02b, 0xc02f, 0xc02c, 0xc030,
				0xcca9, 0xcca8, 0xc013, 0xc014,
			},
			Extensions: []uint16{
				0x0000, 0x0017, 0x0023, 0x000d,
				0x0005, 0x000b, 0x000a, 0x0010,
				0x0012, 0x002b, 0x0033, 0x002d,
			},
			EllipticCurves: []uint16{0x001d, 0x0017, 0x0018, 0x0019},
			ECPointFormats: []uint8{0x00},
			ALPN:           []string{"h2", "http/1.1"},
			SignatureAlgs:  []uint16{0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501},
		},
		QUICConfig: QUICConfig{
			Version:           0x00000001,
			ConnectionIDLen:   8,
			InitialPacketSize: 1200,
			ACKFrequency:      2,
		},
	}
	
	// Microsoft Teams
	c.profiles["teams-windows"] = &ChameleonProfile{
		Name: "Microsoft Teams",
		TLSFingerprint: TLSFingerprint{
			Version:        0x0303,
			CipherSuites:   []uint16{0x1301, 0x1302, 0x1303, 0xc02b, 0xc02f},
			Extensions:     []uint16{0x0000, 0x0017, 0x0023, 0x000d, 0x0010},
			EllipticCurves: []uint16{0x001d, 0x0017},
			ECPointFormats: []uint8{0x00},
			ALPN:           []string{"h2"},
		},
		QUICConfig: QUICConfig{
			Version:           0x00000001,
			ConnectionIDLen:   8,
			InitialPacketSize: 1200,
			ACKFrequency:      1,
		},
	}
	
	log.Printf("[Chameleon] 加载 %d 个配置文件", len(c.profiles))
}

// SetProfile 设置活跃配置文件
func (c *Chameleon) SetProfile(name string) error {
	profile, ok := c.profiles[name]
	if !ok {
		return fmt.Errorf("配置文件不存在: %s", name)
	}
	
	c.active = profile
	log.Printf("[Chameleon] 切换到配置文件: %s", name)
	
	return nil
}

// GenerateTLSClientHello 生成 TLS ClientHello
func (c *Chameleon) GenerateTLSClientHello() ([]byte, error) {
	if c.active == nil {
		return nil, fmt.Errorf("未设置活跃配置文件")
	}
	
	fp := c.active.TLSFingerprint
	
	// TLS 记录层
	record := make([]byte, 0, 512)
	
	// 记录类型: Handshake (0x16)
	record = append(record, 0x16)
	
	// 版本: TLS 1.0 (0x0301) - 兼容性
	record = append(record, 0x03, 0x01)
	
	// 长度占位符
	lengthPos := len(record)
	record = append(record, 0x00, 0x00)
	
	// Handshake 类型: ClientHello (0x01)
	record = append(record, 0x01)
	
	// Handshake 长度占位符
	hsLengthPos := len(record)
	record = append(record, 0x00, 0x00, 0x00)
	
	// 客户端版本
	record = append(record, byte(fp.Version>>8), byte(fp.Version))
	
	// 随机数 (32 字节)
	random := make([]byte, 32)
	rand.Read(random)
	record = append(record, random...)
	
	// Session ID (空)
	record = append(record, 0x00)
	
	// Cipher Suites
	record = append(record, byte(len(fp.CipherSuites)*2>>8), byte(len(fp.CipherSuites)*2))
	for _, suite := range fp.CipherSuites {
		record = append(record, byte(suite>>8), byte(suite))
	}
	
	// Compression Methods (null)
	record = append(record, 0x01, 0x00)
	
	// Extensions
	extData := c.buildExtensions(fp)
	record = append(record, byte(len(extData)>>8), byte(len(extData)))
	record = append(record, extData...)
	
	// 填充长度
	hsLength := len(record) - hsLengthPos - 3
	record[hsLengthPos] = byte(hsLength >> 16)
	record[hsLengthPos+1] = byte(hsLength >> 8)
	record[hsLengthPos+2] = byte(hsLength)
	
	recordLength := len(record) - lengthPos - 2
	record[lengthPos] = byte(recordLength >> 8)
	record[lengthPos+1] = byte(recordLength)
	
	return record, nil
}

// buildExtensions 构建扩展
func (c *Chameleon) buildExtensions(fp TLSFingerprint) []byte {
	ext := make([]byte, 0, 256)
	
	// Server Name (SNI)
	if hasExtension(fp.Extensions, 0x0000) {
		sni := c.buildSNIExtension("example.com")
		ext = append(ext, sni...)
	}
	
	// Supported Groups
	if hasExtension(fp.Extensions, 0x000a) {
		groups := c.buildSupportedGroupsExtension(fp.EllipticCurves)
		ext = append(ext, groups...)
	}
	
	// EC Point Formats
	if hasExtension(fp.Extensions, 0x000b) {
		formats := c.buildECPointFormatsExtension(fp.ECPointFormats)
		ext = append(ext, formats...)
	}
	
	// Signature Algorithms
	if hasExtension(fp.Extensions, 0x000d) {
		sigAlgs := c.buildSignatureAlgorithmsExtension(fp.SignatureAlgs)
		ext = append(ext, sigAlgs...)
	}
	
	// ALPN
	if hasExtension(fp.Extensions, 0x0010) {
		alpn := c.buildALPNExtension(fp.ALPN)
		ext = append(ext, alpn...)
	}
	
	// Supported Versions
	if hasExtension(fp.Extensions, 0x002b) {
		versions := c.buildSupportedVersionsExtension()
		ext = append(ext, versions...)
	}
	
	return ext
}

// buildSNIExtension 构建 SNI 扩展
func (c *Chameleon) buildSNIExtension(hostname string) []byte {
	ext := make([]byte, 0, 64)
	ext = append(ext, 0x00, 0x00) // Extension type
	
	length := 2 + 1 + 2 + len(hostname)
	ext = append(ext, byte(length>>8), byte(length))
	ext = append(ext, byte((length-2)>>8), byte(length-2))
	ext = append(ext, 0x00) // Name type: host_name
	ext = append(ext, byte(len(hostname)>>8), byte(len(hostname)))
	ext = append(ext, []byte(hostname)...)
	
	return ext
}

// buildSupportedGroupsExtension 构建支持的组
func (c *Chameleon) buildSupportedGroupsExtension(groups []uint16) []byte {
	ext := make([]byte, 0, 32)
	ext = append(ext, 0x00, 0x0a) // Extension type
	
	length := 2 + len(groups)*2
	ext = append(ext, byte(length>>8), byte(length))
	ext = append(ext, byte((len(groups)*2)>>8), byte(len(groups)*2))
	
	for _, group := range groups {
		ext = append(ext, byte(group>>8), byte(group))
	}
	
	return ext
}

// buildECPointFormatsExtension 构建 EC 点格式
func (c *Chameleon) buildECPointFormatsExtension(formats []uint8) []byte {
	ext := make([]byte, 0, 16)
	ext = append(ext, 0x00, 0x0b) // Extension type
	
	length := 1 + len(formats)
	ext = append(ext, byte(length>>8), byte(length))
	ext = append(ext, byte(len(formats)))
	ext = append(ext, formats...)
	
	return ext
}

// buildSignatureAlgorithmsExtension 构建签名算法
func (c *Chameleon) buildSignatureAlgorithmsExtension(algs []uint16) []byte {
	ext := make([]byte, 0, 32)
	ext = append(ext, 0x00, 0x0d) // Extension type
	
	length := 2 + len(algs)*2
	ext = append(ext, byte(length>>8), byte(length))
	ext = append(ext, byte((len(algs)*2)>>8), byte(len(algs)*2))
	
	for _, alg := range algs {
		ext = append(ext, byte(alg>>8), byte(alg))
	}
	
	return ext
}

// buildALPNExtension 构建 ALPN
func (c *Chameleon) buildALPNExtension(protocols []string) []byte {
	ext := make([]byte, 0, 64)
	ext = append(ext, 0x00, 0x10) // Extension type
	
	protoData := make([]byte, 0, 32)
	for _, proto := range protocols {
		protoData = append(protoData, byte(len(proto)))
		protoData = append(protoData, []byte(proto)...)
	}
	
	length := 2 + len(protoData)
	ext = append(ext, byte(length>>8), byte(length))
	ext = append(ext, byte(len(protoData)>>8), byte(len(protoData)))
	ext = append(ext, protoData...)
	
	return ext
}

// buildSupportedVersionsExtension 构建支持的版本
func (c *Chameleon) buildSupportedVersionsExtension() []byte {
	ext := make([]byte, 0, 16)
	ext = append(ext, 0x00, 0x2b) // Extension type
	ext = append(ext, 0x00, 0x03) // Length
	ext = append(ext, 0x02)       // Versions length
	ext = append(ext, 0x03, 0x04) // TLS 1.3
	
	return ext
}

// GenerateQUICConnectionID 生成 QUIC 连接 ID
func (c *Chameleon) GenerateQUICConnectionID() []byte {
	if c.active == nil {
		return nil
	}
	
	connID := make([]byte, c.active.QUICConfig.ConnectionIDLen)
	rand.Read(connID)
	
	return connID
}

// GetQUICInitialPacketSize 获取 QUIC 初始包大小
func (c *Chameleon) GetQUICInitialPacketSize() uint16 {
	if c.active == nil {
		return 1200
	}
	
	return c.active.QUICConfig.InitialPacketSize
}

// hasExtension 检查是否包含扩展
func hasExtension(extensions []uint16, ext uint16) bool {
	for _, e := range extensions {
		if e == ext {
			return true
		}
	}
	return false
}

// GetJA4Fingerprint 计算 JA4 指纹
func (c *Chameleon) GetJA4Fingerprint() string {
	if c.active == nil {
		return ""
	}
	
	// JA4 格式: <version>_<ciphers>_<extensions>_<curves>
	// 简化版本
	fp := c.active.TLSFingerprint
	
	return fmt.Sprintf("%04x_%d_%d_%d",
		fp.Version,
		len(fp.CipherSuites),
		len(fp.Extensions),
		len(fp.EllipticCurves),
	)
}
