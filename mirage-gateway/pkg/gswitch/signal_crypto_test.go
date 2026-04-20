package gswitch

import (
	"crypto/ed25519"
	"testing"
	"time"
)

func TestSignalCryptoRoundTrip(t *testing.T) {
	// 生成密钥对
	verifyKey, signingKey, err := GenerateSigningKeyPair()
	if err != nil {
		t.Fatalf("生成签名密钥失败: %v", err)
	}

	clientPub, clientPriv, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("生成 X25519 密钥失败: %v", err)
	}

	// OS 侧 Publisher
	publisher := NewSignalCryptoPublisher(signingKey, clientPub)

	// 客户端侧 Resolver
	resolver := NewSignalCryptoResolver(verifyKey, clientPriv)

	// 构造信令
	payload := &SignalPayload{
		Timestamp: time.Now().Unix(),
		TTL:       300,
		Gateways: []GatewayEntry{
			{IP: [4]byte{192, 168, 1, 100}, Port: 443, Priority: 10},
			{IP: [4]byte{10, 0, 0, 1}, Port: 8443, Priority: 5},
		},
		Domains: []string{
			"gw1.cdn-static.example.com",
			"gw2.edge-cache.example.net",
		},
	}

	// Seal
	sealed, err := publisher.SealSignal(payload)
	if err != nil {
		t.Fatalf("SealSignal 失败: %v", err)
	}

	t.Logf("密文长度: %d bytes", len(sealed))

	// Open
	decoded, err := resolver.OpenSignal(sealed)
	if err != nil {
		t.Fatalf("OpenSignal 失败: %v", err)
	}

	// 验证内容
	if decoded.Timestamp != payload.Timestamp {
		t.Errorf("Timestamp 不匹配: got %d, want %d", decoded.Timestamp, payload.Timestamp)
	}
	if decoded.TTL != payload.TTL {
		t.Errorf("TTL 不匹配: got %d, want %d", decoded.TTL, payload.TTL)
	}
	if len(decoded.Gateways) != len(payload.Gateways) {
		t.Fatalf("Gateway 数量不匹配: got %d, want %d", len(decoded.Gateways), len(payload.Gateways))
	}
	for i, gw := range decoded.Gateways {
		if gw.IP != payload.Gateways[i].IP {
			t.Errorf("Gateway[%d] IP 不匹配", i)
		}
		if gw.Port != payload.Gateways[i].Port {
			t.Errorf("Gateway[%d] Port 不匹配", i)
		}
		if gw.Priority != payload.Gateways[i].Priority {
			t.Errorf("Gateway[%d] Priority 不匹配", i)
		}
	}
	if len(decoded.Domains) != len(payload.Domains) {
		t.Fatalf("Domain 数量不匹配: got %d, want %d", len(decoded.Domains), len(payload.Domains))
	}
	for i, d := range decoded.Domains {
		if d != payload.Domains[i] {
			t.Errorf("Domain[%d] 不匹配: got %s, want %s", i, d, payload.Domains[i])
		}
	}
}

func TestSignalCryptoTamperDetection(t *testing.T) {
	verifyKey, signingKey, _ := GenerateSigningKeyPair()
	clientPub, clientPriv, _ := GenerateX25519KeyPair()

	publisher := NewSignalCryptoPublisher(signingKey, clientPub)
	resolver := NewSignalCryptoResolver(verifyKey, clientPriv)

	payload := &SignalPayload{
		Timestamp: time.Now().Unix(),
		TTL:       300,
		Gateways:  []GatewayEntry{{IP: [4]byte{1, 2, 3, 4}, Port: 443, Priority: 1}},
		Domains:   []string{"test.example.com"},
	}

	sealed, _ := publisher.SealSignal(payload)

	// 篡改密文中间的一个字节
	sealed[50] ^= 0xFF

	_, err := resolver.OpenSignal(sealed)
	if err == nil {
		t.Fatal("篡改后的密文应该解密失败")
	}
	t.Logf("篡改检测成功: %v", err)
}

func TestSignalCryptoWrongKey(t *testing.T) {
	_, signingKey, _ := GenerateSigningKeyPair()
	clientPub, _, _ := GenerateX25519KeyPair()

	// 用另一组密钥尝试解密
	verifyKey2, _, _ := GenerateSigningKeyPair()
	_, clientPriv2, _ := GenerateX25519KeyPair()

	publisher := NewSignalCryptoPublisher(signingKey, clientPub)
	resolver := NewSignalCryptoResolver(verifyKey2, clientPriv2) // 错误的密钥

	payload := &SignalPayload{
		Timestamp: time.Now().Unix(),
		TTL:       300,
		Gateways:  []GatewayEntry{{IP: [4]byte{1, 2, 3, 4}, Port: 443, Priority: 1}},
		Domains:   []string{"test.example.com"},
	}

	sealed, _ := publisher.SealSignal(payload)

	_, err := resolver.OpenSignal(sealed)
	if err == nil {
		t.Fatal("错误密钥应该解密失败")
	}
	t.Logf("密钥不匹配检测成功: %v", err)
}

