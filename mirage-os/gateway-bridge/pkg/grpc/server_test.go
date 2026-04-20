package grpc

import (
	"context"
	"testing"

	pb "mirage-proto/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"pgregory.net/rapid"
)

// Feature: mirage-os-brain, Property 10: 无效 gRPC 请求拒绝
func TestProperty_InvalidRequestRejection(t *testing.T) {
	srv := &Server{}

	rapid.Check(t, func(t *rapid.T) {
		emptyID := rapid.SampledFrom([]string{"", ""}).Draw(t, "emptyID")
		timestamp := rapid.SampledFrom([]int64{0, 0}).Draw(t, "zeroTimestamp")

		// 测试空 gateway_id
		_, err := srv.SyncHeartbeat(context.Background(), &pb.HeartbeatRequest{
			GatewayId: emptyID,
			Timestamp: 1234567890,
		})
		if err == nil {
			t.Fatal("expected error for empty gateway_id")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", st.Code())
		}

		// 测试零 timestamp
		_, err = srv.SyncHeartbeat(context.Background(), &pb.HeartbeatRequest{
			GatewayId: "gw-test",
			Timestamp: timestamp,
		})
		if err == nil {
			t.Fatal("expected error for zero timestamp")
		}
		st, _ = status.FromError(err)
		if st.Code() != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", st.Code())
		}
	})
}

// 单元测试：ReportTraffic 空 gateway_id 拒绝
func TestReportTraffic_EmptyGatewayID(t *testing.T) {
	srv := &Server{}
	_, err := srv.ReportTraffic(context.Background(), &pb.TrafficRequest{
		GatewayId: "",
	})
	if err == nil {
		t.Fatal("expected error for empty gateway_id")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", st.Code())
	}
}

// 单元测试：ReportThreat 空 gateway_id 拒绝
func TestReportThreat_EmptyGatewayID(t *testing.T) {
	srv := &Server{}
	_, err := srv.ReportThreat(context.Background(), &pb.ThreatRequest{
		GatewayId: "",
	})
	if err == nil {
		t.Fatal("expected error for empty gateway_id")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", st.Code())
	}
}

// 单元测试：mapStatus 映射
func TestMapStatus(t *testing.T) {
	tests := []struct {
		input    pb.GatewayStatus
		expected string
	}{
		{pb.GatewayStatus_ONLINE, "ONLINE"},
		{pb.GatewayStatus_DEGRADED, "DEGRADED"},
		{pb.GatewayStatus_EMERGENCY, "OFFLINE"},
	}
	for _, tt := range tests {
		got := mapStatus(tt.input)
		if got != tt.expected {
			t.Errorf("mapStatus(%v) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}
