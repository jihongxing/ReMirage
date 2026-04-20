package raft

import (
	"crypto/rand"
	"testing"

	"mirage-os/pkg/crypto"

	"pgregory.net/rapid"
)

// Feature: mirage-os-completion, Property 8: Shamir 份额验证
// **Validates: Requirements 5.4**
func TestProperty_ValidateShareCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		index := rapid.IntRange(-2, 8).Draw(t, "index")
		valueLen := rapid.IntRange(0, 64).Draw(t, "value_len")
		value := make([]byte, valueLen)
		if valueLen > 0 {
			rand.Read(value)
		}

		share := &crypto.Share{Index: index, Value: value}
		result := ValidateShare(share)

		expected := index >= 1 && index <= 5 && valueLen == 32
		if result != expected {
			t.Fatalf("ValidateShare(index=%d, len=%d) = %v, expected %v",
				index, valueLen, result, expected)
		}
	})
}

// Feature: mirage-os-completion, Property 8 (nil case)
func TestValidateShare_Nil(t *testing.T) {
	if ValidateShare(nil) {
		t.Fatal("expected false for nil share")
	}
}

// Feature: mirage-os-completion, Property 9: Shamir 秘密分享 Round-Trip
// **Validates: Requirements 6.3**
func TestProperty_ShamirRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机 32 字节密钥
		secret := make([]byte, 32)
		rand.Read(secret)

		// SplitSecret 3-of-5
		shares, err := crypto.SplitSecret(secret, crypto.ShamirConfig{Threshold: 3, Shares: 5})
		if err != nil {
			t.Fatalf("SplitSecret failed: %v", err)
		}

		if len(shares) != 5 {
			t.Fatalf("expected 5 shares, got %d", len(shares))
		}

		// 从 5 个份额中随机选 3 个不同的
		i0 := rapid.IntRange(0, 4).Draw(t, "idx0")
		i1 := rapid.IntRange(0, 3).Draw(t, "idx1")
		if i1 >= i0 {
			i1++
		}
		i2 := rapid.IntRange(0, 2).Draw(t, "idx2")
		remaining := []int{}
		for x := 0; x < 5; x++ {
			if x != i0 && x != i1 {
				remaining = append(remaining, x)
			}
		}
		i2idx := remaining[i2]
		selected := []crypto.Share{shares[i0], shares[i1], shares[i2idx]}

		// CombineShares
		recovered, err := crypto.CombineShares(selected)
		if err != nil {
			t.Fatalf("CombineShares failed: %v", err)
		}

		// 验证恢复结果等于原始密钥
		if len(recovered) != len(secret) {
			t.Fatalf("recovered length %d != secret length %d", len(recovered), len(secret))
		}
		for i := range secret {
			if recovered[i] != secret[i] {
				t.Fatalf("mismatch at byte %d: %d != %d", i, recovered[i], secret[i])
			}
		}
	})
}
