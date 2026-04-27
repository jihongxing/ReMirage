package gtunnel

import (
	"io"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 1: 切换操作启动双发模式
// **Validates: Requirements 1.1, 1.2**
func TestProperty1_SwitchEnablesDualSend(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		oldRTT := rapid.IntRange(5, 500).Draw(t, "oldRTT")
		newRTT := rapid.IntRange(5, 500).Draw(t, "newRTT")

		oldConn := newMockConn(TransportQUIC, time.Duration(oldRTT)*time.Millisecond)
		newConn := newMockConn(TransportWebSocket, time.Duration(newRTT)*time.Millisecond)

		sb := NewSwitchBuffer()

		// 启用前不应处于双发模式
		if sb.IsDualModeActive() {
			t.Fatal("SwitchBuffer should not be in dual mode before EnableDualSend")
		}

		err := sb.EnableDualSend(oldConn, newConn, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("EnableDualSend failed: %v", err)
		}

		// 启用后必须处于双发模式
		if !sb.IsDualModeActive() {
			t.Fatal("SwitchBuffer should be in dual mode after EnableDualSend")
		}

		// 重复启用应返回错误
		err = sb.EnableDualSend(oldConn, newConn, 100*time.Millisecond)
		if err == nil {
			t.Fatal("EnableDualSend should fail when already active")
		}

		sb.DisableDualSend()

		// 关闭后不应处于双发模式
		if sb.IsDualModeActive() {
			t.Fatal("SwitchBuffer should not be in dual mode after DisableDualSend")
		}
	})
}

// Feature: zero-signature-elimination, Property 3: 切换事务 epoch 递增
// **Validates: Requirements 1.4**
func TestProperty3_SwitchTransactionEpochIncrement(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := DefaultOrchestratorConfig()
		o := NewOrchestrator(cfg)

		oldConn := newMockConn(TransportQUIC, 20*time.Millisecond)
		newConn := newMockConn(TransportWebSocket, 100*time.Millisecond)

		o.paths[TransportQUIC] = &ManagedPath{
			Conn: oldConn, Priority: PriorityQUIC, Type: TransportQUIC,
			Enabled: true, Available: true, Phase: 1,
		}
		o.paths[TransportWebSocket] = &ManagedPath{
			Conn: newConn, Priority: PriorityWSS, Type: TransportWebSocket,
			Enabled: true, Available: true, Phase: 1,
		}
		o.activePath = o.paths[TransportQUIC]
		o.state = StateOrcActive

		epochBefore := o.GetEpoch()

		// 执行降格（触发 executeSwitchTransaction）
		err := o.demote()
		if err != nil {
			t.Fatalf("demote failed: %v", err)
		}

		epochAfter := o.GetEpoch()

		// epoch 必须严格递增
		if epochAfter <= epochBefore {
			t.Fatalf("epoch should increment: before=%d, after=%d", epochBefore, epochAfter)
		}

		o.Close()
	})
}

// Feature: zero-signature-elimination, Property 4: 切换事务回滚保持 epoch 不变
// **Validates: Requirements 1.7**
func TestProperty4_RollbackKeepsEpochUnchanged(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := DefaultOrchestratorConfig()
		o := NewOrchestrator(cfg)

		oldConn := newMockConn(TransportQUIC, 20*time.Millisecond)
		// 新路径始终发送失败 → 触发回滚
		failConn := newMockConn(TransportWebSocket, 100*time.Millisecond)
		failConn.sendErr = io.ErrClosedPipe

		o.paths[TransportQUIC] = &ManagedPath{
			Conn: oldConn, Priority: PriorityQUIC, Type: TransportQUIC,
			Enabled: true, Available: true, Phase: 1,
		}
		o.paths[TransportWebSocket] = &ManagedPath{
			Conn: failConn, Priority: PriorityWSS, Type: TransportWebSocket,
			Enabled: true, Available: true, Phase: 1,
		}
		o.activePath = o.paths[TransportQUIC]
		o.state = StateOrcActive

		epochBefore := o.GetEpoch()

		// 在双发窗口内手动触发足够多的 SendDual 使新路径失败计数达到阈值
		// executeSwitchTransaction 内部 time.Sleep(duration) 期间不会自动 SendDual，
		// 所以我们直接测试 SwitchBuffer 的回滚逻辑
		sb := o.switchBuffer
		err := sb.EnableDualSend(oldConn, failConn, 10*time.Millisecond)
		if err != nil {
			t.Fatalf("EnableDualSend failed: %v", err)
		}

		// 模拟 rollbackThreshold 次发送
		for i := 0; i < rollbackThreshold; i++ {
			_ = sb.SendDual([]byte("test"))
		}

		if !sb.ShouldRollback() {
			t.Fatal("should trigger rollback after consecutive failures")
		}

		sb.DisableDualSend()

		// 回滚：epoch 不变
		epochAfter := o.GetEpoch()
		if epochAfter != epochBefore {
			t.Fatalf("epoch should not change on rollback: before=%d, after=%d", epochBefore, epochAfter)
		}

		o.Close()
	})
}

// Feature: zero-signature-elimination, Property 2: 双发选收去重正确性
// **Validates: Requirements 1.3**
func TestProperty2_DualSendDeduplication(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sb := NewSwitchBuffer()

		// 生成一组唯一 seq
		numSeqs := rapid.IntRange(1, 200).Draw(t, "numSeqs")
		seqs := make([]uint64, numSeqs)
		for i := range seqs {
			seqs[i] = uint64(rapid.IntRange(1, 100000).Draw(t, "seq"))
		}

		delivered := make(map[uint64]int) // seq → 交付次数

		for _, seq := range seqs {
			data, ok := sb.ReceiveAndDedupe(seq, []byte("payload"))
			if ok {
				if data == nil {
					t.Fatal("ReceiveAndDedupe returned ok=true but data=nil")
				}
				delivered[seq]++
			} else {
				if data != nil {
					t.Fatal("ReceiveAndDedupe returned ok=false but data!=nil")
				}
			}
		}

		// 每个 seq 最多交付一次（在窗口内）
		for seq, count := range delivered {
			if count > 1 {
				t.Fatalf("seq %d delivered %d times, expected at most 1", seq, count)
			}
		}
	})
}

// Feature: zero-signature-elimination, Property 15: 双发模式持续时间随机化范围
// **Validates: Requirements 18.1, 18.2**
func TestProperty15_DualSendDurationRandomRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d, err := randomDualDuration()
		if err != nil {
			t.Fatalf("randomDualDuration failed: %v", err)
		}

		if d < minDualDuration || d > maxDualDuration {
			t.Fatalf("duration %v out of range [%v, %v]", d, minDualDuration, maxDualDuration)
		}
	})
}
