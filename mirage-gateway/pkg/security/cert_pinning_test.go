package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func generateTestCert(t interface{ Fatal(...interface{}) }) *x509.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// Property 4: 证书钉扎验证往返
func TestProperty_CertPinRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := NewCertPin("")
		cert := generateTestCert(t)

		if err := cp.PinCertificate(cert); err != nil {
			t.Fatalf("PinCertificate 失败: %v", err)
		}

		// 相同证书验证应通过
		if err := cp.VerifyPin(cert); err != nil {
			t.Fatalf("相同证书验证失败: %v", err)
		}

		// 不同证书验证应失败
		otherCert := generateTestCert(t)
		if err := cp.VerifyPin(otherCert); err == nil {
			t.Fatal("不同证书验证应失败")
		}
	})
}

// Property 5: 更新后旧证书拒绝
func TestProperty_CertPinUpdateRejectsOld(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := NewCertPin("")
		certA := generateTestCert(t)
		certB := generateTestCert(t)

		if err := cp.PinCertificate(certA); err != nil {
			t.Fatalf("PinCertificate A 失败: %v", err)
		}

		if err := cp.UpdatePin(certB); err != nil {
			t.Fatalf("UpdatePin B 失败: %v", err)
		}

		// B 应通过
		if err := cp.VerifyPin(certB); err != nil {
			t.Fatalf("更新后 B 验证失败: %v", err)
		}

		// A 应被拒绝
		if err := cp.VerifyPin(certA); err == nil {
			t.Fatal("更新后 A 应被拒绝")
		}
	})
}

// 单元测试: 预设指纹加载
func TestCertPin_PresetHash(t *testing.T) {
	cert := generateTestCert(t)
	cp := NewCertPin("")
	cp.PinCertificate(cert)
	hash := cp.GetPinnedHash()

	cp2 := NewCertPin(hash)
	if !cp2.IsPinned() {
		t.Fatal("预设指纹应标记为已钉扎")
	}
	if err := cp2.VerifyPin(cert); err != nil {
		t.Fatalf("预设指纹验证失败: %v", err)
	}
}

// 单元测试: 未钉扎时 VerifyPin 返回错误
func TestCertPin_NotPinned(t *testing.T) {
	cp := NewCertPin("")
	cert := generateTestCert(t)
	if err := cp.VerifyPin(cert); err == nil {
		t.Fatal("未钉扎时应返回错误")
	}
}

// 单元测试: 无效预设指纹
func TestCertPin_InvalidPresetHash(t *testing.T) {
	cp := NewCertPin("invalid-hex")
	if cp.IsPinned() {
		t.Fatal("无效指纹不应标记为已钉扎")
	}
}