func TestSignalCryptoReplayRejection(t *testing.T) {
	verifyKey, signingKey, _ := GenerateSigningKeyPair()
	clientPub, clientPriv, _ := GenerateX25519KeyPair()

	publisher := NewSignalCryptoPublisher(signingKey, clientPub)
	resolver := NewSignalCryptoResolver(verifyKey, clientPriv)

	// 第一条信令
	payload1 := &SignalPayload{
		Timestamp: time.Now().Unix(),
		TTL:       300,
		Gateways:  []GatewayEntry{{IP: [4]byte{1, 2, 3, 4}, Port: 443, Priority: 1}},
		Domains:   []string{"first.example.com"},
	}
	sealed1, _ := publisher.SealSignal(payload1)
	_, err := resolver.OpenSignal(sealed1)
	if err != nil {
		t.Fatalf("第一条信令应该成功: %v", err)
	}

	// 第二条信令（更新的时间戳）
	payload2 := &SignalPayload{
		Timestamp: time.Now().Unix() + 1,
		TTL:       300,
		Gateways:  []GatewayEntry{{IP: [4]byte{5, 6, 7, 8}, Port: 443, Priority: 1}},
		Domains:   []string{"second.example.com"},
	}
	sealed2, _ := publisher.SealSignal(payload2)
	_, err = resolver.OpenSignal(sealed2)
	if err != nil {
		t.Fatalf("第二条信令应该成功: %v", err)
	}

	// 重放第一条信令（时间戳更老）
	sealed1Again, _ := publisher.SealSignal(payload1)
	_, err = resolver.OpenSignal(sealed1Again)
	if err == nil {
		t.Fatal("重放旧信令应该被拒绝")
	}
	t.Logf("重放检测成功: %v", err)
}

func TestSignalCryptoExpiredSignal(t *testing.T) {
	verifyKey, signingKey, _ := GenerateSigningKeyPair()
	clientPub, clientPriv, _ := GenerateX25519KeyPair()

	publisher := NewSignalCryptoPublisher(signingKey, clientPub)
	resolver := NewSignalCryptoResolver(verifyKey, clientPriv)

	// 过期信令（TTL=1s，时间戳在 10s 前）
	payload := &SignalPayload{
		Timestamp: time.Now().Unix() - 10,
		TTL:       1,
		Gateways:  []GatewayEntry{{IP: [4]byte{1, 2, 3, 4}, Port: 443, Priority: 1}},
		Domains:   []string{"expired.example.com"},
	}
	sealed, _ := publisher.SealSignal(payload)

	_, err := resolver.OpenSignal(sealed)
	if err == nil {
		t.Fatal("过期信令应该被拒绝")
	}
	t.Logf("过期检测成功: %v", err)
}

func TestSignalCryptoForgedSignature(t *testing.T) {
	_, signingKey, _ := GenerateSigningKeyPair()
	clientPub, clientPriv, _ := GenerateX25519KeyPair()

	// 攻击者用自己的签名密钥
	attackerVerifyKey, attackerSigningKey, _ := GenerateSigningKeyPair()
	_ = attackerSigningKey

	// Publisher 用正确的加密密钥但攻击者的签名密钥
	fakePublisher := NewSignalCryptoPublisher(attackerSigningKey, clientPub)
	// Resolver 用正确的 OS 验证公钥
	resolver := NewSignalCryptoResolver(ed25519.PublicKey(signingKey.Public().(ed25519.PublicKey)), clientPriv)

	_ = attackerVerifyKey

	payload := &SignalPayload{
		Timestamp: time.Now().Unix(),
		TTL:       300,
		Gateways:  []GatewayEntry{{IP: [4]byte{6, 6, 6, 6}, Port: 443, Priority: 1}},
		Domains:   []string{"evil.example.com"},
	}
	sealed, _ := fakePublisher.SealSignal(payload)

	_, err := resolver.OpenSignal(sealed)
	if err == nil {
		t.Fatal("伪造签名应该验证失败")
	}
	t.Logf("伪造签名检测成功: %v", err)
}
