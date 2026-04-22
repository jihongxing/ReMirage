package stealth

import (
	"fmt"
	"testing"
	"time"

	pb "mirage-proto/gen"

	"pgregory.net/rapid"
)

type mockAuditCollector struct {
	lostCommands []string
}

func (m *mockAuditCollector) OnCommandLost(commandID string, ct pb.ControlCommandType) {
	m.lostCommands = append(m.lostCommands, commandID)
}

// TestProperty12_RetryLimit verifies that CommandTracker retries do not exceed maxRetry.
// **Validates: Requirements 9.3**
func TestProperty12_RetryLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxRetry := rapid.IntRange(1, 5).Draw(t, "maxRetry")
		audit := &mockAuditCollector{}
		tracker := NewCommandTracker(1*time.Millisecond, maxRetry, audit)

		cmdID := fmt.Sprintf("cmd-%d", rapid.IntRange(1, 10000).Draw(t, "id"))
		cmd := &pb.ControlCommand{
			CommandId:   cmdID,
			CommandType: pb.ControlCommandType_PERSONA_FLIP,
			Epoch:       1,
			Timestamp:   time.Now().UnixNano(),
		}

		tracker.Track(cmd)

		totalRetries := 0
		for i := 0; i < maxRetry+5; i++ {
			time.Sleep(2 * time.Millisecond)
			resend := tracker.CheckTimeouts()
			totalRetries += len(resend)
		}

		// Total retries should not exceed maxRetry
		if totalRetries > maxRetry {
			t.Fatalf("total retries %d > maxRetry %d", totalRetries, maxRetry)
		}

		// Command should have been removed and reported
		if tracker.PendingCount() != 0 {
			t.Fatalf("expected 0 pending, got %d", tracker.PendingCount())
		}
		if len(audit.lostCommands) != 1 {
			t.Fatalf("expected 1 lost command, got %d", len(audit.lostCommands))
		}
	})
}

// TestCommandTracker_AcknowledgeRemoves verifies Acknowledge removes from pending.
func TestCommandTracker_AcknowledgeRemoves(t *testing.T) {
	tracker := NewCommandTracker(5*time.Second, 3, nil)
	cmd := &pb.ControlCommand{
		CommandId:   "ack-test",
		CommandType: pb.ControlCommandType_ROLLBACK,
		Epoch:       1,
		Timestamp:   time.Now().UnixNano(),
	}

	tracker.Track(cmd)
	if tracker.PendingCount() != 1 {
		t.Fatalf("expected 1 pending")
	}

	tracker.Acknowledge("ack-test")
	if tracker.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after ack")
	}
}

// TestCommandTracker_TimeoutResend verifies timed-out commands are returned for resend.
func TestCommandTracker_TimeoutResend(t *testing.T) {
	audit := &mockAuditCollector{}
	tracker := NewCommandTracker(1*time.Millisecond, 3, audit)

	cmd := &pb.ControlCommand{
		CommandId:   "timeout-test",
		CommandType: pb.ControlCommandType_BUDGET_SYNC,
		Epoch:       1,
		Timestamp:   time.Now().UnixNano(),
	}

	tracker.Track(cmd)
	time.Sleep(5 * time.Millisecond)

	resend := tracker.CheckTimeouts()
	if len(resend) != 1 {
		t.Fatalf("expected 1 resend, got %d", len(resend))
	}
	if resend[0].CommandId != "timeout-test" {
		t.Fatalf("wrong command resent")
	}
}

// TestCommandTracker_MaxRetryTriggersAudit verifies max retry triggers AuditCollector.
func TestCommandTracker_MaxRetryTriggersAudit(t *testing.T) {
	audit := &mockAuditCollector{}
	tracker := NewCommandTracker(1*time.Millisecond, 2, audit)

	cmd := &pb.ControlCommand{
		CommandId:   "max-retry-test",
		CommandType: pb.ControlCommandType_SURVIVAL_MODE_CHANGE,
		Epoch:       1,
		Timestamp:   time.Now().UnixNano(),
	}

	tracker.Track(cmd)

	// Exhaust retries
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Millisecond)
		tracker.CheckTimeouts()
	}

	if len(audit.lostCommands) != 1 || audit.lostCommands[0] != "max-retry-test" {
		t.Fatalf("expected lost command 'max-retry-test', got %v", audit.lostCommands)
	}
	if tracker.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after max retry")
	}
}
