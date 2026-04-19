package benchmarks

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func benchmarkResourceUsage(b *testing.B, concurrency int) {
	var wg sync.WaitGroup
	done := make(chan struct{})

	// 模拟并发连接
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 1024*64) // 64KB per connection
			for {
				select {
				case <-done:
					return
				default:
					// 模拟数据处理
					for j := range buf {
						buf[j] = byte(j)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	// 等待稳定
	time.Sleep(500 * time.Millisecond)

	// 采集指标
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	allocMB := float64(memStats.Alloc) / 1024 / 1024
	goroutines := runtime.NumGoroutine()
	gcPauseNs := memStats.PauseNs[(memStats.NumGC+255)%256]

	b.ReportMetric(allocMB, "alloc-MB")
	b.ReportMetric(float64(goroutines), "goroutines")
	b.ReportMetric(float64(gcPauseNs)/1e6, "gc-pause-ms")

	if allocMB > 200 {
		b.Errorf("⚠️ 内存占用 %.1f MB 超过 200MB 阈值", allocMB)
	}

	close(done)
	wg.Wait()
}

func BenchmarkResourceUsage_10(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkResourceUsage(b, 10)
	}
}

func BenchmarkResourceUsage_50(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkResourceUsage(b, 50)
	}
}

func BenchmarkResourceUsage_100(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkResourceUsage(b, 100)
	}
}
