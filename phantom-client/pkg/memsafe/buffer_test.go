package memsafe

import (
	"testing"

	"pgregory.net/rapid"
)

// Property 3: Wipe 后全零验证
func TestProperty_WipeZeroes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		data := rapid.SliceOfN(rapid.Byte(), 1, 4096).Draw(t, "data")
		buf, err := NewSecureBuffer(len(data))
		if err != nil {
			t.Fatal(err)
		}
		_ = buf.Write(data)
		buf.Wipe()
		content := buf.Read()
		for i, b := range content {
			if b != 0 {
				t.Fatalf("byte %d not zero after Wipe: got %d", i, b)
			}
		}
	})
}

// mlock 降级单元测试
func TestMlockDegradation(t *testing.T) {
	buf, err := NewSecureBuffer(64)
	if err != nil {
		t.Fatal(err)
	}
	testData := []byte("sensitive-key-material")
	_ = buf.Write(testData)
	got := buf.Read()
	if string(got) != string(testData) {
		t.Fatalf("expected %q, got %q", testData, got)
	}
	buf.Wipe()
	if !buf.IsWiped() {
		t.Fatal("expected wiped=true")
	}
}

func TestWipeAll(t *testing.T) {
	// Reset global registry
	registryMu.Lock()
	registry = nil
	registryMu.Unlock()

	buf1, _ := NewSecureBuffer(32)
	buf2, _ := NewSecureBuffer(64)
	_ = buf1.Write([]byte("secret1"))
	_ = buf2.Write([]byte("secret2"))

	WipeAll()

	for i, b := range buf1.Read() {
		if b != 0 {
			t.Fatalf("buf1 byte %d not zero", i)
		}
	}
	for i, b := range buf2.Read() {
		if b != 0 {
			t.Fatalf("buf2 byte %d not zero", i)
		}
	}
}
