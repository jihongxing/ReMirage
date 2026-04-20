// Package gswitch - 信令共振加密基础设施
// 实现 Sign-then-Encrypt 流水线：
//  1. Ed25519 签名明文
//  2. X25519 ephemeral ECDH → HKDF-SHA256 派生对称密钥
//  3. ChaCha20-Poly1305 AEAD 加密 [明文+签名]
//
// 密文格式: [EphemeralPubKey 32B] + [Nonce 12B] + [Ciphertext+Tag]
// 明文格式: [SignalPayload] + [Ed25519 Signature 64B]
package gswitch

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// SignalCrypto 信令加解密器
type SignalCrypto struct {
	// OS 侧：持有 Ed25519 私钥（签名）+ 客户端群体 X25519 公钥（加密）
	signingKey    ed25519.PrivateKey // 64B，OS 持有
	peerPublicKey [32]byte           // 客户端群体 X25519 公钥

	// 客户端侧：持有 Ed25519 公钥（验签）+ 自身 X25519 私钥（解密）
	verifyKey      ed25519.PublicKey // 32B，客户端持有
	decryptPrivKey [32]byte          // 客户端 X25519 私钥

	// 反重放：上次成功接收的信令时间戳
	lastAcceptedTimestamp int64
}

// NewSignalCryptoPublisher 创建 OS 侧发布器（签名+加密）
func NewSignalCryptoPublisher(signingKey ed25519.PrivateKey, clientGroupPubKey [32]byte) *SignalCrypto {
	return &SignalCrypto{
		signingKey:    signingKey,
		peerPublicKey: clientGroupPubKey,
	}
}

// NewSignalCryptoResolver 创建客户端侧解析器（解密+验签）
func NewSignalCryptoResolver(verifyKey ed25519.PublicKey, decryptPrivKey [32]byte) *SignalCrypto {
	return &SignalCrypto{
		verifyKey:      verifyKey,
		decryptPrivKey: decryptPrivKey,
	}
}

// ============================================================
// 信令包结构
// ============================================================

const (
	signalMagic   = "MRSG"
	signalVersion = 0x01
	ed25519SigLen = 64
	x25519PubLen  = 32
	nonceLen      = 12 // ChaCha20-Poly1305 nonce
	// HKDF info 字符串，绑定协议上下文防止跨协议密钥复用
	hkdfInfo = "mirage-signal-v1-chacha20poly1305"
)

// deriveSymmetricKey 使用 HKDF-SHA256 从 ECDH 共享密钥派生 ChaCha20 对称密钥
// 防止直接使用原始 ECDH 输出（可能存在低熵位）
func deriveSymmetricKey(sharedSecret, ephPub, peerPub []byte) ([]byte, error) {
	// salt = SHA256(ephPub || peerPub)，绑定双方公钥防止 key-compromise impersonation
	saltInput := make([]byte, 0, len(ephPub)+len(peerPub))
	saltInput = append(saltInput, ephPub...)
	saltInput = append(saltInput, peerPub...)
	salt := sha256.Sum256(saltInput)

	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt[:], []byte(hkdfInfo))
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("HKDF 密钥派生失败: %w", err)
	}
	return key, nil
}

// GatewayEntry 网关条目
type GatewayEntry struct {
	IP       [4]byte // IPv4 网络字节序
	Port     uint16
	Priority uint8
}

// SignalPayload 信令明文载荷
type SignalPayload struct {
	Timestamp int64          // Unix epoch seconds
	TTL       uint32         // 有效期（秒）
	Gateways  []GatewayEntry // 存活网关列表
	Domains   []string       // 备用域名列表
}

// SerializePayload 序列化信令载荷（确定性字节序列）
func SerializePayload(p *SignalPayload) []byte {
	// 预估大小
	size := 4 + 1 + 8 + 4 + 1 + len(p.Gateways)*7 + 1
	for _, d := range p.Domains {
		size += 1 + len(d)
	}
	buf := make([]byte, 0, size)

	// Magic (4B)
	buf = append(buf, []byte(signalMagic)...)
	// Version (1B)
	buf = append(buf, signalVersion)
	// Timestamp (8B LE)
	ts := make([]byte, 8)
	binary.LittleEndian.PutUint64(ts, uint64(p.Timestamp))
	buf = append(buf, ts...)
	// TTL (4B LE)
	ttl := make([]byte, 4)
	binary.LittleEndian.PutUint32(ttl, p.TTL)
	buf = append(buf, ttl...)
	// Gateway Count (1B)
	buf = append(buf, byte(len(p.Gateways)))
	// Gateways
	for _, gw := range p.Gateways {
		buf = append(buf, gw.IP[:]...)
		port := make([]byte, 2)
		binary.BigEndian.PutUint16(port, gw.Port)
		buf = append(buf, port...)
		buf = append(buf, gw.Priority)
	}
	// Domain Count (1B)
	buf = append(buf, byte(len(p.Domains)))
	// Domains
	for _, d := range p.Domains {
		buf = append(buf, byte(len(d)))
		buf = append(buf, []byte(d)...)
	}

	return buf
}

