package crypto

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: core-hardening, Property 1: Shamir 往返一致性
func TestProperty_ShamirRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret := make([]byte, 32)
		for i := range secret {
			secret[i] = byte(rapid.IntRange(0, 255).Draw(t, "byte"))
		}

		engine, err := NewShamirEngine(3, 5)
		if err != nil {
			t.Fatal(err)
		}

		shares, err := engine.Split(secret)
		if err != nil {
			t.Fatal(err)
		}

		// 选取任意 3 个份额（随机选 3 个不同的索引）
		i0 := rapid.IntRange(0, 4).Draw(t, "i0")
		i1 := rapid.IntRange(0, 4).Draw(t, "i1")
		for i1 == i0 {
			i1 = rapid.IntRange(0, 4).Draw(t, "i1r")
		}
		i2 := rapid.IntRange(0, 4).Draw(t, "i2")
		for i2 == i0 || i2 == i1 {
			i2 = rapid.IntRange(0, 4).Draw(t, "i2r")
		}
		selected := []Share{shares[i0], shares[i1], shares[i2]}

		recovered, err := engine.Combine(selected)
		if err != nil {
			t.Fatal(err)
		}

		for i := range secret {
			if secret[i] != recovered[i] {
				t.Fatalf("mismatch at byte %d: want %d, got %d", i, secret[i], recovered[i])
			}
		}
	})
}

// Feature: core-hardening, Property 2: 份额不足拒绝
func TestProperty_ShamirInsufficientShares(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret := make([]byte, 32)
		for i := range secret {
			secret[i] = byte(rapid.IntRange(0, 255).Draw(t, "byte"))
		}

		engine, _ := NewShamirEngine(3, 5)
		shares, err := engine.Split(secret)
		if err != nil {
			t.Fatal(err)
		}

		// 只取 2 个份额
		_, err = engine.Combine(shares[:2])
		if err == nil {
			t.Fatal("expected error with insufficient shares")
		}
	})
}

// Feature: core-hardening, Property 3: GF(256) 封闭性
func TestProperty_GF256Closure(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		a := byte(rapid.IntRange(0, 255).Draw(t, "a"))
		b := byte(rapid.IntRange(0, 255).Draw(t, "b"))

		result := gfMul(a, b)
		// 结果在 0-255 范围内（byte 类型保证）
		_ = result

		// 非零元素的逆元性质
		if a != 0 {
			inv := gfInv(a)
			product := gfMul(a, inv)
			if product != 1 {
				t.Fatalf("gfMul(%d, gfInv(%d)) = %d, want 1", a, a, product)
			}
		}
	})
}

// Feature: core-hardening, Property 7: 份额唯一性
func TestProperty_ShamirShareUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret := make([]byte, 32)
		for i := range secret {
			secret[i] = byte(rapid.IntRange(0, 255).Draw(t, "byte"))
		}

		engine, _ := NewShamirEngine(3, 5)
		shares, err := engine.Split(secret)
		if err != nil {
			t.Fatal(err)
		}

		// 断言 5 个 x 坐标互不相同
		seen := make(map[byte]bool)
		for _, s := range shares {
			if seen[s.X] {
				t.Fatalf("duplicate x coordinate: %d", s.X)
			}
			seen[s.X] = true
		}

		// x 坐标应为 1,2,3,4,5
		for i := 0; i < 5; i++ {
			if shares[i].X != byte(i+1) {
				t.Fatalf("share %d: expected x=%d, got x=%d", i, i+1, shares[i].X)
			}
		}
	})
}
