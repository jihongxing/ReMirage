package daemon

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthGuardian_RegisterAndRunOnce(t *testing.T) {
	hg := NewHealthGuardian(30 * time.Second)

	hg.Register("tun_check", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "tun_check", Healthy: true, Detail: "TUN device exists"}
	})
	hg.Register("quic_check", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "quic_check", Healthy: false, Detail: "QUIC connection lost"}
	})

	results := hg.RunOnce(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Healthy {
		t.Fatal("tun_check should be healthy")
	}
	if results[1].Healthy {
		t.Fatal("quic_check should be unhealthy")
	}
}

func TestHealthGuardian_OnRepairCallback(t *testing.T) {
	hg := NewHealthGuardian(30 * time.Second)

	var repairCount atomic.Int32
	hg.SetOnRepair(func(check string, err error) {
		repairCount.Add(1)
	})

	hg.Register("failing_check", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "failing_check", Healthy: false, Detail: "broken"}
	})
	hg.Register("passing_check", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "passing_check", Healthy: true, Detail: "ok"}
	})

	hg.RunOnce(context.Background())

	if repairCount.Load() != 1 {
		t.Fatalf("expected 1 repair callback, got %d", repairCount.Load())
	}
}

func TestHealthGuardian_DefaultInterval(t *testing.T) {
	hg := NewHealthGuardian(0)
	if hg.interval != 30*time.Second {
		t.Fatalf("expected default 30s interval, got %v", hg.interval)
	}
}

func TestHealthGuardian_StartStop(t *testing.T) {
	hg := NewHealthGuardian(10 * time.Millisecond)

	var checkCount atomic.Int32
	hg.Register("counter", func(ctx context.Context) HealthCheck {
		checkCount.Add(1)
		return HealthCheck{Name: "counter", Healthy: true}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hg.Start(ctx)
		close(done)
	}()

	// Let it run a few cycles
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if checkCount.Load() == 0 {
		t.Fatal("expected at least one check execution")
	}
}