// DeserializePayload 反序列化信令载荷
func DeserializePayload(data []byte) (*SignalPayload, error) {
	if len(data) < 18 { // 4+1+8+4+1 minimum
		return nil, errors.New("信令数据过短")
	}

	// Magic
	if string(data[0:4]) != signalMagic {
		return nil, errors.New("无效的信令 Magic")
	}
	// Version
	if data[4] != signalVersion {
		return nil, fmt.Errorf("不支持的信令版本: %d", data[4])
	}

	offset := 5
	// Timestamp
	ts := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8
	// TTL
	ttl := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// Gateway Count
	if offset >= len(data) {
		return nil, errors.New("数据截断: gateway count")
	}
	gwCount := int(data[offset])
	offset++

	// Gateways
	gateways := make([]GatewayEntry, 0, gwCount)
	for i := 0; i < gwCount; i++ {
		if offset+7 > len(data) {
			return nil, errors.New("数据截断: gateway entry")
		}
		var gw GatewayEntry
		copy(gw.IP[:], data[offset:offset+4])
		gw.Port = binary.BigEndian.Uint16(data[offset+4 : offset+6])
		gw.Priority = data[offset+6]
		gateways = append(gateways, gw)
		offset += 7
	}

	// Domain Count
	if offset >= len(data) {
		return nil, errors.New("数据截断: domain count")
	}
	domCount := int(data[offset])
	offset++

	// Domains
	domains := make([]string, 0, domCount)
	for i := 0; i < domCount; i++ {
		if offset >= len(data) {
			return nil, errors.New("数据截断: domain len")
		}
		dLen := int(data[offset])
		offset++
		if offset+dLen > len(data) {
			return nil, errors.New("数据截断: domain name")
		}
		domains = append(domains, string(data[offset:offset+dLen]))
		offset += dLen
	}

	return &SignalPayload{
		Timestamp: ts,
		TTL:       ttl,
		Gateways:  gateways,
		Domains:   domains,
	}, nil
}

// ============================================================
// Sign-then-Encrypt（OS 侧发布）
// ============================================================

// SealSignal 加密信令（OS 侧调用）
// 流水线：明文序列化 → Ed25519 签名 → X25519 ephemeral ECDH → ChaCha20-Poly1305 加密
// 输出：[EphemeralPubKey 32B] + [Nonce 12B] + [Ciphertext+Tag]
func (sc *SignalCrypto) SealSignal(payload *SignalPayload) ([]byte, error) {
	if sc.signingKey == nil {
		return nil, errors.New("签名密钥未设置（非 Publisher 模式）")
	}

	// 1. 序列化明文
	plaintext := SerializePayload(payload)

	// 2. Sign-then-Encrypt: 先签名明文
	signature := ed25519.Sign(sc.signingKey, plaintext)
	// 拼接 [明文 + 签名]
	signedData := make([]byte, len(plaintext)+ed25519SigLen)
	copy(signedData, plaintext)
	copy(signedData[len(plaintext):], signature)

	// 3. 生成临时 X25519 密钥对（每条信令独立，前向保密）
	var ephPriv [32]byte
	if _, err := rand.Read(ephPriv[:]); err != nil {
		return nil, fmt.Errorf("生成临时私钥失败: %w", err)
	}
	ephPub, err := curve25519.X25519(ephPriv[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("计算临时公钥失败: %w", err)
	}

	// 4. ECDH：临时私钥 × 客户端群体公钥 → 原始共享密钥
	rawSharedSecret, err := curve25519.X25519(ephPriv[:], sc.peerPublicKey[:])
	if err != nil {
		return nil, fmt.Errorf("ECDH 协商失败: %w", err)
	}

	// 5. HKDF 派生对称密钥（不直接使用原始 ECDH 输出）
	symmetricKey, err := deriveSymmetricKey(rawSharedSecret, ephPub, sc.peerPublicKey[:])
	if err != nil {
		return nil, err
	}

	// 6. 构造 AEAD
	aead, err := chacha20poly1305.New(symmetricKey)
	if err != nil {
		return nil, fmt.Errorf("创建 AEAD 失败: %w", err)
	}

	// 7. 生成唯一 Nonce（crypto/rand，绝不复用）
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("生成 Nonce 失败: %w", err)
	}

	// 8. 加密（AAD = ephPub，绑定临时公钥防止密文被移植到其他 ephemeral 上下文）
	ciphertext := aead.Seal(nil, nonce, signedData, ephPub)

	// 9. 组装输出：[EphPubKey 32B] + [Nonce 12B] + [Ciphertext+Tag]
	output := make([]byte, 0, x25519PubLen+nonceLen+len(ciphertext))
	output = append(output, ephPub...)
	output = append(output, nonce...)
	output = append(output, ciphertext...)

	return output, nil
}

// ============================================================
// Decrypt-then-Verify（客户端侧解析）
// ============================================================

