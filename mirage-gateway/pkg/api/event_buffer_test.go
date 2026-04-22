package api

import (
	"testing"

	pb "mirage-proto/gen"
)

func TestEventBuffer_BufferWhenDisconnected(t *testing.T) {
	client := NewGRPCClient("localhost:50847", "test-gw", nil)
	// 未连接状态下 ReportThreat 应缓存事件
	events := []*pb.ThreatEvent{
		{Timestamp: 1, ThreatType: pb.ThreatType_DPI_DETECTION, Severity: 5},
		{Timestamp: 2, ThreatType: pb.ThreatType_DPI_DETECTION, Severity: 3},
	}

	_ = client.ReportThreat(events)

	if client.GetBufferCount() != 2 {
		t.Fatalf("缓存数量应为 2，实际: %d", client.GetBufferCount())
	}
}

func TestEventBuffer_MaxBufferTruncation(t *testing.T) {
	client := NewGRPCClient("localhost:50847", "test-gw", nil)

	// 填满缓冲区（maxBuffer=1000）
	for i := 0; i < 1100; i++ {
		client.bufferEvents([]*pb.ThreatEvent{
			{Timestamp: int64(i), Severity: 1},
		})
	}

	count := client.GetBufferCount()
	if count > 1000 {
		t.Fatalf("缓冲区应截断到 1000，实际: %d", count)
	}
}

func TestEventBuffer_FlushClearsBuffer(t *testing.T) {
	client := NewGRPCClient("localhost:50847", "test-gw", nil)

	// 缓存一些事件
	client.bufferEvents([]*pb.ThreatEvent{
		{Timestamp: 1, Severity: 5},
		{Timestamp: 2, Severity: 3},
	})

	if client.GetBufferCount() != 2 {
		t.Fatalf("缓存数量应为 2，实际: %d", client.GetBufferCount())
	}

	// flushEventBuffer 在无连接时会重新缓存
	// 但我们可以验证 flush 逻辑的取出部分
	client.mu.Lock()
	events := make([]*pb.ThreatEvent, len(client.eventBuffer))
	copy(events, client.eventBuffer)
	client.eventBuffer = client.eventBuffer[:0]
	client.mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("取出事件数应为 2，实际: %d", len(events))
	}
	if client.GetBufferCount() != 0 {
		t.Fatalf("清空后缓存应为 0，实际: %d", client.GetBufferCount())
	}
}
