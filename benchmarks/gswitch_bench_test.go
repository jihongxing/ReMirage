package benchmarks

import (
	"testing"
	"time"

	"mirage-gateway/pkg/gswitch"
)

// BenchmarkGSwitchReincarnation 测量 G-Switch 转生端到端延迟
func BenchmarkGSwitchReincarnation(b *testing.B) {
	mgr := gswitch.NewGSwitchManager(nil, nil)

	// 预填充热备域名池
	domains := make([]string, 100)
	for i := range domains {
		domains[i] = "standby-domain-" + time.Now().Format("150405") + ".cdn.example.com"
	}
	mgr.ImportDomains(domains)

	// 激活初始域名
	mgr.AddDomain("active.cdn.example.com", "1.2.3.4")
	if d := mgr.GetCurrentDomain(); d == nil {
		// 手动激活
		mgr.ImportDomains([]string{"initial.cdn.example.com"})
	}

	var maxLatency time.Duration
	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 补充热备池
		mgr.ImportDomains([]string{"refill-" + time.Now().Format("150405.000") + ".cdn.example.com"})

		start := time.Now()
		err := mgr.TriggerEscape("benchmark-test")
		elapsed := time.Since(start)

		if err != nil {
			// 热备池可能耗尽，补充后继续
			mgr.ImportDomains(domains)
			continue
		}

		latencies = append(latencies, elapsed)
		if elapsed > maxLatency {
			maxLatency = elapsed
		}
	}
	b.StopTimer()

	if len(latencies) > 0 {
		p99 := percentile(latencies, 99)
		b.ReportMetric(float64(p99.Nanoseconds()), "p99-ns")
		b.ReportMetric(float64(maxLatency.Nanoseconds()), "max-ns")
		if p99 > 5*time.Second {
			b.Errorf("⚠️ P99 延迟 %v 超过 5s 阈值", p99)
		}
	}

	mgr.Stop()
}
