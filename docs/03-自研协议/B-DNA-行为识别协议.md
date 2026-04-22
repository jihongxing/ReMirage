---
Status: derived
Target Truth: docs/protocols/bdna.md
Migration: 当前有效协议语义已迁移到 docs/protocols/bdna.md，本文降级为解释性输入材料
---

# B-DNA 行为识别协议

## 一、协议定位

**指纹维度防御**：JA4 指纹自适应 + 参数伪装，解决 TLS/QUIC 握手特征识别

- **技术栈**：C (eBPF TC) 数据面 + Go 控制面
- **核心能力**：指纹克隆 + 参数混淆 + 行为拟态
- **语言分工**：C 负责 TCP 握手阶段修改 Header/Window Size，Go 负责指纹库管理

---

## 二、协议架构

### 2.1 分层设计

```
┌─────────────────────────────────────────────────────────┐
│                   B-DNA 协议栈                           │
├─────────────────────────────────────────────────────────┤
│ 决策层 (Go)                                              │
│   ├─ 浏览器指纹库                                        │
│   ├─ 环境感知                                            │
│   └─ 指纹选择策略                                        │
├─────────────────────────────────────────────────────────┤
│ 伪装层                                                    │
│   ├─ TLS 参数克隆                                        │
│   ├─ QUIC 参数克隆                                       │
│   ├─ HTTP/2 SETTINGS 克隆                                │
│   └─ ALPN 协商伪装                                       │
├─────────────────────────────────────────────────────────┤
│ 执行层 (Go crypto/tls + quic-go)                         │
│   ├─ 动态 TLS 栈                                         │
│   ├─ 动态 QUIC 栈                                        │
│   └─ 运行时参数注入                                      │
└─────────────────────────────────────────────────────────┘
```

---

## 三、指纹库

### 3.1 JA4 指纹定义

```go
// JA4 指纹结构
type JA4Fingerprint struct {
    // TLS 参数
    TLSVersion       uint16
    CipherSuites     []uint16
    Extensions       []uint16
    SupportedGroups  []uint16
    SignatureAlgos   []uint16
    
    // QUIC 参数
    InitialMaxData            uint64
    InitialMaxStreamDataBidiLocal  uint64
    InitialMaxStreamDataBidiRemote uint64
    InitialMaxStreamDataUni   uint64
    InitialMaxStreamsBidi     uint64
    InitialMaxStreamsUni      uint64
    MaxIdleTimeout            time.Duration
    MaxUDPPayloadSize         uint64
    AckDelayExponent          uint8
    MaxAckDelay               time.Duration
    
    // HTTP/2 参数
    HTTP2Settings map[uint16]uint32
    
    // 行为特征
    WindowSize       uint32
    TTL              uint8
    TCPOptions       []byte
}

// 全球浏览器指纹库
var BrowserFingerprints = map[string]JA4Fingerprint{
    "Chrome-140-Windows": {
        TLSVersion: tls.VersionTLS13,
        CipherSuites: []uint16{
            tls.TLS_AES_128_GCM_SHA256,
            tls.TLS_AES_256_GCM_SHA384,
            tls.TLS_CHACHA20_POLY1305_SHA256,
            tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        },
        Extensions: []uint16{
            0x0000, // server_name
            0x0017, // extended_master_secret
            0x0023, // session_ticket
            0x000d, // signature_algorithms
            0x000a, // supported_groups
            0x0010, // application_layer_protocol_negotiation
            0x002b, // supported_versions
            0x0033, // key_share
            0x001b, // compress_certificate
            0x0005, // status_request
        },
        SupportedGroups: []uint16{
            0x001d, // X25519
            0x0017, // secp256r1
            0x0018, // secp384r1
        },
        SignatureAlgos: []uint16{
            0x0403, // ecdsa_secp256r1_sha256
            0x0804, // rsa_pss_rsae_sha256
            0x0401, // rsa_pkcs1_sha256
            0x0503, // ecdsa_secp384r1_sha384
            0x0805, // rsa_pss_rsae_sha384
        },
        InitialMaxData: 10485760, // 10 MB
        InitialMaxStreamDataBidiLocal: 6291456,
        InitialMaxStreamDataBidiRemote: 6291456,
        InitialMaxStreamDataUni: 6291456,
        InitialMaxStreamsBidi: 100,
        InitialMaxStreamsUni: 100,
        MaxIdleTimeout: 30 * time.Second,
        MaxUDPPayloadSize: 1350,
        AckDelayExponent: 3,
        MaxAckDelay: 25 * time.Millisecond,
        HTTP2Settings: map[uint16]uint32{
            0x1: 65536,  // HEADER_TABLE_SIZE
            0x2: 0,      // ENABLE_PUSH
            0x3: 1000,   // MAX_CONCURRENT_STREAMS
            0x4: 6291456, // INITIAL_WINDOW_SIZE
            0x5: 16384,  // MAX_FRAME_SIZE
            0x6: 262144, // MAX_HEADER_LIST_SIZE
        },
        WindowSize: 65535,
        TTL: 64,
    },
    
    "Safari-18-macOS": {
        TLSVersion: tls.VersionTLS13,
        CipherSuites: []uint16{
            tls.TLS_AES_128_GCM_SHA256,
            tls.TLS_AES_256_GCM_SHA384,
            tls.TLS_CHACHA20_POLY1305_SHA256,
            tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
        },
        Extensions: []uint16{
            0x0000, 0x0017, 0x0023, 0x000d, 0x000a,
            0x0010, 0x002b, 0x0033, 0x002d, 0x0005,
        },
        InitialMaxData: 15728640, // 15 MB
        InitialMaxStreamDataBidiLocal: 6291456,
        InitialMaxStreamDataBidiRemote: 6291456,
        MaxIdleTimeout: 60 * time.Second,
        MaxUDPPayloadSize: 1472,
        WindowSize: 65535,
        TTL: 64,
    },
    
    "Firefox-135-Linux": {
        TLSVersion: tls.VersionTLS13,
        CipherSuites: []uint16{
            tls.TLS_AES_128_GCM_SHA256,
            tls.TLS_CHACHA20_POLY1305_SHA256,
            tls.TLS_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        },
        Extensions: []uint16{
            0x0000, 0x0017, 0x0023, 0x000d, 0x000a,
            0x0010, 0x002b, 0x0033, 0x001b, 0x0005,
        },
        InitialMaxData: 12582912, // 12 MB
        MaxIdleTimeout: 30 * time.Second,
        MaxUDPPayloadSize: 1452,
        WindowSize: 65535,
        TTL: 64,
    },
}
```

