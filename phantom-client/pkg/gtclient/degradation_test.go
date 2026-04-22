package gtclient

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- Property 16: 退化事件完整性测试 ---
// Feature: v1-client-productization, Property 16: 退化事件完整性
// **Validates: Requirements 10.2, 10.3, 10.5**
func TestProperty_DegradationEventCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random degradation level transition
		fromLevel := DegradationLevel(rapid.IntRange(0, 2).Draw(t, "fromLevel"))
		toLevel := DegradationLevel(rapid.IntRange(0, 2).Draw(t, "toLevel"))
		reason := rapid.StringMatching(`[a-z ]{5,30}`).Draw(t, "reason")
		attempts := rapid.IntRange(0, 100).Draw(t, "attempts")

		if toLevel < fromLevel {
			// Recovery: from higher to lower level → Duration must be > 0
			duration := time.Duration(rapid.Int64Range(1, int64(time.Hour)).Draw(t, "duration"))
			event := NewRecoveryEvent(toLevel, reason, attempts, duration)

			if event.Level != toLevel {
				t.Fatalf("level: got %v, want %v", event.Level, toLevel)
			}
			if event.Reason == "" {
				t.Fatal("reason must be non-empty")
			}
			if event.EnteredAt.IsZero() {
				t.Fatal("enteredAt must be non-zero")
			}
			if event.Duration <= 0 {
				t.Fatalf("recovery event duration must be > 0, got %v", event.Duration)
			}
		} else {
			// Degradation or same level → Duration is 0
			event := NewDegradationEvent(toLevel, reason, attempts)

			if event.Level != toLevel {
				t.Fatalf("level: got %v, want %v", event.Level, toLevel)
			}
			if event.Reason == "" {
				t.Fatal("reason must be non-empty")
			}
			if event.EnteredAt.IsZero() {
				t.Fatal("enteredAt must be non-zero")
			}
			if event.Duration != 0 {
				t.Fatalf("non-recovery event duration must be 0, got %v", event.Duration)
			}
		}
	})
}

func TestDegradationLevel_String(t *testing.T) {
	tests := []struct {
		level DegradationLevel
		want  string
	}{
		{L1_Normal, "L1_Normal"},
		{L2_Degraded, "L2_Degraded"},
		{L3_LastResort, "L3_LastResort"},
		{DegradationLevel(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("DegradationLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestGTunnelClient_DegradationLevel(t *testing.T) {
	config := makeTestConfig()
	c := NewGTunnelClient(config)
	defer c.Close()

	// Initial level should be L1_Normal (zero value)
	if c.DegradationLevel() != L1_Normal {
		t.Fatalf("expected L1_Normal, got %v", c.DegradationLevel())
	}

	// Test setDegradation with callback
	var received []DegradationEvent
	c.SetOnDegradation(func(e DegradationEvent) {
		received = append(received, e)
	})

	c.setDegradation(L2_Degraded, "bootstrap fallback", 3)
	if c.DegradationLevel() != L2_Degraded {
		t.Fatalf("expected L2_Degraded, got %v", c.DegradationLevel())
	}

	// Wait a tiny bit to ensure duration > 0 on recovery
	time.Sleep(time.Millisecond)

	c.setDegradation(L1_Normal, "recovered via runtime topo", 1)
	if c.DegradationLevel() != L1_Normal {
		t.Fatalf("expected L1_Normal, got %v", c.DegradationLevel())
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	// First event: degradation to L2
	if received[0].Level != L2_Degraded {
		t.Fatalf("event[0] level: got %v, want L2_Degraded", received[0].Level)
	}
	if received[0].Duration != 0 {
		t.Fatalf("event[0] duration should be 0 for degradation, got %v", received[0].Duration)
	}

	// Second event: recovery to L1
	if received[1].Level != L1_Normal {
		t.Fatalf("event[1] level: got %v, want L1_Normal", received[1].Level)
	}
	if received[1].Duration <= 0 {
		t.Fatalf("event[1] duration should be > 0 for recovery, got %v", received[1].Duration)
	}
}
