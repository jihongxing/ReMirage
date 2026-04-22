package ctlock

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

type mockAudit struct {
	called bool
}

func (m *mockAudit) OnOverflow(elapsed time.Duration, slotNs int64) {
	m.called = true
}

// TestProperty10_MinimumHoldTime verifies that ProcessControl execution time ≥ minDuration.
// 降级后验证"持锁时间 ≥ minDuration"而非"持锁时间 = constDuration"。
// **Validates: Requirements 10.1, 10.4**
func TestProperty10_MinimumHoldTime(t *testing.T) {
	minDurationNs := int64(10_000_000) // 10ms

	rapid.Check(t, func(t *rapid.T) {
		workUs := rapid.Int64Range(100, 3000).Draw(t, "workUs") // 0.1-3ms of work
		ctl := NewConstantTimeLock(minDurationNs, nil)

		start := time.Now()
		ctl.ProcessControl(func() error {
			deadline := time.Now().Add(time.Duration(workUs) * time.Microsecond)
			for time.Now().Before(deadline) {
			}
			return nil
		})
		elapsed := time.Since(start)

		minDuration := time.Duration(minDurationNs)
		if elapsed < minDuration {
			t.Fatalf("elapsed %v < minDuration %v", elapsed, minDuration)
		}
	})
}

// TestProperty10_StegoMinimumHoldTime verifies ProcessStego execution time ≥ minDuration.
func TestProperty10_StegoMinimumHoldTime(t *testing.T) {
	minDurationNs := int64(10_000_000) // 10ms
	ctl := NewConstantTimeLock(minDurationNs, nil)

	start := time.Now()
	ctl.ProcessStego(true, func() error {
		time.Sleep(1 * time.Millisecond)
		return nil
	})
	elapsed := time.Since(start)

	minDuration := time.Duration(minDurationNs)
	if elapsed < minDuration {
		t.Fatalf("stego(true) elapsed %v < minDuration %v", elapsed, minDuration)
	}

	start2 := time.Now()
	ctl.ProcessStego(false, func() error {
		return nil
	})
	elapsed2 := time.Since(start2)

	if elapsed2 < minDuration {
		t.Fatalf("stego(false) elapsed %v < minDuration %v", elapsed2, minDuration)
	}
}

// TestTimedLock_Overflow verifies that when handler exceeds 2x minDuration,
// AuditCollector is called.
func TestTimedLock_Overflow(t *testing.T) {
	audit := &mockAudit{}
	minDurationNs := int64(1_000_000) // 1ms
	ctl := NewConstantTimeLock(minDurationNs, audit)

	ctl.ProcessControl(func() error {
		time.Sleep(50 * time.Millisecond) // 远超 2x minDuration
		return nil
	})

	if !audit.called {
		t.Fatal("expected AuditCollector.OnOverflow to be called")
	}
	if ctl.Overflows() != 1 {
		t.Fatalf("expected 1 overflow, got %d", ctl.Overflows())
	}
}

// TestTimedLock_MutexProtection verifies concurrent access is serialized.
func TestTimedLock_MutexProtection(t *testing.T) {
	minDurationNs := int64(1_000_000) // 1ms
	ctl := NewConstantTimeLock(minDurationNs, nil)

	counter := 0
	done := make(chan struct{}, 10)

	for i := 0; i < 10; i++ {
		go func() {
			ctl.ProcessControl(func() error {
				counter++
				return nil
			})
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if counter != 10 {
		t.Fatalf("expected counter=10, got %d (race condition?)", counter)
	}
}