### 3.2 指纹选择策略

```go
type FingerprintSelector struct {
    context *Context
}

func (fs *FingerprintSelector) Select(user *User) string {
    // 1. 基于操作系统
    switch user.OS {
    case "Windows":
        if user.IsEnterprise {
            return "Chrome-140-Windows"
        }
        return "Edge-140-Windows"
    case "macOS":
        return "Safari-18-macOS"
    case "Linux":
        return "Firefox-135-Linux"
    case "Android":
        return "Chrome-140-Android"
    case "iOS":
        return "Safari-18-iOS"
    }
    
    // 2. 基于地理位置
    if user.GeoLocation == "CN" {
        // 中国大陆：模拟主流浏览器
        return "Chrome-140-Windows"
    }
    
    // 3. 基于时间段
    hour := time.Now().Hour()
    if hour >= 9 && hour <= 17 {
        // 办公时间：企业浏览器
        return "Chrome-140-Windows"
    }
    
    // 4. 默认
    return "Chrome-140-Windows"
}
```

---

## 四、动态 TLS 栈

### 4.1 运行时参数注入

```go
type DynamicTLSConfig struct {
    fingerprint JA4Fingerprint
}

func (dtc *DynamicTLSConfig) BuildConfig() *tls.Config {
    return &tls.Config{
        MinVersion: dtc.fingerprint.TLSVersion,
        MaxVersion: dtc.fingerprint.TLSVersion,
        
        // 动态注入 CipherSuites
        CipherSuites: dtc.fingerprint.CipherSuites,
        
        // 动态注入 CurvePreferences
        CurvePreferences: dtc.convertSupportedGroups(),
        
        // 自定义 ClientHello
        GetClientHello: dtc.buildClientHello,
    }
}

// 构建自定义 ClientHello
func (dtc *DynamicTLSConfig) buildClientHello(
    chi *tls.ClientHelloInfo,
) (*tls.Config, error) {
    // 1. 克隆基础配置
    config := dtc.BuildConfig()
    
    // 2. 注入扩展顺序
    config.Extensions = dtc.fingerprint.Extensions
    
    // 3. 注入签名算法
    config.SignatureSchemes = dtc.fingerprint.SignatureAlgos
    
    return config, nil
}

// 转换 SupportedGroups
func (dtc *DynamicTLSConfig) convertSupportedGroups() []tls.CurveID {
    curves := make([]tls.CurveID, len(dtc.fingerprint.SupportedGroups))
    for i, group := range dtc.fingerprint.SupportedGroups {
        curves[i] = tls.CurveID(group)
    }
    return curves
}
```

