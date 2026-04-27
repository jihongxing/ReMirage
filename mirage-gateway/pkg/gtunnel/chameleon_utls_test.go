package gtunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"testing"
	"time"
)

// TestDialWithUTLS_Handshake 验证 dialWithUTLS 能成功完成 TLS 握手
func TestDialWithUTLS_Handshake(t *testing.T) {
	// 生成自签名证书
	cert := generateTestCert(t)

	// 启动 TLS 服务端
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("TLS listen 失败: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	// 服务端 accept 循环
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		conn.Read(buf)
	}()

	// 使用 dialWithUTLS 连接
	baseTLS := &tls.Config{
		InsecureSkipVerify: true,
	}
	conn, err := dialWithUTLS(addr, "localhost", baseTLS)
	if err != nil {
		t.Fatalf("dialWithUTLS 失败: %v", err)
	}
	defer conn.Close()

	// 验证连接可用
	_, err = conn.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
}

// TestDialChameleon_UsesUTLS 验证 DialChameleon 使用 NetDialTLSContext（uTLS 路径）
// 通过检查 DialChameleon 构建的 dialer 不设置 TLSClientConfig 来间接验证
func TestDialChameleon_UsesUTLS(t *testing.T) {
	// 生成自签名证书
	cert := generateTestCert(t)

	// 启动简单的 HTTPS 服务端（完成 TLS 握手后返回 HTTP 400）
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("TLS listen 失败: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	// 服务端：接受连接，完成 TLS 握手，读取请求后返回 HTTP 400
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				c.Read(buf)
				// 返回 HTTP 400 Bad Request（不支持 WebSocket 升级）
				c.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n"))
			}(conn)
		}
	}()

	// 尝试 DialChameleon — 预期 WebSocket 升级会失败（服务端不支持 WS），
	// 但 TLS 握手应该成功（使用 uTLS）
	config := ChameleonDialConfig{
		Endpoint:    "wss://" + addr + "/api/v2/stream",
		SNI:         "localhost",
		DialTimeout: 2 * time.Second,
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	}

	// DialChameleon 会因为服务端不支持 WebSocket 而失败
	_, err = DialChameleon(t.Context(), config)
	if err == nil {
		t.Fatal("预期 DialChameleon 失败（服务端不支持 WebSocket），但成功了")
	}

	// 验证错误链中包含 "uTLS" 或 "WebSocket 拨号失败"
	// 这证明 DialChameleon 走的是 uTLS 路径（NetDialTLSContext），而非 Go 原生 TLSClientConfig
	errStr := err.Error()
	if !contains(errStr, "WebSocket 拨号失败") {
		t.Fatalf("预期 WebSocket 拨号失败错误，实际: %v", err)
	}
	// 错误链中应包含 "uTLS" — 证明走的是 dialWithUTLS 路径
	if !contains(errStr, "uTLS") {
		t.Fatalf("错误链中未包含 'uTLS'，可能未使用 uTLS 路径: %v", err)
	}
	t.Logf("✅ DialChameleon 使用 uTLS 路径: %v", err)
}

// TestParseWSEndpoint 验证 endpoint URL 解析
func TestParseWSEndpoint(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
	}{
		{"wss://example.com:443/path", "example.com:443"},
		{"wss://example.com/path", "example.com"},
		{"wss://1.2.3.4:8443/api", "1.2.3.4:8443"},
	}

	for _, tt := range tests {
		u, err := parseWSEndpoint(tt.input)
		if err != nil {
			t.Errorf("parseWSEndpoint(%q) 失败: %v", tt.input, err)
			continue
		}
		if u.Host != tt.wantHost {
			t.Errorf("parseWSEndpoint(%q).Host = %q, want %q", tt.input, u.Host, tt.wantHost)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("生成证书失败: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}
