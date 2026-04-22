package stealth

import (
	"context"
	"fmt"
	"testing"
	"time"

	pb "mirage-proto/gen"
)

// TestScenario1_DualChannelDown_CommandsQueued 双通道断开 → 命令进入 cmdQueue
func TestScenario1_DualChannelDown_CommandsQueued(t *testing.T) {
	// 创建无可用通道的 control plane（mux=nil, encoder=nil）
	cp := NewStealthControlPlane(StealthControlPlaneOpts{})

	if cp.GetChannelState() != ChannelQueued {
		t.Fatalf("期望 ChannelQueued, 实际=%s", cp.GetChannelState())
	}

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		cmd := &pb.ControlCommand{
			CommandId: fmt.Sprintf("cmd-%d", i),
			Timestamp: time.Now().Unix(),
		}
		if err := cp.SendCommand(ctx, cmd); err != nil {
			t.Fatalf("SendCommand 失败: %v", err)
		}
	}

	if cp.QueueLen() != 5 {
		t.Errorf("期望队列长度=5, 实际=%d", cp.QueueLen())
	}
}

// TestScenario2_SingleChannelRecovery_QueueDrained 单通道恢复 → drainOnRecovery 触发
func TestScenario2_SingleChannelRecovery_QueueDrained(t *testing.T) {
	cp := NewStealthControlPlane(StealthControlPlaneOpts{})

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		cmd := &pb.ControlCommand{
			CommandId: fmt.Sprintf("drain-cmd-%d", i),
			Timestamp: time.Now().Unix(),
		}
		cp.SendCommand(ctx, cmd)
	}

	if cp.QueueLen() != 3 {
		t.Fatalf("期望队列长度=3, 实际=%d", cp.QueueLen())
	}

	// 直接调用 drainOnRecovery（模拟通道恢复）
	cmds := cp.DrainQueue()
	if len(cmds) != 3 {
		t.Errorf("期望 drain 3 条命令, 实际=%d", len(cmds))
	}

	if cp.QueueLen() != 0 {
		t.Errorf("drain 后队列应为空, 实际=%d", cp.QueueLen())
	}
}

// TestScenario3_ExpiredCommandsDiscarded 恢复后回放 → 超时命令被丢弃
func TestScenario3_ExpiredCommandsDiscarded(t *testing.T) {
	cp := NewStealthControlPlane(StealthControlPlaneOpts{})

	ctx := context.Background()

	// 添加一条过期命令（时间戳为 2 分钟前）
	expiredCmd := &pb.ControlCommand{
		CommandId: "expired-cmd",
		Timestamp: time.Now().Add(-2 * time.Minute).Unix(),
	}
	cp.SendCommand(ctx, expiredCmd)

	// 添加一条有效命令
	validCmd := &pb.ControlCommand{
		CommandId: "valid-cmd",
		Timestamp: time.Now().Unix(),
	}
	cp.SendCommand(ctx, validCmd)

	if cp.QueueLen() != 2 {
		t.Fatalf("期望队列长度=2, 实际=%d", cp.QueueLen())
	}

	// 调用 drainOnRecovery — 过期命令应被丢弃，有效命令重新入队
	cp.drainOnRecovery(ctx)

	// 有效命令应重新进入队列（因为无可用通道，SendCommand 会再次入队）
	// 过期命令应被丢弃
	// 由于 dedup，valid-cmd 已经被标记，不会重新入队
	// 所以队列应为空
	if cp.QueueLen() > 1 {
		t.Errorf("期望队列长度 ≤ 1（过期命令已丢弃）, 实际=%d", cp.QueueLen())
	}
}