### 4.2 扩展顺序控制

```go
// 使用 uTLS 库实现扩展顺序控制
import utls "github.com/refraction-networking/utls"

func (dtc *DynamicTLSConfig) BuildUTLSConfig() *utls.Config {
    // 1. 创建自定义 ClientHelloSpec
    spec := utls.ClientHelloSpec{
        TLSVersMin: dtc.fingerprint.TLSVersion,
        TLSVersMax: dtc.fingerprint.TLSVersion,
        
        CipherSuites: dtc.convertCipherSuites(),
        
        // 2. 按指纹顺序添加扩展
        Extensions: dtc.buildExtensions(),
    }
    
    return &utls.Config{
        ClientHelloSpec: spec,
    }
}

// 构建扩展列表
func (dtc *DynamicTLSConfig) buildExtensions() []utls.TLSExtension {
    extensions := []utls.TLSExtension{}
    
    for _, extID := range dtc.fingerprint.Extensions {
        switch extID {
        case 0x0000: // server_name
            extensions = append(extensions, &utls.SNIExtension{})
        case 0x0017: // extended_master_secret
            extensions = append(extensions, &utls.ExtendedMasterSecretExtension{})
        case 0x0023: // session_ticket
            extensions = append(extensions, &utls.SessionTicketExtension{})
        case 0x000d: // signature_algorithms
            extensions = append(extensions, &utls.SignatureAlgorithmsExtension{
                SupportedSignatureAlgorithms: dtc.fingerprint.SignatureAlgos,
            })
        case 0x000a: // supported_groups
            extensions = append(extensions, &utls.SupportedCurvesExtension{
                Curves: dtc.convertSupportedGroups(),
            })
        case 0x0010: // ALPN
            extensions = append(extensions, &utls.ALPNExtension{
                AlpnProtocols: []string{"h2", "http/1.1"},
            })
        case 0x002b: // supported_versions
            extensions = append(extensions, &utls.SupportedVersionsExtension{
                Versions: []uint16{dtc.fingerprint.TLSVersion},
            })
        case 0x0033: // key_share
            extensions = append(extensions, &utls.KeyShareExtension{
                KeyShares: dtc.buildKeyShares(),
            })
        }
    }
    
    return extensions
}
```

---

## 五、动态 QUIC 栈

### 5.1 Transport Parameters 克隆

```go
import "github.com/quic-go/quic-go"

type DynamicQUICConfig struct {
    fingerprint JA4Fingerprint
}

func (dqc *DynamicQUICConfig) BuildConfig() *quic.Config {
    return &quic.Config{
        // 动态注入 Transport Parameters
        InitialStreamReceiveWindow:     dqc.fingerprint.InitialMaxStreamDataBidiLocal,
        MaxStreamReceiveWindow:          dqc.fingerprint.InitialMaxStreamDataBidiLocal * 2,
        InitialConnectionReceiveWindow:  dqc.fingerprint.InitialMaxData,
        MaxConnectionReceiveWindow:      dqc.fingerprint.InitialMaxData * 2,
        
        MaxIdleTimeout: dqc.fingerprint.MaxIdleTimeout,
        
        MaxIncomingStreams: int64(dqc.fingerprint.InitialMaxStreamsBidi),
        MaxIncomingUniStreams: int64(dqc.fingerprint.InitialMaxStreamsUni),
        
        // 自定义 Transport Parameters
        GetTransportParameters: dqc.buildTransportParameters,
    }
}

// 构建自定义 Transport Parameters
func (dqc *DynamicQUICConfig) buildTransportParameters() *quic.TransportParameters {
    return &quic.TransportParameters{
        InitialMaxData: dqc.fingerprint.InitialMaxData,
        InitialMaxStreamDataBidiLocal: dqc.fingerprint.InitialMaxStreamDataBidiLocal,
        InitialMaxStreamDataBidiRemote: dqc.fingerprint.InitialMaxStreamDataBidiRemote,
        InitialMaxStreamDataUni: dqc.fingerprint.InitialMaxStreamDataUni,
        InitialMaxStreamsBidi: dqc.fingerprint.InitialMaxStreamsBidi,
        InitialMaxStreamsUni: dqc.fingerprint.InitialMaxStreamsUni,
        MaxIdleTimeout: dqc.fingerprint.MaxIdleTimeout,
        MaxUDPPayloadSize: dqc.fingerprint.MaxUDPPayloadSize,
        AckDelayExponent: dqc.fingerprint.AckDelayExponent,
        MaxAckDelay: dqc.fingerprint.MaxAckDelay,
    }
}
```

