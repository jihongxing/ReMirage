package benchmarks

import (
	"crypto/rand"
	"testing"
	"time"

	"mirage-gateway/pkg/gtunnel"
)

func benchmarkFECEncode(b *testing.B, size int) {
	fec := gtunnel.NewFECProcessor()
	data := make([]byte, size)
	rand.Read(data)

	var maxLatency time.Duration
	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, err := fec.Encode(data)
		elapsed := time.Since(start)
		if err != nil {
			b.Fatal(err)
		}
		latencies = append(latencies, elapsed)
		if elapsed > maxLatency {
			maxLatency = elapsed
		}
	}
	b.StopTimer()

	// 计算 P99
	if len(latencies) > 0 {
		p99 := percentile(latencies, 99)
		b.ReportMetric(float64(p99.Nanoseconds()), "p99-ns")
		if p99 > 1*time.Millisecond {
			b.Logf("⚠️ P99 延迟 %v 超过 1ms 阈值", p99)
		}
	}
}

func benchmarkFECDecode(b *testing.B, size int) {
	fec := gtunnel.NewFECProcessor()
	data := make([]byte, size)
	rand.Read(data)

	shards, err := fec.Encode(data)
	if err != nil {
		b.Fatal(err)
	}

	indices := make([]int, len(shards))
	for i := range indices {
		indices[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := fec.Decode(shards, indices)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFECEncode_64B(b *testing.B)   { benchmarkFECEncode(b, 64) }
func BenchmarkFECEncode_512B(b *testing.B)  { benchmarkFECEncode(b, 512) }
func BenchmarkFECEncode_1500B(b *testing.B) { benchmarkFECEncode(b, 1500) }
func BenchmarkFECEncode_9000B(b *testing.B) { benchmarkFECEncode(b, 9000) }

func BenchmarkFECDecode_64B(b *testing.B)   { benchmarkFECDecode(b, 64) }
func BenchmarkFECDecode_512B(b *testing.B)  { benchmarkFECDecode(b, 512) }
func BenchmarkFECDecode_1500B(b *testing.B) { benchmarkFECDecode(b, 1500) }
func BenchmarkFECDecode_9000B(b *testing.B) { benchmarkFECDecode(b, 9000) }

// percentile 计算延迟百分位数
func percentile(latencies []time.Duration, p int) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	// 简单排序
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	idx := (len(sorted) * p) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
