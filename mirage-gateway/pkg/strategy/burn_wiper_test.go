package strategy

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBurnWiper_WipeSlice(t *testing.T) {
	wiper := NewBurnWiper()

	// 模拟一个 32 字节的 AES 密钥
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = 0xAB
	}
	original := make([]byte, 32)
	copy(original, secret)

	wiper.RegisterSecret("test-aes-key", secret)
	wiper.Burn()

	// 验证密钥已被擦除（全零）
	allZero := true
	for _, b := range secret {
		if b != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Fatal("密钥未被擦除为全零")
	}

	// 验证不等于原始值
	if bytes.Equal(secret, original) {
		t.Fatal("密钥仍等于原始值")
	}
}

func TestBurnWiper_WipeFile(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	secretFile := filepath.Join(tmpDir, "gateway.yaml")
	content := []byte("master_key: DEADBEEF1234567890ABCDEF\npsk: supersecret\n")
	if err := os.WriteFile(secretFile, content, 0600); err != nil {
		t.Fatal(err)
	}

	wiper := NewBurnWiper()
	wiper.RegisterPath(secretFile)
	wiper.Burn()

	// 验证文件已被删除
	if _, err := os.Stat(secretFile); !os.IsNotExist(err) {
		t.Fatal("文件未被删除")
	}
}

func TestBurnWiper_WipeGlob(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建多个匹配文件
	for _, name := range []string{"mirage-log1.txt", "mirage-log2.txt", "other.txt"} {
		os.WriteFile(filepath.Join(tmpDir, name), []byte("sensitive"), 0600)
	}

	wiper := NewBurnWiper()
	wiper.RegisterGlob(filepath.Join(tmpDir, "mirage-*"))
	wiper.Burn()

	// mirage-* 应该被删除
	matches, _ := filepath.Glob(filepath.Join(tmpDir, "mirage-*"))
	if len(matches) != 0 {
		t.Fatalf("glob 匹配的文件未被删除: %v", matches)
	}

	// other.txt 应该保留
	if _, err := os.Stat(filepath.Join(tmpDir, "other.txt")); err != nil {
		t.Fatal("非匹配文件被误删")
	}
}

func TestBurnWiper_DoubleCallSafe(t *testing.T) {
	wiper := NewBurnWiper()
	secret := []byte{1, 2, 3, 4}
	wiper.RegisterSecret("key", secret)

	wiper.Burn()
	wiper.Burn() // 第二次调用不应 panic

	for _, b := range secret {
		if b != 0 {
			t.Fatal("密钥未被擦除")
		}
	}
}

func TestBurnWiper_ImplementsSensitiveData(t *testing.T) {
	var _ SensitiveData = (*BurnWiper)(nil)
}
