package gtclient

import (
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- Property 9: 指数退避计算测试 ---
// Feature: v1-client-productization, Property 9: 指数退避计算
// **Validates: Requirements 2.2, 6.4**
func TestProperty_ExponentialBackoffComputation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate base in [100ms, 30s], max in [base, 10min]
		baseMs := rapid.IntRange(100, 30000).Draw(t, "baseMs")
		base := time.Duration(baseMs) * time.Millisecond
		maxMs := rapid.IntRange(baseMs, 600000).Draw(t, "maxMs")
		maxD := time.Duration(maxMs) * time.Millisecond
		n := rapid.IntRange(0, 30).Draw(t, "failCount")

		eb := NewExponentialBackoff(base, maxD)
		eb.FailCount = n

		got := eb.Next()

		// Expected: min(base * 2^N, max)
		expected := time.Duration(float64(base) * math.Pow(2, float64(n)))
		if expected > maxD || expected <= 0 {
			expected = maxD
		}

		if got != expected {
			t.Fatalf("N=%d base=%v max=%v: got %v, expected %v", n, base, maxD, got, expected)
		}
	})
}

func TestExponentialBackoff_Reset(t *testing.T) {
	eb := NewExponentialBackoff(time.Second, time.Minute)
	eb.FailCount = 5
	eb.Reset()
	if eb.FailCount != 0 {
		t.Fatalf("expected 0 after reset, got %d", eb.FailCount)
	}
	if eb.Next() != time.Second {
		t.Fatalf("expected base after reset, got %v", eb.Next())
	}
}

func TestExponentialBackoff_Record(t *testing.T) {
	eb := NewExponentialBackoff(time.Second, time.Minute)
	d := eb.Record()
	if eb.FailCount != 1 {
		t.Fatalf("expected failCount=1, got %d", eb.FailCount)
	}
	// 1s * 2^1 = 2s
	if d != 2*time.Second {
		t.Fatalf("expected 2s, got %v", d)
	}
}

func TestExponentialBackoff_OverflowCap(t *testing.T) {
	eb := NewExponentialBackoff(time.Second, 10*time.Minute)
	eb.FailCount = 100 // huge exponent → overflow
	got := eb.Next()
	if got != 10*time.Minute {
		t.Fatalf("expected max on overflow, got %v", got)
	}
}