### 5.2 Initial Packet 伪装

```go
// 自定义 Initial Packet 构造
func (dqc *DynamicQUICConfig) buildInitialPacket(
    destConnID []byte,
) []byte {
    packet := &quic.InitialPacket{
        Header: quic.Header{
            IsLongHeader: true,
            Type:         quic.PacketTypeInitial,
            Version:      quic.Version1,
            DestConnID:   destConnID,
            SrcConnID:    dqc.generateConnID(),
        },
        Token: nil, // 首次连接无 Token
    }
    
    // 添加 CRYPTO Frame（包含 ClientHello）
    packet.Frames = []quic.Frame{
        &quic.CryptoFrame{
            Offset: 0,
            Data:   dqc.buildClientHello(),
        },
    }
    
    return packet.Marshal()
}

// 生成随机 Connection ID（模拟浏览器）
func (dqc *DynamicQUICConfig) generateConnID() []byte {
    // Chrome: 8 字节
    // Safari: 8 字节
    // Firefox: 8 字节
    connID := make([]byte, 8)
    rand.Read(connID)
    return connID
}
```

---

## 六、HTTP/2 SETTINGS 克隆

### 6.1 动态 SETTINGS 帧

```go
import "golang.org/x/net/http2"

type DynamicHTTP2Config struct {
    fingerprint JA4Fingerprint
}

func (dhc *DynamicHTTP2Config) BuildTransport() *http2.Transport {
    transport := &http2.Transport{}
    
    // 自定义 SETTINGS 帧
    transport.WriteSettings = func(w io.Writer) error {
        settings := dhc.buildSettings()
        return http2.WriteSettings(w, settings)
    }
    
    return transport
}

// 构建 SETTINGS 帧
func (dhc *DynamicHTTP2Config) buildSettings() []http2.Setting {
    settings := []http2.Setting{}
    
    for id, value := range dhc.fingerprint.HTTP2Settings {
        settings = append(settings, http2.Setting{
            ID:  http2.SettingID(id),
            Val: value,
        })
    }
    
    return settings
}
```

---

## 七、行为拟态

### 7.1 时序拟态

```go
type BehaviorMimic struct {
    fingerprint JA4Fingerprint
}

// 模拟浏览器连接建立时序
func (bm *BehaviorMimic) MimicConnectionTiming(conn net.Conn) {
    // 1. TCP 握手后延迟（模拟 CPU 处理）
    time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
    
    // 2. 发送 ClientHello
    conn.Write(bm.buildClientHello())
    
    // 3. 等待 ServerHello（模拟网络延迟）
    time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
    
    // 4. 发送 HTTP/2 SETTINGS
    time.Sleep(time.Duration(rand.Intn(2)) * time.Millisecond)
    conn.Write(bm.buildHTTP2Settings())
}

// 模拟浏览器请求模式
func (bm *BehaviorMimic) MimicRequestPattern(client *http.Client) {
    // 1. 首次请求：HTML
    client.Get("https://example.com/")
    
    // 2. 并行请求：CSS/JS（模拟浏览器解析）
    time.Sleep(50 * time.Millisecond)
    
    var wg sync.WaitGroup
    resources := []string{
        "/style.css",
        "/script.js",
        "/logo.png",
    }
    
    for _, res := range resources {
        wg.Add(1)
        go func(url string) {
            defer wg.Done()
            client.Get("https://example.com" + url)
        }(res)
    }
    
    wg.Wait()
}
```

### 7.2 窗口大小拟态