// OpenSignal 解密并验证信令（客户端侧调用）
// 输入：[EphemeralPubKey 32B] + [Nonce 12B] + [Ciphertext+Tag]
// 输出：验证通过的 SignalPayload
func (sc *SignalCrypto) OpenSignal(sealed []byte) (*SignalPayload, error) {
	if sc.verifyKey == nil {
		return nil, errors.New("验证公钥未设置（非 Resolver 模式）")
	}

	// 1. 解析头部
	minLen := x25519PubLen + nonceLen + chacha20poly1305.Overhead + 18 + ed25519SigLen
	if len(sealed) < minLen {
		return nil, fmt.Errorf("密文过短: %d < %d", len(sealed), minLen)
	}

	ephPub := sealed[:x25519PubLen]
	nonce := sealed[x25519PubLen : x25519PubLen+nonceLen]
	ciphertext := sealed[x25519PubLen+nonceLen:]

	// 2. ECDH：客户端私钥 × 临时公钥 → 原始共享密钥
	rawSharedSecret, err := curve25519.X25519(sc.decryptPrivKey[:], ephPub)
	if err != nil {
		return nil, fmt.Errorf("ECDH 解密协商失败: %w", err)
	}

	// 3. HKDF 派生对称密钥（与 SealSignal 对称）
	// SealSignal 使用 deriveSymmetricKey(secret, ephPub, peerPublicKey)
	// 其中 peerPublicKey = 客户端群体公钥
	// 客户端侧需要计算自己的公钥来匹配
	clientPub, pubErr := curve25519.X25519(sc.decryptPrivKey[:], curve25519.Basepoint)
	if pubErr != nil {
		return nil, fmt.Errorf("计算客户端公钥失败: %w", pubErr)
	}
	symmetricKey, err := deriveSymmetricKey(rawSharedSecret, ephPub, clientPub)
	if err != nil {
		return nil, err
	}

	// 4. 构造 AEAD
	aead, err := chacha20poly1305.New(symmetricKey)
	if err != nil {
		return nil, fmt.Errorf("创建 AEAD 失败: %w", err)
	}

	// 5. 解密（AAD = ephPub，与加密侧对称）
	signedData, err := aead.Open(nil, nonce, ciphertext, ephPub)
	if err != nil {
		return nil, fmt.Errorf("AEAD 解密失败（密文被篡改或密钥不匹配）: %w", err)
	}

	// 5. 分离明文和签名
	if len(signedData) < ed25519SigLen {
		return nil, errors.New("解密数据过短，无法提取签名")
	}
	plaintext := signedData[:len(signedData)-ed25519SigLen]
	signature := signedData[len(signedData)-ed25519SigLen:]

	// 6. 验证 Ed25519 签名
	if !ed25519.Verify(sc.verifyKey, plaintext, signature) {
		return nil, errors.New("Ed25519 签名验证失败（信令被伪造）")
	}

	// 7. 反序列化
	payload, err := DeserializePayload(plaintext)
	if err != nil {
		return nil, fmt.Errorf("信令反序列化失败: %w", err)
	}

	// 8. 反重放校验：Timestamp + TTL
	if err := sc.validateTimestamp(payload); err != nil {
		return nil, err
	}

	// 9. 更新最后接受时间戳
	sc.lastAcceptedTimestamp = payload.Timestamp

	return payload, nil
}

// validateTimestamp 时间戳硬核校验（Anti-Replay）
func (sc *SignalCrypto) validateTimestamp(p *SignalPayload) error {
	now := time.Now().Unix()

	// 信令是否过期：Timestamp + TTL < now
	expiry := p.Timestamp + int64(p.TTL)
	if expiry < now {
		return fmt.Errorf("信令已过期: ts=%d, ttl=%d, now=%d", p.Timestamp, p.TTL, now)
	}

	// 信令是否来自未来（时钟偏差容忍 60s）
	if p.Timestamp > now+60 {
		return fmt.Errorf("信令时间戳来自未来: ts=%d, now=%d", p.Timestamp, now)
	}

	// 反重放：不接受比上次成功接收更老的信令
	if p.Timestamp <= sc.lastAcceptedTimestamp {
		return fmt.Errorf("信令重放检测: ts=%d <= last=%d", p.Timestamp, sc.lastAcceptedTimestamp)
	}

	return nil
}

// ============================================================
// 密钥生成工具
// ============================================================

// GenerateSigningKeyPair 生成 Ed25519 签名密钥对（OS 侧初始化时调用一次）
func GenerateSigningKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("生成 Ed25519 密钥对失败: %w", err)
	}
	return pub, priv, nil
}

// GenerateX25519KeyPair 生成 X25519 密钥对（客户端群体密钥）
func GenerateX25519KeyPair() (publicKey [32]byte, privateKey [32]byte, err error) {
	if _, err := rand.Read(privateKey[:]); err != nil {
		return [32]byte{}, [32]byte{}, fmt.Errorf("生成 X25519 私钥失败: %w", err)
	}
	// Clamp（curve25519.X25519 内部会做，但显式 clamp 更安全）
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	pub, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return [32]byte{}, [32]byte{}, fmt.Errorf("计算 X25519 公钥失败: %w", err)
	}
	copy(publicKey[:], pub)
	return publicKey, privateKey, nil
}
