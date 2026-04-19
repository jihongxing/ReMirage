package crypto

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"
)

// Feature: core-hardening, Property 4: AES-GCM 往返一致性
func TestProperty_AESGCMRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成 32 字节密钥
		key := make([]byte, 32)
		for i := range key {
			key[i] = byte(rapid.IntRange(0, 255).Draw(t, "keyByte"))
		}

		// 生成 1-10000 字节明文
		size := rapid.IntRange(1, 10000).Draw(t, "size")
		plaintext := make([]byte, size)
		for i := range plaintext {
			plaintext[i] = byte(rapid.IntRange(0, 255).Draw(t, "ptByte"))
		}

		hk := NewHotKey()
		if err := hk.Activate(key); err != nil {
			t.Fatal(err)
		}
		defer hk.Deactivate()

		ciphertext, err := hk.Encrypt(plaintext)
		if err != nil {
			t.Fatal(err)
		}

		decrypted, err := hk.Decrypt(ciphertext)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Fatal("decrypted data does not match plaintext")
		}
	})
}

// Feature: core-hardening, Property 8: 内存清零
func TestProperty_MemoryWipe(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	hk := NewHotKey()
	if err := hk.Activate(key); err != nil {
		t.Fatal(err)
	}

	// 验证密钥已激活
	if !hk.IsActive() {
		t.Fatal("expected active")
	}

	hk.Deactivate()

	// 断言 key 缓冲区全零
	hk.mu.RLock()
	defer hk.mu.RUnlock()
	for i, b := range hk.key {
		if b != 0 {
			t.Fatalf("key byte %d not zeroed: %d", i, b)
		}
	}
}

// 未激活时 Encrypt 返回 error
func TestHotKey_EncryptNotActive(t *testing.T) {
	hk := NewHotKey()
	_, err := hk.Encrypt([]byte("test"))
	if err == nil {
		t.Fatal("expected error when encrypting with inactive key")
	}
}

// 未激活时 Decrypt 返回 error
func TestHotKey_DecryptNotActive(t *testing.T) {
	hk := NewHotKey()
	_, err := hk.Decrypt([]byte("test"))
	if err == nil {
		t.Fatal("expected error when decrypting with inactive key")
	}
}
