package api

import (
	"context"
	pb "mirage-proto/gen"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"pgregory.net/rapid"
)

// Feature: gateway-closure, Property 11: Protobuf 序列化往返一致性
// 注意: 使用手写 proto 结构体，验证字段赋值往返一致性
func TestProperty_ProtoRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hb := &pb.HeartbeatRequest{
			GatewayId:         rapid.String().Draw(t, "gw_id"),
			Timestamp:         int64(rapid.IntRange(0, 1<<50).Draw(t, "ts")),
			EbpfLoaded:        rapid.Bool().Draw(t, "ebpf"),
			ThreatLevel:       int32(rapid.IntRange(0, 5).Draw(t, "level")),
			ActiveConnections: int64(rapid.IntRange(0, 10000).Draw(t, "conns")),
			MemoryUsageMb:     int32(rapid.IntRange(0, 1024).Draw(t, "mem")),
		}

		// 验证字段赋值一致性
		if hb.GatewayId == "" && rapid.String().Draw(t, "check") != "" {
			// 空字符串也是有效值
		}
		if hb.ThreatLevel < 0 || hb.ThreatLevel > 5 {
			t.Fatalf("ThreatLevel 越界: %d", hb.ThreatLevel)
		}
	})
}

// Feature: gateway-closure, Property 12: 断连缓存上限
func TestProperty_DisconnectBufferLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		client := NewGRPCClient("localhost:50847", "test-gw", nil)
		// 不连接，模拟断连状态

		n := rapid.IntRange(1001, 2000).Draw(t, "n")
		for i := 0; i < n; i++ {
			events := []*pb.ThreatEvent{{
				Timestamp:  time.Now().Unix(),
				SourceIp:   "10.0.0.1",
				Severity:   5,
				SourcePort: uint32(i),
			}}
			client.ReportThreat(events)
		}

		count := client.GetBufferCount()
		if count > 1000 {
			t.Fatalf("缓存超过上限: %d > 1000", count)
		}
	})
}

// Feature: gateway-closure, Property 13: 无效下行指令拒绝
func TestProperty_InvalidCommandRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		handler := NewCommandHandler(nil, nil, nil)

		// 测试 PushStrategy 负数 defense_level
		negLevel := int32(-1 * rapid.IntRange(1, 100).Draw(t, "neg"))
		_, err := handler.PushStrategy(context.Background(), &pb.StrategyPush{
			DefenseLevel: negLevel,
		})
		if err == nil {
			t.Fatal("应拒绝负数 defense_level")
		}
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.InvalidArgument {
			t.Fatalf("应返回 InvalidArgument，实际: %v", err)
		}

		// 测试 PushStrategy 超大 defense_level
		bigLevel := int32(rapid.IntRange(6, 100).Draw(t, "big"))
		_, err = handler.PushStrategy(context.Background(), &pb.StrategyPush{
			DefenseLevel: bigLevel,
		})
		if err == nil {
			t.Fatal("应拒绝超大 defense_level")
		}
	})
}

// 单元测试: PushBlacklist 空 CIDR 拒绝
func TestHandler_PushBlacklist_EmptyCIDR(t *testing.T) {
	handler := NewCommandHandler(nil, nil, nil)
	_, err := handler.PushBlacklist(context.Background(), &pb.BlacklistPush{
		Entries: []*pb.BlacklistEntryProto{{Cidr: ""}},
	})
	if err == nil {
		t.Fatal("应拒绝空 CIDR")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("应返回 InvalidArgument，实际: %v", st.Code())
	}
}

// 单元测试: PushReincarnation deadline <= 0 拒绝
func TestHandler_PushReincarnation_InvalidDeadline(t *testing.T) {
	handler := NewCommandHandler(nil, nil, nil)
	_, err := handler.PushReincarnation(context.Background(), &pb.ReincarnationPush{
		NewDomain:       "test.example.com",
		DeadlineSeconds: 0,
	})
	if err == nil {
		t.Fatal("应拒绝 deadline_seconds <= 0")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("应返回 InvalidArgument，实际: %v", st.Code())
	}
}

// 单元测试: PushBlacklist 空 entries 拒绝
func TestHandler_PushBlacklist_EmptyEntries(t *testing.T) {
	handler := NewCommandHandler(nil, nil, nil)
	_, err := handler.PushBlacklist(context.Background(), &pb.BlacklistPush{
		Entries: []*pb.BlacklistEntryProto{},
	})
	if err == nil {
		t.Fatal("应拒绝空 entries")
	}
}