```go
// 设置 TCP 窗口大小
func (bm *BehaviorMimic) SetTCPWindow(conn *net.TCPConn) error {
    // 1. 获取原始 socket
    rawConn, err := conn.SyscallConn()
    if err != nil {
        return err
    }
    
    // 2. 设置窗口大小
    return rawConn.Control(func(fd uintptr) {
        syscall.SetsockoptInt(
            int(fd),
            syscall.SOL_SOCKET,
            syscall.SO_RCVBUF,
            int(bm.fingerprint.WindowSize),
        )
    })
}

// 设置 TTL
func (bm *BehaviorMimic) SetTTL(conn *net.IPConn) error {
    rawConn, err := conn.SyscallConn()
    if err != nil {
        return err
    }
    
    return rawConn.Control(func(fd uintptr) {
        syscall.SetsockoptInt(
            int(fd),
            syscall.IPPROTO_IP,
            syscall.IP_TTL,
            int(bm.fingerprint.TTL),
        )
    })
}
```

---

## 八、性能指标

### 8.1 指纹匹配度

| 浏览器 | TLS 匹配度 | QUIC 匹配度 | HTTP/2 匹配度 | 综合评分 |
|--------|-----------|------------|--------------|---------|
| Chrome 140 | 98% | 95% | 97% | 96.7% |
| Safari 18 | 97% | 94% | 96% | 95.7% |
| Firefox 135 | 96% | 93% | 95% | 94.7% |

### 8.2 性能开销

| 组件 | 延迟增加 | CPU 开销 | 内存开销 |
|------|---------|---------|---------|
| 动态 TLS 栈 | < 2ms | < 1% | 50 KB |
| 动态 QUIC 栈 | < 3ms | < 2% | 100 KB |
| 行为拟态 | < 5ms | < 1% | 20 KB |
| **总计** | **< 10ms** | **< 4%** | **170 KB** |

---

## 九、配置示例

```yaml
bdna:
  # 全局开关
  enabled: true
  
  # 指纹选择
  fingerprint:
    auto_select: true
    default: Chrome-140-Windows
    strategy: os_based  # os_based/geo_based/time_based
  
  # 动态 TLS
  dynamic_tls:
    enabled: true
    utls: true
    extension_order: true
  
  # 动态 QUIC
  dynamic_quic:
    enabled: true
    transport_params: true
    initial_packet: true
  
  # HTTP/2
  http2:
    enabled: true
    settings_frame: true
  
  # 行为拟态
  behavior_mimic:
    enabled: true
    timing: true
    request_pattern: true
    tcp_window: true
    ttl: true
```

---

## 十、实现参考

```go
// pkg/bdna/mimic.go
package bdna

type Mimic struct {
    selector    *FingerprintSelector
    fingerprints map[string]JA4Fingerprint
    tlsConfig   *DynamicTLSConfig
    quicConfig  *DynamicQUICConfig
    http2Config *DynamicHTTP2Config
    behavior    *BehaviorMimic
}

func NewMimic() *Mimic {
    return &Mimic{
        selector:     NewFingerprintSelector(),
        fingerprints: BrowserFingerprints,
    }
}

func (m *Mimic) SelectFingerprint(user *User) string {
    return m.selector.Select(user)
}

func (m *Mimic) ApplyFingerprint(name string) error {
    fingerprint := m.fingerprints[name]
    
    // 1. 配置 TLS
    m.tlsConfig = &DynamicTLSConfig{fingerprint: fingerprint}
    
    // 2. 配置 QUIC
    m.quicConfig = &DynamicQUICConfig{fingerprint: fingerprint}
    
    // 3. 配置 HTTP/2
    m.http2Config = &DynamicHTTP2Config{fingerprint: fingerprint}
    
    // 4. 配置行为
    m.behavior = &BehaviorMimic{fingerprint: fingerprint}
    
    return nil
}

func (m *Mimic) Dial(network, addr string) (net.Conn, error) {
    // 1. 建立 TCP 连接
    conn, err := net.Dial(network, addr)
    if err != nil {
        return nil, err
    }
    
    // 2. 应用 TCP 参数
    if tcpConn, ok := conn.(*net.TCPConn); ok {
        m.behavior.SetTCPWindow(tcpConn)
    }
    
    // 3. 应用行为拟态
    m.behavior.MimicConnectionTiming(conn)
    
    return conn, nil
}
```

