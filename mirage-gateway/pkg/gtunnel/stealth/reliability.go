package stealth

import (
	"sync"
	"time"

	pb "mirage-proto/gen"
)

// AuditCollector is a local interface to avoid circular imports.
type AuditCollector interface {
	OnCommandLost(commandID string, commandType pb.ControlCommandType)
}

// pendingCommand tracks a command awaiting acknowledgment.
type pendingCommand struct {
	cmd     *pb.ControlCommand
	retries int
	sentAt  time.Time
}

// CommandTracker tracks command reliability for Scheme B.
type CommandTracker struct {
	pending  sync.Map
	timeout  time.Duration
	maxRetry int
	audit    AuditCollector
}

// NewCommandTracker creates a command tracker.
// timeout defaults to 5000ms, maxRetry defaults to 3.
func NewCommandTracker(timeout time.Duration, maxRetry int, audit AuditCollector) *CommandTracker {
	if timeout <= 0 {
		timeout = 5000 * time.Millisecond
	}
	if maxRetry <= 0 {
		maxRetry = 3
	}
	return &CommandTracker{
		timeout:  timeout,
		maxRetry: maxRetry,
		audit:    audit,
	}
}

// Track begins tracking a command.
func (t *CommandTracker) Track(cmd *pb.ControlCommand) {
	t.pending.Store(cmd.CommandId, &pendingCommand{
		cmd:     cmd,
		retries: 0,
		sentAt:  time.Now(),
	})
}

// Acknowledge confirms a command was received.
func (t *CommandTracker) Acknowledge(commandID string) {
	t.pending.Delete(commandID)
}

// CheckTimeouts returns commands that have timed out and need resending.
// Commands exceeding maxRetry are removed and reported via AuditCollector.
func (t *CommandTracker) CheckTimeouts() []*pb.ControlCommand {
	var resend []*pb.ControlCommand
	now := time.Now()

	t.pending.Range(func(key, value any) bool {
		pc := value.(*pendingCommand)
		if now.Sub(pc.sentAt) > t.timeout {
			pc.retries++
			if pc.retries > t.maxRetry {
				t.pending.Delete(key)
				if t.audit != nil {
					t.audit.OnCommandLost(pc.cmd.CommandId, pc.cmd.CommandType)
				}
			} else {
				pc.sentAt = now
				resend = append(resend, pc.cmd)
			}
		}
		return true
	})

	return resend
}

// PendingCount returns the number of pending commands.
func (t *CommandTracker) PendingCount() int {
	count := 0
	t.pending.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// MaxRetry returns the configured max retry count.
func (t *CommandTracker) MaxRetry() int {
	return t.maxRetry
}
